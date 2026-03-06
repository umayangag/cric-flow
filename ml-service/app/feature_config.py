import json
import os
from functools import lru_cache
from typing import List

from .logging import get_struct_logger, init_logging

# Initialize logging early
init_logging(service="ml-service", version="0.3.0")
logger = get_struct_logger()


class FeatureConfigError(RuntimeError):
    """Raised when feature names cannot be loaded from the shared configuration file."""


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
                msg = f"Feature config at {path} is not a JSON object (dict)."
                logger.error(msg)
                raise FeatureConfigError(msg)
            return data
    except FileNotFoundError as e:
        msg = (
            f"Feature config file not found at {path}. Set FEATURE_CONFIG_PATH or provide configs/feature_vectors.json."
        )
        logger.error(msg)
        raise FeatureConfigError(msg) from e
    except (json.JSONDecodeError, OSError) as e:
        msg = f"Failed to load or parse feature config from {path}: {e}"
        logger.error(msg)
        raise FeatureConfigError(msg) from e


def get_feature_names(kind: str) -> List[str]:
    kind = kind.lower().strip()
    if kind not in {"batting", "bowling", "fielding"}:
        raise ValueError(f"unknown feature kind: {kind}")

    path = _config_path()
    data = _load_config(path)

    names = data.get(kind)
    if not isinstance(names, list) or not names:
        msg = (
            f"Feature names for kind '{kind}' are missing or invalid in shared config at {path}. "
            f"Ensure the key '{kind}' exists with a non-empty list of feature names."
        )
        logger.error(msg)
        raise FeatureConfigError(msg)

    # Basic structural validation: all names must be strings
    if not all(isinstance(n, str) and n for n in names):
        msg = f"Feature names for kind '{kind}' must be a list of non-empty strings in {path}."
        logger.error(msg)
        raise FeatureConfigError(msg)

    return names


# Canonical raw windowed stat names (v2): contiguous block in config from *_mean_w3 to *_innings_in_last_90d.
_RAW_START = "mean_w3"
_RAW_END = "innings_in_last_90d"


def _raw_stat_prefix(kind: str) -> str:
    if kind == "batting":
        return "batting_"
    if kind == "bowling":
        return "bowling_"
    raise ValueError(f"unknown feature kind for raw stats: {kind}")


@lru_cache(maxsize=None)
def get_raw_stat_feature_names(kind: str) -> List[str]:
    """Return raw windowed stat feature names from configs/feature_vectors.json.

    Derives the list by taking the contiguous block from <kind>_mean_w3 through
    <kind>_innings_in_last_90d in the config, so a single source of truth is kept.
    """
    kind = kind.lower().strip()
    prefix = _raw_stat_prefix(kind)
    start_marker = prefix + _RAW_START
    end_marker = prefix + _RAW_END

    names = get_feature_names(kind)
    try:
        i = names.index(start_marker)
        j = names.index(end_marker)
    except ValueError as e:
        msg = (
            f"Raw stat markers {start_marker!r} / {end_marker!r} not found in {kind} list from config. "
            "Ensure configs/feature_vectors.json has the v2 raw windowed stat block."
        )
        logger.error(msg)
        raise FeatureConfigError(msg) from e

    if j < i:
        msg = f"Raw stat block for {kind}: {end_marker!r} must appear after {start_marker!r} in config."
        logger.error(msg)
        raise FeatureConfigError(msg)

    return names[i : j + 1]
