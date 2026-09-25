"""The archive's venues and fixtures, as the weather acquisition needs them.

One pass over the Cricsheet JSON files yields, per match, the facts the session-window
inference reads (format, gender, competition, match number, the first day) and, per venue,
what helps place it: the cities Cricsheet names beside it (``info.city`` is present on 93 %
of files and never in the database, which stores the venue name alone), and the countries
the sides that played there come from. The venue key is the name normalised the identity
way -- case, accents and punctuation folded -- so two spellings of one ground share a row.
"""

from __future__ import annotations

import collections
import json
import logging
import os
import re
import unicodedata
from dataclasses import dataclass, field
from datetime import date
from typing import Dict, Iterable, List, Optional, Sequence, Tuple

logger = logging.getLogger(__name__)

# International sides -> the ISO alpha-2 country they play at home in. Only what the
# archive names; a side absent here casts no vote, which is a weaker placement, not a
# wrong one. The West Indies vote for every territory a West Indian ground could be in.
WEST_INDIES = ("JM", "TT", "BB", "GY", "LC", "VC", "AG", "GD", "KN", "DM")
TEAM_COUNTRIES: Dict[str, Tuple[str, ...]] = {
    "india": ("IN",),
    "australia": ("AU",),
    "england": ("GB",),
    "scotland": ("GB",),
    "wales": ("GB",),
    "new zealand": ("NZ",),
    "south africa": ("ZA",),
    "pakistan": ("PK",),
    "sri lanka": ("LK",),
    "bangladesh": ("BD",),
    "zimbabwe": ("ZW",),
    "afghanistan": ("AF",),
    "ireland": ("IE", "GB"),
    "netherlands": ("NL",),
    "nepal": ("NP",),
    "namibia": ("NA",),
    "oman": ("OM",),
    "united arab emirates": ("AE",),
    "united states of america": ("US",),
    "canada": ("CA",),
    "papua new guinea": ("PG",),
    "hong kong": ("HK",),
    "kenya": ("KE",),
    "uganda": ("UG",),
    "bermuda": ("BM",),
    "kuwait": ("KW",),
    "qatar": ("QA",),
    "bahrain": ("BH",),
    "saudi arabia": ("SA",),
    "malaysia": ("MY",),
    "singapore": ("SG",),
    "thailand": ("TH",),
    "japan": ("JP",),
    "germany": ("DE",),
    "italy": ("IT",),
    "denmark": ("DK",),
    "jersey": ("JE",),
    "guernsey": ("GG",),
    "austria": ("AT",),
    "belgium": ("BE",),
    "france": ("FR",),
    "spain": ("ES",),
    "portugal": ("PT",),
    "norway": ("NO",),
    "sweden": ("SE",),
    "finland": ("FI",),
    "estonia": ("EE",),
    "romania": ("RO",),
    "bulgaria": ("BG",),
    "greece": ("GR",),
    "cyprus": ("CY",),
    "malta": ("MT",),
    "isle of man": ("IM",),
    "gibraltar": ("GI",),
    "luxembourg": ("LU",),
    "czech republic": ("CZ",),
    "hungary": ("HU",),
    "serbia": ("RS",),
    "croatia": ("HR",),
    "slovenia": ("SI",),
    "turkey": ("TR",),
    "israel": ("IL",),
    "nigeria": ("NG",),
    "ghana": ("GH",),
    "sierra leone": ("SL",),
    "botswana": ("BW",),
    "mozambique": ("MZ",),
    "malawi": ("MW",),
    "rwanda": ("RW",),
    "tanzania": ("TZ",),
    "eswatini": ("SZ",),
    "lesotho": ("LS",),
    "seychelles": ("SC",),
    "cameroon": ("CM",),
    "gambia": ("GM",),
    "mali": ("ML",),
    "zambia": ("ZM",),
    "argentina": ("AR",),
    "brazil": ("BR",),
    "chile": ("CL",),
    "peru": ("PE",),
    "mexico": ("MX",),
    "cayman islands": ("KY",),
    "bahamas": ("BS",),
    "belize": ("BZ",),
    "panama": ("PA",),
    "costa rica": ("CR",),
    "fiji": ("FJ",),
    "vanuatu": ("VU",),
    "samoa": ("WS",),
    "cook islands": ("CK",),
    "philippines": ("PH",),
    "indonesia": ("ID",),
    "bhutan": ("BT",),
    "maldives": ("MV",),
    "myanmar": ("MM",),
    "china": ("CN",),
    "south korea": ("KR",),
    "mongolia": ("MN",),
    "cambodia": ("KH",),
    "vietnam": ("VN",),
    "iran": ("IR",),
    "tajikistan": ("TJ",),
    "uzbekistan": ("UZ",),
    "west indies": WEST_INDIES,
}

# Domestic competitions -> the country they are played in, by a phrase of Cricsheet's event
# name matched at word boundaries, first entry wins. A needle names a *competition* and
# never a country or an international side: "zimbabwe" used to be here and matched none of
# Zimbabwe's domestic cricket and all 443 of its tours ("Zimbabwe tour of Australia" voted
# ZW for Townsville), and "twenty20 cup" matched only the ACC's, never England's (DATA-03).
COMPETITION_COUNTRIES: Tuple[Tuple[str, Tuple[str, ...]], ...] = (
    ("indian premier league", ("IN",)),
    ("syed mushtaq ali", ("IN",)),
    ("vijay hazare", ("IN",)),
    ("ranji", ("IN",)),
    ("duleep", ("IN",)),
    ("women's premier league", ("IN",)),
    ("senior women's", ("IN",)),
    ("vitality", ("GB",)),
    ("natwest", ("GB",)),
    ("county championship", ("GB",)),
    ("royal london", ("GB",)),
    ("the hundred", ("GB",)),
    ("charlotte edwards", ("GB",)),
    ("rachael heyhoe", ("GB",)),
    ("bob willis", ("GB",)),
    ("ecb", ("GB",)),
    ("metro bank", ("GB",)),
    ("friends", ("GB",)),
    ("pro40", ("GB",)),
    ("cricket super league", ("GB",)),
    ("kia super league", ("GB",)),
    ("big bash", ("AU",)),
    ("sheffield shield", ("AU",)),
    ("marsh", ("AU",)),
    ("one-day cup (australia)", ("AU",)),
    ("matador", ("AU",)),
    ("ryobi", ("AU",)),
    ("jlt", ("AU",)),
    ("ford ranger", ("AU",)),
    ("women's national cricket league", ("AU",)),
    ("bangladesh premier league", ("BD",)),
    ("dhaka premier", ("BD",)),
    ("caribbean premier league", WEST_INDIES + ("US",)),
    ("pakistan super league", ("PK", "AE")),
    ("national t20", ("PK",)),
    ("quaid", ("PK",)),
    ("plunket", ("NZ",)),
    ("super smash", ("NZ",)),
    ("ford trophy", ("NZ",)),
    ("hallyburton", ("NZ",)),
    ("new zealand cricket", ("NZ",)),
    ("csa", ("ZA",)),
    ("ram slam", ("ZA",)),
    ("mzansi", ("ZA",)),
    ("sa20", ("ZA",)),
    ("momentum", ("ZA",)),
    ("sunfoil", ("ZA",)),
    ("4-day", ("ZA",)),
    ("lanka premier", ("LK",)),
    ("major league tournament", ("LK",)),
    ("major clubs", ("LK",)),
    ("international league t20", ("AE",)),
    ("abu dhabi t10", ("AE",)),
    ("afghanistan premier", ("AE",)),
    ("major league cricket", ("US",)),
    ("minor league", ("US",)),
    ("cricket ireland", ("IE", "GB")),
    ("nepal premier", ("NP",)),
    ("kwibuka", ("RW",)),
    ("kalahari", ("BW",)),
    ("global t20 canada", ("CA",)),
)

_NON_KEY = re.compile(r"[^a-z0-9]+")

#: Each needle as a whole-phrase pattern: "ecb" must not match inside "Recbury", and the
#: lookarounds do that without the trailing space the table used to carry (DATA-03).
_COMPETITION_PATTERNS: Tuple[Tuple["re.Pattern[str]", Tuple[str, ...]], ...] = tuple(
    (re.compile(rf"(?<!\w){re.escape(needle)}(?!\w)", re.IGNORECASE), countries)
    for needle, countries in COMPETITION_COUNTRIES
)


def venue_key(name: str) -> str:
    """The identity-way key: accents folded to ASCII, case folded, punctuation and runs of
    whitespace collapsed to one space. Two spellings of one ground share a key."""
    folded = unicodedata.normalize("NFKD", name).encode("ascii", "ignore").decode("ascii").casefold()
    return _NON_KEY.sub(" ", folded).strip()


@dataclass(frozen=True)
class Fixture:
    """One archived match, as the session-window inference and the join read it."""

    match_id: str
    venue: str
    date: date  # the first day; multi-day matches are keyed by their first day
    match_type: str  # Cricsheet's: T20, IT20, ODI, ODM, Test, MDM
    gender: str
    competition: str
    match_number: Optional[int]
    teams: Tuple[str, str]
    international: bool

    @property
    def venue_key(self) -> str:
        return venue_key(self.venue)


@dataclass
class VenueFacts:
    """What the archive says about where a venue is, before anything is asked outside."""

    venue: str
    cities: collections.Counter = field(default_factory=collections.Counter)
    country_votes: collections.Counter = field(default_factory=collections.Counter)
    dates: set = field(default_factory=set)

    @property
    def key(self) -> str:
        return venue_key(self.venue)

    def city_hint(self) -> str:
        """The city Cricsheet names most often beside this venue, else the last
        comma-separated part of the venue name ("Wankhede Stadium, Mumbai"), else nothing."""
        named = [(n, c) for c, n in self.cities.items() if c]
        if named:
            return max(named)[1]
        if "," in self.venue:
            return self.venue.rsplit(",", 1)[1].strip()
        return ""


def competition_countries(competition: str) -> Tuple[str, ...]:
    """The country a Cricsheet event name is played in, by the first needle that matches it
    as a whole phrase. Nothing when no competition is recognised: silence is a weaker
    placement than a vote, and a wrong vote is worse than either (DATA-03)."""
    for pattern, countries in _COMPETITION_PATTERNS:
        if pattern.search(competition):
            return countries
    return ()


def team_countries(team: str) -> Tuple[str, ...]:
    return TEAM_COUNTRIES.get(team.strip().casefold(), ())


def _fixture_from_info(match_id: str, info: dict) -> Fixture:
    event = info.get("event") or {}
    number = event.get("match_number")
    teams = tuple(info.get("teams") or ("", ""))
    return Fixture(
        match_id=match_id,
        venue=(info.get("venue") or "").strip(),
        date=date.fromisoformat(info["dates"][0]),
        match_type=info.get("match_type", ""),
        gender=info.get("gender", ""),
        competition=(event.get("name") or "").strip(),
        match_number=int(number) if isinstance(number, int) else None,
        teams=(teams[0], teams[1]) if len(teams) >= 2 else (teams[0] if teams else "", ""),
        international=info.get("team_type") == "international",
    )


def read_archive(cricsheet_dir: str) -> Tuple[List[Fixture], Dict[str, VenueFacts]]:
    """Every match's fixture and every venue's facts, from the JSON files' ``info`` blocks.
    The whole archive is read once; it is a few tens of seconds."""
    fixtures: List[Fixture] = []
    venues: Dict[str, VenueFacts] = {}
    names = sorted(n for n in os.listdir(cricsheet_dir) if n.endswith(".json"))
    for name in names:
        with open(os.path.join(cricsheet_dir, name)) as fh:
            info = json.load(fh).get("info") or {}
        if not info.get("dates"):
            continue
        fixture = _fixture_from_info(name[: -len(".json")], info)
        if not fixture.venue:
            continue
        fixtures.append(fixture)
        facts = venues.setdefault(fixture.venue_key, VenueFacts(venue=fixture.venue))
        facts.cities[(info.get("city") or "").strip()] += 1
        facts.dates.add(fixture.date)
        for country in _votes(fixture):
            facts.country_votes[country] += 1
    logger.info("archive: %d fixtures at %d venues from %d files", len(fixtures), len(venues), len(names))
    return fixtures, venues


def _votes(fixture: Fixture) -> Iterable[str]:
    if fixture.international:
        for team in fixture.teams:
            yield from team_countries(team)
    yield from competition_countries(fixture.competition)


def dates_by_venue(fixtures: Sequence[Fixture]) -> Dict[str, List[date]]:
    """The distinct first days played at each venue key, sorted: the cache's key space."""
    out: Dict[str, set] = collections.defaultdict(set)
    for fixture in fixtures:
        out[fixture.venue_key].add(fixture.date)
    return {key: sorted(days) for key, days in out.items()}
