"""Run identity (H-16), the loader's refusal (D-6) and rating staleness (H-11).

These three are one subject. An artifact set is trusted because a manifest says which run
wrote it and what shape it is in; a set that cannot say so, or whose arrays are not the
arrays this code reads, is refused at load with an error naming the run; and a run that
loads fine can still be too old to answer a live request with.
"""

from __future__ import annotations

import importlib
import json
import os
from datetime import date, timedelta

import joblib
import numpy as np
import pytest
from fastapi.testclient import TestClient

from app import xi_service
from app.models.xi import XiOptimizeRequest
from ml.xi import contract as C
from ml.xi import runs
from ml.xi.ratings import RatingState
from ml.xi.runs import RunArtifactsInvalid
from ml.xi.store import STATE_ARRAY_NAMES, FormatModels, _state_to_payload, save_ratings, state_shape


class _ConstantModel:
    """Enough of a classifier for the store to load and answer with."""

    def predict_proba(self, rows):
        rows = np.atleast_2d(rows)
        return np.column_stack([np.full(len(rows), 0.4), np.full(len(rows), 0.6)])


def _state(players: int = 3, last_date: date | None = None) -> RatingState:
    state = RatingState()
    for i in range(players):
        state.players.slot(f"player{i}")
    state.matches_seen = players
    state.last_date = last_date or date(2026, 8, 30)
    return state


def _write_run(root, run_id: str = "20260902T101500Z-ab12cd34", *, last_date: date | None = None) -> str:
    """A complete, loadable run: ratings, one format's models, and a manifest."""
    directory = runs.run_dir(str(root), run_id)
    os.makedirs(directory, exist_ok=True)
    state = _state(last_date=last_date)
    save_ratings(state, directory)
    joblib.dump(
        FormatModels(
            format_code="T20",
            objective=_ConstantModel(),
            display=_ConstantModel(),
            # The contract's own lists: the store refuses a win artifact fitted on any
            # other columns (D-6), so a "complete run" has to carry the served shape.
            objective_cols=list(C.XI_FEATURE_COLS),
            display_cols=list(C.DISPLAY_FEATURE_COLS),
            metadata={},
        ),
        os.path.join(directory, "xi_win_T20.joblib"),
    )
    runs.write_manifest(
        directory,
        runs.RunManifest(
            run_id=run_id,
            created_at="2026-09-02T10:15:00+00:00",
            cutoff="2025-09-01",
            dataset_sha="abc123",
            git_sha="deadbee",
            state_shape=state_shape(state),
            formats=["T20"],
        ),
    )
    return directory


# --- H-16: a run names itself -------------------------------------------------------


def test_manifest_round_trips_with_the_shape_the_loader_checks(tmp_path):
    directory = _write_run(tmp_path)

    manifest = runs.read_manifest(directory)

    assert manifest.run_id == "20260902T101500Z-ab12cd34"
    assert manifest.state_shape["players"] == 3
    assert set(manifest.state_shape["arrays"]) == set(STATE_ARRAY_NAMES)


def test_run_ids_sort_newest_last_and_never_collide():
    first, second = runs.new_run_id(), runs.new_run_id()

    assert first != second
    assert len(first) == len("20260902T101500Z-ab12cd34")


def test_dataset_sha_is_the_matches_not_their_order():
    same = runs.dataset_sha(["b|2025-01-02", "a|2025-01-01"])

    assert runs.dataset_sha(["a|2025-01-01", "b|2025-01-02"]) == same
    assert runs.dataset_sha(["a|2025-01-01"]) != same


def test_list_runs_reports_a_directory_with_no_manifest_as_not_a_run(tmp_path):
    _write_run(tmp_path, "20260901T090000Z-11111111")
    os.makedirs(runs.run_dir(str(tmp_path), "20260902T090000Z-22222222"))

    listed = runs.list_runs(str(tmp_path))

    assert [entry["run_id"] for entry in listed] == [
        "20260902T090000Z-22222222",
        "20260901T090000Z-11111111",
    ], "newest first"
    assert listed[0]["has_manifest"] is False
    assert listed[1]["has_manifest"] is True
    assert runs.newest_run_id(str(tmp_path)) == "20260901T090000Z-11111111", "a non-run is never the newest run"


def test_current_is_a_pointer_and_refuses_a_run_that_is_not_one(tmp_path):
    _write_run(tmp_path)

    assert runs.read_current(str(tmp_path)) is None
    runs.set_current(str(tmp_path), "20260902T101500Z-ab12cd34")
    assert runs.read_current(str(tmp_path)) == "20260902T101500Z-ab12cd34"

    with pytest.raises(RunArtifactsInvalid, match="no manifest.json"):
        runs.set_current(str(tmp_path), "never-trained")


# --- D-6: a shape this code cannot serve is refused, by name ------------------------


def test_a_run_directory_with_no_manifest_is_refused(tmp_path):
    """The first half of D-6: an artifact was trusted because it loaded."""
    directory = _write_run(tmp_path)
    os.remove(runs.manifest_path(directory))

    with pytest.raises(RunArtifactsInvalid) as excinfo:
        xi_service.XiStore.load(directory)

    assert "manifest.json" in str(excinfo.value)


def test_a_pre_p2_rating_artifact_is_refused_naming_the_missing_arrays(tmp_path):
    """The D-6 artifact itself: written before P-2, so it carries none of the nine arrays
    P-2 and P-3 added. It used to load and then raise IndexError on the first request
    past slot 1024."""
    directory = _write_run(tmp_path)
    payload = _state_to_payload(_state())
    for name in ("bat_pos_sum", "bat_pos_n", "xi_n", "seq_num", "seq_den"):
        payload["arrays"].pop(name)
    joblib.dump(payload, os.path.join(directory, "xi_ratings.joblib"))

    with pytest.raises(RunArtifactsInvalid) as excinfo:
        xi_service.XiStore.load(directory)

    message = str(excinfo.value)
    assert "20260902T101500Z-ab12cd34" in message, "the refusal names the run"
    assert "bat_pos_sum" in message and "seq_den" in message


def test_an_array_narrower_than_the_players_it_names_is_refused(tmp_path):
    """The other half: the arrays are all present but one is the width an older, smaller
    state had -- the shape that produced `index 13433 is out of bounds for axis 1 with
    size 1024`."""
    directory = _write_run(tmp_path)
    payload = _state_to_payload(_state(players=5))
    payload["arrays"]["pelo"] = payload["arrays"]["pelo"][..., :2]
    joblib.dump(payload, os.path.join(directory, "xi_ratings.joblib"))

    with pytest.raises(RunArtifactsInvalid) as excinfo:
        xi_service.XiStore.load(directory)

    message = str(excinfo.value)
    assert "pelo has width 2, expected 5" in message


def test_a_win_artifact_fitted_on_other_columns_is_refused_naming_them(tmp_path):
    """D-6 for the models, not the ratings: B-7 took `t1_pelo_std` / `t2_pelo_std` out of
    the display columns, and an artifact still carrying them loads perfectly well -- the
    serving path builds its row from the artifact's own `display_cols` -- while answering
    with the incoherent surface the change was made to remove."""
    directory = _write_run(tmp_path)
    path = os.path.join(directory, "xi_win_T20.joblib")
    models = joblib.load(path)
    models.display_cols = list(C.DISPLAY_FEATURE_COLS) + ["t1_pelo_std", "t2_pelo_std"]
    joblib.dump(models, path)

    with pytest.raises(RunArtifactsInvalid) as excinfo:
        xi_service.XiStore.load(directory)

    message = str(excinfo.value)
    assert "20260902T101500Z-ab12cd34" in message, "the refusal names the run"
    assert "display_cols" in message and "t1_pelo_std" in message
    assert "Retrain." in message


def test_a_refused_run_is_reported_rather_than_silently_unloaded(tmp_path):
    """/xi/status has to say *why* nothing is loaded, or a refused artifact set reads
    exactly like a box that has never trained.

    The run is the one `current` names, which is the operational case: the artifacts a
    running service was pointed at stopped being loadable, and nothing else notices.
    """
    directory = _write_run(tmp_path)
    runs.set_current(str(tmp_path), "20260902T101500Z-ab12cd34")
    os.remove(runs.manifest_path(directory))
    registry = xi_service.XiRegistry()

    status = registry.reload(str(tmp_path))

    assert status["loaded"] is False
    assert "manifest.json" in status["error"]


def test_a_half_written_run_never_becomes_the_newest_run(tmp_path):
    """A retrain that died before writing its manifest must not take the service down
    with it: the last good run keeps serving, and the wreckage shows on /artifacts/status."""
    _write_run(tmp_path, "20260901T090000Z-11111111")
    os.makedirs(runs.run_dir(str(tmp_path), "20260902T090000Z-22222222"))
    registry = xi_service.XiRegistry()

    status = registry.reload(str(tmp_path))

    assert status["run_id"] == "20260901T090000Z-11111111"


def test_a_loadable_run_is_reported_with_its_manifest(tmp_path):
    _write_run(tmp_path)
    registry = xi_service.XiRegistry()

    status = registry.reload(str(tmp_path))

    assert status["loaded"] is True
    assert status["run_id"] == "20260902T101500Z-ab12cd34"
    assert status["manifest"]["dataset_sha"] == "abc123"
    assert status["manifest"]["git_sha"] == "deadbee"
    assert runs.read_current(str(tmp_path)) == "20260902T101500Z-ab12cd34", "loading a run publishes it"


def test_reload_can_swap_between_two_runs(tmp_path):
    _write_run(tmp_path, "20260901T090000Z-11111111")
    _write_run(tmp_path, "20260902T090000Z-22222222")
    registry = xi_service.XiRegistry()

    newest = registry.reload(str(tmp_path))
    older = registry.reload(str(tmp_path), "20260901T090000Z-11111111")

    assert newest["run_id"] == "20260902T090000Z-22222222", "with no pointer, the newest run"
    assert older["run_id"] == "20260901T090000Z-11111111"
    assert runs.read_current(str(tmp_path)) == "20260901T090000Z-11111111"


# --- H-11: ratings older than the limit refuse rather than answer --------------------


def test_fresh_ratings_pass_the_staleness_check(tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    _write_run(tmp_path, last_date=date.today() - timedelta(days=2))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    freshness = registry.freshness()

    assert freshness.fresh is True
    assert freshness.age_days == 2
    assert freshness.code is None


def test_stale_ratings_refuse_a_live_request_with_a_machine_readable_code(tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    _write_run(tmp_path, last_date=date.today() - timedelta(days=40))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    with pytest.raises(xi_service.RatingsStale) as excinfo:
        registry.store_as_of("T20", None)

    assert excinfo.value.payload["code"] == "RATINGS_STALE"
    assert "retrain" in excinfo.value.payload["hint"]
    assert registry.status().ratings.code == "RATINGS_STALE", "the verdict is on /xi/status too"


def test_a_backtest_naming_its_own_as_of_is_not_refused(tmp_path, monkeypatch):
    """H-11 is about live requests. A backtest names the date it wants served, so "how old
    is today's state?" is not a question about it -- refusing one would break the harness
    for a reason that does not describe it."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    _write_run(tmp_path, last_date=date.today() - timedelta(days=40))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    store = registry.store_as_of("T20", date.today())

    assert store is not None


def test_the_check_is_off_when_the_limit_is_zero(tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    _write_run(tmp_path, last_date=date(2020, 1, 1))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    assert registry.freshness().fresh is True


def test_optimize_refuses_a_live_request_against_stale_ratings(tmp_path, monkeypatch):
    """The refusal reaches the endpoint layer, not only the registry."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    _write_run(tmp_path, last_date=date.today() - timedelta(days=40))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    request = XiOptimizeRequest(format="T20", pool_player_ids=[f"player{i}" for i in range(3)], objective="ratings")
    with pytest.raises(xi_service.XiUnavailable) as excinfo:
        xi_service.optimize(request, registry)

    assert excinfo.value.payload["code"] == "RATINGS_STALE"


# --- the endpoints ------------------------------------------------------------------


@pytest.fixture
def client(tmp_path, monkeypatch):
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    monkeypatch.setenv("ENABLE_HOT_RELOAD", "1")
    monkeypatch.delenv("ADMIN_API_KEY", raising=False)
    module = importlib.reload(importlib.import_module("app.main"))
    return TestClient(module.app)


def test_artifacts_status_lists_the_runs_and_says_which_is_loaded(client, tmp_path):
    _write_run(tmp_path)
    client.post("/admin/reload")

    data = client.get("/artifacts/status").json()

    assert data["current_run"] == "20260902T101500Z-ab12cd34"
    assert data["loaded_run"] == "20260902T101500Z-ab12cd34"
    assert data["runs"][0]["loaded"] is True


def test_reload_of_a_refused_run_answers_409_with_the_reason(client, tmp_path):
    directory = _write_run(tmp_path)
    os.remove(runs.manifest_path(directory))

    resp = client.post("/admin/reload?run=20260902T101500Z-ab12cd34")

    assert resp.status_code == 409
    assert resp.json()["detail"]["code"] == "RUN_ARTIFACTS_INVALID"


def test_health_reports_the_run_and_the_freshness_verdict(client, tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    _write_run(tmp_path, last_date=date.today() - timedelta(days=40))
    client.post("/admin/reload")

    data = client.get("/health").json()

    assert data["status"] == "ok", "an unservable model is not a dead process"
    assert data["run_id"] == "20260902T101500Z-ab12cd34"
    assert data["ratings"]["code"] == "RATINGS_STALE"


def test_reload_with_no_run_publishes_the_newest_one(client, tmp_path):
    """The pipeline's publish step. `retrain` writes a run and publishes nothing, so the
    reload after it — which names no run, because neither step passes an id to the other
    — has to reach the run just built.

    It did not. `current` is set by every reload, and the endpoint consulted it first, so
    on any box that had ever reloaded, this call re-loaded the run already serving. It
    succeeded, the run plan completed, and `/xi/status` went on reporting the older run's
    `ratings_through` — the whole chain green while nothing was published. Two runs on the
    box A-5 was written on were never served that way.
    """
    _write_run(tmp_path, "20260901T090000Z-11111111")
    client.post("/admin/reload?run=20260901T090000Z-11111111")

    _write_run(tmp_path, "20260903T154222Z-4e009a52")
    resp = client.post("/admin/reload")

    assert resp.status_code == 200
    assert resp.json()["run_id"] == "20260903T154222Z-4e009a52"
    assert runs.read_current(str(tmp_path)) == "20260903T154222Z-4e009a52"


def test_reload_of_a_named_run_still_rolls_back(client, tmp_path):
    """Rolling back is naming a run, and stays that way: publishing the newest by default
    must not take away the only way to serve an earlier one."""
    _write_run(tmp_path, "20260901T090000Z-11111111")
    _write_run(tmp_path, "20260903T154222Z-4e009a52")
    client.post("/admin/reload")

    resp = client.post("/admin/reload?run=20260901T090000Z-11111111")

    assert resp.json()["run_id"] == "20260901T090000Z-11111111"
    assert runs.read_current(str(tmp_path)) == "20260901T090000Z-11111111"


def test_a_restart_serves_what_was_published_not_the_newest_run(tmp_path):
    """The startup path is deliberately the other rule. It calls the registry directly,
    where `current` wins, so a deliberate rollback survives a restart — the newest run on
    disk may be exactly the one an operator rolled away from."""
    _write_run(tmp_path, "20260901T090000Z-11111111")
    _write_run(tmp_path, "20260903T154222Z-4e009a52")
    runs.set_current(str(tmp_path), "20260901T090000Z-11111111")

    status = xi_service.XiRegistry().reload(str(tmp_path))

    assert status["run_id"] == "20260901T090000Z-11111111"


def test_reload_with_no_run_and_nothing_on_disk_says_so(client):
    """An empty box still answers rather than failing on the newest run being None."""
    resp = client.post("/admin/reload")

    assert resp.status_code == 200
    assert resp.json()["loaded"] is False


def test_retrain_requires_a_cutoff(client):
    resp = client.post("/admin/train/retrain")

    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "CUTOFF_REQUIRED"


def test_manifest_written_by_retrain_is_the_one_the_loader_reads(tmp_path):
    """The write and the read are the same contract; a manifest this code cannot read
    back would move D-6 rather than close it."""
    directory = _write_run(tmp_path)

    with open(runs.manifest_path(directory)) as fh:
        raw = json.load(fh)

    assert set(raw) == set(runs.RunManifest.__dataclass_fields__)
