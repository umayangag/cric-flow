"""Unit tests for app.model_metadata."""

from unittest.mock import patch

from app.model_metadata import (
    BATTING_OUTPUTS,
    BOWLING_OUTPUTS,
    EXTRAS_OUTPUTS,
    FIELDING_OUTPUTS,
    WIN_OUTPUTS,
    get_model_metadata,
    get_model_modes,
)


def test_get_model_modes_registry():
    """Model mode registry includes legacy and per_format with name, available, deprecated, description."""
    modes = get_model_modes()
    assert len(modes) >= 2
    names = [m["name"] for m in modes]
    assert "legacy" in names
    assert "per_format" in names
    for m in modes:
        assert "name" in m
        assert "available" in m
        assert "deprecated" in m
        assert "description" in m


def test_get_model_metadata_returns_all_kinds():
    """get_model_metadata returns model_modes, batting, bowling, fielding, extras, win, combination_meta."""
    meta = get_model_metadata()
    assert "model_modes" in meta
    assert isinstance(meta["model_modes"], list)
    assert "batting" in meta
    assert "bowling" in meta
    assert "fielding" in meta
    assert "extras" in meta
    assert "win" in meta
    assert "combination_meta" in meta


def test_get_model_metadata_batting_structure():
    """Batting metadata has features, outputs, level, hasScaler, artifactsPattern, note."""
    meta = get_model_metadata()
    bat = meta["batting"]
    assert "features" in bat
    assert "outputs" in bat
    assert bat["outputs"] == BATTING_OUTPUTS
    assert bat["level"] == "player"
    assert bat["hasScaler"] is True
    assert "artifactsPattern" in bat
    assert "perFormat" in bat["artifactsPattern"]
    assert "legacy" in bat["artifactsPattern"]
    assert "note" in bat


def test_get_model_metadata_bowling_structure():
    """Bowling metadata has correct outputs."""
    meta = get_model_metadata()
    bowl = meta["bowling"]
    assert bowl["outputs"] == BOWLING_OUTPUTS
    assert bowl["level"] == "player"


def test_get_model_metadata_fielding_structure():
    """Fielding metadata has correct outputs."""
    meta = get_model_metadata()
    field = meta["fielding"]
    assert field["outputs"] == FIELDING_OUTPUTS


def test_get_model_metadata_extras_win():
    """Extras and win are match-level, no scaler."""
    meta = get_model_metadata()
    assert meta["extras"]["level"] == "match"
    assert meta["extras"]["hasScaler"] is False
    assert meta["extras"]["outputs"] == EXTRAS_OUTPUTS
    assert meta["win"]["level"] == "match"
    assert meta["win"]["outputs"] == WIN_OUTPUTS


def test_get_model_metadata_combination_meta():
    """combination_meta has meta level and static features/outputs."""
    meta = get_model_metadata()
    cm = meta["combination_meta"]
    assert cm["level"] == "meta"
    assert "features" in cm
    assert "outputs" in cm
    assert "score_weights" in str(cm["outputs"])


def test_get_model_metadata_handles_feature_config_exception():
    """When get_feature_names raises, features list is empty."""
    with patch("app.model_metadata.get_feature_names", side_effect=Exception("config error")):
        meta = get_model_metadata()
    for kind in ("batting", "bowling", "fielding"):
        assert meta[kind]["features"] == []


def test_get_model_metadata_extras_win_import_error():
    """When ml.train_extras or ml.win_features import fails, features list is empty."""
    import builtins
    import sys

    real_import = builtins.__import__
    blocked = ("ml.train_extras", "ml.win_features")
    saved = {}
    for mod in blocked:
        if mod in sys.modules:
            saved[mod] = sys.modules.pop(mod)

    def fake_import(name, *args, **kwargs):
        if name in blocked:
            raise ImportError("fake")
        return real_import(name, *args, **kwargs)

    try:
        with patch.object(builtins, "__import__", fake_import):
            meta = get_model_metadata()
        assert meta["extras"]["features"] == []
        assert meta["win"]["features"] == []
    finally:
        sys.modules.update(saved)
