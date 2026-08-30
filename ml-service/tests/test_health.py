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
    assert {"batting", "bowling", "fielding"}.issubset(set(data["metadata"].keys()))
    assert "counters" in data
    assert {f"{kind}_formats" for kind in ARTIFACT_KIND_NAMES}.issubset(set(data["counters"].keys()))
    for kind in ARTIFACT_KIND_NAMES:
        assert f"loaded_{kind}_formats" in data
        assert f"legacy_{kind}_available" not in data


def test_health_reports_innings_as_a_first_class_kind(monkeypatch, tmp_path):
    """The innings model is reported like any other: trained artifacts must be visible."""
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    (tmp_path / "innings_scaler_T20.joblib").write_bytes(b"x")
    (tmp_path / "innings_model_T20.joblib").write_bytes(b"y")

    app_module = importlib.reload(importlib.import_module("app.main"))
    data = TestClient(app_module.app).get("/health").json()

    assert sorted(a["file"] for a in data["artifacts"]["innings"]) == [
        "innings_model_T20.joblib",
        "innings_scaler_T20.joblib",
    ]
    assert "loaded_innings_formats" in data
