"""Unit tests for the simulator harness's walk-forward summary (ml.xi.sim_harness).

B-12: a fold whose calibration window was too thin to fit the shared match factor ships a
different simulator, and the summary used to average its intervals in with everyone else's
and say nothing. These tests hold the summary to saying it.
"""

from __future__ import annotations

from typing import Any, Dict, Optional

import pytest

from ml.xi import glossary, sim_harness


def _fold(coverage: float, width: float, shared_factor: Optional[Dict[str, Any]]) -> Dict[str, Any]:
    """One window's simulation report, cut down to what the summary reads."""
    return {
        "n_matches": 100,
        "calibration": {"runs_balls_rho": 0.5, "shared_factor": shared_factor},
        "win": {"brier": {"display": 0.20, "simulated": 0.21}},
        "totals": {
            "first_innings": {"coverage_80": coverage, "width_80": width},
            "chase": {"coverage_80": coverage - 0.05, "width_80": width - 10.0},
        },
    }


_CALIBRATED = {"sd": 0.12, "n_matches": 60}


def test_summarize_folds_is_none_when_no_window_simulated() -> None:
    assert (
        sim_harness.summarize_folds({"2025-01-01": None, "2025-04-01": {"skipped_reason": "too few matches"}}) is None
    )


def test_summary_counts_and_names_the_windows_that_shipped_without_a_shared_factor() -> None:
    folds = {
        "2025-01-01": _fold(0.78, 150.0, _CALIBRATED),
        "2025-04-01": _fold(0.76, 148.0, _CALIBRATED),
        "2025-07-01": _fold(0.50, 105.0, None),
    }

    summary = sim_harness.summarize_folds(folds)

    assert summary["shared_factor_folds"]["folds_scored"] == 3
    assert summary["shared_factor_folds"]["with_shared_factor"] == 2
    assert summary["shared_factor_folds"]["without_shared_factor"] == 1
    assert summary["shared_factor_folds"]["windows_without_shared_factor"] == ["2025-07-01"]


def test_the_pooled_totals_average_every_fold_and_the_split_holds_the_factorless_one_out() -> None:
    """The defect and its correction in one assertion: both figures are published."""
    folds = {
        "2025-01-01": _fold(0.78, 150.0, _CALIBRATED),
        "2025-04-01": _fold(0.76, 148.0, _CALIBRATED),
        "2025-07-01": _fold(0.50, 105.0, None),
    }

    summary = sim_harness.summarize_folds(folds)

    pooled = summary["totals"]["first_innings"]
    calibrated = summary["shared_factor_folds"]["totals_with_shared_factor"]["first_innings"]
    assert pooled["coverage_80"]["mean"] == pytest.approx(0.68)
    assert pooled["coverage_80"]["n_folds"] == 3
    assert calibrated["coverage_80"]["mean"] == pytest.approx(0.77)
    assert calibrated["coverage_80"]["n_folds"] == 2
    assert calibrated["width_80"]["mean"] == pytest.approx(149.0)


def test_the_chase_is_split_the_same_way_as_the_first_innings() -> None:
    """A factorless fold ships without a chase response either, so both totals are affected."""
    folds = {"2025-01-01": _fold(0.78, 150.0, _CALIBRATED), "2025-07-01": _fold(0.50, 105.0, None)}

    summary = sim_harness.summarize_folds(folds)

    chase = summary["shared_factor_folds"]["totals_with_shared_factor"]["chase"]
    assert chase["coverage_80"]["mean"] == pytest.approx(0.73)
    assert summary["totals"]["chase"]["coverage_80"]["mean"] == pytest.approx(0.59)


def test_a_run_whose_every_fold_had_a_factor_names_no_window_and_repeats_the_totals() -> None:
    folds = {"2025-01-01": _fold(0.78, 150.0, _CALIBRATED), "2025-04-01": _fold(0.76, 148.0, _CALIBRATED)}

    summary = sim_harness.summarize_folds(folds)

    split = summary["shared_factor_folds"]
    assert split["without_shared_factor"] == 0
    assert split["windows_without_shared_factor"] == []
    assert split["totals_with_shared_factor"] == summary["totals"]


def test_a_run_whose_every_fold_was_factorless_still_reports_the_pooled_totals() -> None:
    summary = sim_harness.summarize_folds({"2025-01-01": _fold(0.50, 105.0, None)})

    assert summary["shared_factor_folds"]["with_shared_factor"] == 0
    assert summary["shared_factor_folds"]["totals_with_shared_factor"] is None
    assert summary["totals"]["first_innings"]["coverage_80"]["mean"] == pytest.approx(0.50)


@pytest.mark.parametrize(
    "fold, expected",
    [
        (None, False),
        ({"win": {}}, False),
        ({"win": {}, "calibration": None}, False),
        ({"win": {}, "calibration": {"shared_factor": None}}, False),
        ({"win": {}, "calibration": {"shared_factor": _CALIBRATED}}, True),
    ],
    ids=["no fold", "no calibration node", "a format with no simulator", "too thin to fit one", "fitted"],
)
def test_has_shared_factor_reads_the_folds_own_calibration_node(fold, expected) -> None:
    assert sim_harness.has_shared_factor(fold) is expected


def test_the_split_the_summary_adds_is_explained_by_the_glossary() -> None:
    """L-1: the new node reaches a surface with an explainer, or it does not reach one."""
    report = {
        "formats": {
            "ODI": {
                "walk_forward": {
                    "summary": {"simulation": sim_harness.summarize_folds({"2025-01-01": _fold(0.50, 105.0, None)})}
                }
            }
        }
    }

    assert "shared_factor_folds" in glossary.metric_keys(report)
    assert glossary.check_report(report) == []
