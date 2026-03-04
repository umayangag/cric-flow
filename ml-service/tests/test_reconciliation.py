"""Unit tests for app.reconciliation (innings feature vector and prediction)."""

import numpy as np

from app.models import BacktestPlayerPred
from app.reconciliation import (
    INNINGS_FEATURE_COLS,
    build_innings_feature_vector,
    predict_innings,
    rescale_player_predictions,
)


def test_build_innings_feature_vector_shape():
    """build_innings_feature_vector returns (1, n_features) array."""
    X = build_innings_feature_vector(
        inning_number=1,
        bat_consistency_sum=1.0,
        bowl_consistency_sum=1.0,
        bat_form_sum=0.5,
        bowl_form_sum=0.5,
    )
    assert X.shape == (1, len(INNINGS_FEATURE_COLS))
    assert X[0, 3] == 1.0  # inning_number
    assert X[0, 12] == 1.0  # bat_consistency_sum


def test_predict_innings_without_scaler():
    """predict_innings returns (runs, wickets) when scaler is None."""
    model = _make_mock_innings_model()
    runs, wickets = predict_innings(
        None,
        model,
        inning_number=1,
        bat_consistency_sum=1.0,
        bowl_consistency_sum=1.0,
        bat_form_sum=0.5,
        bowl_form_sum=0.5,
    )
    assert isinstance(runs, float)
    assert isinstance(wickets, float)
    assert runs >= 0
    assert 0 <= wickets <= 10


def test_predict_innings_with_scaler():
    """predict_innings transforms X with scaler when provided."""
    from sklearn.preprocessing import StandardScaler

    scaler = StandardScaler()
    X_sample = build_innings_feature_vector(
        inning_number=1,
        bat_consistency_sum=1.0,
        bowl_consistency_sum=1.0,
        bat_form_sum=0.5,
        bowl_form_sum=0.5,
    )
    scaler.fit(X_sample)
    model = _make_mock_innings_model()
    runs, wickets = predict_innings(
        scaler,
        model,
        inning_number=1,
        bat_consistency_sum=1.0,
        bowl_consistency_sum=1.0,
        bat_form_sum=0.5,
        bowl_form_sum=0.5,
    )
    assert isinstance(runs, float)
    assert isinstance(wickets, float)


def _make_mock_innings_model():
    """Minimal sklearn-style model that returns [runs, wickets] for predict(X)."""
    from sklearn.base import BaseEstimator

    class MockInningsModel(BaseEstimator):
        def predict(self, X):
            return np.array([[120.0, 5.0]])

    return MockInningsModel()


def test_rescale_player_predictions():
    """rescale_player_predictions rescales runs and wickets to match innings totals."""
    preds = [
        BacktestPlayerPred(player_id=1, runs=50.0, balls=30.0, wickets=0.0, economy=10.0),
        BacktestPlayerPred(player_id=2, runs=50.0, balls=30.0, wickets=2.0, economy=8.0),
        BacktestPlayerPred(player_id=3, runs=40.0, balls=24.0, wickets=1.0, economy=6.0),
    ]
    team1 = {1, 2}
    team2 = {3}
    out = rescale_player_predictions(
        preds,
        team1_ids=team1,
        team2_ids=team2,
        innings1_runs=120.0,
        innings1_wickets=3.0,
        innings2_runs=60.0,
        innings2_wickets=1.0,
    )
    assert len(out) == 3
    total_runs_team1 = sum(p.runs for p in out if p.player_id in team1)
    total_runs_team2 = sum(p.runs for p in out if p.player_id in team2)
    assert abs(total_runs_team1 - 120.0) < 0.01
    assert abs(total_runs_team2 - 60.0) < 0.01


def test_rescale_player_predictions_player_with_no_balls():
    """rescale_player_predictions skips economy recompute when balls is None or 0."""
    preds = [
        BacktestPlayerPred(player_id=1, runs=60.0, balls=None, wickets=0.0, economy=9.0),
        BacktestPlayerPred(player_id=2, runs=60.0, balls=36.0, wickets=2.0, economy=8.0),
    ]
    out = rescale_player_predictions(
        preds,
        team1_ids={1},
        team2_ids={2},
        innings1_runs=120.0,
        innings1_wickets=2.0,
        innings2_runs=60.0,
        innings2_wickets=0.0,
    )
    assert len(out) == 2
    assert out[0].economy == 9.0
