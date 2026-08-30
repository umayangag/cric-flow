"""Generate-match orchestration (reconciled scorecard + win probability)."""

from __future__ import annotations

from datetime import datetime
from typing import Any, Dict, List

from ml.config import get_win_coherence_config
from ml.win_coherence_metrics import win_probability_coherence_from_margin
from ml.win_features_from_reconciled import build_win_features_standardized

from ..logging import get_struct_logger
from ..models.backtest import InningsSummary, MatchContext
from ..prediction_settings import GenerateMatchSettings
from .endpoints import run_win_prediction, run_win_prediction_enhanced
from .innings import sum_team_feature
from .players import predict_players_with_features

logger = get_struct_logger()


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
    if features_map:
        t1_feats = {pid: features_map.get(pid, {}) for pid in team1_ids if pid in features_map}
        t2_feats = {pid: features_map.get(pid, {}) for pid in team2_ids if pid in features_map}
        from .models.predict import WinFeaturesEnhanced

        match_ctx_for_win = WinFeaturesEnhanced(
            format_id=match_context.format_id,
            venue_id=match_context.venue_id,
            team1_opposition_id=match_context.team1_opposition_id,
            team2_opposition_id=match_context.team2_opposition_id,
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
        t1_bat_cons, t1_bowl_cons = sum_team_feature(features_map, team1_ids, "batting_std_w10", "bowling_std_w10")
        t1_bat_form, t1_bowl_form = sum_team_feature(features_map, team1_ids, "batting_mean_w5", "bowling_mean_w5")
        t2_bat_cons, t2_bowl_cons = sum_team_feature(features_map, team2_ids, "batting_std_w10", "bowling_std_w10")
        t2_bat_form, t2_bowl_form = sum_team_feature(features_map, team2_ids, "batting_mean_w5", "bowling_mean_w5")
        wf = build_win_features_standardized(
            format_code=fmt,
            format_id=int(match_context.format_id),
            venue_id=int(match_context.venue_id),
            team1_opposition_id=int(match_context.team1_opposition_id),
            team2_opposition_id=int(match_context.team2_opposition_id),
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
        scale = 25.0
        try:
            wc_cfg = get_win_coherence_config()
            scale = float(wc_cfg.get("scale", scale))
        except Exception:
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
