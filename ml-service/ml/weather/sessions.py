"""The session window: when a match started, inferred, because Cricsheet does not say.

Every rule here is a norm -- the hour a competition or a country usually starts a match of
that format -- and every match records which rule placed it (``SessionWindow.rule``), so
the inference's error is inspectable rather than hidden in a feature. The pre-match window
the features read is the hours before ``start_hour``; the day/night flag says whether play
ran into the evening, which is a fact about the schedule, not the weather.

What the rules know: the format class; whether the sides are international; the venue's
country (from the geocoding table); the gender; and where a competition stages two matches
on one day, which of the two this was -- the lower match number is the afternoon game.
What they do not know: the actual start time, day/night Tests, a league's one-off day
game. The census ``backfill --report`` prints says how many matches each rule placed.
"""

from __future__ import annotations

import collections
from dataclasses import dataclass
from typing import Dict, Mapping, Sequence

from ml.weather.venues import Fixture

FIRST_CLASS_TYPES = ("Test", "MDM")
ONE_DAY_TYPES = ("ODI", "ODM")
T20_TYPES = ("T20", "IT20")

#: A T20 that starts at or after this hour finishes under lights.
T20_NIGHT_FROM_HOUR = 16
#: A one-day match that starts at or after this hour finishes under lights.
ONE_DAY_NIGHT_FROM_HOUR = 12

#: Men's ODIs at home, by country: the usual local start hour. Day/night in the
#: subcontinent, Australia and the Gulf; morning starts in England, Ireland, Zimbabwe and
#: the Caribbean.
ODI_START_BY_COUNTRY: Dict[str, int] = {
    "IN": 13,
    "LK": 14,
    "PK": 15,
    "AE": 15,
    "AU": 14,
    "BD": 14,
    "ZA": 13,
    "NZ": 14,
    "GB": 11,
    "IE": 10,
    "ZW": 9,
}
ODI_START_DEFAULT = 10
#: Domestic one-day cricket: morning starts; India's earlier still.
ONE_DAY_DOMESTIC_START_BY_COUNTRY: Dict[str, int] = {"IN": 9, "GB": 11}
ONE_DAY_DOMESTIC_START_DEFAULT = 10
FIRST_CLASS_START_BY_COUNTRY: Dict[str, int] = {"GB": 11}
FIRST_CLASS_START_DEFAULT = 10

#: Men's bilateral T20Is at home, by country. Associate members play by day (the default).
T20I_START_BY_COUNTRY: Dict[str, int] = {
    "IN": 19,
    "PK": 19,
    "LK": 19,
    "BD": 18,
    "AE": 18,
    "AU": 19,
    "ZA": 18,
    "NZ": 19,
    "GB": 18,
    "IE": 15,
    "ZW": 13,
}
T20I_START_DEFAULT = 10
#: Women's bilateral T20Is: afternoon, most often before a men's evening game.
T20I_WOMEN_START = 14
#: Where a day carries several matches at one venue -- associate events, mostly -- the
#: successive local start hours.
MULTI_HEADER_SLOTS = (10, 14, 18)


@dataclass(frozen=True)
class LeagueNorm:
    """A T20 league's usual start hours: alone on the day, and the two of a double-header
    (which the league stages at two grounds, so the rank is per competition and day)."""

    single: int
    double: tuple


#: Substring of Cricsheet's event name -> the league's norm.
T20_LEAGUE_NORMS: Sequence[tuple] = (
    ("indian premier league", LeagueNorm(19, (15, 19))),
    ("women's premier league", LeagueNorm(19, (15, 19))),
    ("women's big bash", LeagueNorm(14, (14, 18))),
    ("big bash", LeagueNorm(19, (13, 19))),
    ("pakistan super league", LeagueNorm(19, (14, 19))),
    ("caribbean premier league", LeagueNorm(19, (10, 19))),
    ("bangladesh premier league", LeagueNorm(18, (13, 18))),
    ("lanka premier league", LeagueNorm(19, (15, 19))),
    ("international league t20", LeagueNorm(18, (14, 18))),
    ("sa20", LeagueNorm(17, (13, 17))),
    ("major league cricket", LeagueNorm(19, (15, 19))),
    ("nepal premier league", LeagueNorm(17, (13, 17))),
    ("the hundred women", LeagueNorm(15, (15, 15))),
    ("the hundred men", LeagueNorm(18, (18, 18))),
    ("women's super smash", LeagueNorm(14, (14, 14))),
    ("charlotte edwards cup", LeagueNorm(14, (14, 14))),
    ("cricket super league", LeagueNorm(14, (14, 14))),
    ("vitality blast women", LeagueNorm(14, (14, 14))),
    ("syed mushtaq ali", LeagueNorm(9, (9, 13))),
)
#: Domestic T20 with many games a day across grounds: evening in the week, afternoon at the
#: weekend (England's Blast, New Zealand's Super Smash, South Africa's competitions).
T20_DOMESTIC_WEEKDAY_START = 18
T20_DOMESTIC_WEEKEND_START = 14
#: Countries whose domestic T20 is played under lights at all; elsewhere it is a day game.
T20_DOMESTIC_NIGHT_COUNTRIES = frozenset({"IN", "PK", "LK", "BD", "AE", "AU", "ZA", "GB", "NZ", "ZW"})
T20_DOMESTIC_DAY_START = 10

ICC_T20_NEEDLES = ("t20 world cup", "world twenty20", "world t20")
ICC_T20_MEN_SLOTS = (15, 19)
ICC_T20_WOMEN_SLOTS = (15, 19)


@dataclass(frozen=True)
class SessionWindow:
    start_hour: int
    night: bool
    rule: str


def _league_norm(competition: str):
    lowered = competition.casefold()
    for needle, norm in T20_LEAGUE_NORMS:
        if needle in lowered:
            return needle, norm
    return None, None


def _is_icc_t20_event(competition: str) -> bool:
    lowered = competition.casefold()
    return any(needle in lowered for needle in ICC_T20_NEEDLES) and "qualifier" not in lowered


def infer(
    fixture: Fixture, country_code: str, rank_on_day: int, matches_on_day: int, rank_at_venue: int
) -> SessionWindow:
    """The window for one fixture. ``rank_on_day`` / ``matches_on_day`` are the fixture's
    place among its competition's matches that day (by match number); ``rank_at_venue`` the
    same at this venue alone."""
    country = country_code or "??"
    women = fixture.gender == "female"
    if fixture.match_type in FIRST_CLASS_TYPES:
        return SessionWindow(FIRST_CLASS_START_BY_COUNTRY.get(country, FIRST_CLASS_START_DEFAULT), False, "first_class")
    if fixture.match_type in ONE_DAY_TYPES:
        if fixture.match_type == "ODI" and fixture.international and not women:
            hour = ODI_START_BY_COUNTRY.get(country, ODI_START_DEFAULT)
            return SessionWindow(hour, hour >= ONE_DAY_NIGHT_FROM_HOUR, f"odi_men:{country}")
        if fixture.match_type == "ODI" and fixture.international:
            return SessionWindow(ODI_START_DEFAULT, False, "odi_women")
        hour = ONE_DAY_DOMESTIC_START_BY_COUNTRY.get(country, ONE_DAY_DOMESTIC_START_DEFAULT)
        return SessionWindow(hour, hour >= ONE_DAY_NIGHT_FROM_HOUR, f"one_day_domestic:{country}")
    if fixture.match_type not in T20_TYPES:
        return SessionWindow(ONE_DAY_DOMESTIC_START_DEFAULT, False, f"unknown_type:{fixture.match_type}")
    return _infer_t20(fixture, country, women, rank_on_day, matches_on_day, rank_at_venue)


def _infer_t20(fixture, country, women, rank_on_day, matches_on_day, rank_at_venue) -> SessionWindow:
    needle, norm = _league_norm(fixture.competition)
    if norm is not None:
        if matches_on_day >= 2:
            hour = norm.double[min(rank_on_day, 1)]
            return SessionWindow(hour, hour >= T20_NIGHT_FROM_HOUR, f"t20_league:{needle}:double_{rank_on_day}")
        return SessionWindow(norm.single, norm.single >= T20_NIGHT_FROM_HOUR, f"t20_league:{needle}:single")
    if _is_icc_t20_event(fixture.competition):
        slots = ICC_T20_WOMEN_SLOTS if women else ICC_T20_MEN_SLOTS
        hour = slots[min(rank_on_day, 1)] if matches_on_day >= 2 else slots[-1]
        return SessionWindow(hour, hour >= T20_NIGHT_FROM_HOUR, f"t20i_icc:{'women' if women else 'men'}")
    if fixture.international:
        if women:
            return SessionWindow(T20I_WOMEN_START, False, "t20i_women")
        if country in T20I_START_BY_COUNTRY:
            hour = T20I_START_BY_COUNTRY[country]
            return SessionWindow(hour, hour >= T20_NIGHT_FROM_HOUR, f"t20i_men:{country}")
        hour = MULTI_HEADER_SLOTS[min(rank_at_venue, len(MULTI_HEADER_SLOTS) - 1)]
        return SessionWindow(hour, hour >= T20_NIGHT_FROM_HOUR, f"t20i_associate:slot_{rank_at_venue}")
    if women or country not in T20_DOMESTIC_NIGHT_COUNTRIES:
        hour = MULTI_HEADER_SLOTS[min(rank_at_venue, len(MULTI_HEADER_SLOTS) - 1)]
        return SessionWindow(hour, hour >= T20_NIGHT_FROM_HOUR, f"t20_domestic_day:slot_{rank_at_venue}")
    weekend = fixture.date.weekday() >= 5
    hour = T20_DOMESTIC_WEEKEND_START if weekend else T20_DOMESTIC_WEEKDAY_START
    return SessionWindow(hour, hour >= T20_NIGHT_FROM_HOUR, f"t20_domestic:{'weekend' if weekend else 'weekday'}")


def _sort_key(fixture: Fixture):
    return (fixture.match_number if fixture.match_number is not None else 10**9, fixture.match_id)


def assign(fixtures: Sequence[Fixture], country_by_venue_key: Mapping[str, str]) -> Dict[str, SessionWindow]:
    """A window per match id, with the double-header ranks computed over the whole set."""
    on_day: Dict[tuple, list] = collections.defaultdict(list)
    at_venue: Dict[tuple, list] = collections.defaultdict(list)
    for fixture in fixtures:
        on_day[(fixture.competition, fixture.date)].append(fixture)
        at_venue[(fixture.competition, fixture.date, fixture.venue_key)].append(fixture)
    rank_on_day: Dict[str, int] = {}
    rank_at_venue: Dict[str, int] = {}
    for group in on_day.values():
        for rank, fixture in enumerate(sorted(group, key=_sort_key)):
            rank_on_day[fixture.match_id] = rank
    for group in at_venue.values():
        for rank, fixture in enumerate(sorted(group, key=_sort_key)):
            rank_at_venue[fixture.match_id] = rank
    return {
        f.match_id: infer(
            f,
            country_by_venue_key.get(f.venue_key, ""),
            rank_on_day[f.match_id],
            len(on_day[(f.competition, f.date)]),
            rank_at_venue[f.match_id],
        )
        for f in fixtures
    }


def census(windows: Mapping[str, SessionWindow]) -> Dict[str, int]:
    """How many matches each rule placed, most first: the inspectable record."""
    counts = collections.Counter(w.rule for w in windows.values())
    return dict(counts.most_common())
