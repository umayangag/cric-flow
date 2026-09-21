"""A stop that stops (D-11).

`POST /ops/pipeline/stop` used to answer `{"cancelled": 1}` while `ml.xi.retrain` carried
on inside this container, still burning CPU, until someone killed it by hand. go-app's
cancellation closed its own HTTP request; nothing here held a handle to the process, so
there was nothing for it to reach. Two things were wrong -- the console reported a
cancellation that did not happen, and the compute lane read as free while a retrain was
still writing -- and these tests hold both of them shut.
"""

from __future__ import annotations

import asyncio
import importlib
import os
import subprocess
import sys
import threading
import time
from typing import Any, Dict, List, Optional

import pytest
from fastapi.testclient import TestClient

from app import training_orchestrator

# A stand-in for a training run: a process that spawns a worker (as the model fits do) and
# then sits there. Both have to die -- orphaned workers were half of what D-11 left behind.
_PARENT_WITH_WORKER = (
    "import subprocess,sys,time;"
    "w=subprocess.Popen([sys.executable,'-c','import time;time.sleep(120)']);"
    "print(w.pid,flush=True);"
    "time.sleep(120)"
)


def _alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except OSError:
        return False
    return True


@pytest.fixture
def fake_training_process():
    """Register a live process as the retrain step's run, and clean it up either way."""
    started: List[tuple[subprocess.Popen, training_orchestrator.LiveRun]] = []

    def start() -> tuple[subprocess.Popen, int, training_orchestrator.LiveRun]:
        proc = subprocess.Popen(
            [sys.executable, "-c", _PARENT_WITH_WORKER],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            start_new_session=True,
        )
        worker_pid = int(proc.stdout.readline().strip())
        run = training_orchestrator._processes.start(training_orchestrator.TRAINING_MODULES["retrain"], lambda: proc)
        started.append((proc, run))
        return proc, worker_pid, run

    yield start

    for proc, run in started:
        training_orchestrator._processes.finish(run)
        if proc.poll() is None:
            proc.kill()
            proc.wait()


def test_stop_training_kills_the_process_and_its_workers(fake_training_process) -> None:
    """The claim a stop makes is about a process, so the process has to be gone."""
    proc, worker_pid, _ = fake_training_process()
    assert _alive(proc.pid) and _alive(worker_pid)

    stopped = training_orchestrator.stop_training("retrain")

    assert stopped == ["retrain"]
    assert proc.returncode is not None, "stop_training returned before the process was gone"
    time.sleep(0.3)
    assert not _alive(worker_pid), "the worker outlived its parent, which is what kept burning CPU"


def test_stop_training_reports_nothing_when_nothing_runs() -> None:
    """A Stop pressed on an idle pipeline is a normal event, not an error."""
    assert training_orchestrator.stop_training() == []
    assert training_orchestrator.stop_training("retrain") == []


def test_stop_training_refuses_a_step_it_does_not_run() -> None:
    with pytest.raises(ValueError, match="unknown training step"):
        training_orchestrator.stop_training("precompute")


def test_a_stopped_run_is_reported_as_stopped_not_as_failed(fake_training_process) -> None:
    """`Training failed (exit -15)` would send a reader looking for a bug that is not there."""
    proc, _, run = fake_training_process()

    training_orchestrator.stop_training("retrain")

    assert run.stopped
    assert proc.returncode != 0, "the process really did exit non-zero; the wording is the point"


def test_training_stopped_is_still_a_value_error() -> None:
    """Every existing caller handles ValueError, so the distinction is additive: the ones
    that care can tell a Stop from a break, and the ones that do not are unaffected."""
    assert issubclass(training_orchestrator.TrainingStopped, ValueError)


# --- the endpoint go-app now asks, instead of assuming ------------------------------


@pytest.fixture
def client(tmp_path, monkeypatch) -> TestClient:
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    monkeypatch.setenv("ENABLE_HOT_RELOAD", "1")
    monkeypatch.delenv("ADMIN_API_KEY", raising=False)
    module = importlib.reload(importlib.import_module("app.main"))
    return TestClient(module.app)


def test_stop_endpoint_reports_the_step_it_stopped(client, fake_training_process) -> None:
    proc, _, _ = fake_training_process()

    resp = client.post("/admin/train/stop")

    assert resp.status_code == 200, resp.text
    assert resp.json()["stopped"] == ["retrain"]
    assert proc.returncode is not None


def test_stop_endpoint_answers_200_with_nothing_running(client) -> None:
    resp = client.post("/admin/train/stop")

    assert resp.status_code == 200
    assert resp.json()["stopped"] == [], "a Stop pressed twice is not an error"


def test_stop_endpoint_refuses_an_unknown_step(client) -> None:
    resp = client.post("/admin/train/stop?step=precompute")

    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "UNKNOWN_STEP"


def test_a_stopped_retrain_answers_409_not_500(client, monkeypatch) -> None:
    """A Stop is an operator doing their job, so the run it ends is not a failure.

    Answering 500 TRAIN_FAILED (and logging a stack trace) told everything downstream that
    something broke, which is the same untruth as D-11 wearing a different hat.
    """

    def stopped_run(*_args: Any, **_kwargs: Any) -> None:
        raise training_orchestrator.TrainingStopped("Training stopped on request")

    monkeypatch.setattr(training_orchestrator, "run_retrain", stopped_run)

    resp = client.post("/admin/train/retrain?cutoff=2025-09-01")

    assert resp.status_code == 409, resp.text
    assert resp.json()["detail"]["code"] == "TRAIN_STOPPED"


def test_a_genuinely_broken_retrain_still_answers_500(client, monkeypatch) -> None:
    """The distinction only means something if the other side of it still works."""

    def broken_run(*_args: Any, **_kwargs: Any) -> None:
        raise ValueError("Training failed (exit 1)")

    monkeypatch.setattr(training_orchestrator, "run_retrain", broken_run)

    resp = client.post("/admin/train/retrain?cutoff=2025-09-01")

    assert resp.status_code == 500
    assert resp.json()["detail"]["code"] == "TRAIN_FAILED"


def test_stop_endpoint_is_declared_on_the_boundary_contract(client) -> None:
    """H-24: go-app posts here and reads `stopped` out of the answer, so both the route
    and the field it reads are in contracts/ops-console.contract.json."""
    import json
    from pathlib import Path

    contract_path = Path(__file__).resolve().parents[2] / "contracts" / "ops-console.contract.json"
    with open(contract_path) as fh:
        contract = json.load(fh)

    stop_calls = [c for c in contract["ml_service_calls"] if c["path"] == "/admin/train/stop"]
    assert stop_calls, "go-app's stop call is not declared"
    assert stop_calls[0]["method"] == "POST"

    body = client.post("/admin/train/stop").json()
    assert contract["stop_response_field"] in body, "go-app reads a field this service does not send"


# --- the step slot is held until the process is gone --------------------------------


def test_the_step_slot_is_held_until_the_subprocess_is_gone(monkeypatch) -> None:
    """The second half of D-11, and the subtler one.

    `asyncio.to_thread` gives the event loop a future it can cancel, but cancelling it
    neither stops the thread nor touches the subprocess the thread is waiting on. Before
    the fix, go-app dropping its request let the request coroutine return at once: the
    lane read as free while a retrain was still writing, so a second retrain started then
    would have run beside the first. The coroutine must not finish before the training
    thread does -- that thread is what releases the step's slot, and the slot is what
    refuses a second retrain (SERVE-06).
    """
    main = importlib.reload(importlib.import_module("app.main"))
    released: Dict[str, Any] = {}
    # A threading.Event, not an asyncio one: it is set from the worker thread and waited
    # on from another thread, so the event loop is not the thing being signalled.
    running = threading.Event()

    def slow_step(*_args: Any) -> None:
        """Stands for a training thread that keeps going after its future is cancelled."""
        running.set()
        time.sleep(0.4)
        released["thread_finished"] = time.monotonic()

    stopped_steps: List[str] = []

    def record_stop(step: str, _logger: Optional[Any] = None) -> List[str]:
        stopped_steps.append(step)
        return [step]

    # Through `monkeypatch`, because this replaces a function on the real module object:
    # left in place it would answer every later test's stop with "I stopped it".
    monkeypatch.setattr(main.training_orchestrator, "stop_training", record_stop)

    async def scenario() -> None:
        task = asyncio.create_task(main._run_training_step("retrain", slow_step))
        assert await asyncio.to_thread(running.wait, 5.0), "the training thread never started"

        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
        released["request_returned_at"] = time.monotonic()

    asyncio.run(scenario())

    assert stopped_steps == ["retrain"], "cancelling the request must reach the subprocess"
    assert "thread_finished" in released, "the request returned before the thread finished"
    assert released["request_returned_at"] >= released["thread_finished"], (
        "the lane came free while the training thread was still running"
    )


# --- one run per step, and a stop that does not lie about one (SERVE-06) -------------

# A stand-in for a training module: real, in the standard library, and over in
# milliseconds. Every test below drives the orchestrator's own code path -- a retrain is
# ten minutes and writes artifacts, so it is the module that is substituted, not the
# machinery being tested.
_INSTANT_MODULE = "timeit"
_INSTANT_ARGS = ["-n", "1", "-r", "1", "pass"]


class _RecordingLogger:
    """Enough of the service logger to read back what a stop said it did."""

    def __init__(self) -> None:
        self.infos: List[tuple[str, Dict[str, Any]]] = []

    def info(self, event: str, **fields: Any) -> None:
        self.infos.append((event, fields))

    def warning(self, event: str, **fields: Any) -> None:  # pragma: no cover - not this path
        self.infos.append((event, fields))


@pytest.fixture
def instant_training_module(monkeypatch):
    """Point the retrain step at a module that exits at once, and hand back its name."""
    monkeypatch.setitem(training_orchestrator.TRAINING_MODULES, "retrain", _INSTANT_MODULE)
    return _INSTANT_MODULE


def test_a_second_run_of_a_step_is_refused_and_names_the_one_already_running(fake_training_process) -> None:
    """Two retrains do not fit: they write the same cross-run data-quality baseline and
    publish progress under the same step, so the second is refused rather than started.

    Keyed by module, the registry took the second handle over the first: the first run
    became unaddressable by `stop_training` and whichever finished first deregistered the
    other. Refusing the second is what makes the slot mean something.
    """
    proc, _, _ = fake_training_process()
    module = training_orchestrator.TRAINING_MODULES["retrain"]

    with pytest.raises(training_orchestrator.TrainingAlreadyRunning) as refusal:
        training_orchestrator._processes.start(module, lambda: pytest.fail("a second process was spawned"))

    assert refusal.value.step == "retrain"
    assert refusal.value.pid == proc.pid, "the refusal names the run that is running"
    assert "retrain is already running" in str(refusal.value)
    assert training_orchestrator.stop_training("retrain") == ["retrain"], "the first run is still addressable"


def test_the_retrain_endpoint_refuses_a_second_run_by_naming_the_first(client, fake_training_process) -> None:
    """What the refusal says on the wire: 409, a code, the running run, and what to do.

    §8.7 -- a status code on its own leaves the operator to go and find out whether
    anything is running at all, which is the question this service is the answer to.
    """
    proc, _, _ = fake_training_process()

    resp = client.post("/admin/train/retrain?cutoff=2025-09-01")

    assert resp.status_code == 409, resp.text
    detail = resp.json()["detail"]
    assert detail["code"] == "TRAIN_ALREADY_RUNNING"
    assert f"pid {proc.pid}" in detail["message"]
    assert "retrain is already running" in detail["message"]
    assert "/admin/train/stop?step=retrain" in detail["hint"]
    assert proc.poll() is None, "refusing the second run must not disturb the first"


def test_the_step_slot_is_free_again_once_the_run_is_over(instant_training_module) -> None:
    """A refusal that outlived its run would be worse than the defect it replaced."""
    training_orchestrator.run_training_subprocess(instant_training_module, _INSTANT_ARGS)

    training_orchestrator.run_training_subprocess(instant_training_module, _INSTANT_ARGS)


def test_two_different_steps_may_run_at_once(fake_training_process) -> None:
    """The rule is per step, not global: `retrain` and `evaluate` share no output file,
    so `evaluate` is not refused because a retrain is running."""
    fake_training_process()
    evaluate_module = training_orchestrator.TRAINING_MODULES["evaluate"]
    finished = subprocess.Popen([sys.executable, "-c", "raise SystemExit(0)"], start_new_session=True)
    finished.wait()

    run = training_orchestrator._processes.start(evaluate_module, lambda: finished)

    assert run.step == "evaluate"
    training_orchestrator._processes.finish(run)


def test_a_stop_that_arrives_after_the_run_finished_stops_nothing() -> None:
    """The stop/exit race, pinned on an ordering that cannot go the other way.

    The child is waited on *before* the stop is asked for, so there is no timing to get
    lucky with: at the moment of the stop the process is already gone and only its slot
    remains, which is the window the worker thread closes when `communicate` returns.
    Marking the run stopped there turned a retrain that succeeded into `TrainingStopped`
    and a 409 on the wire.
    """
    finished = subprocess.Popen([sys.executable, "-c", "raise SystemExit(0)"], start_new_session=True)
    finished.wait()
    run = training_orchestrator._processes.start(training_orchestrator.TRAINING_MODULES["retrain"], lambda: finished)
    logger = _RecordingLogger()

    try:
        stopped = training_orchestrator.stop_training("retrain", logger)
    finally:
        training_orchestrator._processes.finish(run)

    assert stopped == [], "there was nothing to stop: the run had already finished"
    assert not run.stopped, "a run that finished on its own keeps the outcome it earned"
    assert any("nothing to stop" in event for event, _ in logger.infos), (
        "the operator's log has to say a stop found the run already finished, not go quiet"
    )


def test_a_run_that_finished_before_the_stop_is_not_reported_as_stopped(instant_training_module, monkeypatch) -> None:
    """The same race through the function an operator's request actually runs.

    The worker thread is held inside the window -- after `communicate` has returned and
    before the slot is released -- by a `finish` that waits for the test, so the stop
    lands in the window every time rather than sometimes. Before the fix this run raised
    `TrainingStopped` and the endpoint answered 409 for a run that had already succeeded.
    """
    in_the_window = threading.Event()
    stop_done = threading.Event()
    real_finish = training_orchestrator._processes.finish

    def finish_after_the_stop(run: training_orchestrator.LiveRun) -> None:
        in_the_window.set()
        assert stop_done.wait(30.0), "the test never got to its stop"
        real_finish(run)

    monkeypatch.setattr(training_orchestrator._processes, "finish", finish_after_the_stop)
    outcome: Dict[str, Any] = {}

    def run_it() -> None:
        try:
            training_orchestrator.run_training_subprocess(instant_training_module, _INSTANT_ARGS)
            outcome["raised"] = None
        except BaseException as e:  # noqa: BLE001 - the test is about which exception, if any
            outcome["raised"] = e

    worker = threading.Thread(target=run_it)
    worker.start()
    assert in_the_window.wait(30.0), "the subprocess never reached the end of its run"
    stopped = training_orchestrator.stop_training("retrain")
    stop_done.set()
    worker.join(30.0)

    assert stopped == [], "the run had already exited, so the stop stopped nothing"
    assert outcome["raised"] is None, "a run that succeeded was reported as stopped"
