"""Shared factories for the ml.xi unit tests: hand-built matches, deliveries and sources."""

from __future__ import annotations

from datetime import date, timedelta
from typing import List, Optional, Sequence

import numpy as np

from ml.xi.sources import Deliveries, MatchRecord


def make_deliveries(
    batters: List[str],
    bowlers: List[str],
    runs: List[int],
    wickets: List[int],
    overs: Optional[List[int]] = None,
    innings: Optional[List[int]] = None,
    players_out: Optional[List[str]] = None,
) -> Deliveries:
    n = len(batters)
    return Deliveries(
        over=np.asarray(overs, dtype=int) if overs is not None else np.arange(n) // 6,
        innings=np.asarray(innings, dtype=int) if innings is not None else np.zeros(n, dtype=int),
        batter=np.asarray(batters, dtype=object),
        bowler=np.asarray(bowlers, dtype=object),
        runs_batter=np.asarray(runs, dtype=float),
        runs_total=np.asarray(runs, dtype=float),
        runs_bowler=np.asarray(runs, dtype=float),
        wicket=np.asarray(wickets, dtype=float),
        bowler_wicket=np.asarray(wickets, dtype=float),
        stumping=np.zeros(n),
        fielders=[[] for _ in range(n)],
        player_out=np.asarray(players_out, dtype=object)
        if players_out is not None
        else np.asarray(["" for _ in range(n)], dtype=object),
    )


def make_match(
    mid: str,
    day: int,
    winner: Optional[str],
    team1_players: List[str],
    team2_players: List[str],
    deliveries: Deliveries,
    fmt: str = "T20",
    team1: str = "A",
    team2: str = "B",
    gender: str = "male",
    start: date = date(2024, 1, 1),
) -> MatchRecord:
    return MatchRecord(
        match_id=mid,
        match_date=start + timedelta(days=day),
        format_code=fmt,
        team1=team1,
        team2=team2,
        venue="V",
        gender=gender,
        team1_players=team1_players,
        team2_players=team2_players,
        winner=winner,
        result=None,
        deliveries=deliveries,
    )


def xi(prefix: str) -> List[str]:
    return [f"{prefix}{i}" for i in range(11)]


class ListSource:
    """A MatchSource over an in-memory list; counts are left for the pass to infer."""

    def __init__(self, matches: Sequence[MatchRecord]):
        self.matches = list(matches)

    def iter_matches(self):
        yield from self.matches

    def birth_dates(self):
        return {}
