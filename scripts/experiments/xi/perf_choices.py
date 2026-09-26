"""P-3's modelling choices, made on the walk-forward folds and never on the locked window.

Four questions, answered in order because each uses the previous answer:

  grid       the hyperparameter point (``performance.HYPERPARAMETER_GRID``) by pinball loss
  structure  direct quantile / Poisson vs two-part (P(involved) x conditional), by pinball
  sequence   E1: each sequence family added to the base inputs; keep if pinball moves > 1 %
  transfer   E6: T20 + T20I joint with a format indicator vs separate; keep separate unless
             joint wins by > 0.01 within-match Spearman

Every fit is three seeds unless the stage says otherwise; T20 and ODI are the formats with
enough folds to resolve anything, and E6 scores T20I as well. Runs and wickets -- the
acceptance targets -- are the targets fitted; the tables go into
docs/ML_PIPELINE_REARCHITECTURE_PLAN.md (P-3, §5).

    python perf_choices.py --cricsheet-dir ../../../data/go-app/cricsheet --out choices.json
    python perf_choices.py --cache frame.pkl --out choices.json          # reuse the frame
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from typing import Dict, List, Optional, Sequence

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from ml.xi import contract as C  # noqa: E402
from ml.xi import perf_baselines, perf_harness  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import fold_windows  # noqa: E402

logger = logging.getLogger("perf_choices")

FORMATS = ("T20", "ODI")
TARGETS = ("runs", "wickets")
SEEDS = (0, 1, 2)
MIN_TRAIN_ROWS = 2000
MIN_EVAL_ROWS = 200


def load_frame(cricsheet_dir: Optional[str], cache: Optional[str]) -> pd.DataFrame:
    if cache and os.path.exists(cache):
        logger.info("frame from cache %s", cache)
        return pd.read_pickle(cache)
    from ml.xi.builder import build
    from ml.xi.sources import CricsheetJsonSource
    result = build(CricsheetJsonSource(cricsheet_dir))
    frame = perf_baselines.add_baseline_predictors(result.player_frame)
    if cache:
        frame.to_pickle(cache)
    return frame


def run_folds(frame: pd.DataFrame, train_formats: Sequence[str], eval_format: str, spec: P.FitSpec) -> Dict:
    """Per-fold model and career-mean metrics for the spec's targets, and their mean over folds."""
    folds: List[Dict] = []
    for cutoff, end in fold_windows():
        train = frame[frame.format_code.isin(train_formats) & (frame.match_date < cutoff)]
        evaluation = frame[(frame.format_code == eval_format) & (frame.match_date >= cutoff) & (frame.match_date < end)]
        if len(train) < MIN_TRAIN_ROWS or len(evaluation) < MIN_EVAL_ROWS:
            continue
        started = time.perf_counter()
        report = perf_harness.evaluate_window(train, evaluation, eval_format, spec).report
        fold = {"cutoff": cutoff.date().isoformat(), "seconds": round(time.perf_counter() - started, 1)}
        for target, entry in report["targets"].items():
            fold[target] = {
                "pinball": entry["model"]["pinball"],
                "spearman": entry["model"]["within_match_spearman"],
                "mae": entry["model"]["mae"],
                "career_mean_pinball": entry["career_mean"]["pinball"],
                "career_mean_spearman": entry["career_mean"]["within_match_spearman"],
            }
        folds.append(fold)
        logger.info(
            "%s <- %s @ %s: %s",
            eval_format,
            "+".join(train_formats),
            fold["cutoff"],
            {t: fold[t]["pinball"] for t in spec.targets},
        )
    summary = {
        target: {
            metric: float(np.mean([f[target][metric] for f in folds if f[target][metric] is not None]))
            for metric in ("pinball", "spearman", "mae", "career_mean_pinball", "career_mean_spearman")
        }
        for target in spec.targets
    }
    return {"folds": folds, "summary": summary, "n_folds": len(folds)}


def stage_grid(frame: pd.DataFrame) -> Dict:
    """One seed per point: the grid is coarse and the seed spread is measured later."""
    out: Dict = {"by_point": {}}
    for name in P.HYPERPARAMETER_GRID:
        out["by_point"][name] = {
            fmt: run_folds(
                frame,
                [fmt],
                fmt,
                P.default_spec(shared_factor=False, hyperparameters=name, seeds=(0,), targets=TARGETS),
            )
            for fmt in FORMATS
        }
    # Pick by pinball relative to the default point, averaged over formats and targets.
    reference = out["by_point"][P.DEFAULT_HYPERPARAMETERS]
    scores = {}
    for name, per_format in out["by_point"].items():
        ratios = [
            per_format[fmt]["summary"][t]["pinball"] / reference[fmt]["summary"][t]["pinball"]
            for fmt in FORMATS
            for t in TARGETS
        ]
        scores[name] = float(np.mean(ratios))
    out["relative_pinball"] = scores
    out["chosen"] = min(scores, key=scores.get)
    return out


def stage_structure(frame: pd.DataFrame, hyperparameters: str) -> Dict:
    out: Dict = {"by_structure": {}}
    for structure in P.STRUCTURES:
        spec = P.default_spec(
            shared_factor=False,
            hyperparameters=hyperparameters,
            structure={t.name: structure for t in P.TARGETS},
            seeds=SEEDS,
            targets=TARGETS,
        )
        out["by_structure"][structure] = {fmt: run_folds(frame, [fmt], fmt, spec) for fmt in FORMATS}
    return decide_structure(out)


# Two-part costs three times the fits of direct; it earns its place only by beating direct
# by more than this share of pinball loss in every format. A smaller gain is a tie, and a
# tie goes to the simpler structure.
STRUCTURE_MIN_GAIN_PCT = 0.5


def decide_structure(out: Dict) -> Dict:
    chosen, gains = {}, {}
    for target in TARGETS:
        gains[target] = {}
        for fmt in FORMATS:
            direct = out["by_structure"]["direct"][fmt]["summary"][target]["pinball"]
            two_part = out["by_structure"]["two_part"][fmt]["summary"][target]["pinball"]
            gains[target][fmt] = float((direct - two_part) / direct * 100.0)
        chosen[target] = "two_part" if min(gains[target].values()) > STRUCTURE_MIN_GAIN_PCT else "direct"
    out["two_part_gain_pct"] = gains
    out["chosen"] = chosen
    return out


def stage_sequence(frame: pd.DataFrame, hyperparameters: str, structure: Dict[str, str]) -> Dict:
    """E1: base inputs vs base + one family at a time."""
    full_structure = {t.name: structure.get(t.name, "direct") for t in P.TARGETS}
    configs = {"base": ()} | {family: (family,) for family in C.SEQUENCE_FAMILIES}
    out: Dict = {"by_family": {}}
    for name, families in configs.items():
        spec = P.default_spec(
            shared_factor=False,
            sequence_families=families,
            hyperparameters=hyperparameters,
            structure=full_structure,
            seeds=SEEDS,
            targets=TARGETS,
        )
        out["by_family"][name] = {fmt: run_folds(frame, [fmt], fmt, spec) for fmt in FORMATS}
    base = out["by_family"]["base"]
    table = {}
    for family in C.SEQUENCE_FAMILIES:
        table[family] = {}
        for fmt in FORMATS:
            for target in TARGETS:
                b = base[fmt]["summary"][target]["pinball"]
                f = out["by_family"][family][fmt]["summary"][target]["pinball"]
                table[family][f"{fmt}_{target}_pinball_change_pct"] = float((f - b) / b * 100.0)
    out["pinball_change_pct"] = table
    # Keep a family only if it lowers pinball by more than 1 % on some (format, target) and
    # raises it on none by more than the seed spread deserves (plan §5: > 1 % rule).
    kept = [
        family for family, changes in table.items() if min(changes.values()) < -1.0 and max(changes.values()) <= 1.0
    ]
    out["kept"] = kept
    return out


def stage_transfer(frame: pd.DataFrame, hyperparameters: str, structure: Dict[str, str], families) -> Dict:
    """E6: T20I (and T20) scored from a separate fit vs a joint T20 + T20I fit."""
    full_structure = {t.name: structure.get(t.name, "direct") for t in P.TARGETS}
    separate = P.default_spec(
        shared_factor=False,
        sequence_families=families,
        hyperparameters=hyperparameters,
        structure=full_structure,
        seeds=SEEDS,
        targets=TARGETS,
    )
    joint = P.default_spec(
        shared_factor=False,
        sequence_families=families,
        joint_format=True,
        hyperparameters=hyperparameters,
        structure=full_structure,
        seeds=SEEDS,
        targets=TARGETS,
    )
    out: Dict = {"separate": {}, "joint": {}}
    for fmt in C.E6_JOINT_FORMATS:
        out["separate"][fmt] = run_folds(frame, [fmt], fmt, separate)
        out["joint"][fmt] = run_folds(frame, list(C.E6_JOINT_FORMATS), fmt, joint)
    deltas = {
        f"{fmt}_{target}_spearman_delta": out["joint"][fmt]["summary"][target]["spearman"]
        - out["separate"][fmt]["summary"][target]["spearman"]
        for fmt in C.E6_JOINT_FORMATS
        for target in TARGETS
    }
    out["joint_minus_separate"] = deltas
    out["joint_wins"] = all(d > 0.01 for d in deltas.values())
    return out


def _write(path: str, report: Dict) -> None:
    with open(path, "w") as fh:
        json.dump(report, fh, indent=2)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--cricsheet-dir")
    p.add_argument("--cache", help="pickle of the baselined player frame; written if absent")
    p.add_argument("--out", required=True)
    p.add_argument("--stages", nargs="+", default=["grid", "structure", "sequence", "transfer"])
    args = p.parse_args(argv)
    frame = load_frame(args.cricsheet_dir, args.cache)
    report: Dict = {
        "cutoffs": [c.date().isoformat() for c, _ in fold_windows()],
        "seeds": list(SEEDS),
        "formats": list(FORMATS),
        "targets": list(TARGETS),
    }
    if os.path.exists(args.out):
        with open(args.out) as fh:
            report.update(json.load(fh))
    hyperparameters = report.get("grid", {}).get("chosen", P.DEFAULT_HYPERPARAMETERS)
    if "structure" in report:
        report["structure"] = decide_structure(report["structure"])  # the rule may have moved since the fits
    structure = report.get("structure", {}).get("chosen", {})
    families = tuple(report.get("sequence", {}).get("kept", ()))
    if "grid" in args.stages:
        report["grid"] = stage_grid(frame)
        hyperparameters = report["grid"]["chosen"]
        _write(args.out, report)
    if "structure" in args.stages:
        report["structure"] = stage_structure(frame, hyperparameters)
        structure = report["structure"]["chosen"]
        _write(args.out, report)
    if "sequence" in args.stages:
        report["sequence"] = stage_sequence(frame, hyperparameters, structure)
        families = tuple(report["sequence"]["kept"])
        _write(args.out, report)
    if "transfer" in args.stages:
        report["transfer"] = stage_transfer(frame, hyperparameters, structure, families)
        _write(args.out, report)
    logger.info(
        "choices: hyperparameters=%s structure=%s families=%s joint=%s",
        hyperparameters,
        structure,
        families,
        report.get("transfer", {}).get("joint_wins"),
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
