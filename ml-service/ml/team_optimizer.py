"""Server-side team selection optimization via hill-climb with batch model inference.

Moves the win-probability hill-climb loop from the Go-app into the ML service,
eliminating per-candidate HTTP round-trips.  A single POST to
``/optimize/team-selection`` replaces hundreds of ``POST /predict/win-enhanced``
calls.

The hill-climb algorithm mirrors the Go implementation in
``go-app/internal/services/teamselect/optimize.go`` (greedy seed + single-swap
improvement passes), but evaluates swap candidates in vectorised batches via
``model.predict_proba(X_batch)`` instead of one-at-a-time HTTP calls.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Dict, List, Mapping, Sequence

import numpy as np

from .win_features import (
    _DIST_STAT_KEYS,
    _DIST_SUFFIXES,
    _GROUP_TO_PLAYER_KEY,
    MATCH_CONTEXT_COLS,
    WIN_ENHANCED_FEATURE_COLS,
    aggregate_team_features_from_player_maps,
    build_feature_vector,
)

_COL_INDEX: Dict[str, int] = {col: i for i, col in enumerate(WIN_ENHANCED_FEATURE_COLS)}
_N_COLS = len(WIN_ENHANCED_FEATURE_COLS)


# ---------------------------------------------------------------------------
# Data classes
# ---------------------------------------------------------------------------


@dataclass
class PoolPlayer:
    player_id: int
    name: str
    is_bowler: bool
    is_keeper: bool
    bat_score: float
    bowl_score: float
    field_score: float
    features: Dict[str, float] = field(default_factory=dict)


@dataclass
class SelectionConstraints:
    size: int = 11
    min_bowlers: int = 5
    require_keeper: bool = True


@dataclass
class ScoreWeights:
    bat: float = 0.45
    bowl: float = 0.40
    field: float = 0.10
    keeper_bonus: float = 0.02


@dataclass
class OptimizationResult:
    selected: List[PoolPlayer]
    win_probability: float
    iterations_used: int
    evals_performed: int


# ---------------------------------------------------------------------------
# Greedy seed selection (mirrors Go ``teamselect.Select``)
# ---------------------------------------------------------------------------


def _clamp01(x: float) -> float:
    return max(0.0, min(1.0, x))


def _score_player(p: PoolPlayer, w: ScoreWeights) -> float:
    s = w.bat * _clamp01(p.bat_score) + w.bowl * _clamp01(p.bowl_score)
    if w.field > 0:
        s += w.field * _clamp01(p.field_score)
    if p.is_keeper:
        s += w.keeper_bonus
    return s


def _satisfies_constraints(team: Sequence[PoolPlayer], c: SelectionConstraints) -> bool:
    keepers = sum(1 for p in team if p.is_keeper)
    bowlers = sum(1 for p in team if p.is_bowler)
    if c.require_keeper and keepers < 1:
        return False
    return bowlers >= c.min_bowlers


def greedy_select(
    pool: List[PoolPlayer],
    weights: ScoreWeights,
    constraints: SelectionConstraints,
) -> List[PoolPlayer]:
    """Greedy seed selection matching Go's ``teamselect.Select``."""
    if constraints.size < 1:
        raise ValueError("invalid size")
    if constraints.min_bowlers < 0:
        raise ValueError("invalid min bowlers")
    if len(pool) < constraints.size:
        raise ValueError("insufficient pool size")

    scored = sorted(pool, key=lambda p: (-_score_player(p, weights), p.name))
    team = list(scored[: constraints.size])
    rest = list(scored[constraints.size :])

    if constraints.require_keeper and not any(p.is_keeper for p in team):
        keeper_idx = next((i for i, p in enumerate(rest) if p.is_keeper), None)
        if keeper_idx is None:
            raise ValueError("no keeper available")
        rep_idx = next((i for i in range(len(team) - 1, -1, -1) if not team[i].is_keeper), None)
        if rep_idx is None:
            raise ValueError("cannot satisfy keeper constraint")
        team[rep_idx], rest[keeper_idx] = rest[keeper_idx], team[rep_idx]

    bowler_count = sum(1 for p in team if p.is_bowler)
    while bowler_count < constraints.min_bowlers:
        bowler_idx = next((i for i, p in enumerate(rest) if p.is_bowler), None)
        if bowler_idx is None:
            raise ValueError("not enough bowlers to satisfy constraint")
        rep_idx = next(
            (
                i
                for i in range(len(team) - 1, -1, -1)
                if not team[i].is_bowler and not (constraints.require_keeper and team[i].is_keeper)
            ),
            None,
        )
        if rep_idx is None:
            rep_idx = next((i for i in range(len(team) - 1, -1, -1) if not team[i].is_bowler), None)
        if rep_idx is None:
            raise ValueError("cannot replace to satisfy bowler constraint")
        team[rep_idx], rest[bowler_idx] = rest[bowler_idx], team[rep_idx]
        bowler_count += 1

    return team


# ---------------------------------------------------------------------------
# Opponent feature precomputation (constant across all evaluations)
# ---------------------------------------------------------------------------


def _precompute_fixed_team_stats(
    features: Mapping[int, Mapping[str, float]],
    team_number: int,
) -> Dict[str, float]:
    """Compute distribution stats for a fixed team (opponent or non-changing side)."""
    result: Dict[str, float] = {}
    for group_name, player_key, t_num in _GROUP_TO_PLAYER_KEY:
        if t_num != team_number:
            continue
        values = [fm.get(player_key, 0.0) for fm in features.values()]
        arr = np.array(values, dtype=np.float64) if values else np.array([], dtype=np.float64)
        if len(arr) == 0:
            stats = {k: 0.0 for k in _DIST_STAT_KEYS}
        else:
            top3 = np.sort(arr)[-3:] if len(arr) >= 3 else arr
            stats = {
                "sum": float(np.sum(arr)),
                "mean": float(np.mean(arr)),
                "std": float(np.std(arr)),
                "max": float(np.max(arr)),
                "min": float(np.min(arr)),
                "top3_mean": float(np.mean(top3)),
                "count": float(len(arr)),
            }
        for suffix, stat_key in zip(_DIST_SUFFIXES, _DIST_STAT_KEYS):
            val = stats[stat_key]
            result[group_name + suffix] = 0.0 if (isinstance(val, float) and math.isnan(val)) else val
    return result


# ---------------------------------------------------------------------------
# Vectorised batch evaluation
# ---------------------------------------------------------------------------


def _compute_derived_features_batch(feature_matrix: np.ndarray) -> None:
    """Compute derived feature columns in-place over the batch feature matrix.

    Mirrors ``win_features.compute_derived_features`` but operates on entire
    batch rows simultaneously via numpy column slicing.
    """

    def _col(name: str) -> np.ndarray:
        return feature_matrix[:, _COL_INDEX[name]]

    def _safe_ratio(num: np.ndarray, den: np.ndarray, default: float = 1.0) -> np.ndarray:
        out = np.full_like(num, default)
        mask = np.abs(den) >= 1e-9
        out[mask] = num[mask] / den[mask]
        return out

    feature_matrix[:, _COL_INDEX["bat_form_matchup_ratio_team1"]] = _safe_ratio(
        _col("team1_bat_form_mean"), _col("team2_bowl_form_mean")
    )
    feature_matrix[:, _COL_INDEX["bat_form_matchup_ratio_team2"]] = _safe_ratio(
        _col("team2_bat_form_mean"), _col("team1_bowl_form_mean")
    )
    feature_matrix[:, _COL_INDEX["bat_cons_matchup_ratio_team1"]] = _safe_ratio(
        _col("team1_bat_consistency_mean"), _col("team2_bowl_consistency_mean")
    )
    feature_matrix[:, _COL_INDEX["bat_cons_matchup_ratio_team2"]] = _safe_ratio(
        _col("team2_bat_consistency_mean"), _col("team1_bowl_consistency_mean")
    )
    feature_matrix[:, _COL_INDEX["bowl_depth_diff"]] = _col("team1_bowl_consistency_count") - _col(
        "team2_bowl_consistency_count"
    )
    feature_matrix[:, _COL_INDEX["bat_form_top3_diff"]] = _col("team1_bat_form_top3_mean") - _col(
        "team2_bat_form_top3_mean"
    )
    feature_matrix[:, _COL_INDEX["bowl_form_top3_diff"]] = _col("team1_bowl_form_top3_mean") - _col(
        "team2_bowl_form_top3_mean"
    )
    feature_matrix[:, _COL_INDEX["bat_cons_top3_diff"]] = _col("team1_bat_consistency_top3_mean") - _col(
        "team2_bat_consistency_top3_mean"
    )
    feature_matrix[:, _COL_INDEX["bowl_cons_top3_diff"]] = _col("team1_bowl_consistency_top3_mean") - _col(
        "team2_bowl_consistency_top3_mean"
    )
    feature_matrix[:, _COL_INDEX["team1_bat_form_spread"]] = _col("team1_bat_form_max") - _col("team1_bat_form_min")
    feature_matrix[:, _COL_INDEX["team2_bat_form_spread"]] = _col("team2_bat_form_max") - _col("team2_bat_form_min")
    feature_matrix[:, _COL_INDEX["team1_bowl_form_spread"]] = _col("team1_bowl_form_max") - _col("team1_bowl_form_min")
    feature_matrix[:, _COL_INDEX["team2_bowl_form_spread"]] = _col("team2_bowl_form_max") - _col("team2_bowl_form_min")


def _batch_evaluate_candidates(
    team: List[PoolPlayer],
    swap_pos: int,
    candidates: List[PoolPlayer],
    opponent_stats: Dict[str, float],
    match_context: Mapping[str, float],
    team_is_team1: bool,
    model,  # sklearn-compatible classifier
) -> np.ndarray:
    """Evaluate all swap candidates for a given position in a single batch.

    Builds one feature matrix of shape ``(n_candidates, n_features)`` and calls
    ``model.predict_proba`` once instead of N separate HTTP calls.

    Returns an array of win probabilities from the *candidate team's* perspective.
    """
    n_cand = len(candidates)
    feature_matrix = np.zeros((n_cand, _N_COLS), dtype=np.float64)

    # Match context columns are constant across all candidates.
    for col in MATCH_CONTEXT_COLS:
        if col in _COL_INDEX:
            feature_matrix[:, _COL_INDEX[col]] = float(match_context.get(col, 0.0))

    # Opponent-side distribution stats are constant.
    for col_name, val in opponent_stats.items():
        if col_name in _COL_INDEX:
            feature_matrix[:, _COL_INDEX[col_name]] = val

    # Candidate-team distribution stats vary per candidate (vectorised).
    base_players = [team[j] for j in range(len(team)) if j != swap_pos]
    candidate_team_number = 1 if team_is_team1 else 2

    for group_name, player_key, t_num in _GROUP_TO_PLAYER_KEY:
        if t_num != candidate_team_number:
            continue
        base_vals = np.array([p.features.get(player_key, 0.0) for p in base_players], dtype=np.float64)
        cand_vals = np.array([c.features.get(player_key, 0.0) for c in candidates], dtype=np.float64)

        full_vals = np.column_stack([np.tile(base_vals, (n_cand, 1)), cand_vals.reshape(-1, 1)])

        stat_arrays = {
            "sum": full_vals.sum(axis=1),
            "mean": full_vals.mean(axis=1),
            "std": full_vals.std(axis=1),
            "max": full_vals.max(axis=1),
            "min": full_vals.min(axis=1),
            "top3_mean": (
                np.sort(full_vals, axis=1)[:, -3:].mean(axis=1) if full_vals.shape[1] >= 3 else full_vals.mean(axis=1)
            ),
            "count": np.full(n_cand, float(full_vals.shape[1])),
        }
        for suffix, stat_key in zip(_DIST_SUFFIXES, _DIST_STAT_KEYS):
            col_name = group_name + suffix
            vals = stat_arrays[stat_key]
            vals = np.where(np.isnan(vals), 0.0, vals)
            feature_matrix[:, _COL_INDEX[col_name]] = vals

    _compute_derived_features_batch(feature_matrix)

    proba = model.predict_proba(feature_matrix)
    if proba.shape[1] > 1:
        p_team1 = proba[:, 1].astype(np.float64)
    else:
        p_team1 = proba.ravel().astype(np.float64)
        if model.classes_[0] != 1:
            p_team1 = 1.0 - p_team1

    if not team_is_team1:
        p_team1 = 1.0 - p_team1

    return p_team1


# ---------------------------------------------------------------------------
# Single-team evaluation (for scoring the initial seed)
# ---------------------------------------------------------------------------


def _evaluate_single_team(
    team: List[PoolPlayer],
    opponent_features: Mapping[int, Mapping[str, float]],
    match_context: Mapping[str, float],
    team_is_team1: bool,
    model,
    format_code: str,
) -> float:
    """Win probability of a single team composition (used for initial seed score)."""
    team_feats: Dict[int, Dict[str, float]] = {p.player_id: dict(p.features) for p in team}
    opp_feats = dict(opponent_features)

    if team_is_team1:
        feature_dict = aggregate_team_features_from_player_maps(
            team_feats, opp_feats, match_context, format_code=format_code
        )
    else:
        feature_dict = aggregate_team_features_from_player_maps(
            opp_feats, team_feats, match_context, format_code=format_code
        )

    vec = build_feature_vector(feature_dict)
    X = np.array([vec], dtype=np.float64)
    proba = model.predict_proba(X)
    if proba.shape[1] > 1:
        p = float(proba[0, 1])
    else:
        p = float(proba[0, 0]) if model.classes_[0] == 1 else 1.0 - float(proba[0, 0])

    if not team_is_team1:
        p = 1.0 - p
    return p


# ---------------------------------------------------------------------------
# Main optimisation entry point
# ---------------------------------------------------------------------------


def optimize_team_by_win_probability(
    pool: List[PoolPlayer],
    opponent_features: Mapping[int, Mapping[str, float]],
    match_context: Mapping[str, float],
    constraints: SelectionConstraints,
    weights: ScoreWeights,
    team_is_team1: bool,
    model,
    max_iterations: int = 50,
    max_evals: int = 500,
    format_code: str = "",
) -> OptimizationResult:
    """Hill-climb team optimisation with vectorised batch model inference.

    1. Greedy-seeds a starting XI (same algorithm as Go ``Select``).
    2. Precomputes opponent-side distribution stats (constant).
    3. For each swap position, batch-evaluates all valid candidates in a single
       ``model.predict_proba`` call.
    4. Accepts the first improving swap per iteration (first-improve strategy,
       matching Go implementation).
    """
    seed = greedy_select(pool, weights, constraints)
    team_ids = {p.player_id for p in seed}
    rest = [p for p in pool if p.player_id not in team_ids]

    opponent_team_number = 2 if team_is_team1 else 1
    opponent_stats = _precompute_fixed_team_stats(opponent_features, opponent_team_number)

    # Extend match context with format one-hot columns using the shared aggregation helper.
    context_extended = aggregate_team_features_from_player_maps({}, {}, match_context, format_code=format_code or "")

    current_prob = _evaluate_single_team(
        seed, opponent_features, context_extended, team_is_team1, model, format_code or ""
    )
    eval_count = 1
    iterations_used = 0

    for iteration in range(max_iterations):
        improved = False
        budget_exhausted = False

        for i in range(len(seed)):
            valid_indices: List[int] = []
            for j, candidate in enumerate(rest):
                trial = list(seed)
                trial[i] = candidate
                if _satisfies_constraints(trial, constraints):
                    valid_indices.append(j)

            if not valid_indices:
                continue

            if max_evals > 0:
                remaining = max_evals - eval_count
                if remaining <= 0:
                    budget_exhausted = True
                    break
                if len(valid_indices) > remaining:
                    valid_indices = valid_indices[:remaining]

            candidates = [rest[j] for j in valid_indices]
            probas = _batch_evaluate_candidates(
                seed, i, candidates, opponent_stats, context_extended, team_is_team1, model
            )
            eval_count += len(candidates)

            best_local = int(np.argmax(probas))
            if probas[best_local] > current_prob:
                old_player = seed[i]
                best_rest_idx = valid_indices[best_local]
                seed[i] = rest[best_rest_idx]
                rest[best_rest_idx] = old_player
                current_prob = float(probas[best_local])
                improved = True
                break

            if max_evals > 0 and eval_count >= max_evals:
                budget_exhausted = True
                break

        iterations_used = iteration + 1
        if not improved or budget_exhausted:
            break

    seed.sort(key=lambda p: p.name)
    return OptimizationResult(
        selected=seed,
        win_probability=current_prob,
        iterations_used=iterations_used,
        evals_performed=eval_count,
    )
