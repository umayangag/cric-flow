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

from .config import get_reconciliation_config
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
    - For positive deltas, increment entries with largest fractional parts.
    - For negative deltas, decrement entries with largest values first,
      without driving any entry below zero.
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

    if delta > 0:
        # For positive delta, add 1 to entries with largest positive frac first.
        # Sort by descending fractional part
        ordered = sorted(keys, key=lambda k: fracs[k], reverse=True)
        idx = 0
        while delta > 0 and ordered:
            k = ordered[idx % len(ordered)]
            result[k] += 1
            delta -= 1
            idx += 1
    else:
        # For negative delta, subtract 1 from entries with largest current value
        # first, while avoiding negative counts. This keeps the total close to
        # the original distribution even when the target is below the current sum.
        ordered = sorted(keys, key=lambda k: floors[k], reverse=True)
        while delta < 0 and ordered:
            changed = False
            for k in ordered:
                if delta == 0:
                    break
                if result[k] <= 0:
                    continue
                result[k] -= 1
                delta += 1
                changed = True
            if not changed:
                # All entries are already zero; cannot reduce further.
                break

    return result


def _win_conditioned_innings_targets(
    pref: MatchReconciliationInputs,
    win_probability: float,
) -> tuple[float | None, float | None]:
    """Derive innings run targets from win probability and existing preferred totals.

    When both innings have preferred runs and a win probability is provided,
    the targets are adjusted so that the margin is consistent with the win
    probability. A win_probability > 0.5 means team1 (batting inn 1) is favored,
    so inn1 runs should be higher than inn2 runs.

    Returns (inn1_target_runs, inn2_target_runs).
    """
    innings_by_number = {i.inning_number: i for i in pref.innings}
    inn1 = innings_by_number.get(1)
    inn2 = innings_by_number.get(2)
    if inn1 is None or inn2 is None:
        return None, None
    if inn1.preferred_runs is None or inn2.preferred_runs is None:
        return inn1.preferred_runs, inn2.preferred_runs

    total = inn1.preferred_runs + inn2.preferred_runs
    if total <= 0:
        return inn1.preferred_runs, inn2.preferred_runs

    # Convert win probability to expected margin: ranges from -total/2 to +total/2.
    # At p=0.5 -> margin=0, at p=1.0 -> margin=total/2 (team1 wins by half total).
    # Uses a simple linear mapping, clipped to keep both totals non-negative.
    margin_fraction = (win_probability - 0.5) * 2.0
    max_margin = total * 0.4
    implied_margin = margin_fraction * max_margin
    inn1_target = max(0.0, (total + implied_margin) / 2.0)
    inn2_target = max(0.0, (total - implied_margin) / 2.0)

    return inn1_target, inn2_target


def reconcile_match_players(
    pref: MatchReconciliationInputs,
    team1_id: int,
    team2_id: int,
    win_probability: float | None = None,
) -> Mapping[int, ReconciledPlayerStats]:
    """Reconcile per-player stats for a two-innings limited-overs match.

    When ``win_probability`` is provided, innings run targets are adjusted to
    be consistent with the predicted outcome: a higher team1 win probability
    shifts runs toward innings 1 (team1 batting) and away from innings 2.

    Returns:
        Mapping from player_id to integer stats (runs, balls, wickets) that:
        - Respect innings-level runs and wickets targets (from innings model).
        - Stay close to original model outputs in a weighted least-squares sense.
    """
    if win_probability is None and pref.win is not None:
        win_probability = pref.win.team1_win_probability

    if win_probability is not None:
        inn1_target, inn2_target = _win_conditioned_innings_targets(pref, win_probability)
        if inn1_target is not None and inn2_target is not None:
            updated_innings = []
            for inn in pref.innings:
                inn_copy = inn.model_copy()
                if inn_copy.inning_number == 1:
                    inn_copy.preferred_runs = inn1_target
                elif inn_copy.inning_number == 2:
                    inn_copy.preferred_runs = inn2_target
                updated_innings.append(inn_copy)
            pref = pref.model_copy(update={"innings": updated_innings})

    cfg = get_reconciliation_config()
    builder = ProblemBuilder(
        runs_weight=cfg.get("runs_weight", 1.0),
        wickets_weight=cfg.get("wickets_weight", 1.0),
        soft_favor_top_order_balls=cfg.get("soft_favor_top_order_balls", 0.0),
    )
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
