"""Quantile recalibration on a temporal fold (H-5).

When a quantile forecast's coverage is off nominal by more than the tolerance -- read per
end, because the targets have a point mass at zero (see ``perf_metrics``) -- each level is
re-mapped by an isotonic correction fitted on a fold the model did not train on: rows are
binned by the predicted quantile, the empirical quantile of the outcome at that level is
taken per bin, and an isotonic regression through the bins gives a non-decreasing map from
predicted to corrected quantile. Fitted on a temporal fold strictly before what it is
scored on, so the correction is as out-of-sample as the model it corrects (H-21).

Two properties of the binning matter, because the targets are zero-inflated and a
predicted quantile is therefore often exactly 0 for a large block of rows:

* **A bin never holds part of a tie.** Rows sharing a predicted value are one point to any
  map from predicted to corrected, so splitting them across bins would estimate the same
  point several times from disjoint fractions of the evidence and then have to reconcile
  the disagreement. Bins are grown out of whole tie groups instead, and the estimate at a
  repeated prediction is the pooled one.
* **The monotone repair is a least-squares isotonic fit** (``IsotonicRegression``), not a
  running maximum. A running maximum resolves every violation upward, which biases the
  correction in the direction of the noisiest bin; pool-adjacent-violators averages them.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, List, Sequence

import numpy as np
from sklearn.isotonic import IsotonicRegression

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


def _tie_safe_bins(predicted_level: np.ndarray) -> List[np.ndarray]:
    """Row indices grouped into at most ``N_BINS`` bins of roughly equal size, ordered by
    the predicted quantile and never splitting rows that share one.

    A bin is grown by adding whole tie groups until it holds its share of the rows; the
    leftover joins the last bin rather than forming a short one of its own. With a large
    point mass -- every row whose predicted q10 is 0 -- this yields fewer than ``N_BINS``
    bins, which is the honest resolution the predictions support."""
    order = np.argsort(predicted_level, kind="stable")
    sorted_values = predicted_level[order]
    tie_groups = np.split(order, np.flatnonzero(np.diff(sorted_values)) + 1)
    rows_per_bin = max(1, len(order) // N_BINS)
    bins: List[np.ndarray] = []
    pending: List[np.ndarray] = []
    pending_size = 0
    for group in tie_groups:
        pending.append(group)
        pending_size += len(group)
        if pending_size >= rows_per_bin:
            bins.append(np.concatenate(pending))
            pending, pending_size = [], 0
    if not pending:
        return bins
    leftover = np.concatenate(pending)
    if not bins:
        return [leftover]
    bins[-1] = np.concatenate([bins[-1], leftover])
    return bins


@dataclass
class QuantileRecalibration:
    """Per-level monotone maps from predicted quantile to corrected quantile."""

    levels: List[float]
    #: Per level: the isotonic map, fitted on the bins' predicted means against their
    #: empirical quantiles and clipped to the fitted range outside it.
    maps: List[IsotonicRegression]
    n_fit: int

    @classmethod
    def fit(
        cls, predicted: np.ndarray, y: np.ndarray, levels: Sequence[float] = QUANTILE_LEVELS
    ) -> "QuantileRecalibration":
        if len(y) < MIN_ROWS:
            raise ValueError(f"recalibration needs at least {MIN_ROWS} rows, got {len(y)}")
        maps = []
        for i, level in enumerate(levels):
            q = predicted[:, i]
            bins = _tie_safe_bins(q)
            x = np.asarray([q[b].mean() for b in bins])
            corrected = np.asarray([np.quantile(y[b], level) for b in bins])
            weights = np.asarray([len(b) for b in bins], dtype=float)
            # Isotonic in the predicted quantile: a higher forecast never maps lower.
            fitted = IsotonicRegression(increasing=True, out_of_bounds="clip")
            maps.append(fitted.fit(x, corrected, sample_weight=weights))
        return cls(list(levels), maps, int(len(y)))

    def apply(self, predicted: np.ndarray) -> np.ndarray:
        out = np.empty_like(predicted, dtype=float)
        for i in range(len(self.levels)):
            out[:, i] = self.maps[i].predict(predicted[:, i])
        # Corrected levels are re-sorted so the interval cannot cross after the map.
        return np.sort(out, axis=1)
