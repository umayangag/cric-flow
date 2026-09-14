"""The display model is one fit, and the report says so (EVAL-02).

``HistGradientBoostingClassifier``'s ``random_state`` reaches only the early-stopping
validation split (which sklearn switches on above 10,000 rows) and the binning subsample
(above 200,000). Below that the three "seeds" the pipeline used to fit were the same
model three times, and the spread it reported across them -- and called a noise floor --
was identically zero. These tests pin the premise on the model's own settings and hold
the report to fitting once and reporting that fit's score.
"""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest
from sklearn.metrics import roc_auc_score

from ml.xi import contract as C
from ml.xi.train import _xy, make_display_model, train_format

SWAP_COLUMNS = ("team_elo_diff", "team_form_diff", "venue_fam_diff", "team_h2h")


def synthetic_win_rows(n: int, seed: int = 0) -> pd.DataFrame:
    """Win rows the models can be fitted and marginalised on: every display column, both
    orientations of every side feature (H-3), a monthly date and a label the first display
    column separates."""
    rng = np.random.default_rng(seed)
    columns = list(C.DISPLAY_FEATURE_COLS) + list(C.XI_FEATURE_COLS)
    columns += [f"{prefix}_{stem}" for stem in C.SIDE_FEATURE_STEMS for prefix in ("t1", "t2", "d")]
    columns += list(SWAP_COLUMNS)
    frame = pd.DataFrame({col: rng.normal(size=n) for col in dict.fromkeys(columns)})
    frame["team_h2h"] = rng.uniform(size=n)
    frame["match_date"] = pd.Timestamp("2023-01-01") + pd.to_timedelta(np.arange(n), unit="D")
    frame["format_code"] = "T20"
    frame["gender"] = "male"
    signal = frame[C.DISPLAY_FEATURE_COLS[0]] + 0.5 * rng.normal(size=n)
    frame[C.TARGET_COL] = (signal > 0).astype(float)
    return frame


def test_display_refits_under_other_seeds_are_identical_below_the_early_stopping_threshold() -> None:
    """EVAL-02's premise, on the display model's own settings: below 10,000 rows the seed
    reaches nothing, so a spread across seeds measures nothing."""
    rows = synthetic_win_rows(2000)
    x, y = _xy(rows, C.DISPLAY_FEATURE_COLS)

    predictions = [
        make_display_model(C.DISPLAY_FEATURE_COLS).set_params(random_state=seed).fit(x, y).predict_proba(x[:200])[:, 1]
        for seed in (0, 1, 2)
    ]

    assert np.array_equal(predictions[0], predictions[1])
    assert np.array_equal(predictions[0], predictions[2])


def test_train_format_fits_the_display_model_once_and_reports_that_fits_score() -> None:
    """The report carries the served display model's own AUC and Brier -- the shape the
    objective's entry has -- and no seed list or spread across seeds."""
    rows = synthetic_win_rows(400)
    cutoff = pd.Timestamp("2024-01-01")

    models, report = train_format(rows, "T20", cutoff)

    holdout = rows[rows.match_date >= cutoff]
    x_holdout, y_holdout = _xy(holdout, C.DISPLAY_FEATURE_COLS)
    served_auc = roc_auc_score(y_holdout, models.display.predict_proba(x_holdout)[:, 1])
    assert "seeds" not in report
    assert set(report["display"]) == {"auc", "brier"}
    assert report["display"]["auc"] == pytest.approx(served_auc)
    assert set(report["by_gender"]["male"]) == {"n_holdout", "objective_auc", "display_auc"}
