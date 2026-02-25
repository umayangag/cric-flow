"""
Resource-aware concurrency for ML training and tuning.

Uses available CPU and optional memory limits to suggest n_jobs so that:
- With more resources we use more parallelism.
- With less memory/CPU we use less to avoid OOM and thrashing.

Per-job MB and memory fraction come from config (ml.resources) when present.
"""

import logging
import os

logger = logging.getLogger(__name__)

# Defaults when config ml.resources is missing (documented in config.default.json).
_DEFAULT_TRAINING_MB_PER_JOB = 400
_DEFAULT_TUNING_MB_PER_JOB = 500
_DEFAULT_PREDICTION_MB_PER_JOB = 100
_DEFAULT_MEMORY_USAGE_FRACTION_PERCENT = 80
# When process memory limit (MB) is at or below this, training uses 1 job to avoid OOM.
_DEFAULT_TRAINING_LOW_MEMORY_THRESHOLD_MB = 2560


def _get_resources_config():  # lazy import to avoid circular dependency with config
    from ml import config as _config

    cfg = _config.get_config()
    return (cfg.get("ml") or {}).get("resources") or {}


def _cpu_count() -> int:
    n = os.cpu_count()
    return n if n is not None and n >= 1 else 1


def _memory_limit_mb() -> int:
    """Detect process memory limit in MB (cgroup v2/v1 or env). Returns 0 if unknown."""
    # Explicit env (e.g. set by orchestrator)
    env_mb = os.environ.get("ML_MEMORY_LIMIT_MB")
    if env_mb:
        try:
            return int(env_mb)
        except ValueError:
            pass
    # cgroup v2: memory.max
    for path in (
        "/sys/fs/cgroup/memory.max",
        "/sys/fs/cgroup/memory/memory.limit_in_bytes",
    ):
        try:
            with open(path, encoding="utf-8") as f:
                s = f.read().strip()
            if not s or s == "max":
                continue
            limit_bytes = int(s)
            if limit_bytes <= 0:
                continue
            return limit_bytes // (1024 * 1024)
        except (OSError, ValueError):
            continue
    return 0


def suggested_n_jobs(kind: str = "training") -> int:
    """
    Return a resource-aware n_jobs value for the given workload kind.

    kind: "training" | "tuning" | "prediction"
    Precedence: ML_N_JOBS env > min(cpu_count, memory_based_cap, ML_N_JOBS_MAX).
    """
    explicit = os.environ.get("ML_N_JOBS")
    if explicit is not None:
        try:
            n = int(explicit)
            if n >= 1:
                return n
            if n != -1:
                logger.warning(
                    "ML_N_JOBS=%d is invalid (must be >= 1 or -1 for auto); falling back to auto-detection.", n
                )
            # Fall through for n == -1 or other invalid values to use auto-detection
        except ValueError:
            pass

    cpu = _cpu_count()
    cap_env = os.environ.get("ML_N_JOBS_MAX")
    cap = cpu
    if cap_env is not None:
        try:
            cap = min(cpu, int(cap_env))
        except ValueError:
            pass

    limit_mb = _memory_limit_mb()
    # When limit is unknown (e.g. subprocess in container without env/cgroup), training uses 1 job to avoid OOM
    if kind == "training" and limit_mb <= 0:
        cap = min(cap, 1)
    if limit_mb > 0:
        res = _get_resources_config()
        if kind == "training":
            per_job = res.get("training_mb_per_job", _DEFAULT_TRAINING_MB_PER_JOB)
        elif kind == "tuning":
            per_job = res.get("tuning_mb_per_job", _DEFAULT_TUNING_MB_PER_JOB)
        else:
            per_job = res.get("prediction_mb_per_job", _DEFAULT_PREDICTION_MB_PER_JOB)
        frac = res.get("memory_usage_fraction_percent", _DEFAULT_MEMORY_USAGE_FRACTION_PERCENT)
        if not isinstance(per_job, (int, float)) or per_job < 1:
            per_job = (
                _DEFAULT_TRAINING_MB_PER_JOB
                if kind == "training"
                else _DEFAULT_TUNING_MB_PER_JOB
                if kind == "tuning"
                else _DEFAULT_PREDICTION_MB_PER_JOB
            )
        if not isinstance(frac, (int, float)) or frac < 1:
            frac = _DEFAULT_MEMORY_USAGE_FRACTION_PERCENT
        threshold_mb = res.get("training_low_memory_threshold_mb", _DEFAULT_TRAINING_LOW_MEMORY_THRESHOLD_MB)
        if not isinstance(threshold_mb, (int, float)) or threshold_mb < 0:
            threshold_mb = _DEFAULT_TRAINING_LOW_MEMORY_THRESHOLD_MB
        # When memory is tight, treat usable fraction as one job to avoid OOM (fielding/extras/win use significant data + model memory)
        usable_mb = limit_mb * int(frac) // 100
        if kind == "training" and usable_mb > 0 and limit_mb <= int(threshold_mb):
            per_job = max(int(per_job), usable_mb)
        # Use at most frac% of limit for worker processes
        memory_cap = max(1, usable_mb // int(per_job))
        cap = min(cap, memory_cap)
        logger.debug(
            "resources: memory-based n_jobs cap kind=%s limit_mb=%s per_job=%s cap=%s",
            kind,
            limit_mb,
            per_job,
            cap,
        )

    n = max(1, cap)
    return n
