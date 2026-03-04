"""Tests for ml.tuning.consistency_tuning."""

from unittest.mock import patch

from ml.tuning.consistency_tuning import augment_tuning_report_with_consistency


def test_augment_tuning_report_with_consistency_adds_penalty_and_combined_when_enabled():
    report = {
        "best_cv_score": -3.5,
        "consistency_metrics": {
            "mean_abs_pct_delta_runs": 10.0,
            "mean_abs_pct_delta_wickets": 5.0,
        },
    }
    with patch("ml.tuning.consistency_tuning.get_consistency_regularization_config") as get_cfg:
        get_cfg.return_value = {"enabled": True, "lambda_runs": 0.01, "lambda_wickets": 0.01}
        augment_tuning_report_with_consistency(report, "batting", "T20")
    assert "consistency_penalty" in report
    assert report["consistency_penalty"] == 0.15  # 0.01*10 + 0.01*5
    assert report["base_mae"] == 3.5
    assert report["score_combined_mae"] == 3.5 + 0.15


def test_augment_tuning_report_with_consistency_no_op_when_disabled():
    report = {"best_cv_score": -3.5, "consistency_metrics": {"mean_abs_pct_delta_runs": 10.0}}
    with patch("ml.tuning.consistency_tuning.get_consistency_regularization_config") as get_cfg:
        get_cfg.return_value = {"enabled": False, "lambda_runs": 0.01, "lambda_wickets": 0.01}
        augment_tuning_report_with_consistency(report, "batting", None)
    assert "consistency_penalty" not in report
    assert "score_combined_mae" not in report


def test_augment_tuning_report_with_consistency_no_op_when_no_consistency_metrics():
    report = {"best_cv_score": -3.5}
    with patch("ml.tuning.consistency_tuning.get_consistency_regularization_config") as get_cfg:
        get_cfg.return_value = {"enabled": True, "lambda_runs": 0.01, "lambda_wickets": 0.01}
        augment_tuning_report_with_consistency(report, "innings", "ODI")
    assert "consistency_penalty" not in report
