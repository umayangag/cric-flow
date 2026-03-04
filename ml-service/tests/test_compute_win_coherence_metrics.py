"""Tests for ml.compute_win_coherence_metrics."""

import pandas as pd

from ml.compute_win_coherence_metrics import aggregate_win_coherence


def test_aggregate_win_coherence_overall_and_by_format():
    df = pd.DataFrame(
        {
            "margin": [10, -5, 30, -20],
            "p_model_team1": [0.7, 0.3, 0.9, 0.1],
            "format": ["T20", "T20", "ODI", "ODI"],
        }
    )

    summary = aggregate_win_coherence(df)

    assert summary["overall"]["count"] == 4
    assert 0.0 <= summary["overall"]["mean_abs_diff"] <= 1.0

    by_format = summary["by_format"]
    assert set(by_format.keys()) == {"T20", "ODI"}
    assert by_format["T20"]["count"] == 2
    assert by_format["ODI"]["count"] == 2


def test_aggregate_win_coherence_raises_on_missing_columns():
    df = pd.DataFrame({"margin": [10], "p_model_team1": [0.7]})

    # Missing margin column name should raise
    try:
        aggregate_win_coherence(df, margin_col="missing_margin")
    except ValueError as e:
        assert "Column 'missing_margin' not found" in str(e)
    else:
        assert False, "Expected ValueError for missing margin column"
