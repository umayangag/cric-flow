"""Tests for the /admin/train/* routes.

Each route is a thin shell around one training_orchestrator call: it decides whether
the run is allowed, what it needs before starting (a cutoff, an exported CSV), and
what it forwards. Training itself is stubbed -- what is asserted here is the shell.
"""

from __future__ import annotations

import importlib
import os
from typing import Any, Dict, List

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


@pytest.fixture
def client(app_module) -> TestClient:
    return TestClient(app_module.app)


@pytest.fixture
def recorded_calls(app_module, monkeypatch) -> Dict[str, List[Any]]:
    """Stub every training entry point; return the map of step -> forwarded arguments."""
    calls: Dict[str, List[Any]] = {}

    def record(step: str):
        def run(*args: Any, **kwargs: Any) -> None:
            calls[step] = list(args)

        return run

    monkeypatch.setattr(app_module.training_orchestrator, "run_win_training", record("win"))
    monkeypatch.setattr(app_module.training_orchestrator, "run_auto_tune", record("auto_tune"))
    return calls


@pytest.mark.parametrize("step", ["win"])
def test_cutoff_bound_training_steps_run_with_the_trimmed_cutoff(client, recorded_calls, step):
    """Training is temporal, and forwards the cutoff it was given."""
    resp = client.post(f"/admin/train/{step}?cutoff=  {CUTOFF}  ")

    assert resp.status_code == 200
    assert resp.json()["step"] == step
    assert recorded_calls[step][0] == CUTOFF


@pytest.mark.parametrize("step", ["win", "auto-tune"])
def test_cutoff_bound_training_steps_reject_a_missing_cutoff(client, recorded_calls, step):
    """Training without a cutoff would leak post-cutoff data, so it is refused up front."""
    resp = client.post(f"/admin/train/{step}?cutoff=   ")

    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "CUTOFF_REQUIRED"
    assert recorded_calls == {}


def test_auto_tune_forwards_its_search_options(client, recorded_calls, app_module):
    """The query flags select the search: model, format(s), rescreen, algorithms."""
    resp = client.post(
        f"/admin/train/auto-tune?cutoff={CUTOFF}&model=win&format=  t20  "
        "&all_formats=true&rescreen=1&algorithms=  rf,gbr  "
    )

    assert resp.status_code == 200
    assert resp.json() == {"status": "ok", "step": "auto-tune"}
    cutoff, go_app_url, model, use_all_formats, fmt, do_rescreen, algorithms = recorded_calls["auto_tune"][:7]
    assert cutoff == CUTOFF
    assert go_app_url == app_module._settings.go_app_url
    assert (model, fmt, algorithms) == ("win", "t20", "rf,gbr")
    assert use_all_formats is True
    assert do_rescreen is True


@pytest.mark.parametrize("flag_value", ["", "0", "no", "off"])
def test_auto_tune_treats_unset_flags_as_off(client, recorded_calls, flag_value):
    """Only an explicit affirmative widens the search; anything else leaves it narrow."""
    resp = client.post(f"/admin/train/auto-tune?cutoff={CUTOFF}&all_formats={flag_value}&rescreen={flag_value}")

    assert resp.status_code == 200
    _, _, _, use_all_formats, _, do_rescreen, _ = recorded_calls["auto_tune"][:7]
    assert use_all_formats is False
    assert do_rescreen is False


def test_training_is_refused_when_hot_reload_is_disabled(tmp_path, monkeypatch):
    """A serving-only deployment must not be able to start a training run."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    os.environ.pop("ENABLE_HOT_RELOAD", None)
    module = importlib.reload(importlib.import_module("app.main"))

    def fail(*args: Any, **kwargs: Any) -> None:
        raise AssertionError("training must not start while hot reload is disabled")

    monkeypatch.setattr(module.training_orchestrator, "run_win_training", fail)

    resp = TestClient(module.app).post("/admin/train/win")

    assert resp.status_code == 403
    assert resp.json()["detail"]["code"] == "TRAIN_DISABLED"


def test_step_progress_reports_the_live_run(client, app_module, monkeypatch):
    """go-app polls this per tick, so it answers with whatever the orchestrator has."""
    asked: Dict[str, str] = {}

    def fake_progress(step: str, run_id: str = "") -> Dict[str, Any]:
        asked.update({"step": step, "run_id": run_id})
        return {"step": step, "percent": 42}

    monkeypatch.setattr(app_module.training_orchestrator, "get_step_progress", fake_progress)

    resp = client.get("/admin/train/progress?step=train_win&run_id=r-1")

    assert resp.status_code == 200
    assert resp.json()["percent"] == 42
    assert asked == {"step": "train_win", "run_id": "r-1"}


def test_auto_tune_progress_delegates_to_the_orchestrator(client, app_module, monkeypatch):
    """The auto-tune route is a second view of the same progress, not a second reader."""
    monkeypatch.setattr(
        app_module.training_orchestrator, "get_auto_tune_progress", lambda: {"step": "auto_tune", "percent": 7}
    )

    resp = client.get("/admin/train/auto-tune/progress")

    assert resp.status_code == 200
    assert resp.json() == {"step": "auto_tune", "percent": 7}
