import importlib
import os
from typing import List

import numpy as np
import pytest
from fastapi.testclient import TestClient


class DummyScaler:
    def transform(self, X: np.ndarray) -> np.ndarray:
        return X  # identity


class DummyBatModel:
    def predict(self, X: np.ndarray) -> List[List[float]]:
        # Return 5 columns per row: runs, balls, fours, sixes, batting_position
        # strike_rate is derived as runs/balls*100
        out = []
        for _ in X:
            out.append([1.0, 2.0, 3.0, 4.0, 5.0])
        return np.array(out)


class DummyBowlModel:
    def predict(self, X: np.ndarray) -> List[List[float]]:
        # Return 3 columns per row: runs, balls, wickets
        # econ is derived as runs/(balls/6)
        out = []
        for _ in X:
            out.append([10.0, 11.0, 12.0])
        return np.array(out)


BAT_ROW = {
    "batting_consistency": 0.0,
    "batting_form": 0.0,
    "batting_temp": 0,
    "batting_wind": 0,
    "batting_rain": 0,
    "batting_humidity": 0,
    "batting_cloud": 0,
    "batting_pressure": 0,
    "batting_viscosity": 0,
    "batting_inning": 1,
    "batting_session": 1,
    "toss": 0,
    "venue": 0.0,
    "opposition": 0.0,
    "season": 0,
    "player_name": "P",
    "format": "ODI",
}

BOWL_ROW = {
    "bowling_consistency": 0.0,
    "bowling_form": 0.0,
    "bowling_temp": 0,
    "bowling_wind": 0,
    "bowling_rain": 0,
    "bowling_humidity": 0,
    "bowling_cloud": 0,
    "bowling_pressure": 0,
    "bowling_viscosity": 0,
    "batting_inning": 1,
    "bowling_session": 1,
    "toss": 0,
    "bowling_venue": 0.0,
    "bowling_opposition": 0.0,
    "season": 0,
    "player_name": "P",
    "format": "ODI",
}


def _client_with_mocks(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    # Import app first so it binds BAT_MODELS/BOWL_MODELS to the dict objects
    app_module = importlib.import_module("app.main")
    # Now mutate the shared registries via artifacts module (same dict objects)
    artifacts_module = importlib.import_module("app.artifacts")

    artifacts_module.BAT_MODELS.clear()
    artifacts_module.BAT_MODELS["ODI"] = (DummyScaler(), DummyBatModel())
    artifacts_module.BOWL_MODELS.clear()
    artifacts_module.BOWL_MODELS["ODI"] = (DummyScaler(), DummyBowlModel())

    # Do NOT reload app_module here; reloading would wipe mocks via reload_artifacts()
    return TestClient(app_module.app)


def test_predict_batting_success_with_mock_model(tmp_path):
    client = _client_with_mocks(tmp_path)
    resp = client.post("/predict/batting", json=[BAT_ROW, BAT_ROW])
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list) and len(data) == 2
    for item in data:
        assert set(item.keys()) == {
            "runs_scored",
            "balls_faced",
            "fours_scored",
            "sixes_scored",
            "batting_position",
            "strike_rate",
        }
        assert item["runs_scored"] == 1.0
        # strike_rate = runs/balls*100 = 1/2*100 = 50.0
        assert item["strike_rate"] == 50.0


def test_predict_bowling_success_with_mock_model(tmp_path):
    client = _client_with_mocks(tmp_path)
    resp = client.post("/predict/bowling", json=[BOWL_ROW])
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list) and len(data) == 1
    obj = data[0]
    assert set(obj.keys()) == {"runs_conceded", "deliveries", "wickets_taken", "econ"}
    assert obj["runs_conceded"] == 10.0
    # econ = runs/(balls/6) = 10/(11/6) = 60/11
    assert obj["econ"] == pytest.approx(60.0 / 11.0)
