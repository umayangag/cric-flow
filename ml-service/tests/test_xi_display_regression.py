"""Unit tests for display-regression (ml.xi.display_regression): a display-AUC fall
against the previous accepted run, judged only on the same population.

The retrodiction cases use the numbers the batch records carry (docs/AUDIT_FINDINGS.md,
batches 2-4): batch 4's T20 fall of 3.24 fold sd on IMPORT-09's taxonomy change must
re-baseline, the same fall on unchanged rows must fail, and ODI / TEST's moves of a
twentieth of a fold sd must pass quietly.
"""

from __future__ import annotations

from typing import Dict, Optional

import pandas as pd
import pytest

from ml.xi import display_regression as dr

WINDOWS = dr.windows(["2024-01-01", "2024-04-01", "2024-07-01"], "2026-09-02")

# Batch 3 (harness of 2026-09-20) and batch 4 (2026-09-25), walk-forward display AUC with
# the fold sd on the after column, and the decided development rows per level. Batch 3's
# T20 held 3,825 decided internationals that IMPORT-09 moved into T20I.
BATCH3_T20_ROWS = {"club": 8305, "international": 3825}
BATCH4_T20_ROWS = {"club": 8305}
ODI_ROWS = {"club": 1483, "international": 3512}
TEST_ROWS = {"club": 1342, "international": 753}


def _reference(
    mean: Optional[float],
    sd: Optional[float] = 0.04,
    rows: Optional[Dict[str, int]] = None,
    n_folds: int = 11,
    generated_at: str = "2026-09-20T00:00:00+00:00",
    windows: Optional[Dict] = None,
) -> dr.Reference:
    display = None if mean is None else {"mean": mean, "sd": sd, "n_folds": n_folds, "gates_consulted": 30}
    return dr.Reference(
        generated_at=generated_at,
        display_auc=display,
        development_rows_by_level=rows,
        windows=WINDOWS if windows is None else windows,
    )


# --- Retrodiction on the recorded batches ------------------------------------------------


def test_batch4_t20_fall_on_a_taxonomy_change_rebaselines_and_prints_the_move() -> None:
    """Batch 3 -> 4, T20: 0.7294 -> 0.5934 at fold sd 0.0419 is -3.24 sd, and IMPORT-09
    moved 3,825 of 12,130 rows out of the format. The false positive this gate exists to
    avoid: a re-baseline that says why, with the fall printed beside it, never a failure."""
    baseline = _reference(0.7294, 0.0481, BATCH3_T20_ROWS)
    current = _reference(0.5934, 0.0419, BATCH4_T20_ROWS, generated_at="2026-09-25T00:00:00+00:00")

    node = dr.decide(current, baseline)

    assert node["verdict"] == dr.VERDICT_REBASELINED
    assert node["display_auc_move_in_fold_sd"] == pytest.approx(-3.25, abs=0.01)
    assert "-3.25 fold sd" in node["reason"]
    assert "international 3,825 -> 0 (31.5% of the previous run's 12,130 rows)" in node["reason"]
    assert node["level_moves"]["international"] == {
        "before": 3825,
        "after": 0,
        "share_of_format": pytest.approx(0.3153, abs=1e-3),
    }
    assert node["level_moves"]["club"]["share_of_format"] == 0.0
    assert node["baseline"] == current.as_dict(), "the new population is the reference from here on"


def test_the_same_fall_on_unchanged_rows_fails() -> None:
    """The regression the gate is for: batch 4's number on batch 3's population."""
    baseline = _reference(0.7294, 0.0481, BATCH3_T20_ROWS)
    current = _reference(0.5934, 0.0419, BATCH3_T20_ROWS, generated_at="2026-09-25T00:00:00+00:00")

    node = dr.decide(current, baseline)

    assert node["verdict"] == dr.VERDICT_FAIL
    assert node["display_auc_move_in_fold_sd"] == pytest.approx(-3.25, abs=0.01)
    assert node["reason"].endswith("the model got worse, not the population")
    assert "0.5934 against the previous accepted run's 0.7294 (2026-09-20T00:00:00+00:00)" in node["reason"]
    assert node["baseline"] == baseline.as_dict(), "a failed run does not become the reference"


@pytest.mark.parametrize(
    "before, after, sd, rows",
    [
        # batch 3 -> 4
        (0.7074, 0.7108, 0.0757, ODI_ROWS),
        (0.6331, 0.6400, 0.1041, TEST_ROWS),
        # batch 2 -> 3
        (0.7110, 0.7074, 0.0739, ODI_ROWS),
        (0.6286, 0.6331, 0.0977, TEST_ROWS),
        (0.7295, 0.7294, 0.0481, BATCH3_T20_ROWS),
        (0.7618, 0.7624, 0.0485, {"international": 2073}),
    ],
)
def test_the_recorded_odi_test_and_batch2_to_3_moves_pass_quietly(before: float, after: float, sd: float, rows) -> None:
    """Moves of 0.00-0.07 fold sd on unchanged rows: passes, with the move printed."""
    node = dr.decide(_reference(after, sd, rows), _reference(before, sd, rows))

    assert node["verdict"] == dr.VERDICT_PASS
    assert abs(node["display_auc_move_in_fold_sd"]) < 0.1
    assert node["reason"].endswith("within one fold sd on the same windows and the same per-level rows")


def test_t20i_batch3_to_4_rebaselines_on_the_rows_it_gained_even_though_it_rose() -> None:
    """T20I received the 3,825 rows: 2,073 -> 5,898 is 185 % of its previous rows. A rise
    never fails, and the population is still recorded as changed."""
    node = dr.decide(
        _reference(0.8142, 0.0352, {"international": 5898}, n_folds=11),
        _reference(0.7624, 0.0485, {"international": 2073}, n_folds=10),
    )

    assert node["verdict"] == dr.VERDICT_REBASELINED
    assert node["display_auc_move_in_fold_sd"] > 1.0
    assert "fold count differs (10 folds before, 11 now)" in node["reason"]


# --- What "materially unchanged" means ----------------------------------------------------


@pytest.mark.parametrize("extra_rows", [0, 2, 25, 415])
def test_ordinary_archive_drift_under_five_percent_still_decides(extra_rows: int) -> None:
    """IMPORT-08's fold changed two rows; a quarter's corrections a handful more. Up to 5 %
    of the format's rows the comparison is like-for-like and a genuine fall fails."""
    baseline = _reference(0.70, 0.04, BATCH4_T20_ROWS)
    current = _reference(0.70 - 1.2 * 0.04, 0.04, {"club": 8305 + extra_rows})

    node = dr.decide(current, baseline)

    assert node["verdict"] == dr.VERDICT_FAIL
    assert node["level_moves"]["club"]["share_of_format"] <= dr.MAX_LEVEL_MOVE_SHARE


def test_a_level_moving_past_five_percent_of_the_format_rebaselines() -> None:
    baseline = _reference(0.70, 0.04, BATCH4_T20_ROWS)
    current = _reference(0.70 - 1.2 * 0.04, 0.04, {"club": 8305 + 416})

    node = dr.decide(current, baseline)

    assert node["verdict"] == dr.VERDICT_REBASELINED
    assert "club 8,305 -> 8,721 (5.0% of the previous run's 8,305 rows)" in node["reason"]


def test_a_level_appearing_from_nothing_counts_as_a_move_from_zero() -> None:
    node = dr.decide(
        _reference(0.70, 0.04, {"club": 8305, "international": 900}), _reference(0.70, 0.04, BATCH4_T20_ROWS)
    )

    assert node["verdict"] == dr.VERDICT_REBASELINED
    assert node["level_moves"]["international"] == {
        "before": 0,
        "after": 900,
        "share_of_format": pytest.approx(900 / 8305),
    }


def test_a_previous_run_with_no_rows_has_no_share_to_read() -> None:
    """Dividing by nothing is not a share; the change reads None and is material."""
    moves = dr.level_moves({}, {"club": 10})

    assert moves == {"club": {"before": 0, "after": 10, "share_of_format": None}}
    assert (
        dr.decide(_reference(0.70, 0.04, {"club": 10}), _reference(0.70, 0.04, {}))["verdict"] == dr.VERDICT_REBASELINED
    )


def test_moved_windows_rebaseline_before_any_count_is_read() -> None:
    """A rotation (A-4) adds a season, 5-15 % of a format's rows, and scores different
    matches: the windows check catches it whatever the counts say."""
    rotated = dr.windows(["2024-04-01", "2024-07-01", "2024-10-01"], "2027-09-02")
    node = dr.decide(_reference(0.60, 0.04, BATCH4_T20_ROWS, windows=rotated), _reference(0.70, 0.04, BATCH4_T20_ROWS))

    assert node["verdict"] == dr.VERDICT_REBASELINED
    assert "the walk-forward windows moved" in node["reason"]
    assert node["level_moves"] is None


def test_a_baseline_without_row_counts_rebaselines_and_still_prints_the_move() -> None:
    """The report on disk before this gate landed carries display numbers and no counts:
    the move is visible, the comparison is not shown like-for-like, and the run re-baselines."""
    node = dr.decide(_reference(0.5934, 0.0419, BATCH4_T20_ROWS), _reference(0.7294, 0.0481, rows=None))

    assert node["verdict"] == dr.VERDICT_REBASELINED
    assert node["display_auc_move_in_fold_sd"] == pytest.approx(-3.25, abs=0.01)
    assert "carries no per-level row counts" in node["reason"]


# --- When the gate cannot decide ------------------------------------------------------------


def test_no_previous_accepted_run_is_undecided_and_this_run_is_the_baseline() -> None:
    current = _reference(0.70, 0.04, BATCH4_T20_ROWS)

    node = dr.decide(current, None)

    assert node["verdict"] == dr.VERDICT_UNDECIDED
    assert node["reason"].startswith("no previous accepted harness report to compare against")
    assert node["compared_against"] is None and node["display_auc_move_in_fold_sd"] is None
    assert node["baseline"] == current.as_dict()


def test_no_display_number_this_run_is_undecided() -> None:
    node = dr.decide(_reference(None, rows=BATCH4_T20_ROWS), _reference(0.70, 0.04, BATCH4_T20_ROWS))

    assert node["verdict"] == dr.VERDICT_UNDECIDED
    assert node["reason"] == "no walk-forward display AUC this run, so there is nothing to compare"


def test_no_display_number_on_the_previous_run_is_undecided() -> None:
    node = dr.decide(_reference(0.70, 0.04, BATCH4_T20_ROWS), _reference(None, rows=BATCH4_T20_ROWS))

    assert node["verdict"] == dr.VERDICT_UNDECIDED
    assert "carries no walk-forward display AUC for this format" in node["reason"]


def test_a_single_fold_has_no_spread_to_read_a_move_against() -> None:
    node = dr.decide(
        _reference(0.60, 0.0, BATCH4_T20_ROWS, n_folds=1), _reference(0.70, 0.0, BATCH4_T20_ROWS, n_folds=1)
    )

    assert node["verdict"] == dr.VERDICT_UNDECIDED
    assert "no spread" in node["reason"]


# --- The previous accepted run: resolved from the report the run overwrites ------------------


def _previous_report(passed: bool, carried_baseline: Optional[Dict] = None, with_counts: bool = True) -> Dict:
    node: Dict = {"walk_forward": {"summary": {"display_auc": {"mean": 0.7294, "sd": 0.0481, "n_folds": 11}}}}
    node["display_regression"] = {
        "current": {"development_rows_by_level": BATCH3_T20_ROWS} if with_counts else {},
        "baseline": carried_baseline,
    }
    return {
        "generated_at": "2026-09-20T00:00:00+00:00",
        "cutoffs": WINDOWS["cutoffs"],
        "locked_start": WINDOWS["locked_start"],
        "formats": {"T20": node},
        "gates": {"passed": passed},
    }


def test_an_accepted_previous_report_is_read_for_its_own_numbers() -> None:
    baseline = dr.baseline_for(_previous_report(passed=True), "T20")

    assert baseline == dr.Reference(
        generated_at="2026-09-20T00:00:00+00:00",
        display_auc={"mean": 0.7294, "sd": 0.0481, "n_folds": 11},
        development_rows_by_level=BATCH3_T20_ROWS,
        windows=WINDOWS,
    )
    assert baseline is not None and baseline.display_auc == {"mean": 0.7294, "sd": 0.0481, "n_folds": 11}


def test_a_previous_report_that_did_not_pass_hands_over_the_baseline_it_carried() -> None:
    """One failure does not erase the reference: the failed run compared against the last
    accepted one and carries it, so a re-run is judged against the same numbers."""
    carried = _reference(0.7100, 0.05, BATCH3_T20_ROWS, generated_at="2026-09-14T00:00:00+00:00").as_dict()

    baseline = dr.baseline_for(_previous_report(passed=False, carried_baseline=carried), "T20")

    assert baseline == dr.Reference(**carried)


def test_a_previous_report_that_did_not_pass_and_carried_nothing_gives_no_baseline() -> None:
    assert dr.baseline_for(_previous_report(passed=False), "T20") is None


def test_an_accepted_report_written_before_the_gate_has_no_counts() -> None:
    baseline = dr.baseline_for(_previous_report(passed=True, with_counts=False), "T20")

    assert baseline is not None and baseline.development_rows_by_level is None


@pytest.mark.parametrize("previous", [None, {"formats": {}, "gates": {"passed": True}}])
def test_no_previous_report_or_no_format_in_it_gives_no_baseline(previous) -> None:
    assert dr.baseline_for(previous, "T20") is None


def test_a_pass_carries_the_reference_it_was_judged_against() -> None:
    """A run this gate passes may still fail another gate; the next run then reads the
    baseline this one carried, which is the previous accepted run, not this one."""
    baseline = _reference(0.70, 0.04, BATCH4_T20_ROWS)

    node = dr.decide(_reference(0.71, 0.04, BATCH4_T20_ROWS, generated_at="later"), baseline)

    assert node["verdict"] == dr.VERDICT_PASS
    assert node["baseline"] == baseline.as_dict()


# --- Counting the rows a fold can read --------------------------------------------------------


def test_development_rows_are_counted_per_level_before_the_locked_start() -> None:
    frame = pd.DataFrame(
        {
            "match_date": pd.to_datetime(["2024-01-05", "2024-02-05", "2025-03-05", "2026-09-02", "2026-10-01"]),
            "competition_level": ["club", "international", "club", "club", "international"],
        }
    )

    assert dr.development_rows_by_level(frame, "2026-09-02") == {"club": 2, "international": 1}


def test_rows_with_no_recorded_level_count_under_a_named_key() -> None:
    frame = pd.DataFrame({"match_date": pd.to_datetime(["2024-01-05", "2024-02-05"]), "competition_level": ["", None]})

    assert dr.development_rows_by_level(frame, "2026-09-02") == {dr.UNRECORDED_LEVEL: 2}


def test_a_frame_without_the_column_counts_every_row_as_unrecorded() -> None:
    frame = pd.DataFrame({"match_date": pd.to_datetime(["2024-01-05"])})

    assert dr.development_rows_by_level(frame, "2026-09-02") == {dr.UNRECORDED_LEVEL: 1}


def test_a_reference_round_trips_through_its_dict() -> None:
    reference = _reference(0.70, 0.04, BATCH4_T20_ROWS)

    assert dr.Reference(**reference.as_dict()) == reference
