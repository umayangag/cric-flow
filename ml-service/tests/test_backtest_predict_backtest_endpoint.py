import json
from datetime import datetime

from fastapi.testclient import TestClient

from app.main import app


client = TestClient(app)


def test_backtest_predict_players_mode_deterministic_and_schema():
    body = {
        "cutoff_date": "2024-10-30T14:00:00Z",
        "player_ids": [1, 2, 3],
    }
    r1 = client.post("/ml/backtest/predict", json=body)
    assert r1.status_code == 200, r1.text
    data1 = r1.json()
    assert "players" in data1 and isinstance(data1["players"], list)
    # Ensure required keys for each player, including optional fielding keys
    for p in data1["players"]:
        assert set(["player_id", "runs"]).issubset(p.keys())
        # Fielding keys should be present in baseline and be numbers
        assert "catches" in p and isinstance(p["catches"], (int, float))
        assert "run_outs" in p and isinstance(p["run_outs"], (int, float))

    # Deterministic: same input -> same output
    r2 = client.post("/ml/backtest/predict", json=body)
    assert r2.status_code == 200
    data2 = r2.json()
    # Deterministic: full payload equality ensures fielding values are stable too
    assert data1 == data2


def test_backtest_predict_match_mode_schema_and_winner_present():
    body = {
        "cutoff_date": "2024-10-30T14:00:00Z",
        "teams": ["IND", "AUS"],
    }
    r = client.post("/ml/backtest/predict", json=body)
    assert r.status_code == 200, r.text
    data = r.json()
    assert "match" in data and isinstance(data["match"], dict)
    m = data["match"]
    for k in ["runs", "wickets", "extras", "winner_team_code"]:
        assert k in m
    assert m["winner_team_code"] in {"IND", "AUS"}


def test_backtest_predict_requires_either_players_or_teams():
    body = {"cutoff_date": "2024-10-30T14:00:00Z"}
    r = client.post("/ml/backtest/predict", json=body)
    assert r.status_code == 400
    detail = r.json().get("detail", {})
    assert isinstance(detail, dict)
    assert detail.get("code") == "INVALID_REQUEST"


def test_backtest_predict_teams_validation():
    # teams must be exactly two
    body = {"cutoff_date": "2024-10-30T14:00:00Z", "teams": ["IND"]}
    r = client.post("/ml/backtest/predict", json=body)
    assert r.status_code == 422
