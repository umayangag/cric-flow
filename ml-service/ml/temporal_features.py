"""Cyclical temporal feature transforms to replace raw match_date_unix / season_id.

Raw temporal features like ``match_date_unix`` and ``season_id`` cause temporal
leakage — the model memorises *when* a match happened rather than learning from
match conditions.  This module provides cyclical (sin/cos) encodings of month-of-
year and day-of-week that capture seasonal patterns without leaking the exact date.

The same transforms are applied:
  * In Python during training (via :func:`add_temporal_features_to_df` or
    :func:`replace_temporal_columns`).
  * In Go at prediction time (see ``go-app/internal/features/temporal.go``).

Usage::

    from ml.temporal_features import TEMPORAL_FEATURE_COLS, replace_temporal_columns
    df = replace_temporal_columns(df, date_col="match_date", unix_col="match_date_unix")
"""

from __future__ import annotations

import math
from typing import List

import numpy as np
import pandas as pd

# Canonical column names produced by the temporal transform.
TEMPORAL_FEATURE_COLS: List[str] = [
    "month_sin",
    "month_cos",
    "day_of_week_sin",
    "day_of_week_cos",
]

# Columns that temporal features replace (removed from feature vectors).
_REPLACED_COLS: List[str] = ["match_date_unix", "season_id", "season"]


def compute_temporal_from_unix(unix_seconds: np.ndarray) -> dict[str, np.ndarray]:
    """Compute cyclical temporal features from unix timestamps (seconds).

    Parameters
    ----------
    unix_seconds : array-like of float
        Unix epoch seconds (e.g. from ``match_date_unix`` column).

    Returns
    -------
    dict mapping column name → numpy array of float64 values.
    """
    ts = np.asarray(unix_seconds, dtype=np.float64)
    # Treat 0 as missing (aligned with Go prediction path); NaN stays missing.
    ts = np.where(ts == 0, np.nan, ts)
    # Convert to pandas Timestamps for reliable month / day-of-week extraction.
    dt_index = pd.to_datetime(ts, unit="s", utc=True)
    months = np.array(dt_index.month, dtype=np.float64)  # 1–12
    dows = np.array(dt_index.dayofweek, dtype=np.float64)  # 0=Mon … 6=Sun

    two_pi = 2.0 * math.pi
    return {
        "month_sin": np.sin(two_pi * months / 12.0),
        "month_cos": np.cos(two_pi * months / 12.0),
        "day_of_week_sin": np.sin(two_pi * dows / 7.0),
        "day_of_week_cos": np.cos(two_pi * dows / 7.0),
    }


def compute_temporal_from_date(date_series: pd.Series) -> dict[str, np.ndarray]:
    """Compute cyclical temporal features from a date string or datetime Series.

    Parameters
    ----------
    date_series : pd.Series
        Dates as strings (``"2024-03-15"``) or ``datetime64``.

    Returns
    -------
    dict mapping column name → numpy array of float64 values.
    """
    dt = pd.to_datetime(date_series, errors="coerce", utc=True)
    months = np.array(dt.dt.month, dtype=np.float64)
    dows = np.array(dt.dt.dayofweek, dtype=np.float64)

    two_pi = 2.0 * math.pi
    return {
        "month_sin": np.sin(two_pi * months / 12.0),
        "month_cos": np.cos(two_pi * months / 12.0),
        "day_of_week_sin": np.sin(two_pi * dows / 7.0),
        "day_of_week_cos": np.cos(two_pi * dows / 7.0),
    }


def add_temporal_features_to_df(
    df: pd.DataFrame,
    *,
    date_col: str | None = "match_date",
    unix_col: str | None = "match_date_unix",
) -> pd.DataFrame:
    """Add temporal feature columns to *df* (in-place) from the best available source.

    Prefers *date_col* when present; falls back to *unix_col*.  If neither is
    available the temporal columns are filled with 0.0.

    Returns the same DataFrame (modified in-place) for chaining.
    """
    n = len(df)
    if date_col and date_col in df.columns:
        feats = compute_temporal_from_date(df[date_col])
    elif unix_col and unix_col in df.columns:
        feats = compute_temporal_from_unix(pd.to_numeric(df[unix_col], errors="coerce").values)
    else:
        feats = {col: np.zeros(n) for col in TEMPORAL_FEATURE_COLS}

    for col, vals in feats.items():
        df[col] = vals
    # Replace NaN (from unparseable dates) with 0.
    for col in TEMPORAL_FEATURE_COLS:
        df[col] = df[col].fillna(0.0)
    return df


def replace_temporal_columns(
    df: pd.DataFrame,
    *,
    date_col: str | None = "match_date",
    unix_col: str | None = "match_date_unix",
    drop_replaced: bool = False,
) -> pd.DataFrame:
    """Add temporal features and optionally drop the raw columns they replace.

    When *drop_replaced* is True, ``match_date_unix``, ``season_id``, and
    ``season`` columns are removed from *df*.  The raw columns are **not**
    dropped by default so that downstream code (e.g. time-decay weighting)
    that still needs ``match_date`` / ``match_date_unix`` keeps working.
    """
    add_temporal_features_to_df(df, date_col=date_col, unix_col=unix_col)
    if drop_replaced:
        for col in _REPLACED_COLS:
            if col in df.columns:
                df.drop(columns=[col], inplace=True)
    return df


def temporal_from_unix_scalar(unix_seconds: float) -> dict[str, float]:
    """Compute temporal features for a single unix timestamp (prediction time).

    Used by Go-side prediction (via Python bridge) or Python prediction helpers.
    """
    if unix_seconds == 0:
        return {col: 0.0 for col in TEMPORAL_FEATURE_COLS}
    arr = np.array([unix_seconds], dtype=np.float64)
    feats = compute_temporal_from_unix(arr)
    return {k: float(v[0]) for k, v in feats.items()}
