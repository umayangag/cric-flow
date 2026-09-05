"""X-3: what a match was worth, derived from event fields the archive already holds.

Cricsheet's ``info.event`` names the competition (``name``), the fixture's number in it
(``match_number``), the round (``stage``) and the pool (``group``). Nothing new is
acquired here: this module turns those four into two things the rest of the system can
use, and says plainly how much of the archive each one reaches.

**A stage label per match** -- ``group`` / ``knockout`` / ``final`` / ``bilateral``, or
unlabelled. Where the archive states a ``stage`` the label is read from it through the
vocabulary the files actually use (about sixty spellings of a dozen rounds); where it does
not, a match is a ``bilateral`` fixture if its edition had exactly two clubs and a ``group``
fixture if the edition had three or more and the match carries a number or a pool. A match
that fits none of those stays unlabelled -- the archive's silence is not evidence of a
group stage.

**A dead-rubber flag, only where a table is reconstructible as-of.** Two shapes qualify:

* a **bilateral series**, where the "table" is the scoreline -- as of a match, one side has
  won more than half the series' fixtures, so the series is decided;
* a **league with playoffs**, where the number of qualifying places is observable (the
  clubs that played the edition's knockout matches) and the group table is reconstructible
  from the edition's earlier results. A side is dead when it is *mathematically* out of the
  top k or *mathematically* in it, on the conservative arithmetic below, and the match is a
  dead rubber when either side is.

Everything else -- a round robin with no knockout stage, a competition whose playoff round
the archive does not label -- is **not flagged**, and the coverage report says so. A flag
that guessed would be worse than no flag: the point of the flag is to clean a measurement.

**H-21.** The stage label uses only what is knowable before the first ball: the fixture's
own event fields, and the number of clubs in its edition. The dead-rubber flag additionally
uses the edition's *fixture list* (which clubs still have matches to play, and how many)
and the *number of qualifying places* -- both published before a season starts, and both
read here from the archive as a proxy for that publication. **Results are read strictly
as-of**: only matches on dates strictly before the fixture's own date count toward a table,
so a match never learns the result of another match played the same day. This is why the
dead-rubber flag is a *measurement* input (X-3's E5 hygiene) and never a model feature,
while the knockout flag -- knowable from the fixture alone -- is the one X-3 offers the
display model.
"""

from __future__ import annotations

import logging
import re
from collections import defaultdict
from dataclasses import dataclass
from datetime import date, timedelta
from typing import Dict, Iterable, List, Optional, Protocol, Sequence, Tuple

logger = logging.getLogger(__name__)

#: The stage labels. An empty label is "the archive does not say", never "group".
STAGE_GROUP = "group"
STAGE_KNOCKOUT = "knockout"
STAGE_FINAL = "final"
STAGE_BILATERAL = "bilateral"
STAGE_UNLABELLED = ""
STAGE_LABELS: Tuple[str, ...] = (STAGE_GROUP, STAGE_KNOCKOUT, STAGE_FINAL, STAGE_BILATERAL)

#: Two matches of one competition more than this far apart belong to different editions.
#: Competitions run annually and a single edition finishes inside two months; the gap
#: between one season's last match and the next season's first is never this short.
EDITION_GAP_DAYS = 60

#: Points for a win and for a result the archive records as no result, a tie or a draw --
#: the near-universal limited-overs league table. Competitions with bonus points exist;
#: none of the arithmetic below is sensitive to them, because it only ever asks whether a
#: gap is unbridgeable, and a bonus point makes a gap easier to bridge, not harder.
POINTS_WIN = 2
POINTS_NO_RESULT = 1

# The stage vocabulary, as three ordered tests over the archive's own spellings. Order is
# the whole design: "Super League Final" is a final, not a super league, and "5th Place
# Play-Off Semi Final" is a knockout, not a final.
_GROUP_WORDS = (
    "group",
    "super league",
    "super six",
    "super eight",
    "super four",
    "super three",
    "super 10",
    "first round",
    "second round",
    "pool",
    "plate",
    "elite",
    "zone",
    "league stage",
)
_KNOCKOUT_WORDS = (
    "semi",
    "quarter",
    "eliminator",
    "elimination",
    "qualifier",
    "qualifying play",
    "play-off",
    "play off",
    "playoff",
    "challenger",
    "preliminary",
    "knockout",
    "place",  # "3rd Place Play-Off", "7th place playoff": a knockout, never the final
)
_FINAL_WORDS = ("final",)


def _normalise(stage: str) -> str:
    """Lower case with runs of whitespace collapsed, so 'Semi-Final' and 'Semi Final'
    are one spelling and 'Play-Off' matches the hyphenated and spaced forms alike."""
    return re.sub(r"\s+", " ", stage.strip().lower())


def stage_label_from_event_stage(stage: str) -> str:
    """The label the archive's own ``event.stage`` states, or unlabelled.

    Only a stage the vocabulary recognises produces a label: 'T20' and 'ODI' appear in the
    field on four matches, and they say nothing about a round.
    """
    text = _normalise(stage)
    if not text:
        return STAGE_UNLABELLED
    knockout = any(word in text for word in _KNOCKOUT_WORDS)
    final = any(word in text for word in _FINAL_WORDS)
    if any(word in text for word in _GROUP_WORDS) and not (knockout or final):
        return STAGE_GROUP
    if knockout:
        return STAGE_KNOCKOUT
    if final:
        return STAGE_FINAL
    return STAGE_UNLABELLED


@dataclass(frozen=True)
class MatchStakes:
    """What one match was worth, as far as the archive can say."""

    stage_label: str = STAGE_UNLABELLED
    dead_rubber: bool = False
    #: Whether a table was reconstructible for this match at all. ``dead_rubber`` False
    #: with this False means "not known", which is a different fact from "known live".
    dead_rubber_known: bool = False

    @property
    def stage_known(self) -> bool:
        return self.stage_label != STAGE_UNLABELLED

    @property
    def is_knockout(self) -> bool:
        return self.stage_label in (STAGE_KNOCKOUT, STAGE_FINAL)


UNLABELLED = MatchStakes()


class MatchHeader(Protocol):
    """What the derivation reads from a match. Both rating-pass sources satisfy it, and
    so does anything else that can name a fixture -- which is what keeps this module free
    of a dependency on either source."""

    match_id: str
    match_date: date
    format_code: str
    gender: str
    team1: str
    team2: str
    competition: str
    match_number: Optional[int]
    event_stage: str
    event_group: str
    winner: Optional[str]


@dataclass(frozen=True)
class Header:
    """A concrete ``MatchHeader`` for a caller that has the fixture's fields but not a
    ``MatchRecord`` -- the Postgres source, which knows every match before it has read a
    single delivery, and the tests."""

    match_id: str
    match_date: date
    format_code: str
    gender: str
    team1: str
    team2: str
    competition: str
    match_number: Optional[int]
    event_stage: str
    event_group: str
    winner: Optional[str]


@dataclass(frozen=True)
class Edition:
    """One running of one competition: its matches in date order, and its clubs."""

    key: Tuple[str, str, str]  # competition, format, gender
    headers: Tuple[MatchHeader, ...]

    @property
    def clubs(self) -> frozenset:
        return frozenset(h.team1 for h in self.headers) | frozenset(h.team2 for h in self.headers)


def editions(headers: Iterable[MatchHeader]) -> List[Edition]:
    """Split matches into editions: one competition, one format, one gender, and no gap
    longer than ``EDITION_GAP_DAYS`` between consecutive fixtures.

    Editions matter because a table is a fact about one running of a competition. The
    Indian Premier League is one event name and eighteen tables; a series scoreline resets
    every tour. Matches with no event name form no edition -- they are unlabelled.
    """
    by_key: Dict[Tuple[str, str, str], List[MatchHeader]] = defaultdict(list)
    for header in headers:
        if header.competition:
            by_key[(header.competition, header.format_code, header.gender)].append(header)
    out: List[Edition] = []
    gap = timedelta(days=EDITION_GAP_DAYS)
    for key, group in by_key.items():
        group.sort(key=lambda h: (h.match_date, str(h.match_id)))
        run: List[MatchHeader] = [group[0]]
        for header in group[1:]:
            if header.match_date - run[-1].match_date > gap:
                out.append(Edition(key=key, headers=tuple(run)))
                run = []
            run.append(header)
        out.append(Edition(key=key, headers=tuple(run)))
    return out


def _stage_labels(edition: Edition) -> Dict[str, str]:
    """Every match's stage label: the archive's own stage where it states one, and the
    edition's shape where it does not."""
    clubs = len(edition.clubs)
    labels: Dict[str, str] = {}
    for header in edition.headers:
        label = stage_label_from_event_stage(header.event_stage)
        if not label:
            if clubs == 2:
                label = STAGE_BILATERAL
            elif clubs >= 3 and (header.match_number is not None or header.event_group):
                label = STAGE_GROUP
        labels[str(header.match_id)] = label
    return labels


def _bilateral_dead_rubbers(edition: Edition) -> Dict[str, bool]:
    """A two-club series: dead from the fixture at which one side has won more than half
    of the series' matches, counted from strictly earlier dates only."""
    scheduled = len(edition.headers)
    wins: Dict[str, int] = defaultdict(int)
    out: Dict[str, bool] = {}
    # Matches are in date order; results are applied only once the date has advanced, so a
    # fixture never sees another played on its own day (H-21).
    pending: List[MatchHeader] = []
    current: Optional[date] = None
    for header in edition.headers:
        if current is not None and header.match_date != current:
            for done in pending:
                if done.winner:
                    wins[done.winner] += 1
            pending.clear()
        current = header.match_date
        decided = max(wins.values()) if wins else 0
        out[str(header.match_id)] = 2 * decided > scheduled
        pending.append(header)
    return out


def _league_dead_rubbers(edition: Edition, labels: Dict[str, str]) -> Optional[Dict[str, bool]]:
    """A league whose top ``k`` go through: dead where neither side's group result can
    still change whether it is one of them.

    ``k`` is the number of clubs the edition's knockout matches were played by -- the
    published playoff format, read from the archive. A side is **out** when at least ``k``
    rivals already hold more points than it could reach by winning all its remaining
    fixtures, and **through** when at most ``k - 1`` rivals could reach its current points
    at all. Both tests use rivals' *current* points against the side's *maximum*, which is
    the conservative direction: neither can fire on a table that is still open, and both
    ignore net run rate, which can only make a place harder to take, never easier to hold.
    """
    knockout_clubs = {
        club
        for header in edition.headers
        if labels[str(header.match_id)] in (STAGE_KNOCKOUT, STAGE_FINAL)
        for club in (header.team1, header.team2)
    }
    k = len(knockout_clubs)
    group_matches = [h for h in edition.headers if labels[str(h.match_id)] == STAGE_GROUP]
    if k < 2 or not group_matches:
        return None

    remaining: Dict[str, int] = defaultdict(int)
    for header in group_matches:
        remaining[header.team1] += 1
        remaining[header.team2] += 1
    points: Dict[str, int] = {club: 0 for club in remaining}
    # A cut that takes everyone decides nothing, so the table cannot say a match is dead.
    # (It happens: a four-club edition whose knockout round is two semi-finals.)
    if k >= len(points):
        return None

    out: Dict[str, bool] = {}
    pending: List[MatchHeader] = []
    current: Optional[date] = None
    for header in group_matches:
        # Day close, as the rating pass folds results in: a fixture reads only dates
        # strictly before its own, so a match played the same day is still to come both
        # in the table and in the fixture list (H-21).
        if current is not None and header.match_date != current:
            for done in pending:
                _apply_result(done, points)
                remaining[done.team1] -= 1
                remaining[done.team2] -= 1
            pending.clear()
        current = header.match_date
        out[str(header.match_id)] = any(_is_dead(club, points, remaining, k) for club in (header.team1, header.team2))
        pending.append(header)
    # A knockout match is never a dead rubber; saying so is part of the flag's coverage.
    for header in edition.headers:
        out.setdefault(str(header.match_id), False)
    return out


def _apply_result(header: MatchHeader, points: Dict[str, int]) -> None:
    if header.winner:
        points[header.winner] = points.get(header.winner, 0) + POINTS_WIN
        return
    for club in (header.team1, header.team2):
        points[club] = points.get(club, 0) + POINTS_NO_RESULT


def _is_dead(club: str, points: Dict[str, int], remaining: Dict[str, int], k: int) -> bool:
    """Whether this club's remaining group matches can still change its qualification."""
    own_points = points.get(club, 0)
    own_maximum = own_points + POINTS_WIN * remaining.get(club, 0)
    rivals = [other for other in points if other != club]
    certainly_above = sum(1 for other in rivals if points[other] > own_maximum)
    possibly_above = sum(1 for other in rivals if points[other] + POINTS_WIN * remaining.get(other, 0) >= own_points)
    return certainly_above >= k or possibly_above <= k - 1


def derive(headers: Sequence[MatchHeader]) -> Dict[str, MatchStakes]:
    """Stakes for every match, keyed by match id. Both rating-pass sources call this with
    the matches they are about to yield, so the two produce the same labels or the parity
    check says which field they disagree about."""
    out: Dict[str, MatchStakes] = {}
    for edition in editions(headers):
        labels = _stage_labels(edition)
        if len(edition.clubs) == 2:
            dead = _bilateral_dead_rubbers(edition)
        else:
            dead = _league_dead_rubbers(edition, labels)
        for header in edition.headers:
            key = str(header.match_id)
            out[key] = MatchStakes(
                stage_label=labels[key],
                dead_rubber=bool(dead.get(key, False)) if dead is not None else False,
                dead_rubber_known=dead is not None and key in dead,
            )
    logger.info(
        "stakes: %d matches labelled of %d, %d knockout, %d dead rubbers of %d with a reconstructible table",
        sum(1 for s in out.values() if s.stage_known),
        len(headers),
        sum(1 for s in out.values() if s.is_knockout),
        sum(1 for s in out.values() if s.dead_rubber),
        sum(1 for s in out.values() if s.dead_rubber_known),
    )
    return out


def vocabulary(headers: Sequence[MatchHeader]) -> Dict[str, Dict[str, int]]:
    """Every ``event.stage`` spelling the archive uses and what it was labelled, so the
    vocabulary can be read rather than trusted. A spelling that lands on ``unlabelled`` is
    either meaningless ('T20') or a gap in the table, and this is where it shows."""
    counts: Dict[str, Dict[str, int]] = {}
    for header in headers:
        text = header.event_stage.strip()
        if not text:
            continue
        entry = counts.setdefault(text, {"matches": 0, "label": stage_label_from_event_stage(text)})
        entry["matches"] += 1
    return dict(sorted(counts.items(), key=lambda kv: (-kv[1]["matches"], kv[0])))


def coverage(headers: Sequence[MatchHeader], stakes: Dict[str, MatchStakes]) -> Dict[str, Dict[str, int]]:
    """Label coverage per format and gender, and overall: how many matches each label
    reaches, and how many have a reconstructible table. The honest version of "we derived
    match stakes" is this table, printed whatever it says."""
    buckets: Dict[str, Dict[str, int]] = defaultdict(lambda: defaultdict(int))
    for header in headers:
        entry = stakes.get(str(header.match_id), UNLABELLED)
        for scope in ("all", header.format_code, f"{header.format_code}/{header.gender}"):
            bucket = buckets[scope]
            bucket["matches"] += 1
            bucket["stage_known"] += int(entry.stage_known)
            bucket[f"label_{entry.stage_label or 'unlabelled'}"] += int(True)
            bucket["knockout"] += int(entry.is_knockout)
            bucket["dead_rubber_known"] += int(entry.dead_rubber_known)
            bucket["dead_rubber"] += int(entry.dead_rubber)
    return {scope: dict(counts) for scope, counts in sorted(buckets.items())}
