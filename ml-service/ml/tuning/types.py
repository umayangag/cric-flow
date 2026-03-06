"""Constants, column definitions, and shared types for the tuning package."""

from __future__ import annotations

from typing import Any, Dict, List, Optional

# Phase 1 trials per algorithm; Phase 2 Optuna trials (when Optuna available)
PHASE1_TRIALS_PER_ALGORITHM = 5
PHASE2_TRIALS = 40

# Algorithm keys for filtering
AVAILABLE_ALGORITHMS = frozenset({"rf", "gb", "et", "hgb", "quantile", "stacked", "mlp"})

# ── Sequence columns ────────────────────────────────────────────────────

BAT_SEQ_COLS = [
    "bat_prev_sr",
    "bat_prev_out_rate",
    "bat_window_sr_12_pp",
    "bat_window_boundary_rate_12_pp",
    "bat_entry_sr_1_6",
    "bat_set_sr_13_30",
    "bat_react_after_dot_sr",
    "bat_after_k_dots_boundary_p_k2",
]
BOWL_SEQ_COLS = [
    "bowl_prev_wkt_rate",
    "bowl_window_econ_24_death",
    "bowl_window_wkt_rate_24_death",
    "bowl_extras_wide_rate_pp",
    "bowl_react_after_boundary_wkt_rate_next",
    "bowl_spell_first_over_wkt_rate",
    "bowl_over_ball1_wkt_rate",
    "bowl_over_ball6_wkt_rate",
]

# ── Feature / target column lists ───────────────────────────────────────

BAT_RAW_STAT_COLS = [
    "batting_mean_w3",
    "batting_mean_w5",
    "batting_mean_w10",
    "batting_mean_w20",
    "batting_std_w5",
    "batting_std_w10",
    "batting_max_w10",
    "batting_min_w10",
    "batting_median_w10",
    "batting_last_1",
    "batting_last_2",
    "batting_last_3",
    "batting_career_mean",
    "batting_career_count",
    "batting_pct_zero_w10",
    "batting_trend_w5",
    "batting_days_since_last",
    "batting_innings_in_last_90d",
]
BOWL_RAW_STAT_COLS = [
    "bowling_mean_w3",
    "bowling_mean_w5",
    "bowling_mean_w10",
    "bowling_mean_w20",
    "bowling_std_w5",
    "bowling_std_w10",
    "bowling_max_w10",
    "bowling_min_w10",
    "bowling_median_w10",
    "bowling_last_1",
    "bowling_last_2",
    "bowling_last_3",
    "bowling_career_mean",
    "bowling_career_count",
    "bowling_pct_zero_w10",
    "bowling_trend_w5",
    "bowling_days_since_last",
    "bowling_innings_in_last_90d",
]

BATTING_FEATURE_COLS = (
    [
        "batting_consistency",
        "batting_form",
        "batting_form_short",
        "batting_form_long",
        "batting_momentum",
    ]
    + BAT_RAW_STAT_COLS
    + [
        "temp",
        "wind",
        "rain",
        "humidity",
        "cloud",
        "pressure",
        "viscosity",
        "inning",
        "batting_session",
        "toss",
        "batting_venue",
        "batting_opposition",
        "season_id",
        "match_date_unix",
    ]
    + BAT_SEQ_COLS
)
BATTING_TARGET_COLS = ["runs", "balls", "fours", "sixes", "batting_position"]

BOWLING_FEATURE_COLS = (
    [
        "bowling_consistency",
        "bowling_form",
        "bowling_momentum",
        "bowling_career_avg",
    ]
    + BOWL_RAW_STAT_COLS
    + [
        "temp",
        "wind",
        "rain",
        "humidity",
        "cloud",
        "pressure",
        "viscosity",
        "inning",
        "bowling_session",
        "toss",
        "bowling_venue",
        "bowling_opposition",
        "season_id",
        "match_date_unix",
    ]
    + BOWL_SEQ_COLS
)
BOWLING_TARGET_COLS = ["runs", "balls", "wickets"]

# ── Target names registry (model_kind -> target column names) ───────────

_TARGET_NAMES_BY_KIND: Dict[str, List[str]] = {
    "batting": BATTING_TARGET_COLS,
    "bowling": BOWLING_TARGET_COLS,
}

# Lazily add fielding/innings if modules available
try:
    from ml import train_fielding as _train_fielding_mod

    if hasattr(_train_fielding_mod, "FIELDING_TARGET_COLS"):
        _TARGET_NAMES_BY_KIND["fielding"] = getattr(_train_fielding_mod, "FIELDING_TARGET_COLS")
except ImportError:
    pass
try:
    from ml import train_innings as _train_innings_mod

    if hasattr(_train_innings_mod, "INNINGS_TARGET_COLS"):
        _TARGET_NAMES_BY_KIND["innings"] = getattr(_train_innings_mod, "INNINGS_TARGET_COLS")
except ImportError:
    pass


def target_names_for_model(model_kind: str) -> Optional[List[str]]:
    """Return target column names for per-target MAE when available."""
    return _TARGET_NAMES_BY_KIND.get(model_kind)


# ── Optional training module references (for data loaders) ─────────────

_train_fielding: Any = None
_train_extras: Any = None
_train_win: Any = None
_train_innings: Any = None

try:
    from ml import train_fielding as _tf

    _train_fielding = _tf
except ImportError:
    pass
try:
    from ml import train_extras as _te

    _train_extras = _te
except ImportError:
    pass
try:
    from ml import train_win as _tw

    _train_win = _tw
except ImportError:
    pass
try:
    from ml import train_innings as _ti

    _train_innings = _ti
except ImportError:
    pass

# ── Phase 1 coarse parameter grids ─────────────────────────────────────

_PHASE1_COARSE_RF = {
    "est__estimator__n_estimators": [50, 150, 300, 500],
    "est__estimator__max_depth": [6, 12, 20],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_GB = {
    "est__estimator__n_estimators": [50, 150, 300, 500],
    "est__estimator__max_depth": [4, 8, 12],
    "est__estimator__learning_rate": [0.05, 0.15],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_ET = {
    "est__estimator__n_estimators": [50, 150, 300, 500],
    "est__estimator__max_depth": [6, 12, 20],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_HGB = {
    "est__estimator__max_iter": [100, 200, 300],
    "est__estimator__max_depth": [4, 8, 12],
    "est__estimator__learning_rate": [0.05, 0.15],
    "est__estimator__min_samples_leaf": [4, 8, 12, 16],
}
_PHASE1_COARSE_MLP_REG = {
    "est__estimator__hidden_layer_sizes": [(64, 64), (128, 64), (128, 128, 64)],
    "est__estimator__activation": ["relu"],
    "est__estimator__alpha": [0.0001, 0.001],
    "est__estimator__max_iter": [500, 1000],
}
_PHASE1_COARSE_MLP_CLF = {
    "est__hidden_layer_sizes": [(64, 64), (128, 64)],
    "est__activation": ["relu"],
    "est__alpha": [0.0001, 0.001],
    "est__max_iter": [500, 1000],
}
