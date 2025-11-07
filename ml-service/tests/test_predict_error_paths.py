import importlib
import os

from fastapi.testclient import TestClient


def _client(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    return TestClient(app_module.app)


def _bat_row(fmt=None):
    return {
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
        "format": fmt,
    }


def _bowl_row(fmt=None):
    return {
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
        "format": fmt,
    }


def test_batting_missing_format_without_legacy(tmp_path):
    client = _client(tmp_path)
    resp = client.post("/predict/batting", json=[_bat_row(None)])
    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "MISSING_FORMAT"


def test_batting_model_not_loaded_for_unknown_format(tmp_path):
    client = _client(tmp_path)
    resp = client.post("/predict/batting", json=[_bat_row("ODI")])
    assert resp.status_code == 404
    assert resp.json()["detail"]["code"] == "MODEL_NOT_LOADED"


def test_bowling_missing_format_without_legacy(tmp_path):
    client = _client(tmp_path)
    resp = client.post("/predict/bowling", json=[_bowl_row(None)])
    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "MISSING_FORMAT"


def test_bowling_model_not_loaded_for_unknown_format(tmp_path):
    client = _client(tmp_path)
    resp = client.post("/predict/bowling", json=[_bowl_row("ODI")])
    assert resp.status_code == 404
    assert resp.json()["detail"]["code"] == "MODEL_NOT_LOADED"
