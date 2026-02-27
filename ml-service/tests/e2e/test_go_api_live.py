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
    """GET /api/backtest/accuracy-trend returns 200 (may have empty data)."""
    resp = httpx.get(
        f"{_go_api_url()}/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS",
        headers=_api_headers(),
        timeout=10.0,
    )
    assert resp.status_code == 200, resp.text
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
