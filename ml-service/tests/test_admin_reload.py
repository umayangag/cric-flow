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
    assert "legacy_batting" in data
    assert "legacy_bowling" in data
