"""Unit tests for ml.config (config load, merge, training params, defaults)."""

import pytest

import ml.config as config_mod
from ml.config import (
    TRAINING_MODELS,
    TRAINING_REQUIRED_KEYS,
    _deep_merge,
    default_artifacts_dir,
    default_go_app_export_dir,
    get_feature_defaults,
    get_prediction_defaults,
    get_training_data_fetch_timeout_sec,
    get_training_params,
    get_training_subprocess_timeout_sec,
    get_tuning_config,
    get_tuning_search_space,
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


def test_get_training_params_batting_from_default_config():
    """get_training_params('batting') with default config returns valid params."""
    config_mod._cached = None
    params = get_training_params("batting")
    assert params["n_estimators"] >= 1
    assert params["max_depth"] >= 1
    assert params["random_state"] is not None
    assert params["joblib_compress"] in range(10)
    assert params["estimator"] in ("rf", "gb", "stacked", "quantile")
    assert "n_jobs" in params


def test_get_training_params_bowling_from_default_config():
    """get_training_params('bowling') with default config."""
    config_mod._cached = None
    params = get_training_params("bowling")
    assert params["n_estimators"] >= 1
    assert params["estimator"] in ("rf", "gb", "stacked", "quantile")


def test_get_training_params_unknown_model_raises():
    """Unknown model raises ValueError."""
    with pytest.raises(ValueError, match="Unknown model"):
        get_training_params("unknown_model")


def test_get_training_params_missing_ml_block_raises(monkeypatch):
    """Config without ml block raises ValueError."""
    config_mod._cached = {"inputs": {}}
    try:
        with pytest.raises(ValueError, match="ml.training"):
            get_training_params("batting")
    finally:
        config_mod._cached = None


def test_get_training_params_missing_training_block_raises(monkeypatch):
    """Config with ml but no training raises ValueError."""
    config_mod._cached = {"ml": {}}
    try:
        with pytest.raises(ValueError, match="ml.training"):
            get_training_params("batting")
    finally:
        config_mod._cached = None


def test_get_training_params_missing_model_block_raises(monkeypatch):
    """Config with ml.training but no batting block raises ValueError."""
    config_mod._cached = {"ml": {"training": {"bowling": {}}}}
    try:
        with pytest.raises(ValueError, match="ml.training.batting"):
            get_training_params("batting")
    finally:
        config_mod._cached = None


def test_get_training_params_missing_keys_raises(monkeypatch):
    """ml.training.batting missing required key raises ValueError."""
    config_mod._cached = {
        "ml": {
            "training": {
                "batting": {
                    "n_estimators": 100,
                    "max_depth": 8,
                    # missing random_state, joblib_compress
                }
            }
        }
    }
    try:
        with pytest.raises(ValueError, match="missing required keys"):
            get_training_params("batting")
    finally:
        config_mod._cached = None


def test_get_training_params_invalid_types_raises(monkeypatch):
    """Invalid integer types in block raise ValueError."""
    config_mod._cached = {
        "ml": {
            "training": {
                "batting": {
                    "n_estimators": "many",
                    "max_depth": 8,
                    "random_state": 42,
                    "joblib_compress": 3,
                }
            }
        }
    }
    try:
        with pytest.raises(ValueError, match="must be integers"):
            get_training_params("batting")
    finally:
        config_mod._cached = None


def test_get_training_params_invalid_joblib_compress_raises(monkeypatch):
    """joblib_compress outside 0-9 raises ValueError."""
    config_mod._cached = {
        "ml": {
            "training": {
                "batting": {
                    "n_estimators": 100,
                    "max_depth": 8,
                    "random_state": 42,
                    "joblib_compress": 10,
                }
            }
        }
    }
    try:
        with pytest.raises(ValueError, match="0 and 9"):
            get_training_params("batting")
    finally:
        config_mod._cached = None


def test_get_training_params_estimator_aliases(monkeypatch):
    """Estimator gb, stacked, quantile and aliases are normalized."""
    base = {
        "n_estimators": 50,
        "max_depth": 5,
        "random_state": 42,
        "joblib_compress": 1,
    }
    for est_val, expected in [
        ("gb", "gb"),
        ("GBM", "gb"),
        ("gradient_boosting", "gb"),
        ("stacked", "stacked"),
        ("stacking", "stacked"),
        ("ensemble", "stacked"),
        ("quantile", "quantile"),
        ("qr", "quantile"),
        ("rf", "rf"),
        ("random_forest", "random_forest"),  # config keeps as-is (no normalize to "rf")
    ]:
        config_mod._cached = {"ml": {"training": {"batting": {**base, "estimator": est_val}}}}
        try:
            params = get_training_params("batting")
            assert params["estimator"] == expected
        finally:
            config_mod._cached = None


def test_get_training_params_learning_rate_float_parsing(monkeypatch):
    """learning_rate and quantile_level parsed as float; invalid -> default."""
    config_mod._cached = {
        "ml": {
            "training": {
                "batting": {
                    "n_estimators": 50,
                    "max_depth": 5,
                    "random_state": 42,
                    "joblib_compress": 1,
                    "estimator": "gb",
                    "learning_rate": "0.05",
                    "quantile_level": "0.9",
                }
            }
        }
    }
    try:
        params = get_training_params("batting")
        assert params["learning_rate"] == 0.05
        assert params["quantile_level"] == 0.9
    finally:
        config_mod._cached = None


def test_default_go_app_export_dir_from_config(monkeypatch):
    """default_go_app_export_dir returns config value when set."""
    config_mod._cached = {"inputs": {"go_app_export_dir": "/custom/export"}}
    try:
        assert default_go_app_export_dir() == "/custom/export"
    finally:
        config_mod._cached = None


def test_default_go_app_export_dir_fallback(monkeypatch):
    """default_go_app_export_dir returns fallback when inputs missing."""
    config_mod._cached = {}
    try:
        out = default_go_app_export_dir()
        assert "output" in out and "go-app" in out
    finally:
        config_mod._cached = None


def test_default_artifacts_dir_from_config(monkeypatch):
    """default_artifacts_dir returns config value when set."""
    config_mod._cached = {"outputs": {"artifacts_dir": "/custom/artifacts"}}
    try:
        assert default_artifacts_dir() == "/custom/artifacts"
    finally:
        config_mod._cached = None


def test_get_training_data_fetch_timeout_sec_default(monkeypatch):
    """Timeout from config or 3600 default; invalid int -> 600."""
    config_mod._cached = {"inputs": {}}
    try:
        # default 3600 when key missing
        assert get_training_data_fetch_timeout_sec() == 3600
    finally:
        config_mod._cached = None


def test_get_training_data_fetch_timeout_sec_invalid_returns_600(monkeypatch):
    """Invalid timeout value returns 600."""
    config_mod._cached = {"inputs": {"training_data_fetch_timeout_sec": "x"}}
    try:
        assert get_training_data_fetch_timeout_sec() == 600
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


def test_get_tuning_config(monkeypatch):
    """get_tuning_config returns cv_splits, n_iter, n_jobs, etc."""
    config_mod._cached = None
    cfg = get_tuning_config()
    assert "cv_splits" in cfg
    assert "n_iter" in cfg
    assert "n_jobs" in cfg
    assert "random_state" in cfg
    assert "scoring" in cfg
    assert "search_space" in cfg


def test_get_tuning_search_space_rf(monkeypatch):
    """get_tuning_search_space('rf') returns dict when configured."""
    config_mod._cached = None
    space = get_tuning_search_space("rf")
    # default config has search_space.rf
    assert space is None or isinstance(space, dict)


def test_get_tuning_search_space_missing_returns_none(monkeypatch):
    """get_tuning_search_space with no search_space returns None."""
    config_mod._cached = {"ml": {"tuning": {}}}
    try:
        assert get_tuning_search_space("rf") is None
        assert get_tuning_search_space("gb") is None
    finally:
        config_mod._cached = None


def test_get_feature_defaults(monkeypatch):
    """get_feature_defaults returns common and fielding defaults."""
    config_mod._cached = None
    fd = get_feature_defaults()
    assert "common" in fd
    assert "fielding" in fd
    assert "temp" in fd["common"]
    assert "consistency" in fd["fielding"] or "venue" in fd["fielding"]


def test_get_prediction_defaults(monkeypatch):
    """get_prediction_defaults returns economy default."""
    config_mod._cached = None
    pd_def = get_prediction_defaults()
    assert "economy" in pd_def
    assert isinstance(pd_def["economy"], float)


def test_training_required_keys_constant():
    """TRAINING_REQUIRED_KEYS and TRAINING_MODELS are defined."""
    assert "n_estimators" in TRAINING_REQUIRED_KEYS
    assert "batting" in TRAINING_MODELS
    assert "bowling" in TRAINING_MODELS
