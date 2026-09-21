"""E5 -- the natural experiment for selection, lineup-only, in the L4 harness (plan §8.6-§8.8).

Selection changes one thing: the eleven. The two gates S-6 shipped on (swap monotonicity,
specific-XI-beyond-typical-XI) ask whether the objective ranks elevens; neither varies one
side while holding the rest of the world still. E5 does. For each consecutive pair of one
side's matches in a format with 1-3 lineup changes, **both** elevens are scored against
match k+1's opponent at match k+1's as-of ratings, so nothing varies but the eleven:

    Δobjective = P(win | XI_{k+1}, opp_{k+1}, as-of k+1) - P(win | XI_k, opp_{k+1}, as-of k+1)
    Δresult    = won_{k+1} - won_k                            (scored on the pairs where it is +/-1)

and the metric is sign agreement between the two over the pairs whose result moved.

**§5's as-played form is not implemented, on purpose.** Scoring each match with its own
opponent makes a nonzero Δresult an identity on ``won_k`` (Δresult = -1 iff the side won
match k), and match k+1's as-of state already contains match k's result, so the metric
measures whether the rating update anticipates mean reversion -- not selection (§8.6).

**The bar is derived, not chosen** (§8.7 point 3, §8.8). The objective claims a median
|Δ| of about 0.02 win probability for a 1-3 player change; a sign-agreement rate cannot
exceed what that effect size implies. ``derived_bar`` takes the objective at its word --
every fixture's outcome is Bernoulli at the objective's own probability -- and simulates
the agreement rate an exactly-right objective would produce on these pairs; the bar is
that distribution's 5th percentile, so it already carries the sampling noise of the pairs
available. A format then passes or fails a bar it could in principle reach.

Walk-forward (H-19): a pair is scored by the objective of the fold whose window holds
match k+1, fitted strictly before that fold's cutoff; the locked window is scored beside
the folds and labelled, never used for a choice.
"""

from __future__ import annotations

import logging
import math
from collections import defaultdict
from dataclasses import dataclass, field
from typing import Any, Callable, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi.asof import AsOfRatings
from ml.xi.folds import summarise_over_folds
from ml.xi.ratings import aggregate_side, xi_feature_vector
from ml.xi.sources import MatchSource

logger = logging.getLogger(__name__)

XI_SIZE = 11
MIN_CHANGES = 1
MAX_CHANGES = 3
#: Bernoulli replicates behind the derived bar, and the quantile of their agreement rate
#: that is the bar: an exactly-right objective falls below it one run in twenty.
BAR_REPLICATES = 2000
BAR_QUANTILE = 0.05
BAR_SEED = 20260902

DEFINITION = (
    "for each consecutive pair of one side's matches with 1-3 lineup changes, both elevens are scored "
    "against match k+1's opponent at match k+1's as-of ratings, and the sign of the objective's difference "
    "is compared with the sign of the result change over the pairs whose result moved"
)
WHY_NOT_AS_PLAYED = (
    "§5's as-played form is not computed: with each match scored against its own opponent a nonzero Δresult "
    "is an identity on won_k, which match k+1's as-of state already contains, so it scores mean reversion "
    "rather than selection (§8.6)"
)

#: P(team1 wins) for a matrix of XI feature rows -- a fold's fitted objective or a run's.
Proba = Callable[[np.ndarray], np.ndarray]


@dataclass
class LineupPair:
    """One side's consecutive matches k and k+1, 1-3 players apart."""

    format_code: str
    team: str
    before_match_id: Any
    after_match_id: Any
    after_date: pd.Timestamp
    changes: int
    before_keys: Tuple[str, ...]
    after_keys: Tuple[str, ...]
    #: Aggregates from the frame's own rows: the eleven that played match k+1, and its opponent.
    after_side: Dict[str, float]
    opponent_side: Dict[str, float]
    #: Match k's probability is read from its own frame row (for the derived bar's null world).
    before_side_played: Dict[str, float]
    before_opponent_played: Dict[str, float]
    d_result: int
    #: Match k's eleven with match k+1's as-of ratings; filled by ``score_previous_elevens``.
    before_side: Optional[Dict[str, float]] = field(default=None)


def _side_from_row(row: Any, prefix: str) -> Dict[str, float]:
    return {stem: float(getattr(row, f"{prefix}_{stem}")) for stem in C.SIDE_FEATURE_STEMS}


def build_pairs(frame: pd.DataFrame, player_frame: pd.DataFrame) -> List[LineupPair]:
    """Every side's consecutive decided matches per format that differ by 1-3 players, with
    full elevens on both sides of match k+1 (an oversized squad is not an eleven)."""
    if frame.empty or player_frame.empty:
        return []
    keys_by_side: Dict[Tuple[Any, int], Tuple[str, ...]] = {
        (match_id, int(side)): tuple(group.player_key.tolist())
        for (match_id, side), group in player_frame.groupby(["match_id", "side"], sort=False)
    }
    ordered = frame.sort_values(["match_date", "match_id"], kind="stable")
    history: Dict[Tuple[str, str], List[Tuple[Any, int]]] = defaultdict(list)
    for row in ordered.itertuples():
        history[(row.format_code, row.team1)].append((row, 1))
        history[(row.format_code, row.team2)].append((row, 2))
    pairs: List[LineupPair] = []
    for (format_code, team), appearances in history.items():
        for (before_row, before_side), (after_row, after_side) in zip(appearances, appearances[1:]):
            before_keys = keys_by_side.get((before_row.match_id, before_side), ())
            after_keys = keys_by_side.get((after_row.match_id, after_side), ())
            opponent_keys = keys_by_side.get((after_row.match_id, 3 - after_side), ())
            if not (len(before_keys) == len(after_keys) == len(opponent_keys) == XI_SIZE):
                continue
            changes = len(set(after_keys) - set(before_keys))
            if not MIN_CHANGES <= changes <= MAX_CHANGES:
                continue
            won_before = float(getattr(before_row, C.TARGET_COL)) == (1.0 if before_side == 1 else 0.0)
            won_after = float(getattr(after_row, C.TARGET_COL)) == (1.0 if after_side == 1 else 0.0)
            own, opp = ("t1", "t2") if after_side == 1 else ("t2", "t1")
            own_before, opp_before = ("t1", "t2") if before_side == 1 else ("t2", "t1")
            pairs.append(
                LineupPair(
                    format_code=format_code,
                    team=team,
                    before_match_id=before_row.match_id,
                    after_match_id=after_row.match_id,
                    after_date=pd.Timestamp(after_row.match_date),
                    changes=changes,
                    before_keys=before_keys,
                    after_keys=after_keys,
                    after_side=_side_from_row(after_row, own),
                    opponent_side=_side_from_row(after_row, opp),
                    before_side_played=_side_from_row(before_row, own_before),
                    before_opponent_played=_side_from_row(before_row, opp_before),
                    d_result=int(won_after) - int(won_before),
                )
            )
    logger.info("E5: %d lineup pairs with %d-%d changes", len(pairs), MIN_CHANGES, MAX_CHANGES)
    return pairs


def score_previous_elevens(
    pairs: Sequence[LineupPair], source: MatchSource, gender_split_context: bool = False
) -> Dict[str, Any]:
    """Fill each pair's ``before_side``: match k's eleven read from the as-of serving
    state at match k+1's date -- one advancing pass over the source, in date order (the
    same path a backtest's ``as_of`` request takes). The eleven that did play match k+1 is
    read from the same state and compared with the frame's row as a parity check on the
    pairing itself: a wrong side, or a state that drifted from the training pass, shows
    up here as a nonzero difference."""
    asof = AsOfRatings(source, gender_split_context=gender_split_context)
    max_difference = 0.0
    scored = 0
    for pair in sorted(pairs, key=lambda p: (p.after_date, str(p.after_match_id))):
        on = pair.after_date.date()
        state = asof.state_as_of(on)
        pair.before_side = aggregate_side(
            state.side_vectors(pair.format_code, list(pair.before_keys), on=on), pair.format_code
        )
        fielded = aggregate_side(state.side_vectors(pair.format_code, list(pair.after_keys), on=on), pair.format_code)
        max_difference = max(
            max_difference, max(abs(fielded[stem] - pair.after_side[stem]) for stem in C.SIDE_FEATURE_STEMS)
        )
        scored += 1
    logger.info(
        "E5: previous elevens scored for %d pairs; fielded-eleven parity max abs difference %.2e",
        scored,
        max_difference,
    )
    return {"pairs_scored": scored, "fielded_eleven_max_abs_difference": max_difference}


def _marginalised(
    proba: Proba, own: Sequence[Dict[str, float]], opp: Sequence[Dict[str, float]], columns: List[str]
) -> np.ndarray:
    """P(own wins) for each (own, opp) pair, both batting orders averaged (H-3)."""
    if not own:
        return np.empty(0)
    rows = []
    for a, b in zip(own, opp):
        rows.append(xi_feature_vector(a, b, columns))
        rows.append(xi_feature_vector(b, a, columns))
    p = proba(np.vstack(rows))
    return 0.5 * (p[0::2] + (1.0 - p[1::2]))


@dataclass(frozen=True)
class ScoredPairs:
    """The objective's three probabilities per pair, and the result change."""

    p_before_played: np.ndarray  # match k as played (its own opponent, its own as-of)
    p_previous: np.ndarray  # match k's eleven in match k+1's fixture, at k+1's as-of
    p_after: np.ndarray  # match k+1 as played
    d_result: np.ndarray
    changes: np.ndarray

    @property
    def d_lineup(self) -> np.ndarray:
        return self.p_after - self.p_previous

    def __len__(self) -> int:
        return int(len(self.d_result))


def score_pairs(pairs: Sequence[LineupPair], proba: Proba, columns: List[str]) -> ScoredPairs:
    scorable = [p for p in pairs if p.before_side is not None]
    return ScoredPairs(
        p_before_played=_marginalised(
            proba, [p.before_side_played for p in scorable], [p.before_opponent_played for p in scorable], columns
        ),
        p_previous=_marginalised(
            proba, [p.before_side for p in scorable], [p.opponent_side for p in scorable], columns
        ),
        p_after=_marginalised(proba, [p.after_side for p in scorable], [p.opponent_side for p in scorable], columns),
        d_result=np.asarray([p.d_result for p in scorable], dtype=int),
        changes=np.asarray([p.changes for p in scorable], dtype=int),
    )


def agreement(d_objective: np.ndarray, d_result: np.ndarray) -> Dict[str, Any]:
    """Sign agreement over the pairs where the result moved and the objective has a
    preference; both exclusions are counted so the denominator is explicit."""
    moved = d_result != 0
    preference = d_objective != 0
    scored = moved & preference
    n = int(scored.sum())
    agreed = int(((d_objective > 0) == (d_result > 0))[scored].sum())
    rate = agreed / n if n else None
    se = math.sqrt(rate * (1 - rate) / n) if rate is not None and n else None
    return {
        "pairs_scored": n,
        "agreed": agreed,
        "agreement": rate,
        "standard_error": se,
        "ci95": [rate - 1.96 * se, rate + 1.96 * se] if se is not None else None,
        "excluded_result_unchanged": int((~moved).sum()),
        "excluded_objective_indifferent": int((moved & ~preference).sum()),
    }


def effect_size(d_objective: np.ndarray) -> Dict[str, Any]:
    """How much the objective itself claims a 1-3 player change is worth; the bar below is
    a function of this, so it is reported beside it."""
    values = np.abs(d_objective)
    if not len(values):
        return {"n": 0, "median_abs": None, "mean_abs": None, "p90_abs": None}
    return {
        "n": int(len(values)),
        "median_abs": float(np.median(values)),
        "mean_abs": float(values.mean()),
        "p90_abs": float(np.quantile(values, 0.9)),
    }


def derived_bar(
    scored: ScoredPairs, replicates: int = BAR_REPLICATES, quantile: float = BAR_QUANTILE, seed: int = BAR_SEED
) -> Dict[str, Any]:
    """The agreement rate an exactly-right objective would produce on these pairs.

    Null world: match k's result is Bernoulli at the objective's P(win) for match k as
    played, match k+1's at the objective's P(win) for match k+1 as played -- so the
    lineup-only Δ is exactly what the objective says it is, and the opponent change between
    the two matches is priced the way the objective prices it. The expected rate has a
    closed form; the replicates give its sampling distribution for this many pairs, whose
    ``quantile`` is the bar. Pairs on which the objective has no preference are excluded
    here as they are from the observed rate."""
    preference = scored.d_lineup != 0
    a, b, up_claimed = scored.p_before_played[preference], scored.p_after[preference], scored.d_lineup[preference] > 0
    n = int(preference.sum())
    if n == 0:
        return {"n_pairs": 0, "expected_if_exactly_right": None, "bar": None}
    p_up, p_down = (1.0 - a) * b, a * (1.0 - b)
    expected = float(np.where(up_claimed, p_up, p_down).sum() / (p_up + p_down).sum())
    rng = np.random.default_rng(seed)
    rates = np.empty(replicates)
    for r in range(replicates):
        won_before = rng.random(n) < a
        won_after = rng.random(n) < b
        moved = won_before != won_after
        agreed = (won_after & ~won_before) == up_claimed
        n_moved = int(moved.sum())
        rates[r] = float((agreed & moved).sum() / n_moved) if n_moved else np.nan
    rates = rates[~np.isnan(rates)]
    return {
        "n_pairs": n,
        "expected_if_exactly_right": expected,
        "simulated_mean": float(rates.mean()),
        "simulated_sd": float(rates.std()),
        "bar": float(np.quantile(rates, quantile)),
        "bar_quantile": quantile,
        "replicates": int(len(rates)),
        "seed": seed,
    }


def _section(scored: ScoredPairs) -> Dict[str, Any]:
    out = agreement(scored.d_lineup, scored.d_result)
    out["effect_size"] = effect_size(scored.d_lineup)
    out["derived_bar"] = derived_bar(scored)
    bar = out["derived_bar"]["bar"]
    out["passes_derived_bar"] = None if out["agreement"] is None or bar is None else bool(out["agreement"] >= bar)
    out["by_changes"] = {
        str(k): agreement(scored.d_lineup[scored.changes == k], scored.d_result[scored.changes == k])["agreement"]
        for k in range(MIN_CHANGES, MAX_CHANGES + 1)
    }
    return out


def _concat(parts: Sequence[ScoredPairs]) -> ScoredPairs:
    def cat(name: str) -> np.ndarray:
        return np.concatenate([getattr(p, name) for p in parts]) if parts else np.empty(0)

    return ScoredPairs(cat("p_before_played"), cat("p_previous"), cat("p_after"), cat("d_result"), cat("changes"))


def decision_reason(format_code: str, decision: Dict[str, Any], served: bool) -> str:
    """The sentence the report and the log carry per format: what E5 said against which
    bar, and whether optimised selection is served -- the two must be read together."""
    if decision["agreement"] is None or decision["bar"] is None:
        verdict = "E5 could not be scored (no pairs whose result moved)"
    else:
        verdict = (
            f"E5 lineup-only agreement {decision['agreement']:.3f} over {decision['pairs_scored']} pairs against the "
            f"derived bar {decision['bar']:.3f} (an exactly-right objective would score "
            f"{decision['expected_if_exactly_right']:.3f}): {'passes' if decision['passes_derived_bar'] else 'fails'}"
        )
    policy = "served" if served else "not served"
    return f"optimised selection {policy} in {format_code}, because {verdict}"


def evaluate_format(
    format_code: str,
    pairs: Sequence[LineupPair],
    fold_objectives: Sequence[Tuple[pd.Timestamp, pd.Timestamp, Optional[Proba]]],
    locked_objective: Tuple[pd.Timestamp, Optional[Proba]],
    columns: List[str],
    served: bool,
) -> Dict[str, Any]:
    """E5 for one format: per fold, pooled over the folds (the decision), and the locked
    window beside them. ``served`` is the serving policy for the format, so the report can
    say whether policy and this run's verdict agree."""
    own = [p for p in pairs if p.format_code == format_code]
    folds: List[Dict[str, Any]] = []
    fold_scores: List[ScoredPairs] = []
    for cutoff, end, proba in fold_objectives:
        window = [p for p in own if cutoff <= p.after_date < end]
        entry: Dict[str, Any] = {
            "cutoff": cutoff.date().isoformat(),
            "end": end.date().isoformat(),
            "pairs": len(window),
        }
        if proba is None or not window:
            entry["skipped_reason"] = "no objective for this fold" if proba is None else "no pairs in the window"
            folds.append(entry)
            continue
        scored = score_pairs(window, proba, columns)
        fold_scores.append(scored)
        entry.update(agreement(scored.d_lineup, scored.d_result))
        entry["effect_size"] = effect_size(scored.d_lineup)
        folds.append(entry)
    pooled = _concat(fold_scores)
    development = _section(pooled)
    development["note"] = (
        "the decision: every fold's pairs, each scored by the objective fitted before its fold's cutoff"
    )
    locked_start, locked_proba = locked_objective
    locked_pairs = [p for p in own if p.after_date >= locked_start]
    if locked_proba is not None and locked_pairs:
        locked = _section(score_pairs(locked_pairs, locked_proba, columns))
    else:
        locked = {"pairs_scored": 0, "agreement": None, "skipped_reason": "no objective or no pairs"}
    locked["note"] = (
        f"locked window from {locked_start.date().isoformat()} (H-19): scored once per release, never used for a choice"
    )
    rates = [f.get("agreement") for f in folds if f.get("agreement") is not None]
    decision = {
        "agreement": development["agreement"],
        "pairs_scored": development["pairs_scored"],
        "standard_error": development["standard_error"],
        "bar": development["derived_bar"].get("bar"),
        "expected_if_exactly_right": development["derived_bar"].get("expected_if_exactly_right"),
        "passes_derived_bar": development["passes_derived_bar"],
        "optimised_selection_served": served,
    }
    decision["reason"] = decision_reason(format_code, decision, served)
    logger.info("%-5s %s", format_code, decision["reason"])
    return {
        "definition": DEFINITION,
        "why_not_as_played": WHY_NOT_AS_PLAYED,
        "changes_range": [MIN_CHANGES, MAX_CHANGES],
        "pairs": {
            "total": len(own),
            "unscored_previous_eleven": sum(1 for p in own if p.before_side is None),
            "development": int(sum(f["pairs"] for f in folds)),
            "locked": len(locked_pairs),
        },
        "walk_forward": {"folds": folds, "summary": {"agreement": summarise_over_folds(rates)}},
        "development": development,
        "locked": locked,
        "decision": decision,
    }
