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
def test_health_reports_the_loaded_run_live():
    """GET /health names the run being served and the ratings' freshness verdict."""
    resp = httpx.get(f"{_base_url()}/health", timeout=5.0)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "loaded" in body
    assert "run_id" in body
    assert "ratings" in body


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
