"""Enhanced win-prediction feature definitions and aggregation.

This module replaces the old sum-only approach (ml.win_features_from_reconciled)
with distribution-aware features: for each of 8 feature groups
(team1/team2 x bat/bowl x consistency/form), the model receives sum, mean, std,
max, min, top3_mean, and count -- plus derived matchup and depth features.

Two entry points:
1. **Training**: Features come from the Go-app CSV export, which now includes
   pre-computed distribution statistics per feature group.  The training script
   just references ``WIN_ENHANCED_FEATURE_COLS`` for the column list.
2. **Inference**: Features are computed on-the-fly from per-player feature maps
   via ``aggregate_team_features_from_player_maps()``.
"""

from __future__ import annotations

import math
from typing import Dict, List, Mapping, Sequence

import numpy as np

# ---------------------------------------------------------------------------
# Feature group / column definitions (must match Go-app win export headers)
# ---------------------------------------------------------------------------

_FEATURE_GROUPS = [
    "team1_bat_consistency",
    "team1_bowl_consistency",
    "team2_bat_consistency",
    "team2_bowl_consistency",
    "team1_bat_form",
    "team1_bowl_form",
    "team2_bat_form",
    "team2_bowl_form",
]

_DIST_SUFFIXES = ["_sum", "_mean", "_std", "_max", "_min", "_top3_mean", "_count"]

MATCH_CONTEXT_COLS = [
    "format_id",
    "venue_id",
    "match_date_unix",
    "team1_opposition_id",
    "team2_opposition_id",
    "toss_winner_opposition_id",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
]

_DIST_FEATURE_COLS: List[str] = []
for _grp in _FEATURE_GROUPS:
    for _sfx in _DIST_SUFFIXES:
        _DIST_FEATURE_COLS.append(_grp + _sfx)

DERIVED_FEATURE_COLS = [
    "bat_form_matchup_ratio_team1",
    "bat_form_matchup_ratio_team2",
    "bat_cons_matchup_ratio_team1",
    "bat_cons_matchup_ratio_team2",
    "bowl_depth_diff",
    "bat_form_top3_diff",
    "bowl_form_top3_diff",
    "bat_cons_top3_diff",
    "bowl_cons_top3_diff",
    "team1_bat_form_spread",
    "team2_bat_form_spread",
    "team1_bowl_form_spread",
    "team2_bowl_form_spread",
]

WIN_ENHANCED_FEATURE_COLS: List[str] = MATCH_CONTEXT_COLS + _DIST_FEATURE_COLS + DERIVED_FEATURE_COLS

WIN_TARGET_COL = "team1_wins"


def _safe_ratio(numerator: float, denominator: float, default: float = 1.0) -> float:
    if abs(denominator) < 1e-9:
        return default
    return numerator / denominator


def compute_derived_features(row: Mapping[str, float]) -> Dict[str, float]:
    """Compute matchup, depth, and spread features from base distribution stats.

    ``row`` must contain the MATCH_CONTEXT_COLS + _DIST_FEATURE_COLS keys.
    Returns a dict with DERIVED_FEATURE_COLS keys.
    """
    g = row.get

    # Matchup ratios: how team's batting form compares to opposition bowling form.
    # Higher -> batting team is stronger relative to bowling attack.
    bat_form_matchup_t1 = _safe_ratio(g("team1_bat_form_mean", 0.0), g("team2_bowl_form_mean", 0.0))
    bat_form_matchup_t2 = _safe_ratio(g("team2_bat_form_mean", 0.0), g("team1_bowl_form_mean", 0.0))
    bat_cons_matchup_t1 = _safe_ratio(g("team1_bat_consistency_mean", 0.0), g("team2_bowl_consistency_mean", 0.0))
    bat_cons_matchup_t2 = _safe_ratio(g("team2_bat_consistency_mean", 0.0), g("team1_bowl_consistency_mean", 0.0))

    # Bowling depth: difference in number of bowlers between teams.
    bowl_depth_diff = g("team1_bowl_consistency_count", 0.0) - g("team2_bowl_consistency_count", 0.0)

    # Top-3 quality differences: which team has stronger top performers.
    bat_form_top3_diff = g("team1_bat_form_top3_mean", 0.0) - g("team2_bat_form_top3_mean", 0.0)
    bowl_form_top3_diff = g("team1_bowl_form_top3_mean", 0.0) - g("team2_bowl_form_top3_mean", 0.0)
    bat_cons_top3_diff = g("team1_bat_consistency_top3_mean", 0.0) - g("team2_bat_consistency_top3_mean", 0.0)
    bowl_cons_top3_diff = g("team1_bowl_consistency_top3_mean", 0.0) - g("team2_bowl_consistency_top3_mean", 0.0)

    # Spread (max - min): how evenly distributed skill is across the team.
    t1_bat_form_spread = g("team1_bat_form_max", 0.0) - g("team1_bat_form_min", 0.0)
    t2_bat_form_spread = g("team2_bat_form_max", 0.0) - g("team2_bat_form_min", 0.0)
    t1_bowl_form_spread = g("team1_bowl_form_max", 0.0) - g("team1_bowl_form_min", 0.0)
    t2_bowl_form_spread = g("team2_bowl_form_max", 0.0) - g("team2_bowl_form_min", 0.0)

    return {
        "bat_form_matchup_ratio_team1": bat_form_matchup_t1,
        "bat_form_matchup_ratio_team2": bat_form_matchup_t2,
        "bat_cons_matchup_ratio_team1": bat_cons_matchup_t1,
        "bat_cons_matchup_ratio_team2": bat_cons_matchup_t2,
        "bowl_depth_diff": bowl_depth_diff,
        "bat_form_top3_diff": bat_form_top3_diff,
        "bowl_form_top3_diff": bowl_form_top3_diff,
        "bat_cons_top3_diff": bat_cons_top3_diff,
        "bowl_cons_top3_diff": bowl_cons_top3_diff,
        "team1_bat_form_spread": t1_bat_form_spread,
        "team2_bat_form_spread": t2_bat_form_spread,
        "team1_bowl_form_spread": t1_bowl_form_spread,
        "team2_bowl_form_spread": t2_bowl_form_spread,
    }


# ---------------------------------------------------------------------------
# Inference-time aggregation from per-player feature maps
# ---------------------------------------------------------------------------


def _dist_stats_from_values(values: Sequence[float]) -> Dict[str, float]:
    """Compute sum, mean, std, max, min, top3_mean, count from a list of floats."""
    if not values:
        return {"sum": 0.0, "mean": 0.0, "std": 0.0, "max": 0.0, "min": 0.0, "top3_mean": 0.0, "count": 0}
    arr = np.array(values, dtype=np.float64)
    top3 = np.sort(arr)[-3:] if len(arr) >= 3 else arr
    return {
        "sum": float(np.sum(arr)),
        "mean": float(np.mean(arr)),
        "std": float(np.std(arr)),
        "max": float(np.max(arr)),
        "min": float(np.min(arr)),
        "top3_mean": float(np.mean(top3)),
        "count": len(arr),
    }


def aggregate_team_features_from_player_maps(
    team1_features: Mapping[int, Mapping[str, float]],
    team2_features: Mapping[int, Mapping[str, float]],
    match_context: Mapping[str, float],
) -> Dict[str, float]:
    """Build the full enhanced win feature vector from per-player feature maps.

    Used at inference time when the Go-app passes all player features to the ML
    service.  The returned dict contains all keys in ``WIN_ENHANCED_FEATURE_COLS``.

    Args:
        team1_features: {player_id: {feature_name: value}} for team 1 (batting inn 1).
        team2_features: {player_id: {feature_name: value}} for team 2 (batting inn 2).
        match_context: Dict with keys from MATCH_CONTEXT_COLS.
    """
    result: Dict[str, float] = {}
    for k in MATCH_CONTEXT_COLS:
        result[k] = float(match_context.get(k, 0.0))

    _PLAYER_KEY_MAP = {
        "team1_bat_consistency": ("batting_consistency", team1_features),
        "team1_bowl_consistency": ("bowling_consistency", team1_features),
        "team2_bat_consistency": ("batting_consistency", team2_features),
        "team2_bowl_consistency": ("bowling_consistency", team2_features),
        "team1_bat_form": ("batting_form", team1_features),
        "team1_bowl_form": ("bowling_form", team1_features),
        "team2_bat_form": ("batting_form", team2_features),
        "team2_bowl_form": ("bowling_form", team2_features),
    }

    for group_name, (player_key, team_feats) in _PLAYER_KEY_MAP.items():
        values = [fm.get(player_key, 0.0) for fm in team_feats.values()]
        stats = _dist_stats_from_values(values)
        for sfx_name, sfx_key in zip(_DIST_SUFFIXES, ["sum", "mean", "std", "max", "min", "top3_mean", "count"]):
            col = group_name + sfx_name
            val = stats[sfx_key]
            result[col] = float(val) if not (isinstance(val, float) and math.isnan(val)) else 0.0

    derived = compute_derived_features(result)
    result.update(derived)

    return result


def build_feature_vector(feature_dict: Mapping[str, float]) -> List[float]:
    """Convert a feature dict (from aggregation or CSV row) into an ordered list
    matching ``WIN_ENHANCED_FEATURE_COLS``."""
    return [float(feature_dict.get(c, 0.0)) for c in WIN_ENHANCED_FEATURE_COLS]
