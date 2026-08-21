"""Prediction orchestration by domain; import from ``app.prediction_service`` as before."""

from .endpoints import (
    extras_feature_vector,
    resolve_model_pair,
    run_batting_prediction,
    run_bowling_prediction,
    run_extras_prediction,
    run_team_optimization,
    run_win_prediction,
    run_win_prediction_enhanced,
    validate_predict_batch,
    win_feature_vector,
)
from .generate_match import generate_match
from .innings import predict_match_innings, sum_team_feature
from .players import (
    _assemble_player_predictions,
    _resolve_prediction_model_pairs,
    predict_players_batch,
    predict_players_with_features,
)

_sum_team_feature = sum_team_feature

from ..prediction_settings import GenerateMatchSettings, round_datetime_to_granularity

__all__ = [
    "GenerateMatchSettings",
    "round_datetime_to_granularity",
    "predict_match_innings",
    "predict_players_with_features",
    "predict_players_batch",
    "generate_match",
    "resolve_model_pair",
    "validate_predict_batch",
    "run_batting_prediction",
    "run_bowling_prediction",
    "run_extras_prediction",
    "run_win_prediction",
    "run_win_prediction_enhanced",
    "run_team_optimization",
    "extras_feature_vector",
    "win_feature_vector",
    "_assemble_player_predictions",
    "_resolve_prediction_model_pairs",
    "_sum_team_feature",
]
