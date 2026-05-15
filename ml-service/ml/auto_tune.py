"""Backward-compatible shim — all logic lives in ml.tuning package.

Usage unchanged:
    python -m ml.auto_tune --model batting --csv ...
    from ml.auto_tune import run_auto_tune, AVAILABLE_ALGORITHMS, ...

AGENTS: When modifying auto-tune behavior, keep it uniform across all ML algorithms
and all match formats. Do not add logic that applies only to one algorithm (e.g. quantile)
or only to one format (e.g. T20). Prefer data- or config-driven logic so the same code path
handles every model and format. See ml/tuning/AGENTS_AUTO_TUNE.md for the full note.
"""

from __future__ import annotations

# ── Tuning config helper (used by tests) ────────────────────────────────
from ml.config import get_tuning_config as _get_tuning_config  # noqa: F401

# ── Re-export CLI ───────────────────────────────────────────────────────
from ml.tuning.cli import main  # noqa: F401

# ── Re-export cv / metrics ──────────────────────────────────────────────
from ml.tuning.cv_metrics import (  # noqa: F401
    _add_final_report_details,
    _compute_learning_curve_regression,
    _compute_metrics_classification,
    _compute_metrics_regression,
    _compute_mlqa_audit,
    _effective_n_jobs,
    _effective_timeseries_gap,
    _extract_feature_importance,
    _feature_names_for_mlqa_report,
    _get_cv_object,
    _mlqa_feature_names,
)

# ── Re-export data loaders ──────────────────────────────────────────────
from ml.tuning.data_loaders import (  # noqa: F401
    LoaderResult,
    _load_via_csv_or_api,
    _sort_df_by_match_date,
    _sort_rows_by_match_date,
    feature_matrix_after_training_transforms,
    load_batting_csv,
    load_batting_from_api,
    load_bowling_csv,
    load_bowling_from_api,
    load_extras_csv,
    load_extras_from_api,
    load_fielding_csv,
    load_fielding_from_api,
    load_innings_from_api,
    load_win_csv,
    load_win_from_api,
)

# ── Re-export optuna search runners ─────────────────────────────────────
from ml.tuning.optuna_search import (  # noqa: F401
    _count_combinations,
    _run_search,
    _run_search_classification,
    _run_search_single_regression,
    _run_search_two_phase,
    _run_search_two_phase_single_regression,
    _save_artifacts,
    _save_artifacts_model_only,
)

# ── Re-export public runners ────────────────────────────────────────────
from ml.tuning.runners import (  # noqa: F401
    _maybe_run_autogluon_and_compare,
    _maybe_run_pycaret_ranking,
    _run_search_two_phase_classification,
    run_auto_tune,
    run_auto_tune_extras,
    run_auto_tune_win,
)

# ── Re-export search space ──────────────────────────────────────────────
from ml.tuning.search_space import (  # noqa: F401
    _build_pipeline,
    _build_pipeline_single_regression,
    _coarse_to_single_prefix,
    _get_prior_tuned_algorithm,
    _normalize_hidden_layer_sizes,
    _phase1_candidates_classification,
    _phase1_candidates_regression,
    _phase1_candidates_regression_single,
    _prior_params_to_optuna_regression,
    _search_space_classification,
    _search_space_regression,
    _search_space_regression_single,
    _to_pipeline_params,
    _to_pipeline_params_single,
)

# ── Re-export types / constants ─────────────────────────────────────────
from ml.tuning.types import (  # noqa: F401
    _PHASE1_COARSE_ET,
    _PHASE1_COARSE_GB,
    _PHASE1_COARSE_HGB,
    _PHASE1_COARSE_MLP_CLF,
    _PHASE1_COARSE_MLP_REG,
    _PHASE1_COARSE_RF,
    AVAILABLE_ALGORITHMS,
    BAT_SEQ_COLS,
    BATTING_FEATURE_COLS,
    BATTING_TARGET_COLS,
    BOWL_SEQ_COLS,
    BOWLING_FEATURE_COLS,
    BOWLING_TARGET_COLS,
    PHASE1_TRIALS_PER_ALGORITHM,
    PHASE2_TRIALS,
)

if __name__ == "__main__":
    main()
