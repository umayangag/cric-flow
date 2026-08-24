import importlib
import os

from fastapi.testclient import TestClient


def _load_app(tmp_path, enable_reload: bool = False):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    if enable_reload:
        os.environ["ENABLE_HOT_RELOAD"] = "1"
    else:
        os.environ.pop("ENABLE_HOT_RELOAD", None)
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    return app_module


def test_admin_reload_disabled_returns_403(tmp_path):
    m = _load_app(tmp_path, enable_reload=False)
    client = TestClient(m.app)
    resp = client.post("/admin/reload")
    assert resp.status_code == 403
    detail = resp.json()["detail"]
    assert detail["code"] == "RELOAD_DISABLED"


def test_admin_reload_enabled_returns_summary(tmp_path):
    m = _load_app(tmp_path, enable_reload=True)
    client = TestClient(m.app)
    resp = client.post("/admin/reload")
    assert resp.status_code == 200
    data = resp.json()
    # Summary comes from app.artifacts.summary()
    assert data["status"] == "reloaded"
    assert "loaded_batting_formats" in data
    assert "loaded_bowling_formats" in data
    assert "legacy_batting" not in data
    assert "legacy_bowling" not in data


def test_admin_reload_with_api_key_wrong_returns_401(tmp_path):
    """When ADMIN_API_KEY is set and X-API-Key is wrong, returns 401."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    os.environ["ENABLE_HOT_RELOAD"] = "1"
    os.environ["ADMIN_API_KEY"] = "secret123"
    try:
        app_module = importlib.import_module("app.main")
        importlib.reload(app_module)
        client = TestClient(app_module.app)
        resp = client.post("/admin/reload", headers={"X-API-Key": "wrong"})
        assert resp.status_code == 401
        assert resp.json()["detail"]["code"] == "UNAUTHORIZED"
    finally:
        os.environ.pop("ADMIN_API_KEY", None)
        os.environ.pop("ENABLE_HOT_RELOAD", None)


def test_admin_reload_with_api_key_correct_succeeds(tmp_path):
    """When ADMIN_API_KEY is set and X-API-Key matches, reload succeeds."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    os.environ["ENABLE_HOT_RELOAD"] = "1"
    os.environ["ADMIN_API_KEY"] = "secret123"
    try:
        app_module = importlib.import_module("app.main")
        importlib.reload(app_module)
        client = TestClient(app_module.app)
        resp = client.post("/admin/reload", headers={"X-API-Key": "secret123"})
        assert resp.status_code == 200
        assert resp.json()["status"] == "reloaded"
    finally:
        os.environ.pop("ADMIN_API_KEY", None)
        os.environ.pop("ENABLE_HOT_RELOAD", None)
