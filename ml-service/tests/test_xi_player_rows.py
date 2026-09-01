"""Unit tests for the player-match rows (L1) and the expected-role state behind them."""

from __future__ import annotations

import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.ratings import RatingState
from ml.xi.rows import match_actuals, player_feature_rows, serving_match
from ml.xi.sources import batting_positions
from tests.xi_fixtures import ListSource, make_deliveries, make_match, xi


def test_batting_positions_orders_by_first_appearance_per_innings() -> None:
    d = make_deliveries(
        batters=["a0", "a1", "a0", "a2", "b0", "b1"],
        bowlers=["b5"] * 4 + ["a5"] * 2,
        runs=[1] * 6,
        wickets=[0] * 6,
        innings=[0, 0, 0, 0, 1, 1],
    )

    positions = batting_positions(d)

    assert positions == {"a0": 1, "a1": 2, "a2": 3, "b0": 1, "b1": 2}


def test_batting_positions_keeps_first_batting_innings() -> None:
    """A TEST batter batting in both innings keeps the first innings' slot."""
    d = make_deliveries(
        batters=["a0", "a1", "a1", "a0"],
        bowlers=["b5"] * 4,
        runs=[1] * 4,
        wickets=[0] * 4,
        innings=[0, 0, 2, 2],
    )

    positions = batting_positions(d)

    assert positions == {"a0": 1, "a1": 2}


def test_match_actuals_counts_batting_bowling_and_dismissals() -> None:
    d = make_deliveries(
        batters=["a0", "a0", "a0", "a1"],
        bowlers=["b0", "b0", "b1", "b1"],
        runs=[4, 6, 1, 0],
        wickets=[0, 0, 0, 1],
        players_out=["", "", "", "a1"],
    )
    m = make_match("m1", 0, "A", xi("a"), xi("b"), d)

    actuals = match_actuals(m)

    a0 = actuals["a0"]
    assert a0["balls_faced"] == 3 and a0["runs"] == 11 and a0["fours"] == 1 and a0["sixes"] == 1
    assert a0["batting_position"] == 1 and a0["dismissals"] == 0
    a1 = actuals["a1"]
    assert a1["dismissals"] == 1 and a1["batting_position"] == 2
    b0 = actuals["b0"]
    assert b0["balls_bowled"] == 2 and b0["runs_conceded"] == 10 and b0["wickets"] == 0
    b1 = actuals["b1"]
    assert b1["balls_bowled"] == 2 and b1["runs_conceded"] == 1 and b1["wickets"] == 1


def _two_match_source() -> ListSource:
    t1, t2 = xi("a"), xi("b")
    d = make_deliveries(
        batters=["a0"] * 6 + ["b0"] * 6,
        bowlers=["b5"] * 6 + ["a5"] * 6,
        runs=[4] * 6 + [1] * 6,
        wickets=[0] * 12,
        innings=[0] * 6 + [1] * 6,
    )
    return ListSource(
        [
            make_match("m1", 0, "A", t1, t2, d),
            make_match("m2", 1, "A", t1, t2, d),
        ]
    )


def test_match_actuals_credits_catches_to_named_fielders_on_caught_dismissals() -> None:
    d = make_deliveries(
        batters=["a0", "a1", "a2", "a3"],
        bowlers=["b0"] * 4,
        runs=[0] * 4,
        wickets=[1, 1, 1, 0],
        players_out=["a0", "a1", "a2", ""],
    )
    d.fielders = [["b3"], ["b4"], ["b3"], []]
    d.stumping[1] = 1.0  # the second is a stumping: the keeper takes no catch
    d.bowler_wicket[2] = 0.0  # the third is a run out: not the bowler's, no catch

    actuals = match_actuals(make_match("m1", 0, "A", xi("a"), xi("b"), d))

    assert actuals["b3"]["catches"] == 1
    assert "b4" not in actuals or actuals["b4"]["catches"] == 0
    assert actuals["b0"]["wickets"] == 2


def test_serving_feature_rows_equal_the_training_frames_feature_columns() -> None:
    """The endpoint builds rows from ids through the same assembly the training pass uses,
    so for the same state and the same elevens the feature columns are identical."""
    source = _two_match_source()
    frame = build(source).player_frame
    state = RatingState()
    state.update(source.matches[0])
    second = source.matches[1]
    stub = serving_match(
        second.format_code, second.team1_players, second.team2_players, "A", "B", "V", second.match_date
    )

    served = pd.DataFrame(player_feature_rows(state, stub)[1])
    expected = frame[frame.match_id == "m2"].reset_index(drop=True)

    assert list(served.player_key) == list(expected.player_key)
    pd.testing.assert_frame_equal(
        served[C.PLAYER_MATCH_FEATURE_COLS].reset_index(drop=True), expected[C.PLAYER_MATCH_FEATURE_COLS]
    )
    assert not set(served.columns) & set(C.PLAYER_MATCH_TARGET_COLS)


def test_serving_rows_without_team_names_read_neutral_context() -> None:
    source = _two_match_source()
    state = build(source).state
    stub = serving_match("T20", xi("a"), xi("b"), None, None, None, state.last_date)

    rows = pd.DataFrame(player_feature_rows(state, stub)[1])

    assert (rows.elo_edge == 0.0).all() and (rows.venue_bf_rate == 0.5).all() and (rows.venue_n == 0.0).all()


def test_player_frame_covers_all_xi_players_with_exact_columns() -> None:
    """H-20: one row per XI player per decided match, batted or not, and the column list
    is the contract's, in order."""
    result = build(_two_match_source())

    assert list(result.player_frame.columns) == C.PLAYER_MATCH_COLS
    assert len(result.player_frame) == 2 * 22
    m1 = result.player_frame[result.player_frame.match_id == "m1"]
    assert set(m1.player_key) == set(xi("a")) | set(xi("b"))
    never_involved = m1[m1.player_key == "a7"].iloc[0]
    assert never_involved.balls_faced == 0 and never_involved.balls_bowled == 0
    assert never_involved.batting_position == 0


def test_player_rows_are_as_of_and_join_actuals() -> None:
    """The first match's rows carry a blank as-of state; the second match's rows see the
    first; actual performance is joined onto the same rows."""
    result = build(_two_match_source())
    frame = result.player_frame

    first = frame[(frame.match_id == "m1") & (frame.player_key == "a0")].iloc[0]
    second = frame[(frame.match_id == "m2") & (frame.player_key == "a0")].iloc[0]

    assert first.bat_rate == pytest.approx(0.0)
    assert first.exp_bat_position == pytest.approx(C.BAT_POSITION_PRIOR)
    assert first.runs == 24 and first.balls_faced == 6 and first.fours == 6
    assert second.bat_rate > 0
    assert second.exp_bat_position < C.BAT_POSITION_PRIOR  # batted first drags the slot up
    assert second.bat_innings_share == pytest.approx(1.0)
    non_batter = frame[(frame.match_id == "m2") & (frame.player_key == "a7")].iloc[0]
    assert non_batter.bat_innings_share == pytest.approx(0.0)
    assert non_batter.exp_bat_position == pytest.approx(C.BAT_POSITION_PRIOR)


def test_player_rows_mirror_side_aggregates_and_win_frame() -> None:
    """own_/opp_ aggregates equal the win frame's t1_/t2_ columns for side 1, swapped for
    side 2, so the two frames describe the same match through one set of numbers."""
    result = build(_two_match_source())
    win_row = result.frame[result.frame.match_id == "m2"].iloc[0]
    players = result.player_frame[result.player_frame.match_id == "m2"]

    side1 = players[players.side == 1].iloc[0]
    side2 = players[players.side == 2].iloc[0]
    for stem in C.SIDE_FEATURE_STEMS:
        assert side1[f"own_{stem}"] == pytest.approx(win_row[f"t1_{stem}"])
        assert side1[f"opp_{stem}"] == pytest.approx(win_row[f"t2_{stem}"])
        assert side2[f"own_{stem}"] == pytest.approx(win_row[f"t2_{stem}"])
        assert side2[f"opp_{stem}"] == pytest.approx(win_row[f"t1_{stem}"])
    assert side1.elo_edge == pytest.approx(win_row.team_elo_diff)
    assert side2.elo_edge == pytest.approx(-win_row.team_elo_diff)


def test_phase_rates_split_death_overs_from_powerplay() -> None:
    """A batter who only scores in the death overs earns a death rate, not a powerplay one."""
    t1, t2 = xi("a"), xi("b")
    death = make_deliveries(
        batters=["a0"] * 6,
        bowlers=["b5"] * 6,
        runs=[6] * 6,
        wickets=[0] * 6,
        overs=[16, 17, 18, 19, 19, 19],
    )
    result = build(ListSource([make_match("m1", 0, "A", t1, t2, death), make_match("m2", 1, "A", t1, t2, death)]))

    row = result.player_frame[(result.player_frame.match_id == "m2") & (result.player_frame.player_key == "a0")].iloc[0]
    assert row.bat_death_rate > 0
    assert row.bat_pp_rate == pytest.approx(0.0)
    assert row.bat_mid_rate == pytest.approx(0.0)


def test_undecided_matches_produce_no_player_rows_but_update_state() -> None:
    t1, t2 = xi("a"), xi("b")
    d = make_deliveries(["a0"] * 6, ["b5"] * 6, [4] * 6, [0] * 6)
    result = build(
        ListSource(
            [
                make_match("m1", 0, None, t1, t2, d),
                make_match("m2", 1, "A", t1, t2, d),
            ]
        )
    )

    assert set(result.player_frame.match_id) == {"m2"}
    row = result.player_frame[result.player_frame.player_key == "a0"].iloc[0]
    assert row.bat_rate > 0  # the undecided match still fed the as-of state


def test_win_row_carries_innings_outcomes_and_simulation_context() -> None:
    """E2's targets (what each innings did) and the simulator's as-of inputs travel on the
    win row; the outcomes are never on the player rows, where they would be a leak."""
    source = _two_match_source()

    frame = build(source).frame

    assert set(C.INNINGS_OUTCOME_COLS) <= set(frame.columns)
    assert set(C.SIMULATION_CONTEXT_COLS) <= set(frame.columns)
    first = frame[frame.match_id == "m1"].iloc[0]
    second = frame[frame.match_id == "m2"].iloc[0]
    d = source.matches[0].deliveries
    assert first.innings1_runs == d.runs_total[d.innings == 0].sum()
    assert first.innings1_deliveries == (d.innings == 0).sum()
    assert first.ctx_innings_deliveries == 120.0  # nothing before the first match: the law
    assert second.ctx_innings_deliveries != 120.0  # the first match has been folded in
    assert not set(C.INNINGS_OUTCOME_COLS) & set(C.PLAYER_MATCH_COLS)
