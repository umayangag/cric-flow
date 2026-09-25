"""L2-B: the distributional player-performance model.

Per format, one artifact answers "what will each of these twenty-two players do" as
distributions, never as points: quantiles (0.1 / 0.5 / 0.9) of runs, balls faced and runs
conceded; a count distribution of wickets and catches (Poisson rate -> P(0), P(1), P(2+));
and P(bats) / P(bowls). The point shown anywhere is the median of the distribution.

Population (H-20): every XI player of every decided match, with "did not bat" as 0 runs
from 0 balls and "did not bowl" as 0 wickets -- never rows selected by what happened. Two
structures are available per target and chosen on the walk-forward folds by pinball loss:

* ``direct``   -- one model per quantile (or one Poisson model) on the unconditional rows;
* ``two_part`` -- P(involved) from a classifier on the unconditional rows, times the
                  distribution given involvement fitted on the rows where it happened, with
                  the involvement *predicted*, never read from the outcome. The served
                  quantiles are those of the mixture, so the two structures are scored on
                  the same population with the same loss.

Inputs are the row's as-of vectors and role, the kept sequence families (E1), both sides'
aggregates, venue context, the Elo edge, the kept fixture-context families (A-1: the
ground's and the competition's as-of scoring level), the player's age at the match date if
gate X-1b kept it, and the innings (bat first / chase). The innings
is the toss, not the result; it is marginalised at prediction -- predict under both and
serve the mixture of the two (``_marginalise``: the involvement probabilities averaged, a
quantile target's distribution mixed through the reconstruction the simulator draws from
and inverted at the served levels, a count target's two zero-inflated Poissons mixed
exactly) -- unless the caller knows it (the same knob the win model has). Averaging the
quantiles level by level, or the Poisson parameters one by one, was EVAL-14: the former is
the quantile of no distribution and narrows the 10-90 interval whenever the two innings
differ, the latter's mean was ``avg(p) * avg(rate)`` rather than ``avg(p * rate)``.

Each booster's iteration count is chosen on the most recent tenth of the training rows
by date, cut at a match boundary, by the booster's own loss there (EVAL-09) -- then it is
refitted on every row for exactly that count with sklearn's early stopping off. The
eleven rows of one side share 44 of the 62 inputs, so sklearn's shuffled validation
tenth was a near-copy of the training rows and the point it stopped at was optimistic;
the choice fold's rows come after every row the choice fit saw, as the served rows will.
"""

from __future__ import annotations

import logging
import time
from dataclasses import dataclass, field, replace
from typing import Any, Callable, Dict, List, Mapping, Optional, Sequence, Tuple

import numpy as np
import pandas as pd
from scipy.stats import poisson
from sklearn.ensemble import HistGradientBoostingClassifier, HistGradientBoostingRegressor
from sklearn.metrics import log_loss, mean_pinball_loss, mean_poisson_deviance

from ml.xi import contract as C
from ml.xi import simulator
from ml.xi.perf_calibration import MIN_ROWS as MIN_RECALIBRATION_ROWS
from ml.xi.perf_calibration import QuantileRecalibration
from ml.xi.perf_metrics import QUANTILE_LEVELS

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class TargetSpec:
    name: str
    kind: str  # "quantile" | "count"
    involvement: Optional[str]  # "bats" | "bowls" | None: the two-part model's first part
    headline: bool = True  # catches are reported, never a headline (plan §3)


TARGETS: Tuple[TargetSpec, ...] = (
    TargetSpec("runs", "quantile", "bats"),
    TargetSpec("balls_faced", "quantile", "bats"),
    TargetSpec("wickets", "count", "bowls"),
    TargetSpec("runs_conceded", "quantile", "bowls"),
    TargetSpec("catches", "count", None, headline=False),
)
TARGET_BY_NAME: Dict[str, TargetSpec] = {t.name: t for t in TARGETS}
#: What "involved" means, per part: the outcome column whose positivity defines it. Used as
#: a *label* for the classifier and to select the conditional rows -- never as a feature.
INVOLVEMENT_COLS: Dict[str, str] = {"bats": "balls_faced", "bowls": "balls_bowled"}
#: Levels the conditional part of a two-part quantile model is fitted at; the mixture
#: quantile at a served level is interpolated between them.
CONDITIONAL_LEVELS: Tuple[float, ...] = tuple(float(x) for x in np.round(np.arange(0.1, 0.91, 0.1), 2))
STRUCTURES: Tuple[str, ...] = ("direct", "two_part")

#: The small fixed grid (plan §4: a 10-point grid per model is enough). Tuned inside the
#: walk-forward folds only; the choice and its table are in the plan (P-3).
HYPERPARAMETER_GRID: Dict[str, Dict[str, float]] = {
    "small": {"learning_rate": 0.05, "max_leaf_nodes": 15, "min_samples_leaf": 200, "l2_regularization": 1.0},
    "medium": {"learning_rate": 0.05, "max_leaf_nodes": 31, "min_samples_leaf": 100, "l2_regularization": 1.0},
    "large": {"learning_rate": 0.1, "max_leaf_nodes": 63, "min_samples_leaf": 50, "l2_regularization": 0.0},
}
DEFAULT_HYPERPARAMETERS = "medium"
#: The ceiling on any booster's iterations; the count each one runs is chosen below it.
MAX_ITER = 300
#: EVAL-09: the most recent share of the training rows, by date and cut at a match
#: boundary, that each booster's iteration count is chosen on -- the same tenth sklearn's
#: early stopping used to draw at random from among rows whose match-mates it trained on.
#: The choice fit sees only the rows before it; the served fit is then a fresh fit on every
#: row for the chosen count, with early stopping off (``choose_iterations``).
ITERATION_CHOICE_FRACTION = 0.1
#: The one random state the boosters run under. ``HistGradientBoosting``'s random state
#: reaches only the early-stopping split -- off here -- and the binning subsample above
#: 200,000 rows (EVAL-02), so repeated fits under other seeds are the same model below that
#: line and differ by the binning draw alone above it. There is nothing to average over.
RANDOM_STATE = 0
#: Structure per target, decided on the walk-forward folds by pinball loss (plan §5.3):
#: two-part only where it beat direct by more than 0.5 % in every format -- wickets (1.0 %
#: T20, 0.9 % ODI); for runs it was a tie (0.2 % / -0.05 %) at three times the fits.
DEFAULT_STRUCTURE: Dict[str, str] = {t.name: ("two_part" if t.name == "wickets" else "direct") for t in TARGETS}
#: Targets whose quantiles are recalibrated on a temporal fold (H-5), decided by the
#: harness's coverage on the walk-forward folds; empty while coverage is nominal.
RECALIBRATED_TARGETS: Tuple[str, ...] = ()
#: The temporal calibration fold: the last quarter of the training rows by date. Shared by
#: the quantile recalibration (H-5) and the simulator's shared match factor (plan P-4).
CALIBRATION_DAYS = 92
#: Draws per calibration fixture when fitting the shared factor: enough for a mean and sd.
SHARED_FACTOR_SAMPLES = 400
MIN_FIT_ROWS = 200


@dataclass(frozen=True)
class FitSpec:
    """Everything a fit is a function of, besides the rows."""

    feature_cols: Tuple[str, ...]
    hyperparameters: Mapping[str, float]
    structure: Mapping[str, str] = field(default_factory=lambda: dict(DEFAULT_STRUCTURE))
    recalibrate: Tuple[str, ...] = RECALIBRATED_TARGETS
    hyperparameters_name: str = DEFAULT_HYPERPARAMETERS
    # Experiments fit a subset of targets; production fits them all.
    targets: Tuple[str, ...] = tuple(t.name for t in TARGETS)
    # Whether the simulator's shared match factor is fitted on the temporal calibration fold
    # (``simulator.SHARED_FACTOR`` decides the default; the fit then needs the match frame).
    shared_factor: bool = False
    # Which fixture-context families the feature columns include (A-1), recorded so a run's
    # manifest says what its performance model read.
    fixture_context_families: Tuple[str, ...] = C.FIXTURE_CONTEXT_FAMILIES_KEPT
    # Whether the feature columns include the player's age at the match date (X-1b family 1).
    age: bool = C.AGE_FEATURES_KEPT
    # Which chase response the simulator's calibration fits on the calibration fold (A-2,
    # ``simulator.CHASE_RESPONSE_ARMS``); needs the shared factor, whose per-match factors
    # take the pitch out of the difficulty.
    chase_response: str = "none"
    # Which dispersion term the chasing innings carries beside the shared factor (B-11, plan
    # §8.14 and §8.15, ``simulator.CHASE_DISPERSION_ARMS``); every arm reads the same
    # per-match factors, so it needs the shared factor for the same reason the response does.
    chase_dispersion: str = "none"

    def as_dict(self) -> Dict:
        return {
            "n_features": len(self.feature_cols),
            "feature_cols": list(self.feature_cols),
            "hyperparameters": self.hyperparameters_name,
            "hyperparameter_values": dict(self.hyperparameters),
            "structure": dict(self.structure),
            "recalibrate": list(self.recalibrate),
            "targets": list(self.targets),
            "shared_factor": self.shared_factor,
            "fixture_context_families": list(self.fixture_context_families),
            "age": self.age,
            "chase_response": self.chase_response,
            "chase_dispersion": self.chase_dispersion,
        }

    @property
    def target_specs(self) -> Tuple[TargetSpec, ...]:
        return tuple(TARGET_BY_NAME[name] for name in self.targets)

    @property
    def involvement_parts(self) -> Tuple[str, ...]:
        """Which P(involved) heads to fit: all of them for a full model (they are served),
        only the ones a two-part target needs for an experiment's subset."""
        if set(self.targets) == {t.name for t in TARGETS}:
            return tuple(INVOLVEMENT_COLS)
        return tuple(
            part
            for part in INVOLVEMENT_COLS
            if any(self.structure[t.name] == "two_part" and t.involvement == part for t in self.target_specs)
        )


def default_spec(
    sequence_families: Tuple[str, ...] = C.SEQUENCE_FAMILIES_KEPT,
    joint_format: bool = False,
    hyperparameters: str = DEFAULT_HYPERPARAMETERS,
    structure: Optional[Mapping[str, str]] = None,
    recalibrate: Optional[Sequence[str]] = None,
    targets: Optional[Sequence[str]] = None,
    shared_factor: Optional[bool] = None,
    fixture_context_families: Tuple[str, ...] = C.FIXTURE_CONTEXT_FAMILIES_KEPT,
    chase_response: Optional[str] = None,
    chase_dispersion: Optional[str] = None,
    age: bool = C.AGE_FEATURES_KEPT,
) -> FitSpec:
    """The production spec, with the module's decided defaults read at call time so a
    test can change one without rebinding every caller."""
    return FitSpec(
        feature_cols=tuple(C.performance_feature_cols(sequence_families, joint_format, fixture_context_families, age)),
        hyperparameters=HYPERPARAMETER_GRID[hyperparameters],
        hyperparameters_name=hyperparameters,
        structure=dict(structure or DEFAULT_STRUCTURE),
        recalibrate=tuple(RECALIBRATED_TARGETS if recalibrate is None else recalibrate),
        targets=tuple(targets) if targets is not None else tuple(t.name for t in TARGETS),
        shared_factor=simulator.SHARED_FACTOR if shared_factor is None else bool(shared_factor),
        fixture_context_families=tuple(fixture_context_families),
        age=bool(age),
        chase_response=simulator.CHASE_RESPONSE if chase_response is None else chase_response,
        chase_dispersion=simulator.CHASE_DISPERSION if chase_dispersion is None else chase_dispersion,
    )


def design_matrix(rows: pd.DataFrame, feature_cols: Sequence[str], bats_first: Optional[bool] = None) -> np.ndarray:
    """The model's input matrix. ``bats_first`` overrides the innings column for every row
    (a counterfactual orientation); ``None`` reads each row's own side."""
    x = np.empty((len(rows), len(feature_cols)), dtype=float)
    for j, col in enumerate(feature_cols):
        if col == C.BATS_FIRST_COL:
            x[:, j] = float(bats_first) if bats_first is not None else (rows.side.to_numpy() == 1).astype(float)
        elif col == C.FORMAT_INDICATOR_COL:
            x[:, j] = (rows.format_code.to_numpy() == C.E6_JOINT_FORMATS[1]).astype(float)
        else:
            x[:, j] = rows[col].to_numpy(dtype=float)
    return np.nan_to_num(x)


# --- distributions --------------------------------------------------------------------


def _interp_rows(levels: np.ndarray, values: np.ndarray, at: np.ndarray) -> np.ndarray:
    """Row-wise linear interpolation of ``values`` (n, k) defined at ``levels`` (k,) at the
    per-row abscissae ``at`` (n,), which are clipped to the level range."""
    at = np.clip(at, levels[0], levels[-1])
    upper = np.clip(np.searchsorted(levels, at, side="right"), 1, len(levels) - 1)
    lower = upper - 1
    span = levels[upper] - levels[lower]
    weight = (at - levels[lower]) / span
    rows = np.arange(len(at))
    return values[rows, lower] * (1.0 - weight) + values[rows, upper] * weight


def mixture_quantiles(
    p_involved: np.ndarray, conditional: np.ndarray, levels: Sequence[float] = QUANTILE_LEVELS
) -> np.ndarray:
    """Quantiles of the mixture (1 - p) * delta_0 + p * F_c, where F_c is known through its
    quantiles at ``CONDITIONAL_LEVELS``. Below the zero mass the quantile is 0; above it
    the conditional level is (tau - (1 - p)) / p, interpolated between the fitted levels
    with 0 at level 0 and the top fitted quantile beyond the last."""
    grid = np.concatenate([[0.0], CONDITIONAL_LEVELS, [1.0]])
    extended = np.column_stack([np.zeros(len(conditional)), conditional, conditional[:, -1]])
    out = np.empty((len(conditional), len(levels)))
    zero_mass = 1.0 - p_involved
    for i, tau in enumerate(levels):
        conditional_level = (tau - zero_mass) / np.maximum(p_involved, 1e-9)
        out[:, i] = np.where(tau <= zero_mass, 0.0, _interp_rows(grid, extended, conditional_level))
    return out


#: A quantile set is one (zero_inflation, rate) pair per row: with probability
#: ``zero_inflation`` the count is 0, else Poisson(rate); ``zero_inflation`` is 0 for the
#: direct structure.
CountComponent = Tuple[np.ndarray, np.ndarray]


def count_distribution(components: Sequence[CountComponent]) -> Dict[str, np.ndarray]:
    """The equal-weight mixture of the zero-inflated Poissons in ``components`` -- one for
    a known innings, two for the toss marginalised. Mean, P(0), P(1) and P(2+) are the
    means of the components'; the quantiles invert the mean of the components' CDFs on
    the integers (the smallest count at which it reaches the level)."""
    zero_inflation = np.stack([z for z, _ in components])  # (m, n)
    rate = np.stack([r for _, r in components])
    p = 1.0 - zero_inflation
    e = np.exp(-rate)
    p0 = np.mean(zero_inflation + p * e, axis=0)
    p1 = np.mean(p * rate * e, axis=0)
    support = np.arange(int(poisson.ppf(1.0 - 1e-9, rate.max())) + 2)
    cdf = np.mean(
        [z[:, np.newaxis] + (1.0 - z)[:, np.newaxis] * poisson.cdf(support, r[:, np.newaxis]) for z, r in components],
        axis=0,
    )  # (n, len(support))
    quantiles = np.column_stack([np.argmax(cdf >= tau, axis=1) for tau in QUANTILE_LEVELS]).astype(float)
    return {
        "mean": np.mean(p * rate, axis=0),
        "p0": p0,
        "p1": p1,
        "p2plus": np.clip(1.0 - p0 - p1, 0.0, 1.0),
        "quantiles": quantiles,
    }


#: Bisection steps when inverting a mixture CDF: the bracket is the components' own
#: quantiles at the level, so 60 halvings resolve any served scale to well under 1e-12.
MIXTURE_BISECTION_STEPS = 60


def mixture_of_quantile_sets(components: Sequence[np.ndarray], levels: Sequence[float] = QUANTILE_LEVELS) -> np.ndarray:
    """Quantiles at ``levels`` of the equal-weight mixture of the distributions each (n, k)
    quantile set describes, under the one reconstruction this system gives three quantiles
    (``simulator.quantile_function``, whose inverse ``simulator.cumulative_probability``
    is): the mixture's CDF is the mean of the components' and is inverted by bisection
    between the components' own quantiles at each level, which bracket the mixture's. With
    one component it returns that component exactly."""
    stacked = np.stack(components)  # (m, n, k)
    low, high = stacked.min(axis=0), stacked.max(axis=0)
    wanted = np.asarray(levels)[np.newaxis, :]
    for _ in range(MIXTURE_BISECTION_STEPS):
        mid = 0.5 * (low + high)
        # ``cumulative_probability`` reads values as (levels, rows) against (rows, 3).
        cdf = np.mean([simulator.cumulative_probability(q, mid.T).T for q in components], axis=0)
        below = cdf < wanted
        low, high = np.where(below, mid, low), np.where(below, high, mid)
    return high


# --- fitting --------------------------------------------------------------------------


def _regressor(loss: str, hyperparameters: Mapping[str, float], max_iter: int, quantile: Optional[float] = None):
    return HistGradientBoostingRegressor(
        loss=loss,
        quantile=quantile,
        max_iter=max_iter,
        random_state=RANDOM_STATE,
        early_stopping=False,
        **hyperparameters,
    )


def _classifier(hyperparameters: Mapping[str, float], max_iter: int):
    return HistGradientBoostingClassifier(
        max_iter=max_iter, random_state=RANDOM_STATE, early_stopping=False, **hyperparameters
    )


class ConstantEstimator:
    """Stands in for a fit the data cannot support: a count target that is zero on every
    training row (no catches recorded) or an involvement that is the same for everyone.
    Predicts that constant; recorded with zero iterations so the report shows it."""

    n_iter_ = 0

    def __init__(self, value: float):
        self.value = float(value)

    def predict(self, x: np.ndarray) -> np.ndarray:
        return np.full(len(x), self.value)

    def predict_proba(self, x: np.ndarray) -> np.ndarray:
        p = np.full(len(x), self.value)
        return np.column_stack([1.0 - p, p])


@dataclass(frozen=True)
class Booster:
    """One of a member's gradient-boosted estimators: what it predicts, the rows it is
    fitted on (its population, H-20) and the outcome it is fitted to. The one enumeration
    (``boosters``) feeds both the iteration choice and the fit, so the population a count
    is chosen on is the population it is then fitted on."""

    key: str  # "p_bats", "runs_q0.5", "wickets": the name its iteration count is recorded under
    kind: str  # "involvement" (a classifier) | "quantile" | "count" (Poisson)
    name: str  # the involvement part, or the target
    mask: np.ndarray  # the rows of its population
    y: np.ndarray  # its label (0 / 1) or outcome on every row; read through ``mask``
    quantile: Optional[float] = None


def boosters(rows: pd.DataFrame, spec: FitSpec) -> List[Booster]:
    """Every booster a member holds under ``spec``, in the order their fitted estimators are
    kept: the involvement classifiers, then per target one regressor per level (or one
    Poisson regressor), on the unconditional rows or -- two-part -- on the involved ones."""
    involved = {part: rows[col].to_numpy() > 0 for part, col in INVOLVEMENT_COLS.items()}
    everyone = np.ones(len(rows), dtype=bool)
    out = [
        Booster(f"p_{part}", "involvement", part, everyone, involved[part].astype(int))
        for part in spec.involvement_parts
    ]
    for t in spec.target_specs:
        structure = spec.structure[t.name]
        if structure == "two_part" and t.involvement is None:
            raise ValueError(f"{t.name} has no involvement part; only 'direct' is possible")
        mask = involved[t.involvement] if structure == "two_part" else everyone
        if mask.sum() < MIN_FIT_ROWS:
            raise ValueError(f"{t.name}: {int(mask.sum())} rows to fit on, need {MIN_FIT_ROWS}")
        y = rows[t.name].to_numpy(dtype=float)
        if t.kind == "quantile":
            levels = CONDITIONAL_LEVELS if structure == "two_part" else QUANTILE_LEVELS
            out.extend(Booster(f"{t.name}_q{q:g}", "quantile", t.name, mask, y, q) for q in levels)
        else:
            out.append(Booster(t.name, "count", t.name, mask, y))
    return out


def _fit_booster(booster: Booster, x: np.ndarray, hyperparameters: Mapping[str, float], max_iter: int):
    """The booster fitted on its population for exactly ``max_iter`` iterations -- or a
    constant, when the population cannot support a fit (everyone involved, or nobody; a
    count that is zero on every row)."""
    y = booster.y[booster.mask]
    if booster.kind == "involvement":
        if y.all() or not y.any():
            return ConstantEstimator(float(y.mean()))
        return _classifier(hyperparameters, max_iter).fit(x[booster.mask], y)
    if booster.kind == "count":
        if y.sum() <= 0:
            return ConstantEstimator(0.0)
        return _regressor("poisson", hyperparameters, max_iter).fit(x[booster.mask], y)
    return _regressor("quantile", hyperparameters, max_iter, quantile=booster.quantile).fit(x[booster.mask], y)


def _staged_loss(booster: Booster, model: Any, x: np.ndarray, y: np.ndarray) -> np.ndarray:
    """The booster's own training loss on ``(x, y)`` after each iteration it ran: log
    loss for a classifier, the pinball loss at its level for a quantile, the Poisson
    deviance for a count."""
    if booster.kind == "involvement":
        return np.array([log_loss(y, p[:, 1], labels=[0, 1]) for p in model.staged_predict_proba(x)])
    if booster.kind == "count":
        return np.array([mean_poisson_deviance(y, np.maximum(p, 1e-9)) for p in model.staged_predict(x)])
    return np.array([mean_pinball_loss(y, p, alpha=booster.quantile) for p in model.staged_predict(x)])


def _temporal_choice_split(rows: pd.DataFrame) -> Tuple[np.ndarray, np.ndarray]:
    """Masks of the rows the choice fit trains on and the later rows it is scored on: the
    most recent ``ITERATION_CHOICE_FRACTION`` of the rows by date. A match has one date, so
    the cut falls at a match boundary and no match's rows land on both sides."""
    dates = rows.match_date.to_numpy()
    boundary = np.sort(dates)[int(len(dates) * (1.0 - ITERATION_CHOICE_FRACTION))]
    later = dates >= boundary
    return ~later, later


@dataclass(frozen=True)
class IterationChoice:
    """Per booster, the iteration count it is fitted for, and the record of how that was
    chosen -- the fit's metadata carries it, so a run says on what evidence."""

    iterations: Dict[str, int]
    record: Dict[str, Any]


def choose_iterations(rows: pd.DataFrame, format_code: str, spec: FitSpec) -> IterationChoice:
    """EVAL-09: each booster's iteration count, chosen where the served model will be used
    -- on rows after every row it was fitted on. The choice fit runs to ``MAX_ITER`` on the
    rows before the temporal cut and the count is the iteration at which its own loss on
    the rows after the cut is lowest. A booster whose population is too thin on either side
    to read a loss curve off, or that the earlier rows cannot support at all, runs
    ``MAX_ITER``; so does every booster when the history is too short to cut."""
    earlier, later = _temporal_choice_split(rows)
    record: Dict[str, Any] = {
        "max_iter": MAX_ITER,
        "fraction": ITERATION_CHOICE_FRACTION,
        "n_fit": int(earlier.sum()),
        "n_choice": int(later.sum()),
        "choice_from": rows.match_date[later].min().date().isoformat() if later.any() else None,
    }
    if earlier.sum() < MIN_FIT_ROWS or later.sum() < MIN_FIT_ROWS:
        logger.warning(
            "%s: %d rows before the iteration-choice cut and %d after, need %d each; every booster runs %d iterations",
            format_code,
            earlier.sum(),
            later.sum(),
            MIN_FIT_ROWS,
            MAX_ITER,
        )
        return IterationChoice({}, {**record, "reason": "too few rows to choose on", "chosen": {}})
    x = design_matrix(rows, spec.feature_cols)
    chosen: Dict[str, int] = {}
    for booster in boosters(rows, spec):
        fit_mask, choice_mask = booster.mask & earlier, booster.mask & later
        if fit_mask.sum() < MIN_FIT_ROWS or choice_mask.sum() < MIN_FIT_ROWS:
            continue
        model = _fit_booster(replace(booster, mask=fit_mask), x, spec.hyperparameters, MAX_ITER)
        if isinstance(model, ConstantEstimator):
            continue
        curve = _staged_loss(booster, model, x[choice_mask], booster.y[choice_mask])
        chosen[booster.key] = int(np.argmin(curve)) + 1
    logger.info(
        "%s: iterations chosen on %d rows from %s: %s", format_code, record["n_choice"], record["choice_from"], chosen
    )
    return IterationChoice(
        chosen, {**record, "reason": "each booster's own loss on the rows after the cut", "chosen": chosen}
    )


@dataclass
class SeedMember:
    """One fitted set of estimators. There is one per model: ``seed`` is the random state
    it ran under, which reaches only the binning subsample above 200,000 rows (see
    ``RANDOM_STATE``). The name is the artifact's; a run fitted before EVAL-09 still
    loads."""

    seed: int
    involvement: Dict[str, Any]  # "bats" / "bowls" -> classifier on the unconditional rows
    quantile_direct: Dict[str, List[Any]]  # target -> one regressor per QUANTILE_LEVELS
    quantile_conditional: Dict[str, List[Any]]  # target -> one regressor per CONDITIONAL_LEVELS
    count_rate: Dict[str, Any]  # target -> Poisson regressor (all rows, or the involved rows)
    iterations: Dict[str, int]  # booster key -> boosting iterations run


def _fit_member(x: np.ndarray, rows: pd.DataFrame, spec: FitSpec, iterations: Mapping[str, int]) -> SeedMember:
    """Every booster fitted on ``rows`` for the count ``iterations`` names for it --
    ``MAX_ITER`` for one it does not name."""
    member = SeedMember(RANDOM_STATE, {}, {}, {}, {}, {})
    for booster in boosters(rows, spec):
        fitted = _fit_booster(booster, x, spec.hyperparameters, iterations.get(booster.key, MAX_ITER))
        member.iterations[booster.key] = int(fitted.n_iter_)
        if booster.kind == "involvement":
            member.involvement[booster.name] = fitted
        elif booster.kind == "count":
            member.count_rate[booster.name] = fitted
        elif spec.structure[booster.name] == "two_part":
            member.quantile_conditional.setdefault(booster.name, []).append(fitted)
        else:
            member.quantile_direct.setdefault(booster.name, []).append(fitted)
    return member


def _predict_member(member: SeedMember, x: np.ndarray, spec: FitSpec) -> Dict[str, Any]:
    n = len(x)
    structure = spec.structure
    p = {part: clf.predict_proba(x)[:, 1] for part, clf in member.involvement.items()}
    out: Dict[str, Any] = {f"p_{part}": p[part] for part in p}
    for t in spec.target_specs:
        if t.kind == "quantile":
            if structure[t.name] == "direct":
                q = np.column_stack([est.predict(x) for est in member.quantile_direct[t.name]])
            else:
                conditional = np.column_stack([est.predict(x) for est in member.quantile_conditional[t.name]])
                q = mixture_quantiles(p[t.involvement], np.maximum(np.sort(conditional, axis=1), 0.0))
            # Independently fitted quantiles can cross; sorting is the standard repair.
            out[t.name] = {"quantiles": np.maximum(np.sort(q, axis=1), 0.0)}
        else:
            rate = np.maximum(member.count_rate[t.name].predict(x), 1e-9)
            zero_inflation = np.zeros(n) if structure[t.name] == "direct" else 1.0 - p[t.involvement]
            out[t.name] = {"components": [(zero_inflation, rate)]}
    return out


def _marginalise(orientations: Sequence[Dict[str, Any]]) -> Dict[str, Any]:
    """The forecast over the innings each entry of ``orientations`` was predicted under, at
    equal weight: an involvement probability is the mean (a mixture of Bernoullis is a
    Bernoulli at the mean); a quantile target is the mixture of the orientations'
    distributions, inverted at the served levels; a count target keeps every orientation's
    (zero inflation, rate) pair for ``count_distribution`` to mix exactly. One orientation
    passes through unchanged."""
    out: Dict[str, Any] = {}
    for key, value in orientations[0].items():
        if not isinstance(value, dict):
            out[key] = np.mean([o[key] for o in orientations], axis=0)
        elif "quantiles" in value:
            out[key] = {"quantiles": mixture_of_quantile_sets([o[key]["quantiles"] for o in orientations])}
        else:
            out[key] = {"components": [component for o in orientations for component in o[key]["components"]]}
    return out


@dataclass
class PerformanceModels:
    """One format's fitted performance model: the artifact ``xi_perf_<FORMAT>.joblib``."""

    format_code: str
    spec: FitSpec
    members: List[SeedMember]
    calibration: Dict[str, QuantileRecalibration]
    metadata: Dict
    # What the simulator (L2-C) takes from this fit's training rows: the runs-balls copula
    # and, when the spec asks for it, the shared match factor from the calibration fold.
    simulation: Optional[simulator.SimulatorCalibration] = None

    @property
    def member(self) -> SeedMember:
        """The one fit this model serves (EVAL-09). ``members`` keeps the artifact's list
        shape; a list of any other length is an artifact this code cannot serve, and says
        so rather than averaging fits it no longer has a rule for (EVAL-14)."""
        if len(self.members) != 1:
            raise ValueError(f"{self.format_code}: {len(self.members)} performance members in the artifact, expected 1")
        return self.members[0]

    def _predict_oriented_raw(self, rows: pd.DataFrame, bats_first: Optional[bool]) -> Dict[str, Any]:
        return _predict_member(self.member, design_matrix(rows, self.spec.feature_cols, bats_first), self.spec)

    def _finalize(self, raw: Dict[str, Any]) -> Dict[str, Any]:
        out: Dict[str, Any] = {key: raw[key] for key in ("p_bats", "p_bowls") if key in raw}
        for t in self.spec.target_specs:
            if t.kind == "quantile":
                quantiles = raw[t.name]["quantiles"]
                recalibration = self.calibration.get(t.name)
                out[t.name] = {"quantiles": recalibration.apply(quantiles) if recalibration else quantiles}
            else:
                out[t.name] = count_distribution(raw[t.name]["components"])
        return out

    def predict_oriented(self, rows: pd.DataFrame, bats_first: Optional[bool]) -> Dict[str, Any]:
        """Predictions under a known innings: ``True`` / ``False`` for every row, or
        ``None`` for each row's own ``side`` (scoring played matches with the toss known)."""
        return self._finalize(_marginalise([self._predict_oriented_raw(rows, bats_first)]))

    def predict_marginalised(self, rows: pd.DataFrame) -> Dict[str, Any]:
        """Predictions before the toss: the equal-weight mixture of batting first and
        chasing, target by target (``_marginalise``)."""
        both = [self._predict_oriented_raw(rows, True), self._predict_oriented_raw(rows, False)]
        return self._finalize(_marginalise(both))

    @property
    def iterations(self) -> Dict[str, int]:
        """Per booster, the iterations the served model ran."""
        return dict(self.member.iterations)


def _temporal_calibration_split(rows: pd.DataFrame) -> Tuple[pd.DataFrame, pd.DataFrame]:
    """The training rows split at ``CALIBRATION_DAYS`` before their last date: the earlier
    part fits, the later part calibrates (a temporal fold, H-21)."""
    boundary = rows.match_date.max() - pd.Timedelta(days=CALIBRATION_DAYS)
    return rows[rows.match_date < boundary], rows[rows.match_date >= boundary]


@dataclass(frozen=True)
class FoldCalibration:
    """What the calibration fold contributes to the simulator, beside the copula the fit
    rows give. Every part is optional: a format without an innings length has none, and a
    fold too thin to hold a residual distribution has none either.

    ``chase_sample`` is the evidence a chase-side fit reads, kept whether or not one was
    fitted -- as the shared factor keeps its own sample (plan §8.13) -- so an experiment can
    refit from the same calibration draws without simulating the fold again.
    """

    shared_factor: Optional[simulator.SharedFactor] = None
    chase_response: Optional[simulator.ChaseResponse] = None
    chase_dispersion: Optional[simulator.ChaseDispersionTerm] = None
    chase_sample: Optional[simulator.ChaseCalibrationSample] = None


#: What each ``simulator.CHASE_DISPERSION_ARMS`` arm fits from the calibration fold. Both take
#: the fold's shared factor and its chase sample; §8.14's independent term needs only the
#: latter, §8.15's correlated one reads the first innings' residuals off the former.
CHASE_DISPERSION_FITTERS: Mapping[str, Callable[[simulator.SharedFactor, simulator.ChaseCalibrationSample], Any]] = {
    "independent": lambda shared_factor, sample: simulator.fit_chase_dispersion(sample),
    "correlated": simulator.fit_correlated_chase_dispersion,
}


def _fit_simulator_calibration(
    model: PerformanceModels,
    calibration_rows: pd.DataFrame,
    match_frame: pd.DataFrame,
    chase_response: str,
    chase_dispersion: str,
) -> FoldCalibration:
    """The simulator's fold-fitted parts from the calibration fold's complete first innings
    (plan P-4, §8.10 and §8.14): fixtures the members did not train on, simulated toss-known
    without any of them, so each learns the residual after the members. The shared factor
    deconvolves the first-innings residuals; the chase response and the chase dispersion read
    the same matches' chases against the chasing side's expected total on the factor's own
    pitch. A format without an innings length has no simulator and so none of them; a fold
    too thin to hold a residual distribution ships none of them, and says so."""
    if model.format_code not in simulator.SIMULATED_FORMATS:
        return FoldCalibration()
    matches = match_frame[match_frame.match_id.isin(set(calibration_rows.match_id))]
    matches = matches[simulator.complete_first_innings(matches)]
    if len(matches) < simulator.MIN_SHARED_FACTOR_MATCHES:
        logger.warning(
            "%s: %d complete first innings in the calibration fold, need %d; the simulator ships without a shared "
            "factor, a chase response or a chase dispersion",
            model.format_code,
            len(matches),
            simulator.MIN_SHARED_FACTOR_MATCHES,
        )
        return FoldCalibration()
    fixtures = simulator.fixtures_from_rows(calibration_rows, matches, model.predict_oriented)
    outcomes = matches.set_index("match_id").loc[[f.match_id for f in fixtures]]
    actual_first = outcomes.innings1_runs.to_numpy(dtype=float)
    rho = simulator.calibrate(calibration_rows).runs_balls_rho
    draws = simulator.simulate_calibration_fixtures(fixtures, rho, SHARED_FACTOR_SAMPLES, seed=0)
    shared_factor = simulator.fit_shared_factor(
        simulator.SharedFactorCalibrationSample(
            np.asarray([f.match_id for f in fixtures], dtype=object), actual_first, draws.first_mean, draws.first_sd
        )
    )
    sample = simulator.chase_calibration_sample(
        actual_first,
        outcomes.innings2_runs.to_numpy(dtype=float),
        outcomes[C.TARGET_COL].to_numpy(dtype=float) == 0.0,
        draws.chase_expected,
        draws.chase_log_sd,
        shared_factor.factors,
    )
    fold = FoldCalibration(shared_factor=shared_factor, chase_sample=sample)
    if chase_response != "none":
        response = simulator.fit_chase_response(sample, chase_response)
        logger.info(
            "%s: chase response (%s) on %d calibration chases: level %+.3f slope %+.3f sigma %.3f",
            model.format_code,
            chase_response,
            len(sample),
            response.level,
            response.slope,
            response.sigma,
        )
        fold = replace(fold, chase_response=response)
    fitter = CHASE_DISPERSION_FITTERS.get(chase_dispersion)
    if fitter is not None:
        dispersion = fitter(shared_factor, sample)
        logger.info(
            "%s: chase dispersion (%s) on %d calibration chases: %s",
            model.format_code,
            chase_dispersion,
            len(sample),
            dispersion.as_dict(),
        )
        fold = replace(fold, chase_dispersion=dispersion)
    return fold


def _fit_members(rows: pd.DataFrame, spec: FitSpec, iterations: Mapping[str, int]) -> List[SeedMember]:
    return [_fit_member(design_matrix(rows, spec.feature_cols), rows, spec, iterations)]


def _calibration_fold(rows: pd.DataFrame, format_code: str, spec: FitSpec) -> Tuple[pd.DataFrame, pd.DataFrame]:
    """The rows before the temporal calibration fold and the fold itself -- empty when the
    spec fits nothing on a fold, or when the history before it is too short to fit the
    members that predict it."""
    if not (spec.recalibrate or spec.shared_factor):
        return rows, rows.iloc[0:0]
    fold_rows, calibration_rows = _temporal_calibration_split(rows)
    if len(fold_rows) < MIN_FIT_ROWS:
        logger.warning(
            "%s: %d rows before the calibration fold, need %d; no recalibration or shared factor is fitted",
            format_code,
            len(fold_rows),
            MIN_FIT_ROWS,
        )
        return rows, rows.iloc[0:0]
    return fold_rows, calibration_rows


def _fit_recalibration(
    fold_model: "PerformanceModels", calibration_rows: pd.DataFrame, format_code: str, spec: FitSpec
) -> Dict[str, QuantileRecalibration]:
    """The quantile corrections ``spec.recalibrate`` asks for, fitted on the fold the
    model did not train on -- or nothing at all when that fold is thinner than a binned
    empirical quantile can be read off (``perf_calibration.MIN_ROWS``).

    Refusing the correction is the right call on a fold that thin: ten bins of a hundred
    rows estimate a 0.1 quantile from ten outcomes each, and the map would be noise
    applied to every served forecast. But shipping the uncorrected quantiles is a
    substitution, so the caller records which targets went uncorrected and every surface
    that reports the model names them (plan §8.7)."""
    if not spec.recalibrate:
        return {}
    if len(calibration_rows) < MIN_RECALIBRATION_ROWS:
        logger.warning(
            "%s: the calibration fold holds %d rows, fewer than the %d a recalibration needs; "
            "%s keep their uncorrected quantiles",
            format_code,
            len(calibration_rows),
            MIN_RECALIBRATION_ROWS,
            ", ".join(spec.recalibrate),
        )
        return {}
    fitted = {}
    for target in spec.recalibrate:
        raw = fold_model.predict_marginalised(calibration_rows)[target]["quantiles"]
        fitted[target] = QuantileRecalibration.fit(raw, calibration_rows[target].to_numpy(dtype=float))
    return fitted


def _fit_fold_parts(
    fold_rows: pd.DataFrame,
    calibration_rows: pd.DataFrame,
    format_code: str,
    spec: FitSpec,
    match_frame: Optional[pd.DataFrame],
    iterations: Mapping[str, int],
) -> Tuple[Dict[str, QuantileRecalibration], FoldCalibration]:
    """The parts fitted on the calibration fold's residuals: members fitted on the rows
    before the fold predict the fold, which they did not train on (H-21), and the quantile
    recalibration (H-5) and the simulator's calibration (P-4) read the residuals. These
    members are discarded afterwards; the served ones are refitted on every row."""
    fold_model = PerformanceModels(format_code, spec, _fit_members(fold_rows, spec, iterations), {}, {})
    fold_model.calibration.update(_fit_recalibration(fold_model, calibration_rows, format_code, spec))
    simulation = (
        _fit_simulator_calibration(
            fold_model, calibration_rows, match_frame, spec.chase_response, spec.chase_dispersion
        )
        if spec.shared_factor
        else FoldCalibration()
    )
    return fold_model.calibration, simulation


def fit_performance(
    rows: pd.DataFrame, format_code: str, spec: FitSpec, match_frame: Optional[pd.DataFrame] = None
) -> PerformanceModels:
    """Fit the format's model on ``rows`` (the training population, every XI player) under
    ``spec``: each booster's iteration count chosen on the most recent tenth of the rows
    (``choose_iterations``), one member fitted on every row for those counts, and -- for
    the targets ``spec.recalibrate`` names -- a quantile recalibration fitted on the last
    ``CALIBRATION_DAYS`` of the rows against members that did not train on them. The
    simulator's calibration comes from the same rows: the runs-balls copula from every row
    and, under ``spec.shared_factor``, the shared match factor from the calibration fold,
    which needs the win rows (``match_frame``).

    Choose and calibrate on rows after the ones fitted, refit on the full history: the
    iteration choice and the fold-fitted parts must read rows the fit could not have
    learned, and the served members must read the most recent rows -- the ratings they
    serve beside already do. The fold members differ from the served ones only by the
    fold's rows (2-5 % of a format's history)."""
    if len(rows) < MIN_FIT_ROWS:
        raise ValueError(f"{format_code}: {len(rows)} training rows, need {MIN_FIT_ROWS}")
    if spec.shared_factor and match_frame is None:
        raise ValueError(f"{format_code}: the shared match factor needs the match frame")
    if spec.chase_response != "none" and not spec.shared_factor:
        raise ValueError(f"{format_code}: the chase response needs the shared match factor")
    if spec.chase_dispersion not in simulator.CHASE_DISPERSION_ARMS:
        raise ValueError(
            f"{format_code}: unknown chase dispersion arm {spec.chase_dispersion!r}; "
            f"one of {simulator.CHASE_DISPERSION_ARMS}"
        )
    if spec.chase_dispersion != "none" and not spec.shared_factor:
        raise ValueError(f"{format_code}: the chase dispersion needs the shared match factor")
    started = time.perf_counter()
    choice = choose_iterations(rows, format_code, spec)
    fold_rows, calibration_rows = _calibration_fold(rows, format_code, spec)
    calibration, fold = (
        _fit_fold_parts(fold_rows, calibration_rows, format_code, spec, match_frame, choice.iterations)
        if len(calibration_rows)
        else ({}, FoldCalibration())
    )
    model = PerformanceModels(format_code, spec, _fit_members(rows, spec, choice.iterations), calibration, {})
    model.simulation = simulator.calibrate(
        rows, fold.shared_factor, fold.chase_response, fold.chase_dispersion, fold.chase_sample
    )
    model.metadata = {
        "format_code": format_code,
        "n_train": int(len(rows)),
        "n_calibration": int(len(calibration_rows)),
        # H-5, and plan §8.7: which targets this model's quantiles are corrected for, and
        # which the spec asked for and did not get because the fold could not carry one.
        # A quantile that was never recalibrated must not read like one that was.
        "recalibrated": sorted(calibration),
        "recalibration_skipped": sorted(set(spec.recalibrate) - set(calibration)),
        "train_from": rows.match_date.min().date().isoformat(),
        "train_to": rows.match_date.max().date().isoformat(),
        # The fold the recalibration and the shared factor were fitted on; its members saw
        # only the rows before it.
        "calibration_from": calibration_rows.match_date.min().date().isoformat() if len(calibration_rows) else None,
        "spec": spec.as_dict(),
        # What the served boosters ran, and the choice that fixed it (EVAL-09): the cut, the
        # rows on each side of it and the count each booster's own loss picked there.
        "iterations": model.iterations,
        "iteration_choice": choice.record,
        "simulation": model.simulation.as_dict(),
        "fit_seconds": round(time.perf_counter() - started, 1),
    }
    return model
