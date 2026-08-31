"""Match sources for the rating pass.

A source yields matches in date order, each with both elevens and its deliveries. Two
implementations: Postgres (the go-app schema -- production) and a Cricsheet JSON directory
(offline reproduction and tests). Both key players by the Cricsheet registry identifier --
``player.external_id`` for Postgres, ``info.registry.people`` for JSON -- so the two paths
produce the same keys for the same people and their artifacts are comparable (P-1).
"""

from __future__ import annotations

import json
import logging
import os
from dataclasses import dataclass, field
from datetime import date
from typing import Iterator, List, Optional, Protocol, Sequence

import numpy as np

from ml.xi.contract import FORMAT_CODES

logger = logging.getLogger(__name__)

# Wicket kinds credited to the bowler. Run outs, retirements and obstruction are not.
BOWLER_CREDITED_KINDS = frozenset({"caught", "bowled", "lbw", "caught and bowled", "stumped", "hit wicket"})


@dataclass
class Deliveries:
    """Column-oriented deliveries of one match, in playing order."""

    over: np.ndarray  # int, over number within the innings
    innings: np.ndarray  # int, 0-based innings index
    batter: np.ndarray  # object, player key
    bowler: np.ndarray  # object, player key
    runs_batter: np.ndarray  # float
    runs_total: np.ndarray  # float
    wicket: np.ndarray  # float 0/1, any dismissal on the ball
    bowler_wicket: np.ndarray  # float 0/1, dismissal credited to the bowler
    stumping: np.ndarray  # float 0/1
    fielders: List[Sequence[str]] = field(default_factory=list)  # per ball, keys of credited fielders

    def __len__(self) -> int:
        return int(len(self.over))

    @staticmethod
    def empty() -> "Deliveries":
        z = np.zeros(0)
        return Deliveries(
            z.astype(int), z.astype(int), np.array([], dtype=object), np.array([], dtype=object), z, z, z, z, z, []
        )


@dataclass
class MatchRecord:
    match_id: str
    match_date: date
    format_code: str
    team1: str  # side batting first
    team2: str
    venue: str
    gender: str
    team1_players: List[str]
    team2_players: List[str]
    winner: Optional[str]  # team1 / team2 name, or None (no result, tie, draw)
    result: Optional[str]  # 'tie' | 'draw' | 'no result' | None
    deliveries: Deliveries

    @property
    def outcome(self) -> Optional[float]:
        """1.0 team1 won, 0.0 team2 won, None otherwise."""
        if self.winner == self.team1:
            return 1.0
        if self.winner == self.team2:
            return 0.0
        return None


class MatchSource(Protocol):
    def iter_matches(self) -> Iterator[MatchRecord]:
        """Yield matches in ascending date order."""
        ...


# ---------------------------------------------------------------------------
# Cricsheet JSON directory
# ---------------------------------------------------------------------------


def detect_format(match_type: str, teams: Sequence[str], international_teams: Sequence[str]) -> str:
    """Mirror go-app/internal/cricsheet/format.go so offline runs use the same taxonomy."""
    mt = match_type.strip().upper()
    if mt in ("TEST", "MDM"):
        return "TEST"
    if mt in ("ODI", "ODM"):
        return "ODI"
    if mt in ("T20I", "IT20"):
        return "T20I"
    if mt == "T20":
        intl = {t.strip().lower() for t in international_teams}
        if sum(t.strip().lower() in intl for t in teams) >= 2:
            return "T20I"
        return "T20"
    return ""


def _credited_fielder_keys(wickets: list, registry: dict) -> List[str]:
    """Player keys for the fielders credited on one delivery.

    A fielder Cricsheet cannot name is credited to nobody. 469 dismissals in the current
    dataset -- 451 caught, 13 run out, 5 stumped -- record their fielder as
    ``{"substitute": true}`` and nothing else, and keying those on the empty name folded
    all 469 into a single rating slot: one fictional cricketer with a fielding record built
    from 365 different matches. The wicket itself is unaffected; it is counted from the
    dismissal kind, not from who took it.

    The go-app importer has always dropped them (``Collection.UnmarshalJSON`` keeps only
    entries with a name), so this is also what makes the two sources agree key for key.
    """
    keys: List[str] = []
    for wicket in wickets:
        for fielder in wicket.get("fielders", []):
            name = fielder.get("name")
            if not name:
                continue
            keys.append(registry.get(name, "name:" + name))
    return keys


def _deliveries_from_cricsheet(innings: list, registry: dict) -> Deliveries:
    over, inn, bat, bowl, rb, rt, wk, bwk, st, fld = [], [], [], [], [], [], [], [], [], []
    for inning_index, inning in enumerate(innings):
        for ov in inning.get("overs", []):
            for b in ov.get("deliveries", []):
                over.append(ov["over"])
                inn.append(inning_index)
                bat.append(registry.get(b["batter"], "name:" + b["batter"]))
                bowl.append(registry.get(b["bowler"], "name:" + b["bowler"]))
                rb.append(b["runs"]["batter"])
                rt.append(b["runs"]["total"])
                wickets = b.get("wickets") or []
                wk.append(1.0 if wickets else 0.0)
                bwk.append(1.0 if any(w["kind"] in BOWLER_CREDITED_KINDS for w in wickets) else 0.0)
                st.append(1.0 if any(w["kind"] == "stumped" for w in wickets) else 0.0)
                fld.append(_credited_fielder_keys(wickets, registry))
    return Deliveries(
        np.asarray(over, dtype=int),
        np.asarray(inn, dtype=int),
        np.asarray(bat, dtype=object),
        np.asarray(bowl, dtype=object),
        np.asarray(rb, dtype=float),
        np.asarray(rt, dtype=float),
        np.asarray(wk, dtype=float),
        np.asarray(bwk, dtype=float),
        np.asarray(st, dtype=float),
        fld,
    )


def parse_cricsheet_file(path: str, international_teams: Sequence[str]) -> Optional[MatchRecord]:
    """One Cricsheet JSON file -> MatchRecord, or None if it is not a usable two-team match."""
    with open(path) as fh:
        data = json.load(fh)
    info = data["info"]
    teams = info.get("teams") or []
    fmt = detect_format(info.get("match_type", ""), teams, international_teams)
    innings = data.get("innings") or []
    if not fmt or len(teams) != 2 or not innings or innings[0].get("team") not in teams:
        return None
    registry = (info.get("registry") or {}).get("people") or {}
    team1 = innings[0]["team"]
    team2 = teams[1] if teams[0] == team1 else teams[0]
    players = info.get("players") or {}
    if team1 not in players or team2 not in players:
        return None
    outcome = info.get("outcome") or {}
    return MatchRecord(
        match_id=os.path.basename(path).rsplit(".", 1)[0],
        match_date=date.fromisoformat(info["dates"][0]),
        format_code=fmt,
        team1=team1,
        team2=team2,
        venue=info.get("venue") or "",
        gender=info.get("gender") or "",
        team1_players=[registry.get(n, "name:" + n) for n in players[team1]],
        team2_players=[registry.get(n, "name:" + n) for n in players[team2]],
        winner=outcome.get("winner"),
        result=outcome.get("result"),
        deliveries=_deliveries_from_cricsheet(innings, registry),
    )


class CricsheetJsonSource:
    """All ``*.json`` files in a directory, sorted by (date, id)."""

    def __init__(self, directory: str, international_teams: Sequence[str], formats: Sequence[str] = FORMAT_CODES):
        self.directory = directory
        self.international_teams = list(international_teams)
        self.formats = set(formats)

    def iter_matches(self) -> Iterator[MatchRecord]:
        names = sorted(n for n in os.listdir(self.directory) if n.endswith(".json"))
        records: List[MatchRecord] = []
        skipped = 0
        for n in names:
            rec = parse_cricsheet_file(os.path.join(self.directory, n), self.international_teams)
            if rec is None or rec.format_code not in self.formats:
                skipped += 1
                continue
            records.append(rec)
        logger.info("cricsheet source: %d matches, %d skipped", len(records), skipped)
        records.sort(key=lambda r: (r.match_date, r.match_id))
        yield from records


# ---------------------------------------------------------------------------
# Postgres (go-app schema)
# ---------------------------------------------------------------------------

_MATCH_SQL = """
SELECT m.match_id, m.match_date, mf.code, m.gender, COALESCE(m.venue_id, 0),
       mi.batting_team_opposition_id, mi.bowling_team_opposition_id, m.outcome_winner_opposition_id
FROM match m
JOIN match_format mf ON mf.id = m.format_id
JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
WHERE mf.code = ANY(%s) AND m.match_date < %s
ORDER BY m.match_date, m.match_id
"""


def _player_key(alias: str) -> str:
    """The player key expression for one joined ``player`` alias.

    The key is the Cricsheet registry identifier, exactly as the JSON path builds it, so
    a rating artifact trained from Postgres and one trained from the raw files describe
    the same people and are comparable row for row (P-1). ``external_id`` is unique, so
    joining player changes no cardinality, and the fallback mirrors the JSON path's for a
    person the source has no registry entry for.
    """
    return f"COALESCE({alias}.external_id, 'name:' || {alias}.player_name)"


_PLAYERS_SQL = f"""
SELECT {_player_key("p")}, mp.opposition_id
FROM match_player mp
JOIN player p ON p.id = mp.player_id
WHERE mp.match_id = %s
"""

# Fielders are stored as an array of ids, so their keys are looked up in a lateral join
# that preserves the array's order -- the rating pass credits them positionally.
_BALLS_SQL = f"""
SELECT be.innings, be.over,
       {_player_key("striker")}, {_player_key("bowler")},
       be.runs_batter, be.runs_total, be.wicket_kind,
       (SELECT array_agg({_player_key("f")} ORDER BY u.position)
        FROM unnest(be.fielder_ids) WITH ORDINALITY AS u(fielder_id, position)
        JOIN player f ON f.id = u.fielder_id)
FROM ball_event be
LEFT JOIN player striker ON striker.id = be.striker_id
LEFT JOIN player bowler ON bowler.id = be.bowler_id
WHERE be.match_id = %s
ORDER BY be.innings, be.ball_seq
"""


class PostgresSource:
    """Reads match, match_player and ball_event from the go-app database.

    ``winner`` is the opposition id as a string, matching the team keys, so Elo and
    head-to-head state are keyed the way the go-app identifies teams -- which, since the
    identity migration, is one id per (team name, gender) rather than one per name.

    Players are keyed by ``player.external_id``, the Cricsheet registry identifier, so
    this source and the JSON one produce the same key for the same person (P-1).
    """

    def __init__(self, connection, formats: Sequence[str] = FORMAT_CODES, before: Optional[date] = None):
        self.connection = connection
        self.formats = list(formats)
        self.before = before or date(9999, 1, 1)

    def iter_matches(self) -> Iterator[MatchRecord]:
        with self.connection.cursor() as cur:
            cur.execute(_MATCH_SQL, (self.formats, self.before))
            matches = cur.fetchall()
        logger.info("postgres source: %d matches", len(matches))
        for match_id, match_date, fmt, gender, venue_id, team1_id, team2_id, winner_id in matches:
            with self.connection.cursor() as cur:
                cur.execute(_PLAYERS_SQL, (match_id,))
                players = cur.fetchall()
                cur.execute(_BALLS_SQL, (match_id,))
                balls = cur.fetchall()
            t1 = [key for key, opp in players if opp == team1_id]
            t2 = [key for key, opp in players if opp == team2_id]
            if not t1 or not t2:
                logger.warning("match %s has no recorded squad for a side; skipped", match_id)
                continue
            yield MatchRecord(
                match_id=str(match_id),
                match_date=match_date,
                format_code=fmt,
                team1=str(team1_id),
                team2=str(team2_id),
                venue=str(venue_id),
                gender=gender or "",
                team1_players=t1,
                team2_players=t2,
                winner=None if winner_id is None else str(winner_id),
                result=None,
                deliveries=_deliveries_from_rows(balls),
            )


def _deliveries_from_rows(rows) -> Deliveries:
    if not rows:
        return Deliveries.empty()
    innings = np.asarray([r[0] for r in rows], dtype=int)
    innings = innings - innings.min()
    kinds = [r[6] or "" for r in rows]
    return Deliveries(
        over=np.asarray([r[1] for r in rows], dtype=int),
        innings=innings,
        batter=np.asarray([r[2] if r[2] else "" for r in rows], dtype=object),
        bowler=np.asarray([r[3] if r[3] else "" for r in rows], dtype=object),
        runs_batter=np.asarray([r[4] for r in rows], dtype=float),
        runs_total=np.asarray([r[5] for r in rows], dtype=float),
        wicket=np.asarray([1.0 if k else 0.0 for k in kinds]),
        bowler_wicket=np.asarray([1.0 if k in BOWLER_CREDITED_KINDS else 0.0 for k in kinds]),
        stumping=np.asarray([1.0 if k == "stumped" else 0.0 for k in kinds]),
        fielders=[list(r[7] or []) for r in rows],
    )
