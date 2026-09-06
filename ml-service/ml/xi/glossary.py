"""L-1: what every reported number means, beside the code that computes it.

`auc 0.723`, `dispersion ratio 1.02`, `pinball 3.00` are correct and illegible: they mean
nothing to a reader who has not lived inside the plan. This module is the glossary that
makes them legible -- one entry per reported metric key, carrying a plain-language name,
an explanation a non-statistician can read, the reference band **this system measured**
(not textbook folklore), which direction is better, and -- where a defensible anchor
exists -- that same band as two numbers, so a surface can paint the value instead of
printing the sentence.

It lives here, next to the metrics, for the reason `gates.py` lives here: the harness
embeds it in ``xi_evaluate_report.json`` and the service serves it, so every surface
renders one source and the frontend holds no metric prose of its own (H-24). The copy is
the table in ``docs/FOLLOW_UP_PLAN.md`` § 3, which is where a rewording starts.

``check_report`` is the completeness gate. It walks the report and fails on any metric key
that neither has an entry here nor is declared in ``NON_METRIC_KEYS`` -- so a new metric
cannot ship unexplained, and a new count cannot slip in as a metric without someone saying
which it is. Walking stops at a key that has an entry: an entry explains its whole node,
whether that node is a number, a fold summary or a small block of related rates.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any, Dict, List, Sequence, Tuple

#: The direction that is better, as the popover prints it.
HIGHER = "higher is better"
LOWER = "lower is better"
NOMINAL = "closer to the nominal is better"
PAIRED = "no good direction alone -- read it beside its pair"
EXACT = "one value is right, everything else is a bug"

#: The same judgement in one word, for a surface that has to colour a change rather than
#: print a sentence. It is derived from ``better`` so the two can never disagree.
DIRECTIONS = {HIGHER: "higher", LOWER: "lower", NOMINAL: "nominal", PAIRED: "none", EXACT: "exact"}


@dataclass(frozen=True)
class Scale:
    """The two anchors a surface paints a value between, read with ``direction``.

    ``good`` is where the colour is fully green and ``bad`` where it is fully red, so one
    pair covers ``higher`` (``bad`` below ``good``) and ``lower`` (``bad`` above it). For
    ``nominal`` the value is read by its distance from ``good``, and ``bad`` is the
    distance at which it is fully wrong; for ``exact``, only ``good`` is right.

    The anchors are this system's measured range or a gate's threshold -- the same
    judgement ``band`` states in prose -- so a colour cannot disagree with the sentence
    beside it. A metric that means nothing without its pair carries no scale and is
    painted no colour, which is the honest answer rather than a shade of grey.
    """

    bad: float
    good: float


@dataclass(frozen=True)
class Metric:
    """One reported metric key, explained.

    ``band`` is the reference the value is read against: this system's measured range,
    the gate's threshold, or the baseline it must beat. ``better`` says which way is
    progress, in words, because a surface that shows a delta has to colour it, and
    ``scale`` says the same in numbers for the surface that actually does.
    """

    key: str
    name: str
    explanation: str
    band: str
    better: str
    #: The anchors ``band`` states in prose, for a surface that paints the value rather
    #: than printing the sentence. ``None`` where no defensible anchor exists.
    scale: "Scale | None" = None

    @property
    def direction(self) -> str:
        """Which way is progress, in one word: ``higher``, ``lower``, ``nominal``,
        ``exact``, or ``none`` for a number that means nothing without its pair."""
        return DIRECTIONS.get(self.better, "none")


# --- Shared copy -----------------------------------------------------------------
# One family, one explanation: `objective_auc` and `display_auc` are the same statistic
# of two different models, and two copies of the sentence would drift apart.

AUC_EXPLANATION = (
    "Of two random opposing claims, how often the model ranks the actual winner higher. "
    "0.5 is a coin flip and 1.0 is perfect."
)
AUC_BAND = (
    "0.50 chance; 0.55 weak; 0.65+ useful (H-17's selection line); 0.70-0.75 is this system's "
    "measured range and the practical ceiling for cricket. Treat above 0.80 as a red flag for "
    "leakage, not brilliance."
)
BRIER_EXPLANATION = (
    "Squared error of the stated probability, averaged over matches. It rewards a probability "
    "that is both right and confident, and punishes confident and wrong."
)
BRIER_BAND = (
    "Must beat the base-rate Brier printed beside it (about 0.25 here); this system reaches "
    "0.20-0.22. The comparison is the point, not the absolute number."
)
SPEARMAN_EXPLANATION = (
    "Rank agreement between predicted and actual player performance inside one match: did the "
    "model put the right players at the top of the card, whatever the absolute numbers were."
)
SPEARMAN_BAND = (
    "About 0.35 is the ceiling the game itself sets; this system reaches about 0.33. It is not "
    "expected to climb much further."
)
PINBALL_EXPLANATION = (
    "The proper score for a quantile forecast: it is minimised only by the true quantiles, so a "
    "model cannot buy a better score by narrowing or widening its intervals."
)
PINBALL_BAND = "Only meaningful against the career-quantile baseline printed beside it; lower is better."
PIT_TAIL_EXPLANATION = (
    "The share of outcomes that fell below the stated 10th percentile, or above the stated 90th. "
    "It is how an interval fails: both tails fat means the intervals are too narrow."
)
PIT_TAIL_BAND = "0.10 at each tail when calibrated; both well above 0.10 means the intervals are too narrow."
E5_EXPLANATION = (
    "When only the eleven changed between one side's consecutive matches, how often the model's "
    "preference between the two elevens matched the direction the result moved."
)
E5_BAND = (
    "Read against the derived bar printed beside it (about 0.50-0.51): an exactly-right model "
    "scores only about 0.52 on these pairs, so small margins are expected and the bar accounts "
    "for that."
)
PARITY_BAND = "Exactly 0.0. Anything else is a bug, not a shade of grey."
MARKET_BAND = (
    "Read it beside the joined coverage printed with it, and beside the other arm on the same "
    "matches -- never on its own. It is a different, usually much smaller, set of matches than the "
    "format's headline numbers."
)
MARKET_DELTA_BAND = (
    "Zero is parity with the market. Read it through the 95 % interval beside it: an interval "
    "spanning zero means the two are indistinguishable on the matches scored."
)

#: The anchors behind the shared bands above. Each is the sentence as two numbers: an
#: AUC is painted from chance to the measured ceiling, a Brier from the base rate it
#: must beat to what this system reaches, and so on. Where the band gives a gate
#: threshold rather than a range, the threshold is the red end.
AUC_SCALE = Scale(bad=0.50, good=0.75)
BRIER_SCALE = Scale(bad=0.25, good=0.18)
SPEARMAN_SCALE = Scale(bad=0.0, good=0.35)
PIT_TAIL_SCALE = Scale(bad=0.20, good=0.10)
E5_SCALE = Scale(bad=0.50, good=0.52)
PARITY_SCALE = Scale(bad=0.0, good=0.0)
SEED_SD_SCALE = Scale(bad=0.010, good=0.0)
VIOLATION_SCALE = Scale(bad=0.02, good=0.0)
#: The display surface used to be painted against its own 3-7 % range, because H-4's 2 %
#: is a contract on the objective and a scale saturating red everywhere says nothing about
#: which format is worse. Since B-7's decision the surface reads exactly 0.0000 in every
#: format, so the reader's question changed: any nonzero value is a regression in the one
#: column the display model no longer sees, and it should paint red as early as H-4's does.
DISPLAY_VIOLATION_SCALE = Scale(bad=0.02, good=0.0)
SPECIFIC_XI_SCALE = Scale(bad=0.0, good=0.02)
COVERAGE_SCALE = Scale(bad=0.65, good=0.80)
E2_TOLERANCE_SCALE = Scale(bad=0.01, good=0.0)
#: X-4's gap is painted from "the market is 0.05 of AUC ahead" to parity with it.
MARKET_DELTA_SCALE = Scale(bad=0.05, good=0.0)


METRICS: Tuple[Metric, ...] = (
    # --- The win models: how well a probability ranks and how well it is stated ---
    Metric(
        key="objective_auc",
        name="Objective AUC",
        explanation="The value the optimiser maximises when it picks an eleven. " + AUC_EXPLANATION,
        band=AUC_BAND,
        better=HIGHER,
        scale=AUC_SCALE,
    ),
    Metric(
        key="display_auc",
        name="Display AUC",
        explanation="The same measure for the probability a user is actually shown. " + AUC_EXPLANATION,
        band=AUC_BAND,
        better=HIGHER,
        scale=AUC_SCALE,
    ),
    Metric(
        key="display_auc_mean",
        name="Display AUC (mean over seeds)",
        explanation=(
            "The displayed model is fitted under several random seeds and its AUC averaged, so a "
            "lucky seed cannot be read as an improvement. " + AUC_EXPLANATION
        ),
        band=AUC_BAND,
        better=HIGHER,
        scale=AUC_SCALE,
    ),
    Metric(
        key="display_auc_seed_sd",
        name="Display AUC spread across seeds",
        explanation=(
            "How far the displayed model's AUC moves when only the random seed changes. It is the "
            "noise floor: a change smaller than this is not evidence of anything (H-14)."
        ),
        band="A few thousandths here. Any claimed improvement must be larger than it.",
        better=LOWER,
        scale=SEED_SD_SCALE,
    ),
    Metric(
        key="display_auc_seed_sd_mean",
        name="Display AUC seed spread, mean over folds",
        explanation=(
            "The seed-to-seed spread of the displayed AUC, averaged over the walk-forward folds. "
            "It is the size below which a difference between releases means nothing."
        ),
        band="A few thousandths here. Any claimed improvement must be larger than it.",
        better=LOWER,
        scale=SEED_SD_SCALE,
    ),
    Metric(
        key="objective_brier",
        name="Objective Brier",
        explanation="The objective's probability, scored rather than ranked. " + BRIER_EXPLANATION,
        band=BRIER_BAND,
        better=LOWER,
        scale=BRIER_SCALE,
    ),
    Metric(
        key="display_brier_mean",
        name="Display Brier (mean over seeds)",
        explanation="The displayed probability, scored rather than ranked, averaged over seeds. " + BRIER_EXPLANATION,
        band=BRIER_BAND,
        better=LOWER,
        scale=BRIER_SCALE,
    ),
    Metric(
        key="brier",
        name="Brier score",
        explanation=BRIER_EXPLANATION,
        band=BRIER_BAND,
        better=LOWER,
        scale=BRIER_SCALE,
    ),
    Metric(
        key="base_rate_brier",
        name="Base-rate Brier",
        explanation=(
            "The Brier score of predicting the training base rate for every match -- the score to "
            "beat. A model that cannot beat it has learned nothing about the fixture."
        ),
        band="About 0.25 for a near-even base rate. The model's Brier must sit below it.",
        better=PAIRED,
    ),
    # --- Selection: does knowing the eleven help, and does the model rank elevens? ---
    Metric(
        key="specific_vs_typical_delta",
        name="Specific-XI delta",
        explanation=(
            "Extra win-prediction accuracy from knowing the exact eleven, against knowing only the "
            "side's typical eleven. Above zero means the model reads the players, not the badge."
        ),
        band="Positive means the eleven carries signal; measured +0.012 +/- 0.010 (T20) -- real but small.",
        better=HIGHER,
        scale=SPECIFIC_XI_SCALE,
    ),
    Metric(
        key="delta",
        name="Specific-XI delta",
        explanation=(
            "The AUC of the eleven that played minus the AUC of the side's typical eleven, in one "
            "fold. Above zero means the model reads the players, not the badge."
        ),
        band="Positive means the eleven carries signal; measured +0.012 +/- 0.010 (T20) -- real but small.",
        better=HIGHER,
        scale=SPECIFIC_XI_SCALE,
    ),
    Metric(
        key="auc_specific_xi",
        name="AUC of the eleven that played",
        explanation="The objective scored on the actual elevens. " + AUC_EXPLANATION,
        band=AUC_BAND,
        better=HIGHER,
        scale=AUC_SCALE,
    ),
    Metric(
        key="auc_typical_xi",
        name="AUC of the side's typical eleven",
        explanation=(
            "The same objective scored on the mean of the side's last ten aggregates instead of the "
            "eleven that played -- the control the specific-XI delta is measured against. " + AUC_EXPLANATION
        ),
        band=AUC_BAND,
        better=PAIRED,
    ),
    Metric(
        key="swap_violation_share",
        name="Swap violations",
        explanation=(
            "How often upgrading one player -- raising his ratings, leaving the other ten and the "
            "opponent alone -- *lowers* the predicted win chance. Each one is the model contradicting "
            "itself about what a better player is."
        ),
        band="Under 2 % passes (H-4); this system measures under 1 %.",
        better=LOWER,
        scale=VIOLATION_SCALE,
    ),
    Metric(
        key="violation_share",
        name="Swap violations",
        explanation=(
            "How often upgrading one player -- raising his ratings, leaving the other ten and the "
            "opponent alone -- *lowers* the predicted win chance, in one fold."
        ),
        band="Under 2 % passes (H-4); this system measures under 1 %.",
        better=LOWER,
        scale=VIOLATION_SCALE,
    ),
    Metric(
        key="display_swap_violation_share",
        name="Swap violations, display surface",
        explanation=(
            "The same one-player upgrade, scored on the model a person actually watches -- the "
            "number that moves in the Team Lab when a player is swapped in. The team context is "
            "held exactly as the fixture had it, because a selector cannot change it. Each "
            "violation is a swap that made the eleven better and the displayed win chance worse."
        ),
        band=(
            "0.0000 in every format: since B-7's decision the display model no longer reads the "
            "spread of player Elo across an eleven (t1_pelo_std / t2_pelo_std), the one column an "
            "upgrade moved that nothing constrained, so every upgrade now raises the displayed "
            "probability by construction. It used to measure 3-7 %. Still not a gate -- H-4's 2 % "
            "line is a contract on the objective the optimiser maximises -- but anything above "
            "0.0000 here is a regression. It cost display AUC to buy: -0.0055 ODI, -0.0047 TEST, "
            "-0.0004 T20, +0.0088 T20I (B-7)."
        ),
        better=LOWER,
        scale=DISPLAY_VIOLATION_SCALE,
    ),
    Metric(
        key="display_swap_monotonicity",
        name="Swap violations, display surface (one fold)",
        explanation=(
            "One fold's count of the same probe: how many one-player upgrades were tried on the "
            "displayed surface, how many lowered the displayed win chance, and their share."
        ),
        band=(
            "Zero violations in every fold since B-7 took the spread of player Elo out of the "
            "display columns. A fold above zero means an unconstrained column an upgrade moves has "
            "come back. H-4's 2 % line is the objective's contract, not this surface's (B-7)."
        ),
        better=LOWER,
        scale=DISPLAY_VIOLATION_SCALE,
    ),
    Metric(
        key="agreement",
        name="E5 lineup agreement",
        explanation=E5_EXPLANATION,
        band=E5_BAND,
        better=HIGHER,
        scale=E5_SCALE,
    ),
    Metric(
        key="bar",
        name="E5 derived bar",
        explanation=(
            "The agreement a model that knows nothing would reach on these pairs, simulated from the "
            "objective's own claimed effect sizes. It is derived, never chosen, so the threshold "
            "cannot be moved to fit the answer."
        ),
        band="About 0.50-0.51 here. The measured agreement must clear it to serve optimised selection.",
        better=PAIRED,
    ),
    Metric(
        key="expected_if_exactly_right",
        name="E5 score of an exactly-right model",
        explanation=(
            "What a model that is right about every pair would score, given how small the changes "
            "between consecutive elevens actually are. It is the ceiling, and it is low -- which is "
            "why the bar is low too."
        ),
        band="About 0.52 here. A measured agreement near the bar is not a weak model; it is a small effect.",
        better=PAIRED,
    ),
    Metric(
        key="simulated_mean",
        name="E5 null mean",
        explanation="The mean agreement over simulated replicates of a model with no lineup knowledge.",
        band="About 0.50: the null is a coin flip on pairs whose result moved.",
        better=PAIRED,
    ),
    Metric(
        key="simulated_sd",
        name="E5 null spread",
        explanation=(
            "The spread of agreement across those simulated replicates -- how far the null wanders "
            "on this many pairs. The bar is a quantile of it."
        ),
        band="Wider with fewer pairs; it is why a small format's E5 cannot decide anything.",
        better=PAIRED,
    ),
    Metric(
        key="standard_error",
        name="Standard error of the agreement",
        explanation=(
            "The sampling error of the agreement rate, from the number of pairs scored. A difference "
            "smaller than it is not a difference."
        ),
        band="Read the agreement as agreement +/- this. Overlap with the bar means undecided, not passed.",
        better=LOWER,
    ),
    Metric(
        key="ci95",
        name="95 % interval of the agreement",
        explanation=(
            "The range the agreement rate could plausibly be, given how many pairs were scored. "
            "Whether the bar sits inside it is the whole verdict."
        ),
        band="A bar inside the interval means undecided; the pass needs the interval clear of it.",
        better=PAIRED,
    ),
    Metric(
        key="effect_size",
        name="Objective effect size on E5 pairs",
        explanation=(
            "How large a win-probability change the objective itself claims between the two elevens "
            "of a pair -- the median, mean and 90th percentile of the absolute change. The derived "
            "bar is simulated from these, so the model is judged against its own claim."
        ),
        band="A few points of win probability at most; consecutive elevens differ by one to three players.",
        better=PAIRED,
    ),
    # --- The performance model: ranking players, and the intervals around them ---
    Metric(
        key="within_match_spearman",
        name="Spearman (within-match)",
        explanation=SPEARMAN_EXPLANATION,
        band=SPEARMAN_BAND,
        better=HIGHER,
        scale=SPEARMAN_SCALE,
    ),
    Metric(
        key="within_match_spearman_involved",
        name="Spearman among the players involved",
        explanation=(
            "The same rank agreement, restricted to the players who actually did the thing -- who "
            "batted, who bowled. It is the harder and more honest version: ranking a bowler above a "
            "non-bowler is not skill."
        ),
        band=SPEARMAN_BAND,
        better=HIGHER,
        scale=SPEARMAN_SCALE,
    ),
    Metric(
        key="top3_hit_rate",
        name="Top-3 hit rate",
        explanation=(
            "How often a player the model put in a side's top three for this target really finished "
            "in the top three. A blunt, readable companion to Spearman."
        ),
        band="No absolute target: read it against the career-mean baseline on the same rows.",
        better=HIGHER,
    ),
    Metric(
        key="mae",
        name="Median absolute error",
        explanation=(
            "How far the predicted median sat from what happened, on average. It is reported for "
            "orientation only: point error is not what a distributional model is judged on (H-22)."
        ),
        band="Read against the career-mean baseline beside it, never on its own.",
        better=LOWER,
    ),
    Metric(
        key="pinball",
        name="Pinball loss",
        explanation=PINBALL_EXPLANATION,
        band=PINBALL_BAND,
        better=LOWER,
    ),
    Metric(
        key="pinball_by_level",
        name="Pinball loss by quantile level",
        explanation=(
            "The same proper score split by the quantile it scores (0.1, 0.5, 0.9), so a model that "
            "is good in the middle and wrong in a tail can be seen to be exactly that."
        ),
        band=PINBALL_BAND,
        better=LOWER,
    ),
    Metric(
        key="coverage_80",
        name="10-90 coverage",
        explanation=(
            "How often reality landed inside the stated 80 % range. It is the first question to ask "
            "of any interval: a range nobody's outcomes fall inside is not a forecast."
        ),
        band=(
            "Nominal 0.80; within +/- 0.03 is calibrated (H-5). Far below means overconfident, far above means vague."
        ),
        better=NOMINAL,
        scale=COVERAGE_SCALE,
    ),
    Metric(
        key="coverage_80_strict",
        name="10-90 coverage, strict",
        explanation=(
            "The same coverage counting an outcome exactly on the boundary as outside. On a discrete "
            "target -- wickets, catches -- the two versions bracket the truth, and quoting only one "
            "of them would flatter or damn the model by a tie."
        ),
        band="Read as a bracket with the inclusive coverage: calibration means 0.80 lies between them.",
        better=NOMINAL,
        scale=COVERAGE_SCALE,
    ),
    Metric(
        key="width_80",
        name="10-90 interval width",
        explanation=(
            "How narrow the stated range is, in the target's own units. Narrowing is the only way to "
            "become more useful without becoming wrong."
        ),
        band=(
            "No good absolute value: narrower while coverage holds is progress, narrower while "
            "coverage falls is a regression (H-22). Read it only beside coverage."
        ),
        better=PAIRED,
    ),
    Metric(
        key="q10",
        name="Exceedance at the 10th percentile",
        explanation=(
            "The share of outcomes at or below the stated 10th percentile, counted strictly and "
            "inclusively because ties on a discrete target are real."
        ),
        band="0.10 when calibrated; H-5 recalibrates a target whose rate sits outside +/- 0.03 of it.",
        better=NOMINAL,
        scale=PIT_TAIL_SCALE,
    ),
    Metric(
        key="q90",
        name="Exceedance at the 90th percentile",
        explanation=(
            "The share of outcomes at or below the stated 90th percentile, counted strictly and "
            "inclusively because ties on a discrete target are real."
        ),
        band="0.90 when calibrated; H-5 recalibrates a target whose rate sits outside +/- 0.03 of it.",
        better=NOMINAL,
        scale=Scale(bad=0.80, good=0.90),
    ),
    Metric(
        key="probabilities",
        name="Discrete-outcome probabilities",
        explanation=(
            "For a small-count target such as wickets, the model states a probability per outcome "
            "rather than a quantile. This block scores those probabilities and shows their "
            "calibration curve."
        ),
        band="Judged by the Brier score and the reliability curve inside it.",
        better=PAIRED,
    ),
    Metric(
        key="reliability",
        name="Reliability (calibration curve)",
        explanation=(
            "Predicted probability against observed frequency, bucket by bucket. It is where a "
            "well-ranked but badly-stated probability shows itself: good AUC, wrong numbers."
        ),
        band="Predicted about equals observed in every bucket when calibrated.",
        better=PAIRED,
    ),
    Metric(
        key="vs_career_mean",
        name="Against the career-mean baseline",
        explanation=(
            "The model's score minus the unconditional career mean's on the same rows, metric by "
            "metric. The baseline knows only who the player is, so this is what the fixture, the "
            "form and the conditions were worth."
        ),
        band="Above zero on Spearman and pinball, or the model is not earning its complexity.",
        better=HIGHER,
    ),
    Metric(
        key="vs_career_quantiles",
        name="Against the career-quantile baseline",
        explanation=(
            "The same comparison against a baseline that also states intervals -- each player's own "
            "career quantiles. It is the harder baseline, and the one pinball loss must beat."
        ),
        band="Above zero on pinball, or the intervals are not worth more than the player's history.",
        better=HIGHER,
    ),
    # --- The simulator ---
    Metric(
        key="dispersion_ratio",
        name="Dispersion ratio",
        explanation=(
            "The actual spread of totals around the simulated mean, over the spread the simulator "
            "itself drew. It answers whether the simulation is as uncertain as reality is."
        ),
        band="1.0 calibrated; above 1 overconfident (it was 1.42 before the shared match factor); below 1 vague.",
        better=NOMINAL,
        scale=Scale(bad=1.40, good=1.0),
    ),
    Metric(
        key="bias",
        name="Bias of the simulated total",
        explanation=(
            "Actual total minus simulated mean, averaged. It separates a level error -- every total "
            "too high -- from a spread error, which need different fixes."
        ),
        band="Zero when unbiased; the ODI folds run -47 to +35 runs per quarter, which is a level error to chase.",
        better=NOMINAL,
    ),
    Metric(
        key="median_mae",
        name="Median absolute error of the total",
        explanation="How far the simulated median total sat from the innings actually scored, on average.",
        band="Orientation only: a simulator is judged on coverage and dispersion, not point error (H-22).",
        better=LOWER,
    ),
    Metric(
        key="actual_sd_around_simulated_mean",
        name="Actual spread around the simulated mean",
        explanation="How widely real totals scattered around what the simulator expected. The numerator of the dispersion ratio.",
        band="Read only as the top half of the dispersion ratio.",
        better=PAIRED,
    ),
    Metric(
        key="simulated_sd_mean",
        name="Simulated spread",
        explanation="How widely the simulator's own draws scattered. The denominator of the dispersion ratio.",
        band="Read only as the bottom half of the dispersion ratio.",
        better=PAIRED,
    ),
    Metric(
        key="below_q10",
        name="PIT tail below the 10th percentile",
        explanation=PIT_TAIL_EXPLANATION,
        band=PIT_TAIL_BAND,
        better=NOMINAL,
        scale=PIT_TAIL_SCALE,
    ),
    Metric(
        key="above_q90",
        name="PIT tail above the 90th percentile",
        explanation=PIT_TAIL_EXPLANATION,
        band=PIT_TAIL_BAND,
        better=NOMINAL,
        scale=PIT_TAIL_SCALE,
    ),
    Metric(
        key="pit_deciles",
        name="PIT deciles",
        explanation=(
            "Where real totals fell in the simulated distribution, counted per decile. A calibrated "
            "simulator puts a tenth of outcomes in each; a U shape means the draws are too narrow."
        ),
        band="0.10 in every decile when calibrated.",
        better=NOMINAL,
        scale=PIT_TAIL_SCALE,
    ),
    Metric(
        key="shared_factor_folds",
        name="Folds with a shared match factor",
        explanation=(
            "How many walk-forward folds simulated with the shared match factor fitted, which "
            "windows did not, and the same totals over the folds that did. A calibration window "
            "holding too few complete first innings to deconvolve a factor ships the un-widened "
            "simulator, whose intervals are much too narrow, so the two figures say how far a thin "
            "fold pulled the pooled one."
        ),
        band=(
            "Every scored fold, ideally. Where one is thin the pooled first-innings coverage sits "
            "below the calibrated folds' -- 0.750 against 0.770 in ODI, 0.776 against 0.780 in "
            "T20I, on the run that found it (B-12)."
        ),
        better=HIGHER,
    ),
    Metric(
        key="delta_brier_simulated_minus_display",
        name="Simulated minus display Brier",
        explanation=(
            "How much worse (positive) or better (negative) the simulator's win probability scores "
            "than the display model's on the same matches. E2 asks exactly this."
        ),
        band="Within 0.01 of the display model to be served as a probability; beyond it the simulation is a description only.",
        better=LOWER,
        scale=E2_TOLERANCE_SCALE,
    ),
    Metric(
        key="delta_brier_mean",
        name="Simulated minus display Brier, mean over folds",
        explanation="The E2 difference averaged over the walk-forward folds -- the number the decision is actually made on.",
        band="Within the 0.01 tolerance to serve the simulated probability (E2).",
        better=LOWER,
        scale=E2_TOLERANCE_SCALE,
    ),
    Metric(
        key="delta_brier_sd",
        name="Simulated minus display Brier, spread over folds",
        explanation="How much the E2 difference moves between folds. A mean inside tolerance with a large spread has not settled.",
        band="Read beside the mean: a spread larger than the tolerance means the folds disagree.",
        better=LOWER,
    ),
    Metric(
        key="p_bat_first_wins",
        name="Share won batting first",
        explanation=(
            "How often the side batting first won -- as the simulator drew it, and as it really "
            "happened. Two numbers far apart mean the simulated match, not the model's ranking, is wrong."
        ),
        band="The simulated share should sit within a point or two of the actual share.",
        better=NOMINAL,
    ),
    # --- Parity and leakage: the checks that make everything above trustworthy ---
    Metric(
        key="max_abs_difference",
        name="Train / serve parity (H-8)",
        explanation=(
            "The largest difference between a feature built by the training path and the same feature "
            "built by the as-of serving path, for the same match. It is what makes a measured number "
            "a statement about what users are served."
        ),
        band=PARITY_BAND,
        better=EXACT,
        scale=PARITY_SCALE,
    ),
    Metric(
        key="fielded_eleven_max_abs_difference",
        name="Previous-eleven parity",
        explanation=(
            "The same parity check for the elevens E5 reads from the serving path: the aggregates of "
            "a side's previous eleven must match what training saw, or the experiment is scoring a "
            "code path difference."
        ),
        band=PARITY_BAND,
        better=EXACT,
        scale=PARITY_SCALE,
    ),
    Metric(
        key="auc",
        name="Leak canary: a single column's AUC",
        explanation=(
            "How well one column alone predicts the winner over the development window. A column that "
            "should know nothing and predicts well is a leak until proven otherwise."
        ),
        band="0.65 or above in a limited-overs format makes the column a suspect to review (H-2).",
        better=PAIRED,
    ),
    Metric(
        key="test_auc",
        name="Leak canary: the same column in TEST",
        explanation=(
            "The control. A genuine signal survives in TEST; a leak that comes from the way a "
            "limited-overs row is built usually does not."
        ),
        band="A suspect is a column at 0.65+ in a limited-overs format and 0.55 or below here (H-2).",
        better=PAIRED,
    ),
    # --- Prediction surfaces: what a user is shown for one upcoming match ---
    Metric(
        key="win_probability",
        name="Win probability",
        explanation=(
            "The stated chance that this side wins, from the model named beside it. It is a "
            "probability, not a prediction: over many matches at 70 %, about seven in ten should be won."
        ),
        band="Judged by Brier and reliability, not by whether the favourite won; see the evaluation report.",
        better=PAIRED,
    ),
    Metric(
        key="range_10_90",
        name="10-90 range",
        explanation=(
            "The band the model expects this number to land in eight times out of ten. The point "
            "beside it is the median: a median with no range reads as a promise the model never made."
        ),
        band=(
            "Calibrated when reality lands inside it about 80 % of the time; measured 0.786 (T20) and "
            "0.790 (ODI) on the locked window."
        ),
        better=PAIRED,
    ),
    Metric(
        key="marginal_value",
        name="Marginal value",
        explanation=(
            "The win probability the side loses if this player were replaced by an average one. It is "
            "the reason he is in the eleven, stated as a number."
        ),
        band="A few points of win probability across a typical XI. It is a ranking aid, not a promise.",
        better=HIGHER,
    ),
    # --- P1-3: the "why this player" card. Only what the selection consumed ---
    Metric(
        key="xi_role",
        name="Role in the eleven",
        explanation=(
            "Which requirement of the selection this player answers, read off the same as-of vectors "
            "the objective reads. 'Keeper' means the state has credited him with a stumping, which is "
            "the flag the keeper constraint counts and the objective's has_keeper feature reads; "
            "'bowling option' means his expected balls bowled clear the format's threshold, which is "
            "what the minimum-bowlers constraint counts and the n_bowlers feature reads. A player can "
            "answer both, or neither and be picked on his rating alone."
        ),
        band="A label, not a measurement: it says what the selection counted him as, not how good he is.",
        better=PAIRED,
    ),
    Metric(
        key="selection_rating",
        name="Selection rating",
        explanation=(
            "The one composite the selection orders a pool by: decayed batting impact per innings, plus "
            "decayed bowling impact per innings, plus Elo above 1500 in units of 400 points. It fills the "
            "constraints and then the rest of the eleven, so it is the seed every search starts from and "
            "the whole answer where a format is not searched at all."
        ),
        band=(
            "Unitless and pool-relative -- read the percentile beside it, not this number. It is not the "
            "win model's opinion: the two disagree about who is better (docs/BUG_BACKLOG.md B-8)."
        ),
        better=HIGHER,
    ),
    Metric(
        key="rating_percentile",
        name="Rating percentile in the pool",
        explanation=(
            "Where the selection rating stands among the candidates this eleven was actually chosen out "
            "of: the share of that pool the player outranks, so the pool's best reads 100 and its worst 0. "
            "It says nothing about cricketers outside the pool."
        ),
        band="0-100 within one pool. A small pool makes every percentile coarse; the pool size is shown beside it.",
        better=HIGHER,
    ),
    Metric(
        key="next_best_gap",
        name="Gap to the next best",
        explanation=(
            "How much win probability the eleven would lose if this player were swapped for the best "
            "available replacement left in the pool -- the same one-for-one swaps the search itself scores, "
            "under the same constraints and against the same opposing eleven."
        ),
        band=(
            "A point estimate: this computation yields no interval, so none is shown. At or below zero it "
            "means the search stopped on its evaluation budget rather than at a local optimum."
        ),
        better=HIGHER,
    ),
    # --- P1-4: the Lab's honesty surfaces. The sources by name, and the total itself ---
    # Each source of a served number is a label the Lab shows and opens an explainer from,
    # so a reader learns which model made the number without leaving the number. The
    # values are go-app's wire vocabulary (contracts/ops-console.contract.json,
    # `win_probability_sources` and `forecast_sources`); the key is `<field>_<value>`.
    Metric(
        key="innings_total",
        name="Simulated innings total",
        explanation=(
            "The total the simulator expects this side to make: the median-band total the per-player "
            "lines and the extras sum to, all from one set of drawn whole matches, with the 10-90 range "
            "those draws produced beside it. Nothing is rescaled toward the win probability, and the "
            "range is shown exactly as drawn."
        ),
        band=(
            "Calibrated when reality lands inside the 10-90 range about 80 % of the time: measured 0.786 "
            "(T20) and 0.790 (ODI) on the locked window. Split by day and night the same ranges cover "
            "0.734 by day against 0.841 at night (T20) and 0.715 against 0.932 (ODI) -- too narrow by "
            "day, too wide at night (X-2, H-22). That gap is neither adjusted for nor hidden here."
        ),
        better=PAIRED,
    ),
    Metric(
        key="win_probability_source_display",
        name="Display model",
        explanation=(
            "The headline probability came from the display model: a gradient-boosted model over both "
            "elevens' as-of rating vectors and the fixture's context, built so that a one-player upgrade "
            "never lowers the number it shows. It reads an eleven; it does not search for one."
        ),
        band=(
            "Walk-forward AUC 0.70-0.75, this system's measured range; swap-violation share 0.0000 in "
            "every format since B-7. Judged by Brier and reliability in the evaluation report."
        ),
        better=PAIRED,
    ),
    Metric(
        key="win_probability_source_simulator",
        name="Simulator win share",
        explanation=(
            "The share of simulated matches this side won, over the same draws the totals and the "
            "scorecard come from. Where it is not the headline it is shown beside the display model's "
            "number, never blended with it; which model is the headline was chosen per format on the folds (E2)."
        ),
        band=(
            "Read against the display model's Brier beside it (delta_brier_simulated_minus_display in the "
            "report); the two should agree in direction and usually within a few points."
        ),
        better=PAIRED,
    ),
    Metric(
        key="forecast_source_simulator",
        name="Simulated match",
        explanation=(
            "The per-player runs, balls, wickets and conceded, their 10-90 ranges, the innings totals and "
            "the scorecard are one set of drawn whole matches, so the lines and the extras sum to the total. "
            "Nothing is rescaled toward the win probability."
        ),
        band=(
            "2,000 draws per fixture as served. Coverage of the 10-90 ranges is measured at 0.786 (T20) and "
            "0.790 (ODI) on the locked window, with the day/night split recorded under 'Simulated innings total'."
        ),
        better=PAIRED,
    ),
    Metric(
        key="forecast_source_performance_quantiles",
        name="Performance quantiles",
        explanation=(
            "The per-player numbers are the performance model's own median and 10-90 quantiles, reported "
            "directly, because this format has no innings length for the simulator to draw. There is no "
            "simulated match, so no innings total, no scorecard and no economy rate: eleven medians do not "
            "sum to an innings, and none is invented."
        ),
        band=(
            "Judged by the per-player coverage and pinball loss in the evaluation report; nothing here is "
            "summed into a total."
        ),
        better=PAIRED,
    ),
    Metric(
        key="spread_share",
        name="Spread share",
        explanation=(
            "How much of the innings total's uncertainty this player contributes -- his covariance "
            "with the total, over the total's variance. The shares across the eleven sum to one."
        ),
        band="Relative: read it across the eleven, not against a threshold.",
        better=PAIRED,
    ),
    Metric(
        key="spread_runs",
        name="Spread, in runs",
        explanation="The same share expressed in runs: the player's share of the total's standard deviation.",
        band="Relative: read it across the eleven, not against a threshold.",
        better=PAIRED,
    ),
    Metric(
        key="economy",
        name="Economy rate",
        explanation="Predicted runs conceded per over: the conceded median divided by the overs the model expects him to bowl.",
        band="Format-dependent; read it against the other bowlers in the same eleven.",
        better=LOWER,
    ),
    # --- X-4: the market benchmark. A yardstick, never an input ---
    Metric(
        key="market_auc",
        name="Market AUC",
        explanation=(
            "The same measure applied to the betting market's closing price, de-vigged into a "
            "probability. It is the best publicly observable forecast of the same match, so it is the "
            "yardstick the display model is held against. " + AUC_EXPLANATION
        ),
        band=MARKET_BAND,
        better=HIGHER,
        scale=AUC_SCALE,
    ),
    Metric(
        key="market_brier",
        name="Market Brier",
        explanation="The same squared-error score for the market's de-vigged closing probability. " + BRIER_EXPLANATION,
        band=MARKET_BAND,
        better=LOWER,
        scale=BRIER_SCALE,
    ),
    Metric(
        key="display_toss_aware_auc",
        name="Display AUC, toss-aware",
        explanation=(
            "The displayed model read at the batting order that actually happened, rather than averaged "
            "over both. The served probability marginalises over the toss because the toss is unknown "
            "when a user asks; the closing market price is struck after it. This arm gives the market's "
            "information set to our model, so the comparison is like for like."
        ),
        band=MARKET_BAND,
        better=HIGHER,
        scale=AUC_SCALE,
    ),
    Metric(
        key="display_toss_aware_brier",
        name="Display Brier, toss-aware",
        explanation="The same squared-error score for the toss-aware reading of the displayed model.",
        band=MARKET_BAND,
        better=LOWER,
        scale=BRIER_SCALE,
    ),
    Metric(
        key="market_minus_display_auc",
        name="Market minus display, AUC",
        explanation=(
            "How much better the market ranks the same matches than the probability a user is shown. "
            "Positive means the market is ahead; it is the distance to the practical ceiling, in AUC."
        ),
        band=MARKET_DELTA_BAND,
        better=LOWER,
        scale=MARKET_DELTA_SCALE,
    ),
    Metric(
        key="market_minus_display_brier",
        name="Market minus display, Brier",
        explanation=(
            "The same gap in Brier score. Negative means the market's probabilities are better stated "
            "than ours; positive means ours are."
        ),
        band=MARKET_DELTA_BAND,
        better=HIGHER,
    ),
    Metric(
        key="market_minus_toss_aware_auc",
        name="Market minus display, AUC, toss-aware",
        explanation=(
            "The same gap against the toss-aware arm -- the honest one, because both sides then know who batted first."
        ),
        band=MARKET_DELTA_BAND,
        better=LOWER,
        scale=MARKET_DELTA_SCALE,
    ),
    Metric(
        key="market_minus_display_auc_ci95",
        name="95 % interval of the market-minus-display AUC gap",
        explanation=(
            "The range the gap could plausibly be, from resampling the scored matches in pairs (both "
            "arms score the same matches, so the resample keeps them together). An interval spanning "
            "zero means the two are indistinguishable on this many matches."
        ),
        band="Read the gap only through this interval; one spanning zero is not a gap.",
        better=PAIRED,
    ),
    Metric(
        key="market_minus_toss_aware_auc_ci95",
        name="95 % interval of the toss-aware AUC gap",
        explanation="The same interval for the like-for-like comparison.",
        band="Read the gap only through this interval; one spanning zero is not a gap.",
        better=PAIRED,
    ),
    Metric(
        key="market_overround",
        name="Market overround",
        explanation=(
            "The two sides' implied probabilities added up before de-vigging. On an exchange the excess "
            "over 1.0 is the back/lay spread rather than a bookmaker's margin, which is why it is small."
        ),
        band="About 1.00 on an exchange; a bookmaker's would be 1.05 or more. Reported so the de-vig is auditable.",
        better=PAIRED,
    ),
    Metric(
        key="joined_share",
        name="Joined coverage",
        explanation=(
            "The share of the format's evaluated matches that a closing price could be joined to. It is "
            "part of the benchmark's answer, not a footnote: a market comparison over a tenth of a "
            "format says what it says about that tenth."
        ),
        band="0.0 means the benchmark says nothing about this format. Read every market number beside it.",
        better=HIGHER,
        scale=Scale(bad=0.0, good=1.0),
    ),
)

REGISTRY: Dict[str, Metric] = {metric.key: metric for metric in METRICS}


#: Keys the completeness gate must not read as metrics, and why. A key here is a count, an
#: identifier, a configured input, or a record of how the model was fitted -- never a
#: measurement of how well it did. Declaring a container skips everything under it.
NON_METRIC_KEYS: Dict[str, str] = {
    # Denominators and counts.
    "n": "a row count",
    "n_train": "a row count",
    "n_eval": "a row count",
    "n_holdout": "a row count",
    "n_folds": "how many folds a summary averaged",
    "n_rows": "a row count",
    "n_player_rows": "a row count",
    "n_matches": "a match count",
    "n_samples": "how many draws the simulator took",
    "n_pairs": "a pair count",
    "n_features": "how many columns the model was fitted on",
    "n_calibration": "how many rows were held back for calibration",
    "pairs": "E5 pair counts, by fold and by window",
    "pairs_scored": "how many pairs the agreement was computed over",
    "agreed": "the numerator of the agreement rate",
    "upgrades": "how many one-player upgrades were tried",
    "violations": "the numerator of the violation share",
    "excluded_result_unchanged": "pairs E5 cannot score because the result did not move",
    "excluded_objective_indifferent": "pairs E5 cannot score because the objective has no preference",
    "unscored_previous_eleven": "pairs whose previous eleven the serving path could not rebuild",
    "matches_compared": "how many matches the parity check rebuilt",
    "player_rows_compared": "how many player rows the parity check rebuilt",
    "win_rows_compared": "how many win rows the parity check rebuilt",
    "performance_predictions_compared": "how many predictions the parity check rebuilt",
    "simulations_compared": "how many simulated innings the parity check rebuilt",
    "by_changes": "pair counts split by how many players changed",
    # X-4's join: how many closing quotes there were and what became of each.
    "rows_read": "how many odds rows the cached files held",
    "quotes": "how many closing quotes were loaded",
    "quotes_loaded": "how many closing quotes were loaded",
    "quotes_unusable": "quotes dropped before any join: not two runners, or an unreadable price",
    "quotes_joined": "how many quotes reached a match",
    "quotes_unknown_team": "quotes whose team the identity layer does not know",
    "quotes_no_match": "quotes whose date and teams match no fixture we hold",
    "quotes_ambiguous": "quotes whose date and teams match more than one fixture",
    "label_disagreements": "joined quotes whose recorded winner is not the one our result says -- the join's integrity check",
    "quotes_joined_outside_scored_windows": "joined quotes whose match falls before the first walk-forward cutoff",
    "matches_in_windows": "how many matches the harness scored in this format's windows",
    "matches_joined": "how many of those a closing price was joined to",
    # Inputs and configuration, not results.
    "seed": "the random seed a simulation was run under",
    "seeds": "the random seeds the display model was fitted under",
    "replicates": "how many null replicates the derived bar was simulated from",
    "bar_quantile": "which quantile of the null the bar is taken at",
    "tolerance": "E2's configured tolerance, not a measurement",
    "changes_range": "how many players may differ between E5's paired elevens -- the experiment's definition",
    "eval_positive_rate": "the label's base rate in the evaluation window -- a property of the data",
    "train_positive_rate": "the label's base rate in the training window -- a property of the data",
    # Records of the run, not of its accuracy.
    "fit": "what the fitted model was: hyperparameters, iterations and timings",
    "calibration": "the constants the simulator was calibrated with: correlations and the shared-factor variance split",
    "latency": "how fast the harness ran, not how well the model did",
    "data_quality": "H-15's counts of what the rating pass read, skipped and could not resolve",
    "hyperparameters": "the values the grid chose",
}

#: The shape ``_stats`` writes when a number is summarised over folds. It terminates a
#: walk: the key above it is the metric, and mean / sd / n_folds are its summary.
_FOLD_STAT_KEYS = frozenset({"mean", "sd", "n_folds"})


def as_dict() -> Dict[str, Dict[str, Any]]:
    """The glossary as the report embeds it and the service serves it."""
    return {metric.key: {**asdict(metric), "direction": metric.direction} for metric in METRICS}


def describe(key: str) -> str:
    """One line for a log or a script: the name, the band and the direction."""
    metric = REGISTRY[key]
    return f"{metric.key} ({metric.name}) - {metric.explanation} Reference: {metric.band} ({metric.better})"


def _is_number(node: Any) -> bool:
    return isinstance(node, (int, float)) and not isinstance(node, bool)


def _is_reported_number(node: Any) -> bool:
    """A number, a fold summary of one, or a list of numbers -- the shapes a metric takes."""
    if _is_number(node):
        return True
    if isinstance(node, dict):
        return bool(node) and set(node) <= _FOLD_STAT_KEYS
    if isinstance(node, list):
        return bool(node) and all(_is_number(item) for item in node)
    return False


def metric_keys(report: Any) -> List[str]:
    """Every metric key the report emits, in the order first met.

    A key is a metric when it carries a number the report is reporting: a bare value, a
    ``{mean, sd, n_folds}`` summary, or a list of them. Keys declared in
    ``NON_METRIC_KEYS`` are skipped with everything under them, and a key with an entry
    ends the walk there -- its entry explains the whole node.
    """
    found: List[str] = []
    seen: set = set()

    def visit(node: Any) -> None:
        if isinstance(node, dict):
            for key, value in node.items():
                if key in NON_METRIC_KEYS:
                    continue
                if key in REGISTRY or _is_reported_number(value):
                    if key not in seen:
                        seen.add(key)
                        found.append(key)
                    continue
                visit(value)
        elif isinstance(node, list):
            for item in node:
                visit(item)

    visit(report)
    return found


def check_report(report: Dict[str, Any]) -> List[str]:
    """The completeness gate: every metric the report emits must be explained.

    Returns the problems; an empty list is a report whose every number a reader can look
    up. A new metric key that is neither glossaried here nor declared a non-metric fails
    it, which is the point -- a metric cannot reach a surface unexplained.
    """
    problems: List[str] = []
    for metric in METRICS:
        for field in ("name", "explanation", "band", "better"):
            if not getattr(metric, field).strip():
                problems.append(f"metric {metric.key} declares no '{field}'")
    for key in metric_keys(report):
        if key not in REGISTRY:
            problems.append(f"metric '{key}' is reported with no glossary entry")
    embedded = report.get("glossary", {}).get("entries")
    if embedded is not None and set(embedded) != set(REGISTRY):
        problems.append("the report's embedded glossary does not match the code's")
    return problems


def check_metric_names(keys: Sequence[str], source: str) -> List[str]:
    """The same gate for keys reported outside the L4 report -- the run manifest's
    headline metrics, which every Workbench surface renders by key."""
    return [
        f"{source} reports '{key}' with no glossary entry"
        for key in keys
        if key not in REGISTRY and key not in NON_METRIC_KEYS
    ]
