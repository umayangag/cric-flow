"""Optimiser, store round-trip, and the Cricsheet JSON source (ml.xi)."""

from __future__ import annotations

import json
import os
from datetime import date, timedelta
from typing import List

import numpy as np
import pytest

from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.optimizer import Constraints, marginal_values, select_xi, select_xi_by_ratings
from ml.xi.retrain import retrain
from ml.xi.sources import CricsheetJsonSource, Deliveries, MatchRecord, detect_format, parse_cricsheet_file
from ml.xi.store import XiStore, load_ratings, save_ratings


def _deliveries(
    batters: List[str], bowlers: List[str], runs: List[int], wickets: List[int], stumped_by=None
) -> Deliveries:
    n = len(batters)
    stump = np.zeros(n)
    fielders = [[] for _ in range(n)]
    if stumped_by is not None:
        stump[-1] = 1.0
        wickets = list(wickets)
        wickets[-1] = 1
        fielders[-1] = [stumped_by]
    return Deliveries(
        over=np.arange(n) // 6,
        innings=np.zeros(n, dtype=int),
        batter=np.asarray(batters, dtype=object),
        bowler=np.asarray(bowlers, dtype=object),
        runs_batter=np.asarray(runs, dtype=float),
        runs_total=np.asarray(runs, dtype=float),
        wicket=np.asarray(wickets, dtype=float),
        bowler_wicket=np.asarray(wickets, dtype=float),
        stumping=stump,
        fielders=fielders,
    )


def skill_rank(key: str) -> int:
    return int(key[1:])


class _ListSource:
    def __init__(self, matches):
        self.matches = matches

    def iter_matches(self):
        yield from self.matches


def _synthetic_history(n_matches: int = 160, seed: int = 0):
    """Two squads of 14 with graded skill; stronger XIs win more often. Both sides bat and
    bowl (two innings), so both have bowling options. Produces enough rows to fit both
    models for T20."""
    rng = np.random.RandomState(seed)
    squad_a = [f"a{i}" for i in range(14)]
    squad_b = [f"b{i}" for i in range(14)]
    skill = {p: (14 - i) / 14 for i, p in enumerate(squad_a)}
    skill.update({p: (14 - i) / 14 for i, p in enumerate(squad_b)})

    def innings(batting_xi, bowling_xi):
        batters = sorted(batting_xi, key=lambda p: -skill[p])[:6]
        bowlers = sorted(bowling_xi, key=lambda p: skill[p])[:5]
        bat_seq = [p for p in batters for _ in range(10)]  # 60 balls: 12 per bowler, a T20 bowling option
        bowl_seq = [bowlers[i % 5] for i in range(len(bat_seq))]
        runs = [int(rng.rand() < skill[b] * 0.6) * int(rng.choice([1, 2, 4, 6])) for b in bat_seq]
        wk = [int(rng.rand() < 0.04 * (1.5 - skill[b])) for b in bat_seq]
        return bat_seq, bowl_seq, runs, wk

    matches = []
    for k in range(n_matches):
        xi_a = list(rng.choice(squad_a, 11, replace=False))
        xi_b = list(rng.choice(squad_b, 11, replace=False))
        strength_a = sum(skill[p] for p in xi_a)
        strength_b = sum(skill[p] for p in xi_b)
        p_a = 1 / (1 + np.exp(-(strength_a - strength_b) * 1.5))
        winner = "A" if rng.rand() < p_a else "B"
        b1, w1, r1, k1 = innings(xi_a, xi_b)
        b2, w2, r2, k2 = innings(xi_b, xi_a)
        d = _deliveries(b1 + b2, w1 + w2, r1 + r2, k1 + k2, stumped_by=xi_b[0])
        d.innings = np.array([0] * len(b1) + [1] * len(b2))
        matches.append(
            MatchRecord(
                f"m{k}", date(2023, 1, 1) + timedelta(days=k), "T20", "A", "B", "V", "male", xi_a, xi_b, winner, None, d
            )
        )
    return matches, squad_a, squad_b


@pytest.fixture(scope="module")
def trained_store(tmp_path_factory) -> tuple:
    matches, squad_a, squad_b = _synthetic_history()
    result = build(_ListSource(matches))
    out = tmp_path_factory.mktemp("xi_artifacts")
    import pandas as pd

    written = retrain(result, str(out), pd.Timestamp("2023-05-01"), formats=["T20"])
    assert written["summary"]["formats"][0]["n_train"] > 50
    return XiStore.load(written["run_dir"]), squad_a, squad_b, matches


def test_store_round_trip_preserves_state(tmp_path) -> None:
    matches, _, _ = _synthetic_history(n_matches=20)
    state = build(_ListSource(matches)).state
    save_ratings(state, str(tmp_path))
    loaded = load_ratings(str(tmp_path))
    keys = ["a0", "a3", "b9", "never-seen"]
    for k, v in state.side_vectors("T20", keys).items():
        np.testing.assert_allclose(loaded.side_vectors("T20", keys)[k], v)
    assert loaded.team_elo[("T20", "A")] == pytest.approx(state.team_elo[("T20", "A")])
    assert loaded.last_date == state.last_date
    assert loaded.simulation_context("T20", "male") == state.simulation_context("T20", "male")
    assert dict(loaded.venue_scoring) == dict(state.venue_scoring) and len(state.venue_scoring) > 0
    assert dict(loaded.competition_scoring) == dict(state.competition_scoring)


def test_store_round_trip_preserves_the_gender_split_flag(tmp_path) -> None:
    """The E7 flag is part of the feature definition, so an artifact must carry it."""
    matches, _, _ = _synthetic_history(n_matches=5)
    state = build(_ListSource(matches), gender_split_context=True).state
    save_ratings(state, str(tmp_path))

    assert load_ratings(str(tmp_path)).gender_split_context is True


def test_select_xi_respects_constraints_and_beats_seed(trained_store) -> None:
    store, squad_a, squad_b, matches = trained_store
    opponent = matches[-1].team2_players
    res = select_xi(store, "T20", squad_a, opponent, Constraints(team_size=11, min_bowlers=3, require_keeper=False))
    assert len(res.selected) == 11
    assert len(set(res.selected)) == 11
    assert set(res.selected) <= set(squad_a)
    assert 0.0 <= res.win_probability <= 1.0
    assert res.improved_over_seed >= 0.0
    v = store.side_vectors("T20", res.selected)
    assert C.is_bowling_option(v["exp_balls_bowled"], "T20").sum() >= 3


def test_select_xi_must_include_and_exclude(trained_store) -> None:
    store, squad_a, squad_b, matches = trained_store
    opponent = matches[-1].team2_players
    c = Constraints(team_size=11, min_bowlers=0, require_keeper=False, must_include=["a13"], must_exclude=["a0"])
    res = select_xi(store, "T20", squad_a, opponent, c)
    assert "a13" in res.selected
    assert "a0" not in res.selected


def test_select_xi_raises_when_constraints_cannot_be_met(trained_store) -> None:
    store, squad_a, _, matches = trained_store
    with pytest.raises(ValueError, match="constraints"):
        select_xi(store, "T20", squad_a[:8], matches[-1].team2_players, Constraints(team_size=11))


def test_optimised_xi_scores_at_least_the_fielded_xi(trained_store) -> None:
    store, squad_a, _, matches = trained_store
    last = matches[-1]
    fielded = store.objective_probability(
        "T20", store.side_vectors("T20", last.team1_players), store.side_vectors("T20", last.team2_players)
    )
    res = select_xi(
        store, "T20", squad_a, last.team2_players, Constraints(team_size=11, min_bowlers=0, require_keeper=False)
    )
    assert res.win_probability >= fielded - 1e-9


def test_reading_an_unknown_player_does_not_add_him_to_the_state(trained_store) -> None:
    """A serving read must not invent a player.

    ``side_vectors`` used to assign a slot to any key it had not seen, so a request naming
    someone unknown -- a debutant, or a caller sending the wrong kind of id -- grew the
    loaded state permanently, and ``known_players`` reported him *known* on the second
    identical request. The state must be the same before and after a read.
    """
    store, squad_a, _, _ = trained_store
    before = len(store.state.players)

    assert store.known_players(["nobody-has-this-key"]) == [False]
    store.side_vectors("T20", squad_a[:3] + ["nobody-has-this-key"])

    assert len(store.state.players) == before
    assert store.known_players(["nobody-has-this-key"]) == [False]


def test_an_unknown_player_reads_as_a_debutant_not_as_a_neighbour(trained_store) -> None:
    """The reserved column no update writes, so two different unknown keys read alike and
    neither picks up the values of whoever happens to sit at the next slot."""
    store, _, _, _ = trained_store

    first = store.side_vectors("T20", ["unknown-one"])
    second = store.side_vectors("T20", ["unknown-two"])

    for key, values in first.items():
        assert values[0] == pytest.approx(second[key][0]), key
    assert first["pelo"][0] == pytest.approx(C.ELO_INITIAL)
    assert first["bat_rate"][0] == pytest.approx(0.0)


def test_select_xi_by_ratings_meets_the_constraints_without_an_opponent(trained_store) -> None:
    store, squad_a, _, _ = trained_store
    selected = select_xi_by_ratings(
        store, "T20", squad_a, Constraints(team_size=11, min_bowlers=3, require_keeper=False)
    )
    assert len(set(selected)) == 11
    assert set(selected) <= set(squad_a)
    v = store.side_vectors("T20", selected)
    assert C.is_bowling_option(v["exp_balls_bowled"], "T20").sum() >= 3


def test_select_xi_by_ratings_is_the_search_seed_and_evaluates_no_model(trained_store) -> None:
    store, squad_a, _, matches = trained_store
    c = Constraints(team_size=11, min_bowlers=3, require_keeper=False)
    rating_ordered = select_xi_by_ratings(store, "T20", squad_a, c)
    searched = select_xi(store, "T20", squad_a, matches[-1].team2_players, c)
    assert searched.improved_over_seed >= 0.0
    # The rating-ordered pick is where the search starts, so it can only be the searched XI
    # when the search found nothing better -- and it is reached without an opponent XI.
    assert (set(rating_ordered) == set(searched.selected)) == (searched.improved_over_seed == 0.0)


def test_select_xi_by_ratings_raises_when_constraints_cannot_be_met(trained_store) -> None:
    store, squad_a, _, _ = trained_store
    with pytest.raises(ValueError, match="constraints"):
        select_xi_by_ratings(store, "T20", squad_a[:8], Constraints(team_size=11))


def test_marginal_values_cover_every_player_and_rank_the_strongest_highest(trained_store) -> None:
    store, squad_a, _, matches = trained_store
    last = matches[-1]
    mv = marginal_values(store, "T20", last.team1_players, last.team2_players)
    assert set(mv) == set(last.team1_players)
    best = max(mv, key=mv.get)
    top_batters = sorted(last.team1_players, key=skill_rank)[:6]
    assert best in top_batters, "the most valuable player is one of the six who bat, and they are the most skilled"


def test_display_probability_accepts_missing_context(trained_store) -> None:
    store, _, _, matches = trained_store
    last = matches[-1]
    p_plain = store.display_probability("T20", last.team1_players, last.team2_players)
    p_ctx = store.display_probability(
        "T20", last.team1_players, last.team2_players, team1_name="A", team2_name="B", venue="V"
    )
    assert 0.0 <= p_plain <= 1.0 and 0.0 <= p_ctx <= 1.0


# ---------------------------------------------------------------------------
# Cricsheet JSON source
# ---------------------------------------------------------------------------

_INTL = ["India", "Australia"]


def _cricsheet_doc(match_type: str, teams, players, winner, day: int) -> dict:
    return {
        "info": {
            "dates": [(date(2024, 3, 1) + timedelta(days=day)).isoformat()],
            "match_type": match_type,
            "teams": teams,
            "gender": "male",
            "venue": "Ground",
            "event": {"name": "Cup"},
            "registry": {"people": {n: f"id_{n}" for t in players.values() for n in t}},
            "players": players,
            "outcome": {"winner": winner} if winner else {"result": "no result"},
        },
        "innings": [
            {
                "team": teams[0],
                "overs": [
                    {
                        "over": 0,
                        "deliveries": [
                            {
                                "batter": players[teams[0]][0],
                                "bowler": players[teams[1]][10],
                                "non_striker": players[teams[0]][1],
                                "runs": {"batter": 4, "extras": 0, "total": 4},
                            },
                            {
                                "batter": players[teams[0]][0],
                                "bowler": players[teams[1]][10],
                                "non_striker": players[teams[0]][1],
                                "runs": {"batter": 0, "extras": 0, "total": 0},
                                "wickets": [
                                    {
                                        "kind": "stumped",
                                        "player_out": players[teams[0]][0],
                                        "fielders": [{"name": players[teams[1]][0]}],
                                    }
                                ],
                            },
                            {
                                "batter": players[teams[0]][1],
                                "bowler": players[teams[1]][10],
                                "non_striker": players[teams[0]][2],
                                "runs": {"batter": 0, "extras": 1, "total": 1},
                                "extras": {"wides": 1},
                                "wickets": [
                                    {
                                        "kind": "run out",
                                        "player_out": players[teams[0]][1],
                                        "fielders": [{"name": players[teams[1]][3]}],
                                    }
                                ],
                            },
                        ],
                    }
                ],
            }
        ],
    }


@pytest.mark.parametrize(
    "match_type,teams,expected",
    [
        ("Test", ["India", "Australia"], "TEST"),
        ("MDM", ["X", "Y"], "TEST"),
        ("ODI", ["X", "Y"], "ODI"),
        ("ODM", ["X", "Y"], "ODI"),
        ("IT20", ["X", "Y"], "T20I"),
        ("T20", ["India", "Australia"], "T20I"),
        ("T20", ["India", "Club"], "T20"),
        ("Hundred", ["X", "Y"], ""),
    ],
)
def test_detect_format_mirrors_go_app(match_type, teams, expected) -> None:
    assert detect_format(match_type, teams, _INTL) == expected


def test_parse_cricsheet_file_reads_squads_and_deliveries(tmp_path) -> None:
    players = {"India": [f"I{i}" for i in range(11)], "Australia": [f"A{i}" for i in range(11)]}
    path = tmp_path / "1.json"
    path.write_text(json.dumps(_cricsheet_doc("T20", ["India", "Australia"], players, "Australia", 0)))

    rec = parse_cricsheet_file(str(path), _INTL)

    assert rec is not None
    assert rec.format_code == "T20I"
    # A team key carries the gender: 130 names in the dataset belong to both a men's and a
    # women's side, and one key for both gave them one Elo.
    assert rec.team1 == "India|male" and rec.team2 == "Australia|male"
    assert rec.outcome == 0.0, "the winner is keyed the same way, or it matches neither side"
    assert rec.competition == "Cup", "the event name is the competition key (A-1)"
    assert rec.team1_players[0] == "id_I0" and len(rec.team2_players) == 11
    d = rec.deliveries
    assert len(d) == 3
    assert list(d.wicket) == [0.0, 1.0, 1.0]
    assert list(d.bowler_wicket) == [0.0, 1.0, 0.0], "a run out is not the bowler's wicket"
    assert list(d.stumping) == [0.0, 1.0, 0.0]
    assert d.fielders[1] == ["id_A0"]
    assert d.runs_total[2] == 1.0 and d.runs_batter[2] == 0.0


def test_cricsheet_source_orders_by_date_and_skips_unusable_files(tmp_path) -> None:
    players = {"X": [f"X{i}" for i in range(11)], "Y": [f"Y{i}" for i in range(11)]}
    (tmp_path / "b.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], players, "X", 5)))
    (tmp_path / "a.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], players, None, 1)))
    (tmp_path / "c.json").write_text(json.dumps(_cricsheet_doc("Hundred", ["X", "Y"], players, "X", 2)))
    (tmp_path / "notes.txt").write_text("ignored")

    recs = list(CricsheetJsonSource(str(tmp_path), _INTL).iter_matches())

    assert [r.match_id for r in recs] == ["a", "b"]
    assert recs[0].outcome is None and recs[0].result == "no result"
    result = build(CricsheetJsonSource(str(tmp_path), _INTL))
    assert result.n_undecided == 1 and len(result.frame) == 1
    assert result.state.keeper[result.state.players.slot("id_Y0")] == 1.0, "the stumping marks the keeper"
    assert os.path.exists(tmp_path)


def test_predictions_are_symmetric_in_batting_order(trained_store) -> None:
    """Before the toss P(A beats B) must equal 1 - P(B beats A): both models marginalise over
    who bats first, so the optimiser's objective does not depend on an unknown."""
    store, _, _, matches = trained_store
    last = matches[-1]
    a, b = last.team1_players, last.team2_players
    p_ab = store.objective_probability("T20", store.side_vectors("T20", a), store.side_vectors("T20", b))
    p_ba = store.objective_probability("T20", store.side_vectors("T20", b), store.side_vectors("T20", a))
    assert p_ab == pytest.approx(1.0 - p_ba)
    d_ab = store.display_probability("T20", a, b, team1_name="A", team2_name="B", venue="V")
    d_ba = store.display_probability("T20", b, a, team1_name="B", team2_name="A", venue="V")
    assert d_ab == pytest.approx(1.0 - d_ba)
    known = store.display_probability("T20", a, b, team1_name="A", team2_name="B", venue="V", team1_bats_first=True)
    chase = store.display_probability("T20", a, b, team1_name="A", team2_name="B", venue="V", team1_bats_first=False)
    assert d_ab == pytest.approx(0.5 * (known + chase))
    res_t1 = select_xi(
        store, "T20", a + b[:3], b, Constraints(team_size=11, min_bowlers=0, require_keeper=False), team_is_team1=True
    )
    res_t2 = select_xi(
        store, "T20", a + b[:3], b, Constraints(team_size=11, min_bowlers=0, require_keeper=False), team_is_team1=False
    )
    assert res_t1.win_probability == pytest.approx(res_t2.win_probability)
    assert sorted(res_t1.selected) == sorted(res_t2.selected)
