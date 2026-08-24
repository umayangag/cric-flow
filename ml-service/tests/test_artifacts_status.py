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
        assert data["formats"][fmt]["batting"]["exists"] is False
        assert data["formats"][fmt]["bowling"]["exists"] is False
    assert "legacy" not in data


def _touch(path: str):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(b"")
    # Set deterministic mtime
    ts = int(time.time())
    os.utime(path, (ts, ts))


def test_artifacts_status_per_format_discovery(tmp_path):
    # Create per-format artifacts (scaler + model for batting/bowling)
    _touch(tmp_path / "batting_scaler_ODI.joblib")
    _touch(tmp_path / "batting_model_ODI.joblib")
    _touch(tmp_path / "bowling_scaler_T20I.joblib")
    _touch(tmp_path / "bowling_model_T20I.joblib")

    m = reload_app_with_dir(str(tmp_path))
    from fastapi.testclient import TestClient

    client = TestClient(m.app)
    r = client.get("/artifacts/status")
    assert r.status_code == 200
    data = r.json()
    assert data["formats"]["ODI"]["batting"]["exists"] is True
    assert data["formats"]["T20I"]["bowling"]["exists"] is True


def test_artifacts_status_loaded_flags(tmp_path, monkeypatch):
    # Create per-format artifacts (scaler + model)
    _touch(tmp_path / "batting_scaler_TEST.joblib")
    _touch(tmp_path / "batting_model_TEST.joblib")
    _touch(tmp_path / "bowling_scaler_TEST.joblib")
    _touch(tmp_path / "bowling_model_TEST.joblib")

    m = reload_app_with_dir(str(tmp_path))
    import app.artifacts as art

    # Monkeypatch registries to simulate loaded state
    monkeypatch.setitem(art.BAT_MODELS, "TEST", object())
    monkeypatch.setitem(art.BOWL_MODELS, "TEST", object())

    from fastapi.testclient import TestClient

    client = TestClient(m.app)
    r = client.get("/artifacts/status")
    assert r.status_code == 200
    data = r.json()
    assert data["formats"]["TEST"]["batting"]["loaded"] is True
    assert data["formats"]["TEST"]["bowling"]["loaded"] is True


def test_artifacts_reload_per_format(tmp_path):
    """Reload with per-format scaler/model files exercises _load_per_format success path."""
    import app.artifacts as art

    joblib.dump({}, tmp_path / "batting_scaler_T20.joblib")
    joblib.dump({}, tmp_path / "batting_model_T20.joblib")
    joblib.dump({}, tmp_path / "bowling_scaler_T20.joblib")
    joblib.dump({}, tmp_path / "bowling_model_T20.joblib")

    out = art.reload(str(tmp_path))
    assert "T20" in out["loaded_batting_formats"]
    assert "T20" in out["loaded_bowling_formats"]
    assert art.BAT_MODELS.get("T20") is not None
    assert art.BOWL_MODELS.get("T20") is not None
