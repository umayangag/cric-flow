"""Constants, column definitions, and shared types for the tuning package."""

from __future__ import annotations

from typing import Any

# Phase 1 trials per algorithm; Phase 2 Optuna trials (when Optuna available)
PHASE1_TRIALS_PER_ALGORITHM = 5
PHASE2_TRIALS = 40

# Algorithm keys for filtering
AVAILABLE_ALGORITHMS = frozenset({"rf", "gb", "et", "hgb", "quantile", "stacked", "mlp"})

# ── Optional training module reference (for the data loader) ───────────
#
# One is left: the batting, bowling, fielding, extras and innings trainers went in P-5.

_train_win: Any = None

try:
    from ml import train_win as _tw

    _train_win = _tw
except ImportError:
    pass

# ── Phase 1 coarse parameter grids ─────────────────────────────────────

_PHASE1_COARSE_RF = {
    "est__estimator__n_estimators": [50, 150, 300, 500],
    "est__estimator__max_depth": [6, 12, 20],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_GB = {
    "est__estimator__n_estimators": [50, 150, 300, 500],
    "est__estimator__max_depth": [4, 8, 12],
    "est__estimator__learning_rate": [0.05, 0.15],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_ET = {
    "est__estimator__n_estimators": [50, 150, 300, 500],
    "est__estimator__max_depth": [6, 12, 20],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_HGB = {
    "est__estimator__max_iter": [100, 200, 300],
    "est__estimator__max_depth": [4, 8, 12],
    "est__estimator__learning_rate": [0.05, 0.15],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_MLP_REG = {
    "est__estimator__hidden_layer_sizes": [(64, 64), (128, 64), (128, 128, 64)],
    "est__estimator__activation": ["relu"],
    "est__estimator__alpha": [0.0001, 0.001],
    "est__estimator__max_iter": [500, 1000],
}
_PHASE1_COARSE_MLP_CLF = {
    "est__hidden_layer_sizes": [(64, 64), (128, 64)],
    "est__activation": ["relu"],
    "est__alpha": [0.0001, 0.001],
    "est__max_iter": [500, 1000],
}
