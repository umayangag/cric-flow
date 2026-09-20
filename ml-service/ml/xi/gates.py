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

A standing gate -- one the report prints every run -- also carries its ``decides`` clause
as code (EVAL-04): a ``Threshold`` beside the ``report_path``, evaluated by
``check_report`` on the number the report carries there. Until it did, ``check_report``
verified only that the path existed, so a run whose objective AUC had fallen under H-17's
line, or whose swap share had crossed H-4's, reported ``gates.passed: true`` and changed
nothing served. The clauses encode what the prose already says and nothing more:

* H-17, E5 and specific-vs-typical are *scoping* gates -- their prose decides whether a
  format is offered an optimised selection, and that policy is set by hand in
  ``optimizer.OPTIMISED_SELECTION_FORMATS`` from the report, deliberately (plan §8.8). The
  enforceable form is the contrapositive: a format **served** an optimised selection must
  carry the evidence the policy rests on, and fails the run when it does not. A format
  not served is not failed by them -- withholding the surface is what the prose asks for.
* E2 is the same shape for the simulated P(win): served as the headline
  (``simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED``) only within tolerance.
* H-4 and H-8 are unconditional: the objective's swap-violation share under 2 % in every
  format, and parity passed.
* H-5's clause decides an *action* the harness already takes (``perf_harness
  .recalibration_needed`` names the targets and the locked window recalibrates those it
  has a fold for), so there is nothing for it to refuse; the report carries the request,
  what was applied and what was skipped, so a reader can see the difference (EVAL-08).
  Whether a skipped correction should *fail* a run is a threshold this gate does not yet
  carry, deliberately: it would be a new failure mode, not a clause the prose already
  states. H-22 compares against the previous release,
  which one report does not carry; H-2 and X-4 inform. They carry no threshold and say so.
* A gate an experiment script runs (``report_path`` None) is evaluated by that script; its
  clause stays prose here.

No clause anywhere is read against a seed-to-seed spread. The display model is one fit
per window: its random seed reached only sklearn's early-stopping split, so the three
"seeds" the harness used to fit were bit-identical below 10,000 rows and the spread they
reported was a zero floor that never bound (EVAL-02). The experiment gates whose clause
named that spread (X-3-stakes, B-7-display-monotone, the X-2 families) say so in place;
each was in practice decided on the fold-level standard error of the paired difference,
which is the noise floor every gate here reads (H-14).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Tuple

#: Prefix for a path read from the whole report rather than from one format's node.
REPORT_SCOPE = "report:"

#: H-17: the walk-forward mean objective AUC under which a format is not offered an
#: optimised selection (plan §8, `optimizer.NOT_OPTIMISED_REASONS`).
H17_MIN_OBJECTIVE_AUC = 0.65
#: H-4: the share of one-player upgrades that may lower the objective's P(win).
H4_MAX_SWAP_VIOLATION_SHARE = 0.02


@dataclass(frozen=True)
class Threshold:
    """A gate's ``decides`` clause as code, beside its number.

    ``failure`` reads the value the report carries at the gate's ``report_path`` and the
    node it sits in (the format's node, or the whole report for a ``report:`` path -- the
    node is what says whether the format is served), and returns why the clause fails, or
    None when it holds. It returns the reason rather than a bool because the reason is
    what the report prints and the operator reads.
    """

    #: The clause a reader can check by hand against the number, rendered beside it.
    rule: str
    failure: Callable[[Any, Dict[str, Any]], Optional[str]]


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
    #: The decides clause evaluated on that number. None where the clause decides an action
    #: the harness already takes, needs a previous release, or informs (the docstring says
    #: which) -- and for every gate a script reports.
    threshold: Optional[Threshold] = None


def _fold_mean(value: Any) -> Optional[float]:
    """The walk-forward mean at a summary path, or None when no fold produced the number."""
    if isinstance(value, dict) and value.get("mean") is not None:
        return float(value["mean"])
    return None


def _optimised_selection_served(node: Dict[str, Any]) -> bool:
    """The serving policy as the format's node records it: ``evaluate`` writes the constant
    `optimizer.OPTIMISED_SELECTION_FORMATS` into E5's decision, so a report is checked
    against what it says was served rather than against whatever the code says now."""
    return bool((node.get("selection_decision") or {}).get("optimised_selection_served"))


def _h17_failure(value: Any, node: Dict[str, Any]) -> Optional[str]:
    if not _optimised_selection_served(node):
        return None
    mean = _fold_mean(value)
    if mean is None:
        return "an optimised selection is served with no walk-forward objective AUC to stand on"
    if mean < H17_MIN_OBJECTIVE_AUC:
        return (
            f"an optimised selection is served on a walk-forward mean objective AUC of {mean:.4f}, "
            f"under the {H17_MIN_OBJECTIVE_AUC} line"
        )
    return None


def _h4_failure(value: Any, node: Dict[str, Any]) -> Optional[str]:
    mean = _fold_mean(value)
    if mean is None:
        # No fold scored the format at all; a format served on that absence fails H-17.
        return None
    if mean >= H4_MAX_SWAP_VIOLATION_SHARE:
        return (
            f"the share of one-player upgrades that lower the objective's P(win) is {mean:.4f}, "
            f"not under {H4_MAX_SWAP_VIOLATION_SHARE}"
        )
    return None


def _specific_vs_typical_failure(value: Any, node: Dict[str, Any]) -> Optional[str]:
    if not _optimised_selection_served(node):
        return None
    mean = _fold_mean(value)
    if mean is None:
        return "an optimised selection is served with no specific-vs-typical delta to stand on"
    if mean <= 0.0:
        return (
            f"an optimised selection is served while the specific eleven adds {mean:+.4f} AUC over the typical "
            "eleven, not above zero"
        )
    return None


def _e5_failure(value: Any, node: Dict[str, Any]) -> Optional[str]:
    if not _optimised_selection_served(node):
        return None
    decision = (node.get("e5_lineup_only") or {}).get("decision") or {}
    if decision.get("passes_derived_bar"):
        return None
    if value is None:
        return "an optimised selection is served with E5 unscored (no pairs whose result moved)"
    return (
        f"an optimised selection is served while E5 lineup-only agreement {value:.4f} does not clear the "
        f"derived bar {decision.get('bar')}"
    )


def _e2_failure(value: Any, node: Dict[str, Any]) -> Optional[str]:
    decision = node.get("simulation_decision") or {}
    if not decision.get("served") or value:
        return None
    return (
        "the simulated P(win) is served as the headline while it is not within tolerance of the display model: "
        f"{decision.get('reason')}"
    )


def _h8_failure(value: Any, node: Dict[str, Any]) -> Optional[str]:
    return None if value is True else "the as-of serving path and the training pass disagree"


GATES: Tuple[Gate, ...] = (
    Gate(
        id="H-17",
        name="Objective ranks (format scope)",
        varies="the two elevens' as-of features across a fold's evaluation matches",
        fixed="the fold's cutoff, the objective fitted before it, the labels",
        decides="walk-forward mean objective AUC >= 0.65, else the format is not offered an optimised selection",
        report_path="walk_forward.summary.objective_auc",
        threshold=Threshold(
            rule="where an optimised selection is served, mean >= 0.65; a served format with no number fails",
            failure=_h17_failure,
        ),
    ),
    Gate(
        id="H-4",
        name="Swap monotonicity",
        varies="one player's scoring, wicket and Elo ratings, raised by one population sd each",
        fixed="the other ten, the opponent eleven, the as-of date, the fold's objective",
        decides="share of upgrades that lower P(win) < 2 %",
        report_path="walk_forward.summary.swap_violation_share",
        threshold=Threshold(rule="mean < 0.02 in every format the folds scored", failure=_h4_failure),
    ),
    Gate(
        id="specific-vs-typical",
        name="Specific XI beyond typical XI",
        varies="whether the objective reads the eleven that played or the side's mean of its last 10 aggregates",
        fixed="the evaluation matches, the labels, the fold's objective",
        decides="AUC(specific) - AUC(typical) > 0",
        report_path="walk_forward.summary.specific_vs_typical_delta",
        # A selection gate (P-5 shipped the XI path on it beside H-4), so it is read as
        # H-17 is: the eleven must add something where an optimised eleven is served.
        threshold=Threshold(
            rule="where an optimised selection is served, mean > 0; a served format with no number fails",
            failure=_specific_vs_typical_failure,
        ),
    ),
    Gate(
        id="E5",
        name="Natural experiment, lineup-only",
        varies="the eleven: match k's against match k+1's, 1-3 players apart",
        fixed="match k+1's opponent eleven, match k+1's as-of ratings, the fold's objective",
        decides="sign agreement between the objective's preference and the result change, over pairs whose "
        "result moved, at or above the bar derived from the objective's own claimed effect size (§8.8)",
        report_path="e5_lineup_only.decision.agreement",
        threshold=Threshold(
            rule="where an optimised selection is served, passes_derived_bar is true; a served format E5 could not "
            "score fails",
            failure=_e5_failure,
        ),
    ),
    Gate(
        id="E2",
        name="Simulated P(win) against the display model",
        varies="which model produces P(win): the simulator's draws or the display model",
        fixed="the fixtures, the as-of forecasts, the fold's models, the labels",
        decides="Brier(simulated) - Brier(display) <= 0.01 on the folds, else the simulation is a description only",
        report_path="simulation_decision.simulated_win_probability_within_tolerance",
        threshold=Threshold(
            rule="where the simulated P(win) is served as the headline, within tolerance is true",
            failure=_e2_failure,
        ),
    ),
    Gate(
        id="H-5",
        name="Quantile coverage",
        varies="the quantile level (0.1 / 0.9) and the target",
        fixed="the evaluation rows, the fold's performance model",
        decides="each level's exceedance sits between its strict and inclusive rate +/- 0.03, else the target is "
        "recalibrated on a temporal fold for the locked window -- and where that window's fold is too thin to "
        "carry a binned empirical quantile, or fitted no model at all, the target goes uncorrected and is named "
        "in locked.recalibration_skipped beside the request in locked.recalibration_requested",
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
        threshold=Threshold(rule="passed is true", failure=_h8_failure),
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
    Gate(
        id="X-1b-age",
        name="Age in the performance model",
        varies="whether the performance model reads the player's age at the match date and the known-age "
        "indicator (AGE_COLS) -- none (today's model) or age -- one fit per arm per fold; a player without a "
        "date of birth reads age 0 with the indicator 0, a category of his own and never an imputed age",
        fixed="the rows (one frame, the age columns on every row, both arms read the same rows), the eleven "
        "quarterly cutoffs (A-4's rotated set), the three seeds, the hyperparameters, the structure per target, "
        "every other input column, the labels; the simulator is not run -- this is a performance-model gate",
        decides="the family is kept only if, against the no-age arm on the same folds, the mean pinball loss of "
        "runs or of wickets improves by more than 0.5 % (E1's noise band) AND by more than one fold-level "
        "standard error of the paired difference, in both T20 and ODI, with every quantile headline target's "
        "10-90 coverage within +/- 0.03 of the control's (H-22; width reported beside it). T20 is decided on "
        "men's rows: women's T20 has a date of birth for 61.4 % of appearances (X-1a) and is out of scope for "
        "the age family, reported beside the verdict and never deciding it. T20I and TEST are reported, not "
        "decided on. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="X-1b-cold-start",
        name="Age-aware cold start: the debutant prior shaped by age",
        varies="what a player with no history in the format and a known age reads from the state -- the "
        "neutral vector (today's cold start) or his age band's as-of debut profile (balls per match and the "
        "four shrunk impacts, pooled over every earlier debutant of that band) -- two passes over the source, "
        "every model refitted per fold on each pass's frame",
        fixed="the source, the dates of birth, the age bands (cuts at 22 / 26 / 30 / 34, the population's "
        "quartiles, chosen before any outcome was read), the cutoffs, the seeds, the hyperparameters, the model "
        "classes, the performance model's columns (family 1's decided setting, the same for both arms), the "
        "labels, and the H-10 probe: the same 50 evaluation matches per fold, the same replaced player (team1's "
        "lowest player Elo), the same probe ages (19 / 27 / 34 / unknown), the end-of-pass debut tables",
        decides="kept only if, in both T20 and ODI: (a) H-10 stays bounded -- for every probe age the arm's "
        "median debutant-swap delta p is within +/- 0.02 of the control's and its 10th percentile within "
        "+/- 0.03 of the control's; (b) the pinball loss of runs or of wickets on the held-out debut rows "
        "(career 0 in the format, the only rows whose own vectors the prior changes) improves against the "
        "control by more than 0.5 % AND more than one fold-level standard error of the paired difference; "
        "(c) the per-player vectors of every row with history are identical between the arms (max abs "
        "difference 0.0 -- a check, not a metric); and, as a guard in every format, the display AUC does not "
        "fall by more than one paired fold-level standard error and H-4's swap share stays under 2 %. T20 is "
        "decided on men's rows, as family 1. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="X-3-e5",
        name="E5 hygiene: the lineup-only measurement with rotation-suspect pairs filtered",
        varies="which of E5's pairs the measurement reads -- all of them (today's E5), those with neither match "
        "a dead rubber, those with neither match a knockout or final, neither of the two, and the same pairs "
        "down-weighted to a half rather than dropped",
        fixed="the rows, the eleven quarterly cutoffs, E5's pairs and the previous eleven read once from the "
        "as-of serving path, the objective (one logistic fit per fold on XI_FEATURE_COLS, the same model every "
        "arm scores with), the lineup-only definition, the bar's derivation (Bernoulli at the objective's own "
        "probabilities, 2,000 replicates, the 5th percentile, three seeds) -- re-derived on each arm's own "
        "weights, because a filter changes the sampling noise as well as the sample",
        decides="nothing automatically -- this gate INFORMS. It says whether T20's E5 null survives removing the "
        "matches where a 1-3 player change is most likely to be rotation rather than selection. An arm that "
        "clears its re-derived bar is evidence for revisiting the T20 scoping through the P-7 machinery, and is "
        "recorded as such; the scoping does not move on this run",
        report_path=None,
    ),
    Gate(
        id="X-3-stakes",
        name="Match stakes in the display model",
        varies="whether the display model reads the two stakes columns (STAKES_COLS: the knockout flag and the "
        "stage-known indicator) beyond XI_FEATURE_COLS + TEAM_CONTEXT_COLS -- one fit per arm per fold",
        fixed="the rows (one frame, the stakes columns on every win row, both arms read the same rows), the "
        "eleven quarterly cutoffs, the display model's one fit per window (EVAL-02 replaced the three-seed "
        "loop, whose members were bit-identical), the hyperparameters, the monotone constraints, the "
        "marginalisation over batting order, the labels; the objective, the performance model and the simulator "
        "are not refitted -- this is a display-model gate",
        decides="the family is kept only if, against the no-stakes arm on the same folds, the mean walk-forward "
        "display AUC rises by more than one fold-level standard error of the paired difference, in every "
        "format, with the swap-violation share of the display surface still under H-4's 2 %. A recorded null "
        "ships nothing. (As run, the clause also named the control's seed-to-seed standard deviation; EVAL-02 "
        "found that spread to be identically zero -- the three seeds were the same fit -- so the standard error "
        "was the floor that bound)",
        report_path=None,
    ),
    Gate(
        id="B-7-display-monotone",
        name="Monotone team context in the display model",
        varies="whether the display model's three directional team-context columns "
        "(team_elo_diff, team_form_diff, venue_fam_diff) carry a +1 monotone constraint or the 0 they "
        "carry today -- one fit per arm per fold",
        fixed="the rows (one frame, both arms read the same rows), the eleven quarterly cutoffs, the display "
        "model's one fit per window, the hyperparameters, the columns the model reads (DISPLAY_FEATURE_COLS is unchanged), "
        "the constraints on every XI column, the marginalisation over batting order, the swap probe (the same "
        "50 evaluation matches per fold, one player's five ratings raised by one population sd, the team "
        "context held at the fixture's values), the labels; the objective, the performance model and the "
        "simulator are not refitted -- this is a display-model gate",
        decides="the constrained display model ships only if BOTH, in every format: (a) the mean walk-forward "
        "display swap-violation share falls by more than one fold-level standard error of the paired "
        "difference AND by at least a quarter of the control's own distance from H-4's 2 % line, so a fall "
        "inside the noise or a fall too small to matter is not a pass; and (b) display AUC falls by no more "
        "than one fold-level standard error of the paired difference (as run, the clause also named the "
        "control's seed-to-seed standard deviation, which EVAL-02 found identically zero: the three seeds were "
        "the same fit). Violations falling while AUC degrades past (b) does NOT ship on "
        "this gate's judgement: it is a product trade-off between a coherent surface and discrimination, and "
        "is recorded with its fold table for the decision to be made deliberately. Neither moving is a "
        "recorded null, and the measurement stays either way",
        report_path=None,
    ),
    Gate(
        id="B-7-pelo-spread",
        name="The one free column the upgrade moves",
        varies="whether the display model reads t1_pelo_std and t2_pelo_std -- the spread of player Elo across "
        "an eleven, the only column in DISPLAY_FEATURE_COLS that a one-player upgrade moves and the monotone "
        "contract leaves free (measured: it moves on 100 % of upgrades in every format, and no constrained "
        "column ever moves against its direction) -- one fit per arm per fold",
        fixed="the rows, the eleven quarterly cutoffs, the display model's one fit per window, the "
        "hyperparameters, every "
        "other column and its constraint, the marginalisation over batting order, the swap probe, the labels; "
        "the objective keeps the column and is not refitted -- H-4 is measured on a linear model that does not "
        "have this problem",
        decides="nothing automatically -- this gate INFORMS. B-7 scoped a constraint on team context, not a "
        "change to what the display model reads, and dropping a column from the display contract moves a "
        "served artifact's feature list. It prices the fix the mechanism actually points at: what the swap "
        "violations and the display AUC would be without the free column, so the trade can be decided "
        "deliberately rather than inferred. It read exactly 0.0000 violations in every format and every fold "
        "for -0.0004 (T20), +0.0088 (T20I), -0.0055 (ODI), -0.0047 (TEST) of display AUC, and the trade was "
        "then TAKEN by decision, not by this gate: the columns are out of DISPLAY_FEATURE_COLS (and, since "
        "FEAT-14, out of the objective's columns too -- no win model reads them), display AUC was spent on a "
        "Team Lab surface that is coherent by construction, and the cost is recorded in docs/BUG_BACKLOG.md § B-7",
        report_path=None,
    ),
    Gate(
        id="X-4",
        name="Market benchmark: the closing price beside the display model",
        varies="which probability is scored -- the market's de-vigged closing price, the display model as "
        "served (marginalised over the toss), or the same display model read at the orientation that "
        "actually happened (the market's own information set)",
        fixed="the matches (only those a closing price joined to), the labels, the walk-forward windows, the "
        "display models the fold itself fitted, the de-vig method and the price point (best back at the "
        "first ball)",
        decides="nothing automatically -- this gate INFORMS. It prices the distance between the display model "
        "and the market on the matches both cover, per format, with the joined coverage printed beside every "
        "number; no feature, threshold or format scoping moves on its result, and odds are never a model "
        "input",
        report_path=REPORT_SCOPE + "market_benchmark.formats",
    ),
    Gate(
        id="X-2-daynight",
        name="Day/night flag in the win and performance models",
        varies="whether the display model and the performance model read the day/night flag (wx_night) -- a fact about the schedule inferred per match from the documented session rules, not a weather reading beyond the "
        "columns each reads today -- one display fit per arm per fold, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the display model's one fit per window and the performance seeds, the hyperparameters, the "
        "monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than one fold-level standard error of the paired difference (as run, the "
        "clause also named the control's seed-to-seed standard deviation, which EVAL-02 found identically zero: "
        "the three seeds were the same fit), with the display swap-violation share under H-4's 2 %; (b) in "
        "T20 and ODI, with the family in the performance model, the simulated first-innings and chase 10-90 "
        "coverage stay within +/- 0.03 of the control's and the widths do not grow, on the day matches and on the "
        "night matches separately (H-22), with no headline pinball worse by more than 0.5 %; (c) in T20 and ODI, "
        "E2 -- Brier(simulated) - Brier(display) -- moves by no more than one fold-level standard error and stays "
        "within its 0.01 tolerance. T20I and TEST are reported. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="X-2-humidity-temperature",
        name="Pre-match humidity and temperature",
        varies="whether the display model and the performance model read the pre-match humidity and temperature (wx_pre_humidity, wx_pre_temp_c, the mean of the three ERA5 hours before the inferred start) with wx_known beyond the "
        "columns each reads today -- one display fit per arm per fold, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the display model's one fit per window and the performance seeds, the hyperparameters, the "
        "monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than one fold-level standard error of the paired difference (as run, the "
        "clause also named the control's seed-to-seed standard deviation, which EVAL-02 found identically zero: "
        "the three seeds were the same fit), with the display swap-violation share under H-4's 2 %; (b) in "
        "T20 and ODI, with the family in the performance model, the simulated first-innings and chase 10-90 "
        "coverage stay within +/- 0.03 of the control's and the widths do not grow, on the day matches and on the "
        "night matches separately (H-22), with no headline pinball worse by more than 0.5 %; (c) in T20 and ODI, "
        "E2 -- Brier(simulated) - Brier(display) -- moves by no more than one fold-level standard error and stays "
        "within its 0.01 tolerance. T20I and TEST are reported. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="X-2-dew",
        name="Dew-likelihood proxy",
        varies="whether the display model and the performance model read the dew proxy (wx_dew_proxy: the night flag times the pre-match relative humidity) with wx_known beyond the "
        "columns each reads today -- one display fit per arm per fold, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the display model's one fit per window and the performance seeds, the hyperparameters, the "
        "monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than one fold-level standard error of the paired difference (as run, the "
        "clause also named the control's seed-to-seed standard deviation, which EVAL-02 found identically zero: "
        "the three seeds were the same fit), with the display swap-violation share under H-4's 2 %; (b) in "
        "T20 and ODI, with the family in the performance model, the simulated first-innings and chase 10-90 "
        "coverage stay within +/- 0.03 of the control's and the widths do not grow, on the day matches and on the "
        "night matches separately (H-22), with no headline pinball worse by more than 0.5 %; (c) in T20 and ODI, "
        "E2 -- Brier(simulated) - Brier(display) -- moves by no more than one fold-level standard error and stays "
        "within its 0.01 tolerance. T20I and TEST are reported. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="X-2-rain",
        name="Rain that has fallen before the match",
        varies="whether the display model and the performance model read the rain already fallen (wx_rain_prior_day_mm: the day before plus the match day's hours before the start; wx_rain_prior_week_mm: the seven days before) with wx_known beyond the "
        "columns each reads today -- one display fit per arm per fold, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the display model's one fit per window and the performance seeds, the hyperparameters, the "
        "monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than one fold-level standard error of the paired difference (as run, the "
        "clause also named the control's seed-to-seed standard deviation, which EVAL-02 found identically zero: "
        "the three seeds were the same fit), with the display swap-violation share under H-4's 2 %; (b) in "
        "T20 and ODI, with the family in the performance model, the simulated first-innings and chase 10-90 "
        "coverage stay within +/- 0.03 of the control's and the widths do not grow, on the day matches and on the "
        "night matches separately (H-22), with no headline pinball worse by more than 0.5 %; (c) in T20 and ODI, "
        "E2 -- Brier(simulated) - Brier(display) -- moves by no more than one fold-level standard error and stays "
        "within its 0.01 tolerance. T20I and TEST are reported. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="SIM-DN-split",
        name="Day/night-conditional shared factor: one residual pool per population",
        varies="which residual pool the simulator's shared match factor is sampled from -- one pool fitted from "
        "every calibration match (the control, today's simulator) or one pool per pre-match day/night population, "
        "each fitted and deconvolved from its own calibration matches under the same rule and the same 30-match "
        "guard, the fixture drawing from the pool of the population it belongs to; a population under the guard "
        "falls back to the pooled factor and the fold records it",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three performance seeds, the "
        "hyperparameters, the performance model (one fit per fold, shared by the arms, so every headline pinball is "
        "identical by construction), the display models the simulator is scored against (the control's, fitted once "
        "per fold and shared by the arms, so the display AUC is identical by construction), the 92-day calibration "
        "fold and the calibration draws the factor is fitted from, the chase response (none), the simulator's draw "
        "count and its seeds (common random numbers across arms), the day/night label itself (inferred pre-match "
        "from ml.weather.sessions' documented session rules, H-21), the labels",
        decides="in T20, paired per fold against the control with one fold-level standard error as the floor: the "
        "first-innings 10-90 coverage's distance from 0.80 shrinks on the day matches and on the night matches "
        "separately, the first-innings dispersion ratio's distance from 1.0 shrinks in both populations, the "
        "match-count-pooled first-innings width does not grow by more than 1 % (H-22: coverage bought by inflating "
        "the interval fails), the chase coverage's distance from 0.80 is no worse by more than one standard error "
        "in either population, and E2 -- Brier(simulated) - Brier(display) -- moves by no more than one fold-level "
        "standard error and stays within its 0.01 tolerance. ODI cannot decide it (its night side clears the "
        "simulator's 20-match floor in 2 of 11 folds and its night calibration fold clears the 30-match guard in "
        "1) and is reported. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="SIM-DN-scale",
        name="Day/night-scaled shared factor: a scale mixture of one pooled shape",
        varies="which residual pool the simulator's shared match factor is sampled from -- one pool fitted from "
        "every calibration match (the control, today's simulator) or that same pool rescaled per pre-match "
        "day/night population, each population's deviations from one multiplied by sqrt(that population's excess "
        "variance / the pooled excess variance): the same shape and the same matches, spread varied and location "
        "left pooled, under a 15-match floor per population because a scale is one moment rather than a "
        "distribution (at 15 matches a variance's own relative standard error, sqrt(2/(n-1)) = 38 %, is under half "
        "the ~60 % variance gap between the two populations that X-2's dispersion ratios imply); a population "
        "under the floor falls back to the pooled factor and the fold records it",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three performance seeds, the "
        "hyperparameters, the performance model (one fit per fold, shared by the arms, so every headline pinball is "
        "identical by construction), the display models the simulator is scored against (the control's, fitted once "
        "per fold and shared by the arms, so the display AUC is identical by construction), the 92-day calibration "
        "fold and the calibration draws the factor is fitted from, the chase response (none), the simulator's draw "
        "count and its seeds (common random numbers across arms), the day/night label itself (inferred pre-match "
        "from ml.weather.sessions' documented session rules, H-21), the labels",
        decides="in T20, paired per fold against the control with one fold-level standard error as the floor: the "
        "first-innings 10-90 coverage's distance from 0.80 shrinks on the day matches and on the night matches "
        "separately, the first-innings dispersion ratio's distance from 1.0 shrinks in both populations, the "
        "match-count-pooled first-innings width does not grow by more than 1 % (H-22: coverage bought by inflating "
        "the interval fails), the chase coverage's distance from 0.80 is no worse by more than one standard error "
        "in either population, and E2 -- Brier(simulated) - Brier(display) -- moves by no more than one fold-level "
        "standard error and stays within its 0.01 tolerance. ODI cannot decide it (its night side clears the "
        "simulator's 20-match floor in 2 of 11 folds and its night calibration fold clears the 15-match floor in "
        "3, never in the same fold) and is reported. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="SIM-IN-chase",
        name="A dispersion term the two innings do not share: the chase's own, alone",
        varies="whether the chasing side's runs draws carry a second dispersion factor beside the shared match "
        "factor -- drawn independently per draw with mean one, so it adds spread and no level (A-2 gated the chase "
        "*level* and recorded a null), its log spread the excess of the chase's fitted residual scale over what the "
        "draws themselves produce on the same calibration matches, deconvolved as fit_shared_factor deconvolves the "
        "first innings and fitted by A-2's censored (Tobit) estimator because roughly half the calibration chases "
        "are won and so right-censored at the target; the shared match factor is the control's, pooled, in this arm",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three performance seeds, the "
        "hyperparameters, the performance model (one fit per fold, shared by the arms, so every headline pinball is "
        "identical by construction), the display models the simulator is scored against (the control's, fitted once "
        "per fold and shared by the arms, so the display AUC is identical by construction), the 92-day calibration "
        "fold and the calibration draws both terms are fitted from, the chase response (none), the simulator's draw "
        "count and its seeds (common random numbers across arms), the day/night label itself (inferred pre-match "
        "from ml.weather.sessions' documented session rules, H-21), the labels. The first innings' draws are taken "
        "before the chase's from the same stream and are bit-identical to the control's, so its coverage cannot "
        "move in this arm -- that is what makes it the isolating arm, and it is registered as one",
        decides="in T20, paired per fold against the control with one fold-level standard error as the effect-size "
        "floor, and in BOTH populations and BOTH innings, because §8.13's two candidates failed exactly by buying "
        "the first innings with the chase: the 10-90 coverage's distance from 0.80 shrinks for the first innings "
        "and for the chase, on the day matches and on the night matches separately (four clauses), and the "
        "dispersion ratio's distance from 1.0 shrinks in the same four cells. H-22, with the sign each side needs: "
        "the match-count-pooled FIRST-INNINGS width may not grow by more than 1 %, because §8.13 showed that "
        "correction is a reallocation and not an inflation; the CHASE has no width cap because its correction is by "
        "design a widening, and the guard against buying its coverage with width is its own dispersion clause, "
        "which a mere inflation would push past 1.0 the other way. Width is reported beside coverage in every cell "
        "either way. E2 states its sign rather than being symmetric (this family has now tripped three times on an "
        "improvement -- §8.9, §8.10, §X-2): e2_not_degraded fails only if Brier(simulated) - Brier(display) GROWS "
        "by more than one fold-level standard error, or leaves its 0.01 tolerance; a fall passes. ODI cannot decide "
        "it (its night side clears the simulator's 20-match floor in 2 of 11 folds) and is reported. A recorded "
        "null ships nothing",
        report_path=None,
    ),
    Gate(
        id="SIM-IN-both",
        name="A dispersion term the two innings do not share, with the shared factor scaled per population",
        varies="both levers at once: the chase's own dispersion factor exactly as SIM-IN-chase fits it, and the "
        "shared match factor rescaled per pre-match day/night population under §8.13's SIM-DN-scale rule (each "
        "population's deviations from one multiplied by sqrt(that population's excess variance / the pooled excess "
        "variance), one shape borrowed across both, spread varied and location left pooled, under a 15-match floor "
        "per population; a population under the floor falls back to the pooled factor and the fold records it). "
        "The two are composed because they correct different halves of one defect and neither can pass alone: "
        "§8.13's population lever moves the first innings and was blocked only by the chase, and the chase term "
        "leaves the first innings bit-identical by construction",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three performance seeds, the "
        "hyperparameters, the performance model (one fit per fold, shared by the arms, so every headline pinball is "
        "identical by construction), the display models the simulator is scored against (the control's, fitted once "
        "per fold and shared by the arms, so the display AUC is identical by construction), the 92-day calibration "
        "fold and the calibration draws both terms are fitted from, the chase response (none), the simulator's draw "
        "count and its seeds (common random numbers across arms), the day/night label itself (inferred pre-match "
        "from ml.weather.sessions' documented session rules, H-21), the labels",
        decides="the same clauses as SIM-IN-chase, on the same folds, with the same signs: in T20, paired per fold "
        "against the control with one fold-level standard error as the floor, the 10-90 coverage's distance from "
        "0.80 and the dispersion ratio's distance from 1.0 both shrink for the first innings and for the chase, by "
        "day and by night (eight clauses); the pooled first-innings width does not grow by more than 1 % and the "
        "chase's width is guarded by its dispersion clause rather than a cap; e2_not_degraded is signed -- only a "
        "growth of more than one fold-level standard error, or leaving the 0.01 tolerance, fails it. ODI cannot "
        "decide it and is reported. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="SIM-IN-corr",
        name="The chase's dispersion, correlated with the first innings' realised residual",
        varies="how the chasing side's second dispersion factor is drawn, and what it is fitted against. §8.14's "
        "term was drawn INDEPENDENTLY of everything else and fitted against the chase's expectation on the shared "
        "factor's own pitch; this one is drawn so that it moves with the FIRST INNINGS' realised log residual in "
        "the same draw -- exp(slope * (ln T1 - the draws' mean ln T1) + independent_log_sd * z), centred to mean "
        "one per fixture so it adds spread and no level -- and is fitted against the chase's OWN expectation, with "
        "the pitch a thing to be explained rather than divided out. Both coefficients are a difference between the "
        "data and the control: the censored (Tobit) regression of the calibration fold's chase residual on its "
        "first innings' residual gives the data's slope and residual scale, the control's own slope (Vf / V1) and "
        "residual variance (V2 - Vf^2 / V1) are composed from the shared factor's log variance and each innings' "
        "draw spread, and the term carries the difference. §8.14's null was E2 and its mechanism was understood: "
        "an independent term widens the MARGIN, which decides the match, so P(win) moves toward 0.5. A correlated "
        "term puts the extra spread into the two totals together, where it largely cancels in their difference. "
        "The shared match factor is the control's, pooled, in this arm",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three performance seeds, the "
        "hyperparameters, the performance model (one fit per fold, shared by the arms, so every headline pinball is "
        "identical by construction), the display models the simulator is scored against (the control's, fitted once "
        "per fold and shared by the arms, so the display AUC is identical by construction), the 92-day calibration "
        "fold and the calibration draws every term is fitted from, the chase response (none), the simulator's draw "
        "count and its seeds (common random numbers across arms), the day/night label itself (inferred pre-match "
        "from ml.weather.sessions' documented session rules, H-21), the labels. The first innings' draws are taken "
        "before the chase's from the same stream and are bit-identical to the control's, so its coverage cannot "
        "move in this arm -- that is what makes it the isolating arm, and it is registered as one",
        decides="in T20, paired per fold against the control with one fold-level standard error as the effect-size "
        "floor, and in BOTH populations and BOTH innings, because §8.13's candidates failed by buying the first "
        "innings with the chase and §8.14's by buying the totals with the margin: the 10-90 coverage's distance "
        "from 0.80 shrinks for the first innings and for the chase, on the day matches and on the night matches "
        "separately (four clauses), and the dispersion ratio's distance from 1.0 shrinks in the same four cells. "
        "H-22 with the sign each side needs: the match-count-pooled FIRST-INNINGS width may not grow by more than "
        "1 %; the CHASE has no width cap because its correction is by design a widening, and the guard against "
        "buying its coverage with width is its own dispersion clause, which a mere inflation would push past 1.0 "
        "the other way. Width is reported beside coverage in every cell either way. E2 is SIGNED and is the clause "
        "this candidate exists to pass: e2_not_degraded fails only if Brier(simulated) - Brier(display) GROWS by "
        "more than one fold-level standard error, or leaves its 0.01 tolerance; a fall passes. ODI cannot decide "
        "it (its night side clears the simulator's 20-match floor in 2 of 11 folds) and is reported. A recorded "
        "null ships nothing",
        report_path=None,
    ),
    Gate(
        id="SIM-IN-corrboth",
        name="The correlated chase dispersion, with the shared factor scaled per population",
        varies="both levers at once: the chase's correlated dispersion exactly as SIM-IN-corr fits it, and the "
        "shared match factor rescaled per pre-match day/night population under §8.13's SIM-DN-scale rule (each "
        "population's deviations from one multiplied by sqrt(that population's excess variance / the pooled excess "
        "variance), one shape borrowed across both, spread varied and location left pooled, under a 15-match floor "
        "per population; a population under the floor falls back to the pooled factor and the fold records it). "
        "The two are composed because they correct different halves of one defect and neither can pass alone: "
        "§8.13's population lever moves the first innings and was blocked only by the chase, and the chase term "
        "leaves the first innings bit-identical by construction",
        fixed="the rows, the eleven quarterly cutoffs (A-4's rotated set), the three performance seeds, the "
        "hyperparameters, the performance model (one fit per fold, shared by the arms, so every headline pinball is "
        "identical by construction), the display models the simulator is scored against (the control's, fitted once "
        "per fold and shared by the arms, so the display AUC is identical by construction), the 92-day calibration "
        "fold and the calibration draws every term is fitted from, the chase response (none), the simulator's draw "
        "count and its seeds (common random numbers across arms), the day/night label itself (inferred pre-match "
        "from ml.weather.sessions' documented session rules, H-21), the labels",
        decides="the same clauses as SIM-IN-corr, on the same folds, with the same signs: in T20, paired per fold "
        "against the control with one fold-level standard error as the floor, the 10-90 coverage's distance from "
        "0.80 and the dispersion ratio's distance from 1.0 both shrink for the first innings and for the chase, by "
        "day and by night (eight clauses); the pooled first-innings width does not grow by more than 1 % and the "
        "chase's width is guarded by its dispersion clause rather than a cap; e2_not_degraded is signed -- only a "
        "growth of more than one fold-level standard error, or leaving the 0.01 tolerance, fails it. ODI cannot "
        "decide it and is reported. A recorded null ships nothing",
        report_path=None,
    ),
)

REGISTRY: Dict[str, Gate] = {gate.id: gate for gate in GATES}


def as_dict() -> Dict[str, Dict[str, Any]]:
    """The registry as the report embeds it: the triple, the path, and the threshold's rule
    (a clause is code, and the report carries what a reader can check by hand)."""
    return {
        gate.id: {
            "id": gate.id,
            "name": gate.name,
            "varies": gate.varies,
            "fixed": gate.fixed,
            "decides": gate.decides,
            "report_path": gate.report_path,
            "threshold": None if gate.threshold is None else gate.threshold.rule,
        }
        for gate in GATES
    }


def describe(gate_id: str) -> str:
    """One line a script prints before it runs a gate: the triple, in the H-23 order."""
    gate = REGISTRY[gate_id]
    return f"{gate.id} ({gate.name}) - varies: {gate.varies}; fixed: {gate.fixed}; decides: {gate.decides}"


_MISSING = object()


def _value_at(node: Any, path: str) -> Any:
    """The value at a dotted path, or ``_MISSING`` when the path is not carried."""
    for key in path.split("."):
        if not isinstance(node, dict) or key not in node:
            return _MISSING
        node = node[key]
    return node


def _check_gate_at(gate: Gate, node: Dict[str, Any], path: str, where: str) -> Optional[str]:
    """One gate against one node: the path must be carried, and the clause must hold."""
    value = _value_at(node, path)
    if value is _MISSING:
        return f"gate {gate.id}: {where} carries nothing at {gate.report_path}"
    if gate.threshold is None:
        return None
    failure = gate.threshold.failure(value, node)
    return None if failure is None else f"gate {gate.id}: {where} fails its threshold: {failure}"


def check_report(report: Dict[str, Any]) -> List[str]:
    """Every registered gate with a report path must be carried by the report -- at the
    top level or under every format -- every gate must declare a non-empty triple, and
    every standing gate's threshold must hold on the number the report carries. Returns
    the problems; an empty list is a report that says what its gates vary and whose
    gates all pass."""
    problems: List[str] = []
    for gate in GATES:
        for field in ("varies", "fixed", "decides"):
            if not getattr(gate, field).strip():
                problems.append(f"gate {gate.id} declares no '{field}'")
        if gate.report_path is None:
            continue
        if gate.report_path.startswith(REPORT_SCOPE):
            problem = _check_gate_at(gate, report, gate.report_path[len(REPORT_SCOPE) :], "report")
            if problem:
                problems.append(problem)
            continue
        for format_code, node in report.get("formats", {}).items():
            problem = _check_gate_at(gate, node, gate.report_path, format_code)
            if problem:
                problems.append(problem)
    embedded = report.get("gates", {}).get("registry")
    if embedded is not None and set(embedded) != set(REGISTRY):
        problems.append("the report's embedded registry does not match the code's")
    return problems
