"""Unit tests for the L4 evaluation harness (ml.xi.evaluate)."""

from __future__ import annotations

import json

import pandas as pd
import pytest

from ml.xi import evaluate as ev
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history


def test_fold_windows_end_where_the_locked_window_starts() -> None:
    windows = ev.fold_windows()

    assert len(windows) == len(ev.WALK_FORWARD_CUTOFFS)
    assert windows[0][0] == pd.Timestamp("2024-01-01")
    assert windows[-1] == (pd.Timestamp("2025-06-01"), pd.Timestamp(ev.LOCKED_START))


def test_stats_ignores_missing_folds() -> None:
    stats = ev._stats([0.7, None, 0.8])

    assert stats["n_folds"] == 2
    assert stats["mean"] == pytest.approx(0.75)


def test_stats_is_none_when_no_fold_produced_the_number() -> None:
    assert ev._stats([None, None]) is None


def test_evaluate_win_window_reports_why_it_skipped() -> None:
    frame = pd.DataFrame({"match_date": [pd.Timestamp("2023-01-01")], "team1_wins": [1.0]})

    fold, model = ev._evaluate_win_window(frame, pd.Timestamp("2023-06-01"), pd.Timestamp("2023-09-01"))

    assert model is None
    assert fold["skipped_reason"] == "insufficient training rows"


@pytest.fixture(scope="module")
def harness_report(tmp_path_factory) -> dict:
    """One end-to-end harness run over a compressed synthetic timeline."""
    matches, _, _ = _synthetic_history(160)

    # Compress the walk-forward onto the synthetic 2023 timeline.
    original = (ev.WALK_FORWARD_CUTOFFS, ev.LOCKED_START)
    ev.WALK_FORWARD_CUTOFFS = ["2023-03-01", "2023-04-01"]
    ev.LOCKED_START = "2023-05-01"
    try:
        report = ev.evaluate(_ListSource(matches), lambda: _ListSource(matches))
    finally:
        ev.WALK_FORWARD_CUTOFFS, ev.LOCKED_START = original
    out = tmp_path_factory.mktemp("harness") / "report.json"
    out.write_text(json.dumps(report))  # the report must be JSON-serializable
    return report


def test_harness_reports_walk_forward_with_spread(harness_report) -> None:
    summary = harness_report["formats"]["T20"]["walk_forward"]["summary"]

    assert summary["objective_auc"]["n_folds"] == 2
    assert 0.0 < summary["objective_auc"]["mean"] < 1.0
    assert summary["display_auc"]["sd"] >= 0.0
    assert summary["base_rate_brier"]["mean"] > 0.0


def test_harness_scores_the_locked_window_once_and_labels_it(harness_report) -> None:
    locked = harness_report["formats"]["T20"]["locked"]

    assert "locked window" in locked["note"]
    assert "objective_auc" in locked


def test_harness_serving_parity_passes(harness_report) -> None:
    parity = harness_report["serving_parity"]

    assert parity["passed"], parity["mismatches"]
    assert parity["matches_compared"] == ev.PARITY_LAST_N


def test_harness_reports_selection_metrics_per_fold(harness_report) -> None:
    folds = [f for f in harness_report["formats"]["T20"]["walk_forward"]["folds"] if "objective_auc" in f]

    assert folds, "no fold scored"
    fold = folds[0]
    assert fold["swap_monotonicity"]["upgrades"] > 0
    assert 0.0 <= fold["swap_monotonicity"]["violation_share"] <= 1.0
    assert fold["specific_vs_typical"]["n"] >= 20


def test_harness_reports_performance_baselines_with_empty_interval_columns(harness_report) -> None:
    fold = [f for f in harness_report["formats"]["T20"]["walk_forward"]["folds"] if "objective_auc" in f][0]
    runs = fold["performance"]["runs"]

    assert runs["career_mean"]["mae"] is not None
    assert runs["interval_width_80"] is None  # H-22: the column exists, P-3 fills it
    assert runs["interval_coverage_80"] is None


def test_harness_reports_the_leak_canary(harness_report) -> None:
    canary = harness_report["leak_canary"]

    assert "T20" in canary["best_single_column"]
    assert isinstance(canary["test_control_suspects"], list)


def test_harness_skips_formats_without_data(harness_report) -> None:
    odi = harness_report["formats"]["ODI"]

    assert odi["n_matches"] == 0
    assert all("skipped_reason" in fold for fold in odi["walk_forward"]["folds"])
