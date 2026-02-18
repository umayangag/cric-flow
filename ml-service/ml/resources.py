"""
Resource-aware concurrency for ML training and tuning.

Uses available CPU and optional memory limits to suggest n_jobs so that:
- With more resources we use more parallelism.
- With less memory/CPU we use less to avoid OOM and thrashing.
"""

import logging
import os

logger = logging.getLogger(__name__)

# Estimated memory per parallel job (MB) for training (RandomForest/GBM trees).
# Used when a memory limit is detectable (e.g. cgroup in containers).
_TRAINING_MB_PER_JOB = 400
_TUNING_MB_PER_JOB = 500  # tuning runs multiple CV fits
_PREDICTION_MB_PER_JOB = 100


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
            if n == -1:
                pass  # fall through to auto
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
    if limit_mb > 0:
        if kind == "training":
            per_job = _TRAINING_MB_PER_JOB
        elif kind == "tuning":
            per_job = _TUNING_MB_PER_JOB
        else:
            per_job = _PREDICTION_MB_PER_JOB
        # Use at most 70% of limit for worker processes
        memory_cap = max(1, (limit_mb * 70 // 100) // per_job)
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
