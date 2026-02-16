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


# Required keys per model under ml.training.<model>; all training scripts use these strictly (no magic defaults).
TRAINING_REQUIRED_KEYS = ("n_estimators", "max_depth", "random_state", "joblib_compress")

# Models that have their own training block in config (ml.training.batting, ml.training.bowling, etc.).
TRAINING_MODELS = ("batting", "bowling", "fielding", "extras", "win")


def get_training_params(model: str) -> Dict[str, Any]:
    """
    Load ML training parameters from config for the given model (ml.training.<model>).
    All values must be set in config; no defaults or env overrides.
    Raises ValueError if config is missing or any required key is absent.
    model: one of "batting", "bowling". Used by train_batting*, train_bowling*, train_on_the_fly.
    """
    if model not in TRAINING_MODELS:
        raise ValueError(
            f"Unknown model {model!r}. Must be one of: {', '.join(TRAINING_MODELS)}."
        )
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    if not isinstance(ml, dict):
        raise ValueError(
            "config.json must define 'ml'. Add ml.training.batting and ml.training.bowling with "
            "n_estimators, max_depth, random_state, joblib_compress."
        )
    training = ml.get("training")
    if not isinstance(training, dict):
        raise ValueError(
            "config.json must define 'ml.training' with per-model blocks (batting, bowling)."
        )
    block = training.get(model)
    if not isinstance(block, dict):
        raise ValueError(
            f"config.json must define 'ml.training.{model}' with keys: "
            + ", ".join(TRAINING_REQUIRED_KEYS)
        )
    missing = [k for k in TRAINING_REQUIRED_KEYS if k not in block]
    if missing:
        raise ValueError(
            f"ml.training.{model} is missing required keys: " + ", ".join(missing) + ". Set them in config.json."
        )
    n_estimators = block["n_estimators"]
    max_depth = block["max_depth"]
    random_state = block["random_state"]
    joblib_compress = block["joblib_compress"]
    try:
        n_estimators = int(n_estimators)
        max_depth = int(max_depth)
        random_state = int(random_state)
        joblib_compress = int(joblib_compress)
    except (TypeError, ValueError) as e:
        raise ValueError(
            f"ml.training.{model} values must be integers: n_estimators, max_depth, random_state, joblib_compress."
        ) from e
    if joblib_compress < 0 or joblib_compress > 9:
        raise ValueError(f"ml.training.{model}.joblib_compress must be between 0 and 9.")
    return {
        "n_estimators": n_estimators,
        "max_depth": max_depth,
        "random_state": random_state,
        "joblib_compress": joblib_compress,
    }


def get_tuning_config() -> Dict[str, Any]:
    """
    Load tuning config from ml.tuning (optional). Used by auto_tune.
    Returns: cv_splits, n_iter, scoring. Missing keys get defaults.
    """
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    tuning = ml.get("tuning") if isinstance(ml, dict) else None
    if not isinstance(tuning, dict):
        return {"cv_splits": 5, "n_iter": 25, "scoring": "neg_mean_absolute_error"}
    return {
        "cv_splits": int(tuning.get("cv_splits", 5)),
        "n_iter": int(tuning.get("n_iter", 25)),
        "scoring": str(tuning.get("scoring", "neg_mean_absolute_error")),
    }


# Built-in feature defaults when building feature vectors from a sparse go-app map (prediction time).
# Used by app/backtest_service build_*_features_from_map when a key is missing.
_BUILTIN_FEATURE_DEFAULTS = {
    "common": {
        "temp": 25,
        "humidity": 50,
        "wind": 0,
        "rain": 0,
        "cloud": 0,
        "pressure": 0,
        "viscosity": 0,
        "inning": 1,
        "session": 1,
        "toss": 0,
    },
    "fielding": {
        "consistency": 0.5,
        "form": 0.0,
        "venue": 0.5,
        "opposition": 0.5,
    },
}


def get_feature_defaults() -> Dict[str, Any]:
    """
    Load feature_defaults from ml.feature_defaults (optional). Used when building
    batting/bowling/fielding feature vectors from a sparse map; missing keys get
    these defaults so prediction can proceed without failing.
    Returns: dict with "common" (weather/context) and "fielding" (fielding-specific) sub-dicts.
    """
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    fd = ml.get("feature_defaults") if isinstance(ml, dict) else None
    if not isinstance(fd, dict):
        return _BUILTIN_FEATURE_DEFAULTS.copy()
    result: Dict[str, Any] = {"common": {}, "fielding": {}}
    builtin_common = _BUILTIN_FEATURE_DEFAULTS["common"]
    builtin_fielding = _BUILTIN_FEATURE_DEFAULTS["fielding"]
    for k, v in builtin_common.items():
        result["common"][k] = fd.get("common", {}).get(k, v) if isinstance(fd.get("common"), dict) else v
    for k, v in builtin_fielding.items():
        result["fielding"][k] = fd.get("fielding", {}).get(k, v) if isinstance(fd.get("fielding"), dict) else v
    return result
