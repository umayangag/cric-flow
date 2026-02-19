import os

# Load config defaults (with env override support)
import config as svc_config
import joblib
import pandas as pd
from dataset_definitions import output_batting_columns

output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

predictor = joblib.load(os.path.join(output_dir, "batting_model.joblib"))
input_scaler = joblib.load(os.path.join(output_dir, "batting_scaler.joblib"))
# Optional: legacy artifacts may include output_scaler; we train in raw Y now (see docs/ml-and-training.md)
_output_scaler_path = os.path.join(output_dir, "batting_output_scaler.joblib")
output_scaler = joblib.load(_output_scaler_path) if os.path.isfile(_output_scaler_path) else None


def calculate_strike_rate(row):
    if row["balls_faced"] == 0:
        return 0
    return row["runs_scored"] * 100 / row["balls_faced"]


def predict_batting(dataset):
    scaled_dataset = input_scaler.transform(dataset)
    predicted = predictor.predict(scaled_dataset)
    if output_scaler is not None:
        predicted = output_scaler.inverse_transform(predicted)
    result = pd.DataFrame(predicted, columns=output_batting_columns)
    for column in output_batting_columns:
        dataset[column] = result[column]
    dataset["strike_rate"] = dataset.apply(lambda row: calculate_strike_rate(row), axis=1)
    return dataset
