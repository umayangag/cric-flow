"""Unit tests for ml.training_progress, the trainers' milestone vocabulary."""

import threading

import pytest

from ml import run_progress
from ml import training_progress as tp


@pytest.fixture(autouse=True)
def captured():
    """Collect emitted payloads instead of writing files."""
    events = []
    run_progress.set_progress_file(None, "run-test")
    run_progress.set_callback(events.append)
    tp.set_total_formats(0)
    yield events
    run_progress.set_callback(None)
    run_progress.set_progress_file(None, "")


def phases(events):
    return [e.get("phase") for e in events]


def test_step_name_matches_the_pipeline_registry():
    """go-app, data_migrations and the ops console all speak in step ids.

    A second vocabulary here would need translating at every boundary, so the mapping
    is asserted rather than left to convention.
    """
    assert tp.step_name("retrain") == "retrain"
    assert tp.step_name("evaluate") == "evaluate"
    assert tp.step_name("") == "unknown", "a nameless step is named, not left blank"


def test_data_loaded_reports_shape(captured):
    tp.data_loaded("retrain", "T20I", rows=12345, features=42, targets=5)

    event = captured[0]
    assert event["step"] == "retrain"
    assert event["phase"] == tp.PHASE_LOAD
    assert event["metrics"] == {"rows": 12345, "features": 42, "targets": 5}
    assert event["format"] == "T20I"
    assert "12,345 rows" in event["message"]


# The plan singles this out: every trainer already called drop_low_variance_columns
# and every one discarded the result, so a feature that had gone silently constant was
# removed without anyone being told.
def test_columns_dropped_names_the_columns(captured):
    tp.columns_dropped("retrain", "ODI", ["weather_composite", "rain"], kept=40)

    event = captured[0]
    assert event["phase"] == tp.PHASE_FEATURES
    assert event["dropped_columns"] == ["weather_composite", "rain"]
    assert event["metrics"] == {"dropped": 2, "kept": 40}
    assert event["format"] == "ODI"


def test_columns_dropped_is_silent_when_nothing_was_dropped(captured):
    """A healthy run should not emit an event saying nothing happened."""
    tp.columns_dropped("retrain", "ODI", [], kept=42)
    tp.columns_dropped("retrain", "ODI", None, kept=42)

    assert captured == []


def test_columns_dropped_truncates_a_pathological_list(captured):
    """A progress file is not a log; the count is the signal, the names are detail."""
    names = [f"col_{i}" for i in range(tp.MAX_REPORTED_COLUMNS + 10)]
    tp.columns_dropped("retrain", None, names, kept=1)

    event = captured[0]
    assert len(event["dropped_columns"]) == tp.MAX_REPORTED_COLUMNS
    assert event["dropped_columns_truncated"] == 10
    assert event["metrics"]["dropped"] == len(names), "the count still reports every column"


def test_fold_reports_index_and_metrics(captured):
    tp.fold("retrain", "T20I", index=3, total_folds=5, metrics={"accuracy": 0.71, "brier": 0.19})

    event = captured[0]
    assert event["phase"] == tp.PHASE_CV
    assert event["current"] == 3
    assert event["total"] == 5
    assert event["metrics"] == {"accuracy": 0.71, "brier": 0.19}


def test_metrics_drop_values_that_are_not_json(captured):
    """A NaN from a degenerate fold serialises to the literal NaN, which is not JSON.

    One bad metric must not make the whole progress file unparseable.
    """
    tp.fold("retrain", None, 1, 2, {"accuracy": float("nan"), "brier": float("inf"), "log_loss": 0.4})

    assert captured[0]["metrics"] == {"log_loss": 0.4}


def test_metrics_survive_numpy_scalars(captured):
    np = pytest.importorskip("numpy")
    tp.fold("retrain", None, 1, 2, {"accuracy": np.float64(0.75), "n": np.int64(3)})

    assert captured[0]["metrics"] == {"accuracy": 0.75, "n": 3.0}


def test_artifact_written_reports_names_and_sizes(captured, tmp_path):
    model = tmp_path / "win_model_T20I.joblib"
    model.write_bytes(b"x" * 2048)
    scaler = tmp_path / "win_scaler_T20I.joblib"
    scaler.write_bytes(b"y" * 512)

    tp.artifact_written("retrain", "T20I", [str(scaler), str(model)])

    event = captured[0]
    assert event["phase"] == tp.PHASE_ARTIFACT
    assert event["metrics"]["artifact_bytes"] == 2560
    assert event["metrics"]["artifact_count"] == 2
    names = [a["path"] for a in event["artifacts"]]
    assert names == ["win_scaler_T20I.joblib", "win_model_T20I.joblib"]
    assert all("/" not in n for n in names), "basenames only; the full path is the server's business"


def test_artifact_written_skips_files_that_are_not_there(captured, tmp_path):
    tp.artifact_written("retrain", "T20I", [str(tmp_path / "never_written.joblib")])

    assert captured == [], "nothing was written, so there is nothing to report"


def test_format_done_advances_the_completed_count(captured):
    tp.set_total_formats(3)
    tp.format_done("retrain", "T20I")
    tp.format_done("retrain", "ODI")

    assert captured[0]["current"] == 1
    assert captured[0]["total"] == 3
    assert captured[1]["current"] == 2


# Formats train concurrently, so "which format is running" has no single answer.
# Counting completed formats is the number that stays true under concurrency.
def test_completed_count_is_correct_under_concurrency(captured):
    tp.set_total_formats(20)

    def worker(fmt):
        tp.format_done("retrain", fmt)

    threads = [threading.Thread(target=worker, args=(f"F{i}",)) for i in range(20)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()

    currents = sorted(e["current"] for e in captured)
    assert currents == list(range(1, 21)), "every format counted exactly once, none lost to a race"


def test_set_total_formats_starts_the_count_over(captured):
    """It is called before the loop, so it resets rather than preserves.

    A version that carried the completed count forward would make a second run in the
    same process start at "4 of 4".
    """
    tp.set_total_formats(4)
    tp.format_done("innings", "T20I")
    assert captured[-1]["current"] == 1

    tp.set_total_formats(2)
    tp.format_done("innings", "ODI")
    assert captured[-1]["current"] == 1, "the count restarted with the new total"
    assert captured[-1]["total"] == 2


def test_start_and_finish_bracket_a_run(captured, tmp_path):
    path = tp.start("retrain", total_formats=2, out_dir=str(tmp_path))
    assert path is not None
    tp.finish("retrain", saved=2)

    assert phases(captured) == [tp.PHASE_LOAD, tp.PHASE_DONE]
    assert captured[-1]["saved"] == 2


def test_finish_removes_the_progress_file(tmp_path):
    """A file left behind reads as a run still going."""
    run_progress.set_callback(None)
    path = tp.start("retrain", total_formats=1, out_dir=str(tmp_path))
    tp.data_loaded("retrain", "T20I", 10, 5)
    assert path and __import__("os").path.exists(path)

    tp.finish("retrain", saved=1)
    assert not __import__("os").path.exists(path)


# Telemetry must never fail a training run. These feed the emitters input a caller
# should not pass, and the requirement is simply that nothing propagates: a trainer
# that finished successfully must not be reported as failed because of a bad label.
@pytest.mark.parametrize(
    "call",
    [
        pytest.param(lambda: tp.data_loaded("retrain", None, "not-a-number", 5), id="non-numeric-rows"),
        pytest.param(lambda: tp.columns_dropped("batting", None, ["a"], "not-a-number"), id="non-numeric-kept"),
        pytest.param(lambda: tp.fold("batting", None, "x", "y", {"a": object()}), id="non-numeric-fold"),
        pytest.param(lambda: tp.artifact_written("batting", None, [None]), id="none-path"),
        pytest.param(lambda: tp.format_done("retrain", None, {"a": object()}), id="unserialisable-metric"),
        pytest.param(lambda: tp.step_name(None), id="no-model-name"),
    ],
)
def test_emitters_never_raise_on_bad_input(call, captured):
    call()


def test_emitters_swallow_their_own_failures(captured, monkeypatch):
    """If rendering an event blows up, the training run carries on regardless."""

    def exploding_emit(_event):
        raise RuntimeError("transport on fire")

    monkeypatch.setattr(run_progress, "emit", exploding_emit)

    tp.data_loaded("retrain", "T20I", 10, 5)
    tp.columns_dropped("batting", "T20I", ["a"], 4)
    tp.fold("batting", "T20I", 1, 2, {"x": 1.0})
    tp.fitting("batting", "T20I", 10, 5)
    tp.format_done("retrain", "T20I")
    tp.finish("retrain", 1)
