"""
Auto-tune progress reporting.

This is now a thin adapter over `ml.run_progress`, which is the one progress channel
every step uses (ops plan O-1). It stays as its own module because auto-tune's callers
pass a dozen tuning-specific keyword arguments — algorithm, trial, best score — that do
not generalise, and forcing them through the shared `Event` at every call site would
make the emitters harder to read for no gain.

What it does *not* do any more is own a mechanism. There is one file format, one
atomic-write path and one staleness rule, and they live in `run_progress`.

The emitted payload keeps auto-tune's fields at the top level, where its existing
consumers — `training_orchestrator.get_auto_tune_progress`, go-app's SSE payload and
the ops console — already read them. The envelope adds `v`, `run_id`, `step` and `ts`
alongside; those are additive and ignored by readers that do not want them.
"""

from __future__ import annotations

from typing import Any, Callable, Dict, Optional

from ml import run_progress

#: The step name auto-tune's progress files are written under.
STEP = "auto_tune"


def set_progress_file(path: Optional[str], run_id: str = "") -> None:
    """Set the progress file path. None disables file writing."""
    run_progress.set_progress_file(path, run_id or STEP)


def get_progress_file() -> Optional[str]:
    """Return the current progress file path."""
    return run_progress.get_progress_file()


def configure(run_id: Optional[str] = None, directory: Optional[str] = None) -> Optional[str]:
    """Point auto-tune's progress at a per-run file and return the path."""
    return run_progress.configure(STEP, run_id=run_id, directory=directory)


def set_progress_callback(cb: Optional[Callable[[Dict[str, Any]], None]]) -> None:
    """Optional callback for progress (e.g. for tests)."""
    run_progress.set_callback(cb)


def write_progress(
    phase: str,
    model_kind: str,
    format_suffix: Optional[str],
    task_index: int,
    task_total: int,
    algorithm: Optional[str] = None,
    hyperparams: Optional[Dict[str, Any]] = None,
    trial: Optional[int] = None,
    trials_total: Optional[int] = None,
    best_score: Optional[float] = None,
    best_algorithm: Optional[str] = None,
    message: Optional[str] = None,
    algorithms_screened: Optional[list] = None,
    algorithms_requested: Optional[list] = None,
    activity: Optional[str] = None,
) -> None:
    """Write auto-tune progress through the shared channel."""
    extra: Dict[str, Any] = {
        "model_kind": model_kind,
        "format_suffix": format_suffix or "",
        "task_index": task_index,
        "task_total": task_total,
    }
    optional = {
        "algorithm": algorithm,
        "hyperparams": hyperparams,
        "trial": trial,
        "trials_total": trials_total,
        "best_score": best_score,
        "best_algorithm": best_algorithm,
        "algorithms_screened": algorithms_screened,
        "algorithms_requested": algorithms_requested,
        "activity": activity,
    }
    extra.update({k: v for k, v in optional.items() if v is not None})

    run_progress.emit(
        run_progress.Event(
            step=STEP,
            phase=phase,
            # task_index/task_total are auto-tune's countable dimension, so they are
            # also the generic current/total. Kept in extra as well: the ops console
            # reads the original names, and O-5 will move it to the generic ones.
            current=task_index,
            total=task_total,
            message=message or "",
            extra=extra,
        )
    )


def clear_progress() -> None:
    """Remove progress file when auto-tune completes or fails."""
    run_progress.clear()
