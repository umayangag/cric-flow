"""Reconciliation preference models (μ_player, μ_innings, μ_win)."""

from typing import List, Optional

from pydantic import BaseModel, Field

from .predict import BattingPrediction, BowlingPrediction, ExtrasPrediction, WinPrediction


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
