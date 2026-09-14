"""Fit the XI-responsive win models and the performance models, and report held-out
discrimination.

This is the model-fitting half of the ``retrain`` step; ``ml.xi.retrain`` is the command,
and it is what decides where the artifacts go (one run directory) and what the run is
called. Nothing here knows about runs.

Per format two models are fitted on rows before the cutoff and scored on rows at or after it:

* objective  -- logistic regression on XI_FEATURE_COLS. Additive, so a hill-climb over XIs
                sees a smooth surface and a one-player upgrade never lowers the score
                (measured: <1% of upgrades move p by less than 0, vs 12% for unconstrained
                boosting). This is what the optimiser maximises.
* display    -- monotone-constrained gradient boosting on XI + team-context columns.
                Higher AUC; used for the probability shown to users.

The report records, per format: AUC and Brier for both models, the base-rate Brier, and the
best single raw column's AUC -- a model that cannot beat its own best column by a clear
margin is not being measured (S-3c). Each model is fitted once: the display model's
``random_state`` is not a source of replicate fits (EVAL-02, ``DISPLAY_RANDOM_STATE``), so
there is no seed spread to report, and the noise a difference is read against is the
harness's fold-level paired standard error (H-14).

The same run fits the performance model (L2-B, ``ml.xi.performance``) per format on the
player-match rows before the cutoff and scores it on the rows after it, per target beside
the career-mean baselines -- never pooled (H-12). Its artifact is ``xi_perf_<FORMAT>.joblib``.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import brier_score_loss, roc_auc_score
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

from ml.xi import contract as C
from ml.xi import perf_baselines, perf_harness, quality
from ml.xi.builder import BuildResult
from ml.xi.performance import default_spec, fit_performance
from ml.xi.store import FormatModels, save_models, save_performance, save_ratings

logger = logging.getLogger(__name__)

REPORT_NAME = "xi_win_report.json"


def make_objective_model() -> object:
    return make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=3000))


# The whole hyperparameter search this pipeline has (§9.3): three points for the display
# model, chosen inside the training rows and recorded in the run manifest.
#
# It replaces the Optuna / PyCaret / AutoGluon stack, which was removed because the model
# class was measured twice not to be the constraint. The first entry is what the display
# model has always been fitted with, and it is first on purpose -- see ``choose_display_params``.
DISPLAY_GRID: Tuple[Dict[str, float], ...] = (
    {"max_depth": 3, "learning_rate": 0.04, "max_iter": 300},
    {"max_depth": 3, "learning_rate": 0.08, "max_iter": 200},
    {"max_depth": 4, "learning_rate": 0.04, "max_iter": 300},
)

# A candidate has to beat the incumbent by more than this on the inner validation split
# before it displaces it. H-14's rule, applied to a choice rather than a report:
# differences under the noise floor are not evidence, and a grid that reshuffles the
# model on 0.001 of AUC every release is a source of drift, not of quality.
DISPLAY_GRID_MARGIN = 0.002

# The last fraction of the training rows, by date, that the grid is scored on. It is
# inside the training window and strictly before the holdout, so choosing a
# hyperparameter cannot see the rows the run is scored on (H-19: the locked window is
# never used for a choice).
GRID_VALIDATION_FRACTION = 0.2

# The display model is fitted once, under one fixed random state. HistGradientBoosting's
# ``random_state`` reaches nothing but the early-stopping validation split (which sklearn
# switches on by itself above 10,000 rows -- EVAL-01) and the binning subsample above
# 200,000 rows, so refitting under other seeds gave bit-identical models in every format
# but T20 and, in T20, measured only the luck of that split. The three-seed loop that used
# to sit here therefore reported a "seed spread" of exactly zero and called it a noise
# floor (EVAL-02). The fixed value keeps the T20 fit reproducible; it is not a replicate.
DISPLAY_RANDOM_STATE = 0


def make_display_model(
    columns: List[str],
    params: Optional[Dict[str, float]] = None,
    constrain_team_context: bool = C.DISPLAY_CONTEXT_MONOTONE_KEPT,
) -> object:
    """The display model. ``constrain_team_context`` is gate B-7's arm switch and defaults
    to what the gate decided; nothing but that experiment should pass it."""
    settings = dict(DISPLAY_GRID[0] if params is None else params)
    return HistGradientBoostingClassifier(
        max_depth=int(settings["max_depth"]),
        learning_rate=float(settings["learning_rate"]),
        max_iter=int(settings["max_iter"]),
        l2_regularization=1.0,
        min_samples_leaf=40,
        random_state=DISPLAY_RANDOM_STATE,
        monotonic_cst=C.monotone_directions(columns, constrain_team_context),
    )


def choose_display_params(train: pd.DataFrame) -> Dict:
    """Pick the display model's hyperparameters from ``DISPLAY_GRID``, inside the
    training rows.

    The split is temporal, not random: the rows a model is chosen on have to come after
    the rows it was fitted on, or the choice is made under a leak the serving path never
    enjoys. The incumbent (grid point 0) keeps its place unless a candidate beats it by
    more than ``DISPLAY_GRID_MARGIN``, so an unresolvable difference leaves the model
    where it is instead of moving it.

    Returns the chosen params and the scores, which go into the run manifest -- "which
    hyperparameters, and on what evidence" is exactly what the deleted tuning stack
    recorded in a database table nobody could join back to an artifact.
    """
    ordered = train.sort_values("match_date")
    split = int(len(ordered) * (1.0 - GRID_VALIDATION_FRACTION))
    inner_train, inner_valid = ordered.iloc[:split], ordered.iloc[split:]
    incumbent = dict(DISPLAY_GRID[0])
    if len(inner_train) < 50 or len(inner_valid) < 20 or inner_valid[C.TARGET_COL].nunique() < 2:
        return {"params": incumbent, "reason": "too few rows to choose on", "scores": []}

    x_tr, y_tr = _xy(inner_train, C.DISPLAY_FEATURE_COLS)
    x_va, y_va = _xy(inner_valid, C.DISPLAY_FEATURE_COLS)
    scores = []
    for candidate in DISPLAY_GRID:
        model = make_display_model(C.DISPLAY_FEATURE_COLS, candidate).fit(x_tr, y_tr)
        scores.append({"params": dict(candidate), "auc": float(roc_auc_score(y_va, model.predict_proba(x_va)[:, 1]))})
    baseline = scores[0]["auc"]
    best = max(scores[1:], key=lambda s: s["auc"], default=None)
    if best is not None and best["auc"] > baseline + DISPLAY_GRID_MARGIN:
        return {"params": best["params"], "reason": "beat the incumbent on the inner split", "scores": scores}
    return {
        "params": incumbent,
        "reason": f"no candidate beat the incumbent by more than {DISPLAY_GRID_MARGIN}",
        "scores": scores,
    }


def _xy(frame: pd.DataFrame, cols: List[str]):
    return frame[cols].fillna(0.0).to_numpy(dtype=float), frame[C.TARGET_COL].to_numpy(dtype=float)


def _score(model, x_te: np.ndarray, y_te: np.ndarray) -> Dict[str, float]:
    p = model.predict_proba(x_te)[:, 1]
    return {"auc": float(roc_auc_score(y_te, p)), "brier": float(brier_score_loss(y_te, p))}


def swap_orientation(frame: pd.DataFrame) -> pd.DataFrame:
    """The same fixtures with the sides exchanged (team2 bats first). Used to score the
    serving path, which averages both batting orders because the toss is unknown."""
    out = frame.copy()
    for stem in C.SIDE_FEATURE_STEMS:
        out[f"t1_{stem}"], out[f"t2_{stem}"] = frame[f"t2_{stem}"], frame[f"t1_{stem}"]
        out[f"d_{stem}"] = -frame[f"d_{stem}"]
    for col in ("team_elo_diff", "team_form_diff", "venue_fam_diff"):
        out[col] = -frame[col]
    out["team_h2h"] = 1.0 - frame["team_h2h"]
    return out


def marginalised_probabilities(model, te: pd.DataFrame, cols: List[str]) -> np.ndarray:
    """P(team1 wins) per row as the serving path computes it: both batting orders averaged."""
    x_a, _ = _xy(te, cols)
    x_b, _ = _xy(swap_orientation(te), cols)
    return 0.5 * (model.predict_proba(x_a)[:, 1] + (1.0 - model.predict_proba(x_b)[:, 1]))


def _score_marginalised(model, te: pd.DataFrame, cols: List[str]) -> Dict[str, float]:
    y = te[C.TARGET_COL].to_numpy(dtype=float)
    p = marginalised_probabilities(model, te, cols)
    return {"auc": float(roc_auc_score(y, p)), "brier": float(brier_score_loss(y, p))}


def gender_breakdown(objective, display, te: pd.DataFrame) -> Dict[str, Dict[str, float]]:
    """Holdout discrimination split by the gender of the match.

    20% of the dataset is women's cricket, and until P-1 it shared team identities with
    the men's game and blended careers wherever two people spelled their name the same.
    An aggregate AUC cannot show what that cost, because the men's subset dominates it --
    so identity work is measured here (E4) or not at all. Reported for information, never
    as a gate: the women's holdouts are small enough that a difference under ~0.03 is not
    resolvable.
    """
    out: Dict[str, Dict[str, float]] = {}
    for gender, rows in te.groupby("gender", sort=True):
        entry: Dict[str, float] = {"n_holdout": int(len(rows))}
        if len(rows) >= 20 and rows[C.TARGET_COL].nunique() == 2:
            entry["objective_auc"] = _score_marginalised(objective, rows, C.XI_FEATURE_COLS)["auc"]
            entry["display_auc"] = _score_marginalised(display, rows, C.DISPLAY_FEATURE_COLS)["auc"]
        out[str(gender)] = entry
    return out


def best_single_column(frame_te: pd.DataFrame, cols: Sequence[str]) -> Dict[str, float]:
    y = frame_te[C.TARGET_COL].to_numpy(dtype=float)
    best = ("", 0.5)
    for c in cols:
        a = roc_auc_score(y, frame_te[c].fillna(0.0))
        a = max(a, 1.0 - a)
        if a > best[1]:
            best = (c, float(a))
    return {"column": best[0], "auc": best[1]}


def train_format(
    frame: pd.DataFrame,
    format_code: str,
    cutoff: pd.Timestamp,
    gender_split_context: bool = False,
) -> tuple:
    """Fit both models for one format; return (FormatModels, report dict)."""
    d = frame[frame.format_code == format_code]
    tr, te = d[d.match_date < cutoff], d[d.match_date >= cutoff]
    report: Dict = {
        "format_code": format_code,
        "n_train": int(len(tr)),
        "n_holdout": int(len(te)),
    }
    if len(tr) < 50 or tr[C.TARGET_COL].nunique() < 2:
        report["skipped_reason"] = "insufficient training rows"
        return None, report
    x_obj_tr, y_tr = _xy(tr, C.XI_FEATURE_COLS)
    x_dis_tr, _ = _xy(tr, C.DISPLAY_FEATURE_COLS)
    objective = make_objective_model().fit(x_obj_tr, y_tr)
    grid = choose_display_params(tr)
    report["hyperparameters"] = grid
    display = make_display_model(C.DISPLAY_FEATURE_COLS, grid["params"]).fit(x_dis_tr, y_tr)
    if len(te) >= 20 and te[C.TARGET_COL].nunique() == 2:
        x_obj_te, y_te = _xy(te, C.XI_FEATURE_COLS)
        x_dis_te, _ = _xy(te, C.DISPLAY_FEATURE_COLS)
        report.update(
            {
                "holdout_positive_rate": float(y_te.mean()),
                "objective": _score(objective, x_obj_te, y_te),
                "objective_marginalised": _score_marginalised(objective, te, C.XI_FEATURE_COLS),
                "display": _score(display, x_dis_te, y_te),
                "display_marginalised": _score_marginalised(display, te, C.DISPLAY_FEATURE_COLS),
                "base_rate_brier": float(brier_score_loss(y_te, np.full(len(y_te), y_tr.mean()))),
                "best_single_column": best_single_column(te, C.DISPLAY_FEATURE_COLS),
                "by_gender": gender_breakdown(objective, display, te),
            }
        )
    else:
        report["holdout_note"] = "holdout too small or single-class; no discrimination numbers"
    metadata = {
        "format_code": format_code,
        "cutoff": cutoff.date().isoformat(),
        "n_train": int(len(tr)),
        "rating_params": {
            "decay_per_match": C.DECAY_PER_MATCH,
            "prior_balls": C.PRIOR_BALLS,
            "k_team_elo": C.K_TEAM_ELO,
            "k_player_elo": C.K_PLAYER_ELO,
        },
        "gender_split_context": gender_split_context,
        "hyperparameters": report.get("hyperparameters", {}),
        "report": report,
    }
    models = FormatModels(
        format_code=format_code,
        objective=objective,
        display=display,
        objective_cols=list(C.XI_FEATURE_COLS),
        display_cols=list(C.DISPLAY_FEATURE_COLS),
        metadata=metadata,
    )
    return models, report


def train_performance(
    player_frame: pd.DataFrame, match_frame: pd.DataFrame, format_code: str, cutoff: pd.Timestamp, artifacts_dir: str
) -> Dict:
    """Fit the format's performance model on the player rows before the cutoff, score it on
    the rows after, and write ``xi_perf_<FORMAT>.joblib``. The rows must carry the
    baseline predictors; ``match_frame`` (the win rows) feeds the simulator's calibration.
    Under E6's joint option the T20 and T20I artifacts hold one fit."""
    train, joint = perf_harness.training_rows(player_frame, format_code, cutoff)
    holdout = player_frame[(player_frame.format_code == format_code) & (player_frame.match_date >= cutoff)]
    report: Dict = {"n_train": int(len(train)), "n_holdout": int(len(holdout)), "joint_t20_formats": joint}
    if len(train) < perf_harness.MIN_TRAIN_ROWS:
        report["skipped_reason"] = "insufficient training rows"
        return report
    model = fit_performance(
        train, format_code, default_spec(joint_format=joint), match_frame[match_frame.match_date < cutoff]
    )
    report["fit"] = model.metadata
    if len(holdout) >= perf_harness.MIN_EVAL_ROWS:
        report["targets"] = perf_harness.score_targets(model, train, holdout)
    else:
        report["holdout_note"] = "holdout too small; no performance numbers"
    report["artifact"] = save_performance(model, format_code, artifacts_dir)
    return report


def train_all(
    result: BuildResult,
    artifacts_dir: str,
    cutoff: pd.Timestamp,
    formats: Sequence[str] = C.FORMAT_CODES,
    baseline_dir: Optional[str] = None,
) -> Dict:
    """Fit and write every model for one run into ``artifacts_dir``, and return the report.

    ``baseline_dir`` is where the data-quality baseline lives (H-15). It is separate from
    the artifacts directory because the baseline is *across* runs -- the last accepted
    counts -- while the artifacts belong to one; reading it from the run directory would
    compare every run against nothing.
    """
    os.makedirs(artifacts_dir, exist_ok=True)
    reports = []
    gender_split_context = result.state.gender_split_context
    player_frame = perf_baselines.add_baseline_predictors(result.player_frame)
    for fmt in formats:
        models, report = train_format(result.frame, fmt, cutoff, gender_split_context=gender_split_context)
        reports.append(report)
        if models is not None:
            path = save_models(models, artifacts_dir)
            logger.info(
                "%s: objective AUC %s, display AUC %s -> %s",
                fmt,
                report.get("objective", {}).get("auc"),
                report.get("display", {}).get("auc"),
                path,
            )
        else:
            logger.warning("%s: skipped (%s)", fmt, report.get("skipped_reason"))
        report["performance"] = train_performance(player_frame, result.frame, fmt, cutoff, artifacts_dir)
        _log_performance(fmt, report["performance"])
    save_ratings(result.state, artifacts_dir)
    # Read before anything is written: the baseline is the last accepted run's counts.
    gate = quality.check(result.quality, quality.load_baseline(baseline_dir or artifacts_dir))
    summary = {
        "cutoff": cutoff.date().isoformat(),
        "n_rows": int(len(result.frame)),
        "n_player_rows": int(len(result.player_frame)),
        "n_undecided": result.n_undecided,
        quality.REPORT_KEY: result.quality.as_dict(),
        "data_quality_failures": gate.failures,
        "formats": reports,
    }
    with open(os.path.join(artifacts_dir, REPORT_NAME), "w") as fh:
        json.dump(summary, fh, indent=2)
    return summary


def _log_performance(format_code: str, report: Dict) -> None:
    if "skipped_reason" in report:
        logger.warning("%s performance model: skipped (%s)", format_code, report["skipped_reason"])
        return
    for target, entry in report.get("targets", {}).items():
        if not entry["headline"]:
            continue
        model, baseline = entry["model"], entry["career_mean"]
        values = (
            model["within_match_spearman"],
            baseline["within_match_spearman"],
            model["pinball"],
            baseline["pinball"],
            model["interval"]["coverage_80"],
            model["interval"]["width_80"],
        )
        if any(v is None for v in values):
            continue  # a holdout too small to rank; the report still carries what was measured
        logger.info(
            "%-5s %-14s Spearman %.3f (career mean %.3f) | pinball %.3f (%.3f) | coverage %.3f width %.2f",
            format_code,
            target,
            *values,
        )


def _international_teams_from_config() -> List[str]:
    """The go-app config's international team list, so offline runs use go-app's format taxonomy."""
    here = os.path.dirname(os.path.abspath(__file__))
    for candidate in (
        os.path.join(here, "..", "..", "..", "go-app", "config.json"),
        os.environ.get("GO_APP_CONFIG", ""),
    ):
        if candidate and os.path.exists(candidate):
            with open(candidate) as fh:
                return list(json.load(fh).get("formats", {}).get("international_teams", []))
    logger.warning("go-app config.json not found; T20 between international sides will not be classed as T20I")
    return []
