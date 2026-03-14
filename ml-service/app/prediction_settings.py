"""Settings and small helpers shared by prediction orchestration.

Keeps prediction_service focused on feature building and orchestration logic.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Optional


@dataclass
class GenerateMatchSettings:
    """Configuration for generate_match (models dir, train-on-the-fly, go-app URL, cache granularity)."""

    models_dir: str
    enable_train_on_the_fly: bool
    go_app_url: str
    go_app_api_key: Optional[str]
    train_latest_cache_granularity: str


def round_datetime_to_granularity(dt: datetime, granularity: str) -> datetime:
    """Round datetime down to the given boundary. Used for cache-key stability when use_latest_model=True."""
    gran = (granularity or "").strip().lower()
    if gran in ("none", ""):
        return dt
    if gran == "second":
        return dt.replace(microsecond=0)
    if gran == "minute":
        return dt.replace(second=0, microsecond=0)
    if gran == "hour":
        return dt.replace(minute=0, second=0, microsecond=0)
    if gran == "day":
        return dt.replace(hour=0, minute=0, second=0, microsecond=0)
    return dt  # fallback: no rounding
