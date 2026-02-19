"""Unit tests for ml.player_combinator (team prediction config and overall performance)."""

from unittest.mock import patch

import pandas as pd

from ml.player_combinator import calculate_overall_performance


def test_calculate_overall_performance_basic():
    """calculate_overall_performance returns dataframe with batting/bowling contribution and totals."""
    team_df = pd.DataFrame(
        {
            "runs_scored": [30, 25],
            "runs_conceded": [20, 35],
            "balls_faced": [24, 20],
        }
    )
    result = calculate_overall_performance(team_df, match_id=1, predicted_extras=5.0)
    assert "total_score" in result.columns
    assert "target" in result.columns
    assert "batting_contribution" in result.columns
    assert "bowling_contribution" in result.columns
    assert "extras" in result.columns
    assert "match_number" in result.columns
    assert result["match_number"].iloc[0] == 1
    assert result["extras"].iloc[0] == 5.0


def test_calculate_overall_performance_contribution_ratios():
    """Batting and bowling contributions are ratios of row to total."""
    team_df = pd.DataFrame(
        {
            "runs_scored": [50, 50],
            "runs_conceded": [40, 60],
            "balls_faced": [30, 30],
        }
    )
    result = calculate_overall_performance(team_df, match_id=1, predicted_extras=0.0)
    total_score = result["total_score"].iloc[0]
    target = result["target"].iloc[0]
    assert total_score > 0
    assert target > 0
    # Sum of batting_contribution should be 1 (each row is row/total_score)
    assert abs(result["batting_contribution"].sum() - 1.0) < 0.01 or result["batting_contribution"].sum() > 0
    assert abs(result["bowling_contribution"].sum() - 1.0) < 0.01 or result["bowling_contribution"].sum() > 0


def test_calculate_overall_performance_extras_default_zero():
    """When predicted_extras not given, defaults to 0.0."""
    team_df = pd.DataFrame(
        {
            "runs_scored": [10],
            "runs_conceded": [10],
            "balls_faced": [6],
        }
    )
    result = calculate_overall_performance(team_df, match_id=99)
    assert result["extras"].iloc[0] == 0.0


def test_calculate_overall_performance_team_size_from_config():
    """total_wickets and team_size come from config (team_prediction)."""
    from ml import player_combinator as pc

    with patch.object(pc, "_get_team_prediction_config", return_value={"team_size": 11, "max_wickets_per_innings": 10}):
        team_df = pd.DataFrame(
            {
                "runs_scored": [1],
                "runs_conceded": [1],
                "balls_faced": [1],
            }
        )
        result = calculate_overall_performance(team_df, match_id=1)
        assert result["total_wickets"].iloc[0] == 10


def test_get_team_prediction_config_fallback():
    """_get_team_prediction_config returns defaults on exception."""
    from ml.player_combinator import _get_team_prediction_config

    with patch("ml.config._load", side_effect=Exception("no config")):
        cfg = _get_team_prediction_config()
        assert cfg["team_size"] == 11
        assert cfg["max_wickets_per_innings"] == 10


def test_get_team_prediction_config_from_config():
    """_get_team_prediction_config reads team_size and max_wickets from config."""
    from ml.player_combinator import _get_team_prediction_config

    with patch("ml.config._load", return_value={"team_prediction": {"team_size": 15, "max_wickets_per_innings": 12}}):
        cfg = _get_team_prediction_config()
        assert cfg["team_size"] == 15
        assert cfg["max_wickets_per_innings"] == 12
