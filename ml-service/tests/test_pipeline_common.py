"""Unit tests for ml.pipeline_common."""

import numpy as np
import pandas as pd

from ml.pipeline_common import (
    brier_score,
    check_delta_guardrail,
    compute_time_decay_weights,
    get_scaler,
)


def test_compute_time_decay_weights_valid():
    """compute_time_decay_weights returns exponential decay for valid dates."""
    dates = pd.Series(pd.date_range("2020-01-01", periods=5, freq="365D"))
    w = compute_time_decay_weights(dates, halflife_years=2.0)
    assert w is not None
    assert len(w) == 5
    assert w[-1] > w[0]
    np.testing.assert_allclose(w[-1], 1.0, rtol=1e-5)


def test_compute_time_decay_weights_none():
    """compute_time_decay_weights returns None for None dates."""
    assert compute_time_decay_weights(None) is None


def test_compute_time_decay_weights_empty():
    """compute_time_decay_weights returns None for empty series."""
    assert compute_time_decay_weights(pd.Series(dtype=object)) is None


def test_compute_time_decay_weights_all_invalid():
    """compute_time_decay_weights returns None when all dates invalid."""
    dates = pd.Series(["not-a-date", "also-invalid"])
    assert compute_time_decay_weights(dates) is None


def test_compute_time_decay_weights_reference_date_datetime():
    """compute_time_decay_weights handles datetime reference_date."""
    dates = pd.Series(pd.date_range("2022-01-01", periods=3, freq="D"))
    ref = pd.Timestamp("2022-01-03")
    w = compute_time_decay_weights(dates, reference_date=ref)
    assert w is not None
    assert len(w) == 3
    np.testing.assert_allclose(w[-1], 1.0, rtol=1e-5)


def test_get_scaler_robust():
    """get_scaler returns RobustScaler when use_robust=True."""
    scaler = get_scaler(use_robust=True)
    from sklearn.preprocessing import RobustScaler

    assert isinstance(scaler, RobustScaler)


def test_get_scaler_standard():
    """get_scaler returns StandardScaler when use_robust=False."""
    scaler = get_scaler(use_robust=False)
    from sklearn.preprocessing import StandardScaler

    assert isinstance(scaler, StandardScaler)


def test_brier_score():
    """brier_score returns mean squared error of predictions."""
    y_true = np.array([1, 0, 1, 0])
    y_prob = np.array([0.9, 0.1, 0.8, 0.2])
    score = brier_score(y_true, y_prob)
    expected = float(np.mean((y_true - y_prob) ** 2))
    assert abs(score - expected) < 1e-10


def test_check_delta_guardrail_pass():
    """check_delta_guardrail passes when delta below threshold."""
    passes, delta = check_delta_guardrail(0.80, 0.78, threshold=0.08)
    assert passes is True
    assert abs(delta - (-0.02)) < 1e-10


def test_check_delta_guardrail_fail():
    """check_delta_guardrail fails when delta exceeds threshold."""
    passes, delta = check_delta_guardrail(0.80, 0.65, threshold=0.08)
    assert passes is False
    assert abs(delta - (-0.15)) < 1e-10


def test_compute_time_decay_weights_ref_date_conversion():
    """compute_time_decay_weights handles reference_date with .date() method (e.g. datetime)."""
    from datetime import datetime

    dates = pd.Series(pd.date_range("2022-01-01", periods=3, freq="D"))
    ref = datetime(2022, 1, 3)
    w = compute_time_decay_weights(dates, reference_date=ref)
    assert w is not None
    assert len(w) == 3


def test_compute_time_decay_weights_ref_date_conversion_raises():
    """compute_time_decay_weights catches reference_date.date() TypeError and continues."""
    class BadRef:
        def date(self):
            raise TypeError("no date")

        def __sub__(self, other):
            # After inner except, ref stays as BadRef; subtraction must work for valid result
            from datetime import date

            return type("Delta", (), {"days": 0})()

    dates = pd.Series(pd.date_range("2022-01-01", periods=3, freq="D"))
    w = compute_time_decay_weights(dates, reference_date=BadRef())
    assert w is not None
    assert len(w) == 3


def test_compute_time_decay_weights_exception_path():
    """compute_time_decay_weights returns None on exception during computation."""
    class BadRef:
        def __sub__(self, other):
            raise ValueError("cannot subtract")

    dates = pd.Series(pd.date_range("2022-01-01", periods=3, freq="D"))
    w = compute_time_decay_weights(dates, reference_date=BadRef())
    assert w is None
