# Plan: Add Fielding Events + Aggregates (with direct-hit) and Integrate with ML

## Overview
We will add end-to-end support for fielding data derived from wicket events (caught, run out, stumped). This includes:
- A normalized `fielding_event` table capturing all fielding-related wicket events with rich attributes (including `is_direct_hit`).
- Continued use and extension of the existing `fielding_data` table as the per-player-per-match aggregate store, adding `stumpings` and `runouts_direct_hits` and a clean, deterministic aggregation flow from events.
- Updates to the Go ETL (Cricsheet ingest + repositories) to populate events and recompute aggregates idempotently.
- Export changes so ML datasets include fielding metrics.
- ML service schema updates to accept and use the new columns (even if not immediately leveraged in modeling).

We will use a clean, auditable approach: Events → Aggregation → Export. Extra attributes (like direct-hit) are stored in events and also summarized in aggregates (`runouts_direct_hits`).

---

## Scope and Phases

- Phase 1: Schema + Parser/Repo + Unit tests
- Phase 2: Backfill + Export + Integration tests
- Phase 3: ML-service updates + tests
- Phase 4: Docs + CI adjustments

We will work on a feature branch (e.g., `feat/fielding-events-and-aggregates`) with small PRs per phase.

---

## Files to Create or Modify

- New (Go, DB):
  - `go-app/migrations/000X_fielding_event.sql` — create `fielding_event` table and indices.
  - `go-app/migrations/000Y_fielding_data_extend.sql` — extend `fielding_data` with `stumpings`, `runouts_direct_hits` (and optionally generated `fielding_involvements`).
  - `go-app/internal/db/repo_fielding_event.go` — insert/query events; recompute aggregates for a match.
  - `go-app/cmd/backfill-fielding/main.go` — backfill command to derive events+aggregates for historical matches.

- Modify (Go):
  - `go-app/internal/cricsheet/cricsheet.go` — ensure `Wicket` model includes needed fields (kind, fielders, bowler, over/ball, inning, batter out).
  - `go-app/internal/cricsheet/ingest.go` — emit `fielding_event` entries during wicket processing; then recompute `fielding_data` for the match.
  - `go-app/internal/db/repo_fielding.go` — extend structs and SQL to support `stumpings` and `runouts_direct_hits` in upsert; preserve existing columns.
  - `go-app/cmd/export-dataset/main.go` — join `fielding_data` for each player/match and append new columns to the CSV.
  - `go-app/Makefile` — add targets for migrations/backfill if needed.

- Modify (Python, ML):
  - `ml-service/ml/dataset_definitions.py` — include new fielding columns.
  - `ml-service/ml/queries.py` or CSV loader locations — accept/forward new columns.
  - Potential training scripts to accept the extended dataset (no modeling changes required immediately).

- Docs:
  - `README.md` — describe new tables/columns, commands, and dataset fields.
  - `docs/` — any schema diagrams or parity reports to be updated, e.g., `docs/schema_parity_report.md`.

---

## High-Level Changes per File

- `go-app/migrations/000X_fielding_event.sql`:
  - Create table `fielding_event` (PostgreSQL syntax assumed; adjust as needed for target DB):
    ```sql
    CREATE TABLE IF NOT EXISTS fielding_event (
      id BIGSERIAL PRIMARY KEY,
      match_id BIGINT NOT NULL,
      innings SMALLINT NOT NULL,
      over SMALLINT NOT NULL,
      ball SMALLINT NOT NULL,
      batter_out_id BIGINT NULL,
      fielder_id BIGINT NULL,
      bowler_id BIGINT NULL,
      kind TEXT NOT NULL CHECK (kind IN ('caught','run_out','stumped','other')),
      assist_role TEXT NULL CHECK (assist_role IN ('primary','assist','keeper')),
      is_direct_hit BOOLEAN NOT NULL DEFAULT FALSE,
      notes TEXT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      UNIQUE (match_id, innings, over, ball, fielder_id, kind, COALESCE(assist_role, ''))
    );

    CREATE INDEX IF NOT EXISTS idx_fielding_event_match ON fielding_event(match_id);
    CREATE INDEX IF NOT EXISTS idx_fielding_event_player ON fielding_event(fielder_id);
    ```

- `go-app/migrations/000Y_fielding_data_extend.sql`:
  - Extend existing `fielding_data` (found in `0001_init.sql`) with additional columns.
    ```sql
    ALTER TABLE fielding_data
      ADD COLUMN IF NOT EXISTS stumpings INT NOT NULL DEFAULT 0,
      ADD COLUMN IF NOT EXISTS runouts_direct_hits INT NOT NULL DEFAULT 0;
    -- Optional generated column if DB supports it; otherwise compute in queries/export
    -- ALTER TABLE fielding_data
    --   ADD COLUMN IF NOT EXISTS fielding_involvements INT GENERATED ALWAYS AS (catches + run_outs + stumpings) STORED;
    ```

- `go-app/internal/db/repo_fielding_event.go`:
  - Types: `FieldingEvent` struct.
  - Methods:
    - `InsertFieldingEvent(ctx context.Context, e FieldingEvent) error` (idempotent on unique key)
    - `RecomputeFieldingAggregates(ctx context.Context, matchID int64) error` — compute counts grouped by `(match_id, fielder_id)` and upsert into `fielding_data`:
      - `catches`: `kind='caught'`
      - `run_outs`: `kind='run_out'`
      - `stumpings`: `kind='stumped'`
      - `runouts_direct_hits`: `kind='run_out' AND is_direct_hit=TRUE`

- `go-app/internal/cricsheet/ingest.go`:
  - While processing `Wicket`s, parse the dismissal text/fields to determine fielders and attributes:
    - caught: 1 fielder (or bowler if "c & b").
    - run out: 1–2 fielders; produce two events if two names; set `is_direct_hit=TRUE` if text indicates direct hit.
    - stumped: wicketkeeper credited with `assist_role='keeper'`.
  - Resolve player names → `player_id` using existing lookup logic.
  - Insert events; after match ingest, call `RecomputeFieldingAggregates`.

- `go-app/internal/db/repo_fielding.go`:
  - Extend `Fielding` struct and `UpsertFielding` SQL to include `stumpings` and `runouts_direct_hits` (retain existing columns: `catches`, `run_outs`, `dropped_catches`, `missed_run_outs`).

- `go-app/cmd/export-dataset/main.go`:
  - LEFT JOIN `fielding_data` for `(match_id, player_id)` and emit columns:
    - `catches`, `run_outs`, `stumpings`, `runouts_direct_hits`, and computed `fielding_involvements` (either from generated column or `catches + run_outs + stumpings`).
  - Default zeros if no row in `fielding_data`.

- `ml-service/ml/dataset_definitions.py` and loaders:
  - Add new feature columns (as raw integers) to the dataset definitions so models can optionally use them now or later.

- `go-app/cmd/backfill-fielding/main.go`:
  - Iterate matches, re-derive and insert `fielding_event` (idempotent) and recompute `fielding_data` per match.

---

## Tests to Add/Update

- Go unit tests:
  - `internal/cricsheet/ingest_test.go`: parsing cases produce correct `FieldingEvent`s
    - caught (single fielder)
    - caught & bowled (bowler as fielder, kind='caught')
    - run out (one fielder)
    - run out (two fielders)
    - run out with direct hit (set `is_direct_hit=TRUE` for the relevant fielder)
    - stumped (assign keeper with `assist_role='keeper'`)
    - lbw / bowled → no fielding events
  - `internal/db/repo_fielding_event_test.go`: idempotent insert; aggregate recompute correctness (counts incl. `runouts_direct_hits`).

- Go integration tests:
  - Sample Cricsheet JSON (tiny fixture) ingested end-to-end → assert `fielding_event` rows and `fielding_data` counts.
  - Export dataset contains expected fielding columns and values for chosen players.

- Python (ml-service):
  - Update dataset schema tests to include new columns; ensure loaders accept them and maintain array/tensor shapes.

---

## Acceptance Criteria

1. `fielding_event` table exists with uniqueness on `(match_id, innings, over, ball, fielder_id, kind, COALESCE(assist_role,''))` and is populated during ingest.
2. `fielding_data` is extended with `stumpings` and `runouts_direct_hits` and is recomputed deterministically from events per match.
3. Idempotency: re-ingesting the same match does not create duplicate events or change aggregates unexpectedly.
4. Exported dataset includes columns: `catches`, `run_outs`, `stumpings`, `runouts_direct_hits`, and `fielding_involvements` with correct values; players with no fielding events get zeros.
5. ML-service reads datasets with new columns without errors; all tests pass in CI.

---

## Verification Commands

- Migrations:
  - `cd go-app && make migrate` (or equivalent migration runner)
- Ingest + recompute (sample):
  - `go run ./go-app/cmd/etl-importer --source=cricsheet --match <match_id>`
  - (automatically recomputes fielding aggregates post-ingest)
- Backfill all:
  - `go run ./go-app/cmd/backfill-fielding --all` (supports `--force` to rebuild)
- Export with fielding columns:
  - `go run ./go-app/cmd/export-dataset --out output/dataset_with_fielding.csv`
- Spot-check SQL:
  - `SELECT * FROM fielding_event WHERE match_id=<id> ORDER BY innings, over, ball;`
  - `SELECT * FROM fielding_data WHERE match_id=<id> ORDER BY fielding_involvements DESC;`
- Tests:
  - Go: `cd go-app && make test` (or `go test ./...`)
  - Python: `cd ml-service && pytest -q`

---

## Branching, Commits, PR

- Create branch: `feat/fielding-events-and-aggregates`.
- Conventional Commits per phase, e.g.:
  - `feat(db): add fielding_event table and extend fielding_data with stumpings and direct hits`
  - `feat(ingest): emit fielding events and recompute aggregates`
  - `feat(export): include fielding columns in dataset export`
  - `feat(ml): accept fielding columns in dataset definitions`
  - `test(go,ml): add unit and integration tests for fielding pipeline`
- Open small PRs per phase; request review.

---

## Notes / Edge Cases

- Caught-and-bowled: store as `kind='caught'` with `fielder_id=bowler_id`.
- Run-outs with two fielders: create two events; both count towards `run_outs`. Mark direct hit when indicated (Cricsheet text/metadata); only count direct-hit in `runouts_direct_hits` for that fielder.
- Non-fielding dismissals (lbw, bowled, hit wicket): no events created.
- Unknown player mapping: allow `NULL fielder_id` in events; exclude `NULL` from aggregates; log warnings.
- Additional raw details may be placed into `notes` for future use; not consumed by ML currently.
