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

from ml.config import (
    default_artifacts_dir,
    default_go_app_export_dir,
    get_pipeline_common_config,
    get_training_data_fetch_timeout_sec,
    get_training_params,
)
from ml.pipeline_common import compute_time_decay_weights
from ml.temporal_features import TEMPORAL_FEATURE_COLS, add_temporal_features_to_df
from ml.utils import make_base_estimator
from ml.win_features import _FORMAT_CODES as WIN_FORMAT_CODES  # reuse configured formats for one-hot

logger = logging.getLogger(__name__)

# Minimum number of samples to train the innings model per format
MIN_SAMPLES_FOR_FORMAT = 10

# Format as categorical one-hot (same convention as win/extras)
INNINGS_FORMAT_ONE_HOT_COLS = [f"format_is_{code}" for code in WIN_FORMAT_CODES] + ["format_is_OTHER"]

# Derived feature columns computed from base columns during training.
INNINGS_DERIVED_COLS = [
    "form_differential",        # bat_form_sum - bowl_form_sum
    "consistency_differential", # bat_consistency_sum - bowl_consistency_sum
    "weather_composite",        # weighted combination of rain + humidity + cloud
]

# Feature columns for innings model: venue, inning_number, opposition, weather, team sums,
# cyclical temporal features, derived features, then format one-hot.
INNINGS_FEATURE_COLS = [
    "venue_id",
    "inning_number",
    "opposition_id",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
    "bat_consistency_sum",
    "bowl_consistency_sum",
    "bat_form_sum",
    "bowl_form_sum",
] + TEMPORAL_FEATURE_COLS + INNINGS_DERIVED_COLS + INNINGS_FORMAT_ONE_HOT_COLS
INNINGS_TARGET_COLS = ["innings_runs", "innings_wickets"]


def _add_derived_features(df: pd.DataFrame) -> None:
    """Compute derived features in-place from base columns.

    - form_differential: bat_form_sum - bowl_form_sum (batting strength vs bowling quality)
    - consistency_differential: bat_consistency_sum - bowl_consistency_sum
    - weather_composite: 0.5 * rain + 0.3 * humidity/100 + 0.2 * cloud/100 (normalised 0–1 scale)
    """
    def _col(name: str) -> pd.Series:
        if name in df.columns:
            return pd.to_numeric(df[name], errors="coerce").fillna(0.0)
        return pd.Series(np.zeros(len(df)), index=df.index)

    bat_form = _col("bat_form_sum")
    bowl_form = _col("bowl_form_sum")
    bat_cons = _col("bat_consistency_sum")
    bowl_cons = _col("bowl_consistency_sum")
    rain = _col("rain")
    humidity = _col("humidity")
    cloud = _col("cloud")

    df["form_differential"] = bat_form - bowl_form
    df["consistency_differential"] = bat_cons - bowl_cons
    df["weather_composite"] = 0.5 * rain + 0.3 * (humidity / 100.0) + 0.2 * (cloud / 100.0)


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
    dict[str, tuple[np.ndarray, np.ndarray, StandardScaler, Optional[np.ndarray]]],
    Optional[np.ndarray],
    Optional[np.ndarray],
    Optional[StandardScaler],
]:
    """Build X, Y, scaler, weights per format_code. Also returns (all_X_raw, all_Y, legacy_scaler) for legacy model."""
    if not headers or not rows:
        return {}, None, None, None
    df = pd.DataFrame(rows, columns=headers)
    # Compute cyclical temporal features (replaces season_id / match_date_unix).
    add_temporal_features_to_df(df)
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
            return {}, None, None, None
        X_raw = df[feat_cols].astype(float).values
        Y = df[INNINGS_TARGET_COLS].astype(float).values
        scaler = StandardScaler()
        X = scaler.fit_transform(X_raw)
        w = _weights(df)
        return {"_ALL_": (X, Y, scaler, w)}, X_raw, Y, scaler
    out = {}
    all_X_raw_list = []
    all_Y_list = []
    all_weights_list = []
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in INNINGS_FEATURE_COLS if c in g.columns] + INNINGS_TARGET_COLS)
        if g.empty or len(g) < MIN_SAMPLES_FOR_FORMAT:
            continue
        feat_cols_fmt = [c for c in INNINGS_FEATURE_COLS if c in g.columns]
        X_raw = g[feat_cols_fmt].astype(float).values
        Y = g[INNINGS_TARGET_COLS].astype(float).values
        scaler = StandardScaler()
        X = scaler.fit_transform(X_raw)
        w = _weights(g)
        out[fmt] = (X, Y, scaler, w)
        all_X_raw_list.append(X_raw)
        all_Y_list.append(Y)
        all_weights_list.append(w)
    if not out:
        return {}, None, None, None
    all_X_raw = np.vstack(all_X_raw_list)
    all_Y = np.vstack(all_Y_list)
    legacy_scaler = StandardScaler()
    legacy_scaler.fit(all_X_raw)
    return out, all_X_raw, all_Y, legacy_scaler


def train_and_save(
    X: np.ndarray,
    Y: np.ndarray,
    scaler: StandardScaler,
    out_dir: str,
    format_code: str,
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train innings MultiOutputRegressor and save scaler + model for format_code."""
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
    logger.info("train_innings.saved format=%s n=%s out_dir=%s", format_code, X.shape[0], out_dir)


def train_and_save_legacy(
    X: np.ndarray,
    Y: np.ndarray,
    scaler: StandardScaler,
    out_dir: str,
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train one unified innings model on all data and save as legacy (innings_scaler.joblib, innings_model.joblib)."""
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
        by_format, all_X_raw, all_Y, legacy_scaler = rows_to_xy_by_format(headers, rows)
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
        by_format, all_X_raw, all_Y, legacy_scaler = rows_to_xy_by_format(headers, rows)

    if not by_format:
        logger.error("train_innings.no_data hint=empty or insufficient rows")
        sys.exit(1)
    for fmt, (X, Y, scaler, w) in by_format.items():
        logger.info("pipeline: train_innings processing format=%s n=%s", fmt, X.shape[0])
        train_and_save(X, Y, scaler, out_dir, fmt, sample_weight=w)

    # Unified (legacy) model: train on all data combined
    if (
        all_X_raw is not None
        and all_Y is not None
        and legacy_scaler is not None
        and all_X_raw.shape[0] >= MIN_SAMPLES_FOR_FORMAT
    ):
        all_X = legacy_scaler.transform(all_X_raw)
        all_weights_list = [w for _, (_, _, _, w) in by_format.items()]
        all_weights = np.concatenate(all_weights_list) if all(w is not None for w in all_weights_list) else None
        train_and_save_legacy(all_X, all_Y, legacy_scaler, out_dir, sample_weight=all_weights)


if __name__ == "__main__":
    main()
