"""Tests for `/artifacts/status` staleness: does the loaded object come from the file on disk?

`loaded` alone cannot answer that. Training rewrites an artifact under a long-running
process, which keeps serving what it loaded at startup until something reloads it.
"""

import os

import joblib
import pytest

import app.artifacts as artifacts
from app.artifact_service import build_artifacts_status


@pytest.fixture(autouse=True)
def clear_registries(tmp_path_factory):
    """Leave the module-level registries empty so registry state cannot leak between tests."""
    yield
    artifacts.reload(str(tmp_path_factory.mktemp("empty_models_dir")))


def _write_win_model(models_dir) -> str:
    model_path = models_dir / "win_model_T20.joblib"
    joblib.dump({}, model_path)
    return str(model_path)


def _win_cell(models_dir) -> dict:
    return build_artifacts_status(str(models_dir))["formats"]["T20"]["win"]


def test_freshly_reloaded_artifact_is_loaded_and_not_stale(tmp_path):
    """A model loaded from the file currently on disk reports loaded, not stale."""
    _write_win_model(tmp_path)

    artifacts.reload(str(tmp_path))
    cell = _win_cell(tmp_path)

    assert cell["loaded"] is True
    assert cell["stale"] is False
    assert cell["loaded_modified"] == cell["modified"]


def test_artifact_rewritten_after_load_is_reported_stale(tmp_path):
    """Retraining after load leaves the old object serving, which is what stale means."""
    model_path = _write_win_model(tmp_path)
    artifacts.reload(str(tmp_path))

    retrained_at = os.path.getmtime(model_path) + 60
    os.utime(model_path, (retrained_at, retrained_at))
    cell = _win_cell(tmp_path)

    assert cell["loaded"] is True
    assert cell["stale"] is True


def test_registry_entry_this_process_did_not_load_is_reported_stale(tmp_path, monkeypatch):
    """An entry that cannot be attributed to a loaded file is not claimed to be current."""
    _write_win_model(tmp_path)
    monkeypatch.setitem(artifacts.WIN_MODELS, "T20", object())

    cell = _win_cell(tmp_path)

    assert cell["loaded"] is True
    assert cell["stale"] is True
    assert "loaded_modified" not in cell


def test_artifact_on_disk_but_not_loaded_has_no_stale_verdict(tmp_path):
    """Nothing is loaded, so there is nothing to call stale — only exists is reported."""
    _write_win_model(tmp_path)

    cell = _win_cell(tmp_path)

    assert cell["exists"] is True
    assert "loaded" not in cell
    assert "stale" not in cell
