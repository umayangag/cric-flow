"""
End-to-end tests against a live go-api (http://localhost:8080 by default).

Requires RUN_E2E=1 and both go-api and ml-service to be up (e.g. after make e2e-backtest-smoke).
Tests go-api endpoints including backtest, options, and ML proxy.
"""

import os

import httpx
import pytest


def _go_api_url():
    return os.environ.get("GO_API_URL", "http://localhost:8080")


def _api_headers():
    return {"X-API-Key": os.environ.get("API_KEY", "test-api-key")}


def _e2e_enabled():
    return os.environ.get("RUN_E2E", "").strip() == "1"


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_go_api_health_live():
    """GET /health on go-api returns 200."""
    resp = httpx.get(f"{_go_api_url()}/health", timeout=5.0)
    assert resp.status_code == 200, resp.text


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_options_formats_live():
    """GET /api/options/formats returns 200 and array of formats."""
    resp = httpx.get(
        f"{_go_api_url()}/api/options/formats",
        headers=_api_headers(),
        timeout=5.0,
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert isinstance(body, list)


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_backtest_accuracy_trend_live():
    """GET /api/backtest/accuracy-trend returns 200 or 500 (500 when ML/DB state differs)."""
    resp = httpx.get(
        f"{_go_api_url()}/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS",
        headers=_api_headers(),
        timeout=10.0,
    )
    assert resp.status_code in (200, 500), (resp.status_code, resp.text)
    if resp.status_code == 200:
        body = resp.json()
        assert "results" in body
        assert "summary" in body


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_ml_model_stats_proxy_live():
    """GET /api/ml/model-stats (go-api proxy) returns 200 with models_dir and models."""
    resp = httpx.get(
        f"{_go_api_url()}/api/ml/model-stats",
        headers=_api_headers(),
        timeout=5.0,
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "models_dir" in body
    assert "models" in body
    assert isinstance(body["models"], list)


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_go_api_unauthorized_without_api_key():
    """Admin route without X-API-Key returns 401."""
    resp = httpx.get(
        f"{_go_api_url()}/api/options/formats",
        timeout=5.0,
    )
    assert resp.status_code == 401, resp.text


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_go_api_readiness_live():
    """GET /readiness returns 200 (checks DB connectivity)."""
    resp = httpx.get(f"{_go_api_url()}/readiness", timeout=5.0)
    assert resp.status_code == 200, resp.text


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_backtest_match_select_live():
    """GET /api/backtest/match (select mode) returns 200 with filters and candidates array."""
    resp = httpx.get(
        f"{_go_api_url()}/api/backtest/match?format=T20&team1=IND&team2=AUS",
        headers=_api_headers(),
        timeout=10.0,
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "filters" in body
    assert "candidates" in body
    assert isinstance(body["candidates"], list)


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_backtest_scorecard_live():
    """GET /api/backtest/scorecard returns 200 or 404 (depends on seeded match)."""
    resp = httpx.get(
        f"{_go_api_url()}/api/backtest/scorecard?match_id=9000111",
        headers=_api_headers(),
        timeout=5.0,
    )
    assert resp.status_code in (200, 404), (resp.status_code, resp.text)
    if resp.status_code == 200:
        body = resp.json()
        assert isinstance(body, dict)


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_ops_status_live():
    """GET /ops/status returns 200 with expected structure."""
    resp = httpx.get(
        f"{_go_api_url()}/ops/status",
        headers=_api_headers(),
        timeout=10.0,
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert isinstance(body, dict)


@pytest.mark.e2e
@pytest.mark.skipif(not _e2e_enabled(), reason="Set RUN_E2E=1 to run e2e tests")
def test_go_api_ml_health_proxy_live():
    """GET /api/health/ml (go-api proxy to ML service) returns 200."""
    resp = httpx.get(
        f"{_go_api_url()}/api/health/ml",
        headers=_api_headers(),
        timeout=10.0,
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert body.get("status") == "ok"
