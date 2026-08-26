"""End-to-end check that a real trainer publishes the milestones O-2 promises.

The unit tests above prove `training_progress` builds correct events. These prove the
trainers actually *call* it — the failure mode being a perfectly correct emitter that
nothing invokes, which no unit test would notice.
"""

import numpy as np
import pytest

from ml import run_progress
from ml import training_progress as tp
from ml.train_batting import BATTING_SPEC
from ml.training_pipeline import TrainingPipeline


@pytest.fixture
def captured():
    events = []
    run_progress.set_progress_file(None, "run-test")
    run_progress.set_callback(events.append)
    tp.set_total_formats(1)
    yield events
    run_progress.set_callback(None)
    run_progress.set_progress_file(None, "")


def phases_seen(events):
    return {e.get("phase") for e in events}


def test_train_and_save_reports_fitting_and_artifacts(captured, tmp_path):
    """train_and_save is shared by batting, bowling and fielding, so one test covers three."""
    rng = np.random.default_rng(0)
    n_features = len(BATTING_SPEC.feature_cols)
    X = rng.normal(size=(60, n_features))
    Y = rng.normal(size=(60, len(BATTING_SPEC.target_cols)))

    pipeline = TrainingPipeline(BATTING_SPEC)
    pipeline.train_and_save(
        X,
        Y,
        str(tmp_path),
        {"n_estimators": 3, "max_depth": 2, "random_state": 0, "joblib_compress": 0},
        suffix="T20I",
        metadata={"feature_names": list(BATTING_SPEC.feature_cols)},
    )

    assert tp.PHASE_FIT in phases_seen(captured), "the long silent part of a run must announce itself"
    assert tp.PHASE_ARTIFACT in phases_seen(captured)

    artifact_events = [e for e in captured if e.get("phase") == tp.PHASE_ARTIFACT]
    assert len(artifact_events) == 1
    written = artifact_events[0]["artifacts"]
    names = sorted(a["path"] for a in written)
    assert names == ["batting_model_T20I.joblib", "batting_scaler_T20I.joblib"]
    # Size is the postcondition worth checking: a model file of a few hundred bytes is
    # a failed fit that reported success.
    assert all(a["bytes"] > 0 for a in written)
    assert artifact_events[0]["step"] == "train_batting"


def test_fitting_event_carries_the_data_shape(captured, tmp_path):
    rng = np.random.default_rng(1)
    n_features = len(BATTING_SPEC.feature_cols)
    X = rng.normal(size=(40, n_features))
    Y = rng.normal(size=(40, len(BATTING_SPEC.target_cols)))

    TrainingPipeline(BATTING_SPEC).train_and_save(
        X,
        Y,
        str(tmp_path),
        {"n_estimators": 2, "max_depth": 2, "random_state": 0, "joblib_compress": 0},
        suffix="ODI",
        metadata={"feature_names": list(BATTING_SPEC.feature_cols)},
    )

    fit = next(e for e in captured if e.get("phase") == tp.PHASE_FIT)
    assert fit["metrics"] == {"rows": 40, "features": n_features}
    assert fit["format"] == "ODI"


# The plan singles this out: the trainers all called drop_low_variance_columns and all
# discarded the result, so a feature that had gone silently constant vanished without
# anyone being told.
#
# This is a source check rather than a behavioural one, deliberately. Reaching every
# drop site behaviourally needs a valid row schema per trainer -- four different ones --
# and a test that stubs its way there passes vacuously when the fixture fails to build.
# What actually needs guarding is that no *new* drop site appears without an emit
# beside it, and that is a property of the source.
DROP_CALL = "drop_low_variance_columns("
EMIT_CALL = "columns_dropped("


@pytest.mark.parametrize(
    "path",
    [
        "ml/train_win.py",
        "ml/train_extras.py",
        "ml/train_innings.py",
        "ml/train_fielding.py",
    ],
)
def test_every_low_variance_drop_is_reported(path):
    lines = open(path, encoding="utf-8").read().splitlines()

    drop_sites = [
        i
        for i, line in enumerate(lines)
        # Skip the import line and the emit's own call.
        if DROP_CALL in line and not line.strip().startswith(("from ", "import "))
    ]
    assert drop_sites, f"{path}: expected at least one low-variance drop"

    for i in drop_sites:
        window = "\n".join(lines[i : i + 3])
        assert EMIT_CALL in window, (
            f"{path}:{i + 1} drops low-variance columns without reporting them; "
            "a silently constant feature would vanish unnoticed"
        )


def test_the_shared_pipeline_reports_through_train_and_save():
    """batting, bowling and fielding go through TrainingPipeline, which emits for them."""
    source = open("ml/training_pipeline.py", encoding="utf-8").read()
    for call in ("fitting(", "artifact_written(", "data_loaded(", "format_done("):
        assert call in source, f"training_pipeline no longer emits {call}"


def test_every_trainer_imports_the_emitter():
    """A trainer that never imports training_progress cannot possibly report anything."""
    import importlib

    for module_name in (
        "ml.train_batting",
        "ml.train_bowling",
        "ml.train_fielding",
        "ml.train_extras",
        "ml.train_win",
        "ml.train_innings",
    ):
        module = importlib.import_module(module_name)
        source = open(module.__file__, encoding="utf-8").read()
        assert "training_progress" in source or "training_pipeline" in source, (
            f"{module_name} publishes no progress at all"
        )
