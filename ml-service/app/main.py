import os
from typing import List

import joblib
import numpy as np
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="Cricket ML Service", version="0.2.0")


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


# Load artifacts if present
# Prefer ML_SERVICE_OUTPUT_DIR, then MODELS_DIR, then config.json default, else ../../output/ml-service
try:
    import config as svc_config  # from ml-service/config.py
    _cfg_default_models_dir = svc_config.default_artifacts_dir()
except Exception:
    _cfg_default_models_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "output", "ml-service"))

_default_models_dir = _cfg_default_models_dir
MODELS_DIR = os.environ.get("ML_SERVICE_OUTPUT_DIR", os.environ.get("MODELS_DIR", _default_models_dir))
_bat_scaler = None
_bat_model = None
_bow_scaler = None
_bow_model = None

try:
    _bat_scaler = joblib.load(os.path.join(MODELS_DIR, "batting_scaler.joblib"))
    _bat_model = joblib.load(os.path.join(MODELS_DIR, "batting_model.joblib"))
except Exception:
    _bat_scaler = None
    _bat_model = None

try:
    _bow_scaler = joblib.load(os.path.join(MODELS_DIR, "bowling_scaler.joblib"))
    _bow_model = joblib.load(os.path.join(MODELS_DIR, "bowling_model.joblib"))
except Exception:
    _bow_scaler = None
    _bow_model = None


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
    return {
        "status": "ok",
        "batting_model": bool(_bat_model),
        "bowling_model": bool(_bow_model),
    }


@app.post("/predict/batting", response_model=List[BattingPrediction])
async def predict_batting(features: List[BattingFeatures]):
    if _bat_model is None:
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

    X = np.array([_batting_feature_vector(f) for f in features], dtype=float)
    if _bat_scaler is not None:
        X = _bat_scaler.transform(X)
    try:
        Y = _bat_model.predict(X)
        # Expect 6 outputs per row
        preds = []
        for row in Y:
            # handle both 1D and 2D
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
    if _bow_model is None:
        return [
            BowlingPrediction(runs_conceded=0.0, deliveries=0.0, wickets_taken=0.0, econ=0.0)
            for _ in features
        ]

    X = np.array([_bowling_feature_vector(f) for f in features], dtype=float)
    if _bow_scaler is not None:
        X = _bow_scaler.transform(X)
    try:
        Y = _bow_model.predict(X)
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
