"""Metrics for the performance model and its baselines, per target and per population,
never pooled (H-12).

Ranking is what a selector consumes (within-match Spearman, top-3 hit); the proper score of
a distribution is the pinball loss at its quantile levels; and an interval is judged by its
coverage *and* its width together (H-22): narrower at nominal coverage is progress, narrower
below it is a regression.

Coverage is reported twice because the targets are counts with a point mass at zero. A
calibrated 0.1 quantile of a batter who bats in half his matches is 0, and "0 <= y" then
holds for every row -- so the inclusive coverage of a calibrated interval legitimately
exceeds nominal, while the strict coverage (both ends exclusive) falls short of it. The
nominal 0.80 should sit between the two; H-5 reads both.
"""

from __future__ import annotations

from typing import Dict, Optional, Sequence

import numpy as np
import pandas as pd

QUANTILE_LEVELS = (0.1, 0.5, 0.9)
# Matches with fewer players than this carry no ranking signal.
MIN_PLAYERS_FOR_RANKING = 4
MIN_PLAYERS_FOR_TOP3 = 6


def pinball(y: np.ndarray, q: np.ndarray, level: float) -> float:
    """Mean pinball loss of one quantile forecast at ``level``."""
    diff = y - q
    return float(np.mean(np.maximum(level * diff, (level - 1.0) * diff)))


def mean_pinball(y: np.ndarray, quantiles: np.ndarray, levels: Sequence[float] = QUANTILE_LEVELS) -> float:
    """Mean over levels of the pinball loss; the single proper score of a quantile set."""
    return float(np.mean([pinball(y, quantiles[:, i], level) for i, level in enumerate(levels)]))


def interval_stats(y: np.ndarray, low: np.ndarray, high: np.ndarray) -> Dict:
    """Coverage and width of the 10-90 interval (H-22), plus each end's exceedance twice:
    ``strict`` is P(y < q), ``inclusive`` is P(y <= q). A calibrated quantile at level tau
    has strict <= tau <= inclusive, with equality only where the outcome is continuous."""
    return {
        "coverage_80": float(np.mean((y >= low) & (y <= high))),
        "coverage_80_strict": float(np.mean((y > low) & (y < high))),
        "width_80": float(np.mean(high - low)),
        "q10": {"strict": float(np.mean(y < low)), "inclusive": float(np.mean(y <= low))},
        "q90": {"strict": float(np.mean(y < high)), "inclusive": float(np.mean(y <= high))},
    }


def within_match_spearman(match_ids: pd.Series, predicted: np.ndarray, actual: np.ndarray) -> Optional[float]:
    """Mean per-match Spearman between predicted and actual over matches with at least
    ``MIN_PLAYERS_FOR_RANKING`` players and variance in both columns."""
    frame = pd.DataFrame({"m": match_ids.to_numpy(), "p": predicted, "a": actual})
    groups = frame.groupby("m", sort=False)
    ranks = pd.DataFrame({"m": frame.m, "rp": groups.p.rank(), "ra": groups.a.rank()})
    centred = ranks[["rp", "ra"]] - ranks.groupby("m", sort=False)[["rp", "ra"]].transform("mean")
    per_match = pd.DataFrame(
        {
            "n": ranks.groupby("m", sort=False).size(),
            "cross": (centred.rp * centred.ra).groupby(ranks.m, sort=False).sum(),
            "vp": (centred.rp**2).groupby(ranks.m, sort=False).sum(),
            "va": (centred.ra**2).groupby(ranks.m, sort=False).sum(),
        }
    )
    usable = per_match[(per_match.n >= MIN_PLAYERS_FOR_RANKING) & (per_match.vp > 0) & (per_match.va > 0)]
    if usable.empty:
        return None
    return float((usable.cross / np.sqrt(usable.vp * usable.va)).mean())


def top3_hit_rate(
    match_ids: pd.Series, player_keys: pd.Series, predicted: np.ndarray, actual: np.ndarray
) -> Optional[float]:
    """Mean share of the actual top-3 performers found in the predicted top-3, over matches
    with at least ``MIN_PLAYERS_FOR_TOP3`` players. Ties go to the earlier row, as before."""
    frame = pd.DataFrame({"m": match_ids.to_numpy(), "p": predicted, "a": actual})
    groups = frame.groupby("m", sort=False)
    top_predicted = groups.p.rank(ascending=False, method="first") <= 3
    top_actual = groups.a.rank(ascending=False, method="first") <= 3
    sizes = groups.m.transform("size")
    hits = (top_predicted & top_actual & (sizes >= MIN_PLAYERS_FOR_TOP3)).groupby(frame.m, sort=False).sum()
    usable = hits[groups.size() >= MIN_PLAYERS_FOR_TOP3]
    if usable.empty:
        return None
    return float((usable / 3.0).mean())


def score_point(rows: pd.DataFrame, point: np.ndarray, actual: str) -> Dict:
    """Ranking and point metrics of one point predictor on one population."""
    y = rows[actual].to_numpy(dtype=float)
    return {
        "n": int(len(rows)),
        "mae": float(np.abs(y - point).mean()) if len(rows) else None,
        "within_match_spearman": within_match_spearman(rows.match_id, point, y) if len(rows) else None,
        "top3_hit_rate": top3_hit_rate(rows.match_id, rows.player_key, point, y) if len(rows) else None,
    }


def score_quantiles(rows: pd.DataFrame, quantiles: np.ndarray, actual: str) -> Dict:
    """Point metrics of the median plus the proper score and interval statistics of the
    (q10, q50, q90) forecast."""
    y = rows[actual].to_numpy(dtype=float)
    out = score_point(rows, quantiles[:, 1], actual)
    if not len(rows):
        out.update({"pinball": None, "pinball_by_level": None, "interval": None})
        return out
    out["pinball"] = mean_pinball(y, quantiles)
    out["pinball_by_level"] = {
        str(level): pinball(y, quantiles[:, i], level) for i, level in enumerate(QUANTILE_LEVELS)
    }
    out["interval"] = interval_stats(y, quantiles[:, 0], quantiles[:, 2])
    return out


def score_count_probabilities(rows: pd.DataFrame, p0: np.ndarray, p1: np.ndarray, actual: str) -> Dict:
    """Reliability of the wicket-count probabilities: mean predicted against observed
    frequency for P(0), P(1), P(2+), and the Brier score of the three-way forecast."""
    y = rows[actual].to_numpy(dtype=float)
    p2 = 1.0 - p0 - p1
    observed = {"0": y == 0, "1": y == 1, "2+": y >= 2}
    predicted = {"0": p0, "1": p1, "2+": p2}
    brier = float(np.mean(sum((predicted[k] - observed[k].astype(float)) ** 2 for k in observed)))
    return {
        "brier": brier,
        "reliability": {
            k: {"predicted": float(predicted[k].mean()), "observed": float(observed[k].mean())} for k in observed
        },
    }
