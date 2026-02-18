"""Shared utilities for ML training and prediction."""

from typing import Any, Dict

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
