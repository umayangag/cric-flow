"""Tests for ml.consistency_eval."""

from app.models import BacktestPlayerPred
from ml.consistency_eval import (
    build_before_after_stats_from_predictions,
    compute_consistency_metrics,
)


def _make_simple_preds():
    # Two players per team with simple, consistent lines.
    return [
        BacktestPlayerPred(player_id=1, runs=30.0, balls=20.0, fours=3.0, sixes=1.0, wickets=0.0, economy=6.0),
        BacktestPlayerPred(player_id=2, runs=25.0, balls=18.0, fours=2.0, sixes=1.0, wickets=1.0, economy=6.5),
        BacktestPlayerPred(player_id=3, runs=40.0, balls=22.0, fours=4.0, sixes=2.0, wickets=2.0, economy=7.0),
        BacktestPlayerPred(player_id=4, runs=10.0, balls=10.0, fours=1.0, sixes=0.0, wickets=0.0, economy=8.0),
    ]


def test_build_before_after_stats_from_predictions_roundtrip():
    preds = _make_simple_preds()
    team1_ids = [1, 2]
    team2_ids = [3, 4]

    # Set innings targets close to the sum of predicted runs so reconciliation
    # only needs small adjustments.
    inn1_runs = 55.0  # team1 batting
    inn2_runs = 50.0  # team2 batting

    before, after = build_before_after_stats_from_predictions(
        preds,
        team1_ids=team1_ids,
        team2_ids=team2_ids,
        inn1_runs=inn1_runs,
        inn1_wkts=3.0,
        inn2_runs=inn2_runs,
        inn2_wkts=2.0,
        match_id=123,
        format_code="T20",
    )

    # We expect a before/after entry for each player that was reconciled.
    pred_ids = {p.player_id for p in preds}
    assert set(before.keys()).issubset(pred_ids)
    assert set(after.keys()).issubset(pred_ids)
    assert before.keys() == after.keys()

    # Basic sanity: totals before should reflect the raw predicted runs.
    total_before_runs = sum(s.batting_runs for s in before.values())
    assert total_before_runs == sum(int(p.runs) for p in preds)


def test_compute_consistency_metrics_uses_adjustment_magnitude():
    preds = _make_simple_preds()
    before, after = build_before_after_stats_from_predictions(
        preds,
        team1_ids=[1, 2],
        team2_ids=[3, 4],
        inn1_runs=55.0,
        inn1_wkts=3.0,
        inn2_runs=50.0,
        inn2_wkts=2.0,
        match_id=456,
        format_code="ODI",
    )

    metrics = compute_consistency_metrics(before, after)

    # Check that the key summary fields are present and non-negative.
    assert "mean_abs_delta_runs" in metrics
    assert "mean_abs_delta_wickets" in metrics
    assert "mean_abs_pct_delta_runs" in metrics
    assert "mean_abs_pct_delta_wickets" in metrics
    assert "total_before_runs" in metrics
    assert "total_before_wickets" in metrics

    assert metrics["mean_abs_delta_runs"] >= 0.0
    assert metrics["mean_abs_delta_wickets"] >= 0.0
    assert metrics["mean_abs_pct_delta_runs"] >= 0.0
    assert metrics["mean_abs_pct_delta_wickets"] >= 0.0
    assert metrics["total_before_runs"] > 0.0
