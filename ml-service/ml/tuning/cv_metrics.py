"""Cross-validation, metrics computation, MLQA audit, and feature importance."""

from __future__ import annotations

import logging
import os
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
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
from sklearn.model_selection import (
    KFold,
    TimeSeriesSplit,
    cross_val_predict,
    cross_val_score,
    learning_curve,
)
from sklearn.pipeline import Pipeline

from ml.config import get_mlqa_config, get_tuning_config
from ml.tuning.types import (
    BATTING_FEATURE_COLS,
    BOWLING_FEATURE_COLS,
    _train_extras,
    _train_fielding,
    _train_innings,
    _train_win,
)
from ml.utils import extract_feature_importance_from_estimator

logger = logging.getLogger(__name__)

# Simpler algorithms for complexity check: prefer these if within 1% of best
_MLQA_SIMPLER_ALGS = frozenset({"rf", "gb", "et", "hgb", "quantile"})
_MLQA_COMPLEX_ALGS = frozenset({"mlp", "stacked"})
_MLQA_THRESHOLD_FALLBACK_MSG = "MLQA config not found or invalid, using default thresholds for overfitting/stability."


def _mlqa_overfitting_and_stability_thresholds() -> Tuple[float, float]:
    """Return (overfitting_delta_threshold, stability_fold_std_threshold) from MLQA or hard-coded defaults.

    Thresholds are *relative* (fraction of score magnitude). Default 0.10 means 10%.
    """
    try:
        mlqa = get_mlqa_config()
        return (mlqa["overfitting_delta_threshold"], mlqa["stability_fold_std_threshold"])
    except Exception:
        logger.warning(_MLQA_THRESHOLD_FALLBACK_MSG)
        return (0.10, 0.08)


def _to_relative(absolute_value: float, reference_score: float) -> float:
    """Convert an absolute metric (delta or std) to a relative fraction of the score magnitude.

    For neg_mean_absolute_error the scores are large negative numbers (e.g. -7.9);
    comparing an absolute std of 0.12 against a fixed 0.05 threshold is meaningless.
    Instead we compute 0.12 / abs(-7.9) ≈ 0.015 (1.5%) which is a fair comparison.
    """
    magnitude = abs(reference_score)
    if magnitude < 1e-9:
        return 0.0
    return abs(absolute_value) / magnitude


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
# min_samples_leaf biased higher (4–16) to favor stability and reduce CV fold variance
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


def _effective_timeseries_gap(gap: int, n_samples: int, small_dataset_threshold: int = 5000) -> int:
    """Cap gap for small datasets; large gaps hurt small models (e.g. win with ~2k samples).

    Use gap=0 when n_samples < small_dataset_threshold so win/extras keep prior behavior. For larger
    datasets (bowling, batting), cap gap at 5% of samples to avoid over-restrictive splits.
    Config: ml.tuning.timeseries_small_dataset_threshold.
    """
    if gap <= 0:
        return 0
    if n_samples < small_dataset_threshold:
        return 0  # Small datasets: no gap to avoid regressions (win, extras, etc.)
    return min(gap, max(1, n_samples // 20))  # Cap at ~5% of data


def _get_cv_object(
    validation_method: str,
    cv_splits: int,
    n_samples: int,
    random_state: int = 42,
    gap: int = 0,
    small_dataset_threshold: int = 5000,
):
    """Return a CV splitter for RandomizedSearchCV. validation_method: kfold | walk_forward.

    When walk_forward, uses TimeSeriesSplit with optional gap (samples between train/test) to
    reduce temporal leakage. Config: ml.tuning.timeseries_split_gap. Gap is capped for small
    datasets via _effective_timeseries_gap (ml.tuning.timeseries_small_dataset_threshold).
    """
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
        effective_gap = _effective_timeseries_gap(max(0, int(gap)), n_samples, small_dataset_threshold)
        return TimeSeriesSplit(n_splits=n_splits, gap=effective_gap)
    return KFold(n_splits=kfold_splits, shuffle=True, random_state=random_state)


def _compute_metrics_regression(
    pipe: Pipeline,
    X: np.ndarray,
    y: np.ndarray,
    cv: Any,
    target_names: Optional[List[str]] = None,
) -> Dict[str, Any]:
    """Compute regression metrics from cross-validated predictions.

    Returns dict with mae, rmse, r2, r2_pct, median_ae, max_error, explained_variance,
    target_context (mean, std, min, max for MAE interpretation), baseline comparison
    (naive MAE, improvement %), learning_curve summary, and per_target_mae when
    target_names is provided for multi-output models.
    - mae: mean absolute error (interpretable units)
    - baseline_improvement_pct: top-level for UI (how much better than naive)
    - mae_pct_of_mean: top-level for UI (MAE as % of target mean)
    - per_target_mae: per-target MAE for multi-output (e.g. bowling runs, balls, wickets)
    """
    try:
        y_arr = np.asarray(y)
        y_flat = y_arr.ravel()
        y_pred = cross_val_predict(pipe, X, y, cv=cv)
        y_pred_flat = np.asarray(y_pred).ravel()
        mae = float(mean_absolute_error(y_flat, y_pred_flat))
        rmse = float(np.sqrt(mean_squared_error(y_flat, y_pred_flat)))
        r2 = float(r2_score(y_flat, y_pred_flat))
        # r2 can be negative; clamp for display
        r2_pct = max(0.0, min(100.0, r2 * 100))
        median_ae = float(median_absolute_error(y_flat, y_pred_flat))
        worst_err = float(max_error(y_flat, y_pred_flat))
        expl_var = float(explained_variance_score(y_flat, y_pred_flat))

        # Target context: MAE vs target scale (e.g. extras mean 10–12, MAE 6 = ~50% error)
        target_mean = float(np.mean(y_flat))
        target_std = float(np.std(y_flat)) if len(y_flat) > 1 else 0.0
        target_min = float(np.min(y_flat))
        target_max = float(np.max(y_flat))
        mae_pct_of_mean = round((mae / target_mean * 100), 2) if target_mean != 0 else None

        # Baseline comparison: naive model predicts mean every time
        naive_pred = np.full_like(y_flat, target_mean)
        baseline_mae = float(mean_absolute_error(y_flat, naive_pred))
        baseline_improvement_pct = round((baseline_mae - mae) / baseline_mae * 100, 2) if baseline_mae > 0 else 0.0

        out: Dict[str, Any] = {
            "mae": round(mae, 4),
            "rmse": round(rmse, 4),
            "r2": round(r2, 4),
            "r2_pct": round(r2_pct, 2),
            "median_ae": round(median_ae, 4),
            "max_error": round(worst_err, 4),
            "explained_variance": round(expl_var, 4),
            # Top-level for UI: key tuning/eval metrics
            "baseline_improvement_pct": baseline_improvement_pct,
            "mae_pct_of_mean": mae_pct_of_mean,
            "target_mean": round(target_mean, 4),
            "target_std": round(target_std, 4),
            "target_context": {
                "target_mean": round(target_mean, 4),
                "target_std": round(target_std, 4),
                "target_min": round(target_min, 4),
                "target_max": round(target_max, 4),
                "mae_pct_of_mean": mae_pct_of_mean,
            },
            "baseline_comparison": {
                "baseline_mae": round(baseline_mae, 4),
                "baseline_improvement_pct": baseline_improvement_pct,
            },
        }

        # Per-target MAE for multi-output (bowling: runs, balls, wickets; batting: runs, balls, etc.)
        if target_names and y_arr.ndim == 2 and y_arr.shape[1] > 1:
            y_pred_arr = np.asarray(y_pred)
            n_t = y_arr.shape[1]
            if y_pred_arr.ndim == 2 and y_pred_arr.shape[1] >= n_t:
                per_target: Dict[str, float] = {}
                for j in range(min(n_t, len(target_names))):
                    mae_j = float(mean_absolute_error(y_arr[:, j], y_pred_arr[:, j]))
                    per_target[f"mae_{target_names[j]}"] = round(mae_j, 4)
                out["per_target_mae"] = per_target

        # Learning curve: does validation still improve with more data? overfitting?
        lc = _compute_learning_curve_regression(pipe, X, y, cv, "neg_mean_absolute_error")
        if lc:
            out["learning_curve"] = lc
            # Top-level for UI
            out["overfitting_gap"] = lc.get("overfitting_gap")
            out["val_still_improving"] = lc.get("val_still_improving")

        return out
    except Exception as e:
        logger.warning("auto_tune.compute_metrics_regression_failed error=%s", e)
        return {}


def _compute_learning_curve_regression(
    pipe: Pipeline, X: np.ndarray, y: np.ndarray, cv: Any, scoring: str = "neg_mean_absolute_error"
) -> Optional[Dict[str, Any]]:
    """Compute learning curve summary for regression models.

    Shows whether validation is still improving with more data (needing more data) or
    if train-val gap is large (overfitting, needing higher regularization).

    Returns dict with:
    - val_still_improving: True if val score at 100% train size > val at ~50%
    - overfitting_gap: train_score - val_score at max size (large = overfitting)
    - train_sizes: fractions of data used
    - train_scores_mean, val_scores_mean: mean scores per train size
    """
    try:
        n_samples = X.shape[0]
        if n_samples < 20:
            return None
        # Use 5 fractions: 0.2, 0.4, 0.6, 0.8, 1.0
        train_sizes_frac = np.linspace(0.2, 1.0, 5)
        train_sizes_abs, train_scores, val_scores = learning_curve(
            pipe, X, y, cv=cv, scoring=scoring, train_sizes=train_sizes_frac, n_jobs=1
        )
        train_mean = np.mean(train_scores, axis=1)
        val_mean = np.mean(val_scores, axis=1)
        n_pts = len(train_sizes_frac)
        mid_idx = max(0, n_pts // 2 - 1)
        val_at_mid = val_mean[mid_idx]
        val_at_full = val_mean[-1]
        val_still_improving = val_at_full > val_at_mid
        overfitting_gap = float(train_mean[-1] - val_mean[-1])
        return {
            "val_still_improving": bool(val_still_improving),
            "overfitting_gap": round(overfitting_gap, 4),
            "train_sizes_frac": [round(float(x), 2) for x in train_sizes_frac],
            "train_scores_mean": [round(float(x), 4) for x in train_mean],
            "val_scores_mean": [round(float(x), 4) for x in val_mean],
        }
    except Exception as e:
        logger.warning("auto_tune.learning_curve_failed error=%s", e)
        return None


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


def _mlqa_feature_names(model_kind: str) -> Optional[List[str]]:
    """Return feature names for MLQA sensitivity analysis and feature importance when available."""
    if model_kind == "batting":
        return BATTING_FEATURE_COLS
    if model_kind == "bowling":
        return BOWLING_FEATURE_COLS
    if model_kind == "fielding" and _train_fielding is not None:
        return _train_fielding.FIELDING_FEATURE_COLS
    if model_kind == "extras" and _train_extras is not None:
        return _train_extras.EXTRAS_FEATURE_COLS
    if model_kind == "win" and _train_win is not None:
        return _train_win.WIN_FEATURE_COLS
    if model_kind == "innings" and _train_innings is not None:
        return _train_innings.INNINGS_FEATURE_COLS
    return None


def _extract_feature_importance(
    pipe: Pipeline,
    feature_names: Optional[List[str]],
    n_features: int,
) -> Optional[Dict[str, float]]:
    """Extract feature importance from the best pipeline (tree-based models only).

    Returns dict {feature_name: importance} or None if not available (e.g. MLP, linear).
    """
    est = pipe.named_steps.get("est") if pipe else None
    return extract_feature_importance_from_estimator(est, feature_names, max_features=n_features)


def compute_mlqa_overfitting_stability(
    pipe: Pipeline,
    X: np.ndarray,
    y: np.ndarray,
    cv: Any,
    scoring: str,
    val_score: float,
    fold_std_override: Optional[float] = None,
) -> Tuple[bool, float, float, float]:
    """Compute overfitting delta, CV fold std, and combined violation; return pass and metrics.

    Used by auto-tune to rank candidates: prefer pass, then lowest violation (closest to pass),
    then best CV score. violation = how much over threshold (delta + sigma excess).
    Returns: (passes, delta, fold_std, violation).
    """
    from sklearn.base import clone
    from sklearn.metrics import get_scorer

    delta_thresh, std_thresh = _mlqa_overfitting_and_stability_thresholds()
    pipe_fit = clone(pipe)
    pipe_fit.fit(X, y)
    scorer = get_scorer(scoring)
    train_score_val = scorer(pipe_fit, X, y)
    abs_delta = abs(float(train_score_val) - float(val_score))
    if fold_std_override is not None:
        fold_std = float(fold_std_override)
    else:
        fold_scores = cross_val_score(pipe, X, y, cv=cv, scoring=scoring)
        fold_std = float(np.std(fold_scores))
    # Use relative metrics: fraction of score magnitude (handles neg_mean_absolute_error scale)
    rel_delta = _to_relative(abs_delta, val_score)
    rel_std = _to_relative(fold_std, val_score)
    overfitting_ok = rel_delta <= delta_thresh
    stability_ok = rel_std <= std_thresh
    violation = max(0.0, rel_delta - delta_thresh) + max(0.0, rel_std - std_thresh)
    return (overfitting_ok and stability_ok, abs_delta, fold_std, violation)


def compute_stability_focus(
    n_samples: int,
) -> Tuple[bool, float]:
    """Determine if we should bias tuning toward stability from dataset/training parameters.

    When n_samples is below stability_focus_sample_size_low (small data) or above
    stability_focus_sample_size_high (large temporal data), CV fold variance tends to be
    higher; returning focus=True and the configured stability_violation_weight lets the
    tuner narrow search bounds and penalize stability violations more. Works for any
    format and any algorithm.

    Returns:
        (stability_focus: bool, stability_violation_weight: float)
    """
    tuning = get_tuning_config()
    low = tuning["stability_focus_sample_size_low"]
    high = tuning["stability_focus_sample_size_high"]
    weight = tuning["stability_violation_weight"]
    focus = n_samples < low or n_samples > high
    return (focus, weight if focus else 1.0)


def compute_mlqa_penalized_score(
    pipe: Pipeline,
    X: np.ndarray,
    y: np.ndarray,
    scoring: str,
    mean_score: float,
    fold_std: float,
    stability_violation_weight: Optional[float] = None,
    train_score: Optional[float] = None,
) -> Tuple[bool, float, float]:
    """Compute MLQA pass, violation, and penalized score from existing CV mean and fold std.

    Used by Optuna objectives to avoid duplicating MLQA logic and to avoid running
    cross_val_score twice. Caller must have already run cross_val_score to get mean_score
    and fold_std. Returns (pass_audit, violation, penalized_score) where penalized_score
    is mean_score if pass_audit else mean_score - violation. On config or fit error
    returns (False, inf, -inf).

    When stability_violation_weight is provided and > 1, the stability part of the
    violation is weighted so that Optuna more strongly prefers trials with lower
    CV fold std (data-driven stability focus, applies to all algorithms).

    When train_score is provided, skip the expensive clone+fit+score step and use
    the caller-supplied value directly (performance optimization for Optuna trials).
    """
    from sklearn.base import clone
    from sklearn.metrics import get_scorer

    delta_thresh, std_thresh = _mlqa_overfitting_and_stability_thresholds()
    stab_weight = max(1.0, stability_violation_weight or 1.0)
    try:
        if train_score is not None:
            train_score_val = float(train_score)
        else:
            pipe_fit = clone(pipe)
            pipe_fit.fit(X, y)
            scorer = get_scorer(scoring)
            train_score_val = scorer(pipe_fit, X, y)
        abs_delta = abs(float(train_score_val) - mean_score)
        # Use relative metrics: fraction of score magnitude
        rel_delta = _to_relative(abs_delta, mean_score)
        rel_std = _to_relative(fold_std, mean_score)
        pass_audit = rel_delta <= delta_thresh and rel_std <= std_thresh
        delta_excess = max(0.0, rel_delta - delta_thresh)
        std_excess = max(0.0, rel_std - std_thresh)
        violation = delta_excess + stab_weight * std_excess
        penalized = mean_score if pass_audit else mean_score - violation
        return (pass_audit, violation, penalized)
    except Exception:
        return (False, float("inf"), float("-inf"))


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

        mlqa = get_mlqa_config()
        delta_thresh = mlqa["overfitting_delta_threshold"]
        std_thresh = mlqa["stability_fold_std_threshold"]
        dip_low = mlqa["bias_dip_low"]
        dip_high = mlqa["bias_dip_high"]
        top_weight_thresh = mlqa["sensitivity_top_weight_threshold"]

        # 1. Overfitting: train vs val delta (relative to score magnitude)
        pipe_fit = clone(pipe)
        pipe_fit.fit(X, y)
        scorer = get_scorer(scoring)
        train_score_val = scorer(pipe_fit, X, y)
        abs_delta = abs(float(train_score_val) - float(val_score))
        rel_delta = _to_relative(abs_delta, val_score)
        overfitting_risk = rel_delta > delta_thresh
        if overfitting_risk:
            findings.append(
                f"High Overfitting Risk: Train–Validation Δ = {abs_delta:.4f} "
                f"(relative {rel_delta:.2%} > {delta_thresh:.0%})."
            )
            status_flags.append("overfitting")
        else:
            findings.append(
                f"Overfitting check OK: Δ = {abs_delta:.4f} (relative {rel_delta:.2%} ≤ {delta_thresh:.0%})."
            )

        # 2. Stability: CV fold std (relative to score magnitude)
        fold_scores = cross_val_score(pipe, X, y, cv=cv, scoring=scoring)
        fold_std = float(np.std(fold_scores))
        rel_std = _to_relative(fold_std, val_score)
        unstable = rel_std > std_thresh
        if unstable:
            findings.append(
                f"Unstable: CV fold σ = {fold_std:.4f} (relative {rel_std:.2%} > {std_thresh:.0%})."
            )
            status_flags.append("unstable")
        else:
            findings.append(f"Stability OK: CV fold σ = {fold_std:.4f} (relative {rel_std:.2%}).")

        # 3. Bias & Fairness
        fairness = report.get("fairness_metrics") or {}
        dip = fairness.get("disparate_impact_ratio")
        if dip is not None:
            biased = dip < dip_low or dip > dip_high
            if biased:
                findings.append(f"Biased Model: disparate_impact_ratio = {dip:.4f} outside [{dip_low}, {dip_high}].")
                status_flags.append("biased")
                bias_report = "Model shows disparate impact; review protected group treatment before deployment."
            else:
                findings.append(f"Fairness OK: disparate_impact_ratio = {dip:.4f} in [{dip_low}, {dip_high}].")
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
                if top_weight > top_weight_thresh:
                    findings.append(
                        f"Potential Data Leakage / Low Robustness: top feature '{top_name}' = {top_weight * 100:.1f}%."
                    )
                    status_flags.append("sensitivity")
                else:
                    findings.append(
                        f"Sensitivity OK: top feature weight = {top_weight * 100:.1f}% ≤ {top_weight_thresh * 100:.0f}%."
                    )
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
                "overfitting": {
                    "delta": round(abs_delta, 4),
                    "relative_delta": round(rel_delta, 4),
                    "threshold": delta_thresh,
                    "flagged": overfitting_risk,
                },
                "stability": {
                    "cv_std": round(fold_std, 4),
                    "relative_cv_std": round(rel_std, 4),
                    "threshold": std_thresh,
                    "flagged": unstable,
                    "cv_fold_scores": [round(float(s), 4) for s in fold_scores],
                },
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


def _add_final_report_details(
    report: Dict[str, Any],
    pipe: Pipeline,
    X: np.ndarray,
    y: np.ndarray,
    cv: Any,
    scoring: str,
    task_type: str,
    model_kind: str,
) -> None:
    """Computes and adds MLQA audit and feature importance to the report."""
    report["mlqa_audit"] = _compute_mlqa_audit(
        report, pipe, X, y, cv, scoring, task_type, _mlqa_feature_names(model_kind)
    )
    feature_names = _mlqa_feature_names(model_kind)
    n_features = X.shape[1]
    fi = _extract_feature_importance(pipe, feature_names, n_features)
    if fi:
        report["feature_importance"] = fi
    else:
        # For MLP and other models without feature_importances_, use SHAP
        try:
            from ml.shap_explanations import compute_shap_importance

            shap_fi = compute_shap_importance(
                pipe,
                X,
                feature_names=feature_names,
                task_type=task_type,
                max_background=100,
                max_eval=300,
            )
            if shap_fi:
                report["feature_importance"] = shap_fi
                report["explainer"] = "shap"
        except Exception as e:
            logger.debug("SHAP importance skipped: %s", e)
