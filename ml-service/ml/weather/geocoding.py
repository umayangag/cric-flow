"""Venue -> coordinates, curated in a CSV that is written once and then only appended to.

ERA5 is a 0.25-degree reanalysis (~28 km cells), so a ground's city places it as well as
the ground itself would; the query is the city Cricsheet names beside the venue, or the
venue name's own trailing part. The Open-Meteo geocoding API returns every place of that
name, and the archive's country votes (``venues.VenueFacts``) pick among them -- Hamilton
is in New Zealand when New Zealand's sides play there. A venue that nothing places is
recorded as ``unmappable`` with the reason, never guessed at.

The CSV is the curated table: rows already present are never rewritten by a run, so a
hand correction survives, and a later run asks only about venues the file has no row for.
"""

from __future__ import annotations

import csv
import logging
import os
import time
from dataclasses import asdict, dataclass, fields
from typing import Dict, List, Mapping, Optional, Protocol, Sequence

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


def choose(candidates: Sequence[Candidate], votes: Sequence[str]) -> Optional[Candidate]:
    """The candidate in the best-voted country, the most populous of those; with no vote
    to go on, the most populous place of that name. None when there is nothing to choose."""
    if not candidates:
        return None
    rank = {country: i for i, country in enumerate(votes)}
    return min(candidates, key=lambda c: (rank.get(c.country_code, len(rank)), -c.population))


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


def locate(facts: VenueFacts, client: GeocodingClient) -> VenueLocation:
    """One venue's row: the first query with a candidate decides."""
    votes = facts.countries()
    tried: List[str] = []
    for query in _queries(facts):
        tried.append(query)
        chosen = choose(client.search(query), votes)
        if chosen is None:
            continue
        note = "" if not votes or chosen.country_code in votes else "country not among the archive's votes"
        if not votes:
            note = "no country vote; most populous place of that name"
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
            countries_voted=" ".join(votes),
            note=note,
        )
    return VenueLocation(
        venue=facts.venue,
        venue_key=facts.key,
        status=STATUS_UNMAPPABLE,
        query=" | ".join(tried),
        countries_voted=" ".join(votes),
        note="no place found for any query",
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
