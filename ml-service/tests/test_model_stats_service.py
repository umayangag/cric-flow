"""Tests for model_stats_service: dynamic model-kind discovery."""

import os
from typing import Optional, Tuple

import pytest

from app.model_stats_service import (
    build_model_stats,
    get_model_artifact_stats,
    parse_model_filename,
)


class TestParseModelFilename:
    """parse_model_filename should recognise any <kind>_model[_<FORMAT>].joblib pattern."""

    @pytest.mark.parametrize(
        "fname,expected",
        [
            ("batting_model.joblib", ("batting", None)),
            ("batting_model_ODI.joblib", ("batting", "ODI")),
            ("batting_model_T20.joblib", ("batting", "T20")),
            ("bowling_model.joblib", ("bowling", None)),
            ("bowling_model_T20I.joblib", ("bowling", "T20I")),
            ("fielding_model_TEST.joblib", ("fielding", "TEST")),
            ("extras_model.joblib", ("extras", None)),
            ("extras_model_ODI.joblib", ("extras", "ODI")),
            ("win_model.joblib", ("win", None)),
            ("win_model_T20.joblib", ("win", "T20")),
            ("innings_model.joblib", ("innings", None)),
            ("innings_model_ODI.joblib", ("innings", "ODI")),
            ("innings_model_T20I.joblib", ("innings", "T20I")),
            ("batting_share_model.joblib", ("batting_share", None)),
            ("batting_share_model_T20.joblib", ("batting_share", "T20")),
            ("bowling_share_model.joblib", ("bowling_share", None)),
            ("bowling_share_model_ODI.joblib", ("bowling_share", "ODI")),
        ],
    )
    def test_valid_filenames(self, fname: str, expected: Tuple[str, Optional[str]]) -> None:
        assert parse_model_filename(fname) == expected

    @pytest.mark.parametrize(
        "fname",
        [
            "batting_scaler_ODI.joblib",
            "tuning_report_batting_ODI.json",
            "README.md",
            "random_file.joblib",
            "batting_model.json",
            "",
        ],
    )
    def test_non_model_files_return_none(self, fname: str) -> None:
        assert parse_model_filename(fname) is None

    def test_case_insensitive(self) -> None:
        result = parse_model_filename("Batting_Model_ODI.joblib")
        assert result == ("batting", "ODI")


class TestGetModelArtifactStats:
    """get_model_artifact_stats returns correct size (including scaler) and metadata."""

    def test_model_with_scaler(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "batting_model_ODI.joblib").write_bytes(b"m" * 100)
        (p / "batting_scaler_ODI.joblib").write_bytes(b"s" * 50)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "batting", "ODI", "batting_model_ODI.joblib")
        assert rec is not None
        assert rec["model_kind"] == "batting"
        assert rec["model_name"] == "Batting"
        assert rec["match_format"] == "ODI"
        assert rec["size_bytes"] == 150

    def test_model_without_scaler(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "extras_model_T20.joblib").write_bytes(b"m" * 200)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "extras", "T20", "extras_model_T20.joblib")
        assert rec is not None
        assert rec["model_kind"] == "extras"
        assert rec["model_name"] == "Extras"
        assert rec["size_bytes"] == 200

    def test_innings_model_included(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "innings_model_ODI.joblib").write_bytes(b"m" * 80)
        (p / "innings_scaler_ODI.joblib").write_bytes(b"s" * 40)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "innings", "ODI", "innings_model_ODI.joblib")
        assert rec is not None
        assert rec["model_kind"] == "innings"
        assert rec["model_name"] == "Innings"
        assert rec["size_bytes"] == 120

    def test_batting_share_display_name(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "batting_share_model_T20.joblib").write_bytes(b"m" * 50)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(
            str(p), entries, "batting_share", "T20", "batting_share_model_T20.joblib"
        )
        assert rec is not None
        assert rec["model_kind"] == "batting_share"
        assert rec["model_name"] == "Batting Share"

    def test_unified_model_format(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "innings_model.joblib").write_bytes(b"m" * 60)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "innings", None, "innings_model.joblib")
        assert rec is not None
        assert rec["match_format"] == "Unified"


class TestBuildModelStats:
    """build_model_stats should discover all model kinds dynamically."""

    def test_discovers_all_model_kinds(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "batting_model_ODI.joblib").write_bytes(b"x" * 10)
        (p / "bowling_model_T20.joblib").write_bytes(b"x" * 10)
        (p / "innings_model_ODI.joblib").write_bytes(b"x" * 10)
        (p / "batting_share_model_T20.joblib").write_bytes(b"x" * 10)
        (p / "bowling_share_model_ODI.joblib").write_bytes(b"x" * 10)
        (p / "extras_model.joblib").write_bytes(b"x" * 10)
        (p / "win_model_T20.joblib").write_bytes(b"x" * 10)

        result = build_model_stats(str(p))
        names = {(m["model_kind"], m["match_format"]) for m in result["models"]}
        assert ("batting", "ODI") in names
        assert ("bowling", "T20") in names
        assert ("innings", "ODI") in names
        assert ("batting_share", "T20") in names
        assert ("bowling_share", "ODI") in names
        assert ("extras", "Unified") in names
        assert ("win", "T20") in names

    def test_ignores_non_model_files(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "batting_scaler_ODI.joblib").write_bytes(b"x" * 10)
        (p / "tuning_report_batting_ODI.json").write_text("{}")
        (p / "README.md").write_text("hello")

        result = build_model_stats(str(p))
        assert result["models"] == []

    def test_sorted_output(self, tmp_path: object) -> None:
        p = tmp_path  # type: ignore[assignment]
        (p / "win_model.joblib").write_bytes(b"x")
        (p / "batting_model.joblib").write_bytes(b"x")
        (p / "innings_model.joblib").write_bytes(b"x")

        result = build_model_stats(str(p))
        names = [m["model_name"] for m in result["models"]]
        assert names == sorted(names)
