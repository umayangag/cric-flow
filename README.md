# Cric App (Monorepo)

This repository contains the Go data pipeline/services and the Python ML inference service, ported from the original `src/` Python prototype (which remains intact for reference).

Directories:
- go-app/ — Go services (API, Cricsheet importer, dataset export, tools, migrations, team predictor CLI)
- ml-service/ — Python FastAPI service for predictions and training scripts (precomputation now lives in go-app)
- src/ — Original prototype (reference only)

## Quick start (happy-path)

Prerequisites:
- Docker + Docker Compose
- Go 1.25+
- Python 3.10+ (only needed if training models locally without Docker)

Environment defaults used by Go services/API:
- POSTGRES_HOST=localhost, POSTGRES_PORT=5432, POSTGRES_DB=cricket_data
- POSTGRES_USER=postgres, POSTGRES_PASSWORD=postgres, POSTGRES_SSLMODE=disable

### 0) One-time local setup (tools, venv, hooks)
Initialize dev tooling for both components, aligned with CI formatters/linters.
```
make init
# or per component:
make -C go-app init
make -C ml-service init
```
- For Python, activate the venv after init:
```
cd ml-service && source .venv/bin/activate
```

### 1) One-line bootstrap (recommended)
This single command brings up Docker services, applies migrations, imports Cricsheet JSON data, precomputes metrics, exports datasets, trains ML models, and restarts the ML service to load artifacts.
```
make up-all
```

### Or, bring up Postgres (and optional services) manually
```
make dev-up
```

### 2) Run DB migrations
```
make migrate
```

### 3) Import Cricsheet JSON (idempotent)
This reads local Cricsheet `.json` files under `data/` and upserts into Postgres using the Go importer.
```
make cricsheet-import
# run again to confirm idempotency
make cricsheet-import
```

### 4) Precompute player metrics (form/venue/opposition/consistency)
This triggers the Go API to compute and store features in Postgres (no ML dependency).
```
make precompute
# or with filters
curl -X POST http://localhost:8080/precompute -H 'Content-Type: application/json' -d '{"season":"2019","formats":["ODI","T20I"]}'
# check status (in-memory, resets on restart)
curl -s http://localhost:8080/precompute/status | jq
```

### 5) Export model datasets (now includes unified cross-format files)
The exporter now has a unified mode that writes a single merged CSV per task (batting/bowling) across all formats, and includes leakage-free, date-indexed (as-of) per-format features for TEST/ODI/T20I/T20.

Recommended (Makefile runs both unified and legacy for compatibility):
```
make export-dataset
```
Outputs:
- Unified (new):
  - `output/go-app/batting_encoded_all.csv`
  - `output/go-app/bowling_encoded_all.csv`
- Legacy (still produced for current training scripts):
  - `output/go-app/batting_encoded.csv`
  - `output/go-app/bowling_encoded.csv`

To call the exporter directly:
```
cd go-app && GO_APP_OUTPUT_DIR=../output/go-app \
  go run ./cmd/export-dataset -unified=1
# and (legacy unsuffixed files) without flags
cd go-app && GO_APP_OUTPUT_DIR=../output/go-app \
  go run ./cmd/export-dataset
```

Column naming (examples):
- Batting unified columns: `bat_form_TEST_asof`, `bat_form_ODI_asof`, `bat_form_T20I_asof`, `bat_form_T20_asof`, with matching `n_samples_*` reliability columns.
- Opposition/Venue: `bat_vs_opp_FMT_asof`, `bat_at_venue_FMT_asof` (and `bowl_*` analogues for bowling).
- Each row also includes `format_code` for the match.

### 6) Train ML artifacts (optional but recommended)
From the exported CSVs, create `joblib` scaler/model files consumed by the ML service.
```
make train-all
```
Artifacts will be saved under `output/ml-service/`.

### 7) Start ML service and check health
```
make ml-serve
# in another terminal
curl -s http://localhost:8000/health | jq
```
Expected: `{"status":"ok","batting_model":true,"bowling_model":true}` once artifacts are trained.

### 8) Run API (optional orchestration)
```
make api
# liveness/readiness
curl -s http://localhost:8080/health
curl -s http://localhost:8080/readiness
```

### 9) Predict team (DB-backed, end-to-end)
Requires a `match_id` that exists in the DB from the import step. This path mirrors the prototype logic but builds features from the DB and calls the ML service for per-player and win predictions.
```
cd go-app
GO_APP_CONFIG=./config.json \
POSTGRES_HOST=localhost POSTGRES_DB=cricket_data POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres \
go run ./cmd/team-select -match <match_id> -format T20 -season 2019 -size 11 -min-bowlers 5 --require-keeper --from-db=true
```
Output shows the ranked XI and the team average winning probability.

Alternative (CSV pool path, prototype-style):
```
# generate pool.csv with the Python helper and then call the win model via service
make team-predictor MATCH=<match_id>
```
- Team selection parameters can be configured in `go-app/config.json` (sections `team`, `predictor`, and `selection`) or overridden via CLI flags to `go-app/cmd/team-select`. 

## System architecture
For a high-level diagram of how components connect and the order of execution from raw data to the final team prediction, see:
- docs/ARCHITECTURE.md

## Configuration and paths
This repo standardizes file IO locations and makes them configurable via JSON, environment variables, and CLI flags.

- Directory conventions:
  - Inputs come from `data/{package}/...` (e.g., `data/go-app/cricsheet`)
  - Outputs go to `output/{package}/...` (e.g., `output/go-app`, `output/ml-service`)
- Config files:
  - `go-app/config.json` (for Go application settings, including team selection parameters)
  - `ml-service/config.json` (for ML service settings, including team prediction parameters)
- Precedence (highest to lowest):
  1. CLI flags/args (`-dir`, `-out`, `--csv`, `--out`)
  2. Environment variables (`GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`, `ML_SERVICE_OUTPUT_DIR`, etc.)
  3. Config file (JSON) in the component directory
  4. Built-in defaults

See `docs/CONFIG.md` for full schema and examples.

## Formatting and linting
- Aggregate format both components:
```
make fmt
```
- Check formatting only (fails on diff):
```
make fmt-check
```
- Per component:
  - Go: `make -C go-app fmt` or `make -C go-app fmt-check`
  - Python: `make -C ml-service fmt` or `make -C ml-service fmt-check`
- Developer hooks (pre-commit runs gofumpt/golines for Go and black/isort for Python):
```
make install-hooks
```
- Additional lint helpers:
  - Go: `make lint-go`
  - Python: `make lint-py`

## Notes
- Data ingestion now uses Cricsheet JSON files (no HTML scraping or external requests during import).
- Unique constraints and upsert logic ensure idempotent persistence.
- Optional placeholders can be inserted by the importer: weather rows per innings and zeroed fielding rows (see Makefile target `cricsheet-import` or API `/import/cricsheet`).
- The ML service returns non-zero predictions only when trained artifacts are present.
- For development speed, this setup targets the happy path first; additional edge cases can be covered by enhancing the importer as needed.

## CI
Two separate GitHub Actions workflows:
- Go App: `.github/workflows/go-app-ci.yml` — spins up Postgres, applies migrations, checks formatting (gofumpt/golines), builds and tests Go modules.
- ML Service: `.github/workflows/ml-service-ci.yml` — installs deps, runs isort/black checks, and sanity-compiles the app and training scripts.

## Troubleshooting
- If API cannot connect to DB, ensure Postgres is up: `make dev-up` and check `docker compose ps`.
- If ML `/health` shows models=false, (re)run `make train-all` after exporting datasets.
- If the importer reports 0 files processed, ensure you have Cricsheet `.json` files under `data/` (or pass `-dir` to `cricsheet-import`).



## As-of (time-indexed) precompute — single command for all formats (new)
The legacy `make precompute` triggers the API’s seasonal/aggregate metrics and does not populate the new `*_asof` tables. To fill the date-indexed snapshot tables (`player_form_asof`, `player_consistency_asof`, `player_vs_opposition_asof`, `player_at_venue_asof`), run the as-of precompute across all formats. If you don’t pass a date, it defaults to today (UTC):

```
make precompute-asof
```

Notes and parameters:
- `ASOF` — optional cutoff date (YYYY-MM-DD). Defaults to today (UTC). Features are computed using only matches strictly BEFORE this date.
- `ALPHA` — EWM alpha (default 0.3)
- `LASTN` — window N for consistency (default 10)
- History window (how many past matches to consider) is controlled by `features.history_window_matches` in `go-app/config.json` (0 = unlimited).

Examples:
```
# Snapshots as of today (UTC) for all formats (TEST, ODI, T20I, T20)
make precompute-asof

# Snapshots as of a specific date
make precompute-asof ASOF=2020-12-31

# With tuned parameters
make precompute-asof ASOF=2020-12-31 ALPHA=0.35 LASTN=12
```

Behind the scenes this runs the Go CLI `go-app/cmd/precompute-features` once per format with `-as-of`, applying DB migrations automatically. Ensure you have already imported data (e.g., via `make cricsheet-import`).
