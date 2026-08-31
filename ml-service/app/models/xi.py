"""Request / response models for the XI-responsive win endpoints (``/xi/*``).

Players are identified by id only. The ML service holds the as-of rating state, so callers
send who is playing, not what their features are -- which is also what makes the training
and serving paths compute the same function of the same eleven names (S-3c).
"""

from __future__ import annotations

from typing import Dict, List, Optional

from pydantic import BaseModel, Field, field_validator


def _upper(v: Optional[str]) -> Optional[str]:
    return v if v is None or v == "" else v.strip().upper()


class XiConstraints(BaseModel):
    team_size: int = Field(default=11, ge=1, le=15)
    min_bowlers: int = Field(default=5, ge=0, le=11)
    require_keeper: bool = True
    must_include: List[int] = Field(default_factory=list)
    must_exclude: List[int] = Field(default_factory=list)


class XiOptimizeRequest(BaseModel):
    format: str
    pool_player_ids: List[int] = Field(..., min_length=1)
    opponent_player_ids: List[int] = Field(..., min_length=1)
    team_is_team1: bool = Field(default=True, description="Whether the pool's side bats first")
    constraints: XiConstraints = Field(default_factory=XiConstraints)
    max_evaluations: int = Field(default=20000, ge=100, le=200000)

    @field_validator("format", mode="before")
    def _format_upper(cls, v: str) -> str:
        return _upper(v) or ""


class XiOptimizeResponse(BaseModel):
    selected_player_ids: List[int]
    win_probability: float = Field(..., ge=0, le=1)
    evaluations: int
    improved_over_seed: float
    unknown_player_ids: List[int] = Field(
        default_factory=list, description="Pool ids with no rating history; treated as debutants"
    )
    marginal_values: Dict[int, float] = Field(
        default_factory=dict, description="P(win) lost if the player were replaced by an average one"
    )


class XiWinRequest(BaseModel):
    format: str
    team1_player_ids: List[int] = Field(..., min_length=1)
    team2_player_ids: List[int] = Field(..., min_length=1)
    team1_id: Optional[int] = Field(
        default=None, description="opposition id of the side batting first (for team-level context)"
    )
    team2_id: Optional[int] = None
    venue_id: Optional[int] = None

    @field_validator("format", mode="before")
    def _format_upper(cls, v: str) -> str:
        return _upper(v) or ""


class XiWinResponse(BaseModel):
    team1_win_probability: float = Field(..., ge=0, le=1)
    objective_probability: float = Field(
        ..., ge=0, le=1, description="XI-only model, the value the optimiser maximises"
    )


class XiStatusResponse(BaseModel):
    loaded: bool
    formats: List[str]
    players: int
    ratings_through: Optional[str]
    report: Optional[dict] = None
