# Go Application (Importer/ETL/API)

Go services for importing Cricsheet JSON, dataset export, and serving an HTTP API. This component integrates with Postgres and the Python ML service.

Components:
- `cmd/cricsheet-importer`: CLI to import Cricsheet JSON files into the DB (idempotent; optional placeholders).
- `cmd/export-dataset`: CLI to export model-ready CSVs for ML training.
- `cmd/api`: HTTP API server (health/readiness + orchestration endpoints, including `/import/cricsheet` and `/precompute`).
- `cmd/team-predictor`: CLI to select a cricket team based on ML predictions, reading from a pre-generated player pool.
- `cmd/team-select`: CLI to run the end-to-end team selection pipeline either from the DB or from a CSV pool.
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

## Testing & Mocks

We use interfaces and mockery-generated mocks instead of hand-written fakes.

- Interfaces with `//go:generate` live near their packages, e.g.:
  - `internal/mlclient/predictor.go` defines `Predictor` and has a `mockery` directive.
  - `internal/db/connector.go` defines `Connector` and has a `mockery` directive.
- Generate mocks locally:
```
make -C go-app mocks
# or from this directory
make mocks
```
This requires mockery installed:
```
go install github.com/vektra/mockery/v2@latest
```

- Tests import mocks from `internal/*/mocks` and configure behavior with `testify/mock`:
```
ml := &mlclientmocks.Predictor{}
ml.On("PredictWin", mock.Anything, players).Return(players, nil)
```

Database testing
- For simple orchestration tests, depend on the `db.Connector` interface and mock its `Connect` method using `internal/db/mocks`.
- For repository-level tests that need to simulate `database/sql` primitives (e.g., `Rows`, `Result`), prefer `sqlmock` or explicit small interfaces around usage points. Some packages under `internal/cricsheet/mocks` already use `testify/mock`.

Note: Legacy hand-written fakes have been removed from tests in favor of mocks for consistency and maintainability.

### Testing conventions

- Unit tests
  - Exactly one `*_test.go` file per production file in a package (e.g., `client.go` → `client_test.go`).
  - Use table-driven tests: `tests := []struct{ name string; ... }{... }` with `t.Run(tc.name, ...)`.
  - Avoid conditional logic inside tests; extract helpers for comparisons and setup.
  - Shared helpers belong in `helpers_test.go` within the same package; fixtures under `tests/fixtures/`.

- Integration tests
  - Must live under `go-app/integration/` or be clearly named `*_integration_test.go`.
  - Tests should be deterministic and offline by default. If a dependency (e.g., Postgres) is not available, they must `t.Skipf` with a clear message.
  - Keep runtime bounded (use timeouts). See `integration/export_fielding_integration_test.go` for patterns like `runWithTimeout` and repo-relative paths.

- How to run
  - All tests (unit + integration):
    ```
    go test ./go-app/...
    ```
    Integration tests will gracefully skip if Postgres is not reachable.
  - Only integration tests (verbose):
    ```
    go test ./go-app/integration -v
    ```

- Formatting and vet
  - Ensure formatting and vet checks are clean prior to PRs:
    ```
    gofmt -s -l go-app | grep -v "^$" || true
    go vet ./go-app/...
    ```

## Run programs

### Team selection (DB-backed or CSV pool)
Prerequisites:
- Postgres up with imported data and precomputed metrics (see repo root README steps)
- ML service running at http://localhost:8000 (only required when using `team-predictor`)

Using the Makefile convenience target (recommended):
```
# DB-backed selection (build features from DB)
make team-select MATCH=262039498036 SEASON=2025 FORMAT=T20 SIZE=11 MIN_BOWLERS=5 REQUIRE_KEEPER=1 FROM_DB=1

# CSV-backed selection (use pre-generated ml-service/ml/pool.csv)
make team-select MATCH=262039498036 SEASON=2025 FORMAT=T20 FROM_DB=0 POOL=../ml-service/ml/pool.csv
```
Direct invocation of the CLI:
```
# DB-backed
go run ./cmd/team-select -match 262039498036 -format T20 -season 2025 -size 11 -min-bowlers 5 -require-keeper --from-db=true

# CSV-backed
go run ./cmd/team-select -match 262039498036 -format T20 -season 2025 -pool ../ml-service/ml/pool.csv --from-db=false
```
Flags:
- `-match` (required), `-season` (required), `-format` (TEST|ODI|T20I|T20), `-size`, `-min-bowlers`, `-require-keeper`, `-pool`, `-from-db`

### Team predictor (uses ml-service predictions and ml/pool.csv)
Prerequisites:
- ML service running: `make -C ../ml-service run` (or `make ml-serve` from repo root)
- `ml-service/ml/pool.csv` generated: `make -C ../ml-service export-pool MATCH=1193505`

Using the Makefile convenience target:
```
make team-predictor MATCH=1193505 SEASON=2025 FORMAT=T20 BAT=6 BOWL=5
```
Direct invocation of the CLI:
```
GO_APP_CONFIG=./config.json \
  go run ./cmd/team-predictor -match 1193505 -format T20 -season 2025 -bat 6 -bowl 5
```
Notes:
- Default counts for batters/bowlers fall back to `go-app/config.json` if not provided.
- Output prints the selected XI ordered by predicted winning probability.

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

---

### CLI quick reference (team-select, team-predictor)
- team-select flags: `-match` (required), `-season` (required), `-format` (TEST|ODI|T20I|T20), `-size`, `-min-bowlers`, `-require-keeper`, `-pool`, `-from-db`
- team-predictor flags: `-match` (required), `-season` (required), `-format` (TEST|ODI|T20I|T20), `-bat`, `-bowl`

Examples:
```
# DB-backed selection
make -C go-app team-select MATCH=262039498036 SEASON=2025 FORMAT=T20 SIZE=11 MIN_BOWLERS=5 REQUIRE_KEEPER=1 FROM_DB=1

# CSV-backed selection
make -C go-app team-select MATCH=262039498036 SEASON=2025 FORMAT=T20 FROM_DB=0 POOL=../ml-service/ml/pool.csv

# Team predictor using ml-service predictions
make -C go-app team-predictor MATCH=1193505 SEASON=2025 FORMAT=T20 BAT=6 BOWL=5
```

### Make targets (hygiene)
- Format: `make -C go-app fmt` (writes) | Check-only: `make -C go-app fmt-check`
- Vet: `make -C go-app vet`
- Tests: `make -C go-app test`
- Lint (optional if installed): `make -C go-app lint`

Troubleshooting:
- Install tools once via `make -C go-app init` (adds gofumpt/golines; suggests golangci-lint).
- Ensure Go 1.25+ and `$(go env GOPATH)/bin` on PATH.


---

## Testing & Coverage

This project enforces a coverage gate in CI and provides Makefile targets to run coverage locally.

Quick start:
```
# One-time (local): install gofumpt/golines like CI does
make -C go-app init

# Run vet + tests with coverage on the default scoped packages
make -C go-app vet
make -C go-app coverage

# Enforce a minimum coverage threshold (CI uses 90%)
COV_MIN=90 make -C go-app coverage-check

# View the total coverage line
make -C go-app coverage-func

# Generate an HTML report at go-app/coverage.html
make -C go-app coverage-html
```

Notes:
- Scope: By default, coverage is calculated over unit-testable internal packages:
  - `./internal/config ./internal/predictor ./internal/mlclient ./internal/cricsheet`
  You can override the scope:
  ```
  make -C go-app coverage COVERAGE_PACKAGES="./internal/cricsheet ./internal/db"
  ```
- Gate: The Makefile’s `COV_MIN` default is 80 for flexibility locally; CI enforces 90%:
  - See `.github/workflows/go-ci.yml` which runs:
    ```
    make -C go-app coverage
    COV_MIN=90 make -C go-app coverage-check
    ```
- Convenience: Run a CI-like local check in one go:
  ```
  make -C go-app coverage-ci
  ```

## Cricsheet adapter seams (offline tests)

To keep tests offline and deterministic, `internal/cricsheet/ingest.go` depends on small interfaces:
- `CricsheetDB` for DB operations
- `WeatherClient` for enqueueing async weather jobs

The default adapters delegate to real packages. In tests, swap them with fakes:
```go
prevDB := cricsheet.SetCricsheetDB(fakeDB)
prevW  := cricsheet.SetWeatherClient(fakeWeather)
// ... run tests ...
cricsheet.SetCricsheetDB(prevDB)
cricsheet.SetWeatherClient(prevW)
```

The integration-style tests under `internal/cricsheet` use in-memory fakes and temporary files only—no network or DB connections.

## Local formatting

CI installs `gofumpt` automatically. Locally, run once:
```
make -C go-app init
```
Then you can check formatting just like CI:
```
make -C go-app fmt-check
```


