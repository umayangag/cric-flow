"""OPS-01: the committed reference data is under test.

``reference-data/venue-geocoding.csv`` and ``reference-data/era5-venue-days.jsonl`` are
data a person edits -- the curation flow is a hand edit to a row, and six audit fixes
(DATA-01 .. DATA-06) rewrote both files. Nothing parsed them in CI, so a broken row merged
green and only surfaced at the next backfill or restore.

What is checked here is what a hand edit or a re-fetch can plausibly get wrong and nothing
else would notice: a key that no longer folds from its own venue name, a duplicated key
that makes the coordinate join ambiguous, a point that is not in the country the row
claims, an hour array that is not a local day long, a cached day for a venue the table
does not hold, and prior-day rain windows that disagree where they overlap.

Two neighbours own the rest: ``test_weather.py`` already holds every mapped row to the
archive's own country votes (DATA-01), and ``go-app/internal/venues/reference_data_test.go``
holds the same ``venue_key`` column to Go's ``venues.NormalizeName`` (IMPORT-08), so the
two languages' folds cannot drift apart.
"""

from __future__ import annotations

import csv
import json
import os
from datetime import date, timedelta
from typing import Dict, List, Sequence
from zoneinfo import available_timezones

import pytest

from ml.weather import geocoding, venues

REPO_ROOT = os.path.join(os.path.dirname(__file__), "..", "..")
GEOCODING_CSV = os.path.join(REPO_ROOT, "reference-data", "venue-geocoding.csv")
ERA5_JSONL = os.path.join(REPO_ROOT, "reference-data", "era5-venue-days.jsonl")

HOURS_PER_DAY = 24
PRIOR_DAYS = 7

# Every country the curated table places a ground in, as a generous whole-country bounding
# box (min latitude, min longitude, max latitude, max longitude). The boxes are deliberately
# coarse -- they are a containment check, not a precision one. What they catch is the class
# of error a hand edit or a geocoder actually makes: a sign flipped, latitude and longitude
# transposed, a decimal point moved, or a ground placed in a country it is not in (DATA-02's
# country centroids were 10 km out, which no box would see; a Kensington Oval placed in
# Australia is what this sees). A box whose longitudes wrap the antimeridian is written with
# its minimum greater than its maximum, which is how Fiji is expressed.
COUNTRY_BOXES: Dict[str, tuple] = {
    "AE": (22.6, 51.5, 26.1, 56.4),
    "AG": (16.9, -62.0, 17.8, -61.6),
    "AR": (-55.1, -73.6, -21.8, -53.6),
    "AT": (46.3, 9.5, 49.1, 17.2),
    "AU": (-43.7, 112.9, -10.0, 153.7),
    "BB": (13.0, -59.7, 13.4, -59.4),
    "BD": (20.6, 88.0, 26.7, 92.7),
    "BE": (49.4, 2.5, 51.6, 6.4),
    "BG": (41.2, 22.3, 44.3, 28.7),
    "BM": (32.2, -65.0, 32.5, -64.6),
    "BR": (-33.8, -74.1, 5.3, -34.7),
    "BT": (26.6, 88.7, 28.4, 92.2),
    "BW": (-26.9, 19.9, -17.7, 29.4),
    "CA": (41.6, -141.1, 83.2, -52.6),
    "CN": (18.1, 73.5, 53.6, 134.8),
    "CO": (-4.3, -79.1, 12.6, -66.8),
    "CR": (8.0, -86.0, 11.3, -82.5),
    "CY": (34.5, 32.2, 35.8, 34.7),
    "CZ": (48.5, 12.0, 51.1, 18.9),
    "DE": (47.2, 5.8, 55.1, 15.1),
    "DK": (54.5, 8.0, 57.8, 15.3),
    "DM": (15.1, -61.6, 15.7, -61.2),
    "EE": (57.5, 21.7, 59.8, 28.3),
    "ES": (27.6, -18.3, 43.9, 4.4),
    "FI": (59.7, 19.0, 70.2, 31.7),
    "FJ": (-21.0, 176.8, -12.4, -178.0),
    "FR": (41.3, -5.2, 51.2, 9.6),
    "GB": (49.8, -8.7, 60.9, 1.9),
    "GD": (11.9, -61.9, 12.4, -61.3),
    "GG": (49.3, -2.8, 49.8, -2.1),
    "GH": (4.7, -3.3, 11.2, 1.3),
    "GI": (36.0, -5.4, 36.2, -5.3),
    "GR": (34.7, 19.3, 41.8, 29.8),
    "GY": (1.1, -61.4, 8.6, -56.4),
    "HK": (22.1, 113.8, 22.6, 114.5),
    "HR": (42.3, 13.4, 46.6, 19.5),
    "HU": (45.7, 16.1, 48.6, 22.9),
    "ID": (-11.1, 94.9, 6.1, 141.1),
    "IE": (51.4, -10.8, 55.5, -5.9),
    "IN": (6.7, 68.1, 35.6, 97.5),
    "IT": (35.4, 6.6, 47.2, 18.6),
    "JE": (49.1, -2.3, 49.3, -2.0),
    "JM": (17.6, -78.5, 18.6, -76.1),
    "JP": (24.0, 122.9, 45.6, 146.0),
    "KE": (-4.8, 33.9, 5.6, 42.0),
    "KH": (10.3, 102.3, 14.7, 107.7),
    "KN": (17.0, -63.0, 17.5, -62.5),
    "KR": (33.0, 124.5, 38.7, 131.1),
    "KW": (28.5, 46.5, 30.2, 48.5),
    "KY": (19.2, -81.5, 19.8, -79.7),
    "LC": (13.7, -61.1, 14.2, -60.8),
    "LK": (5.8, 79.5, 10.0, 82.0),
    "LU": (49.4, 5.7, 50.2, 6.6),
    "MT": (35.7, 14.1, 36.1, 14.6),
    "MW": (-17.2, 32.6, -9.3, 36.0),
    "MX": (14.4, -118.5, 32.8, -86.7),
    "MY": (0.8, 98.9, 7.4, 119.3),
    "NA": (-29.0, 11.7, -16.9, 25.3),
    "NC": (-22.8, 163.5, -19.5, 168.3),
    "NG": (4.2, 2.6, 13.9, 14.7),
    "NL": (50.7, 3.3, 53.7, 7.3),
    "NO": (57.9, 4.0, 71.3, 31.2),
    "NP": (26.3, 80.0, 30.5, 88.3),
    "NZ": (-47.4, 166.3, -34.3, 178.7),
    "OM": (16.6, 51.9, 26.5, 60.0),
    "PA": (7.1, -83.1, 9.7, -77.1),
    "PG": (-11.7, 140.8, -1.3, 156.0),
    "PH": (4.5, 116.9, 21.2, 126.7),
    "PK": (23.6, 60.8, 37.1, 77.9),
    "PT": (32.3, -31.4, 42.2, -6.1),
    "QA": (24.4, 50.7, 26.2, 51.7),
    "RO": (43.6, 20.2, 48.3, 29.8),
    "RS": (42.2, 18.8, 46.2, 23.1),
    "RW": (-2.9, 28.8, -1.0, 31.0),
    "SE": (55.3, 10.9, 69.1, 24.2),
    "SG": (1.1, 103.6, 1.5, 104.1),
    "SZ": (-27.4, 30.7, -25.7, 32.2),
    "TH": (5.6, 97.3, 20.5, 105.7),
    "TT": (10.0, -62.0, 11.4, -60.5),
    "TZ": (-11.8, 29.3, -0.9, 40.5),
    "UG": (-1.5, 29.5, 4.3, 35.1),
    "US": (24.4, -125.0, 49.4, -66.9),
    "VC": (12.5, -61.6, 13.4, -61.1),
    "VU": (-20.3, 166.5, -13.0, 170.3),
    "WS": (-14.1, -172.9, -13.4, -171.4),
    "ZA": (-35.0, 16.4, -22.1, 32.9),
    "ZW": (-22.5, 25.2, -15.6, 33.1),
}


def is_inside_country(country_code: str, latitude: float, longitude: float) -> bool:
    """Whether a point falls in the country's bounding box, antimeridian included."""
    min_latitude, min_longitude, max_latitude, max_longitude = COUNTRY_BOXES[country_code]
    if not min_latitude <= latitude <= max_latitude:
        return False
    if min_longitude <= max_longitude:
        return min_longitude <= longitude <= max_longitude
    return longitude >= min_longitude or longitude <= max_longitude


def geocoding_table_problems(path: str) -> List[str]:
    """Every way the curated venue table can be wrong, as one list of readable problems."""
    problems: List[str] = []
    with open(path, newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    if not rows:
        return [f"{path}: holds no rows"]
    if tuple(rows[0]) != geocoding.CSV_COLUMNS:
        return [f"{path}: columns are {tuple(rows[0])}, expected {geocoding.CSV_COLUMNS}"]

    zones = available_timezones()
    seen_keys: Dict[str, str] = {}
    seen_venues = set()
    for row in rows:
        venue, key = row["venue"], row["venue_key"]
        if key != venues.venue_key(venue):
            problems.append(f"{venue!r}: key {key!r} is not the fold of its own name ({venues.venue_key(venue)!r})")
        if key in seen_keys:
            problems.append(f"{venue!r}: shares key {key!r} with {seen_keys[key]!r}")
        seen_keys[key] = venue
        if venue in seen_venues:
            problems.append(f"{venue!r}: the spelling appears twice")
        seen_venues.add(venue)

        if row["status"] not in (geocoding.STATUS_MAPPED, geocoding.STATUS_UNMAPPABLE):
            problems.append(f"{venue!r}: status {row['status']!r} is neither mapped nor unmappable")
            continue
        if row["status"] == geocoding.STATUS_UNMAPPABLE:
            if row["latitude"] or row["longitude"]:
                problems.append(f"{venue!r}: unmappable but carries coordinates")
            continue

        if not row["latitude"] or not row["longitude"]:
            problems.append(f"{venue!r}: mapped but carries no coordinates")
            continue
        latitude, longitude = float(row["latitude"]), float(row["longitude"])
        if row["timezone"] not in zones:
            problems.append(f"{venue!r}: timezone {row['timezone']!r} is not an IANA zone")
        if row["country_code"] not in COUNTRY_BOXES:
            problems.append(f"{venue!r}: country code {row['country_code']!r} has no bounding box in this test")
        elif not is_inside_country(row["country_code"], latitude, longitude):
            problems.append(f"{venue!r}: ({latitude}, {longitude}) is not inside {row['country_code']}")
        geocoding.parse_votes(row["countries_voted"])
    return problems


def era5_day_problems(path: str, locations: Dict[str, geocoding.VenueLocation]) -> List[str]:
    """Every way a cached ERA5 day can be wrong, held against the venue table it joins to."""
    problems: List[str] = []
    days: Dict[tuple, dict] = {}
    with open(path, encoding="utf-8") as handle:
        for number, line in enumerate(handle, start=1):
            line = line.strip()
            if not line:
                continue
            day = json.loads(line)
            where = f"{path}:{number}"
            missing = {"venue", "date", "tz", "t", "rh", "p", "p7", "fetched_at"} - set(day)
            if missing:
                problems.append(f"{where}: missing {sorted(missing)}")
                continue
            identity = (day["venue"], day["date"])
            if identity in days:
                problems.append(f"{where}: {identity} is cached twice")
            days[identity] = day

            date.fromisoformat(day["date"])
            for name, length in (("t", HOURS_PER_DAY), ("rh", HOURS_PER_DAY), ("p", HOURS_PER_DAY), ("p7", PRIOR_DAYS)):
                if len(day[name]) != length:
                    problems.append(f"{where}: {name!r} has {len(day[name])} slots, expected {length}")
            location = locations.get(day["venue"])
            if location is None:
                problems.append(f"{where}: venue {day['venue']!r} is not in the curated table")
            elif location.timezone != day["tz"]:
                problems.append(f"{where}: zone {day['tz']!r} disagrees with the table's {location.timezone!r}")
            problems.extend(_reading_range_problems(where, day))

    problems.extend(_prior_window_problems(days))
    return problems


def _reading_range_problems(where: str, day: dict) -> List[str]:
    """Readings outside what a surface observation can be."""
    problems = []
    for name, low, high in (("t", -70.0, 60.0), ("rh", 0.0, 100.0), ("p", 0.0, 500.0)):
        for hour, value in enumerate(day[name]):
            if value is not None and not low <= value <= high:
                problems.append(f"{where}: {name}[{hour}] = {value} is outside [{low}, {high}]")
    return problems


def _prior_window_problems(days: Dict[tuple, dict]) -> List[str]:
    """Two cached days one day apart describe six of the same prior days, and each prior
    entry is that local day's own rain total. Where the windows overlap they must agree
    entry for entry -- which is the invariant a wrong UTC offset breaks (DATA-04), because
    the shifted day's total is summed over the wrong hours in exactly one of the two."""
    problems = []
    for (venue, day_string), day in days.items():
        previous = days.get((venue, (date.fromisoformat(day_string) - timedelta(days=1)).isoformat()))
        if previous is None:
            continue
        if day["p7"][:-1] != previous["p7"][1:]:
            problems.append(
                f"{venue} {day_string}: prior-day rain {day['p7']} disagrees with the day before's {previous['p7']}"
            )
    return problems


@pytest.fixture(scope="module")
def committed_locations() -> Dict[str, geocoding.VenueLocation]:
    return geocoding.read_locations(GEOCODING_CSV)


def test_the_committed_venue_table_is_well_formed() -> None:
    """The curated table itself: keys fold from their own names and are unique, every
    mapped row carries coordinates in a real IANA zone, and every point is in the country
    the row names."""
    assert geocoding_table_problems(GEOCODING_CSV) == []


def test_the_committed_era5_days_are_well_formed(committed_locations) -> None:
    """The cached weather days: one row per (venue, date), a full local day of hours per
    variable, a week of prior-day totals, a venue the table holds, and the table's zone."""
    assert era5_day_problems(ERA5_JSONL, committed_locations) == []


def test_every_venue_in_the_table_has_cached_days_and_the_reverse() -> None:
    """The two files are joined on the key, so a row on one side with no partner on the
    other is a restore that will silently produce no weather for those matches."""
    with open(GEOCODING_CSV, newline="", encoding="utf-8") as handle:
        table_keys = {row["venue_key"] for row in csv.DictReader(handle)}
    cached_keys = set()
    with open(ERA5_JSONL, encoding="utf-8") as handle:
        for line in handle:
            if line.strip():
                cached_keys.add(json.loads(line)["venue"])

    assert cached_keys - table_keys == set()
    assert table_keys - cached_keys == set()


def test_the_committed_era5_days_are_warmest_in_the_afternoon() -> None:
    """A physical check on the hour indexing, independent of the code that wrote it: the
    slots are a local clock day, so pooled over every cached day the warmest slot is an
    afternoon hour and the coldest is around dawn. DATA-04 had 12.8% of these days an hour
    out, and a regression that shifted them wholesale -- a UTC request read as local, an
    offset applied the wrong way -- moves this peak off the afternoon.
    """
    totals = [0.0] * HOURS_PER_DAY
    counts = [0] * HOURS_PER_DAY
    with open(ERA5_JSONL, encoding="utf-8") as handle:
        for line in handle:
            if not line.strip():
                continue
            for hour, value in enumerate(json.loads(line)["t"]):
                if value is not None:
                    totals[hour] += value
                    counts[hour] += 1

    means = [totals[hour] / counts[hour] for hour in range(HOURS_PER_DAY)]

    assert 12 <= means.index(max(means)) <= 17
    assert 3 <= means.index(min(means)) <= 8


def _write_table(path: str, rows: Sequence[dict]) -> str:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=geocoding.CSV_COLUMNS)
        writer.writeheader()
        writer.writerows(rows)
    return path


def _sound_row(**overrides) -> dict:
    row = {name: "" for name in geocoding.CSV_COLUMNS}
    row.update(
        venue="Lord's, London",
        venue_key="lord s london",
        status=geocoding.STATUS_MAPPED,
        country_code="GB",
        country="United Kingdom",
        latitude="51.5299",
        longitude="-0.1727",
        timezone="Europe/London",
        countries_voted="GB:10",
        source=geocoding.SOURCE,
    )
    row.update(overrides)
    return row


@pytest.mark.parametrize(
    "corruption,expected",
    [
        pytest.param({"venue_key": "lords london"}, "is not the fold of its own name", id="key stops folding"),
        pytest.param({"latitude": "-51.5299"}, "is not inside GB", id="latitude sign flipped"),
        pytest.param({"longitude": "51.5299", "latitude": "-0.1727"}, "is not inside GB", id="coordinates swapped"),
        pytest.param({"country_code": "IN"}, "is not inside IN", id="placed in another country"),
        pytest.param({"timezone": "Europe/Londonn"}, "is not an IANA zone", id="zone misspelled"),
        pytest.param({"latitude": "", "longitude": ""}, "carries no coordinates", id="coordinates dropped"),
        pytest.param({"status": "located"}, "is neither mapped nor unmappable", id="unknown status"),
    ],
)
def test_a_corrupted_venue_row_is_refused(tmp_path, corruption, expected) -> None:
    """Each check bites: one field edited the way a hand edit slips, one problem reported."""
    path = _write_table(str(tmp_path / "geo.csv"), [_sound_row(**corruption)])

    problems = geocoding_table_problems(path)

    assert any(expected in problem for problem in problems), problems


def test_a_duplicated_venue_key_is_refused(tmp_path) -> None:
    """Two rows sharing a key make the coordinate join ambiguous, and one of the two
    grounds unreachable."""
    rows = [_sound_row(), _sound_row(venue="Lord's. London", latitude="51.5300")]
    path = _write_table(str(tmp_path / "geo.csv"), rows)

    problems = geocoding_table_problems(path)

    assert any("shares key" in problem for problem in problems), problems


def _era5_line(**overrides) -> dict:
    day = {
        "venue": "lord s london",
        "date": "2024-06-01",
        "tz": "Europe/London",
        "t": [15.0] * HOURS_PER_DAY,
        "rh": [70.0] * HOURS_PER_DAY,
        "p": [0.0] * HOURS_PER_DAY,
        "p7": [0.0] * PRIOR_DAYS,
        "fetched_at": "2026-09-05",
    }
    day.update(overrides)
    return day


def _write_days(path: str, days: Sequence[dict]) -> str:
    with open(path, "w", encoding="utf-8") as handle:
        for day in days:
            handle.write(json.dumps(day) + "\n")
    return path


@pytest.mark.parametrize(
    "corruption,expected",
    [
        pytest.param({"t": [15.0] * 23}, "'t' has 23 slots", id="a short temperature day"),
        pytest.param({"p7": [0.0] * 6}, "'p7' has 6 slots", id="a short prior week"),
        pytest.param({"venue": "lords london"}, "is not in the curated table", id="a venue the table lacks"),
        pytest.param({"tz": "Europe/Dublin"}, "disagrees with the table's", id="a zone the table disagrees with"),
        pytest.param({"rh": [140.0] * HOURS_PER_DAY}, "is outside [0.0, 100.0]", id="impossible humidity"),
    ],
)
def test_a_corrupted_cached_day_is_refused(tmp_path, corruption, expected) -> None:
    """Each cached-day check bites on the shape a re-fetch can plausibly produce."""
    locations = {
        "lord s london": geocoding.VenueLocation(
            "Lord's, London", "lord s london", geocoding.STATUS_MAPPED, timezone="Europe/London"
        )
    }
    path = _write_days(str(tmp_path / "era5.jsonl"), [_era5_line(**corruption)])

    problems = era5_day_problems(path, locations)

    assert any(expected in problem for problem in problems), problems


def test_a_cached_day_missing_a_field_is_refused(tmp_path) -> None:
    """A row written by an older or a broken reducer, with a field simply absent."""
    day = _era5_line()
    del day["p7"]
    path = _write_days(str(tmp_path / "era5.jsonl"), [day])

    problems = era5_day_problems(path, {})

    assert any("missing ['p7']" in problem for problem in problems), problems


def test_prior_day_windows_that_disagree_where_they_overlap_are_refused(tmp_path) -> None:
    """The DATA-04 shape: one of two adjacent cached days summed a prior local day over
    the wrong hours, so the six days the two windows share no longer agree."""
    locations = {
        "lord s london": geocoding.VenueLocation(
            "Lord's, London", "lord s london", geocoding.STATUS_MAPPED, timezone="Europe/London"
        )
    }
    earlier = _era5_line(date="2024-05-31", p7=[1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0])
    later = _era5_line(date="2024-06-01", p7=[2.0, 3.0, 4.0, 5.0, 6.0, 9.9, 8.0])
    path = _write_days(str(tmp_path / "era5.jsonl"), [earlier, later])

    problems = era5_day_problems(path, locations)

    assert any("disagrees with the day before" in problem for problem in problems), problems


def test_adjacent_cached_days_that_agree_are_accepted(tmp_path) -> None:
    """The same pair, correct: the later day's window is the earlier one shifted by one."""
    locations = {
        "lord s london": geocoding.VenueLocation(
            "Lord's, London", "lord s london", geocoding.STATUS_MAPPED, timezone="Europe/London"
        )
    }
    earlier = _era5_line(date="2024-05-31", p7=[1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0])
    later = _era5_line(date="2024-06-01", p7=[2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0])
    path = _write_days(str(tmp_path / "era5.jsonl"), [earlier, later])

    assert era5_day_problems(path, locations) == []
