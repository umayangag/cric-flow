"""
Auto-tune hyperparameters and model type for batting, bowling, fielding, extras, and win.

Searches over algorithms (RandomForest, GradientBoosting) and hyperparameters using
RandomizedSearchCV with temporal or K-fold CV. Saves the best scaler + model in the
same format as train_* scripts, plus a tuning report JSON. Extras and win save model only (no scaler).

Usage:
  # From CSV (per-format)
  python -m ml.auto_tune --model batting --csv ../output/go-app/batting_encoded_T20.csv --format T20
  python -m ml.auto_tune --model fielding --csv path/to/fielding.csv --format T20

  # From go-app API (required for extras and win)
  GO_APP_URL=http://localhost:8080 python -m ml.auto_tune --model extras --from-api --cutoff 2024-12-01T00:00:00Z --all-formats
  GO_APP_URL=http://localhost:8080 python -m ml.auto_tune --model win --from-api --cutoff 2024-12-01T00:00:00Z --all-formats

  # All models, all formats (extras/win from API only)
  python -m ml.auto_tune --model all --all-formats --from-api --cutoff 2024-12-01T00:00:00Z

  # Fine-tune one model for one format then apply best params to config (see docs/ML_AUTO_TUNE.md)
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
from typing import Any, Dict, List, Optional, Tuple

import numpy as np

logger = logging.getLogger(__name__)
import joblib
import pandas as pd
from sklearn.ensemble import (
    GradientBoostingClassifier,
    GradientBoostingRegressor,
    RandomForestClassifier,
    RandomForestRegressor,
    StackingRegressor,
)
from sklearn.linear_model import Ridge
from sklearn.model_selection import RandomizedSearchCV
from sklearn.multioutput import MultiOutputRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

# Ensure ml-service root is on path for config and app imports
_ML_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _ML_ROOT not in sys.path:
    sys.path.insert(0, _ML_ROOT)

from ml.config import get_training_params, get_tuning_config, get_tuning_search_space, save_tuned_params_to_go_app

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

    candidates = [
        ("RandomForestRegressor", RandomForestRegressor(), rf_params),
        ("GradientBoostingRegressor", GradientBoostingRegressor(), gb_params),
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
        candidates.append(("QuantileRegressor", qr, {"est__estimator__max_depth": [tp.get("max_depth", 12)]}))
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
        candidates.append(("StackingRegressor", stacked, {"est__estimator__final_estimator__random_state": [rs]}))
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


def _search_space_regression_single(model_kind: str) -> List[Tuple[str, Any, Dict[str, Any]]]:
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
        ("RandomForestRegressor", RandomForestRegressor(), rf_params),
        ("GradientBoostingRegressor", GradientBoostingRegressor(), gb_params),
    ]


def _search_space_classification(model_kind: str) -> List[Tuple[str, Any, Dict[str, Any]]]:
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
        ("RandomForestClassifier", RandomForestClassifier(), rf_params),
        ("GradientBoostingClassifier", GradientBoostingClassifier(), gb_params),
    ]


def _run_search_single_regression(
    X: np.ndarray,
    y: np.ndarray,
    model_kind: str,
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Run RandomizedSearchCV for single-output regression. Returns (best_pipeline, best_params, report)."""
    candidates = _search_space_regression_single(model_kind)
    best_score = None
    best_pipe = None
    best_params = None
    all_cv_results: List[Dict[str, Any]] = []
    tuning_cfg = get_tuning_config()
    n_jobs = tuning_cfg.get("n_jobs", 1)
    if os.environ.get("AUTO_TUNE_N_JOBS") is not None:
        try:
            n_jobs = int(os.environ["AUTO_TUNE_N_JOBS"])
        except ValueError:
            pass
    for name, base_est, param_dist in candidates:
        pipe = _build_pipeline_single_regression(base_est)
        search = RandomizedSearchCV(
            pipe,
            param_distributions=param_dist,
            n_iter=min(n_iter, max(1, _count_combinations(param_dist) // 2)),
            cv=cv_splits,
            scoring=scoring,
            random_state=random_state,
            n_jobs=n_jobs,
            error_score="raise",
        )
        search.fit(X, y)
        all_cv_results.append(
            {"estimator": name, "best_score": float(search.best_score_), "best_params": search.best_params_}
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
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "candidates": all_cv_results,
    }
    return best_pipe, best_params, report


def _run_search_classification(
    X: np.ndarray,
    y: np.ndarray,
    model_kind: str,
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Run RandomizedSearchCV for binary classification (win). Returns (best_pipeline, best_params, report)."""
    candidates = _search_space_classification(model_kind)
    best_score = None
    best_pipe = None
    best_params = None
    all_cv_results: List[Dict[str, Any]] = []
    tuning_cfg = get_tuning_config()
    n_jobs = tuning_cfg.get("n_jobs", 1)
    if os.environ.get("AUTO_TUNE_N_JOBS") is not None:
        try:
            n_jobs = int(os.environ["AUTO_TUNE_N_JOBS"])
        except ValueError:
            pass
    for name, base_est, param_dist in candidates:
        pipe = Pipeline([("scaler", StandardScaler()), ("est", base_est)])
        search = RandomizedSearchCV(
            pipe,
            param_distributions=param_dist,
            n_iter=min(n_iter, max(1, _count_combinations(param_dist) // 2)),
            cv=cv_splits,
            scoring=scoring,
            random_state=random_state,
            n_jobs=n_jobs,
            error_score="raise",
        )
        search.fit(X, y)
        all_cv_results.append(
            {"estimator": name, "best_score": float(search.best_score_), "best_params": search.best_params_}
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
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "candidates": all_cv_results,
    }
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


def _run_search(
    X: np.ndarray,
    Y: np.ndarray,
    model_kind: str,
    cv_splits: int,
    n_iter: int,
    scoring: str,
    random_state: int = 42,
) -> Tuple[Pipeline, Dict[str, Any], Dict[str, Any]]:
    """Run RandomizedSearchCV over algorithms and params. Returns (best_pipeline, best_params, report)."""
    candidates = _search_space_regression(model_kind)
    best_score = None
    best_pipe = None
    best_params = None
    all_cv_results: List[Dict[str, Any]] = []

    tuning_cfg = get_tuning_config()
    n_jobs = tuning_cfg.get("n_jobs", 1)
    if os.environ.get("AUTO_TUNE_N_JOBS") is not None:
        try:
            n_jobs = int(os.environ["AUTO_TUNE_N_JOBS"])
        except ValueError:
            pass
    for name, base_est, param_dist in candidates:
        pipe = _build_pipeline(base_est)
        search = RandomizedSearchCV(
            pipe,
            param_distributions=param_dist,
            n_iter=min(n_iter, max(1, _count_combinations(param_dist) // 2)),
            cv=cv_splits,
            scoring=scoring,
            random_state=random_state,
            n_jobs=n_jobs,
            error_score="raise",
        )
        search.fit(X, Y)
        all_cv_results.append(
            {
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
                key = k.replace("est__estimator__", "")
                config_snippet[key] = v

    report = {
        "model_kind": model_kind,
        "best_cv_score": best_score,
        "best_params": best_params,
        "config_snippet": config_snippet,
        "scoring": scoring,
        "cv_splits": cv_splits,
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "n_targets": int(Y.shape[1]),
        "candidates": all_cv_results,
    }
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

    data = fetch_training_data(go_app_url, format_code, cutoff, api_key)
    bat = data.get("batting") or {}
    headers = bat.get("headers") or []
    rows = bat.get("rows") or []
    return _batting_rows_to_xy(headers, rows)


def load_bowling_from_api(
    go_app_url: str, format_code: str, cutoff: str, api_key: Optional[str]
) -> Tuple[np.ndarray, np.ndarray]:
    from app.train_on_the_fly import _bowling_rows_to_xy, fetch_training_data

    data = fetch_training_data(go_app_url, format_code, cutoff, api_key)
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
) -> Dict[str, Any]:
    """Run search, save artifacts and report. Returns report dict."""
    tuning = _get_tuning_config()
    cv_splits = tuning["cv_splits"]
    n_iter = tuning["n_iter"]
    scoring = tuning["scoring"]
    random_state = tuning.get("random_state") or get_training_params(model_kind).get("random_state", 42)
    params = get_training_params(model_kind)
    joblib_compress = params["joblib_compress"]

    best_pipe, best_params, report = _run_search(X, Y, model_kind, cv_splits, n_iter, scoring, random_state)
    _save_artifacts(best_pipe, out_dir, model_kind, format_suffix, joblib_compress, report)
    return report


def run_auto_tune_extras(
    X: np.ndarray,
    Y: np.ndarray,
    format_suffix: Optional[str],
    out_dir: str,
) -> Dict[str, Any]:
    """Run single-output regression search for extras; save model only + report."""
    tuning = _get_tuning_config()
    cv_splits = tuning["cv_splits"]
    n_iter = tuning["n_iter"]
    scoring = tuning.get("scoring", "neg_mean_absolute_error")
    params = get_training_params("extras")
    random_state = tuning.get("random_state") or params.get("random_state", 42)
    joblib_compress = params["joblib_compress"]
    y = Y.ravel() if Y.ndim > 1 else Y
    best_pipe, _, report = _run_search_single_regression(X, y, "extras", cv_splits, n_iter, scoring, random_state)
    _save_artifacts_model_only(best_pipe, out_dir, "extras", format_suffix, joblib_compress, report)
    return report


def run_auto_tune_win(
    X: np.ndarray,
    Y: np.ndarray,
    format_suffix: Optional[str],
    out_dir: str,
) -> Dict[str, Any]:
    """Run classification search for win; save model only + report."""
    tuning = _get_tuning_config()
    cv_splits = tuning["cv_splits"]
    n_iter = tuning["n_iter"]
    scoring = "accuracy"
    params = get_training_params("win")
    random_state = tuning.get("random_state") or params.get("random_state", 42)
    joblib_compress = params["joblib_compress"]
    y = Y.ravel() if Y.ndim > 1 else Y
    best_pipe, _, report = _run_search_classification(X, y, "win", cv_splits, n_iter, scoring, random_state)
    _save_artifacts_model_only(best_pipe, out_dir, "win", format_suffix, joblib_compress, report)
    return report


def main() -> None:
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
    args = parser.parse_args()

    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR")
    if not out_dir:
        from ml.config import default_artifacts_dir

        out_dir = default_artifacts_dir()

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
        if not go_app_url or not report.get("config_snippet"):
            return
        try:
            save_tuned_params_to_go_app(go_app_url, model, format_suffix or "", report["config_snippet"], api_key)
        except ValueError as e:
            logger.warning(
                "auto_tune.save_tuned_params_failed model=%s format=%s error=%s",
                model,
                format_suffix,
                e,
            )

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
                            report = run_auto_tune_extras(X, Y, fcode, out_dir)
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
                            report = run_auto_tune_win(X, Y, fcode, out_dir)
                            _maybe_save_tuned_params(args.go_app_url, "win", fcode, report, args.api_key or None)
                            logger.info(
                                "auto_tune.done model=win format=%s n=%s best_cv_score=%s",
                                fcode,
                                X.shape[0],
                                report["best_cv_score"],
                            )
                        continue
                    if model_kind == "batting":
                        X, Y = load_batting_from_api(args.go_app_url, fmt or "all", args.cutoff, args.api_key or None)
                    elif model_kind == "bowling":
                        X, Y = load_bowling_from_api(args.go_app_url, fmt or "all", args.cutoff, args.api_key or None)
                    else:
                        by_f = load_fielding_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt)
                        if not by_f:
                            logger.warning("auto_tune.no_fielding_data format=%s", fmt)
                            continue
                        # Run once per format from API
                        for fcode, (X, Y) in by_f.items():
                            if X.size == 0 or Y.size == 0:
                                continue
                            report = run_auto_tune(model_kind, X, Y, fcode, out_dir)
                            _maybe_save_tuned_params(args.go_app_url, model_kind, fcode, report, args.api_key or None)
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
                report = run_auto_tune(model_kind, X, Y, format_suffix, out_dir)
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
                    default_dir = os.environ.get("GO_APP_OUTPUT_DIR", os.path.join(_ML_ROOT, "..", "output", "go-app"))
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
                        report = run_auto_tune(model_kind, X, Y, fcode, out_dir)
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
                    logger.warning("auto_tune.skip_csv_not_found model=%s format=%s path=%s", model_kind, fmt, csv_path)
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
                report = run_auto_tune(model_kind, X, Y, format_suffix, out_dir)
                _maybe_save_tuned_params(args.go_app_url, model_kind, format_suffix, report, args.api_key or None)
                logger.info(
                    "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                    model_kind,
                    format_suffix,
                    X.shape[0],
                    report["best_cv_score"],
                )


if __name__ == "__main__":
    main()
