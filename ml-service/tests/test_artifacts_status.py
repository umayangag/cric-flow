import importlib
import os
import time

import joblib


def reload_app_with_dir(root: str):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = root
    # Reload module to pick up new env var and recompute MODELS_DIR
    import app.main as m

    importlib.reload(m)
    return m


def test_artifacts_status_empty_dir(tmp_path):
    m = reload_app_with_dir(str(tmp_path))
    from fastapi.testclient import TestClient

    client = TestClient(m.app)
    r = client.get("/artifacts/status")
    assert r.status_code == 200
    data = r.json()
    assert data["root"].endswith(str(tmp_path)) or data["root"] == str(tmp_path)
    for fmt in ["TEST", "ODI", "T20I", "T20"]:
        assert data["formats"][fmt]["win"]["exists"] is False
    assert "legacy" not in data


def _touch(path: str):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(b"")
    # Set deterministic mtime
    ts = int(time.time())
    os.utime(path, (ts, ts))


def test_artifacts_status_per_format_discovery(tmp_path):
    # The win kind has no scaler; a stray one on disk must not change discovery.
    _touch(tmp_path / "win_scaler_ODI.joblib")
    _touch(tmp_path / "win_model_ODI.joblib")

    m = reload_app_with_dir(str(tmp_path))
    from fastapi.testclient import TestClient

    client = TestClient(m.app)
    r = client.get("/artifacts/status")
    assert r.status_code == 200
    data = r.json()
    assert data["formats"]["ODI"]["win"]["exists"] is True


def test_artifacts_status_loaded_flags(tmp_path, monkeypatch):
    _touch(tmp_path / "win_scaler_TEST.joblib")
    _touch(tmp_path / "win_model_TEST.joblib")

    m = reload_app_with_dir(str(tmp_path))
    import app.artifacts as art

    # Monkeypatch registries to simulate loaded state
    monkeypatch.setitem(art.WIN_MODELS, "TEST", object())

    from fastapi.testclient import TestClient

    client = TestClient(m.app)
    r = client.get("/artifacts/status")
    assert r.status_code == 200
    data = r.json()
    assert data["formats"]["TEST"]["win"]["loaded"] is True


def test_artifacts_reload_per_format(tmp_path):
    """Reload with a per-format model file exercises the _load_per_format success path."""
    import app.artifacts as art

    joblib.dump({}, tmp_path / "win_scaler_T20.joblib")
    joblib.dump({}, tmp_path / "win_model_T20.joblib")

    out = art.reload(str(tmp_path))
    assert "T20" in out["loaded_win_formats"]
    assert art.WIN_MODELS.get("T20") is not None
