"""app.xi_service (registry, optimize, predict) and the Postgres match source, without a
database or a running FastAPI app."""

from __future__ import annotations

import json
from datetime import date, timedelta
from typing import List

import numpy as np
import pandas as pd
import pytest
from pydantic import ValidationError

from app import xi_service
from app.models.xi import PerformancePredictRequest, SimulateRequest, XiConstraints, XiOptimizeRequest, XiWinRequest
from ml.xi.builder import build
from ml.xi.retrain import main as retrain_main
from ml.xi.retrain import retrain
from ml.xi.simulator import SimulationUnavailable
from ml.xi.sources import BOWLER_CREDITED_KINDS, Deliveries, MatchRecord, PostgresSource, _deliveries_from_rows
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history
from tests.xi_perf_fixtures import fast_fits


def _registry_keyed_history(n: int = 160):
    """The synthetic history with registry-style player keys, as both sources yield.

    The keys are hex-looking strings on purpose. They were renamed to integers here while the
    ``/xi/*`` contract declared ``List[int]`` -- a test-only workaround that made the suite
    agree with a wire format no real player id could satisfy, which is why P-5 shipped the
    mismatch: every test passed against keys the store would never hold.
    """
    matches, squad_a, squad_b = _synthetic_history(n)
    rename = {k: f"{1000 + i:08x}" for i, k in enumerate(squad_a)}
    rename.update({k: f"{2000 + i:08x}" for i, k in enumerate(squad_b)})

    def remap(keys: List[str]) -> List[str]:
        return [rename[k] for k in keys]

    out = []
    for m in matches:
        d = m.deliveries
        d2 = Deliveries(
            d.over, d.innings, np.asarray(remap(list(d.batter)), dtype=object), np.asarray(remap(list(d.bowler)), dtype=object),
            d.runs_batter, d.runs_total, d.wicket, d.bowler_wicket, d.stumping, [remap(f) for f in d.fielders],
        )  # fmt: skip
        out.append(
            MatchRecord(
                m.match_id,
                m.match_date,
                # T20I: the same laws as the synthetic T20 history, and a format that is
                # offered an optimised selection (T20 is scoped off by E5, plan §8.8).
                "T20I",
                "7",
                "8",
                "3",
                m.gender,
                remap(m.team1_players),
                remap(m.team2_players),
                {"A": "7", "B": "8"}.get(m.winner),
                m.result,
                d2,
            )
        )
    return out, [rename[k] for k in squad_a], [rename[k] for k in squad_b]


@pytest.fixture(autouse=True)
def _staleness_off(monkeypatch) -> None:
    """This module's fixtures are a synthetic 2023 history, so every state in it is stale.
    H-11 has its own tests (``test_runs_and_reload``); turning it off here keeps these
    tests measuring what they name."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")


@pytest.fixture(scope="module")
def artifacts_dir(tmp_path_factory) -> tuple:
    """One run, written the way ``retrain`` writes one: artifacts and manifest under
    ``runs/<id>/``. The registry is pointed at the root, not at the run directory."""
    matches, squad_a, squad_b = _registry_keyed_history()
    out = tmp_path_factory.mktemp("xi_service_artifacts")
    with fast_fits():
        written = retrain(build(_ListSource(matches)), str(out), pd.Timestamp("2023-05-01"), formats=["T20I"])
    assert "targets" in written["summary"]["formats"][0]["performance"]
    return str(out), squad_a, squad_b, matches


@pytest.fixture
def registry(artifacts_dir) -> xi_service.XiRegistry:
    reg = xi_service.XiRegistry()
    reg.reload(artifacts_dir[0])
    return reg


def test_status_lists_the_performance_formats(registry) -> None:
    assert registry.status().performance_formats == ["T20I"]


def test_predict_performance_returns_distributions_for_both_elevens(registry, artifacts_dir) -> None:
    _, squad_a, squad_b, _ = artifacts_dir
    req = PerformancePredictRequest(
        format="t20i", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11] + ["nosuchplayer"]
    )

    res = xi_service.predict_performance(req, registry)

    assert len(res.players) == 23 and res.innings_marginalised is True
    assert [p.side for p in res.players] == [1] * 11 + [2] * 12
    assert res.unknown_player_ids == ["nosuchplayer"]
    first = res.players[0]
    assert first.player_id == squad_a[0]
    assert 0.0 <= first.p_bats <= 1.0 and first.runs.q10 <= first.runs.median <= first.runs.q90
    assert first.wickets.p0 + first.wickets.p1 + first.wickets.p2_plus == pytest.approx(1.0)
    assert res.served_ratings.run_id == registry.status().run_id


def test_predict_performance_marginalises_unless_the_toss_is_known(registry, artifacts_dir) -> None:
    _, squad_a, squad_b, _ = artifacts_dir
    base = dict(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11])

    unknown = xi_service.predict_performance(PerformancePredictRequest(**base), registry)
    first = xi_service.predict_performance(PerformancePredictRequest(**base, team1_bats_first=True), registry)
    chase = xi_service.predict_performance(PerformancePredictRequest(**base, team1_bats_first=False), registry)

    assert first.innings_marginalised is False
    assert [p.side for p in chase.players] == [1] * 11 + [2] * 11  # side is the eleven, not the innings
    for u, f, c in zip(unknown.players, first.players, chase.players):
        assert u.runs.median == pytest.approx(0.5 * (f.runs.median + c.runs.median), abs=1e-9)


def test_predict_performance_without_an_artifact_is_unavailable(registry) -> None:
    registry._store.performance = {}

    with pytest.raises(xi_service.XiUnavailable, match="no performance model"):
        xi_service.predict_performance(
            PerformancePredictRequest(format="T20I", team1_player_ids=["a"], team2_player_ids=["b"]), registry
        )


def test_registry_reports_absent_artifacts_without_raising(tmp_path) -> None:
    reg = xi_service.XiRegistry()
    status = reg.reload(str(tmp_path))
    assert status["loaded"] is False
    with pytest.raises(xi_service.XiUnavailable):
        reg.store("T20I")
    assert xi_service.loaded_formats(reg) == {"loaded_xi_formats": []}


def test_registry_survives_a_corrupt_artifact(tmp_path) -> None:
    """A run whose manifest reads but whose joblib does not: refused, and the reason is
    kept rather than the process taken down."""
    from ml.xi import runs

    directory = runs.run_dir(str(tmp_path), "20260902T101500Z-corrupt0")
    (directory + "/").replace("//", "/")
    import os

    os.makedirs(directory)
    (tmp_path / "runs" / "20260902T101500Z-corrupt0" / "xi_ratings.joblib").write_bytes(b"not a joblib file")
    runs.write_manifest(
        directory,
        runs.RunManifest(
            run_id="20260902T101500Z-corrupt0",
            created_at="2026-09-02T10:15:00+00:00",
            cutoff="2025-09-01",
            ratings_through="2025-08-31",
            dataset_sha="",
            git_sha="",
        ),
    )
    reg = xi_service.XiRegistry()

    status = reg.reload(str(tmp_path))

    assert status["loaded"] is False
    assert status["error"]


def test_registry_status_after_load(registry, artifacts_dir) -> None:
    status = xi_service.status(registry)
    assert status.loaded and status.formats == ["T20I"]
    assert status.players > 20
    assert status.ratings_through is not None
    assert status.report["formats"][0]["format_code"] == "T20I"
    # H-16: the status names the run and what its manifest recorded.
    assert status.run_id
    assert status.manifest["formats"] == ["T20I"]
    assert status.manifest["hyperparameters"]["T20I"]["params"]
    with pytest.raises(xi_service.XiUnavailable, match="ODI"):
        registry.store("ODI")


def test_evaluate_report_is_read_from_the_loaded_artifacts_directory(registry, tmp_path) -> None:
    """The report and the models it describes must come from one directory.

    Resolving it through ``ml.config.default_artifacts_dir()`` instead looked equivalent and
    was not: that is the last fallback in the chain, so in any deployment that sets
    ``ML_SERVICE_OUTPUT_DIR`` -- the container does -- the service loaded artifacts from one
    place and looked for the report in another, and answered 503 with a report on disk.
    """
    models_dir = tmp_path / "artifacts"
    models_dir.mkdir()
    (models_dir / "xi_evaluate_report.json").write_text(json.dumps({"formats": {"T20I": {}}}))
    registry.reload(str(models_dir))

    assert xi_service.evaluate_report(registry)["formats"] == {"T20I": {}}


def test_evaluate_report_reports_a_missing_report_with_the_path_it_looked_in(registry, tmp_path) -> None:
    empty = tmp_path / "empty"
    empty.mkdir()
    registry.reload(str(empty))

    with pytest.raises(xi_service.XiUnavailable, match="no evaluation report at"):
        xi_service.evaluate_report(registry)


def test_evaluate_report_refuses_before_any_reload() -> None:
    """A registry that has never been reloaded has nowhere to read from, and says so rather
    than falling back to a directory nobody configured."""
    with pytest.raises(xi_service.XiUnavailable, match="never been loaded"):
        xi_service.evaluate_report(xi_service.XiRegistry())


def test_optimize_returns_ids_marginals_and_unknowns(registry, artifacts_dir) -> None:
    _, squad_a, squad_b, matches = artifacts_dir
    opponent = list(matches[-1].team2_players)
    req = XiOptimizeRequest(
        format="t20i",
        pool_player_ids=squad_a + ["nosuchplayer"],
        opponent_player_ids=opponent,
        constraints=XiConstraints(team_size=11, min_bowlers=3, require_keeper=False, must_exclude=[squad_a[0]]),
    )
    res = xi_service.optimize(req, registry)
    assert len(res.selected_player_ids) == 11
    assert squad_a[0] not in res.selected_player_ids
    assert res.unknown_player_ids == ["nosuchplayer"]
    assert set(res.marginal_values) == set(res.selected_player_ids)
    assert res.objective == "win"
    assert res.optimised is True
    assert 0.0 <= res.win_probability <= 1.0


def test_optimize_rating_ordered_needs_no_opponent_and_is_marked_not_optimised(registry, artifacts_dir) -> None:
    _, squad_a, _, _ = artifacts_dir
    req = XiOptimizeRequest(
        format="t20i",
        pool_player_ids=squad_a,
        objective="ratings",
        constraints=XiConstraints(team_size=11, min_bowlers=3, require_keeper=False),
    )
    res = xi_service.optimize(req, registry)
    assert len(res.selected_player_ids) == 11
    assert res.objective == "ratings"
    assert res.optimised is False
    assert res.win_probability is None
    assert res.evaluations == 0
    assert res.marginal_values == {}


def test_optimize_carries_a_selection_reason_for_every_player_it_picked(registry, artifacts_dir) -> None:
    """P1-3: the card is assembled from what the selection read, so every selected player
    must arrive with that state and the pool the percentile is taken over."""
    _, squad_a, _, matches = artifacts_dir
    req = XiOptimizeRequest(
        format="t20i",
        pool_player_ids=squad_a,
        opponent_player_ids=list(matches[-1].team2_players),
        constraints=XiConstraints(team_size=11, min_bowlers=3, require_keeper=False),
    )

    res = xi_service.optimize(req, registry)

    assert set(res.selection_reasons) == set(res.selected_player_ids)
    for player_id, reason in res.selection_reasons.items():
        assert reason.pool_size == len(squad_a)
        assert 0.0 <= reason.rating_percentile <= 100.0
        assert reason.best_alternative is not None
        assert reason.best_alternative.player_id not in res.selected_player_ids
        assert player_id not in {reason.best_alternative.player_id}


def test_a_rating_ordered_selection_reason_names_no_alternative(registry, artifacts_dir) -> None:
    """The rating-ordered path maximises nothing, so the card gets no win-model comparison
    to show for a format the policy scoped off (P1-3 § 3)."""
    _, squad_a, _, _ = artifacts_dir
    req = XiOptimizeRequest(
        format="t20i",
        pool_player_ids=squad_a,
        objective="ratings",
        constraints=XiConstraints(team_size=11, min_bowlers=3, require_keeper=False),
    )

    res = xi_service.optimize(req, registry)

    assert set(res.selection_reasons) == set(res.selected_player_ids)
    assert all(reason.best_alternative is None for reason in res.selection_reasons.values())
    assert all(reason.best_alternative_note is None for reason in res.selection_reasons.values())


def test_optimize_refuses_the_win_objective_where_it_does_not_rank(registry, artifacts_dir) -> None:
    """H-17: TEST has no objective that ranks, so the win objective is not offered there."""
    _, squad_a, _, matches = artifacts_dir
    req = XiOptimizeRequest(
        format="TEST",
        pool_player_ids=squad_a,
        opponent_player_ids=list(matches[-1].team2_players),
    )
    with pytest.raises(xi_service.XiUnavailable, match="H-17") as excinfo:
        xi_service.optimize(req, registry)
    assert "not offered an optimised selection" in str(excinfo.value)
    assert "objective='ratings'" in str(excinfo.value)


def test_every_format_is_either_optimised_or_names_why_not() -> None:
    """The two policy tables are one map: a format is searched on the win objective unless
    NOT_OPTIMISED_REASONS says why it is not, and every reason names its rule."""
    from ml.xi import contract as C
    from ml.xi.optimizer import NOT_OPTIMISED_REASONS, OPTIMISED_SELECTION_FORMATS

    assert OPTIMISED_SELECTION_FORMATS | set(NOT_OPTIMISED_REASONS) == set(C.FORMAT_CODES)
    assert not OPTIMISED_SELECTION_FORMATS & set(NOT_OPTIMISED_REASONS)
    for format_code, reason in NOT_OPTIMISED_REASONS.items():
        assert "H-17" in reason or "E5" in reason, (format_code, reason)


def test_optimize_win_objective_requires_an_opponent_xi(registry, artifacts_dir) -> None:
    _, squad_a, _, _ = artifacts_dir
    req = XiOptimizeRequest(format="T20I", pool_player_ids=squad_a)
    with pytest.raises(xi_service.XiUnavailable, match="opponent_player_ids"):
        xi_service.optimize(req, registry)


def test_optimize_infeasible_constraints_raise_value_error(registry, artifacts_dir) -> None:
    _, squad_a, _, matches = artifacts_dir
    req = XiOptimizeRequest(
        format="T20I", pool_player_ids=squad_a[:5], opponent_player_ids=list(matches[-1].team2_players)
    )
    with pytest.raises(ValueError):
        xi_service.optimize(req, registry)


def test_optimize_holds_the_must_include_lock_through_the_endpoint(registry, artifacts_dir) -> None:
    """B-10, at the boundary go-app calls: a must_include id on the request is in the
    answer, on the searched path and on the rating-ordered one alike."""
    _, squad_a, _, matches = artifacts_dir
    opponent = list(matches[-1].team2_players)
    unlocked = xi_service.optimize(
        XiOptimizeRequest(
            format="t20i",
            pool_player_ids=squad_a,
            opponent_player_ids=opponent,
            constraints=XiConstraints(team_size=11, min_bowlers=0, require_keeper=False),
        ),
        registry,
    )
    required = sorted(set(squad_a) - set(unlocked.selected_player_ids))
    assert required, "this fixture needs a pool bigger than the eleven"
    constraints = XiConstraints(team_size=11, min_bowlers=0, require_keeper=False, must_include=required)

    searched = xi_service.optimize(
        XiOptimizeRequest(
            format="t20i", pool_player_ids=squad_a, opponent_player_ids=opponent, constraints=constraints
        ),
        registry,
    )
    rating_ordered = xi_service.optimize(
        XiOptimizeRequest(format="t20i", pool_player_ids=squad_a, objective="ratings", constraints=constraints),
        registry,
    )

    assert set(required) <= set(searched.selected_player_ids)
    assert set(required) <= set(rating_ordered.selected_player_ids)


def test_optimize_refuses_a_must_include_id_the_pool_does_not_hold(registry, artifacts_dir) -> None:
    """The conflict reaches the caller as a named reason rather than an eleven that
    quietly leaves the asked-for player out (§8.7)."""
    _, squad_a, _, matches = artifacts_dir
    req = XiOptimizeRequest(
        format="T20I",
        pool_player_ids=squad_a,
        opponent_player_ids=list(matches[-1].team2_players),
        constraints=XiConstraints(team_size=11, min_bowlers=0, require_keeper=False, must_include=["nosuchplayer"]),
    )

    with pytest.raises(ValueError, match="this pool does not hold: nosuchplayer"):
        xi_service.optimize(req, registry)


def test_predict_win_with_and_without_context(registry, artifacts_dir) -> None:
    _, _, _, matches = artifacts_dir
    last = matches[-1]
    t1, t2 = list(last.team1_players), list(last.team2_players)
    plain = xi_service.predict_win(XiWinRequest(format="T20I", team1_player_ids=t1, team2_player_ids=t2), registry)
    ctx = xi_service.predict_win(
        XiWinRequest(format="T20I", team1_player_ids=t1, team2_player_ids=t2, team1_id=7, team2_id=8, venue_id=3),
        registry,
    )
    for r in (plain, ctx):
        assert 0.0 <= r.team1_win_probability <= 1.0
        assert 0.0 <= r.objective_probability <= 1.0
    assert plain.objective_probability == pytest.approx(ctx.objective_probability), "context never enters the objective"


def test_predict_win_reports_no_constraint_check_when_none_was_asked_for(registry, artifacts_dir) -> None:
    """The optimised path sends no constraints, and gets no check back."""
    _, _, _, matches = artifacts_dir
    last = matches[-1]

    res = xi_service.predict_win(
        XiWinRequest(
            format="T20I", team1_player_ids=list(last.team1_players), team2_player_ids=list(last.team2_players)
        ),
        registry,
    )

    assert res.team1_constraint_check is None and res.team2_constraint_check is None


def test_predict_win_checks_a_hand_built_eleven_against_its_constraints(registry, artifacts_dir) -> None:
    """Play mode: the eleven is scored as sent, and the constraints are reported, not applied."""
    _, _, _, matches = artifacts_dir
    last = matches[-1]
    xi1, xi2 = list(last.team1_players), list(last.team2_players)

    res = xi_service.predict_win(
        XiWinRequest(
            format="T20I",
            team1_player_ids=xi1,
            team2_player_ids=xi2,
            team1_constraints=XiConstraints(team_size=11, min_bowlers=0, require_keeper=False),
            team2_constraints=XiConstraints(team_size=11, min_bowlers=0, require_keeper=False),
        ),
        registry,
    )

    assert res.team1_constraint_check.team_size == len(xi1)
    assert res.team1_constraint_check.bowlers >= 0
    assert res.team1_constraint_check.met is True
    assert res.team2_constraint_check.met is True


def test_a_constraint_a_hand_built_eleven_breaks_is_reported_not_repaired(registry, artifacts_dir) -> None:
    """An eleven short of bowlers, or missing a must-include, comes back broken and unchanged."""
    _, _, _, matches = artifacts_dir
    last = matches[-1]
    xi1, xi2 = list(last.team1_players), list(last.team2_players)
    unreachable_bowlers = 11

    res = xi_service.predict_win(
        XiWinRequest(
            format="T20I",
            team1_player_ids=xi1,
            team2_player_ids=xi2,
            team1_constraints=XiConstraints(
                team_size=11,
                min_bowlers=unreachable_bowlers,
                require_keeper=False,
                must_include=[xi1[0], "nosuchplayer"],
            ),
        ),
        registry,
    )

    assert res.team1_constraint_check.met is False
    assert res.team1_constraint_check.bowlers < unreachable_bowlers
    assert res.team1_constraint_check.min_bowlers == unreachable_bowlers
    assert res.team1_constraint_check.missing_must_include == ["nosuchplayer"]
    assert 0.0 <= res.team1_win_probability <= 1.0, "the eleven is still scored as it was sent"


def test_the_constraint_check_never_moves_the_probability(registry, artifacts_dir) -> None:
    """Checking is a measurement of the eleven, not a change to it."""
    _, _, _, matches = artifacts_dir
    last = matches[-1]
    request = {
        "format": "T20I",
        "team1_player_ids": list(last.team1_players),
        "team2_player_ids": list(last.team2_players),
    }

    plain = xi_service.predict_win(XiWinRequest(**request), registry)
    checked = xi_service.predict_win(
        XiWinRequest(**request, team1_constraints=XiConstraints(min_bowlers=5)),
        registry,
    )

    assert checked.team1_win_probability == pytest.approx(plain.team1_win_probability)
    assert checked.served_ratings == plain.served_ratings


# ---------------------------------------------------------------------------
# Postgres source, with a fake connection
# ---------------------------------------------------------------------------


class _FakeCursor:
    def __init__(self, tables):
        self.tables = tables
        self.rows = []

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def execute(self, sql, params):
        if "count(*) FROM match" in sql:
            self.rows = [(len(self.tables["matches"]),)]
        elif "FROM match m" in sql:
            self.rows = self.tables["matches"]
        elif "FROM match_player" in sql:
            self.rows = self.tables["players"].get(params[0], [])
        elif "FROM ball_event" in sql:
            self.rows = self.tables["balls"].get(params[0], [])
        else:
            raise AssertionError(sql)

    def fetchall(self):
        return self.rows

    def fetchone(self):
        return self.rows[0]


class _FakeConnection:
    def __init__(self, tables):
        self.tables = tables

    def cursor(self):
        return _FakeCursor(self.tables)


def test_postgres_source_maps_rows_and_skips_sides_without_squads() -> None:
    tables = {
        # Columns follow _MATCH_SQL: ..., winner, event name, match number, stage, group.
        "matches": [
            (1, date(2024, 1, 1), "T20I", "male", 5, 10, 20, 20, "Tri-series", 1, "", ""),
            (2, date(2024, 1, 2), "T20I", "male", 5, 10, 20, None, "", None, "", ""),
            (3, date(2024, 1, 3), "T20I", "male", None, 10, 20, 10, None, None, None, None),
        ],
        # Player columns are keys, not ids: the query resolves player.external_id (P-1).
        "players": {
            1: [(f"a{i:07x}", 10) for i in range(11)] + [(f"b{i:07x}", 20) for i in range(11)],
            2: [(f"a{i:07x}", 10) for i in range(11)],  # side 20 has no squad -> skipped
            3: [(f"a{i:07x}", 10) for i in range(11)] + [(f"b{i:07x}", 20) for i in range(11)],
        },
        "balls": {
            1: [
                (1, 0, "a0000000", "b0000000", 4, 4, None, None, None),
                (1, 0, "a0000000", "b0000000", 0, 0, "caught", ["b0000005"], "a0000000"),
                (2, 0, "b0000000", "a0000000", 0, 1, "run out", ["a0000005"], "b0000000"),
            ],
            3: [(1, 3, "a0000001", "b0000000", 1, 1, None, None, None)],
        },
    }
    recs = list(PostgresSource(_FakeConnection(tables), formats=["T20I"]).iter_matches())

    assert [r.match_id for r in recs] == ["1", "3"]
    first = recs[0]
    assert first.team1 == "10" and first.team2 == "20" and first.winner == "20" and first.outcome == 0.0
    assert first.venue == "5" and first.competition == "Tri-series"
    # a match without a venue or an event reads the empty key on both sources (A-1)
    assert recs[1].venue == "" and recs[1].competition == ""
    assert first.team1_players[0] == "a0000000" and len(first.team2_players) == 11
    d = first.deliveries
    assert list(d.innings) == [0, 0, 1]
    assert list(d.bowler_wicket) == [0.0, 1.0, 0.0]
    assert list(d.wicket) == [0.0, 1.0, 1.0]
    assert d.fielders[1] == ["b0000005"]
    assert recs[1].outcome == 1.0
    assert "caught" in BOWLER_CREDITED_KINDS and "run out" not in BOWLER_CREDITED_KINDS
    # The event fields reach the record verbatim and the stakes derivation reads them over
    # the whole set the source yields (X-3). Both matches here are between the same two
    # clubs, so the named edition is a bilateral series; the match with no event name
    # belongs to no edition and stays unlabelled rather than being guessed at.
    assert first.match_number == 1 and first.event_stage == "" and first.event_group == ""
    assert first.stakes.stage_label == "bilateral" and not first.stakes.dead_rubber
    assert recs[1].stakes.stage_label == "" and not recs[1].stakes.dead_rubber_known


def test_postgres_source_keys_players_by_the_registry_identifier() -> None:
    """The Postgres and JSON paths must produce the same key for the same person, so a
    rating artifact built from either is comparable (P-1). The SQL is what enforces it."""
    from ml.xi.sources import _BALLS_SQL, _PLAYERS_SQL

    assert "p.external_id" in _PLAYERS_SQL
    assert "striker.external_id" in _BALLS_SQL and "bowler.external_id" in _BALLS_SQL
    # A person the source has no registry entry for falls back the same way the JSON path
    # does, rather than silently keying on an integer the other path cannot produce.
    assert "'name:' || p.player_name" in _PLAYERS_SQL


def test_postgres_source_carries_missing_delivery_players_as_empty_keys() -> None:
    """striker_id and bowler_id are nullable, so the join can yield NULL. That must become
    an empty key, not the string "None", which would become a rated player."""
    d = _deliveries_from_rows([(1, 0, None, None, 0, 0, None, None, None)])

    assert list(d.batter) == [""] and list(d.bowler) == [""]
    assert d.fielders == [[]]


def test_postgres_balls_are_read_in_playing_order_not_legal_ball_order() -> None:
    """``ball_seq`` counts legal balls, so a wide shares it with the delivery before it and
    the order within the tie was the planner's; (innings, over, ball) is unique. The
    sequence features are the first to read delivery order, and H-8 caught the tie."""
    from ml.xi.sources import _BALLS_SQL

    order_by = _BALLS_SQL.strip().splitlines()[-1]

    assert order_by == "ORDER BY be.innings, be.over, be.ball"
    assert "ball_seq" not in _BALLS_SQL


def test_deliveries_from_rows_handles_empty() -> None:
    assert len(_deliveries_from_rows([])) == 0


def test_retrain_cli_runs_on_a_tiny_cricsheet_directory(tmp_path) -> None:
    """The command end to end on two files: every format is skipped for lack of rows, and
    the run -- report, rating state and manifest -- is still written."""
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    src = tmp_path / "json"
    src.mkdir()
    players = {"X": [f"X{i}" for i in range(11)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "a.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], players, "X", 1)))
    (src / "b.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], players, "Y", 2)))
    out = tmp_path / "artifacts"

    rc = retrain_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert rc == 0
    from ml.xi import runs

    run_id = runs.newest_run_id(str(out))
    assert run_id, "a completed retrain leaves a run with a manifest"
    directory = runs.run_dir(str(out), run_id)
    report = json.loads(
        (directory + "/xi_win_report.json").replace("//", "/") and open(directory + "/xi_win_report.json").read()
    )
    assert report["n_rows"] == 2
    assert all("skipped_reason" in f for f in report["formats"])
    assert (out / "runs" / run_id / "xi_ratings.joblib").exists()


# ---------------------------------------------------------------------------
# Fielder credit
# ---------------------------------------------------------------------------


def test_credited_fielder_keys_skips_a_fielder_the_source_cannot_name() -> None:
    """469 dismissals in the dataset name their fielder as {"substitute": true} and nothing
    else. Keyed on the empty name they became one rating slot -- a fictional cricketer with
    a fielding record built from 365 matches."""
    from ml.xi.sources import _credited_fielder_keys

    wickets = [{"kind": "caught", "fielders": [{"substitute": True}]}]

    assert _credited_fielder_keys(wickets, {}) == []


def test_credited_fielder_keys_still_credits_a_named_substitute() -> None:
    """3,324 substitute fielders in the dataset *are* named, and every one has a registry
    entry. A substitute is a person; only an unnamed one is nobody."""
    from ml.xi.sources import _credited_fielder_keys

    wickets = [{"kind": "caught", "fielders": [{"name": "KH Sciver-Brunt", "substitute": True}]}]

    assert _credited_fielder_keys(wickets, {"KH Sciver-Brunt": "6a434bd3"}) == ["6a434bd3"]


def test_credited_fielder_keys_falls_back_to_the_name_without_a_registry_entry() -> None:
    from ml.xi.sources import _credited_fielder_keys

    wickets = [{"kind": "run out", "fielders": [{"name": "Unregistered Fielder"}]}]

    assert _credited_fielder_keys(wickets, {}) == ["name:Unregistered Fielder"]


def test_credited_fielder_keys_keeps_the_named_fielders_of_a_mixed_dismissal() -> None:
    """A run out can credit two fielders and Cricsheet may name only one of them."""
    from ml.xi.sources import _credited_fielder_keys

    wickets = [{"kind": "run out", "fielders": [{"substitute": True}, {"name": "MM Ali"}]}]

    assert _credited_fielder_keys(wickets, {"MM Ali": "abc12345"}) == ["abc12345"]


def test_an_unnamed_substitute_costs_the_fielding_credit_and_not_the_wicket() -> None:
    """The dismissal is counted from its kind, so dropping the fielder loses who took the
    catch and nothing else."""
    from ml.xi.sources import _deliveries_from_cricsheet

    innings = [
        {
            "overs": [
                {
                    "over": 0,
                    "deliveries": [
                        {
                            "batter": "A1",
                            "bowler": "B1",
                            "runs": {"batter": 0, "total": 0},
                            "wickets": [{"kind": "caught", "fielders": [{"substitute": True}]}],
                        }
                    ],
                }
            ]
        }
    ]

    d = _deliveries_from_cricsheet(innings, {"A1": "aaa", "B1": "bbb"})

    assert list(d.wicket) == [1.0]
    assert list(d.bowler_wicket) == [1.0]
    assert d.fielders == [[]]


# ---------------------------------------------------------------------------
# As-of serving (P-2): the registry answers "ratings as of date D"
# ---------------------------------------------------------------------------


def _registry_with_as_of(artifacts_dir) -> tuple:
    """A fresh registry over the shared artifacts, serving the as-of pass from memory."""
    out, squad_a, squad_b, matches = artifacts_dir
    reg = xi_service.XiRegistry()
    reg.reload(out)
    reg.as_of_source_factory = lambda: _ListSource(matches)
    return reg, squad_a, squad_b, matches


def test_store_as_of_none_serves_the_loaded_state(artifacts_dir) -> None:
    reg, _, _, _ = _registry_with_as_of(artifacts_dir)

    assert reg.store_as_of("T20I", None) is reg.store("T20I")


def test_store_as_of_future_date_serves_the_loaded_state(artifacts_dir) -> None:
    """Ratings through today already are the as-of state for any later date."""
    reg, _, _, matches = _registry_with_as_of(artifacts_dir)
    after_everything = matches[-1].match_date + timedelta(days=1)

    assert reg.store_as_of("T20I", after_everything) is reg.store("T20I")


def test_store_as_of_past_date_serves_a_state_that_stops_there(artifacts_dir) -> None:
    reg, _, _, matches = _registry_with_as_of(artifacts_dir)
    as_of = matches[80].match_date

    store = reg.store_as_of("T20I", as_of)

    assert store is not reg.store("T20I")
    assert store.state.last_date < as_of
    assert store.state.matches_seen < reg.store("T20I").state.matches_seen
    assert store.models is reg.store("T20I").models  # same fitted models, earlier ratings


def test_predict_win_with_as_of_uses_the_earlier_ratings(artifacts_dir) -> None:
    reg, squad_a, squad_b, matches = _registry_with_as_of(artifacts_dir)
    request = dict(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11])

    today = xi_service.predict_win(XiWinRequest(**request), registry=reg)
    early = xi_service.predict_win(XiWinRequest(**request, as_of=matches[30].match_date), registry=reg)

    assert today.team1_win_probability != pytest.approx(early.team1_win_probability, abs=1e-12)
    # The stamp is read off the state that answered: a backtest's answer is dated by the
    # as-of state it was served from, not by the through-today state (P1-5).
    assert today.served_ratings.ratings_through == reg.status().ratings_through
    assert early.served_ratings.ratings_through < matches[30].match_date.isoformat()
    assert early.served_ratings.run_id == today.served_ratings.run_id


def test_simulate_returns_totals_scorecard_and_both_win_probabilities(registry, artifacts_dir) -> None:
    _, squad_a, squad_b, _ = artifacts_dir
    req = SimulateRequest(format="t20i", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11], n_samples=300)

    res = xi_service.simulate(req, registry)

    assert res.n_samples == 300 and res.toss_marginalised is True and res.format == "T20I"
    assert len(res.team1.players) == 11 and [p.side for p in res.team2.players] == [2] * 11
    lines = sum(p.scorecard.runs for p in res.team1.players) + res.team1.extras_scorecard
    assert lines == pytest.approx(res.team1.total.scorecard)  # the scorecard sums to the total by construction
    assert res.team1.total.q10 <= res.team1.total.median <= res.team1.total.q90
    assert sum(p.spread_share for p in res.team1.players) + res.team1.extras_spread_share == pytest.approx(1.0)
    assert 0.0 <= res.win_probability.simulated <= 1.0 and 0.0 <= res.win_probability.display <= 1.0
    assert res.win_probability.headline_source == "display"  # E2's rule: the display model stays the headline
    assert res.win_probability.headline == res.win_probability.display
    assert res.margin.p_bat_first_wins + res.margin.p_chaser_wins + res.margin.p_tie == pytest.approx(1.0)
    status = registry.status()
    assert res.served_ratings.run_id == status.run_id
    assert res.served_ratings.ratings_through == status.ratings_through


def test_simulate_is_deterministic_for_a_seed_and_honours_a_known_toss(registry, artifacts_dir) -> None:
    _, squad_a, squad_b, _ = artifacts_dir
    base = dict(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11], n_samples=200)

    first = xi_service.simulate(SimulateRequest(**base, seed=5), registry)
    again = xi_service.simulate(SimulateRequest(**base, seed=5), registry)
    known = xi_service.simulate(SimulateRequest(**base, seed=5, team1_bats_first=True), registry)

    assert first.team1.total == again.team1.total and first.win_probability == again.win_probability
    assert known.toss_marginalised is False


def test_simulate_refuses_a_format_without_an_innings_length(registry) -> None:
    with pytest.raises(SimulationUnavailable):
        xi_service.simulate(SimulateRequest(format="TEST", team1_player_ids=["a"], team2_player_ids=["b"]), registry)


def test_a_numeric_player_id_is_refused_rather_than_silently_unrated() -> None:
    """D-7a: the wire carries the Cricsheet registry id. A caller sending go-app's numeric
    `player_id` used to be accepted and match nobody, so every player came back unrated and
    the XI was eleven debutants. The contract now rejects it at the boundary."""
    with pytest.raises(ValidationError):
        XiOptimizeRequest(format="T20I", pool_player_ids=[1, 2, 3], opponent_player_ids=["2911de16"])
