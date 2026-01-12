import os
from datetime import datetime
from typing import List

import numpy as np

from .models import BacktestMatchAgg, BacktestPlayerPred


def resolve_model_version(app_version_fallback: str) -> str:
    """Return a model version string for responses.

    Preference order:
    1) ENV MODEL_VERSION
    2) Provided FastAPI app.version fallback
    """
    mv = os.environ.get("MODEL_VERSION", "").strip()
    if mv:
        return mv
    return app_version_fallback or "unknown"


def _deterministic_rng_seed(*parts: str) -> int:
    """Build a stable 32-bit seed from text parts.

    Simple non-cryptographic hash to keep baseline predictions deterministic.
    """
    acc = 0x345678
    for p in parts:
        for ch in str(p):
            acc = (acc * 1000003) ^ ord(ch)
        acc &= 0xFFFFFFFF
    return acc or 42


def predict_players_baseline(cutoff: datetime, player_ids: List[int]) -> List[BacktestPlayerPred]:
    """Deterministic simple baseline for player stats used in backtests.

    Pseudo-random but stable per (cutoff, player_id).
    """
    out: List[BacktestPlayerPred] = []
    for pid in player_ids:
        seed = _deterministic_rng_seed(cutoff.isoformat(), str(pid))
        rng = np.random.default_rng(seed)
        runs = float(max(0.0, np.round(rng.normal(20.0, 12.0))))
        wickets = float(max(0.0, np.round(rng.uniform(0.0, 3.0), 1)))
        economy = float(np.round(5.0 + rng.random() * 5.0, 1))
        catches = float(rng.integers(0, 4))
        run_outs = float(rng.integers(0, 3))
        out.append(
            BacktestPlayerPred(
                player_id=int(pid),
                runs=runs,
                wickets=wickets,
                economy=economy,
                catches=catches,
                run_outs=run_outs,
            )
        )
    return out


def predict_match_baseline(cutoff: datetime, teams: List[str]) -> BacktestMatchAgg:
    """Deterministic simple baseline aggregate for a match used in backtests."""
    a, b = teams[0].upper(), teams[1].upper()
    seed = _deterministic_rng_seed(cutoff.isoformat(), a, b)
    rng = np.random.default_rng(seed)
    # Aggregate team runs: sample a plausible total and enforce a sensible lower bound
    base_runs = int(np.clip(np.round(rng.uniform(120, 190)), 50, 400))
    runs = float(base_runs)
    wickets = float(np.clip(np.round(rng.uniform(4, 8)), 2, 10))
    extras = float(int(np.clip(np.round(rng.uniform(5, 15)), 0, 25)))
    w_seed_a = _deterministic_rng_seed(a)
    w_seed_b = _deterministic_rng_seed(b)
    winner = a if (w_seed_a ^ seed) >= (w_seed_b ^ seed) else b
    return BacktestMatchAgg(runs=runs, wickets=wickets, extras=extras, winner_team_code=winner)
