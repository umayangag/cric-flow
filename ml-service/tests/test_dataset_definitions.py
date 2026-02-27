"""Unit tests for ml.dataset_definitions."""

import numpy as np

from ml.dataset_definitions import (
    all_batting_columns,
    all_bowling_columns,
    derived_batting_columns,
    derived_bowling_columns,
    input_batting_columns,
    input_bowling_columns,
    match_summary_columns,
    output_batting_columns,
    output_bowling_columns,
    player_columns,
)


def test_player_columns_non_empty():
    assert len(player_columns) > 0
    assert "id" in player_columns
    assert "player_name" in player_columns


def test_input_output_batting_columns():
    assert "batting_consistency" in input_batting_columns
    assert "runs_scored" in output_batting_columns
    assert "strike_rate" in derived_batting_columns


def test_input_output_bowling_columns():
    assert "bowling_consistency" in input_bowling_columns
    assert "runs_conceded" in output_bowling_columns
    assert "econ" in derived_bowling_columns


def test_all_batting_columns_concatenation():
    assert isinstance(all_batting_columns, np.ndarray)
    assert len(all_batting_columns) == len(input_batting_columns) + len(output_batting_columns) + len(
        derived_batting_columns
    )


def test_all_bowling_columns_concatenation():
    assert isinstance(all_bowling_columns, np.ndarray)
    assert len(all_bowling_columns) == len(input_bowling_columns) + len(output_bowling_columns) + len(
        derived_bowling_columns
    )


def test_match_summary_columns():
    assert "total_score" in match_summary_columns
    assert "result" in match_summary_columns
