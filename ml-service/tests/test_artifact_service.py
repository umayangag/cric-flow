"""Unit tests for app.artifact_service (find_per_format_artifact, health helpers)."""

from unittest.mock import patch

from app.artifact_service import (
    ARTIFACT_KIND_NAMES,
    build_artifacts_status,
    build_health_response,
    find_per_format_artifact,
)


def test_find_per_format_artifact_listdir_oserror_returns_none():
    """When listdir raises OSError, find_per_format_artifact returns None."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        assert find_per_format_artifact("/nonexistent", "ODI", "win") is None
        assert find_per_format_artifact("/nonexistent", "T20", "win") is None


def test_find_per_format_artifact_unknown_kind_returns_none(tmp_path):
    """Unknown kind returns None."""
    assert find_per_format_artifact(str(tmp_path), "ODI", "unknown") is None


def test_find_per_format_artifact_missing_model_returns_none(tmp_path):
    """When model file is missing, returns None."""
    assert find_per_format_artifact(str(tmp_path), "ODI", "win") is None


def test_find_per_format_artifact_path_not_file_returns_none(tmp_path):
    """When path is a directory (not a file), returns None."""
    (tmp_path / "win_model_ODI.joblib").mkdir()
    assert find_per_format_artifact(str(tmp_path), "ODI", "win") is None


def test_find_per_format_artifact_stat_raises_returns_none(tmp_path):
    """When os.stat raises, returns None."""
    (tmp_path / "win_model_ODI.joblib").write_bytes(b"")
    with patch("os.stat", side_effect=OSError(13, "Permission denied")):
        assert find_per_format_artifact(str(tmp_path), "ODI", "win") is None


def test_find_per_format_artifact_success_returns_path_and_mtime(tmp_path):
    """When the model exists, returns (path, mtime). The win kind carries no scaler."""
    (tmp_path / "win_model_ODI.joblib").write_bytes(b"y")
    result = find_per_format_artifact(str(tmp_path), "ODI", "win")
    assert result is not None
    path, mtime = result
    assert path.endswith("win_model_ODI.joblib")
    assert isinstance(mtime, (int, float))


def test_build_artifacts_status_structure(tmp_path):
    """build_artifacts_status returns timestamp, root, formats."""
    status = build_artifacts_status(str(tmp_path))
    assert "timestamp" in status
    assert status["root"] == str(tmp_path)
    assert "formats" in status
    assert "ODI" in status["formats"]
    assert status["formats"]["ODI"]["win"]["exists"] is False
    assert "legacy" not in status


def test_build_health_response_artifacts_info_listdir_oserror():
    """When listdir fails in _artifacts_info, health still returns structure."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        resp = build_health_response("/nonexistent")
    assert resp["status"] == "ok"
    assert resp["artifacts"]["win"] == []
    assert resp["metadata"]["win"] == []


def test_build_health_response_artifacts_info_stat_oserror(tmp_path):
    """When stat fails for one file, that file still appears with file key only."""
    (tmp_path / "win_model_ODI.joblib").write_bytes(b"")
    with patch("os.stat", side_effect=OSError(13, "Permission denied")):
        resp = build_health_response(str(tmp_path))
    assert resp["status"] == "ok"
    batting_artifacts = resp["artifacts"]["win"]
    assert len(batting_artifacts) == 1
    assert batting_artifacts[0]["file"] == "win_model_ODI.joblib"
    # size_bytes/modified may be missing when stat failed
    assert "file" in batting_artifacts[0]


def test_build_health_response_metadata_info_listdir_oserror():
    """When listdir fails in _metadata_info, metadata list is empty."""
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        resp = build_health_response("/nonexistent")
    assert resp["metadata"]["win"] == []


def test_build_artifacts_status_covers_every_artifact_kind(tmp_path):
    """Every kind the loader knows about has a cell in the matrix, and no kind it does not."""
    status = build_artifacts_status(str(tmp_path))
    assert ARTIFACT_KIND_NAMES == ["win"], "P-5 left one legacy artifact family; P-6 removes it"
    for fmt, row in status["formats"].items():
        assert sorted(row) == sorted(ARTIFACT_KIND_NAMES), f"missing kinds for {fmt}"


def test_build_health_response_covers_every_artifact_kind(tmp_path):
    """Health reports loaded formats, files and counters for every kind."""
    resp = build_health_response(str(tmp_path))
    for name in ARTIFACT_KIND_NAMES:
        assert f"loaded_{name}_formats" in resp
        assert name in resp["artifacts"]
        assert f"{name}_formats" in resp["counters"]
