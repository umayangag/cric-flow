"""HTTP /predict endpoint handlers (batting, bowling, extras, win, team selection)."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Dict, List, Tuple

if TYPE_CHECKING:
    from ml.team_optimizer import OptimizationResult, PoolPlayer, ScoreWeights, SelectionConstraints

import numpy as np
from fastapi import HTTPException

from ml.train_extras import EXTRAS_FEATURE_COLS, LEGACY_EXTRAS_FEATURE_COLS
from ml.win_features import (
    WIN_ENHANCED_FEATURE_COLS,
    aggregate_team_features_from_player_maps,
    build_feature_vector,
    compute_derived_features,
    format_one_hot_from_code,
)

from ..artifacts import BAT_MODELS, BOWL_MODELS, EXTRAS_META, EXTRAS_MODELS, WIN_MODELS
from ..errors import error_payload
from ..features import batting_feature_vector, bowling_feature_vector
from ..logging import get_struct_logger
from ..models import (
    BattingFeatures,
    BattingPrediction,
    BowlingFeatures,
    BowlingPrediction,
    ExtrasFeatures,
    ExtrasPrediction,
    WinFeatures,
    WinPrediction,
)

logger = get_struct_logger()


def _resolve_extras_feature_order(fmt_key: str) -> List[str]:
    """Column order: sidecar feature_names when loaded, else the declared column layout."""
    meta = EXTRAS_META.get(fmt_key)
    if meta is not None:
        names = meta.get("feature_names")
        if isinstance(names, list) and names:
            return [str(n) for n in names]
    if LEGACY_EXTRAS_FEATURE_COLS:
        return list(LEGACY_EXTRAS_FEATURE_COLS)
    return list(EXTRAS_FEATURE_COLS)


def resolve_model_pair(
    registry: Dict[str, Any],
    fmt: str,
    domain: str,
) -> Tuple[Any, Any]:
    """Resolve (scaler, model) pair from the registry by format.

    ``format`` is required: artifacts are per-format and there is no fallback tier.
    Raises HTTPException if it is missing or no model is loaded for it.
    """
    fmt_upper = (fmt or "").strip().upper()
    if fmt_upper:
        pair = registry.get(fmt_upper)
        if not pair:
            logger.warning(f"predict.{domain}.model_not_loaded", format=fmt_upper, available=list(registry.keys()))
            raise HTTPException(
                status_code=404,
                detail=error_payload(
                    code="MODEL_NOT_LOADED",
                    message=f"Model for format {fmt_upper} not loaded",
                    hint="Train artifacts for this format and place them under the models directory.",
                    available=list(registry.keys()),
                ),
            )
        return pair
    logger.info(f"predict.{domain}.rejected", reason="missing_format")
    raise HTTPException(
        status_code=400,
        detail=error_payload(
            code="MISSING_FORMAT",
            message="Missing 'format'",
            hint=f"Set 'format' on the request; {domain} models are per-format.",
            available=list(registry.keys()),
        ),
    )


def validate_predict_batch(features: list, domain: str, max_batch_size: int) -> None:
    """Validate common predict endpoint preconditions: non-empty, within batch limit, same format."""
    if not features:
        logger.info(f"predict.{domain}.rejected", reason="empty_batch")
        raise HTTPException(
            status_code=400,
            detail=error_payload(
                code="EMPTY_BATCH",
                message="Empty features list",
                hint="Send at least one feature row with the required fields.",
            ),
        )
    if max_batch_size and len(features) > max_batch_size:
        raise HTTPException(
            status_code=400,
            detail=error_payload(
                code="BATCH_TOO_LARGE",
                message="Batch size exceeds limit",
                hint=f"Send at most {max_batch_size} features per request.",
            ),
        )
    fmt = (features[0].format or "").strip().upper()
    if fmt:
        for f in features:
            if (f.format or "").strip().upper() != fmt:
                logger.info(f"predict.{domain}.rejected", reason="mixed_formats", batch_size=len(features))
                raise HTTPException(
                    status_code=400,
                    detail=error_payload(
                        code="MIXED_FORMATS",
                        message="All feature rows must have the same format",
                        hint="Ensure every row uses the same 'format' code.",
                    ),
                )


def run_batting_prediction(features: List[BattingFeatures]) -> List[BattingPrediction]:
    """Execute batting prediction pipeline and return typed results."""
    fmt = (features[0].format or "").strip().upper()
    scaler, model = resolve_model_pair(BAT_MODELS, fmt, "batting")
    logger.info("predict.batting.start", batch=len(features), format=fmt)
    X = np.array([batting_feature_vector(f) for f in features], dtype=float)
    if scaler is not None:
        X = scaler.transform(X)
    try:
        Y = model.predict(X)
        preds = []
        for row in Y:
            vals = row if np.ndim(row) == 1 else row.ravel()
            vals = list(vals) + [0.0] * max(0, 5 - len(vals))
            runs = float(vals[0])
            balls = float(vals[1])
            sr = (runs / balls * 100.0) if balls > 0 else 0.0
            preds.append(
                BattingPrediction(
                    runs_scored=runs,
                    balls_faced=balls,
                    fours_scored=float(vals[2]),
                    sixes_scored=float(vals[3]),
                    batting_position=float(vals[4]),
                    strike_rate=sr,
                )
            )
        logger.info("predict.batting.success", predictions=len(preds))
        return preds
    except Exception as exc:
        logger.exception("predict.batting.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="PREDICT_FAILED",
                message="Batting prediction failed",
                hint="See server logs for stacktrace using request_id",
            ),
        )


def run_bowling_prediction(features: List[BowlingFeatures]) -> List[BowlingPrediction]:
    """Execute bowling prediction pipeline and return typed results."""
    fmt = (features[0].format or "").strip().upper()
    scaler, model = resolve_model_pair(BOWL_MODELS, fmt, "bowling")
    logger.info("predict.bowling.start", batch=len(features), format=fmt)
    X = np.array([bowling_feature_vector(f) for f in features], dtype=float)
    if scaler is not None:
        X = scaler.transform(X)
    try:
        Y = model.predict(X)
        preds = []
        for row in Y:
            vals = row if np.ndim(row) == 1 else row.ravel()
            vals = list(vals) + [0.0] * max(0, 3 - len(vals))
            runs_conceded = float(vals[0])
            deliveries = float(vals[1])
            wickets_taken = float(vals[2])
            econ = (runs_conceded / (deliveries / 6.0)) if deliveries > 0 else 0.0
            preds.append(
                BowlingPrediction(
                    runs_conceded=runs_conceded,
                    deliveries=deliveries,
                    wickets_taken=wickets_taken,
                    econ=econ,
                )
            )
        logger.info("predict.bowling.success", predictions=len(preds))
        return preds
    except Exception as exc:
        logger.exception("predict.bowling.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="PREDICT_FAILED",
                message="Bowling prediction failed",
                hint="See server logs for stacktrace using request_id",
            ),
        )


def extras_feature_vector(f: ExtrasFeatures) -> np.ndarray:
    """Build feature vector in sidecar or declared column order (exclude 'format' key)."""
    fmt = (f.format or "").strip().upper() if isinstance(f.format, str) else ""
    fmt_key = fmt
    cols = _resolve_extras_feature_order(fmt_key)
    if not cols:
        return np.zeros(0)
    d = f.model_dump()
    d.update(format_one_hot_from_code(fmt))
    return np.array([float(d.get(c, 0)) for c in cols], dtype=float)


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


def run_extras_prediction(features: List[ExtrasFeatures]) -> List[ExtrasPrediction]:
    """Execute extras prediction pipeline and return typed results."""
    fmt = (features[0].format or "").strip().upper()
    model = EXTRAS_MODELS.get(fmt) if fmt else None
    if not model:
        available = list(EXTRAS_MODELS.keys())
        logger.warning("predict.extras.model_not_loaded", format=fmt or "", available=available)
        raise HTTPException(
            status_code=404,
            detail=error_payload(
                code="MODEL_NOT_LOADED",
                message="Extras model not loaded",
                hint="Train extras artifacts (e.g. make train-extras) for this format.",
                available=available,
            ),
        )
    X = np.array([extras_feature_vector(f) for f in features], dtype=float)
    if X.size == 0:
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="FEATURE_ORDER_EMPTY",
                message="Extras feature column list is empty (check artifacts and sidecar)",
            ),
        )
    try:
        y = model.predict(X)
        y_flat = np.asarray(y).ravel()
        return [ExtrasPrediction(total_extras=float(v)) for v in y_flat]
    except Exception as exc:
        logger.exception("predict.extras.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="PREDICT_FAILED", message="Extras prediction failed", hint="See server logs"),
        )


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


# ---------------------------------------------------------------------------
# Team selection optimisation (server-side hill-climb)
# ---------------------------------------------------------------------------


def run_team_optimization(
    fmt: str,
    pool: "List[PoolPlayer]",
    opponent_features: Dict[int, Dict[str, float]],
    match_context: Dict[str, float],
    constraints: "SelectionConstraints",
    weights: "ScoreWeights",
    team_is_team1: bool,
    max_iterations: int,
    max_evals: int,
) -> "OptimizationResult":
    """Resolve the win model by format and delegate to the team optimizer."""
    from ml.team_optimizer import optimize_team_by_win_probability

    fmt_upper = (fmt or "").strip().upper()
    model = _resolve_win_model(fmt_upper)

    return optimize_team_by_win_probability(
        pool=pool,
        opponent_features=opponent_features,
        match_context=match_context,
        constraints=constraints,
        weights=weights,
        team_is_team1=team_is_team1,
        model=model,
        format_code=fmt_upper,
        max_iterations=max_iterations,
        max_evals=max_evals,
    )
