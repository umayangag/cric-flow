"""Tests for ml.dataset_provenance — carrying the dataset digest into artifacts.

A CSV on disk says nothing about the dataset behind its rows, so a model trained from
one carries no way back to it. go-app writes an export manifest beside the CSVs; this
module reads it and shapes it for a model's sidecar (ops plan P-1).
"""

import json

import pytest

from ml import dataset_provenance as dp


@pytest.fixture
def export_dir(tmp_path):
    """An export directory with a manifest, as go-app's exporter leaves it."""
    (tmp_path / dp.EXPORT_MANIFEST_NAME).write_text(
        json.dumps(
            {
                "v": 1,
                "exported_at": "2026-08-27T12:00:00Z",
                "formats": ["T20I", "ODI"],
                "unified": True,
                "files": [{"name": "batting_encoded_T20I.csv", "bytes": 4096}],
                "provenance": {
                    "dataset_sha256": "deadbeef",
                    "dataset_source_url": "https://cricsheet.org/downloads/all_json.zip",
                    "dataset_feed": "all",
                    "dataset_extracted_at": "2026-08-26T10:00:00Z",
                    "dataset_match_files": 19998,
                },
            }
        )
    )
    return str(tmp_path)


def test_provenance_carries_the_dataset_identity(export_dir):
    got = dp.provenance_for(export_dir)

    assert got["dataset_sha256"] == "deadbeef"
    assert got["dataset_feed"] == "all"
    assert got["dataset_match_files"] == 19998
    assert got["exported_at"] == "2026-08-27T12:00:00Z"


# Two models trained from the same export with different cutoffs are different models,
# and nothing else on disk says so.
def test_cutoff_is_included_because_nothing_else_records_it(export_dir):
    got = dp.provenance_for(export_dir, cutoff="2026-01-01T00:00:00Z")
    assert got["training_cutoff"] == "2026-01-01T00:00:00Z"

    without = dp.provenance_for(export_dir)
    assert "training_cutoff" not in without, "absent is not the same as empty"


# A missing manifest is a normal state: CSVs produced before P-1, or by hand, have
# none. Returning {} lets the caller record "unknown" — the answer P-2 flags — rather
# than an object full of nulls that reads as "we looked and found nothing".
def test_no_manifest_yields_nothing_rather_than_blanks(tmp_path):
    assert dp.provenance_for(str(tmp_path)) == {}
    assert dp.read_export_manifest(str(tmp_path)) == {}


def test_unreadable_manifest_is_not_fatal(tmp_path):
    (tmp_path / dp.EXPORT_MANIFEST_NAME).write_text("{not json")
    assert dp.provenance_for(str(tmp_path)) == {}

    (tmp_path / dp.EXPORT_MANIFEST_NAME).write_text("[1, 2, 3]")
    assert dp.provenance_for(str(tmp_path)) == {}, "a JSON array is not a manifest"


def test_a_manifest_with_unknown_provenance_yields_only_the_export_time(tmp_path):
    """An export from a data directory populated by hand knows when, not what."""
    (tmp_path / dp.EXPORT_MANIFEST_NAME).write_text(
        json.dumps({"v": 1, "exported_at": "2026-08-27T12:00:00Z", "provenance": {}})
    )

    got = dp.provenance_for(str(tmp_path))
    assert got == {"exported_at": "2026-08-27T12:00:00Z"}
    assert "dataset_sha256" not in got


# Copying the manifest wholesale would mean a future manifest field silently appearing
# in every artifact ever written afterwards.
def test_only_the_listed_fields_are_copied(tmp_path):
    (tmp_path / dp.EXPORT_MANIFEST_NAME).write_text(
        json.dumps(
            {
                "v": 1,
                "provenance": {"dataset_sha256": "abc", "some_future_field": "should not travel"},
            }
        )
    )

    got = dp.provenance_for(str(tmp_path))
    assert got == {"dataset_sha256": "abc"}


def test_attach_adds_the_block_to_metadata(export_dir):
    metadata = {"feature_names": ["a", "b"]}
    dp.attach(metadata, csv_dir=export_dir, cutoff="2026-01-01T00:00:00Z")

    assert metadata["feature_names"] == ["a", "b"], "existing keys are untouched"
    assert metadata["provenance"]["dataset_sha256"] == "deadbeef"
    assert metadata["provenance"]["training_cutoff"] == "2026-01-01T00:00:00Z"


def test_attach_omits_the_key_when_nothing_is_known(tmp_path):
    metadata = {"feature_names": ["a"]}
    dp.attach(metadata, csv_dir=str(tmp_path))

    assert "provenance" not in metadata, "an empty block reads as 'we looked', not 'nobody looked'"


# Provenance is descriptive. A model that trained successfully must not fail to save
# because its manifest was unreadable.
def test_attach_never_raises(monkeypatch):
    def boom(*_args, **_kwargs):
        raise RuntimeError("filesystem on fire")

    monkeypatch.setattr(dp, "provenance_for", boom)
    metadata = {"feature_names": ["a"]}
    dp.attach(metadata, csv_dir="/anywhere")

    assert metadata == {"feature_names": ["a"]}


def test_manifest_path_is_beside_the_csvs(tmp_path):
    assert dp.export_manifest_path(str(tmp_path)).endswith(dp.EXPORT_MANIFEST_NAME)
    assert dp.export_manifest_path(str(tmp_path)).startswith(str(tmp_path))
