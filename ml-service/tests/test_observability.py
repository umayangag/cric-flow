import importlib
import os

from fastapi.testclient import TestClient


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
    client = _client(tmp_path)
    rid = "req-400"
    # Trigger 400 by sending empty list
    resp = client.post("/predict/batting", headers={"X-Request-ID": rid}, json=[])
    assert resp.status_code == 400
    body = resp.json()
    assert "detail" in body
    detail = body["detail"]
    assert detail["code"] == "EMPTY_BATCH"
    assert detail.get("request_id") == rid


def test_error_payload_contains_request_id_on_500(tmp_path):
    # Force model predict to raise to produce 500
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    artifacts_module = importlib.import_module("app.artifacts")

    class BoomModel:
        def predict(self, X):  # noqa: N802
            raise RuntimeError("boom")

    class IdentityScaler:
        def transform(self, X):  # noqa: N802
            return X

    artifacts_module.BAT_MODELS.clear()
    artifacts_module.BAT_MODELS["ODI"] = (IdentityScaler(), BoomModel())

    client = TestClient(app_module.app)

    rid = "req-500"
    row = {
        "batting_consistency": 0.0,
        "batting_form": 0.0,
        "batting_temp": 0,
        "batting_wind": 0,
        "batting_rain": 0,
        "batting_humidity": 0,
        "batting_cloud": 0,
        "batting_pressure": 0,
        "batting_viscosity": 0,
        "batting_inning": 1,
        "batting_session": 1,
        "toss": 0,
        "venue": 0.0,
        "opposition": 0.0,
        "season": 0,
        "player_name": "P",
        "format": "ODI",
    }
    resp = client.post("/predict/batting", headers={"X-Request-ID": rid}, json=[row])
    assert resp.status_code == 500
    body = resp.json()
    assert "detail" in body
    detail = body["detail"]
    assert detail["code"] == "PREDICT_FAILED"
    assert detail.get("request_id") == rid
