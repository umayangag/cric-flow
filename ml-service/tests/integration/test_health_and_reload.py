"""
Integration tests: /health after reload.

Exercises reload(models_dir) -> GET /health returns status and artifact info.
"""

import importlib
import os

import pytest
from fastapi.testclient import TestClient


@pytest.mark.integration
def test_health_returns_ok_and_artifact_keys(tmp_path):
    """GET /health returns 200 with status ok and artifact/counter keys."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.reload(str(tmp_path))

    client = TestClient(app_module.app)
    resp = client.get("/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body.get("status") == "ok"
    assert "loaded_win_formats" in body
    assert "artifacts" in body
    assert isinstance(body["loaded_win_formats"], list)


@pytest.mark.integration
def test_health_after_reload_empty_dir(tmp_path):
    """GET /health after reload(empty_dir) returns empty loaded formats lists."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")
    artifacts_module.reload(str(tmp_path))

    client = TestClient(app_module.app)
    resp = client.get("/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body["loaded_win_formats"] == []
