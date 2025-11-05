import os

import joblib
import pandas as pd
from dataset_definitions import input_batting_columns, output_batting_columns
from sklearn import preprocessing
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor

# Load config defaults (with env override support)
import config as svc_config

regr = RandomForestRegressor(max_depth=100, n_estimators=100, max_features="auto", random_state=0)
mltreg = MultiOutputRegressor(regr)
predictor = mltreg

# Resolve dataset and artifacts dirs
default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
dataset_source = os.path.join(default_csv_dir, "batting_encoded.csv")

input_data = pd.read_csv(dataset_source)
training_input_columns = input_batting_columns.copy()
training_input_columns.remove("player_name")

X = input_data[training_input_columns]
y = input_data[output_batting_columns]  # Labels

input_scaler = preprocessing.StandardScaler().fit(X)
input_data_scaled = input_scaler.transform(X)
X = pd.DataFrame(data=input_data_scaled, columns=X.columns)

output_scaler = preprocessing.StandardScaler().fit(y)
output_data_scaled = output_scaler.transform(y)
y = pd.DataFrame(data=output_data_scaled, columns=y.columns)

predictor.fit(X, y)

# Save the trained model and scalers
output_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())
os.makedirs(output_dir, exist_ok=True)
joblib.dump(predictor, os.path.join(output_dir, "batting_model.joblib"))
joblib.dump(input_scaler, os.path.join(output_dir, "batting_scaler.joblib"))
joblib.dump(output_scaler, os.path.join(output_dir, "batting_output_scaler.joblib"))
