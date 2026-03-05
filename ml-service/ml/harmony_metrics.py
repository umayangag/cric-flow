"""Harmony and realism metrics for reconciled outputs.

These helpers are intended for offline analysis and dashboards. They operate on
simple 1D samples (e.g. innings totals, wickets per innings, extras per
innings) rather than accessing storage directly.
"""

from __future__ import annotations

from typing import Any, Dict, Iterable, Tuple

import numpy as np

# Allowed bands for realism metrics (§3.2.2): delta_mean, delta_std, delta_p50
# relative to historical. Reconciled outputs outside these bands may trigger alerts.
# Keys: stat label; values: (abs delta_mean max, abs delta_std max, abs delta_p50 max).
REALISM_ALLOWED_BANDS: Dict[str, Tuple[float, float, float]] = {
    "runs_per_innings": (25.0, 15.0, 20.0),
    "wickets_per_innings": (1.5, 1.2, 1.5),
    "extras_per_innings": (5.0, 4.0, 5.0),
    "runs_per_wicket": (10.0, 8.0, 10.0),
}
DEFAULT_REALISM_BAND: Tuple[float, float, float] = (50.0, 30.0, 40.0)


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


def realism_within_allowed_bands(
    metrics: Dict[str, float],
    stat_label: str = "runs_per_innings",
) -> Dict[str, Any]:
    """Check whether realism metrics fall within allowed bands for the given stat.

    Args:
        metrics: Dict from realism_metrics_vs_historical (delta_mean, delta_std, delta_p50).
        stat_label: Key into REALISM_ALLOWED_BANDS (e.g. runs_per_innings, wickets_per_innings).

    Returns:
        Dict with within_band (bool), delta_mean_ok, delta_std_ok, delta_p50_ok, and band_used.
    """
    band = REALISM_ALLOWED_BANDS.get(stat_label, DEFAULT_REALISM_BAND)
    max_dm, max_ds, max_dp = band
    dm = abs(metrics.get("delta_mean", 0.0))
    ds = abs(metrics.get("delta_std", 0.0))
    dp = abs(metrics.get("delta_p50", 0.0))
    return {
        "within_band": dm <= max_dm and ds <= max_ds and dp <= max_dp,
        "delta_mean_ok": dm <= max_dm,
        "delta_std_ok": ds <= max_ds,
        "delta_p50_ok": dp <= max_dp,
        "band_used": list(band),
        "stat_label": stat_label,
    }
