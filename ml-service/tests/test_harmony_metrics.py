"""Tests for ml.harmony_metrics."""

from ml.harmony_metrics import distribution_summary, realism_metrics_vs_historical


def test_distribution_summary_handles_empty_and_non_empty():
    empty = distribution_summary([])
    assert empty["count"] == 0.0
    assert empty["mean"] == 0.0

    vals = [1.0, 2.0, 3.0, 4.0]
    summary = distribution_summary(vals)
    assert summary["count"] == 4.0
    assert summary["mean"] == 2.5
    assert summary["p50"] == 2.5


def test_realism_metrics_vs_historical_reports_deltas():
    reconciled = [10.0, 12.0, 14.0]
    historical = [9.0, 11.0, 13.0]
    metrics = realism_metrics_vs_historical(reconciled, historical)

    assert metrics["reconciled_mean"] > metrics["historical_mean"]
    assert metrics["delta_mean"] == metrics["reconciled_mean"] - metrics["historical_mean"]

