# Plan: Phase 2 Implementation — Backfill CLI polish + Exporter (all paths) + Integration Tests

## Objective
Complete Phase 2 by:
- Extending ALL exporter paths (legacy, per-format, unified, and inference) to include fielding aggregates.
- Finalizing the backfill CLI behavior (iterate matches and recompute aggregates idempotently; `--force` reserved for Phase 2.5).
- Adding deterministic integration tests with a tiny Cricsheet fixture to validate end-to-end ingest → events → `fielding_data` aggregates → CSV export with fielding columns.

This phase does not change ML-service yet (Phase 3). It ensures exported CSVs have the new fielding columns consistently everywhere.

---

## Files to Create or Modify

- Modify (Go):
  - `go-app/cmd/export-dataset/main.go`
    - Extend SQL/headers for:
      - `exportBattingFormat`, `exportBowlingFormat` (training, per-format)
      - `exportBattingFormatInference`, `exportBowlingFormatInference` (inputs-only, per-format)
      - `exportBattingUnified`, `exportBowlingUnified` (single merged CSVs across formats)
    - Ensure each path joins `fielding_data` and outputs the five columns:
      - `catches`, `run_outs`, `stumpings`, `runouts_direct_hits`, and `fielding_involvements`.
    - Default zeros when no `fielding_data` row exists.
  - (if any helper is shared within `main.go`, adjust scanning column counts accordingly.)

- Create (Go test fixtures + tests):
  - `tests/fixtures/cricsheet/sample_fielding_match.json` — minimal match encoding:
    - Includes examples for: caught, caught-and-bowled, run out (1 fielder), run out (2 fielders), stumped, and a non-fielding dismissal.
  - `go-app/integration/export_fielding_integration_test.go` — integration test that:
    - Boots a test DB (using existing DB helpers), runs migrations.
    - Ingests the `sample_fielding_match.json` via `cricsheet.ImportMatchFile`.
    - Calls exporter functions (legacy OR per-format path) and writes to a temp dir.
    - Loads the CSV(s) and asserts fielding columns and values for known players.

- Modify (Go):
  - `go-app/Makefile`
    - Ensure `make test` picks up integration tests.
    - Add a convenience target to run the exporter with a temp output dir (optional).

- Docs:
  - Update `README.md` usage snippets for exporter to mention new fielding columns (optional in this phase; can be in Phase 4).

---

## High-Level Changes per File

### go-app/cmd/export-dataset/main.go
- For each exporter path:
  - LEFT JOIN `fielding_data fd ON fd.match_id = <base>.match_id AND fd.player_id = <base>.player_id`.
  - SELECT columns:
    - `COALESCE(fd.catches,0) AS catches`
    - `COALESCE(fd.run_outs,0) AS run_outs`
    - `COALESCE(fd.stumpings,0) AS stumpings`
    - `COALESCE(fd.runouts_direct_hits,0) AS runouts_direct_hits`
    - `COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0) + COALESCE(fd.stumpings,0) AS fielding_involvements`
  - Append headers in the same order for each CSV.
  - Adjust row scanning (`scanRow`) counts to reflect added columns.
- Paths to ensure parity:
  - Legacy combined: already done (verify and keep consistent).
  - Per-format training: `exportBattingFormat`, `exportBowlingFormat`.
  - Inference inputs-only: `exportBattingFormatInference`, `exportBowlingFormatInference` — include fielding columns too, or document as inputs; if inference schema expects them, include; otherwise keep present but acceptable as zeros if unavailable.
  - Unified: `exportBattingUnified`, `exportBowlingUnified` — add the same join and columns at the end of the select list and header.

### tests/fixtures/cricsheet/sample_fielding_match.json
- Craft a single short match covering dismissals:
  - Caught by `FielderA`.
  - Caught-and-bowled by `BowlerB` (credited as catch).
  - Run out by `FielderC` (single assist).
  - Run out by `FielderC` and `FielderD` (double assist).
  - Stumped by `KeeperE`.
  - LBW (no fielding event).

### go-app/integration/export_fielding_integration_test.go
- Steps:
  1. Start an ephemeral Postgres (or use test container setup already in repo) and run migrations via `db.RunMigrations`.
  2. Call `cricsheet.ImportMatchFile` on the fixture to populate `fielding_event` and recompute `fielding_data`.
  3. Invoke exporter function(s) to write CSV(s) to a temp dir.
  4. Parse CSV(s) and assert rows for known players contain the expected counts for the five fielding columns.
- Keep assertions deterministic and small. No network calls.

### go-app/Makefile
- Ensure `go test ./...` runs integration tests under `go-app/integration/`.
- Add `test-integration` phony target if needed.

---

## Tests and Expected Assertions

- Aggregates for the sample fixture (example expectations; adjust to the exact names used in the JSON fixture):
  - `FielderA`: catches = 1
  - `BowlerB`: catches = 1 (caught-and-bowled)
  - `FielderC`: run_outs = 2 (one single + one shared); `runouts_direct_hits` depends on data; if unspecified, 0.
  - `FielderD`: run_outs = 1
  - `KeeperE`: stumpings = 1
  - All: `fielding_involvements = catches + run_outs + stumpings`

- CSVs produced must include columns (at tail):
  - `catches, run_outs, stumpings, runouts_direct_hits, fielding_involvements`

---

## Acceptance Criteria
1) All exporter paths (legacy, per-format, unified, inference) include and correctly populate the five fielding columns.
2) Backfill CLI iterates matches and recomputes aggregates idempotently; `--force` remains no-op in this phase (documented).
3) Integration test ingests the fixture and validates the exported CSV fielding values for known players.
4) `make test` passes locally and in CI.

---

## Verification Commands
- Migrations:
  - `cd go-app && make migrate`
- Ingest and export (manual):
  - `go run ./go-app/cmd/export-dataset --out output/go-app --all-formats`
  - `go run ./go-app/cmd/export-dataset --out output/go-app --all-formats --inference-only`
  - `go run ./go-app/cmd/export-dataset --out output/go-app --unified`
- Backfill (manual):
  - `go run ./go-app/cmd/backfill-fielding --all`
- Tests:
  - `cd go-app && go test ./...`

---

## Branching & Commits
- Branch: `feat/phase2-fielding-export`
- Conventional commits:
  - `feat(export): add fielding columns to all exporter paths`
  - `test(integration): add fixture and integration test for fielding export`
  - `chore(make): ensure integration tests run via make`

---

## Notes / Edge Cases
- Inference CSVs: If the inference consumer expects the same schema as training inputs, include fielding columns. If not, we still include them at the end to maintain parity; downstream can ignore extra columns. We will document this in Phase 3 when updating the ML service schema.
- Direct hit detection remains false unless explicitly inferred from data; kept 0 in exports unless events set `is_direct_hit=true`.
- If `weather_data` session alignment differs across paths, keep existing semantics; fielding join is independent of weather.