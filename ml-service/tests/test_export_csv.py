"""Tests for ml.export_csv (width contract on go-app export CSVs)."""

from __future__ import annotations

from pathlib import Path

import pandas as pd
import pytest

from ml.export_csv import MisalignedExportError, read_export_csv


def _write(path: Path, *lines: str) -> str:
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return str(path)


def test_aligned_export_is_read_normally(tmp_path: Path) -> None:
    """A header matching its rows in width parses into the columns it names."""
    path = _write(
        tmp_path / "win_encoded_all.csv",
        "match_id,team1_wins,format_code",
        "1,1,ODI",
        "2,0,T20",
    )

    df = read_export_csv(path)

    assert list(df.columns) == ["match_id", "team1_wins", "format_code"]
    assert df["team1_wins"].tolist() == [1, 0]
    assert df["format_code"].tolist() == ["ODI", "T20"]


def test_header_narrower_than_rows_is_rejected(tmp_path: Path) -> None:
    """The win regression: surplus row fields would silently shift every named column."""
    path = _write(
        tmp_path / "win_encoded_all.csv",
        "match_id,team1_wins,format_code",
        "1,1,ODI,0.5,7",
        "2,0,T20,0.4,9",
    )

    with pytest.raises(MisalignedExportError) as excinfo:
        read_export_csv(path)

    message = str(excinfo.value)
    assert "3 columns" in message
    assert "5 fields" in message
    assert "make export-dataset" in message


def test_header_wider_than_rows_is_rejected(tmp_path: Path) -> None:
    """A header naming columns the rows do not carry is equally out of contract."""
    path = _write(
        tmp_path / "extras_encoded_all.csv",
        "match_id,extras,format_code,venue_id",
        "1,12,ODI",
    )

    with pytest.raises(MisalignedExportError):
        read_export_csv(path)


def test_quoted_commas_count_as_one_field(tmp_path: Path) -> None:
    """Field counting follows CSV quoting rather than splitting on every comma."""
    path = _write(
        tmp_path / "innings_encoded_all.csv",
        "match_id,venue_name,runs",
        '1,"Lord\'s, London",250',
    )

    df = read_export_csv(path)

    assert df["venue_name"].tolist() == ["Lord's, London"]


def test_header_only_export_is_read_as_empty(tmp_path: Path) -> None:
    """With no data row there is nothing to disagree with, so the empty frame passes through."""
    path = _write(tmp_path / "fielding_encoded_all.csv", "match_id,catches")

    df = read_export_csv(path)

    assert df.empty
    assert list(df.columns) == ["match_id", "catches"]


def test_empty_export_falls_through_to_pandas(tmp_path: Path) -> None:
    """A file with no header at all has no width to check; pandas reports the empty file."""
    path = tmp_path / "win_encoded_all.csv"
    path.write_text("", encoding="utf-8")

    with pytest.raises(pd.errors.EmptyDataError):
        read_export_csv(str(path))
