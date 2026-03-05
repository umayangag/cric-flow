"""Core prediction logic for backtest and player-level predictions.

Extracted from app.main to keep route handlers thin. Contains:
- Feature transform pipeline (batting, bowling, fielding)
- Train-on-the-fly fallback when no pre-trained artifacts exist
- Innings prediction and hybrid reconciliation
- Match-level and player-level prediction orchestration
"""

from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple

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
    from ml.train_win import WIN_FEATURE_COLS
except ImportError:
    WIN_FEATURE_COLS = []

logger = get_struct_logger()


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
) -> Optional[Tuple[float, float, float, float]]:
    """Predict innings runs and wickets for both innings. Returns (inn1_runs, inn1_wkts, inn2_runs, inn2_wkts) or None if no model."""
    innings_pair = (INNINGS_MODELS.get(fmt_upper) if fmt_upper else None) or INNINGS_MODELS.get("_LEGACY_")
    if innings_pair is None:
        return None
    scaler_inn, model_inn = innings_pair
    team1_ids = {int(pid) for pid in match_context.team1_player_ids}
    team2_ids = {int(pid) for pid in match_context.team2_player_ids}

    t1_bat_cons, t1_bowl_cons = _sum_team_feature(features_map, team1_ids, "batting_consistency", "bowling_consistency")
    t1_bat_form, t1_bowl_form = _sum_team_feature(features_map, team1_ids, "batting_form", "bowling_form")
    t2_bat_cons, t2_bowl_cons = _sum_team_feature(features_map, team2_ids, "batting_consistency", "bowling_consistency")
    t2_bat_form, t2_bowl_form = _sum_team_feature(features_map, team2_ids, "batting_form", "bowling_form")
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
    fmt_upper = (fmt or "").strip().upper()
    use_share = (
        use_share_models_config()
        and match_context is not None
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
            player_count=len(player_ids),
        )
        bat_pair, bowl_pair = train_on_the_fly_cached(go_app_url, fmt_upper, cutoff_iso, go_app_api_key)

    scaler_bat, model_bat = bat_pair
    scaler_bowl, model_bowl = bowl_pair

    bat_features: List[BattingFeatures] = []
    bowl_features: List[BowlingFeatures] = []
    for pid in player_ids:
        fm = features_map.get(str(pid)) or features_map.get(str(int(pid))) or {}
        bat_features.append(build_batting_features_from_map(pid, cutoff, fmt_upper, fm))
        bowl_features.append(build_bowling_features_from_map(pid, cutoff, fmt_upper, fm))

    # Feature order must match training (configs/feature_vectors.json). Apply feature_transforms if in metadata.
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
            X_bat = np.array(bat_vecs, dtype=float)
        else:
            X_bat = np.array([batting_feature_vector(f) for f in bat_features], dtype=float)
    except Exception as e:
        logger.exception("predict.feature_transform.failed", error=str(e))
        raise HTTPException(
            status_code=500,
            detail="Feature transformation failed; prediction pipeline cannot proceed with incorrect feature data.",
        ) from e
    if scaler_bat is not None:
        X_bat = scaler_bat.transform(X_bat)
    Y_bat = model_bat.predict(X_bat)

    try:
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
            X_bowl = np.array(bowl_vecs, dtype=float)
        else:
            X_bowl = np.array([bowling_feature_vector(f) for f in bowl_features], dtype=float)
    except Exception as e:
        logger.exception("predict.bowling_feature_transform.failed", error=str(e))
        raise HTTPException(
            status_code=500,
            detail="Bowling feature transformation failed; prediction pipeline cannot proceed with incorrect feature data.",
        ) from e
    if scaler_bowl is not None:
        X_bowl = scaler_bowl.transform(X_bowl)
    Y_bowl = model_bowl.predict(X_bowl)

    # Phase 3 share path: predict innings first when use_share so we can multiply shares
    inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = 0.0, 0.0, 0.0, 0.0
    if use_share and match_context is not None:
        predicted = predict_match_innings(match_context, features_map, fmt_upper)
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

    # Fielding: if we have fielding artifacts, predict catches/run_outs and merge into player preds
    field_pair = (FIELD_MODELS.get(fmt_upper) if fmt_upper else None) or FIELD_MODELS.get("_LEGACY_")
    if field_pair is not None:
        scaler_fld, model_fld = field_pair
        field_features_list = [
            build_fielding_features_from_map(
                int(pid), cutoff, fmt_upper, features_map.get(str(pid)) or features_map.get(str(int(pid))) or {}
            )
            for pid in player_ids
        ]
        X_fld = np.array([fielding_feature_vector(f) for f in field_features_list], dtype=float)
        if scaler_fld is not None:
            X_fld = scaler_fld.transform(X_fld)
        Y_fld = model_fld.predict(X_fld)
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

    # Hybrid reconciliation: when match_context and innings model are available, run inference-time reconciliation
    if match_context is not None and not use_share:
        predicted = predict_match_innings(match_context, features_map, fmt_upper)
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


def generate_match(
    cutoff: datetime,
    player_ids: List[int],
    fmt: str,
    features_map: Dict[str, Dict[str, float]],
    match_context: MatchContext,
    models_dir: str,
    enable_train_on_the_fly: bool,
    go_app_url: str,
    go_app_api_key: Optional[str],
    train_latest_cache_granularity: str,
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
        models_dir,
        enable_train_on_the_fly,
        go_app_url,
        go_app_api_key,
        train_latest_cache_granularity,
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
    try:
        from ml.win_features_from_reconciled import build_win_features_standardized
    except ImportError:
        build_win_features_standardized = None
    p_team1 = 0.5
    if build_win_features_standardized is not None:
        t1_bat_cons, t1_bowl_cons = _sum_team_feature(
            features_map, team1_ids, "batting_consistency", "bowling_consistency"
        )
        t1_bat_form, t1_bowl_form = _sum_team_feature(features_map, team1_ids, "batting_form", "bowling_form")
        t2_bat_cons, t2_bowl_cons = _sum_team_feature(
            features_map, team2_ids, "batting_consistency", "bowling_consistency"
        )
        t2_bat_form, t2_bowl_form = _sum_team_feature(features_map, team2_ids, "batting_form", "bowling_form")
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
    """Build feature vector in WIN_FEATURE_COLS order (exclude 'format' key)."""
    if not WIN_FEATURE_COLS:
        return np.zeros(0)
    d = f.model_dump()
    return np.array([float(d.get(c, 0)) for c in WIN_FEATURE_COLS], dtype=float)


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


def run_win_prediction(features: List[WinFeatures]) -> List[WinPrediction]:
    """Execute win prediction pipeline and return typed results."""
    fmt = (features[0].format or "").strip().upper()
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
    X = np.array([win_feature_vector(f) for f in features], dtype=float)
    if X.size == 0:
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="FEATURE_ORDER_EMPTY", message="WIN_FEATURE_COLS not available"),
        )
    try:
        proba = model.predict_proba(X)
        if proba.shape[1] > 1:
            p_team1 = proba[:, 1]
        else:
            p_team1 = proba.ravel() if model.classes_[0] == 1 else 1.0 - proba.ravel()
        return [WinPrediction(team1_win_probability=float(p)) for p in p_team1]
    except Exception as exc:
        logger.exception("predict.win.error", error=str(exc))
        raise HTTPException(
            status_code=500,
            detail=error_payload(code="PREDICT_FAILED", message="Win prediction failed", hint="See server logs"),
        )
