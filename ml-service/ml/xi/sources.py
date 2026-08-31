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
from typing import Dict, Iterator, List, Optional, Protocol, Sequence, Tuple

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
    # Key of the player dismissed on the ball, "" when nobody was. The first-listed wicket
    # only, which is all the go-app importer stores (ball_event.player_out_id), so the two
    # sources produce the same player-match rows.
    player_out: np.ndarray = field(default_factory=lambda: np.array([], dtype=object))

    def __len__(self) -> int:
        return int(len(self.over))

    @staticmethod
    def empty() -> "Deliveries":
        z = np.zeros(0)
        empty_keys = np.array([], dtype=object)
        return Deliveries(z.astype(int), z.astype(int), empty_keys, empty_keys, z, z, z, z, z, [], empty_keys.copy())


def batting_positions(deliveries: Deliveries) -> Dict[str, int]:
    """1-based batting position per player key: the order of first appearance on strike in
    the player's first batting innings. A player who never faced a ball has no entry --
    the source does not record a batting order, only who actually batted."""
    positions: Dict[str, int] = {}
    for inning in np.unique(deliveries.innings):
        batters = deliveries.batter[deliveries.innings == inning]
        unique_keys, first_index = np.unique(batters, return_index=True)
        for position, i in enumerate(np.argsort(first_index), start=1):
            positions.setdefault(unique_keys[i], position)
    return positions


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


@dataclass
class SourceCounts:
    """What a source did with every match its store holds.

    A source that quietly drops a match is the failure mode this exists to make visible:
    the database ran for two years holding 22,425 matches for 22,734 files and nothing
    said so (§10.4 of the rearchitecture plan). Every match is therefore accounted for --
    yielded, out of scope, or unusable -- and the data-quality gate asserts the three add
    up to what was offered.
    """

    offered: int = 0  # matches the store holds, before this run's filters
    out_of_scope: int = 0  # a format or date range this run did not ask for
    unusable: int = 0  # in scope, but not a two-team match with two recorded squads
    yielded: int = 0

    @property
    def accounted(self) -> int:
        return self.out_of_scope + self.unusable + self.yielded

    def as_dict(self) -> dict:
        return {
            "offered": self.offered,
            "out_of_scope": self.out_of_scope,
            "unusable": self.unusable,
            "yielded": self.yielded,
        }


class MatchSource(Protocol):
    #: Populated while iterating; read by the rating pass once iteration is done.
    counts: SourceCounts

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
    over, inn, bat, bowl, rb, rt, wk, bwk, st, fld, out = [], [], [], [], [], [], [], [], [], [], []
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
                # The first-listed wicket only, mirroring the go-app importer.
                out_name = (wickets[0].get("player_out") or "").strip() if wickets else ""
                out.append(registry.get(out_name, "name:" + out_name) if out_name else "")
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
        np.asarray(out, dtype=object),
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
    squad1, squad2 = _squads(players[team1], players[team2], registry)
    return MatchRecord(
        match_id=os.path.basename(path).rsplit(".", 1)[0],
        match_date=date.fromisoformat(info["dates"][0]),
        format_code=fmt,
        team1=team1,
        team2=team2,
        venue=info.get("venue") or "",
        gender=info.get("gender") or "",
        team1_players=squad1,
        team2_players=squad2,
        winner=outcome.get("winner"),
        result=outcome.get("result"),
        deliveries=_deliveries_from_cricsheet(innings, registry),
    )


def _squads(names1: Sequence[str], names2: Sequence[str], registry: dict) -> Tuple[List[str], List[str]]:
    """Both sides as player keys, with a person named on both sides left out of both.

    ``info.registry`` is keyed by name *within a file*, so two people who share a scorecard
    name collapse into one identifier and the source cannot say which side each delivery
    belongs to. Two files in the current dataset do this -- KV Sharma for Vidarbha and
    Railways, J Butler for the Isle of Man and Guernsey -- and keeping the key in both
    squads hands one player's ratings to both teams at once.

    The go-app importer has always dropped them from both sides rather than guessing
    (S-3c in the win-probability checklist); this is that rule, applied to the other source.
    Both matches lose one player from an eleven, which is the honest reading of a source
    that does not know.
    """
    squad1 = [registry.get(n, "name:" + n) for n in names1]
    squad2 = [registry.get(n, "name:" + n) for n in names2]
    contested = set(squad1) & set(squad2)
    if not contested:
        return squad1, squad2
    logger.warning("player key on both sides, omitted from both squads: %s", ", ".join(sorted(contested)))
    return [k for k in squad1 if k not in contested], [k for k in squad2 if k not in contested]


class CricsheetJsonSource:
    """All ``*.json`` files in a directory, sorted by (date, id)."""

    def __init__(self, directory: str, international_teams: Sequence[str], formats: Sequence[str] = FORMAT_CODES):
        self.directory = directory
        self.international_teams = list(international_teams)
        self.formats = set(formats)
        self.counts = SourceCounts()

    def iter_matches(self) -> Iterator[MatchRecord]:
        names = sorted(n for n in os.listdir(self.directory) if n.endswith(".json"))
        self.counts = SourceCounts(offered=len(names))
        records: List[MatchRecord] = []
        for n in names:
            rec = parse_cricsheet_file(os.path.join(self.directory, n), self.international_teams)
            # The two reasons a file yields nothing are worth telling apart: a format this
            # run did not ask for is expected, a file that will not parse into a two-team
            # match with two squads is a fact about the archive.
            if rec is None:
                self.counts.unusable += 1
                continue
            if rec.format_code not in self.formats:
                self.counts.out_of_scope += 1
                continue
            records.append(rec)
        self.counts.yielded = len(records)
        logger.info(
            "cricsheet source: %d matches from %d files (%d out of scope, %d unusable)",
            self.counts.yielded,
            self.counts.offered,
            self.counts.out_of_scope,
            self.counts.unusable,
        )
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

# Every match in the date range, whatever its format, so a run can say how many it left
# out on purpose. Without it "22,425 matches" is a number with nothing to check it against.
_MATCH_COUNT_SQL = "SELECT count(*) FROM match WHERE match_date < %s"


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
        JOIN player f ON f.id = u.fielder_id),
       {_player_key("pout")}
FROM ball_event be
LEFT JOIN player striker ON striker.id = be.striker_id
LEFT JOIN player bowler ON bowler.id = be.bowler_id
LEFT JOIN player pout ON pout.id = be.player_out_id
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
        self.counts = SourceCounts()

    def iter_matches(self) -> Iterator[MatchRecord]:
        with self.connection.cursor() as cur:
            cur.execute(_MATCH_COUNT_SQL, (self.before,))
            offered = int(cur.fetchone()[0])
            cur.execute(_MATCH_SQL, (self.formats, self.before))
            matches = cur.fetchall()
        self.counts = SourceCounts(offered=offered, out_of_scope=offered - len(matches))
        logger.info("postgres source: %d matches of %d in the date range", len(matches), offered)
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
                self.counts.unusable += 1
                continue
            self.counts.yielded += 1
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
        player_out=np.asarray([r[8] or "" for r in rows], dtype=object),
    )
