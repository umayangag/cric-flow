"""Unit tests for app.artifacts edge cases (listdir failure, missing model file, load exception)."""

from unittest.mock import MagicMock, patch

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
    with patch("app.artifacts.joblib.load", return_value=MagicMock()):
        artifacts_mod.BAT_MODELS.clear()
        artifacts_mod._load_per_format(str(tmp_path))
    assert "T20" not in artifacts_mod.BAT_MODELS


def test_load_per_format_load_failure_logs_error(tmp_path):
    """When joblib.load raises for a file, error is logged and loop continues."""
    with patch("os.listdir", return_value=["batting_scaler_T20.joblib"]):
        with patch("app.artifacts.joblib.load", side_effect=ValueError("bad format")):
            with patch.object(artifacts_mod.logger, "error") as mock_err:
                artifacts_mod.BAT_MODELS.clear()
                artifacts_mod._load_per_format(str(tmp_path))
                mock_err.assert_called()
    assert "T20" not in artifacts_mod.BAT_MODELS


def test_reload_returns_summary(tmp_path):
    """reload() returns summary with loaded_batting_formats etc."""
    with patch("app.artifacts.joblib.load", side_effect=Exception("no files")):
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


def test_load_legacy_batting_fail_skips():
    """When batting_scaler or batting_model joblib.load fails, legacy batting is skipped."""
    with patch("app.artifacts.joblib.load", side_effect=Exception("bad pickle")):
        artifacts_mod.BAT_MODELS.clear()
        artifacts_mod._load_legacy(str("/nonexistent"))
    assert "_LEGACY_" not in artifacts_mod.BAT_MODELS


def test_load_legacy_bowling_fail_skips():
    """When bowling joblib.load fails, legacy bowling is skipped."""
    with patch("app.artifacts.joblib.load", side_effect=[MagicMock(), MagicMock(), Exception("bad")]):
        artifacts_mod.BAT_MODELS.clear()
        artifacts_mod.BOWL_MODELS.clear()
        artifacts_mod._load_legacy(str("/nonexistent"))
    assert "_LEGACY_" in artifacts_mod.BAT_MODELS
    assert "_LEGACY_" not in artifacts_mod.BOWL_MODELS


def test_load_legacy_fielding_extras_win_fail_skips(tmp_path):
    """When fielding/extras/win joblib.load fails, those legacy models are skipped."""
    def load_side_effect(path, *args, **kwargs):
        # Check filename only (path may contain "fielding" from test name)
        import os
        fname = os.path.basename(str(path)).lower()
        if "fielding" in fname or "extras" in fname or fname.startswith("win_"):
            raise Exception("bad")
        return MagicMock()

    with patch("app.artifacts.joblib.load", side_effect=load_side_effect):
        artifacts_mod.BAT_MODELS.clear()
        artifacts_mod.BOWL_MODELS.clear()
        artifacts_mod.FIELD_MODELS.clear()
        artifacts_mod.EXTRAS_MODELS.clear()
        artifacts_mod.WIN_MODELS.clear()
        artifacts_mod._load_legacy(str(tmp_path))
    assert "_LEGACY_" in artifacts_mod.BAT_MODELS
    assert "_LEGACY_" in artifacts_mod.BOWL_MODELS
    assert "_LEGACY_" not in artifacts_mod.FIELD_MODELS
    assert "_LEGACY_" not in artifacts_mod.EXTRAS_MODELS
    assert "_LEGACY_" not in artifacts_mod.WIN_MODELS


def test_load_per_format_bowling_scaler_without_model(tmp_path):
    """When bowling_scaler_X exists but bowling_model_X does not, warning and skip."""
    (tmp_path / "bowling_scaler_ODI.joblib").write_bytes(b"x")
    with patch("app.artifacts.joblib.load", return_value=MagicMock()):
        artifacts_mod.BOWL_MODELS.clear()
        artifacts_mod._load_per_format(str(tmp_path))
    assert "ODI" not in artifacts_mod.BOWL_MODELS


def test_load_per_format_fielding_scaler_without_model(tmp_path):
    """When fielding_scaler_X exists but fielding_model_X does not, warning and skip."""
    (tmp_path / "fielding_scaler_T20.joblib").write_bytes(b"x")
    with patch("app.artifacts.joblib.load", return_value=MagicMock()):
        artifacts_mod.FIELD_MODELS.clear()
        artifacts_mod._load_per_format(str(tmp_path))
    assert "T20" not in artifacts_mod.FIELD_MODELS


def test_load_per_format_extras_and_win_success(tmp_path):
    """Per-format extras_model and win_model load successfully when files exist."""
    (tmp_path / "extras_model_ODI.joblib").write_bytes(b"x")
    (tmp_path / "win_model_ODI.joblib").write_bytes(b"x")
    mock_model = MagicMock()
    with patch("app.artifacts.joblib.load", return_value=mock_model):
        artifacts_mod.EXTRAS_MODELS.clear()
        artifacts_mod.WIN_MODELS.clear()
        artifacts_mod._load_per_format(str(tmp_path))
    assert "ODI" in artifacts_mod.EXTRAS_MODELS
    assert "ODI" in artifacts_mod.WIN_MODELS
