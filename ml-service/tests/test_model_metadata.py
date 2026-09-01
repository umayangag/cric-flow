"""app.model_metadata: what GET /model-metadata tells the Workbench about the win model."""

from unittest.mock import patch

from app.model_metadata import WIN_OUTPUTS, get_model_metadata


def test_get_model_metadata_describes_only_the_win_model() -> None:
    """One card is left: the batting, bowling, fielding, extras, innings and
    combination-meta models went in P-5, and a card for a model that cannot exist is a
    dead panel."""
    meta = get_model_metadata()

    assert set(meta) == {"win"}


def test_get_model_metadata_win_structure() -> None:
    """The win card carries its feature order, its output and how its artifacts are named."""
    win = get_model_metadata()["win"]

    assert win["outputs"] == WIN_OUTPUTS
    assert win["level"] == "match"
    assert win["hasScaler"] is False
    assert win["artifactsPattern"]["perFormat"] == "win_model_<FMT>.joblib"
    assert isinstance(win["features"], list)
    assert win["features"], "the win model's feature order comes from ml.win_features"


def test_get_model_metadata_survives_an_unimportable_feature_order() -> None:
    """A metadata endpoint must answer even when the feature module cannot be imported:
    an empty list is a fact the UI can render, an exception is a 500."""
    with patch.dict("sys.modules", {"ml.win_features": None}):
        win = get_model_metadata()["win"]

    assert win["features"] == []
    assert win["outputs"] == WIN_OUTPUTS
