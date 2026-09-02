"""Unit tests for the L4 evaluation harness (ml.xi.evaluate)."""

from __future__ import annotations

import json

import pandas as pd
import pytest

from ml.xi import evaluate as ev
from ml.xi import glossary
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history
from tests.xi_perf_fixtures import fast_fits


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

    fold, model, displays = ev._evaluate_win_window(frame, pd.Timestamp("2023-06-01"), pd.Timestamp("2023-09-01"))

    assert model is None and displays == []
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
        with fast_fits():
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


def test_harness_serving_parity_passes_for_rows_and_performance_predictions(harness_report) -> None:
    parity = harness_report["serving_parity"]

    assert parity["passed"], parity["mismatches"]
    assert parity["matches_compared"] == ev.PARITY_LAST_N
    assert parity["performance_predictions_compared"] == ev.PARITY_LAST_N * 22


def test_harness_reports_selection_metrics_per_fold(harness_report) -> None:
    folds = [f for f in harness_report["formats"]["T20"]["walk_forward"]["folds"] if "objective_auc" in f]

    assert folds, "no fold scored"
    fold = folds[0]
    assert fold["swap_monotonicity"]["upgrades"] > 0
    assert 0.0 <= fold["swap_monotonicity"]["violation_share"] <= 1.0
    assert fold["specific_vs_typical"]["n"] >= 20


def test_harness_reports_the_performance_model_beside_its_baselines(harness_report) -> None:
    t20 = harness_report["formats"]["T20"]
    scored = [f for f in t20["walk_forward"]["folds"] if "objective_auc" in f]
    runs = scored[-1]["performance"]["targets"]["runs"]

    assert runs["career_mean"]["mae"] is not None
    assert runs["model"]["interval"]["width_80"] is not None  # H-22: width beside coverage
    assert runs["model"]["interval"]["coverage_80"] is not None
    summary = t20["walk_forward"]["summary"]["performance"]["targets"]["runs"]
    assert summary["vs_career_mean"]["pinball"]["n_folds"] >= 1
    assert t20["locked"]["recalibrated_targets"] == list(t20["locked"]["performance"]["fit"]["spec"]["recalibrate"])


def test_harness_reports_the_leak_canary(harness_report) -> None:
    canary = harness_report["leak_canary"]

    assert "T20" in canary["best_single_column"]
    assert isinstance(canary["test_control_suspects"], list)


def test_harness_skips_formats_without_data(harness_report) -> None:
    odi = harness_report["formats"]["ODI"]

    assert odi["n_matches"] == 0
    assert all("skipped_reason" in fold for fold in odi["walk_forward"]["folds"])


def test_harness_fills_the_e5_slot_with_the_lineup_only_metric_and_its_derived_bar(harness_report) -> None:
    e5 = harness_report["formats"]["T20"]["e5_lineup_only"]

    assert harness_report["e5_previous_elevens"]["fielded_eleven_max_abs_difference"] == pytest.approx(0.0, abs=1e-9)
    assert e5["pairs"]["total"] > 0 and e5["pairs"]["unscored_previous_eleven"] == 0
    assert e5["why_not_as_played"].startswith("§5's as-played form is not computed")
    assert e5["development"]["pairs_scored"] > 0
    assert e5["development"]["derived_bar"]["bar"] is not None
    assert "locked window" in e5["locked"]["note"]
    decision = harness_report["formats"]["T20"]["selection_decision"]
    # The served flag is the policy (plan §8.8: T20 is scoped off by E5), read beside the verdict.
    assert decision["optimised_selection_served"] is False
    assert decision["reason"].startswith("optimised selection not served in T20, because E5 lineup-only agreement")


def test_harness_e5_slot_exists_for_a_format_with_no_data(harness_report) -> None:
    e5 = harness_report["formats"]["ODI"]["e5_lineup_only"]

    assert e5["pairs"]["total"] == 0
    assert e5["decision"]["agreement"] is None
    assert e5["decision"]["reason"].endswith("E5 could not be scored (no pairs whose result moved)")


def test_harness_embeds_the_gate_registry_and_checks_it(harness_report) -> None:
    gates_node = harness_report["gates"]

    assert gates_node["passed"], gates_node["problems"]
    assert set(gates_node["registry"]) >= {"E5", "E2", "H-4", "H-8", "H-17", "E3"}
    assert gates_node["registry"]["E5"]["varies"].startswith("the eleven")


def test_harness_explains_every_metric_it_reports(harness_report) -> None:
    """L-1's completeness gate, over a real harness report: a metric the report prints
    with no glossary entry fails here, before it can reach a surface unexplained."""
    glossary_node = harness_report["glossary"]

    assert glossary_node["passed"], glossary_node["problems"]
    assert glossary.metric_keys(harness_report), "the report should carry metrics to explain"
    assert set(glossary_node["entries"]) == set(glossary.REGISTRY)
    assert glossary_node["entries"]["dispersion_ratio"]["band"].startswith("1.0 calibrated")


def _fake_report(parity_passed: bool = True, gates_passed: bool = True) -> dict:
    """The shape ``main`` reads back: one format with a summary, a decision and the verdicts."""
    return {
        "formats": {
            "T20": {
                "walk_forward": {
                    "summary": {"objective_auc": {"mean": 0.72, "sd": 0.01, "n_folds": 2}, "performance": None}
                },
                "simulation_decision": {
                    "delta_brier_mean": 0.003,
                    "delta_brier_sd": 0.004,
                    "n_folds": 2,
                    "simulated_win_probability_within_tolerance": True,
                },
                "selection_decision": {"reason": "optimised selection not served in T20, because E5 ... fails"},
            }
        },
        "serving_parity": {"passed": parity_passed, "mismatches": [] if parity_passed else ["row 1"]},
        "gates": {"passed": gates_passed, "problems": [] if gates_passed else ["gate E5: T20 carries nothing"]},
    }


@pytest.mark.parametrize(
    "parity_passed, gates_passed, expected_exit",
    [(True, True, 0), (False, True, 1), (True, False, 1)],
)
def test_main_writes_the_report_and_fails_on_parity_or_gate_problems(
    tmp_path, monkeypatch, parity_passed: bool, gates_passed: bool, expected_exit: int
) -> None:
    """The run's exit code is the two verdicts that make a report untrustworthy: H-8 parity
    and the H-23 gate registry. Everything else is reported, never fatal."""
    monkeypatch.setattr(
        ev, "evaluate", lambda source, factory, gender_split_context=False: _fake_report(parity_passed, gates_passed)
    )
    out = tmp_path / "report"

    code = ev.main(["--cricsheet-dir", str(tmp_path), "--out", str(out)])

    assert code == expected_exit
    written = json.loads((out / ev.REPORT_NAME).read_text())
    assert written["serving_parity"]["passed"] is parity_passed
    assert written["gates"]["passed"] is gates_passed
