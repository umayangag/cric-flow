import os
from typing import Dict, List, Optional, Tuple

import joblib
import numpy as np
import pandas as pd
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from ml.match_win_predict import predict_for_team
from ml.calculate_features import calculate_features
from ml.db import get_db_connection

app = FastAPI(title="Cricket ML Service", version="0.3.0")

ENABLE_HOT_RELOAD = os.environ.get("ENABLE_HOT_RELOAD", "").strip().lower() in {"1", "true", "yes"}


class BattingFeatures(BaseModel):
    batting_consistency: float
    batting_form: float
    batting_temp: int
    batting_wind: int
    batting_rain: int
    batting_humidity: int
    batting_cloud: int
    batting_pressure: int
    batting_viscosity: int
    batting_inning: int
    batting_session: int
    toss: int
    venue: float
    opposition: float
    season: int
    player_name: str
    format: Optional[str] = None


class BowlingFeatures(BaseModel):
    bowling_consistency: float
    bowling_form: float
    bowling_temp: int
    bowling_wind: int
    bowling_rain: int
    bowling_humidity: int
    bowling_cloud: int
    bowling_pressure: int
    bowling_viscosity: int
    batting_inning: int
    bowling_session: int
    toss: int
    bowling_venue: float
    bowling_opposition: float
    season: int
    player_name: str
    format: Optional[str] = None


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


# Load artifacts (per-format if available)
# Prefer ML_SERVICE_OUTPUT_DIR, then MODELS_DIR, then config.json default, else ../../output/ml-service
try:
    import config as svc_config  # from ml-service/config.py

    _cfg_default_models_dir = svc_config.default_artifacts_dir()
except Exception:
    _cfg_default_models_dir = os.path.abspath(
        os.path.join(os.path.dirname(__file__), "..", "..", "output", "ml-service")
    )

_default_models_dir = _cfg_default_models_dir
MODELS_DIR = os.environ.get(
    "ML_SERVICE_OUTPUT_DIR", os.environ.get("MODELS_DIR", _default_models_dir)
)

# Registries: map format code -> (scaler, model). Legacy unsuffixed artifacts are stored under key "_LEGACY_".
BAT_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}


def _error_payload(
    code: str, message: str, hint: Optional[str] = None, available: Optional[List[str]] = None
) -> dict:
    payload = {"code": code, "message": message}
    if hint:
        payload["hint"] = hint
    if available is not None:
        payload["available_formats"] = sorted([x for x in available if x != "_LEGACY_"])
    return payload


def _reload_artifacts() -> dict:
    """Rescan MODELS_DIR and reload registries.
    Returns a summary dict with loaded formats for batting and bowling.
    """
    BAT_MODELS.clear()
    BOWL_MODELS.clear()
    # Legacy
    try:
        bat_scaler = joblib.load(os.path.join(MODELS_DIR, "batting_scaler.joblib"))
        bat_model = joblib.load(os.path.join(MODELS_DIR, "batting_model.joblib"))
        BAT_MODELS["_LEGACY_"] = (bat_scaler, bat_model)
    except Exception:
        pass
    try:
        bowl_scaler = joblib.load(os.path.join(MODELS_DIR, "bowling_scaler.joblib"))
        bowl_model = joblib.load(os.path.join(MODELS_DIR, "bowling_model.joblib"))
        BOWL_MODELS["_LEGACY_"] = (bowl_scaler, bowl_model)
    except Exception:
        pass
    # Per-format
    try:
        for fname in os.listdir(MODELS_DIR):
            lf = fname.lower()
            if lf.startswith("batting_scaler_") and lf.endswith(".joblib"):
                code = fname[len("batting_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(MODELS_DIR, fname))
                mname = f"batting_model_{code}.joblib"
                mpath = os.path.join(MODELS_DIR, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BAT_MODELS[code] = (scaler, model)
            if lf.startswith("bowling_scaler_") and lf.endswith(".joblib"):
                code = fname[len("bowling_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(MODELS_DIR, fname))
                mname = f"bowling_model_{code}.joblib"
                mpath = os.path.join(MODELS_DIR, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BOWL_MODELS[code] = (scaler, model)
    except Exception:
        pass
    return {
        "loaded_batting_formats": sorted([k for k in BAT_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_bowling_formats": sorted([k for k in BOWL_MODELS.keys() if k != "_LEGACY_"]),
        "legacy_batting": "_LEGACY_" in BAT_MODELS,
        "legacy_bowling": "_LEGACY_" in BOWL_MODELS,
    }


# Initial load of artifacts (legacy + per-format)
try:
    bat_scaler = joblib.load(os.path.join(MODELS_DIR, "batting_scaler.joblib"))
    bat_model = joblib.load(os.path.join(MODELS_DIR, "batting_model.joblib"))
    BAT_MODELS["_LEGACY_"] = (bat_scaler, bat_model)
except Exception:
    pass

try:
    bowl_scaler = joblib.load(os.path.join(MODELS_DIR, "bowling_scaler.joblib"))
    bowl_model = joblib.load(os.path.join(MODELS_DIR, "bowling_model.joblib"))
    BOWL_MODELS["_LEGACY_"] = (bowl_scaler, bowl_model)
except Exception:
    pass

# Load per-format artifacts (files named *_{FORMAT}.joblib)
try:
    for fname in os.listdir(MODELS_DIR):
        lf = fname.lower()
        if lf.startswith("batting_scaler_") and lf.endswith(".joblib"):
            code = fname[len("batting_scaler_") : -len(".joblib")].upper()
            scaler = joblib.load(os.path.join(MODELS_DIR, fname))
            # find model counterpart
            mname = f"batting_model_{code}.joblib"
            mpath = os.path.join(MODELS_DIR, mname)
            if os.path.exists(mpath):
                model = joblib.load(mpath)
                BAT_MODELS[code] = (scaler, model)
        if lf.startswith("bowling_scaler_") and lf.endswith(".joblib"):
            code = fname[len("bowling_scaler_") : -len(".joblib")].upper()
            scaler = joblib.load(os.path.join(MODELS_DIR, fname))
            mname = f"bowling_model_{code}.joblib"
            mpath = os.path.join(MODELS_DIR, mname)
            if os.path.exists(mpath):
                model = joblib.load(mpath)
                BOWL_MODELS[code] = (scaler, model)
except Exception:
    # Don't crash on load errors; endpoints will fall back to zeros or return helpful errors
    pass


def _batting_feature_vector(f: BattingFeatures) -> List[float]:
    return [
        f.batting_consistency,
        f.batting_form,
        f.batting_temp,
        f.batting_wind,
        f.batting_rain,
        f.batting_humidity,
        f.batting_cloud,
        f.batting_pressure,
        f.batting_viscosity,
        f.batting_inning,
        f.batting_session,
        f.toss,
        f.venue,
        f.opposition,
        f.season,
    ]


def _bowling_feature_vector(f: BowlingFeatures) -> List[float]:
    return [
        f.bowling_consistency,
        f.bowling_form,
        f.bowling_temp,
        f.bowling_wind,
        f.bowling_rain,
        f.bowling_humidity,
        f.bowling_cloud,
        f.bowling_pressure,
        f.bowling_viscosity,
        f.batting_inning,
        f.bowling_session,
        f.toss,
        f.bowling_venue,
        f.bowling_opposition,
        f.season,
    ]


@app.get("/health")
async def health():
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

    X = np.array([_batting_feature_vector(f) for f in features], dtype=float)
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
        return preds
    except Exception:
        return [
            BattingPrediction(
                runs_scored=0.0,
                balls_faced=0.0,
                fours_scored=0.0,
                sixes_scored=0.0,
                batting_position=0.0,
                strike_rate=0.0,
            )
            for _ in features
        ]


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

    X = np.array([_bowling_feature_vector(f) for f in features], dtype=float)
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
        return preds
    except Exception:
        return [
            BowlingPrediction(runs_conceded=0.0, deliveries=0.0, wickets_taken=0.0, econ=0.0)
            for _ in features
        ]


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

    df = pd.DataFrame([p.dict() for p in players])
    predictions, _ = predict_for_team(df)
    return [PlayerPrediction(**p) for p in predictions.to_dict("records")]


@app.post("/precompute")
async def precompute():
    db_connection = get_db_connection()
    calculate_features(db_connection)
    return {"status": "ok"}


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
