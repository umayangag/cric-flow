"""Tests for ml.export_csv (width contract on go-app export CSVs)."""

from __future__ import annotations

from pathlib import Path

import pandas as pd
import pytest

from ml.export_csv import MisalignedExportError, UndatedExportError, read_export_csv


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


# --- training cutoff ---------------------------------------------------------
#
# The trainers prefer the export CSV over the API and used to read it whole, so
# --cutoff governed only the fallback path nobody takes. A run asked to train to a
# cutoff trained on everything instead, destroying the holdout that any honest
# evaluation depends on.


def _dated_csv(tmp_path, rows):
    path = tmp_path / "dated.csv"
    lines = ["match_id,match_date,team1_wins"]
    lines += [f"{i},{d},{w}" for i, (d, w) in enumerate(rows, start=1)]
    path.write_text("\n".join(lines) + "\n")
    return str(path)


def test_read_export_csv_without_a_cutoff_keeps_every_row(tmp_path):
    """The default is unchanged: no cutoff, no filtering."""
    path = _dated_csv(tmp_path, [("2023-01-01", 1), ("2025-06-01", 0)])

    df = read_export_csv(path)

    assert len(df) == 2


def test_read_export_csv_drops_rows_on_or_after_the_cutoff(tmp_path):
    """Strictly before, so what training drops is exactly what a holdout keeps."""
    path = _dated_csv(tmp_path, [("2023-01-01", 1), ("2025-01-01", 0), ("2025-06-01", 1)])

    df = read_export_csv(path, cutoff="2025-01-01T00:00:00Z")

    assert list(df["match_date"]) == ["2023-01-01"]


def test_read_export_csv_cutoff_keeps_nothing_when_every_match_is_later(tmp_path):
    """An empty training set is a loud, checkable outcome, not a silent full-data run."""
    path = _dated_csv(tmp_path, [("2025-06-01", 1)])

    df = read_export_csv(path, cutoff="2024-01-01T00:00:00Z")

    assert df.empty


def test_read_export_csv_drops_undated_rows_under_a_cutoff(tmp_path):
    """A row of unknown vintage cannot be shown to precede the cutoff."""
    path = _dated_csv(tmp_path, [("2023-01-01", 1), ("not-a-date", 0)])

    df = read_export_csv(path, cutoff="2025-01-01T00:00:00Z")

    assert list(df["match_date"]) == ["2023-01-01"]


def test_read_export_csv_refuses_a_cutoff_on_an_undated_export(tmp_path):
    """Honouring the flag is impossible here, and training on everything is the bug."""
    path = tmp_path / "undated.csv"
    path.write_text("a,b\n1,2\n")

    with pytest.raises(UndatedExportError, match="match_date"):
        read_export_csv(str(path), cutoff="2025-01-01T00:00:00Z")


def test_read_export_csv_rejects_an_unparseable_cutoff(tmp_path):
    """A cutoff that silently became 'no cutoff' is how this bug looked in the first place."""
    path = _dated_csv(tmp_path, [("2023-01-01", 1)])

    with pytest.raises(ValueError, match="unparseable cutoff"):
        read_export_csv(path, cutoff="whenever")


def test_cutoff_is_applied_after_the_width_check(tmp_path):
    """A misaligned export must fail on its width, not be quietly narrowed by a cutoff."""
    path = tmp_path / "wide.csv"
    path.write_text("match_id,match_date\n1,2023-01-01,extra\n")

    with pytest.raises(MisalignedExportError):
        read_export_csv(str(path), cutoff="2025-01-01T00:00:00Z")
