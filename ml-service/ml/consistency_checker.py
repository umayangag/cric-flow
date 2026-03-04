"""Automated consistency checker for reconciled scorecards.

Re-evaluates cricket accounting constraints (docs/match-schema.md §2) on
aggregated scorecard data and returns a list of violation messages. Used
pre- and post-reconciliation to flag violations and to log adjustment
magnitude.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, List, Mapping, Optional

from .reconciliation_service import ReconciledPlayerStats


@dataclass
class InningsTargets:
    """Target totals for one innings (batting team perspective)."""

    batting_team_id: int
    bowling_team_id: int
    runs: float
    wickets: float
    legal_balls: Optional[float] = None


def check_reconciled_scorecard_consistency(
    stats: Mapping[int, ReconciledPlayerStats],
    team1_id: int,
    team2_id: int,
    inn1: Optional[InningsTargets] = None,
    inn2: Optional[InningsTargets] = None,
) -> List[str]:
    """Check that reconciled player stats satisfy innings-level accounting rules.

    Rules checked (per docs/match-schema.md):
    - Team batting runs sum = innings runs target.
    - Team bowling runs conceded sum = innings runs target.
    - Team bowling wickets sum = innings wickets target.
    - If legal_balls is set: batting balls (batting team) = bowling balls (bowling team) = legal_balls.
    - All stats non-negative; wickets per player in [0, 10].

    Returns:
        List of violation messages (empty if consistent).
    """
    violations: List[str] = []

    def team_stats(tid: int):
        return [s for s in stats.values() if s.team_id == tid]

    # Per-player bounds
    for pid, s in stats.items():
        if s.batting_runs < 0:
            violations.append(f"player_id={pid}: batting_runs={s.batting_runs} < 0")
        if s.batting_balls < 0:
            violations.append(f"player_id={pid}: batting_balls={s.batting_balls} < 0")
        if s.bowling_runs < 0:
            violations.append(f"player_id={pid}: bowling_runs={s.bowling_runs} < 0")
        if s.bowling_balls < 0:
            violations.append(f"player_id={pid}: bowling_balls={s.bowling_balls} < 0")
        if not (0 <= s.wickets <= 10):
            violations.append(f"player_id={pid}: wickets={s.wickets} not in [0, 10]")

    if inn1 is not None:
        t1 = team_stats(team1_id)
        t2 = team_stats(team2_id)
        bat_runs_1 = sum(s.batting_runs for s in t1)
        bowl_runs_2 = sum(s.bowling_runs for s in t2)
        wkts_2 = sum(s.wickets for s in t2)
        target_runs = int(round(inn1.runs))
        target_wkts = int(round(inn1.wickets))
        if bat_runs_1 != target_runs:
            violations.append(
                f"innings1: sum(batting_runs team1)={bat_runs_1} != target_runs={target_runs}"
            )
        if bowl_runs_2 != target_runs:
            violations.append(
                f"innings1: sum(bowling_runs team2)={bowl_runs_2} != target_runs={target_runs}"
            )
        if wkts_2 != target_wkts:
            violations.append(
                f"innings1: sum(wickets team2)={wkts_2} != target_wickets={target_wkts}"
            )
        if inn1.legal_balls is not None:
            balls_bat_1 = sum(s.batting_balls for s in t1)
            balls_bowl_2 = sum(s.bowling_balls for s in t2)
            target_balls = int(round(inn1.legal_balls))
            if balls_bat_1 != target_balls:
                violations.append(
                    f"innings1: sum(batting_balls team1)={balls_bat_1} != legal_balls={target_balls}"
                )
            if balls_bowl_2 != target_balls:
                violations.append(
                    f"innings1: sum(bowling_balls team2)={balls_bowl_2} != legal_balls={target_balls}"
                )

    if inn2 is not None:
        t1 = team_stats(team1_id)
        t2 = team_stats(team2_id)
        bat_runs_2 = sum(s.batting_runs for s in t2)
        bowl_runs_1 = sum(s.bowling_runs for s in t1)
        wkts_1 = sum(s.wickets for s in t1)
        target_runs = int(round(inn2.runs))
        target_wkts = int(round(inn2.wickets))
        if bat_runs_2 != target_runs:
            violations.append(
                f"innings2: sum(batting_runs team2)={bat_runs_2} != target_runs={target_runs}"
            )
        if bowl_runs_1 != target_runs:
            violations.append(
                f"innings2: sum(bowling_runs team1)={bowl_runs_1} != target_runs={target_runs}"
            )
        if wkts_1 != target_wkts:
            violations.append(
                f"innings2: sum(wickets team1)={wkts_1} != target_wickets={target_wkts}"
            )
        if inn2.legal_balls is not None:
            balls_bat_2 = sum(s.batting_balls for s in t2)
            balls_bowl_1 = sum(s.bowling_balls for s in t1)
            target_balls = int(round(inn2.legal_balls))
            if balls_bat_2 != target_balls:
                violations.append(
                    f"innings2: sum(batting_balls team2)={balls_bat_2} != legal_balls={target_balls}"
                )
            if balls_bowl_1 != target_balls:
                violations.append(
                    f"innings2: sum(bowling_balls team1)={balls_bowl_1} != legal_balls={target_balls}"
                )

    return violations


def adjustment_magnitude(
    before: Mapping[int, ReconciledPlayerStats],
    after: Mapping[int, ReconciledPlayerStats],
) -> Dict[str, float]:
    """Compute summary metrics for reconciliation adjustment magnitude.

    Returns dict with mean_abs_delta_runs, mean_abs_delta_wickets,
    mean_abs_pct_delta_runs (percentage of pre-run total), and
    mean_abs_pct_delta_wickets.
    """
    deltas_runs: List[float] = []
    deltas_wkts: List[float] = []
    pct_runs: List[float] = []
    pct_wkts: List[float] = []
    for pid, a in after.items():
        b = before.get(pid)
        if b is None:
            continue
        dr = abs(a.batting_runs - b.batting_runs)
        dw = abs(a.wickets - b.wickets)
        deltas_runs.append(float(dr))
        deltas_wkts.append(float(dw))
        if b.batting_runs > 0:
            pct_runs.append(dr / b.batting_runs * 100.0)
        if b.wickets > 0:
            pct_wkts.append(dw / b.wickets * 100.0)
    total_before_runs = sum(s.batting_runs for s in before.values())
    total_before_wkts = sum(s.wickets for s in before.values())
    return {
        "mean_abs_delta_runs": sum(deltas_runs) / len(deltas_runs) if deltas_runs else 0.0,
        "mean_abs_delta_wickets": sum(deltas_wkts) / len(deltas_wkts) if deltas_wkts else 0.0,
        "mean_abs_pct_delta_runs": sum(pct_runs) / len(pct_runs) if pct_runs else 0.0,
        "mean_abs_pct_delta_wickets": sum(pct_wkts) / len(pct_wkts) if pct_wkts else 0.0,
        "total_before_runs": total_before_runs,
        "total_before_wickets": total_before_wkts,
    }
