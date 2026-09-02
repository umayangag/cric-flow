"""Unit tests for ml.auto_tune helper functions and data loading.

Tests in this file must NOT run real model training (no Optuna/search, no fitting
multi-estimator pipelines). Use mocks for run_auto_tune* entry points. Any new
test that would run real training belongs in a separate module (e.g. behind
RUN_AUTO_TUNE_SMOKE=1) so CI stays fast.
"""

import os
from unittest.mock import patch

import numpy as np
import pytest
from sklearn.ensemble import RandomForestClassifier, RandomForestRegressor
from sklearn.model_selection import KFold, TimeSeriesSplit
from sklearn.multioutput import MultiOutputRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from ml.tuning.cv_metrics import (
    _add_final_report_details,
    _compute_metrics_classification,
    _compute_mlqa_audit,
    _effective_n_jobs,
    _effective_timeseries_gap,
    _get_cv_object,
)
from ml.tuning.optuna_search import _count_combinations, _save_artifacts_model_only
from ml.tuning.runners import run_auto_tune_win
from ml.tuning.search_space import (
    _get_prior_tuned_algorithm,
    _normalize_hidden_layer_sizes,
    _phase1_candidates_classification,
    _prior_params_to_optuna_params,
    _search_space_classification,
)
from ml.tuning.types import AVAILABLE_ALGORITHMS


def _single_output_pipeline(estimator):
    """The shape every scorer here is handed: one scaler, one estimator.

    The MLQA audit and the report details are model-agnostic -- they read a fitted
    pipeline, not a model kind -- so a small regressor is the cheapest fixture for them.
    """
    return Pipeline([("scaler", StandardScaler()), ("est", estimator)])


def test_available_algorithms_non_empty():
    """AVAILABLE_ALGORITHMS contains expected algorithm keys."""
    assert len(AVAILABLE_ALGORITHMS) > 0
    assert "rf" in AVAILABLE_ALGORITHMS
    assert "gb" in AVAILABLE_ALGORITHMS
    assert "et" in AVAILABLE_ALGORITHMS
    assert "hgb" in AVAILABLE_ALGORITHMS


def test_get_cv_object_kfold():
    """_get_cv_object returns KFold for kfold validation."""
    cv = _get_cv_object("kfold", cv_splits=5, n_samples=100)
    assert isinstance(cv, KFold)
    assert cv.n_splits == 5


def test_get_cv_object_walk_forward():
    """_get_cv_object returns TimeSeriesSplit for walk_forward with enough samples."""
    cv = _get_cv_object("walk_forward", cv_splits=5, n_samples=50)
    assert isinstance(cv, TimeSeriesSplit)


def test_effective_timeseries_gap():
    """_effective_timeseries_gap caps gap for small datasets (win, extras)."""
    assert _effective_timeseries_gap(200, 2007) == 0  # win: small dataset, no gap
    assert _effective_timeseries_gap(200, 4000) == 0  # below 5000
    assert _effective_timeseries_gap(200, 23862) == 200  # bowling: use full gap
    assert _effective_timeseries_gap(500, 10000) == 500  # cap at n/20=500
    assert _effective_timeseries_gap(500, 5000) == 250  # cap at 5000/20=250
    assert _effective_timeseries_gap(0, 10000) == 0


def test_get_cv_object_tiny_samples_raises():
    """_get_cv_object raises for fewer than 2 samples."""
    with pytest.raises(ValueError, match="at least 2 samples"):
        _get_cv_object("kfold", cv_splits=5, n_samples=1)


def test_get_cv_object_tiny_samples_kfold():
    """_get_cv_object uses KFold for very small datasets with walk_forward."""
    cv = _get_cv_object("walk_forward", cv_splits=5, n_samples=2)
    assert isinstance(cv, KFold)


def test_effective_n_jobs_override():
    """_effective_n_jobs returns override when provided."""
    assert _effective_n_jobs({"n_jobs": 1}, n_jobs_override=4) == 4


def test_effective_n_jobs_from_config():
    """_effective_n_jobs returns config value when no override."""
    assert _effective_n_jobs({"n_jobs": 3}) == 3


def test_effective_n_jobs_from_env():
    """_effective_n_jobs uses AUTO_TUNE_N_JOBS when set."""
    with patch.dict(os.environ, {"AUTO_TUNE_N_JOBS": "8"}):
        assert _effective_n_jobs({"n_jobs": 1}) == 8


def test_effective_n_jobs_invalid_env_falls_back():
    """_effective_n_jobs falls back to config when env is invalid."""
    with patch.dict(os.environ, {"AUTO_TUNE_N_JOBS": "abc"}):
        assert _effective_n_jobs({"n_jobs": 5}) == 5


def test_count_combinations():
    """_count_combinations computes product of param list lengths."""
    d = {"a": [1, 2, 3], "b": [10, 20]}
    assert _count_combinations(d) == 6
    assert _count_combinations({}) == 1
    assert _count_combinations({"x": [1]}) == 1


def test_compute_metrics_classification_minimal():
    """_compute_metrics_classification returns accuracy, precision, recall, f1."""
    pipe = _minimal_model_only_pipeline(regression=False)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).randint(0, 2, 50)
    pipe.fit(X, y)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    metrics = _compute_metrics_classification(pipe, X, y, cv)
    assert "accuracy" in metrics
    assert "accuracy_pct" in metrics
    assert "precision" in metrics
    assert "recall" in metrics
    assert "f1" in metrics


def test_compute_mlqa_audit_missing_score_returns_warning():
    """_compute_mlqa_audit returns WARNING when report has no best_cv_score."""
    pipe = _minimal_model_only_pipeline(regression=True)
    X = np.random.RandomState(42).rand(30, 5)
    y = np.random.RandomState(43).rand(30)
    pipe.fit(X, y)
    cv = KFold(n_splits=2, shuffle=True, random_state=42)
    audit = _compute_mlqa_audit({}, pipe, X, y, cv, "neg_mean_absolute_error", "regression")
    assert audit["audit_status"] == "WARNING"
    assert "Missing validation score" in audit["key_findings"][0]


def test_compute_mlqa_audit_with_fairness_metrics():
    """_compute_mlqa_audit includes fairness check when report has fairness_metrics."""
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = _single_output_pipeline(est)
    X = np.random.RandomState(42).rand(80, 5)
    y = np.random.RandomState(43).rand(80)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {
        "best_cv_score": -0.4,
        "fairness_metrics": {"disparate_impact_ratio": 0.95},
    }
    audit = _compute_mlqa_audit(
        report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", ["f0", "f1", "f2", "f3", "f4"]
    )
    assert "Fairness" in " ".join(audit["key_findings"]) or "fairness" in audit["bias_report"].lower()


def test_add_final_report_details_adds_mlqa_and_feature_importance():
    """_add_final_report_details adds mlqa_audit and feature_importance to report."""
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = _single_output_pipeline(est)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).rand(50)
    pipe.fit(X, y)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {"best_cv_score": -0.35}
    _add_final_report_details(report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", "batting")
    assert "mlqa_audit" in report
    assert "feature_importance" in report
    assert len(report["feature_importance"]) <= 5


def test_compute_mlqa_audit():
    """_compute_mlqa_audit returns audit_status, key_findings, bias_report, final_verdict."""
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = _single_output_pipeline(est)
    X = np.random.RandomState(42).rand(80, 5)
    y = np.random.RandomState(43).rand(80)
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {
        "best_cv_score": -0.4,
        "candidates": [{"algorithm": "rf", "best_score": -0.4}],
        "algorithms": ["rf"],
    }
    audit = _compute_mlqa_audit(
        report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", ["f0", "f1", "f2", "f3", "f4"]
    )
    assert audit["audit_status"] in ("PASS", "FAIL", "WARNING")
    assert "key_findings" in audit
    assert isinstance(audit["key_findings"], list)
    assert "bias_report" in audit
    assert audit["final_verdict"] in ("Proceed to Deployment", "Rollback & Re-tune")
    if "checks" in audit:
        assert "overfitting" in audit["checks"] or "stability" in audit["checks"]


def test_search_space_classification():
    """_search_space_classification returns candidates for win."""
    cands = _search_space_classification("win")
    assert len(cands) >= 2
    assert cands[0][0] == "rf"
    assert cands[1][0] == "gb"


def test_phase1_candidates_classification():
    """_phase1_candidates_classification returns classification candidates."""
    allow = frozenset({"rf", "gb"})
    cands = _phase1_candidates_classification(allow)
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


def test_run_auto_tune_win_minimal(tmp_path):
    """run_auto_tune_win returns report and writes model artifact; search is mocked to avoid real training."""
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
        report = run_auto_tune_win(
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


def test_save_artifacts_model_only_without_format_suffix_refuses(tmp_path):
    """_save_artifacts_model_only refuses an unsuffixed write, same as _save_artifacts."""
    pipe = _minimal_model_only_pipeline(regression=False)
    pipe.fit(np.random.rand(10, 3), np.random.randint(0, 2, 10))
    with pytest.raises(ValueError, match="missing_format_suffix"):
        _save_artifacts_model_only(pipe, str(tmp_path), "win", None, joblib_compress=0, report={})
    assert list(tmp_path.iterdir()) == []


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


def test_normalize_hidden_layer_sizes():
    """_normalize_hidden_layer_sizes accepts tuple, list, JSON string; returns tuple or None."""
    assert _normalize_hidden_layer_sizes((64, 32)) == (64, 32)
    assert _normalize_hidden_layer_sizes([64, 32]) == (64, 32)
    assert _normalize_hidden_layer_sizes("[64, 32]") == (64, 32)
    assert _normalize_hidden_layer_sizes(None) is None
    assert _normalize_hidden_layer_sizes("not-json") is None
    assert _normalize_hidden_layer_sizes([1.5, 2]) == (1, 2)


def test_prior_params_to_optuna_params():
    """_prior_params_to_optuna_params strips est__ prefix and passes through common keys."""
    out = _prior_params_to_optuna_params("rf", {"est__estimator__n_estimators": 50, "max_depth": 5})
    assert out["algorithm"] == "rf"
    assert out["n_estimators"] == 50
    assert out["max_depth"] == 5
    out2 = _prior_params_to_optuna_params("mlp", {"hidden_layer_sizes": [64, 32]})
    assert out2["hidden_layer_sizes"] == (64, 32)


# ---------------------------------------------------------------------------
# Tests for relative threshold logic (_to_relative, penalized score, audit)
# ---------------------------------------------------------------------------


def test_to_relative_basic():
    """_to_relative computes abs(value)/abs(reference) correctly."""
    from ml.tuning.cv_metrics import _to_relative

    # 0.12 std on a score of -7.9 → ~1.5%
    assert abs(_to_relative(0.12, -7.9) - 0.12 / 7.9) < 1e-9
    # Large delta on large score is small relative
    assert abs(_to_relative(1.0, -20.0) - 0.05) < 1e-9
    # Zero reference with non-zero absolute error → infinite relative error
    assert _to_relative(0.5, 0.0) == float("inf")
    assert _to_relative(0.0, 0.0) == 0.0
    # Positive reference works the same
    assert abs(_to_relative(0.1, 2.0) - 0.05) < 1e-9


def test_to_relative_near_zero_reference():
    """_to_relative treats near-zero reference as inf relative error when absolute is non-zero."""
    from ml.tuning.cv_metrics import _to_relative

    assert _to_relative(0.01, 1e-12) == float("inf")
    assert _to_relative(0.0, 1e-12) == 0.0


def test_penalized_score_uses_relative_thresholds():
    """compute_mlqa_penalized_score uses relative thresholds so large-magnitude scores pass."""
    from ml.tuning.cv_metrics import compute_mlqa_penalized_score

    pipe = _minimal_model_only_pipeline(regression=True)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).rand(50)

    # Simulate a model with mean_score=-8.0, fold_std=0.12, train_score=-7.5
    # Absolute delta=0.5, relative delta=0.5/8.0=6.25% (under 10% threshold)
    # Absolute std=0.12, relative std=0.12/8.0=1.5% (under 8% threshold)
    # Should PASS with relative thresholds
    pass_audit, violation, penalized = compute_mlqa_penalized_score(
        pipe,
        X,
        y,
        "neg_mean_absolute_error",
        mean_score=-8.0,
        fold_std=0.12,
        train_score=-7.5,
    )
    assert pass_audit is True
    assert violation == 0.0
    assert penalized == -8.0


def test_penalized_score_fails_with_high_relative_delta():
    """compute_mlqa_penalized_score fails when relative delta exceeds threshold."""
    from ml.tuning.cv_metrics import compute_mlqa_penalized_score

    pipe = _minimal_model_only_pipeline(regression=True)
    X = np.random.RandomState(42).rand(50, 5)
    y = np.random.RandomState(43).rand(50)

    # mean_score=-2.0, train_score=-0.1 → delta=1.9, relative=1.9/2.0=95% (way over 10%)
    pass_audit, violation, penalized = compute_mlqa_penalized_score(
        pipe,
        X,
        y,
        "neg_mean_absolute_error",
        mean_score=-2.0,
        fold_std=0.01,
        train_score=-0.1,
    )
    assert pass_audit is False
    assert violation > 0.0
    assert penalized < -2.0


def test_mlqa_audit_uses_relative_thresholds():
    """_compute_mlqa_audit with large-magnitude scores passes when relative metrics are small."""
    est = RandomForestRegressor(n_estimators=10, random_state=42)
    pipe = _single_output_pipeline(est)
    X = np.random.RandomState(42).rand(80, 5)
    y = np.random.RandomState(43).rand(80) * 10  # larger target → larger magnitude scores
    cv = KFold(n_splits=3, shuffle=True, random_state=42)
    report = {"best_cv_score": -2.5, "candidates": [{"algorithm": "rf", "best_score": -2.5}]}
    audit = _compute_mlqa_audit(
        report, pipe, X, y, cv, "neg_mean_absolute_error", "regression", ["f0", "f1", "f2", "f3", "f4"]
    )
    # With relative thresholds, the audit checks should include relative info
    if "checks" in audit:
        assert "relative_delta" in audit["checks"]["overfitting"]
        assert "relative_cv_std" in audit["checks"]["stability"]
