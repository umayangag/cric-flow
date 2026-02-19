"""
Integration tests: train_on_the_fly with mocked go-app HTTP response.

Exercises the full path: fetch (mocked) -> _batting_rows_to_xy / _bowling_rows_to_xy
-> _train_batting_in_memory / _train_bowling_in_memory -> return (scaler, model).
Uses the same header/row shape as unit tests (BATTING_FEATURE_COLS / BOWLING_FEATURE_COLS).
"""

from unittest.mock import patch

import pytest

from app.train_on_the_fly import train_on_the_fly

# Reuse exact header/row builders from unit tests so feature columns match
from tests.test_train_on_the_fly import (
    _batting_headers,
    _bowling_headers,
    _one_batting_row,
    _one_bowling_row,
)


@pytest.mark.integration
def test_train_on_the_fly_full_pipeline_returns_models():
    """Mock fetch returns batting+bowling data; train_on_the_fly returns two (scaler, model) pairs."""
    bat_headers = _batting_headers()
    bowl_headers = _bowling_headers()
    data = {
        "batting": {"headers": bat_headers, "rows": [_one_batting_row(), _one_batting_row()]},
        "bowling": {"headers": bowl_headers, "rows": [_one_bowling_row(), _one_bowling_row()]},
    }

    with patch("app.train_on_the_fly.fetch_training_data", return_value=data):
        (scaler_bat, model_bat), (scaler_bowl, model_bowl) = train_on_the_fly(
            "http://goapp", "T20", "2024-10-30T00:00:00Z"
        )

    assert scaler_bat is not None and model_bat is not None
    assert scaler_bowl is not None and model_bowl is not None
    assert hasattr(model_bat, "predict")
    assert hasattr(model_bowl, "predict")


@pytest.mark.integration
def test_train_on_the_fly_cached_integration(tmp_path, monkeypatch):
    """train_on_the_fly_cached with cache dir: first call fetches, second can use cache (if implemented)."""
    from app.train_on_the_fly import train_on_the_fly_cached

    bat_headers = _batting_headers()
    bowl_headers = _bowling_headers()
    data = {
        "batting": {"headers": bat_headers, "rows": [_one_batting_row()] * 5},
        "bowling": {"headers": bowl_headers, "rows": [_one_bowling_row()] * 5},
    }

    monkeypatch.setenv("ML_SERVICE_CACHE_DIR", str(tmp_path / "cache"))
    with patch("app.train_on_the_fly.fetch_training_data", return_value=data):
        result = train_on_the_fly_cached("http://goapp", "T20", "2024-10-30T00:00:00Z")

    assert result is not None
    (s_bat, m_bat), (s_bowl, m_bowl) = result
    assert m_bat is not None and m_bowl is not None
