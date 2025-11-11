import os

import psycopg2

# Prefer environment overrides first (supports both POSTGRES_* and legacy DB_*),
# then fall back to ml-service/config.json via ml.config.
try:
    import config as svc_config  # type: ignore
except Exception:  # pragma: no cover - fallback if config module not importable
    svc_config = None  # type: ignore


def _cfg_default(getter_name: str, fallback: str) -> str:
    if svc_config is None:
        return fallback
    try:
        getter = getattr(svc_config, getter_name)
        val = getter()
        if val:
            return str(val)
    except Exception:
        pass
    return fallback


def get_db_connection():
    host = os.environ.get("POSTGRES_HOST") or os.environ.get("DB_HOST") or _cfg_default(
        "default_db_host", "localhost"
    )
    port = os.environ.get("POSTGRES_PORT") or os.environ.get("DB_PORT") or _cfg_default(
        "default_db_port", "5432"
    )
    name = os.environ.get("POSTGRES_DB") or os.environ.get("DB_NAME") or _cfg_default(
        "default_db_name", "cricket_data"
    )
    user = os.environ.get("POSTGRES_USER") or os.environ.get("DB_USER") or _cfg_default(
        "default_db_user", "postgres"
    )
    password = os.environ.get("POSTGRES_PASSWORD") or os.environ.get("DB_PASSWORD") or _cfg_default(
        "default_db_password", "postgres"
    )

    return psycopg2.connect(
        host=host,
        port=port,
        dbname=name,
        user=user,
        password=password,
    )
