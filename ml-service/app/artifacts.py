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

# Registries: map format code -> (scaler, model). Legacy unsuffixed artifacts are stored under key "_LEGACY_".
BAT_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
FIELD_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
EXTRAS_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level extras regressor)
WIN_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level win classifier)


def _load_legacy(models_dir: str) -> None:
    try:
        bat_scaler = joblib.load(os.path.join(models_dir, "batting_scaler.joblib"))
        bat_model = joblib.load(os.path.join(models_dir, "batting_model.joblib"))
        BAT_MODELS["_LEGACY_"] = (bat_scaler, bat_model)
    except Exception:
        pass
    try:
        bowl_scaler = joblib.load(os.path.join(models_dir, "bowling_scaler.joblib"))
        bowl_model = joblib.load(os.path.join(models_dir, "bowling_model.joblib"))
        BOWL_MODELS["_LEGACY_"] = (bowl_scaler, bowl_model)
    except Exception:
        pass


def _load_per_format(models_dir: str) -> None:
    try:
        for fname in os.listdir(models_dir):
            lf = fname.lower()
            if lf.startswith("batting_scaler_") and lf.endswith(".joblib"):
                code = fname[len("batting_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"batting_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BAT_MODELS[code] = (scaler, model)
            if lf.startswith("bowling_scaler_") and lf.endswith(".joblib"):
                code = fname[len("bowling_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"bowling_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    BOWL_MODELS[code] = (scaler, model)
            if lf.startswith("fielding_scaler_") and lf.endswith(".joblib"):
                code = fname[len("fielding_scaler_") : -len(".joblib")].upper()
                scaler = joblib.load(os.path.join(models_dir, fname))
                mname = f"fielding_model_{code}.joblib"
                mpath = os.path.join(models_dir, mname)
                if os.path.exists(mpath):
                    model = joblib.load(mpath)
                    FIELD_MODELS[code] = (scaler, model)
            if lf.startswith("extras_model_") and lf.endswith(".joblib"):
                code = fname[len("extras_model_") : -len(".joblib")].upper()
                model = joblib.load(os.path.join(models_dir, fname))
                EXTRAS_MODELS[code] = model
            if lf.startswith("win_model_") and lf.endswith(".joblib"):
                code = fname[len("win_model_") : -len(".joblib")].upper()
                model = joblib.load(os.path.join(models_dir, fname))
                WIN_MODELS[code] = model
    except Exception:
        # listing may fail; just ignore to keep service running
        pass


def reload(models_dir: str) -> dict:
    """Rescan models_dir and reload registries. Returns a summary dict."""
    BAT_MODELS.clear()
    BOWL_MODELS.clear()
    FIELD_MODELS.clear()
    EXTRAS_MODELS.clear()
    WIN_MODELS.clear()
    _load_legacy(models_dir)
    _load_per_format(models_dir)
    return summary()


def summary() -> dict:
    return {
        "loaded_batting_formats": sorted([k for k in BAT_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_bowling_formats": sorted([k for k in BOWL_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_fielding_formats": sorted(FIELD_MODELS.keys()),
        "loaded_extras_formats": sorted(EXTRAS_MODELS.keys()),
        "loaded_win_formats": sorted(WIN_MODELS.keys()),
        "legacy_batting": "_LEGACY_" in BAT_MODELS,
        "legacy_bowling": "_LEGACY_" in BOWL_MODELS,
    }
