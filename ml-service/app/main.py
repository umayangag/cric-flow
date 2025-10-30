from fastapi import FastAPI
from pydantic import BaseModel, Field
from typing import List, Optional

app = FastAPI(title="Cricket ML Service", version="0.1.0")


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


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/predict/batting", response_model=List[BattingPrediction])
async def predict_batting(features: List[BattingFeatures]):
    # TODO: load models and scalers; for now, return zeros as placeholder
    preds = []
    for _ in features:
        preds.append(BattingPrediction(
            runs_scored=0.0,
            balls_faced=0.0,
            fours_scored=0.0,
            sixes_scored=0.0,
            batting_position=0.0,
            strike_rate=0.0,
        ))
    return preds


@app.post("/predict/bowling", response_model=List[BowlingPrediction])
async def predict_bowling(features: List[BowlingFeatures]):
    # TODO: load models and scalers; for now, return zeros as placeholder
    preds = []
    for _ in features:
        preds.append(BowlingPrediction(
            runs_conceded=0.0,
            deliveries=0.0,
            wickets_taken=0.0,
            econ=0.0,
        ))
    return preds
