import os

import joblib
import pandas as pd
from sklearn import preprocessing
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor

from . import tracking
from .config import default_artifacts_dir, default_go_app_export_dir
from .dataset_definitions import input_batting_columns, output_batting_columns


def run_training():
    # Resolve dataset and artifacts dirs
    export_dir = os.environ.get("GO_APP_OUTPUT_DIR", default_go_app_export_dir())
    # Prefer unified merged dataset if present; fall back to legacy
    unified = os.path.join(export_dir, "batting_encoded_all.csv")
    legacy = os.path.join(export_dir, "batting_encoded.csv")
    dataset_source = unified if os.path.exists(unified) else legacy

    input_data = pd.read_csv(dataset_source)

    # Decide columns dynamically based on dataset flavor
    using_unified = os.path.basename(dataset_source) == "batting_encoded_all.csv"

    if using_unified:
        # Labels available in unified export
        y_cols = [c for c in ["runs", "balls", "fours", "sixes", "batting_position"] if c in input_data.columns]
        # Base context inputs
        base_cols = [
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
            "season_id",
        ]
        # Per-format as-of features for batting (no manual weights)
        asof_cols = [c for c in input_data.columns if c.startswith("bat_") and c.endswith("_asof")]
        # One-hot encode format_code inline by expanding columns (keep training script simple)
        fmt_cols = []
        if "format_code" in input_data.columns:
            dummies = pd.get_dummies(input_data["format_code"], prefix="fmt", dummy_na=False)
            fmt_cols = list(dummies.columns)
            input_data = pd.concat([input_data.drop(columns=["format_code"]), dummies], axis=1)
        x_cols = [c for c in (base_cols + asof_cols + fmt_cols) if c in input_data.columns]
    else:
        # Legacy dataset path retains old contracts
        train_inp = input_batting_columns.copy()
        if "player_name" in train_inp:
            train_inp.remove("player_name")
        x_cols = train_inp
        y_cols = output_batting_columns

    # Assemble matrices
    X = input_data[x_cols].copy()
    y = input_data[y_cols].copy()

    # Impute/normalize: fill missing numeric values (expected for as-of columns when no history)
    X = X.fillna(0.0)

    # Scale inputs/labels (to keep API parity)
    input_scaler = preprocessing.StandardScaler().fit(X)
    X_scaled = input_scaler.transform(X)
    X = pd.DataFrame(data=X_scaled, columns=X.columns)

    output_scaler = preprocessing.StandardScaler().fit(y)
    y_scaled = output_scaler.transform(y)
    y = pd.DataFrame(data=y_scaled, columns=y.columns)

    # Model (trees cope with expanded inputs well)
    regr = RandomForestRegressor(max_depth=100, n_estimators=200, random_state=0)
    predictor = MultiOutputRegressor(regr)
    predictor.fit(X, y)

    # Save the trained model and scalers
    output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", default_artifacts_dir())
    os.makedirs(output_dir, exist_ok=True)
    joblib.dump(predictor, os.path.join(output_dir, "batting_model.joblib"))
    joblib.dump(input_scaler, os.path.join(output_dir, "batting_scaler.joblib"))
    joblib.dump(output_scaler, os.path.join(output_dir, "batting_output_scaler.joblib"))


if __name__ == "__main__":
    with tracking.track("train-batting", {"type": "batting"}):
        run_training()
