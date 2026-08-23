"""Backtest, batch, and historical match API models."""

from datetime import datetime
from typing import Dict, List, Optional

from pydantic import BaseModel, ConfigDict, Field, field_validator

class MatchContext(BaseModel):
    """Match context for hybrid reconciliation. When provided with innings model, player predictions are rescaled."""

    team1_player_ids: List[int] = Field(..., description="Player IDs for team 1 (bats in innings 1)")
    team2_player_ids: List[int] = Field(..., description="Player IDs for team 2 (bats in innings 2)")
    venue_id: float = Field(default=0, description="Venue ID for innings model")
    format_id: float = Field(default=0, description="Format ID for innings model")
    team1_opposition_id: float = Field(default=0, description="Opposition ID when team1 bats (team2)")
    team2_opposition_id: float = Field(default=0, description="Opposition ID when team2 bats (team1)")
    temp: int = Field(default=0, description="Weather temp")
    wind: int = Field(default=0, description="Weather wind")
    rain: int = Field(default=0, description="Weather rain")
    humidity: int = Field(default=0, description="Weather humidity")
    cloud: int = Field(default=0, description="Weather cloud")
    pressure: int = Field(default=0, description="Weather pressure")
    viscosity: int = Field(default=0, description="Weather viscosity")

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
        description="Per-player features from go-app; when present with format, use loaded models"
    )
    # When True: use latest model (artifacts or train-on-the-fly with "now" cutoff).
    # When False (default): strict temporal - train-on-the-fly uses cutoff for training data.
    # Default False preserves reproducibility for backtests; True is for QA/eval with current models.
    use_latest_model: bool = Field(
        default=False,
        description="Use latest model; when False, train strictly before cutoff_date"
    )
    match_context: Optional[MatchContext] = Field(
        default=None,
        description="Match context (team assignment, venue, etc.) for hybrid reconciliation"
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
    balls: Optional[float] = None
    fours: Optional[float] = None
    sixes: Optional[float] = None
    wickets: Optional[float] = None
    economy: Optional[float] = None
    catches: Optional[float] = None
    run_outs: Optional[float] = None

class BacktestPlayersResponse(BaseModel):
    players: List[BacktestPlayerPred]

class GenerateMatchRequest(BaseModel):
    """Request for POST /api/ml/generate-match: match context + players + features."""

    cutoff_date: datetime = Field(..., description="RFC3339 cutoff; train strictly before this date")
    player_ids: List[int] = Field(..., description="Player IDs for both teams")
    format: str = Field(..., description="Format code (e.g. T20, ODI)")
    features: Dict[str, Dict[str, float]] = Field(
        default_factory=dict,
        description="Per-player features (player_id as str -> feature name -> value)"
    )
    match_context: MatchContext = Field(..., description="Team assignment, venue, season, opposition, weather")
    use_latest_model: bool = Field(default=False, description="Use latest model when True")

    @field_validator("player_ids")
    def _player_ids_positive(cls, v: List[int]):
        for pid in v:
            if pid <= 0:
                raise ValueError("player_ids must be positive integers")
        return v

class InningsSummary(BaseModel):
    """One innings summary for generate-match response."""

    inning_number: int = Field(..., ge=1, le=2)
    runs: float = Field(..., ge=0)
    wickets: float = Field(..., ge=0, le=10)

class GenerateMatchResponse(BaseModel):
    """Response from POST /api/ml/generate-match: reconciled scorecards + win probability."""

    players: List[BacktestPlayerPred] = Field(..., description="Reconciled per-player predictions")
    innings: List[InningsSummary] = Field(..., description="Per-innings totals (1 and 2)")
    win_probability_team1: float = Field(..., ge=0, le=1, description="P(team1 wins) from win model")
    model_version: str = Field(default="", description="Model version label for traceability")

class BacktestMatchAgg(BaseModel):
    runs: float
    wickets: float
    extras: float
    winner_team_code: str

class BacktestMatchResponse(BaseModel):
    model_config = ConfigDict(protected_namespaces=())

    match: BacktestMatchAgg
    # Added to align with Go client expectations
    model_version: str

class BatchPredictItem(BaseModel):
    """One item in a batch prediction request — same fields as BacktestPredictRequest but for player predictions only."""

    cutoff_date: datetime = Field(..., description="RFC3339 cutoff; train strictly before this date")
    player_ids: List[int] = Field(..., min_length=1, max_length=100, description="Player IDs to predict for")
    format: str = Field(..., description="Format code (e.g. T20, ODI)")
    features: Dict[str, Dict[str, float]] = Field(
        default_factory=dict,
        description="Per-player features (player_id as str -> feature name -> value)"
    )
    use_latest_model: bool = Field(default=False, description="Use latest model when True")
    match_context: Optional[MatchContext] = Field(default=None, description="Match context for reconciliation")

    @field_validator("player_ids")
    def _player_ids_positive(cls, v: List[int]):
        for pid in v:
            if pid <= 0:
                raise ValueError("player_ids must be positive integers")
        return v

class BatchPredictRequest(BaseModel):
    """Request for POST /ml/backtest/predict-batch — multiple prediction sets in one call."""

    requests: List[BatchPredictItem] = Field(..., min_length=1, max_length=500)

class BatchPredictResultItem(BaseModel):
    """One result in a batch prediction response."""

    players: List[BacktestPlayerPred]

class BatchPredictResponse(BaseModel):
    """Response for POST /ml/backtest/predict-batch — one result per request item."""

    results: List[BatchPredictResultItem]

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
        description="Alternative to match_id: {format, team1, team2, match_date}"
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
    model_config = ConfigDict(protected_namespaces=())

    players: List[PlayerComparison]
    match: MatchComparison
    metrics: BacktestMetrics
    model_version: str
