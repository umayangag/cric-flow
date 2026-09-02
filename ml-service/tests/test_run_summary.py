"""Tests for the terminal run summary (ops plan O-4).

A result is not progress. The progress file is removed when a run ends -- a file left
behind reads as a run still going -- but the outcome has to survive that, because
whoever wants it asks only after the run is over.
"""

import pytest

from app import training_orchestrator
from ml import run_progress
from ml import training_progress as tp


@pytest.fixture(autouse=True)
def isolated(tmp_path, monkeypatch):
    monkeypatch.delenv("AUTO_TUNE_PROGRESS_FILE", raising=False)
    monkeypatch.delenv("PIPELINE_RUN_ID", raising=False)
    monkeypatch.setattr(run_progress, "progress_dir", lambda directory=None: str(tmp_path / "progress"))
    run_progress.set_progress_file(None, "")
    run_progress.set_callback(None)
    yield
    run_progress.set_progress_file(None, "")
    run_progress.set_callback(None)


def train_a_run():
    """Drive one trainer's worth of milestones, then finish it."""
    tp.start("retrain", total_formats=2)
    tp.data_loaded("retrain", "T20I", rows=1200, features=42, targets=5)
    tp.columns_dropped("retrain", "T20I", ["weather_composite"], kept=41)
    tp.fitting("retrain", "T20I", 1200, 41)
    tp.format_done("retrain", "T20I", {"rmse": 24.1})
    tp.data_loaded("retrain", "ODI", rows=900, features=42, targets=5)
    tp.format_done("retrain", "ODI", {"rmse": 31.7})
    tp.finish("retrain", saved=2)


def test_the_summary_outlives_the_progress_file():
    train_a_run()

    assert training_orchestrator.get_step_progress("retrain") == {}, "progress is gone, as it should be"
    summary = training_orchestrator.get_run_summary("retrain")
    assert summary, "the outcome must survive the run that produced it"
    assert summary["saved"] == 2
    assert summary["formats_completed"] == 2


def test_the_summary_records_each_format():
    train_a_run()
    summary = training_orchestrator.get_run_summary("retrain")

    by_format = {f["format"]: f for f in summary["formats"]}
    assert set(by_format) == {"T20I", "ODI"}
    assert by_format["T20I"]["rows"] == 1200
    assert by_format["T20I"]["features"] == 42
    assert by_format["T20I"]["metrics"] == {"rmse": 24.1}
    assert by_format["ODI"]["metrics"] == {"rmse": 31.7}


# The plan singles this out: a silently constant feature was being dropped with nobody
# told. Keeping it in the run's permanent record is what makes it answerable later.
def test_the_summary_remembers_the_dropped_columns():
    train_a_run()
    summary = training_orchestrator.get_run_summary("retrain")

    assert summary["dropped_columns"] == {"T20I": ["weather_composite"]}


def test_the_summary_records_artifacts(tmp_path):
    model = tmp_path / "batting_model_T20I.joblib"
    model.write_bytes(b"x" * 4096)

    tp.start("retrain", total_formats=1)
    tp.artifact_written("batting", "T20I", [str(model)])
    tp.format_done("retrain", "T20I")
    tp.finish("retrain", saved=1)

    summary = training_orchestrator.get_run_summary("retrain")
    artifacts = summary["formats"][0]["artifacts"]
    assert artifacts == [{"path": "batting_model_T20I.joblib", "bytes": 4096}]


def test_a_second_run_does_not_inherit_the_first():
    """Two runs in one process must not merge. start() resets the accumulator."""
    train_a_run()

    tp.start("retrain", total_formats=1)
    tp.data_loaded("retrain", "TEST", rows=10, features=5)
    tp.format_done("retrain", "TEST")
    tp.finish("retrain", saved=1)

    summary = training_orchestrator.get_run_summary("retrain")
    assert [f["format"] for f in summary["formats"]] == ["TEST"]
    assert summary["saved"] == 1
    assert summary["dropped_columns"] == {}, "the first run's drops are not this run's"


def test_a_run_that_never_finished_has_no_summary():
    """A crashed run leaves progress but no result; {} says so rather than guessing."""
    tp.start("retrain", total_formats=1)
    tp.data_loaded("retrain", "T20I", 10, 5)

    assert training_orchestrator.get_run_summary("retrain") == {}


def test_summary_does_not_confuse_steps():
    tp.start("retrain", total_formats=1)
    tp.format_done("retrain", "T20I")
    tp.finish("retrain", saved=1)

    tp.start("evaluate", total_formats=1)
    tp.format_done("evaluate", "ODI")
    tp.finish("evaluate", saved=1)

    assert training_orchestrator.get_run_summary("retrain")["formats"][0]["format"] == "T20I"
    assert training_orchestrator.get_run_summary("evaluate")["formats"][0]["format"] == "ODI"


def test_get_run_summary_requires_a_step():
    assert training_orchestrator.get_run_summary("") == {}
    assert training_orchestrator.get_run_summary("   ") == {}


def test_get_run_summary_by_run_id(monkeypatch):
    monkeypatch.setenv("PIPELINE_RUN_ID", "named-run")
    train_a_run()

    assert training_orchestrator.get_run_summary("retrain", "named-run")["saved"] == 2
    assert training_orchestrator.get_run_summary("retrain", "other-run") == {}


# The result file also ends in .json, and would be read as live progress if the
# progress listing did not exclude it -- exactly the "a finished run looks like a
# running one" confusion the two lifetimes exist to avoid.
def test_a_result_file_is_never_mistaken_for_live_progress():
    train_a_run()

    assert training_orchestrator.get_step_progress("retrain") == {}
    assert run_progress.list_for_step("retrain") == []


def test_train_response_carries_the_summary():
    train_a_run()

    body = training_orchestrator.train_response("retrain")
    assert body["status"] == "ok"
    assert body["step"] == "retrain"
    assert body["summary"]["saved"] == 2


def test_train_response_omits_an_absent_summary():
    """An uninstrumented step reports status only, rather than an empty object."""
    body = training_orchestrator.train_response("evaluate")

    assert body == {"status": "ok", "step": "evaluate"}
