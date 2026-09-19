"""SERVE-01: what one slow request does to everything else the process is doing.

Every route used to be ``async def`` around synchronous work -- an optimise of up to 200k
model evaluations, a simulate of 20k draws, an as-of sweep of ``ball_event``, a reload of a
directory of joblib files -- so the request that asked for it owned the event loop until it
was done and nothing else was served, ``/health`` included. These tests hold one handler
inside its own work and ask ``/health`` how long it waited.

The client is driven from a second thread on purpose: ``TestClient`` used as a context
manager keeps one event loop for every request through it, which is the arrangement the
service really runs in. Without the ``with`` block each request would get a loop of its own
and a blocked loop would prove nothing.
"""

from __future__ import annotations

import importlib
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from typing import Any, Dict, List

import pytest
from fastapi.testclient import TestClient

from app.xi_service import XiUnavailable
from tests.xi_fixtures import xi

#: How long the stand-in work holds the thread it is running on. Long enough that a blocked
#: loop is unmistakable beside the health budget, short enough that a run against a service
#: that does block still finishes.
BLOCKING_SECONDS = 3.0

#: What ``/health`` may take while that work is in flight. It reads two attributes off the
#: registry, so the honest figure is a millisecond; a second is the generous version of "it
#: did not wait for the prediction".
HEALTH_BUDGET_SECONDS = 1.0

_ELEVENS: Dict[str, Any] = {"format": "T20", "team1_player_ids": xi("a"), "team2_player_ids": xi("b")}

#: One case per route the finding names: where it posts, what it posts, which callable to
#: hold inside, and what the caller is answered once it is let go.
BLOCKING_ROUTES: List[tuple] = [
    ("/xi/predict-win", _ELEVENS, "service", "predict_win", 503),
    ("/performance/predict", _ELEVENS, "service", "predict_performance", 503),
    ("/simulate", _ELEVENS, "service", "simulate", 503),
    ("/xi/player-roles", {"format": "T20", "player_ids": xi("a")}, "service", "player_roles", 503),
    (
        "/xi/optimize",
        {"format": "T20", "pool_player_ids": xi("a"), "opponent_player_ids": xi("b"), "objective": "win"},
        "service",
        "optimize",
        503,
    ),
    ("/admin/reload", None, "app", "_reload_run", 500),
]


def _app_client(tmp_path, monkeypatch) -> tuple:
    """A freshly imported app over an empty artifacts root, with the admin routes open."""
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    monkeypatch.setenv("ENABLE_HOT_RELOAD", "1")
    monkeypatch.setenv("ADMIN_API_KEY", "")
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    return app_module, TestClient(app_module.app)


def _held_call(entered: threading.Event, release: threading.Event):
    """A stand-in for the route's real work: it announces that it is running, holds its
    thread the way a real optimise or sweep does, then refuses.

    Refusing rather than returning keeps the case to one moving part -- the route is
    reached, it computes, the caller is answered -- with no artifacts to build. The two
    statuses in the table are the two the refusal already maps to: a prediction route
    turns ``XiUnavailable`` into 503, and ``/admin/reload`` reports a load that failed."""

    def call(*args: Any, **kwargs: Any) -> Any:
        entered.set()
        release.wait(BLOCKING_SECONDS)
        raise XiUnavailable("stand-in for work that takes the thread")

    return call


@pytest.mark.parametrize("path,payload,holder,attribute,status", BLOCKING_ROUTES)
def test_health_answers_while_a_route_is_computing(tmp_path, monkeypatch, path, payload, holder, attribute, status):
    """The event loop is free while a handler computes, so liveness is still answerable."""
    app_module, client = _app_client(tmp_path, monkeypatch)
    entered, release = threading.Event(), threading.Event()
    monkeypatch.setattr(
        {"app": app_module, "service": app_module.xi_service}[holder], attribute, _held_call(entered, release)
    )

    with client, ThreadPoolExecutor(max_workers=1) as pool:
        in_flight = pool.submit(client.post, path, json=payload)
        assert entered.wait(BLOCKING_SECONDS), f"{path} never reached its handler"
        started = time.perf_counter()
        health = client.get("/health")
        waited = time.perf_counter() - started
        release.set()
        answered = in_flight.result()

    assert health.status_code == 200
    assert waited < HEALTH_BUDGET_SECONDS, (
        f"/health waited {waited:.2f}s for {path}, which was holding its thread for "
        f"{BLOCKING_SECONDS:.0f}s -- the handler is running on the event loop"
    )
    assert answered.status_code == status, "and the request that was held still gets its own answer"
