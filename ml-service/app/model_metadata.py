"""
Model metadata for the Workbench UI: features, outputs, level, artifacts pattern, notes.

Generated from the source of truth: feature_vectors.json (batting, bowling, fielding),
train_extras.EXTRAS_FEATURE_COLS, win_features.WIN_ENHANCED_FEATURE_COLS, and static metadata.
Served by GET /model-metadata so the frontend stays in sync with the backend.

Model mode registry: central list of prediction/artifact modes (legacy vs per-format)
so UIs and callers can show mode names, deprecation, and descriptions without hardcoding.
"""

from typing import Any, Dict, List, TypedDict

from .feature_config import get_feature_names

# --- Model mode registry (single source of truth for legacy vs per-format vs unified) ---


class ModelModeEntry(TypedDict, total=False):
    name: str
    available: bool
    deprecated: bool
    description: str


MODEL_MODE_REGISTRY: List[ModelModeEntry] = [
    {
        "name": "legacy",
        "available": True,
        "deprecated": False,
        "description": "Legacy unified model (no format suffix); used when format is omitted or no per-format artifact is loaded.",
    },
    {
        "name": "per_format",
        "available": True,
        "deprecated": False,
        "description": "Per-format model (e.g. T20, ODI); used when format is provided and matching artifacts are loaded.",
    },
]


def get_model_modes() -> List[Dict[str, Any]]:
    """Return the model mode registry for UIs and API. Safe to extend with runtime availability later."""
    return [dict(entry) for entry in MODEL_MODE_REGISTRY]


# Output column names per model (aligned with prediction response and training scripts)
BATTING_OUTPUTS = [
    "runs_scored",
    "balls_faced",
    "fours_scored",
    "sixes_scored",
    "batting_position",
    "strike_rate",
]
BOWLING_OUTPUTS = ["runs_conceded", "deliveries", "wickets_taken", "economy"]
FIELDING_OUTPUTS = ["catches", "run_outs", "stumpings"]
EXTRAS_OUTPUTS = ["total_extras"]
WIN_OUTPUTS = ["team1_win_probability"]

# Static metadata: level, hasScaler, artifactsPattern, note (not derivable from config)
_STATIC: Dict[str, Dict[str, Any]] = {
    "batting": {
        "level": "player",
        "hasScaler": True,
        "artifactsPattern": {
            "perFormat": "batting_scaler_<FMT>.joblib + batting_model_<FMT>.joblib",
            "legacy": "batting_scaler.joblib + batting_model.joblib",
        },
        "note": "Player-level; same feature families used for match-level models. Prediction uses per-format model when format (e.g. T20) is provided and loaded; otherwise legacy.",
    },
    "bowling": {
        "level": "player",
        "hasScaler": True,
        "artifactsPattern": {
            "perFormat": "bowling_scaler_<FMT>.joblib + bowling_model_<FMT>.joblib",
            "legacy": "bowling_scaler.joblib + bowling_model.joblib",
        },
        "note": "Player-level; aggregates feed into extras and win. Per-format and legacy same as batting.",
    },
    "fielding": {
        "level": "player",
        "hasScaler": True,
        "artifactsPattern": {
            "perFormat": "fielding_scaler_<FMT>.joblib + fielding_model_<FMT>.joblib",
            "legacy": "fielding_scaler.joblib + fielding_model.joblib",
        },
        "note": "Player-level; combined with batting/bowling for team selection. Per-format and legacy same as batting.",
    },
    "extras": {
        "level": "match",
        "hasScaler": False,
        "artifactsPattern": {
            "perFormat": "extras_model_<FMT>.joblib",
            "legacy": "extras_model.joblib",
        },
        "note": "Match-level; uses same weather + aggregates of player consistency/form from batting/bowling snapshot data. One model per format or legacy.",
    },
    "win": {
        "level": "match",
        "hasScaler": False,
        "artifactsPattern": {
            "perFormat": "win_model_<FMT>.joblib",
            "legacy": "win_model.joblib",
        },
        "note": "Match-level; team1 = batting first, team2 = bowling first. Same feature families as batting/bowling/fielding. One model per format or legacy.",
    },
    "combination_meta": {
        "level": "meta",
        "hasScaler": False,
        "artifactsPattern": {
            "perFormat": "Optional per-format weights in combination_meta.json",
            "legacy": "combination_meta.json (unified weights)",
        },
        "note": "Optional. Learns weights to combine batting/bowling/fielding scores from backtest outcomes. Uses CSV with bat_score, bowl_score, field_score, is_keeper, format, target. Output is JSON (not joblib); go-app can load via selection.meta_model_path.",
    },
}


def _extras_feature_cols() -> List[str]:
    try:
        from ml.train_extras import EXTRAS_FEATURE_COLS

        return list(EXTRAS_FEATURE_COLS)
    except ImportError:
        return []


def _win_feature_cols() -> List[str]:
    try:
        from ml.win_features import WIN_ENHANCED_FEATURE_COLS

        return list(WIN_ENHANCED_FEATURE_COLS)
    except ImportError:
        return []


def get_model_metadata() -> Dict[str, Any]:
    """Build model metadata from feature config and training modules. One source of truth for the UI.
    Includes model_modes (registry of legacy/per_format) for frontend display and deprecation hints.
    """
    out: Dict[str, Any] = {"model_modes": get_model_modes()}

    for kind in ("batting", "bowling", "fielding"):
        try:
            features = get_feature_names(kind)
        except Exception:
            features = []
        static = _STATIC[kind]
        outputs = BATTING_OUTPUTS if kind == "batting" else (BOWLING_OUTPUTS if kind == "bowling" else FIELDING_OUTPUTS)
        out[kind] = {
            "features": features,
            "outputs": outputs,
            "level": static["level"],
            "hasScaler": static["hasScaler"],
            "artifactsPattern": static["artifactsPattern"],
            "note": static.get("note"),
        }

    for kind in ("extras", "win"):
        features = _extras_feature_cols() if kind == "extras" else _win_feature_cols()
        static = _STATIC[kind]
        outputs = EXTRAS_OUTPUTS if kind == "extras" else WIN_OUTPUTS
        out[kind] = {
            "features": features,
            "outputs": outputs,
            "level": static["level"],
            "hasScaler": static["hasScaler"],
            "artifactsPattern": static["artifactsPattern"],
            "note": static.get("note"),
        }

    # combination_meta: no dynamic features; UI-only description
    static = _STATIC["combination_meta"]
    out["combination_meta"] = {
        "features": ["bat_score", "bowl_score", "field_score", "is_keeper", "format (one-hot)"],
        "outputs": ["score_weights (Ridge coefficients for team selection)"],
        "level": static["level"],
        "hasScaler": static["hasScaler"],
        "artifactsPattern": static["artifactsPattern"],
        "note": static.get("note"),
    }

    return out
