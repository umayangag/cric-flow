"""E5 — the natural experiment for selection (plan §5, E5), for one format.

The two gates S-6 shipped on ask whether the objective *ranks* elevens (swap monotonicity)
and whether it prefers the specific eleven over a typical one. Neither varies one side while
holding the rest of the world still, which is why P-0's winner-accuracy gate could not settle
selection and its re-run could not either (§8.5): both arms optimised both sides, and an arm
that moves a fixture toward parity must lose winner accuracy however good its elevens are.

E5 varies one side. Take one club's consecutive matches in a format, keep the pairs where it
changed 1-3 players, and ask whether the objective moved in the same direction the result did.

    Δobjective = P(club wins | XI, opponent XI)_{k+1} − P(club wins | XI, opponent XI)_k
    Δresult    = won_{k+1} − won_k                                     (scored on ±1 only)

Both are read as played, which is the plan's spec and the only version whose Δresult is
observed. Its confound is that the opponent changes between the two matches — so the run also
reports a **lineup-only** decomposition, both elevens scored against match k+1's opponent at
match k+1's as-of, which isolates the change the club actually made. That one has no clean
Δresult of its own, so it is context, not the decision.

Every call carries `as_of` = the match date, so no pair is scored by a rating state that
contains its own result. The decision runs on matches **before** the locked window; the locked
window is scored separately and labelled, never used for the choice (H-19).

Decision rule: agreement > 55 % on ≥ 300 pairs means the objective is selecting on real
signal. Record either way.

Usage: python scripts/experiments/xi/e5_natural_experiment.py --format T20
"""

from __future__ import annotations

import argparse
import json
import math
import statistics
import sys
from collections import defaultdict
from dataclasses import dataclass
from typing import Dict, List, Optional, Sequence, Tuple

import psycopg2
import requests

ML_BASE = "http://localhost:8000"
LOCKED_FROM = "2025-09-01"
XI_SIZE = 11
MIN_CHANGES = 1
MAX_CHANGES = 3

# One row per (match, club): who it fielded, who it played, and whether it won. `club`
# follows canonical_id so a rename does not split a side's history in two (I-4).
SIDES_SQL = """
SELECT m.match_id,
       m.match_date,
       m.venue_id,
       COALESCE(o.canonical_id, o.id) AS club,
       mp.opposition_id AS side_id,
       COALESCE(winner.canonical_id, winner.id) AS winner_club,
       array_agg(p.external_id ORDER BY p.external_id) AS xi
FROM match m
JOIN match_format mf ON mf.id = m.format_id
JOIN match_player mp ON mp.match_id = m.match_id
JOIN opposition o ON o.id = mp.opposition_id
JOIN player p ON p.id = mp.player_id
LEFT JOIN opposition winner ON winner.id = m.outcome_winner_opposition_id
WHERE mf.code = %(format_code)s
  AND m.outcome_winner_opposition_id IS NOT NULL
  AND p.external_id IS NOT NULL
GROUP BY m.match_id, m.match_date, m.venue_id, club, mp.opposition_id, winner_club
"""


@dataclass(frozen=True)
class Side:
    match_id: int
    match_date: str
    venue_id: Optional[int]
    club: int
    side_id: int
    xi: Tuple[str, ...]
    won: bool


@dataclass
class Pair:
    club: int
    before: Side
    after: Side
    before_opponent: Side
    after_opponent: Side
    changes: int
    d_objective: Optional[float] = None
    d_objective_lineup: Optional[float] = None

    @property
    def d_result(self) -> int:
        return int(self.after.won) - int(self.before.won)


class ObjectiveModel:
    """`/xi/predict-win`'s objective probability, which is the number selection maximises.

    The objective's features are all differences between the two elevens, so orientation is
    the only thing team order decides; the club is always team1 here."""

    def __init__(self, base: str = ML_BASE) -> None:
        self.session = requests.Session()
        self.base = base
        self.failures = 0

    def probability(self, side: Side, opponent_xi: Sequence[str], as_of: str, opponent_id: int) -> Optional[float]:
        body = {
            "format": self.format_code,
            "team1_player_ids": list(side.xi),
            "team2_player_ids": list(opponent_xi),
            "team1_id": side.side_id,
            "team2_id": opponent_id,
            "as_of": as_of,
        }
        if side.venue_id is not None:
            body["venue_id"] = side.venue_id
        response = self.session.post(f"{self.base}/xi/predict-win", json=body, timeout=900)
        if response.status_code != 200:
            self.failures += 1
            return None
        return response.json()["objective_probability"]


def load_sides(conn, format_code: str) -> Tuple[Dict[int, List[Side]], Dict[int, List[Side]]]:
    """Every club's matches in date order, one entry per club per match."""
    with conn.cursor() as cur:
        cur.execute(SIDES_SQL, {"format_code": format_code})
        rows = cur.fetchall()
    by_match: Dict[int, List[Side]] = defaultdict(list)
    for match_id, match_date, venue_id, club, side_id, winner_club, xi in rows:
        by_match[match_id].append(
            Side(
                match_id=match_id,
                match_date=match_date.isoformat(),
                venue_id=venue_id,
                club=club,
                side_id=side_id,
                xi=tuple(xi),
                won=(winner_club == club),
            )
        )
    by_club: Dict[int, List[Side]] = defaultdict(list)
    for sides in by_match.values():
        if len(sides) != 2:
            continue  # a match whose two sides did not both resolve is not a fixture
        for side in sides:
            by_club[side.club].append(side)
    for sides in by_club.values():
        sides.sort(key=lambda s: (s.match_date, s.match_id))
    return by_club, by_match


def build_pairs(by_club: Dict[int, List[Side]], by_match: Dict[int, List[Side]]) -> List[Pair]:
    """Consecutive matches of one club that differ by MIN_CHANGES..MAX_CHANGES players."""
    pairs: List[Pair] = []
    for club, sides in by_club.items():
        for before, after in zip(sides, sides[1:]):
            if len(before.xi) != XI_SIZE or len(after.xi) != XI_SIZE:
                continue
            changes = len(set(after.xi) - set(before.xi))
            if not MIN_CHANGES <= changes <= MAX_CHANGES:
                continue
            opponents = {m.match_id: next(s for s in by_match[m.match_id] if s.club != club) for m in (before, after)}
            if len(opponents[after.match_id].xi) != XI_SIZE:
                continue
            pairs.append(
                Pair(
                    club=club,
                    before=before,
                    after=after,
                    before_opponent=opponents[before.match_id],
                    after_opponent=opponents[after.match_id],
                    changes=changes,
                )
            )
    return pairs


def agreement(pairs: Sequence[Pair], attribute: str) -> Dict[str, Optional[float]]:
    """Sign agreement between a Δobjective and Δresult, over the pairs where the result moved.

    A pair whose result did not move carries no direction to agree with, and a Δobjective of
    exactly zero expresses no preference; both are excluded and counted."""
    scored = 0
    agreed = 0
    no_result_change = 0
    no_preference = 0
    for pair in pairs:
        delta = getattr(pair, attribute)
        if delta is None:
            continue
        if pair.d_result == 0:
            no_result_change += 1
            continue
        if delta == 0:
            no_preference += 1
            continue
        scored += 1
        agreed += int((delta > 0) == (pair.d_result > 0))
    rate = (agreed / scored) if scored else None
    return {
        "pairs_scored": scored,
        "agreed": agreed,
        "agreement": rate,
        "standard_error": math.sqrt(rate * (1 - rate) / scored) if rate is not None and scored else None,
        "excluded_result_unchanged": no_result_change,
        "excluded_objective_indifferent": no_preference,
    }


def effect_size(pairs: Sequence[Pair], attribute: str) -> Dict[str, Optional[float]]:
    """How big a move the objective itself claims. A gate cannot resolve an effect the
    objective does not claim to produce, so this bounds what E5 could ever have seen."""
    values = [abs(getattr(p, attribute)) for p in pairs if getattr(p, attribute) is not None]
    if not values:
        return {"n": 0, "median_abs": None, "p90_abs": None}
    values.sort()
    return {
        "n": len(values),
        "median_abs": statistics.median(values),
        "p90_abs": values[int(0.9 * (len(values) - 1))],
    }


def evaluate(model: ObjectiveModel, pairs: Sequence[Pair]) -> None:
    """Score every pair, walking as-of dates forward so the serving state advances once.

    Each match's own probability is shared by the two pairs it belongs to, so it is computed
    once; the lineup-only probability is per pair because it is the *previous* eleven scored
    in this fixture."""
    as_played: Dict[Tuple[int, int], Optional[float]] = {}
    tasks: List[Tuple[str, Pair, str]] = []
    for pair in pairs:
        tasks.append((pair.before.match_date, pair, "before"))
        tasks.append((pair.after.match_date, pair, "after"))
        tasks.append((pair.after.match_date, pair, "lineup"))
    tasks.sort(key=lambda t: t[0])

    for index, (as_of, pair, kind) in enumerate(tasks, 1):
        if kind == "lineup":
            # The previous eleven, in this fixture, at this as-of: what the club gave up.
            previous = model.probability(
                Side(**{**pair.after.__dict__, "xi": pair.before.xi}),
                pair.after_opponent.xi,
                as_of,
                pair.after_opponent.side_id,
            )
            current = as_played.get((pair.after.match_id, pair.after.club))
            if previous is not None and current is not None:
                pair.d_objective_lineup = current - previous
            continue
        side = pair.before if kind == "before" else pair.after
        opponent = pair.before_opponent if kind == "before" else pair.after_opponent
        key = (side.match_id, side.club)
        if key not in as_played:
            as_played[key] = model.probability(side, opponent.xi, as_of, opponent.side_id)
        if kind == "after":
            before_p = as_played.get((pair.before.match_id, pair.before.club))
            after_p = as_played[key]
            if before_p is not None and after_p is not None:
                pair.d_objective = after_p - before_p
        if index % 500 == 0:
            print(f"  {index}/{len(tasks)} calls", flush=True)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--format", dest="format_code", default="T20")
    parser.add_argument("--out", default="output/e5_natural_experiment.json")
    parser.add_argument("--dsn", default="postgresql://postgres:postgres@localhost:5432/cricket_data")
    args = parser.parse_args()

    conn = psycopg2.connect(args.dsn)
    try:
        by_club, by_match = load_sides(conn, args.format_code)
    finally:
        conn.close()

    pairs = build_pairs(by_club, by_match)
    development = [p for p in pairs if p.after.match_date < LOCKED_FROM]
    locked = [p for p in pairs if p.after.match_date >= LOCKED_FROM]
    print(
        f"{args.format_code}: {len(pairs)} pairs with {MIN_CHANGES}-{MAX_CHANGES} changes "
        f"({len(development)} development, {len(locked)} locked window)",
        flush=True,
    )

    model = ObjectiveModel()
    model.format_code = args.format_code
    evaluate(model, development + locked)

    def section(group: Sequence[Pair]) -> Dict:
        # Pairs whose two matches were against the same club: the opponent barely moves, so
        # Δresult reads the lineup change rather than the fixture change. This is the version
        # of E5 with the confound taken out, and the one the decision should rest on.
        same_opponent = [p for p in group if p.before_opponent.club == p.after_opponent.club]
        return {
            "pairs": len(group),
            "as_played": agreement(group, "d_objective"),
            "lineup_only": agreement(group, "d_objective_lineup"),
            "same_opponent": {
                "pairs": len(same_opponent),
                "as_played": agreement(same_opponent, "d_objective"),
                "lineup_only": agreement(same_opponent, "d_objective_lineup"),
            },
            "effect_size": {
                "as_played": effect_size(group, "d_objective"),
                "lineup_only": effect_size(group, "d_objective_lineup"),
            },
            "by_changes": {
                str(n): agreement([p for p in group if p.changes == n], "d_objective")
                for n in range(MIN_CHANGES, MAX_CHANGES + 1)
            },
        }

    report = {
        "format": args.format_code,
        "changes_range": [MIN_CHANGES, MAX_CHANGES],
        "decision_rule": "agreement > 0.55 on >= 300 pairs",
        "call_failures": model.failures,
        "development": section(development),
        "locked_window": section(locked),
    }
    with open(args.out, "w") as fh:
        json.dump(report, fh, indent=2)
    print(json.dumps({k: report[k] for k in ("development", "locked_window")}, indent=2))
    print(f"\nwritten to {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
