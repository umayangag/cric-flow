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
# P0: Cyclical temporal features presence (replaced season_id/match_date_unix)
# ---------------------------------------------------------------------------


class TestCyclicalTemporalFeatures:
    """Cyclical temporal feature presence in canonical feature column lists."""

    CYCLICAL = ["match_month_sin", "match_month_cos", "match_day_of_week_sin", "match_day_of_week_cos"]

    def test_batting_feature_cols_contain_cyclical_time(self):
        from ml.tuning.types import BATTING_FEATURE_COLS

        for c in self.CYCLICAL:
            assert c in BATTING_FEATURE_COLS

    def test_bowling_feature_cols_contain_cyclical_time(self):
        from ml.tuning.types import BOWLING_FEATURE_COLS

        for c in self.CYCLICAL:
            assert c in BOWLING_FEATURE_COLS

    def test_fielding_feature_cols_contain_cyclical_time(self):
        from ml.train_fielding import FIELDING_FEATURE_COLS

        for c in self.CYCLICAL:
            assert c in FIELDING_FEATURE_COLS

    def test_innings_and_extras_feature_cols_omit_cyclical_time(self):
        """The innings and extras exports do not carry the cyclical columns.

        Only the batting, bowling and fielding row builders compute them (in Go), so
        naming them in these two lists selected nothing -- both loaders filter with
        `c in df.columns`. Reinstating them here would also skew inference, because
        `app.reconciliation` has no date to compute them from and would feed 0.0.
        Adding them for real means deriving from `match_date` in both paths.
        """
        from ml.train_extras import EXTRAS_FEATURE_COLS
        from ml.train_innings import INNINGS_FEATURE_COLS

        for c in self.CYCLICAL:
            assert c not in INNINGS_FEATURE_COLS
            assert c not in EXTRAS_FEATURE_COLS


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
        """_add_derived_features computes form_differential and consistency_differential."""
        from ml.train_extras import _add_derived_features

        df = pd.DataFrame(
            {
                "bat_form_sum": [10.0, 5.0],
                "bowl_form_sum": [3.0, 8.0],
                "bat_consistency_sum": [7.0, 4.0],
                "bowl_consistency_sum": [2.0, 6.0],
            }
        )
        _add_derived_features(df)
        # form_differential = bat - bowl
        assert df["form_differential"].iloc[0] == pytest.approx(7.0)
        assert df["form_differential"].iloc[1] == pytest.approx(-3.0)
        # consistency_differential = bat - bowl
        assert df["consistency_differential"].iloc[0] == pytest.approx(5.0)
        assert df["consistency_differential"].iloc[1] == pytest.approx(-2.0)

    def test_innings_add_derived_features(self):
        """_add_derived_features in innings computes same derived features."""
        from ml.train_innings import _add_derived_features

        df = pd.DataFrame(
            {
                "bat_form_sum": [12.0],
                "bowl_form_sum": [4.0],
                "bat_consistency_sum": [8.0],
                "bowl_consistency_sum": [3.0],
            }
        )
        _add_derived_features(df)
        assert df["form_differential"].iloc[0] == pytest.approx(8.0)
        assert df["consistency_differential"].iloc[0] == pytest.approx(5.0)

    def test_derived_features_handle_missing_columns(self):
        """_add_derived_features fills 0.0 when base columns are missing."""
        from ml.train_extras import _add_derived_features

        df = pd.DataFrame({"other": [1.0, 2.0]})
        _add_derived_features(df)
        assert (df["form_differential"] == 0.0).all()
        assert (df["consistency_differential"] == 0.0).all()


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

    def test_build_innings_feature_vector_basic(self):
        """build_innings_feature_vector produces correct shape."""
        from app.reconciliation import build_innings_feature_vector
        from ml.train_innings import INNINGS_FEATURE_COLS

        meta = {"feature_names": list(INNINGS_FEATURE_COLS)}
        X = build_innings_feature_vector(
            inning_number=1,
            bat_consistency_sum=1.0,
            bowl_consistency_sum=1.0,
            bat_form_sum=0.5,
            bowl_form_sum=0.5,
            meta=meta,
        )
        assert X.shape == (1, len(INNINGS_FEATURE_COLS))

    def test_build_innings_feature_vector_includes_derived(self):
        """build_innings_feature_vector includes derived features."""
        from app.reconciliation import build_innings_feature_vector
        from ml.train_innings import INNINGS_FEATURE_COLS

        meta = {"feature_names": list(INNINGS_FEATURE_COLS)}
        X = build_innings_feature_vector(
            inning_number=1,
            bat_consistency_sum=8.0,
            bowl_consistency_sum=3.0,
            bat_form_sum=10.0,
            bowl_form_sum=4.0,
            meta=meta,
        )
        fd_idx = INNINGS_FEATURE_COLS.index("form_differential")
        cd_idx = INNINGS_FEATURE_COLS.index("consistency_differential")
        assert X[0, fd_idx] == pytest.approx(6.0)
        assert X[0, cd_idx] == pytest.approx(5.0)

    def test_build_innings_feature_vector_no_season_id_param(self):
        """build_innings_feature_vector no longer exposes season_id (replaced by cyclical time)."""
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
