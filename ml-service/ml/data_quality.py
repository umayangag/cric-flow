"""
Shared data quality utilities for training: imputation and outlier clipping.

Imputation strategy:
- Categorical-like encoded features (venue, opposition, season_id): use -1 sentinel
  to distinguish "unknown/missing" from valid encoded values (which start at 0).
- Numeric features (form, consistency, momentum, weather, seq cols): use column median
  so missing values don't bias the model toward zero.
- At prediction time, the same sentinel/median values must be applied. Medians are
  saved in training metadata for prediction-time consistency.

Outlier clipping:
- Configurable percentile-based clipping on target columns to reduce influence of
  extreme values on MSE-based training.
"""

from __future__ import annotations

import logging
from typing import Dict, List, Optional, Tuple

import numpy as np
import pandas as pd

logger = logging.getLogger(__name__)

# Features that represent encoded categorical IDs — use -1 sentinel for missing.
CATEGORICAL_SENTINEL_FEATURES = frozenset(
    {
        "batting_venue",
        "batting_opposition",
        "bowling_venue",
        "bowling_opposition",
        "season_id",
        "venue",
        "opposition",
    }
)

# Default sentinel value for missing categorical features.
CATEGORICAL_SENTINEL = -1.0


def impute_features(
    df: pd.DataFrame,
    feature_columns: List[str],
    categorical_features: Optional[frozenset] = None,
    precomputed_medians: Optional[Dict[str, float]] = None,
) -> Tuple[pd.DataFrame, Dict[str, float]]:
    """Impute missing feature values with median (numeric) or sentinel (categorical).

    Args:
        df: DataFrame with feature columns.
        feature_columns: List of feature column names to impute.
        categorical_features: Set of column names to treat as categorical (use sentinel).
            Defaults to CATEGORICAL_SENTINEL_FEATURES.
        precomputed_medians: If provided, use these medians instead of computing from df.
            Used at prediction time for consistency with training.

    Returns:
        Tuple of (imputed DataFrame copy, dict of column->median used for numeric cols).
    """
    if categorical_features is None:
        categorical_features = CATEGORICAL_SENTINEL_FEATURES

    df = df.copy()
    medians: Dict[str, float] = {}

    for col in feature_columns:
        if col not in df.columns:
            continue

        if col in categorical_features:
            df[col] = df[col].fillna(CATEGORICAL_SENTINEL)
        else:
            if precomputed_medians and col in precomputed_medians:
                median_val = precomputed_medians[col]
            else:
                median_val = float(df[col].median()) if not df[col].isna().all() else 0.0
            medians[col] = median_val
            df[col] = df[col].fillna(median_val)

    return df, medians


def clip_target_outliers(
    targets: np.ndarray,
    percentile: float = 99.0,
    target_names: Optional[List[str]] = None,
) -> Tuple[np.ndarray, Dict[str, Dict[str, float]]]:
    """Clip target values at the given percentile to reduce outlier influence.

    Args:
        targets: 2D array of shape (n_samples, n_targets).
        percentile: Upper percentile to clip at (e.g., 99.0 clips at 99th percentile).
        target_names: Optional names for logging.

    Returns:
        Tuple of (clipped targets, dict of target_name -> {upper_bound, n_clipped}).
    """
    if percentile >= 100.0 or percentile <= 0.0:
        return targets, {}

    clipped = targets.copy()
    clip_info: Dict[str, Dict[str, float]] = {}

    for col_idx in range(clipped.shape[1]):
        col_data = clipped[:, col_idx]
        upper = float(np.percentile(col_data, percentile))
        lower = float(np.percentile(col_data, 100.0 - percentile))
        n_clipped_upper = int(np.sum(col_data > upper))
        n_clipped_lower = int(np.sum(col_data < lower))
        clipped[:, col_idx] = np.clip(col_data, lower, upper)

        name = target_names[col_idx] if target_names and col_idx < len(target_names) else f"target_{col_idx}"
        clip_info[name] = {
            "upper_bound": upper,
            "lower_bound": lower,
            "n_clipped_upper": n_clipped_upper,
            "n_clipped_lower": n_clipped_lower,
        }

    total_clipped = sum(v["n_clipped_upper"] + v["n_clipped_lower"] for v in clip_info.values())
    if total_clipped > 0:
        logger.info(
            "data_quality.clip_target_outliers percentile=%.1f total_clipped=%d details=%s",
            percentile,
            total_clipped,
            {k: v for k, v in clip_info.items() if v["n_clipped_upper"] + v["n_clipped_lower"] > 0},
        )

    return clipped, clip_info
