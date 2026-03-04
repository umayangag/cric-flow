"""Unit tests for BacktestCache behavior (in-memory backtest cache)."""

import time
from typing import Any, Dict, List
from unittest.mock import patch

from app.backtest_cache import BacktestCache


def _make_ids(ids: List[Any]) -> List[Any]:
    # Helper to emphasize that BacktestCache should be order-insensitive for IDs.
    return ids


def test_backtest_cache_put_and_get_with_sorted_ids() -> None:
    """put() followed by get() with the same logical key returns payload regardless of ID order."""
    cache = BacktestCache(ttl_seconds=60, disabled=False)
    payload: Dict[str, Any] = {"ok": True}

    mode = "players"
    cutoff_iso = "2025-02-26T12:00:00Z"
    ids = _make_ids([3, 1, 2])

    cache.put(mode, cutoff_iso, ids, payload)

    # Order of IDs should not matter because BacktestCache sorts internally.
    got = cache.get(mode, cutoff_iso, _make_ids([2, 3, 1]))
    assert got == payload


def test_backtest_cache_disabled_or_zero_ttl_is_noop() -> None:
    """When disabled or ttl_seconds <= 0, cache behaves as a no-op."""
    payload = {"value": 1}
    ids = [1, 2, 3]

    # disabled=True: put/get should not store anything.
    cache_disabled = BacktestCache(ttl_seconds=60, disabled=True)
    cache_disabled.put("players", "2025-02-26T12:00:00Z", ids, payload)
    assert cache_disabled.get("players", "2025-02-26T12:00:00Z", ids) is None

    # ttl_seconds <= 0: put/get should also be no-op.
    cache_zero_ttl = BacktestCache(ttl_seconds=0, disabled=False)
    cache_zero_ttl.put("players", "2025-02-26T12:00:00Z", ids, payload)
    assert cache_zero_ttl.get("players", "2025-02-26T12:00:00Z", ids) is None


def test_backtest_cache_increment_and_reset_compute_counts() -> None:
    """increment_players_compute / increment_match_compute update counts; reset clears them."""
    cache = BacktestCache(ttl_seconds=60, disabled=False)

    cache.increment_players_compute()
    cache.increment_players_compute()
    cache.increment_match_compute()

    players, match = cache.get_compute_counts()
    assert players == 2
    assert match == 1

    cache.reset()
    players_after, match_after = cache.get_compute_counts()
    assert players_after == 0
    assert match_after == 0


def test_backtest_cache_get_expired_returns_none() -> None:
    """get() returns None when entry is past TTL."""
    cache = BacktestCache(ttl_seconds=60, disabled=False)
    cache.put("players", "2025-02-26T12:00:00Z", [1, 2], {"ok": True})
    with patch("app.backtest_cache.time") as mock_time:
        mock_time.time.side_effect = [1000.0, 1070.0]
        cache.put("players", "2025-02-26T12:00:00Z", [1, 2], {"ok": True})
        got = cache.get("players", "2025-02-26T12:00:00Z", [1, 2])
    assert got is None
