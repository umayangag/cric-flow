"""IMPORT-06 on the rating pass: a wicket is what the vocabulary says it is.

A run out is a wicket the innings lost and not the bowler's; a batter retired hurt is
neither, and is nobody's dismissal; a delivery with two wickets is read whole. The
vocabulary is ``configs/wicket_kinds.json``, the file the go-app importer reads too.
"""

from __future__ import annotations

import json
from typing import List, Tuple

import numpy as np
import pytest

from ml.xi.rows import innings_outcomes, match_actuals
from ml.xi.sources import _deliveries_from_cricsheet, _deliveries_from_rows, wicket_columns
from ml.xi.wicketkinds import ENV_VAR, FILE_NAME, Vocabulary, load
from tests.xi_fixtures import make_deliveries, make_match, xi

# --- the vocabulary ----------------------------------------------------------------------

# Every kind the archive uses as of 2026-09-13, with what the scorecard makes of it:
# (is a wicket lost, is the bowler's).
ARCHIVE_KINDS: List[Tuple[str, bool, bool]] = [
    ("bowled", True, True),
    ("caught", True, True),
    ("caught and bowled", True, True),
    ("hit wicket", True, True),
    ("lbw", True, True),
    ("stumped", True, True),
    ("run out", True, False),
    ("retired out", True, False),
    ("obstructing the field", True, False),
    ("handled the ball", True, False),
    ("hit the ball twice", True, False),
    ("timed out", True, False),
    ("retired hurt", False, False),
    ("retired not out", False, False),
]


@pytest.mark.parametrize("kind,is_dismissal,credited", ARCHIVE_KINDS)
def test_the_committed_vocabulary_classifies_every_kind_the_archive_uses(
    kind: str, is_dismissal: bool, credited: bool
) -> None:
    """The go-app importer credits and counts by this same file; a kind read differently
    on one side would be the divergence make xi-parity exists to catch."""
    vocabulary = load()

    assert vocabulary.is_dismissal(kind) is is_dismissal
    assert vocabulary.is_credited_to_bowler(kind) is credited


def test_case_and_space_are_not_a_different_kind() -> None:
    assert load().is_dismissal("  Run Out ") is True


def test_a_kind_the_vocabulary_does_not_know_is_an_error_not_a_guess() -> None:
    with pytest.raises(ValueError, match="mankaded"):
        load().is_dismissal("mankaded")


@pytest.mark.parametrize(
    "lists,message",
    [
        (({"caught"}, {"caught"}, {"retired hurt"}), "listed twice"),
        (({"caught"}, {"run out"}, set()), "is empty"),
        (({"Caught"}, {"run out"}, {"retired hurt"}), "spelling"),
    ],
)
def test_a_vocabulary_that_cannot_classify_is_rejected(lists, message: str) -> None:
    credited, not_credited, not_out = lists
    with pytest.raises(ValueError, match=message):
        Vocabulary(frozenset(credited), frozenset(not_credited), frozenset(not_out))


def test_load_reads_the_file_named_by_the_environment(tmp_path, monkeypatch) -> None:
    path = tmp_path / FILE_NAME
    path.write_text(
        json.dumps(
            {"credited_to_bowler": ["bowled"], "dismissal_not_credited": ["run out"], "not_out": ["retired hurt"]}
        )
    )
    monkeypatch.setenv(ENV_VAR, str(path))

    vocabulary = load()

    assert vocabulary.credited_to_bowler == frozenset({"bowled"})


def test_a_missing_vocabulary_is_an_error(tmp_path) -> None:
    """Unlike the team lineage: nothing can say what a run out is to the bowler without it."""
    with pytest.raises(FileNotFoundError):
        load(str(tmp_path / "absent.json"))


# --- the one rule for both sources --------------------------------------------------------


@pytest.mark.parametrize(
    "wickets,want_wicket,want_bowler,want_stumping,want_out",
    [
        ([], 0.0, 0.0, 0.0, []),
        ([("caught", "a1")], 1.0, 1.0, 0.0, ["a1"]),
        ([("stumped", "a1")], 1.0, 1.0, 1.0, ["a1"]),
        ([("run out", "a2")], 1.0, 0.0, 0.0, ["a2"]),
        ([("retired out", "a1")], 1.0, 0.0, 0.0, ["a1"]),
        ([("retired hurt", "a1")], 0.0, 0.0, 0.0, []),
        ([("retired not out", "a1")], 0.0, 0.0, 0.0, []),
        ([("bowled", "a1"), ("run out", "a2")], 2.0, 1.0, 0.0, ["a1", "a2"]),
        ([("caught", "a1"), ("retired hurt", "a2")], 1.0, 1.0, 0.0, ["a1"]),
        ([("run out", "")], 1.0, 0.0, 0.0, []),
    ],
)
def test_wicket_columns_read_a_ball_the_way_the_scorecard_does(
    wickets, want_wicket: float, want_bowler: float, want_stumping: float, want_out
) -> None:
    wicket, bowler_wicket, stumping, players_out = wicket_columns([wickets])

    assert list(wicket) == [want_wicket]
    assert list(bowler_wicket) == [want_bowler]
    assert list(stumping) == [want_stumping]
    assert players_out == [want_out]


def _ball(batter: str, non_striker: str, wickets: list) -> dict:
    return {
        "batter": batter,
        "bowler": "B1",
        "non_striker": non_striker,
        "runs": {"batter": 0, "extras": 0, "total": 0},
        "wickets": wickets,
    }


# One over of B1's: A1 caught, A2 run out from the non-striker's end, A4 retired hurt,
# A5 bowled while A6 is run out on the same ball, A7 retired out -- six wicket records,
# five wickets lost, two of them B1's. The go-app fixture wicketsMatchJSON is the same over.
INNINGS = [
    {
        "overs": [
            {
                "over": 0,
                "deliveries": [
                    _ball("A1", "A2", [{"player_out": "A1", "kind": "caught", "fielders": [{"name": "B2"}]}]),
                    _ball("A3", "A2", [{"player_out": "A2", "kind": "run out", "fielders": [{"name": "B3"}]}]),
                    _ball("A4", "A3", [{"player_out": "A4", "kind": "retired hurt"}]),
                    _ball(
                        "A5", "A6", [{"player_out": "A5", "kind": "bowled"}, {"player_out": "A6", "kind": "run out"}]
                    ),
                    _ball("A7", "A3", [{"player_out": "A7", "kind": "retired out"}]),
                    _ball("A8", "A3", []),
                ],
            }
        ]
    }
]
REGISTRY = {f"A{i}": f"a{i}" for i in range(1, 9)}
REGISTRY.update({"B1": "b1", "B2": "b2", "B3": "b3"})


def test_the_archive_path_reads_every_wicket_by_the_vocabulary() -> None:
    d = _deliveries_from_cricsheet(INNINGS, REGISTRY)

    assert list(d.wicket) == [1.0, 1.0, 0.0, 2.0, 1.0, 0.0]
    assert list(d.bowler_wicket) == [1.0, 0.0, 0.0, 1.0, 0.0, 0.0]
    assert d.players_out == [["a1"], ["a2"], [], ["a5", "a6"], ["a7"], []]


def test_the_postgres_path_reads_every_wicket_by_the_vocabulary() -> None:
    """The rows carry the ball's wickets as two arrays in wicket order -- kinds and keys
    from ball_event_wicket -- and read the same as the archive path ball for ball."""

    def row(kinds, keys):
        return (1, 0, "a1", "b1", 0, 0, kinds, None, keys, 0, 0, 0, 0)

    d = _deliveries_from_rows(
        [
            row(["caught"], ["a1"]),
            row(["run out"], ["a2"]),
            row(["retired hurt"], ["a4"]),
            row(["bowled", "run out"], ["a5", "a6"]),
            row(["retired out"], ["a7"]),
            row(None, None),
        ]
    )

    assert list(d.wicket) == [1.0, 1.0, 0.0, 2.0, 1.0, 0.0]
    assert list(d.bowler_wicket) == [1.0, 0.0, 0.0, 1.0, 0.0, 0.0]
    assert d.players_out == [["a1"], ["a2"], [], ["a5", "a6"], ["a7"], []]


def test_a_dismissed_player_the_database_could_not_key_is_a_wicket_and_nobodys_dismissal() -> None:
    d = _deliveries_from_rows([(1, 0, "a1", "b1", 0, 0, ["run out"], None, [None], 0, 0, 0, 0)])

    assert list(d.wicket) == [1.0]
    assert d.players_out == [[]]


# --- the targets ------------------------------------------------------------------------


def _over_of_b1():
    return make_match("m", 0, "B", xi("a"), xi("b"), _deliveries_from_cricsheet(INNINGS, REGISTRY))


def test_the_dismissals_target_counts_the_innings_a_batter_was_out_in() -> None:
    """A2, run out at the non-striker's end, and A6, the second wicket of a delivery, were
    out; A4 retired hurt and was not."""
    actuals = match_actuals(_over_of_b1())

    assert actuals["a1"]["dismissals"] == 1.0
    assert actuals["a2"]["dismissals"] == 1.0
    assert actuals["a4"]["dismissals"] == 0.0
    assert actuals["a6"]["dismissals"] == 1.0
    assert actuals["a7"]["dismissals"] == 1.0


def test_the_wickets_target_is_the_bowlers_column() -> None:
    """The catch and the bowled are B1's; the run outs and the retirements are not."""
    actuals = match_actuals(_over_of_b1())

    assert actuals["b1"]["wickets"] == 2.0


def test_innings_wickets_count_both_wickets_of_a_delivery_and_not_the_retirement() -> None:
    outcomes = innings_outcomes(_over_of_b1())

    assert outcomes["innings1_wickets"] == 5.0


def test_a_hand_made_deliveries_with_no_players_out_records_no_dismissals() -> None:
    """The fixture shape that predates players_out: the wicket flag alone is not a
    dismissal against anyone, which is what keeps the older tests honest."""
    d = make_deliveries(["a1"], ["b1"], [0], [1])
    d.players_out = []

    actuals = match_actuals(make_match("m", 0, "B", xi("a"), xi("b"), d))

    assert actuals["a1"]["dismissals"] == 0.0
    assert np.isclose(actuals["b1"]["wickets"], 1.0)
