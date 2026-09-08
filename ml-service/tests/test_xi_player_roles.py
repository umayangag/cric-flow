"""`POST /xi/player-roles`: the two role predicates read for an arbitrary list of ids (P3-1).

The point of the route is that it answers for players who are in no eleven — an auction
list spans clubs and holds nobody the search has picked — and the point of these tests is
that answering for them does not become a *second* definition of a role. The first test
pins this route's answer equal to `selection_reasons`', player for player, on the same run.
"""

from __future__ import annotations

import pandas as pd
import pytest

from app import xi_service
from app.models.xi import PlayerRolesRequest
from ml.xi.builder import build
from ml.xi.optimizer import Constraints, select_xi_by_ratings, selection_reasons
from ml.xi.retrain import retrain
from ml.xi.roles import SELECTION_ROLES
from tests.test_xi_optimizer_and_store import _ListSource
from tests.test_xi_service_and_postgres import _registry_keyed_history
from tests.xi_perf_fixtures import fast_fits


@pytest.fixture(autouse=True)
def _staleness_off(monkeypatch) -> None:
    """The fixture history is synthetic and dated 2023, so every state in it is stale.
    H-11 is exercised by name in its own test below; elsewhere it is turned off so these
    tests measure what they say they measure."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "0")


@pytest.fixture(scope="module")
def artifacts_dir(tmp_path_factory) -> tuple:
    """One run written the way ``retrain`` writes one, with registry-style player ids."""
    matches, squad_a, squad_b = _registry_keyed_history()
    out = tmp_path_factory.mktemp("player_roles_artifacts")
    with fast_fits():
        retrain(build(_ListSource(matches)), str(out), pd.Timestamp("2023-05-01"), formats=["T20I"])
    return str(out), squad_a, squad_b


@pytest.fixture
def registry(artifacts_dir) -> xi_service.XiRegistry:
    reg = xi_service.XiRegistry()
    reg.reload(artifacts_dir[0])
    return reg


def test_player_roles_equal_the_selections_own_reasons_on_the_same_ids(registry, artifacts_dir) -> None:
    """The rule the extraction exists for: the role this route reports for a player is the
    role the selection reports for the same player, on the same run, exactly."""
    _, squad_a, _ = artifacts_dir
    store = registry.store("T20I")
    constraints = Constraints(team_size=11, min_bowlers=3, require_keeper=False)
    selected = select_xi_by_ratings(store, "T20I", squad_a, constraints)
    reasons = selection_reasons(store, "T20I", selected, squad_a, constraints=constraints)

    answer = xi_service.player_roles(PlayerRolesRequest(format="T20I", player_ids=selected), registry)

    roles_by_id = {player.player_id: player.roles for player in answer.players}
    assert set(roles_by_id) == set(selected)
    for key in selected:
        assert roles_by_id[key] == reasons[key].roles, f"the two readers disagree about {key}"


def test_player_roles_name_only_the_two_predicates_the_objective_reads(registry, artifacts_dir) -> None:
    """No third role is invented for a pool the selection never saw."""
    _, squad_a, _ = artifacts_dir

    answer = xi_service.player_roles(PlayerRolesRequest(format="T20I", player_ids=squad_a), registry)

    assert [player.player_id for player in answer.players] == squad_a
    for player in answer.players:
        assert set(player.roles) <= set(SELECTION_ROLES)
        assert player.known is True


def test_player_roles_report_an_id_the_state_has_never_seen_as_unknown(registry, artifacts_dir) -> None:
    """§8.7: an unknown player is reported unknown, with no role invented for him — which
    is a different answer from "he neither keeps nor bowls"."""
    _, squad_a, _ = artifacts_dir
    asked = [squad_a[0], "ffffffff", squad_a[1]]

    answer = xi_service.player_roles(PlayerRolesRequest(format="T20I", player_ids=asked), registry)

    assert answer.unknown_player_ids == ["ffffffff"]
    unknown = next(player for player in answer.players if player.player_id == "ffffffff")
    assert unknown.known is False
    assert unknown.roles == []
    assert [player.known for player in answer.players] == [True, False, True]


def test_player_roles_carry_the_run_and_date_that_served_them(registry, artifacts_dir) -> None:
    """P1-5: the stamp comes off the store that answered, not off a later status read."""
    _, squad_a, _ = artifacts_dir

    answer = xi_service.player_roles(PlayerRolesRequest(format="T20I", player_ids=squad_a[:3]), registry)

    assert answer.format == "T20I"
    assert answer.served_ratings.run_id == registry.status().run_id
    assert answer.served_ratings.ratings_through == registry.status().ratings.ratings_through


def test_player_roles_are_refused_on_a_stale_registry(registry, artifacts_dir, monkeypatch) -> None:
    """H-11: a role read is a live request, so past the limit it is refused by name rather
    than answered from ratings that have moved on."""
    _, squad_a, _ = artifacts_dir
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "1")

    with pytest.raises(xi_service.RatingsStale) as raised:
        xi_service.player_roles(PlayerRolesRequest(format="T20I", player_ids=squad_a[:3]), registry)

    assert raised.value.payload["code"] == "RATINGS_STALE"


def test_player_roles_refuse_a_format_the_run_does_not_serve(registry, artifacts_dir) -> None:
    """A format with no loaded model is refused rather than answered from another one's."""
    _, squad_a, _ = artifacts_dir

    with pytest.raises(xi_service.XiUnavailable):
        xi_service.player_roles(PlayerRolesRequest(format="ODI", player_ids=squad_a[:3]), registry)
