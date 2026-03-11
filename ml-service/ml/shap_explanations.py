"""SHAP (SHapley Additive exPlanations) for auto-tuned models.

Provides feature importance via mean absolute SHAP values for models that do not
expose feature_importances_ (e.g. MLPClassifier, MLPRegressor) and optionally
for tree-based models as a consistent alternative. Used by auto-tune report
generation when tree-based importance is not available.
"""

from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
from sklearn.pipeline import Pipeline

logger = logging.getLogger(__name__)

# Tree-based estimators that support SHAP TreeExplainer (exact, fast)
_TREE_ESTIMATOR_CLASSES = (
    "RandomForestClassifier",
    "RandomForestRegressor",
    "GradientBoostingClassifier",
    "GradientBoostingRegressor",
    "ExtraTreesClassifier",
    "ExtraTreesRegressor",
    "HistGradientBoostingClassifier",
    "HistGradientBoostingRegressor",
)


def _is_tree_estimator(est: Any) -> bool:
    """Return True if the estimator is tree-based and supports TreeExplainer."""
    if est is None:
        return False
    return type(est).__name__ in _TREE_ESTIMATOR_CLASSES


def _get_background_and_estimator(
    pipe: Pipeline,
    X: np.ndarray,
    max_background: int,
    random_state: int = 42,
) -> Tuple[np.ndarray, Any, bool]:
    """Get background sample and the estimator to explain; handle scaler if present.

    Returns (X_background, estimator, use_transformed).
    When use_transformed is True, X_background is scaled and caller must pass
    scaled data to the explainer.
    """
    rng = np.random.default_rng(random_state)
    n = X.shape[0]
    size = min(max_background, n)
    indices = rng.choice(n, size=size, replace=False) if n > size else np.arange(n)
    X_sample = X[indices]

    if pipe is None or not hasattr(pipe, "named_steps"):
        return X_sample, pipe, False

    scaler = pipe.named_steps.get("scaler")
    est = pipe.named_steps.get("est")
    if scaler is not None and est is not None:
        X_transformed = scaler.transform(X_sample)
        return X_transformed, est, True
    return X_sample, est or pipe, False


def compute_shap_importance(
    pipe: Pipeline,
    X: np.ndarray,
    feature_names: Optional[List[str]] = None,
    task_type: str = "regression",
    max_background: int = 100,
    max_eval: int = 300,
    random_state: int = 42,
) -> Optional[Dict[str, float]]:
    """Compute mean absolute SHAP value per feature as a substitute for feature importance.

    For tree-based estimators uses TreeExplainer (exact, fast). For MLP and other
    models uses KernelExplainer with a background sample and limited evaluations.

    Args:
        pipe: Fitted sklearn Pipeline (e.g. scaler + est).
        X: Feature matrix used to compute SHAP (e.g. training or holdout).
        feature_names: Optional list of feature names; defaults to feature_0, feature_1, ...
        task_type: "regression" or "classification".
        max_background: Max number of background samples for explainer.
        max_eval: Max evaluations for KernelExplainer (ignored for TreeExplainer).
        random_state: For background sampling.

    Returns:
        Dict mapping feature name to mean absolute SHAP value, or None on failure.
    """
    try:
        import shap
    except ImportError:
        logger.warning("shap not installed; skipping SHAP importance")
        return None

    if X.size == 0 or X.shape[1] == 0:
        return None

    n_features = X.shape[1]
    if feature_names and len(feature_names) >= n_features:
        names = list(feature_names)[:n_features]
    else:
        names = [f"feature_{i}" for i in range(n_features)]

    X_bg, est, use_transformed = _get_background_and_estimator(pipe, X, max_background, random_state)
    if est is None:
        return None

    is_tree = _is_tree_estimator(est)
    if is_tree:
        try:
            explainer = shap.TreeExplainer(est, X_bg, feature_perturbation="interventional")
            # For classification, shap_values is list of arrays per class; use positive class
            if task_type == "classification" and hasattr(est, "classes_") and len(est.classes_) == 2:
                sv = explainer.shap_values(X_bg)
                if isinstance(sv, list):
                    # Binary: use class 1
                    vals = np.asarray(sv[1])
                else:
                    vals = np.asarray(sv)
            else:
                vals = np.asarray(explainer.shap_values(X_bg))
            if vals.ndim == 3:
                vals = vals[:, :, -1] if vals.shape[2] > 1 else vals[:, :, 0]
            mean_abs = np.abs(vals).mean(axis=0)
        except Exception as e:
            logger.warning("shap.TreeExplainer failed: %s", e)
            return None
    else:
        # MLP or other: use KernelExplainer; pipeline receives raw X
        model_for_shap = pipe
        rng = np.random.default_rng(random_state)
        n = X.shape[0]
        bg_size = min(max_background, n)
        bg_idx = rng.choice(n, size=bg_size, replace=False) if n > bg_size else np.arange(n)
        X_bg_raw = X[bg_idx]

        try:
            if task_type == "classification":
                predict_fn = lambda x: model_for_shap.predict_proba(x)  # noqa: E731
            else:
                predict_fn = lambda x: model_for_shap.predict(x)  # noqa: E731
            explainer = shap.KernelExplainer(predict_fn, X_bg_raw)
            sv = explainer.shap_values(X_bg_raw, nsamples=max_eval, silent=True)
            if isinstance(sv, list):
                sv = sv[1] if len(sv) == 2 else sv[0]
            vals = np.asarray(sv)
            if vals.ndim == 3:
                vals = vals[:, :, -1] if vals.shape[2] > 1 else vals[:, :, 0]
            mean_abs = np.abs(vals).mean(axis=0)
        except Exception as e:
            logger.warning("shap.KernelExplainer failed: %s", e)
            return None

    if len(mean_abs) != len(names):
        return None
    return {names[i]: float(mean_abs[i]) for i in range(len(names))}
