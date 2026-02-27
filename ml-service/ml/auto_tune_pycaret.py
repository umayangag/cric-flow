"""
PyCaret-based algorithm ranking for auto-tune Stage 1.

Uses PyCaret compare_models() to quickly evaluate 15+ algorithms and return
the top N for downstream Optuna fine-tuning. Gracefully falls back when PyCaret
is not installed.

Maps PyCaret model IDs to our algorithm keys: rf, gb, et, hgb, mlp, etc.
"""

from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd

logger = logging.getLogger(__name__)

_HAS_PYCARET = False
try:
    from pycaret.regression import compare_models, setup as setup_regression
    from pycaret.classification import compare_models as compare_models_clf, setup as setup_classification

    _HAS_PYCARET = True
except ImportError:
    pass

# Map PyCaret model IDs to our algorithm keys (rf, gb, et, hgb, mlp, etc.)
# PyCaret regression: rf, et, gbr, lightgbm, xgboost, mlp, ...
# PyCaret classification: rf, et, gbc, lightgbm, xgboost, mlp, ...
_PYCARET_TO_OUR_KEY: Dict[str, str] = {
    "rf": "rf",
    "Random Forest": "rf",
    "et": "et",
    "Extra Trees": "et",
    "gbr": "gb",
    "gbc": "gb",
    "Gradient Boosting": "gb",
    "Gradient Boosting Classifier": "gb",
    "Gradient Boosting Regressor": "gb",
    "lightgbm": "lightgbm",
    "Light Gradient Boosting": "lightgbm",
    "xgboost": "xgboost",
    "Extreme Gradient Boosting": "xgboost",
    "mlp": "mlp",
    "MLP Regressor": "mlp",
    "MLP Classifier": "mlp",
    "hgb": "hgb",
    "Hist Gradient Boosting": "hgb",
}

# Our supported algorithms that Optuna can tune (we don't have lightgbm/xgboost in Optuna yet, so map to closest)
_OUR_OPTUNA_ALGORITHMS = frozenset({"rf", "gb", "et", "hgb", "mlp"})
_FALLBACK_FOR_UNSUPPORTED = "gb"  # when PyCaret returns lightgbm/xgboost, we use gb for Optuna


def _model_id_to_our_key(model_id: str) -> str:
    """Map PyCaret model ID or name to our algorithm key."""
    s = str(model_id).strip().lower()
    for pycaret_id, our_key in _PYCARET_TO_OUR_KEY.items():
        if pycaret_id.lower() == s:
            return our_key
    # try by match
    if "random" in s and "forest" in s:
        return "rf"
    if "extra" in s and "tree" in s:
        return "et"
    if "gradient" in s and "boost" in s and "hist" not in s:
        return "gb"
    if "hist" in s or "hgb" in s:
        return "hgb"
    if "light" in s or "lgb" in s:
        return "lightgbm"
    if "xgboost" in s or "xgb" in s:
        return "xgboost"
    if "mlp" in s or "neural" in s:
        return "mlp"
    return "rf"


def _our_key_for_optuna(our_key: str) -> str:
    """Map to an algorithm we support in Optuna. lightgbm/xgboost -> gb."""
    if our_key in _OUR_OPTUNA_ALGORITHMS:
        return our_key
    return _FALLBACK_FOR_UNSUPPORTED


def run_pycaret_ranking_regression(
    X: np.ndarray,
    y: np.ndarray,
    cv_splits: int = 5,
    n_select: int = 3,
    random_state: int = 42,
    include: Optional[List[str]] = None,
    exclude: Optional[List[str]] = None,
    verbose: bool = False,
) -> Tuple[List[str], List[Dict[str, Any]], bool]:
    """
    Run PyCaret compare_models for regression; return top algorithm keys for Optuna.

    For multi-output Y, uses first column only for ranking (e.g. runs for batting).

    Returns:
        (algorithms_for_optuna, ranking_details, success)
        algorithms_for_optuna: list of our keys (rf, gb, et, hgb, mlp) to pass to Optuna
        ranking_details: list of {model_id, our_key, score, ...} for logging
        success: True if PyCaret ran successfully
    """
    if not _HAS_PYCARET:
        logger.info("auto_tune_pycaret.pycaret_not_installed skip_ranking")
        return [], [], False

    try:
        # Flatten y if multi-output
        y_flat = y.ravel() if y.ndim > 1 and y.shape[1] > 1 else (y.ravel() if y.ndim > 1 else y)
        df = pd.DataFrame(X)
        df["_target"] = y_flat
        n_cols = X.shape[1]
        feature_cols = [str(i) for i in range(n_cols)]

        setup_kwargs: Dict[str, Any] = {
            "data": df,
            "target": "_target",
            "fold": cv_splits,
            "session_id": random_state,
            "verbose": verbose,
            "html": False,
        }
        if include:
            setup_kwargs["include"] = include

        setup_regression(**setup_kwargs)
        compare_result = compare_models(include=include, exclude=exclude, n_select=n_select, verbose=verbose)

        if compare_result is None:
            return [], [], False

        # compare_models can return single model or list when n_select > 1
        models = compare_result if isinstance(compare_result, list) else [compare_result]
        ranking: List[Dict[str, Any]] = []
        seen: set = set()
        algorithms: List[str] = []

        for i, m in enumerate(models):
            model_id = getattr(m, "__class__", m).__name__ if hasattr(m, "__class__") else str(m)
            our_key = _model_id_to_our_key(model_id)
            optuna_key = _our_key_for_optuna(our_key)
            ranking.append({"model_id": model_id, "our_key": our_key, "optuna_key": optuna_key, "rank": i + 1})
            if optuna_key not in seen:
                seen.add(optuna_key)
                algorithms.append(optuna_key)
            if len(algorithms) >= n_select:
                break

        if not algorithms:
            return [], ranking, True
        return algorithms, ranking, True

    except Exception as e:
        logger.warning("auto_tune_pycaret.regression_failed error=%s", e, exc_info=True)
        return [], [], False


def run_pycaret_ranking_classification(
    X: np.ndarray,
    y: np.ndarray,
    cv_splits: int = 5,
    n_select: int = 3,
    random_state: int = 42,
    include: Optional[List[str]] = None,
    exclude: Optional[List[str]] = None,
    verbose: bool = False,
) -> Tuple[List[str], List[Dict[str, Any]], bool]:
    """
    Run PyCaret compare_models for classification; return top algorithm keys for Optuna.

    Returns:
        (algorithms_for_optuna, ranking_details, success)
    """
    if not _HAS_PYCARET:
        logger.info("auto_tune_pycaret.pycaret_not_installed skip_ranking")
        return [], [], False

    try:
        y_flat = np.asarray(y).ravel().astype(int)
        df = pd.DataFrame(X)
        df["_target"] = y_flat
        n_cols = X.shape[1]
        feature_cols = [str(i) for i in range(n_cols)]

        setup_kwargs = {
            "data": df,
            "target": "_target",
            "fold": cv_splits,
            "session_id": random_state,
            "verbose": verbose,
            "html": False,
        }
        if include:
            setup_kwargs["include"] = include

        setup_classification(**setup_kwargs)
        compare_result = compare_models_clf(include=include, exclude=exclude, n_select=n_select, verbose=verbose)

        if compare_result is None:
            return [], [], False

        models = compare_result if isinstance(compare_result, list) else [compare_result]
        ranking: List[Dict[str, Any]] = []
        seen: set = set()
        algorithms: List[str] = []

        for i, m in enumerate(models):
            model_id = getattr(m, "__class__", m).__name__ if hasattr(m, "__class__") else str(m)
            our_key = _model_id_to_our_key(model_id)
            optuna_key = _our_key_for_optuna(our_key)
            ranking.append({"model_id": model_id, "our_key": our_key, "optuna_key": optuna_key, "rank": i + 1})
            if optuna_key not in seen:
                seen.add(optuna_key)
                algorithms.append(optuna_key)
            if len(algorithms) >= n_select:
                break

        if not algorithms:
            return [], ranking, True
        return algorithms, ranking, True

    except Exception as e:
        logger.warning("auto_tune_pycaret.classification_failed error=%s", e, exc_info=True)
        return [], [], False


def is_available() -> bool:
    """Return True if PyCaret is installed and usable."""
    return _HAS_PYCARET
