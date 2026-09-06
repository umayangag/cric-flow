"""X-2's gates: the four weather families, one at a time (docs/EXTERNAL_DATA_PLAN.md § X-2).

The data is ``ml.weather``'s: the curated venue coordinates, the cached ERA5 days and the
session windows inferred per match, turned into ``features.WEATHER_FAMILIES`` and joined by
match id onto ``sim_frame_cache.py``'s frames. Every column is fixed before the first ball
(H-21). Three gates per family, on the walk-forward folds and never the locked window (H-19):

**(a) display** (``--display``): X-3's display arm -- the display model refitted per fold
and seed with the family's columns beyond XI_FEATURE_COLS + TEAM_CONTEXT_COLS, against the
same folds without them, on AUC beyond the seed noise with the swap-violation share held.

**(b) simulator** (``--simulate --format``): A-1's arm -- the performance model refitted
per fold with the family injected as a fixture-context family, the control's display
models, shared factor rule and simulator seeds held; the simulated first-innings and chase
10-90 coverage and width, split by the day/night flag (H-22) and pooled from the split by
match count, the dispersion ratio per split, and the headline pinballs.

**(c) E2** from the same simulations: Brier(simulated) - Brier(display) against the control.

``--decide`` prints the per-family tables and the verdict: a family ships only past all
three, (a) in every format and (b)/(c) in T20 and ODI. Expect nulls -- A-1 and A-2 found
neither venue context nor a chase response in the totals, which bounds what weather adds.

    python x2_weather_context.py --display --frames frames.pkl --cricsheet-dir ... --out x2_display.json
    python x2_weather_context.py --simulate --format T20 --frames frames.pkl --cricsheet-dir ... --out x2_sim_T20.json
    python x2_weather_context.py --decide x2_display.json x2_sim_T20.json x2_sim_ODI.json x2_sim_T20I.json
"""

from __future__ import annotations

import argparse
import json
import logging
import math
import os
import sys
import time
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from sim_frame_cache import load_frames  # noqa: E402
from x3_match_stakes import display_arm, stakes_feature_verdict  # noqa: E402

from ml.weather import archive, features, geocoding, sessions, venues  # noqa: E402
from ml.weather.backfill import DEFAULT_CACHE, DEFAULT_GEOCODING  # noqa: E402
from ml.xi import contract as C  # noqa: E402
from ml.xi import gates, perf_harness, sim_harness, simulator  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import DISPLAY_SEEDS, fold_windows  # noqa: E402
from ml.xi.train import _xy, make_display_model  # noqa: E402

logger = logging.getLogger("x2_weather_context")

FAMILIES: Tuple[str, ...] = tuple(features.WEATHER_FAMILIES)
GATE_IDS: Dict[str, str] = {
    "daynight": "X-2-daynight",
    "humidity_temperature": "X-2-humidity-temperature",
    "dew": "X-2-dew",
    "rain": "X-2-rain",
}
#: The formats gates (b) and (c) decide on; (a) decides in every format; the rest report.
DECIDED_FORMATS = ("T20", "ODI")
CONTROL = "none"
COVERAGE_TOLERANCE = 0.03
PINBALL_TOLERANCE = 0.005
E2_TOLERANCE = 0.01
SIM_SAMPLES = sim_harness.HARNESS_SAMPLES
HEADLINE_TARGETS = tuple(t.name for t in P.TARGETS if t.headline)
SPLITS = ("day", "night")
SIM_KEYS = (
    "first_bias",
    "first_coverage_80",
    "first_width_80",
    "first_dispersion_ratio",
    "chase_bias",
    "chase_coverage_80",
    "chase_width_80",
    "delta_brier",
)

# The performance model reads a fixture-context family by name (``contract.FIXTURE_CONTEXT_FAMILIES``);
# the weather families are registered under the same mechanism for this run only.
for _family, _cols in features.WEATHER_FAMILIES.items():
    C.FIXTURE_CONTEXT_FAMILIES[f"weather_{_family}"] = list(_cols)


# --- The join ---------------------------------------------------------------------------


def weather_rows(cricsheet_dir: str, geocoding_path: str, cache_path: str) -> pd.DataFrame:
    """One row per archived match: the family columns and the session rule that placed it."""
    fixtures, _ = venues.read_archive(cricsheet_dir)
    locations = geocoding.read_locations(geocoding_path)
    cache = archive.WeatherCache(cache_path)
    windows = sessions.assign(fixtures, {k: loc.country_code for k, loc in locations.items() if loc.mapped})
    rows = []
    for fixture in fixtures:
        entry = cache.get(fixture.venue_key, fixture.date)
        day = entry if isinstance(entry, archive.DayWeather) else None
        row = features.feature_row(windows[fixture.match_id], day)
        row["match_id"] = str(fixture.match_id)
        row["session_rule"] = windows[fixture.match_id].rule
        rows.append(row)
    frame = pd.DataFrame(rows)
    logger.info(
        "weather rows: %d matches, %.1f %% with pre-match readings, %.1f %% at night",
        len(frame),
        100 * frame[features.KNOWN_COL].mean(),
        100 * frame[features.NIGHT_COL].mean(),
    )
    return frame


def join_weather(frame: pd.DataFrame, weather: pd.DataFrame) -> pd.DataFrame:
    """The weather columns onto a frame by match id; a match the archive index lacks reads
    the unknown category."""
    out = frame.copy()
    out["match_id"] = out["match_id"].astype(str)
    merged = out.merge(weather[["match_id"] + features.WEATHER_COLS], on="match_id", how="left")
    for col in features.WEATHER_COLS:
        merged[col] = merged[col].fillna(0.0)
    return merged


# --- Gate (a): the display model --------------------------------------------------------


def run_display(match_frame: pd.DataFrame, player_frame: pd.DataFrame, out: str) -> Dict[str, Any]:
    control_cols = list(C.XI_FEATURE_COLS + C.TEAM_CONTEXT_COLS)
    result: Dict[str, Any] = {"gates": {f: gates.describe(GATE_IDS[f]) for f in FAMILIES}, "formats": {}}
    for format_code in C.FORMAT_CODES:
        format_frame = match_frame[match_frame.format_code == format_code]
        started = time.perf_counter()
        control = display_arm(format_frame, player_frame, format_code, control_cols)
        node: Dict[str, Any] = {"control": control, "families": {}}
        for family in FAMILIES:
            arm = display_arm(format_frame, player_frame, format_code, control_cols + features.WEATHER_FAMILIES[family])
            node["families"][family] = {"arm": arm, "verdict": stakes_feature_verdict(control, arm)}
            logger.info(
                "%-5s display %-22s AUC %+.4f ± %.4f (seed sd %.4f)",
                format_code,
                family,
                node["families"][family]["verdict"]["auc_delta"]["mean"] or 0.0,
                node["families"][family]["verdict"]["auc_delta"]["se"] or 0.0,
                node["families"][family]["verdict"]["control_seed_sd"] or 0.0,
            )
        node["seconds"] = round(time.perf_counter() - started, 1)
        result["formats"][format_code] = node
        _write(out, result)
    return result


# --- Gates (b) and (c): the performance model and the simulator ---------------------------


def _display_models(train_matches: pd.DataFrame) -> List[Any]:
    x, y = _xy(train_matches, C.DISPLAY_FEATURE_COLS)
    return [make_display_model(C.DISPLAY_FEATURE_COLS, seed).fit(x, y) for seed in DISPLAY_SEEDS]


def _simulation_summary(report: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    if "skipped_reason" in report:
        return None
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
    }


def _pooled(splits: Dict[str, Optional[Dict[str, Any]]]) -> Optional[Dict[str, Any]]:
    """Coverage, width, bias and E2's delta are means over matches, so the pooled figure is
    the match-count-weighted mean of the splits; the dispersion ratio is not, and is left
    to the splits."""
    present = [s for s in splits.values() if s]
    if not present:
        return None
    n = float(sum(s["n_matches"] for s in present))
    out: Dict[str, Any] = {"n_matches": int(n)}
    for key in SIM_KEYS:
        if key == "first_dispersion_ratio":
            continue
        values = [(s[key], s["n_matches"]) for s in present if s.get(key) is not None]
        out[key] = float(sum(v * w for v, w in values) / sum(w for _, w in values)) if values else None
    return out


def _performance_summary(targets: Dict[str, Dict]) -> Dict[str, Any]:
    return {"pinball": {name: targets[name]["model"]["pinball"] for name in HEADLINE_TARGETS}}


def run_fold(
    player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, cutoff, end, arms: Dict[str, Tuple[str, ...]]
) -> Dict[str, Any]:
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
        "n_night_matches": int(eval_matches[features.NIGHT_COL].sum()),
    }
    if len(train) < perf_harness.MIN_TRAIN_ROWS or len(evaluation) < perf_harness.MIN_EVAL_ROWS:
        fold["skipped"] = True
        return fold
    simulated = fmt in simulator.SIMULATED_FORMATS
    started = time.perf_counter()
    displays = _display_models(train_matches) if simulated else []
    base_rate = float(train_matches[C.TARGET_COL].mean())
    night = eval_matches[features.NIGHT_COL] == 1.0
    windows = {"day": eval_matches[~night], "night": eval_matches[night]}
    for arm, families in arms.items():
        spec = P.default_spec(joint_format=joint, shared_factor=simulated, fixture_context_families=families)
        model = P.fit_performance(train, fmt, spec, train_matches if simulated else None)
        entry: Dict[str, Any] = {
            "families": list(families),
            "n_features": len(spec.feature_cols),
            "performance": _performance_summary(perf_harness.score_targets(model, train, evaluation)),
            "fit_seconds": model.metadata["fit_seconds"],
        }
        if simulated:
            splits = {}
            for split, split_matches in windows.items():
                split_players = evaluation[evaluation.match_id.isin(split_matches.match_id)]
                splits[split] = _simulation_summary(
                    sim_harness.evaluate_window(
                        model, displays, split_matches, split_players, fmt, base_rate, SIM_SAMPLES
                    )
                )
            entry["simulation"] = {**splits, "all": _pooled(splits)}
        fold[arm] = entry
        pooled = (entry.get("simulation") or {}).get("all") or {}
        logger.info(
            "%s @ %s %-27s first cov %s width %s | chase cov %s | ΔBrier %s | runs pinball %.3f | %.0f s",
            fmt,
            fold["cutoff"],
            arm,
            _f(pooled.get("first_coverage_80"), "%.3f"),
            _f(pooled.get("first_width_80"), "%.1f"),
            _f(pooled.get("chase_coverage_80"), "%.3f"),
            _f(pooled.get("delta_brier"), "%+.4f"),
            entry["performance"]["pinball"]["runs"],
            entry["fit_seconds"],
        )
    fold["seconds"] = round(time.perf_counter() - started, 1)
    return fold


def run_simulate(player_frame: pd.DataFrame, match_frame: pd.DataFrame, fmt: str, out: str) -> Dict[str, Any]:
    arms: Dict[str, Tuple[str, ...]] = {CONTROL: ()}
    arms.update({family: (f"weather_{family}",) for family in FAMILIES})
    result: Dict[str, Any] = {
        "gates": {f: gates.describe(GATE_IDS[f]) for f in FAMILIES},
        "format": fmt,
        "decided_format": fmt in DECIDED_FORMATS,
        "seeds": list(P.DEFAULT_SEEDS),
        "sim_samples": SIM_SAMPLES,
        "arms": {arm: list(f) for arm, f in arms.items()},
        "folds": [],
    }
    if os.path.exists(out):
        with open(out) as fh:
            previous = json.load(fh)
        result["folds"] = previous.get("folds", [])
        logger.info("resuming %s from %d scored folds", out, len(result["folds"]))
    done = {f["cutoff"] for f in result["folds"]}
    for cutoff, end in fold_windows():
        if cutoff.date().isoformat() in done:
            continue
        result["folds"].append(run_fold(player_frame, match_frame, fmt, cutoff, end, arms))
        _write(out, result)
    result["means"] = arm_means(result["folds"], list(arms))
    result["verdicts"] = {family: simulator_verdict(result["folds"], result["means"], family) for family in FAMILIES}
    _write(out, result)
    return result


# --- Verdicts ---------------------------------------------------------------------------


def _mean(values: Sequence[Optional[float]]) -> Optional[float]:
    present = [float(v) for v in values if v is not None]
    return float(np.mean(present)) if present else None


def _paired(folds: List[Dict], arm: str, getter) -> Dict[str, Any]:
    values = []
    for fold in folds:
        if fold.get("skipped") or arm not in fold:
            continue
        a, b = getter(fold[arm]), getter(fold[CONTROL])
        if a is not None and b is not None:
            values.append(a - b)
    if not values:
        return {"mean": None, "se": None, "n_folds": 0}
    se = float(np.std(values, ddof=1) / math.sqrt(len(values))) if len(values) > 1 else float("nan")
    return {"mean": float(np.mean(values)), "se": se, "n_folds": len(values)}


def _sim_value(entry: Dict[str, Any], split: str, key: str) -> Optional[float]:
    node = (entry.get("simulation") or {}).get(split)
    return None if not node else node.get(key)


def arm_means(folds: List[Dict[str, Any]], arms: Sequence[str]) -> Dict[str, Dict[str, Any]]:
    scored = [f for f in folds if not f.get("skipped")]
    out: Dict[str, Dict[str, Any]] = {}
    for arm in arms:
        entries = [f[arm] for f in scored if arm in f]
        summary: Dict[str, Any] = {
            "n_folds": len(entries),
            "pinball": {name: _mean([e["performance"]["pinball"][name] for e in entries]) for name in HEADLINE_TARGETS},
        }
        if entries and entries[0].get("simulation"):
            for split in (*SPLITS, "all"):
                summary[split] = {key: _mean([_sim_value(e, split, key) for e in entries]) for key in SIM_KEYS}
                summary[split]["n_matches"] = _mean([_sim_value(e, split, "n_matches") for e in entries])
        out[arm] = summary
    return out


def simulator_verdict(folds: List[Dict], means: Dict[str, Dict[str, Any]], family: str) -> Dict[str, Any]:
    """Gates (b) and (c) for one family on one format: coverage holds and width does not
    grow on both splits and both innings; E2 moves by no more than one paired s.e. and
    stays within tolerance; no headline pinball worse by more than the tolerance."""
    control, candidate = means[CONTROL], means[family]
    if "all" not in candidate:
        return {"decided": False, "reason": "no simulator for this format"}
    checks: Dict[str, bool] = {}
    for split in SPLITS:
        for innings in ("first", "chase"):
            c_cov, a_cov = control[split][f"{innings}_coverage_80"], candidate[split][f"{innings}_coverage_80"]
            c_w, a_w = control[split][f"{innings}_width_80"], candidate[split][f"{innings}_width_80"]
            checks[f"{split}_{innings}_coverage_holds"] = (
                c_cov is not None and a_cov is not None and abs(a_cov - c_cov) <= COVERAGE_TOLERANCE
            )
            checks[f"{split}_{innings}_width_does_not_grow"] = c_w is not None and a_w is not None and a_w <= c_w
    e2 = _paired(folds, family, lambda e: _sim_value(e, "all", "delta_brier"))
    checks["e2_unchanged"] = (
        e2["mean"] is not None
        and e2["se"] == e2["se"]
        and abs(e2["mean"]) <= e2["se"]
        and (candidate["all"]["delta_brier"] or 0.0) <= E2_TOLERANCE
    )
    checks["pinball_no_worse"] = all(
        candidate["pinball"][t] <= control["pinball"][t] * (1.0 + PINBALL_TOLERANCE) for t in HEADLINE_TARGETS
    )
    return {
        "decided": True,
        "checks": checks,
        "passes": all(checks.values()),
        "e2_delta": e2,
        "coverage_delta": {
            f"{split}_{innings}": _paired(
                folds, family, lambda e, s=split, i=innings: _sim_value(e, s, f"{i}_coverage_80")
            )
            for split in SPLITS
            for innings in ("first", "chase")
        },
    }


# --- Driver -----------------------------------------------------------------------------


def _write(path: str, payload: Dict[str, Any]) -> None:
    with open(path, "w") as fh:
        json.dump(payload, fh, indent=2)


def _f(value: Optional[float], pattern: str) -> str:
    return "n/a" if value is None or value != value else pattern % value


def _frames(args) -> Tuple[pd.DataFrame, pd.DataFrame]:
    player_frame, match_frame = load_frames(None, args.frames)
    weather = weather_rows(args.cricsheet_dir, args.geocoding, args.cache)
    return join_weather(player_frame, weather), join_weather(match_frame, weather)


def decide(paths: Sequence[str]) -> None:
    display, sims = None, {}
    for path in paths:
        with open(path) as fh:
            payload = json.load(fh)
        if "format" in payload:
            sims[payload["format"]] = payload
        else:
            display = payload
    for family in FAMILIES:
        print(gates.describe(GATE_IDS[family]))
        print()
    ships: Dict[str, bool] = {}
    if display:
        print("### Gate (a) -- the family in the display model (walk-forward, 3 seeds)")
        print()
        print(
            "| format | family | display AUC (control) | with family | Δ ± se | seed sd | Brier Δ | swap share (control → arm) | verdict |"
        )
        print("|---|---|---:|---:|---|---:|---|---:|---|")
        for format_code, node in display["formats"].items():
            for family, fam in node["families"].items():
                v = fam["verdict"]
                ships[family] = ships.get(family, True) and v["ships"]
                print(
                    f"| {format_code} | {family} | {_f(v['control_auc'], '%.4f')} | {_f(v['arm_auc'], '%.4f')} | "
                    f"{_f(v['auc_delta']['mean'], '%+.4f')} ± {_f(v['auc_delta']['se'], '%.4f')} | "
                    f"{_f(v['control_seed_sd'], '%.4f')} | {_f(v['brier_delta']['mean'], '%+.4f')} | "
                    f"{_f(v['control_swap_violation_share'], '%.4f')} → {_f(v['swap_violation_share'], '%.4f')} | "
                    f"{'beyond noise' if v['beyond_noise'] else 'inside noise'}{'' if v['swap_within_h4'] else '; H-4 fails'} |"
                )
        print()
    for fmt, payload in sims.items():
        means = payload["means"]
        print(
            f"### Gates (b) and (c) -- {fmt}: the family in the performance model, simulated ({payload['sim_samples']} draws, {len(payload['folds'])} folds)"
        )
        print()
        print(
            "| arm | split | matches/fold | first bias | first coverage | first width | dispersion | chase bias | chase coverage | chase width | Δ Brier (E2) | runs pinball | wickets pinball | verdict |"
        )
        print("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|")
        for arm, m in means.items():
            if "all" not in m:
                continue
            verdict = payload["verdicts"].get(arm, {})
            for split in ("all", *SPLITS):
                s = m[split]
                if arm == CONTROL:
                    cell = "control"
                elif split != "all":
                    cell = ""
                else:
                    failed = [k for k, ok in verdict["checks"].items() if not ok]
                    cell = ("passes" if verdict["passes"] else "fails: " + ", ".join(failed)) + (
                        "" if payload["decided_format"] else " (reported)"
                    )
                print(
                    f"| {arm} | {split} | {_f(s.get('n_matches'), '%.0f')} | {_f(s.get('first_bias'), '%+.1f')} | "
                    f"{_f(s.get('first_coverage_80'), '%.3f')} | {_f(s.get('first_width_80'), '%.1f')} | "
                    f"{_f(s.get('first_dispersion_ratio'), '%.3f')} | {_f(s.get('chase_bias'), '%+.1f')} | "
                    f"{_f(s.get('chase_coverage_80'), '%.3f')} | {_f(s.get('chase_width_80'), '%.1f')} | "
                    f"{_f(s.get('delta_brier'), '%+.4f')} | {m['pinball']['runs']:.3f} | {m['pinball']['wickets']:.4f} | {cell} |"
                )
        print()
        for family in FAMILIES:
            v = payload["verdicts"][family]
            if not v.get("decided"):
                continue
            e2 = v["e2_delta"]
            cov = ", ".join(
                f"{k} {_f(d['mean'], '%+.3f')} ± {_f(d['se'], '%.3f')}" for k, d in v["coverage_delta"].items()
            )
            print(
                f"- {fmt} {family}: paired Δ E2 {_f(e2['mean'], '%+.4f')} ± {_f(e2['se'], '%.4f')} over {e2['n_folds']} folds; paired Δ coverage: {cov}"
            )
            if payload["decided_format"]:
                ships[family] = ships.get(family, True) and v["passes"]
        print()
    for family in FAMILIES:
        print(f"{family}: {'SHIPS' if ships.get(family) else 'not shipped (a recorded null)'}")


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--frames", help="pickle written by sim_frame_cache.py")
    p.add_argument(
        "--cricsheet-dir", help="directory of Cricsheet JSON files (the fixtures the windows are inferred for)"
    )
    p.add_argument("--geocoding", default=DEFAULT_GEOCODING)
    p.add_argument("--cache", default=DEFAULT_CACHE)
    p.add_argument("--out", help="where the run's JSON goes (a simulate run resumes from it)")
    p.add_argument("--display", action="store_true", help="gate (a) in every format")
    p.add_argument("--simulate", action="store_true", help="gates (b) and (c) for --format")
    p.add_argument("--format", choices=list(C.FORMAT_CODES))
    p.add_argument("--decide", nargs="+", help="result files; prints the tables and the verdict")
    args = p.parse_args(argv)
    if args.decide:
        decide(args.decide)
        return 0
    if not (args.frames and args.cricsheet_dir and args.out and (args.display or (args.simulate and args.format))):
        p.error("--frames, --cricsheet-dir, --out and one of --display / --simulate --format are required")
    for family in FAMILIES:
        print(gates.describe(GATE_IDS[family]))
    player_frame, match_frame = _frames(args)
    if args.display:
        run_display(match_frame, player_frame, args.out)
    else:
        run_simulate(player_frame, match_frame, args.format, args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
