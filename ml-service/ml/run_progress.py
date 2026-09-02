"""
Cross-process progress reporting for pipeline steps.

A training step runs as a subprocess of ml-service, which itself is a subprocess of
nothing go-app can see into. `training_orchestrator.run_training_subprocess` calls
`subprocess.run(..., capture_output=True)`, which buffers everything until exit: on
success the output is discarded, on failure only a tail is logged. During a ten-minute
training run there is nothing to observe, anywhere (ops plan, gap 2).

A subprocess cannot push into its parent's memory, but it can write a file. `retrain`
already solved this once; this module is that mechanism generalised so every step uses
one channel rather than growing a second (ops plan O-1).

Three properties this module owes its callers:

- **One file per run.** A single global path means two runs — concurrent, or one
  started before the reader noticed the last finished — overwrite each other's state,
  and the reader cannot tell which run it is looking at.
- **Atomic writes.** A reader polling a file being written sees a truncated JSON
  document. Writing to a temp file and renaming means a reader sees the old event or
  the new one, never half of either.
- **Emission never breaks the step.** Progress is telemetry. A full disk or a
  read-only directory must not fail a training run that is otherwise fine, so every
  failure here is logged and swallowed.
"""

from __future__ import annotations

import glob
import json
import logging
import os
import re
import tempfile
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Callable, Dict, List, Optional

logger = logging.getLogger(__name__)

#: Schema version of the emitted event. Bump on a breaking change to the envelope so a
#: reader can tell what it is holding rather than guessing from which keys are present.
SCHEMA_VERSION = 1

#: Subdirectory under the artifacts directory holding per-run progress files.
PROGRESS_DIRNAME = "progress"

#: Suffix for a run's terminal summary, written when the run ends.
#:
#: A result is not progress. The progress file is removed when the run finishes --
#: a file left behind reads as a run still going -- but the *outcome* has to survive
#: that, because whoever wants it (go-app, persisting into data_migrations.metadata)
#: asks only after the run is over. Two files, two lifetimes.
RESULT_SUFFIX = ".result.json"

#: Envelope keys. `extra` may not overwrite these — a step-specific field that shadowed
#: `step` or `run_id` would make the event describe the wrong run.
RESERVED_KEYS = frozenset({"v", "run_id", "step", "phase", "current", "total", "metrics", "message", "ts"})

#: Progress files older than this are treated as abandoned by `latest_for_step`. A
#: crashed run leaves its file behind, and a stale file read as live is worse than no
#: progress at all: it shows a run that is not happening.
STALE_AFTER_SEC = 30 * 60

#: Anything outside this becomes an underscore in a filename component. Run ids reach
#: us from go-app and from env, so they are sanitised rather than trusted -- a run id
#: of "../../etc/x" must not decide where this process writes.
_UNSAFE_NAME = re.compile(r"[^A-Za-z0-9._-]+")


@dataclass
class Event:
    """One progress observation.

    `step` and `phase` say where the run is; `current`/`total` say how far through a
    countable thing it is (folds, formats, epochs); `metrics` carries numbers worth
    showing (rmse, accuracy). `extra` is for step-specific fields that do not
    generalise -- the grid's chosen params, for instance.
    """

    step: str
    phase: str = ""
    current: Optional[int] = None
    total: Optional[int] = None
    metrics: Optional[Dict[str, Any]] = None
    message: str = ""
    extra: Dict[str, Any] = field(default_factory=dict)

    def to_payload(self, run_id: str) -> Dict[str, Any]:
        """Render the event as the JSON object written to the progress file."""
        payload: Dict[str, Any] = {
            "v": SCHEMA_VERSION,
            "run_id": run_id,
            "step": self.step,
            "ts": datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z"),
        }
        if self.phase:
            payload["phase"] = self.phase
        if self.current is not None:
            payload["current"] = self.current
        if self.total is not None:
            payload["total"] = self.total
        if self.metrics:
            payload["metrics"] = self.metrics
        if self.message:
            payload["message"] = self.message
        # Merged at the top level rather than nested so a consumer of one step's
        # fields reads them where it always did. Reserved keys win.
        for key, value in self.extra.items():
            if key not in RESERVED_KEYS:
                payload[key] = value
        return payload


# Module state. A step runs in its own process, so one channel per process is the
# whole of the concurrency model.
_progress_file: Optional[str] = None
_run_id: str = ""
_callback: Optional[Callable[[Dict[str, Any]], None]] = None


def safe_component(value: str) -> str:
    """Reduce a string to something safe to put in a filename."""
    cleaned = _UNSAFE_NAME.sub("_", (value or "").strip())
    return cleaned.strip("._-") or "unknown"


def progress_dir(directory: Optional[str] = None) -> str:
    """Return the directory holding progress files."""
    if directory:
        return os.path.join(directory, PROGRESS_DIRNAME)
    try:
        from ml.config import default_artifacts_dir

        base = default_artifacts_dir()
    except Exception:  # pragma: no cover - config import failure is environment-specific
        base = os.path.join("..", "..", "output", "ml-service")
    return os.path.join(base, PROGRESS_DIRNAME)


def progress_path(step: str, run_id: str, directory: Optional[str] = None) -> str:
    """Return the progress file path for one run of one step."""
    name = f"{safe_component(step)}__{safe_component(run_id)}.json"
    return os.path.join(progress_dir(directory), name)


def result_path(step: str, run_id: str, directory: Optional[str] = None) -> str:
    """Return the terminal-summary path for one run of one step."""
    name = f"{safe_component(step)}__{safe_component(run_id)}{RESULT_SUFFIX}"
    return os.path.join(progress_dir(directory), name)


def write_result(summary: Dict[str, Any]) -> Optional[str]:
    """Persist a run's terminal summary beside its progress file. Never raises."""
    path = _progress_file
    if not path:
        return None
    target = path[: -len(".json")] + RESULT_SUFFIX if path.endswith(".json") else path + RESULT_SUFFIX
    _write_atomic(target, summary)
    return target


def read_result(step: str, run_id: str, directory: Optional[str] = None) -> Dict[str, Any]:
    """Read one run's terminal summary. Returns {} when it is missing or unreadable."""
    return read(result_path(step, run_id, directory))


def latest_result_for_step(step: str, directory: Optional[str] = None) -> Dict[str, Any]:
    """Return the newest terminal summary for a step, or {}.

    No staleness rule here, unlike `latest_for_step`: a result is *supposed* to outlive
    its run. Age says nothing about whether it is the answer being asked for.
    """
    pattern = os.path.join(progress_dir(directory), f"{safe_component(step)}__*{RESULT_SUFFIX}")
    try:
        paths = sorted(glob.glob(pattern), key=_mtime, reverse=True)
    except OSError:  # pragma: no cover - glob rarely raises
        return {}
    for path in paths:
        payload = read(path)
        if payload:
            return payload
    return {}


def configure(step: str, run_id: Optional[str] = None, directory: Optional[str] = None) -> Optional[str]:
    """Point this process's progress channel at a file for one run, and return it.

    `run_id` defaults to the process id, which is unique among live runs and stable
    for the run's lifetime -- enough to keep two runs apart without requiring the
    caller to invent an identifier it has no use for.
    """
    resolved_run = str(run_id or os.getpid())
    path = progress_path(step, resolved_run, directory)
    set_progress_file(path, resolved_run)
    return path


def set_progress_file(path: Optional[str], run_id: str = "") -> None:
    """Set the progress file directly. None disables writing."""
    global _progress_file, _run_id
    _progress_file = path
    _run_id = run_id or _run_id


def get_progress_file() -> Optional[str]:
    """Return the current progress file path, or None when writing is disabled."""
    return _progress_file


def get_run_id() -> str:
    """Return the run id events are stamped with."""
    return _run_id


def set_callback(cb: Optional[Callable[[Dict[str, Any]], None]]) -> None:
    """Set an in-process observer of emitted payloads. Used by tests."""
    global _callback
    _callback = cb


def emit(event: Event) -> None:
    """Publish one progress event. Never raises."""
    try:
        payload = event.to_payload(_run_id)
    except Exception as e:  # pragma: no cover - to_payload is total over its inputs
        logger.warning("run_progress.render_failed step=%s error=%s", event.step, e)
        return

    if _callback is not None:
        try:
            _callback(payload)
        except Exception as e:
            logger.warning("run_progress.callback_failed error=%s", e)

    path = _progress_file
    if not path:
        return
    _write_atomic(path, payload)


def clear() -> None:
    """Remove this run's progress file. Never raises.

    Callers put this in a `finally`: a file left behind after the run ends is read as
    a run still in flight, which is a worse answer than no progress at all.
    """
    path = _progress_file
    if not path:
        return
    try:
        os.remove(path)
    except FileNotFoundError:
        pass
    except OSError as e:
        logger.warning("run_progress.clear_failed path=%s error=%s", path, e)


def read(path: str) -> Dict[str, Any]:
    """Read one progress file. Returns {} when it is missing or unreadable.

    A partially written file cannot be observed -- writes are atomic -- but a file
    written by an older version, or truncated by a full disk mid-rename, still can be,
    and neither is worth raising over.
    """
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (FileNotFoundError, NotADirectoryError):
        return {}
    except (json.JSONDecodeError, OSError) as e:
        logger.warning("run_progress.read_failed path=%s error=%s", path, e)
        return {}
    return data if isinstance(data, dict) else {}


def list_for_step(step: str, directory: Optional[str] = None) -> List[str]:
    """Return progress files for a step, newest first."""
    pattern = os.path.join(progress_dir(directory), f"{safe_component(step)}__*.json")
    try:
        paths = glob.glob(pattern)
    except OSError:  # pragma: no cover - glob rarely raises
        return []
    # Result files also end in .json and would otherwise be read as live progress —
    # which is exactly the "a finished run looks like a running one" confusion the
    # two separate lifetimes exist to avoid.
    paths = [p for p in paths if not p.endswith(RESULT_SUFFIX)]
    return sorted(paths, key=_mtime, reverse=True)


def latest_for_step(
    step: str, directory: Optional[str] = None, stale_after_sec: int = STALE_AFTER_SEC
) -> Dict[str, Any]:
    """Return the most recent live progress for a step, or {}.

    Files older than `stale_after_sec` are skipped: a crashed run leaves its file
    behind, and reporting it as current shows a run that is not happening. Pass 0 to
    disable the check.
    """
    now = time.time()
    for path in list_for_step(step, directory):
        if stale_after_sec > 0 and (now - _mtime(path)) > stale_after_sec:
            continue
        payload = read(path)
        if payload:
            return payload
    return {}


def _mtime(path: str) -> float:
    try:
        return os.path.getmtime(path)
    except OSError:
        return 0.0


def _write_atomic(path: str, payload: Dict[str, Any]) -> None:
    """Write JSON via a temp file and rename, so a reader never sees a partial document."""
    directory = os.path.dirname(path) or "."
    tmp_path = ""
    try:
        os.makedirs(directory, exist_ok=True)
        # The temp file must share a directory with the destination: os.replace is
        # only atomic within one filesystem, and /tmp is often a different one.
        fd, tmp_path = tempfile.mkstemp(dir=directory, prefix=".progress-", suffix=".tmp")
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(payload, f, indent=2)
            f.flush()
            os.fsync(f.fileno())
        os.replace(tmp_path, path)
        tmp_path = ""
    except OSError as e:
        logger.warning("run_progress.write_failed path=%s error=%s", path, e)
    finally:
        if tmp_path:
            try:
                os.remove(tmp_path)
            except OSError:
                pass
