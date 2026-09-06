"""The four weather families X-2 gates, each computable before the first ball (H-21).

Every column reads the hours *before* the inferred start (``sessions.SessionWindow``) or
the days before the match day; nothing reads the match's own hours. A match whose day the
archive has no readings for reads 0.0 on every weather column and 0.0 on ``wx_known`` --
its own category, the shape ``age`` / ``age_known`` uses -- never an imputed climate.

The rain family is precipitation that has already fallen (the day before, the week before,
the hours before the start), which is what the archive can say pre-match; the forecast-side
equivalent at serving time would be the forecast API's precipitation probability, and the
serving rule is written only if the family ships.
"""

from __future__ import annotations

from typing import Dict, List, Optional, Sequence

from ml.weather.archive import DayWeather
from ml.weather.sessions import SessionWindow

#: The hours before the inferred start the humidity and temperature are averaged over.
PRE_MATCH_HOURS = 3
KNOWN_COL = "wx_known"
NIGHT_COL = "wx_night"
WEATHER_FAMILIES: Dict[str, List[str]] = {
    "daynight": [NIGHT_COL],
    "humidity_temperature": ["wx_pre_temp_c", "wx_pre_humidity", KNOWN_COL],
    "dew": ["wx_dew_proxy", KNOWN_COL],
    "rain": ["wx_rain_prior_day_mm", "wx_rain_prior_week_mm", KNOWN_COL],
}
WEATHER_COLS: List[str] = sorted({col for cols in WEATHER_FAMILIES.values() for col in cols})


def _mean(values: Sequence[Optional[float]]) -> Optional[float]:
    present = [float(v) for v in values if v is not None]
    return sum(present) / len(present) if present else None


def _total(values: Sequence[Optional[float]]) -> float:
    return float(sum(float(v) for v in values if v is not None))


def unknown_row(window: SessionWindow) -> Dict[str, float]:
    row = {col: 0.0 for col in WEATHER_COLS}
    row[NIGHT_COL] = 1.0 if window.night else 0.0
    return row


def feature_row(window: SessionWindow, day: Optional[DayWeather]) -> Dict[str, float]:
    """Every weather column for one match, from its window and its day's readings."""
    row = unknown_row(window)
    if day is None:
        return row
    start = window.start_hour
    pre_hours = range(max(0, start - PRE_MATCH_HOURS), start)
    temperature = _mean([day.temperature_c[h] for h in pre_hours])
    humidity = _mean([day.relative_humidity[h] for h in pre_hours])
    if temperature is None or humidity is None:
        return row
    row[KNOWN_COL] = 1.0
    row["wx_pre_temp_c"] = temperature
    row["wx_pre_humidity"] = humidity
    row["wx_dew_proxy"] = row[NIGHT_COL] * humidity / 100.0
    yesterday = day.prior_precipitation_mm[-1] if day.prior_precipitation_mm else None
    row["wx_rain_prior_day_mm"] = (0.0 if yesterday is None else float(yesterday)) + _total(
        day.precipitation_mm[h] for h in range(0, start)
    )
    row["wx_rain_prior_week_mm"] = _total(day.prior_precipitation_mm)
    return row
