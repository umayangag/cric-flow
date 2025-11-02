# Go Application (Importer/ETL/API)

Go services for importing Cricsheet JSON, dataset export, and serving an HTTP API. This component integrates with Postgres and the Python ML service.

Components:
- `cmd/cricsheet-importer`: CLI to import Cricsheet JSON files into the DB (idempotent; optional placeholders).
- `cmd/export-dataset`: CLI to export model-ready CSVs for ML training.
- `cmd/api`: HTTP API server (health/readiness + orchestration endpoints, including `/import/cricsheet` and `/precompute`).
- `cmd/team-predictor`: CLI to select a cricket team based on ML predictions, reading from a pre-generated player pool.
- `cmd/tools/migrate`: DB migration runner.
- `internal/*`: packages for Cricsheet parsing, contracts, repos, ML client, etc. (Note: Feature calculation logic has moved to the ML service).

Prerequisites:
- Go 1.25+
- Postgres reachable using the following defaults (override via env):
  - `POSTGRES_HOST=localhost`, `POSTGRES_PORT=5432`, `POSTGRES_DB=cricket_data`
  - `POSTGRES_USER=postgres`, `POSTGRES_PASSWORD=postgres`, `POSTGRES_SSLMODE=disable`

## One-time setup
Install tools and download modules used by CI and local dev:
```
make init
```
This installs `gofumpt` and `golines` into `$(go env GOPATH)/bin`. Ensure that directory is on your `PATH`.

## Configuration
The Go app reads defaults from `go-app/config.json`, environment variables, and CLI flags in this precedence:
1. CLI flags (e.g., `-dir`, `-out`)
2. Environment variables (`GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`)
3. Config file `go-app/config.json`
4. Built-in defaults

Schema for `go-app/config.json` (Note: `etl_dir` is no longer used as ETL is handled by the ML service):
```json
{
  "inputs": {
    "cricsheet_dir": "../data/go-app/cricsheet"
  },
  "outputs": {
    "export_dir": "../output/go-app"
  }
}
```
See `docs/CONFIG.md` for details and examples.

## Common tasks (Makefile)
- Build binaries:
```
make build
```
- Run API locally on :8080:
```
make run-api
```
- Import Cricsheet JSON into the DB (from repo root or here):
```
make -C .. cricsheet-import
# or directly (uses GO_APP_INPUT_DIR or config.json default)
GO_APP_INPUT_DIR=../data/go-app/cricsheet \
  go run ./cmd/cricsheet-importer -dir=$GO_APP_INPUT_DIR --placeholders-weather --placeholders-fielding
```
- Export model datasets (writes to output/go-app by default):
```
make -C .. export-dataset
# or directly (uses GO_APP_OUTPUT_DIR or config.json default)
GO_APP_OUTPUT_DIR=../output/go-app \
  go run ./cmd/export-dataset -out=$GO_APP_OUTPUT_DIR
```
- Apply DB migrations (uses env vars above):
```
make migrate
```
- Docker images:
```
make docker-build
make docker-run
```

## Formatting and checks
- Format Go code (gofumpt + golines):
```
make fmt
```
- Check formatting only (fails on diff), mirrors CI:
```
make fmt-check
```
- Vet and tests:
```
make vet
make test
```

Notes:
- File IO conventions: inputs under `data/go-app/...`, outputs under `output/go-app`.
- Formatting/linting conventions match the GitHub Actions workflow.
- See repo root `README.md` for end-to-end workflows and orchestration commands.
```