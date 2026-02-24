"""
Train fielding model (catches, run_outs, stumpings) from go-app export CSV or training-data API.

Same pipeline as batting/bowling: when no --cutoff/--go-app-url, reads from GO_APP_OUTPUT_DIR
fielding_encoded_all.csv (and per-format fielding_encoded_<FMT>.csv when present). With
--cutoff and --go-app-url, fetches from GET .../api/backtest/training-data?format=all&cutoff=...
Saves fielding_scaler_<FMT>.joblib and fielding_model_<FMT>.joblib to artifacts dir.

Usage:
  python -m ml.train_fielding  # use fielding_encoded_all.csv from GO_APP_OUTPUT_DIR (after export)
  GO_APP_URL=... python -m ml.train_fielding --cutoff 2024-12-01T00:00:00Z  # fetch from API
  python -m ml.train_fielding --csv path/to/fielding_export.csv
"""

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

import joblib
import numpy as np
import pandas as pd
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

# Add parent so ml.config and app.train_on_the_fly are importable
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from ml.config import (
    default_artifacts_dir,
    default_go_app_export_dir,
    get_training_data_fetch_timeout_sec,
    get_training_params,
)
from ml.utils import make_base_estimator

logger = logging.getLogger(__name__)

FIELDING_FEATURE_COLS = [
    "fielding_consistency",
    "fielding_form",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
    "inning",
    "toss",
    "fielding_venue",
    "fielding_opposition",
    "season_id",
]
FIELDING_TARGET_COLS = ["catches", "run_outs", "stumpings"]


def fetch_fielding_data(go_app_url: str, cutoff_iso: str, api_key=None):
    """Fetch training data from go-app; return dict with fielding headers and rows."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format=all&cutoff={urllib.parse.quote(cutoff_iso)}"
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
            "Go-app may have closed the connection before the response finished (e.g. server write timeout). "
            "Increase go-app server.http_write_timeout_sec (e.g. 600) in go-app/config.json and restart go-app."
        )
        if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
            raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
        raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
    return data.get("fielding") or {"headers": [], "rows": []}


def rows_to_xy_by_format(headers: list[str], rows: list[list[str]]) -> dict[str, tuple[np.ndarray, np.ndarray]]:
    """Build X, Y per format_code. Returns dict format_code -> (X, Y)."""
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    for c in FIELDING_FEATURE_COLS + FIELDING_TARGET_COLS:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    # Impute missing feature values with 0 (align with train_on_the_fly dropna or fillna strategy)
    for c in FIELDING_FEATURE_COLS:
        if c in df.columns:
            df[c] = df[c].fillna(0.0)
    if "format_code" not in df.columns:
        # Single format: use "_ALL_" as key
        df = df.dropna(subset=[c for c in FIELDING_FEATURE_COLS if c in df.columns])
        if df.empty:
            return {}
        X = df[[c for c in FIELDING_FEATURE_COLS if c in df.columns]].astype(float).values
        Y = df[[c for c in FIELDING_TARGET_COLS if c in df.columns]].astype(float).values
        return {"_ALL_": (X, Y)}
    out = {}
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in FIELDING_FEATURE_COLS if c in g.columns])
        if g.empty:
            continue
        X = g[[c for c in FIELDING_FEATURE_COLS if c in g.columns]].astype(float).values
        Y = g[[c for c in FIELDING_TARGET_COLS if c in g.columns]].astype(float).values
        if X.shape[0] < 10:
            continue
        out[fmt] = (X, Y)
    return out


def train_and_save(
    X: np.ndarray,
    Y: np.ndarray,
    out_dir: str,
    format_code: str,
) -> None:
    """Train fielding model and save scaler + model for format_code.

    Input normalization (StandardScaler) on X only; targets Y in raw units.
    See docs/ml-and-training.md.
    """
    params = get_training_params("fielding", format_code)
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    base = make_base_estimator(params)
    model = MultiOutputRegressor(base)
    model.fit(Xs, Y)

    # Extract and store feature importance (average across MultiOutputRegressor estimators)
    feature_importance = None
    if hasattr(model, "estimators_") and len(model.estimators_) > 0:
        imps = []
        for est in model.estimators_:
            if hasattr(est, "feature_importances_"):
                imps.append(est.feature_importances_)
        if imps:
            feature_importance = {
                FIELDING_FEATURE_COLS[i]: float(np.mean([arr[i] for arr in imps]))
                for i in range(min(len(FIELDING_FEATURE_COLS), len(imps[0])))
            }
            top = sorted(feature_importance.items(), key=lambda x: -x[1])[:5]
            logger.info("train_fielding.feature_importance_top5 %s", top)

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


def train_and_save_legacy(X: np.ndarray, Y: np.ndarray, out_dir: str) -> None:
    """Train one unified fielding model on all data and save as legacy (fielding_scaler.joblib, fielding_model.joblib)."""
    params = get_training_params("fielding", None)
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    base = make_base_estimator(params)
    model = MultiOutputRegressor(base)
    model.fit(Xs, Y)
    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    joblib.dump(scaler, os.path.join(out_dir, "fielding_scaler.joblib"), compress=compress)
    joblib.dump(model, os.path.join(out_dir, "fielding_model.joblib"), compress=compress)
    logger.info("train_fielding.saved_unified out_dir=%s rows=%s", out_dir, X.shape[0])


def main() -> None:
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

    if args.csv:
        if not os.path.isfile(args.csv):
            logger.error("train_fielding.csv_not_found path=%s", args.csv)
            sys.exit(1)
        logger.info("train_fielding.loading_csv path=%s", args.csv)
        df = pd.read_csv(args.csv)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format = rows_to_xy_by_format(headers, rows)
    elif args.go_app_url and args.cutoff:
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
        # Same as batting/bowling: read from export dir (run export-dataset first)
        csv_path = os.path.join(default_csv_dir, "fielding_encoded_all.csv")
        if not os.path.isfile(csv_path):
            logger.error(
                "train_fielding.csv_not_found path=%s hint=Run export-dataset first (pipeline or make export-dataset)",
                csv_path,
            )
            sys.exit(1)
        logger.info("train_fielding.loading_csv path=%s", csv_path)
        df = pd.read_csv(csv_path)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format = rows_to_xy_by_format(headers, rows)

    if not by_format:
        logger.error("train_fielding.no_data hint=empty or insufficient rows")
        sys.exit(1)

    formats_items = list(by_format.items())
    max_workers = min(
        len(formats_items),
        max(1, int(os.environ.get("ML_TRAIN_FORMAT_WORKERS", "4"))),
    )

    def _train_one_format(item):
        fmt, (X, Y) = item
        train_and_save(X, Y, out_dir, fmt)
        logger.info("train_fielding.saved format=%s n=%s out_dir=%s", fmt, X.shape[0], out_dir)

    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        list(executor.map(_train_one_format, formats_items))

    # Unified (overall) model: train on all data combined for legacy/fallback
    all_X = np.vstack([X for _, (X, _) in by_format.items()])
    all_Y = np.vstack([Y for _, (_, Y) in by_format.items()])
    del by_format
    gc.collect()
    if all_X.shape[0] >= 10:
        train_and_save_legacy(all_X, all_Y, out_dir)


if __name__ == "__main__":
    main()
