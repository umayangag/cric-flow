"""Two-phase Optuna/RandomizedSearchCV search runners for regression and classification.

AGENTS: Changes here must apply uniformly to all algorithms (rf, gb, quantile, et, hgb, mlp, …)
and all formats (TEST, ODI, T20, T20I). Do not add algorithm-specific or format-specific
branches without applying the same behaviour elsewhere. See AGENTS_AUTO_TUNE.md in this package.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Callable, Dict, List, Optional, Tuple

import joblib
import numpy as np
from sklearn.ensemble import (
    ExtraTreesRegressor,
    GradientBoostingRegressor,
    HistGradientBoostingRegressor,
    RandomForestRegressor,
)
from sklearn.model_selection import RandomizedSearchCV, cross_val_score
from sklearn.neural_network import MLPRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from ml.config import get_training_params, get_tuning_config
from ml.tuning.cv_metrics import (
    _add_final_report_details,
    _compute_metrics_classification,
    _compute_metrics_regression,
    _effective_n_jobs,
    _get_cv_object,
    compute_mlqa_overfitting_stability,
    compute_mlqa_penalized_score,
    compute_stability_focus,
)
from ml.tuning.search_space import (
    _build_pipeline,
    _build_pipeline_single_regression,
    _phase1_candidates_regression,
    _phase1_candidates_regression_single,
    _search_space_classification,
    _search_space_regression,
    _search_space_regression_single,
)
from ml.tuning.types import (
    PHASE1_TRIALS_PER_ALGORITHM,
    PHASE2_TRIALS,
)
from ml.tuning.types import (
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


def _get_phase2_bounds(tuning_cfg: Dict[str, Any], stability_focus: bool) -> Dict[str, Any]:
    """Return Phase 2 suggest bounds from config (fully populated by get_tuning_config)."""
    bounds = tuning_cfg["stability_focus_bounds"] if stability_focus else tuning_cfg["default_bounds"]
    out = dict(bounds)
    # Normalize mlp_hidden_layer_sizes to list of tuples in case of JSON list-of-lists
    v = out.get("mlp_hidden_layer_sizes")
    if isinstance(v, (list, tuple)) and v:
        out["mlp_hidden_layer_sizes"] = [tuple(x) for x in v if isinstance(x, (list, tuple))]
    return out


def _suggest_phase2_regression_estimator(
    trial: Any,
    alg: str,
    bounds: Dict[str, Any],
    random_state: int,
    model_kind: Optional[str] = None,
    tuning_cfg: Optional[Dict[str, Any]] = None,
) -> Optional[Any]:
    """Suggest hyperparameters and return a configured regression estimator for Phase 2 Optuna.

    Shared by _run_search_two_phase_single_regression (_obj) and _run_search_two_phase (_optuna_objective).
    Handles rf, gb, et, hgb, mlp; and quantile when model_kind is set. Returns None for unknown alg.
    When tuning_cfg is provided, quantile fallback params come from tuning_cfg.quantile_fallback.
    """
    leaf_min = bounds["min_samples_leaf_min"]
    leaf_high = bounds["min_samples_leaf_max"]
    lr_high = bounds["learning_rate_max"]
    n_est_min = bounds["n_estimators_min"]
    mlp_alpha_min = bounds["mlp_alpha_min"]
    mlp_sizes = bounds["mlp_hidden_layer_sizes"]

    if alg == "rf":
        return RandomForestRegressor(
            n_estimators=trial.suggest_int("n_estimators", n_est_min, 600, step=50),
            max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
            min_samples_leaf=trial.suggest_int("min_samples_leaf", leaf_min, leaf_high),
            random_state=random_state,
        )
    if alg == "gb":
        return GradientBoostingRegressor(
            n_estimators=trial.suggest_int("n_estimators", n_est_min, 600, step=50),
            max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
            learning_rate=trial.suggest_float("learning_rate", 0.01, lr_high, log=True),
            min_samples_leaf=trial.suggest_int("min_samples_leaf", leaf_min, leaf_high),
            random_state=random_state,
        )
    if alg == "quantile" and model_kind is not None:
        try:
            tp = get_training_params(model_kind)
            return GradientBoostingRegressor(
                n_estimators=trial.suggest_int("n_estimators", n_est_min, 600, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 20, step=2),
                learning_rate=trial.suggest_float("learning_rate", 0.01, lr_high, log=True),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", leaf_min, leaf_high),
                random_state=random_state,
                loss="quantile",
                alpha=tp.get("quantile_level", 0.5),
            )
        except (ValueError, KeyError):
            fallback = (tuning_cfg or {}).get("quantile_fallback") or {}
            n_est = int(fallback.get("n_estimators", 200))
            depth = int(fallback.get("max_depth", 12))
            alpha = float(fallback.get("alpha", 0.5))
            return GradientBoostingRegressor(
                n_estimators=n_est,
                max_depth=depth,
                random_state=random_state,
                loss="quantile",
                alpha=alpha,
            )
    if alg == "et":
        return ExtraTreesRegressor(
            n_estimators=trial.suggest_int("n_estimators", n_est_min, 600, step=50),
            max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
            min_samples_leaf=trial.suggest_int("min_samples_leaf", leaf_min, leaf_high),
            random_state=random_state,
        )
    if alg == "hgb":
        return HistGradientBoostingRegressor(
            max_iter=trial.suggest_int("max_iter", n_est_min, 400, step=50),
            max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
            learning_rate=trial.suggest_float("learning_rate", 0.01, lr_high, log=True),
            min_samples_leaf=trial.suggest_int("min_samples_leaf", leaf_min, leaf_high),
            random_state=random_state,
        )
    if alg == "mlp":
        sizes = trial.suggest_categorical("hidden_layer_sizes", mlp_sizes)
        alpha = trial.suggest_float("alpha", mlp_alpha_min, 1e-1, log=True)
        lr_init = trial.suggest_float("learning_rate_init", 1e-4, 1e-1, log=True)
        return MLPRegressor(
            hidden_layer_sizes=sizes,
            alpha=alpha,
            learning_rate_init=lr_init,
            max_iter=1000,
            early_stopping=True,
            random_state=random_state,
        )
    return None


def _setup_phase2_stability_focus(
    n_samples: int,
    cv_splits: int,
    validation_method: str,
    tuning_cfg: Dict[str, Any],
    prior_params: Optional[Dict[str, Any]],
    winners: List[str],
    model_kind: str,
) -> Tuple[bool, float, Dict[str, Any]]:
    """Determine stability focus and prepare Phase 2 search bounds (shared by single and two-phase runners)."""
    stability_focus, stability_weight = compute_stability_focus(n_samples)
    bounds = _get_phase2_bounds(tuning_cfg, stability_focus)
    mlp_sizes = list(bounds.get("mlp_hidden_layer_sizes", []))

    def _add_mlp_size(s: Any) -> None:
        if s is None:
            return
        t = tuple(s) if isinstance(s, (list, tuple)) else s
        if t not in mlp_sizes:
            mlp_sizes.append(t)

    if prior_params and prior_params.get("algorithm") == "mlp":
        _add_mlp_size(prior_params.get("hidden_layer_sizes"))
    if stability_focus:
        for alg in winners:
            seed = _stability_seed_trial_params(alg, model_kind, tuning_cfg)
            if seed and alg == "mlp":
                _add_mlp_size(seed.get("hidden_layer_sizes"))
            if seed is not None:
                break

    final_bounds = dict(bounds)
    final_bounds["mlp_hidden_layer_sizes"] = mlp_sizes
    return stability_focus, stability_weight, final_bounds


def _run_phase2_optuna_study(
    objective: Callable[[Any], float],
    n_phase2: int,
    random_state: int,
    prior_params: Optional[Dict[str, Any]],
    stability_focus: bool,
    winners: List[str],
    model_kind: str,
    tuning_cfg: Dict[str, Any],
    n_samples: int,
    callback: Callable[[Any, Any], None],
) -> Any:
    """Run Phase 2 Optuna study: create study, enqueue prior/stability trials, optimize.
    Shared by _run_search_two_phase_single_regression and _run_search_two_phase.
    """
    study = optuna.create_study(
        direction="maximize", sampler=optuna.samplers.TPESampler(seed=random_state, n_startup_trials=5)
    )
    if prior_params:
        try:
            study.enqueue_trial(prior_params)
        except Exception as e:
            logger.debug("auto_tune.enqueue_prior_trial_skipped error=%s", e)
    if stability_focus:
        for alg in winners:
            seed = _stability_seed_trial_params(alg, model_kind, tuning_cfg)
            if seed is not None:
                try:
                    study.enqueue_trial(seed)
                    logger.info(
                        "auto_tune.stability_seed enqueued %s trial (n_samples=%s) for MLQA stability",
                        alg,
                        n_samples,
                    )
                except Exception as e:
                    logger.debug("auto_tune.enqueue_stability_seed_skipped error=%s", e)
                break
    study.optimize(objective, n_trials=n_phase2, n_jobs=1, show_progress_bar=False, callbacks=[callback])
    return study


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
            std_test_score = float(search.cv_results_["std_test_score"][search.best_index_])
            results.append((key, name, float(search.best_score_), std_test_score, best_params, search.best_estimator_))
        # Rank by: pass first, then lowest violation (lowest overfitting + fold σ), then best score.
        enriched_single: List[Tuple[bool, float, float, str, str, Dict[str, Any], Pipeline]] = []
        for key, name, score, fold_std, params, pipe in results:
            try:
                pass_audit, _, _, violation = compute_mlqa_overfitting_stability(
                    pipe, X, y, cv, scoring, score, fold_std_override=fold_std
                )
            except Exception as e:
                logger.debug("auto_tune.phase1_mlqa_skip algorithm=%s error=%s", key, e)
                pass_audit = False
                violation = float("inf")
            enriched_single.append((pass_audit, violation, score, key, name, params, pipe))
        enriched_single.sort(key=lambda x: (not x[0], x[1], -x[2]))
        best_key = enriched_single[0][3]
        best_name = enriched_single[0][4]
        best_score = enriched_single[0][2]
        best_params = enriched_single[0][5]
        best_pipe = enriched_single[0][6]
        winners = [enriched_single[0][3]] + ([enriched_single[1][3]] if len(enriched_single) > 1 else [])
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

    n_phase2 = min(n_iter, PHASE2_TRIALS)
    stability_focus, stability_weight, bounds = _setup_phase2_stability_focus(
        X.shape[0], cv_splits, validation_method, tuning_cfg, prior_params, winners, model_kind
    )

    def _obj(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        est = _suggest_phase2_regression_estimator(trial, alg, bounds, random_state, tuning_cfg=tuning_cfg)
        if est is None:
            raise ValueError(f"unsupported algorithm for Phase 2 single regression: {alg}")
        pipe = _build_pipeline_single_regression(est)
        scores = cross_val_score(pipe, X, y, cv=cv, scoring=scoring, n_jobs=n_jobs)
        mean_score = float(scores.mean())
        fold_std = float(np.std(scores))
        pass_audit, _violation, penalized_score = compute_mlqa_penalized_score(
            pipe,
            X,
            y,
            scoring,
            mean_score,
            fold_std,
            stability_violation_weight=stability_weight,
        )
        trial.set_user_attr("mean_cv_score", mean_score)
        return penalized_score

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

    study = _run_phase2_optuna_study(
        _obj,
        n_phase2,
        random_state,
        prior_params,
        stability_focus,
        winners,
        model_kind,
        tuning_cfg,
        X.shape[0],
        _cb,
    )
    if study.best_trial:
        p = study.best_params
        best_score = float(study.best_trial.user_attrs.get("mean_cv_score", study.best_value))
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


def _stability_seed_trial_params(
    algorithm: str,
    model_kind: str,
    tuning_cfg: Dict[str, Any],
) -> Optional[Dict[str, Any]]:
    """Return a single stability-oriented trial params dict from config, or None.

    Uses ml.tuning.stability_seed_params[algorithm]; no hardcoded params. For quantile,
    alpha (quantile_level) is taken from ml.training.<model_kind>.
    """
    seeds = tuning_cfg.get("stability_seed_params") or {}
    raw = seeds.get(algorithm) if isinstance(seeds, dict) else None
    if not isinstance(raw, dict) or not raw:
        return None
    out = dict(raw)
    out["algorithm"] = algorithm
    if algorithm == "quantile":
        try:
            get_training_params(model_kind)  # ensure model has training config
        except (ValueError, KeyError):
            return None
        # alpha (quantile_level) is not a trial param; objective gets it from get_training_params
    if algorithm == "mlp" and "hidden_layer_sizes" in out:
        h = out["hidden_layer_sizes"]
        out["hidden_layer_sizes"] = tuple(h) if isinstance(h, (list, tuple)) else h
    return out


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
    enriched: List[Tuple[bool, float, float, str, str, Dict[str, Any], Pipeline]] = []
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
            std_test_score = float(search.cv_results_["std_test_score"][search.best_index_])
            results.append((key, name, float(search.best_score_), std_test_score, best_params, search.best_estimator_))
        # Rank by: pass first, then lowest violation (lowest overfitting + fold σ), then best CV score.
        enriched: List[Tuple[bool, float, float, str, str, Dict[str, Any], Pipeline]] = []
        for key, name, score, fold_std, params, pipe in results:
            try:
                pass_audit, _delta, _fold_std, violation = compute_mlqa_overfitting_stability(
                    pipe, X, Y, cv, scoring, score, fold_std_override=fold_std
                )
            except Exception as e:
                logger.debug("auto_tune.phase1_mlqa_skip algorithm=%s error=%s", key, e)
                pass_audit = False
                violation = float("inf")
            enriched.append((pass_audit, violation, score, key, name, params, pipe))
        enriched.sort(key=lambda x: (not x[0], x[1], -x[2]))  # pass first, then lowest violation, then score
        best_key = enriched[0][3]
        best_name = enriched[0][4]
        best_score = enriched[0][2]
        best_params = enriched[0][5]
        best_pipe = enriched[0][6]
        winners = [enriched[0][3]] + ([enriched[1][3]] if len(enriched) > 1 else [])

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

    # Data-driven stability: when sample size suggests higher CV fold variance,
    # narrow bounds and optionally weight stability higher in the penalized objective.
    stability_focus, stability_weight, bounds = _setup_phase2_stability_focus(
        X.shape[0], cv_splits, validation_method, tuning_cfg, prior_params, winners, model_kind
    )

    def _optuna_objective(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        est = _suggest_phase2_regression_estimator(
            trial, alg, bounds, random_state, model_kind=model_kind, tuning_cfg=tuning_cfg
        )
        if est is None:
            raise ValueError(f"Algorithm '{alg}' is not supported for Phase 2 fine-tuning.")
        pipe = _build_pipeline(est)
        scores = cross_val_score(pipe, X, Y, cv=cv, scoring=scoring, n_jobs=n_jobs)
        mean_score = float(scores.mean())
        fold_std = float(np.std(scores))
        _pass_audit, _violation, penalized_score = compute_mlqa_penalized_score(
            pipe,
            X,
            Y,
            scoring,
            mean_score,
            fold_std,
            stability_violation_weight=stability_weight,
        )
        trial.set_user_attr("mean_cv_score", mean_score)
        return penalized_score

    study = _run_phase2_optuna_study(
        _optuna_objective,
        n_phase2,
        random_state,
        prior_params,
        stability_focus,
        winners,
        model_kind,
        tuning_cfg,
        X.shape[0],
        _progress_callback,
    )

    if study.best_trial:
        params = study.best_params
        # Use actual CV mean for report, not the composite score (which may be penalized).
        best_score = float(study.best_trial.user_attrs.get("mean_cv_score", study.best_value))
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
        best_params = {"est__estimator__" + k: v for k, v in params.items()}
    else:
        # Keep Phase 1 choice when no Optuna best: MLQA-best (lowest violation) if we ranked, else score-best.
        best_params = enriched[0][5] if enriched else results[0][3]

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
