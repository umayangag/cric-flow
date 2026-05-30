"""Unit tests for app.prediction_service with mocked dependencies."""

from unittest.mock import MagicMock, patch

import numpy as np

from app.models import MatchContext
from app.prediction_service import _sum_team_feature, predict_match_innings


def test_sum_team_feature_empty_ids():
    """_sum_team_feature with empty ids returns (0, 0)."""
    features_map = {"1": {"batting_std_w10": 1.0, "bowling_std_w10": 2.0}}
    bat_sum, bowl_sum = _sum_team_feature(features_map, set(), "batting_std_w10", "bowling_std_w10")
    assert bat_sum == 0.0
    assert bowl_sum == 0.0


def test_sum_team_feature_missing_player_uses_defaults():
    """Missing player id in features_map contributes 0."""
    features_map = {"1": {"batting_std_w10": 3.0, "bowling_std_w10": 4.0}}
    bat_sum, bowl_sum = _sum_team_feature(features_map, {1, 999}, "batting_std_w10", "bowling_std_w10")
    assert bat_sum == 3.0
    assert bowl_sum == 4.0


def test_sum_team_feature_missing_key_uses_zero():
    """Missing key in feature dict contributes 0."""
    features_map = {"1": {"batting_std_w10": 1.5}}
    bat_sum, bowl_sum = _sum_team_feature(features_map, {1}, "batting_std_w10", "bowling_std_w10")
    assert bat_sum == 1.5
    assert bowl_sum == 0.0


def test_sum_team_feature_sums_multiple_players():
    """_sum_team_feature sums batting and bowling keys across players."""
    features_map = {
        "1": {"batting_std_w10": 1.0, "bowling_std_w10": 2.0},
        "2": {"batting_std_w10": 3.0, "bowling_std_w10": 4.0},
    }
    bat_sum, bowl_sum = _sum_team_feature(features_map, {1, 2}, "batting_std_w10", "bowling_std_w10")
    assert bat_sum == 4.0
    assert bowl_sum == 6.0


def test_predict_match_innings_returns_none_when_no_model():
    """predict_match_innings returns None when INNINGS_MODELS has no entry for format."""
    ctx = MatchContext(team1_player_ids=[1, 2], team2_player_ids=[3, 4])
    features_map = {
        "1": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
        "2": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
        "3": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
        "4": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
    }
    with patch("app.prediction_service.innings.INNINGS_MODELS", {}):
        result = predict_match_innings(ctx, features_map, "ODI", 1700000000.0)
    assert result is None


def test_predict_match_innings_returns_tuple_when_model_present():
    """predict_match_innings returns (inn1_runs, inn1_wkts, inn2_runs, inn2_wkts) when model exists."""
    ctx = MatchContext(team1_player_ids=[1, 2], team2_player_ids=[3, 4])
    features_map = {
        "1": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
        "2": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
        "3": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
        "4": {"batting_std_w10": 0.5, "bowling_std_w10": 0.5, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
    }
    fake_scaler = MagicMock()
    fake_scaler.transform.return_value = np.array([[0.0] * 10])
    fake_model = MagicMock()
    fake_model.predict.return_value = np.array([[50.0, 2.0]])
    innings_pair = (fake_scaler, fake_model)
    with patch("app.prediction_service.innings.INNINGS_MODELS", {"ODI": innings_pair}):
        with patch("app.prediction_service.innings.predict_innings") as mock_predict:
            mock_predict.return_value = (55.0, 3.0)
            result = predict_match_innings(ctx, features_map, "ODI", 1700000000.0)
    assert result is not None
    inn1_runs, inn1_wkts, inn2_runs, inn2_wkts = result
    assert inn1_runs == 55.0
    assert inn1_wkts == 3.0
    assert inn2_runs == 55.0
    assert inn2_wkts == 3.0
    assert mock_predict.call_count == 2


def test_predict_match_innings_uses_legacy_when_format_missing():
    """predict_match_innings falls back to _LEGACY_ when format key not in INNINGS_MODELS."""
    ctx = MatchContext(team1_player_ids=[1], team2_player_ids=[2])
    features_map = {
        "1": {"batting_std_w10": 0.0, "bowling_std_w10": 0.0, "batting_mean_w5": 0.0, "bowling_mean_w5": 0.0},
        "2": {"batting_std_w10": 0.0, "bowling_std_w10": 0.0, "batting_mean_w5": 0.0, "bowling_mean_w5": 0.0},
    }
    fake_scaler = MagicMock()
    fake_scaler.transform.return_value = np.array([[0.0]])
    fake_model = MagicMock()
    fake_model.predict.return_value = np.array([[40.0, 2.0]])
    with patch("app.prediction_service.innings.INNINGS_MODELS", {"_LEGACY_": (fake_scaler, fake_model)}):
        with patch("app.prediction_service.innings.predict_innings", return_value=(40.0, 2.0)):
            result = predict_match_innings(ctx, features_map, "T20", 1700000000.0)
    assert result == (40.0, 2.0, 40.0, 2.0)
