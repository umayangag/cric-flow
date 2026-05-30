"""Data loading functions for CSV and go-app API sources.

Return-type convention
----------------------
Every loader returns either :class:`LoaderResult` (single feature matrix / target
pair, used by batting/bowling) or ``Dict[str, LoaderResult]`` (grouped by format
code, used by extras/win/fielding/innings). The typed envelope makes call sites
explicit about which optional fields (``feature_names``, ``sample_weight``) a
given loader actually populates and eliminates the polymorphic tuple unpacking
that used to live here.
"""

from __future__ import annotations

import logging
import os
from typing import Any, Callable, Dict, List, NamedTuple, Optional, Tuple

import numpy as np
import pandas as pd


class LoaderResult(NamedTuple):
    """Typed envelope returned by data loaders.

    Attributes:
        X: Feature matrix of shape ``(n_samples, n_features)``.
        Y: Target matrix; shape depends on model kind (``(n,)`` or ``(n, k)``).
        feature_names: Column names matching ``X.shape[1]``; ``None`` when the
            loader cannot infer names (e.g. fielding legacy CSVs).
        sample_weight: Optional per-row weights (typically time-decay weights
            emitted by extras/win/fielding loaders).
    """

    X: np.ndarray
    Y: np.ndarray
    feature_names: Optional[List[str]] = None
    sample_weight: Optional[np.ndarray] = None


from ml.data_quality import drop_low_variance_columns, impute_features
from ml.match_level_derived_features import add_match_level_derived_features_to_df
from ml.tuning.types import (
    BAT_SEQ_COLS,
    BATTING_FEATURE_COLS,
    BATTING_TARGET_COLS,
    BOWL_SEQ_COLS,
    BOWLING_FEATURE_COLS,
    BOWLING_TARGET_COLS,
    _train_extras,
    _train_fielding,
    _train_innings,
    _train_win,
)
from ml.win_features import get_format_codes

logger = logging.getLogger(__name__)


def feature_matrix_after_training_transforms(
    X_raw: np.ndarray,
    base_feature_names: List[str],
    model_kind: str,
) -> Tuple[np.ndarray, List[str]]:
    """Apply the same log1p / interaction transforms as offline training; return X and column names."""
    from ml.feature_transforms import apply_transforms, get_transform_config

    transform_config = get_transform_config(model_kind)
    if transform_config.get("add_interactions") or transform_config.get("add_log1p"):
        return apply_transforms(X_raw, list(base_feature_names), transform_config, model_kind)
    return X_raw, list(base_feature_names)


def _wrap_extras_pack(pack: Tuple[Any, ...]) -> LoaderResult:
    """Wrap a train_extras ``(X, Y, w, feat_cols)`` tuple in a LoaderResult."""
    x, y, w, feat = pack[0], pack[1], pack[2], pack[3]
    return LoaderResult(x, y, feature_names=list(feat) if feat is not None else None, sample_weight=w)


def _wrap_win_pack(pack: Tuple[Any, ...]) -> LoaderResult:
    """Wrap a train_win ``(X, Y, w, fmt_feature_cols)`` tuple in a LoaderResult."""
    x, y, w, feat = pack[0], pack[1], pack[2], pack[3]
    return LoaderResult(x, y, feature_names=list(feat) if feat is not None else None, sample_weight=w)


def _wrap_fielding_pack(pack: Tuple[Any, ...]) -> LoaderResult:
    """Wrap a train_fielding ``(X, Y, w, feat_cols)`` tuple in a LoaderResult."""
    x, y, w, feat = pack[0], pack[1], pack[2], pack[3]
    return LoaderResult(x, y, feature_names=list(feat) if feat is not None else None, sample_weight=w)


def _sort_df_by_match_date(df: pd.DataFrame) -> pd.DataFrame:
    """Sorts a DataFrame by 'match_date' if the column exists."""
    if "match_date" in df.columns:
        return df.sort_values("match_date", kind="mergesort").reset_index(drop=True)
    return df


def _sort_rows_by_match_date(headers: List[str], rows: List[List[str]]) -> List[List[str]]:
    """Sort rows by match_date column for temporal order. Returns sorted rows."""
    if not headers or not rows:
        return rows
    date_cols = ("match_date", "match-date", "date")
    try:
        idx = next(i for i, h in enumerate(headers) if h in date_cols)
    except StopIteration:
        return rows
    try:
        return sorted(rows, key=lambda r: str(r[idx]) if idx < len(r) and r[idx] is not None else "")
    except (TypeError, ValueError) as e:
        logger.warning(
            "_sort_rows_by_match_date failed to sort rows, returning original order. error=%s",
            e,
        )
        return rows


def load_batting_csv(path: str) -> LoaderResult:
    df = pd.read_csv(path)
    df = _sort_df_by_match_date(df)
    for col in BAT_SEQ_COLS:
        if col not in df.columns:
            df[col] = 0.0
        else:
            df[col] = df[col].fillna(0.0)
    # Drop rows missing essential targets (aligned with train_batting)
    target_subset = [c for c in BATTING_TARGET_COLS if c in df.columns]
    if target_subset:
        df = df.dropna(subset=target_subset)
    # Impute missing feature values (median for numeric, -1 for categorical)
    feature_cols_in_df = [c for c in BATTING_FEATURE_COLS if c in df.columns]
    df, _ = impute_features(df, feature_cols_in_df)
    for c in BATTING_FEATURE_COLS:
        if c not in df.columns:
            df[c] = 0.0
    X_raw = df[BATTING_FEATURE_COLS].astype(float).values
    X, feature_names_used = feature_matrix_after_training_transforms(X_raw, list(BATTING_FEATURE_COLS), "batting")
    y_cols = [c for c in BATTING_TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if Y.shape[1] < len(BATTING_TARGET_COLS):
        pad = np.zeros((Y.shape[0], len(BATTING_TARGET_COLS) - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    # Strike rate is NOT a training target — it is derived from runs/balls and would cause
    # target leakage. It is computed post-prediction at inference time.
    return LoaderResult(X, Y, feature_names=feature_names_used)


def load_bowling_csv(path: str) -> LoaderResult:
    df = pd.read_csv(path)
    df = _sort_df_by_match_date(df)
    for col in BOWL_SEQ_COLS:
        if col not in df.columns:
            df[col] = 0.0
        else:
            df[col] = df[col].fillna(0.0)
    # Normalize toss: CSV may have "bat"/"field" strings
    if "toss" in df.columns and df["toss"].dtype == object:
        df["toss"] = df["toss"].astype(str).str.strip().str.lower().map(lambda x: 1.0 if x == "bat" else 0.0)
    # Drop rows missing essential targets (aligned with train_bowling)
    target_subset = [c for c in BOWLING_TARGET_COLS if c in df.columns]
    if target_subset:
        df = df.dropna(subset=target_subset)
    # Impute missing feature values (median for numeric, -1 for categorical)
    feature_cols_in_df = [c for c in BOWLING_FEATURE_COLS if c in df.columns]
    df, _ = impute_features(df, feature_cols_in_df)
    for c in BOWLING_FEATURE_COLS:
        if c not in df.columns:
            df[c] = 0.0
    X_raw = df[BOWLING_FEATURE_COLS].astype(float).values
    X, feature_names_used = feature_matrix_after_training_transforms(X_raw, list(BOWLING_FEATURE_COLS), "bowling")
    y_cols = [c for c in BOWLING_TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if Y.shape[1] < len(BOWLING_TARGET_COLS):
        pad = np.zeros((Y.shape[0], len(BOWLING_TARGET_COLS) - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    # Economy rate is NOT a training target — it is derived from runs/balls and would cause
    # target leakage. It is computed post-prediction at inference time.
    return LoaderResult(X, Y, feature_names=feature_names_used)


def load_fielding_csv(path: str, format_code: Optional[str] = None) -> Dict[str, LoaderResult]:
    if _train_fielding is None:
        logger.error("auto_tune.load_fielding_csv.train_fielding_unavailable")
        raise RuntimeError("ml.train_fielding not available for fielding CSV")
    df = pd.read_csv(path)
    df = _sort_df_by_match_date(df)
    headers = list(df.columns)
    rows = df.values.astype(str).tolist()
    by_format = _train_fielding.rows_to_xy_by_format(headers, rows)
    wrapped = {fmt: _wrap_fielding_pack(pack) for fmt, pack in by_format.items()}
    if format_code and format_code in wrapped:
        legacy = wrapped.get("_LEGACY_")
        out = {format_code: wrapped[format_code]}
        if legacy is not None:
            out["_LEGACY_"] = legacy
        return out
    return wrapped


def load_extras_csv(path: str, format_code: Optional[str] = None) -> Dict[str, LoaderResult]:
    if _train_extras is None:
        logger.error("auto_tune.load_extras_csv.train_extras_unavailable")
        raise RuntimeError("ml.train_extras not available for extras CSV")
    df = pd.read_csv(path)
    df = _sort_df_by_match_date(df)
    headers = list(df.columns)
    rows = df.values.astype(str).tolist()
    by_format = _train_extras.rows_to_xy_by_format(headers, rows)
    wrapped = {fmt: _wrap_extras_pack(pack) for fmt, pack in by_format.items()}
    if format_code and format_code in wrapped:
        # Preserve any special legacy entry so callers can still access it when filtering.
        legacy = wrapped.get("_LEGACY_")
        out = {format_code: wrapped[format_code]}
        if legacy is not None:
            out["_LEGACY_"] = legacy
        return out
    return wrapped


def load_win_csv(path: str, format_code: Optional[str] = None) -> Dict[str, LoaderResult]:
    if _train_win is None:
        logger.error("auto_tune.load_win_csv.train_win_unavailable")
        raise RuntimeError("ml.train_win not available for win CSV")
    df = pd.read_csv(path)
    df = _sort_df_by_match_date(df)
    headers = list(df.columns)
    rows = df.values.astype(str).tolist()
    by_format = _train_win.rows_to_xy_by_format(headers, rows)
    wrapped = {fmt: _wrap_win_pack(pack) for fmt, pack in by_format.items()}
    if format_code and format_code in wrapped:
        return {format_code: wrapped[format_code]}
    return wrapped


def load_batting_from_api(go_app_url: str, format_code: str, cutoff: str, api_key: Optional[str]) -> LoaderResult:
    from app.train_on_the_fly import _batting_feature_cols, _batting_rows_to_xy, fetch_training_data

    data = fetch_training_data(go_app_url, format_code, cutoff, api_key, sections="batting")
    bat = data.get("batting") or {}
    headers = bat.get("headers") or []
    rows = _sort_rows_by_match_date(headers, bat.get("rows") or [])
    X_raw, Y = _batting_rows_to_xy(headers, rows)
    base = _batting_feature_cols()
    X, names = feature_matrix_after_training_transforms(X_raw, base, "batting")
    return LoaderResult(X, Y, feature_names=names)


def load_bowling_from_api(go_app_url: str, format_code: str, cutoff: str, api_key: Optional[str]) -> LoaderResult:
    from app.train_on_the_fly import _bowling_feature_cols, _bowling_rows_to_xy, fetch_training_data

    data = fetch_training_data(go_app_url, format_code, cutoff, api_key, sections="bowling")
    bowl = data.get("bowling") or {}
    headers = bowl.get("headers") or []
    rows = _sort_rows_by_match_date(headers, bowl.get("rows") or [])
    X_raw, Y = _bowling_rows_to_xy(headers, rows)
    base = _bowling_feature_cols()
    X, names = feature_matrix_after_training_transforms(X_raw, base, "bowling")
    return LoaderResult(X, Y, feature_names=names)


def load_fielding_from_api(
    go_app_url: str, cutoff: str, api_key: Optional[str], format_filter: Optional[str] = None
) -> Dict[str, LoaderResult]:
    if _train_fielding is None:
        logger.error("auto_tune.load_fielding_from_api.train_fielding_unavailable")
        raise RuntimeError("ml.train_fielding not available for fielding API")
    field = _train_fielding.fetch_fielding_data(go_app_url, cutoff, api_key)
    headers = field.get("headers") or []
    rows = _sort_rows_by_match_date(headers, field.get("rows") or [])
    by_format = _train_fielding.rows_to_xy_by_format(headers, rows)
    wrapped = {fmt: _wrap_fielding_pack(pack) for fmt, pack in by_format.items()}
    if format_filter and format_filter in wrapped:
        legacy = wrapped.get("_LEGACY_")
        out = {format_filter: wrapped[format_filter]}
        if legacy is not None:
            out["_LEGACY_"] = legacy
        return out
    return wrapped


def load_extras_from_api(
    go_app_url: str, cutoff: str, api_key: Optional[str], format_filter: Optional[str] = None
) -> Dict[str, LoaderResult]:
    if _train_extras is None:
        logger.error("auto_tune.load_extras_from_api.train_extras_unavailable")
        raise RuntimeError("ml.train_extras not available for extras API")
    extras = _train_extras.fetch_extras_data(go_app_url, cutoff, api_key)
    headers = extras.get("headers") or []
    rows = _sort_rows_by_match_date(headers, extras.get("rows") or [])
    by_format = _train_extras.rows_to_xy_by_format(headers, rows)
    wrapped = {fmt: _wrap_extras_pack(pack) for fmt, pack in by_format.items()}
    if format_filter and format_filter in wrapped:
        legacy = wrapped.get("_LEGACY_")
        out = {format_filter: wrapped[format_filter]}
        if legacy is not None:
            out["_LEGACY_"] = legacy
        return out
    return wrapped


def load_innings_from_api(
    go_app_url: str, cutoff: str, api_key: Optional[str], format_code: Optional[str]
) -> Dict[str, LoaderResult]:
    """Load innings training data from go-app API for auto_tune."""
    if _train_innings is None:
        logger.error("auto_tune.load_innings_from_api.train_innings_unavailable")
        raise RuntimeError("ml.train_innings not available for innings API")
    innings = _train_innings.fetch_innings_data(go_app_url, cutoff, api_key)
    headers = innings.get("headers") or []
    rows = _sort_rows_by_match_date(headers, innings.get("rows") or [])
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    # Compute derived features (same as train_innings.rows_to_xy_by_format).
    add_match_level_derived_features_to_df(df)
    for c in _train_innings.INNINGS_FEATURE_COLS + _train_innings.INNINGS_TARGET_COLS:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    for c in _train_innings.INNINGS_FEATURE_COLS:
        if c in df.columns:
            df[c] = df[c].fillna(0.0)
    # Add format one-hot (same as train_innings.rows_to_xy_by_format)
    fmt_series = (
        df["format_code"].astype(str).str.strip().str.upper()
        if "format_code" in df.columns
        else pd.Series(["OTHER"] * len(df), index=df.index)
    )
    for col in _train_innings.INNINGS_FORMAT_ONE_HOT_COLS:
        if col == "format_is_OTHER":
            df[col] = (~fmt_series.isin(get_format_codes())).astype(float)
        else:
            code = col.replace("format_is_", "")
            df[col] = (fmt_series == code).astype(float)
    feat_cols = [c for c in _train_innings.INNINGS_FEATURE_COLS if c in df.columns]
    # Per-format: exclude format one-hot cols (constant within a single format group).
    per_format_exclude = frozenset(_train_innings.INNINGS_FORMAT_ONE_HOT_COLS)
    out: Dict[str, LoaderResult] = {}
    if "format_code" not in df.columns:
        df = df.dropna(subset=feat_cols + _train_innings.INNINGS_TARGET_COLS)
        if df.empty or len(df) < _train_innings.MIN_SAMPLES_FOR_FORMAT:
            return {}
        X_raw = df[feat_cols].astype(float).values
        X_raw, feat_cols, _dropped = drop_low_variance_columns(X_raw, feat_cols)
        Y = df[_train_innings.INNINGS_TARGET_COLS].astype(float).values
        out["_ALL_"] = LoaderResult(X_raw, Y, feature_names=feat_cols)
    else:
        for fmt, g in df.groupby("format_code"):
            fmt = str(fmt).strip().upper() or "_ALL_"
            g = g.dropna(
                subset=[c for c in _train_innings.INNINGS_FEATURE_COLS if c in g.columns]
                + _train_innings.INNINGS_TARGET_COLS
            )
            if g.empty or len(g) < _train_innings.MIN_SAMPLES_FOR_FORMAT:
                continue
            feat_cols_fmt = [
                c for c in _train_innings.INNINGS_FEATURE_COLS if c in g.columns and c not in per_format_exclude
            ]
            X_raw = g[feat_cols_fmt].astype(float).values
            X_raw, feat_cols_fmt, _dropped = drop_low_variance_columns(X_raw, feat_cols_fmt)
            Y = g[_train_innings.INNINGS_TARGET_COLS].astype(float).values
            out[fmt] = LoaderResult(X_raw, Y, feature_names=feat_cols_fmt)
    if format_code and format_code in out:
        return {format_code: out[format_code]}
    return out


def load_win_from_api(
    go_app_url: str, cutoff: str, api_key: Optional[str], format_filter: Optional[str] = None
) -> Dict[str, LoaderResult]:
    if _train_win is None:
        logger.error("auto_tune.load_win_from_api.train_win_unavailable")
        raise RuntimeError("ml.train_win not available for win API")
    win = _train_win.fetch_win_data(go_app_url, cutoff, api_key)
    headers = win.get("headers") or []
    rows = _sort_rows_by_match_date(headers, win.get("rows") or [])
    by_format = _train_win.rows_to_xy_by_format(headers, rows)
    wrapped = {fmt: _wrap_win_pack(pack) for fmt, pack in by_format.items()}
    if format_filter and format_filter in wrapped:
        return {format_filter: wrapped[format_filter]}
    return wrapped


def _load_via_csv_or_api(
    csv_path: str,
    load_csv_fn: Callable[[], Any],
    load_api_fn: Callable[[], Any],
    *,
    can_fallback_to_api: bool = True,
) -> Any:
    """Try loading from CSV; if not found, fall back to API when can_fallback_to_api is True."""
    if os.path.isfile(csv_path):
        logger.info("auto_tune.loading_csv path=%s (prefer over API)", csv_path)
        return load_csv_fn()
    if not can_fallback_to_api:
        logger.warning(
            "auto_tune.csv_not_found path=%s hint=Run export-dataset or provide --go-app-url and --cutoff",
            csv_path,
        )
        return None
    logger.warning(
        "auto_tune.csv_not_found path=%s falling_back_to_api hint=Run export-dataset first for faster training",
        csv_path,
    )
    return load_api_fn()
