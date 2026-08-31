"""Performance baselines over the player-match frame (L4's secondary-goal metrics).

The predictors here are the ones any performance model must beat (P-3): the player's own
as-of career mean, and the rating pass's expectation. Metrics are the consumer's --
within-match Spearman and top-3 hit for ranking, MAE for the point -- reported per target
and per format, never pooled (H-12/H-13). Interval width and coverage columns exist from
the start and stay empty until P-3 ships a distributional model (H-22).

The frame itself is unconditional (every XI player has a row, H-20); the *baseline check*
conditions on who batted or bowled, because that is what ``perf_experiment.py`` measured
and what its reference numbers (career-mean Spearman ~0.32 T20 / ~0.34 ODI) mean.
"""

from __future__ import annotations

from typing import Dict, List, Optional

import numpy as np
import pandas as pd
from scipy.stats import spearmanr

# The experiment's league-average runs per ball, used by the rating-expectation predictor.
LEAGUE_RUNS_PER_BALL = 1.25
# The experiment's wickets-per-ball equivalent for bowling expectation.
LEAGUE_WICKETS_PER_BALL = 0.05
# Players with fewer prior innings than this are excluded from the check, matching the
# experiment (a career mean over one innings is not a predictor).
MIN_PRIOR_INNINGS = 3

# The two targets the baselines cover: runs for batters, wickets for bowlers.
TARGETS = ("runs", "wickets")


def add_baseline_predictors(player_frame: pd.DataFrame) -> pd.DataFrame:
    """The frame with as-of baseline predictor columns added.

    ``career_mean_*`` are expanding means over the player's *previous* batted / bowled
    innings (shifted, so a row never sees itself); ``rating_expect_*`` are pure functions
    of the row's as-of vectors.
    """
    out = player_frame.sort_values(["match_date", "match_id"], kind="stable").copy()
    batted = out.balls_faced > 0
    bowled = out.balls_bowled > 0
    # Per (player, format): a T20 career mean should not predict an ODI innings. The shift
    # keeps the mean strictly as-of its row; like the reference experiment, it does not
    # re-apply day-close batching, because these are baseline predictors, not features.
    by_player = [out.player_key, out.format_code]

    runs_when_batted = out.runs.where(batted)
    out["career_mean_runs"] = runs_when_batted.groupby(by_player).transform(lambda s: s.shift(1).expanding().mean())
    out["prior_batting_innings"] = batted.groupby(by_player).cumsum() - batted.astype(int)

    wickets_when_bowled = out.wickets.where(bowled)
    out["career_mean_wickets"] = wickets_when_bowled.groupby(by_player).transform(
        lambda s: s.shift(1).expanding().mean()
    )
    out["prior_bowling_innings"] = bowled.groupby(by_player).cumsum() - bowled.astype(int)

    out["rating_expect_runs"] = np.maximum(out.exp_balls_faced * (LEAGUE_RUNS_PER_BALL + out.bat_rate), 0.0)
    out["rating_expect_wickets"] = np.maximum(out.exp_balls_bowled * (LEAGUE_WICKETS_PER_BALL + out.bowl_wrate), 0.0)
    return out


def _within_match_spearman(rows: pd.DataFrame, predicted: str, actual: str) -> Optional[float]:
    """Mean per-match Spearman between predicted and actual, over matches with at least
    four players and variance in both columns."""
    correlations: List[float] = []
    for _, group in rows.groupby("match_id", sort=False):
        if len(group) < 4 or group[actual].std() == 0 or group[predicted].std() == 0:
            continue
        rho = spearmanr(group[predicted], group[actual]).correlation
        if not np.isnan(rho):
            correlations.append(float(rho))
    return float(np.mean(correlations)) if correlations else None


def _top3_hit_rate(rows: pd.DataFrame, predicted: str, actual: str) -> Optional[float]:
    """Mean share of the actual top-3 performers found in the predicted top-3, over
    matches with at least six players."""
    hits: List[float] = []
    for _, group in rows.groupby("match_id", sort=False):
        if len(group) < 6:
            continue
        top_predicted = set(group.nlargest(3, predicted).player_key)
        top_actual = set(group.nlargest(3, actual).player_key)
        hits.append(len(top_predicted & top_actual) / 3.0)
    return float(np.mean(hits)) if hits else None


def _score_predictor(rows: pd.DataFrame, predicted: str, actual: str) -> Dict:
    return {
        "n": int(len(rows)),
        "mae": float(np.abs(rows[actual] - rows[predicted]).mean()) if len(rows) else None,
        "within_match_spearman": _within_match_spearman(rows, predicted, actual),
        "top3_hit_rate": _top3_hit_rate(rows, predicted, actual),
    }


def _population(rows: pd.DataFrame, target: str, fill: float) -> pd.DataFrame:
    """The conditioned population the baselines are defined on, with predictor NaNs filled
    by the population's overall mean stand-in (the experiment used the training mean)."""
    if target == "runs":
        population = rows[(rows.balls_faced > 0) & (rows.prior_batting_innings >= MIN_PRIOR_INNINGS)].copy()
        population["career_mean_runs"] = population.career_mean_runs.fillna(fill)
    else:
        population = rows[(rows.balls_bowled > 0) & (rows.prior_bowling_innings >= MIN_PRIOR_INNINGS)].copy()
        population["career_mean_wickets"] = population.career_mean_wickets.fillna(fill)
    return population


def score_window(rows: pd.DataFrame) -> Dict:
    """Both targets' baseline metrics over one evaluation window's player rows."""
    report: Dict = {}
    for target in TARGETS:
        fill = float(rows[target].mean()) if len(rows) else 0.0
        population = _population(rows, target, fill)
        report[target] = {
            "career_mean": _score_predictor(population, f"career_mean_{target}", target),
            "rating_expectation": _score_predictor(population, f"rating_expect_{target}", target),
            # H-22: interval sharpness columns exist from the start; P-3's quantile model fills them.
            "interval_width_80": None,
            "interval_coverage_80": None,
        }
    return report
