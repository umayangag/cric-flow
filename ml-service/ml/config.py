import json
import os
from typing import Any, Dict, Optional

# Simple JSON config loader for ml-service
# Precedence elsewhere should be: flag/arg > env > config.json > built-in defaults

_cached: Optional[Dict[str, Any]] = None


def _load() -> Dict[str, Any]:
    global _cached
    if _cached is not None:
        return _cached
    cfg: Dict[str, Any] = {}
    # Search order:
    # 1) ML_SERVICE_CONFIG env var path
    # 2) ./config.json (project root when running from ml-service)
    # 3) ../config.json (when running from a subpackage like ml/)
    candidates = []
    env_path = os.environ.get("ML_SERVICE_CONFIG")
    if env_path:
        candidates.append(env_path)
    candidates.extend(
        [
            os.path.join(os.getcwd(), "config.json"),
            os.path.abspath(os.path.join(os.getcwd(), "..", "config.json")),
        ]
    )
    for p in candidates:
        try:
            with open(p, "r", encoding="utf-8") as f:
                cfg = json.load(f)
                _cached = cfg
                return cfg
        except Exception:
            continue
    _cached = cfg
    return cfg


def default_go_app_export_dir() -> str:
    cfg = _load()
    try:
        val = cfg.get("inputs", {}).get("go_app_export_dir")
        if val:
            return val
    except Exception:
        pass
    # built-in fallback
    return os.path.join("../..", "..", "output", "go-app")


def default_artifacts_dir() -> str:
    cfg = _load()
    try:
        val = cfg.get("outputs", {}).get("artifacts_dir")
        if val:
            return val
    except Exception:
        pass
    # built-in fallback
    return os.path.join("../..", "..", "output", "ml-service")


# Required keys under ml.training; all training scripts use these strictly (no magic defaults).
TRAINING_REQUIRED_KEYS = ("n_estimators", "max_depth", "random_state", "joblib_compress")


def get_training_params() -> Dict[str, Any]:
    """
    Load ML training parameters from config (ml.training). All values must be set in config;
    no defaults or env overrides. Raises ValueError if config is missing or any required key is absent.
    Used by train_batting_model, train_bowling_model, train_batting, train_bowling, train_on_the_fly.
    """
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    if not isinstance(ml, dict):
        raise ValueError(
            "config.json must define 'ml'. Add ml.training with n_estimators, max_depth, random_state, joblib_compress."
        )
    training = ml.get("training")
    if not isinstance(training, dict):
        raise ValueError(
            "config.json must define 'ml.training' with keys: "
            + ", ".join(TRAINING_REQUIRED_KEYS)
            + ". Used for model training and artifact serialization."
        )
    missing = [k for k in TRAINING_REQUIRED_KEYS if k not in training]
    if missing:
        raise ValueError(
            "ml.training is missing required keys: " + ", ".join(missing) + ". Set them in config.json."
        )
    n_estimators = training["n_estimators"]
    max_depth = training["max_depth"]
    random_state = training["random_state"]
    joblib_compress = training["joblib_compress"]
    try:
        n_estimators = int(n_estimators)
        max_depth = int(max_depth)
        random_state = int(random_state)
        joblib_compress = int(joblib_compress)
    except (TypeError, ValueError) as e:
        raise ValueError(
            "ml.training values must be integers: n_estimators, max_depth, random_state, joblib_compress."
        ) from e
    if joblib_compress < 0 or joblib_compress > 9:
        raise ValueError("ml.training.joblib_compress must be between 0 and 9.")
    return {
        "n_estimators": n_estimators,
        "max_depth": max_depth,
        "random_state": random_state,
        "joblib_compress": joblib_compress,
    }
