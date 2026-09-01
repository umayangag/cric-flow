"""Quantile recalibration on a temporal fold (H-5).

When a quantile forecast's coverage is off nominal by more than the tolerance -- read per
end, because the targets have a point mass at zero (see ``perf_metrics``) -- each level is
re-mapped by an isotonic (monotone, binned) correction fitted on a fold the model did not
train on: rows are binned by the predicted quantile, the empirical quantile of the outcome
at that level is taken per bin, the bin values are made non-decreasing, and predictions are
mapped by interpolation. Fitted on a temporal fold strictly before what it is scored on,
so the correction is as out-of-sample as the model it corrects (H-21).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, List, Sequence

import numpy as np

from ml.xi.perf_metrics import QUANTILE_LEVELS

#: Coverage further than this from nominal triggers recalibration (H-5).
COVERAGE_TOLERANCE = 0.03
#: Equal-frequency bins per level; few enough that each holds hundreds of rows on a quarter.
N_BINS = 10
MIN_ROWS = 200


def coverage_off_nominal(interval: Dict) -> bool:
    """Whether an interval (``perf_metrics.interval_stats``) is miscalibrated at either
    end: a level must sit between its strict and inclusive exceedance, give or take the
    tolerance. For a continuous outcome the two readings coincide and this is the plain
    "coverage within +-0.03 of nominal"; a point mass at zero widens the band only where
    the data genuinely do."""
    for level, key in ((QUANTILE_LEVELS[0], "q10"), (QUANTILE_LEVELS[-1], "q90")):
        end = interval[key]
        if not (end["strict"] - COVERAGE_TOLERANCE <= level <= end["inclusive"] + COVERAGE_TOLERANCE):
            return True
    return False


@dataclass
class QuantileRecalibration:
    """Per-level monotone maps from predicted quantile to corrected quantile."""

    levels: List[float]
    knots_x: List[np.ndarray]  # per level: bin centres of the predicted quantile (non-decreasing)
    knots_y: List[np.ndarray]  # per level: corrected quantile at those knots (non-decreasing)
    n_fit: int

    @classmethod
    def fit(
        cls, predicted: np.ndarray, y: np.ndarray, levels: Sequence[float] = QUANTILE_LEVELS
    ) -> "QuantileRecalibration":
        if len(y) < MIN_ROWS:
            raise ValueError(f"recalibration needs at least {MIN_ROWS} rows, got {len(y)}")
        knots_x, knots_y = [], []
        for i, level in enumerate(levels):
            q = predicted[:, i]
            order = np.argsort(q, kind="stable")
            bins = np.array_split(order, N_BINS)
            x = np.asarray([q[b].mean() for b in bins if len(b)])
            corrected = np.asarray([np.quantile(y[b], level) for b in bins if len(b)])
            # Isotonic in the predicted quantile: a higher forecast never maps lower.
            knots_x.append(x)
            knots_y.append(np.maximum.accumulate(corrected))
        return cls(list(levels), knots_x, knots_y, int(len(y)))

    def apply(self, predicted: np.ndarray) -> np.ndarray:
        out = np.empty_like(predicted, dtype=float)
        for i in range(len(self.levels)):
            out[:, i] = np.interp(predicted[:, i], self.knots_x[i], self.knots_y[i])
        # Corrected levels are re-sorted so the interval cannot cross after the map.
        return np.sort(out, axis=1)
