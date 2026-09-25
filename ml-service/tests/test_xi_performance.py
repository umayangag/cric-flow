"""Unit tests for the performance model (L2-B): distributions, structures, marginalisation."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import perf_calibration
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
    plain = P.count_distribution([(np.array([0.0]), np.array([1.0]))])
    inflated = P.count_distribution([(np.array([0.5]), np.array([1.0]))])

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


def test_performance_feature_cols_read_only_the_kept_fixture_context_families() -> None:
    """The frame always carries the four fixture-context columns; the model reads a
    family's two only when gate A-1 kept it, and the spec records which."""
    none = C.performance_feature_cols(fixture_context_families=())
    venue = C.performance_feature_cols(fixture_context_families=("venue",))
    both = C.performance_feature_cols(fixture_context_families=("venue", "competition"))

    assert not set(none) & set(C.FIXTURE_CONTEXT_COLS)
    assert set(venue) - set(none) == set(C.FIXTURE_CONTEXT_FAMILIES["venue"])
    assert set(both) - set(none) == set(C.FIXTURE_CONTEXT_COLS)
    assert set(C.FIXTURE_CONTEXT_COLS) <= set(C.PLAYER_MATCH_FEATURE_COLS)
    assert P.default_spec(fixture_context_families=("competition",)).as_dict()["fixture_context_families"] == [
        "competition"
    ]


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
        return P.fit_performance(
            frame[frame.match_date < pd.Timestamp("2023-05-01")], "T20", P.default_spec(shared_factor=False)
        )


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
    assert fitted.metadata["n_train"] > 0 and "seeds" not in fitted.metadata["spec"]


def test_marginalised_prediction_mixes_both_innings(fitted, frame) -> None:
    """EVAL-14: the involvement probability is the mean of the two innings'; the quantiles
    are the mixture's -- the two oriented sets bracket each level, the mixture's median sits
    between the medians, and its 10-90 interval is never narrower than the level average
    the model used to serve."""
    rows = frame[frame.match_date >= pd.Timestamp("2023-05-01")].head(30)

    marginalised = fitted.predict_marginalised(rows)
    first = fitted.predict_oriented(rows, True)
    chase = fitted.predict_oriented(rows, False)

    np.testing.assert_allclose(marginalised["p_bats"], 0.5 * (first["p_bats"] + chase["p_bats"]))
    quantiles = marginalised["runs"]["quantiles"]
    low = np.minimum(first["runs"]["quantiles"], chase["runs"]["quantiles"])
    high = np.maximum(first["runs"]["quantiles"], chase["runs"]["quantiles"])
    assert np.all(quantiles >= low - 1e-9) and np.all(quantiles <= high + 1e-9)
    np.testing.assert_allclose(
        quantiles, P.mixture_of_quantile_sets([first["runs"]["quantiles"], chase["runs"]["quantiles"]])
    )
    averaged = 0.5 * (first["runs"]["quantiles"] + chase["runs"]["quantiles"])
    assert np.all(quantiles[:, 2] - quantiles[:, 0] >= averaged[:, 2] - averaged[:, 0] - 1e-9)


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
        model = P.fit_performance(
            train, "T20", P.default_spec(structure=structure, targets=("runs", "wickets"), shared_factor=False)
        )
    prediction = model.predict_marginalised(rows)

    assert model.members[0].quantile_conditional["runs"] and "runs" not in model.members[0].quantile_direct
    # Where the zero mass reaches the 0.1 level the quantile is 0; a near-certain batter keeps a positive one.
    rarely_bats = prediction["p_bats"] <= 0.9
    assert rarely_bats.any() and np.all(prediction["runs"]["quantiles"][rarely_bats, 0] == 0.0)
    assert np.all(prediction["wickets"]["p0"] >= 1.0 - prediction["p_bowls"] - 1e-9)


def test_recalibration_is_fitted_on_the_last_quarter_and_applied(frame) -> None:
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]

    with fast_fits():
        model = P.fit_performance(
            train, "T20", P.default_spec(recalibrate=("runs",), targets=("runs",), shared_factor=False)
        )

    recalibration = model.calibration["runs"]
    assert isinstance(recalibration, QuantileRecalibration)
    assert recalibration.n_fit == model.metadata["n_calibration"] > 0
    assert pd.Timestamp(model.metadata["calibration_from"]) == _calibration_fold_start(train)
    assert pd.Timestamp(model.metadata["train_to"]) == train.match_date.max()  # the served members saw the fold
    prediction = model.predict_marginalised(train.tail(20))["runs"]["quantiles"]
    assert np.all(np.diff(prediction, axis=1) >= 0)


def test_degenerate_columns_fall_back_to_constants(frame) -> None:
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")].copy()
    train["catches"] = 0.0
    train["balls_faced"] = 1.0  # everyone bats

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=False))
    prediction = model.predict_marginalised(train.head(5))

    assert isinstance(model.members[0].count_rate["catches"], P.ConstantEstimator)
    np.testing.assert_allclose(prediction["catches"]["mean"], 0.0, atol=1e-8)  # the rate floor
    np.testing.assert_allclose(prediction["p_bats"], 1.0)


def test_fit_refuses_too_few_rows(frame) -> None:
    with pytest.raises(ValueError, match="training rows"):
        P.fit_performance(frame.head(10), "T20", P.default_spec(shared_factor=False))


def _calibration_fold_start(rows: pd.DataFrame) -> pd.Timestamp:
    """The first date of the rows' temporal calibration fold (``CALIBRATION_DAYS`` before the last)."""
    return rows.match_date[rows.match_date >= rows.match_date.max() - pd.Timedelta(days=P.CALIBRATION_DAYS)].min()


def _record_member_fits(monkeypatch) -> list:
    """Every ``_fit_member`` call as (last training date, the member it returned)."""
    fits: list = []
    original = P._fit_member

    def recording(x, rows, spec, iterations):
        member = original(x, rows, spec, iterations)
        fits.append((rows.match_date.max(), member))
        return member

    monkeypatch.setattr(P, "_fit_member", recording)
    return fits


def _frames_for_shared_factor():
    from ml.xi import perf_baselines
    from ml.xi.builder import build
    from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history

    matches, _, _ = _synthetic_history(160)
    result = build(_ListSource(matches))
    return perf_baselines.add_baseline_predictors(result.player_frame), result.frame


def test_shared_factor_is_fitted_on_the_calibration_fold_from_complete_first_innings() -> None:
    player_frame, match_frame = _frames_for_shared_factor()
    train = player_frame[player_frame.match_date < pd.Timestamp("2023-06-01")]
    matches = match_frame.copy()
    matches["innings1_deliveries"] = 120.0  # the synthetic innings are short; call them complete

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=True), matches)

    factor = model.simulation.shared_factor
    assert factor is not None and factor.n_matches >= 30
    assert 0.0 <= factor.shrink <= 1.0 and np.all(factor.factors >= 0.0)
    assert model.metadata["simulation"]["shared_factor"]["n_matches"] == factor.n_matches
    fold_dates = train.drop_duplicates("match_id").set_index("match_id").match_date.loc[factor.sample_read.match_ids]
    assert (fold_dates >= _calibration_fold_start(train)).all()  # the residuals are the fold's
    assert pd.Timestamp(model.metadata["train_to"]) == train.match_date.max()  # the served members saw the fold
    assert model.metadata["n_train"] == len(train)
    assert 0.0 <= model.simulation.runs_balls_rho < 1.0


def test_shared_factor_is_skipped_with_a_warning_when_the_fold_is_thin() -> None:
    player_frame, match_frame = _frames_for_shared_factor()
    train = player_frame[player_frame.match_date < pd.Timestamp("2023-06-01")]

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=True), match_frame)

    assert model.simulation.shared_factor is None  # no synthetic first innings ran its overs
    assert model.metadata["simulation"]["shared_factor"] is None


def test_shared_factor_needs_the_match_frame() -> None:
    player_frame, _ = _frames_for_shared_factor()

    with pytest.raises(ValueError, match="match frame"):
        P.fit_performance(player_frame, "T20", P.default_spec(shared_factor=True))


def test_shared_factor_is_not_fitted_for_a_format_without_an_innings_length() -> None:
    """TEST has no simulator (H-17); its fit must not try to simulate the calibration fold."""
    player_frame, match_frame = _frames_for_shared_factor()
    train = player_frame[player_frame.match_date < pd.Timestamp("2023-06-01")].copy()
    train["format_code"] = "TEST"
    matches = match_frame.copy()
    matches["format_code"] = "TEST"
    matches["innings1_wickets"] = 10.0  # every first innings "complete" by the all-out rule

    with fast_fits():
        model = P.fit_performance(train, "TEST", P.default_spec(shared_factor=True), matches)

    assert model.simulation.shared_factor is None


def test_chase_response_is_fitted_on_the_calibration_folds_chases_beside_the_shared_factor() -> None:
    player_frame, match_frame = _frames_for_shared_factor()
    train = player_frame[player_frame.match_date < pd.Timestamp("2023-06-01")]
    matches = match_frame.copy()
    matches["innings1_deliveries"] = 120.0

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=True, chase_response="both"), matches)

    response = model.simulation.chase_response
    assert response is not None and response.arm == "both"
    assert len(response.sample) == model.simulation.shared_factor.n_matches
    assert np.isfinite([response.level, response.slope, response.sigma]).all() and response.sigma > 0
    assert model.metadata["simulation"]["chase_response"]["n_matches"] == len(response.sample)
    assert model.metadata["spec"]["chase_response"] == "both"


def test_chase_response_none_fits_nothing_and_needs_the_shared_factor() -> None:
    player_frame, match_frame = _frames_for_shared_factor()
    train = player_frame[player_frame.match_date < pd.Timestamp("2023-06-01")]
    matches = match_frame.copy()
    matches["innings1_deliveries"] = 120.0

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=True, chase_response="none"), matches)
    assert model.simulation.chase_response is None and model.metadata["simulation"]["chase_response"] is None
    with pytest.raises(ValueError, match="chase response needs the shared"):
        P.fit_performance(train, "T20", P.default_spec(shared_factor=False, chase_response="slope"))


def test_fold_parts_read_members_that_did_not_see_the_fold_and_the_served_members_are_refitted_on_every_row(
    monkeypatch,
) -> None:
    """Calibrate on the fold, refit on the full history (EVAL-03): the shared factor is
    fitted against members trained short of the fold, and the members served are a second
    fit on every row -- the fold included -- so the served forecast is not 92 days stale."""
    player_frame, match_frame = _frames_for_shared_factor()
    train = player_frame[player_frame.match_date < pd.Timestamp("2023-06-01")]
    matches = match_frame.copy()
    matches["innings1_deliveries"] = 120.0
    fits = _record_member_fits(monkeypatch)
    calibrated_against: list = []
    original = P._fit_simulator_calibration

    def recording(model, *args):
        calibrated_against.append(model.members[0])
        return original(model, *args)

    monkeypatch.setattr(P, "_fit_simulator_calibration", recording)

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=True), matches)

    assert len(fits) == 2  # the fold members, then the served ones (the iteration choice fits no member)
    (fold_last_date, fold_member), (served_last_date, served_member) = fits
    assert fold_last_date < _calibration_fold_start(train)
    assert served_last_date == train.match_date.max()
    assert calibrated_against == [fold_member]
    assert model.members == [served_member] and served_member is not fold_member
    assert model.simulation.shared_factor is not None


def test_recalibration_reads_the_fold_members_and_is_served_with_the_refitted_ones(monkeypatch, frame) -> None:
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]
    fits = _record_member_fits(monkeypatch)
    fitted_on: list = []
    original = QuantileRecalibration.fit

    def recording(predicted, y, levels=P.QUANTILE_LEVELS):
        fitted_on.append(predicted)
        return original(predicted, y, levels)

    monkeypatch.setattr(QuantileRecalibration, "fit", staticmethod(recording))

    with fast_fits():
        model = P.fit_performance(
            train, "T20", P.default_spec(recalibrate=("runs",), targets=("runs",), shared_factor=False)
        )

    (_, fold_member), (_, served_member) = fits
    fold_only = P.PerformanceModels("T20", model.spec, [fold_member], {}, {})
    fold_rows = train[train.match_date >= _calibration_fold_start(train)]
    np.testing.assert_allclose(fitted_on[0], fold_only.predict_marginalised(fold_rows)["runs"]["quantiles"])
    assert model.members == [served_member]


def test_without_fold_fitted_parts_the_members_are_fitted_once_on_every_row(monkeypatch, frame) -> None:
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]
    fits = _record_member_fits(monkeypatch)

    with fast_fits():
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=False))

    assert [date for date, _ in fits] == [train.match_date.max()]
    assert model.metadata["n_calibration"] == 0 and model.metadata["calibration_from"] is None
    assert model.metadata["n_train"] == len(train)


def test_a_history_too_short_to_hold_out_the_fold_fits_once_on_every_row_and_warns(monkeypatch, frame, caplog) -> None:
    train = frame[frame.match_date >= frame.match_date.max() - pd.Timedelta(days=P.CALIBRATION_DAYS + 8)]
    assert (train.match_date < _calibration_fold_start(train)).sum() < P.MIN_FIT_ROWS <= len(train)  # the scenario
    fits = _record_member_fits(monkeypatch)

    with fast_fits(), caplog.at_level("WARNING"):
        model = P.fit_performance(
            train, "T20", P.default_spec(recalibrate=("runs",), targets=("runs",), shared_factor=False)
        )

    assert [date for date, _ in fits] == [train.match_date.max()]
    assert model.calibration == {} and model.metadata["n_calibration"] == 0
    assert "no recalibration or shared factor is fitted" in caplog.text


def test_a_calibration_fold_too_thin_to_recalibrate_is_named_rather_than_fatal(frame) -> None:
    """EVAL-08: a sparse format's 92-day fold can hold fewer rows than a binned empirical
    quantile can be read off. The fit called ``QuantileRecalibration.fit`` unguarded, so
    the ValueError it raises below ``MIN_ROWS`` aborted the whole run. It now ships the
    uncorrected quantiles and records which targets went uncorrected."""
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]
    fold = train[train.match_date >= _calibration_fold_start(train)]
    thin = pd.concat(
        [train[train.match_date < _calibration_fold_start(train)], fold[fold.match_date == fold.match_date.max()]]
    )

    with fast_fits():
        model = P.fit_performance(
            thin, "T20", P.default_spec(recalibrate=("runs",), targets=("runs",), shared_factor=False)
        )

    assert 0 < model.metadata["n_calibration"] < perf_calibration.MIN_ROWS
    assert model.calibration == {}
    assert model.metadata["recalibrated"] == []
    assert model.metadata["recalibration_skipped"] == ["runs"]
    quantiles = model.predict_marginalised(thin.tail(20))["runs"]["quantiles"]
    assert np.all(np.diff(quantiles, axis=1) >= 0)


# --- EVAL-09: the iteration count is chosen on rows after the fit, never on a shuffle ------


def _fitted_boosters(member: P.SeedMember):
    """Every sklearn estimator a member holds, under the key its iteration count is recorded by."""
    for part, clf in member.involvement.items():
        yield f"p_{part}", clf
    for target, fitted in member.quantile_direct.items():
        for q, est in zip(P.QUANTILE_LEVELS, fitted):
            yield f"{target}_q{q:g}", est
    for target, fitted in member.quantile_conditional.items():
        for q, est in zip(P.CONDITIONAL_LEVELS, fitted):
            yield f"{target}_q{q:g}", est
    for target, est in member.count_rate.items():
        yield target, est


def test_the_iteration_choice_split_is_temporal_and_never_puts_a_match_on_both_sides(frame) -> None:
    """EVAL-09: sklearn's own early-stopping split drew a shuffled tenth of the rows, so the
    ten match-mates of a validation row -- sharing its side's 44 context columns -- were
    training rows. The choice fold is the most recent tenth by date, cut at a match
    boundary: every match is wholly on one side, and the later side comes after the earlier."""
    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]

    earlier, later = P._temporal_choice_split(train)

    assert earlier.sum() + later.sum() == len(train) and not np.any(earlier & later)
    assert set(train.match_id[earlier]).isdisjoint(train.match_id[later])
    assert train.match_date[earlier].max() < train.match_date[later].min()
    assert later.sum() >= P.ITERATION_CHOICE_FRACTION * len(train) > 0
    assert (train.match_id.value_counts()[train.match_id[later].unique()] == 22).all()  # whole matches, both sides


def test_every_served_booster_runs_its_chosen_count_with_sklearn_early_stopping_off(fitted) -> None:
    """The served fit is a fresh fit on every row for exactly the count chosen on the
    temporal fold, with no validation split of its own; a booster the fold was too thin to
    choose for runs the ceiling. On main every booster carried ``early_stopping=True``."""
    choice = fitted.metadata["iteration_choice"]

    boosters = dict(_fitted_boosters(fitted.members[0]))

    assert choice["reason"] == "each booster's own loss on the rows after the cut"
    assert set(choice["chosen"]) <= set(boosters) and choice["chosen"]  # something was chosen
    assert choice["n_fit"] + choice["n_choice"] == fitted.metadata["n_train"]
    assert 1 <= min(choice["chosen"].values()) and max(choice["chosen"].values()) <= choice["max_iter"]
    fitted_boosters = {key: est for key, est in boosters.items() if not isinstance(est, P.ConstantEstimator)}
    assert fitted_boosters and set(fitted_boosters) >= set(choice["chosen"])
    for key, est in fitted_boosters.items():
        assert est.early_stopping is False, key
        assert est.n_iter_ == choice["chosen"].get(key, choice["max_iter"]), key
    assert fitted.metadata["iterations"] == {key: est.n_iter_ for key, est in boosters.items()}
    assert len(fitted.members) == 1


def test_the_chosen_count_is_where_the_boosters_own_loss_on_the_later_rows_is_lowest(frame) -> None:
    """Reproduced for P(bats) with sklearn directly: a classifier fitted to the ceiling on
    the rows before the cut, its log loss on the rows after it read off staged
    predictions, and the chosen count is the iteration that loss is lowest at."""
    from sklearn.metrics import log_loss

    train = frame[frame.match_date < pd.Timestamp("2023-05-01")]
    spec = P.default_spec(shared_factor=False)
    earlier, later = P._temporal_choice_split(train)
    x = P.design_matrix(train, spec.feature_cols)
    label = (train.balls_faced.to_numpy() > 0).astype(int)

    with fast_fits():
        choice = P.choose_iterations(train, "T20", spec)
        reference = P.HistGradientBoostingClassifier(
            max_iter=P.MAX_ITER, random_state=P.RANDOM_STATE, early_stopping=False, **spec.hyperparameters
        ).fit(x[earlier], label[earlier])
    curve = [log_loss(label[later], p[:, 1], labels=[0, 1]) for p in reference.staged_predict_proba(x[later])]

    assert choice.iterations["p_bats"] == int(np.argmin(curve)) + 1
    assert choice.record["chosen"] == choice.iterations
    assert choice.record["choice_from"] == train.match_date[later].min().date().isoformat()


def test_a_history_too_short_to_cut_runs_the_ceiling_everywhere_and_warns(frame, caplog) -> None:
    last_dates = sorted(frame.match_date.unique())[-20:]
    train = frame[frame.match_date.isin(last_dates)]
    _, later = P._temporal_choice_split(train)
    assert later.sum() < P.MIN_FIT_ROWS <= len(train)  # enough to fit, not enough for its tenth to choose on

    with fast_fits(), caplog.at_level("WARNING"):
        model = P.fit_performance(train, "T20", P.default_spec(shared_factor=False, targets=("runs",)))

    assert model.metadata["iteration_choice"]["reason"] == "too few rows to choose on"
    assert model.metadata["iteration_choice"]["chosen"] == {}
    assert "every booster runs 40 iterations" in caplog.text
    assert {est.n_iter_ for _, est in _fitted_boosters(model.members[0])} == {40}


# --- EVAL-14: the toss is marginalised by mixing distributions, not by averaging parameters ---


class _ByInnings:
    """An estimator whose answer depends on the innings column alone: ``bats_first`` when
    the row bats first, ``chase`` otherwise."""

    n_iter_ = 1

    def __init__(self, bats_first: float, chase: float, innings_column: int):
        self.bats_first, self.chase, self.innings_column = bats_first, chase, innings_column

    def predict(self, x: np.ndarray) -> np.ndarray:
        return np.where(x[:, self.innings_column] == 1.0, self.bats_first, self.chase)

    def predict_proba(self, x: np.ndarray) -> np.ndarray:
        p = self.predict(x)
        return np.column_stack([1.0 - p, p])


def _innings_dependent_model(frame) -> P.PerformanceModels:
    """Runs (direct) at 10 / 30 / 60 batting first and 20 / 50 / 100 chasing; wickets
    (two-part) with P(bowls) 0.2 and rate 3.0 batting first, 1.0 and 0.5 chasing."""
    spec = P.default_spec(targets=("runs", "wickets"), shared_factor=False)
    column = spec.feature_cols.index(C.BATS_FIRST_COL)
    member = P.SeedMember(
        seed=0,
        involvement={"bowls": _ByInnings(0.2, 1.0, column)},
        quantile_direct={
            "runs": [_ByInnings(10.0, 20.0, column), _ByInnings(30.0, 50.0, column), _ByInnings(60.0, 100.0, column)]
        },
        quantile_conditional={},
        count_rate={"wickets": _ByInnings(3.0, 0.5, column)},
        iterations={},
    )
    return P.PerformanceModels("T20", spec, [member], {}, {})


def test_marginalised_quantiles_are_the_mixtures_not_the_level_average(frame) -> None:
    """The finding's own test. Level-by-level averaging served [15, 40, 80] for a batter
    whose two innings read [10, 30, 60] and [20, 50, 100]: a 10-90 interval of 65 that the
    mixture of the two -- under the reconstruction the simulator draws from -- puts at
    [12, 90]. The lower end is where the two piecewise-linear CDFs average to 0.1; the
    upper end is in the first innings' exponential tail."""
    model = _innings_dependent_model(frame)
    rows = frame.head(4)

    quantiles = model.predict_marginalised(rows)["runs"]["quantiles"]

    np.testing.assert_allclose(quantiles[:, 0], 12.0, atol=1e-6)
    assert np.all((quantiles[:, 1] > 30.0) & (quantiles[:, 1] < 50.0))
    np.testing.assert_allclose(quantiles[:, 2], 90.0, atol=0.05)
    assert np.all(quantiles[:, 2] - quantiles[:, 0] > 65.0)
    np.testing.assert_allclose(model.predict_oriented(rows, True)["runs"]["quantiles"], [[10.0, 30.0, 60.0]] * 4)


def test_marginalised_count_is_the_mixture_of_the_two_zero_inflated_poissons(frame) -> None:
    """Averaging P(bowls) and the rate separately gave a wickets mean of avg(p) * avg(rate)
    = 0.6 * 1.75 = 1.05 for a bowler who bowls a fifth of the time batting first at rate 3
    and always chasing at rate 0.5; the mixture's mean is avg(p * rate) = 0.55, and its
    P(0) the mean of the two innings' P(0)."""
    model = _innings_dependent_model(frame)
    rows = frame.head(3)

    wickets = model.predict_marginalised(rows)["wickets"]

    np.testing.assert_allclose(wickets["mean"], 0.55)
    np.testing.assert_allclose(wickets["p0"], 0.5 * ((0.8 + 0.2 * np.exp(-3.0)) + np.exp(-0.5)))
    np.testing.assert_allclose(wickets["p0"] + wickets["p1"] + wickets["p2plus"], 1.0)
    np.testing.assert_allclose(model.predict_marginalised(rows)["p_bowls"], 0.6)


def test_count_distribution_of_two_components_inverts_the_mean_cdf() -> None:
    """A mixture of a certain zero and a Poisson(4): P(0) = 0.5 + 0.5e^-4, and its quantiles
    are the smallest counts at which the mean CDF reaches each level -- 0 at 0.1 and 0.5,
    Poisson(4)'s 0.8-quantile (6) at 0.9 -- none of which the parameter average
    (zero inflation 0.5, rate 2) gives."""
    mixture = P.count_distribution([(np.array([1.0]), np.array([1e-9])), (np.array([0.0]), np.array([4.0]))])
    averaged = P.count_distribution([(np.array([0.5]), np.array([2.0]))])

    assert mixture["p0"][0] == pytest.approx(0.5 + 0.5 * np.exp(-4.0))
    assert mixture["mean"][0] == pytest.approx(2.0)
    np.testing.assert_allclose(mixture["quantiles"][0], [0.0, 0.0, 6.0])
    assert averaged["quantiles"][0][2] != 6.0


def test_mixture_of_one_quantile_set_is_that_set_and_of_identical_sets_is_the_same() -> None:
    quantiles = np.array([[0.0, 12.0, 40.0], [3.0, 20.0, 55.0]])

    np.testing.assert_allclose(P.mixture_of_quantile_sets([quantiles]), quantiles)
    np.testing.assert_allclose(P.mixture_of_quantile_sets([quantiles, quantiles]), quantiles)


def test_a_model_holding_other_than_one_member_is_refused(fitted) -> None:
    """EVAL-09 left one fit; the member average that used to sit in front of it is gone,
    and an artifact with any other count is named rather than served."""
    two = P.PerformanceModels("T20", fitted.spec, fitted.members * 2, {}, {})

    with pytest.raises(ValueError, match="2 performance members"):
        two.predict_oriented(pd.DataFrame(), True)
    assert fitted.member is fitted.members[0]
