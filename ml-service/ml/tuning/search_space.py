"""Search space definitions, pipeline builders, and phase-1 candidate generators."""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
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
from sklearn.multioutput import MultiOutputRegressor
from sklearn.neural_network import MLPClassifier, MLPRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from ml.config import get_training_params, get_tuned_params_from_go_app, get_tuning_config, get_tuning_search_space
from ml.tuning.types import (
    AVAILABLE_ALGORITHMS,
    _PHASE1_COARSE_ET,
    _PHASE1_COARSE_GB,
    _PHASE1_COARSE_HGB,
    _PHASE1_COARSE_MLP_CLF,
    _PHASE1_COARSE_MLP_REG,
    _PHASE1_COARSE_RF,
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
            if key == "hidden_layer_sizes":
                normalized = _normalize_hidden_layer_sizes(p[key])
                if normalized is not None:
                    out[key] = normalized
            else:
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
        et_space = get_tuning_search_space("et")
        if et_space:
            p = _to_pipeline_params(et_space, rs)
        else:
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
            candidates.append(("quantile", "QuantileRegressor", qr, {"est__estimator__max_depth": [6, 10, 14, 20]}))
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
