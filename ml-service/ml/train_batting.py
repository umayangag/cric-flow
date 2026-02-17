import argparse
import json
import logging
import os
from typing import Optional

import config as svc_config  # loaded from ml-service/config.json if present
import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

from .config import get_training_params

logger = logging.getLogger(__name__)

# Minimal training script to produce placeholder artifacts compatible with app.main
# Supports training per-format; artifacts saved with format suffixes when provided.
# By default consumes the Go export from ../../output/go-app/batting_encoded.csv (legacy)
# or batting_encoded_<FORMAT>.csv when --format is set.
# Feature order must match ml-service/app/main.py -> _batting_feature_vector

FEATURE_COLS = [
    "batting_consistency",
    "batting_form",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
    "inning",
    "batting_session",
    "toss",
    "batting_venue",
    "batting_opposition",
    "season_id",
]

TARGET_COLS = [
    "runs",  # runs_scored
    "balls",  # balls_faced
    "fours",  # fours_scored
    "sixes",  # sixes_scored
    "batting_position",
    # strike_rate may be absent; derive if missing
]


def load_dataset(path: str):
    if not os.path.exists(path):
        logger.error("train_batting.load_dataset.file_not_found path=%s", path)
        raise FileNotFoundError(path)
    df = pd.read_csv(path)
    # Map Go export headers to expected names if needed
    col_map = {
        "temp": "temp",
        "wind": "wind",
        "rain": "rain",
        "humidity": "humidity",
        "cloud": "cloud",
        "pressure": "pressure",
        "viscosity": "viscosity",
        "inning": "inning",
        "batting_session": "batting_session",
        "toss": "toss",
        "batting_venue": "batting_venue",
        "batting_opposition": "batting_opposition",
        "season_id": "season_id",
        "batting_consistency": "batting_consistency",
        "batting_form": "batting_form",
        "runs": "runs",
        "balls": "balls",
        "fours": "fours",
        "sixes": "sixes",
        "batting_position": "batting_position",
        "strike_rate": "strike_rate",
    }
    df = df.rename(columns=col_map)
    # Filter rows with required feature columns
    df = df.dropna(subset=[c for c in FEATURE_COLS if c in df.columns])
    X = df[FEATURE_COLS].astype(float).values
    # Build Y with up to 6 outputs (pad strike_rate if missing)
    y_cols = [c for c in TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    # Add strike_rate column if present; else derive from runs/balls
    if "strike_rate" in df.columns:
        sr = df["strike_rate"].astype(float).values.reshape(-1, 1)
    else:
        # Avoid division by zero: sr = (runs/balls)*100 if balls>0 else 0
        runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
        balls = df.get("balls", pd.Series(np.ones(len(df)))).astype(float).values
        sr = np.where(balls > 0, (runs / balls) * 100.0, 0.0).reshape(-1, 1)
    # Ensure Y has 5 columns (TARGET_COLS) then append sr to make 6
    # If some target columns are missing, pad with zeros
    needed = len(TARGET_COLS)
    if Y.shape[1] < needed:
        pad = np.zeros((Y.shape[0], needed - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    Y = np.concatenate([Y, sr], axis=1)
    return X, Y


def train_and_save(
    X,
    Y,
    out_dir: str,
    training_params: dict,
    suffix: Optional[str] = None,
    metadata: Optional[dict] = None,
):
    """Train and save artifacts. training_params must come from get_training_params("batting") (config only).

    Normalizes X with StandardScaler (fit on provided data); Y kept in raw units.
    See docs/ML_DATA_AND_NORMALIZATION.md.
    """
    os.makedirs(out_dir, exist_ok=True)
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    n_estimators = training_params["n_estimators"]
    max_depth = training_params["max_depth"]
    random_state = training_params["random_state"]
    compress = training_params["joblib_compress"]
    n_jobs = training_params.get("n_jobs", -1)
    model = MultiOutputRegressor(
        RandomForestRegressor(
            n_estimators=n_estimators,
            random_state=random_state,
            max_depth=max_depth,
            n_jobs=n_jobs,
        )
    )
    model.fit(Xs, Y)
    if suffix:
        joblib.dump(scaler, os.path.join(out_dir, f"batting_scaler_{suffix}.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, f"batting_model_{suffix}.joblib"), compress=compress)
    else:
        joblib.dump(scaler, os.path.join(out_dir, "batting_scaler.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, "batting_model.joblib"), compress=compress)
    # Save training metadata if provided
    if metadata is not None:
        meta_path = os.path.join(out_dir, f"batting_metadata_{suffix or 'LEGACY'}.json")
        try:
            with open(meta_path, "w", encoding="utf-8") as f:
                json.dump(metadata, f, indent=2)
        except OSError as e:
            logger.warning("train_batting.train_and_save.metadata_save_failed path=%s error=%s", meta_path, e)


def _config_formats() -> list[str]:
    # Try to read ml.formats from ml-service/config.json via raw JSON
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f)
            fmts = data.get("ml", {}).get("formats") or []
            return [str(x).upper() for x in fmts if isinstance(x, (str, int))]
    except Exception:
        return []


def main():
    parser = argparse.ArgumentParser()
    # Default input CSV from GO_APP_OUTPUT_DIR or ../../output/go-app
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
    default_out_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

    parser.add_argument(
        "--csv",
        default="",
        help="Path to batting CSV (overrides format-based resolution)",
    )
    parser.add_argument(
        "--out",
        default=default_out_dir,
        help="Output dir for artifacts (default from ML_SERVICE_OUTPUT_DIR or ../../output/ml-service)",
    )
    parser.add_argument(
        "--format",
        default="",
        help="Single format code (e.g., ODI, T20I). When set, reads batting_encoded_<FORMAT>.csv.",
    )
    parser.add_argument(
        "--formats",
        default="",
        help="Comma-separated list of formats to train. Overrides --format.",
    )
    parser.add_argument(
        "--all-formats",
        action="store_true",
        help="Train for all formats from config (ml.formats).",
    )
    args = parser.parse_args()

    # All training parameters from config (ml.training.batting); no env overrides or magic values
    training_params = get_training_params("batting")

    targets: list[str] = []
    if args.all_formats:
        targets = _config_formats()
    elif args.formats:
        targets = [s.strip().upper() for s in args.formats.split(",") if s.strip()]
    elif args.format:
        targets = [args.format.strip().upper()]

    # Auto-detect formats when none explicitly provided
    if not targets and not args.csv:
        # 1) Prefer formats from config that actually exist on disk
        cfg_fmts = _config_formats()
        existing_cfg_fmts = [
            f for f in cfg_fmts if os.path.exists(os.path.join(default_csv_dir, f"batting_encoded_{f}.csv"))
        ]
        if existing_cfg_fmts:
            targets = existing_cfg_fmts
        else:
            # 2) Otherwise, glob for batting_encoded_*.csv in the export directory
            try:
                for name in os.listdir(default_csv_dir):
                    if name.startswith("batting_encoded_") and name.endswith(".csv"):
                        suffix = name[len("batting_encoded_") : -len(".csv")]
                        if suffix:
                            targets.append(str(suffix).upper())
            except Exception:
                pass

    # If still no targets detected, fall back to legacy single CSV path
    if not targets:
        csv_path = args.csv or os.path.join(default_csv_dir, "batting_encoded.csv")
        try:
            X, Y = load_dataset(csv_path)
        except FileNotFoundError as e:
            logger.error("train_batting.legacy_csv_not_found path=%s error=%s", csv_path, e)
            raise SystemExit(1) from e
        if X.size == 0 or Y.size == 0:
            logger.error("train_batting.no_data path=%s", csv_path)
            return
        meta = {
            "csv_path": csv_path,
            "rows": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "n_targets": int(Y.shape[1]),
            "format": None,
            "model": "RandomForestRegressor",
            "hyperparams": training_params,
        }
        train_and_save(X, Y, args.out, training_params, None, meta)
        logger.info("train_batting.saved_legacy out_dir=%s", args.out)
        return

    # Per-format training loop
    for fmt in targets:
        csv_path = args.csv or os.path.join(default_csv_dir, f"batting_encoded_{fmt}.csv")
        if not os.path.exists(csv_path):
            logger.warning("train_batting.skip_format_csv_not_found format=%s path=%s", fmt, csv_path)
            continue
        try:
            X, Y = load_dataset(csv_path)
        except Exception as e:
            logger.error("train_batting.load_dataset_failed format=%s path=%s error=%s", fmt, csv_path, e)
            continue
        if X.size == 0 or Y.size == 0:
            logger.warning("train_batting.skip_format_no_data format=%s path=%s", fmt, csv_path)
            continue
        meta = {
            "csv_path": csv_path,
            "rows": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "n_targets": int(Y.shape[1]),
            "format": fmt,
            "model": "RandomForestRegressor",
            "hyperparams": training_params,
        }
        train_and_save(X, Y, args.out, training_params, fmt, meta)
        logger.info("train_batting.saved_format format=%s out_dir=%s rows=%s", fmt, args.out, int(X.shape[0]))


if __name__ == "__main__":
    main()
