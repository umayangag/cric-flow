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
SPECIFIC_XI_SCALE = Scale(bad=0.0, good=0.02)
COVERAGE_SCALE = Scale(bad=0.65, good=0.80)
E2_TOLERANCE_SCALE = Scale(bad=0.01, good=0.0)


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
