"""API tests for main.py's routes (TestClient) on a box with nothing trained.

An empty artifacts root is a real state -- a fresh deployment is in it -- and every one
of these routes has to answer it rather than fail on it.
"""

import importlib
import os

from fastapi.testclient import TestClient


def _app_client(tmp_path):
    """Load app with ML_SERVICE_OUTPUT_DIR set; return (app_module, client)."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)
    return app_module, TestClient(app_module.app)


def test_health_reports_that_nothing_is_loaded(tmp_path):
    _, client = _app_client(tmp_path)

    data = client.get("/health").json()

    assert data["status"] == "ok", "a process with no model is alive, not unhealthy"
    assert data["loaded"] is False
    assert data["run_id"] is None
    assert data["ratings"]["fresh"] is False, "no ratings is not fresh ratings"
    assert data["ratings"]["age_days"] is None, "and it has no age, rather than an invented one"


def test_artifacts_status_lists_no_runs(tmp_path):
    _, client = _app_client(tmp_path)

    data = client.get("/artifacts/status").json()

    assert data["root"] == str(tmp_path)
    assert data["runs"] == []
    assert data["current_run"] is None
    assert data["loaded_run"] is None


def test_evaluate_report_says_where_it_looked(tmp_path):
    """503 with the path, not a 500: the report is produced by a step the operator runs."""
    _, client = _app_client(tmp_path)

    resp = client.get("/xi/evaluate-report")

    assert resp.status_code == 503
    assert "xi_evaluate_report.json" in resp.json()["detail"]["message"]


def test_predict_win_without_a_model_is_unavailable_not_a_crash(tmp_path):
    _, client = _app_client(tmp_path)

    resp = client.post("/xi/predict-win", json={"format": "T20", "team1_player_ids": ["a"], "team2_player_ids": ["b"]})

    assert resp.status_code == 503
    assert resp.json()["detail"]["code"] == "XI_MODEL_UNAVAILABLE"
