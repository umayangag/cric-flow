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

from ml.xi.biography import BirthDates, load_birth_dates_csv, load_birth_dates_postgres
from ml.xi.contract import FORMAT_CODES
from ml.xi.geography import VenueCountries, load_venue_countries_csv, load_venue_countries_postgres
from ml.xi.lineage import TeamLineage
from ml.xi.lineage import load as load_lineage
from ml.xi.stakes import UNLABELLED, Header, MatchStakes
from ml.xi.stakes import derive as derive_stakes
from ml.xi.wicketkinds import Vocabulary
from ml.xi.wicketkinds import vocabulary as wicket_vocabulary

logger = logging.getLogger(__name__)


@dataclass
class Deliveries:
    """Column-oriented deliveries of one match, in playing order."""

    over: np.ndarray  # int, over number within the innings
    innings: np.ndarray  # int, 0-based innings index
    batter: np.ndarray  # object, player key
    bowler: np.ndarray  # object, player key
    runs_batter: np.ndarray  # float
    runs_total: np.ndarray  # float, everything the innings scored off the ball
    # The part of runs_total charged to the bowler, ``runs_conceded_by_bowler`` on both
    # sources: byes, leg-byes and penalty runs are the innings' and not his (FEAT-08).
    runs_bowler: np.ndarray  # float
    # 1.0 when the striker faced the ball -- everything but a wide -- ``faced_by_batter`` on
    # both sources. A no-ball is faced; neither is one of the bowler's six (IMPORT-05).
    faced: np.ndarray  # float 0/1
    # The wickets the innings lost on the ball, by the vocabulary (``wicket_columns``):
    # every kind but a batter retired hurt or retired not out. A count, not a flag -- a
    # delivery can carry two dismissals, and one in the archive carries ten -- though it
    # is 0 or 1 on all but sixteen of 11.6 million (IMPORT-06).
    wicket: np.ndarray  # float
    bowler_wicket: np.ndarray  # float 0/1, a dismissal credited to the bowler on the ball
    stumping: np.ndarray  # float 0/1
    fielders: List[Sequence[str]] = field(default_factory=list)  # per ball, keys of credited fielders
    # Per ball, the keys of the players dismissed on it -- every wicket the file lists,
    # retirements not out left out -- in the file's order. Empty on a ball nobody was out on.
    players_out: List[Sequence[str]] = field(default_factory=list)

    def __len__(self) -> int:
        return int(len(self.over))

    @staticmethod
    def empty() -> "Deliveries":
        z = np.zeros(0)
        empty_keys = np.array([], dtype=object)
        return Deliveries(z.astype(int), z.astype(int), empty_keys, empty_keys, z, z, z, z, z, z, z, [], [])


def runs_conceded_by_bowler(runs_total, byes, legbyes, penalty) -> np.ndarray:
    """The part of each delivery's total that is charged to the bowler: the batter's runs
    plus wides and no-balls, which is the total less byes, leg-byes and penalty runs.

    Byes and leg-byes are the fielding side's -- a keeper's miss, a deflection off the
    pad -- and a penalty is the umpire's, so a bowler's runs conceded that includes them
    carries his keeper's quality as noise (FEAT-08). This is the one rule for both rating
    sources, and it is the rule the go-app importer applies to ``bowling_data.runs``
    (``cricsheet.Delivery.RunsConcededByBowler``, IMPORT-04), so a bowler's figures in the
    database and his ``runs_conceded`` target agree delivery for delivery.
    """
    return (
        np.asarray(runs_total, dtype=float)
        - np.asarray(byes, dtype=float)
        - np.asarray(legbyes, dtype=float)
        - np.asarray(penalty, dtype=float)
    )


#: One ball's wickets as (kind, player key) pairs in the file's order; "" for a player the
#: source could not key.
BallWickets = Sequence[Tuple[str, str]]


def wicket_columns(
    wickets_per_ball: Sequence[BallWickets], vocabulary: Optional[Vocabulary] = None
) -> Tuple[np.ndarray, np.ndarray, np.ndarray, List[Sequence[str]]]:
    """Each ball's wickets -> ``wicket``, ``bowler_wicket``, ``stumping`` and ``players_out``.

    The vocabulary (``configs/wicket_kinds.json``) says what each kind is: a run out is a
    wicket the innings lost and not the bowler's, a batter retired hurt is neither and is
    nobody's dismissal. This is the one rule for both rating sources, and it is the rule
    the go-app importer applies to ``bowling_data.wickets`` and ``match_inning.wickets_lost``
    (``cricsheet.Delivery.TallyWickets``, the same file), so a bowler's wickets in the
    database and his ``wickets`` target agree kind for kind, and a batter's ``dismissals``
    target counts the innings he was out in and not the one he retired hurt from. Every
    wicket on the ball is read: until IMPORT-06 was fixed the database held the first one
    only, and this path mirrored it.
    """
    vocabulary = vocabulary or wicket_vocabulary()
    n = len(wickets_per_ball)
    wicket, bowler_wicket, stumping = np.zeros(n), np.zeros(n), np.zeros(n)
    players_out: List[Sequence[str]] = []
    for i, wickets in enumerate(wickets_per_ball):
        dismissed: List[str] = []
        for kind, key in wickets:
            if vocabulary.is_dismissal(kind):
                wicket[i] += 1.0
                if key:
                    dismissed.append(key)
            if vocabulary.is_credited_to_bowler(kind):
                bowler_wicket[i] = 1.0
            if vocabulary.is_stumping(kind):
                stumping[i] = 1.0
        players_out.append(dismissed)
    return wicket, bowler_wicket, stumping, players_out


def faced_by_batter(wides) -> np.ndarray:
    """1.0 for each delivery the striker faced: every ball but a wide, which passes out of
    his reach and is not one he could have played.

    A no-ball is faced -- he may hit it -- so it is one of his balls while it is not one of
    the bowler's six; a wide is neither. This is the one rule for both rating sources, and
    it is the rule the go-app importer applies to ``batting_data.balls``
    (``cricsheet.Delivery.FacedByBatter``), so a batter's balls in the database and his
    ``balls_faced`` target agree delivery for delivery. Until IMPORT-05 was fixed this path
    counted every delivery as faced, wides included, and the importer counted none of the
    no-balls: two definitions erring in opposite directions.
    """
    return (np.asarray(wides, dtype=float) == 0).astype(float)


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
    # The eleven each side started with: everyone the source lists for the side, less the
    # replacements (FEAT-02). Cricsheet lists everyone who took the field, so a side that
    # used a concussion substitute, an impact player or a supersub is listed as twelve;
    # rating and aggregating the twelve is a train/serve mismatch (serving always
    # aggregates eleven) and a post-start leak (a replacement is decided during the match).
    team1_players: List[str]
    team2_players: List[str]
    # The side the match went to: team1 / team2, or None for a no-result, a draw or a tie
    # nobody broke. A tie settled by a super over or a bowl-out has a winner (IMPORT-02);
    # ``result`` stays 'tie' beside it, which is how such a win is told from an outright one.
    winner: Optional[str]
    result: Optional[str]  # 'tie' | 'draw' | 'no result' | None
    deliveries: Deliveries
    # The competition the match was played in: Cricsheet's ``info.event.name``, which the
    # importer stores verbatim as ``match.event_name``, so both sources spell it the same.
    # Empty when the source names none; an empty key is no key (``RatingState.update``).
    competition: str = ""
    # The rest of Cricsheet's ``info.event``, stored verbatim by both sources (migration
    # 0011 for Postgres) and read only by ``ml.xi.stakes``: the fixture's number in its
    # competition, the round, and the pool.
    match_number: Optional[int] = None
    event_stage: str = ""
    event_group: str = ""
    # What the match was worth (X-3), filled by the source once it knows the whole edition:
    # a fixture's stakes are a fact about its competition, not about the file it came in.
    # A record built for the serving path carries the unlabelled default.
    stakes: MatchStakes = UNLABELLED
    # The keys of the players who joined either side after the match started -- the ones
    # left out of ``team1_players`` / ``team2_players`` -- so the pass can count them and
    # ``make xi-parity`` can compare the count across sources. Their deliveries stay in
    # ``deliveries``: what they did is theirs, and the ledgers read it as anyone else's.
    replacements: List[str] = field(default_factory=list)
    # The side that won the toss, as a team key (FEAT-05): Cricsheet's ``info.toss.winner``
    # on both sources, through the same club-and-gender key the sides carry. None where the
    # source records no toss, and on a record built for the serving path.
    toss_winner: Optional[str] = None
    # The last day the match was played on (FEAT-09): Cricsheet's ``dates[-1]`` on both
    # sources -- ``match.match_end_date`` (migration 0024) on Postgres. ``match_date``,
    # ``dates[0]``, stays the day every feature is read at; this is the day after which the
    # match is folded into the state, so a Test's later days are not in the state of a
    # fixture played while it was still on. None is a match played on its one day.
    match_end_date: Optional[date] = None

    @property
    def last_day(self) -> date:
        """The day the match ended: its last day, or its only one."""
        return self.match_end_date or self.match_date

    @property
    def toss_won_by_team1(self) -> Optional[float]:
        """1.0 when the side batting first won the toss (it chose to bat), 0.0 when it was
        put in, None when the source records no toss. The decision is not read: on every
        match of the archive it is exactly this comparison (0 of 22,905 disagree)."""
        if self.toss_winner is None:
            return None
        return 1.0 if self.toss_winner == self.team1 else 0.0

    @property
    def outcome(self) -> Optional[float]:
        """1.0 team1 won, 0.0 team2 won, None otherwise."""
        if self.winner == self.team1:
            return 1.0
        if self.winner == self.team2:
            return 0.0
        return None

    @property
    def drawn_or_tied(self) -> bool:
        """The match reached a result that nobody won: a draw, or a tie no tie-breaker
        settled. Form reads it as half a win for each side (``RatingState.update``), so it
        is the one place ``result`` changes a feature -- which is why the rating pass counts
        it and ``make xi-parity`` compares the count (FEAT-04). A tie-breaker win has a
        winner and is a win; a no-result moves nothing."""
        return self.outcome is None and self.result in DRAWN_OR_TIED_RESULTS


#: Cricsheet's ``outcome.result`` values for a match both sides played to a finish without
#: either winning it. ``'no result'`` is not one: an abandoned match tells form nothing.
DRAWN_OR_TIED_RESULTS = ("tie", "draw")


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

    def team_key_for(self, name: str, gender: str) -> Optional[str]:
        """The key this source rates a club under, given the (name, gender) an outside
        dataset spells it with.

        It is the identity layer's join point (D-10): a dataset that arrives keyed by team
        name -- X-4's closing odds are the first -- reaches our matches through this and
        never through string equality on a frame column, so a rename or a men's/women's
        namesake cannot join to the wrong side. ``None`` when the source does not know the
        name; a caller counts that rather than guessing.
        """
        ...

    def birth_dates(self) -> BirthDates:
        """Date of birth per player key, for every player the source knows one for
        (X-1b). The rating state reads it once; a player absent from the map has an
        unknown age, which is a category of its own and never an imputed value."""
        ...

    def venue_countries(self) -> VenueCountries:
        """Home region per venue key, for every venue the curated table places
        (``ml.xi.geography``, FEAT-05). Static, read once; a venue absent from the map
        is in no region and the match reads as neutral ground."""
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


def winning_team(outcome: dict) -> Optional[str]:
    """The side a Cricsheet ``info.outcome`` says the match went to, or None.

    The outright ``winner``, else the side that won the tie-breaker -- ``eliminator`` for a
    super over, ``bowl_out`` for a bowl-out -- else None for a draw, a no-result or a tie
    that was left as one. The go-app importer applies the same rule
    (``cricsheet.Outcome.WinningTeam``, IMPORT-02), so both sources agree on which matches
    have a winner and enter the frame.
    """
    for key in ("winner", "eliminator", "bowl_out"):
        side = str(outcome.get(key) or "").strip()
        if side:
            return side
    return None


def played_innings(innings: list) -> List[dict]:
    """The innings of the match, in playing order, without super overs.

    A super over is a tie-breaker Cricsheet appends to the innings list (``super_over:
    true``; 226 of them in the current archive), not an innings anyone bats a career in.
    The go-app importer leaves them out through ``Match.PlayedInnings`` (IMPORT-01), and
    this is the same rule on the archive path, so the two sources yield the same
    deliveries for a tied match and the H-8 parity check compares like with like.
    """
    return [inning for inning in innings if not inning.get("super_over")]


def _deliveries_from_cricsheet(innings: list, registry: dict) -> Deliveries:
    over, inn, bat, bowl, rb, rt, fld = [], [], [], [], [], [], []
    wickets_per_ball: List[BallWickets] = []
    wides, byes, legbyes, penalty = [], [], [], []
    for inning_index, inning in enumerate(played_innings(innings)):
        for ov in inning.get("overs", []):
            for b in ov.get("deliveries", []):
                over.append(ov["over"])
                inn.append(inning_index)
                bat.append(registry.get(b["batter"], "name:" + b["batter"]))
                bowl.append(registry.get(b["bowler"], "name:" + b["bowler"]))
                rb.append(b["runs"]["batter"])
                rt.append(b["runs"]["total"])
                # Cricsheet writes ``extras`` only on a delivery that has some, as an object
                # of the kinds present; a kind it does not name is zero.
                extras = b.get("extras") or {}
                wides.append(extras.get("wides", 0))
                byes.append(extras.get("byes", 0))
                legbyes.append(extras.get("legbyes", 0))
                penalty.append(extras.get("penalty", 0))
                wickets = b.get("wickets") or []
                fld.append(_credited_fielder_keys(wickets, registry))
                wickets_per_ball.append(_wickets_of(wickets, registry))
    wicket, bowler_wicket, stumping, players_out = wicket_columns(wickets_per_ball)
    return Deliveries(
        np.asarray(over, dtype=int),
        np.asarray(inn, dtype=int),
        np.asarray(bat, dtype=object),
        np.asarray(bowl, dtype=object),
        np.asarray(rb, dtype=float),
        np.asarray(rt, dtype=float),
        runs_conceded_by_bowler(rt, byes, legbyes, penalty),
        faced_by_batter(wides),
        wicket,
        bowler_wicket,
        stumping,
        fld,
        players_out,
    )


def _wickets_of(wickets: list, registry: dict) -> BallWickets:
    """One delivery's ``wickets`` list as (kind, player key) pairs, in the file's order."""
    pairs: List[Tuple[str, str]] = []
    for w in wickets:
        name = (w.get("player_out") or "").strip()
        pairs.append((w["kind"], registry.get(name, "name:" + name) if name else ""))
    return pairs


def parse_cricsheet_file(
    path: str, international_teams: Sequence[str], lineage: Optional[TeamLineage] = None
) -> Optional[MatchRecord]:
    """One Cricsheet JSON file -> MatchRecord, or None if it is not a usable two-team match.

    ``lineage`` maps a club's superseded name onto its current one, so a rebrand does not
    reset the team's Elo and head-to-head. The database does the same through
    ``opposition.canonical_id``; both read ``configs/team_lineage.json``.
    """
    with open(path) as fh:
        data = json.load(fh)
    info = data["info"]
    teams = info.get("teams") or []
    fmt = detect_format(info.get("match_type", ""), teams, international_teams)
    innings = data.get("innings") or []
    if not fmt or len(teams) != 2 or not innings or innings[0].get("team") not in teams:
        return None
    registry = (info.get("registry") or {}).get("people") or {}
    event = info.get("event") or {}
    team1 = innings[0]["team"]
    team2 = teams[1] if teams[0] == team1 else teams[0]
    players = info.get("players") or {}
    if team1 not in players or team2 not in players:
        return None
    outcome = info.get("outcome") or {}
    squad1, squad2 = _squads(players[team1], players[team2], registry)
    squad1, squad2, replacements = _starting_elevens(team1, squad1, team2, squad2, replacement_keys(innings, registry))
    # Team *keys* become the club, and carry the gender; the squad lookups above and the
    # winner comparison below use the names the file actually carries, which is why the
    # mapping happens here and not when the names are read.
    gender = info.get("gender") or ""
    lineage = lineage or TeamLineage()
    club1, club2 = team_key(team1, gender, lineage), team_key(team2, gender, lineage)
    winner = winning_team(outcome)
    if winner:
        winner = team_key(winner, gender, lineage)
    toss_winner = str((info.get("toss") or {}).get("winner") or "").strip()
    return MatchRecord(
        match_id=os.path.basename(path).rsplit(".", 1)[0],
        match_date=date.fromisoformat(info["dates"][0]),
        format_code=fmt,
        team1=club1,
        team2=club2,
        venue=info.get("venue") or "",
        gender=gender,
        team1_players=squad1,
        team2_players=squad2,
        winner=winner,
        result=outcome.get("result"),
        deliveries=_deliveries_from_cricsheet(innings, registry),
        competition=event.get("name") or "",
        match_number=_match_number(event.get("match_number")),
        event_stage=str(event.get("stage") or "").strip(),
        # Cricsheet writes a pool as a string in most files and as a bare number in the
        # rest; the importer normalises the same way (cricsheet.FlexibleTag).
        event_group="" if event.get("group") is None else str(event["group"]).strip(),
        replacements=replacements,
        toss_winner=team_key(toss_winner, gender, lineage) if toss_winner else None,
        match_end_date=date.fromisoformat(info["dates"][-1]),
    )


def replacement_keys(innings: list, registry: dict) -> List[Tuple[str, str]]:
    """The players who joined a side after the match started, as (team, key) pairs in the
    order the file records them coming in.

    Cricsheet keeps a replacement where it happened: a ``replacements.match`` entry on the
    delivery he came in at -- ``in``, ``out``, ``team``, ``reason`` -- not in ``info``, so
    nothing in the twelve-man list says which of them joined late. The entries are read in
    playing order over every innings, and a player who had already gone ``out`` of an
    earlier entry is not a replacement: in the one such file in the archive (1234909, a
    covid replacement) the stand-in went back out and the man he stood in for came back,
    and the side that started is the one without the stand-in. A ``role`` entry -- a
    substitute finishing an injured bowler's over -- changes nobody's membership and is
    not read.

    The team is part of the answer: one file (1537342) names as the man who came in for
    one side a player listed for the other, and matching by key alone would take a starter
    off the wrong team. ``cricsheet.Match.ReplacementPlayers`` is the same rule for the
    go-app importer, which writes it as ``match_player.is_replacement``; ``make xi-parity``
    compares the count.
    """
    pairs: List[Tuple[str, str]] = []
    seen: set = set()
    for inning in innings:
        for over in inning.get("overs", []):
            for ball in over.get("deliveries", []):
                for entry in (ball.get("replacements") or {}).get("match", []):
                    team = str(entry.get("team") or "").strip()
                    came_in = (team, str(entry.get("in") or "").strip())
                    went_out = (team, str(entry.get("out") or "").strip())
                    if came_in[1] and came_in not in seen:
                        pairs.append((team, registry.get(came_in[1], "name:" + came_in[1])))
                    seen.update((came_in, went_out))
    return pairs


def _starting_elevens(
    team1: str,
    squad1: Sequence[str],
    team2: str,
    squad2: Sequence[str],
    replacements: Sequence[Tuple[str, str]],
) -> Tuple[List[str], List[str], List[str]]:
    """Both squads without their replacements, and the replacements that were found --
    each looked for in the side the entry names, as the importer does.

    A replacement its side does not list is logged and not invented: the one in the
    current archive (1537342) names a player the other side lists, so his own side keeps
    him and the side he was said to join stays a twelve, which the pass counts among the
    oversized -- the same reading the importer gives it.
    """
    listed = {team1: set(squad1), team2: set(squad2)}
    found = [key for team, key in replacements if key in listed.get(team, set())]
    unlisted = [f"{team}: {key}" for team, key in replacements if key not in listed.get(team, set())]
    if unlisted:
        logger.warning("replacement not in the side's info.players, side kept as listed: %s", ", ".join(unlisted))
    excluded = set(found)
    return [k for k in squad1 if k not in excluded], [k for k in squad2 if k not in excluded], found


def _match_number(value: object) -> Optional[int]:
    """The fixture's number in its competition, or None where the archive gives none or
    gives something that is not a number."""
    try:
        return int(value)  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return None


def team_key(name: str, gender: str, lineage: "TeamLineage") -> str:
    """The key one team is rated under: its club, and its gender.

    Both halves are identity fixes the database already has and this source did not. 130 of
    the 394 team names in the dataset belong to *both* a men's and a women's side, so a
    name alone gave Australia's two teams one Elo (I-3); and a club that renames is one club
    (I-4). ``opposition.canonical_id`` and the ``(opposition_name, gender)`` key are how
    Postgres says the same thing, and ``make xi-parity`` is what noticed they differed.
    """
    return f"{lineage.club(name, gender)}|{gender}"


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

    def __init__(
        self,
        directory: str,
        international_teams: Sequence[str],
        formats: Sequence[str] = FORMAT_CODES,
        lineage: Optional[TeamLineage] = None,
        birth_dates_path: Optional[str] = None,
        venue_countries_path: Optional[str] = None,
    ):
        self.directory = directory
        self.international_teams = list(international_teams)
        self.formats = set(formats)
        self.lineage = lineage if lineage is not None else load_lineage()
        # The archive carries no biography; the CSV ``ml.xi.biography --export`` writes from
        # the database is how the offline path reads the same dates of birth.
        self.birth_dates_path = birth_dates_path
        # Nor a country: the curated venue table is the one place either source reads it
        # from, keyed by the venue name folded the identity way (``ml.xi.geography``).
        self.venue_countries_path = venue_countries_path
        self.counts = SourceCounts()

    def birth_dates(self) -> BirthDates:
        return load_birth_dates_csv(self.birth_dates_path)

    def venue_countries(self) -> VenueCountries:
        if self.venue_countries_path is None:
            return load_venue_countries_csv()
        return load_venue_countries_csv(self.venue_countries_path)

    def team_key_for(self, name: str, gender: str) -> Optional[str]:
        """The club key for a name, through the same lineage the parser uses. Every name
        yields one -- the archive path has no registry of teams to check a name against --
        so a name this dataset never fielded resolves to a key no match carries, which the
        caller drops as unmatched."""
        return team_key(name, gender, self.lineage)

    def iter_matches(self) -> Iterator[MatchRecord]:
        names = sorted(n for n in os.listdir(self.directory) if n.endswith(".json"))
        self.counts = SourceCounts(offered=len(names))
        records: List[MatchRecord] = []
        for n in names:
            rec = parse_cricsheet_file(os.path.join(self.directory, n), self.international_teams, self.lineage)
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
        # Stakes are derived over the whole set this source yields, because an edition's
        # table is a fact about its fixtures rather than about any one of them (X-3).
        stakes = derive_stakes(records)
        for record in records:
            record.stakes = stakes.get(str(record.match_id), UNLABELLED)
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

# Team keys are the *club*: COALESCE(canonical_id, id) folds a club's superseded rows onto
# its current one, so a rebrand does not restart the team's Elo, form and head-to-head
# (I-4). The Cricsheet source does the same through configs/team_lineage.json.
# A match without a venue yields the empty venue, as the archive path does, so neither
# source accumulates unnamed grounds under a key the other cannot produce.
# ``m.result`` is Cricsheet's own word for a match with no outright winner (migration
# 0017); the archive path reads the same field, so a drawn Test moves both sides' form the
# same way on both sources (FEAT-04). A winner beside 'tie' is a tie-breaker win.
# The toss winner (FEAT-05) is keyed through the same COALESCE as the sides, so "did the
# side batting first win the toss" is one equality on both sources. ``toss_decision`` is
# not read: it is that equality on every row of the archive.
# The sides come from the first innings, and the join to it is a LEFT JOIN on purpose
# (FEAT-11): SQL filters by format and date only, which is what ``out_of_scope`` counts,
# and a match with no first innings -- a toss and then rain -- reaches Python with both
# sides NULL and is counted ``unusable`` there, as the archive path counts a file with no
# innings. An inner join dropped it here, silently, into the other count.
# ``match_end_date`` (migration 0024, FEAT-09) is the match's last day; a database migrated
# but not yet re-imported holds NULL, which reads as the start date -- every match folded at
# the close of its first day, as before -- and ``make xi-parity`` reports the difference
# against the archive as ``multi_day_matches``.
_MATCH_SQL = """
SELECT m.match_id, m.match_date, mf.code, m.gender, m.venue_id,
       COALESCE(bat.canonical_id, bat.id),
       COALESCE(bowl.canonical_id, bowl.id),
       COALESCE(win.canonical_id, win.id),
       COALESCE(m.event_name, ''), m.match_number,
       COALESCE(m.event_stage, ''), COALESCE(m.event_group, ''),
       m.result,
       COALESCE(toss.canonical_id, toss.id),
       COALESCE(m.match_end_date, m.match_date)
FROM match m
JOIN match_format mf ON mf.id = m.format_id
LEFT JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
LEFT JOIN opposition bat ON bat.id = mi.batting_team_opposition_id
LEFT JOIN opposition bowl ON bowl.id = mi.bowling_team_opposition_id
LEFT JOIN opposition win ON win.id = m.outcome_winner_opposition_id
LEFT JOIN opposition toss ON toss.id = m.toss_winner_opposition_id
WHERE mf.code = ANY(%s) AND m.match_date < %s
ORDER BY m.match_date, m.match_id
"""

# Every match in the date range, whatever its format, so a run can say how many it left
# out on purpose. Without it "22,425 matches" is a number with nothing to check it against.
_MATCH_COUNT_SQL = "SELECT count(*) FROM match WHERE match_date < %s"

# The identity layer's name -> key table, for a dataset that arrives keyed by team name
# (X-4's closing odds). The same COALESCE the match query uses, so both speak of one club.
_TEAM_KEYS_SQL = "SELECT opposition_name, gender, COALESCE(canonical_id, id) FROM opposition"


def _player_key(alias: str) -> str:
    """The player key expression for one joined ``player`` alias.

    The key is the Cricsheet registry identifier, exactly as the JSON path builds it, so
    a rating artifact trained from Postgres and one trained from the raw files describe
    the same people and are comparable row for row (P-1). ``external_id`` is unique, so
    joining player changes no cardinality, and the fallback mirrors the JSON path's for a
    person the source has no registry entry for.
    """
    return f"COALESCE({alias}.external_id, 'name:' || {alias}.player_name)"


# ``is_replacement`` (migration 0020, FEAT-02) marks the member who joined after the match
# started; the eleven that started is the rows without it. A database migrated but not
# yet re-imported holds false on every row, so it hands over the twelve, which ``make
# xi-parity`` reports against the archive as ``oversized_squads`` and
# ``replacement_players`` differing.
_PLAYERS_SQL = f"""
SELECT {_player_key("p")}, COALESCE(o.canonical_id, o.id), mp.is_replacement
FROM match_player mp
JOIN player p ON p.id = mp.player_id
JOIN opposition o ON o.id = mp.opposition_id
WHERE mp.match_id = %s
"""

# Fielders come from ``fielding_event`` -- one row per credited fielder per ball (the
# catcher, the keeper of a stumping, each run-out assist) -- because the importer leaves
# ``ball_event.fielder_ids`` NULL on every row. Until P-3 this source read that empty
# column, so it credited no catches and, since the keeper flag comes from stumpings, held
# no keeper at all: a pool from the database could not satisfy ``require_keeper``. The
# archive path never had the defect; the H-8 comparison of the two sources found it.
#
# Playing order is (innings, over, ball), which is unique. ``ball_seq`` is the *legal*-ball
# counter: a wide or no-ball shares it with the legal delivery before it (238,772 such
# pairs), so ordering by it left their relative order to the query planner. Nothing noticed
# until the sequence families (P-3) read the order of deliveries, and the H-8 parity check
# found two players whose dot-streak shares differed between two reads of the same match.
#
# The extras by kind (migration 0018, IMPORT-04) are what let the bowler be charged only
# his own runs and the batter be counted only the balls he faced. A database migrated but
# not yet re-imported holds zeros in them, so it charges him everything and counts every
# wide as faced, which ``make xi-parity`` reports against the archive.
#
# Wickets come from ``ball_event_wicket`` (migration 0019, IMPORT-06) -- one row per wicket
# on the ball, in the file's order -- as two arrays, the kinds and the dismissed players'
# keys, which ``wicket_columns`` classifies by the same vocabulary the importer used.
# ``ball_event`` used to carry one kind and one player, so a delivery with two wickets
# lost its second; ``make xi-parity`` compares ``dismissals`` so that cannot come back.
_BALLS_SQL = f"""
SELECT be.innings, be.over,
       {_player_key("striker")}, {_player_key("bowler")},
       be.runs_batter, be.runs_total,
       (SELECT array_agg(w.kind ORDER BY w.wicket_number)
        FROM ball_event_wicket w
        WHERE w.match_id = be.match_id AND w.innings = be.innings AND w.over = be.over AND w.ball = be.ball),
       (SELECT array_agg({_player_key("f")} ORDER BY fe.id)
        FROM fielding_event fe
        JOIN player f ON f.id = fe.fielder_id
        WHERE fe.match_id = be.match_id AND fe.innings = be.innings AND fe.over = be.over AND fe.ball = be.ball),
       (SELECT array_agg({_player_key("pout")} ORDER BY w.wicket_number)
        FROM ball_event_wicket w
        LEFT JOIN player pout ON pout.id = w.player_out_id
        WHERE w.match_id = be.match_id AND w.innings = be.innings AND w.over = be.over AND w.ball = be.ball),
       be.extras_byes, be.extras_legbyes, be.extras_penalty, be.extras_wides
FROM ball_event be
LEFT JOIN player striker ON striker.id = be.striker_id
LEFT JOIN player bowler ON bowler.id = be.bowler_id
WHERE be.match_id = %s
ORDER BY be.innings, be.over, be.ball
"""


class PostgresSource:
    """Reads match, match_player and ball_event from the go-app database.

    ``winner`` is the opposition id as a string, matching the team keys, so Elo and
    head-to-head state are keyed the way the go-app identifies teams -- which, since the
    identity migration, is one id per (team name, gender) rather than one per name.

    Players are keyed by ``player.external_id``, the Cricsheet registry identifier, so
    this source and the JSON one produce the same key for the same person (P-1).
    """

    def __init__(
        self,
        connection,
        formats: Sequence[str] = FORMAT_CODES,
        before: Optional[date] = None,
        venue_countries_path: Optional[str] = None,
    ):
        self.connection = connection
        self.formats = list(formats)
        self.before = before or date(9999, 1, 1)
        self.venue_countries_path = venue_countries_path
        self.counts = SourceCounts()
        self._team_keys: Optional[Dict[Tuple[str, str], str]] = None

    def team_key_for(self, name: str, gender: str) -> Optional[str]:
        """The club key for a (name, gender), read from ``opposition`` -- the same
        ``COALESCE(canonical_id, id)`` the match query keys teams by, so a rebranded club
        resolves to the row its matches are recorded under. Read once and cached: it is a
        small table and the caller asks per odds row."""
        if self._team_keys is None:
            with self.connection.cursor() as cur:
                cur.execute(_TEAM_KEYS_SQL)
                self._team_keys = {(str(row[0]), str(row[1])): str(row[2]) for row in cur.fetchall()}
        return self._team_keys.get((name, gender))

    def birth_dates(self) -> BirthDates:
        return load_birth_dates_postgres(self.connection)

    def venue_countries(self) -> VenueCountries:
        """Keyed by ``venue.id`` as a string -- the key this source's records carry -- each
        row's name joined to the curated table (``ml.xi.geography``)."""
        if self.venue_countries_path is None:
            return load_venue_countries_postgres(self.connection)
        return load_venue_countries_postgres(self.connection, self.venue_countries_path)

    def iter_matches(self) -> Iterator[MatchRecord]:
        with self.connection.cursor() as cur:
            cur.execute(_MATCH_COUNT_SQL, (self.before,))
            offered = int(cur.fetchone()[0])
            cur.execute(_MATCH_SQL, (self.formats, self.before))
            matches = cur.fetchall()
        self.counts = SourceCounts(offered=offered, out_of_scope=offered - len(matches))
        logger.info("postgres source: %d matches of %d in the date range", len(matches), offered)
        matches = self._with_a_first_innings(matches)
        # The whole edition before the first delivery is read: the same derivation the
        # archive path runs, over the same set of matches, so the two agree or H-15 says
        # which field they differ on (X-3). "The same set" is exact while
        # ``unusable_matches`` is zero, as it is on this dataset -- a match this source
        # later drops for having no recorded squad is in this fixture list and not in the
        # archive path's, which the parity counts would report as a difference.
        stakes = derive_stakes([_stakes_header(row) for row in matches])
        for row in matches:
            match_id, match_date, fmt, gender, venue_id, team1_id, team2_id, winner_id, event_name = row[:9]
            with self.connection.cursor() as cur:
                cur.execute(_PLAYERS_SQL, (match_id,))
                players = cur.fetchall()
                cur.execute(_BALLS_SQL, (match_id,))
                balls = cur.fetchall()
            t1 = [key for key, opp, replacement in players if opp == team1_id and not replacement]
            t2 = [key for key, opp, replacement in players if opp == team2_id and not replacement]
            replacements = [key for key, _opp, replacement in players if replacement]
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
                venue="" if venue_id is None else str(venue_id),
                gender=gender or "",
                team1_players=t1,
                team2_players=t2,
                winner=None if winner_id is None else str(winner_id),
                result=row[12],
                deliveries=_deliveries_from_rows(balls),
                competition=event_name or "",
                match_number=row[9],
                event_stage=row[10] or "",
                event_group=row[11] or "",
                stakes=stakes.get(str(match_id), UNLABELLED),
                replacements=replacements,
                toss_winner=None if row[13] is None else str(row[13]),
                match_end_date=row[14],
            )

    def _with_a_first_innings(self, matches: Sequence[Sequence]) -> List[Sequence]:
        """The rows that have a first innings to name the sides from; the rest are
        counted unusable, as the archive path counts a file with no innings (FEAT-11)."""
        usable: List[Sequence] = []
        for row in matches:
            if row[5] is None or row[6] is None:
                logger.warning("match %s has no first innings to name its sides from; unusable", row[0])
                self.counts.unusable += 1
                continue
            usable.append(row)
        return usable


def _stakes_header(row: Sequence) -> Header:
    """One row of ``_MATCH_SQL`` as the stakes derivation reads it. Teams are the club
    keys the records below carry, so an edition's clubs are counted the same way on both
    sources."""
    return Header(
        match_id=str(row[0]),
        match_date=row[1],
        format_code=row[2],
        gender=row[3] or "",
        team1=str(row[5]),
        team2=str(row[6]),
        competition=row[8] or "",
        match_number=row[9],
        event_stage=row[10] or "",
        event_group=row[11] or "",
        winner=None if row[7] is None else str(row[7]),
    )


def _deliveries_from_rows(rows) -> Deliveries:
    if not rows:
        return Deliveries.empty()
    # ``ball_event.innings`` is 1-based and is the innings' position in
    # ``Match.PlayedInnings`` -- the same position the archive path enumerates -- so one
    # subtracted is the 0-based index, on every match. It used to be the smallest innings
    # present, which is the same number only while innings 1 bowled a ball: a first innings
    # forfeited or made up entirely of extras is absent from ``ball_event``, and the two
    # sources would then have numbered the rest of the match differently and neither said
    # so (IMPORT-12). Fourteen innings in the archive are forfeited and one is all extras,
    # and none of them is the first, which is the only reason this never misfired.
    innings = np.asarray([r[0] for r in rows], dtype=int) - 1
    runs_total = np.asarray([r[5] for r in rows], dtype=float)
    # A wicket whose player the database could not key reads as "" here, as on the
    # archive path; the kinds and keys are two arrays of one length, in wicket order.
    wicket, bowler_wicket, stumping, players_out = wicket_columns(
        [list(zip(r[6] or [], [key or "" for key in (r[8] or [])])) for r in rows]
    )
    return Deliveries(
        over=np.asarray([r[1] for r in rows], dtype=int),
        innings=innings,
        batter=np.asarray([r[2] if r[2] else "" for r in rows], dtype=object),
        bowler=np.asarray([r[3] if r[3] else "" for r in rows], dtype=object),
        runs_batter=np.asarray([r[4] for r in rows], dtype=float),
        runs_total=runs_total,
        runs_bowler=runs_conceded_by_bowler(
            runs_total, [r[9] for r in rows], [r[10] for r in rows], [r[11] for r in rows]
        ),
        faced=faced_by_batter([r[12] for r in rows]),
        wicket=wicket,
        bowler_wicket=bowler_wicket,
        stumping=stumping,
        fielders=[list(r[7] or []) for r in rows],
        players_out=players_out,
    )
