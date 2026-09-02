"""Unit tests for ml.tracking duration parsing and db_connection (pure functions, no DB)."""

import os
from unittest.mock import MagicMock, patch

from ml.tracking import (
    PIPELINE_COMMANDS,
    _parse_go_duration,
    db_connection,
    parse_stale_cancel_age_seconds,
)


def test_parse_go_duration_simple():
    """_parse_go_duration parses simple units."""
    assert _parse_go_duration("90s") == 90
    assert _parse_go_duration("24h") == 24 * 3600
    assert _parse_go_duration("1h") == 3600
    assert _parse_go_duration("30m") == 30 * 60
    assert _parse_go_duration("1m") == 60


def test_parse_go_duration_compound():
    """_parse_go_duration parses compound formats like 1h30m."""
    assert _parse_go_duration("1h30m") == 3600 + 30 * 60
    assert _parse_go_duration("2h15m30s") == 2 * 3600 + 15 * 60 + 30


def test_parse_go_duration_whitespace():
    """Whitespace is stripped before parsing."""
    assert _parse_go_duration("  24h  ") == 24 * 3600
    assert _parse_go_duration("1h 30m") == 3600 + 30 * 60  # space removed


def test_parse_go_duration_subsecond():
    """_parse_go_duration parses ms, us, ns."""
    assert _parse_go_duration("1000ms") == 1
    assert _parse_go_duration("2000ms") == 2
    assert _parse_go_duration("1s") == 1


def test_parse_go_duration_invalid():
    """Invalid input returns None."""
    assert _parse_go_duration("") is None
    assert _parse_go_duration("abc") is None
    assert _parse_go_duration("24") is None
    assert _parse_go_duration("24hx") is None
    assert _parse_go_duration("24hz") is None
    assert _parse_go_duration("0h") is None
    assert _parse_go_duration("-1h") is None


def test_parse_stale_cancel_age_from_env():
    """parse_stale_cancel_age_seconds uses TRACKING_STALE_CANCEL_AGE when set."""
    with patch.dict(os.environ, {"TRACKING_STALE_CANCEL_AGE": "90s"}):
        assert parse_stale_cancel_age_seconds() == 90


def test_parse_stale_cancel_age_legacy_minutes():
    """parse_stale_cancel_age_seconds uses legacy TRACKING_STALE_CANCEL_AGE_MINUTES."""
    with patch.dict(os.environ, {"TRACKING_STALE_CANCEL_AGE": "", "TRACKING_STALE_CANCEL_AGE_MINUTES": "60"}):
        assert parse_stale_cancel_age_seconds() == 3600


def test_parse_stale_cancel_age_empty_returns_none():
    """When neither env is set, returns None (use default)."""
    with patch.dict(
        os.environ, {"TRACKING_STALE_CANCEL_AGE": "", "TRACKING_STALE_CANCEL_AGE_MINUTES": ""}, clear=False
    ):
        assert parse_stale_cancel_age_seconds() is None


def test_parse_stale_cancel_age_invalid_returns_none():
    """parse_stale_cancel_age_seconds returns None when TRACKING_STALE_CANCEL_AGE is invalid."""
    with patch.dict(os.environ, {"TRACKING_STALE_CANCEL_AGE": "invalid", "TRACKING_STALE_CANCEL_AGE_MINUTES": ""}):
        assert parse_stale_cancel_age_seconds() is None


def test_parse_stale_cancel_age_legacy_invalid_returns_none():
    """parse_stale_cancel_age_seconds returns None when TRACKING_STALE_CANCEL_AGE_MINUTES is invalid."""
    with patch.dict(
        os.environ, {"TRACKING_STALE_CANCEL_AGE": "", "TRACKING_STALE_CANCEL_AGE_MINUTES": "abc"}, clear=False
    ):
        assert parse_stale_cancel_age_seconds() is None


def test_pipeline_commands_are_the_registry_steps():
    """The singleton list is the compute lane, and it has to name the same commands go-app's
    registry does -- a step missing from one side is a step that can run twice at once."""
    assert set(PIPELINE_COMMANDS) == {"cricsheet-import", "xi-retrain", "xi-evaluate", "xi-reload"}


def test_db_connection_close_raises_logs_and_swallows():
    """When conn.close() raises, db_connection logs and swallows (lines 46-48)."""
    mock_conn = MagicMock()
    mock_conn.close.side_effect = OSError("connection reset")

    with patch("ml.tracking.get_connection", return_value=mock_conn):
        with db_connection():
            pass
    mock_conn.close.assert_called_once()
