"""Backtest player prediction: model resolution, feature matrices, batching."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
from fastapi import HTTPException

from ml.config import get_prediction_defaults
from ml.reconciliation_adapter import apply_constraint_reconciliation_from_backtest_preds

from ..artifacts import (
    BAT_MODELS,
    BAT_SHARE_MODELS,
    BOWL_MODELS,
    BOWL_SHARE_MODELS,
    FIELD_MODELS,
    INNINGS_MODELS
)
from ..artifacts import _use_share_models as use_share_models_config
from ..backtest_service import (
    build_batting_features_from_map,
    build_bowling_features_from_map,
    build_fielding_features_from_map
)
from ..feature_config import get_feature_names
from ..features import batting_feature_vector, bowling_feature_vector, fielding_feature_vector
from ..logging import get_struct_logger
from ..models import BacktestPlayerPred, BatchPredictItem, MatchContext
from ..prediction_settings import round_datetime_to_granularity
from ..train_on_the_fly import train_on_the_fly_cached
from .innings import predict_match_innings

logger = get_struct_logger()

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
    has_match_context: bool
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
                hint="Train-on-the-fly is disabled. Pre-train and load artifacts for this format, or set ENABLE_TRAIN_ON_THE_FLY=1."
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
                hint="Set GO_APP_URL to the go-app base URL for train-on-the-fly."
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
                cache_granularity=train_latest_cache_granularity
            )
        logger.warning(
            "backtest_predict.train_on_the_fly.triggered",
            msg="TRAIN_ON_THE_FLY: starting (CPU/RAM intensive) - fetches data from go-app and trains models in memory",
            fmt=fmt_upper,
            cutoff_iso=cutoff_iso,
            go_app_url=go_app_url,
            player_count=player_count
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
        fmt_upper=fmt_upper
    )

def _build_batting_feature_matrix(
    player_ids: List[int],
    cutoff: datetime,
    fmt_upper: str,
    features_map: Dict[str, Dict[str, float]],
    models_dir: str
) -> np.ndarray:
    """Build unscaled batting feature matrix for a list of players."""
    bat_features = [
        build_batting_features_from_map(
            pid,
            cutoff,
            fmt_upper,
            features_map.get(str(pid)) or features_map.get(str(int(pid))) or {}
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
            detail="Feature transformation failed; prediction pipeline cannot proceed with incorrect feature data."
        ) from e

def _build_bowling_feature_matrix(
    player_ids: List[int],
    cutoff: datetime,
    fmt_upper: str,
    features_map: Dict[str, Dict[str, float]],
    models_dir: str
) -> np.ndarray:
    """Build unscaled bowling feature matrix for a list of players."""
    bowl_features = [
        build_bowling_features_from_map(
            pid,
            cutoff,
            fmt_upper,
            features_map.get(str(pid)) or features_map.get(str(int(pid))) or {}
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
            detail="Bowling feature transformation failed; prediction pipeline cannot proceed with incorrect feature data."
        ) from e

def _build_fielding_feature_matrix(
    player_ids: List[int],
    cutoff: datetime,
    fmt_upper: str,
    features_map: Dict[str, Dict[str, float]]
) -> np.ndarray:
    """Build unscaled fielding feature matrix for a list of players."""
    field_features_list = [
        build_fielding_features_from_map(
            int(pid),
            cutoff,
            fmt_upper,
            features_map.get(str(pid)) or features_map.get(str(int(pid))) or {}
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
    Y_fld: Optional[np.ndarray] = None
) -> List[BacktestPlayerPred]:
    """Convert raw model outputs into BacktestPlayerPred list.

    Handles share-model conversion, fielding merge, and hybrid reconciliation.
    """
    inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = 0.0, 0.0, 0.0, 0.0
    if use_share and match_context is not None:
        predicted = predict_match_innings(match_context, features_map, fmt_upper)
        if predicted is not None:
            inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = predicted

    team1_ids = {int(pid) for pid in (match_context.team1_player_ids or [])} if match_context else set()
    team2_ids = {int(pid) for pid in (match_context.team2_player_ids or [])} if match_context else set()

    out: List[BacktestPlayerPred] = []
    default_econ = get_prediction_defaults()["economy"]
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
                run_outs=run_outs
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
                    run_outs=run_outs
                )
            )
        out = out_new

    if match_context is not None and not use_share:
        predicted = predict_match_innings(match_context, features_map, fmt_upper)
        if predicted is not None:
            inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = predicted
            team1_ids = {int(pid) for pid in match_context.team1_player_ids}
            team2_ids = {int(pid) for pid in match_context.team2_player_ids}
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
                default_economy=default_econ
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
                total_before_wickets=adj.get("total_before_wickets")
            )
            if adj.get("violations"):
                logger.warning(
                    "backtest_predict.reconciliation.violations",
                    violations=adj["violations"]
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
    match_context: Optional[MatchContext] = None
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
        has_match_context=match_context is not None
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
        Y_fld=Y_fld
    )

def predict_players_batch(
    items: List[BatchPredictItem],
    models_dir: str,
    enable_train_on_the_fly: bool,
    go_app_url: str,
    go_app_api_key: Optional[str],
    train_latest_cache_granularity: str
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
            has_match_context=item.match_context is not None
        )
        resolved_list.append(resolved)
        X_bat_list.append(
            _build_batting_feature_matrix(
                item.player_ids,
                item.cutoff_date,
                resolved.fmt_upper,
                item.features or {},
                models_dir
            )
        )
        X_bowl_list.append(
            _build_bowling_feature_matrix(
                item.player_ids,
                item.cutoff_date,
                resolved.fmt_upper,
                item.features or {},
                models_dir
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
        Y_per_item: List[Optional[np.ndarray]]
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
        Y_bat_per_item
    )
    _batched_scale_and_predict(
        lambda idx: id(resolved_list[idx].model_bowl),
        X_bowl_list,
        lambda idx: resolved_list[idx].scaler_bowl,
        lambda idx: resolved_list[idx].model_bowl,
        Y_bowl_per_item
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
                items[idx].features or {}
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
            Y_fld=Y_fld_per_item[idx]
        )
        results.append(preds)
    return results
