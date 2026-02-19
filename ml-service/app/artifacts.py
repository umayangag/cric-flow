"""Model artifact registry and loader utilities.

This module keeps in-memory registries mapping a format code (e.g., "T20") to
the tuple (scaler, model). It supports two styles of artifacts:
- Legacy artifacts without a format suffix, stored under the special key
  "_LEGACY_".
- Per-format artifacts with filenames like `batting_scaler_T20.joblib` and
  `batting_model_T20.joblib` discovered under a configured models directory.

Use `reload(models_dir)` to (re)scan a directory and populate the registries.
The lightweight `summary()` function returns a snapshot indicating which
formats are currently loaded.
"""

import os
from typing import Dict, Optional, Tuple

import joblib

from app.logging import get_struct_logger

logger = get_struct_logger()

# Registries: map format code -> (scaler, model). Legacy unsuffixed artifacts are stored under key "_LEGACY_".
BAT_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
FIELD_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
EXTRAS_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level extras regressor)
WIN_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level win classifier)


def _load_legacy(models_dir: str) -> None:
    """Load legacy (unsuffixed) joblib artifacts into _LEGACY_ registries.

    Security: joblib uses pickle. Restrict filesystem permissions on models_dir
    and only load artifacts from trusted sources to avoid insecure deserialization.
    """
    try:
        bat_scaler = joblib.load(os.path.join(models_dir, "batting_scaler.joblib"))
        bat_model = joblib.load(os.path.join(models_dir, "batting_model.joblib"))
        BAT_MODELS["_LEGACY_"] = (bat_scaler, bat_model)
        logger.info("artifacts.load_legacy.batting", models_dir=models_dir)
    except Exception as e:
        logger.debug("artifacts.load_legacy.batting_skip", models_dir=models_dir, error=str(e))
    try:
        bowl_scaler = joblib.load(os.path.join(models_dir, "bowling_scaler.joblib"))
        bowl_model = joblib.load(os.path.join(models_dir, "bowling_model.joblib"))
        BOWL_MODELS["_LEGACY_"] = (bowl_scaler, bowl_model)
        logger.info("artifacts.load_legacy.bowling", models_dir=models_dir)
    except Exception as e:
        logger.debug("artifacts.load_legacy.bowling_skip", models_dir=models_dir, error=str(e))
    try:
        field_scaler = joblib.load(os.path.join(models_dir, "fielding_scaler.joblib"))
        field_model = joblib.load(os.path.join(models_dir, "fielding_model.joblib"))
        FIELD_MODELS["_LEGACY_"] = (field_scaler, field_model)
        logger.info("artifacts.load_legacy.fielding", models_dir=models_dir)
    except Exception as e:
        logger.debug("artifacts.load_legacy.fielding_skip", models_dir=models_dir, error=str(e))
    try:
        extras_model = joblib.load(os.path.join(models_dir, "extras_model.joblib"))
        EXTRAS_MODELS["_LEGACY_"] = extras_model
        logger.info("artifacts.load_legacy.extras", models_dir=models_dir)
    except Exception as e:
        logger.debug("artifacts.load_legacy.extras_skip", models_dir=models_dir, error=str(e))
    try:
        win_model = joblib.load(os.path.join(models_dir, "win_model.joblib"))
        WIN_MODELS["_LEGACY_"] = win_model
        logger.info("artifacts.load_legacy.win", models_dir=models_dir)
    except Exception as e:
        logger.debug("artifacts.load_legacy.win_skip", models_dir=models_dir, error=str(e))


def _load_per_format(models_dir: str) -> None:
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
                    logger.info("artifacts.load_per_format.batting", format=code, models_dir=models_dir)
                else:
                    logger.warning("artifacts.load_per_format.batting_model_missing", format=code, path=mpath)
            if lf.startswith("bowling_scaler_") and lf.endswith(".joblib"):
                code = fname[len("bowling_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"bowling_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BOWL_MODELS[code] = (scaler, model)
                    logger.info("artifacts.load_per_format.bowling", format=code, models_dir=models_dir)
                else:
                    logger.warning("artifacts.load_per_format.bowling_model_missing", format=code, path=mpath)
            if lf.startswith("fielding_scaler_") and lf.endswith(".joblib"):
                code = fname[len("fielding_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"fielding_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    FIELD_MODELS[code] = (scaler, model)
                    logger.info("artifacts.load_per_format.fielding", format=code, models_dir=models_dir)
                else:
                    logger.warning("artifacts.load_per_format.fielding_model_missing", format=code, path=mpath)
            if lf.startswith("extras_model_") and lf.endswith(".joblib"):
                code = fname[len("extras_model_") : -len(".joblib")].upper()
                model = joblib.load(os.path.join(models_dir, fname))
                EXTRAS_MODELS[code] = model
                logger.info("artifacts.load_per_format.extras", format=code, models_dir=models_dir)
            if lf.startswith("win_model_") and lf.endswith(".joblib"):
                code = fname[len("win_model_") : -len(".joblib")].upper()
                model = joblib.load(os.path.join(models_dir, fname))
                WIN_MODELS[code] = model
                logger.info("artifacts.load_per_format.win", format=code, models_dir=models_dir)
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
    _load_legacy(models_dir)
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
        "loaded_batting_formats": sorted([k for k in BAT_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_bowling_formats": sorted([k for k in BOWL_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_fielding_formats": sorted([k for k in FIELD_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_extras_formats": sorted([k for k in EXTRAS_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_win_formats": sorted([k for k in WIN_MODELS.keys() if k != "_LEGACY_"]),
        "legacy_batting": "_LEGACY_" in BAT_MODELS,
        "legacy_bowling": "_LEGACY_" in BOWL_MODELS,
        "legacy_fielding": "_LEGACY_" in FIELD_MODELS,
        "legacy_extras": "_LEGACY_" in EXTRAS_MODELS,
        "legacy_win": "_LEGACY_" in WIN_MODELS,
    }
