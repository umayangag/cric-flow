import logging
from typing import Optional

_logger: Optional[logging.Logger] = None


def get_logger() -> logging.Logger:
    global _logger
    if _logger is None:
        logger = logging.getLogger("ml-service")
        if not logger.handlers:
            handler = logging.StreamHandler()
            fmt = logging.Formatter(
                fmt="%(asctime)s %(levelname)s [%(name)s] %(message)s",
                datefmt="%Y-%m-%dT%H:%M:%S",
            )
            handler.setFormatter(fmt)
            logger.addHandler(handler)
        logger.setLevel(logging.INFO)
        _logger = logger
    return _logger
