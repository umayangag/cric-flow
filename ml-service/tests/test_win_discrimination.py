"""Tests for ml.win_discrimination, the win model's held-out report."""

import json
from typing import List, Optional

import joblib
import numpy as np
import pytest

from ml.win_discrimination import (
    FormatReport,
    evaluate_predictions,
    positive_class_probability,
    reliability_curve,
    run_report,
    select_holdout_rows,
)
from ml.win_features import MATCH_CONTEXT_BASE_COLS, WIN_TARGET_COL


class StubModel:
    """A win model that returns fixed probabilities.

    ``classes`` of length one reproduces a model fitted on a constant target, which is
    what the misaligned win export used to produce.
    """

    def __init__(
        self,
        probabilities: List[float],
        classes: Optional[List[float]] = None,
        n_features: int = 0,
    ) -> None:
        self._probabilities = probabilities
        self.classes_ = np.array(classes if classes is not None else [0.0, 1.0])
        if n_features:
            self.n_features_in_ = n_features

    def predict_proba(self, X: np.ndarray) -> np.ndarray:
        probs = np.array(self._probabilities[: X.shape[0]], dtype=float)
        if len(self.classes_) == 1:
            return probs.reshape(-1, 1)
        return np.column_stack([1.0 - probs, probs])


def _export_headers() -> List[str]:
    """Headers shaped like the win export: context, target, grouping, distribution stats."""
    from ml.win_features import _DIST_FEATURE_COLS  # noqa: PLC2701 - mirrors the real export

    return list(MATCH_CONTEXT_BASE_COLS) + [WIN_TARGET_COL, "format_code", "match_date"] + list(_DIST_FEATURE_COLS)


def _export_row(headers: List[str], match_date: str, team1_wins: int, fmt: str = "T20") -> List[str]:
    values = []
    for h in headers:
        if h == "match_date":
            values.append(match_date)
        elif h == "format_code":
            values.append(fmt)
        elif h == WIN_TARGET_COL:
            values.append(str(team1_wins))
        else:
            values.append("1.0")
    return values


def _holdout_rows(headers: List[str], count: int = 12, fmt: str = "T20") -> List[List[str]]:
    """A window of dated rows with alternating outcomes."""
    return [_export_row(headers, f"2024-03-{day:02d}", day % 2, fmt=fmt) for day in range(1, count + 1)]


def _write_model(
    directory,
    feature_cols: List[str],
    probabilities: List[float],
    fmt: str = "T20",
    n_features: Optional[int] = None,
) -> None:
    """Write a win artifact and the metadata sidecar that names the columns it was trained on."""
    joblib.dump(
        StubModel(probabilities, n_features=n_features if n_features is not None else len(feature_cols)),
        directory / f"win_model_{fmt}.joblib",
    )
    with open(directory / f"win_model_{fmt}_metadata.json", "w") as f:
        json.dump({"format_code": fmt, "feature_cols": feature_cols}, f)


# --- holdout selection -------------------------------------------------------


def test_select_holdout_rows_keeps_only_matches_on_or_after_the_cutoff() -> None:
    """The holdout is carved out of the export, not requested from the API."""
    headers = ["match_date", "team1_wins"]
    rows = [["2023-06-01", "1"], ["2024-01-01", "0"], ["2024-07-15", "1"]]

    holdout = select_holdout_rows(headers, rows, "2024-01-01T00:00:00Z")

    assert [r[0] for r in holdout] == ["2024-01-01", "2024-07-15"]


def test_select_holdout_rows_drops_undated_rows() -> None:
    """A row of unknown vintage cannot be shown to be out of sample."""
    headers = ["match_date", "team1_wins"]
    rows = [["not-a-date", "1"], ["2024-07-15", "1"]]

    holdout = select_holdout_rows(headers, rows, "2024-01-01T00:00:00Z")

    assert [r[0] for r in holdout] == ["2024-07-15"]


def test_select_holdout_rows_without_a_date_column_returns_nothing() -> None:
    """No date column means no way to prove a row is held out."""
    assert select_holdout_rows(["team1_wins"], [["1"]], "2024-01-01T00:00:00Z") == []


def test_select_holdout_rows_rejects_an_unparseable_cutoff() -> None:
    """A silently ignored cutoff would report in-sample rows as held out."""
    with pytest.raises(ValueError, match="unparseable train cutoff"):
        select_holdout_rows(["match_date"], [["2024-01-01"]], "whenever")


# --- probability extraction and metrics --------------------------------------


def test_positive_class_probability_reads_the_second_column() -> None:
    """A two-class model exposes P(team1 wins) as the positive column."""
    probs = positive_class_probability(StubModel([0.25, 0.75]), np.zeros((2, 3)))

    assert probs.tolist() == [0.25, 0.75]


def test_positive_class_probability_handles_a_single_class_model() -> None:
    """A model fitted on a constant target returns one column; it must not index-error."""
    probs = positive_class_probability(StubModel([0.9, 0.9], classes=[0.0]), np.zeros((2, 3)))

    # classes_ == [0] means the single column is P(team1 loses).
    assert probs.tolist() == [pytest.approx(0.1), pytest.approx(0.1)]


def test_reliability_curve_buckets_predictions_and_counts_outcomes() -> None:
    """Each bucket reports how often the predicted band actually happened."""
    curve = reliability_curve(np.array([0.0, 0.0, 1.0, 1.0]), np.array([0.05, 0.15, 0.85, 0.95]), bins=2)

    assert [b.count for b in curve] == [2, 2]
    assert curve[0].observed_rate == 0.0
    assert curve[1].observed_rate == 1.0


def test_reliability_curve_last_bucket_includes_one() -> None:
    """A prediction of exactly 1.0 must land somewhere."""
    curve = reliability_curve(np.array([1.0]), np.array([1.0]), bins=4)

    assert sum(b.count for b in curve) == 1


def test_evaluate_predictions_scores_a_discriminating_model() -> None:
    """A model that ranks winners above losers scores an AUC of 1."""
    report = evaluate_predictions("T20", np.array([0.0, 0.0, 1.0, 1.0]), np.array([0.1, 0.2, 0.8, 0.9]))

    assert report.auc == pytest.approx(1.0)
    assert report.matches == 4
    assert report.positive_rate == pytest.approx(0.5)
    assert report.warnings == []


def test_evaluate_predictions_names_a_one_sided_holdout() -> None:
    """AUC is undefined when every match went the same way; say so, do not raise."""
    report = evaluate_predictions("T20", np.array([1.0, 1.0]), np.array([0.6, 0.7]))

    assert report.auc is None
    assert any("single outcome" in w for w in report.warnings)


def test_evaluate_predictions_names_a_constant_model() -> None:
    """A model that ranks nothing is the finding the search work waits on."""
    report = evaluate_predictions("T20", np.array([0.0, 1.0]), np.array([0.5, 0.5]))

    assert any("constant probability" in w for w in report.warnings)


# --- the report --------------------------------------------------------------


def test_run_report_scores_a_matching_model(tmp_path) -> None:
    """The happy path: a model whose sidecar names present columns is scored."""
    headers = _export_headers()
    rows = _holdout_rows(headers)
    outcomes = [int(r[headers.index(WIN_TARGET_COL)]) for r in rows]
    # Predict each outcome exactly, so a working report must show a perfect ranking.
    _write_model(
        tmp_path,
        ["team1_bat_form_sum", "team2_bat_form_sum"],
        [0.9 if o == 1 else 0.1 for o in outcomes],
    )

    reports = run_report(headers, rows, "2024-01-01T00:00:00Z", str(tmp_path))

    assert len(reports) == 1
    assert reports[0].skipped_reason is None
    assert reports[0].matches == len(rows)
    assert reports[0].auc == pytest.approx(1.0)


def test_run_report_evaluates_a_small_window(tmp_path) -> None:
    """A handful of matches is still a report; the trainer's min-row floor is not applied here."""
    headers = _export_headers()
    rows = [_export_row(headers, "2024-03-01", 1), _export_row(headers, "2024-04-01", 0)]
    _write_model(tmp_path, ["team1_bat_form_sum", "team2_bat_form_sum"], [0.9, 0.1])

    reports = run_report(headers, rows, "2024-01-01T00:00:00Z", str(tmp_path))

    assert len(reports) == 1
    assert reports[0].skipped_reason is None
    assert reports[0].matches == 2


def test_run_report_reports_no_holdout_when_the_window_is_empty() -> None:
    """A cutoff after every match yields nothing to measure."""
    headers = _export_headers()
    rows = [_export_row(headers, "2023-03-01", 1)]

    reports = run_report(headers, rows, "2024-01-01T00:00:00Z", "/nonexistent")

    assert reports == [FormatReport(format_code="ALL", skipped_reason="no matches on or after the train cutoff")]


def test_run_report_skips_a_format_with_no_artifact(tmp_path) -> None:
    """A missing model is a finding, reported per format rather than raised."""
    headers = _export_headers()
    rows = _holdout_rows(headers)

    reports = run_report(headers, rows, "2024-01-01T00:00:00Z", str(tmp_path))

    assert len(reports) == 1
    assert reports[0].format_code == "T20"
    assert "no win_model_T20.joblib" in (reports[0].skipped_reason or "")


def test_run_report_skips_a_model_with_no_metadata(tmp_path) -> None:
    """Without the sidecar there is no way to know which columns the model expects."""
    headers = _export_headers()
    rows = _holdout_rows(headers)
    joblib.dump(StubModel([0.5] * len(rows)), tmp_path / "win_model_T20.joblib")

    reports = run_report(headers, rows, "2024-01-01T00:00:00Z", str(tmp_path))

    assert "columns it was trained on are unknown" in (reports[0].skipped_reason or "")


def test_run_report_skips_when_the_export_lacks_a_trained_column(tmp_path) -> None:
    """A model trained on a column this export no longer carries must not be scored."""
    headers = _export_headers()
    rows = _holdout_rows(headers)
    _write_model(tmp_path, ["team1_bat_form_sum", "a_column_that_was_removed"], [0.5] * len(rows))

    reports = run_report(headers, rows, "2024-01-01T00:00:00Z", str(tmp_path))

    assert "missing 1 column(s)" in (reports[0].skipped_reason or "")


def test_run_report_refuses_when_artifact_and_sidecar_disagree(tmp_path) -> None:
    """An artifact expecting a different width than its own sidecar names is not scored."""
    headers = _export_headers()
    rows = _holdout_rows(headers)
    _write_model(tmp_path, ["team1_bat_form_sum", "team2_bat_form_sum"], [0.5] * len(rows), n_features=3)

    reports = run_report(headers, rows, "2024-01-01T00:00:00Z", str(tmp_path))

    assert "the artifact and its sidecar disagree" in (reports[0].skipped_reason or "")


# --- operator-facing output --------------------------------------------------


def test_format_summary_renders_scored_and_skipped_formats() -> None:
    """The log table must show a skipped format, not quietly omit it."""
    from ml.win_discrimination import format_summary

    scored = evaluate_predictions("T20", np.array([0.0, 1.0]), np.array([0.2, 0.8]))
    skipped = FormatReport(format_code="TEST", matches=5, skipped_reason="no win_model_TEST.joblib")

    table = format_summary([scored, skipped])

    assert "T20" in table
    assert "TEST" in table
    assert "no win_model_TEST.joblib" in table


def test_reports_to_json_carries_the_window_and_every_format() -> None:
    """The written report must say which window produced it."""
    from ml.win_discrimination import _reports_to_json  # noqa: PLC2701 - output shape is behaviour

    payload = _reports_to_json(
        [FormatReport(format_code="T20", matches=3)],
        "2024-01-01T00:00:00Z",
        "2024-09-01T00:00:00Z",
    )

    assert payload["train_cutoff"] == "2024-01-01T00:00:00Z"
    assert payload["eval_cutoff"] == "2024-09-01T00:00:00Z"
    assert [f["format_code"] for f in payload["formats"]] == ["T20"]


# --- CLI ---------------------------------------------------------------------


def test_main_refuses_without_a_go_app_url(monkeypatch) -> None:
    """The report is useless without somewhere to fetch matches from; say so, do not traceback."""
    from ml import win_discrimination

    monkeypatch.delenv("GO_APP_URL", raising=False)

    assert win_discrimination.main(["--train-cutoff", "2024-01-01T00:00:00Z"]) == 2


def test_main_reports_when_the_export_is_empty(monkeypatch) -> None:
    """An export with no rows is an operator problem, not a crash."""
    from ml import win_discrimination

    monkeypatch.setattr(win_discrimination, "fetch_win_data", lambda *a, **k: {"headers": [], "rows": []})

    exit_code = win_discrimination.main(["--train-cutoff", "2024-01-01T00:00:00Z", "--go-app-url", "http://go-app"])

    assert exit_code == 1


def test_main_writes_a_report_naming_the_window(tmp_path, monkeypatch) -> None:
    """The written report must record which window produced it, and every format in it."""
    from ml import win_discrimination

    headers = _export_headers()
    rows = _holdout_rows(headers)
    _write_model(tmp_path, ["team1_bat_form_sum", "team2_bat_form_sum"], [0.5] * len(rows))
    monkeypatch.setattr(win_discrimination, "fetch_win_data", lambda *a, **k: {"headers": headers, "rows": rows})
    out_path = tmp_path / "report.json"

    exit_code = win_discrimination.main(
        [
            "--train-cutoff",
            "2024-01-01T00:00:00Z",
            "--eval-cutoff",
            "2024-09-01T00:00:00Z",
            "--go-app-url",
            "http://go-app",
            "--artifacts-dir",
            str(tmp_path),
            "--out",
            str(out_path),
        ]
    )

    assert exit_code == 0
    written = json.loads(out_path.read_text())
    assert written["train_cutoff"] == "2024-01-01T00:00:00Z"
    assert written["eval_cutoff"] == "2024-09-01T00:00:00Z"
    assert [f["format_code"] for f in written["formats"]] == ["T20"]
