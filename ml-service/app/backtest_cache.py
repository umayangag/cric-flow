"""Backtest response cache for /ml/backtest/predict.

Cache keys are (mode, cutoff_iso, tuple(sorted(ids))):
- mode: "players" or "match"
- cutoff_iso: RFC3339 UTC (e.g. from request cutoff_date). When use_latest_model=True,
  the caller rounds cutoff for key stability (see TRAIN_ON_THE_FLY_LATEST_CACHE_GRANULARITY).
- ids: player IDs (players) or team names (match), sorted for stable keys.

Entries expire after ttl_seconds. When disabled=True or ttl_seconds<=0, get/put are no-ops.
Player and match full-computation counts are tracked for tests and observability.
"""

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Tuple

CacheKey = Tuple[str, str, Tuple[Any, ...]]


@dataclass
class BacktestCache:
    """In-memory cache for backtest endpoints.

    Encapsulates:
    - enable/disable flag and TTL semantics
    - key construction (mode, cutoff_iso, sorted IDs)
    - basic counters for how often full computation runs
    """

    ttl_seconds: int
    disabled: bool = False
    _store: Dict[CacheKey, Tuple[float, Dict[str, Any]]] = field(default_factory=dict)
    _players_compute_count: int = 0
    _match_compute_count: int = 0

    def _make_key(self, mode: str, cutoff_iso: str, ids: List[Any]) -> CacheKey:
        return (mode, cutoff_iso, tuple(sorted(ids)))

    def get(self, mode: str, cutoff_iso: str, ids: List[Any]) -> Optional[Dict[str, Any]]:
        if self.disabled or self.ttl_seconds <= 0:
            return None
        key = self._make_key(mode, cutoff_iso, ids)
        rec = self._store.get(key)
        if not rec:
            return None
        ts, payload = rec
        if (time.time() - ts) > self.ttl_seconds:
            self._store.pop(key, None)
            return None
        return payload

    def put(self, mode: str, cutoff_iso: str, ids: List[Any], payload: Dict[str, Any]) -> None:
        if self.disabled or self.ttl_seconds <= 0:
            return
        key = self._make_key(mode, cutoff_iso, ids)
        self._store[key] = (time.time(), payload)

    def reset(self) -> None:
        self._store.clear()
        self._players_compute_count = 0
        self._match_compute_count = 0

    def increment_players_compute(self) -> None:
        self._players_compute_count += 1

    def increment_match_compute(self) -> None:
        self._match_compute_count += 1

    def get_compute_counts(self) -> Tuple[int, int]:
        return self._players_compute_count, self._match_compute_count
