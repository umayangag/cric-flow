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

from app import serving_compute, xi_service
from app.models.xi import PerformancePredictRequest, SimulateRequest, XiConstraints, XiOptimizeRequest, XiWinRequest
from ml.xi import simulator
from ml.xi.biography import DAYS_PER_YEAR
from ml.xi.builder import build
from ml.xi.perf_calibration import QuantileRecalibration
from ml.xi.retrain import main as retrain_main
from ml.xi.retrain import retrain
from ml.xi.simulator import SimulationUnavailable
from ml.xi.sources import Deliveries, MatchRecord, PostgresSource, _deliveries_from_rows
from ml.xi.store import STATE_ARRAY_NAMES
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history
from tests.xi_fixtures import xi
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
            d.runs_batter, d.runs_total, d.runs_bowler, d.faced, d.wicket, d.bowler_wicket, d.stumping, [remap(f) for f in d.fielders],
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
        format="t20i", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:10] + ["nosuchplayer"]
    )

    res = xi_service.predict_performance(req, registry)

    assert len(res.players) == 22 and res.innings_marginalised is True
    assert [p.side for p in res.players] == [1] * 11 + [2] * 11
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


def test_predict_performance_reports_the_venue_the_model_actually_read(registry, artifacts_dir) -> None:
    """P3-2: the ground comes back as the two columns the model consumed, and a request the
    served state has no context for is reported as neutral rather than as a projection at a
    ground the model knows something about (§8.7)."""
    _, squad_a, squad_b, _ = artifacts_dir
    req = PerformancePredictRequest(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11])

    res = xi_service.predict_performance(req, registry)

    assert res.venue_context.venue_n == 0.0
    assert res.venue_context.venue_bf_rate == pytest.approx(0.5)
    assert res.venue_context.neutral is True


def test_predict_performance_says_which_quantiles_the_answering_model_corrects(registry, artifacts_dir) -> None:
    """EVAL-08, plan 8.7: a quantile that was never recalibrated must not be
    indistinguishable from one that was, so the answer names the targets this model's own
    calibration carries -- none of them where the fold was too thin to fit one, or where
    H-5 asked for none."""
    _, squad_a, squad_b, _ = artifacts_dir
    req = PerformancePredictRequest(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11])

    rng = np.random.RandomState(0)
    fitted = QuantileRecalibration.fit(
        np.sort(rng.uniform(0.0, 50.0, size=(300, 3)), axis=1), rng.uniform(0.0, 50.0, size=300)
    )

    uncorrected = xi_service.predict_performance(req, registry)
    registry._served.store.performance["T20I"].calibration["runs"] = fitted
    corrected = xi_service.predict_performance(req, registry)

    assert uncorrected.recalibrated_targets == []
    assert corrected.recalibrated_targets == ["runs"]
    first = corrected.players[0].runs
    assert first.q10 <= first.median <= first.q90
    assert first.median != uncorrected.players[0].runs.median


def test_predict_performance_without_an_artifact_is_unavailable(registry) -> None:
    registry._served.store.performance = {}

    with pytest.raises(xi_service.XiUnavailable, match="no performance model"):
        xi_service.predict_performance(
            PerformancePredictRequest(format="T20I", team1_player_ids=xi("a"), team2_player_ids=xi("b")), registry
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
            git_sha=runs.UNKNOWN,
            dataset_digest={"scheme": runs.DATASET_DIGEST_SCHEME, "matches": 0},
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
        format="T20I",
        pool_player_ids=squad_a,
        opponent_player_ids=list(matches[-1].team2_players),
        # Every place a bowling option and a keeper among them: a pool this size cannot.
        constraints=XiConstraints(min_bowlers=11, require_keeper=True),
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


def test_predict_win_reports_which_reading_it_answered(registry, artifacts_dir) -> None:
    """The response says whether the displayed probability read the toss or averaged both
    orders, so a caller does not have to re-read its own request to know which of the two
    quantities it is holding (GO-07, plan §8.7)."""
    _, _, _, matches = artifacts_dir
    last = matches[-1]
    t1, t2 = list(last.team1_players), list(last.team2_players)

    marginal = xi_service.predict_win(XiWinRequest(format="T20I", team1_player_ids=t1, team2_player_ids=t2), registry)
    bats_first = xi_service.predict_win(
        XiWinRequest(format="T20I", team1_player_ids=t1, team2_player_ids=t2, team1_bats_first=True), registry
    )
    chases = xi_service.predict_win(
        XiWinRequest(format="T20I", team1_player_ids=t1, team2_player_ids=t2, team1_bats_first=False), registry
    )

    assert marginal.toss_marginalised is True
    assert bats_first.toss_marginalised is False and chases.toss_marginalised is False
    # The marginalised reading is the average of the two oriented ones, which is what makes
    # them three different numbers rather than one under three labels.
    assert marginal.team1_win_probability == pytest.approx(
        0.5 * (bats_first.team1_win_probability + chases.team1_win_probability)
    )
    # The objective has no batting-order feature, so it is the same number either way and
    # the reading never describes it.
    assert marginal.objective_probability == pytest.approx(bats_first.objective_probability)
    assert marginal.objective_probability == pytest.approx(chases.objective_probability)


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

    def execute(self, sql, params=None):
        if "count(*) FROM match" in sql:
            self.rows = [(len(self.tables["matches"]),)]
        elif "FROM match m" in sql:
            self.rows = self.tables["matches"]
        elif "FROM match_player" in sql:
            self.rows = self.tables["players"].get(params[0], [])
        elif "FROM ball_event" in sql:
            self.rows = self.tables["balls"].get(params[0], [])
        elif "FROM venue" in sql:
            self.rows = self.tables.get("venues", [])
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
        # Columns follow _MATCH_SQL: ..., winner, event name, match number, stage, group,
        # result, toss winner. Match 2 is a tie settled by a super over: a winner with
        # result 'tie'; match 3 records no toss.
        "matches": [
            (1, date(2024, 1, 1), "T20I", "male", 5, 10, 20, 20, "Tri-series", 1, "", "", None, 10),
            (2, date(2024, 1, 2), "T20I", "male", 5, 10, 20, None, "", None, "", "", None, 20),
            (3, date(2024, 1, 3), "T20I", "male", None, 10, 20, 10, None, None, None, None, "tie", None),
        ],
        # Player columns are keys, not ids: the query resolves player.external_id (P-1).
        "players": {
            1: [(f"a{i:07x}", 10, False) for i in range(11)] + [(f"b{i:07x}", 20, False) for i in range(11)],
            2: [(f"a{i:07x}", 10, False) for i in range(11)],  # side 20 has no squad -> skipped
            3: [(f"a{i:07x}", 10, False) for i in range(11)] + [(f"b{i:07x}", 20, False) for i in range(11)],
        },
        # Ball columns 6 and 8 are the wickets on the ball as two arrays in wicket order,
        # the kinds and the dismissed players' keys (ball_event_wicket, IMPORT-06).
        "balls": {
            1: [
                (1, 0, "a0000000", "b0000000", 4, 4, None, None, None, 0, 0, 0, 0),
                (1, 0, "a0000000", "b0000000", 0, 0, ["caught"], ["b0000005"], ["a0000000"], 0, 0, 0, 0),
                (2, 0, "b0000000", "a0000000", 0, 1, ["run out"], ["a0000005"], ["b0000000"], 0, 0, 0, 0),
            ],
            3: [(1, 3, "a0000001", "b0000000", 1, 1, None, None, None, 0, 0, 0, 0)],
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
    assert d.players_out == [[], ["a0000000"], ["b0000000"]]
    assert recs[1].outcome == 1.0
    # The event fields reach the record verbatim and the stakes derivation reads them over
    # the whole set the source yields (X-3). Both matches here are between the same two
    # clubs, so the named edition is a bilateral series; the match with no event name
    # belongs to no edition and stays unlabelled rather than being guessed at.
    assert first.match_number == 1 and first.event_stage == "" and first.event_group == ""
    assert first.stakes.stage_label == "bilateral" and not first.stakes.dead_rubber
    assert recs[1].stakes.stage_label == "" and not recs[1].stakes.dead_rubber_known
    # The result reaches the record as the archive path reads it (IMPORT-02 / FEAT-04): a
    # match won outright carries none, and a tie-breaker win keeps 'tie' beside its winner.
    assert first.result is None
    assert recs[1].result == "tie" and recs[1].winner == "10"
    # The toss winner (FEAT-05) is keyed as the sides are, so "the side batting first won
    # it" is one equality; a match the database records no toss for reads None, never 0.
    assert first.toss_winner == "10" and first.toss_won_by_team1 == 1.0
    assert recs[1].toss_winner is None and recs[1].toss_won_by_team1 is None


def test_postgres_source_places_venues_through_the_curated_table(tmp_path) -> None:
    """FEAT-05's static input on the database path: ``venue.country`` is NULL on every row
    of the archive, so the source joins each venue's *name* to the curated table by the
    identity-way key and hands the state a region per ``venue.id``. A venue the table has
    no row for is in no region."""
    curated = tmp_path / "venue-geocoding.csv"
    curated.write_text(
        "venue,venue_key,status,query,place,admin1,country_code,country,latitude,longitude,timezone,"
        "countries_voted,source,note\n"
        '"Eden Gardens, Kolkata",eden gardens kolkata,mapped,Kolkata,Kolkata,WB,IN,India,22.5,88.3,'
        "Asia/Kolkata,IN:9,open-meteo-geocoding,\n"
        '"Kensington Oval, Bridgetown",kensington oval bridgetown,mapped,Bridgetown,Bridgetown,,BB,Barbados,'
        "13.1,-59.6,America/Barbados,BB:2,open-meteo-geocoding,\n"
    )
    tables = {
        "matches": [],
        "players": {},
        "balls": {},
        "venues": [(5, "Eden Gardens, KOLKATA"), (6, "Kensington Oval, Bridgetown"), (7, "Nowhere Park")],
    }

    placed = PostgresSource(_FakeConnection(tables), venue_countries_path=str(curated)).venue_countries()

    assert placed == {"5": "IN", "6": "WI"}, "keyed by id; the West Indies fold to one region; Nowhere Park is unplaced"


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
    d = _deliveries_from_rows([(1, 0, None, None, 0, 0, None, None, None, 0, 0, 0, 0)])

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


def _ball_row(innings: int, over: int) -> tuple:
    """One ``_BALLS_SQL`` row, with only the columns these cases read filled in."""
    return (innings, over, "a", "b", 0, 0, None, None, None, 0, 0, 0, 0)


def test_postgres_innings_are_numbered_by_position_not_by_the_smallest_present() -> None:
    """IMPORT-12. ``ball_event.innings`` is the innings' position in the match, 1-based, so
    one subtracted is the 0-based index the archive path enumerates. Normalising by the
    smallest innings present was the same number only while innings 1 bowled a ball: a
    first innings that is forfeited, or made up entirely of extras, has no ``ball_event``
    row, and subtracting its absence renamed every innings after it."""
    d = _deliveries_from_rows([_ball_row(2, 0), _ball_row(3, 0)])

    assert list(d.innings) == [1, 2], "the second and third innings keep being the second and third"


def test_the_two_sources_number_a_forfeited_first_innings_the_same_way() -> None:
    """The parity check (H-15) compares counts across the database and the archive, and
    can only do so while both call the same innings by the same number. A forfeited first
    innings is the shape that used to split them: fourteen innings in the archive are
    forfeited and one is all extras, and the only reason this never misfired is that none
    of them is the first."""
    from ml.xi.sources import _deliveries_from_cricsheet

    innings = [
        {"team": "Alpha", "forfeited": True},
        {"team": "Beta", "overs": [{"over": 0, "deliveries": [_cricsheet_delivery()]}]},
        {"team": "Alpha", "overs": [{"over": 0, "deliveries": [_cricsheet_delivery()]}]},
    ]

    archive = _deliveries_from_cricsheet(innings, {})
    # What the importer writes for that match: nothing for the forfeited innings, and the
    # positions the scorecard gives the other two.
    database = _deliveries_from_rows([_ball_row(2, 0), _ball_row(3, 0)])

    assert list(archive.innings) == list(database.innings) == [1, 2]


def _cricsheet_delivery() -> dict:
    return {"batter": "A1", "bowler": "B1", "non_striker": "A2", "runs": {"batter": 0, "total": 0}}


# ---------------------------------------------------------------------------
# Runs charged to the bowler (FEAT-08): one rule on both sources
# ---------------------------------------------------------------------------


def test_runs_conceded_by_bowler_is_the_importers_rule() -> None:
    """Total less byes, leg-byes and penalty runs: wides and no-balls are the bowler's,
    the rest are the innings' (``cricsheet.Delivery.RunsConcededByBowler``)."""
    from ml.xi.sources import runs_conceded_by_bowler

    # a four, a wide, a no-ball with four leg-byes off it, four byes, a five-run penalty
    charged = runs_conceded_by_bowler(
        [4, 1, 5, 4, 5], byes=[0, 0, 0, 4, 0], legbyes=[0, 0, 4, 0, 0], penalty=[0, 0, 0, 0, 5]
    )

    assert list(charged) == [4.0, 1.0, 1.0, 0.0, 0.0]


def _no_ball_with_four_leg_byes() -> dict:
    """IMPORT-04's example as Cricsheet writes it: a no-ball the batter missed that ran
    away for four leg-byes. Five runs to the innings, one to the bowler."""
    return {
        "batter": "A1",
        "bowler": "B1",
        "runs": {"batter": 0, "extras": 5, "total": 5},
        "extras": {"noballs": 1, "legbyes": 4},
    }


def test_the_archive_path_charges_the_bowler_only_the_runs_he_conceded() -> None:
    """A no-ball with four leg-byes charges the bowler 1, not 5, and the innings still
    scores 5; a delivery with no ``extras`` object charges him everything it scored."""
    from ml.xi.sources import _deliveries_from_cricsheet

    plain_four = {"batter": "A1", "bowler": "B1", "runs": {"batter": 4, "extras": 0, "total": 4}}
    innings = [{"overs": [{"over": 0, "deliveries": [_no_ball_with_four_leg_byes(), plain_four]}]}]

    d = _deliveries_from_cricsheet(innings, {})

    assert list(d.runs_total) == [5.0, 4.0]
    assert list(d.runs_bowler) == [1.0, 4.0]


def test_the_postgres_path_charges_the_bowler_only_the_runs_he_conceded() -> None:
    """The same delivery read from ``ball_event`` with its extras by kind (migration
    0018): the bowler is charged 1 of the 5, exactly as the archive path charges him."""
    squad = [(f"a{i:07x}", 10, False) for i in range(11)] + [(f"b{i:07x}", 20, False) for i in range(11)]
    no_ball_with_four_leg_byes = (1, 0, "a0000000", "b0000000", 0, 5, None, None, None, 0, 4, 0, 0)
    plain_four = (1, 0, "a0000000", "b0000000", 4, 4, None, None, None, 0, 0, 0, 0)
    tables = {
        "matches": [(1, date(2024, 1, 1), "T20I", "male", 5, 10, 20, 20, "", None, "", "", None, 10)],
        "players": {1: squad},
        "balls": {1: [no_ball_with_four_leg_byes, plain_four]},
    }

    (record,) = PostgresSource(_FakeConnection(tables), formats=["T20I"]).iter_matches()

    assert list(record.deliveries.runs_total) == [5.0, 4.0]
    assert list(record.deliveries.runs_bowler) == [1.0, 4.0]


def test_postgres_balls_read_the_extras_by_kind() -> None:
    """The three kinds the bowler is not charged come from the row itself: nothing else
    in ``ball_event`` can say how many of a no-ball's five runs were leg-byes."""
    from ml.xi.sources import _BALLS_SQL

    assert "be.extras_byes, be.extras_legbyes, be.extras_penalty" in _BALLS_SQL


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


def _one_over_innings(batter: str, bowler: str, runs: int, super_over: bool = False) -> dict:
    """One innings of one delivery, flagged as a super over when asked."""
    inning = {
        "overs": [
            {"over": 0, "deliveries": [{"batter": batter, "bowler": bowler, "runs": {"batter": runs, "total": runs}}]}
        ]
    }
    if super_over:
        inning["super_over"] = True
    return inning


def test_the_archive_source_leaves_a_super_over_out_of_the_deliveries() -> None:
    """A tie-breaker is not an innings of the match: its deliveries are dropped and the
    innings that are kept are numbered as the go-app importer numbers them (IMPORT-01)."""
    from ml.xi.sources import _deliveries_from_cricsheet

    innings = [
        _one_over_innings("A1", "B1", 4),
        _one_over_innings("B1", "A1", 6),
        _one_over_innings("B7", "A4", 6, super_over=True),
        _one_over_innings("A1", "B1", 1, super_over=True),
    ]

    d = _deliveries_from_cricsheet(innings, {})

    assert list(d.innings) == [0, 1]
    assert list(d.batter) == ["name:A1", "name:B1"], "B7 batted only in the super over"
    assert list(d.bowler) == ["name:B1", "name:A1"], "A4 bowled only in the super over"
    assert list(d.runs_total) == [4.0, 6.0]


@pytest.mark.parametrize(
    "outcome,expected",
    [
        ({"winner": "Alpha", "by": {"runs": 12}}, "Alpha"),
        ({"result": "tie", "eliminator": "Beta"}, "Beta"),
        ({"result": "tie", "bowl_out": "Alpha"}, "Alpha"),
        ({"result": "tie"}, None),
        ({"result": "no result"}, None),
        ({"result": "draw"}, None),
        ({"winner": "Beta", "method": "Awarded"}, "Beta"),
        ({}, None),
    ],
)
def test_winning_team_reads_the_outcome_as_the_importer_does(outcome, expected) -> None:
    """The outright winner, else the side that won the tie-breaker, else none: the rule
    the go-app importer applies (IMPORT-02), so both sources agree on who has a winner."""
    from ml.xi.sources import winning_team

    assert winning_team(outcome) == expected


def test_played_innings_keeps_a_declared_or_forfeited_innings() -> None:
    """Only the super-over flag removes an innings; a first-class match's short innings
    are innings of the match."""
    from ml.xi.sources import played_innings

    innings = [{"team": "A", "declared": True}, {"team": "B"}, {"team": "A", "forfeited": True}, {"team": "B"}]

    assert played_innings(innings) == innings


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


def test_an_as_of_store_is_a_snapshot_the_advancing_pass_cannot_change(artifacts_dir) -> None:
    """SERVE-01: the pass folds matches into one ``RatingState`` in place, so a request
    handed the live object watches its own ratings move when the next request advances the
    pass -- silently, and on the threadpool with a reader half way through an array."""
    reg, _, _, matches = _registry_with_as_of(artifacts_dir)
    form_key = ("T20I", "7")

    early = reg.store_as_of("T20I", matches[30].match_date)
    seen, through = early.state.matches_seen, early.state.last_date
    pelo, elo = np.copy(early.state.pelo), early.state.team_elo.get(form_key)
    form = list(early.state.team_results.get(form_key, ()))
    later = reg.store_as_of("T20I", matches[80].match_date)

    assert later.state.matches_seen > seen, "the pass did advance, which is what makes this a test"
    assert early.state.matches_seen == seen and early.state.last_date == through
    np.testing.assert_array_equal(early.state.pelo, pelo)
    assert early.state.team_elo.get(form_key) == elo
    assert list(early.state.team_results.get(form_key, ())) == form, "a form list is appended to in place"


def test_one_as_of_date_is_answered_from_one_snapshot(artifacts_dir) -> None:
    """A backtest asks four surfaces about one fixture. They share the snapshot: identical
    ratings by construction, and one copy of the state rather than four in flight."""
    reg, _, _, matches = _registry_with_as_of(artifacts_dir)
    as_of = matches[40].match_date

    first = reg.store_as_of("T20I", as_of)
    again = reg.store_as_of("T20I", as_of)

    assert again is first


def test_reloading_a_run_drops_the_as_of_snapshot(artifacts_dir) -> None:
    """The snapshot belongs to the run that was serving when it was taken; a reload that
    replaced the models must not leave a store pairing the new state with the old ones."""
    out, _, _, matches = artifacts_dir
    reg, _, _, _ = _registry_with_as_of(artifacts_dir)
    as_of = matches[40].match_date
    before = reg.store_as_of("T20I", as_of)

    reg.reload(out)

    assert reg.store_as_of("T20I", as_of) is not before


def test_a_served_prediction_computes_with_one_thread_per_numeric_library(registry, artifacts_dir, monkeypatch) -> None:
    """SERVE-01's bill, pinned on a real prediction rather than on the decorator: the
    service functions carry the limit, so the whole answer -- rows, model, simulator draws
    -- is computed with one thread per library. Measured, on the four serving paths
    sequentially: optimize 268 -> 192 ms, predict-win 7.5 -> 1.8 ms, performance 265 -> 33
    ms, simulate 390 -> 140 ms. Nothing the service does wants a thread per core."""
    _, squad_a, squad_b, _ = artifacts_dir
    outside = serving_compute.library_thread_counts()
    inside = []
    stamp = xi_service._served_ratings
    monkeypatch.setattr(
        xi_service,
        "_served_ratings",
        lambda store: inside.extend(serving_compute.library_thread_counts()) or stamp(store),
    )

    xi_service.predict_win(
        XiWinRequest(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11]), registry=registry
    )

    assert outside, "threadpoolctl found no library to control; this assertion would prove nothing"
    assert inside == [1] * len(outside)


def test_serving_an_as_of_request_never_writes_to_the_rating_state(artifacts_dir) -> None:
    """The other half of SERVE-01: a read must be a read. ``_read_slots`` used to grow the
    arrays for the reserved unrated column on the read path, which several threads sharing
    one snapshot would race on. Every array is made read-only here, so any write on any of
    the four surfaces raises instead of passing unnoticed."""
    reg, squad_a, squad_b, matches = _registry_with_as_of(artifacts_dir)
    as_of = matches[40].match_date
    store = reg.store_as_of("T20I", as_of)
    for name in STATE_ARRAY_NAMES:
        getattr(store.state, name).flags.writeable = False
    eleven = dict(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11], as_of=as_of)

    xi_service.predict_win(XiWinRequest(**eleven), registry=reg)
    xi_service.predict_performance(PerformancePredictRequest(**eleven), registry=reg)
    xi_service.simulate(SimulateRequest(**eleven, n_samples=100), registry=reg)
    xi_service.optimize(
        XiOptimizeRequest(
            format="T20I",
            pool_player_ids=squad_a,
            opponent_player_ids=squad_b[:11],
            as_of=as_of,
            constraints=XiConstraints(team_size=11, min_bowlers=3, require_keeper=False),
        ),
        registry=reg,
    )

    assert len(store.state.players) == len(reg.store_as_of("T20I", as_of).state.players)


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


# --- SERVE-04: the fixture is dated by the caller, and the answer says by whom --------


def _eleven_against_eleven(artifacts_dir) -> dict:
    _, squad_a, squad_b, _ = artifacts_dir
    return {"format": "T20I", "team1_player_ids": squad_a[:11], "team2_player_ids": squad_b[:11]}


def test_a_performance_request_builds_its_rows_for_the_fixture_date_it_names(registry, artifacts_dir) -> None:
    """SERVE-04: the serving path stamped ``state.last_date`` on every fixture, so every
    date-dependent feature was read at the last match the state held rather than at the
    match being asked about. ``_fixture_rows`` is the seam where the resolved fixture meets
    the row assembly, so it is where the two are pinned together.

    The rows carry the age at the fixture date, which is the column the X-1b family reads
    and the one that moves by a day for every day the two dates are apart."""
    fixture_day = date(2024, 3, 1)
    born = date(1995, 6, 1)
    store = registry.store("T20I")
    request = PerformancePredictRequest(**_eleven_against_eleven(artifacts_dir), match_date=fixture_day)
    for key in request.team1_player_ids + request.team2_player_ids:
        store.state.birth_dates[key] = born

    stamp = xi_service.served_fixture(request)
    _, rows, _ = xi_service._fixture_rows(store, request, stamp)

    assert (stamp.match_date, stamp.match_date_source) == (fixture_day.isoformat(), "request")
    assert store.state.last_date != fixture_day, "the two dates differ, which is what makes this a test"
    assert (rows.match_date == pd.Timestamp(fixture_day)).all()
    assert rows.age.round(6).eq(round((fixture_day - born).days / DAYS_PER_YEAR, 6)).all()


def test_a_fixture_with_no_date_is_dated_today_and_says_so(registry, artifacts_dir) -> None:
    """A caller who names no date still gets one, because the features cannot be computed
    without it. What §8.7 forbids is that substitution being silent, so the answer carries
    the date it used and where the date came from -- never the state's own date, which is
    what it silently used before."""
    request = PerformancePredictRequest(**_eleven_against_eleven(artifacts_dir))

    res = xi_service.predict_performance(request, registry)

    assert res.fixture.match_date_source == "today"
    assert res.fixture.match_date == date.today().isoformat()
    assert res.fixture.match_date != registry.status().ratings_through


def test_a_backtest_dates_its_fixture_by_the_date_it_is_backtesting(registry, artifacts_dir) -> None:
    """``as_of`` selects which ratings answer; go-app sets it to a played match's own day,
    so it also dates the fixture when the caller named no ``match_date``. A named
    ``match_date`` still wins: the two answer different questions and only one of them is
    "when is this match played"."""
    as_of = date(2023, 4, 1)
    named = date(2023, 4, 20)

    dated_by_as_of = xi_service.served_fixture(
        PerformancePredictRequest(**_eleven_against_eleven(artifacts_dir), as_of=as_of)
    )
    dated_by_request = xi_service.served_fixture(
        PerformancePredictRequest(**_eleven_against_eleven(artifacts_dir), as_of=as_of, match_date=named)
    )

    assert (dated_by_as_of.match_date, dated_by_as_of.match_date_source) == (as_of.isoformat(), "as_of")
    assert (dated_by_request.match_date, dated_by_request.match_date_source) == (named.isoformat(), "request")


def test_a_simulated_fixture_carries_the_same_stamp_the_performance_rows_were_built_from(
    registry, artifacts_dir
) -> None:
    """One resolution serves both surfaces: a caller comparing ``/simulate`` with
    ``/performance/predict`` for the same fixture must not find them dated differently."""
    request = SimulateRequest(
        **_eleven_against_eleven(artifacts_dir), match_date=date(2024, 3, 1), n_samples=300, gender="female"
    )

    simulated = xi_service.simulate(request, registry)
    predicted = xi_service.predict_performance(PerformancePredictRequest(**request.model_dump()), registry)

    assert simulated.fixture.match_date == "2024-03-01" and simulated.fixture.gender == "female"
    assert simulated.fixture == predicted.fixture


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


def test_simulate_returns_the_totals_draws_only_when_they_are_asked_for(registry, artifacts_dir) -> None:
    """P3-2: a caller pooling several grounds needs the draws themselves, because a
    mixture's quantiles are not the mean of its parts'. They are off by default -- no
    surface needs n_samples floats a side -- and when returned they are the same numbers the
    quantiles summarise, so inverting them reproduces the served median exactly."""
    _, squad_a, squad_b, _ = artifacts_dir
    base = dict(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11], n_samples=200, seed=3)

    without = xi_service.simulate(SimulateRequest(**base), registry)
    with_draws = xi_service.simulate(SimulateRequest(**base, return_total_draws=True), registry)

    assert without.team1.total_draws is None and without.team2.total_draws is None
    assert len(with_draws.team1.total_draws) == 200 and len(with_draws.team2.total_draws) == 200
    assert with_draws.team1.total == without.team1.total  # the same draws, summarised the same way
    assert np.quantile(with_draws.team1.total_draws, 0.5) == pytest.approx(with_draws.team1.total.median)


def test_simulate_says_whether_its_simulator_carried_a_shared_factor(registry, artifacts_dir) -> None:
    """P2-4 / B-12: the flag is read off the calibration that drew the samples, so a record
    can keep factored and factorless predictions apart without a status call."""
    _, squad_a, squad_b, _ = artifacts_dir
    req = SimulateRequest(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11], n_samples=100)
    _, model = registry.performance("T20I", None)

    res = xi_service.simulate(req, registry)

    assert res.shared_factor is (model.simulation.shared_factor is not None)


def test_simulate_reports_no_shared_factor_when_the_calibration_fitted_none(
    registry, artifacts_dir, monkeypatch
) -> None:
    """The thin-fold case: the simulator ships without a factor and the answer says so."""
    _, squad_a, squad_b, _ = artifacts_dir
    req = SimulateRequest(format="T20I", team1_player_ids=squad_a[:11], team2_player_ids=squad_b[:11], n_samples=100)
    _, model = registry.performance("T20I", None)
    monkeypatch.setattr(
        model, "simulation", simulator.SimulatorCalibration(model.simulation.runs_balls_rho, None, None)
    )

    res = xi_service.simulate(req, registry)

    assert model.simulation.shared_factor is None
    assert res.shared_factor is False


def test_simulate_refuses_a_format_without_an_innings_length(registry) -> None:
    with pytest.raises(SimulationUnavailable):
        xi_service.simulate(
            SimulateRequest(format="TEST", team1_player_ids=xi("a"), team2_player_ids=xi("b")), registry
        )


def test_a_numeric_player_id_is_refused_rather_than_silently_unrated() -> None:
    """D-7a: the wire carries the Cricsheet registry id. A caller sending go-app's numeric
    `player_id` used to be accepted and match nobody, so every player came back unrated and
    the XI was eleven debutants. The contract now rejects it at the boundary."""
    with pytest.raises(ValidationError):
        XiOptimizeRequest(format="T20I", pool_player_ids=[1, 2, 3], opponent_player_ids=["2911de16"])


# ---------------------------------------------------------------------------
# A ball faced (IMPORT-05): one rule on both sources
# ---------------------------------------------------------------------------


def test_faced_by_batter_is_the_importers_rule() -> None:
    """Every delivery but a wide: a no-ball is faced, a wide is not
    (``cricsheet.Delivery.FacedByBatter``)."""
    from ml.xi.sources import faced_by_batter

    # a four, a wide, a no-ball, four byes, a penalty beside a wide
    faced = faced_by_batter([0, 1, 0, 0, 1])

    assert list(faced) == [1.0, 0.0, 1.0, 1.0, 0.0]


def _no_ball() -> dict:
    """A no-ball the batter played and missed, as Cricsheet writes it: one run to the
    innings and to the bowler, and a ball the batter faced."""
    return {"batter": "A1", "bowler": "B1", "runs": {"batter": 0, "extras": 1, "total": 1}, "extras": {"noballs": 1}}


def _wide() -> dict:
    """A wide: one run to the innings and to the bowler, and a ball nobody faced."""
    return {"batter": "A1", "bowler": "B1", "runs": {"batter": 0, "extras": 1, "total": 1}, "extras": {"wides": 1}}


def test_the_archive_path_counts_a_no_ball_faced_and_a_wide_not() -> None:
    """The no-ball is faced and the wide is not; a delivery with no ``extras`` object is
    faced."""
    from ml.xi.sources import _deliveries_from_cricsheet

    plain_four = {"batter": "A1", "bowler": "B1", "runs": {"batter": 4, "extras": 0, "total": 4}}
    innings = [{"overs": [{"over": 0, "deliveries": [_no_ball(), _wide(), plain_four]}]}]

    d = _deliveries_from_cricsheet(innings, {})

    assert list(d.faced) == [1.0, 0.0, 1.0]


def test_the_postgres_path_counts_a_no_ball_faced_and_a_wide_not() -> None:
    """The same deliveries read from ``ball_event`` with ``extras_wides`` (migration
    0018): the no-ball is faced and the wide is not, exactly as the archive path reads
    them."""
    squad = [(f"a{i:07x}", 10, False) for i in range(11)] + [(f"b{i:07x}", 20, False) for i in range(11)]
    no_ball = (1, 0, "a0000000", "b0000000", 0, 1, None, None, None, 0, 0, 0, 0)
    wide = (1, 0, "a0000000", "b0000000", 0, 1, None, None, None, 0, 0, 0, 1)
    plain_four = (1, 0, "a0000000", "b0000000", 4, 4, None, None, None, 0, 0, 0, 0)
    tables = {
        "matches": [(1, date(2024, 1, 1), "T20I", "male", 5, 10, 20, 20, "", None, "", "", None, 10)],
        "players": {1: squad},
        "balls": {1: [no_ball, wide, plain_four]},
    }

    (record,) = PostgresSource(_FakeConnection(tables), formats=["T20I"]).iter_matches()

    assert list(record.deliveries.faced) == [1.0, 0.0, 1.0]


def test_postgres_balls_read_the_wides_and_not_is_legal() -> None:
    """Whether the batter faced the ball is ``extras_wides``, read from the row itself:
    ``is_legal`` is the bowler's count and would say a no-ball was not faced."""
    from ml.xi.sources import _BALLS_SQL

    assert "be.extras_wides" in _BALLS_SQL
    assert "is_legal" not in _BALLS_SQL


def test_simulate_reports_the_same_p_bats_as_performance_predict_for_the_same_eleven(registry, artifacts_dir) -> None:
    """SERVE-05 / H-8: two surfaces, one question, one number. The simulator's ``p_bats``
    and ``p_bowls`` are the classifier's forecasts the draws were made from, so they equal
    ``/performance/predict``'s for the same eleven and toss -- known and marginalised --
    and the share of draws that realised them is served as ``batted_share`` /
    ``bowled_share``, not under the forecast's name."""
    base = _eleven_against_eleven(artifacts_dir)
    for toss in (True, None):
        request = SimulateRequest(**base, team1_bats_first=toss, n_samples=301, seed=2)

        simulated = xi_service.simulate(request, registry)
        predicted = xi_service.predict_performance(PerformancePredictRequest(**request.model_dump()), registry)

        forecast = {(p.side, p.player_id): p for p in predicted.players}
        for player in simulated.team1.players + simulated.team2.players:
            expected = forecast[(player.side, player.player_id)]
            assert player.p_bats == pytest.approx(expected.p_bats, abs=1e-12)
            assert player.p_bowls == pytest.approx(expected.p_bowls, abs=1e-12)
            assert 0.0 <= player.batted_share <= 1.0 and 0.0 <= player.bowled_share <= 1.0
