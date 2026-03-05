"""Tests for ml.consistency_checker."""

from ml.consistency_checker import (
    InningsTargets,
    adjustment_magnitude,
    check_reconciled_scorecard_consistency,
)
from ml.reconciliation_service import ReconciledPlayerStats


def test_check_reconciled_scorecard_consistency_passes_when_totals_match():
    # Team1 bat 60 (inn1), bowl 70 (inn2). Team2 bowl 60 (inn1), bat 70 (inn2).
    stats = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=60, batting_balls=120, bowling_runs=35, bowling_balls=60, wickets=1
        ),
        2: ReconciledPlayerStats(
            player_id=2, team_id=100, batting_runs=0, batting_balls=0, bowling_runs=35, bowling_balls=60, wickets=1
        ),
        3: ReconciledPlayerStats(
            player_id=3, team_id=200, batting_runs=70, batting_balls=120, bowling_runs=60, bowling_balls=120, wickets=3
        ),
        4: ReconciledPlayerStats(
            player_id=4, team_id=200, batting_runs=0, batting_balls=0, bowling_runs=0, bowling_balls=0, wickets=0
        ),
    }
    inn1 = InningsTargets(batting_team_id=100, bowling_team_id=200, runs=60.0, wickets=3.0, legal_balls=120.0)
    inn2 = InningsTargets(batting_team_id=200, bowling_team_id=100, runs=70.0, wickets=2.0, legal_balls=120.0)
    violations = check_reconciled_scorecard_consistency(stats, team1_id=100, team2_id=200, inn1=inn1, inn2=inn2)
    assert violations == []


def test_check_reconciled_scorecard_consistency_flags_mismatch():
    # Team1 bat 50 (target 60); team2 bowl 50 (target 60); team2 bat 70, team1 bowl 70 but balls 100 (target 120).
    stats = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=50, batting_balls=100, bowling_runs=70, bowling_balls=100, wickets=2
        ),
        2: ReconciledPlayerStats(
            player_id=2, team_id=200, batting_runs=70, batting_balls=120, bowling_runs=50, bowling_balls=120, wickets=3
        ),
    }
    inn1 = InningsTargets(batting_team_id=100, bowling_team_id=200, runs=60.0, wickets=3.0, legal_balls=120.0)
    inn2 = InningsTargets(batting_team_id=200, bowling_team_id=100, runs=70.0, wickets=2.0, legal_balls=120.0)
    violations = check_reconciled_scorecard_consistency(stats, team1_id=100, team2_id=200, inn1=inn1, inn2=inn2)
    assert any("innings1" in v and "batting_runs" in v for v in violations)
    assert any("innings2" in v and "bowling_balls" in v for v in violations)


def test_check_reconciled_scorecard_consistency_flags_negative_wickets():
    stats = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=0, batting_balls=0, bowling_runs=0, bowling_balls=0, wickets=11
        ),
    }
    violations = check_reconciled_scorecard_consistency(stats, team1_id=100, team2_id=200)
    assert any("wickets" in v and "11" in v for v in violations)


def test_adjustment_magnitude():
    before = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=30, batting_balls=20, bowling_runs=40, bowling_balls=24, wickets=1
        ),
        2: ReconciledPlayerStats(
            player_id=2, team_id=100, batting_runs=30, batting_balls=25, bowling_runs=35, bowling_balls=24, wickets=2
        ),
    }
    after = {
        1: ReconciledPlayerStats(
            player_id=1, team_id=100, batting_runs=32, batting_balls=22, bowling_runs=38, bowling_balls=24, wickets=1
        ),
        2: ReconciledPlayerStats(
            player_id=2, team_id=100, batting_runs=28, batting_balls=23, bowling_runs=37, bowling_balls=24, wickets=2
        ),
    }
    mag = adjustment_magnitude(before, after)
    assert mag["mean_abs_delta_runs"] == 2.0  # (2 + 2) / 2
    assert mag["mean_abs_delta_wickets"] == 0.0
    assert mag["total_before_runs"] == 60
    assert mag["total_before_wickets"] == 3
