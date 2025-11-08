# Plan: Fielding Pipeline — Phase 2 Finalization and Handoff to Phase 3

## Objective
Finalize Phase 2 by validating exporter outputs end-to-end using a deterministic Cricsheet fixture, polishing the backfill CLI UX, and ensuring exporter parity across all paths. Prepare the groundwork and scope for Phase 3 (ML-service schema updates) without making ML-service changes in this phase.

## Scope (Phase 2 Finalization)
- Tests (Go integration): Run exporter(s) and assert presence/order/values of the five fielding columns.
- Backfill CLI: Clarify `--force` behavior in help; add progress logging. No behavioral change to aggregation recompute.
- Exporter parity: Verify headers and SELECT column orders across all exporter paths (legacy, per-format, unified, inference). Add a guard to prevent header regressions.
- Documentation: Minimal README updates for new commands/columns (optional, can shift to Phase 4 if needed).

## Files to Create or Modify
- Modify (Go tests):
  - `go-app/integration/export_fielding_integration_test.go`
    - Enhance to execute `go run ./cmd/export-dataset` (per-format or unified) to a temp directory.
    - Load emitted CSV(s); assert headers include, in order: `catches`, `run_outs`, `stumpings`, `runouts_direct_hits`, `fielding_involvements`.
    - Assert at least one row contains expected non-zero counts per the fixture (e.g., `catches=1`, `run_outs=2`, `stumpings=1`).

- Modify (Go CLI):
  - `go-app/cmd/backfill-fielding/main.go`
    - Update flag help to clarify `--force` is reserved (no-op currently).
    - Add log lines summarizing per-match updated aggregates (counts) after `RecomputeFieldingAggregates` (best-effort; no query required, just info that recompute completed).

- Modify (Go):
  - `go-app/cmd/export-dataset/main.go`
    - Verify all exporter paths already include the five fielding columns and identical order.
    - Optionally add a small internal check that header sizes match expected constants.

- Modify (Makefile):
  - `go-app/Makefile`
    - Ensure `make test` runs the integration test (already configured). Keep `test-int` target for convenience.

- Fixture (already present):
  - `tests/fixtures/cricsheet/sample_fielding_match.json`

- Docs (optional for Phase 2; otherwise Phase 4):
  - `README.md` — Add a short section describing exporter fielding columns and backfill usage.

## High-Level Changes
- Integration test will:
  1) Start a test DB, run `db.RunMigrations` on `go-app/migrations`.
  2) Ingest the fixture via `cricsheet.ImportDir`.
  3) Run exporter with `--all-formats` (or choose one format available in fixture) into a temp directory.
  4) Read CSV(s) and assert header contains the five fielding columns in the expected order.
  5) Identify at least one row (player) with expected values per the fixture and assert numeric equality for the five columns.

- Backfill CLI help clarifies `--force` is reserved; add periodic progress logs and a final `ok` per match.

- Exporter paths parity (verify):
  - Legacy: `exportBattingLegacy`, `exportBowlingLegacy` (transitively via `exportBatting`/`exportBowling` if used).
  - Per-format (training): `exportBattingFormat`, `exportBowlingFormat`.
  - Inference: `exportBattingFormatInference`, `exportBowlingFormatInference`.
  - Unified: `exportBattingUnified`, `exportBowlingUnified`.

## Tests to Add/Enhance
- `go-app/integration/export_fielding_integration_test.go`
  - Assert aggregator totals based on fixture remain: e.g., caught=1, run_out=2 (two fielders), stumped=1; `runouts_direct_hits=0` unless the fixture marks one.
  - After exporter run, assert CSV header includes the five columns; check at least one player row’s values match fixture-derived expectations.

## Acceptance Criteria
1) Integration test runs exporter and validates fielding columns’ presence, order, and values in emitted CSV(s).
2) Backfill CLI prints clear progress and clarifies `--force` help text.
3) Exporter column parity across all paths is verified and guarded by tests.
4) `make test` passes locally and in CI.

## Verification Commands
- Run migrations:
  - `cd go-app && make migrate`
- Ingest the fixture and run tests:
  - `cd go-app && make test-int`
  - Or full suite: `cd go-app && make test`
- Run exporter manually (writing to default output dir):
  - `cd go-app && make export-dataset`
- Backfill all matches (idempotent):
  - `cd go-app && make backfill-fielding`

## Phase 3 (Preview) — ML-service schema updates
- Modify `ml-service/ml/dataset_definitions.py` and loaders to accept:
  - `catches`, `run_outs`, `stumpings`, `runouts_direct_hits`, `fielding_involvements`
- Tests under `ml-service/tests/` to validate schema and zero-fill behavior for older CSVs.
- CI: `.github/workflows/ml-service-ci.yml` already validates exports if present.

## Branching & PRs
- Continue working on branch: `feat/phase2-fielding-export` for Phase 2 finalization.
- Open a focused PR once integration test asserts exporter columns/values and all checks pass.
- Create `feat/phase3-ml-fielding` for ML-service schema updates next.
