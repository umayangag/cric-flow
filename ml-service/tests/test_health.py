import importlib

from fastapi.testclient import TestClient


def test_health_endpoint_basic(monkeypatch, tmp_path):
    # Point models dir to an empty temporary directory BEFORE import
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))

    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    client = TestClient(app_module.app)
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()

    # Basic structure assertions
    assert data["status"] == "ok"
    assert "models_dir" in data
    assert "artifacts" in data and set(data["artifacts"].keys()) == {"batting", "bowling"}
    assert "metadata" in data and set(data["metadata"].keys()) == {"batting", "bowling"}
    assert "counters" in data and {"batting_formats", "bowling_formats"}.issubset(set(data["counters"].keys()))
    assert "loaded_batting_formats" in data
    assert "loaded_bowling_formats" in data
    assert "legacy_batting_available" in data
    assert "legacy_bowling_available" in data
