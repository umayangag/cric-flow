"""Tests for ml.win_features: enhanced win feature definitions and aggregation."""

from __future__ import annotations

import math

import pytest

from ml.win_features import (
    _DIST_FEATURE_COLS,
    DERIVED_FEATURE_COLS,
    MATCH_CONTEXT_COLS,
    WIN_ENHANCED_FEATURE_COLS,
    _dist_stats_from_values,
    aggregate_team_features_from_player_maps,
    build_feature_vector,
    compute_derived_features,
    format_one_hot_from_code,
    get_format_codes,
    get_format_one_hot_columns,
)


class TestDistStatsFromValues:
    def test_empty_values_returns_zeros(self) -> None:
        stats = _dist_stats_from_values([])
        assert stats["sum"] == 0.0
        assert stats["mean"] == 0.0
        assert stats["count"] == 0.0

    def test_single_value(self) -> None:
        stats = _dist_stats_from_values([5.0])
        assert stats["sum"] == 5.0
        assert stats["mean"] == 5.0
        assert stats["std"] == 0.0
        assert stats["max"] == 5.0
        assert stats["min"] == 5.0
        assert stats["top3_mean"] == 5.0
        assert stats["count"] == 1.0

    def test_multiple_values_computes_correct_stats(self) -> None:
        values = [10.0, 20.0, 30.0, 40.0, 50.0]
        stats = _dist_stats_from_values(values)
        assert stats["sum"] == 150.0
        assert stats["mean"] == 30.0
        assert stats["max"] == 50.0
        assert stats["min"] == 10.0
        assert stats["count"] == 5.0
        assert stats["top3_mean"] == pytest.approx(40.0)
        assert stats["std"] > 0

    def test_top3_mean_with_fewer_than_three(self) -> None:
        stats = _dist_stats_from_values([7.0, 3.0])
        assert stats["top3_mean"] == pytest.approx(5.0)


class TestComputeDerivedFeatures:
    def test_returns_all_derived_columns(self) -> None:
        row = {col: 1.0 for col in MATCH_CONTEXT_COLS + _DIST_FEATURE_COLS}
        derived = compute_derived_features(row)
        for col in DERIVED_FEATURE_COLS:
            assert col in derived, f"missing derived feature: {col}"

    def test_matchup_ratio_safe_division(self) -> None:
        row = {col: 0.0 for col in MATCH_CONTEXT_COLS + _DIST_FEATURE_COLS}
        derived = compute_derived_features(row)
        assert derived["bat_form_matchup_ratio_team1"] == 1.0
        assert derived["bat_cons_matchup_ratio_team2"] == 1.0

    def test_bowl_depth_diff(self) -> None:
        row = {col: 0.0 for col in MATCH_CONTEXT_COLS + _DIST_FEATURE_COLS}
        row["team1_bowl_consistency_count"] = 5.0
        row["team2_bowl_consistency_count"] = 3.0
        derived = compute_derived_features(row)
        assert derived["bowl_depth_diff"] == 2.0

    def test_spread_calculation(self) -> None:
        row = {col: 0.0 for col in MATCH_CONTEXT_COLS + _DIST_FEATURE_COLS}
        row["team1_bat_form_max"] = 10.0
        row["team1_bat_form_min"] = 2.0
        derived = compute_derived_features(row)
        assert derived["team1_bat_form_spread"] == 8.0


class TestAggregateTeamFeatures:
    def test_produces_all_enhanced_feature_columns(self) -> None:
        team1 = {
            1: {"batting_consistency": 80.0, "bowling_consistency": 20.0, "batting_form": 70.0, "bowling_form": 15.0},
            2: {"batting_consistency": 60.0, "bowling_consistency": 30.0, "batting_form": 50.0, "bowling_form": 25.0},
        }
        team2 = {
            3: {"batting_consistency": 90.0, "bowling_consistency": 10.0, "batting_form": 85.0, "bowling_form": 5.0},
        }
        ctx = {col: 1.0 for col in MATCH_CONTEXT_COLS}
        result = aggregate_team_features_from_player_maps(team1, team2, ctx)
        for col in WIN_ENHANCED_FEATURE_COLS:
            assert col in result, f"missing feature: {col}"
            assert not math.isnan(result[col]), f"NaN in feature: {col}"

    def test_match_context_passed_through(self) -> None:
        ctx = {"format_id": 42.0, "venue_id": 7.0}
        result = aggregate_team_features_from_player_maps({}, {}, ctx)
        assert result["format_id"] == 42.0
        assert result["venue_id"] == 7.0

    def test_empty_teams_produce_zero_stats(self) -> None:
        ctx = {col: 0.0 for col in MATCH_CONTEXT_COLS}
        result = aggregate_team_features_from_player_maps({}, {}, ctx)
        assert result["team1_bat_consistency_sum"] == 0.0
        assert result["team1_bat_consistency_count"] == 0.0


class TestBuildFeatureVector:
    def test_length_matches_column_count(self) -> None:
        feature_dict = {col: float(i) for i, col in enumerate(WIN_ENHANCED_FEATURE_COLS)}
        vec = build_feature_vector(feature_dict)
        assert len(vec) == len(WIN_ENHANCED_FEATURE_COLS)

    def test_missing_keys_default_to_zero(self) -> None:
        vec = build_feature_vector({})
        assert all(v == 0.0 for v in vec)

    def test_preserves_order(self) -> None:
        feature_dict = {col: float(i) for i, col in enumerate(WIN_ENHANCED_FEATURE_COLS)}
        vec = build_feature_vector(feature_dict)
        for i, col in enumerate(WIN_ENHANCED_FEATURE_COLS):
            assert vec[i] == float(i), f"wrong value at index {i} ({col})"


class TestFormatOneHotPublicApi:
    def test_get_format_one_hot_columns_matches_codes_plus_other(self) -> None:
        codes = get_format_codes()
        cols = get_format_one_hot_columns()
        assert cols == [f"format_is_{code}" for code in codes] + ["format_is_OTHER"]

    def test_format_one_hot_from_code_known_format(self) -> None:
        codes = get_format_codes()
        one_hot = format_one_hot_from_code(codes[0])
        assert one_hot[f"format_is_{codes[0]}"] == 1.0
        assert sum(one_hot.values()) == 1.0

    def test_format_one_hot_from_code_unknown_uses_other(self) -> None:
        one_hot = format_one_hot_from_code("NOT_A_REAL_FORMAT_XYZ")
        assert one_hot["format_is_OTHER"] == 1.0
        assert sum(one_hot.values()) == 1.0


class TestFeatureColumnDefinitions:
    def test_no_duplicate_feature_columns(self) -> None:
        assert len(WIN_ENHANCED_FEATURE_COLS) == len(set(WIN_ENHANCED_FEATURE_COLS))

    def test_dist_feature_count(self) -> None:
        assert len(_DIST_FEATURE_COLS) == 8 * 7

    def test_derived_feature_count(self) -> None:
        assert len(DERIVED_FEATURE_COLS) == 13
