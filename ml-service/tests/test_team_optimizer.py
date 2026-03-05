"""Tests for ml.team_optimizer — server-side hill-climb team selection."""

from __future__ import annotations

import numpy as np
import pytest

from ml.team_optimizer import (
    OptimizationResult,
    PoolPlayer,
    ScoreWeights,
    SelectionConstraints,
    _batch_evaluate_candidates,
    _clamp01,
    _compute_derived_features_batch,
    _evaluate_single_team,
    _precompute_fixed_team_stats,
    _satisfies_constraints,
    _score_player,
    greedy_select,
    optimize_team_by_win_probability,
)
from ml.win_features import WIN_ENHANCED_FEATURE_COLS


def _make_player(
    pid: int,
    name: str,
    *,
    is_bowler: bool = False,
    is_keeper: bool = False,
    bat_score: float = 0.5,
    bowl_score: float = 0.3,
    bat_cons: float = 0.5,
    bowl_cons: float = 0.3,
    bat_form: float = 0.5,
    bowl_form: float = 0.3,
) -> PoolPlayer:
    return PoolPlayer(
        player_id=pid,
        name=name,
        is_bowler=is_bowler,
        is_keeper=is_keeper,
        bat_score=bat_score,
        bowl_score=bowl_score,
        field_score=0.0,
        features={
            "batting_consistency": bat_cons,
            "bowling_consistency": bowl_cons,
            "batting_form": bat_form,
            "bowling_form": bowl_form,
        },
    )


def _make_pool(n: int = 15, min_bowlers: int = 6, keepers: int = 1) -> list[PoolPlayer]:
    """Build a deterministic pool with enough bowlers and keepers."""
    pool: list[PoolPlayer] = []
    for i in range(n):
        pool.append(
            _make_player(
                pid=100 + i,
                name=f"Player_{i:02d}",
                is_bowler=i < min_bowlers,
                is_keeper=i < keepers,
                bat_score=0.9 - i * 0.04,
                bowl_score=0.8 - i * 0.03 if i < min_bowlers else 0.1,
                bat_cons=0.7 - i * 0.02,
                bowl_cons=0.6 - i * 0.02 if i < min_bowlers else 0.1,
                bat_form=0.8 - i * 0.03,
                bowl_form=0.7 - i * 0.03 if i < min_bowlers else 0.1,
            )
        )
    return pool


class _FakeModel:
    """Minimal sklearn-like classifier that returns higher probability when
    team1_bat_form_mean (a specific column) is higher."""

    classes_ = np.array([0, 1])

    def predict_proba(self, X: np.ndarray) -> np.ndarray:
        bat_form_mean_idx = WIN_ENHANCED_FEATURE_COLS.index("team1_bat_form_mean")
        signal = X[:, bat_form_mean_idx]
        p = np.clip(0.3 + signal * 0.5, 0.01, 0.99)
        return np.column_stack([1 - p, p])


# ---------------------------------------------------------------------------
# Unit tests
# ---------------------------------------------------------------------------


class TestScorePlayer:
    def test_basic_scoring(self) -> None:
        p = _make_player(1, "A", bat_score=0.8, bowl_score=0.6)
        w = ScoreWeights()
        score = _score_player(p, w)
        expected = 0.45 * 0.8 + 0.40 * 0.6
        assert abs(score - expected) < 1e-9

    def test_keeper_bonus(self) -> None:
        p = _make_player(1, "A", is_keeper=True, bat_score=0.5, bowl_score=0.5)
        w = ScoreWeights()
        without_bonus = 0.45 * 0.5 + 0.40 * 0.5
        assert _score_player(p, w) == pytest.approx(without_bonus + w.keeper_bonus)

    def test_clamp01(self) -> None:
        assert _clamp01(-0.5) == 0.0
        assert _clamp01(1.5) == 1.0
        assert _clamp01(0.7) == 0.7


class TestSatisfiesConstraints:
    def test_valid(self) -> None:
        team = [
            _make_player(1, "A", is_keeper=True, is_bowler=True),
            _make_player(2, "B", is_bowler=True),
        ]
        assert _satisfies_constraints(team, SelectionConstraints(size=2, min_bowlers=2, require_keeper=True))

    def test_missing_keeper(self) -> None:
        team = [_make_player(1, "A"), _make_player(2, "B")]
        assert not _satisfies_constraints(team, SelectionConstraints(size=2, min_bowlers=0, require_keeper=True))

    def test_insufficient_bowlers(self) -> None:
        team = [_make_player(1, "A", is_keeper=True), _make_player(2, "B")]
        assert not _satisfies_constraints(team, SelectionConstraints(size=2, min_bowlers=1, require_keeper=True))


class TestGreedySelect:
    def test_basic_selection(self) -> None:
        pool = _make_pool(n=15, min_bowlers=6, keepers=1)
        constraints = SelectionConstraints(size=11, min_bowlers=5, require_keeper=True)
        team = greedy_select(pool, ScoreWeights(), constraints)
        assert len(team) == 11
        assert _satisfies_constraints(team, constraints)

    def test_insufficient_pool_raises(self) -> None:
        pool = _make_pool(n=5)
        with pytest.raises(ValueError, match="insufficient pool size"):
            greedy_select(pool, ScoreWeights(), SelectionConstraints(size=11))

    def test_no_keeper_raises(self) -> None:
        pool = [_make_player(i, f"P{i}", is_bowler=True) for i in range(11)]
        with pytest.raises(ValueError, match="no keeper available"):
            greedy_select(pool, ScoreWeights(), SelectionConstraints(size=11, min_bowlers=5, require_keeper=True))


class TestPrecomputeFixedTeamStats:
    def test_produces_expected_keys(self) -> None:
        features = {
            1: {"batting_consistency": 0.8, "bowling_consistency": 0.5, "batting_form": 0.9, "bowling_form": 0.4},
            2: {"batting_consistency": 0.6, "bowling_consistency": 0.7, "batting_form": 0.7, "bowling_form": 0.6},
        }
        stats = _precompute_fixed_team_stats(features, team_number=2)
        assert "team2_bat_consistency_mean" in stats
        assert "team2_bowl_form_sum" in stats
        assert stats["team2_bat_consistency_mean"] == pytest.approx(0.7)

    def test_empty_features(self) -> None:
        stats = _precompute_fixed_team_stats({}, team_number=1)
        assert stats.get("team1_bat_consistency_mean", 0.0) == 0.0


class TestBatchEvaluateCandidates:
    def test_returns_correct_shape(self) -> None:
        pool = _make_pool(n=15, min_bowlers=6, keepers=1)
        team = pool[:11]
        candidates = pool[11:]
        opponent_feats = {p.player_id: p.features for p in pool[:5]}
        opponent_stats = _precompute_fixed_team_stats(opponent_feats, team_number=2)
        match_ctx = {col: 0.0 for col in WIN_ENHANCED_FEATURE_COLS[:13]}

        probas = _batch_evaluate_candidates(
            team, 0, candidates, opponent_stats, match_ctx, True, _FakeModel()
        )
        assert probas.shape == (len(candidates),)
        assert all(0.0 <= p <= 1.0 for p in probas)


class TestOptimizeTeamByWinProbability:
    def test_returns_valid_result(self) -> None:
        pool = _make_pool(n=15, min_bowlers=6, keepers=1)
        opponent_features = {p.player_id: p.features for p in pool[:5]}
        match_context = {col: 0.0 for col in WIN_ENHANCED_FEATURE_COLS[:13]}
        constraints = SelectionConstraints(size=11, min_bowlers=5, require_keeper=True)

        result = optimize_team_by_win_probability(
            pool=pool,
            opponent_features=opponent_features,
            match_context=match_context,
            constraints=constraints,
            weights=ScoreWeights(),
            team_is_team1=True,
            model=_FakeModel(),
            max_iterations=5,
            max_evals=100,
        )
        assert isinstance(result, OptimizationResult)
        assert len(result.selected) == 11
        assert 0.0 <= result.win_probability <= 1.0
        assert result.iterations_used >= 1
        assert result.evals_performed >= 1
        assert _satisfies_constraints(result.selected, constraints)
        names = [p.name for p in result.selected]
        assert names == sorted(names), "Selected team should be sorted by name"

    def test_team_is_team2(self) -> None:
        pool = _make_pool(n=15, min_bowlers=6, keepers=1)
        opponent_features = {p.player_id: p.features for p in pool[:5]}
        match_context = {col: 0.0 for col in WIN_ENHANCED_FEATURE_COLS[:13]}
        constraints = SelectionConstraints(size=11, min_bowlers=5, require_keeper=True)

        result = optimize_team_by_win_probability(
            pool=pool,
            opponent_features=opponent_features,
            match_context=match_context,
            constraints=constraints,
            weights=ScoreWeights(),
            team_is_team1=False,
            model=_FakeModel(),
            max_iterations=5,
            max_evals=100,
        )
        assert len(result.selected) == 11
        assert 0.0 <= result.win_probability <= 1.0

    def test_respects_eval_budget(self) -> None:
        pool = _make_pool(n=15, min_bowlers=6, keepers=1)
        opponent_features = {p.player_id: p.features for p in pool[:5]}
        match_context = {col: 0.0 for col in WIN_ENHANCED_FEATURE_COLS[:13]}
        constraints = SelectionConstraints(size=11, min_bowlers=5, require_keeper=True)

        result = optimize_team_by_win_probability(
            pool=pool,
            opponent_features=opponent_features,
            match_context=match_context,
            constraints=constraints,
            weights=ScoreWeights(),
            team_is_team1=True,
            model=_FakeModel(),
            max_iterations=100,
            max_evals=5,
        )
        assert result.evals_performed <= 5 + 11  # budget + one batch can slightly overshoot
