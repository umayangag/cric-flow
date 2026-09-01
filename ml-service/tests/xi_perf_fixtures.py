"""Shared setup for the performance-model tests: a small synthetic player frame and a
fast fit configuration (one seed, few iterations) that the module-scoped fixtures apply
and undo themselves, since monkeypatch is function-scoped."""

from __future__ import annotations

from contextlib import contextmanager
from typing import Iterator

import pandas as pd

from ml.xi import perf_baselines
from ml.xi import performance as P
from ml.xi.builder import build
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history

FAST_MAX_ITER = 40


@contextmanager
def fast_fits() -> Iterator[None]:
    """One seed and a short boosting run: enough to exercise every path, not to be good."""
    original = (P.DEFAULT_SEEDS, P.MAX_ITER)
    P.DEFAULT_SEEDS, P.MAX_ITER = (0,), FAST_MAX_ITER
    try:
        yield
    finally:
        P.DEFAULT_SEEDS, P.MAX_ITER = original


def synthetic_player_frame(n_matches: int = 160) -> pd.DataFrame:
    matches, _, _ = _synthetic_history(n_matches)
    return perf_baselines.add_baseline_predictors(build(_ListSource(matches)).player_frame)
