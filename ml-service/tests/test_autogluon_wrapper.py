"""Unit tests for ml.autogluon_wrapper."""

import numpy as np
import pytest

from ml.autogluon_wrapper import AutogluonPredictorWrapper, is_available


def test_is_available():
    """is_available returns bool."""
    result = is_available()
    assert isinstance(result, bool)


def test_autogluon_predictor_wrapper_init():
    """AutogluonPredictorWrapper stores path and is_regression."""
    w = AutogluonPredictorWrapper("/fake/path", is_regression=True)
    assert w.path == "/fake/path"
    assert w.is_regression is True


def test_autogluon_predictor_wrapper_classification():
    """is_regression=False for classification."""
    w = AutogluonPredictorWrapper("/fake/path", is_regression=False)
    assert w.is_regression is False


def test_autogluon_predictor_wrapper_getstate_setstate():
    """Wrapper is pickle-compatible via __getstate__/__setstate__."""
    w = AutogluonPredictorWrapper("/some/dir", is_regression=True)
    state = w.__getstate__()
    assert state == {"_path": "/some/dir", "_is_regression": True}

    w2 = AutogluonPredictorWrapper.__new__(AutogluonPredictorWrapper)
    w2.__setstate__(state)
    assert w2.path == "/some/dir"
    assert w2.is_regression is True
    assert w2._predictor is None


def test_autogluon_predictor_wrapper_predict_proba_regression_raises():
    """predict_proba raises when is_regression=True."""
    w = AutogluonPredictorWrapper("/fake/path", is_regression=True)
    with pytest.raises(ValueError, match="predict_proba is only for classification"):
        w.predict_proba(np.array([[1, 2]]))


def test_autogluon_predictor_wrapper_classes_regression_raises():
    """classes_ raises when is_regression=True."""
    w = AutogluonPredictorWrapper("/fake/path", is_regression=True)
    with pytest.raises(AttributeError, match="classes_ is only for classification"):
        _ = w.classes_
