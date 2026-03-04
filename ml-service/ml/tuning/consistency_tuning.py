"""Consistency-aware tuning helpers (Stage 2.2).

When ml.consistency_regularization.enabled is true and the tuning report
contains consistency_metrics (e.g. from an offline validation step or future
hook), this module computes the consistency penalty and combined score for
logging and for optional use in ranking.

Combined score is defined as: base_mae + lambda_consistency * consistency_penalty
(lower is better). Regression tuning uses neg_mean_absolute_error so
best_cv_score = -base_mae; we derive base_mae from that when augmenting.
"""

from __future__ import annotations

import logging
from typing import Any, Dict, Optional

from ml.config import get_consistency_regularization_config
from ml.consistency_losses import consistency_penalty_from_metrics

logger = logging.getLogger(__name__)


def augment_tuning_report_with_consistency(
    report: Dict[str, Any],
    model_kind: str,
    format_suffix: Optional[str] = None,
) -> None:
    """If consistency regularization is enabled and report has consistency_metrics, add penalty and combined score.

    Modifies report in place. When consistency_metrics are missing or config is disabled,
    does nothing (backward-compatible). If consistency evaluation fails, we do not add
    consistency_penalty and leave ranking to base_cv_score only (guard rail).
    """
    cfg = get_consistency_regularization_config(model_kind)
    if not cfg.get("enabled", False):
        return
    metrics = report.get("consistency_metrics")
    if not metrics or not isinstance(metrics, dict):
        return
    try:
        penalty = consistency_penalty_from_metrics(
            metrics,
            weight_runs=cfg.get("lambda_runs", 0.01),
            weight_wickets=cfg.get("lambda_wickets", 0.01),
        )
    except (TypeError, ValueError, KeyError) as e:
        logger.warning(
            "tuning.consistency_penalty_compute_failed model=%s format=%s error=%s; using base score only",
            model_kind,
            format_suffix or "",
            e,
        )
        return
    report["consistency_penalty"] = round(float(penalty), 6)
    best_cv = report.get("best_cv_score")
    if best_cv is not None:
        try:
            base_mae = float(-(best_cv))
            score_combined_mae = base_mae + penalty
            report["base_mae"] = round(base_mae, 6)
            report["score_combined_mae"] = round(score_combined_mae, 6)
            logger.info(
                "tuning.consistency.combined model=%s format=%s base_mae=%s consistency_penalty=%s score_combined_mae=%s",
                model_kind,
                format_suffix or "",
                report["base_mae"],
                report["consistency_penalty"],
                report["score_combined_mae"],
            )
        except (TypeError, ValueError):
            pass
