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
    factor = S.SharedFactor(np.array([0.7, 1.0, 1.3]), 3, 0.06, 0.0, 1.0)

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

    fitted = S.fit_shared_factor(actual, simulated_mean, simulated_sd)

    assert 0.05 < np.std(fitted.factors) < 0.15  # near the true 0.1, never the raw residual sd
    assert fitted.shrink < 1.0 and fitted.within_variance == pytest.approx(0.01)
    with pytest.raises(ValueError, match="calibration matches"):
        S.fit_shared_factor(actual[:10], simulated_mean[:10], simulated_sd[:10])


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
