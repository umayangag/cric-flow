"""Unit tests for the per-ball sequence flags (E1) and their as-of accumulation."""

from __future__ import annotations

import numpy as np
import pytest

from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.sequence import sequence_flags
from ml.xi.sources import Deliveries
from tests.xi_fixtures import ListSource, make_deliveries, make_match, xi


def test_batter_dot_streaks_reset_on_a_scoring_ball_and_stay_within_the_stream() -> None:
    d = make_deliveries(
        batters=["a0", "a0", "a0", "a1", "a0", "a0"],
        bowlers=["b0"] * 6,
        runs=[0, 0, 4, 0, 0, 0],
        wickets=[0] * 6,
    )

    flags = sequence_flags(d)

    # a0: dots before each of their balls: 0, 1, 2 (then a boundary), 0, 1; a1's single dot is its own stream
    assert list(flags.bat_dots_before) == [0, 1, 2, 0, 0, 1]
    assert list(flags.bat_after_boundary) == [False, False, False, False, True, False]


def test_bowler_streams_ignore_which_batter_faced() -> None:
    d = make_deliveries(
        batters=["a0", "a1", "a0", "a1"],
        bowlers=["b0", "b0", "b0", "b0"],
        runs=[0, 0, 0, 1],
        wickets=[0, 0, 1, 0],
    )

    flags = sequence_flags(d)

    assert list(flags.bowl_dots_before) == [0, 1, 2, 3]  # the wicket ball was a dot too
    assert list(flags.bowl_after_wicket) == [False, False, False, True]
    assert list(flags.bat_dots_before) == [0, 0, 1, 1]


def test_streams_do_not_cross_innings() -> None:
    d = make_deliveries(
        batters=["a0", "a0", "a0", "a0"],
        bowlers=["b0"] * 4,
        runs=[0, 0, 0, 0],
        wickets=[0] * 4,
        innings=[0, 0, 2, 2],
    )

    flags = sequence_flags(d)

    assert list(flags.bat_dots_before) == [0, 1, 0, 1]


def test_spells_group_alternate_end_overs_and_split_on_a_longer_gap() -> None:
    d = make_deliveries(
        batters=["a0"] * 8,
        bowlers=["b0"] * 8,
        runs=[1] * 8,
        wickets=[0] * 8,
        overs=[0, 0, 2, 2, 4, 4, 9, 9],
    )

    flags = sequence_flags(d)

    assert list(flags.spell_first_over) == [True, True, False, False, False, False, True, True]
    assert flags.spells_per_bowler == {"b0": (2, 4)}


def test_empty_deliveries_produce_empty_flags() -> None:
    flags = sequence_flags(Deliveries.empty())

    assert len(flags.bat_dots_before) == 0
    assert flags.spells_per_bowler == {}


def _two_match_source() -> ListSource:
    d1 = make_deliveries(
        batters=["a0"] * 6 + ["b0"] * 6,
        bowlers=["b5"] * 6 + ["a5"] * 6,
        runs=[0, 0, 0, 4, 0, 0, 1, 1, 1, 1, 1, 1],
        wickets=[0] * 12,
        overs=[0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
        innings=[0] * 6 + [1] * 6,
    )
    d2 = make_deliveries(batters=["a0"], bowlers=["b5"], runs=[1], wickets=[0])
    return ListSource([make_match("m1", 0, "A", xi("a"), xi("b"), d1), make_match("m2", 1, "A", xi("a"), xi("b"), d2)])


def test_sequence_columns_are_as_of_and_read_neutral_without_history() -> None:
    frame = build(_two_match_source()).player_frame
    first = frame[(frame.match_id == "m1") & (frame.player_key == "a0")].iloc[0]
    second = frame[(frame.match_id == "m2") & (frame.player_key == "a0")].iloc[0]

    for key in C.PLAYER_SEQUENCE_KEYS:
        assert key in frame.columns
    assert first.bat_stuck_share == pytest.approx(0.0)
    assert first.bowl_spell_overs == pytest.approx(2.0)  # the spell-length prior
    # a0 faced 6 balls, of which balls 3 and 4 (0-based 2, 3) followed >= 2 dots; the fourth was a four
    assert second.bat_stuck_share == pytest.approx(2.0 / (6.0 + C.SEQUENCE_PRIOR_BALLS))
    assert second.bat_release_rate > 0.0
    b5 = frame[(frame.match_id == "m2") & (frame.player_key == "b5")].iloc[0]
    assert b5.bowl_squeeze_share > 0.0
    assert b5.bowl_spell_overs == pytest.approx((1.0 + 2.0) / (1.0 + 1.0))


def test_sequence_flags_are_finite_on_the_synthetic_history() -> None:
    from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history

    matches, _, _ = _synthetic_history(20)
    frame = build(_ListSource(matches)).player_frame

    assert np.isfinite(frame[C.PLAYER_SEQUENCE_KEYS].to_numpy()).all()
