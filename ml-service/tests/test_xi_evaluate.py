"""Unit tests for the L4 evaluation harness (ml.xi.evaluate)."""

from __future__ import annotations

import json
from contextlib import contextmanager
from typing import Iterator, List

import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import evaluate as ev
from ml.xi import gates, glossary
from ml.xi.train import _score_marginalised
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history
from tests.test_xi_train import synthetic_win_rows
from tests.xi_perf_fixtures import fast_fits


def test_fold_windows_end_where_the_locked_window_starts() -> None:
    windows = ev.fold_windows()

    assert len(windows) == len(ev.WALK_FORWARD_CUTOFFS)
    assert windows[0][0] == pd.Timestamp("2024-01-01")
    assert windows[-1] == (pd.Timestamp(ev.WALK_FORWARD_CUTOFFS[-1]), pd.Timestamp(ev.LOCKED_START))


def test_the_retired_locked_window_is_covered_by_the_folds() -> None:
    """A-4: rotation retires the spent window into the walk-forward folds, with no gap."""
    windows = ev.fold_windows()
    retired = [(start, end) for start, end in windows if start >= pd.Timestamp(ev.LOCKED_PREVIOUS_START)]

    assert retired, "the previous locked window is not covered by any fold"
    assert retired[0][0] == pd.Timestamp(ev.LOCKED_PREVIOUS_START)
    assert retired[-1][1] == pd.Timestamp(ev.LOCKED_START)
    assert [end for _, end in retired[:-1]] == [start for start, _ in retired[1:]]


def test_locked_window_states_where_the_line_is_and_when_it_moved() -> None:
    window = ev.locked_window()

    assert window["start"] == ev.LOCKED_START
    assert window["rotated_on"] == ev.LOCKED_ROTATED_ON
    assert window["previous_start"] == ev.LOCKED_PREVIOUS_START
    assert window["retired_into_folds"][0] == ev.LOCKED_PREVIOUS_START
    assert ev.LOCKED_START in ev.locked_note() and ev.LOCKED_PREVIOUS_START in ev.locked_note()


def test_stats_ignores_missing_folds() -> None:
    stats = ev._stats([0.7, None, 0.8])

    assert stats["n_folds"] == 2
    assert stats["mean"] == pytest.approx(0.75)


def test_stats_is_none_when_no_fold_produced_the_number() -> None:
    assert ev._stats([None, None]) is None


def test_evaluate_win_window_reports_why_it_skipped() -> None:
    frame = pd.DataFrame({"match_date": [pd.Timestamp("2023-01-01")], "team1_wins": [1.0]})

    fold, model, display = ev._evaluate_win_window(frame, pd.Timestamp("2023-06-01"), pd.Timestamp("2023-09-01"))

    assert model is None and display is None
    assert fold["skipped_reason"] == "insufficient training rows"


def test_evaluate_win_window_fits_the_display_model_once_and_reports_no_seed_spread() -> None:
    """EVAL-02: one display fit per window, its own score under the wire key, and no
    spread across seeds -- the seeds were the same fit, so the number was a zero floor."""
    rows = synthetic_win_rows(400)
    cutoff, end = pd.Timestamp("2023-11-01"), pd.Timestamp("2024-02-01")

    fold, objective, display = ev._evaluate_win_window(rows, cutoff, end)

    evaluation = rows[(rows.match_date >= cutoff) & (rows.match_date < end)]
    assert objective is not None and hasattr(display, "predict_proba")
    assert "display_auc_seed_sd" not in fold
    assert fold["display_auc_mean"] == pytest.approx(
        _score_marginalised(display, evaluation, C.DISPLAY_FEATURE_COLS)["auc"]
    )


def _no_cached_odds(tmp_path_factory) -> str:
    """An empty odds directory, so a harness run is the same on a machine that happens to
    have X-4's cache and one that does not. Tests are offline; the market benchmark then
    reports zero coverage, which is what it should say."""
    return str(tmp_path_factory.mktemp("no-cached-odds"))


@contextmanager
def _synthetic_timeline(cutoffs: List[str], locked_start: str) -> Iterator[None]:
    """Compress the walk-forward onto the synthetic 2023 timeline, and serve no format an
    optimised selection while on it: the serving policy is a hand-set fact about the real
    archive (`optimizer.OPTIMISED_SELECTION_FORMATS`), and a synthetic history has none --
    a format served on no evidence is exactly what the H-17 and E5 clauses fail."""
    original = (ev.WALK_FORWARD_CUTOFFS, ev.LOCKED_START, ev.OPTIMISED_SELECTION_FORMATS)
    ev.WALK_FORWARD_CUTOFFS = cutoffs
    ev.LOCKED_START = locked_start
    ev.OPTIMISED_SELECTION_FORMATS = frozenset()
    try:
        yield
    finally:
        ev.WALK_FORWARD_CUTOFFS, ev.LOCKED_START, ev.OPTIMISED_SELECTION_FORMATS = original


@pytest.fixture(scope="module")
def harness_report(tmp_path_factory) -> dict:
    """One end-to-end harness run over a compressed synthetic timeline."""
    matches, _, _ = _synthetic_history(160)

    with _synthetic_timeline(["2023-03-01", "2023-04-01"], "2023-05-01"), fast_fits():
        report = ev.evaluate(
            _ListSource(matches),
            lambda: _ListSource(matches),
            market_odds_dir=_no_cached_odds(tmp_path_factory),
        )
    out = tmp_path_factory.mktemp("harness") / "report.json"
    out.write_text(json.dumps(report))  # the report must be JSON-serializable
    return report


@pytest.fixture(scope="module")
def freshly_rotated_report(tmp_path_factory) -> dict:
    """A harness run whose locked window was just rotated and holds no matches yet (A-4)."""
    matches, _, _ = _synthetic_history(160)

    with _synthetic_timeline(["2023-03-01", "2023-04-01", "2023-05-01"], "2030-01-01"), fast_fits():
        return ev.evaluate(
            _ListSource(matches),
            lambda: _ListSource(matches),
            market_odds_dir=_no_cached_odds(tmp_path_factory),
        )


def test_an_empty_locked_window_says_so_rather_than_scoring_noise(freshly_rotated_report) -> None:
    locked = freshly_rotated_report["formats"]["T20"]["locked"]

    assert locked["n_eval"] == 0
    assert locked["skipped_reason"] == "evaluation window too small or single-class"


def test_parity_falls_back_to_the_last_fold_model_when_the_window_is_empty(freshly_rotated_report) -> None:
    """H-8 keeps a model to serve while a rotated window fills (A-4)."""
    parity = freshly_rotated_report["serving_parity"]

    assert freshly_rotated_report["formats"]["T20"]["parity_model_window"] == "2023-05-01"
    assert parity["passed"], parity["mismatches"]
    assert parity["performance_predictions_compared"] == ev.PARITY_LAST_N * 22


def test_harness_reports_walk_forward_with_spread(harness_report) -> None:
    summary = harness_report["formats"]["T20"]["walk_forward"]["summary"]

    assert summary["objective_auc"]["n_folds"] == 2
    assert 0.0 < summary["objective_auc"]["mean"] < 1.0
    assert summary["display_auc"]["sd"] >= 0.0
    assert summary["base_rate_brier"]["mean"] > 0.0


def test_harness_reports_no_spread_across_seeds(harness_report) -> None:
    """The spread a difference is read against is the one over folds; the harness fits
    each model once and names no seeds, so nothing it prints can be mistaken for a
    seed-to-seed noise floor (EVAL-02)."""
    summary = harness_report["formats"]["T20"]["walk_forward"]["summary"]

    assert "seeds" not in harness_report
    assert "display_auc_seed_sd_mean" not in summary
    assert all("display_auc_seed_sd" not in fold for fold in harness_report["formats"]["T20"]["walk_forward"]["folds"])


def test_harness_scores_the_locked_window_once_and_labels_it(harness_report) -> None:
    locked = harness_report["formats"]["T20"]["locked"]

    assert "locked window" in locked["note"]
    assert "objective_auc" in locked


def test_harness_report_carries_the_window_and_its_rotation(harness_report) -> None:
    """A reader of any number can tell which window it came from (A-4)."""
    window = harness_report["locked_window"]

    assert harness_report["locked_start"] == window["start"]
    assert window["rotated_on"] and window["previous_start"]
    assert window["reason"].strip()


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


def test_harness_reports_the_display_surfaces_swap_share_beside_the_objectives(harness_report) -> None:
    """B-7: the display model is probed too, per fold and as a walk-forward mean."""
    walk_forward = harness_report["formats"]["T20"]["walk_forward"]
    fold = next(f for f in walk_forward["folds"] if "objective_auc" in f)

    assert fold["display_swap_monotonicity"]["upgrades"] > 0
    assert 0.0 <= fold["display_swap_monotonicity"]["violation_share"] <= 1.0
    assert walk_forward["summary"]["display_swap_violation_share"]["n_folds"] >= 1


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
    """The report carries every gate where the registry says it does, and the verdicts it
    prints are the clauses evaluated on its own numbers (EVAL-04). The synthetic history
    fits the objective on ~70 rows per fold with the weakest players bowling, so it is not
    monotone in a player's ratings and H-4 fails it, which is what the clause is for; the
    real archive measures 0.2-0.8 % (plan §8). What is pinned here is that the harness
    reports the verdict rather than a pass, and that nothing structural is wrong."""
    gates_node = harness_report["gates"]
    verdicts = [p for p in gates_node["problems"] if "fails its threshold" in p]

    assert gates_node["passed"] is (gates_node["problems"] == [])
    assert verdicts == gates_node["problems"], "a structural problem: a gate printed without an entry, or unread"
    assert {p.split(":")[0] for p in verdicts} <= {f"gate {g.id}" for g in gates.GATES if g.threshold is not None}
    assert set(gates_node["registry"]) >= {"E5", "E2", "H-4", "H-8", "H-17", "E3", "X-4"}
    assert gates_node["registry"]["E5"]["varies"].startswith("the eleven")
    assert gates_node["registry"]["H-4"]["threshold"].startswith("mean < 0.02")
    assert gates_node["registry"]["X-4"]["decides"].startswith("nothing automatically")


def test_harness_carries_the_market_benchmark_even_with_no_odds_cached(harness_report) -> None:
    """X-4: a run with no cached odds still says what the benchmark covered — nothing —
    rather than dropping the section, so a reader is never left guessing whether it ran."""
    benchmark = harness_report["market_benchmark"]

    assert benchmark["available"] is False
    assert set(benchmark["formats"]) == set(ev.C.FORMAT_CODES)
    assert benchmark["formats"]["T20"]["joined_share"] == 0.0
    assert benchmark["formats"]["T20"]["pooled"] is None


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
        "market_benchmark": {
            "formats": {"T20": {"matches_in_windows": 100, "matches_joined": 0, "joined_share": 0.0, "pooled": None}}
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
        ev,
        "evaluate",
        lambda source, factory, gender_split_context=False, market_odds_dir=None: _fake_report(
            parity_passed, gates_passed
        ),
    )
    out = tmp_path / "report"

    code = ev.main(["--cricsheet-dir", str(tmp_path), "--out", str(out)])

    assert code == expected_exit
    written = json.loads((out / ev.REPORT_NAME).read_text())
    assert written["serving_parity"]["passed"] is parity_passed
    assert written["gates"]["passed"] is gates_passed
