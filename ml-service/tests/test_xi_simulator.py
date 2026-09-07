"""Unit tests for the match simulator (ml.xi.simulator): the design's identities hold on
every draw, the distribution reconstruction is exact at the fitted quantiles, the chase
ends at the target, the toss is marginalised, and the shared factor is deconvolved."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import simulator as S


def _side(seed: int, strength: float = 1.0) -> S.SideForecast:
    """A plausible T20 eleven: openers who nearly always bat, a tail that rarely does, five
    bowlers with expected balls, forecasts as (q10, q50, q90)."""
    rng = np.random.default_rng(seed)
    slot = np.arange(1, 12, dtype=float) + rng.random(11) * 0.1
    p_bats = np.clip(1.05 - 0.09 * np.arange(11), 0.05, 1.0)
    q50 = np.maximum(28.0 - 3.0 * np.arange(11), 0.0) * p_bats
    # ``strength`` scales runs, not balls: a weaker side scores slower off the same deliveries.
    runs = np.column_stack([np.where(p_bats > 0.9, q50 * 0.2, 0.0), q50, q50 * 2.2 + 8.0]) * strength
    balls = np.column_stack([np.where(p_bats > 0.9, q50 * 0.25, 0.0), q50 * 0.9, q50 * 1.8 + 8.0])
    exp_balls_bowled = np.array([0, 0, 0, 0, 0, 0, 12, 24, 24, 24, 24], dtype=float)
    p_bowls = np.array([0.02, 0.02, 0.02, 0.02, 0.05, 0.3, 0.7, 0.98, 0.98, 0.98, 0.98])
    conceded = np.column_stack([exp_balls_bowled * 0.8, exp_balls_bowled * 1.3, exp_balls_bowled * 1.9])
    return S.SideForecast(
        player_keys=np.array([f"p{seed}_{i}" for i in range(11)], dtype=object),
        exp_bat_position=slot,
        exp_balls_bowled=exp_balls_bowled,
        p_bats=p_bats,
        p_bowls=p_bowls,
        runs=runs,
        balls=balls,
        conceded=conceded,
        wickets_mean=exp_balls_bowled / 24.0 * 1.2,
    )


def _teams(strength2: float = 1.0):
    return S.SideForecasts(_side(1), _side(11)), S.SideForecasts(_side(2, strength2), _side(12, strength2))


def _calibration_sample(n: int) -> S.SharedFactorCalibrationSample:
    """A stand-in for the evidence a fitted shared factor keeps, where a test builds the
    factor by hand rather than fitting it."""
    return S.SharedFactorCalibrationSample(
        np.arange(n).astype(object), np.full(n, 150.0), np.full(n, 150.0), np.full(n, 15.0)
    )


CONTEXT = S.MatchContext("T20", extras_per_ball=0.08, innings_deliveries=124.0, bowler_wicket_share=0.9)
CALIBRATION = S.SimulatorCalibration(runs_balls_rho=0.9)


@pytest.fixture(scope="module")
def draws() -> S.MatchDraws:
    team1, team2 = _teams()
    return S.simulate_match(team1, team2, CONTEXT, n=1500, seed=3, calibration=CALIBRATION)


def test_quantile_function_is_exact_at_the_fitted_levels_and_monotone() -> None:
    quantiles = np.array([[0.0, 12.0, 40.0], [3.0, 20.0, 55.0]])  # two players
    level = np.tile(np.array([0.1, 0.5, 0.9]), (2, 1)).T  # (draws, players)

    at_knots = S.quantile_function(quantiles, level)
    fine = S.quantile_function(quantiles, np.tile(np.linspace(0.0, 0.999, 200), (2, 1)).T)

    np.testing.assert_allclose(at_knots, quantiles.T)
    assert (np.diff(fine, axis=0) >= -1e-9).all()
    assert fine[0, 0] == 0.0 and fine[-1, 1] > 55.0  # zero at level 0, a tail above q90


def test_every_draw_respects_the_laws_and_the_identities(draws) -> None:
    for team, other in ((draws.team1, draws.team2), (draws.team2, draws.team1)):
        np.testing.assert_allclose(team.runs.sum(axis=1) + team.extras, team.total)
        np.testing.assert_allclose(other.conceded.sum(axis=1), team.total)  # bowlers' figures sum to the innings
        assert (team.deliveries <= CONTEXT.deliveries).all()
        assert (team.wickets_lost <= C.MAX_WICKETS).all()
        assert (other.wickets.sum(axis=1) <= team.wickets_lost).all()  # the rest are run-outs
        assert (team.bowled_balls <= CONTEXT.bowler_cap).all()
        assert ((team.balls > 0) == (team.runs > 0) | (team.balls > 0)).all()  # runs only from balls faced


def test_the_chase_ends_at_the_target_and_never_beyond(draws) -> None:
    first_total = np.where(draws.team1_bats_first, draws.team1.total, draws.team2.total)
    chase_total = np.where(draws.team1_bats_first, draws.team2.total, draws.team1.total)

    chaser_won = chase_total > first_total
    assert chaser_won.any() and (~chaser_won).any()
    np.testing.assert_array_equal(chase_total[chaser_won], first_total[chaser_won] + 1)
    assert (draws.winner == np.where(chaser_won, np.where(draws.team1_bats_first, 2, 1), draws.winner)).all()


def test_toss_is_marginalised_unless_known() -> None:
    team1, team2 = _teams()

    unknown = S.simulate_match(team1, team2, CONTEXT, n=400, seed=1, calibration=CALIBRATION)
    known = S.simulate_match(team1, team2, CONTEXT, n=400, seed=1, team1_bats_first=False, calibration=CALIBRATION)

    assert unknown.team1_bats_first.sum() == 200 and not known.team1_bats_first.any()
    assert S.summarize(unknown)["toss_marginalised"] and not S.summarize(known)["toss_marginalised"]


def test_a_stronger_side_wins_more_often() -> None:
    team1, weaker = _teams(strength2=0.7)

    p = S.summarize(S.simulate_match(team1, weaker, CONTEXT, n=1000, seed=0, calibration=CALIBRATION))["win"]

    assert p["team1"] > 0.65 and abs(p["team1"] + p["team2"] + p["tie"] - 1.0) < 1e-9


def test_median_band_scorecard_sums_to_the_side_total_by_construction(draws) -> None:
    summary = S.summarize_team(draws.team1)

    lines = sum(p["scorecard"]["runs"] for p in summary["players"]) + summary["extras"]["scorecard"]
    shares = sum(p["spread_share"] for p in summary["players"]) + summary["extras"]["spread_share"]

    assert lines == pytest.approx(summary["total"]["scorecard"])
    assert abs(summary["total"]["scorecard"] - summary["total"]["median"]) <= 2.0
    assert shares == pytest.approx(1.0)
    assert summary["total"]["q10"] <= summary["total"]["median"] <= summary["total"]["q90"]


def test_draws_are_deterministic_given_inputs_and_seed() -> None:
    team1, team2 = _teams()

    a = S.simulate_match(team1, team2, CONTEXT, n=300, seed=7, calibration=CALIBRATION)
    b = S.simulate_match(team1, team2, CONTEXT, n=300, seed=7, calibration=CALIBRATION)
    c = S.simulate_match(team1, team2, CONTEXT, n=300, seed=8, calibration=CALIBRATION)

    np.testing.assert_array_equal(a.team1.runs, b.team1.runs)
    assert not np.array_equal(a.team1.total, c.team1.total)


def test_comonotonic_draws_collapse_the_spread_and_the_copula_restores_it() -> None:
    """The measurement that changed the design: with runs and balls fully coupled under the
    balls budget the total's spread collapses; a partial coupling keeps it."""
    team1, team2 = _teams()

    tight = S.simulate_match(team1, team2, CONTEXT, 800, 0, True, S.SimulatorCalibration(0.999))
    loose = S.simulate_match(team1, team2, CONTEXT, 800, 0, True, S.SimulatorCalibration(0.8))

    assert tight.team1.total.std() < 0.5 * loose.team1.total.std()


def test_shared_factor_widens_totals_and_is_shared_by_both_innings() -> None:
    team1, team2 = _teams()
    factor = S.SharedFactor(np.array([0.7, 1.0, 1.3]), 3, 0.06, 0.0, 1.0, _calibration_sample(3))

    without = S.simulate_match(team1, team2, CONTEXT, 800, 0, True, S.SimulatorCalibration(0.9))
    with_factor = S.simulate_match(team1, team2, CONTEXT, 800, 0, True, S.SimulatorCalibration(0.9, factor))

    assert with_factor.team1.total.std() > 1.2 * without.team1.total.std()
    # A common factor makes the two innings' runs move together.
    assert np.corrcoef(with_factor.team1.total, with_factor.team2.runs.sum(axis=1))[0, 1] > 0.2


def test_fit_shared_factor_deconvolves_the_simulators_own_dispersion() -> None:
    rng = np.random.default_rng(0)
    simulated_mean = np.full(200, 150.0)
    simulated_sd = np.full(200, 15.0)
    true_factor = rng.normal(1.0, 0.1, 200)
    actual = simulated_mean * true_factor + rng.normal(0.0, 15.0, 200)
    sample = S.SharedFactorCalibrationSample(np.arange(200).astype(object), actual, simulated_mean, simulated_sd)

    fitted = S.fit_shared_factor(sample)

    assert 0.05 < np.std(fitted.factors) < 0.15  # near the true 0.1, never the raw residual sd
    assert fitted.shrink < 1.0 and fitted.within_variance == pytest.approx(0.01)
    assert len(fitted.sample_read) == 200  # the evidence is kept so an arm can refit from it
    with pytest.raises(ValueError, match="calibration matches"):
        S.fit_shared_factor(sample.take(np.arange(10)))


def test_shared_factor_calibration_sample_take_selects_the_same_matches_in_every_column() -> None:
    sample = S.SharedFactorCalibrationSample(
        np.array(["a", "b", "c"], dtype=object),
        np.array([150.0, 160.0, 170.0]),
        np.array([148.0, 158.0, 168.0]),
        np.array([15.0, 16.0, 17.0]),
    )

    taken = sample.take(np.array([False, True, True]))

    assert list(taken.match_ids) == ["b", "c"]
    assert list(taken.actual) == [160.0, 170.0]
    assert list(taken.simulated_mean) == [158.0, 168.0]
    assert list(taken.simulated_sd) == [16.0, 17.0]


def test_runs_balls_copula_rho_from_rank_correlation() -> None:
    rng = np.random.default_rng(0)
    balls = rng.integers(0, 40, 500).astype(float)
    runs = np.where(balls > 0, balls * 1.2 + rng.normal(0, 3, 500), 0.0)

    rho = S.runs_balls_copula_rho(runs, balls)

    assert 0.9 < rho < 0.999
    assert S.runs_balls_copula_rho(np.ones(50), np.ones(50)) == 0.0  # constant: undefined, so none


def test_bowling_attribution_drafts_enough_bowlers_for_the_innings() -> None:
    side = _side(5)
    rng = np.random.default_rng(0)
    innings = S.InningsDraws(
        runs=np.zeros((50, 11)),
        balls=np.zeros((50, 11)),
        extras=np.zeros(50),
        total=np.full(50, 160.0),
        wickets=np.full(50, 6.0),
        deliveries=np.full(50, 124.0),
        reached_target=np.zeros(50, dtype=bool),
        untruncated_total=np.full(50, 160.0),
    )

    bowling = S.bowling_attribution(rng, side, CONTEXT, innings)

    assert ((bowling.balls > 0).sum(axis=1) >= 5).all()  # a fifth each at most, so five at least
    np.testing.assert_allclose(bowling.conceded.sum(axis=1), 160.0)
    assert (bowling.wickets.sum(axis=1) <= 6).all()


def test_simulate_match_refuses_a_format_without_an_innings_length() -> None:
    team1, team2 = _teams()

    with pytest.raises(S.SimulationUnavailable):
        S.simulate_match(team1, team2, S.MatchContext("TEST", 0.05, 0.0, 0.9), n=10)


def test_complete_first_innings_means_all_out_or_the_overs_bowled() -> None:
    rows = pd.DataFrame(
        {
            "format_code": ["T20", "T20", "T20", "ODI"],
            "innings1_wickets": [10.0, 4.0, 5.0, 3.0],
            "innings1_deliveries": [90.0, 123.0, 80.0, 300.0],
        }
    )

    np.testing.assert_array_equal(S.complete_first_innings(rows), [True, True, False, True])


def test_fixtures_from_rows_build_both_orientations_per_side() -> None:
    keys = [f"a{i}" for i in range(11)] + [f"b{i}" for i in range(11)]
    rows = pd.DataFrame(
        {
            "match_id": ["m1"] * 22,
            "side": [1] * 11 + [2] * 11,
            "player_key": keys,
            "exp_bat_position": list(range(1, 12)) * 2,
            "exp_balls_bowled": [0.0] * 6 + [24.0] * 5 + [0.0] * 6 + [24.0] * 5,
        }
    )
    win_rows = pd.DataFrame(
        [
            {
                "match_id": "m1",
                "format_code": "T20",
                "ctx_extras_per_ball": 0.07,
                "ctx_innings_deliveries": 123.0,
                "ctx_bowler_wicket_share": 0.88,
            }
        ]
    )
    calls = []

    def predict(frame: pd.DataFrame, bats_first):
        calls.append(bats_first)
        n = len(frame)
        q = np.column_stack([np.zeros(n), np.full(n, 10.0 + (5.0 if bats_first else 0.0)), np.full(n, 30.0)])
        return {
            "p_bats": np.full(n, 0.8),
            "p_bowls": np.full(n, 0.4),
            "runs": {"quantiles": q},
            "balls_faced": {"quantiles": q},
            "runs_conceded": {"quantiles": q},
            "wickets": {"mean": np.full(n, 0.5)},
        }

    fixtures = S.fixtures_from_rows(rows, win_rows, predict)

    assert calls == [True, False] and len(fixtures) == 1
    fixture = fixtures[0]
    assert (
        list(fixture.team1.bat_first.player_keys) == keys[:11]
        and list(fixture.team2.bat_first.player_keys) == keys[11:]
    )
    assert fixture.team1.bat_first.runs[0, 1] == 15.0 and fixture.team1.chasing.runs[0, 1] == 10.0
    assert fixture.context == S.MatchContext("T20", 0.07, 123.0, 0.88)


# --- the chase response (plan §8.10, gate A-2) -----------------------------------------


def _chase_sample(
    n: int, level: float, slope: float, sigma: float, seed: int = 0, simulated_log_sd: float = 0.0
) -> S.ChaseCalibrationSample:
    """Calibration chases drawn from the response's own model: a log difficulty, a log
    response around level + slope * x, and a win wherever the response reached it.
    ``simulated_log_sd`` is what the draws' own spread was on those matches."""
    rng = np.random.default_rng(seed)
    difficulty = rng.normal(0.0, 0.2, n)
    response = level + slope * difficulty + rng.normal(0.0, sigma, n)
    return S.ChaseCalibrationSample(difficulty, response, response >= difficulty, np.full(n, simulated_log_sd))


def test_fit_chase_response_recovers_the_coefficients_under_censoring() -> None:
    sample = _chase_sample(3000, level=0.02, slope=-0.4, sigma=0.15)
    no_level = _chase_sample(3000, level=0.0, slope=-0.4, sigma=0.15, seed=1)

    both = S.fit_chase_response(sample, "both")
    slope_only = S.fit_chase_response(no_level, "slope")
    level_only = S.fit_chase_response(sample, "level")

    assert both is not None and both.slope == pytest.approx(-0.4, abs=0.04)
    assert both.level == pytest.approx(0.02, abs=0.02) and both.sigma == pytest.approx(0.15, abs=0.02)
    assert slope_only is not None and slope_only.level == 0.0 and slope_only.slope == pytest.approx(-0.4, abs=0.05)
    assert level_only is not None and level_only.slope == 0.0
    assert both.as_dict()["n_won"] == int(sample.censored.sum())


def test_fit_chase_response_none_fits_nothing_and_guards_thin_folds() -> None:
    sample = _chase_sample(40, level=0.0, slope=-0.3, sigma=0.1)

    assert S.fit_chase_response(sample, "none") is None
    with pytest.raises(ValueError, match="calibration chases"):
        S.fit_chase_response(_chase_sample(10, 0.0, -0.3, 0.1), "both")
    with pytest.raises(ValueError, match="lost chases"):
        S.fit_chase_response(
            S.ChaseCalibrationSample(sample.difficulty, sample.response, np.ones(40, bool), np.zeros(40)), "both"
        )
    with pytest.raises(ValueError, match="unknown chase response arm"):
        S.fit_chase_response(sample, "collapse")


def test_chase_calibration_sample_takes_the_pitch_out_of_the_difficulty() -> None:
    actual_first = np.array([180.0, 120.0])
    actual_chase = np.array([181.0, 90.0])
    factors = np.array([1.2, 0.8])
    chase_expected = np.array([150.0, 150.0])

    sample = S.chase_calibration_sample(
        actual_first, actual_chase, np.array([True, False]), chase_expected, np.array([0.2, 0.3]), factors
    )

    np.testing.assert_allclose(sample.difficulty, np.log([181.0 / 180.0, 121.0 / 120.0]))
    np.testing.assert_allclose(sample.response, np.log([181.0 / 180.0, 90.0 / 120.0]))
    assert sample.censored.tolist() == [True, False]
    np.testing.assert_allclose(sample.simulated_log_sd, [0.2, 0.3])


def test_a_negative_slope_lowers_hard_chases_and_raises_easy_ones() -> None:
    team1, team2 = _teams()
    sample = _chase_sample(200, level=0.0, slope=-0.5, sigma=0.1)
    response = S.ChaseResponse("slope", 0.0, -0.5, 0.1, sample)
    without = S.simulate_match(team1, team2, CONTEXT, 1500, 0, True, S.SimulatorCalibration(0.9))
    with_response = S.simulate_match(team1, team2, CONTEXT, 1500, 0, True, S.SimulatorCalibration(0.9, None, response))

    # Common random numbers: the first innings is untouched, so the targets are the same.
    np.testing.assert_array_equal(without.team1.total, with_response.team1.total)
    hard = without.team1.total > np.quantile(without.team1.total, 0.75)
    easy = without.team1.total < np.quantile(without.team1.total, 0.25)
    assert with_response.team2.untruncated_total[hard].mean() < without.team2.untruncated_total[hard].mean() - 5
    assert with_response.team2.untruncated_total[easy].mean() > without.team2.untruncated_total[easy].mean() + 5
    # The chase still ends at the target and never beyond.
    chaser_won = with_response.team2.total > with_response.team1.total
    np.testing.assert_array_equal(with_response.team2.total[chaser_won], with_response.team1.total[chaser_won] + 1)


def test_a_zero_response_is_todays_simulator() -> None:
    team1, team2 = _teams()
    response = S.ChaseResponse("both", 0.0, 0.0, 0.1, _chase_sample(40, 0.0, 0.0, 0.1))

    without = S.simulate_match(team1, team2, CONTEXT, 600, 0, True, S.SimulatorCalibration(0.9))
    with_zero = S.simulate_match(team1, team2, CONTEXT, 600, 0, True, S.SimulatorCalibration(0.9, None, response))

    np.testing.assert_array_equal(without.team2.total, with_zero.team2.total)
    np.testing.assert_array_equal(without.team2.runs, with_zero.team2.runs)


def test_calibration_draws_carry_the_chases_expected_untruncated_total() -> None:
    team1, team2 = _teams()
    fixture = S.Fixture("m1", team1, team2, CONTEXT)

    draws = S.simulate_calibration_fixtures([fixture], 0.9, 300, seed=0)

    assert draws.first_mean.shape == (1,) and draws.first_sd[0] > 0
    # The chasing side's expected total is its innings before the truncation: above the
    # truncated chase's mean, which the target caps.
    reference = S.simulate_match(team1, team2, CONTEXT, 300, 0, True, S.SimulatorCalibration(0.9))
    assert draws.chase_expected[0] == pytest.approx(reference.team2.untruncated_total.mean())
    assert draws.chase_expected[0] > reference.team2.total.mean()


# --- the chase's own dispersion (plan §8.14, gate B-11) ---------------------------------


def test_fit_chase_dispersion_deconvolves_the_draws_own_spread() -> None:
    sample = _chase_sample(4000, level=0.0, slope=0.0, sigma=0.30, simulated_log_sd=0.18)

    fitted = S.fit_chase_dispersion(sample)

    assert fitted.fitted_log_sd == pytest.approx(0.30, abs=0.02)
    assert fitted.simulated_log_sd == pytest.approx(0.18, abs=1e-12)
    assert fitted.excess_log_sd == pytest.approx(np.sqrt(fitted.fitted_log_sd**2 - 0.18**2), abs=1e-9)
    assert fitted.n_matches == 4000 and fitted.n_won == int(sample.censored.sum())


def test_a_chase_already_as_wide_as_the_data_gets_no_extra_dispersion() -> None:
    sample = _chase_sample(2000, level=0.0, slope=0.0, sigma=0.15, simulated_log_sd=0.6)

    fitted = S.fit_chase_dispersion(sample)

    assert fitted.excess_log_sd == 0.0
    np.testing.assert_array_equal(fitted.sample(np.random.default_rng(0), 100), np.ones(100))


def test_the_dispersion_term_is_mean_one_so_it_adds_spread_and_no_level() -> None:
    fitted = S.ChaseDispersion(0.40, 0.20, 0.3464, 500, 240)

    factors = fitted.sample(np.random.default_rng(0), 200_000)

    assert factors.mean() == pytest.approx(1.0, abs=0.005)
    assert np.log(factors).std() == pytest.approx(0.3464, abs=0.005)


def test_the_chase_widens_and_the_first_innings_does_not_move() -> None:
    team1, team2 = _teams()
    pool = S.SharedFactor(np.linspace(0.8, 1.2, 60), 60, 0.02, 0.01, 0.7, _calibration_sample(60))
    control = S.SimulatorCalibration(0.9, pool)
    widened = S.SimulatorCalibration(0.9, pool, None, S.ChaseDispersion(0.40, 0.20, 0.3464, 500, 240))

    without = S.simulate_match(team1, team2, CONTEXT, 4000, 0, True, control)
    with_term = S.simulate_match(team1, team2, CONTEXT, 4000, 0, True, widened)

    # The first innings is drawn before the chase, from the same stream: bit-identical.
    np.testing.assert_array_equal(without.team1.total, with_term.team1.total)
    assert with_term.team2.untruncated_total.std() > without.team2.untruncated_total.std() * 1.2
    # The chase still ends at the target and never beyond it.
    chaser_won = with_term.team2.total > with_term.team1.total
    np.testing.assert_array_equal(with_term.team2.total[chaser_won], with_term.team1.total[chaser_won] + 1)


def test_the_dispersion_term_is_reported_beside_the_shared_factor() -> None:
    fitted = S.ChaseDispersion(0.40, 0.20, 0.3464, 500, 240)

    reported = S.SimulatorCalibration(0.9, None, None, fitted).as_dict()

    assert reported["chase_dispersion"] == {
        "fitted_log_sd": 0.40,
        "simulated_log_sd": 0.20,
        "excess_log_sd": 0.3464,
        "n_matches": 500,
        "n_won": 240,
    }
    assert S.SimulatorCalibration(0.9).as_dict()["chase_dispersion"] is None


def test_calibration_draws_carry_the_chases_own_log_spread() -> None:
    team1, team2 = _teams()
    fixture = S.Fixture("m1", team1, team2, CONTEXT)

    draws = S.simulate_calibration_fixtures([fixture], 0.9, 300, seed=0)

    reference = S.simulate_match(team1, team2, CONTEXT, 300, 0, True, S.SimulatorCalibration(0.9))
    expected = np.log(np.maximum(reference.team2.untruncated_total, 1.0)).std()
    assert draws.chase_log_sd[0] == pytest.approx(expected)
