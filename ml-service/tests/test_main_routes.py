"""API/integration tests for main.py routes (TestClient)."""

import importlib
import os
from unittest.mock import patch

from fastapi.testclient import TestClient


def _app_client(tmp_path):
    """Load app with ML_SERVICE_OUTPUT_DIR set; return (app_module, client)."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    return app_module, TestClient(app_module.app)


def test_health_returns_ok_and_structure(tmp_path):
    """GET /health returns 200 and status, models_dir, artifacts, metadata."""
    _, client = _app_client(tmp_path)
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data.get("status") == "ok"
    assert "models_dir" in data
    assert "artifacts" in data
    assert "metadata" in data


def test_artifacts_status_returns_structure(tmp_path):
    """GET /artifacts/status returns timestamp, root, formats, legacy."""
    _, client = _app_client(tmp_path)
    resp = client.get("/artifacts/status")
    assert resp.status_code == 200
    data = resp.json()
    assert "timestamp" in data
    assert "root" in data
    assert "formats" in data
    assert "legacy" in data
    assert "ODI" in data["formats"]
    assert "batting" in data["formats"]["ODI"]


def test_model_metadata_error_returns_500(tmp_path):
    """GET /model-metadata when get_model_metadata raises returns 500."""
    _, client = _app_client(tmp_path)
    with patch("app.main.get_model_metadata", side_effect=RuntimeError("metadata load failed")):
        resp = client.get("/model-metadata")
    assert resp.status_code == 500
    detail = resp.json().get("detail", {})
    if isinstance(detail, dict):
        assert detail.get("code") == "METADATA_ERROR" or "metadata" in str(detail).lower()


def test_model_stats_error_returns_500(tmp_path):
    """GET /model-stats when build_model_stats raises returns 500."""
    _, client = _app_client(tmp_path)
    with patch("app.main.build_model_stats", side_effect=ValueError("scan failed")):
        resp = client.get("/model-stats")
    assert resp.status_code == 500
    detail = resp.json().get("detail", {})
    if isinstance(detail, dict):
        assert detail.get("code") == "MODEL_STATS_ERROR" or "stats" in str(detail).lower()


def test_backtest_predict_player_ids_without_format_returns_400(tmp_path):
    """POST /ml/backtest/predict with player_ids but no format/features returns 400."""
    _, client = _app_client(tmp_path)
    payload = {
        "cutoff_date": "2024-06-01T00:00:00Z",
        "player_ids": [1, 2, 3],
    }
    resp = client.post("/ml/backtest/predict", json=payload)
    assert resp.status_code == 400
    data = resp.json()
    assert "detail" in data
    detail = data["detail"]
    if isinstance(detail, dict):
        assert detail.get("code") == "FORMAT_AND_FEATURES_REQUIRED"


def test_predict_bowling_route_success(tmp_path):
    """POST /predict/bowling with mock model returns 200 and list of predictions."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.BOWL_MODELS.clear()
    artifacts_module.BOWL_MODELS["ODI"] = (_DummyScaler(), _DummyBowlModel())
    client = TestClient(app_module.app)
    row = {
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
    resp = client.post("/predict/bowling", json=[row])
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list)
    assert len(data) == 1
    assert "runs_conceded" in data[0]
    assert "deliveries" in data[0]
    assert "wickets_taken" in data[0]
    assert "econ" in data[0]


def test_predict_extras_route_success(tmp_path):
    """POST /predict/extras with mock model returns 200."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.EXTRAS_MODELS.clear()
    artifacts_module.EXTRAS_MODELS["ODI"] = _DummyExtrasModel()
    client = TestClient(app_module.app)
    row = {
        "format_id": 0,
        "venue_id": 0,
        "match_month_sin": 0,
        "match_month_cos": 0,
        "match_day_of_week_sin": 0,
        "match_day_of_week_cos": 0,
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
    resp = client.post("/predict/extras", json=[row])
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list)
    assert len(data) == 1


def test_predict_win_route_success(tmp_path):
    """POST /predict/win with mock model returns 200."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.WIN_MODELS.clear()
    artifacts_module.WIN_MODELS["ODI"] = _DummyWinModel()
    client = TestClient(app_module.app)
    row = {
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
    resp = client.post("/predict/win", json=[row])
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list)
    assert len(data) == 1


def test_optimize_team_selection_invalid_opponent_player_id_returns_400(tmp_path):
    """POST /optimize/team-selection with invalid opponent_features key (non-digit) returns 400."""
    _, client = _app_client(tmp_path)
    payload = {
        "format": "ODI",
        "pool": [
            {
                "player_id": 1,
                "name": "X",
                "is_bowler": False,
                "is_keeper": False,
                "bat_score": 0,
                "bowl_score": 0,
                "field_score": 0,
                "features": {},
            }
        ],
        "opponent_features": {"invalid_key": {"batting_mean_w5": 1.0}},
        "match_context": {},
        "constraints": {"size": 11, "min_bowlers": 3, "require_keeper": True},
        "weights": {"bat": 1.0, "bowl": 1.0, "field": 1.0, "keeper_bonus": 0.0},
        "team_is_team1": True,
        "max_iterations": 100,
        "max_evals": 500,
    }
    resp = client.post("/optimize/team-selection", json=payload)
    assert resp.status_code == 400
    assert resp.json().get("detail", {}).get("code") == "INVALID_PLAYER_ID"


# Dummy models for predict route tests
class _DummyScaler:
    def transform(self, X):
        import numpy as np

        return X if hasattr(X, "shape") else np.array(X)


class _DummyBowlModel:
    def predict(self, X):
        import numpy as np

        n = len(X) if hasattr(X, "__len__") else X.shape[0]
        return np.array([[10.0, 11.0, 12.0]] * n)


class _DummyExtrasModel:
    def predict(self, X):
        import numpy as np

        n = len(X) if hasattr(X, "__len__") else X.shape[0]
        return np.array([2.5] * n)


class _DummyWinModel:
    def predict_proba(self, X):
        import numpy as np

        n = len(X) if hasattr(X, "__len__") else X.shape[0]
        return np.array([[0.3, 0.7]] * n)

    def predict(self, X):
        import numpy as np

        n = len(X) if hasattr(X, "__len__") else X.shape[0]
        return np.array([1] * n)
