"""Tests for training_orchestrator helpers and subprocess wrapper."""

import os
from types import SimpleNamespace
from typing import Any, Dict, List, Optional

import pytest

from app import training_orchestrator


class DummyLogger:
    def __init__(self) -> None:
        self.infos: List[Dict[str, Any]] = []
        self.warnings: List[Dict[str, Any]] = []
        self.errors: List[Dict[str, Any]] = []

    def info(self, *args: Any, **kwargs: Any) -> None:  # pragma: no cover - trivial logger plumbing
        self.infos.append({"args": args, "kwargs": kwargs})

    def warning(self, *args: Any, **kwargs: Any) -> None:  # pragma: no cover - trivial logger plumbing
        self.warnings.append({"args": args, "kwargs": kwargs})

    def error(self, *args: Any, **kwargs: Any) -> None:
        self.errors.append({"args": args, "kwargs": kwargs})


def test_ml_service_root_points_to_ml_package_root() -> None:
    root = training_orchestrator.ml_service_root()
    assert os.path.isdir(root)
    # ml package should live directly under the returned root
    assert os.path.isfile(os.path.join(root, "ml", "__init__.py"))


def test_export_csvs_available_true_when_matching_csv(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("GO_APP_OUTPUT_DIR", str(tmp_path))
    (tmp_path / "batting_encoded_all.csv").write_text("x,y\n", encoding="utf-8")

    assert training_orchestrator.export_csvs_available("batting_encoded_") is True
    # Non-matching prefix should be false
    assert training_orchestrator.export_csvs_available("bowling_encoded_") is False


def test_export_csvs_available_listdir_oserror_returns_false(tmp_path, monkeypatch) -> None:
    """When listdir raises OSError, export_csvs_available returns False."""
    from unittest.mock import patch

    monkeypatch.setenv("GO_APP_OUTPUT_DIR", str(tmp_path))
    with patch("os.listdir", side_effect=OSError(2, "No such file")):
        assert training_orchestrator.export_csvs_available("batting_encoded_") is False


def test_export_csvs_available_empty_dir_returns_false(tmp_path, monkeypatch) -> None:
    """When dir exists but has no matching CSV, returns False."""
    monkeypatch.setenv("GO_APP_OUTPUT_DIR", str(tmp_path))
    assert training_orchestrator.export_csvs_available("batting_encoded_") is False


def test_run_training_subprocess_success_uses_env_and_timeout(monkeypatch) -> None:
    calls: Dict[str, Any] = {}

    def fake_get_timeout() -> int:
        return 42

    def fake_run(
        cmd: List[str],
        cwd: str,
        env: Dict[str, str],
        capture_output: bool,
        text: bool,
        timeout: int,
    ) -> Any:
        calls["cmd"] = cmd
        calls["cwd"] = cwd
        calls["env"] = env
        calls["capture_output"] = capture_output
        calls["text"] = text
        calls["timeout"] = timeout
        return SimpleNamespace(returncode=0, stdout="ok", stderr="")

    # Patch lower-level dependencies
    import ml.config as ml_config

    monkeypatch.setattr(ml_config, "get_training_subprocess_timeout_sec", fake_get_timeout)
    monkeypatch.setattr(training_orchestrator.subprocess, "run", fake_run)

    logger = DummyLogger()

    training_orchestrator.run_training_subprocess(
        "ml.train_batting",
        extra_args=["--from-api", "--cutoff", "2024-01-01"],
        extra_env={"EXTRA_FLAG": "1"},
        logger=logger,
    )

    assert calls["cmd"][:3] == [training_orchestrator.sys.executable, "-m", "ml.train_batting"]
    assert calls["cwd"] == training_orchestrator.ml_service_root()
    assert calls["env"]["SKIP_PIPELINE_TRACKING"] == "1"
    assert calls["env"]["EXTRA_FLAG"] == "1"
    assert calls["timeout"] == 42
    # Logger should have been used at least once
    assert any(
        "pipeline: starting training subprocess" in (rec["args"][0] if rec["args"] else "") for rec in logger.infos
    )


def test_run_training_subprocess_timeout_raises(monkeypatch) -> None:
    def fake_get_timeout() -> int:
        return 1

    def fake_run(*_args: Any, **_kwargs: Any) -> Any:
        raise training_orchestrator.subprocess.TimeoutExpired(cmd="ml.train_win", timeout=1)

    import ml.config as ml_config

    monkeypatch.setattr(ml_config, "get_training_subprocess_timeout_sec", fake_get_timeout)
    monkeypatch.setattr(training_orchestrator.subprocess, "run", fake_run)

    logger = DummyLogger()
    with pytest.raises(ValueError) as exc:
        training_orchestrator.run_training_subprocess("ml.train_win", logger=logger)
    assert "timed out" in str(exc.value)
    assert logger.errors, "expected timeout to log an error"


def test_run_training_subprocess_failure_raises(monkeypatch) -> None:
    def fake_get_timeout() -> int:
        return 10

    def fake_run(*_args: Any, **_kwargs: Any) -> Any:
        return SimpleNamespace(returncode=2, stdout="some\nstdout", stderr="some\nstderr")

    import ml.config as ml_config

    monkeypatch.setattr(ml_config, "get_training_subprocess_timeout_sec", fake_get_timeout)
    monkeypatch.setattr(training_orchestrator.subprocess, "run", fake_run)

    logger = DummyLogger()
    with pytest.raises(ValueError) as exc:
        training_orchestrator.run_training_subprocess("ml.train_win", logger=logger)
    assert "Training failed" in str(exc.value)
    assert logger.errors, "expected failure to log an error"


def test_run_win_training_forwards_the_cutoff_and_the_go_app_url(monkeypatch) -> None:
    """The win trainer is temporal: it is handed the cutoff it was given, never none."""
    calls: Dict[str, Any] = {}

    def fake_run_training_subprocess(
        module: str,
        extra_args: Optional[List[str]] = None,
        extra_env: Optional[Dict[str, str]] = None,
        logger: Optional[Any] = None,
    ) -> None:
        calls.update(module=module, extra_args=extra_args or [], extra_env=extra_env or {}, logger=logger)

    monkeypatch.setattr(training_orchestrator, "run_training_subprocess", fake_run_training_subprocess)

    logger = DummyLogger()
    training_orchestrator.run_win_training("2024-01-01T00:00:00Z", "http://localhost:8080", logger=logger)

    assert calls["module"] == "ml.train_win"
    assert calls["extra_args"] == [
        "--cutoff",
        "2024-01-01T00:00:00Z",
        "--go-app-url",
        "http://localhost:8080",
    ]
    assert calls["extra_env"].get("ML_N_JOBS") == "-1"
    assert isinstance(calls["logger"], DummyLogger)


def test_run_auto_tune_invokes_training_subprocess_with_expected_args(monkeypatch) -> None:
    captured: Dict[str, Any] = {}

    def fake_run_training_subprocess(
        module: str,
        extra_args: Optional[List[str]] = None,
        extra_env: Optional[Dict[str, str]] = None,
        logger: Optional[Any] = None,
    ) -> None:
        captured["module"] = module
        captured["extra_args"] = extra_args or []
        captured["extra_env"] = extra_env
        captured["logger"] = logger

    monkeypatch.setattr(training_orchestrator, "run_training_subprocess", fake_run_training_subprocess)

    logger = DummyLogger()
    training_orchestrator.run_auto_tune(
        cutoff="2025-01-01T00:00:00Z",
        go_app_url="http://localhost:8080",
        model="win",
        use_all_formats=False,
        fmt="T20",
        rescreen=True,
        algorithms="xgboost,lightgbm",
        logger=logger,
    )

    assert captured["module"] == "ml.auto_tune"
    args = captured["extra_args"]
    # Core flags should be threaded through
    assert "--model" in args and "win" in args
    assert "--from-api" in args and "--cutoff" in args and "--go-app-url" in args
    assert "--format" in args and "T20" in args
    assert "--rescreen" in args
    assert "--algorithms" in args
    # When single_task is true we set AUTO_TUNE_N_JOBS
    assert captured["extra_env"] == {"AUTO_TUNE_N_JOBS": "-1"}
    assert isinstance(captured["logger"], DummyLogger)
