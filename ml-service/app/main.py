import os
import time
import uuid
from datetime import datetime
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field, field_validator

from ml.match_win_predict import predict_for_team

from . import settings as app_settings
from .artifacts import BAT_MODELS, BOWL_MODELS
from .artifacts import reload as reload_artifacts
from .artifacts import summary as artifacts_summary
from .errors import error_payload
from .features import batting_feature_vector, bowling_feature_vector
from .logging import bind_request_context, get_struct_logger, init_logging

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


class BattingFeatures(BaseModel):
    batting_consistency: float = Field(..., ge=0)
    batting_form: float = Field(..., ge=0)
    batting_temp: int
    batting_wind: int = Field(..., ge=0)
    batting_rain: int = Field(..., ge=0)
    batting_humidity: int = Field(..., ge=0)
    batting_cloud: int = Field(..., ge=0)
    batting_pressure: int = Field(..., ge=0)
    batting_viscosity: int = Field(..., ge=0, le=1)
    batting_inning: int = Field(..., ge=1, le=2)
    batting_session: int = Field(..., ge=1, le=3)
    toss: int = Field(..., ge=0, le=1)
    venue: float
    opposition: float
    season: int = Field(..., ge=0)
    player_name: str
    format: Optional[str] = None

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        v2 = v.strip().upper()
        if v2 not in {"TEST", "ODI", "T20", "T20I"}:
            # allow empty/unknown formats by returning original; the route enforces when required
            return v2
        return v2


class BowlingFeatures(BaseModel):
    bowling_consistency: float = Field(..., ge=0)
    bowling_form: float = Field(..., ge=0)
    bowling_temp: int
    bowling_wind: int = Field(..., ge=0)
    bowling_rain: int = Field(..., ge=0)
    bowling_humidity: int = Field(..., ge=0)
    bowling_cloud: int = Field(..., ge=0)
    bowling_pressure: int = Field(..., ge=0)
    bowling_viscosity: int = Field(..., ge=0, le=1)
    batting_inning: int = Field(..., ge=1, le=2)
    bowling_session: int = Field(..., ge=1, le=3)
    toss: int = Field(..., ge=0, le=1)
    bowling_venue: float
    bowling_opposition: float
    season: int = Field(..., ge=0)
    player_name: str
    format: Optional[str] = None

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        v2 = v.strip().upper()
        if v2 not in {"TEST", "ODI", "T20", "T20I"}:
            return v2
        return v2


# -------------------- Backtest endpoint models --------------------


class BacktestPredictRequest(BaseModel):
    cutoff_date: datetime = Field(..., description="RFC3339 cutoff; train strictly before this date")
    # one of the following should be present
    player_ids: Optional[List[int]] = Field(default=None, description="Player IDs to predict for")
    teams: Optional[List[str]] = Field(default=None, description="Two team codes/names for match aggregates")

    @field_validator("teams")
    def _teams_len_two(cls, v: Optional[List[str]]):
        if v is None:
            return v
        if len(v) != 2:
            raise ValueError("teams must have exactly two items")
        return [str(v[0]).strip().upper(), str(v[1]).strip().upper()]

    @field_validator("player_ids")
    def _player_ids_positive(cls, v: Optional[List[int]]):
        if v is None:
            return v
        for pid in v:
            if int(pid) <= 0:
                raise ValueError("player_ids must be positive integers")
        return [int(pid) for pid in v]


class BacktestPlayerPred(BaseModel):
    player_id: int
    runs: float
    wickets: Optional[float] = None
    economy: Optional[float] = None
    catches: Optional[float] = None
    run_outs: Optional[float] = None


class BacktestPlayersResponse(BaseModel):
    players: List[BacktestPlayerPred]


class BacktestMatchAgg(BaseModel):
    runs: float
    wickets: float
    extras: float
    winner_team_code: str


class BacktestMatchResponse(BaseModel):
    match: BacktestMatchAgg


def _deterministic_rng_seed(*parts: str) -> int:
    """Build a stable 32-bit seed from text parts."""
    acc = 0x345678
    for p in parts:
        for ch in str(p):
            acc = (acc * 1000003) ^ ord(ch)
        acc &= 0xFFFFFFFF
    return acc or 42


def _predict_players_baseline(cutoff: datetime, player_ids: List[int]) -> List[BacktestPlayerPred]:
    global BACKTEST_PLAYERS_COMPUTE_COUNT
    BACKTEST_PLAYERS_COMPUTE_COUNT += 1
    # Deterministic simple baseline: pseudo-random but stable per (cutoff, pid)
    out: List[BacktestPlayerPred] = []
    for pid in player_ids:
        seed = _deterministic_rng_seed(cutoff.isoformat(), str(pid))
        rng = np.random.default_rng(seed)
        # Runs in [0, 100) but skewed
        runs = float(np.round(rng.normal(20.0, 12.0)))
        runs = float(max(0.0, runs))
        # Wickets mostly 0-3
        wickets = float(max(0.0, np.round(rng.uniform(0.0, 3.0), 1)))
        # Economy between 5 and 10
        economy = float(np.round(5.0 + rng.random() * 5.0, 1))
        # Fielding: small integer counts 0–3, deterministic
        catches = float(int(rng.integers(0, 4)))
        run_outs = float(int(rng.integers(0, 3)))
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
    return out


def _predict_match_baseline(cutoff: datetime, teams: List[str]) -> BacktestMatchAgg:
    global BACKTEST_MATCH_COMPUTE_COUNT
    BACKTEST_MATCH_COMPUTE_COUNT += 1
    a, b = teams[0].upper(), teams[1].upper()
    seed = _deterministic_rng_seed(cutoff.isoformat(), a, b)
    rng = np.random.default_rng(seed)
    runs = float(np.round(120 + rng.normal(0, 20)))
    runs = float(max(50.0, runs))
    wickets = float(int(np.clip(np.round(rng.uniform(4, 8)), 2, 10)))
    extras = float(int(np.clip(np.round(rng.uniform(5, 15)), 0, 25)))
    # Winner: pick based on a stable comparison of hashed values
    w_seed_a = _deterministic_rng_seed(a)
    w_seed_b = _deterministic_rng_seed(b)
    winner = a if (w_seed_a ^ seed) >= (w_seed_b ^ seed) else b
    return BacktestMatchAgg(runs=runs, wickets=wickets, extras=extras, winner_team_code=winner)


@app.post("/ml/backtest/predict")
def backtest_predict(req: BacktestPredictRequest):
    # Strict cutoff semantics are honored implicitly by not using post-cutoff data.
    # We still parse/validate the timestamp.
    cutoff = req.cutoff_date
    cutoff_iso = cutoff.replace(tzinfo=None).isoformat() + "Z" if cutoff.tzinfo else cutoff.isoformat() + "Z"
    if req.player_ids is not None:
        cached = _cache_get("players", cutoff_iso, list(req.player_ids))
        if cached is not None:
            return JSONResponse(status_code=200, content=cached)
        preds = _predict_players_baseline(cutoff, req.player_ids)
        body = BacktestPlayersResponse(players=preds).model_dump()
        _cache_put("players", cutoff_iso, list(req.player_ids), body)
        return JSONResponse(status_code=200, content=body)
    if req.teams is not None:
        cached = _cache_get("match", cutoff_iso, list(req.teams))
        if cached is not None:
            return JSONResponse(status_code=200, content=cached)
        match = _predict_match_baseline(cutoff, req.teams)
        body = BacktestMatchResponse(match=match).model_dump()
        _cache_put("match", cutoff_iso, list(req.teams), body)
        return JSONResponse(status_code=200, content=body)
    raise HTTPException(
        status_code=400,
        detail=error_payload(
            code="INVALID_REQUEST",
            message="provide either player_ids or teams",
            hint="Body must include one of: {player_ids:[..]} or {teams:[team1,team2]}",
        ),
    )


class BattingPrediction(BaseModel):
    runs_scored: float
    balls_faced: float
    fours_scored: float
    sixes_scored: float
    batting_position: float
    strike_rate: float


class BowlingPrediction(BaseModel):
    runs_conceded: float
    deliveries: float
    wickets_taken: float
    econ: float


class PlayerPrediction(BaseModel):
    player_name: str
    runs_scored: float
    balls_faced: float
    fours_scored: float
    sixes_scored: float
    batting_position: float
    strike_rate: float
    runs_conceded: float
    deliveries: float
    wickets_taken: float
    econ: float
    winning_probability: Optional[float] = None


class TeamWinResponse(BaseModel):
    players: List[PlayerPrediction]
    team_win_probability: float


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
    reload_artifacts(MODELS_DIR)
except Exception:
    # Don't crash on load errors; endpoints will fall back to zeros or return helpful errors
    pass


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
                    except Exception:
                        out.append({"file": fname})
        except Exception:
            pass
        return sorted(out, key=lambda x: x.get("file", ""))

    def _metadata_info(prefix: str) -> List[str]:
        names: List[str] = []
        try:
            for fname in os.listdir(MODELS_DIR):
                lf = fname.lower()
                if lf.startswith(prefix) and lf.endswith(".json"):
                    names.append(fname)
        except Exception:
            pass
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


@app.post("/predict/batting", response_model=List[BattingPrediction])
async def predict_batting(features: List[BattingFeatures]):
    if not features:
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


@app.post("/predict-win", response_model=List[PlayerPrediction])
async def predict_win(players: List[PlayerPrediction]):
    if not players:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="EMPTY_BATCH",
                message="Empty players list",
                hint="Send at least one player with the required fields.",
            ),
        )

    logger.info("predict.win.start", players=len(players))
    try:
        df = pd.DataFrame([p.model_dump() for p in players])
        predictions, _ = predict_for_team(df)
        out = [PlayerPrediction(**p) for p in predictions.to_dict("records")]
        logger.info("predict.win.success", players=len(out))
        return out
    except Exception as exc:
        logger.exception("predict.win.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="PREDICT_FAILED",
                message="Team win prediction failed",
                hint="See server logs for stacktrace using request_id",
            ),
        )


@app.post("/predict/win", response_model=TeamWinResponse)
async def predict_win_wrapped(players: List[PlayerPrediction]):
    if not players:
        raise HTTPException(
            status_code=400,
            detail=_error_payload(
                code="EMPTY_BATCH",
                message="Empty players list",
                hint="Send at least one player with the required fields.",
            ),
        )

    df = pd.DataFrame([p.model_dump() for p in players])
    predictions, team_mean = predict_for_team(df)
    wrapped = TeamWinResponse(
        players=[PlayerPrediction(**p) for p in predictions.to_dict("records")],
        team_win_probability=float(team_mean),
    )
    return wrapped


@app.post("/admin/reload")
async def admin_reload():
    """Rescan the models directory and reload artifacts.
    Guarded by ENABLE_HOT_RELOAD env flag to avoid accidental reloads in prod.
    """
    if not ENABLE_HOT_RELOAD:
        raise HTTPException(
            status_code=403,
            detail=_error_payload(
                code="RELOAD_DISABLED",
                message="Hot reload is disabled",
                hint="Set ENABLE_HOT_RELOAD=1 to enable /admin/reload.",
            ),
        )
    summary = _reload_artifacts()
    return {"status": "reloaded", **summary}
