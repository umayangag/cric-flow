"""Unit tests for the H-23 gate registry (ml.xi.gates), and for the standing gates' clauses
being code a report can fail (EVAL-04)."""

from __future__ import annotations

from typing import Optional

import pytest

from ml.xi import gates


def _format_node(served: bool = False, objective_auc: Optional[float] = 0.70) -> dict:
    """One format's node carrying every standing gate's number at a passing value, with the
    serving policy written where ``evaluate`` writes it."""
    return {
        "walk_forward": {
            "summary": {
                "objective_auc": None if objective_auc is None else {"mean": objective_auc, "sd": 0.02, "n_folds": 3},
                "swap_violation_share": {"mean": 0.004, "sd": 0.003, "n_folds": 3},
                "specific_vs_typical_delta": {"mean": 0.02, "sd": 0.01, "n_folds": 3},
                "performance": None,
            }
        },
        "locked": {"recalibrated_targets": []},
        "simulation_decision": {"simulated_win_probability_within_tolerance": True, "served": False},
        "e5_lineup_only": {
            "decision": {
                "agreement": 0.56,
                "bar": 0.50,
                "passes_derived_bar": True,
                "optimised_selection_served": served,
            }
        },
        "selection_decision": {"passes_derived_bar": True, "optimised_selection_served": served},
    }


def _report_with_every_gate() -> dict:
    return {
        "leak_canary": {"test_control_suspects": []},
        "serving_parity": {"passed": True},
        "market_benchmark": {"formats": {"T20": {}, "ODI": {}}},
        "formats": {"T20": _format_node(served=False), "ODI": _format_node(served=True)},
        "gates": {"registry": gates.as_dict()},
    }


def test_every_gate_declares_its_triple() -> None:
    for gate in gates.GATES:
        assert gate.varies.strip(), gate.id
        assert gate.fixed.strip(), gate.id
        assert gate.decides.strip(), gate.id


def test_check_report_passes_a_report_that_carries_every_gate() -> None:
    assert gates.check_report(_report_with_every_gate()) == []


def test_check_report_names_a_format_missing_a_gate() -> None:
    report = _report_with_every_gate()
    del report["formats"]["ODI"]["e5_lineup_only"]

    problems = gates.check_report(report)

    assert problems == ["gate E5: ODI carries nothing at e5_lineup_only.decision.agreement"]


def test_check_report_names_a_missing_report_level_gate() -> None:
    report = _report_with_every_gate()
    del report["serving_parity"]

    problems = gates.check_report(report)

    assert problems == ["gate H-8: report carries nothing at report:serving_parity.passed"]


def test_check_report_rejects_an_embedded_registry_that_drifted() -> None:
    report = _report_with_every_gate()
    report["gates"]["registry"].pop("E5")

    assert "the report's embedded registry does not match the code's" in gates.check_report(report)


def test_describe_states_the_triple_in_h23_order() -> None:
    line = gates.describe("E3")

    assert line.startswith("E3 (Batting order) - varies: ")
    assert "; fixed: " in line and "; decides: " in line


def test_describe_refuses_an_unregistered_gate() -> None:
    with pytest.raises(KeyError):
        gates.describe("P-0 winner accuracy")


@pytest.mark.parametrize("gate_id", ["E3", "A-1", "A-2", "A-3", "SIM-DN-split", "SIM-DN-scale"])
def test_script_reported_gates_have_no_report_path(gate_id: str) -> None:
    """A gate an experiment script runs is registered for its triple, not for a report path."""
    assert gates.REGISTRY[gate_id].report_path is None


# --- EVAL-04: a standing gate's clause is code, and a report can fail it -------------


@pytest.mark.parametrize("gate_id", ["H-17", "H-4", "specific-vs-typical", "E5", "E2", "H-8"])
def test_every_standing_gate_carries_its_clause_as_a_threshold(gate_id: str) -> None:
    """The gates the report prints every run evaluate their decides text, not only its path."""
    gate = gates.REGISTRY[gate_id]

    assert gate.report_path is not None
    assert gate.threshold is not None and gate.threshold.rule.strip(), gate_id


@pytest.mark.parametrize("gate_id", ["H-5", "H-22", "H-2", "X-4"])
def test_a_gate_whose_clause_is_an_action_or_informs_carries_no_threshold(gate_id: str) -> None:
    """H-5 decides a recalibration the harness applies, H-22 needs the previous release, H-2
    and X-4 inform: none has anything to refuse, and none pretends to."""
    assert gates.REGISTRY[gate_id].threshold is None


def test_the_embedded_registry_carries_each_threshold_as_its_rule() -> None:
    """A clause is code; what the report embeds beside the number is the rule a reader can
    check by hand, and null where there is none."""
    embedded = gates.as_dict()

    assert embedded["H-4"]["threshold"] == "mean < 0.02 in every format the folds scored"
    assert embedded["H-5"]["threshold"] is None
    assert embedded["E3"]["threshold"] is None


def test_h17_fails_a_served_format_whose_objective_auc_is_under_the_line() -> None:
    """The scenario the finding names: a served format's walk-forward AUC drops under 0.65
    and, before this, nothing served changed and the report still said passed."""
    report = _report_with_every_gate()
    report["formats"]["ODI"]["walk_forward"]["summary"]["objective_auc"]["mean"] = 0.64

    problems = gates.check_report(report)

    assert problems == [
        "gate H-17: ODI fails its threshold: an optimised selection is served on a walk-forward mean objective "
        "AUC of 0.6400, under the 0.65 line"
    ]


def test_h17_does_not_fail_a_format_that_is_not_served_under_the_line() -> None:
    """The prose scopes the surface off under the line; a format already scoped off (TEST)
    is what the rule asks for, not a failure of it."""
    report = _report_with_every_gate()
    report["formats"]["T20"]["walk_forward"]["summary"]["objective_auc"]["mean"] = 0.60

    assert gates.check_report(report) == []


def test_h17_fails_a_served_format_with_no_objective_auc_at_all() -> None:
    report = _report_with_every_gate()
    report["formats"]["ODI"]["walk_forward"]["summary"]["objective_auc"] = None

    problems = gates.check_report(report)

    assert problems == [
        "gate H-17: ODI fails its threshold: an optimised selection is served with no walk-forward objective AUC "
        "to stand on"
    ]


def test_h4_fails_any_format_whose_swap_share_reaches_two_percent() -> None:
    """H-4 is unconditional: the objective is a contract in every format, served or not."""
    report = _report_with_every_gate()
    report["formats"]["T20"]["walk_forward"]["summary"]["swap_violation_share"]["mean"] = 0.02

    problems = gates.check_report(report)

    assert problems == [
        "gate H-4: T20 fails its threshold: the share of one-player upgrades that lower the objective's P(win) "
        "is 0.0200, not under 0.02"
    ]


def test_h4_skips_a_format_no_fold_scored() -> None:
    """No fold produced the number: nothing to hold to the line (a served format on that
    absence is H-17's failure, not H-4's)."""
    report = _report_with_every_gate()
    report["formats"]["T20"]["walk_forward"]["summary"]["swap_violation_share"] = None

    assert gates.check_report(report) == []


def test_specific_vs_typical_fails_a_served_format_whose_delta_is_not_positive() -> None:
    report = _report_with_every_gate()
    report["formats"]["ODI"]["walk_forward"]["summary"]["specific_vs_typical_delta"]["mean"] = 0.0

    problems = gates.check_report(report)

    assert problems == [
        "gate specific-vs-typical: ODI fails its threshold: an optimised selection is served while the specific "
        "eleven adds +0.0000 AUC over the typical eleven, not above zero"
    ]


def test_e5_fails_a_served_format_that_does_not_clear_its_derived_bar() -> None:
    report = _report_with_every_gate()
    decision = report["formats"]["ODI"]["e5_lineup_only"]["decision"]
    decision.update({"agreement": 0.49, "passes_derived_bar": False})

    problems = gates.check_report(report)

    assert problems == [
        "gate E5: ODI fails its threshold: an optimised selection is served while E5 lineup-only agreement 0.4900 "
        "does not clear the derived bar 0.5"
    ]


def test_e5_fails_a_served_format_it_could_not_score() -> None:
    report = _report_with_every_gate()
    decision = report["formats"]["ODI"]["e5_lineup_only"]["decision"]
    decision.update({"agreement": None, "passes_derived_bar": None})

    problems = gates.check_report(report)

    assert problems == [
        "gate E5: ODI fails its threshold: an optimised selection is served with E5 unscored (no pairs whose "
        "result moved)"
    ]


def test_e5_does_not_fail_a_format_that_is_not_served() -> None:
    """T20's standing state: fails its bar and is served the rating-ordered eleven."""
    report = _report_with_every_gate()
    decision = report["formats"]["T20"]["e5_lineup_only"]["decision"]
    decision.update({"agreement": 0.49, "passes_derived_bar": False})

    assert gates.check_report(report) == []


def test_e2_fails_a_simulated_headline_that_is_out_of_tolerance() -> None:
    report = _report_with_every_gate()
    report["formats"]["T20"]["simulation_decision"] = {
        "simulated_win_probability_within_tolerance": False,
        "served": True,
        "reason": "worse than the display model by more than the tolerance",
    }

    problems = gates.check_report(report)

    assert problems == [
        "gate E2: T20 fails its threshold: the simulated P(win) is served as the headline while it is not within "
        "tolerance of the display model: worse than the display model by more than the tolerance"
    ]


def test_e2_does_not_fail_a_description_that_is_not_the_headline() -> None:
    """Out of tolerance and not served is what the prose decides: a description only."""
    report = _report_with_every_gate()
    report["formats"]["T20"]["simulation_decision"] = {
        "simulated_win_probability_within_tolerance": False,
        "served": False,
    }

    assert gates.check_report(report) == []


def test_h8_fails_a_report_whose_parity_did_not_pass() -> None:
    report = _report_with_every_gate()
    report["serving_parity"]["passed"] = False

    problems = gates.check_report(report)

    assert problems == ["gate H-8: report fails its threshold: the as-of serving path and the training pass disagree"]


def test_every_failing_clause_is_reported_not_only_the_first() -> None:
    """Two formats failing two gates give four problems: the operator reads all of them."""
    report = _report_with_every_gate()
    for node in report["formats"].values():
        node["walk_forward"]["summary"]["swap_violation_share"]["mean"] = 0.05
    report["formats"]["ODI"]["walk_forward"]["summary"]["objective_auc"]["mean"] = 0.5
    report["serving_parity"]["passed"] = False

    problems = gates.check_report(report)

    assert [p.split(":")[0] for p in problems] == ["gate H-17", "gate H-4", "gate H-4", "gate H-8"]
