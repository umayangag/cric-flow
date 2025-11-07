import importlib


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
