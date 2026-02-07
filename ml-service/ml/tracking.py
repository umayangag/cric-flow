import json
import os
from contextlib import contextmanager

# Try importing db, handle failure gracefully if running outside context
try:
    import db
except ImportError:
    db = None


def get_connection():
    if db and hasattr(db, "get_db_connection"):
        return db.get_db_connection()

    # Fallback/Direct connection
    import psycopg2

    return psycopg2.connect(
        host=os.environ.get("DB_HOST", "localhost"),
        port=os.environ.get("DB_PORT", "5432"),
        dbname=os.environ.get("DB_NAME", "cricinfo"),
        user=os.environ.get("DB_USER"),
        password=os.environ.get("DB_PASSWORD"),
    )


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
            print(f"[Tracking] Failed to start: {e}")
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
