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
        "stage-known indicator) beyond XI_FEATURE_COLS + TEAM_CONTEXT_COLS -- one fit per arm per fold per seed",
        fixed="the rows (one frame, the stakes columns on every win row, both arms read the same rows), the "
        "eleven quarterly cutoffs, the three display seeds, the hyperparameters, the monotone constraints, the "
        "marginalisation over batting order, the labels; the objective, the performance model and the simulator "
        "are not refitted -- this is a display-model gate",
        decides="the family is kept only if, against the no-stakes arm on the same folds, the mean walk-forward "
        "display AUC rises by more than both the control's own seed-to-seed standard deviation and one "
        "fold-level standard error of the paired difference, in every format, with the swap-violation share of "
        "the display surface still under H-4's 2 %. A recorded null ships nothing",
        report_path=None,
    ),
    Gate(
        id="B-7-display-monotone",
        name="Monotone team context in the display model",
        varies="whether the display model's three directional team-context columns "
        "(team_elo_diff, team_form_diff, venue_fam_diff) carry a +1 monotone constraint or the 0 they "
        "carry today -- one fit per arm per fold per seed",
        fixed="the rows (one frame, both arms read the same rows), the eleven quarterly cutoffs, the three "
        "display seeds, the hyperparameters, the columns the model reads (DISPLAY_FEATURE_COLS is unchanged), "
        "the constraints on every XI column, the marginalisation over batting order, the swap probe (the same "
        "50 evaluation matches per fold, one player's five ratings raised by one population sd, the team "
        "context held at the fixture's values), the labels; the objective, the performance model and the "
        "simulator are not refitted -- this is a display-model gate",
        decides="the constrained display model ships only if BOTH, in every format: (a) the mean walk-forward "
        "display swap-violation share falls by more than one fold-level standard error of the paired "
        "difference AND by at least a quarter of the control's own distance from H-4's 2 % line, so a fall "
        "inside the noise or a fall too small to matter is not a pass; and (b) display AUC falls by no more "
        "than one fold-level standard error of the paired difference and no more than the control's "
        "seed-to-seed standard deviation. Violations falling while AUC degrades past (b) does NOT ship on "
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
        "column ever moves against its direction) -- one fit per arm per fold per seed",
        fixed="the rows, the eleven quarterly cutoffs, the three display seeds, the hyperparameters, every "
        "other column and its constraint, the marginalisation over batting order, the swap probe, the labels; "
        "the objective keeps the column and is not refitted -- H-4 is measured on a linear model that does not "
        "have this problem",
        decides="nothing automatically -- this gate INFORMS. B-7 scoped a constraint on team context, not a "
        "change to what the display model reads, and dropping a column from the display contract moves a "
        "served artifact's feature list. It prices the fix the mechanism actually points at: what the swap "
        "violations and the display AUC would be without the free column, so the trade can be decided "
        "deliberately rather than inferred. Nothing ships on it in this item",
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
        "columns each reads today -- one display fit per arm per fold per seed, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the three display seeds and the performance seeds, the hyperparameters, the monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than both the control's seed-to-seed standard deviation and one fold-level "
        "standard error of the paired difference, with the display swap-violation share under H-4's 2 %; (b) in "
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
        "columns each reads today -- one display fit per arm per fold per seed, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the three display seeds and the performance seeds, the hyperparameters, the monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than both the control's seed-to-seed standard deviation and one fold-level "
        "standard error of the paired difference, with the display swap-violation share under H-4's 2 %; (b) in "
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
        "columns each reads today -- one display fit per arm per fold per seed, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the three display seeds and the performance seeds, the hyperparameters, the monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than both the control's seed-to-seed standard deviation and one fold-level "
        "standard error of the paired difference, with the display swap-violation share under H-4's 2 %; (b) in "
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
        "columns each reads today -- one display fit per arm per fold per seed, one performance fit per arm per fold",
        fixed="the rows (one frame; the weather columns joined by (venue, match day) onto every win row and player "
        "row from the cached ERA5 days, the session window inferred by the documented rules), the eleven quarterly "
        "cutoffs, the three display seeds and the performance seeds, the hyperparameters, the monotone constraints, "
        "the shared factor's fitting rule, the display models the simulator is scored against (the control's, "
        "fitted once per fold and shared by the arms), the simulator, its draw count and its seeds (common random "
        "numbers across arms), the labels; every column is fixed before the first ball (H-21)",
        decides="kept only if, against the control on the same folds: (a) in every format the mean walk-forward "
        "display AUC rises by more than both the control's seed-to-seed standard deviation and one fold-level "
        "standard error of the paired difference, with the display swap-violation share under H-4's 2 %; (b) in "
        "T20 and ODI, with the family in the performance model, the simulated first-innings and chase 10-90 "
        "coverage stay within +/- 0.03 of the control's and the widths do not grow, on the day matches and on the "
        "night matches separately (H-22), with no headline pinball worse by more than 0.5 %; (c) in T20 and ODI, "
        "E2 -- Brier(simulated) - Brier(display) -- moves by no more than one fold-level standard error and stays "
        "within its 0.01 tolerance. T20I and TEST are reported. A recorded null ships nothing",
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
