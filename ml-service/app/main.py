"""FastAPI application composition layer for the Cricket ML Service.

This module wires together route handlers, middleware, and startup logic.
Domain logic lives in dedicated modules:
- prediction_service: backtest predictions, individual predict endpoints
- artifact_service: artifact discovery, health, artifacts status
- model_stats_service: model stats scanning and reporting
- training_orchestrator: training subprocess management
- backtest_service: historical backtest, feature building
- backtest_cache: in-memory prediction cache
"""

import asyncio
import functools
import hmac
import math
import os
import sys
import threading
import time
import traceback
import uuid
from contextlib import asynccontextmanager
from datetime import timezone
from typing import Any, Dict, List, Optional, Tuple

from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from . import settings as app_settings
from . import training_orchestrator
from .artifact_service import build_artifacts_status, build_health_response
from .artifacts import reload as reload_artifacts
from .artifacts import summary as artifacts_summary
from .backtest_cache import BacktestCache
from .backtest_service import DeterministicInMemoryRepo
from .backtest_service import historical_backtest as svc_historical_backtest
from .backtest_service import predict_match_baseline as svc_predict_match_baseline
from .backtest_service import resolve_model_version as svc_resolve_model_version
from .errors import error_payload
from .logging import bind_request_context, get_struct_logger, init_logging
from .model_metadata import get_model_metadata
from .model_stats_service import build_model_stats
from .models.backtest import (
    BacktestMatchResponse,
    BacktestPlayersResponse,
    BacktestPredictRequest,
    BatchPredictRequest,
    BatchPredictResponse,
    BatchPredictResultItem,
    GenerateMatchRequest,
    GenerateMatchResponse,
    HistoricalMatchBacktestRequest,
)
from .models.features import BattingFeatures, BowlingFeatures
from .models.predict import (
    BattingPrediction,
    BowlingPrediction,
    ExtrasFeatures,
    ExtrasPrediction,
    TeamOptimizationRequest,
    TeamOptimizationResponse,
    TeamOptimizationSelectedPlayer,
    WinFeatures,
    WinFeaturesEnhanced,
    WinPrediction,
)
from .prediction_service.endpoints import (
    run_batting_prediction,
    run_bowling_prediction,
    run_extras_prediction,
    run_team_optimization,
    run_win_prediction,
    run_win_prediction_enhanced,
    validate_predict_batch,
)
from .prediction_service.generate_match import generate_match
from .prediction_service.players import predict_players_batch, predict_players_with_features
from .prediction_settings import GenerateMatchSettings

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
DISABLE_BACKTEST_CACHE = _settings.disable_backtest_cache
CACHE_TTL_SECONDS = _settings.backtest_cache_ttl_seconds
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
# Backtest cache
# ---------------------------------------------------------------------------

_backtest_cache = BacktestCache(ttl_seconds=CACHE_TTL_SECONDS, disabled=DISABLE_BACKTEST_CACHE)


def reset_backtest_cache() -> None:
    """Utility for tests to clear cache and counters."""
    _backtest_cache.reset()


def get_backtest_compute_counts() -> Tuple[int, int]:
    return _backtest_cache.get_compute_counts()


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
    """Rescan MODELS_DIR and reload registries using artifacts module."""
    reload_artifacts(MODELS_DIR)
    return artifacts_summary()


try:
    logger.info("startup.artifacts.load.start", models_dir=MODELS_DIR)
    reload_artifacts(MODELS_DIR)
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
    """Return model metadata from the source of truth (feature_vectors.json)."""
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


@app.post("/ml/backtest/predict")
def backtest_predict(req: BacktestPredictRequest):
    cutoff = req.cutoff_date
    cutoff_with_tz = cutoff if cutoff.tzinfo is not None else cutoff.replace(tzinfo=timezone.utc)
    cutoff_utc = cutoff_with_tz.astimezone(timezone.utc)
    cutoff_iso = cutoff_utc.isoformat().replace("+00:00", "Z")
    if req.player_ids is not None:
        use_full_pipeline = (
            req.format is not None and (req.format or "").strip() and req.features is not None and len(req.features) > 0
        )
        if not use_full_pipeline:
            logger.warning(
                "backtest_predict.player_rejected",
                reason="format_and_features_required",
                has_format=bool(req.format and (req.format or "").strip()),
                has_features=bool(req.features and len(req.features) > 0),
                player_count=len(req.player_ids or []),
                cutoff_iso=cutoff_iso,
            )
            raise HTTPException(
                status_code=400,
                detail=error_payload(
                    code="FORMAT_AND_FEATURES_REQUIRED",
                    message="Player predictions require format and features",
                    hint="Send format and features (per-player feature map). No baseline fallback.",
                ),
            )
        logger.info(
            "backtest_predict.player.start",
            format=req.format,
            cutoff_iso=cutoff_iso,
            player_count=len(req.player_ids),
        )
        cached = None if req.match_context else _backtest_cache.get("players", cutoff_iso, list(req.player_ids))
        if cached is not None:
            logger.info("backtest_predict.player.cache_hit", cutoff_iso=cutoff_iso, player_count=len(req.player_ids))
            return JSONResponse(status_code=200, content=cached)
        _backtest_cache.increment_players_compute()
        try:
            preds = predict_players_with_features(
                cutoff,
                req.player_ids,
                req.format or "",
                req.features,
                MODELS_DIR,
                _settings.enable_train_on_the_fly,
                _settings.go_app_url,
                _settings.go_app_api_key or None,
                TRAIN_LATEST_CACHE_GRANULARITY,
                req.use_latest_model,
                req.match_context,
            )
        except ValueError as e:
            logger.exception(
                "backtest_predict.player.train_on_the_fly_failed",
                format=req.format,
                cutoff_iso=cutoff_iso,
                player_count=len(req.player_ids),
                error=str(e),
            )
            raise HTTPException(
                status_code=503,
                detail=error_payload(
                    code="TRAIN_ON_THE_FLY_FAILED",
                    message=str(e),
                    hint="Ensure GO_APP_URL is set and go-app has training data for this format and cutoff.",
                ),
            ) from e
        except Exception as e:
            logger.exception(
                "backtest_predict.player.prediction_failed",
                format=req.format,
                cutoff_iso=cutoff_iso,
                player_count=len(req.player_ids),
                error=str(e),
            )
            raise HTTPException(
                status_code=503,
                detail=error_payload(
                    code="PREDICTION_FAILED",
                    message="Train-on-the-fly or prediction failed",
                    hint=str(e),
                ),
            ) from e
        logger.info(
            "backtest_predict.player.success",
            format=req.format,
            cutoff_iso=cutoff_iso,
            player_count=len(req.player_ids),
            predictions_count=len(preds),
        )
        body = BacktestPlayersResponse(players=preds).model_dump()
        _backtest_cache.put("players", cutoff_iso, list(req.player_ids), body)
        return JSONResponse(status_code=200, content=body)
    if req.teams is not None:
        cached = _backtest_cache.get("match", cutoff_iso, list(req.teams))
        if cached is not None:
            return JSONResponse(status_code=200, content=cached)
        _backtest_cache.increment_match_compute()
        match = svc_predict_match_baseline(cutoff, req.teams)
        body = BacktestMatchResponse(
            match=match, model_version=svc_resolve_model_version(getattr(app, "version", ""))
        ).model_dump()
        _backtest_cache.put("match", cutoff_iso, list(req.teams), body)
        return JSONResponse(status_code=200, content=body)
    logger.warning("backtest_predict.invalid_request", reason="missing_player_ids_and_teams", cutoff_iso=cutoff_iso)
    raise HTTPException(
        status_code=400,
        detail=error_payload(
            code="INVALID_REQUEST",
            message="provide either player_ids or teams",
            hint="Body must include one of: {player_ids:[..]} or {teams:[team1,team2]}",
        ),
    )


@app.post("/ml/backtest/predict-batch", response_model=BatchPredictResponse)
def backtest_predict_batch(req: BatchPredictRequest):
    """Batch prediction: run multiple player-prediction sets in a single HTTP call.

    Each item is equivalent to a POST /ml/backtest/predict with player_ids.
    Models are loaded once and shared across all items in the batch.
    """
    logger.info("backtest_predict_batch.start", batch_size=len(req.requests))
    try:
        all_results = predict_players_batch(
            items=req.requests,
            models_dir=MODELS_DIR,
            enable_train_on_the_fly=_settings.enable_train_on_the_fly,
            go_app_url=_settings.go_app_url,
            go_app_api_key=_settings.go_app_api_key or None,
            train_latest_cache_granularity=TRAIN_LATEST_CACHE_GRANULARITY,
        )
    except ValueError as e:
        logger.exception("backtest_predict_batch.failed", error=str(e))
        raise HTTPException(
            status_code=503,
            detail=error_payload(code="BATCH_PREDICT_FAILED", message=str(e)),
        ) from e
    except Exception as e:
        logger.exception("backtest_predict_batch.error", error=str(e))
        raise HTTPException(
            status_code=503,
            detail=error_payload(code="BATCH_PREDICT_FAILED", message="Batch prediction failed"),
        ) from e
    logger.info("backtest_predict_batch.success", batch_size=len(req.requests))
    return BatchPredictResponse(
        results=[BatchPredictResultItem(players=preds) for preds in all_results],
    )


@app.post("/api/ml/generate-match", response_model=GenerateMatchResponse)
def api_generate_match(req: GenerateMatchRequest):
    """Generate a reconciled match: per-player stats, innings totals, and win probability (§5.1.1)."""
    cutoff = req.cutoff_date
    cutoff_with_tz = cutoff if cutoff.tzinfo is not None else cutoff.replace(tzinfo=timezone.utc)
    try:
        result = generate_match(
            cutoff_with_tz,
            req.player_ids,
            req.format or "",
            req.features or {},
            req.match_context,
            GenerateMatchSettings(
                models_dir=MODELS_DIR,
                enable_train_on_the_fly=_settings.enable_train_on_the_fly,
                go_app_url=_settings.go_app_url,
                go_app_api_key=_settings.go_app_api_key or None,
                train_latest_cache_granularity=TRAIN_LATEST_CACHE_GRANULARITY,
            ),
            req.use_latest_model,
            model_version=svc_resolve_model_version(getattr(app, "version", "")),
        )
    except ValueError as e:
        logger.warning("generate_match.validation_failed", error=str(e))
        raise HTTPException(status_code=400, detail=error_payload(code="VALIDATION_FAILED", message=str(e))) from e
    except Exception as e:
        logger.exception("generate_match.error", error=str(e))
        raise HTTPException(
            status_code=503,
            detail=error_payload(code="GENERATE_MATCH_FAILED", message=str(e)),
        ) from e
    return GenerateMatchResponse(
        players=result["players"],
        innings=result["innings"],
        win_probability_team1=result["win_probability_team1"],
        model_version=result["model_version"],
    )


@app.post("/ml/backtest/match")
def historical_backtest_match(req: HistoricalMatchBacktestRequest):
    """Historical backtest for a specific already-played match."""
    logger.info("historical_backtest.match.start", match_id=req.match_id, has_filters=req.filters is not None)
    repo = DeterministicInMemoryRepo()
    try:
        resp = svc_historical_backtest(req, repo, svc_resolve_model_version(getattr(app, "version", "")))
    except ValueError as e:
        logger.error("historical_backtest.match.validation_failed", match_id=req.match_id, error=str(e))
        raise HTTPException(status_code=422, detail=str(e)) from e
    logger.info("historical_backtest.match.success", match_id=req.match_id)
    return JSONResponse(status_code=200, content=resp.model_dump())


@app.post("/predict/batting", response_model=List[BattingPrediction])
async def predict_batting(features: List[BattingFeatures]):
    validate_predict_batch(features, "batting", MAX_PREDICT_BATCH_SIZE)
    return run_batting_prediction(features)


@app.post("/predict/bowling", response_model=List[BowlingPrediction])
async def predict_bowling(features: List[BowlingFeatures]):
    validate_predict_batch(features, "bowling", MAX_PREDICT_BATCH_SIZE)
    return run_bowling_prediction(features)


@app.post("/predict/extras", response_model=List[ExtrasPrediction])
async def predict_extras(features: List[ExtrasFeatures]):
    validate_predict_batch(features, "extras", MAX_PREDICT_BATCH_SIZE)
    return run_extras_prediction(features)


@app.post("/predict/win", response_model=List[WinPrediction])
async def predict_win(features: List[WinFeatures]):
    validate_predict_batch(features, "win", MAX_PREDICT_BATCH_SIZE)
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


@app.post("/optimize/team-selection", response_model=TeamOptimizationResponse)
async def optimize_team_selection(request: TeamOptimizationRequest):
    """Server-side team selection optimisation via hill-climb with batch inference.

    Replaces hundreds of ``POST /predict/win-enhanced`` calls with a single
    request.  The ML service runs the full greedy-seed + hill-climb loop
    internally using vectorised ``model.predict_proba`` batches.
    """
    from ml.team_optimizer import PoolPlayer, ScoreWeights, SelectionConstraints

    def _validated_player_id(k: str) -> int:
        if not k.isdigit() or len(k) > 20:
            raise HTTPException(
                status_code=400,
                detail=_error_payload(code="INVALID_PLAYER_ID", message=f"Invalid player ID key: {k!r}"),
            )
        return int(k)

    def _reject_non_finite(features: Dict[str, float], context: str) -> None:
        for name, val in features.items():
            if not math.isfinite(val):
                raise HTTPException(
                    status_code=400,
                    detail=_error_payload(
                        code="NON_FINITE_FEATURE",
                        message=f"Non-finite value in {context}: {name}={val!r}",
                    ),
                )

    for p in request.pool:
        _reject_non_finite(p.features, f"pool player {p.player_id}")

    pool = [
        PoolPlayer(
            player_id=p.player_id,
            name=p.name,
            is_bowler=p.is_bowler,
            is_keeper=p.is_keeper,
            bat_score=p.bat_score,
            bowl_score=p.bowl_score,
            field_score=p.field_score,
            features=p.features,
        )
        for p in request.pool
    ]
    opponent_features = {_validated_player_id(k): v for k, v in request.opponent_features.items()}

    for pid, feats in opponent_features.items():
        _reject_non_finite(feats, f"opponent player {pid}")

    try:
        result = run_team_optimization(
            fmt=request.format or "",
            pool=pool,
            opponent_features=opponent_features,
            match_context=request.match_context,
            constraints=SelectionConstraints(
                size=request.constraints.size,
                min_bowlers=request.constraints.min_bowlers,
                require_keeper=request.constraints.require_keeper,
            ),
            weights=ScoreWeights(
                bat=request.weights.bat,
                bowl=request.weights.bowl,
                field=request.weights.field,
                keeper_bonus=request.weights.keeper_bonus,
            ),
            team_is_team1=request.team_is_team1,
            max_iterations=request.max_iterations,
            max_evals=request.max_evals,
        )
    except ValueError as exc:
        raise HTTPException(
            status_code=422,
            detail=_error_payload(
                code="OPTIMIZATION_CONSTRAINT_ERROR",
                message=str(exc),
                hint="Check pool composition satisfies constraints (keeper, bowlers, size).",
            ),
        ) from exc

    return TeamOptimizationResponse(
        selected=[TeamOptimizationSelectedPlayer(player_id=p.player_id, name=p.name) for p in result.selected],
        win_probability=result.win_probability,
        iterations_used=result.iterations_used,
        evals_performed=result.evals_performed,
    )


# ---------------------------------------------------------------------------
# Admin endpoints
# ---------------------------------------------------------------------------


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
    global _model_stats_cache
    _model_stats_cache = None
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
                return await f(request, *args, **kwargs)
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

        return wrapped

    return decorator


@app.post("/admin/train/batting")
@_require_admin_train("batting", "Batting training failed")
async def admin_train_batting(request: Request, cutoff: str = ""):
    """Run batting model training per format."""
    async with _get_training_semaphore():
        await asyncio.to_thread(
            training_orchestrator.run_batting_training,
            (cutoff or "").strip(),
            _settings.go_app_url,
            logger,
        )
    return training_orchestrator.train_response("batting")


@app.post("/admin/train/bowling")
@_require_admin_train("bowling", "Bowling training failed")
async def admin_train_bowling(request: Request, cutoff: str = ""):
    """Run bowling model training per format."""
    async with _get_training_semaphore():
        await asyncio.to_thread(
            training_orchestrator.run_bowling_training,
            (cutoff or "").strip(),
            _settings.go_app_url,
            logger,
        )
    return training_orchestrator.train_response("bowling")


@app.post("/admin/train/fielding")
@_require_admin_train("fielding", "Fielding training failed")
async def admin_train_fielding(request: Request, cutoff: str = ""):
    """Run fielding model training."""
    async with _get_training_semaphore():
        await asyncio.to_thread(
            training_orchestrator.run_fielding_training,
            (cutoff or "").strip(),
            _settings.go_app_url,
            logger,
        )
    return training_orchestrator.train_response("fielding")


@app.post("/admin/train/extras")
@_require_admin_train("extras", "Extras training failed")
async def admin_train_extras(request: Request, cutoff: str = ""):
    """Run extras model training. Requires cutoff."""
    cutoff = (cutoff or "").strip()
    if not cutoff:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="CUTOFF_REQUIRED",
                message="Extras training requires cutoff",
                hint="Pass query param cutoff (RFC3339), e.g. ?cutoff=2025-01-01T00:00:00Z",
            ),
        )
    async with _get_training_semaphore():
        await asyncio.to_thread(training_orchestrator.run_extras_training, cutoff, _settings.go_app_url, logger)
    return training_orchestrator.train_response("extras")


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


@app.post("/admin/train/innings")
@_require_admin_train("innings", "Innings training failed")
async def admin_train_innings(request: Request, cutoff: str = ""):
    """Run innings model training. Requires cutoff."""
    cutoff = (cutoff or "").strip()
    if not cutoff:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="CUTOFF_REQUIRED",
                message="Innings training requires cutoff",
                hint="Pass query param cutoff (RFC3339), e.g. ?cutoff=2025-01-01T00:00:00Z",
            ),
        )
    async with _get_training_semaphore():
        await asyncio.to_thread(training_orchestrator.run_innings_training, cutoff, _settings.go_app_url, logger)
    return training_orchestrator.train_response("innings")


@app.post("/admin/train/combination-meta")
@_require_admin_train("combination-meta", "Combination-meta training failed")
async def admin_train_combination_meta(request: Request):
    """Train the score-combination meta-model from the backtest contributions CSV.

    The CSV is produced by go-app's POST /api/backtest/export-contributions. A missing
    input is a precondition, not a server error, so it returns 400 with the command to
    run rather than a 500 that says "check the logs".
    """
    csv_path = training_orchestrator.combination_meta_csv_path()
    if not os.path.isfile(csv_path):
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="CONTRIBUTIONS_CSV_MISSING",
                message=f"combination-meta training needs {csv_path}, which does not exist",
                hint="Run POST /api/backtest/export-contributions on go-app first, then retry.",
            ),
        )
    async with _get_training_semaphore():
        await asyncio.to_thread(training_orchestrator.run_combination_meta_training, logger)
    # combination_meta is not instrumented, so this carries no summary today. Routing
    # it through the same builder means it gains one the moment it is.
    return training_orchestrator.train_response("combination_meta")


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
