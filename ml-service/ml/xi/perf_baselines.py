"""Performance baselines over the player-match frame: what the performance model (L2-B)
must beat, on the same population it is scored on.

Every predictor here is as-of by construction and defined on the *unconditional*
population -- every XI player, "did not bat" as 0 (H-20). Three of them:

* ``career_mean_<target>``      -- the player's expanding mean of the target over their
                                   previous rows in the format, shifted so a row never sees
                                   itself. The acceptance baseline (plan P-3).
* ``career_q{10,50,90}_<target>`` -- the same history's empirical quantiles: the
                                   distributional baseline H-22's width-at-coverage is read
                                   against.
* ``rating_expect_<target>``    -- a pure function of the row's as-of vectors (expected balls
                                   times a league rate plus the impact rating).

A player with no history in the format has NaN career predictors; the harness fills them
with the training window's figure, which is what a model that knew nothing would say.

Every career statistic is read *at day close*, the rule the rating pass folds matches by
(``builder.build``): a player's second match of a day is predicted from the history his
first match of that day saw. Without it the baseline read a same-day earlier match the
model's features could not, and the model was being asked to beat a predictor with more
information than it had (EVAL-16).
"""

from __future__ import annotations

from typing import Dict, List, Sequence

import numpy as np
import pandas as pd

from ml.xi.perf_metrics import QUANTILE_LEVELS
from ml.xi.performance import TARGETS

# League-average rates used by the rating-expectation predictor (the experiment's values).
LEAGUE_RUNS_PER_BALL = 1.25
LEAGUE_WICKETS_PER_BALL = 0.05

BASELINE_TARGETS = tuple(t.name for t in TARGETS)
CAREER_QUANTILE_COLS = {level: f"career_q{int(round(level * 100))}" for level in QUANTILE_LEVELS}


def _first_row_of_each_day(group_keys: Sequence[np.ndarray]) -> np.ndarray:
    """For each row of a date-ordered frame, the position of the first row sharing its
    ``group_keys`` -- (player, format, date) here.

    Positions, not values, so the caller can re-read any column at them; and numpy keys,
    so nothing depends on the frame's index being unique.
    """
    positions = pd.Series(np.arange(len(group_keys[0])))
    return positions.groupby(list(group_keys), sort=False).transform("min").to_numpy()


def _at_day_close(column: pd.Series, first_row_of_day: np.ndarray) -> pd.Series:
    """``column`` re-read at the first row of each row's day.

    A statistic that is strictly as-of its own row becomes strictly as-of its row's *date*:
    every row of a date carries what the day's first match carried, which is the history
    the rating pass gives a feature on that date (EVAL-16).
    """
    return pd.Series(column.to_numpy()[first_row_of_day], index=column.index)


def add_baseline_predictors(player_frame: pd.DataFrame) -> pd.DataFrame:
    """The frame, date-ordered, with the baseline predictor columns for every target."""
    out = player_frame.sort_values(["match_date", "match_id"], kind="stable").copy()
    # Per (player, format): a T20 career should not predict an ODI innings. The shift keeps
    # each statistic strictly as-of its own row, and the day close then keeps it as-of the
    # row's date, so the baseline and the model read the same history (EVAL-16). On the
    # archive it touches 5,927 of 469,743 rows -- 2.1% of T20, 0.6% of T20I, none of ODI or
    # TEST, since only a T20 player plays twice in a day -- and moves ``career_mean_runs``
    # on them by 1.3 runs on average.
    by_player: List[pd.Series] = [out.player_key, out.format_code]
    first_row_of_day = _first_row_of_each_day(
        [out.player_key.to_numpy(), out.format_code.to_numpy(), out.match_date.to_numpy()]
    )
    out["prior_appearances"] = _at_day_close(out.groupby(by_player).cumcount(), first_row_of_day)
    for target in BASELINE_TARGETS:
        grouped = out[target].groupby(by_player)
        career_mean = grouped.transform(lambda s: s.shift(1).expanding().mean())
        out[f"career_mean_{target}"] = _at_day_close(career_mean, first_row_of_day)
        for level, stem in CAREER_QUANTILE_COLS.items():
            quantile = grouped.transform(lambda s, q=level: s.shift(1).expanding().quantile(q))
            out[f"{stem}_{target}"] = _at_day_close(quantile, first_row_of_day)
    out["rating_expect_runs"] = np.maximum(out.exp_balls_faced * (LEAGUE_RUNS_PER_BALL + out.bat_rate), 0.0)
    out["rating_expect_balls_faced"] = out.exp_balls_faced
    out["rating_expect_wickets"] = np.maximum(out.exp_balls_bowled * (LEAGUE_WICKETS_PER_BALL + out.bowl_wrate), 0.0)
    out["rating_expect_runs_conceded"] = np.maximum(out.exp_balls_bowled * (LEAGUE_RUNS_PER_BALL - out.bowl_rate), 0.0)
    return out


def baseline_predictions(train_rows: pd.DataFrame, eval_rows: pd.DataFrame, target: str) -> Dict[str, Dict]:
    """The baseline forecasts for one target on ``eval_rows``: a point for every predictor
    and, for the career quantiles, an interval. NaNs (no history) take the training
    window's mean / quantiles."""
    y_train = train_rows[target].to_numpy(dtype=float)
    fill_mean = float(y_train.mean()) if len(y_train) else 0.0
    fill_quantiles = np.quantile(y_train, QUANTILE_LEVELS) if len(y_train) else np.zeros(len(QUANTILE_LEVELS))
    career_mean = eval_rows[f"career_mean_{target}"].fillna(fill_mean).to_numpy(dtype=float)
    career_quantiles = np.column_stack(
        [
            eval_rows[f"{stem}_{target}"].fillna(fill_quantiles[i]).to_numpy(dtype=float)
            for i, stem in enumerate(CAREER_QUANTILE_COLS.values())
        ]
    )
    out: Dict[str, Dict] = {
        "career_mean": {"point": career_mean},
        "career_quantiles": {"point": career_quantiles[:, 1], "quantiles": career_quantiles},
    }
    rating_col = f"rating_expect_{target}"
    if rating_col in eval_rows:
        out["rating_expectation"] = {"point": eval_rows[rating_col].to_numpy(dtype=float)}
    return out
