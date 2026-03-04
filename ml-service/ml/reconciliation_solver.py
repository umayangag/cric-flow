"""Numerical solver for reconciliation problems.

Given a `ReconciliationProblem` (variables, preferred values μ, weights W, and
linear equality constraints), this module computes an adjusted vector x* that:

- Minimizes  \sum_i w_i (x_i - μ_i)^2
- Subject to A x = b  (encoded via LinearConstraint objects)

This is a small quadratic program with diagonal Hessian; we solve it in closed
form using Lagrange multipliers:

    x* = μ - W^{-1} A^T λ
    where λ solves (A W^{-1} A^T) λ = A μ - b

For numerical robustness:
- We clamp very small weights to a minimum positive value.
- If the constraint system is singular or ill-conditioned, we fall back to the
  closest solution in a least-squares sense.
"""

from __future__ import annotations

from typing import Tuple

import numpy as np

from .reconciliation_core import ConstraintKind, ReconciliationProblem


def _build_constraint_matrix(
    problem: ReconciliationProblem,
) -> Tuple[np.ndarray, np.ndarray]:
    """Build (A, b) from problem.constraints.

    Returns:
        A: (m, n) matrix, where m = number of constraints, n = number of vars.
        b: (m,) right-hand side vector.
    """
    n_vars = len(problem.variables)
    m = len(problem.constraints)
    if m == 0:
        return np.empty((0, n_vars), dtype=float), np.empty((0,), dtype=float)

    A = np.zeros((m, n_vars), dtype=float)
    b = np.zeros((m,), dtype=float)

    for row, c in enumerate(problem.constraints):
        for idx, coef in c.coefficients.items():
            if 0 <= idx < n_vars:
                A[row, idx] = coef
        b[row] = c.rhs

    return A, b


def solve_reconciliation_problem(
    problem: ReconciliationProblem,
    min_weight: float = 1e-6,
) -> np.ndarray:
    """Solve the constrained least-squares reconciliation problem.

    Args:
        problem: ReconciliationProblem with variables, μ, weights, constraints.
        min_weight: Minimum allowed weight when inverting W (prevents division
            by zero when some weights are accidentally set to 0).

    Returns:
        x: Optimal variable vector (shape (n_vars,)). If there are no
           constraints, this is simply μ. On numerical failure, returns μ.
    """
    mu = np.asarray(problem.mu, dtype=float)
    w = np.asarray(problem.weights, dtype=float)

    n_vars = mu.shape[0]
    if w.shape != (n_vars,):
        raise ValueError("weights must have same length as mu")

    # Only hard constraints enter the linear system; soft constraints are not enforced here.
    hard_constraints = [c for c in problem.constraints if c.kind == ConstraintKind.HARD]
    if not hard_constraints:
        return mu.copy()

    A, b = _build_constraint_matrix(
        ReconciliationProblem(
            variables=problem.variables,
            mu=problem.mu,
            weights=problem.weights,
            constraints=hard_constraints,
        )
    )
    if A.size == 0:
        return mu.copy()

    # Invert W (diagonal) safely.
    w_safe = np.maximum(w, float(min_weight))
    inv_w = 1.0 / w_safe

    # Compute matrices for λ system: (A W^{-1} A^T) λ = A μ - b
    # Shape notes:
    #   A: (m, n), inv_w: (n,), so A * inv_w is broadcasting along columns.
    A_scaled = A * inv_w  # (m, n)
    M = A_scaled @ A.T  # (m, m)
    rhs = A @ mu - b  # (m,)

    # Solve for λ, with robust fallbacks.
    try:
        # Preferred path: exact solve.
        lamb = np.linalg.solve(M, rhs)
    except np.linalg.LinAlgError:
        # Fallback: least-squares solution if M is singular.
        lamb, *_ = np.linalg.lstsq(M, rhs, rcond=None)

    # Recover x* = μ - W^{-1} A^T λ
    x = mu - inv_w * (A.T @ lamb)
    return x
