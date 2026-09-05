"""The ERA5 archive, reduced to one day per (venue, date) and cached as a real answer.

Open-Meteo's historical API serves hourly ERA5 readings for any coordinate and date range
without a key, for non-commercial use, under CC BY 4.0 (docs/config-and-data.md § Data-source
licence register). What is kept per (venue key, date) is the match day's 24 local hours of
temperature, relative humidity and precipitation, plus the precipitation totals of the seven
days before it -- enough to compute any pre-match window later without asking again, and a
few hundred bytes per day rather than the hourly archive of every venue's every year.

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
from typing import Dict, Iterable, List, Mapping, Optional, Protocol, Sequence, Tuple, Union

import httpx

from ml.weather.geocoding import VenueLocation

logger = logging.getLogger(__name__)

ARCHIVE_URL = "https://archive-api.open-meteo.com/v1/archive"
HOURLY_VARIABLES = ("temperature_2m", "relative_humidity_2m", "precipitation")
DAILY_VARIABLES = ("precipitation_sum",)
SOURCE = "open-meteo-era5"
SOURCE_LICENSE = "CC-BY-4.0"
#: Days of precipitation history kept before each match day (the rain family's window).
PRIOR_DAYS = 7
#: Two match days at one venue closer than this are fetched in one call.
CLUSTER_GAP_DAYS = 45
#: The archive trails the present; days younger than this are not asked for yet.
ARCHIVE_LAG_DAYS = 7
#: Pacing: the published limit is 600 calls a minute; a tenth of that is plenty.
PAUSE_SECONDS = 0.3
RETRY_PAUSES = (5.0, 30.0, 120.0)


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
            "p": _rounded(self.precipitation_mm, 2),
            "p7": _rounded(self.prior_precipitation_mm, 2),
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
        if os.path.exists(path):
            with open(path) as fh:
                for raw in fh:
                    raw = raw.strip()
                    if not raw:
                        continue
                    try:
                        entry = entry_from_line(json.loads(raw))
                    except (ValueError, KeyError):
                        # A torn final line is what an interrupted append leaves; dropping
                        # it costs one cluster on the next run.
                        continue
                    self.entries[(entry.venue_key, entry.day)] = entry

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
    """The parts of one archive call the reduction reads."""

    timezone: str
    hourly_time: List[str]  # local ISO minutes, "2015-03-01T00:00"
    hourly: Dict[str, List[Optional[float]]] = field(default_factory=dict)
    daily_time: List[str] = field(default_factory=list)
    daily: Dict[str, List[Optional[float]]] = field(default_factory=dict)


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
            "daily": ",".join(DAILY_VARIABLES),
            "timezone": "auto",
        }
        for attempt, retry_pause in enumerate((*RETRY_PAUSES, None)):
            self.calls += 1
            response = self._http.get(ARCHIVE_URL, params=params)
            time.sleep(self._pause)
            if response.status_code == 200:
                payload = response.json()
                return ArchiveResponse(
                    timezone=payload.get("timezone", ""),
                    hourly_time=list(payload.get("hourly", {}).get("time", [])),
                    hourly={k: list(v) for k, v in payload.get("hourly", {}).items() if k != "time"},
                    daily_time=list(payload.get("daily", {}).get("time", [])),
                    daily={k: list(v) for k, v in payload.get("daily", {}).items() if k != "time"},
                )
            reason = _error_reason(response)
            if response.status_code == 429 and "daily" in reason.casefold():
                raise DailyLimitReached(reason)
            if retry_pause is None or response.status_code not in (429, 500, 502, 503, 504):
                response.raise_for_status()
            logger.warning(
                "archive call failed (%s: %s); retry %d in %.0f s",
                response.status_code,
                reason,
                attempt + 1,
                retry_pause,
            )
            time.sleep(retry_pause)
        raise RuntimeError("unreachable")


def _error_reason(response: httpx.Response) -> str:
    try:
        return str(response.json().get("reason", response.text))
    except ValueError:
        return response.text


def clusters(days: Sequence[date], gap_days: int = CLUSTER_GAP_DAYS) -> List[List[date]]:
    """Sorted days grouped so that consecutive members are at most ``gap_days`` apart: one
    archive call per group, spanning it and its ``PRIOR_DAYS`` lead."""
    out: List[List[date]] = []
    for day in sorted(set(days)):
        if out and (day - out[-1][-1]).days <= gap_days:
            out[-1].append(day)
        else:
            out.append([day])
    return out


def reduce_days(venue_key: str, response: ArchiveResponse, days: Sequence[date], fetched_at: str) -> List[Entry]:
    """The entries for ``days`` out of one response: the day's 24 local hours per variable
    and the prior days' precipitation totals; a day with no readings is a miss."""
    by_hour: Dict[str, int] = {t: i for i, t in enumerate(response.hourly_time)}
    by_day: Dict[str, int] = {t: i for i, t in enumerate(response.daily_time)}
    out: List[Entry] = []
    for day in days:
        prefix = day.isoformat()
        indexes = [by_hour.get(f"{prefix}T{hour:02d}:00") for hour in range(24)]
        series = {}
        for variable in HOURLY_VARIABLES:
            values = response.hourly.get(variable, [])
            series[variable] = [None if i is None or i >= len(values) else values[i] for i in indexes]
        if all(v is None for v in series["temperature_2m"]):
            out.append(Miss(venue_key, day, "no hourly readings in the archive", fetched_at))
            continue
        prior = []
        for back in range(PRIOR_DAYS, 0, -1):
            i = by_day.get((day - timedelta(days=back)).isoformat())
            sums = response.daily.get("precipitation_sum", [])
            prior.append(None if i is None or i >= len(sums) else sums[i])
        out.append(
            DayWeather(
                venue_key=venue_key,
                day=day,
                timezone=response.timezone,
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
        if location is None or not location.mapped:
            counts["unmapped_days"] += len(wanted)
            continue
        recent = [d for d in wanted if d > horizon]
        counts["too_recent"] += len(recent)
        wanted = [d for d in wanted if d <= horizon]
        if not wanted:
            continue
        counts["venues"] += 1
        for cluster in clusters(wanted):
            fetched_at = datetime.now(timezone.utc).isoformat(timespec="seconds")
            response = client.fetch(
                location.latitude, location.longitude, cluster[0] - timedelta(days=PRIOR_DAYS), cluster[-1]
            )
            entries = reduce_days(key, response, cluster, fetched_at)
            cache.append(entries)
            counts["calls"] += 1
            counts["days_fetched"] += sum(1 for e in entries if isinstance(e, DayWeather))
            counts["misses"] += sum(1 for e in entries if isinstance(e, Miss))
        if counts["venues"] % 25 == 0:
            logger.info("weather: %s", counts)
    return counts
