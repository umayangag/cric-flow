"""The gender split of the XI win report (E4 in docs/ML_PIPELINE_REARCHITECTURE_PLAN.md).

Identity errors -- 163 names covering 348 people, and 130 team names shared by a men's and
a women's side -- fall disproportionately on the 20% of the dataset that is women's
cricket. An aggregate AUC cannot show what fixing them is worth, because the men's subset
dominates it, so the report has to split.
"""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi.train import _score_marginalised, gender_breakdown


class _ConstantModel:
    """Scores a row by its first display column, so AUC is decided by the fixture."""

    def __init__(self, column_index: int = 0) -> None:
        self.column_index = column_index

    def predict_proba(self, x: np.ndarray) -> np.ndarray:
        p = 1.0 / (1.0 + np.exp(-x[:, self.column_index]))
        return np.column_stack([1.0 - p, p])


@pytest.fixture
def holdout() -> pd.DataFrame:
    rng = np.random.default_rng(0)
    n = 120
    # Both orientations of every side feature, because the serving path scores the
    # fixture with the sides exchanged as well (H-3).
    columns = list(C.DISPLAY_FEATURE_COLS)
    columns += [f"{prefix}_{stem}" for stem in C.SIDE_FEATURE_STEMS for prefix in ("t1", "t2", "d")]
    frame = pd.DataFrame({col: rng.normal(size=n) for col in dict.fromkeys(columns)})
    frame["team_h2h"] = rng.uniform(size=n)
    frame["gender"] = ["female"] * 60 + ["male"] * 60
    # The signal is in the first display column, so both subsets are separable.
    frame[C.TARGET_COL] = (frame[C.DISPLAY_FEATURE_COLS[0]] > 0).astype(float)
    return frame


def test_gender_breakdown_reports_each_gender_separately(holdout) -> None:
    model = _ConstantModel()

    out = gender_breakdown(model, model, holdout)

    assert set(out) == {"female", "male"}
    assert out["female"]["n_holdout"] == 60
    assert out["male"]["n_holdout"] == 60
    assert out["female"]["objective_auc"] > 0.5
    assert out["male"]["display_auc"] > 0.5


def test_gender_breakdown_reports_the_count_but_no_auc_for_a_tiny_subset(holdout) -> None:
    """TEST cricket has 24 women's matches in the whole dataset. A number computed on a
    handful of rows would read as a measurement; the count alone is the honest report."""
    small = pd.concat([holdout[holdout.gender == "male"], holdout[holdout.gender == "female"].head(5)])
    model = _ConstantModel()

    out = gender_breakdown(model, model, small)

    assert out["female"] == {"n_holdout": 5}
    assert "display_auc" in out["male"]


def test_gender_breakdown_reports_the_count_but_no_auc_for_a_single_class_subset(holdout) -> None:
    one_sided = holdout.copy()
    one_sided.loc[one_sided.gender == "female", C.TARGET_COL] = 1.0
    model = _ConstantModel()

    out = gender_breakdown(model, model, one_sided)

    assert out["female"] == {"n_holdout": 60}


def test_gender_breakdown_reports_the_one_display_fits_own_auc(holdout) -> None:
    """The display model is one fit (EVAL-02): the breakdown carries its AUC on the
    subset and no spread across seeds, which would be a spread of nothing."""
    display = _ConstantModel(1)

    out = gender_breakdown(_ConstantModel(0), display, holdout)

    female = holdout[holdout.gender == "female"]
    assert out["female"]["display_auc"] == pytest.approx(
        _score_marginalised(display, female, C.DISPLAY_FEATURE_COLS)["auc"]
    )
    assert "display_auc_sd" not in out["female"]
