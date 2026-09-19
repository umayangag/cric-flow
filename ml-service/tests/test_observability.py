import importlib
import os

from fastapi.testclient import TestClient

from tests.xi_fixtures import xi


def _client(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    return TestClient(app_module.app)


def test_x_request_id_echo_and_health(tmp_path):
    client = _client(tmp_path)
    rid = "test-rid-123"
    resp = client.get("/health", headers={"X-Request-ID": rid})
    assert resp.status_code == 200
    # Echo header
    assert resp.headers.get("X-Request-ID") == rid
    # Basic shape
    body = resp.json()
    assert body.get("status") == "ok"


def test_error_payload_contains_request_id_on_400(tmp_path):
    """A refused request carries the request id, so a 4xx in the log and the one the
    caller saw are the same event."""
    client = _client(tmp_path)
    rid = "req-400"
    # An XI optimisation for a format whose objective does not rank (H-17) is refused.
    resp = client.post(
        "/xi/optimize",
        headers={"X-Request-ID": rid},
        json={"format": "TEST", "pool_player_ids": xi("a"), "opponent_player_ids": xi("b")},
    )
    assert resp.status_code == 503
    detail = resp.json()["detail"]
    assert detail["code"] == "XI_MODEL_UNAVAILABLE"
    assert detail.get("request_id") == rid


def test_error_payload_contains_request_id_on_500(tmp_path):
    """An unhandled failure is still a payload with the request id in it, not a bare 500."""
    client = _client(tmp_path)
    rid = "req-500"

    from app import xi_service

    def boom(*_args, **_kwargs):
        raise RuntimeError("boom")

    original = xi_service.status
    xi_service.status = boom
    try:
        resp = client.get("/xi/status", headers={"X-Request-ID": rid})
    finally:
        xi_service.status = original

    assert resp.status_code == 500
    detail = resp.json()["detail"]
    assert detail["code"] == "UNHANDLED_EXCEPTION"
    assert detail.get("request_id") == rid
