"""A-2's gate: does a target-conditional chasing innings bring the simulated chase's tails
to the data? Decided on the walk-forward folds and never on the locked window (H-19).

Four arms per fold -- the chase response the simulator applies to the chasing side's runs
draws (``simulator.CHASE_RESPONSE_ARMS``): ``none`` (today's simulator), ``level`` (the
control: slope held at zero), ``slope`` and ``both`` -- from **one** L2-B fit per fold and
one fitted calibration sample; everything else held fixed: the rows, the cutoffs, the three
seeds, the hyperparameters, the shared factor and its fit, the display models, the
simulator's draw count and seeds (common random numbers: the response consumes none), the
labels. Per arm and fold it records what the gate decides on (plan §8.10): the chase 10-90
coverage and bias, the first-innings coverage and width (H-22), E2's Brier delta -- and
beside them the chase width, the below-q10 / above-q90 shares, the margins, the fitted
coefficients and sigma against the simulated chase's own spread. The gate's triple
(``ml.xi.gates``, H-23) is printed before anything runs.

    python a2_chase_tails.py --frames frames.pkl --format T20 --out a2_T20.json
    python a2_chase_tails.py --decide a2_T20.json a2_ODI.json a2_T20I.json   # the verdict

T20I is reported, not decided on; TEST has no simulator.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from typing import Any, Dict, List, Optional, Sequence

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from sim_frame_cache import load_frames  # noqa: E402

from ml.xi import contract as C  # noqa: E402
from ml.xi import gates, perf_harness, sim_harness, simulator  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import fold_windows  # noqa: E402
from ml.xi.train import _xy, make_display_model  # noqa: E402

logger = logging.getLogger("a2_chase_tails")

GATE_ID = "A-2"
ARMS = simulator.CHASE_RESPONSE_ARMS
CONTROL, LEVEL_CONTROL = "none", "level"
CANDIDATES = ("slope", "both")
#: The formats the gate decides on; the others are reported.
DECIDED_FORMATS = ("T20", "ODI")
SIM_SAMPLES = sim_harness.HARNESS_SAMPLES
NOMINAL_COVERAGE = 0.8
#: The gate's tolerances (plan §8.10): first-innings coverage may move this much; E2's own
#: tolerance bounds the arm's Brier delta.
FIRST_COVERAGE_TOLERANCE = 0.03
BRIER_TOLERANCE = sim_harness.BRIER_TOLERANCE


def _display_model(train_matches: pd.DataFrame) -> Any:
    x, y = _xy(train_matches, C.DISPLAY_FEATURE_COLS)
    return make_display_model(C.DISPLAY_FEATURE_COLS).fit(x, y)


def _summary(report: Dict[str, Any]) -> Dict[str, Any]:
    """The simulator numbers the gate reads and reports, from one ``evaluate_window``."""
    first, chase = report["totals"]["first_innings"], report["totals"]["chase"]
    margin = report["margin"]
    response = (report["calibration"] or {}).get("chase_response")
    return {
        "n_matches": report["n_matches"],
        "chase_coverage_80": chase.get("coverage_80"),
        "chase_width_80": chase.get("width_80"),
        "chase_bias": chase.get("bias"),
        "chase_below_q10": chase.get("below_q10"),
        "chase_above_q90": chase.get("above_q90"),
        "first_coverage_80": first.get("coverage_80"),
        "first_width_80": first.get("width_80"),
        "first_bias": first.get("bias"),
        "brier_simulated": report["win"]["brier"]["simulated"],
        "delta_brier": report["win"]["delta_brier_simulated_minus_display"],
        "p_bat_first_wins_simulated": report["win"]["p_bat_first_wins"]["simulated"],
        "p_bat_first_wins_actual": report["win"]["p_bat_first_wins"]["actual"],
        "margin_runs_coverage_80": margin["runs_when_bat_first_wins"].get("coverage_80"),
        "margin_runs_width_80": margin["runs_when_bat_first_wins"].get("width_80"),
        "margin_balls_coverage_80": margin["balls_remaining_when_chaser_wins"].get("coverage_80"),
        "margin_balls_width_80": margin["balls_remaining_when_chaser_wins"].get("width_80"),
        "response": response,
    }


def _simulated_chase_log_sd(model: P.PerformanceModels, eval_matches: pd.DataFrame, evaluation: pd.DataFrame) -> float:
    """The simulated untruncated chase's spread on the log scale (toss-known, the fold's
    calibration without a response), the yardstick the fitted sigma is read against."""
    fixtures = simulator.fixtures_from_rows(evaluation, eval_matches, model.predict_oriented)
    without = simulator.SimulatorCalibration(model.simulation.runs_balls_rho, model.simulation.shared_factor)
    spreads = []
    for i, fixture in enumerate(fixtures[:100]):
        draws = simulator.simulate_match(fixture.team1, fixture.team2, fixture.context, 400, i, True, without)
        spreads.append(np.std(np.log(np.maximum(draws.team2.untruncated_total, 1.0))))
    return float(np.mean(spreads)) if spreads else float("nan")


def run_fold(player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, cutoff, end) -> Dict[str, Any]:
    train, joint = perf_harness.training_rows(player_frame, fmt, cutoff)
    evaluation = player_frame[
        (player_frame.format_code == fmt) & (player_frame.match_date >= cutoff) & (player_frame.match_date < end)
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
    if len(train) < perf_harness.MIN_TRAIN_ROWS or len(evaluation) < perf_harness.MIN_EVAL_ROWS:
        fold["skipped"] = True
        fold["skipped_reason"] = "too few training or evaluation rows"
        return fold
    started = time.perf_counter()
    display = _display_model(train_matches)
    base_rate = float(train_matches[C.TARGET_COL].mean())
    # One fit per fold: the members, the shared factor and the ``both`` response's sample.
    model = P.fit_performance(
        train, fmt, P.default_spec(joint_format=joint, shared_factor=True, chase_response="both"), train_matches
    )
    fitted = model.simulation
    fold["fit_seconds"] = model.metadata["fit_seconds"]
    if fitted.chase_response is None:
        fold["skipped"] = True
        fold["skipped_reason"] = "the calibration fold is too thin for a shared factor and a chase response"
        return fold
    fold["simulated_chase_log_sd"] = _simulated_chase_log_sd(model, eval_matches, evaluation)
    sample = fitted.chase_response.sample
    for arm in ARMS:
        response = simulator.fit_chase_response(sample, arm)
        model.simulation = simulator.SimulatorCalibration(fitted.runs_balls_rho, fitted.shared_factor, response)
        report = sim_harness.evaluate_window(model, display, eval_matches, evaluation, fmt, base_rate, SIM_SAMPLES)
        fold[arm] = _summary(report)
        logger.info(
            "%s @ %s %-6s chase coverage %.3f width %.1f bias %+.1f below-q10 %.3f | first %.3f / %.1f | Δ Brier %+.4f%s",
            fmt,
            fold["cutoff"],
            arm,
            fold[arm]["chase_coverage_80"],
            fold[arm]["chase_width_80"],
            fold[arm]["chase_bias"],
            fold[arm]["chase_below_q10"],
            fold[arm]["first_coverage_80"],
            fold[arm]["first_width_80"],
            fold[arm]["delta_brier"],
            ""
            if response is None
            else " | level %+.3f slope %+.3f sigma %.3f" % (response.level, response.slope, response.sigma),
        )
    model.simulation = fitted
    fold["seconds"] = round(time.perf_counter() - started, 1)
    return fold


def _fmt(value: Optional[float], pattern: str) -> str:
    return "n/a" if value is None else pattern % value


def _paired(folds: List[Dict[str, Any]], arm: str, against: str, quantity) -> Dict[str, Any]:
    """The paired difference per fold of ``quantity(summary)`` between two arms: its mean,
    its standard error over folds and the per-fold values."""
    values = np.asarray([quantity(f[arm]) - quantity(f[against]) for f in folds], dtype=float)
    se = float(np.std(values, ddof=1) / np.sqrt(len(values))) if len(values) > 1 else float("nan")
    return {"mean": float(values.mean()), "se": se, "by_fold": values.tolist()}


def coverage_distance(summary: Dict[str, Any]) -> float:
    return abs(NOMINAL_COVERAGE - summary["chase_coverage_80"])


def abs_chase_bias(summary: Dict[str, Any]) -> float:
    return abs(summary["chase_bias"])


def arm_means(folds: List[Dict[str, Any]]) -> Dict[str, Dict[str, Any]]:
    """Per arm, the means over the scored folds of everything reported."""
    scored = [f for f in folds if not f.get("skipped")]
    out: Dict[str, Dict[str, Any]] = {}
    numeric = [k for k in (scored[0][CONTROL] if scored else {}) if k not in ("n_matches", "response")]
    for arm in ARMS:
        summaries = [f[arm] for f in scored]
        entry: Dict[str, Any] = {"n_folds": len(summaries)}
        for key in numeric:
            present = [s[key] for s in summaries if s.get(key) is not None]
            entry[key] = float(np.mean(present)) if present else None
        entry["mean_abs_chase_bias"] = float(np.mean([abs_chase_bias(s) for s in summaries])) if summaries else None
        entry["mean_coverage_distance"] = (
            float(np.mean([coverage_distance(s) for s in summaries])) if summaries else None
        )
        responses = [s["response"] for s in summaries if s.get("response")]
        if responses:
            entry["level"] = float(np.mean([r["level"] for r in responses]))
            entry["slope"] = float(np.mean([r["slope"] for r in responses]))
            entry["sigma"] = float(np.mean([r["sigma"] for r in responses]))
            entry["slope_by_fold"] = [r["slope"] for r in responses]
            entry["level_by_fold"] = [r["level"] for r in responses]
            entry["n_calibration_matches"] = float(np.mean([r["n_matches"] for r in responses]))
        out[arm] = entry
    if scored:
        out["simulated_chase_log_sd"] = float(np.mean([f["simulated_chase_log_sd"] for f in scored]))
    return out


def verdict(folds: List[Dict[str, Any]], arm: str) -> Dict[str, Any]:
    """The gate's rule for one arm against ``none`` on one format (plan §8.10), paired per
    fold with one standard error as the floor; a candidate must also beat the level control."""
    scored = [f for f in folds if not f.get("skipped")]
    if len(scored) < 2:
        return {"decided": False, "reason": "fewer than two scored folds"}
    distance = _paired(scored, arm, CONTROL, coverage_distance)
    bias = _paired(scored, arm, CONTROL, abs_chase_bias)
    first_coverage = _paired(scored, arm, CONTROL, lambda s: s["first_coverage_80"])
    first_width = _paired(scored, arm, CONTROL, lambda s: s["first_width_80"])
    brier = _paired(scored, arm, CONTROL, lambda s: s["brier_simulated"])
    delta_brier_mean = float(np.mean([f[arm]["delta_brier"] for f in scored]))
    checks = {
        "coverage_moves_toward_nominal": distance["mean"] < -distance["se"],
        "abs_bias_shrinks": bias["mean"] < -bias["se"],
        "first_innings_held": abs(first_coverage["mean"]) <= FIRST_COVERAGE_TOLERANCE and first_width["mean"] <= 0.0,
        "e2_holds": delta_brier_mean <= BRIER_TOLERANCE and brier["mean"] <= brier["se"],
    }
    out: Dict[str, Any] = {
        "decided": True,
        **checks,
        "passes": all(checks.values()),
        "coverage_distance": distance,
        "abs_bias": bias,
        "first_coverage": first_coverage,
        "first_width": first_width,
        "brier": brier,
        "delta_brier_mean": delta_brier_mean,
        "within_nominal_band": abs(NOMINAL_COVERAGE - float(np.mean([f[arm]["chase_coverage_80"] for f in scored])))
        <= 0.03,
    }
    if arm in CANDIDATES:
        against_level = _paired(scored, arm, LEVEL_CONTROL, coverage_distance)
        out["beats_level_control"] = against_level["mean"] < -against_level["se"]
        out["coverage_distance_vs_level"] = against_level
        out["ships"] = out["passes"] and out["beats_level_control"]
    return out


def run_format(frames_path: str, fmt: str, out: str) -> Dict[str, Any]:
    player_frame, match_frame = load_frames(None, frames_path)
    folds = [run_fold(player_frame, match_frame, fmt, cutoff, end) for cutoff, end in fold_windows()]
    result = {
        "gate": gates.describe(GATE_ID),
        "format": fmt,
        "decided_format": fmt in DECIDED_FORMATS,
        "seeds": list(P.DEFAULT_SEEDS),
        "sim_samples": SIM_SAMPLES,
        "cutoffs": [f["cutoff"] for f in folds],
        "folds": folds,
        "means": arm_means(folds),
        "verdicts": {arm: verdict(folds, arm) for arm in ARMS if arm != CONTROL},
    }
    with open(out, "w") as fh:
        json.dump(result, fh, indent=2)
    logger.info("written %s", out)
    return result


def _cell(fmt: str, arm: str, v: Dict[str, Any]) -> str:
    if arm == CONTROL:
        return "control"
    if not v.get("decided"):
        return "not decided: " + v.get("reason", "")
    failed = [
        k for k in ("coverage_moves_toward_nominal", "abs_bias_shrinks", "first_innings_held", "e2_holds") if not v[k]
    ]
    cell = "passes" if v["passes"] else "fails: " + ", ".join(failed)
    if arm in CANDIDATES and v["passes"]:
        cell += "; beats level" if v["beats_level_control"] else "; does not beat level"
    if arm == LEVEL_CONTROL:
        cell = "control: " + cell
    if fmt not in DECIDED_FORMATS:
        cell = "reported: " + cell
    return cell


def decide(paths: Sequence[str]) -> Dict[str, Any]:
    """The cross-format verdict: a candidate ships only if it ships in every decided
    format; ``both`` over ``slope`` only if it beats it on the coverage distance by more
    than one standard error. Prints the tables the plan records."""
    results = {}
    for path in paths:
        with open(path) as fh:
            result = json.load(fh)
        results[result["format"]] = result
    print(gates.describe(GATE_ID))
    print()
    header = (
        "| format | arm | folds | chase coverage | below q10 | above q90 | chase width | chase bias | first coverage | "
        "first width | Δ Brier | P(bat-first wins) sim / actual | margin runs cov / width | margin balls cov / width | "
        "level | slope | σ vs sim | verdict |"
    )
    print(header)
    print("|" + "---|" * (header.count("|") - 1))
    for fmt, result in results.items():
        means = result["means"]
        for arm in ARMS:
            m = means[arm]
            v = result["verdicts"].get(arm, {})
            print(
                f"| {fmt} | {arm} | {m['n_folds']} | {_fmt(m['chase_coverage_80'], '%.3f')} | "
                f"{_fmt(m['chase_below_q10'], '%.3f')} | {_fmt(m['chase_above_q90'], '%.3f')} | "
                f"{_fmt(m['chase_width_80'], '%.1f')} | {_fmt(m['chase_bias'], '%+.1f')} | "
                f"{_fmt(m['first_coverage_80'], '%.3f')} | {_fmt(m['first_width_80'], '%.1f')} | "
                f"{_fmt(m['delta_brier'], '%+.4f')} | "
                f"{_fmt(m['p_bat_first_wins_simulated'], '%.3f')} / {_fmt(m['p_bat_first_wins_actual'], '%.3f')} | "
                f"{_fmt(m['margin_runs_coverage_80'], '%.3f')} / {_fmt(m['margin_runs_width_80'], '%.1f')} | "
                f"{_fmt(m['margin_balls_coverage_80'], '%.3f')} / {_fmt(m['margin_balls_width_80'], '%.1f')} | "
                f"{_fmt(m.get('level'), '%+.3f')} | {_fmt(m.get('slope'), '%+.3f')} | "
                f"{_fmt(m.get('sigma'), '%.3f')} vs {_fmt(means.get('simulated_chase_log_sd'), '%.3f')} | "
                f"{_cell(fmt, arm, v)} |"
            )
    print()
    print(
        "Paired differences per fold against none (mean ± se): coverage distance from 0.80, |chase bias|, simulated Brier"
    )
    for fmt, result in results.items():
        for arm in (a for a in ARMS if a != CONTROL):
            v = result["verdicts"][arm]
            if not v.get("decided"):
                continue
            extra = ""
            if "coverage_distance_vs_level" in v:
                d = v["coverage_distance_vs_level"]
                extra = f"; vs level: {d['mean']:+.4f} ± {d['se']:.4f}"
            print(
                f"{fmt} {arm}: distance {v['coverage_distance']['mean']:+.4f} ± {v['coverage_distance']['se']:.4f}; "
                f"|bias| {v['abs_bias']['mean']:+.2f} ± {v['abs_bias']['se']:.2f}; "
                f"Brier {v['brier']['mean']:+.5f} ± {v['brier']['se']:.5f}{extra}"
            )
    print()
    print("Per-fold slope (both arm):")
    for fmt, result in results.items():
        slopes = result["means"]["both"].get("slope_by_fold", [])
        print(f"{fmt}: " + ", ".join(f"{s:+.3f}" for s in slopes))
    print()
    ships = {}
    for arm in CANDIDATES:
        per_format = {fmt: results[fmt]["verdicts"][arm] for fmt in DECIDED_FORMATS if fmt in results}
        ships[arm] = (
            bool(per_format)
            and len(per_format) == len(DECIDED_FORMATS)
            and all(v.get("ships", False) for v in per_format.values())
        )
    chosen = "none"
    if ships["slope"] and ships["both"]:
        chosen = "slope"
        both_beats_slope = all(
            (lambda d: d["mean"] < -d["se"])(
                _paired([f for f in results[fmt]["folds"] if not f.get("skipped")], "both", "slope", coverage_distance)
            )
            for fmt in DECIDED_FORMATS
        )
        if both_beats_slope:
            chosen = "both"
    elif ships["slope"]:
        chosen = "slope"
    elif ships["both"]:
        chosen = "both"
    for arm, s in ships.items():
        print(f"{arm}: {'SHIPS' if s else 'does not ship'}")
    print(f"CHASE_RESPONSE = {chosen!r}")
    return {"chosen": chosen, "ships": ships}


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--frames", help="pickle from sim_frame_cache.py")
    p.add_argument("--format", choices=list(simulator.SIMULATED_FORMATS))
    p.add_argument("--out")
    p.add_argument("--decide", nargs="+", help="per-format result files; prints the tables and the verdict")
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
