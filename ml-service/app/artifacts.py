"""Model artifact registry and loader utilities.

This module keeps in-memory registries mapping a format code (e.g., "T20") to
the tuple (scaler, model). Artifacts are always per-format, with filenames like
`batting_scaler_T20.joblib` and `batting_model_T20.joblib`, discovered under a
configured models directory.

Use `reload(models_dir)` to (re)scan a directory and populate the registries.
The lightweight `summary()` function returns a snapshot indicating which
formats are currently loaded.
"""

import os
from typing import Any, Dict, Optional, Tuple

import joblib

from app.logging import get_struct_logger

logger = get_struct_logger()

# Registries: map format code -> (scaler, model).
BAT_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
FIELD_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
EXTRAS_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level extras regressor)
WIN_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level win classifier)
INNINGS_MODELS: Dict[
    str, Tuple[Optional[object], Optional[object]]
] = {}  # format -> (scaler, model) for innings runs/wickets
# Phase 3 share models: predict runs_share, wickets_share; multiply by innings totals for consistency
BAT_SHARE_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_SHARE_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}

# Sidecar metadata per (kind, format). See ml.artifact_sidecar. Keyed by the same format code
# convention (e.g. "T20") so prediction code can look up alongside the model.
INNINGS_META: Dict[str, Dict[str, Any]] = {}
EXTRAS_META: Dict[str, Dict[str, Any]] = {}


def _load_meta(models_dir: str, kind: str, format_code: Optional[str]) -> Optional[Dict[str, Any]]:
    """Best-effort sidecar load; returns ``None`` when the ml package is unavailable."""
    try:
        from ml.artifact_sidecar import read_artifact_meta
    except ImportError:
        return None
    return read_artifact_meta(models_dir, kind, format_code)


def _use_share_models() -> bool:
    """True if ml.use_share_models is enabled in config."""
    try:
        from ml.config import get_config

        return bool((get_config().get("ml") or {}).get("use_share_models"))
    except Exception:
        return False


def _load_per_format(models_dir: str) -> None:
    """Load per-format joblib artifacts.

    Security: joblib uses pickle; only load artifacts from trusted sources and restrict
    filesystem access to models_dir to avoid insecure deserialization.
    """
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.error("artifacts.load_per_format.listdir_failed", models_dir=models_dir, error=str(e))
        return
    for fname in entries:
        lf = fname.lower()
        try:
            if lf.startswith("batting_scaler_") and lf.endswith(".joblib"):
                code = fname[len("batting_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"batting_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BAT_MODELS[code] = (scaler, model)
                    logger.info(
                        "artifacts.load_per_format.batting",
                        format=code,
                        artifact_type="batting",
                        models_dir=models_dir,
                    )
                else:
                    logger.warning(
                        "artifacts.load_per_format.batting_model_missing",
                        format=code,
                        artifact_type="batting",
                        path=mpath,
                    )
            if lf.startswith("bowling_scaler_") and lf.endswith(".joblib") and not lf.startswith("bowling_share_"):
                code = fname[len("bowling_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"bowling_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BOWL_MODELS[code] = (scaler, model)
                    logger.info(
                        "artifacts.load_per_format.bowling",
                        format=code,
                        artifact_type="bowling",
                        models_dir=models_dir,
                    )
                else:
                    logger.warning(
                        "artifacts.load_per_format.bowling_model_missing",
                        format=code,
                        artifact_type="bowling",
                        path=mpath,
                    )
            if lf.startswith("batting_share_scaler_") and lf.endswith(".joblib"):
                code = fname[len("batting_share_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"batting_share_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BAT_SHARE_MODELS[code] = (scaler, model)
                    logger.info(
                        "artifacts.load_per_format.batting_share",
                        format=code,
                        artifact_type="batting_share",
                        models_dir=models_dir,
                    )
                else:
                    logger.warning(
                        "artifacts.load_per_format.batting_share_model_missing",
                        format=code,
                        artifact_type="batting_share",
                        path=mpath,
                    )
            if lf.startswith("bowling_share_scaler_") and lf.endswith(".joblib"):
                code = fname[len("bowling_share_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"bowling_share_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BOWL_SHARE_MODELS[code] = (scaler, model)
                    logger.info(
                        "artifacts.load_per_format.bowling_share",
                        format=code,
                        artifact_type="bowling_share",
                        models_dir=models_dir,
                    )
                else:
                    logger.warning(
                        "artifacts.load_per_format.bowling_share_model_missing",
                        format=code,
                        artifact_type="bowling_share",
                        path=mpath,
                    )
            if lf.startswith("fielding_scaler_") and lf.endswith(".joblib"):
                code = fname[len("fielding_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"fielding_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    FIELD_MODELS[code] = (scaler, model)
                    logger.info(
                        "artifacts.load_per_format.fielding",
                        format=code,
                        artifact_type="fielding",
                        models_dir=models_dir,
                    )
                else:
                    logger.warning(
                        "artifacts.load_per_format.fielding_model_missing",
                        format=code,
                        artifact_type="fielding",
                        path=mpath,
                    )
            if lf.startswith("extras_model_") and lf.endswith(".joblib"):
                code = fname[len("extras_model_") : -len(".joblib")].upper()
                model = joblib.load(os.path.join(models_dir, fname))
                EXTRAS_MODELS[code] = model
                meta = _load_meta(models_dir, "extras", code)
                if meta is not None:
                    EXTRAS_META[code] = meta
                logger.info(
                    "artifacts.load_per_format.extras",
                    format=code,
                    artifact_type="extras",
                    models_dir=models_dir,
                )
            if lf.startswith("win_model_") and lf.endswith(".joblib"):
                code = fname[len("win_model_") : -len(".joblib")].upper()
                model = joblib.load(os.path.join(models_dir, fname))
                WIN_MODELS[code] = model
                logger.info(
                    "artifacts.load_per_format.win",
                    format=code,
                    artifact_type="win",
                    models_dir=models_dir,
                )
            if lf.startswith("innings_scaler_") and lf.endswith(".joblib"):
                code = fname[len("innings_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"innings_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    INNINGS_MODELS[code] = (scaler, model)
                    meta = _load_meta(models_dir, "innings", code)
                    if meta is not None:
                        INNINGS_META[code] = meta
                    logger.info(
                        "artifacts.load_per_format.innings",
                        format=code,
                        artifact_type="innings",
                        models_dir=models_dir,
                    )
                else:
                    logger.warning(
                        "artifacts.load_per_format.innings_model_missing",
                        format=code,
                        artifact_type="innings",
                        path=mpath,
                    )
        except Exception as e:
            logger.error(
                "artifacts.load_per_format.load_failed",
                fname=fname,
                models_dir=models_dir,
                error=str(e),
            )


def reload(models_dir: str) -> dict:
    """Rescan models_dir and reload registries. Returns a summary dict."""
    logger.info("artifacts.reload.start", models_dir=models_dir)
    BAT_MODELS.clear()
    BOWL_MODELS.clear()
    FIELD_MODELS.clear()
    EXTRAS_MODELS.clear()
    WIN_MODELS.clear()
    INNINGS_MODELS.clear()
    BAT_SHARE_MODELS.clear()
    BOWL_SHARE_MODELS.clear()
    INNINGS_META.clear()
    EXTRAS_META.clear()
    _load_per_format(models_dir)
    out = summary()
    logger.info(
        "artifacts.reload.done",
        models_dir=models_dir,
        batting_formats=out["loaded_batting_formats"],
        bowling_formats=out["loaded_bowling_formats"],
        fielding_formats=out["loaded_fielding_formats"],
    )
    return out


def summary() -> dict:
    return {
        "loaded_batting_formats": sorted(BAT_MODELS.keys()),
        "loaded_bowling_formats": sorted(BOWL_MODELS.keys()),
        "loaded_fielding_formats": sorted(FIELD_MODELS.keys()),
        "loaded_extras_formats": sorted(EXTRAS_MODELS.keys()),
        "loaded_win_formats": sorted(WIN_MODELS.keys()),
        "loaded_innings_formats": sorted(INNINGS_MODELS.keys()),
        "loaded_batting_share_formats": sorted(BAT_SHARE_MODELS.keys()),
        "loaded_bowling_share_formats": sorted(BOWL_SHARE_MODELS.keys()),
    }
