"""Unit tests for ml.auto_tune helper functions and data loading.

Tests in this file must NOT run real model training (no Optuna/search, no fitting
multi-estimator pipelines). Use mocks for run_auto_tune* entry points. Any new
test that would run real training belongs in a separate module (e.g. behind
RUN_AUTO_TUNE_SMOKE=1) so CI stays fast.
"""

import os
from unittest.mock import patch

import numpy as np
import pandas as pd
import pytest
from sklearn.ensemble import RandomForestClassifier, RandomForestRegressor
from sklearn.model_selection import KFold, TimeSeriesSplit
from sklearn.multioutput import MultiOutputRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from ml.auto_tune import (
    AVAILABLE_ALGORITHMS,
    BAT_SEQ_COLS,
    BATTING_FEATURE_COLS,
    BOWL_SEQ_COLS,
    BOWLING_FEATURE_COLS,
    BOWLING_TARGET_COLS,
    _get_prior_tuned_algorithm,
    _normalize_hidden_layer_sizes,
    _prior_params_to_optuna_regression,
    _save_artifacts,
    _save_artifacts_model_only,
    load_bowling_csv,
)
from ml.tuning.types import target_names_for_model


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
    """BATTING_FEATURE_COLS includes raw stat features, context, and sequence cols."""
    assert "batting_mean_w3" in BATTING_FEATURE_COLS
    assert "batting_mean_w5" in BATTING_FEATURE_COLS
    assert "temp" in BATTING_FEATURE_COLS
    for c in BAT_SEQ_COLS:
        assert c in BATTING_FEATURE_COLS


def test_bowling_feature_cols_contains_required():
    """BOWLING_FEATURE_COLS includes raw stat features, context, and sequence cols."""
    assert "bowling_mean_w3" in BOWLING_FEATURE_COLS
    assert "bowling_mean_w5" in BOWLING_FEATURE_COLS
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


def test_effective_timeseries_gap():
    """_effective_timeseries_gap caps gap for small datasets (win, extras)."""
    m = _get_module()
    assert m._effective_timeseries_gap(200, 2007) == 0  # win: small dataset, no gap
    assert m._effective_timeseries_gap(200, 4000) == 0  # below 5000
    assert m._effective_timeseries_gap(200, 23862) == 200  # bowling: use full gap
    assert m._effective_timeseries_gap(500, 10000) == 500  # cap at n/20=500
    assert m._effective_timeseries_gap(500, 5000) == 250  # cap at 5000/20=250
    assert m._effective_timeseries_gap(0, 10000) == 0


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
    """_compute_metrics_regression returns dict with mae, rmse, r2, target_context, baseline, learning_curve."""
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
    assert "target_context" in metrics
    ctx = metrics["target_context"]
    assert "target_mean" in ctx
    assert "target_std" in ctx
    assert "mae_pct_of_mean" in ctx
    assert "baseline_comparison" in metrics
    bc = metrics["baseline_comparison"]
    assert "baseline_mae" in bc
    assert "baseline_improvement_pct" in bc
    assert "learning_curve" in metrics
    lc = metrics["learning_curve"]
    assert "val_still_improving" in lc
    assert "overfitting_gap" in lc


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


def test_compute_metrics_regression_multi_output_per_target_mae():
    """_compute_metrics_regression with 2D y and target_names includes per_target_mae."""
    m = _get_module()
    pipe = _minimal_batting_pipeline()
    X = np.random.RandomState(42).rand(50, 5)
    Y = np.random.RandomState(43).rand(50, 3)
    pipe.fit(X, Y)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    metrics = m._compute_metrics_regression(pipe, X, Y, cv, target_names=["runs", "balls", "wickets"])
    assert "per_target_mae" in metrics
    assert "mae_runs" in metrics["per_target_mae"]
    assert "mae_balls" in metrics["per_target_mae"]
    assert "mae_wickets" in metrics["per_target_mae"]


def test_compute_metrics_classification_minimal():
    """_compute_metrics_classification returns accuracy, precision, recall, f1."""
    m = _get_module()
    pipe = _minimal_model_only_pipeline(regression=False)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).randint(0, 2, 50)
    pipe.fit(X, y)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    metrics = m._compute_metrics_classification(pipe, X, y, cv)
    assert "accuracy" in metrics
    assert "accuracy_pct" in metrics
    assert "precision" in metrics
    assert "recall" in metrics
    assert "f1" in metrics


def test_compute_mlqa_audit_missing_score_returns_warning():
    """_compute_mlqa_audit returns WARNING when report has no best_cv_score."""
    m = _get_module()
    pipe = _minimal_model_only_pipeline(regression=True)
    X = np.random.RandomState(42).rand(30, 5)
    y = np.random.RandomState(43).rand(30)
    pipe.fit(X, y)
    cv = KFold(n_splits=2, shuffle=True, random_state=42)
    audit = m._compute_mlqa_audit({}, pipe, X, y, cv, "neg_mean_absolute_error", "regression")
    assert audit["audit_status"] == "WARNING"
    assert "Missing validation score" in audit["key_findings"][0]


def test_compute_learning_curve_regression_small_n_returns_none():
    """_compute_learning_curve_regression returns None when n_samples < 20."""
    m = _get_module()
    pipe = _minimal_model_only_pipeline(regression=True)
    X = np.random.RandomState(42).rand(10, 5)
    y = np.random.RandomState(43).rand(10)
    pipe.fit(X, y)
    cv = KFold(n_splits=2, shuffle=True, random_state=42)
    lc = m._compute_learning_curve_regression(pipe, X, y, cv, "neg_mean_absolute_error")
    assert lc is None


def test_compute_mlqa_audit_with_fairness_metrics():
    """_compute_mlqa_audit includes fairness check when report has fairness_metrics."""
    m = _get_module()
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = m._build_pipeline_single_regression(est)
    X = np.random.RandomState(42).rand(80, 5)
    y = np.random.RandomState(43).rand(80)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {
        "best_cv_score": -0.4,
        "fairness_metrics": {"disparate_impact_ratio": 0.95},
    }
    audit = m._compute_mlqa_audit(
        report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", ["f0", "f1", "f2", "f3", "f4"]
    )
    assert "Fairness" in " ".join(audit["key_findings"]) or "fairness" in audit["bias_report"].lower()


def test_add_final_report_details_adds_mlqa_and_feature_importance():
    """_add_final_report_details adds mlqa_audit and feature_importance to report."""
    m = _get_module()
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = m._build_pipeline_single_regression(est)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).rand(50)
    pipe.fit(X, y)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {"best_cv_score": -0.35}
    m._add_final_report_details(report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", "batting")
    assert "mlqa_audit" in report
    assert "feature_importance" in report
    assert len(report["feature_importance"]) <= 5


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


def _minimal_batting_pipeline():
    """Minimal pipeline for run_auto_tune (scaler + model) so _save_artifacts can dump without running search."""
    return Pipeline(
        [
            ("scaler", StandardScaler()),
            ("est", MultiOutputRegressor(RandomForestRegressor(n_estimators=1, random_state=42))),
        ]
    )


def _minimal_model_only_pipeline(regression=True):
    """Minimal pipeline for extras/win (model only) so _save_artifacts_model_only can dump without running search."""
    est = (
        RandomForestRegressor(n_estimators=1, random_state=42)
        if regression
        else RandomForestClassifier(n_estimators=1, random_state=42)
    )
    return Pipeline([("scaler", StandardScaler()), ("est", est)])


def test_run_auto_tune_batting_minimal(tmp_path):
    """run_auto_tune returns report and writes artifacts; search is mocked to avoid real training."""
    m = _get_module()
    np.random.seed(42)
    X = np.random.rand(40, 5).astype(np.float32)
    Y = np.random.rand(40, 5).astype(np.float32)
    out_dir = str(tmp_path / "out")
    minimal_pipe = _minimal_batting_pipeline()
    minimal_pipe.fit(X, Y)
    mock_report = {"best_algorithm": "rf", "metrics": {}}

    with patch("ml.tuning.runners._run_search_two_phase", return_value=(minimal_pipe, {}, mock_report)):
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
    """run_auto_tune_extras returns report and writes model artifact; search is mocked to avoid real training."""
    m = _get_module()
    np.random.seed(42)
    X = np.random.rand(40, 5).astype(np.float32)
    Y = np.random.rand(40).astype(np.float32)
    out_dir = str(tmp_path / "out_extras")
    minimal_pipe = _minimal_model_only_pipeline(regression=True)
    minimal_pipe.fit(X, Y)
    mock_report = {"best_algorithm": "rf"}
    # Omit best_cv_score so run_auto_tune_extras does not invoke _maybe_run_autogluon_and_compare

    with patch(
        "ml.tuning.runners._run_search_two_phase_single_regression",
        return_value=(minimal_pipe, {}, mock_report),
    ):
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
    """run_auto_tune_win returns report and writes model artifact; search is mocked to avoid real training."""
    m = _get_module()
    np.random.seed(42)
    X = np.random.rand(40, 5).astype(np.float32)
    Y = np.random.randint(0, 2, size=40).astype(np.float32)
    out_dir = str(tmp_path / "out_win")
    minimal_pipe = _minimal_model_only_pipeline(regression=False)
    minimal_pipe.fit(X, Y)
    mock_report = {"best_algorithm": "rf"}
    # Omit best_cv_score so run_auto_tune_win does not invoke _maybe_run_autogluon_and_compare

    with patch(
        "ml.tuning.runners._run_search_two_phase_classification",
        return_value=(minimal_pipe, {}, mock_report),
    ):
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


def test_save_artifacts_writes_scaler_model_and_report(tmp_path):
    """_save_artifacts writes scaler, model, and report JSON (covers optuna_search save path without search)."""
    pipe = _minimal_batting_pipeline()
    pipe.fit(np.random.rand(10, 3), np.random.rand(10, 2))
    report = {"best_algorithm": "rf", "best_cv_score": -0.5}
    out_dir = str(tmp_path / "artifacts")
    _save_artifacts(pipe, out_dir, "batting", "ODI", joblib_compress=1, report=report)
    assert (tmp_path / "artifacts" / "batting_scaler_ODI.joblib").exists()
    assert (tmp_path / "artifacts" / "batting_model_ODI.joblib").exists()
    report_path = tmp_path / "artifacts" / "tuning_report_batting_ODI.json"
    assert report_path.exists()
    import json

    with open(report_path, encoding="utf-8") as f:
        loaded = json.load(f)
    assert loaded["best_algorithm"] == "rf"


def test_save_artifacts_without_format_suffix(tmp_path):
    """_save_artifacts with format_suffix=None uses unscoped filenames."""
    pipe = _minimal_batting_pipeline()
    pipe.fit(np.random.rand(10, 3), np.random.rand(10, 2))
    _save_artifacts(pipe, str(tmp_path), "bowling", None, joblib_compress=0, report={})
    assert (tmp_path / "bowling_scaler.joblib").exists()
    assert (tmp_path / "bowling_model.joblib").exists()
    assert (tmp_path / "tuning_report_bowling.json").exists()


def test_save_artifacts_model_only_writes_model_and_report(tmp_path):
    """_save_artifacts_model_only writes model and report JSON (covers optuna_search save path without search)."""
    pipe = _minimal_model_only_pipeline(regression=True)
    pipe.fit(np.random.rand(10, 3), np.random.rand(10))
    report = {"best_algorithm": "gb"}
    out_dir = str(tmp_path / "extras")
    _save_artifacts_model_only(pipe, out_dir, "extras", "T20", joblib_compress=1, report=report)
    assert (tmp_path / "extras" / "extras_model_T20.joblib").exists()
    import json

    with open(tmp_path / "extras" / "tuning_report_extras_T20.json", encoding="utf-8") as f:
        loaded = json.load(f)
    assert loaded["best_algorithm"] == "gb"


def test_save_artifacts_model_only_without_format_suffix(tmp_path):
    """_save_artifacts_model_only with format_suffix=None uses unscoped filenames."""
    pipe = _minimal_model_only_pipeline(regression=False)
    pipe.fit(np.random.rand(10, 3), np.random.randint(0, 2, 10))
    _save_artifacts_model_only(pipe, str(tmp_path), "win", None, joblib_compress=0, report={})
    assert (tmp_path / "win_model.joblib").exists()
    assert (tmp_path / "tuning_report_win.json").exists()


def test_get_prior_tuned_algorithm_from_report_file(tmp_path):
    """_get_prior_tuned_algorithm returns (algorithm, params) when tuning_report exists in out_dir."""
    report_path = tmp_path / "tuning_report_batting_T20.json"
    report_path.write_text(
        '{"algorithms": ["rf"], "config_snippet": {"n_estimators": 50}, "best_params": {}}',
        encoding="utf-8",
    )
    out_dir = str(tmp_path)
    result = _get_prior_tuned_algorithm("batting", "T20", out_dir)
    assert result is not None
    algo, prior = result
    assert algo == "rf"
    assert prior.get("n_estimators") == 50


def test_get_prior_tuned_algorithm_returns_none_when_no_report(tmp_path):
    """_get_prior_tuned_algorithm returns None when out_dir has no tuning report."""
    result = _get_prior_tuned_algorithm("batting", "T20", str(tmp_path))
    assert result is None


def test_run_auto_tune_with_prior_report_uses_prior_algorithm(tmp_path):
    """run_auto_tune with prior report in out_dir uses prior algorithm (covers prior branch; search mocked)."""
    (tmp_path / "tuning_report_batting_T20.json").write_text(
        '{"algorithms": ["rf"], "config_snippet": {"n_estimators": 20}, "best_params": {}}',
        encoding="utf-8",
    )
    m = _get_module()
    np.random.seed(42)
    X = np.random.rand(40, 5).astype(np.float32)
    Y = np.random.rand(40, 5).astype(np.float32)
    out_dir = str(tmp_path)
    minimal_pipe = _minimal_batting_pipeline()
    minimal_pipe.fit(X, Y)
    with patch("ml.tuning.runners._run_search_two_phase", return_value=(minimal_pipe, {}, {"best_algorithm": "rf"})):
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
            rescreen=False,
        )
    assert report.get("best_algorithm") == "rf"
    assert (tmp_path / "batting_scaler_T20.joblib").exists()
    assert (tmp_path / "batting_model_T20.joblib").exists()


def test_normalize_hidden_layer_sizes():
    """_normalize_hidden_layer_sizes accepts tuple, list, JSON string; returns tuple or None."""
    assert _normalize_hidden_layer_sizes((64, 32)) == (64, 32)
    assert _normalize_hidden_layer_sizes([64, 32]) == (64, 32)
    assert _normalize_hidden_layer_sizes("[64, 32]") == (64, 32)
    assert _normalize_hidden_layer_sizes(None) is None
    assert _normalize_hidden_layer_sizes("not-json") is None
    assert _normalize_hidden_layer_sizes([1.5, 2]) == (1, 2)


def test_target_names_for_model():
    """target_names_for_model returns list for batting/bowling, None for unknown."""
    bat = target_names_for_model("batting")
    assert bat is not None and "runs" in bat
    bowl = target_names_for_model("bowling")
    assert bowl is not None and "runs" in bowl
    assert target_names_for_model("unknown_kind") is None


def test_prior_params_to_optuna_regression():
    """_prior_params_to_optuna_regression strips est__ prefix and passes through common keys."""
    out = _prior_params_to_optuna_regression("rf", {"est__estimator__n_estimators": 50, "max_depth": 5})
    assert out["algorithm"] == "rf"
    assert out["n_estimators"] == 50
    assert out["max_depth"] == 5
    out2 = _prior_params_to_optuna_regression("mlp", {"hidden_layer_sizes": [64, 32]})
    assert out2["hidden_layer_sizes"] == (64, 32)
