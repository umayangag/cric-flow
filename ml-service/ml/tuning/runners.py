"""The auto-tune runner: search the win model's hyperparameters, two-phase.

The regression runners -- batting, bowling, fielding, extras, innings -- went with their
trainers in P-5, and with them the shared multi-output scoring. What is left
tunes one classifier, and P-6 removes the stack entirely.
"""

from __future__ import annotations

import json
import logging
import os
import shutil
from typing import Any, Dict, List, Optional, Tuple

import joblib
import numpy as np
from sklearn.ensemble import (
    ExtraTreesClassifier,
    GradientBoostingClassifier,
    HistGradientBoostingClassifier,
    RandomForestClassifier,
)
from sklearn.model_selection import RandomizedSearchCV, cross_val_score
from sklearn.neural_network import MLPClassifier
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from ml.config import (
    get_training_params,
    get_tuning_config,
)

try:
    from ml import auto_tune_progress as _progress
except ImportError:
    _progress = None

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

from ml.tuning.cv_metrics import (
    _add_final_report_details,
    _compute_metrics_classification,
    _effective_n_jobs,
    _get_cv_object,
)
from ml.tuning.optuna_search import (
    _count_combinations,
    _run_search_classification,
    _save_artifacts_model_only,
)
from ml.tuning.search_space import (
    _get_prior_tuned_algorithm,
    _phase1_candidates_classification,
    _prior_params_to_optuna_params,
)
from ml.tuning.types import (
    PHASE1_TRIALS_PER_ALGORITHM,
    PHASE2_TRIALS,
)

logger = logging.getLogger(__name__)


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
    scoring: Optional[str] = None,
) -> Tuple[bool, Optional[Any], Dict[str, Any]]:
    """Run AutoGluon when enabled; compare to Optuna score. Returns (autogluon_wins, wrapper_or_none, report_updates).

    ``scoring`` is the metric the Optuna search optimised. It is passed to AutoGluon so
    both numbers measure the same thing -- the comparison below is a plain ``>``, and
    two different metrics either side of it is a category error, not a close call.
    """

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
            X, y, time_limit_seconds=time_limit, presets=presets, eval_metric=scoring or "roc_auc"
        )

    ag_better = ok and ag_score is not None and ag_score > optuna_best_score
    if not ok or not ag_better or not persist_path:
        return False, None, {"autogluon_tried": True, "autogluon_score": ag_score, "autogluon_wins": False}

    fmt = format_suffix.replace(" ", "_")
    ag_dir = os.path.join(out_dir, f"autogluon_{model_kind}_{fmt}")
    os.makedirs(out_dir, exist_ok=True)
    if os.path.isdir(ag_dir):
        shutil.rmtree(ag_dir)
    shutil.copytree(persist_path, ag_dir)

    wrapper = AutogluonPredictorWrapper(ag_dir, is_regression=(task_type == "regression"))
    return True, wrapper, {"autogluon_tried": True, "autogluon_score": ag_score, "autogluon_wins": True}


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
    feature_names_for_report: Optional[List[str]] = None,
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
    gap = max(0, int(tuning_cfg.get("timeseries_split_gap", 0) or 0))
    small_thresh = int(tuning_cfg.get("timeseries_small_dataset_threshold", 5000) or 5000)
    cv = _get_cv_object(
        validation_method, cv_splits, X.shape[0], random_state, gap=gap, small_dataset_threshold=small_thresh
    )
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
            feature_names_for_report=feature_names_for_report,
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
            _add_final_report_details(
                report, best_pipe, X, y, cv, scoring, "classification", model_kind, feature_names_for_report
            )
        return best_pipe, best_params, report

    def _obj(trial: Any) -> float:
        alg = trial.suggest_categorical("algorithm", winners)
        if alg == "rf":
            est = RandomForestClassifier(
                n_estimators=trial.suggest_int("n_estimators", 50, 600, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
                random_state=random_state,
            )
        elif alg == "gb":
            est = GradientBoostingClassifier(
                n_estimators=trial.suggest_int("n_estimators", 50, 600, step=50),
                max_depth=trial.suggest_int("max_depth", 3, 20, step=1),
                learning_rate=trial.suggest_float("learning_rate", 0.01, 0.2, log=True),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
                random_state=random_state,
            )
        elif alg == "et":
            est = ExtraTreesClassifier(
                n_estimators=trial.suggest_int("n_estimators", 50, 600, step=50),
                max_depth=trial.suggest_int("max_depth", 4, 24, step=2),
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
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
                min_samples_leaf=trial.suggest_int("min_samples_leaf", 4, 24),
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
        _add_final_report_details(
            report, best_pipe, X, y, cv, scoring, "classification", model_kind, feature_names_for_report
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
    algorithms_explicitly_passed: bool = False,
) -> Dict[str, Any]:
    """Run two-phase classification search for win; save model only + report."""
    tuning = _get_tuning_config()
    cv_splits = tuning["cv_splits"]
    n_iter = int(tuning["n_iter"])
    if fast_mode:
        n_iter = min(n_iter, 15)
    # Ranking, not accuracy. Team selection takes an argmax over candidate XIs, so only
    # the order the model puts them in can change which side is picked -- a threshold
    # metric is blind to every improvement that does not cross 0.5, and rewards leaning
    # on the majority outcome. Tuning for accuracy can therefore buy a worse selector.
    #
    # ml.tuning.scoring is not consulted: it holds a regression metric for the other
    # models, and silently applying neg_mean_absolute_error to a classifier would be
    # worse than ignoring it.
    scoring = "roc_auc"
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
            prior_params = _prior_params_to_optuna_params(prior_algo, prior_cfg)
        logger.info(
            "auto_tune.using_prior_algorithm model=win format=%s algorithm=%s (fine-tune only)",
            format_suffix,
            prior_algo,
        )
    if use_pycaret is not False and prior is None and not algorithms_explicitly_passed:
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
            X, y, "classification", use_autogluon, "win", format_suffix, out_dir, float(optuna_score), scoring
        )
        report.update(ag_updates)
        if ag_wins and ag_wrapper is not None:
            os.makedirs(out_dir, exist_ok=True)
            model_path = os.path.join(
                out_dir,
                f"win_model_{format_suffix.replace(' ', '_')}.joblib",
            )
            joblib.dump(ag_wrapper, model_path, compress=joblib_compress)
            report_path = os.path.join(out_dir, f"tuning_report_win_{format_suffix}.json")
            with open(report_path, "w", encoding="utf-8") as f:
                json.dump(report, f, indent=2)
            return report
    _save_artifacts_model_only(best_pipe, out_dir, "win", format_suffix, joblib_compress, report)
    return report
