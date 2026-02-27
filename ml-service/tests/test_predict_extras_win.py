"""Tests for /predict/extras and /predict/win endpoints."""

import importlib
import os

import numpy as np
from fastapi.testclient import TestClient


class DummyExtrasModel:
    def predict(self, X: np.ndarray):
        return np.array([2.5] * len(X))


class DummyWinModel:
    def predict_proba(self, X: np.ndarray):
        n = len(X)
        return np.array([[0.3, 0.7]] * n)

    def predict(self, X: np.ndarray):
        return np.array([1] * len(X))


EXTRAS_ROW = {
    "format_id": 0,
    "venue_id": 0,
    "season_id": 0,
    "temp": 0,
    "wind": 0,
    "rain": 0,
    "humidity": 0,
    "cloud": 0,
    "pressure": 0,
    "viscosity": 0,
    "bat_consistency_sum": 0.0,
    "bowl_consistency_sum": 0.0,
    "bat_form_sum": 0.0,
    "bowl_form_sum": 0.0,
    "format": "ODI",
}

WIN_ROW = {
    "format_id": 0,
    "venue_id": 0,
    "team1_opposition_id": 0,
    "team2_opposition_id": 0,
    "toss_winner_opposition_id": 0,
    "temp": 0,
    "wind": 0,
    "rain": 0,
    "humidity": 0,
    "cloud": 0,
    "pressure": 0,
    "viscosity": 0,
    "team1_bat_consistency_sum": 0.0,
    "team1_bowl_consistency_sum": 0.0,
    "team2_bat_consistency_sum": 0.0,
    "team2_bowl_consistency_sum": 0.0,
    "team1_bat_form_sum": 0.0,
    "team1_bowl_form_sum": 0.0,
    "team2_bat_form_sum": 0.0,
    "team2_bowl_form_sum": 0.0,
    "format": "ODI",
}


def _client_with_extras_win_mocks(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.EXTRAS_MODELS.clear()
    artifacts_module.EXTRAS_MODELS["ODI"] = DummyExtrasModel()
    artifacts_module.WIN_MODELS.clear()
    artifacts_module.WIN_MODELS["ODI"] = DummyWinModel()
    return TestClient(app_module.app)


def test_predict_extras_success(tmp_path):
    """predict/extras returns predictions when model is loaded."""
    client = _client_with_extras_win_mocks(tmp_path)
    resp = client.post("/predict/extras", json=[EXTRAS_ROW])
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list) and len(data) == 1
    assert data[0]["total_extras"] == 2.5


def test_predict_extras_empty_rejected(tmp_path):
    """predict/extras rejects empty batch."""
    client = _client_with_extras_win_mocks(tmp_path)
    resp = client.post("/predict/extras", json=[])
    assert resp.status_code == 400


def test_predict_win_success(tmp_path):
    """predict/win returns team1 win probability when model is loaded."""
    client = _client_with_extras_win_mocks(tmp_path)
    resp = client.post("/predict/win", json=[WIN_ROW])
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list) and len(data) == 1
    assert "team1_win_probability" in data[0]
    assert 0 <= data[0]["team1_win_probability"] <= 1


def test_predict_win_empty_rejected(tmp_path):
    """predict/win rejects empty batch."""
    client = _client_with_extras_win_mocks(tmp_path)
    resp = client.post("/predict/win", json=[])
    assert resp.status_code == 400
