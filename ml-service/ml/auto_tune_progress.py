"""
Auto-tune progress reporting. Writes live status to a JSON file for frontend consumption.
Read by GET /admin/train/auto-tune/progress.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Callable, Dict, Optional

logger = logging.getLogger(__name__)

_PROGRESS_FILE: Optional[str] = None
_PROGRESS_CALLBACK: Optional[Callable[[Dict[str, Any]], None]] = None


def set_progress_file(path: Optional[str]) -> None:
    """Set the progress file path. None disables file writing."""
    global _PROGRESS_FILE
    _PROGRESS_FILE = path


def get_progress_file() -> Optional[str]:
    """Return the current progress file path."""
    return _PROGRESS_FILE


def set_progress_callback(cb: Optional[Callable[[Dict[str, Any]], None]]) -> None:
    """Optional callback for progress (e.g. for tests)."""
    global _PROGRESS_CALLBACK
    _PROGRESS_CALLBACK = cb


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
    """Write progress to file and optional callback."""
    payload: Dict[str, Any] = {
        "phase": phase,
        "model_kind": model_kind,
        "format_suffix": format_suffix or "",
        "task_index": task_index,
        "task_total": task_total,
    }
    if algorithm is not None:
        payload["algorithm"] = algorithm
    if hyperparams is not None:
        payload["hyperparams"] = hyperparams
    if trial is not None:
        payload["trial"] = trial
    if trials_total is not None:
        payload["trials_total"] = trials_total
    if best_score is not None:
        payload["best_score"] = best_score
    if best_algorithm is not None:
        payload["best_algorithm"] = best_algorithm
    if message is not None:
        payload["message"] = message
    if algorithms_screened is not None:
        payload["algorithms_screened"] = algorithms_screened
    if algorithms_requested is not None:
        payload["algorithms_requested"] = algorithms_requested
    if activity is not None:
        payload["activity"] = activity

    if _PROGRESS_CALLBACK:
        try:
            _PROGRESS_CALLBACK(payload)
        except Exception as e:
            logger.warning("auto_tune_progress.callback_failed error=%s", e)

    path = _PROGRESS_FILE
    if not path:
        return
    try:
        os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            json.dump(payload, f, indent=2)
    except OSError as e:
        logger.warning("auto_tune_progress.write_failed path=%s error=%s", path, e)


def clear_progress() -> None:
    """Remove progress file when auto-tune completes or fails."""
    path = _PROGRESS_FILE
    if path and os.path.isfile(path):
        try:
            os.remove(path)
        except OSError as e:
            logger.warning("auto_tune_progress.clear_failed path=%s error=%s", path, e)
