"""L4's performance section: fit the performance model on one window's training rows,
score it on the window's evaluation rows beside the baselines, per target and format.

Everything here is on the unconditional population (H-20). The model's headline numbers
are the pre-toss (innings-marginalised) predictions -- what is served -- with the
toss-known variant beside them. Ranking uses the distribution's mean for counts (a median
of 0 cannot rank bowlers) and the median for quantile targets; the point error is always
the median's; the proper score is the pinball loss; and every interval carries its width
beside its coverage (H-22).
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi import perf_baselines, perf_metrics
from ml.xi.perf_calibration import coverage_off_nominal
from ml.xi.performance import (
    INVOLVEMENT_COLS,
    TARGETS,
    FitSpec,
    PerformanceModels,
    TargetSpec,
    default_spec,
    fit_performance,
)

logger = logging.getLogger(__name__)

MIN_TRAIN_ROWS = 1000
MIN_EVAL_ROWS = 200

BASELINE_NAMES = ("career_mean", "career_quantiles", "rating_expectation")
MODEL_NAMES = ("model", "model_toss_known")


def score_forecast(t: TargetSpec, forecast: Dict[str, np.ndarray], rows: pd.DataFrame) -> Dict:
    """Metrics of one forecast of target ``t`` on ``rows``. A forecast carries a ``point``
    (used for ranking) and optionally ``quantiles`` (n, 3); a count forecast also carries
    ``p0`` / ``p1`` for the probability check."""
    ranking_point = forecast["point"]
    out = perf_metrics.score_point(rows, ranking_point, t.name)
    out["within_match_spearman_involved"] = _spearman_among_involved(t, ranking_point, rows)
    quantiles = forecast.get("quantiles")
    if quantiles is None:
        out.update({"pinball": None, "pinball_by_level": None, "interval": None})
        # A point forecast at every level: the literal "career mean" pinball loss.
        if len(rows):
            point_at_levels = np.repeat(ranking_point[:, None], len(perf_metrics.QUANTILE_LEVELS), axis=1)
            out["pinball"] = perf_metrics.mean_pinball(rows[t.name].to_numpy(dtype=float), point_at_levels)
        return out
    distribution = perf_metrics.score_quantiles(rows, quantiles, t.name)
    out.update({k: v for k, v in distribution.items() if k != "mae"})
    out["mae"] = distribution["mae"]  # the median's error, whatever ranked
    if "p0" in forecast and len(rows):
        out["probabilities"] = perf_metrics.score_count_probabilities(rows, forecast["p0"], forecast["p1"], t.name)
    return out


def _spearman_among_involved(t: TargetSpec, point: np.ndarray, rows: pd.DataFrame) -> Optional[float]:
    """Diagnostic only, never a headline: the ranking among the players who did bat / bowl.

    The headline Spearman is on the unconditional population, where a side's non-bowlers
    all take 0 and tie; a predictor that gives them one identical value (the career mean
    does) is rewarded for the tie, one that resolves them (any model) is charged for it.
    Restricting to the involved answers the selector's other question -- who among the
    bowlers takes the wickets -- on a population chosen by the outcome (the ceiling
    plan §1 quotes), so it is reported beside the headline, not instead of it.
    """
    if t.involvement is None or not len(rows) or INVOLVEMENT_COLS[t.involvement] not in rows:
        return None
    involved = rows[INVOLVEMENT_COLS[t.involvement]].to_numpy() > 0
    if involved.sum() < perf_metrics.MIN_PLAYERS_FOR_RANKING:
        return None
    subset = rows[involved]
    return perf_metrics.within_match_spearman(subset.match_id, point[involved], subset[t.name].to_numpy(dtype=float))


def model_forecasts(prediction: Dict[str, Any], t: TargetSpec) -> Dict[str, np.ndarray]:
    """A ``PerformanceModels`` prediction for one target in ``score_forecast``'s shape."""
    entry = prediction[t.name]
    if t.kind == "quantile":
        return {"point": entry["quantiles"][:, 1], "quantiles": entry["quantiles"]}
    return {"point": entry["mean"], "quantiles": entry["quantiles"], "p0": entry["p0"], "p1": entry["p1"]}


def _delta(model: Dict, baseline: Dict) -> Dict[str, Optional[float]]:
    """How far the model is ahead of a baseline: positive means better on both."""

    def diff(a: Optional[float], b: Optional[float]) -> Optional[float]:
        return None if a is None or b is None else float(a - b)

    return {
        "spearman": diff(model["within_match_spearman"], baseline["within_match_spearman"]),
        "pinball": diff(baseline["pinball"], model["pinball"]),
        "mae": diff(baseline["mae"], model["mae"]),
    }


def score_targets(model: PerformanceModels, train_rows: pd.DataFrame, eval_rows: pd.DataFrame) -> Dict[str, Dict]:
    """Per target the model fitted: the model (pre-toss and toss-known) and every baseline
    on ``eval_rows``."""
    marginalised = model.predict_marginalised(eval_rows)
    toss_known = model.predict_oriented(eval_rows, None)
    report: Dict[str, Dict] = {}
    for t in model.spec.target_specs:
        entry = {
            "headline": t.headline,
            "model": score_forecast(t, model_forecasts(marginalised, t), eval_rows),
            "model_toss_known": score_forecast(t, model_forecasts(toss_known, t), eval_rows),
        }
        for name, forecast in perf_baselines.baseline_predictions(train_rows, eval_rows, t.name).items():
            entry[name] = score_forecast(t, forecast, eval_rows)
        entry["vs_career_mean"] = _delta(entry["model"], entry["career_mean"])
        entry["vs_career_quantiles"] = _delta(entry["model"], entry["career_quantiles"])
        report[t.name] = entry
    return report


@dataclass
class WindowResult:
    report: Dict[str, Any]
    model: Optional[PerformanceModels]


def training_rows(player_frame: pd.DataFrame, format_code: str, cutoff: pd.Timestamp) -> Tuple[pd.DataFrame, bool]:
    """The rows a format's model trains on before ``cutoff``, and whether that is the E6
    joint T20 + T20I population (in which case the model reads the format indicator)."""
    joint = C.E6_JOINT_T20_FORMATS and format_code in C.E6_JOINT_FORMATS
    formats = list(C.E6_JOINT_FORMATS) if joint else [format_code]
    rows = player_frame[player_frame.format_code.isin(formats) & (player_frame.match_date < cutoff)]
    return rows, bool(joint)


def evaluate_window(train_rows: pd.DataFrame, eval_rows: pd.DataFrame, format_code: str, spec: FitSpec) -> WindowResult:
    """Fit on ``train_rows`` (< cutoff), score on ``eval_rows`` ([cutoff, end)). The rows
    must carry the baseline predictors (``perf_baselines.add_baseline_predictors``)."""
    skip = {"n_train": int(len(train_rows)), "n_eval": int(len(eval_rows))}
    if len(train_rows) < MIN_TRAIN_ROWS:
        return WindowResult({**skip, "skipped_reason": "insufficient training rows"}, None)
    if len(eval_rows) < MIN_EVAL_ROWS:
        return WindowResult({**skip, "skipped_reason": "evaluation window too small"}, None)
    model = fit_performance(train_rows, format_code, spec)
    logger.info(
        "%s performance model: %d train rows, %d eval rows, %.0f s, iterations %s",
        format_code,
        len(train_rows),
        len(eval_rows),
        model.metadata["fit_seconds"],
        {k: int(v) for k, v in model.iterations.items()},
    )
    report = {**skip, "fit": model.metadata, "targets": score_targets(model, train_rows, eval_rows)}
    return WindowResult(report, model)


def evaluate_fold(
    player_frame: pd.DataFrame,
    format_code: str,
    cutoff: pd.Timestamp,
    end: pd.Timestamp,
    recalibrate: Tuple[str, ...] = (),
) -> WindowResult:
    """One walk-forward (or locked) window of the L4 harness for a format."""
    train, joint = training_rows(player_frame, format_code, cutoff)
    window = player_frame[
        (player_frame.format_code == format_code)
        & (player_frame.match_date >= cutoff)
        & (player_frame.match_date < end)
    ]
    spec = default_spec(joint_format=joint, recalibrate=recalibrate)
    return evaluate_window(train, window, format_code, spec)


def recalibration_needed(summary: Optional[Dict]) -> List[str]:
    """H-5: quantile targets whose walk-forward mean exceedance is off nominal at either
    end (``perf_calibration.coverage_off_nominal``); the locked window and the served
    artifact recalibrate exactly these on a temporal fold."""
    if not summary or "targets" not in summary:
        return []
    needed = []
    for t in TARGETS:
        interval = summary["targets"].get(t.name, {}).get("model", {}).get("interval")
        if t.kind != "quantile" or not interval:
            continue
        ends = {
            key: {reading: interval[key][reading]["mean"] for reading in ("strict", "inclusive")}
            for key in ("q10", "q90")
        }
        if coverage_off_nominal(ends):
            needed.append(t.name)
    return needed


def summarize_folds(folds: List[Dict]) -> Any:
    """Mean and spread over folds of every numeric leaf of the per-fold performance
    reports, keeping the nesting; leaves no fold produced are dropped."""
    present = [f for f in folds if f is not None]
    if not present:
        return None
    sample = present[0]
    if isinstance(sample, dict):
        return {key: summarize_folds([f.get(key) for f in present if isinstance(f, dict)]) for key in sample}
    if isinstance(sample, (int, float)) and not isinstance(sample, bool):
        values = [float(f) for f in present if isinstance(f, (int, float)) and not isinstance(f, bool)]
        return {"mean": float(np.mean(values)), "sd": float(np.std(values)), "n_folds": len(values)}
    return sample
