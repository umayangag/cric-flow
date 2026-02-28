"""Unit tests for ml.auto_tune_autogluon."""

from unittest.mock import patch

import numpy as np

from ml.auto_tune_autogluon import (
    _numpy_to_df,
    is_available,
    run_autogluon_classification,
    run_autogluon_regression,
)


def test_is_available():
    """is_available returns bool."""
    result = is_available()
    assert isinstance(result, bool)


def test_numpy_to_df_x_only():
    """_numpy_to_df converts X to DataFrame with f0, f1, ... columns."""
    X = np.array([[1, 2], [3, 4]])
    df = _numpy_to_df(X)
    assert list(df.columns) == ["f0", "f1"]
    assert df.shape == (2, 2)
    assert df.iloc[0, 0] == 1 and df.iloc[1, 1] == 4


def test_numpy_to_df_with_y():
    """_numpy_to_df adds _target when y is provided."""
    X = np.array([[1], [2]])
    y = np.array([10, 20])
    df = _numpy_to_df(X, y)
    assert "_target" in df.columns
    assert list(df["_target"]) == [10, 20]


def test_numpy_to_df_1d_y():
    """_numpy_to_df handles 1d y."""
    X = np.array([[1], [2], [3]])
    y = np.array([10, 20, 30])
    df = _numpy_to_df(X, y)
    assert list(df["_target"]) == [10, 20, 30]


@patch("ml.auto_tune_autogluon._HAS_AUTOGLUON", False)
def test_run_autogluon_regression_no_autogluon():
    """When AutoGluon is not installed, regression returns None, None, None, False."""
    X = np.random.RandomState(42).rand(20, 5)
    y = np.random.RandomState(43).rand(20)
    pred, score, path, success = run_autogluon_regression(X, y)
    assert pred is None
    assert score is None
    assert path is None
    assert success is False


@patch("ml.auto_tune_autogluon._HAS_AUTOGLUON", False)
def test_run_autogluon_classification_no_autogluon():
    """When AutoGluon is not installed, classification returns None, None, None, False."""
    X = np.random.RandomState(42).rand(20, 5)
    y = (np.random.RandomState(43).rand(20) > 0.5).astype(int)
    pred, score, path, success = run_autogluon_classification(X, y)
    assert pred is None
    assert score is None
    assert path is None
    assert success is False
