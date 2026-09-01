"""HTTP /predict endpoint handlers for the windowed-form win model.

The batting, bowling, fielding and extras handlers, and the server-side team-selection
hill-climb that read the same artifacts, went with their models in P-5; selection and the
per-player forecast are now XI-layer calls (``app.xi_service``). What is left is the win
model itself, which P-6 removes.
"""

from __future__ import annotations

from typing import Dict, List

import numpy as np
from fastapi import HTTPException

from ml.win_features import (
    WIN_ENHANCED_FEATURE_COLS,
    aggregate_team_features_from_player_maps,
    build_feature_vector,
    compute_derived_features,
    format_one_hot_from_code,
)

from ..artifacts import WIN_MODELS
from ..errors import error_payload
from ..logging import get_struct_logger
from ..models.predict import WinFeatures, WinPrediction

logger = get_struct_logger()


def win_feature_vector(f: WinFeatures) -> np.ndarray:
    """Build feature vector for the enhanced win model from a WinFeatures instance.

    Pads scalar WinFeatures fields into WIN_ENHANCED_FEATURE_COLS (distribution stats
    default to 0; derived features computed from available scalars).
    """
    d = f.model_dump()
    fmt = (f.format or "").strip().upper() if isinstance(f.format, str) else ""
    d.update(format_one_hot_from_code(fmt))
    d.update(compute_derived_features(d))
    return np.array([float(d.get(c, 0)) for c in WIN_ENHANCED_FEATURE_COLS], dtype=float)


def _resolve_win_model(fmt: str):
    """Resolve the win model by format. Raises HTTPException if missing."""
    model = WIN_MODELS.get(fmt) if fmt else None
    if not model:
        available = list(WIN_MODELS.keys())
        logger.warning("predict.win.model_not_loaded", format=fmt or "", available=available)
        raise HTTPException(
            status_code=404,
            detail=error_payload(
                code="MODEL_NOT_LOADED",
                message="Win model not loaded",
                hint="Train win artifacts (e.g. make train-win) for this format.",
                available=available,
            ),
        )
    return model


def _predict_win_proba(model, X: np.ndarray) -> List[float]:
    """Run model.predict_proba and extract team1 win probability."""
    proba = model.predict_proba(X)
    if proba.shape[1] > 1:
        p_team1 = proba[:, 1]
    else:
        p_team1 = proba.ravel() if model.classes_[0] == 1 else 1.0 - proba.ravel()
    return [float(p) for p in p_team1]


def run_win_prediction(features: List[WinFeatures]) -> List[WinPrediction]:
    """Execute win prediction from WinFeatures (backward-compatible scalar path)."""
    fmt = (features[0].format or "").strip().upper()
    model = _resolve_win_model(fmt)
    X = np.array([win_feature_vector(f) for f in features], dtype=float)
    if X.size == 0:
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="FEATURE_ORDER_EMPTY", message="WIN_ENHANCED_FEATURE_COLS not available"),
        )
    try:
        probas = _predict_win_proba(model, X)
        return [WinPrediction(team1_win_probability=p) for p in probas]
    except Exception as exc:
        logger.exception("predict.win.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="PREDICT_FAILED", message="Win prediction failed", hint="See server logs"),
        )


def run_win_prediction_enhanced(
    fmt: str,
    match_context: Dict[str, float],
    team1_player_features: Dict[str, Dict[str, float]],
    team2_player_features: Dict[str, Dict[str, float]],
) -> WinPrediction:
    """Execute win prediction using per-player features with on-the-fly aggregation.

    This is the primary path for the win-first architecture: the Go-app passes
    all per-player features, and the ML service computes distribution statistics
    and derived features before feeding to the model.
    """
    fmt_upper = (fmt or "").strip().upper()
    model = _resolve_win_model(fmt_upper)

    def _validated_player_id(k: str) -> int:
        if not k.isdigit() or len(k) > 20:
            raise HTTPException(
                status_code=400,
                detail=error_payload(code="INVALID_PLAYER_ID", message=f"Invalid player ID key: {k!r}"),
            )
        return int(k)

    t1_feats = {_validated_player_id(k): v for k, v in team1_player_features.items()}
    t2_feats = {_validated_player_id(k): v for k, v in team2_player_features.items()}
    fmt_upper = (fmt or "").strip().upper()

    feature_dict = aggregate_team_features_from_player_maps(t1_feats, t2_feats, match_context, format_code=fmt_upper)
    feature_vec = build_feature_vector(feature_dict)
    X = np.array([feature_vec], dtype=float)

    try:
        probas = _predict_win_proba(model, X)
        return WinPrediction(team1_win_probability=probas[0])
    except Exception as exc:
        logger.exception("predict.win_enhanced.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="PREDICT_FAILED", message="Enhanced win prediction failed", hint="See server logs"
            ),
        )
