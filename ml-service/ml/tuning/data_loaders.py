"""Data loading for the win model, from CSV or the go-app API.

The loader returns ``Dict[str, LoaderResult]`` keyed by format code. The batting,
bowling, fielding, extras and innings loaders went with their trainers in P-5.
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


from ml.export_csv import read_export_csv
from ml.tuning.types import _train_win

logger = logging.getLogger(__name__)


def _wrap_win_pack(pack: Tuple[Any, ...]) -> LoaderResult:
    """Wrap a train_win ``(X, Y, w, fmt_feature_cols)`` tuple in a LoaderResult."""
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


def load_win_csv(path: str, format_code: Optional[str] = None) -> Dict[str, LoaderResult]:
    if _train_win is None:
        logger.error("auto_tune.load_win_csv.train_win_unavailable")
        raise RuntimeError("ml.train_win not available for win CSV")
    df = read_export_csv(path)
    df = _sort_df_by_match_date(df)
    headers = list(df.columns)
    rows = df.values.astype(str).tolist()
    by_format = _train_win.rows_to_xy_by_format(headers, rows)
    wrapped = {fmt: _wrap_win_pack(pack) for fmt, pack in by_format.items()}
    if format_code and format_code in wrapped:
        return {format_code: wrapped[format_code]}
    return wrapped


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
