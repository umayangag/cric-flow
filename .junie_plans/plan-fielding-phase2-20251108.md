# Plan: Fielding Pipeline — Phase 2 (Backfill + Export + Integration Tests)

## Overview
Phase 2 continues from Phase 1 (schema + ingest wiring) and delivers:
- Historical backfill of `fielding_event` and recomputed aggregates in `fielding_data`.
- Dataset exporter updated to include fielding columns for ML.
- Integration tests to validate the end-to-end pipeline with a small deterministic Cricsheet fixture.

This phase remains backward-compatible and idempotent.

---

## Files to Create or Modify

- Create (Go CLI):
  - `go-app/cmd/backfill-fielding/main.go`
    - Flags: `--all`, `--match <id>`, `--force` (rebuild events and aggregates even if present), `--batch-size`.
    - Logic: iterate matches → derive/insert `fielding_event` idempotently → `RecomputeFieldingAggregates(matchID)`.

- Modify (Go):
  - `go-app/cmd/export-dataset/main.go`
    - Left join `fielding_data` on `(match_id, player_id)`; include new columns and defaults.
    - New CSV columns (appended to the end unless schema dictates otherwise):
      - `catches` (int)
      - `run_outs` (int)
      - `stumpings` (int)
      - `runouts_direct_hits` (int)
      - `fielding_involvements` (int; computed as `catches + run_outs + stumpings` if not generated in DB)

- Modify (Go, repositories if needed):
  - `go-app/internal/db/repo_fielding.go`
    - Add helper getters for reading `fielding_data` during export if exporter uses repos (optional depending on exporter style).

- Tests (Go):
  - `go-app/internal/cricsheet/ingest_integration_test.go` (or under `go-app/internal/cricsheet/`):
    - Ingest a tiny Cricsheet JSON fixture.
    - Assert rows in `fielding_event` and aggregates in `fielding_data` (including direct-hit = 0 if not detectable yet).
  - `go-app/cmd/export-dataset/export_integration_test.go` (or package-local):
    - Run exporter against the ingested match and verify CSV contains the new fielding columns with correct values.

- Test fixtures:
  - `tests/fixtures/cricsheet/sample_fielding_match.json` — minimal match JSON with at least:
    - One `caught` dismissal
    - One `run out` with two fielders
    - One `stumped`
    - One `lbw` or `bowled` (no fielding event)

- Docs:
  - `README.md` — update usage for backfill command and exporter fields.

---

## High-Level Changes per File

- `go-app/cmd/backfill-fielding/main.go`
  - Connect DB → enumerate matches from `match_details` or from available Cricsheet source (prefer DB since ingest already loaded matches).
  - For each match:
    - Optionally remove existing `fielding_event` rows when `--force`.
    - Re-parse source or reuse existing wicket data via the ingest path; safest is to re-run `ImportMatchFile` in a mode that only emits fielding events, but to avoid double-writing batting/bowling, prefer a dedicated event-derivation routine that walks innings deliveries and only writes events.
    - Call `db.RecomputeFieldingAggregates(ctx, matchID)`.
  - Logging with progress and per-match summary counts.

- `go-app/cmd/export-dataset/main.go`
  - Add LEFT JOIN to `fielding_data`; nulls → zeros.
  - Append columns and ensure existing downstream consumers remain unchanged (column order documented here and in ml-service).

- Tests
  - Spin up a temporary test DB (or use the project’s test harness) → run migrations → ingest fixture → validate.

---

## Tests to Add

- Integration: Ingest + Aggregates
  - Input: `sample_fielding_match.json`.
  - Assertions:
    - `fielding_event` contains:
      - 1+ `caught` rows with the correct fielder(s)
      - 2 `run_out` rows (two fielders credited)
      - 1 `stumped` row with `assist_role='keeper'`
      - No events for `lbw`/`bowled`
    - `fielding_data` counts per `(match_id, player_id)` reflect the above and `runouts_direct_hits=0` unless flagged.

- Integration: Exporter
  - After ingest, run the exporter and parse the generated CSV; assert added columns exist and selected players have the expected counts and zeros for players without events.

Note: Direct-hit detection remains default false until Phase 5 (optional heuristics). Tests should not depend on it being true.

---

## Acceptance Criteria

1) `backfill-fielding` command exists and can process `--all` or a single `--match <id>`; it is idempotent and supports `--force`.
2) Running backfill after a fresh ingest produces `fielding_event` rows and updates `fielding_data` aggregates without duplications.
3) Exporter includes the new fielding columns with correct non-negative integers and defaults to zeros when missing.
4) Integration tests pass locally (`go test ./...`), verifying end-to-end behavior on the sample fixture.

---

## Verification Commands

- Migrations (if not yet applied):
  - `cd go-app && make migrate` (or) `go run ./go-app/cmd/tools/migrate -dir migrations`

- Ingest sample fixture:
  - `go run ./go-app/cmd/cricsheet-importer --dir tests/fixtures/cricsheet`

- Backfill all matches:
  - `go run ./go-app/cmd/backfill-fielding --all`
  - With force rebuild:
  - `go run ./go-app/cmd/backfill-fielding --all --force`

- Export dataset with fielding columns:
  - `go run ./go-app/cmd/export-dataset --out output/dataset_with_fielding.csv`

- Spot-check SQL:
  - `SELECT kind, assist_role, is_direct_hit, COUNT(*) FROM fielding_event GROUP BY 1,2,3 ORDER BY 1,2,3;`
  - `SELECT * FROM fielding_data ORDER BY match_id, player_id;`

- Tests:
  - `cd go-app && go test ./...`

---

## Branching & PRs

- Branch: `feat/phase2-fielding-export`
- Commits:
  - `feat(backfill): add backfill-fielding command`
  - `feat(export): append fielding columns to dataset export`
  - `test(integration): add cricsheet fixture and end-to-end tests for fielding`
  - `docs: update README with backfill and export columns`

---

## Notes / Risks / Mitigations

- Idempotency: Backfill must rely on `fielding_event` unique key to avoid duplicates; use `ON CONFLICT DO NOTHING` inserts.
- Performance: For large datasets, process matches in batches; `--batch-size` and periodic logging.
- Data parity: Ensure player name resolution is consistent with ingest; unresolved names log warnings and are excluded from aggregates (NULL fielder_id).
- Backward compatibility: Exporter fills zeros when `fielding_data` rows are absent.
