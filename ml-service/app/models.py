from datetime import datetime
from typing import Dict, List, Optional

from pydantic import BaseModel, ConfigDict, Field, field_validator

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
    match_date_unix: float = Field(default=0.0, ge=0)
    player_name: str
    format: Optional[str] = None
    # Optional sequential features (0 when absent; used when go-app exports with -enable-seq)
    bat_prev_sr: float = Field(default=0.0, ge=0)
    bat_prev_out_rate: float = Field(default=0.0, ge=0)
    bat_window_sr_12_pp: float = Field(default=0.0, ge=0)
    bat_window_boundary_rate_12_pp: float = Field(default=0.0, ge=0)
    bat_entry_sr_1_6: float = Field(default=0.0, ge=0)
    bat_set_sr_13_30: float = Field(default=0.0, ge=0)
    bat_react_after_dot_sr: float = Field(default=0.0, ge=0)
    bat_after_k_dots_boundary_p_k2: float = Field(default=0.0, ge=0)

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
    bowling_career_avg: float = Field(default=0.0, ge=0)
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
    match_date_unix: float = Field(default=0.0, ge=0)
    player_name: str
    format: Optional[str] = None
    # Optional sequential features (0 when absent; used when go-app exports with -enable-seq)
    bowl_prev_wkt_rate: float = Field(default=0.0, ge=0)
    bowl_window_econ_24_death: float = Field(default=0.0, ge=0)
    bowl_window_wkt_rate_24_death: float = Field(default=0.0, ge=0)
    bowl_extras_wide_rate_pp: float = Field(default=0.0, ge=0)
    bowl_react_after_boundary_wkt_rate_next: float = Field(default=0.0, ge=0)
    bowl_spell_first_over_wkt_rate: float = Field(default=0.0, ge=0)
    bowl_over_ball1_wkt_rate: float = Field(default=0.0, ge=0)
    bowl_over_ball6_wkt_rate: float = Field(default=0.0, ge=0)

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        v2 = v.strip().upper()
        return v2


# -------------------- Backtest endpoint models --------------------


class MatchContext(BaseModel):
    """Match context for hybrid reconciliation. When provided with innings model, player predictions are rescaled."""

    team1_player_ids: List[int] = Field(..., description="Player IDs for team 1 (bats in innings 1)")
    team2_player_ids: List[int] = Field(..., description="Player IDs for team 2 (bats in innings 2)")
    venue_id: float = Field(default=0, description="Venue ID for innings model")
    season_id: float = Field(default=0, description="Season ID for innings model")
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
        description="Per-player features from go-app; when present with format, use loaded models",
    )
    # When True: use latest model (artifacts or train-on-the-fly with "now" cutoff).
    # When False (default): strict temporal - train-on-the-fly uses cutoff for training data.
    # Default False preserves reproducibility for backtests; True is for QA/eval with current models.
    use_latest_model: bool = Field(
        default=False,
        description="Use latest model; when False, train strictly before cutoff_date",
    )
    match_context: Optional[MatchContext] = Field(
        default=None,
        description="Match context (team assignment, venue, etc.) for hybrid reconciliation",
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
        description="Per-player features (player_id as str -> feature name -> value)",
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


# -------------------- Extras / Win prediction (match-level, unified features) --------------------


class ExtrasFeatures(BaseModel):
    """Match-level features for extras model. Same families as batting/bowling/fielding."""

    format_id: int = Field(default=0, ge=0, description="Format dimension id")
    venue_id: int = Field(default=0, ge=0)
    season_id: int = Field(default=0, ge=0)
    match_date_unix: float = Field(default=0.0, ge=0)
    temp: int = Field(default=0)
    wind: int = Field(default=0, ge=0)
    rain: int = Field(default=0, ge=0)
    humidity: int = Field(default=0, ge=0)
    cloud: int = Field(default=0, ge=0)
    pressure: int = Field(default=0, ge=0)
    viscosity: int = Field(default=0, ge=0, le=2)
    bat_consistency_sum: float = Field(default=0.0, ge=0)
    bowl_consistency_sum: float = Field(default=0.0, ge=0)
    bat_form_sum: float = Field(default=0.0, ge=0)
    bowl_form_sum: float = Field(default=0.0, ge=0)
    format: Optional[str] = Field(
        default=None, description="Format code for per-format model selection (e.g. T20, ODI)"
    )

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None or v == "":
            return v
        return v.strip().upper()


class ExtrasPrediction(BaseModel):
    total_extras: float = Field(..., description="Predicted total extras for the match")


class WinFeatures(BaseModel):
    """Match-level features for win model. Same families as batting/bowling/fielding."""

    format_id: int = Field(default=0, ge=0)
    venue_id: int = Field(default=0, ge=0)
    match_date_unix: float = Field(default=0.0, ge=0)
    team1_opposition_id: int = Field(default=0, ge=0)
    team2_opposition_id: int = Field(default=0, ge=0)
    toss_winner_opposition_id: int = Field(default=0, ge=0)
    temp: int = Field(default=0)
    wind: int = Field(default=0, ge=0)
    rain: int = Field(default=0, ge=0)
    humidity: int = Field(default=0, ge=0)
    cloud: int = Field(default=0, ge=0)
    pressure: int = Field(default=0, ge=0)
    viscosity: int = Field(default=0, ge=0, le=2)
    team1_bat_consistency_sum: float = Field(default=0.0, ge=0)
    team1_bowl_consistency_sum: float = Field(default=0.0, ge=0)
    team2_bat_consistency_sum: float = Field(default=0.0, ge=0)
    team2_bowl_consistency_sum: float = Field(default=0.0, ge=0)
    team1_bat_form_sum: float = Field(default=0.0, ge=0)
    team1_bowl_form_sum: float = Field(default=0.0, ge=0)
    team2_bat_form_sum: float = Field(default=0.0, ge=0)
    team2_bowl_form_sum: float = Field(default=0.0, ge=0)
    format: Optional[str] = Field(default=None, description="Format code for per-format model selection")

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None or v == "":
            return v
        return v.strip().upper()


class WinFeaturesEnhanced(BaseModel):
    """Enhanced win prediction request: match context + per-player feature maps.

    The ML service aggregates per-player features into distribution statistics
    (mean, std, max, min, top3_mean) and computes derived matchup features.
    """

    format_id: int = Field(default=0, ge=0)
    venue_id: int = Field(default=0, ge=0)
    match_date_unix: float = Field(default=0.0, ge=0)
    team1_opposition_id: int = Field(default=0, ge=0)
    team2_opposition_id: int = Field(default=0, ge=0)
    toss_winner_opposition_id: int = Field(default=0, ge=0)
    temp: int = Field(default=0)
    wind: int = Field(default=0, ge=0)
    rain: int = Field(default=0, ge=0)
    humidity: int = Field(default=0, ge=0)
    cloud: int = Field(default=0, ge=0)
    pressure: int = Field(default=0, ge=0)
    viscosity: int = Field(default=0, ge=0, le=2)
    team1_player_features: Dict[str, Dict[str, float]] = Field(
        ..., description="Per-player feature maps for team1: {player_id: {feature_name: value}}"
    )
    team2_player_features: Dict[str, Dict[str, float]] = Field(
        ..., description="Per-player feature maps for team2: {player_id: {feature_name: value}}"
    )
    format: Optional[str] = Field(default=None, description="Format code for per-format model selection")

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None or v == "":
            return v
        return v.strip().upper()


class WinPrediction(BaseModel):
    team1_win_probability: float = Field(..., ge=0, le=1, description="Probability that team1 (batting first) wins")


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
    model_config = ConfigDict(protected_namespaces=())

    players: List[PlayerComparison]
    match: MatchComparison
    metrics: BacktestMetrics
    model_version: str


# -------------------- Reconciliation inputs (preferred values) --------------------


class PlayerReconciliationPreferences(BaseModel):
    """Preferred per-player outputs from individual models for one match.

    This aggregates model-specific predictions into a single structure that the
    reconciliation layer can consume. All fields are optional to allow use in
    flows where only a subset of models are present (e.g. no fielding model).
    """

    player_id: int = Field(..., description="Canonical player id")
    team_id: int = Field(..., description="Team id (batting first = team1, chasing = team2)")

    # Per-player model outputs (preferred / unconstrained values)
    batting: Optional[BattingPrediction] = Field(
        default=None, description="Preferred batting line for this player (runs, balls, boundaries)."
    )
    bowling: Optional[BowlingPrediction] = Field(
        default=None, description="Preferred bowling line for this player (runs conceded, balls, wickets, econ)."
    )

    # Fielding preferences can be extended later if/when a dedicated fielding model is wired in.
    catches: Optional[float] = Field(
        default=None,
        description="Preferred number of catches from fielding model (if available).",
        ge=0,
    )
    run_outs: Optional[float] = Field(
        default=None,
        description="Preferred number of run-outs from fielding model (if available).",
        ge=0,
    )


class InningsReconciliationPreferences(BaseModel):
    """Preferred match-level outputs for a single innings.

    These typically come from an innings model and extras model; they act as
    targets that player-level lines should reconcile to.
    """

    inning_number: int = Field(..., ge=1, le=4)
    batting_team_id: int = Field(..., description="Team id batting in this innings")
    bowling_team_id: int = Field(..., description="Team id bowling in this innings")

    # Innings model outputs (preferred totals)
    preferred_runs: Optional[float] = Field(
        default=None,
        description="Preferred total runs for this innings (e.g. from innings model).",
        ge=0,
    )
    preferred_wickets: Optional[float] = Field(
        default=None,
        description="Preferred wickets lost in this innings (e.g. from innings model).",
        ge=0,
        le=10,
    )

    # Optional preferred balls/termination info for innings-end modeling
    preferred_legal_balls: Optional[float] = Field(
        default=None,
        description="Preferred number of legal balls bowled in this innings.",
        ge=0,
    )
    balls_per_innings: Optional[int] = Field(
        default=None,
        description="Maximum legal balls for this innings (format cap, e.g. 120 for T20).",
        ge=0,
    )
    is_all_out: Optional[bool] = Field(
        default=None,
        description="When set, indicates whether the innings is expected to end by all-out (True) or by overs/target (False).",
    )

    # Extras model outputs (preferred extras for this innings or match, depending on usage)
    preferred_extras: Optional[ExtrasPrediction] = Field(
        default=None,
        description="Preferred extras prediction to align with per-bowler and per-batsman lines.",
    )


class MatchReconciliationInputs(BaseModel):
    """Bundle of all preferred model outputs needed for reconciliation.

    This ties together:
    - Per-player batting/bowling/fielding preferences (μ_player)
    - Per-innings totals preferences from innings/extras models (μ_innings)
    - Optional win model output (μ_win) for diagnostic consistency checks.
    """

    match_id: int = Field(..., description="Canonical match id for these preferences")
    format: Optional[str] = Field(default=None, description="Format code (T20, ODI, TEST, etc.)")

    players: List[PlayerReconciliationPreferences] = Field(
        ..., description="Preferred per-player outputs for all players in the match."
    )
    innings: List[InningsReconciliationPreferences] = Field(
        ..., description="Preferred per-innings totals and extras for the match."
    )
    win: Optional[WinPrediction] = Field(
        default=None,
        description="Preferred win probability for team1 (optional; used for monitoring/soft consistency).",
    )
