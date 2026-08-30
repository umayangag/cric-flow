"""model-stats reports what a single-train run measured, labelled as a holdout.

Previously a model with no tuning report showed only its size and mtime, so the whole
Mode A path was invisible in the ML Model Stats tab.
"""

import json

import pytest

from app.model_stats_service import build_model_stats, enrich_with_training_metrics

HOLDOUT_METRICS = {
    "mae": 6.5,
    "rmse": 9.1,
    "r2_pct": 41.2,
    "per_target_mae": {"mae_runs": 7.1, "mae_balls": 5.9},
    "holdout_rows": 200,
}

SIDECAR = {
    "metrics": HOLDOUT_METRICS,
    "score_source": "holdout",
    "validation_method": "trailing_holdout",
    "algorithm": "rf",
    "n_samples": 1000,
    "n_features": 30,
    "trained_at": "2026-08-30T10:00:00Z",
    "duration_seconds": 42.5,
}


def write_sidecar(tmp_path, name, payload):
    (tmp_path / name).write_text(json.dumps(payload))


def test_holdout_metrics_reach_the_table(tmp_path):
    write_sidecar(tmp_path, "batting_metadata_T20I.json", SIDECAR)

    rec = {"tuned": False}
    enrich_with_training_metrics(rec, str(tmp_path), "batting", "T20I")

    assert rec["accuracy_display"] == "MAE=6.5, RMSE=9.1, R²=41.2%"
    assert rec["score_source"] == "holdout"
    assert rec["algorithm"] == "Random Forest"
    assert rec["trained_at"] == "2026-08-30T10:00:00Z"
    assert rec["duration_seconds"] == 42.5


def test_nested_metrics_are_flattened_for_the_details_panel(tmp_path):
    write_sidecar(tmp_path, "batting_metadata_T20I.json", SIDECAR)

    rec = {"tuned": False}
    enrich_with_training_metrics(rec, str(tmp_path), "batting", "T20I")

    assert rec["metrics"]["mae_runs"] == 7.1
    assert rec["metrics"]["mae_balls"] == 5.9


# A cross-validated score measured over the whole dataset is the stronger measurement.
# Overwriting it with one slice's holdout would be a downgrade, not an update.
def test_a_tuning_report_is_never_overwritten_by_a_holdout(tmp_path):
    write_sidecar(tmp_path, "batting_metadata_T20I.json", SIDECAR)

    rec = {"tuned": True, "accuracy_display": "MAE=5.10", "score_source": "tuning_cv"}
    enrich_with_training_metrics(rec, str(tmp_path), "batting", "T20I")

    assert rec["accuracy_display"] == "MAE=5.10"
    assert rec["score_source"] == "tuning_cv"


# train_win writes its walk-forward scores under cv_metrics, not metrics — two writers,
# two key names, and the tab has to read both.
def test_win_walk_forward_metrics_are_read_from_cv_metrics(tmp_path):
    write_sidecar(
        tmp_path,
        "win_meta_T20I.json",
        {"cv_metrics": {"accuracy_pct": 63.4}, "algorithm": "gb"},
    )

    rec = {"tuned": False}
    enrich_with_training_metrics(rec, str(tmp_path), "win", "T20I")

    assert rec["accuracy_display"] == "63.4%"
    assert rec["algorithm"] == "Gradient Boosting"


@pytest.mark.parametrize(
    "payload",
    [
        {"feature_names": ["a"]},  # sidecar from before holdout scoring existed
        {"metrics": {}},  # holdout ran but could not be scored
    ],
)
def test_an_unscored_model_reports_nothing_rather_than_zero(tmp_path, payload):
    write_sidecar(tmp_path, "batting_metadata_T20I.json", payload)

    rec = {"tuned": False}
    enrich_with_training_metrics(rec, str(tmp_path), "batting", "T20I")

    assert "accuracy_display" not in rec
    assert "score_source" not in rec


def test_build_model_stats_labels_an_untuned_model_as_holdout(tmp_path):
    (tmp_path / "batting_model_T20I.joblib").write_bytes(b"x")
    write_sidecar(tmp_path, "batting_metadata_T20I.json", SIDECAR)

    models = build_model_stats(str(tmp_path))["models"]

    assert len(models) == 1
    assert models[0]["tuned"] is False
    assert models[0]["score_source"] == "holdout"
    assert models[0]["accuracy_display"] == "MAE=6.5, RMSE=9.1, R²=41.2%"
