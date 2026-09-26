"""Unit tests for the biography input (X-1b): the readers, the age columns on the rows,
the artifact round trip, and the age-band debut prior in the rating state."""

from __future__ import annotations

from datetime import date

import numpy as np
import pandas as pd
import pytest

from ml.xi import biography
from ml.xi import contract as C
from ml.xi import performance as P
from ml.xi.builder import build
from ml.xi.ratings import DEBUT_MATCHES, DEBUT_PRIOR_KEYS, RatingState, debut_prior_vectors
from ml.xi.rows import player_feature_rows
from ml.xi.sources import CricsheetJsonSource
from ml.xi.store import load_ratings, save_ratings
from tests.xi_fixtures import ListSource, make_deliveries, make_match, xi

BIRTH = {"a0": date(2000, 6, 15), "b0": date(1990, 1, 1)}


class _Source(ListSource):
    """An in-memory source that knows two players' dates of birth."""

    def birth_dates(self):
        return dict(BIRTH)

    def venue_countries(self):
        return {}


def _matches(days, winner="A"):
    t1, t2 = xi("a"), xi("b")
    d = make_deliveries(["a0"] * 6 + ["b0"] * 6, ["b5"] * 6 + ["a5"] * 6, [4] * 12, [0] * 12)
    d.innings = np.array([0] * 6 + [1] * 6)
    return [make_match(f"m{day}", day, winner, t1, t2, d) for day in days]


def test_age_years_counts_elapsed_days_over_the_mean_year() -> None:
    assert biography.age_years(date(2000, 6, 15), date(2024, 6, 15)) == pytest.approx(24.0, abs=0.01)


def test_age_years_reads_a_1_january_birth_date_as_mid_year() -> None:
    """DATA-07: Wikidata cannot distinguish a genuine 1 January birth from a year-precision
    date rendered as 1 January, so age_years reads every 1 January as mid-year -- half the
    naive value's overstatement, not the up-to-a-year-high the raw date would give."""
    naive = (date(2024, 1, 1) - date(2000, 1, 1)).days / biography.DAYS_PER_YEAR

    age = biography.age_years(date(2000, 1, 1), date(2024, 1, 1))

    assert age == pytest.approx(naive - 0.5, abs=0.01)
    assert age != pytest.approx(naive, abs=0.01)


def test_age_vectors_leave_a_player_without_a_birth_date_as_his_own_category() -> None:
    """Missing is 0.0 / 0.0 -- an indicator of its own, never an imputed age."""
    out = biography.age_vectors(BIRTH, ["a0", "nobody", "b0"], date(2024, 6, 15))

    # b0's birth date (1990-01-01) is read as mid-year (DATA-07): ~33.95, not the naive 34.45.
    np.testing.assert_allclose(out["age"], [24.0, 0.0, 33.95], atol=0.01)
    np.testing.assert_array_equal(out["age_known"], [1.0, 0.0, 1.0])


def test_age_band_cuts_at_the_contract_bounds() -> None:
    np.testing.assert_array_equal(C.age_band([19.0, 22.0, 25.9, 30.0, 40.0]), [0, 1, 1, 3, 4])
    assert C.N_AGE_BANDS == 5


def test_csv_round_trip_preserves_every_birth_date(tmp_path) -> None:
    path = str(tmp_path / "birth-dates.csv")

    written = biography.write_birth_dates_csv(path, BIRTH)

    assert written == 2
    assert biography.load_birth_dates_csv(path) == BIRTH


def test_csv_reader_refuses_a_file_without_the_columns(tmp_path) -> None:
    path = tmp_path / "wrong.csv"
    path.write_text("id,dob\na0,2000-06-15\n")

    with pytest.raises(ValueError, match="lacks column"):
        biography.load_birth_dates_csv(str(path))


def test_no_birth_dates_file_means_every_age_unknown(tmp_path) -> None:
    source = CricsheetJsonSource(str(tmp_path), birth_dates_path=None)

    assert source.birth_dates() == {}


def test_count_known_counts_only_the_players_with_a_date() -> None:
    assert biography.count_known(BIRTH, ["a0", "a1", "b0"]) == 2


def test_player_rows_carry_the_age_at_the_match_date_and_the_indicator() -> None:
    matches = _matches([0, 1])
    result = build(_Source(matches))

    rows = result.player_frame
    a0 = rows[(rows.match_id == "m1") & (rows.player_key == "a0")].iloc[0]
    a1 = rows[(rows.match_id == "m1") & (rows.player_key == "a1")].iloc[0]
    assert a0.age == pytest.approx(biography.age_years(BIRTH["a0"], matches[1].match_date))
    assert a0.age_known == 1.0
    assert (a1.age, a1.age_known) == (0.0, 0.0)
    assert set(C.AGE_COLS) <= set(C.PLAYER_MATCH_FEATURE_COLS)
    assert result.quality.players_with_birth_date == 2


def test_performance_feature_cols_read_the_age_columns_only_when_kept() -> None:
    without = C.performance_feature_cols(age=False)
    with_age = C.performance_feature_cols(age=True)

    assert not set(without) & set(C.AGE_COLS)
    assert set(with_age) - set(without) == set(C.AGE_COLS)
    assert P.default_spec(age=True).as_dict()["age"] is True


def test_artifact_round_trip_keeps_birth_dates_and_the_cold_start_flag(tmp_path) -> None:
    state = build(_Source(_matches([0, 1])), age_aware_cold_start=True).state

    save_ratings(state, str(tmp_path))
    loaded = load_ratings(str(tmp_path))

    assert loaded.birth_dates == BIRTH
    assert loaded.age_aware_cold_start is True
    np.testing.assert_array_equal(loaded.debut_bat, state.debut_bat)


def test_debut_prior_reads_neutral_for_a_band_nobody_has_debuted_in() -> None:
    empty = np.zeros((C.N_AGE_BANDS, 4))

    out = debut_prior_vectors(empty, empty, np.array([0, 4]))

    for key in DEBUT_PRIOR_KEYS:
        np.testing.assert_array_equal(out[key], [0.0, 0.0])


def test_debut_prior_applies_the_side_vectors_formulas_to_the_bands_pooled_sums() -> None:
    bat = np.zeros((C.N_AGE_BANDS, 4))
    bat[2] = [-30.0, 240.0, 4.0, 12.0]  # impact, balls, wickets, matches

    out = debut_prior_vectors(bat, np.zeros((C.N_AGE_BANDS, 4)), np.array([2]))

    assert out["exp_balls_faced"][0] == pytest.approx(20.0)
    assert out["bat_rate"][0] == pytest.approx(-30.0 / (240.0 + C.PRIOR_BALLS))
    assert out["bat_wrate"][0] == pytest.approx(4.0 / (240.0 + C.PRIOR_BALLS))


def test_cold_start_prior_moves_only_debutants_of_known_age() -> None:
    """After one match a0 (known age) has history and b-side b0 too; a fresh key with a
    known age reads the band's profile, a fresh key without one stays neutral, and every
    player with history is untouched."""
    matches = _matches([0, 1, 2])
    off = build(_Source(matches), age_aware_cold_start=False).state
    on = build(_Source(matches), age_aware_cold_start=True).state
    on.birth_dates["fresh"] = date(2000, 1, 1)  # ~23.5 at the read date (mid-year, DATA-07): a0's band
    on_date = date(2024, 1, 10)

    keys = ["a0", "b0", "fresh", "unknown"]
    control = off.side_vectors("T20", keys, on=on_date)
    prior = on.side_vectors("T20", keys, on=on_date)

    for key in DEBUT_PRIOR_KEYS:
        np.testing.assert_array_equal(prior[key][:2], control[key][:2])  # history: untouched
        assert prior[key][3] == control[key][3] == 0.0  # no date of birth: neutral
    assert prior["career"][2] == 0.0
    assert control["exp_balls_faced"][2] == 0.0
    assert prior["exp_balls_faced"][2] == pytest.approx(6.0)  # a0's six debut balls, the band's only debut
    assert prior["bat_rate"][2] != 0.0
    assert on.debut_bat[C.FORMAT_INDEX["T20"]][1, 3] == 1.0


class _TwoDebutantsOfOneBand(_Source):
    """a0 (23) and a1 (~23.5, mid-year DATA-07 reading of a 1 January date) debut on the
    same day in the same band; only a0 bats."""

    def birth_dates(self):
        return {**BIRTH, "a1": date(2000, 1, 1)}

    def venue_countries(self):
        return {}


def test_debut_prior_divides_the_bands_balls_by_every_debut_appearance() -> None:
    """The band's expected balls faced is per debutant, not per debutant who got a ball
    (FEAT-01): a0's six balls over two debut appearances read 3, and the appearance a1
    made without batting counts in both tables."""
    state = build(_TwoDebutantsOfOneBand(_matches([0])), age_aware_cold_start=True).state
    state.birth_dates["fresh"] = date(2000, 1, 1)
    f = C.FORMAT_INDEX["T20"]

    prior = state.side_vectors("T20", ["fresh"], on=date(2024, 1, 10))

    assert state.debut_bat[f][1, DEBUT_MATCHES] == 2.0
    assert state.debut_bowl[f][1, DEBUT_MATCHES] == 2.0
    assert prior["exp_balls_faced"][0] == pytest.approx(3.0)
    assert prior["exp_balls_bowled"][0] == 0.0


def test_cold_start_prior_is_off_by_default_and_read_at_the_states_date() -> None:
    state = build(_Source(_matches([0, 1])), age_aware_cold_start=True).state
    state.birth_dates["fresh"] = date(2000, 1, 1)

    explicit = state.side_vectors("T20", ["fresh"], on=state.last_date)
    implicit = state.side_vectors("T20", ["fresh"])

    for key in DEBUT_PRIOR_KEYS:
        assert explicit[key][0] == implicit[key][0]
    assert C.AGE_AWARE_COLD_START is False
    assert C.AGE_FEATURES_KEPT is False


def test_serving_rows_carry_the_same_age_columns_as_the_training_frame() -> None:
    matches = _matches([0, 1])
    result = build(_Source(matches))
    state = RatingState(birth_dates=BIRTH)
    state.update(matches[0])

    _, rows = player_feature_rows(state, matches[1])

    frame = result.player_frame[result.player_frame.match_id == "m1"]
    served = pd.DataFrame(rows).set_index(["side", "player_key"])
    expected = frame.set_index(["side", "player_key"])
    for col in C.AGE_COLS:
        np.testing.assert_allclose(served[col].loc[expected.index], expected[col])
