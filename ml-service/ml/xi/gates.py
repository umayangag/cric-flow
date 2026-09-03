"""H-23: a gate must name what varies between its arms and what is held fixed.

Three gates in this project reported a clean pass or fail while moving for a reason other
than the thing they named (plan §8.5, §8.6): P-0's winner accuracy scored the *fixture*
(both arms optimised both sides), E5's as-played form scored mean reversion, and the P-0
run reached neither arm it named. Every one of them would have been visible at design time
under one rule: beside its number, a gate states the quantity it varies, the quantities it
holds fixed, and what decides.

This module is that rule made machine-checkable. Every gate the L4 report prints is
registered here with its varied / fixed / decides triple and the path at which the report
carries its number; ``check_report`` fails a report that prints a gate the registry does
not describe, or describes a gate the report does not carry. The report embeds the
registry so its consumers render the triple beside the number. An experiment script that
runs a gate the report does not print (E3) reads its triple from here and states it before
running, for the same reason.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any, Dict, List, Optional, Tuple

#: Prefix for a path read from the whole report rather than from one format's node.
REPORT_SCOPE = "report:"


@dataclass(frozen=True)
class Gate:
    id: str
    name: str
    varies: str
    fixed: str
    decides: str
    #: Where the report carries the number: relative to each format's node, or to the whole
    #: report with the ``report:`` prefix. None for a gate an experiment script reports.
    report_path: Optional[str]


GATES: Tuple[Gate, ...] = (
    Gate(
        id="H-17",
        name="Objective ranks (format scope)",
        varies="the two elevens' as-of features across a fold's evaluation matches",
        fixed="the fold's cutoff, the objective fitted before it, the labels",
        decides="walk-forward mean objective AUC >= 0.65, else the format is not offered an optimised selection",
        report_path="walk_forward.summary.objective_auc",
    ),
    Gate(
        id="H-4",
        name="Swap monotonicity",
        varies="one player's scoring, wicket and Elo ratings, raised by one population sd each",
        fixed="the other ten, the opponent eleven, the as-of date, the fold's objective",
        decides="share of upgrades that lower P(win) < 2 %",
        report_path="walk_forward.summary.swap_violation_share",
    ),
    Gate(
        id="specific-vs-typical",
        name="Specific XI beyond typical XI",
        varies="whether the objective reads the eleven that played or the side's mean of its last 10 aggregates",
        fixed="the evaluation matches, the labels, the fold's objective",
        decides="AUC(specific) - AUC(typical) > 0",
        report_path="walk_forward.summary.specific_vs_typical_delta",
    ),
    Gate(
        id="E5",
        name="Natural experiment, lineup-only",
        varies="the eleven: match k's against match k+1's, 1-3 players apart",
        fixed="match k+1's opponent eleven, match k+1's as-of ratings, the fold's objective",
        decides="sign agreement between the objective's preference and the result change, over pairs whose "
        "result moved, at or above the bar derived from the objective's own claimed effect size (§8.8)",
        report_path="e5_lineup_only.decision.agreement",
    ),
    Gate(
        id="E2",
        name="Simulated P(win) against the display model",
        varies="which model produces P(win): the simulator's draws or the display model",
        fixed="the fixtures, the as-of forecasts, the fold's models, the labels",
        decides="Brier(simulated) - Brier(display) <= 0.01 on the folds, else the simulation is a description only",
        report_path="simulation_decision.simulated_win_probability_within_tolerance",
    ),
    Gate(
        id="H-5",
        name="Quantile coverage",
        varies="the quantile level (0.1 / 0.9) and the target",
        fixed="the evaluation rows, the fold's performance model",
        decides="each level's exceedance sits between its strict and inclusive rate +/- 0.03, else the target is "
        "recalibrated on a temporal fold for the locked window",
        report_path="locked.recalibrated_targets",
    ),
    Gate(
        id="H-22",
        name="Sharpness at fixed calibration",
        varies="the release",
        fixed="coverage at nominal, the target, the population",
        decides="a narrower 10-90 interval with coverage held is progress; narrower with coverage falling fails",
        report_path="walk_forward.summary.performance",
    ),
    Gate(
        id="H-2",
        name="Leak canary",
        varies="the single column read",
        fixed="the development-window labels, per format",
        decides="a column at AUC >= 0.65 in a limited-overs format and <= 0.55 in TEST is a suspect to be reviewed",
        report_path=REPORT_SCOPE + "leak_canary.test_control_suspects",
    ),
    Gate(
        id="H-8",
        name="Train / serve parity",
        varies="the code path: the training pass or the as-of serving path",
        fixed="the last 50 matches, their elevens, the performance model, the simulator seed",
        decides="max abs difference <= 1e-9 across rows, predictions and draws, else the run fails",
        report_path=REPORT_SCOPE + "serving_parity.passed",
    ),
    Gate(
        id="E3",
        name="Batting order",
        varies="the order of the top seven batting slots",
        fixed="the eleven, the opponent eleven, the as-of date, batting first, the random numbers",
        decides="share of elevens whose best sampled order moves the confirmed simulated median total by > 3 %; "
        "above 30 % adds a batting-order suggestion to L3",
        report_path=None,
    ),
    Gate(
        id="A-1",
        name="Fixture-conditional level",
        varies="which fixture-context families the performance model reads: none, venue, competition, both -- "
        "one fit per arm per fold",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three seeds, the hyperparameters, the shared factor's "
        "fitting rule (the 92-day calibration fold), the display models, the simulator and its draw count, "
        "the labels",
        decides="a family is kept only if, against the no-context arm on the same folds, the mean per-quarter |bias| "
        "of the simulated first-innings mean shrinks while 10-90 coverage stays within +/- 0.03, width does not "
        "grow (H-22) and every headline target's pinball is no worse by more than 0.5 %, in both T20 and ODI; "
        "a recorded null ships no feature",
        report_path=None,
    ),
    Gate(
        id="A-2",
        name="Chase tails: a target-conditional chasing innings",
        varies="the chase response the simulator applies to the chasing side's runs draws -- none, level (the "
        "control: slope held at zero), slope, both -- from one L2-B fit per fold and one fitted sample",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three seeds, the hyperparameters, the "
        "performance model (fitted once per fold, shared by the arms), the display models, the shared factor and "
        "its fitting rule, the simulator's draw count and seeds (common random numbers), the labels",
        decides="in both T20 and ODI, paired per fold against none with one fold-level standard error as the floor: "
        "the chase 10-90 coverage's distance from 0.80 shrinks, mean |chase bias| shrinks, first-innings coverage "
        "stays within +/- 0.03 with width not growing (H-22), and E2 does not degrade (mean delta Brier within "
        "0.01, paired difference not worse by more than one standard error); a candidate arm ships only if it "
        "also beats the level control on the coverage distance by more than one standard error; a recorded null "
        "ships nothing",
        report_path=None,
    ),
    Gate(
        id="A-3",
        name="The T20 lineup signal: feature families in the selection objective",
        varies="the feature family the selection objective reads beyond XI_FEATURE_COLS -- none (today's "
        "objective), phase matchup (a: each side's per-phase batting and bowling impact and the same-phase "
        "product against the opposing attack), role balance (b: top-order, specialist-batter, sixth-bowler and "
        "keeper-batting counts and the batting x bowling, all-rounder x tail and attack-size interactions) -- one "
        "logistic fit per arm per fold; and, as a measurement diagnostic on every arm rather than an arm, E5's "
        "evidence reweighted by the |delta objective| the arm itself claims (c)",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), E5's pairs (the same consecutive "
        "1-3-change pairs, the previous eleven read once from the same as-of pass), the bar's derivation "
        "(Bernoulli at the arm's own probabilities, 2,000 replicates, the 5th percentile), the objective's model "
        "class and regularisation, the display models, the labels",
        decides="in T20, an arm's pooled walk-forward lineup-only agreement at or above the bar re-derived from "
        "that arm's own claimed effect size, under each of three bar seeds; an arm that clears it ships only if in "
        "every format its fold objective AUC is not lower than today's by more than one fold-level standard error "
        "(paired) and its swap-violation share stays under H-4's 2 %; the reweighted (c) reading is reported "
        "beside the verdict and never decides on its own; a recorded null ships nothing and T20 stays "
        "rating-ordered",
        report_path=None,
    ),
)

REGISTRY: Dict[str, Gate] = {gate.id: gate for gate in GATES}


def as_dict() -> Dict[str, Dict[str, Any]]:
    """The registry as the report embeds it."""
    return {gate.id: asdict(gate) for gate in GATES}


def describe(gate_id: str) -> str:
    """One line a script prints before it runs a gate: the triple, in the H-23 order."""
    gate = REGISTRY[gate_id]
    return f"{gate.id} ({gate.name}) - varies: {gate.varies}; fixed: {gate.fixed}; decides: {gate.decides}"


def _path_exists(node: Any, path: str) -> bool:
    for key in path.split("."):
        if not isinstance(node, dict) or key not in node:
            return False
        node = node[key]
    return True


def check_report(report: Dict[str, Any]) -> List[str]:
    """Every registered gate with a report path must be carried by the report -- at the
    top level or under every format -- and every gate must declare a non-empty triple.
    Returns the problems; an empty list is a report that says what its gates vary."""
    problems: List[str] = []
    for gate in GATES:
        for field in ("varies", "fixed", "decides"):
            if not getattr(gate, field).strip():
                problems.append(f"gate {gate.id} declares no '{field}'")
        if gate.report_path is None:
            continue
        if gate.report_path.startswith(REPORT_SCOPE):
            if not _path_exists(report, gate.report_path[len(REPORT_SCOPE) :]):
                problems.append(f"gate {gate.id}: report carries nothing at {gate.report_path}")
            continue
        for format_code, node in report.get("formats", {}).items():
            if not _path_exists(node, gate.report_path):
                problems.append(f"gate {gate.id}: {format_code} carries nothing at {gate.report_path}")
    embedded = report.get("gates", {}).get("registry")
    if embedded is not None and set(embedded) != set(REGISTRY):
        problems.append("the report's embedded registry does not match the code's")
    return problems
