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

import json
import math
import os
from typing import Dict, List, Mapping, Optional, Sequence, Tuple

import numpy as np


def _load_format_codes() -> List[str]:
    """Read ml.formats from config.json (or ML_SERVICE_CONFIG) for one-hot encoding.

    Falls back to a sensible default list when config is missing or invalid.
    """
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f)
        fmts = data.get("ml", {}).get("formats") or []
        out = [str(x).strip().upper() for x in fmts if isinstance(x, (str, int)) and str(x).strip()]
        if out:
            return out
    except Exception:
        pass
    # Fallback ordering is stable to keep column order deterministic
    return ["TEST", "ODI", "T20", "T20I"]


_FORMAT_CODES: List[str] = _load_format_codes()
_FORMAT_ONE_HOT_COLS: List[str] = [f"format_is_{code}" for code in _FORMAT_CODES] + ["format_is_OTHER"]


def get_format_codes() -> List[str]:
    """Return configured format codes for one-hot encoding (from ``ml.formats`` in config)."""
    return list(_FORMAT_CODES)


def get_format_one_hot_columns() -> List[str]:
    """Return ``format_is_<CODE>`` column names plus ``format_is_OTHER``."""
    return list(_FORMAT_ONE_HOT_COLS)


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

_DIST_STAT_KEYS = ["sum", "mean", "std", "max", "min", "top3_mean", "count"]

# Maps each export feature group to (player-level feature key, team number).
# team number is 1 or 2; used at inference time to look up the correct team's
# player feature maps. v2: use raw stat keys (std_w10 = consistency, mean_w5 = form).
_GROUP_TO_PLAYER_KEY: List[Tuple[str, str, int]] = [
    ("team1_bat_consistency", "batting_std_w10", 1),
    ("team1_bowl_consistency", "bowling_std_w10", 1),
    ("team2_bat_consistency", "batting_std_w10", 2),
    ("team2_bowl_consistency", "bowling_std_w10", 2),
    ("team1_bat_form", "batting_mean_w5", 1),
    ("team1_bowl_form", "bowling_mean_w5", 1),
    ("team2_bat_form", "batting_mean_w5", 2),
    ("team2_bowl_form", "bowling_mean_w5", 2),
]

# Base match context columns (before categorical expansion of format).
MATCH_CONTEXT_BASE_COLS = [
    "format_id",  # kept for compatibility but excluded from model features
    "venue_id",
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

# Full context column list including one-hot encoded format indicators. This is
# the superset used when aggregating from player maps and when building team
# optimisation batches.
MATCH_CONTEXT_COLS: List[str] = MATCH_CONTEXT_BASE_COLS + list(_FORMAT_ONE_HOT_COLS)

_DIST_FEATURE_COLS: List[str] = [grp + sfx for grp in _FEATURE_GROUPS for sfx in _DIST_SUFFIXES]

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


def format_one_hot_from_code(format_code: Optional[str]) -> Dict[str, float]:
    """Build one-hot mapping for a format code over :func:`get_format_one_hot_columns`."""
    out = {col: 0.0 for col in _FORMAT_ONE_HOT_COLS}
    if not format_code:
        out["format_is_OTHER"] = 1.0
        return out
    fmt = str(format_code).strip().upper()
    col = f"format_is_{fmt}"
    if col in out:
        out[col] = 1.0
    else:
        out["format_is_OTHER"] = 1.0
    return out


_MATCH_CONTEXT_FEATURE_COLS: List[str] = [c for c in MATCH_CONTEXT_COLS if c != "format_id"]
WIN_ENHANCED_FEATURE_COLS: List[str] = _MATCH_CONTEXT_FEATURE_COLS + _DIST_FEATURE_COLS + DERIVED_FEATURE_COLS

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
        return {"sum": 0.0, "mean": 0.0, "std": 0.0, "max": 0.0, "min": 0.0, "top3_mean": 0.0, "count": 0.0}
    arr = np.array(values, dtype=np.float64)
    top3 = np.sort(arr)[-3:] if len(arr) >= 3 else arr
    return {
        "sum": float(np.sum(arr)),
        "mean": float(np.mean(arr)),
        "std": float(np.std(arr)),
        "max": float(np.max(arr)),
        "min": float(np.min(arr)),
        "top3_mean": float(np.mean(top3)),
        "count": float(len(arr)),
    }


def aggregate_team_features_from_player_maps(
    team1_features: Mapping[int, Mapping[str, float]],
    team2_features: Mapping[int, Mapping[str, float]],
    match_context: Mapping[str, float],
    format_code: Optional[str] = None,
) -> Dict[str, float]:
    """Build the full enhanced win feature vector from per-player feature maps.

    Used at inference time when the Go-app passes all player features to the ML
    service.  The returned dict contains all keys in ``WIN_ENHANCED_FEATURE_COLS``.

    Args:
        team1_features: {player_id: {feature_name: value}} for team 1 (batting inn 1).
        team2_features: {player_id: {feature_name: value}} for team 2 (batting inn 2).
        match_context: Dict with keys from MATCH_CONTEXT_COLS.
    """
    team_by_number = {1: team1_features, 2: team2_features}
    result: Dict[str, float] = {}

    # Base context columns copied directly
    for k in MATCH_CONTEXT_BASE_COLS:
        result[k] = float(match_context.get(k, 0.0))

    # One-hot encoded format columns
    one_hot = format_one_hot_from_code(format_code)
    for k, v in one_hot.items():
        result[k] = float(v)

    for group_name, player_key, team_num in _GROUP_TO_PLAYER_KEY:
        team_feats = team_by_number[team_num]
        values = [fm.get(player_key, 0.0) for fm in team_feats.values()]
        stats = _dist_stats_from_values(values)
        for suffix, stat_key in zip(_DIST_SUFFIXES, _DIST_STAT_KEYS):
            val = stats[stat_key]
            result[group_name + suffix] = 0.0 if (isinstance(val, float) and math.isnan(val)) else float(val)

    result.update(compute_derived_features(result))
    return result


def build_feature_vector(feature_dict: Mapping[str, float]) -> List[float]:
    """Convert a feature dict (from aggregation or CSV row) into an ordered list
    matching ``WIN_ENHANCED_FEATURE_COLS``."""
    return [float(feature_dict.get(c, 0.0)) for c in WIN_ENHANCED_FEATURE_COLS]
