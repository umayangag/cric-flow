# Plan: Fielding Pipeline — Phase 2 Finalization (Fixtures + Integration Tests + Backfill polish)

## Objective
Complete Phase 2 by adding a deterministic Cricsheet fixture, end-to-end integration tests, and backfill polish so the five fielding columns are validated across exporters. Ensure Makefile targets and README guidance are updated.

## Scope (Phase 2 Final)
- Test fixtures and integration tests (Go)
- Backfill CLI minor polish (help/usage notes if needed)
- Makefile targets for convenience
- README updates for commands and dataset schema additions

## Files to Create or Modify

Create:
- `tests/fixtures/cricsheet/sample_fielding_match.json`
  - Minimal Cricsheet v1.1 JSON containing:
    - One `caught` dismissal
    - One `run out` with two fielders
    - One `stumped`
    - One `lbw` (no fielding event)

- `go-app/integration/export_fielding_integration_test.go`
  - Spins up test DB (using existing db.Connect with test env), runs migrations, ingests the fixture via `cricsheet.ImportDir`, and verifies:
    - `fielding_event` rows contents (kinds, assist_role, direct hit default false)
    - `fielding_data` aggregates per `(match_id, player_id)`
    - Exporters produce CSVs that include: `catches`, `run_outs`, `stumpings`, `runouts_direct_hits`, `fielding_involvements`

Modify:
- `go-app/Makefile`
  - Add convenient targets (if missing):
    - `test-int`: runs only integration tests in `go-app/integration`
    - `export`: wrapper for `cmd/export-dataset` with default `GO_APP_OUTPUT_DIR=../output/go-app`
    - `backfill-fielding`: wrapper to run the backfill CLI with `--all`

- `README.md`
  - Document new fielding columns and commands for backfill and export verification.

Optional polish (if required by tests):
- `go-app/cmd/backfill-fielding/main.go`
  - Improve `--force` help text (kept reserved) + clearer progress logging.

## High-Level Changes per File

- `tests/fixtures/cricsheet/sample_fielding_match.json`
  - Small, deterministic (few overs) JSON consistent with types in `internal/cricsheet/cricsheet.go`.

- `go-app/integration/export_fielding_integration_test.go`
  - Setup: create temp DB (respect env `POSTGRES_*`), run `db.RunMigrations` on `go-app/migrations`.
  - Ingest: call `cricsheet.ImportDir(ctx, "../../tests/fixtures/cricsheet", &Options{})`.
  - Assert with SQL:
    - Fielding events: counts by kind and assist_role; ensure no rows for lbw/bowled.
    - Fielding aggregates: per player counts for catches, run_outs, stumpings, runouts_direct_hits (0), and computed involvements.
  - Export:
    - Run exporters: unified and per-format (choose a format used by fixture, e.g., `T20`).
    - Load CSVs and assert header contains five fielding columns and selected row values are correct.

- `go-app/Makefile`
  - `test-int`: `go test ./integration -v` (or package path as created).
  - `export`: `go run ./cmd/export-dataset --all-formats --out $(GO_APP_OUTPUT_DIR)`.
  - `backfill-fielding`: `go run ./cmd/backfill-fielding --all`.

- `README.md`
  - Add examples:
    - Apply migrations
    - Ingest fixtures
    - Backfill fielding
    - Export datasets
    - Where to find resulting CSVs and columns added

## Tests to Add
- Integration tests (Go):
  1) Ingest + Aggregates
     - Verify `fielding_event` contents (kinds + assist roles; `is_direct_hit=false` by default).
     - Verify `fielding_data` aggregates per fielder.
  2) Exporter CSVs
     - Verify fielding columns exist and have expected values for at least two players (one with events, one without → zeros).

Note: Direct-hit detection remains default false; do not assert true in tests.

## Acceptance Criteria
1) Fixture ingests successfully and produces expected `fielding_event` and `fielding_data` rows.
2) Backfill CLI recomputes aggregates idempotently for the match present in the fixture (no duplicates).
3) All exporters (legacy, per-format, unified, inference) include the five fielding columns; CSVs validate in tests for presence and sample values.
4) Makefile targets enable simple local runs; README documents usage.

## Verification Commands
- Migrations:
  - `cd go-app && make migrate`
- Ingest fixture:
  - `go run ./go-app/cmd/cricsheet-importer --dir tests/fixtures/cricsheet`
- Backfill fielding:
  - `go run ./go-app/cmd/backfill-fielding --all`
- Export datasets:
  - `GO_APP_OUTPUT_DIR=output/go-app go run ./go-app/cmd/export-dataset --all-formats`
- Spot-check SQL:
  - `SELECT kind, assist_role, is_direct_hit, COUNT(*) FROM fielding_event GROUP BY 1,2,3 ORDER BY 1,2,3;`
  - `SELECT * FROM fielding_data ORDER BY match_id, player_id;`
- Tests:
  - `cd go-app && go test ./...`

## Branching & Commits
- Branch: `feat/phase2-fielding-export`
- Conventional commits (examples):
  - `test(integration): add cricsheet fixture and end-to-end tests for fielding`
  - `chore(make): add convenience targets for fielding backfill and exports`
  - `docs(readme): document fielding columns and commands`

## Risks & Mitigations
- Name resolution differences: ensure fixture uses consistent player names; unresolved names will produce NULL `fielder_id` and be excluded from aggregates.
- DB availability in CI: guard tests with environment variables or use docker-compose service; skip integration tests if DB is unavailable (mark with build tag or conditional if needed).
