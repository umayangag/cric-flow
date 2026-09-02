"""Unit tests for the H-23 gate registry (ml.xi.gates)."""

from __future__ import annotations

import copy

import pytest

from ml.xi import gates


def _report_with_every_gate() -> dict:
    node = {
        "walk_forward": {
            "summary": {
                "objective_auc": None,
                "swap_violation_share": None,
                "specific_vs_typical_delta": None,
                "performance": None,
            }
        },
        "locked": {"recalibrated_targets": []},
        "simulation_decision": {"simulated_win_probability_within_tolerance": True},
        "e5_lineup_only": {"decision": {"agreement": None}},
    }
    return {
        "leak_canary": {"test_control_suspects": []},
        "serving_parity": {"passed": True},
        "formats": {"T20": node, "ODI": copy.deepcopy(node)},
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


def test_script_reported_gates_have_no_report_path() -> None:
    assert gates.REGISTRY["E3"].report_path is None
