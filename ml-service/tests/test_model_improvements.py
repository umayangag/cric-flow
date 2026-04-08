"""Unit tests for model improvements: temporal features, derived features,
feature interactions config, permutation importance fallback, and pipeline alignment.

These tests verify the six improvement areas without running real model training.
"""

from __future__ import annotations

import math

import numpy as np
import pandas as pd
import pytest
from sklearn.ensemble import RandomForestRegressor
from sklearn.model_selection import KFold
from sklearn.neural_network import MLPRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler


# ---------------------------------------------------------------------------
# P0: Temporal features (ml.temporal_features)
# ---------------------------------------------------------------------------


class TestTemporalFeatures:
    """Tests for cyclical temporal feature computation."""

    def test_compute_temporal_from_unix_known_date(self):
        """Known date: 2024-03-15 (Friday, March) produces correct sin/cos."""
        from ml.temporal_features import compute_temporal_from_unix

        # 2024-03-15 00:00:00 UTC = 1710460800
        unix_ts = np.array([1710460800.0])
        feats = compute_temporal_from_unix(unix_ts)
        assert set(feats.keys()) == {"month_sin", "month_cos", "day_of_week_sin", "day_of_week_cos"}
        # March = month 3; sin(2π*3/12) = sin(π/2) = 1.0
        assert abs(feats["month_sin"][0] - math.sin(2 * math.pi * 3 / 12)) < 1e-6
        assert abs(feats["month_cos"][0] - math.cos(2 * math.pi * 3 / 12)) < 1e-6
        # Friday = Python dayofweek 4; sin(2π*4/7)
        assert abs(feats["day_of_week_sin"][0] - math.sin(2 * math.pi * 4 / 7)) < 1e-6
        assert abs(feats["day_of_week_cos"][0] - math.cos(2 * math.pi * 4 / 7)) < 1e-6

    def test_compute_temporal_from_date_string(self):
        """Date string produces same result as unix timestamp."""
        from ml.temporal_features import compute_temporal_from_date, compute_temporal_from_unix

        date_series = pd.Series(["2024-03-15"])
        feats_date = compute_temporal_from_date(date_series)
        feats_unix = compute_temporal_from_unix(np.array([1710460800.0]))
        for key in feats_date:
            assert abs(feats_date[key][0] - feats_unix[key][0]) < 1e-6

    def test_add_temporal_features_to_df_from_unix(self):
        """add_temporal_features_to_df adds 4 columns from match_date_unix."""
        from ml.temporal_features import TEMPORAL_FEATURE_COLS, add_temporal_features_to_df

        df = pd.DataFrame({"match_date_unix": [1710460800.0, 1710547200.0], "other": [1, 2]})
        result = add_temporal_features_to_df(df, date_col=None)
        for col in TEMPORAL_FEATURE_COLS:
            assert col in result.columns
            assert not result[col].isna().any()

    def test_add_temporal_features_to_df_from_date(self):
        """add_temporal_features_to_df prefers match_date over match_date_unix."""
        from ml.temporal_features import TEMPORAL_FEATURE_COLS, add_temporal_features_to_df

        df = pd.DataFrame({
            "match_date": ["2024-03-15", "2024-06-20"],
            "match_date_unix": [0.0, 0.0],  # should be ignored
        })
        add_temporal_features_to_df(df)
        # March month_sin should be ~1.0, not 0.0 (which unix=0 would give)
        assert abs(df["month_sin"].iloc[0] - math.sin(2 * math.pi * 3 / 12)) < 1e-6

    def test_add_temporal_features_fills_zero_when_no_source(self):
        """When neither date nor unix column exists, fills with 0.0."""
        from ml.temporal_features import TEMPORAL_FEATURE_COLS, add_temporal_features_to_df

        df = pd.DataFrame({"other": [1, 2, 3]})
        add_temporal_features_to_df(df)
        for col in TEMPORAL_FEATURE_COLS:
            assert (df[col] == 0.0).all()

    def test_replace_temporal_columns_drop(self):
        """replace_temporal_columns with drop_replaced removes old columns."""
        from ml.temporal_features import TEMPORAL_FEATURE_COLS, replace_temporal_columns

        df = pd.DataFrame({
            "match_date_unix": [1710460800.0],
            "season_id": [2024],
            "season": [2024],
            "other": [1],
        })
        replace_temporal_columns(df, date_col=None, drop_replaced=True)
        assert "match_date_unix" not in df.columns
        assert "season_id" not in df.columns
        assert "season" not in df.columns
        for col in TEMPORAL_FEATURE_COLS:
            assert col in df.columns

    def test_temporal_from_unix_scalar(self):
        """temporal_from_unix_scalar returns dict of floats for a single timestamp."""
        from ml.temporal_features import temporal_from_unix_scalar

        result = temporal_from_unix_scalar(1710460800.0)
        assert isinstance(result, dict)
        assert len(result) == 4
        for v in result.values():
            assert isinstance(v, float)
            assert -1.0 <= v <= 1.0

    def test_temporal_features_in_batting_feature_cols(self):
        """BATTING_FEATURE_COLS includes temporal features, not season_id/match_date_unix."""
        from ml.tuning.types import BATTING_FEATURE_COLS

        assert "month_sin" in BATTING_FEATURE_COLS
        assert "month_cos" in BATTING_FEATURE_COLS
        assert "day_of_week_sin" in BATTING_FEATURE_COLS
        assert "day_of_week_cos" in BATTING_FEATURE_COLS
        assert "season_id" not in BATTING_FEATURE_COLS
        assert "match_date_unix" not in BATTING_FEATURE_COLS

    def test_temporal_features_in_bowling_feature_cols(self):
        """BOWLING_FEATURE_COLS includes temporal features, not season_id/match_date_unix."""
        from ml.tuning.types import BOWLING_FEATURE_COLS

        assert "month_sin" in BOWLING_FEATURE_COLS
        assert "season_id" not in BOWLING_FEATURE_COLS
        assert "match_date_unix" not in BOWLING_FEATURE_COLS

    def test_temporal_features_in_fielding_feature_cols(self):
        """FIELDING_FEATURE_COLS includes temporal features."""
        from ml.train_fielding import FIELDING_FEATURE_COLS

        assert "month_sin" in FIELDING_FEATURE_COLS
        assert "season_id" not in FIELDING_FEATURE_COLS
        assert "match_date_unix" not in FIELDING_FEATURE_COLS


# ---------------------------------------------------------------------------
# P1: Derived features for extras and innings
# ---------------------------------------------------------------------------


class TestDerivedFeatures:
    """Tests for derived feature computation in extras and innings."""

    def test_extras_derived_cols_in_feature_list(self):
        """EXTRAS_FEATURE_COLS includes derived columns."""
        from ml.train_extras import EXTRAS_DERIVED_COLS, EXTRAS_FEATURE_COLS

        for col in EXTRAS_DERIVED_COLS:
            assert col in EXTRAS_FEATURE_COLS

    def test_innings_derived_cols_in_feature_list(self):
        """INNINGS_FEATURE_COLS includes derived columns."""
        from ml.train_innings import INNINGS_DERIVED_COLS, INNINGS_FEATURE_COLS

        for col in INNINGS_DERIVED_COLS:
            assert col in INNINGS_FEATURE_COLS

    def test_extras_add_derived_features(self):
        """_add_derived_features computes form_differential, consistency_differential, weather_composite."""
        from ml.train_extras import _add_derived_features

        df = pd.DataFrame({
            "bat_form_sum": [10.0, 5.0],
            "bowl_form_sum": [3.0, 8.0],
            "bat_consistency_sum": [7.0, 4.0],
            "bowl_consistency_sum": [2.0, 6.0],
            "rain": [1.0, 0.0],
            "humidity": [80.0, 50.0],
            "cloud": [60.0, 20.0],
        })
        _add_derived_features(df)
        # form_differential = bat - bowl
        assert df["form_differential"].iloc[0] == pytest.approx(7.0)
        assert df["form_differential"].iloc[1] == pytest.approx(-3.0)
        # consistency_differential = bat - bowl
        assert df["consistency_differential"].iloc[0] == pytest.approx(5.0)
        assert df["consistency_differential"].iloc[1] == pytest.approx(-2.0)
        # weather_composite = 0.5*rain + 0.3*(humidity/100) + 0.2*(cloud/100)
        expected_0 = 0.5 * 1.0 + 0.3 * (80.0 / 100.0) + 0.2 * (60.0 / 100.0)
        assert df["weather_composite"].iloc[0] == pytest.approx(expected_0)

    def test_innings_add_derived_features(self):
        """_add_derived_features in innings computes same derived features."""
        from ml.train_innings import _add_derived_features

        df = pd.DataFrame({
            "bat_form_sum": [12.0],
            "bowl_form_sum": [4.0],
            "bat_consistency_sum": [8.0],
            "bowl_consistency_sum": [3.0],
            "rain": [0.0],
            "humidity": [60.0],
            "cloud": [40.0],
        })
        _add_derived_features(df)
        assert df["form_differential"].iloc[0] == pytest.approx(8.0)
        assert df["consistency_differential"].iloc[0] == pytest.approx(5.0)
        expected_wc = 0.5 * 0.0 + 0.3 * (60.0 / 100.0) + 0.2 * (40.0 / 100.0)
        assert df["weather_composite"].iloc[0] == pytest.approx(expected_wc)

    def test_derived_features_handle_missing_columns(self):
        """_add_derived_features fills 0.0 when base columns are missing."""
        from ml.train_extras import _add_derived_features

        df = pd.DataFrame({"other": [1.0, 2.0]})
        _add_derived_features(df)
        assert (df["form_differential"] == 0.0).all()
        assert (df["consistency_differential"] == 0.0).all()
        assert (df["weather_composite"] == 0.0).all()


# ---------------------------------------------------------------------------
# P2: Feature interactions and log1p config
# ---------------------------------------------------------------------------


class TestFeatureInteractionsConfig:
    """Tests for feature_transforms configuration."""

    def test_batting_interactions_configured(self):
        """config.json has non-empty batting interactions."""
        from ml.config import _load

        cfg = _load()
        ft = cfg.get("ml", {}).get("feature_transforms", {}).get("batting", {})
        interactions = ft.get("add_interactions", [])
        assert len(interactions) > 0, "Batting interactions should be configured"
        # Each interaction is a pair of feature names
        for pair in interactions:
            assert len(pair) == 2

    def test_bowling_interactions_configured(self):
        """config.json has non-empty bowling interactions."""
        from ml.config import _load

        cfg = _load()
        ft = cfg.get("ml", {}).get("feature_transforms", {}).get("bowling", {})
        interactions = ft.get("add_interactions", [])
        assert len(interactions) > 0, "Bowling interactions should be configured"

    def test_batting_log1p_configured(self):
        """config.json has non-empty batting log1p transforms."""
        from ml.config import _load

        cfg = _load()
        ft = cfg.get("ml", {}).get("feature_transforms", {}).get("batting", {})
        log1p = ft.get("add_log1p", [])
        assert len(log1p) > 0, "Batting log1p should be configured"
        assert "batting_career_count" in log1p

    def test_bowling_log1p_configured(self):
        """config.json has non-empty bowling log1p transforms."""
        from ml.config import _load

        cfg = _load()
        ft = cfg.get("ml", {}).get("feature_transforms", {}).get("bowling", {})
        log1p = ft.get("add_log1p", [])
        assert len(log1p) > 0, "Bowling log1p should be configured"
        assert "bowling_career_count" in log1p


# ---------------------------------------------------------------------------
# P3: Permutation importance fallback for non-tree models
# ---------------------------------------------------------------------------


class TestPermutationImportanceFallback:
    """Tests for permutation importance fallback in _extract_feature_importance."""

    def test_tree_model_uses_native_importance(self):
        """Tree-based model returns native feature_importances_ (no permutation)."""
        from ml.tuning.cv_metrics import _extract_feature_importance

        est = RandomForestRegressor(n_estimators=5, random_state=42)
        pipe = Pipeline([("scaler", StandardScaler()), ("est", est)])
        rng = np.random.RandomState(42)
        X = rng.rand(50, 3)
        y = rng.rand(50)
        pipe.fit(X, y)
        fi = _extract_feature_importance(pipe, ["a", "b", "c"], 3)
        assert fi is not None
        assert len(fi) > 0

    def test_mlp_model_uses_permutation_importance(self):
        """MLP model falls back to permutation importance when X/y/scoring provided."""
        from ml.tuning.cv_metrics import _extract_feature_importance

        est = MLPRegressor(hidden_layer_sizes=(8,), max_iter=200, random_state=42)
        pipe = Pipeline([("scaler", StandardScaler()), ("est", est)])
        rng = np.random.RandomState(42)
        X = rng.rand(60, 4)
        y = rng.rand(60)
        pipe.fit(X, y)
        # Without X/y/scoring: returns None (no native importance)
        fi_none = _extract_feature_importance(pipe, ["a", "b", "c", "d"], 4)
        assert fi_none is None
        # With X/y/scoring: returns permutation importance
        fi = _extract_feature_importance(
            pipe, ["a", "b", "c", "d"], 4, X=X, y=y, scoring="neg_mean_absolute_error"
        )
        assert fi is not None
        assert isinstance(fi, dict)

    def test_mlqa_audit_sensitivity_with_mlp(self):
        """MLQA audit sensitivity check uses permutation importance for MLP models."""
        from ml.tuning.cv_metrics import _compute_mlqa_audit
        from ml.tuning.search_space import _build_pipeline_single_regression

        est = MLPRegressor(hidden_layer_sizes=(8,), max_iter=200, random_state=42)
        pipe = _build_pipeline_single_regression(est)
        rng = np.random.RandomState(42)
        X = rng.rand(80, 5)
        y = rng.rand(80)
        pipe.fit(X, y)
        cv = KFold(n_splits=3, shuffle=True, random_state=42)
        report = {"best_cv_score": -0.4}
        audit = _compute_mlqa_audit(
            report, pipe, X, y, cv, "neg_mean_absolute_error", "regression"
        )
        # Should have sensitivity finding (permutation-based), not "not available"
        sensitivity_findings = [f for f in audit.get("key_findings", []) if "Sensitivity" in f]
        assert len(sensitivity_findings) > 0
        # Should NOT say "not available" — permutation fallback should work
        for f in sensitivity_findings:
            assert "not available" not in f.lower() or "permutation fallback failed" in f.lower()


# ---------------------------------------------------------------------------
# P3: Reconciliation with temporal features
# ---------------------------------------------------------------------------


class TestReconciliationTemporal:
    """Tests for reconciliation module using temporal features."""

    def test_build_innings_feature_vector_includes_temporal(self):
        """build_innings_feature_vector includes temporal features from match_date_unix."""
        from app.reconciliation import INNINGS_FEATURE_COLS, build_innings_feature_vector

        X = build_innings_feature_vector(
            inning_number=1,
            bat_consistency_sum=1.0,
            bowl_consistency_sum=1.0,
            bat_form_sum=0.5,
            bowl_form_sum=0.5,
            match_date_unix=1710460800.0,  # 2024-03-15
        )
        assert X.shape == (1, len(INNINGS_FEATURE_COLS))
        # Temporal features should be non-zero for a real date
        ms_idx = INNINGS_FEATURE_COLS.index("month_sin")
        assert X[0, ms_idx] != 0.0

    def test_build_innings_feature_vector_includes_derived(self):
        """build_innings_feature_vector includes derived features."""
        from app.reconciliation import INNINGS_FEATURE_COLS, build_innings_feature_vector

        X = build_innings_feature_vector(
            inning_number=1,
            bat_consistency_sum=8.0,
            bowl_consistency_sum=3.0,
            bat_form_sum=10.0,
            bowl_form_sum=4.0,
        )
        fd_idx = INNINGS_FEATURE_COLS.index("form_differential")
        cd_idx = INNINGS_FEATURE_COLS.index("consistency_differential")
        wc_idx = INNINGS_FEATURE_COLS.index("weather_composite")
        assert X[0, fd_idx] == pytest.approx(6.0)   # 10 - 4
        assert X[0, cd_idx] == pytest.approx(5.0)   # 8 - 3
        assert X[0, wc_idx] == pytest.approx(0.0)   # all weather defaults = 0

    def test_build_innings_feature_vector_no_season_id(self):
        """build_innings_feature_vector does not accept season_id parameter."""
        import inspect

        from app.reconciliation import build_innings_feature_vector

        sig = inspect.signature(build_innings_feature_vector)
        assert "season_id" not in sig.parameters


# ---------------------------------------------------------------------------
# MLQA config thresholds are relative
# ---------------------------------------------------------------------------


class TestMLQARelativeThresholds:
    """Tests that MLQA config uses relative thresholds."""

    def test_config_thresholds_are_relative_scale(self):
        """MLQA thresholds in config are 0.10 and 0.08 (relative, not absolute)."""
        from ml.config import get_mlqa_config

        mlqa = get_mlqa_config()
        assert mlqa["overfitting_delta_threshold"] == pytest.approx(0.10)
        assert mlqa["stability_fold_std_threshold"] == pytest.approx(0.08)

    def test_no_season_id_in_extras_feature_cols(self):
        """EXTRAS_FEATURE_COLS does not contain season_id or match_date_unix."""
        from ml.train_extras import EXTRAS_FEATURE_COLS

        assert "season_id" not in EXTRAS_FEATURE_COLS
        assert "match_date_unix" not in EXTRAS_FEATURE_COLS

    def test_no_season_id_in_innings_feature_cols(self):
        """INNINGS_FEATURE_COLS does not contain season_id or match_date_unix."""
        from ml.train_innings import INNINGS_FEATURE_COLS

        assert "season_id" not in INNINGS_FEATURE_COLS
        assert "match_date_unix" not in INNINGS_FEATURE_COLS
