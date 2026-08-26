"""
Training-step progress: the milestones a trainer publishes while it runs.

This sits on `ml.run_progress` (the transport) and gives the six trainers a vocabulary
instead of a dict to fill in. The distinction matters: every trainer emitting the same
named milestones is what lets one panel render all of them, and what stops "rows" being
`rows` in one step and `n_rows` in the next.

The agreed fidelity for this plan is **structured milestones and metrics**, not raw
stdout (ops plan, scope decisions). `run_training_subprocess` keeps
`capture_output=True`; the events carry the signal.

Every function here is total: an emitter that raised would turn a successful
ten-minute training run into a failed one, which is a bad trade for telemetry. The
transport already swallows its own errors; these wrappers also guard the *argument
computation*, because statting a file or summarising an array can fail on its own.
"""

from __future__ import annotations

import functools
import logging
import os
import threading
from typing import Any, Callable, Dict, Iterable, List, Mapping, Optional, Sequence, TypeVar, cast

from ml import run_progress
from ml.run_progress import Event

logger = logging.getLogger(__name__)

F = TypeVar("F", bound=Callable[..., Any])

#: Phases a training run moves through. Named here so a reader can switch on them
#: rather than matching free text.
PHASE_LOAD = "load"
PHASE_FEATURES = "features"
PHASE_FIT = "fit"
PHASE_CV = "cv"
PHASE_ARTIFACT = "artifact"
PHASE_DONE = "done"

#: Cap on how many dropped column names travel in one event. A pathological run could
#: drop hundreds; the count is the signal, the names are the detail, and a progress
#: file is not a log.
MAX_REPORTED_COLUMNS = 40


def never_raises(fn: F) -> F:
    """Make an emitter total.

    Guarding only the transport is not enough: the failure that reaches a caller is
    usually in the *arguments* -- statting a path that turned out to be None, coercing
    a row count that arrived as a string, summarising an array that is not one. A
    trainer that finished successfully must not be reported as failed because a label
    was the wrong type, so the guard wraps the whole call.
    """

    @functools.wraps(fn)
    def wrapper(*args: Any, **kwargs: Any) -> Any:
        try:
            return fn(*args, **kwargs)
        except Exception as e:
            logger.warning("training_progress.%s_failed error=%s", fn.__name__, e)
            return None

    return cast(F, wrapper)


def step_name(model_name: str) -> str:
    """Map a model name to the pipeline step id go-app and the UI already use.

    `train_batting`, not `batting`: the step registry, `data_migrations.command` and
    the ops console all speak in step ids, and a second vocabulary here would need
    translating at every boundary.
    """
    try:
        name = (model_name or "").strip()
    except Exception:
        name = ""
    if not name:
        return "train_unknown"
    return name if name.startswith("train_") else f"train_{name}"


class _FormatCounter:
    """Counts finished formats across the threads that train them.

    Formats train concurrently (`ML_TRAIN_FORMAT_WORKERS`), so "which format is
    running" has no single answer and `current` cannot mean "the one in flight".
    Counting *completed* formats is a number that stays true under concurrency, and
    the format each event is about travels in `extra`.
    """

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._total = 0
        self._completed = 0

    def reset(self, total: int) -> None:
        with self._lock:
            self._total = max(0, int(total))
            self._completed = 0

    def complete_one(self) -> tuple:
        with self._lock:
            self._completed += 1
            return self._completed, self._total

    def snapshot(self) -> tuple:
        with self._lock:
            return self._completed, self._total


_formats = _FormatCounter()


@never_raises
def start(model_name: str, total_formats: int = 0, out_dir: Optional[str] = None) -> Optional[str]:
    """Open the progress channel for a training run. Returns the file path, or None."""
    step = step_name(model_name)
    try:
        _formats.reset(total_formats)
        run_id = os.environ.get("PIPELINE_RUN_ID") or None
        path = run_progress.configure(step, run_id=run_id, directory=out_dir)
        _emit(Event(step=step, phase=PHASE_LOAD, current=0, total=total_formats, message="Starting"))
        return path
    except Exception as e:  # pragma: no cover - configure is defensive already
        logger.warning("training_progress.start_failed model=%s error=%s", model_name, e)
        return None


@never_raises
def set_total_formats(total: int) -> None:
    """Tell the counter how many formats this run will train, and start counting.

    Separate from `start` because the trainers that build their own format map only
    know the count after loading the data, and a total of zero would render as
    "3 of 0" rather than as "unknown".

    It resets the completed count as well as the total. Every caller invokes it before
    the training loop begins, so there is no progress to preserve — and a version that
    tried to preserve some would be carrying state across runs in the same process,
    which is how a second run starts at "4 of 4".
    """
    try:
        _formats.reset(total)
    except Exception as e:  # pragma: no cover - arithmetic on ints
        logger.warning("training_progress.set_total_failed error=%s", e)


@never_raises
def finish(model_name: str, saved: int = 0) -> None:
    """Emit a final event and close the channel.

    `clear` is deliberate: a progress file left behind reads as a run still going,
    which is worse than no progress at all.
    """
    step = step_name(model_name)
    completed, total = _formats.snapshot()
    _emit(
        Event(
            step=step,
            phase=PHASE_DONE,
            current=completed,
            total=total,
            message=f"Saved {saved} model(s)",
            extra={"saved": saved},
        )
    )
    try:
        run_progress.clear()
    except Exception as e:  # pragma: no cover - clear is defensive already
        logger.warning("training_progress.finish_failed model=%s error=%s", model_name, e)


@never_raises
def data_loaded(
    model_name: str,
    fmt: Optional[str],
    rows: int,
    features: int,
    targets: Optional[int] = None,
) -> None:
    """Report the shape of what was loaded.

    Rows and columns are the first thing worth knowing: a training run against 200
    rows because the export was stale looks exactly like one against 200,000 until
    something says otherwise.
    """
    metrics: Dict[str, Any] = {"rows": int(rows), "features": int(features)}
    if targets is not None:
        metrics["targets"] = int(targets)
    completed, total = _formats.snapshot()
    _emit(
        Event(
            step=step_name(model_name),
            phase=PHASE_LOAD,
            current=completed,
            total=total,
            metrics=metrics,
            message=f"Loaded {rows:,} rows x {features} features" + (f" ({fmt})" if fmt else ""),
            extra=_fmt_extra(fmt),
        )
    )


@never_raises
def columns_dropped(model_name: str, fmt: Optional[str], dropped: Sequence[str], kept: int) -> None:
    """Report low-variance columns removed before fitting.

    This is the one the plan singles out. Every trainer already calls
    `drop_low_variance_columns` and every one discarded the returned list into
    `_dropped`, so a feature that had gone silently constant -- an export bug, a
    column the pipeline stopped populating -- was removed without anyone being told.
    Emitting it is how that becomes visible instead of quietly discarded.
    """
    names = list(dropped or [])
    if not names:
        return
    extra: Dict[str, Any] = _fmt_extra(fmt)
    extra["dropped_columns"] = names[:MAX_REPORTED_COLUMNS]
    if len(names) > MAX_REPORTED_COLUMNS:
        extra["dropped_columns_truncated"] = len(names) - MAX_REPORTED_COLUMNS
    completed, total = _formats.snapshot()
    _emit(
        Event(
            step=step_name(model_name),
            phase=PHASE_FEATURES,
            current=completed,
            total=total,
            metrics={"dropped": len(names), "kept": int(kept)},
            message=f"Dropped {len(names)} low-variance column(s)",
            extra=extra,
        )
    )


@never_raises
def fold(
    model_name: str,
    fmt: Optional[str],
    index: int,
    total_folds: int,
    metrics: Optional[Mapping[str, Any]] = None,
) -> None:
    """Report one cross-validation fold and its metrics.

    Only the trainers that cross-validate call this -- `train_win` splits with
    `TimeSeriesSplit`; the rest fit once. A step that emitted fake folds to look busy
    would be worse than one that says nothing.
    """
    _emit(
        Event(
            step=step_name(model_name),
            phase=PHASE_CV,
            current=int(index),
            total=int(total_folds),
            metrics=_clean_metrics(metrics),
            message=f"Fold {index}/{total_folds}" + (f" ({fmt})" if fmt else ""),
            extra=_fmt_extra(fmt),
        )
    )


@never_raises
def fitting(model_name: str, fmt: Optional[str], rows: int, features: int) -> None:
    """Report that fitting has begun. This is the long, silent part of a run."""
    completed, total = _formats.snapshot()
    _emit(
        Event(
            step=step_name(model_name),
            phase=PHASE_FIT,
            current=completed,
            total=total,
            metrics={"rows": int(rows), "features": int(features)},
            message=f"Fitting on {rows:,} rows" + (f" ({fmt})" if fmt else ""),
            extra=_fmt_extra(fmt),
        )
    )


@never_raises
def artifact_written(model_name: str, fmt: Optional[str], paths: Iterable[str]) -> None:
    """Report the artifacts a format produced, with their sizes.

    Size is the postcondition worth checking: a model file that exists but is a few
    hundred bytes is a failed fit that reported success, which is the shape of failure
    this repo keeps meeting.
    """
    written: List[Dict[str, Any]] = []
    total_bytes = 0
    for path in paths or []:
        try:
            size = os.path.getsize(path)
        except OSError:
            continue
        written.append({"path": os.path.basename(path), "bytes": int(size)})
        total_bytes += int(size)
    if not written:
        return

    completed, total = _formats.snapshot()
    extra = _fmt_extra(fmt)
    extra["artifacts"] = written
    _emit(
        Event(
            step=step_name(model_name),
            phase=PHASE_ARTIFACT,
            current=completed,
            total=total,
            metrics={"artifact_bytes": total_bytes, "artifact_count": len(written)},
            message=f"Wrote {len(written)} artifact(s)" + (f" for {fmt}" if fmt else ""),
            extra=extra,
        )
    )


@never_raises
def format_done(model_name: str, fmt: Optional[str], metrics: Optional[Mapping[str, Any]] = None) -> None:
    """Mark one format finished and advance the completed count."""
    completed, total = _formats.complete_one()
    _emit(
        Event(
            step=step_name(model_name),
            phase=PHASE_DONE,
            current=completed,
            total=total,
            metrics=_clean_metrics(metrics),
            message=f"Finished {fmt}" if fmt else "Finished a format",
            extra=_fmt_extra(fmt),
        )
    )


def _fmt_extra(fmt: Optional[str]) -> Dict[str, Any]:
    """Carry the format on every event: under concurrency it is the only thing that
    says which run an event belongs to."""
    return {"format": fmt} if fmt else {}


def _clean_metrics(metrics: Optional[Mapping[str, Any]]) -> Optional[Dict[str, Any]]:
    """Keep metrics JSON-serialisable and finite.

    Scores arrive as numpy scalars, and a NaN from a degenerate fold serialises to
    the literal `NaN`, which is not valid JSON and would make the whole progress file
    unreadable to a strict parser.
    """
    if not metrics:
        return None
    out: Dict[str, Any] = {}
    for key, value in metrics.items():
        try:
            number = float(value)
        except (TypeError, ValueError):
            out[str(key)] = str(value)
            continue
        if number != number or number in (float("inf"), float("-inf")):
            continue
        out[str(key)] = number
    return out or None


def _emit(event: Event) -> None:
    """Publish, swallowing anything that goes wrong. Telemetry never fails a run."""
    try:
        run_progress.emit(event)
    except Exception as e:  # pragma: no cover - run_progress.emit is already total
        logger.warning("training_progress.emit_failed step=%s error=%s", event.step, e)
