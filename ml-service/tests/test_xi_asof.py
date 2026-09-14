"""Unit tests for the as-of serving path (ml.xi.asof) and the H-8 parity check."""

from __future__ import annotations

from datetime import date

import numpy as np
import pytest

from ml.xi.asof import AsOfRatings, AsOfServer, serving_parity
from ml.xi.builder import build
from ml.xi.ratings import RatingState
from ml.xi.sources import Deliveries
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
    corrupted.loc[corrupted.match_id == "m4", "d_pelo_mean"] += 5.0

    report = serving_parity(ListSource(matches), corrupted, result.player_frame, last_n=3)

    assert not report["passed"]
    assert any("d_pelo_mean" in m for m in report["mismatches"])


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
