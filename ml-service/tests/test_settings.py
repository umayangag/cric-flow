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
