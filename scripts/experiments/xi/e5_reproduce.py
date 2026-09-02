"""E5, lineup-only, reproduced through the harness's own code (plan §8.6, §8.8).

`e5_natural_experiment.py` produced §8.6's figures over HTTP against the serving artifact:
every consecutive pair of one club's matches with 1-3 lineup changes, both elevens scored
against the later match's opponent at that match's as-of ratings, sign agreement with the
result change. This script is the wiring test for `ml.xi.natural_experiment` -- the same
pairs, the same as-of serving path (`ml.xi.asof.AsOfRatings`) and one run's objective
model, so it must land on the same numbers: T20 0.509, ODI 0.521, T20I 0.589 on the
development pairs, before the locked window.

What it is not: the walk-forward figure. A run's objective is fitted on rows through its
cutoff, so scoring pairs before that cutoff with it is in-sample for the model's weights
(the ratings are as-of either way). `make evaluate` scores each pair with the objective of
the fold whose window holds it; those are the choice-facing numbers, and this script exists
to show the two computations agree where they overlap in method.

    python scripts/experiments/xi/e5_reproduce.py --run output/ml-service/runs/<id>
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from typing import Dict, List

import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from ml.xi import natural_experiment as ne  # noqa: E402
from ml.xi.builder import build  # noqa: E402
from ml.xi.store import XiStore  # noqa: E402

logger = logging.getLogger("e5_reproduce")

REFERENCE = {"T20": 0.509, "ODI": 0.521, "T20I": 0.589}  # §8.6, development pairs, lineup-only
# §8.6's development set is every pair before the locked window as it stood at P-7. The
# harness's LOCKED_START rotates (A-4) and this reproduction must not, or it would compare
# a different set of pairs against a fixed reference.
DEVELOPMENT_END = "2025-09-01"


def _source_factory(cricsheet_dir: str | None):
    if cricsheet_dir:
        from ml.xi.sources import CricsheetJsonSource
        from ml.xi.train import _international_teams_from_config

        teams = _international_teams_from_config()
        return lambda: CricsheetJsonSource(cricsheet_dir, teams)
    from ml.db import get_db_connection
    from ml.xi.sources import PostgresSource

    connection = get_db_connection()
    return lambda: PostgresSource(connection)


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--run", required=True, help="a run directory holding xi_win_<FORMAT>.joblib and manifest.json")
    p.add_argument("--cricsheet-dir", default=None, help="read the raw archive instead of the database")
    p.add_argument("--out", default="output/e5_reproduce.json")
    args = p.parse_args()

    factory = _source_factory(args.cricsheet_dir)
    started = time.perf_counter()
    result = build(factory(), progress=lambda i: logger.info("rating pass: %d matches", i))
    pairs = ne.build_pairs(result.frame, result.player_frame)
    previous = ne.score_previous_elevens(pairs, factory())
    logger.info("frame, pairs and previous elevens in %.0f s", time.perf_counter() - started)

    store = XiStore.load(args.run)
    locked_start = pd.Timestamp(DEVELOPMENT_END)
    report: Dict[str, Dict] = {"run": store.manifest.run_id, "previous_elevens": previous, "formats": {}}
    for format_code in REFERENCE:
        models = store.models[format_code]
        own: List[ne.LineupPair] = [pr for pr in pairs if pr.format_code == format_code]
        development = [pr for pr in own if pr.after_date < locked_start]
        locked = [pr for pr in own if pr.after_date >= locked_start]
        sections = {}
        for label, group in (("development", development), ("locked_window", locked)):
            scored = ne.score_pairs(group, models.objective_proba, models.objective_cols)
            sections[label] = {
                "pairs": len(group),
                **ne.agreement(scored.d_lineup, scored.d_result),
                "effect_size": ne.effect_size(scored.d_lineup),
                "derived_bar": ne.derived_bar(scored),
            }
        observed = sections["development"]["agreement"]
        report["formats"][format_code] = {
            **sections,
            "reference_8_6": REFERENCE[format_code],
            "reproduces_8_6": observed is not None and abs(observed - REFERENCE[format_code]) < 0.0015,
        }
        logger.info(
            "%-5s development pairs %d, lineup-only %.3f ± %.3f (n=%d) vs §8.6 %.3f; derived bar %.3f "
            "(exactly right %.3f); locked %.3f (n=%d)",
            format_code,
            len(development),
            observed or float("nan"),
            sections["development"]["standard_error"] or float("nan"),
            sections["development"]["pairs_scored"],
            REFERENCE[format_code],
            sections["development"]["derived_bar"].get("bar") or float("nan"),
            sections["development"]["derived_bar"].get("expected_if_exactly_right") or float("nan"),
            sections["locked_window"]["agreement"] or float("nan"),
            sections["locked_window"]["pairs_scored"],
        )
    os.makedirs(os.path.dirname(args.out) or ".", exist_ok=True)
    with open(args.out, "w") as fh:
        json.dump(report, fh, indent=2)
    logger.info("written to %s", args.out)
    return 0 if all(f["reproduces_8_6"] for f in report["formats"].values()) else 1


if __name__ == "__main__":
    sys.exit(main())
