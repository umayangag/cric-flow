from typing import List, Optional

from pydantic import BaseModel


class ErrorDetail(BaseModel):
    code: str
    message: str
    hint: Optional[str] = None
    available_formats: Optional[List[str]] = None


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
    return payload
