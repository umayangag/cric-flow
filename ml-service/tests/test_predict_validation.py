import importlib
import os

from fastapi.testclient import TestClient


def create_client(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    # Import inside to ensure env takes effect before main resolves settings
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    return TestClient(app_module.app)


def test_batting_empty_batch(tmp_path):
    client = create_client(tmp_path)
    resp = client.post("/predict/batting", json=[])
    assert resp.status_code == 400
    detail = resp.json()["detail"]
    assert detail["code"] == "EMPTY_BATCH"


def test_bowling_empty_batch(tmp_path):
    client = create_client(tmp_path)
    resp = client.post("/predict/bowling", json=[])
    assert resp.status_code == 400
    detail = resp.json()["detail"]
    assert detail["code"] == "EMPTY_BATCH"


def _batting_row(fmt: str):
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


def test_batting_mixed_formats(tmp_path):
    client = create_client(tmp_path)
    body = [_batting_row("ODI"), _batting_row("T20")]
    resp = client.post("/predict/batting", json=body)
    assert resp.status_code == 400
    detail = resp.json()["detail"]
    assert detail["code"] == "MIXED_FORMATS"
