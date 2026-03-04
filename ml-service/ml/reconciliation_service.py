"""High-level reconciliation service API.

This module wraps:
- ProblemBuilder (variable + constraint construction),
- solve_reconciliation_problem (continuous QP),
- and a small integer rounding layer,

to produce **integer per-player scorecards** that obey core cricket
constraints (team runs and wickets per innings) while staying close to the
original model outputs.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Mapping

import numpy as np

from app.models import MatchReconciliationInputs
from .reconciliation_core import ProblemBuilder, VariableKind
from .reconciliation_solver import solve_reconciliation_problem


@dataclass
class ReconciledPlayerStats:
    """Integer per-player stats after reconciliation."""

    player_id: int
    team_id: int
    batting_runs: int = 0
    batting_balls: int = 0
    bowling_runs: int = 0
    bowling_balls: int = 0
    wickets: int = 0


def _sum_preserving_round(values: Dict[int, float], target: float) -> Dict[int, int]:
    """Round values to integers while preserving the total sum.

    Strategy:
    - Start from floor(values).
    - Compute how many units we need to add/subtract to reach target_int
      (rounded target).
    - Adjust entries in descending order of fractional part magnitude until
      the total matches.
    """
    if not values:
        return {}

    target_int = int(round(float(target)))
    # Initial integer values and fractional parts
    floors = {k: int(np.floor(v)) for k, v in values.items()}
    fracs = {k: float(v - floors[k]) for k, v in values.items()}

    current_sum = sum(floors.values())
    delta = target_int - current_sum

    result = floors.copy()
    if delta == 0:
        return result

    keys = list(values.keys())

    # For positive delta, add 1 to entries with largest positive frac first.
    # For negative delta, subtract 1 from entries with smallest frac (closest to floor)
    # but keep values non-negative.
    if delta > 0:
        # Sort by descending fractional part
        ordered = sorted(keys, key=lambda k: fracs[k], reverse=True)
        idx = 0
        while delta > 0 and ordered:
            k = ordered[idx % len(ordered)]
            result[k] += 1
            delta -= 1
            idx += 1
    else:  # delta < 0
        # Sort by ascending fractional part so we subtract from those closest to floor
        ordered = sorted(keys, key=lambda k: fracs[k])
        idx = 0
        while delta < 0 and ordered:
            k = ordered[idx % len(ordered)]
            if result[k] > 0:
                result[k] -= 1
                delta += 1
            idx += 1

    return result


def reconcile_match_players(
    pref: MatchReconciliationInputs,
    team1_id: int,
    team2_id: int,
) -> Mapping[int, ReconciledPlayerStats]:
    """Reconcile per-player stats for a two-innings limited-overs match.

    Returns:
        Mapping from player_id to integer stats (runs, balls, wickets) that:
        - Respect innings-level runs and wickets targets (from innings model).
        - Stay close to original model outputs in a weighted least-squares sense.
    """
    builder = ProblemBuilder()
    problem = builder.build_from_match(pref, team1_id=team1_id, team2_id=team2_id)
    x_cont = solve_reconciliation_problem(problem)

    # Initialize per-player stats from continuous solution (will be rounded).
    stats: Dict[int, ReconciledPlayerStats] = {}
    for var, val in zip(problem.variables, x_cont):
        s = stats.get(var.player_id)
        if s is None:
            s = ReconciledPlayerStats(player_id=var.player_id, team_id=var.team_id)
            stats[var.player_id] = s

        v = max(0.0, float(val))
        if var.kind == VariableKind.BAT_RUNS:
            s.batting_runs = int(round(v))
        elif var.kind == VariableKind.BAT_BALLS:
            s.batting_balls = int(round(v))
        elif var.kind == VariableKind.BOWL_RUNS:
            s.bowling_runs = int(round(v))
        elif var.kind == VariableKind.BOWL_BALLS:
            s.bowling_balls = int(round(v))
        elif var.kind == VariableKind.BOWL_WKTS:
            # Cap wickets at 10 per player to avoid pathological rounding.
            s.wickets = min(int(round(v)), 10)

    # Apply sum-preserving adjustments so team totals match innings targets.
    innings_by_number = {i.inning_number: i for i in pref.innings}
    inn1 = innings_by_number.get(1)
    inn2 = innings_by_number.get(2)

    # Helper to adjust one stat type within a team to hit a target.
    def _adjust_team_stat(
        team_id: int,
        get_value,
        set_value,
        target: float,
    ) -> None:
        # Collect current continuous-derived integers for this team
        vals: Dict[int, float] = {}
        for pid, s in stats.items():
            if s.team_id != team_id:
                continue
            vals[pid] = float(get_value(s))
        if not vals:
            return
        rounded = _sum_preserving_round(vals, target)
        for pid, new_val in rounded.items():
            s = stats[pid]
            set_value(s, max(0, new_val))

    # Runs constraints
    if inn1 is not None and inn1.preferred_runs is not None:
        # Team1 batting runs = innings1 runs
        _adjust_team_stat(
            team_id=team1_id,
            get_value=lambda s: s.batting_runs,
            set_value=lambda s, v: setattr(s, "batting_runs", v),
            target=inn1.preferred_runs,
        )
        # Team2 bowling runs conceded = innings1 runs
        _adjust_team_stat(
            team_id=team2_id,
            get_value=lambda s: s.bowling_runs,
            set_value=lambda s, v: setattr(s, "bowling_runs", v),
            target=inn1.preferred_runs,
        )

    if inn2 is not None and inn2.preferred_runs is not None:
        # Team2 batting runs = innings2 runs
        _adjust_team_stat(
            team_id=team2_id,
            get_value=lambda s: s.batting_runs,
            set_value=lambda s, v: setattr(s, "batting_runs", v),
            target=inn2.preferred_runs,
        )
        # Team1 bowling runs conceded = innings2 runs
        _adjust_team_stat(
            team_id=team1_id,
            get_value=lambda s: s.bowling_runs,
            set_value=lambda s, v: setattr(s, "bowling_runs", v),
            target=inn2.preferred_runs,
        )

    # Wickets constraints
    if inn1 is not None and inn1.preferred_wickets is not None:
        _adjust_team_stat(
            team_id=team2_id,
            get_value=lambda s: s.wickets,
            set_value=lambda s, v: setattr(s, "wickets", min(v, 10)),
            target=inn1.preferred_wickets,
        )

    if inn2 is not None and inn2.preferred_wickets is not None:
        _adjust_team_stat(
            team_id=team1_id,
            get_value=lambda s: s.wickets,
            set_value=lambda s, v: setattr(s, "wickets", min(v, 10)),
            target=inn2.preferred_wickets,
        )

    # Balls constraints, when preferred_legal_balls is provided.
    # These ensure that total balls faced by batsmen and balls bowled by bowlers
    # per team/innings line up with the preferred innings-level ball counts.
    if inn1 is not None and inn1.preferred_legal_balls is not None:
        _adjust_team_stat(
            team_id=team1_id,
            get_value=lambda s: s.batting_balls,
            set_value=lambda s, v: setattr(s, "batting_balls", v),
            target=inn1.preferred_legal_balls,
        )
        _adjust_team_stat(
            team_id=team2_id,
            get_value=lambda s: s.bowling_balls,
            set_value=lambda s, v: setattr(s, "bowling_balls", v),
            target=inn1.preferred_legal_balls,
        )

    if inn2 is not None and inn2.preferred_legal_balls is not None:
        _adjust_team_stat(
            team_id=team2_id,
            get_value=lambda s: s.batting_balls,
            set_value=lambda s, v: setattr(s, "batting_balls", v),
            target=inn2.preferred_legal_balls,
        )
        _adjust_team_stat(
            team_id=team1_id,
            get_value=lambda s: s.bowling_balls,
            set_value=lambda s, v: setattr(s, "bowling_balls", v),
            target=inn2.preferred_legal_balls,
        )

    return stats

