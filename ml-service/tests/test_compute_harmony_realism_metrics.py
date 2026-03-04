"""Tests for ml.compute_harmony_realism_metrics."""

import pandas as pd

from ml.compute_harmony_realism_metrics import compute_realism_for_column


def test_compute_realism_for_column_basic_deltas():
    rec_df = pd.DataFrame({"innings_runs": [150, 160, 170]})
    hist_df = pd.DataFrame({"innings_runs": [140, 150, 160]})

    summary = compute_realism_for_column(rec_df, hist_df, column="innings_runs", stat_label="runs_per_innings")

    assert summary["stat"] == "runs_per_innings"
    assert summary["column"] == "innings_runs"
    # Means: rec ~160, hist ~150 => delta_mean > 0
    assert summary["delta_mean"] > 0
    # Medians: rec ~160, hist ~150 => delta_p50 > 0
    assert summary["delta_p50"] > 0


def test_compute_realism_for_column_raises_on_missing_column():
    rec_df = pd.DataFrame({"a": [1, 2, 3]})
    hist_df = pd.DataFrame({"a": [1, 2, 3]})

    try:
        compute_realism_for_column(rec_df, hist_df, column="b")
    except ValueError as e:
        assert "Column 'b' not found" in str(e)
    else:
        assert False, "Expected ValueError for missing column"

