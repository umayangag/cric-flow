"""Unit tests for ml.auto_tune_pycaret."""

from unittest.mock import patch

import numpy as np

from ml.auto_tune_pycaret import (
    _model_id_to_our_key,
    _our_key_for_optuna,
    is_available,
    run_pycaret_ranking_classification,
    run_pycaret_ranking_regression,
)


def test_is_available():
    """is_available returns bool (PyCaret may or may not be installed)."""
    result = is_available()
    assert isinstance(result, bool)


def test_model_id_to_our_key_exact():
    """Exact PyCaret IDs map to our keys."""
    assert _model_id_to_our_key("rf") == "rf"
    assert _model_id_to_our_key("et") == "et"
    assert _model_id_to_our_key("gbr") == "gb"
    assert _model_id_to_our_key("lightgbm") == "lightgbm"
    assert _model_id_to_our_key("xgboost") == "xgboost"
    assert _model_id_to_our_key("mlp") == "mlp"
    assert _model_id_to_our_key("hgb") == "hgb"


def test_model_id_to_our_key_full_names():
    """Full model names map correctly."""
    assert _model_id_to_our_key("Random Forest") == "rf"
    assert _model_id_to_our_key("Extra Trees") == "et"
    assert _model_id_to_our_key("Gradient Boosting Regressor") == "gb"
    assert _model_id_to_our_key("Light Gradient Boosting") == "lightgbm"
    assert _model_id_to_our_key("Extreme Gradient Boosting") == "xgboost"
    assert _model_id_to_our_key("MLP Regressor") == "mlp"
    assert _model_id_to_our_key("Hist Gradient Boosting") == "hgb"


def test_model_id_to_our_key_fuzzy():
    """Fuzzy matching when exact match not found."""
    assert _model_id_to_our_key("random forest model") == "rf"
    assert _model_id_to_our_key("extra tree regressor") == "et"
    assert _model_id_to_our_key("gradient boosting") == "gb"
    assert _model_id_to_our_key("histogram gradient") == "hgb"
    assert _model_id_to_our_key("neural network mlp") == "mlp"


def test_model_id_to_our_key_unknown_fallback():
    """Unknown model ID falls back to rf."""
    assert _model_id_to_our_key("unknown_model") == "rf"


def test_our_key_for_optuna_supported():
    """Supported keys pass through."""
    for key in ("rf", "gb", "et", "hgb", "mlp"):
        assert _our_key_for_optuna(key) == key


def test_our_key_for_optuna_unsupported_maps_to_gb():
    """lightgbm and xgboost map to gb for Optuna."""
    assert _our_key_for_optuna("lightgbm") == "gb"
    assert _our_key_for_optuna("xgboost") == "gb"


@patch("ml.auto_tune_pycaret._HAS_PYCARET", False)
def test_run_pycaret_ranking_regression_no_pycaret():
    """When PyCaret is not installed, regression returns [], [], False."""
    X = np.random.RandomState(42).rand(20, 5)
    y = np.random.RandomState(43).rand(20)
    algos, ranking, success = run_pycaret_ranking_regression(X, y)
    assert algos == []
    assert ranking == []
    assert success is False


@patch("ml.auto_tune_pycaret._HAS_PYCARET", False)
def test_run_pycaret_ranking_classification_no_pycaret():
    """When PyCaret is not installed, classification returns [], [], False."""
    X = np.random.RandomState(42).rand(20, 5)
    y = (np.random.RandomState(43).rand(20) > 0.5).astype(int)
    algos, ranking, success = run_pycaret_ranking_classification(X, y)
    assert algos == []
    assert ranking == []
    assert success is False
