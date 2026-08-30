"""
AutoGluon integration for auto-tune Stage 3.

Fits TabularPredictor as an alternative to Optuna-tuned sklearn models.
When AutoGluon achieves better validation score than Optuna, we use it.
Saves predictor to a directory + wrapper joblib for go-app compatibility.
"""

from __future__ import annotations

import logging
import tempfile
from typing import Optional, Tuple

import numpy as np
import pandas as pd

logger = logging.getLogger(__name__)

_HAS_AUTOGLUON = False
try:
    from autogluon.tabular import TabularPredictor

    _HAS_AUTOGLUON = True
except ImportError:
    pass


def is_available() -> bool:
    return _HAS_AUTOGLUON


def _numpy_to_df(X: np.ndarray, y: Optional[np.ndarray] = None) -> pd.DataFrame:
    """Convert X, y to DataFrame with column names AutoGluon expects."""
    n_cols = X.shape[1]
    cols = [f"f{i}" for i in range(n_cols)]
    df = pd.DataFrame(X, columns=cols)
    if y is not None:
        df["_target"] = np.asarray(y).ravel()
    return df


def run_autogluon_regression(
    X: np.ndarray,
    y: np.ndarray,
    time_limit_seconds: int = 300,
    presets: str = "medium_quality",
    random_state: int = 42,
) -> Tuple[Optional[TabularPredictor], Optional[float], Optional[str], bool]:
    """
    Fit AutoGluon TabularPredictor for regression.

    Returns:
        (predictor, neg_mae_score, save_path, success)
        neg_mae_score: negative MAE (higher is better, comparable to Optuna scoring).
        save_path: directory where predictor is saved (for wrapper).
    """
    if not _HAS_AUTOGLUON:
        logger.info("auto_tune_autogluon.autogluon_not_installed skip")
        return None, None, None, False

    try:
        df = _numpy_to_df(X, y)
        persist_path = tempfile.mkdtemp(prefix="autogluon_reg_")
        predictor = TabularPredictor(
            label="_target",
            problem_type="regression",
            eval_metric="mean_absolute_error",
            path=persist_path,
        )
        predictor.fit(df, time_limit=time_limit_seconds, presets=presets, verbosity=1)
        leaderboard = predictor.leaderboard(silent=True)
        if leaderboard is None or leaderboard.empty:
            return None, None, None, False
        mae = float(leaderboard["score_val"].iloc[0])
        neg_mae = -mae
        predictor.save()
        return predictor, neg_mae, persist_path, True
    except Exception as e:
        logger.warning("auto_tune_autogluon.regression_failed error=%s", e, exc_info=True)
        return None, None, None, False


def run_autogluon_classification(
    X: np.ndarray,
    y: np.ndarray,
    time_limit_seconds: int = 300,
    presets: str = "medium_quality",
    eval_metric: str = "roc_auc",
) -> Tuple[Optional[TabularPredictor], Optional[float], Optional[str], bool]:
    """
    Fit AutoGluon TabularPredictor for binary classification.

    ``eval_metric`` comes from the caller because the score is compared against the
    Optuna search's, and whichever is higher wins. The two must measure the same thing;
    comparing an AUC with an accuracy would decide the win model on a category error.

    Returns:
        (predictor, score_on_eval_metric, save_path, success)
    """
    if not _HAS_AUTOGLUON:
        return None, None, None, False

    try:
        y_flat = np.asarray(y).ravel().astype(int)
        df = _numpy_to_df(X, y_flat)
        persist_path = tempfile.mkdtemp(prefix="autogluon_clf_")
        predictor = TabularPredictor(
            label="_target",
            problem_type="binary",
            eval_metric=eval_metric,
            path=persist_path,
        )
        predictor.fit(df, time_limit=time_limit_seconds, presets=presets, verbosity=1)
        leaderboard = predictor.leaderboard(silent=True)
        if leaderboard is None or leaderboard.empty:
            return None, None, None, False
        score = float(leaderboard["score_val"].iloc[0])
        predictor.save()
        return predictor, score, persist_path, True
    except Exception as e:
        logger.warning("auto_tune_autogluon.classification_failed error=%s", e, exc_info=True)
        return None, None, None, False
