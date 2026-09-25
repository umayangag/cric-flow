"""The ERA5 archive, reduced to one day per (venue, date) and cached as a real answer.

Open-Meteo's historical API serves hourly ERA5 readings for any coordinate and date range
without a key, for non-commercial use, under CC BY 4.0 (docs/config-and-data.md § Data-source
licence register). What is kept per (venue key, date) is the match day's 24 local hours of
temperature, relative humidity and precipitation, plus the precipitation totals of the seven
days before it -- enough to compute any pre-match window later without asking again, and a
few hundred bytes per day rather than the hourly archive of every venue's every year.

Readings are asked for in UTC and placed on the venue's local clock here, per hour, from
the IANA zone the curated table holds. The service's own ``timezone=auto`` stamps a whole
range with the offset the zone is on at the moment of the call, which puts every day on the
other side of a DST transition one hour out (DATA-04).

The cache is append-only JSON Lines, one line per (venue, date), flushed after every call
so an interrupted run loses at most the cluster in flight. A day the archive has no
readings for is written as a miss with its reason: that is an answer, and a restore must not
re-ask it. Days too recent for the archive (it trails by several days) are not asked yet,
so they are neither a miss nor a hit.
"""

from __future__ import annotations

import json
import logging
import os
import time
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta, timezone
from datetime import time as clock_time
from typing import Dict, Iterable, List, Mapping, Optional, Protocol, Sequence, Tuple, Union
from zoneinfo import ZoneInfo

import httpx

from ml.weather.geocoding import VenueLocation

logger = logging.getLogger(__name__)

ARCHIVE_URL = "https://archive-api.open-meteo.com/v1/archive"
HOURLY_VARIABLES = ("temperature_2m", "relative_humidity_2m", "precipitation")
SOURCE = "open-meteo-era5"
SOURCE_LICENSE = "CC-BY-4.0"
#: Days of precipitation history kept before each match day (the rain family's window).
PRIOR_DAYS = 7
#: Two match days at one venue closer than this are fetched in one call.
CLUSTER_GAP_DAYS = 45
#: The archive trails the present; days younger than this are not asked for yet. One day
#: more than the margin the service needs, because a call is widened by a day at each end
#: so that a local day's hours are covered whatever the venue's offset from UTC (DATA-04).
ARCHIVE_LAG_DAYS = 8
#: A call covers this many days either side of the days it is for, so that every local
#: hour of the first and last day has a UTC hour in the response.
UTC_MARGIN_DAYS = 1
#: Pacing: the published limit is 600 calls a minute; a tenth of that is plenty.
PAUSE_SECONDS = 0.3
RETRY_PAUSES = (5.0, 30.0, 120.0)
RETRYABLE_STATUSES = (429, 500, 502, 503, 504)
#: The hourly quota is weighted by the data a call returns, so a backfill of long ranges
#: meets it well under the nominal 5,000 calls; the service says to try again next hour,
#: and an unattended run waits for it in these steps rather than failing.
HOURLY_LIMIT_PAUSE_SECONDS = 15 * 60
HOURLY_LIMIT_MAX_WAITS = 6


class DailyLimitReached(RuntimeError):
    """The service's daily quota is spent; resume tomorrow, the cache holds what was fetched."""


@dataclass
class DayWeather:
    venue_key: str
    day: date
    timezone: str
    temperature_c: List[Optional[float]]  # 24 local hours
    relative_humidity: List[Optional[float]]
    precipitation_mm: List[Optional[float]]
    prior_precipitation_mm: List[Optional[float]]  # the PRIOR_DAYS days before, oldest first
    fetched_at: str = ""

    def to_line(self) -> dict:
        return {
            "venue": self.venue_key,
            "date": self.day.isoformat(),
            "tz": self.timezone,
            "t": _rounded(self.temperature_c, 1),
            "rh": _rounded(self.relative_humidity, 0),
            "p": _rounded(self.precipitation_mm, 1),
            "p7": _rounded(self.prior_precipitation_mm, 1),
            "fetched_at": self.fetched_at,
        }


@dataclass
class Miss:
    venue_key: str
    day: date
    reason: str
    fetched_at: str = ""

    def to_line(self) -> dict:
        return {
            "venue": self.venue_key,
            "date": self.day.isoformat(),
            "miss": self.reason,
            "fetched_at": self.fetched_at,
        }


Entry = Union[DayWeather, Miss]


def _rounded(values: Sequence[Optional[float]], digits: int) -> list:
    return [None if v is None else (int(round(v)) if digits == 0 else round(float(v), digits)) for v in values]


def entry_from_line(line: dict) -> Entry:
    day = date.fromisoformat(line["date"])
    if "miss" in line:
        return Miss(line["venue"], day, line["miss"], line.get("fetched_at", ""))
    return DayWeather(
        venue_key=line["venue"],
        day=day,
        timezone=line.get("tz", ""),
        temperature_c=list(line["t"]),
        relative_humidity=list(line["rh"]),
        precipitation_mm=list(line["p"]),
        prior_precipitation_mm=list(line["p7"]),
        fetched_at=line.get("fetched_at", ""),
    )


class WeatherCache:
    """The (venue key, date) -> entry store, on disk as append-only JSON Lines."""

    def __init__(self, path: str) -> None:
        self.path = path
        self.entries: Dict[Tuple[str, date], Entry] = {}
        #: Unparsable lines tolerated on load; only ever the torn final line (DATA-06).
        self.skipped_lines = 0
        if os.path.exists(path):
            self._load(path)

    def _load(self, path: str) -> None:
        """Read every line, tolerating exactly one unparsable line and only as the last.

        A torn final line is what an interrupted append leaves and costs one cluster on
        the next run. An unparsable line anywhere else means the file was corrupted after
        it was written, and silently dropping it would turn the days it held back into
        gaps a later run would re-ask for at whatever coordinates the table then held --
        so it raises, loudly, with the line number (DATA-06).
        """
        torn: Optional[Tuple[int, str]] = None
        with open(path) as fh:
            for number, raw in enumerate(fh, start=1):
                raw = raw.strip()
                if not raw:
                    continue
                if torn is not None:
                    line_number, reason = torn
                    raise ValueError(
                        f"{path}: line {line_number} could not be read ({reason}) and is not the last line; "
                        f"the cache is corrupt, not merely torn by an interrupted append"
                    )
                try:
                    entry = entry_from_line(json.loads(raw))
                except (ValueError, KeyError) as exc:
                    torn = (number, f"{type(exc).__name__}: {exc}")
                    continue
                self.entries[(entry.venue_key, entry.day)] = entry
        if torn is not None:
            self.skipped_lines = 1
            logger.warning("%s: dropped torn final line %d (%s)", path, torn[0], torn[1])

    def get(self, venue_key: str, day: date) -> Optional[Entry]:
        return self.entries.get((venue_key, day))

    def __len__(self) -> int:
        return len(self.entries)

    def append(self, entries: Iterable[Entry]) -> None:
        """Record answers and flush before returning, so the file is always a truthful
        account of what has been asked."""
        os.makedirs(os.path.dirname(os.path.abspath(self.path)), exist_ok=True)
        with open(self.path, "a") as fh:
            for entry in entries:
                self.entries[(entry.venue_key, entry.day)] = entry
                fh.write(json.dumps(entry.to_line(), separators=(",", ":")) + "\n")
            fh.flush()

    def days(self) -> Iterable[DayWeather]:
        return (e for e in self.entries.values() if isinstance(e, DayWeather))

    def counts(self) -> Dict[str, int]:
        hits = sum(1 for e in self.entries.values() if isinstance(e, DayWeather))
        return {"days": hits, "misses": len(self.entries) - hits}


@dataclass
class ArchiveResponse:
    """The parts of one archive call the reduction reads: hourly readings stamped in UTC."""

    hourly_time: List[str]  # UTC ISO minutes, "2015-03-01T00:00"
    hourly: Dict[str, List[Optional[float]]] = field(default_factory=dict)


class ArchiveClient(Protocol):
    def fetch(self, latitude: float, longitude: float, start: date, end: date) -> ArchiveResponse: ...


class OpenMeteoArchive:
    """The archive endpoint, paced, with a retry on transient errors and a distinct error
    when the daily quota is spent."""

    def __init__(self, http: Optional[httpx.Client] = None, pause_seconds: float = PAUSE_SECONDS) -> None:
        self._http = http or httpx.Client(timeout=60.0)
        self._pause = pause_seconds
        self.calls = 0

    def fetch(self, latitude: float, longitude: float, start: date, end: date) -> ArchiveResponse:
        params = {
            "latitude": f"{latitude:.4f}",
            "longitude": f"{longitude:.4f}",
            "start_date": start.isoformat(),
            "end_date": end.isoformat(),
            "hourly": ",".join(HOURLY_VARIABLES),
            # UTC, never "auto": the service stamps a whole range with the offset the zone
            # happens to be on *at the moment of the call*, so an "auto" response labels a
            # January day in Sydney with the January-in-September offset. The local hours
            # are built here instead, per hour, from the venue's IANA zone (DATA-04).
            "timezone": "UTC",
        }
        reason = ""
        hourly_waits = 0
        for attempt, retry_pause in enumerate((*RETRY_PAUSES, None)):
            self.calls += 1
            try:
                response = self._http.get(ARCHIVE_URL, params=params)
                time.sleep(self._pause)
                if response.status_code == 200:
                    return _parse(response.json())
                reason = _error_reason(response)
                if response.status_code == 429 and "daily" in reason.casefold():
                    raise DailyLimitReached(reason)
                if response.status_code == 429 and "hourly" in reason.casefold():
                    if hourly_waits >= HOURLY_LIMIT_MAX_WAITS:
                        raise RuntimeError(f"archive: hourly quota still spent after {hourly_waits} waits: {reason}")
                    hourly_waits += 1
                    logger.warning("archive: hourly quota spent (%s); waiting %d s", reason, HOURLY_LIMIT_PAUSE_SECONDS)
                    time.sleep(HOURLY_LIMIT_PAUSE_SECONDS)
                    continue
                if response.status_code not in RETRYABLE_STATUSES:
                    response.raise_for_status()
            except (httpx.TransportError, ValueError) as exc:
                # A dropped connection, or a 200 whose body is not JSON (the service does
                # that under load): transient, like a 503, and retried the same way.
                reason = f"{type(exc).__name__}: {exc}"
            if retry_pause is None:
                raise RuntimeError(f"archive call failed after {attempt + 1} attempts: {reason}")
            logger.warning("archive call failed (%s); retry %d in %.0f s", reason, attempt + 1, retry_pause)
            time.sleep(retry_pause)
        raise RuntimeError("unreachable")


def _parse(payload: dict) -> ArchiveResponse:
    return ArchiveResponse(
        hourly_time=list(payload.get("hourly", {}).get("time", [])),
        hourly={k: list(v) for k, v in payload.get("hourly", {}).items() if k != "time"},
    )


def _error_reason(response: httpx.Response) -> str:
    try:
        return str(response.json().get("reason", response.text))
    except ValueError:
        return response.text


def clusters(days: Sequence[date], gap_days: int = CLUSTER_GAP_DAYS) -> List[List[date]]:
    """Sorted days grouped so that consecutive members are at most ``gap_days`` apart: one
    archive call per group, spanning it, its ``PRIOR_DAYS`` lead and a day's margin at
    each end. A group may span a season; the local hours are built per hour from the
    venue's zone, so a DST transition inside the span costs nothing (DATA-04)."""
    out: List[List[date]] = []
    for day in sorted(set(days)):
        if out and (day - out[-1][-1]).days <= gap_days:
            out[-1].append(day)
        else:
            out.append([day])
    return out


def local_hour_keys(day: date, zone: str) -> List[str]:
    """The UTC hour labels of a local day's 24 clock hours, 00:00 to 23:00 in ``zone``.

    The offset is resolved per hour from the IANA zone, so a day on either side of a DST
    transition lands on the right UTC hours (DATA-04). ERA5 is an hourly grid, so a zone
    whose offset is not a whole number of hours (Asia/Kolkata's +5:30) is floored to the
    whole hour, which is what the service itself does for its local grid.
    """
    info = ZoneInfo(zone)
    keys = []
    for hour in range(24):
        local = datetime.combine(day, clock_time(hour), tzinfo=info)
        whole_hours = local.utcoffset() // timedelta(hours=1)
        utc = datetime.combine(day, clock_time(hour)) - timedelta(hours=whole_hours)
        keys.append(f"{utc.date().isoformat()}T{utc.hour:02d}:00")
    return keys


class _HourlyReadings:
    """One response's hourly series, read by UTC hour label."""

    def __init__(self, response: ArchiveResponse) -> None:
        self._index = {label: i for i, label in enumerate(response.hourly_time)}
        self._series = response.hourly

    def values(self, variable: str, keys: Sequence[str]) -> List[Optional[float]]:
        values = self._series.get(variable, [])
        out: List[Optional[float]] = []
        for key in keys:
            i = self._index.get(key)
            out.append(None if i is None or i >= len(values) else values[i])
        return out

    def has_temperature(self, keys: Sequence[str]) -> bool:
        return any(v is not None for v in self.values("temperature_2m", keys))


def reduce_days(
    venue_key: str, response: ArchiveResponse, days: Sequence[date], fetched_at: str, zone: str
) -> List[Entry]:
    """The entries for ``days`` out of one response: each day's 24 local hours in ``zone``
    per variable and the local daily precipitation totals of the week before; a day the
    archive has no readings for is a miss."""
    readings = _HourlyReadings(response)
    out: List[Entry] = []
    for day in days:
        keys = local_hour_keys(day, zone)
        series = {variable: readings.values(variable, keys) for variable in HOURLY_VARIABLES}
        if all(v is None for v in series["temperature_2m"]):
            neighbours = (day - timedelta(days=1), day + timedelta(days=1))
            if not any(readings.has_temperature(local_hour_keys(n, zone)) for n in neighbours):
                # A miss is permanent: a restore never re-asks it. A day with no readings
                # whose neighbours have none either is a call that came back empty, not an
                # archive gap, so it is refused rather than recorded (DATA-06).
                raise ValueError(
                    f"{venue_key} {day.isoformat()}: the response holds no readings for this day "
                    f"or either neighbour; refusing to record a permanent miss from an empty answer"
                )
            out.append(Miss(venue_key, day, "no hourly readings in the archive", fetched_at))
            continue
        prior = []
        for back in range(PRIOR_DAYS, 0, -1):
            hours = readings.values("precipitation", local_hour_keys(day - timedelta(days=back), zone))
            present = [v for v in hours if v is not None]
            # The local day's own total, summed here rather than taken from the service's
            # daily block, which under a UTC request would be a UTC day (DATA-04).
            prior.append(float(sum(present)) if present else None)
        out.append(
            DayWeather(
                venue_key=venue_key,
                day=day,
                timezone=zone,
                temperature_c=series["temperature_2m"],
                relative_humidity=series["relative_humidity_2m"],
                precipitation_mm=series["precipitation"],
                prior_precipitation_mm=prior,
                fetched_at=fetched_at,
            )
        )
    return out


def backfill(
    locations: Mapping[str, VenueLocation],
    days_by_venue: Mapping[str, Sequence[date]],
    cache: WeatherCache,
    client: ArchiveClient,
    today: Optional[date] = None,
) -> Dict[str, int]:
    """Fetch every (venue, day) the cache has no entry for, one call per cluster, appending
    as it goes. Venues without coordinates are skipped (the geocoding table records why).
    Returns what it did; raises ``DailyLimitReached`` with the cache intact."""
    today = today or date.today()
    horizon = today - timedelta(days=ARCHIVE_LAG_DAYS)
    counts = {"venues": 0, "calls": 0, "days_fetched": 0, "misses": 0, "too_recent": 0, "unmapped_days": 0}
    for key in sorted(days_by_venue):
        location = locations.get(key)
        wanted = [d for d in days_by_venue[key] if cache.get(key, d) is None]
        if not wanted:
            continue
        # A row with no IANA zone cannot be reduced to local hours, so it counts as
        # unmapped rather than being fetched into a day whose hours mean nothing.
        if location is None or not location.mapped or not location.timezone:
            counts["unmapped_days"] += len(wanted)
            continue
        recent = [d for d in wanted if d > horizon]
        counts["too_recent"] += len(recent)
        wanted = [d for d in wanted if d <= horizon]
        if not wanted:
            continue
        counts["venues"] += 1
        for cluster in clusters(wanted):
            fetched_at = datetime.now(timezone.utc).date().isoformat()
            response = client.fetch(
                location.latitude,
                location.longitude,
                cluster[0] - timedelta(days=PRIOR_DAYS + UTC_MARGIN_DAYS),
                cluster[-1] + timedelta(days=UTC_MARGIN_DAYS),
            )
            entries = reduce_days(key, response, cluster, fetched_at, location.timezone)
            cache.append(entries)
            counts["calls"] += 1
            counts["days_fetched"] += sum(1 for e in entries if isinstance(e, DayWeather))
            counts["misses"] += sum(1 for e in entries if isinstance(e, Miss))
        if counts["venues"] % 25 == 0:
            logger.info("weather: %s", counts)
    return counts
