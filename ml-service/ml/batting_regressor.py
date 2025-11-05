import os

import joblib
import pandas as pd
from dataset_definitions import output_batting_columns

# Load config defaults (with env override support)
import config as svc_config

output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

predictor = joblib.load(os.path.join(output_dir, "batting_model.joblib"))
input_scaler = joblib.load(os.path.join(output_dir, "batting_scaler.joblib"))
output_scaler = joblib.load(os.path.join(output_dir, "batting_output_scaler.joblib"))


def calculate_strike_rate(row):
    if row["balls_faced"] == 0:
        return 0
    return row["runs_scored"] * 100 / row["balls_faced"]


def predict_batting(dataset):
    scaled_dataset = input_scaler.transform(dataset)
    predicted = predictor.predict(scaled_dataset)
    result = pd.DataFrame(output_scaler.inverse_transform(predicted), columns=output_batting_columns)
    for column in output_batting_columns:
        dataset[column] = result[column]
    dataset["strike_rate"] = dataset.apply(lambda row: calculate_strike_rate(row), axis=1)
    return dataset
