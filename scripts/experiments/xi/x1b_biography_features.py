"""X-1b's two runnable gates, decided on the walk-forward folds and never on the locked
window (H-19). The third family, handedness / style matchups, has no labels to build
from (X-1a: a bowling style for 265 and a batting hand for 18 of 13,662 players) and is
recorded in the plan as not runnable rather than fitted to 1.9 % of the registry.

Family 1, gate ``X-1b-age`` -- does the performance model gain from the player's age at
the match date? Two arms per fold, ``none`` and ``age`` (``contract.AGE_COLS``), one frame,
everything else fixed; decided by the pinball loss of runs or wickets against the no-age
arm, beyond E1's 0.5 % band and one fold-level standard error, with the quantile
targets' coverage held (H-22). T20 is decided on men's rows: women's T20 has a date of
birth for 61.4 % of appearances and is out of scope, reported and never deciding.

Family 3, gate ``X-1b-cold-start`` -- should a debutant of known age start from his age
band's debut profile rather than the neutral vector? Two passes over the source (the
prior off and on), every model refitted per fold on each pass's frame; decided by H-10
staying bounded (the debutant-swap distribution against the control's), the pinball of
runs or wickets on the held-out debut rows, the check that no player with history moved,
and a no-degradation guard on the display AUC and H-4.

Both triples are registered in ``ml.xi.gates`` (H-23) and printed before anything runs.

    python x1b_biography_features.py --family age --frames frames_off.pkl --format T20 --out age_T20.json
    python x1b_biography_features.py --family cold-start --frames frames_off.pkl --frames-on frames_on.pkl \\
        --format T20 --out cold_T20.json [--age]
    python x1b_biography_features.py --decide age age_T20.json age_ODI.json ...
    python x1b_biography_features.py --decide cold-start cold_T20.json cold_ODI.json ...
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

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from sim_frame_cache import load_debut_tables, load_frames  # noqa: E402

from ml.xi import contract as C  # noqa: E402
from ml.xi import gates, perf_harness, perf_metrics, selection_metrics  # noqa: E402
from ml.xi import performance as P  # noqa: E402
from ml.xi.evaluate import SWAP_MAX_MATCHES, fold_windows  # noqa: E402
from ml.xi.ratings import DEBUT_PRIOR_KEYS, RatingState, aggregate_side, debut_prior_vectors  # noqa: E402
from ml.xi.selection_metrics import _objective_probability  # noqa: E402
from ml.xi.train import _score_marginalised, _xy, make_display_model, make_objective_model  # noqa: E402

logger = logging.getLogger("x1b_biography_features")

GATE_IDS = {"age": "X-1b-age", "cold-start": "X-1b-cold-start"}
DECIDED_FORMATS = ("T20", "ODI")
#: The targets the gates decide on, and the quantile targets whose coverage must hold.
DECIDING_TARGETS = ("runs", "wickets")
HEADLINE_TARGETS = tuple(t.name for t in P.TARGETS if t.headline)
QUANTILE_HEADLINES = tuple(t.name for t in P.TARGETS if t.headline and t.kind == "quantile")
#: E1's noise band: a pinball change smaller than this is not evidence.
PINBALL_NOISE = 0.005
COVERAGE_TOLERANCE = 0.03
#: H-10's bounds for family 3: how far the arm's debutant-swap distribution may sit from
#: the control's, at the median and at the 10th percentile.
H10_MEDIAN_TOLERANCE = 0.02
H10_P10_TOLERANCE = 0.03
H10_PROBE_AGES: Tuple[Optional[float], ...] = (19.0, 27.0, 34.0, None)
H10_MATCHES = 50
SWAP_LIMIT = 0.02
#: A player with at most this many earlier matches in the format is "low history".
LOW_HISTORY_MATCHES = 2
#: The rows a format's verdict is read on: men's rows in T20 (women's T20 is out of the
#: age family's scope on X-1a's 61.4 % coverage), every row elsewhere.
DECISION_SLICE = {"T20": "men"}

Slices = Dict[str, np.ndarray]


def row_slices(rows: pd.DataFrame, fmt: str) -> Slices:
    """Boolean masks over ``rows``: the population, the gender halves, the low-history
    slices, and the slice the format's verdict is read on."""
    everyone = np.ones(len(rows), dtype=bool)
    men = rows.gender.to_numpy() == C.GENDER_MALE
    career = rows.career.to_numpy(dtype=float)
    slices = {
        "all": everyone,
        "men": men,
        "women": ~men,
        "debut": career == 0,
        "low_history": career <= LOW_HISTORY_MATCHES,
    }
    slices["decision"] = slices[DECISION_SLICE.get(fmt, "all")]
    return slices


def _target_scores(prediction: Dict[str, Any], t: P.TargetSpec, rows: pd.DataFrame, mask: np.ndarray) -> Dict:
    """Pinball (and, for a quantile target, coverage and width) of one target's forecast
    on the masked rows, from the same quantiles the harness scores."""
    if mask.sum() == 0:
        return {"n": 0, "pinball": None, "coverage_80": None, "width_80": None}
    y = rows[t.name].to_numpy(dtype=float)[mask]
    quantiles = perf_harness.model_forecasts(prediction, t)["quantiles"][mask]
    out: Dict[str, Any] = {"n": int(mask.sum()), "pinball": perf_metrics.mean_pinball(y, quantiles)}
    if t.kind == "quantile":
        interval = perf_metrics.interval_stats(y, quantiles[:, 0], quantiles[:, 2])
        out["coverage_80"] = interval["coverage_80"]
        out["width_80"] = interval["width_80"]
    return out


def score_slices(model: P.PerformanceModels, evaluation: pd.DataFrame, slices: Slices) -> Dict[str, Dict]:
    """Per slice and fitted target, the pre-toss (served) forecast's scores."""
    prediction = model.predict_marginalised(evaluation)
    return {
        name: {t.name: _target_scores(prediction, t, evaluation, mask) for t in model.spec.target_specs}
        for name, mask in slices.items()
    }


def _window(player_frame: pd.DataFrame, fmt: str, cutoff, end) -> pd.DataFrame:
    return player_frame[
        (player_frame.format_code == fmt) & (player_frame.match_date >= cutoff) & (player_frame.match_date < end)
    ]


def _known_share(rows: pd.DataFrame, slices: Slices) -> Dict[str, Optional[float]]:
    known = rows.age_known.to_numpy(dtype=float)
    return {name: (float(known[mask].mean()) if mask.any() else None) for name, mask in slices.items()}


# --- family 1: age in the performance model ---------------------------------------------

AGE_ARMS = {"none": False, "age": True}


def run_age_fold(player_frame: pd.DataFrame, fmt: str, cutoff, end) -> Dict[str, Any]:
    train, joint = perf_harness.training_rows(player_frame, fmt, cutoff)
    evaluation = _window(player_frame, fmt, cutoff, end)
    fold: Dict[str, Any] = {
        "cutoff": cutoff.date().isoformat(),
        "end": end.date().isoformat(),
        "n_train": int(len(train)),
        "n_eval": int(len(evaluation)),
    }
    if len(train) < perf_harness.MIN_TRAIN_ROWS or len(evaluation) < perf_harness.MIN_EVAL_ROWS:
        fold["skipped"] = True
        return fold
    slices = row_slices(evaluation, fmt)
    fold["age_known_share"] = _known_share(evaluation, slices)
    fold["n_rows"] = {name: int(mask.sum()) for name, mask in slices.items()}
    started = time.perf_counter()
    for arm, age in AGE_ARMS.items():
        spec = P.default_spec(joint_format=joint, shared_factor=False, age=age)
        model = P.fit_performance(train, fmt, spec)
        fold[arm] = {
            "n_features": len(spec.feature_cols),
            "scores": score_slices(model, evaluation, slices),
            "fit_seconds": model.metadata["fit_seconds"],
        }
        decision = fold[arm]["scores"]["decision"]
        logger.info(
            "%s @ %s %-5s runs pinball %.4f (cov %.3f width %.1f) wickets %.4f | %.0f s",
            fmt,
            fold["cutoff"],
            arm,
            decision["runs"]["pinball"],
            decision["runs"]["coverage_80"],
            decision["runs"]["width_80"],
            decision["wickets"]["pinball"],
            fold[arm]["fit_seconds"],
        )
    fold["seconds"] = round(time.perf_counter() - started, 1)
    return fold


# --- family 3: the age-aware cold start ---------------------------------------------------


def _side_vectors_from_rows(rows: pd.DataFrame) -> Dict[str, np.ndarray]:
    return {name: rows[name].to_numpy(dtype=float) for name in C.PLAYER_VECTOR_KEYS}


def _neutral_vector(fmt: str) -> Dict[str, np.ndarray]:
    """What an unseen player reads from a state with the prior off: the cold start H-10
    measured, one element per key."""
    vectors = RatingState().side_vectors(fmt, ["__debutant__"])
    return {name: vectors[name] for name in C.PLAYER_VECTOR_KEYS}


def _debutant_vector(fmt: str, age: Optional[float], tables: Optional[Dict[str, np.ndarray]]) -> Dict[str, np.ndarray]:
    """The debutant the probe drafts in: neutral, or -- under the arm, for a known age --
    his band's profile from the end-of-pass tables, exactly as ``side_vectors`` reads it."""
    vector = _neutral_vector(fmt)
    if tables is None or age is None:
        return vector
    f = C.FORMAT_INDEX[fmt]
    prior = debut_prior_vectors(tables["debut_bat"][f], tables["debut_bowl"][f], C.age_band([age]))
    for key in DEBUT_PRIOR_KEYS:
        vector[key] = prior[key]
    return vector


def debutant_swap(
    objective, window_players: pd.DataFrame, fmt: str, tables: Optional[Dict[str, np.ndarray]]
) -> Dict[str, Dict[str, Optional[float]]]:
    """H-10's probe: in the first ``H10_MATCHES`` evaluation matches, replace team1's
    lowest-Elo player by a debutant and record the objective's delta p, per probe age."""
    deltas: Dict[str, List[float]] = {str(age): [] for age in H10_PROBE_AGES}
    for match_id in window_players.match_id.drop_duplicates().tolist()[:H10_MATCHES]:
        match = window_players[window_players.match_id == match_id]
        side1, side2 = match[match.side == 1], match[match.side == 2]
        if side1.empty or side2.empty:
            continue
        own, opponent = _side_vectors_from_rows(side1), aggregate_side(_side_vectors_from_rows(side2), fmt)
        base = _objective_probability(objective, C.XI_FEATURE_COLS, aggregate_side(own, fmt), opponent)
        replaced = int(np.argmin(own["pelo"]))
        for age in H10_PROBE_AGES:
            debutant = _debutant_vector(fmt, age, tables)
            swapped = {name: values.copy() for name, values in own.items()}
            for name in C.PLAYER_VECTOR_KEYS:
                swapped[name][replaced] = debutant[name][0]
            p = _objective_probability(objective, C.XI_FEATURE_COLS, aggregate_side(swapped, fmt), opponent)
            deltas[str(age)].append(p - base)
    out: Dict[str, Dict[str, Optional[float]]] = {}
    for age, values in deltas.items():
        if not values:
            out[age] = {"n": 0, "median": None, "p10": None, "p90": None}
            continue
        arr = np.asarray(values)
        out[age] = {
            "n": len(arr),
            "median": float(np.median(arr)),
            "p10": float(np.percentile(arr, 10)),
            "p90": float(np.percentile(arr, 90)),
        }
    return out


def vectors_untouched(frame_off: pd.DataFrame, frame_on: pd.DataFrame) -> Dict[str, Any]:
    """Check (c): every row with history has the same per-player vector under both
    passes; the prior may move only the debut rows' own vectors and, through the side
    aggregates, the rows of any side fielding a debutant of known age."""
    key = ["match_id", "side", "player_key"]
    keys = C.PLAYER_VECTOR_KEYS + C.PLAYER_ROLE_KEYS + C.PLAYER_SEQUENCE_KEYS
    left = frame_off.set_index(key)[keys].sort_index()
    right = frame_on.set_index(key)[keys].sort_index()
    if not left.index.equals(right.index):
        raise ValueError("the two passes produced different rows")
    history = left["career"].to_numpy(dtype=float) > 0
    difference = np.abs(left[keys].to_numpy(dtype=float) - right[keys].to_numpy(dtype=float))
    own_moved = difference.max(axis=1) > 0
    aggregates = [f"own_{s}" for s in C.SIDE_FEATURE_STEMS] + [f"opp_{s}" for s in C.SIDE_FEATURE_STEMS]
    side_difference = np.abs(
        frame_off.set_index(key)[aggregates].sort_index().to_numpy(dtype=float)
        - frame_on.set_index(key)[aggregates].sort_index().to_numpy(dtype=float)
    ).max(axis=1)
    return {
        "rows": int(len(left)),
        "rows_with_history": int(history.sum()),
        "history_vectors_max_abs_difference": float(difference[history].max()) if history.any() else 0.0,
        "debut_rows_whose_vector_moved": int((own_moved & ~history).sum()),
        "debut_rows": int((~history).sum()),
        "rows_whose_side_aggregates_moved": int((side_difference > 0).sum()),
    }


def run_cold_start_fold(
    frames: Dict[str, Tuple[pd.DataFrame, pd.DataFrame]],
    tables: Dict[str, Optional[Dict[str, np.ndarray]]],
    fmt: str,
    cutoff,
    end,
    age: bool,
) -> Dict[str, Any]:
    fold: Dict[str, Any] = {"cutoff": cutoff.date().isoformat(), "end": end.date().isoformat()}
    started = time.perf_counter()
    for arm, (player_frame, match_frame) in frames.items():
        train, joint = perf_harness.training_rows(player_frame, fmt, cutoff)
        evaluation = _window(player_frame, fmt, cutoff, end)
        matches = match_frame[match_frame.format_code == fmt]
        train_matches = matches[matches.match_date < cutoff]
        eval_matches = matches[(matches.match_date >= cutoff) & (matches.match_date < end)]
        entry: Dict[str, Any] = {
            "n_train": int(len(train)),
            "n_eval": int(len(evaluation)),
            "n_train_matches": int(len(train_matches)),
            "n_eval_matches": int(len(eval_matches)),
        }
        if (
            len(train) < perf_harness.MIN_TRAIN_ROWS
            or len(evaluation) < perf_harness.MIN_EVAL_ROWS
            or len(train_matches) < 50
            or len(eval_matches) < 20
            or eval_matches[C.TARGET_COL].nunique() < 2
        ):
            fold["skipped"] = True
            fold[arm] = entry
            return fold
        x_objective, y = _xy(train_matches, C.XI_FEATURE_COLS)
        x_display, _ = _xy(train_matches, C.DISPLAY_FEATURE_COLS)
        objective = make_objective_model(C.XI_FEATURE_COLS).fit(x_objective, y)
        display = make_display_model(C.DISPLAY_FEATURE_COLS).fit(x_display, y)
        entry["objective_auc"] = _score_marginalised(objective, eval_matches, C.XI_FEATURE_COLS)["auc"]
        entry["display_auc"] = _score_marginalised(display, eval_matches, C.DISPLAY_FEATURE_COLS)["auc"]
        swap = selection_metrics.swap_monotonicity(
            objective, C.XI_FEATURE_COLS, evaluation, fmt, max_matches=SWAP_MAX_MATCHES
        )
        entry["swap_violation_share"] = None if swap is None else swap["violation_share"]
        entry["debutant_swap"] = debutant_swap(objective, evaluation, fmt, tables[arm])
        slices = row_slices(evaluation, fmt)
        entry["n_rows"] = {name: int(mask.sum()) for name, mask in slices.items()}
        spec = P.default_spec(joint_format=joint, shared_factor=False, age=age, targets=DECIDING_TARGETS)
        model = P.fit_performance(train, fmt, spec)
        entry["scores"] = score_slices(model, evaluation, slices)
        entry["fit_seconds"] = model.metadata["fit_seconds"]
        fold[arm] = entry
        debut = entry["scores"]["debut"]
        logger.info(
            "%s @ %s %-9s display AUC %.3f swap %s | debut rows %d runs pinball %s wickets %s | swap dp median %s",
            fmt,
            fold["cutoff"],
            arm,
            entry["display_auc"],
            _fmt(entry["swap_violation_share"], "%.4f"),
            debut["runs"]["n"],
            _fmt(debut["runs"]["pinball"], "%.4f"),
            _fmt(debut["wickets"]["pinball"], "%.4f"),
            {age_: _fmt(v["median"], "%+.4f") for age_, v in entry["debutant_swap"].items()},
        )
    fold["seconds"] = round(time.perf_counter() - started, 1)
    return fold


# --- the verdicts --------------------------------------------------------------------------


def _fmt(value: Optional[float], pattern: str) -> str:
    return "n/a" if value is None else pattern % value


def paired(control: Sequence[Optional[float]], arm: Sequence[Optional[float]]) -> Dict[str, Optional[float]]:
    """The improvement ``control - arm`` paired over folds: its mean, standard error and
    size relative to the control's mean. Folds where either side is missing are dropped."""
    pairs = [(c, a) for c, a in zip(control, arm) if c is not None and a is not None]
    if not pairs:
        return {"n_folds": 0, "mean": None, "se": None, "relative": None}
    diff = np.asarray([c - a for c, a in pairs])
    control_mean = float(np.mean([c for c, _ in pairs]))
    se = float(np.std(diff, ddof=1) / np.sqrt(len(diff))) if len(diff) > 1 else float("nan")
    return {
        "n_folds": len(diff),
        "mean": float(diff.mean()),
        "se": se,
        "relative": float(diff.mean() / control_mean) if control_mean else None,
        "control_mean": control_mean,
        "arm_mean": float(np.mean([a for _, a in pairs])),
    }


def improves_beyond_noise(stats: Dict[str, Optional[float]]) -> bool:
    return (
        stats["mean"] is not None
        and stats["relative"] is not None
        and stats["relative"] > PINBALL_NOISE
        and stats["mean"] > stats["se"]
    )


def _per_fold(folds: List[Dict], arm: str, *path: str) -> List[Optional[float]]:
    out = []
    for fold in folds:
        node: Any = fold.get(arm)
        for key in path:
            node = None if node is None else node.get(key)
        out.append(node)
    return out


def _means(values: Sequence[Optional[float]]) -> Optional[float]:
    present = [float(v) for v in values if v is not None]
    return float(np.mean(present)) if present else None


def age_verdict(folds: List[Dict], slice_name: str = "decision") -> Dict[str, Any]:
    """Family 1's rule on one format: runs or wickets pinball improves beyond noise, and
    every quantile headline target's coverage holds, on the format's decision slice."""
    scored = [f for f in folds if not f.get("skipped")]
    pinball = {
        target: paired(
            _per_fold(scored, "none", "scores", slice_name, target, "pinball"),
            _per_fold(scored, "age", "scores", slice_name, target, "pinball"),
        )
        for target in HEADLINE_TARGETS
    }
    coverage = {}
    for target in QUANTILE_HEADLINES:
        control = _means(_per_fold(scored, "none", "scores", slice_name, target, "coverage_80"))
        arm = _means(_per_fold(scored, "age", "scores", slice_name, target, "coverage_80"))
        coverage[target] = {
            "control": control,
            "age": arm,
            "width_control": _means(_per_fold(scored, "none", "scores", slice_name, target, "width_80")),
            "width_age": _means(_per_fold(scored, "age", "scores", slice_name, target, "width_80")),
            "holds": control is not None and arm is not None and abs(arm - control) <= COVERAGE_TOLERANCE,
        }
    improved = {target: improves_beyond_noise(pinball[target]) for target in DECIDING_TARGETS}
    coverage_holds = all(entry["holds"] for entry in coverage.values())
    return {
        "slice": slice_name,
        "n_folds": len(scored),
        "pinball": pinball,
        "coverage": coverage,
        "improves": improved,
        "coverage_holds": coverage_holds,
        "passes": any(improved.values()) and coverage_holds,
    }


def cold_start_verdict(folds: List[Dict], untouched: Dict[str, Any], slice_name: str = "decision") -> Dict[str, Any]:
    """Family 3's rule on one format: H-10 bounded against the control, debut-row pinball
    improves beyond noise, no player with history moved, and the guard holds."""
    scored = [f for f in folds if not f.get("skipped")]
    h10 = {}
    for age in H10_PROBE_AGES:
        key = str(age)
        control_median = _means(_per_fold(scored, "none", "debutant_swap", key, "median"))
        arm_median = _means(_per_fold(scored, "age_prior", "debutant_swap", key, "median"))
        control_p10 = _means(_per_fold(scored, "none", "debutant_swap", key, "p10"))
        arm_p10 = _means(_per_fold(scored, "age_prior", "debutant_swap", key, "p10"))
        bounded = (
            None not in (control_median, arm_median, control_p10, arm_p10)
            and abs(arm_median - control_median) <= H10_MEDIAN_TOLERANCE
            and abs(arm_p10 - control_p10) <= H10_P10_TOLERANCE
        )
        h10[key] = {
            "control_median": control_median,
            "arm_median": arm_median,
            "control_p10": control_p10,
            "arm_p10": arm_p10,
            "control_p90": _means(_per_fold(scored, "none", "debutant_swap", key, "p90")),
            "arm_p90": _means(_per_fold(scored, "age_prior", "debutant_swap", key, "p90")),
            "bounded": bounded,
        }
    debut_pinball = {
        target: paired(
            _per_fold(scored, "none", "scores", "debut", target, "pinball"),
            _per_fold(scored, "age_prior", "scores", "debut", target, "pinball"),
        )
        for target in DECIDING_TARGETS
    }
    population_pinball = {
        target: paired(
            _per_fold(scored, "none", "scores", slice_name, target, "pinball"),
            _per_fold(scored, "age_prior", "scores", slice_name, target, "pinball"),
        )
        for target in DECIDING_TARGETS
    }
    display = paired(
        # sign: paired() computes control - arm, so a *negative* mean is the arm ahead
        _per_fold(scored, "none", "display_auc"),
        _per_fold(scored, "age_prior", "display_auc"),
    )
    display_guard = display["mean"] is None or display["mean"] <= max(display["se"], 0.0)
    swap_share = _means(_per_fold(scored, "age_prior", "swap_violation_share"))
    swap_guard = swap_share is None or swap_share < SWAP_LIMIT
    improved = {target: improves_beyond_noise(debut_pinball[target]) for target in DECIDING_TARGETS}
    checks = {
        "h10_bounded": all(entry["bounded"] for entry in h10.values()),
        "debut_pinball_improves": any(improved.values()),
        "history_untouched": untouched["history_vectors_max_abs_difference"] == 0.0,
        "display_auc_guard": display_guard,
        "swap_guard": swap_guard,
    }
    return {
        "slice": slice_name,
        "n_folds": len(scored),
        "h10": h10,
        "debut_pinball": debut_pinball,
        "population_pinball": population_pinball,
        "improves": improved,
        "display_auc": display,
        "swap_violation_share": swap_share,
        "untouched": untouched,
        "checks": checks,
        "passes": all(checks.values()),
    }


# --- drivers -------------------------------------------------------------------------------


def _resume_folds(out: str) -> List[Dict[str, Any]]:
    """The folds an interrupted run already wrote to ``out``, so a re-run pays only for
    the fold that was in flight. Every fold is written as soon as it is scored."""
    if not os.path.exists(out):
        return []
    with open(out) as fh:
        folds = json.load(fh).get("folds", [])
    if folds:
        logger.info("resuming %s: %d folds already scored (through %s)", out, len(folds), folds[-1]["cutoff"])
    return folds


def _run_folds(out: str, header: Dict[str, Any], score_fold, verdicts) -> Dict[str, Any]:
    folds = _resume_folds(out)
    done = {f["cutoff"] for f in folds}
    for cutoff, end in fold_windows():
        if cutoff.date().isoformat() in done:
            continue
        folds.append(score_fold(cutoff, end))
        _write({**header, "cutoffs": [f["cutoff"] for f in folds], "folds": folds, "complete": False}, out)
    result = {**header, "cutoffs": [f["cutoff"] for f in folds], "folds": folds, "complete": True}
    result["verdicts"] = verdicts(folds)
    _write(result, out)
    return result


def run_age(frames_path: str, fmt: str, out: str) -> Dict[str, Any]:
    player_frame, _ = load_frames(None, frames_path)
    header = {
        "gate": gates.describe(GATE_IDS["age"]),
        "family": "age",
        "format": fmt,
        "decided_format": fmt in DECIDED_FORMATS,
        "decision_slice": DECISION_SLICE.get(fmt, "all"),
        "seeds": list(P.DEFAULT_SEEDS),
    }
    return _run_folds(
        out,
        header,
        lambda cutoff, end: run_age_fold(player_frame, fmt, cutoff, end),
        lambda folds: {
            name: age_verdict(folds, name) for name in ("decision", "all", "men", "women", "debut", "low_history")
        },
    )


def run_cold_start(frames_off: str, frames_on: str, fmt: str, out: str, age: bool) -> Dict[str, Any]:
    frames = {"none": load_frames(None, frames_off), "age_prior": load_frames(None, frames_on)}
    tables = {"none": None, "age_prior": load_debut_tables(frames_on)}
    untouched = vectors_untouched(
        frames["none"][0][frames["none"][0].format_code == fmt],
        frames["age_prior"][0][frames["age_prior"][0].format_code == fmt],
    )
    logger.info("%s: %s", fmt, untouched)
    header = {
        "gate": gates.describe(GATE_IDS["cold-start"]),
        "family": "cold-start",
        "format": fmt,
        "decided_format": fmt in DECIDED_FORMATS,
        "decision_slice": DECISION_SLICE.get(fmt, "all"),
        "performance_reads_age": age,
        "seeds": list(P.DEFAULT_SEEDS),
        "probe_ages": [str(a) for a in H10_PROBE_AGES],
        "untouched": untouched,
    }
    return _run_folds(
        out,
        header,
        lambda cutoff, end: run_cold_start_fold(frames, tables, fmt, cutoff, end, age),
        lambda folds: {name: cold_start_verdict(folds, untouched, name) for name in ("decision", "all")},
    )


def _write(result: Dict[str, Any], out: str) -> None:
    with open(out, "w") as fh:
        json.dump(result, fh, indent=2)
    logger.info("written %s", out)


def _load(paths: Sequence[str]) -> Dict[str, Dict[str, Any]]:
    results = {}
    for path in paths:
        with open(path) as fh:
            result = json.load(fh)
        results[result["format"]] = result
    return results


def decide_age(paths: Sequence[str]) -> bool:
    results = _load(paths)
    print(gates.describe(GATE_IDS["age"]))
    print()
    header = (
        "| format | slice | folds | rows | age known | runs pinball none → age (Δ ± se, rel) | wickets pinball none → age (Δ ± se, rel) "
        "| balls pinball | conceded pinball | runs coverage none → age | runs width | balls coverage | conceded coverage | verdict |"
    )
    print(header)
    print("|" + "---|" * (header.count("|") - 1))
    for fmt, result in results.items():
        scored = [f for f in result["folds"] if not f.get("skipped")]
        for name in ("all", "men", "women", "debut", "low_history"):
            v = result["verdicts"][name]
            rows = sum(f["n_rows"][name] for f in scored)
            known = _means([f["age_known_share"][name] for f in scored])
            if name == result["decision_slice"]:
                verdict = ("passes" if v["passes"] else "fails") + ("" if result["decided_format"] else " (reported)")
                name = f"{name} (decides)"
            else:
                verdict = "reported"
            print(_age_row(fmt, name, v, rows, known, verdict))
    kept = all(results[fmt]["verdicts"]["decision"]["passes"] for fmt in DECIDED_FORMATS if fmt in results) and all(
        fmt in results for fmt in DECIDED_FORMATS
    )
    print()
    print(f"age family: {'KEPT' if kept else 'not kept'}")
    return kept


def _age_row(fmt: str, name: str, v: Dict, rows: int, known: Optional[float], verdict: str) -> str:
    def pin(target: str) -> str:
        s = v["pinball"][target]
        if s["mean"] is None:
            return "n/a"
        return f"{s['control_mean']:.4f} → {s['arm_mean']:.4f} ({s['mean']:+.4f} ± {s['se']:.4f}, {100 * s['relative']:+.2f} %)"

    def cov(target: str) -> str:
        c = v["coverage"][target]
        return "n/a" if c["control"] is None else f"{c['control']:.3f} → {c['age']:.3f}"

    runs_width = v["coverage"]["runs"]
    width = (
        "n/a"
        if runs_width["width_control"] is None
        else f"{runs_width['width_control']:.1f} → {runs_width['width_age']:.1f}"
    )
    return (
        f"| {fmt} | {name} | {v['n_folds']} | {rows:,} | {_fmt(known, '%.3f')} | {pin('runs')} | {pin('wickets')} | "
        f"{pin('balls_faced')} | {pin('runs_conceded')} | {cov('runs')} | {width} | {cov('balls_faced')} | "
        f"{cov('runs_conceded')} | {verdict} |"
    )


def decide_cold_start(paths: Sequence[str]) -> bool:
    results = _load(paths)
    print(gates.describe(GATE_IDS["cold-start"]))
    print()
    header = (
        "| format | folds | debut rows | history vectors max diff | debut vectors moved | rows whose side aggregates moved "
        "| display AUC none → prior (Δ ± se) | swap share (prior) | debut runs pinball none → prior (Δ ± se, rel) "
        "| debut wickets pinball none → prior (Δ ± se, rel) | population runs pinball | population wickets pinball | verdict |"
    )
    print(header)
    print("|" + "---|" * (header.count("|") - 1))
    for fmt, result in results.items():
        v = result["verdicts"]["decision"]
        u = v["untouched"]
        scored = [f for f in result["folds"] if not f.get("skipped")]
        debut_rows = sum(f["age_prior"]["n_rows"]["debut"] for f in scored)
        d = v["display_auc"]

        def pin(node: Dict) -> str:
            if node["mean"] is None:
                return "n/a"
            return f"{node['control_mean']:.4f} → {node['arm_mean']:.4f} ({node['mean']:+.4f} ± {node['se']:.4f}, {100 * node['relative']:+.2f} %)"

        failed = [k for k, ok in v["checks"].items() if not ok]
        verdict = "passes" if v["passes"] else "fails: " + ", ".join(failed)
        if not result["decided_format"]:
            verdict = "reported: " + verdict
        print(
            f"| {fmt} | {v['n_folds']} | {debut_rows:,} | {u['history_vectors_max_abs_difference']:.3g} | "
            f"{u['debut_rows_whose_vector_moved']:,} of {u['debut_rows']:,} | {u['rows_whose_side_aggregates_moved']:,} of {u['rows']:,} | "
            f"{d['control_mean']:.3f} → {d['arm_mean']:.3f} ({-d['mean']:+.4f} ± {d['se']:.4f}) | {_fmt(v['swap_violation_share'], '%.4f')} | "
            f"{pin(v['debut_pinball']['runs'])} | {pin(v['debut_pinball']['wickets'])} | "
            f"{pin(v['population_pinball']['runs'])} | {pin(v['population_pinball']['wickets'])} | {verdict} |"
        )
    print()
    print("| format | probe age | control Δp median / p10 / p90 | prior Δp median / p10 / p90 | bounded |")
    print("|---|---|---|---|---|")
    for fmt, result in results.items():
        for age, h in result["verdicts"]["decision"]["h10"].items():
            print(
                f"| {fmt} | {age} | {_fmt(h['control_median'], '%+.4f')} / {_fmt(h['control_p10'], '%+.4f')} / {_fmt(h['control_p90'], '%+.4f')} | "
                f"{_fmt(h['arm_median'], '%+.4f')} / {_fmt(h['arm_p10'], '%+.4f')} / {_fmt(h['arm_p90'], '%+.4f')} | "
                f"{'yes' if h['bounded'] else 'no'} |"
            )
    kept = all(results[fmt]["verdicts"]["decision"]["passes"] for fmt in DECIDED_FORMATS if fmt in results) and all(
        fmt in results for fmt in DECIDED_FORMATS
    )
    print()
    print(f"age-aware cold start: {'KEPT' if kept else 'not kept'}")
    return kept


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--family", choices=list(GATE_IDS))
    p.add_argument("--frames", help="pickle from sim_frame_cache.py (the prior off)")
    p.add_argument("--frames-on", help="pickle from sim_frame_cache.py --age-aware-cold-start (family 3)")
    p.add_argument("--format", choices=list(C.FORMAT_CODES))
    p.add_argument("--out")
    p.add_argument(
        "--age", action="store_true", help="family 3: the performance model reads AGE_COLS (family 1's verdict)"
    )
    p.add_argument(
        "--decide", nargs="+", help="<family> then the per-format result files; prints the table and the verdict"
    )
    args = p.parse_args()
    if args.decide:
        family, paths = args.decide[0], args.decide[1:]
        (decide_age if family == "age" else decide_cold_start)(paths)
        return 0
    if not (args.family and args.frames and args.format and args.out):
        p.error("--family, --frames, --format and --out are required to run a format")
    print(gates.describe(GATE_IDS[args.family]))
    if args.family == "age":
        run_age(args.frames, args.format, args.out)
    else:
        if not args.frames_on:
            p.error("--frames-on is required for the cold-start family")
        run_cold_start(args.frames, args.frames_on, args.format, args.out, args.age)
    return 0


if __name__ == "__main__":
    sys.exit(main())
