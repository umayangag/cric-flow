import json
import logging
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, Optional

from ml.resources import suggested_n_jobs

logger = logging.getLogger(__name__)

# Simple JSON config loader for ml-service.
# Precedence: flag/arg > env > config.json (merged over config.default.json) > config.default.json only.
# Defaults when config is missing or invalid are defined below; same values are in config.default.json.
DEFAULT_TRAINING_DATA_FETCH_TIMEOUT_SEC = 3600
DEFAULT_TRAINING_DATA_FETCH_TIMEOUT_INVALID_FALLBACK_SEC = 600
DEFAULT_GO_APP_REQUEST_TIMEOUT_SEC = 30

_cached: Optional[Dict[str, Any]] = None

_CONFIG_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


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
    """Return the merged config (same as internal _load). Used by resources and other modules."""
    return _load()


def default_go_app_export_dir() -> str:
    """Return go_app_export_dir from config (inputs.go_app_export_dir)."""
    cfg = _load()
    val = (cfg.get("inputs") or {}).get("go_app_export_dir")
    return str(val) if val else os.path.join("..", "..", "output", "go-app")


def default_artifacts_dir() -> str:
    """Return artifacts_dir from config (outputs.artifacts_dir)."""
    cfg = _load()
    val = (cfg.get("outputs") or {}).get("artifacts_dir")
    return str(val) if val else os.path.join("..", "..", "output", "ml-service")


def get_training_data_fetch_timeout_sec() -> int:
    """Return timeout in seconds for fetching training data from go-app (inputs.training_data_fetch_timeout_sec)."""
    cfg = _load()
    inputs = cfg.get("inputs") or {}
    val = inputs.get("training_data_fetch_timeout_sec", DEFAULT_TRAINING_DATA_FETCH_TIMEOUT_SEC)
    invalid_fallback = inputs.get(
        "training_data_fetch_timeout_invalid_fallback_sec", DEFAULT_TRAINING_DATA_FETCH_TIMEOUT_INVALID_FALLBACK_SEC
    )
    try:
        return int(val)
    except (TypeError, ValueError):
        return (
            int(invalid_fallback)
            if isinstance(invalid_fallback, (int, float))
            else DEFAULT_TRAINING_DATA_FETCH_TIMEOUT_INVALID_FALLBACK_SEC
        )


# Required keys per model under ml.training.<model>; all training scripts use these strictly (no magic defaults).
TRAINING_REQUIRED_KEYS = ("n_estimators", "max_depth", "random_state", "joblib_compress")

# Models that have their own training block in config (ml.training.batting, ml.training.bowling, etc.).
TRAINING_MODELS = ("batting", "bowling", "fielding", "extras", "win")


def _go_app_request_timeout_sec() -> int:
    """Return timeout in seconds for go-app HTTP requests (inputs.go_app_request_timeout_sec)."""
    cfg = _load()
    val = (cfg.get("inputs") or {}).get("go_app_request_timeout_sec", DEFAULT_GO_APP_REQUEST_TIMEOUT_SEC)
    try:
        return int(val) if val else DEFAULT_GO_APP_REQUEST_TIMEOUT_SEC
    except (TypeError, ValueError):
        return DEFAULT_GO_APP_REQUEST_TIMEOUT_SEC


def get_tuned_params_from_go_app(
    go_app_url: str, model: str, format_code: str, api_key: Optional[str] = None
) -> Optional[Dict[str, Any]]:
    """Fetch latest tuned params for model+format from go-app. Returns None on 404 or error."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/ml/tuned-params?model={urllib.parse.quote(model)}&format={urllib.parse.quote(format_code)}"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=_go_app_request_timeout_sec()) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None
        logger.warning("config.get_tuned_params_from_go_app.http_error url=%s code=%s", url, e.code)
        return None
    except OSError as e:
        logger.warning("config.get_tuned_params_from_go_app.request_failed url=%s error=%s", url, e)
        return None
    params = data.get("params")
    if isinstance(params, dict):
        return params
    if isinstance(params, str):
        try:
            return json.loads(params)
        except json.JSONDecodeError:
            return None
    return None


def save_tuned_params_to_go_app(
    go_app_url: str,
    model: str,
    format_code: str,
    params: Dict[str, Any],
    api_key: Optional[str] = None,
) -> None:
    """POST tuned params to go-app so they are stored in the DB for future training."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/ml/tuned-params"
    payload = json.dumps({"model": model, "format": format_code, "params": params}).encode("utf-8")
    req = urllib.request.Request(url, data=payload, method="POST")
    req.add_header("Content-Type", "application/json")
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=_go_app_request_timeout_sec()) as resp:
            if 200 <= resp.status < 300:
                logger.info("config.save_tuned_params_to_go_app.saved model=%s format=%s", model, format_code)
            return
    except urllib.error.HTTPError as e:
        logger.warning("config.save_tuned_params_to_go_app.http_error url=%s code=%s body=%s", url, e.code, e.read())
        raise ValueError(f"go-app tuned-params POST failed: HTTP {e.code}") from e
    except OSError as e:
        logger.warning("config.save_tuned_params_to_go_app.request_failed url=%s error=%s", url, e)
        raise ValueError(f"go-app tuned-params request failed: {e}") from e


def get_training_params(model: str, format_code: Optional[str] = None) -> Dict[str, Any]:
    """
    Load ML training parameters from config for the given model (ml.training.<model>).
    If format_code is set and GO_APP_URL is set, fetches latest tuned params from go-app and
    merges them over config (DB params override config). Falls back to config only when no
    tuned params exist for that model+format.
    Raises ValueError if config is missing or any required key is absent.
    """
    if model not in TRAINING_MODELS:
        logger.error("config.get_training_params.unknown_model model=%s allowed=%s", model, TRAINING_MODELS)
        raise ValueError(f"Unknown model {model!r}. Must be one of: {', '.join(TRAINING_MODELS)}.")
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    if not isinstance(ml, dict):
        logger.error("config.get_training_params.missing_ml_block model=%s", model)
        raise ValueError(
            "config.json must define 'ml'. Add ml.training.batting and ml.training.bowling with "
            "n_estimators, max_depth, random_state, joblib_compress."
        )
    training = ml.get("training")
    if not isinstance(training, dict):
        logger.error("config.get_training_params.missing_training_block model=%s", model)
        raise ValueError("config.json must define 'ml.training' with per-model blocks (batting, bowling).")
    block = dict(training.get(model) or {})
    if not isinstance(training.get(model), dict):
        logger.error("config.get_training_params.missing_model_block model=%s", model)
        raise ValueError(
            f"config.json must define 'ml.training.{model}' with keys: " + ", ".join(TRAINING_REQUIRED_KEYS)
        )
    # Overlay latest tuned params from go-app when available
    go_app_url = os.environ.get("GO_APP_URL", "").strip()
    if format_code is not None and go_app_url:
        overlay = get_tuned_params_from_go_app(go_app_url, model, format_code or "", os.environ.get("GO_APP_API_KEY"))
        if overlay:
            block = _deep_merge(block, overlay)
    missing = [k for k in TRAINING_REQUIRED_KEYS if k not in block]
    if missing:
        logger.error("config.get_training_params.missing_keys model=%s missing=%s", model, missing)
        raise ValueError(
            f"ml.training.{model} is missing required keys: " + ", ".join(missing) + ". Set them in config.json."
        )
    # Optional: n_jobs for RandomForest (default -1 = resource-aware)
    n_jobs = block.get("n_jobs", -1)
    if n_jobs == -1:
        n_jobs = suggested_n_jobs("training")
    result = dict(block)
    result["n_jobs"] = int(n_jobs)
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
        logger.error("config.get_training_params.invalid_types model=%s error=%s", model, e)
        raise ValueError(
            f"ml.training.{model} values must be integers: n_estimators, max_depth, random_state, joblib_compress."
        ) from e
    if joblib_compress < 0 or joblib_compress > 9:
        logger.error(
            "config.get_training_params.invalid_joblib_compress model=%s joblib_compress=%s", model, joblib_compress
        )
        raise ValueError(f"ml.training.{model}.joblib_compress must be between 0 and 9.")
    estimator = (block.get("estimator") or "rf").strip().lower()
    if estimator in ("gb", "gbm", "gradient_boosting"):
        estimator = "gb"
    elif estimator in ("stacked", "stacking", "ensemble"):
        estimator = "stacked"
    elif estimator in ("quantile", "qr"):
        estimator = "quantile"
    elif estimator not in ("rf", "random_forest"):
        estimator = "rf"
    learning_rate = block.get("learning_rate", 0.1)
    try:
        learning_rate = float(learning_rate)
    except (TypeError, ValueError):
        learning_rate = 0.1
    quantile_level = block.get("quantile_level", 0.5)
    try:
        quantile_level = float(quantile_level)
    except (TypeError, ValueError):
        quantile_level = 0.5
    quantile_level = max(0.01, min(0.99, quantile_level))
    return {
        "n_estimators": n_estimators,
        "max_depth": max_depth,
        "random_state": random_state,
        "joblib_compress": joblib_compress,
        "n_jobs": result["n_jobs"],
        "estimator": estimator,
        "learning_rate": learning_rate,
        "quantile_level": quantile_level,
    }


def get_tuning_config() -> Dict[str, Any]:
    """
    Load tuning config from ml.tuning. Used by auto_tune.
    All values come from config (config.default.json or user config.json).
    """
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    tuning = (ml.get("tuning") if isinstance(ml, dict) else None) or {}
    n_jobs = tuning.get("n_jobs", -1)
    if n_jobs == -1:
        n_jobs = suggested_n_jobs("tuning")
    return {
        "cv_splits": int(tuning.get("cv_splits", 5)),
        "n_iter": int(tuning.get("n_iter", 25)),
        "n_jobs": int(n_jobs),
        "random_state": int(tuning.get("random_state", 42)),
        "scoring": str(tuning.get("scoring", "neg_mean_absolute_error")),
        "search_space": tuning.get("search_space"),
    }


def get_tuning_search_space(estimator_key: str) -> Optional[Dict[str, Any]]:
    """Return search space for auto_tune from ml.tuning.search_space.<rf|gb>. None if not configured.
    estimator_key: 'rf' for RandomForest, 'gb' for GradientBoosting.
    """
    cfg = get_tuning_config()
    space = cfg.get("search_space")
    if not isinstance(space, dict):
        return None
    return space.get(estimator_key) if isinstance(space.get(estimator_key), dict) else None


def get_feature_defaults() -> Dict[str, Any]:
    """
    Load feature_defaults from ml.feature_defaults. Used when building
    batting/bowling/fielding feature vectors from a sparse map; missing keys get these defaults.
    All values come from config (config.default.json or user config.json).
    """
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    fd = (ml.get("feature_defaults") if isinstance(ml, dict) else None) or {}
    common = fd.get("common") if isinstance(fd.get("common"), dict) else {}
    fielding = fd.get("fielding") if isinstance(fd.get("fielding"), dict) else {}
    defaults_common = {
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
    }
    defaults_fielding = {"consistency": 0.5, "form": 0.0, "venue": 0.5, "opposition": 0.5}
    return {
        "common": {k: common.get(k, v) for k, v in defaults_common.items()},
        "fielding": {k: fielding.get(k, v) for k, v in defaults_fielding.items()},
    }


def get_prediction_defaults() -> Dict[str, Any]:
    """Load prediction defaults (e.g. economy when missing) from ml.prediction_defaults."""
    cfg = _load()
    ml = cfg.get("ml") if isinstance(cfg, dict) else None
    pd_def = (ml.get("prediction_defaults") if isinstance(ml, dict) else None) or {}
    return {"economy": float(pd_def.get("economy", 6.0))}
