"""Contract tests for the enhanced win model's inputs.

Two defects motivate this file. ``to_match_context_dict`` read seven weather
attributes that had been removed from the model, so ``POST /predict/win-enhanced``
raised ``AttributeError`` on every request and no test noticed. And the toss winner was
a training feature that the serving path could only ever send as zero, because a side is
selected before the toss is taken.
"""

from app.models.predict import WinFeaturesEnhanced
from ml.win_features import (
    MATCH_CONTEXT_BASE_COLS,
    WIN_ENHANCED_FEATURE_COLS,
    aggregate_team_features_from_player_maps,
)


def _request() -> WinFeaturesEnhanced:
    """A minimal well-formed enhanced win request."""
    return WinFeaturesEnhanced(
        format_id=1,
        venue_id=2,
        team1_opposition_id=3,
        team2_opposition_id=4,
        team1_player_features={"10": {"batting_mean_w5": 1.0, "bowling_std_w10": 0.5}},
        team2_player_features={"20": {"batting_mean_w5": 2.0, "bowling_std_w10": 1.5}},
        format="T20",
    )


def test_to_match_context_dict_does_not_raise() -> None:
    """The builder must only read fields the model declares."""
    context = _request().to_match_context_dict()

    assert context["venue_id"] == 2.0
    assert context["team1_opposition_id"] == 3.0


def test_match_context_covers_exactly_the_declared_context_columns() -> None:
    """Every context column the aggregator reads must be supplied, and no extras."""
    context = _request().to_match_context_dict()

    assert set(context) == set(MATCH_CONTEXT_BASE_COLS)


def test_aggregation_accepts_the_request_context() -> None:
    """The request's context must flow into a full feature vector without KeyError."""
    request = _request()

    features = aggregate_team_features_from_player_maps(
        {10: {"batting_mean_w5": 1.0, "bowling_std_w10": 0.5}},
        {20: {"batting_mean_w5": 2.0, "bowling_std_w10": 1.5}},
        request.to_match_context_dict(),
        format_code="T20",
    )

    for column in WIN_ENHANCED_FEATURE_COLS:
        assert column in features, f"aggregation did not produce {column}"


def test_toss_winner_is_not_a_win_feature() -> None:
    """The toss is unknown when a side is picked, so it cannot be an input."""
    assert "toss_winner_opposition_id" not in WIN_ENHANCED_FEATURE_COLS
    assert "toss_winner_opposition_id" not in MATCH_CONTEXT_BASE_COLS
    assert not hasattr(_request(), "toss_winner_opposition_id")
