"""
End-to-end tests against a live ML service (http://localhost:8000 by default).

Skip unless RUN_E2E=1. Used after `make e2e-backtest-smoke` or when services are up.
"""

import os

import httpx
import pytest


def _base_url():
    return os.environ.get("ML_SERVICE_URL", "http://localhost:8000")


def _e2e_enabled():
    return os.environ.get("RUN_E2E", "").strip() == "1"


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_health_live():
    """GET /health on live ML service returns 200 and status ok."""
    resp = httpx.get(f"{_base_url()}/health", timeout=5.0)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert body.get("status") == "ok"


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_health_artifacts_live():
    """GET /health on live ML service returns 200 and artifact keys."""
    resp = httpx.get(f"{_base_url()}/health", timeout=5.0)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "loaded_batting_formats" in body
    assert "loaded_bowling_formats" in body


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_backtest_predict_players_live():
    """POST /ml/backtest/predict with players + format + features returns 200 (or 503 if no models)."""
    payload = {
        "cutoff_date": "2024-10-30T14:00:00Z",
        "player_ids": [1, 2],
        "format": "T20",
        "features": {
            "1": {
                "batting_consistency": 0.5,
                "batting_form": 0.5,
                "venue": 0.5,
                "opposition": 0.5,
                "season": 2024,
            },
            "2": {
                "batting_consistency": 0.4,
                "batting_form": 0.4,
                "venue": 0.5,
                "opposition": 0.5,
                "season": 2024,
            },
        },
    }
    resp = httpx.post(
        f"{_base_url()}/ml/backtest/predict",
        json=payload,
        timeout=30.0,
    )
    # 200 with predictions, or 503/422 if service has no models or validation fails
    assert resp.status_code in (200, 422, 503), (resp.status_code, resp.text)


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_backtest_predict_match_baseline_live():
    """POST /ml/backtest/predict with teams only returns match baseline (no ML)."""
    payload = {
        "cutoff_date": "2024-10-30T14:00:00Z",
        "teams": ["IND", "AUS"],
    }
    resp = httpx.post(
        f"{_base_url()}/ml/backtest/predict",
        json=payload,
        timeout=10.0,
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "runs" in body or "winner_team_code" in body or "match" in body


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_artifacts_status_live():
    """GET /artifacts/status returns 200 with formats and legacy structure."""
    resp = httpx.get(f"{_base_url()}/artifacts/status", timeout=5.0)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "runs" in body
    assert "current_run" in body
    assert "loaded_run" in body
    assert "timestamp" in body


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_xi_status_live():
    """GET /xi/status returns 200 and names the run it is serving, if any (H-16)."""
    resp = httpx.get(f"{_base_url()}/xi/status", timeout=5.0)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "loaded" in body
    assert "run_id" in body
    assert "ratings" in body


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_openapi_schema_live():
    """GET /openapi.json returns 200 with valid OpenAPI structure."""
    resp = httpx.get(f"{_base_url()}/openapi.json", timeout=5.0)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "openapi" in body
    assert "paths" in body
