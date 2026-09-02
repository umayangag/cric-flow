"""FastAPI application composition layer for the Cricket ML Service.

This module wires together route handlers, middleware, and startup logic.
Domain logic lives in dedicated modules:
- xi_service: selection, the displayed win probability, the performance model, the
  simulator, L4's evaluation report, and which run is loaded -- the whole surface
- ml.xi.runs: run identity (H-16) -- the manifest, the `current` pointer, the refusal
- training_orchestrator: retrain and evaluate subprocess management
"""

import asyncio
import functools
import hmac
import os
import sys
import threading
import time
import traceback
import uuid
from contextlib import asynccontextmanager
from typing import Any, Optional

from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from ml.xi import runs
from ml.xi.runs import RunArtifactsInvalid
from ml.xi.simulator import SimulationUnavailable

from . import settings as app_settings
from . import training_orchestrator, xi_service
from .errors import error_payload
from .logging import bind_request_context, get_struct_logger, init_logging
from .models.xi import (
    PerformancePredictRequest,
    PerformancePredictResponse,
    SimulateRequest,
    SimulateResponse,
    XiOptimizeRequest,
    XiOptimizeResponse,
    XiStatusResponse,
    XiWinRequest,
    XiWinResponse,
)

# ---------------------------------------------------------------------------
# Lifespan
# ---------------------------------------------------------------------------


@asynccontextmanager
async def _lifespan(app: FastAPI) -> Any:
    try:  # pragma: no cover - lifespan startup; tested indirectly via TestClient
        from ml.tracking import cancel_in_progress_on_startup, parse_stale_cancel_age_seconds

        stale_seconds = parse_stale_cancel_age_seconds()
        n = cancel_in_progress_on_startup(stale_seconds=stale_seconds)
        if n:
            logger.info("startup.cancelled_stale_migrations", count=n, stale_seconds=stale_seconds)
    except Exception as e:  # pragma: no cover
        logger.warning("startup.cancel_stale_migrations_failed", error=str(e))
    yield
    logger.info(
        "shutdown.complete", message="ml-service shutting down; check logs for errors if process exited unexpectedly"
    )


app = FastAPI(title="Cricket ML Service", version="0.3.0", lifespan=_lifespan)

# Initialize logging early
init_logging(service="ml-service", version=app.version)
logger = get_struct_logger()


# ---------------------------------------------------------------------------
# Crash logging
# ---------------------------------------------------------------------------


def _install_crash_logging() -> None:
    """Ensure uncaught exceptions and thread crashes are logged before exit."""
    _orig_excepthook = sys.excepthook

    def _excepthook(exc_type: type, exc_value: BaseException, exc_tb: Any) -> None:
        logger.critical(
            "unhandled_exception",
            error_type=exc_type.__name__ if exc_type else "",
            error=str(exc_value) if exc_value else "",
            traceback="".join(traceback.format_exception(exc_type, exc_value, exc_tb))
            if exc_type and exc_value
            else "",
        )
        if _orig_excepthook and _orig_excepthook is not _excepthook:
            _orig_excepthook(exc_type, exc_value, exc_tb)

    sys.excepthook = _excepthook

    _orig_thread_excepthook = getattr(threading, "excepthook", None)

    def _thread_excepthook(args: Any) -> None:
        exc_type = getattr(args, "exc_type", None)
        exc_value = getattr(args, "exc_value", None)
        exc_tb = getattr(args, "exc_traceback", None)
        thread = getattr(args, "thread", None)
        logger.critical(
            "unhandled_thread_exception",
            error_type=exc_type.__name__ if exc_type else "",
            error=str(exc_value) if exc_value else "",
            thread_name=getattr(thread, "name", "") if thread else "",
            traceback="".join(traceback.format_exception(exc_type, exc_value, exc_tb))
            if exc_type and exc_value
            else "",
        )
        if _orig_thread_excepthook:
            _orig_thread_excepthook(args)

    threading.excepthook = _thread_excepthook


_install_crash_logging()

# ---------------------------------------------------------------------------
# Centralized configuration
# ---------------------------------------------------------------------------

_settings = app_settings.load_ml_service_settings()

ENABLE_HOT_RELOAD = _settings.enable_hot_reload
ADMIN_API_KEY = _settings.admin_api_key
MAX_CONCURRENT_TRAINING_JOBS = _settings.max_concurrent_training_jobs

_training_semaphore: Optional[asyncio.Semaphore] = None


def _get_training_semaphore() -> asyncio.Semaphore:
    global _training_semaphore
    if _training_semaphore is None:
        _training_semaphore = asyncio.Semaphore(MAX_CONCURRENT_TRAINING_JOBS)
    return _training_semaphore


def _verify_admin_api_key(request: Request) -> None:
    """If ADMIN_API_KEY is set, require X-API-Key header. Raises HTTPException 401 if invalid."""
    if not ADMIN_API_KEY:
        return
    client_key = (request.headers.get("X-API-Key") or "").strip()
    if not hmac.compare_digest(client_key.encode("utf-8"), ADMIN_API_KEY.encode("utf-8")):
        raise HTTPException(
            status_code=401,
            detail=error_payload(
                code="UNAUTHORIZED",
                message="Invalid or missing API key",
                hint="Set X-API-Key header to ADMIN_API_KEY value.",
            ),
        )


# ---------------------------------------------------------------------------
# CORS
# ---------------------------------------------------------------------------

_allowed_origins = _settings.frontend_allowed_origins
app.add_middleware(
    CORSMiddleware,
    allow_origins=_allowed_origins,
    allow_credentials=True,
    allow_methods=_settings.cors_allow_methods,
    allow_headers=_settings.cors_allow_headers,
)

# ---------------------------------------------------------------------------
# Request context middleware
# ---------------------------------------------------------------------------

_error_payload = error_payload


@app.middleware("http")
async def request_context_middleware(request: Request, call_next):
    rid = request.headers.get("X-Request-ID") or str(uuid.uuid4())
    bind_request_context(rid)
    start = time.time()
    logger.info("request.start", method=request.method, path=request.url.path)
    try:
        response: Response = await call_next(request)
    except Exception as exc:
        logger.exception("request.error", method=request.method, path=request.url.path)
        data = error_payload(
            code="UNHANDLED_EXCEPTION",
            message=str(exc) or exc.__class__.__name__,
            hint="Check server logs with the provided request_id for details.",
        )
        response = JSONResponse(status_code=500, content={"detail": data})
    finally:
        duration_ms = round((time.time() - start) * 1000, 2)
        status = getattr(response, "status_code", 0)
        logger.info(
            "request.end",
            method=request.method,
            path=request.url.path,
            status_code=status,
            duration_ms=duration_ms,
        )
    response.headers["X-Request-ID"] = rid
    return response


# ---------------------------------------------------------------------------
# Artifact loading
# ---------------------------------------------------------------------------

try:
    import config as svc_config
except Exception:
    svc_config = None  # type: ignore

MODELS_DIR = app_settings.get_models_dir(svc_config)
os.makedirs(MODELS_DIR, exist_ok=True)


def _reload_run(run_id: Optional[str] = None) -> dict:
    """Load a run and serve it, returning what /xi/status would say about the result."""
    return xi_service.REGISTRY.reload(MODELS_DIR, run_id)


try:
    logger.info("startup.artifacts.load.start", models_dir=MODELS_DIR)
    _reload_run()
    logger.info("startup.artifacts.load.done", models_dir=MODELS_DIR)
except Exception as e:
    logger.error("startup.artifacts.load.failed", models_dir=MODELS_DIR, error=str(e), exc_info=True)


# ---------------------------------------------------------------------------
# Route handlers — thin wrappers delegating to service modules
# ---------------------------------------------------------------------------


@app.get("/health")
async def health():
    """Liveness plus the one fact that decides whether a prediction can be served: which
    run is loaded, how far its ratings go, and whether they are fresh enough (H-11, H-16)."""
    logger.info("health.check.start")
    status = xi_service.status()
    return {
        "status": "ok",
        "models_dir": MODELS_DIR,
        "loaded": status.loaded,
        "run_id": status.run_id,
        "loaded_xi_formats": status.formats,
        "loaded_performance_formats": status.performance_formats,
        "ratings": None if status.ratings is None else status.ratings.model_dump(),
        "error": status.error,
    }


@app.get("/artifacts/status")
async def artifacts_status():
    """Every run on disk, which one `current` points at, and which one is loaded.

    It reports runs rather than a formats-by-model-kind matrix because a run is what an
    artifact belongs to now (H-16). A run that was refused appears here with the reason
    (D-6): an empty panel and a refused artifact set look the same otherwise, and only
    one of them is something an operator has to act on.
    """
    status = xi_service.status()
    current = runs.read_current(MODELS_DIR)
    listing = runs.list_runs(MODELS_DIR)
    for entry in listing:
        entry["current"] = entry.get("run_id") == current
        entry["loaded"] = entry.get("run_id") == status.run_id
    return {
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "root": MODELS_DIR,
        "reachable": True,
        "current_run": current,
        "loaded_run": status.run_id,
        "ratings_through": status.ratings_through,
        "ratings": None if status.ratings is None else status.ratings.model_dump(),
        "error": status.error,
        "runs": listing,
    }


@app.post("/admin/reload")
async def admin_reload(request: Request, run: str = ""):
    """Point `current` at a run and load it -- the `reload` pipeline step.

    ``?run=<id>`` names the run, which is how an operator swaps between two runs. With
    no ``run`` it loads whichever run `current` names, and if nothing does, the newest
    one -- so `retrain` followed by `reload` serves the run just built.
    """
    if not ENABLE_HOT_RELOAD:
        logger.info("admin.reload.rejected", reason="disabled")
        raise HTTPException(
            status_code=403,
            detail=_error_payload(
                code="RELOAD_DISABLED",
                message="Hot reload is disabled",
                hint="Set ENABLE_HOT_RELOAD=1 to enable /admin/reload.",
            ),
        )
    _verify_admin_api_key(request)
    run_id = (run or "").strip() or None
    logger.info("admin.reload.start", models_dir=MODELS_DIR, run_id=run_id)
    try:
        summary = _reload_run(run_id)
    except RunArtifactsInvalid as e:
        # A named run that is not one, or whose arrays this code cannot serve (D-6).
        # 409 rather than 500: nothing is broken, the request named something unusable.
        logger.error("admin.reload.refused", models_dir=MODELS_DIR, run_id=run_id, error=str(e))
        raise HTTPException(
            status_code=409,
            detail=_error_payload(
                code="RUN_ARTIFACTS_INVALID",
                message=str(e),
                hint="run the retrain step to produce a run this code wrote, then reload",
            ),
        ) from e
    except Exception as e:
        logger.error("admin.reload.failed", models_dir=MODELS_DIR, run_id=run_id, error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=_error_payload(code="RELOAD_FAILED", message="Artifact reload failed", hint=str(e)),
        ) from e
    if summary.get("error"):
        # The run was refused on load rather than by a raised exception: the registry
        # keeps serving whatever it had, and the refusal is the answer.
        raise HTTPException(
            status_code=409,
            detail=_error_payload(
                code="RUN_ARTIFACTS_INVALID",
                message=summary["error"],
                hint="run the retrain step to produce a run this code wrote, then reload",
            ),
        )
    logger.info("admin.reload.success", models_dir=MODELS_DIR, run_id=summary.get("run_id"))
    return {"status": "reloaded", **summary}


def _require_admin_train(step: str, fail_message: str):
    """Decorator for /admin/train/* endpoints: ENABLE_HOT_RELOAD check, admin API key verification, ValueError -> 500."""

    def decorator(f):
        @functools.wraps(f)
        async def wrapped(request: Request, *args: Any, **kwargs: Any) -> Any:
            if not ENABLE_HOT_RELOAD:
                logger.info("admin.train.rejected", step=step, reason="disabled")
                raise HTTPException(
                    status_code=403,
                    detail=_error_payload(
                        code="TRAIN_DISABLED",
                        message="Admin train is disabled",
                        hint="Set ENABLE_HOT_RELOAD=1 to enable /admin/train/*.",
                    ),
                )
            _verify_admin_api_key(request)
            try:
                response = await f(request, *args, **kwargs)
            except ValueError as e:
                logger.error("admin.train.failed", step=step, error=str(e), exc_info=True)
                raise HTTPException(
                    status_code=500,
                    detail=_error_payload(
                        code="TRAIN_FAILED",
                        message=fail_message,
                        hint="Check server logs with the provided request_id for details.",
                    ),
                ) from e
            return response

        return wrapped

    return decorator


@app.post("/admin/train/retrain")
@_require_admin_train("retrain", "Retrain failed")
async def admin_train_retrain(request: Request, cutoff: str = ""):
    """Build one run: rating pass, XI win models, performance models, report, manifest.

    It publishes nothing. `reload` moves `current`, so a retrain that turns out badly
    leaves the run before it exactly where it was.
    """
    cutoff = (cutoff or "").strip()
    if not cutoff:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="CUTOFF_REQUIRED",
                message="Retrain requires cutoff",
                hint="Pass query param cutoff (RFC3339 or YYYY-MM-DD), e.g. ?cutoff=2025-09-01",
            ),
        )
    async with _get_training_semaphore():
        await asyncio.to_thread(training_orchestrator.run_retrain, cutoff, MODELS_DIR, logger)
    return training_orchestrator.train_response("retrain")


@app.post("/admin/train/evaluate")
@_require_admin_train("evaluate", "Evaluate failed")
async def admin_train_evaluate(request: Request, cutoff: str = ""):
    """Run L4 and write its report. Touches no artifact `current` points at."""
    async with _get_training_semaphore():
        await asyncio.to_thread(training_orchestrator.run_evaluate, (cutoff or "").strip(), MODELS_DIR, logger)
    return training_orchestrator.train_response("evaluate")


@app.get("/admin/train/progress")
async def admin_train_progress(request: Request, step: str = "", run_id: str = ""):
    """Return live progress for a training step (ops plan O-3).

    `step` is a pipeline step id (`retrain`, `evaluate`). With no `run_id`
    this reports the live run -- the newest non-stale progress file for that step.
    Naming a `run_id` reads exactly that run, finished or not.

    An empty object means "nothing is running", which is a normal answer, not an error:
    go-app polls this on a timer and a 404 per tick would be noise.
    """
    _verify_admin_api_key(request)
    return training_orchestrator.get_step_progress(step, run_id)


# ---------------------------------------------------------------------------
# XI-responsive win model (S-10): every input is a function of the two elevens.
# ---------------------------------------------------------------------------


@app.get("/xi/status", response_model=XiStatusResponse)
async def xi_status():
    """Which run is loaded and what its manifest says (H-16), which formats it serves, how
    far its ratings run and whether they are fresh enough to answer with (H-11), and the
    run's own training report."""
    return xi_service.status()


@app.get("/xi/evaluate-report")
async def xi_evaluate_report():
    """L4's evaluation report: the walk-forward table, the locked window, per-target
    performance metrics with width beside coverage, the simulator's E2 section, the
    selection metrics and the train/serve parity check -- everything `make xi-evaluate`
    measured, as the backtest surfaces render it."""
    try:
        return xi_service.evaluate_report()
    except xi_service.XiUnavailable as exc:
        raise HTTPException(status_code=503, detail=exc.payload) from exc


@app.post("/xi/predict-win", response_model=XiWinResponse)
async def xi_predict_win(request: XiWinRequest):
    """P(team1 wins) for two elevens given by player id. team1 is the side batting first."""
    try:
        return xi_service.predict_win(request)
    except xi_service.XiUnavailable as exc:
        raise HTTPException(status_code=503, detail=exc.payload) from exc


@app.post("/performance/predict", response_model=PerformancePredictResponse)
async def performance_predict(request: PerformancePredictRequest):
    """Per-player performance distributions (L2-B) for two elevens given by player id:
    median and 10-90 range of runs, balls faced and runs conceded; P(bats) / P(bowls);
    wicket probabilities P(0), P(1), P(2+). Both batting orders are averaged unless
    ``team1_bats_first`` is given."""
    try:
        return xi_service.predict_performance(request)
    except xi_service.XiUnavailable as exc:
        raise HTTPException(status_code=503, detail=exc.payload) from exc


@app.post("/simulate", response_model=SimulateResponse)
async def simulate(request: SimulateRequest):
    """Draw the match from the performance model's forecasts for two elevens (L2-C): each
    side's total (median, 10-90), per-player ranges and the median-band scorecard that sums
    to the total, the margin, P(win) by simulation beside the display model's, and each
    player's contribution to the total's spread. Limited-overs formats only."""
    try:
        return xi_service.simulate(request)
    except xi_service.XiUnavailable as exc:
        raise HTTPException(status_code=503, detail=exc.payload) from exc
    except SimulationUnavailable as exc:
        raise HTTPException(
            status_code=422,
            detail=_error_payload(code="SIMULATION_UNSUPPORTED_FORMAT", message=str(exc), hint="use T20, T20I or ODI"),
        ) from exc


@app.post("/xi/optimize", response_model=XiOptimizeResponse)
async def xi_optimize(request: XiOptimizeRequest):
    """Pick the XI from a pool: ``objective="win"`` maximises P(win) against a fixed opponent
    XI, ``objective="ratings"`` returns the rating-ordered pick and maximises nothing.

    The objective is a function of the eleven ids only (plus the opponent's), so the search
    inside is exact with respect to what the model can express; everything the caller decides
    -- availability, format, who the opponent fields -- comes in as the pool and the opponent list.

    A format whose objective does not rank (H-17: TEST) is offered ``"ratings"`` only, and the
    response says ``optimised: false`` so every consumer down to the UI can label it.
    """
    try:
        return xi_service.optimize(request)
    except xi_service.XiUnavailable as exc:
        raise HTTPException(status_code=503, detail=exc.payload) from exc
    except ValueError as exc:
        raise HTTPException(
            status_code=422,
            detail=_error_payload(
                code="OPTIMIZATION_CONSTRAINT_ERROR",
                message=str(exc),
                hint="Check the pool satisfies the constraints (size, bowling options, keeper).",
            ),
        ) from exc
