# Go Application (Importer / API / Export)

Go services: Cricsheet import and the HTTP API. Integrates with Postgres and the Python ML
service. go-app owns availability, the fixture and the constraints on an XI; ml-service owns
the models and every feature they read.

**Components:** `cmd/cricsheet-importer`, `cmd/api`, `cmd/migrate`, `cmd/print_canonical`;
`internal/*` (Cricsheet, contracts, repos, ML client).

**Prerequisites:** Go 1.26+, Postgres (defaults: `POSTGRES_HOST=localhost`, `POSTGRES_PORT=5432`, `POSTGRES_DB=cricket_data`, etc.). Override via env.

## Setup and config
- **One-time:** `make init` — installs gofumpt, golines, golangci-lint; ensure `$(go env GOPATH)/bin` on PATH.
- **Config:** Precedence: CLI flags → env → `go-app/config.json` → defaults. See **docs/config-and-data.md** for full schema and examples.

## Common tasks
- **Build / run:** `make build`, `make run-api` (API on :8080)
- **Import:** `make cricsheet-import` (or with `PLACEHOLDERS=1`, `FAIL_FAST=0`). From repo root: `make cricsheet-import`
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
  - Keep runtime bounded (use timeouts); prefer `runWithTimeout` and repo-relative paths.

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

The pipeline is three steps and they are driven from the repo root (`make cricsheet-import`,
`make retrain`, `make reload`) or from the ops console. go-app's own CLIs are the importer and
the migrator; the team-selection and prediction CLIs went with the windowed-form path in P-5,
and the precompute and export commands with their pipelines in P-6.

## Formatting and checks
`make fmt`, `make fmt-check`, `make vet`, `make test`. Inputs: `data/go-app/...`, outputs: `output/go-app`. See root README and **docs/overview.md**.

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

# Enforce the minimum coverage threshold (Makefile default and CI: 74)
make -C go-app coverage-check

# View the total coverage line
make -C go-app coverage-func

# Generate an HTML report at go-app/coverage.html
make -C go-app coverage-html
```

Notes:
- **Coverage-check must run after coverage:** `coverage-check` reads `go-app/coverage.out`. Always run `make -C go-app coverage` first (or use `make go-app-check` / `make ci-go` from repo root, which run coverage then coverage-check in order).
- Scope: Coverage excludes `cmd/*`, `*/mocks`, `internal/models`, `internal/db` and `internal/server` (DB- and HTTP-heavy; covered by integration and API tests). Override with `COVERAGE_PACKAGES=./...` for full scope.
- Gate: `COV_MIN` is 74 in `go-app/Makefile`, `COV_MIN_GO` in the root Makefile and `COV_MIN` in the workflow. All three move together, upward only.
- Convenience: Run a CI-like local check in one go:
  ```
  make -C go-app coverage-ci
  ```

Cricsheet tests use seams (`SetCricsheetDB`, `SetWeatherClient`). See **docs/quality-and-debugging.md**.





---

## Env and layout
See `.env.example`: `LOG_FORMAT`, `LOG_LEVEL`, `ML_SERVICE_URL`, Postgres vars. Logger:
`internal/logger` (slog); config: `internal/config.Load()`, with retired keys refused by name in
`internal/config/retired_keys.go`. See **docs/quality-and-debugging.md** for test standards.
