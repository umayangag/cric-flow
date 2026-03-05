"""Compute harmony realism metrics for reconciled outputs vs historical data.

This script is a thin CLI wrapper around `ml.harmony_metrics.realism_metrics_vs_historical`.

It is intended for offline analysis and monitoring, not the core prediction path.

Example:

    python -m ml.compute_harmony_realism_metrics \\
        --reconciled-csv reconciled_innings.csv \\
        --historical-csv historical_innings.csv \\
        --column innings_runs \\
        --stat-label runs_per_innings \\
        --output-json realism_runs.json
"""

from __future__ import annotations

import argparse
import json
from typing import Any, Dict, List, Optional

import pandas as pd

from .harmony_metrics import realism_metrics_vs_historical


def compute_realism_for_column(
    reconciled_df: "pd.DataFrame",
    historical_df: "pd.DataFrame",
    column: str,
    stat_label: Optional[str] = None,
) -> Dict[str, Any]:
    """Compute realism metrics for a single numeric column present in both dataframes."""
    if column not in reconciled_df.columns:
        raise ValueError(f"Column '{column}' not found in reconciled data.")
    if column not in historical_df.columns:
        raise ValueError(f"Column '{column}' not found in historical data.")

    rec_vals = reconciled_df[column].dropna().astype(float).tolist()
    hist_vals = historical_df[column].dropna().astype(float).tolist()

    metrics = realism_metrics_vs_historical(rec_vals, hist_vals)
    label = stat_label or column
    out: Dict[str, Any] = {"stat": label, "column": column}
    out.update(metrics)
    return out


def main(argv: Optional[List[str]] = None) -> None:
    parser = argparse.ArgumentParser(description="Compute harmony realism metrics for one stat column.")
    parser.add_argument(
        "--reconciled-csv",
        required=True,
        help="CSV with reconciled outputs (e.g. simulated or model outputs).",
    )
    parser.add_argument(
        "--historical-csv",
        required=True,
        help="CSV with historical reference data.",
    )
    parser.add_argument(
        "--column",
        required=True,
        help="Column name to compare between reconciled and historical CSVs.",
    )
    parser.add_argument(
        "--stat-label",
        default="",
        help="Optional human-readable stat label (e.g. runs_per_innings). Defaults to column name.",
    )
    parser.add_argument(
        "--output-json",
        default="-",
        help="Path to write JSON summary (default: stdout).",
    )
    args = parser.parse_args(argv)

    rec_df = pd.read_csv(args.reconciled_csv)
    hist_df = pd.read_csv(args.historical_csv)

    summary = compute_realism_for_column(
        rec_df,
        hist_df,
        column=args.column,
        stat_label=args.stat_label or None,
    )

    payload = json.dumps(summary, indent=2, sort_keys=True)
    if args.output_json == "-" or not args.output_json:
        print(payload)
    else:
        with open(args.output_json, "w", encoding="utf-8") as f:
            f.write(payload)


if __name__ == "__main__":
    main()
