"""P-4's simulator choices, made on the walk-forward folds and never on the locked window.

Two questions, each answered by `ml.xi.sim_harness.evaluate_window` (E2's own metrics) per
fold for T20 and ODI:

  chase     which L2-B orientation feeds the chasing side -- its chasing forecasts (the
            innings as a known feature, plus the target truncation) or its bat-first
            forecasts (the truncation alone carries the chase)?
  factor    are simulated first-innings totals under-dispersed without a shared match
            factor (PIT, dispersion ratio, 10-90 coverage), and does the factor -- fitted on
            the temporal calibration fold, deconvolved -- restore coverage without
            over-widening? Both arms use the *same* members (fitted short of the
            calibration fold) so the factor's effect is isolated; the production fit
            without a factor (all rows) is reported beside them.

Every fit is three seeds. Writes one JSON with per-fold rows and fold means; the tables go
into docs/ML_PIPELINE_REARCHITECTURE_PLAN.md (P-4).

    python sim_choices.py --frames frames.pkl --format T20 --out sim_choices_T20.json
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from typing import Any, Dict, List

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from sim_frame_cache import load_frames  # noqa: E402

from ml.xi import contract as C  # noqa: E402
from ml.xi import perf_harness, sim_harness, simulator  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import fold_windows  # noqa: E402
from ml.xi.train import _xy, make_display_model  # noqa: E402

logger = logging.getLogger("sim_choices")

MIN_TRAIN_ROWS = 2000
MIN_EVAL_ROWS = 200
SIM_SAMPLES = 1000


def _display_model(train_matches: pd.DataFrame) -> Any:
    x, y = _xy(train_matches, C.DISPLAY_FEATURE_COLS)
    return make_display_model(C.DISPLAY_FEATURE_COLS).fit(x, y)


def _summary(report: Dict[str, Any]) -> Dict[str, Any]:
    """The few numbers a choice is made on."""
    first = report["totals"]["first_innings"]
    return {
        "n_matches": report["n_matches"],
        "delta_brier": report["win"]["delta_brier_simulated_minus_display"],
        "brier_simulated": report["win"]["brier"]["simulated"],
        "brier_display": report["win"]["brier"]["display"],
        "p_bat_first_wins_simulated": report["win"]["p_bat_first_wins"]["simulated"],
        "p_bat_first_wins_actual": report["win"]["p_bat_first_wins"]["actual"],
        "first_coverage_80": first.get("coverage_80"),
        "first_width_80": first.get("width_80"),
        "first_bias": first.get("bias"),
        "first_dispersion_ratio": first.get("dispersion_ratio"),
        "first_pit_deciles": first.get("pit_deciles"),
        "chase_coverage_80": report["totals"]["chase"].get("coverage_80"),
        "chase_width_80": report["totals"]["chase"].get("width_80"),
        "chase_bias": report["totals"]["chase"].get("bias"),
        "margin_runs_coverage_80": report["margin"]["runs_when_bat_first_wins"].get("coverage_80"),
        "margin_balls_coverage_80": report["margin"]["balls_remaining_when_chaser_wins"].get("coverage_80"),
        "ms_per_fixture": report["latency"]["ms_per_fixture_at_harness_samples"],
        "calibration": report["calibration"],
    }


def run_fold(player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, cutoff, end) -> Dict[str, Any]:
    train, _ = perf_harness.training_rows(player_frame, fmt, cutoff)
    evaluation = player_frame[
        (player_frame.format_code == fmt) & (player_frame.match_date >= cutoff) & (player_frame.match_date < end)
    ]
    matches = match_frame[match_frame.format_code == fmt]
    train_matches, eval_matches = (
        matches[matches.match_date < cutoff],
        matches[(matches.match_date >= cutoff) & (matches.match_date < end)],
    )
    if len(train) < MIN_TRAIN_ROWS or len(evaluation) < MIN_EVAL_ROWS:
        return {"cutoff": cutoff.date().isoformat(), "skipped": True}
    started = time.perf_counter()
    display = _display_model(train_matches)
    base_rate = float(train_matches[C.TARGET_COL].mean())
    fold: Dict[str, Any] = {"cutoff": cutoff.date().isoformat(), "n_eval_matches": int(len(eval_matches))}

    def measure(model: P.PerformanceModels) -> Dict[str, Any]:
        return _summary(
            sim_harness.evaluate_window(model, display, eval_matches, evaluation, fmt, base_rate, SIM_SAMPLES)
        )

    # Production fit (all rows, no factor): the chase-orientation question.
    production = P.fit_performance(train, fmt, P.default_spec(shared_factor=False), train_matches)
    for orientation in ("chasing", "bat_first"):
        simulator.CHASE_ORIENTATION = orientation
        fold[f"production_{orientation}"] = measure(production)
    simulator.CHASE_ORIENTATION = "chasing"
    # Calibration-fold fit: the same members with and without the shared factor.
    with_factor = P.fit_performance(train, fmt, P.default_spec(shared_factor=True), train_matches)
    fold["calibration_fit_with_factor"] = measure(with_factor)
    with_factor.simulation = simulator.SimulatorCalibration(with_factor.simulation.runs_balls_rho, None)
    fold["calibration_fit_without_factor"] = measure(with_factor)
    fold["seconds"] = round(time.perf_counter() - started, 1)
    logger.info(
        "%s @ %s: production coverage %.3f (ratio %.2f) | factor off %.3f (%.2f) -> on %.3f (%.2f, width %.1f -> %.1f); "
        "Δ Brier %+.4f; %.0f s",
        fmt,
        fold["cutoff"],
        fold["production_chasing"]["first_coverage_80"],
        fold["production_chasing"]["first_dispersion_ratio"],
        fold["calibration_fit_without_factor"]["first_coverage_80"],
        fold["calibration_fit_without_factor"]["first_dispersion_ratio"],
        fold["calibration_fit_with_factor"]["first_coverage_80"],
        fold["calibration_fit_with_factor"]["first_dispersion_ratio"],
        fold["calibration_fit_without_factor"]["first_width_80"],
        fold["calibration_fit_with_factor"]["first_width_80"],
        fold["production_chasing"]["delta_brier"],
        fold["seconds"],
    )
    return fold


def fold_means(folds: List[Dict[str, Any]]) -> Dict[str, Dict[str, Any]]:
    scored = [f for f in folds if not f.get("skipped")]
    out: Dict[str, Dict[str, Any]] = {}
    for variant in (
        "production_chasing",
        "production_bat_first",
        "calibration_fit_without_factor",
        "calibration_fit_with_factor",
    ):
        out[variant] = {}
        for key in scored[0][variant]:
            values = [f[variant][key] for f in scored if isinstance(f[variant][key], (int, float))]
            if values:
                out[variant][key] = {
                    "mean": float(np.mean(values)),
                    "sd": float(np.std(values)),
                    "n_folds": len(values),
                }
        deciles = [f[variant]["first_pit_deciles"] for f in scored if f[variant]["first_pit_deciles"]]
        if deciles:
            out[variant]["first_pit_deciles_mean"] = [float(x) for x in np.mean(deciles, axis=0)]
    return out


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--frames", required=True, help="pickle from sim_frame_cache.py")
    p.add_argument("--format", required=True, choices=list(simulator.SIMULATED_FORMATS))
    p.add_argument("--out", required=True)
    args = p.parse_args()
    player_frame, match_frame = load_frames(None, args.frames)
    folds = [run_fold(player_frame, match_frame, args.format, cutoff, end) for cutoff, end in fold_windows()]
    result = {
        "format": args.format,
        "seeds": list(P.DEFAULT_SEEDS),
        "sim_samples": SIM_SAMPLES,
        "folds": folds,
        "means": fold_means(folds),
    }
    with open(args.out, "w") as fh:
        json.dump(result, fh, indent=2)
    logger.info("written %s", args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
