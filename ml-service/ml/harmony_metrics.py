"""Harmony and realism metrics for reconciled outputs.

These helpers are intended for offline analysis and dashboards. They operate on
simple 1D samples (e.g. innings totals, wickets per innings, extras per
innings) rather than accessing storage directly.
"""

from __future__ import annotations

from typing import Dict, Iterable, Tuple

import numpy as np


def distribution_summary(x: Iterable[float]) -> Dict[str, float]:
    """Return basic summary stats for a 1D sample."""
    arr = np.asarray(list(x), dtype=float)
    if arr.size == 0:
        return {
            "count": 0.0,
            "mean": 0.0,
            "std": 0.0,
            "p05": 0.0,
            "p50": 0.0,
            "p95": 0.0,
        }
    return {
        "count": float(arr.size),
        "mean": float(arr.mean()),
        "std": float(arr.std(ddof=0)),
        "p05": float(np.percentile(arr, 5)),
        "p50": float(np.percentile(arr, 50)),
        "p95": float(np.percentile(arr, 95)),
    }


def realism_metrics_vs_historical(
    reconciled: Iterable[float],
    historical: Iterable[float],
) -> Dict[str, float]:
    """Compare reconciled distribution to historical reference via simple stats.

    Returns:
        Dict containing per-distribution summaries plus deltas in mean/std and
        median, suitable for dashboards or alerts.
    """
    rec = np.asarray(list(reconciled), dtype=float)
    hist = np.asarray(list(historical), dtype=float)

    rec_sum = distribution_summary(rec)
    hist_sum = distribution_summary(hist)

    return {
        "reconciled_mean": rec_sum["mean"],
        "historical_mean": hist_sum["mean"],
        "delta_mean": rec_sum["mean"] - hist_sum["mean"],
        "reconciled_std": rec_sum["std"],
        "historical_std": hist_sum["std"],
        "delta_std": rec_sum["std"] - hist_sum["std"],
        "reconciled_p50": rec_sum["p50"],
        "historical_p50": hist_sum["p50"],
        "delta_p50": rec_sum["p50"] - hist_sum["p50"],
    }

