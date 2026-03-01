"""Unit tests for backtest_service build_*_features_from_map (coverage)."""

from datetime import datetime
from unittest.mock import patch

from app.backtest_service import (
    build_batting_features_from_map,
    build_bowling_features_from_map,
    build_fielding_features_from_map,
)


def _base_batting_map():
    return {
        "batting_consistency": 0.5,
        "batting_form": 20.0,
        "batting_form_short": 18.0,
        "batting_form_long": 19.0,
        "batting_momentum": 0.1,
        "batting_temp": 28,
        "batting_wind": 3,
        "batting_rain": 0,
        "batting_humidity": 55,
        "batting_cloud": 10,
        "batting_pressure": 1010,
        "batting_viscosity": 0,
        "batting_inning": 1,
        "batting_session": 1,
        "toss": 1,
        "venue": 0.4,
        "opposition": 0.6,
        "season": 2024,
    }


def _base_bowling_map():
    return {
        "bowling_consistency": 0.4,
        "bowling_form": 1.5,
        "bowling_form_short": 1.4,
        "bowling_form_long": 1.3,
        "bowling_momentum": 0.05,
        "bowling_career_avg": 1.4,
        "bowling_temp": 28,
        "bowling_wind": 3,
        "bowling_rain": 0,
        "bowling_humidity": 55,
        "bowling_cloud": 10,
        "bowling_pressure": 1010,
        "bowling_viscosity": 0,
        "batting_inning": 1,
        "bowling_session": 2,
        "toss": 0,
        "bowling_venue": 0.5,
        "bowling_opposition": 0.5,
        "season": 2024,
    }


def test_build_batting_features_from_map_with_seq_keys():
    """Seq keys are extracted and default to 0 when absent."""
    cutoff = datetime(2024, 6, 15)
    m = _base_batting_map()
    m["bat_window_sr_12_pp"] = 125.0
    m["bat_prev_sr"] = 110.0
    f = build_batting_features_from_map(1, cutoff, "T20", m)
    assert f.bat_window_sr_12_pp == 125.0
    assert f.bat_prev_sr == 110.0
    assert f.bat_prev_out_rate == 0.0  # absent, default 0


def test_build_bowling_features_from_map_with_seq_keys():
    """Seq keys are extracted and default to 0 when absent."""
    cutoff = datetime(2024, 6, 15)
    m = _base_bowling_map()
    m["bowl_window_econ_24_death"] = 8.5
    m["bowl_prev_wkt_rate"] = 0.15
    f = build_bowling_features_from_map(1, cutoff, "ODI", m)
    assert f.bowl_window_econ_24_death == 8.5
    assert f.bowl_prev_wkt_rate == 0.15
    assert f.bowl_over_ball1_wkt_rate == 0.0  # absent, default 0


def test_build_fielding_features_from_map():
    """Fielding features built from map with config defaults for missing keys."""
    cutoff = datetime(2024, 6, 15)
    m = {
        "fielding_consistency": 0.6,
        "fielding_form": 0.2,
        "season": 2024,
    }
    f = build_fielding_features_from_map(1, cutoff, "T20", m)
    assert f.fielding_consistency == 0.6
    assert f.fielding_form == 0.2
    assert f.fielding_season == 2024


def test_feature_defaults_raises_propagates():
    """When get_feature_defaults raises, _feature_defaults propagates (lines 74-75)."""
    import pytest

    from app.backtest_service import build_fielding_features_from_map

    def raiser():
        raise ValueError("config error")

    cutoff = datetime(2024, 6, 15)
    m = {"fielding_consistency": 0.6, "fielding_form": 0.2, "season": 2024}
    with patch("app.backtest_service.get_feature_defaults", side_effect=raiser):
        with pytest.raises(ValueError, match="config error"):
            build_fielding_features_from_map(1, cutoff, "T20", m)
