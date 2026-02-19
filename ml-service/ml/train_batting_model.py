import os

import joblib
import pandas as pd
from sklearn import preprocessing
from sklearn.multioutput import MultiOutputRegressor

from . import tracking
from .config import default_artifacts_dir, default_go_app_export_dir, get_training_params
from .dataset_definitions import input_batting_columns, output_batting_columns
from .utils import make_base_estimator


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

    # Impute: fill missing numeric values (expected for as-of columns when no history).
    # Same strategy as train_batting/train_on_the_fly so training and prediction stay aligned.
    X = X.fillna(0.0)

    # Normalize inputs only (StandardScaler). Targets Y stay in raw units for interpretable API output.
    # See docs/ML_DATA_AND_NORMALIZATION.md.
    input_scaler = preprocessing.StandardScaler().fit(X)
    X_scaled = input_scaler.transform(X)

    # Model: all hyperparameters from config (ml.training); no magic values
    params = get_training_params("batting")
    predictor = MultiOutputRegressor(make_base_estimator(params))
    predictor.fit(X_scaled, y)

    # Save the trained model and input scaler only (joblib_compress from config).
    # No output scaler: predictions are in raw units.
    output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", default_artifacts_dir())
    os.makedirs(output_dir, exist_ok=True)
    compress = params["joblib_compress"]
    joblib.dump(
        predictor,
        os.path.join(output_dir, "batting_model.joblib"),
        compress=compress,
    )
    joblib.dump(
        input_scaler,
        os.path.join(output_dir, "batting_scaler.joblib"),
        compress=compress,
    )


def _skip_tracking() -> bool:
    """When True, caller (e.g. /admin/train/*) already owns the pipeline step; skip tracking here."""
    return os.environ.get("SKIP_PIPELINE_TRACKING", "").strip().lower() in ("1", "true", "yes")


if __name__ == "__main__":
    if _skip_tracking():
        run_training()
    else:
        with tracking.track("train-batting", {"type": "batting"}):
            run_training()
