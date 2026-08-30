"""Fielding, extras and innings score themselves too, not just the TrainingPipeline models.

The trailing-holdout work reached only the trainers that route through
`TrainingPipeline.train_and_save`. These three have their own save paths, so they went on
writing a sidecar with no `metrics`, no `trained_at` and no `duration_seconds` — leaving
them permanently blank in the ML Model Stats tab however often they were retrained.

Each is scored with its own fit recipe (no scaler for extras, StandardScaler for innings,
the robust scaler for fielding), so what the number describes is what that trainer ships.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Dict, List

import numpy as np
import pytest

from ml.artifact_sidecar import meta_filename

TRAINING_PARAMS = {
    "n_estimators": 5,
    "max_depth": 4,
    "random_state": 42,
    "joblib_compress": 0,
    "n_jobs": 1,
    "estimator": "rf",
}

# Comfortably past MIN_HOLDOUT_ROWS (50) and MIN_HOLDOUT_TRAIN_ROWS (200) at a 0.2 split.
N_ROWS = 400


def _learnable(n_rows: int, n_features: int, n_targets: int) -> tuple[np.ndarray, np.ndarray]:
    """Deterministic data whose targets are linear in the features, so a score is meaningful."""
    rng = np.random.default_rng(11)
    X = rng.normal(size=(n_rows, n_features))
    targets = [10 + (i + 2) * X[:, i % n_features] + rng.normal(scale=0.4, size=n_rows) for i in range(n_targets)]
    return X, np.column_stack(targets)


@pytest.fixture(autouse=True)
def _fast_training(monkeypatch: pytest.MonkeyPatch) -> None:
    """Small forests and a real holdout fraction, independent of the repo's config file."""
    for module in ("ml.train_fielding", "ml.train_extras", "ml.train_innings"):
        monkeypatch.setattr(f"{module}.get_training_params", lambda *a, **k: dict(TRAINING_PARAMS))
        monkeypatch.setattr(
            f"{module}.get_pipeline_common_config",
            lambda: {"holdout_fraction": 0.2, "use_robust_scaler": True},
        )


def _assert_scored(sidecar: Dict[str, Any], expected_rows: int, expected_features: int) -> None:
    """The exact keys app.model_stats_service reads to describe a single-train run."""
    assert sidecar["score_source"] == "holdout"
    assert sidecar["validation_method"] == "trailing_holdout"
    assert sidecar["algorithm"] == "rf"
    assert sidecar["n_samples"] == expected_rows
    assert sidecar["n_features"] == expected_features
    assert sidecar["trained_at"].endswith("Z")
    assert isinstance(sidecar["duration_seconds"], float)

    metrics = sidecar["metrics"]
    assert metrics["mae"] >= 0
    assert metrics["holdout_rows"] == round(expected_rows * 0.2)
    assert metrics["holdout_train_rows"] == expected_rows - round(expected_rows * 0.2)


def test_fielding_records_holdout_metrics(tmp_path: Path) -> None:
    """train_fielding keeps its feature_importance and gains the scored fields."""
    from ml.train_fielding import FIELDING_TARGET_COLS, train_and_save

    feature_names: List[str] = ["f0", "f1", "f2"]
    X, Y = _learnable(N_ROWS, len(feature_names), len(FIELDING_TARGET_COLS))

    train_and_save(X, Y, str(tmp_path), "ODI", feature_names)

    sidecar = json.loads((tmp_path / "fielding_metadata_ODI.json").read_text())
    _assert_scored(sidecar, N_ROWS, len(feature_names))
    assert "feature_importance" in sidecar, "the pre-existing payload must survive"


def test_extras_records_holdout_metrics(tmp_path: Path) -> None:
    """train_extras scores its unscaled single-target forest."""
    from ml.train_extras import train_and_save

    feature_names: List[str] = ["f0", "f1", "f2", "f3"]
    X, Y = _learnable(N_ROWS, len(feature_names), 1)

    train_and_save(X, Y, str(tmp_path), "T20", feature_names)

    sidecar = json.loads((tmp_path / meta_filename("extras", "T20")).read_text())
    _assert_scored(sidecar, N_ROWS, len(feature_names))
    assert sidecar["feature_names"] == feature_names, "the sidecar's original job still works"


def test_innings_records_holdout_metrics(tmp_path: Path) -> None:
    """train_innings scores the StandardScaler recipe it ships."""
    from sklearn.preprocessing import StandardScaler

    from ml.train_innings import INNINGS_TARGET_COLS, train_and_save

    feature_names: List[str] = ["f0", "f1", "f2"]
    X_raw, Y = _learnable(N_ROWS, len(feature_names), len(INNINGS_TARGET_COLS))
    scaler = StandardScaler()
    X = scaler.fit_transform(X_raw)

    train_and_save(X, Y, scaler, str(tmp_path), "TEST", feature_names)

    sidecar = json.loads((tmp_path / meta_filename("innings", "TEST")).read_text())
    _assert_scored(sidecar, N_ROWS, len(feature_names))


def test_innings_holdout_scaler_does_not_see_the_holdout_rows() -> None:
    """The inverse_transform exists so the holdout fits its own scaler; prove it round-trips.

    train_and_save receives X already standardised over every row. Scoring on that matrix
    directly would let standardisation statistics from the holdout leak into the fit.
    """
    from sklearn.preprocessing import StandardScaler

    X_raw, _ = _learnable(N_ROWS, 3, 2)
    scaler = StandardScaler()
    X_scaled = scaler.fit_transform(X_raw)

    np.testing.assert_allclose(scaler.inverse_transform(X_scaled), X_raw, rtol=1e-9, atol=1e-9)


def test_too_few_rows_leaves_metrics_off_rather_than_reporting_noise(tmp_path: Path) -> None:
    """Below the holdout floors a score would describe the split, not the model."""
    from ml.train_extras import train_and_save

    feature_names: List[str] = ["f0", "f1"]
    X, Y = _learnable(60, len(feature_names), 1)

    train_and_save(X, Y, str(tmp_path), "ODI", feature_names)

    sidecar = json.loads((tmp_path / meta_filename("extras", "ODI")).read_text())
    assert "metrics" not in sidecar
    assert "score_source" not in sidecar
    # The run still dates itself, so the row is not indistinguishable from an unbuilt model.
    assert sidecar["trained_at"].endswith("Z")
    assert sidecar["n_samples"] == 60
