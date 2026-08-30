"""A single-train run scores itself on a trailing holdout and records it on the artifact.

Without this a Mode A retrain reported only its file size, so a model trained from a
broken export looked exactly like a good one in the ML Model Stats tab.
"""

import json

import numpy as np
import pytest

from ml.training_pipeline import ModelSpec, TrainingPipeline

FEATURE_COLS = ["f0", "f1", "f2"]
TARGET_COLS = ["runs", "balls"]

TRAINING_PARAMS = {
    "n_estimators": 5,
    "max_depth": 4,
    "random_state": 42,
    "joblib_compress": 0,
    "n_jobs": 1,
    "estimator": "rf",
}


@pytest.fixture
def spec() -> ModelSpec:
    return ModelSpec(
        name="batting",
        feature_cols=list(FEATURE_COLS),
        target_cols=list(TARGET_COLS),
        artifact_prefix="batting",
    )


def make_dataset(n_rows: int):
    """Learnable, deterministic data: targets are linear in the features plus noise."""
    rng = np.random.default_rng(7)
    X = rng.normal(size=(n_rows, len(FEATURE_COLS)))
    runs = 20 + 5 * X[:, 0] + rng.normal(scale=0.5, size=n_rows)
    balls = 15 + 3 * X[:, 1] + rng.normal(scale=0.5, size=n_rows)
    return X, np.column_stack([runs, balls])


def test_holdout_scores_the_last_rows_and_reports_the_split(spec):
    pipeline = TrainingPipeline(spec)
    X, Y = make_dataset(1000)

    metrics = pipeline.evaluate_holdout(X, Y, TRAINING_PARAMS, holdout_fraction=0.2)

    assert metrics["holdout_rows"] == 200
    assert metrics["holdout_train_rows"] == 800
    assert metrics["mae"] > 0
    assert set(metrics["per_target_mae"]) == {"mae_runs", "mae_balls"}


def test_holdout_beats_the_naive_baseline_on_learnable_data(spec):
    pipeline = TrainingPipeline(spec)
    X, Y = make_dataset(1000)

    metrics = pipeline.evaluate_holdout(X, Y, TRAINING_PARAMS, holdout_fraction=0.2)

    assert metrics["baseline_improvement_pct"] > 0


@pytest.mark.parametrize(
    "n_rows,fraction",
    [
        (1000, 0.0),  # explicitly disabled
        (100, 0.2),  # holdout slice below MIN_HOLDOUT_ROWS
        (200, 0.5),  # holdout slice big enough, training slice below MIN_HOLDOUT_TRAIN_ROWS
    ],
)
def test_holdout_is_omitted_rather_than_guessed(spec, n_rows, fraction):
    pipeline = TrainingPipeline(spec)
    X, Y = make_dataset(n_rows)

    assert pipeline.evaluate_holdout(X, Y, TRAINING_PARAMS, holdout_fraction=fraction) is None


def test_holdout_is_scored_against_unclipped_targets(spec):
    """Clipping is part of the recipe under test, so it must not touch the answer key."""
    pipeline = TrainingPipeline(spec)
    X, Y = make_dataset(1000)
    # One extreme value in the holdout slice that aggressive clipping would hide.
    Y[-1, 0] = 5000.0
    params = {**TRAINING_PARAMS, "target_clip_percentile": 50.0}

    metrics = pipeline.evaluate_holdout(X, Y, params, holdout_fraction=0.2)

    assert metrics["max_error"] > 1000


def test_train_and_save_writes_metrics_and_timing_to_the_sidecar(spec, tmp_path):
    pipeline = TrainingPipeline(spec)
    X, Y = make_dataset(1000)

    pipeline.train_and_save(
        X,
        Y,
        str(tmp_path),
        TRAINING_PARAMS,
        suffix="T20I",
        metadata={"feature_names": list(FEATURE_COLS)},
    )

    sidecar = json.loads((tmp_path / "batting_metadata_T20I.json").read_text())
    assert sidecar["score_source"] == "holdout"
    assert sidecar["validation_method"] == "trailing_holdout"
    assert sidecar["metrics"]["mae"] > 0
    assert sidecar["algorithm"] == "rf"
    assert sidecar["n_samples"] == 1000
    assert sidecar["trained_at"].endswith("Z")
    assert sidecar["duration_seconds"] >= 0


def test_train_and_save_still_ships_artifacts_when_the_holdout_is_skipped(spec, tmp_path):
    """A dataset too small to score is still a dataset worth training on."""
    pipeline = TrainingPipeline(spec)
    X, Y = make_dataset(100)

    pipeline.train_and_save(
        X,
        Y,
        str(tmp_path),
        TRAINING_PARAMS,
        suffix="T20I",
        metadata={"feature_names": list(FEATURE_COLS)},
    )

    sidecar = json.loads((tmp_path / "batting_metadata_T20I.json").read_text())
    assert (tmp_path / "batting_model_T20I.joblib").exists()
    assert "metrics" not in sidecar
    assert "score_source" not in sidecar
    assert sidecar["trained_at"].endswith("Z")
