import os
import sys

import joblib
import pandas as pd
from dataset_definitions import input_bowling_columns, output_bowling_columns
from sklearn import preprocessing
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor

# Load config defaults (with env override support)
import config as svc_config

# Add parent directory to path to allow importing config
parent_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if parent_dir not in sys.path:
    sys.path.insert(0, parent_dir)

# Resolve dataset and artifacts dirs
export_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
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

regr = RandomForestRegressor(max_depth=100, n_estimators=200, random_state=0)
predictor = MultiOutputRegressor(regr)
predictor.fit(X, y)

# Save the trained model and scalers
output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())
os.makedirs(output_dir, exist_ok=True)
joblib.dump(predictor, os.path.join(output_dir, "bowling_model.joblib"))
joblib.dump(input_scaler, os.path.join(output_dir, "bowling_scaler.joblib"))
joblib.dump(output_scaler, os.path.join(output_dir, "bowling_output_scaler.joblib"))
