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
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from . import settings as app_settings
from .artifacts import BAT_MODELS, BOWL_MODELS, EXTRAS_MODELS, FIELD_MODELS, WIN_MODELS
from .artifacts import reload as reload_artifacts
from .artifacts import summary as artifacts_summary
from .backtest_service import (
    DeterministicInMemoryRepo,
    build_batting_features_from_map,
    build_bowling_features_from_map,
    build_fielding_features_from_map,
)
from .backtest_service import historical_backtest as svc_historical_backtest
from .backtest_service import predict_match_baseline as svc_predict_match_baseline
from .backtest_service import resolve_model_version as svc_resolve_model_version
from .errors import error_payload
from .feature_config import get_feature_names
from .features import batting_feature_vector, bowling_feature_vector, fielding_feature_vector
from .logging import bind_request_context, get_struct_logger, init_logging
from .model_metadata import get_model_metadata
from .models import (
    BacktestMatchResponse,
    BacktestPlayerPred,
    BacktestPlayersResponse,
    BacktestPredictRequest,
    BattingFeatures,
    BattingPrediction,
    BowlingFeatures,
    BowlingPrediction,
    ExtrasFeatures,
    ExtrasPrediction,
    HistoricalMatchBacktestRequest,
    WinFeatures,
    WinPrediction,
)
from .train_on_the_fly import train_on_the_fly_cached

try:
    from ml.config import get_prediction_defaults
except ImportError:
    get_prediction_defaults = None
try:
    from ml.train_extras import EXTRAS_FEATURE_COLS
except ImportError:
    EXTRAS_FEATURE_COLS = []
try:
    from ml.train_win import WIN_FEATURE_COLS
except ImportError:
    WIN_FEATURE_COLS = []


@asynccontextmanager
async def _lifespan(app: FastAPI) -> Any:
    # On startup: cancel only stale IN_PROGRESS (older than threshold) so we don't cancel another instance's run.
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


def _install_crash_logging() -> None:
    """Ensure uncaught exceptions and thread crashes are logged before exit."""
    _orig_excepthook = sys.excepthook

    def _excepthook(exc_type: type, exc_value: BaseException, exc_tb: Any) -> None:
        logger.error(
            "uncaught_exception",
            exc_info=(exc_type, exc_value, exc_tb),
            error_type=exc_type.__name__ if exc_type else "",
            error=str(exc_value),
            traceback="".join(traceback.format_exception(exc_type, exc_value, exc_tb)),
        )
        _orig_excepthook(exc_type, exc_value, exc_tb)

    sys.excepthook = _excepthook

    if hasattr(threading, "excepthook"):  # Python 3.8+
        _orig_thread_excepthook = threading.excepthook

        def _thread_excepthook(args: Any) -> None:
            exc_type = getattr(args, "exc_type", None)
            exc_value = getattr(args, "exc_value", None)
            exc_tb = getattr(args, "exc_traceback", None)
            thread = getattr(args, "thread", None)
            logger.error(
                "uncaught_thread_exception",
                exc_info=(exc_type, exc_value, exc_tb),
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

ENABLE_HOT_RELOAD = os.environ.get("ENABLE_HOT_RELOAD", "").strip().lower() in {"1", "true", "yes"}
ADMIN_API_KEY = (os.environ.get("ADMIN_API_KEY") or "").strip()

# Max concurrent training jobs (admin train); prevents DoS via many concurrent requests
MAX_CONCURRENT_TRAINING_JOBS = max(1, int(os.environ.get("MAX_CONCURRENT_TRAINING_JOBS", "1")))
_training_semaphore: Optional[asyncio.Semaphore] = None


def _get_training_semaphore() -> asyncio.Semaphore:
    global _training_semaphore
    if _training_semaphore is None:
        _training_semaphore = asyncio.Semaphore(MAX_CONCURRENT_TRAINING_JOBS)
    return _training_semaphore


def _get_go_app_url() -> str:
    """Return GO_APP_URL env with fallback to http://localhost:8080 (empty/missing -> default)."""
    return (os.environ.get("GO_APP_URL") or "").strip() or "http://localhost:8080"


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


MAX_PREDICT_BATCH_SIZE = int(os.environ.get("MAX_PREDICT_BATCH_SIZE", "10000"))

# -------------------- Simple in-memory cache for backtest endpoint --------------------
DISABLE_BACKTEST_CACHE = os.environ.get("DISABLE_BACKTEST_CACHE", "").strip().lower() in {"1", "true", "yes"}
CACHE_TTL_SECONDS = int(os.environ.get("BACKTEST_CACHE_TTL", "300") or "300")

# Cache key: (mode, cutoff_iso, tuple(sorted(ids)))
_backtest_cache: Dict[Tuple[str, str, Tuple[Any, ...]], Tuple[float, Dict[str, Any]]] = {}
BACKTEST_PLAYERS_COMPUTE_COUNT = 0
BACKTEST_MATCH_COMPUTE_COUNT = 0

# -------------------- CORS for local frontend dev --------------------
# Allow the Vite dev server by default; can be overridden via FRONTEND_ORIGIN
_frontend_origin_env = os.environ.get("FRONTEND_ORIGIN", "http://localhost:5173")
_allowed_origins = [o.strip() for o in _frontend_origin_env.split(",") if o.strip()]
app.add_middleware(
    CORSMiddleware,
    allow_origins=_allowed_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


def _cache_get(mode: str, cutoff_iso: str, ids: List[Any]) -> Optional[Dict[str, Any]]:
    if DISABLE_BACKTEST_CACHE or CACHE_TTL_SECONDS <= 0:
        return None
    key = (mode, cutoff_iso, tuple(sorted(ids)))
    rec = _backtest_cache.get(key)
    if not rec:
        return None
    ts, payload = rec
    if (time.time() - ts) > CACHE_TTL_SECONDS:
        _backtest_cache.pop(key, None)
        return None
    return payload


def _cache_put(mode: str, cutoff_iso: str, ids: List[Any], payload: Dict[str, Any]) -> None:
    if DISABLE_BACKTEST_CACHE or CACHE_TTL_SECONDS <= 0:
        return
    key = (mode, cutoff_iso, tuple(sorted(ids)))
    _backtest_cache[key] = (time.time(), payload)


def reset_backtest_cache() -> None:
    """Utility for tests to clear cache and counters."""
    global _backtest_cache, BACKTEST_PLAYERS_COMPUTE_COUNT, BACKTEST_MATCH_COMPUTE_COUNT
    _backtest_cache = {}
    BACKTEST_PLAYERS_COMPUTE_COUNT = 0
    BACKTEST_MATCH_COMPUTE_COUNT = 0


def get_backtest_compute_counts() -> Tuple[int, int]:
    return BACKTEST_PLAYERS_COMPUTE_COUNT, BACKTEST_MATCH_COMPUTE_COUNT


@app.middleware("http")
async def request_context_middleware(request: Request, call_next):
    # Request ID from header or generate new
    rid = request.headers.get("X-Request-ID") or str(uuid.uuid4())
    bind_request_context(rid)
    start = time.time()
    # Log request start
    logger.info(
        "request.start",
        method=request.method,
        path=request.url.path,
    )
    try:
        response: Response = await call_next(request)
    except Exception as exc:
        # Log exception and return JSON error with request_id
        logger.exception(
            "request.error",
            method=request.method,
            path=request.url.path,
        )
        data = error_payload(
            code="UNHANDLED_EXCEPTION",
            message=str(exc) or exc.__class__.__name__,
            hint="Check server logs with the provided request_id for details.",
        )
        response = JSONResponse(status_code=500, content={"detail": data})
    finally:
        duration_ms = int((time.time() - start) * 1000)
        # Always log end with status (if available)
        status = getattr(response, "status_code", 0)
        logger.info(
            "request.end",
            method=request.method,
            path=request.url.path,
            status_code=status,
            duration_ms=duration_ms,
        )
    # Echo request id header
    response.headers["X-Request-ID"] = rid
    return response


# Models are imported from app.models (see imports above)


def _predict_players_with_features(
    cutoff: datetime,
    player_ids: List[int],
    fmt: str,
    features_map: Dict[str, Dict[str, float]],
) -> List[BacktestPlayerPred]:
    """Run full pipeline: build feature objects from map, run batting/bowling models, return predictions.
    When no pre-trained artifacts are loaded for the format, trains on the fly from go-app training data.
    """
    fmt_upper = (fmt or "").strip().upper()
    bat_pair = BAT_MODELS.get(fmt_upper) if fmt_upper else None
    bowl_pair = BOWL_MODELS.get(fmt_upper) if fmt_upper else None
    if not bat_pair or not bowl_pair:
        go_app_url = (os.environ.get("GO_APP_URL") or "").strip()
        if not go_app_url:
            logger.error(
                "backtest_predict.train_on_the_fly.missing_go_app_url",
                format=fmt_upper,
                hint="Set GO_APP_URL to the go-app base URL for train-on-the-fly.",
            )
            raise ValueError(
                "GO_APP_URL is required for train-on-the-fly when no artifacts are loaded for format=%s" % fmt_upper
            )
        _cutoff_tz = cutoff if cutoff.tzinfo else cutoff.replace(tzinfo=timezone.utc)
        cutoff_iso = _cutoff_tz.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
        api_key = (os.environ.get("GO_APP_API_KEY") or "").strip() or None
        logger.info(
            "backtest_predict.train_on_the_fly.triggered",
            format=fmt_upper,
            cutoff_iso=cutoff_iso,
            go_app_url=go_app_url,
            player_count=len(player_ids),
        )
        bat_pair, bowl_pair = train_on_the_fly_cached(go_app_url, fmt_upper, cutoff_iso, api_key)

    scaler_bat, model_bat = bat_pair
    scaler_bowl, model_bowl = bowl_pair

    bat_features: List[BattingFeatures] = []
    bowl_features: List[BowlingFeatures] = []
    for pid in player_ids:
        fm = features_map.get(str(pid)) or features_map.get(str(int(pid))) or {}
        bat_features.append(build_batting_features_from_map(pid, cutoff, fmt_upper, fm))
        bowl_features.append(build_bowling_features_from_map(pid, cutoff, fmt_upper, fm))

    # Feature order must match training (configs/feature_vectors.json). Apply feature_transforms if in metadata.
    try:
        from ml.feature_transforms import build_extended_vector_from_features, load_transform_config_from_metadata

        bat_transform = load_transform_config_from_metadata(MODELS_DIR, "batting", fmt_upper)
        base_names = get_feature_names("batting")
        if bat_transform.get("add_interactions") or bat_transform.get("add_log1p"):
            bat_vecs = []
            for i, f in enumerate(bat_features):
                fm = features_map.get(str(player_ids[i])) or features_map.get(str(int(player_ids[i]))) or {}
                fm_for_interactions = {n: getattr(f, n) for n in base_names}
                fm_for_interactions.update(fm)
                base_vals = batting_feature_vector(f)
                ext = build_extended_vector_from_features(base_vals, base_names, fm_for_interactions, bat_transform)
                bat_vecs.append(ext)
            X_bat = np.array(bat_vecs, dtype=float)
        else:
            X_bat = np.array([batting_feature_vector(f) for f in bat_features], dtype=float)
    except Exception as e:
        logger.exception("predict.feature_transform.failed", error=str(e))
        raise HTTPException(
            status_code=500,
            detail="Feature transformation failed; prediction pipeline cannot proceed with incorrect feature data.",
        ) from e
    if scaler_bat is not None:
        X_bat = scaler_bat.transform(X_bat)
    Y_bat = model_bat.predict(X_bat)

    try:
        bowl_transform = load_transform_config_from_metadata(MODELS_DIR, "bowling", fmt_upper)
        base_names_bowl = get_feature_names("bowling")
        if bowl_transform.get("add_interactions") or bowl_transform.get("add_log1p"):
            bowl_vecs = []
            for i, f in enumerate(bowl_features):
                fm = features_map.get(str(player_ids[i])) or features_map.get(str(int(player_ids[i]))) or {}
                base_vals = bowling_feature_vector(f)
                fm_for_interactions = dict(zip(base_names_bowl, base_vals))
                fm_for_interactions.update(fm)
                ext = build_extended_vector_from_features(
                    base_vals, base_names_bowl, fm_for_interactions, bowl_transform
                )
                bowl_vecs.append(ext)
            X_bowl = np.array(bowl_vecs, dtype=float)
        else:
            X_bowl = np.array([bowling_feature_vector(f) for f in bowl_features], dtype=float)
    except Exception as e:
        logger.exception("predict.bowling_feature_transform.failed", error=str(e))
        raise HTTPException(
            status_code=500,
            detail="Bowling feature transformation failed; prediction pipeline cannot proceed with incorrect feature data.",
        ) from e
    if scaler_bowl is not None:
        X_bowl = scaler_bowl.transform(X_bowl)
    Y_bowl = model_bowl.predict(X_bowl)

    out: List[BacktestPlayerPred] = []
    for i, pid in enumerate(player_ids):
        row_bat = np.atleast_1d(Y_bat[i]).ravel()
        row_bowl = np.atleast_1d(Y_bowl[i]).ravel()
        vals_bat = list(row_bat) + [0.0] * max(0, 6 - len(row_bat))
        vals_bowl = list(row_bowl) + [0.0] * max(0, 4 - len(row_bowl))
        runs = float(max(0.0, vals_bat[0]))
        wickets = float(max(0.0, vals_bowl[2])) if len(vals_bowl) > 2 else 0.0
        default_econ = get_prediction_defaults()["economy"] if get_prediction_defaults else 6.0
        economy = float(max(0.0, vals_bowl[3])) if len(vals_bowl) > 3 else default_econ
        catches, run_outs = 0.0, 0.0
        out.append(
            BacktestPlayerPred(
                player_id=int(pid),
                runs=runs,
                wickets=wickets,
                economy=economy,
                catches=catches,
                run_outs=run_outs,
            )
        )

    # Fielding: if we have fielding artifacts, predict catches/run_outs and merge into player preds (per-format or legacy)
    field_pair = (FIELD_MODELS.get(fmt_upper) if fmt_upper else None) or FIELD_MODELS.get("_LEGACY_")
    if field_pair is not None:
        scaler_fld, model_fld = field_pair
        field_features_list = [
            build_fielding_features_from_map(
                int(pid), cutoff, fmt_upper, features_map.get(str(pid)) or features_map.get(str(int(pid))) or {}
            )
            for pid in player_ids
        ]
        X_fld = np.array([fielding_feature_vector(f) for f in field_features_list], dtype=float)
        if scaler_fld is not None:
            X_fld = scaler_fld.transform(X_fld)
        Y_fld = model_fld.predict(X_fld)
        out_new: List[BacktestPlayerPred] = []
        for i, pred in enumerate(out):
            row_fld = np.atleast_1d(Y_fld[i]).ravel()
            vals_fld = list(row_fld) + [0.0] * max(0, 3 - len(row_fld))
            catches = float(max(0.0, vals_fld[0])) if len(vals_fld) > 0 else 0.0
            run_outs = float(max(0.0, vals_fld[1])) if len(vals_fld) > 1 else 0.0
            out_new.append(
                BacktestPlayerPred(
                    player_id=pred.player_id,
                    runs=pred.runs,
                    wickets=pred.wickets,
                    economy=pred.economy,
                    catches=catches,
                    run_outs=run_outs,
                )
            )
        out = out_new

    return out


@app.post("/ml/backtest/predict")
def backtest_predict(req: BacktestPredictRequest):
    # Strict cutoff semantics are honored implicitly by not using post-cutoff data.
    # We still parse/validate the timestamp.
    cutoff = req.cutoff_date
    # Convert to UTC first, then format as ISO 8601 with trailing 'Z'
    # If the datetime is naive (no tzinfo), assume it is already UTC to avoid localtime assumptions.
    cutoff_with_tz = cutoff if cutoff.tzinfo is not None else cutoff.replace(tzinfo=timezone.utc)
    cutoff_utc = cutoff_with_tz.astimezone(timezone.utc)
    cutoff_iso = cutoff_utc.isoformat().replace("+00:00", "Z")
    if req.player_ids is not None:
        # Player predictions require format and features (no deterministic baseline)
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
        cached = _cache_get("players", cutoff_iso, list(req.player_ids))
        if cached is not None:
            logger.info("backtest_predict.player.cache_hit", cutoff_iso=cutoff_iso, player_count=len(req.player_ids))
            return JSONResponse(status_code=200, content=cached)
        global BACKTEST_PLAYERS_COMPUTE_COUNT
        BACKTEST_PLAYERS_COMPUTE_COUNT += 1
        try:
            preds = _predict_players_with_features(cutoff, req.player_ids, req.format or "", req.features)
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
        _cache_put("players", cutoff_iso, list(req.player_ids), body)
        return JSONResponse(status_code=200, content=body)
    if req.teams is not None:
        cached = _cache_get("match", cutoff_iso, list(req.teams))
        if cached is not None:
            return JSONResponse(status_code=200, content=cached)
        global BACKTEST_MATCH_COMPUTE_COUNT
        BACKTEST_MATCH_COMPUTE_COUNT += 1
        match = svc_predict_match_baseline(cutoff, req.teams)
        body = BacktestMatchResponse(
            match=match, model_version=svc_resolve_model_version(getattr(app, "version", ""))
        ).model_dump()
        _cache_put("match", cutoff_iso, list(req.teams), body)
        return JSONResponse(status_code=200, content=body)
    logger.warning(
        "backtest_predict.invalid_request",
        reason="missing_player_ids_and_teams",
        cutoff_iso=cutoff_iso,
    )
    raise HTTPException(
        status_code=400,
        detail=error_payload(
            code="INVALID_REQUEST",
            message="provide either player_ids or teams",
            hint="Body must include one of: {player_ids:[..]} or {teams:[team1,team2]}",
        ),
    )


@app.post("/ml/backtest/match")
def historical_backtest_match(req: HistoricalMatchBacktestRequest):
    """Historical backtest for a specific already-played match.

    Trains/predicts strictly using data before cutoff_date and compares against actuals.
    Currently uses a deterministic in-memory repo and simple baselines; swap in a DB-backed
    repo and true models as they become available.
    """
    logger.info(
        "historical_backtest.match.start",
        match_id=req.match_id,
        has_filters=req.filters is not None,
    )
    repo = DeterministicInMemoryRepo()
    try:
        resp = svc_historical_backtest(req, repo, svc_resolve_model_version(getattr(app, "version", "")))
    except ValueError as e:
        logger.error(
            "historical_backtest.match.validation_failed",
            match_id=req.match_id,
            error=str(e),
        )
        raise HTTPException(status_code=422, detail=str(e)) from e
    logger.info("historical_backtest.match.success", match_id=req.match_id)
    return JSONResponse(status_code=200, content=resp.model_dump())


# Team win models are imported from app.models


# Load artifacts (per-format if available)
# Prefer ML_SERVICE_OUTPUT_DIR, then MODELS_DIR, then config.json default, else ../../output/ml-service
try:
    import config as svc_config  # from ml-service/ml/config.py or project root
except Exception:
    svc_config = None  # type: ignore

MODELS_DIR = app_settings.get_models_dir(svc_config)
os.makedirs(MODELS_DIR, exist_ok=True)

# Registries provided by app.artifacts module (imported above)


# Delegate to centralized error helper
_error_payload = error_payload


def _reload_artifacts() -> dict:
    """Rescan MODELS_DIR and reload registries using artifacts module."""
    reload_artifacts(MODELS_DIR)
    return artifacts_summary()


# Initial load of artifacts (legacy + per-format) via artifacts module
try:
    logger.info("startup.artifacts.load.start", models_dir=MODELS_DIR)
    reload_artifacts(MODELS_DIR)
    logger.info("startup.artifacts.load.done", models_dir=MODELS_DIR)
except Exception as e:
    logger.error(
        "startup.artifacts.load.failed",
        models_dir=MODELS_DIR,
        error=str(e),
        exc_info=True,
    )


@app.get("/health")
async def health():
    logger.info("health.check.start")

    def _artifacts_info(prefix: str) -> List[dict]:
        out = []
        try:
            for fname in os.listdir(MODELS_DIR):
                if fname.lower().startswith(prefix) and fname.lower().endswith(".joblib"):
                    fpath = os.path.join(MODELS_DIR, fname)
                    try:
                        st = os.stat(fpath)
                        out.append(
                            {
                                "file": fname,
                                "size_bytes": st.st_size,
                                "modified": int(st.st_mtime),
                            }
                        )
                    except OSError as e:
                        logger.warning("health.artifacts_info.stat_failed", file=fname, error=str(e))
                        out.append({"file": fname})
        except OSError as e:
            logger.warning("health.artifacts_info.listdir_failed", models_dir=MODELS_DIR, error=str(e))
        return sorted(out, key=lambda x: x.get("file", ""))

    def _metadata_info(prefix: str) -> List[str]:
        names: List[str] = []
        try:
            for fname in os.listdir(MODELS_DIR):
                lf = fname.lower()
                if lf.startswith(prefix) and lf.endswith(".json"):
                    names.append(fname)
        except OSError as e:
            logger.warning("health.metadata_info.listdir_failed", models_dir=MODELS_DIR, error=str(e))
        return sorted(names)

    return {
        "status": "ok",
        "loaded_batting_formats": sorted([k for k in BAT_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_bowling_formats": sorted([k for k in BOWL_MODELS.keys() if k != "_LEGACY_"]),
        "legacy_batting_available": "_LEGACY_" in BAT_MODELS,
        "legacy_bowling_available": "_LEGACY_" in BOWL_MODELS,
        "models_dir": MODELS_DIR,
        "artifacts": {
            "batting": _artifacts_info("batting_"),
            "bowling": _artifacts_info("bowling_"),
        },
        "metadata": {
            "batting": _metadata_info("batting_metadata_"),
            "bowling": _metadata_info("bowling_metadata_"),
        },
        "counters": {
            "batting_formats": len([k for k in BAT_MODELS.keys() if k != "_LEGACY_"]),
            "bowling_formats": len([k for k in BOWL_MODELS.keys() if k != "_LEGACY_"]),
        },
    }


# -------------------- Ops: Artifacts Status Endpoint --------------------
_FORMATS = ["TEST", "ODI", "T20I", "T20"]


def _find_artifact(models_dir: str, fmt: str, batting: bool) -> Optional[Tuple[str, float]]:
    """Return (path, mtime) for the first matching artifact if found.
    Preferred names: batting_<FORMAT>.joblib / bowling_<FORMAT>.joblib.
    Fallback: files containing tokens 'bat' or 'bowl' and the format code.
    """
    hit = _find_per_format_artifact(models_dir, fmt, "batting" if batting else "bowling")
    return hit


def _find_per_format_artifact(
    models_dir: str,
    fmt: str,
    kind: str,
) -> Optional[Tuple[str, float]]:
    """Return (path, mtime) for per-format artifact of given kind.
    kind in: batting, bowling, fielding, extras, win.
    Batting/bowling/fielding require both scaler and model (e.g. batting_scaler_T20.joblib + batting_model_T20.joblib).
    """
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.debug(
            "artifacts_status.find_per_format.listdir_failed",
            models_dir=models_dir,
            fmt=fmt,
            kind=kind,
            error=str(e),
        )
        return None
    if kind == "batting":
        scaler_name = f"batting_scaler_{fmt}.joblib"
        model_name = f"batting_model_{fmt}.joblib"
    elif kind == "bowling":
        scaler_name = f"bowling_scaler_{fmt}.joblib"
        model_name = f"bowling_model_{fmt}.joblib"
    elif kind == "fielding":
        scaler_name = f"fielding_scaler_{fmt}.joblib"
        model_name = f"fielding_model_{fmt}.joblib"
    elif kind == "extras":
        model_name = f"extras_model_{fmt}.joblib"
        scaler_name = None
    elif kind == "win":
        model_name = f"win_model_{fmt}.joblib"
        scaler_name = None
    else:
        return None
    if scaler_name and scaler_name not in entries:
        return None
    if model_name not in entries:
        return None
    path = os.path.join(models_dir, model_name)
    try:
        st = os.stat(path)
        if not os.path.isfile(path):
            return None
        return path, st.st_mtime
    except Exception:
        return None


def _find_legacy_artifact(models_dir: str, kind: str) -> Optional[Tuple[str, float]]:
    """Return (path, mtime) for legacy (unified) artifact if present.
    kind in: batting, bowling, fielding, extras, win.
    Batting/bowling/fielding require both scaler and model; path/mtime from model file.
    """
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.debug(
            "artifacts_status.find_legacy.listdir_failed",
            models_dir=models_dir,
            kind=kind,
            error=str(e),
        )
        return None
    if kind == "batting":
        if "batting_scaler.joblib" not in entries or "batting_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "batting_model.joblib")
    elif kind == "bowling":
        if "bowling_scaler.joblib" not in entries or "bowling_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "bowling_model.joblib")
    elif kind == "fielding":
        if "fielding_scaler.joblib" not in entries or "fielding_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "fielding_model.joblib")
    elif kind == "extras":
        if "extras_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "extras_model.joblib")
    elif kind == "win":
        if "win_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "win_model.joblib")
    else:
        return None
    try:
        st = os.stat(path)
        if not os.path.isfile(path):
            return None
        return path, st.st_mtime
    except Exception:
        return None


def _legacy_status_obj(
    models_dir: str,
    kind: str,
    loaded: bool,
) -> Dict[str, Any]:
    """Build { exists, path?, modified?, loaded } for one legacy model kind."""
    out: Dict[str, Any] = {"exists": False}
    hit = _find_legacy_artifact(models_dir, kind)
    if hit is not None:
        p, mt = hit
        out["exists"] = True
        out["path"] = p
        out["modified"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mt))
    out["loaded"] = loaded
    return out


@app.get("/artifacts/status")
async def artifacts_status():
    """Report presence and (optionally) loaded state of artifacts per format and legacy (unified).

    Shape:
    {
      "timestamp": ISO8601,
      "root": MODELS_DIR,
      "formats": {
        "ODI": {
          "batting": {"exists": bool, "path": str?, "modified": str?, "loaded": bool?},
          "bowling": {...},
          "fielding": {...},
          "extras": {...},
          "win": {...}
        },
        ...
      },
      "legacy": {
        "batting": {"exists": bool, "path": str?, "modified": str?, "loaded": bool},
        "bowling": {...},
        "fielding": {...},
        "extras": {...},
        "win": {...}
      }
    }
    """
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    root = MODELS_DIR
    artifact_kinds = ["batting", "bowling", "fielding", "extras", "win"]
    loaded_registries: Dict[str, Any] = {
        "batting": BAT_MODELS,
        "bowling": BOWL_MODELS,
        "fielding": FIELD_MODELS,
        "extras": EXTRAS_MODELS,
        "win": WIN_MODELS,
    }
    formats_out: Dict[str, Dict[str, Any]] = {}
    for fmt in _FORMATS:
        row: Dict[str, Dict[str, Any]] = {}
        for kind in artifact_kinds:
            obj: Dict[str, Any] = {"exists": False}
            hit = _find_per_format_artifact(root, fmt, kind)
            if hit is not None:
                p, mt = hit
                obj["exists"] = True
                obj["path"] = p
                obj["modified"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mt))
            try:
                reg = loaded_registries.get(kind)
                if reg is not None and fmt in reg:
                    obj["loaded"] = True
            except Exception as e:
                logger.debug("artifacts_status.loaded_check", fmt=fmt, kind=kind, error=str(e))
            row[kind] = obj
        formats_out[fmt] = row
    formats = formats_out

    legacy: Dict[str, Dict[str, Any]] = {
        "batting": _legacy_status_obj(root, "batting", "_LEGACY_" in BAT_MODELS),
        "bowling": _legacy_status_obj(root, "bowling", "_LEGACY_" in BOWL_MODELS),
        "fielding": _legacy_status_obj(root, "fielding", "_LEGACY_" in FIELD_MODELS),
        "extras": _legacy_status_obj(root, "extras", "_LEGACY_" in EXTRAS_MODELS),
        "win": _legacy_status_obj(root, "win", "_LEGACY_" in WIN_MODELS),
    }

    return {"timestamp": ts, "root": root, "formats": formats, "legacy": legacy}


@app.get("/model-metadata")
async def model_metadata():
    """Return model metadata (features, outputs, level, artifacts pattern) from the source of truth.

    Used by the Workbench UI so it stays in sync with feature_vectors.json and training scripts.
    """
    try:
        return get_model_metadata()
    except Exception as e:
        logger.exception("model_metadata.error", error=str(e))
        raise HTTPException(status_code=500, detail={"code": "METADATA_ERROR", "message": str(e)}) from e


@app.post("/predict/batting", response_model=List[BattingPrediction])
async def predict_batting(features: List[BattingFeatures]):
    if not features:
        logger.info("predict.batting.rejected", reason="empty_batch")
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="EMPTY_BATCH",
                message="Empty features list",
                hint="Send at least one feature row with the required fields.",
            ),
        )
    # Determine format
    fmt = (features[0].format or "").strip().upper()
    if fmt:
        # Validate all rows have same format
        for f in features:
            if (f.format or "").strip().upper() != fmt:
                logger.info("predict.batting.rejected", reason="mixed_formats", batch_size=len(features))
                raise HTTPException(
                    status_code=400,
                    detail=_error_payload(
                        code="MIXED_FORMATS",
                        message="All feature rows must have the same format",
                        hint="Ensure every row uses the same 'format' code.",
                    ),
                )
        pair = BAT_MODELS.get(fmt)
        if not pair:
            logger.warning("predict.batting.model_not_loaded", format=fmt, available=list(BAT_MODELS.keys()))
            raise HTTPException(
                status_code=404,
                detail=_error_payload(
                    code="MODEL_NOT_LOADED",
                    message=f"Model for format {fmt} not loaded",
                    hint="Train artifacts for this format and place them under the models directory.",
                    available=list(BAT_MODELS.keys()),
                ),
            )
        scaler, model = pair
    else:
        # Legacy fallback
        pair = BAT_MODELS.get("_LEGACY_")
        if not pair:
            logger.info("predict.batting.rejected", reason="missing_format_no_legacy")
            raise HTTPException(
                status_code=400,
                detail=_error_payload(
                    code="MISSING_FORMAT",
                    message="Missing 'format' and no legacy batting model loaded",
                    hint="Set 'format' in the request or train legacy artifacts.",
                ),
            )
        scaler, model = pair

    logger.info(
        "predict.batting.start", batch=len(features), format=fmt or ("LEGACY" if "_LEGACY_" in BAT_MODELS else "")
    )
    X = np.array([batting_feature_vector(f) for f in features], dtype=float)
    if scaler is not None:
        X = scaler.transform(X)
    try:
        Y = model.predict(X)
        preds = []
        for row in Y:
            vals = row if np.ndim(row) == 1 else row.ravel()
            vals = list(vals) + [0.0] * max(0, 6 - len(vals))
            preds.append(
                BattingPrediction(
                    runs_scored=float(vals[0]),
                    balls_faced=float(vals[1]),
                    fours_scored=float(vals[2]),
                    sixes_scored=float(vals[3]),
                    batting_position=float(vals[4]),
                    strike_rate=float(vals[5]),
                )
            )
        logger.info("predict.batting.success", predictions=len(preds))
        return preds
    except Exception as exc:
        logger.exception("predict.batting.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="PREDICT_FAILED",
                message="Batting prediction failed",
                hint="See server logs for stacktrace using request_id",
            ),
        )


@app.post("/predict/bowling", response_model=List[BowlingPrediction])
async def predict_bowling(features: List[BowlingFeatures]):
    if not features:
        logger.info("predict.bowling.rejected", reason="empty_batch")
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="EMPTY_BATCH",
                message="Empty features list",
                hint="Send at least one feature row with the required fields.",
            ),
        )
    fmt = (features[0].format or "").strip().upper()
    if fmt:
        for f in features:
            if (f.format or "").strip().upper() != fmt:
                logger.info("predict.bowling.rejected", reason="mixed_formats", batch_size=len(features))
                raise HTTPException(
                    status_code=400,
                    detail=_error_payload(
                        code="MIXED_FORMATS",
                        message="All feature rows must have the same format",
                        hint="Ensure every row uses the same 'format' code.",
                    ),
                )
        pair = BOWL_MODELS.get(fmt)
        if not pair:
            logger.warning("predict.bowling.model_not_loaded", format=fmt, available=list(BOWL_MODELS.keys()))
            raise HTTPException(
                status_code=404,
                detail=_error_payload(
                    code="MODEL_NOT_LOADED",
                    message=f"Model for format {fmt} not loaded",
                    hint="Train artifacts for this format and place them under the models directory.",
                    available=list(BOWL_MODELS.keys()),
                ),
            )
        scaler, model = pair
    else:
        pair = BOWL_MODELS.get("_LEGACY_")
        if not pair:
            logger.info("predict.bowling.rejected", reason="missing_format_no_legacy")
            raise HTTPException(
                status_code=400,
                detail=_error_payload(
                    code="MISSING_FORMAT",
                    message="Missing 'format' and no legacy bowling model loaded",
                    hint="Set 'format' in the request or train legacy artifacts.",
                ),
            )
        scaler, model = pair

    logger.info(
        "predict.bowling.start", batch=len(features), format=fmt or ("LEGACY" if "_LEGACY_" in BOWL_MODELS else "")
    )
    X = np.array([bowling_feature_vector(f) for f in features], dtype=float)
    if scaler is not None:
        X = scaler.transform(X)
    try:
        Y = model.predict(X)
        preds = []
        for row in Y:
            vals = row if np.ndim(row) == 1 else row.ravel()
            vals = list(vals) + [0.0] * max(0, 4 - len(vals))
            preds.append(
                BowlingPrediction(
                    runs_conceded=float(vals[0]),
                    deliveries=float(vals[1]),
                    wickets_taken=float(vals[2]),
                    econ=float(vals[3]),
                )
            )
        logger.info("predict.bowling.success", predictions=len(preds))
        return preds
    except Exception as exc:
        logger.exception("predict.bowling.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="PREDICT_FAILED",
                message="Bowling prediction failed",
                hint="See server logs for stacktrace using request_id",
            ),
        )


def _extras_feature_vector(f: ExtrasFeatures) -> np.ndarray:
    """Build feature vector in EXTRAS_FEATURE_COLS order (exclude 'format' key)."""
    if not EXTRAS_FEATURE_COLS:
        return np.zeros(0)
    d = f.model_dump()
    return np.array([float(d.get(c, 0)) for c in EXTRAS_FEATURE_COLS], dtype=float)


def _win_feature_vector(f: WinFeatures) -> np.ndarray:
    """Build feature vector in WIN_FEATURE_COLS order (exclude 'format' key)."""
    if not WIN_FEATURE_COLS:
        return np.zeros(0)
    d = f.model_dump()
    return np.array([float(d.get(c, 0)) for c in WIN_FEATURE_COLS], dtype=float)


@app.post("/predict/extras", response_model=List[ExtrasPrediction])
async def predict_extras(features: List[ExtrasFeatures]):
    """Predict total extras per match using the loaded extras model (unified features)."""
    if not features:
        logger.info("predict.extras.rejected", reason="empty_batch")
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="EMPTY_BATCH",
                message="Empty features list",
                hint="Send at least one ExtrasFeatures row.",
            ),
        )
    if len(features) > MAX_PREDICT_BATCH_SIZE:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="BATCH_TOO_LARGE",
                message="Batch size exceeds limit",
                hint=f"Send at most {MAX_PREDICT_BATCH_SIZE} features per request.",
            ),
        )
    fmt = (features[0].format or "").strip().upper()
    model = EXTRAS_MODELS.get(fmt) if fmt else EXTRAS_MODELS.get("_LEGACY_")
    if not model:
        available = [k for k in EXTRAS_MODELS.keys() if k != "_LEGACY_"]
        logger.warning("predict.extras.model_not_loaded", format=fmt or "LEGACY", available=available)
        raise HTTPException(
            status_code=404,
            detail=_error_payload(
                code="MODEL_NOT_LOADED",
                message="Extras model not loaded",
                hint="Train extras artifacts (e.g. make train-extras) and ensure format matches or use legacy.",
                available=available,
            ),
        )
    X = np.array([_extras_feature_vector(f) for f in features], dtype=float)
    if X.size == 0:
        raise HTTPException(
            status_code=500,
            detail=_error_payload(code="FEATURE_ORDER_EMPTY", message="EXTRAS_FEATURE_COLS not available"),
        )
    try:
        y = model.predict(X)
        y_flat = np.asarray(y).ravel()
        return [ExtrasPrediction(total_extras=float(v)) for v in y_flat]
    except Exception as exc:
        logger.exception("predict.extras.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="PREDICT_FAILED", message="Extras prediction failed", hint="See server logs"),
        )


@app.post("/predict/win", response_model=List[WinPrediction])
async def predict_win(features: List[WinFeatures]):
    """Predict team1 win probability per match using the loaded win model (unified features)."""
    if not features:
        logger.info("predict.win.rejected", reason="empty_batch")
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="EMPTY_BATCH",
                message="Empty features list",
                hint="Send at least one WinFeatures row.",
            ),
        )
    if len(features) > MAX_PREDICT_BATCH_SIZE:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="BATCH_TOO_LARGE",
                message="Batch size exceeds limit",
                hint=f"Send at most {MAX_PREDICT_BATCH_SIZE} features per request.",
            ),
        )
    fmt = (features[0].format or "").strip().upper()
    model = WIN_MODELS.get(fmt) if fmt else WIN_MODELS.get("_LEGACY_")
    if not model:
        available = [k for k in WIN_MODELS.keys() if k != "_LEGACY_"]
        logger.warning("predict.win.model_not_loaded", format=fmt or "LEGACY", available=available)
        raise HTTPException(
            status_code=404,
            detail=_error_payload(
                code="MODEL_NOT_LOADED",
                message="Win model not loaded",
                hint="Train win artifacts (e.g. make train-win) and ensure format matches or use legacy.",
                available=available,
            ),
        )
    X = np.array([_win_feature_vector(f) for f in features], dtype=float)
    if X.size == 0:
        raise HTTPException(
            status_code=500,
            detail=_error_payload(code="FEATURE_ORDER_EMPTY", message="WIN_FEATURE_COLS not available"),
        )
    try:
        proba = model.predict_proba(X)
        if proba.shape[1] > 1:
            # class 1 = team1 wins
            p_team1 = proba[:, 1]
        else:
            # Single-class training data: check model.classes_ to interpret probability
            p_team1 = proba.ravel() if model.classes_[0] == 1 else 1.0 - proba.ravel()
        return [WinPrediction(team1_win_probability=float(p)) for p in p_team1]
    except Exception as exc:
        logger.exception("predict.win.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="PREDICT_FAILED", message="Win prediction failed", hint="See server logs"),
        )


@app.post("/admin/reload")
async def admin_reload(request: Request):
    """Rescan the models directory and reload artifacts.
    Guarded by ENABLE_HOT_RELOAD env flag. If ADMIN_API_KEY is set, requires X-API-Key header.
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
    logger.info("admin.reload.start", models_dir=MODELS_DIR)
    try:
        summary = _reload_artifacts()
        logger.info("admin.reload.success", models_dir=MODELS_DIR, summary=summary)
        return {"status": "reloaded", **summary}
    except Exception as e:
        logger.error("admin.reload.failed", models_dir=MODELS_DIR, error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=_error_payload(
                code="RELOAD_FAILED",
                message="Artifact reload failed",
                hint=str(e),
            ),
        ) from e


def _ml_service_root() -> str:
    """Return the ml-service project root (directory containing the 'ml' package)."""
    import ml as _ml  # noqa: PLC0415

    return os.path.dirname(os.path.dirname(os.path.abspath(_ml.__file__)))


def _run_training_subprocess(
    module: str,
    extra_args: Optional[List[str]] = None,
    extra_env: Optional[Dict[str, str]] = None,
) -> None:
    """Run a training module as subprocess; raises on non-zero exit or timeout.
    Timeout from config (inputs.training_subprocess_timeout_sec) or env TRAINING_SUBPROCESS_TIMEOUT_SEC (default 7 days).
    Sets SKIP_PIPELINE_TRACKING=1 so the subprocess does not try to start tracking (go-app already owns the step).
    extra_env: optional env vars to merge into the subprocess env (e.g. AUTO_TUNE_N_JOBS for single-task auto-tune).
    """
    import subprocess

    from ml.config import get_training_subprocess_timeout_sec

    root = _ml_service_root()
    cmd = [sys.executable, "-m", module]
    if extra_args:
        cmd.extend(extra_args)
    env = {**os.environ, "SKIP_PIPELINE_TRACKING": "1"}
    if extra_env:
        env.update(extra_env)
    timeout_sec = get_training_subprocess_timeout_sec()
    try:
        proc = subprocess.run(
            cmd,
            cwd=root,
            env=env,
            capture_output=True,
            text=True,
            timeout=timeout_sec,
        )
    except subprocess.TimeoutExpired as e:
        logger.error("admin.train.timeout", module=module, timeout_sec=timeout_sec)
        raise ValueError(f"Training timed out after {timeout_sec}s") from e
    if proc.returncode != 0:
        stderr = (proc.stderr or "")[:500]
        logger.error(
            "admin.train.failed",
            module=module,
            returncode=proc.returncode,
            stderr=stderr,
        )
        raise ValueError(f"Training failed (exit {proc.returncode}): {stderr}")


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
                raise HTTPException(
                    status_code=500,
                    detail=_error_payload(
                        code="TRAIN_FAILED",
                        message=fail_message,
                        hint=str(e),
                    ),
                ) from e

        return wrapped

    return decorator


def _export_csvs_available(prefix: str) -> bool:
    """True if GO_APP_OUTPUT_DIR contains at least one CSV matching prefix (e.g. batting_encoded_*, bowling_encoded_*)."""
    out_dir = (os.environ.get("GO_APP_OUTPUT_DIR") or "").strip()
    if not out_dir or not os.path.isdir(out_dir):
        return False
    try:
        for name in os.listdir(out_dir):
            if name.startswith(prefix) and name.endswith(".csv"):
                return True
    except OSError:
        pass
    return False


def _unified_batting_csv_available() -> bool:
    """True if batting_encoded_all.csv (or legacy batting_encoded.csv) exists in export dir for unified model training."""
    from ml.config import default_go_app_export_dir

    out_dir = (os.environ.get("GO_APP_OUTPUT_DIR") or "").strip() or default_go_app_export_dir()
    if not out_dir or not os.path.isdir(out_dir):
        return False
    return os.path.isfile(os.path.join(out_dir, "batting_encoded_all.csv")) or os.path.isfile(
        os.path.join(out_dir, "batting_encoded.csv")
    )


def _unified_bowling_csv_available() -> bool:
    """True if bowling_encoded_all.csv (or legacy bowling_encoded.csv) exists in export dir for unified model training."""
    from ml.config import default_go_app_export_dir

    out_dir = (os.environ.get("GO_APP_OUTPUT_DIR") or "").strip() or default_go_app_export_dir()
    if not out_dir or not os.path.isdir(out_dir):
        return False
    return os.path.isfile(os.path.join(out_dir, "bowling_encoded_all.csv")) or os.path.isfile(
        os.path.join(out_dir, "bowling_encoded.csv")
    )


@app.post("/admin/train/batting")
@_require_admin_train("batting", "Batting training failed")
async def admin_train_batting(request: Request, cutoff: str = ""):
    """Run batting model training per format (TEST, ODI, T20I, T20).
    If query param cutoff (RFC3339) is set: fetch training data from go-app API (same as fielding).
    When cutoff is set but GO_APP_OUTPUT_DIR has batting_encoded_*.csv, prefer CSV to avoid API dependency.
    Otherwise: read from GO_APP_OUTPUT_DIR CSVs. Writes to MODELS_DIR. Guarded by ENABLE_HOT_RELOAD.
    """
    cutoff = (cutoff or "").strip()
    csv_available = _export_csvs_available("batting_encoded_")
    use_api = bool(cutoff) and not csv_available
    if use_api:
        go_app_url = _get_go_app_url()
        extra = ["--from-api", "--cutoff", cutoff, "--all-formats", "--go-app-url", go_app_url]
        logger.info("admin.train.start", step="batting", per_format=True, from_api=True, go_app_url=go_app_url)
    else:
        extra = ["--all-formats"]
        logger.info(
            "admin.train.start",
            step="batting",
            per_format=True,
            from_api=False,
            from_csv=bool(cutoff and csv_available),
        )
    _train_env = {"ML_N_JOBS": "-1"}  # Use resource-aware parallelism for faster training
    async with _get_training_semaphore():
        await asyncio.to_thread(_run_training_subprocess, "ml.train_batting", extra, _train_env)
    # Also train unified model (batting_model.joblib / batting_scaler.joblib) when unified CSV exists
    if _unified_batting_csv_available():
        try:
            from ml.train_batting_model import run_training as run_unified_batting

            # run_training() is called directly (no __main__ block), so it does not use
            # pipeline tracking; no need to set SKIP_PIPELINE_TRACKING (avoids thread-unsafe os.environ mutation).
            await asyncio.to_thread(run_unified_batting)
            logger.info("admin.train.success", step="batting", unified=True)
        except Exception as e:
            logger.warning("admin.train.unified_batting_failed", error=str(e))
    else:
        logger.info("admin.train.success", step="batting", unified=False)
    return {"status": "ok", "step": "batting"}


@app.post("/admin/train/bowling")
@_require_admin_train("bowling", "Bowling training failed")
async def admin_train_bowling(request: Request, cutoff: str = ""):
    """Run bowling model training per format (TEST, ODI, T20I, T20).
    If query param cutoff (RFC3339) is set: fetch training data from go-app API (same as fielding).
    When cutoff is set but GO_APP_OUTPUT_DIR has bowling_encoded_*.csv, prefer CSV to avoid API dependency.
    Otherwise: read from GO_APP_OUTPUT_DIR CSVs. Guarded by ENABLE_HOT_RELOAD.
    """
    cutoff = (cutoff or "").strip()
    csv_available = _export_csvs_available("bowling_encoded_")
    use_api = bool(cutoff) and not csv_available
    if use_api:
        go_app_url = _get_go_app_url()
        extra = ["--from-api", "--cutoff", cutoff, "--all-formats", "--go-app-url", go_app_url]
        logger.info("admin.train.start", step="bowling", per_format=True, from_api=True, go_app_url=go_app_url)
    else:
        extra = ["--all-formats"]
        logger.info(
            "admin.train.start",
            step="bowling",
            per_format=True,
            from_api=False,
            from_csv=bool(cutoff and csv_available),
        )
    _train_env = {"ML_N_JOBS": "-1"}  # Use resource-aware parallelism for faster training
    async with _get_training_semaphore():
        await asyncio.to_thread(_run_training_subprocess, "ml.train_bowling", extra, _train_env)
    # Also train unified model (bowling_model.joblib / bowling_scaler.joblib) when unified CSV exists
    if _unified_bowling_csv_available():
        try:
            from ml.train_bowling_model import run_training as run_unified_bowling

            # run_training() is called directly (no __main__ block), so it does not use
            # pipeline tracking; no need to set SKIP_PIPELINE_TRACKING (avoids thread-unsafe os.environ mutation).
            await asyncio.to_thread(run_unified_bowling)
            logger.info("admin.train.success", step="bowling", unified=True)
        except Exception as e:
            logger.warning("admin.train.unified_bowling_failed", error=str(e))
    else:
        logger.info("admin.train.success", step="bowling", unified=False)
    return {"status": "ok", "step": "bowling"}


@app.post("/admin/train/fielding")
@_require_admin_train("fielding", "Fielding training failed")
async def admin_train_fielding(request: Request, cutoff: str = ""):
    """Run fielding model training. Same pipeline as batting/bowling: optional cutoff.
    If cutoff provided: fetch from go-app training-data API. If omitted: use fielding_encoded_all.csv from GO_APP_OUTPUT_DIR (run export first).
    Guarded by ENABLE_HOT_RELOAD. Blocks until complete.
    """
    cutoff = (cutoff or "").strip()
    if cutoff:
        go_app_url = _get_go_app_url()
        args = ["--cutoff", cutoff, "--go-app-url", go_app_url]
        logger.info("admin.train.start", step="fielding", cutoff=cutoff, go_app_url=go_app_url)
    else:
        args = []
        logger.info("admin.train.start", step="fielding", source="csv")
    _train_env = {"ML_N_JOBS": "-1"}  # Use resource-aware parallelism for faster training
    async with _get_training_semaphore():
        await asyncio.to_thread(
            _run_training_subprocess,
            "ml.train_fielding",
            args,
            _train_env,
        )
    logger.info("admin.train.success", step="fielding")
    return {"status": "ok", "step": "fielding"}


@app.post("/admin/train/extras")
@_require_admin_train("extras", "Extras training failed")
async def admin_train_extras(request: Request, cutoff: str = ""):
    """Run extras model training (uses go-app training-data API). Requires query param cutoff (RFC3339).
    Guarded by ENABLE_HOT_RELOAD. Blocks until complete.
    """
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
    go_app_url = _get_go_app_url()
    logger.info("admin.train.start", step="extras", cutoff=cutoff, go_app_url=go_app_url)
    _train_env = {"ML_N_JOBS": "-1"}  # Use resource-aware parallelism for faster training
    async with _get_training_semaphore():
        await asyncio.to_thread(
            _run_training_subprocess,
            "ml.train_extras",
            ["--cutoff", cutoff, "--go-app-url", go_app_url],
            _train_env,
        )
    logger.info("admin.train.success", step="extras")
    return {"status": "ok", "step": "extras"}


@app.post("/admin/train/win")
@_require_admin_train("win", "Win training failed")
async def admin_train_win(request: Request, cutoff: str = ""):
    """Run win model training (uses go-app training-data API). Requires query param cutoff (RFC3339).
    Guarded by ENABLE_HOT_RELOAD. Blocks until complete.
    """
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
    go_app_url = _get_go_app_url()
    logger.info("admin.train.start", step="win", cutoff=cutoff, go_app_url=go_app_url)
    _train_env = {"ML_N_JOBS": "-1"}  # Use resource-aware parallelism for faster training
    async with _get_training_semaphore():
        await asyncio.to_thread(
            _run_training_subprocess,
            "ml.train_win",
            ["--cutoff", cutoff, "--go-app-url", go_app_url],
            _train_env,
        )
    logger.info("admin.train.success", step="win")
    return {"status": "ok", "step": "win"}


_VALID_AUTO_TUNE_MODELS = ("batting", "bowling", "fielding", "extras", "win", "all")
_VALID_AUTO_TUNE_FORMATS = ("TEST", "ODI", "T20", "T20I")


@app.post("/admin/train/auto-tune")
@_require_admin_train("auto-tune", "Auto-tune failed")
async def admin_train_auto_tune(
    request: Request,
    cutoff: str = "",
    model: str = "all",
    all_formats: str = "",
    unified: str = "",
):
    """Run auto-tune for selected model(s) and format(s).
    Query params: model (batting|bowling|fielding|extras|win|all), format (TEST|ODI|T20|T20I),
    all_formats (1|true = tune each per-format), unified (1|true = tune unified model only, no format).
    When all_formats is set, format is ignored. When unified is set, no --format or --all-formats is passed.
    Uses go-app training-data API (--from-api). Optional cutoff (RFC3339). Guarded by ENABLE_HOT_RELOAD.
    """
    model = (model or "all").strip().lower()
    if model not in _VALID_AUTO_TUNE_MODELS:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="INVALID_MODEL",
                message="Invalid model",
                hint=f"model must be one of: {', '.join(_VALID_AUTO_TUNE_MODELS)}",
            ),
        )
    use_all_formats = (all_formats or "").strip().lower() in ("1", "true", "yes")
    use_unified = (unified or "").strip().lower() in ("1", "true", "yes")
    fmt = (request.query_params.get("format") or "").strip().upper()
    if not use_all_formats and not use_unified:
        if not fmt or fmt not in _VALID_AUTO_TUNE_FORMATS:
            raise HTTPException(
                status_code=400,
                detail=_error_payload(
                    code="FORMAT_REQUIRED",
                    message="Single format required when not using all formats or unified",
                    hint="Pass format (TEST, ODI, T20, T20I), all_formats=1, or unified=1",
                ),
            )
    cutoff = (cutoff or "").strip()
    if not cutoff:
        cutoff = datetime.now(timezone.utc).strftime("%Y-%m-%dT00:00:00Z")
    go_app_url = _get_go_app_url()
    extra = [
        "--model",
        model,
        "--from-api",
        "--cutoff",
        cutoff,
        "--go-app-url",
        go_app_url,
    ]
    if use_all_formats:
        extra.append("--all-formats")
    elif not use_unified:
        extra.extend(["--format", fmt])
    # Use parallel when multiple (model, format) tasks will run; each parallel subprocess uses 1 job.
    # When single task (one model + one format or unified), allow multi-CPU via AUTO_TUNE_N_JOBS=-1 (resource-aware).
    single_task = model != "all" and (not use_all_formats or use_unified)
    if model == "all" or use_all_formats:
        extra.append("--parallel")
    subprocess_env: Optional[Dict[str, str]] = None
    if single_task:
        subprocess_env = {"AUTO_TUNE_N_JOBS": "-1"}
    api_key = (os.environ.get("GO_APP_API_KEY") or "").strip()
    if api_key:
        extra.extend(["--api-key", api_key])
    logger.info(
        "admin.train.start",
        step="auto-tune",
        cutoff=cutoff,
        go_app_url=go_app_url,
        model=model,
        all_formats=use_all_formats,
        unified=use_unified,
        format=fmt or None,
        single_task=single_task,
    )
    async with _get_training_semaphore():
        await asyncio.to_thread(
            _run_training_subprocess,
            "ml.auto_tune",
            extra,
            subprocess_env,
        )
    logger.info("admin.train.success", step="auto-tune")
    return {"status": "ok", "step": "auto-tune"}
