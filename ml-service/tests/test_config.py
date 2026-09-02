"""Unit tests for ml.config: load, merge, and the settings the service still honours."""

from unittest.mock import patch

import ml.config as config_mod
from ml.config import (
    DEFAULT_RATINGS_MAX_AGE_DAYS,
    _deep_merge,
    default_artifacts_dir,
    get_format_codes,
    get_ratings_max_age_days,
    get_training_subprocess_timeout_sec,
)


def test_deep_merge_shallow():
    """_deep_merge with no nested dicts overwrites keys."""
    base = {"a": 1, "b": 2}
    override = {"b": 20, "c": 3}
    out = _deep_merge(base, override)
    assert out == {"a": 1, "b": 20, "c": 3}
    assert base == {"a": 1, "b": 2}  # base unchanged


def test_deep_merge_nested():
    """_deep_merge merges nested dicts recursively."""
    base = {"a": {"x": 1, "y": 2}, "b": 3}
    override = {"a": {"y": 20, "z": 4}}
    out = _deep_merge(base, override)
    assert out == {"a": {"x": 1, "y": 20, "z": 4}, "b": 3}


def test_deep_merge_override_replaces_non_dict():
    """When override value is not a dict, it replaces base value."""
    base = {"a": {"x": 1}}
    override = {"a": "string"}
    out = _deep_merge(base, override)
    assert out == {"a": "string"}


def test_default_artifacts_dir_from_config(monkeypatch):
    """default_artifacts_dir returns config value when set."""
    config_mod._cached = {"outputs": {"artifacts_dir": "/custom/artifacts"}}
    try:
        assert default_artifacts_dir() == "/custom/artifacts"
    finally:
        config_mod._cached = None


def test_get_training_subprocess_timeout_sec_from_config():
    """training_subprocess_timeout_sec from config when set."""
    config_mod._cached = {"inputs": {"training_subprocess_timeout_sec": 120}}
    try:
        assert get_training_subprocess_timeout_sec() == 120
    finally:
        config_mod._cached = None


def test_get_training_subprocess_timeout_sec_default_7_days(monkeypatch):
    """When config and env are unset, default is 7 days (604800 sec)."""
    config_mod._cached = {"inputs": {}}
    monkeypatch.delenv("TRAINING_SUBPROCESS_TIMEOUT_SEC", raising=False)
    try:
        assert get_training_subprocess_timeout_sec() == 7 * 24 * 3600
    finally:
        config_mod._cached = None


def test_get_training_subprocess_timeout_sec_invalid_config_falls_back(monkeypatch):
    """Invalid config value (str/non-int) falls back to env or default."""
    config_mod._cached = {"inputs": {"training_subprocess_timeout_sec": "not_an_int"}}
    monkeypatch.delenv("TRAINING_SUBPROCESS_TIMEOUT_SEC", raising=False)
    try:
        assert get_training_subprocess_timeout_sec() == 7 * 24 * 3600
    finally:
        config_mod._cached = None


def test_get_training_subprocess_timeout_sec_invalid_env_falls_back(monkeypatch):
    """Invalid env value falls back to default."""
    config_mod._cached = {"inputs": {}}
    monkeypatch.setenv("TRAINING_SUBPROCESS_TIMEOUT_SEC", "invalid")
    try:
        assert get_training_subprocess_timeout_sec() == 7 * 24 * 3600
    finally:
        config_mod._cached = None


def test_config_load_default_missing_returns_empty():
    """When default config file does not exist, empty dict is used (line 59 else)."""
    config_mod._cached = None
    try:
        with patch("os.path.isfile", return_value=False):
            cfg = config_mod.get_config()
        assert cfg == {}
    finally:
        config_mod._cached = None


def test_config_load_default_fails_returns_empty(monkeypatch):
    """When default config load raises, empty dict is used (lines 61-63)."""
    config_mod._cached = None
    try:

        def fake_load(path):
            raise ValueError("broken default")

        with patch("ml.config._load_json", side_effect=fake_load):
            cfg = config_mod.get_config()
        assert cfg == {}
    finally:
        config_mod._cached = None


def test_find_user_config_path_env(tmp_path, monkeypatch):
    """_find_user_config_path returns ML_SERVICE_CONFIG when set and file exists (line 29)."""
    config_path = tmp_path / "custom_config.json"
    config_path.write_text("{}")
    monkeypatch.setenv("ML_SERVICE_CONFIG", str(config_path))
    config_mod._cached = None
    try:
        from ml.config import _find_user_config_path

        assert _find_user_config_path() == str(config_path)
    finally:
        config_mod._cached = None


def test_find_user_config_path_config_json_in_dir(tmp_path, monkeypatch):
    """_find_user_config_path finds config.json in ml-service dir (lines 30-34)."""
    monkeypatch.delenv("ML_SERVICE_CONFIG", raising=False)
    config_json = tmp_path / "config.json"
    config_json.write_text("{}")
    monkeypatch.chdir(tmp_path)
    config_mod._cached = None
    try:
        from ml.config import _find_user_config_path

        # Should find config.json in cwd
        result = _find_user_config_path()
        assert result == str(config_json)
    finally:
        config_mod._cached = None


def test_config_load_user_fails_keeps_default(monkeypatch):
    """When user config load raises, default config is kept (lines 71-72)."""
    config_mod._cached = None
    try:

        def fake_load(path):
            if "config.default" in path or path.endswith("config.default.json"):
                return {"inputs": {}}
            raise ValueError("broken user")

        with patch("ml.config._load_json", side_effect=fake_load):
            with patch("ml.config._find_user_config_path", return_value="/tmp/config.json"):
                cfg = config_mod.get_config()
        assert cfg == {"inputs": {}}
    finally:
        config_mod._cached = None


def test_ratings_max_age_days_default(monkeypatch):
    """H-11's limit, when nothing overrides it."""
    monkeypatch.delenv("XI_RATINGS_MAX_AGE_DAYS", raising=False)
    config_mod._cached = {"ml": {}}
    try:
        assert get_ratings_max_age_days() == DEFAULT_RATINGS_MAX_AGE_DAYS
    finally:
        config_mod._cached = None


def test_ratings_max_age_days_from_config(monkeypatch):
    monkeypatch.delenv("XI_RATINGS_MAX_AGE_DAYS", raising=False)
    config_mod._cached = {"ml": {"ratings_max_age_days": 3}}
    try:
        assert get_ratings_max_age_days() == 3
    finally:
        config_mod._cached = None


def test_ratings_max_age_days_env_wins(monkeypatch):
    """A deployment overrides the limit without editing config.json."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "30")
    config_mod._cached = {"ml": {"ratings_max_age_days": 3}}
    try:
        assert get_ratings_max_age_days() == 30
    finally:
        config_mod._cached = None


def test_ratings_max_age_days_invalid_env_falls_back(monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "soon")
    config_mod._cached = {"ml": {"ratings_max_age_days": 5}}
    try:
        assert get_ratings_max_age_days() == 5
    finally:
        config_mod._cached = None


def test_format_codes_fall_back_to_the_canonical_list(monkeypatch):
    config_mod._cached = {"ml": {}}
    try:
        assert get_format_codes() == ["TEST", "ODI", "T20", "T20I"]
    finally:
        config_mod._cached = None
