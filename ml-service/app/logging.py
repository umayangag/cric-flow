import logging
import os
import sys
from contextvars import ContextVar
from typing import Optional

import structlog

# Context variables
request_id_var: ContextVar[Optional[str]] = ContextVar("request_id", default=None)
service_var: ContextVar[str] = ContextVar("service", default="ml-service")
version_var: ContextVar[Optional[str]] = ContextVar("version", default=None)

# Keep a stdlib logger singleton for backward compatibility with existing tests
_std_logger_singleton: Optional[logging.Logger] = None


def init_logging(service: str = "ml-service", version: Optional[str] = None) -> None:
    """Initialize structlog + stdlib logging.

    Respects environment variables:
      - LOG_LEVEL: DEBUG|INFO|WARNING|ERROR (default INFO)
      - LOG_FORMAT: json|console (default json)
    """
    global _std_logger_singleton

    level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    log_format = os.environ.get("LOG_FORMAT", "json").lower()

    # Set context defaults
    service_var.set(service)
    version_var.set(version)

    # Configure stdlib root logger
    root = logging.getLogger()
    root.setLevel(level)
    # Replace handlers
    handler = logging.StreamHandler(stream=sys.stdout)

    if log_format == "console":
        formatter = structlog.stdlib.ProcessorFormatter(
            foreign_pre_chain=[
                structlog.contextvars.merge_contextvars,
                structlog.processors.add_log_level,
                structlog.processors.TimeStamper(fmt="iso"),
            ],
            processors=[structlog.stdlib.ProcessorFormatter.remove_processors_meta, structlog.dev.ConsoleRenderer()],
        )
    else:
        formatter = structlog.stdlib.ProcessorFormatter(
            foreign_pre_chain=[
                structlog.contextvars.merge_contextvars,
                structlog.processors.add_log_level,
                structlog.processors.TimeStamper(fmt="iso"),
            ],
            processors=[
                structlog.stdlib.ProcessorFormatter.remove_processors_meta,
                structlog.processors.JSONRenderer(sort_keys=True),
            ],
        )
    handler.setFormatter(formatter)
    root.handlers = [handler]

    # Configure structlog
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.stdlib.ProcessorFormatter.wrap_for_formatter,
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=True,
    )

    # Initialize std logger singleton
    if _std_logger_singleton is None:
        _std_logger_singleton = logging.getLogger(service)
        # Ensure at least one handler (root handler is already set)
        if not _std_logger_singleton.handlers:
            _std_logger_singleton.addHandler(handler)
        _std_logger_singleton.setLevel(level)


def bind_request_context(request_id: Optional[str]) -> None:
    request_id_var.set(request_id)
    structlog.contextvars.clear_contextvars()
    # Seed common fields for this context
    structlog.contextvars.bind_contextvars(
        request_id=request_id,
        service=service_var.get(),
        version=version_var.get(),
    )


def get_struct_logger() -> structlog.stdlib.BoundLogger:
    """Preferred structured logger."""
    return structlog.get_logger()


def get_logger() -> logging.Logger:
    """Backward-compatible stdlib logger singleton used in older code/tests.

    Note: structlog is initialized via init_logging(); this function returns the
    named stdlib logger which routes through the configured ProcessorFormatter.
    """
    # global _std_logger_singleton
    if _std_logger_singleton is None:
        init_logging()
    assert _std_logger_singleton is not None
    return _std_logger_singleton
