import importlib

from fastapi.testclient import TestClient

from app.artifact_service import ARTIFACT_KIND_NAMES


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
    assert "artifacts" in data
    # Asserted against the loader's own kind list, not a copy of it: a model kind that the
    # service can load but health never mentions is the failure this endpoint had.
    assert set(ARTIFACT_KIND_NAMES) == set(data["artifacts"].keys())
    assert "metadata" in data
    assert set(data["metadata"]) == set(ARTIFACT_KIND_NAMES)
    assert "counters" in data
    assert {f"{kind}_formats" for kind in ARTIFACT_KIND_NAMES}.issubset(set(data["counters"].keys()))
    for kind in ARTIFACT_KIND_NAMES:
        assert f"loaded_{kind}_formats" in data
        assert f"legacy_{kind}_available" not in data
