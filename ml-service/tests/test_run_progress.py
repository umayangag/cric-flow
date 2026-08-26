"""Unit tests for ml.run_progress, the shared cross-process progress channel."""

import json
import os
import time
from pathlib import Path
from unittest.mock import patch

import pytest

from ml import run_progress
from ml.run_progress import Event


@pytest.fixture(autouse=True)
def reset_channel():
    """Each test starts with a disabled channel and no observer."""
    run_progress.set_progress_file(None, "")
    run_progress.set_callback(None)
    yield
    run_progress.set_progress_file(None, "")
    run_progress.set_callback(None)


def read_json(path) -> dict:
    return json.loads(Path(path).read_text())


def test_event_carries_the_versioned_envelope():
    """The schema is versioned from the start so a reader knows what it is holding."""
    payload = Event(step="train_batting", phase="cv", current=3, total=5, metrics={"rmse": 24.1}).to_payload("run-7")

    assert payload["v"] == run_progress.SCHEMA_VERSION
    assert payload["run_id"] == "run-7"
    assert payload["step"] == "train_batting"
    assert payload["phase"] == "cv"
    assert payload["current"] == 3
    assert payload["total"] == 5
    assert payload["metrics"] == {"rmse": 24.1}
    assert payload["ts"].endswith("Z")


def test_event_omits_what_was_not_given():
    """Absent is absent: a null metric is not the same as no metric."""
    payload = Event(step="train_win").to_payload("run-1")

    for key in ("phase", "current", "total", "metrics", "message"):
        assert key not in payload


def test_current_zero_is_reported_not_dropped():
    """Fold 0 of 5 is real progress; a falsy-check would silently drop it."""
    payload = Event(step="train_win", current=0, total=5).to_payload("run-1")

    assert payload["current"] == 0
    assert payload["total"] == 5


def test_extra_is_merged_at_the_top_level():
    """Step-specific fields sit beside the envelope, where existing readers find them."""
    payload = Event(step="auto_tune", extra={"algorithm": "ridge", "trial": 4}).to_payload("run-1")

    assert payload["algorithm"] == "ridge"
    assert payload["trial"] == 4


def test_extra_cannot_overwrite_the_envelope():
    """A step-specific field shadowing run_id would make the event describe another run."""
    payload = Event(step="train_win", extra={"run_id": "spoofed", "step": "other", "v": 99}).to_payload("run-1")

    assert payload["run_id"] == "run-1"
    assert payload["step"] == "train_win"
    assert payload["v"] == run_progress.SCHEMA_VERSION


def test_configure_gives_each_run_its_own_file(tmp_path):
    """One file per run: two runs of one step must not overwrite each other."""
    first = run_progress.configure("train_batting", run_id="run-a", directory=str(tmp_path))
    run_progress.emit(Event(step="train_batting", phase="first"))

    second = run_progress.configure("train_batting", run_id="run-b", directory=str(tmp_path))
    run_progress.emit(Event(step="train_batting", phase="second"))

    assert first != second
    assert read_json(first)["phase"] == "first", "the earlier run's state must survive"
    assert read_json(second)["phase"] == "second"


def test_configure_defaults_the_run_id_to_the_pid(tmp_path):
    """A caller with no run id of its own still gets a distinct file."""
    path = run_progress.configure("train_win", directory=str(tmp_path))

    assert str(os.getpid()) in path
    assert run_progress.get_run_id() == str(os.getpid())


def test_run_id_is_sanitised_into_the_filename(tmp_path):
    """Run ids arrive from go-app and from env, so they choose no paths of their own."""
    path = run_progress.configure("train_win", run_id="../../etc/passwd", directory=str(tmp_path))

    resolved = Path(path).resolve()
    assert str(tmp_path.resolve()) in str(resolved), "a crafted run id must not escape the directory"
    assert ".." not in Path(path).name


def test_emit_writes_the_file(tmp_path):
    path = run_progress.configure("train_extras", run_id="r1", directory=str(tmp_path))
    run_progress.emit(Event(step="train_extras", phase="loading", current=1, total=4))

    payload = read_json(path)
    assert payload["phase"] == "loading"
    assert payload["run_id"] == "r1"


def test_emit_leaves_no_temp_files_behind(tmp_path):
    """Writes go via a temp file; a directory littered with them is a reader's problem."""
    run_progress.configure("train_extras", run_id="r1", directory=str(tmp_path))
    for i in range(5):
        run_progress.emit(Event(step="train_extras", current=i, total=5))

    files = sorted(p.name for p in Path(run_progress.progress_dir(str(tmp_path))).iterdir())
    assert files == ["train_extras__r1.json"]


def test_write_is_atomic_via_rename(tmp_path):
    """A reader polling the file sees the old event or the new one, never half of either.

    Asserted through os.replace rather than by racing a reader: the guarantee is that
    the visible file is only ever swapped whole, and that is what replace provides.
    """
    run_progress.configure("train_bowling", run_id="r1", directory=str(tmp_path))

    seen = []
    real_replace = os.replace

    def spy(src, dst):
        seen.append((src, dst))
        return real_replace(src, dst)

    with patch("ml.run_progress.os.replace", spy):
        run_progress.emit(Event(step="train_bowling", phase="cv"))

    assert len(seen) == 1
    src, dst = seen[0]
    assert Path(src).parent == Path(dst).parent, "replace is only atomic within one filesystem"


def test_emit_without_a_file_is_a_no_op():
    """A step with no configured channel still runs."""
    run_progress.set_progress_file(None)
    run_progress.emit(Event(step="train_win", phase="cv"))  # no error


def test_emit_never_raises_when_the_directory_is_unwritable(tmp_path):
    """Progress is telemetry: a full or read-only disk must not fail a training run."""
    blocked = tmp_path / "blocked"
    blocked.write_text("not a directory")
    run_progress.set_progress_file(str(blocked / "progress" / "x.json"), "r1")

    run_progress.emit(Event(step="train_win", phase="cv"))  # no error


def test_callback_failure_never_reaches_the_caller():
    def boom(_):
        raise ValueError("observer exploded")

    run_progress.set_callback(boom)
    run_progress.emit(Event(step="train_win"))  # no error


def test_clear_removes_the_file(tmp_path):
    path = run_progress.configure("train_win", run_id="r1", directory=str(tmp_path))
    run_progress.emit(Event(step="train_win"))
    assert Path(path).exists()

    run_progress.clear()
    assert not Path(path).exists()


def test_clear_is_quiet_when_there_is_nothing_to_remove(tmp_path):
    run_progress.configure("train_win", run_id="r1", directory=str(tmp_path))
    run_progress.clear()
    run_progress.clear()  # already gone; no error


def test_read_returns_empty_for_missing_or_corrupt(tmp_path):
    assert run_progress.read(str(tmp_path / "nope.json")) == {}

    corrupt = tmp_path / "corrupt.json"
    corrupt.write_text("{not json")
    assert run_progress.read(str(corrupt)) == {}

    not_an_object = tmp_path / "list.json"
    not_an_object.write_text("[1, 2, 3]")
    assert run_progress.read(str(not_an_object)) == {}, "a JSON array is not a progress event"


def test_latest_for_step_picks_the_newest_run(tmp_path):
    run_progress.configure("train_batting", run_id="old", directory=str(tmp_path))
    run_progress.emit(Event(step="train_batting", phase="old-run"))
    old_path = run_progress.get_progress_file()
    os.utime(old_path, (time.time() - 60, time.time() - 60))

    run_progress.configure("train_batting", run_id="new", directory=str(tmp_path))
    run_progress.emit(Event(step="train_batting", phase="new-run"))

    latest = run_progress.latest_for_step("train_batting", directory=str(tmp_path))
    assert latest["phase"] == "new-run"


def test_latest_for_step_skips_a_stale_file(tmp_path):
    """A crashed run leaves its file behind; reporting it shows a run that is not happening."""
    path = run_progress.configure("train_batting", run_id="crashed", directory=str(tmp_path))
    run_progress.emit(Event(step="train_batting", phase="abandoned"))
    long_ago = time.time() - (run_progress.STALE_AFTER_SEC + 60)
    os.utime(path, (long_ago, long_ago))

    assert run_progress.latest_for_step("train_batting", directory=str(tmp_path)) == {}
    # Opting out is available for a caller that wants the last known state regardless.
    assert (
        run_progress.latest_for_step("train_batting", directory=str(tmp_path), stale_after_sec=0)["phase"]
        == "abandoned"
    )


def test_latest_for_step_does_not_confuse_steps(tmp_path):
    run_progress.configure("train_batting", run_id="r1", directory=str(tmp_path))
    run_progress.emit(Event(step="train_batting", phase="batting-phase"))
    run_progress.configure("train_bowling", run_id="r1", directory=str(tmp_path))
    run_progress.emit(Event(step="train_bowling", phase="bowling-phase"))

    assert run_progress.latest_for_step("train_batting", directory=str(tmp_path))["phase"] == "batting-phase"
    assert run_progress.latest_for_step("train_bowling", directory=str(tmp_path))["phase"] == "bowling-phase"


def test_latest_for_step_is_empty_when_nothing_ran(tmp_path):
    assert run_progress.latest_for_step("train_win", directory=str(tmp_path)) == {}


def test_safe_component_never_returns_empty():
    """A name reduced to nothing still has to produce a usable filename."""
    assert run_progress.safe_component("...") == "unknown"
    assert run_progress.safe_component("") == "unknown"
    assert run_progress.safe_component("train/batting") == "train_batting"
