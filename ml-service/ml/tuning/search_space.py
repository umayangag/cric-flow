"""Search space definitions, pipeline builders, and phase-1 candidate generators."""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Dict, List, Optional, Tuple

from sklearn.ensemble import (
    ExtraTreesClassifier,
    GradientBoostingClassifier,
    HistGradientBoostingClassifier,
    RandomForestClassifier,
)
from sklearn.neural_network import MLPClassifier

from ml.config import get_tuned_params_from_go_app, get_tuning_config, get_tuning_search_space
from ml.tuning.types import (
    _PHASE1_COARSE_ET,
    _PHASE1_COARSE_GB,
    _PHASE1_COARSE_HGB,
    _PHASE1_COARSE_MLP_CLF,
    _PHASE1_COARSE_RF,
    AVAILABLE_ALGORITHMS,
)

logger = logging.getLogger(__name__)


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
                if algo in AVAILABLE_ALGORITHMS:
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


def _normalize_hidden_layer_sizes(v: Any) -> Optional[Tuple[int, ...]]:
    """Convert JSON-serialized hidden_layer_sizes to tuple for Optuna suggest_categorical.

    JSON/API/store may return [64, 64] (list) or '[64, 64]' (string); Optuna requires (64, 64) (tuple)
    or suggest_categorical raises ValueError. Returns None if value cannot be normalized.
    """
    if v is None:
        return None
    if isinstance(v, tuple):
        return v
    if isinstance(v, list):
        try:
            return tuple(int(x) for x in v)
        except (TypeError, ValueError):
            return None
    if isinstance(v, str):
        try:
            parsed = json.loads(v)
            if isinstance(parsed, list):
                return tuple(int(x) for x in parsed)
        except (json.JSONDecodeError, TypeError, ValueError):
            pass
    return None


def _prior_params_to_optuna_params(algo: str, params: Dict[str, Any]) -> Dict[str, Any]:
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
            if key == "hidden_layer_sizes":
                normalized = _normalize_hidden_layer_sizes(p[key])
                if normalized is not None:
                    out[key] = normalized
            else:
                out[key] = p[key]
    return out


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


def _coarse_to_single_prefix(d: Dict[str, Any]) -> Dict[str, Any]:
    """Convert est__estimator__* to est__* for single-estimator pipelines."""
    return {k.replace("est__estimator__", "est__"): v for k, v in d.items()}


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
