"""L4: the one temporal evaluation harness (H-19).

Rolling-origin walk-forward for every number a choice may be based on, plus the locked
window -- matches at or after ``LOCKED_START`` -- scored once per release and labeled as
such, never used for a choice. Per format it reports, with mean and spread over cutoffs
(and seeds, where a model has one):

* win-model objective and display AUC / Brier against the base rate;
* the specific-XI-beyond-typical-XI delta and swap monotonicity (the selection gates
  that replace P-0's winner accuracy);
* the best-single-column leak canary with the TEST-format control (H-2);
* performance baselines from the player-match rows -- within-match Spearman, top-3 hit,
  per-target MAE for the career-mean and rating-expectation predictors -- with interval
  width and coverage columns that stay empty until P-3 (H-22);
* the train/serve parity check (H-8): the last ``PARITY_LAST_N`` matches rebuilt from the
  as-of serving path and compared with the training frame.

One command, one JSON report:

  python -m ml.xi.evaluate --cricsheet-dir <dir>     # offline reproduction
  python -m ml.xi.evaluate --postgres                # the go-app database
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
from datetime import datetime, timezone
from typing import Callable, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi import perf_baselines, selection_metrics
from ml.xi.asof import serving_parity
from ml.xi.builder import build
from ml.xi.sources import MatchSource
from ml.xi.train import _score_marginalised, _xy, make_display_model, make_objective_model

logger = logging.getLogger(__name__)

REPORT_NAME = "xi_evaluate_report.json"

# Rolling origins for every choice-facing number. Each fold trains on rows strictly before
# its cutoff and scores the window up to the next cutoff; the last window ends where the
# locked window begins.
WALK_FORWARD_CUTOFFS: List[str] = [
    "2024-01-01",
    "2024-04-01",
    "2024-07-01",
    "2024-10-01",
    "2025-01-01",
    "2025-04-01",
    "2025-06-01",
]
# Scored once per release, labeled, never used for a choice (H-19).
LOCKED_START = "2025-09-01"
DISPLAY_SEEDS: Tuple[int, ...] = (0, 1, 2)
PARITY_LAST_N = 50
# How many evaluation matches feed the swap-monotonicity probe per fold.
SWAP_MAX_MATCHES = 50

MIN_TRAIN_ROWS = 50
MIN_EVAL_ROWS = 20


def fold_windows() -> List[Tuple[pd.Timestamp, pd.Timestamp]]:
    boundaries = [pd.Timestamp(c) for c in WALK_FORWARD_CUTOFFS] + [pd.Timestamp(LOCKED_START)]
    return list(zip(boundaries[:-1], boundaries[1:]))


def _stats(values: Sequence[Optional[float]]) -> Optional[Dict]:
    """Mean and spread over folds, ignoring folds that could not produce the number."""
    present = [v for v in values if v is not None]
    if not present:
        return None
    return {"mean": float(np.mean(present)), "sd": float(np.std(present)), "n_folds": len(present)}


def _evaluate_win_window(
    format_frame: pd.DataFrame, cutoff: pd.Timestamp, end: pd.Timestamp
) -> Tuple[Optional[Dict], Optional[object]]:
    """Win-model metrics for one (train < cutoff, eval [cutoff, end)) split; also returns
    the fitted objective model for the selection metrics."""
    train = format_frame[format_frame.match_date < cutoff]
    evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
    skip = {
        "cutoff": cutoff.date().isoformat(),
        "end": end.date().isoformat(),
        "n_train": int(len(train)),
        "n_eval": int(len(evaluation)),
    }
    if len(train) < MIN_TRAIN_ROWS or train[C.TARGET_COL].nunique() < 2:
        skip["skipped_reason"] = "insufficient training rows"
        return skip, None
    if len(evaluation) < MIN_EVAL_ROWS or evaluation[C.TARGET_COL].nunique() < 2:
        skip["skipped_reason"] = "evaluation window too small or single-class"
        return skip, None
    x_objective, y_train = _xy(train, C.XI_FEATURE_COLS)
    x_display, _ = _xy(train, C.DISPLAY_FEATURE_COLS)
    objective = make_objective_model().fit(x_objective, y_train)
    displays = [make_display_model(C.DISPLAY_FEATURE_COLS, seed).fit(x_display, y_train) for seed in DISPLAY_SEEDS]
    objective_scores = _score_marginalised(objective, evaluation, C.XI_FEATURE_COLS)
    display_scores = [_score_marginalised(m, evaluation, C.DISPLAY_FEATURE_COLS) for m in displays]
    y_eval = evaluation[C.TARGET_COL].to_numpy(dtype=float)
    base_rate_brier = float(np.mean((y_eval - y_train.mean()) ** 2))
    metrics = dict(skip)
    metrics.update(
        {
            "eval_positive_rate": float(y_eval.mean()),
            "objective_auc": objective_scores["auc"],
            "objective_brier": objective_scores["brier"],
            "display_auc_mean": float(np.mean([s["auc"] for s in display_scores])),
            "display_auc_seed_sd": float(np.std([s["auc"] for s in display_scores])),
            "display_brier_mean": float(np.mean([s["brier"] for s in display_scores])),
            "base_rate_brier": base_rate_brier,
        }
    )
    return metrics, objective


def _evaluate_fold(
    format_code: str,
    format_frame: pd.DataFrame,
    format_players: pd.DataFrame,
    cutoff: pd.Timestamp,
    end: pd.Timestamp,
) -> Dict:
    fold, objective = _evaluate_win_window(format_frame, cutoff, end)
    if objective is None:
        return fold
    window_players = format_players[(format_players.match_date >= cutoff) & (format_players.match_date < end)]
    fold["swap_monotonicity"] = selection_metrics.swap_monotonicity(
        objective, C.XI_FEATURE_COLS, window_players, format_code, max_matches=SWAP_MAX_MATCHES
    )
    fold["specific_vs_typical"] = selection_metrics.specific_vs_typical(
        objective, C.XI_FEATURE_COLS, format_frame, cutoff, end
    )
    fold["performance"] = perf_baselines.score_window(window_players)
    return fold


def _summarize_folds(folds: List[Dict]) -> Dict:
    scored = [f for f in folds if "objective_auc" in f]

    def over_folds(path: Callable[[Dict], Optional[float]]) -> Optional[Dict]:
        return _stats([path(f) for f in scored])

    def nested(fold: Dict, *keys: str) -> Optional[float]:
        node = fold
        for key in keys:
            if node is None:
                return None
            node = node.get(key)
        return node

    summary = {
        "objective_auc": over_folds(lambda f: f["objective_auc"]),
        "objective_brier": over_folds(lambda f: f["objective_brier"]),
        "display_auc": over_folds(lambda f: f["display_auc_mean"]),
        "display_auc_seed_sd_mean": over_folds(lambda f: f["display_auc_seed_sd"]),
        "base_rate_brier": over_folds(lambda f: f["base_rate_brier"]),
        "swap_violation_share": over_folds(lambda f: nested(f, "swap_monotonicity", "violation_share")),
        "specific_vs_typical_delta": over_folds(lambda f: nested(f, "specific_vs_typical", "delta")),
        "performance": {},
    }
    for target in perf_baselines.TARGETS:
        summary["performance"][target] = {
            predictor: {
                metric: over_folds(lambda f, p=predictor, m=metric, t=target: nested(f, "performance", t, p, m))
                for metric in ("mae", "within_match_spearman", "top3_hit_rate")
            }
            for predictor in ("career_mean", "rating_expectation")
        }
    return summary


def evaluate_format(format_code: str, frame: pd.DataFrame, player_frame: pd.DataFrame) -> Dict:
    format_frame = frame[frame.format_code == format_code]
    format_players = player_frame[player_frame.format_code == format_code]
    folds = [_evaluate_fold(format_code, format_frame, format_players, cutoff, end) for cutoff, end in fold_windows()]
    locked = _evaluate_fold(format_code, format_frame, format_players, pd.Timestamp(LOCKED_START), pd.Timestamp.max)
    locked["note"] = "locked window (H-19): scored once per release, never used for a choice"
    return {
        "n_matches": int(len(format_frame)),
        "walk_forward": {"folds": folds, "summary": _summarize_folds(folds)},
        "locked": locked,
    }


def evaluate(
    source: MatchSource,
    parity_source_factory: Callable[[], MatchSource],
    gender_split_context: bool = False,
) -> Dict:
    """Run the harness over a source and return the report dict."""
    result = build(
        source,
        progress=lambda i: logger.info("rating pass: %d matches", i),
        gender_split_context=gender_split_context,
    )
    player_frame = perf_baselines.add_baseline_predictors(result.player_frame)
    dev_start, dev_end = pd.Timestamp(WALK_FORWARD_CUTOFFS[0]), pd.Timestamp(LOCKED_START)
    report = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "source": type(source).__name__,
        "cutoffs": list(WALK_FORWARD_CUTOFFS),
        "locked_start": LOCKED_START,
        "seeds": list(DISPLAY_SEEDS),
        "gender_split_context": gender_split_context,
        "n_rows": int(len(result.frame)),
        "n_player_rows": int(len(result.player_frame)),
        "data_quality": result.quality.as_dict(),
        "leak_canary": selection_metrics.leak_canary(result.frame, dev_start, dev_end),
        "formats": {},
    }
    for format_code in C.FORMAT_CODES:
        logger.info("evaluating %s", format_code)
        report["formats"][format_code] = evaluate_format(format_code, result.frame, player_frame)
    logger.info("serving parity (H-8): rebuilding the last %d matches from the as-of path", PARITY_LAST_N)
    report["serving_parity"] = serving_parity(
        parity_source_factory(),
        result.frame,
        result.player_frame,
        last_n=PARITY_LAST_N,
        gender_split_context=gender_split_context,
    )
    return report


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    src = p.add_mutually_exclusive_group(required=True)
    src.add_argument("--cricsheet-dir", help="directory of Cricsheet JSON files")
    src.add_argument("--postgres", action="store_true", help="read the go-app database (POSTGRES_* env vars)")
    p.add_argument("--out", default=None, help="report directory (default: ml.config.default_artifacts_dir())")
    p.add_argument(
        "--gender-split-context",
        action="store_true",
        help="E7 (H-7): split the context baselines (runs/wickets per format x over) by gender",
    )
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    if args.cricsheet_dir:
        from ml.xi.sources import CricsheetJsonSource
        from ml.xi.train import _international_teams_from_config

        international_teams = _international_teams_from_config()

        def source_factory() -> MatchSource:
            return CricsheetJsonSource(args.cricsheet_dir, international_teams)

    else:
        from ml.db import get_db_connection
        from ml.xi.sources import PostgresSource

        connection = get_db_connection()

        def source_factory() -> MatchSource:
            return PostgresSource(connection)

    out_dir = args.out
    if out_dir is None:
        from ml.config import default_artifacts_dir

        out_dir = default_artifacts_dir()
    report = evaluate(source_factory(), source_factory, gender_split_context=args.gender_split_context)
    os.makedirs(out_dir, exist_ok=True)
    path = os.path.join(out_dir, REPORT_NAME)
    with open(path, "w") as fh:
        json.dump(report, fh, indent=2)
    logger.info("report written to %s", path)
    for format_code, entry in report["formats"].items():
        summary = entry["walk_forward"]["summary"]
        objective = summary.get("objective_auc")
        if objective:
            logger.info(
                "%-5s walk-forward objective AUC %.3f ± %.3f over %d folds",
                format_code,
                objective["mean"],
                objective["sd"],
                objective["n_folds"],
            )
    if not report["serving_parity"]["passed"]:
        logger.error("serving parity (H-8) FAILED: %s", report["serving_parity"]["mismatches"][:5])
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
