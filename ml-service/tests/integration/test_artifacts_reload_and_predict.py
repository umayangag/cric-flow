"""
Integration tests: reload artifacts from disk then run prediction through the app.

Uses real joblib-serialized scalers and small sklearn models so the full path
(reload -> BAT_MODELS populated -> predict endpoint) is exercised.
"""

import importlib
import os

import joblib
import numpy as np
import pytest
from fastapi.testclient import TestClient
from sklearn.ensemble import RandomForestRegressor
from sklearn.preprocessing import StandardScaler


@pytest.mark.integration
def test_reload_artifacts_from_dir_and_predict_batting(tmp_path):
    """After reload from a dir with batting scaler+model, POST /predict/batting returns 200."""
    # Build minimal valid artifacts: scaler + model (batting feature vector length = 26, output = 6 cols)
    n_features = 26
    scaler = StandardScaler()
    scaler.fit(np.random.RandomState(42).randn(10, n_features))
    model = RandomForestRegressor(n_estimators=2, max_depth=2, random_state=42)
    model.fit(np.random.RandomState(43).randn(10, n_features), np.random.RandomState(44).randn(10, 6))

    bat_scaler_path = tmp_path / "batting_scaler_ODI.joblib"
    bat_model_path = tmp_path / "batting_model_ODI.joblib"
    joblib.dump(scaler, bat_scaler_path)
    joblib.dump(model, bat_model_path)

    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.reload(str(tmp_path))

    client = TestClient(app_module.app)
    row = {
        "batting_consistency": 0.5,
        "batting_form": 0.5,
        "batting_temp": 25,
        "batting_wind": 0,
        "batting_rain": 0,
        "batting_humidity": 50,
        "batting_cloud": 0,
        "batting_pressure": 0,
        "batting_viscosity": 0,
        "batting_inning": 1,
        "batting_session": 1,
        "toss": 0,
        "venue": 0.5,
        "opposition": 0.5,
        "season": 2024,
        "player_name": "Test",
        "format": "ODI",
    }
    resp = client.post("/predict/batting", json=[row])
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert isinstance(data, list) and len(data) == 1
    assert "runs_scored" in data[0] and "balls_faced" in data[0]


@pytest.mark.integration
def test_reload_artifacts_from_dir_and_predict_bowling(tmp_path):
    """After reload from a dir with bowling scaler+model, POST /predict/bowling returns 200."""
    n_features = 25  # bowling: 17 base + 8 seq (consistency, form, momentum, career_avg + 7 weather + 3 inning/session/toss + 3 venue/opp/season + 8 seq)
    scaler = StandardScaler()
    scaler.fit(np.random.RandomState(42).randn(10, n_features))
    model = RandomForestRegressor(n_estimators=2, max_depth=2, random_state=42)
    model.fit(np.random.RandomState(43).randn(10, n_features), np.random.RandomState(44).randn(10, 4))

    bowl_scaler_path = tmp_path / "bowling_scaler_ODI.joblib"
    bowl_model_path = tmp_path / "bowling_model_ODI.joblib"
    joblib.dump(scaler, bowl_scaler_path)
    joblib.dump(model, bowl_model_path)

    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.reload(str(tmp_path))

    client = TestClient(app_module.app)
    row = {
        "bowling_consistency": 0.5,
        "bowling_form": 0.5,
        "bowling_temp": 25,
        "bowling_wind": 0,
        "bowling_rain": 0,
        "bowling_humidity": 50,
        "bowling_cloud": 0,
        "bowling_pressure": 0,
        "bowling_viscosity": 0,
        "batting_inning": 1,
        "bowling_session": 1,
        "toss": 0,
        "bowling_venue": 0.5,
        "bowling_opposition": 0.5,
        "season": 2024,
        "player_name": "Test",
        "format": "ODI",
    }
    resp = client.post("/predict/bowling", json=[row])
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert isinstance(data, list) and len(data) == 1
    assert "runs_conceded" in data[0] and "deliveries" in data[0]
