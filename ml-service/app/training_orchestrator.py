"""Training orchestration: subprocess invocation and job helpers for /admin/train/*.

Two steps run through here: ``retrain`` (``ml.xi.retrain`` -- the rating pass, the win
models, the performance models, the report and the run manifest) and ``evaluate``
(``ml.xi.evaluate`` -- L4 at a cutoff, touching no artifact ``current`` points at).

Handlers in main.py validate HTTP input and call these functions; they do not embed
subprocess or env logic. Concurrency (semaphore) remains in main so async boundaries
stay clear.
"""

from __future__ import annotations

import os
import subprocess
import sys
from typing import Any, Dict, List, Optional

# Logger type: any object with info, warning, error, debug
Logger = Any


def ml_service_root() -> str:
    """Return the ml-service project root (directory containing the 'ml' package)."""
    import ml as _ml  # noqa: PLC0415

    return os.path.dirname(os.path.dirname(os.path.abspath(_ml.__file__)))


def run_training_subprocess(
    module: str,
    extra_args: Optional[List[str]] = None,
    extra_env: Optional[Dict[str, str]] = None,
    logger: Optional[Logger] = None,
) -> None:
    """Run a training module as subprocess; raises ValueError on non-zero exit or timeout.

    Timeout from ml.config.get_training_subprocess_timeout_sec (env TRAINING_SUBPROCESS_TIMEOUT_SEC).
    Sets SKIP_PIPELINE_TRACKING=1. extra_env is merged into subprocess env.
    """
    from ml.config import get_training_subprocess_timeout_sec

    root = ml_service_root()
    cmd = [sys.executable, "-m", module]
    if extra_args:
        cmd.extend(extra_args)
    if logger:
        logger.info(
            "pipeline: starting training subprocess",
            module=module,
            extra_args=extra_args or [],
            cwd=root,
        )
    env = {**os.environ, "SKIP_PIPELINE_TRACKING": "1"}
    # Force subprocess to load config from ml-service root so MLQA/tuning use the same config as the server.
    config_path = os.path.join(root, "config.json")
    if os.path.isfile(config_path):
        env["ML_SERVICE_CONFIG"] = config_path
    if extra_env:
        env.update(extra_env)
    timeout_sec = get_training_subprocess_timeout_sec()
    try:
        proc = subprocess.run(
            cmd,
            cwd=root,
            env=env,
            capture_output=True,
            text=True,
            timeout=timeout_sec,
        )
    except subprocess.TimeoutExpired as e:
        if logger:
            logger.error(
                "pipeline: training subprocess timed out",
                module=module,
                timeout_sec=timeout_sec,
            )
        raise ValueError(f"Training timed out after {timeout_sec}s") from e
    if proc.returncode != 0:
        stdout_lines = (proc.stdout or "").strip().splitlines() if proc.stdout else []
        stderr_lines = (proc.stderr or "").strip().splitlines() if proc.stderr else []
        max_lines = 100
        stdout_tail = "\n".join(stdout_lines[-max_lines:]) if stdout_lines else "(empty)"
        stderr_tail = "\n".join(stderr_lines[-max_lines:]) if stderr_lines else "(empty)"
        if logger:
            logger.error(
                "pipeline: training subprocess failed",
                module=module,
                returncode=proc.returncode,
                subprocess_stdout=stdout_tail,
                subprocess_stderr=stderr_tail,
            )
        raise ValueError(f"Training failed (exit {proc.returncode})")


def run_retrain(cutoff: str, artifacts_dir: str, logger: Optional[Logger] = None) -> None:
    """Run the retrain step. Raises ValueError on failure.

    Reads the database rather than any exported CSV: the rating pass is one ordered scan
    of ``ball_event``, which is why the precompute and export steps could go at all.
    """
    if logger:
        logger.info("admin.train.start", step="retrain", cutoff=cutoff, artifacts_dir=artifacts_dir)
    run_training_subprocess(
        "ml.xi.retrain",
        ["--postgres", "--cutoff", cutoff, "--out", artifacts_dir],
        logger=logger,
    )
    if logger:
        logger.info("admin.train.success", step="retrain")


def run_evaluate(cutoff: str, artifacts_dir: str, logger: Optional[Logger] = None) -> None:
    """Run L4 at a cutoff, writing its report beside the runs and publishing nothing.

    ``cutoff`` is accepted and logged but not passed on: the harness's rolling origins
    and its locked window are the definition of L4 (H-19), and letting a caller move
    them would make two runs of "evaluate" incomparable -- which is the failure H-23
    is about. It is here because the pipeline step carries a cutoff for every step.
    """
    if logger:
        logger.info("admin.train.start", step="evaluate", cutoff=cutoff, artifacts_dir=artifacts_dir)
    run_training_subprocess("ml.xi.evaluate", ["--postgres", "--out", artifacts_dir], logger=logger)
    if logger:
        logger.info("admin.train.success", step="evaluate")


def get_step_progress(step: str, run_id: str = "") -> Dict[str, Any]:
    """Return live progress for a pipeline step, or {} when nothing is running.

    With one progress file per run (ops plan O-1) there is no single path to read, so
    this asks for *the live run*: the newest non-stale file for the step. A crashed run
    leaves its file behind, and reporting that as current would show a run that is not
    happening -- `run_progress.latest_for_step` is where that rule lives, so every step
    applies it identically.

    Naming a `run_id` reads exactly that run, including a finished or stale one. That
    is what a caller wants when it is asking about a specific run rather than "what is
    happening now".
    """
    from ml import run_progress

    step = (step or "").strip()
    if not step:
        return {}

    if run_id.strip():
        return run_progress.read(run_progress.progress_path(step, run_id.strip()))

    return run_progress.latest_for_step(step)


def get_run_summary(step: str, run_id: str = "") -> Dict[str, Any]:
    """Return the terminal summary of a training run, or {} when there is none.

    Written by `training_progress.finish` as the subprocess exits, and deliberately
    outliving the progress file: the progress file is removed when a run ends -- a
    file left behind reads as a run still going -- but the outcome has to survive
    that, because whoever wants it asks only after the run is over.

    A step that is not instrumented (or a run that died before finishing) has no
    summary, and {} says so rather than inventing one.
    """
    from ml import run_progress

    step = (step or "").strip()
    if not step:
        return {}
    if run_id.strip():
        return run_progress.read_result(step, run_id.strip())
    return run_progress.latest_result_for_step(step)


def train_response(step: str, run_id: str = "") -> Dict[str, Any]:
    """Build a training endpoint's success body, carrying the run summary when there is one.

    go-app persists this into `data_migrations.metadata` (ops plan O-4), which is what
    finally makes a finished run answerable: how many rows, which formats, what was
    dropped, what was written. Returning it on the response rather than having go-app
    poll for it is the only reliable moment -- by the time go-app could ask, the
    progress file is gone.
    """
    from ml.training_progress import step_name

    body: Dict[str, Any] = {"status": "ok", "step": step}
    summary = get_run_summary(step_name(step), run_id)
    if summary:
        body["summary"] = summary
    return body
