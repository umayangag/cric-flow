"""Core prediction logic for backtest and player-level predictions.

Extracted from app.main to keep route handlers thin. Contains:
- Feature transform pipeline (batting, bowling, fielding)
- Train-on-the-fly fallback when no pre-trained artifacts exist
- Innings prediction and hybrid reconciliation
- Match-level and player-level prediction orchestration
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Any, Dict, List, Optional, Tuple

if TYPE_CHECKING:
    from ml.team_optimizer import OptimizationResult, PoolPlayer, ScoreWeights, SelectionConstraints

import numpy as np
from fastapi import HTTPException

from .artifacts import (
    BAT_MODELS,
    BAT_SHARE_MODELS,
    BOWL_MODELS,
    BOWL_SHARE_MODELS,
    EXTRAS_MODELS,
    FIELD_MODELS,
    INNINGS_MODELS,
    WIN_MODELS,
)
from .artifacts import _use_share_models as use_share_models_config
from .backtest_service import (
    build_batting_features_from_map,
    build_bowling_features_from_map,
    build_fielding_features_from_map,
)
from .errors import error_payload
from .feature_config import get_feature_names
from .features import batting_feature_vector, bowling_feature_vector, fielding_feature_vector
from .logging import get_struct_logger
from .models import (
    BacktestPlayerPred,
    BatchPredictItem,
    BattingFeatures,
    BattingPrediction,
    BowlingFeatures,
    BowlingPrediction,
    ExtrasFeatures,
    ExtrasPrediction,
    InningsSummary,
    MatchContext,
    WinFeatures,
    WinPrediction,
)
from .reconciliation import predict_innings, rescale_player_predictions
from .train_on_the_fly import train_on_the_fly_cached

try:
    from ml.config import get_prediction_defaults, get_win_coherence_config
    from ml.reconciliation_adapter import apply_constraint_reconciliation_from_backtest_preds
except ImportError:
    get_prediction_defaults = None  # type: ignore[assignment]
    get_win_coherence_config = None  # type: ignore[assignment]
    apply_constraint_reconciliation_from_backtest_preds = None  # type: ignore[assignment]

try:
    from ml.train_extras import EXTRAS_FEATURE_COLS
except ImportError:
    EXTRAS_FEATURE_COLS = []

try:
    from ml.win_features import (
        WIN_ENHANCED_FEATURE_COLS,
        aggregate_team_features_from_player_maps,
        build_feature_vector,
        compute_derived_features,
    )
except ImportError:
    WIN_ENHANCED_FEATURE_COLS = []
    aggregate_team_features_from_player_maps = None  # type: ignore[assignment]
    build_feature_vector = None  # type: ignore[assignment]
    compute_derived_features = None  # type: ignore[assignment]

logger = get_struct_logger()


@dataclass
class GenerateMatchSettings:
    models_dir: str
    enable_train_on_the_fly: bool
    go_app_url: str
    go_app_api_key: Optional[str]
    train_latest_cache_granularity: str


def round_datetime_to_granularity(dt: datetime, granularity: str) -> datetime:
    """Round datetime down to the given boundary. Used for cache-key stability when use_latest_model=True."""
    gran = (granularity or "").strip().lower()
    if gran in ("none", ""):
        return dt
    if gran == "second":
        return dt.replace(microsecond=0)
    if gran == "minute":
        return dt.replace(second=0, microsecond=0)
    if gran == "hour":
        return dt.replace(minute=0, second=0, microsecond=0)
    if gran == "day":
        return dt.replace(hour=0, minute=0, second=0, microsecond=0)
    return dt  # fallback: no rounding


def _sum_team_feature(
    features_map: Dict[str, Dict[str, float]],
    ids: set,
    key_bat: str,
    key_bowl: str,
) -> Tuple[float, float]:
    """Sum batting and bowling feature values for a set of player IDs. Used by predict_match_innings and generate_match."""
    bat_sum, bowl_sum = 0.0, 0.0
    for pid in ids:
        fm = features_map.get(str(pid), {})
        bat_sum += fm.get(key_bat, 0.0)
        bowl_sum += fm.get(key_bowl, 0.0)
    return bat_sum, bowl_sum


def predict_match_innings(
    match_context: MatchContext,
    features_map: Dict[str, Dict[str, float]],
    fmt_upper: str,
    match_date_unix: float,
) -> Optional[Tuple[float, float, float, float]]:
    """Predict innings runs and wickets for both innings. Returns (inn1_runs, inn1_wkts, inn2_runs, inn2_wkts) or None if no model."""
    innings_pair = (INNINGS_MODELS.get(fmt_upper) if fmt_upper else None) or INNINGS_MODELS.get("_LEGACY_")
    if innings_pair is None:
        return None
    scaler_inn, model_inn = innings_pair
    team1_ids = {int(pid) for pid in match_context.team1_player_ids}
    team2_ids = {int(pid) for pid in match_context.team2_player_ids}

    # v2: consistency = std_w10, form = mean_w5 (from feature_raw_stats_snapshots)
    t1_bat_cons, t1_bowl_cons = _sum_team_feature(features_map, team1_ids, "batting_std_w10", "bowling_std_w10")
    t1_bat_form, t1_bowl_form = _sum_team_feature(features_map, team1_ids, "batting_mean_w5", "bowling_mean_w5")
    t2_bat_cons, t2_bowl_cons = _sum_team_feature(features_map, team2_ids, "batting_std_w10", "bowling_std_w10")
    t2_bat_form, t2_bowl_form = _sum_team_feature(features_map, team2_ids, "batting_mean_w5", "bowling_mean_w5")
    inn1_runs, inn1_wkts = predict_innings(
        scaler_inn,
        model_inn,
        inning_number=1,
        bat_consistency_sum=t1_bat_cons,
        bowl_consistency_sum=t2_bowl_cons,
        bat_form_sum=t1_bat_form,
        bowl_form_sum=t2_bowl_form,
        format_id=match_context.format_id,
        venue_id=match_context.venue_id,
        season_id=match_context.season_id,
        match_date_unix=match_date_unix,
        opposition_id=match_context.team1_opposition_id,
        temp=match_context.temp,
        wind=match_context.wind,
        rain=match_context.rain,
        humidity=match_context.humidity,
        cloud=match_context.cloud,
        pressure=match_context.pressure,
        viscosity=match_context.viscosity,
    )
    inn2_runs, inn2_wkts = predict_innings(
        scaler_inn,
        model_inn,
        inning_number=2,
        bat_consistency_sum=t2_bat_cons,
        bowl_consistency_sum=t1_bowl_cons,
        bat_form_sum=t2_bat_form,
        bowl_form_sum=t1_bowl_form,
        format_id=match_context.format_id,
        venue_id=match_context.venue_id,
        season_id=match_context.season_id,
        match_date_unix=match_date_unix,
        opposition_id=match_context.team2_opposition_id,
        temp=match_context.temp,
        wind=match_context.wind,
        rain=match_context.rain,
        humidity=match_context.humidity,
        cloud=match_context.cloud,
        pressure=match_context.pressure,
        viscosity=match_context.viscosity,
    )
    return inn1_runs, inn1_wkts, inn2_runs, inn2_wkts


_TEAM1_ID = 1
_TEAM2_ID = 2


# ---------------------------------------------------------------------------
# Shared helpers for predict_players_with_features & predict_players_batch
# ---------------------------------------------------------------------------


@dataclass
class _ResolvedModels:
    """Resolved batting/bowling model pairs for a prediction item."""

    scaler_bat: Any
    model_bat: Any
    scaler_bowl: Any
    model_bowl: Any
    use_share: bool
    fmt_upper: str


def _resolve_prediction_model_pairs(
    fmt: str,
    models_dir: str,
    enable_train_on_the_fly: bool,
    go_app_url: str,
    go_app_api_key: Optional[str],
    train_latest_cache_granularity: str,
    cutoff: datetime,
    use_latest_model: bool,
    player_count: int,
    *,
    has_match_context: bool,
) -> _ResolvedModels:
    """Resolve batting/bowling model pairs for a format, with train-on-the-fly fallback."""
    fmt_upper = (fmt or "").strip().upper()
    use_share = (
        use_share_models_config()
        and has_match_context
        and ((INNINGS_MODELS.get(fmt_upper) if fmt_upper else None) or INNINGS_MODELS.get("_LEGACY_")) is not None
    )
    if use_share:
        bat_pair = (BAT_SHARE_MODELS.get(fmt_upper) if fmt_upper else None) or BAT_SHARE_MODELS.get("_LEGACY_")
        bowl_pair = (BOWL_SHARE_MODELS.get(fmt_upper) if fmt_upper else None) or BOWL_SHARE_MODELS.get("_LEGACY_")
    else:
        bat_pair = BAT_MODELS.get(fmt_upper) if fmt_upper else None
        bowl_pair = BOWL_MODELS.get(fmt_upper) if fmt_upper else None
    if not bat_pair or not bowl_pair:
        if not enable_train_on_the_fly:
            logger.error(
                "backtest_predict.train_on_the_fly.disabled",
                format=fmt_upper,
                hint="Train-on-the-fly is disabled. Pre-train and load artifacts for this format, or set ENABLE_TRAIN_ON_THE_FLY=1.",
            )
            raise ValueError(
                "No artifacts loaded for format=%s. Train-on-the-fly is disabled (resource consuming). "
                "Pre-train models for this format, or set ENABLE_TRAIN_ON_THE_FLY=1 to allow on-the-fly training."
                % fmt_upper
            )
        if not go_app_url:
            logger.error(
                "backtest_predict.train_on_the_fly.missing_go_app_url",
                format=fmt_upper,
                hint="Set GO_APP_URL to the go-app base URL for train-on-the-fly.",
            )
            raise ValueError(
                "GO_APP_URL is required for train-on-the-fly when no artifacts are loaded for format=%s" % fmt_upper
            )
        _cutoff_tz = cutoff if cutoff.tzinfo else cutoff.replace(tzinfo=timezone.utc)
        cutoff_iso = _cutoff_tz.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
        if use_latest_model:
            now_utc = datetime.now(timezone.utc)
            rounded = round_datetime_to_granularity(now_utc, train_latest_cache_granularity)
            cutoff_iso = rounded.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
            logger.info(
                "backtest_predict.train_on_the_fly.use_latest",
                format=fmt_upper,
                training_cutoff_iso=cutoff_iso,
                cache_granularity=train_latest_cache_granularity,
            )
        logger.warning(
            "backtest_predict.train_on_the_fly.triggered",
            msg="TRAIN_ON_THE_FLY: starting (CPU/RAM intensive) - fetches data from go-app and trains models in memory",
            fmt=fmt_upper,
            cutoff_iso=cutoff_iso,
            go_app_url=go_app_url,
            player_count=player_count,
        )
        bat_pair, bowl_pair = train_on_the_fly_cached(go_app_url, fmt_upper, cutoff_iso, go_app_api_key)

    scaler_bat, model_bat = bat_pair
    scaler_bowl, model_bowl = bowl_pair
    return _ResolvedModels(
        scaler_bat=scaler_bat,
        model_bat=model_bat,
        scaler_bowl=scaler_bowl,
        model_bowl=model_bowl,
        use_share=use_share,
        fmt_upper=fmt_upper,
    )


def _build_batting_feature_matrix(
    player_ids: List[int],
    cutoff: datetime,
    fmt_upper: str,
    features_map: Dict[str, Dict[str, float]],
    models_dir: str,
) -> np.ndarray:
    """Build unscaled batting feature matrix for a list of players."""
    bat_features = [
        build_batting_features_from_map(
            pid,
            cutoff,
            fmt_upper,
            features_map.get(str(pid)) or features_map.get(str(int(pid))) or {},
        )
        for pid in player_ids
    ]
    try:
        from ml.feature_transforms import build_extended_vector_from_features, load_transform_config_from_metadata

        bat_transform = load_transform_config_from_metadata(models_dir, "batting", fmt_upper)
        base_names = get_feature_names("batting")
        if bat_transform.get("add_interactions") or bat_transform.get("add_log1p"):
            bat_vecs = []
            for i, f in enumerate(bat_features):
                fm = features_map.get(str(player_ids[i])) or features_map.get(str(int(player_ids[i]))) or {}
                fm_for_interactions = {n: getattr(f, n) for n in base_names}
                fm_for_interactions.update(fm)
                base_vals = batting_feature_vector(f)
                ext = build_extended_vector_from_features(base_vals, base_names, fm_for_interactions, bat_transform)
                bat_vecs.append(ext)
            return np.array(bat_vecs, dtype=float)
        return np.array([batting_feature_vector(f) for f in bat_features], dtype=float)
    except Exception as e:
        logger.exception("predict.feature_transform.failed", error=str(e))
        raise HTTPException(
            status_code=500,
            detail="Feature transformation failed; prediction pipeline cannot proceed with incorrect feature data.",
        ) from e


def _build_bowling_feature_matrix(
    player_ids: List[int],
    cutoff: datetime,
    fmt_upper: str,
    features_map: Dict[str, Dict[str, float]],
    models_dir: str,
) -> np.ndarray:
    """Build unscaled bowling feature matrix for a list of players."""
    bowl_features = [
        build_bowling_features_from_map(
            pid,
            cutoff,
            fmt_upper,
            features_map.get(str(pid)) or features_map.get(str(int(pid))) or {},
        )
        for pid in player_ids
    ]
    try:
        from ml.feature_transforms import build_extended_vector_from_features, load_transform_config_from_metadata

        bowl_transform = load_transform_config_from_metadata(models_dir, "bowling", fmt_upper)
        base_names_bowl = get_feature_names("bowling")
        if bowl_transform.get("add_interactions") or bowl_transform.get("add_log1p"):
            bowl_vecs = []
            for i, f in enumerate(bowl_features):
                fm = features_map.get(str(player_ids[i])) or features_map.get(str(int(player_ids[i]))) or {}
                base_vals = bowling_feature_vector(f)
                fm_for_interactions = dict(zip(base_names_bowl, base_vals))
                fm_for_interactions.update(fm)
                ext = build_extended_vector_from_features(
                    base_vals, base_names_bowl, fm_for_interactions, bowl_transform
                )
                bowl_vecs.append(ext)
            return np.array(bowl_vecs, dtype=float)
        return np.array([bowling_feature_vector(f) for f in bowl_features], dtype=float)
    except Exception as e:
        logger.exception("predict.bowling_feature_transform.failed", error=str(e))
        raise HTTPException(
            status_code=500,
            detail="Bowling feature transformation failed; prediction pipeline cannot proceed with incorrect feature data.",
        ) from e


def _build_fielding_feature_matrix(
    player_ids: List[int],
    cutoff: datetime,
    fmt_upper: str,
    features_map: Dict[str, Dict[str, float]],
) -> np.ndarray:
    """Build unscaled fielding feature matrix for a list of players."""
    field_features_list = [
        build_fielding_features_from_map(
            int(pid),
            cutoff,
            fmt_upper,
            features_map.get(str(pid)) or features_map.get(str(int(pid))) or {},
        )
        for pid in player_ids
    ]
    return np.array([fielding_feature_vector(f) for f in field_features_list], dtype=float)


def _assemble_player_predictions(
    Y_bat: np.ndarray,
    Y_bowl: np.ndarray,
    player_ids: List[int],
    use_share: bool,
    match_context: Optional[MatchContext],
    cutoff: datetime,
    fmt_upper: str,
    features_map: Dict[str, Dict[str, float]],
    *,
    Y_fld: Optional[np.ndarray] = None,
) -> List[BacktestPlayerPred]:
    """Convert raw model outputs into BacktestPlayerPred list.

    Handles share-model conversion, fielding merge, and hybrid reconciliation.
    """
    inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = 0.0, 0.0, 0.0, 0.0
    if use_share and match_context is not None:
        match_date_unix = float(cutoff.timestamp()) if cutoff else 0.0
        predicted = predict_match_innings(match_context, features_map, fmt_upper, match_date_unix)
        if predicted is not None:
            inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = predicted

    team1_ids = {int(pid) for pid in (match_context.team1_player_ids or [])} if match_context else set()
    team2_ids = {int(pid) for pid in (match_context.team2_player_ids or [])} if match_context else set()

    out: List[BacktestPlayerPred] = []
    default_econ = get_prediction_defaults()["economy"] if get_prediction_defaults else 6.0
    for i, pid in enumerate(player_ids):
        row_bat = np.atleast_1d(Y_bat[i]).ravel()
        row_bowl = np.atleast_1d(Y_bowl[i]).ravel()
        vals_bat = list(row_bat) + [0.0] * max(0, 5 - len(row_bat))
        vals_bowl = list(row_bowl) + [0.0] * max(
            0, 3 - len(row_bowl)
        )  # runs or runs_share, balls, wickets or wickets_share

        if use_share and team1_ids and team2_ids and (inn1_runs > 0 or inn2_runs > 0):
            runs_share_bat = float(max(0.0, min(1.0, vals_bat[0])))
            runs_share_bowl = float(max(0.0, min(1.0, vals_bowl[0]))) if len(vals_bowl) > 0 else 0.0
            wickets_share = float(max(0.0, min(1.0, vals_bowl[2]))) if len(vals_bowl) > 2 else 0.0
            balls = float(max(0.0, vals_bat[1])) if len(vals_bat) > 1 else None
            balls_bowled = float(max(0.0, vals_bowl[1])) if len(vals_bowl) > 1 else 6.0
            in_team1 = int(pid) in team1_ids
            runs = runs_share_bat * (inn1_runs if in_team1 else inn2_runs)
            wickets = wickets_share * (inn2_wkts if in_team1 else inn1_wkts)
            r_conceded = runs_share_bowl * (inn2_runs if in_team1 else inn1_runs)
            economy = (r_conceded / (balls_bowled / 6.0)) if balls_bowled > 0 else default_econ
        else:
            runs = float(max(0.0, vals_bat[0]))
            balls = float(max(0.0, vals_bat[1])) if len(vals_bat) > 1 else None
            wickets = float(max(0.0, vals_bowl[2])) if len(vals_bowl) > 2 else 0.0
            r_conceded = float(max(0.0, vals_bowl[0])) if len(vals_bowl) > 0 else 0.0
            balls_bowled = float(max(0.0, vals_bowl[1])) if len(vals_bowl) > 1 else 6.0
            economy = (r_conceded / (balls_bowled / 6.0)) if balls_bowled > 0 else default_econ

        fours = float(max(0.0, vals_bat[2])) if len(vals_bat) > 2 else None
        sixes = float(max(0.0, vals_bat[3])) if len(vals_bat) > 3 else None
        catches, run_outs = 0.0, 0.0
        out.append(
            BacktestPlayerPred(
                player_id=int(pid),
                runs=runs,
                balls=balls,
                fours=fours,
                sixes=sixes,
                wickets=wickets,
                economy=economy,
                catches=catches,
                run_outs=run_outs,
            )
        )

    if Y_fld is not None:
        out_new: List[BacktestPlayerPred] = []
        for i, pred in enumerate(out):
            row_fld = np.atleast_1d(Y_fld[i]).ravel()
            vals_fld = list(row_fld) + [0.0] * max(0, 3 - len(row_fld))
            catches = float(max(0.0, vals_fld[0])) if len(vals_fld) > 0 else 0.0
            run_outs = float(max(0.0, vals_fld[1])) if len(vals_fld) > 1 else 0.0
            out_new.append(
                BacktestPlayerPred(
                    player_id=pred.player_id,
                    runs=pred.runs,
                    balls=pred.balls,
                    fours=pred.fours,
                    sixes=pred.sixes,
                    wickets=pred.wickets,
                    economy=pred.economy,
                    catches=catches,
                    run_outs=run_outs,
                )
            )
        out = out_new

    if match_context is not None and not use_share:
        match_date_unix = float(cutoff.timestamp()) if cutoff else 0.0
        predicted = predict_match_innings(match_context, features_map, fmt_upper, match_date_unix)
        if predicted is not None:
            inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = predicted
            team1_ids = {int(pid) for pid in match_context.team1_player_ids}
            team2_ids = {int(pid) for pid in match_context.team2_player_ids}
            if apply_constraint_reconciliation_from_backtest_preds is not None:
                out, adj = apply_constraint_reconciliation_from_backtest_preds(
                    out,
                    team1_ids,
                    team2_ids,
                    inn1_runs,
                    inn1_wkts,
                    inn2_runs,
                    inn2_wkts,
                    match_id=0,
                    format_code=fmt_upper,
                    default_economy=default_econ,
                )
                logger.info(
                    "backtest_predict.reconciliation.applied",
                    format=fmt_upper,
                    innings1_runs=inn1_runs,
                    innings2_runs=inn2_runs,
                    innings1_wickets=inn1_wkts,
                    innings2_wickets=inn2_wkts,
                    mean_abs_delta_runs=adj.get("mean_abs_delta_runs"),
                    mean_abs_delta_wickets=adj.get("mean_abs_delta_wickets"),
                    mean_abs_pct_delta_runs=adj.get("mean_abs_pct_delta_runs"),
                    mean_abs_pct_delta_wickets=adj.get("mean_abs_pct_delta_wickets"),
                    total_before_runs=adj.get("total_before_runs"),
                    total_before_wickets=adj.get("total_before_wickets"),
                )
                if adj.get("violations"):
                    logger.warning(
                        "backtest_predict.reconciliation.violations",
                        violations=adj["violations"],
                    )
            else:
                out = rescale_player_predictions(
                    out,
                    team1_ids,
                    team2_ids,
                    inn1_runs,
                    inn1_wkts,
                    inn2_runs,
                    inn2_wkts,
                    default_economy=default_econ,
                )
                logger.info(
                    "backtest_predict.reconciliation.applied",
                    innings1_runs=inn1_runs,
                    innings2_runs=inn2_runs,
                    innings1_wickets=inn1_wkts,
                    innings2_wickets=inn2_wkts,
                )

    return out


# ---------------------------------------------------------------------------
# Public prediction functions
# ---------------------------------------------------------------------------


def predict_players_with_features(
    cutoff: datetime,
    player_ids: List[int],
    fmt: str,
    features_map: Dict[str, Dict[str, float]],
    models_dir: str,
    enable_train_on_the_fly: bool,
    go_app_url: str,
    go_app_api_key: Optional[str],
    train_latest_cache_granularity: str,
    use_latest_model: bool = False,
    match_context: Optional[MatchContext] = None,
) -> List[BacktestPlayerPred]:
    """Run full pipeline: build feature objects from map, run batting/bowling models, return predictions.

    When no pre-trained artifacts are loaded for the format, trains on the fly from go-app training data.
    """
    resolved = _resolve_prediction_model_pairs(
        fmt,
        models_dir,
        enable_train_on_the_fly,
        go_app_url,
        go_app_api_key,
        train_latest_cache_granularity,
        cutoff,
        use_latest_model,
        len(player_ids),
        has_match_context=match_context is not None,
    )

    X_bat = _build_batting_feature_matrix(player_ids, cutoff, resolved.fmt_upper, features_map, models_dir)
    if resolved.scaler_bat is not None:
        X_bat = resolved.scaler_bat.transform(X_bat)
    Y_bat = resolved.model_bat.predict(X_bat)

    X_bowl = _build_bowling_feature_matrix(player_ids, cutoff, resolved.fmt_upper, features_map, models_dir)
    if resolved.scaler_bowl is not None:
        X_bowl = resolved.scaler_bowl.transform(X_bowl)
    Y_bowl = resolved.model_bowl.predict(X_bowl)

    Y_fld = None
    field_pair = (FIELD_MODELS.get(resolved.fmt_upper) if resolved.fmt_upper else None) or FIELD_MODELS.get("_LEGACY_")
    if field_pair is not None:
        scaler_fld, model_fld = field_pair
        X_fld = _build_fielding_feature_matrix(player_ids, cutoff, resolved.fmt_upper, features_map)
        if scaler_fld is not None:
            X_fld = scaler_fld.transform(X_fld)
        Y_fld = model_fld.predict(X_fld)

    return _assemble_player_predictions(
        Y_bat,
        Y_bowl,
        player_ids,
        resolved.use_share,
        match_context,
        cutoff,
        resolved.fmt_upper,
        features_map,
        Y_fld=Y_fld,
    )


def predict_players_batch(
    items: List[BatchPredictItem],
    models_dir: str,
    enable_train_on_the_fly: bool,
    go_app_url: str,
    go_app_api_key: Optional[str],
    train_latest_cache_granularity: str,
) -> "List[List[BacktestPlayerPred]]":
    """Run predictions for all items, aggregating feature matrices for efficient batched model.predict() calls.

    Groups items that share the same model into a single feature matrix, performs one
    model.predict() call per group, then partitions results back to individual items
    for per-item post-processing (share-model conversion, reconciliation, etc.).
    """
    if not items:
        return []

    # Phase 1: Resolve models and build unscaled feature matrices per item.
    resolved_list: List[_ResolvedModels] = []
    X_bat_list: List[np.ndarray] = []
    X_bowl_list: List[np.ndarray] = []
    for item in items:
        resolved = _resolve_prediction_model_pairs(
            item.format or "",
            models_dir,
            enable_train_on_the_fly,
            go_app_url,
            go_app_api_key,
            train_latest_cache_granularity,
            item.cutoff_date,
            item.use_latest_model,
            len(item.player_ids),
            has_match_context=item.match_context is not None,
        )
        resolved_list.append(resolved)
        X_bat_list.append(
            _build_batting_feature_matrix(
                item.player_ids,
                item.cutoff_date,
                resolved.fmt_upper,
                item.features or {},
                models_dir,
            )
        )
        X_bowl_list.append(
            _build_bowling_feature_matrix(
                item.player_ids,
                item.cutoff_date,
                resolved.fmt_upper,
                item.features or {},
                models_dir,
            )
        )

    # Phase 2: Group items by model identity, concatenate features, predict once per group.
    n_items = len(items)
    Y_bat_per_item: List[Optional[np.ndarray]] = [None] * n_items
    Y_bowl_per_item: List[Optional[np.ndarray]] = [None] * n_items
    Y_fld_per_item: List[Optional[np.ndarray]] = [None] * n_items

    def _batched_scale_and_predict(
        model_key_fn,
        X_list: List[np.ndarray],
        scaler_fn,
        model_fn,
        Y_per_item: List[Optional[np.ndarray]],
    ) -> None:
        """Group items by model identity, concatenate, scale, predict, partition."""
        groups: Dict[int, List[int]] = {}
        for idx in range(n_items):
            groups.setdefault(model_key_fn(idx), []).append(idx)
        for _key, indices in groups.items():
            sample_idx = indices[0]
            X_all = np.concatenate([X_list[i] for i in indices])
            scaler = scaler_fn(sample_idx)
            if scaler is not None:
                X_all = scaler.transform(X_all)
            Y_all = model_fn(sample_idx).predict(X_all)
            offset = 0
            for idx in indices:
                n_rows = X_list[idx].shape[0]
                Y_per_item[idx] = Y_all[offset : offset + n_rows]
                offset += n_rows

    _batched_scale_and_predict(
        lambda idx: id(resolved_list[idx].model_bat),
        X_bat_list,
        lambda idx: resolved_list[idx].scaler_bat,
        lambda idx: resolved_list[idx].model_bat,
        Y_bat_per_item,
    )
    _batched_scale_and_predict(
        lambda idx: id(resolved_list[idx].model_bowl),
        X_bowl_list,
        lambda idx: resolved_list[idx].scaler_bowl,
        lambda idx: resolved_list[idx].model_bowl,
        Y_bowl_per_item,
    )

    # Fielding: build features per item, then batch predict across items sharing the same model.
    X_fld_list: List[Optional[np.ndarray]] = [None] * n_items
    fld_pairs: List[Optional[Tuple[Any, Any]]] = [None] * n_items
    for idx in range(n_items):
        fmt_upper = resolved_list[idx].fmt_upper
        field_pair = (FIELD_MODELS.get(fmt_upper) if fmt_upper else None) or FIELD_MODELS.get("_LEGACY_")
        if field_pair is not None:
            fld_pairs[idx] = field_pair
            X_fld_list[idx] = _build_fielding_feature_matrix(
                items[idx].player_ids,
                items[idx].cutoff_date,
                fmt_upper,
                items[idx].features or {},
            )

    fld_groups: Dict[int, List[int]] = {}
    for idx in range(n_items):
        if fld_pairs[idx] is not None:
            _scaler_fld, model_fld = fld_pairs[idx]  # type: ignore[misc]
            fld_groups.setdefault(id(model_fld), []).append(idx)
    for _key, indices in fld_groups.items():
        scaler_fld, model_fld = fld_pairs[indices[0]]  # type: ignore[misc]
        X_all = np.concatenate([X_fld_list[i] for i in indices])  # type: ignore[arg-type]
        if scaler_fld is not None:
            X_all = scaler_fld.transform(X_all)
        Y_all = model_fld.predict(X_all)
        offset = 0
        for idx in indices:
            n_rows = X_fld_list[idx].shape[0]  # type: ignore[union-attr]
            Y_fld_per_item[idx] = Y_all[offset : offset + n_rows]
            offset += n_rows

    # Phase 3: Post-process per item (share-model conversion, reconciliation, etc.).
    results: List[List[BacktestPlayerPred]] = []
    for idx in range(n_items):
        preds = _assemble_player_predictions(
            Y_bat_per_item[idx],  # type: ignore[arg-type]
            Y_bowl_per_item[idx],  # type: ignore[arg-type]
            items[idx].player_ids,
            resolved_list[idx].use_share,
            items[idx].match_context,
            items[idx].cutoff_date,
            resolved_list[idx].fmt_upper,
            items[idx].features or {},
            Y_fld=Y_fld_per_item[idx],
        )
        results.append(preds)
    return results


def generate_match(
    cutoff: datetime,
    player_ids: List[int],
    fmt: str,
    features_map: Dict[str, Dict[str, float]],
    match_context: MatchContext,
    settings: GenerateMatchSettings,
    use_latest_model: bool = False,
    model_version: str = "",
) -> Dict[str, Any]:
    """Produce reconciled scorecards and win probability for a single match (§5.1.1).

    Calls predict_players_with_features with match_context (so reconciliation runs),
    aggregates team totals, builds WinFeatures from context + features, runs win model,
    and logs win coherence (margin vs model win prob) for monitoring (§3.3.3).
    """
    preds = predict_players_with_features(
        cutoff,
        player_ids,
        fmt,
        features_map,
        settings.models_dir,
        settings.enable_train_on_the_fly,
        settings.go_app_url,
        settings.go_app_api_key,
        settings.train_latest_cache_granularity,
        use_latest_model,
        match_context=match_context,
    )
    team1_ids = {int(pid) for pid in match_context.team1_player_ids}
    team2_ids = {int(pid) for pid in match_context.team2_player_ids}
    team1_runs = sum(p.runs for p in preds if p.player_id in team1_ids)
    team2_runs = sum(p.runs for p in preds if p.player_id in team2_ids)
    inn1_wickets = sum(p.wickets or 0 for p in preds if p.player_id in team2_ids)
    inn2_wickets = sum(p.wickets or 0 for p in preds if p.player_id in team1_ids)
    innings = [
        InningsSummary(inning_number=1, runs=team1_runs, wickets=float(inn1_wickets)),
        InningsSummary(inning_number=2, runs=team2_runs, wickets=float(inn2_wickets)),
    ]
    p_team1 = 0.5
    if aggregate_team_features_from_player_maps is not None and features_map:
        t1_feats = {pid: features_map.get(pid, {}) for pid in team1_ids if pid in features_map}
        t2_feats = {pid: features_map.get(pid, {}) for pid in team2_ids if pid in features_map}
        from .models import WinFeaturesEnhanced

        match_ctx_for_win = WinFeaturesEnhanced(
            format_id=match_context.format_id,
            venue_id=match_context.venue_id,
            match_date_unix=0.0,
            team1_opposition_id=match_context.team1_opposition_id,
            team2_opposition_id=match_context.team2_opposition_id,
            toss_winner_opposition_id=0,
            temp=match_context.temp,
            wind=match_context.wind,
            rain=match_context.rain,
            humidity=match_context.humidity,
            cloud=match_context.cloud,
            pressure=match_context.pressure,
            viscosity=match_context.viscosity,
            team1_player_features={},
            team2_player_features={},
        ).to_match_context_dict()
        try:
            result = run_win_prediction_enhanced(
                fmt=(fmt or "").strip().upper(),
                match_context=match_ctx_for_win,
                team1_player_features={str(k): v for k, v in t1_feats.items()},
                team2_player_features={str(k): v for k, v in t2_feats.items()},
            )
            p_team1 = result.team1_win_probability
        except Exception:
            logger.warning("generate_match.enhanced_win_failed, falling back to legacy")
            p_team1 = 0.5
    if p_team1 == 0.5:
        try:
            from ml.win_features_from_reconciled import build_win_features_standardized
        except ImportError:
            build_win_features_standardized = None
        if build_win_features_standardized is not None:
            t1_bat_cons, t1_bowl_cons = _sum_team_feature(
                features_map, team1_ids, "batting_std_w10", "bowling_std_w10"
            )
            t1_bat_form, t1_bowl_form = _sum_team_feature(features_map, team1_ids, "batting_mean_w5", "bowling_mean_w5")
            t2_bat_cons, t2_bowl_cons = _sum_team_feature(
                features_map, team2_ids, "batting_std_w10", "bowling_std_w10"
            )
            t2_bat_form, t2_bowl_form = _sum_team_feature(features_map, team2_ids, "batting_mean_w5", "bowling_mean_w5")
            wf = build_win_features_standardized(
                format_code=fmt,
                format_id=int(match_context.format_id),
                venue_id=int(match_context.venue_id),
                season_id=int(match_context.season_id),
                team1_opposition_id=int(match_context.team1_opposition_id),
                team2_opposition_id=int(match_context.team2_opposition_id),
                toss_winner_opposition_id=0,
                team1_bat_consistency_sum=t1_bat_cons,
                team1_bowl_consistency_sum=t1_bowl_cons,
                team2_bat_consistency_sum=t2_bat_cons,
                team2_bowl_consistency_sum=t2_bowl_cons,
                team1_bat_form_sum=t1_bat_form,
                team1_bowl_form_sum=t1_bowl_form,
                team2_bat_form_sum=t2_bat_form,
                team2_bowl_form_sum=t2_bowl_form,
            )
            win_preds = run_win_prediction([wf])
            if win_preds:
                p_team1 = win_preds[0].team1_win_probability
    margin = team1_runs - team2_runs
    try:
        from ml.win_coherence_metrics import win_probability_coherence_from_margin

        scale = 25.0
        if get_win_coherence_config is not None:
            try:
                wc_cfg = get_win_coherence_config()
                scale = float(wc_cfg.get("scale", scale))
            except Exception:
                # Fall back to hardcoded default if config is missing or invalid.
                scale = 25.0

        coh = win_probability_coherence_from_margin(p_team1, margin, scale=scale)
        logger.info(
            "win_coherence.metrics",
            format=(fmt or "").strip().upper(),
            p_model_team1=coh.get("p_model_team1"),
            p_implied_team1=coh.get("p_implied_team1"),
            abs_diff=coh.get("abs_diff"),
            margin=margin,
        )
    except Exception:
        pass
    return {
        "players": preds,
        "innings": innings,
        "win_probability_team1": p_team1,
        "model_version": model_version,
    }


# ---------------------------------------------------------------------------
# Individual predict endpoint logic (batting, bowling, extras, win)
# ---------------------------------------------------------------------------


def resolve_model_pair(
    registry: Dict[str, Any],
    fmt: str,
    domain: str,
) -> Tuple[Any, Any]:
    """Resolve (scaler, model) pair from registry by format, with legacy fallback.

    Raises HTTPException if no model is available.
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
    pair = registry.get("_LEGACY_")
    if not pair:
        logger.info(f"predict.{domain}.rejected", reason="missing_format_no_legacy")
        raise HTTPException(
            status_code=400,
            detail=error_payload(
                code="MISSING_FORMAT",
                message=f"Missing 'format' and no legacy {domain} model loaded",
                hint="Set 'format' in the request or train legacy artifacts.",
            ),
        )
    return pair


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
    logger.info(
        "predict.batting.start", batch=len(features), format=fmt or ("LEGACY" if "_LEGACY_" in BAT_MODELS else "")
    )
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
    logger.info(
        "predict.bowling.start", batch=len(features), format=fmt or ("LEGACY" if "_LEGACY_" in BOWL_MODELS else "")
    )
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
    """Build feature vector in EXTRAS_FEATURE_COLS order (exclude 'format' key)."""
    if not EXTRAS_FEATURE_COLS:
        return np.zeros(0)
    d = f.model_dump()
    return np.array([float(d.get(c, 0)) for c in EXTRAS_FEATURE_COLS], dtype=float)


def win_feature_vector(f: WinFeatures) -> np.ndarray:
    """Build feature vector for the enhanced win model from a WinFeatures instance.

    When the enhanced feature module is available, pads the scalar WinFeatures
    fields into the full WIN_ENHANCED_FEATURE_COLS vector (distribution stats
    default to 0, derived features computed from what's available).
    Falls back to simple scalar vector if enhanced module is not loaded.
    """
    if not WIN_ENHANCED_FEATURE_COLS:
        return np.zeros(0)
    d = f.model_dump()
    if compute_derived_features is not None:
        derived = compute_derived_features(d)
        d.update(derived)
    return np.array([float(d.get(c, 0)) for c in WIN_ENHANCED_FEATURE_COLS], dtype=float)


def run_extras_prediction(features: List[ExtrasFeatures]) -> List[ExtrasPrediction]:
    """Execute extras prediction pipeline and return typed results."""
    fmt = (features[0].format or "").strip().upper()
    model = EXTRAS_MODELS.get(fmt) if fmt else EXTRAS_MODELS.get("_LEGACY_")
    if not model:
        available = [k for k in EXTRAS_MODELS.keys() if k != "_LEGACY_"]
        logger.warning("predict.extras.model_not_loaded", format=fmt or "LEGACY", available=available)
        raise HTTPException(
            status_code=404,
            detail=error_payload(
                code="MODEL_NOT_LOADED",
                message="Extras model not loaded",
                hint="Train extras artifacts (e.g. make train-extras) and ensure format matches or use legacy.",
                available=available,
            ),
        )
    X = np.array([extras_feature_vector(f) for f in features], dtype=float)
    if X.size == 0:
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="FEATURE_ORDER_EMPTY", message="EXTRAS_FEATURE_COLS not available"),
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
    """Resolve win model by format with legacy fallback. Raises HTTPException if missing."""
    model = WIN_MODELS.get(fmt) if fmt else WIN_MODELS.get("_LEGACY_")
    if not model:
        available = [k for k in WIN_MODELS.keys() if k != "_LEGACY_"]
        logger.warning("predict.win.model_not_loaded", format=fmt or "LEGACY", available=available)
        raise HTTPException(
            status_code=404,
            detail=error_payload(
                code="MODEL_NOT_LOADED",
                message="Win model not loaded",
                hint="Train win artifacts (e.g. make train-win) and ensure format matches or use legacy.",
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
    if aggregate_team_features_from_player_maps is None or build_feature_vector is None:
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="ENHANCED_WIN_NOT_AVAILABLE",
                message="ml.win_features module not loaded",
                hint="Ensure ml-service has the win_features module installed.",
            ),
        )

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

    feature_dict = aggregate_team_features_from_player_maps(t1_feats, t2_feats, match_context)
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

    if aggregate_team_features_from_player_maps is None or build_feature_vector is None:
        raise HTTPException(
            status_code=500,
            detail=error_payload(
                code="ENHANCED_WIN_NOT_AVAILABLE",
                message="ml.win_features module not loaded",
                hint="Ensure ml-service has the win_features module installed.",
            ),
        )

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
        max_iterations=max_iterations,
        max_evals=max_evals,
    )
