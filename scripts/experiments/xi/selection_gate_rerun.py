"""Re-run of P-0's selection gate over the locked window, with the D-7a id contract fixed.

P-0 reported `xi` 0.560 versus `greedy` 0.569 winner accuracy over 332 locked-window
matches and concluded that win-probability selection did not beat greedy. That run went
through the broken id contract (D-7a, plan §10.6): go-app sent its numeric ``player.id``
where ml-service keys everything by the Cricsheet registry id, so every player the
optimiser scored and every player the win model read was an unrated debutant.

**What this script can and cannot reproduce.** The `greedy` arm is gone: it scored players
on the per-player batting / bowling / fielding models P-5 deleted, so there is nothing left
to run it with. What is recoverable is the *xi* arm, and — by deliberately sending the ids
D-7a sent — the exact serving condition that produced P-0's number. So the arms here are:

  winprob   both XIs by alternating best response on the win objective, registry ids
  ratings   both XIs rating-ordered, evaluating no model — the surviving unoptimised
            baseline, standing in for `greedy`; it differs from the pool and the
            constraints in nothing but whether the objective is consulted
  d7a       `winprob` with the numeric player id in place of the registry id: P-0's
            condition, reproduced rather than argued about
  fielded   the XIs that actually took the field, as the reference the other three are
            selections against

Every arm's winner comes from the same display win model on the two XIs it chose, which is
how P-0 scored its arms. Winner accuracy is the only metric here grounded in what happened;
mean P(win) says an arm moved its own objective, not that it moved somewhere true; and
divergence says whether the search is doing anything at all.

Usage: python scripts/experiments/xi/selection_gate_rerun.py [--limit N] [--out FILE]
"""

from __future__ import annotations

import argparse
import json
import random
import sys
from calendar import monthrange
from collections import defaultdict
from datetime import date
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Sequence, Tuple

import psycopg2
import requests

ML_BASE = "http://localhost:8000"
LOCKED_FROM = "2025-09-01"
LOCKED_TO = "2026-08-25"
FORMATS = ("T20", "T20I", "ODI")
TEAM_SIZE = 11
# P-0's gate handler's own defaults: min_bowlers from config.DefaultMinBowlers, and
# require_keeper false because selectionComparisonRequest carried a bare bool nobody set.
MIN_BOWLERS = 5
REQUIRE_KEEPER = False
BEST_RESPONSE_ROUNDS = 3

MATCHES_SQL = """
SELECT m.match_id, mf.code, m.match_date, m.venue_id,
       first_inn.batting_team_opposition_id AS team1_id,
       first_inn.bowling_team_opposition_id AS team2_id,
       m.outcome_winner_opposition_id
FROM match m
JOIN match_format mf ON mf.id = m.format_id
JOIN LATERAL (
    SELECT batting_team_opposition_id, bowling_team_opposition_id
    FROM match_inning WHERE match_id = m.match_id ORDER BY inning_number LIMIT 1
) first_inn ON true
WHERE mf.code = ANY(%(formats)s)
  AND m.match_date >= %(locked_from)s AND m.match_date <= %(locked_to)s
  AND m.outcome_winner_opposition_id IS NOT NULL
  AND first_inn.batting_team_opposition_id IS NOT NULL
  AND first_inn.bowling_team_opposition_id IS NOT NULL
ORDER BY m.match_date, m.match_id
"""

# The pool go-app would have offered on that date, transcribed from
# db.ListPlayerPoolByOpposition: everyone who has batted or bowled for the club in this
# format strictly before the match, and — since D-12 — no earlier than the recency window
# ending at that date. `club` follows canonical_id so a rename does not halve the pool (I-4).
#
# The window is relative to the fixture's own date, never to today, so a 2019 fixture sees
# the players of 2019. The retirement ledger is *not* applied and `is_retired` is not read:
# the ledger holds claims made now, and a claim made now is not evidence about who was
# available then (H-19). That is also why this rerun's numbers move: the pre-D-12 rerun
# offered every player who had ever appeared for the club, and comparing a windowed run
# against that one compares two different questions.
POOL_SQL = """
WITH club AS (
    SELECT id FROM opposition WHERE COALESCE(canonical_id, id) = %(team_id)s
), eligible AS (
    SELECT bd.player_id AS id
    FROM batting_data bd
    JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
    JOIN match m ON m.match_id = bd.match_id
    WHERE m.format_id = %(format_id)s AND m.match_date < %(cutoff)s
      AND (%(since)s::date IS NULL OR m.match_date >= %(since)s)
      AND mi.batting_team_opposition_id IN (SELECT id FROM club)
    UNION
    SELECT bw.player_id
    FROM bowling_data bw
    JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
    JOIN match m ON m.match_id = bw.match_id
    WHERE m.format_id = %(format_id)s AND m.match_date < %(cutoff)s
      AND (%(since)s::date IS NULL OR m.match_date >= %(since)s)
      AND mi.bowling_team_opposition_id IN (SELECT id FROM club)
)
SELECT p.id, COALESCE(p.external_id, ''), p.is_wicket_keeper
FROM player p JOIN eligible e ON e.id = p.id
ORDER BY p.player_name, p.id
"""

# The per-format recency window go-app defaults to, in months (D-12). Kept in step with
# go-app/config.json `pool.recency_months` and internal/config's DefaultPoolRecencyMonths;
# `--pool-window-months` overrides it and `--all-time-pool` restores the pre-D-12 pool for
# a like-for-like comparison against the recorded run.
POOL_RECENCY_MONTHS = {"TEST": 12, "ODI": 12, "T20": 12, "T20I": 9}


def window_start(cutoff: str, months: int) -> Optional[str]:
    """The first match date a recency-bounded pool accepts, as go-app computes it.

    Month arithmetic is clamped, not normalised: 31 March less one month is 28 February,
    because a window is a span of months and a user reading "the last 12 months" means the
    same day of the month, or that month's last day where there is no such day.
    """
    if months <= 0:
        return None
    cut = date.fromisoformat(cutoff)
    year, month = cut.year, cut.month - months
    while month <= 0:
        month += 12
        year -= 1
    day = min(cut.day, monthrange(year, month)[1])
    return date(year, month, day).isoformat()


FIELDED_SQL = """
SELECT mp.opposition_id, COALESCE(p.external_id, '')
FROM match_player mp
JOIN player p ON p.id = mp.player_id
WHERE mp.match_id = %(match_id)s
"""


@dataclass
class Fixture:
    match_id: int
    format_code: str
    match_date: str
    venue_id: Optional[int]
    team1_id: int
    team2_id: int
    winner_id: int
    pool1: List[str] = field(default_factory=list)
    pool2: List[str] = field(default_factory=list)
    numeric1: List[str] = field(default_factory=list)
    numeric2: List[str] = field(default_factory=list)
    fielded1: List[str] = field(default_factory=list)
    fielded2: List[str] = field(default_factory=list)


class MlService:
    """The two XI endpoints, with as_of on every call so a played match is never scored
    with a rating state that already contains its result."""

    def __init__(self, base: str = ML_BASE) -> None:
        self.session = requests.Session()
        self.base = base
        self.last_error = ""

    def optimize(
        self, fx: Fixture, objective: str, pool: Sequence[str], opponent: Sequence[str], team_is_team1: bool
    ) -> Optional[List[str]]:
        body = {
            "format": fx.format_code,
            "pool_player_ids": list(pool),
            "opponent_player_ids": list(opponent),
            "team_is_team1": team_is_team1,
            "objective": objective,
            "as_of": fx.match_date,
            "constraints": {
                "team_size": TEAM_SIZE,
                "min_bowlers": MIN_BOWLERS,
                "require_keeper": REQUIRE_KEEPER,
            },
        }
        r = self.session.post(f"{self.base}/xi/optimize", json=body, timeout=900)
        if r.status_code != 200:
            self.last_error = f"{r.status_code} {r.text[:200]}"
            return None
        return r.json()["selected_player_ids"]

    def predict_win(self, fx: Fixture, xi1: Sequence[str], xi2: Sequence[str]) -> Optional[float]:
        body = {
            "format": fx.format_code,
            "team1_player_ids": list(xi1),
            "team2_player_ids": list(xi2),
            "team1_id": fx.team1_id,
            "team2_id": fx.team2_id,
            "as_of": fx.match_date,
        }
        if fx.venue_id is not None:
            body["venue_id"] = fx.venue_id
        r = self.session.post(f"{self.base}/xi/predict-win", json=body, timeout=900)
        if r.status_code != 200:
            self.last_error = f"{r.status_code} {r.text[:200]}"
            return None
        return r.json()["team1_win_probability"]


def select_both_xis(
    ml: MlService, fx: Fixture, pool1: Sequence[str], pool2: Sequence[str], objective: str
) -> Optional[Tuple[List[str], List[str]]]:
    """go-app's alternating best response: seed both sides by rating order, then optimise
    each side against the other side's current XI until the pair stops moving."""
    xi1 = ml.optimize(fx, "ratings", pool1, [], True)
    xi2 = ml.optimize(fx, "ratings", pool2, [], False)
    if xi1 is None or xi2 is None:
        return None
    if objective == "ratings":
        return xi1, xi2
    for _ in range(BEST_RESPONSE_ROUNDS):
        next1 = ml.optimize(fx, "win", pool1, xi2, True)
        next2 = ml.optimize(fx, "win", pool2, next1 or xi1, False)
        if next1 is None or next2 is None:
            return None
        settled = set(next1) == set(xi1) and set(next2) == set(xi2)
        xi1, xi2 = next1, next2
        if settled:
            break
    return xi1, xi2


@dataclass
class ArmTally:
    decided: int = 0
    correct: int = 0
    failed: int = 0
    prob_sum: float = 0.0
    # Why an arm could not select. Kept because an arm that never ran is a finding, not
    # a gap: it is what go-app would have fallen back from.
    last_error: str = ""
    xis: Dict[int, Tuple[List[str], List[str]]] = field(default_factory=dict)

    def record(self, fx: Fixture, p: Optional[float], xi1: List[str], xi2: List[str]) -> None:
        if p is None:
            self.failed += 1
            return
        self.decided += 1
        self.prob_sum += p
        predicted = fx.team1_id if p >= 0.5 else fx.team2_id
        self.correct += int(predicted == fx.winner_id)
        self.xis[fx.match_id] = (xi1, xi2)

    def report(self) -> Dict[str, Optional[float]]:
        return {
            "matches": self.decided,
            "failed": self.failed,
            "winner_accuracy": (self.correct / self.decided) if self.decided else None,
            "mean_team1_win_probability": (self.prob_sum / self.decided) if self.decided else None,
            "last_error": self.last_error or None,
        }


def divergence(a: ArmTally, b: ArmTally) -> Optional[float]:
    """Mean players per match that two arms picked differently, over both sides."""
    shared = set(a.xis) & set(b.xis)
    if not shared:
        return None
    total = 0
    for match_id in shared:
        (a1, a2), (b1, b2) = a.xis[match_id], b.xis[match_id]
        total += len(set(a1) ^ set(b1)) // 2 + len(set(a2) ^ set(b2)) // 2
    return total / len(shared)


def load_fixtures(
    conn,
    limit: Optional[int],
    per_format: Optional[int],
    seed: int,
    pool_window_months: Optional[int],
    all_time_pool: bool,
) -> List[Fixture]:
    with conn.cursor() as cur:
        cur.execute(
            MATCHES_SQL,
            {"formats": list(FORMATS), "locked_from": LOCKED_FROM, "locked_to": LOCKED_TO},
        )
        rows = cur.fetchall()
        cur.execute("SELECT code, id FROM match_format")
        format_ids = dict(cur.fetchall())

    fixtures: List[Fixture] = []
    for match_id, format_name, match_date, venue_id, team1_id, team2_id, winner_id in rows:
        fixtures.append(
            Fixture(
                match_id=match_id,
                format_code=format_name,
                match_date=match_date.isoformat(),
                venue_id=venue_id,
                team1_id=team1_id,
                team2_id=team2_id,
                winner_id=winner_id,
            )
        )
    if per_format:
        # A seeded sample per format, so each format's accuracy has a usable denominator
        # and the run is not swamped by the 1,635 domestic T20s. Re-sorted by date: the
        # as-of server serves a run that walks forward with one pass and rebuilds from
        # scratch for one that walks back.
        rng = random.Random(seed)
        by_format: Dict[str, List[Fixture]] = defaultdict(list)
        for fx in fixtures:
            by_format[fx.format_code].append(fx)
        sampled: List[Fixture] = []
        for fmt in sorted(by_format):
            group = by_format[fmt]
            sampled.extend(group if len(group) <= per_format else rng.sample(group, per_format))
        fixtures = sorted(sampled, key=lambda f: (f.match_date, f.match_id))
    if limit:
        fixtures = fixtures[-limit:]

    with conn.cursor() as cur:
        for fx in fixtures:
            format_id = format_ids[fx.format_code]
            for side, team_id in ((1, fx.team1_id), (2, fx.team2_id)):
                months = (
                    0
                    if all_time_pool
                    else (pool_window_months or POOL_RECENCY_MONTHS.get(fx.format_code, 12))
                )
                cur.execute(
                    POOL_SQL,
                    {
                        "team_id": team_id,
                        "format_id": format_id,
                        "cutoff": fx.match_date,
                        "since": window_start(fx.match_date, months),
                    },
                )
                keys, numeric = [], []
                for player_id, external_id, _ in cur.fetchall():
                    if not external_id:
                        continue
                    keys.append(external_id)
                    numeric.append(str(player_id))
                if side == 1:
                    fx.pool1, fx.numeric1 = keys, numeric
                else:
                    fx.pool2, fx.numeric2 = keys, numeric
            cur.execute(FIELDED_SQL, {"match_id": fx.match_id})
            for opposition_id, external_id in cur.fetchall():
                if not external_id:
                    continue
                if opposition_id == fx.team1_id:
                    fx.fielded1.append(external_id)
                else:
                    fx.fielded2.append(external_id)
    return fixtures


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--limit", type=int, default=0, help="most recent N matches only")
    parser.add_argument("--per-format", type=int, default=200, help="seeded sample size per format")
    parser.add_argument("--seed", type=int, default=20260902)
    parser.add_argument("--out", default="output/selection_gate_rerun.json")
    parser.add_argument("--dsn", default="postgresql://postgres:postgres@localhost:5432/cricket_data")
    parser.add_argument(
        "--pool-window-months",
        type=int,
        default=0,
        help="recency window for the pool, relative to each fixture's date (0 = the per-format default)",
    )
    parser.add_argument(
        "--all-time-pool",
        action="store_true",
        help="the pre-D-12 unbounded pool, for a like-for-like comparison with the recorded run",
    )
    args = parser.parse_args()

    conn = psycopg2.connect(args.dsn)
    try:
        fixtures = load_fixtures(
            conn,
            args.limit or None,
            args.per_format or None,
            args.seed,
            args.pool_window_months or None,
            args.all_time_pool,
        )
    finally:
        conn.close()
    print(f"{len(fixtures)} decided locked-window matches ({LOCKED_FROM} .. {LOCKED_TO})", flush=True)

    ml = MlService()
    arms = {name: ArmTally() for name in ("winprob", "ratings", "d7a", "fielded")}
    per_format: Dict[str, Dict[str, ArmTally]] = defaultdict(lambda: {n: ArmTally() for n in arms})
    skipped = 0

    for i, fx in enumerate(fixtures, 1):
        if len(fx.pool1) < TEAM_SIZE or len(fx.pool2) < TEAM_SIZE:
            skipped += 1
            continue
        for name, pool1, pool2, objective in (
            ("winprob", fx.pool1, fx.pool2, "win"),
            ("ratings", fx.pool1, fx.pool2, "ratings"),
            ("d7a", fx.numeric1, fx.numeric2, "win"),
        ):
            chosen = select_both_xis(ml, fx, pool1, pool2, objective)
            if chosen is None:
                arms[name].failed += 1
                arms[name].last_error = ml.last_error
                per_format[fx.format_code][name].failed += 1
                per_format[fx.format_code][name].last_error = ml.last_error
                continue
            xi1, xi2 = chosen
            p = ml.predict_win(fx, xi1, xi2)
            arms[name].record(fx, p, xi1, xi2)
            per_format[fx.format_code][name].record(fx, p, xi1, xi2)
        if len(fx.fielded1) >= TEAM_SIZE and len(fx.fielded2) >= TEAM_SIZE:
            p = ml.predict_win(fx, fx.fielded1, fx.fielded2)
            arms["fielded"].record(fx, p, fx.fielded1, fx.fielded2)
            per_format[fx.format_code]["fielded"].record(fx, p, fx.fielded1, fx.fielded2)
        if i % 25 == 0:
            print(f"  {i}/{len(fixtures)}", flush=True)

    report = {
        "locked_window": [LOCKED_FROM, LOCKED_TO],
        "formats": list(FORMATS),
        "matches_offered": len(fixtures),
        "sample_per_format": args.per_format,
        "seed": args.seed,
        "matches_skipped_small_pool": skipped,
        "constraints": {"team_size": TEAM_SIZE, "min_bowlers": MIN_BOWLERS, "require_keeper": REQUIRE_KEEPER},
        "arms": {name: tally.report() for name, tally in arms.items()},
        "divergence_players_per_match": {
            "winprob_vs_ratings": divergence(arms["winprob"], arms["ratings"]),
            "winprob_vs_d7a": divergence(arms["winprob"], arms["d7a"]),
            "winprob_vs_fielded": divergence(arms["winprob"], arms["fielded"]),
        },
        "per_format": {
            fmt: {name: tally.report() for name, tally in by_arm.items()}
            for fmt, by_arm in sorted(per_format.items())
        },
    }
    with open(args.out, "w") as fh:
        json.dump(report, fh, indent=2)
    print(json.dumps(report["arms"], indent=2))
    print(f"\nwritten to {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
