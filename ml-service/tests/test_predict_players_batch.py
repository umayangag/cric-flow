"""Tests for the batched prediction flow in predict_players_batch.

Verifies that cross-item feature-matrix aggregation produces the same results
as calling predict_players_with_features per item, and that the batching
logic correctly partitions results back.
"""

from __future__ import annotations

from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from app.models import BatchPredictItem
from app.prediction_service import (
    _assemble_player_predictions,
    _resolve_prediction_model_pairs,
    predict_players_batch,
    predict_players_with_features,
)

N_BAT_FEATURES = 15
N_BOWL_FEATURES = 12
N_FLD_FEATURES = 8


def _make_mock_model(n_outputs: int) -> MagicMock:
    """Create a mock sklearn model that returns deterministic predictions based on input sums."""
    model = MagicMock()

    def _predict(X):
        out = np.zeros((X.shape[0], n_outputs))
        for i in range(X.shape[0]):
            row_sum = float(np.sum(X[i]))
            for j in range(n_outputs):
                out[i, j] = abs(row_sum * (j + 1) * 0.1)
        return out

    model.predict = MagicMock(side_effect=_predict)
    return model


def _make_mock_scaler() -> MagicMock:
    scaler = MagicMock()
    scaler.transform = MagicMock(side_effect=lambda X: X)
    return scaler


CUTOFF = datetime(2024, 10, 30, 14, 0, 0, tzinfo=timezone.utc)
PLAYER_IDS = [101, 102, 103]
FEATURES: dict = {
    "101": {"batting_consistency": 0.5, "batting_form": 20.0},
    "102": {"batting_consistency": 0.4, "batting_form": 15.0},
    "103": {"batting_consistency": 0.6, "batting_form": 25.0},
}


def _fake_bat_feature_matrix(player_ids, cutoff, fmt_upper, features_map, models_dir):
    """Return a deterministic unscaled batting feature matrix."""
    rows = []
    for pid in player_ids:
        fm = features_map.get(str(pid), {})
        row = np.full(N_BAT_FEATURES, float(pid) * 0.01 + fm.get("batting_consistency", 0))
        rows.append(row)
    return np.array(rows, dtype=float)


def _fake_bowl_feature_matrix(player_ids, cutoff, fmt_upper, features_map, models_dir):
    """Return a deterministic unscaled bowling feature matrix."""
    rows = []
    for pid in player_ids:
        fm = features_map.get(str(pid), {})
        row = np.full(N_BOWL_FEATURES, float(pid) * 0.02 + fm.get("batting_form", 0))
        rows.append(row)
    return np.array(rows, dtype=float)


def _fake_fld_feature_matrix(player_ids, cutoff, fmt_upper, features_map):
    """Return a deterministic unscaled fielding feature matrix."""
    rows = []
    for pid in player_ids:
        row = np.full(N_FLD_FEATURES, float(pid) * 0.005)
        rows.append(row)
    return np.array(rows, dtype=float)


@pytest.fixture()
def _mock_artifacts():
    """Patch global model registries and feature builders so prediction code paths execute without real data."""
    bat_model = _make_mock_model(5)
    bowl_model = _make_mock_model(3)
    fld_model = _make_mock_model(2)
    bat_scaler = _make_mock_scaler()
    bowl_scaler = _make_mock_scaler()
    fld_scaler = _make_mock_scaler()

    with (
        patch.dict("app.prediction_service.players.BAT_MODELS", {"T20": (bat_scaler, bat_model)}),
        patch.dict("app.prediction_service.players.BOWL_MODELS", {"T20": (bowl_scaler, bowl_model)}),
        patch.dict("app.prediction_service.players.FIELD_MODELS", {"T20": (fld_scaler, fld_model)}),
        patch.dict("app.prediction_service.players.BAT_SHARE_MODELS", {}),
        patch.dict("app.prediction_service.players.BOWL_SHARE_MODELS", {}),
        patch.dict("app.prediction_service.players.INNINGS_MODELS", {}),
        patch("app.prediction_service.players.use_share_models_config", return_value=False),
        patch("app.prediction_service.players.get_prediction_defaults", return_value={"economy": 6.0}),
        patch("app.prediction_service.players._build_batting_feature_matrix", side_effect=_fake_bat_feature_matrix),
        patch("app.prediction_service.players._build_bowling_feature_matrix", side_effect=_fake_bowl_feature_matrix),
        patch("app.prediction_service.players._build_fielding_feature_matrix", side_effect=_fake_fld_feature_matrix),
    ):
        yield {
            "bat_model": bat_model,
            "bowl_model": bowl_model,
            "fld_model": fld_model,
        }


def test_predict_players_batch_empty_items() -> None:
    """Empty batch returns empty list without calling any models."""
    result = predict_players_batch(
        items=[],
        models_dir="/tmp",
        enable_train_on_the_fly=False,
        go_app_url="",
        go_app_api_key=None,
        train_latest_cache_granularity="hour",
    )
    assert result == []


@pytest.mark.usefixtures("_mock_artifacts")
def test_predict_players_batch_single_item_matches_direct_call(_mock_artifacts) -> None:
    """A single-item batch must produce the same results as predict_players_with_features."""
    direct_preds = predict_players_with_features(
        CUTOFF,
        PLAYER_IDS,
        "T20",
        FEATURES,
        "/tmp",
        False,
        "",
        None,
        "hour",
    )

    batch_item = BatchPredictItem(
        cutoff_date=CUTOFF,
        player_ids=PLAYER_IDS,
        format="T20",
        features=FEATURES,
    )
    batch_results = predict_players_batch(
        items=[batch_item],
        models_dir="/tmp",
        enable_train_on_the_fly=False,
        go_app_url="",
        go_app_api_key=None,
        train_latest_cache_granularity="hour",
    )

    assert len(batch_results) == 1
    batch_preds = batch_results[0]
    assert len(batch_preds) == len(direct_preds)
    for bp, dp in zip(batch_preds, direct_preds):
        assert bp.player_id == dp.player_id
        assert bp.runs == pytest.approx(dp.runs, abs=1e-9)
        assert bp.wickets == pytest.approx(dp.wickets, abs=1e-9)
        assert bp.economy == pytest.approx(dp.economy, abs=1e-9)
        assert bp.catches == pytest.approx(dp.catches, abs=1e-9)
        assert bp.run_outs == pytest.approx(dp.run_outs, abs=1e-9)


@pytest.mark.usefixtures("_mock_artifacts")
def test_predict_players_batch_multiple_items_same_model(_mock_artifacts) -> None:
    """Multiple items sharing the same model use one batched model.predict() call per model type."""
    items = [
        BatchPredictItem(cutoff_date=CUTOFF, player_ids=[101, 102], format="T20", features=FEATURES),
        BatchPredictItem(cutoff_date=CUTOFF, player_ids=[103], format="T20", features=FEATURES),
    ]
    batch_results = predict_players_batch(
        items=items,
        models_dir="/tmp",
        enable_train_on_the_fly=False,
        go_app_url="",
        go_app_api_key=None,
        train_latest_cache_granularity="hour",
    )

    assert len(batch_results) == 2
    assert len(batch_results[0]) == 2
    assert len(batch_results[1]) == 1

    bat_model = _mock_artifacts["bat_model"]
    bowl_model = _mock_artifacts["bowl_model"]
    fld_model = _mock_artifacts["fld_model"]
    assert bat_model.predict.call_count == 1, "Batting model should be called once for both items"
    assert bowl_model.predict.call_count == 1, "Bowling model should be called once for both items"
    assert fld_model.predict.call_count == 1, "Fielding model should be called once for both items"

    concat_bat_arg = bat_model.predict.call_args[0][0]
    assert concat_bat_arg.shape[0] == 3, "Concatenated batting matrix should have 3 rows (2+1 players)"


@pytest.mark.usefixtures("_mock_artifacts")
def test_predict_players_batch_results_partitioned_correctly(_mock_artifacts) -> None:
    """Verify that batched predictions are correctly partitioned back to each item."""
    items = [
        BatchPredictItem(cutoff_date=CUTOFF, player_ids=[101, 102], format="T20", features=FEATURES),
        BatchPredictItem(cutoff_date=CUTOFF, player_ids=[103], format="T20", features=FEATURES),
    ]
    batch_results = predict_players_batch(
        items=items,
        models_dir="/tmp",
        enable_train_on_the_fly=False,
        go_app_url="",
        go_app_api_key=None,
        train_latest_cache_granularity="hour",
    )

    direct_preds_item0 = predict_players_with_features(
        CUTOFF,
        [101, 102],
        "T20",
        FEATURES,
        "/tmp",
        False,
        "",
        None,
        "hour",
    )
    direct_preds_item1 = predict_players_with_features(
        CUTOFF,
        [103],
        "T20",
        FEATURES,
        "/tmp",
        False,
        "",
        None,
        "hour",
    )

    for bp, dp in zip(batch_results[0], direct_preds_item0):
        assert bp.player_id == dp.player_id
        assert bp.runs == pytest.approx(dp.runs, abs=1e-9)
    for bp, dp in zip(batch_results[1], direct_preds_item1):
        assert bp.player_id == dp.player_id
        assert bp.runs == pytest.approx(dp.runs, abs=1e-9)


def test_assemble_player_predictions_basic() -> None:
    """_assemble_player_predictions converts raw arrays into BacktestPlayerPred list."""
    Y_bat = np.array([[10.0, 20.0, 2.0, 1.0, 0.0], [5.0, 10.0, 1.0, 0.0, 0.0]])
    Y_bowl = np.array([[8.0, 24.0, 1.0], [12.0, 30.0, 2.0]])

    with patch("app.prediction_service.players.get_prediction_defaults", return_value={"economy": 6.0}):
        preds = _assemble_player_predictions(
            Y_bat,
            Y_bowl,
            [1, 2],
            False,
            None,
            CUTOFF,
            "T20",
            {},
        )

    assert len(preds) == 2
    assert preds[0].player_id == 1
    assert preds[0].runs == pytest.approx(10.0)
    assert preds[0].balls == pytest.approx(20.0)
    assert preds[0].fours == pytest.approx(2.0)
    assert preds[0].wickets == pytest.approx(1.0)
    assert preds[0].economy == pytest.approx(8.0 / (24.0 / 6.0))
    assert preds[1].player_id == 2
    assert preds[1].runs == pytest.approx(5.0)


def test_assemble_player_predictions_with_fielding() -> None:
    """_assemble_player_predictions merges fielding predictions when Y_fld is provided."""
    Y_bat = np.array([[10.0, 20.0, 2.0, 1.0, 0.0]])
    Y_bowl = np.array([[8.0, 24.0, 1.0]])
    Y_fld = np.array([[1.5, 0.3]])

    with patch("app.prediction_service.players.get_prediction_defaults", return_value={"economy": 6.0}):
        preds = _assemble_player_predictions(
            Y_bat,
            Y_bowl,
            [1],
            False,
            None,
            CUTOFF,
            "T20",
            {},
            Y_fld=Y_fld,
        )

    assert len(preds) == 1
    assert preds[0].catches == pytest.approx(1.5)
    assert preds[0].run_outs == pytest.approx(0.3)


def test_assemble_player_predictions_clamps_negative_values() -> None:
    """Negative raw predictions should be clamped to zero."""
    Y_bat = np.array([[-5.0, -1.0, -2.0, -3.0, 0.0]])
    Y_bowl = np.array([[-8.0, -24.0, -1.0]])

    with patch("app.prediction_service.players.get_prediction_defaults", return_value={"economy": 6.0}):
        preds = _assemble_player_predictions(
            Y_bat,
            Y_bowl,
            [1],
            False,
            None,
            CUTOFF,
            "T20",
            {},
        )

    assert preds[0].runs == 0.0
    assert preds[0].balls == 0.0
    assert preds[0].wickets == 0.0


def test_resolve_prediction_model_pairs_raises_when_no_artifacts_and_no_train() -> None:
    """_resolve_prediction_model_pairs raises ValueError when no artifacts and train-on-the-fly disabled."""
    with (
        patch.dict("app.prediction_service.players.BAT_MODELS", {}),
        patch.dict("app.prediction_service.players.BOWL_MODELS", {}),
        patch.dict("app.prediction_service.players.BAT_SHARE_MODELS", {}),
        patch.dict("app.prediction_service.players.BOWL_SHARE_MODELS", {}),
        patch.dict("app.prediction_service.players.INNINGS_MODELS", {}),
        patch("app.prediction_service.players.use_share_models_config", return_value=False),
    ):
        with pytest.raises(ValueError, match="Train-on-the-fly is disabled"):
            _resolve_prediction_model_pairs(
                "T20",
                "/tmp",
                False,
                "",
                None,
                "hour",
                CUTOFF,
                False,
                3,
                has_match_context=False,
            )


def test_resolve_prediction_model_pairs_raises_when_no_go_app_url() -> None:
    """_resolve_prediction_model_pairs raises ValueError when go_app_url missing for train-on-the-fly."""
    with (
        patch.dict("app.prediction_service.players.BAT_MODELS", {}),
        patch.dict("app.prediction_service.players.BOWL_MODELS", {}),
        patch.dict("app.prediction_service.players.BAT_SHARE_MODELS", {}),
        patch.dict("app.prediction_service.players.BOWL_SHARE_MODELS", {}),
        patch.dict("app.prediction_service.players.INNINGS_MODELS", {}),
        patch("app.prediction_service.players.use_share_models_config", return_value=False),
    ):
        with pytest.raises(ValueError, match="GO_APP_URL is required"):
            _resolve_prediction_model_pairs(
                "T20",
                "/tmp",
                True,
                "",
                None,
                "hour",
                CUTOFF,
                False,
                3,
                has_match_context=False,
            )


def test_resolve_prediction_model_pairs_returns_loaded_models() -> None:
    """_resolve_prediction_model_pairs returns loaded model pairs when available."""
    bat_pair = (_make_mock_scaler(), _make_mock_model(5))
    bowl_pair = (_make_mock_scaler(), _make_mock_model(3))

    with (
        patch.dict("app.prediction_service.players.BAT_MODELS", {"T20": bat_pair}),
        patch.dict("app.prediction_service.players.BOWL_MODELS", {"T20": bowl_pair}),
        patch.dict("app.prediction_service.players.INNINGS_MODELS", {}),
        patch("app.prediction_service.players.use_share_models_config", return_value=False),
    ):
        resolved = _resolve_prediction_model_pairs(
            "T20",
            "/tmp",
            False,
            "",
            None,
            "hour",
            CUTOFF,
            False,
            3,
            has_match_context=False,
        )

    assert resolved.fmt_upper == "T20"
    assert resolved.use_share is False
    assert resolved.model_bat is bat_pair[1]
    assert resolved.model_bowl is bowl_pair[1]


def test_resolve_prediction_model_pairs_normalizes_format() -> None:
    """Format string should be upper-cased and stripped."""
    bat_pair = (_make_mock_scaler(), _make_mock_model(5))
    bowl_pair = (_make_mock_scaler(), _make_mock_model(3))

    with (
        patch.dict("app.prediction_service.players.BAT_MODELS", {"ODI": bat_pair}),
        patch.dict("app.prediction_service.players.BOWL_MODELS", {"ODI": bowl_pair}),
        patch.dict("app.prediction_service.players.INNINGS_MODELS", {}),
        patch("app.prediction_service.players.use_share_models_config", return_value=False),
    ):
        resolved = _resolve_prediction_model_pairs(
            " odi ",
            "/tmp",
            False,
            "",
            None,
            "hour",
            CUTOFF,
            False,
            3,
            has_match_context=False,
        )

    assert resolved.fmt_upper == "ODI"
