"""Utilities to aggregate ball-by-ball state into per-player scorecards.

This module is the **canonical place** where we define how to derive
per-batsman and per-bowler lines from `InningsState` / `MatchState`.

The rules here must be kept in sync with `docs/match-schema.md` and form
the bridge between the ball-by-ball representation and higher-level models
that work on batting/bowling lines or team aggregates.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Iterable, Mapping, Optional, Tuple

from .match_schema import (
    BallEvent,
    ExtrasKind,
    InningsState,
    MatchState,
    WicketKind,
)


@dataclass
class BattingLine:
    """Per-batsman batting line derived from ball-by-ball events."""

    player_id: int
    runs: int = 0
    balls_faced: int = 0
    fours: int = 0
    sixes: int = 0
    dismissed: bool = False


@dataclass
class BowlingLine:
    """Per-bowler bowling line derived from ball-by-ball events."""

    player_id: int
    runs_conceded: int = 0
    legal_balls: int = 0
    wickets: int = 0

    @property
    def overs_and_balls(self) -> Tuple[int, int]:
        """Return (completed_overs, balls_in_current_over) for this bowler."""
        return divmod(self.legal_balls, 6)


def _get_or_create_batting_line(
    lines: Dict[int, BattingLine], player_id: int
) -> BattingLine:
    line = lines.get(player_id)
    if line is None:
        line = BattingLine(player_id=player_id)
        lines[player_id] = line
    return line


def _get_or_create_bowling_line(
    lines: Dict[int, BowlingLine], player_id: int
) -> BowlingLine:
    line = lines.get(player_id)
    if line is None:
        line = BowlingLine(player_id=player_id)
        lines[player_id] = line
    return line


def build_batting_scorecard(innings: InningsState) -> Dict[int, BattingLine]:
    """Aggregate per-batsman batting lines from an innings.

    Rules (aligned with docs/match-schema.md):
    - A batsman faces a ball when `is_legal = True` and he is the striker.
    - Runs attributed to the batsman come from `runs_batter` for the striker.
    - Fours/sixes are inferred from `runs_batter == 4` / `runs_batter == 6`
      when there are no extras on the ball. This is a pragmatic heuristic;
      historical data may encode boundaries differently, but boundaries are
      not used in the hard reconciliation constraints.
    - A batsman is marked dismissed when `player_out_id` equals their ID.
    """
    lines: Dict[int, BattingLine] = {}

    for b in innings.balls:
        # Striker batting line
        striker = _get_or_create_batting_line(lines, b.striker_id)

        # Balls faced: only legal balls count as balls faced in this schema.
        if b.is_legal:
            striker.balls_faced += 1

        # Runs to batter
        if b.runs_batter > 0:
            striker.runs += b.runs_batter
            # Heuristic boundaries: only when extras are zero.
            if b.runs_batter == 4 and b.runs_extras == 0:
                striker.fours += 1
            elif b.runs_batter == 6 and b.runs_extras == 0:
                striker.sixes += 1

        # Dismissal can affect either striker or non-striker (e.g. run out)
        if b.player_out_id is not None:
            out_id = b.player_out_id
            out_line = _get_or_create_batting_line(lines, out_id)
            out_line.dismissed = True

    return lines


def _bowler_runs_conceded_from_ball(b: BallEvent) -> int:
    """Compute runs conceded by the bowler on a single ball.

    Convention (see docs/match-schema.md §2.2):
    - Wides and no-balls are charged to the bowler.
    - Byes, leg-byes, and penalty runs are *not* charged to the bowler.
    - Batter runs always count against the bowler.
    """
    extras_kind = b.extras_kind_enum()
    # Extras not charged to bowler
    if extras_kind in {ExtrasKind.BYE, ExtrasKind.LEG_BYE, ExtrasKind.PENALTY}:
        return b.runs_batter
    # All other extras (including wides, no-balls) count to bowler
    return b.runs_total


def _wicket_counts_for_bowler(b: BallEvent) -> int:
    """Return 1 if this ball credits a wicket to the bowler, else 0.

    Standard convention:
    - Bowler gets credit for: bowled, caught, lbw, stumped, hit wicket, and
      most "other" dismissals.
    - Bowler does *not* get credit for run outs or retirements.
    """
    kind = b.wicket_kind_enum()
    if kind is None:
        return 0
    if kind in {WicketKind.RUN_OUT, WicketKind.RETIRED}:
        return 0
    return 1


def build_bowling_scorecard(innings: InningsState) -> Dict[int, BowlingLine]:
    """Aggregate per-bowler bowling lines from an innings.

    Rules:
    - Legal deliveries: count balls for which `is_legal = True`.
    - Runs conceded: sum of `_bowler_runs_conceded_from_ball`.
    - Wickets: sum of `_wicket_counts_for_bowler`.
    """
    lines: Dict[int, BowlingLine] = {}

    for b in innings.balls:
        line = _get_or_create_bowling_line(lines, b.bowler_id)

        if b.is_legal:
            line.legal_balls += 1

        line.runs_conceded += _bowler_runs_conceded_from_ball(b)
        line.wickets += _wicket_counts_for_bowler(b)

    return lines


def aggregate_match_batting(
    match: MatchState,
) -> Mapping[int, BattingLine]:
    """Aggregate batting lines across all innings in a match by player_id."""
    agg: Dict[int, BattingLine] = {}
    for inn in match.innings_list:
        inn_lines = build_batting_scorecard(inn)
        for pid, line in inn_lines.items():
            total = agg.get(pid)
            if total is None:
                agg[pid] = BattingLine(
                    player_id=pid,
                    runs=line.runs,
                    balls_faced=line.balls_faced,
                    fours=line.fours,
                    sixes=line.sixes,
                    dismissed=line.dismissed,
                )
            else:
                total.runs += line.runs
                total.balls_faced += line.balls_faced
                total.fours += line.fours
                total.sixes += line.sixes
                # If dismissed in any innings, mark as dismissed overall
                total.dismissed = total.dismissed or line.dismissed
    return agg


def aggregate_match_bowling(
    match: MatchState,
) -> Mapping[int, BowlingLine]:
    """Aggregate bowling lines across all innings in a match by player_id."""
    agg: Dict[int, BowlingLine] = {}
    for inn in match.innings_list:
        inn_lines = build_bowling_scorecard(inn)
        for pid, line in inn_lines.items():
            total = agg.get(pid)
            if total is None:
                agg[pid] = BowlingLine(
                    player_id=pid,
                    runs_conceded=line.runs_conceded,
                    legal_balls=line.legal_balls,
                    wickets=line.wickets,
                )
            else:
                total.runs_conceded += line.runs_conceded
                total.legal_balls += line.legal_balls
                total.wickets += line.wickets
    return agg

