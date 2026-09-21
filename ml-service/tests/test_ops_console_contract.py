"""ml-service's half of the go-app <-> ml-service contract (H-24).

``contracts/ops-console.contract.json`` is generated from go-app's pipeline registry and
declares every literal that crosses the boundary: the wire format of the training cutoff,
the admin endpoints go-app posts to with the query parameters it names, and the format
vocabulary both services match on. go-app asserts its behaviour against that file; these
are the assertions from this side.

They exist because of D-9. go-app formatted the cutoff as RFC3339, this service parsed it
as a date, and each side's tests agreed with its own component -- so the console's Retrain
button failed on the subprocess's first line of work with every gate green. A literal on a
service boundary is declared once and checked from both ends, or it is not checked at all.
"""

from __future__ import annotations

import importlib
import json
import re
from datetime import date, datetime, timedelta, timezone
from pathlib import Path
from types import SimpleNamespace
from typing import Any, Dict, List, Optional

import pytest
from fastapi.testclient import TestClient

from app import training_orchestrator, xi_service
from ml.xi import contract as C
from ml.xi import retrain
from ml.xi.ratings import RatingState
from ml.xi.roles import SELECTION_ROLES

CONTRACT_PATH = Path(__file__).resolve().parents[2] / "contracts" / "ops-console.contract.json"


@pytest.fixture(scope="module")
def contract() -> Dict[str, Any]:
    """The generated contract, read off disk the way the other side wrote it."""
    with open(CONTRACT_PATH) as fh:
        return json.load(fh)


@pytest.fixture
def client(tmp_path, monkeypatch) -> TestClient:
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path))
    monkeypatch.setenv("ENABLE_HOT_RELOAD", "1")
    monkeypatch.delenv("ADMIN_API_KEY", raising=False)
    module = importlib.reload(importlib.import_module("app.main"))
    return TestClient(module.app)


def go_app_default_cutoff() -> str:
    """The string go-app sends when the console's cutoff box is left empty.

    ``pipelinesvc.DefaultCutoff()`` is ``time.Now().UTC().Format(time.DateOnly)``; this is
    that value, computed the same way. go-app's own test asserts its default matches the
    contract's pattern, so the two ends meet on the contract rather than on this line.
    """
    return datetime.now(timezone.utc).strftime("%Y-%m-%d")


# --- the cutoff's wire format -------------------------------------------------------


def test_the_contract_example_matches_the_pattern_it_publishes(contract) -> None:
    """A contract whose own example fails its pattern would prove nothing on either side."""
    assert re.fullmatch(contract["cutoff"]["pattern"], contract["cutoff"]["example"])


def test_the_cutoff_parser_accepts_the_contract_format(contract) -> None:
    """The CLI accepts every value the contract says go-app may send."""
    parsed = retrain.parse_cutoff(contract["cutoff"]["example"])

    assert parsed.date().isoformat() == contract["cutoff"]["example"]


def test_the_cutoff_parser_accepts_go_apps_default(contract) -> None:
    default = go_app_default_cutoff()
    assert re.fullmatch(contract["cutoff"]["pattern"], default), "go-app's default must be in the wire format"

    assert retrain.parse_cutoff(default).date().isoformat() == default


@pytest.mark.parametrize(
    "value,expected",
    [
        ("2026-09-02", "2026-09-02"),
        ("2026-09-02T18:33:11Z", "2026-09-02"),  # the string D-9 died on
        ("2026-09-02T18:33:11+05:30", "2026-09-02"),
        ("2026-09-02 18:33:11", "2026-09-02"),
        ("  2026-09-02  ", "2026-09-02"),
    ],
)
def test_the_cutoff_parser_reads_a_timestamp_as_its_date(value: str, expected: str) -> None:
    """A cutoff is a date, so the time of day is truncated rather than refused: a
    hand-typed timestamp -- and any caller still formatting one -- cannot reproduce D-9."""
    assert retrain.parse_cutoff(value).date().isoformat() == expected


@pytest.mark.parametrize("value", ["", "not-a-date", "01-09-2025", "2025-13-40"])
def test_the_cutoff_parser_names_the_format_it_wanted(value: str) -> None:
    """Refusing is still the right answer for a value that is no date at all -- and the
    message says which formats would have worked, which the raw isoformat error did not."""
    with pytest.raises(ValueError, match="YYYY-MM-DD"):
        retrain.parse_cutoff(value)


# --- the admin surface go-app calls -------------------------------------------------


def test_every_declared_call_is_a_route_that_takes_those_query_parameters(contract, client) -> None:
    """Each endpoint go-app posts to exists here, by that method, with those parameters.

    Renaming a route or a query parameter on either side now fails a test instead of a
    run -- the same species of defect as D-9, one level up from the value.
    """
    schema = client.app.openapi()["paths"]

    for call in contract["ml_service_calls"]:
        operation = schema.get(call["path"], {}).get(call["method"].lower())
        assert operation is not None, f"go-app calls {call['method']} {call['path']}, which this service does not serve"
        accepted = {
            parameter["name"] for parameter in operation.get("parameters", []) if parameter.get("in") == "query"
        }
        missing = set(call["query"]) - accepted
        assert not missing, f"{call['path']} does not accept {sorted(missing)}"


# --- the format vocabulary ----------------------------------------------------------


def test_both_services_match_on_the_same_format_codes(contract) -> None:
    """go-app writes these codes into match rows; the rating pass switches on them when
    it reads them back. A code added on one side only is a format silently trained on
    nothing."""
    assert set(contract["format_codes"]) == set(C.FORMAT_CODES)


# --- the gender vocabulary ----------------------------------------------------------


def test_both_services_match_on_the_same_team_genders(contract) -> None:
    """go-app writes ``match.gender`` and now accepts a gender on the prediction request;
    this service matches on the literal to group E7's context baselines. A value spelled
    differently on one side is a match quietly read into the wrong baseline group (D-10)."""
    assert set(contract["team_genders"]) == set(C.TEAM_GENDERS)


def test_the_context_group_split_keys_on_a_gender_the_contract_publishes() -> None:
    """The E7 split is the one place this service compares a gender to a literal, so the
    literal it compares against has to be one go-app can actually send."""
    state = RatingState(gender_split_context=True)

    assert C.GENDER_FEMALE in C.TEAM_GENDERS
    assert state._ctx_group(C.GENDER_FEMALE) == 1
    assert state._ctx_group(C.GENDER_MALE) == 0


# --- the selection-role vocabulary (P1-3) -------------------------------------------


def test_both_services_match_on_the_same_selection_roles(contract) -> None:
    """This service computes the roles a "why this player" card names, go-app carries them
    and the card turns each into a chip. A role spelled differently on one side is a chip
    the UI cannot render for a constraint the objective really did read."""
    assert set(contract["selection_roles"]) == set(SELECTION_ROLES)


# --- the freshness vocabulary (P2-1) ------------------------------------------------


def _stale_verdict(days_old: int) -> Any:
    """A verdict on a run whose data boundary is `days_old` days back, as ``freshness()``
    builds one (SERVE-03)."""
    boundary = date.today() - timedelta(days=days_old)
    return xi_service.RatingsFreshness(
        fresh=False,
        data_age_days=days_old,
        max_age_days=14,
        data_through=boundary.isoformat(),
        ratings_through=boundary.isoformat(),
        code="RATINGS_STALE",
    )


def test_the_live_refusal_raises_the_code_the_contract_publishes(contract) -> None:
    """H-11's code originates here: this service computes the verdict and refuses the
    request. go-app copies the verdict onto /ops/status and the console keys its remedy
    off the literal, so a code spelled differently here is a refusal no surface explains."""
    refusal = xi_service.RatingsStale(_stale_verdict(40))

    assert refusal.payload["code"] == contract["ratings_stale_code"]
    assert "retrain" in refusal.payload["hint"]


def test_the_status_verdict_reports_the_same_published_code(contract, monkeypatch) -> None:
    """The refusal and the reported verdict are one computation (H-11), so the code on
    ``/xi/status`` is the code the request would be refused with -- and both are the
    contract's."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")
    registry = xi_service.XiRegistry()
    # The verdict reads two dates off the loaded store: the boundary its data was built to
    # (the manifest's cutoff, which it is taken on) and the last match it folded in. A stub
    # store is the smallest arrangement that puts both there without a run on disk.
    boundary = date.today() - timedelta(days=40)
    registry._served = xi_service.ServedRun(
        store=SimpleNamespace(
            state=SimpleNamespace(last_date=boundary),
            manifest=SimpleNamespace(cutoff=boundary.isoformat()),
        ),
        report=None,
        directory="",
    )

    verdict = registry.freshness()

    assert verdict.fresh is False
    assert verdict.code == contract["ratings_stale_code"]


def test_nothing_loaded_carries_no_code_and_no_invented_date(monkeypatch) -> None:
    """With nothing loaded there is no state to be stale: the request is refused for the
    other reason, and go-app spells that state `not_loaded` rather than `stale`."""
    monkeypatch.setenv("XI_RATINGS_MAX_AGE_DAYS", "14")

    verdict = xi_service.XiRegistry().freshness()

    assert verdict.fresh is False
    assert verdict.ratings_through is None
    assert verdict.code is None


# --- the regression seam D-9 needed -------------------------------------------------


@pytest.mark.parametrize(
    "cutoff",
    [
        pytest.param(go_app_default_cutoff(), id="go_apps_default"),
        pytest.param("2026-09-02T18:33:11Z", id="a_hand_typed_timestamp"),
        pytest.param("2025-09-01", id="a_typed_date"),
    ],
)
def test_retrain_endpoint_accepts_the_cutoff_go_app_sends(client, monkeypatch, cutoff: str) -> None:
    """POST /admin/train/retrain end to end, with the real argument parser.

    The subprocess is stubbed -- a retrain takes minutes -- but the stub runs
    ``ml.xi.retrain``'s own ``_parse_args`` and ``parse_cutoff`` over the arguments the
    orchestrator actually built. That is the seam D-9 crossed: every layer above and
    below it was tested, and the join was tested by nothing. Unskipped and in CI, because
    a regression test behind RUN_E2E=1 is a regression test that does not run.
    """
    parsed: Dict[str, Any] = {}

    def parsing_stub(
        module: str,
        extra_args: Optional[List[str]] = None,
        extra_env: Optional[Dict[str, str]] = None,
        logger: Optional[Any] = None,
    ) -> None:
        args = retrain._parse_args(extra_args or [])
        parsed["module"] = module
        parsed["cutoff"] = retrain.parse_cutoff(args.cutoff)

    monkeypatch.setattr(training_orchestrator, "run_training_subprocess", parsing_stub)

    resp = client.post(f"/admin/train/retrain?cutoff={cutoff}")

    assert resp.status_code == 200, resp.text
    assert parsed["module"] == "ml.xi.retrain"
    assert parsed["cutoff"].date().isoformat() == cutoff[:10]


def test_every_track_record_metric_key_has_a_glossary_entry(contract: Dict[str, Any]) -> None:
    """P2-4 / L-1: every number the track record labels must be explainable from the one
    glossary this service serves. go-app declares the keys it renders under; this is the
    completeness gate for a surface that never passes through the harness's report."""
    from ml.xi import glossary

    keys = contract["track_record_metric_keys"]

    assert keys, "the contract declares the track record's metric keys"
    assert glossary.check_metric_names(keys, "track record") == []
    assert all(key in glossary.REGISTRY for key in keys)


def test_every_auction_metric_key_has_a_glossary_entry(contract: Dict[str, Any]) -> None:
    """P3-1 / L-1: the Auction tab labels counts, not model measurements, but the rule is
    the same — every labelled number opens an explainer served from this one glossary. The
    band prose is where the counts say what they are not: not a selection, not a projection,
    and overlapping rather than a partition."""
    from ml.xi import glossary

    keys = contract["auction_metric_keys"]

    assert keys, "the contract declares the auction surface's metric keys"
    assert glossary.check_metric_names(keys, "auction") == []
    assert all(key in glossary.REGISTRY for key in keys)


def test_the_auction_roles_are_the_selections_own_and_not_a_new_vocabulary(contract: Dict[str, Any]) -> None:
    """The rule the item is built on: the auction module reads the objective's two role
    predicates and invents none beside them. A third role appearing in the contract's
    selection_roles because the auction wanted one would be exactly the drift
    ``ml.xi.roles`` was extracted to prevent."""
    assert contract["selection_roles"] == list(SELECTION_ROLES)


def test_the_auction_player_states_are_the_three_an_auction_has(contract: Dict[str, Any]) -> None:
    """H-24: the state vocabulary go-app writes, the database's CHECK constraint holds and
    the Auction tab renders. Asserted here so a fourth state cannot reach the wire without
    every side that matches on it being updated in the same commit."""
    assert contract["auction_player_states"] == ["available", "sold", "unsold"]
