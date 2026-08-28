"""model-stats carries each model's provenance to the Workbench (ops plan P-2)."""

import json

import pytest

from app.model_stats_service import build_model_stats, enrich_with_provenance

PROVENANCE = {
    "dataset_sha256": "deadbeef",
    "dataset_feed": "all",
    "training_cutoff": "2026-01-01T00:00:00Z",
}


def write_sidecar(tmp_path, name, payload):
    (tmp_path / name).write_text(json.dumps(payload))


# TrainingPipeline writes <kind>_metadata_<fmt>.json; artifact_sidecar writes
# <kind>_meta_<fmt>.json. Two names because two writers, and both must be read.
@pytest.mark.parametrize(
    "sidecar_name",
    ["batting_metadata_T20I.json", "batting_meta_T20I.json"],
)
def test_provenance_is_read_from_either_sidecar(tmp_path, sidecar_name):
    write_sidecar(tmp_path, sidecar_name, {"feature_names": ["a"], "provenance": PROVENANCE})

    rec = {}
    enrich_with_provenance(rec, str(tmp_path), "batting", "T20I")

    assert rec["provenance"]["dataset_sha256"] == "deadbeef"
    assert rec["provenance"]["training_cutoff"] == "2026-01-01T00:00:00Z"


# A model trained before P-1, or from CSVs with no export manifest, records nothing.
# Omission is the answer; an empty block would read as "we looked and found nothing".
def test_no_sidecar_leaves_the_key_absent(tmp_path):
    rec = {}
    enrich_with_provenance(rec, str(tmp_path), "batting", "T20I")
    assert "provenance" not in rec


def test_sidecar_without_provenance_leaves_the_key_absent(tmp_path):
    write_sidecar(tmp_path, "batting_meta_T20I.json", {"feature_names": ["a"]})

    rec = {}
    enrich_with_provenance(rec, str(tmp_path), "batting", "T20I")
    assert "provenance" not in rec


def test_empty_provenance_is_not_attached(tmp_path):
    write_sidecar(tmp_path, "batting_meta_T20I.json", {"provenance": {}})

    rec = {}
    enrich_with_provenance(rec, str(tmp_path), "batting", "T20I")
    assert "provenance" not in rec


def test_unreadable_sidecar_is_not_fatal(tmp_path):
    (tmp_path / "batting_meta_T20I.json").write_text("{not json")

    rec = {}
    enrich_with_provenance(rec, str(tmp_path), "batting", "T20I")
    assert "provenance" not in rec


def test_the_first_readable_sidecar_wins(tmp_path):
    """TrainingPipeline's metadata is preferred: it is written by the run that trained."""
    write_sidecar(tmp_path, "batting_metadata_T20I.json", {"provenance": {"dataset_sha256": "from-pipeline"}})
    write_sidecar(tmp_path, "batting_meta_T20I.json", {"provenance": {"dataset_sha256": "from-sidecar"}})

    rec = {}
    enrich_with_provenance(rec, str(tmp_path), "batting", "T20I")
    assert rec["provenance"]["dataset_sha256"] == "from-pipeline"


def test_build_model_stats_includes_provenance(tmp_path):
    """The whole path: an artifact on disk, its sidecar, and the record the API returns."""
    (tmp_path / "batting_model_T20I.joblib").write_bytes(b"x" * 512)
    write_sidecar(tmp_path, "batting_metadata_T20I.json", {"provenance": PROVENANCE})

    stats = build_model_stats(str(tmp_path))
    models = {m["model_name"]: m for m in stats["models"]}

    assert models, "the artifact should have produced a record"
    record = next(iter(models.values()))
    assert record["provenance"]["dataset_sha256"] == "deadbeef"


def test_build_model_stats_without_provenance(tmp_path):
    (tmp_path / "batting_model_T20I.joblib").write_bytes(b"x" * 512)

    stats = build_model_stats(str(tmp_path))
    record = stats["models"][0]
    assert "provenance" not in record
