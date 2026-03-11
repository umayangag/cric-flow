"""Tests for SHAP-based feature importance (MLP and tree fallback)."""

from __future__ import annotations

import numpy as np
import pytest
from sklearn.neural_network import MLPClassifier, MLPRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler


def test_compute_shap_importance_mlp_regression_returns_dict():
    """compute_shap_importance returns mean |SHAP| dict for MLPRegressor."""
    pytest.importorskip("shap")
    from ml.shap_explanations import compute_shap_importance

    est = MLPRegressor(hidden_layer_sizes=(8,), max_iter=200, random_state=42)
    pipe = Pipeline([("scaler", StandardScaler()), ("est", est)])
    rng = np.random.RandomState(42)
    X = rng.randn(80, 4)
    y = rng.randn(80)
    pipe.fit(X, y)
    names = ["a", "b", "c", "d"]
    out = compute_shap_importance(pipe, X, feature_names=names, task_type="regression", max_background=20, max_eval=25)
    assert out is not None
    assert set(out.keys()) == set(names)
    for v in out.values():
        assert isinstance(v, (int, float)) and v >= 0


def test_compute_shap_importance_mlp_classification_returns_dict():
    """compute_shap_importance returns mean |SHAP| dict for MLPClassifier."""
    pytest.importorskip("shap")
    from ml.shap_explanations import compute_shap_importance

    est = MLPClassifier(hidden_layer_sizes=(8,), max_iter=200, random_state=42)
    pipe = Pipeline([("scaler", StandardScaler()), ("est", est)])
    rng = np.random.RandomState(42)
    X = rng.randn(100, 4)
    y = (X[:, 0] + X[:, 1] > 0).astype(int)
    pipe.fit(X, y)
    names = ["f0", "f1", "f2", "f3"]
    out = compute_shap_importance(pipe, X, feature_names=names, task_type="classification", max_background=20, max_eval=25)
    assert out is not None
    assert set(out.keys()) == set(names)


def test_compute_shap_importance_tree_returns_dict():
    """compute_shap_importance returns dict for tree estimator (TreeExplainer path)."""
    pytest.importorskip("shap")
    from sklearn.ensemble import RandomForestRegressor

    from ml.shap_explanations import compute_shap_importance

    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = Pipeline([("scaler", StandardScaler()), ("est", est)])
    rng = np.random.RandomState(42)
    X = rng.randn(60, 3)
    y = rng.randn(60)
    pipe.fit(X, y)
    out = compute_shap_importance(pipe, X, feature_names=None, task_type="regression", max_background=20)
    assert out is not None
    assert len(out) == 3
    assert all(k.startswith("feature_") for k in out.keys())


def test_add_final_report_details_with_mlp_uses_shap():
    """_add_final_report_details populates feature_importance from SHAP when estimator is MLP."""
    pytest.importorskip("shap")
    from sklearn.model_selection import KFold

    from ml.tuning import cv_metrics
    from ml.tuning.search_space import _build_pipeline_single_regression

    est = MLPRegressor(hidden_layer_sizes=(8,), max_iter=150, random_state=42)
    pipe = _build_pipeline_single_regression(est)
    rng = np.random.RandomState(42)
    X = rng.randn(70, 5)
    y = rng.randn(70)
    pipe.fit(X, y)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {"best_cv_score": -0.4}
    cv_metrics._add_final_report_details(
        report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", "extras"
    )
    assert "mlqa_audit" in report
    assert "feature_importance" in report
    assert report.get("explainer") == "shap"
