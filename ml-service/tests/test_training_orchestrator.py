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
        raise training_orchestrator.subprocess.TimeoutExpired(cmd="ml.train_batting", timeout=1)

    import ml.config as ml_config

    monkeypatch.setattr(ml_config, "get_training_subprocess_timeout_sec", fake_get_timeout)
    monkeypatch.setattr(training_orchestrator.subprocess, "run", fake_run)

    logger = DummyLogger()
    with pytest.raises(ValueError) as exc:
        training_orchestrator.run_training_subprocess("ml.train_batting", logger=logger)
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
        training_orchestrator.run_training_subprocess("ml.train_bowling", logger=logger)
    assert "Training failed" in str(exc.value)
    assert logger.errors, "expected failure to log an error"


def _capture_run_training(monkeypatch, fn_name: str, cutoff: str, csv_available: bool) -> Dict[str, Any]:
    """Helper to capture calls to run_training_subprocess from *_training helpers."""
    calls: Dict[str, Any] = {}

    def fake_export(prefix: str) -> bool:
        assert prefix in ("batting_encoded_", "bowling_encoded_")
        return csv_available

    def fake_run_training_subprocess(
        module: str,
        extra_args: Optional[List[str]] = None,
        extra_env: Optional[Dict[str, str]] = None,
        logger: Optional[Any] = None,
    ) -> None:
        calls["module"] = module
        calls["extra_args"] = extra_args or []
        calls["extra_env"] = extra_env or {}
        calls["logger"] = logger

    monkeypatch.setattr(training_orchestrator, "export_csvs_available", fake_export)

    monkeypatch.setattr(training_orchestrator, "run_training_subprocess", fake_run_training_subprocess)

    logger = DummyLogger()
    go_app_url = "http://localhost:8080"
    if fn_name == "run_batting_training":
        training_orchestrator.run_batting_training(cutoff, go_app_url, logger=logger)
    else:
        training_orchestrator.run_bowling_training(cutoff, go_app_url, logger=logger)
    calls["logger"] = logger
    return calls


@pytest.mark.parametrize(
    "fn_name,cutoff,csv_available,expected_from_api",
    [
        ("run_batting_training", "2024-01-01T00:00:00Z", False, True),
        ("run_batting_training", "", False, False),
        ("run_bowling_training", "2024-01-01T00:00:00Z", False, True),
    ],
)
def test_run_batting_and_bowling_training_builds_expected_args(
    monkeypatch, fn_name: str, cutoff: str, csv_available: bool, expected_from_api: bool
) -> None:
    calls = _capture_run_training(monkeypatch, fn_name, cutoff, csv_available)
    assert calls["module"] in ("ml.train_batting", "ml.train_bowling")
    args = calls["extra_args"]
    if expected_from_api:
        assert "--from-api" in args and "--cutoff" in args and "--go-app-url" in args
    else:
        # CSV available or no cutoff -> local all-formats training
        assert args == ["--all-formats"]
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
        model="batting",
        use_all_formats=False,
        fmt="T20",
        rescreen=True,
        algorithms="xgboost,lightgbm",
        logger=logger,
    )

    assert captured["module"] == "ml.auto_tune"
    args = captured["extra_args"]
    # Core flags should be threaded through
    assert "--model" in args and "batting" in args
    assert "--from-api" in args and "--cutoff" in args and "--go-app-url" in args
    assert "--format" in args and "T20" in args
    assert "--rescreen" in args
    assert "--algorithms" in args
    # When single_task is true we set AUTO_TUNE_N_JOBS
    assert captured["extra_env"] == {"AUTO_TUNE_N_JOBS": "-1"}
    assert isinstance(captured["logger"], DummyLogger)
