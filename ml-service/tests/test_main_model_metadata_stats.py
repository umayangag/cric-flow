"""Tests for /model-metadata and /model-stats endpoints."""

import importlib
import os

from fastapi.testclient import TestClient


def test_model_metadata_returns_structure(tmp_path):
    """model-metadata returns metadata structure."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    client = TestClient(app_module.app)
    resp = client.get("/model-metadata")
    assert resp.status_code == 200
    data = resp.json()
    assert set(data) == {"win"}
    assert data["win"]["level"] == "match"


def test_model_stats_returns_list(tmp_path):
    """model-stats returns list of model artifact stats."""
    (tmp_path / "win_model_ODI.joblib").write_bytes(b"x" * 10)
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    client = TestClient(app_module.app)
    resp = client.get("/model-stats")
    assert resp.status_code == 200
    data = resp.json()
    models = data.get("models", data) if isinstance(data, dict) else data
    assert isinstance(models, list)
    assert any(r.get("model_name") == "Win" and r.get("match_format") == "ODI" for r in models)
