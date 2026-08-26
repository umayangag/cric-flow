"""
Train innings model (innings-level runs, wickets) from go-app training-data API or CSV.

Fetches GET {GO_APP_URL}/api/backtest/training-data?format=all&cutoff=..., extracts innings
headers/rows, groups by format_code, trains one model per format, saves
innings_scaler_<FMT>.joblib and innings_model_<FMT>.joblib to artifacts dir.

Used for hybrid reconciliation: innings model predicts innings_runs and innings_wickets;
player predictions are rescaled to match these totals for consistency.

Usage:
  GO_APP_URL=http://localhost:8080 python -m ml.train_innings --cutoff 2024-12-01T00:00:00Z
  python -m ml.train_innings --csv path/to/innings_export.csv
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Optional

import joblib
import numpy as np
import pandas as pd
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from ml.artifact_sidecar import write_artifact_meta
from ml.config import (
    default_artifacts_dir,
    default_go_app_export_dir,
    get_match_level_derived_config,
    get_pipeline_common_config,
    get_training_data_fetch_timeout_sec,
    get_training_params,
)
from ml.data_quality import drop_low_variance_columns
from ml.match_level_derived_features import (
    MATCH_LEVEL_DERIVED_FEATURE_COLS,
    add_match_level_derived_features_to_df,
)
from ml.pipeline_common import compute_time_decay_weights
from ml.utils import make_base_estimator
from ml.win_features import get_format_codes, get_format_one_hot_columns

logger = logging.getLogger(__name__)

# Minimum number of samples to train the innings model per format
MIN_SAMPLES_FOR_FORMAT = 10

# Format as categorical one-hot (same convention as win/extras)
WIN_FORMAT_CODES = get_format_codes()
INNINGS_FORMAT_ONE_HOT_COLS = get_format_one_hot_columns()

# Derived feature columns: shared with extras / reconciliation (see match_level_derived_features).
INNINGS_DERIVED_COLS = list(MATCH_LEVEL_DERIVED_FEATURE_COLS)

# Feature columns for innings model: season, venue, inning_number, opposition,
# weather, team sums, derived features, then format one-hot.
INNINGS_FEATURE_COLS = (
    [
        "venue_id",
        "inning_number",
        "opposition_id",
        "match_month_sin",
        "match_month_cos",
        "match_day_of_week_sin",
        "match_day_of_week_cos",
        "bat_consistency_sum",
        "bowl_consistency_sum",
        "bat_form_sum",
        "bowl_form_sum",
    ]
    + INNINGS_DERIVED_COLS
    + INNINGS_FORMAT_ONE_HOT_COLS
)

# Column order for artifacts trained before derived features and sidecars (matches pre-refactor training).
# Reconciliation and callers without sidecar metadata must use this so older joblib models still align.
LEGACY_INNINGS_FEATURE_COLS = [
    "venue_id",
    "inning_number",
    "opposition_id",
    "bat_consistency_sum",
    "bowl_consistency_sum",
    "bat_form_sum",
    "bowl_form_sum",
] + INNINGS_FORMAT_ONE_HOT_COLS
INNINGS_TARGET_COLS = ["innings_runs", "innings_wickets"]

_add_derived_features = add_match_level_derived_features_to_df


def fetch_innings_data(go_app_url: str, cutoff_iso: str, api_key=None):
    """Fetch training data from go-app; return dict with innings headers and rows."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format=all&cutoff={urllib.parse.quote(cutoff_iso)}&sections=innings"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=get_training_data_fetch_timeout_sec()) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        logger.error(
            "train_innings.fetch_innings_data.http_error url=%s code=%s body_preview=%s",
            url,
            e.code,
            (body[:200] + "..." if len(body) > 200 else body),
        )
        raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
    except OSError as e:
        err_msg = str(e).strip()
        logger.error("train_innings.fetch_innings_data.os_error url=%s error=%s", url, e)
        hint = (
            "Go-app may have closed the connection before the response finished (e.g. server write timeout). "
            "Increase go-app server.http_write_timeout_sec (e.g. 600) in go-app/config.json and restart go-app."
        )
        if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
            raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
        raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
    return data.get("innings") or {"headers": [], "rows": []}


def rows_to_xy_by_format(
    headers: list, rows: list[list]
) -> tuple[
    dict[str, tuple[np.ndarray, np.ndarray, StandardScaler, Optional[np.ndarray], list[str]]],
    Optional[np.ndarray],
    Optional[np.ndarray],
    Optional[StandardScaler],
    Optional[list[str]],
]:
    """Build X, Y, scaler, weights, feature_names per format_code.

    Also returns ``(all_X_raw, all_Y, legacy_scaler, legacy_feature_names)`` for the
    legacy unified model. The legacy pool **retains** ``format_is_*`` columns (each
    per-format group drops them because they are constant within the group, but the
    unified model must see formats), and a single low-variance drop is applied on
    the aggregated matrix so the pool is rectangular.
    """
    if not headers or not rows:
        return {}, None, None, None, None
    df = pd.DataFrame(rows, columns=headers)
    # Compute derived features from base columns.
    _add_derived_features(df)
    for c in INNINGS_FEATURE_COLS + INNINGS_TARGET_COLS:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    for c in INNINGS_FEATURE_COLS:
        if c in df.columns:
            df[c] = df[c].fillna(0.0)

    # Add format one-hot from format_code (or default to OTHER when format_code missing)
    fmt_series = (
        df["format_code"].astype(str).str.strip().str.upper()
        if "format_code" in df.columns
        else pd.Series(["OTHER"] * len(df), index=df.index)
    )
    for col in INNINGS_FORMAT_ONE_HOT_COLS:
        if col == "format_is_OTHER":
            df[col] = (~fmt_series.isin(WIN_FORMAT_CODES)).astype(float)
        else:
            code = col.replace("format_is_", "")
            df[col] = (fmt_series == code).astype(float)

    pipe_cfg = get_pipeline_common_config()
    halflife = pipe_cfg.get("time_decay_halflife_years", 2.0)

    def _weights(g: pd.DataFrame) -> Optional[np.ndarray]:
        if "match_date" not in g.columns:
            return None
        return compute_time_decay_weights(g["match_date"], halflife_years=halflife)

    feat_cols = [c for c in INNINGS_FEATURE_COLS if c in df.columns]
    if "format_code" not in df.columns:
        df = df.dropna(subset=feat_cols + INNINGS_TARGET_COLS)
        if df.empty or len(df) < MIN_SAMPLES_FOR_FORMAT:
            return {}, None, None, None, None
        X_raw = df[feat_cols].astype(float).values
        X_raw, feat_cols, _dropped = drop_low_variance_columns(X_raw, feat_cols)
        Y = df[INNINGS_TARGET_COLS].astype(float).values
        scaler = StandardScaler()
        X = scaler.fit_transform(X_raw)
        w = _weights(df)
        return {"_ALL_": (X, Y, scaler, w, feat_cols)}, X_raw, Y, scaler, list(feat_cols)
    out = {}
    # The legacy (unified) pool keeps the full feature list including format_is_*.
    legacy_feat_cols = [c for c in INNINGS_FEATURE_COLS if c in df.columns]
    legacy_rows: list[np.ndarray] = []
    all_Y_list = []
    all_weights_list = []
    # Per-format: exclude format one-hot cols (constant within a single format group).
    per_format_exclude = frozenset(INNINGS_FORMAT_ONE_HOT_COLS)
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in INNINGS_FEATURE_COLS if c in g.columns] + INNINGS_TARGET_COLS)
        if g.empty or len(g) < MIN_SAMPLES_FOR_FORMAT:
            continue
        feat_cols_fmt = [c for c in INNINGS_FEATURE_COLS if c in g.columns and c not in per_format_exclude]
        X_raw = g[feat_cols_fmt].astype(float).values
        X_raw, feat_cols_fmt, _dropped = drop_low_variance_columns(X_raw, feat_cols_fmt)
        Y = g[INNINGS_TARGET_COLS].astype(float).values
        scaler = StandardScaler()
        X = scaler.fit_transform(X_raw)
        w = _weights(g)
        out[fmt] = (X, Y, scaler, w, feat_cols_fmt)
        legacy_rows.append(g[legacy_feat_cols].astype(float).values)
        all_Y_list.append(Y)
        all_weights_list.append(w)
    if not out:
        return {}, None, None, None, None
    all_X_raw = np.vstack(legacy_rows)
    all_Y = np.vstack(all_Y_list)
    # Single low-variance drop on the aggregated pool so legacy_feat_cols matches all_X_raw width.
    all_X_raw, legacy_feat_cols, _dropped_legacy = drop_low_variance_columns(all_X_raw, legacy_feat_cols)
    legacy_scaler = StandardScaler()
    legacy_scaler.fit(all_X_raw)
    return out, all_X_raw, all_Y, legacy_scaler, legacy_feat_cols


def train_and_save(
    X: np.ndarray,
    Y: np.ndarray,
    scaler: StandardScaler,
    out_dir: str,
    format_code: str,
    feature_names: list[str],
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train innings MultiOutputRegressor and save scaler + model + sidecar for format_code."""
    if X.shape[1] != len(feature_names):
        raise ValueError(
            f"train_innings.train_and_save.feature_mismatch X.shape[1]={X.shape[1]} names={len(feature_names)}"
        )
    params = get_training_params("innings", format_code)
    base_estimator = make_base_estimator(params)
    model = MultiOutputRegressor(base_estimator, n_jobs=params.get("n_jobs", -1))
    if sample_weight is not None:
        model.fit(X, Y, sample_weight=sample_weight)
    else:
        model.fit(X, Y)
    os.makedirs(out_dir, exist_ok=True)
    compress = params.get("joblib_compress", 3)
    code = format_code.replace(" ", "_")
    joblib.dump(scaler, os.path.join(out_dir, f"innings_scaler_{code}.joblib"), compress=compress)
    joblib.dump(model, os.path.join(out_dir, f"innings_model_{code}.joblib"), compress=compress)
    write_artifact_meta(
        out_dir,
        "innings",
        format_code,
        feature_names,
        derived_weights=get_match_level_derived_config(),
    )
    logger.info("train_innings.saved format=%s n=%s out_dir=%s", format_code, X.shape[0], out_dir)


def train_and_save_legacy(
    X: np.ndarray,
    Y: np.ndarray,
    scaler: StandardScaler,
    out_dir: str,
    feature_names: list[str],
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train unified innings model on all data and save legacy artifacts + sidecar."""
    if X.shape[1] != len(feature_names):
        raise ValueError(
            f"train_innings.train_and_save_legacy.feature_mismatch X.shape[1]={X.shape[1]} names={len(feature_names)}"
        )
    params = get_training_params("innings", None)
    base_estimator = make_base_estimator(params)
    model = MultiOutputRegressor(base_estimator, n_jobs=params.get("n_jobs", -1))
    if sample_weight is not None:
        model.fit(X, Y, sample_weight=sample_weight)
    else:
        model.fit(X, Y)
    os.makedirs(out_dir, exist_ok=True)
    compress = params.get("joblib_compress", 3)
    joblib.dump(scaler, os.path.join(out_dir, "innings_scaler.joblib"), compress=compress)
    joblib.dump(model, os.path.join(out_dir, "innings_model.joblib"), compress=compress)
    write_artifact_meta(
        out_dir,
        "innings",
        None,
        feature_names,
        derived_weights=get_match_level_derived_config(),
    )
    logger.info("train_innings.saved_unified out_dir=%s rows=%s", out_dir, X.shape[0])


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    ap = argparse.ArgumentParser(
        description="Train innings model (runs, wickets per innings) from go-app training-data API or CSV"
    )
    ap.add_argument("--cutoff", default="", help="RFC3339 cutoff (required for API fallback)")
    ap.add_argument(
        "--csv",
        default="",
        help="Path to innings CSV (optional; else use GO_APP_OUTPUT_DIR/innings_encoded_all.csv or API)",
    )
    ap.add_argument("--out", default="", help="Artifacts output dir (default from config)")
    ap.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""), help="Go-app base URL")
    ap.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""), help="Optional API key")
    args = ap.parse_args()
    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR") or default_artifacts_dir()
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR") or default_go_app_export_dir()
    csv_path = args.csv or os.path.join(default_csv_dir, "innings_encoded_all.csv")

    if os.path.isfile(csv_path):
        logger.info("train_innings.loading_csv path=%s (prefer CSV over API)", csv_path)
        df = pd.read_csv(csv_path)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format, all_X_raw, all_Y, legacy_scaler, legacy_feat_names = rows_to_xy_by_format(headers, rows)
    else:
        if not args.go_app_url or not args.cutoff:
            logger.error(
                "train_innings.csv_not_found path=%s hint=Run export-dataset first, or provide --go-app-url and --cutoff for API fallback",
                csv_path,
            )
            sys.exit(1)
        logger.warning(
            "train_innings.csv_not_found path=%s falling_back_to_api hint=Run export-dataset first for faster training",
            csv_path,
        )
        logger.info("train_innings.fetching_api go_app_url=%s cutoff=%s", args.go_app_url, args.cutoff)
        try:
            innings = fetch_innings_data(args.go_app_url, args.cutoff, args.api_key or None)
        except ValueError as e:
            logger.error("train_innings.fetch_failed error=%s", e)
            sys.exit(1)
        headers = innings.get("headers") or []
        rows = innings.get("rows") or []
        by_format, all_X_raw, all_Y, legacy_scaler, legacy_feat_names = rows_to_xy_by_format(headers, rows)

    if not by_format:
        logger.error("train_innings.no_data hint=empty or insufficient rows")
        sys.exit(1)
    for fmt, (X, Y, scaler, w, feat_names) in by_format.items():
        logger.info("pipeline: train_innings processing format=%s n=%s", fmt, X.shape[0])
        train_and_save(X, Y, scaler, out_dir, fmt, feat_names, sample_weight=w)

    # Unified (legacy) model: train on all data combined
    if (
        all_X_raw is not None
        and all_Y is not None
        and legacy_scaler is not None
        and legacy_feat_names is not None
        and all_X_raw.shape[0] >= MIN_SAMPLES_FOR_FORMAT
    ):
        all_X = legacy_scaler.transform(all_X_raw)
        all_weights_list = [w for _, (_, _, _, w, _fn) in by_format.items()]
        all_weights = np.concatenate(all_weights_list) if all(w is not None for w in all_weights_list) else None
        train_and_save_legacy(all_X, all_Y, legacy_scaler, out_dir, legacy_feat_names, sample_weight=all_weights)


if __name__ == "__main__":
    main()
