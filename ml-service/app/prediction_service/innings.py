"""Innings totals prediction for hybrid reconciliation."""

from __future__ import annotations

from typing import Dict, Optional, Tuple

from ..artifacts import INNINGS_MODELS
from ..models import MatchContext
from ..reconciliation import predict_innings


def sum_team_feature(
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
    # Resolve sidecar metadata for the same registry key chosen above so inference uses the
    # exact feature order and derived-feature weights pinned at training time.
    from app.artifacts import INNINGS_META  # local import to avoid a cycle at module load

    meta_inn = None
    if fmt_upper and INNINGS_MODELS.get(fmt_upper) is not None:
        meta_inn = INNINGS_META.get(fmt_upper)
    if meta_inn is None:
        meta_inn = INNINGS_META.get("_LEGACY_")
    team1_ids = {int(pid) for pid in match_context.team1_player_ids}
    team2_ids = {int(pid) for pid in match_context.team2_player_ids}

    # v2: consistency = std_w10, form = mean_w5 (from feature_raw_stats_snapshots)
    t1_bat_cons, t1_bowl_cons = sum_team_feature(features_map, team1_ids, "batting_std_w10", "bowling_std_w10")
    t1_bat_form, t1_bowl_form = sum_team_feature(features_map, team1_ids, "batting_mean_w5", "bowling_mean_w5")
    t2_bat_cons, t2_bowl_cons = sum_team_feature(features_map, team2_ids, "batting_std_w10", "bowling_std_w10")
    t2_bat_form, t2_bowl_form = sum_team_feature(features_map, team2_ids, "batting_mean_w5", "bowling_mean_w5")
    inn1_runs, inn1_wkts = predict_innings(
        scaler_inn,
        model_inn,
        inning_number=1,
        bat_consistency_sum=t1_bat_cons,
        bowl_consistency_sum=t2_bowl_cons,
        bat_form_sum=t1_bat_form,
        bowl_form_sum=t2_bowl_form,
        venue_id=match_context.venue_id,
        season_id=match_context.season_id,
        opposition_id=match_context.team1_opposition_id,
        match_date_unix=match_date_unix,
        temp=match_context.temp,
        wind=match_context.wind,
        rain=match_context.rain,
        humidity=match_context.humidity,
        cloud=match_context.cloud,
        pressure=match_context.pressure,
        viscosity=match_context.viscosity,
        format_code=fmt_upper,
        meta=meta_inn,
    )
    inn2_runs, inn2_wkts = predict_innings(
        scaler_inn,
        model_inn,
        inning_number=2,
        bat_consistency_sum=t2_bat_cons,
        bowl_consistency_sum=t1_bowl_cons,
        bat_form_sum=t2_bat_form,
        bowl_form_sum=t1_bowl_form,
        venue_id=match_context.venue_id,
        season_id=match_context.season_id,
        opposition_id=match_context.team2_opposition_id,
        match_date_unix=match_date_unix,
        temp=match_context.temp,
        wind=match_context.wind,
        rain=match_context.rain,
        humidity=match_context.humidity,
        cloud=match_context.cloud,
        pressure=match_context.pressure,
        viscosity=match_context.viscosity,
        format_code=fmt_upper,
        meta=meta_inn,
    )
    return inn1_runs, inn1_wkts, inn2_runs, inn2_wkts
