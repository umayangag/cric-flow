"""Regression metrics shared by the tuning search and the training pipeline.

Both callers need the same numbers from the same ``(y_true, y_pred)`` pair — the tuning
search gets its predictions from ``cross_val_predict`` over the whole dataset, the
training pipeline from a trailing holdout — so the arithmetic lives here instead of
being duplicated on either side. Whoever produces the predictions decides what the
numbers *mean*; this module only decides how they are computed and rounded, which is
what makes a tuned score and a holdout score comparable in shape if not in provenance.
"""

from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional

import numpy as np
from sklearn.metrics import (
    explained_variance_score,
    max_error,
    mean_absolute_error,
    mean_squared_error,
    median_absolute_error,
    r2_score,
)

logger = logging.getLogger(__name__)


def compute_regression_metrics(
    y_true: Any,
    y_pred: Any,
    target_names: Optional[List[str]] = None,
) -> Dict[str, Any]:
    """Score predictions against observed targets.

    Returns dict with mae, rmse, r2, r2_pct, median_ae, max_error, explained_variance,
    target_context (mean, std, min, max for MAE interpretation), baseline comparison
    (naive MAE, improvement %), and per_target_mae when target_names is provided for
    multi-output models.
    - mae: mean absolute error (interpretable units)
    - baseline_improvement_pct: top-level for UI (how much better than naive)
    - mae_pct_of_mean: top-level for UI (MAE as % of target mean)
    - per_target_mae: per-target MAE for multi-output (e.g. bowling runs, balls, wickets)

    Returns {} when the arrays cannot be scored: a missing score is reported by
    omission, never as a zero that would read as a real measurement.
    """
    try:
        y_arr = np.asarray(y_true)
        y_flat = y_arr.ravel()
        y_pred_arr = np.asarray(y_pred)
        y_pred_flat = y_pred_arr.ravel()
        mae = float(mean_absolute_error(y_flat, y_pred_flat))
        rmse = float(np.sqrt(mean_squared_error(y_flat, y_pred_flat)))
        r2 = float(r2_score(y_flat, y_pred_flat))
        # r2 can be negative; clamp for display
        r2_pct = max(0.0, min(100.0, r2 * 100))
        median_ae = float(median_absolute_error(y_flat, y_pred_flat))
        worst_err = float(max_error(y_flat, y_pred_flat))
        expl_var = float(explained_variance_score(y_flat, y_pred_flat))

        # Target context: MAE vs target scale (e.g. extras mean 10–12, MAE 6 = ~50% error)
        target_mean = float(np.mean(y_flat))
        target_std = float(np.std(y_flat)) if len(y_flat) > 1 else 0.0
        target_min = float(np.min(y_flat))
        target_max = float(np.max(y_flat))
        mae_pct_of_mean = round((mae / target_mean * 100), 2) if target_mean != 0 else None

        # Baseline comparison: naive model predicts mean every time
        naive_pred = np.full_like(y_flat, target_mean)
        baseline_mae = float(mean_absolute_error(y_flat, naive_pred))
        baseline_improvement_pct = round((baseline_mae - mae) / baseline_mae * 100, 2) if baseline_mae > 0 else 0.0

        out: Dict[str, Any] = {
            "mae": round(mae, 4),
            "rmse": round(rmse, 4),
            "r2": round(r2, 4),
            "r2_pct": round(r2_pct, 2),
            "median_ae": round(median_ae, 4),
            "max_error": round(worst_err, 4),
            "explained_variance": round(expl_var, 4),
            # Top-level for UI: key tuning/eval metrics
            "baseline_improvement_pct": baseline_improvement_pct,
            "mae_pct_of_mean": mae_pct_of_mean,
            "target_mean": round(target_mean, 4),
            "target_std": round(target_std, 4),
            "target_context": {
                "target_mean": round(target_mean, 4),
                "target_std": round(target_std, 4),
                "target_min": round(target_min, 4),
                "target_max": round(target_max, 4),
                "mae_pct_of_mean": mae_pct_of_mean,
            },
            "baseline_comparison": {
                "baseline_mae": round(baseline_mae, 4),
                "baseline_improvement_pct": baseline_improvement_pct,
            },
        }

        # Per-target MAE for multi-output (bowling: runs, balls, wickets; batting: runs, balls, etc.)
        if target_names and y_arr.ndim == 2 and y_arr.shape[1] > 1:
            n_targets = y_arr.shape[1]
            if y_pred_arr.ndim == 2 and y_pred_arr.shape[1] >= n_targets:
                per_target: Dict[str, float] = {}
                for j in range(min(n_targets, len(target_names))):
                    mae_j = float(mean_absolute_error(y_arr[:, j], y_pred_arr[:, j]))
                    per_target[f"mae_{target_names[j]}"] = round(mae_j, 4)
                out["per_target_mae"] = per_target

        return out
    except (ValueError, TypeError) as e:
        logger.warning("metrics.compute_regression_metrics_failed error=%s", e)
        return {}
