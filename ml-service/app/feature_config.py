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
