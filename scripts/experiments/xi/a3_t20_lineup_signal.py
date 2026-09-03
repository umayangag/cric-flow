"""A-3's gate: can a feature family make the T20 selection objective select? Decided on the
walk-forward folds' lineup-only E5 against its re-derived bar -- never on AUC, never on the
locked window (H-19).

Arms (``FAMILIES``): the selection objective refitted per fold on ``XI_FEATURE_COLS`` plus
one family's columns -- ``none`` (today's objective), ``phase_matchup`` (a) and
``role_balance`` (b) -- everything else fixed: the rows, the cutoffs, E5's pairs and the
previous elevens read once from the as-of pass, the bar's derivation, the model class. Per
arm and format it records the fold objective AUC and swap-violation share (the
no-degradation guard), and E5 per fold and pooled with the bar re-derived from the arm's
own claimed effect size under three seeds. Family (c) -- E5's evidence reweighted by the
|delta objective| the arm claims -- changes the measurement, not the model, so it is a
diagnostic computed on every arm and reported beside the verdict, never the verdict. The
gate's triple (``ml.xi.gates``, H-23) is printed before anything runs.

    python a3_t20_lineup_signal.py --frames frames.pkl --cricsheet-dir data/go-app/cricsheet \\
        --pairs-cache a3_pairs.pkl --out a3.json
    python a3_t20_lineup_signal.py --decide a3.json      # the tables and the verdict

Cricsheet carries no bowling style, so the spin/pace split the follow-up plan named for
family (a) cannot be built from what the rating pass reads; the phase axis (powerplay /
middle / death) is the per-player split the pass already accumulates, and it is the
matchup axis used here.
"""

from __future__ import annotations

import argparse
import json
import logging
import math
import os
import sys
import time
from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd
from sklearn.metrics import roc_auc_score

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from sim_frame_cache import load_frames  # noqa: E402

from ml.xi import contract as C  # noqa: E402
from ml.xi import gates  # noqa: E402
from ml.xi import natural_experiment as ne  # noqa: E402
from ml.xi.asof import AsOfRatings  # noqa: E402
from ml.xi.evaluate import MIN_EVAL_ROWS, MIN_TRAIN_ROWS, SWAP_MAX_MATCHES, fold_windows  # noqa: E402
from ml.xi.ratings import aggregate_side  # noqa: E402
from ml.xi.selection_metrics import _VIOLATION_EPS  # noqa: E402
from ml.xi.sources import CricsheetJsonSource  # noqa: E402
from ml.xi.train import _international_teams_from_config, make_objective_model  # noqa: E402

logger = logging.getLogger("a3_t20_lineup_signal")

GATE_ID = "A-3"
CONTROL = "none"
#: The format the gate decides on; the others are reported (and guard the no-degradation clause).
DECIDED_FORMAT = "T20"
#: The bar is re-derived under each of these seeds; an arm must clear all three.
BAR_SEEDS = (ne.BAR_SEED, ne.BAR_SEED + 1, ne.BAR_SEED + 2)
#: H-4's line for the swap-violation share.
SWAP_VIOLATION_LIMIT = 0.02
#: Every per-player key the as-of state serves for an eleven (``RatingState.side_vectors``).
PLAYER_KEYS: List[str] = C.PLAYER_VECTOR_KEYS + C.PLAYER_ROLE_KEYS + C.PLAYER_SEQUENCE_KEYS
#: The one-player upgrade of H-4's probe: these keys, each raised by one population sd.
UPGRADE_KEYS = ("bat_rate", "bat_wrate", "bowl_rate", "bowl_wrate", "pelo")
TOP_ORDER_POSITION = 3.5
SPECIALIST_BATTER_SHARE = 0.6
BALANCED_ATTACK_SIZE = 6.0


# --- Families ---------------------------------------------------------------------------

Stems = Dict[str, float]


@dataclass(frozen=True)
class Family:
    """One arm: extra side stems computed from an eleven's as-of vectors, and cross terms
    of the two sides' stems (the matchup proper). Columns follow the contract's pattern --
    ``d_``, ``t1_``, ``t2_`` per stem -- plus one ``x_`` column per cross term."""

    name: str
    side_stems: Callable[[Dict[str, np.ndarray], str, Stems], Stems]
    cross_terms: Callable[[Stems, Stems], Stems]
    stem_names: Tuple[str, ...]
    cross_names: Tuple[str, ...]

    @property
    def columns(self) -> List[str]:
        cols = [f"{prefix}_{stem}" for stem in self.stem_names for prefix in ("d", "t1", "t2")]
        return cols + [f"x_{name}" for name in self.cross_names]


def _no_stems(vectors: Dict[str, np.ndarray], format_code: str, base: Stems) -> Stems:
    return {}


def _no_cross(own: Stems, opp: Stems) -> Stems:
    return {}


def _phase_stems(vectors: Dict[str, np.ndarray], format_code: str, base: Stems) -> Stems:
    """Per phase: batting impact (runs above expectation per ball, times expected balls
    faced, summed over the eleven) and bowling impact (runs saved, times expected balls
    bowled) -- the phase-resolved form of ``imp_bat_sum`` / ``imp_bowl_sum``."""
    ebf, ebb = vectors["exp_balls_faced"], vectors["exp_balls_bowled"]
    out: Stems = {}
    for phase in C.PHASE_NAMES:
        out[f"bat_{phase}"] = float((vectors[f"bat_{phase}_rate"] * ebf).sum())
        out[f"bowl_{phase}"] = float((vectors[f"bowl_{phase}_rate"] * ebb).sum())
    return out


def _phase_cross(own: Stems, opp: Stems) -> Stems:
    """Same-phase batting-against-bowling products, oriented (team1 batting against team2's
    attack minus the reverse) so the marginalised probability is coherent under H-3."""
    return {
        f"{phase}_matchup": own[f"bat_{phase}"] * opp[f"bowl_{phase}"] - opp[f"bat_{phase}"] * own[f"bowl_{phase}"]
        for phase in C.PHASE_NAMES
    }


def _role_stems(vectors: Dict[str, np.ndarray], format_code: str, base: Stems) -> Stems:
    """Role balance beyond ``n_bowlers`` / ``n_allrounders`` / ``has_keeper``: who bats at
    the top, how many specialist batters, the sixth bowling option, whether the keeper
    bats, and three within-side interactions the additive objective cannot express."""
    ebf, ebb = vectors["exp_balls_faced"], vectors["exp_balls_bowled"]
    bat = vectors["bat_rate"] * ebf
    is_bowler = np.asarray(C.is_bowling_option(ebb, format_code), dtype=bool)
    keeper = vectors["keeper"] > 0.5
    return {
        "n_top_order": float((vectors["exp_bat_position"] <= TOP_ORDER_POSITION).sum()),
        "n_specialist_batters": float(((vectors["bat_innings_share"] >= SPECIALIST_BATTER_SHARE) & ~is_bowler).sum()),
        "bowl_depth_6th": float(np.sort(ebb)[::-1][5]) if len(ebb) > 5 else 0.0,
        "keeper_bat": float(bat[keeper].max()) if keeper.any() else 0.0,
        "bat_x_bowl": base["imp_bat_top6"] * base["imp_bowl_top5"],
        "allround_x_tail": base["n_allrounders"] * base["imp_bat_tail"],
        "attack_balance": -abs(base["n_bowlers"] - BALANCED_ATTACK_SIZE),
    }


FAMILIES: Tuple[Family, ...] = (
    Family(CONTROL, _no_stems, _no_cross, (), ()),
    Family(
        "phase_matchup",
        _phase_stems,
        _phase_cross,
        tuple(f"{kind}_{phase}" for phase in C.PHASE_NAMES for kind in ("bat", "bowl")),
        tuple(f"{phase}_matchup" for phase in C.PHASE_NAMES),
    ),
    Family(
        "role_balance",
        _role_stems,
        _no_cross,
        (
            "n_top_order",
            "n_specialist_batters",
            "bowl_depth_6th",
            "keeper_bat",
            "bat_x_bowl",
            "allround_x_tail",
            "attack_balance",
        ),
        (),
    ),
)
FAMILY_BY_NAME = {family.name: family for family in FAMILIES}


# --- Feature construction from per-player vectors ---------------------------------------


def side_stems(family: Family, vectors: Dict[str, np.ndarray], format_code: str) -> Stems:
    base = aggregate_side(vectors, format_code)
    return {**base, **family.side_stems(vectors, format_code, base)}


def feature_row(family: Family, own: Stems, opp: Stems) -> Dict[str, float]:
    row: Dict[str, float] = {}
    for stem in C.SIDE_FEATURE_STEMS + list(family.stem_names):
        row[f"t1_{stem}"], row[f"t2_{stem}"], row[f"d_{stem}"] = own[stem], opp[stem], own[stem] - opp[stem]
    for name, value in family.cross_terms(own, opp).items():
        row[f"x_{name}"] = value
    return row


def objective_columns(family: Family) -> List[str]:
    return list(C.XI_FEATURE_COLS) + family.columns


def feature_matrix(family: Family, pairs: Sequence[Tuple[Stems, Stems]]) -> np.ndarray:
    columns = objective_columns(family)
    return np.asarray([[feature_row(family, a, b)[c] for c in columns] for a, b in pairs], dtype=float)


def marginalised(model, family: Family, own: Sequence[Stems], opp: Sequence[Stems]) -> np.ndarray:
    """P(own wins), both batting orders averaged (H-3), exactly as the serving path does."""
    if not own:
        return np.empty(0)
    both = [pair for a, b in zip(own, opp) for pair in ((a, b), (b, a))]
    p = model.predict_proba(feature_matrix(family, both))[:, 1]
    return 0.5 * (p[0::2] + (1.0 - p[1::2]))


class SideVectors:
    """Every (match, side)'s per-player as-of vectors, from the player frame -- the same
    numbers ``RatingState.side_vectors`` served when the row was built (H-8)."""

    def __init__(self, player_frame: pd.DataFrame):
        ordered = player_frame.sort_values(["match_id", "side"], kind="stable")
        values = ordered[PLAYER_KEYS].to_numpy(dtype=float)
        self._rows: Dict[Tuple[Any, int], np.ndarray] = {}
        self._keys: Dict[Tuple[Any, int], Tuple[str, ...]] = {}
        player_keys = ordered.player_key.to_numpy()
        for (match_id, side), index in ordered.groupby(["match_id", "side"], sort=False).indices.items():
            self._rows[(match_id, int(side))] = values[index]
            self._keys[(match_id, int(side))] = tuple(player_keys[index])

    def vectors(self, match_id: Any, side: int) -> Dict[str, np.ndarray]:
        return matrix_to_vectors(self._rows[(match_id, side)])

    def keys(self, match_id: Any, side: int) -> Tuple[str, ...]:
        return self._keys[(match_id, side)]


def matrix_to_vectors(matrix: np.ndarray) -> Dict[str, np.ndarray]:
    return {key: matrix[:, j] for j, key in enumerate(PLAYER_KEYS)}


def vectors_to_matrix(vectors: Dict[str, np.ndarray]) -> np.ndarray:
    return np.column_stack([np.asarray(vectors[key], dtype=float) for key in PLAYER_KEYS])


# --- E5's pairs, with every eleven as vectors -------------------------------------------


@dataclass
class PairVectors:
    """One lineup pair with the five elevens E5 scores as per-player matrices: match k+1's
    fielded eleven and its opponent, match k's eleven at match k+1's as-of (from the as-of
    pass), and match k's two elevens as played (for the derived bar's null world)."""

    format_code: str
    after_date: pd.Timestamp
    changes: int
    d_result: int
    after: np.ndarray
    opponent: np.ndarray
    previous_at_after: np.ndarray
    before_played: np.ndarray
    before_opponent_played: np.ndarray


def _side_index(row_match_id: Any, team: str, frame: pd.DataFrame) -> int:
    match = frame[frame.match_id == row_match_id].iloc[0]
    return 1 if match.team1 == team else 2


def build_pair_vectors(
    pairs: Sequence[ne.LineupPair], sides: SideVectors, match_frame: pd.DataFrame, cricsheet_dir: str
) -> Tuple[List[PairVectors], Dict[str, Any]]:
    """Match k's eleven read from the as-of serving state at match k+1's date -- one
    advancing pass, as ``natural_experiment.score_previous_elevens`` does -- with the
    fielded eleven read from the same state and checked against the frame's vectors."""
    by_match = match_frame.set_index("match_id")
    asof = AsOfRatings(CricsheetJsonSource(cricsheet_dir, _international_teams_from_config()))
    out: List[PairVectors] = []
    max_difference = 0.0
    started = time.perf_counter()
    for i, pair in enumerate(sorted(pairs, key=lambda p: (p.after_date, str(p.after_match_id)))):
        state = asof.state_as_of(pair.after_date.date())
        previous = vectors_to_matrix(state.side_vectors(pair.format_code, list(pair.before_keys)))
        fielded = vectors_to_matrix(state.side_vectors(pair.format_code, list(pair.after_keys)))
        after_side = 1 if by_match.loc[pair.after_match_id].team1 == pair.team else 2
        before_side = 1 if by_match.loc[pair.before_match_id].team1 == pair.team else 2
        after = sides.vectors(pair.after_match_id, after_side)
        frame_order = {key: j for j, key in enumerate(sides.keys(pair.after_match_id, after_side))}
        reordered = vectors_to_matrix(after)[[frame_order[key] for key in pair.after_keys]]
        max_difference = max(max_difference, float(np.abs(reordered - fielded).max()))
        out.append(
            PairVectors(
                format_code=pair.format_code,
                after_date=pair.after_date,
                changes=pair.changes,
                d_result=pair.d_result,
                after=vectors_to_matrix(after),
                opponent=vectors_to_matrix(sides.vectors(pair.after_match_id, 3 - after_side)),
                previous_at_after=previous,
                before_played=vectors_to_matrix(sides.vectors(pair.before_match_id, before_side)),
                before_opponent_played=vectors_to_matrix(sides.vectors(pair.before_match_id, 3 - before_side)),
            )
        )
        if (i + 1) % 5000 == 0:
            logger.info("as-of pass: %d pairs in %.0f s", i + 1, time.perf_counter() - started)
    parity = {"pairs": len(out), "fielded_eleven_max_abs_difference": max_difference}
    logger.info("E5: %d pairs; fielded-eleven parity max abs difference %.2e", len(out), max_difference)
    return out, parity


# --- Per fold, per arm -----------------------------------------------------------------


def _match_pairs(family: Family, frame: pd.DataFrame, sides: SideVectors) -> List[Tuple[Stems, Stems]]:
    return [
        (
            side_stems(family, sides.vectors(match_id, 1), format_code),
            side_stems(family, sides.vectors(match_id, 2), format_code),
        )
        for match_id, format_code in zip(frame.match_id, frame.format_code)
    ]


def _base_parity(rows: List[Tuple[Stems, Stems]], frame: pd.DataFrame) -> float:
    """The base stems recomputed from the player frame against the win frame's own
    columns -- zero if the reconstruction reads what the pass wrote."""
    worst = 0.0
    for (own, opp), row in zip(rows, frame.itertuples()):
        for stem in C.SIDE_FEATURE_STEMS:
            worst = max(worst, abs(own[stem] - getattr(row, f"t1_{stem}")), abs(opp[stem] - getattr(row, f"t2_{stem}")))
    return worst


def _swap_probe(model, family: Family, frame: pd.DataFrame, sides: SideVectors, format_code: str) -> Optional[Dict]:
    """H-4's probe with the arm's feature construction: one player's five ratings raised by
    one population sd; the share of upgrades that lower P(win)."""
    window_ids = frame.match_id.drop_duplicates().tolist()[:SWAP_MAX_MATCHES]
    if not window_ids:
        return None
    pooled = np.vstack([sides._rows[(match_id, side)] for match_id in window_ids for side in (1, 2)])
    steps = {key: float(pooled[:, PLAYER_KEYS.index(key)].std()) for key in UPGRADE_KEYS}
    upgrades = violations = 0
    for match_id in window_ids:
        own, opp = sides.vectors(match_id, 1), sides.vectors(match_id, 2)
        opponent = side_stems(family, opp, format_code)
        base = marginalised(model, family, [side_stems(family, own, format_code)], [opponent])[0]
        upgraded_sides = []
        for j in range(len(own["pelo"])):
            upgraded = {key: values.copy() for key, values in own.items()}
            for key, step in steps.items():
                upgraded[key][j] += step
            upgraded_sides.append(side_stems(family, upgraded, format_code))
        p = marginalised(model, family, upgraded_sides, [opponent] * len(upgraded_sides))
        upgrades += len(p)
        violations += int((p < base - _VIOLATION_EPS).sum())
    return {"upgrades": upgrades, "violations": violations, "violation_share": violations / upgrades}


def _score_pairs(model, family: Family, window: Sequence[PairVectors]) -> ne.ScoredPairs:
    def stems(matrices: Sequence[np.ndarray], format_codes: Sequence[str]) -> List[Stems]:
        return [side_stems(family, matrix_to_vectors(m), f) for m, f in zip(matrices, format_codes)]

    formats = [p.format_code for p in window]
    after = stems([p.after for p in window], formats)
    opponent = stems([p.opponent for p in window], formats)
    previous = stems([p.previous_at_after for p in window], formats)
    before = stems([p.before_played for p in window], formats)
    before_opp = stems([p.before_opponent_played for p in window], formats)
    return ne.ScoredPairs(
        p_before_played=marginalised(model, family, before, before_opp),
        p_previous=marginalised(model, family, previous, opponent),
        p_after=marginalised(model, family, after, opponent),
        d_result=np.asarray([p.d_result for p in window], dtype=int),
        changes=np.asarray([p.changes for p in window], dtype=int),
    )


def run_fold(
    family: Family,
    format_code: str,
    format_frame: pd.DataFrame,
    sides: SideVectors,
    pairs: Sequence[PairVectors],
    cutoff: pd.Timestamp,
    end: pd.Timestamp,
) -> Tuple[Dict[str, Any], Optional[ne.ScoredPairs]]:
    train = format_frame[format_frame.match_date < cutoff]
    evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
    fold: Dict[str, Any] = {
        "cutoff": cutoff.date().isoformat(),
        "end": end.date().isoformat(),
        "n_train": int(len(train)),
        "n_eval": int(len(evaluation)),
    }
    if len(train) < MIN_TRAIN_ROWS or train[C.TARGET_COL].nunique() < 2:
        fold["skipped_reason"] = "insufficient training rows"
        return fold, None
    if len(evaluation) < MIN_EVAL_ROWS or evaluation[C.TARGET_COL].nunique() < 2:
        fold["skipped_reason"] = "evaluation window too small or single-class"
        return fold, None
    started = time.perf_counter()
    train_rows = _match_pairs(family, train, sides)
    fold["base_stem_parity_max_abs_difference"] = _base_parity(train_rows, train)
    model = make_objective_model().fit(feature_matrix(family, train_rows), train[C.TARGET_COL].to_numpy(dtype=float))
    eval_rows = _match_pairs(family, evaluation, sides)
    p = marginalised(model, family, [a for a, _ in eval_rows], [b for _, b in eval_rows])
    fold["objective_auc"] = float(roc_auc_score(evaluation[C.TARGET_COL].to_numpy(dtype=float), p))
    fold["swap_monotonicity"] = _swap_probe(model, family, evaluation, sides, format_code)
    window = [q for q in pairs if q.format_code == format_code and cutoff <= q.after_date < end]
    fold["pairs"] = len(window)
    scored = _score_pairs(model, family, window) if window else None
    if scored is not None:
        fold["e5"] = ne.agreement(scored.d_lineup, scored.d_result)
        fold["e5"]["effect_size"] = ne.effect_size(scored.d_lineup)
    fold["seconds"] = round(time.perf_counter() - started, 1)
    logger.info(
        "%-13s %-5s @ %s AUC %.3f swap %.4f pairs %d E5 %s (%.1f s)",
        family.name,
        format_code,
        fold["cutoff"],
        fold["objective_auc"],
        (fold["swap_monotonicity"] or {}).get("violation_share", float("nan")),
        len(window),
        "n/a" if scored is None or fold["e5"]["agreement"] is None else "%.3f" % fold["e5"]["agreement"],
        fold["seconds"],
    )
    return fold, scored


# --- Family (c): the measurement reweighted by the claimed |delta| ---------------------


def reweighted_agreement(
    scored: ne.ScoredPairs, weights: np.ndarray, seeds: Sequence[int] = BAR_SEEDS
) -> Dict[str, Any]:
    """Sign agreement with each pair weighted by ``weights`` (over the pairs whose result
    moved and on which the objective has a preference), and the bar derived the same way:
    the weighted agreement an exactly-right objective would produce, its closed-form
    expectation and the 5th percentile over Bernoulli replicates -- so a weighting that
    favours the pairs the objective is surest about raises the bar with the number."""
    preference = scored.d_lineup != 0
    moved = scored.d_result != 0
    w = weights * preference
    up_claimed = scored.d_lineup > 0
    agreed = (up_claimed == (scored.d_result > 0)) & moved & preference
    denominator = float((w * moved).sum())
    if denominator <= 0.0:
        return {"agreement": None, "bar": None}
    observed = float((w * agreed).sum() / denominator)
    a, b = scored.p_before_played, scored.p_after
    p_up, p_down = (1.0 - a) * b, a * (1.0 - b)
    expected = float((w * np.where(up_claimed, p_up, p_down)).sum() / (w * (p_up + p_down)).sum())
    bars = {}
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
        "effective_pairs": float(denominator),
        "expected_if_exactly_right": expected,
        "bar_by_seed": bars,
        "passes_all_seeds": all(observed >= bar for bar in bars.values()),
    }


def by_effect_tercile(scored: ne.ScoredPairs) -> Dict[str, Any]:
    """Agreement inside each tercile of the claimed |delta| against that tercile's
    exactly-right expectation: if the null is rotation noise, the bottom tercile sits at
    chance and the top one at its expectation."""
    magnitude = np.abs(scored.d_lineup)
    preference = magnitude > 0
    if preference.sum() < 3:
        return {}
    cuts = np.quantile(magnitude[preference], [1 / 3, 2 / 3])
    out = {}
    for name, mask in (
        ("bottom", preference & (magnitude <= cuts[0])),
        ("middle", preference & (magnitude > cuts[0]) & (magnitude <= cuts[1])),
        ("top", preference & (magnitude > cuts[1])),
    ):
        subset = ne.ScoredPairs(
            scored.p_before_played[mask],
            scored.p_previous[mask],
            scored.p_after[mask],
            scored.d_result[mask],
            scored.changes[mask],
        )
        entry = ne.agreement(subset.d_lineup, subset.d_result)
        entry["median_abs_delta"] = float(np.median(magnitude[mask])) if mask.any() else None
        entry["expected_if_exactly_right"] = ne.derived_bar(subset, replicates=1).get("expected_if_exactly_right")
        out[name] = entry
    return out


# --- The arm's verdict per format -------------------------------------------------------


def pooled_section(scored: ne.ScoredPairs) -> Dict[str, Any]:
    out = ne.agreement(scored.d_lineup, scored.d_result)
    out["effect_size"] = ne.effect_size(scored.d_lineup)
    bars = {str(seed): ne.derived_bar(scored, seed=seed) for seed in BAR_SEEDS}
    out["derived_bar"] = {
        "expected_if_exactly_right": bars[str(BAR_SEEDS[0])]["expected_if_exactly_right"],
        "by_seed": {seed: entry["bar"] for seed, entry in bars.items()},
        "simulated_sd": bars[str(BAR_SEEDS[0])].get("simulated_sd"),
    }
    values = [v for v in out["derived_bar"]["by_seed"].values() if v is not None]
    out["passes_derived_bar_all_seeds"] = (
        None if out["agreement"] is None or not values else bool(all(out["agreement"] >= v for v in values))
    )
    out["by_changes"] = {
        str(k): ne.agreement(scored.d_lineup[scored.changes == k], scored.d_result[scored.changes == k])["agreement"]
        for k in range(ne.MIN_CHANGES, ne.MAX_CHANGES + 1)
    }
    out["reweighted_by_abs_delta"] = reweighted_agreement(scored, np.abs(scored.d_lineup))
    top_half = np.abs(scored.d_lineup) >= np.median(np.abs(scored.d_lineup)[scored.d_lineup != 0])
    out["top_half_by_abs_delta"] = reweighted_agreement(scored, top_half.astype(float))
    out["by_effect_tercile"] = by_effect_tercile(scored)
    return out


def run_arm(
    family: Family, match_frame: pd.DataFrame, sides: SideVectors, pairs: Sequence[PairVectors]
) -> Dict[str, Any]:
    out: Dict[str, Any] = {"columns_added": family.columns, "formats": {}}
    for format_code in C.FORMAT_CODES:
        format_frame = match_frame[match_frame.format_code == format_code]
        folds, scored = [], []
        for cutoff, end in fold_windows():
            fold, fold_scored = run_fold(family, format_code, format_frame, sides, pairs, cutoff, end)
            folds.append(fold)
            if fold_scored is not None:
                scored.append(fold_scored)
        pooled = ne._concat(scored)
        out["formats"][format_code] = {
            "folds": folds,
            "development": pooled_section(pooled) if len(pooled) else {"agreement": None, "pairs_scored": 0},
        }
    return out


def _paired(a: List[Dict], b: List[Dict], key: str) -> Dict[str, Any]:
    values = np.asarray(
        [fa[key] - fb[key] for fa, fb in zip(a, b) if key in fa and key in fb and fa["cutoff"] == fb["cutoff"]],
        dtype=float,
    )
    if not len(values):
        return {"mean": None, "se": None, "n_folds": 0}
    se = float(np.std(values, ddof=1) / math.sqrt(len(values))) if len(values) > 1 else float("nan")
    return {"mean": float(values.mean()), "se": se, "n_folds": int(len(values))}


def verdicts(arms: Dict[str, Dict[str, Any]]) -> Dict[str, Any]:
    """The gate's rule per arm: the decided format's pooled agreement against its own bar
    under every seed, and the no-degradation guard in every format."""
    control = arms[CONTROL]["formats"]
    out: Dict[str, Any] = {}
    for name, arm in arms.items():
        entry: Dict[str, Any] = {"guard": {}}
        decided = arm["formats"][DECIDED_FORMAT]["development"]
        entry["clears_bar"] = decided.get("passes_derived_bar_all_seeds")
        for format_code, node in arm["formats"].items():
            auc = _paired(node["folds"], control[format_code]["folds"], "objective_auc")
            shares = [f["swap_monotonicity"]["violation_share"] for f in node["folds"] if f.get("swap_monotonicity")]
            swap_mean = float(np.mean(shares)) if shares else None
            entry["guard"][format_code] = {
                "auc_delta": auc,
                "auc_not_degraded": auc["mean"] is None or auc["mean"] >= -(auc["se"] if auc["se"] == auc["se"] else 0),
                "swap_violation_share": swap_mean,
                "swap_within_h4": swap_mean is None or swap_mean < SWAP_VIOLATION_LIMIT,
            }
        entry["guard_holds"] = all(g["auc_not_degraded"] and g["swap_within_h4"] for g in entry["guard"].values())
        entry["ships"] = bool(entry["clears_bar"]) and entry["guard_holds"] and name != CONTROL
        out[name] = entry
    return out


# --- Driver -----------------------------------------------------------------------------


def run(frames_path: str, cricsheet_dir: str, pairs_cache: Optional[str], out: str) -> Dict[str, Any]:
    player_frame, match_frame = load_frames(None, frames_path)
    sides = SideVectors(player_frame)
    if pairs_cache and os.path.exists(pairs_cache):
        logger.info("pair vectors from cache %s", pairs_cache)
        pairs, parity = pd.read_pickle(pairs_cache)
    else:
        lineup_pairs = ne.build_pairs(match_frame, player_frame)
        pairs, parity = build_pair_vectors(lineup_pairs, sides, match_frame, cricsheet_dir)
        if pairs_cache:
            pd.to_pickle((pairs, parity), pairs_cache)
    arms = {family.name: run_arm(family, match_frame, sides, pairs) for family in FAMILIES}
    result = {
        "gate": gates.describe(GATE_ID),
        "decided_format": DECIDED_FORMAT,
        "bar_seeds": list(BAR_SEEDS),
        "cutoffs": [c.date().isoformat() for c, _ in fold_windows()],
        "pairs": parity,
        "arms": arms,
        "verdicts": verdicts(arms),
    }
    with open(out, "w") as fh:
        json.dump(result, fh, indent=2)
    logger.info("written %s", out)
    return result


def _f(value: Optional[float], pattern: str) -> str:
    return "n/a" if value is None or value != value else pattern % value


def _fold_mean(folds: List[Dict], key: str) -> Optional[float]:
    values = [f[key] for f in folds if key in f]
    return float(np.mean(values)) if values else None


def decide(path: str) -> None:
    with open(path) as fh:
        result = json.load(fh)
    print(result["gate"])
    print()
    print(f"E5 pairs: {result['pairs']}")
    print()
    print(
        "| arm | format | folds | objective AUC (Δ vs none ± se) | swap share | E5 pairs scored | agreement ± se | "
        "claimed median |Δ| | exactly-right | bar (3 seeds) | by changes 1 / 2 / 3 | verdict |"
    )
    print("|---|---|---:|---|---:|---:|---|---:|---:|---|---|---|")
    for name, arm in result["arms"].items():
        v = result["verdicts"][name]
        for format_code, node in arm["formats"].items():
            dev, folds = node["development"], node["folds"]
            guard = v["guard"][format_code]
            scored = [f for f in folds if "objective_auc" in f]
            delta = guard["auc_delta"]
            bars = dev.get("derived_bar", {}).get("by_seed", {})
            bar_text = "–".join(_f(x, "%.3f") for x in (min(bars.values()), max(bars.values()))) if bars else "n/a"
            if format_code == DECIDED_FORMAT and name != CONTROL:
                cell = ("clears" if v["clears_bar"] else "fails") + (
                    "; guard holds" if v["guard_holds"] else "; guard fails"
                )
                cell += " → SHIPS" if v["ships"] else ""
            elif format_code == DECIDED_FORMAT:
                cell = "control: " + ("passes" if dev.get("passes_derived_bar_all_seeds") else "fails")
            else:
                cell = "reported: " + ("passes" if dev.get("passes_derived_bar_all_seeds") else "fails")
                cell += "" if guard["auc_not_degraded"] and guard["swap_within_h4"] else "; guard fails"
            changes = dev.get("by_changes", {})
            print(
                f"| {name} | {format_code} | {len(scored)} | {_f(_fold_mean(folds, 'objective_auc'), '%.3f')} "
                f"({_f(delta['mean'], '%+.3f')} ± {_f(delta['se'], '%.3f')}) | "
                f"{_f(guard['swap_violation_share'], '%.4f')} | {dev.get('pairs_scored', 0)} | "
                f"{_f(dev.get('agreement'), '%.3f')} ± {_f(dev.get('standard_error'), '%.3f')} | "
                f"{_f((dev.get('effect_size') or {}).get('median_abs'), '%.4f')} | "
                f"{_f(dev.get('derived_bar', {}).get('expected_if_exactly_right'), '%.3f')} | {bar_text} | "
                f"{' / '.join(_f(changes.get(str(k)), '%.3f') for k in (1, 2, 3))} | {cell} |"
            )
    print()
    print("Family (c) -- the measurement reweighted by the claimed |Δ|, on every arm (reported, never decides):")
    print(
        "| arm | format | unweighted: agreement / exactly-right / bar | |Δ|-weighted: agreement / exactly-right / bar | "
        "top half by |Δ|: agreement / exactly-right / bar | terciles of |Δ| (bottom / middle / top): agreement vs exactly-right |"
    )
    print("|---|---|---|---|---|---|")
    for name, arm in result["arms"].items():
        for format_code, node in arm["formats"].items():
            dev = node["development"]
            if dev.get("agreement") is None:
                continue
            bars = dev["derived_bar"]["by_seed"]

            def cell(entry: Dict[str, Any]) -> str:
                if entry.get("agreement") is None:
                    return "n/a"
                b = entry["bar_by_seed"]
                return (
                    f"{entry['agreement']:.3f} / {entry['expected_if_exactly_right']:.3f} / "
                    f"{min(b.values()):.3f}–{max(b.values()):.3f}"
                    + (" passes" if entry["passes_all_seeds"] else " fails")
                )

            terciles = dev.get("by_effect_tercile", {})
            tercile_text = " / ".join(
                f"{_f(terciles.get(t, {}).get('agreement'), '%.3f')} vs "
                f"{_f(terciles.get(t, {}).get('expected_if_exactly_right'), '%.3f')} (n={terciles.get(t, {}).get('pairs_scored', 0)})"
                for t in ("bottom", "middle", "top")
            )
            print(
                f"| {name} | {format_code} | {dev['agreement']:.3f} / {dev['derived_bar']['expected_if_exactly_right']:.3f} / "
                f"{min(bars.values()):.3f}–{max(bars.values()):.3f}"
                f"{' passes' if dev['passes_derived_bar_all_seeds'] else ' fails'} | "
                f"{cell(dev['reweighted_by_abs_delta'])} | {cell(dev['top_half_by_abs_delta'])} | {tercile_text} |"
            )
    print()
    print("Per-fold T20 agreement (none / phase_matchup / role_balance):")
    control = result["arms"][CONTROL]["formats"][DECIDED_FORMAT]["folds"]
    for i, fold in enumerate(control):
        cells = []
        for name in result["arms"]:
            f = result["arms"][name]["formats"][DECIDED_FORMAT]["folds"][i]
            cells.append(_f((f.get("e5") or {}).get("agreement"), "%.3f"))
        print(f"{fold['cutoff']} (pairs {fold.get('pairs', 0)}): " + " / ".join(cells))
    print()
    shipped = [name for name, v in result["verdicts"].items() if v["ships"]]
    for name, v in result["verdicts"].items():
        if name == CONTROL:
            continue
        print(
            f"{name}: {'SHIPS' if v['ships'] else 'does not ship'} (clears bar: {v['clears_bar']}, guard: {v['guard_holds']})"
        )
    print(f"T20 optimised selection: {'ON with ' + ', '.join(shipped) if shipped else 'stays OFF (rating-ordered)'}")


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--frames", help="pickle from sim_frame_cache.py")
    p.add_argument("--cricsheet-dir", help="the archive the as-of pass reads the previous elevens from")
    p.add_argument("--pairs-cache", help="pickle of the pairs' elevens as vectors (written on the first run)")
    p.add_argument("--out")
    p.add_argument("--decide", help="a result file; prints the tables and the verdict")
    args = p.parse_args()
    if args.decide:
        decide(args.decide)
        return 0
    if not (
        args.frames and args.out and (args.cricsheet_dir or (args.pairs_cache and os.path.exists(args.pairs_cache)))
    ):
        p.error("--frames, --out and --cricsheet-dir (or an existing --pairs-cache) are required to run")
    print(gates.describe(GATE_ID))
    run(args.frames, args.cricsheet_dir, args.pairs_cache, args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
