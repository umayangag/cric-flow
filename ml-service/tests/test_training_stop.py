"""A stop that stops (D-10).

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
# then sits there. Both have to die -- orphaned workers were half of what D-10 left behind.
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
    """Register a live process under the retrain module, and clean it up either way."""
    started: List[subprocess.Popen] = []

    def start() -> tuple[subprocess.Popen, int]:
        proc = subprocess.Popen(
            [sys.executable, "-c", _PARENT_WITH_WORKER],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            start_new_session=True,
        )
        started.append(proc)
        worker_pid = int(proc.stdout.readline().strip())
        training_orchestrator._processes.register(training_orchestrator.TRAINING_MODULES["retrain"], proc)
        return proc, worker_pid

    yield start

    for proc in started:
        training_orchestrator._processes.unregister(training_orchestrator.TRAINING_MODULES["retrain"])
        if proc.poll() is None:
            proc.kill()
            proc.wait()


def test_stop_training_kills_the_process_and_its_workers(fake_training_process) -> None:
    """The claim a stop makes is about a process, so the process has to be gone."""
    proc, worker_pid = fake_training_process()
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
    proc, _ = fake_training_process()
    module = training_orchestrator.TRAINING_MODULES["retrain"]

    training_orchestrator.stop_training("retrain")

    assert training_orchestrator._processes.was_stopped(module)
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
    proc, _ = fake_training_process()

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
    something broke, which is the same untruth as D-10 wearing a different hat.
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


# --- the compute slot is held until the process is gone -----------------------------


def test_the_compute_slot_is_held_until_the_subprocess_is_gone() -> None:
    """The second half of D-10, and the subtler one.

    `asyncio.to_thread` gives the event loop a future it can cancel, but cancelling it
    neither stops the thread nor touches the subprocess the thread is waiting on. Before
    the fix, go-app dropping its request released the training semaphore immediately: the
    compute lane read as free while a retrain was still writing, so a second retrain
    started then would have run beside the first.
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

    async def scenario() -> None:
        main.training_orchestrator.stop_training = record_stop  # type: ignore[assignment]
        task = asyncio.create_task(main._run_training_step("retrain", slow_step))
        assert await asyncio.to_thread(running.wait, 5.0), "the training thread never started"
        semaphore = main._get_training_semaphore()
        assert semaphore.locked() or semaphore._value < main.MAX_CONCURRENT_TRAINING_JOBS

        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
        released["semaphore_free_at"] = time.monotonic()

    asyncio.run(scenario())

    assert stopped_steps == ["retrain"], "cancelling the request must reach the subprocess"
    assert "thread_finished" in released, "the slot was released before the thread finished"
    assert released["semaphore_free_at"] >= released["thread_finished"], (
        "the compute slot came free while the training thread was still running"
    )
