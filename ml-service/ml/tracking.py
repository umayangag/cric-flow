import json
import logging
from contextlib import contextmanager
from typing import Optional

from dotenv import load_dotenv

from ml.db import get_db_connection

# Load environment variables from .env file if it exists
load_dotenv()

# Commands that form the main pipeline; only one may be IN_PROGRESS at a time (singleton).
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


# Default age (minutes) for considering an IN_PROGRESS run stale on startup. Override via TRACKING_STALE_CANCEL_AGE_MINUTES.
DEFAULT_STALE_CANCEL_AGE_MINUTES = 24 * 60  # 24 hours


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
    try:
        conn = get_connection()
        try:
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
                print(f"[Tracking] Cancelled {n} stale IN_PROGRESS migration(s) on startup (older than {stale_minutes}m)")
            return n
        finally:
            conn.close()
    except Exception as e:
        logging.warning("[Tracking] Failed to cancel stale IN_PROGRESS on startup: %s", e)
        return 0


def has_any_pipeline_in_progress() -> bool:
    """True if any pipeline command has an IN_PROGRESS row (singleton check)."""
    try:
        conn = get_connection()
        try:
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
        finally:
            conn.close()
    except Exception as e:
        logging.warning("[Tracking] Failed to check pipeline busy: %s", e)
        return False


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
