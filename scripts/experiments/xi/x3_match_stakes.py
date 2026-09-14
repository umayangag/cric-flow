"""X-3's two gates, and the coverage figures that qualify both (docs/EXTERNAL_DATA_PLAN.md).

The derivation itself lives in ``ml.xi.stakes`` and runs inside the rating pass on both
sources. This script measures it and then uses it twice, on the walk-forward folds and
never on the locked window (H-19):

**Coverage** (``--coverage``). What the labels reach, per format and gender: the stage
vocabulary the archive spells, how many matches each label covers, and how many have a
reconstructible table. A derivation is worth exactly its coverage, so this prints first.

**Use 1, E5 hygiene** (gate ``X-3-e5``). §8.6 read T20's E5 failure as rotation noise --
"in domestic T20, much of a 1-3 player change is squad rotation". Dead rubbers and
knockouts are where rotation is heaviest, so E5 is re-run with those pairs excluded and,
separately, down-weighted. Nothing about the objective, the pairs or the bar's derivation
changes: only which pairs the measurement reads, and the bar is re-derived on each arm's
own pairs so a filter cannot clear the bar by shrinking the sample. It decides nothing --
it says whether the null survives the cleaning.

**Use 2, a stakes feature** (gate ``X-3-stakes``). The display model refitted per fold with
``STAKES_COLS`` added, against the same folds without them: display AUC beyond the seed
noise, and H-4's swap-violation share unchanged. Expect a null.

The display swap probe this script introduced now lives in
``ml.xi.selection_metrics.display_swap_monotonicity``, where the harness reports it per
fold as well (B-7); this script calls it rather than keeping a second copy.

    python x3_match_stakes.py --coverage --cricsheet-dir ../../../data/go-app/cricsheet \\
        --out x3_coverage.json
    python x3_match_stakes.py --frames frames.pkl --cricsheet-dir ... --out x3.json
    python x3_match_stakes.py --decide x3.json --coverage-json x3_coverage.json
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

from ml.xi import contract as C  # noqa: E402
from ml.xi import gates  # noqa: E402
from ml.xi import natural_experiment as ne  # noqa: E402
from ml.xi import stakes as stakes_module  # noqa: E402
from ml.xi.evaluate import (  # noqa: E402
    LOCKED_PREVIOUS_START,
    MIN_EVAL_ROWS,
    MIN_TRAIN_ROWS,
    SWAP_MAX_MATCHES,
    fold_windows,
)
from ml.xi.selection_metrics import display_swap_monotonicity  # noqa: E402
from ml.xi.sources import CricsheetJsonSource  # noqa: E402
from ml.xi.train import (  # noqa: E402
    _international_teams_from_config,
    _score_marginalised,
    _xy,
    make_display_model,
    make_objective_model,
)

logger = logging.getLogger("x3_match_stakes")

E5_GATE_ID = "X-3-e5"
FEATURE_GATE_ID = "X-3-stakes"
#: The format §8.6 blamed rotation for, and the one use 1 is about; the others are reported.
DECIDED_FORMAT = "T20"
#: The bar is re-derived under each of these seeds, as A-3's was; an arm must clear all three.
BAR_SEEDS = (ne.BAR_SEED, ne.BAR_SEED + 1, ne.BAR_SEED + 2)
#: H-4's line for the swap-violation share.
SWAP_VIOLATION_LIMIT = 0.02
#: The weight a down-weighted arm gives a rotation-suspect pair. Half, not a swept value:
#: the arm exists to show whether the reading moves at all when such pairs count for less.
DOWNWEIGHT = 0.5


# --- The pair filters (use 1's arms) ----------------------------------------------------


def _pair_flags(pairs: Sequence[ne.LineupPair], stakes: Dict[str, stakes_module.MatchStakes]) -> Dict[str, np.ndarray]:
    """Per pair: whether either of its two matches was a dead rubber, and whether either
    was a knockout. A pair is two consecutive matches of one side, and rotation in either
    of them is rotation in the comparison."""
    dead, knockout = [], []
    for pair in pairs:
        before = stakes.get(str(pair.before_match_id), stakes_module.UNLABELLED)
        after = stakes.get(str(pair.after_match_id), stakes_module.UNLABELLED)
        dead.append(before.dead_rubber or after.dead_rubber)
        knockout.append(before.is_knockout or after.is_knockout)
    return {"dead_rubber": np.asarray(dead, dtype=bool), "knockout": np.asarray(knockout, dtype=bool)}


#: The arms, as weights over the pairs: the control keeps every pair at 1, an "exclude" arm
#: drops the suspect pairs, and the down-weighted arm halves them.
ARMS: Tuple[str, ...] = (
    "all",
    "exclude_dead_rubber",
    "exclude_knockout",
    "exclude_both",
    "downweight_both",
)


def arm_weights(arm: str, flags: Dict[str, np.ndarray]) -> np.ndarray:
    suspect = {
        "all": np.zeros(len(flags["dead_rubber"]), dtype=bool),
        "exclude_dead_rubber": flags["dead_rubber"],
        "exclude_knockout": flags["knockout"],
        "exclude_both": flags["dead_rubber"] | flags["knockout"],
        "downweight_both": flags["dead_rubber"] | flags["knockout"],
    }[arm]
    weight = DOWNWEIGHT if arm == "downweight_both" else 0.0
    return np.where(suspect, weight, 1.0)


def weighted_agreement(scored: ne.ScoredPairs, weights: np.ndarray, seeds: Sequence[int] = BAR_SEEDS) -> Dict[str, Any]:
    """Sign agreement with each pair carrying ``weights``, and the bar derived on the same
    weights (A-3's ``reweighted_agreement``, which is the form this reuses).

    The bar has to move with the filter or the comparison is rigged: dropping pairs changes
    both the sample the objective is judged on and the sampling noise of the judgement, and
    only a bar re-derived on the surviving pairs carries both.
    """
    preference = scored.d_lineup != 0
    moved = scored.d_result != 0
    w = weights * preference
    up_claimed = scored.d_lineup > 0
    agreed = (up_claimed == (scored.d_result > 0)) & moved & preference
    denominator = float((w * moved).sum())
    if denominator <= 0.0:
        return {"agreement": None, "effective_pairs": 0.0, "bar_by_seed": {}, "passes_all_seeds": None}
    observed = float((w * agreed).sum() / denominator)
    a, b = scored.p_before_played, scored.p_after
    p_up, p_down = (1.0 - a) * b, a * (1.0 - b)
    expected = float((w * np.where(up_claimed, p_up, p_down)).sum() / (w * (p_up + p_down)).sum())
    bars: Dict[str, float] = {}
    for seed in seeds:
        rng = np.random.default_rng(seed)
        rates = []
        for _ in range(ne.BAR_REPLICATES):
            won_before, won_after = rng.random(len(a)) < a, rng.random(len(a)) < b
            sim_moved = won_before != won_after
            sim_agreed = ((won_after & ~won_before) == up_claimed) & sim_moved
            den = float((w * sim_moved).sum())
            if den > 0:
                rates.append(float((w * sim_agreed).sum() / den))
        bars[str(seed)] = float(np.quantile(rates, ne.BAR_QUANTILE))
    return {
        "agreement": observed,
        "effective_pairs": denominator,
        "pairs_kept": int((weights > 0).sum()),
        "expected_if_exactly_right": expected,
        "standard_error": float(math.sqrt(observed * (1 - observed) / denominator)) if denominator > 0 else None,
        "bar_by_seed": bars,
        "passes_all_seeds": bool(all(observed >= bar for bar in bars.values())),
        "median_abs_claimed_delta": float(np.median(np.abs(scored.d_lineup[preference]))) if preference.any() else None,
    }


# --- Use 1: E5 under each filter --------------------------------------------------------


def _fold_objectives(
    format_frame: pd.DataFrame,
) -> List[Tuple[pd.Timestamp, pd.Timestamp, Optional[ne.Proba], Dict[str, Any]]]:
    """One objective per fold, fitted strictly before the fold's cutoff -- the same model
    E5 in L4 scores its pairs with, refitted here so this script needs no run artifacts."""
    out = []
    for cutoff, end in fold_windows():
        train = format_frame[format_frame.match_date < cutoff]
        evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
        entry = {"cutoff": cutoff.date().isoformat(), "end": end.date().isoformat(), "n_train": int(len(train))}
        if len(train) < MIN_TRAIN_ROWS or train[C.TARGET_COL].nunique() < 2:
            entry["skipped_reason"] = "insufficient training rows"
            out.append((cutoff, end, None, entry))
            continue
        if len(evaluation) < MIN_EVAL_ROWS or evaluation[C.TARGET_COL].nunique() < 2:
            entry["skipped_reason"] = "evaluation window too small or single-class"
            out.append((cutoff, end, None, entry))
            continue
        x, y = _xy(train, C.XI_FEATURE_COLS)
        model = make_objective_model().fit(x, y)
        entry["objective_auc"] = _score_marginalised(model, evaluation, C.XI_FEATURE_COLS)["auc"]
        out.append((cutoff, end, _proba_of(model), entry))
    return out


def _proba_of(model) -> ne.Proba:
    """P(team1 wins) for a matrix of feature rows, bound to one fold's fitted model."""
    return lambda x: model.predict_proba(x)[:, 1]


def e5_by_arm(
    format_code: str,
    pairs: Sequence[ne.LineupPair],
    format_frame: pd.DataFrame,
    stakes: Dict[str, stakes_module.MatchStakes],
) -> Dict[str, Any]:
    """E5 pooled over the walk-forward folds, once per arm, on one format.

    The pairs, the objectives and the scoring are computed once and shared: an arm is a
    vector of weights over the very same scored pairs, which is what makes the arms
    comparable and the run cheap.
    """
    own = [p for p in pairs if p.format_code == format_code]
    folds = _fold_objectives(format_frame)
    scored_parts: List[ne.ScoredPairs] = []
    kept_pairs: List[ne.LineupPair] = []
    fold_reports: List[Dict[str, Any]] = []
    for cutoff, end, proba, entry in folds:
        window = [p for p in own if cutoff <= p.after_date < end]
        entry["pairs"] = len(window)
        fold_reports.append(entry)
        if proba is None or not window:
            continue
        scored = ne.score_pairs(window, proba, C.XI_FEATURE_COLS)
        entry["e5"] = ne.agreement(scored.d_lineup, scored.d_result)
        scored_parts.append(scored)
        kept_pairs.extend([p for p in window if p.before_side is not None])
    if not scored_parts:
        return {"folds": fold_reports, "arms": {}}
    pooled = ne._concat(scored_parts)
    flags = _pair_flags(kept_pairs, stakes)
    arms = {}
    for arm in ARMS:
        weights = arm_weights(arm, flags)
        entry = weighted_agreement(pooled, weights)
        entry["pairs_suspect"] = int((weights < 1.0).sum())
        arms[arm] = entry
    # The control restricted to the pairs the folds held before A-4 rotated the locked
    # window: the figure §8.6 and P-7 recorded came from that fold set, and reproducing it
    # is what says the arms above vary the filter rather than the window.
    before_rotation = np.asarray(
        [pair.after_date < pd.Timestamp(LOCKED_PREVIOUS_START) for pair in kept_pairs], dtype=float
    )
    return {
        "folds": fold_reports,
        "pairs_pooled": int(len(pooled)),
        "suspect_share": {
            "dead_rubber": float(flags["dead_rubber"].mean()),
            "knockout": float(flags["knockout"].mean()),
        },
        "arms": arms,
        "control_before_locked_rotation": weighted_agreement(pooled, before_rotation),
    }


# --- Use 2: the stakes feature in the display model -------------------------------------


def display_arm(format_frame: pd.DataFrame, player_frame: pd.DataFrame, format_code: str, columns: List[str]) -> Dict:
    """Walk-forward display AUC, Brier and the swap-violation share for one column set."""
    folds: List[Dict[str, Any]] = []
    format_players = player_frame[player_frame.format_code == format_code]
    for cutoff, end in fold_windows():
        train = format_frame[format_frame.match_date < cutoff]
        evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
        entry: Dict[str, Any] = {"cutoff": cutoff.date().isoformat(), "n_train": int(len(train))}
        if len(train) < MIN_TRAIN_ROWS or train[C.TARGET_COL].nunique() < 2:
            entry["skipped_reason"] = "insufficient training rows"
            folds.append(entry)
            continue
        if len(evaluation) < MIN_EVAL_ROWS or evaluation[C.TARGET_COL].nunique() < 2:
            entry["skipped_reason"] = "evaluation window too small or single-class"
            folds.append(entry)
            continue
        x, y = _xy(train, columns)
        model = make_display_model(columns).fit(x, y)
        scores = _score_marginalised(model, evaluation, columns)
        entry["display_auc_mean"] = scores["auc"]
        entry["display_brier_mean"] = scores["brier"]
        window_players = format_players[(format_players.match_date >= cutoff) & (format_players.match_date < end)]
        entry["swap_monotonicity"] = display_swap_monotonicity(
            model, columns, evaluation, window_players, format_code, max_matches=SWAP_MAX_MATCHES
        )
        folds.append(entry)
    return {"columns": columns, "folds": folds}


def _paired(a: List[Dict], b: List[Dict], key: str) -> Dict[str, Any]:
    values = np.asarray(
        [fa[key] - fb[key] for fa, fb in zip(a, b) if key in fa and key in fb and fa["cutoff"] == fb["cutoff"]],
        dtype=float,
    )
    if not len(values):
        return {"mean": None, "se": None, "n_folds": 0}
    se = float(np.std(values, ddof=1) / math.sqrt(len(values))) if len(values) > 1 else float("nan")
    return {"mean": float(values.mean()), "se": se, "n_folds": int(len(values))}


def _mean(folds: List[Dict], key: str) -> Optional[float]:
    values = [f[key] for f in folds if key in f]
    return float(np.mean(values)) if values else None


def _swap_mean(folds: List[Dict]) -> Optional[float]:
    shares = [f["swap_monotonicity"]["violation_share"] for f in folds if f.get("swap_monotonicity")]
    return float(np.mean(shares)) if shares else None


def stakes_feature_verdict(control: Dict, arm: Dict) -> Dict[str, Any]:
    """The gate: display AUC up by more than one fold-level standard error of the paired
    difference, in every format, with H-4's share still under 2 %. (The clause once also
    named the control's seed-to-seed sd; that spread was identically zero -- EVAL-02 -- so
    the standard error was always the floor that bound.)"""
    auc = _paired(arm["folds"], control["folds"], "display_auc_mean")
    swap = _swap_mean(arm["folds"])
    beyond_noise = auc["mean"] is not None and auc["se"] == auc["se"] and auc["mean"] > auc["se"]
    return {
        "auc_delta": auc,
        "control_swap_violation_share": _swap_mean(control["folds"]),
        "swap_delta_vs_control": None
        if swap is None or _swap_mean(control["folds"]) is None
        else swap - _swap_mean(control["folds"]),
        "control_auc": _mean(control["folds"], "display_auc_mean"),
        "arm_auc": _mean(arm["folds"], "display_auc_mean"),
        "brier_delta": _paired(arm["folds"], control["folds"], "display_brier_mean"),
        "swap_violation_share": swap,
        "swap_within_h4": swap is None or swap < SWAP_VIOLATION_LIMIT,
        "beyond_noise": bool(beyond_noise),
        "ships": bool(beyond_noise) and (swap is None or swap < SWAP_VIOLATION_LIMIT),
    }


# --- Coverage ---------------------------------------------------------------------------


def coverage_report(cricsheet_dir: str) -> Dict[str, Any]:
    """The label-coverage figures, measured over the archive the rating pass reads."""
    source = CricsheetJsonSource(cricsheet_dir, _international_teams_from_config())
    started = time.perf_counter()
    records = list(source.iter_matches())
    derived = {str(r.match_id): r.stakes for r in records}
    logger.info("coverage: %d matches parsed in %.0f s", len(records), time.perf_counter() - started)
    return {
        "matches": len(records),
        "counts": source.counts.as_dict(),
        "coverage": stakes_module.coverage(records, derived),
        "vocabulary": stakes_module.vocabulary(records),
        "edition_gap_days": stakes_module.EDITION_GAP_DAYS,
    }


# --- Driver -----------------------------------------------------------------------------


def run(frames_path: str, cricsheet_dir: str, pairs_cache: Optional[str], out: str) -> Dict[str, Any]:
    player_frame, match_frame = load_frames(None, frames_path)
    source = CricsheetJsonSource(cricsheet_dir, _international_teams_from_config())
    if pairs_cache and os.path.exists(pairs_cache):
        logger.info("pairs from cache %s", pairs_cache)
        pairs, stakes, parity = pd.read_pickle(pairs_cache)
    else:
        pairs = ne.build_pairs(match_frame, player_frame)
        parity = ne.score_previous_elevens(pairs, source)
        archive = CricsheetJsonSource(cricsheet_dir, _international_teams_from_config())
        stakes = {str(record.match_id): record.stakes for record in archive.iter_matches()}
        if pairs_cache:
            pd.to_pickle((pairs, stakes, parity), pairs_cache)

    e5: Dict[str, Any] = {}
    feature: Dict[str, Any] = {}
    for format_code in C.FORMAT_CODES:
        format_frame = match_frame[match_frame.format_code == format_code]
        started = time.perf_counter()
        e5[format_code] = e5_by_arm(format_code, pairs, format_frame, stakes)
        logger.info("%-5s E5 arms in %.0f s", format_code, time.perf_counter() - started)
        started = time.perf_counter()
        control = display_arm(format_frame, player_frame, format_code, list(C.XI_FEATURE_COLS + C.TEAM_CONTEXT_COLS))
        arm = display_arm(
            format_frame, player_frame, format_code, list(C.XI_FEATURE_COLS + C.TEAM_CONTEXT_COLS + C.STAKES_COLS)
        )
        feature[format_code] = {
            "control": control,
            "stakes": arm,
            "verdict": stakes_feature_verdict(control, arm),
        }
        logger.info("%-5s display arms in %.0f s", format_code, time.perf_counter() - started)

    result = {
        "gates": {E5_GATE_ID: gates.describe(E5_GATE_ID), FEATURE_GATE_ID: gates.describe(FEATURE_GATE_ID)},
        "decided_format": DECIDED_FORMAT,
        "bar_seeds": list(BAR_SEEDS),
        "downweight": DOWNWEIGHT,
        "cutoffs": [c.date().isoformat() for c, _ in fold_windows()],
        "pairs": parity,
        "e5_hygiene": e5,
        "stakes_feature": feature,
    }
    with open(out, "w") as fh:
        json.dump(result, fh, indent=2)
    logger.info("written %s", out)
    return result


def _f(value: Optional[float], pattern: str) -> str:
    return "n/a" if value is None or value != value else pattern % value


def decide(path: str, coverage_path: Optional[str]) -> None:
    with open(path) as fh:
        result = json.load(fh)
    for gate_id, line in result["gates"].items():
        print(line)
        print()
    if coverage_path:
        print_coverage(coverage_path)
    print("### Use 1 -- E5 under the pair filters (walk-forward, pooled; the bar re-derived per arm)")
    print()
    print("| format | arm | pairs kept | effective pairs | agreement ± se | exactly-right | bar (3 seeds) | verdict |")
    print("|---|---|---:|---:|---|---:|---|---|")
    for format_code, node in result["e5_hygiene"].items():
        for arm, entry in node.get("arms", {}).items():
            bars = entry.get("bar_by_seed", {})
            bar_text = "–".join(_f(x, "%.3f") for x in (min(bars.values()), max(bars.values()))) if bars else "n/a"
            verdict = (
                "n/a" if entry.get("passes_all_seeds") is None else ("passes" if entry["passes_all_seeds"] else "fails")
            )
            print(
                f"| {format_code} | {arm} | {entry.get('pairs_kept', 0)} | {_f(entry.get('effective_pairs'), '%.0f')} | "
                f"{_f(entry.get('agreement'), '%.3f')} ± {_f(entry.get('standard_error'), '%.3f')} | "
                f"{_f(entry.get('expected_if_exactly_right'), '%.3f')} | {bar_text} | {verdict} |"
            )
    print()
    print(
        "Suspect share of pooled pairs (either match of the pair), and the control on the "
        "pairs the folds held before A-4 rotated the locked window:"
    )
    for format_code, node in result["e5_hygiene"].items():
        share = node.get("suspect_share", {})
        before = node.get("control_before_locked_rotation", {})
        print(
            f"- {format_code}: dead rubber {_f(share.get('dead_rubber'), '%.3f')}, "
            f"knockout {_f(share.get('knockout'), '%.3f')} of {node.get('pairs_pooled', 0)} pairs; "
            f"pre-rotation control {_f(before.get('agreement'), '%.3f')} "
            f"over {_f(before.get('effective_pairs'), '%.0f')} pairs"
        )
    print()
    print("### Use 2 -- the stakes columns in the display model")
    print()
    print(
        "| format | display AUC (control) | with stakes | Δ ± se | Brier Δ | "
        "swap share (control → arm) | verdict |"
    )
    print("|---|---:|---:|---|---|---:|---|")
    for format_code, node in result["stakes_feature"].items():
        v = node["verdict"]
        print(
            f"| {format_code} | {_f(v['control_auc'], '%.4f')} | {_f(v['arm_auc'], '%.4f')} | "
            f"{_f(v['auc_delta']['mean'], '%+.4f')} ± {_f(v['auc_delta']['se'], '%.4f')} | "
            f"{_f(v['brier_delta']['mean'], '%+.4f')} | "
            f"{_f(v['control_swap_violation_share'], '%.4f')} → {_f(v['swap_violation_share'], '%.4f')} | "
            f"{'beyond noise' if v['beyond_noise'] else 'inside noise'}"
            f"{'' if v['swap_within_h4'] else '; H-4 fails'}{' → SHIPS' if v['ships'] else ''} |"
        )


def print_coverage(path: str) -> None:
    with open(path) as fh:
        report = json.load(fh)
    print(f"### Label coverage ({report['matches']} matches, editions split at a {report['edition_gap_days']}-day gap)")
    print()
    print("| scope | matches | stage known | group | knockout | final | bilateral | unlabelled | table known | dead |")
    print("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
    for scope, counts in report["coverage"].items():
        print(
            f"| {scope} | {counts['matches']} | {counts['stage_known']} "
            f"({100 * counts['stage_known'] / counts['matches']:.1f} %) | "
            f"{counts.get('label_group', 0)} | {counts.get('label_knockout', 0)} | {counts.get('label_final', 0)} | "
            f"{counts.get('label_bilateral', 0)} | {counts.get('label_unlabelled', 0)} | "
            f"{counts['dead_rubber_known']} ({100 * counts['dead_rubber_known'] / counts['matches']:.1f} %) | "
            f"{counts['dead_rubber']} |"
        )
    print()
    unlabelled = {k: v for k, v in report["vocabulary"].items() if not v["label"]}
    print(
        f"Stage spellings in the archive: {len(report['vocabulary'])}, "
        f"of which {len(unlabelled)} the vocabulary does not recognise "
        f"({sum(v['matches'] for v in unlabelled.values())} matches: {', '.join(sorted(unlabelled)) or 'none'})."
    )
    print()


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--frames", help="pickle written by sim_frame_cache.py")
    p.add_argument("--cricsheet-dir", help="directory of Cricsheet JSON files")
    p.add_argument("--pairs-cache", help="pickle of E5's pairs and the stakes table")
    p.add_argument("--out", help="where the run's JSON goes")
    p.add_argument("--coverage", action="store_true", help="measure label coverage and write it to --out")
    p.add_argument("--decide", help="print the tables from a finished run's JSON")
    p.add_argument("--coverage-json", help="a coverage run's JSON, printed with --decide")
    args = p.parse_args(argv)

    print(gates.describe(E5_GATE_ID))
    print(gates.describe(FEATURE_GATE_ID))
    if args.decide:
        decide(args.decide, args.coverage_json)
        return 0
    if args.coverage:
        if not (args.cricsheet_dir and args.out):
            p.error("--coverage needs --cricsheet-dir and --out")
        with open(args.out, "w") as fh:
            json.dump(coverage_report(args.cricsheet_dir), fh, indent=2)
        print_coverage(args.out)
        return 0
    if not (
        args.frames and args.out and (args.cricsheet_dir or (args.pairs_cache and os.path.exists(args.pairs_cache)))
    ):
        p.error("--frames, --out and --cricsheet-dir (or an existing --pairs-cache) are required to run")
    run(args.frames, args.cricsheet_dir, args.pairs_cache, args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
