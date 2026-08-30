"""Tests for ml.match_level_derived_features (shared extras/innings derived columns)."""

from __future__ import annotations

import pandas as pd
import pytest

from ml.match_level_derived_features import (
    MATCH_LEVEL_DERIVED_FEATURE_COLS,
    add_match_level_derived_features_to_df,
    compute_match_level_derived_features_scalars,
)


def test_match_level_derived_feature_cols_order() -> None:
    assert MATCH_LEVEL_DERIVED_FEATURE_COLS == (
        "form_differential",
        "consistency_differential",
    )


def test_add_match_level_derived_features_to_df() -> None:
    df = pd.DataFrame(
        {
            "bat_form_sum": [10.0, 5.0],
            "bowl_form_sum": [3.0, 8.0],
            "bat_consistency_sum": [7.0, 4.0],
            "bowl_consistency_sum": [2.0, 6.0],
        }
    )
    add_match_level_derived_features_to_df(df)
    assert df["form_differential"].iloc[0] == pytest.approx(7.0)
    assert df["form_differential"].iloc[1] == pytest.approx(-3.0)
    assert df["consistency_differential"].iloc[0] == pytest.approx(5.0)
    assert df["consistency_differential"].iloc[1] == pytest.approx(-2.0)


def test_add_match_level_derived_features_only_adds_the_declared_columns() -> None:
    """The weather composite is gone; nothing should reintroduce a third column."""
    df = pd.DataFrame({"bat_form_sum": [1.0], "bowl_form_sum": [0.0]})
    add_match_level_derived_features_to_df(df)
    added = set(df.columns) - {"bat_form_sum", "bowl_form_sum"}
    assert added == set(MATCH_LEVEL_DERIVED_FEATURE_COLS)


def test_add_match_level_derived_features_missing_columns_zero_fill() -> None:
    df = pd.DataFrame({"other": [1.0, 2.0]})
    add_match_level_derived_features_to_df(df)
    assert (df["form_differential"] == 0.0).all()
    assert (df["consistency_differential"] == 0.0).all()


def test_compute_match_level_derived_features_scalars_matches_dataframe_row() -> None:
    row = {
        "bat_form_sum": 12.0,
        "bowl_form_sum": 4.0,
        "bat_consistency_sum": 8.0,
        "bowl_consistency_sum": 3.0,
    }
    df = pd.DataFrame([row])
    add_match_level_derived_features_to_df(df)
    pair = compute_match_level_derived_features_scalars(
        row["bat_form_sum"],
        row["bowl_form_sum"],
        row["bat_consistency_sum"],
        row["bowl_consistency_sum"],
    )
    assert pair[0] == pytest.approx(float(df["form_differential"].iloc[0]))
    assert pair[1] == pytest.approx(float(df["consistency_differential"].iloc[0]))


def test_compute_match_level_derived_features_scalars_non_finite_to_zero() -> None:
    fd, cd = compute_match_level_derived_features_scalars(
        float("nan"),
        2.0,
        float("inf"),
        1.0,
    )
    assert fd == pytest.approx(0.0 - 2.0)
    assert cd == pytest.approx(0.0 - 1.0)
