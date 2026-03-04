"""Consistency-aware loss helpers for Stage 2 training.

These functions are designed to be used from offline training scripts. They do
not change the online inference path.

The idea is to:
- Train base models as usual.
- Run reconciliation on a validation set to obtain "after" stats.
- Use these helpers to compute regularization penalties that quantify how much
  reconciliation has to move the raw model outputs.
"""

from __future__ import annotations

from typing import Dict, Mapping

from .consistency_checker import ReconciledPlayerStats, adjustment_magnitude


def consistency_penalty_from_metrics(
    metrics: Dict[str, float],
    weight_runs: float = 1.0,
    weight_wickets: float = 1.0,
) -> float:
    """Compute scalar consistency penalty from adjustment_magnitude-style metrics.

    Use this when you already have a metrics dict (e.g. from
    consistency_eval.compute_consistency_metrics or from logs) and want a single
    penalty for tuning/ranking. Same formula as consistency_regularization_loss.
    """
    pct_runs = float(metrics.get("mean_abs_pct_delta_runs", 0.0))
    pct_wkts = float(metrics.get("mean_abs_pct_delta_wickets", 0.0))
    return weight_runs * pct_runs + weight_wickets * pct_wkts


def consistency_regularization_loss(
    before: Mapping[int, ReconciledPlayerStats],
    after: Mapping[int, ReconciledPlayerStats],
    weight_runs: float = 1.0,
    weight_wickets: float = 1.0,
) -> float:
    """Compute a scalar consistency penalty from before/after stats.

    The penalty is a weighted combination of mean absolute percentage deltas
    for runs and wickets. It is intended to be *added* to existing training
    losses (possibly with a small weight) or used as a metric to select among
    candidate models in tuning.
    """
    metrics = adjustment_magnitude(before, after)
    pct_runs = metrics.get("mean_abs_pct_delta_runs", 0.0)
    pct_wkts = metrics.get("mean_abs_pct_delta_wickets", 0.0)
    return float(weight_runs * pct_runs + weight_wickets * pct_wkts)

