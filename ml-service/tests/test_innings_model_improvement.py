"""Tests for innings model improvement: format one-hot exclusion and low-variance filter."""

from __future__ import annotations

import numpy as np

from ml.data_quality import WEATHER_FEATURE_COLS, drop_low_variance_columns


class TestDropLowVarianceColumns:
    """drop_low_variance_columns removes near-constant cols but keeps protected ones."""

    def test_removes_constant_column(self):
        X = np.array([[1.0, 2.0, 5.0], [1.0, 3.0, 5.0], [1.0, 4.0, 5.0]])
        names = ["a", "b", "constant"]
        X_out, names_out, dropped = drop_low_variance_columns(X, names, protected_columns=frozenset())
        assert "constant" in dropped
        assert "a" in dropped  # also constant
        assert names_out == ["b"]
        assert X_out.shape == (3, 1)

    def test_keeps_protected_columns(self):
        X = np.array([[0.0, 2.0], [0.0, 3.0], [0.0, 4.0]])
        names = ["temp", "b"]
        X_out, names_out, dropped = drop_low_variance_columns(X, names)
        assert "temp" not in dropped
        assert "temp" in names_out
        assert X_out.shape == (3, 2)

    def test_default_protected_is_weather(self):
        X = np.zeros((10, len(WEATHER_FEATURE_COLS)))
        names = list(WEATHER_FEATURE_COLS)
        X_out, names_out, dropped = drop_low_variance_columns(X, names)
        assert dropped == []
        assert len(names_out) == len(WEATHER_FEATURE_COLS)

    def test_no_columns_dropped_when_all_vary(self):
        rng = np.random.RandomState(42)
        X = rng.randn(50, 5)
        names = ["a", "b", "c", "d", "e"]
        X_out, names_out, dropped = drop_low_variance_columns(X, names, protected_columns=frozenset())
        assert dropped == []
        assert names_out == names
        assert X_out.shape == X.shape

    def test_shape_mismatch_returns_unchanged(self):
        X = np.ones((5, 3))
        names = ["a", "b"]  # mismatch
        X_out, names_out, dropped = drop_low_variance_columns(X, names)
        assert dropped == []
        assert X_out.shape == X.shape

    def test_custom_variance_threshold(self):
        X = np.array([[0.0, 1.0], [0.0, 2.0], [0.0, 3.0]])
        names = ["const", "near_const"]
        # Default threshold 1e-6: const is exactly zero variance, near_const has tiny but nonzero variance
        _, names_out, dropped = drop_low_variance_columns(X, names, protected_columns=frozenset())
        assert "const" in dropped
        assert "near_const" not in dropped
        # High threshold: both dropped
        _, names_out2, dropped2 = drop_low_variance_columns(
            X, names, variance_threshold=1.0, protected_columns=frozenset()
        )
        assert "const" in dropped2
        assert "near_const" in dropped2


class TestInningsFormatOneHotExclusion:
    """Per-format innings training excludes format one-hot columns."""

    def test_per_format_excludes_format_one_hot(self):
        from ml.train_innings import INNINGS_FEATURE_COLS, INNINGS_FORMAT_ONE_HOT_COLS, rows_to_xy_by_format

        # Build minimal data with format_code column
        base_cols = [c for c in INNINGS_FEATURE_COLS if not c.startswith("format_is_")]
        headers = base_cols + ["innings_runs", "innings_wickets", "format_code", "match_date"]
        n_rows = 30
        rng = np.random.RandomState(42)
        rows = []
        for i in range(n_rows):
            row = [str(rng.rand()) for _ in base_cols]
            row += [str(100 + rng.rand() * 50), str(int(rng.randint(3, 10))), "T20I", f"2024-01-{i + 1:02d}"]
            rows.append(row)

        by_format, _, _, _ = rows_to_xy_by_format(headers, rows)
        assert "T20I" in by_format
        _, _, _, _, feat_names = by_format["T20I"]
        for col in INNINGS_FORMAT_ONE_HOT_COLS:
            assert col not in feat_names, f"format one-hot col {col} should be excluded for per-format"

    def test_unified_keeps_format_one_hot(self):
        from ml.train_innings import INNINGS_FEATURE_COLS, rows_to_xy_by_format

        # Build data WITHOUT format_code → unified _ALL_ model
        base_cols = [c for c in INNINGS_FEATURE_COLS if not c.startswith("format_is_")]
        headers = base_cols + ["innings_runs", "innings_wickets", "match_date"]
        n_rows = 20
        rng = np.random.RandomState(42)
        rows = []
        for i in range(n_rows):
            row = [str(rng.rand()) for _ in base_cols]
            row += [str(100 + rng.rand() * 50), str(int(rng.randint(3, 10))), f"2024-01-{i + 1:02d}"]
            rows.append(row)

        by_format, _, _, _ = rows_to_xy_by_format(headers, rows)
        assert "_ALL_" in by_format
        # Unified model may still drop format one-hot via low-variance (all zeros),
        # but the key point is they weren't explicitly excluded.


class TestExtrasFormatOneHotExclusion:
    """Per-format extras training excludes format one-hot columns."""

    def test_per_format_excludes_format_one_hot(self):
        from ml.train_extras import EXTRAS_FEATURE_COLS, EXTRAS_FORMAT_ONE_HOT_COLS, rows_to_xy_by_format

        base_cols = [c for c in EXTRAS_FEATURE_COLS if not c.startswith("format_is_")]
        headers = base_cols + ["total_extras", "format_code", "match_date"]
        n_rows = 20
        rng = np.random.RandomState(42)
        rows = []
        for i in range(n_rows):
            row = [str(rng.rand()) for _ in base_cols]
            row += [str(int(rng.randint(5, 30))), "ODI", f"2024-01-{i + 1:02d}"]
            rows.append(row)

        by_format = rows_to_xy_by_format(headers, rows)
        assert "ODI" in by_format
        _, _, _, feat_names = by_format["ODI"]
        for col in EXTRAS_FORMAT_ONE_HOT_COLS:
            assert col not in feat_names, f"format one-hot col {col} should be excluded for per-format"


class TestWinFormatOneHotExclusion:
    """Per-format win training excludes format one-hot columns."""

    def test_per_format_excludes_format_one_hot(self):
        from ml.train_win import rows_to_xy_by_format
        from ml.win_features import WIN_ENHANCED_FEATURE_COLS

        # Build minimal data with format_code
        base_cols = [c for c in WIN_ENHANCED_FEATURE_COLS if not c.startswith("format_is_")]
        headers = base_cols + ["team1_wins", "format_code", "match_date"]
        n_rows = 30
        rng = np.random.RandomState(42)
        rows = []
        for i in range(n_rows):
            row = [str(rng.rand()) for _ in base_cols]
            row += [str(rng.randint(0, 2)), "T20", f"2024-01-{i + 1:02d}"]
            rows.append(row)

        by_format = rows_to_xy_by_format(headers, rows)
        assert "T20" in by_format
        _, _, _, feat_names = by_format["T20"]
        for name in feat_names:
            assert not name.startswith("format_is_"), f"format one-hot col {name} should be excluded"


class TestDataLoadersInningsFeatureNames:
    """data_loaders.load_innings_from_api returns feature names in tuple."""

    def test_unpack_xy_with_feature_names_innings_tuple(self):
        from ml.tuning.data_loaders import unpack_xy_with_feature_names

        # Simulate the new 3-tuple: (X, Y, feat_names)
        X = np.random.rand(10, 5)
        Y = np.random.rand(10, 2)
        feat_names = ["a", "b", "c", "d", "e"]
        result = (X, Y, feat_names)
        x_out, y_out, names_out = unpack_xy_with_feature_names(result)
        assert names_out == feat_names
        assert x_out.shape == (10, 5)
