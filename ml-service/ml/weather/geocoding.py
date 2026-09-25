"""Venue -> coordinates, curated in a CSV that is written once and then only appended to.

ERA5 is a 0.25-degree reanalysis (~28 km cells), so a ground's city places it as well as
the ground itself would; the query is the city Cricsheet names beside the venue, or the
venue name's own trailing part. The Open-Meteo geocoding API returns every place of that
name, and the archive's country votes (``venues.VenueFacts``) pick among them -- Hamilton
is in New Zealand when New Zealand's sides play there. A venue that nothing places is
recorded as ``unmappable`` with the reason, never guessed at.

The votes are evidence, and the chooser reads them as such. A candidate in the top-voted
country is supported. A candidate in a country the archive voted for less is a homonym
until shown otherwise -- Bangalore Town, Sindh is not where India plays -- so it is refused
whenever the top vote's lead is one chance would not produce, and kept, with the note
saying so, when the lead is not evidence either way (an associate ground visited by
everyone, a neutral venue, a one-series ground). A candidate in a country nobody voted
for is read the same way -- silence against a weak top vote is not a contradiction and the
candidate is kept with its note; silence against a strong one (five votes or more) is.

The CSV is the curated table: rows already present are never rewritten by a run, so a
hand correction survives, and a later run asks only about venues the file has no row for.
"""

from __future__ import annotations

import csv
import logging
import math
import os
import time
from dataclasses import asdict, dataclass, fields
from typing import Dict, List, Mapping, Optional, Protocol, Sequence, Tuple

import httpx

from ml.weather.venues import VenueFacts

logger = logging.getLogger(__name__)

GEOCODING_URL = "https://geocoding-api.open-meteo.com/v1/search"
STATUS_MAPPED = "mapped"
STATUS_UNMAPPABLE = "unmappable"
SOURCE = "open-meteo-geocoding"
#: The results asked for per query: enough to hold every same-named place that matters.
CANDIDATES_PER_QUERY = 10
#: Seconds between geocoding calls; the service is free and asks for restraint, not speed.
PAUSE_SECONDS = 0.2
#: A candidate outside the archive's top-voted country is a conflict when the top country's
#: lead over the candidate's country is one chance would not produce: a one-sided sign test
#: on the two countries' votes, at this level. Over the whole curated table the test refuses
#: every misplacement the archive had eleven or more votes against (Chinnaswamy 102 to 2,
#: Mirpur 105 to 33, Providence 70 to 50) and cannot refuse a 2-to-1 one, which no reading
#: of the votes could; those are placed by hand.
CONFLICT_P_VALUE = 0.05

#: How the archive's votes support a candidate's country, best first.
SUPPORT_TOP = "top"
SUPPORT_MINORITY = "minority"
SUPPORT_UNVOTED = "unvoted"
SUPPORT_CONFLICT = "conflict"
_SUPPORT_RANK = {SUPPORT_TOP: 0, SUPPORT_MINORITY: 1, SUPPORT_UNVOTED: 2}

NOTE_NO_VOTES = "no country vote; most populous place of that name"
NOTE_UNVOTED = "country not among the archive's votes"
NOTE_NO_PLACE = "no place found for any query"
#: The stem of ``support_note``'s minority-vote sentence, before the two countries and
#: their counts -- shared with ``curation_summary`` (DATA-09) so the two read one string.
NOTE_MINORITY_PREFIX = "country not the archive's top vote ("
#: A hand-placed row's note starts with this, set directly on the row by a person rather
#: than computed by ``support_note`` -- the CSV's own record of what asking the archive's
#: votes could not settle.
HAND_CURATED_PREFIX = "hand-curated"


@dataclass(frozen=True)
class Candidate:
    name: str
    latitude: float
    longitude: float
    country_code: str
    country: str
    timezone: str
    population: int
    admin1: str = ""


@dataclass
class VenueLocation:
    """One row of the curated table."""

    venue: str
    venue_key: str
    status: str
    query: str = ""
    place: str = ""
    admin1: str = ""
    country_code: str = ""
    country: str = ""
    latitude: Optional[float] = None
    longitude: Optional[float] = None
    timezone: str = ""
    countries_voted: str = ""
    source: str = SOURCE
    note: str = ""

    @property
    def mapped(self) -> bool:
        return self.status == STATUS_MAPPED and self.latitude is not None and self.longitude is not None


CSV_COLUMNS = tuple(f.name for f in fields(VenueLocation))


class GeocodingClient(Protocol):
    def search(self, name: str) -> List[Candidate]: ...


class OpenMeteoGeocoding:
    """The free geocoding endpoint. No key; the same non-commercial terms as the archive."""

    def __init__(self, http: Optional[httpx.Client] = None, pause_seconds: float = PAUSE_SECONDS) -> None:
        self._http = http or httpx.Client(timeout=30.0)
        self._pause = pause_seconds

    def search(self, name: str) -> List[Candidate]:
        response = self._http.get(
            GEOCODING_URL, params={"name": name, "count": CANDIDATES_PER_QUERY, "language": "en", "format": "json"}
        )
        response.raise_for_status()
        time.sleep(self._pause)
        return [
            Candidate(
                name=item.get("name", ""),
                latitude=float(item["latitude"]),
                longitude=float(item["longitude"]),
                country_code=item.get("country_code", ""),
                country=item.get("country", ""),
                timezone=item.get("timezone", ""),
                population=int(item.get("population") or 0),
                admin1=item.get("admin1", ""),
            )
            for item in response.json().get("results", [])
            if "latitude" in item and "longitude" in item
        ]


# --- The votes as evidence ----------------------------------------------------------------


def format_votes(votes: Mapping[str, int]) -> str:
    """The ``countries_voted`` column: ``BD:105 ZW:37 IN:33``, most votes first."""
    return " ".join(f"{code}:{n}" for code, n in sorted(votes.items(), key=lambda kv: (-kv[1], kv[0])))


def parse_votes(text: str) -> Dict[str, int]:
    """``format_votes`` read back; a column written before the counts were recorded raises,
    because a row whose votes cannot be audited is not a curated row."""
    out: Dict[str, int] = {}
    for token in text.split():
        code, _, count = token.partition(":")
        if not count.isdigit():
            raise ValueError(f"countries_voted carries no count for {code!r}: {text!r}")
        out[code] = int(count)
    return out


def top_vote(votes: Mapping[str, int]) -> Tuple[str, int]:
    """The best-voted country and its votes; ties broken by code so the answer is stable."""
    return min(votes.items(), key=lambda kv: (-kv[1], kv[0]))


def lead_beyond_chance(top_votes: int, candidate_votes: int) -> bool:
    """Whether ``top_votes`` against ``candidate_votes`` is a lead a fair coin would produce
    with probability under ``CONFLICT_P_VALUE``: the one-sided sign test."""
    tosses = top_votes + candidate_votes
    at_most = sum(math.comb(tosses, k) for k in range(candidate_votes + 1))
    return at_most / 2**tosses < CONFLICT_P_VALUE


def support(country_code: str, votes: Mapping[str, int]) -> str:
    """What the archive's votes say about a candidate in ``country_code``: ``top`` when no
    country out-votes it, ``conflict`` when the top vote's lead over it is beyond chance
    (zero votes included), else ``unvoted`` when nobody voted for it and ``minority``
    when some did."""
    if not votes:
        return SUPPORT_TOP
    mine = votes.get(country_code, 0)
    _, best = top_vote(votes)
    if mine == best:
        return SUPPORT_TOP
    if lead_beyond_chance(best, mine):
        return SUPPORT_CONFLICT
    return SUPPORT_UNVOTED if mine == 0 else SUPPORT_MINORITY


def support_note(country_code: str, votes: Mapping[str, int]) -> str:
    """The ``note`` a mapped row carries for its support: empty only for the top vote, so
    every weaker placement is visible on the row itself."""
    if not votes:
        return NOTE_NO_VOTES
    kind = support(country_code, votes)
    if kind == SUPPORT_TOP:
        return ""
    if kind == SUPPORT_UNVOTED:
        return NOTE_UNVOTED
    code, best = top_vote(votes)
    return f"{NOTE_MINORITY_PREFIX}{code} {best} to {votes[country_code]})"


def choose(candidates: Sequence[Candidate], votes: Mapping[str, int]) -> Optional[Candidate]:
    """The most populous candidate in the best-supported country -- the top vote, then a
    country the top vote does not out-vote beyond chance, then a country nobody voted for.
    A candidate the top vote conflicts with is never chosen, so a query whose every place
    conflicts yields nothing and the next query is asked. None when nothing remains."""
    supported = [c for c in candidates if support(c.country_code, votes) != SUPPORT_CONFLICT]
    if not supported:
        return None
    return min(supported, key=lambda c: (_SUPPORT_RANK[support(c.country_code, votes)], -c.population))


def _queries(facts: VenueFacts) -> List[str]:
    """What to ask, in order: the city, then the venue's own name parts."""
    out = []
    hint = facts.city_hint()
    if hint:
        out.append(hint)
    for part in reversed([p.strip() for p in facts.venue.split(",")]):
        if part and part not in out:
            out.append(part)
    return out


def _conflict_note(conflicts: Sequence[Candidate], votes: Mapping[str, int]) -> str:
    code, best = top_vote(votes)
    places = ", ".join(f"{c.name} ({c.country_code})" for c in conflicts)
    return f"every candidate conflicts with the archive's top vote ({code} {best}): {places}"


def locate(facts: VenueFacts, client: GeocodingClient) -> VenueLocation:
    """One venue's row: the first query with a supported candidate decides. When every query
    found only conflicting places the row is unmappable and its note names them."""
    votes: Dict[str, int] = dict(facts.country_votes)
    tried: List[str] = []
    conflicts: List[Candidate] = []
    for query in _queries(facts):
        tried.append(query)
        candidates = client.search(query)
        chosen = choose(candidates, votes)
        if chosen is None:
            conflicts.extend(c for c in candidates if support(c.country_code, votes) == SUPPORT_CONFLICT)
            continue
        return VenueLocation(
            venue=facts.venue,
            venue_key=facts.key,
            status=STATUS_MAPPED,
            query=query,
            place=chosen.name,
            admin1=chosen.admin1,
            country_code=chosen.country_code,
            country=chosen.country,
            latitude=chosen.latitude,
            longitude=chosen.longitude,
            timezone=chosen.timezone,
            countries_voted=format_votes(votes),
            note=support_note(chosen.country_code, votes),
        )
    return VenueLocation(
        venue=facts.venue,
        venue_key=facts.key,
        status=STATUS_UNMAPPABLE,
        query=" | ".join(tried),
        countries_voted=format_votes(votes),
        note=_conflict_note(conflicts, votes) if conflicts else NOTE_NO_PLACE,
    )


def read_locations(path: str) -> Dict[str, VenueLocation]:
    """The curated table keyed by venue key; a missing file is an empty table."""
    if not os.path.exists(path):
        return {}
    out: Dict[str, VenueLocation] = {}
    with open(path, newline="") as fh:
        for row in csv.DictReader(fh):
            row = {k: v for k, v in row.items() if k in CSV_COLUMNS}
            for column in ("latitude", "longitude"):
                row[column] = float(row[column]) if row.get(column) else None
            location = VenueLocation(**row)
            out[location.venue_key] = location
    return out


@dataclass(frozen=True)
class CurationSummary:
    """How the curated table's mapped rows break down by what placed them (DATA-09): the
    counts a doc describing the table should quote, derived from the table itself rather
    than typed by hand and left to drift the next time a row is hand-placed or re-voted."""

    total: int
    top_vote: int
    minority_vote: int
    unvoted: int
    no_votes: int
    hand_curated: int


def curation_summary(locations: Mapping[str, VenueLocation]) -> CurationSummary:
    """Classify every mapped row by its ``note`` (DATA-09): the archive's top-voted country
    (an empty note), a country the top vote does not out-vote beyond chance, a country
    nobody voted for, no vote at all, or hand-placed. A row whose note matches none of
    these raises, so a new note shape is caught here rather than silently mis-counted."""
    counts = {"top_vote": 0, "minority_vote": 0, "unvoted": 0, "no_votes": 0, "hand_curated": 0}
    for location in locations.values():
        if location.status != STATUS_MAPPED:
            continue
        note = location.note
        if note.startswith(HAND_CURATED_PREFIX):
            counts["hand_curated"] += 1
        elif note == "":
            counts["top_vote"] += 1
        elif note == NOTE_UNVOTED:
            counts["unvoted"] += 1
        elif note == NOTE_NO_VOTES:
            counts["no_votes"] += 1
        elif note.startswith(NOTE_MINORITY_PREFIX):
            counts["minority_vote"] += 1
        else:
            raise ValueError(f"{location.venue_key}: note matches no known placement kind: {note!r}")
    return CurationSummary(total=sum(counts.values()), **counts)


def write_locations(path: str, locations: Mapping[str, VenueLocation]) -> None:
    """The whole table, sorted by venue so a diff shows only what changed."""
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    with open(path, "w", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=CSV_COLUMNS)
        writer.writeheader()
        for location in sorted(locations.values(), key=lambda loc: loc.venue):
            row = asdict(location)
            for column in ("latitude", "longitude"):
                row[column] = "" if row[column] is None else f"{row[column]:.4f}"
            writer.writerow(row)


def geocode_missing(
    venues: Mapping[str, VenueFacts], locations: Dict[str, VenueLocation], client: GeocodingClient
) -> int:
    """Ask about every venue the table has no row for, adding rows in place. Returns how
    many were asked about; the rows already present are not touched."""
    asked = 0
    for key in sorted(venues):
        if key in locations:
            continue
        locations[key] = locate(venues[key], client)
        asked += 1
        if asked % 50 == 0:
            logger.info("geocoded %d venues", asked)
    return asked
