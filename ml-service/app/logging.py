import logging
import os
import sys
from contextvars import ContextVar
from typing import Optional

import structlog

# ANSI escape codes for coloring error/warn lines in docker logs and terminals
_ANSI_RED = "\033[31m"
_ANSI_YELLOW = "\033[33m"
_ANSI_RESET = "\033[0m"

# Context variables
request_id_var: ContextVar[Optional[str]] = ContextVar("request_id", default=None)
service_var: ContextVar[str] = ContextVar("service", default="ml-service")
version_var: ContextVar[Optional[str]] = ContextVar("version", default=None)

# Keep a stdlib logger singleton for backward compatibility with existing tests
_std_logger_singleton: Optional[logging.Logger] = None


class _ColorStreamHandler(logging.StreamHandler):
    """StreamHandler that wraps error lines in red and warning lines in yellow when LOG_COLOR=1."""

    def __init__(self, stream, formatter: logging.Formatter, use_color: bool) -> None:
        super().__init__(stream)
        self._formatter = formatter
        self._use_color = use_color

    def format(self, record: logging.LogRecord) -> str:
        msg = self._formatter.format(record)
        if not self._use_color:
            return msg
        if record.levelno >= logging.ERROR:
            return f"{_ANSI_RED}{msg}{_ANSI_RESET}"
        if record.levelno >= logging.WARNING:
            return f"{_ANSI_YELLOW}{msg}{_ANSI_RESET}"
        return msg


def init_logging(service: str = "ml-service", version: Optional[str] = None) -> None:
    """Initialize structlog + stdlib logging.

    Respects environment variables:
      - LOG_LEVEL: DEBUG|INFO|WARNING|ERROR (default INFO)
      - LOG_FORMAT: json|console (default json)
      - LOG_COLOR: 1 to enable ANSI color for error (red) and warn (yellow) in docker/terminal logs
    """
    global _std_logger_singleton

    level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    log_format = os.environ.get("LOG_FORMAT", "json").lower()
    use_color = os.environ.get("LOG_COLOR", "").strip() == "1"

    # Set context defaults
    service_var.set(service)
    version_var.set(version)

    # Configure stdlib root logger
    root = logging.getLogger()
    root.setLevel(level)

    if log_format == "console":
        renderer = structlog.dev.ConsoleRenderer(
            colors=use_color,
            force_colors=use_color,  # so docker logs (non-TTY) still get colors
        )
        formatter = structlog.stdlib.ProcessorFormatter(
            foreign_pre_chain=[
                structlog.contextvars.merge_contextvars,
                structlog.processors.add_log_level,
                structlog.processors.TimeStamper(fmt="iso"),
            ],
            processors=[structlog.stdlib.ProcessorFormatter.remove_processors_meta, renderer],
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

    if use_color and log_format == "json":
        handler = _ColorStreamHandler(sys.stdout, formatter, use_color=True)
    else:
        handler = logging.StreamHandler(stream=sys.stdout)
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
    if _std_logger_singleton is None:
        init_logging()
    assert _std_logger_singleton is not None
    return _std_logger_singleton
