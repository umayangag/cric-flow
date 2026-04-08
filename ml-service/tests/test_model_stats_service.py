"""Tests for model_stats_service: dynamic model-kind discovery."""

import os
from pathlib import Path
from typing import Optional, Tuple

import pytest

from app.model_stats_service import (
    _recompute_mlqa_audit_from_report,
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

    def test_model_with_scaler(self, tmp_path: Path) -> None:
        p = tmp_path
        (p / "batting_model_ODI.joblib").write_bytes(b"m" * 100)
        (p / "batting_scaler_ODI.joblib").write_bytes(b"s" * 50)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "batting", "ODI", "batting_model_ODI.joblib")
        assert rec is not None
        assert rec["model_kind"] == "batting"
        assert rec["model_name"] == "Batting"
        assert rec["match_format"] == "ODI"
        assert rec["size_bytes"] == 150

    def test_model_without_scaler(self, tmp_path: Path) -> None:
        p = tmp_path
        (p / "extras_model_T20.joblib").write_bytes(b"m" * 200)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "extras", "T20", "extras_model_T20.joblib")
        assert rec is not None
        assert rec["model_kind"] == "extras"
        assert rec["model_name"] == "Extras"
        assert rec["size_bytes"] == 200

    def test_innings_model_included(self, tmp_path: Path) -> None:
        p = tmp_path
        (p / "innings_model_ODI.joblib").write_bytes(b"m" * 80)
        (p / "innings_scaler_ODI.joblib").write_bytes(b"s" * 40)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "innings", "ODI", "innings_model_ODI.joblib")
        assert rec is not None
        assert rec["model_kind"] == "innings"
        assert rec["model_name"] == "Innings"
        assert rec["size_bytes"] == 120

    def test_batting_share_display_name(self, tmp_path: Path) -> None:
        p = tmp_path
        (p / "batting_share_model_T20.joblib").write_bytes(b"m" * 50)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "batting_share", "T20", "batting_share_model_T20.joblib")
        assert rec is not None
        assert rec["model_kind"] == "batting_share"
        assert rec["model_name"] == "Batting Share"

    def test_unified_model_format(self, tmp_path: Path) -> None:
        p = tmp_path
        (p / "innings_model.joblib").write_bytes(b"m" * 60)
        entries = os.listdir(str(p))
        rec = get_model_artifact_stats(str(p), entries, "innings", None, "innings_model.joblib")
        assert rec is not None
        assert rec["match_format"] == "Unified"


class TestBuildModelStats:
    """build_model_stats should discover all model kinds dynamically."""

    def test_discovers_all_model_kinds(self, tmp_path: Path) -> None:
        p = tmp_path
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

    def test_ignores_non_model_files(self, tmp_path: Path) -> None:
        p = tmp_path
        (p / "batting_scaler_ODI.joblib").write_bytes(b"x" * 10)
        (p / "tuning_report_batting_ODI.json").write_text("{}")
        (p / "README.md").write_text("hello")

        result = build_model_stats(str(p))
        assert result["models"] == []

    def test_sorted_output(self, tmp_path: Path) -> None:
        p = tmp_path
        (p / "win_model.joblib").write_bytes(b"x")
        (p / "batting_model.joblib").write_bytes(b"x")
        (p / "innings_model.joblib").write_bytes(b"x")

        result = build_model_stats(str(p))
        names = [m["model_name"] for m in result["models"]]
        assert names == sorted(names)


class TestRecomputeMLQAAuditRelativeThresholds:
    """_recompute_mlqa_audit_from_report uses relative thresholds (fraction of score magnitude)."""

    def test_large_magnitude_score_passes_with_small_relative_std(self) -> None:
        """CV fold std=0.08 on score=-4.80 is 1.7% relative, well under 8% threshold → PASS."""
        report = {
            "best_cv_score": -4.80,
            "mlqa_audit": {
                "audit_status": "FAIL",
                "key_findings": ["old finding"],
                "bias_report": "No protected groups defined; fairness audit skipped.",
                "final_verdict": "Rollback & Re-tune",
                "checks": {
                    "overfitting": {"delta": 0.05, "flagged": True},
                    "stability": {"cv_std": 0.0845, "flagged": True, "cv_fold_scores": [-4.70, -4.79, -4.63, -4.80, -4.58]},
                },
            },
        }
        result = _recompute_mlqa_audit_from_report(report)
        assert result is not None
        assert result["audit_status"] == "PASS"
        assert result["final_verdict"] == "Proceed to Deployment"
        # Checks should include relative metrics
        assert "relative_delta" in result["checks"]["overfitting"]
        assert "relative_cv_std" in result["checks"]["stability"]
        assert "threshold" in result["checks"]["stability"]
        # Relative std: 0.0845 / 4.80 ≈ 1.76% (under 8%)
        rel_std = result["checks"]["stability"]["relative_cv_std"]
        assert rel_std < 0.08

    def test_high_relative_delta_fails(self) -> None:
        """Delta=1.0 on score=-2.0 is 50% relative, way over 10% threshold → FAIL."""
        report = {
            "best_cv_score": -2.0,
            "mlqa_audit": {
                "audit_status": "PASS",
                "key_findings": [],
                "bias_report": "",
                "final_verdict": "Proceed to Deployment",
                "checks": {
                    "overfitting": {"delta": 1.0, "flagged": False},
                    "stability": {"cv_std": 0.01, "flagged": False},
                },
            },
        }
        result = _recompute_mlqa_audit_from_report(report)
        assert result is not None
        assert result["audit_status"] == "FAIL"
        assert result["checks"]["overfitting"]["flagged"] is True
        # Relative delta: 1.0 / 2.0 = 50%
        assert result["checks"]["overfitting"]["relative_delta"] == pytest.approx(0.5, abs=0.01)

    def test_missing_best_cv_score_falls_back_gracefully(self) -> None:
        """When best_cv_score is missing, score_magnitude=0 → any non-zero metric is inf → FAIL."""
        report = {
            "mlqa_audit": {
                "audit_status": "PASS",
                "key_findings": [],
                "bias_report": "",
                "final_verdict": "Proceed to Deployment",
                "checks": {
                    "overfitting": {"delta": 0.01, "flagged": False},
                    "stability": {"cv_std": 0.01, "flagged": False},
                },
            },
        }
        result = _recompute_mlqa_audit_from_report(report)
        assert result is not None
        # With no reference score, relative metrics are inf → both flagged
        assert result["audit_status"] == "FAIL"
