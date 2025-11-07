import os
from typing import Dict, Optional, Tuple

import joblib

# Registries: map format code -> (scaler, model). Legacy unsuffixed artifacts are stored under key "_LEGACY_".
BAT_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}


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
    except Exception:
        # listing may fail; just ignore to keep service running
        pass


def reload(models_dir: str) -> dict:
    """Rescan models_dir and reload registries. Returns a summary dict."""
    BAT_MODELS.clear()
    BOWL_MODELS.clear()
    _load_legacy(models_dir)
    _load_per_format(models_dir)
    return summary()


def summary() -> dict:
    return {
        "loaded_batting_formats": sorted([k for k in BAT_MODELS.keys() if k != "_LEGACY_"]),
        "loaded_bowling_formats": sorted([k for k in BOWL_MODELS.keys() if k != "_LEGACY_"]),
        "legacy_batting": "_LEGACY_" in BAT_MODELS,
        "legacy_bowling": "_LEGACY_" in BOWL_MODELS,
    }
