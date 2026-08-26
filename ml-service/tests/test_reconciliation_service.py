"""Tests for ml.reconciliation_service.reconcile_match_players."""

from unittest.mock import patch

from app.models.predict import BattingPrediction, BowlingPrediction
from app.models.reconciliation import (
    InningsReconciliationPreferences,
    MatchReconciliationInputs,
    PlayerReconciliationPreferences,
)
from ml.reconciliation_service import (
    ReconciledPlayerStats,
    _get_max_margin_fraction,
    _sum_preserving_round,
    reconcile_match_players,
)


def _make_match_inputs() -> MatchReconciliationInputs:
    players = [
        PlayerReconciliationPreferences(
            player_id=1,
            team_id=100,
            batting=BattingPrediction(
                runs_scored=30.3,
                balls_faced=20.0,
                fours_scored=3.0,
                sixes_scored=1.0,
                batting_position=1.0,
                strike_rate=151.5,
            ),
            bowling=BowlingPrediction(
                runs_conceded=39.7,
                deliveries=24.0,
                wickets_taken=1.2,
                econ=9.9,
            ),
        ),
        PlayerReconciliationPreferences(
            player_id=2,
            team_id=100,
            batting=BattingPrediction(
                runs_scored=19.8,
                balls_faced=15.0,
                fours_scored=2.0,
                sixes_scored=0.0,
                batting_position=2.0,
                strike_rate=132.0,
            ),
            bowling=BowlingPrediction(
                runs_conceded=20.2,
                deliveries=12.0,
                wickets_taken=1.9,
                econ=10.1,
            ),
        ),
        PlayerReconciliationPreferences(
            player_id=3,
            team_id=200,
            batting=BattingPrediction(
                runs_scored=34.6,
                balls_faced=25.0,
                fours_scored=4.0,
                sixes_scored=1.0,
                batting_position=1.0,
                strike_rate=138.4,
            ),
            bowling=BowlingPrediction(
                runs_conceded=30.5,
                deliveries=24.0,
                wickets_taken=2.4,
                econ=7.6,
            ),
        ),
        PlayerReconciliationPreferences(
            player_id=4,
            team_id=200,
            batting=BattingPrediction(
                runs_scored=15.4,
                balls_faced=10.0,
                fours_scored=1.0,
                sixes_scored=0.0,
                batting_position=2.0,
                strike_rate=148.0,
            ),
            bowling=BowlingPrediction(
                runs_conceded=34.9,
                deliveries=24.0,
                wickets_taken=0.9,
                econ=8.7,
            ),
        ),
    ]

    innings_prefs = [
        InningsReconciliationPreferences(
            inning_number=1,
            batting_team_id=100,
            bowling_team_id=200,
            preferred_runs=60.0,
            preferred_wickets=3.0,
            preferred_legal_balls=120.0,
            balls_per_innings=120,
            is_all_out=False,
        ),
        InningsReconciliationPreferences(
            inning_number=2,
            batting_team_id=200,
            bowling_team_id=100,
            preferred_runs=70.0,
            preferred_wickets=2.0,
            preferred_legal_balls=120.0,
            balls_per_innings=120,
            is_all_out=False,
        ),
    ]

    return MatchReconciliationInputs(
        match_id=1,
        format="T20",
        players=players,
        innings=innings_prefs,
        win=None,
    )


def test_reconcile_match_players_produces_integer_stats_and_respects_totals():
    pref = _make_match_inputs()
    stats = reconcile_match_players(pref, team1_id=100, team2_id=200)

    # Type and basic properties
    assert all(isinstance(s, ReconciledPlayerStats) for s in stats.values())

    # All key stats must be integers and non-negative
    for s in stats.values():
        assert isinstance(s.batting_runs, int)
        assert isinstance(s.batting_balls, int)
        assert isinstance(s.bowling_runs, int)
        assert isinstance(s.bowling_balls, int)
        assert isinstance(s.wickets, int)
        assert s.batting_runs >= 0
        assert s.batting_balls >= 0
        assert s.bowling_runs >= 0
        assert s.bowling_balls >= 0
        assert 0 <= s.wickets <= 10

    # Check innings 1 totals: team1 batting, team2 bowling, wickets
    inn1 = pref.innings[0]
    team1_bat_runs = sum(s.batting_runs for s in stats.values() if s.team_id == 100)
    team2_bowl_runs = sum(s.bowling_runs for s in stats.values() if s.team_id == 200)
    team2_wkts = sum(s.wickets for s in stats.values() if s.team_id == 200)

    assert team1_bat_runs == int(round(inn1.preferred_runs or 0.0))
    assert team2_bowl_runs == int(round(inn1.preferred_runs or 0.0))
    assert team2_wkts == int(round(inn1.preferred_wickets or 0.0))

    # Check innings 2 totals: team2 batting, team1 bowling, wickets
    inn2 = pref.innings[1]
    team2_bat_runs = sum(s.batting_runs for s in stats.values() if s.team_id == 200)
    team1_bowl_runs = sum(s.bowling_runs for s in stats.values() if s.team_id == 100)
    team1_wkts = sum(s.wickets for s in stats.values() if s.team_id == 100)

    assert team2_bat_runs == int(round(inn2.preferred_runs or 0.0))
    assert team1_bowl_runs == int(round(inn2.preferred_runs or 0.0))
    assert team1_wkts == int(round(inn2.preferred_wickets or 0.0))

    # Balls consistency: for each innings, batting balls of batting team
    # equal bowling balls of bowling team and match preferred_legal_balls.
    team1_bat_balls = sum(s.batting_balls for s in stats.values() if s.team_id == 100)
    team2_bowl_balls = sum(s.bowling_balls for s in stats.values() if s.team_id == 200)
    assert team1_bat_balls == team2_bowl_balls == int(round(inn1.preferred_legal_balls or 0.0))

    team2_bat_balls = sum(s.batting_balls for s in stats.values() if s.team_id == 200)
    team1_bowl_balls = sum(s.bowling_balls for s in stats.values() if s.team_id == 100)
    assert team2_bat_balls == team1_bowl_balls == int(round(inn2.preferred_legal_balls or 0.0))


def test_sum_preserving_round_handles_positive_and_negative_deltas():
    # Positive delta: target sum above current floors
    vals_pos = {1: 3.2, 2: 1.7, 3: 0.1}
    target_pos = 7.0  # round -> 7
    out_pos = _sum_preserving_round(vals_pos, target_pos)
    assert sum(out_pos.values()) == int(round(target_pos))
    assert all(isinstance(v, int) and v >= 0 for v in out_pos.values())

    # Negative delta: target sum below current floors
    vals_neg = {1: 5.0, 2: 4.0, 3: 3.0}
    target_neg = 8.0  # round -> 8, floors sum to 12
    out_neg = _sum_preserving_round(vals_neg, target_neg)
    assert sum(out_neg.values()) == int(round(target_neg))
    assert all(isinstance(v, int) and v >= 0 for v in out_neg.values())


def test_sum_preserving_round_empty_values_returns_empty():
    """_sum_preserving_round with empty dict returns empty dict."""
    assert _sum_preserving_round({}, 10.0) == {}


def test_get_max_margin_fraction_invalid_config_returns_default():
    """_get_max_margin_fraction returns 0.4 when config value is not floatable."""
    with patch(
        "ml.reconciliation_service.get_reconciliation_config", return_value={"max_margin_fraction": "not_a_number"}
    ):
        assert _get_max_margin_fraction() == 0.4
    with patch("ml.reconciliation_service.get_reconciliation_config", return_value={"max_margin_fraction": None}):
        assert _get_max_margin_fraction() == 0.4
