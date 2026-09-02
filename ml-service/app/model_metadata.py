"""Model metadata for the Workbench UI: features, outputs, level, artifacts pattern, notes.

One model is left with a metadata card: the windowed-form win classifier, whose feature
order comes from ``ml.win_features.WIN_ENHANCED_FEATURE_COLS``. The batting, bowling,
fielding, extras, innings and combination-meta entries went with their models in P-5, and
this one goes with the win model in P-6. The XI layer's own models describe themselves
through ``GET /xi/status`` and L4's report.

Served by GET /model-metadata so the frontend stays in sync with the backend.
"""

from typing import Any, Dict, List

WIN_OUTPUTS = ["team1_win_probability"]

# Static metadata: level, hasScaler, artifactsPattern, note (not derivable from config)
_STATIC: Dict[str, Dict[str, Any]] = {
    "win": {
        "level": "match",
        "hasScaler": False,
        "artifactsPattern": {
            "perFormat": "win_model_<FMT>.joblib",
        },
        "note": "Match-level; team1 = batting first, team2 = bowling first. One model per format.",
    },
}


def _win_feature_cols() -> List[str]:
    try:
        from ml.win_features import WIN_ENHANCED_FEATURE_COLS

        return list(WIN_ENHANCED_FEATURE_COLS)
    except Exception:
        return []


def get_model_metadata() -> Dict[str, Any]:
    """Build model metadata from the win model's feature columns. One source of truth for the UI."""
    static = _STATIC["win"]
    return {
        "win": {
            "features": _win_feature_cols(),
            "outputs": WIN_OUTPUTS,
            "level": static["level"],
            "hasScaler": static["hasScaler"],
            "artifactsPattern": static["artifactsPattern"],
            "note": static.get("note"),
        }
    }
