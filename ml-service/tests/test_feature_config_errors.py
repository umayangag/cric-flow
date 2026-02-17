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
