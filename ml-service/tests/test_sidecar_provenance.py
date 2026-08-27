"""The provenance a trained artifact carries with it (ops plan P-1).

The unit tests for dataset_provenance prove the block is built correctly. These prove
the trainers actually *attach* it — the failure mode being a correct helper that
nothing calls, which no unit test would notice.
"""

import json
from pathlib import Path

import numpy as np
import pytest

from ml import dataset_provenance as dp
from ml.artifact_sidecar import read_artifact_meta, write_artifact_meta
from ml.train_batting import BATTING_SPEC
from ml.training_pipeline import TrainingPipeline

MANIFEST = {
    "v": 1,
    "exported_at": "2026-08-27T12:00:00Z",
    "provenance": {
        "dataset_sha256": "deadbeef",
        "dataset_feed": "all",
        "dataset_match_files": 19998,
    },
}


@pytest.fixture
def export_dir(tmp_path):
    d = tmp_path / "export"
    d.mkdir()
    (d / dp.EXPORT_MANIFEST_NAME).write_text(json.dumps(MANIFEST))
    return d


def test_training_pipeline_stamps_the_sidecar(export_dir, tmp_path):
    """batting, bowling and fielding share train_and_save, so one test covers three."""
    rng = np.random.default_rng(0)
    n_features = len(BATTING_SPEC.feature_cols)
    X = rng.normal(size=(40, n_features))
    Y = rng.normal(size=(40, len(BATTING_SPEC.target_cols)))

    out_dir = tmp_path / "artifacts"
    TrainingPipeline(BATTING_SPEC).train_and_save(
        X,
        Y,
        str(out_dir),
        {"n_estimators": 2, "max_depth": 2, "random_state": 0, "joblib_compress": 0},
        suffix="T20I",
        metadata={
            "feature_names": list(BATTING_SPEC.feature_cols),
            "csv_dir": str(export_dir),
            "cutoff": "2026-01-01T00:00:00Z",
        },
    )

    written = json.loads((out_dir / "batting_metadata_T20I.json").read_text())
    provenance = written["provenance"]
    assert provenance["dataset_sha256"] == "deadbeef"
    assert provenance["dataset_feed"] == "all"
    assert provenance["training_cutoff"] == "2026-01-01T00:00:00Z"
    # Sits beside feature_names because it answers the same kind of question about the
    # artifact: what shape it expects, and what it learned from.
    assert "feature_names" in written


def test_training_pipeline_omits_provenance_it_cannot_establish(tmp_path):
    """CSVs produced before P-1, or by hand, have no manifest."""
    rng = np.random.default_rng(1)
    X = rng.normal(size=(30, len(BATTING_SPEC.feature_cols)))
    Y = rng.normal(size=(30, len(BATTING_SPEC.target_cols)))

    out_dir = tmp_path / "artifacts"
    TrainingPipeline(BATTING_SPEC).train_and_save(
        X,
        Y,
        str(out_dir),
        {"n_estimators": 2, "max_depth": 2, "random_state": 0, "joblib_compress": 0},
        suffix="ODI",
        metadata={
            "feature_names": list(BATTING_SPEC.feature_cols),
            "csv_dir": str(tmp_path / "no-such-export"),
        },
    )

    written = json.loads((out_dir / "batting_metadata_ODI.json").read_text())
    assert "provenance" not in written, "unknown is omitted, never blanked"


def test_write_artifact_meta_stamps_the_sidecar(export_dir, tmp_path, monkeypatch):
    """extras and innings write through write_artifact_meta, so it stamps for both."""
    monkeypatch.setenv("GO_APP_OUTPUT_DIR", str(export_dir))

    out_dir = tmp_path / "artifacts"
    write_artifact_meta(str(out_dir), "extras", "T20I", ["a", "b"])

    meta = read_artifact_meta(str(out_dir), "extras", "T20I")
    assert meta is not None
    assert meta["provenance"]["dataset_sha256"] == "deadbeef"
    assert meta["feature_names"] == ["a", "b"]


def test_write_artifact_meta_without_a_manifest(tmp_path, monkeypatch):
    monkeypatch.setenv("GO_APP_OUTPUT_DIR", str(tmp_path / "empty"))

    out_dir = tmp_path / "artifacts"
    write_artifact_meta(str(out_dir), "innings", "ODI", ["a"])

    meta = read_artifact_meta(str(out_dir), "innings", "ODI")
    assert meta is not None
    assert "provenance" not in meta


# A correct helper that nothing calls would pass every unit test above.
def test_every_trainer_reaches_a_stamping_path():
    for path, expected in {
        "ml/training_pipeline.py": "attach_provenance",
        "ml/train_win.py": "attach_provenance",
        "ml/artifact_sidecar.py": "provenance_for",
    }.items():
        source = Path(path).read_text(encoding="utf-8")
        assert expected in source, f"{path} writes a sidecar without provenance"
