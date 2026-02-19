"""
Train extras model (match-level total_extras) from go-app training-data API or CSV.

Fetches GET {GO_APP_URL}/api/backtest/training-data?format=all&cutoff=..., extracts extras
headers/rows, groups by format_code, trains one model per format, saves
extras_model_<FMT>.joblib to artifacts dir.

Usage:
  GO_APP_URL=http://localhost:8080 python -m ml.train_extras --cutoff 2024-12-01T00:00:00Z
  python -m ml.train_extras --csv path/to/extras_export.csv
"""

import argparse
import json
import logging
import os
import sys
import urllib.error
import urllib.request

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestRegressor

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from ml.config import default_artifacts_dir, get_training_data_fetch_timeout_sec, get_training_params

logger = logging.getLogger(__name__)

EXTRAS_FEATURE_COLS = ["format_id", "venue_id", "season_id"]
EXTRAS_TARGET_COL = "total_extras"


def fetch_extras_data(go_app_url: str, cutoff_iso: str, api_key=None):
    """Fetch training data from go-app; return dict with extras headers and rows."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format=all&cutoff={cutoff_iso}"
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
        logger.error("train_extras.fetch_extras_data.os_error url=%s error=%s", url, e)
        raise ValueError(f"Go-app training-data request failed: {e}") from e
    return data.get("extras") or {"headers": [], "rows": []}


def rows_to_xy_by_format(headers: list, rows: list[list]) -> dict[str, tuple[np.ndarray, np.ndarray]]:
    """Build X, Y per format_code. Returns dict format_code -> (X, Y)."""
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    for c in EXTRAS_FEATURE_COLS + [EXTRAS_TARGET_COL]:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    for c in EXTRAS_FEATURE_COLS:
        if c in df.columns:
            df[c] = df[c].fillna(0.0)
    if "format_code" not in df.columns:
        df = df.dropna(subset=[c for c in EXTRAS_FEATURE_COLS if c in df.columns] + [EXTRAS_TARGET_COL])
        if df.empty:
            return {}
        X = df[[c for c in EXTRAS_FEATURE_COLS if c in df.columns]].astype(float).values
        Y = df[EXTRAS_TARGET_COL].astype(float).values.reshape(-1, 1)
        return {"_ALL_": (X, Y)}
    out = {}
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in EXTRAS_FEATURE_COLS if c in g.columns] + [EXTRAS_TARGET_COL])
        if g.empty or len(g) < 10:
            continue
        X = g[[c for c in EXTRAS_FEATURE_COLS if c in g.columns]].astype(float).values
        Y = g[EXTRAS_TARGET_COL].astype(float).values.reshape(-1, 1)
        out[fmt] = (X, Y)
    return out


def train_and_save(X: np.ndarray, Y: np.ndarray, out_dir: str, format_code: str) -> None:
    """Train extras regressor and save model for format_code (no scaler; artifacts loader expects model only)."""
    params = get_training_params("extras", format_code)
    model = RandomForestRegressor(
        n_estimators=params["n_estimators"],
        max_depth=params["max_depth"],
        random_state=params["random_state"],
        n_jobs=params.get("n_jobs", -1),
    )
    model.fit(X, Y.ravel())
    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    code = format_code.replace(" ", "_")
    joblib.dump(model, os.path.join(out_dir, f"extras_model_{code}.joblib"), compress=compress)


def main() -> None:
    ap = argparse.ArgumentParser(description="Train extras model from go-app training-data API or CSV")
    ap.add_argument("--cutoff", default="", help="RFC3339 cutoff (required if not using --csv)")
    ap.add_argument("--csv", default="", help="Path to extras CSV (optional; else fetch from API)")
    ap.add_argument("--out", default="", help="Artifacts output dir (default from config)")
    ap.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""), help="Go-app base URL")
    ap.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""), help="Optional API key")
    args = ap.parse_args()
    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR") or default_artifacts_dir()

    if args.csv:
        if not os.path.isfile(args.csv):
            logger.error("train_extras.csv_not_found path=%s", args.csv)
            sys.exit(1)
        logger.info("train_extras.loading_csv path=%s", args.csv)
        df = pd.read_csv(args.csv)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format = rows_to_xy_by_format(headers, rows)
    else:
        if not args.go_app_url or not args.cutoff:
            logger.error("train_extras.missing_args hint=Provide --go-app-url and --cutoff, or --csv")
            sys.exit(1)
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
    for fmt, (X, Y) in by_format.items():
        train_and_save(X, Y, out_dir, fmt)
        logger.info("train_extras.saved format=%s n=%s out_dir=%s", fmt, X.shape[0], out_dir)


if __name__ == "__main__":
    main()
