from datetime import datetime
from typing import List, Optional

from pydantic import BaseModel, Field, field_validator

# -------------------- Feature input models --------------------


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
        # Allow empty/unknown formats by returning normalized value
        if v2 not in {"TEST", "ODI", "T20", "T20I"}:
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
    # Added to align with Go client expectations
    model_version: str


# -------------------- Team win endpoint models --------------------


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
