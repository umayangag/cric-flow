"""Unit tests for the performance metrics and the quantile recalibration (H-5, H-22)."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import perf_metrics as M
from ml.xi.perf_calibration import QuantileRecalibration, coverage_off_nominal


def test_pinball_is_the_asymmetric_absolute_error() -> None:
    y, q = np.array([0.0, 10.0]), np.array([5.0, 5.0])

    assert M.pinball(y, q, 0.5) == pytest.approx(2.5)
    assert M.pinball(y, q, 0.9) == pytest.approx((0.5 + 4.5) / 2)
    assert M.mean_pinball(y, np.column_stack([q, q, q])) == pytest.approx((2.5 + 2.5 + 2.5) / 3)


def test_interval_stats_report_both_readings_of_each_end() -> None:
    y = np.array([0.0, 0.0, 5.0, 12.0])
    low, high = np.zeros(4), np.full(4, 10.0)

    stats = M.interval_stats(y, low, high)

    assert stats["coverage_80"] == pytest.approx(0.75)
    assert stats["coverage_80_strict"] == pytest.approx(0.25)
    assert stats["width_80"] == pytest.approx(10.0)
    assert stats["q10"] == {"strict": 0.0, "inclusive": 0.5}
    assert stats["q90"] == {"strict": 0.75, "inclusive": 0.75}


def test_within_match_spearman_averages_usable_matches_only() -> None:
    match_ids = pd.Series(["m1"] * 4 + ["m2"] * 4 + ["m3"] * 3 + ["m4"] * 4)
    predicted = np.array([1, 2, 3, 4, 1, 2, 3, 4, 1, 2, 3, 5, 5, 5, 5], dtype=float)
    actual = np.array([1, 2, 3, 4, 4, 3, 2, 1, 1, 2, 3, 1, 2, 3, 4], dtype=float)

    rho = M.within_match_spearman(match_ids, predicted, actual)

    assert rho == pytest.approx(0.0)  # m1 = +1, m2 = -1; m3 too small, m4 constant prediction


def test_within_match_spearman_is_none_without_a_usable_match() -> None:
    assert M.within_match_spearman(pd.Series(["m"] * 3), np.arange(3.0), np.arange(3.0)) is None


def test_top3_hit_rate_counts_shared_top_three_per_match() -> None:
    match_ids = pd.Series(["m1"] * 6 + ["m2"] * 6)
    keys = pd.Series([f"p{i}" for i in range(12)])
    predicted = np.array([6, 5, 4, 3, 2, 1, 6, 5, 4, 3, 2, 1], dtype=float)
    actual = np.array([6, 5, 4, 3, 2, 1, 1, 2, 3, 4, 5, 6], dtype=float)

    assert M.top3_hit_rate(match_ids, keys, predicted, actual) == pytest.approx(0.5)


def test_score_quantiles_reports_median_error_pinball_and_interval() -> None:
    rows = pd.DataFrame({"match_id": ["m"] * 4, "player_key": list("abcd"), "runs": [0.0, 10.0, 20.0, 30.0]})
    quantiles = np.column_stack([np.zeros(4), np.array([5.0, 10.0, 20.0, 25.0]), np.full(4, 40.0)])

    out = M.score_quantiles(rows, quantiles, "runs")

    assert out["mae"] == pytest.approx(2.5)
    assert out["pinball"] > 0
    assert set(out["pinball_by_level"]) == {"0.1", "0.5", "0.9"}
    assert out["interval"]["coverage_80"] == pytest.approx(1.0)


def test_count_probabilities_reliability_and_brier() -> None:
    rows = pd.DataFrame({"match_id": ["m"] * 4, "player_key": list("abcd"), "wickets": [0.0, 1.0, 2.0, 0.0]})
    p0, p1 = np.array([0.5, 0.5, 0.5, 0.5]), np.array([0.3, 0.3, 0.3, 0.3])

    out = M.score_count_probabilities(rows, p0, p1, "wickets")

    assert out["reliability"]["0"] == {"predicted": 0.5, "observed": 0.5}
    assert out["reliability"]["2+"]["observed"] == pytest.approx(0.25)
    assert 0.0 < out["brier"] < 1.0


def test_coverage_off_nominal_reads_each_end_against_its_level() -> None:
    calibrated = {"q10": {"strict": 0.0, "inclusive": 0.4}, "q90": {"strict": 0.88, "inclusive": 0.9}}
    upper_too_low = {"q10": {"strict": 0.05, "inclusive": 0.12}, "q90": {"strict": 0.8, "inclusive": 0.82}}
    lower_too_high = {"q10": {"strict": 0.2, "inclusive": 0.25}, "q90": {"strict": 0.9, "inclusive": 0.9}}

    assert not coverage_off_nominal(calibrated)
    assert coverage_off_nominal(upper_too_low)
    assert coverage_off_nominal(lower_too_high)


def test_recalibration_moves_coverage_toward_nominal_and_stays_monotone() -> None:
    rng = np.random.RandomState(0)
    scale = rng.uniform(5.0, 30.0, size=2000)
    y = rng.exponential(scale)
    # A forecast that is too narrow: the true 0.9 quantile of Exp(scale) is 2.3 * scale.
    predicted = np.column_stack([0.02 * scale, 0.6 * scale, 1.2 * scale])

    calibration = QuantileRecalibration.fit(predicted, y)
    corrected = calibration.apply(predicted)

    before = np.mean(y <= predicted[:, 2])
    after = np.mean(y <= corrected[:, 2])
    assert before < 0.8
    assert after == pytest.approx(0.9, abs=0.03)
    for knots in calibration.knots_y:
        assert np.all(np.diff(knots) >= 0)
    assert np.all(np.diff(corrected, axis=1) >= 0)


def test_recalibration_refuses_a_fold_too_small_to_bin() -> None:
    with pytest.raises(ValueError, match="rows"):
        QuantileRecalibration.fit(np.zeros((10, 3)), np.zeros(10))
