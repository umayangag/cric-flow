import json
import os
from typing import Any, Dict

# Simple JSON config loader for ml-service
# Precedence elsewhere should be: flag/arg > env > config.json > built-in defaults

_cached: Dict[str, Any] | None = None


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
    candidates.extend([
        os.path.join(os.getcwd(), "config.json"),
        os.path.abspath(os.path.join(os.getcwd(), "..", "config.json")),
    ])
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
    return os.path.join("..", "..", "output", "go-app")


def default_artifacts_dir() -> str:
    cfg = _load()
    try:
        val = cfg.get("outputs", {}).get("artifacts_dir")
        if val:
            return val
    except Exception:
        pass
    # built-in fallback
    return os.path.join("..", "..", "output", "ml-service")
