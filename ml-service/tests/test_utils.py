"""Unit tests for ml.utils (make_base_estimator and shared ML utilities)."""

from ml.utils import make_base_estimator


def test_make_base_estimator_rf_default():
    """Default estimator type is rf -> RandomForestRegressor."""
    params = {
        "n_estimators": 50,
        "max_depth": 8,
        "random_state": 42,
        "n_jobs": -1,
    }
    est = make_base_estimator(params)
    assert est is not None
    assert type(est).__name__ == "RandomForestRegressor"
    assert est.n_estimators == 50
    assert est.max_depth == 8
    assert est.random_state == 42
    assert est.n_jobs == -1


def test_make_base_estimator_rf_explicit():
    """Explicit estimator='rf' returns RandomForestRegressor."""
    params = {
        "n_estimators": 100,
        "max_depth": 10,
        "random_state": 1,
        "estimator": "rf",
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "RandomForestRegressor"
    assert est.n_estimators == 100
    assert est.n_jobs == -1  # default when omitted


def test_make_base_estimator_random_forest_alias():
    """estimator='random_forest' is treated as rf."""
    params = {
        "n_estimators": 20,
        "max_depth": 5,
        "random_state": 0,
        "estimator": "random_forest",
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "RandomForestRegressor"


def test_make_base_estimator_gb():
    """estimator='gb' returns GradientBoostingRegressor."""
    params = {
        "n_estimators": 80,
        "max_depth": 6,
        "random_state": 7,
        "estimator": "gb",
        "learning_rate": 0.05,
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "GradientBoostingRegressor"
    assert est.learning_rate == 0.05
    assert est.n_estimators == 80


def test_make_base_estimator_gb_exact():
    """Only estimator='gb' (exact) returns GradientBoostingRegressor; aliases come from config."""
    params = {
        "n_estimators": 30,
        "max_depth": 4,
        "random_state": 2,
        "estimator": "gb",
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "GradientBoostingRegressor"


def test_make_base_estimator_quantile():
    """estimator='quantile' returns GradientBoostingRegressor with quantile loss."""
    params = {
        "n_estimators": 40,
        "max_depth": 5,
        "random_state": 3,
        "estimator": "quantile",
        "learning_rate": 0.1,
        "quantile_level": 0.9,
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "GradientBoostingRegressor"
    assert est.loss == "quantile"
    assert est.alpha == 0.9


def test_make_base_estimator_quantile_exact():
    """Only estimator='quantile' (exact) returns quantile GBM; 'qr' alias is normalized in config."""
    params = {
        "n_estimators": 25,
        "max_depth": 3,
        "random_state": 0,
        "estimator": "quantile",
        "quantile_level": 0.5,
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "GradientBoostingRegressor"
    assert est.loss == "quantile"
    assert est.alpha == 0.5


def test_make_base_estimator_stacked():
    """estimator='stacked' returns StackingRegressor (rf + gb + Ridge)."""
    params = {
        "n_estimators": 60,
        "max_depth": 7,
        "random_state": 99,
        "estimator": "stacked",
        "learning_rate": 0.1,
        "n_jobs": 2,
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "StackingRegressor"
    assert len(est.estimators) == 2
    assert est.n_jobs is None  # StackingRegressor doesn't take n_jobs in constructor


def test_make_base_estimator_stacked_exact():
    """Only estimator='stacked' (exact) returns StackingRegressor; aliases normalized in config."""
    params = {
        "n_estimators": 10,
        "max_depth": 2,
        "random_state": 0,
        "estimator": "stacked",
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "StackingRegressor"


def test_make_base_estimator_n_jobs_default():
    """When n_jobs is omitted, default is -1 for RF."""
    params = {
        "n_estimators": 5,
        "max_depth": 2,
        "random_state": 0,
    }
    est = make_base_estimator(params)
    assert est.n_jobs == -1


def test_make_base_estimator_unknown_estimator_falls_back_to_rf():
    """Unknown estimator string falls back to RandomForestRegressor."""
    params = {
        "n_estimators": 5,
        "max_depth": 2,
        "random_state": 0,
        "estimator": "unknown_estimator",
    }
    est = make_base_estimator(params)
    assert type(est).__name__ == "RandomForestRegressor"


def test_make_base_estimator_quantile_level_clamped():
    """quantile_level is used as-is in [0.01, 0.99]; default 0.5."""
    params = {
        "n_estimators": 5,
        "max_depth": 2,
        "random_state": 0,
        "estimator": "quantile",
        "quantile_level": 0.25,
    }
    est = make_base_estimator(params)
    assert est.alpha == 0.25
