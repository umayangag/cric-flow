"""Unit tests for ml.auto_tune helper functions and data loading."""

import os
from unittest.mock import patch

import numpy as np
import pandas as pd
import pytest
from sklearn.ensemble import RandomForestRegressor
from sklearn.model_selection import KFold, TimeSeriesSplit
from sklearn.pipeline import Pipeline

from ml.auto_tune import (
    AVAILABLE_ALGORITHMS,
    BAT_SEQ_COLS,
    BATTING_FEATURE_COLS,
    BOWL_SEQ_COLS,
    BOWLING_FEATURE_COLS,
    BOWLING_TARGET_COLS,
    load_bowling_csv,
)


def _get_module():
    import ml.auto_tune as m

    return m


def test_available_algorithms_non_empty():
    """AVAILABLE_ALGORITHMS contains expected algorithm keys."""
    assert len(AVAILABLE_ALGORITHMS) > 0
    assert "rf" in AVAILABLE_ALGORITHMS
    assert "gb" in AVAILABLE_ALGORITHMS
    assert "et" in AVAILABLE_ALGORITHMS
    assert "hgb" in AVAILABLE_ALGORITHMS


def test_bat_seq_cols_non_empty():
    """BAT_SEQ_COLS defines batting sequence columns."""
    assert len(BAT_SEQ_COLS) > 0
    assert "bat_prev_sr" in BAT_SEQ_COLS


def test_bowl_seq_cols_non_empty():
    """BOWL_SEQ_COLS defines bowling sequence columns."""
    assert len(BOWL_SEQ_COLS) > 0
    assert "bowl_prev_wkt_rate" in BOWL_SEQ_COLS


def test_batting_feature_cols_contains_required():
    """BATTING_FEATURE_COLS includes base features and sequence cols."""
    assert "batting_consistency" in BATTING_FEATURE_COLS
    assert "batting_form" in BATTING_FEATURE_COLS
    assert "temp" in BATTING_FEATURE_COLS
    for c in BAT_SEQ_COLS:
        assert c in BATTING_FEATURE_COLS


def test_bowling_feature_cols_contains_required():
    """BOWLING_FEATURE_COLS includes base features and sequence cols."""
    assert "bowling_consistency" in BOWLING_FEATURE_COLS
    assert "bowling_form" in BOWLING_FEATURE_COLS
    for c in BOWL_SEQ_COLS:
        assert c in BOWLING_FEATURE_COLS


def test_get_cv_object_kfold():
    """_get_cv_object returns KFold for kfold validation."""
    m = _get_module()
    cv = m._get_cv_object("kfold", cv_splits=5, n_samples=100)
    assert isinstance(cv, KFold)
    assert cv.n_splits == 5


def test_get_cv_object_walk_forward():
    """_get_cv_object returns TimeSeriesSplit for walk_forward with enough samples."""
    m = _get_module()
    cv = m._get_cv_object("walk_forward", cv_splits=5, n_samples=50)
    assert isinstance(cv, TimeSeriesSplit)


def test_get_cv_object_tiny_samples_raises():
    """_get_cv_object raises for fewer than 2 samples."""
    m = _get_module()
    with pytest.raises(ValueError, match="at least 2 samples"):
        m._get_cv_object("kfold", cv_splits=5, n_samples=1)


def test_get_cv_object_tiny_samples_kfold():
    """_get_cv_object uses KFold for very small datasets with walk_forward."""
    m = _get_module()
    cv = m._get_cv_object("walk_forward", cv_splits=5, n_samples=2)
    assert isinstance(cv, KFold)


def test_effective_n_jobs_override():
    """_effective_n_jobs returns override when provided."""
    m = _get_module()
    assert m._effective_n_jobs({"n_jobs": 1}, n_jobs_override=4) == 4


def test_effective_n_jobs_from_config():
    """_effective_n_jobs returns config value when no override."""
    m = _get_module()
    assert m._effective_n_jobs({"n_jobs": 3}) == 3


def test_effective_n_jobs_from_env():
    """_effective_n_jobs uses AUTO_TUNE_N_JOBS when set."""
    m = _get_module()
    with patch.dict(os.environ, {"AUTO_TUNE_N_JOBS": "8"}):
        assert m._effective_n_jobs({"n_jobs": 1}) == 8


def test_effective_n_jobs_invalid_env_falls_back():
    """_effective_n_jobs falls back to config when env is invalid."""
    m = _get_module()
    with patch.dict(os.environ, {"AUTO_TUNE_N_JOBS": "abc"}):
        assert m._effective_n_jobs({"n_jobs": 5}) == 5


def test_coarse_to_single_prefix():
    """_coarse_to_single_prefix converts est__estimator__* to est__*."""
    m = _get_module()
    d = {"est__estimator__n_estimators": [50, 100], "est__estimator__max_depth": [6, 12]}
    out = m._coarse_to_single_prefix(d)
    assert out == {"est__n_estimators": [50, 100], "est__max_depth": [6, 12]}


def test_to_pipeline_params():
    """_to_pipeline_params converts config_space to pipeline param format."""
    m = _get_module()
    config = {"n_estimators": [50, 100], "max_depth": 12}
    out = m._to_pipeline_params(config, random_state=42)
    assert "est__estimator__random_state" in out
    assert out["est__estimator__random_state"] == [42]
    assert "est__estimator__n_estimators" in out
    assert out["est__estimator__n_estimators"] == [50, 100]
    assert "est__estimator__max_depth" in out
    assert out["est__estimator__max_depth"] == [12]


def test_to_pipeline_params_skips_random_state():
    """_to_pipeline_params skips random_state key in config_space."""
    m = _get_module()
    config = {"random_state": 99, "n_estimators": [50]}
    out = m._to_pipeline_params(config, random_state=42)
    assert out["est__estimator__random_state"] == [42]


def test_build_pipeline():
    """_build_pipeline creates MultiOutputRegressor pipeline."""
    m = _get_module()
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = m._build_pipeline(est)
    assert isinstance(pipe, Pipeline)
    assert "scaler" in pipe.named_steps
    assert "est" in pipe.named_steps


def test_build_pipeline_single_regression():
    """_build_pipeline_single_regression creates single-estimator pipeline."""
    m = _get_module()
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = m._build_pipeline_single_regression(est)
    assert isinstance(pipe, Pipeline)
    assert "scaler" in pipe.named_steps
    assert "est" in pipe.named_steps


def test_count_combinations():
    """_count_combinations computes product of param list lengths."""
    m = _get_module()
    d = {"a": [1, 2, 3], "b": [10, 20]}
    assert m._count_combinations(d) == 6
    assert m._count_combinations({}) == 1
    assert m._count_combinations({"x": [1]}) == 1


def test_compute_metrics_regression():
    """_compute_metrics_regression returns dict with mae, rmse, r2, etc."""
    m = _get_module()
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = m._build_pipeline_single_regression(est)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).rand(50)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    metrics = m._compute_metrics_regression(pipe, X, y, cv)
    assert "mae" in metrics
    assert "rmse" in metrics
    assert "r2" in metrics
    assert "r2_pct" in metrics
    assert "median_ae" in metrics
    assert "max_error" in metrics
    assert "explained_variance" in metrics


def test_compute_metrics_regression_single_output():
    """_compute_metrics_regression works with 1d y."""
    m = _get_module()
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = m._build_pipeline_single_regression(est)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).rand(50)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    metrics = m._compute_metrics_regression(pipe, X, y, cv)
    assert "mae" in metrics
    assert "r2" in metrics


def test_compute_mlqa_audit():
    """_compute_mlqa_audit returns audit_status, key_findings, bias_report, final_verdict."""
    m = _get_module()
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = m._build_pipeline_single_regression(est)
    X = np.random.RandomState(42).rand(80, 5)
    y = np.random.RandomState(43).rand(80)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {
        "best_cv_score": -0.4,
        "candidates": [{"algorithm": "rf", "best_score": -0.4}],
        "algorithms": ["rf"],
    }
    audit = m._compute_mlqa_audit(
        report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", ["f0", "f1", "f2", "f3", "f4"]
    )
    assert audit["audit_status"] in ("PASS", "FAIL", "WARNING")
    assert "key_findings" in audit
    assert isinstance(audit["key_findings"], list)
    assert "bias_report" in audit
    assert audit["final_verdict"] in ("Proceed to Deployment", "Rollback & Re-tune")
    if "checks" in audit:
        assert "overfitting" in audit["checks"] or "stability" in audit["checks"]


def test_load_bowling_csv_minimal(tmp_path):
    """load_bowling_csv loads minimal valid CSV."""
    base_cols = [c for c in BOWLING_FEATURE_COLS if c not in BOWL_SEQ_COLS]
    df = pd.DataFrame({c: [1.0] * 5 for c in base_cols})
    for c in BOWL_SEQ_COLS:
        df[c] = 0.0
    df["bowling_form_short"] = df["bowling_form"]
    df["bowling_form_long"] = df["bowling_form"]
    df["runs"] = [20, 30, 40, 50, 60]
    df["balls"] = [24, 24, 24, 24, 24]
    df["wickets"] = [1, 2, 0, 1, 2]
    path = tmp_path / "bowling.csv"
    df.to_csv(path, index=False)
    X, Y = load_bowling_csv(str(path))
    assert X.shape[0] == 5
    assert X.shape[1] == len(BOWLING_FEATURE_COLS)
    assert Y.shape[0] == 5
    assert Y.shape[1] >= len(BOWLING_TARGET_COLS)


def test_search_space_regression_batting():
    """_search_space_regression returns candidates for batting."""
    m = _get_module()
    cands = m._search_space_regression("batting")
    assert len(cands) >= 2
    names = [c[0] for c in cands]
    assert "rf" in names
    assert "gb" in names


def test_search_space_regression_single():
    """_search_space_regression_single returns candidates for extras."""
    m = _get_module()
    cands = m._search_space_regression_single("extras")
    assert len(cands) >= 2
    assert cands[0][0] == "rf"
    assert cands[1][0] == "gb"


def test_search_space_classification():
    """_search_space_classification returns candidates for win."""
    m = _get_module()
    cands = m._search_space_classification("win")
    assert len(cands) >= 2
    assert cands[0][0] == "rf"
    assert cands[1][0] == "gb"


def test_phase1_candidates_regression():
    """_phase1_candidates_regression returns phase1 candidates."""
    m = _get_module()
    allow = frozenset({"rf", "gb"})
    cands = m._phase1_candidates_regression("batting", allow)
    assert len(cands) == 2
    assert cands[0][0] == "rf"
    assert cands[1][0] == "gb"


def test_phase1_candidates_regression_single():
    """_phase1_candidates_regression_single returns single-output phase1 candidates."""
    m = _get_module()
    allow = frozenset({"rf", "gb"})
    cands = m._phase1_candidates_regression_single(allow)
    assert len(cands) == 2


def test_phase1_candidates_classification():
    """_phase1_candidates_classification returns classification candidates."""
    m = _get_module()
    allow = frozenset({"rf", "gb"})
    cands = m._phase1_candidates_classification(allow)
    assert len(cands) == 2


def test_run_auto_tune_batting_minimal(tmp_path):
    """run_auto_tune completes with minimal data for batting (fast smoke test)."""
    m = _get_module()
    np.random.seed(42)
    X = np.random.rand(40, 5).astype(np.float32)
    Y = np.random.rand(40, 5).astype(np.float32)
    out_dir = str(tmp_path / "out")
    report = m.run_auto_tune(
        model_kind="batting",
        X=X,
        Y=Y,
        format_suffix="T20",
        out_dir=out_dir,
        algorithms=["rf"],
        validation_method="kfold",
        n_jobs_override=1,
        fast_mode=True,
    )
    assert "metrics" in report or "best_algorithm" in report
    assert (tmp_path / "out" / "batting_scaler_T20.joblib").exists()
    assert (tmp_path / "out" / "batting_model_T20.joblib").exists()


def test_run_auto_tune_extras_minimal(tmp_path):
    """run_auto_tune_extras completes with minimal data (fast smoke test)."""
    m = _get_module()
    np.random.seed(42)
    X = np.random.rand(40, 5).astype(np.float32)
    Y = np.random.rand(40).astype(np.float32)
    out_dir = str(tmp_path / "out_extras")
    report = m.run_auto_tune_extras(
        X=X,
        Y=Y,
        format_suffix="T20",
        out_dir=out_dir,
        algorithms=["rf"],
        validation_method="kfold",
        n_jobs_override=1,
        fast_mode=True,
    )
    assert "best_cv_score" in report or "best_algorithm" in report
    assert (tmp_path / "out_extras" / "extras_model_T20.joblib").exists()


def test_run_auto_tune_win_minimal(tmp_path):
    """run_auto_tune_win completes with minimal binary classification data (fast smoke test)."""
    m = _get_module()
    np.random.seed(42)
    X = np.random.rand(40, 5).astype(np.float32)
    Y = np.random.randint(0, 2, size=40).astype(np.float32)
    out_dir = str(tmp_path / "out_win")
    report = m.run_auto_tune_win(
        X=X,
        Y=Y,
        format_suffix="T20",
        out_dir=out_dir,
        algorithms=["rf"],
        validation_method="kfold",
        n_jobs_override=1,
        fast_mode=True,
    )
    assert "best_cv_score" in report or "best_algorithm" in report
    assert (tmp_path / "out_win" / "win_model_T20.joblib").exists()
