"""Offline analysis for reconciliation adjustment metrics (§3.1.2).

This script reads structured logs (structlog JSON) emitted by
`app.prediction_service.predict_players_with_features` and aggregates the
`backtest_predict.reconciliation.applied` events by format and optionally
by time window (week or month) when log records include a timestamp.

Usage (example):

    python -m ml.analyze_reconciliation_adjustments \\
        --log-file path/to/ml-service.log \\
        --model-family players \\
        --output-json summary.json

    python -m ml.analyze_reconciliation_adjustments \\
        --log-file path/to/ml-service.log \\
        --group-by month \\
        --output-json summary_by_month.json

The script is intentionally simple and file-based so it can be run against:

- Local docker logs (after extracting the ml-service container logs),
- Archived JSON logs from production,
- Synthetic logs produced in offline experiments.
"""

from __future__ import annotations

import argparse
import json
from collections import defaultdict
from datetime import datetime
from typing import Any, Dict, Iterable, List, Optional, TextIO

EVENT_NAME = "backtest_predict.reconciliation.applied"
METRIC_KEYS = [
    "mean_abs_delta_runs",
    "mean_abs_delta_wickets",
    "mean_abs_pct_delta_runs",
    "mean_abs_pct_delta_wickets",
    "total_before_runs",
    "total_before_wickets",
]
TIMESTAMP_KEYS = ("timestamp", "time", "@timestamp")


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


def _parse_timestamp(rec: Dict[str, Any]) -> Optional[datetime]:
    """Extract datetime from a log record (timestamp, time, @timestamp)."""
    for key in TIMESTAMP_KEYS:
        val = rec.get(key)
        if val is None:
            continue
        if isinstance(val, (int, float)):
            try:
                return datetime.utcfromtimestamp(float(val))
            except (ValueError, OSError):
                continue
        if isinstance(val, str):
            s = val.strip().replace("Z", "+00:00")
            try:
                return datetime.fromisoformat(s)
            except ValueError:
                for fmt in ("%Y-%m-%dT%H:%M:%S.%f", "%Y-%m-%dT%H:%M:%S", "%Y-%m-%d %H:%M:%S"):
                    try:
                        return datetime.strptime(s[:19], fmt)
                    except ValueError:
                        continue
    return None


def _time_bucket(dt: datetime, group_by: str) -> str:
    """Return a string key for the time window (e.g. '2024-01' for month, '2024-W03' for week)."""
    if group_by == "month":
        return dt.strftime("%Y-%m")
    if group_by == "week":
        iso = dt.isocalendar()
        return f"{iso[0]}-W{iso[1]:02d}"
    return ""


def analyze_reconciliation_records(
    records: Iterable[Dict[str, Any]],
    model_family: str = "players",
    group_by: Optional[str] = None,
) -> Dict[str, Any]:
    """Aggregate adjustment metrics from reconciliation-applied events (§3.1.2).

    Args:
        records: Iterable of structured log records (already parsed JSON).
        model_family: Logical model family label for reporting
            (e.g. 'batting', 'bowling', 'innings', 'win', 'players').
        group_by: Optional time window: 'week' or 'month'. When set, records with a
            parseable timestamp are also aggregated by that window (output includes
            by_week or by_month). Records without a timestamp are still counted in
            overall and by_format.

    Returns:
        Dict with overall, by_format, and optionally by_week/by_month aggregates.
    """
    totals_overall: Dict[str, float] = defaultdict(float)
    counts_overall = 0

    per_format_totals: Dict[str, Dict[str, float]] = defaultdict(lambda: defaultdict(float))
    per_format_counts: Dict[str, int] = defaultdict(int)

    per_time_totals: Dict[str, Dict[str, float]] = defaultdict(lambda: defaultdict(float))
    per_time_counts: Dict[str, int] = defaultdict(int)

    for rec in records:
        event = rec.get("event") or rec.get("msg") or rec.get("message")
        if event != EVENT_NAME:
            continue
        fmt = str(rec.get("format") or "").upper() or "UNKNOWN"
        counts_overall += 1
        per_format_counts[fmt] += 1
        for key in METRIC_KEYS:
            try:
                val = float(rec.get(key, 0.0))
            except (TypeError, ValueError):
                val = 0.0
            totals_overall[key] += val
            per_format_totals[fmt][key] += val

        if group_by in ("week", "month"):
            dt = _parse_timestamp(rec)
            if dt is not None:
                bucket = _time_bucket(dt, group_by)
                if bucket:
                    per_time_counts[bucket] += 1
                    for key in METRIC_KEYS:
                        try:
                            val = float(rec.get(key, 0.0))
                        except (TypeError, ValueError):
                            val = 0.0
                        per_time_totals[bucket][key] += val

    def _avg(total: float, count: int) -> float:
        return float(total / count) if count > 0 else 0.0

    overall_summary: Dict[str, Any] = {
        "event": EVENT_NAME,
        "model_family": model_family,
        "count": counts_overall,
    }
    for key in METRIC_KEYS:
        overall_summary[f"avg_{key}"] = _avg(totals_overall[key], counts_overall)

    by_format: Dict[str, Dict[str, Any]] = {}
    for fmt, count in per_format_counts.items():
        fmt_totals = per_format_totals[fmt]
        summary: Dict[str, Any] = {"format": fmt, "count": count}
        for key in METRIC_KEYS:
            summary[f"avg_{key}"] = _avg(fmt_totals[key], count)
        by_format[fmt] = summary

    out: Dict[str, Any] = {
        "overall": overall_summary,
        "by_format": by_format,
    }
    if group_by in ("week", "month") and per_time_counts:
        time_key = f"by_{group_by}"
        by_time: Dict[str, Dict[str, Any]] = {}
        for bucket, count in sorted(per_time_counts.items()):
            totals = per_time_totals[bucket]
            summary = {"window": bucket, "count": count}
            for key in METRIC_KEYS:
                summary[f"avg_{key}"] = _avg(totals[key], count)
            by_time[bucket] = summary
        out[time_key] = by_time
    return out


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
        "--group-by",
        choices=["week", "month"],
        default=None,
        help="Aggregate by time window when log records include a timestamp (timestamp/time/@timestamp).",
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

    summary = analyze_reconciliation_records(records, model_family=args.model_family, group_by=args.group_by)

    output = json.dumps(summary, indent=2, sort_keys=True)
    if args.output_json == "-" or not args.output_json:
        print(output)
    else:
        with open(args.output_json, "w", encoding="utf-8") as f:
            f.write(output)


if __name__ == "__main__":
    main()
