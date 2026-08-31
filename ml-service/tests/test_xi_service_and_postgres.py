"""app.xi_service (registry, optimize, predict) and the Postgres match source, without a
database or a running FastAPI app."""

from __future__ import annotations

import json
from datetime import date
from typing import List

import numpy as np
import pandas as pd
import pytest

from app import xi_service
from app.models.xi import XiConstraints, XiOptimizeRequest, XiWinRequest
from ml.xi.builder import build
from ml.xi.sources import BOWLER_CREDITED_KINDS, Deliveries, MatchRecord, PostgresSource, _deliveries_from_rows
from ml.xi.train import main as train_main
from ml.xi.train import train_all
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history


def _numeric_history(n: int = 160):
    """The synthetic history with integer-looking player keys, as the Postgres source yields."""
    matches, squad_a, squad_b = _synthetic_history(n)
    rename = {k: str(1000 + i) for i, k in enumerate(squad_a)}
    rename.update({k: str(2000 + i) for i, k in enumerate(squad_b)})

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
                m.format_code,
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
    return out, [int(rename[k]) for k in squad_a], [int(rename[k]) for k in squad_b]


@pytest.fixture(scope="module")
def artifacts_dir(tmp_path_factory) -> tuple:
    matches, squad_a, squad_b = _numeric_history()
    out = tmp_path_factory.mktemp("xi_service_artifacts")
    train_all(build(_ListSource(matches)), str(out), pd.Timestamp("2023-05-01"), formats=["T20"])
    return str(out), squad_a, squad_b, matches


@pytest.fixture
def registry(artifacts_dir) -> xi_service.XiRegistry:
    reg = xi_service.XiRegistry()
    reg.reload(artifacts_dir[0])
    return reg


def test_registry_reports_absent_artifacts_without_raising(tmp_path) -> None:
    reg = xi_service.XiRegistry()
    status = reg.reload(str(tmp_path))
    assert status["loaded"] is False
    with pytest.raises(xi_service.XiUnavailable):
        reg.store("T20")
    assert xi_service.loaded_formats(reg) == {"loaded_xi_formats": []}


def test_registry_survives_a_corrupt_artifact(tmp_path) -> None:
    (tmp_path / "xi_ratings.joblib").write_bytes(b"not a joblib file")
    reg = xi_service.XiRegistry()
    assert reg.reload(str(tmp_path))["loaded"] is False


def test_registry_status_after_load(registry, artifacts_dir) -> None:
    status = xi_service.status(registry)
    assert status.loaded and status.formats == ["T20"]
    assert status.players > 20
    assert status.ratings_through is not None
    assert status.report["formats"][0]["format_code"] == "T20"
    with pytest.raises(xi_service.XiUnavailable, match="ODI"):
        registry.store("ODI")


def test_optimize_returns_ids_marginals_and_unknowns(registry, artifacts_dir) -> None:
    _, squad_a, squad_b, matches = artifacts_dir
    opponent = [int(k) for k in matches[-1].team2_players]
    req = XiOptimizeRequest(
        format="t20",
        pool_player_ids=squad_a + [99999],
        opponent_player_ids=opponent,
        constraints=XiConstraints(team_size=11, min_bowlers=3, require_keeper=False, must_exclude=[squad_a[0]]),
    )
    res = xi_service.optimize(req, registry)
    assert len(res.selected_player_ids) == 11
    assert squad_a[0] not in res.selected_player_ids
    assert res.unknown_player_ids == [99999]
    assert set(res.marginal_values) == set(res.selected_player_ids)
    assert 0.0 <= res.win_probability <= 1.0


def test_optimize_infeasible_constraints_raise_value_error(registry, artifacts_dir) -> None:
    _, squad_a, _, matches = artifacts_dir
    req = XiOptimizeRequest(
        format="T20", pool_player_ids=squad_a[:5], opponent_player_ids=[int(k) for k in matches[-1].team2_players]
    )
    with pytest.raises(ValueError):
        xi_service.optimize(req, registry)


def test_predict_win_with_and_without_context(registry, artifacts_dir) -> None:
    _, _, _, matches = artifacts_dir
    last = matches[-1]
    t1, t2 = [int(k) for k in last.team1_players], [int(k) for k in last.team2_players]
    plain = xi_service.predict_win(XiWinRequest(format="T20", team1_player_ids=t1, team2_player_ids=t2), registry)
    ctx = xi_service.predict_win(
        XiWinRequest(format="T20", team1_player_ids=t1, team2_player_ids=t2, team1_id=7, team2_id=8, venue_id=3),
        registry,
    )
    for r in (plain, ctx):
        assert 0.0 <= r.team1_win_probability <= 1.0
        assert 0.0 <= r.objective_probability <= 1.0
    assert plain.objective_probability == pytest.approx(ctx.objective_probability), "context never enters the objective"


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
        if "FROM match m" in sql:
            self.rows = self.tables["matches"]
        elif "FROM match_player" in sql:
            self.rows = self.tables["players"].get(params[0], [])
        elif "FROM ball_event" in sql:
            self.rows = self.tables["balls"].get(params[0], [])
        else:
            raise AssertionError(sql)

    def fetchall(self):
        return self.rows


class _FakeConnection:
    def __init__(self, tables):
        self.tables = tables

    def cursor(self):
        return _FakeCursor(self.tables)


def test_postgres_source_maps_rows_and_skips_sides_without_squads() -> None:
    tables = {
        "matches": [
            (1, date(2024, 1, 1), "T20", "male", 5, 10, 20, 20),
            (2, date(2024, 1, 2), "T20", "male", 5, 10, 20, None),
            (3, date(2024, 1, 3), "T20", "male", 5, 10, 20, 10),
        ],
        # Player columns are keys, not ids: the query resolves player.external_id (P-1).
        "players": {
            1: [(f"a{i:07x}", 10) for i in range(11)] + [(f"b{i:07x}", 20) for i in range(11)],
            2: [(f"a{i:07x}", 10) for i in range(11)],  # side 20 has no squad -> skipped
            3: [(f"a{i:07x}", 10) for i in range(11)] + [(f"b{i:07x}", 20) for i in range(11)],
        },
        "balls": {
            1: [
                (1, 0, "a0000000", "b0000000", 4, 4, None, None),
                (1, 0, "a0000000", "b0000000", 0, 0, "caught", ["b0000005"]),
                (2, 0, "b0000000", "a0000000", 0, 1, "run out", ["a0000005"]),
            ],
            3: [(1, 3, "a0000001", "b0000000", 1, 1, None, None)],
        },
    }
    recs = list(PostgresSource(_FakeConnection(tables), formats=["T20"]).iter_matches())

    assert [r.match_id for r in recs] == ["1", "3"]
    first = recs[0]
    assert first.team1 == "10" and first.team2 == "20" and first.winner == "20" and first.outcome == 0.0
    assert first.team1_players[0] == "a0000000" and len(first.team2_players) == 11
    d = first.deliveries
    assert list(d.innings) == [0, 0, 1]
    assert list(d.bowler_wicket) == [0.0, 1.0, 0.0]
    assert list(d.wicket) == [0.0, 1.0, 1.0]
    assert d.fielders[1] == ["b0000005"]
    assert recs[1].outcome == 1.0
    assert "caught" in BOWLER_CREDITED_KINDS and "run out" not in BOWLER_CREDITED_KINDS


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
    d = _deliveries_from_rows([(1, 0, None, None, 0, 0, None, None)])

    assert list(d.batter) == [""] and list(d.bowler) == [""]
    assert d.fielders == [[]]


def test_deliveries_from_rows_handles_empty() -> None:
    assert len(_deliveries_from_rows([])) == 0


def test_train_cli_runs_on_a_tiny_cricsheet_directory(tmp_path) -> None:
    """The CLI end to end on two files: every format is skipped for lack of rows, the report
    and the rating state are still written."""
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    src = tmp_path / "json"
    src.mkdir()
    players = {"X": [f"X{i}" for i in range(11)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "a.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], players, "X", 1)))
    (src / "b.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], players, "Y", 2)))
    out = tmp_path / "artifacts"
    frame_out = tmp_path / "frame.csv"

    rc = train_main(
        ["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out), "--frame-out", str(frame_out)]
    )

    assert rc == 0
    report = json.loads((out / "xi_win_report.json").read_text())
    assert report["n_rows"] == 2
    assert all("skipped_reason" in f for f in report["formats"])
    assert (out / "xi_ratings.joblib").exists()
    assert frame_out.exists()


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
