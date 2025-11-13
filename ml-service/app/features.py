import json
import os
from typing import Any, List

from .feature_config import get_feature_names

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


_DEF_PATH = os.environ.get("FEATURE_CONFIG_PATH") or os.path.join(
    os.path.dirname(__file__), "..", "configs", "feature_vectors.json"
)


def _load_feature_names(kind: str) -> List[str]:
    try:
        with open(_DEF_PATH, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        vals = data.get(kind) or []
        if kind == "batting" and len(vals) == len(_DEFAULT_BATTING):
            return vals
        if kind == "bowling" and len(vals) == len(_DEFAULT_BOWLING):
            return vals
    except Exception:
        pass
    return _DEFAULT_BATTING if kind == "batting" else _DEFAULT_BOWLING


def batting_feature_vector(f: Any) -> List[float]:
    names = _load_feature_names("batting")
    return [getattr(f, n) for n in names]


def bowling_feature_vector(f: Any) -> List[float]:
    names = get_feature_names("bowling")
    return [getattr(f, n) for n in names]
