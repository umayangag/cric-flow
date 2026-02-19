"""Unit tests for app.artifacts edge cases (listdir failure, missing model file, load exception)."""

import os
from unittest.mock import MagicMock, patch

import pytest

import app.artifacts as artifacts_mod


def test_load_per_format_listdir_failure(tmp_path):
    """When listdir raises OSError, _load_per_format returns without crashing."""
    with patch.object(artifacts_mod, "_load_legacy", side_effect=lambda _: None):
        with patch("os.listdir", side_effect=OSError(2, "No such file")):
            artifacts_mod.BAT_MODELS.clear()
            artifacts_mod.BOWL_MODELS.clear()
            artifacts_mod._load_per_format(str(tmp_path))
    assert artifacts_mod.BAT_MODELS == {}
    assert artifacts_mod.BOWL_MODELS == {}


def test_load_per_format_scaler_without_model_file(tmp_path):
    """When batting_scaler_X.joblib exists but batting_model_X.joblib does not, warning and skip."""
    (tmp_path / "batting_scaler_T20.joblib").write_bytes(b"x")
    with patch("joblib.load", return_value=MagicMock()):
        artifacts_mod.BAT_MODELS.clear()
        artifacts_mod._load_per_format(str(tmp_path))
    assert "T20" not in artifacts_mod.BAT_MODELS


def test_load_per_format_load_failure_logs_error(tmp_path):
    """When joblib.load raises for a file, error is logged and loop continues."""
    with patch("os.listdir", return_value=["batting_scaler_T20.joblib"]):
        with patch("joblib.load", side_effect=ValueError("bad format")):
            with patch.object(artifacts_mod.logger, "error") as mock_err:
                artifacts_mod.BAT_MODELS.clear()
                artifacts_mod._load_per_format(str(tmp_path))
                mock_err.assert_called()
    assert "T20" not in artifacts_mod.BAT_MODELS


def test_reload_returns_summary(tmp_path):
    """reload() returns summary with loaded_batting_formats etc."""
    with patch("joblib.load", side_effect=Exception("no files")):
        out = artifacts_mod.reload(str(tmp_path))
    assert "loaded_batting_formats" in out
    assert "loaded_bowling_formats" in out
    assert "legacy_batting" in out
    assert "legacy_bowling" in out


def test_summary_after_clear():
    """summary() returns sorted lists and legacy flags."""
    artifacts_mod.BAT_MODELS.clear()
    artifacts_mod.BOWL_MODELS.clear()
    artifacts_mod.FIELD_MODELS.clear()
    artifacts_mod.EXTRAS_MODELS.clear()
    artifacts_mod.WIN_MODELS.clear()
    s = artifacts_mod.summary()
    assert s["loaded_batting_formats"] == []
    assert s["loaded_bowling_formats"] == []
    assert s["legacy_batting"] is False
    assert s["legacy_bowling"] is False
