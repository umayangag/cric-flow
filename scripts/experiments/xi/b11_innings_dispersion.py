"""B-11: a dispersion term the two innings do not share, independent (§8.14) or correlated
with the first innings (§8.15). Decided on the walk-forward folds and never on the locked
window (H-19).

The defect (docs/BUG_BACKLOG.md § B-11, plan §8.13): the simulator fits **one** dispersion to
two populations and its 10-90 interval is wrong in opposite directions on each. T20
first-innings coverage 0.734 by day against 0.841 at night at nominal 0.80 (width 78.4 / 96.3,
dispersion 1.099 / 0.865); the chase reads 0.689 / 0.759, under-covered in both.

§8.13 gated two candidates on the shared match factor -- conditioning its pool on the
pre-match day/night label, and rescaling it per population -- and recorded two nulls. Both
fix the day side, both overshoot the night side, and both make the night *chase* worse by
about five standard errors, because at night the first innings is over-covered while the
chase is under-covered **under one and the same factor**: narrowing it moves one toward
nominal and the other away, monotonically. Its conclusion is this script's premise: the lever
is wrong, not its setting. A-2 (plan §8.10) reached the same place from the chase side -- the
fitted chase residual scale 0.38-0.46 against the simulated chase's own 0.23-0.25, so the
miss is the chase's *dispersion*, not a level by difficulty.

§8.14 gated the chase's own dispersion term and recorded a **third** null. It came much
closer -- seven of eight cells moved toward nominal and the chase's tails reached 0.084 /
0.097, both nominal, with no level fitted -- and it failed on the signed ``e2_not_degraded``
at +0.0043 ± 0.0013. Its mechanism was understood and is this run's premise: an
**independent** chase term widens the *margin*, and the margin decides the match, so the
simulated P(win) moves toward 0.5. Interval calibration bought with probability calibration.

Four arms per fold, differing only in what multiplies the runs draws (``ml.xi.gates``
SIM-IN-corr / SIM-IN-corrboth, printed before anything runs; ``chase`` re-runs §8.14's
already-gated SIM-IN-chase as a **reference arm** so the two ways of widening the chase are
paired draw for draw against one control):

* ``control``  -- today's simulator: one pooled shared factor, both innings.
* ``chase``    -- §8.14's term: the shared factor unchanged, and the chasing side's runs draws
  multiplied by a second, **independent** mean-one factor (``simulator.fit_chase_dispersion``).
  Re-run, not re-gated: its verdict is recorded in §8.14.
* ``corr``     -- §8.15's candidate: the same second factor, drawn so that it **moves with the
  first innings' realised log residual** in the same draw and fitted against the chase's own
  expectation (``simulator.fit_correlated_chase_dispersion``). The **isolating arm**: the
  first innings' draws are taken before the chase's from the same stream, so they are
  bit-identical to the control's and its first-innings clauses cannot pass. It says how much
  of ``corrboth``'s movement is the new lever, the way A-2's level arm did.
* ``corrboth`` -- that same correlated term, composed with §8.13's ``SIM-DN-scale`` rule for
  the shared factor (imported from ``daynight_dispersion``, not reimplemented). The two levers
  correct different halves of one defect and neither can pass alone.

Everything else is held: one L2-B fit per fold shared by the arms, the control's display
models, the calibration fold and the draws every term is fitted from, the simulator's draw
count and its seeds, the labels.

**Common random numbers, exactly.** Every deciding number comes from the toss-known
simulation, which plays one orientation, so the arms' first-innings draws and their chase
draws are paired draw for draw. The one place they are not is E2's *pre-toss* probability:
that simulation plays both orientations in one stream, and the chase's extra draw shifts the
stream for the second half. E2 is a no-degradation guard here, not a deciding clause, and the
extra noise lands in its paired standard error rather than in its sign.

    python b11_innings_dispersion.py --frames frames.pkl --cricsheet-dir ... --probe
    python b11_innings_dispersion.py --frames frames.pkl --cricsheet-dir ... --format T20 --out b11_T20.json
    python b11_innings_dispersion.py --decide b11_T20.json b11_ODI.json
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

import daynight_dispersion as dn  # noqa: E402
from sim_frame_cache import load_frames  # noqa: E402

from ml.xi import gates, perf_harness, sim_harness, simulator  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import fold_windows  # noqa: E402

logger = logging.getLogger("b11_innings_dispersion")

CONTROL = dn.CONTROL
CHASE, CORR, CORRBOTH = "chase", "corr", "corrboth"
ARMS = (CONTROL, CHASE, CORR, CORRBOTH)
#: The arms this run gates. ``chase`` is a reference arm: §8.14 gated SIM-IN-chase and
#: recorded its null, and it is re-run here only so the independent and the correlated way of
#: widening the chase are paired draw for draw against one control.
CANDIDATES = (CORR, CORRBOTH)
REFERENCE = (CHASE,)
GATE_IDS = {CHASE: "SIM-IN-chase", CORR: "SIM-IN-corr", CORRBOTH: "SIM-IN-corrboth"}
POPULATIONS = dn.POPULATIONS
#: The format the gates decide on, for §8.13's reason: ODI's night side clears the harness's
#: 20-match floor in 2 of 11 folds and its night calibration fold clears the guards in 1,
#: never in the same fold, so its night rows are the control by construction.
DECIDED_FORMATS = ("T20",)
NOMINAL_COVERAGE = dn.NOMINAL_COVERAGE
NOMINAL_DISPERSION = dn.NOMINAL_DISPERSION
#: H-22, one-sided and per innings: the first innings' pooled interval may not be inflated by
#: more than this to buy coverage. The chase has no cap -- its correction *is* a widening --
#: and its dispersion clause is what an inflation would fail instead.
FIRST_WIDTH_TOLERANCE = dn.WIDTH_TOLERANCE
BRIER_TOLERANCE = sim_harness.BRIER_TOLERANCE
#: The censored fit needs a residual scale, and the scale is undefined without lost chases.
MIN_LOST_CHASES = 2


# --- the feasibility probe: can the folds decide anything at all? ------------------------


def probe_fold(
    player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, cutoff: pd.Timestamp, end: pd.Timestamp
) -> Dict[str, Any]:
    """What one fold holds, without fitting anything: the evaluation matches each population
    would be scored on, and the calibration matches each fitted term would read.

    It reproduces ``performance._fit_simulator_calibration``'s own selection -- the last
    ``CALIBRATION_DAYS`` of the training rows, restricted to the matches whose first innings
    ran its course -- by calling the same functions, so the counts are the ones the fit will
    see rather than an estimate of them.
    """
    train, _ = perf_harness.training_rows(player_frame, fmt, cutoff)
    matches = match_frame[match_frame.format_code == fmt]
    evaluation = matches[(matches.match_date >= cutoff) & (matches.match_date < end)]
    entry: Dict[str, Any] = {"cutoff": cutoff.date().isoformat(), "end": end.date().isoformat()}
    entry["n_eval_matches"] = {p: int(len(w)) for p, w in _split_by_population(evaluation).items()}
    entry["eval_populations_over_harness_floor"] = sorted(
        p for p, n in entry["n_eval_matches"].items() if n >= sim_harness.MIN_MATCHES
    )
    if not len(train):
        entry["skipped_reason"] = "no training rows"
        return entry
    _, calibration_rows = P._temporal_calibration_split(train)
    calibration = matches[matches.match_id.isin(set(calibration_rows.match_id))]
    calibration = calibration[simulator.complete_first_innings(calibration)]
    entry["n_calibration_matches"] = {p: int(len(w)) for p, w in _split_by_population(calibration).items()}
    entry["n_calibration_matches"]["all"] = int(len(calibration))
    lost = calibration[calibration[dn.C.TARGET_COL].to_numpy(dtype=float) == 1.0]
    entry["n_calibration_chases_lost"] = int(len(lost))
    entry["shared_factor_fits"] = len(calibration) >= simulator.MIN_SHARED_FACTOR_MATCHES
    entry["chase_dispersion_fits"] = bool(
        entry["shared_factor_fits"] and entry["n_calibration_chases_lost"] >= MIN_LOST_CHASES
    )
    entry["scale_populations_over_floor"] = sorted(
        p for p in POPULATIONS if entry["n_calibration_matches"][p] >= dn.SCALE_MIN_MATCHES
    )
    return entry


def _split_by_population(matches: pd.DataFrame) -> Dict[str, pd.DataFrame]:
    is_night = matches[dn.NIGHT_COL] == 1.0
    return {"day": matches[~is_night], "night": matches[is_night]}


def probe(player_frame: pd.DataFrame, match_frame: pd.DataFrame, formats: Sequence[str]) -> Dict[str, Any]:
    """The probe over every fold of every simulated format, and what it means per format."""
    out: Dict[str, Any] = {"formats": {}}
    for fmt in formats:
        folds = [probe_fold(player_frame, match_frame, fmt, cutoff, end) for cutoff, end in fold_windows()]
        decidable = [
            f
            for f in folds
            if f.get("chase_dispersion_fits") and set(f["eval_populations_over_harness_floor"]) == set(POPULATIONS)
        ]
        both_decidable = [f for f in decidable if set(f["scale_populations_over_floor"]) == set(POPULATIONS)]
        out["formats"][fmt] = {
            "folds": folds,
            "n_folds": len(folds),
            "chase_arm_decidable_folds": [f["cutoff"] for f in decidable],
            "both_arm_decidable_folds": [f["cutoff"] for f in both_decidable],
            "decided": fmt in DECIDED_FORMATS,
        }
        logger.info(
            "%s: %d folds, %d can decide the chase arm (both populations over the harness's %d-match floor and a "
            "chase dispersion fittable), %d can also decide the both arm (both populations over the %d-match scale "
            "floor)",
            fmt,
            len(folds),
            len(decidable),
            sim_harness.MIN_MATCHES,
            len(both_decidable),
            dn.SCALE_MIN_MATCHES,
        )
    return out


# --- one fold ----------------------------------------------------------------------------

#: The chase's own dispersion and its tails, beside the first innings' -- §8.13's summary
#: carried neither, and this gate decides on both innings.
EXTRA_KEYS = {
    "chase_dispersion_ratio": ("chase", "dispersion_ratio"),
    "chase_below_q10": ("chase", "below_q10"),
    "chase_above_q90": ("chase", "above_q90"),
}


def _summary(report: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """§8.13's summary of one ``evaluate_window``, plus the chase's dispersion and tails."""
    base = dn._summary(report)
    if base is None:
        return None
    totals = report["totals"]
    for key, (innings, field) in EXTRA_KEYS.items():
        base[key] = totals[innings].get(field)
    return base


#: Keys whose pooled figure is a mean over matches. A dispersion ratio is a ratio of spreads
#: and is not, so neither the first innings' nor the chase's is pooled.
POOLABLE = (*dn.POOLABLE, "chase_below_q10", "chase_above_q90")


def _pooled_summary(populations: Dict[str, Optional[Dict[str, Any]]]) -> Optional[Dict[str, Any]]:
    present = [s for s in populations.values() if s]
    if not present:
        return None
    out: Dict[str, Any] = {"n_matches": int(sum(s["n_matches"] for s in present))}
    for key in POOLABLE:
        weighted = [(s[key], s["n_matches"]) for s in present if s.get(key) is not None]
        out[key] = float(sum(v * w for v, w in weighted) / sum(w for _, w in weighted)) if weighted else None
    return out


def arm_calibrations(
    fitted: simulator.SimulatorCalibration,
    masks: Dict[str, np.ndarray],
) -> tuple[Dict[str, Dict[str, simulator.SimulatorCalibration]], Dict[str, Any]]:
    """Per arm and population, the calibration the fixtures of that population are simulated
    under, and what the fold records about how each was built."""
    pooled_factor = fitted.shared_factor
    scaled, scale_note = dn.arm_factors(dn.SCALE, pooled_factor, masks)
    # Both dispersion terms are fitted from the same calibration draws and the same pooled
    # factor, so the arms differ only in how the term they apply is drawn.
    independent = simulator.fit_chase_dispersion(fitted.chase_sample)
    correlated = simulator.fit_correlated_chase_dispersion(pooled_factor, fitted.chase_sample)
    rho = fitted.runs_balls_rho
    calibrations = {
        CONTROL: {p: simulator.SimulatorCalibration(rho, pooled_factor) for p in POPULATIONS},
        CHASE: {p: simulator.SimulatorCalibration(rho, pooled_factor, None, independent) for p in POPULATIONS},
        CORR: {p: simulator.SimulatorCalibration(rho, pooled_factor, None, correlated) for p in POPULATIONS},
        CORRBOTH: {p: simulator.SimulatorCalibration(rho, scaled[p], None, correlated) for p in POPULATIONS},
    }
    note = {
        "chase_dispersion": independent.as_dict(),
        "correlated_chase_dispersion": correlated.as_dict(),
        "shared_factor_scale": scale_note,
    }
    return calibrations, note


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
    windows = _split_by_population(eval_matches)
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
    displays = dn._display_models(train_matches)
    base_rate = float(train_matches[dn.C.TARGET_COL].mean())
    # One fit per fold: the members, the recalibration, the pooled shared factor and the
    # calibration fold's chase sample. The arms differ only in what multiplies the runs
    # draws, so nothing else may be refitted.
    model = P.fit_performance(train, fmt, P.default_spec(joint_format=joint, shared_factor=True), train_matches)
    fitted = model.simulation
    fold["fit_seconds"] = model.metadata["fit_seconds"]
    if fitted.shared_factor is None:
        fold["skipped"] = True
        fold["skipped_reason"] = "the calibration fold is too thin for a shared factor"
        return fold
    if int((~fitted.chase_sample.censored).sum()) < MIN_LOST_CHASES:
        fold["skipped"] = True
        fold["skipped_reason"] = "too few lost chases in the calibration fold for a residual scale"
        return fold
    night_by_id = dict(zip(match_frame.match_id.astype(str), match_frame[dn.NIGHT_COL]))
    masks = dn._population_masks(fitted.shared_factor.sample_read, night_by_id)
    fold["n_calibration_matches"] = {p: int(m.sum()) for p, m in masks.items()}
    calibrations, note = arm_calibrations(fitted, masks)
    fold["calibration"] = note
    for arm in ARMS:
        summaries: Dict[str, Optional[Dict[str, Any]]] = {}
        for population, window in windows.items():
            model.simulation = calibrations[arm][population]
            players = evaluation[evaluation.match_id.isin(window.match_id)]
            summaries[population] = _summary(
                sim_harness.evaluate_window(model, displays, window, players, fmt, base_rate, dn.SIM_SAMPLES)
            )
        fold[arm] = {**summaries, "all": _pooled_summary(summaries)}
        logger.info(
            "%s @ %s %-7s | first day %s/%s night %s/%s | chase day %s/%s night %s/%s | pooled ΔBrier %s",
            fmt,
            fold["cutoff"],
            arm,
            dn._f((summaries["day"] or {}).get("first_coverage_80"), "%.3f"),
            dn._f((summaries["day"] or {}).get("first_dispersion_ratio"), "%.3f"),
            dn._f((summaries["night"] or {}).get("first_coverage_80"), "%.3f"),
            dn._f((summaries["night"] or {}).get("first_dispersion_ratio"), "%.3f"),
            dn._f((summaries["day"] or {}).get("chase_coverage_80"), "%.3f"),
            dn._f((summaries["day"] or {}).get("chase_dispersion_ratio"), "%.3f"),
            dn._f((summaries["night"] or {}).get("chase_coverage_80"), "%.3f"),
            dn._f((summaries["night"] or {}).get("chase_dispersion_ratio"), "%.3f"),
            dn._f((fold[arm]["all"] or {}).get("delta_brier"), "%+.4f"),
        )
    model.simulation = fitted
    fold["seconds"] = round(time.perf_counter() - started, 1)
    return fold


# --- the verdict --------------------------------------------------------------------------


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


def _grows(delta: Dict[str, Any]) -> bool:
    """A **signed** no-degradation reading: the quantity grew by more than one fold-level
    standard error. §8.13's judgment call (2) asked the next gate of this family to state the
    sign it cares about, because a symmetric 'unchanged' clause has now failed three times on
    an improvement."""
    return (
        delta["mean"] is not None
        and delta["se"] is not None
        and delta["se"] == delta["se"]
        and delta["mean"] > abs(delta["se"])
    )


def verdict(
    folds: List[Dict[str, Any]],
    means: Dict[str, Dict[str, Any]],
    arm: str,
    decided: bool,
) -> Dict[str, Any]:
    """The gate's clauses for one arm on one format, paired per fold against the control."""
    distances = {
        ("first", "coverage"): (NOMINAL_COVERAGE, "first_coverage_80"),
        ("first", "dispersion"): (NOMINAL_DISPERSION, "first_dispersion_ratio"),
        ("chase", "coverage"): (NOMINAL_COVERAGE, "chase_coverage_80"),
        ("chase", "dispersion"): (NOMINAL_DISPERSION, "chase_dispersion_ratio"),
    }
    paired = {
        f"{innings}_{quantity}": {p: dn._paired(folds, arm, dn._distance_to(target, p, key)) for p in POPULATIONS}
        for (innings, quantity), (target, key) in distances.items()
    }
    brier = dn._paired(folds, arm, lambda f, a: dn._value(f, a, "all", "delta_brier"))
    control_width = means[CONTROL]["all"].get("first_width_80")
    arm_width = means[arm]["all"].get("first_width_80")
    checks = {
        f"{p}_{name}_moves_to_nominal": dn._shrinks(node[p]) for name, node in paired.items() for p in POPULATIONS
    }
    checks["pooled_first_width_not_inflated"] = (
        control_width is not None
        and arm_width is not None
        and arm_width <= control_width * (1.0 + FIRST_WIDTH_TOLERANCE)
    )
    # Signed: only a *growth* fails. A fall is the simulated P(win) moving toward the display
    # model's, which is an improvement and must not be read as a breach.
    checks["e2_not_degraded"] = (
        brier["mean"] is not None
        and brier["se"] == brier["se"]
        and not _grows(brier)
        and abs(means[arm]["all"].get("delta_brier") or 0.0) <= BRIER_TOLERANCE
    )
    return {
        "decided": decided,
        "checks": checks,
        "passes": all(checks.values()),
        "paired_distance_deltas": paired,
        "e2_delta": brier,
        "pooled_first_width": {"control": control_width, "arm": arm_width},
    }


# --- the driver ---------------------------------------------------------------------------


def _write(path: str, payload: Dict[str, Any]) -> None:
    with open(path, "w") as fh:
        json.dump(payload, fh, indent=2)


def run(player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, out: str) -> Dict[str, Any]:
    result: Dict[str, Any] = {
        "gates": {arm: gates.describe(GATE_IDS[arm]) for arm in (*CANDIDATES, *REFERENCE)},
        "format": fmt,
        "decided_format": fmt in DECIDED_FORMATS,
        "seeds": list(P.DEFAULT_SEEDS),
        "sim_samples": dn.SIM_SAMPLES,
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
    # The reference arm is scored on the same clauses so its numbers are comparable, but only
    # the candidates' verdicts can ship anything; §8.14 already recorded the reference's.
    result["verdicts"] = {
        arm: verdict(result["folds"], result["means"], arm, result["decided_format"] and arm in CANDIDATES)
        for arm in (*CANDIDATES, *REFERENCE)
    }
    _write(out, result)
    return result


COLUMNS = (
    ("matches/fold", "n_matches", "%.0f"),
    ("first coverage", "first_coverage_80", "%.3f"),
    ("first width", "first_width_80", "%.1f"),
    ("first dispersion", "first_dispersion_ratio", "%.3f"),
    ("chase coverage", "chase_coverage_80", "%.3f"),
    ("chase width", "chase_width_80", "%.1f"),
    ("chase dispersion", "chase_dispersion_ratio", "%.3f"),
    ("chase below q10", "chase_below_q10", "%.3f"),
    ("chase above q90", "chase_above_q90", "%.3f"),
    ("Δ Brier (E2)", "delta_brier", "%+.4f"),
)


def decide(paths: Sequence[str]) -> None:
    for arm in CANDIDATES:
        print(gates.describe(GATE_IDS[arm]))
        print()
    ships = {arm: True for arm in CANDIDATES}
    decided_any = False
    for path in paths:
        with open(path) as fh:
            payload = json.load(fh)
        fmt, means = payload["format"], payload["means"]
        scored = [f for f in payload["folds"] if not f.get("skipped")]
        print(
            f"### {fmt} -- the chase's dispersion, independent and correlated "
            f"({payload['sim_samples']} draws, {len(scored)} folds"
            f"{'' if payload['decided_format'] else ', reported not decided'})"
        )
        print()
        print("| arm | population | folds scored | " + " | ".join(name for name, _, _ in COLUMNS) + " | verdict |")
        print("|---|---|---:|" + "---:|" * len(COLUMNS) + "---|")
        for arm in ARMS:
            for population in ("all", *POPULATIONS):
                node = means[arm][population]
                cells = " | ".join(dn._f(node.get(key), pattern) for _, key, pattern in COLUMNS)
                cell = {CONTROL: "control", CHASE: "reference (§8.14's SIM-IN-chase)"}.get(arm, "")
                if arm in CANDIDATES and population == "all":
                    failed = [k for k, ok in payload["verdicts"][arm]["checks"].items() if not ok]
                    cell = "passes" if not failed else "fails: " + ", ".join(failed)
                print(f"| {arm} | {population} | {node['n_scored_folds']} | {cells} | {cell} |")
        print()
        for arm in (*CANDIDATES, *REFERENCE):
            v = payload["verdicts"][arm]
            for name, node in v["paired_distance_deltas"].items():
                readings = ", ".join(
                    f"{p} {dn._f(node[p]['mean'], '%+.4f')} ± {dn._f(node[p]['se'], '%.4f')} "
                    f"({node[p]['n_folds']} folds)"
                    for p in POPULATIONS
                )
                print(f"- {fmt} {arm}: paired {name} |Δ to nominal| -- {readings}")
            e2 = v["e2_delta"]
            print(
                f"- {fmt} {arm}: paired Δ E2 {dn._f(e2['mean'], '%+.4f')} ± {dn._f(e2['se'], '%.4f')} over "
                f"{e2['n_folds']} folds; pooled first-innings width "
                f"{dn._f(v['pooled_first_width']['control'], '%.1f')} → "
                f"{dn._f(v['pooled_first_width']['arm'], '%.1f')}"
            )
            if payload["decided_format"] and arm in CANDIDATES:
                decided_any = True
                ships[arm] = ships[arm] and v["passes"]
        print()
    print(
        "The display model and the performance model are the control's in every arm (one fit per fold, shared), so "
        "the display AUC and every headline pinball are identical by construction, not measured. The first innings' "
        "draws in the chase and corr arms are the control's draw for draw, so their first-innings clauses cannot "
        "pass: they are the isolating arms, registered as such. The chase arm is §8.14's, re-run for the paired "
        "contrast and not re-gated here."
    )
    print()
    if not decided_any:
        print(f"no verdict: none of these files is a deciding format ({', '.join(DECIDED_FORMATS)})")
        return
    for arm in CANDIDATES:
        print(f"{arm}: {'SHIPS' if ships[arm] else 'not shipped (a recorded null)'}")


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--frames", help="pickle written by sim_frame_cache.py")
    p.add_argument("--cricsheet-dir", help="directory of Cricsheet JSON files (the fixtures the labels are for)")
    p.add_argument("--geocoding", default=dn.DEFAULT_GEOCODING)
    p.add_argument("--format", choices=list(simulator.SIMULATED_FORMATS))
    p.add_argument("--probe", action="store_true", help="the feasibility probe only, over every simulated format")
    p.add_argument("--out", help="where the run's JSON goes (a re-run resumes from it)")
    p.add_argument("--decide", nargs="+", help="result files; prints the tables and the verdict")
    args = p.parse_args(argv)
    if args.decide:
        decide(args.decide)
        return 0
    if not (args.frames and args.cricsheet_dir):
        p.error("--frames and --cricsheet-dir are required")
    if not args.probe and not (args.format and args.out):
        p.error("--format and --out are required unless --probe")
    labels = dn.night_labels(args.cricsheet_dir, args.geocoding)
    player_frame, match_frame = load_frames(None, args.frames)
    player_frame, match_frame = dn.join_night(player_frame, labels), dn.join_night(match_frame, labels)
    if args.probe:
        result = probe(player_frame, match_frame, simulator.SIMULATED_FORMATS)
        if args.out:
            _write(args.out, result)
        print(json.dumps(result["formats"], indent=2, default=str))
        return 0
    for arm in CANDIDATES:
        print(gates.describe(GATE_IDS[arm]))
    run(player_frame, match_frame, args.format, args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
