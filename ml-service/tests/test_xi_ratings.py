"""Unit tests for the XI-responsive rating pass (ml.xi)."""

from __future__ import annotations

from dataclasses import replace
from datetime import date, timedelta
from typing import List

import numpy as np
import pytest

from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.ratings import (
    STATE_ARRAY_NAMES,
    STATE_TABLE_NAMES,
    RatingState,
    aggregate_side,
    match_features,
)
from ml.xi.roles import ROLE_BOWLING_OPTION, count_bowling_options, roles_of
from ml.xi.sources import Deliveries, MatchRecord


def _deliveries(batters: List[str], bowlers: List[str], runs: List[int], wickets: List[int]) -> Deliveries:
    n = len(batters)
    return Deliveries(
        over=np.arange(n) // 6,
        innings=np.zeros(n, dtype=int),
        batter=np.asarray(batters, dtype=object),
        bowler=np.asarray(bowlers, dtype=object),
        runs_batter=np.asarray(runs, dtype=float),
        runs_total=np.asarray(runs, dtype=float),
        runs_bowler=np.asarray(runs, dtype=float),
        faced=np.ones(n),
        wicket=np.asarray(wickets, dtype=float),
        bowler_wicket=np.asarray(wickets, dtype=float),
        stumping=np.zeros(n),
        fielders=[[] for _ in range(n)],
        # a wicket is the striker's own dismissal unless a test says otherwise (FEAT-07)
        players_out=[[batter] if wicket else [] for batter, wicket in zip(batters, wickets)],
    )


def _match(
    mid: str, day: int, winner: str, t1: List[str], t2: List[str], deliveries: Deliveries, fmt: str = "T20"
) -> MatchRecord:
    return MatchRecord(
        match_id=mid,
        match_date=date(2024, 1, 1) + timedelta(days=day),
        format_code=fmt,
        team1="A",
        team2="B",
        venue="V",
        gender="male",
        team1_players=t1,
        team2_players=t2,
        winner=winner,
        result=None,
        deliveries=deliveries,
    )


def _xi(prefix: str) -> List[str]:
    return [f"{prefix}{i}" for i in range(11)]


class _ListSource:
    def __init__(self, matches):
        self.matches = matches

    def iter_matches(self):
        yield from self.matches

    def birth_dates(self):
        return {}

    def venue_countries(self):
        return {}


def test_every_accumulator_the_pass_keeps_is_named_in_the_state_field_lists() -> None:
    """Three things walk these tuples -- the artifact writer, its D-6 refusal and
    ``snapshot`` -- so an accumulator missing from them is written by the pass and read
    back by none of them: absent from the run on disk, and shared rather than copied with
    the serving snapshot."""
    state = RatingState()

    arrays = {name for name, value in vars(state).items() if isinstance(value, np.ndarray)}
    tables = {name for name, value in vars(state).items() if isinstance(value, dict)}

    assert arrays == set(STATE_ARRAY_NAMES)
    assert tables == set(STATE_TABLE_NAMES)


def test_a_snapshot_keeps_the_ratings_it_was_taken_from() -> None:
    """SERVE-01: the snapshot is what a serving request is handed while the pass that
    produced it advances. Every accumulator has to be copied -- the arrays, the Elo tables
    and the form lists the update appends to -- or the copy moves with the pass."""
    t1, t2 = _xi("a"), _xi("b")
    heavy = _deliveries([t1[0]] * 24, [t2[5]] * 24, [6] * 24, [0] * 24)
    state = RatingState()
    state.update(_match("m1", 0, "A", t1, t2, heavy))
    frozen = state.snapshot()
    before = {key: np.copy(value) for key, value in frozen.side_vectors("T20", t1).items()}

    state.update(_match("m2", 1, "A", t1, t2, heavy))

    assert state.matches_seen == 2 and frozen.matches_seen == 1
    assert len(state.team_results[("T20", "A")]) == 2 and len(frozen.team_results[("T20", "A")]) == 1
    after = frozen.side_vectors("T20", t1)
    for key, value in before.items():
        np.testing.assert_array_equal(after[key], value)
    assert state.side_vectors("T20", t1)["career"][0] > before["career"][0], "the pass did move on"


def test_features_are_as_of_and_never_see_their_own_match() -> None:
    """The row for match k is identical whether or not later matches exist, and a match's
    own result does not move its own row."""
    t1, t2 = _xi("a"), _xi("b")
    heavy = _deliveries([t1[0]] * 24, [t2[5]] * 24, [6] * 24, [0] * 24)  # a0 dominates
    m1 = _match("m1", 0, "A", t1, t2, heavy)
    m2 = _match("m2", 1, "A", t1, t2, heavy)
    m3 = _match("m3", 2, "B", t1, t2, heavy)

    short = build(_ListSource([m1, m2])).frame
    long = build(_ListSource([m1, m2, m3])).frame

    first_short = short[short.match_id == "m1"].iloc[0]
    first_long = long[long.match_id == "m1"].iloc[0]
    for col in C.XI_FEATURE_COLS + C.TEAM_CONTEXT_COLS + C.FIXTURE_CONTEXT_COLS:
        assert first_short[col] == pytest.approx(first_long[col])
    # a match's own scoreboard never reaches its own row: m1 is the first at the ground
    for col in C.FIXTURE_CONTEXT_COLS:
        assert first_short[col] == 1.0
    # before any history the two sides are indistinguishable
    assert first_short["d_imp_bat_sum"] == pytest.approx(0.0)
    assert first_short["team_elo_diff"] == pytest.approx(0.0)
    # after m1, team A's batting impact and Elo lead
    second = long[long.match_id == "m2"].iloc[0]
    assert second["d_imp_bat_sum"] > 0
    assert second["team_elo_diff"] > 0
    assert second["d_pelo_mean"] > 0


def test_impact_rating_rewards_runs_above_context_and_penalises_wickets() -> None:
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    # a0 scores 6 every ball; a1 gets out every ball for 0 against the same bowler
    d = _deliveries([t1[0]] * 12 + [t1[1]] * 12, [t2[0]] * 24, [6] * 12 + [0] * 12, [0] * 12 + [1] * 12)
    state.update(_match("m", 0, "A", t1, t2, d))

    v = state.side_vectors("T20", [t1[0], t1[1], "unknown"])
    assert v["bat_rate"][0] > 0 > v["bat_rate"][1]
    assert v["bat_wrate"][0] > v["bat_wrate"][1]
    assert v["exp_balls_faced"][0] == pytest.approx(12.0)
    assert v["exp_balls_faced"][2] == 0.0, "an unseen player has no involvement and neutral rates"
    assert v["bat_rate"][2] == 0.0
    assert v["pelo"][2] == C.ELO_INITIAL
    vb = state.side_vectors("T20", [t2[0]])
    assert vb["exp_balls_bowled"][0] == pytest.approx(24.0)
    assert vb["bowl_wrate"][0] > 0, "twelve wickets in 24 balls is far above the baseline"


def _decayed_appearances(n: int) -> float:
    """What ``xi_n`` reads after ``n`` consecutive appearances: the latest undecayed."""
    return sum(C.DECAY_PER_MATCH**j for j in range(n))


@pytest.mark.parametrize("batted_in", [0, 9], ids=["first of ten", "last of ten"])
def test_involvement_is_balls_per_xi_appearance_not_per_match_batted(batted_in: int) -> None:
    """A tailender in ten XIs who batted once for 30 balls reads about 3 balls per
    appearance -- the 30 forgotten at the XI clock over the appearances since -- not the
    30 an opener who faces 30 every match reads (FEAT-01)."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    opener, tailender, stand_in, bowler = t1[0], t1[10], t1[1], t2[0]
    for k in range(10):
        second = tailender if k == batted_in else stand_in
        d = _deliveries([opener] * 30 + [second] * 30, [bowler] * 60, [1] * 60, [0] * 60)
        state.update(_match(f"m{k}", k, "A", t1, t2, d))

    v = state.side_vectors("T20", [opener, tailender])

    appearances_since = 9 - batted_in
    expected = 30.0 * C.DECAY_PER_MATCH**appearances_since / _decayed_appearances(10)
    assert v["exp_balls_faced"][1] == pytest.approx(expected)
    assert v["exp_balls_faced"][1] < 5.0, "one innings in ten is not opener-level involvement"
    assert v["exp_balls_faced"][0] == pytest.approx(30.0), "constant involvement reads as itself on any clock"


def test_a_part_timer_with_one_spell_is_not_a_bowling_option() -> None:
    """Two overs bowled once in ten appearances read as 12 / (decayed ten), under the T20
    threshold, so the same predicate that counts ``n_bowlers`` and names the optimiser's
    bowling cover stops counting him; a bowler who bowls two overs every match still clears
    it exactly (FEAT-01)."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    frontline, part_timer = t2[0], t2[10]
    for k in range(10):
        bowlers = [frontline] * 12 + ([part_timer] * 12 if k == 9 else [])
        d = _deliveries([t1[0]] * len(bowlers), bowlers, [1] * len(bowlers), [0] * len(bowlers))
        state.update(_match(f"m{k}", k, "A", t1, t2, d))

    v = state.side_vectors("T20", t2)

    assert v["exp_balls_bowled"][10] == pytest.approx(12.0 / _decayed_appearances(10))
    assert v["exp_balls_bowled"][10] < C.MIN_BOWLING_BALLS["T20"]
    assert not C.is_bowling_option(v["exp_balls_bowled"][10], "T20")
    assert C.is_bowling_option(v["exp_balls_bowled"][0], "T20")
    assert count_bowling_options(v["exp_balls_bowled"], "T20") == 1
    assert aggregate_side(v, "T20")["n_bowlers"] == 1.0
    assert ROLE_BOWLING_OPTION not in roles_of(v, 10, "T20")
    assert ROLE_BOWLING_OPTION in roles_of(v, 0, "T20")


@pytest.mark.parametrize(
    ("fmt", "allocation"),
    [
        ("T20", int(C.INNINGS_LEGAL_BALLS["T20"] * C.BOWLER_MAX_SHARE)),
        ("T20I", int(C.INNINGS_LEGAL_BALLS["T20I"] * C.BOWLER_MAX_SHARE)),
        ("ODI", int(C.INNINGS_LEGAL_BALLS["ODI"] * C.BOWLER_MAX_SHARE)),
        ("TEST", 120),  # no allocation in the laws; twenty overs is a frontline bowler's day
    ],
)
def test_a_frontline_bowler_bowling_his_allocation_in_half_his_appearances_is_a_bowling_option(
    fmt: str, allocation: int
) -> None:
    """Ten appearances, his full allocation in the five older ones and nothing in the
    latest: he reads under half the allocation per appearance, which is where the
    per-match-bowled thresholds (12 / 30 / 60) put him exactly on or under the line once
    FEAT-01 divided by every appearance. A bowler a captain turns to in half his matches
    is a bowling option (FEAT-15)."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    frontline = t2[0]
    for k in range(10):
        bowlers = [frontline] * allocation if k % 2 == 0 else [t2[1]] * allocation
        d = _deliveries([t1[0]] * allocation, bowlers, [1] * allocation, [0] * allocation)
        state.update(_match(f"m{k}", k, "A", t1, t2, d, fmt=fmt))

    v = state.side_vectors(fmt, [frontline])

    assert v["exp_balls_bowled"][0] < allocation / 2, "the latest appearance, undecayed, is one he did not bowl in"
    assert C.is_bowling_option(v["exp_balls_bowled"][0], fmt)
    assert ROLE_BOWLING_OPTION in roles_of(v, 0, fmt)


def test_a_ball_by_someone_not_named_for_the_match_is_nobodys_involvement() -> None:
    """A batter the deliveries name but neither eleven does has no appearance to divide
    by, so his balls land on no involvement numerator rather than on one with no clock."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    d = _deliveries(["ghost"] * 12, [t2[0]] * 12, [1] * 12, [0] * 12)
    state.update(_match("m", 0, "A", t1, t2, d))

    v = state.side_vectors("T20", ["ghost", t2[0]])

    assert v["exp_balls_faced"][0] == 0.0
    assert v["exp_balls_bowled"][1] == pytest.approx(12.0)


def test_ratings_are_per_format() -> None:
    """The accumulators are per format: a T20 match lands on the T20 sums and not the ODI
    ones, and the career counts stay per format (so ``n_debutants`` still counts a format
    debutant, whatever his rate reads -- FEAT-06 changes the rate, not the count). The
    *read* of either format pools the other as its prior mean, which is the point."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    d = _deliveries([t1[0]] * 12, [t2[0]] * 12, [6] * 12, [0] * 12)
    state.update(_match("m", 0, "A", t1, t2, d, fmt="ODI"))
    slot = state.players.key_to_slot[t1[0]]
    odi, t20 = C.FORMAT_INDEX["ODI"], C.FORMAT_INDEX["T20"]
    odi_sums_before = (state.bat_rae[odi, slot], state.bat_balls[odi, slot])
    assert state.side_vectors("T20", [t1[0]])["career"][0] == 0.0
    assert state.side_vectors("T20", [t1[0]])["career_all"][0] == 1.0
    assert aggregate_side(state.side_vectors("T20", t1), "T20")["n_debutants"] == 11.0

    state.update(_match("m2", 1, "B", t1, t2, _deliveries([t1[0]] * 12, [t2[0]] * 12, [0] * 12, [1] * 12), fmt="T20"))

    assert (state.bat_rae[odi, slot], state.bat_balls[odi, slot]) == odi_sums_before
    assert state.bat_rae[t20, slot] < 0 < state.bat_rae[odi, slot]
    assert state.side_vectors("T20", [t1[0]])["bat_rate"][0] < state.side_vectors("ODI", [t1[0]])["bat_rate"][0]
    assert state.side_vectors("T20", [t1[0]])["career"][0] == 1.0


def test_a_format_debutant_reads_his_rate_from_the_formats_he_has_played() -> None:
    """FEAT-06: a player with no history in the format used to read the neutral vector --
    ``PRIOR_BALLS`` of shrinkage toward *zero* -- so a T20I regular on IPL debut was an
    unknown. He now reads his own rate elsewhere: the prior mean is his pooled other-format
    rate, and with no balls in the format the rate is exactly that prior."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    d = _deliveries([t1[0]] * 12, [t2[0]] * 12, [6] * 12, [0] * 12)
    state.update(_match("m", 0, "A", t1, t2, d, fmt="T20I"))
    t20i = state.side_vectors("T20I", [t1[0], t2[0]])

    t20 = state.side_vectors("T20", [t1[0], t2[0]])

    assert t20i["bat_rate"][0] > 0 and t20i["bowl_rate"][1] < 0
    assert t20["bat_rate"][0] == pytest.approx(t20i["bat_rate"][0])
    assert t20["bowl_rate"][1] == pytest.approx(t20i["bowl_rate"][1])
    assert t20["bat_pp_rate"][0] == pytest.approx(t20i["bat_pp_rate"][0])
    assert t20["bat_stuck_share"][0] == pytest.approx(t20i["bat_stuck_share"][0])
    # What is not pooled: involvement, Elo and the role keys keep their neutral values.
    assert t20["exp_balls_faced"][0] == 0.0 and t20["career"][0] == 0.0 and t20["pelo"][0] == C.ELO_INITIAL


def test_a_single_format_player_reads_the_plain_shrunk_rate_bit_for_bit() -> None:
    """The other half of FEAT-06's contract: with no history outside the format the prior
    mean is the old zero and the rate is the old ``sum / (balls + PRIOR_BALLS)``, so every
    single-format player -- most of the archive -- is unchanged by the pooling."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    state.update(_match("m", 0, "A", t1, t2, _deliveries([t1[0]] * 12, [t2[0]] * 12, [6] * 12, [0] * 12)))
    slot = state.players.key_to_slot[t1[0]]
    f = C.FORMAT_INDEX["T20"]

    vectors = state.side_vectors("T20", [t1[0]])

    assert vectors["bat_rate"][0] == state.bat_rae[f, slot] / (state.bat_balls[f, slot] + C.PRIOR_BALLS)
    assert vectors["bowl_spell_overs"][0] == pytest.approx(2.0), "the spell prior's mean is unchanged too"


def test_the_formats_own_balls_take_over_from_the_pooled_prior() -> None:
    """A player with balls in both formats reads a blend: his format's sum plus
    ``PRIOR_BALLS`` of his other-format rate, over his format's balls plus ``PRIOR_BALLS``
    -- the same weight the zero prior had, so the format's own evidence wins at the old pace."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    state.update(_match("m1", 0, "A", t1, t2, _deliveries([t1[0]] * 12, [t2[0]] * 12, [6] * 12, [0] * 12), fmt="ODI"))
    state.update(_match("m2", 1, "A", t1, t2, _deliveries([t1[0]] * 12, [t2[0]] * 12, [0] * 12, [0] * 12), fmt="T20"))
    slot = state.players.key_to_slot[t1[0]]
    odi, t20 = C.FORMAT_INDEX["ODI"], C.FORMAT_INDEX["T20"]
    prior_mean = state.bat_rae[odi, slot] / (state.bat_balls[odi, slot] + C.PRIOR_BALLS)
    expected = (state.bat_rae[t20, slot] + C.PRIOR_BALLS * prior_mean) / (state.bat_balls[t20, slot] + C.PRIOR_BALLS)

    assert state.side_vectors("T20", [t1[0]])["bat_rate"][0] == pytest.approx(expected)
    assert 0 < state.side_vectors("T20", [t1[0]])["bat_rate"][0] < prior_mean


def _bowl_rate_after(deliveries: Deliveries) -> float:
    """The bowler t2[0]'s ``bowl_rate`` after one match of the given deliveries."""
    t1, t2 = _xi("a"), _xi("b")
    state = RatingState()
    state.update(_match("m", 0, "A", t1, t2, deliveries))
    return float(state.side_vectors("T20", [t2[0]])["bowl_rate"][0])


def test_the_bowlers_ledger_charges_him_only_the_runs_he_conceded() -> None:
    """FEAT-08: an over of no-balls that each ran away for four leg-byes is thirty runs
    to the innings and six to the bowler. His runs-saved ledger reads it as six singles
    would, and not as thirty -- the keeper's misses are not his."""
    t1, t2 = _xi("a"), _xi("b")
    no_balls_with_four_leg_byes = _deliveries([t1[0]] * 6, [t2[0]] * 6, [0] * 6, [0] * 6)
    no_balls_with_four_leg_byes.runs_total = np.full(6, 5.0)
    no_balls_with_four_leg_byes.runs_bowler = np.full(6, 1.0)
    six_singles = _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6)
    charged_the_lot = replace(no_balls_with_four_leg_byes, runs_bowler=np.full(6, 5.0))

    assert _bowl_rate_after(no_balls_with_four_leg_byes) == pytest.approx(_bowl_rate_after(six_singles))
    assert _bowl_rate_after(no_balls_with_four_leg_byes) > _bowl_rate_after(charged_the_lot)


def _bat_wrates_after(deliveries: Deliveries, keys: List[str]) -> List[float]:
    """Each key's ``bat_wrate`` after one T20 match of the given deliveries."""
    t1, t2 = _xi("a"), _xi("b")
    state = RatingState()
    state.update(_match("m", 0, "A", t1, t2, deliveries))
    return [float(v) for v in state.side_vectors("T20", keys)["bat_wrate"]]


def test_a_run_out_at_the_non_strikers_end_is_charged_to_the_batter_who_was_out() -> None:
    """FEAT-07 / B-16: a0 faces six balls and a1, backing up, is run out on the third. The
    dismissal is a1's -- his ``dismissals`` target counts it -- so his wicket rate falls and
    a0's reads exactly what it would had nobody been out: the ledger charges the batter
    dismissed, not the batter facing."""
    t1, t2 = _xi("a"), _xi("b")
    partner_run_out = _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0, 0, 1, 0, 0, 0])
    partner_run_out.bowler_wicket[2] = 0.0
    partner_run_out.players_out[2] = [t1[1]]
    nobody_out = _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6)

    striker, partner = _bat_wrates_after(partner_run_out, [t1[0], t1[1]])
    striker_alone, _ = _bat_wrates_after(nobody_out, [t1[0], t1[1]])

    assert striker == pytest.approx(striker_alone)
    assert partner < 0.0


def test_a_batters_own_dismissal_is_still_his() -> None:
    """The striker is charged the dismissals that were his, and only those: out on the
    sixth ball he reads below a batter who survived the same six."""
    t1, t2 = _xi("a"), _xi("b")
    bowled_last_ball = _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 5 + [1])
    survived = _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6)

    assert _bat_wrates_after(bowled_last_ball, [t1[0]])[0] < _bat_wrates_after(survived, [t1[0]])[0]


def test_aggregate_side_role_coverage_and_monotone_direction() -> None:
    fmt = "T20"
    base = {
        "exp_balls_faced": np.full(11, 10.0),
        "exp_balls_bowled": np.array([24.0] * 5 + [0.0] * 6),
        "bat_rate": np.zeros(11),
        "bat_wrate": np.zeros(11),
        "bowl_rate": np.zeros(11),
        "bowl_wrate": np.zeros(11),
        "career": np.full(11, 10.0),
        "career_all": np.full(11, 10.0),
        "pelo": np.full(11, 1500.0),
        "keeper": np.array([1.0] + [0.0] * 10),
    }
    agg = aggregate_side(base, fmt)
    assert agg["n_bowlers"] == 5
    assert agg["has_keeper"] == 1.0
    assert agg["n_debutants"] == 0

    better = {k: v.copy() for k, v in base.items()}
    better["bat_rate"][6] = 0.5
    agg_better = aggregate_side(better, fmt)
    assert agg_better["imp_bat_sum"] > agg["imp_bat_sum"]
    assert agg_better["imp_bat_top6"] >= agg["imp_bat_top6"]

    row = match_features(agg_better, agg)
    assert row["d_imp_bat_sum"] > 0
    assert set(C.XI_FEATURE_COLS) <= set(row)


def test_monotone_directions_follow_the_contract() -> None:
    dirs = dict(zip(C.XI_FEATURE_COLS, C.monotone_directions(C.XI_FEATURE_COLS)))
    assert dirs["d_imp_bat_sum"] == 1
    assert dirs["t1_imp_bowl_sum"] == 1
    assert dirs["t2_imp_bowl_sum"] == -1
    assert dirs["d_n_debutants"] == -1
    assert C.monotone_directions(["t1_pelo_std"]) == [0]
    assert C.monotone_directions(C.TEAM_CONTEXT_COLS) == [0] * len(C.TEAM_CONTEXT_COLS)


def test_team_context_is_constrained_only_when_gate_b7s_switch_is_on() -> None:
    """B-7's arm switch. The shipped default leaves every context column free; the gate's
    arm constrains the four whose direction is knowable and nothing else."""
    constrained = dict(zip(C.TEAM_CONTEXT_COLS, C.monotone_directions(C.TEAM_CONTEXT_COLS, True)))
    assert constrained["home_diff"] == 1

    assert constrained["team_elo_diff"] == 1
    assert constrained["team_form_diff"] == 1
    assert constrained["venue_fam_diff"] == 1
    assert constrained["team_h2h"] == 0
    assert constrained["venue_n"] == 0
    assert C.DISPLAY_CONTEXT_MONOTONE_KEPT is False
    assert C.monotone_directions(C.XI_FEATURE_COLS, True) == C.monotone_directions(C.XI_FEATURE_COLS, False)


def test_no_win_model_reads_the_elo_spread() -> None:
    """B-7 took the spread of player Elo across an eleven out of the display model and
    FEAT-14 out of the objective: it is the one stem a one-player upgrade moves whose
    direction the contract cannot declare, so a model that reads it can lower P(win) on an
    upgrade whatever the other coefficients do. The aggregate itself stays -- the
    performance model reads it as ``own_pelo_std`` -- and neither win list may quietly
    drift back."""
    assert "pelo_std" in C.SIDE_FEATURE_STEMS
    assert not any(column.endswith("_pelo_std") for column in C.XI_FEATURE_COLS)
    assert not any(column.endswith("_pelo_std") for column in C.DISPLAY_FEATURE_COLS)
    assert set(C.XI_FEATURE_COLS) <= set(C.DISPLAY_FEATURE_COLS)


def test_every_xi_column_an_upgrade_moves_carries_a_direction() -> None:
    """FEAT-14's structural claim, stated on the contract: the objective is monotone under
    H-4's upgrade because every column it reads either has a sign to be bounded by, or is
    a stem the five upgraded ratings do not touch (expected balls, all-rounder and
    debutant counts are functions of involvement and career, not of the ratings)."""
    unsigned_stems = {
        column.split("_", 1)[1]
        for column, direction in zip(C.XI_FEATURE_COLS, C.monotone_directions(C.XI_FEATURE_COLS))
        if direction == 0
    }

    assert unsigned_stems == {"exp_balls_bowled_top5", "exp_balls_faced_sum", "n_allrounders"}


def test_build_rejects_out_of_order_sources() -> None:
    t1, t2 = _xi("a"), _xi("b")
    d = _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6)
    later = _match("m2", 5, "A", t1, t2, d)
    earlier = _match("m1", 0, "A", t1, t2, d)
    with pytest.raises(ValueError, match="not in date order"):
        build(_ListSource([later, earlier]))


def test_build_skips_undecided_matches_as_rows_but_folds_them_into_state() -> None:
    t1, t2 = _xi("a"), _xi("b")
    d = _deliveries([t1[0]] * 12, [t2[0]] * 12, [6] * 12, [0] * 12)
    no_result = MatchRecord("nr", date(2024, 1, 1), "T20", "A", "B", "V", "male", t1, t2, None, "no result", d)
    decided = _match("m", 1, "A", t1, t2, d)

    result = build(_ListSource([no_result, decided]))

    assert result.n_undecided == 1
    assert list(result.frame.match_id) == ["m"]
    assert result.frame.iloc[0]["d_imp_bat_sum"] > 0, "the no-result's deliveries still rate the players"
    assert result.frame.iloc[0]["team_elo_diff"] == pytest.approx(0.0), "but no result moves no Elo"


def test_a_draw_or_an_unbroken_tie_is_half_a_win_of_form_and_moves_no_elo() -> None:
    """The definition ``drawn_or_tied_matches`` guards (FEAT-04): a draw and a tie nobody
    broke give each side half a win of form and leave Elo alone; a tie a super over settled
    is a win for the side that won it; a no-result tells form nothing."""
    t1, t2 = _xi("a"), _xi("b")
    d = _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6)
    drawn = replace(_match("drawn", 0, None, t1, t2, d), result="draw")
    tied = replace(_match("tied", 1, None, t1, t2, d), result="tie")
    super_over = replace(_match("super-over", 2, "A", t1, t2, d), result="tie")
    abandoned = replace(_match("abandoned", 3, None, t1, t2, d), result="no result")
    state = RatingState()

    for match in (drawn, tied, abandoned):
        state.update(match)
    elo_after_draws = state.team_elo[("T20", "A")]
    state.update(super_over)

    assert state.team_results[("T20", "A")] == [0.5, 0.5, 1.0]
    assert state.team_results[("T20", "B")] == [0.5, 0.5, 0.0]
    assert elo_after_draws == pytest.approx(C.ELO_INITIAL), "a draw moves no Elo"
    assert state.team_elo[("T20", "A")] > elo_after_draws, "a tie-breaker win is a win"


def test_same_day_matches_do_not_see_each_other() -> None:
    """Two matches on one date are applied at day close: neither row reflects the other."""
    t1, t2 = _xi("a"), _xi("b")
    heavy = _deliveries([t1[0]] * 24, [t2[5]] * 24, [6] * 24, [0] * 24)
    m1 = _match("m1", 0, "A", t1, t2, heavy)
    m2 = _match("m2", 0, "A", t1, t2, heavy)  # same day as m1
    m3 = _match("m3", 1, "A", t1, t2, heavy)  # next day

    frame = build(_ListSource([m1, m2, m3])).frame.set_index("match_id")

    assert frame.loc["m2", "d_imp_bat_sum"] == pytest.approx(frame.loc["m1", "d_imp_bat_sum"])
    assert frame.loc["m2", "team_elo_diff"] == pytest.approx(0.0)
    assert frame.loc["m3", "d_imp_bat_sum"] > 0
    assert frame.loc["m3", "team_elo_diff"] > 0


def test_simulation_context_starts_at_the_laws_of_the_game_and_is_as_of() -> None:
    """Before any match the context is the laws (legal balls, no extras, every dismissal the
    bowler's); after one it is that match's rates, read before the next is folded in."""
    t1, t2 = _xi("a"), _xi("b")
    # Two innings: the first not all out over 12 deliveries with 3 runs of extras; the
    # second all out (10 dismissals, 8 credited to bowlers) and so not a full innings.
    d = _deliveries(["a0"] * 12 + ["b0"] * 12, ["b5"] * 12 + ["a5"] * 12, [1] * 24, [0] * 12 + [1] * 10 + [0] * 2)
    d.innings = np.array([0] * 12 + [1] * 12)
    d.runs_total = d.runs_batter + np.array([1, 1, 1] + [0] * 21, dtype=float)
    # Two of the three extras are the bowler's (a wide and a no-ball); the third is a bye.
    d.runs_bowler = d.runs_batter + np.array([1, 1, 0] + [0] * 21, dtype=float)
    d.bowler_wicket = d.wicket.copy()
    d.bowler_wicket[12:14] = 0.0
    state = RatingState()

    before = state.simulation_context("T20", "male")
    state.update(_match("m1", 0, "A", t1, t2, d))
    after = state.simulation_context("T20", "male")

    assert before == {
        "ctx_extras_per_ball": 0.0,
        "ctx_bowler_extras_per_ball": 0.0,
        "ctx_innings_deliveries": 120.0,
        "ctx_bowler_wicket_share": 1.0,
    }
    assert after["ctx_extras_per_ball"] == pytest.approx(3.0 / 25.0)  # one prior delivery
    assert after["ctx_bowler_extras_per_ball"] == pytest.approx(2.0 / 25.0)  # the bye is the innings' (SERVE-12)
    assert after["ctx_innings_deliveries"] == pytest.approx((120.0 + 12.0) / 2.0)  # one prior innings
    assert after["ctx_bowler_wicket_share"] == pytest.approx((1.0 + 8.0) / (1.0 + 10.0))
    assert state.simulation_context("ODI", "male")["ctx_innings_deliveries"] == 300.0  # per format


def test_fixture_context_reads_the_grounds_and_the_competitions_as_of_level() -> None:
    """Before any history every column is exactly 1.0; after one match at a ground the
    ground reads that match's rate relative to the format's, shrunk; a ground or a
    competition the state has never seen, or an unnamed one, keeps reading exactly 1.0."""
    t1, t2 = _xi("a"), _xi("b")
    # 24 deliveries at six an over-ball, no dismissals: far above any format baseline
    heavy = _deliveries([t1[0]] * 24, [t2[5]] * 24, [6] * 24, [0] * 24)
    played = _match("m1", 0, "A", t1, t2, heavy)
    played.competition = "Cup"
    state = RatingState()

    before = state.fixture_context(played)
    state.update(played)
    same_ground = state.fixture_context(played)
    other_ground = _match("m2", 1, "A", t1, t2, heavy)
    other_ground.venue, other_ground.competition = "W", "League"
    elsewhere = state.fixture_context(other_ground)
    unnamed = _match("m3", 1, "A", t1, t2, heavy)
    unnamed.venue, unnamed.competition = "", ""

    assert all(value == 1.0 for value in before.values())
    assert same_ground["venue_run_rate_rel"] > 1.0 and same_ground["competition_run_rate_rel"] > 1.0
    assert same_ground["venue_wicket_rate_rel"] < 1.0, "no dismissal in 24 balls reads below the format's rate"
    f = C.FORMAT_INDEX["T20"]
    format_rate = state.ctx_runs[0, f].sum() / state.ctx_balls[0, f].sum()
    expected = 1.0 + (24 * 6.0 - 24 * format_rate) / ((24 + C.FIXTURE_CONTEXT_PRIOR_BALLS) * format_rate)
    assert same_ground["venue_run_rate_rel"] == pytest.approx(expected)
    assert same_ground["venue_run_rate_rel"] == pytest.approx(same_ground["competition_run_rate_rel"])
    assert all(value == 1.0 for value in elsewhere.values())
    assert all(value == 1.0 for value in state.fixture_context(unnamed).values())
    state.update(unnamed)
    # neither a read of an unseen key nor an update under an empty one writes into the state
    assert set(state.venue_scoring) == {("T20", "V")} and set(state.competition_scoring) == {("T20", "Cup")}
    assert state.fixture_context(_match("m4", 2, "A", t1, t2, heavy, fmt="ODI"))["venue_run_rate_rel"] == 1.0


def test_reading_an_unknown_player_does_not_register_him() -> None:
    """D-7b: serving is a read. A request naming a player the state has never seen must
    leave the state exactly as the artifact left it, or two identical requests either side
    of a third can disagree."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    state.update(_match("m", 0, "A", t1, t2, _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6)))
    keys_before = dict(state.players.key_to_slot)

    state.side_vectors("T20", ["never-seen-1", "never-seen-2"])

    assert state.players.key_to_slot == keys_before, "a read registered a player it was only asked about"


_TEAM_TABLES = ("team_elo", "team_results", "head_to_head", "venue_bat_first", "team_venue_matches", "team_countries")


def _team_table_sizes(state: RatingState) -> dict:
    return {name: len(getattr(state, name)) for name in _TEAM_TABLES}


def test_reading_team_context_for_an_unknown_key_does_not_write_it() -> None:
    """B-1, the D-7 class at team level: a request naming a team, a venue or a head-to-head
    pair the state has never seen must leave all five keyed tables exactly as the artifact
    left them, while still reading the neutral values the empty entries would have given."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")
    played = _match("m", 0, "A", t1, t2, _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6))
    state.update(played)
    sizes_before = _team_table_sizes(state)

    unknown_team = _match("r1", 1, "A", t1, t2, played.deliveries)
    unknown_team.team2 = "never-seen-team"
    unknown_venue = _match("r2", 1, "A", t1, t2, played.deliveries)
    unknown_venue.venue = ""
    unknown_pair = _match("r3", 1, "A", t1, t2, played.deliveries, fmt="ODI")

    for record in (unknown_team, unknown_venue, unknown_pair):
        state.team_context(record)

    assert _team_table_sizes(state) == sizes_before, "a read wrote a team, venue or head-to-head key"
    # and the values are the neutral ones, unchanged by the fix
    assert state.team_context(unknown_team)["team_elo_diff"] == pytest.approx(
        state.team_elo[("T20", "A")] - C.ELO_INITIAL
    )
    assert state.team_context(unknown_venue)["venue_n"] == 0.0
    assert state.team_context(unknown_venue)["venue_bf_rate"] == 0.5
    assert state.team_context(unknown_pair)["team_h2h"] == 0.5
    assert state.team_context(unknown_pair)["team_h2h_n"] == 0.0
    assert state.team_context(unknown_pair)["team_elo_diff"] == 0.0, "neither side is rated in ODI"


def test_update_still_writes_the_team_tables() -> None:
    """The other half of B-1: writing on write is correct, so ``update`` keeps creating the
    keys a match names. A fix that stopped it would stop the ratings accumulating at all."""
    state = RatingState()
    t1, t2 = _xi("a"), _xi("b")

    state.update(_match("m", 0, "A", t1, t2, _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6)))

    assert _team_table_sizes(state) == {
        "team_elo": 2,
        "team_results": 2,
        "head_to_head": 2,
        "venue_bat_first": 1,
        "team_venue_matches": 2,
        "team_countries": 0,  # the venue is in no region, so nobody played anywhere
    }
    assert state.team_elo[("T20", "A")] > state.team_elo[("T20", "B")]


# ---------------------------------------------------------------------------
# Home advantage (FEAT-05)
# ---------------------------------------------------------------------------

_REGIONS = {"v": "IN", "w": "AU"}


def _away_match(mid: str, day: int, team1: str, team2: str, venue: str, winner: str = "A") -> MatchRecord:
    t1, t2 = _xi("a"), _xi("b")
    record = _match(mid, day, winner, t1, t2, _deliveries([t1[0]] * 6, [t2[0]] * 6, [1] * 6, [0] * 6))
    record.team1, record.team2, record.venue = team1, team2, venue
    return record


def test_home_diff_reads_the_teams_past_and_never_the_match_itself() -> None:
    """FEAT-05: a side is at home when the ground's region is the one it has played in
    most *before today*. Nothing is looked up about the team in a whole-archive table, so
    the first match of a team's life reads 0 even though, seen whole, it was played at home
    -- and a later match played abroad does not change what an earlier row read."""
    m1 = _away_match("m1", 0, "A", "B", "V")  # both A and B play their first match in India
    m2 = _away_match("m2", 1, "A", "C", "V")  # A has a past in India; C has none
    m3 = _away_match("m3", 2, "A", "C", "W")  # in Australia: A's past says India, C's says India
    m4 = _away_match("m4", 3, "C", "A", "W")  # C's past is now split IN 1 / AU 1: no home

    def frame(*matches):
        source = _ListSource(list(matches))
        source.venue_countries = lambda: dict(_REGIONS)
        return build(source).frame.set_index("match_id")["home_diff"]

    short, long = frame(m1, m2), frame(m1, m2, m3, m4)

    assert short["m1"] == 0.0 and long["m1"] == 0.0, "no past, so nobody is at home yet"
    assert short["m2"] == 1.0 and long["m2"] == 1.0, "A is at home, C has no past; the later matches change nothing"
    assert long["m3"] == 0.0, "Australia is home to neither; both sides' past says India"
    assert long["m4"] == 0.0, "C's most-played regions tie, so C is nowhere; A is still Indian"


def test_home_sides_read_a_region_from_the_venue_table_and_a_snapshot_keeps_the_teams_past() -> None:
    """The two inputs: the ground's region comes from the static venue table (a ground it
    does not place is neutral for everyone), the team's from ``team_countries``, which the
    pass writes at update and a snapshot copies value by value (SERVE-01)."""
    state = RatingState(venue_countries=_REGIONS)
    state.update(_away_match("m1", 0, "A", "B", "V"))
    frozen = state.snapshot()

    state.update(_away_match("m2", 1, "A", "B", "V"))
    unplaced = _away_match("m3", 2, "A", "B", "unplaced ground")

    assert state.home_sides(_away_match("m3", 2, "A", "B", "V")) == (1.0, 1.0)
    assert state.home_sides(unplaced) == (0.0, 0.0)
    assert state.team_context(unplaced)["home_diff"] == 0.0
    assert state.team_countries["A"] == {"IN": 2} and frozen.team_countries["A"] == {"IN": 1}


def test_reading_home_sides_for_an_unknown_team_does_not_write_it() -> None:
    """B-1 for the new table: a request naming a team the state has never seen reads it
    as nowhere and leaves the table as the artifact left it."""
    state = RatingState(venue_countries=_REGIONS)
    state.update(_away_match("m1", 0, "A", "B", "V"))

    home = state.home_sides(_away_match("m2", 1, "A", "never-seen-team", "V"))

    assert home == (1.0, 0.0)
    assert set(state.team_countries) == {"A", "B"}


def test_home_and_toss_are_display_columns_the_objective_never_reads() -> None:
    """The constraint FEAT-05 lands under: both facts are constant with respect to the
    eleven, so they reach the displayed probability only. ``XI_FEATURE_COLS`` -- what
    ``objective_probability`` reads and ``objective_auc`` scores -- is untouched, so the
    optimiser stays toss-blind (B-21) and H-17's line keeps its meaning."""
    assert "home_diff" in C.TEAM_CONTEXT_COLS and "home_diff" in C.DISPLAY_FEATURE_COLS
    assert C.TOSS_COL in C.DISPLAY_FEATURE_COLS
    assert "home_diff" not in C.XI_FEATURE_COLS and C.TOSS_COL not in C.XI_FEATURE_COLS
    assert C.monotone_directions(["home_diff"], True) == [1]
