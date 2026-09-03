"""L2-C: the match simulator, derived from L2-B and trained on nothing.

For two elevens at an ``as_of`` it draws whole matches from the performance model's
per-player distributions and returns everything from the same draws: each side's total
(median, 10-90), each player's median and range, the median-band scorecard that sums to
the innings total by construction, the margin as cricket states it, P(win) by simulation,
and each player's contribution to the total's spread. The design is written down in
docs/ML_PIPELINE_REARCHITECTURE_PLAN.md (§3, "The innings sample") and is followed here
step for step; in short:

* the **batting side is authoritative** -- an innings is sampled sequentially by expected
  slot until the balls or the wickets run out, and the bowling side's figures are
  attributions of that innings, so the total is produced once and never rescaled;
* a player's three quantiles become a quantile function (piecewise linear, exponential
  tail above 0.9) that reproduces the fitted quantiles exactly and invents nothing else;
* the chase ends at the target; the toss is marginalised unless known (H-3);
* the chasing side's runs may respond to the target's difficulty -- the target over the
  side's expected total on the day's pitch -- through a log-linear response whose two
  coefficients are fitted by censored maximum likelihood on the calibration fold (plan
  §8.10, gate A-2), never set by hand; ``CHASE_RESPONSE`` records which arm ships;
* every input is an L2-B output for the fixture or an as-of rate from the rating pass
  (``contract.SIMULATION_CONTEXT_COLS``) -- the only constants are the laws of the game
  (H-21: nothing the simulator consumes is in-sample for the fixture);
* the optional shared match factor (a pitch or a day is common to both innings) is sampled
  from the as-of residual distribution, never a hand-set CV, and is on only where E2 found
  totals under-dispersed without it.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Sequence

import numpy as np
import pandas as pd
from scipy.optimize import minimize
from scipy.special import log_ndtr, ndtr

from ml.xi import contract as C

#: Levels the reconstruction interpolates between: the zero point and the fitted quantiles.
QUANTILE_KNOTS = np.array([0.0, 0.1, 0.5, 0.9])
#: ln(0.5 / 0.1): an exponential upper half has q90 - q50 equal to its scale times this.
TAIL_LOG = float(np.log(5.0))
#: The scorecard is the mean over the draws whose total lies in the central tenth of the
#: total's distribution; it sums to that band's mean total by construction.
MEDIAN_BAND = 0.05
DEFAULT_SAMPLES = 2000
#: Formats the simulator runs for: those with an innings length (H-17: TEST stays greedy).
SIMULATED_FORMATS = tuple(f for f in C.FORMAT_CODES if C.INNINGS_LEGAL_BALLS[f])
#: Which L2-B orientation feeds the chasing side. "chasing" is the innings as a known feature
#: (the model's chasing rates and the target truncation both act); "bat_first" lets the
#: truncation alone carry the chase. Measured on the walk-forward folds (plan P-4,
#: ``scripts/experiments/xi/sim_choices.py``): no effect -- Brier within 0.0003, P(bat-first
#: wins) within 0.01 -- so the design's default stays.
CHASE_ORIENTATION = "chasing"
#: Whether one shared match factor multiplies every batter's runs draw. Decided on the folds
#: from the dispersion of simulated totals against actual (plan P-4): without it the PIT of
#: first-innings totals is U-shaped (0.18 / 0.19 in the end deciles) and the dispersion ratio
#: 1.42 T20 / 1.36 ODI; with the deconvolved as-of factor the ratio is 1.02 / 1.02 and 10-90
#: coverage moves 0.64 -> 0.76 (T20) and 0.58 -> 0.74 (ODI). On.
SHARED_FACTOR = True
#: The arms of gate A-2 (plan §8.10): what the chase response fits. ``none`` is today's
#: simulator (the target enters only through the truncation); ``level`` is the control
#: (the slope held at zero); ``slope`` and ``both`` are the candidates.
CHASE_RESPONSE_ARMS = ("none", "level", "slope", "both")
#: Which chase response the simulator applies to the chasing side's runs draws:
#: exp(level + slope * ln(target / expected)), with the coefficients fitted on the shared
#: factor's calibration fold. Decided on the walk-forward folds by gate A-2
#: (``scripts/experiments/xi/a2_chase_tails.py``); the before/after is plan §8.10.
CHASE_RESPONSE = "none"
#: E2's rule (plan §5): the simulator's P(win) may be displayed only if it is within 0.01
#: Brier of the display model on the walk-forward folds; otherwise it is a description of
#: the draws. Measured (P-4, walk-forward folds): within tolerance in T20 (+0.0028 ± 0.0057)
#: and ODI (+0.0034 ± 0.0131) -- a probability, but not a better one -- so the display model
#: stays the headline
#: (plan §3) and the simulated P(win) is served beside it. Per format.
SIMULATED_WIN_PROBABILITY_DISPLAYED: Dict[str, bool] = {f: False for f in SIMULATED_FORMATS}
#: Deconvolution guard: a residual sample smaller than this is not a distribution.
MIN_SHARED_FACTOR_MATCHES = 30


class SimulationUnavailable(ValueError):
    """The fixture cannot be simulated: a format without an innings length."""


# --- inputs ---------------------------------------------------------------------------


@dataclass(frozen=True)
class MatchContext:
    """The as-of rates and the laws the sample needs (``SIMULATION_CONTEXT_COLS``)."""

    format_code: str
    extras_per_ball: float
    innings_deliveries: float
    bowler_wicket_share: float

    @classmethod
    def from_row(cls, format_code: str, row: Any) -> "MatchContext":
        """From anything with the context columns as attributes or keys (a frame row, a
        ``simulation_context`` dict)."""
        get = row.get if isinstance(row, dict) else lambda key: getattr(row, key)
        return cls(
            format_code,
            float(get("ctx_extras_per_ball")),
            float(get("ctx_innings_deliveries")),
            float(get("ctx_bowler_wicket_share")),
        )

    @property
    def deliveries(self) -> int:
        return int(round(self.innings_deliveries))

    @property
    def bowler_cap(self) -> int:
        """The most one bowler may deliver: a fifth of the innings, in whole deliveries."""
        return max(int(np.floor(C.BOWLER_MAX_SHARE * self.innings_deliveries)), 1)


@dataclass(frozen=True)
class SideForecast:
    """L2-B's forecasts for one eleven under one orientation, in the eleven's own order."""

    player_keys: np.ndarray  # (k,) object
    exp_bat_position: np.ndarray  # (k,)
    exp_balls_bowled: np.ndarray  # (k,)
    p_bats: np.ndarray  # (k,)
    p_bowls: np.ndarray  # (k,)
    runs: np.ndarray  # (k, 3) quantiles at QUANTILE_LEVELS
    balls: np.ndarray  # (k, 3)
    conceded: np.ndarray  # (k, 3)
    wickets_mean: np.ndarray  # (k,)

    def __len__(self) -> int:
        return int(len(self.player_keys))

    @property
    def batting_order(self) -> np.ndarray:
        """Slot order: by expected batting position, ties by the eleven's order."""
        return np.argsort(self.exp_bat_position, kind="stable")


def side_forecast(rows: pd.DataFrame, prediction: Dict[str, Any], indices: np.ndarray) -> SideForecast:
    """One side's forecast from the rows and an L2-B prediction dict for them (as
    ``PerformanceModels.predict_oriented`` returns), taking the rows at ``indices``."""
    return SideForecast(
        player_keys=rows.player_key.to_numpy()[indices],
        exp_bat_position=rows.exp_bat_position.to_numpy(dtype=float)[indices],
        exp_balls_bowled=rows.exp_balls_bowled.to_numpy(dtype=float)[indices],
        p_bats=np.clip(prediction["p_bats"][indices], 0.0, 1.0),
        p_bowls=np.clip(prediction["p_bowls"][indices], 0.0, 1.0),
        runs=prediction["runs"]["quantiles"][indices],
        balls=prediction["balls_faced"]["quantiles"][indices],
        conceded=prediction["runs_conceded"]["quantiles"][indices],
        wickets_mean=prediction["wickets"]["mean"][indices],
    )


@dataclass(frozen=True)
class SideForecasts:
    """A side's forecasts under both orientations, so the toss can be marginalised."""

    bat_first: SideForecast
    chasing: SideForecast

    @property
    def when_chasing(self) -> SideForecast:
        return self.chasing if CHASE_ORIENTATION == "chasing" else self.bat_first


# --- calibration: what the simulator learns from the training rows (as-of) --------------


@dataclass(frozen=True)
class SimulatorCalibration:
    """The two numbers the simulator takes from the fit's training rows, both as-of for the
    fixture because the rows all precede the cutoff (H-21).

    ``runs_balls_rho`` is the Gaussian-copula correlation between a batter's runs and balls
    given that he batted, from the rank correlation on the training rows: fully
    comonotonic draws would fix every batter's strike rate and, under the balls budget,
    collapse the total's spread (measured: sd 4.6 on a synthetic side); independent draws
    would let a batter score 60 off 8. ``shared_factor`` is the match-level factor of the
    design, present only where E2 found it needed.
    """

    runs_balls_rho: float
    shared_factor: Optional["SharedFactor"] = None
    #: The chase response (plan §8.10), present only under a ``CHASE_RESPONSE`` arm that
    #: fits one; it needs the shared factor's per-match factors, so never without one.
    chase_response: Optional["ChaseResponse"] = None

    def as_dict(self) -> Dict[str, Any]:
        return {
            "runs_balls_rho": self.runs_balls_rho,
            "shared_factor": None if self.shared_factor is None else self.shared_factor.as_dict(),
            "chase_response": None if self.chase_response is None else self.chase_response.as_dict(),
        }


def runs_balls_copula_rho(runs: np.ndarray, balls: np.ndarray) -> float:
    """Gaussian-copula correlation from the Spearman correlation of (runs, balls) among the
    rows with a ball faced: rho = 2 sin(pi * rho_s / 6)."""
    batted = balls > 0
    if batted.sum() < 3 or np.unique(runs[batted]).size < 2 or np.unique(balls[batted]).size < 2:
        return 0.0  # no rank correlation is defined on a constant column
    spearman = pd.Series(runs[batted]).corr(pd.Series(balls[batted]), method="spearman")
    if not np.isfinite(spearman):
        return 0.0
    return float(np.clip(2.0 * np.sin(np.pi * spearman / 6.0), 0.0, 0.999))


def calibrate(
    rows: pd.DataFrame,
    shared_factor: Optional["SharedFactor"] = None,
    chase_response: Optional["ChaseResponse"] = None,
) -> SimulatorCalibration:
    """The calibration from the fit's training rows and the calibration fold's fits."""
    return SimulatorCalibration(
        runs_balls_copula_rho(rows.runs.to_numpy(dtype=float), rows.balls_faced.to_numpy(dtype=float)),
        shared_factor,
        chase_response,
    )


# --- the shared match factor ----------------------------------------------------------


@dataclass(frozen=True)
class SharedFactor:
    """One multiplicative factor per draw, shared by both innings, sampled from the as-of
    residual distribution: actual / simulated-mean first-innings totals on matches the
    members did not train on, deconvolved of the simulator's own dispersion (shrunk toward
    one by sqrt(excess variance / residual variance)). Never a hand-set CV."""

    factors: np.ndarray
    n_matches: int
    residual_variance: float
    within_variance: float
    shrink: float

    def sample(self, rng: np.random.Generator, n: int) -> np.ndarray:
        return rng.choice(self.factors, size=n, replace=True)

    def as_dict(self) -> Dict[str, float]:
        return {
            "n_matches": self.n_matches,
            "residual_variance": self.residual_variance,
            "within_variance": self.within_variance,
            "shrink": self.shrink,
            "factor_sd": float(np.std(self.factors)),
        }


def fit_shared_factor(actual: np.ndarray, simulated_mean: np.ndarray, simulated_sd: np.ndarray) -> SharedFactor:
    """Deconvolve the match-level residuals. ``actual`` are first-innings totals; the other
    two describe the simulator's own distribution for each (without a factor)."""
    if len(actual) < MIN_SHARED_FACTOR_MATCHES:
        raise ValueError(f"{len(actual)} calibration matches; need {MIN_SHARED_FACTOR_MATCHES} for a shared factor")
    mean = np.maximum(simulated_mean, 1.0)
    ratio = actual / mean
    residual_variance = float(np.var(ratio))
    within_variance = float(np.mean((simulated_sd / mean) ** 2))
    excess = max(residual_variance - within_variance, 0.0)
    shrink = float(np.sqrt(excess / residual_variance)) if residual_variance > 0 else 0.0
    factors = 1.0 + (ratio - 1.0) * shrink
    return SharedFactor(np.maximum(factors, 0.0), int(len(actual)), residual_variance, within_variance, shrink)


# --- the chase response ---------------------------------------------------------------


@dataclass(frozen=True)
class ChaseCalibrationSample:
    """The calibration fold's chases as the response's fit reads them (plan §8.10): per
    match the log difficulty x = ln(target / (factor * expected)), the log response
    y = ln(actual chase / (factor * expected)) and whether the chaser won -- in which case
    the untruncated innings would have reached the target and y is censored at x. The
    factor is the shared factor's own value for the match, so the pitch is taken out of
    the difficulty at fit time as the sampled factor takes it out at draw time."""

    difficulty: np.ndarray
    response: np.ndarray
    censored: np.ndarray

    def __len__(self) -> int:
        return int(len(self.difficulty))


@dataclass(frozen=True)
class ChaseResponse:
    """How the chasing side's runs respond to the target's difficulty: every runs draw is
    multiplied by exp(level + slope * ln(difficulty)), the coefficients fitted by censored
    (Tobit) maximum likelihood on the calibration fold -- never set by hand. ``sigma`` is
    the fit's residual scale on the log scale, a nuisance parameter reported beside the
    simulated chase's own spread. The sample the fit read is kept, as the shared factor
    keeps its factors, so the other arms can be refitted from the same evidence."""

    arm: str
    level: float
    slope: float
    sigma: float
    sample: ChaseCalibrationSample

    def factor(self, difficulty: np.ndarray) -> np.ndarray:
        return np.exp(self.level + self.slope * np.log(np.maximum(difficulty, 1e-6)))

    def as_dict(self) -> Dict[str, Any]:
        return {
            "arm": self.arm,
            "level": self.level,
            "slope": self.slope,
            "sigma": self.sigma,
            "n_matches": int(len(self.sample)),
            "n_won": int(self.sample.censored.sum()),
            "difficulty_sd": float(np.std(self.sample.difficulty)),
        }


def chase_calibration_sample(
    actual_first: np.ndarray,
    actual_chase: np.ndarray,
    chaser_won: np.ndarray,
    chase_expected: np.ndarray,
    factors: np.ndarray,
) -> ChaseCalibrationSample:
    """The fit's inputs for the calibration matches: actual first-innings and chase totals,
    the result, the chasing side's expected untruncated total from the no-factor
    simulation, and the shared factor's per-match factors in the same order."""
    expected = np.maximum(np.asarray(factors, dtype=float) * np.asarray(chase_expected, dtype=float), 1.0)
    target = np.asarray(actual_first, dtype=float) + 1.0
    chase = np.maximum(np.asarray(actual_chase, dtype=float), 1.0)
    return ChaseCalibrationSample(
        np.log(target / expected), np.log(chase / expected), np.asarray(chaser_won, dtype=bool)
    )


def fit_chase_response(sample: ChaseCalibrationSample, arm: str) -> Optional[ChaseResponse]:
    """Censored maximum likelihood of y = level + slope * x + N(0, sigma^2), with y >= x
    where the chaser won: a lost chase contributes its density, a won chase the probability
    the untruncated innings reached the target. ``arm`` says which coefficients are free;
    ``none`` fits nothing. Deterministic."""
    if arm not in CHASE_RESPONSE_ARMS:
        raise ValueError(f"unknown chase response arm {arm!r}; one of {CHASE_RESPONSE_ARMS}")
    if arm == "none":
        return None
    if len(sample) < MIN_SHARED_FACTOR_MATCHES:
        raise ValueError(f"{len(sample)} calibration chases; need {MIN_SHARED_FACTOR_MATCHES} for a chase response")
    lost = ~sample.censored
    if lost.sum() < 2:
        raise ValueError(f"{int(lost.sum())} lost chases in the calibration fold; the residual scale is undefined")
    fit_level, fit_slope = arm in ("level", "both"), arm in ("slope", "both")
    x, y = sample.difficulty, sample.response

    def unpack(theta: np.ndarray) -> tuple[float, float, float]:
        level = float(theta[0]) if fit_level else 0.0
        slope = float(theta[1]) if fit_slope else 0.0
        return level, slope, float(np.exp(theta[2]))

    def negative_log_likelihood(theta: np.ndarray) -> float:
        level, slope, sigma = unpack(theta)
        mean = level + slope * x
        z_lost = (y[lost] - mean[lost]) / sigma
        density = 0.5 * z_lost**2 + np.log(sigma)
        # P(y >= x) = Phi((mean - x) / sigma), through log_ndtr for the far tail.
        survival = log_ndtr((mean[~lost] - x[~lost]) / sigma)
        return float(density.sum() - survival.sum())

    start = np.array([0.0, 0.0, np.log(max(float(np.std(y[lost])), 1e-3))])
    result = minimize(negative_log_likelihood, start, method="Nelder-Mead", options={"xatol": 1e-6, "fatol": 1e-8})
    level, slope, sigma = unpack(result.x)
    return ChaseResponse(arm, level, slope, sigma, sample)


# --- distributions --------------------------------------------------------------------


def quantile_function(quantiles: np.ndarray, level: np.ndarray) -> np.ndarray:
    """Values of the reconstructed distributions at ``level`` (n, k): piecewise linear through
    (0, 0) and the three fitted quantiles ``quantiles`` (k, 3), exponential above 0.9 with
    scale (q90 - q50) / ln 5. Exact at the fitted levels."""
    knots = np.concatenate([np.zeros((len(quantiles), 1)), quantiles], axis=1)  # (k, 4)
    level = np.clip(level, 0.0, 1.0 - 1e-12)
    upper = np.clip(np.searchsorted(QUANTILE_KNOTS, level, side="right"), 1, len(QUANTILE_KNOTS) - 1)
    lower = upper - 1
    weight = (level - QUANTILE_KNOTS[lower]) / (QUANTILE_KNOTS[upper] - QUANTILE_KNOTS[lower])
    k_index = np.broadcast_to(np.arange(knots.shape[0]), level.shape)
    body = knots[k_index, lower] * (1.0 - weight) + knots[k_index, upper] * weight
    scale = (quantiles[:, 2] - quantiles[:, 1]) / TAIL_LOG
    tail = quantiles[:, 2] + scale * np.log(0.1 / (1.0 - level))
    return np.where(level > QUANTILE_KNOTS[-1], tail, body)


def _conditional_level(p_involved: np.ndarray, u: np.ndarray) -> np.ndarray:
    """The unconditional level of a draw made *given involvement*: the upper ``p`` part."""
    return 1.0 - p_involved + p_involved * u


def _expected_strike_rate(side: SideForecast, order: np.ndarray) -> np.ndarray:
    """Runs per ball a batter is expected to score given that he bats: the conditional
    medians' ratio, falling back to the side's mean where a forecast has no runs."""
    level = _conditional_level(side.p_bats[order], np.full((1, len(order)), 0.5))
    runs = quantile_function(side.runs[order], level)[0]
    balls = np.maximum(quantile_function(side.balls[order], level)[0], 1.0)
    rate = runs / balls
    positive = rate > 0
    fallback = float(rate[positive].mean()) if positive.any() else 0.0
    return np.where(positive, rate, fallback)


# --- the innings ----------------------------------------------------------------------


@dataclass
class InningsDraws:
    """One side's batting innings over the draws, arrays in slot order (n, k) / (n,)."""

    runs: np.ndarray
    balls: np.ndarray
    extras: np.ndarray
    total: np.ndarray
    wickets: np.ndarray
    deliveries: np.ndarray
    reached_target: np.ndarray  # False for a first innings
    #: The total before the target truncation (the total itself for a first innings): what
    #: the chase response's fit reads as the side's expected chase.
    untruncated_total: np.ndarray


def _distribute_remainder(runs: np.ndarray, balls: np.ndarray, remainder: np.ndarray, rate: np.ndarray) -> None:
    """The not-out pair faces the innings' unused deliveries at their expected strike
    rates, in place. ``remainder`` is zero where the innings is all out or already full."""
    batted = balls > 0
    depth = batted.sum(axis=1)
    rows = np.arange(len(balls))
    last = np.maximum(depth - 1, 0)
    previous = np.maximum(depth - 2, 0)
    to_previous = np.where(depth >= 2, np.rint(remainder * 0.5), 0.0)
    for index, extra_balls in ((previous, to_previous), (last, remainder - to_previous)):
        balls[rows, index] += extra_balls
        runs[rows, index] += np.rint(extra_balls * rate[index])


def _first_innings_constraints(
    runs: np.ndarray, balls: np.ndarray, capacity: int, rate: np.ndarray
) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    """Apply the innings length: truncate at the capacity in slot order (runs pro rata at
    the sampled strike rate), then let the not-out pair face any deficit. Returns
    (deliveries used, wickets, all_out)."""
    k = balls.shape[1]
    before = np.cumsum(balls, axis=1) - balls
    allowed = np.clip(capacity - before, 0.0, None)
    truncated = np.minimum(balls, allowed)
    fraction = np.divide(truncated, balls, out=np.zeros_like(balls), where=balls > 0)
    runs[:] = np.rint(runs * fraction)
    balls[:] = truncated
    used = balls.sum(axis=1)
    all_out = ((balls > 0).sum(axis=1) == k) & (used < capacity)
    remainder = np.where(all_out, 0.0, capacity - used)
    _distribute_remainder(runs, balls, remainder, rate)
    used = balls.sum(axis=1)
    depth = (balls > 0).sum(axis=1)
    wickets = np.where(all_out, C.MAX_WICKETS, np.clip(depth - 2, 0, C.MAX_WICKETS)).astype(float)
    return used, wickets, all_out


def _chase(
    runs: np.ndarray, balls: np.ndarray, extras: np.ndarray, target: np.ndarray
) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    """End the innings at the target: contributions are runs plus extras pro rata to balls;
    the batter whose cumulative contribution reaches the target keeps exactly what was
    needed, later batters do not bat. Returns (extras, total, reached), with runs and
    balls updated in place."""
    used = np.maximum(balls.sum(axis=1), 1.0)
    per_ball_extras = extras / used
    contribution = runs + per_ball_extras[:, None] * balls
    cumulative = np.cumsum(contribution, axis=1)
    before = cumulative - contribution
    reached_at = cumulative >= target[:, None]
    reached = reached_at.any(axis=1)
    first = np.argmax(reached_at, axis=1)
    columns = np.arange(balls.shape[1])[None, :]
    needed = target[:, None] - before
    fraction = np.where(
        columns < first[:, None],
        1.0,
        np.where(
            columns == first[:, None],
            np.divide(needed, contribution, out=np.ones_like(contribution), where=contribution > 0),
            0.0,
        ),
    )
    fraction = np.where(reached[:, None], np.clip(fraction, 0.0, 1.0), 1.0)
    runs[:] = np.rint(runs * fraction)
    balls[:] = np.where((fraction > 0) & (balls > 0), np.maximum(np.rint(balls * fraction), 1.0), 0.0)
    extras = np.where(reached, target - runs.sum(axis=1), extras)
    total = runs.sum(axis=1) + extras
    return extras, total, reached


def _coupled_uniforms(rng: np.random.Generator, n: int, k: int, rho: float) -> tuple[np.ndarray, np.ndarray]:
    """Two (n, k) uniforms with Gaussian-copula correlation ``rho``: balls first, runs given
    balls, so a batter who faces more balls tends to score more without his strike rate
    being fixed."""
    z_balls = rng.standard_normal((n, k))
    z_runs = rho * z_balls + np.sqrt(1.0 - rho**2) * rng.standard_normal((n, k))
    return ndtr(z_balls), ndtr(z_runs)


def batting_innings(
    rng: np.random.Generator,
    side: SideForecast,
    context: MatchContext,
    n: int,
    rho: float,
    target: Optional[np.ndarray] = None,
    factor: Optional[np.ndarray] = None,
    chase_response: Optional[ChaseResponse] = None,
) -> InningsDraws:
    """Sample one side's innings ``n`` times (plan §3, steps 1-7 and the chase; §8.10 for
    the chase response, applied to a chase's runs draws before the truncation)."""
    order = side.batting_order
    k = len(order)
    p_bats = side.p_bats[order]
    # Depth: one uniform per draw; the first B batters in slot order bat, at least two.
    depth = np.clip((p_bats[None, :] > rng.random((n, 1))).sum(axis=1), min(2, k), k)
    bats = np.arange(k)[None, :] < depth[:, None]
    u_balls, u_runs = _coupled_uniforms(rng, n, k, rho)
    balls = quantile_function(side.balls[order], _conditional_level(p_bats[None, :], u_balls))
    runs = quantile_function(side.runs[order], _conditional_level(p_bats[None, :], u_runs))
    if factor is not None:
        runs = runs * factor[:, None]
    runs = np.maximum(np.rint(runs), 0.0) * bats
    balls = np.maximum(np.rint(balls), 1.0) * bats
    rate = _expected_strike_rate(side, order)
    used, wickets, _ = _first_innings_constraints(runs, balls, context.deliveries, rate)
    extras = rng.poisson(context.extras_per_ball * used).astype(float)
    reached = np.zeros(n, dtype=bool)
    if target is None:
        total = runs.sum(axis=1) + extras
        untruncated = total
    else:
        if chase_response is not None:
            _respond_to_difficulty(runs, extras, target, factor, chase_response)
        untruncated = runs.sum(axis=1) + extras
        extras, total, reached = _chase(runs, balls, extras, target)
        used = balls.sum(axis=1)
        depth_after = (balls > 0).sum(axis=1)
        wickets = np.where(reached, np.clip(depth_after - 2, 0, C.MAX_WICKETS), wickets).astype(float)
    inverse = np.empty(k, dtype=int)
    inverse[order] = np.arange(k)
    return InningsDraws(runs[:, inverse], balls[:, inverse], extras, total, wickets, used, reached, untruncated)


def _respond_to_difficulty(
    runs: np.ndarray, extras: np.ndarray, target: np.ndarray, factor: Optional[np.ndarray], response: ChaseResponse
) -> None:
    """Scale each draw's runs by the response to its difficulty, in place: the target over
    the side's expected total on the draw's pitch, where the expected total is the mean
    over the draws of the batters' runs divided by their shared factor, plus the extras."""
    pitch = np.ones(len(target)) if factor is None else factor
    batted = runs.sum(axis=1)
    expected = max(float(np.mean(batted / np.maximum(pitch, 1e-6))) + float(np.mean(extras)), 1.0)
    difficulty = target / (np.maximum(pitch, 1e-6) * expected)
    runs[:] = np.rint(runs * response.factor(difficulty)[:, None])


# --- bowling attribution --------------------------------------------------------------


@dataclass
class BowlingDraws:
    """The bowling side's attributions over the draws, in the eleven's own order (n, k)."""

    balls: np.ndarray
    conceded: np.ndarray
    wickets: np.ndarray


def _draft_bowlers(rng: np.random.Generator, side: SideForecast, cap: int, deliveries: np.ndarray) -> np.ndarray:
    """Who bowls: each with P(bowls), topped up in expected-balls order until the side can
    deliver the innings under the per-bowler cap."""
    bowls = rng.random((len(deliveries), len(side))) < side.p_bowls[None, :]
    needed = np.ceil(deliveries / max(cap, 1.0)).astype(int)
    short = needed - bowls.sum(axis=1)
    for j in np.argsort(-side.exp_balls_bowled, kind="stable"):
        add = (short > 0) & ~bowls[:, j]
        bowls[add, j] = True
        short = short - add
    return bowls


def _share_deliveries(bowls: np.ndarray, weight: np.ndarray, cap: int, deliveries: np.ndarray) -> np.ndarray:
    """Deliveries per bowler in proportion to ``weight`` among those bowling, water-filled
    under the cap (three passes cover any eleven)."""
    balls = np.zeros(bowls.shape)
    remaining = deliveries.astype(float)
    open_ = bowls.copy()
    for _ in range(3):
        w = open_ * weight[None, :]
        total_w = w.sum(axis=1, keepdims=True)
        share = np.divide(w * remaining[:, None], total_w, out=np.zeros_like(w), where=total_w > 0)
        capped = np.minimum(balls + share, cap)
        added = capped - balls
        balls = capped
        remaining = remaining - added.sum(axis=1)
        open_ = open_ & (balls < cap - 1e-9)
    return np.rint(balls)


def _multinomial_rows(rng: np.random.Generator, counts: np.ndarray, weight: np.ndarray) -> np.ndarray:
    """Split each row's count over the columns with the row's weights (uniform where all
    weights are zero)."""
    total = weight.sum(axis=1, keepdims=True)
    probabilities = np.where(total > 0, weight / np.where(total > 0, total, 1.0), 1.0 / weight.shape[1])
    return rng.multinomial(counts.astype(int), probabilities).astype(float)


def bowling_attribution(
    rng: np.random.Generator, side: SideForecast, context: MatchContext, innings: InningsDraws
) -> BowlingDraws:
    """Attribute the batting side's innings to the bowlers (plan §3, "Bowling attribution")."""
    bowls = _draft_bowlers(rng, side, context.bowler_cap, innings.deliveries)
    weight = side.exp_balls_bowled + 1e-3  # a drafted debutant still gets a share
    balls = _share_deliveries(bowls, weight, context.bowler_cap, innings.deliveries)
    expected_balls = np.maximum(side.exp_balls_bowled, 1.0)
    run_rate = side.conceded[:, 1] / expected_balls
    run_rate = np.where(run_rate > 0, run_rate, run_rate[run_rate > 0].mean() if (run_rate > 0).any() else 1.0)
    conceded = _multinomial_rows(rng, innings.total, balls * run_rate[None, :])
    wicket_rate = side.wickets_mean / expected_balls
    bowler_wickets = rng.binomial(innings.wickets.astype(int), context.bowler_wicket_share)
    wickets = _multinomial_rows(rng, bowler_wickets, balls * wicket_rate[None, :])
    return BowlingDraws(balls, conceded, wickets)


# --- the match ------------------------------------------------------------------------


@dataclass
class TeamDraws:
    """Everything one team did over the draws, in the eleven's own order."""

    player_keys: np.ndarray
    runs: np.ndarray  # (n, k)
    balls: np.ndarray
    bowled_balls: np.ndarray
    conceded: np.ndarray
    wickets: np.ndarray  # taken
    total: np.ndarray  # (n,)
    extras: np.ndarray
    wickets_lost: np.ndarray
    deliveries: np.ndarray
    untruncated_total: np.ndarray


@dataclass
class MatchDraws:
    team1: TeamDraws
    team2: TeamDraws
    team1_bats_first: np.ndarray  # (n,) bool
    winner: np.ndarray  # (n,) 1, 2 or 0 for a tie

    @property
    def n(self) -> int:
        return int(len(self.winner))


def _empty_team(keys: np.ndarray, n: int) -> TeamDraws:
    k = len(keys)
    return TeamDraws(keys, *(np.zeros((n, k)) for _ in range(5)), *(np.zeros(n) for _ in range(5)))


def _fill(team: TeamDraws, rows: np.ndarray, batting: InningsDraws, bowling: BowlingDraws) -> None:
    team.runs[rows], team.balls[rows] = batting.runs, batting.balls
    team.total[rows], team.extras[rows] = batting.total, batting.extras
    team.wickets_lost[rows], team.deliveries[rows] = batting.wickets, batting.deliveries
    team.untruncated_total[rows] = batting.untruncated_total
    team.bowled_balls[rows], team.conceded[rows], team.wickets[rows] = bowling.balls, bowling.conceded, bowling.wickets


def simulate_match(
    team1: SideForecasts,
    team2: SideForecasts,
    context: MatchContext,
    n: int = DEFAULT_SAMPLES,
    seed: int = 0,
    team1_bats_first: Optional[bool] = None,
    calibration: Optional[SimulatorCalibration] = None,
) -> MatchDraws:
    """Draw ``n`` matches. With the toss unknown half the draws are played each way, each
    with the matching forecasts; known, every draw is played that way. ``calibration`` is
    the artifact's (``PerformanceModels.simulation``); without one the runs-balls copula
    is independent and there is no shared factor."""
    calibration = calibration or SimulatorCalibration(0.0)
    shared_factor = calibration.shared_factor
    if context.format_code not in SIMULATED_FORMATS:
        raise SimulationUnavailable(f"format {context.format_code!r} has no innings length; nothing to simulate")
    rng = np.random.default_rng(seed)
    first_flags = np.arange(n) < (n + 1) // 2 if team1_bats_first is None else np.full(n, bool(team1_bats_first))
    out1, out2 = _empty_team(team1.bat_first.player_keys, n), _empty_team(team2.bat_first.player_keys, n)
    for team1_first in (True, False):
        rows = np.flatnonzero(first_flags == team1_first)
        if not len(rows):
            continue
        first, second = (team1, team2) if team1_first else (team2, team1)
        first_out, second_out = (out1, out2) if team1_first else (out2, out1)
        factor = shared_factor.sample(rng, len(rows)) if shared_factor is not None else None
        rho = calibration.runs_balls_rho
        innings1 = batting_innings(rng, first.bat_first, context, len(rows), rho, factor=factor)
        bowling1 = bowling_attribution(rng, second.when_chasing, context, innings1)
        innings2 = batting_innings(
            rng,
            second.when_chasing,
            context,
            len(rows),
            rho,
            target=innings1.total + 1.0,
            factor=factor,
            chase_response=calibration.chase_response,
        )
        bowling2 = bowling_attribution(rng, first.bat_first, context, innings2)
        _fill(first_out, rows, innings1, bowling2)
        _fill(second_out, rows, innings2, bowling1)
    winner = np.where(out1.total > out2.total, 1, np.where(out2.total > out1.total, 2, 0))
    return MatchDraws(out1, out2, first_flags, winner)


# --- summaries ------------------------------------------------------------------------


def _range(values: np.ndarray) -> Dict[str, float]:
    q10, median, q90 = np.quantile(values, [0.1, 0.5, 0.9])
    return {"q10": float(q10), "median": float(median), "q90": float(q90)}


def _median_band(total: np.ndarray) -> np.ndarray:
    low, high = np.quantile(total, [0.5 - MEDIAN_BAND, 0.5 + MEDIAN_BAND])
    band = (total >= low) & (total <= high)
    return band if band.any() else np.ones(len(total), dtype=bool)


def _spread_contributions(runs: np.ndarray, extras: np.ndarray, total: np.ndarray) -> tuple[np.ndarray, float, float]:
    """Cov(player, total) / Var(total) per player and for the extras; they sum to one."""
    sd = float(np.std(total))
    if sd <= 0:
        return np.zeros(runs.shape[1]), 0.0, 0.0
    centred_total = total - total.mean()
    players = ((runs - runs.mean(axis=0)) * centred_total[:, None]).mean(axis=0) / sd**2
    extras_share = float(((extras - extras.mean()) * centred_total).mean() / sd**2)
    return players, extras_share, sd


def summarize_team(team: TeamDraws) -> Dict[str, Any]:
    band = _median_band(team.total)
    shares, extras_share, sd = _spread_contributions(team.runs, team.extras, team.total)
    players: List[Dict[str, Any]] = []
    for i, key in enumerate(team.player_keys):
        players.append(
            {
                "player_key": key,
                "p_bats": float((team.balls[:, i] > 0).mean()),
                "p_bowls": float((team.bowled_balls[:, i] > 0).mean()),
                "runs": _range(team.runs[:, i]),
                "balls_faced": _range(team.balls[:, i]),
                "wickets": _range(team.wickets[:, i]),
                "runs_conceded": _range(team.conceded[:, i]),
                "balls_bowled": _range(team.bowled_balls[:, i]),
                "scorecard": {
                    "runs": float(team.runs[band, i].mean()),
                    "balls_faced": float(team.balls[band, i].mean()),
                    "wickets": float(team.wickets[band, i].mean()),
                    "runs_conceded": float(team.conceded[band, i].mean()),
                    "balls_bowled": float(team.bowled_balls[band, i].mean()),
                },
                "spread_share": float(shares[i]),
                "spread_runs": float(shares[i] * sd),
            }
        )
    total = _range(team.total)
    total.update({"mean": float(team.total.mean()), "sd": sd, "scorecard": float(team.total[band].mean())})
    return {
        "total": total,
        "extras": {"scorecard": float(team.extras[band].mean()), "spread_share": extras_share},
        "wickets_lost": _range(team.wickets_lost),
        "players": players,
    }


def summarize_margin(draws: MatchDraws) -> Dict[str, Any]:
    """The margin as cricket states it: runs when the side batting first wins; balls
    remaining and wickets in hand when the chaser wins."""
    first_total = np.where(draws.team1_bats_first, draws.team1.total, draws.team2.total)
    second_total = np.where(draws.team1_bats_first, draws.team2.total, draws.team1.total)
    second_deliveries = np.where(draws.team1_bats_first, draws.team2.deliveries, draws.team1.deliveries)
    second_wickets = np.where(draws.team1_bats_first, draws.team2.wickets_lost, draws.team1.wickets_lost)
    bat_first_won = first_total > second_total
    chaser_won = second_total > first_total
    capacity = np.maximum(second_deliveries.max(), 1.0)
    out: Dict[str, Any] = {
        "p_bat_first_wins": float(bat_first_won.mean()),
        "p_chaser_wins": float(chaser_won.mean()),
        "p_tie": float((draws.winner == 0).mean()),
    }
    if bat_first_won.any():
        out["runs_when_bat_first_wins"] = _range((first_total - second_total)[bat_first_won])
    if chaser_won.any():
        out["balls_remaining_when_chaser_wins"] = _range((capacity - second_deliveries)[chaser_won])
        out["wickets_in_hand_when_chaser_wins"] = _range((C.MAX_WICKETS - second_wickets)[chaser_won])
    return out


def summarize(draws: MatchDraws) -> Dict[str, Any]:
    return {
        "n_samples": draws.n,
        "team1": summarize_team(draws.team1),
        "team2": summarize_team(draws.team2),
        "win": {
            "team1": float((draws.winner == 1).mean()),
            "team2": float((draws.winner == 2).mean()),
            "tie": float((draws.winner == 0).mean()),
        },
        "margin": summarize_margin(draws),
        "toss_marginalised": bool(draws.team1_bats_first.any() and not draws.team1_bats_first.all()),
    }


# --- fixtures from frames -------------------------------------------------------------


def complete_first_innings(win_rows: pd.DataFrame) -> np.ndarray:
    """Which matches' first innings ran their course -- all out, or at least the legal
    balls delivered -- so their total is comparable with a simulated full innings. A
    rain-shortened innings is neither and is left out of the totals check."""
    legal = win_rows.format_code.map(lambda f: C.INNINGS_LEGAL_BALLS.get(f) or np.inf).to_numpy(dtype=float)
    wickets = win_rows.innings1_wickets.to_numpy(dtype=float)
    deliveries = win_rows.innings1_deliveries.to_numpy(dtype=float)
    return (wickets >= C.MAX_WICKETS) | (deliveries >= legal)


Predict = Callable[[pd.DataFrame, Optional[bool]], Dict[str, Any]]


@dataclass(frozen=True)
class Fixture:
    """One match's simulator inputs, assembled from player rows and the win row."""

    match_id: str
    team1: SideForecasts
    team2: SideForecasts
    context: MatchContext


def fixtures_from_rows(player_rows: pd.DataFrame, win_rows: pd.DataFrame, predict: Predict) -> List[Fixture]:
    """Simulator inputs for every match in ``win_rows`` whose players are in ``player_rows``.
    ``predict`` is the L2-B prediction for rows under a forced orientation; it is called
    twice (both sides batting first, both chasing) for the whole frame at once. The frame's
    ``side`` column says who batted first, which the simulator ignores here: it re-derives
    both orientations, so the fixture can be simulated pre-toss or toss-known."""
    if not len(player_rows):
        return []
    rows = player_rows.reset_index(drop=True)
    bat_first, chasing = predict(rows, True), predict(rows, False)
    by_match = rows.groupby("match_id", sort=False).indices
    side = rows.side.to_numpy()
    fixtures: List[Fixture] = []
    for win_row in win_rows.itertuples():
        indices = by_match.get(win_row.match_id)
        if indices is None:
            continue
        sides = []
        for s in (1, 2):
            own = indices[side[indices] == s]
            sides.append(SideForecasts(side_forecast(rows, bat_first, own), side_forecast(rows, chasing, own)))
        fixtures.append(
            Fixture(win_row.match_id, sides[0], sides[1], MatchContext.from_row(win_row.format_code, win_row))
        )
    return fixtures


@dataclass(frozen=True)
class CalibrationDraws:
    """Per calibration fixture, simulated toss-known without a factor or a response: the
    first innings' simulated mean and sd (the shared factor's inputs) and the chasing
    side's expected untruncated total (the chase response's)."""

    first_mean: np.ndarray
    first_sd: np.ndarray
    chase_expected: np.ndarray


def simulate_calibration_fixtures(fixtures: Sequence[Fixture], rho: float, n: int, seed: int) -> CalibrationDraws:
    """Simulate each calibration fixture as it was played (toss known) under the members'
    forecasts alone, so what the fold-fitted parts learn is the residual after them."""
    without = SimulatorCalibration(rho)
    first_mean, first_sd, chase_expected = [], [], []
    for i, fixture in enumerate(fixtures):
        draws = simulate_match(
            fixture.team1, fixture.team2, fixture.context, n, seed + i, team1_bats_first=True, calibration=without
        )
        first_mean.append(draws.team1.total.mean())
        first_sd.append(draws.team1.total.std())
        chase_expected.append(draws.team2.untruncated_total.mean())
    return CalibrationDraws(np.asarray(first_mean), np.asarray(first_sd), np.asarray(chase_expected))
