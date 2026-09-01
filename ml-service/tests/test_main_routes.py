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
    """GET /artifacts/status returns timestamp, root, formats."""
    _, client = _app_client(tmp_path)
    resp = client.get("/artifacts/status")
    assert resp.status_code == 200
    data = resp.json()
    assert "timestamp" in data
    assert "root" in data
    assert "formats" in data
    assert "legacy" not in data
    assert "ODI" in data["formats"]
    assert "win" in data["formats"]["ODI"]


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


class _DummyWinModel:
    def predict_proba(self, X):
        import numpy as np

        n = len(X) if hasattr(X, "__len__") else X.shape[0]
        return np.array([[0.3, 0.7]] * n)
