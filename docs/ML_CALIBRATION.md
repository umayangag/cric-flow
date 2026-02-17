# Probability calibration for classifiers

This document describes how to **calibrate predicted probabilities** for classifiers (e.g. win model) and how to **evaluate calibration** with reliability diagrams.

---

## 1. Overview

Many classifiers (e.g. MLP, RandomForest) output scores that are not well-calibrated: a predicted probability of 0.7 may not correspond to 70% actual positive rate. **Calibration** adjusts these so that predicted probabilities reflect true frequencies.

**Methods:**
- **Platt scaling (sigmoid):** Fits a logistic regression on the outputs.
- **Isotonic regression:** Non-parametric; often better for complex patterns.

**Evaluation:**
- **Reliability diagram:** Plot mean predicted prob vs fraction of positives per bin; ideal = diagonal.
- **Brier score:** Mean squared error of probabilities; lower = better.
- **ECE (Expected Calibration Error):** Weighted mean of |predicted - actual| per bin.

---

## 2. Module: `ml.calibrate`

### `calibrate_classifier(base_estimator, X, y, method='isotonic', cv=5)`

Wraps a classifier with `sklearn.calibration.CalibratedClassifierCV`.

```python
from sklearn.linear_model import LogisticRegression
from ml.calibrate import calibrate_classifier

clf = LogisticRegression().fit(X_train, y_train)
calibrated = calibrate_classifier(clf, X_val, y_val, method="isotonic", cv="prefit")
# Or: calibrated = calibrate_classifier(LogisticRegression(), X_train, y_train, method="isotonic", cv=5)
probs = calibrated.predict_proba(X_test)[:, 1]
```

### `reliability_diagram_data(y_true, y_prob, n_bins=10)`

Returns per-bin mean predicted prob, fraction of positives, counts, and Brier score.

```python
from ml.calibrate import reliability_diagram_data

data = reliability_diagram_data(y_true, y_prob, n_bins=10)
# data["mean_predicted"], data["fraction_positives"], data["brier_score"]
```

### `evaluate_calibration(y_true, y_prob, n_bins=10)`

Same as above, plus `ece` (Expected Calibration Error).

---

## 3. When to use

- **Win model:** If you train a classifier for match/team win (e.g. `win_model_<FMT>.joblib`), calibrate on a held-out set before deployment. Use `method='isotonic'` when you have enough samples per bin.
- **Per-player winning probability:** If the win model outputs per-player contribution probabilities, calibration improves interpretability (e.g. "60% win contribution" ≈ 60% of such players are in winning teams).

---

## 4. Workflow

1. **Train** base classifier on training set.
2. **Calibrate** on validation set (same cutoff rules; no leakage):
   ```python
   calibrated = calibrate_classifier(clf, X_val, y_val, method="isotonic", cv="prefit")
   ```
3. **Save** the calibrated model (joblib) instead of or in addition to the base model.
4. **Evaluate** on test set with `evaluate_calibration`; inspect Brier score and ECE. Optionally plot a reliability diagram.

---

## 5. References

- sklearn: `sklearn.calibration.CalibratedClassifierCV`
- `ml-service/ml/calibrate.py`
- Win model artifacts: `win_model_<FMT>.joblib` (see **docs/ML_MODELS_COMBINED.md**)
