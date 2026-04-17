"""Tests for ml.match_level_derived_features (shared extras/innings derived columns)."""

from __future__ import annotations

import logging

import pandas as pd
import pytest

from ml import match_level_derived_features as mldf
from ml.match_level_derived_features import (
    MATCH_LEVEL_DERIVED_FEATURE_COLS,
    add_match_level_derived_features_to_df,
    compute_match_level_derived_features_scalars,
    resolve_weights,
)


@pytest.fixture(autouse=True)
def _reset_weight_sum_warning_flag() -> None:
    """The weight-sum warning is emitted at most once per process; reset between tests."""
    mldf._reset_weight_sum_warning_state_for_tests()
    yield
    mldf._reset_weight_sum_warning_state_for_tests()


def test_match_level_derived_feature_cols_order() -> None:
    assert MATCH_LEVEL_DERIVED_FEATURE_COLS == (
        "form_differential",
        "consistency_differential",
        "weather_composite",
    )


def test_add_match_level_derived_features_to_df() -> None:
    df = pd.DataFrame(
        {
            "bat_form_sum": [10.0, 5.0],
            "bowl_form_sum": [3.0, 8.0],
            "bat_consistency_sum": [7.0, 4.0],
            "bowl_consistency_sum": [2.0, 6.0],
            "rain": [1.0, 0.0],
            "humidity": [80.0, 50.0],
            "cloud": [60.0, 20.0],
        }
    )
    add_match_level_derived_features_to_df(df)
    assert df["form_differential"].iloc[0] == pytest.approx(7.0)
    assert df["form_differential"].iloc[1] == pytest.approx(-3.0)
    assert df["consistency_differential"].iloc[0] == pytest.approx(5.0)
    assert df["consistency_differential"].iloc[1] == pytest.approx(-2.0)
    expected_0 = 0.5 * 1.0 + 0.3 * (80.0 / 100.0) + 0.2 * (60.0 / 100.0)
    assert df["weather_composite"].iloc[0] == pytest.approx(expected_0)


def test_add_match_level_derived_features_missing_columns_zero_fill() -> None:
    df = pd.DataFrame({"other": [1.0, 2.0]})
    add_match_level_derived_features_to_df(df)
    assert (df["form_differential"] == 0.0).all()
    assert (df["consistency_differential"] == 0.0).all()
    assert (df["weather_composite"] == 0.0).all()


def test_compute_match_level_derived_features_scalars_matches_dataframe_row() -> None:
    row = {
        "bat_form_sum": 12.0,
        "bowl_form_sum": 4.0,
        "bat_consistency_sum": 8.0,
        "bowl_consistency_sum": 3.0,
        "rain": 0.0,
        "humidity": 60.0,
        "cloud": 40.0,
    }
    df = pd.DataFrame([row])
    add_match_level_derived_features_to_df(df)
    triplet = compute_match_level_derived_features_scalars(
        row["bat_form_sum"],
        row["bowl_form_sum"],
        row["bat_consistency_sum"],
        row["bowl_consistency_sum"],
        row["rain"],
        row["humidity"],
        row["cloud"],
    )
    assert triplet[0] == pytest.approx(float(df["form_differential"].iloc[0]))
    assert triplet[1] == pytest.approx(float(df["consistency_differential"].iloc[0]))
    assert triplet[2] == pytest.approx(float(df["weather_composite"].iloc[0]))


def test_compute_match_level_derived_features_scalars_non_finite_to_zero() -> None:
    fd, cd, wc = compute_match_level_derived_features_scalars(
        float("nan"),
        2.0,
        float("inf"),
        1.0,
        float("nan"),
        100.0,
        50.0,
    )
    assert fd == pytest.approx(0.0 - 2.0)
    assert cd == pytest.approx(0.0 - 1.0)
    assert wc == pytest.approx(0.5 * 0.0 + 0.3 * 1.0 + 0.2 * 0.5)


def test_compute_scalars_respects_explicit_weights() -> None:
    """Inference should be able to pin training-time weights via the sidecar."""
    pinned = {
        "weather_composite_rain_weight": 1.0,
        "weather_composite_humidity_weight": 0.0,
        "weather_composite_cloud_weight": 0.0,
    }
    _, _, wc = compute_match_level_derived_features_scalars(
        0.0,
        0.0,
        0.0,
        0.0,
        1.0,
        100.0,
        100.0,
        weights=pinned,
    )
    assert wc == pytest.approx(1.0)


def test_add_to_df_respects_explicit_weights() -> None:
    """Pinned weights at inference override the current service config."""
    df = pd.DataFrame({"rain": [0.0, 1.0], "humidity": [100.0, 0.0], "cloud": [0.0, 0.0]})
    pinned = {
        "weather_composite_rain_weight": 2.0,
        "weather_composite_humidity_weight": 0.5,
        "weather_composite_cloud_weight": 0.0,
    }
    add_match_level_derived_features_to_df(df, weights=pinned)
    assert df["weather_composite"].iloc[0] == pytest.approx(0.5 * 1.0)  # humidity/100 * 0.5
    assert df["weather_composite"].iloc[1] == pytest.approx(2.0 * 1.0)


def test_add_to_df_partial_weights_fill_from_config() -> None:
    """Partial overrides fall back to the service config for missing keys."""
    df = pd.DataFrame({"rain": [1.0], "humidity": [100.0], "cloud": [100.0]})
    partial = {"weather_composite_rain_weight": 0.0}
    add_match_level_derived_features_to_df(df, weights=partial)
    default_humidity = 0.3
    default_cloud = 0.2
    assert df["weather_composite"].iloc[0] == pytest.approx(default_humidity + default_cloud)


def test_resolve_weights_warns_when_sum_exceeds_envelope(caplog: pytest.LogCaptureFixture) -> None:
    """Weight sum > 1.5 should emit a single logger warning."""
    noisy = {
        "weather_composite_rain_weight": 1.0,
        "weather_composite_humidity_weight": 0.8,
        "weather_composite_cloud_weight": 0.5,  # sum = 2.3
    }
    with caplog.at_level(logging.WARNING, logger=mldf.__name__):
        resolve_weights(noisy)
    assert any("weather_composite weight sum" in r.message for r in caplog.records)


def test_resolve_weights_warns_only_once(caplog: pytest.LogCaptureFixture) -> None:
    """Repeated calls with out-of-range weights should not spam the log."""
    noisy = {
        "weather_composite_rain_weight": 5.0,
        "weather_composite_humidity_weight": 0.0,
        "weather_composite_cloud_weight": 0.0,
    }
    with caplog.at_level(logging.WARNING, logger=mldf.__name__):
        for _ in range(5):
            resolve_weights(noisy)
    warnings = [r for r in caplog.records if "weather_composite weight sum" in r.message]
    assert len(warnings) == 1


def test_resolve_weights_within_envelope_emits_no_warning(caplog: pytest.LogCaptureFixture) -> None:
    """Canonical weights (sum ~ 1.0) must not trigger the warning."""
    with caplog.at_level(logging.WARNING, logger=mldf.__name__):
        resolve_weights(None)  # falls back to config defaults
    assert not any("weather_composite weight sum" in r.message for r in caplog.records)
