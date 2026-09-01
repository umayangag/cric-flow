"""Unit tests for the performance model (L2-B): distributions, structures, marginalisation."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import performance as P
from ml.xi.perf_calibration import QuantileRecalibration
from tests.xi_perf_fixtures import fast_fits, synthetic_player_frame


def test_mixture_quantiles_with_certain_involvement_are_the_conditional_quantiles() -> None:
    conditional = np.tile(np.arange(1.0, 10.0), (2, 1))  # q0.1 = 1 ... q0.9 = 9

    out = P.mixture_quantiles(np.array([1.0, 1.0]), conditional)

    np.testing.assert_allclose(out, [[1.0, 5.0, 9.0], [1.0, 5.0, 9.0]])


def test_mixture_quantiles_place_the_zero_mass_first() -> None:
    conditional = np.arange(1.0, 10.0)[None, :]

    out = P.mixture_quantiles(np.array([0.5]), conditional)

    # tau = 0.1 sits inside the 0.5 zero mass; tau = 0.9 is conditional level 0.8 -> 8
    np.testing.assert_allclose(out[0], [0.0, 0.0, 8.0])


def test_count_distribution_matches_poisson_and_zero_inflates() -> None:
    plain = P.count_distribution(np.array([0.0]), np.array([1.0]))
    inflated = P.count_distribution(np.array([0.5]), np.array([1.0]))

    assert plain["p0"][0] == pytest.approx(np.exp(-1.0))
    assert plain["p1"][0] == pytest.approx(np.exp(-1.0))
    assert plain["mean"][0] == pytest.approx(1.0)
    np.testing.assert_allclose(plain["quantiles"][0], [0.0, 1.0, 2.0])
    assert inflated["p0"][0] == pytest.approx(0.5 + 0.5 * np.exp(-1.0))
    assert inflated["mean"][0] == pytest.approx(0.5)
    assert inflated["quantiles"][0][0] == 0.0
    assert inflated["p0"][0] + inflated["p1"][0] + inflated["p2plus"][0] == pytest.approx(1.0)


def test_design_matrix_reads_the_innings_from_the_row_or_the_override() -> None:
    rows = pd.DataFrame({"side": [1, 2], "format_code": ["T20", "T20I"], "career": [3.0, 4.0]})
    cols = ["career", C.BATS_FIRST_COL, C.FORMAT_INDICATOR_COL]

    own = P.design_matrix(rows, cols)
    forced = P.design_matrix(rows, cols, bats_first=False)

    np.testing.assert_allclose(own, [[3.0, 1.0, 0.0], [4.0, 0.0, 1.0]])
    np.testing.assert_allclose(forced[:, 1], [0.0, 0.0])


def test_performance_feature_cols_never_include_an_outcome_column() -> None:
    cols = C.performance_feature_cols(tuple(C.SEQUENCE_FAMILIES), joint_format=True)

    assert not set(cols) & set(C.PLAYER_MATCH_TARGET_COLS)
    assert C.BATS_FIRST_COL in cols and C.FORMAT_INDICATOR_COL in cols
    assert all(key in cols for key in C.PLAYER_SEQUENCE_KEYS)


def test_involvement_parts_follow_the_targets_fitted() -> None:
    full = P.default_spec()
    subset = P.default_spec(targets=("runs",))
    two_part = P.default_spec(targets=("runs",), structure={**P.DEFAULT_STRUCTURE, "runs": "two_part"})

    assert full.involvement_parts == ("bats", "bowls")
    assert subset.involvement_parts == ()
    assert two_part.involvement_parts == ("bats",)


@pytest.fixture(scope="module")
def frame() -> pd.DataFrame:
    return synthetic_player_frame()


@pytest.fixture(scope="module")
def fitted(frame) -> P.PerformanceModels:
    with fast_fits():
        return P.fit_performance(frame[frame.match_date < pd.Timestamp("2023-05-01")], "T20", P.default_spec())


def test_fit_produces_every_output_in_range(fitted, frame) -> None:
    rows = frame[frame.match_date >= pd.Timestamp("2023-05-01")]

    prediction = fitted.predict_marginalised(rows)

    assert prediction["p_bats"].shape == (len(rows),)
    assert np.all((prediction["p_bats"] >= 0) & (prediction["p_bats"] <= 1))
    for t in P.TARGETS:
        q = prediction[t.name]["quantiles"]
        assert q.shape == (len(rows), 3)
        assert np.all(np.diff(q, axis=1) >= 0) and np.all(q >= 0)
    wickets = prediction["wickets"]
    np.testing.assert_allclose(wickets["p0"] + wickets["p1"] + wickets["p2plus"], 1.0)
    assert fitted.metadata["n_train"] > 0 and fitted.metadata["spec"]["seeds"] == [0]


def test_marginalised_prediction_is_the_mean_of_both_innings(fitted, frame) -> None:
    rows = frame[frame.match_date >= pd.Timestamp("2023-05-01")].head(30)

    marginalised = fitted.predict_marginalised(rows)["runs"]["quantiles"]
    first = fitted.predict_oriented(rows, True)["runs"]["quantiles"]
    chase = fitted.predict_oriented(rows, False)["runs"]["quantiles"]

    np.testing.assert_allclose(marginalised, 0.5 * (first + chase), atol=1e-9)


def test_oriented_prediction_without_an_override_reads_each_rows_side(fitted, frame) -> None:
    rows = frame[frame.match_date >= pd.Timestamp("2023-05-01")].head(30)
    sides = rows.side.to_numpy()

    own = fitted.predict_oriented(rows, None)["runs"]["quantiles"]
    first = fitted.predict_oriented(rows, True)["runs"]["quantiles"]
    chase = fitted.predict_oriented(rows, False)["runs"]["quantiles"]

    np.testing.assert_allclose(own[sides == 1], first[sides == 1])
    np.testing.assert_allclose(own[sides == 2], chase[sides == 2])


def test_two_part_structure_fits_and_serves_mixture_quantiles(frame) -> None:
    structure = {**P.DEFAULT_STRUCTURE, "runs": "two_part", "wickets": "two_part"}
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]
    rows = frame[frame.match_date >= pd.Timestamp("2023-05-01")]

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(structure=structure, targets=("runs", "wickets")))
    prediction = model.predict_marginalised(rows)

    assert model.members[0].quantile_conditional["runs"] and "runs" not in model.members[0].quantile_direct
    # Where the zero mass reaches the 0.1 level the quantile is 0; a near-certain batter keeps a positive one.
    rarely_bats = prediction["p_bats"] <= 0.9
    assert rarely_bats.any() and np.all(prediction["runs"]["quantiles"][rarely_bats, 0] == 0.0)
    assert np.all(prediction["wickets"]["p0"] >= 1.0 - prediction["p_bowls"] - 1e-9)


def test_recalibration_is_fitted_on_the_last_quarter_and_applied(frame) -> None:
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(recalibrate=("runs",), targets=("runs",)))

    assert isinstance(model.calibration["runs"], QuantileRecalibration)
    assert model.metadata["n_calibration"] > 0
    assert pd.Timestamp(model.metadata["train_to"]) < train.match_date.max()
    prediction = model.predict_marginalised(train.tail(20))["runs"]["quantiles"]
    assert np.all(np.diff(prediction, axis=1) >= 0)


def test_degenerate_columns_fall_back_to_constants(frame) -> None:
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")].copy()
    train["catches"] = 0.0
    train["balls_faced"] = 1.0  # everyone bats

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec())
    prediction = model.predict_marginalised(train.head(5))

    assert isinstance(model.members[0].count_rate["catches"], P.ConstantEstimator)
    np.testing.assert_allclose(prediction["catches"]["mean"], 0.0, atol=1e-8)  # the rate floor
    np.testing.assert_allclose(prediction["p_bats"], 1.0)


def test_fit_refuses_too_few_rows(frame) -> None:
    with pytest.raises(ValueError, match="training rows"):
        P.fit_performance(frame.head(10), "T20", P.default_spec())
