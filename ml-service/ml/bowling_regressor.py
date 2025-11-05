import os

# Load config defaults (with env override support)
import config as svc_config
import joblib
import pandas as pd
from dataset_definitions import output_bowling_columns

output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

predictor = joblib.load(os.path.join(output_dir, "bowling_model.joblib"))
input_scaler = joblib.load(os.path.join(output_dir, "bowling_scaler.joblib"))
output_scaler = joblib.load(os.path.join(output_dir, "bowling_output_scaler.joblib"))


def calculate_econ(row):
    if row["deliveries"] == 0:
        return 0
    return row["runs_conceded"] * 6 / row["deliveries"]


def predict_bowling(dataset):
    scaled_dataset = input_scaler.transform(dataset)
    predicted = predictor.predict(scaled_dataset)
    result = pd.DataFrame(output_scaler.inverse_transform(predicted), columns=output_bowling_columns)
    for column in output_bowling_columns:
        dataset[column] = result[column]
    dataset["econ"] = dataset.apply(lambda row: calculate_econ(row), axis=1)
    return dataset
