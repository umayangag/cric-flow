"""Unit tests for L4's performance section (ml.xi.perf_harness)."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import gates
from ml.xi import perf_harness as H
from ml.xi import performance as P
from tests.xi_perf_fixtures import fast_fits, synthetic_player_frame


@pytest.fixture(scope="module")
def frame() -> pd.DataFrame:
    return synthetic_player_frame()


@pytest.fixture(scope="module")
def window(frame) -> H.WindowResult:
    with fast_fits():
        return H.evaluate_fold(frame, "T20", pd.Timestamp("2023-04-15"), pd.Timestamp("2023-06-01"))


def test_window_reports_model_and_baselines_per_target(window) -> None:
    targets = window.report["targets"]

    assert set(targets) == {t.name for t in P.TARGETS}
    runs = targets["runs"]
    for name in H.MODEL_NAMES + H.BASELINE_NAMES:
        assert runs[name]["n"] == window.report["n_eval"]
    assert runs["model"]["interval"]["width_80"] >= 0.0
    assert runs["career_mean"]["interval"] is None and runs["career_mean"]["pinball"] is not None
    assert runs["career_quantiles"]["interval"]["coverage_80"] <= 1.0
    assert "spearman" in runs["vs_career_mean"] and "pinball" in runs["vs_career_mean"]
    assert targets["wickets"]["model"]["probabilities"]["reliability"]["0"]["observed"] <= 1.0
    assert -1.0 <= targets["wickets"]["model"]["within_match_spearman_involved"] <= 1.0
    assert targets["catches"]["model"]["within_match_spearman_involved"] is None
    assert targets["catches"]["headline"] is False


def test_window_skips_with_a_reason_when_data_is_short(frame) -> None:
    early = H.evaluate_fold(frame, "T20", pd.Timestamp("2023-01-10"), pd.Timestamp("2023-02-01"))
    empty = H.evaluate_fold(frame, "T20", pd.Timestamp("2023-04-15"), pd.Timestamp("2023-04-16"))

    assert early.model is None and early.report["skipped_reason"] == "insufficient training rows"
    assert empty.model is None and empty.report["skipped_reason"] == "evaluation window too small"


def test_training_rows_follow_the_e6_decision(frame, monkeypatch) -> None:
    t20i = frame.head(22).copy()
    t20i["format_code"] = "T20I"
    mixed = pd.concat([frame, t20i])

    separate, joint_flag = H.training_rows(mixed, "T20", pd.Timestamp("2024-01-01"))
    monkeypatch.setattr(C, "E6_JOINT_T20_FORMATS", True)
    joint, joint_flag_on = H.training_rows(mixed, "T20", pd.Timestamp("2024-01-01"))

    assert not joint_flag and set(separate.format_code) == {"T20"}
    assert joint_flag_on and set(joint.format_code) == {"T20", "T20I"}


def test_summarize_folds_keeps_nesting_and_drops_missing_leaves() -> None:
    folds = [
        {"targets": {"runs": {"model": {"pinball": 1.0, "interval": {"width_80": 3.0}}, "headline": True}}},
        {"targets": {"runs": {"model": {"pinball": 3.0, "interval": {"width_80": 5.0}}, "headline": True}}},
        None,
    ]

    summary = H.summarize_folds(folds)

    assert summary["targets"]["runs"]["model"]["pinball"] == {
        "mean": 2.0,
        "sd": 1.0,
        "n_folds": 2,
        "gates_consulted": gates.folds_consulted_count(),
    }
    assert summary["targets"]["runs"]["model"]["interval"]["width_80"]["mean"] == 4.0
    assert summary["targets"]["runs"]["headline"] is True


def test_recalibration_needed_names_miscalibrated_quantile_targets_only() -> None:
    def interval(q90_strict: float):
        return {
            "q10": {"strict": {"mean": 0.0}, "inclusive": {"mean": 0.4}},
            "q90": {"strict": {"mean": q90_strict}, "inclusive": {"mean": q90_strict + 0.01}},
        }

    summary = {
        "targets": {
            "runs": {"model": {"interval": interval(0.95)}},
            "balls_faced": {"model": {"interval": interval(0.89)}},
            "wickets": {"model": {"interval": interval(0.99)}},
        }
    }

    assert H.recalibration_needed(summary) == ["runs"]
    assert H.recalibration_needed(None) == []


def test_score_forecast_point_baseline_has_pinball_but_no_interval() -> None:
    rows = pd.DataFrame({"match_id": ["m"] * 4, "player_key": list("abcd"), "runs": [0.0, 10.0, 20.0, 30.0]})

    out = H.score_forecast(P.TARGET_BY_NAME["runs"], {"point": np.full(4, 15.0)}, rows)

    assert out["interval"] is None
    assert out["pinball"] == pytest.approx(np.mean([15, 5, 5, 15]) * (0.1 + 0.5 + 0.9) / 3, rel=0.4)
    assert out["mae"] == pytest.approx(10.0)
