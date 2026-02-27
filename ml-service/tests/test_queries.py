"""Unit tests for ml.queries (SQL query strings)."""

from ml import queries


def test_batting_dataset_query():
    """batting_dataset_query contains expected tables."""
    assert "batting_data" in queries.batting_dataset_query
    assert "player" in queries.batting_dataset_query
    assert "weather" in queries.batting_dataset_query


def test_batting_win_dataset_query():
    """batting_win_dataset_query contains expected tables."""
    assert "batting_data" in queries.batting_win_dataset_query
    assert "match_details" in queries.batting_win_dataset_query


def test_bowling_dataset_query():
    """bowling_dataset_query contains expected tables."""
    assert "bowling_data" in queries.bowling_dataset_query
    assert "player" in queries.bowling_dataset_query
