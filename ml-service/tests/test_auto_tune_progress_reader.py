"""Tests for training_orchestrator's auto-tune progress reader.

The reader is the seam where per-run progress files (ops plan O-1) meet an endpoint
that takes no run id. GET /admin/train/auto-tune/progress has to answer "what is
running now?" without being told which run to look at, and go-app polls it while a
training step is in flight.
"""

import json
import os
import time

import pytest

from app import training_orchestrator
from ml import run_progress


@pytest.fixture(autouse=True)
def isolated_progress(tmp_path, monkeypatch):
    """Point the progress directory at a temp dir and reset the channel."""
    monkeypatch.delenv("AUTO_TUNE_PROGRESS_FILE", raising=False)
    monkeypatch.setattr(run_progress, "progress_dir", lambda directory=None: str(tmp_path / "progress"))
    run_progress.set_progress_file(None, "")
    run_progress.set_callback(None)
    yield
    run_progress.set_progress_file(None, "")


def test_returns_empty_when_nothing_is_running():
    assert training_orchestrator.get_auto_tune_progress() == {}


def test_returns_the_live_run(monkeypatch):
    from ml import auto_tune_progress

    auto_tune_progress.configure(run_id="run-1")
    auto_tune_progress.write_progress("tuning", "batting", "T20", 1, 3, algorithm="ridge")

    got = training_orchestrator.get_auto_tune_progress()
    assert got["phase"] == "tuning"
    assert got["algorithm"] == "ridge"
    assert got["model_kind"] == "batting"
    assert got["run_id"] == "run-1"


def test_prefers_the_newest_of_several_runs():
    from ml import auto_tune_progress

    auto_tune_progress.configure(run_id="older")
    auto_tune_progress.write_progress("tuning", "batting", None, 1, 3, message="old")
    older = run_progress.get_progress_file()
    os.utime(older, (time.time() - 120, time.time() - 120))

    auto_tune_progress.configure(run_id="newer")
    auto_tune_progress.write_progress("tuning", "bowling", None, 2, 3, message="new")

    assert training_orchestrator.get_auto_tune_progress()["message"] == "new"


def test_ignores_a_crashed_run(monkeypatch):
    """A run that died leaves its file; showing it would report a run that is not happening."""
    from ml import auto_tune_progress

    auto_tune_progress.configure(run_id="crashed")
    auto_tune_progress.write_progress("tuning", "batting", None, 1, 3)
    path = run_progress.get_progress_file()
    long_ago = time.time() - (run_progress.STALE_AFTER_SEC + 60)
    os.utime(path, (long_ago, long_ago))

    assert training_orchestrator.get_auto_tune_progress() == {}


def test_env_var_still_pins_an_explicit_file(tmp_path, monkeypatch):
    """AUTO_TUNE_PROGRESS_FILE survives so a fixed path can still be watched or asserted on."""
    pinned = tmp_path / "pinned.json"
    pinned.write_text(json.dumps({"phase": "pinned", "v": 1}))
    monkeypatch.setenv("AUTO_TUNE_PROGRESS_FILE", str(pinned))

    assert training_orchestrator.get_auto_tune_progress_path() == str(pinned)
    assert training_orchestrator.get_auto_tune_progress()["phase"] == "pinned"


def test_pinned_file_missing_reads_as_empty(tmp_path, monkeypatch):
    monkeypatch.setenv("AUTO_TUNE_PROGRESS_FILE", str(tmp_path / "absent.json"))

    assert training_orchestrator.get_auto_tune_progress() == {}


def test_keeps_the_payload_shape_go_app_reads():
    """go-app forwards this straight into the SSE payload the ops console renders.

    The envelope was added underneath these fields, not in place of them; if that ever
    stops being true the console silently loses its auto-tune detail, so it is asserted
    here rather than assumed.
    """
    from ml import auto_tune_progress

    auto_tune_progress.configure(run_id="run-1")
    auto_tune_progress.write_progress(
        "fine_tuning",
        "extras",
        "ODI",
        1,
        2,
        algorithm="lgbm",
        trial=3,
        trials_total=10,
        best_score=0.85,
        activity="running_trial",
        algorithms_screened=["ridge", "lgbm"],
    )

    got = training_orchestrator.get_auto_tune_progress()
    for key in (
        "phase",
        "model_kind",
        "format_suffix",
        "task_index",
        "task_total",
        "algorithm",
        "trial",
        "trials_total",
        "best_score",
        "activity",
        "algorithms_screened",
    ):
        assert key in got, f"the ops console reads {key}"


def test_cleared_progress_disappears_from_the_reader():
    from ml import auto_tune_progress

    auto_tune_progress.configure(run_id="run-1")
    auto_tune_progress.write_progress("tuning", "batting", None, 1, 3)
    assert training_orchestrator.get_auto_tune_progress() != {}

    auto_tune_progress.clear_progress()
    assert training_orchestrator.get_auto_tune_progress() == {}
