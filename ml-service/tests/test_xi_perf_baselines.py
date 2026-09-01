"""Unit tests for the performance baselines over the unconditional player-match frame."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import perf_baselines as pb


def _frame(rows) -> pd.DataFrame:
    defaults = {
        "match_id": "m",
        "match_date": pd.Timestamp("2024-01-01"),
        "format_code": "T20",
        "player_key": "p1",
        "balls_faced": 0.0,
        "runs": 0.0,
        "balls_bowled": 0.0,
        "wickets": 0.0,
        "runs_conceded": 0.0,
        "catches": 0.0,
        "exp_balls_faced": 0.0,
        "bat_rate": 0.0,
        "exp_balls_bowled": 0.0,
        "bowl_wrate": 0.0,
        "bowl_rate": 0.0,
    }
    return pd.DataFrame([{**defaults, **row} for row in rows])


def _row(match: int, runs: float, player: str = "p1", fmt: str = "T20", balls: float = 10.0) -> dict:
    return {
        "match_id": f"m{match}",
        "match_date": pd.Timestamp("2024-01-01") + pd.Timedelta(days=match),
        "format_code": fmt,
        "player_key": player,
        "balls_faced": balls,
        "runs": runs,
    }


def test_career_mean_is_strictly_as_of() -> None:
    out = pb.add_baseline_predictors(_frame([_row(1, 10.0), _row(2, 20.0), _row(3, 30.0)]))

    assert np.isnan(out.career_mean_runs.iloc[0])
    assert out.career_mean_runs.iloc[1] == pytest.approx(10.0)
    assert out.career_mean_runs.iloc[2] == pytest.approx(15.0)
    assert list(out.prior_appearances) == [0, 1, 2]


def test_career_mean_is_unconditional_a_match_not_batted_in_counts_as_zero() -> None:
    """H-20: the baseline predicts the row's target on the same population the model is
    scored on, so an XI appearance without an innings is a 0, not a gap."""
    out = pb.add_baseline_predictors(_frame([_row(1, 10.0), _row(2, 0.0, balls=0.0), _row(3, 30.0)]))

    assert out.career_mean_runs.iloc[2] == pytest.approx(5.0)


def test_career_quantiles_are_the_as_of_empirical_quantiles() -> None:
    out = pb.add_baseline_predictors(_frame([_row(m, float(m * 10)) for m in range(1, 6)]))

    last = out.iloc[-1]
    assert last.career_q50_runs == pytest.approx(25.0)
    assert last.career_q10_runs == pytest.approx(13.0)
    assert last.career_q90_runs == pytest.approx(37.0)


def test_career_mean_does_not_cross_formats() -> None:
    out = pb.add_baseline_predictors(_frame([_row(1, 100.0, fmt="ODI"), _row(2, 10.0), _row(3, 20.0)]))

    t20 = out[out.format_code == "T20"]
    assert np.isnan(t20.career_mean_runs.iloc[0])
    assert t20.career_mean_runs.iloc[1] == pytest.approx(10.0)


def test_rating_expectation_is_a_pure_function_of_the_row() -> None:
    out = pb.add_baseline_predictors(
        _frame([{"exp_balls_faced": 20.0, "bat_rate": 0.25, "exp_balls_bowled": 24.0, "bowl_wrate": 0.01}])
    )

    assert out.rating_expect_runs.iloc[0] == pytest.approx(20.0 * (pb.LEAGUE_RUNS_PER_BALL + 0.25))
    assert out.rating_expect_wickets.iloc[0] == pytest.approx(24.0 * (pb.LEAGUE_WICKETS_PER_BALL + 0.01))
    assert out.rating_expect_balls_faced.iloc[0] == pytest.approx(20.0)


def test_baseline_predictions_fill_missing_history_from_the_training_window() -> None:
    frame = pb.add_baseline_predictors(_frame([_row(1, 10.0), _row(2, 30.0), _row(3, 0.0, player="new")]))
    train, evaluation = frame.iloc[:2], frame.iloc[2:]

    forecasts = pb.baseline_predictions(train, evaluation, "runs")

    assert forecasts["career_mean"]["point"][0] == pytest.approx(20.0)
    assert forecasts["career_quantiles"]["quantiles"][0][1] == pytest.approx(20.0)
    assert "rating_expectation" in forecasts
    assert "rating_expectation" not in pb.baseline_predictions(train, evaluation, "catches")
