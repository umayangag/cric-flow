"""Unit tests for the match-stakes derivation (X-3): the stage vocabulary, edition
splitting, the two dead-rubber tables, and the columns the derivation puts on a win row."""

from __future__ import annotations

from datetime import date, timedelta
from typing import List, Optional

import numpy as np
import pytest

from ml.xi import contract as C
from ml.xi import stakes
from ml.xi.builder import build
from ml.xi.rows import stakes_columns
from tests.xi_fixtures import ListSource, make_deliveries, make_match, xi


def header(
    match_id: str,
    day: int,
    team1: str,
    team2: str,
    winner: Optional[str] = None,
    competition: str = "Cup",
    match_number: Optional[int] = None,
    event_stage: str = "",
    event_group: str = "",
    fmt: str = "T20",
) -> stakes.Header:
    return stakes.Header(
        match_id=match_id,
        match_date=date(2024, 1, 1) + timedelta(days=day),
        format_code=fmt,
        gender="male",
        team1=team1,
        team2=team2,
        competition=competition,
        match_number=match_number,
        event_stage=event_stage,
        event_group=event_group,
        winner=winner,
    )


# --- The stage vocabulary ---------------------------------------------------------------


@pytest.mark.parametrize(
    "spelling,expected",
    [
        ("Final", stakes.STAGE_FINAL),
        ("Trophy Final", stakes.STAGE_FINAL),
        ("Super League Final", stakes.STAGE_FINAL),
        ("Semi Final", stakes.STAGE_KNOCKOUT),
        ("Semi-Final", stakes.STAGE_KNOCKOUT),
        ("Quarter-final", stakes.STAGE_KNOCKOUT),
        ("Qualifier 1", stakes.STAGE_KNOCKOUT),
        ("Eliminator", stakes.STAGE_KNOCKOUT),
        ("Preliminary Final", stakes.STAGE_KNOCKOUT),
        ("3rd Place Play-Off", stakes.STAGE_KNOCKOUT),
        ("5th Place Play-Off Semi Final", stakes.STAGE_KNOCKOUT),
        ("Group Stage", stakes.STAGE_GROUP),
        ("Qualifying Group", stakes.STAGE_GROUP),
        ("Super Sixes", stakes.STAGE_GROUP),
        ("First Round", stakes.STAGE_GROUP),
        ("T20", stakes.STAGE_UNLABELLED),
        ("", stakes.STAGE_UNLABELLED),
    ],
)
def test_stage_vocabulary_reads_the_spellings_the_archive_uses(spelling: str, expected: str) -> None:
    """A round the archive names is labelled; a match type in the stage field is not."""
    assert stakes.stage_label_from_event_stage(spelling) == expected


def test_a_placing_playoff_is_a_knockout_and_not_the_final() -> None:
    """'3rd Place Play-Off' contains 'final' in no spelling but is still a knockout, and
    the label must not be 'final' -- the final is one match, and this is not it."""
    assert stakes.stage_label_from_event_stage("3rd Place Play-off") == stakes.STAGE_KNOCKOUT


# --- Editions ---------------------------------------------------------------------------


def test_editions_split_on_a_gap_longer_than_the_window() -> None:
    headers = [
        header("a", 0, "X", "Y"),
        header("b", 5, "X", "Y"),
        header("c", 5 + stakes.EDITION_GAP_DAYS + 1, "X", "Y"),
    ]

    found = stakes.editions(headers)

    assert [len(edition.headers) for edition in found] == [2, 1]


def test_a_match_with_no_competition_forms_no_edition_and_stays_unlabelled() -> None:
    headers = [header("a", 0, "X", "Y", competition=""), header("b", 1, "X", "Y", competition="")]

    assert stakes.derive(headers) == {}


# --- Bilateral series --------------------------------------------------------------------


def _series(results: List[Optional[str]]) -> List[stakes.Header]:
    """A five-match series between X and Y, one match a day, with the given winners."""
    return [header(f"m{i}", i, "X", "Y", winner=results[i], match_number=i + 1) for i in range(len(results))]


def test_a_bilateral_series_is_labelled_bilateral_on_every_match() -> None:
    derived = stakes.derive(_series(["X", "Y", "X", None, "X"]))

    assert {s.stage_label for s in derived.values()} == {stakes.STAGE_BILATERAL}


def test_a_series_becomes_a_dead_rubber_once_one_side_has_won_more_than_half() -> None:
    """Three of five wins settles a five-match series, so matches four and five are dead."""
    derived = stakes.derive(_series(["X", "X", "X", "Y", "Y"]))

    assert [derived[f"m{i}"].dead_rubber for i in range(5)] == [False, False, False, True, True]
    assert all(derived[f"m{i}"].dead_rubber_known for i in range(5))


def test_a_series_that_stays_level_never_becomes_a_dead_rubber() -> None:
    derived = stakes.derive(_series(["X", "Y", "X", "Y", "X"]))

    assert not any(derived[f"m{i}"].dead_rubber for i in range(5))


def test_a_series_reads_only_dates_strictly_before_the_fixture() -> None:
    """Two matches on one day: the second cannot see the first's result (H-21)."""
    headers = [
        header("m0", 0, "X", "Y", winner="X", match_number=1),
        header("m1", 1, "X", "Y", winner="X", match_number=2),
        header("m2", 1, "X", "Y", winner="X", match_number=3),
    ]

    derived = stakes.derive(headers)

    # After one win of three the series is not settled; after two it is, but m2 shares m1's
    # date and so still reads one win.
    assert [derived[f"m{i}"].dead_rubber for i in range(3)] == [False, False, False]


# --- League with playoffs ----------------------------------------------------------------


def _league() -> List[stakes.Header]:
    """Four clubs, a single round robin (six matches, one a day) and a final: two through.

    X wins its first two and Y its next two, so by the fifth fixture neither Z nor W can
    reach the top two and neither X nor Y can be caught -- which is what makes the last
    two group matches dead rubbers and the first four live.
    """
    return [
        header("g1", 0, "X", "W", winner="X", match_number=1),
        header("g2", 1, "Y", "Z", winner="Y", match_number=2),
        header("g3", 2, "X", "Z", winner="X", match_number=3),
        header("g4", 3, "Y", "W", winner="Y", match_number=4),
        header("g5", 4, "X", "Y", winner="X", match_number=5),
        header("g6", 5, "W", "Z", winner="Z", match_number=6),
        header("f1", 8, "X", "Y", winner="X", event_stage="Final"),
    ]


def test_a_league_labels_its_rounds_from_the_stage_and_its_pool_from_the_number() -> None:
    derived = stakes.derive(_league())

    assert derived["g1"].stage_label == stakes.STAGE_GROUP
    assert derived["f1"].stage_label == stakes.STAGE_FINAL
    assert derived["f1"].is_knockout
    assert not derived["g1"].is_knockout


def test_a_cut_that_takes_every_club_leaves_the_table_unreadable() -> None:
    """Four clubs and four semi-finalists decides nothing, so no match is flagged."""
    headers = _league()[:6] + [
        header("s1", 7, "X", "Y", winner="X", event_stage="Semi Final"),
        header("s2", 7, "Z", "W", winner="Z", event_stage="Semi Final"),
    ]

    derived = stakes.derive(headers)

    assert not any(entry.dead_rubber_known for entry in derived.values())


def test_a_league_flags_the_fixtures_neither_side_can_still_change() -> None:
    """From the fifth fixture the top two are settled and the bottom two are out, so the
    last two group matches are dead rubbers and the first four are live."""
    derived = stakes.derive(_league())

    assert [derived[f"g{i}"].dead_rubber for i in range(1, 7)] == [False, False, False, False, True, True]
    assert all(derived[f"g{i}"].dead_rubber_known for i in range(1, 7))


def test_a_knockout_match_is_known_and_never_a_dead_rubber() -> None:
    derived = stakes.derive(_league())

    assert derived["f1"].dead_rubber_known and not derived["f1"].dead_rubber


def test_a_league_without_a_knockout_round_has_no_reconstructible_table() -> None:
    """No knockout matches means no observable cut, so nothing is flagged -- the honest
    answer where the archive does not say how many clubs went through."""
    derived = stakes.derive(_league()[:6])

    assert not any(entry.dead_rubber_known for entry in derived.values())
    assert all(entry.stage_label == stakes.STAGE_GROUP for entry in derived.values())


def _four_club_pool(x_result: Optional[str]) -> List[stakes.Header]:
    """Four clubs, two through, a final. X's first two fixtures are the only thing that
    varies: ``"X"`` for two wins, ``None`` for two no-results."""
    return [
        header("m1", 0, "X", "Z", winner=x_result, match_number=1),
        header("m2", 0, "Y", "W", winner="Y", match_number=2),
        header("m3", 1, "X", "W", winner=x_result, match_number=3),
        header("m4", 1, "Y", "Z", winner="Y", match_number=4),
        header("m5", 2, "Z", "W", winner="Z", match_number=5),
        header("m6", 3, "X", "Y", winner="X", match_number=6),
        header("f1", 6, "X", "Y", winner="X", event_stage="Final"),
    ]


def test_two_wins_each_put_the_bottom_two_out_of_reach() -> None:
    """X and Y on four points with two through: Z and W, on zero with one game each, can
    reach two at most, so their fixture decides nothing."""
    derived = stakes.derive(_four_club_pool("X"))

    assert derived["m5"].dead_rubber


def test_a_no_result_is_a_point_each_and_keeps_the_bottom_two_alive() -> None:
    """The same two fixtures washed out leave X on two and Z and W on one apiece, so
    second place is still open and their fixture is live. A no result read as a loss would
    eliminate them both, which is the arithmetic this pins."""
    derived = stakes.derive(_four_club_pool(None))

    assert derived["m5"].dead_rubber_known and not derived["m5"].dead_rubber


def test_a_tournament_fixture_with_neither_a_number_nor_a_pool_stays_unlabelled() -> None:
    """Three clubs and no fixture number is a match the archive does not place, and an
    unplaced match is unlabelled rather than assumed to be a group game."""
    headers = [
        header("a", 0, "X", "Y", winner="X"),
        header("b", 1, "Y", "Z", winner="Y"),
        header("c", 2, "X", "Z", winner="X"),
    ]

    derived = stakes.derive(headers)

    assert {entry.stage_label for entry in derived.values()} == {stakes.STAGE_UNLABELLED}
    assert not any(entry.dead_rubber_known for entry in derived.values())


def _two_pool_league() -> List[stakes.Header]:
    """Two pools of three clubs, one through from each, then a final.

    Pool A (X, Y, Z) and pool B (P, Q, R) play their own round robins on the same days.
    X wins both of its matches, so its last-day pool fixture is already settled; pool B
    stays level to the end. Pooling the two tables into one would score X against clubs it
    never plays, which is the mistake this pins.
    """
    return [
        header("a1", 0, "X", "Y", winner="X", event_group="A", match_number=1),
        header("b1", 0, "P", "Q", winner="P", event_group="B", match_number=2),
        header("a2", 1, "X", "Z", winner="X", event_group="A", match_number=3),
        header("b2", 1, "Q", "R", winner="Q", event_group="B", match_number=4),
        header("a3", 2, "Y", "Z", winner="Y", event_group="A", match_number=5),
        header("b3", 2, "P", "R", winner="R", event_group="B", match_number=6),
        header("f1", 5, "X", "P", winner="X", event_stage="Final"),
    ]


def test_each_pool_gets_its_own_table() -> None:
    """One club through from each pool: X is safe in A by the last day, and nobody in B is."""
    derived = stakes.derive(_two_pool_league())

    assert derived["a3"].dead_rubber_known and derived["b3"].dead_rubber_known
    # a3 is Y v Z, and by then X has won both of its matches and cannot be caught.
    assert derived["a3"].dead_rubber
    # Pool B is level at the last fixture, so nothing there is settled.
    assert not derived["b3"].dead_rubber
    assert not derived["b1"].dead_rubber and not derived["b2"].dead_rubber


def test_a_pool_whose_cut_is_unreadable_is_left_unflagged_without_taking_the_others() -> None:
    """Pool B loses its finalist, so only pool A has an observable cut -- and pool A is
    still flagged rather than the whole edition going dark."""
    headers = [h for h in _two_pool_league() if h.match_id != "f1"]
    headers.append(header("f1", 5, "X", "Y", winner="X", event_stage="Final"))

    derived = stakes.derive(headers)

    assert derived["a3"].dead_rubber_known
    assert not derived["b3"].dead_rubber_known


# --- Coverage and the vocabulary report --------------------------------------------------


def test_coverage_counts_every_label_per_scope() -> None:
    headers = _league()
    derived = stakes.derive(headers)

    report = stakes.coverage(headers, derived)

    assert report["all"]["matches"] == 7
    assert report["all"]["label_group"] == 6
    assert report["all"]["knockout"] == 1
    assert report["T20/male"]["matches"] == 7


def test_vocabulary_reports_each_spelling_with_the_label_it_produced() -> None:
    report = stakes.vocabulary(_league())

    assert report["Final"] == {"matches": 1, "label": stakes.STAGE_FINAL}
    assert stakes.vocabulary(_league()[:6]) == {}


# --- What reaches the win row ------------------------------------------------------------


def test_stakes_columns_are_zero_for_a_match_with_no_stakes() -> None:
    """The serving path builds records without stakes; both columns read the unlabelled
    category rather than an implied group fixture."""
    match = make_match("m", 0, "A", xi("a"), xi("b"), make_deliveries(["a0"], ["b0"], [1], [0]))

    assert stakes_columns(match) == {"stakes_knockout": 0.0, "stakes_stage_known": 0.0}


def test_a_knockout_match_reaches_the_win_frame_as_a_flag() -> None:
    deliveries = make_deliveries(["a0"] * 6 + ["b0"] * 6, ["b5"] * 6 + ["a5"] * 6, [4] * 12, [0] * 12)
    deliveries.innings = np.array([0] * 6 + [1] * 6)
    matches = []
    for day in range(3):
        match = make_match(f"m{day}", day, "A", xi("a"), xi("b"), deliveries)
        match.stakes = stakes.MatchStakes(
            stage_label=stakes.STAGE_FINAL if day == 2 else stakes.STAGE_GROUP,
            dead_rubber=False,
            dead_rubber_known=True,
        )
        matches.append(match)

    result = build(ListSource(matches))

    assert set(C.STAKES_COLS) <= set(result.frame.columns)
    assert list(result.frame.stakes_knockout) == [0.0, 0.0, 1.0]
    assert list(result.frame.stakes_stage_known) == [1.0, 1.0, 1.0]
    assert result.quality.matches_with_stage_label == 3
    assert result.quality.knockout_matches == 1
    assert result.quality.matches_with_reconstructible_table == 3
    assert result.quality.dead_rubber_matches == 0


def test_the_display_model_does_not_read_the_stakes_columns_while_the_gate_is_a_null() -> None:
    """X-3's use 2 recorded a null, so the columns are in the frame and out of the model."""
    assert C.STAKES_FEATURES_KEPT is False
    assert not set(C.STAKES_COLS) & set(C.DISPLAY_FEATURE_COLS)
