from typing import List, Optional

from pydantic import BaseModel

from logging import request_id_var


class ErrorDetail(BaseModel):
    code: str
    message: str
    hint: Optional[str] = None
    available_formats: Optional[List[str]] = None
    request_id: Optional[str] = None


def error_payload(
    code: str,
    message: str,
    hint: Optional[str] = None,
    available: Optional[List[str]] = None,
) -> dict:
    payload = {"code": code, "message": message}
    if hint:
        payload["hint"] = hint
    if available is not None:
        payload["available_formats"] = sorted([x for x in available if x != "_LEGACY_"])
    # Prefer ContextVar, but fall back to structlog context if needed
    rid = request_id_var.get()
    if not rid:
        try:
            from structlog.contextvars import get_contextvars  # lazy import

            rid = get_contextvars().get("request_id")  # type: ignore[assignment]
        except Exception:
            rid = None
    if rid:
        payload["request_id"] = rid
    return payload
