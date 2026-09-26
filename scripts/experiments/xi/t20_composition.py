"""Batch 4's T20 fall, scored like-for-like: composition or regression? (experiment, not a gate)

Batch 4's harness read T20's objective AUC at 0.5792 against batch 3's 0.6966 (-2.8 fold sd,
every fold down) and recorded IMPORT-09's composition change -- 3,888 international T20s
moved out of T20 into T20I, leaving club cricket -- as the likely cause without attributing
it: nobody had scored the old code on the new population. This script does the 2 x 2 x 2:

* two **codes** -- batch 3's rating pass (``926a924d``, ``--code-root`` at a checkout of it)
  and the current one -- each over
* two **taxonomies** -- the archive as it stands (``archive``) and batch 3's (``batch3``: a
  T20I of Cricsheet type ``T20`` between sides not both in go-app's old ``international_teams``
  list goes back to ``T20``, the rule ``go-app/internal/cricsheet/format.go`` had before
  IMPORT-09), applied at the source so the rating state pools the way it did then;
* two **recipes** per surface, run under the current code with the recipe as parameters:
  batch 3's objective (40 columns, ``C`` 0.3, no recency weight) and batch 4's (29 columns,
  EVAL-13's grid over ``C`` and the half-life), plus EVAL-13's arms in between; batch 3's
  display (40 XI + 7 context columns) and batch 4's (29 + 8 + the toss). The current
  ``SignedLogisticRegression`` differs from batch 3's only by the ``sample_weight`` hook,
  which at unit weights is the same fit, so the old recipe under the new code *is* the old
  recipe.

Every cell is scored on the harness's own windows (``ml.xi.evaluate.fold_windows``), per
fold and pooled over folds, with the evaluation rows split by ``match.competition_level``
so the like-for-like comparison is on the same club matches. ``levels`` does the second
question -- per-format AUC by competition level, pooled model and level-only model -- on one
frame. Nothing here touches a run directory, a threshold or the served model.

    PYTHONPATH= python t20_composition.py build --taxonomy archive --env-file .env --out b4_archive.pkl
    PYTHONPATH= python t20_composition.py build --code-root <926a924d>/ml-service --taxonomy batch3 ...
    python t20_composition.py score --frames tag=path [tag=path ...] --formats T20 --out score.json
    python t20_composition.py levels --frames b4_archive.pkl --out levels.json
    python t20_composition.py decide --score score.json --levels levels.json
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import pickle
import sys
import time
from dataclasses import replace
from typing import Any, Dict, Iterator, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd
from sklearn.metrics import brier_score_loss, roc_auc_score

logger = logging.getLogger("t20_composition")

DEFAULT_CODE_ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service")

#: go-app's ``formats.international_teams`` at 926a924d, with ``treat_t20i_as_subset`` on:
#: a Cricsheet ``T20`` between two of these was a T20I; every other ``T20`` was club T20.
BATCH3_INTERNATIONAL_TEAMS = frozenset(
    name.lower()
    for name in (
        "Afghanistan",
        "Australia",
        "Bangladesh",
        "England",
        "India",
        "Ireland",
        "New Zealand",
        "Pakistan",
        "South Africa",
        "Sri Lanka",
        "West Indies",
        "Zimbabwe",
    )
)

# Batch 3's win-model contract at 926a924d: a differential for every stem plus both sides'
# raw values for the side stems (40 columns of rank 29, EVAL-13), and seven context columns.
BATCH3_STEMS: List[str] = [
    "pelo_mean",
    "pelo_top3",
    "pelo_min",
    "imp_bat_sum",
    "imp_bat_top6",
    "imp_bat_tail",
    "imp_bat_wk",
    "imp_bowl_sum",
    "imp_bowl_top5",
    "imp_bowl_wk",
    "imp_bowl_wk_top5",
    "n_bowlers",
    "exp_balls_bowled_top5",
    "exp_balls_faced_sum",
    "n_allrounders",
    "exp_mean_matches",
    "n_debutants",
    "exp_mean_matches_all",
]
BATCH3_SIDE_STEMS: List[str] = [
    "pelo_mean",
    "imp_bat_sum",
    "imp_bat_top6",
    "imp_bat_tail",
    "imp_bat_wk",
    "imp_bowl_sum",
    "imp_bowl_top5",
    "imp_bowl_wk",
    "imp_bowl_wk_top5",
    "n_bowlers",
    "n_debutants",
]
BATCH3_XI_COLS: List[str] = (
    [f"d_{s}" for s in BATCH3_STEMS] + [f"t1_{s}" for s in BATCH3_SIDE_STEMS] + [f"t2_{s}" for s in BATCH3_SIDE_STEMS]
)
BATCH3_CONTEXT_COLS: List[str] = [
    "team_elo_diff",
    "team_form_diff",
    "team_h2h",
    "team_h2h_n",
    "venue_bf_rate",
    "venue_n",
    "venue_fam_diff",
]
BATCH3_DISPLAY_COLS: List[str] = BATCH3_XI_COLS + BATCH3_CONTEXT_COLS

#: FEAT-05's two display columns, absent from a batch-3 frame; filled with their neutral
#: value there so the current ``swap_orientation`` can run. Only the ``b4`` display arm
#: reads them, and on such a frame that arm is labelled as running without them.
FEAT05_NEUTRAL: Dict[str, float] = {"home_diff": 0.0, "toss_won_by_team1": 0.5}

LEVELS = ("club", "international")
MIN_SUBSET_ROWS = 20

_LEVELS_SQL = """
SELECT m.match_id, mf.code, m.competition_level, m.gender, m.original_match_type
FROM match m JOIN match_format mf ON mf.id = m.format_id
"""
_T20I_SIDES_SQL = """
SELECT m.match_id, m.original_match_type, bat.opposition_name, bowl.opposition_name
FROM match m
JOIN match_format mf ON mf.id = m.format_id
JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
JOIN opposition bat ON bat.id = mi.batting_team_opposition_id
JOIN opposition bowl ON bowl.id = mi.bowling_team_opposition_id
WHERE mf.code = 'T20I'
"""


# --- build: one rating pass under one code over one taxonomy -----------------------------


def load_environment(env_file: Optional[str]) -> None:
    if env_file:
        from dotenv import load_dotenv

        load_dotenv(env_file)


def read_levels(connection) -> pd.DataFrame:
    """Every match's archive format and competition level, keyed the way the frame keys it."""
    with connection.cursor() as cur:
        cur.execute(_LEVELS_SQL)
        rows = cur.fetchall()
    return pd.DataFrame(
        [(str(r[0]), r[1], r[2], r[3], r[4]) for r in rows],
        columns=["match_id", "archive_format", "competition_level", "gender", "original_match_type"],
    )


def batch3_format_remap(connection) -> Dict[str, str]:
    """``match_id -> 'T20'`` for every archive T20I that batch 3's importer filed under T20."""
    with connection.cursor() as cur:
        cur.execute(_T20I_SIDES_SQL)
        rows = cur.fetchall()
    remap: Dict[str, str] = {}
    for match_id, match_type, side1, side2 in rows:
        if (match_type or "").strip().upper() != "T20":
            continue  # 'IT20' files were T20I under both rules
        both_international = all(s.strip().lower() in BATCH3_INTERNATIONAL_TEAMS for s in (side1 or "", side2 or ""))
        if not both_international:
            remap[str(match_id)] = "T20"
    logger.info("batch-3 taxonomy: %d archive T20I matches go back to T20", len(remap))
    return remap


class RemappedSource:
    """A source whose records carry another taxonomy's format code; everything else delegates."""

    def __init__(self, inner: Any, remap: Dict[str, str]):
        self.inner = inner
        self.remap = remap
        self.remapped = 0

    def __getattr__(self, name: str) -> Any:
        return getattr(self.inner, name)

    def iter_matches(self) -> Iterator[Any]:
        for record in self.inner.iter_matches():
            code = self.remap.get(record.match_id)
            if code is None:
                yield record
            else:
                self.remapped += 1
                yield replace(record, format_code=code)


def build_frame(taxonomy: str, code_root: str, out: str) -> None:
    import ml
    from ml.db import get_db_connection
    from ml.xi.builder import build
    from ml.xi.sources import PostgresSource

    logger.info("rating pass code: %s", os.path.dirname(os.path.abspath(ml.__file__)))
    connection = get_db_connection()
    levels = read_levels(connection)
    source: Any = PostgresSource(connection)
    if taxonomy == "batch3":
        source = RemappedSource(source, batch3_format_remap(connection))
    started = time.time()
    result = build(source, progress=lambda i: logger.info("rating pass: %d matches", i))
    seconds = time.time() - started
    frame = result.frame.copy()
    frame["match_date"] = pd.to_datetime(frame["match_date"])
    payload = {
        "code_root": os.path.abspath(code_root),
        "taxonomy": taxonomy,
        "seconds": seconds,
        "remapped": getattr(source, "remapped", 0),
        "n_rows": int(len(frame)),
        "rows_by_format": {k: int(v) for k, v in frame.format_code.value_counts().sort_index().items()},
        "frame": frame,
        "levels": levels,
    }
    with open(out, "wb") as fh:
        pickle.dump(payload, fh)
    logger.info(
        "frame written to %s: %d rows %s in %.0f s (%d remapped)",
        out,
        len(frame),
        payload["rows_by_format"],
        seconds,
        payload["remapped"],
    )


def load_frame(path: str) -> Dict[str, Any]:
    with open(path, "rb") as fh:
        payload = pickle.load(fh)
    frame = payload["frame"]
    levels = payload["levels"].set_index("match_id")
    frame["competition_level"] = frame.match_id.map(levels.competition_level)
    payload["feat05_filled"] = [col for col in FEAT05_NEUTRAL if col not in frame.columns]
    for col in payload["feat05_filled"]:
        frame[col] = FEAT05_NEUTRAL[col]
    payload["frame"] = frame
    return payload


# --- the recipes, as parameters ----------------------------------------------------------


def objective_arms() -> Dict[str, Tuple[List[str], Sequence[Dict[str, Optional[float]]]]]:
    """Name -> (columns, grid). A one-point grid is a fixed setting."""
    from ml.xi import contract as C
    from ml.xi.train import OBJECTIVE_GRID

    fixed_b3 = ({"C": 0.3, "half_life_years": None},)
    forced_b4_pick = ({"C": 0.1, "half_life_years": 4.0},)
    c_only = tuple(p for p in OBJECTIVE_GRID if p["half_life_years"] is None)
    return {
        "b3": (BATCH3_XI_COLS, fixed_b3),
        "b4": (list(C.XI_FEATURE_COLS), OBJECTIVE_GRID),
        "cols29_fixed": (list(C.XI_FEATURE_COLS), fixed_b3),
        "cols40_grid": (BATCH3_XI_COLS, OBJECTIVE_GRID),
        "cols29_c_grid_no_recency": (list(C.XI_FEATURE_COLS), c_only),
        "cols29_forced_c0.1_hl4": (list(C.XI_FEATURE_COLS), forced_b4_pick),
    }


def display_arms() -> Dict[str, List[str]]:
    from ml.xi import contract as C

    return {
        "b3": BATCH3_DISPLAY_COLS,
        "b4": list(C.DISPLAY_FEATURE_COLS),
        "cols29_ctx7": list(C.XI_FEATURE_COLS) + BATCH3_CONTEXT_COLS,
    }


def fit_objective_arm(train: pd.DataFrame, cols: List[str], grid: Sequence[Dict]) -> Tuple[object, Dict]:
    from ml.xi.train import _choose_on_inner_split, marginalised_probabilities

    if len(grid) == 1:
        record = {"params": dict(grid[0]), "reason": "fixed", "scores": []}
    else:

        def score(candidate: Dict, inner_train: pd.DataFrame, inner_valid: pd.DataFrame) -> float:
            model = _fit_objective(inner_train, cols, candidate)
            return roc_auc_score(inner_valid["team1_wins"], marginalised_probabilities(model, inner_valid, cols))

        record = _choose_on_inner_split(train, grid, score)
    return _fit_objective(train, cols, record["params"]), record


def _fit_objective(rows: pd.DataFrame, cols: List[str], params: Dict) -> object:
    from ml.xi.train import _xy, make_objective_model, recency_weights

    x, y = _xy(rows, cols)
    weights = recency_weights(rows.match_date, params["half_life_years"])
    return make_objective_model(cols, params).fit(x, y, signedlogisticregression__sample_weight=weights)


def fit_display_arm(train: pd.DataFrame, cols: List[str]) -> Tuple[object, Dict]:
    from ml.xi.train import (
        DISPLAY_GRID,
        _choose_on_inner_split,
        _xy,
        make_display_model,
    )

    def score(candidate: Dict, inner_train: pd.DataFrame, inner_valid: pd.DataFrame) -> float:
        x_tr, y_tr = _xy(inner_train, cols)
        x_va, y_va = _xy(inner_valid, cols)
        return roc_auc_score(y_va, make_display_model(cols, candidate).fit(x_tr, y_tr).predict_proba(x_va)[:, 1])

    record = _choose_on_inner_split(train, DISPLAY_GRID, score)
    x, y = _xy(train, cols)
    return make_display_model(cols, record["params"]).fit(x, y), record


def score_subsets(
    model: object, evaluation: pd.DataFrame, cols: List[str]
) -> Tuple[Dict[str, Dict], Dict[str, np.ndarray]]:
    """The marginalised AUC / Brier on every level subset of the window's rows that has
    enough of both classes, and the probabilities for the pooled-over-folds reading."""
    from ml.xi.train import marginalised_probabilities

    probabilities = marginalised_probabilities(model, evaluation, cols)
    scores: Dict[str, Dict] = {}
    predictions: Dict[str, np.ndarray] = {}
    for name, mask in _subset_masks(evaluation).items():
        y = evaluation.loc[mask, "team1_wins"].to_numpy(dtype=float)
        p = probabilities[mask.to_numpy()]
        entry: Dict[str, Any] = {"n": int(len(y))}
        if len(y) >= MIN_SUBSET_ROWS and len(np.unique(y)) == 2:
            entry["auc"] = float(roc_auc_score(y, p))
            entry["brier"] = float(brier_score_loss(y, p))
        scores[name] = entry
        predictions[name] = np.column_stack([y, p])
    return scores, predictions


def _subset_masks(rows: pd.DataFrame) -> Dict[str, pd.Series]:
    masks = {"all": pd.Series(True, index=rows.index)}
    for level in LEVELS:
        mask = rows.competition_level == level
        if mask.any():
            masks[level] = mask
    return masks


# --- score: the 2 x 2 x 2 on one format ------------------------------------------------


def windows() -> List[Tuple[pd.Timestamp, pd.Timestamp, str]]:
    from ml.xi.evaluate import LOCKED_START, fold_windows

    out = [(cutoff, end, "fold") for cutoff, end in fold_windows()]
    out.append((pd.Timestamp(LOCKED_START), pd.Timestamp("2100-01-01"), "locked"))
    return out


def training_subsets(format_frame: pd.DataFrame) -> List[str]:
    """``all`` always; ``club`` too when the format pools both levels, so the rating state's
    composition (pooled at the source) can be told from the training rows' composition."""
    present = set(format_frame.competition_level.dropna().unique())
    return ["all", "club"] if present >= set(LEVELS) else ["all"]


def score_frame(payload: Dict[str, Any], format_code: str) -> Dict[str, Any]:
    from ml.xi.evaluate import MIN_EVAL_ROWS, MIN_TRAIN_ROWS

    frame = payload["frame"]
    format_frame = frame[frame.format_code == format_code]
    objective = objective_arms()
    display = display_arms()
    out: Dict[str, Any] = {
        "taxonomy": payload["taxonomy"],
        "code_root": payload["code_root"],
        "feat05_filled": payload["feat05_filled"],
        "format": format_code,
        "n_rows": int(len(format_frame)),
        "rows_by_level": {k: int(v) for k, v in format_frame.competition_level.value_counts().items()},
        "training_subsets": training_subsets(format_frame),
        "windows": [],
    }
    pooled: Dict[str, Dict[str, List[np.ndarray]]] = {}
    for cutoff, end, kind in windows():
        train_all = format_frame[format_frame.match_date < cutoff]
        evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
        window: Dict[str, Any] = {
            "cutoff": cutoff.date().isoformat(),
            "end": end.date().isoformat() if kind == "fold" else None,
            "kind": kind,
            "n_train": int(len(train_all)),
            "n_eval": int(len(evaluation)),
            "n_eval_by_level": {k: int(v) for k, v in evaluation.competition_level.value_counts().items()},
            "arms": {},
        }
        if len(evaluation) < MIN_EVAL_ROWS or evaluation["team1_wins"].nunique() < 2:
            window["skipped_reason"] = "evaluation window too small or single-class"
            out["windows"].append(window)
            continue
        for train_on in out["training_subsets"]:
            train = train_all if train_on == "all" else train_all[train_all.competition_level == "club"]
            if len(train) < MIN_TRAIN_ROWS or train["team1_wins"].nunique() < 2:
                continue
            for name, (cols, grid) in objective.items():
                model, record = fit_objective_arm(train, cols, grid)
                key = f"{train_on}/objective/{name}"
                window["arms"][key] = _arm_entry(model, evaluation, cols, record, kind, key, pooled)
            for name, cols in display.items():
                model, record = fit_display_arm(train, cols)
                key = f"{train_on}/display/{name}"
                window["arms"][key] = _arm_entry(model, evaluation, cols, record, kind, key, pooled)
        out["windows"].append(window)
        logger.info("%s %s %s: %d arms scored", payload["taxonomy"], format_code, window["cutoff"], len(window["arms"]))
    out["summary"] = summarise(out["windows"], pooled)
    return out


def _arm_entry(
    model: object,
    evaluation: pd.DataFrame,
    cols: List[str],
    record: Dict,
    kind: str,
    key: str,
    pooled: Dict[str, Dict[str, List[np.ndarray]]],
) -> Dict[str, Any]:
    scores, predictions = score_subsets(model, evaluation, cols)
    if kind == "fold":
        for subset, values in predictions.items():
            pooled.setdefault(key, {}).setdefault(subset, []).append(values)
    return {"pick": record["params"], "reason": record["reason"], "scores": scores}


def summarise(window_reports: List[Dict], pooled: Dict[str, Dict[str, List[np.ndarray]]]) -> Dict[str, Any]:
    """Per arm and subset: the fold mean and sd (ddof=1, as ``ml.xi.folds``), the per-fold
    values, the pooled-over-folds AUC on the concatenated fold predictions, and the locked
    window's bare value."""
    summary: Dict[str, Any] = {}
    folds = [w for w in window_reports if w["kind"] == "fold" and "arms" in w and w["arms"]]
    locked = [w for w in window_reports if w["kind"] == "locked" and w.get("arms")]
    keys = sorted({k for w in folds + locked for k in w["arms"]})
    subsets = ["all", *LEVELS]
    for key in keys:
        summary[key] = {}
        for subset in subsets:
            values = [w["arms"][key]["scores"].get(subset, {}).get("auc") for w in folds if key in w["arms"]]
            briers = [w["arms"][key]["scores"].get(subset, {}).get("brier") for w in folds if key in w["arms"]]
            present = [v for v in values if v is not None]
            if not present and not pooled.get(key, {}).get(subset):
                continue
            entry: Dict[str, Any] = {
                "per_fold_auc": values,
                "mean": float(np.mean(present)) if present else None,
                "sd": float(np.std(present, ddof=1)) if len(present) > 1 else None,
                "n_folds": len(present),
                "brier_mean": float(np.mean([b for b in briers if b is not None])) if any(briers) else None,
            }
            stacked = pooled.get(key, {}).get(subset)
            if stacked:
                joined = np.vstack(stacked)
                if len(np.unique(joined[:, 0])) == 2:
                    entry["pooled_auc"] = float(roc_auc_score(joined[:, 0], joined[:, 1]))
                    entry["pooled_n"] = int(len(joined))
            if locked and key in locked[0]["arms"]:
                entry["locked"] = locked[0]["arms"][key]["scores"].get(subset)
            summary[key][subset] = entry
    return summary


def run_score(frames: Dict[str, str], formats: Sequence[str], out: str) -> None:
    report: Dict[str, Any] = {"generated_at": pd.Timestamp.utcnow().isoformat(), "frames": {}}
    for tag, path in frames.items():
        payload = load_frame(path)
        report["frames"][tag] = {fmt: score_frame(payload, fmt) for fmt in formats}
    with open(out, "w") as fh:
        json.dump(report, fh, indent=1)
    logger.info("score report written to %s", out)


# --- levels: per-format AUC by competition level on one frame -----------------------------


def score_levels(payload: Dict[str, Any], format_code: str) -> Dict[str, Any]:
    """Batch 4's recipes fitted on all training rows (``pooled``) and on each level's rows
    alone (``level_only``), scored on the window's rows per level; plus the best single
    display column per level over the development windows."""
    from ml.xi.evaluate import (
        LOCKED_START,
        MIN_EVAL_ROWS,
        MIN_TRAIN_ROWS,
        WALK_FORWARD_CUTOFFS,
    )

    frame = payload["frame"]
    format_frame = frame[frame.format_code == format_code]
    xi_cols, grid = objective_arms()["b4"]
    display_cols = display_arms()["b4"]
    out: Dict[str, Any] = {
        "format": format_code,
        "rows_by_level": {k: int(v) for k, v in format_frame.competition_level.value_counts().items()},
        "best_single_column": {},
        "windows": [],
    }
    development = format_frame[
        (format_frame.match_date >= pd.Timestamp(WALK_FORWARD_CUTOFFS[0]))
        & (format_frame.match_date < pd.Timestamp(LOCKED_START))
    ]
    for level, rows in development.groupby("competition_level"):
        out["best_single_column"][str(level)] = best_single_column(rows, display_cols)
    pooled: Dict[str, Dict[str, List[np.ndarray]]] = {}
    for cutoff, end, kind in windows():
        train_all = format_frame[format_frame.match_date < cutoff]
        evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
        window: Dict[str, Any] = {
            "cutoff": cutoff.date().isoformat(),
            "kind": kind,
            "n_train_by_level": {k: int(v) for k, v in train_all.competition_level.value_counts().items()},
            "n_eval_by_level": {k: int(v) for k, v in evaluation.competition_level.value_counts().items()},
            "arms": {},
        }
        if len(evaluation) < MIN_EVAL_ROWS or evaluation["team1_wins"].nunique() < 2:
            window["skipped_reason"] = "evaluation window too small or single-class"
            out["windows"].append(window)
            continue
        fits: List[Tuple[str, pd.DataFrame]] = [("pooled", train_all)]
        for level in LEVELS:
            fits.append((f"level_only/{level}", train_all[train_all.competition_level == level]))
        for arm, train in fits:
            if len(train) < MIN_TRAIN_ROWS or train["team1_wins"].nunique() < 2:
                continue
            model, record = fit_objective_arm(train, xi_cols, grid)
            key = f"{arm}/objective"
            window["arms"][key] = _arm_entry(model, evaluation, xi_cols, record, kind, key, pooled)
            model, record = fit_display_arm(train, display_cols)
            key = f"{arm}/display"
            window["arms"][key] = _arm_entry(model, evaluation, display_cols, record, kind, key, pooled)
        out["windows"].append(window)
        logger.info("levels %s %s: %d arms", format_code, window["cutoff"], len(window["arms"]))
    out["summary"] = summarise(out["windows"], pooled)
    return out


def best_single_column(rows: pd.DataFrame, cols: Sequence[str]) -> Dict[str, Any]:
    y = rows["team1_wins"].to_numpy(dtype=float)
    if len(rows) < MIN_SUBSET_ROWS or len(np.unique(y)) < 2:
        return {"n": int(len(rows))}
    aucs = {c: max(a, 1.0 - a) for c in cols for a in [roc_auc_score(y, rows[c].fillna(0.0))]}
    best = max(aucs, key=aucs.get)
    return {
        "column": best,
        "auc": float(aucs[best]),
        "n": int(len(rows)),
        "team_elo_diff": float(aucs["team_elo_diff"]),
    }


def run_levels(path: str, formats: Sequence[str], out: str) -> None:
    payload = load_frame(path)
    report = {
        "generated_at": pd.Timestamp.utcnow().isoformat(),
        "taxonomy": payload["taxonomy"],
        "code_root": payload["code_root"],
        "formats": {fmt: score_levels(payload, fmt) for fmt in formats},
    }
    with open(out, "w") as fh:
        json.dump(report, fh, indent=1)
    logger.info("levels report written to %s", out)


# --- decide: the tables ------------------------------------------------------------------


def _cell(summary: Dict, key: str, subset: str) -> Optional[Dict]:
    return summary.get(key, {}).get(subset)


def _fmt(entry: Optional[Dict]) -> str:
    if not entry or entry.get("mean") is None:
        return "—"
    pooled = entry.get("pooled_auc")
    pooled = float("nan") if pooled is None else pooled
    locked = (entry.get("locked") or {}).get("auc")
    return (
        f"{entry['mean']:.4f} ± {(entry['sd'] or 0.0):.4f} (n {entry['n_folds']}, pooled {pooled:.4f}"
        + (f", locked {locked:.3f}/{entry['locked']['n']}" if locked is not None else "")
        + ")"
    )


def paired(summary_a: Dict, key_a: str, summary_b: Dict, key_b: str, subset: str) -> Optional[Dict[str, float]]:
    """Per-fold paired difference b - a on the folds both produced, with its sd and
    standard error, so a difference is read against the noise of the same windows."""
    a, b = _cell(summary_a, key_a, subset), _cell(summary_b, key_b, subset)
    if not a or not b:
        return None
    pairs = [(x, y) for x, y in zip(a["per_fold_auc"], b["per_fold_auc"]) if x is not None and y is not None]
    if len(pairs) < 2:
        return None
    diffs = np.array([y - x for x, y in pairs])
    sd = float(np.std(diffs, ddof=1))
    return {
        "mean_diff": float(diffs.mean()),
        "sd": sd,
        "se": sd / np.sqrt(len(diffs)),
        "t": float(diffs.mean() / (sd / np.sqrt(len(diffs)))) if sd > 0 else None,
        "n_folds": len(diffs),
        "folds_down": int((diffs < 0).sum()),
    }


def _print_paired(label: str, comparison: Optional[Dict[str, float]]) -> None:
    if comparison is None:
        print(f"| {label} | — | | | |")
        return
    if comparison["t"] is None:
        print(f"| {label} | {comparison['mean_diff']:+.4f} | 0 | identical arms | 0/{comparison['n_folds']} |")
        return
    print(
        f"| {label} | {comparison['mean_diff']:+.4f} | {comparison['se']:.4f} | {comparison['t']:+.2f} "
        f"| {comparison['folds_down']}/{comparison['n_folds']} |"
    )


def decide_score(report: Dict, format_code: str) -> None:
    frames = report["frames"]
    print(f"\n## {format_code}: every cell, objective and display, by evaluation subset\n")
    print("| frame | training rows | arm | all rows | club rows | international rows |")
    print("|---|---|---|---|---|---|")
    for tag, per_format in frames.items():
        entry = per_format.get(format_code)
        if not entry:
            continue
        for key in sorted(entry["summary"]):
            train_on, surface, arm = key.split("/", 2)
            cells = [_fmt(_cell(entry["summary"], key, s)) for s in ("all", "club", "international")]
            print(f"| {tag} ({entry['taxonomy']}) | {train_on} | {surface}/{arm} | " + " | ".join(cells) + " |")
    print("\n### Paired over the same folds (b − a; positive means b higher)\n")
    print("| comparison | mean Δ | se | t | folds down |")
    print("|---|---|---|---|---|")
    pairs = [
        (
            "old code → new code, batch-3 taxonomy, b3 recipe, all rows",
            "b3code_old",
            "all/objective/b3",
            "b4code_old",
            "all/objective/b3",
            "all",
        ),
        (
            "old code → new code, archive taxonomy (club only), b3 recipe",
            "b3code_new",
            "all/objective/b3",
            "b4code_new",
            "all/objective/b3",
            "all",
        ),
        (
            "old code → new code, archive taxonomy (club only), b4 recipe",
            "b3code_new",
            "all/objective/b4",
            "b4code_new",
            "all/objective/b4",
            "all",
        ),
        (
            "b3 recipe → b4 recipe, new code, archive taxonomy (club only)",
            "b4code_new",
            "all/objective/b3",
            "b4code_new",
            "all/objective/b4",
            "all",
        ),
        (
            "b3 recipe → b4 recipe, old code, archive taxonomy (club only)",
            "b3code_new",
            "all/objective/b3",
            "b3code_new",
            "all/objective/b4",
            "all",
        ),
        (
            "b3 recipe → b4 recipe, new code, batch-3 taxonomy, club rows",
            "b4code_old",
            "all/objective/b3",
            "b4code_old",
            "all/objective/b4",
            "club",
        ),
        (
            "batch-3 taxonomy → archive taxonomy, old code, b3 recipe, club rows",
            "b3code_old",
            "all/objective/b3",
            "b3code_new",
            "all/objective/b3",
            "club",
        ),
        (
            "batch-3 taxonomy → archive taxonomy, new code, b4 recipe, club rows",
            "b4code_old",
            "all/objective/b4",
            "b4code_new",
            "all/objective/b4",
            "club",
        ),
        (
            "pooled → club-only training rows, old code, batch-3 taxonomy, b3 recipe, club rows",
            "b3code_old",
            "all/objective/b3",
            "b3code_old",
            "club/objective/b3",
            "club",
        ),
        (
            "pooled → club-only training rows, new code, batch-3 taxonomy, b4 recipe, club rows",
            "b4code_old",
            "all/objective/b4",
            "b4code_old",
            "club/objective/b4",
            "club",
        ),
        (
            "EVAL-13 columns 40 → 29 at fixed C 0.3, new code, archive",
            "b4code_new",
            "all/objective/b3",
            "b4code_new",
            "all/objective/cols29_fixed",
            "all",
        ),
        (
            "EVAL-13 grid on 29 columns (fixed → grid), new code, archive",
            "b4code_new",
            "all/objective/cols29_fixed",
            "b4code_new",
            "all/objective/b4",
            "all",
        ),
        (
            "EVAL-13 grid on 40 columns (fixed → grid), new code, archive",
            "b4code_new",
            "all/objective/b3",
            "b4code_new",
            "all/objective/cols40_grid",
            "all",
        ),
        (
            "recency axis (C-only grid → full grid), 29 columns, new code, archive",
            "b4code_new",
            "all/objective/cols29_c_grid_no_recency",
            "b4code_new",
            "all/objective/b4",
            "all",
        ),
        (
            "forced (C 0.1, 4y) vs fixed (C 0.3, none), 29 columns, new code, archive",
            "b4code_new",
            "all/objective/cols29_fixed",
            "b4code_new",
            "all/objective/cols29_forced_c0.1_hl4",
            "all",
        ),
        (
            "DISPLAY old code → new code, archive taxonomy, b3 display",
            "b3code_new",
            "all/display/b3",
            "b4code_new",
            "all/display/b3",
            "all",
        ),
        (
            "DISPLAY b3 → b4 columns, new code, archive taxonomy",
            "b4code_new",
            "all/display/b3",
            "b4code_new",
            "all/display/b4",
            "all",
        ),
        (
            "DISPLAY b3 → cols29+ctx7, new code, archive taxonomy",
            "b4code_new",
            "all/display/b3",
            "b4code_new",
            "all/display/cols29_ctx7",
            "all",
        ),
        (
            "DISPLAY old code → new code, batch-3 taxonomy, b3 display, all rows",
            "b3code_old",
            "all/display/b3",
            "b4code_old",
            "all/display/b3",
            "all",
        ),
        (
            "DISPLAY batch-3 → archive taxonomy, new code, b4 display, club rows",
            "b4code_old",
            "all/display/b4",
            "b4code_new",
            "all/display/b4",
            "club",
        ),
    ]
    for label, tag_a, key_a, tag_b, key_b, subset in pairs:
        a = frames.get(tag_a, {}).get(format_code, {}).get("summary", {})
        b = frames.get(tag_b, {}).get(format_code, {}).get("summary", {})
        _print_paired(label, paired(a, key_a, b, key_b, subset))
    print("\n### Grid picks per fold (b4 recipe, objective)\n")
    for tag, per_format in frames.items():
        entry = per_format.get(format_code)
        if not entry:
            continue
        picks = [w["arms"].get("all/objective/b4", {}).get("pick") for w in entry["windows"] if w.get("arms")]
        print(f"- {tag}: " + ", ".join(f"C {p['C']}/{p['half_life_years']}" for p in picks if p))


def decide_levels(report: Dict) -> None:
    print(f"\n## Per-format AUC by competition level ({report['taxonomy']} taxonomy)\n")
    print(
        "| format | level | rows | best single column (dev) | pooled objective | level-only objective | pooled display | level-only display |"
    )
    print("|---|---|---|---|---|---|---|---|")
    for fmt, entry in report["formats"].items():
        summary = entry["summary"]
        for level in ("all", *LEVELS):
            if level != "all" and level not in entry["rows_by_level"]:
                continue
            level_only = f"level_only/{level}"
            best = entry["best_single_column"].get(level, {})
            best_text = f"{best.get('column', '—')} {best['auc']:.3f} (n {best['n']})" if "auc" in best else "—"
            print(
                f"| {fmt} | {level} | {entry['rows_by_level'].get(level, sum(entry['rows_by_level'].values()))} | {best_text} "
                f"| {_fmt(_cell(summary, 'pooled/objective', level))} | {_fmt(_cell(summary, f'{level_only}/objective', level))} "
                f"| {_fmt(_cell(summary, 'pooled/display', level))} | {_fmt(_cell(summary, f'{level_only}/display', level))} |"
            )
    print("\n### Pooled vs level-only training, paired over folds on the level's own rows\n")
    print("| format / level / surface | mean Δ (level-only − pooled) | se | t | folds down |")
    print("|---|---|---|---|---|")
    for fmt, entry in report["formats"].items():
        for level in LEVELS:
            for surface in ("objective", "display"):
                comparison = paired(
                    entry["summary"], f"pooled/{surface}", entry["summary"], f"level_only/{level}/{surface}", level
                )
                _print_paired(f"{fmt} / {level} / {surface}", comparison)


# --- CLI ---------------------------------------------------------------------------------


def _parse_frames(values: Sequence[str]) -> Dict[str, str]:
    frames: Dict[str, str] = {}
    for value in values:
        tag, _, path = value.partition("=")
        if not path:
            raise argparse.ArgumentTypeError(f"--frames takes tag=path, got {value!r}")
        frames[tag] = path
    return frames


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--code-root", default=DEFAULT_CODE_ROOT, help="the ml-service directory whose code runs the pass")
    p.add_argument("--env-file", default=None, help=".env with POSTGRES_* for the database")
    sub = p.add_subparsers(dest="command", required=True)
    build = sub.add_parser("build")
    build.add_argument("--taxonomy", choices=("archive", "batch3"), required=True)
    build.add_argument("--out", required=True)
    score = sub.add_parser("score")
    score.add_argument("--frames", nargs="+", required=True, help="tag=path per frame pickle")
    score.add_argument("--formats", nargs="+", default=["T20"])
    score.add_argument("--out", required=True)
    levels = sub.add_parser("levels")
    levels.add_argument("--frames", required=True)
    levels.add_argument("--formats", nargs="+", default=["T20", "T20I", "ODI", "TEST"])
    levels.add_argument("--out", required=True)
    decide = sub.add_parser("decide")
    decide.add_argument("--score", nargs="*", default=[], help="score reports; several are merged by frame tag")
    decide.add_argument("--levels", nargs="*", default=[])
    decide.add_argument("--formats", nargs="+", default=["T20"])
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = parse_args(argv)
    sys.path.insert(0, os.path.abspath(args.code_root))
    load_environment(args.env_file)
    if args.command == "build":
        build_frame(args.taxonomy, args.code_root, args.out)
    elif args.command == "score":
        run_score(_parse_frames(args.frames), args.formats, args.out)
    elif args.command == "levels":
        run_levels(args.frames, args.formats, args.out)
    else:
        merged: Dict[str, Any] = {"frames": {}}
        for path in args.score:
            with open(path) as fh:
                merged["frames"].update(json.load(fh)["frames"])
        for fmt in args.formats if merged["frames"] else []:
            decide_score(merged, fmt)
        for path in args.levels:
            with open(path) as fh:
                decide_levels(json.load(fh))
    return 0


if __name__ == "__main__":
    sys.exit(main())
