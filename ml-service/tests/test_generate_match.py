"""Tests for generate_match orchestration (reconciled scorecard + win probability).

The function fans out to the player pipeline and to two win models, so every test
here stubs those boundaries and asserts what generate_match itself decides: how it
splits players into innings, which win path it takes, and what it falls back to.
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, List

import pytest
from fastapi.testclient import TestClient

import app.prediction_service.generate_match as generate_match_module
from app.models.backtest import BacktestPlayerPred, InningsSummary, MatchContext
from app.models.predict import WinPrediction
from app.prediction_service.generate_match import generate_match
from app.prediction_settings import GenerateMatchSettings

CUTOFF = datetime(2026, 1, 1, tzinfo=timezone.utc)
TEAM1_IDS = [1, 2]
TEAM2_IDS = [3, 4]


@pytest.fixture
def settings() -> GenerateMatchSettings:
    """Settings pointing at nothing real: every collaborator is stubbed."""
    return GenerateMatchSettings(
        models_dir="/nonexistent",
        enable_train_on_the_fly=False,
        go_app_url="http://go-app.invalid",
        go_app_api_key=None,
        train_latest_cache_granularity="hour",
    )


@pytest.fixture
def match_context() -> MatchContext:
    """Two teams of two, with ids the win models can echo back."""
    return MatchContext(
        team1_player_ids=TEAM1_IDS,
        team2_player_ids=TEAM2_IDS,
        venue_id=7,
        format_id=3,
        team1_opposition_id=11,
        team2_opposition_id=12,
    )


@pytest.fixture
def features_map() -> Dict[str, Dict[str, float]]:
    """Per-player features keyed by player id as a string, as go-app sends them."""
    return {
        "1": {"batting_std_w10": 4.0, "bowling_std_w10": 1.0, "batting_mean_w5": 30.0, "bowling_mean_w5": 0.5},
        "2": {"batting_std_w10": 6.0, "bowling_std_w10": 2.0, "batting_mean_w5": 20.0, "bowling_mean_w5": 1.5},
        "3": {"batting_std_w10": 5.0, "bowling_std_w10": 3.0, "batting_mean_w5": 25.0, "bowling_mean_w5": 2.0},
        "4": {"batting_std_w10": 3.0, "bowling_std_w10": 4.0, "batting_mean_w5": 15.0, "bowling_mean_w5": 2.5},
    }


def _player_predictions() -> List[BacktestPlayerPred]:
    """Team1 scores 90 while taking 3 wickets; team2 scores 70 taking 2."""
    return [
        BacktestPlayerPred(player_id=1, runs=50.0, wickets=1.0),
        BacktestPlayerPred(player_id=2, runs=40.0, wickets=1.0),
        BacktestPlayerPred(player_id=3, runs=30.0, wickets=2.0),
        BacktestPlayerPred(player_id=4, runs=40.0, wickets=1.0),
    ]


@pytest.fixture
def stub_players(monkeypatch) -> List[Dict[str, Any]]:
    """Replace the player pipeline; return the list that records its calls."""
    calls: List[Dict[str, Any]] = []

    def fake_predict(*args: Any, **kwargs: Any) -> List[BacktestPlayerPred]:
        calls.append({"args": args, "kwargs": kwargs})
        return _player_predictions()

    monkeypatch.setattr(generate_match_module, "predict_players_with_features", fake_predict)
    return calls


def test_generate_match_aggregates_innings_from_team_assignment(
    monkeypatch, stub_players, settings, match_context, features_map
):
    """Innings 1 is team1's runs and the wickets team2's bowlers took, innings 2 the mirror."""
    monkeypatch.setattr(
        generate_match_module,
        "run_win_prediction_enhanced",
        lambda **kwargs: WinPrediction(team1_win_probability=0.73),
    )

    result = generate_match(CUTOFF, TEAM1_IDS + TEAM2_IDS, "t20", features_map, match_context, settings)

    innings = result["innings"]
    assert (innings[0].inning_number, innings[0].runs, innings[0].wickets) == (1, 90.0, 3.0)
    assert (innings[1].inning_number, innings[1].runs, innings[1].wickets) == (2, 70.0, 2.0)
    assert result["players"] == _player_predictions()


def test_generate_match_uses_the_enhanced_win_probability(
    monkeypatch, stub_players, settings, match_context, features_map
):
    """With features present the enhanced model decides, and the legacy model is never called."""
    seen: Dict[str, Any] = {}

    def fake_enhanced(**kwargs: Any) -> WinPrediction:
        seen.update(kwargs)
        return WinPrediction(team1_win_probability=0.73)

    def fail_legacy(features):
        raise AssertionError("legacy win model must not run when the enhanced model answered")

    monkeypatch.setattr(generate_match_module, "run_win_prediction_enhanced", fake_enhanced)
    monkeypatch.setattr(generate_match_module, "run_win_prediction", fail_legacy)

    result = generate_match(CUTOFF, TEAM1_IDS + TEAM2_IDS, "t20", features_map, match_context, settings)

    assert result["win_probability_team1"] == 0.73
    assert seen["fmt"] == "T20"
    assert seen["match_context"] == {
        "format_id": 3.0,
        "venue_id": 7.0,
        "team1_opposition_id": 11.0,
        "team2_opposition_id": 12.0,
    }
    assert sorted(seen["team1_player_features"]) == ["1", "2"]
    assert sorted(seen["team2_player_features"]) == ["3", "4"]
    assert seen["team1_player_features"]["1"] == features_map["1"]


def test_generate_match_falls_back_to_the_legacy_win_model(
    monkeypatch, stub_players, settings, match_context, features_map
):
    """An enhanced-model failure is logged and the sum-based legacy model answers instead."""
    legacy_features = []

    def fake_legacy(features):
        legacy_features.extend(features)
        return [WinPrediction(team1_win_probability=0.61)]

    monkeypatch.setattr(
        generate_match_module,
        "run_win_prediction_enhanced",
        lambda **kwargs: (_ for _ in ()).throw(RuntimeError("no win model loaded")),
    )
    monkeypatch.setattr(generate_match_module, "run_win_prediction", fake_legacy)

    result = generate_match(CUTOFF, TEAM1_IDS + TEAM2_IDS, "t20", features_map, match_context, settings)

    assert result["win_probability_team1"] == 0.61
    win_features = legacy_features[0]
    assert win_features.format == "T20"
    assert win_features.team1_bat_consistency_sum == 10.0
    assert win_features.team2_bowl_form_sum == 4.5


def test_generate_match_without_features_skips_the_enhanced_model(monkeypatch, stub_players, settings, match_context):
    """No features means nothing to aggregate, so only the legacy model is consulted."""

    def fail_enhanced(**kwargs: Any) -> WinPrediction:
        raise AssertionError("enhanced win model must not run without features")

    monkeypatch.setattr(generate_match_module, "run_win_prediction_enhanced", fail_enhanced)
    monkeypatch.setattr(
        generate_match_module, "run_win_prediction", lambda features: [WinPrediction(team1_win_probability=0.44)]
    )

    result = generate_match(CUTOFF, TEAM1_IDS + TEAM2_IDS, "T20", {}, match_context, settings)

    assert result["win_probability_team1"] == 0.44


def test_generate_match_keeps_an_even_probability_when_no_win_model_predicts(
    monkeypatch, stub_players, settings, match_context
):
    """An empty legacy result leaves the even prior rather than inventing a winner."""
    monkeypatch.setattr(generate_match_module, "run_win_prediction", lambda features: [])

    result = generate_match(CUTOFF, TEAM1_IDS + TEAM2_IDS, "T20", {}, match_context, settings)

    assert result["win_probability_team1"] == 0.5


def test_generate_match_survives_an_unreadable_coherence_config(monkeypatch, stub_players, settings, match_context):
    """Coherence logging is diagnostics: a broken config must not fail the prediction."""
    monkeypatch.setattr(
        generate_match_module,
        "get_win_coherence_config",
        lambda: (_ for _ in ()).throw(OSError("config unreadable")),
    )
    monkeypatch.setattr(
        generate_match_module, "run_win_prediction", lambda features: [WinPrediction(team1_win_probability=0.55)]
    )

    result = generate_match(CUTOFF, TEAM1_IDS + TEAM2_IDS, "T20", {}, match_context, settings, model_version="v9")

    assert result["win_probability_team1"] == 0.55
    assert result["model_version"] == "v9"


def test_generate_match_forwards_settings_and_context_to_the_player_pipeline(
    monkeypatch, stub_players, settings, match_context
):
    """Reconciliation only runs when match_context reaches the player pipeline."""
    monkeypatch.setattr(
        generate_match_module, "run_win_prediction", lambda features: [WinPrediction(team1_win_probability=0.5)]
    )

    generate_match(CUTOFF, TEAM1_IDS + TEAM2_IDS, "T20", {}, match_context, settings, use_latest_model=True)

    args = stub_players[0]["args"]
    assert args[0] is CUTOFF
    assert args[1] == TEAM1_IDS + TEAM2_IDS
    assert args[4] == settings.models_dir
    assert args[6] == settings.go_app_url
    assert args[9] is True
    assert stub_players[0]["kwargs"]["match_context"] is match_context


@pytest.fixture
def route_client(monkeypatch):
    """TestClient for /api/ml/generate-match with the orchestration stubbed out."""
    import app.main as main_module

    def build(result_or_error):
        def fake_generate_match(*args: Any, **kwargs: Any) -> Dict[str, Any]:
            if isinstance(result_or_error, Exception):
                raise result_or_error
            return result_or_error

        monkeypatch.setattr(main_module, "generate_match", fake_generate_match)
        return TestClient(main_module.app)

    return build


def _generate_match_request() -> Dict[str, Any]:
    return {
        "cutoff_date": "2026-01-01T00:00:00Z",
        "player_ids": TEAM1_IDS + TEAM2_IDS,
        "format": "T20",
        "features": {},
        "match_context": {"team1_player_ids": TEAM1_IDS, "team2_player_ids": TEAM2_IDS},
    }


def test_generate_match_route_returns_the_reconciled_match(route_client):
    """The route serializes players, innings and win probability from the orchestrator."""
    client = route_client(
        {
            "players": _player_predictions(),
            "innings": [
                InningsSummary(inning_number=1, runs=90.0, wickets=3.0),
                InningsSummary(inning_number=2, runs=70.0, wickets=2.0),
            ],
            "win_probability_team1": 0.73,
            "model_version": "v9",
        }
    )

    resp = client.post("/api/ml/generate-match", json=_generate_match_request())

    assert resp.status_code == 200
    body = resp.json()
    assert body["win_probability_team1"] == 0.73
    assert body["model_version"] == "v9"
    assert [i["runs"] for i in body["innings"]] == [90.0, 70.0]
    assert len(body["players"]) == 4


def test_generate_match_route_reports_bad_input_as_a_client_error(route_client):
    """A ValueError is the caller's data, not a server fault, so it answers 400."""
    client = route_client(ValueError("unknown format"))

    resp = client.post("/api/ml/generate-match", json=_generate_match_request())

    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "VALIDATION_FAILED"


def test_generate_match_route_reports_a_failed_prediction_as_unavailable(route_client):
    """Anything else means the models could not answer: 503, with the reason."""
    client = route_client(RuntimeError("no batting model for T20"))

    resp = client.post("/api/ml/generate-match", json=_generate_match_request())

    assert resp.status_code == 503
    assert resp.json()["detail"]["code"] == "GENERATE_MATCH_FAILED"
