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
    assert "artifacts" in data
    assert {"batting", "bowling", "fielding", "extras", "win"} == set(data["artifacts"].keys())
    assert "metadata" in data
    assert {"batting", "bowling", "fielding"}.issubset(set(data["metadata"].keys()))
    assert "counters" in data
    assert {"batting_formats", "bowling_formats", "fielding_formats", "extras_formats", "win_formats"}.issubset(
        set(data["counters"].keys())
    )
    assert "loaded_batting_formats" in data
    assert "loaded_bowling_formats" in data
    assert "loaded_fielding_formats" in data
    assert "loaded_extras_formats" in data
    assert "loaded_win_formats" in data
    for kind in ("batting", "bowling", "fielding", "extras", "win"):
        assert f"legacy_{kind}_available" not in data
