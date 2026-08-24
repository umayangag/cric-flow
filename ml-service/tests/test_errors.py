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


def test_error_payload_request_id_structlog_get_raises():
    """When get_contextvars() raises, error_payload still returns valid payload (lines 34-35)."""
    from unittest.mock import patch

    from app.logging import request_id_var

    token = request_id_var.set("")
    try:
        with patch("structlog.contextvars.get_contextvars", side_effect=Exception("structlog nope")):
            p = error_payload("ERR", "msg")
            assert p["code"] == "ERR"
            assert p["message"] == "msg"
            assert "request_id" not in p
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


def test_error_payload_table_driven():
    """Table-driven tests for error_payload code/message/hint/available combinations."""
    cases = [
        {"code": "X", "message": "Y", "hint": None, "available": None},
        {"code": "INVALID_FORMAT", "message": "Bad format", "hint": "Use T20 or ODI", "available": None},
        {"code": "ERR", "message": "msg", "hint": None, "available": []},
        {"code": "ERR", "message": "msg", "hint": None, "available": ["T20", "ODI"]},
        {"code": "ERR", "message": "msg", "hint": None, "available": ["T20I", "TEST", "ODI"]},
    ]
    for c in cases:
        p = error_payload(c["code"], c["message"], hint=c["hint"], available=c["available"])
        assert p["code"] == c["code"]
        assert p["message"] == c["message"]
        if c["hint"] is not None:
            assert p["hint"] == c["hint"]
        else:
            assert "hint" not in p or p.get("hint") is None
        if c["available"] is not None:
            expected = sorted(c["available"])
            assert p.get("available_formats") == expected
        else:
            assert "available_formats" not in p


def test_error_detail_model_table_driven():
    """Table-driven tests for ErrorDetail model field combinations."""
    cases = [
        ({"code": "X", "message": "Y"}, {"code": "X", "message": "Y", "hint": None, "available_formats": None}),
        (
            {"code": "A", "message": "B", "hint": "H"},
            {"code": "A", "message": "B", "hint": "H", "available_formats": None},
        ),
        (
            {"code": "C", "message": "D", "available_formats": ["T20", "ODI"]},
            {"code": "C", "message": "D", "hint": None, "available_formats": ["T20", "ODI"]},
        ),
        (
            {"code": "E", "message": "F", "hint": "h", "available_formats": ["TEST"]},
            {"code": "E", "message": "F", "hint": "h", "available_formats": ["TEST"]},
        ),
    ]
    for kwargs, expected in cases:
        e = ErrorDetail(**kwargs)
        assert e.code == expected["code"]
        assert e.message == expected["message"]
        assert e.hint == expected["hint"]
        assert e.available_formats == expected["available_formats"]
