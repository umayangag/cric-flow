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
import signal
import subprocess
import sys
import threading
from typing import Any, Dict, List, Optional

# Logger type: any object with info, warning, error, debug
Logger = Any

# The module each pipeline step runs. Declared once because two things need the mapping:
# starting a step, and stopping the one that is running.
TRAINING_MODULES: Dict[str, str] = {
    "retrain": "ml.xi.retrain",
    "evaluate": "ml.xi.evaluate",
}


class TrainingStopped(ValueError):
    """A training run ended because someone asked it to.

    It subclasses ValueError so every existing caller still handles it, and exists so the
    ones that care can tell an operator's Stop from a run that broke. Reporting a stop as
    `admin.train.failed` with a stack trace sends whoever reads the log looking for a bug
    that is not there -- which is the same species of untruth as D-11 itself.
    """


# How long a stopped process is given to exit on SIGTERM before SIGKILL. A retrain's
# work is one ordered scan and some model fits, none of which need unwinding, so this is
# a courtesy rather than a requirement -- but it is long enough for joblib workers to go
# down with their parent instead of being orphaned.
TERMINATE_GRACE_SEC = 10.0


def ml_service_root() -> str:
    """Return the ml-service project root (directory containing the 'ml' package)."""
    import ml as _ml  # noqa: PLC0415

    return os.path.dirname(os.path.dirname(os.path.abspath(_ml.__file__)))


class _TrainingProcesses:
    """The training subprocesses this service currently has running, by module.

    It exists because of D-11: go-app's Stop cancelled its own HTTP request and reported
    `{"cancelled": 1}`, while `ml.xi.retrain` carried on inside this container burning CPU
    with nothing holding a handle to it. A process nobody can address is a process nobody
    can stop, and "cancelled" was a claim about it that was not true.

    Every method is safe to call from the worker threads that run the steps and from the
    event loop thread that serves the stop request.
    """

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._live: Dict[str, subprocess.Popen] = {}
        self._stopped: set[str] = set()

    def register(self, module: str, process: subprocess.Popen) -> None:
        with self._lock:
            self._live[module] = process
            self._stopped.discard(module)

    def unregister(self, module: str) -> None:
        with self._lock:
            self._live.pop(module, None)

    def was_stopped(self, module: str) -> bool:
        """Whether this module's last run ended because someone asked it to."""
        with self._lock:
            return module in self._stopped

    def running_modules(self) -> List[str]:
        with self._lock:
            return list(self._live)

    def stop(self, module: str, logger: Optional[Logger] = None) -> bool:
        """Terminate one module's process and *wait for it to be gone*.

        The wait is the point. Returning as soon as the signal is sent would move the
        lie one layer along -- this service would then be the one claiming a stop it had
        not confirmed. Returns False when there was nothing to stop.
        """
        with self._lock:
            process = self._live.get(module)
            if process is None:
                return False
            self._stopped.add(module)

        # The child runs in its own session (start_new_session below), so signalling the
        # group reaches the model-fitting workers it spawned. Orphaned workers were half
        # of what D-11 left burning CPU.
        _signal_group(process, signal.SIGTERM)
        try:
            process.wait(timeout=TERMINATE_GRACE_SEC)
        except subprocess.TimeoutExpired:
            if logger:
                logger.warning(
                    "pipeline: training subprocess ignored SIGTERM; killing",
                    module=module,
                    grace_sec=TERMINATE_GRACE_SEC,
                )
            _signal_group(process, signal.SIGKILL)
            process.wait()
        if logger:
            logger.info("pipeline: training subprocess stopped", module=module, returncode=process.returncode)
        return True


def _signal_group(process: subprocess.Popen, sig: int) -> None:
    """Signal a process and the group it leads, tolerating one that has already exited.

    A process that exits between the check and the signal is the normal race, not an
    error: the caller wanted it gone and it is gone.
    """
    try:
        os.killpg(os.getpgid(process.pid), sig)
    except (ProcessLookupError, PermissionError, OSError):
        try:
            process.send_signal(sig)
        except (ProcessLookupError, OSError):
            pass


_processes = _TrainingProcesses()


def stop_training(step: str = "", logger: Optional[Logger] = None) -> List[str]:
    """Stop the training subprocess of one step, or of every step when none is named.

    Returns the steps actually stopped -- an empty list when nothing was running, which
    is a true answer and not an error. Each name in it is a process this call watched
    exit, so a caller may report it as stopped without qualifying the claim.
    """
    step = (step or "").strip()
    if step and step not in TRAINING_MODULES:
        raise ValueError(f"unknown training step {step!r}; expected one of {sorted(TRAINING_MODULES)}")
    wanted = [step] if step else list(TRAINING_MODULES)
    stopped: List[str] = []
    for name in wanted:
        if _processes.stop(TRAINING_MODULES[name], logger):
            stopped.append(name)
    return stopped


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
    # Popen rather than subprocess.run, so the process is addressable while it runs: run()
    # keeps its handle on its own stack, which is why a stop had nothing to stop (D-11).
    # start_new_session puts the child at the head of its own process group, so stopping it
    # reaches the workers it spawns.
    proc = subprocess.Popen(
        cmd,
        cwd=root,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        start_new_session=True,
    )
    _processes.register(module, proc)
    try:
        stdout, stderr = proc.communicate(timeout=timeout_sec)
    except subprocess.TimeoutExpired as e:
        if logger:
            logger.error(
                "pipeline: training subprocess timed out",
                module=module,
                timeout_sec=timeout_sec,
            )
        _processes.stop(module, logger)
        raise ValueError(f"Training timed out after {timeout_sec}s") from e
    finally:
        _processes.unregister(module)
    # A run that ended because someone asked it to is not a failure, and reporting
    # "Training failed (exit -15)" would send whoever reads the log looking for a bug that
    # is not there.
    if _processes.was_stopped(module):
        if logger:
            logger.info("pipeline: training subprocess stopped on request", module=module)
        raise TrainingStopped("Training stopped on request")
    if proc.returncode != 0:
        stdout_lines = (stdout or "").strip().splitlines() if stdout else []
        stderr_lines = (stderr or "").strip().splitlines() if stderr else []
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
        TRAINING_MODULES["retrain"],
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
    run_training_subprocess(TRAINING_MODULES["evaluate"], ["--postgres", "--out", artifacts_dir], logger=logger)
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
