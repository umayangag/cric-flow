"""app.artifacts edge cases: an unreadable directory, a missing model file, a bad load.

One artifact family is left -- the windowed-form win classifier -- so these exercise the
kind-generic loader through it. The batting, bowling, fielding, extras, innings and share
registries went with their models in P-5.
"""

from unittest.mock import MagicMock, patch

import app.artifacts as artifacts_mod


def test_load_per_format_listdir_failure(tmp_path):
    """When listdir raises OSError, _load_per_format returns without crashing."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        artifacts_mod.WIN_MODELS.clear()
        artifacts_mod._load_per_format(str(tmp_path))
    assert artifacts_mod.WIN_MODELS == {}


def test_load_per_format_load_failure_logs_error(tmp_path):
    """When joblib.load raises for a file, the error is logged and the loop continues."""
    (tmp_path / "win_model_T20.joblib").write_bytes(b"x")
    with patch("app.artifacts.joblib.load", side_effect=ValueError("bad format")):
        with patch.object(artifacts_mod.logger, "error") as mock_err:
            artifacts_mod.WIN_MODELS.clear()
            artifacts_mod._load_per_format(str(tmp_path))
            mock_err.assert_called()
    assert "T20" not in artifacts_mod.WIN_MODELS


def test_load_per_format_skips_a_kind_whose_model_file_is_missing(tmp_path):
    """A listing that names a model file which is not there is a warning and a skip, not a
    crash: a half-written training run must not take the service down."""
    with patch("os.listdir", return_value=["win_model_T20.joblib"]):
        with patch.object(artifacts_mod.logger, "warning") as mock_warn:
            artifacts_mod.WIN_MODELS.clear()
            artifacts_mod._load_per_format(str(tmp_path))
            mock_warn.assert_called()
    assert "T20" not in artifacts_mod.WIN_MODELS


def test_load_per_format_win_success(tmp_path):
    """A per-format win model loads when its file exists, and records its mtime."""
    (tmp_path / "win_model_ODI.joblib").write_bytes(b"x")
    with patch("app.artifacts.joblib.load", return_value=MagicMock()):
        artifacts_mod.WIN_MODELS.clear()
        artifacts_mod._load_per_format(str(tmp_path))
    assert "ODI" in artifacts_mod.WIN_MODELS
    assert artifacts_mod.loaded_model_mtime("win", "ODI") is not None


def test_reload_returns_summary(tmp_path):
    """reload() reports the formats each kind loaded, and nothing it no longer has."""
    with patch("app.artifacts.joblib.load", side_effect=Exception("no files")):
        out = artifacts_mod.reload(str(tmp_path))
    assert "loaded_win_formats" in out
    assert "loaded_batting_formats" not in out


def test_summary_after_clear():
    """summary() returns sorted per-format lists."""
    artifacts_mod.WIN_MODELS.clear()
    s = artifacts_mod.summary()
    assert s["loaded_win_formats"] == []


def test_discover_format_codes_reads_the_prefix_off_the_listing():
    """Discovery is by filename prefix, upper-cased, and ignores everything else."""
    kind = artifacts_mod.ARTIFACT_KINDS_BY_NAME["win"]
    entries = ["win_model_t20.joblib", "win_model_ODI.joblib", "notes.txt", "xi_ratings.joblib"]

    assert sorted(artifacts_mod._discover_format_codes(entries, kind)) == ["ODI", "T20"]
