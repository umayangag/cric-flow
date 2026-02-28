"""
Auto-tune hyperparameters and model type for batting, bowling, fielding, extras, and win.

Two-phase approach:
  1. Phase 1 (screening): Coarse search over all algorithms (RF, GBM, ExtraTrees, HistGradientBoosting, etc.)
     to identify the best algorithm(s).
  2. Phase 2 (fine-tuning): Optuna TPE on the winner(s) with finer hyperparameter ranges for convergence.

Writes live progress to a JSON file for frontend display (algorithm, hyperparams, trial, etc.).
Saves the best scaler + model in the same format as train_* scripts, plus a tuning report JSON.
Extras and win save model only (no scaler).

Usage:
  # From CSV (per-format)
  python -m ml.auto_tune --model batting --csv ../output/go-app/batting_encoded_T20.csv --format T20
  python -m ml.auto_tune --model fielding --csv path/to/fielding.csv --format T20

  # From go-app API (required for extras and win)
  GO_APP_URL=http://localhost:8080 python -m ml.auto_tune --model extras --from-api --cutoff 2024-12-01T00:00:00Z --all-formats
  GO_APP_URL=http://localhost:8080 python -m ml.auto_tune --model win --from-api --cutoff 2024-12-01T00:00:00Z --all-formats

  # All models, all formats (extras/win from API only)
  python -m ml.auto_tune --model all --all-formats --from-api --cutoff 2024-12-01T00:00:00Z

  # Fine-tune one model for one format then apply best params to config (see docs/ml-and-training.md)

When prior tuned params exist (go-app or tuning_report_*.json), auto_tune skips algorithm
screening and only fine-tunes hyperparameters of the existing algorithm (incremental training).
Use --rescreen to force full algorithm re-evaluation.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Dict, List, Optional, Tuple

import numpy as np

logger = logging.getLogger(__name__)
import joblib
import pandas as pd
from sklearn.ensemble import (
    ExtraTreesClassifier,
    ExtraTreesRegressor,
    GradientBoostingClassifier,
    GradientBoostingRegressor,
    HistGradientBoostingClassifier,
    HistGradientBoostingRegressor,
    RandomForestClassifier,
    RandomForestRegressor,
    StackingRegressor,
)
from sklearn.linear_model import Ridge
from sklearn.metrics import (
    accuracy_score,
    explained_variance_score,
    f1_score,
    max_error,
    mean_absolute_error,
    mean_squared_error,
    median_absolute_error,
    precision_score,
    r2_score,
    recall_score,
    roc_auc_score,
)
from sklearn.model_selection import KFold, RandomizedSearchCV, TimeSeriesSplit, cross_val_predict, cross_val_score
from sklearn.multioutput import MultiOutputRegressor
from sklearn.neural_network import MLPClassifier, MLPRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

# Ensure ml-service root is on path for config and app imports
_ML_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _ML_ROOT not in sys.path:
    sys.path.insert(0, _ML_ROOT)

from ml import auto_tune_progress as _progress
from ml.config import (
    get_training_params,
    get_tuned_params_from_go_app,
    get_tuning_config,
    get_tuning_search_space,
    save_tuned_params_to_go_app,
)

try:
    from ml import auto_tune_pycaret as _pycaret
except ImportError:
    _pycaret = None
try:
    from ml import auto_tune_autogluon as _autogluon
    from ml.autogluon_wrapper import AutogluonPredictorWrapper
except ImportError:
    _autogluon = None
    AutogluonPredictorWrapper = None

try:
    import optuna

    _HAS_OPTUNA = True
except ImportError:
    _HAS_OPTUNA = False

# Phase 1 trials per algorithm; Phase 2 Optuna trials (when Optuna available)
PHASE1_TRIALS_PER_ALGORITHM = 5
PHASE2_TRIALS = 25

# Batting/bowling CSV loading inlined (train_batting/train_bowling require top-level config).
# Fielding / extras / win: optional imports for API data and rows_to_xy_by_format.
_train_fielding = None
_train_extras = None
_train_win = None
try:
    from ml import train_fielding as _train_fielding_mod

    _train_fielding = _train_fielding_mod
except ImportError:
    pass
try:
    from ml import train_extras as _train_extras_mod

    _train_extras = _train_extras_mod
except ImportError:
    pass
try:
    from ml import train_win as _train_win_mod

    _train_win = _train_win_mod
except ImportError:
    pass

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
BATTING_FEATURE_COLS = [
    "batting_consistency",
    "batting_form",
    "batting_form_short",
    "batting_form_long",
    "batting_momentum",
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
] + BAT_SEQ_COLS
BATTING_TARGET_COLS = ["runs", "balls", "fours", "sixes", "batting_position"]
BOWLING_FEATURE_COLS = [
    "bowling_consistency",
    "bowling_form",
    "bowling_form_short",
    "bowling_form_long",
    "bowling_momentum",
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
] + BOWL_SEQ_COLS
BOWLING_TARGET_COLS = ["runs", "balls", "wickets"]


def _get_tuning_config() -> Dict[str, Any]:
    return get_tuning_config()


def _maybe_run_pycaret_ranking(
    X: np.ndarray,
    Y: np.ndarray,
    task_type: str,
    use_pycaret: Optional[bool],
    model_kind: str,
    format_suffix: Optional[str],
    task_index: int,
    task_total: int,
) -> Optional[List[str]]:
    """Run PyCaret algorithm ranking when enabled. Returns algorithms override or None."""
    if use_pycaret is False:
        return None
    tuning = _get_tuning_config()
    stages = tuning.get("stages") or {}
    pycaret_cfg = stages.get("pycaret") or {}
    if not pycaret_cfg.get("enabled", False):
        return None
    if _pycaret is None or not _pycaret.is_available():
        return None
    n_select = int(pycaret_cfg.get("n_select", 3))
    cv_splits = tuning.get("cv_splits", 5)
    random_state = tuning.get("random_state", 42)
    _progress.write_progress(
        phase="pycaret",
        model_kind=model_kind,
        format_suffix=format_suffix,
        task_index=task_index,
        task_total=task_total,
        message="PyCaret algorithm ranking",
    )
    if task_type == "regression":
        y_use = Y[:, 0] if Y.ndim > 1 and Y.shape[1] > 1 else (Y.ravel() if Y.ndim > 1 else Y)
        algorithms, ranking, ok = _pycaret.run_pycaret_ranking_regression(
            X, y_use, cv_splits=cv_splits, n_select=n_select, random_state=random_state
        )
    else:
        y_use = np.asarray(Y).ravel().astype(int)
        algorithms, ranking, ok = _pycaret.run_pycaret_ranking_classification(
            X, y_use, cv_splits=cv_splits, n_select=n_select, random_state=random_state
        )
    if ok and algorithms:
        _progress.write_progress(
            phase="pycaret",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            message=f"PyCaret top algorithms: {algorithms}",
            algorithms_screened=algorithms,
        )
        return algorithms
    return None


def _maybe_run_autogluon_and_compare(
    X: np.ndarray,
    y: np.ndarray,
    task_type: str,
    use_autogluon: Optional[bool],
    model_kind: str,
    format_suffix: Optional[str],
    out_dir: str,
    optuna_best_score: float,
) -> Tuple[bool, Optional[Any], Dict[str, Any]]:
    """Run AutoGluon when enabled; compare to Optuna score. Returns (autogluon_wins, wrapper_or_none, report_updates)."""
    import shutil

    if use_autogluon is False or AutogluonPredictorWrapper is None:
        return False, None, {}
    tuning = _get_tuning_config()
    stages = tuning.get("stages") or {}
    ag_cfg = stages.get("autogluon") or {}
    if not ag_cfg.get("enabled", False):
        return False, None, {}
    models_list = ag_cfg.get("models") or []
    if model_kind not in models_list:
        return False, None, {}
    if _autogluon is None or not _autogluon.is_available():
        return False, None, {}

    time_limit = int(ag_cfg.get("time_limit_seconds", 300))
    presets = str(ag_cfg.get("presets", "medium_quality"))

    _progress.write_progress(
        phase="autogluon",
        model_kind=model_kind,
        format_suffix=format_suffix,
        task_index=0,
        task_total=1,
        message="Fitting AutoGluon",
    )

    if task_type == "regression":
        _, ag_score, persist_path, ok = _autogluon.run_autogluon_regression(
            X, y, time_limit_seconds=time_limit, presets=presets
        )
    else:
        _, ag_score, persist_path, ok = _autogluon.run_autogluon_classification(
            X, y, time_limit_seconds=time_limit, presets=presets
        )

    ag_better = ok and ag_score is not None and ag_score > optuna_best_score
    if not ok or not ag_better or not persist_path:
        return False, None, {"autogluon_tried": True, "autogluon_score": ag_score, "autogluon_wins": False}

    fmt = (format_suffix or "LEGACY").replace(" ", "_")
    ag_dir = os.path.join(out_dir, f"autogluon_{model_kind}_{fmt}")
    os.makedirs(out_dir, exist_ok=True)
    if os.path.isdir(ag_dir):
        shutil.rmtree(ag_dir)
    shutil.copytree(persist_path, ag_dir)

    wrapper = AutogluonPredictorWrapper(ag_dir, is_regression=(task_type == "regression"))
    return True, wrapper, {"autogluon_tried": True, "autogluon_score": ag_score, "autogluon_wins": True}


def _effective_n_jobs(tuning_cfg: Dict[str, Any], n_jobs_override: Optional[int] = None) -> int:
    """Resolve n_jobs: override if set, else config, else env AUTO_TUNE_N_JOBS. When result is -1, use resource-aware suggested_n_jobs('tuning') for multi-CPU."""
    if n_jobs_override is not None and n_jobs_override >= 1:
        return n_jobs_override
    n_jobs = tuning_cfg.get("n_jobs", 1)
    if os.environ.get("AUTO_TUNE_N_JOBS") is not None:
        try:
            n_jobs = int(os.environ["AUTO_TUNE_N_JOBS"])
        except ValueError:
            pass
    if n_jobs == -1:
        from ml.resources import suggested_n_jobs

        return max(1, suggested_n_jobs("tuning"))
    return max(1, int(n_jobs))


# Algorithm keys for filtering: rf, gb, et, hgb, quantile, stacked, mlp
AVAILABLE_ALGORITHMS = frozenset({"rf", "gb", "et", "hgb", "quantile", "stacked", "mlp"})


# Phase 1: coarse param grids for algorithm screening (few trials, large steps)
_PHASE1_COARSE_RF = {
    "est__estimator__n_estimators": [50, 150, 300],
    "est__estimator__max_depth": [6, 12, 20],
    "est__estimator__min_samples_leaf": [2, 8],
}
_PHASE1_COARSE_GB = {
    "est__estimator__n_estimators": [50, 150, 300],
    "est__estimator__max_depth": [4, 8, 12],
    "est__estimator__learning_rate": [0.05, 0.15],
    "est__estimator__min_samples_leaf": [2, 8],
}
_PHASE1_COARSE_ET = {
    "est__estimator__n_estimators": [50, 150, 300],
    "est__estimator__max_depth": [6, 12, 20],
    "est__estimator__min_samples_leaf": [2, 8],
}
_PHASE1_COARSE_HGB = {
    "est__estimator__max_iter": [100, 200, 300],
    "est__estimator__max_depth": [4, 8, 12],
    "est__estimator__learning_rate": [0.05, 0.15],
    "est__estimator__min_samples_leaf": [2, 8],
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


def _get_cv_object(validation_method: str, cv_splits: int, n_samples: int, random_state: int = 42):
    """Return a CV splitter for RandomizedSearchCV. validation_method: kfold | walk_forward."""
    if n_samples < 2:
        raise ValueError(f"Need at least 2 samples for cross-validation, got {n_samples}")
    # KFold requires n_splits <= n_samples and n_splits >= 2
    kfold_splits = max(2, min(cv_splits, n_samples))
    if validation_method == "walk_forward":
        # TimeSeriesSplit requires n_samples >= n_splits + 1; fallback to KFold for tiny datasets
        n_splits = min(cv_splits, max(2, n_samples // 3))
        if n_samples < n_splits + 1 and n_samples >= 2:
            return KFold(n_splits=max(2, min(cv_splits, n_samples - 1)), shuffle=True, random_state=random_state)
        if n_samples < n_splits + 1:
            return KFold(n_splits=kfold_splits, shuffle=True, random_state=random_state)
        return TimeSeriesSplit(n_splits=n_splits)
    return KFold(n_splits=kfold_splits, shuffle=True, random_state=random_state)


def _compute_metrics_regression(pipe: Pipeline, X: np.ndarray, y: np.ndarray, cv: Any) -> Dict[str, Any]:
    """Compute regression metrics from cross-validated predictions.

    Returns dict with mae, rmse, r2, r2_pct, median_ae, max_error, explained_variance.
    - mae: mean absolute error (interpretable units)
    - rmse: root mean squared error (penalizes large errors more)
    - median_ae: median absolute error (robust to outliers)
    - max_error: worst single prediction error
    - explained_variance: 0–1, fraction of variance explained; negative if worse than predicting mean
    """
    try:
        y_pred = cross_val_predict(pipe, X, y, cv=cv)
        mae = float(mean_absolute_error(y, y_pred))
        rmse = float(np.sqrt(mean_squared_error(y, y_pred)))
        r2 = float(r2_score(y, y_pred))
        # r2 can be negative; clamp for display
        r2_pct = max(0.0, min(100.0, r2 * 100))
        median_ae = float(median_absolute_error(y, y_pred))
        worst_err = float(max_error(y, y_pred))
        expl_var = float(explained_variance_score(y, y_pred))
        return {
            "mae": round(mae, 4),
            "rmse": round(rmse, 4),
            "r2": round(r2, 4),
            "r2_pct": round(r2_pct, 2),
            "median_ae": round(median_ae, 4),
            "max_error": round(worst_err, 4),
            "explained_variance": round(expl_var, 4),
        }
    except Exception as e:
        logger.warning("auto_tune.compute_metrics_regression_failed error=%s", e)
        return {}


def _compute_metrics_classification(pipe: Pipeline, X: np.ndarray, y: np.ndarray, cv: Any) -> Dict[str, Any]:
    """Compute classification metrics from cross-validated predictions. Returns dict with accuracy, accuracy_pct, etc."""
    try:
        y_pred = cross_val_predict(pipe, X, y, cv=cv)
        accuracy = float(accuracy_score(y, y_pred))
        precision = float(precision_score(y, y_pred, zero_division=0))
        recall = float(recall_score(y, y_pred, zero_division=0))
        f1 = float(f1_score(y, y_pred, zero_division=0))
        metrics: Dict[str, Any] = {
            "accuracy": round(accuracy, 4),
            "accuracy_pct": round(accuracy * 100, 2),
            "precision": round(precision, 4),
            "precision_pct": round(precision * 100, 2),
            "recall": round(recall, 4),
            "recall_pct": round(recall * 100, 2),
            "f1": round(f1, 4),
            "f1_pct": round(f1 * 100, 2),
        }
        if hasattr(pipe, "predict_proba"):
            try:
                y_proba = cross_val_predict(pipe, X, y, cv=cv, method="predict_proba")
                if y_proba.ndim == 2 and y_proba.shape[1] >= 2:
                    auc = float(roc_auc_score(y, y_proba[:, 1]))
                    metrics["roc_auc"] = round(auc, 4)
                    metrics["roc_auc_pct"] = round(auc * 100, 2)
            except Exception:
                pass
        return metrics
    except Exception as e:
        logger.warning("auto_tune.compute_metrics_classification_failed error=%s", e)
        return {}


# Simpler algorithms for complexity check: prefer these if within 1% of best
_MLQA_SIMPLER_ALGS = frozenset({"rf", "gb", "et", "hgb", "quantile"})
_MLQA_COMPLEX_ALGS = frozenset({"mlp", "stacked"})


def _mlqa_feature_names(model_kind: str) -> Optional[List[str]]:
    """Return feature names for MLQA sensitivity analysis when available."""
    if model_kind == "batting":
        return BATTING_FEATURE_COLS
    if model_kind == "bowling":
        return BOWLING_FEATURE_COLS
    return None


def _compute_mlqa_audit(
    report: Dict[str, Any],
    pipe: Pipeline,
    X: np.ndarray,
    y: np.ndarray,
    cv: Any,
    scoring: str,
    task_type: str,
    feature_names: Optional[List[str]] = None,
) -> Dict[str, Any]:
    """Run MLQA audit: overfitting, stability, bias, sensitivity, complexity.

    Returns dict with: audit_status (PASS/FAIL/WARNING), key_findings, bias_report,
    final_verdict, and structured checks for frontend display.
    """
    findings: List[str] = []
    status_flags: List[str] = []
    bias_report = "No protected groups defined; fairness audit skipped."

    val_score = report.get("best_cv_score")
    if val_score is None:
        return {
            "audit_status": "WARNING",
            "key_findings": ["Missing validation score; audit incomplete."],
            "bias_report": bias_report,
            "final_verdict": "Insufficient data for deployment recommendation.",
        }

    try:
        from sklearn.base import clone
        from sklearn.metrics import get_scorer

        # 1. Overfitting: train vs val delta
        pipe_fit = clone(pipe)
        pipe_fit.fit(X, y)
        scorer = get_scorer(scoring)
        train_score_val = scorer(pipe_fit, X, y)
        delta = abs(float(train_score_val) - float(val_score))
        overfitting_risk = delta > 0.08
        if overfitting_risk:
            findings.append(f"High Overfitting Risk: Train–Validation Δ = {delta:.4f} (> 8%).")
            status_flags.append("overfitting")
        else:
            findings.append(f"Overfitting check OK: Δ = {delta:.4f} ≤ 8%.")

        # 2. Stability: CV fold std
        fold_scores = cross_val_score(pipe, X, y, cv=cv, scoring=scoring)
        fold_std = float(np.std(fold_scores))
        unstable = fold_std > 0.05
        if unstable:
            findings.append(f"Unstable: CV fold σ = {fold_std:.4f} (> 0.05).")
            status_flags.append("unstable")
        else:
            findings.append(f"Stability OK: CV fold σ = {fold_std:.4f}.")

        # 3. Bias & Fairness
        fairness = report.get("fairness_metrics") or {}
        dip = fairness.get("disparate_impact_ratio")
        if dip is not None:
            biased = dip < 0.8 or dip > 1.25
            if biased:
                findings.append(f"Biased Model: disparate_impact_ratio = {dip:.4f} outside [0.8, 1.25].")
                status_flags.append("biased")
                bias_report = "Model shows disparate impact; review protected group treatment before deployment."
            else:
                findings.append(f"Fairness OK: disparate_impact_ratio = {dip:.4f} in [0.8, 1.25].")
                bias_report = "Model treats subgroups equitably within defined fairness bounds."

        # 4. Sensitivity: top 3 features
        est = pipe.named_steps.get("est")
        imps = None
        if est is not None:
            if hasattr(est, "estimators_") and len(est.estimators_) > 0:
                imp_list = [e.feature_importances_ for e in est.estimators_ if hasattr(e, "feature_importances_")]
                if imp_list:
                    imps = np.mean(imp_list, axis=0)
            elif hasattr(est, "feature_importances_"):
                imps = est.feature_importances_
        if imps is not None:
            if imps.ndim > 1:
                imps = np.mean(imps, axis=0)
            total = float(np.sum(imps))
            if total > 0:
                sorted_idx = np.argsort(-imps)[:3]
                top_weight = float(imps[sorted_idx[0]] / total)
                names = feature_names if feature_names and len(feature_names) == len(imps) else None
                top_name = names[sorted_idx[0]] if names else f"feature_{sorted_idx[0]}"
                if top_weight > 0.70:
                    findings.append(
                        f"Potential Data Leakage / Low Robustness: top feature '{top_name}' = {top_weight * 100:.1f}%."
                    )
                    status_flags.append("sensitivity")
                else:
                    findings.append(f"Sensitivity OK: top feature weight = {top_weight * 100:.1f}% ≤ 70%.")
        else:
            findings.append("Sensitivity: feature importance not available (linear/non-tree model).")

        # 5. Complexity
        candidates = report.get("candidates") or []
        best_algo = None
        best_score = val_score
        if candidates and isinstance(candidates[0], dict):
            best_algo = (candidates[0].get("algorithm") or "").lower()
        simpler_recommendation = None
        if best_algo and best_algo in _MLQA_COMPLEX_ALGS:
            for c in candidates[1:]:
                if not isinstance(c, dict):
                    continue
                s = c.get("best_score")
                if s is None:
                    continue
                alg = (c.get("algorithm") or "").lower()
                if alg in _MLQA_SIMPLER_ALGS:
                    gap = abs(float(s) - float(best_score))
                    if gap / max(abs(float(best_score)), 1e-9) <= 0.01:
                        simpler_recommendation = alg
                        break
        if simpler_recommendation:
            findings.append(
                f"Complexity: simpler model ({simpler_recommendation}) within 1% of best; recommend for production."
            )
            status_flags.append("complexity")

        # Aggregate status and verdict
        if status_flags:
            audit_status = (
                "FAIL" if any(f in ("overfitting", "unstable", "biased") for f in status_flags) else "WARNING"
            )
        else:
            audit_status = "PASS"
        if audit_status == "FAIL":
            final_verdict = "Rollback & Re-tune"
        else:
            final_verdict = "Proceed to Deployment"

        return {
            "audit_status": audit_status,
            "key_findings": findings,
            "bias_report": bias_report,
            "final_verdict": final_verdict,
            "checks": {
                "overfitting": {"delta": round(delta, 4), "flagged": overfitting_risk},
                "stability": {"cv_std": round(fold_std, 4), "flagged": unstable},
            },
        }
    except Exception as e:
        logger.warning("auto_tune.mlqa_audit_failed error=%s", e)
        return {
            "audit_status": "WARNING",
            "key_findings": [f"Audit failed: {e!s}"],
            "bias_report": bias_report,
            "final_verdict": "Insufficient data for deployment recommendation.",
        }


def _get_prior_tuned_algorithm(
    model_kind: str,
    format_suffix: Optional[str],
    out_dir: str,
) -> Optional[Tuple[str, Optional[Dict[str, Any]]]]:
    """Fetch prior tuned algorithm and params from go-app or tuning report file.

    When re-tuning a model, we stick to the same algorithm and only fine-tune hyperparams.
    Returns (algorithm_key, prior_params) or None if no prior tuning exists.
    """
    go_app_url = os.environ.get("GO_APP_URL", "").strip()
    if go_app_url:
        format_key = format_suffix if format_suffix else ""
        params = get_tuned_params_from_go_app(go_app_url, model_kind, format_key, os.environ.get("GO_APP_API_KEY"))
        if params:
            alg_list = params.get("algorithms")
            if isinstance(alg_list, list) and len(alg_list) > 0:
                algo = str(alg_list[0]).lower().strip()
                if algo in ("rf", "gb", "et", "hgb", "mlp", "quantile", "stacked"):
                    return (algo, params)
            estimator = (params.get("estimator") or "").strip().lower()
            if estimator in ("rf", "gb", "et", "hgb", "mlp", "quantile", "stacked", "gbm"):
                algo = "gb" if estimator in ("gb", "gbm") else estimator
                return (algo, params)

    if out_dir and os.path.isdir(out_dir):
        suffix = f"_{format_suffix}" if format_suffix else ""
        report_name = f"tuning_report_{model_kind}{suffix}.json"
        report_path = os.path.join(out_dir, report_name)
        if os.path.isfile(report_path):
            try:
                with open(report_path, "r", encoding="utf-8") as f:
                    report = json.load(f)
                alg_list = report.get("algorithms")
                if isinstance(alg_list, list) and len(alg_list) > 0:
                    algo = str(alg_list[0]).lower().strip()
                    if algo in ("rf", "gb", "et", "hgb", "mlp", "quantile", "stacked"):
                        prior = report.get("config_snippet") or report.get("best_params") or {}
                        return (algo, prior)
            except Exception as e:
                logger.debug("auto_tune.read_prior_report_failed path=%s error=%s", report_path, e)
    return None


def _prior_params_to_optuna_regression(algo: str, params: Dict[str, Any]) -> Dict[str, Any]:
    """Convert stored config_snippet to Optuna trial params for regression."""
    out: Dict[str, Any] = {"algorithm": algo}
    p = {}
    for k, v in params.items():
        key = k
        for prefix in ("est__estimator__", "est__"):
            if key.startswith(prefix):
                key = key[len(prefix) :]
                break
        p[key] = v
    for key in (
        "n_estimators",
        "max_depth",
        "min_samples_leaf",
        "learning_rate",
        "max_iter",
        "hidden_layer_sizes",
        "alpha",
        "learning_rate_init",
    ):
        if key in p and p[key] is not None:
            out[key] = p[key]
    return out


def _to_pipeline_params(config_space: Dict[str, Any], random_state: int) -> Dict[str, Any]:
    """Convert config search_space dict to Pipeline param format (est__estimator__*)."""
    out = {"est__estimator__random_state": [random_state]}
    for k, v in config_space.items():
        if k == "random_state":
            continue
        key = f"est__estimator__{k}"
        if v is not None and hasattr(v, "__iter__") and not isinstance(v, (str, bytes)):
            out[key] = list(v)  # JSON null → None for max_depth etc.
        else:
            out[key] = [v]
    return out


def _search_space_regression(model_kind: str) -> List[Tuple[str, Any, Dict[str, Any]]]:
    """Return list of (estimator_name, base_estimator, param_distributions) for regression.
    Reads from ml.tuning.search_space in config when present; otherwise uses built-in default from config.
    """
    tuning = get_tuning_config()
    rs = tuning.get("random_state", 42)

    rf_space = get_tuning_search_space("rf")
    if rf_space:
        rf_params = _to_pipeline_params(rf_space, rs)
    else:
        rf_params = {
            "est__estimator__random_state": [rs],
            "est__estimator__n_estimators": [50, 100, 150, 200, 300],
            "est__estimator__max_depth": [6, 8, 10, 12, 16, 20, None],
            "est__estimator__min_samples_split": [2, 5, 10],
            "est__estimator__min_samples_leaf": [1, 2, 4],
        }

    gb_space = get_tuning_search_space("gb")
    if gb_space:
        gb_params = _to_pipeline_params(gb_space, rs)
    else:
        gb_params = {
            "est__estimator__random_state": [rs],
            "est__estimator__n_estimators": [50, 100, 150, 200],
            "est__estimator__max_depth": [3, 4, 5, 6, 8],
            "est__estimator__learning_rate": [0.01, 0.05, 0.1],
            "est__estimator__min_samples_split": [2, 5],
            "est__estimator__min_samples_leaf": [1, 2],
        }

    candidates: List[Tuple[str, str, Any, Dict[str, Any]]] = [
        ("rf", "RandomForestRegressor", RandomForestRegressor(), rf_params),
        ("gb", "GradientBoostingRegressor", GradientBoostingRegressor(), gb_params),
    ]
    # Add quantile (GBM with loss=quantile) for median/interval prediction
    try:
        tp = get_training_params(model_kind)
        quantile_level = tp.get("quantile_level", 0.5)
        qr = GradientBoostingRegressor(
            n_estimators=tp.get("n_estimators", 200),
            max_depth=tp.get("max_depth", 12),
            random_state=rs,
            learning_rate=tp.get("learning_rate", 0.1),
            loss="quantile",
            alpha=quantile_level,
        )
        candidates.append(
            ("quantile", "QuantileRegressor", qr, {"est__estimator__max_depth": [tp.get("max_depth", 12)]})
        )
    except (ValueError, KeyError):
        pass

    # Add stacked (RF + GBM + Ridge) using training params; no param search for stacked
    try:
        tp = get_training_params(model_kind)
        rf = RandomForestRegressor(
            n_estimators=tp.get("n_estimators", 200),
            max_depth=tp.get("max_depth", 12),
            random_state=rs,
        )
        gb = GradientBoostingRegressor(
            n_estimators=tp.get("n_estimators", 200),
            max_depth=tp.get("max_depth", 12),
            random_state=rs,
            learning_rate=tp.get("learning_rate", 0.1),
        )
        stacked = StackingRegressor(
            estimators=[("rf", rf), ("gb", gb)],
            final_estimator=Ridge(alpha=1.0, random_state=rs),
        )
        candidates.append(
            ("stacked", "StackingRegressor", stacked, {"est__estimator__final_estimator__random_state": [rs]})
        )
    except (ValueError, KeyError):
        pass
    return candidates


def _build_pipeline(estimator: Any) -> Pipeline:
    return Pipeline(
        [
            ("scaler", StandardScaler()),
            ("est", MultiOutputRegressor(estimator)),
        ]
    )


def _build_pipeline_single_regression(estimator: Any) -> Pipeline:
    """Pipeline for single-output regression (extras)."""
    return Pipeline([("scaler", StandardScaler()), ("est", estimator)])


def _to_pipeline_params_single(
    config_space: Dict[str, Any], random_state: int, prefix: str = "est__"
) -> Dict[str, Any]:
    """Param dict for single-estimator pipeline (extras): est__n_estimators, etc."""
    out = {f"{prefix}random_state": [random_state]}
    for k, v in config_space.items():
        if k == "random_state":
            continue
        key = f"{prefix}{k}"
        if v is not None and hasattr(v, "__iter__") and not isinstance(v, (str, bytes)):
            out[key] = list(v)
        else:
            out[key] = [v]
    return out


def _search_space_regression_single(model_kind: str) -> List[Tuple[str, str, Any, Dict[str, Any]]]:
    """Search space for single-output regression (extras)."""
    tuning = get_tuning_config()
    rs = tuning.get("random_state", 42)
    rf_space = get_tuning_search_space("rf")
    rf_params = (
        _to_pipeline_params_single(rf_space, rs)
        if rf_space
        else _to_pipeline_params_single({"n_estimators": [50, 100, 150, 200], "max_depth": [6, 8, 10, 12, None]}, rs)
    )
    gb_space = get_tuning_search_space("gb")
    gb_params = (
        _to_pipeline_params_single(gb_space, rs)
        if gb_space
        else _to_pipeline_params_single(
            {"n_estimators": [50, 100, 150], "max_depth": [3, 4, 5, 6], "learning_rate": [0.01, 0.05, 0.1]}, rs
        )
    )
    return [
        ("rf", "RandomForestRegressor", RandomForestRegressor(), rf_params),
        ("gb", "GradientBoostingRegressor", GradientBoostingRegressor(), gb_params),
    ]


def _phase1_candidates_regression(model_kind: str, allow: frozenset) -> List[Tuple[str, str, Any, Dict[str, Any]]]:
    """Phase 1 coarse candidates for multi-output regression (batting/bowling/fielding)."""
    rs = get_tuning_config().get("random_state", 42)
    candidates: List[Tuple[str, str, Any, Dict[str, Any]]] = []
    if "rf" in allow:
        p = dict(_PHASE1_COARSE_RF)
        p["est__estimator__random_state"] = [rs]
        candidates.append(("rf", "RandomForestRegressor", RandomForestRegressor(), p))
    if "gb" in allow:
        p = dict(_PHASE1_COARSE_GB)
        p["est__estimator__random_state"] = [rs]
        candidates.append(("gb", "GradientBoostingRegressor", GradientBoostingRegressor(), p))
    if "et" in allow:
        p = dict(_PHASE1_COARSE_ET)
        p["est__estimator__random_state"] = [rs]
        candidates.append(("et", "ExtraTreesRegressor", ExtraTreesRegressor(), p))
    if "hgb" in allow:
        p = dict(_PHASE1_COARSE_HGB)
        p["est__estimator__random_state"] = [rs]
        candidates.append(("hgb", "HistGradientBoostingRegressor", HistGradientBoostingRegressor(), p))
    if "quantile" in allow:
        try:
            tp = get_training_params(model_kind)
            qr = GradientBoostingRegressor(
                n_estimators=tp.get("n_estimators", 200),
                max_depth=tp.get("max_depth", 12),
                random_state=rs,
                learning_rate=tp.get("learning_rate", 0.1),
                loss="quantile",
                alpha=tp.get("quantile_level", 0.5),
            )
            candidates.append(
                ("quantile", "QuantileRegressor", qr, {"est__estimator__max_depth": [6, 10, 14, 20]})
            )
        except (ValueError, KeyError):
            pass
    if "stacked" in allow:
        try:
            tp = get_training_params(model_kind)
            stacked = StackingRegressor(
                estimators=[
                    ("rf", RandomForestRegressor(n_estimators=100, max_depth=12, random_state=rs)),
                    ("gb", GradientBoostingRegressor(n_estimators=100, max_depth=8, random_state=rs)),
                ],
                final_estimator=Ridge(alpha=1.0, random_state=rs),
            )
            candidates.append(
                ("stacked", "StackingRegressor", stacked, {"est__estimator__final_estimator__random_state": [rs]})
            )
        except (ValueError, KeyError):
            pass
    if "mlp" in allow:
        p = dict(_PHASE1_COARSE_MLP_REG)
        p["est__estimator__random_state"] = [rs]
        candidates.append(("mlp", "MLPRegressor", MLPRegressor(early_stopping=True, random_state=rs), p))
    return candidates


def _coarse_to_single_prefix(d: Dict[str, Any]) -> Dict[str, Any]:
    """Convert est__estimator__* to est__* for single-estimator pipelines."""
    return {k.replace("est__estimator__", "est__"): v for k, v in d.items()}


def _phase1_candidates_regression_single(allow: frozenset) -> List[Tuple[str, str, Any, Dict[str, Any]]]:
    """Phase 1 coarse candidates for single-output regression (extras)."""
    rs = get_tuning_config().get("random_state", 42)
    candidates: List[Tuple[str, str, Any, Dict[str, Any]]] = []
    for key, name, est_factory, coarse in [
        ("rf", "RandomForestRegressor", RandomForestRegressor, _PHASE1_COARSE_RF),
        ("gb", "GradientBoostingRegressor", GradientBoostingRegressor, _PHASE1_COARSE_GB),
        ("et", "ExtraTreesRegressor", ExtraTreesRegressor, _PHASE1_COARSE_ET),
        ("hgb", "HistGradientBoostingRegressor", HistGradientBoostingRegressor, _PHASE1_COARSE_HGB),
    ]:
        if key in allow:
            p = _coarse_to_single_prefix(dict(coarse))
            p["est__random_state"] = [rs]
            candidates.append((key, name, est_factory(), p))
    if "mlp" in allow:
        p = _coarse_to_single_prefix(dict(_PHASE1_COARSE_MLP_REG))
        p["est__random_state"] = [rs]
        candidates.append(("mlp", "MLPRegressor", MLPRegressor(early_stopping=True, random_state=rs), p))
    return candidates


def _phase1_candidates_classification(allow: frozenset) -> List[Tuple[str, str, Any, Dict[str, Any]]]:
    """Phase 1 coarse candidates for classification (win)."""
    rs = get_tuning_config().get("random_state", 42)
    candidates: List[Tuple[str, str, Any, Dict[str, Any]]] = []
    for key, name, est_factory, coarse in [
        ("rf", "RandomForestClassifier", RandomForestClassifier, _PHASE1_COARSE_RF),
        ("gb", "GradientBoostingClassifier", GradientBoostingClassifier, _PHASE1_COARSE_GB),
        ("et", "ExtraTreesClassifier", ExtraTreesClassifier, _PHASE1_COARSE_ET),
        ("hgb", "HistGradientBoostingClassifier", HistGradientBoostingClassifier, _PHASE1_COARSE_HGB),
    ]:
        if key in allow:
            p = _coarse_to_single_prefix(dict(coarse))
            p["est__random_state"] = [rs]
            candidates.append((key, name, est_factory(), p))
    if "mlp" in allow:
        p = dict(_PHASE1_COARSE_MLP_CLF)
        p["est__random_state"] = [rs]
        candidates.append(("mlp", "MLPClassifier", MLPClassifier(early_stopping=True, random_state=rs), p))
    return candidates


def _search_space_classification(model_kind: str) -> List[Tuple[str, str, Any, Dict[str, Any]]]:
    """Search space for binary classification (win)."""
    tuning = get_tuning_config()
    rs = tuning.get("random_state", 42)
    rf_space = get_tuning_search_space("rf")
    rf_params = (
        _to_pipeline_params_single(rf_space, rs)
        if rf_space
        else _to_pipeline_params_single({"n_estimators": [50, 100, 150, 200], "max_depth": [6, 8, 10, 12, None]}, rs)
    )
    gb_space = get_tuning_search_space("gb")
    gb_params = (
        _to_pipeline_params_single(gb_space, rs)
        if gb_space
        else _to_pipeline_params_single(
            {"n_estimators": [50, 100, 150], "max_depth": [3, 4, 5, 6], "learning_rate": [0.01, 0.05, 0.1]}, rs
        )
    )
    return [
        ("rf", "RandomForestClassifier", RandomForestClassifier(), rf_params),
        ("gb", "GradientBoostingClassifier", GradientBoostingClassifier(), gb_params),
    ]


def _run_search_two_phase_single_regression(
    X: np.ndarray,
    y: np.ndarray,
    model_kind: str,
    format_suffix: Optional[str],
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
    algorithms: Optional[List[str]] = None,
    validation_method: str = "walk_forward",
    n_jobs_override: Optional[int] = None,
    task_index: int = 0,
    task_total: int = 1,
    prior_params: Optional[Dict[str, Any]] = None,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Two-phase search for single-output regression (extras). Skips Phase 1 when single algorithm (prior)."""
    tuning_cfg = get_tuning_config()
    algs = algorithms or ["rf", "gb"]
    if algs == "all" or (isinstance(algs, list) and "all" in [str(a).lower() for a in algs]):
        algs = ["rf", "gb", "et", "hgb"]
        stages = tuning_cfg.get("stages") or {}
        neural = stages.get("neural") or {}
        if neural.get("mlp", True):
            algs = list(algs) + ["mlp"]
    allow = frozenset(str(a).lower().strip() for a in (algs if isinstance(algs, (list, tuple)) else [algs]))
    cv = _get_cv_object(validation_method, cv_splits, X.shape[0], random_state)
    n_jobs = _effective_n_jobs(tuning_cfg, n_jobs_override)
    candidates = _phase1_candidates_regression_single(allow)
    if not candidates:
        return _run_search_single_regression(
            X,
            y,
            model_kind,
            cv_splits,
            n_iter,
            scoring,
            random_state,
            algorithms,
            validation_method,
            n_jobs_override,
        )

    results: List[Tuple[str, str, float, Dict[str, Any], Pipeline]] = []
    if len(candidates) == 1 and _HAS_OPTUNA:
        key, name, base_est, param_dist = candidates[0]
        _progress.write_progress(
            phase="fine_tuning",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            algorithm=key,
            message=f"Fine-tune only ({name}), skipping screening",
        )
        pipe = _build_pipeline_single_regression(base_est)
        pipe.fit(X, y)
        initial_score = float(cross_val_score(pipe, X, y, cv=cv, scoring=scoring, n_jobs=n_jobs).mean())
        best_params = {}
        if param_dist:
            for k, vs in param_dist.items():
                if isinstance(vs, (list, tuple)) and len(vs) > 0:
                    best_params[k] = vs[0]
                elif vs is not None:
                    best_params[k] = vs
        results = [(key, name, initial_score, best_params, pipe)]
        best_key, best_name, best_score, best_params, best_pipe = key, name, initial_score, best_params, pipe
        winners = [key]
    else:
        _progress.write_progress(
            phase="screening",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            message="Screening algorithms (extras)",
            algorithms_screened=[c[0] for c in candidates],
        )
        for key, name, base_est, param_dist in candidates:
            _progress.write_progress(
                phase="screening",
                model_kind=model_kind,
                format_suffix=format_suffix,
                task_index=task_index,
                task_total=task_total,
                algorithm=key,
                message=f"Testing {name}",
            )
            pipe = _build_pipeline_single_regression(base_est)
            n_phase1 = min(PHASE1_TRIALS_PER_ALGORITHM, max(1, _count_combinations(param_dist) // 2))
            search = RandomizedSearchCV(
                pipe,
                param_distributions=param_dist,
                n_iter=n_phase1,
                cv=cv,
                scoring=scoring,
                random_state=random_state,
                n_jobs=n_jobs,
                error_score="raise",
            )
            search.fit(X, y)
            best_params = dict(search.best_params_)
            params_display = {k.replace("est__", ""): v for k, v in best_params.items()}
            _progress.write_progress(
                phase="screening",
                model_kind=model_kind,
                format_suffix=format_suffix,
                task_index=task_index,
                task_total=task_total,
                algorithm=key,
                hyperparams=params_display,
                best_score=float(search.best_score_),
            )
            results.append((key, name, float(search.best_score_), best_params, search.best_estimator_))
        results.sort(key=lambda r: r[2], reverse=True)
        best_key, best_name, best_score, best_params, best_pipe = results[0]
        winners = [r[0] for r in results[:2]]
    if not _HAS_OPTUNA:
        config_snippet = {k.replace("est__", ""): v for k, v in best_params.items()}
        report = {
            "model_kind": model_kind,
            "best_cv_score": best_score,
            "best_params": best_params,
            "config_snippet": config_snippet,
            "scoring": scoring,
            "cv_splits": cv_splits,
            "validation_method": validation_method,
            "algorithms": [r[0] for r in results],
            "algorithms_requested": [r[0] for r in results],
            "n_samples": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "candidates": [{"algorithm": r[0], "best_score": r[2]} for r in results],
        }
        if best_pipe:
            report["metrics"] = _compute_metrics_regression(best_pipe, X, y, cv)
            report["mlqa_audit"] = _compute_mlqa_audit(
                report, best_pipe, X, y, cv, scoring, "regression", _mlqa_feature_names(model_kind)
            )
        return best_pipe, best_params, report

    def _obj(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        if alg == "rf":
            est = RandomForestRegressor(
                n_estimators=trial.suggest_int("n_estimators", 50, 350, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        elif alg == "gb":
            est = GradientBoostingRegressor(
                n_estimators=trial.suggest_int("n_estimators", 50, 350, step=50),
                max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
                learning_rate=trial.suggest_float("learning_rate", 0.01, 0.2, log=True),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        elif alg == "et":
            est = ExtraTreesRegressor(
                n_estimators=trial.suggest_int("n_estimators", 50, 350, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        elif alg == "mlp":
            sizes = trial.suggest_categorical("hidden_layer_sizes", [(64, 64), (128, 64), (128, 128, 64)])
            alpha = trial.suggest_float("alpha", 1e-4, 1e-1, log=True)
            lr_init = trial.suggest_float("learning_rate_init", 1e-4, 1e-1, log=True)
            est = MLPRegressor(
                hidden_layer_sizes=sizes,
                alpha=alpha,
                learning_rate_init=lr_init,
                max_iter=1000,
                early_stopping=True,
                random_state=random_state,
            )
        else:
            est = HistGradientBoostingRegressor(
                max_iter=trial.suggest_int("max_iter", 50, 400, step=50),
                max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
                learning_rate=trial.suggest_float("learning_rate", 0.01, 0.2, log=True),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        pipe = _build_pipeline_single_regression(est)
        scores = cross_val_score(pipe, X, y, cv=cv, scoring=scoring, n_jobs=n_jobs)
        return float(scores.mean())

    n_phase2 = min(n_iter, PHASE2_TRIALS)

    def _cb(study: Any, trial: Any) -> None:
        _progress.write_progress(
            phase="fine_tuning",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            algorithm=str(trial.params.get("algorithm", best_key)),
            hyperparams=trial.params,
            trial=trial.number + 1,
            trials_total=n_phase2,
            best_score=float(study.best_value) if study.best_trial else best_score,
            best_algorithm=best_key,
            message=f"Fine-tuning {best_name}",
        )

    study = optuna.create_study(
        direction="maximize", sampler=optuna.samplers.TPESampler(seed=random_state, n_startup_trials=5)
    )
    if prior_params:
        try:
            study.enqueue_trial(prior_params)
        except Exception as e:
            logger.debug("auto_tune.enqueue_prior_trial_skipped error=%s", e)
    study.optimize(_obj, n_trials=n_phase2, n_jobs=1, show_progress_bar=False, callbacks=[_cb])
    if study.best_trial:
        p = study.best_params
        alg = p.get("algorithm", best_key)
        if alg == "rf":
            est = RandomForestRegressor(
                n_estimators=p["n_estimators"],
                max_depth=p["max_depth"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "gb":
            est = GradientBoostingRegressor(
                n_estimators=p["n_estimators"],
                max_depth=p["max_depth"],
                learning_rate=p["learning_rate"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "et":
            est = ExtraTreesRegressor(
                n_estimators=p["n_estimators"],
                max_depth=p["max_depth"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "mlp":
            est = MLPRegressor(
                hidden_layer_sizes=p.get("hidden_layer_sizes", (128, 64)),
                alpha=p.get("alpha", 0.001),
                learning_rate_init=p.get("learning_rate_init", 0.001),
                max_iter=1000,
                early_stopping=True,
                random_state=random_state,
            )
        else:
            est = HistGradientBoostingRegressor(
                max_iter=p["max_iter"],
                max_depth=p["max_depth"],
                learning_rate=p["learning_rate"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        best_pipe = _build_pipeline_single_regression(est)
        best_pipe.fit(X, y)
        best_score = float(study.best_value)
        best_params = {"est__" + k: v for k, v in p.items()}
    config_snippet = {k.replace("est__", ""): v for k, v in best_params.items()}
    report = {
        "model_kind": model_kind,
        "best_cv_score": best_score,
        "best_params": best_params,
        "config_snippet": config_snippet,
        "scoring": scoring,
        "cv_splits": cv_splits,
        "validation_method": validation_method,
        "algorithms": winners,
        "algorithms_requested": [r[0] for r in results],
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "candidates": [{"algorithm": r[0], "best_score": r[2]} for r in results],
    }
    if best_pipe:
        report["metrics"] = _compute_metrics_regression(best_pipe, X, y, cv)
        report["mlqa_audit"] = _compute_mlqa_audit(
            report, best_pipe, X, y, cv, scoring, "regression", _mlqa_feature_names(model_kind)
        )
    return best_pipe, best_params, report


def _run_search_single_regression(
    X: np.ndarray,
    y: np.ndarray,
    model_kind: str,
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
    algorithms: Optional[List[str]] = None,
    validation_method: str = "walk_forward",
    n_jobs_override: Optional[int] = None,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Run RandomizedSearchCV for single-output regression. Returns (best_pipeline, best_params, report)."""
    tuning_cfg = get_tuning_config()
    algorithms = algorithms or tuning_cfg.get("algorithms", ["rf", "gb"])
    validation_method = validation_method or tuning_cfg.get("validation_method", "walk_forward")
    allow = frozenset(a.lower() for a in algorithms)
    candidates = [(k, n, e, p) for k, n, e, p in _search_space_regression_single(model_kind) if k in allow]
    if not candidates:
        raise ValueError(f"No algorithms selected for extras; available: rf, gb. You requested: {list(algorithms)}")
    cv = _get_cv_object(validation_method, cv_splits, X.shape[0], random_state)
    best_score = None
    best_pipe = None
    best_params = None
    all_cv_results: List[Dict[str, Any]] = []
    n_jobs = _effective_n_jobs(tuning_cfg, n_jobs_override)
    algorithms_used: List[str] = []
    for key, name, base_est, param_dist in candidates:
        pipe = _build_pipeline_single_regression(base_est)
        search = RandomizedSearchCV(
            pipe,
            param_distributions=param_dist,
            n_iter=min(n_iter, max(1, _count_combinations(param_dist) // 2)),
            cv=cv,
            scoring=scoring,
            random_state=random_state,
            n_jobs=n_jobs,
            error_score="raise",
        )
        search.fit(X, y)
        algorithms_used.append(key)
        all_cv_results.append(
            {
                "algorithm": key,
                "estimator": name,
                "best_score": float(search.best_score_),
                "best_params": search.best_params_,
            }
        )
        if best_score is None or search.best_score_ > best_score:
            best_score = search.best_score_
            best_pipe = search.best_estimator_
            best_params = dict(search.best_params_)
    config_snippet = {}
    if best_params:
        for k, v in best_params.items():
            if k.startswith("est__"):
                config_snippet[k.replace("est__", "")] = v
    report = {
        "model_kind": model_kind,
        "best_cv_score": best_score,
        "best_params": best_params,
        "config_snippet": config_snippet,
        "scoring": scoring,
        "cv_splits": cv_splits,
        "validation_method": validation_method,
        "algorithms": algorithms_used,
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "candidates": all_cv_results,
    }
    if best_pipe is not None:
        report["metrics"] = _compute_metrics_regression(best_pipe, X, y, cv)
        report["mlqa_audit"] = _compute_mlqa_audit(
            report, best_pipe, X, y, cv, scoring, "regression", _mlqa_feature_names(model_kind)
        )
    return best_pipe, best_params, report


def _run_search_classification(
    X: np.ndarray,
    y: np.ndarray,
    model_kind: str,
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
    algorithms: Optional[List[str]] = None,
    validation_method: str = "walk_forward",
    n_jobs_override: Optional[int] = None,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Run RandomizedSearchCV for binary classification (win). Returns (best_pipeline, best_params, report)."""
    tuning_cfg = get_tuning_config()
    algorithms = algorithms or tuning_cfg.get("algorithms", ["rf", "gb"])
    validation_method = validation_method or tuning_cfg.get("validation_method", "walk_forward")
    allow = frozenset(a.lower() for a in algorithms)
    candidates = [(k, n, e, p) for k, n, e, p in _search_space_classification(model_kind) if k in allow]
    if not candidates:
        raise ValueError(f"No algorithms selected for win; available: rf, gb. You requested: {list(algorithms)}")
    cv = _get_cv_object(validation_method, cv_splits, X.shape[0], random_state)
    best_score = None
    best_pipe = None
    best_params = None
    all_cv_results: List[Dict[str, Any]] = []
    n_jobs = _effective_n_jobs(tuning_cfg, n_jobs_override)
    algorithms_used: List[str] = []
    for key, name, base_est, param_dist in candidates:
        pipe = Pipeline([("scaler", StandardScaler()), ("est", base_est)])
        search = RandomizedSearchCV(
            pipe,
            param_distributions=param_dist,
            n_iter=min(n_iter, max(1, _count_combinations(param_dist) // 2)),
            cv=cv,
            scoring=scoring,
            random_state=random_state,
            n_jobs=n_jobs,
            error_score="raise",
        )
        search.fit(X, y)
        algorithms_used.append(key)
        all_cv_results.append(
            {
                "algorithm": key,
                "estimator": name,
                "best_score": float(search.best_score_),
                "best_params": search.best_params_,
            }
        )
        if best_score is None or search.best_score_ > best_score:
            best_score = search.best_score_
            best_pipe = search.best_estimator_
            best_params = dict(search.best_params_)
    config_snippet = {}
    if best_params:
        for k, v in best_params.items():
            if k.startswith("est__"):
                config_snippet[k.replace("est__", "")] = v
    report = {
        "model_kind": model_kind,
        "best_cv_score": best_score,
        "best_params": best_params,
        "config_snippet": config_snippet,
        "scoring": scoring,
        "cv_splits": cv_splits,
        "validation_method": validation_method,
        "algorithms": algorithms_used,
        "algorithms_requested": algorithms_used,
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "candidates": all_cv_results,
    }
    if best_pipe is not None:
        report["metrics"] = _compute_metrics_classification(best_pipe, X, y, cv)
        report["mlqa_audit"] = _compute_mlqa_audit(
            report, best_pipe, X, y, cv, scoring, "classification", _mlqa_feature_names(model_kind)
        )
    return best_pipe, best_params, report


def _save_artifacts_model_only(
    pipeline: Pipeline,
    out_dir: str,
    model_kind: str,
    format_suffix: Optional[str],
    joblib_compress: int,
    report: Dict[str, Any],
) -> None:
    """Save model only (no scaler) for extras/win; plus tuning report."""
    os.makedirs(out_dir, exist_ok=True)
    model = pipeline.named_steps["est"]
    if format_suffix:
        model_path = os.path.join(out_dir, f"{model_kind}_model_{format_suffix}.joblib")
        report_path = os.path.join(out_dir, f"tuning_report_{model_kind}_{format_suffix}.json")
    else:
        model_path = os.path.join(out_dir, f"{model_kind}_model.joblib")
        report_path = os.path.join(out_dir, f"tuning_report_{model_kind}.json")
    joblib.dump(model, model_path, compress=joblib_compress)
    with open(report_path, "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2)


def _run_search_two_phase(
    X: np.ndarray,
    Y: np.ndarray,
    model_kind: str,
    format_suffix: Optional[str],
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
    algorithms: Optional[List[str]] = None,
    validation_method: str = "walk_forward",
    n_jobs_override: Optional[int] = None,
    task_index: int = 0,
    task_total: int = 1,
    prior_params: Optional[Dict[str, Any]] = None,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Two-phase search: coarse algorithm screening, then Optuna fine-tuning on winner(s).
    When algorithms has a single element (from prior tuning), Phase 1 is skipped and we go
    straight to Optuna fine-tuning. prior_params can seed the first Optuna trial (warm start).
    """
    tuning_cfg = get_tuning_config()
    algs = algorithms or tuning_cfg.get("algorithms", ["rf", "gb", "quantile"])
    if algs == "all" or (isinstance(algs, list) and "all" in [str(a).lower() for a in algs]):
        algs = ["rf", "gb", "et", "hgb", "quantile", "stacked"]
        stages = tuning_cfg.get("stages") or {}
        neural = stages.get("neural") or {}
        if neural.get("mlp", True):
            algs = list(algs) + ["mlp"]
    allow = frozenset(str(a).lower().strip() for a in (algs if isinstance(algs, (list, tuple)) else [algs]))
    cv = _get_cv_object(validation_method, cv_splits, X.shape[0], random_state)
    n_jobs = _effective_n_jobs(tuning_cfg, n_jobs_override)
    candidates = _phase1_candidates_regression(model_kind, allow)
    if not candidates:
        return _run_search(
            X,
            Y,
            model_kind,
            cv_splits,
            n_iter,
            scoring,
            random_state,
            algorithms,
            validation_method,
            n_jobs_override,
        )

    # Skip Phase 1 when single algorithm (prior fine-tune): go straight to Optuna
    results: List[Tuple[str, str, float, Dict[str, Any], Pipeline]] = []
    if len(candidates) == 1 and _HAS_OPTUNA:
        key, name, base_est, param_dist = candidates[0]
        _progress.write_progress(
            phase="fine_tuning",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            algorithm=key,
            message=f"Fine-tune only ({name}), skipping algorithm screening",
            algorithms_screened=[key],
        )
        pipe = _build_pipeline(base_est)
        pipe.fit(X, Y)
        initial_score = float(cross_val_score(pipe, X, Y, cv=cv, scoring=scoring, n_jobs=n_jobs).mean())
        best_params = {}
        if param_dist:
            for k, vs in param_dist.items():
                if isinstance(vs, (list, tuple)) and len(vs) > 0:
                    best_params[k] = vs[0]
                elif vs is not None:
                    best_params[k] = vs
        results = [(key, name, initial_score, best_params, pipe)]
        best_key, best_name, best_score, best_params, best_pipe = key, name, initial_score, best_params, pipe
        winners = [key]
    else:
        # Phase 1: coarse screening
        _progress.write_progress(
            phase="screening",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            message="Screening algorithms with coarse hyperparameters",
            algorithms_screened=[c[0] for c in candidates],
        )
        for i, (key, name, base_est, param_dist) in enumerate(candidates):
            _progress.write_progress(
                phase="screening",
                model_kind=model_kind,
                format_suffix=format_suffix,
                task_index=task_index,
                task_total=task_total,
                algorithm=key,
                message=f"Testing {name}",
                algorithms_screened=[c[0] for c in candidates],
            )
            pipe = _build_pipeline(base_est)
            n_phase1 = min(PHASE1_TRIALS_PER_ALGORITHM, max(1, _count_combinations(param_dist) // 2))
            search = RandomizedSearchCV(
                pipe,
                param_distributions=param_dist,
                n_iter=n_phase1,
                cv=cv,
                scoring=scoring,
                random_state=random_state,
                n_jobs=n_jobs,
                error_score="raise",
            )
            search.fit(X, Y)
            best_params = dict(search.best_params_)
            params_display = {k.replace("est__estimator__", ""): v for k, v in best_params.items()}
            _progress.write_progress(
                phase="screening",
                model_kind=model_kind,
                format_suffix=format_suffix,
                task_index=task_index,
                task_total=task_total,
                algorithm=key,
                hyperparams=params_display,
                best_score=float(search.best_score_),
                message=f"{name} best score: {search.best_score_:.4f}",
            )
            results.append((key, name, float(search.best_score_), best_params, search.best_estimator_))
        results.sort(key=lambda r: r[2], reverse=True)
        best_key, best_name, best_score, best_params, best_pipe = results[0]
        winners = [r[0] for r in results[:2]]

    if not _HAS_OPTUNA or len(winners) == 0:
        config_snippet = {k.replace("est__estimator__", ""): v for k, v in best_params.items()}
        report = {
            "model_kind": model_kind,
            "best_cv_score": best_score,
            "best_params": best_params,
            "config_snippet": config_snippet,
            "scoring": scoring,
            "cv_splits": cv_splits,
            "validation_method": validation_method,
            "algorithms": [r[0] for r in results],
            "algorithms_requested": [c[0] for c in candidates],
            "n_samples": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "n_targets": int(Y.shape[1]),
            "candidates": [{"algorithm": r[0], "estimator": r[1], "best_score": r[2]} for r in results],
            "phase": "screening_only",
        }
        if best_pipe is not None:
            report["metrics"] = _compute_metrics_regression(best_pipe, X, Y, cv)
            report["mlqa_audit"] = _compute_mlqa_audit(
                report, best_pipe, X, Y, cv, scoring, "regression", _mlqa_feature_names(model_kind)
            )
        return best_pipe, best_params, report

    # Phase 2: Optuna fine-tuning on winner(s)
    n_phase2 = min(n_iter, PHASE2_TRIALS)

    def _progress_callback(study: Any, trial: Any) -> None:
        t = trial.number + 1
        params = trial.params
        _progress.write_progress(
            phase="fine_tuning",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            algorithm=str(params.get("algorithm", best_key)),
            hyperparams=params,
            trial=t,
            trials_total=n_phase2,
            best_score=float(study.best_value) if study.best_trial else best_score,
            best_algorithm=best_key,
            message=f"Fine-tuning {best_name} (trial {t}/{n_phase2})",
        )

    def _optuna_objective(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        if alg == "rf":
            n_est = trial.suggest_int("n_estimators", 50, 350, step=50)
            depth = trial.suggest_int("max_depth", 4, 24, step=2)
            leaf = trial.suggest_int("min_samples_leaf", 1, 8)
            est = RandomForestRegressor(
                n_estimators=n_est, max_depth=depth, min_samples_leaf=leaf, random_state=random_state
            )
        elif alg == "gb":
            n_est = trial.suggest_int("n_estimators", 50, 350, step=50)
            depth = trial.suggest_int("max_depth", 3, 20, step=1)
            lr = trial.suggest_float("learning_rate", 0.01, 0.2, log=True)
            leaf = trial.suggest_int("min_samples_leaf", 1, 8)
            est = GradientBoostingRegressor(
                n_estimators=n_est, max_depth=depth, learning_rate=lr, min_samples_leaf=leaf, random_state=random_state
            )
        elif alg == "quantile":
            try:
                tp = get_training_params(model_kind)
                n_est = trial.suggest_int("n_estimators", 50, 350, step=50)
                depth = trial.suggest_int("max_depth", 4, 20, step=2)
                lr = trial.suggest_float("learning_rate", 0.01, 0.2, log=True)
                leaf = trial.suggest_int("min_samples_leaf", 1, 8)
                est = GradientBoostingRegressor(
                    n_estimators=n_est,
                    max_depth=depth,
                    learning_rate=lr,
                    min_samples_leaf=leaf,
                    random_state=random_state,
                    loss="quantile",
                    alpha=tp.get("quantile_level", 0.5),
                )
            except (ValueError, KeyError):
                est = GradientBoostingRegressor(
                    n_estimators=200, max_depth=12, random_state=random_state, loss="quantile", alpha=0.5
                )
        elif alg == "et":
            n_est = trial.suggest_int("n_estimators", 50, 350, step=50)
            depth = trial.suggest_int("max_depth", 4, 24, step=2)
            leaf = trial.suggest_int("min_samples_leaf", 1, 8)
            est = ExtraTreesRegressor(
                n_estimators=n_est, max_depth=depth, min_samples_leaf=leaf, random_state=random_state
            )
        elif alg == "hgb":
            n_est = trial.suggest_int("max_iter", 50, 400, step=50)
            depth = trial.suggest_int("max_depth", 3, 14, step=1)
            lr = trial.suggest_float("learning_rate", 0.01, 0.2, log=True)
            leaf = trial.suggest_int("min_samples_leaf", 1, 8)
            est = HistGradientBoostingRegressor(
                max_iter=n_est, max_depth=depth, learning_rate=lr, min_samples_leaf=leaf, random_state=random_state
            )
        elif alg == "mlp":
            sizes = trial.suggest_categorical(
                "hidden_layer_sizes", [(64, 64), (128, 64), (128, 128, 64), (256, 128, 64)]
            )
            alpha = trial.suggest_float("alpha", 1e-4, 1e-1, log=True)
            lr_init = trial.suggest_float("learning_rate_init", 1e-4, 1e-1, log=True)
            est = MLPRegressor(
                hidden_layer_sizes=sizes,
                alpha=alpha,
                learning_rate_init=lr_init,
                max_iter=1000,
                early_stopping=True,
                random_state=random_state,
            )
        else:
            n_est = trial.suggest_int("n_estimators", 100, 300, step=50)
            depth = trial.suggest_int("max_depth", 8, 16, step=2)
            est = RandomForestRegressor(n_estimators=n_est, max_depth=depth, random_state=random_state)
        pipe = _build_pipeline(est)
        scores = cross_val_score(pipe, X, Y, cv=cv, scoring=scoring, n_jobs=n_jobs)
        return float(scores.mean())

    study = optuna.create_study(
        direction="maximize", sampler=optuna.samplers.TPESampler(seed=random_state, n_startup_trials=5)
    )
    if prior_params:
        try:
            study.enqueue_trial(prior_params)
        except Exception as e:
            logger.debug("auto_tune.enqueue_prior_trial_skipped error=%s", e)
    study.optimize(
        _optuna_objective, n_trials=n_phase2, n_jobs=1, show_progress_bar=False, callbacks=[_progress_callback]
    )

    if study.best_trial:
        params = study.best_params
        alg = params.get("algorithm", best_key)
        if alg == "rf":
            est = RandomForestRegressor(
                n_estimators=params["n_estimators"],
                max_depth=params["max_depth"],
                min_samples_leaf=params["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "gb":
            est = GradientBoostingRegressor(
                n_estimators=params["n_estimators"],
                max_depth=params["max_depth"],
                learning_rate=params["learning_rate"],
                min_samples_leaf=params["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "quantile":
            try:
                tp = get_training_params(model_kind)
                est = GradientBoostingRegressor(
                    n_estimators=params.get("n_estimators", 200),
                    max_depth=params.get("max_depth", 12),
                    learning_rate=params.get("learning_rate", 0.1),
                    min_samples_leaf=params.get("min_samples_leaf", 2),
                    random_state=random_state,
                    loss="quantile",
                    alpha=tp.get("quantile_level", 0.5),
                )
            except (ValueError, KeyError):
                est = GradientBoostingRegressor(
                    n_estimators=params.get("n_estimators", 200),
                    max_depth=params.get("max_depth", 12),
                    random_state=random_state,
                    loss="quantile",
                    alpha=0.5,
                )
        elif alg == "et":
            est = ExtraTreesRegressor(
                n_estimators=params["n_estimators"],
                max_depth=params["max_depth"],
                min_samples_leaf=params["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "hgb":
            est = HistGradientBoostingRegressor(
                max_iter=params["max_iter"],
                max_depth=params["max_depth"],
                learning_rate=params["learning_rate"],
                min_samples_leaf=params["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "mlp":
            est = MLPRegressor(
                hidden_layer_sizes=params.get("hidden_layer_sizes", (128, 64)),
                alpha=params.get("alpha", 0.001),
                learning_rate_init=params.get("learning_rate_init", 0.001),
                max_iter=1000,
                early_stopping=True,
                random_state=random_state,
            )
        else:
            est = RandomForestRegressor(
                n_estimators=params.get("n_estimators", 200),
                max_depth=params.get("max_depth", 12),
                random_state=random_state,
            )
        best_pipe = _build_pipeline(est)
        best_pipe.fit(X, Y)
        best_score = float(study.best_value)
        best_params = {"est__estimator__" + k: v for k, v in params.items()}
    else:
        best_params = results[0][3]

    config_snippet = {
        k.replace("est__estimator__", ""): v for k, v in best_params.items() if k.startswith("est__estimator__")
    }
    if not config_snippet:
        config_snippet = {k.replace("est__", ""): v for k, v in best_params.items()}
    report = {
        "model_kind": model_kind,
        "best_cv_score": best_score,
        "best_params": best_params,
        "config_snippet": config_snippet,
        "scoring": scoring,
        "cv_splits": cv_splits,
        "validation_method": validation_method,
        "algorithms": winners,
        "algorithms_requested": [c[0] for c in candidates],
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "n_targets": int(Y.shape[1]),
        "candidates": [{"algorithm": r[0], "estimator": r[1], "best_score": r[2]} for r in results],
        "phase": "two_phase",
    }
    if best_pipe is not None:
        report["metrics"] = _compute_metrics_regression(best_pipe, X, Y, cv)
    return best_pipe, best_params, report


def _run_search(
    X: np.ndarray,
    Y: np.ndarray,
    model_kind: str,
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
    algorithms: Optional[List[str]] = None,
    validation_method: str = "walk_forward",
    n_jobs_override: Optional[int] = None,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Run RandomizedSearchCV over algorithms and params. Returns (best_pipeline, best_params, report)."""
    tuning_cfg = get_tuning_config()
    algorithms = algorithms or tuning_cfg.get("algorithms", ["rf", "gb", "quantile"])
    validation_method = validation_method or tuning_cfg.get("validation_method", "walk_forward")
    allow = frozenset(a.lower() for a in algorithms)
    candidates = [(k, n, e, p) for k, n, e, p in _search_space_regression(model_kind) if k in allow]
    if not candidates:
        raise ValueError(
            f"No algorithms selected for {model_kind}; available: rf, gb, quantile, et, hgb, stacked, mlp. You requested: {list(algorithms)}"
        )
    cv = _get_cv_object(validation_method, cv_splits, X.shape[0], random_state)
    best_score = None
    best_pipe = None
    best_params = None
    all_cv_results: List[Dict[str, Any]] = []

    n_jobs = _effective_n_jobs(tuning_cfg, n_jobs_override)
    algorithms_used: List[str] = []
    for key, name, base_est, param_dist in candidates:
        pipe = _build_pipeline(base_est)
        search = RandomizedSearchCV(
            pipe,
            param_distributions=param_dist,
            n_iter=min(n_iter, max(1, _count_combinations(param_dist) // 2)),
            cv=cv,
            scoring=scoring,
            random_state=random_state,
            n_jobs=n_jobs,
            error_score="raise",
        )
        search.fit(X, Y)
        algorithms_used.append(key)
        all_cv_results.append(
            {
                "algorithm": key,
                "estimator": name,
                "best_score": float(search.best_score_),
                "best_params": search.best_params_,
            }
        )
        if best_score is None or search.best_score_ > best_score:
            best_score = search.best_score_
            best_pipe = search.best_estimator_
            best_params = dict(search.best_params_)

    # Build config snippet for ml.training.<model> (strip est__estimator__ prefix)
    config_snippet = {}
    if best_params:
        for k, v in best_params.items():
            if k.startswith("est__estimator__"):
                key_str = k.replace("est__estimator__", "")
                config_snippet[key_str] = v

    report = {
        "model_kind": model_kind,
        "best_cv_score": best_score,
        "best_params": best_params,
        "config_snippet": config_snippet,
        "scoring": scoring,
        "cv_splits": cv_splits,
        "validation_method": validation_method,
        "algorithms": algorithms_used,
        "algorithms_requested": algorithms_used,
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "n_targets": int(Y.shape[1]),
        "candidates": all_cv_results,
    }
    if best_pipe is not None:
        report["metrics"] = _compute_metrics_regression(best_pipe, X, Y, cv)
        report["mlqa_audit"] = _compute_mlqa_audit(
            report, best_pipe, X, Y, cv, scoring, "regression", _mlqa_feature_names(model_kind)
        )
    return best_pipe, best_params, report


def _count_combinations(param_dist: Dict[str, Any]) -> int:
    n = 1
    for v in param_dist.values():
        if hasattr(v, "__len__") and not isinstance(v, (str, bytes)):
            n *= len(v)
        else:
            n *= 1
    return n


def _save_artifacts(
    pipeline: Pipeline,
    out_dir: str,
    model_kind: str,
    format_suffix: Optional[str],
    joblib_compress: int,
    report: Dict[str, Any],
) -> None:
    """Extract scaler and model from pipeline; save as existing train_* scripts do."""
    os.makedirs(out_dir, exist_ok=True)
    scaler = pipeline.named_steps["scaler"]
    model = pipeline.named_steps["est"]

    if format_suffix:
        scaler_path = os.path.join(out_dir, f"{model_kind}_scaler_{format_suffix}.joblib")
        model_path = os.path.join(out_dir, f"{model_kind}_model_{format_suffix}.joblib")
        report_path = os.path.join(out_dir, f"tuning_report_{model_kind}_{format_suffix}.json")
    else:
        scaler_path = os.path.join(out_dir, f"{model_kind}_scaler.joblib")
        model_path = os.path.join(out_dir, f"{model_kind}_model.joblib")
        report_path = os.path.join(out_dir, f"tuning_report_{model_kind}.json")

    import joblib

    joblib.dump(scaler, scaler_path, compress=joblib_compress)
    joblib.dump(model, model_path, compress=joblib_compress)
    with open(report_path, "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2)


# ---- Data loading (CSV) ----
def load_batting_csv(path: str) -> Tuple[np.ndarray, np.ndarray]:
    df = pd.read_csv(path)
    for col in ("batting_form_short", "batting_form_long"):
        if col not in df.columns and "batting_form" in df.columns:
            df[col] = df["batting_form"]
    if "batting_momentum" not in df.columns:
        df["batting_momentum"] = 0.0
    for col in BAT_SEQ_COLS:
        if col not in df.columns:
            df[col] = 0.0
        else:
            df[col] = df[col].fillna(0.0)
    required = [c for c in BATTING_FEATURE_COLS if c not in BAT_SEQ_COLS]
    df = df.dropna(subset=[c for c in required if c in df.columns])
    X_raw = df[BATTING_FEATURE_COLS].astype(float).values
    from .feature_transforms import apply_transforms, get_transform_config

    transform_config = get_transform_config("batting")
    if transform_config.get("add_interactions") or transform_config.get("add_log1p"):
        X, _ = apply_transforms(X_raw, list(BATTING_FEATURE_COLS), transform_config, "batting")
    else:
        X = X_raw
    y_cols = [c for c in BATTING_TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if Y.shape[1] < len(BATTING_TARGET_COLS):
        pad = np.zeros((Y.shape[0], len(BATTING_TARGET_COLS) - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
    balls = df.get("balls", pd.Series(np.ones(len(df)))).astype(float).values
    sr = np.zeros((len(runs), 1))
    ok = balls > 0
    sr[ok] = (runs[ok] / balls[ok]) * 100.0
    Y = np.concatenate([Y, sr], axis=1)
    return X, Y


def load_bowling_csv(path: str) -> Tuple[np.ndarray, np.ndarray]:
    df = pd.read_csv(path)
    for col in ("bowling_form_short", "bowling_form_long"):
        if col not in df.columns and "bowling_form" in df.columns:
            df[col] = df["bowling_form"]
    if "bowling_momentum" not in df.columns:
        df["bowling_momentum"] = 0.0
    for col in BOWL_SEQ_COLS:
        if col not in df.columns:
            df[col] = 0.0
        else:
            df[col] = df[col].fillna(0.0)
    required = [c for c in BOWLING_FEATURE_COLS if c not in BOWL_SEQ_COLS]
    df = df.dropna(subset=[c for c in required if c in df.columns])
    X = df[BOWLING_FEATURE_COLS].astype(float).values
    y_cols = [c for c in BOWLING_TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if Y.shape[1] < len(BOWLING_TARGET_COLS):
        pad = np.zeros((Y.shape[0], len(BOWLING_TARGET_COLS) - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
    balls = df.get("balls", pd.Series(np.ones(len(df)) * 6)).astype(float).values
    overs = np.where(balls > 0, balls / 6.0, 1.0)
    econ = np.where(overs > 0, runs / overs, 0.0).reshape(-1, 1)
    Y = np.concatenate([Y, econ], axis=1)
    return X, Y


def load_fielding_csv(path: str, format_code: Optional[str] = None) -> Dict[str, Tuple[np.ndarray, np.ndarray]]:
    if _train_fielding is None:
        logger.error("auto_tune.load_fielding_csv.train_fielding_unavailable")
        raise RuntimeError("ml.train_fielding not available for fielding CSV")
    df = pd.read_csv(path)
    headers = list(df.columns)
    rows = df.values.astype(str).tolist()
    by_format = _train_fielding.rows_to_xy_by_format(headers, rows)
    if format_code and format_code in by_format:
        return {format_code: by_format[format_code]}
    return by_format


def load_batting_from_api(
    go_app_url: str, format_code: str, cutoff: str, api_key: Optional[str]
) -> Tuple[np.ndarray, np.ndarray]:
    from app.train_on_the_fly import _batting_rows_to_xy, fetch_training_data

    data = fetch_training_data(go_app_url, format_code, cutoff, api_key, sections="batting")
    bat = data.get("batting") or {}
    headers = bat.get("headers") or []
    rows = bat.get("rows") or []
    return _batting_rows_to_xy(headers, rows)


def load_bowling_from_api(
    go_app_url: str, format_code: str, cutoff: str, api_key: Optional[str]
) -> Tuple[np.ndarray, np.ndarray]:
    from app.train_on_the_fly import _bowling_rows_to_xy, fetch_training_data

    data = fetch_training_data(go_app_url, format_code, cutoff, api_key, sections="bowling")
    bowl = data.get("bowling") or {}
    headers = bowl.get("headers") or []
    rows = bowl.get("rows") or []
    return _bowling_rows_to_xy(headers, rows)


def load_fielding_from_api(
    go_app_url: str, cutoff: str, api_key: Optional[str], format_filter: Optional[str] = None
) -> Dict[str, Tuple[np.ndarray, np.ndarray]]:
    if _train_fielding is None:
        logger.error("auto_tune.load_fielding_from_api.train_fielding_unavailable")
        raise RuntimeError("ml.train_fielding not available for fielding API")
    field = _train_fielding.fetch_fielding_data(go_app_url, cutoff, api_key)
    headers = field.get("headers") or []
    rows = field.get("rows") or []
    by_format = _train_fielding.rows_to_xy_by_format(headers, rows)
    if format_filter and format_filter in by_format:
        return {format_filter: by_format[format_filter]}
    return by_format


def load_extras_from_api(
    go_app_url: str, cutoff: str, api_key: Optional[str], format_filter: Optional[str] = None
) -> Dict[str, Tuple[np.ndarray, np.ndarray]]:
    if _train_extras is None:
        logger.error("auto_tune.load_extras_from_api.train_extras_unavailable")
        raise RuntimeError("ml.train_extras not available for extras API")
    extras = _train_extras.fetch_extras_data(go_app_url, cutoff, api_key)
    headers = extras.get("headers") or []
    rows = extras.get("rows") or []
    by_format = _train_extras.rows_to_xy_by_format(headers, rows)
    if format_filter and format_filter in by_format:
        return {format_filter: by_format[format_filter]}
    return by_format


def load_win_from_api(
    go_app_url: str, cutoff: str, api_key: Optional[str], format_filter: Optional[str] = None
) -> Dict[str, Tuple[np.ndarray, np.ndarray]]:
    if _train_win is None:
        logger.error("auto_tune.load_win_from_api.train_win_unavailable")
        raise RuntimeError("ml.train_win not available for win API")
    win = _train_win.fetch_win_data(go_app_url, cutoff, api_key)
    headers = win.get("headers") or []
    rows = win.get("rows") or []
    by_format = _train_win.rows_to_xy_by_format(headers, rows)
    if format_filter and format_filter in by_format:
        return {format_filter: by_format[format_filter]}
    return by_format


def run_auto_tune(
    model_kind: str,
    X: np.ndarray,
    Y: np.ndarray,
    format_suffix: Optional[str],
    out_dir: str,
    algorithms: Optional[List[str]] = None,
    validation_method: Optional[str] = None,
    n_jobs_override: Optional[int] = None,
    task_index: int = 0,
    task_total: int = 1,
    use_pycaret: Optional[bool] = None,
    fast_mode: bool = False,
    rescreen: bool = False,
) -> Dict[str, Any]:
    """Run two-phase search, save artifacts and report. Returns report dict."""
    tuning = _get_tuning_config()
    cv_splits = tuning["cv_splits"]
    n_iter = int(tuning["n_iter"])
    if fast_mode:
        n_iter = min(n_iter, 15)
    scoring = tuning["scoring"]
    random_state = tuning.get("random_state") or get_training_params(model_kind).get("random_state", 42)
    params = get_training_params(model_kind)
    joblib_compress = params["joblib_compress"]
    algorithms = algorithms if algorithms is not None else tuning.get("algorithms")
    prior_params: Optional[Dict[str, Any]] = None
    prior = None if rescreen else _get_prior_tuned_algorithm(model_kind, format_suffix, out_dir)
    if prior is not None:
        prior_algo, prior_cfg = prior
        algorithms = [prior_algo]
        if prior_cfg:
            prior_params = _prior_params_to_optuna_regression(prior_algo, prior_cfg)
        logger.info(
            "auto_tune.using_prior_algorithm model=%s format=%s algorithm=%s (skipping screening, fine-tune only)",
            model_kind,
            format_suffix or "(unified)",
            prior_algo,
        )
    if use_pycaret is not False and prior is None:
        pycaret_algos = _maybe_run_pycaret_ranking(
            X, Y, "regression", use_pycaret, model_kind, format_suffix, task_index, task_total
        )
        if pycaret_algos is not None:
            algorithms = pycaret_algos
    validation_method = validation_method or tuning.get("validation_method", "walk_forward")

    best_pipe, best_params, report = _run_search_two_phase(
        X,
        Y,
        model_kind,
        format_suffix,
        cv_splits,
        n_iter,
        scoring,
        random_state,
        algorithms,
        validation_method,
        n_jobs_override,
        task_index,
        task_total,
        prior_params=prior_params,
    )
    _save_artifacts(best_pipe, out_dir, model_kind, format_suffix, joblib_compress, report)
    return report


def run_auto_tune_extras(
    X: np.ndarray,
    Y: np.ndarray,
    format_suffix: Optional[str],
    out_dir: str,
    algorithms: Optional[List[str]] = None,
    validation_method: Optional[str] = None,
    n_jobs_override: Optional[int] = None,
    task_index: int = 0,
    task_total: int = 1,
    use_pycaret: Optional[bool] = None,
    fast_mode: bool = False,
    use_autogluon: Optional[bool] = None,
    rescreen: bool = False,
) -> Dict[str, Any]:
    """Run two-phase single-output regression search for extras; save model only + report."""
    tuning = _get_tuning_config()
    cv_splits = tuning["cv_splits"]
    n_iter = int(tuning["n_iter"])
    if fast_mode:
        n_iter = min(n_iter, 15)
    scoring = tuning.get("scoring", "neg_mean_absolute_error")
    params = get_training_params("extras")
    random_state = tuning.get("random_state") or params.get("random_state", 42)
    joblib_compress = params["joblib_compress"]
    algorithms = algorithms if algorithms is not None else tuning.get("algorithms")
    prior_params = None
    prior = None if rescreen else _get_prior_tuned_algorithm("extras", format_suffix, out_dir)
    if prior is not None:
        prior_algo, prior_cfg = prior
        algorithms = [prior_algo]
        if prior_cfg:
            prior_params = _prior_params_to_optuna_regression(prior_algo, prior_cfg)
        logger.info(
            "auto_tune.using_prior_algorithm model=extras format=%s algorithm=%s (fine-tune only)",
            format_suffix or "(unified)",
            prior_algo,
        )
    if use_pycaret is not False and prior is None:
        pycaret_algos = _maybe_run_pycaret_ranking(
            X, Y, "regression", use_pycaret, "extras", format_suffix, task_index, task_total
        )
        if pycaret_algos is not None:
            algorithms = pycaret_algos
    validation_method = validation_method or tuning.get("validation_method", "walk_forward")
    y = Y.ravel() if Y.ndim > 1 else Y
    best_pipe, _, report = _run_search_two_phase_single_regression(
        X,
        y,
        "extras",
        format_suffix,
        cv_splits,
        n_iter,
        scoring,
        random_state,
        algorithms,
        validation_method,
        n_jobs_override,
        task_index,
        task_total,
        prior_params=prior_params,
    )
    optuna_score = report.get("best_cv_score")
    if optuna_score is not None:
        ag_wins, ag_wrapper, ag_updates = _maybe_run_autogluon_and_compare(
            X, y, "regression", use_autogluon, "extras", format_suffix, out_dir, float(optuna_score)
        )
        report.update(ag_updates)
        if ag_wins and ag_wrapper is not None:
            os.makedirs(out_dir, exist_ok=True)
            model_path = os.path.join(
                out_dir,
                f"extras_model_{format_suffix.replace(' ', '_')}.joblib" if format_suffix else "extras_model.joblib",
            )
            joblib.dump(ag_wrapper, model_path, compress=joblib_compress)
            report_path = os.path.join(
                out_dir, f"tuning_report_extras_{format_suffix}.json" if format_suffix else "tuning_report_extras.json"
            )
            with open(report_path, "w", encoding="utf-8") as f:
                json.dump(report, f, indent=2)
            return report
    _save_artifacts_model_only(best_pipe, out_dir, "extras", format_suffix, joblib_compress, report)
    return report


def _run_search_two_phase_classification(
    X: np.ndarray,
    y: np.ndarray,
    model_kind: str,
    format_suffix: Optional[str],
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
    algorithms: Optional[List[str]] = None,
    validation_method: str = "walk_forward",
    n_jobs_override: Optional[int] = None,
    task_index: int = 0,
    task_total: int = 1,
    prior_params: Optional[Dict[str, Any]] = None,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Two-phase search for classification (win). Skips Phase 1 when single algorithm (prior)."""
    tuning_cfg = get_tuning_config()
    algs = algorithms or ["rf", "gb"]
    if algs == "all" or (isinstance(algs, list) and "all" in [str(a).lower() for a in algs]):
        algs = ["rf", "gb", "et", "hgb"]
        stages = tuning_cfg.get("stages") or {}
        neural = stages.get("neural") or {}
        if neural.get("mlp", True):
            algs = list(algs) + ["mlp"]
    allow = frozenset(str(a).lower().strip() for a in (algs if isinstance(algs, (list, tuple)) else [algs]))
    cv = _get_cv_object(validation_method, cv_splits, X.shape[0], random_state)
    n_jobs = _effective_n_jobs(tuning_cfg, n_jobs_override)
    candidates = _phase1_candidates_classification(allow)
    if not candidates:
        return _run_search_classification(
            X,
            y,
            model_kind,
            cv_splits,
            n_iter,
            scoring,
            random_state,
            algorithms,
            validation_method,
            n_jobs_override,
        )

    results: List[Tuple[str, str, float, Dict[str, Any], Pipeline]] = []
    if len(candidates) == 1 and _HAS_OPTUNA:
        key, name, base_est, param_dist = candidates[0]
        _progress.write_progress(
            phase="fine_tuning",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            algorithm=key,
            message=f"Fine-tune only ({name}), skipping screening",
        )
        pipe = Pipeline([("scaler", StandardScaler()), ("est", base_est)])
        pipe.fit(X, y)
        initial_score = float(cross_val_score(pipe, X, y, cv=cv, scoring=scoring, n_jobs=n_jobs).mean())
        best_params = {}
        if param_dist:
            for k, vs in param_dist.items():
                if isinstance(vs, (list, tuple)) and len(vs) > 0:
                    best_params[k] = vs[0]
                elif vs is not None:
                    best_params[k] = vs
        results = [(key, name, initial_score, best_params, pipe)]
        best_key, best_name, best_score, best_params, best_pipe = key, name, initial_score, best_params, pipe
        winners = [key]
    else:
        _progress.write_progress(
            phase="screening",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            message="Screening algorithms (win)",
            algorithms_screened=[c[0] for c in candidates],
        )
        for key, name, base_est, param_dist in candidates:
            _progress.write_progress(
                phase="screening",
                model_kind=model_kind,
                format_suffix=format_suffix,
                task_index=task_index,
                task_total=task_total,
                algorithm=key,
                message=f"Testing {name}",
            )
            pipe = Pipeline([("scaler", StandardScaler()), ("est", base_est)])
            n_phase1 = min(PHASE1_TRIALS_PER_ALGORITHM, max(1, _count_combinations(param_dist) // 2))
            search = RandomizedSearchCV(
                pipe,
                param_distributions=param_dist,
                n_iter=n_phase1,
                cv=cv,
                scoring=scoring,
                random_state=random_state,
                n_jobs=n_jobs,
                error_score="raise",
            )
            search.fit(X, y)
            best_params = dict(search.best_params_)
            params_display = {k.replace("est__", ""): v for k, v in best_params.items()}
            _progress.write_progress(
                phase="screening",
                model_kind=model_kind,
                format_suffix=format_suffix,
                task_index=task_index,
                task_total=task_total,
                algorithm=key,
                hyperparams=params_display,
                best_score=float(search.best_score_),
            )
            results.append((key, name, float(search.best_score_), best_params, search.best_estimator_))
        results.sort(key=lambda r: r[2], reverse=True)
        best_key, best_name, best_score, best_params, best_pipe = results[0]
        winners = [r[0] for r in results[:2]]
    if not _HAS_OPTUNA:
        config_snippet = {k.replace("est__", ""): v for k, v in best_params.items()}
        report = {
            "model_kind": model_kind,
            "best_cv_score": best_score,
            "best_params": best_params,
            "config_snippet": config_snippet,
            "scoring": scoring,
            "cv_splits": cv_splits,
            "validation_method": validation_method,
            "algorithms": [r[0] for r in results],
            "algorithms_requested": [r[0] for r in results],
            "n_samples": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "candidates": [{"algorithm": r[0], "best_score": r[2]} for r in results],
        }
        if best_pipe:
            report["metrics"] = _compute_metrics_classification(best_pipe, X, y, cv)
            report["mlqa_audit"] = _compute_mlqa_audit(
                report, best_pipe, X, y, cv, scoring, "classification", _mlqa_feature_names(model_kind)
            )
        return best_pipe, best_params, report

    def _obj(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        if alg == "rf":
            est = RandomForestClassifier(
                n_estimators=trial.suggest_int("n_estimators", 50, 350, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        elif alg == "gb":
            est = GradientBoostingClassifier(
                n_estimators=trial.suggest_int("n_estimators", 50, 350, step=50),
                max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
                learning_rate=trial.suggest_float("learning_rate", 0.01, 0.2, log=True),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        elif alg == "et":
            est = ExtraTreesClassifier(
                n_estimators=trial.suggest_int("n_estimators", 50, 350, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        elif alg == "mlp":
            sizes = trial.suggest_categorical("hidden_layer_sizes", [(64, 64), (128, 64), (128, 128, 64)])
            alpha = trial.suggest_float("alpha", 1e-4, 1e-1, log=True)
            lr_init = trial.suggest_float("learning_rate_init", 1e-4, 1e-1, log=True)
            est = MLPClassifier(
                hidden_layer_sizes=sizes,
                alpha=alpha,
                learning_rate_init=lr_init,
                max_iter=1000,
                early_stopping=True,
                random_state=random_state,
            )
        else:
            est = HistGradientBoostingClassifier(
                max_iter=trial.suggest_int("max_iter", 50, 400, step=50),
                max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
                learning_rate=trial.suggest_float("learning_rate", 0.01, 0.2, log=True),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 1, 8),
                random_state=random_state,
            )
        pipe = Pipeline([("scaler", StandardScaler()), ("est", est)])
        scores = cross_val_score(pipe, X, y, cv=cv, scoring=scoring, n_jobs=n_jobs)
        return float(scores.mean())

    def _cb(study: Any, trial: Any) -> None:
        _progress.write_progress(
            phase="fine_tuning",
            model_kind=model_kind,
            format_suffix=format_suffix,
            task_index=task_index,
            task_total=task_total,
            algorithm=str(trial.params.get("algorithm", best_key)),
            hyperparams=trial.params,
            trial=trial.number + 1,
            trials_total=min(n_iter, PHASE2_TRIALS),
            best_score=float(study.best_value) if study.best_trial else best_score,
            best_algorithm=best_key,
            message=f"Fine-tuning {best_name}",
        )

    n_phase2 = min(n_iter, PHASE2_TRIALS)
    study = optuna.create_study(
        direction="maximize", sampler=optuna.samplers.TPESampler(seed=random_state, n_startup_trials=5)
    )
    if prior_params:
        try:
            study.enqueue_trial(prior_params)
        except Exception as e:
            logger.debug("auto_tune.enqueue_prior_trial_skipped error=%s", e)
    study.optimize(_obj, n_trials=n_phase2, n_jobs=1, show_progress_bar=False, callbacks=[_cb])
    if study.best_trial:
        p = study.best_params
        alg = p.get("algorithm", best_key)
        if alg == "rf":
            est = RandomForestClassifier(
                n_estimators=p["n_estimators"],
                max_depth=p["max_depth"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "gb":
            est = GradientBoostingClassifier(
                n_estimators=p["n_estimators"],
                max_depth=p["max_depth"],
                learning_rate=p["learning_rate"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "et":
            est = ExtraTreesClassifier(
                n_estimators=p["n_estimators"],
                max_depth=p["max_depth"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        elif alg == "mlp":
            est = MLPClassifier(
                hidden_layer_sizes=p.get("hidden_layer_sizes", (128, 64)),
                alpha=p.get("alpha", 0.001),
                learning_rate_init=p.get("learning_rate_init", 0.001),
                max_iter=1000,
                early_stopping=True,
                random_state=random_state,
            )
        else:
            est = HistGradientBoostingClassifier(
                max_iter=p["max_iter"],
                max_depth=p["max_depth"],
                learning_rate=p["learning_rate"],
                min_samples_leaf=p["min_samples_leaf"],
                random_state=random_state,
            )
        best_pipe = Pipeline([("scaler", StandardScaler()), ("est", est)])
        best_pipe.fit(X, y)
        best_score = float(study.best_value)
        best_params = {"est__" + k: v for k, v in p.items()}
    config_snippet = {k.replace("est__", ""): v for k, v in best_params.items()}
    report = {
        "model_kind": model_kind,
        "best_cv_score": best_score,
        "best_params": best_params,
        "config_snippet": config_snippet,
        "scoring": scoring,
        "cv_splits": cv_splits,
        "validation_method": validation_method,
        "algorithms": winners,
        "algorithms_requested": [r[0] for r in results],
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "candidates": [{"algorithm": r[0], "best_score": r[2]} for r in results],
    }
    if best_pipe:
        report["metrics"] = _compute_metrics_classification(best_pipe, X, y, cv)
        report["mlqa_audit"] = _compute_mlqa_audit(
            report, best_pipe, X, y, cv, scoring, "classification", _mlqa_feature_names(model_kind)
        )
    return best_pipe, best_params, report


def run_auto_tune_win(
    X: np.ndarray,
    Y: np.ndarray,
    format_suffix: Optional[str],
    out_dir: str,
    algorithms: Optional[List[str]] = None,
    validation_method: Optional[str] = None,
    n_jobs_override: Optional[int] = None,
    task_index: int = 0,
    task_total: int = 1,
    use_pycaret: Optional[bool] = None,
    fast_mode: bool = False,
    use_autogluon: Optional[bool] = None,
    rescreen: bool = False,
) -> Dict[str, Any]:
    """Run two-phase classification search for win; save model only + report."""
    tuning = _get_tuning_config()
    cv_splits = tuning["cv_splits"]
    n_iter = int(tuning["n_iter"])
    if fast_mode:
        n_iter = min(n_iter, 15)
    scoring = "accuracy"
    params = get_training_params("win")
    random_state = tuning.get("random_state") or params.get("random_state", 42)
    joblib_compress = params["joblib_compress"]
    algorithms = algorithms if algorithms is not None else tuning.get("algorithms")
    prior_params = None
    prior = None if rescreen else _get_prior_tuned_algorithm("win", format_suffix, out_dir)
    if prior is not None:
        prior_algo, prior_cfg = prior
        algorithms = [prior_algo]
        if prior_cfg:
            prior_params = _prior_params_to_optuna_regression(prior_algo, prior_cfg)
        logger.info(
            "auto_tune.using_prior_algorithm model=win format=%s algorithm=%s (fine-tune only)",
            format_suffix or "(unified)",
            prior_algo,
        )
    if use_pycaret is not False and prior is None:
        pycaret_algos = _maybe_run_pycaret_ranking(
            X, Y, "classification", use_pycaret, "win", format_suffix, task_index, task_total
        )
        if pycaret_algos is not None:
            algorithms = pycaret_algos
    validation_method = validation_method or tuning.get("validation_method", "walk_forward")
    y = Y.ravel() if Y.ndim > 1 else Y
    best_pipe, _, report = _run_search_two_phase_classification(
        X,
        y,
        "win",
        format_suffix,
        cv_splits,
        n_iter,
        scoring,
        random_state,
        algorithms,
        validation_method,
        n_jobs_override,
        task_index,
        task_total,
        prior_params=prior_params,
    )
    optuna_score = report.get("best_cv_score")
    if optuna_score is not None:
        ag_wins, ag_wrapper, ag_updates = _maybe_run_autogluon_and_compare(
            X, y, "classification", use_autogluon, "win", format_suffix, out_dir, float(optuna_score)
        )
        report.update(ag_updates)
        if ag_wins and ag_wrapper is not None:
            os.makedirs(out_dir, exist_ok=True)
            model_path = os.path.join(
                out_dir,
                f"win_model_{format_suffix.replace(' ', '_')}.joblib" if format_suffix else "win_model.joblib",
            )
            joblib.dump(ag_wrapper, model_path, compress=joblib_compress)
            report_path = os.path.join(
                out_dir, f"tuning_report_win_{format_suffix}.json" if format_suffix else "tuning_report_win.json"
            )
            with open(report_path, "w", encoding="utf-8") as f:
                json.dump(report, f, indent=2)
            return report
    _save_artifacts_model_only(best_pipe, out_dir, "win", format_suffix, joblib_compress, report)
    return report


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    parser = argparse.ArgumentParser(description="Auto-tune ML models (batting, bowling, fielding, extras, win)")
    parser.add_argument(
        "--model",
        choices=["batting", "bowling", "fielding", "extras", "win", "all"],
        default="batting",
    )
    parser.add_argument("--csv", default="", help="Path to CSV (for batting/bowling/fielding)")
    parser.add_argument("--from-api", action="store_true", help="Fetch data from go-app training-data API")
    parser.add_argument("--cutoff", default="", help="RFC3339 cutoff (required with --from-api)")
    parser.add_argument(
        "--format", default="", help="Format code (e.g. T20, ODI); used for artifact suffix and API filter"
    )
    parser.add_argument("--all-formats", action="store_true", help="Loop over ml.formats and tune each (CSV only)")
    parser.add_argument("--out", default="", help="Output dir (default: ML_SERVICE_OUTPUT_DIR or config)")
    parser.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""))
    parser.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""))
    parser.add_argument(
        "--algorithms",
        default="",
        help="Comma-separated algorithms to tune: rf, gb, quantile (regression only), stacked (batting/bowling/fielding only). Default: all from config.",
    )
    parser.add_argument(
        "--validation-method",
        choices=["kfold", "walk_forward"],
        default="",
        help="Validation method: kfold or walk_forward (temporal). Default: walk_forward (from config).",
    )
    parser.add_argument(
        "--parallel",
        action="store_true",
        help="Run multiple (model, format) tasks in parallel, using up to 80%% of available CPUs (each task uses 1 job).",
    )
    parser.add_argument(
        "--no-pycaret",
        action="store_true",
        help="Skip PyCaret algorithm ranking (use config algorithms only).",
    )
    parser.add_argument(
        "--fast",
        action="store_true",
        help="Fast mode: reduce Optuna trials to 15, skip PyCaret and AutoGluon.",
    )
    parser.add_argument(
        "--no-autogluon",
        action="store_true",
        help="Skip AutoGluon accuracy boost stage.",
    )
    parser.add_argument(
        "--rescreen",
        action="store_true",
        help="Force full algorithm screening even when prior tuned params exist (ignore fine-tune-only mode).",
    )
    args = parser.parse_args()

    logger.info(
        "pipeline: auto_tune starting model=%s from_api=%s format=%s all_formats=%s out_dir=%s",
        args.model,
        args.from_api,
        args.format or "(detected)",
        args.all_formats,
        args.out or "(default)",
    )

    algorithms_override: Optional[List[str]] = None
    if args.algorithms:
        algorithms_override = [a.strip().lower() for a in args.algorithms.split(",") if a.strip()]
    validation_method_override: Optional[str] = args.validation_method or None
    use_pycaret: Optional[bool] = False if (args.no_pycaret or args.fast) else None
    fast_mode: bool = bool(args.fast)
    use_autogluon: Optional[bool] = False if (args.no_autogluon or args.fast) else None

    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR")
    if not out_dir:
        from ml.config import default_artifacts_dir

        out_dir = default_artifacts_dir()

    progress_path = os.environ.get("AUTO_TUNE_PROGRESS_FILE") or os.path.join(out_dir, "auto_tune_progress.json")
    _progress.set_progress_file(progress_path)

    def _config_formats() -> List[str]:
        try:
            with open(os.path.join(_ML_ROOT, "config.json"), "r", encoding="utf-8") as f:
                data = json.load(f)
                fmts = data.get("ml", {}).get("formats") or []
                return [str(x).upper() for x in fmts if isinstance(x, (str, int))]
        except Exception:
            return ["T20", "ODI", "T20I"]

    models = ["batting", "bowling", "fielding", "extras", "win"] if args.model == "all" else [args.model]
    formats_to_run: List[Optional[str]] = [None]
    if args.all_formats:
        formats_to_run = _config_formats()
    elif args.format:
        formats_to_run = [args.format.strip().upper()]

    def _maybe_save_tuned_params(
        go_app_url: str,
        model: str,
        format_suffix: Optional[str],
        report: Dict[str, Any],
        api_key: Optional[str],
    ) -> None:
        """Save best params to go-app per (model, format) so they are stored in DB and used when retraining (GO_APP_URL must be set)."""
        if not go_app_url or not report.get("config_snippet"):
            return
        params_to_save = dict(report["config_snippet"])
        params_to_save["algorithms"] = report.get("algorithms", [])
        params_to_save["validation_method"] = report.get("validation_method", "walk_forward")
        # Build metrics for DB: always include best_cv_score and tuning context for debugging
        metrics_to_save: Dict[str, Any] = dict(report.get("metrics") or {})
        if report.get("best_cv_score") is not None:
            metrics_to_save["best_cv_score"] = report["best_cv_score"]
        for key in ("scoring", "cv_splits", "validation_method", "n_samples", "n_features", "n_targets"):
            if report.get(key) is not None:
                metrics_to_save[key] = report[key]
        try:
            save_tuned_params_to_go_app(
                go_app_url, model, format_suffix or "", params_to_save, api_key, metrics=metrics_to_save
            )
            logger.info("auto_tune.params_saved_to_db model=%s format=%s", model, format_suffix or "(unified)")
        except ValueError as e:
            logger.warning(
                "auto_tune.save_tuned_params_failed model=%s format=%s error=%s",
                model,
                format_suffix,
                e,
            )

    def _build_parallel_tasks() -> List[List[str]]:
        """Build list of argv for each (model, format) to run as subprocess (AUTO_TUNE_N_JOBS=1)."""
        cmds: List[List[str]] = []
        for model_kind in models:
            if model_kind in ("extras", "win") and not args.from_api:
                continue
            for fmt in formats_to_run:
                argv = [sys.executable, "-m", "ml.auto_tune", "--model", model_kind, "--out", out_dir]
                if fmt:
                    argv.extend(["--format", fmt])
                if args.from_api:
                    argv.extend(["--from-api", "--cutoff", args.cutoff or "", "--go-app-url", args.go_app_url or ""])
                    if args.api_key:
                        argv.extend(["--api-key", args.api_key])
                if args.csv:
                    argv.extend(["--csv", args.csv])
                if algorithms_override:
                    argv.extend(["--algorithms", ",".join(algorithms_override)])
                if validation_method_override:
                    argv.extend(["--validation-method", validation_method_override])
                if args.no_pycaret:
                    argv.append("--no-pycaret")
                if args.fast:
                    argv.append("--fast")
                if args.no_autogluon:
                    argv.append("--no-autogluon")
                if args.rescreen:
                    argv.append("--rescreen")
                cmds.append(argv)
        return cmds

    try:
        if args.parallel:
            parallel_tasks = _build_parallel_tasks()
            cpu_count = os.cpu_count() or 1
            max_workers = min(len(parallel_tasks), max(1, int(cpu_count * 0.8)))
            if len(parallel_tasks) <= 1:
                logger.info("auto_tune.parallel only one task, running sequentially")
            else:
                logger.info(
                    "auto_tune.parallel running %s tasks with max_workers=%s (80%% of %s CPUs)",
                    len(parallel_tasks),
                    max_workers,
                    cpu_count,
                )
                env = os.environ.copy()
                env["AUTO_TUNE_N_JOBS"] = "1"
                failed = 0
                with ThreadPoolExecutor(max_workers=max_workers) as executor:
                    futures = {
                        executor.submit(subprocess.run, argv, env=env, cwd=_ML_ROOT, capture_output=False): argv
                        for argv in parallel_tasks
                    }
                    for future in as_completed(futures):
                        argv = futures[future]
                        try:
                            result = future.result()
                            if result.returncode != 0:
                                failed += 1
                                logger.warning("auto_tune.parallel task failed: %s", " ".join(argv[:10]))
                        except Exception as e:
                            failed += 1
                            logger.warning("auto_tune.parallel task error: %s", e)
                if failed:
                    sys.exit(1)
                return

        for model_kind in models:
            for fmt in formats_to_run:
                format_suffix = fmt if fmt else None
                if args.from_api:
                    if not args.go_app_url or not args.cutoff:
                        logger.error("auto_tune.from_api_requires_go_app_url_and_cutoff")
                        sys.exit(1)
                    try:
                        if model_kind == "extras":
                            by_f = load_extras_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt)
                            if not by_f:
                                logger.warning("auto_tune.no_extras_data format=%s", fmt)
                                continue
                            for fcode, (X, Y) in by_f.items():
                                if X.size == 0 or Y.size == 0:
                                    continue
                                report = run_auto_tune_extras(
                                    X,
                                    Y,
                                    fcode,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    use_autogluon=use_autogluon,
                                    rescreen=args.rescreen,
                                )
                                _maybe_save_tuned_params(args.go_app_url, "extras", fcode, report, args.api_key or None)
                                logger.info(
                                    "auto_tune.done model=extras format=%s n=%s best_cv_score=%s",
                                    fcode,
                                    X.shape[0],
                                    report["best_cv_score"],
                                )
                            continue
                        if model_kind == "win":
                            by_f = load_win_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt)
                            if not by_f:
                                logger.warning("auto_tune.no_win_data format=%s", fmt)
                                continue
                            for fcode, (X, Y) in by_f.items():
                                if X.size == 0 or Y.size == 0:
                                    continue
                                report = run_auto_tune_win(
                                    X,
                                    Y,
                                    fcode,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    use_autogluon=use_autogluon,
                                    rescreen=args.rescreen,
                                )
                                _maybe_save_tuned_params(args.go_app_url, "win", fcode, report, args.api_key or None)
                                logger.info(
                                    "auto_tune.done model=win format=%s n=%s best_cv_score=%s",
                                    fcode,
                                    X.shape[0],
                                    report["best_cv_score"],
                                )
                            continue
                        if model_kind == "batting":
                            X, Y = load_batting_from_api(
                                args.go_app_url, fmt or "all", args.cutoff, args.api_key or None
                            )
                        elif model_kind == "bowling":
                            X, Y = load_bowling_from_api(
                                args.go_app_url, fmt or "all", args.cutoff, args.api_key or None
                            )
                        else:
                            by_f = load_fielding_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt)
                            if not by_f:
                                logger.warning("auto_tune.no_fielding_data format=%s", fmt)
                                continue
                            # Run once per format from API
                            for fcode, (X, Y) in by_f.items():
                                if X.size == 0 or Y.size == 0:
                                    continue
                                report = run_auto_tune(
                                    model_kind,
                                    X,
                                    Y,
                                    fcode,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    rescreen=args.rescreen,
                                )
                                _maybe_save_tuned_params(
                                    args.go_app_url, model_kind, fcode, report, args.api_key or None
                                )
                                logger.info(
                                    "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                                    model_kind,
                                    fcode,
                                    X.shape[0],
                                    report["best_cv_score"],
                                )
                            continue
                    except (ValueError, RuntimeError) as e:
                        logger.error("auto_tune.from_api_load_failed model=%s format=%s error=%s", model_kind, fmt, e)
                        raise SystemExit(1) from e
                    if X.size == 0 or Y.size == 0:
                        logger.warning("auto_tune.no_data model=%s format=%s", model_kind, fmt)
                        continue
                    report = run_auto_tune(
                        model_kind,
                        X,
                        Y,
                        format_suffix,
                        out_dir,
                        algorithms_override,
                        validation_method_override,
                        use_pycaret=use_pycaret,
                        fast_mode=fast_mode,
                        rescreen=args.rescreen,
                    )
                    _maybe_save_tuned_params(args.go_app_url, model_kind, format_suffix, report, args.api_key or None)
                    logger.info(
                        "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                        model_kind,
                        format_suffix,
                        X.shape[0],
                        report["best_cv_score"],
                    )
                else:
                    # CSV (extras and win are API-only)
                    if model_kind in ("extras", "win"):
                        logger.warning(
                            "auto_tune.skip_extras_win_require_from_api model=%s hint=Use --from-api and --cutoff",
                            model_kind,
                        )
                        continue
                    if model_kind == "fielding":
                        default_dir = os.environ.get(
                            "GO_APP_OUTPUT_DIR", os.path.join(_ML_ROOT, "..", "output", "go-app")
                        )
                        csv_path = args.csv or os.path.join(default_dir, f"fielding_encoded_{fmt or 'ALL'}.csv")
                        if not os.path.isfile(csv_path) and not args.csv:
                            csv_path = args.csv or ""
                        if not csv_path or not os.path.isfile(csv_path):
                            logger.warning("auto_tune.skip_csv_not_found model=%s format=%s", model_kind, fmt)
                            continue
                        try:
                            by_f = load_fielding_csv(csv_path, fmt)
                        except (RuntimeError, FileNotFoundError) as e:
                            logger.error("auto_tune.load_fielding_csv_failed path=%s error=%s", csv_path, e)
                            continue
                        for fcode, (X, Y) in by_f.items():
                            if X.size == 0 or Y.size == 0:
                                continue
                            report = run_auto_tune(
                                model_kind,
                                X,
                                Y,
                                fcode,
                                out_dir,
                                algorithms_override,
                                validation_method_override,
                                use_pycaret=use_pycaret,
                                fast_mode=fast_mode,
                                rescreen=args.rescreen,
                            )
                            _maybe_save_tuned_params(args.go_app_url, model_kind, fcode, report, args.api_key or None)
                            logger.info(
                                "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                                model_kind,
                                fcode,
                                X.shape[0],
                                report["best_cv_score"],
                            )
                        continue
                    default_dir = os.environ.get("GO_APP_OUTPUT_DIR", os.path.join(_ML_ROOT, "..", "output", "go-app"))
                    csv_path = args.csv or os.path.join(default_dir, f"{model_kind}_encoded_{fmt or 'LEGACY'}.csv")
                    if not os.path.isfile(csv_path):
                        csv_path = args.csv or os.path.join(default_dir, f"{model_kind}_encoded.csv")
                    if not os.path.isfile(csv_path):
                        logger.warning(
                            "auto_tune.skip_csv_not_found model=%s format=%s path=%s", model_kind, fmt, csv_path
                        )
                        continue
                    try:
                        if model_kind == "batting":
                            X, Y = load_batting_csv(csv_path)
                        else:
                            X, Y = load_bowling_csv(csv_path)
                    except Exception as e:
                        logger.error("auto_tune.load_csv_failed model=%s path=%s error=%s", model_kind, csv_path, e)
                        continue
                    if X.size == 0 or Y.size == 0:
                        logger.warning("auto_tune.no_data_in_csv path=%s", csv_path)
                        continue
                    report = run_auto_tune(
                        model_kind,
                        X,
                        Y,
                        format_suffix,
                        out_dir,
                        algorithms_override,
                        validation_method_override,
                        use_pycaret=use_pycaret,
                        fast_mode=fast_mode,
                        rescreen=args.rescreen,
                    )
                    _maybe_save_tuned_params(args.go_app_url, model_kind, format_suffix, report, args.api_key or None)
                    logger.info(
                        "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                        model_kind,
                        format_suffix,
                        X.shape[0],
                        report["best_cv_score"],
                    )
    finally:
        _progress.clear_progress()


if __name__ == "__main__":
    main()
