"""FastAPI application composition layer for the Cricket ML Service.

This module wires together route handlers, middleware, and startup logic.
Domain logic lives in dedicated modules:
- xi_service: selection, the displayed win probability, the performance model, the
  simulator, and L4's evaluation report -- the whole prediction surface after P-5
- prediction_service.endpoints: the windowed-form win model, which P-6 removes
- artifact_service: artifact discovery, health, artifacts status
- model_stats_service: model stats scanning and reporting
- training_orchestrator: training subprocess management
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
from typing import Any, Dict, List, Optional, Tuple

from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from ml.xi.simulator import SimulationUnavailable

from . import settings as app_settings
from . import training_orchestrator, xi_service
from .artifact_service import build_artifacts_status, build_health_response
from .artifacts import reload as reload_artifacts
from .artifacts import summary as artifacts_summary
from .errors import error_payload
from .logging import bind_request_context, get_struct_logger, init_logging
from .model_metadata import get_model_metadata
from .model_stats_service import build_model_stats
from .models.predict import (
    WinFeatures,
    WinFeaturesEnhanced,
    WinPrediction,
)
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
from .prediction_service.endpoints import (
    run_win_prediction,
    run_win_prediction_enhanced,
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
MAX_PREDICT_BATCH_SIZE = _settings.max_predict_batch_size
TRAIN_LATEST_CACHE_GRANULARITY = _settings.train_latest_cache_granularity
MODEL_STATS_CACHE_TTL = _settings.model_stats_cache_ttl

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
# Model stats cache
# ---------------------------------------------------------------------------

_model_stats_cache: Optional[Tuple[float, Dict[str, Any]]] = None

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


def _reload_artifacts() -> dict:
    """Rescan MODELS_DIR, reload the registries, and drop the model-stats cache it feeds."""
    global _model_stats_cache
    _model_stats_cache = None
    reload_artifacts(MODELS_DIR)
    xi_service.REGISTRY.reload(MODELS_DIR)
    summary = artifacts_summary()
    summary.update(xi_service.loaded_formats())
    return summary


try:
    logger.info("startup.artifacts.load.start", models_dir=MODELS_DIR)
    reload_artifacts(MODELS_DIR)
    xi_service.REGISTRY.reload(MODELS_DIR)
    logger.info("startup.artifacts.load.done", models_dir=MODELS_DIR)
except Exception as e:
    logger.error("startup.artifacts.load.failed", models_dir=MODELS_DIR, error=str(e), exc_info=True)


# ---------------------------------------------------------------------------
# Route handlers — thin wrappers delegating to service modules
# ---------------------------------------------------------------------------


@app.get("/health")
async def health():
    logger.info("health.check.start")
    return build_health_response(MODELS_DIR)


@app.get("/artifacts/status")
async def artifacts_status():
    """Report presence and loaded state of artifacts per format and legacy."""
    return build_artifacts_status(MODELS_DIR)


@app.get("/model-metadata")
async def model_metadata():
    """Return the win model's metadata: feature order, output, artifact naming."""
    try:
        return get_model_metadata()
    except Exception as e:
        logger.exception("model_metadata.error", error=str(e))
        raise HTTPException(status_code=500, detail={"code": "METADATA_ERROR", "message": str(e)}) from e


@app.get("/model-stats")
async def model_stats():
    """Return details of trained ML models: name, format, tuned params, algorithm, accuracy, size, modified."""
    global _model_stats_cache
    try:
        if MODEL_STATS_CACHE_TTL > 0 and _model_stats_cache is not None:
            ts, payload = _model_stats_cache
            if (time.time() - ts) <= MODEL_STATS_CACHE_TTL:
                return payload
        result = build_model_stats(MODELS_DIR)
        if MODEL_STATS_CACHE_TTL > 0:
            _model_stats_cache = (time.time(), result)
        return result
    except Exception as e:
        logger.exception("model_stats.error", error=str(e))
        raise HTTPException(status_code=500, detail={"code": "MODEL_STATS_ERROR", "message": str(e)}) from e


@app.post("/predict/win", response_model=List[WinPrediction])
async def predict_win(features: List[WinFeatures]):
    if len(features) > MAX_PREDICT_BATCH_SIZE:
        raise HTTPException(
            status_code=413,
            detail=error_payload(
                code="BATCH_TOO_LARGE",
                message=f"win batch of {len(features)} exceeds the limit of {MAX_PREDICT_BATCH_SIZE}",
                hint="Split the request, or raise MAX_PREDICT_BATCH_SIZE.",
            ),
        )
    return run_win_prediction(features)


@app.post("/predict/win-enhanced", response_model=WinPrediction)
async def predict_win_enhanced(request: WinFeaturesEnhanced):
    """Enhanced win prediction using per-player features with on-the-fly aggregation.

    Accepts per-player feature maps for both teams and computes distribution
    statistics (mean, std, max, min, top3_mean) and derived matchup features
    before running the win model.
    """
    return run_win_prediction_enhanced(
        fmt=request.format or "",
        match_context=request.to_match_context_dict(),
        team1_player_features=request.team1_player_features,
        team2_player_features=request.team2_player_features,
    )


@app.post("/admin/reload")
async def admin_reload(request: Request):
    """Rescan the models directory and reload artifacts."""
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
    logger.info("admin.reload.start", models_dir=MODELS_DIR)
    try:
        summary = _reload_artifacts()
        logger.info("admin.reload.success", models_dir=MODELS_DIR, summary=summary)
        return {"status": "reloaded", **summary}
    except Exception as e:
        logger.error("admin.reload.failed", models_dir=MODELS_DIR, error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=_error_payload(code="RELOAD_FAILED", message="Artifact reload failed", hint=str(e)),
        ) from e


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
            # Training just wrote new artifact files. Without this the process keeps serving
            # the objects it loaded at startup, so a COMPLETED run never reaches inference.
            # A reload failure does not fail the run: the artifacts are on disk either way.
            try:
                summary = await asyncio.to_thread(_reload_artifacts)
                logger.info("admin.train.artifacts_reloaded", step=step, summary=summary)
            except Exception as e:
                logger.error(
                    "admin.train.artifacts_reload_failed",
                    step=step,
                    models_dir=MODELS_DIR,
                    error=str(e),
                    exc_info=True,
                )
            return response

        return wrapped

    return decorator


@app.post("/admin/train/win")
@_require_admin_train("win", "Win training failed")
async def admin_train_win(request: Request, cutoff: str = ""):
    """Run win model training. Requires cutoff."""
    cutoff = (cutoff or "").strip()
    if not cutoff:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="CUTOFF_REQUIRED",
                message="Win training requires cutoff",
                hint="Pass query param cutoff (RFC3339), e.g. ?cutoff=2025-01-01T00:00:00Z",
            ),
        )
    async with _get_training_semaphore():
        await asyncio.to_thread(training_orchestrator.run_win_training, cutoff, _settings.go_app_url, logger)
    return training_orchestrator.train_response("win")


@app.post("/admin/train/auto-tune")
@_require_admin_train("auto-tune", "Auto-tune failed")
async def admin_train_auto_tune(
    request: Request,
    cutoff: str = "",
    model: str = "all",
    format: str = "",
    all_formats: str = "",
    rescreen: str = "",
    algorithms: str = "",
):
    """Run auto-tune hyperparameter search."""
    cutoff = (cutoff or "").strip()
    if not cutoff:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="CUTOFF_REQUIRED",
                message="Auto-tune requires cutoff",
                hint="Pass query param cutoff (RFC3339), e.g. ?cutoff=2025-01-01T00:00:00Z",
            ),
        )
    use_all_formats = (all_formats or "").strip().lower() in ("1", "true", "yes")
    do_rescreen = (rescreen or "").strip().lower() in ("1", "true", "yes")
    async with _get_training_semaphore():
        await asyncio.to_thread(
            training_orchestrator.run_auto_tune,
            cutoff,
            _settings.go_app_url,
            model,
            use_all_formats,
            (format or "").strip(),
            do_rescreen,
            (algorithms or "").strip(),
            logger,
        )
    return {"status": "ok", "step": "auto-tune"}


@app.get("/admin/train/progress")
async def admin_train_progress(request: Request, step: str = "", run_id: str = ""):
    """Return live progress for a training step (ops plan O-3).

    `step` is a pipeline step id (`train_batting`, `auto_tune`, ...). With no `run_id`
    this reports the live run -- the newest non-stale progress file for that step.
    Naming a `run_id` reads exactly that run, finished or not.

    An empty object means "nothing is running", which is a normal answer, not an error:
    go-app polls this on a timer and a 404 per tick would be noise.
    """
    _verify_admin_api_key(request)
    return training_orchestrator.get_step_progress(step, run_id)


@app.get("/admin/train/auto-tune/progress")
async def admin_train_auto_tune_progress(request: Request):
    """Return current auto-tune progress (if running).

    The auto-tune view of `/admin/train/progress`. It delegates rather than
    duplicating; it survives because it is a released endpoint.
    """
    _verify_admin_api_key(request)
    return training_orchestrator.get_auto_tune_progress()


# ---------------------------------------------------------------------------
# XI-responsive win model (S-10): every input is a function of the two elevens.
# ---------------------------------------------------------------------------


@app.get("/xi/status", response_model=XiStatusResponse)
async def xi_status():
    """Which formats have an XI win model loaded, how far the ratings run, and the last training report."""
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
