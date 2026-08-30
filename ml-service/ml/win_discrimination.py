"""Held-out discrimination report for the win model.

The optimiser in ``ml.team_optimizer`` maximises the win model's output over candidate
XIs. That search is only as good as the model's *ranking* of teams, so before tuning the
search it is worth knowing whether the model ranks anything at all.

This module answers that question on matches the model never trained on: AUC, Brier
score and a reliability curve, per format.

**Calibration is deliberately not the headline.** Selection takes an argmax, and no
monotone recalibration can change which XI ranks highest, so a poorly calibrated model
can still select perfectly. The reliability curve is here for the probability we
*display*, and Brier is reported because it decomposes into both. AUC is the number that
gates the search work.

Usage:
  GO_APP_URL=http://localhost:8080 python -m ml.win_discrimination \
      --train-cutoff 2024-01-01T00:00:00Z
"""

from __future__ import annotations

import argparse
import json
import logging
import os
from dataclasses import asdict, dataclass, field
from typing import Any, Dict, List, Optional, Sequence

import joblib
import numpy as np
import pandas as pd

from ml.config import default_artifacts_dir
from ml.train_win import build_win_feature_frame, fetch_win_data
from ml.win_features import WIN_TARGET_COL

logger = logging.getLogger(__name__)

# Reliability curve resolution. Ten buckets over [0,1] is enough to see a curve bend
# without splitting a few hundred holdout matches into empty bins.
DEFAULT_RELIABILITY_BINS = 10


@dataclass
class ReliabilityBin:
    """One bucket of the reliability curve."""

    lower: float
    upper: float
    count: int
    mean_predicted: Optional[float]
    observed_rate: Optional[float]


@dataclass
class FormatReport:
    """What the holdout says about one format's win model.

    ``skipped_reason`` is populated instead of the metrics whenever the question could
    not be asked -- no artifact, no holdout rows, one class. Those are findings, not
    errors, and a report that omitted them would read as though the format were fine.
    """

    format_code: str
    matches: int = 0
    positives: int = 0
    positive_rate: Optional[float] = None
    auc: Optional[float] = None
    brier: Optional[float] = None
    mean_predicted: Optional[float] = None
    model_classes: Optional[List[float]] = None
    reliability: List[ReliabilityBin] = field(default_factory=list)
    skipped_reason: Optional[str] = None
    warnings: List[str] = field(default_factory=list)


def date_column_index(headers: Sequence[str]) -> int:
    """Return the index of match_date, or -1 when the export does not carry it."""
    try:
        return list(headers).index("match_date")
    except ValueError:
        return -1


def select_holdout_rows(
    headers: Sequence[str],
    rows: Sequence[Sequence[str]],
    train_cutoff: str,
) -> List[Sequence[str]]:
    """Return the rows at or after ``train_cutoff``.

    The training-data API returns everything before the cutoff it is given, so the
    holdout is carved out here rather than requested. Rows whose date cannot be parsed
    are dropped rather than guessed at: a row of unknown vintage cannot be shown to be
    out of sample, and quietly counting it would defeat the point of the report.
    """
    index = date_column_index(headers)
    if index < 0:
        return []
    boundary = pd.to_datetime(train_cutoff, utc=True, errors="coerce")
    if pd.isna(boundary):
        raise ValueError(f"unparseable train cutoff: {train_cutoff!r}")

    # The export writes ISO dates; naming the format keeps pandas from guessing per row.
    dates = pd.to_datetime([row[index] for row in rows], utc=True, errors="coerce", format="ISO8601")
    return [row for row, date in zip(rows, dates) if not pd.isna(date) and date >= boundary]


def positive_class_probability(model: Any, X: np.ndarray) -> np.ndarray:
    """Return P(team1 wins) for each row.

    A model fitted on a single-class target -- which is what the misaligned win export
    produced before the column contract was repaired -- yields a one-column
    ``predict_proba``. That is handled explicitly so the report can say so, rather than
    raising an index error three frames away from the cause.
    """
    proba = model.predict_proba(X)
    if proba.ndim == 2 and proba.shape[1] > 1:
        return proba[:, 1]
    flat = proba.ravel()
    classes = list(getattr(model, "classes_", []))
    if classes and float(classes[0]) == 1.0:
        return flat
    return 1.0 - flat


def reliability_curve(
    y_true: np.ndarray,
    y_prob: np.ndarray,
    bins: int = DEFAULT_RELIABILITY_BINS,
) -> List[ReliabilityBin]:
    """Bucket predictions and report observed frequency against mean prediction."""
    edges = np.linspace(0.0, 1.0, bins + 1)
    out: List[ReliabilityBin] = []
    for i in range(bins):
        lower, upper = float(edges[i]), float(edges[i + 1])
        # The last bucket owns 1.0; every other is half-open, so no prediction is counted twice.
        in_bin = (y_prob >= lower) & (y_prob < upper) if i < bins - 1 else (y_prob >= lower) & (y_prob <= upper)
        count = int(in_bin.sum())
        out.append(
            ReliabilityBin(
                lower=lower,
                upper=upper,
                count=count,
                mean_predicted=float(y_prob[in_bin].mean()) if count else None,
                observed_rate=float(y_true[in_bin].mean()) if count else None,
            )
        )
    return out


def evaluate_predictions(
    format_code: str,
    y_true: np.ndarray,
    y_prob: np.ndarray,
    model_classes: Optional[List[float]] = None,
    bins: int = DEFAULT_RELIABILITY_BINS,
) -> FormatReport:
    """Score one format's holdout predictions."""
    from sklearn.metrics import brier_score_loss, roc_auc_score

    report = FormatReport(
        format_code=format_code,
        matches=int(y_true.shape[0]),
        positives=int(y_true.sum()),
        model_classes=model_classes,
    )
    report.positive_rate = float(y_true.mean())
    report.mean_predicted = float(y_prob.mean())
    report.brier = float(brier_score_loss(y_true, y_prob))
    report.reliability = reliability_curve(y_true, y_prob, bins=bins)

    # AUC needs both outcomes present; a one-sided holdout is a property of the window,
    # not a failure, so it is named rather than raised.
    if len(np.unique(y_true)) < 2:
        report.warnings.append("holdout contains a single outcome, so AUC is undefined; widen the window")
        return report
    report.auc = float(roc_auc_score(y_true, y_prob))

    if len(np.unique(y_prob)) == 1:
        report.warnings.append("the model returns one constant probability, so it ranks no team above another")
    return report


def load_win_model(artifacts_dir: str, format_code: str) -> Optional[Any]:
    """Load win_model_<FMT>.joblib, or None when it has not been trained."""
    path = os.path.join(artifacts_dir, f"win_model_{format_code}.joblib")
    if not os.path.isfile(path):
        return None
    return joblib.load(path)


def load_model_feature_cols(artifacts_dir: str, format_code: str) -> Optional[List[str]]:
    """Return the feature columns the model was trained on, from its metadata sidecar.

    Evaluation must use the model's own column list, in its own order. Re-deriving the
    list from the holdout would put it through the training run's variance filter a
    second time, on different data, and quietly score a different matrix than the one
    the model expects.
    """
    path = os.path.join(artifacts_dir, f"win_model_{format_code}_metadata.json")
    if not os.path.isfile(path):
        return None
    with open(path) as f:
        metadata = json.load(f)
    cols = metadata.get("feature_cols")
    return [str(c) for c in cols] if cols else None


def run_report(
    headers: Sequence[str],
    rows: Sequence[Sequence[str]],
    train_cutoff: str,
    artifacts_dir: str,
    bins: int = DEFAULT_RELIABILITY_BINS,
) -> List[FormatReport]:
    """Build a per-format report from raw export rows.

    Never returns an empty list: a window with nothing to measure is a finding, and a
    report of no rows would otherwise render as a report of nothing wrong.
    """
    holdout = select_holdout_rows(headers, rows, train_cutoff)
    if not holdout:
        return [FormatReport(format_code="ALL", skipped_reason="no matches on or after the train cutoff")]

    # The trainer's own frame builder, so the features are constructed the same way.
    # Column *selection* comes from the model's metadata, not from this data.
    frame, _feature_cols = build_win_feature_frame(list(headers), [list(r) for r in holdout])
    if frame.empty:
        return [
            FormatReport(
                format_code="ALL",
                matches=len(holdout),
                skipped_reason=f"{len(holdout)} holdout matches produced no usable feature rows",
            )
        ]

    if "format_code" in frame.columns:
        groups = [(str(fmt).strip().upper() or "_ALL_", g) for fmt, g in frame.groupby("format_code")]
    else:
        groups = [("_ALL_", frame)]

    reports: List[FormatReport] = []
    for format_code, group in sorted(groups, key=lambda pair: pair[0]):
        reports.append(_evaluate_group(format_code, group, artifacts_dir, bins))
    return reports


def _evaluate_group(
    format_code: str,
    group: pd.DataFrame,
    artifacts_dir: str,
    bins: int,
) -> FormatReport:
    """Score one format's holdout rows, or say why it could not be scored."""
    rows_available = int(group.shape[0])

    model = load_win_model(artifacts_dir, format_code)
    if model is None:
        return FormatReport(
            format_code=format_code,
            matches=rows_available,
            skipped_reason=f"no win_model_{format_code}.joblib in {artifacts_dir}",
        )

    feature_cols = load_model_feature_cols(artifacts_dir, format_code)
    if not feature_cols:
        return FormatReport(
            format_code=format_code,
            matches=rows_available,
            skipped_reason=(
                f"no win_model_{format_code}_metadata.json, so the columns it was trained on are unknown; "
                "re-train to write the sidecar"
            ),
        )

    missing = [c for c in feature_cols if c not in group.columns]
    if missing:
        return FormatReport(
            format_code=format_code,
            matches=rows_available,
            skipped_reason=(
                f"the export is missing {len(missing)} column(s) the model was trained on "
                f"(first: {missing[0]}); re-export and re-train against one contract"
            ),
        )

    scored = group.dropna(subset=[WIN_TARGET_COL])
    if scored.empty:
        return FormatReport(
            format_code=format_code,
            matches=rows_available,
            skipped_reason="no holdout row carries an outcome",
        )

    X = scored[feature_cols].astype(float).values
    expected = getattr(model, "n_features_in_", None)
    if expected is not None and int(expected) != X.shape[1]:
        return FormatReport(
            format_code=format_code,
            matches=int(scored.shape[0]),
            skipped_reason=(
                f"model expects {int(expected)} features, its metadata names {X.shape[1]}; "
                "the artifact and its sidecar disagree, so re-train"
            ),
        )

    y_prob = positive_class_probability(model, X)
    return evaluate_predictions(
        format_code,
        np.asarray(scored[WIN_TARGET_COL].astype(float).values, dtype=float),
        np.asarray(y_prob, dtype=float),
        model_classes=[float(c) for c in getattr(model, "classes_", [])],
        bins=bins,
    )


def format_summary(reports: Sequence[FormatReport]) -> str:
    """Render the reports as a table for the log."""
    lines = [f"{'format':<8}{'matches':>9}{'pos rate':>10}{'AUC':>8}{'Brier':>8}  notes"]
    for r in reports:
        if r.skipped_reason:
            lines.append(f"{r.format_code:<8}{r.matches:>9}{'-':>10}{'-':>8}{'-':>8}  {r.skipped_reason}")
            continue
        auc = f"{r.auc:.3f}" if r.auc is not None else "n/a"
        lines.append(
            f"{r.format_code:<8}{r.matches:>9}{r.positive_rate:>10.3f}{auc:>8}{r.brier:>8.3f}  " + "; ".join(r.warnings)
        )
    return "\n".join(lines)


def _reports_to_json(reports: Sequence[FormatReport], train_cutoff: str, eval_cutoff: str) -> Dict[str, Any]:
    return {
        "train_cutoff": train_cutoff,
        "eval_cutoff": eval_cutoff,
        "formats": [asdict(r) for r in reports],
    }


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Held-out discrimination report for the win model")
    parser.add_argument(
        "--train-cutoff",
        required=True,
        help="RFC3339 cutoff the win model was trained to; matches on or after it form the holdout",
    )
    parser.add_argument(
        "--eval-cutoff",
        default="",
        help="RFC3339 upper bound of the holdout window (default: now)",
    )
    parser.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""), help="Go-app base URL")
    parser.add_argument("--artifacts-dir", default="", help="Directory holding win_model_<FMT>.joblib")
    parser.add_argument("--bins", type=int, default=DEFAULT_RELIABILITY_BINS, help="Reliability curve buckets")
    parser.add_argument(
        "--out", default="", help="Write the JSON report here (default: <artifacts-dir>/win_discrimination.json)"
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    args = _parse_args(argv)
    if not args.go_app_url:
        logger.error("win_discrimination.no_go_app_url set GO_APP_URL or pass --go-app-url")
        return 2

    eval_cutoff = args.eval_cutoff or pd.Timestamp.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
    artifacts_dir = args.artifacts_dir or default_artifacts_dir()

    data = fetch_win_data(args.go_app_url, eval_cutoff, os.environ.get("GO_APP_API_KEY"))
    headers, rows = data.get("headers") or [], data.get("rows") or []
    if not rows:
        logger.error("win_discrimination.no_rows cutoff=%s", eval_cutoff)
        return 1

    reports = run_report(headers, rows, args.train_cutoff, artifacts_dir, bins=args.bins)
    logger.info(
        "win_discrimination.report train_cutoff=%s eval_cutoff=%s\n%s",
        args.train_cutoff,
        eval_cutoff,
        format_summary(reports),
    )

    out_path = args.out or os.path.join(artifacts_dir, "win_discrimination.json")
    os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
    with open(out_path, "w") as f:
        json.dump(_reports_to_json(reports, args.train_cutoff, eval_cutoff), f, indent=2)
    logger.info("win_discrimination.written path=%s", out_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
