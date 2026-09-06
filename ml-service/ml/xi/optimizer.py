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
from typing import Dict, List, Optional, Sequence, Tuple

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
        "the objective ranks (walk-forward AUC 0.70) but has not shown it selects: E5 lineup-only agreement "
        "0.503 over 2,168 walk-forward pairs against the derived bar 0.506 on the rotated folds (0.490 against "
        "0.501 when P-7 decided it; 0.509 against 0.512 on all 12,413 development pairs), and neither of A-3's "
        "feature families changed the verdict (plan §8.8, §8.11)"
    ),
}
OPTIMISED_SELECTION_FORMATS = frozenset(fmt for fmt in C.FORMAT_CODES if fmt not in NOT_OPTIMISED_REASONS)

# The constraint state a "why this player" card may name (P1-3): the requirements in this
# module that a selected player answers, each read off the same as-of vectors the objective
# reads. Two, not three: ``must_include`` is deliberately absent even now that go-app sends
# and this module enforces it (B-10), because a lock is the caller's own input echoed back
# and not something the selection read *about* the player -- and the answer already reports
# it per side (``selection.must_include``) rather than per card.
# Wire vocabulary, declared once in contracts/ops-console.contract.json and asserted from
# every side (H-24).
ROLE_KEEPER = "keeper"
ROLE_BOWLING_OPTION = "bowling_option"
SELECTION_ROLES: Tuple[str, ...] = (ROLE_KEEPER, ROLE_BOWLING_OPTION)


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
    """The must-include / must-exclude keys as pool positions.

    An id the pool does not hold has no position and so cannot be locked; it is dropped
    here and caught by ``_seed_or_conflict``, which is the one place that turns "this
    cannot be satisfied" into a sentence. Dropping it silently was B-10's shape.
    """
    key_index = {k: i for i, k in enumerate(pool.keys)}
    locked = [key_index[k] for k in c.must_include if k in key_index]
    banned = {key_index[k] for k in c.must_exclude if k in key_index}
    return locked, banned


def rating_order_score(vectors: Dict[str, np.ndarray]) -> np.ndarray:
    """The composite the selection orders a pool by: decayed batting impact per innings,
    plus decayed bowling impact per innings, plus the player's Elo above the initial rating
    in units of 400 points.

    It is the *only* ordering this module has. ``_greedy_seed`` fills the constraints and
    then the rest of the eleven by it, so it is the seed every search starts from and the
    whole answer where a format is not searched at all (``select_xi_by_ratings``). One
    definition, so what the card calls a player's rating is what actually ranked him
    (P1-3) -- and so B-8, which is this composite disagreeing with the display model, has
    one place to point at.
    """
    return (
        vectors["bat_rate"] * vectors["exp_balls_faced"]
        + vectors["bowl_rate"] * vectors["exp_balls_bowled"]
        + (vectors["pelo"] - C.ELO_INITIAL) / 400.0
    )


def rating_percentiles(scores: np.ndarray) -> np.ndarray:
    """Where each ``rating_order_score`` stands in its own pool, 0-100: the share of the
    pool the player outranks, as strictly-lower count over pool size minus one.

    So the pool's top player reads 100 and its bottom reads 0, tied players read the same,
    and the number means "this pool", never "all cricketers": the pool is what the
    selection actually chose out of.
    """
    n = len(scores)
    if n <= 1:
        return np.full(n, 100.0)
    lower = (scores[:, None] > scores[None, :]).sum(axis=1)
    return 100.0 * lower / (n - 1)


def _greedy_seed(pool: _Pool, c: Constraints, locked: List[int], banned: set) -> List[int]:
    """Rank players by their solo marginal value, fill role constraints first, then the rest.

    It returns the best it could assemble, feasible or not; ``_seed_or_conflict`` is what
    decides. Keeping the two apart is what lets an infeasible answer be *explained* rather
    than reported as one undifferentiated ``None``.
    """
    order = list(np.argsort(-rating_order_score(pool.vectors)))
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
    return chosen


class ConstraintConflict(ValueError):
    """A constraint set no eleven in this pool can satisfy, with the reason named.

    A ``ValueError`` so the route's existing 422 still catches it; what it adds is *which*
    constraint failed. "pool cannot satisfy the constraints (size / bowlers / keeper)"
    leaves a caller who locked two players guessing between three answers, and a locked
    player is exactly the case where the caller can act on the difference (B-10).
    """


def _unresolved_must_include(pool: _Pool, c: Constraints) -> List[str]:
    """must_include ids this pool does not hold, and so cannot lock anyone to."""
    held = set(pool.keys)
    return [key for key in c.must_include if key not in held]


def _conflict_reason(pool: _Pool, c: Constraints, locked: List[int], chosen: List[int]) -> str:
    """Why this pool cannot field an eleven under these constraints, most specific first.

    Read off the seed the search would have started from, so the sentence describes what
    actually happened rather than a second opinion about it.
    """
    if len(locked) > c.team_size:
        return f"must_include names {len(locked)} players and the team holds {c.team_size}"
    if len(pool.keys) < c.team_size:
        return f"the pool holds {len(pool.keys)} players and the team needs {c.team_size}"
    lock = f" alongside the {len(locked)} must_include player(s)" if locked else ""
    if len(chosen) > c.team_size:
        return (
            f"the keeper and bowling options asked for do not fit in {c.team_size} places{lock}: "
            f"filling every constraint needs {len(chosen)} players"
        )
    if c.require_keeper and not any(pool.is_keeper(i) for i in chosen):
        return f"no wicketkeeper can be selected{lock}"
    bowlers = int(sum(pool.is_bowler(i) for i in chosen))
    if bowlers < c.min_bowlers:
        return f"only {bowlers} of the {c.min_bowlers} bowling options asked for can be selected{lock}"
    return f"the pool cannot fill {c.team_size} places{lock}"


def _seed_or_conflict(pool: _Pool, c: Constraints, locked: List[int], banned: set) -> List[int]:
    """The seed, or a ``ConstraintConflict`` naming the constraint that made one impossible.

    An unresolvable ``must_include`` id is refused even when the eleven would otherwise be
    feasible: the caller asked for a player this pool cannot field, and answering with a
    perfectly good eleven that does not hold him is the silent relaxation B-10 records.
    """
    unresolved = _unresolved_must_include(pool, c)
    if unresolved:
        raise ConstraintConflict(
            f"must_include names {len(unresolved)} player(s) this pool does not hold: {', '.join(unresolved)}"
        )
    chosen = _greedy_seed(pool, c, locked, banned)
    if not pool.feasible(chosen, c):
        raise ConstraintConflict(_conflict_reason(pool, c, locked, chosen))
    return chosen


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
    seed = _seed_or_conflict(pool, c, locked, banned)
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
    chosen = _seed_or_conflict(pool, c, locked, banned)
    logger.info("xi rating-ordered pick: %s, pool %d, no objective evaluated", format_code, len(pool.keys))
    return [pool.keys[i] for i in chosen]


@dataclass(frozen=True)
class BestAlternative:
    """The pool player the objective would most like to have instead of this one, and what
    swapping him in costs: P(win) with the selected player minus P(win) with the alternative.

    It is the search's own last question, asked once more and reported: exactly the
    candidates ``_best_neighbour`` scores, under the same constraints, against the same
    opponent XI. So the gap is a point estimate from one model evaluation and carries no
    interval -- nothing in this module produces one -- and a value at or below zero means
    the search stopped on its evaluation budget rather than at a local optimum.
    """

    player_key: str
    win_probability_gap: float


@dataclass(frozen=True)
class SelectionReason:
    """Why one selected player is in the eleven, in the terms the selection itself used.

    Every field is read off the selection's own state: ``roles`` from the constraint
    predicates ``_Pool`` evaluates, ``selection_rating`` and ``rating_percentile`` from the
    composite ``_greedy_seed`` orders by, ``best_alternative`` from the swap candidates the
    search scores. Nothing here is a second opinion about the player (P1-3 § 1).
    """

    roles: List[str]
    selection_rating: float
    rating_percentile: float
    pool_size: int
    best_alternative: Optional[BestAlternative] = None
    #: Why there is no alternative, where the objective was asked and could not name one.
    #: ``None`` on the rating-ordered path, where nothing was maximised and the response's
    #: own ``optimised: false`` is the reason.
    best_alternative_note: Optional[str] = None


NO_FEASIBLE_ALTERNATIVE = (
    "no swap from the rest of this pool keeps the eleven inside its constraints, so the "
    "objective was never offered a replacement for this player"
)


def _best_alternatives(pool: _Pool, current: List[int], c: Constraints, banned: set) -> Dict[int, BestAlternative]:
    """For each player in the eleven, the excluded pool player whose swap-in the objective
    scores highest, and the P(win) that swap costs.

    The candidate set is ``_best_neighbour``'s single-swap neighbourhood -- every feasible
    one-for-one replacement -- scored in one batch, so the answer is the search's own
    ranking rather than a new one invented for the card.
    """
    base = pool.score(current)
    ins = [j for j in range(len(pool.keys)) if j not in current and j not in banned]
    candidates: List[List[int]] = []
    swaps: List[Tuple[int, int]] = []
    for out in current:
        for j in ins:
            candidate = [j if x == out else x for x in current]
            if pool.feasible(candidate, c):
                candidates.append(candidate)
                swaps.append((out, j))
    if not candidates:
        return {}
    scores = pool.score_many(candidates)
    best: Dict[int, Tuple[int, float]] = {}
    for (out, j), score in zip(swaps, scores):
        if out not in best or score > best[out][1]:
            best[out] = (j, float(score))
    return {
        out: BestAlternative(player_key=pool.keys[j], win_probability_gap=base - score)
        for out, (j, score) in best.items()
    }


def selection_reasons(
    store: XiStore,
    format_code: str,
    xi_keys: Sequence[str],
    pool_keys: Sequence[str],
    opponent_keys: Sequence[str] = (),
    constraints: Optional[Constraints] = None,
    team_is_team1: bool = True,
) -> Dict[str, SelectionReason]:
    """What the selection read about each player it picked, keyed by registry id (P1-3).

    ``opponent_keys`` empty is the rating-ordered path: nothing was maximised, so no
    alternative is scored and none is reported. With an opponent it is the searched path,
    and the eleven is measured against the pool it was searched out of.

    A player the pool does not hold gets no entry rather than a blank one -- the eleven
    would then not be the eleven this pool produced, and a card explaining a selection that
    did not happen is the failure this whole item exists to avoid.
    """
    c = constraints or Constraints()
    pool = _Pool(store, format_code, pool_keys, opponent_keys, team_is_team1)
    position = {key: i for i, key in enumerate(pool.keys)}
    scores = rating_order_score(pool.vectors)
    percentiles = rating_percentiles(scores)

    current = [position[key] for key in xi_keys if key in position]
    missing = [key for key in xi_keys if key not in position]
    if missing:
        logger.warning("xi.selection_reasons.unpooled_players: %s", missing[:10])

    alternatives: Dict[int, BestAlternative] = {}
    if opponent_keys and current:
        _, banned = _locked_and_banned(pool, c)
        alternatives = _best_alternatives(pool, current, c, banned)

    out: Dict[str, SelectionReason] = {}
    for i in current:
        roles = []
        if pool.is_keeper(i):
            roles.append(ROLE_KEEPER)
        if pool.is_bowler(i):
            roles.append(ROLE_BOWLING_OPTION)
        alternative = alternatives.get(i)
        out[pool.keys[i]] = SelectionReason(
            roles=roles,
            selection_rating=float(scores[i]),
            rating_percentile=float(percentiles[i]),
            pool_size=len(pool.keys),
            best_alternative=alternative,
            best_alternative_note=(
                NO_FEASIBLE_ALTERNATIVE if opponent_keys and current and alternative is None else None
            ),
        )
    return out


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
