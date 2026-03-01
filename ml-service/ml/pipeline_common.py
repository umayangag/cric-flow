"""
Shared preprocessing and validation for all ML training pipelines.

Provides:
- Time-decay sample weights (recent matches weighted more)
- RobustScaler vs StandardScaler (model-agnostic preprocessing)
- Brier score (win probability calibration)
- Delta guardrail (train-val gap < threshold)
"""

from __future__ import annotations

import logging
import warnings
from datetime import date
from typing import Optional

import numpy as np
import pandas as pd
from sklearn.preprocessing import RobustScaler, StandardScaler

logger = logging.getLogger(__name__)


def compute_time_decay_weights(
    dates: pd.Series,
    reference_date: Optional[date] = None,
    halflife_years: float = 2.0,
) -> Optional[np.ndarray]:
    """Exponential time decay: recent matches weighted more.

    w = 0.5^(years_ago / halflife). Matches from 2y ago get 0.5x weight.
    Returns None if dates are invalid or empty.
    """
    if dates is None or len(dates) == 0:
        return None
    try:
        with warnings.catch_warnings():
            warnings.filterwarnings("ignore", message=".*Could not infer format.*")
            dt = pd.to_datetime(dates, errors="coerce")
        valid = dt.notna()
        if not valid.any():
            return None
        d = dt[valid].dt.date
        ref = reference_date or max(d)
        try:
            if hasattr(ref, "date") and callable(ref.date):
                ref = ref.date()
        except (TypeError, AttributeError):
            pass
        years_ago = np.array([(ref - x).days / 365.25 for x in d], dtype=float)
        w = np.power(0.5, years_ago / halflife_years)
        out = np.ones(len(dates), dtype=float)
        out[valid.values] = w
        return out
    except Exception as e:
        logger.warning("pipeline_common.compute_time_decay_weights.failed error=%s", e)
        return None


def get_scaler(use_robust: bool = True):
    """Return RobustScaler (outlier-robust) or StandardScaler."""
    return RobustScaler() if use_robust else StandardScaler()


def brier_score(y_true: np.ndarray, y_prob: np.ndarray) -> float:
    """Brier score for probabilistic predictions (0 = perfect, 1 = worst)."""
    return float(np.mean((y_true - y_prob) ** 2))


def check_delta_guardrail(
    train_metric: float,
    val_metric: float,
    threshold: float = 0.08,
) -> tuple[bool, float]:
    """Check if train-val gap (delta) is below threshold.

    Returns (passes, delta). Overfitting: val worse than train -> positive delta.
    """
    delta = val_metric - train_metric
    passes = abs(delta) < threshold
    return passes, float(delta)
