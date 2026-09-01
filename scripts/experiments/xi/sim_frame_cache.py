"""Build and cache the rating pass's two frames (player rows and win rows) once, so the
simulator experiments (``sim_choices.py``) and re-runs do not pay the pass each time.

    python sim_frame_cache.py --cricsheet-dir ../../../data/go-app/cricsheet --out frames.pkl
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
from ml.xi.train import _international_teams_from_config  # noqa: E402

logger = logging.getLogger("sim_frame_cache")


def load_frames(cricsheet_dir: str | None, cache: str | None) -> tuple[pd.DataFrame, pd.DataFrame]:
    """(player_frame with baseline predictors, win frame), from the cache when it exists."""
    if cache and os.path.exists(cache):
        logger.info("frames from cache %s", cache)
        return pd.read_pickle(cache)
    started = time.perf_counter()
    result = build(
        CricsheetJsonSource(cricsheet_dir, _international_teams_from_config()),
        progress=lambda i: logger.info("rating pass: %d matches", i),
    )
    frames = (perf_baselines.add_baseline_predictors(result.player_frame), result.frame)
    logger.info("pass done in %.0f s: %d player rows, %d matches", time.perf_counter() - started, *map(len, frames))
    if cache:
        pd.to_pickle(frames, cache)
    return frames


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--cricsheet-dir", required=True)
    p.add_argument("--out", required=True)
    args = p.parse_args()
    load_frames(args.cricsheet_dir, args.out)
