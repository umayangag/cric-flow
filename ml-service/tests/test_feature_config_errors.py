"""Tests for feature_config error paths to improve coverage."""

import importlib
import json

import pytest


def test_get_feature_names_unknown_kind():
    from app.feature_config import get_feature_names

    with pytest.raises(ValueError, match="unknown feature kind"):
        get_feature_names("unknown")


def test_get_feature_names_file_not_found(tmp_path, monkeypatch):
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(tmp_path / "nonexistent.json"))
    mod = importlib.import_module("app.feature_config")
    importlib.reload(mod)
    mod._load_config.cache_clear()
    with pytest.raises(mod.FeatureConfigError):
        mod.get_feature_names("batting")


def test_get_feature_names_not_a_dict(tmp_path, monkeypatch):
    path = tmp_path / "config.json"
    path.write_text("[1, 2, 3]", encoding="utf-8")
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(path))
    mod = importlib.import_module("app.feature_config")
    importlib.reload(mod)
    mod._load_config.cache_clear()
    with pytest.raises(mod.FeatureConfigError):
        mod.get_feature_names("batting")


def test_get_feature_names_missing_or_empty_list(tmp_path, monkeypatch):
    path = tmp_path / "config.json"
    path.write_text(json.dumps({"batting": []}), encoding="utf-8")
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(path))
    mod = importlib.import_module("app.feature_config")
    importlib.reload(mod)
    mod._load_config.cache_clear()
    with pytest.raises(mod.FeatureConfigError):
        mod.get_feature_names("batting")


def test_get_feature_names_non_string_in_list(tmp_path, monkeypatch):
    path = tmp_path / "config.json"
    path.write_text(json.dumps({"batting": ["a", 1], "bowling": ["x"]}), encoding="utf-8")
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(path))
    mod = importlib.import_module("app.feature_config")
    importlib.reload(mod)
    mod._load_config.cache_clear()
    with pytest.raises(mod.FeatureConfigError):
        mod.get_feature_names("batting")


def test_get_feature_names_json_decode_error(tmp_path, monkeypatch):
    """Invalid JSON raises FeatureConfigError (covers json.JSONDecodeError path)."""
    path = tmp_path / "bad.json"
    path.write_text("{ invalid json }", encoding="utf-8")
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(path))
    mod = importlib.import_module("app.feature_config")
    importlib.reload(mod)
    mod._load_config.cache_clear()
    with pytest.raises(mod.FeatureConfigError):
        mod.get_feature_names("batting")


def test_raw_stat_prefix_unknown_kind():
    """_raw_stat_prefix raises ValueError for non-batting/bowling kind."""
    from app.feature_config import _raw_stat_prefix

    with pytest.raises(ValueError, match="unknown feature kind for raw stats"):
        _raw_stat_prefix("fielding")
    with pytest.raises(ValueError, match="unknown feature kind for raw stats"):
        _raw_stat_prefix("unknown")


def test_get_raw_stat_feature_names_markers_missing(tmp_path, monkeypatch):
    """get_raw_stat_feature_names raises when start/end markers not in config list."""
    path = tmp_path / "config.json"
    # Batting list without batting_mean_w3 / batting_innings_in_last_90d
    path.write_text(
        json.dumps({"batting": ["other_a", "other_b"], "bowling": ["bowling_mean_w3", "bowling_innings_in_last_90d"]}),
        encoding="utf-8",
    )
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(path))
    mod = importlib.import_module("app.feature_config")
    importlib.reload(mod)
    mod._load_config.cache_clear()
    mod.get_raw_stat_feature_names.cache_clear()
    with pytest.raises(mod.FeatureConfigError, match="Raw stat markers"):
        mod.get_raw_stat_feature_names("batting")


def test_get_raw_stat_feature_names_end_before_start(tmp_path, monkeypatch):
    """get_raw_stat_feature_names raises when end marker appears before start marker."""
    path = tmp_path / "config.json"
    # Batting list with end marker before start marker
    path.write_text(
        json.dumps(
            {
                "batting": ["batting_innings_in_last_90d", "batting_mean_w3", "batting_mean_w5"],
                "bowling": ["bowling_mean_w3", "bowling_innings_in_last_90d"],
            }
        ),
        encoding="utf-8",
    )
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(path))
    mod = importlib.import_module("app.feature_config")
    importlib.reload(mod)
    mod._load_config.cache_clear()
    mod.get_raw_stat_feature_names.cache_clear()
    with pytest.raises(mod.FeatureConfigError, match="must appear after"):
        mod.get_raw_stat_feature_names("batting")
