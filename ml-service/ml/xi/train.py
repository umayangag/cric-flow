"""Train the XI-responsive win models and report held-out discrimination.

Usage (from ml-service/):
  python -m ml.xi.train --cricsheet-dir data/cricsheet --cutoff 2025-09-01
  python -m ml.xi.train --postgres --cutoff 2025-09-01

Per format two models are fitted on rows before the cutoff and scored on rows at or after it:

* objective  -- logistic regression on XI_FEATURE_COLS. Additive, so a hill-climb over XIs
                sees a smooth surface and a one-player upgrade never lowers the score
                (measured: <1% of upgrades move p by less than 0, vs 12% for unconstrained
                boosting). This is what the optimiser maximises.
* display    -- monotone-constrained gradient boosting on XI + team-context columns.
                Higher AUC; used for the probability shown to users.

The report records, per format: AUC and Brier for both models over several seeds, the
base-rate Brier, and the best single raw column's AUC -- a model that cannot beat its own
best column by a clear margin is not being measured (S-3c).
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
from datetime import date
from typing import Dict, List, Optional, Sequence

import numpy as np
import pandas as pd
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import brier_score_loss, roc_auc_score
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

from ml.xi import contract as C
from ml.xi import quality
from ml.xi.builder import BuildResult, build
from ml.xi.store import FormatModels, save_models, save_ratings

logger = logging.getLogger(__name__)

REPORT_NAME = "xi_win_report.json"


def make_objective_model() -> object:
    return make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=3000))


def make_display_model(columns: List[str], seed: int) -> object:
    return HistGradientBoostingClassifier(
        max_depth=3,
        learning_rate=0.04,
        max_iter=300,
        l2_regularization=1.0,
        min_samples_leaf=40,
        random_state=seed,
        monotonic_cst=C.monotone_directions(columns),
    )


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


def _score_marginalised(model, te: pd.DataFrame, cols: List[str]) -> Dict[str, float]:
    x_a, y = _xy(te, cols)
    x_b, _ = _xy(swap_orientation(te), cols)
    p = 0.5 * (model.predict_proba(x_a)[:, 1] + (1.0 - model.predict_proba(x_b)[:, 1]))
    return {"auc": float(roc_auc_score(y, p)), "brier": float(brier_score_loss(y, p))}


def gender_breakdown(objective, display_models: Sequence, te: pd.DataFrame) -> Dict[str, Dict[str, float]]:
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
            display_aucs = [_score_marginalised(m, rows, C.DISPLAY_FEATURE_COLS)["auc"] for m in display_models]
            entry["display_auc_mean"] = float(np.mean(display_aucs))
            entry["display_auc_sd"] = float(np.std(display_aucs))
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
    frame: pd.DataFrame, format_code: str, cutoff: pd.Timestamp, seeds: Sequence[int] = (0, 1, 2)
) -> tuple:
    """Fit both models for one format; return (FormatModels, report dict)."""
    d = frame[frame.format_code == format_code]
    tr, te = d[d.match_date < cutoff], d[d.match_date >= cutoff]
    report: Dict = {
        "format_code": format_code,
        "n_train": int(len(tr)),
        "n_holdout": int(len(te)),
        "seeds": list(seeds),
    }
    if len(tr) < 50 or tr[C.TARGET_COL].nunique() < 2:
        report["skipped_reason"] = "insufficient training rows"
        return None, report
    x_obj_tr, y_tr = _xy(tr, C.XI_FEATURE_COLS)
    x_dis_tr, _ = _xy(tr, C.DISPLAY_FEATURE_COLS)
    objective = make_objective_model().fit(x_obj_tr, y_tr)
    display_models = [make_display_model(C.DISPLAY_FEATURE_COLS, s).fit(x_dis_tr, y_tr) for s in seeds]
    if len(te) >= 20 and te[C.TARGET_COL].nunique() == 2:
        x_obj_te, y_te = _xy(te, C.XI_FEATURE_COLS)
        x_dis_te, _ = _xy(te, C.DISPLAY_FEATURE_COLS)
        obj = _score(objective, x_obj_te, y_te)
        dis = [_score(m, x_dis_te, y_te) for m in display_models]
        report.update(
            {
                "holdout_positive_rate": float(y_te.mean()),
                "objective": obj,
                "objective_marginalised": _score_marginalised(objective, te, C.XI_FEATURE_COLS),
                "display_marginalised": _score_marginalised(display_models[0], te, C.DISPLAY_FEATURE_COLS),
                "display": {
                    "auc_mean": float(np.mean([r["auc"] for r in dis])),
                    "auc_sd": float(np.std([r["auc"] for r in dis])),
                    "brier_mean": float(np.mean([r["brier"] for r in dis])),
                },
                "base_rate_brier": float(brier_score_loss(y_te, np.full(len(y_te), y_tr.mean()))),
                "best_single_column": best_single_column(te, C.DISPLAY_FEATURE_COLS),
                "by_gender": gender_breakdown(objective, display_models, te),
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
        "report": report,
    }
    models = FormatModels(
        format_code=format_code,
        objective=objective,
        display=display_models[0],
        objective_cols=list(C.XI_FEATURE_COLS),
        display_cols=list(C.DISPLAY_FEATURE_COLS),
        metadata=metadata,
    )
    return models, report


def train_all(
    result: BuildResult,
    artifacts_dir: str,
    cutoff: pd.Timestamp,
    formats: Sequence[str] = C.FORMAT_CODES,
) -> Dict:
    os.makedirs(artifacts_dir, exist_ok=True)
    reports = []
    for fmt in formats:
        models, report = train_format(result.frame, fmt, cutoff)
        reports.append(report)
        if models is not None:
            path = save_models(models, artifacts_dir)
            logger.info(
                "%s: objective AUC %s, display AUC %s -> %s",
                fmt,
                report.get("objective", {}).get("auc"),
                report.get("display", {}).get("auc_mean"),
                path,
            )
        else:
            logger.warning("%s: skipped (%s)", fmt, report.get("skipped_reason"))
    save_ratings(result.state, artifacts_dir)
    # Read before anything is written: the baseline is the last accepted run's counts.
    gate = quality.check(result.quality, quality.load_baseline(artifacts_dir))
    summary = {
        "cutoff": cutoff.date().isoformat(),
        "n_rows": int(len(result.frame)),
        "n_undecided": result.n_undecided,
        quality.REPORT_KEY: result.quality.as_dict(),
        "data_quality_failures": gate.failures,
        "formats": reports,
    }
    with open(os.path.join(artifacts_dir, REPORT_NAME), "w") as fh:
        json.dump(summary, fh, indent=2)
    return summary


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


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    src = p.add_mutually_exclusive_group(required=True)
    src.add_argument("--cricsheet-dir", help="directory of Cricsheet JSON files")
    src.add_argument("--postgres", action="store_true", help="read the go-app database (POSTGRES_* env vars)")
    p.add_argument("--cutoff", required=True, help="YYYY-MM-DD; rows before it train, rows at/after it are the holdout")
    p.add_argument("--out", default=None, help="artifacts directory (default: ml.config.default_artifacts_dir())")
    p.add_argument("--formats", nargs="+", default=list(C.FORMAT_CODES))
    p.add_argument("--frame-out", default=None, help="optional path to also write the training frame as CSV")
    p.add_argument(
        "--accept-data-quality",
        action="store_true",
        help="record this run's data-quality counts as the baseline even if the gate failed (H-15)",
    )
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    cutoff = pd.Timestamp(date.fromisoformat(args.cutoff))
    if args.cricsheet_dir:
        from ml.xi.sources import CricsheetJsonSource

        source = CricsheetJsonSource(args.cricsheet_dir, _international_teams_from_config(), args.formats)
    else:
        from ml.db import get_db_connection
        from ml.xi.sources import PostgresSource

        source = PostgresSource(get_db_connection(), args.formats)
    out_dir = args.out
    if out_dir is None:
        from ml.config import default_artifacts_dir

        out_dir = default_artifacts_dir()
    result = build(source, progress=lambda i: logger.info("rating pass: %d matches", i))
    if args.frame_out:
        result.frame.to_csv(args.frame_out, index=False)
    summary = train_all(result, out_dir, cutoff, args.formats)
    for r in summary["formats"]:
        if "objective" in r:
            logger.info(
                "%-5s n_tr=%5d n_te=%4d | objective AUC %.3f (serving, toss unknown: %.3f) | display AUC %.3f±%.3f "
                "(serving: %.3f) Brier %.3f | base Brier %.3f | best column %s %.3f",
                r["format_code"], r["n_train"], r["n_holdout"], r["objective"]["auc"], r["objective_marginalised"]["auc"],
                r["display"]["auc_mean"], r["display"]["auc_sd"], r["display_marginalised"]["auc"],
                r["display_marginalised"]["brier"], r["base_rate_brier"],
                r["best_single_column"]["column"], r["best_single_column"]["auc"],
            )  # fmt: skip
    failures = summary["data_quality_failures"]
    if failures and not args.accept_data_quality:
        # The artifacts and the report are written either way: an operator has to see what
        # the run produced in order to judge whether the new counts are right. What a
        # failure withholds is the baseline, so re-running cannot clear the gate on its own.
        for failure in failures:
            logger.error("data-quality gate: %s", failure)
        logger.error(
            "data-quality gate failed (%d %s). Artifacts and %s are written but the baseline is "
            "unchanged, so a re-run will fail the same way. Review the counts; if they are right, "
            "re-run with --accept-data-quality.",
            len(failures),
            "check" if len(failures) == 1 else "checks",
            REPORT_NAME,
        )
        return 1
    if failures:
        logger.warning("data-quality gate failed but --accept-data-quality was given; recording the new baseline")
    path = quality.save_baseline(out_dir, result.quality)
    logger.info("data-quality gate passed; baseline at %s", path)
    return 0


if __name__ == "__main__":
    sys.exit(main())
