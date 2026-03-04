"""Tests for ml.consistency_losses."""

from ml.consistency_losses import consistency_regularization_loss
from ml.consistency_checker import ReconciledPlayerStats


def test_consistency_regularization_loss_zero_when_no_change():
    before = {
        1: ReconciledPlayerStats(player_id=1, team_id=100, batting_runs=30, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=2),
    }
    after = {
        1: ReconciledPlayerStats(player_id=1, team_id=100, batting_runs=30, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=2),
    }
    loss = consistency_regularization_loss(before, after)
    assert loss == 0.0


def test_consistency_regularization_loss_positive_when_adjusted():
    before = {
        1: ReconciledPlayerStats(player_id=1, team_id=100, batting_runs=30, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=2),
    }
    after = {
        1: ReconciledPlayerStats(player_id=1, team_id=100, batting_runs=33, batting_balls=20, bowling_runs=0, bowling_balls=0, wickets=3),
    }
    loss = consistency_regularization_loss(before, after, weight_runs=1.0, weight_wickets=1.0)
    assert loss > 0.0

