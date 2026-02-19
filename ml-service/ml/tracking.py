import json
import logging
import os
import re
from contextlib import contextmanager
from typing import Optional

from dotenv import load_dotenv

from ml.db import get_db_connection

# Load environment variables from .env file if it exists
load_dotenv()

logger = logging.getLogger(__name__)

# Commands that form the main pipeline; only one may be IN_PROGRESS at a time (singleton).
# Must stay in sync with go-app/internal/pipeline/job.go PipelineCommands; apply changes in both.
# Future mitigation: shared config (JSON/YAML), API from go-app, or build-time generation. Until then, keep in sync manually.
PIPELINE_COMMANDS = (
    "cricsheet-import",
    "precompute-features",
    "export-dataset",
    "train-batting",
    "train-bowling",
    "train-fielding",
    "train-extras",
    "train-win",
)


def get_connection():
    return get_db_connection()


@contextmanager
def db_connection():
    """Context manager for DB connections; ensures close and logs close errors."""
    conn = None
    try:
        conn = get_connection()
        yield conn
    finally:
        if conn is not None:
            try:
                conn.close()
            except Exception as e:
                logger.warning("tracking.connection_close_failed", extra={"error": str(e)})


# Default age (seconds) for considering an IN_PROGRESS run stale on startup. Aligns with go-app.
DEFAULT_STALE_CANCEL_AGE_SECONDS = 24 * 60 * 60  # 24 hours


def _parse_go_duration(raw: str) -> Optional[int]:
    """Parse Go-style duration (e.g. 24h, 1h30m, 2h15m30s) into seconds.
    Aligns with Go time.ParseDuration: supports ns, us/µs, ms, s, m, h in any order.
    Returns None on parse failure or non-positive result."""
    raw = "".join(raw.strip().lower().split())
    if not raw:
        return None
    # Match sequences of number + unit (Go format)
    pattern = re.compile(r"(\d*\.?\d+)(ns|us|µs|ms|s|m|h)")
    total_ns = 0.0
    pos = 0
    for m in pattern.finditer(raw):
        if m.start() != pos:
            return None  # gap before match = invalid
        pos = m.end()
        num = float(m.group(1))
        unit = m.group(2)
        if unit == "ns":
            total_ns += num
        elif unit in ("us", "µs"):
            total_ns += num * 1000
        elif unit == "ms":
            total_ns += num * 1_000_000
        elif unit == "s":
            total_ns += num * 1_000_000_000
        elif unit == "m":
            total_ns += num * 60 * 1_000_000_000
        elif unit == "h":
            total_ns += num * 3600 * 1_000_000_000
    if pos != len(raw):
        return None  # trailing junk
    secs = int(total_ns / 1_000_000_000)
    if secs <= 0:
        return None
    return secs


def parse_stale_cancel_age_seconds() -> Optional[int]:
    """Parse TRACKING_STALE_CANCEL_AGE (e.g. 24h, 1h30m, 90s) or legacy TRACKING_STALE_CANCEL_AGE_MINUTES.
    Aligns with go-app time.ParseDuration; supports compound formats like 1h30m.
    Returns None to use default. For seconds ('s'), 0 or negative uses default."""
    raw = os.environ.get("TRACKING_STALE_CANCEL_AGE", "").strip()
    if raw:
        secs = _parse_go_duration(raw)
        if secs is not None:
            return secs
        logger.warning("tracking.invalid_TRACKING_STALE_CANCEL_AGE", extra={"value": raw})
        return None
    raw = os.environ.get("TRACKING_STALE_CANCEL_AGE_MINUTES", "").strip()
    if not raw:
        return None
    try:
        return int(raw) * 60  # legacy: minutes -> seconds
    except ValueError:
        logger.warning("tracking.invalid_TRACKING_STALE_CANCEL_AGE_MINUTES", extra={"value": raw})
        return None


def cancel_in_progress_on_startup(
    reason: str = "interrupted (ml-service restart or crash)",
    stale_seconds: Optional[int] = None,
) -> int:
    """Mark IN_PROGRESS rows as CANCELLED only if started longer than stale_seconds ago.
    Avoids cancelling a pipeline that another instance is currently running when this one restarts.
    """
    if stale_seconds is None:
        stale_seconds = DEFAULT_STALE_CANCEL_AGE_SECONDS
    if stale_seconds <= 0:
        return 0
    try:
        with db_connection() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    """
                    UPDATE data_migrations
                    SET status = 'CANCELLED', completed_at = NOW(), error_message = %s
                    WHERE status = 'IN_PROGRESS' AND started_at < NOW() - (%s * interval '1 second')
                    """,
                    (reason, stale_seconds),
                )
                n = cur.rowcount
            conn.commit()
            if n:
                logger.info(
                    "tracking.cancelled_stale_on_startup",
                    extra={"cancelled": n, "stale_seconds": stale_seconds},
                )
            return n
    except Exception as e:
        logger.warning("tracking.cancel_stale_on_startup_failed", extra={"error": str(e)})
        return 0


def has_any_pipeline_in_progress() -> bool:
    """True if any pipeline command has an IN_PROGRESS row (singleton check)."""
    try:
        with db_connection() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    """
                    SELECT EXISTS(
                        SELECT 1 FROM data_migrations
                        WHERE status = 'IN_PROGRESS' AND command IN %s
                    )
                    """,
                    (tuple(PIPELINE_COMMANDS),),
                )
                return cur.fetchone()[0]
    except Exception as e:
        logger.warning("tracking.check_pipeline_busy_failed", extra={"error": str(e)})
        return True  # fail-closed: assume a pipeline is running to preserve singleton


class Tracker:
    def __init__(self, command, args=None):
        self.command = command
        self.args = args or {}
        self.id = None
        self.conn = None

    def start(self):
        try:
            self.conn = get_connection()
            with self.conn.cursor() as cur:
                if self.command in PIPELINE_COMMANDS:
                    # Atomic singleton: insert only if no other pipeline is IN_PROGRESS.
                    # Prevents race where another instance inserts between check and insert.
                    cur.execute(
                        """
                        INSERT INTO data_migrations (command, args, started_at, status)
                        SELECT %s, %s, NOW(), 'IN_PROGRESS'
                        WHERE NOT EXISTS (
                            SELECT 1 FROM data_migrations
                            WHERE status = 'IN_PROGRESS' AND command IN %s
                        )
                        RETURNING id
                        """,
                        (self.command, json.dumps(self.args), tuple(PIPELINE_COMMANDS)),
                    )
                    row = cur.fetchone()
                    if row is None:
                        raise RuntimeError("Another pipeline step is already running")
                    self.id = row[0]
                else:
                    cur.execute(
                        """
                        INSERT INTO data_migrations (command, args, started_at, status)
                        VALUES (%s, %s, NOW(), 'IN_PROGRESS')
                        RETURNING id
                        """,
                        (self.command, json.dumps(self.args)),
                    )
                    self.id = cur.fetchone()[0]
            self.conn.commit()
            print(f"[Tracking] Started {self.command} (ID: {self.id})")
        except Exception as e:
            logging.error(f"[Tracking] Failed to start: {e}", exc_info=True)
            raise

    def complete(self, metadata=None):
        if not self.id or not self.conn:
            return
        try:
            with self.conn.cursor() as cur:
                cur.execute(
                    """
                    UPDATE data_migrations
                    SET status = 'COMPLETED', completed_at = NOW(), metadata = %s
                    WHERE id = %s
                    """,
                    (json.dumps(metadata or {}), self.id),
                )
            self.conn.commit()
            print(f"[Tracking] Completed {self.command} (ID: {self.id})")
        except Exception as e:
            print(f"[Tracking] Failed to complete: {e}")
        finally:
            self.close()

    def fail(self, error_message):
        if not self.id or not self.conn:
            return
        try:
            with self.conn.cursor() as cur:
                cur.execute(
                    """
                    UPDATE data_migrations
                    SET status = 'FAILED', completed_at = NOW(), error_message = %s
                    WHERE id = %s
                    """,
                    (str(error_message), self.id),
                )
            self.conn.commit()
            print(f"[Tracking] Failed {self.command} (ID: {self.id})")
        except Exception as e:
            print(f"[Tracking] Failed to update failure: {e}")
        finally:
            self.close()

    def close(self):
        if self.conn:
            try:
                self.conn.close()
            except Exception:
                pass
            self.conn = None


@contextmanager
def track(command, args=None):
    t = Tracker(command, args)
    t.start()
    try:
        yield t
        # If the block finishes without exception, mark as complete (unless already closed/failed)
        if t.conn:
            t.complete()
    except Exception as e:
        t.fail(str(e))
        raise
