import json
import os
from functools import lru_cache
from typing import List

from logging import getLogger

logger = getLogger(__name__)
# Legacy default orders (kept as fallback if config missing)
_DEFAULT_BATTING = [
    "batting_consistency",
    "batting_form",
    "batting_temp",
    "batting_wind",
    "batting_rain",
    "batting_humidity",
    "batting_cloud",
    "batting_pressure",
    "batting_viscosity",
    "batting_inning",
    "batting_session",
    "toss",
    "venue",
    "opposition",
    "season",
]

_DEFAULT_BOWLING = [
    "bowling_consistency",
    "bowling_form",
    "bowling_temp",
    "bowling_wind",
    "bowling_rain",
    "bowling_humidity",
    "bowling_cloud",
    "bowling_pressure",
    "bowling_viscosity",
    "batting_inning",
    "bowling_session",
    "toss",
    "bowling_venue",
    "bowling_opposition",
    "season",
]


def _default_config_path() -> str:
    # repo root: ../../ from app/, then configs/feature_vectors.json
    here = os.path.dirname(__file__)
    return os.path.normpath(os.path.join(here, "..", "..", "configs", "feature_vectors.json"))


def _config_path() -> str:
    # Resolve the path each time so the cache key changes if the env var changes.
    return os.environ.get("FEATURE_CONFIG_PATH") or _default_config_path()


@lru_cache(maxsize=None)
def _load_config(path: str) -> dict:
    try:
        with open(path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
            if not isinstance(data, dict):
                logger.warning("Feature config at %s is not a dictionary, falling back to defaults.", path)
                return {}
            return data
    except FileNotFoundError:
        # This is an expected case when no custom config is provided.
        return {}
    except Exception as e:
        # Silent fallback is intentional, but log a warning.
        logger.warning("Failed to load or parse feature config from %s, falling back to defaults. Error: %s", path, e)
        return {}


def get_feature_names(kind: str) -> List[str]:
    kind = kind.lower().strip()
    data = _load_config(_config_path())
    names = []
    if isinstance(data, dict):
        names = data.get(kind) or []
    if kind == "batting":
        return names if names else list(_DEFAULT_BATTING)
    if kind == "bowling":
        return names if names else list(_DEFAULT_BOWLING)
    raise ValueError(f"unknown feature kind: {kind}")
