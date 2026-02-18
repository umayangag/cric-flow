"""Tests for ml.calibrate module."""

import numpy as np
from sklearn.linear_model import LogisticRegression
from sklearn.neural_network import MLPClassifier

from ml.calibrate import calibrate_classifier, evaluate_calibration, reliability_diagram_data


def test_reliability_diagram_data_perfect():
    """Perfectly calibrated: predicted prob = true fraction."""
    np.random.seed(42)
    n = 200
    y_true = np.random.binomial(1, 0.5, n)
    y_prob = y_true.astype(float)  # perfect prediction
    data = reliability_diagram_data(y_true, y_prob, n_bins=5)
    assert data["brier_score"] < 0.01
    assert data["n_samples"] == n
    assert len(data["mean_predicted"]) == 5
    assert len(data["counts"]) == 5


def test_reliability_diagram_data_with_positive_class_index():
    """y_prob can be (n, 2) for binary; we use positive class."""
    np.random.seed(42)
    n = 100
    y_true = np.random.binomial(1, 0.6, n)
    p = np.clip(y_true.astype(float) + np.random.randn(n) * 0.2, 0.01, 0.99)
    y_prob = np.column_stack([1 - p, p])
    data = reliability_diagram_data(y_true, y_prob, n_bins=5)
    assert "brier_score" in data
    assert 0 <= data["brier_score"] <= 1


def test_evaluate_calibration_adds_ece():
    """evaluate_calibration adds ECE field."""
    np.random.seed(42)
    y_true = np.random.binomial(1, 0.5, 50)
    y_prob = np.random.rand(50)
    data = evaluate_calibration(y_true, y_prob, n_bins=5)
    assert "ece" in data
    assert 0 <= data["ece"] <= 1 or np.isnan(data["ece"])


def test_calibrate_classifier_isotonic():
    """Calibrate classifier with isotonic method."""
    np.random.seed(42)
    X = np.random.randn(100, 5)
    y = (X[:, 0] + X[:, 1] + np.random.randn(100) * 0.5 > 0).astype(int)
    clf = LogisticRegression(random_state=42).fit(X, y)
    calibrated = calibrate_classifier(clf, X, y, method="isotonic", cv=3)
    probs = calibrated.predict_proba(X)[:, 1]
    assert probs.shape == (100,)
    assert np.all(probs >= 0) and np.all(probs <= 1)


def test_calibrate_classifier_prefit():
    """Calibrate with prefit base estimator."""
    np.random.seed(42)
    X = np.random.randn(80, 4)
    y = (X[:, 0] > 0).astype(int)
    X_tr, X_val = X[:60], X[60:]
    y_tr, y_val = y[:60], y[60:]
    clf = MLPClassifier(hidden_layer_sizes=(10,), max_iter=200, random_state=42)
    clf.fit(X_tr, y_tr)
    calibrated = calibrate_classifier(clf, X_val, y_val, method="sigmoid", cv="prefit")
    probs = calibrated.predict_proba(X_val)[:, 1]
    assert probs.shape == (20,)
    assert np.all(probs >= 0) and np.all(probs <= 1)
