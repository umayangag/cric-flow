"""Unit tests for app.settings (get_models_dir, _default_models_dir_from_config)."""

import os
from unittest.mock import MagicMock, patch

from app.settings import _default_models_dir_from_config, get_models_dir


def test_get_models_dir_env_ml_service_output_dir(monkeypatch):
    """ML_SERVICE_OUTPUT_DIR takes precedence."""
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", "/env/ml-output")
    monkeypatch.delenv("MODELS_DIR", raising=False)
    out = get_models_dir(svc_config=None)
    assert out == "/env/ml-output"


def test_get_models_dir_env_models_dir_when_no_ml_service(monkeypatch):
    """MODELS_DIR used when ML_SERVICE_OUTPUT_DIR not set."""
    monkeypatch.delenv("ML_SERVICE_OUTPUT_DIR", raising=False)
    monkeypatch.setenv("MODELS_DIR", "/models")
    out = get_models_dir(svc_config=None)
    assert out == "/models"


def test_get_models_dir_fallback_from_config(monkeypatch):
    """When no env, svc_config.default_artifacts_dir() is used."""
    monkeypatch.delenv("ML_SERVICE_OUTPUT_DIR", raising=False)
    monkeypatch.delenv("MODELS_DIR", raising=False)
    mock_config = MagicMock()
    mock_config.default_artifacts_dir.return_value = "/config/artifacts"
    out = get_models_dir(svc_config=mock_config)
    assert out == "/config/artifacts"
    mock_config.default_artifacts_dir.assert_called_once()


def test_get_models_dir_config_raises_uses_builtin(monkeypatch):
    """When svc_config.default_artifacts_dir() raises, use built-in fallback."""
    monkeypatch.delenv("ML_SERVICE_OUTPUT_DIR", raising=False)
    monkeypatch.delenv("MODELS_DIR", raising=False)
    mock_config = MagicMock()
    mock_config.default_artifacts_dir.side_effect = RuntimeError("no config")
    with patch("logging.getLogger") as mock_get_logger:
        mock_log = MagicMock()
        mock_get_logger.return_value = mock_log
        out = get_models_dir(svc_config=mock_config)
    assert "output" in out and "ml-service" in out
    assert os.path.isabs(out)


def test_default_models_dir_from_config_none_returns_builtin():
    """_default_models_dir_from_config(None) returns built-in path."""
    out = _default_models_dir_from_config(None)
    assert "output" in out
    assert "ml-service" in out


def test_default_models_dir_from_config_success():
    """_default_models_dir_from_config with valid config returns its value."""
    mock_config = MagicMock()
    mock_config.default_artifacts_dir.return_value = "/custom"
    out = _default_models_dir_from_config(mock_config)
    assert out == "/custom"


def test_cors_defaults_when_env_unset(monkeypatch):
    """CORS allow lists use explicit defaults instead of wildcard."""
    from app.settings import DEFAULT_CORS_ALLOW_HEADERS, DEFAULT_CORS_ALLOW_METHODS, load_ml_service_settings

    monkeypatch.delenv("CORS_ALLOW_METHODS", raising=False)
    monkeypatch.delenv("CORS_ALLOW_HEADERS", raising=False)
    settings = load_ml_service_settings()
    assert settings.cors_allow_methods == list(DEFAULT_CORS_ALLOW_METHODS)
    assert settings.cors_allow_headers == list(DEFAULT_CORS_ALLOW_HEADERS)
    assert "*" not in settings.cors_allow_methods
    assert "*" not in settings.cors_allow_headers


def test_cors_env_csv_override(monkeypatch):
    """CORS_ALLOW_METHODS and CORS_ALLOW_HEADERS accept comma-separated overrides."""
    from app.settings import load_ml_service_settings

    monkeypatch.setenv("CORS_ALLOW_METHODS", "GET, POST")
    monkeypatch.setenv("CORS_ALLOW_HEADERS", "Content-Type, X-API-Key")
    settings = load_ml_service_settings()
    assert settings.cors_allow_methods == ["GET", "POST"]
    assert settings.cors_allow_headers == ["Content-Type", "X-API-Key"]


def test_settings_carry_no_training_concurrency_knob(monkeypatch):
    """How many training jobs may run is not a number an operator sets (SERVE-06).

    `MAX_CONCURRENT_TRAINING_JOBS` bounded HTTP requests, not runs: above 1 it let two
    retrains write the same data-quality baseline and publish progress under the same
    step, and at 1 it queued the second request behind a ten-minute run instead of
    answering it. The rule that replaced it -- one run per step, the second refused by
    name -- is not configurable, so the setting is gone rather than left to read as if
    it still decided something.
    """
    from app.settings import load_ml_service_settings

    monkeypatch.setenv("MAX_CONCURRENT_TRAINING_JOBS", "4")
    settings = load_ml_service_settings()
    assert not hasattr(settings, "max_concurrent_training_jobs")
