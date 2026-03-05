"""Tests for ml.win_features_from_reconciled."""

from ml.win_features_from_reconciled import build_win_features_standardized


def test_build_win_features_standardized_sets_fields_and_normalizes_format():
    f = build_win_features_standardized(
        format_code="  t20  ",
        format_id=3,
        venue_id=10,
        season_id=2024,
        team1_opposition_id=1,
        team2_opposition_id=2,
        toss_winner_opposition_id=1,
        team1_bat_consistency_sum=10.0,
        team1_bowl_consistency_sum=8.0,
        team2_bat_consistency_sum=9.0,
        team2_bowl_consistency_sum=7.0,
        team1_bat_form_sum=5.0,
        team1_bowl_form_sum=4.0,
        team2_bat_form_sum=6.0,
        team2_bowl_form_sum=3.0,
    )

    assert f.format == "T20"
    assert f.format_id == 3
    assert f.venue_id == 10
    assert f.team1_opposition_id == 1
    assert f.team2_opposition_id == 2
    assert f.toss_winner_opposition_id == 1
    assert f.team1_bat_consistency_sum == 10.0
    assert f.team2_bowl_form_sum == 3.0
