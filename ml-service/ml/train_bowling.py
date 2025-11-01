import argparse
import os

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

# Minimal training script to produce placeholder artifacts for bowling
# By default consumes the Go export from ../../output/go-app/bowling_encoded.csv
# Feature order must match ml-service/app/main.py -> _bowling_feature_vector

FEATURE_COLS = [
    "bowling_consistency",
    "bowling_form",
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
        "runs": "runs",
        "balls": "balls",
        "wickets": "wickets",
        "econ": "econ",
    }
    df = df.rename(columns=col_map)
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


def train_and_save(X, Y, out_dir: str):
    os.makedirs(out_dir, exist_ok=True)
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    model = MultiOutputRegressor(RandomForestRegressor(n_estimators=100, random_state=42))
    model.fit(Xs, Y)
    joblib.dump(scaler, os.path.join(out_dir, "bowling_scaler.joblib"))
    joblib.dump(model, os.path.join(out_dir, "bowling_model.joblib"))


def main():
    parser = argparse.ArgumentParser()
    # Default input CSV from GO_APP_OUTPUT_DIR or ../../output/go-app
    default_csv_dir = os.environ.get(
        "GO_APP_OUTPUT_DIR", os.path.join("..", "..", "output", "go-app")
    )
    default_csv = os.path.join(default_csv_dir, "bowling_encoded.csv")
    # Default output dir from ML_SERVICE_OUTPUT_DIR or ../../output/ml-service
    default_out_dir = os.environ.get(
        "ML_SERVICE_OUTPUT_DIR", os.path.join("..", "..", "output", "ml-service")
    )

    parser.add_argument(
        "--csv",
        default=default_csv,
        help="Path to bowling CSV (default from GO_APP_OUTPUT_DIR or ../../output/go-app)",
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
    print(f"Saved bowling artifacts to {args.out}")


if __name__ == "__main__":
    main()
