"""Compute win-probability coherence metrics from reconciled margins and model outputs.

This script reads a CSV containing at least:

- margin: runs margin in team1's favour (team1_runs - team2_runs),
- p_model_team1: model win probability for team1 in [0, 1],
- optional: format column for per-format breakdown.

It then applies `ml.win_coherence_metrics.win_probability_coherence_from_margin`
per row and aggregates summary statistics (mean abs_diff overall and by format).
"""

from __future__ import annotations

import argparse
import json
from collections import defaultdict
from typing import Any, Dict, List, Optional

import numpy as np
import pandas as pd

from .win_coherence_metrics import win_probability_coherence_from_margin


def aggregate_win_coherence(
    df: "pd.DataFrame",
    margin_col: str = "margin",
    p_model_col: str = "p_model_team1",
    format_col: str = "format",
    scale: float = 25.0,
) -> Dict[str, Any]:
    """Aggregate win-probability coherence metrics overall and by format."""
    if margin_col not in df.columns:
        raise ValueError(f"Column '{margin_col}' not found in input CSV.")
    if p_model_col not in df.columns:
        raise ValueError(f"Column '{p_model_col}' not found in input CSV.")

    margins = df[margin_col].astype(float).values
    p_models = df[p_model_col].astype(float).values
    formats = df[format_col].astype(str).fillna("UNKNOWN").str.upper().values if format_col in df.columns else None

    overall_abs_diffs: List[float] = []
    per_format_abs_diffs: Dict[str, List[float]] = defaultdict(list)

    for i in range(len(margins)):
        metrics = win_probability_coherence_from_margin(p_models[i], margins[i], scale=scale)
        abs_diff = float(metrics["abs_diff"])
        overall_abs_diffs.append(abs_diff)
        if formats is not None:
            per_format_abs_diffs[formats[i]].append(abs_diff)

    overall_summary: Dict[str, Any] = {
        "count": len(overall_abs_diffs),
        "mean_abs_diff": float(np.mean(overall_abs_diffs)) if overall_abs_diffs else 0.0,
    }

    by_format: Dict[str, Dict[str, Any]] = {}
    for fmt, vals in per_format_abs_diffs.items():
        by_format[fmt] = {
            "format": fmt,
            "count": len(vals),
            "mean_abs_diff": float(np.mean(vals)) if vals else 0.0,
        }

    return {
        "overall": overall_summary,
        "by_format": by_format,
    }


def main(argv: Optional[List[str]] = None) -> None:
    parser = argparse.ArgumentParser(description="Compute win-probability coherence metrics from CSV.")
    parser.add_argument(
        "--csv",
        required=True,
        help="CSV containing at least margin and p_model_team1 columns.",
    )
    parser.add_argument(
        "--margin-col",
        default="margin",
        help="Column name for runs margin in team1's favour (default: margin).",
    )
    parser.add_argument(
        "--p-model-col",
        default="p_model_team1",
        help="Column name for model win probability for team1 (default: p_model_team1).",
    )
    parser.add_argument(
        "--format-col",
        default="format",
        help="Optional format column name for per-format breakdown (default: format).",
    )
    parser.add_argument(
        "--scale",
        type=float,
        default=25.0,
        help="Scale parameter for implied probability from margin (default: 25.0).",
    )
    parser.add_argument(
        "--output-json",
        default="-",
        help="Path to write JSON summary (default: stdout).",
    )
    args = parser.parse_args(argv)

    df = pd.read_csv(args.csv)
    summary = aggregate_win_coherence(
        df,
        margin_col=args.margin_col,
        p_model_col=args.p_model_col,
        format_col=args.format_col,
        scale=args.scale,
    )

    payload = json.dumps(summary, indent=2, sort_keys=True)
    if args.output_json == "-" or not args.output_json:
        print(payload)
    else:
        with open(args.output_json, "w", encoding="utf-8") as f:
            f.write(payload)


if __name__ == "__main__":
    main()

