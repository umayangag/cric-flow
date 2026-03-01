"""Tests for main module helpers (e.g. _round_datetime_to_granularity for cache-key stability)."""

from datetime import datetime, timezone

from app.main import (
    _round_datetime_to_granularity,
    get_backtest_compute_counts,
    reset_backtest_cache,
)


def test_round_datetime_to_granularity_none_returns_unchanged():
    """Granularity 'none' or empty returns dt unchanged (exact now for cache key)."""
    dt = datetime(2025, 2, 26, 14, 32, 11, 123456, tzinfo=timezone.utc)
    assert _round_datetime_to_granularity(dt, "none") == dt
    assert _round_datetime_to_granularity(dt, "") == dt
    assert _round_datetime_to_granularity(dt, "NONE") == dt


def test_round_datetime_to_granularity_second():
    """Second granularity zeroes microseconds."""
    dt = datetime(2025, 2, 26, 14, 32, 11, 123456, tzinfo=timezone.utc)
    got = _round_datetime_to_granularity(dt, "second")
    assert got == datetime(2025, 2, 26, 14, 32, 11, 0, tzinfo=timezone.utc)


def test_round_datetime_to_granularity_minute():
    """Minute granularity zeroes seconds and microseconds."""
    dt = datetime(2025, 2, 26, 14, 32, 11, 123456, tzinfo=timezone.utc)
    got = _round_datetime_to_granularity(dt, "minute")
    assert got == datetime(2025, 2, 26, 14, 32, 0, 0, tzinfo=timezone.utc)


def test_round_datetime_to_granularity_hour():
    """Hour granularity zeroes minutes, seconds, microseconds (DoS mitigation default)."""
    dt = datetime(2025, 2, 26, 14, 32, 11, 123456, tzinfo=timezone.utc)
    got = _round_datetime_to_granularity(dt, "hour")
    assert got == datetime(2025, 2, 26, 14, 0, 0, 0, tzinfo=timezone.utc)


def test_round_datetime_to_granularity_day():
    """Day granularity zeroes to midnight UTC."""
    dt = datetime(2025, 2, 26, 14, 32, 11, 123456, tzinfo=timezone.utc)
    got = _round_datetime_to_granularity(dt, "day")
    assert got == datetime(2025, 2, 26, 0, 0, 0, 0, tzinfo=timezone.utc)


def test_round_datetime_to_granularity_unknown_fallback():
    """Unknown granularity falls back to no rounding."""
    dt = datetime(2025, 2, 26, 14, 32, 11, 123456, tzinfo=timezone.utc)
    assert _round_datetime_to_granularity(dt, "invalid") == dt


def test_reset_backtest_cache_and_get_compute_counts():
    """reset_backtest_cache clears cache; get_backtest_compute_counts returns (players, match) counts."""
    reset_backtest_cache()
    players, match = get_backtest_compute_counts()
    assert players == 0
    assert match == 0
