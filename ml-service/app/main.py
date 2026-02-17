import os
import time
import uuid
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from . import settings as app_settings
from .artifacts import BAT_MODELS, BOWL_MODELS, FIELD_MODELS
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
from .features import batting_feature_vector, bowling_feature_vector, fielding_feature_vector
from .logging import bind_request_context, get_struct_logger, init_logging
from .models import (
    BacktestMatchResponse,
    BacktestPlayerPred,
    BacktestPlayersResponse,
    BacktestPredictRequest,
    BattingFeatures,
    BattingPrediction,
    BowlingFeatures,
    BowlingPrediction,
    HistoricalMatchBacktestRequest,
)
from .train_on_the_fly import train_on_the_fly_cached

try:
    from ml.config import get_prediction_defaults
except ImportError:
    get_prediction_defaults = None

app = FastAPI(title="Cricket ML Service", version="0.3.0")

# Initialize logging early
init_logging(service="ml-service", version=app.version)
logger = get_struct_logger()

ENABLE_HOT_RELOAD = os.environ.get("ENABLE_HOT_RELOAD", "").strip().lower() in {"1", "true", "yes"}

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

    # Feature order must match training (configs/feature_vectors.json). Normalize with same scaler as at training.
    X_bat = np.array([batting_feature_vector(f) for f in bat_features], dtype=float)
    if scaler_bat is not None:
        X_bat = scaler_bat.transform(X_bat)
    Y_bat = model_bat.predict(X_bat)

    X_bowl = np.array([bowling_feature_vector(f) for f in bowl_features], dtype=float)
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

    # Fielding: if we have fielding artifacts, predict catches/run_outs and merge into player preds
    field_pair = FIELD_MODELS.get(fmt_upper) if fmt_upper else None
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
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.debug("artifacts_status.find_artifact.listdir_failed", models_dir=models_dir, fmt=fmt, error=str(e))
        return None
    fmt_lower = fmt.lower()
    prefer_prefix = "batting_" if batting else "bowling_"
    token = "bat" if batting else "bowl"
    preferred_name = f"{prefer_prefix}{fmt}.joblib"
    # First pass: exact preferred name
    for name in entries:
        if name == preferred_name:
            path = os.path.join(models_dir, name)
            try:
                st = os.stat(path)
                if not os.path.isdir(path) and name.lower().endswith(".joblib"):
                    return path, st.st_mtime
            except Exception:
                return None
    # Second pass: tolerant match
    for name in entries:
        lower = name.lower()
        if lower.endswith(".joblib") and (token in lower) and (fmt_lower in lower):
            path = os.path.join(models_dir, name)
            try:
                st = os.stat(path)
                if not os.path.isdir(path):
                    return path, st.st_mtime
            except Exception:
                continue
    return None


@app.get("/artifacts/status")
async def artifacts_status():
    """Report presence and (optionally) loaded state of artifacts per format.

    Shape:
    {
      "timestamp": ISO8601,
      "root": MODELS_DIR,
      "formats": {
        "ODI": {"batting": {"exists": bool, "path": str?, "modified": str?, "loaded": bool?}, "bowling": {...}},
        ...
      }
    }
    """
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    root = MODELS_DIR
    formats: Dict[str, Dict[str, Any]] = {}
    for fmt in _FORMATS:
        b_obj: Dict[str, Any] = {"exists": False}
        bow_obj: Dict[str, Any] = {"exists": False}
        # Filesystem presence
        b_hit = _find_artifact(root, fmt, batting=True)
        if b_hit is not None:
            p, mt = b_hit
            b_obj["exists"] = True
            b_obj["path"] = p
            b_obj["modified"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mt))
        w_hit = _find_artifact(root, fmt, batting=False)
        if w_hit is not None:
            p, mt = w_hit
            bow_obj["exists"] = True
            bow_obj["path"] = p
            bow_obj["modified"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mt))
        # Loaded state (best-effort)
        try:
            if fmt in BAT_MODELS:
                b_obj["loaded"] = True
        except Exception as e:
            logger.debug("artifacts_status.batting_loaded_check", fmt=fmt, error=str(e))
        try:
            if fmt in BOWL_MODELS:
                bow_obj["loaded"] = True
        except Exception as e:
            logger.debug("artifacts_status.bowling_loaded_check", fmt=fmt, error=str(e))
        formats[fmt] = {"batting": b_obj, "bowling": bow_obj}

    return {"timestamp": ts, "root": root, "formats": formats}


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


@app.post("/admin/reload")
async def admin_reload():
    """Rescan the models directory and reload artifacts.
    Guarded by ENABLE_HOT_RELOAD env flag to avoid accidental reloads in prod.
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
