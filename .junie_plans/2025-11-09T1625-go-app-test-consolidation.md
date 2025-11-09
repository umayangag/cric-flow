# Plan 1 — Consolidate Go unit tests per production file (table-driven) and fix/relocate integration tests

Parent: —
Active path: 1
Created: 2025-11-09 16:25

## Scope
Sweep the entire `go-app` module (both `internal/*` and `cmd/*`). Consolidate unit tests so that for each production file there is a single colocated `<base>_test.go` using table-driven tests. Identify failing integration tests, fix them, and relocate to the most appropriate location (e.g., `integration/` or clearly marked `*_integration_test.go`). Maintain or improve test coverage. Update docs.

## Conventions
- Unit tests: one `*_test.go` per production file, table‑driven structure, no conditional branches inside tests (use helpers instead).
- Integration tests: live under `go-app/integration/` or named `*_integration_test.go` and are not merged with unit tests.
- Helpers: shared fixtures/builders per package go to `helpers_test.go` and `testdata/`.
- Branching: feature branches only (no direct commits to main). Conventional Commits.

## Files to create/modify (high level)
- Create/update table-driven unit test files per production file across:
  - `internal/mlclient`: `client_test.go`, `mlclient_test.go` (consolidated)
  - `internal/config`: `config_test.go`, `load_test.go` (consolidated)
  - `internal/cricsheet`: verify `format_test.go` consolidation; remove/merge stragglers
  - `internal/predictor`: `<base>_test.go`
  - `internal/selection`: `selection_test.go`, `select_team_db_test.go`
  - `cmd/team-predictor`: `csvparse_test.go`, `predict_test.go`, `selector_test.go`, `flags_test.go` (fold `predict_more*.go`, `flags_more_test.go`)
  - `cmd/team-select`: `orch_test.go` (merge `orch_*.go`)
- Integration tests:
  - Audit `go-app/integration/export_fielding_integration_test.go` (currently failing). Fix data/setup and relocate if needed.
  - Any misplaced “integration-like” tests under `cmd/*` or `internal/*` should be moved to `integration/` with proper setup/teardown.
- Documentation: update `go-app/README.md` test conventions; add `CONTRIBUTING.md` section if applicable.

## Detailed Steps

### 1. Inventory & Mapping (Parent: 1)
- Enumerate production files per package and map existing `*_test.go` files to their targets.
- Classify tests as unit vs integration.
- Deliverable: inventory notes (in PR description) and a checklist per package.

### 2. Define Consolidation Structure (Parent: 1)
- For each production file, specify the canonical test file and the table-driven groups (per function).
- Identify shared fixtures to move into `helpers_test.go` and `testdata/`.

### 3. internal/mlclient consolidation (Parent: 1 → 1.1)
- Production: `client.go`, `mlclient.go`.
- Merge tests targeting `client.go` into a single `client_test.go` with subtests:
  - `New()` defaults; env override.
  - `postJSON`: success; non‑2xx error; marshal error; timeouts; header behaviors.
  - Public methods: `PredictBatting`, `PredictBowling` success/error paths.
- Keep `mlclient.go` tests in `mlclient_test.go` (include cancel behavior from `mlclient_cancel_test.go` if it targets that file).
- Extract shared HTTP test server helpers to `helpers_test.go`.

### 4. internal/config consolidation (Parent: 1 → 1.2)
- Map tests from `config_test.go`, `load_test.go` to corresponding production files; merge duplicates.
- Ensure table-driven patterns; move repeated parsing/builders to helpers.

### 5. internal/cricsheet verification (Parent: 1 → 1.3)
- `format_test.go` appears consolidated; ensure no remaining split files (e.g., `format_more_test.go`).
- If present, fold cases into the table and remove extra files.

### 6. internal/predictor & internal/selection consolidation (Parent: 1 → 1.4)
- For each production file (`predictor.go`, `selection.go`, `select_team_db.go`), gather split tests and fold into the single canonical `<base>_test.go` per file.

### 7. cmd/team-predictor consolidation (Parent: 1 → 1.5)
- Map production files (`csvparse.go`, `predict.go`, `selector.go`, `flags.go`).
- Merge:
  - `predict_more*_test.go` → into `predict_test.go` tables.
  - `flags_more_test.go` → into `flags_test.go` tables.
  - Ensure CSV parsing and selector tests are table-driven and colocated.

### 8. cmd/team-select consolidation (Parent: 1 → 1.6)
- Merge `orch_*.go` tests into a single `orch_test.go` with subtests grouped by function.

### 9. Integration tests: fix & relocate (Parent: 1 → 1.7)
- Diagnose failures in `integration/export_fielding_integration_test.go`:
  - Verify setup (DB, migrations, test data). Provide deterministic fixtures under `testdata/`.
  - Fix failing expectations or fragile timing.
  - If test is actually a unit test, relocate to the correct package; otherwise, keep under `integration/`.
- Ensure integration tests are resilient and can run locally via `docker-compose` if needed; gate under an integration tag if appropriate.

### 10. Documentation (Parent: 1 → 1.8)
- Update `go-app/README.md` with:
  - Unit vs integration conventions.
  - Table-driven requirement and per-file colocation rule.
  - Running tests: `make test` or `go test ./...`; optional `-run` for packages.

### 11. Quality gates & CI (Parent: 1 → 1.9)
- Run formatting and linting: `gofmt -s`, `go vet`; `golangci-lint` if configured.
- Ensure `make test` or `go test ./...` is green and coverage not reduced materially.

### 12. Branching & PRs (Parent: 1 → 1.10)
- Create feature branch: `test/consolidate-unit-tests`.
- Submit iterative PRs per package (mlclient → config → cricsheet → predictor/selection → cmd/* → integration). Keep each PR small (1–3 files + tests) and focused.
- Use Conventional Commits messages (`test:`, `refactor(test):`, `docs:`).

## Tests to add/update
- Add table-driven cases that were previously split into multiple files.
- Add missing error-path tests to maintain coverage parity during merges.
- Add deterministic fixtures in `testdata/` for integration tests; ensure offline execution.

## Acceptance criteria
- For every non-integration production file in `go-app`, there is at most one corresponding unit test file `<base>_test.go` with table-driven tests.
- All integration tests are either under `integration/` or correctly marked and pass reliably.
- `go test ./...` (or `make test`) passes; formatting and vet checks pass.
- Documentation updated to reflect conventions.

## Verification commands
- `make test` (if available) or `go test ./...`
- `gofmt -s -l . | grep -v "^$"` (expect no output)
- `go vet ./...`
- `golangci-lint run` (if configured)
- (Integration) `docker compose up -d` (only if required by integration tests), then run tagged tests.
