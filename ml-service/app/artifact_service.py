"""Artifact discovery and status reporting for the ML service.

Extracted from app.main. Provides:
- Per-format artifact file discovery
- Artifact status endpoint logic (formats × kinds matrix)
- Health endpoint artifact/metadata info helpers
"""

import os
import time
from typing import Any, Dict, List, Optional, Tuple

from .artifacts import (
    BAT_MODELS,
    BOWL_MODELS,
    EXTRAS_MODELS,
    FIELD_MODELS,
    WIN_MODELS,
)
from .logging import get_struct_logger

logger = get_struct_logger()

SUPPORTED_FORMATS = ["TEST", "ODI", "T20I", "T20"]
ARTIFACT_KINDS = ["batting", "bowling", "fielding", "extras", "win"]


def find_per_format_artifact(
    models_dir: str,
    fmt: str,
    kind: str,
) -> Optional[Tuple[str, float]]:
    """Return (path, mtime) for per-format artifact of given kind.

    kind in: batting, bowling, fielding, extras, win.
    Batting/bowling/fielding require both scaler and model.
    """
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.debug(
            "artifacts_status.find_per_format.listdir_failed",
            models_dir=models_dir,
            fmt=fmt,
            kind=kind,
            error=str(e),
        )
        return None
    if kind == "batting":
        scaler_name = f"batting_scaler_{fmt}.joblib"
        model_name = f"batting_model_{fmt}.joblib"
    elif kind == "bowling":
        scaler_name = f"bowling_scaler_{fmt}.joblib"
        model_name = f"bowling_model_{fmt}.joblib"
    elif kind == "fielding":
        scaler_name = f"fielding_scaler_{fmt}.joblib"
        model_name = f"fielding_model_{fmt}.joblib"
    elif kind == "extras":
        model_name = f"extras_model_{fmt}.joblib"
        scaler_name = None
    elif kind == "win":
        model_name = f"win_model_{fmt}.joblib"
        scaler_name = None
    else:
        return None
    if scaler_name and scaler_name not in entries:
        return None
    if model_name not in entries:
        return None
    path = os.path.join(models_dir, model_name)
    try:
        st = os.stat(path)
        if not os.path.isfile(path):
            return None
        return path, st.st_mtime
    except Exception:
        return None


def find_artifact(models_dir: str, fmt: str, batting: bool) -> Optional[Tuple[str, float]]:
    """Return (path, mtime) for the first matching artifact if found."""
    return find_per_format_artifact(models_dir, fmt, "batting" if batting else "bowling")


def build_artifacts_status(models_dir: str) -> Dict[str, Any]:
    """Build the full artifacts status response (formats × kinds)."""
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    loaded_registries: Dict[str, Any] = {
        "batting": BAT_MODELS,
        "bowling": BOWL_MODELS,
        "fielding": FIELD_MODELS,
        "extras": EXTRAS_MODELS,
        "win": WIN_MODELS,
    }
    formats_out: Dict[str, Dict[str, Any]] = {}
    for fmt in SUPPORTED_FORMATS:
        row: Dict[str, Dict[str, Any]] = {}
        for kind in ARTIFACT_KINDS:
            obj: Dict[str, Any] = {"exists": False}
            hit = find_per_format_artifact(models_dir, fmt, kind)
            if hit is not None:
                p, mt = hit
                obj["exists"] = True
                obj["path"] = p
                obj["modified"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mt))
            try:
                reg = loaded_registries.get(kind)
                if reg is not None and fmt in reg:
                    obj["loaded"] = True
            except Exception as e:
                logger.debug("artifacts_status.loaded_check", fmt=fmt, kind=kind, error=str(e))
            row[kind] = obj
        formats_out[fmt] = row

    return {"timestamp": ts, "root": models_dir, "formats": formats_out}


def build_health_response(models_dir: str) -> Dict[str, Any]:
    """Build the /health endpoint response with artifact and metadata info."""
    model_registries = {
        "batting": BAT_MODELS,
        "bowling": BOWL_MODELS,
        "fielding": FIELD_MODELS,
        "extras": EXTRAS_MODELS,
        "win": WIN_MODELS,
    }
    response: dict = {
        "status": "ok",
        "models_dir": models_dir,
        "artifacts": {},
        "metadata": {},
        "counters": {},
    }
    for name, registry in model_registries.items():
        loaded = sorted(registry.keys())
        response[f"loaded_{name}_formats"] = loaded
        response["artifacts"][name] = _artifacts_info(models_dir, f"{name}_")
        if name in ("batting", "bowling", "fielding"):
            response["metadata"][name] = _metadata_info(models_dir, f"{name}_metadata_")
        response["counters"][f"{name}_formats"] = len(loaded)
    return response


def _artifacts_info(models_dir: str, prefix: str) -> List[dict]:
    """List .joblib artifact files matching prefix with size and mtime."""
    out = []
    try:
        for fname in os.listdir(models_dir):
            if fname.lower().startswith(prefix) and fname.lower().endswith(".joblib"):
                fpath = os.path.join(models_dir, fname)
                try:
                    st = os.stat(fpath)
                    out.append(
                        {
                            "file": fname,
                            "size_bytes": st.st_size,
                            "modified": int(st.st_mtime),
                        }
                    )
                except OSError as e:
                    logger.warning("health.artifacts_info.stat_failed", file=fname, error=str(e))
                    out.append({"file": fname})
    except OSError as e:
        logger.warning("health.artifacts_info.listdir_failed", models_dir=models_dir, error=str(e))
    return sorted(out, key=lambda x: x.get("file", ""))


def _metadata_info(models_dir: str, prefix: str) -> List[str]:
    """List .json metadata files matching prefix."""
    names: List[str] = []
    try:
        for fname in os.listdir(models_dir):
            lf = fname.lower()
            if lf.startswith(prefix) and lf.endswith(".json"):
                names.append(fname)
    except OSError as e:
        logger.warning("health.metadata_info.listdir_failed", models_dir=models_dir, error=str(e))
    return sorted(names)
