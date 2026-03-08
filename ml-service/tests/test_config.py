"""Unit tests for ml.config (config load, merge, training params, defaults)."""

from unittest.mock import MagicMock, patch

import pytest

import ml.config as config_mod
from ml.config import (
    TRAINING_MODELS,
    TRAINING_REQUIRED_KEYS,
    _deep_merge,
    default_artifacts_dir,
    default_go_app_export_dir,
    get_feature_defaults,
    get_pipeline_common_config,
    get_prediction_defaults,
    get_training_data_fetch_timeout_sec,
    get_training_params,
    get_training_subprocess_timeout_sec,
    get_tuned_params_from_go_app,
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


def test_get_training_params_invalid_learning_rate_quantile_defaults(monkeypatch):
    """Invalid learning_rate/quantile_level (non-float) fall back to 0.1 and 0.5."""
    config_mod._cached = {
        "ml": {
            "training": {
                "batting": {
                    "n_estimators": 50,
                    "max_depth": 5,
                    "random_state": 42,
                    "joblib_compress": 1,
                    "estimator": "gb",
                    "learning_rate": "bad",
                    "quantile_level": "nope",
                }
            }
        }
    }
    try:
        params = get_training_params("batting")
        assert params["learning_rate"] == 0.1
        assert params["quantile_level"] == 0.5
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


def test_get_training_data_fetch_timeout_sec_invalid_and_invalid_fallback_non_numeric(monkeypatch):
    """When both val and invalid_fallback are non-numeric, returns DEFAULT fallback 600."""
    config_mod._cached = {
        "inputs": {
            "training_data_fetch_timeout_sec": "bad",
            "training_data_fetch_timeout_invalid_fallback_sec": "also_bad",
        }
    }
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


def test_get_tuned_params_from_go_app_invalid_timeout_config(monkeypatch):
    """Invalid go_app_request_timeout_sec in config falls back to default (exercises _go_app_request_timeout_sec)."""
    config_mod._cached = {"inputs": {"go_app_request_timeout_sec": "not_an_int"}}
    try:
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_resp = MagicMock()
            mock_resp.read.return_value = b'{"params": {}}'
            mock_resp.__enter__ = MagicMock(return_value=mock_resp)
            mock_resp.__exit__ = MagicMock(return_value=False)
            mock_urlopen.return_value = mock_resp
            result = get_tuned_params_from_go_app("http://localhost:8080", "batting", "ODI")
            assert result == {}
            mock_urlopen.assert_called_once()
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


def test_get_pipeline_common_config():
    """get_pipeline_common_config returns generalized_pipeline defaults."""
    config_mod._cached = None
    cfg = get_pipeline_common_config()
    assert "use_robust_scaler" in cfg
    assert "time_decay_halflife_years" in cfg
    assert "delta_threshold" in cfg
    assert "min_rows_for_training" in cfg
    assert isinstance(cfg["use_robust_scaler"], bool)
    assert cfg["time_decay_halflife_years"] == 2.0
    assert cfg["delta_threshold"] == 0.08
    assert cfg["min_rows_for_training"] == 10


def test_training_required_keys_constant():
    """TRAINING_REQUIRED_KEYS and TRAINING_MODELS are defined."""
    assert "n_estimators" in TRAINING_REQUIRED_KEYS
    assert "batting" in TRAINING_MODELS
    assert "bowling" in TRAINING_MODELS


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


def test_get_tuned_params_from_go_app_404_returns_none():
    """get_tuned_params_from_go_app returns None on 404."""
    import urllib.error

    err = urllib.error.HTTPError("http://x", 404, "Not Found", None, None)
    with patch("urllib.request.urlopen", side_effect=err):
        result = get_tuned_params_from_go_app("http://localhost:8080", "batting", "ODI")
    assert result is None


def test_get_tuned_params_from_go_app_http_error_non_404_returns_none():
    """get_tuned_params_from_go_app returns None on non-404 HTTPError."""
    import urllib.error

    err = urllib.error.HTTPError("http://x", 500, "Internal Error", None, None)
    with patch("urllib.request.urlopen", side_effect=err):
        result = get_tuned_params_from_go_app("http://localhost:8080", "batting", "ODI")
    assert result is None


def test_get_tuned_params_from_go_app_os_error_returns_none():
    """get_tuned_params_from_go_app returns None on OSError (connection refused)."""
    with patch("urllib.request.urlopen", side_effect=OSError("Connection refused")):
        result = get_tuned_params_from_go_app("http://localhost:8080", "batting", "ODI")
    assert result is None


def test_get_tuned_params_from_go_app_params_string_json_decode_fails():
    """get_tuned_params_from_go_app returns None when params is invalid JSON string."""
    mock_resp = MagicMock()
    mock_resp.read.return_value = b'{"params": "not valid json {"}'
    mock_resp.__enter__ = MagicMock(return_value=mock_resp)
    mock_resp.__exit__ = MagicMock(return_value=False)
    with patch("urllib.request.urlopen", return_value=mock_resp):
        result = get_tuned_params_from_go_app("http://localhost:8080", "batting", "ODI")
    assert result is None


def test_save_tuned_params_to_go_app_success(monkeypatch):
    """save_tuned_params_to_go_app returns without raising on 2xx response."""
    mock_resp = MagicMock()
    mock_resp.status = 200
    mock_resp.__enter__ = MagicMock(return_value=mock_resp)
    mock_resp.__exit__ = MagicMock(return_value=False)
    with patch("urllib.request.urlopen", return_value=mock_resp):
        config_mod.save_tuned_params_to_go_app("http://localhost:8080", "batting", "ODI", {"n_estimators": 100})


def test_save_tuned_params_to_go_app_http_error_raises():
    """save_tuned_params_to_go_app raises ValueError on HTTPError."""
    import urllib.error

    err = urllib.error.HTTPError("http://x", 500, "Error", None, None)
    err.read = lambda: b"error body"
    with patch("urllib.request.urlopen", side_effect=err):
        with pytest.raises(ValueError, match="HTTP 500"):
            config_mod.save_tuned_params_to_go_app("http://localhost:8080", "batting", "ODI", {"n_estimators": 100})


def test_save_tuned_params_to_go_app_os_error_raises():
    """save_tuned_params_to_go_app raises ValueError on OSError."""
    with patch("urllib.request.urlopen", side_effect=OSError("Connection refused")):
        with pytest.raises(ValueError, match="request failed"):
            config_mod.save_tuned_params_to_go_app("http://localhost:8080", "batting", "ODI", {"n_estimators": 100})


def test_get_tuning_config_algorithms_all_expands_to_list(monkeypatch):
    """get_tuning_config with algorithms='all' or None uses default list."""
    config_mod._cached = None
    with patch("ml.config._load", return_value={"ml": {"tuning": {"algorithms": "all", "cv_splits": 5, "n_iter": 25}}}):
        cfg = get_tuning_config()
    assert cfg["algorithms"] == ["rf", "gb", "quantile"]


def test_get_tuning_config_validation_method_invalid_falls_back_to_walk_forward(monkeypatch):
    """get_tuning_config with invalid validation_method uses walk_forward."""
    config_mod._cached = None
    with patch(
        "ml.config._load",
        return_value={"ml": {"tuning": {"validation_method": "other", "cv_splits": 5, "n_iter": 25}}},
    ):
        cfg = get_tuning_config()
    assert cfg["validation_method"] == "walk_forward"
