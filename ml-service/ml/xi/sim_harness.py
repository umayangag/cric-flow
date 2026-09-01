"""L4's simulation section (E2): is the simulator consistent with the display model, and
are its totals calibrated?

For one window it simulates every decided match of the format from the window's fitted
performance model -- toss-known (as the frame recorded it) and pre-toss (both orientations,
half the draws each) -- and reports, beside the display model scored on the same matches:

* **P(win)**: Brier of the simulated probability (pre-toss, the comparable one, and
  toss-known beside it) against the display model's and the base rate, with reliability
  curves for both. E2's rule (plan §5): worse than the display model by more than
  ``BRIER_TOLERANCE`` and the simulator's P(win) is a description, never the displayed
  probability. The rule is applied on the walk-forward folds, never the locked window.
* **Totals** (H-22 applied to totals): coverage *and* width of the simulated 10-90 interval
  against the actual first-innings total, on first innings that ran their course; the PIT
  histogram and the dispersion ratio (actual spread around the simulated mean over the
  simulated spread) say in which direction any miss lies -- under-dispersion is what the
  shared match factor exists for. The chase total is reported the same way, on every match.
* **Margins**: coverage and width of the run margin when the side batting first won and of
  the balls remaining when the chaser did, and the simulated share of bat-first wins
  against the actual one.
* **Latency**: per fixture, at the harness's draw count and at the served default.
"""

from __future__ import annotations

import logging
import time
from typing import Any, Dict, List, Optional, Sequence

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi import perf_harness, simulator
from ml.xi.performance import PerformanceModels
from ml.xi.train import marginalised_probabilities

logger = logging.getLogger(__name__)

#: Draws per fixture in the harness: enough for a 10-90 interval and a P(win) to 0.01.
HARNESS_SAMPLES = 1000
RELIABILITY_BINS = 10
#: E2's tolerance: how much worse than the display model the simulated P(win) may be.
BRIER_TOLERANCE = 0.01
MIN_MATCHES = 20


def _brier(p: np.ndarray, y: np.ndarray) -> Optional[float]:
    return float(np.mean((p - y) ** 2)) if len(y) else None


def reliability(p: np.ndarray, y: np.ndarray, bins: int = RELIABILITY_BINS) -> List[Dict[str, float]]:
    """Mean predicted against observed frequency per equal-width probability bin."""
    edges = np.linspace(0.0, 1.0, bins + 1)
    which = np.clip(np.digitize(p, edges[1:-1]), 0, bins - 1)
    out = []
    for b in range(bins):
        mask = which == b
        if not mask.any():
            continue
        out.append(
            {
                "lo": float(edges[b]),
                "hi": float(edges[b + 1]),
                "n": int(mask.sum()),
                "predicted": float(p[mask].mean()),
                "observed": float(y[mask].mean()),
            }
        )
    return out


def _interval_report(actual: np.ndarray, q10: np.ndarray, q90: np.ndarray) -> Dict[str, Any]:
    if not len(actual):
        return {"n": 0}
    return {
        "n": int(len(actual)),
        "coverage_80": float(np.mean((actual >= q10) & (actual <= q90))),
        "width_80": float(np.mean(q90 - q10)),
        "below_q10": float(np.mean(actual < q10)),
        "above_q90": float(np.mean(actual > q90)),
    }


def _totals_report(actual: np.ndarray, draws: Sequence[np.ndarray]) -> Dict[str, Any]:
    """Coverage, width, PIT deciles and dispersion of actual totals against their simulated
    distributions (one array of draws per match)."""
    if not len(actual):
        return {"n": 0}
    stacked = np.stack([np.quantile(d, [0.1, 0.5, 0.9]) for d in draws])
    means = np.asarray([d.mean() for d in draws])
    sds = np.asarray([d.std() for d in draws])
    pit = np.asarray([np.mean(d < a) + 0.5 * np.mean(d == a) for d, a in zip(draws, actual)])
    deciles = np.histogram(pit, bins=np.linspace(0.0, 1.0, 11))[0] / len(pit)
    residual = actual - means
    out = _interval_report(actual, stacked[:, 0], stacked[:, 2])
    out.update(
        {
            "median_mae": float(np.abs(actual - stacked[:, 1]).mean()),
            "bias": float(residual.mean()),
            "actual_sd_around_simulated_mean": float(residual.std()),
            "simulated_sd_mean": float(sds.mean()),
            # > 1: the simulator is under-dispersed (actual totals scatter more than its draws).
            "dispersion_ratio": float(residual.std() / max(sds.mean(), 1e-9)),
            "pit_deciles": [float(x) for x in deciles],
        }
    )
    return out


def _win_probability(draws: simulator.MatchDraws) -> float:
    """P(team1 wins) with a tie counted half."""
    return float(np.mean(draws.winner == 1) + 0.5 * np.mean(draws.winner == 0))


def evaluate_window(
    model: PerformanceModels,
    display_models: Sequence[Any],
    win_rows: pd.DataFrame,
    player_rows: pd.DataFrame,
    format_code: str,
    train_positive_rate: float,
    n_samples: int = HARNESS_SAMPLES,
    seed: int = 0,
) -> Dict[str, Any]:
    """E2 for one window's matches. ``win_rows`` are the window's win-frame rows (with the
    innings outcomes and the simulation context), ``player_rows`` the same matches' player
    rows, ``display_models`` the window's fitted display models (one per seed)."""
    if format_code not in simulator.SIMULATED_FORMATS:
        return {"skipped_reason": "format has no innings length; not simulated"}
    fixtures = simulator.fixtures_from_rows(player_rows, win_rows, model.predict_oriented)
    if len(fixtures) < MIN_MATCHES:
        return {"n_matches": len(fixtures), "skipped_reason": "too few matches to simulate"}
    by_id = win_rows.set_index("match_id")
    rows = by_id.loc[[f.match_id for f in fixtures]]
    y = rows[C.TARGET_COL].to_numpy(dtype=float)
    calibration = model.simulation

    p_pre, p_known = np.empty(len(fixtures)), np.empty(len(fixtures))
    first_draws: List[np.ndarray] = []
    chase_draws: List[np.ndarray] = []
    margin_runs, margin_balls = [], []
    bat_first_wins = np.empty(len(fixtures))
    started = time.perf_counter()
    for i, fixture in enumerate(fixtures):
        known = simulator.simulate_match(
            fixture.team1, fixture.team2, fixture.context, n_samples, seed + i, True, calibration
        )
        pre = simulator.simulate_match(
            fixture.team1, fixture.team2, fixture.context, n_samples, seed + i, None, calibration
        )
        p_known[i], p_pre[i] = _win_probability(known), _win_probability(pre)
        first_draws.append(known.team1.total)
        chase_draws.append(known.team2.total)
        bat_first_wins[i] = float(np.mean(known.winner == 1))
        team1_won = known.winner == 1
        team2_won = known.winner == 2
        margin_runs.append(
            np.quantile((known.team1.total - known.team2.total)[team1_won], [0.1, 0.9]) if team1_won.any() else None
        )
        margin_balls.append(
            np.quantile((fixture.context.deliveries - known.team2.deliveries)[team2_won], [0.1, 0.9])
            if team2_won.any()
            else None
        )
    seconds = time.perf_counter() - started
    default_started = time.perf_counter()
    simulator.simulate_match(fixtures[0].team1, fixtures[0].team2, fixtures[0].context, simulator.DEFAULT_SAMPLES, seed)
    default_seconds = time.perf_counter() - default_started

    p_display = np.mean(
        [marginalised_probabilities(m, rows.reset_index(), C.DISPLAY_FEATURE_COLS) for m in display_models], axis=0
    )
    brier_display, brier_pre = _brier(p_display, y), _brier(p_pre, y)
    complete = simulator.complete_first_innings(rows)
    actual_first = rows.innings1_runs.to_numpy(dtype=float)
    actual_chase = rows.innings2_runs.to_numpy(dtype=float)
    won_first = y == 1.0
    won_chase = y == 0.0
    runs_margin = np.asarray([m for m, w in zip(margin_runs, won_first) if w and m is not None])
    actual_runs_margin = (actual_first - actual_chase)[won_first][
        [m is not None for m, w in zip(margin_runs, won_first) if w]
    ]
    balls_margin = np.asarray([m for m, w in zip(margin_balls, won_chase) if w and m is not None])
    actual_balls_margin = (rows.ctx_innings_deliveries.round() - rows.innings2_deliveries).to_numpy(dtype=float)[
        won_chase
    ][[m is not None for m, w in zip(margin_balls, won_chase) if w]]
    report = {
        "n_matches": int(len(fixtures)),
        "n_samples": int(n_samples),
        "calibration": None if calibration is None else calibration.as_dict(),
        "chase_orientation": simulator.CHASE_ORIENTATION,
        "win": {
            "brier": {
                "display": brier_display,
                "simulated": brier_pre,
                "simulated_toss_known": _brier(p_known, y),
                "base_rate": _brier(np.full(len(y), train_positive_rate), y),
            },
            "delta_brier_simulated_minus_display": float(brier_pre - brier_display),
            "p_bat_first_wins": {"simulated": float(bat_first_wins.mean()), "actual": float(y.mean())},
            "reliability": {"display": reliability(p_display, y), "simulated": reliability(p_pre, y)},
        },
        "totals": {
            "first_innings": _totals_report(actual_first[complete], [d for d, c in zip(first_draws, complete) if c]),
            "chase": _totals_report(actual_chase, chase_draws),
        },
        "margin": {
            "runs_when_bat_first_wins": _interval_report(actual_runs_margin, runs_margin[:, 0], runs_margin[:, 1])
            if len(runs_margin)
            else {"n": 0},
            "balls_remaining_when_chaser_wins": _interval_report(
                actual_balls_margin, balls_margin[:, 0], balls_margin[:, 1]
            )
            if len(balls_margin)
            else {"n": 0},
        },
        "latency": {
            "ms_per_fixture_at_harness_samples": float(1000.0 * seconds / (2 * len(fixtures))),
            "ms_per_fixture_at_default_samples": float(1000.0 * default_seconds),
        },
    }
    logger.info(
        "%s simulation (E2): %d matches, Brier simulated %.4f vs display %.4f (Δ %+.4f); first-innings totals "
        "coverage %.3f width %.1f dispersion ratio %.2f; %.1f ms per fixture",
        format_code,
        len(fixtures),
        brier_pre,
        brier_display,
        report["win"]["delta_brier_simulated_minus_display"],
        report["totals"]["first_innings"].get("coverage_80", float("nan")),
        report["totals"]["first_innings"].get("width_80", float("nan")),
        report["totals"]["first_innings"].get("dispersion_ratio", float("nan")),
        report["latency"]["ms_per_fixture_at_harness_samples"],
    )
    return report


def _numeric_only(node: Any) -> Any:
    """The report without its lists (reliability curves, PIT deciles), which have no mean
    over folds."""
    if isinstance(node, dict):
        return {k: _numeric_only(v) for k, v in node.items() if not isinstance(v, list)}
    return node


def summarize_folds(folds: List[Dict]) -> Optional[Dict]:
    """Mean and spread over folds of every numeric leaf (``perf_harness.summarize_folds``)."""
    scored = [_numeric_only(f) for f in folds if f and "win" in f]
    return perf_harness.summarize_folds(scored) if scored else None


def decision(summary: Optional[Dict]) -> Dict[str, Any]:
    """E2's rule on the walk-forward summary: may the simulated P(win) be displayed?"""
    if not summary or "win" not in summary:
        return {"simulated_win_probability_displayed": False, "reason": "no simulated folds"}
    delta = summary["win"]["delta_brier_simulated_minus_display"]
    displayable = delta["mean"] <= BRIER_TOLERANCE
    return {
        "simulated_win_probability_displayed": bool(displayable),
        "delta_brier_mean": delta["mean"],
        "delta_brier_sd": delta["sd"],
        "n_folds": delta["n_folds"],
        "tolerance": BRIER_TOLERANCE,
        "reason": (
            "simulated P(win) within tolerance of the display model on the folds"
            if displayable
            else "simulated P(win) worse than the display model by more than the tolerance: a description, not the headline"
        ),
    }
