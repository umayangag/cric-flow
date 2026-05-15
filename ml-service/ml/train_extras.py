"""
Train extras model (match-level total_extras) from go-app training-data API or CSV.

Fetches GET {GO_APP_URL}/api/backtest/training-data?format=all&cutoff=..., extracts extras
headers/rows, groups by format_code, trains one model per format, saves
extras_model_<FMT>.joblib to artifacts dir.

Usage:
  GO_APP_URL=http://localhost:8080 python -m ml.train_extras --cutoff 2024-12-01T00:00:00Z
  python -m ml.train_extras --csv path/to/extras_export.csv
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
from sklearn.ensemble import RandomForestRegressor

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
from ml.win_features import _FORMAT_CODES as WIN_FORMAT_CODES  # reuse configured formats for one-hot encoding

logger = logging.getLogger(__name__)

# Minimum number of samples to train the unified (legacy) extras model
MIN_SAMPLES_FOR_LEGACY = 10

EXTRAS_FORMAT_ONE_HOT_COLS = [f"format_is_{code}" for code in WIN_FORMAT_CODES] + ["format_is_OTHER"]

# Same feature families as batting/bowling/fielding: format (categorical one-hot),
# venue, season, weather, and match-level aggregates of player consistency/form.
# Derived feature columns: shared with innings / reconciliation (see match_level_derived_features).
EXTRAS_DERIVED_COLS = list(MATCH_LEVEL_DERIVED_FEATURE_COLS)

EXTRAS_FEATURE_COLS = (
    [
        "season_id",
        "venue_id",
        "match_date_unix",
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
    ]
    + EXTRAS_DERIVED_COLS
    + EXTRAS_FORMAT_ONE_HOT_COLS
)

# Order for legacy extras artifacts without sidecar metadata (matches pre-derived training).
LEGACY_EXTRAS_FEATURE_COLS = [
    "venue_id",
    "season_id",
    "match_date_unix",
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
] + EXTRAS_FORMAT_ONE_HOT_COLS
EXTRAS_TARGET_COL = "total_extras"

_add_derived_features = add_match_level_derived_features_to_df


def _concat_weights_extras(weights_list: list[Optional[np.ndarray]]) -> Optional[np.ndarray]:
    """Concatenate per-format weights for unified model. Returns None if any format lacks weights."""
    if not weights_list or any(w is None for w in weights_list):
        return None
    return np.concatenate(weights_list)


def fetch_extras_data(go_app_url: str, cutoff_iso: str, api_key=None):
    """Fetch training data from go-app; return dict with extras headers and rows."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format=all&cutoff={urllib.parse.quote(cutoff_iso)}&sections=extras"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=get_training_data_fetch_timeout_sec()) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        logger.error(
            "train_extras.fetch_extras_data.http_error url=%s code=%s body_preview=%s",
            url,
            e.code,
            (body[:200] + "..." if len(body) > 200 else body),
        )
        raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
    except OSError as e:
        err_msg = str(e).strip()
        logger.error("train_extras.fetch_extras_data.os_error url=%s error=%s", url, e)
        hint = (
            "Go-app may have closed the connection before the response finished (e.g. server write timeout). "
            "Increase go-app server.http_write_timeout_sec (e.g. 600) in go-app/config.json and restart go-app."
        )
        if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
            raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
        raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
    return data.get("extras") or {"headers": [], "rows": []}


def rows_to_xy_by_format(
    headers: list, rows: list[list]
) -> dict[str, tuple[np.ndarray, np.ndarray, Optional[np.ndarray], list[str]]]:
    """Build X, Y, weights per format_code.

    Returns dict format_code -> (X, Y, sample_weight, feature_column_names).
    feature_column_names matches X.shape[1] (subset of EXTRAS_FEATURE_COLS present in the frame).

    The special key ``"_LEGACY_"`` (when multiple formats are present) carries the
    aggregated pool used to fit the unified legacy model, retaining ``format_is_*``
    columns so the unified model can learn format-specific behaviour.
    """
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    # Compute derived features from base columns.
    _add_derived_features(df)
    for c in EXTRAS_FEATURE_COLS + [EXTRAS_TARGET_COL]:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    for c in EXTRAS_FEATURE_COLS:
        if c in df.columns:
            df[c] = df[c].fillna(0.0)

    # One-hot encode format as categorical when format_code is available.
    if "format_code" in df.columns:
        fmt_series = df["format_code"].astype(str).str.strip().str.upper()
        for code in WIN_FORMAT_CODES:
            col_name = f"format_is_{code}"
            df[col_name] = (fmt_series == code).astype(float)
        df["format_is_OTHER"] = (~fmt_series.isin(list(WIN_FORMAT_CODES))).astype(float)
    pipe_cfg = get_pipeline_common_config()
    halflife = pipe_cfg.get("time_decay_halflife_years", 2.0)

    def _weights(g: pd.DataFrame) -> Optional[np.ndarray]:
        if "match_date" not in g.columns:
            return None
        return compute_time_decay_weights(g["match_date"], halflife_years=halflife)

    if "format_code" not in df.columns:
        df = df.dropna(subset=[c for c in EXTRAS_FEATURE_COLS if c in df.columns] + [EXTRAS_TARGET_COL])
        if df.empty:
            return {}
        feat_cols = [c for c in EXTRAS_FEATURE_COLS if c in df.columns]
        X = df[feat_cols].astype(float).values
        X, feat_cols, _dropped = drop_low_variance_columns(X, feat_cols)
        Y = df[EXTRAS_TARGET_COL].astype(float).values.reshape(-1, 1)
        w = _weights(df)
        return {"_ALL_": (X, Y, w, feat_cols)}
    out = {}
    # Per-format: exclude format one-hot cols (constant within a single format group).
    per_format_exclude = frozenset(EXTRAS_FORMAT_ONE_HOT_COLS)
    legacy_feat_cols = [c for c in EXTRAS_FEATURE_COLS if c in df.columns]
    legacy_rows: list[np.ndarray] = []
    legacy_Y: list[np.ndarray] = []
    legacy_w: list[Optional[np.ndarray]] = []
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in EXTRAS_FEATURE_COLS if c in g.columns] + [EXTRAS_TARGET_COL])
        if g.empty or len(g) < MIN_SAMPLES_FOR_LEGACY:
            continue
        feat_cols = [c for c in EXTRAS_FEATURE_COLS if c in g.columns and c not in per_format_exclude]
        X = g[feat_cols].astype(float).values
        X, feat_cols, _dropped = drop_low_variance_columns(X, feat_cols)
        Y = g[EXTRAS_TARGET_COL].astype(float).values.reshape(-1, 1)
        w = _weights(g)
        out[fmt] = (X, Y, w, feat_cols)
        legacy_rows.append(g[legacy_feat_cols].astype(float).values)
        legacy_Y.append(Y)
        legacy_w.append(w)
    if legacy_rows:
        all_X = np.vstack(legacy_rows)
        all_Y = np.vstack(legacy_Y)
        # Single low-variance drop on the aggregated legacy pool so column width is consistent.
        all_X, legacy_feat_cols, _ = drop_low_variance_columns(all_X, legacy_feat_cols)
        all_w = _concat_weights_extras(legacy_w)
        out["_LEGACY_"] = (all_X, all_Y, all_w, legacy_feat_cols)
    return out


def train_and_save(
    X: np.ndarray,
    Y: np.ndarray,
    out_dir: str,
    format_code: str,
    feature_names: list[str],
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train extras regressor and save model + sidecar for format_code.

    No scaler is used (extras is a RandomForest); the sidecar pins the feature
    order so inference can shape the vector correctly when per-format
    low-variance drop removes columns.
    """
    if X.shape[1] != len(feature_names):
        raise ValueError(
            f"train_extras.train_and_save.feature_mismatch X.shape[1]={X.shape[1]} names={len(feature_names)}"
        )
    params = get_training_params("extras", format_code)
    model = RandomForestRegressor(
        n_estimators=params["n_estimators"],
        max_depth=params["max_depth"],
        random_state=params["random_state"],
        n_jobs=params.get("n_jobs", -1),
    )
    if sample_weight is not None:
        model.fit(X, Y.ravel(), sample_weight=sample_weight)
    else:
        model.fit(X, Y.ravel())
    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    code = format_code.replace(" ", "_")
    joblib.dump(model, os.path.join(out_dir, f"extras_model_{code}.joblib"), compress=compress)
    write_artifact_meta(
        out_dir,
        "extras",
        format_code,
        feature_names,
        derived_weights=get_match_level_derived_config(),
    )


def train_and_save_legacy(
    X: np.ndarray,
    Y: np.ndarray,
    out_dir: str,
    feature_names: list[str],
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train unified extras model on all data and save legacy artifacts + sidecar."""
    if X.shape[1] != len(feature_names):
        raise ValueError(
            f"train_extras.train_and_save_legacy.feature_mismatch X.shape[1]={X.shape[1]} names={len(feature_names)}"
        )
    params = get_training_params("extras", None)
    model = RandomForestRegressor(
        n_estimators=params["n_estimators"],
        max_depth=params["max_depth"],
        random_state=params["random_state"],
        n_jobs=params.get("n_jobs", -1),
    )
    if sample_weight is not None:
        model.fit(X, Y.ravel(), sample_weight=sample_weight)
    else:
        model.fit(X, Y.ravel())
    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    joblib.dump(model, os.path.join(out_dir, "extras_model.joblib"), compress=compress)
    write_artifact_meta(
        out_dir,
        "extras",
        None,
        feature_names,
        derived_weights=get_match_level_derived_config(),
    )
    logger.info("train_extras.saved_unified out_dir=%s rows=%s", out_dir, X.shape[0])


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    ap = argparse.ArgumentParser(
        description="Train extras model from go-app export CSV (preferred) or training-data API"
    )
    ap.add_argument("--cutoff", default="", help="RFC3339 cutoff (required for API fallback)")
    ap.add_argument(
        "--csv",
        default="",
        help="Path to extras CSV (optional; else use GO_APP_OUTPUT_DIR/extras_encoded_all.csv or API)",
    )
    ap.add_argument("--out", default="", help="Artifacts output dir (default from config)")
    ap.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""), help="Go-app base URL")
    ap.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""), help="Optional API key")
    args = ap.parse_args()
    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR") or default_artifacts_dir()
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR") or default_go_app_export_dir()
    csv_path = args.csv or os.path.join(default_csv_dir, "extras_encoded_all.csv")

    if os.path.isfile(csv_path):
        logger.info("train_extras.loading_csv path=%s (prefer CSV over API)", csv_path)
        df = pd.read_csv(csv_path)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format = rows_to_xy_by_format(headers, rows)
    else:
        if not args.go_app_url or not args.cutoff:
            logger.error(
                "train_extras.csv_not_found path=%s hint=Run export-dataset first, or provide --go-app-url and --cutoff for API fallback",
                csv_path,
            )
            sys.exit(1)
        logger.warning(
            "train_extras.csv_not_found path=%s falling_back_to_api hint=Run export-dataset first for faster training",
            csv_path,
        )
        logger.info("train_extras.fetching_api go_app_url=%s cutoff=%s", args.go_app_url, args.cutoff)
        try:
            extras = fetch_extras_data(args.go_app_url, args.cutoff, args.api_key or None)
        except ValueError as e:
            logger.error("train_extras.fetch_failed error=%s", e)
            sys.exit(1)
        headers = extras.get("headers") or []
        rows = extras.get("rows") or []
        by_format = rows_to_xy_by_format(headers, rows)

    if not by_format:
        logger.error("train_extras.no_data hint=empty or insufficient rows")
        sys.exit(1)

    legacy_pack = by_format.pop("_LEGACY_", None)
    for fmt, (X, Y, w, feat_names) in by_format.items():
        logger.info("pipeline: train_extras processing format=%s n=%s", fmt, X.shape[0])
        train_and_save(X, Y, out_dir, fmt, feat_names, sample_weight=w)
        logger.info("train_extras.saved format=%s n=%s out_dir=%s", fmt, X.shape[0], out_dir)

    if legacy_pack is not None:
        all_X, all_Y, all_weights, legacy_feat_names = legacy_pack
        if all_X.shape[0] >= MIN_SAMPLES_FOR_LEGACY:
            train_and_save_legacy(all_X, all_Y, out_dir, legacy_feat_names, sample_weight=all_weights)


if __name__ == "__main__":
    main()
