"""Unit tests for the performance baselines over the player-match frame."""

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
        "exp_balls_faced": 0.0,
        "bat_rate": 0.0,
        "exp_balls_bowled": 0.0,
        "bowl_wrate": 0.0,
    }
    return pd.DataFrame([{**defaults, **row} for row in rows])


def _batted(match: int, runs: float, player: str = "p1", fmt: str = "T20") -> dict:
    return {
        "match_id": f"m{match}",
        "match_date": pd.Timestamp("2024-01-01") + pd.Timedelta(days=match),
        "format_code": fmt,
        "player_key": player,
        "balls_faced": 10.0,
        "runs": runs,
    }


def test_career_mean_is_strictly_as_of() -> None:
    frame = _frame([_batted(1, 10.0), _batted(2, 20.0), _batted(3, 30.0)])

    out = pb.add_baseline_predictors(frame)

    assert np.isnan(out.career_mean_runs.iloc[0])
    assert out.career_mean_runs.iloc[1] == pytest.approx(10.0)
    assert out.career_mean_runs.iloc[2] == pytest.approx(15.0)
    assert list(out.prior_batting_innings) == [0, 1, 2]


def test_career_mean_does_not_cross_formats() -> None:
    frame = _frame([_batted(1, 100.0, fmt="ODI"), _batted(2, 10.0), _batted(3, 20.0)])

    out = pb.add_baseline_predictors(frame)

    t20 = out[out.format_code == "T20"]
    assert np.isnan(t20.career_mean_runs.iloc[0])  # the ODI hundred is another career
    assert t20.career_mean_runs.iloc[1] == pytest.approx(10.0)


def test_career_mean_skips_matches_not_batted_in() -> None:
    frame = _frame(
        [
            _batted(1, 10.0),
            {**_batted(2, 0.0), "balls_faced": 0.0},  # in the XI, never batted
            _batted(3, 30.0),
        ]
    )

    out = pb.add_baseline_predictors(frame)

    assert out.career_mean_runs.iloc[2] == pytest.approx(10.0)
    assert out.prior_batting_innings.iloc[2] == 1


def test_rating_expectation_is_a_pure_function_of_the_row() -> None:
    frame = _frame([{"exp_balls_faced": 20.0, "bat_rate": 0.25, "exp_balls_bowled": 24.0, "bowl_wrate": 0.01}])

    out = pb.add_baseline_predictors(frame)

    assert out.rating_expect_runs.iloc[0] == pytest.approx(20.0 * (pb.LEAGUE_RUNS_PER_BALL + 0.25))
    assert out.rating_expect_wickets.iloc[0] == pytest.approx(24.0 * (pb.LEAGUE_WICKETS_PER_BALL + 0.01))


def test_score_window_conditions_on_involvement_and_history() -> None:
    rows = [_batted(m, 10.0 * m) for m in range(1, 6)]  # five batted innings
    rows.append({**_batted(6, 0.0), "balls_faced": 0.0})  # never batted: out of the check
    frame = pb.add_baseline_predictors(_frame(rows))
    window = frame[frame.match_date >= pd.Timestamp("2024-01-05")]

    report = pb.score_window(window)

    runs = report["runs"]["career_mean"]
    assert runs["n"] == 2  # m4 and m5: batted with >= 3 prior innings; m6 never batted
    assert runs["mae"] is not None
    assert report["runs"]["interval_width_80"] is None
    assert report["wickets"]["career_mean"]["n"] == 0
