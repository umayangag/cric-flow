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

from ml.config import get_data_quality_config

logger = logging.getLogger(__name__)

# Features that represent encoded categorical IDs — use -1 sentinel for missing.
CATEGORICAL_SENTINEL_FEATURES = frozenset(
    {
        "batting_venue",
        "batting_opposition",
        "bowling_venue",
        "bowling_opposition",
        "match_month_sin",
        "match_month_cos",
        "match_day_of_week_sin",
        "match_day_of_week_cos",
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


# Weather feature columns — kept in feature lists even when currently empty/constant
# because the user plans to populate them in the future.
WEATHER_FEATURE_COLS = frozenset({"temp", "wind", "rain", "humidity", "cloud", "pressure", "viscosity"})


def drop_low_variance_columns(
    X: np.ndarray,
    feature_names: List[str],
    variance_threshold: Optional[float] = None,
    protected_columns: Optional[frozenset] = None,
) -> Tuple[np.ndarray, List[str], List[str]]:
    """Remove effectively constant feature columns that add noise without signal.

    The detection is **scale-aware**: a column is dropped when its standard
    deviation is negligible relative to its magnitude. Concretely, a column is
    flagged when::

        std(col) <= threshold * (|mean(col)| + 1.0)

    The ``+ 1.0`` bootstraps a sensible bar for zero-mean columns so the same
    threshold works for ``season_id`` (magnitude ~2e3) and ``rain`` (magnitude
    ~0.2). Columns whose range (``max - min``) is zero are always treated as
    constant regardless of threshold.

    Weather fields are protected by default (currently often empty but planned
    to be populated), callers can override *protected_columns*.

    Args:
        X: Feature matrix (n_samples, n_features).
        feature_names: Column names matching X.shape[1].
        variance_threshold: Scale-aware coefficient. Defaults to
            ``ml.data_quality.low_variance_threshold`` from service config.
            Recommended values are small (e.g. 1e-6) because the threshold is
            now relative rather than absolute.
        protected_columns: Column names that must never be removed regardless of variance.

    Returns:
        Tuple of (filtered X, filtered feature_names, list of dropped column names).
    """
    if protected_columns is None:
        protected_columns = WEATHER_FEATURE_COLS

    if variance_threshold is None:
        variance_threshold = get_data_quality_config()["low_variance_threshold"]

    if X.shape[1] != len(feature_names):
        logger.warning(
            "data_quality.drop_low_variance_columns.shape_mismatch n_cols=%d n_names=%d",
            X.shape[1],
            len(feature_names),
        )
        return X, list(feature_names), []

    if X.size == 0:
        return X, list(feature_names), []

    n_samples = X.shape[0]
    if n_samples <= 1:
        # With a single row, std is 0 on every column; every non-protected column
        # would be flagged as low-variance and dropped.
        logger.warning(
            "data_quality.drop_low_variance_columns.skip n_samples=%d "
            "(variance filter requires at least 2 rows; check upstream data pipeline if this is common)",
            n_samples,
        )
        return X, list(feature_names), []

    std = np.std(X, axis=0)
    mean_abs = np.abs(np.mean(X, axis=0))
    ranges = np.ptp(X, axis=0)
    scale_aware_threshold = variance_threshold * (mean_abs + 1.0)
    low_variance_mask = (ranges <= 0) | (std <= scale_aware_threshold)
    protected_mask = np.array([name in protected_columns for name in feature_names], dtype=bool)
    drop_mask = low_variance_mask & ~protected_mask
    keep_mask = ~drop_mask
    dropped = [name for name, to_drop in zip(feature_names, drop_mask) if to_drop]

    if dropped:
        logger.info(
            "data_quality.drop_low_variance_columns dropped=%d cols=%s",
            len(dropped),
            dropped,
        )

    return X[:, keep_mask], [n for n, k in zip(feature_names, keep_mask) if k], dropped
