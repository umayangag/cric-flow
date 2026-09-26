"""The competition level as a display-model context column (batch 5's first item).

Cricsheet's ``team_type`` -- ``international`` between national sides, ``club`` otherwise --
is on every match of the archive and on ``match.competition_level`` since migration 0022.
#356 measured the pooled formats' two levels discriminating differently on the display
model (ODI 0.73 international against 0.69 domestic, TEST 0.71 against 0.61) and showed
that splitting them would cost the domestic ODI rows 0.028 of display AUC, so the decision
was to keep the pooling and hand the display model the level as context. These tests pin
the shape that decision lands in: a display column the objective never reads, a value the
batting-order swap leaves alone, a serving path that reads a named level and averages an
unnamed one while saying so, and a parity count that would notice the two sources
recording it differently.
"""

from __future__ import annotations

from dataclasses import replace
from typing import get_args

import numpy as np
import pandas as pd
import pytest

from app.models.xi import CompetitionLevel
from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.display_regression import UNRECORDED_LEVEL
from ml.xi.parity import compare
from ml.xi.ratings import RatingState
from ml.xi.rows import competition_level_columns
from ml.xi.sources import SourceCounts
from ml.xi.store import FormatModels, XiStore
from ml.xi.train import (
    _xy,
    competition_level_breakdown,
    competition_level_variants,
    marginalised_probabilities,
    swap_orientation,
)
from tests.test_xi_data_quality import _CountingSource, _match
from tests.test_xi_train import synthetic_win_rows
from tests.xi_fixtures import xi

#: What the objective read in batch 4 (EVAL-13's 29 columns of full rank). The level is
#: not a function of the two elevens and must never reach this list: H-17's line and
#: ``objective_auc`` (EVAL-05) keep their meaning only while it does not.
OBJECTIVE_COLUMNS_IN_BATCH_4 = 29


# --- The contract ------------------------------------------------------------------------


def test_the_level_is_a_display_column_the_objective_never_reads() -> None:
    """The pin FEAT-05 set for the home flag and the toss, applied to the level: display
    only, and the objective's column list exactly as batch 4 fitted it."""
    assert C.COMPETITION_LEVEL_COL in C.DISPLAY_FEATURE_COLS
    assert C.COMPETITION_LEVEL_COL not in C.XI_FEATURE_COLS
    assert len(C.XI_FEATURE_COLS) == OBJECTIVE_COLUMNS_IN_BATCH_4
    assert all(column.startswith(("d_", "t1_", "t2_")) for column in C.XI_FEATURE_COLS)
    assert C.monotone_directions(C.COMPETITION_LEVEL_COLS, True) == [0], "the level has no direction on P(win)"


@pytest.mark.parametrize(
    "recorded,expected",
    [
        (C.COMPETITION_INTERNATIONAL, 1.0),
        (C.COMPETITION_CLUB, 0.0),
        ("", C.COMPETITION_LEVEL_UNKNOWN),
        ("franchise", C.COMPETITION_LEVEL_UNKNOWN),
    ],
)
def test_the_column_reads_the_two_words_and_nothing_else(recorded: str, expected: float) -> None:
    """1.0 between national sides, 0.0 otherwise, and the unrecorded category -- the
    toss's own shape -- for anything the source did not spell as one of the two."""
    assert C.competition_level_value(recorded) == expected
    match = replace(_match("m0", 0, xi("a"), xi("b")), competition_level=recorded)
    assert competition_level_columns(match) == {C.COMPETITION_LEVEL_COL: expected}


def test_the_request_spells_the_level_as_the_contract_does() -> None:
    """The route's literal and the contract's list are one vocabulary: a word that reaches
    the display model is a word go-app can send, and no other."""
    assert list(get_args(CompetitionLevel)) == C.COMPETITION_LEVELS
    assert set(C.COMPETITION_LEVEL_VALUES) == set(C.COMPETITION_LEVELS)


# --- The win row -------------------------------------------------------------------------


def test_the_pass_writes_the_recorded_level_on_the_win_row_and_counts_it() -> None:
    """Every decided match's row carries its level as the display model reads it, and the
    pass counts what it read per level -- zero-filled over all three keys, so the two
    sources always carry the same keys for ``make xi-parity`` to compare."""
    international = replace(_match("m0", 0, xi("a"), xi("b")), competition_level=C.COMPETITION_INTERNATIONAL)
    club = replace(_match("m1", 1, xi("a"), xi("b")), competition_level=C.COMPETITION_CLUB)
    unrecorded = _match("m2", 2, xi("a"), xi("b"))

    result = build(_CountingSource([international, club, unrecorded], SourceCounts(offered=3, yielded=3)))

    by_id = result.frame.set_index("match_id")[C.COMPETITION_LEVEL_COL]
    assert by_id["m0"] == 1.0 and by_id["m1"] == 0.0 and by_id["m2"] == C.COMPETITION_LEVEL_UNKNOWN
    assert result.quality.matches_read_by_level == {
        C.COMPETITION_INTERNATIONAL: 1,
        C.COMPETITION_CLUB: 1,
        UNRECORDED_LEVEL: 1,
    }


def test_parity_names_two_sources_that_record_the_level_differently() -> None:
    """A database migrated but not re-imported reads every match unrecorded while the
    archive reads its level; every other count agrees, and the display model would be
    fitted on different context. The count is compared, so the difference is named."""
    recorded = replace(_match("m0", 0, xi("a"), xi("b")), competition_level=C.COMPETITION_CLUB)
    counts = SourceCounts(offered=1, yielded=1)

    archive = build(_CountingSource([recorded], counts))
    database = build(_CountingSource([replace(recorded, competition_level="")], counts))

    differences = compare(database, archive)

    assert [line for line in differences if line.startswith("matches_read_by_level")] == [
        f"matches_read_by_level: postgres club=0 international=0 {UNRECORDED_LEVEL}=1, "
        f"cricsheet club=1 international=0 {UNRECORDED_LEVEL}=0"
    ]


# --- Scoring as served -------------------------------------------------------------------


def test_swapping_the_batting_order_leaves_the_level_where_it_is() -> None:
    """The toss reads 1 - x under the swap because the side that won it changes ends; the
    level is the fixture's, not a side's, and changes for neither batting order."""
    rows = synthetic_win_rows(6)
    rows[C.COMPETITION_LEVEL_COL] = [1.0, 0.0, 0.5] * 2

    swapped = swap_orientation(rows)

    assert list(swapped[C.COMPETITION_LEVEL_COL]) == [1.0, 0.0, 0.5] * 2


def test_the_variants_average_only_the_rows_with_no_recorded_level() -> None:
    """A recorded row is read at its own level in both variants; an unrecorded one is read
    at each level in turn, which is what the serving path answers a request that names
    none. A model that does not read the level gets the frame back untouched."""
    rows = synthetic_win_rows(3)
    rows[C.COMPETITION_LEVEL_COL] = [1.0, 0.0, C.COMPETITION_LEVEL_UNKNOWN]

    variants = competition_level_variants(rows, C.DISPLAY_FEATURE_COLS)

    assert [list(v[C.COMPETITION_LEVEL_COL]) for v in variants] == [[1.0, 0.0, 1.0], [1.0, 0.0, 0.0]]
    assert competition_level_variants(rows, C.XI_FEATURE_COLS) == [rows]


class _LevelReader:
    """A display model that answers 0.8 for an international fixture, 0.4 for a club one,
    and nothing else -- so what the served number and the harness read off it is exactly
    how each treats the level. Reads no other column, the toss included."""

    def __init__(self, columns):
        self.level = columns.index(C.COMPETITION_LEVEL_COL)

    def predict_proba(self, rows):
        p = np.where(rows[:, self.level] == 1.0, 0.8, 0.4)
        return np.column_stack([1.0 - p, p])


class _LevelScaledReader:
    """A display model whose answer is the sign of the first XI differential, scaled by
    the level: +0.3 for an international fixture, +0.1 for a club one. The batting-order
    swap negates the differential and leaves the level, so the served mean keeps the
    level's scale -- a level effect that did not interact with the sides would cancel in
    that mean, exactly as it should when nobody knows who bats first."""

    def __init__(self, columns):
        self.level = columns.index(C.COMPETITION_LEVEL_COL)
        self.differential = columns.index(C.XI_FEATURE_COLS[0])

    def predict_proba(self, rows):
        scale = np.where(rows[:, self.level] == 1.0, 0.3, 0.1)
        p = 0.5 + scale * np.sign(rows[:, self.differential])
        return np.column_stack([1.0 - p, p])


def test_the_harness_scores_a_recorded_row_at_its_level_and_an_unrecorded_one_over_both() -> None:
    """EVAL-05: the harness's number is the served one. An archive row records its level,
    as a request from go-app names it, so it is read there; a row with none is the request
    that names none, averaged over both."""
    rows = synthetic_win_rows(3)
    rows[C.XI_FEATURE_COLS[0]] = 1.0
    rows[C.COMPETITION_LEVEL_COL] = [1.0, 0.0, C.COMPETITION_LEVEL_UNKNOWN]

    served = marginalised_probabilities(_LevelScaledReader(C.DISPLAY_FEATURE_COLS), rows, C.DISPLAY_FEATURE_COLS)

    np.testing.assert_allclose(served, [0.8, 0.6, 0.7])


def _level_store() -> XiStore:
    state = RatingState()
    state.reserve_read_capacity()
    models = FormatModels(
        format_code="ODI",
        objective=None,
        display=_LevelReader(C.DISPLAY_FEATURE_COLS),
        objective_cols=list(C.XI_FEATURE_COLS),
        display_cols=list(C.DISPLAY_FEATURE_COLS),
        metadata={},
    )
    return XiStore(state, {"ODI": models})


def test_display_probability_reads_the_named_level_and_averages_over_none() -> None:
    """The serving half of the previous test: a named level is read, an unnamed one is
    averaged over both answers -- never read at the unrecorded value the model was not
    fitted on, which would answer 0.4 here. The batting order is named so the reading is
    one orientation's, not the mean of p and 1 - p."""
    store = _level_store()
    a, b = xi("a"), xi("b")

    def served(level):
        return store.display_probability("ODI", a, b, team1_bats_first=True, competition_level=level)

    assert served(C.COMPETITION_INTERNATIONAL) == pytest.approx(0.8)
    assert served(C.COMPETITION_CLUB) == pytest.approx(0.4)
    assert served(None) == pytest.approx(0.6)


def test_display_probability_refuses_a_level_it_does_not_know() -> None:
    """The route refuses an unknown word at the boundary (422); the store refuses it too,
    so no caller inside the service can read the model at a level nobody declared."""
    with pytest.raises(ValueError, match="franchise"):
        _level_store().display_probability("ODI", xi("a"), xi("b"), competition_level="franchise")


# --- The breakdown the manifest and the harness report ------------------------------------


def test_the_breakdown_scores_each_recorded_level_and_counts_the_rest() -> None:
    """One entry per level with the row count, both AUCs where twenty rows of both classes
    exist, and the unrecorded rows under their own named key -- the table #356 read the
    pooling decision off, on every run."""
    rows = synthetic_win_rows(90)
    rows["competition_level"] = [C.COMPETITION_INTERNATIONAL] * 40 + [C.COMPETITION_CLUB] * 40 + [""] * 10
    rows[C.COMPETITION_LEVEL_COL] = rows["competition_level"].map(C.competition_level_value)

    class _Signal:
        """Ranks by the first display column, which is what the label follows."""

        def predict_proba(self, x):
            p = 1.0 / (1.0 + np.exp(-x[:, 0]))
            return np.column_stack([1.0 - p, p])

    breakdown = competition_level_breakdown(_Signal(), _Signal(), rows, rows_key="n_eval")

    assert set(breakdown) == {C.COMPETITION_INTERNATIONAL, C.COMPETITION_CLUB, UNRECORDED_LEVEL}
    assert breakdown[C.COMPETITION_INTERNATIONAL]["n_eval"] == 40 and breakdown[C.COMPETITION_CLUB]["n_eval"] == 40
    assert 0.5 < breakdown[C.COMPETITION_INTERNATIONAL]["display_auc"] <= 1.0
    assert breakdown[UNRECORDED_LEVEL] == {"n_eval": 10}, "ten rows is too few to score"


def test_the_breakdown_reads_a_frame_without_the_word_column_as_unrecorded() -> None:
    """A synthetic frame carries the numeric column and not the word; every row is then
    the unrecorded level, as ``display_regression.development_rows_by_level`` counts it."""
    rows = synthetic_win_rows(25)

    class _Coin:
        def predict_proba(self, x):
            return np.full((len(x), 2), 0.5)

    breakdown = competition_level_breakdown(_Coin(), _Coin(), rows)

    assert list(breakdown) == [UNRECORDED_LEVEL] and breakdown[UNRECORDED_LEVEL]["n_holdout"] == 25


def test_a_fitted_display_model_reads_the_level_and_the_objective_design_does_not() -> None:
    """End to end on the contract's column lists: the design matrix the objective is fitted
    on has no level column, and the display model's has exactly one."""
    rows = synthetic_win_rows(30)

    x_objective, _ = _xy(rows, C.XI_FEATURE_COLS)
    x_display, _ = _xy(rows, C.DISPLAY_FEATURE_COLS)

    assert x_objective.shape[1] == OBJECTIVE_COLUMNS_IN_BATCH_4
    assert x_display.shape[1] == len(C.XI_FEATURE_COLS) + len(C.TEAM_CONTEXT_COLS) + len(C.TOSS_COLS) + 1
    assert pd.Index(C.DISPLAY_FEATURE_COLS).get_loc(C.COMPETITION_LEVEL_COL) >= len(C.XI_FEATURE_COLS)
