from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_backtest_predict_players_requires_format_and_features():
    """Player predictions require format and features (no baseline fallback)."""
    body = {
        "cutoff_date": "2024-10-30T14:00:00Z",
        "player_ids": [1, 2, 3],
    }
    r = client.post("/ml/backtest/predict", json=body)
    assert r.status_code == 400, r.text
    detail = r.json().get("detail", {})
    assert detail.get("code") == "FORMAT_AND_FEATURES_REQUIRED"


def test_backtest_predict_players_mode_with_format_and_features_schema():
    """With format and features, endpoint returns player predictions (uses loaded models or train-on-the-fly)."""
    body = {
        "cutoff_date": "2024-10-30T14:00:00Z",
        "player_ids": [1, 2, 3],
        "format": "T20",
        "features": {
            "1": {"batting_consistency": 0.5, "batting_form": 20.0},
            "2": {"batting_consistency": 0.4, "batting_form": 15.0},
            "3": {"batting_consistency": 0.6, "batting_form": 25.0},
        },
    }
    # If no T20 artifacts are loaded, train_on_the_fly_cached would be called (needs GO_APP_URL).
    # Mock train_on_the_fly_cached to return minimal in-memory models so we can assert schema without go-app.
    from unittest.mock import patch

    import numpy as np
    from sklearn.ensemble import RandomForestRegressor
    from sklearn.multioutput import MultiOutputRegressor
    from sklearn.preprocessing import StandardScaler

    def _fake_train(*args, **kwargs):
        scaler = StandardScaler()
        scaler.fit(np.zeros((2, 15)))
        bat = (
            scaler,
            MultiOutputRegressor(RandomForestRegressor(n_estimators=2)).fit(np.zeros((2, 15)), np.zeros((2, 6))),
        )
        bowl = (
            scaler,
            MultiOutputRegressor(RandomForestRegressor(n_estimators=2)).fit(np.zeros((2, 15)), np.zeros((2, 4))),
        )
        return bat, bowl

    with patch("app.prediction_service.train_on_the_fly_cached", side_effect=_fake_train):
        with patch.dict("os.environ", {"GO_APP_URL": "http://localhost:9999"}, clear=False):
            r = client.post("/ml/backtest/predict", json=body)
    if r.status_code != 200:
        assert r.status_code == 503, r.text
        return
    data = r.json()
    assert "players" in data and isinstance(data["players"], list)
    for p in data["players"]:
        assert set(["player_id", "runs"]).issubset(p.keys())
        assert "catches" in p and isinstance(p["catches"], (int, float))
        assert "run_outs" in p and isinstance(p["run_outs"], (int, float))


def test_backtest_predict_match_mode_schema_and_winner_present():
    body = {
        "cutoff_date": "2024-10-30T14:00:00Z",
        "teams": ["IND", "AUS"],
    }
    r = client.post("/ml/backtest/predict", json=body)
    assert r.status_code == 200, r.text
    data = r.json()
    # Ensure model_version is present to match Go client's expectations
    assert "model_version" in data
    assert isinstance(data["model_version"], str)
    assert data["model_version"].strip() != ""
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
