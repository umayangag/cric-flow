"""Training orchestration: subprocess invocation and job helpers for /admin/train/*.

Handlers in main.py validate HTTP input and call these functions; they do not
embed subprocess or env logic. Concurrency (semaphore) remains in main so async
boundaries stay clear.
"""

from __future__ import annotations

import json
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


def export_csvs_available(prefix: str) -> bool:
    """True if GO_APP_OUTPUT_DIR contains at least one CSV matching prefix."""
    out_dir = (os.environ.get("GO_APP_OUTPUT_DIR") or "").strip()
    if not out_dir or not os.path.isdir(out_dir):
        return False
    try:
        for name in os.listdir(out_dir):
            if name.startswith(prefix) and name.endswith(".csv"):
                return True
    except OSError:
        pass
    return False


def unified_batting_csv_available() -> bool:
    """True if batting_encoded_all.csv or batting_encoded.csv exists in export dir."""
    from ml.config import default_go_app_export_dir

    out_dir = (os.environ.get("GO_APP_OUTPUT_DIR") or "").strip() or default_go_app_export_dir()
    if not out_dir or not os.path.isdir(out_dir):
        return False
    return os.path.isfile(os.path.join(out_dir, "batting_encoded_all.csv")) or os.path.isfile(
        os.path.join(out_dir, "batting_encoded.csv")
    )


def unified_bowling_csv_available() -> bool:
    """True if bowling_encoded_all.csv or bowling_encoded.csv exists in export dir."""
    from ml.config import default_go_app_export_dir

    out_dir = (os.environ.get("GO_APP_OUTPUT_DIR") or "").strip() or default_go_app_export_dir()
    if not out_dir or not os.path.isdir(out_dir):
        return False
    return os.path.isfile(os.path.join(out_dir, "bowling_encoded_all.csv")) or os.path.isfile(
        os.path.join(out_dir, "bowling_encoded.csv")
    )


def run_batting_training(cutoff: str, go_app_url: str, logger: Optional[Logger] = None) -> None:
    """Run per-format batting training, then unified if CSV available. Raises ValueError on failure."""
    csv_available = export_csvs_available("batting_encoded_")
    use_api = bool(cutoff) and not csv_available
    if use_api:
        extra = ["--from-api", "--cutoff", cutoff, "--all-formats", "--go-app-url", go_app_url]
        if logger:
            logger.info("admin.train.start", step="batting", per_format=True, from_api=True, go_app_url=go_app_url)
    else:
        extra = ["--all-formats"]
        if logger:
            logger.info(
                "admin.train.start",
                step="batting",
                per_format=True,
                from_api=False,
                from_csv=bool(cutoff and csv_available),
            )
    run_training_subprocess("ml.train_batting", extra, {"ML_N_JOBS": "-1"}, logger=logger)
    if unified_batting_csv_available():
        try:
            from ml.train_batting_model import run_training as run_unified_batting

            run_unified_batting()
            if logger:
                logger.info("admin.train.success", step="batting", unified=True)
        except Exception as e:
            if logger:
                logger.warning("admin.train.unified_batting_failed", error=str(e))
    else:
        if logger:
            logger.info("admin.train.success", step="batting", unified=False)


def run_bowling_training(cutoff: str, go_app_url: str, logger: Optional[Logger] = None) -> None:
    """Run per-format bowling training, then unified if CSV available. Raises ValueError on failure."""
    csv_available = export_csvs_available("bowling_encoded_")
    use_api = bool(cutoff) and not csv_available
    if use_api:
        extra = ["--from-api", "--cutoff", cutoff, "--all-formats", "--go-app-url", go_app_url]
        if logger:
            logger.info("admin.train.start", step="bowling", per_format=True, from_api=True, go_app_url=go_app_url)
    else:
        extra = ["--all-formats"]
        if logger:
            logger.info(
                "admin.train.start",
                step="bowling",
                per_format=True,
                from_api=False,
                from_csv=bool(cutoff and csv_available),
            )
    run_training_subprocess("ml.train_bowling", extra, {"ML_N_JOBS": "-1"}, logger=logger)
    if unified_bowling_csv_available():
        try:
            from ml.train_bowling_model import run_training as run_unified_bowling

            run_unified_bowling()
            if logger:
                logger.info("admin.train.success", step="bowling", unified=True)
        except Exception as e:
            if logger:
                logger.warning("admin.train.unified_bowling_failed", error=str(e))
    else:
        if logger:
            logger.info("admin.train.success", step="bowling", unified=False)


def run_fielding_training(cutoff: str, go_app_url: str, logger: Optional[Logger] = None) -> None:
    """Run fielding training. If cutoff: from API; else CSV. Raises ValueError on failure."""
    if cutoff:
        args = ["--cutoff", cutoff, "--go-app-url", go_app_url]
        if logger:
            logger.info("admin.train.start", step="fielding", cutoff=cutoff, go_app_url=go_app_url)
    else:
        args = []
        if logger:
            logger.info("admin.train.start", step="fielding", source="csv")
    run_training_subprocess("ml.train_fielding", args, {"ML_N_JOBS": "-1"}, logger=logger)
    if logger:
        logger.info("admin.train.success", step="fielding")


def run_extras_training(cutoff: str, go_app_url: str, logger: Optional[Logger] = None) -> None:
    """Run extras training (requires cutoff, uses go-app API). Raises ValueError on failure."""
    if logger:
        logger.info("admin.train.start", step="extras", cutoff=cutoff, go_app_url=go_app_url)
    run_training_subprocess(
        "ml.train_extras",
        ["--cutoff", cutoff, "--go-app-url", go_app_url],
        {"ML_N_JOBS": "-1"},
        logger=logger,
    )
    if logger:
        logger.info("admin.train.success", step="extras")


def run_win_training(cutoff: str, go_app_url: str, logger: Optional[Logger] = None) -> None:
    """Run win model training (requires cutoff). Raises ValueError on failure."""
    if logger:
        logger.info("admin.train.start", step="win", cutoff=cutoff, go_app_url=go_app_url)
    run_training_subprocess(
        "ml.train_win",
        ["--cutoff", cutoff, "--go-app-url", go_app_url],
        {"ML_N_JOBS": "-1"},
        logger=logger,
    )
    if logger:
        logger.info("admin.train.success", step="win")


def run_innings_training(cutoff: str, go_app_url: str, logger: Optional[Logger] = None) -> None:
    """Run innings model training (requires cutoff). Raises ValueError on failure."""
    if logger:
        logger.info("admin.train.start", step="innings", cutoff=cutoff, go_app_url=go_app_url)
    run_training_subprocess(
        "ml.train_innings",
        ["--cutoff", cutoff, "--go-app-url", go_app_url],
        {"ML_N_JOBS": "-1"},
        logger=logger,
    )
    if logger:
        logger.info("admin.train.success", step="innings")


def get_auto_tune_progress_path() -> str:
    """Return path to auto-tune progress JSON file."""
    path = os.environ.get("AUTO_TUNE_PROGRESS_FILE")
    if path:
        return path
    try:
        from ml.config import default_artifacts_dir

        return os.path.join(default_artifacts_dir(), "auto_tune_progress.json")
    except Exception:
        return os.path.join("..", "..", "output", "ml-service", "auto_tune_progress.json")


def get_auto_tune_progress() -> Dict[str, Any]:
    """Read and return auto-tune progress JSON; empty dict if missing or invalid."""
    path = get_auto_tune_progress_path()
    if not os.path.isfile(path):
        return {}
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except (json.JSONDecodeError, OSError):
        return {}


VALID_AUTO_TUNE_MODELS = ("batting", "bowling", "fielding", "extras", "win", "innings", "all")
VALID_AUTO_TUNE_FORMATS = ("TEST", "ODI", "T20", "T20I")


def run_auto_tune(
    cutoff: str,
    go_app_url: str,
    model: str,
    use_all_formats: bool,
    use_unified: bool,
    fmt: str,
    rescreen: bool,
    algorithms: str,
    logger: Optional[Logger] = None,
) -> None:
    """Run auto-tune subprocess. Raises ValueError on failure."""
    extra = [
        "--model",
        model,
        "--from-api",
        "--cutoff",
        cutoff,
        "--go-app-url",
        go_app_url,
    ]
    if use_all_formats:
        extra.append("--all-formats")
    elif use_unified:
        extra.append("--unified")
    elif not use_unified:
        extra.extend(["--format", fmt])
    if rescreen:
        extra.append("--rescreen")
    if algorithms.strip():
        extra.extend(["--algorithms", algorithms.strip()])
    extra.append("--parallel")
    subprocess_env: Optional[Dict[str, str]] = None
    single_task = model != "all" and (not use_all_formats or use_unified)
    if single_task:
        subprocess_env = {"AUTO_TUNE_N_JOBS": "-1"}
    if logger:
        logger.info(
            "admin.train.start",
            step="auto-tune",
            cutoff=cutoff,
            go_app_url=go_app_url,
            model=model,
            all_formats=use_all_formats,
            unified=use_unified,
            format=fmt or None,
            rescreen=rescreen,
            algorithms=algorithms.strip() or None,
            single_task=single_task,
        )
    run_training_subprocess("ml.auto_tune", extra, subprocess_env, logger=logger)
    if logger:
        logger.info("admin.train.success", step="auto-tune")
