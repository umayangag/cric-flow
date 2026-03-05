"""Tests for ml.consistency_losses."""

from ml.consistency_checker import ReconciledPlayerStats
from ml.consistency_losses import consistency_penalty_from_metrics, consistency_regularization_loss


def test_consistency_penalty_from_metrics():
    metrics = {"mean_abs_pct_delta_runs": 10.0, "mean_abs_pct_delta_wickets": 5.0}
    penalty = consistency_penalty_from_metrics(metrics, weight_runs=1.0, weight_wickets=1.0)
    assert penalty == 15.0

    penalty_weighted = consistency_penalty_from_metrics(metrics, weight_runs=0.01, weight_wickets=0.02)
    assert penalty_weighted == 0.1 + 0.1


def test_consistency_regularization_loss_zero_when_no_change():
    before = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=30, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=2
        ),
    }
    after = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=30, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=2
        ),
    }
    loss = consistency_regularization_loss(before, after)
    assert loss == 0.0


def test_consistency_regularization_loss_positive_when_adjusted():
    before = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=30, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=2
        ),
    }
    after = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=33, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=3
        ),
    }
    loss = consistency_regularization_loss(before, after, weight_runs=1.0, weight_wickets=1.0)
    assert loss > 0.0
