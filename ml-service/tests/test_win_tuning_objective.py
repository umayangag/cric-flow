"""The win model is tuned for ranking, not for accuracy.

``TabularPredictor`` is patched with ``create=True`` throughout: the real name exists only
when AutoGluon is installed, and CI installs ``requirements-serve.txt``, which omits it.
Standing in for the missing attribute is what lets these guards run in the environment
where the dependency is absent -- the one where a metric drifting out of step would go
unnoticed the longest.

Team selection takes an argmax over candidate XIs, so only the order the model puts them
in can change which side is picked. A threshold metric is blind to every improvement
that does not cross 0.5 and rewards leaning on the majority outcome, so tuning for
accuracy can buy a model that selects worse than the one it replaced.
"""

import inspect
from unittest.mock import MagicMock, patch

import numpy as np
import pandas as pd

from ml.auto_tune_autogluon import run_autogluon_classification
from ml.tuning import runners


def test_win_search_optimises_roc_auc() -> None:
    """The Optuna search for win scores candidates by ranking."""
    source = inspect.getsource(runners.run_auto_tune_win)

    assert 'scoring = "roc_auc"' in source
    assert 'scoring = "accuracy"' not in source


def test_win_search_ignores_the_regression_scoring_in_config() -> None:
    """ml.tuning.scoring holds a regression metric; applying it to a classifier is worse than ignoring it."""
    source = inspect.getsource(runners.run_auto_tune_win)

    assert 'tuning["scoring"]' not in source
    assert 'tuning.get("scoring"' not in source


@patch("ml.auto_tune_autogluon._HAS_AUTOGLUON", True)
@patch("ml.auto_tune_autogluon.TabularPredictor", create=True)
def test_autogluon_classification_uses_the_metric_it_is_given(predictor_cls) -> None:
    """The caller passes the search's metric so the two scores are comparable."""
    predictor = MagicMock()
    predictor.leaderboard.return_value = pd.DataFrame({"score_val": [0.72]})
    predictor_cls.return_value = predictor

    _, score, _, ok = run_autogluon_classification(
        np.array([[1.0], [2.0], [3.0], [4.0]]),
        np.array([0, 1, 0, 1]),
        time_limit_seconds=1,
        eval_metric="roc_auc",
    )

    assert ok is True
    assert score == 0.72
    assert predictor_cls.call_args.kwargs["eval_metric"] == "roc_auc"


@patch("ml.auto_tune_autogluon._HAS_AUTOGLUON", True)
@patch("ml.auto_tune_autogluon.TabularPredictor", create=True)
def test_autogluon_classification_defaults_to_ranking(predictor_cls) -> None:
    """A caller that names no metric still gets the ranking one, never accuracy."""
    predictor = MagicMock()
    predictor.leaderboard.return_value = pd.DataFrame({"score_val": [0.6]})
    predictor_cls.return_value = predictor

    run_autogluon_classification(np.array([[1.0], [2.0]]), np.array([0, 1]), time_limit_seconds=1)

    assert predictor_cls.call_args.kwargs["eval_metric"] == "roc_auc"


def test_the_two_scores_being_compared_come_from_one_metric() -> None:
    """AutoGluon's score and the search's are compared with a plain >, so they must measure the same thing."""
    comparison = inspect.getsource(runners._maybe_run_autogluon_and_compare)

    # The classification branch forwards the caller's scoring rather than naming its own.
    assert "eval_metric=scoring" in comparison
    assert "scoring" in inspect.signature(runners._maybe_run_autogluon_and_compare).parameters


def test_win_forwards_its_scoring_to_the_comparison() -> None:
    """The win runner must actually pass its metric down, or the guard above proves nothing."""
    source = inspect.getsource(runners.run_auto_tune_win)

    assert "float(optuna_score), scoring" in source
