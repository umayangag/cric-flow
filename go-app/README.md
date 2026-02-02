# Go Application (Importer/ETL/API)

Go services for importing Cricsheet JSON, dataset export, and serving an HTTP API. This component integrates with Postgres and the Python ML service.

Components:
- `cmd/cricsheet-importer`: CLI to import Cricsheet JSON files into the DB (idempotent; optional placeholders).
- `cmd/export-dataset`: CLI to export model-ready CSVs for ML training.
- `cmd/api`: HTTP API server (health/readiness + orchestration endpoints, including `/import/cricsheet` and `/precompute`).
- `cmd/team-predictor`: CLI to select a cricket team based on ML predictions, reading from a pre-generated player pool.
- `cmd/team-select`: CLI to run the end-to-end team selection pipeline either from the DB or from a CSV pool.
- `cmd/migrate`: DB migration runner.
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
- Export model datasets (writes to output/go-app by default). Unified cross-format is recommended:
```
make -C .. export-dataset
# or directly (uses GO_APP_OUTPUT_DIR or config.json default)
GO_APP_OUTPUT_DIR=../output/go-app \
  go run ./cmd/export-dataset -unified=1 -out=$GO_APP_OUTPUT_DIR
```

To append optional sequence feature columns to the exports, enable via flag or environment:
```
# Using CLI flag
GO_APP_OUTPUT_DIR=../output/go-app \
  go run ./cmd/export-dataset -unified=1 -enable-seq=1 -out=$GO_APP_OUTPUT_DIR

# Using environment gate (equivalent)
ENABLE_SEQ_FEATURES=1 GO_APP_OUTPUT_DIR=../output/go-app \
  go run ./cmd/export-dataset -unified=1 -out=$GO_APP_OUTPUT_DIR
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
brew install mockery
# Linux/CI: install via your package manager or fallback:
#   go install github.com/vektra/mockery/v2@latest
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





---

## Coverage & CI gates

We maintain an informational coverage gate in CI at 50% while we continue to raise coverage across the codebase. CI runs with a curated package scope to reflect actively maintained and tested components. Locally you can reproduce the CI behavior:

```
COVERAGE_PACKAGES="./internal/cli/... ./internal/commands/... ./internal/services/... \
./internal/adapters/httpx/... ./internal/adapters/clock/... ./internal/adapters/random/... \
./internal/logger ./internal/config ./internal/mlclient ./internal/predictor" \
make coverage && make coverage-func && COV_MIN=50 make coverage-check
```

You can broaden `COVERAGE_PACKAGES` over time and raise `COV_MIN` as coverage improves.

## Mock generation (mockery)

We prefer interface-driven code with mocks generated by `mockery`. Mock generation is centralized at the repo root with `.mockery.yaml` and pinned to `mockery` v3.5.5 for deterministic output.

```
# Install mockery (pinned)
go install github.com/vektra/mockery/v3@v3.6.0

# Generate mocks from repo root
make mock
```

Tests can also use small local fakes in table-driven style where appropriate.

## Environment variables

See `.env.example` for commonly used variables. Copy to `.env` and adjust values:

- `LOG_FORMAT` (`json`|`text`)
- `LOG_LEVEL` (`debug`|`info`|`warn`|`error`)
- `ML_BASE_URL` (base URL for the Python ML service)
- Postgres settings used by `cmd/api` and migration tooling: `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_SSLMODE`

Commands generally accept flags that override env and config defaults.

## Internal Server Package Layout

The `internal/server` package is organized for readability and testability:

- `backtest_handlers.go` — HTTP handlers for backtest-related endpoints (request parsing, response writing only).
- `backtest_services.go` — orchestration and pure helpers that implement the backtest logic; small, named functions.
- `backtest_types.go` — DTOs, small structs, and interfaces used by handlers/services.
- `backtest_seams.go` — overridable seams/interfaces for DB/ML calls to enable unit testing without real dependencies.
- `matches.go` — handler for listing matches filtered by season/date/format; uses a DAO seam (`db.ListMatches`).
- `seasons.go` — handler for querying the next season after a cutoff date; uses a DAO seam (`db.GetNextSeasonAfter`).
- `squads.go` — handler for fetching squads and player predictions for a match; maps DB rows to response DTOs.
- `json.go`, `response.go`, `cors.go`, `router.go`, `app.go` — shared HTTP utilities, app setup, and routing.

Guidelines:
- Handlers: validate/parse, delegate to services, never contain complex logic.
- Services/helpers: prefer pure functions; keep them under ~80 LoC where practical.
- Seams: define clear interfaces to decouple handlers/services from persistence and external clients.
- Tests: use table-driven tests for parsing/aggregation with seams/mocks.

## CLI Flag Helpers and Conventions

The `internal/cli/flags` package provides small, reusable helpers for common
flag parsing and validation patterns. These helpers exist to keep individual
CLIs simple and consistent; they do not change behavior of existing commands.

- `ParseDateISO(value string) (time.Time, error)` — parses `YYYY-MM-DD`.
- `ParseRFC3339(value string) (time.Time, error)` — parses RFC3339 timestamps.
- `ParseCSVList(value string) []string` — splits on commas, trims spaces, drops empties.
- `RequireNonEmpty(name, value string) error` — validates required string flags.
- `ParseDurationFlag(value string) (time.Duration, error)` — parses Go duration strings.

Guidelines:
- Prefer using these helpers for new code; when adopting in existing commands,
  ensure error messages remain compatible with current tests and UX.
- Keep CLI parsing functions pure (no I/O); pass in `*flag.FlagSet` and `[]string`
  where possible to enable table-driven tests.

## Logging and Config Conventions

This project uses a single structured logger based on Go's `slog` via the
`internal/logger` package.

- Initialize once at application start (e.g., in `main`):
  ```go
  logger.SetupFromEnv() // sets the global slog default logger
  log := logger.L()
  log.Info("app started")
  ```
- Retrieve the logger in packages with `logger.L()`; prefer structured fields
  (`log.Info("msg", "key", value)`).
- Environment variables:
  - `LOG_FORMAT` = `json` | `text` (default: `json`)
  - `LOG_LEVEL` = `debug` | `info` | `warn` | `error` (default: `info`)

Configuration loading follows `internal/config.Load()`, with many CLIs allowing
environment variables to provide defaults for flags (kept for backwards
compatibility and convenience). Typical patterns seen across `internal/cli`:

- Call `config.Load()` early to ensure config cache readiness where needed.
- Use `os.Getenv(KEY)` to populate default flag values when present (e.g.,
  `GO_APP_OUTPUT_DIR`, `ENABLE_SEQ_FEATURES`, and tool-specific keys). This is
  a convenience only; flags still validate explicitly.

Guidelines:
- Prefer the shared logger over ad-hoc printing; if adopting in existing code,
  match current message text and level to avoid behavior drift.
- Keep CLI flag parsing pure and deterministic; no logging in parsing helpers.
