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
import threading
from datetime import date, timedelta

import joblib
import numpy as np
import pytest
from fastapi.testclient import TestClient

from app import xi_service
from app.models.xi import XiConstraints, XiOptimizeRequest, XiWinRequest
from ml.xi import contract as C
from ml.xi import runs
from ml.xi.ratings import RatingState
from ml.xi.runs import RunArtifactsInvalid
from ml.xi.store import STATE_ARRAY_NAMES, FormatModels, _state_to_payload, save_ratings, state_shape
from tests.xi_fixtures import ListSource


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


def _write_run(
    root,
    run_id: str = "20260902T101500Z-ab12cd34",
    *,
    last_date: date | None = None,
    manifest_ratings_through: str | None = None,
    unusable_reasons: dict[str, str] | None = None,
) -> str:
    """A complete, loadable run: ratings, one format's models, and a manifest.

    ``manifest_ratings_through`` overrides the date the manifest records; by default it
    is the state's own, which is what ``retrain`` writes. ``unusable_reasons`` writes the
    run as one its retrain judged not usable (EVAL-04)."""
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
            ratings_through=manifest_ratings_through or state.last_date.isoformat(),
            dataset_sha="abc123",
            git_sha="deadbee",
            state_shape=state_shape(state),
            formats=["T20"],
            usable=not unusable_reasons,
            unusable_reasons=dict(unusable_reasons or {}),
        ),
    )
    return directory


def _rewrite_manifest_without(directory: str, key: str) -> None:
    """A manifest written before a field existed: the same file minus that key."""
    path = runs.manifest_path(directory)
    with open(path) as fh:
        raw = json.load(fh)
    del raw[key]
    with open(path, "w") as fh:
        json.dump(raw, fh)


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


# --- EVAL-04: a run its retrain judged not usable is never published ----------------

_DOES_NOT_RANK = {"T20": "objective holdout AUC 0.4800 is not above the base rate's 0.5 on 400 holdout rows"}


def test_current_refuses_a_run_that_is_not_usable_naming_why(tmp_path):
    """Publishing is the pointer, so the refusal lives where the pointer is set."""
    _write_run(tmp_path, unusable_reasons=_DOES_NOT_RANK)

    with pytest.raises(RunArtifactsInvalid, match="not usable and cannot be published -- T20: objective holdout AUC"):
        runs.set_current(str(tmp_path), "20260902T101500Z-ab12cd34")
    assert runs.read_current(str(tmp_path)) is None


def test_the_newest_publishable_run_skips_one_that_is_not_usable(tmp_path):
    """A reload with no run named asks for the newest run that can be served; the newest
    directory on disk is listed, with its reasons, and passed over."""
    _write_run(tmp_path, "20260901T090000Z-11111111")
    _write_run(tmp_path, "20260903T154222Z-4e009a52", unusable_reasons=_DOES_NOT_RANK)

    listed = {entry["run_id"]: entry for entry in runs.list_runs(str(tmp_path))}

    assert runs.newest_run_id(str(tmp_path)) == "20260901T090000Z-11111111"
    assert listed["20260903T154222Z-4e009a52"]["usable"] is False
    assert listed["20260903T154222Z-4e009a52"]["unusable_reasons"] == _DOES_NOT_RANK
    assert listed["20260903T154222Z-4e009a52"]["refused"] is None, "it loads; it is publication that is refused"
    assert listed["20260901T090000Z-11111111"]["usable"] is True


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


# --- P2-2: the manifest says what date its data runs through ------------------------


def test_a_manifest_without_ratings_through_is_refused_naming_the_run(tmp_path):
    """A run written before the field existed is not patched or served with a date read
    off its joblib (§8.7): the manifest reader refuses it, naming the run and the field."""
    directory = _write_run(tmp_path)
    _rewrite_manifest_without(directory, "ratings_through")

    with pytest.raises(RunArtifactsInvalid) as excinfo:
        xi_service.XiStore.load(directory)

    message = str(excinfo.value)
    assert "20260902T101500Z-ab12cd34" in message, "the refusal names the run"
    assert "ratings_through" in message
    assert "make retrain" in message


def test_a_manifest_that_disagrees_with_its_state_is_refused_naming_both_dates(tmp_path):
    """The manifest's date and the state's ``last_date`` were written by one retrain; a
    difference means the directory is not the run its manifest describes."""
    directory = _write_run(tmp_path, last_date=date(2026, 8, 30), manifest_ratings_through="2026-09-01")

    with pytest.raises(RunArtifactsInvalid) as excinfo:
        xi_service.XiStore.load(directory)

    message = str(excinfo.value)
    assert "20260902T101500Z-ab12cd34" in message, "the refusal names the run"
    assert "2026-09-01" in message and "2026-08-30" in message, "and both dates"
    assert "Retrain." in message


def test_a_loaded_run_status_stamp_and_manifest_carry_one_date(tmp_path, monkeypatch):
    """Three readings of the date -- the status (off the state), the served stamp (off
    the state) and the manifest summary (off the file) -- are the same date, because the
    loader asserted it before any of them could be read."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    _write_run(tmp_path, last_date=date(2026, 8, 30))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    status = registry.status()
    win = xi_service.predict_win(_live_win_request(), registry)

    assert status.ratings_through == "2026-08-30"
    assert status.manifest["ratings_through"] == status.ratings_through
    assert win.served_ratings.ratings_through == status.ratings_through
    assert status.manifest["cutoff"] == "2025-09-01", "the cutoff is a different date, and stays one"


def test_the_listing_carries_the_date_per_run_and_the_reason_a_run_cannot_load(tmp_path):
    """ "What date is this run's data?" is answered off the listing for every run on disk,
    and a run that predates the field is listed with the reason, not a blank (§8.7)."""
    _write_run(tmp_path, "20260901T090000Z-11111111", last_date=date(2026, 8, 29))
    older = _write_run(tmp_path, "20260831T090000Z-00000000", last_date=date(2026, 8, 28))
    _rewrite_manifest_without(older, "ratings_through")

    listed = {entry["run_id"]: entry for entry in runs.list_runs(str(tmp_path))}

    assert listed["20260901T090000Z-11111111"]["ratings_through"] == "2026-08-29"
    assert listed["20260901T090000Z-11111111"]["refused"] is None
    assert listed["20260831T090000Z-00000000"]["has_manifest"] is True
    assert "ratings_through" in listed["20260831T090000Z-00000000"]["refused"]
    assert "ratings_through" not in listed["20260831T090000Z-00000000"], "no date is read from anywhere else"
    assert runs.newest_run_id(str(tmp_path)) == "20260901T090000Z-11111111"


def test_a_run_that_predates_the_field_is_never_the_newest_run(tmp_path):
    """A reload with no run named asks for the newest run that can be served; a manifest
    the reader refuses is skipped rather than turned into a refusal nobody asked for."""
    _write_run(tmp_path, "20260901T090000Z-11111111")
    newer = _write_run(tmp_path, "20260902T090000Z-22222222")
    _rewrite_manifest_without(newer, "ratings_through")

    assert runs.newest_run_id(str(tmp_path)) == "20260901T090000Z-11111111"


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


# --- B-13: a refused reload leaves the run that was serving --------------------------

SERVING_RUN = "20260901T090000Z-11111111"
REFUSED_RUN = "20260903T154222Z-4e009a52"


def _registry_serving(tmp_path) -> xi_service.XiRegistry:
    """A registry serving ``SERVING_RUN``, ratings through 2026-08-28."""
    _write_run(tmp_path, SERVING_RUN, last_date=date(2026, 8, 28))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path), SERVING_RUN)
    return registry


def _write_refused_run(tmp_path) -> str:
    """``REFUSED_RUN`` on disk in a shape the loader refuses: no ``ratings_through``."""
    directory = _write_run(tmp_path, REFUSED_RUN, last_date=date(2026, 8, 30))
    _rewrite_manifest_without(directory, "ratings_through")
    return directory


def test_a_refused_reload_leaves_the_run_that_was_serving_unchanged(tmp_path, monkeypatch):
    """Naming an unloadable run is answered with a refusal, not an outage (B-13): the
    registry still serves the run it had -- same id, same date -- keeps answering
    predictions from it, reports the refusal beside it, and publishes nothing."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    registry = _registry_serving(tmp_path)
    before = registry.status().model_dump()
    _write_refused_run(tmp_path)

    status = registry.reload(str(tmp_path), REFUSED_RUN)

    assert status["loaded"] is True
    assert status["run_id"] == before["run_id"] == SERVING_RUN
    assert status["ratings_through"] == before["ratings_through"] == "2026-08-28"
    assert REFUSED_RUN in status["error"]
    assert "ratings_through" in status["error"]
    assert registry.store("T20").manifest.run_id == SERVING_RUN
    assert runs.read_current(str(tmp_path)) == SERVING_RUN, "a refused run is not published"


def test_a_load_that_raises_anything_else_also_leaves_the_served_run(tmp_path, monkeypatch):
    """The generic failure path -- a corrupt joblib, a report that is not JSON -- is the
    same shape: nothing is swapped in, so nothing is taken away."""
    registry = _registry_serving(tmp_path)
    _write_run(tmp_path, REFUSED_RUN)

    def corrupt_load(directory):
        raise RuntimeError("boom")

    monkeypatch.setattr(xi_service.XiStore, "load", corrupt_load)

    status = registry.reload(str(tmp_path), REFUSED_RUN)

    assert status["run_id"] == SERVING_RUN
    assert status["error"] == f"run {REFUSED_RUN}: boom"


def test_the_next_successful_reload_clears_the_refusal(tmp_path):
    """``error`` describes the last reload, so a reload that succeeds after one that was
    refused reports no refusal -- the two would otherwise read as one run's status."""
    registry = _registry_serving(tmp_path)
    _write_refused_run(tmp_path)
    registry.reload(str(tmp_path), REFUSED_RUN)
    _write_run(tmp_path, "20260904T090000Z-22222222")

    status = registry.reload(str(tmp_path), "20260904T090000Z-22222222")

    assert status["run_id"] == "20260904T090000Z-22222222"
    assert status["error"] is None


def test_a_reader_arriving_mid_reload_is_answered_by_the_run_that_was_serving(tmp_path):
    """The swap is one reference assignment after the load: a request that lands while
    the new run is still loading reads the old run, never nothing and never a half-built
    store. Pinned by holding the load open and reading through it."""
    registry = _registry_serving(tmp_path)
    _write_run(tmp_path, REFUSED_RUN, last_date=date(2026, 8, 30))
    load_started, load_may_finish = threading.Event(), threading.Event()
    real_load = xi_service._load_served_run

    def held_open_load(directory):
        load_started.set()
        load_may_finish.wait(5)
        return real_load(directory)

    xi_service._load_served_run = held_open_load
    reload_thread = threading.Thread(target=registry.reload, args=(str(tmp_path), REFUSED_RUN))
    try:
        reload_thread.start()
        assert load_started.wait(5), "the reload never reached the load"
        mid_reload = registry.status().model_dump()
        served_mid_reload = registry.store("T20").manifest.run_id
    finally:
        load_may_finish.set()
        reload_thread.join(5)
        xi_service._load_served_run = real_load

    assert (mid_reload["loaded"], mid_reload["run_id"], served_mid_reload) == (True, SERVING_RUN, SERVING_RUN)
    assert registry.status().run_id == REFUSED_RUN, "and once loaded, the new run serves"


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


def test_a_backtest_naming_a_date_the_as_of_pass_serves_is_not_refused(tmp_path, monkeypatch):
    """H-11 is about the through-today state. A backtest names the date it wants served
    and the as-of pass serves exactly that, so "how old is today's state?" is not a
    question about it -- refusing one would break the harness for a reason that does not
    describe it."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    ratings_through = date.today() - timedelta(days=40)
    _write_run(tmp_path, last_date=ratings_through)
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))
    registry.as_of_source_factory = lambda: ListSource([])

    store = registry.store_as_of("T20", ratings_through - timedelta(days=1))

    assert store is not registry.store("T20"), "the as-of pass answered, not the loaded state"


def test_a_backtest_dated_past_the_loaded_state_is_refused_when_that_state_is_stale(tmp_path, monkeypatch):
    """An as_of past everything the loaded state holds is answered from the through-today
    state unchanged -- the same state a live request gets -- so H-11's verdict on it
    applies whichever way it was asked for. Without this, naming any as_of after the
    ratings-through date is a way around the freshness refusal (SERVE-08)."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    _write_run(tmp_path, last_date=date.today() - timedelta(days=40))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    with pytest.raises(xi_service.RatingsStale) as excinfo:
        registry.store_as_of("T20", date.today() - timedelta(days=10))

    assert excinfo.value.payload["code"] == "RATINGS_STALE"


def test_the_check_is_off_when_the_limit_is_zero(tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    _write_run(tmp_path, last_date=date(2020, 1, 1))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    assert registry.freshness().fresh is True


# --- P1-5: every prediction names the run and the date it was served from ------------


def _live_win_request() -> XiWinRequest:
    return XiWinRequest(format="T20", team1_player_ids=["player0"], team2_player_ids=["player1"])


def test_a_prediction_carries_the_same_run_and_date_the_status_reports(tmp_path, monkeypatch):
    """The stamp and ``/xi/status`` read the same manifest and the same ``last_date``, so a
    payload copied out of the product carries exactly the date the status would show."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    _write_run(tmp_path, last_date=date(2026, 8, 30))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    win = xi_service.predict_win(_live_win_request(), registry)
    status = registry.status()

    assert win.served_ratings.run_id == status.run_id == "20260902T101500Z-ab12cd34"
    assert win.served_ratings.ratings_through == status.ratings_through == "2026-08-30"


def test_optimize_carries_the_stamp_on_the_rating_ordered_path_too(tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    _write_run(tmp_path, last_date=date(2026, 8, 30))
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))

    res = xi_service.optimize(
        XiOptimizeRequest(
            format="T20",
            pool_player_ids=[f"player{i}" for i in range(3)],
            objective="ratings",
            constraints=XiConstraints(team_size=3, min_bowlers=0, require_keeper=False),
        ),
        registry,
    )

    assert res.served_ratings.model_dump() == {"run_id": "20260902T101500Z-ab12cd34", "ratings_through": "2026-08-30"}


def test_a_store_that_cannot_name_its_run_is_refused_rather_than_stamped_blank(tmp_path, monkeypatch):
    """A prediction without its date is the defect the stamp closes, so a store that cannot
    say which run it is -- one assembled without a manifest -- is refused, not answered
    with an empty field. (A state with no date never gets this far: H-11 refuses it first.)"""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    _write_run(tmp_path)
    registry = xi_service.XiRegistry()
    registry.reload(str(tmp_path))
    registry._served.store.manifest = None

    with pytest.raises(xi_service.XiUnavailable, match="cannot name its run or the date"):
        xi_service.predict_win(_live_win_request(), registry)


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


def test_reload_refuses_to_publish_a_run_that_is_not_usable_and_keeps_serving(client, tmp_path):
    """EVAL-04 end to end: the run loads, publication is refused with its reasons on the
    wire, and the run that was serving goes on serving (B-13)."""
    _write_run(tmp_path, "20260901T090000Z-11111111")
    client.post("/admin/reload?run=20260901T090000Z-11111111")
    _write_run(tmp_path, "20260903T154222Z-4e009a52", unusable_reasons=_DOES_NOT_RANK)

    named = client.post("/admin/reload?run=20260903T154222Z-4e009a52")
    newest = client.post("/admin/reload")

    assert named.status_code == 409
    assert named.json()["detail"]["code"] == "RUN_ARTIFACTS_INVALID"
    assert "not usable and cannot be published -- T20: objective holdout AUC" in named.json()["detail"]["message"]
    assert newest.status_code == 200 and newest.json()["run_id"] == "20260901T090000Z-11111111"
    assert runs.read_current(str(tmp_path)) == "20260901T090000Z-11111111"


def test_artifacts_status_answers_the_date_per_run_and_names_what_cannot_load(client, tmp_path):
    """The listing on the wire: ``ratings_through`` on every loadable run, and the refusal
    reason on a run whose manifest predates the field (P2-2, §8.7)."""
    _write_run(tmp_path, "20260902T101500Z-ab12cd34", last_date=date(2026, 8, 30))
    older = _write_run(tmp_path, "20260831T090000Z-00000000", last_date=date(2026, 8, 28))
    _rewrite_manifest_without(older, "ratings_through")
    client.post("/admin/reload")

    data = client.get("/artifacts/status").json()

    by_id = {entry["run_id"]: entry for entry in data["runs"]}
    assert data["loaded_run"] == "20260902T101500Z-ab12cd34"
    assert by_id["20260902T101500Z-ab12cd34"]["ratings_through"] == data["ratings_through"] == "2026-08-30"
    assert "ratings_through" in by_id["20260831T090000Z-00000000"]["refused"]
    assert by_id["20260831T090000Z-00000000"]["loaded"] is False


def test_reload_of_a_run_without_ratings_through_answers_409_naming_it(client, tmp_path):
    directory = _write_run(tmp_path)
    _rewrite_manifest_without(directory, "ratings_through")

    resp = client.post("/admin/reload?run=20260902T101500Z-ab12cd34")

    assert resp.status_code == 409
    assert resp.json()["detail"]["code"] == "RUN_ARTIFACTS_INVALID"
    assert "20260902T101500Z-ab12cd34" in resp.json()["detail"]["message"]
    assert "ratings_through" in resp.json()["detail"]["message"]


def test_a_refused_reload_still_answers_409_and_the_served_run_stays_on_the_wire(client, tmp_path):
    """B-13 end to end: the refusal is the same 409 with the same code and message (D-6 is
    not weakened), the hint names the run still serving, and every status surface goes on
    reporting that run with the refusal beside it."""
    _write_run(tmp_path, SERVING_RUN, last_date=date(2026, 8, 28))
    client.post(f"/admin/reload?run={SERVING_RUN}")
    _write_refused_run(tmp_path)

    resp = client.post(f"/admin/reload?run={REFUSED_RUN}")
    xi_status = client.get("/xi/status").json()
    artifacts = client.get("/artifacts/status").json()

    assert resp.status_code == 409
    assert resp.json()["detail"]["code"] == "RUN_ARTIFACTS_INVALID"
    assert REFUSED_RUN in resp.json()["detail"]["message"]
    assert f"{SERVING_RUN} is still serving" in resp.json()["detail"]["hint"]
    assert (xi_status["loaded"], xi_status["run_id"], xi_status["ratings_through"]) == (True, SERVING_RUN, "2026-08-28")
    assert REFUSED_RUN in xi_status["error"]
    assert (artifacts["loaded_run"], artifacts["current_run"]) == (SERVING_RUN, SERVING_RUN)


def test_health_reports_the_run_and_the_freshness_verdict(client, tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    _write_run(tmp_path, last_date=date.today() - timedelta(days=40))
    client.post("/admin/reload")

    data = client.get("/health").json()

    assert data["status"] == "ok", "an unservable model is not a dead process"
    assert data["run_id"] == "20260902T101500Z-ab12cd34"
    assert data["ratings"]["code"] == "RATINGS_STALE"


def test_a_live_prediction_against_stale_ratings_is_a_503_with_the_code_on_the_wire(client, tmp_path, monkeypatch):
    """H-11 end to end at this service's boundary: the refusal a caller sees carries the
    code, the date, the age against the limit and the step that fixes it (P1-5)."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    stale_through = date.today() - timedelta(days=40)
    _write_run(tmp_path, last_date=stale_through)
    client.post("/admin/reload")

    resp = client.post("/xi/predict-win", json=_live_win_request().model_dump(mode="json", exclude_none=True))

    assert resp.status_code == 503
    detail = resp.json()["detail"]
    assert detail["code"] == "RATINGS_STALE"
    assert stale_through.isoformat() in detail["message"]
    assert "40 days old, limit 14" in detail["message"]
    assert "retrain" in detail["hint"]


def test_a_served_prediction_names_its_run_and_date_on_the_wire(client, tmp_path, monkeypatch):
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")
    _write_run(tmp_path, last_date=date(2026, 8, 30))
    client.post("/admin/reload")

    resp = client.post("/xi/predict-win", json=_live_win_request().model_dump(mode="json", exclude_none=True))

    assert resp.status_code == 200
    assert resp.json()["served_ratings"] == {"run_id": "20260902T101500Z-ab12cd34", "ratings_through": "2026-08-30"}


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
