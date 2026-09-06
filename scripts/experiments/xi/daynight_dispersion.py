"""The simulator's day/night calibration gap: does conditioning the shared match factor on
the pre-match day/night population bring both halves to nominal? Decided on the walk-forward
folds and never on the locked window (H-19).

X-2 (docs/EXTERNAL_DATA_PLAN.md § X-2) split the H-22 interval check by day and night and
found the *control* simulator miscalibrated in opposite directions: T20 first-innings
coverage 0.734 by day against 0.841 at night (nominal 0.80), ODI 0.715 against 0.932, the
pooled figure the average of an under- and an over-dispersed half. The named cause is that
the simulator fits **one** dispersion to two populations -- the shared match factor is one
residual pool per format, fitted on the calibration fold and sampled for every fixture.

Three arms per fold, differing only in that pool (``ml.xi.gates`` SIM-DN-split /
SIM-DN-scale, printed before anything runs):

* ``control`` -- today's simulator: one pooled residual pool.
* ``split`` -- one pool per population, each fitted and deconvolved from its own calibration
  matches under the same rule and the same 30-match guard. Varies location and spread.
* ``scale`` -- the pooled pool rescaled per population, each population's deviations from one
  multiplied by sqrt(its excess variance / the pooled excess variance): one shape borrowed
  across both, spread varied and location left pooled, under a 15-match floor.

Everything else is held: one L2-B fit per fold shared by the arms (so every headline pinball
is identical by construction), the control's display models (so the display AUC is), the
calibration fold and the draws the factor is fitted from, the simulator's draw count and its
seeds (common random numbers), the labels. A population under an arm's floor falls back to
the pooled factor and the fold records it.

A fold whose calibration window is too thin to fit a shared factor at all is **skipped**, not
scored: with no factor there is no pool to condition and the three arms would be the same run.
That matters for reading X-2's ODI figures, which include one such fold (2026-03, 24 complete
first innings against the 30 the deconvolution needs, first-innings coverage 0.500 and
dispersion 1.48 -- the un-widened simulator, not a day/night effect).

The day/night label is ``ml.weather.sessions``' inference from the documented session rules
-- competition and format norms, which are pre-match knowledge (H-21) -- **not** an observed
start time. A match whose actual start differed is mislabelled; the rule that placed each
match is recorded so that error is inspectable, and a null here is a null for *this*
inference.

    python daynight_dispersion.py --frames frames.pkl --cricsheet-dir ... --format T20 --out dn_T20.json
    python daynight_dispersion.py --decide dn_T20.json dn_ODI.json     # the tables and the verdict
"""

from __future__ import annotations

import argparse
import json
import logging
import math
import os
import sys
import time
from typing import Any, Callable, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

sys.path.insert(
    0,
    os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"),
)

from sim_frame_cache import load_frames  # noqa: E402

from ml.weather import geocoding, sessions, venues  # noqa: E402
from ml.weather.backfill import DEFAULT_GEOCODING  # noqa: E402
from ml.xi import contract as C  # noqa: E402
from ml.xi import gates, perf_harness, sim_harness, simulator  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import DISPLAY_SEEDS, fold_windows  # noqa: E402
from ml.xi.train import _xy, make_display_model  # noqa: E402

logger = logging.getLogger("daynight_dispersion")

CONTROL = "control"
SPLIT, SCALE = "split", "scale"
ARMS = (CONTROL, SPLIT, SCALE)
GATE_IDS = {SPLIT: "SIM-DN-split", SCALE: "SIM-DN-scale"}
#: The populations the pre-match label splits the fixtures into.
POPULATIONS = ("day", "night")
NIGHT_COL = "dn_night"
#: The format the gate decides on. ODI's night side clears neither the simulator's 20-match
#: floor nor the factor's guard often enough to decide (see the module docstring's table).
DECIDED_FORMATS = ("T20",)
SIM_SAMPLES = sim_harness.HARNESS_SAMPLES
NOMINAL_COVERAGE = 0.8
NOMINAL_DISPERSION = 1.0
#: The split arm reuses the deconvolution guard; a scale is one moment, not a distribution.
SPLIT_MIN_MATCHES = simulator.MIN_SHARED_FACTOR_MATCHES
SCALE_MIN_MATCHES = 15
#: H-22: the pooled interval may not be inflated by more than this to buy coverage.
WIDTH_TOLERANCE = 0.01
BRIER_TOLERANCE = sim_harness.BRIER_TOLERANCE


# --- the pre-match day/night label ------------------------------------------------------


def night_labels(cricsheet_dir: str, geocoding_path: str) -> pd.DataFrame:
    """One row per archived match: the inferred night flag and the session rule that placed
    it. Nothing here reads a ball of the match (H-21)."""
    fixtures, _ = venues.read_archive(cricsheet_dir)
    locations = geocoding.read_locations(geocoding_path)
    windows = sessions.assign(fixtures, {k: loc.country_code for k, loc in locations.items() if loc.mapped})
    frame = pd.DataFrame(
        [
            {
                "match_id": str(f.match_id),
                NIGHT_COL: 1.0 if windows[f.match_id].night else 0.0,
                "session_rule": windows[f.match_id].rule,
            }
            for f in fixtures
        ]
    )
    logger.info(
        "day/night labels: %d matches, %.1f %% night, %d rules",
        len(frame),
        100 * frame[NIGHT_COL].mean(),
        frame.session_rule.nunique(),
    )
    return frame


def join_night(frame: pd.DataFrame, labels: pd.DataFrame) -> pd.DataFrame:
    """The night flag onto a frame by match id; a match the archive index lacks reads day."""
    out = frame.copy()
    out["match_id"] = out["match_id"].astype(str)
    merged = out.merge(labels[["match_id", NIGHT_COL]], on="match_id", how="left")
    merged[NIGHT_COL] = merged[NIGHT_COL].fillna(0.0)
    return merged


# --- the arms: the shared factor each population draws from ------------------------------


def _population_masks(
    sample: simulator.SharedFactorCalibrationSample, night_by_id: Dict[str, float]
) -> Dict[str, np.ndarray]:
    """The calibration sample's matches split by the pre-match label."""
    is_night = np.asarray([night_by_id.get(str(m), 0.0) == 1.0 for m in sample.match_ids])
    return {"day": ~is_night, "night": is_night}


def _excess_variance(sample: simulator.SharedFactorCalibrationSample) -> float:
    """The residual variance of a sample's ratios beyond the simulator's own dispersion --
    what ``fit_shared_factor`` deconvolves, read here for one population."""
    mean = np.maximum(sample.simulated_mean, 1.0)
    residual = float(np.var(sample.actual / mean))
    within = float(np.mean((sample.simulated_sd / mean) ** 2))
    return max(residual - within, 0.0)


def _rescaled(pooled: simulator.SharedFactor, scale: float) -> simulator.SharedFactor:
    """The pooled pool's deviations from one, multiplied by ``scale``: the same shape and the
    same matches, a different spread."""
    factors = np.maximum(1.0 + (pooled.factors - 1.0) * scale, 0.0)
    return simulator.SharedFactor(
        factors,
        pooled.n_matches,
        pooled.residual_variance,
        pooled.within_variance,
        pooled.shrink * scale,
        pooled.sample_read,
    )


def arm_factors(
    arm: str, pooled: simulator.SharedFactor, masks: Dict[str, np.ndarray]
) -> Tuple[Dict[str, simulator.SharedFactor], Dict[str, Any]]:
    """The factor each population draws from under one arm, and what the fold records about
    how it was built (the per-population match counts, the fitted spread, any fallback)."""
    factors = {population: pooled for population in POPULATIONS}
    note: Dict[str, Any] = {
        "arm": arm,
        "pooled_factor_sd": float(np.std(pooled.factors)),
    }
    if arm == CONTROL:
        return factors, note
    sample = pooled.sample_read
    pooled_excess = _excess_variance(sample)
    for population, mask in masks.items():
        n = int(mask.sum())
        entry: Dict[str, Any] = {"n_calibration_matches": n}
        floor = SPLIT_MIN_MATCHES if arm == SPLIT else SCALE_MIN_MATCHES
        if n < floor:
            entry["fallback"] = f"under the arm's {floor}-match floor; the pooled factor"
        elif arm == SPLIT:
            factors[population] = simulator.fit_shared_factor(sample.take(mask))
        elif pooled_excess <= 0.0:
            entry["fallback"] = "the pooled pool has no excess variance to rescale"
        else:
            scale = math.sqrt(_excess_variance(sample.take(mask)) / pooled_excess)
            entry["scale"] = scale
            factors[population] = _rescaled(pooled, scale)
        entry["factor_sd"] = float(np.std(factors[population].factors))
        note[population] = entry
    return factors, note


# --- one fold ---------------------------------------------------------------------------


def _display_models(train_matches: pd.DataFrame) -> List[Any]:
    x, y = _xy(train_matches, C.DISPLAY_FEATURE_COLS)
    return [make_display_model(C.DISPLAY_FEATURE_COLS, seed).fit(x, y) for seed in DISPLAY_SEEDS]


def _summary(report: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """The numbers the gate reads, from one ``evaluate_window``; None where the population is
    too thin for the harness to simulate it."""
    if "skipped_reason" in report:
        return None
    first, chase = report["totals"]["first_innings"], report["totals"]["chase"]
    return {
        "n_matches": report["n_matches"],
        "first_coverage_80": first.get("coverage_80"),
        "first_width_80": first.get("width_80"),
        "first_bias": first.get("bias"),
        "first_dispersion_ratio": first.get("dispersion_ratio"),
        "first_below_q10": first.get("below_q10"),
        "first_above_q90": first.get("above_q90"),
        "chase_coverage_80": chase.get("coverage_80"),
        "chase_width_80": chase.get("width_80"),
        "chase_bias": chase.get("bias"),
        "delta_brier": report["win"]["delta_brier_simulated_minus_display"],
    }


#: Keys whose pooled figure is a mean over matches, so the match-count-weighted mean of the
#: populations is the pooled figure. The dispersion ratio is a ratio of spreads and is not.
POOLABLE = (
    "first_coverage_80",
    "first_width_80",
    "first_bias",
    "first_below_q10",
    "first_above_q90",
    "chase_coverage_80",
    "chase_width_80",
    "chase_bias",
    "delta_brier",
)


def _pooled_summary(
    populations: Dict[str, Optional[Dict[str, Any]]],
) -> Optional[Dict[str, Any]]:
    present = [s for s in populations.values() if s]
    if not present:
        return None
    out: Dict[str, Any] = {"n_matches": int(sum(s["n_matches"] for s in present))}
    for key in POOLABLE:
        weighted = [(s[key], s["n_matches"]) for s in present if s.get(key) is not None]
        out[key] = float(sum(v * w for v, w in weighted) / sum(w for _, w in weighted)) if weighted else None
    return out


def run_fold(
    player_frame: pd.DataFrame,
    match_frame: pd.DataFrame,
    fmt: str,
    cutoff: pd.Timestamp,
    end: pd.Timestamp,
) -> Dict[str, Any]:
    train, joint = perf_harness.training_rows(player_frame, fmt, cutoff)
    evaluation = player_frame[
        (player_frame.format_code == fmt) & (player_frame.match_date >= cutoff) & (player_frame.match_date < end)
    ]
    matches = match_frame[match_frame.format_code == fmt]
    train_matches = matches[matches.match_date < cutoff]
    eval_matches = matches[(matches.match_date >= cutoff) & (matches.match_date < end)]
    is_night = eval_matches[NIGHT_COL] == 1.0
    windows = {"day": eval_matches[~is_night], "night": eval_matches[is_night]}
    fold: Dict[str, Any] = {
        "cutoff": cutoff.date().isoformat(),
        "end": end.date().isoformat(),
        "n_train": int(len(train)),
        "n_eval": int(len(evaluation)),
        "n_eval_matches": {population: int(len(w)) for population, w in windows.items()},
    }
    if len(train) < perf_harness.MIN_TRAIN_ROWS or len(evaluation) < perf_harness.MIN_EVAL_ROWS:
        fold["skipped"] = True
        fold["skipped_reason"] = "too few training or evaluation rows"
        return fold
    started = time.perf_counter()
    displays = _display_models(train_matches)
    base_rate = float(train_matches[C.TARGET_COL].mean())
    # One fit per fold: the members, the recalibration and the pooled shared factor. The arms
    # differ only in which pool the factor is sampled from, so nothing else may be refitted.
    model = P.fit_performance(
        train,
        fmt,
        P.default_spec(joint_format=joint, shared_factor=True),
        train_matches,
    )
    fitted = model.simulation
    fold["fit_seconds"] = model.metadata["fit_seconds"]
    if fitted.shared_factor is None:
        fold["skipped"] = True
        fold["skipped_reason"] = "the calibration fold is too thin for a shared factor"
        return fold
    night_by_id = dict(zip(match_frame.match_id.astype(str), match_frame[NIGHT_COL]))
    masks = _population_masks(fitted.shared_factor.sample_read, night_by_id)
    fold["n_calibration_matches"] = {p: int(m.sum()) for p, m in masks.items()}
    for arm in ARMS:
        factors, note = arm_factors(arm, fitted.shared_factor, masks)
        summaries: Dict[str, Optional[Dict[str, Any]]] = {}
        for population, window in windows.items():
            model.simulation = simulator.SimulatorCalibration(fitted.runs_balls_rho, factors[population])
            players = evaluation[evaluation.match_id.isin(window.match_id)]
            summaries[population] = _summary(
                sim_harness.evaluate_window(model, displays, window, players, fmt, base_rate, SIM_SAMPLES)
            )
        fold[arm] = {**summaries, "all": _pooled_summary(summaries), "factor": note}
        logger.info(
            "%s @ %s %-7s day cov %s disp %s width %s | night cov %s disp %s width %s | pooled ΔBrier %s",
            fmt,
            fold["cutoff"],
            arm,
            _f((summaries["day"] or {}).get("first_coverage_80"), "%.3f"),
            _f((summaries["day"] or {}).get("first_dispersion_ratio"), "%.3f"),
            _f((summaries["day"] or {}).get("first_width_80"), "%.1f"),
            _f((summaries["night"] or {}).get("first_coverage_80"), "%.3f"),
            _f((summaries["night"] or {}).get("first_dispersion_ratio"), "%.3f"),
            _f((summaries["night"] or {}).get("first_width_80"), "%.1f"),
            _f((fold[arm]["all"] or {}).get("delta_brier"), "%+.4f"),
        )
    model.simulation = fitted
    fold["seconds"] = round(time.perf_counter() - started, 1)
    return fold


# --- the verdict --------------------------------------------------------------------------


def _f(value: Optional[float], pattern: str) -> str:
    return "n/a" if value is None or value != value else pattern % value


def _value(fold: Dict[str, Any], arm: str, population: str, key: str) -> Optional[float]:
    node = fold.get(arm, {}).get(population)
    return None if not node else node.get(key)


def _paired(
    folds: List[Dict[str, Any]],
    arm: str,
    quantity: Callable[[Dict[str, Any], str], Optional[float]],
) -> Dict[str, Any]:
    """The paired per-fold difference (arm - control) of a quantity, its mean and its
    standard error over the folds where both arms have it."""
    values = []
    for fold in folds:
        if fold.get("skipped"):
            continue
        a, b = quantity(fold, arm), quantity(fold, CONTROL)
        if a is not None and b is not None and a == a and b == b:
            values.append(a - b)
    if not values:
        return {"mean": None, "se": None, "n_folds": 0}
    se = float(np.std(values, ddof=1) / math.sqrt(len(values))) if len(values) > 1 else float("nan")
    return {"mean": float(np.mean(values)), "se": se, "n_folds": len(values)}


def _distance_to(target: float, population: str, key: str) -> Callable[[Dict[str, Any], str], Optional[float]]:
    def quantity(fold: Dict[str, Any], arm: str) -> Optional[float]:
        value = _value(fold, arm, population, key)
        return None if value is None else abs(value - target)

    return quantity


def _shrinks(delta: Dict[str, Any]) -> bool:
    """A distance that shrinks by more than one fold-level standard error (the effect-size
    floor A-1's null asked every later gate of this kind to state)."""
    return (
        delta["mean"] is not None
        and delta["se"] is not None
        and delta["se"] == delta["se"]
        and delta["mean"] < -abs(delta["se"])
    )


def _not_worse(delta: Dict[str, Any]) -> bool:
    """A distance that does not grow by more than one fold-level standard error."""
    return (
        delta["mean"] is not None
        and delta["se"] is not None
        and delta["se"] == delta["se"]
        and delta["mean"] <= abs(delta["se"])
    )


def arm_means(folds: List[Dict[str, Any]]) -> Dict[str, Dict[str, Any]]:
    """Per arm and population, the mean over the scored folds of everything recorded."""
    scored = [f for f in folds if not f.get("skipped")]
    out: Dict[str, Dict[str, Any]] = {}
    for arm in ARMS:
        entry: Dict[str, Any] = {"n_folds": len(scored)}
        for population in (*POPULATIONS, "all"):
            present = [f[arm][population] for f in scored if f.get(arm, {}).get(population)]
            keys = sorted({k for node in present for k in node})
            entry[population] = {
                key: (
                    float(np.mean([node[key] for node in present if node.get(key) is not None]))
                    if any(node.get(key) is not None for node in present)
                    else None
                )
                for key in keys
            }
            entry[population]["n_scored_folds"] = len(present)
        out[arm] = entry
    return out


def verdict(
    folds: List[Dict[str, Any]],
    means: Dict[str, Dict[str, Any]],
    arm: str,
    decided: bool,
) -> Dict[str, Any]:
    """The gate's clauses for one arm on one format, paired per fold against the control."""
    coverage = {p: _paired(folds, arm, _distance_to(NOMINAL_COVERAGE, p, "first_coverage_80")) for p in POPULATIONS}
    dispersion = {
        p: _paired(folds, arm, _distance_to(NOMINAL_DISPERSION, p, "first_dispersion_ratio")) for p in POPULATIONS
    }
    chase = {p: _paired(folds, arm, _distance_to(NOMINAL_COVERAGE, p, "chase_coverage_80")) for p in POPULATIONS}
    brier = _paired(folds, arm, lambda f, a: _value(f, a, "all", "delta_brier"))
    control_width = means[CONTROL]["all"].get("first_width_80")
    arm_width = means[arm]["all"].get("first_width_80")
    checks = {
        **{f"{p}_first_coverage_moves_to_nominal": _shrinks(coverage[p]) for p in POPULATIONS},
        **{f"{p}_dispersion_moves_to_one": _shrinks(dispersion[p]) for p in POPULATIONS},
        **{f"{p}_chase_coverage_no_worse": _not_worse(chase[p]) for p in POPULATIONS},
        "pooled_width_not_inflated": (
            control_width is not None and arm_width is not None and arm_width <= control_width * (1.0 + WIDTH_TOLERANCE)
        ),
        "e2_unchanged": (
            brier["mean"] is not None
            and brier["se"] == brier["se"]
            and abs(brier["mean"]) <= abs(brier["se"])
            and abs(means[arm]["all"].get("delta_brier") or 0.0) <= BRIER_TOLERANCE
        ),
    }
    return {
        "decided": decided,
        "checks": checks,
        "passes": all(checks.values()),
        "coverage_distance_delta": coverage,
        "dispersion_distance_delta": dispersion,
        "chase_coverage_distance_delta": chase,
        "e2_delta": brier,
        "pooled_width": {"control": control_width, "arm": arm_width},
    }


# --- the driver ---------------------------------------------------------------------------


def _write(path: str, payload: Dict[str, Any]) -> None:
    with open(path, "w") as fh:
        json.dump(payload, fh, indent=2)


def run(player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, out: str) -> Dict[str, Any]:
    result: Dict[str, Any] = {
        "gates": {arm: gates.describe(GATE_IDS[arm]) for arm in (SPLIT, SCALE)},
        "format": fmt,
        "decided_format": fmt in DECIDED_FORMATS,
        "seeds": list(P.DEFAULT_SEEDS),
        "sim_samples": SIM_SAMPLES,
        "folds": [],
    }
    if os.path.exists(out):
        with open(out) as fh:
            result["folds"] = json.load(fh).get("folds", [])
        logger.info("resuming %s from %d scored folds", out, len(result["folds"]))
    done = {f["cutoff"] for f in result["folds"]}
    for cutoff, end in fold_windows():
        if cutoff.date().isoformat() in done:
            continue
        result["folds"].append(run_fold(player_frame, match_frame, fmt, cutoff, end))
        _write(out, result)
    result["means"] = arm_means(result["folds"])
    result["verdicts"] = {
        arm: verdict(result["folds"], result["means"], arm, result["decided_format"]) for arm in (SPLIT, SCALE)
    }
    _write(out, result)
    return result


COLUMNS = (
    ("matches/fold", "n_matches", "%.0f"),
    ("first coverage", "first_coverage_80", "%.3f"),
    ("first width", "first_width_80", "%.1f"),
    ("dispersion", "first_dispersion_ratio", "%.3f"),
    ("first bias", "first_bias", "%+.1f"),
    ("below q10", "first_below_q10", "%.3f"),
    ("above q90", "first_above_q90", "%.3f"),
    ("chase coverage", "chase_coverage_80", "%.3f"),
    ("chase width", "chase_width_80", "%.1f"),
    ("Δ Brier (E2)", "delta_brier", "%+.4f"),
)


def decide(paths: Sequence[str]) -> None:
    for arm in (SPLIT, SCALE):
        print(gates.describe(GATE_IDS[arm]))
        print()
    ships = {arm: True for arm in (SPLIT, SCALE)}
    decided_any = False
    for path in paths:
        with open(path) as fh:
            payload = json.load(fh)
        fmt, means = payload["format"], payload["means"]
        scored = [f for f in payload["folds"] if not f.get("skipped")]
        print(
            f"### {fmt} -- the shared factor's pool, split by the pre-match day/night label "
            f"({payload['sim_samples']} draws, {len(scored)} folds"
            f"{'' if payload['decided_format'] else ', reported not decided'})"
        )
        print()
        print("| arm | population | folds scored | " + " | ".join(name for name, _, _ in COLUMNS) + " | verdict |")
        print("|---|---|---:|" + "---:|" * len(COLUMNS) + "---|")
        for arm in ARMS:
            for population in ("all", *POPULATIONS):
                node = means[arm][population]
                cells = " | ".join(_f(node.get(key), pattern) for _, key, pattern in COLUMNS)
                cell = "control" if arm == CONTROL else ""
                if arm != CONTROL and population == "all":
                    failed = [k for k, ok in payload["verdicts"][arm]["checks"].items() if not ok]
                    cell = "passes" if not failed else "fails: " + ", ".join(failed)
                print(f"| {arm} | {population} | {node['n_scored_folds']} | {cells} | {cell} |")
        print()
        for arm in (SPLIT, SCALE):
            v = payload["verdicts"][arm]
            for name, node in (
                ("first coverage |Δ to 0.80|", v["coverage_distance_delta"]),
                ("dispersion |Δ to 1.0|", v["dispersion_distance_delta"]),
                ("chase coverage |Δ to 0.80|", v["chase_coverage_distance_delta"]),
            ):
                readings = ", ".join(
                    f"{p} {_f(node[p]['mean'], '%+.4f')} ± {_f(node[p]['se'], '%.4f')} ({node[p]['n_folds']} folds)"
                    for p in POPULATIONS
                )
                print(f"- {fmt} {arm}: paired {name} -- {readings}")
            e2 = v["e2_delta"]
            print(
                f"- {fmt} {arm}: paired Δ E2 {_f(e2['mean'], '%+.4f')} ± {_f(e2['se'], '%.4f')} over {e2['n_folds']} "
                f"folds; pooled width {_f(v['pooled_width']['control'], '%.1f')} → {_f(v['pooled_width']['arm'], '%.1f')}"
            )
            if payload["decided_format"]:
                decided_any = True
                ships[arm] = ships[arm] and v["passes"]
        print()
    print(
        "The display model and the performance model are the control's in every arm (one fit per fold, shared), so "
        "the display AUC and every headline pinball are identical by construction, not measured."
    )
    print()
    if not decided_any:
        print(f"no verdict: none of these files is a deciding format ({', '.join(DECIDED_FORMATS)})")
        return
    for arm in (SPLIT, SCALE):
        print(f"{arm}: {'SHIPS' if ships[arm] else 'not shipped (a recorded null)'}")


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--frames", help="pickle written by sim_frame_cache.py")
    p.add_argument(
        "--cricsheet-dir",
        help="directory of Cricsheet JSON files (the fixtures the windows are inferred for)",
    )
    p.add_argument("--geocoding", default=DEFAULT_GEOCODING)
    p.add_argument("--format", choices=list(simulator.SIMULATED_FORMATS))
    p.add_argument("--out", help="where the run's JSON goes (a re-run resumes from it)")
    p.add_argument("--decide", nargs="+", help="result files; prints the tables and the verdict")
    args = p.parse_args(argv)
    if args.decide:
        decide(args.decide)
        return 0
    if not (args.frames and args.cricsheet_dir and args.format and args.out):
        p.error("--frames, --cricsheet-dir, --format and --out are required")
    for arm in (SPLIT, SCALE):
        print(gates.describe(GATE_IDS[arm]))
    labels = night_labels(args.cricsheet_dir, args.geocoding)
    player_frame, match_frame = load_frames(None, args.frames)
    run(
        join_night(player_frame, labels),
        join_night(match_frame, labels),
        args.format,
        args.out,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
