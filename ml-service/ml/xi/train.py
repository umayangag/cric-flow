"""Fit the XI-responsive win models and the performance models, and report held-out
discrimination.

This is the model-fitting half of the ``retrain`` step; ``ml.xi.retrain`` is the command,
and it is what decides where the artifacts go (one run directory) and what the run is
called. Nothing here knows about runs.

Per format two models are fitted on rows before the cutoff and scored on rows at or after it:

* objective  -- logistic regression on XI_FEATURE_COLS, fitted under the contract's signs
                (``contract.monotone_directions``, the same directions the display model
                obeys): a coefficient on a ``+1`` stem cannot be negative, so the surface a
                hill-climb over XIs sees is additive *and* monotone by construction -- a
                one-player upgrade never lowers the score in either batting order. Before
                FEAT-14 the fit was unconstrained and the T20I objective carried a negative
                own-side weight on ``pelo_mean``. Its regularisation and the recency weight
                on its rows are chosen per format from ``OBJECTIVE_GRID`` on the inner
                temporal split (EVAL-13), the way the display model's settings are. This is
                what the optimiser maximises.
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
from typing import Callable, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.metrics import brier_score_loss, roc_auc_score
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

from ml.xi import contract as C
from ml.xi import perf_baselines, perf_harness, quality
from ml.xi.builder import BuildResult
from ml.xi.display_regression import UNRECORDED_LEVEL
from ml.xi.performance import default_spec, fit_performance
from ml.xi.signed_logistic import SignedLogisticRegression
from ml.xi.store import FormatModels, save_models, save_performance, save_ratings

logger = logging.getLogger(__name__)

REPORT_NAME = "xi_win_report.json"

# The objective's grid (EVAL-13): sklearn's inverse L2 strength ``C``, and the half-life in
# years of an exponential recency weight on the training rows (``None`` weighs every row
# alike). Before it one ``C`` served 1.9k-row TEST and 10k-row T20 -- the penalty is per
# fit, not per row, so the same value regularised TEST five times harder -- and a row from
# 2005 weighed what a row from 2026 did. The first of each axis is the incumbent, what the
# objective has always been fitted with, and ``OBJECTIVE_GRID`` puts that pair first
# because ``_choose_on_inner_split`` keeps grid point 0 unless a candidate beats it by
# more than ``GRID_MARGIN``. Neither axis is evidenced beyond the inner split: the choice
# is recorded per run and per harness window, and the walk-forward number scores the
# recipe with the grid in it.
OBJECTIVE_C_GRID: Tuple[float, ...] = (0.3, 0.1, 1.0)
OBJECTIVE_HALF_LIFE_YEARS_GRID: Tuple[Optional[float], ...] = (None, 8.0, 4.0)
OBJECTIVE_GRID: Tuple[Dict[str, Optional[float]], ...] = tuple(
    {"C": c, "half_life_years": half_life} for half_life in OBJECTIVE_HALF_LIFE_YEARS_GRID for c in OBJECTIVE_C_GRID
)
#: The solver's iteration ceiling. High enough that the bounded fit converges rather than
#: being stopped, so it is a guard and not a hyperparameter.
OBJECTIVE_MAX_ITER = 3000
DAYS_PER_YEAR = 365.25


def make_objective_model(columns: List[str], params: Optional[Dict[str, Optional[float]]] = None) -> object:
    """The selection objective: standardised inputs, then a logistic regression whose
    coefficients are bounded by the contract's sign for each column (FEAT-14), at the grid
    point ``params`` -- the incumbent when none is given."""
    settings = OBJECTIVE_GRID[0] if params is None else params
    return make_pipeline(
        StandardScaler(),
        SignedLogisticRegression(
            signs=C.monotone_directions(columns), C=float(settings["C"]), max_iter=OBJECTIVE_MAX_ITER
        ),
    )


def recency_weights(match_dates: pd.Series, half_life_years: Optional[float]) -> np.ndarray:
    """One weight per row, ``0.5 ** (age / half-life)`` with the age measured back from the
    latest date in ``match_dates`` so the newest row always weighs 1; all ones without a
    half-life."""
    if half_life_years is None:
        return np.ones(len(match_dates))
    age_days = (match_dates.max() - match_dates).dt.total_seconds().to_numpy(dtype=float) / 86400.0
    return np.power(0.5, age_days / (float(half_life_years) * DAYS_PER_YEAR))


def fit_objective(train: pd.DataFrame, params: Dict[str, Optional[float]]) -> object:
    """The objective fitted on ``train`` at one grid point: ``C`` in the estimator, the
    half-life as a weight per row."""
    x, y = _xy(train, C.XI_FEATURE_COLS)
    weights = recency_weights(train.match_date, params["half_life_years"])
    return make_objective_model(C.XI_FEATURE_COLS, params).fit(x, y, signedlogisticregression__sample_weight=weights)


def own_side_sensitivities(objective, columns: List[str]) -> Dict[str, Tuple[float, float]]:
    """Per stem, how the fitted objective's logit moves when the stem rises by one raw unit
    on the side being scored, in each batting order: ``(w_d + w_t1, w_d - w_t2)`` -- the
    side as team1 (bats first) and as team2. The contract's sign holds on the surface
    exactly when both carry it for every signed stem; FEAT-14 found the served T20I
    objective at -0.016 on ``pelo_mean`` with the toss marginalised."""
    scaler = objective.named_steps["standardscaler"]
    estimator = objective.steps[-1][1]
    raw_weights = dict(zip(columns, estimator.coef_[0] / scaler.scale_))
    out: Dict[str, Tuple[float, float]] = {}
    for stem in C.SIDE_FEATURE_STEMS:
        weight_d = raw_weights.get(f"d_{stem}", 0.0)
        weight_t1 = raw_weights.get(f"t1_{stem}", 0.0)
        weight_t2 = raw_weights.get(f"t2_{stem}", 0.0)
        if any(f"{prefix}_{stem}" in raw_weights for prefix in ("d", "t1", "t2")):
            out[stem] = (weight_d + weight_t1, weight_d - weight_t2)
    return out


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
# before it displaces it -- for both grids. H-14's rule, applied to a choice rather than
# a report: differences under the noise floor are not evidence, and a grid that reshuffles
# the model on 0.001 of AUC every release is a source of drift, not of quality.
GRID_MARGIN = 0.002

# The last fraction of the training rows, by date, that the grid is scored on. It is
# inside the training window and strictly before the holdout, so choosing a
# hyperparameter cannot see the rows the run is scored on (H-19: the locked window is
# never used for a choice).
GRID_VALIDATION_FRACTION = 0.2

# The display model is fitted once, under one fixed random state. HistGradientBoosting's
# ``random_state`` reaches nothing but the early-stopping validation split (off, below --
# EVAL-01) and the binning subsample above 200,000 rows, so refitting under other seeds
# gives bit-identical models; while early stopping still switched itself on in T20 the
# seed measured only the luck of that split. The three-seed loop that used to sit here
# therefore reported a "seed spread" of exactly zero and called it a noise floor
# (EVAL-02). The fixed value is a record, not a replicate.
DISPLAY_RANDOM_STATE = 0

# What the display model is fitted under whatever the grid picks. Named here rather than
# spelled inline in the constructor because a number that decides what is fitted and lives
# only in this file cannot be read back off the run it produced: these reach the manifest
# through ``win_model_params`` (EVAL-12), beside the grid's choice rather than inside it.
#
# ``early_stopping`` is explicitly off, never sklearn's ``'auto'`` (EVAL-01). ``'auto'``
# switches early stopping on above 10,000 rows with a random 10 % validation split, so the
# grid, which scores each candidate on the inner 80 % of a format's rows, fitted every
# candidate to its full ``max_iter`` and then the winner was refitted on all the rows --
# above the line in T20 -- to a different, early-stopped iteration count on 90 % of them.
# With it off the model the grid scored is the model fitted, ``max_iter`` means what the
# grid says it means, and no row is held back.
DISPLAY_FIXED_PARAMS: Dict[str, object] = {
    "l2_regularization": 1.0,
    "min_samples_leaf": 40,
    "early_stopping": False,
    "random_state": DISPLAY_RANDOM_STATE,
}


def win_model_params() -> Dict[str, object]:
    """The constants both win models are fitted under, for the run manifest (EVAL-12).

    Not the grid's choice -- ``hyperparameters`` is the only record of that (EVAL-06) --
    but the levers it never varies: without them a reader knows which of three grid points
    was picked and nothing about the model it was picked for.
    """
    return {
        "objective_max_iter": OBJECTIVE_MAX_ITER,
        "display_fixed": dict(DISPLAY_FIXED_PARAMS),
        "grid_margin": GRID_MARGIN,
        "grid_validation_fraction": GRID_VALIDATION_FRACTION,
        "display_context_monotone": C.DISPLAY_CONTEXT_MONOTONE_KEPT,
    }


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
        monotonic_cst=C.monotone_directions(columns, constrain_team_context),
        **DISPLAY_FIXED_PARAMS,
    )


def _choose_on_inner_split(
    train: pd.DataFrame,
    grid: Sequence[Dict],
    score_candidate: Callable[[Dict, pd.DataFrame, pd.DataFrame], float],
) -> Dict:
    """Pick one point of ``grid`` inside the training rows: each candidate is fitted on the
    first ``1 - GRID_VALIDATION_FRACTION`` of them by date and scored on the rest.

    The split is temporal, not random: the rows a model is chosen on have to come after
    the rows it was fitted on, or the choice is made under a leak the serving path never
    enjoys; and it is inside the training window, strictly before the holdout, so a
    choice cannot see the rows the run is scored on (H-19). The incumbent (grid point 0)
    keeps its place unless a candidate beats it by more than ``GRID_MARGIN``, so an
    unresolvable difference leaves the model where it is instead of moving it.

    Returns the chosen params, the reason and every candidate's score, which go into the
    run manifest -- "which hyperparameters, and on what evidence" is exactly what the
    deleted tuning stack recorded in a database table nobody could join back to an
    artifact.
    """
    ordered = train.sort_values("match_date")
    split = int(len(ordered) * (1.0 - GRID_VALIDATION_FRACTION))
    inner_train, inner_valid = ordered.iloc[:split], ordered.iloc[split:]
    incumbent = dict(grid[0])
    if len(inner_train) < 50 or len(inner_valid) < 20 or inner_valid[C.TARGET_COL].nunique() < 2:
        return {"params": incumbent, "reason": "too few rows to choose on", "scores": []}
    scores = [
        {"params": dict(candidate), "auc": float(score_candidate(candidate, inner_train, inner_valid))}
        for candidate in grid
    ]
    baseline = scores[0]["auc"]
    best = max(scores[1:], key=lambda s: s["auc"], default=None)
    if best is not None and best["auc"] > baseline + GRID_MARGIN:
        return {"params": best["params"], "reason": "beat the incumbent on the inner split", "scores": scores}
    return {
        "params": incumbent,
        "reason": f"no candidate beat the incumbent by more than {GRID_MARGIN}",
        "scores": scores,
    }


def choose_display_params(train: pd.DataFrame) -> Dict:
    """The display model's hyperparameters from ``DISPLAY_GRID``, chosen on the inner
    temporal split by the candidate's toss-aware AUC."""

    def score(candidate: Dict, inner_train: pd.DataFrame, inner_valid: pd.DataFrame) -> float:
        x_tr, y_tr = _xy(inner_train, C.DISPLAY_FEATURE_COLS)
        x_va, y_va = _xy(inner_valid, C.DISPLAY_FEATURE_COLS)
        model = make_display_model(C.DISPLAY_FEATURE_COLS, candidate).fit(x_tr, y_tr)
        return roc_auc_score(y_va, model.predict_proba(x_va)[:, 1])

    return _choose_on_inner_split(train, DISPLAY_GRID, score)


def choose_objective_params(train: pd.DataFrame) -> Dict:
    """The objective's ``C`` and recency half-life from ``OBJECTIVE_GRID``, chosen on the
    inner temporal split by the candidate's marginalised AUC -- the reading the optimiser
    and ``/xi/predict-win`` use, since neither knows the toss (EVAL-05)."""

    def score(candidate: Dict, inner_train: pd.DataFrame, inner_valid: pd.DataFrame) -> float:
        return _score_marginalised(fit_objective(inner_train, candidate), inner_valid, C.XI_FEATURE_COLS)["auc"]

    return _choose_on_inner_split(train, OBJECTIVE_GRID, score)


def fit_objective_as_shipped(train: pd.DataFrame) -> Tuple[object, Dict]:
    """The objective the recipe produces from these training rows: the grid's pick, fitted
    on all of them, with the record of the choice -- the objective's counterpart of
    ``fit_display_model_as_shipped``, and both writers use it for the same reason."""
    grid = choose_objective_params(train)
    return fit_objective(train, grid["params"]), grid


def fit_display_model_as_shipped(train: pd.DataFrame) -> Tuple[object, Dict]:
    """The display model the recipe produces from these training rows: the grid's pick,
    fitted on all of them, with the record of the choice.

    Both writers fit through here -- ``train_format`` for the run that ships and the
    harness's ``_evaluate_win_window`` for every walk-forward window -- so the model the
    harness measures is the model a retrain at that cutoff would have served. The harness
    used to fit grid point 0 regardless (EVAL-06), which left a format whose grid picked
    another point with no walk-forward evidence for the model it served.

    The record carries the pick, the reason, every candidate's inner-split score and the
    iterations the fit ran (equal to the chosen ``max_iter`` by construction, EVAL-01); it
    reaches the run manifest under ``hyperparameters`` and each harness window under the
    same key.
    """
    grid = choose_display_params(train)
    x_train, y_train = _xy(train, C.DISPLAY_FEATURE_COLS)
    display = make_display_model(C.DISPLAY_FEATURE_COLS, grid["params"]).fit(x_train, y_train)
    return display, {**grid, "n_iter": int(display.n_iter_)}


def _xy(frame: pd.DataFrame, cols: List[str]):
    return frame[cols].fillna(0.0).to_numpy(dtype=float), frame[C.TARGET_COL].to_numpy(dtype=float)


def _score(model, x_te: np.ndarray, y_te: np.ndarray) -> Dict[str, float]:
    p = model.predict_proba(x_te)[:, 1]
    return {"auc": float(roc_auc_score(y_te, p)), "brier": float(brier_score_loss(y_te, p))}


#: The team-context columns that change sign when the sides are exchanged.
SIGNED_CONTEXT_COLS = ("team_elo_diff", "team_form_diff", "venue_fam_diff", "home_diff")


def swap_orientation(frame: pd.DataFrame) -> pd.DataFrame:
    """The same fixtures with the sides exchanged (team2 bats first). Used to score the
    serving path, which averages both batting orders because the toss is unknown. The
    toss winner is a fact of the fixture, so exchanging the batting order reads it as
    1 - x: the side that won it now bats second. The competition level is a fact of the
    fixture too, and one the exchange leaves alone: it is not read as a side's."""
    out = frame.copy()
    for stem in C.SIDE_FEATURE_STEMS:
        out[f"t1_{stem}"], out[f"t2_{stem}"] = frame[f"t2_{stem}"], frame[f"t1_{stem}"]
        out[f"d_{stem}"] = -frame[f"d_{stem}"]
    for col in SIGNED_CONTEXT_COLS:
        out[col] = -frame[col]
    out["team_h2h"] = 1.0 - frame["team_h2h"]
    if C.TOSS_COL in frame.columns:
        out[C.TOSS_COL] = 1.0 - frame[C.TOSS_COL]
    return out


def toss_variants(frame: pd.DataFrame, cols: List[str]) -> List[pd.DataFrame]:
    """The frame read at each answer to "who won the toss", when the model reads the
    toss at all: what the serving path averages over, since no request carries it."""
    if C.TOSS_COL not in cols:
        return [frame]
    return [frame.assign(**{C.TOSS_COL: value}) for value in (1.0, 0.0)]


def competition_level_variants(frame: pd.DataFrame, cols: List[str]) -> List[pd.DataFrame]:
    """The frame read at each answer to "which level is this fixture", for the rows whose
    source recorded none, when the model reads the level at all. A recorded row is read at
    its own level in both variants -- the reading a request that names the level gets --
    and an unrecorded one is averaged over both, as ``XiStore.display_probability`` averages
    a request that names none; the unrecorded value itself is never read into the model,
    on either path."""
    if C.COMPETITION_LEVEL_COL not in cols:
        return [frame]
    unrecorded = frame[C.COMPETITION_LEVEL_COL] == C.COMPETITION_LEVEL_UNKNOWN
    return [
        frame.assign(**{C.COMPETITION_LEVEL_COL: frame[C.COMPETITION_LEVEL_COL].where(~unrecorded, value)})
        for value in (C.COMPETITION_LEVEL_VALUES[level] for level in C.COMPETITION_LEVELS)
    ]


def marginalised_probabilities(model, te: pd.DataFrame, cols: List[str]) -> np.ndarray:
    """P(team1 wins) per row as the serving path computes it: both batting orders
    averaged and, for a model that reads the toss, both toss winners (FEAT-05) -- the
    same four-way (or two-way) mean ``XiStore.display_probability`` takes -- and, for a
    row with no recorded competition level, both levels."""
    probabilities = []
    for frame, swapped in ((te, False), (swap_orientation(te), True)):
        for toss_variant in toss_variants(frame, cols):
            for variant in competition_level_variants(toss_variant, cols):
                p = model.predict_proba(_xy(variant, cols)[0])[:, 1]
                probabilities.append(1.0 - p if swapped else p)
    return np.mean(probabilities, axis=0)


def _score_marginalised(model, te: pd.DataFrame, cols: List[str]) -> Dict[str, float]:
    y = te[C.TARGET_COL].to_numpy(dtype=float)
    p = marginalised_probabilities(model, te, cols)
    return {"auc": float(roc_auc_score(y, p)), "brier": float(brier_score_loss(y, p))}


#: The smallest subset of holdout rows a breakdown scores: below it, and without both
#: classes, an AUC is noise with a decimal point.
MIN_BREAKDOWN_ROWS = 20


def _breakdown_by(objective, display, te: pd.DataFrame, column: str, rows_key: str) -> Dict[str, Dict[str, float]]:
    """Discrimination of both models on the rows of ``te`` grouped by ``column``: the row
    count under ``rows_key`` for every group, and both AUCs for a group large enough to
    score. Informational: no gate reads a breakdown."""
    out: Dict[str, Dict[str, float]] = {}
    for group, rows in te.groupby(column, sort=True):
        entry: Dict[str, float] = {rows_key: int(len(rows))}
        if len(rows) >= MIN_BREAKDOWN_ROWS and rows[C.TARGET_COL].nunique() == 2:
            entry["objective_auc"] = _score_marginalised(objective, rows, C.XI_FEATURE_COLS)["auc"]
            entry["display_auc"] = _score_marginalised(display, rows, C.DISPLAY_FEATURE_COLS)["auc"]
        out[str(group)] = entry
    return out


def gender_breakdown(objective, display, te: pd.DataFrame) -> Dict[str, Dict[str, float]]:
    """Holdout discrimination split by the gender of the match.

    20% of the dataset is women's cricket, and until P-1 it shared team identities with
    the men's game and blended careers wherever two people spelled their name the same.
    An aggregate AUC cannot show what that cost, because the men's subset dominates it --
    so identity work is measured here (E4) or not at all. Reported for information, never
    as a gate: the women's holdouts are small enough that a difference under ~0.03 is not
    resolvable.
    """
    return _breakdown_by(objective, display, te, "gender", "n_holdout")


def competition_level_breakdown(
    objective, display, te: pd.DataFrame, rows_key: str = "n_holdout"
) -> Dict[str, Dict[str, float]]:
    """Discrimination split by the competition level the source recorded, keyed by the
    level's word and ``UNRECORDED_LEVEL`` where it recorded none.

    The pooled formats are two populations under one code -- ODI is 3,512 internationals
    beside 1,483 domestic one-day rows, TEST 753 Tests beside 1,342 first-class rounds --
    and #356 measured them discriminating differently on the display model (ODI 0.73
    against 0.69, TEST 0.71 against 0.61). A pooled AUC cannot show which side a move
    came from, and the decision to keep them pooled rather than split was taken on this
    split, so it is reported wherever the pooled number is: the run manifest and every
    harness fold. Informational, like the gender split -- the Test rows reach twenty in
    one quarterly window of eleven, so their per-fold entry is mostly the count alone.
    """
    if "competition_level" in te.columns:
        levels = te["competition_level"].fillna("").astype(str).replace("", UNRECORDED_LEVEL)
    else:
        levels = pd.Series(UNRECORDED_LEVEL, index=te.index, dtype="object")
    return _breakdown_by(objective, display, te.assign(competition_level=levels), "competition_level", rows_key)


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
    y_tr = tr[C.TARGET_COL].to_numpy(dtype=float)
    objective, objective_record = fit_objective_as_shipped(tr)
    display, display_record = fit_display_model_as_shipped(tr)
    # One record per model, each the shape its grid writes: pick, reason, every candidate's
    # inner-split score, and for the display model the iterations it ran.
    report["hyperparameters"] = {"objective": objective_record, "display": display_record}
    if len(te) >= 20 and te[C.TARGET_COL].nunique() == 2:
        x_obj_te, y_te = _xy(te, C.XI_FEATURE_COLS)
        x_dis_te, _ = _xy(te, C.DISPLAY_FEATURE_COLS)
        # Each model is scored twice and both readings are named (EVAL-05). The
        # ``_marginalised`` block is the served quantity -- the optimiser and
        # ``/xi/predict-win`` average both batting orders because the toss is unknown when
        # they are asked -- and it is what the manifest quotes, under the same glossary
        # keys the harness reports the same quantity under. The ``_toss_aware`` block reads
        # the model at the order that actually happened, as the market benchmark does; it
        # used to be called plain ``objective`` / ``display`` and reach the manifest as
        # ``objective_auc``, so one key named two quantities depending on who wrote it.
        report.update(
            {
                "holdout_positive_rate": float(y_te.mean()),
                "objective_marginalised": _score_marginalised(objective, te, C.XI_FEATURE_COLS),
                "objective_toss_aware": _score(objective, x_obj_te, y_te),
                "display_marginalised": _score_marginalised(display, te, C.DISPLAY_FEATURE_COLS),
                "display_toss_aware": _score(display, x_dis_te, y_te),
                "base_rate_brier": float(brier_score_loss(y_te, np.full(len(y_te), y_tr.mean()))),
                "best_single_column": best_single_column(te, C.DISPLAY_FEATURE_COLS),
                "by_gender": gender_breakdown(objective, display, te),
                "by_competition_level": competition_level_breakdown(objective, display, te),
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
                "%s: objective AUC %s, display AUC %s (marginalised over the toss, as served) -> %s",
                fmt,
                report.get("objective_marginalised", {}).get("auc"),
                report.get("display_marginalised", {}).get("auc"),
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
