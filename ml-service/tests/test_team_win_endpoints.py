import importlib

import pandas as pd
from fastapi.testclient import TestClient


def _load_app(tmp_path, monkeypatch):
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    m = importlib.import_module("app.main")
    importlib.reload(m)
    return m


def test_predict_win_list_endpoint(tmp_path, monkeypatch):
    m = _load_app(tmp_path, monkeypatch)

    def fake_predict_for_team(df: pd.DataFrame):
        # Return back a DataFrame with required keys and a dummy mean
        out = pd.DataFrame(
            [
                {
                    "player_name": "P1",
                    "runs_scored": 10.0,
                    "balls_faced": 11.0,
                    "fours_scored": 1.0,
                    "sixes_scored": 0.0,
                    "batting_position": 3.0,
                    "strike_rate": 90.0,
                    "runs_conceded": 20.0,
                    "deliveries": 24.0,
                    "wickets_taken": 1.0,
                    "econ": 5.0,
                    "winning_probability": 0.55,
                }
            ]
        )
        return out, 0.55

    monkeypatch.setattr(m, "predict_for_team", fake_predict_for_team)

    client = TestClient(m.app)
    body = [
        {
            "player_name": "P1",
            "runs_scored": 0,
            "balls_faced": 0,
            "fours_scored": 0,
            "sixes_scored": 0,
            "batting_position": 0,
            "strike_rate": 0,
            "runs_conceded": 0,
            "deliveries": 0,
            "wickets_taken": 0,
            "econ": 0,
            "winning_probability": 0.1,
        }
    ]
    resp = client.post("/predict-win", json=body)
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list) and len(data) == 1
    assert data[0]["player_name"] == "P1"
    assert "winning_probability" in data[0]


def test_predict_win_wrapped_endpoint(tmp_path, monkeypatch):
    m = _load_app(tmp_path, monkeypatch)

    def fake_predict_for_team(df: pd.DataFrame):
        out = pd.DataFrame(
            [
                {
                    "player_name": "P2",
                    "runs_scored": 5.0,
                    "balls_faced": 6.0,
                    "fours_scored": 0.0,
                    "sixes_scored": 1.0,
                    "batting_position": 7.0,
                    "strike_rate": 150.0,
                    "runs_conceded": 30.0,
                    "deliveries": 18.0,
                    "wickets_taken": 2.0,
                    "econ": 7.5,
                    "winning_probability": 0.6,
                }
            ]
        )
        return out, 0.6

    monkeypatch.setattr(m, "predict_for_team", fake_predict_for_team)

    client = TestClient(m.app)
    body = [
        {
            "player_name": "P2",
            "runs_scored": 0,
            "balls_faced": 0,
            "fours_scored": 0,
            "sixes_scored": 0,
            "batting_position": 0,
            "strike_rate": 0,
            "runs_conceded": 0,
            "deliveries": 0,
            "wickets_taken": 0,
            "econ": 0,
            "winning_probability": 0.0,
        }
    ]
    resp = client.post("/predict/win", json=body)
    assert resp.status_code == 200
    data = resp.json()
    assert "players" in data and "team_win_probability" in data
    assert isinstance(data["players"], list) and len(data["players"]) == 1
    assert data["team_win_probability"] == 0.6
