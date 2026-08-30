"""
Train fielding model (catches, run_outs, stumpings) from go-app export CSV or training-data API.

Thin wrapper around TrainingPipeline for core training logic. The main() entrypoint
retains fielding-specific behavior: grouping by format_code from a single CSV/API response.

Usage:
  python -m ml.train_fielding                          # use fielding_encoded_all.csv
  GO_APP_URL=... python -m ml.train_fielding --cutoff 2024-12-01T00:00:00Z
  python -m ml.train_fielding --csv path/to/fielding_export.csv
"""

from __future__ import annotations

import argparse
import gc
import json
import logging
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from typing import Optional

import joblib
import numpy as np
import pandas as pd
from sklearn.multioutput import MultiOutputRegressor

from .config import (
    default_artifacts_dir,
    default_go_app_export_dir,
    get_pipeline_common_config,
    get_training_data_fetch_timeout_sec,
    get_training_params,
)
from .data_quality import drop_low_variance_columns
from .export_csv import read_export_csv
from .pipeline_common import compute_time_decay_weights, get_scaler
from .training_pipeline import ModelSpec, TrainingPipeline
from .training_progress import columns_dropped
from .utils import make_base_estimator

logger = logging.getLogger(__name__)

FIELDING_FEATURE_COLS = [
    "fielding_consistency",
    "fielding_form",
    "inning",
    "toss",
    "fielding_venue",
    "fielding_opposition",
    "match_month_sin",
    "match_month_cos",
    "match_day_of_week_sin",
    "match_day_of_week_cos",
]
FIELDING_TARGET_COLS = ["catches", "run_outs", "stumpings"]

# ── ModelSpec (used by auto_tune and other consumers) ────────────────────

FIELDING_SPEC = ModelSpec(
    name="fielding",
    feature_cols=FIELDING_FEATURE_COLS,
    target_cols=FIELDING_TARGET_COLS,
    seq_cols=[],
    artifact_prefix="fielding",
    api_section="fielding",
    target_names_for_clip=FIELDING_TARGET_COLS,
    use_scaler=True,
    use_multi_output=True,
)


# ── Fielding-specific helpers (format grouping) ──────────────────────────


def fetch_fielding_data(go_app_url: str, cutoff_iso: str, api_key=None):
    """Fetch training data from go-app; return dict with fielding headers and rows."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format=all&cutoff={urllib.parse.quote(cutoff_iso)}&sections=fielding"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=get_training_data_fetch_timeout_sec()) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        logger.error(
            "train_fielding.fetch_fielding_data.http_error url=%s code=%s body_preview=%s",
            url,
            e.code,
            (body[:200] + "..." if len(body) > 200 else body),
        )
        raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
    except OSError as e:
        err_msg = str(e).strip()
        logger.error("train_fielding.fetch_fielding_data.os_error url=%s error=%s", url, e)
        hint = (
            "Go-app may have closed the connection before the response finished. "
            "Increase go-app server.http_write_timeout_sec in go-app/config.json."
        )
        if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
            raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
        raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
    return data.get("fielding") or {"headers": [], "rows": []}


def rows_to_xy_by_format(
    headers: list[str], rows: list[list[str]]
) -> dict[str, tuple[np.ndarray, np.ndarray, Optional[np.ndarray], list[str]]]:
    """Build X, Y, weights, feature names per format_code.

    Returns dict format_code -> (X, Y, sample_weight, feature_column_names).
    ``feature_column_names`` matches ``X.shape[1]`` after scale-aware low-variance
    drops (aligned with ``train_extras`` / ``train_win``).
    """
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    for c in FIELDING_FEATURE_COLS + FIELDING_TARGET_COLS:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    for c in FIELDING_FEATURE_COLS:
        if c in df.columns:
            df[c] = df[c].fillna(0.0)
    pipe_cfg = get_pipeline_common_config()
    halflife = pipe_cfg.get("time_decay_halflife_years", 2.0)

    def _weights(g: pd.DataFrame) -> Optional[np.ndarray]:
        if "match_date" not in g.columns:
            return None
        return compute_time_decay_weights(g["match_date"], halflife_years=halflife)

    if "format_code" not in df.columns:
        df = df.dropna(subset=[c for c in FIELDING_FEATURE_COLS if c in df.columns])
        if df.empty:
            return {}
        feat_cols = [c for c in FIELDING_FEATURE_COLS if c in df.columns]
        X = df[feat_cols].astype(float).values
        X, feat_cols, dropped = drop_low_variance_columns(X, feat_cols)
        columns_dropped("fielding", None, dropped, len(feat_cols))
        Y = df[[c for c in FIELDING_TARGET_COLS if c in df.columns]].astype(float).values
        w = _weights(df)
        return {"_ALL_": (X, Y, w, feat_cols)}
    out: dict[str, tuple[np.ndarray, np.ndarray, Optional[np.ndarray], list[str]]] = {}
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in FIELDING_FEATURE_COLS if c in g.columns])
        if g.empty:
            continue
        feat_cols = [c for c in FIELDING_FEATURE_COLS if c in g.columns]
        X = g[feat_cols].astype(float).values
        X, feat_cols, dropped = drop_low_variance_columns(X, feat_cols)
        columns_dropped("fielding", fmt, dropped, len(feat_cols))
        Y = g[[c for c in FIELDING_TARGET_COLS if c in g.columns]].astype(float).values
        if X.shape[0] < 10:
            continue
        w = _weights(g)
        out[fmt] = (X, Y, w, feat_cols)
    return out


def train_and_save(
    X: np.ndarray,
    Y: np.ndarray,
    out_dir: str,
    format_code: str,
    feature_names: list[str],
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train fielding model and save scaler + model for format_code.

    Uses TrainingPipeline internally for scaling, fitting, and feature importance extraction.
    """
    if X.shape[1] != len(feature_names):
        raise ValueError(
            f"train_fielding.train_and_save.feature_mismatch X.shape[1]={X.shape[1]} names={len(feature_names)}"
        )
    params = get_training_params("fielding", format_code)
    pipe_cfg = get_pipeline_common_config()
    scaler = get_scaler(use_robust=pipe_cfg.get("use_robust_scaler", True))
    Xs = scaler.fit_transform(X)
    base = make_base_estimator(params)
    model = MultiOutputRegressor(base)
    if sample_weight is not None:
        model.fit(Xs, Y, sample_weight=sample_weight)
    else:
        model.fit(Xs, Y)

    # Extract feature importance
    feature_importance = TrainingPipeline.extract_feature_importance(model, feature_names)

    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    code = format_code.replace(" ", "_")
    joblib.dump(scaler, os.path.join(out_dir, f"fielding_scaler_{code}.joblib"), compress=compress)
    joblib.dump(model, os.path.join(out_dir, f"fielding_model_{code}.joblib"), compress=compress)
    if feature_importance is not None:
        try:
            with open(os.path.join(out_dir, f"fielding_metadata_{code}.json"), "w", encoding="utf-8") as f:
                json.dump({"feature_importance": feature_importance}, f, indent=2)
        except OSError as e:
            logger.warning("train_fielding.metadata_save_failed path=%s error=%s", out_dir, e)


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    ap = argparse.ArgumentParser(
        description="Train fielding model from go-app export CSV or training-data API (same pipeline as batting/bowling)"
    )
    ap.add_argument(
        "--cutoff", default="", help="RFC3339 cutoff for API fetch (optional; if unset, use CSV from export dir)"
    )
    ap.add_argument(
        "--csv",
        default="",
        help="Path to fielding CSV (optional; else use GO_APP_OUTPUT_DIR/fielding_encoded_all.csv or API)",
    )
    ap.add_argument("--out", default="", help="Artifacts output dir (default from config)")
    ap.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""), help="Go-app base URL for API fetch")
    ap.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""), help="Optional API key")
    args = ap.parse_args()
    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR") or default_artifacts_dir()
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR") or default_go_app_export_dir()

    csv_path = args.csv or os.path.join(default_csv_dir, "fielding_encoded_all.csv")

    if os.path.isfile(csv_path):
        logger.info("train_fielding.loading_csv path=%s (prefer CSV over API)", csv_path)
        # The cutoff governs the CSV path too. Without it a run asked to train to a
        # cutoff trained on every exported match, leaving no holdout to evaluate on.
        df = read_export_csv(csv_path, cutoff=args.cutoff or None)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format = rows_to_xy_by_format(headers, rows)
    elif args.go_app_url and args.cutoff:
        logger.warning(
            "train_fielding.csv_not_found path=%s falling_back_to_api hint=Run export-dataset first for faster training",
            csv_path,
        )
        logger.info("train_fielding.fetching_api go_app_url=%s cutoff=%s", args.go_app_url, args.cutoff)
        try:
            field = fetch_fielding_data(args.go_app_url, args.cutoff, args.api_key or None)
        except ValueError as e:
            logger.error("train_fielding.fetch_failed error=%s", e)
            sys.exit(1)
        headers = field.get("headers") or []
        rows = field.get("rows") or []
        by_format = rows_to_xy_by_format(headers, rows)
    else:
        logger.error(
            "train_fielding.csv_not_found path=%s hint=Run export-dataset first (pipeline or make export-dataset), or provide --go-app-url and --cutoff for API fallback",
            csv_path,
        )
        sys.exit(1)

    if not by_format:
        logger.error("train_fielding.no_data hint=empty or insufficient rows")
        sys.exit(1)

    formats_items = list(by_format.items())
    max_workers = min(
        len(formats_items),
        max(1, int(os.environ.get("ML_TRAIN_FORMAT_WORKERS", "4"))),
    )

    def _train_one_format(item):
        fmt, (X, Y, w, feat_names) = item
        logger.info("pipeline: train_fielding processing format=%s n=%s", fmt, X.shape[0])
        train_and_save(X, Y, out_dir, fmt, feat_names, sample_weight=w)
        logger.info("train_fielding.saved format=%s n=%s out_dir=%s", fmt, X.shape[0], out_dir)

    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        list(executor.map(_train_one_format, formats_items))

    del by_format
    gc.collect()


if __name__ == "__main__":
    main()
