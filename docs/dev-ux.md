# Dev UX — Common Workflows and Commands

Active path: 1 -> 1.8 -> 1.6
Parent: 1.8

This guide summarizes the most common developer tasks for this repo.

## Environment
- Copy `.env.example` to `.env` and adjust as needed. The defaults match Docker Compose networking.
- Go services use:
  - `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_SSLMODE`
  - Optional logging: `LOG_LEVEL`, `LOG_FORMAT`
  - Optional config/paths: `GO_APP_CONFIG`, `GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`, `MIGRATIONS_DIR`

## Bootstrap
- One‑shot environment and tooling init:
```
make init
```
- Bring up Docker services (Postgres, API, ML):
```
make dev-up
```
- Tear down:
```
make dev-down
```
- Tail logs:
```
make logs
```

## Database
- Apply migrations (env vars can override defaults):
```
make migrate
```

## Data ingestion and processing
- Import Cricsheet JSON (idempotent):
```
make cricsheet-import
```
- Precompute metrics:
```
make precompute
```
- Export datasets (unified + legacy):
```
make export-dataset
```

## Go commands (direct)
- Run API:
```
make api
```
- Team predictor (DB-backed + ML):
```
make team-predictor MATCH=<match_id>
```

## Testing, formatting, linting
- Run all Go tests:
```
make go-test
```
- Run Go vet (inside go-app module):
```
cd go-app && go vet ./...
```
- Aggregate formatters (Go + Python):
```
make fmt
```
- Pre-commit hooks:
```
make install-hooks
```

## Mocks
- Install pinned mockery once (for deterministic generation):
```
go install github.com/vektra/mockery/v2@v3.5.5
```
- Generate/update mocks from repo root using `.mockery.yaml`:
```
make mock
```

## Helpful end-to-end flows
- Full happy-path bootstrap (DB up, migrate, import, precompute, export, train, restart ML):
```
make up-all
```
- Format-aware end-to-end:
```
make e2e FORMAT=ODI SEASON=2019
```
