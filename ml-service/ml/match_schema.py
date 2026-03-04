"""Canonical match representation for reconciliation and simulation.

This module defines the internal, cricket-aware schema that all higher-level
models (batting, bowling, extras, win, innings) should ultimately agree on.

It is intentionally close to the ball-by-ball export schema used by
`ml.ball_by_ball_loader` so that:

- Training pipelines can construct `MatchState` / `InningsState` instances
  directly from exported tables or DB queries, and
- Prediction / simulation pipelines can generate new `MatchState` objects and
  derive per-player and per-innings aggregates from them.

Key design choices:
- **Primary resolution:** ball-by-ball (`BallEvent`).
- **Canonical aggregates:** `InningsState` and `MatchState`, with helpers that
  compute team totals, wickets, balls, and extras in a single place.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Dict, List, Optional, Tuple


class ExtrasKind(str, Enum):
    """Subset of extras kinds we care about explicitly.

    The underlying DB / export may use a richer set of values; we represent
    them as strings and normalize into this enum where needed.
    """

    WIDE = "wide"
    NO_BALL = "no_ball"
    BYE = "bye"
    LEG_BYE = "leg_bye"
    PENALTY = "penalty"


class WicketKind(str, Enum):
    """High-level dismissal types (not exhaustive, but enough for accounting)."""

    BOWLED = "bowled"
    CAUGHT = "caught"
    RUN_OUT = "run_out"
    LBW = "lbw"
    STUMPED = "stumped"
    HIT_WICKET = "hit_wicket"
    RETIRED = "retired"
    OTHER = "other"


@dataclass(frozen=True)
class BallEvent:
    """One ball (legal or not) within an innings.

    Field names are aligned with `BALL_COLS` in `ml.ball_by_ball_loader`.
    """

    match_id: int
    innings: int
    over: int
    ball: int
    ball_seq: int
    is_legal: bool
    phase: str
    striker_id: int
    non_striker_id: int
    bowler_id: int
    runs_batter: int
    runs_extras: int
    runs_total: int
    extras_kind: Optional[str] = None
    wicket_kind: Optional[str] = None
    player_out_id: Optional[int] = None

    def extras_kind_enum(self) -> Optional[ExtrasKind]:
        """Best-effort map of raw extras_kind string into ExtrasKind enum."""
        if self.extras_kind is None:
            return None
        key = self.extras_kind.strip().lower()
        if key in {"w", "wide"}:
            return ExtrasKind.WIDE
        if key in {"nb", "no_ball", "no-ball"}:
            return ExtrasKind.NO_BALL
        if key in {"b", "bye"}:
            return ExtrasKind.BYE
        if key in {"lb", "leg_bye", "leg-bye"}:
            return ExtrasKind.LEG_BYE
        if key in {"pen", "penalty"}:
            return ExtrasKind.PENALTY
        return None

    def wicket_kind_enum(self) -> Optional[WicketKind]:
        """Best-effort map of raw wicket_kind string into WicketKind enum."""
        if self.wicket_kind is None:
            return None
        key = self.wicket_kind.strip().lower()
        if key in {"bowled"}:
            return WicketKind.BOWLED
        if key in {"caught", "caught_and_bowled"}:
            return WicketKind.CAUGHT
        if key in {"run out", "run_out"}:
            return WicketKind.RUN_OUT
        if key in {"lbw"}:
            return WicketKind.LBW
        if key in {"stumped"}:
            return WicketKind.STUMPED
        if key in {"hit wicket", "hit_wicket"}:
            return WicketKind.HIT_WICKET
        if key in {"retired", "retired_hurt"}:
            return WicketKind.RETIRED
        return WicketKind.OTHER


@dataclass
class InningsState:
    """Canonical representation of one innings.

    This aggregates `BallEvent`s and exposes derived properties that enforce
    cricket accounting identities:

    - total_runs = sum(runs_total)
    - total_wickets = number of dismissals (capped at 10 for team total)
    - legal_balls = number of balls with is_legal=True
    - extras_total = sum(runs_extras)
    """

    match_id: int
    innings_number: int
    batting_team_id: Optional[int] = None
    bowling_team_id: Optional[int] = None
    target_runs: Optional[int] = None
    balls_per_innings: Optional[int] = None
    balls: List[BallEvent] = field(default_factory=list)

    # -------------------- Derived accounting properties --------------------

    @property
    def total_runs(self) -> int:
        return sum(b.runs_total for b in self.balls)

    @property
    def total_wickets(self) -> int:
        # Team-level wickets are capped at 10; individual events may exceed when
        # data is noisy, but downstream reconciliation should respect this cap.
        wickets = sum(1 for b in self.balls if b.player_out_id is not None)
        return min(wickets, 10)

    @property
    def legal_balls(self) -> int:
        return sum(1 for b in self.balls if b.is_legal)

    @property
    def overs_bowled(self) -> Tuple[int, int]:
        """Return (completed_overs, balls_in_current_over) from legal balls."""
        legal = self.legal_balls
        return divmod(legal, 6)

    @property
    def extras_total(self) -> int:
        return sum(b.runs_extras for b in self.balls)

    @property
    def extras_breakdown(self) -> Dict[ExtrasKind, int]:
        """Aggregate extras by kind over this innings."""
        totals: Dict[ExtrasKind, int] = {}
        for b in self.balls:
            kind = b.extras_kind_enum()
            if kind is None:
                continue
            totals[kind] = totals.get(kind, 0) + b.runs_extras
        return totals

    @property
    def is_all_out(self) -> bool:
        return self.total_wickets >= 10

    @property
    def innings_completed(self) -> bool:
        """True if innings ended by all-out or by using all balls_per_innings."""
        if self.balls_per_innings is None:
            return self.is_all_out
        return self.is_all_out or self.legal_balls >= self.balls_per_innings


@dataclass
class MatchState:
    """Canonical state for a full match (one or two innings, depending on format)."""

    match_id: int
    format_code: Optional[str] = None
    venue_id: Optional[int] = None
    season_id: Optional[int] = None
    outcome_winner_team_id: Optional[int] = None
    innings_list: List[InningsState] = field(default_factory=list)

    @property
    def innings_by_number(self) -> Dict[int, InningsState]:
        return {inn.innings_number: inn for inn in self.innings_list}

    @property
    def team_totals(self) -> Dict[int, int]:
        """Return mapping from batting_team_id to total runs for that innings.

        For multi-innings formats (TEST), this is totals per innings, not per match.
        """
        totals: Dict[int, int] = {}
        for inn in self.innings_list:
            if inn.batting_team_id is None:
                continue
            totals[inn.batting_team_id] = totals.get(inn.batting_team_id, 0) + inn.total_runs
        return totals

    @property
    def team_wickets(self) -> Dict[int, int]:
        """Return mapping from batting_team_id to wickets fallen across innings."""
        wkts: Dict[int, int] = {}
        for inn in self.innings_list:
            if inn.batting_team_id is None:
                continue
            wkts[inn.batting_team_id] = wkts.get(inn.batting_team_id, 0) + inn.total_wickets
        return wkts

    @property
    def is_two_innings_limited_overs(self) -> bool:
        """Heuristic: True for formats like ODI/T20 where we expect exactly 2 innings."""
        if self.format_code is None:
            return False
        return self.format_code.upper() in {"ODI", "T20", "T20I", "ODM", "MDM"}

