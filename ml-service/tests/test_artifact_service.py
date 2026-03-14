"""Unit tests for app.artifact_service (find_per_format_artifact, find_legacy_artifact, health helpers)."""

from unittest.mock import patch

from app.artifact_service import (
    build_artifacts_status,
    build_health_response,
    find_artifact,
    find_legacy_artifact,
    find_per_format_artifact,
    legacy_status_obj,
)


def test_find_per_format_artifact_listdir_oserror_returns_none():
    """When listdir raises OSError, find_per_format_artifact returns None."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        assert find_per_format_artifact("/nonexistent", "ODI", "batting") is None
        assert find_per_format_artifact("/nonexistent", "T20", "win") is None


def test_find_per_format_artifact_unknown_kind_returns_none(tmp_path):
    """Unknown kind returns None."""
    assert find_per_format_artifact(str(tmp_path), "ODI", "unknown") is None


def test_find_per_format_artifact_missing_scaler_returns_none(tmp_path):
    """When scaler file is missing, returns None (batting requires both scaler and model)."""
    (tmp_path / "batting_model_ODI.joblib").write_bytes(b"")
    assert find_per_format_artifact(str(tmp_path), "ODI", "batting") is None


def test_find_per_format_artifact_missing_model_returns_none(tmp_path):
    """When model file is missing, returns None."""
    (tmp_path / "batting_scaler_ODI.joblib").write_bytes(b"")
    assert find_per_format_artifact(str(tmp_path), "ODI", "batting") is None


def test_find_per_format_artifact_path_not_file_returns_none(tmp_path):
    """When path is a directory (not a file), returns None."""
    (tmp_path / "batting_scaler_ODI.joblib").write_bytes(b"")
    (tmp_path / "batting_model_ODI.joblib").mkdir()
    assert find_per_format_artifact(str(tmp_path), "ODI", "batting") is None


def test_find_per_format_artifact_stat_raises_returns_none(tmp_path):
    """When os.stat raises, returns None."""
    (tmp_path / "batting_scaler_ODI.joblib").write_bytes(b"")
    (tmp_path / "batting_model_ODI.joblib").write_bytes(b"")
    with patch("os.stat", side_effect=OSError(13, "Permission denied")):
        assert find_per_format_artifact(str(tmp_path), "ODI", "batting") is None


def test_find_per_format_artifact_success_returns_path_and_mtime(tmp_path):
    """When both scaler and model exist, returns (path, mtime)."""
    (tmp_path / "batting_scaler_ODI.joblib").write_bytes(b"x")
    (tmp_path / "batting_model_ODI.joblib").write_bytes(b"y")
    result = find_per_format_artifact(str(tmp_path), "ODI", "batting")
    assert result is not None
    path, mtime = result
    assert path.endswith("batting_model_ODI.joblib")
    assert isinstance(mtime, (int, float))


def test_find_per_format_artifact_extras_and_win_no_scaler(tmp_path):
    """Extras and win kinds do not require scaler; model only."""
    (tmp_path / "extras_model_ODI.joblib").write_bytes(b"")
    result = find_per_format_artifact(str(tmp_path), "ODI", "extras")
    assert result is not None
    (tmp_path / "win_model_T20.joblib").write_bytes(b"")
    result2 = find_per_format_artifact(str(tmp_path), "T20", "win")
    assert result2 is not None


def test_find_artifact_delegates_to_find_per_format():
    """find_artifact(batting=True/False) delegates to find_per_format_artifact."""
    with patch("app.artifact_service.find_per_format_artifact", return_value=("/p", 123.0)) as m:
        assert find_artifact("/d", "ODI", batting=True) == ("/p", 123.0)
        m.assert_called_once_with("/d", "ODI", "batting")
    with patch("app.artifact_service.find_per_format_artifact", return_value=None):
        assert find_artifact("/d", "T20", batting=False) is None


def test_find_legacy_artifact_listdir_oserror_returns_none():
    """When listdir raises OSError, find_legacy_artifact returns None."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        assert find_legacy_artifact("/nonexistent", "batting") is None


def test_find_legacy_artifact_missing_files_returns_none(tmp_path):
    """When scaler or model is missing, returns None."""
    assert find_legacy_artifact(str(tmp_path), "batting") is None
    (tmp_path / "batting_model.joblib").write_bytes(b"")
    assert find_legacy_artifact(str(tmp_path), "batting") is None


def test_find_legacy_artifact_unknown_kind_returns_none(tmp_path):
    """Unknown kind returns None."""
    assert find_legacy_artifact(str(tmp_path), "unknown") is None


def test_find_legacy_artifact_path_not_file_returns_none(tmp_path):
    """When path is directory, returns None."""
    (tmp_path / "batting_scaler.joblib").write_bytes(b"")
    (tmp_path / "batting_model.joblib").mkdir()
    assert find_legacy_artifact(str(tmp_path), "batting") is None


def test_find_legacy_artifact_stat_raises_returns_none(tmp_path):
    """When os.stat raises, returns None."""
    (tmp_path / "batting_scaler.joblib").write_bytes(b"")
    (tmp_path / "batting_model.joblib").write_bytes(b"")
    with patch("os.stat", side_effect=OSError(13, "Permission denied")):
        assert find_legacy_artifact(str(tmp_path), "batting") is None


def test_find_legacy_artifact_success(tmp_path):
    """When legacy files exist, returns (path, mtime)."""
    (tmp_path / "batting_scaler.joblib").write_bytes(b"")
    (tmp_path / "batting_model.joblib").write_bytes(b"")
    result = find_legacy_artifact(str(tmp_path), "batting")
    assert result is not None
    path, mtime = result
    assert path.endswith("batting_model.joblib")


def test_legacy_status_obj_exists_false_when_not_found(tmp_path):
    """legacy_status_obj has exists=False when find_legacy_artifact returns None."""
    out = legacy_status_obj(str(tmp_path), "batting", loaded=False)
    assert out["exists"] is False
    assert out["loaded"] is False


def test_legacy_status_obj_exists_true_when_found(tmp_path):
    """legacy_status_obj has exists=True and path/modified when artifact found."""
    (tmp_path / "batting_scaler.joblib").write_bytes(b"")
    (tmp_path / "batting_model.joblib").write_bytes(b"")
    out = legacy_status_obj(str(tmp_path), "batting", loaded=True)
    assert out["exists"] is True
    assert "path" in out
    assert "modified" in out
    assert out["loaded"] is True


def test_build_artifacts_status_structure(tmp_path):
    """build_artifacts_status returns timestamp, root, formats, legacy."""
    status = build_artifacts_status(str(tmp_path))
    assert "timestamp" in status
    assert status["root"] == str(tmp_path)
    assert "formats" in status
    assert "ODI" in status["formats"]
    assert status["formats"]["ODI"]["batting"]["exists"] is False
    assert "legacy" in status
    assert status["legacy"]["batting"]["exists"] is False


def test_build_health_response_artifacts_info_listdir_oserror():
    """When listdir fails in _artifacts_info, health still returns structure."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        resp = build_health_response("/nonexistent")
    assert resp["status"] == "ok"
    assert resp["artifacts"]["batting"] == []
    assert resp["metadata"]["batting"] == []


def test_build_health_response_artifacts_info_stat_oserror(tmp_path):
    """When stat fails for one file, that file still appears with file key only."""
    (tmp_path / "batting_model_ODI.joblib").write_bytes(b"")
    with patch("os.stat", side_effect=OSError(13, "Permission denied")):
        resp = build_health_response(str(tmp_path))
    assert resp["status"] == "ok"
    batting_artifacts = resp["artifacts"]["batting"]
    assert len(batting_artifacts) == 1
    assert batting_artifacts[0]["file"] == "batting_model_ODI.joblib"
    # size_bytes/modified may be missing when stat failed
    assert "file" in batting_artifacts[0]


def test_build_health_response_metadata_info_listdir_oserror():
    """When listdir fails in _metadata_info, metadata list is empty."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        resp = build_health_response("/nonexistent")
    assert resp["metadata"]["batting"] == []
    assert resp["metadata"]["bowling"] == []
