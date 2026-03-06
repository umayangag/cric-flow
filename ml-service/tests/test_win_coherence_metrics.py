"""Tests for ml.win_coherence_metrics."""

from ml.win_coherence_metrics import (
    implied_win_probability_from_margin,
    win_probability_coherence_from_margin,
)


def test_implied_win_probability_from_margin_is_symmetric_and_bounded():
    # Zero margin -> ~0.5
    p_zero = implied_win_probability_from_margin(0.0)
    assert 0.49 < p_zero < 0.51

    # Positive margin -> > 0.5, negative margin -> < 0.5
    p_pos = implied_win_probability_from_margin(50.0)
    p_neg = implied_win_probability_from_margin(-50.0)
    assert p_pos > 0.5
    assert p_neg < 0.5

    # Large margins saturate but stay within (0, 1)
    p_large = implied_win_probability_from_margin(1e6)
    p_large_neg = implied_win_probability_from_margin(-1e6)
    assert 0.999 < p_large < 1.0
    assert 0.0 < p_large_neg < 0.001


def test_win_probability_coherence_from_margin_reports_abs_diff():
    m = 30.0
    metrics = win_probability_coherence_from_margin(p_model_team1=0.8, margin=m, scale=25.0)
    assert 0.0 <= metrics["p_model_team1"] <= 1.0
    assert 0.0 <= metrics["p_implied_team1"] <= 1.0
    assert metrics["abs_diff"] == abs(metrics["p_model_team1"] - metrics["p_implied_team1"])


def test_implied_win_probability_negative_scale_falls_back():
    """scale <= 0 falls back to default 25.0."""
    p = implied_win_probability_from_margin(10.0, scale=-5.0)
    p_default = implied_win_probability_from_margin(10.0, scale=25.0)
    assert abs(p - p_default) < 1e-9


def test_implied_win_probability_invalid_scale_falls_back():
    """Non-numeric scale falls back to 25.0."""
    p = implied_win_probability_from_margin(10.0, scale="bad")  # type: ignore[arg-type]
    p_default = implied_win_probability_from_margin(10.0, scale=25.0)
    assert abs(p - p_default) < 1e-9


def test_win_probability_coherence_invalid_model_prob_falls_back():
    """Non-numeric p_model_team1 falls back to 0.5."""
    metrics = win_probability_coherence_from_margin(p_model_team1="bad", margin=0.0)  # type: ignore[arg-type]
    assert metrics["p_model_team1"] == 0.5
