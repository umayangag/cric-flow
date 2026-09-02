import json
import logging
import os
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)

# Simple JSON config loader for ml-service.
# Precedence: flag/arg > env > config.json (merged over config.default.json) > config.default.json only.
# Defaults when config is missing or invalid are defined below; same values are in config.default.json.

# Default for the training subprocess (`/admin/train/*`): 7 days.
DEFAULT_TRAINING_SUBPROCESS_TIMEOUT_SEC = 7 * 24 * 3600  # 604800

# H-11: a prediction against ratings older than this fails rather than answering. Fourteen
# days is roughly two cricket weeks -- long enough that a box between imports is not
# nagged, short enough that a rating state nobody has refreshed cannot quietly serve.
DEFAULT_RATINGS_MAX_AGE_DAYS = 14

_CONFIG_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

_cached: Optional[Dict[str, Any]] = None


def _find_user_config_path() -> Optional[str]:
    """Return path to user config file. Precedence: ML_SERVICE_CONFIG env, then config.json in cwd/parent/ml-service dir."""
    env_path = os.environ.get("ML_SERVICE_CONFIG")
    if env_path and os.path.isfile(env_path):
        return env_path
    for base in [os.getcwd(), os.path.abspath(os.path.join(os.getcwd(), "..")), _CONFIG_DIR]:
        p = os.path.join(base, "config.json")
        if os.path.isfile(p):
            return p
    return None


def _load_json(path: str) -> Dict[str, Any]:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def _deep_merge(base: Dict[str, Any], override: Dict[str, Any]) -> Dict[str, Any]:
    """Merge override into base recursively. Override values take precedence."""
    out = dict(base)
    for k, v in override.items():
        if k in out and isinstance(out[k], dict) and isinstance(v, dict):
            out[k] = _deep_merge(out[k], v)
        else:
            out[k] = v
    return out


def _load() -> Dict[str, Any]:
    global _cached
    if _cached is not None:
        return _cached
    # Load default config first (always present next to config.py)
    default_path = os.path.join(_CONFIG_DIR, "config.default.json")
    try:
        cfg = _load_json(default_path) if os.path.isfile(default_path) else {}
    except Exception as e:
        logger.warning("config._load.default_failed path=%s error=%s", default_path, e)
        cfg = {}

    # Override with user config if found
    user_path = _find_user_config_path()
    if user_path and user_path != default_path:
        try:
            user_cfg = _load_json(user_path)
            cfg = _deep_merge(cfg, user_cfg)
        except Exception as e:
            logger.warning("config._load.user_failed path=%s error=%s", user_path, e)

    _cached = cfg
    return cfg


def get_config() -> Dict[str, Any]:
    """Return the merged config. Used by every other accessor here."""
    return _load()


def default_artifacts_dir() -> str:
    """Return artifacts_dir from config (outputs.artifacts_dir)."""
    cfg = _load()
    val = (cfg.get("outputs") or {}).get("artifacts_dir")
    return str(val) if val else os.path.join("..", "..", "output", "ml-service")


def get_training_subprocess_timeout_sec() -> int:
    """Return timeout in seconds for the training subprocess (inputs.training_subprocess_timeout_sec).
    Fallback: env TRAINING_SUBPROCESS_TIMEOUT_SEC, then 7 days."""
    cfg = _load()
    val = (cfg.get("inputs") or {}).get("training_subprocess_timeout_sec")
    if val is not None:
        try:
            return int(val)
        except (TypeError, ValueError):
            pass
    env_val = os.environ.get("TRAINING_SUBPROCESS_TIMEOUT_SEC")
    if env_val is not None:
        try:
            return int(env_val)
        except ValueError:
            pass
    return DEFAULT_TRAINING_SUBPROCESS_TIMEOUT_SEC


def get_ratings_max_age_days() -> int:
    """How old the loaded rating state may be before a live prediction is refused (H-11).

    ``ml.ratings_max_age_days`` in config, or ``XI_RATINGS_MAX_AGE_DAYS`` in the
    environment, which is what a deployment overrides. Zero or negative turns the check
    off, which is a decision an operator can make and see in the config rather than a
    state the code can drift into.
    """
    env_val = os.environ.get("XI_RATINGS_MAX_AGE_DAYS")
    if env_val is not None:
        try:
            return int(env_val)
        except ValueError:
            logger.warning("config.ratings_max_age_days.invalid_env value=%s", env_val)
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    val = (ml or {}).get("ratings_max_age_days", DEFAULT_RATINGS_MAX_AGE_DAYS)
    try:
        return int(val)
    except (TypeError, ValueError):
        return DEFAULT_RATINGS_MAX_AGE_DAYS


# Canonical cricket format codes. Mirrors go-app/internal/formats.CanonicalCodes();
# the two are kept in step by scripts/check-frontend-backend-sync.mjs via cmd/print_canonical.
CANONICAL_FORMAT_CODES: List[str] = ["TEST", "ODI", "T20", "T20I"]


def get_format_codes() -> List[str]:
    """Return the configured format codes, falling back to CANONICAL_FORMAT_CODES.

    Single source for every caller that previously kept its own copy of the list.
    """
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    fmts = (ml.get("formats") if isinstance(ml, dict) else None) or []
    out = [str(x).strip().upper() for x in fmts if isinstance(x, (str, int)) and str(x).strip()]
    return out or list(CANONICAL_FORMAT_CODES)
