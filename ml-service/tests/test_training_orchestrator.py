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
        training_orchestrator.run_training_subprocess("ml.xi.retrain", logger=logger)
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
        training_orchestrator.run_training_subprocess("ml.xi.retrain", logger=logger)
    assert "Training failed" in str(exc.value)
    assert logger.errors, "expected failure to log an error"


def test_run_retrain_forwards_the_cutoff_and_the_artifacts_root(monkeypatch) -> None:
    """Retrain is temporal and run-scoped: it is handed the cutoff it was given and the
    root the runs live under, never neither."""
    calls: Dict[str, Any] = {}

    def fake_run_training_subprocess(
        module: str,
        extra_args: Optional[List[str]] = None,
        extra_env: Optional[Dict[str, str]] = None,
        logger: Optional[Any] = None,
    ) -> None:
        calls.update(module=module, extra_args=extra_args or [], logger=logger)

    monkeypatch.setattr(training_orchestrator, "run_training_subprocess", fake_run_training_subprocess)

    logger = DummyLogger()
    training_orchestrator.run_retrain("2025-09-01", "/models", logger=logger)

    assert calls["module"] == "ml.xi.retrain"
    assert calls["extra_args"] == ["--postgres", "--cutoff", "2025-09-01", "--out", "/models"]
    assert isinstance(calls["logger"], DummyLogger)


def test_run_evaluate_does_not_pass_the_cutoff_to_the_harness(monkeypatch) -> None:
    """L4's rolling origins and its locked window are its definition (H-19). Letting a
    caller move them would make two evaluate runs incomparable, which is the failure H-23
    is about -- so the step's cutoff is logged and not forwarded."""
    calls: Dict[str, Any] = {}

    def fake_run_training_subprocess(
        module: str,
        extra_args: Optional[List[str]] = None,
        extra_env: Optional[Dict[str, str]] = None,
        logger: Optional[Any] = None,
    ) -> None:
        calls.update(module=module, extra_args=extra_args or [])

    monkeypatch.setattr(training_orchestrator, "run_training_subprocess", fake_run_training_subprocess)

    training_orchestrator.run_evaluate("2025-09-01", "/models", logger=DummyLogger())

    assert calls["module"] == "ml.xi.evaluate"
    assert calls["extra_args"] == ["--postgres", "--out", "/models"]
