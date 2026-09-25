"""The objective's fit under a sign contract (ml.xi.signed_logistic, FEAT-14).

The estimator has two promises: with every sign free it is sklearn's ``LogisticRegression``
to solver tolerance, and with a sign given it never returns a coefficient on the wrong side
of zero -- whatever the rows say, and whether or not the solver reached its tolerance.
"""

from __future__ import annotations

import logging

import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression

from ml.xi.signed_logistic import SignedLogisticRegression, coefficient_bounds


def _rows(seed: int = 0, n: int = 2000):
    """Four columns, the second correlated with the first, with a label whose true weight
    on that second column is negative -- the correlated pair an unconstrained fit resolves
    with opposite signs. (Correlated, not a near-copy: on an exact copy the L2 penalty
    splits the weight evenly and no sign is wrong.)"""
    rng = np.random.default_rng(seed)
    x = rng.normal(size=(n, 4))
    x[:, 1] = x[:, 0] + 0.5 * rng.normal(size=n)
    logit = 1.5 * x[:, 0] - 1.0 * x[:, 1] + 0.5 * x[:, 2] - 0.7 * x[:, 3] + 0.2
    y = (rng.uniform(size=n) < 1.0 / (1.0 + np.exp(-logit))).astype(float)
    return x, y


def test_coefficient_bounds_follow_the_signs() -> None:
    assert coefficient_bounds([1, -1, 0]) == [(0.0, None), (None, 0.0), (None, None)]


def test_free_signs_reproduce_sklearns_fit() -> None:
    """The same penalised loss, so the unconstrained solution is the incumbent model."""
    x, y = _rows()

    ours = SignedLogisticRegression(signs=[0, 0, 0, 0], C=0.3).fit(x, y)
    sklearns = LogisticRegression(C=0.3, max_iter=3000).fit(x, y)

    assert ours.coef_ == pytest.approx(sklearns.coef_, abs=5e-3)
    assert ours.intercept_ == pytest.approx(sklearns.intercept_, abs=5e-3)
    assert ours.predict_proba(x)[:, 1] == pytest.approx(sklearns.predict_proba(x)[:, 1], abs=1e-3)


def test_a_bound_holds_where_the_rows_pull_the_other_way() -> None:
    """The finding's shape: the rows want a negative weight on the collinear copy, the
    contract says non-negative, and the contract wins -- the copy's weight sits exactly at
    zero and the fit still discriminates."""
    x, y = _rows()

    unconstrained = SignedLogisticRegression(signs=[0, 0, 0, 0], C=0.3).fit(x, y)
    bounded = SignedLogisticRegression(signs=[1, 1, 1, -1], C=0.3).fit(x, y)

    assert unconstrained.coef_[0, 1] < 0.0
    assert bounded.coef_[0, 1] == 0.0
    assert np.all(bounded.coef_[0, :3] >= 0.0)
    assert bounded.coef_[0, 3] <= 0.0
    assert bounded.predict(x).mean() == pytest.approx(y.mean(), abs=0.1)


def test_probabilities_are_a_distribution_and_predict_agrees_with_them() -> None:
    x, y = _rows(seed=1, n=300)

    model = SignedLogisticRegression(signs=[1, 0, 1, -1], C=0.3).fit(x, y)
    proba = model.predict_proba(x)

    assert proba.shape == (300, 2)
    assert proba.sum(axis=1) == pytest.approx(np.ones(300))
    assert np.array_equal(model.predict(x), (proba[:, 1] > 0.5).astype(float))
    assert list(model.classes_) == [0.0, 1.0]


def test_fit_records_its_iterations_and_convergence() -> None:
    x, y = _rows(seed=2, n=500)

    model = SignedLogisticRegression(signs=[1, 1, 1, -1], C=0.3).fit(x, y)

    assert model.converged_ is True
    assert model.n_iter_ > 0


def test_a_fit_stopped_early_keeps_its_bounds_and_says_so(caplog: pytest.LogCaptureFixture) -> None:
    """Every L-BFGS-B iterate is inside the box, so the sign contract does not depend on
    the solver finishing; the shortfall is logged, never hidden."""
    x, y = _rows(seed=3, n=500)

    with caplog.at_level(logging.WARNING, logger="ml.xi.signed_logistic"):
        model = SignedLogisticRegression(signs=[1, 1, 1, -1], C=0.3, max_iter=1).fit(x, y)

    assert model.converged_ is False
    assert np.all(model.coef_[0, :3] >= 0.0)
    assert model.coef_[0, 3] <= 0.0
    assert "stopped before tolerance" in caplog.text


def test_fit_rejects_a_sign_list_of_the_wrong_length() -> None:
    x, y = _rows(seed=4, n=100)

    with pytest.raises(ValueError, match="signs for 4 columns"):
        SignedLogisticRegression(signs=[1, 1]).fit(x, y)


def test_fit_rejects_labels_outside_zero_and_one() -> None:
    x, _ = _rows(seed=5, n=100)

    with pytest.raises(ValueError, match="0 / 1"):
        SignedLogisticRegression(signs=[0, 0, 0, 0]).fit(x, np.full(100, 2.0))


def test_a_row_at_weight_zero_is_absent_from_the_fit() -> None:
    """EVAL-13: ``sample_weight`` scales each row's log-loss, so weighting the second half
    of the rows at zero is the fit on the first half alone -- the mechanism the objective's
    recency half-life reaches the estimator through."""
    x, y = _rows()
    half = len(y) // 2
    weights = np.concatenate([np.ones(half), np.zeros(len(y) - half)])

    weighted = SignedLogisticRegression(signs=[0, 0, 0, 0], C=0.3).fit(x, y, sample_weight=weights)
    first_half = SignedLogisticRegression(signs=[0, 0, 0, 0], C=0.3).fit(x[:half], y[:half])
    unweighted = SignedLogisticRegression(signs=[0, 0, 0, 0], C=0.3).fit(x, y)

    assert weighted.coef_ == pytest.approx(first_half.coef_, abs=1e-3)
    assert weighted.intercept_ == pytest.approx(first_half.intercept_, abs=1e-3)
    assert weighted.coef_ != pytest.approx(unweighted.coef_, abs=1e-3)


def test_fit_rejects_a_weight_per_row_that_is_negative_or_the_wrong_length() -> None:
    x, y = _rows(n=50)

    with pytest.raises(ValueError, match="sample_weight"):
        SignedLogisticRegression(signs=[0, 0, 0, 0]).fit(x, y, sample_weight=np.ones(49))
    with pytest.raises(ValueError, match="sample_weight"):
        SignedLogisticRegression(signs=[0, 0, 0, 0]).fit(x, y, sample_weight=-np.ones(50))
