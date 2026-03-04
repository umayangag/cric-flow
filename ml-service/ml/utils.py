"""Shared utilities for ML training and prediction."""

from typing import Any, Dict, List, Optional

import numpy as np
from sklearn.ensemble import GradientBoostingRegressor, RandomForestRegressor, StackingRegressor
from sklearn.linear_model import Ridge


def make_base_estimator(params: Dict[str, Any]) -> Any:
    """Build RF, GBM, quantile, or stacked (RF+GB+Ridge) base estimator from training params.

    Used by train_batting, train_bowling, train_fielding, and app.train_on_the_fly
    to avoid duplicating estimator construction. params is the dict returned by
    get_training_params(model) (e.g. n_estimators, max_depth, random_state, estimator,
    learning_rate, quantile_level, n_jobs).
    """
    n_estimators = params["n_estimators"]
    max_depth = params["max_depth"]
    random_state = params["random_state"]
    n_jobs = params.get("n_jobs", -1)
    learning_rate = params.get("learning_rate", 0.1)
    est_type = params.get("estimator", "rf")
    quantile_level = params.get("quantile_level", 0.5)
    if est_type == "quantile":
        return GradientBoostingRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            learning_rate=learning_rate,
            loss="quantile",
            alpha=quantile_level,
        )
    if est_type == "stacked":
        rf = RandomForestRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            n_jobs=n_jobs,
        )
        gb = GradientBoostingRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            learning_rate=learning_rate,
        )
        return StackingRegressor(
            estimators=[("rf", rf), ("gb", gb)],
            final_estimator=Ridge(alpha=1.0, random_state=random_state),
        )
    if est_type == "gb":
        return GradientBoostingRegressor(
            n_estimators=n_estimators,
            max_depth=max_depth,
            random_state=random_state,
            learning_rate=learning_rate,
        )
    return RandomForestRegressor(
        n_estimators=n_estimators,
        max_depth=max_depth,
        random_state=random_state,
        n_jobs=n_jobs,
    )


def extract_feature_importance_from_estimator(
    estimator: Any,
    feature_names: Optional[List[str]],
    max_features: Optional[int] = None,
) -> Optional[Dict[str, float]]:
    """Extract average feature importance from a tree-based estimator or MultiOutputRegressor.

    Returns a dict mapping feature name to importance, or None when importances are not available.
    Handles single estimators with feature_importances_, MultiOutputRegressor-style estimators_,
    and multi-output importance arrays by averaging across outputs.
    """
    if estimator is None:
        return None

    imps = None
    estimators = getattr(estimator, "estimators_", None)
    if estimators:
        imp_list = [e.feature_importances_ for e in estimators if hasattr(e, "feature_importances_")]
        if imp_list:
            imps = np.mean(imp_list, axis=0)
    elif hasattr(estimator, "feature_importances_"):
        imps = estimator.feature_importances_

    if imps is None:
        return None

    # Multi-output case: average across outputs
    if hasattr(imps, "ndim") and imps.ndim > 1:
        imps = np.mean(imps, axis=0)

    n_total = len(imps)
    if n_total == 0:
        return None
    n = n_total if max_features is None else max(0, min(int(max_features), n_total))
    if n == 0:
        return None

    if feature_names and len(feature_names) >= n:
        names = list(feature_names)[:n]
    else:
        names = [f"feature_{i}" for i in range(n)]

    return {names[i]: float(imps[i]) for i in range(n)}
