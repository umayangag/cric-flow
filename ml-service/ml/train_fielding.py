"""
Train fielding model (catches, run_outs, stumpings) from go-app training-data API or CSV.

Fetches GET {GO_APP_URL}/api/backtest/training-data?format=all&cutoff=..., extracts fielding
headers/rows, groups by format_code, trains one (scaler, model) per format, saves
fielding_scaler_<FMT>.joblib and fielding_model_<FMT>.joblib to artifacts dir.

Usage:
  GO_APP_URL=http://localhost:8080 python -m ml.train_fielding --cutoff 2024-12-01T00:00:00Z
  python -m ml.train_fielding --csv path/to/fielding_export.csv  # if go-app exported CSV
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

# Add parent so ml.config and app.train_on_the_fly are importable
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from ml.config import default_artifacts_dir, get_training_params

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
    url = f"{base}/api/backtest/training-data?format=all&cutoff={cutoff_iso}"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=600) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
    except OSError as e:
        raise ValueError(f"Go-app training-data request failed: {e}") from e
    return data.get("fielding") or {"headers": [], "rows": []}


def rows_to_xy_by_format(
    headers: list[str], rows: list[list[str]]
) -> dict[str, tuple[np.ndarray, np.ndarray]]:
    """Build X, Y per format_code. Returns dict format_code -> (X, Y)."""
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    for c in FIELDING_FEATURE_COLS + FIELDING_TARGET_COLS:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
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
    """Train fielding model and save scaler + model for format_code."""
    params = get_training_params("fielding")
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    model = MultiOutputRegressor(
        RandomForestRegressor(
            n_estimators=params["n_estimators"],
            max_depth=params["max_depth"],
            random_state=params["random_state"],
        )
    )
    model.fit(Xs, Y)
    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    code = format_code.replace(" ", "_")
    joblib.dump(scaler, os.path.join(out_dir, f"fielding_scaler_{code}.joblib"), compress=compress)
    joblib.dump(model, os.path.join(out_dir, f"fielding_model_{code}.joblib"), compress=compress)


def main() -> None:
    ap = argparse.ArgumentParser(description="Train fielding model from go-app training-data API or CSV")
    ap.add_argument("--cutoff", default="", help="RFC3339 cutoff (required if not using --csv)")
    ap.add_argument("--csv", default="", help="Path to fielding CSV (optional; else fetch from API)")
    ap.add_argument("--out", default="", help="Artifacts output dir (default from config)")
    ap.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""), help="Go-app base URL")
    ap.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""), help="Optional API key")
    args = ap.parse_args()
    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR") or default_artifacts_dir()

    if args.csv:
        if not os.path.isfile(args.csv):
            print(f"CSV not found: {args.csv}", file=sys.stderr)
            sys.exit(1)
        df = pd.read_csv(args.csv)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format = rows_to_xy_by_format(headers, rows)
    else:
        if not args.go_app_url or not args.cutoff:
            print("Provide --go-app-url and --cutoff, or --csv", file=sys.stderr)
            sys.exit(1)
        field = fetch_fielding_data(args.go_app_url, args.cutoff, args.api_key or None)
        headers = field.get("headers") or []
        rows = field.get("rows") or []
        by_format = rows_to_xy_by_format(headers, rows)

    if not by_format:
        print("No fielding training data (empty or insufficient rows).", file=sys.stderr)
        sys.exit(1)
    for fmt, (X, Y) in by_format.items():
        train_and_save(X, Y, out_dir, fmt)
        print(f"Trained fielding model for format={fmt} (n={X.shape[0]}) -> {out_dir}")


if __name__ == "__main__":
    main()
