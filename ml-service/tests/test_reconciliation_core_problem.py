"""Unit tests for ml.reconciliation_core.ProblemBuilder."""

import numpy as np

from app.models import (
    BattingPrediction,
    BowlingPrediction,
    InningsReconciliationPreferences,
    MatchReconciliationInputs,
    PlayerReconciliationPreferences,
)
from ml.reconciliation_core import (
    ProblemBuilder,
    VariableKind,
)


def _make_match_inputs() -> MatchReconciliationInputs:
    """Construct a simple two-innings match; player totals match innings targets."""
    # Team1 bat runs 30+30=60, team2 bowl runs 30+30=60; team2 wickets 2+1=3
    # Team2 bat runs 35+35=70, team1 bowl runs 35+35=70; team1 wickets 1+1=2
    players = [
        PlayerReconciliationPreferences(
            player_id=1,
            team_id=100,
            batting=BattingPrediction(
                runs_scored=30.0,
                balls_faced=20.0,
                fours_scored=3.0,
                sixes_scored=1.0,
                batting_position=1.0,
                strike_rate=150.0,
            ),
            bowling=BowlingPrediction(
                runs_conceded=35.0,
                deliveries=24.0,
                wickets_taken=1.0,
                econ=8.75,
            ),
        ),
        PlayerReconciliationPreferences(
            player_id=2,
            team_id=100,
            batting=BattingPrediction(
                runs_scored=30.0,
                balls_faced=15.0,
                fours_scored=2.0,
                sixes_scored=0.0,
                batting_position=2.0,
                strike_rate=133.3,
            ),
            bowling=BowlingPrediction(
                runs_conceded=35.0,
                deliveries=12.0,
                wickets_taken=1.0,
                econ=17.5,
            ),
        ),
        PlayerReconciliationPreferences(
            player_id=3,
            team_id=200,
            batting=BattingPrediction(
                runs_scored=35.0,
                balls_faced=25.0,
                fours_scored=4.0,
                sixes_scored=1.0,
                batting_position=1.0,
                strike_rate=140.0,
            ),
            bowling=BowlingPrediction(
                runs_conceded=30.0,
                deliveries=24.0,
                wickets_taken=2.0,
                econ=7.5,
            ),
        ),
        PlayerReconciliationPreferences(
            player_id=4,
            team_id=200,
            batting=BattingPrediction(
                runs_scored=35.0,
                balls_faced=10.0,
                fours_scored=1.0,
                sixes_scored=0.0,
                batting_position=2.0,
                strike_rate=150.0,
            ),
            bowling=BowlingPrediction(
                runs_conceded=30.0,
                deliveries=24.0,
                wickets_taken=1.0,
                econ=7.5,
            ),
        ),
    ]

    innings_prefs = [
        InningsReconciliationPreferences(
            inning_number=1,
            batting_team_id=100,
            bowling_team_id=200,
            preferred_runs=60.0,
            preferred_wickets=3.0,
        ),
        InningsReconciliationPreferences(
            inning_number=2,
            batting_team_id=200,
            bowling_team_id=100,
            preferred_runs=70.0,
            preferred_wickets=2.0,
        ),
    ]

    return MatchReconciliationInputs(
        match_id=1,
        format="T20",
        players=players,
        innings=innings_prefs,
        win=None,
    )


def test_problem_builder_constructs_variables_and_mu():
    pref = _make_match_inputs()
    builder = ProblemBuilder(runs_weight=2.0, wickets_weight=3.0)

    problem = builder.build_from_match(pref, team1_id=100, team2_id=200)

    # We should have one BAT_RUNS and one BOWL_RUNS/BOWL_WKTS (and *_BALLS) per player.
    kinds = {v.kind for v in problem.variables}
    assert VariableKind.BAT_RUNS in kinds
    assert VariableKind.BOWL_RUNS in kinds
    assert VariableKind.BOWL_WKTS in kinds

    # mu and weights vectors must match number of variables
    assert problem.mu.shape == (len(problem.variables),)
    assert problem.weights.shape == (len(problem.variables),)

    # Weights must reflect config (2.0 for runs, 3.0 for wickets)
    for v, w in zip(problem.variables, problem.weights):
        if v.kind in {VariableKind.BAT_RUNS, VariableKind.BAT_BALLS, VariableKind.BOWL_RUNS, VariableKind.BOWL_BALLS}:
            assert w == 2.0
        elif v.kind == VariableKind.BOWL_WKTS:
            assert w == 3.0


def test_problem_builder_adds_runs_and_wickets_constraints():
    pref = _make_match_inputs()
    builder = ProblemBuilder()
    problem = builder.build_from_match(pref, team1_id=100, team2_id=200)

    # Expect up to 6 constraints as described in the docstring
    assert 1 <= len(problem.constraints) <= 6

    # Helper to evaluate a constraint at μ (checks if preferred values already satisfy it)
    def _constraint_residual_at_mu(idx: int) -> float:
        c = problem.constraints[idx]
        lhs = sum(c.coefficients[i] * problem.mu[i] for i in c.coefficients.keys())
        return lhs - c.rhs

    # Since μ comes directly from model outputs and innings totals are constructed
    # from those same numbers in _make_match_inputs, the residual at μ should be ~0.
    residuals = np.array([_constraint_residual_at_mu(i) for i in range(len(problem.constraints))])
    assert np.allclose(residuals, 0.0)


def test_problem_builder_soft_favor_top_order_balls_scales_weights():
    """With soft_favor_top_order_balls > 0, BAT_BALLS weight is higher for top-order."""
    pref = _make_match_inputs()
    builder_none = ProblemBuilder(soft_favor_top_order_balls=0.0)
    builder_soft = ProblemBuilder(soft_favor_top_order_balls=1.0)

    problem_none = builder_none.build_from_match(pref, team1_id=100, team2_id=200)
    problem_soft = builder_soft.build_from_match(pref, team1_id=100, team2_id=200)

    # Find BAT_BALLS indices and their weights (by player order in pref: 1=pos1, 2=pos2, 3=pos1, 4=pos2)
    def get_ball_weights(prob):
        return [(v.player_id, prob.weights[v.index]) for v in prob.variables if v.kind == VariableKind.BAT_BALLS]

    weights_none = dict(get_ball_weights(problem_none))
    weights_soft = dict(get_ball_weights(problem_soft))

    # With soft=0, all BAT_BALLS weights are equal (runs_weight)
    assert len(weights_none) >= 2
    for w in weights_none.values():
        assert w == 1.0

    # With soft=1, position-1 batsmen (players 1, 3) should have higher weight than position-2 (2, 4)
    assert weights_soft[1] > weights_soft[2]
    assert weights_soft[3] > weights_soft[4]
