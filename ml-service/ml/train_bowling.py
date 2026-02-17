import argparse
import json
import logging
import os
from typing import Optional

import config as svc_config  # loaded from ml-service/config.json if present
import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import GradientBoostingRegressor, RandomForestRegressor, StackingRegressor
from sklearn.linear_model import Ridge
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

from .config import get_training_params

logger = logging.getLogger(__name__)

# Minimal training script to produce placeholder artifacts for bowling
# Supports training per-format; artifacts saved with format suffixes when provided.
# By default consumes the Go export from ../../output/go-app/bowling_encoded.csv
# or bowling_encoded_<FORMAT>.csv when --format is set.
# Feature order must match ml-service/app/main.py -> _bowling_feature_vector

FEATURE_COLS = [
    "bowling_consistency",
    "bowling_form",
    "bowling_form_short",
    "bowling_form_long",
    "bowling_momentum",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
    "inning",  # batting_inning in contracts; export column is inning
    "bowling_session",
    "toss",
    "bowling_venue",
    "bowling_opposition",
    "season_id",
]

TARGET_COLS = [
    "runs",  # runs_conceded
    "balls",  # deliveries
    "wickets",  # wickets_taken
    # econ may be absent; derive if missing
]


def load_dataset(path: str):
    if not os.path.exists(path):
        logger.error("train_bowling.load_dataset.file_not_found path=%s", path)
        raise FileNotFoundError(path)
    df = pd.read_csv(path)
    col_map = {
        "temp": "temp",
        "wind": "wind",
        "rain": "rain",
        "humidity": "humidity",
        "cloud": "cloud",
        "pressure": "pressure",
        "viscosity": "viscosity",
        "inning": "inning",
        "bowling_session": "bowling_session",
        "toss": "toss",
        "bowling_venue": "bowling_venue",
        "bowling_opposition": "bowling_opposition",
        "season_id": "season_id",
        "bowling_consistency": "bowling_consistency",
        "bowling_form": "bowling_form",
        "bowling_form_short": "bowling_form_short",
        "bowling_form_long": "bowling_form_long",
        "bowling_momentum": "bowling_momentum",
        "runs": "runs",
        "balls": "balls",
        "wickets": "wickets",
        "econ": "econ",
    }
    df = df.rename(columns=col_map)
    # Backward compat: fill new columns from old exports
    for col in ("bowling_form_short", "bowling_form_long"):
        if col not in df.columns and "bowling_form" in df.columns:
            df[col] = df["bowling_form"]
    if "bowling_momentum" not in df.columns:
        df["bowling_momentum"] = 0.0
    df = df.dropna(subset=[c for c in FEATURE_COLS if c in df.columns])
    X = df[FEATURE_COLS].astype(float).values
    y_cols = [c for c in TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    # Ensure econ column present or derive: econ = runs / (overs)
    if "econ" in df.columns:
        econ = df["econ"].astype(float).values.reshape(-1, 1)
    else:
        # Approximate econ from runs and balls: overs = balls/6
        runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
        balls = df.get("balls", pd.Series(np.ones(len(df)) * 6)).astype(float).values
        overs = np.where(balls > 0, balls / 6.0, 1.0)
        econ = np.where(overs > 0, runs / overs, 0.0).reshape(-1, 1)
    # Pad missing target columns
    needed = len(TARGET_COLS)
    if Y.shape[1] < needed:
        pad = np.zeros((Y.shape[0], needed - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    # Append econ to make 4 outputs
    Y = np.concatenate([Y, econ], axis=1)
    return X, Y


def train_and_save(
    X,
    Y,
    out_dir: str,
    training_params: dict,
    suffix: Optional[str] = None,
    metadata: Optional[dict] = None,
):
    """Train and save artifacts. training_params must come from get_training_params("bowling") (config only).

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
    estimator_type = training_params.get("estimator", "rf")
    learning_rate = training_params.get("learning_rate", 0.1)
    quantile_level = training_params.get("quantile_level", 0.5)
    if estimator_type == "quantile":
        base_est = GradientBoostingRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            learning_rate=learning_rate,
            loss="quantile",
            alpha=quantile_level,
        )
    elif estimator_type == "stacked":
        rf = RandomForestRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            n_jobs=n_jobs,
        )
        gb = GradientBoostingRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            learning_rate=learning_rate,
        )
        base_est = StackingRegressor(
            estimators=[("rf", rf), ("gb", gb)],
            final_estimator=Ridge(alpha=1.0, random_state=random_state),
        )
    elif estimator_type == "gb":
        base_est = GradientBoostingRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            learning_rate=learning_rate,
        )
    else:
        base_est = RandomForestRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            n_jobs=n_jobs,
        )
    model = MultiOutputRegressor(base_est)
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
                FEATURE_COLS[i]: float(np.mean([arr[i] for arr in imps]))
                for i in range(min(len(FEATURE_COLS), len(imps[0])))
            }
            top = sorted(feature_importance.items(), key=lambda x: -x[1])[:5]
            logger.info("train_bowling.feature_importance_top5 %s", top)

    if suffix:
        joblib.dump(scaler, os.path.join(out_dir, f"bowling_scaler_{suffix}.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, f"bowling_model_{suffix}.joblib"), compress=compress)
    else:
        joblib.dump(scaler, os.path.join(out_dir, "bowling_scaler.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, "bowling_model.joblib"), compress=compress)
    # Save training metadata if provided (include feature importance)
    if metadata is not None:
        if feature_importance is not None:
            metadata["feature_importance"] = feature_importance
        meta_path = os.path.join(out_dir, f"bowling_metadata_{suffix or 'LEGACY'}.json")
        try:
            with open(meta_path, "w", encoding="utf-8") as f:
                json.dump(metadata, f, indent=2)
        except OSError as e:
            logger.warning("train_bowling.train_and_save.metadata_save_failed path=%s error=%s", meta_path, e)


def _config_formats() -> list[str]:
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
    # Default input CSV from GO_APP_OUTPUT_DIR or config.json
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
    # Default output dir from ML_SERVICE_OUTPUT_DIR or config.json
    default_out_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

    parser.add_argument(
        "--csv",
        default="",
        help="Path to bowling CSV (overrides format-based resolution)",
    )
    parser.add_argument(
        "--out",
        default=default_out_dir,
        help="Output dir for artifacts (default from ML_SERVICE_OUTPUT_DIR or ../../output/ml-service)",
    )
    parser.add_argument(
        "--format",
        default="",
        help="Single format code (e.g., ODI, T20I). When set, reads bowling_encoded_<FORMAT>.csv.",
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

    # All training parameters from config (ml.training.bowling); no env overrides or magic values
    training_params = get_training_params("bowling")

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
            f for f in cfg_fmts if os.path.exists(os.path.join(default_csv_dir, f"bowling_encoded_{f}.csv"))
        ]
        if existing_cfg_fmts:
            targets = existing_cfg_fmts
        else:
            # 2) Otherwise, glob for bowling_encoded_*.csv in the export directory
            try:
                for name in os.listdir(default_csv_dir):
                    if name.startswith("bowling_encoded_") and name.endswith(".csv"):
                        suffix = name[len("bowling_encoded_") : -len(".csv")]
                        if suffix:
                            targets.append(str(suffix).upper())
            except Exception:
                pass

    # If still no targets detected, fall back to legacy single CSV path
    if not targets:
        csv_path = args.csv or os.path.join(default_csv_dir, "bowling_encoded.csv")
        try:
            X, Y = load_dataset(csv_path)
        except FileNotFoundError as e:
            logger.error("train_bowling.legacy_csv_not_found path=%s error=%s", csv_path, e)
            raise SystemExit(1) from e
        if X.size == 0 or Y.size == 0:
            logger.error("train_bowling.no_data path=%s", csv_path)
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
        logger.info("train_bowling.saved_legacy out_dir=%s", args.out)
        return

    for fmt in targets:
        csv_path = args.csv or os.path.join(default_csv_dir, f"bowling_encoded_{fmt}.csv")
        if not os.path.exists(csv_path):
            logger.warning("train_bowling.skip_format_csv_not_found format=%s path=%s", fmt, csv_path)
            continue
        try:
            X, Y = load_dataset(csv_path)
        except Exception as e:
            logger.error("train_bowling.load_dataset_failed format=%s path=%s error=%s", fmt, csv_path, e)
            continue
        if X.size == 0 or Y.size == 0:
            logger.warning("train_bowling.skip_format_no_data format=%s path=%s", fmt, csv_path)
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
        logger.info("train_bowling.saved_format format=%s out_dir=%s rows=%s", fmt, args.out, int(X.shape[0]))


if __name__ == "__main__":
    main()
