"""
Wrapper for AutoGluon TabularPredictor to expose predict(X) compatible with sklearn/go-app.

Saves predictor to a directory and stores the path; at load time, lazy-loads the predictor.
Used when AutoGluon wins over Optuna and we need to save artifacts that go-app can load.
"""

from __future__ import annotations

import logging
import os
from typing import Any, Union

import numpy as np
import pandas as pd

logger = logging.getLogger(__name__)

_HAS_AUTOGLUON = False
try:
    from autogluon.tabular import TabularPredictor

    _HAS_AUTOGLUON = True
except ImportError:
    pass


class AutogluonPredictorWrapper:
    """
    Wrapper around AutoGluon TabularPredictor for regression or classification.

    Exposes predict(X) for numpy arrays; compatible with joblib save/load.
    The predictor is stored at _path and lazy-loaded on first predict.
    """

    def __init__(self, path: str, is_regression: bool = True):
        """
        Args:
            path: Directory where TabularPredictor was saved (predictor.save(path) or fit path).
            is_regression: True for regression, False for classification.
        """
        self._path = str(path)
        self._is_regression = bool(is_regression)
        self._predictor: Any = None

    def _ensure_loaded(self) -> None:
        if self._predictor is None:
            if not _HAS_AUTOGLUON:
                raise RuntimeError("AutoGluon not installed; cannot load predictor")
            if not os.path.isdir(self._path):
                raise FileNotFoundError(f"AutoGluon predictor path not found: {self._path}")
            self._predictor = TabularPredictor.load(self._path)

    def predict(self, X: Union[np.ndarray, pd.DataFrame]) -> np.ndarray:
        """Predict on X (n_samples, n_features). Returns (n_samples,) or (n_samples, n_targets)."""
        self._ensure_loaded()
        if isinstance(X, np.ndarray):
            # AutoGluon expects DataFrame with column names
            n_cols = X.shape[1]
            df = pd.DataFrame(X, columns=[f"f{i}" for i in range(n_cols)])
        else:
            df = X
        pred = self._predictor.predict(df)
        out = np.asarray(pred)
        if out.ndim == 1:
            return out
        return out

    def predict_proba(self, X: Union[np.ndarray, pd.DataFrame]) -> np.ndarray:
        """Predict class probabilities (classification only). Returns (n_samples, n_classes)."""
        if self._is_regression:
            raise ValueError("predict_proba is only for classification")
        self._ensure_loaded()
        if isinstance(X, np.ndarray):
            n_cols = X.shape[1]
            df = pd.DataFrame(X, columns=[f"f{i}" for i in range(n_cols)])
        else:
            df = X
        proba = self._predictor.predict_proba(df)
        arr = np.asarray(proba)
        if arr.ndim == 1:
            arr = arr.reshape(-1, 1)
        return arr

    @property
    def path(self) -> str:
        return self._path

    @property
    def is_regression(self) -> bool:
        return self._is_regression

    @property
    def classes_(self) -> np.ndarray:
        """For compatibility with sklearn classifiers (e.g. predict_win). Binary: [0, 1]."""
        if self._is_regression:
            raise AttributeError("classes_ is only for classification")
        self._ensure_loaded()
        return np.array([0, 1])

    def __getstate__(self) -> dict:
        return {"_path": self._path, "_is_regression": self._is_regression}

    def __setstate__(self, state: dict) -> None:
        self._path = state["_path"]
        self._is_regression = state["_is_regression"]
        self._predictor = None


def is_available() -> bool:
    """Return True if AutoGluon is installed."""
    return _HAS_AUTOGLUON
