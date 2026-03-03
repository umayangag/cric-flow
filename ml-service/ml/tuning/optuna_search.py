"""Two-phase Optuna/RandomizedSearchCV search runners for regression and classification."""

from __future__ import annotations

import json
import logging
import os
import time
from typing import Any, Dict, List, Optional, Tuple

import joblib
import numpy as np
from sklearn.ensemble import (
    ExtraTreesRegressor,
    GradientBoostingRegressor,
    HistGradientBoostingRegressor,
    RandomForestRegressor,
    StackingRegressor,
)
from sklearn.linear_model import Ridge
from sklearn.model_selection import RandomizedSearchCV, cross_val_score
from sklearn.multioutput import MultiOutputRegressor
from sklearn.neural_network import MLPRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from ml.config import get_tuning_config
from ml.tuning.cv_metrics import (
    _add_final_report_details,
    _compute_metrics_classification,
    _compute_metrics_regression,
    _effective_n_jobs,
    _effective_timeseries_gap,
    _get_cv_object,
)
from ml.tuning.search_space import (
    _build_pipeline,
    _build_pipeline_single_regression,
    _normalize_hidden_layer_sizes,
    _phase1_candidates_classification,
    _phase1_candidates_regression,
    _phase1_candidates_regression_single,
    _search_space_classification,
    _search_space_regression,
    _search_space_regression_single,
    _to_pipeline_params,
    _to_pipeline_params_single,
)
from ml.tuning.types import (
    AVAILABLE_ALGORITHMS,
    PHASE1_TRIALS_PER_ALGORITHM,
    PHASE2_TRIALS,
    target_names_for_model as _target_names_for_model,
)

try:
    import optuna

    _HAS_OPTUNA = True
except ImportError:
    _HAS_OPTUNA = False

try:
    from ml import auto_tune_progress as _progress
except ImportError:
    _progress = None

logger = logging.getLogger(__name__)

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
    gap = max(0, int(tuning_cfg.get("timeseries_split_gap", 0) or 0))
    small_thresh = int(tuning_cfg.get("timeseries_small_dataset_threshold", 5000) or 5000)
    cv = _get_cv_object(
        validation_method, cv_splits, X.shape[0], random_state, gap=gap, small_dataset_threshold=small_thresh
    )
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
            report["metrics"] = _compute_metrics_regression(
                best_pipe, X, y, cv, target_names=_target_names_for_model(model_kind)
            )
            _add_final_report_details(report, best_pipe, X, y, cv, scoring, "regression", model_kind)
        return best_pipe, best_params, report

    def _obj(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        if alg == "rf":
            est = RandomForestRegressor(
                n_estimators=trial.suggest_int("n_estimators", 50, 600, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
                random_state=random_state,
            )
        elif alg == "gb":
            est = GradientBoostingRegressor(
                n_estimators=trial.suggest_int("n_estimators", 50, 600, step=50),
                max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
                learning_rate=trial.suggest_float("learning_rate", 0.01, 0.2, log=True),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
                random_state=random_state,
            )
        elif alg == "et":
            est = ExtraTreesRegressor(
                n_estimators=trial.suggest_int("n_estimators", 50, 600, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
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
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
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
        report["metrics"] = _compute_metrics_regression(
            best_pipe, X, y, cv, target_names=_target_names_for_model(model_kind)
        )
        _add_final_report_details(report, best_pipe, X, y, cv, scoring, "regression", model_kind)
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
    gap = max(0, int(tuning_cfg.get("timeseries_split_gap", 0) or 0))
    small_thresh = int(tuning_cfg.get("timeseries_small_dataset_threshold", 5000) or 5000)
    cv = _get_cv_object(
        validation_method, cv_splits, X.shape[0], random_state, gap=gap, small_dataset_threshold=small_thresh
    )
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
        report["metrics"] = _compute_metrics_regression(
            best_pipe, X, y, cv, target_names=_target_names_for_model(model_kind)
        )
        _add_final_report_details(report, best_pipe, X, y, cv, scoring, "regression", model_kind)
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
    gap = max(0, int(tuning_cfg.get("timeseries_split_gap", 0) or 0))
    small_thresh = int(tuning_cfg.get("timeseries_small_dataset_threshold", 5000) or 5000)
    cv = _get_cv_object(
        validation_method, cv_splits, X.shape[0], random_state, gap=gap, small_dataset_threshold=small_thresh
    )
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
        _add_final_report_details(report, best_pipe, X, y, cv, scoring, "classification", model_kind)
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
    algorithms_requested: Optional[List[str]] = None,
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
    gap = max(0, int(tuning_cfg.get("timeseries_split_gap", 0) or 0))
    small_thresh = int(tuning_cfg.get("timeseries_small_dataset_threshold", 5000) or 5000)
    cv = _get_cv_object(
        validation_method, cv_splits, X.shape[0], random_state, gap=gap, small_dataset_threshold=small_thresh
    )
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
            algorithms_requested=algorithms_requested,
            activity="initializing",
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
            algorithms_requested=algorithms_requested,
            activity="screening",
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
                algorithms_requested=algorithms_requested,
                activity="cross_validating",
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
                algorithms_screened=[c[0] for c in candidates],
                algorithms_requested=algorithms_requested,
                activity="screening_done",
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
            report["metrics"] = _compute_metrics_regression(
                best_pipe, X, Y, cv, target_names=_target_names_for_model(model_kind)
            )
            _add_final_report_details(report, best_pipe, X, Y, cv, scoring, "regression", model_kind)
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
            algorithms_requested=algorithms_requested,
            activity="running_trial",
        )

    def _optuna_objective(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        if alg == "rf":
            n_est = trial.suggest_int("n_estimators", 50, 600, step=50)
            depth = trial.suggest_int("max_depth", 4, 24, step=2)
            leaf = trial.suggest_int("min_samples_leaf", 4, 24)
            est = RandomForestRegressor(
                n_estimators=n_est, max_depth=depth, min_samples_leaf=leaf, random_state=random_state
            )
        elif alg == "gb":
            n_est = trial.suggest_int("n_estimators", 50, 600, step=50)
            depth = trial.suggest_int("max_depth", 3, 20, step=1)
            lr = trial.suggest_float("learning_rate", 0.01, 0.2, log=True)
            leaf = trial.suggest_int("min_samples_leaf", 4, 24)
            est = GradientBoostingRegressor(
                n_estimators=n_est, max_depth=depth, learning_rate=lr, min_samples_leaf=leaf, random_state=random_state
            )
        elif alg == "quantile":
            try:
                tp = get_training_params(model_kind)
                n_est = trial.suggest_int("n_estimators", 50, 600, step=50)
                depth = trial.suggest_int("max_depth", 4, 20, step=2)
                lr = trial.suggest_float("learning_rate", 0.01, 0.2, log=True)
                leaf = trial.suggest_int("min_samples_leaf", 4, 24)
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
            n_est = trial.suggest_int("n_estimators", 50, 600, step=50)
            depth = trial.suggest_int("max_depth", 4, 24, step=2)
            leaf = trial.suggest_int("min_samples_leaf", 4, 24)
            est = ExtraTreesRegressor(
                n_estimators=n_est, max_depth=depth, min_samples_leaf=leaf, random_state=random_state
            )
        elif alg == "hgb":
            n_est = trial.suggest_int("max_iter", 50, 400, step=50)
            depth = trial.suggest_int("max_depth", 3, 14, step=1)
            lr = trial.suggest_float("learning_rate", 0.01, 0.2, log=True)
            leaf = trial.suggest_int("min_samples_leaf", 4, 24)
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
        report["metrics"] = _compute_metrics_regression(
            best_pipe, X, Y, cv, target_names=_target_names_for_model(model_kind)
        )
        _add_final_report_details(report, best_pipe, X, Y, cv, scoring, "regression", model_kind)
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
    gap = max(0, int(tuning_cfg.get("timeseries_split_gap", 0) or 0))
    small_thresh = int(tuning_cfg.get("timeseries_small_dataset_threshold", 5000) or 5000)
    cv = _get_cv_object(
        validation_method, cv_splits, X.shape[0], random_state, gap=gap, small_dataset_threshold=small_thresh
    )
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
        report["metrics"] = _compute_metrics_regression(
            best_pipe, X, Y, cv, target_names=_target_names_for_model(model_kind)
        )
        _add_final_report_details(report, best_pipe, X, Y, cv, scoring, "regression", model_kind)
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
