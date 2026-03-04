"""Tests for ml.harmony_metrics."""

from ml.harmony_metrics import (
    DEFAULT_REALISM_BAND,
    REALISM_ALLOWED_BANDS,
    distribution_summary,
    realism_metrics_vs_historical,
    realism_within_allowed_bands,
)


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


def test_realism_within_allowed_bands_known_stat():
    """realism_within_allowed_bands uses REALISM_ALLOWED_BANDS for known stat_label; returns dict with within_band."""
    max_dm, max_ds, max_dp = REALISM_ALLOWED_BANDS["runs_per_innings"]
    # within band (abs deltas within limits)
    out = realism_within_allowed_bands({"delta_mean": 0, "delta_std": 0, "delta_p50": 0}, "runs_per_innings")
    assert out["within_band"] is True
    assert out["delta_mean_ok"] is True
    assert out["stat_label"] == "runs_per_innings"
    # at limit
    out2 = realism_within_allowed_bands(
        {"delta_mean": max_dm, "delta_std": max_ds, "delta_p50": max_dp}, "runs_per_innings"
    )
    assert out2["within_band"] is True
    # above limit on delta_mean
    out3 = realism_within_allowed_bands({"delta_mean": max_dm + 1, "delta_std": 0, "delta_p50": 0}, "runs_per_innings")
    assert out3["within_band"] is False
    assert out3["delta_mean_ok"] is False


def test_realism_within_allowed_bands_unknown_stat_uses_default():
    """Unknown stat_label falls back to DEFAULT_REALISM_BAND (max_dm, max_ds, max_dp)."""
    max_dm, max_ds, max_dp = DEFAULT_REALISM_BAND
    out = realism_within_allowed_bands({"delta_mean": 0, "delta_std": 0, "delta_p50": 0}, "unknown_stat")
    assert out["within_band"] is True
    assert out["band_used"] == list(DEFAULT_REALISM_BAND)
    out2 = realism_within_allowed_bands({"delta_mean": max_dm + 1, "delta_std": 0, "delta_p50": 0}, "unknown_stat")
    assert out2["within_band"] is False
