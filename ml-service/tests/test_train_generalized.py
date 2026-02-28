"""Unit tests for ml.train_generalized."""

import json
import os
import tempfile
from unittest.mock import patch

import pytest

from ml.train_generalized import main


@patch("ml.train_generalized.run_generalized_pipeline")
@patch("ml.train_generalized.load_ball_by_ball_from_csv")
def test_train_generalized_csv_success(mock_load_csv, mock_run_pipeline):
    """main with --csv loads from CSV, runs pipeline, and writes metadata."""
    mock_load_csv.return_value = _make_fake_df(200)
    mock_run_pipeline.return_value = {
        "pipeline": _FakePipeline(),
        "best_model": "logistic",
        "best_metrics": {"val_metric": 0.15, "delta": 0.02},
        "summary": "logistic best",
    }
    meta = None
    with tempfile.TemporaryDirectory() as tmp:
        out_path = os.path.join(tmp, "out")
        os.makedirs(out_path, exist_ok=True)
        with patch(
            "sys.argv",
            ["train_generalized", "--csv", "/tmp/fake.csv", "--out", out_path],
        ):
            main()
        mock_load_csv.assert_called_once_with("/tmp/fake.csv")
        mock_run_pipeline.assert_called_once()
        meta_path = os.path.join(out_path, "generalized_pipeline_player-performance_metadata.json")
        assert os.path.exists(meta_path), f"Expected {meta_path}; listing: {os.listdir(out_path)}"
        with open(meta_path, encoding="utf-8") as f:
            meta = json.load(f)
    assert meta["best_model"] == "logistic"
    assert meta["task"] == "player_performance"


@patch("ml.train_generalized.load_ball_by_ball_from_csv")
def test_train_generalized_csv_insufficient_data_exits(mock_load_csv):
    """main with insufficient rows exits with error."""
    mock_load_csv.return_value = _make_fake_df(50)
    with patch("sys.argv", ["train_generalized", "--csv", "/tmp/fake.csv"]):
        with pytest.raises(SystemExit) as exc:
            main()
    assert exc.value.code == 1


@patch("ml.train_generalized.load_ball_by_ball_from_db")
@patch("ml.train_generalized.load_ball_by_ball_from_csv")
def test_train_generalized_no_csv_or_db_exits(mock_load_csv, mock_load_db):
    """main without --csv or --from-db exits."""
    with patch("sys.argv", ["train_generalized"]):
        with pytest.raises(SystemExit) as exc:
            main()
    assert exc.value.code == 1
    mock_load_csv.assert_not_called()
    mock_load_db.assert_not_called()


@patch("ml.train_generalized.run_generalized_pipeline")
@patch("ml.train_generalized.load_ball_by_ball_from_db")
def test_train_generalized_from_db_success(mock_load_db, mock_run_pipeline):
    """main with --from-db loads from DB and runs pipeline."""
    mock_load_db.return_value = _make_fake_df(250)
    mock_run_pipeline.return_value = {
        "pipeline": _FakePipeline(),
        "best_model": "xgboost",
        "best_metrics": {"val_metric": 0.12, "delta": 0.03},
        "summary": "xgboost best",
    }
    with tempfile.TemporaryDirectory() as tmp:
        with patch.dict(os.environ, {"ML_SERVICE_OUTPUT_DIR": tmp}, clear=False):
            with patch(
                "sys.argv",
                ["train_generalized", "--from-db", "--cutoff", "2024-12-01"],
            ):
                main()
    assert mock_load_db.called
    cutoff = mock_load_db.call_args[1].get("cutoff_date")
    assert cutoff is not None
    assert cutoff.year == 2024 and cutoff.month == 12


@patch("ml.train_generalized.run_generalized_pipeline")
@patch("ml.train_generalized.load_ball_by_ball_from_db")
def test_train_generalized_from_db_with_format_filter(mock_load_db, mock_run_pipeline):
    """main with --from-db and --format passes format_codes to loader."""
    mock_load_db.return_value = _make_fake_df(250)
    mock_run_pipeline.return_value = {
        "pipeline": _FakePipeline(),
        "best_model": "logistic",
        "best_metrics": {"val_metric": 0.10, "delta": 0.02},
        "summary": "logistic best",
    }
    with tempfile.TemporaryDirectory() as tmp:
        with patch.dict(os.environ, {"ML_SERVICE_OUTPUT_DIR": tmp}, clear=False):
            with patch(
                "sys.argv",
                ["train_generalized", "--from-db", "--cutoff", "2024-06-01", "--format", "ODI, T20I"],
            ):
                main()
    assert mock_load_db.called
    format_codes = mock_load_db.call_args[1].get("format_codes")
    assert format_codes == ["ODI", "T20I"]


@patch("ml.train_generalized.run_generalized_pipeline")
@patch("ml.train_generalized.load_ball_by_ball_from_csv")
def test_train_generalized_pipeline_error_exits(mock_load_csv, mock_run_pipeline):
    """main exits when pipeline returns error."""
    mock_load_csv.return_value = _make_fake_df(200)
    mock_run_pipeline.return_value = {"error": "Something failed"}
    with patch("sys.argv", ["train_generalized", "--csv", "/tmp/fake.csv"]):
        with pytest.raises(SystemExit) as exc:
            main()
    assert exc.value.code == 1


def _make_fake_df(n: int):
    """Minimal ball-by-ball DataFrame for train_generalized tests."""
    import numpy as np
    import pandas as pd

    rng = np.random.default_rng(42)
    return pd.DataFrame(
        {
            "match_id": np.repeat(np.arange(max(1, n // 50)), 50)[:n],
            "innings": np.tile([1, 2], n // 2)[:n],
            "ball_seq": np.arange(n) % 120,
            "runs_total": rng.integers(0, 7, n),
            "wicket_kind": None,
            "target_runs": 250,
            "match_date": pd.date_range("2022-01-01", periods=n, freq="h"),
            "format_code": "ODI",
            "venue_id": 1,
            "striker_id": rng.integers(1, 50, n),
            "bowler_id": rng.integers(51, 100, n),
        }
    )


class _FakePipeline:
    """Minimal pipeline mock with feature_names_."""

    feature_names_ = ["f1", "f2", "f3"]
