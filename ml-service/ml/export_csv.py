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
"""

from __future__ import annotations

import csv
import logging
from typing import Optional, Tuple

import pandas as pd

logger = logging.getLogger(__name__)


class MisalignedExportError(ValueError):
    """An export CSV whose header width disagrees with its data rows."""


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


def read_export_csv(path: str) -> pd.DataFrame:
    """Read a go-app export CSV, refusing one whose header and rows disagree in width.

    Args:
        path: Path to the export CSV.

    Returns:
        The parsed DataFrame.

    Raises:
        MisalignedExportError: The header names a different number of columns than the
            first data row carries, so every column mapping is suspect.
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
    return pd.read_csv(path)
