"""
Calibration utilities for probability outputs (e.g. win model classifiers).

Wraps classifiers with CalibratedClassifierCV (Platt scaling or isotonic) so that
predicted probabilities reflect true frequencies. Provides reliability diagram
computation for evaluation.

Usage:
  from ml.calibrate import calibrate_classifier, reliability_diagram_data
  calibrated = calibrate_classifier(clf, X_val, y_val, method='isotonic')
  bins_data = reliability_diagram_data(y_true, y_prob, n_bins=10)
"""

from __future__ import annotations

import logging
from typing import Any, Dict, List, Union

import numpy as np
from sklearn.calibration import CalibratedClassifierCV

logger = logging.getLogger(__name__)


def calibrate_classifier(
    base_estimator: Any,
    X: np.ndarray,
    y: np.ndarray,
    method: str = "isotonic",
    cv: Union[int, str] = 5,
) -> CalibratedClassifierCV:
    """
    Wrap a classifier with probability calibration.

    Args:
        base_estimator: Fitted or unfitted classifier with predict_proba.
        X: Features (n_samples, n_features).
        y: Binary labels 0/1.
        method: 'sigmoid' (Platt) or 'isotonic'.
        cv: Cross-validation splits for calibration fit; or 'prefit' if base is already fitted.

    Returns:
        CalibratedClassifierCV instance (fitted when cv != 'prefit').
    """
    calibrated = CalibratedClassifierCV(base_estimator, method=method, cv=cv)
    calibrated.fit(X, y)
    return calibrated


def reliability_diagram_data(
    y_true: np.ndarray,
    y_prob: np.ndarray,
    n_bins: int = 10,
) -> Dict[str, Any]:
    """
    Compute reliability diagram data: per-bin mean predicted prob and fraction of positives.

    A well-calibrated model has mean_predicted ≈ fraction_positives per bin.

    Args:
        y_true: Binary labels 0/1 (n_samples,).
        y_prob: Predicted probability of positive class (n_samples,).
        n_bins: Number of bins.

    Returns:
        Dict with:
            - bin_edges: edges of bins
            - mean_predicted: mean predicted prob per bin
            - fraction_positives: fraction of actual positives per bin
            - counts: sample count per bin
            - brier_score: Brier score (lower = better calibration)
    """
    y_true = np.asarray(y_true).ravel()
    y_prob = np.asarray(y_prob)
    if y_prob.ndim > 1:
        y_prob = y_prob[:, 1]  # positive class
    y_prob = y_prob.ravel()
    bins = np.linspace(0, 1, n_bins + 1)
    indices = np.digitize(y_prob, bins) - 1
    indices = np.clip(indices, 0, n_bins - 1)

    mean_predicted: List[float] = []
    fraction_positives: List[float] = []
    counts: List[int] = []

    for i in range(n_bins):
        mask = indices == i
        cnt = int(np.sum(mask))
        counts.append(cnt)
        if cnt == 0:
            mean_predicted.append(float("nan"))
            fraction_positives.append(float("nan"))
        else:
            mean_predicted.append(float(np.mean(y_prob[mask])))
            fraction_positives.append(float(np.mean(y_true[mask])))

    brier_score = float(np.mean((y_prob - y_true) ** 2))

    return {
        "bin_edges": bins.tolist(),
        "mean_predicted": mean_predicted,
        "fraction_positives": fraction_positives,
        "counts": counts,
        "brier_score": brier_score,
        "n_samples": int(len(y_true)),
    }


def evaluate_calibration(
    y_true: np.ndarray,
    y_prob: np.ndarray,
    n_bins: int = 10,
) -> Dict[str, Any]:
    """
    Evaluate calibration: Brier score and ECE (Expected Calibration Error).

    ECE = weighted mean of |mean_predicted - fraction_positives| per bin.
    """
    data = reliability_diagram_data(y_true, y_prob, n_bins=n_bins)
    ece = 0.0
    total = 0
    for i in range(n_bins):
        c = data["counts"][i]
        if c > 0:
            mp = data["mean_predicted"][i]
            fp = data["fraction_positives"][i]
            if not (np.isnan(mp) or np.isnan(fp)):
                ece += c * abs(mp - fp)
                total += c
    data["ece"] = float(ece / total) if total > 0 else float("nan")
    return data
