"""Unit tests for ml.data_quality."""

import numpy as np
import pandas as pd

from ml.data_quality import (
    CATEGORICAL_SENTINEL,
    CATEGORICAL_SENTINEL_FEATURES,
    clip_target_outliers,
    impute_features,
)


def test_impute_features_skips_missing_columns():
    """impute_features skips columns not in DataFrame."""
    df = pd.DataFrame({"a": [1.0, 2.0, np.nan], "b": [10, 20, 30]})
    out, medians = impute_features(df, ["a", "missing_col", "b"])
    assert "a" in medians
    assert "b" in medians
    assert "missing_col" not in medians
    assert out["a"].iloc[2] == 1.5
    assert len(out) == 3


def test_impute_features_categorical_sentinel():
    """impute_features uses -1 sentinel for categorical columns."""
    df = pd.DataFrame(
        {"venue": [1.0, 2.0, np.nan], "numeric": [10.0, 20.0, np.nan]}
    )
    out, _ = impute_features(df, ["venue", "numeric"], categorical_features=frozenset({"venue"}))
    assert out["venue"].iloc[2] == CATEGORICAL_SENTINEL
    assert out["numeric"].iloc[2] == 15.0


def test_impute_features_precomputed_medians():
    """impute_features uses precomputed_medians when provided."""
    df = pd.DataFrame({"x": [1.0, np.nan, 3.0]})
    out, medians = impute_features(df, ["x"], precomputed_medians={"x": 99.0})
    assert out["x"].iloc[1] == 99.0
    assert medians["x"] == 99.0


def test_impute_features_all_nan_uses_zero():
    """impute_features uses 0.0 when column is all NaN."""
    df = pd.DataFrame({"all_nan": [np.nan, np.nan, np.nan]})
    out, medians = impute_features(df, ["all_nan"])
    assert medians["all_nan"] == 0.0
    assert out["all_nan"].iloc[0] == 0.0


def test_clip_target_outliers_invalid_percentile_returns_unchanged():
    """clip_target_outliers returns unchanged targets when percentile invalid."""
    targets = np.array([[1.0, 2.0], [3.0, 4.0]])
    clipped, info = clip_target_outliers(targets, percentile=100.0)
    np.testing.assert_array_equal(clipped, targets)
    assert info == {}
    clipped2, info2 = clip_target_outliers(targets, percentile=0.0)
    np.testing.assert_array_equal(clipped2, targets)
    assert info2 == {}


def test_clip_target_outliers_clips_and_logs():
    """clip_target_outliers clips outliers and returns clip info (exercises logger path)."""
    targets = np.array([[1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 1000.0]]).T
    clipped, info = clip_target_outliers(targets, percentile=90.0, target_names=["x"])
    assert "x" in info
    assert "upper_bound" in info["x"]
    assert info["x"]["n_clipped_upper"] >= 1
    assert clipped[9, 0] <= info["x"]["upper_bound"]
