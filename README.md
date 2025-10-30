# Cric App (Monorepo)

This repository contains the Go data pipeline/services and the Python ML inference service, ported from the original `src/` Python prototype (which remains intact for reference).

Directories:
- go-app/ — Go services (API, scraper, ETL importer, precompute, dataset export, tools, migrations)
- ml-service/ — Python FastAPI service for predictions + simple training scripts
- src/ — Original prototype (reference only)

## Quick start (happy-path)

Prerequisites:
- Docker + Docker Compose
- Go 1.22+
- Python 3.10+ (only needed if training models locally without Docker)

Environment defaults used by Go services/API:
- POSTGRES_HOST=localhost, POSTGRES_PORT=5432, POSTGRES_DB=cricket_data
- POSTGRES_USER=postgres, POSTGRES_PASSWORD=postgres, POSTGRES_SSLMODE=disable

### 1) One-line bootstrap (recommended)
This single command brings up Docker services, applies migrations, scrapes a tiny window, precomputes metrics, exports datasets, trains ML models, and restarts the ML service to load artifacts.
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

### 3) Scrape a tiny window (idempotent)
This fetches Sri Lanka ODI innings over a narrow date window and upserts to Postgres.
```
make scraper
# run again to confirm idempotency
make scraper
```

### 4) Precompute player metrics (form/venue/opposition/consistency)
```
make precompute SEASON=2019
```
(You can omit SEASON to compute for all seasons present.)

### 5) Export model datasets
```
make export-dataset
# outputs to src/final_data/output/batting_encoded.csv and bowling_encoded.csv
```

### 6) Train ML artifacts (optional but recommended)
From the exported CSVs, create `joblib` scaler/model files consumed by the ML service.
```
make train-all
```
Artifacts will be saved under `ml-service/models/`.

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

### 9) Predict team (happy path CLI)
Requires a `match_id` that exists in the DB from the scrape step.
```
make team-predictor MATCH=<match_id> BAT=6 BOWL=5
```
- Ensures at least 5 bowlers are selected (part-time allowed).
- Adjust `BAT`/`BOWL` as desired; minimum bowlers enforced is 5.

## Notes
- The scraper uses polite rate limiting and retries.
- Unique constraints and upsert logic ensure idempotent persistence.
- The ML service returns non-zero predictions only when trained artifacts are present.
- For development speed, this setup targets the happy path first; additional edge cases can be covered by adding more HTML fixtures and selectors later.

## CI
GitHub Actions workflow at `.github/workflows/ci.yml`:
- Spins up Postgres, applies migrations
- Builds and tests Go modules
- Sanity-compiles Python ML modules and training scripts

## Troubleshooting
- If API cannot connect to DB, ensure Postgres is up: `make dev-up` and check `docker compose ps`.
- If ML `/health` shows models=false, (re)run `make train-all` after exporting datasets.
- If scraper returns no items, try widening the date window in `make scraper` target or in `cmd/scraper` flags.
