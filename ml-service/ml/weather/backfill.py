"""The one command for the weather data: acquire what is missing, restore what is held.

    python -m ml.weather.backfill --cricsheet-dir DIR                 # geocode + fetch what the cache lacks
    python -m ml.weather.backfill --cricsheet-dir DIR --offline --to-db   # the restore: nothing over the network

The two tracked files under ``reference-data/`` are the cache itself -- the venue table
(``venue-geocoding.csv``, curated: rows are added, never rewritten) and the reduced ERA5
days (``era5-venue-days.jsonl``, append-only, misses included) -- so a run on a fresh
clone asks only about venues and days the files do not hold, and ``--offline`` asks
nothing at all. ``--to-db`` writes the coordinates onto ``venue`` and the days into
``venue_weather`` (migration 0012); ``--report`` prints what the files cover and which
session rule placed how many matches.
"""

from __future__ import annotations

import argparse
import collections
import json
import logging
import os
import sys
from datetime import date, datetime, timezone
from typing import Any, Dict, List, Mapping, Optional, Sequence

from ml.weather import archive, features, geocoding, sessions, venues

logger = logging.getLogger(__name__)

REPO_ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..")
DEFAULT_GEOCODING = geocoding.DEFAULT_LOCATIONS_PATH
DEFAULT_CACHE = os.path.join(REPO_ROOT, "reference-data", "era5-venue-days.jsonl")

_WEATHER_UPSERT_SQL = """
INSERT INTO venue_weather (
    venue_id, weather_date, timezone, hourly_temperature_c, hourly_relative_humidity,
    hourly_precipitation_mm, prior_week_precipitation_mm, source, source_license, fetched_at
) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (venue_id, weather_date) DO UPDATE SET
    timezone = EXCLUDED.timezone,
    hourly_temperature_c = EXCLUDED.hourly_temperature_c,
    hourly_relative_humidity = EXCLUDED.hourly_relative_humidity,
    hourly_precipitation_mm = EXCLUDED.hourly_precipitation_mm,
    prior_week_precipitation_mm = EXCLUDED.prior_week_precipitation_mm,
    source = EXCLUDED.source,
    source_license = EXCLUDED.source_license,
    fetched_at = EXCLUDED.fetched_at
"""


def coverage(
    fixtures: Sequence[venues.Fixture],
    locations: Mapping[str, geocoding.VenueLocation],
    cache: archive.WeatherCache,
    windows: Mapping[str, sessions.SessionWindow],
) -> Dict[str, Any]:
    """What the two files reach: per format, matches at a mapped venue, with a weather
    day, with a recorded miss, and not yet asked (``unanswered``); and what the features
    make of them -- how many matches the rules place at night, and how many have readings
    in their pre-match window (``features_known``)."""
    per_format: Dict[str, collections.Counter] = collections.defaultdict(collections.Counter)
    for fixture in fixtures:
        counter = per_format[fixture.match_type]
        counter["matches"] += 1
        location = locations.get(fixture.venue_key)
        if location is None or not location.mapped:
            counter["venue_unmapped"] += 1
            continue
        counter["venue_mapped"] += 1
        entry = cache.get(fixture.venue_key, fixture.date)
        if isinstance(entry, archive.DayWeather):
            counter["weather_day"] += 1
        elif isinstance(entry, archive.Miss):
            counter["weather_miss"] += 1
        else:
            counter["unanswered"] += 1
        row = features.feature_row(windows[fixture.match_id], entry if isinstance(entry, archive.DayWeather) else None)
        counter["night"] += int(row[features.NIGHT_COL])
        counter["features_known"] += int(row[features.KNOWN_COL])
    total = collections.Counter()
    for counter in per_format.values():
        total.update(counter)
    per_format["all"] = total
    return {
        "venues": {
            "total": len(locations),
            "mapped": sum(1 for loc in locations.values() if loc.mapped),
            "unmappable": sum(1 for loc in locations.values() if not loc.mapped),
        },
        "cache": cache.counts(),
        "matches": {fmt: dict(counter) for fmt, counter in per_format.items()},
        "session_rules": sessions.census(windows),
    }


def print_report(report: Dict[str, Any]) -> None:
    v, c = report["venues"], report["cache"]
    print(f"Venues: {v['mapped']} mapped, {v['unmappable']} unmappable of {v['total']}")
    print(f"Cache: {c['days']} venue-days, {c['misses']} misses")
    print()
    print("| format | matches | venue mapped | weather day | miss | unanswered | pre-match readings | night |")
    print("|---|---:|---:|---:|---:|---:|---:|---:|")
    for fmt, counts in report["matches"].items():
        m = counts.get("matches", 0)
        share = 100.0 * counts.get("weather_day", 0) / m if m else 0.0
        known = 100.0 * counts.get("features_known", 0) / m if m else 0.0
        night = 100.0 * counts.get("night", 0) / m if m else 0.0
        print(
            f"| {fmt} | {m} | {counts.get('venue_mapped', 0)} | {counts.get('weather_day', 0)} ({share:.1f} %) | "
            f"{counts.get('weather_miss', 0)} | {counts.get('unanswered', 0)} | {known:.1f} % | {night:.1f} % |"
        )
    print()
    print("| session rule | matches |")
    print("|---|---:|")
    for rule, n in report["session_rules"].items():
        print(f"| {rule} | {n} |")


def write_to_db(
    connection, locations: Mapping[str, geocoding.VenueLocation], cache: archive.WeatherCache
) -> Dict[str, int]:
    """Coordinates onto ``venue``, days into ``venue_weather``, keyed through the venue
    name the importer stored -- the same string the archive spells."""
    now = datetime.now(timezone.utc)
    counts = {"venues_updated": 0, "weather_rows": 0, "venues_without_row": 0}
    with connection.cursor() as cur:
        cur.execute("SELECT id, venue_name FROM venue")
        ids_by_key: Dict[str, List[int]] = collections.defaultdict(list)
        for venue_id, venue_name in cur.fetchall():
            ids_by_key[venues.venue_key(venue_name)].append(venue_id)
        for key, location in locations.items():
            if not location.mapped:
                continue
            if key not in ids_by_key:
                counts["venues_without_row"] += 1
                continue
            for venue_id in ids_by_key[key]:
                cur.execute(
                    "UPDATE venue SET city=%s, country=%s, latitude=%s, longitude=%s, timezone=%s, source=%s, "
                    "verified_at=%s, updated_at=now() WHERE id=%s",
                    (
                        location.place,
                        location.country,
                        location.latitude,
                        location.longitude,
                        location.timezone,
                        location.source,
                        now,
                        venue_id,
                    ),
                )
                counts["venues_updated"] += cur.rowcount
        rows = []
        for day in cache.days():
            for venue_id in ids_by_key.get(day.venue_key, []):
                rows.append(
                    (
                        venue_id,
                        day.day,
                        day.timezone,
                        day.temperature_c,
                        day.relative_humidity,
                        day.precipitation_mm,
                        day.prior_precipitation_mm,
                        archive.SOURCE,
                        archive.SOURCE_LICENSE,
                        day.fetched_at or None,
                    )
                )
        cur.executemany(_WEATHER_UPSERT_SQL, rows)
        counts["weather_rows"] = len(rows)
    connection.commit()
    return counts


def run(args: argparse.Namespace) -> Dict[str, Any]:
    fixtures, facts = venues.read_archive(args.cricsheet_dir)
    locations = geocoding.read_locations(args.geocoding)
    if not args.offline:
        asked = geocoding.geocode_missing(facts, locations, geocoding.OpenMeteoGeocoding())
        if asked:
            geocoding.write_locations(args.geocoding, locations)
        logger.info("geocoding: %d venues asked about, %d rows in %s", asked, len(locations), args.geocoding)
    cache = archive.WeatherCache(args.cache)
    if not args.offline:
        client = archive.OpenMeteoArchive()
        try:
            counts = archive.backfill(locations, venues.dates_by_venue(fixtures), cache, client, today=date.today())
        except archive.DailyLimitReached as exc:
            logger.error("archive: daily quota spent (%s); the cache holds what was fetched, run again tomorrow", exc)
            raise
        logger.info("archive: %s (%d HTTP calls)", counts, client.calls)
    country_by_key = {key: loc.country_code for key, loc in locations.items() if loc.mapped}
    windows = sessions.assign(fixtures, country_by_key)
    report = coverage(fixtures, locations, cache, windows)
    if args.to_db:
        from ml.db import get_db_connection

        written = write_to_db(get_db_connection(), locations, cache)
        logger.info("database: %s", written)
        report["database"] = written
    if args.report:
        print_report(report)
    if args.report_json:
        with open(args.report_json, "w") as fh:
            json.dump(report, fh, indent=2)
    return report


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--cricsheet-dir", required=True, help="directory of Cricsheet JSON files")
    p.add_argument("--geocoding", default=DEFAULT_GEOCODING, help="the curated venue -> coordinates CSV")
    p.add_argument("--cache", default=DEFAULT_CACHE, help="the (venue, date) JSON Lines cache")
    p.add_argument("--offline", action="store_true", help="ask nothing over the network; serve the files")
    p.add_argument("--to-db", action="store_true", help="write venue coordinates and venue_weather (POSTGRES_* env)")
    p.add_argument("--report", action="store_true", help="print coverage and the session-rule census")
    p.add_argument("--report-json", help="also write the coverage report here")
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    run(_parse_args(argv))
    return 0


if __name__ == "__main__":
    sys.exit(main())
