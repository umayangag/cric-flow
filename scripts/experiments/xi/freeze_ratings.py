"""Rewrite ``xi_ratings.joblib`` with a rating state that stops at a date.

Why this exists. The serving rating state is "ratings through today", which is what a
real prediction wants and what ``ml.xi.train`` saves. A *backtest* over already-played
matches wants the opposite: the state as it stood before each match. The S-3b selection
comparison (``POST /api/backtest/selection-comparison``) has no as-of hook, so scoring the
XI arm against the serving artifact would let its ratings — team Elo above all — carry the
results of the very matches being scored.

Freezing the state at the training cutoff removes that channel. It errs the other way:
ratings are stale by up to the width of the window, so the XI arm is handicapped rather
than flattered, and a comparison it wins is a lower bound. The proper fix is an as-of
serving path in the L4 harness (ML_PIPELINE_REARCHITECTURE_PLAN.md, P-2); this is the
one-off that lets P-0 report an honest number in the meantime.

Usage (from ml-service/, with POSTGRES_* set):

    python ../scripts/experiments/xi/freeze_ratings.py --before 2025-09-01 \
        --out ../output/ml-service

The models in that directory are left alone: only the ratings artifact is replaced. Keep a
copy of the full-history artifact if the directory is also serving predictions.
"""

from __future__ import annotations

import argparse
import logging
from datetime import date

from ml.db import get_db_connection
from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.sources import PostgresSource
from ml.xi.store import save_ratings

logger = logging.getLogger("freeze_ratings")


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--before", required=True, help="YYYY-MM-DD; only matches strictly before it are folded in")
    parser.add_argument("--out", required=True, help="artifacts directory to write xi_ratings.joblib into")
    parser.add_argument("--formats", nargs="+", default=list(C.FORMAT_CODES))
    args = parser.parse_args()

    source = PostgresSource(get_db_connection(), args.formats, before=date.fromisoformat(args.before))
    result = build(source, progress=lambda i: logger.info("rating pass: %d matches", i))
    save_ratings(result.state, args.out)
    logger.info("frozen state: %d players, through %s", len(result.state.players), result.state.last_date)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
