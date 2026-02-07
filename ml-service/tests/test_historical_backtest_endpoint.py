from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_historical_backtest_requires_exactly_one_selector():
    cutoff = "2024-10-30T14:00:00Z"
    # Neither match_id nor filters -> 422
    r1 = client.post("/ml/backtest/match", json={"cutoff_date": cutoff})
    assert r1.status_code == 422, r1.text

    # Both match_id and filters -> 422
    r2 = client.post(
        "/ml/backtest/match",
        json={
            "cutoff_date": cutoff,
            "match_id": 123,
            "filters": {
                "format": "T20",
                "team1": "IND",
                "team2": "AUS",
                "match_date": cutoff,
            },
        },
    )
    assert r2.status_code == 422, r2.text


def test_historical_backtest_accepts_match_id_or_filters_and_returns_payload():
    cutoff = "2024-10-30T14:00:00Z"
    # match_id only -> returns payload
    r1 = client.post("/ml/backtest/match", json={"cutoff_date": cutoff, "match_id": 789})
    assert r1.status_code == 200, r1.text
    body1 = r1.json()
    assert isinstance(body1, dict)
    assert "players" in body1 and isinstance(body1["players"], list)
    assert "match" in body1 and isinstance(body1["match"], dict)
    assert "metrics" in body1 and isinstance(body1["metrics"], dict)
    # Basic metrics keys
    for k in ["mae_runs", "rmse_runs"]:
        assert k in body1["metrics"]

    # filters only -> returns payload
    r2 = client.post(
        "/ml/backtest/match",
        json={
            "cutoff_date": cutoff,
            "filters": {
                "format": "ODI",
                "team1": "SL",
                "team2": "PAK",
                "match_date": cutoff,
            },
        },
    )
    assert r2.status_code == 200, r2.text
    body2 = r2.json()
    assert isinstance(body2, dict)
    assert "players" in body2 and isinstance(body2["players"], list)
    assert "match" in body2 and isinstance(body2["match"], dict)
    assert "metrics" in body2 and isinstance(body2["metrics"], dict)
