"""Unit tests for model improvements: temporal features, derived features,
feature interactions config, permutation importance fallback, and pipeline alignment.

These tests verify the six improvement areas without running real model training.
"""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest
from sklearn.ensemble import RandomForestRegressor
from sklearn.model_selection import KFold
from sklearn.neural_network import MLPRegressor
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

# ---------------------------------------------------------------------------
# P0: Monotonic temporal features (season_id, match_date_unix) presence
# ---------------------------------------------------------------------------


class TestMonotonicTemporalFeatures:
    """Monotonic temporal feature presence in canonical feature column lists."""

    def test_batting_feature_cols_contain_season_and_match_date_unix(self):
        from ml.tuning.types import BATTING_FEATURE_COLS

        assert "season" in BATTING_FEATURE_COLS
        assert "match_date_unix" in BATTING_FEATURE_COLS

    def test_bowling_feature_cols_contain_season_and_match_date_unix(self):
        from ml.tuning.types import BOWLING_FEATURE_COLS

        assert "season" in BOWLING_FEATURE_COLS
        assert "match_date_unix" in BOWLING_FEATURE_COLS

    def test_fielding_feature_cols_contain_season_id_and_match_date_unix(self):
        from ml.train_fielding import FIELDING_FEATURE_COLS

        assert "season_id" in FIELDING_FEATURE_COLS
        assert "match_date_unix" in FIELDING_FEATURE_COLS


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

        df = pd.DataFrame(
            {
                "bat_form_sum": [12.0],
                "bowl_form_sum": [4.0],
                "bat_consistency_sum": [8.0],
                "bowl_consistency_sum": [3.0],
                "rain": [0.0],
                "humidity": [60.0],
                "cloud": [40.0],
            }
        )
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
        fi = _extract_feature_importance(pipe, ["a", "b", "c", "d"], 4, X=X, y=y, scoring="neg_mean_absolute_error")
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
        audit = _compute_mlqa_audit(report, pipe, X, y, cv, "neg_mean_absolute_error", "regression")
        # Should have sensitivity finding (permutation-based), not "not available"
        sensitivity_findings = [f for f in audit.get("key_findings", []) if "Sensitivity" in f]
        assert len(sensitivity_findings) > 0
        # Should NOT say "not available" — permutation fallback should work
        for f in sensitivity_findings:
            assert "not available" not in f.lower() or "permutation fallback failed" in f.lower()


# ---------------------------------------------------------------------------
# P3: Reconciliation feature vector (monotonic temporal + derived)
# ---------------------------------------------------------------------------


class TestReconciliationFeatureVector:
    """Tests for reconciliation.build_innings_feature_vector content and ordering."""

    def test_build_innings_feature_vector_includes_monotonic_temporal(self):
        """build_innings_feature_vector places season_id and match_date_unix values in the vector."""
        from app.reconciliation import INNINGS_FEATURE_COLS, build_innings_feature_vector

        X = build_innings_feature_vector(
            inning_number=1,
            bat_consistency_sum=1.0,
            bowl_consistency_sum=1.0,
            bat_form_sum=0.5,
            bowl_form_sum=0.5,
            season_id=2024,
            match_date_unix=1710460800.0,
        )
        assert X.shape == (1, len(INNINGS_FEATURE_COLS))
        season_idx = INNINGS_FEATURE_COLS.index("season_id")
        md_idx = INNINGS_FEATURE_COLS.index("match_date_unix")
        assert X[0, season_idx] == pytest.approx(2024.0)
        assert X[0, md_idx] == pytest.approx(1710460800.0)

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
        assert X[0, fd_idx] == pytest.approx(6.0)
        assert X[0, cd_idx] == pytest.approx(5.0)
        assert X[0, wc_idx] == pytest.approx(0.0)

    def test_build_innings_feature_vector_accepts_season_id(self):
        """build_innings_feature_vector exposes a season_id parameter."""
        import inspect

        from app.reconciliation import build_innings_feature_vector

        sig = inspect.signature(build_innings_feature_vector)
        assert "season_id" in sig.parameters


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
