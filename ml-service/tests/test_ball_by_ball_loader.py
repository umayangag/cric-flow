"""Unit tests for ml.ball_by_ball_loader."""

import tempfile
from unittest.mock import MagicMock, patch

import pandas as pd

from ml.ball_by_ball_loader import (
    BALL_COLS,
    MATCH_CONTEXT_COLS,
    load_ball_by_ball_from_csv,
    load_ball_by_ball_from_db,
)


def test_load_ball_by_ball_from_csv_basic():
    """load_ball_by_ball_from_csv loads CSV and parses match_date."""
    rows = [
        {
            "match_id": 1,
            "innings": 1,
            "over": 1,
            "ball": 1,
            "ball_seq": 1,
            "is_legal": True,
            "phase": "powerplay",
            "striker_id": 10,
            "non_striker_id": 11,
            "bowler_id": 20,
            "runs_batter": 1,
            "runs_extras": 0,
            "runs_total": 1,
            "extras_kind": None,
            "wicket_kind": None,
            "player_out_id": None,
            "match_date": "2024-06-15",
            "format_code": "ODI",
            "venue_id": 1,
            "target_runs": 250,
            "balls_per_innings": 300,
        },
    ]
    df_in = pd.DataFrame(rows)
    with tempfile.NamedTemporaryFile(mode="w", suffix=".csv", delete=False) as f:
        df_in.to_csv(f.name, index=False)
        path = f.name

    df = load_ball_by_ball_from_csv(path)
    assert len(df) == 1
    assert "match_date" in df.columns
    assert df["match_date"].iloc[0].year == 2024
    assert df["format_code"].iloc[0] == "ODI"
    assert df["runs_total"].iloc[0] == 1


def test_load_ball_by_ball_from_csv_no_match_date():
    """load_ball_by_ball_from_csv handles CSV without match_date."""
    df_in = pd.DataFrame([{"match_id": 1, "runs_total": 1}])
    with tempfile.NamedTemporaryFile(mode="w", suffix=".csv", delete=False) as f:
        df_in.to_csv(f.name, index=False)
        path = f.name

    df = load_ball_by_ball_from_csv(path)
    assert len(df) == 1
    assert "match_id" in df.columns


def test_ball_cols_and_match_context_cols():
    """BALL_COLS and MATCH_CONTEXT_COLS define expected columns."""
    assert "match_id" in BALL_COLS
    assert "runs_total" in BALL_COLS
    assert "match_date" in MATCH_CONTEXT_COLS
    assert "format_code" in MATCH_CONTEXT_COLS


@patch("ml.db.get_db_connection")
def test_load_ball_by_ball_from_db_mocked(mock_get_conn):
    """load_ball_by_ball_from_db returns DataFrame from mocked DB."""
    mock_conn = MagicMock()
    mock_get_conn.return_value = mock_conn
    df_fake = pd.DataFrame(
        {
            "match_id": [1, 1],
            "innings": [1, 1],
            "over": [1, 1],
            "ball": [1, 2],
            "ball_seq": [1, 2],
            "is_legal": [True, True],
            "phase": ["powerplay", "powerplay"],
            "striker_id": [10, 10],
            "non_striker_id": [11, 11],
            "bowler_id": [20, 20],
            "runs_batter": [1, 0],
            "runs_extras": [0, 0],
            "runs_total": [1, 0],
            "extras_kind": [None, None],
            "wicket_kind": [None, None],
            "player_out_id": [None, None],
            "match_date": ["2024-06-15", "2024-06-15"],
            "format_id": [1, 1],
            "format_code": ["ODI", "ODI"],
            "venue_id": [1, 1],
            "season_id": [1, 1],
            "target_runs": [250, 250],
            "balls_per_innings": [300, 300],
        }
    )
    with patch("pandas.read_sql", return_value=df_fake):
        df = load_ball_by_ball_from_db(cutoff_date=None, format_codes=None)
    assert len(df) == 2
    assert df["match_date"].iloc[0].year == 2024
    mock_conn.close.assert_called_once()


@patch("ml.db.get_db_connection")
def test_load_ball_by_ball_from_db_no_match_date_column(mock_get_conn):
    """load_ball_by_ball_from_db handles DF without match_date column."""
    mock_conn = MagicMock()
    mock_get_conn.return_value = mock_conn
    df_fake = pd.DataFrame({"match_id": [1], "innings": [1], "runs_total": [1]})
    with patch("pandas.read_sql", return_value=df_fake):
        df = load_ball_by_ball_from_db(cutoff_date=None, format_codes=None)
    assert len(df) == 1
    assert "match_id" in df.columns
    mock_conn.close.assert_called_once()


@patch("ml.db.get_db_connection")
def test_load_ball_by_ball_from_db_with_filters(mock_get_conn):
    """load_ball_by_ball_from_db accepts cutoff_date and format_codes."""
    mock_conn = MagicMock()
    mock_get_conn.return_value = mock_conn
    df_fake = pd.DataFrame(
        {
            "match_id": [1],
            "innings": [1],
            "match_date": ["2024-01-01"],
            "format_code": ["T20I"],
            "runs_total": [1],
        }
    )
    with patch("pandas.read_sql", return_value=df_fake) as mock_read:
        from datetime import date

        load_ball_by_ball_from_db(cutoff_date=date(2024, 6, 1), format_codes=["ODI", "T20I"])
    call_args = mock_read.call_args
    assert call_args[1]["params"] == ["ODI", "T20I", date(2024, 6, 1)]
