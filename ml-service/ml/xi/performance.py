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
average -- unless the caller knows it (the same knob the win model has). Every fit is
repeated over three seeds, which enter through the early-stopping split, and the members'
outputs are averaged.
"""

from __future__ import annotations

import logging
import time
from dataclasses import dataclass, field
from typing import Any, Dict, List, Mapping, Optional, Sequence, Tuple

import numpy as np
import pandas as pd
from scipy.stats import poisson
from sklearn.ensemble import HistGradientBoostingClassifier, HistGradientBoostingRegressor

from ml.xi import contract as C
from ml.xi import simulator
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
MAX_ITER = 300
#: Seeds enter here: which tenth of the training rows early stopping watches.
EARLY_STOPPING: Dict[str, Any] = {"early_stopping": True, "validation_fraction": 0.1, "n_iter_no_change": 20}
DEFAULT_SEEDS: Tuple[int, ...] = (0, 1, 2)
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
    seeds: Tuple[int, ...] = DEFAULT_SEEDS
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

    def as_dict(self) -> Dict:
        return {
            "n_features": len(self.feature_cols),
            "feature_cols": list(self.feature_cols),
            "hyperparameters": self.hyperparameters_name,
            "hyperparameter_values": dict(self.hyperparameters),
            "structure": dict(self.structure),
            "seeds": list(self.seeds),
            "recalibrate": list(self.recalibrate),
            "targets": list(self.targets),
            "shared_factor": self.shared_factor,
            "fixture_context_families": list(self.fixture_context_families),
            "age": self.age,
            "chase_response": self.chase_response,
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
    seeds: Optional[Sequence[int]] = None,
    recalibrate: Optional[Sequence[str]] = None,
    targets: Optional[Sequence[str]] = None,
    shared_factor: Optional[bool] = None,
    fixture_context_families: Tuple[str, ...] = C.FIXTURE_CONTEXT_FAMILIES_KEPT,
    chase_response: Optional[str] = None,
    age: bool = C.AGE_FEATURES_KEPT,
) -> FitSpec:
    """The production spec, with the module's decided defaults read at call time so a
    test can shrink the seeds without rebinding every caller."""
    return FitSpec(
        feature_cols=tuple(C.performance_feature_cols(sequence_families, joint_format, fixture_context_families, age)),
        hyperparameters=HYPERPARAMETER_GRID[hyperparameters],
        hyperparameters_name=hyperparameters,
        structure=dict(structure or DEFAULT_STRUCTURE),
        seeds=tuple(DEFAULT_SEEDS if seeds is None else seeds),
        recalibrate=tuple(RECALIBRATED_TARGETS if recalibrate is None else recalibrate),
        targets=tuple(targets) if targets is not None else tuple(t.name for t in TARGETS),
        shared_factor=simulator.SHARED_FACTOR if shared_factor is None else bool(shared_factor),
        fixture_context_families=tuple(fixture_context_families),
        age=bool(age),
        chase_response=simulator.CHASE_RESPONSE if chase_response is None else chase_response,
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


def count_distribution(zero_inflation: np.ndarray, rate: np.ndarray) -> Dict[str, np.ndarray]:
    """A zero-inflated Poisson: with probability ``zero_inflation`` the count is 0, else
    Poisson(rate). ``zero_inflation`` is 0 for the direct structure."""
    p = 1.0 - zero_inflation
    e = np.exp(-rate)
    p0 = zero_inflation + p * e
    p1 = p * rate * e
    quantiles = np.empty((len(rate), len(QUANTILE_LEVELS)))
    for i, tau in enumerate(QUANTILE_LEVELS):
        conditional_level = np.clip((tau - zero_inflation) / np.maximum(p, 1e-9), 1e-12, 1.0 - 1e-12)
        quantiles[:, i] = np.where(tau <= zero_inflation, 0.0, poisson.ppf(conditional_level, rate))
    return {
        "mean": p * rate,
        "p0": p0,
        "p1": p1,
        "p2plus": np.clip(1.0 - p0 - p1, 0.0, 1.0),
        "quantiles": quantiles,
    }


# --- fitting --------------------------------------------------------------------------


def _regressor(loss: str, seed: int, hyperparameters: Mapping[str, float], quantile: Optional[float] = None):
    return HistGradientBoostingRegressor(
        loss=loss, quantile=quantile, max_iter=MAX_ITER, random_state=seed, **EARLY_STOPPING, **hyperparameters
    )


def _classifier(seed: int, hyperparameters: Mapping[str, float]):
    return HistGradientBoostingClassifier(max_iter=MAX_ITER, random_state=seed, **EARLY_STOPPING, **hyperparameters)


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


def _fit_count(x: np.ndarray, y: np.ndarray, seed: int, hyperparameters: Mapping[str, float]):
    if y.sum() <= 0:
        return ConstantEstimator(0.0)
    return _regressor("poisson", seed, hyperparameters).fit(x, y)


def _fit_involvement(x: np.ndarray, label: np.ndarray, seed: int, hyperparameters: Mapping[str, float]):
    if label.all() or not label.any():
        return ConstantEstimator(float(label.mean()))
    return _classifier(seed, hyperparameters).fit(x, label.astype(int))


@dataclass
class SeedMember:
    """One seed's fitted estimators."""

    seed: int
    involvement: Dict[str, Any]  # "bats" / "bowls" -> classifier on the unconditional rows
    quantile_direct: Dict[str, List[Any]]  # target -> one regressor per QUANTILE_LEVELS
    quantile_conditional: Dict[str, List[Any]]  # target -> one regressor per CONDITIONAL_LEVELS
    count_rate: Dict[str, Any]  # target -> Poisson regressor (all rows, or the involved rows)
    iterations: Dict[str, int]  # estimator name -> boosting iterations used


def _fit_member(x: np.ndarray, rows: pd.DataFrame, spec: FitSpec, seed: int) -> SeedMember:
    hp = spec.hyperparameters
    involved = {part: rows[col].to_numpy() > 0 for part, col in INVOLVEMENT_COLS.items()}
    iterations: Dict[str, int] = {}
    involvement = {}
    for part in spec.involvement_parts:
        clf = _fit_involvement(x, involved[part], seed, hp)
        involvement[part] = clf
        iterations[f"p_{part}"] = int(clf.n_iter_)
    quantile_direct: Dict[str, List[Any]] = {}
    quantile_conditional: Dict[str, List[Any]] = {}
    count_rate: Dict[str, Any] = {}
    for t in spec.target_specs:
        y = rows[t.name].to_numpy(dtype=float)
        structure = spec.structure[t.name]
        if structure == "two_part" and t.involvement is None:
            raise ValueError(f"{t.name} has no involvement part; only 'direct' is possible")
        mask = involved[t.involvement] if structure == "two_part" else np.ones(len(rows), dtype=bool)
        if mask.sum() < MIN_FIT_ROWS:
            raise ValueError(f"{t.name}: {int(mask.sum())} rows to fit on, need {MIN_FIT_ROWS}")
        if t.kind == "quantile":
            levels = CONDITIONAL_LEVELS if structure == "two_part" else QUANTILE_LEVELS
            fitted = [_regressor("quantile", seed, hp, quantile=q).fit(x[mask], y[mask]) for q in levels]
            (quantile_conditional if structure == "two_part" else quantile_direct)[t.name] = fitted
            iterations[t.name] = int(np.mean([m.n_iter_ for m in fitted]))
        else:
            est = _fit_count(x[mask], y[mask], seed, hp)
            count_rate[t.name] = est
            iterations[t.name] = int(est.n_iter_)
    return SeedMember(seed, involvement, quantile_direct, quantile_conditional, count_rate, iterations)


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
            out[t.name] = {"zero_inflation": zero_inflation, "rate": rate}
    return out


def _average(predictions: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Element-wise mean of member (or orientation) predictions; quantiles are averaged
    level by level, count parameters parameter by parameter."""
    first = predictions[0]
    out: Dict[str, Any] = {}
    for key, value in first.items():
        if isinstance(value, dict):
            out[key] = {k: np.mean([p[key][k] for p in predictions], axis=0) for k in value}
        else:
            out[key] = np.mean([p[key] for p in predictions], axis=0)
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

    def _predict_oriented_raw(self, rows: pd.DataFrame, bats_first: Optional[bool]) -> Dict[str, Any]:
        x = design_matrix(rows, self.spec.feature_cols, bats_first)
        return _average([_predict_member(m, x, self.spec) for m in self.members])

    def _finalize(self, raw: Dict[str, Any]) -> Dict[str, Any]:
        out: Dict[str, Any] = {key: raw[key] for key in ("p_bats", "p_bowls") if key in raw}
        for t in self.spec.target_specs:
            if t.kind == "quantile":
                quantiles = raw[t.name]["quantiles"]
                recalibration = self.calibration.get(t.name)
                out[t.name] = {"quantiles": recalibration.apply(quantiles) if recalibration else quantiles}
            else:
                out[t.name] = count_distribution(raw[t.name]["zero_inflation"], raw[t.name]["rate"])
        return out

    def predict_oriented(self, rows: pd.DataFrame, bats_first: Optional[bool]) -> Dict[str, Any]:
        """Predictions under a known innings: ``True`` / ``False`` for every row, or
        ``None`` for each row's own ``side`` (scoring played matches with the toss known)."""
        return self._finalize(self._predict_oriented_raw(rows, bats_first))

    def predict_marginalised(self, rows: pd.DataFrame) -> Dict[str, Any]:
        """Predictions before the toss: the average of batting first and chasing."""
        both = [self._predict_oriented_raw(rows, True), self._predict_oriented_raw(rows, False)]
        return self._finalize(_average(both))

    @property
    def iterations(self) -> Dict[str, float]:
        names = self.members[0].iterations
        return {name: float(np.mean([m.iterations[name] for m in self.members])) for name in names}


def _temporal_calibration_split(rows: pd.DataFrame) -> Tuple[pd.DataFrame, pd.DataFrame]:
    """The training rows split at ``CALIBRATION_DAYS`` before their last date: the earlier
    part fits, the later part calibrates (a temporal fold, H-21)."""
    boundary = rows.match_date.max() - pd.Timedelta(days=CALIBRATION_DAYS)
    return rows[rows.match_date < boundary], rows[rows.match_date >= boundary]


def _fit_simulator_calibration(
    model: PerformanceModels, calibration_rows: pd.DataFrame, match_frame: pd.DataFrame, chase_response: str
) -> Tuple[Optional[simulator.SharedFactor], Optional[simulator.ChaseResponse]]:
    """The simulator's fold-fitted parts from the calibration fold's complete first innings
    (plan P-4 and §8.10): fixtures the members did not train on, simulated toss-known
    without either part, so each learns the residual after the members. The shared factor
    deconvolves the first-innings residuals; the chase response reads the same matches'
    chases against the chasing side's expected total on the factor's own pitch. A format
    without an innings length has no simulator and so neither; a fold too thin to hold a
    residual distribution ships neither, and says so."""
    if model.format_code not in simulator.SIMULATED_FORMATS:
        return None, None
    matches = match_frame[match_frame.match_id.isin(set(calibration_rows.match_id))]
    matches = matches[simulator.complete_first_innings(matches)]
    if len(matches) < simulator.MIN_SHARED_FACTOR_MATCHES:
        logger.warning(
            "%s: %d complete first innings in the calibration fold, need %d; the simulator ships without a shared "
            "factor or a chase response",
            model.format_code,
            len(matches),
            simulator.MIN_SHARED_FACTOR_MATCHES,
        )
        return None, None
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
    if chase_response == "none":
        return shared_factor, None
    sample = simulator.chase_calibration_sample(
        actual_first,
        outcomes.innings2_runs.to_numpy(dtype=float),
        outcomes[C.TARGET_COL].to_numpy(dtype=float) == 0.0,
        draws.chase_expected,
        shared_factor.factors,
    )
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
    return shared_factor, response


def fit_performance(
    rows: pd.DataFrame, format_code: str, spec: FitSpec, match_frame: Optional[pd.DataFrame] = None
) -> PerformanceModels:
    """Fit the format's model on ``rows`` (the training population, every XI player) under
    ``spec``: one member per seed, and -- for the targets ``spec.recalibrate`` names -- a
    quantile recalibration fitted on the last ``CALIBRATION_DAYS`` of the rows, which the
    members then do not train on. The simulator's calibration comes from the same rows:
    the runs-balls copula from the fit rows and, under ``spec.shared_factor``, the shared
    match factor from the calibration fold, which needs the win rows (``match_frame``)."""
    if len(rows) < MIN_FIT_ROWS:
        raise ValueError(f"{format_code}: {len(rows)} training rows, need {MIN_FIT_ROWS}")
    if spec.shared_factor and match_frame is None:
        raise ValueError(f"{format_code}: the shared match factor needs the match frame")
    if spec.chase_response != "none" and not spec.shared_factor:
        raise ValueError(f"{format_code}: the chase response needs the shared match factor")
    started = time.perf_counter()
    hold_out = bool(spec.recalibrate) or spec.shared_factor
    fit_rows, calibration_rows = _temporal_calibration_split(rows) if hold_out else (rows, rows.iloc[0:0])
    if hold_out and len(fit_rows) < MIN_FIT_ROWS:
        # A history shorter than the calibration fold cannot hold one out; the members take
        # every row and the fold-fitted parts (recalibration, shared factor) are not fitted.
        logger.warning(
            "%s: %d rows before the calibration fold, need %d; fitting on every row without recalibration or a shared factor",
            format_code,
            len(fit_rows),
            MIN_FIT_ROWS,
        )
        hold_out = False
        fit_rows, calibration_rows = rows, rows.iloc[0:0]
    x = design_matrix(fit_rows, spec.feature_cols)
    members = [_fit_member(x, fit_rows, spec, seed) for seed in spec.seeds]
    model = PerformanceModels(format_code, spec, members, {}, {})
    for target in spec.recalibrate if hold_out else ():
        raw = model.predict_marginalised(calibration_rows)[target]["quantiles"]
        model.calibration[target] = QuantileRecalibration.fit(raw, calibration_rows[target].to_numpy(dtype=float))
    shared_factor, chase_response = (
        _fit_simulator_calibration(model, calibration_rows, match_frame, spec.chase_response)
        if spec.shared_factor and hold_out
        else (None, None)
    )
    model.simulation = simulator.calibrate(fit_rows, shared_factor, chase_response)
    model.metadata = {
        "format_code": format_code,
        "n_train": int(len(fit_rows)),
        "n_calibration": int(len(calibration_rows)),
        "train_from": fit_rows.match_date.min().date().isoformat(),
        "train_to": fit_rows.match_date.max().date().isoformat(),
        "spec": spec.as_dict(),
        "iterations": model.iterations,
        "simulation": model.simulation.as_dict(),
        "fit_seconds": round(time.perf_counter() - started, 1),
    }
    return model
