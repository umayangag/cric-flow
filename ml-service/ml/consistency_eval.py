"""Helpers for evaluating reconciliation consistency on validation predictions.

This module provides a small, focused API for Stage 2 (consistency‑aware
training) that can be reused by:

- Offline validation / backtest scripts that already emit BacktestPlayerPred,
- Tuning pipelines that want a scalar consistency penalty per candidate model,
- Future TrainingPipeline hooks that compute reconciliation metrics on a
  held‑out set.

It intentionally mirrors the reconciliation path used at inference time
(`ml.reconciliation_adapter`) but returns *before/after* stats and a compact
metrics dict instead of adjusted predictions.
"""

from __future__ import annotations

from typing import Dict, Iterable, Mapping, Optional, Tuple

from app.models import BacktestPlayerPred

from .consistency_checker import ReconciledPlayerStats, adjustment_magnitude
from .reconciliation_adapter import (
    TEAM1_ID,
    TEAM2_ID,
    build_match_reconciliation_inputs_from_backtest_preds,
)
from .reconciliation_service import reconcile_match_players


def build_before_after_stats_from_predictions(
    preds: Iterable[BacktestPlayerPred],
    team1_ids: Iterable[int],
    team2_ids: Iterable[int],
    inn1_runs: float,
    inn1_wkts: float,
    inn2_runs: float,
    inn2_wkts: float,
    match_id: int = 0,
    format_code: Optional[str] = None,
    default_economy: float = 6.0,
) -> Tuple[Mapping[int, ReconciledPlayerStats], Mapping[int, ReconciledPlayerStats]]:
    """Build before/after ReconciledPlayerStats mappings from raw predictions.

    This is the validation‑time analogue of the hybrid reconciliation path used
    in app.prediction_service:

    - Takes unconstrained per‑player predictions (BacktestPlayerPred‑like),
    - Builds MatchReconciliationInputs via the adapter,
    - Runs constraint‑based reconciliation to obtain integer stats,
    - Constructs a pair of mappings {player_id -> ReconciledPlayerStats}:
      *before* (from raw predictions) and *after* (from reconciliation).

    Args:
        preds: Iterable of per‑player predictions for a single match.
        team1_ids: Player ids belonging to team1 (batting first).
        team2_ids: Player ids belonging to team2 (chasing).
        inn1_runs: Preferred innings1 runs (from innings model or labels).
        inn1_wkts: Preferred innings1 wickets.
        inn2_runs: Preferred innings2 runs.
        inn2_wkts: Preferred innings2 wickets.
        match_id: Optional canonical match id (for logging/traceability only).
        format_code: Optional format code (e.g. "T20", "ODI") for defaults.
        default_economy: Fallback economy rate when predictions do not contain
            an explicit economy value but bowling balls are non‑zero.

    Returns:
        Tuple (before_stats, after_stats) where both are mappings keyed by
        player_id.
    """
    preds_list = list(preds)
    if not preds_list:
        return {}, {}

    pref = build_match_reconciliation_inputs_from_backtest_preds(
        preds_list,
        team1_ids=team1_ids,
        team2_ids=team2_ids,
        inn1_runs=inn1_runs,
        inn1_wkts=inn1_wkts,
        inn2_runs=inn2_runs,
        inn2_wkts=inn2_wkts,
        match_id=match_id,
        format_code=format_code,
    )

    reconciled = reconcile_match_players(pref, team1_id=TEAM1_ID, team2_id=TEAM2_ID)
    pid_to_reconciled: Dict[int, ReconciledPlayerStats] = {s.player_id: s for s in reconciled.values()}

    before_stats: Dict[int, ReconciledPlayerStats] = {}
    after_stats: Dict[int, ReconciledPlayerStats] = {}

    for p in preds_list:
        r = pid_to_reconciled.get(p.player_id)
        if r is None:
            # No reconciled stats for this player; skip from metrics to avoid
            # introducing inconsistencies in totals.
            continue

        bowling_balls = int(r.bowling_balls or 0)
        if bowling_balls > 0:
            econ = float(p.economy) if p.economy is not None else float(default_economy)
            bowling_runs = int(econ * (bowling_balls / 6.0))
        else:
            bowling_runs = 0

        before_stats[p.player_id] = ReconciledPlayerStats(
            player_id=p.player_id,
            team_id=r.team_id,
            batting_runs=int(p.runs or 0),
            batting_balls=int(p.balls or 0) if p.balls is not None else 0,
            bowling_runs=bowling_runs,
            bowling_balls=bowling_balls,
            wickets=int(p.wickets or 0) if p.wickets is not None else 0,
        )
        after_stats[p.player_id] = r

    return before_stats, after_stats


def compute_consistency_metrics(
    before_stats: Mapping[int, ReconciledPlayerStats],
    after_stats: Mapping[int, ReconciledPlayerStats],
) -> Dict[str, float]:
    """Compute summary reconciliation metrics from before/after stats.

    This is a thin wrapper around adjustment_magnitude that keeps the public
    API for Stage 2 metrics in one place. It can be used directly from:

    - Training or tuning scripts that already have before/after mappings, or
    - Higher‑level helpers that first call build_before_after_stats_from_predictions.
    """
    return adjustment_magnitude(before_stats, after_stats)
