"""Unit tests for ml.tracking duration parsing (pure functions, no DB)."""

import os
from unittest.mock import patch

from ml.tracking import (
    PIPELINE_COMMANDS,
    _parse_go_duration,
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
    with patch.dict(os.environ, {"TRACKING_STALE_CANCEL_AGE": "", "TRACKING_STALE_CANCEL_AGE_MINUTES": ""}, clear=False):
        assert parse_stale_cancel_age_seconds() is None


def test_pipeline_commands_non_empty():
    """PIPELINE_COMMANDS is non-empty and contains expected commands."""
    assert len(PIPELINE_COMMANDS) > 0
    assert "train-batting" in PIPELINE_COMMANDS
    assert "train-bowling" in PIPELINE_COMMANDS
