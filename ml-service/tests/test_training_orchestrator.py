"""Tests for training_orchestrator helpers and subprocess wrapper."""

import os
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


class FakePopen:
    """A training subprocess that never was.

    The wrapper holds a Popen rather than calling subprocess.run, because a run nobody
    holds a handle to is a run nobody can stop (D-11) -- so these tests stand in for the
    handle, not for the call.
    """

    def __init__(self, returncode: int = 0, stdout: str = "ok", stderr: str = "", raise_timeout: bool = False) -> None:
        self.returncode = returncode
        self.pid = -1  # no such process, so a signal falls through harmlessly
        self._stdout, self._stderr = stdout, stderr
        self._raise_timeout = raise_timeout
        self._running = True
        self.signals: List[int] = []
        self.calls: Dict[str, Any] = {}

    def communicate(self, timeout: Optional[float] = None) -> Any:
        self.calls["timeout"] = timeout
        if self._raise_timeout:
            raise training_orchestrator.subprocess.TimeoutExpired(cmd="ml.xi.retrain", timeout=timeout or 0)
        self._running = False
        return self._stdout, self._stderr

    def send_signal(self, sig: int) -> None:
        self.signals.append(sig)

    def poll(self) -> Optional[int]:
        """None until the process has been waited on, as `Popen.poll` does.

        A stop asks this before it signals anything: a run that has already exited is
        nothing to stop, and saying otherwise reports a success as a stop (SERVE-06).
        """
        return None if self._running else self.returncode

    def wait(self, timeout: Optional[float] = None) -> int:
        self._running = False
        return self.returncode


def patch_popen(monkeypatch, process: FakePopen, timeout_sec: int) -> Dict[str, Any]:
    """Route subprocess creation to `process` and record how it was asked for."""
    calls: Dict[str, Any] = {}

    def fake_popen(cmd: List[str], **kwargs: Any) -> FakePopen:
        calls["cmd"] = cmd
        calls.update(kwargs)
        return process

    import ml.config as ml_config

    monkeypatch.setattr(ml_config, "get_training_subprocess_timeout_sec", lambda: timeout_sec)
    monkeypatch.setattr(training_orchestrator.subprocess, "Popen", fake_popen)
    return calls


def test_run_training_subprocess_success_uses_env_and_timeout(monkeypatch) -> None:
    process = FakePopen()
    calls = patch_popen(monkeypatch, process, timeout_sec=42)
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
    assert process.calls["timeout"] == 42
    # The child leads its own process group, so stopping it reaches the workers it spawns.
    assert calls["start_new_session"] is True
    # Logger should have been used at least once
    assert any(
        "pipeline: starting training subprocess" in (rec["args"][0] if rec["args"] else "") for rec in logger.infos
    )


def test_run_training_subprocess_deregisters_the_process_when_it_finishes(monkeypatch) -> None:
    """A handle left behind would make a later stop address a run that is already over."""
    patch_popen(monkeypatch, FakePopen(), timeout_sec=10)

    training_orchestrator.run_training_subprocess("ml.xi.retrain")

    assert training_orchestrator._processes.running_steps() == []


def test_run_training_subprocess_timeout_raises(monkeypatch) -> None:
    process = FakePopen(raise_timeout=True)
    patch_popen(monkeypatch, process, timeout_sec=1)

    logger = DummyLogger()
    with pytest.raises(ValueError) as exc:
        training_orchestrator.run_training_subprocess("ml.xi.retrain", logger=logger)
    assert "timed out" in str(exc.value)
    assert logger.errors, "expected timeout to log an error"
    # A run abandoned on timeout is killed rather than left behind, which is the same
    # orphan D-11 was about arriving by a different route.
    assert process.signals, "a timed-out subprocess must still be terminated"
    assert training_orchestrator._processes.running_steps() == []


def test_run_training_subprocess_failure_raises(monkeypatch) -> None:
    patch_popen(monkeypatch, FakePopen(returncode=2, stdout="some\nstdout", stderr="some\nstderr"), timeout_sec=10)

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


def test_run_retrain_never_accepts_a_data_quality_regression_on_its_own(monkeypatch) -> None:
    """The one step of the chain that assumes a human is the data-quality gate, and the
    unattended path must fail closed rather than wave it through.

    ``--accept-data-quality`` says "I have looked at the new counts and they are right",
    which is a judgement nobody is present to make when a scheduler runs the cadence
    (A-5) at 06:00 on a Monday. Without it the retrain exits non-zero, the run plan stops
    there, and reload -- the step after it -- never runs, so `current` goes on pointing at
    the run it already pointed at. An orchestrator that passed the flag to keep its own
    chain green would publish exactly the run the gate exists to hold back."""
    calls: Dict[str, Any] = {}

    def fake_run_training_subprocess(
        module: str,
        extra_args: Optional[List[str]] = None,
        extra_env: Optional[Dict[str, str]] = None,
        logger: Optional[Any] = None,
    ) -> None:
        calls.update(extra_args=extra_args or [])

    monkeypatch.setattr(training_orchestrator, "run_training_subprocess", fake_run_training_subprocess)

    training_orchestrator.run_retrain("2025-09-01", "/models", logger=DummyLogger())

    assert "--accept-data-quality" not in calls["extra_args"]


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
