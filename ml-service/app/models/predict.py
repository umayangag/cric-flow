"""Prediction output and match-level feature models."""

from typing import Dict, List, Optional

from pydantic import BaseModel, Field, field_validator


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


class ExtrasFeatures(BaseModel):
    """Match-level features for extras model. Same families as batting/bowling/fielding."""

    format_id: int = Field(default=0, ge=0, description="Format dimension id")
    venue_id: int = Field(default=0, ge=0)
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

    def to_match_context_dict(self) -> Dict[str, float]:
        """Extract the MATCH_CONTEXT_COLS subset as a float dict for the aggregation pipeline."""
        return {
            "format_id": float(self.format_id),
            "venue_id": float(self.venue_id),
            "team1_opposition_id": float(self.team1_opposition_id),
            "team2_opposition_id": float(self.team2_opposition_id),
            "toss_winner_opposition_id": float(self.toss_winner_opposition_id),
            "temp": float(self.temp),
            "wind": float(self.wind),
            "rain": float(self.rain),
            "humidity": float(self.humidity),
            "cloud": float(self.cloud),
            "pressure": float(self.pressure),
            "viscosity": float(self.viscosity),
        }


class WinPrediction(BaseModel):
    team1_win_probability: float = Field(..., ge=0, le=1, description="Probability that team1 (batting first) wins")


# -------------------- Team selection optimisation models --------------------


class TeamOptimizationPoolPlayer(BaseModel):
    player_id: int
    name: str
    is_bowler: bool
    is_keeper: bool
    bat_score: float
    bowl_score: float
    field_score: float = 0.0
    features: Dict[str, float]


class TeamOptimizationWeights(BaseModel):
    bat: float = 0.45
    bowl: float = 0.40
    field: float = 0.10
    keeper_bonus: float = 0.02


class TeamOptimizationConstraints(BaseModel):
    size: int = Field(default=11, ge=1)
    min_bowlers: int = Field(default=5, ge=0)
    require_keeper: bool = True


class TeamOptimizationRequest(BaseModel):
    """Request for server-side team selection optimisation.

    Sends the full player pool, opponent features, and match context in a single
    call so the ML service can run hill-climb optimisation with direct model
    access and batch inference — eliminating per-candidate HTTP round-trips.
    """

    pool: List[TeamOptimizationPoolPlayer] = Field(..., min_length=1)
    opponent_features: Dict[str, Dict[str, float]] = Field(
        ..., description="{player_id: {feature_name: value}} for the fixed opponent team"
    )
    match_context: Dict[str, float] = Field(
        ..., description="MATCH_CONTEXT_COLS values (format_id, venue_id, opposition IDs, weather, etc.)"
    )
    constraints: TeamOptimizationConstraints = Field(default_factory=TeamOptimizationConstraints)
    weights: TeamOptimizationWeights = Field(default_factory=TeamOptimizationWeights)
    team_is_team1: bool = Field(default=True, description="Whether the pool represents team1 (batting first) or team2")
    format: Optional[str] = Field(default=None, description="Format code for per-format model selection")
    max_iterations: int = Field(default=50, ge=1, le=200)
    max_evals: int = Field(default=500, ge=1, le=5000)

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None or v == "":
            return v
        return v.strip().upper()


class TeamOptimizationSelectedPlayer(BaseModel):
    player_id: int
    name: str


class TeamOptimizationResponse(BaseModel):
    selected: List[TeamOptimizationSelectedPlayer]
    win_probability: float = Field(..., ge=0, le=1)
    iterations_used: int
    evals_performed: int
