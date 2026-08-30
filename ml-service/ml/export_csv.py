"""Reading of go-app export CSVs under a width contract the trainers can trust.

A training run is only as trustworthy as the CSV beneath it, and pandas stays silent
about the one corruption that matters most here. When a header names fewer columns
than the rows carry, ``pd.read_csv`` quietly consumes the surplus leading fields as an
index and shifts every named column left by that many places. Nothing raises. The run
simply trains on the wrong columns.

The win export shipped exactly that -- 64 header names over 72-field rows -- so
``team1_wins`` took the values of ``team1_bat_consistency_top3_mean``, the target
collapsed to a single class, and the only complaint came from GradientBoosting minutes
into the run, phrased as a class-count problem far from its cause. Comparing two
integers at the door turns that into an immediate, self-explanatory failure.

The same reasoning covers the training cutoff. Every trainer here prefers the export CSV
over the API and used to read it whole, so ``--cutoff`` governed only the fallback path
nobody takes. A run asked to train to a cutoff trained on everything instead, which is
not a smaller mistake than the width one: it silently destroys the holdout that any
honest evaluation of the model depends on.
"""

from __future__ import annotations

import csv
import logging
from typing import Optional, Tuple

import pandas as pd

logger = logging.getLogger(__name__)

# The column every dated export carries, and the one a cutoff is applied to.
MATCH_DATE_COLUMN = "match_date"


class MisalignedExportError(ValueError):
    """An export CSV whose header width disagrees with its data rows."""


class UndatedExportError(ValueError):
    """A cutoff was requested of an export that carries no match date."""


def _header_and_row_widths(path: str) -> Optional[Tuple[int, int]]:
    """Field counts of the header and the first data row, or None when there is no data row.

    Uses the csv module rather than a naive split so quoted fields containing commas
    are counted as the single fields they are.
    """
    with open(path, "r", newline="", encoding="utf-8") as f:
        reader = csv.reader(f)
        header = next(reader, None)
        if header is None:
            return None
        first_row = next(reader, None)
        if first_row is None:
            return None
        return len(header), len(first_row)


def filter_before_cutoff(df: pd.DataFrame, cutoff: str, path: str = "") -> pd.DataFrame:
    """Keep only the rows strictly before ``cutoff``.

    Strictly before, matching the export's own ``WHERE m.match_date < $1`` and the
    no-future-leakage rule, so that what this drops is exactly what a holdout evaluation
    keeps: the two are complementary by construction.

    Raises:
        UndatedExportError: The export has no match date, so the cutoff cannot be
            honoured. Training on everything instead is what this function exists to
            stop, so it refuses rather than warning.
    """
    if MATCH_DATE_COLUMN not in df.columns:
        logger.error("export_csv.cutoff_without_dates path=%s cutoff=%s", path, cutoff)
        raise UndatedExportError(
            f"{path or 'export'}: a cutoff of {cutoff!r} was requested but the export has no "
            f"{MATCH_DATE_COLUMN!r} column, so the rows on or after it cannot be excluded. "
            "Re-export, or train without a cutoff and accept that there is no holdout."
        )

    boundary = pd.to_datetime(cutoff, utc=True, errors="coerce")
    if pd.isna(boundary):
        raise ValueError(f"unparseable cutoff: {cutoff!r}")

    dates = pd.to_datetime(df[MATCH_DATE_COLUMN], utc=True, errors="coerce", format="ISO8601")
    kept = df[dates.notna() & (dates < boundary)]
    logger.info(
        "export_csv.cutoff_applied path=%s cutoff=%s kept=%s dropped=%s undated=%s",
        path,
        cutoff,
        len(kept),
        len(df) - len(kept),
        int(dates.isna().sum()),
    )
    return kept


def read_export_csv(path: str, cutoff: Optional[str] = None) -> pd.DataFrame:
    """Read a go-app export CSV, refusing one whose header and rows disagree in width.

    Args:
        path: Path to the export CSV.
        cutoff: When set, drop rows on or after this RFC3339 timestamp. Trainers pass
            their ``--cutoff`` so the CSV path honours it; without this the flag governed
            only the API fallback and a run asked to train to a cutoff trained on
            everything.

    Returns:
        The parsed DataFrame.

    Raises:
        MisalignedExportError: The header names a different number of columns than the
            first data row carries, so every column mapping is suspect.
        UndatedExportError: A cutoff was requested of an export with no match date.
    """
    widths = _header_and_row_widths(path)
    if widths is not None:
        header_columns, row_fields = widths
        if header_columns != row_fields:
            logger.error(
                "export_csv.width_mismatch path=%s header_columns=%s row_fields=%s",
                path,
                header_columns,
                row_fields,
            )
            raise MisalignedExportError(
                f"{path}: header names {header_columns} columns but the first data row has "
                f"{row_fields} fields, so column mapping is unreliable. The export is stale or "
                f"its writer is out of contract; re-run `make export-dataset` before training."
            )
    df = pd.read_csv(path)
    if cutoff:
        df = filter_before_cutoff(df, cutoff, path)
    return df
