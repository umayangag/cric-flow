import importlib
import logging


def _reload_logging(monkeypatch, **env):
    for k, v in env.items():
        monkeypatch.setenv(k, str(v))
    mod = importlib.import_module("app.logging")
    importlib.reload(mod)
    return mod


def test_init_logging_console_format(tmp_path, monkeypatch):
    """init_logging with LOG_FORMAT=console uses ConsoleRenderer (lines 66-70)."""
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    monkeypatch.setenv("LOG_FORMAT", "console")
    mod = _reload_logging(monkeypatch)
    mod.init_logging()
    root = logging.getLogger()
    assert root.handlers


def test_init_logging_color_handler(tmp_path, monkeypatch):
    """init_logging with LOG_COLOR=1 and LOG_FORMAT=json uses _ColorStreamHandler (line 92)."""
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    monkeypatch.setenv("LOG_COLOR", "1")
    monkeypatch.setenv("LOG_FORMAT", "json")
    mod = _reload_logging(monkeypatch)
    mod.init_logging()
    root = logging.getLogger()
    assert root.handlers
    from app.logging import _ColorStreamHandler

    assert isinstance(root.handlers[0], _ColorStreamHandler)


def test_color_stream_handler_format_levels(tmp_path, monkeypatch):
    """_ColorStreamHandler.format adds ANSI codes for ERROR/WARNING (lines 32-39)."""
    import io

    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    mod = _reload_logging(monkeypatch)

    fmt = logging.Formatter("%(message)s")
    handler = mod._ColorStreamHandler(io.StringIO(), fmt, use_color=True)

    err_record = logging.LogRecord("x", logging.ERROR, "", 0, "err", (), None)
    warn_record = logging.LogRecord("x", logging.WARNING, "", 0, "warn", (), None)
    info_record = logging.LogRecord("x", logging.INFO, "", 0, "info", (), None)

    err_out = handler.format(err_record)
    warn_out = handler.format(warn_record)
    info_out = handler.format(info_record)

    assert "\033[31m" in err_out
    assert "err" in err_out
    assert "\033[33m" in warn_out
    assert "warn" in warn_out
    assert "\033[" not in info_out
    assert "info" in info_out


def test_color_stream_handler_no_color(tmp_path, monkeypatch):
    """_ColorStreamHandler with use_color=False returns plain msg (lines 27-29)."""
    import io

    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    mod = _reload_logging(monkeypatch)

    fmt = logging.Formatter("%(message)s")
    handler = mod._ColorStreamHandler(io.StringIO(), fmt, use_color=False)

    err_record = logging.LogRecord("x", logging.ERROR, "", 0, "plain", (), None)
    out = handler.format(err_record)
    assert out == "plain"
    assert "\033[" not in out


def test_get_logger_singleton(tmp_path, monkeypatch):
    # Ensure fresh module state
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    logging_mod = importlib.import_module("app.logging")
    importlib.reload(logging_mod)

    logger1 = logging_mod.get_logger()
    logger2 = logging_mod.get_logger()
    assert logger1 is logger2
    # Should have at least one handler
    assert logger1.handlers
