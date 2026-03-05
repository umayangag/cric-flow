"""Tests for ml.reconciliation_solver.solve_reconciliation_problem."""

import numpy as np

from app.models import (
    BattingPrediction,
    BowlingPrediction,
    InningsReconciliationPreferences,
    MatchReconciliationInputs,
    PlayerReconciliationPreferences,
)
from ml.reconciliation_core import ProblemBuilder
from ml.reconciliation_solver import solve_reconciliation_problem


def _make_simple_problem():
    """Reuse the same synthetic setup as in test_reconciliation_core_problem."""
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
                runs_conceded=40.0,
                deliveries=24.0,
                wickets_taken=1.0,
                econ=10.0,
            ),
        ),
        PlayerReconciliationPreferences(
            player_id=2,
            team_id=100,
            batting=BattingPrediction(
                runs_scored=20.0,
                balls_faced=15.0,
                fours_scored=2.0,
                sixes_scored=0.0,
                batting_position=2.0,
                strike_rate=133.3,
            ),
            bowling=BowlingPrediction(
                runs_conceded=20.0,
                deliveries=12.0,
                wickets_taken=2.0,
                econ=10.0,
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
                runs_scored=15.0,
                balls_faced=10.0,
                fours_scored=1.0,
                sixes_scored=0.0,
                batting_position=2.0,
                strike_rate=150.0,
            ),
            bowling=BowlingPrediction(
                runs_conceded=35.0,
                deliveries=24.0,
                wickets_taken=1.0,
                econ=8.75,
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

    pref = MatchReconciliationInputs(
        match_id=1,
        format="T20",
        players=players,
        innings=innings_prefs,
        win=None,
    )
    builder = ProblemBuilder()
    problem = builder.build_from_match(pref, team1_id=100, team2_id=200)
    return problem


def test_solver_respects_constraints_and_stays_close_to_mu():
    problem = _make_simple_problem()
    x = solve_reconciliation_problem(problem)

    # 1) Check that constraints are satisfied (A x == b within tolerance).
    residuals = []
    for c in problem.constraints:
        lhs = sum(coef * x[idx] for idx, coef in c.coefficients.items())
        residuals.append(lhs - c.rhs)
    residuals = np.asarray(residuals, dtype=float)
    assert np.allclose(residuals, 0.0, atol=1e-6)

    # 2) Check that x is close to mu (objective not exploding).
    delta = x - problem.mu
    # Weighted squared error should be small compared to magnitude of μ.
    obj = np.sum(problem.weights * delta * delta)
    baseline = np.sum(problem.weights * problem.mu * problem.mu) + 1e-9
    assert obj < 0.1 * baseline


def test_solver_handles_no_constraints_case():
    problem = _make_simple_problem()
    # Remove all constraints
    problem.constraints.clear()
    x = solve_reconciliation_problem(problem)
    assert np.allclose(x, problem.mu)
