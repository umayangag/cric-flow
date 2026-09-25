"""The display model is one fit, it runs every iteration the grid chose, and the report
says so (EVAL-01, EVAL-02).

``HistGradientBoostingClassifier``'s ``random_state`` reaches only the early-stopping
validation split (which sklearn's ``'auto'`` switches on above 10,000 rows) and the
binning subsample (above 200,000). Below that the three "seeds" the pipeline used to fit
were the same model three times, and the spread it reported across them -- and called a
noise floor -- was identically zero. Above it, ``'auto'`` fitted a different model from
the one the grid had scored: fewer iterations, on a random 90 % of the rows. These tests
pin the premise on the model's own settings, hold early stopping off so the fitted model
is the scored one, and hold the report to fitting once and reporting that fit's score.
"""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest
from sklearn.metrics import roc_auc_score

from ml.xi import contract as C
from ml.xi import evaluate as evaluate_module
from ml.xi import selection_metrics
from ml.xi import train as train_module
from ml.xi.builder import build
from ml.xi.train import (
    _score_marginalised,
    _xy,
    fit_objective,
    make_display_model,
    make_objective_model,
    marginalised_probabilities,
    own_side_sensitivities,
    recency_weights,
    swap_orientation,
    train_format,
)
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history

SWAP_COLUMNS = ("team_elo_diff", "team_form_diff", "venue_fam_diff", "team_h2h", "home_diff")


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


def test_swap_orientation_exchanges_the_batting_order_and_keeps_the_toss_winner() -> None:
    """FEAT-05: with the sides exchanged, home advantage changes sign like the other
    signed context columns, and the toss reads 1 - x -- the side that won it is a fact of
    the fixture and now bats second."""
    rows = synthetic_win_rows(8)
    rows[C.TOSS_COL] = [1.0, 0.0] * 4

    swapped = swap_orientation(rows)

    assert list(swapped["home_diff"]) == list(-rows["home_diff"])
    assert list(swapped[C.TOSS_COL]) == [0.0, 1.0] * 4


def test_marginalised_probabilities_average_both_batting_orders_and_both_toss_winners() -> None:
    """The served reading (EVAL-05): what ``XiStore.display_probability`` answers is the
    mean over both batting orders and, for the display model, both answers to who won the
    toss -- four readings, not two -- so the manifest scores the number that is served."""
    rows = synthetic_win_rows(400)
    x, y = _xy(rows, C.DISPLAY_FEATURE_COLS)
    display = make_display_model(C.DISPLAY_FEATURE_COLS).fit(x, y)
    scored = rows.head(20)
    readings = []
    for frame, swapped in ((scored, False), (swap_orientation(scored), True)):
        for toss in (1.0, 0.0):
            p = display.predict_proba(_xy(frame.assign(**{C.TOSS_COL: toss}), C.DISPLAY_FEATURE_COLS)[0])[:, 1]
            readings.append(1.0 - p if swapped else p)

    served = marginalised_probabilities(display, scored, C.DISPLAY_FEATURE_COLS)

    np.testing.assert_allclose(served, np.mean(readings, axis=0))
    objective_only = marginalised_probabilities(display, scored, C.DISPLAY_FEATURE_COLS)
    assert not np.allclose(objective_only, readings[0]), "the toss is read, so averaging it changes the number"


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


def test_display_model_never_early_stops() -> None:
    """EVAL-01: the setting is explicit, not sklearn's row-count-dependent ``'auto'``, so
    the grid's ``max_iter`` is honoured at every training-set size."""
    model = make_display_model(C.DISPLAY_FEATURE_COLS)

    assert model.early_stopping is False


def test_train_format_fits_every_iteration_the_grid_chose_above_the_early_stopping_threshold() -> None:
    """The finding's own test: above 10,000 training rows -- T20's regime -- the served
    display model runs the ``max_iter`` the grid scored, and the report records the
    iterations it ran beside the choice. Under ``'auto'`` the same fit stops at ~117."""
    rows = synthetic_win_rows(10_400)
    cutoff = rows.match_date.iloc[10_200]

    models, report = train_format(rows, "T20", cutoff)

    chosen_max_iter = report["hyperparameters"]["display"]["params"]["max_iter"]
    assert report["n_train"] > 10_000
    assert models.display.n_iter_ == chosen_max_iter
    assert report["hyperparameters"]["display"]["n_iter"] == chosen_max_iter
    assert models.metadata["hyperparameters"]["display"]["n_iter"] == chosen_max_iter


def test_train_format_fits_the_display_model_once_and_reports_that_fits_score() -> None:
    """The report carries the served display model's own AUC and Brier -- the shape the
    objective's entry has -- and no seed list or spread across seeds. The toss-aware
    block is the model read at the actual batting order; the marginalised one beside it
    is the served reading the manifest quotes (EVAL-05)."""
    rows = synthetic_win_rows(400)
    cutoff = pd.Timestamp("2024-01-01")

    models, report = train_format(rows, "T20", cutoff)

    holdout = rows[rows.match_date >= cutoff]
    x_holdout, y_holdout = _xy(holdout, C.DISPLAY_FEATURE_COLS)
    toss_aware_auc = roc_auc_score(y_holdout, models.display.predict_proba(x_holdout)[:, 1])
    assert "seeds" not in report
    assert set(report["display_toss_aware"]) == {"auc", "brier"}
    assert report["display_toss_aware"]["auc"] == pytest.approx(toss_aware_auc)
    assert report["display_marginalised"]["auc"] == pytest.approx(
        _score_marginalised(models.display, holdout, C.DISPLAY_FEATURE_COLS)["auc"]
    )
    assert set(report["by_gender"]["male"]) == {"n_holdout", "objective_auc", "display_auc"}


# --- FEAT-14: the objective is monotone by construction -----------------------------------


def collinear_elo_rows(n: int = 3000, seed: int = 7) -> pd.DataFrame:
    """Win rows in which ``d_pelo_top3`` is correlated with ``d_pelo_mean`` and the label
    rewards the top-three Elo *more* than the mean -- so an unconstrained fit resolves the
    pair with a negative weight on the mean, exactly what the served T20I objective did."""
    rows = synthetic_win_rows(n, seed)
    # A generator of its own: reseeding with ``seed`` would replay the draw that became
    # ``d_pelo_mean`` and make the pair exactly collinear, where no sign goes wrong.
    rng = np.random.default_rng(seed + 1)
    rows["d_pelo_top3"] = rows["d_pelo_mean"] + 0.7 * rng.normal(size=n)
    logit = 2.0 * rows["d_pelo_top3"] - 1.5 * rows["d_pelo_mean"]
    rows[C.TARGET_COL] = (rng.uniform(size=n) < 1.0 / (1.0 + np.exp(-logit))).astype(float)
    return rows


def _assert_sensitivities_carry_the_contract(sensitivities: dict) -> None:
    """Every stem the objective reads that the contract signs moves the logit the
    contract's way in both batting orders."""
    signed = {stem: C._STEM_DIRECTION[stem] for stem in sensitivities if C._STEM_DIRECTION[stem] != 0}
    assert {"pelo_mean", "pelo_top3", "pelo_min", "imp_bat_sum", "n_debutants"} <= set(signed)
    for stem, direction in signed.items():
        bats_first, bats_second = sensitivities[stem]
        assert direction * bats_first >= 0.0, stem
        assert direction * bats_second >= 0.0, stem


def test_objective_own_side_sensitivity_carries_the_contracts_sign_in_both_batting_orders() -> None:
    """The finding's own test. Fitted on rows that pull ``pelo_mean`` negative, the objective
    still cannot rate a higher-Elo eleven lower: every signed stem's sensitivity has the
    contract's sign whether the side bats first or second. Fails on an unconstrained fit."""
    rows = collinear_elo_rows()
    x, y = _xy(rows, C.XI_FEATURE_COLS)

    objective = make_objective_model(C.XI_FEATURE_COLS).fit(x, y)
    sensitivities = own_side_sensitivities(objective, C.XI_FEATURE_COLS)

    assert sensitivities["pelo_mean"][0] >= 0.0
    assert sensitivities["pelo_mean"][1] >= 0.0
    _assert_sensitivities_carry_the_contract(sensitivities)


def test_objective_reads_the_contracts_sign_for_every_column() -> None:
    objective = make_objective_model(C.XI_FEATURE_COLS)

    assert list(objective.steps[-1][1].signs) == C.monotone_directions(C.XI_FEATURE_COLS)


def test_a_one_player_upgrade_never_lowers_the_fitted_objective() -> None:
    """H-4's probe on the fitted surface, on every axis: exactly zero, not under a line. The
    rows are the ones that made the unconstrained fit prefer the lower-Elo player."""
    rows = collinear_elo_rows()
    x, y = _xy(rows, C.XI_FEATURE_COLS)
    matches, _, _ = _synthetic_history(60)
    player_rows = build(_ListSource(matches)).player_frame

    objective = make_objective_model(C.XI_FEATURE_COLS).fit(x, y)
    report = selection_metrics.swap_monotonicity(objective, C.XI_FEATURE_COLS, player_rows, "T20", max_matches=20)

    assert report["upgrades"] > 0
    assert report["violations"] == 0
    assert report["by_axis"]["pelo_only"]["violations"] == 0
    assert report["by_axis"]["rates_only"]["violations"] == 0


def test_train_format_fits_the_objective_under_the_contract() -> None:
    """The served artifact, not only the factory: what ``train_format`` writes carries the
    sign contract and reads the XI columns without the Elo spread."""
    rows = collinear_elo_rows(600)
    cutoff = rows.match_date.iloc[500]

    models, report = train_format(rows, "T20", cutoff)

    assert models.objective_cols == list(C.XI_FEATURE_COLS)
    assert not any(column.endswith("_pelo_std") for column in models.objective_cols)
    _assert_sensitivities_carry_the_contract(own_side_sensitivities(models.objective, models.objective_cols))
    assert set(report["objective_marginalised"]) == {"auc", "brier"}


# --- EVAL-13: the objective's C and recency weight are chosen per format, on the inner split ---


def test_recency_weights_halve_every_half_life_back_from_the_newest_row() -> None:
    dates = pd.Series(pd.to_datetime(["2020-01-01", "2024-01-01", "2028-01-01"]))

    weights = recency_weights(dates, 4.0)

    assert weights[2] == 1.0
    assert weights[1] == pytest.approx(0.5, rel=1e-3)
    assert weights[0] == pytest.approx(0.25, rel=1e-3)
    assert list(recency_weights(dates, None)) == [1.0, 1.0, 1.0]


def test_the_objective_grid_puts_the_incumbent_first_and_reaches_the_fitted_estimator() -> None:
    """Grid point 0 is what the objective has always been fitted with, and the pick's ``C``
    is the estimator's."""
    rows = collinear_elo_rows(600)

    objective = fit_objective(rows, {"C": 0.1, "half_life_years": None})

    assert train_module.OBJECTIVE_GRID[0] == {"C": 0.3, "half_life_years": None}
    assert len(train_module.OBJECTIVE_GRID) == 9
    assert objective.steps[-1][1].C == 0.1


def test_a_crippled_incumbent_is_displaced_and_the_harness_fits_the_objective_the_grid_would_ship(
    monkeypatch,
) -> None:
    """The finding's own test: the objective's ``C`` and half-life are chosen on the inner
    temporal split -- a half-life that leaves one row with any weight cannot rank, so the
    grid must move off it -- and the harness window fits the same pick ``train_format``
    ships and records it under the manifest's key (the EVAL-06 rule, for this model)."""
    crippled, incumbent = {"C": 0.3, "half_life_years": 1e-4}, dict(train_module.OBJECTIVE_GRID[0])
    monkeypatch.setattr(train_module, "OBJECTIVE_GRID", (crippled, incumbent))
    rows = collinear_elo_rows(600)
    cutoff, end = rows.match_date.iloc[500], rows.match_date.max() + pd.Timedelta(days=1)

    models, report = train_format(rows, "T20", cutoff)
    fold, harness_objective, _ = evaluate_module._evaluate_win_window(rows, cutoff, end)

    record = report["hyperparameters"]["objective"]
    assert record["params"] == incumbent, "the grid stayed on its (crippled) incumbent"
    assert record["reason"] == "beat the incumbent on the inner split"
    assert [score["params"] for score in record["scores"]] == [crippled, incumbent]
    assert record["scores"][0]["auc"] < record["scores"][1]["auc"]
    assert fold["hyperparameters"]["objective"] == record
    assert models.objective.steps[-1][1].C == harness_objective.steps[-1][1].C == incumbent["C"]
    assert set(report["hyperparameters"]) == {"objective", "display"}
