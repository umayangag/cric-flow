"""Build and cache the rating pass's two frames (player rows and win rows) once, so the
simulator experiments (``sim_choices.py``) and re-runs do not pay the pass each time.

    python sim_frame_cache.py --cricsheet-dir ../../../data/go-app/cricsheet --out frames.pkl

X-1b's runs pass the archive's dates of birth (``--birth-dates``, the CSV
``python -m ml.xi.biography --export`` writes) and, for family 3, ``--age-aware-cold-start``,
which runs the pass with the age-band debut prior on. A cache written with either also
carries the end-of-pass debut tables, which the H-10 probe reads through
``load_debut_tables``; ``load_frames`` returns the two frames from either cache format.
"""

from __future__ import annotations

import argparse
import logging
import os
import sys
import time

import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from ml.xi import perf_baselines  # noqa: E402
from ml.xi.builder import build  # noqa: E402
from ml.xi.sources import CricsheetJsonSource  # noqa: E402

logger = logging.getLogger("sim_frame_cache")


def load_frames(
    cricsheet_dir: str | None,
    cache: str | None,
    birth_dates: str | None = None,
    age_aware_cold_start: bool = False,
) -> tuple[pd.DataFrame, pd.DataFrame]:
    """(player_frame with baseline predictors, win frame), from the cache when it exists."""
    if cache and os.path.exists(cache):
        logger.info("frames from cache %s", cache)
        loaded = pd.read_pickle(cache)
        return loaded["frames"] if isinstance(loaded, dict) else loaded
    started = time.perf_counter()
    result = build(
        CricsheetJsonSource(cricsheet_dir, birth_dates_path=birth_dates),
        progress=lambda i: logger.info("rating pass: %d matches", i),
        age_aware_cold_start=age_aware_cold_start,
    )
    frames = (perf_baselines.add_baseline_predictors(result.player_frame), result.frame)
    logger.info("pass done in %.0f s: %d player rows, %d matches", time.perf_counter() - started, *map(len, frames))
    if cache:
        pd.to_pickle(
            {
                "frames": frames,
                "debut_tables": {"debut_bat": result.state.debut_bat, "debut_bowl": result.state.debut_bowl},
                "options": {"birth_dates": birth_dates, "age_aware_cold_start": age_aware_cold_start},
                "quality": result.quality.as_dict(),
            },
            cache,
        )
    return frames


def load_debut_tables(cache: str) -> dict:
    """The end-of-pass age-band debut tables a cache written with birth dates carries."""
    loaded = pd.read_pickle(cache)
    if not isinstance(loaded, dict):
        raise ValueError(f"{cache} was written without debut tables; rebuild it with --birth-dates")
    return loaded["debut_tables"]


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--cricsheet-dir", required=True)
    p.add_argument("--out", required=True)
    p.add_argument("--birth-dates", default=None, help="CSV of player_key,birth_date (X-1b)")
    p.add_argument("--age-aware-cold-start", action="store_true", help="run the pass with the debut prior on")
    args = p.parse_args()
    load_frames(args.cricsheet_dir, args.out, args.birth_dates, args.age_aware_cold_start)
