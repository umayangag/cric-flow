import argparse
import os

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

import config as svc_config  # loaded from ml-service/config.json if present

# Minimal training script to produce placeholder artifacts compatible with app.main
# By default consumes the Go export from ../../output/go-app/batting_encoded.csv
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


def train_and_save(X, Y, out_dir: str):
    os.makedirs(out_dir, exist_ok=True)
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    model = MultiOutputRegressor(RandomForestRegressor(n_estimators=100, random_state=42))
    model.fit(Xs, Y)
    joblib.dump(scaler, os.path.join(out_dir, "batting_scaler.joblib"))
    joblib.dump(model, os.path.join(out_dir, "batting_model.joblib"))


def main():
    parser = argparse.ArgumentParser()
    # Default input CSV from GO_APP_OUTPUT_DIR or ../../output/go-app
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
    default_csv = os.path.join(default_csv_dir, "batting_encoded.csv")
    # Default output dir from ML_SERVICE_OUTPUT_DIR or config
    default_out_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

    parser.add_argument(
        "--csv",
        default=default_csv,
        help="Path to batting CSV (default from GO_APP_OUTPUT_DIR or ../../output/go-app)",
    )
    parser.add_argument(
        "--out",
        default=default_out_dir,
        help="Output dir for artifacts (default from ML_SERVICE_OUTPUT_DIR or ../../output/ml-service)",
    )
    args = parser.parse_args()

    X, Y = load_dataset(args.csv)
    if X.size == 0 or Y.size == 0:
        print("No data found for training. Exiting.")
        return
    train_and_save(X, Y, args.out)
    print(f"Saved batting artifacts to {args.out}")


if __name__ == "__main__":
    main()
