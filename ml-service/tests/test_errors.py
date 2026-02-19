"""Unit tests for app.errors (error_payload, ErrorDetail)."""

from app.errors import ErrorDetail, error_payload


def test_error_payload_minimal():
    """error_payload with only code and message."""
    p = error_payload("ERR", "Something failed")
    assert p["code"] == "ERR"
    assert p["message"] == "Something failed"
    assert "hint" not in p
    assert "available_formats" not in p


def test_error_payload_with_hint():
    """error_payload includes hint when provided."""
    p = error_payload("ERR", "msg", hint="Try again")
    assert p["hint"] == "Try again"


def test_error_payload_available_formats_sorted_and_no_legacy():
    """available_formats is sorted and excludes _LEGACY_."""
    p = error_payload("ERR", "msg", available=["T20", "_LEGACY_", "ODI"])
    assert p["available_formats"] == ["ODI", "T20"]


def test_error_payload_request_id_from_context_var():
    """When request_id_var has value, it appears in payload."""
    from app.logging import request_id_var

    token = request_id_var.set("req-123")
    try:
        p = error_payload("ERR", "msg")
        assert p.get("request_id") == "req-123"
    finally:
        request_id_var.reset(token)


def test_error_payload_request_id_empty_no_crash():
    """When request_id_var is empty, error_payload still returns valid payload without crashing."""
    from app.logging import request_id_var

    token = request_id_var.set("")
    try:
        p = error_payload("ERR", "msg")
        assert p["code"] == "ERR"
        assert p["message"] == "msg"
    finally:
        request_id_var.reset(token)


def test_error_detail_model():
    """ErrorDetail Pydantic model accepts optional fields."""
    e = ErrorDetail(code="X", message="Y")
    assert e.code == "X"
    assert e.message == "Y"
    assert e.hint is None
    e2 = ErrorDetail(code="A", message="B", hint="H", available_formats=["T20"])
    assert e2.hint == "H"
    assert e2.available_formats == ["T20"]
