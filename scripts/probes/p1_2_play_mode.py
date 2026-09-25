"""P1-2's two measurements on the running stack: swap monotonicity and re-score latency.

Both are about what a *user* sees, so both go through the product's own path — the client's
call to go-app, go-app's calls to ml-service, and the answer back — and never through a model
in-process. Nothing here fits, trains or re-measures a model: B-7 measured the display model's
swap-violation share and the served run reads 0.0000 (docs/BUG_BACKLOG.md § B-7). What this
asks is whether the *surface* inherits that, which is a plumbing question.

Three things are measured, in this order.

**Path parity.** Pinning the eleven Optimise just returned must reproduce Optimise's own
numbers exactly. It is the one-code-path claim as an assertion: if a re-score moved a number
without the eleven moving, everything below it is measuring two code paths.

**Swap monotonicity (the P1-2 gate).** An *upgrade* is a swap for a player who is at least as
good on every as-of vector the store holds for him (``contract.PLAYER_VECTOR_KEYS``: both
expected-ball counts, the four rate ratings, both career counts, player Elo and the keeper
flag) and strictly better on one. That is the surface's translation of H-4's upgrade, which
raises one player's five ratings and holds everything else; a swap cannot hold everything
else, so it must dominate instead. The probe builds a degraded eleven by putting a dominated
pool player into the served eleven, then swaps him back out and asserts the displayed
probability does not fall.

**Selector-rank swaps (a diagnostic, and not the gate).** The same test where "better" means
only that the stack's own rating order prefers the incoming player — the order
``select_xi_by_ratings`` uses, which is one composite of impact and Elo. A player can rank
above another on that composite while being worse on some axis the display model reads, so a
fall here is not a broken guarantee. It is measured because the Lab invites exactly this swap
and a user will read the ranking as "better".

Run it against a stack serving the run under test:

    PYTHONPATH=ml-service python scripts/probes/p1_2_play_mode.py \\
        --api http://127.0.0.1:8081 --ml http://127.0.0.1:8001 --repeats 30

It reads the database (player ids to registry ids) and the served run's artifacts, and writes
nothing anywhere.
"""

from __future__ import annotations

import argparse
import json
import os
import statistics
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Sequence, Tuple

import numpy as np
import psycopg2

from ml.xi import contract as C
from ml.xi import runs
from ml.xi.optimizer import Constraints, select_xi_by_ratings
from ml.xi.store import XiStore

#: The four fixtures the probe runs, one per format. Two are optimised (T20I, ODI) and two are
#: rating-ordered (T20, TEST, plan §8.8), and the probe must hold in all four: the displayed
#: probability is the display model's in every format (E2), so the guarantee B-7 bought applies
#: to every surface whatever chose the eleven.
FIXTURES = [
    {"format": "T20I", "team1": "India", "team2": "Australia", "gender": "male"},
    {"format": "ODI", "team1": "England", "team2": "India", "gender": "male"},
    {"format": "T20", "team1": "Mumbai Indians", "team2": "Chennai Super Kings", "gender": "male"},
    {"format": "TEST", "team1": "Australia", "team2": "England", "gender": "male"},
]

MATCH_DATE = "2026-09-10"
#: How many swaps of each kind to try per format.
SWAPS_PER_FORMAT = 8
#: A probability that moves by less than this did not move: the wire carries float64 and the
#: simulator's draws are seeded, so anything above it is a real difference.
EPS = 1e-12


@dataclass
class Stack:
    """The two services, as a client reaches them."""

    api: str
    ml: str
    api_key: str

    def post_api(self, path: str, payload: dict) -> Tuple[dict, float]:
        return _post(f"{self.api}{path}", payload, {"X-API-Key": self.api_key})

    def get_api(self, path: str) -> dict:
        request = urllib.request.Request(f"{self.api}{path}", headers={"X-API-Key": self.api_key})
        with urllib.request.urlopen(request, timeout=300) as response:
            return json.load(response)

    def post_ml(self, path: str, payload: dict) -> Tuple[dict, float]:
        return _post(f"{self.ml}{path}", payload, {})


def _post(url: str, payload: dict, headers: Dict[str, str]) -> Tuple[dict, float]:
    """POST, and return the answer with the wall-clock seconds the round trip took."""
    body = json.dumps(payload).encode()
    request = urllib.request.Request(
        url, data=body, headers={"Content-Type": "application/json", **headers}, method="POST"
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=300) as response:
            answer = json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"{url} answered {error.code}: {error.read().decode()[:400]}") from error
    return answer, time.perf_counter() - started


def registry_ids(player_ids: Sequence[int]) -> Dict[int, str]:
    """The registry ids ml-service knows these players by (``player.external_id``).

    go-app deliberately keeps the registry id off the wire — a client joins on ``player_id`` —
    so a probe that also has to address ml-service reads the mapping from the database.
    Read-only, and the only database access this script makes.
    """
    dsn = {
        "host": os.environ.get("POSTGRES_HOST", "localhost"),
        "port": os.environ.get("POSTGRES_PORT", "5432"),
        "dbname": os.environ.get("POSTGRES_DB", "cricket_data"),
        "user": os.environ.get("POSTGRES_USER", "postgres"),
        "password": os.environ.get("POSTGRES_PASSWORD", "postgres"),
    }
    with psycopg2.connect(**dsn) as connection, connection.cursor() as cursor:
        cursor.execute(
            "SELECT id, external_id FROM player WHERE id = ANY(%s) AND external_id <> ''",
            (list(player_ids),),
        )
        return {row[0]: row[1] for row in cursor.fetchall()}


def predict(stack: Stack, fixture: dict, pinned: Optional[Tuple[List[int], List[int]]] = None) -> Tuple[dict, float]:
    """One prediction: Optimise, or — with ``pinned`` — one Play-mode re-score."""
    payload = {
        "format": fixture["format"],
        "team1": fixture["team1"],
        "team1_gender": fixture["gender"],
        "team2": fixture["team2"],
        "team2_gender": fixture["gender"],
        "match_date": MATCH_DATE,
    }
    if pinned is not None:
        payload["team1_xi"], payload["team2_xi"] = pinned
    return stack.post_api("/api/predict/team-selection", payload)


def candidates(stack: Stack, fmt: str, club_id: int, all_time: bool) -> Dict[int, str]:
    """The pool a prediction for this side would be chosen from, ledger exclusions left out.

    The default is the recency window a prediction actually uses; the all-time list is asked
    for only when the window offers too few players outside the eleven to find a swap, and the
    report says where that happened.
    """
    query = f"format={fmt}&club_id={club_id}&match_date={MATCH_DATE}"
    if all_time:
        query += "&all_time=true"
    answer = stack.get_api(f"/api/options/candidates?{query}")
    return {
        candidate["player_id"]: candidate["player_name"]
        for candidate in answer["candidates"]
        if not candidate["excluded"]
    }


def rating_order(store: XiStore, fmt: str, keys: Sequence[str]) -> List[str]:
    """The serving stack's own rating order over a pool, under no role constraints, so the
    ordering is rating and nothing else.

    It is the stack's own function rather than a copy of its arithmetic — the same
    ``select_xi_by_ratings`` that answers `objective="ratings"` on the wire, which is the order
    the Lab shows for T20 and TEST. It is called in-process only because the endpoint caps an
    eleven at fifteen players and this needs the whole pool ordered; nothing is measured here,
    only the pair to swap is chosen.
    """
    return select_xi_by_ratings(
        store, fmt, list(keys), Constraints(team_size=len(keys), min_bowlers=0, require_keeper=False)
    )


def dominates(vectors: Dict[str, np.ndarray], better: int, worse: int) -> bool:
    """Whether one player is at least as good as another on every as-of vector, and better on
    one.

    `keeper` is a flag rather than a rating, and it is included for the same reason the rest
    are: a swap that loses the side its keeper has changed something the model reads, so it is
    not an unambiguous upgrade and this probe does not claim it as one.
    """
    strictly_better = False
    for key in C.PLAYER_VECTOR_KEYS:
        gap = float(vectors[key][better] - vectors[key][worse])
        if gap < 0:
            return False
        if gap > 0:
            strictly_better = True
    return strictly_better


@dataclass
class SwapResult:
    """One swap on the surface: who came out, who went in, and what the probability did."""

    out_name: str
    in_name: str
    p_before: float
    p_after: float

    @property
    def change(self) -> float:
        return self.p_after - self.p_before

    @property
    def holds(self) -> bool:
        """Monotone means it does not *fall*; an unchanged probability is not a violation."""
        return self.change > -EPS


@dataclass
class FormatReport:
    fmt: str
    optimised: bool = False
    samples: Optional[int] = None
    win_source: str = ""
    parity_gap: Optional[float] = None
    dominating: List[SwapResult] = field(default_factory=list)
    ranked: List[SwapResult] = field(default_factory=list)
    latency_ms: List[float] = field(default_factory=list)
    predict_win_ms: List[float] = field(default_factory=list)
    simulate_ms: List[float] = field(default_factory=list)
    performance_ms: List[float] = field(default_factory=list)
    note: str = ""


def _swap(xi: List[int], out_id: int, in_id: int) -> List[int]:
    return [in_id if player == out_id else player for player in xi]


def probe_format(stack: Stack, store: XiStore, fixture: dict, repeats: int) -> FormatReport:
    """Run all three measurements for one fixture."""
    fmt = fixture["format"]
    report = FormatReport(fmt=fmt)

    searched, _ = predict(stack, fixture)
    report.optimised = searched["selection"]["optimised"]
    report.samples = (searched.get("scorecard") or {}).get("samples")
    report.win_source = searched["win_probability"]["source"]

    xi1 = [player["player_id"] for player in searched["team1"]]
    xi2 = [player["player_id"] for player in searched["team2"]]
    pool = candidates(stack, fmt, searched["team1_side"]["club_id"], all_time=False)

    # Pinning the eleven that was just searched for must reproduce its numbers exactly.
    pinned_base, _ = predict(stack, fixture, (xi1, xi2))
    report.parity_gap = pinned_base["win_probability"]["team1"] - searched["win_probability"]["team1"]
    base_probability = pinned_base["win_probability"]["team1"]

    names = {player["player_id"]: player["player_name"] for player in searched["team1"]}
    names.update(pool)
    upgrades = upgrade_pairs(store, fmt, xi1, pool, names)
    if not upgrades.pairs:
        # The recency window offered no unambiguous upgrade, so the search widens to the
        # all-time list. It changes which players may be picked, not how any is scored.
        pool = candidates(stack, fmt, searched["team1_side"]["club_id"], all_time=True)
        names.update(pool)
        upgrades = upgrade_pairs(store, fmt, xi1, pool, names)
        report.note = "no unambiguous upgrade inside the recency window; the all-time pool was searched"

    for pair in upgrades.pairs[:SWAPS_PER_FORMAT]:
        report.dominating.append(score_swap(stack, fixture, xi1, xi2, pair, base_probability))
    for pair in upgrades.ranked_only[:SWAPS_PER_FORMAT]:
        report.ranked.append(score_swap(stack, fixture, xi1, xi2, pair, base_probability))

    measure_latency(stack, fixture, xi1, xi2, repeats, report)
    return report


@dataclass
class SwapPair:
    """One swap to score: who leaves the eleven, who joins it, and their names."""

    out_id: int
    in_id: int
    out_name: str
    in_name: str
    #: True where the eleven as searched is already the upgraded side, so the *degraded*
    #: eleven is the one that has to be scored.
    degrade_first: bool


@dataclass
class UpgradePairs:
    pairs: List[SwapPair] = field(default_factory=list)
    ranked_only: List[SwapPair] = field(default_factory=list)


def upgrade_pairs(store: XiStore, fmt: str, xi: List[int], pool: Dict[int, str], names: Dict[int, str]) -> UpgradePairs:
    """Every swap worth scoring, in both directions.

    A dominating pair is an upgrade in the sense H-4's probe uses. It is looked for both ways
    round — a pool player who dominates someone in the eleven, and someone in the eleven who
    dominates a pool player, whose swap back out is the same upgrade — because which way round
    a pool happens to offer one is an accident of the fixture, not of the property.
    """
    ids = sorted(set(pool) | set(xi))
    keys = registry_ids(ids)
    ordered_ids = [pid for pid in ids if pid in keys]
    position = {pid: index for index, pid in enumerate(ordered_ids)}
    vectors = store.side_vectors(fmt, [keys[pid] for pid in ordered_ids])
    outside = [pid for pid in ordered_ids if pid not in set(xi)]
    ranked = rating_order(store, fmt, [keys[pid] for pid in ordered_ids])
    rank = {key: index for index, key in enumerate(ranked)}

    found = UpgradePairs()
    for member in xi:
        if member not in position:
            continue
        rank_only_for_member: Optional[SwapPair] = None
        for other in outside:
            outsider, insider = position[other], position[member]
            if dominates(vectors, outsider, insider):
                found.pairs.append(SwapPair(member, other, names[member], names[other], degrade_first=False))
            elif dominates(vectors, insider, outsider):
                found.pairs.append(SwapPair(other, member, names[other], names[member], degrade_first=True))
            elif rank_only_for_member is None and rank.get(keys[member], len(rank)) < rank.get(keys[other], len(rank)):
                # Ranked above him, but not better on every axis: the diagnostic's case.
                rank_only_for_member = SwapPair(other, member, names[other], names[member], degrade_first=True)
        # One rank-only swap per player in the eleven, so the diagnostic spreads over the side
        # rather than piling onto whichever player the pool happens to offer most partners for.
        if rank_only_for_member is not None:
            found.ranked_only.append(rank_only_for_member)
    return found


def score_swap(
    stack: Stack,
    fixture: dict,
    xi1: List[int],
    xi2: List[int],
    pair: SwapPair,
    base_probability: float,
) -> SwapResult:
    """Score one swap: the eleven without the upgrade, against the eleven with it.

    Where the searched eleven already holds the better player, the *other* eleven is the one
    that has to be scored and the searched eleven's own probability is the "after"; where it
    does not, the searched eleven is the "before". Either way one prediction is made and both
    numbers come off the same pinned path.
    """
    if pair.degrade_first:
        degraded, _ = predict(stack, fixture, (_swap(xi1, pair.in_id, pair.out_id), xi2))
        return SwapResult(
            out_name=pair.out_name,
            in_name=pair.in_name,
            p_before=degraded["win_probability"]["team1"],
            p_after=base_probability,
        )
    upgraded, _ = predict(stack, fixture, (_swap(xi1, pair.out_id, pair.in_id), xi2))
    return SwapResult(
        out_name=pair.out_name,
        in_name=pair.in_name,
        p_before=base_probability,
        p_after=upgraded["win_probability"]["team1"],
    )


def measure_latency(
    stack: Stack,
    fixture: dict,
    xi1: List[int],
    xi2: List[int],
    repeats: int,
    report: FormatReport,
) -> None:
    """Time one Play-mode re-score end to end, and the two ml-service calls it makes.

    The re-score is the whole round trip a user waits for: go-app resolves the fixture and both
    candidate pools out of Postgres, then calls ``/xi/predict-win`` and — for a format with an
    innings length — ``/simulate`` at its served draw count. Those two are timed separately
    against ml-service so the total can be attributed rather than guessed at.
    """
    fmt = fixture["format"]
    base, _ = predict(stack, fixture, (xi1, xi2))
    all_keys = registry_ids(xi1 + xi2)
    context = {
        "format": fmt,
        "team1_player_ids": [all_keys[pid] for pid in xi1 if pid in all_keys],
        "team2_player_ids": [all_keys[pid] for pid in xi2 if pid in all_keys],
        "team1_id": base["team1_side"]["club_id"],
        "team2_id": base["team2_side"]["club_id"],
    }
    for _ in range(repeats):
        _, elapsed = predict(stack, fixture, (xi1, xi2))
        report.latency_ms.append(elapsed * 1000)
        _, win_seconds = stack.post_ml("/xi/predict-win", context)
        report.predict_win_ms.append(win_seconds * 1000)
        if base.get("scorecard"):
            _, sim_seconds = stack.post_ml("/simulate", {**context, "seed": 7})
            report.simulate_ms.append(sim_seconds * 1000)
        else:
            # No innings length, so the forecast is L2-B's own quantiles rather than draws.
            _, performance_seconds = stack.post_ml("/performance/predict", context)
            report.performance_ms.append(performance_seconds * 1000)


def quantile(values: Sequence[float], q: float) -> float:
    ordered = sorted(values)
    if not ordered:
        return float("nan")
    index = min(len(ordered) - 1, max(0, int(round(q * (len(ordered) - 1)))))
    return ordered[index]


def _swap_lines(title: str, reports: Sequence[FormatReport], attribute: str) -> List[str]:
    lines = ["", title, "", f"{'format':6} {'swaps':>5} {'falls':>5} {'worst change':>13}   example"]
    for report in reports:
        swaps: List[SwapResult] = getattr(report, attribute)
        falls = [swap for swap in swaps if not swap.holds]
        worst = min((swap.change for swap in swaps), default=float("nan"))
        example = ""
        if swaps:
            first = swaps[0]
            example = f"{first.out_name} -> {first.in_name}: {first.p_before:.4f} -> {first.p_after:.4f}"
        lines.append(f"{report.fmt:6} {len(swaps):5d} {len(falls):5d} {worst:+13.6f}   {example}")
        for swap in falls:
            lines.append(f"       fell: {swap.out_name} -> {swap.in_name}: {swap.p_before:.6f} -> {swap.p_after:.6f}")
        if report.note:
            lines.append(f"       {report.note}")
    return lines


def render(reports: Sequence[FormatReport]) -> str:
    lines = ["", "PATH PARITY: pinning the searched eleven reproduces its numbers", ""]
    lines.append(f"{'format':6} {'optimised':>9} {'headline':>9} {'gap':>14}")
    for report in reports:
        lines.append(
            f"{report.fmt:6} {str(report.optimised):>9} {report.win_source:>9} "
            f"{(report.parity_gap if report.parity_gap is not None else float('nan')):14.10f}"
        )

    lines += _swap_lines("SWAP MONOTONICITY, DOMINATING UPGRADES (the P1-2 gate)", reports, "dominating")
    lines += _swap_lines("SELECTOR-RANK SWAPS (diagnostic; not the gate)", reports, "ranked")

    lines += ["", "PLAY-MODE RE-SCORE LATENCY, END TO END", ""]
    lines.append(
        f"{'format':6} {'n':>4} {'median':>8} {'p95':>8} {'min':>8} {'predict-win':>12} {'forecast':>10} {'draws':>6}"
    )
    for report in reports:
        lines.append(
            f"{report.fmt:6} {len(report.latency_ms):4d} "
            f"{statistics.median(report.latency_ms):8.1f} {quantile(report.latency_ms, 0.95):8.1f} "
            f"{min(report.latency_ms):8.1f} "
            f"{statistics.median(report.predict_win_ms or [float('nan')]):12.1f} "
            f"{statistics.median(report.simulate_ms or report.performance_ms or [float('nan')]):10.1f} "
            f"{report.samples if report.samples else '-':>6}"
        )
    lines += [
        "",
        "Milliseconds, client to client. predict-win and the forecast call (/simulate, or",
        "/performance/predict where the format has no innings length) are timed against",
        "ml-service directly, so the remainder is go-app's own work: two pool queries, the",
        "fixture resolution and the two hops.",
    ]
    return "\n".join(lines)


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api", default="http://127.0.0.1:8080", help="go-app base URL")
    parser.add_argument("--ml", default="http://127.0.0.1:8000", help="ml-service base URL")
    # No default key (OPS-02): it is the one the stack was started with, or nothing.
    parser.add_argument("--api-key", default=os.environ.get("API_KEY", ""), required=not os.environ.get("API_KEY"))
    parser.add_argument("--models-dir", default=os.environ.get("MODELS_DIR", "output/ml-service"))
    parser.add_argument("--repeats", type=int, default=30, help="timed re-scores per format")
    parser.add_argument("--json", default="", help="write the raw measurements here")
    args = parser.parse_args(argv)

    run_id = runs.read_current(args.models_dir) or runs.newest_run_id(args.models_dir)
    if run_id is None:
        print(f"no run under {args.models_dir}", file=sys.stderr)
        return 2
    store = XiStore.load(runs.run_dir(args.models_dir, run_id))
    print(f"served run {run_id}, ratings through {store.state.last_date}")

    stack = Stack(api=args.api.rstrip("/"), ml=args.ml.rstrip("/"), api_key=args.api_key)
    reports = [probe_format(stack, store, fixture, args.repeats) for fixture in FIXTURES]
    print(render(reports))

    if args.json:
        with open(args.json, "w") as handle:
            json.dump(
                [
                    {
                        "format": report.fmt,
                        "optimised": report.optimised,
                        "win_source": report.win_source,
                        "samples": report.samples,
                        "parity_gap": report.parity_gap,
                        "dominating": [vars(swap) for swap in report.dominating],
                        "ranked": [vars(swap) for swap in report.ranked],
                        "latency_ms": report.latency_ms,
                        "predict_win_ms": report.predict_win_ms,
                        "simulate_ms": report.simulate_ms,
                        "performance_ms": report.performance_ms,
                    }
                    for report in reports
                ],
                handle,
                indent=1,
            )

    violations = sum(1 for report in reports for swap in report.dominating if not swap.holds)
    parity = sum(1 for report in reports if report.parity_gap is None or abs(report.parity_gap) > EPS)
    print(f"\ndominating-upgrade violations: {violations}; formats failing path parity: {parity}")
    return 1 if violations or parity else 0


if __name__ == "__main__":
    sys.exit(main())
