"""Two-phase Optuna/RandomizedSearchCV search runners for regression and classification.

AGENTS: Changes here must apply uniformly to all algorithms (rf, gb, quantile, et, hgb, mlp, …)
and all formats (TEST, ODI, T20, T20I). Do not add algorithm-specific or format-specific
branches without applying the same behavior elsewhere. See AGENTS_AUTO_TUNE.md in this package.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Callable, Dict, List, Optional, Tuple

import joblib
import numpy as np
from sklearn.model_selection import RandomizedSearchCV
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from ml.config import get_training_params, get_tuning_config
from ml.tuning.cv_metrics import (
    _add_final_report_details,
    _compute_metrics_classification,
    _effective_n_jobs,
    _get_cv_object,
    compute_stability_focus,
)
from ml.tuning.search_space import (
    _normalize_hidden_layer_sizes,
    _search_space_classification,
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

# Algorithms that Phase 2 Optuna can fine-tune; others (e.g. stacked) are filtered from winners.
_PHASE2_TUNABLE_ALGORITHMS = frozenset({"rf", "gb", "quantile", "et", "hgb", "mlp"})


def _get_phase2_bounds(tuning_cfg: Dict[str, Any], stability_focus: bool) -> Dict[str, Any]:
    """Return Phase 2 suggest bounds from config (fully populated by get_tuning_config)."""
    bounds = tuning_cfg["stability_focus_bounds"] if stability_focus else tuning_cfg["default_bounds"]
    out = dict(bounds)
    # Normalize mlp_hidden_layer_sizes to list of tuples in case of JSON list-of-lists
    v = out.get("mlp_hidden_layer_sizes")
    if isinstance(v, (list, tuple)) and v:
        out["mlp_hidden_layer_sizes"] = [tuple(x) for x in v if isinstance(x, (list, tuple))]
    return out


def _append_unique_normalized_mlp_size(mlp_sizes: List[Any], raw_size: Any) -> None:
    """Append normalized hidden_layer_sizes to mlp_sizes if absent (Phase 2 stability / prior merge)."""
    normalized = _normalize_hidden_layer_sizes(raw_size)
    if normalized is not None and normalized not in mlp_sizes:
        mlp_sizes.append(normalized)


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

    if prior_params and prior_params.get("algorithm") == "mlp":
        _append_unique_normalized_mlp_size(mlp_sizes, prior_params.get("hidden_layer_sizes"))
    if stability_focus:
        for alg in winners:
            seed = _stability_seed_trial_params(alg, model_kind, tuning_cfg)
            if seed and alg == "mlp":
                _append_unique_normalized_mlp_size(mlp_sizes, seed.get("hidden_layer_sizes"))
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
        direction="maximize",
        sampler=optuna.samplers.TPESampler(
            seed=random_state, n_startup_trials=tuning_cfg.get("optuna_tpe_n_startup_trials", 5)
        ),
    )
    if prior_params:
        try:
            study.enqueue_trial(prior_params)
        except ValueError as e:
            logger.warning("auto_tune.enqueue_prior_trial_skipped error=%s", e, exc_info=True)
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
                except ValueError as e:
                    logger.warning("auto_tune.enqueue_stability_seed_skipped error=%s", e, exc_info=True)
                break
    study.optimize(objective, n_trials=n_phase2, n_jobs=1, show_progress_bar=False, callbacks=[callback])
    return study


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
    feature_names_for_report: Optional[List[str]] = None,
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
        _add_final_report_details(
            report, best_pipe, X, y, cv, scoring, "classification", model_kind, feature_names_for_report
        )
    return best_pipe, best_params, report


def _require_format_suffix(model_kind: str, format_suffix: Optional[str]) -> None:
    """Refuse to write an artifact with no format suffix.

    app.artifacts loads by the `<kind>_model_<FMT>` prefix only, so an unsuffixed
    file is one nothing can serve — a silent no-op that reads as a successful run.
    """
    if not format_suffix:
        raise ValueError(f"auto_tune.{model_kind}.missing_format_suffix: artifacts must be written per format")


def _save_artifacts_model_only(
    pipeline: Pipeline,
    out_dir: str,
    model_kind: str,
    format_suffix: Optional[str],
    joblib_compress: int,
    report: Dict[str, Any],
) -> None:
    """Save model only (no scaler) for extras/win; plus tuning report."""
    _require_format_suffix(model_kind, format_suffix)
    os.makedirs(out_dir, exist_ok=True)
    model = pipeline.named_steps["est"]
    model_path = os.path.join(out_dir, f"{model_kind}_model_{format_suffix}.joblib")
    report_path = os.path.join(out_dir, f"tuning_report_{model_kind}_{format_suffix}.json")
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
    # Treat missing, non-dict, or empty mapping as absent: an empty dict would not define useful hyperparams.
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
        normalized_size = _normalize_hidden_layer_sizes(out["hidden_layer_sizes"])
        if normalized_size is not None:
            out["hidden_layer_sizes"] = normalized_size
        else:
            del out["hidden_layer_sizes"]
    return out


def _count_combinations(param_dist: Dict[str, Any]) -> int:
    n = 1
    for v in param_dist.values():
        if hasattr(v, "__len__") and not isinstance(v, (str, bytes)):
            n *= len(v)
        else:
            n *= 1
    return n


# ---- Data loading (CSV) ----
