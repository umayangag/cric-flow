"""Logistic regression whose coefficients are bounded by a sign contract (FEAT-14).

The selection objective is additive, and an additive surface is monotone in a column
exactly when that column's coefficient has the contract's sign. Nothing about an
unconstrained fit gives it that sign: on the batch-2 rows the served T20I objective carried
a *negative* own-side weight on ``pelo_mean``, and the optimiser's surface preferred the
lower-Elo of two otherwise-equal low-involvement players. This estimator fits the same
L2-penalised log-loss ``LogisticRegression(C)`` minimises -- so with every sign free it is
that model to solver tolerance -- under a box on each coefficient: ``+1`` keeps it at or
above zero, ``-1`` at or below, ``0`` leaves it free. Fitted with L-BFGS-B, whose bounds are
exact rather than penalised. The intercept is never bounded or penalised.

The bounds are read in whatever units the design has. A sign survives any positive
rescaling, so the pipeline's ``StandardScaler`` in front changes nothing about what they
mean: a coefficient that is non-negative on the standardised column is non-negative on the
raw one.
"""

from __future__ import annotations

import logging
from typing import List, Optional, Sequence, Tuple

import numpy as np
from scipy.optimize import minimize
from scipy.special import expit
from sklearn.base import BaseEstimator, ClassifierMixin

logger = logging.getLogger(__name__)

#: The two classes this estimator knows; ``y`` must be 0 / 1.
_CLASSES = np.asarray([0.0, 1.0])


def coefficient_bounds(signs: Sequence[int]) -> List[Tuple[Optional[float], Optional[float]]]:
    """One (lower, upper) pair per column from a +1 / -1 / 0 sign contract."""
    bounds: List[Tuple[Optional[float], Optional[float]]] = []
    for sign in signs:
        if sign > 0:
            bounds.append((0.0, None))
        elif sign < 0:
            bounds.append((None, 0.0))
        else:
            bounds.append((None, None))
    return bounds


class SignedLogisticRegression(BaseEstimator, ClassifierMixin):
    """L2-penalised logistic regression with a sign bound per coefficient.

    ``signs`` is one entry per input column, +1 / -1 / 0. ``C`` is sklearn's inverse
    regularisation strength: the objective is ``0.5 * ||w||^2 + C * sum(log-loss)``, the
    intercept unpenalised, which is what ``LogisticRegression(C=C)`` minimises.
    """

    def __init__(self, signs: Sequence[int], C: float = 1.0, max_iter: int = 3000, tol: float = 1e-6):
        self.signs = signs
        self.C = C
        self.max_iter = max_iter
        #: The projected-gradient tolerance L-BFGS-B stops at.
        self.tol = tol

    def fit(self, X, y) -> "SignedLogisticRegression":
        x = np.asarray(X, dtype=float)
        target = np.asarray(y, dtype=float)
        n_columns = x.shape[1]
        if len(self.signs) != n_columns:
            raise ValueError(f"{len(self.signs)} signs for {n_columns} columns")
        if not set(np.unique(target)) <= set(_CLASSES):
            raise ValueError("y must be 0 / 1")
        bounds = coefficient_bounds(self.signs) + [(None, None)]  # the intercept is last, and free

        def penalised_loss_and_gradient(theta: np.ndarray) -> Tuple[float, np.ndarray]:
            weights, intercept = theta[:-1], theta[-1]
            z = x @ weights + intercept
            # log(1 + exp(-s z)) with s = +1 for y = 1, -1 for y = 0, kept stable by logaddexp.
            signed = np.where(target > 0.5, -z, z)
            loss = 0.5 * float(weights @ weights) + self.C * float(np.logaddexp(0.0, signed).sum())
            residual = expit(z) - target
            gradient = np.empty_like(theta)
            gradient[:-1] = weights + self.C * (x.T @ residual)
            gradient[-1] = self.C * residual.sum()
            return loss, gradient

        result = minimize(
            penalised_loss_and_gradient,
            x0=np.zeros(n_columns + 1),
            jac=True,
            method="L-BFGS-B",
            bounds=bounds,
            options={"maxiter": self.max_iter, "gtol": self.tol, "ftol": 0.0},
        )
        # Every L-BFGS-B iterate lies inside the bounds, so the sign contract holds on the
        # returned coefficients whether or not the solver reached its tolerance; a fit that
        # stopped early is a slightly under-optimised objective, not an unconstrained one.
        self.converged_ = bool(result.success)
        if not self.converged_:
            logger.warning("signed logistic fit stopped before tolerance after %d iterations: %s", result.nit, result.message)
        self.coef_ = result.x[np.newaxis, :-1]
        self.intercept_ = result.x[-1:]
        self.n_iter_ = int(result.nit)
        self.classes_ = _CLASSES.copy()
        return self

    def decision_function(self, X) -> np.ndarray:
        return np.asarray(X, dtype=float) @ self.coef_[0] + self.intercept_[0]

    def predict_proba(self, X) -> np.ndarray:
        p = expit(self.decision_function(X))
        return np.column_stack([1.0 - p, p])

    def predict(self, X) -> np.ndarray:
        return (self.decision_function(X) > 0.0).astype(float)
