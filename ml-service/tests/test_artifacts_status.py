import os
import time
import importlib


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
        assert data["formats"][fmt]["batting"]["exists"] is False
        assert data["formats"][fmt]["bowling"]["exists"] is False


def _touch(path: str):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(b"")
    # Set deterministic mtime
    ts = int(time.time())
    os.utime(path, (ts, ts))


def test_artifacts_status_per_format_discovery(tmp_path):
    # Create ODI batting and bowling artifacts with preferred names
    _touch(tmp_path / "batting_ODI.joblib")
    _touch(tmp_path / "bowling_T20I.joblib")

    m = reload_app_with_dir(str(tmp_path))
    from fastapi.testclient import TestClient

    client = TestClient(m.app)
    r = client.get("/artifacts/status")
    assert r.status_code == 200
    data = r.json()
    assert data["formats"]["ODI"]["batting"]["exists"] is True
    assert data["formats"]["T20I"]["bowling"]["exists"] is True


def test_artifacts_status_loaded_flags(tmp_path, monkeypatch):
    # Create artifacts and also mark loaded registries
    _touch(tmp_path / "batting_TEST.joblib")
    _touch(tmp_path / "bowling_TEST.joblib")

    m = reload_app_with_dir(str(tmp_path))

    # Monkeypatch registries to simulate loaded state
    monkeypatch.setitem(m.BAT_MODELS, "TEST", object())
    monkeypatch.setitem(m.BOWL_MODELS, "TEST", object())

    from fastapi.testclient import TestClient

    client = TestClient(m.app)
    r = client.get("/artifacts/status")
    assert r.status_code == 200
    data = r.json()
    assert data["formats"]["TEST"]["batting"]["loaded"] is True
    assert data["formats"]["TEST"]["bowling"]["loaded"] is True
