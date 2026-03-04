"""Tests for ml.analyze_reconciliation_adjustments."""

from ml.analyze_reconciliation_adjustments import analyze_reconciliation_records, EVENT_NAME


def test_analyze_reconciliation_records_aggregates_overall_and_by_format():
    # Build two reconciliation-applied events for different formats.
    records = [
        {
            "event": EVENT_NAME,
            "format": "T20",
            "mean_abs_delta_runs": 4.0,
            "mean_abs_delta_wickets": 0.5,
            "mean_abs_pct_delta_runs": 10.0,
            "mean_abs_pct_delta_wickets": 5.0,
            "total_before_runs": 300.0,
            "total_before_wickets": 8.0,
        },
        {
            "event": EVENT_NAME,
            "format": "ODI",
            "mean_abs_delta_runs": 6.0,
            "mean_abs_delta_wickets": 1.0,
            "mean_abs_pct_delta_runs": 12.0,
            "mean_abs_pct_delta_wickets": 7.0,
            "total_before_runs": 500.0,
            "total_before_wickets": 12.0,
        },
    ]

    summary = analyze_reconciliation_records(records, model_family="players")

    assert summary["overall"]["count"] == 2
    # Overall averages
    assert summary["overall"]["avg_mean_abs_delta_runs"] == 5.0
    assert summary["overall"]["avg_mean_abs_delta_wickets"] == 0.75
    assert summary["overall"]["avg_total_before_runs"] == 400.0
    assert summary["overall"]["avg_total_before_wickets"] == 10.0

    by_format = summary["by_format"]
    assert set(by_format.keys()) == {"T20", "ODI"}
    assert by_format["T20"]["count"] == 1
    assert by_format["T20"]["avg_mean_abs_delta_runs"] == 4.0
    assert by_format["ODI"]["avg_mean_abs_delta_runs"] == 6.0


def test_analyze_reconciliation_records_ignores_non_matching_events_and_bad_values():
    records = [
        {"event": "other_event", "mean_abs_delta_runs": 999.0},
        {
            "event": EVENT_NAME,
            "format": "T20",
            "mean_abs_delta_runs": "not-a-number",
            "mean_abs_delta_wickets": 1.0,
        },
    ]

    summary = analyze_reconciliation_records(records, model_family="innings")

    # Only one valid event counted
    assert summary["overall"]["count"] == 1
    # Non-numeric value should be treated as 0.0
    assert summary["overall"]["avg_mean_abs_delta_runs"] == 0.0
    assert summary["overall"]["avg_mean_abs_delta_wickets"] == 1.0

