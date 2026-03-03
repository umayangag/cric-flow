"""Resolve runtime directories and configuration for model artifacts and service behavior.

This module exposes helpers to determine where model files are loaded from and to
centralize process-wide configuration derived from environment variables.

Precedence for models directory:
  1) Environment variable `ML_SERVICE_OUTPUT_DIR`
  2) Environment variable `MODELS_DIR`
  3) Optional `svc_config.default_artifacts_dir()` when provided
  4) Built-in fallback: `../../output/ml-service`
"""

import os
import os.path as osp
from dataclasses import dataclass
from typing import Any, List, Optional


def _default_models_dir_from_config(svc_config: Optional[Any]) -> str:
    import logging

    _log = logging.getLogger(__name__)
    if svc_config is not None:
        try:
            return svc_config.default_artifacts_dir()  # type: ignore[attr-defined]
        except Exception as e:
            _log.warning("settings.default_artifacts_dir_fallback", error=str(e))
    # built-in fallback ../../output/ml-service (relative to this file)
    here = osp.dirname(__file__)
    return osp.abspath(osp.join(here, "..", "..", "output", "ml-service"))


def get_models_dir(svc_config: Optional[Any] = None) -> str:
    """Resolve models directory with precedence:
    1) ML_SERVICE_OUTPUT_DIR
    2) MODELS_DIR
    3) svc_config.default_artifacts_dir() if provided
    4) ../../output/ml-service
    """
    cfg_default = _default_models_dir_from_config(svc_config)
    return os.environ.get("ML_SERVICE_OUTPUT_DIR", os.environ.get("MODELS_DIR", cfg_default))


def _env_bool(name: str, default: bool = False) -> bool:
    """Parse a boolean environment variable using the shared semantics from app.main."""
    raw = (os.environ.get(name) or "").strip().lower()
    if not raw:
        return default
    return raw in {"1", "true", "yes"}


def _env_int(name: str, default: int) -> int:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return default
    try:
        return int(raw)
    except ValueError:
        return default


@dataclass
class MLServiceSettings:
    """Process-wide configuration for the ML service.

    This centralizes environment-driven behavior so new code does not reach into
    os.environ directly. Existing callers in app.main are wired through this type
    without changing external behavior.
    """

    enable_hot_reload: bool
    admin_api_key: str
    max_concurrent_training_jobs: int
    max_predict_batch_size: int
    disable_backtest_cache: bool
    backtest_cache_ttl_seconds: int
    train_latest_cache_granularity: str
    model_stats_cache_ttl: int
    go_app_url: str
    enable_train_on_the_fly: bool
    go_app_api_key: str
    frontend_origin_raw: str

    @property
    def frontend_allowed_origins(self) -> List[str]:
        return [o.strip() for o in self.frontend_origin_raw.split(",") if o.strip()]


def load_ml_service_settings() -> MLServiceSettings:
    """Load ML service configuration from environment variables.

    This is intentionally a simple, explicit loader instead of a dynamic
    BaseSettings-style model so tests can continue to control behavior by
    setting environment variables and re-importing app.main.
    """

    enable_hot_reload = _env_bool("ENABLE_HOT_RELOAD", default=False)
    admin_api_key = (os.environ.get("ADMIN_API_KEY") or "").strip()
    max_concurrent_training_jobs = max(1, _env_int("MAX_CONCURRENT_TRAINING_JOBS", default=1))
    max_predict_batch_size = _env_int("MAX_PREDICT_BATCH_SIZE", default=10000)
    disable_backtest_cache = _env_bool("DISABLE_BACKTEST_CACHE", default=False)
    backtest_cache_ttl_seconds = _env_int("BACKTEST_CACHE_TTL", default=300)
    train_latest_cache_granularity = (
        (os.environ.get("TRAIN_ON_THE_FLY_LATEST_CACHE_GRANULARITY") or "hour").strip().lower()
    )
    model_stats_cache_ttl = _env_int("MODEL_STATS_CACHE_TTL", default=60)
    go_app_url = (os.environ.get("GO_APP_URL") or "").strip() or "http://localhost:8080"
    enable_train_on_the_fly = _env_bool("ENABLE_TRAIN_ON_THE_FLY", default=False)
    go_app_api_key = (os.environ.get("GO_APP_API_KEY") or "").strip()
    frontend_origin_raw = os.environ.get("FRONTEND_ORIGIN", "http://localhost:5173")

    return MLServiceSettings(
        enable_hot_reload=enable_hot_reload,
        admin_api_key=admin_api_key,
        max_concurrent_training_jobs=max_concurrent_training_jobs,
        max_predict_batch_size=max_predict_batch_size,
        disable_backtest_cache=disable_backtest_cache,
        backtest_cache_ttl_seconds=backtest_cache_ttl_seconds,
        train_latest_cache_granularity=train_latest_cache_granularity,
        model_stats_cache_ttl=model_stats_cache_ttl,
        go_app_url=go_app_url,
        enable_train_on_the_fly=enable_train_on_the_fly,
        go_app_api_key=go_app_api_key,
        frontend_origin_raw=frontend_origin_raw,
    )
