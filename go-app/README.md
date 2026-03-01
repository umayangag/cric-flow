# Go Application (Importer / API / Export)

Go services: Cricsheet import, dataset export, HTTP API. Integrates with Postgres and the Python ML service. Feature computation for training export and at prediction cutoff runs in go-app; ML service trains models and serves predictions.

**Components:** `cmd/cricsheet-importer`, `cmd/export-dataset`, `cmd/api`, `cmd/team-predictor`, `cmd/team-select`, `cmd/migrate`; `internal/*` (Cricsheet, contracts, repos, ML client).

**Prerequisites:** Go 1.26+, Postgres (defaults: `POSTGRES_HOST=localhost`, `POSTGRES_PORT=5432`, `POSTGRES_DB=cricket_data`, etc.). Override via env.

## Setup and config
- **One-time:** `make init` — installs gofumpt, golines, golangci-lint; ensure `$(go env GOPATH)/bin` on PATH.
- **Config:** Precedence: CLI flags → env → `go-app/config.json` → defaults. See **docs/config-and-data.md** for full schema and examples.

## Common tasks
- **Build / run:** `make build`, `make run-api` (API on :8080)
- **Import:** `make cricsheet-import` (or with `PLACEHOLDERS=1`, `FAIL_FAST=0`). From repo root: `make cricsheet-import`
- **Export:** `make export-dataset` (unified); optional sequence columns: `ENABLE_SEQ_FEATURES=1` or `-enable-seq=1`. See **docs/config-and-data.md**
- **Migrations:** `make migrate`
- **Docker:** `make docker-build`, `make docker-run`

## Testing and mocks
- **Mocks:** Generate with `make mock` from repo root (uses `go-app/.mockery.yml`). Install mockery: `go install github.com/vektra/mockery/v3@v3.6.0`. Tests use `internal/*/mocks` and `testify/mock`.
- **Backtest API:** See **docs/apis-backtest-and-ops.md** for endpoints, `ML_SERVICE_URL`, and evaluate flow.

### Testing conventions

- Unit tests
  - Exactly one `*_test.go` file per production file in a package (e.g., `client.go` → `client_test.go`).
  - Use table-driven tests: `tests := []struct{ name string; ... }{... }` with `t.Run(tc.name, ...)`.
  - Avoid conditional logic inside tests; extract helpers for comparisons and setup.
  - Shared helpers belong in `helpers_test.go` within the same package; prefer package-local `testdata/` directories for fixtures to avoid CI path issues.

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

### Unified precompute (features + sequential)
Run both as-of/replay precompute features and sequential feature calculations in a single command:
```
# Replay mode (iterates over matches chronologically)
cd go-app && go run ./cmd/precompute-all -format=T20 -replay=1

# Point-in-time (as-of) mode
cd go-app && go run ./cmd/precompute-all -format=ODI -as-of=2020-12-31

# Options
#   -format       TEST|ODI|T20|T20I (aliases accepted: MDM→TEST, ODM→ODI, IT20→T20I)
#   -ewm-alpha    (0,1] (default 0.3) for as-of precompute EWM aggregates
#   -lastN        int >= 0 (default 10) for consistency windows
#   -seq-targets  comma-separated sequence targets or 'all' (default)
#   -seq-dry-run  list computations without writing (false by default)
```

Makefile convenience targets from repo root:
```
# Single format (pass extra flags via ARGS="...")
make precompute-all FORMAT=T20 ARGS="-replay=1"

# All formats in order: TEST, ODI, T20I, T20
make precompute-all-all-formats ARGS="-as-of=2020-12-31"
```

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
- For CSV-backed selection, use a pre-generated pool CSV (see team-select `POOL` and `FROM_DB=0`). Pool generation via go-app/DB + ML predict is the supported path.

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
`make fmt`, `make fmt-check`, `make vet`, `make test`. Inputs: `data/go-app/...`, outputs: `output/go-app`. See root README and **docs/overview.md**.

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
- Ensure Go 1.26+ and `$(go env GOPATH)/bin` on PATH.


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
- Scope: Coverage excludes `cmd/*`, `*/mocks`, `internal/models`, `internal/safeurl`. Override with `COVERAGE_PACKAGES=./...` for full scope.
- Gate: The Makefile’s `COV_MIN` default is 60; CI uses 60 (see workflow).
- Convenience: Run a CI-like local check in one go:
  ```
  make -C go-app coverage-ci
  ```

Cricsheet tests use seams (`SetCricsheetDB`, `SetWeatherClient`). See **docs/quality-and-debugging.md**.





---

## Env and layout
See `.env.example`: `LOG_FORMAT`, `LOG_LEVEL`, `ML_BASE_URL`, Postgres vars. **internal/server:** handlers in `backtest_handlers.go`, orchestration in `backtest_services.go`, seams in `backtest_seams.go`; **internal/cli/flags:** helpers for date/CSV/duration parsing. Logger: `internal/logger` (slog); config: `internal/config.Load()`. See **docs/quality-and-debugging.md** for test standards.
