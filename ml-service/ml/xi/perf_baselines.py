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
"""

from __future__ import annotations

from typing import Dict

import numpy as np
import pandas as pd

from ml.xi.perf_metrics import QUANTILE_LEVELS
from ml.xi.performance import TARGETS

# League-average rates used by the rating-expectation predictor (the experiment's values).
LEAGUE_RUNS_PER_BALL = 1.25
LEAGUE_WICKETS_PER_BALL = 0.05

BASELINE_TARGETS = tuple(t.name for t in TARGETS)
CAREER_QUANTILE_COLS = {level: f"career_q{int(round(level * 100))}" for level in QUANTILE_LEVELS}


def add_baseline_predictors(player_frame: pd.DataFrame) -> pd.DataFrame:
    """The frame, date-ordered, with the baseline predictor columns for every target."""
    out = player_frame.sort_values(["match_date", "match_id"], kind="stable").copy()
    # Per (player, format): a T20 career should not predict an ODI innings. The shift keeps
    # the statistic strictly as-of its row; like the reference experiment these do not
    # re-apply day-close batching, because they are baseline predictors, not features.
    by_player = [out.player_key, out.format_code]
    out["prior_appearances"] = out.groupby(by_player).cumcount()
    for target in BASELINE_TARGETS:
        grouped = out[target].groupby(by_player)
        out[f"career_mean_{target}"] = grouped.transform(lambda s: s.shift(1).expanding().mean())
        for level, stem in CAREER_QUANTILE_COLS.items():
            out[f"{stem}_{target}"] = grouped.transform(lambda s, q=level: s.shift(1).expanding().quantile(q))
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
