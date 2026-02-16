import os

import joblib
import pandas as pd
from sklearn import preprocessing
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor

from . import tracking
from .config import default_artifacts_dir, default_go_app_export_dir, get_training_params
from .dataset_definitions import input_bowling_columns, output_bowling_columns


def run_training():
    # Resolve dataset and artifacts dirs
    export_dir = os.environ.get("GO_APP_OUTPUT_DIR", default_go_app_export_dir())
    unified = os.path.join(export_dir, "bowling_encoded_all.csv")
    legacy = os.path.join(export_dir, "bowling_encoded.csv")
    dataset_source = unified if os.path.exists(unified) else legacy

    input_data = pd.read_csv(dataset_source)

    using_unified = os.path.basename(dataset_source) == "bowling_encoded_all.csv"

    if using_unified:
        y_cols = [
            c for c in ["runs", "balls", "wickets"] if c in input_data.columns or c in ["runs", "deliveries", "wickets"]
        ]
        # Handle column naming differences: exporter uses runs, balls (bowling has runs conceded, balls delivered)
        if "deliveries" in input_data.columns:
            y_cols = ["runs", "deliveries", "wickets"]
        base_cols = [
            "temp",
            "wind",
            "rain",
            "humidity",
            "cloud",
            "pressure",
            "viscosity",
            "inning",
            "bowling_session",
            "toss",
            "season_id",
        ]
        asof_cols = [c for c in input_data.columns if c.startswith("bowl_") and c.endswith("_asof")]
        fmt_cols = []
        if "format_code" in input_data.columns:
            dummies = pd.get_dummies(input_data["format_code"], prefix="fmt", dummy_na=False)
            fmt_cols = list(dummies.columns)
            input_data = pd.concat([input_data.drop(columns=["format_code"]), dummies], axis=1)
        x_cols = [c for c in (base_cols + asof_cols + fmt_cols) if c in input_data.columns]
    else:
        train_inp = input_bowling_columns.copy()
        if "player_name" in train_inp:
            train_inp.remove("player_name")
        x_cols = train_inp
        y_cols = output_bowling_columns

    X = input_data[x_cols].copy()
    y = input_data[y_cols].copy()

    X = X.fillna(0.0)

    input_scaler = preprocessing.StandardScaler().fit(X)
    X_scaled = input_scaler.transform(X)
    X = pd.DataFrame(data=X_scaled, columns=X.columns)

    output_scaler = preprocessing.StandardScaler().fit(y)
    y_scaled = output_scaler.transform(y)
    y = pd.DataFrame(data=y_scaled, columns=y.columns)

    params = get_training_params()
    regr = RandomForestRegressor(
        max_depth=params["max_depth"],
        n_estimators=params["n_estimators"],
        random_state=params["random_state"],
    )
    predictor = MultiOutputRegressor(regr)
    predictor.fit(X, y)

    output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", default_artifacts_dir())
    os.makedirs(output_dir, exist_ok=True)
    compress = params["joblib_compress"]
    joblib.dump(
        predictor,
        os.path.join(output_dir, "bowling_model.joblib"),
        compress=compress,
    )
    joblib.dump(
        input_scaler,
        os.path.join(output_dir, "bowling_scaler.joblib"),
        compress=compress,
    )
    joblib.dump(
        output_scaler,
        os.path.join(output_dir, "bowling_output_scaler.joblib"),
        compress=compress,
    )


if __name__ == "__main__":
    with tracking.track("train-bowling", {"type": "bowling"}):
        run_training()
