"""Tests for ml.artifact_sidecar (per-artifact feature-name + derived-weight persistence)."""

from __future__ import annotations

import json
from pathlib import Path

from ml.artifact_sidecar import meta_filename, read_artifact_meta, write_artifact_meta


def test_meta_filename_per_format() -> None:
    """Per-format artifacts include the format code; legacy/None omit it."""
    assert meta_filename("innings", "T20") == "innings_meta_T20.json"
    assert meta_filename("extras", "T20I") == "extras_meta_T20I.json"
    assert meta_filename("innings", None) == "innings_meta.json"
    assert meta_filename("extras", "_LEGACY_") == "extras_meta.json"


def test_write_then_read_roundtrip(tmp_path: Path) -> None:
    """write_artifact_meta should produce a file that read_artifact_meta returns as dict."""
    feature_names = ["season_id", "venue_id", "form_differential", "format_is_T20"]
    weights = {
        "weather_composite_rain_weight": 0.5,
        "weather_composite_humidity_weight": 0.3,
        "weather_composite_cloud_weight": 0.2,
    }
    write_artifact_meta(str(tmp_path), "innings", "T20", feature_names, derived_weights=weights)

    meta = read_artifact_meta(str(tmp_path), "innings", "T20")
    assert meta is not None
    assert meta["kind"] == "innings"
    assert meta["format_code"] == "T20"
    assert meta["feature_names"] == feature_names
    assert meta["derived_weights"] == weights


def test_write_is_atomic(tmp_path: Path) -> None:
    """After writing, no .tmp file should remain in the directory."""
    write_artifact_meta(str(tmp_path), "extras", None, ["a", "b"])
    assert not any(p.name.endswith(".tmp") for p in tmp_path.iterdir())


def test_read_missing_returns_none(tmp_path: Path) -> None:
    assert read_artifact_meta(str(tmp_path), "innings", "T20") is None


def test_read_invalid_json_returns_none(tmp_path: Path) -> None:
    """Malformed sidecar must not crash the artifact loader."""
    (tmp_path / "innings_meta_T20.json").write_text("not json", encoding="utf-8")
    assert read_artifact_meta(str(tmp_path), "innings", "T20") is None


def test_read_missing_feature_names_returns_none(tmp_path: Path) -> None:
    """Shape checks: feature_names must be a list; otherwise we refuse to trust it."""
    (tmp_path / "extras_meta.json").write_text(json.dumps({"kind": "extras"}), encoding="utf-8")
    assert read_artifact_meta(str(tmp_path), "extras", None) is None


def test_read_rejects_non_object_root(tmp_path: Path) -> None:
    (tmp_path / "innings_meta.json").write_text(json.dumps(["a", "b"]), encoding="utf-8")
    assert read_artifact_meta(str(tmp_path), "innings", None) is None


def test_legacy_alias_write_reads_as_legacy_filename(tmp_path: Path) -> None:
    """_LEGACY_ sentinel and None should both produce the same legacy file."""
    write_artifact_meta(str(tmp_path), "extras", "_LEGACY_", ["x"])
    meta = read_artifact_meta(str(tmp_path), "extras", None)
    assert meta is not None
    assert meta["feature_names"] == ["x"]
