"""Pick the XI from a pool: either the one that maximises the objective model's P(win)
against a fixed opponent XI, or -- where the objective does not rank -- the rating-ordered
pick that is not optimised at all.

Search: greedy seed by individual marginal value, then steepest-ascent single swaps, then
pair swaps once single swaps stall, with restarts. The objective is additive in the XI
features (see ``ml.xi.train``) so the surface is smooth; pair swaps exist for the role
constraints, where two changes that only help together are common (drop a batter and a
bowler, bring in two all-rounders).

Constraints are expressed through the same as-of vectors the model reads -- a bowling option
is a player whose expected balls bowled clears the format threshold -- so "min bowlers" means
the same thing to the constraint and to the ``n_bowlers`` feature.

``select_xi_by_ratings`` is the second mode (H-17): TEST has no objective that ranks -- the
holdout AUC sits under the 0.65 line in every feature family -- so it is offered a selection
but never an optimised one. It is the search's own seed order, stopped before the search,
and it evaluates no model, which is what lets the formats that do not rank keep a selection
without any of the weights P-5 deleted.
"""

from __future__ import annotations

import itertools
import logging
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Sequence

import numpy as np

from ml.xi import contract as C
from ml.xi.ratings import aggregate_side, xi_feature_vector
from ml.xi.store import XiStore

logger = logging.getLogger(__name__)

# The formats served a rating-ordered XI instead of an optimised one, each with its reason.
# Two rules put a format here, and the reason says which. H-17 (P-5): an objective whose
# holdout AUC is under 0.65 does not rank, so it is not searched over -- TEST, by
# measurement, every feature family leaves it near 0.6. E5 (P-7, plan §8.8): an objective
# that ranks but whose preference between one side's consecutive elevens does not agree
# with the result change at the bar derived from its own claimed effect size has not shown
# that it selects, so the search is not served there either. The harness re-decides E5
# every run and reports whether this policy still agrees with it (``selection_decision``);
# the policy itself is set here by hand, from the report, like E2's. A format absent from
# the map is optimised. Mirrored by the go-app xi path (``notOptimisedReasons``), which
# carries the reason to the UI as the *Not optimised* notice.
NOT_OPTIMISED_REASONS: Dict[str, str] = {
    "TEST": "no win objective that ranks (holdout AUC below 0.65, H-17)",
    "T20": (
        "the objective ranks (holdout AUC 0.72) but has not shown it selects: E5 lineup-only agreement "
        "0.490 over 1,358 walk-forward pairs against the derived bar 0.501, and 0.509 against 0.512 on all "
        "12,413 development pairs (plan §8.8)"
    ),
}
OPTIMISED_SELECTION_FORMATS = frozenset(fmt for fmt in C.FORMAT_CODES if fmt not in NOT_OPTIMISED_REASONS)


@dataclass
class Constraints:
    team_size: int = 11
    min_bowlers: int = 5
    require_keeper: bool = True
    must_include: List[str] = field(default_factory=list)
    must_exclude: List[str] = field(default_factory=list)


@dataclass
class SelectionResult:
    selected: List[str]
    win_probability: float
    evaluations: int
    improved_over_seed: float
    trace: List[str] = field(default_factory=list)


class _Pool:
    """Per-player vectors for the pool, indexable by position; the opponent side is fixed."""

    def __init__(
        self,
        store: XiStore,
        format_code: str,
        pool_keys: Sequence[str],
        opponent_keys: Sequence[str],
        team_is_team1: bool,
    ):
        self.fmt = format_code
        self.keys = list(pool_keys)
        self.vectors = store.side_vectors(format_code, self.keys)
        # A rating-ordered pick reads no opponent and scores nothing, so it may pass an
        # empty opponent; ``score_many`` is then the caller's error, not a silent zero.
        self.opponent = (
            aggregate_side(store.side_vectors(format_code, opponent_keys), format_code) if opponent_keys else None
        )
        self.models = store.models[format_code]
        self.team_is_team1 = team_is_team1
        self.evaluations = 0

    def is_bowler(self, i: int) -> bool:
        return bool(C.is_bowling_option(self.vectors["exp_balls_bowled"][i], self.fmt))

    def is_keeper(self, i: int) -> bool:
        return bool(self.vectors["keeper"][i] > 0)

    def feasible(self, idx: Sequence[int], c: Constraints) -> bool:
        if len(idx) != c.team_size:
            return False
        if sum(self.is_bowler(i) for i in idx) < c.min_bowlers:
            return False
        if c.require_keeper and not any(self.is_keeper(i) for i in idx):
            return False
        return True

    def score_many(self, candidates: Sequence[Sequence[int]]) -> np.ndarray:
        """P(our side wins) for each candidate XI, marginalised over who bats first: the
        objective is scored with the candidate as team1 and as team2 and the two are
        averaged. ``team_is_team1`` then only says which orientation is reported first;
        the number is the same either way, which is what an argmax over XIs should rely on."""
        if self.opponent is None:
            raise ValueError("scoring an XI needs an opponent XI; this pool was built without one")
        rows = []
        for idx in candidates:
            side = aggregate_side({k: v[list(idx)] for k, v in self.vectors.items()}, self.fmt)
            rows.append(xi_feature_vector(side, self.opponent, self.models.objective_cols))
            rows.append(xi_feature_vector(self.opponent, side, self.models.objective_cols))
        self.evaluations += len(candidates)
        p = self.models.objective_proba(np.vstack(rows))
        return 0.5 * (p[0::2] + (1.0 - p[1::2]))

    def score(self, idx: Sequence[int]) -> float:
        return float(self.score_many([idx])[0])


def _locked_and_banned(pool: _Pool, c: Constraints) -> tuple:
    """The must-include / must-exclude keys as pool positions, ignoring ids not in the pool."""
    key_index = {k: i for i, k in enumerate(pool.keys)}
    locked = [key_index[k] for k in c.must_include if k in key_index]
    banned = {key_index[k] for k in c.must_exclude if k in key_index}
    return locked, banned


def _greedy_seed(pool: _Pool, c: Constraints, locked: List[int], banned: set) -> Optional[List[int]]:
    """Rank players by their solo marginal value, fill role constraints first, then the rest."""
    n = len(pool.keys)
    order = list(
        np.argsort(
            -(
                pool.vectors["bat_rate"] * pool.vectors["exp_balls_faced"]
                + pool.vectors["bowl_rate"] * pool.vectors["exp_balls_bowled"]
                + (pool.vectors["pelo"] - C.ELO_INITIAL) / 400.0
            )
        )
    )
    chosen = list(locked)
    if c.require_keeper and not any(pool.is_keeper(i) for i in chosen):
        for i in order:
            if i not in chosen and i not in banned and pool.is_keeper(i):
                chosen.append(i)
                break
    while sum(pool.is_bowler(i) for i in chosen) < c.min_bowlers:
        pick = next((i for i in order if i not in chosen and i not in banned and pool.is_bowler(i)), None)
        if pick is None:
            break
        chosen.append(pick)
    for i in order:
        if len(chosen) >= c.team_size:
            break
        if i not in chosen and i not in banned:
            chosen.append(i)
    if n < c.team_size or not pool.feasible(chosen, c):
        return None
    return chosen[: c.team_size]


def _best_neighbour(pool: _Pool, current: List[int], c: Constraints, locked: set, banned: set, pairs: bool):
    outs = [i for i in current if i not in locked]
    ins = [j for j in range(len(pool.keys)) if j not in current and j not in banned]
    candidates, moves = [], []
    if not pairs:
        for o in outs:
            for j in ins:
                cand = [j if x == o else x for x in current]
                if pool.feasible(cand, c):
                    candidates.append(cand)
                    moves.append(f"{pool.keys[o]}->{pool.keys[j]}")
    else:
        for o1, o2 in itertools.combinations(outs, 2):
            for j1, j2 in itertools.combinations(ins, 2):
                cand = [j1 if x == o1 else (j2 if x == o2 else x) for x in current]
                if pool.feasible(cand, c):
                    candidates.append(cand)
                    moves.append(f"{pool.keys[o1]},{pool.keys[o2]}->{pool.keys[j1]},{pool.keys[j2]}")
    if not candidates:
        return None, None, None
    scores = pool.score_many(candidates)
    k = int(np.argmax(scores))
    return candidates[k], float(scores[k]), moves[k]


def select_xi(
    store: XiStore,
    format_code: str,
    pool_keys: Sequence[str],
    opponent_keys: Sequence[str],
    constraints: Optional[Constraints] = None,
    team_is_team1: bool = True,
    max_evaluations: int = 20000,
    max_pair_swap_pool: int = 30,
) -> SelectionResult:
    c = constraints or Constraints()
    pool = _Pool(store, format_code, pool_keys, opponent_keys, team_is_team1)
    locked, banned = _locked_and_banned(pool, c)
    seed = _greedy_seed(pool, c, locked, banned)
    if seed is None:
        raise ValueError("pool cannot satisfy the constraints (size / bowlers / keeper)")
    current, current_score = seed, pool.score(seed)
    seed_score = current_score
    trace: List[str] = []
    while pool.evaluations < max_evaluations:
        cand, score, move = _best_neighbour(pool, current, c, set(locked), banned, pairs=False)
        if cand is not None and score > current_score + 1e-9:
            current, current_score = cand, score
            trace.append(f"swap {move} -> {score:.4f}")
            continue
        if len(pool.keys) > max_pair_swap_pool:
            break
        cand, score, move = _best_neighbour(pool, current, c, set(locked), banned, pairs=True)
        if cand is not None and score > current_score + 1e-9:
            current, current_score = cand, score
            trace.append(f"pair {move} -> {score:.4f}")
            continue
        break
    logger.info(
        "xi optimiser: %s, %d evaluations, seed %.4f -> %.4f", format_code, pool.evaluations, seed_score, current_score
    )
    return SelectionResult(
        selected=[pool.keys[i] for i in current],
        win_probability=current_score,
        evaluations=pool.evaluations,
        improved_over_seed=current_score - seed_score,
        trace=trace,
    )


def select_xi_by_ratings(
    store: XiStore,
    format_code: str,
    pool_keys: Sequence[str],
    constraints: Optional[Constraints] = None,
) -> List[str]:
    """The rating-ordered XI: the keeper and the bowlers the constraints ask for, then the
    highest-rated players left, all read from the same as-of vectors the win model reads.

    No opponent, no objective, no search -- so nothing here depends on a model whose
    holdout AUC does not clear H-17's 0.65 line, and the caller must label the answer as
    not optimised (the API and the UI both do).
    """
    c = constraints or Constraints()
    pool = _Pool(store, format_code, pool_keys, opponent_keys=[], team_is_team1=True)
    locked, banned = _locked_and_banned(pool, c)
    chosen = _greedy_seed(pool, c, locked, banned)
    if chosen is None:
        raise ValueError("pool cannot satisfy the constraints (size / bowlers / keeper)")
    logger.info("xi rating-ordered pick: %s, pool %d, no objective evaluated", format_code, len(pool.keys))
    return [pool.keys[i] for i in chosen]


def marginal_values(
    store: XiStore, format_code: str, xi_keys: Sequence[str], opponent_keys: Sequence[str], team_is_team1: bool = True
) -> Dict[str, float]:
    """P(win) with the XI minus P(win) with each player replaced by a neutral, average player.
    The explanation the selector shows: who is carrying the side."""
    pool = _Pool(store, format_code, xi_keys, opponent_keys, team_is_team1)
    base_idx = list(range(len(xi_keys)))
    base = pool.score(base_idx)
    out: Dict[str, float] = {}
    for j, key in enumerate(xi_keys):
        vec = {k: v.copy() for k, v in pool.vectors.items()}
        for name in ("bat_rate", "bat_wrate", "bowl_rate", "bowl_wrate"):
            vec[name][j] = 0.0
        vec["pelo"][j] = C.ELO_INITIAL
        vec["career"][j] = float(np.median(pool.vectors["career"]))
        vec["career_all"][j] = float(np.median(pool.vectors["career_all"]))
        saved = pool.vectors
        pool.vectors = vec
        out[key] = base - pool.score(base_idx)
        pool.vectors = saved
    return out
