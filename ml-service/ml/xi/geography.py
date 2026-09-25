"""Where a match is played and where a team is from -- the inputs of the home flag (FEAT-05).

Two facts, from two very different places:

* **A ground's country** is a static fact, read once from the curated venue table
  (``reference-data/venue-geocoding.csv``, the same row X-2's weather is fetched at) and
  carried in the rating state like a date of birth. The database's ``venue.country`` is
  not read: it is NULL on every row of the archive, and a feature that reads a column the
  restore has not filled is a silent null.
* **A team's country** is not written down anywhere and is derived *as-of*: the strict
  mode of the countries the team has played in before today, kept by the rating pass in
  ``RatingState.team_countries`` and advanced at day close like every other accumulator.
  Nothing about a team is looked up in a whole-archive table, so the flag cannot see
  forward (H-21): a side reads as at home only once its past says it is. Pakistan's
  Emirates years read as home in the Emirates, which is what they were.

The West Indies are one side across ten territories, so the ten fold into one region;
every other ground is its ISO country. England, Scotland and Wales share ``GB`` and
Ireland's Belfast grounds are ``GB`` too -- that is the table's granularity, recorded
rather than patched.
"""

from __future__ import annotations

import logging
from typing import Dict, Mapping

from ml.weather.geocoding import DEFAULT_LOCATIONS_PATH, read_locations
from ml.weather.venues import WEST_INDIES, venue_key

logger = logging.getLogger(__name__)

#: Venue key (``ml.weather.venues.venue_key`` of the name; an id string is its own key)
#: -> home region: an ISO alpha-2 country, or ``WEST_INDIES_REGION``.
VenueCountries = Dict[str, str]
#: Per team key, how many past matches it has played in each region.
TeamCountries = Dict[str, Dict[str, int]]

WEST_INDIES_REGION = "WI"
#: What a team the state has never seen has played: nowhere.
NO_COUNTRIES: Mapping[str, int] = {}

_VENUES_SQL = "SELECT id, venue_name FROM venue"


def home_region(country_code: str) -> str:
    """The region a ground's country belongs to for the home flag."""
    return WEST_INDIES_REGION if country_code in WEST_INDIES else country_code


def load_venue_countries_csv(path: str = DEFAULT_LOCATIONS_PATH) -> VenueCountries:
    """Every mapped venue key -> home region, from the curated table. A venue the table
    has no country for is left out: it reads as no region, never as a guess."""
    out = {
        key: home_region(location.country_code)
        for key, location in read_locations(path).items()
        if location.mapped and location.country_code
    }
    logger.info("venue countries: %d venue keys from %s", len(out), path)
    return out


def load_venue_countries_postgres(connection, path: str = DEFAULT_LOCATIONS_PATH) -> VenueCountries:
    """The same table keyed the way the Postgres source keys venues -- the ``venue.id`` as
    a string -- by joining each row's name to the curated table through the venue key."""
    by_key = load_venue_countries_csv(path)
    with connection.cursor() as cur:
        cur.execute(_VENUES_SQL)
        rows = cur.fetchall()
    out: VenueCountries = {}
    for venue_id, venue_name in rows:
        region = by_key.get(venue_key(venue_name or ""), "")
        if region:
            out[str(venue_id)] = region
    logger.info("venue countries: %d of %d database venues placed in a region", len(out), len(rows))
    return out


def venue_region(venue_countries: Mapping[str, str], venue: str) -> str:
    """The region a match's venue is in, or "" when the venue is unnamed or unplaced.
    The lookup folds the name the identity way, which leaves an id string as it is, so
    one read serves both sources."""
    if not venue:
        return ""
    return venue_countries.get(venue_key(venue), "")


def modal_region(counts: Mapping[str, int]) -> str:
    """The region a team has played in most, as-of: "" while it has played nowhere or its
    most-played regions tie. No minimum beyond one match: a threshold would read a new
    club as away at its own ground for its first matches, which is the commoner error."""
    if not counts:
        return ""
    ranked = sorted(counts.items(), key=lambda item: (-item[1], item[0]))
    if len(ranked) > 1 and ranked[0][1] == ranked[1][1]:
        return ""
    return ranked[0][0]
