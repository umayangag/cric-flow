"""Unit tests for ml.auto_tune_progress (progress reporting for auto-tune)."""

import tempfile
from pathlib import Path
from unittest.mock import patch

from ml.auto_tune_progress import (
    clear_progress,
    get_progress_file,
    set_progress_callback,
    set_progress_file,
    write_progress,
)


def test_set_and_get_progress_file():
    """set_progress_file and get_progress_file work correctly."""
    set_progress_file(None)
    assert get_progress_file() is None
    set_progress_file("/tmp/progress.json")
    assert get_progress_file() == "/tmp/progress.json"
    set_progress_file(None)
    assert get_progress_file() is None


def test_set_progress_callback():
    """set_progress_callback stores the callback."""
    set_progress_callback(None)
    captured = []

    def cb(payload):
        captured.append(payload)

    set_progress_callback(cb)
    write_progress("tuning", "batting", "T20", 1, 3, algorithm="ridge")
    assert len(captured) == 1
    assert captured[0]["phase"] == "tuning"
    assert captured[0]["model_kind"] == "batting"
    assert captured[0]["algorithm"] == "ridge"
    set_progress_callback(None)


def test_write_progress_minimal():
    """write_progress with minimal args builds correct payload."""
    captured = []
    set_progress_callback(lambda p: captured.append(p))
    write_progress("phase", "bowling", None, 2, 5)
    assert captured[0] == {
        "phase": "phase",
        "model_kind": "bowling",
        "format_suffix": "",
        "task_index": 2,
        "task_total": 5,
    }
    set_progress_callback(None)


def test_write_progress_full():
    """write_progress with all optional args includes them in payload."""
    captured = []
    set_progress_callback(lambda p: captured.append(p))
    write_progress(
        phase="tuning",
        model_kind="extras",
        format_suffix="ODI",
        task_index=1,
        task_total=2,
        algorithm="lgbm",
        hyperparams={"n_estimators": 100},
        trial=3,
        trials_total=10,
        best_score=0.85,
        best_algorithm="ridge",
        message="Trial 3 complete",
        algorithms_screened=["ridge", "lgbm"],
    )
    p = captured[0]
    assert p["algorithm"] == "lgbm"
    assert p["hyperparams"] == {"n_estimators": 100}
    assert p["trial"] == 3
    assert p["trials_total"] == 10
    assert p["best_score"] == 0.85
    assert p["best_algorithm"] == "ridge"
    assert p["message"] == "Trial 3 complete"
    assert p["algorithms_screened"] == ["ridge", "lgbm"]
    assert p["format_suffix"] == "ODI"
    set_progress_callback(None)


def test_write_progress_to_file():
    """write_progress writes JSON to file when path is set."""
    with tempfile.TemporaryDirectory() as td:
        path = str(Path(td) / "subdir" / "progress.json")
        set_progress_file(path)
        set_progress_callback(None)
        write_progress("phase", "batting", "T20", 1, 2)
        set_progress_file(None)
        content = Path(path).read_text()
        assert '"phase": "phase"' in content
        assert '"model_kind": "batting"' in content
        assert Path(path).parent.exists()


def test_write_progress_no_file_when_path_none():
    """write_progress does nothing when path is None."""
    set_progress_file(None)
    set_progress_callback(None)
    write_progress("phase", "batting", None, 1, 1)  # no file created, no error


def test_clear_progress_removes_file():
    """clear_progress removes the progress file when it exists."""
    with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
        path = f.name
    try:
        Path(path).write_text("{}")
        set_progress_file(path)
        clear_progress()
        set_progress_file(None)
        assert not Path(path).exists()
    finally:
        if Path(path).exists():
            Path(path).unlink()


def test_clear_progress_ignores_missing_file():
    """clear_progress does nothing when file does not exist."""
    set_progress_file("/nonexistent/path/progress.json")
    clear_progress()  # no error
    set_progress_file(None)


def test_callback_exception_logged_not_raised():
    """When callback raises, exception is logged and not propagated."""
    def failing_cb(_):
        raise ValueError("callback error")

    set_progress_callback(failing_cb)
    write_progress("phase", "batting", None, 1, 1)  # no exception
    set_progress_callback(None)


def test_write_progress_oserror_logged(tmp_path):
    """When writing to file fails (e.g. permission), OSError is logged not raised."""
    # Use a path where we can trigger OSError: write to a directory as if it were a file
    dir_path = tmp_path / "isdir"
    dir_path.mkdir()
    set_progress_file(str(dir_path))
    set_progress_callback(None)
    write_progress("phase", "batting", None, 1, 1)  # OSError when opening dir for write
    set_progress_file(None)


def test_clear_progress_oserror_logged(tmp_path):
    """When removing file fails, OSError is logged not raised."""
    path = tmp_path / "progress.json"
    path.write_text("{}")
    set_progress_file(str(path))

    def failing_remove(_):
        raise OSError(13, "Permission denied")

    with patch("ml.auto_tune_progress.os.remove", failing_remove):
        clear_progress()
    set_progress_file(None)
