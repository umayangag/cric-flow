"""A-1's gate: does a fixture-conditional level in L2-B shrink the simulator's per-quarter
bias? Decided on the walk-forward folds and never on the locked window (H-19).

Four arms per fold -- which fixture-context families the performance model reads: ``none``,
``venue``, ``competition``, ``both`` (``contract.FIXTURE_CONTEXT_FAMILIES``) -- everything
else held fixed: the rows, the cutoffs, the three seeds, the hyperparameters, the shared
factor's fitting rule, the display models (fitted once per fold and shared by the arms), the
simulator, its draw count and its seeds (common random numbers across arms), the labels.
Per arm and fold it records what the gate decides on (plan §8.9): the simulated
first-innings mean's bias, the 10-90 coverage and width of the simulated totals (H-22), and
the pinball loss of every headline target; the chase and E2's Brier delta are reported
beside them. The gate's triple (``ml.xi.gates``, H-23) is printed before anything runs.

    python a1_fixture_context.py --frames frames.pkl --format T20 --out a1_T20.json
    python a1_fixture_context.py --decide a1_T20.json a1_ODI.json      # the verdict, both formats

TEST has no simulator and is reported for pinball only; T20I is reported, not decided on.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

sys.path.insert(
    0,
    os.path.join(
        os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"
    ),
)

from sim_frame_cache import load_frames  # noqa: E402

from ml.xi import contract as C  # noqa: E402
from ml.xi import gates, perf_harness, sim_harness, simulator  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import DISPLAY_SEEDS, fold_windows  # noqa: E402
from ml.xi.train import _xy, make_display_model  # noqa: E402

logger = logging.getLogger("a1_fixture_context")

GATE_ID = "A-1"
ARMS: Dict[str, Tuple[str, ...]] = {
    "none": (),
    "venue": ("venue",),
    "competition": ("competition",),
    "both": ("venue", "competition"),
}
#: The formats the gate decides on; the others are reported.
DECIDED_FORMATS = ("T20", "ODI")
SIM_SAMPLES = sim_harness.HARNESS_SAMPLES
#: The gate's tolerances (plan §8.9): coverage may move this much, pinball may worsen by
#: this share (E1's noise band), width may not grow at all.
COVERAGE_TOLERANCE = 0.03
PINBALL_TOLERANCE = 0.005
HEADLINE_TARGETS = tuple(t.name for t in P.TARGETS if t.headline)


def _display_models(train_matches: pd.DataFrame) -> List[Any]:
    x, y = _xy(train_matches, C.DISPLAY_FEATURE_COLS)
    return [
        make_display_model(C.DISPLAY_FEATURE_COLS, seed).fit(x, y)
        for seed in DISPLAY_SEEDS
    ]


def _simulation_summary(report: Dict[str, Any]) -> Dict[str, Any]:
    """The simulator numbers the gate reads, from one ``sim_harness.evaluate_window``."""
    first, chase = report["totals"]["first_innings"], report["totals"]["chase"]
    return {
        "n_matches": report["n_matches"],
        "first_bias": first.get("bias"),
        "first_coverage_80": first.get("coverage_80"),
        "first_width_80": first.get("width_80"),
        "first_dispersion_ratio": first.get("dispersion_ratio"),
        "chase_bias": chase.get("bias"),
        "chase_coverage_80": chase.get("coverage_80"),
        "chase_width_80": chase.get("width_80"),
        "delta_brier": report["win"]["delta_brier_simulated_minus_display"],
        "shared_factor_sd": (report["calibration"] or {}).get("shared_factor", {})
        or {},
    }


def _performance_summary(targets: Dict[str, Dict]) -> Dict[str, Any]:
    """Pinball per headline target (pre-toss, the served prediction) and the runs
    interval's coverage and width."""
    out: Dict[str, Any] = {
        "pinball": {
            name: targets[name]["model"]["pinball"] for name in HEADLINE_TARGETS
        }
    }
    runs_interval = targets["runs"]["model"].get("interval") or {}
    out["runs_coverage_80"] = runs_interval.get("coverage_80")
    out["runs_width_80"] = runs_interval.get("width_80")
    return out


def run_fold(
    player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, cutoff, end
) -> Dict[str, Any]:
    train, joint = perf_harness.training_rows(player_frame, fmt, cutoff)
    evaluation = player_frame[
        (player_frame.format_code == fmt)
        & (player_frame.match_date >= cutoff)
        & (player_frame.match_date < end)
    ]
    matches = match_frame[match_frame.format_code == fmt]
    train_matches = matches[matches.match_date < cutoff]
    eval_matches = matches[(matches.match_date >= cutoff) & (matches.match_date < end)]
    fold: Dict[str, Any] = {
        "cutoff": cutoff.date().isoformat(),
        "end": end.date().isoformat(),
        "n_train": int(len(train)),
        "n_eval": int(len(evaluation)),
        "n_eval_matches": int(len(eval_matches)),
    }
    if (
        len(train) < perf_harness.MIN_TRAIN_ROWS
        or len(evaluation) < perf_harness.MIN_EVAL_ROWS
    ):
        fold["skipped"] = True
        return fold
    simulated = fmt in simulator.SIMULATED_FORMATS
    started = time.perf_counter()
    displays = _display_models(train_matches) if simulated else []
    base_rate = float(train_matches[C.TARGET_COL].mean())
    for arm, families in ARMS.items():
        spec = P.default_spec(
            joint_format=joint,
            shared_factor=simulated,
            fixture_context_families=families,
        )
        model = P.fit_performance(
            train, fmt, spec, train_matches if simulated else None
        )
        entry: Dict[str, Any] = {
            "families": list(families),
            "n_features": len(spec.feature_cols),
            "performance": _performance_summary(
                perf_harness.score_targets(model, train, evaluation)
            ),
            "fit_seconds": model.metadata["fit_seconds"],
        }
        if simulated:
            entry["simulation"] = _simulation_summary(
                sim_harness.evaluate_window(
                    model,
                    displays,
                    eval_matches,
                    evaluation,
                    fmt,
                    base_rate,
                    SIM_SAMPLES,
                )
            )
        fold[arm] = entry
        logger.info(
            "%s @ %s %-11s first bias %s coverage %s width %s | runs pinball %.3f | %.0f s",
            fmt,
            fold["cutoff"],
            arm,
            _fmt(entry.get("simulation", {}).get("first_bias"), "%+.1f"),
            _fmt(entry.get("simulation", {}).get("first_coverage_80"), "%.3f"),
            _fmt(entry.get("simulation", {}).get("first_width_80"), "%.1f"),
            entry["performance"]["pinball"]["runs"],
            entry["fit_seconds"],
        )
    fold["seconds"] = round(time.perf_counter() - started, 1)
    return fold


def _fmt(value: Optional[float], pattern: str) -> str:
    return "n/a" if value is None else pattern % value


def _mean(values: Sequence[Optional[float]]) -> Optional[float]:
    present = [float(v) for v in values if v is not None]
    return float(np.mean(present)) if present else None


def arm_means(folds: List[Dict[str, Any]]) -> Dict[str, Dict[str, Any]]:
    """Per arm, the means over the scored folds of what the gate reads."""
    scored = [f for f in folds if not f.get("skipped")]
    out: Dict[str, Dict[str, Any]] = {}
    for arm in ARMS:
        sims = [f[arm].get("simulation") for f in scored if arm in f]
        perfs = [f[arm]["performance"] for f in scored if arm in f]
        summary: Dict[str, Any] = {
            "n_folds": len(perfs),
            "pinball": {
                name: _mean([p["pinball"][name] for p in perfs])
                for name in HEADLINE_TARGETS
            },
            "runs_coverage_80": _mean([p["runs_coverage_80"] for p in perfs]),
            "runs_width_80": _mean([p["runs_width_80"] for p in perfs]),
        }
        if all(sims):
            summary.update(
                {
                    "mean_abs_first_bias": _mean([abs(s["first_bias"]) for s in sims]),
                    "first_bias": _mean([s["first_bias"] for s in sims]),
                    "first_bias_by_fold": [s["first_bias"] for s in sims],
                    "first_coverage_80": _mean([s["first_coverage_80"] for s in sims]),
                    "first_width_80": _mean([s["first_width_80"] for s in sims]),
                    "first_dispersion_ratio": _mean(
                        [s["first_dispersion_ratio"] for s in sims]
                    ),
                    "mean_abs_chase_bias": _mean([abs(s["chase_bias"]) for s in sims]),
                    "chase_bias": _mean([s["chase_bias"] for s in sims]),
                    "chase_coverage_80": _mean([s["chase_coverage_80"] for s in sims]),
                    "chase_width_80": _mean([s["chase_width_80"] for s in sims]),
                    "delta_brier": _mean([s["delta_brier"] for s in sims]),
                }
            )
        out[arm] = summary
    return out


def verdict(means: Dict[str, Dict[str, Any]], arm: str) -> Dict[str, Any]:
    """The gate's rule for one arm against ``none`` on one format (plan §8.9): |bias|
    shrinks, coverage holds within the tolerance, width does not grow, no headline
    target's pinball worsens by more than the tolerance."""
    control, candidate = means["none"], means[arm]
    if "mean_abs_first_bias" not in candidate:
        pinball_ok = all(
            candidate["pinball"][t] <= control["pinball"][t] * (1.0 + PINBALL_TOLERANCE)
            for t in HEADLINE_TARGETS
        )
        return {
            "decided": False,
            "pinball_no_worse": pinball_ok,
            "reason": "no simulator for this format",
        }
    checks = {
        "abs_bias_shrinks": candidate["mean_abs_first_bias"]
        < control["mean_abs_first_bias"],
        "coverage_holds": abs(
            candidate["first_coverage_80"] - control["first_coverage_80"]
        )
        <= COVERAGE_TOLERANCE,
        "width_does_not_grow": candidate["first_width_80"] <= control["first_width_80"],
        "pinball_no_worse": all(
            candidate["pinball"][t] <= control["pinball"][t] * (1.0 + PINBALL_TOLERANCE)
            for t in HEADLINE_TARGETS
        ),
    }
    return {
        "decided": True,
        **checks,
        "passes": all(checks.values()),
        "abs_bias": [control["mean_abs_first_bias"], candidate["mean_abs_first_bias"]],
        "coverage": [control["first_coverage_80"], candidate["first_coverage_80"]],
        "width": [control["first_width_80"], candidate["first_width_80"]],
        "pinball_worst_ratio": max(
            candidate["pinball"][t] / control["pinball"][t] for t in HEADLINE_TARGETS
        ),
    }


def run_format(frames_path: str, fmt: str, out: str) -> Dict[str, Any]:
    player_frame, match_frame = load_frames(None, frames_path)
    folds = [
        run_fold(player_frame, match_frame, fmt, cutoff, end)
        for cutoff, end in fold_windows()
    ]
    means = arm_means(folds)
    result = {
        "gate": gates.describe(GATE_ID),
        "format": fmt,
        "decided_format": fmt in DECIDED_FORMATS,
        "seeds": list(P.DEFAULT_SEEDS),
        "sim_samples": SIM_SAMPLES,
        "cutoffs": [f["cutoff"] for f in folds],
        "folds": folds,
        "means": means,
        "verdicts": {arm: verdict(means, arm) for arm in ARMS if arm != "none"},
    }
    with open(out, "w") as fh:
        json.dump(result, fh, indent=2)
    logger.info("written %s", out)
    return result


def decide(paths: Sequence[str]) -> Dict[str, Any]:
    """The cross-format verdict: a family is kept only if it passes in every decided
    format. Prints the per-family table the plan records."""
    results = {}
    for path in paths:
        with open(path) as fh:
            result = json.load(fh)
        results[result["format"]] = result
    kept = {}
    for arm in (a for a in ARMS if a != "none"):
        per_format = {
            fmt: results[fmt]["verdicts"][arm]
            for fmt in DECIDED_FORMATS
            if fmt in results
        }
        kept[arm] = bool(per_format) and all(
            v.get("passes", False) for v in per_format.values()
        )
    print(gates.describe(GATE_ID))
    print()
    header = "| format | arm | folds | mean |bias| | mean bias | coverage | width | chase bias | chase coverage | runs pinball | wickets pinball | balls pinball | conceded pinball | Δ Brier | verdict |"
    print(header)
    print("|" + "---|" * (header.count("|") - 1))
    for fmt, result in results.items():
        for arm, m in result["means"].items():
            v = result["verdicts"].get(arm, {})
            if arm == "none":
                cell = "control"
            elif not v.get("decided"):
                cell = "reported (pinball %s)" % (
                    "ok" if v.get("pinball_no_worse") else "worse"
                )
            else:
                failed = [
                    k
                    for k in (
                        "abs_bias_shrinks",
                        "coverage_holds",
                        "width_does_not_grow",
                        "pinball_no_worse",
                    )
                    if not v[k]
                ]
                cell = "passes" if v["passes"] else "fails: " + ", ".join(failed)
                if fmt not in DECIDED_FORMATS:
                    cell = "reported: " + cell
            pin = m["pinball"]
            print(
                f"| {fmt} | {arm} | {m['n_folds']} | {_fmt(m.get('mean_abs_first_bias'), '%.1f')} | "
                f"{_fmt(m.get('first_bias'), '%+.1f')} | {_fmt(m.get('first_coverage_80'), '%.3f')} | "
                f"{_fmt(m.get('first_width_80'), '%.1f')} | {_fmt(m.get('chase_bias'), '%+.1f')} | "
                f"{_fmt(m.get('chase_coverage_80'), '%.3f')} | {pin['runs']:.3f} | {pin['wickets']:.4f} | "
                f"{pin['balls_faced']:.3f} | {pin['runs_conceded']:.3f} | {_fmt(m.get('delta_brier'), '%+.4f')} | {cell} |"
            )
    print()
    # The effect size against its own fold-to-fold noise: the paired difference in |bias|
    # per fold, its mean and standard error. Reported beside the verdict, not part of it.
    for fmt, result in results.items():
        control = result["means"]["none"].get("first_bias_by_fold")
        if not control:
            continue
        for arm in (a for a in ARMS if a != "none"):
            paired = np.abs(result["means"][arm]["first_bias_by_fold"]) - np.abs(
                control
            )
            se = (
                float(np.std(paired, ddof=1) / np.sqrt(len(paired)))
                if len(paired) > 1
                else float("nan")
            )
            print(
                f"{fmt} {arm}: paired Δ|bias| {paired.mean():+.2f} ± {se:.2f} runs (se over {len(paired)} folds)"
            )
    print()
    for arm, is_kept in kept.items():
        print(f"{arm}: {'KEPT' if is_kept else 'not kept'}")
    return {"kept": [arm for arm, is_kept in kept.items() if is_kept], "verdicts": kept}


def main() -> int:
    logging.basicConfig(
        level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s"
    )
    p = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    p.add_argument("--frames", help="pickle from sim_frame_cache.py")
    p.add_argument("--format", choices=list(C.FORMAT_CODES))
    p.add_argument("--out")
    p.add_argument(
        "--decide",
        nargs="+",
        help="per-format result files; prints the table and the verdict",
    )
    args = p.parse_args()
    if args.decide:
        decide(args.decide)
        return 0
    if not (args.frames and args.format and args.out):
        p.error("--frames, --format and --out are required to run a format")
    print(gates.describe(GATE_ID))
    run_format(args.frames, args.format, args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
