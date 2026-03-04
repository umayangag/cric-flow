"""Offline analysis for reconciliation adjustment metrics.

This script reads structured logs (structlog JSON) emitted by
`app.prediction_service.predict_players_with_features` and aggregates the
`backtest_predict.reconciliation.applied` events.

Usage (example):

    python -m ml.analyze_reconciliation_adjustments \\
        --log-file path/to/ml-service.log \\
        --model-family players \\
        --output-json summary.json

The script is intentionally simple and file-based so it can be run against:

- Local docker logs (after extracting the ml-service container logs),
- Archived JSON logs from production,
- Synthetic logs produced in offline experiments.
"""

from __future__ import annotations

import argparse
import json
from collections import defaultdict
from typing import Any, Dict, Iterable, List, Optional, TextIO


EVENT_NAME = "backtest_predict.reconciliation.applied"


def _iter_log_lines(handle: TextIO) -> Iterable[Dict[str, Any]]:
    """Yield parsed JSON objects from a log file; skip malformed lines."""
    for line in handle:
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(obj, dict):
            yield obj


def analyze_reconciliation_records(
    records: Iterable[Dict[str, Any]],
    model_family: str = "players",
) -> Dict[str, Any]:
    """Aggregate adjustment metrics from reconciliation-applied events.

    Args:
        records: Iterable of structured log records (already parsed JSON).
        model_family: Logical model family label for reporting
            (e.g. 'batting', 'bowling', 'innings', 'win', 'players').

    Returns:
        Dict with overall and per-format aggregates, suitable for dashboards
        or saving as a JSON report.
    """
    totals_overall: Dict[str, float] = defaultdict(float)
    counts_overall = 0

    per_format_totals: Dict[str, Dict[str, float]] = defaultdict(lambda: defaultdict(float))
    per_format_counts: Dict[str, int] = defaultdict(int)

    metric_keys = [
        "mean_abs_delta_runs",
        "mean_abs_delta_wickets",
        "mean_abs_pct_delta_runs",
        "mean_abs_pct_delta_wickets",
        "total_before_runs",
        "total_before_wickets",
    ]

    for rec in records:
        event = rec.get("event") or rec.get("msg") or rec.get("message")
        if event != EVENT_NAME:
            continue
        fmt = str(rec.get("format") or "").upper() or "UNKNOWN"
        counts_overall += 1
        per_format_counts[fmt] += 1
        for key in metric_keys:
            try:
                val = float(rec.get(key, 0.0))
            except (TypeError, ValueError):
                val = 0.0
            totals_overall[key] += val
            per_format_totals[fmt][key] += val

    def _avg(total: float, count: int) -> float:
        return float(total / count) if count > 0 else 0.0

    overall_summary = {
        "event": EVENT_NAME,
        "model_family": model_family,
        "count": counts_overall,
    }
    for key in metric_keys:
        overall_summary[f"avg_{key}"] = _avg(totals_overall[key], counts_overall)

    by_format: Dict[str, Dict[str, Any]] = {}
    for fmt, count in per_format_counts.items():
        fmt_totals = per_format_totals[fmt]
        summary: Dict[str, Any] = {"format": fmt, "count": count}
        for key in metric_keys:
            summary[f"avg_{key}"] = _avg(fmt_totals[key], count)
        by_format[fmt] = summary

    return {
        "overall": overall_summary,
        "by_format": by_format,
    }


def main(argv: Optional[List[str]] = None) -> None:
    parser = argparse.ArgumentParser(description="Analyze reconciliation adjustment metrics from structured logs.")
    parser.add_argument(
        "--log-file",
        default="-",
        help="Path to log file (JSON lines). Use '-' or omit for stdin.",
    )
    parser.add_argument(
        "--model-family",
        default="players",
        help="Logical model family label for reporting (e.g. batting, bowling, innings, win, players).",
    )
    parser.add_argument(
        "--output-json",
        default="-",
        help="Path to write JSON summary (default: stdout).",
    )
    args = parser.parse_args(argv)

    if args.log_file == "-" or not args.log_file:
        import sys

        records = _iter_log_lines(sys.stdin)
    else:
        with open(args.log_file, "r", encoding="utf-8") as f:
            records = list(_iter_log_lines(f))

    summary = analyze_reconciliation_records(records, model_family=args.model_family)

    output = json.dumps(summary, indent=2, sort_keys=True)
    if args.output_json == "-" or not args.output_json:
        print(output)
    else:
        with open(args.output_json, "w", encoding="utf-8") as f:
            f.write(output)


if __name__ == "__main__":
    main()

