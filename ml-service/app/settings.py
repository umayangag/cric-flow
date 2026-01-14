"""Resolve runtime directories and configuration for model artifacts.

This module exposes helpers to determine where model files are loaded from.
Precedence for models directory:
  1) Environment variable `ML_SERVICE_OUTPUT_DIR`
  2) Environment variable `MODELS_DIR`
  3) Optional `svc_config.default_artifacts_dir()` when provided
  4) Built-in fallback: `../../output/ml-service`
"""

import os
import os.path as osp
from typing import Any, Optional


def _default_models_dir_from_config(svc_config: Optional[Any]) -> str:
    if svc_config is not None:
        try:
            return svc_config.default_artifacts_dir()  # type: ignore[attr-defined]
        except Exception:
            pass
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
