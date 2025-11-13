import json
import os
from functools import lru_cache
from typing import List

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


@lru_cache(maxsize=1)
def _load_config() -> dict:
    path = os.environ.get("FEATURE_CONFIG_PATH") or _default_config_path()
    try:
        with open(path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
            if not isinstance(data, dict):
                return {}
            return data
    except Exception:
        # Silent fallback is intentional to preserve backward compat
        return {}


def get_feature_names(kind: str) -> List[str]:
    kind = kind.lower().strip()
    data = _load_config()
    names = []
    if isinstance(data, dict):
        names = data.get(kind) or []
    if kind == "batting":
        return names if names else list(_DEFAULT_BATTING)
    if kind == "bowling":
        return names if names else list(_DEFAULT_BOWLING)
    raise ValueError(f"unknown feature kind: {kind}")
