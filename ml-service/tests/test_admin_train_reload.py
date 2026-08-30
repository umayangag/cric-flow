"""Tests that a finished /admin/train/* run reaches inference.

Training writes artifacts to disk from a subprocess; the serving process holds its
registries in memory. Without a reload at the end of the request, a run recorded as
COMPLETED never changes what the service predicts with.
"""

import importlib
import os

import joblib
import pytest
from fastapi.testclient import TestClient

import app.artifacts as artifacts

CUTOFF = "2026-01-01T00:00:00Z"


@pytest.fixture
def app_module(tmp_path):
    """app.main bound to an empty models dir, with admin training enabled."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    os.environ["ENABLE_HOT_RELOAD"] = "1"
    module = importlib.reload(importlib.import_module("app.main"))
    yield module
    os.environ.pop("ENABLE_HOT_RELOAD", None)
    artifacts.reload(str(tmp_path))


def test_completed_innings_training_loads_the_new_artifacts(app_module, tmp_path, monkeypatch):
    """The artifacts a training run writes are serving by the time the request returns."""

    def write_innings_artifacts(cutoff, go_app_url, logger=None):
        joblib.dump({}, tmp_path / "innings_scaler_T20.joblib")
        joblib.dump({}, tmp_path / "innings_model_T20.joblib")

    monkeypatch.setattr(app_module.training_orchestrator, "run_innings_training", write_innings_artifacts)
    artifacts.INNINGS_MODELS.clear()

    resp = TestClient(app_module.app).post(f"/admin/train/innings?cutoff={CUTOFF}")

    assert resp.status_code == 200
    assert "T20" in artifacts.INNINGS_MODELS


def test_reload_failure_does_not_fail_the_training_run(app_module, monkeypatch):
    """The artifacts are on disk either way, so a reload error is logged, not raised."""
    monkeypatch.setattr(app_module.training_orchestrator, "run_innings_training", lambda *a, **k: None)
    monkeypatch.setattr(app_module, "reload_artifacts", _raise_reload_error)

    resp = TestClient(app_module.app).post(f"/admin/train/innings?cutoff={CUTOFF}")

    assert resp.status_code == 200
    assert resp.json()["step"] == "innings"


def _raise_reload_error(models_dir: str):
    raise OSError("models dir unreadable")


def test_failed_training_does_not_reload(app_module, monkeypatch):
    """A run that raised wrote nothing worth loading, so the registries are left alone."""

    def fail(cutoff, go_app_url, logger=None):
        raise ValueError("training blew up")

    reload_calls = []
    monkeypatch.setattr(app_module.training_orchestrator, "run_innings_training", fail)
    monkeypatch.setattr(app_module, "reload_artifacts", lambda models_dir: reload_calls.append(models_dir))

    resp = TestClient(app_module.app).post(f"/admin/train/innings?cutoff={CUTOFF}")

    assert resp.status_code == 500
    assert reload_calls == []
