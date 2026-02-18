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
# Single source of truth: keep in sync with go-app/internal/pipeline/job.go PipelineCommands.
# Any change must be applied in both places. Long-term: shared config or build-time generation.
PIPELINE_COMMANDS = (
    "cricsheet-import",
    "precompute-features",
    "export-dataset",
    "train-batting",
    "train-bowling",
    "train-fielding",
)


def get_connection():
    return get_db_connection()


# Default age (minutes) for considering an IN_PROGRESS run stale on startup.
DEFAULT_STALE_CANCEL_AGE_MINUTES = 24 * 60  # 24 hours


def parse_stale_cancel_age_minutes() -> Optional[int]:
    """Parse TRACKING_STALE_CANCEL_AGE (e.g. 24h, 30m) or TRACKING_STALE_CANCEL_AGE_MINUTES.
    Aligns with go-app; returns minutes or None to use default."""
    raw = os.environ.get("TRACKING_STALE_CANCEL_AGE", "").strip()
    if raw:
        m = re.match(r"^(\d+)(h|m|s)$", raw.lower())
        if m:
            num, unit = int(m.group(1)), m.group(2)
            if unit == "h":
                return num * 60
            if unit == "m":
                return num
            if unit == "s":
                if num == 0:
                    return None  # align with Go: non-positive uses default
                return max(1, num // 60)
            return None
        logger.warning("tracking.invalid_TRACKING_STALE_CANCEL_AGE", extra={"value": raw})
        return None
    raw = os.environ.get("TRACKING_STALE_CANCEL_AGE_MINUTES", "").strip()
    if not raw:
        return None
    try:
        return int(raw)
    except ValueError:
        logger.warning("tracking.invalid_TRACKING_STALE_CANCEL_AGE_MINUTES", extra={"value": raw})
        return None


def cancel_in_progress_on_startup(
    reason: str = "interrupted (ml-service restart or crash)",
    stale_minutes: Optional[int] = None,
) -> int:
    """Mark IN_PROGRESS rows as CANCELLED only if started longer than stale_minutes ago.
    Avoids cancelling a pipeline that another instance is currently running when this one restarts.
    """
    if stale_minutes is None:
        stale_minutes = DEFAULT_STALE_CANCEL_AGE_MINUTES
    if stale_minutes <= 0:
        return 0
    conn = None
    try:
        conn = get_connection()
        with conn.cursor() as cur:
            cur.execute(
                """
                UPDATE data_migrations
                SET status = 'CANCELLED', completed_at = NOW(), error_message = %s
                WHERE status = 'IN_PROGRESS' AND started_at < NOW() - (%s * interval '1 minute')
                """,
                (reason, stale_minutes),
            )
            n = cur.rowcount
        conn.commit()
        if n:
            logger.info(
                "tracking.cancelled_stale_on_startup",
                extra={"cancelled": n, "stale_minutes": stale_minutes},
            )
        return n
    except Exception as e:
        logger.warning("tracking.cancel_stale_on_startup_failed", extra={"error": str(e)})
        return 0
    finally:
        if conn is not None:
            try:
                conn.close()
            except Exception:
                pass


def has_any_pipeline_in_progress() -> bool:
    """True if any pipeline command has an IN_PROGRESS row (singleton check)."""
    conn = None
    try:
        conn = get_connection()
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
    finally:
        if conn is not None:
            try:
                conn.close()
            except Exception:
                pass


class Tracker:
    def __init__(self, command, args=None):
        self.command = command
        self.args = args or {}
        self.id = None
        self.conn = None

    def start(self):
        try:
            if self.command in PIPELINE_COMMANDS and has_any_pipeline_in_progress():
                raise RuntimeError("Another pipeline step is already running")
            self.conn = get_connection()
            with self.conn.cursor() as cur:
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
