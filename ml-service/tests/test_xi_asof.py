"""Unit tests for the as-of serving path (ml.xi.asof) and the H-8 parity check."""

from __future__ import annotations

import json
import os
from datetime import date
from typing import List

import joblib
import numpy as np
import pandas as pd
import pytest
from sklearn.linear_model import LogisticRegression
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

from ml.xi import asof as asof_module
from ml.xi import contract as C
from ml.xi import runs
from ml.xi import store as store_module
from ml.xi.asof import PARITY_TOLERANCE, ROUND_TRIP_RUN_ID, AsOfRatings, AsOfServer, round_trip_store, serving_parity
from ml.xi.builder import build
from ml.xi.ratings import CONTEXT_ARRAY_NAMES, STATE_FLAG_NAMES, RatingState
from ml.xi.runs import RunArtifactsInvalid
from ml.xi.sources import Deliveries
from ml.xi.stakes import STAGE_FINAL, MatchStakes
from ml.xi.store import FormatModels, XiStore
from ml.xi.train import train_format
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history
from tests.xi_fixtures import ListSource, make_deliveries, make_match, xi


def _matches(days, winner="A"):
    t1, t2 = xi("a"), xi("b")
    d = make_deliveries(["a0"] * 6, ["b5"] * 6, [4] * 6, [0] * 6)
    return [make_match(f"m{day}", day, winner, t1, t2, d) for day in days]


def test_state_as_of_folds_only_strictly_earlier_matches() -> None:
    matches = _matches([0, 1, 2])  # 2024-01-01 .. 2024-01-03
    asof = AsOfRatings(ListSource(matches))

    state = asof.state_as_of(date(2024, 1, 2))

    expected = RatingState()
    expected.update(matches[0])
    vectors = state.side_vectors("T20", ["a0"])
    expected_vectors = expected.side_vectors("T20", ["a0"])
    assert vectors["bat_rate"][0] == pytest.approx(expected_vectors["bat_rate"][0])
    assert state.matches_seen == 1


def test_state_as_of_excludes_the_queried_date_itself() -> None:
    """Day-close: a backtest of a match on date D must not see D's other matches."""
    matches = _matches([0, 1])
    asof = AsOfRatings(ListSource(matches))

    state = asof.state_as_of(date(2024, 1, 1))

    assert state.matches_seen == 0


def test_state_as_of_holds_back_a_match_still_being_played(caplog) -> None:
    """FEAT-09: a Test that started on the 1st and ends on the 5th is not in the state a
    fixture on the 3rd is served from -- nobody on the 3rd knew its last three days -- and
    it is folded once the query date is past its last day. The state then holds a match
    through the 5th, so asking for the 5th again is running backwards."""
    t1, t2 = xi("a"), xi("b")
    d = make_deliveries(["a0"] * 6, ["b5"] * 6, [4] * 6, [0] * 6)
    test_match = make_match("test", 0, "A", t1, t2, d, fmt="TEST", last_day=4)  # Jan 1 .. Jan 5
    one_day = make_match("t20", 1, "A", t1, t2, d)  # Jan 2
    asof = AsOfRatings(ListSource([test_match, one_day]))

    during = asof.state_as_of(date(2024, 1, 3))
    seen_during = during.matches_seen
    after = asof.state_as_of(date(2024, 1, 6))

    assert seen_during == 1, "the one-day match on the 2nd is in; the Test still on is not"
    assert after.matches_seen == 2 and after.last_date == date(2024, 1, 5)
    with pytest.raises(ValueError, match="through 2024-01-05"):
        asof.state_as_of(date(2024, 1, 5))


def test_state_as_of_raises_when_asked_to_run_backwards() -> None:
    asof = AsOfRatings(ListSource(_matches([0, 1, 2])))
    asof.state_as_of(date(2024, 1, 3))

    with pytest.raises(ValueError, match="already contains"):
        asof.state_as_of(date(2024, 1, 2))


def test_state_as_of_allows_repeated_and_advancing_queries() -> None:
    asof = AsOfRatings(ListSource(_matches([0, 1, 2])))

    first = asof.state_as_of(date(2024, 1, 2))
    again = asof.state_as_of(date(2024, 1, 2))
    later = asof.state_as_of(date(2024, 1, 4))

    assert first is again is later  # one advancing state
    assert later.matches_seen == 3


def test_as_of_server_rebuilds_when_a_query_goes_backwards() -> None:
    matches = _matches([0, 1, 2])
    builds = []

    def factory():
        builds.append(1)
        return ListSource(matches)

    server = AsOfServer(factory)
    assert server.state_as_of(date(2024, 1, 3)).matches_seen == 2
    assert server.state_as_of(date(2024, 1, 2)).matches_seen == 1
    assert len(builds) == 2


def test_the_as_of_pass_is_built_with_every_registered_state_flag() -> None:
    """SERVE-07. ``AsOfRatings`` builds a fresh ``RatingState``; a flag it is not handed
    reads off, so the pass runs a different arm from the run it is serving. It takes the
    whole of ``RatingState.flags()``, so registering a new flag carries it here rather
    than leaving the next latent divergence for the next audit."""
    all_on = dict.fromkeys(STATE_FLAG_NAMES, True)

    asof = AsOfRatings(ListSource(_matches([0, 1])), all_on)

    assert asof.state.flags() == all_on


def test_the_as_of_server_hands_every_registered_state_flag_to_its_pass() -> None:
    """The same, through the object the serving path actually holds -- including the pass
    it rebuilds when a request goes backwards, which is a second place the flags have to
    reach."""
    all_on = dict.fromkeys(STATE_FLAG_NAMES, True)
    server = AsOfServer(lambda: ListSource(_matches([0, 1, 2])), state_flags=all_on)

    forwards = server.state_as_of(date(2024, 1, 3))
    backwards = server.state_as_of(date(2024, 1, 2))

    assert forwards.flags() == all_on
    assert backwards.flags() == all_on, "the rebuilt pass keeps them too"


def test_serving_parity_passes_on_an_honest_frame() -> None:
    matches = _matches([0, 1, 2, 3, 4])
    result = build(ListSource(matches))

    report = serving_parity(ListSource(matches), result.frame, result.player_frame, last_n=3)

    assert report["passed"], report["mismatches"]
    assert report["matches_compared"] == 3
    assert report["player_rows_compared"] == 3 * 22
    assert report["max_abs_difference"] == pytest.approx(0.0, abs=1e-12)


def test_serving_parity_fails_on_a_corrupted_frame() -> None:
    matches = _matches([0, 1, 2, 3, 4])
    result = build(ListSource(matches))
    corrupted = result.frame.copy()
    corrupted.loc[corrupted.match_id == "m4", "t1_pelo_mean"] += 5.0

    report = serving_parity(ListSource(matches), corrupted, result.player_frame, last_n=3)

    assert not report["passed"]
    assert any("t1_pelo_mean" in m for m in report["mismatches"])


def test_serving_parity_passes_on_a_decided_match_with_no_deliveries() -> None:
    """FEAT-03: such a match has a win row, no player rows and unobserved (NaN) innings
    outcomes on both paths; the rebuild must agree with the frame rather than trip on
    the NaN, and the match still counts as compared."""
    matches = _matches([0, 1, 2, 3]) + [make_match("m4", 4, "A", xi("a"), xi("b"), Deliveries.empty())]
    result = build(ListSource(matches))

    report = serving_parity(ListSource(matches), result.frame, result.player_frame, last_n=3)

    assert report["passed"], report["mismatches"]
    assert report["matches_compared"] == 3
    assert report["win_rows_compared"] == 3
    assert report["player_rows_compared"] == 2 * 22
    assert report["max_abs_difference"] == pytest.approx(0.0, abs=1e-12)


def test_serving_parity_reports_matches_the_source_no_longer_yields() -> None:
    matches = _matches([0, 1, 2, 3, 4])
    result = build(ListSource(matches))

    report = serving_parity(ListSource(matches[:-1]), result.frame, result.player_frame, last_n=3)

    assert not report["passed"]
    assert any("yielded" in m for m in report["mismatches"])


def test_as_of_state_matches_training_frame_row_for_row() -> None:
    """The heart of H-8: an as-of rebuild equals the frame even across same-day matches."""
    t1, t2 = xi("a"), xi("b")
    d = make_deliveries(["a0"] * 6, ["b5"] * 6, [4] * 6, [0] * 6)
    matches = [
        make_match("m1", 0, "A", t1, t2, d),
        make_match("m2", 1, "A", t1, t2, d),
        make_match("m3", 1, "B", t1, t2, d),  # same day as m2
        make_match("m4", 2, "A", t1, t2, d),
    ]
    result = build(ListSource(matches))

    report = serving_parity(ListSource(matches), result.frame, result.player_frame, last_n=4)

    assert report["passed"], report["mismatches"]
    assert report["matches_compared"] == 4


def test_gender_split_context_keeps_separate_baselines() -> None:
    t1, t2 = xi("a"), xi("b")
    d = make_deliveries(["a0"] * 6, ["b5"] * 6, [4] * 6, [0] * 6)
    state = RatingState(gender_split_context=True)
    state.update(make_match("m1", 0, "A", t1, t2, d, gender="female"))

    assert state.ctx_balls[1].sum() > state.ctx_balls[1].size  # the female group moved
    assert state.ctx_balls[0].sum() == pytest.approx(float(state.ctx_balls[0].size))  # the male group did not

    unsplit = RatingState(gender_split_context=False)
    unsplit.update(make_match("m1", 0, "A", t1, t2, d, gender="female"))
    assert unsplit.ctx_balls[0].sum() > unsplit.ctx_balls[0].size
    assert np.all(unsplit.ctx_balls[1] == 1.0)


# --- EVAL-10: the check reaches the artifact and the store ----------------------------


def _synthetic_run(matches):
    """A rating pass and one format's win models fitted the way retrain fits them."""
    result = build(_ListSource(matches))
    models, report = train_format(result.frame, "T20", pd.Timestamp("2023-05-01"))
    assert models is not None, report
    return result, models


@pytest.fixture(scope="module")
def honest_run():
    matches, _, _ = _synthetic_history(160)
    result, models = _synthetic_run(matches)
    return matches, result, models


def _last_n_ids(frame: pd.DataFrame, n: int) -> List[str]:
    return list(frame.sort_values(["match_date", "match_id"], kind="stable").match_id.tail(n))


def test_serving_parity_through_a_round_tripped_store_passes_on_an_honest_run(tmp_path, honest_run) -> None:
    """The store is written to a run directory and loaded back through ``XiStore.load``,
    and what it serves -- the display and objective probabilities, as-of and from the
    loaded through-today state -- agrees with the frame on every compared match."""
    matches, result, models = honest_run
    store = round_trip_store(result.state, {"T20": models}, {}, str(tmp_path / "run"))

    report = serving_parity(_ListSource(matches), result.frame, result.player_frame, last_n=5, store=store)

    assert report["passed"], report["mismatches"]
    assert report["run_id"] == ROUND_TRIP_RUN_ID
    assert report["served_probabilities_compared"] == 5
    assert report["artifact_probabilities_compared"] == 5
    assert report["max_abs_difference"] <= PARITY_TOLERANCE


def test_the_round_trip_refuses_a_state_that_consumed_no_matches(tmp_path) -> None:
    with pytest.raises(ValueError, match="consumed no matches"):
        round_trip_store(RatingState(), {}, {}, str(tmp_path / "run"))


def test_the_round_trip_refuses_to_serve_no_win_model_at_all(tmp_path, honest_run) -> None:
    """A store with no win model is not a store the routes could answer from, and the
    manifest refusal it would otherwise turn into names the wrong thing."""
    _, result, _ = honest_run

    with pytest.raises(ValueError, match="no format fitted a win model"):
        round_trip_store(result.state, {}, {}, str(tmp_path / "run"))


def test_a_win_artifact_carrying_the_pre_b7_display_columns_passes_the_row_check_but_cannot_be_served(
    tmp_path, honest_run
) -> None:
    """B-7's shape: an artifact fitted on ``t1_pelo_std`` / ``t2_pelo_std``. The row
    comparison alone -- the whole of H-8 before EVAL-10 -- passes it, because it never
    looks at an artifact; the store-aware check cannot even begin, because loading the
    run is refused by name (D-6)."""
    matches, result, models = honest_run
    directory = str(tmp_path / "run")
    round_trip_store(result.state, {"T20": models}, {}, directory)
    path = os.path.join(directory, "xi_win_T20.joblib")
    stale = joblib.load(path)
    stale.display_cols = list(C.DISPLAY_FEATURE_COLS) + ["t1_pelo_std", "t2_pelo_std"]
    joblib.dump(stale, path)

    rows_only = serving_parity(_ListSource(matches), result.frame, result.player_frame, last_n=5)
    with pytest.raises(RunArtifactsInvalid) as refused:
        XiStore.load(directory)

    assert rows_only["passed"], rows_only["mismatches"]
    assert ROUND_TRIP_RUN_ID in str(refused.value) and "t1_pelo_std" in str(refused.value)


def test_a_display_column_the_serving_row_cannot_populate_fails_the_served_probability_not_the_rows(
    tmp_path, monkeypatch
) -> None:
    """The next B-7, past the loader: a display column the contract lists and the frame
    carries but the store's row assembly reads as ``row.get(c, 0.0)``. The stakes columns
    are exactly that (``rows.stakes_columns``: a serving record carries none). The loader
    passes the artifact -- its column list is the contract's -- and the rows agree, since
    both paths build them through ``rows.py``; only the served display probability, from
    the store's own row, differs from the frame's."""
    matches, _, _ = _synthetic_history(160)
    for match in matches[::4]:
        match.stakes = MatchStakes(stage_label=STAGE_FINAL)
    result = build(_ListSource(matches))
    display_cols = list(C.DISPLAY_FEATURE_COLS) + ["stakes_knockout"]
    monkeypatch.setattr(C, "DISPLAY_FEATURE_COLS", display_cols)
    y = result.frame[C.TARGET_COL].to_numpy(dtype=float)

    def logistic(columns: List[str]):
        return make_pipeline(StandardScaler(), LogisticRegression()).fit(result.frame[columns].to_numpy(dtype=float), y)

    models = FormatModels(
        format_code="T20",
        objective=logistic(list(C.XI_FEATURE_COLS)),
        display=logistic(display_cols),
        objective_cols=list(C.XI_FEATURE_COLS),
        display_cols=display_cols,
        metadata={},
    )
    store = round_trip_store(result.state, {"T20": models}, {}, str(tmp_path / "run"))
    knockout_ids = {m.match_id for m in matches[::4]} & set(_last_n_ids(result.frame, 5))
    assert knockout_ids, "the fixture must put a knockout match among the compared ones"

    rows_only = serving_parity(_ListSource(matches), result.frame, result.player_frame, last_n=5)
    report = serving_parity(_ListSource(matches), result.frame, result.player_frame, last_n=5, store=store)

    assert rows_only["passed"], rows_only["mismatches"]
    assert not report["passed"]
    assert report["win_rows_compared"] == 5 and report["served_probabilities_compared"] == 5
    assert all("served display probability vs the frame" in m for m in report["mismatches"]), report["mismatches"]
    assert {m.split()[1] for m in report["mismatches"]} == knockout_ids


def test_an_accumulator_the_artifact_does_not_carry_fails_the_loaded_artifact_check_not_the_rows(
    tmp_path, monkeypatch, honest_run
) -> None:
    """D-6's shape, one level above the loader's list: an accumulator the served number
    reads (``pelo``) that the payload does not carry, because the enumeration the writer
    and the loader share does not name it. The loader passes the run -- it walks that same
    list -- the rows agree, and the as-of served probabilities agree too, since the as-of
    comparison replaces the loaded state; only the live reading from the loaded artifact
    differs from the same models over the freshly folded state."""
    matches, result, models = honest_run
    without_pelo = tuple(name for name in store_module.PLAYER_ARRAY_NAMES if name != "pelo")
    monkeypatch.setattr(store_module, "PLAYER_ARRAY_NAMES", without_pelo)
    monkeypatch.setattr(store_module, "STATE_ARRAY_NAMES", without_pelo + tuple(CONTEXT_ARRAY_NAMES))
    store = round_trip_store(result.state, {"T20": models}, {}, str(tmp_path / "run"))

    rows_only = serving_parity(_ListSource(matches), result.frame, result.player_frame, last_n=5)
    report = serving_parity(_ListSource(matches), result.frame, result.player_frame, last_n=5, store=store)

    assert rows_only["passed"], rows_only["mismatches"]
    assert not report["passed"]
    assert report["served_probabilities_compared"] == 5 and report["artifact_probabilities_compared"] == 5
    assert report["mismatches"] and all("from the loaded artifact" in m for m in report["mismatches"])
    # The display model reads ``pelo``; the objective's sign-bounded fit may clamp its
    # coefficient to zero on a synthetic history, so it is the display reading that has
    # to show the missing accumulator, and every mismatch names the artifact as its source.
    assert any("display probability" in m for m in report["mismatches"]), report["mismatches"]


# --- the CLI: a run on disk against a fresh pass -------------------------------------


def _published_run(tmp_path, result, models, matches) -> str:
    """A run under an artifacts root, written the way the harness round-trips one, with
    the manifest carrying the digest of the cricket it was built from, and published."""
    directory = runs.run_dir(str(tmp_path), ROUND_TRIP_RUN_ID)
    round_trip_store(result.state, {"T20": models}, {}, directory)
    path = runs.manifest_path(directory)
    with open(path) as fh:
        raw = json.load(fh)
    raw["dataset_sha"], raw["dataset_digest"] = result.dataset_digest()
    with open(path, "w") as fh:
        json.dump(raw, fh)
    runs.set_current(str(tmp_path), ROUND_TRIP_RUN_ID)
    return directory


def _cli_over(monkeypatch, matches, result) -> None:
    """The archive path of the CLI over an in-memory history: the rating pass returns the
    fixture's result and the source it opens yields the fixture's matches."""
    monkeypatch.setattr("ml.xi.builder.build", lambda source, **kwargs: result)
    monkeypatch.setattr("ml.xi.sources.CricsheetJsonSource", lambda *args, **kwargs: _ListSource(matches))


def test_main_checks_the_run_current_names_and_exits_zero_when_parity_holds(tmp_path, monkeypatch, honest_run):
    matches, result, models = honest_run
    _published_run(tmp_path, result, models, matches)
    _cli_over(monkeypatch, matches, result)

    code = asof_module.main(["--cricsheet-dir", str(tmp_path), "--out", str(tmp_path), "--last-n", "5"])

    assert code == 0


def test_main_exits_one_when_the_named_run_does_not_serve_what_the_frame_says(tmp_path, monkeypatch, honest_run):
    matches, result, models = honest_run
    without_pelo = tuple(name for name in store_module.PLAYER_ARRAY_NAMES if name != "pelo")
    monkeypatch.setattr(store_module, "PLAYER_ARRAY_NAMES", without_pelo)
    monkeypatch.setattr(store_module, "STATE_ARRAY_NAMES", without_pelo + tuple(CONTEXT_ARRAY_NAMES))
    _published_run(tmp_path, result, models, matches)
    _cli_over(monkeypatch, matches, result)

    code = asof_module.main(
        ["--cricsheet-dir", str(tmp_path), "--out", str(tmp_path), "--run", ROUND_TRIP_RUN_ID, "--last-n", "5"]
    )

    assert code == 1


def test_main_refuses_a_run_trained_on_other_cricket_than_the_source_holds(tmp_path, monkeypatch, honest_run):
    """A through-today comparison against other data would measure the data; the CLI
    says so and compares nothing."""
    matches, result, models = honest_run
    round_trip_store(result.state, {"T20": models}, {}, runs.run_dir(str(tmp_path), ROUND_TRIP_RUN_ID))
    _cli_over(monkeypatch, matches, result)

    code = asof_module.main(
        ["--cricsheet-dir", str(tmp_path), "--out", str(tmp_path), "--run", ROUND_TRIP_RUN_ID, "--last-n", "5"]
    )

    assert code == 2


def test_main_refuses_a_run_the_loader_refuses_and_when_nothing_is_published(tmp_path, monkeypatch, honest_run):
    matches, result, models = honest_run
    _cli_over(monkeypatch, matches, result)
    nothing = asof_module.main(["--cricsheet-dir", str(tmp_path), "--out", str(tmp_path)])
    directory = _published_run(tmp_path, result, models, matches)
    os.remove(runs.manifest_path(directory))

    refused = asof_module.main(["--cricsheet-dir", str(tmp_path), "--out", str(tmp_path)])

    assert nothing == 2 and refused == 2
