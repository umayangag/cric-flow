from datetime import datetime
from typing import Dict, List, Optional

from pydantic import BaseModel, Field, field_validator

# -------------------- Feature input models --------------------


class BattingFeatures(BaseModel):
    batting_consistency: float = Field(..., ge=0)
    batting_form: float = Field(..., ge=0)
    batting_form_short: float = Field(default=0.0, ge=0)
    batting_form_long: float = Field(default=0.0, ge=0)
    batting_momentum: float = Field(default=0.0)
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
        return v2


class BowlingFeatures(BaseModel):
    bowling_consistency: float = Field(..., ge=0)
    bowling_form: float = Field(..., ge=0)
    bowling_form_short: float = Field(default=0.0, ge=0)
    bowling_form_long: float = Field(default=0.0, ge=0)
    bowling_momentum: float = Field(default=0.0)
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
        return v2


# -------------------- Backtest endpoint models --------------------


class BacktestPredictRequest(BaseModel):
    cutoff_date: datetime = Field(..., description="RFC3339 cutoff; train strictly before this date")
    # one of the following should be present
    player_ids: Optional[List[int]] = Field(default=None, description="Player IDs to predict for")
    teams: Optional[List[str]] = Field(default=None, description="Two team codes/names for match aggregates")
    # Optional: format code (e.g. T20, ODI) to select per-format models when using features
    format: Optional[str] = Field(default=None, description="Format code for model selection")
    # Optional: per-player feature map for full pipeline (player_id as str -> feature name -> value)
    features: Optional[Dict[str, Dict[str, float]]] = Field(
        default=None,
        description="Per-player features from go-app; when present with format, use loaded models",
    )

    @field_validator("teams")
    def _teams_len_two(cls, v: Optional[List[str]]):
        if v is None:
            return v
        if len(v) != 2:
            raise ValueError("teams must have exactly two items")
        return [v[0].strip().upper(), v[1].strip().upper()]

    @field_validator("player_ids")
    def _player_ids_positive(cls, v: Optional[List[int]]):
        if v is None:
            return v
        for pid in v:
            if pid <= 0:
                raise ValueError("player_ids must be positive integers")
        return v


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


# -------------------- Historical match backtest models --------------------


class HistoricalMatchFilter(BaseModel):
    format: str
    team1: str
    team2: str
    match_date: datetime

    @field_validator("format", mode="before")
    def _fmt_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        return v.strip().upper()

    @field_validator("team1", "team2", mode="before")
    def _teams_norm(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        return v.strip().upper()


class HistoricalMatchBacktestRequest(BaseModel):
    cutoff_date: datetime = Field(..., description="RFC3339 cutoff; train strictly before this date")
    match_id: Optional[int] = Field(default=None, description="Canonical match id")
    filters: Optional[HistoricalMatchFilter] = Field(
        default=None,
        description="Alternative to match_id: {format, team1, team2, match_date}",
    )

    @field_validator("match_id")
    def _match_id_pos(cls, v: Optional[int]):
        if v is None:
            return v
        if v <= 0:
            raise ValueError("match_id must be a positive integer")
        return v

    @field_validator("filters")
    def _exactly_one_selector(cls, v, info):
        data = info.data
        mid = data.get("match_id")
        # Exactly one of match_id or filters must be provided
        if (mid is None and v is None) or (mid is not None and v is not None):
            raise ValueError("provide exactly one of match_id or filters")
        return v


class PlayerPoint(BaseModel):
    runs: float
    wickets: Optional[float] = None
    economy: Optional[float] = None


class PlayerComparison(BaseModel):
    player_id: int
    player_name: Optional[str] = None
    predicted: PlayerPoint
    actual: PlayerPoint
    abs_error_runs: float
    abs_error_wickets: Optional[float] = None


class MatchComparison(BaseModel):
    predicted: BacktestMatchAgg
    actual: BacktestMatchAgg


class BacktestMetrics(BaseModel):
    mae_runs: float
    rmse_runs: float
    mae_wickets: Optional[float] = None
    winner_correct: Optional[bool] = None


class HistoricalMatchBacktestResponse(BaseModel):
    players: List[PlayerComparison]
    match: MatchComparison
    metrics: BacktestMetrics
    model_version: str
