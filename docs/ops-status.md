# Ops Status — Data & ML Readiness Dashboard (API Contract)

This document describes the `/ops/status` endpoint exposed by the Go API. It aggregates the current state of:
- Services availability (Go API and ML service)
- Database connectivity, basic counts, and migration status
- Precompute freshness per cricket format (TEST, ODI, T20I, T20)
- CSV exports presence and basic stats
- ML model artifacts presence and recency
- Fielding and Weather data availability (row counts) for observability
- An ordered list of suggested `make` commands to get the system ready

## Endpoint
- Method: `GET`
- Path: `/ops/status`
- Query params: none

## JSON response (example)
```
{
  "timestamp": "2026-01-22T10:56:00Z",
  "services": {
    "api_health": true,
    "api_readiness": true,
    "ml_health": true
  },
  "db": {
    "connected": true,
    "migration": { "status": "ok", "current": 23, "expected": 23 },
    "counts": { "players": 12345, "matches": 6789, "innings": 13579 }
  },
  "precompute": {
    "last_run": "2026-01-22T09:30:00Z",
    "as_of": "2026-01-22",
    "formats": {
      "TEST": {"status": "ok"},
      "ODI": {"status": "stale"},
      "T20I": {"status": "missing"},
      "T20": {"status": "ok"}
    }
  },
  "exports": {
    "root": "output/go-app",
    "formats": {
      "ODI": {
        "files": [
          {"name": "batting_on.csv", "exists": true, "rows": 123456, "modified": "..."},
          {"name": "bowling_on.csv", "exists": true, "rows": 123456, "modified": "..."}
        ]
      }
    }
  },
  "artifacts": {
    "root": "output/ml-service",
    "formats": {
      "ODI": {
        "batting": {"exists": true, "loaded": true, "modified": "..."},
        "bowling": {"exists": true, "loaded": true, "modified": "..."}
      }
    }
  },
  "suggestions": [
    {"reason": "Exports missing for T20I", "commands": ["cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -format=T20I"]}
  ]
}
```

## Section reference

- `services`
  - `api_health`: liveness of Go API.
  - `api_readiness`: DB connectivity check.
  - `ml_health`: liveness of ML service (`/health`).

- `db`
  - `connected`: result of DB ping.
  - `counts`: `players`, `matches`, `innings` row counts (used to detect empty DBs).
  - `migration`: `{status,current,expected}` where `status ∈ {ok, unknown, out_of_date}`.

- `precompute`
  - Derived from in-memory `precompute.GetStatus()`.
  - Freshness rule: A format included in the last run is `ok` if the run finished on the same UTC day; otherwise `stale`. Formats not included are `missing`.

- `exports`
  - Scans the exports root (non-recursive). Root is taken from **GO_APP_OUTPUT_DIR** when set (e.g. `/output/go-app` in Docker), otherwise from config or default `output/go-app`. This ensures the dashboard reflects exports when the API runs in Docker with a bind-mounted output dir.
  - Recognizes unified files `batting_on.csv` and `bowling_on.csv` (applied to all formats) and per-format files with tokens like `bat`/`bowl` and the format code in the filename.
  - Each file entry includes `exists`, `modified` (RFC3339), and a capped `rows` count for a quick sanity check.

- `artifacts`
  - First probes ML service `/artifacts/status` when available; otherwise falls back to scanning a filesystem root for `*.joblib` files. Fallback root is **GO_APP_ARTIFACTS_ROOT** when set, otherwise `output/ml-service`. In Docker, the ML service HTTP path is normally used; the fallback is for host runs when the ML service is down.
  - Per-format fields: `batting` and `bowling` objects with `exists`, optional `loaded`, optional `modified`, and `path` when discovered from FS.

- `fielding`
  - Summarizes fielding data availability from database table `fielding_data`.
  - Keys: `available` (boolean), `rows` (total rows when connected).

- `weather`
  - Summarizes weather data availability from database table `weather_data`.
  - Keys: `available` (boolean), `rows` (total rows when connected).

- `suggestions`
  - Ordered, actionable `make` commands computed from the snapshot. All commands assume run from project root.
  - Priority rules: DB → Precompute → Exports → Artifacts → Services.
  - Examples (copy-paste ready):
    - DB not ready: `make migrate && make cricsheet-import`
    - Precompute missing/stale: `make precompute-asof` (add `ASOF=YYYY-MM-DD` for a specific date)
    - Exports missing: `make export-dataset`
    - Artifacts missing: `make ml-install && make train-batting`, `make train-bowling`
    - ML service down: `make dev-up` (or `make dev-rebuild`)

## Quick verification

Bring stack up:
```
make dev-up
```
Check health and ops status:
```
curl -s http://localhost:8080/health | jq
curl -s http://localhost:8080/readiness | jq
curl -s http://localhost:8080/ops/status | jq
```

### Frontend visuals

- In the Ops Status UI, Fielding and Weather sections render small tiles showing:
  - Available: yes/no (green/red)
  - Rows: total count (neutral)
- Suggestions are displayed within each section card (DB, Precompute, Exports, Artifacts, Fielding, Weather); there is no global suggestions block.

Force a gap (artifacts) and recheck:
```
rm -rf output/ml-service/*
curl -s http://localhost:8080/ops/status | jq '.artifacts, .suggestions'
```

Recreate exports gap (ODI) and recheck:
```
rm -f output/go-app/*ODI* 2>/dev/null || true
curl -s http://localhost:8080/ops/status | jq '.exports.formats.ODI, .suggestions'
```

Train models and recheck:
```
make train-all
curl -s http://localhost:8080/ops/status | jq '.artifacts, .suggestions'
```
