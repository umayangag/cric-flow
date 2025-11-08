# Plan: Phase 1 — Schema + Parser/Repo + Unit Tests for Fielding Events and Aggregates

## Objective
Implement the first phase of the approved design to extract fielding data from wicket events, persist normalized events, recompute per-player aggregates in `fielding_data`, and cover with unit tests. No backfill or ML export changes in this phase (they come in later phases).

## Scope (Phase 1 only)
- Database
  - Add `fielding_event` table (normalized, idempotent key) with `is_direct_hit`, `assist_role`, and `notes`.
  - Extend existing `fielding_data` with `stumpings` and `runouts_direct_hits` (keep prior columns intact).
- Go ETL (Cricsheet ingest)
  - Parse wicket fielders for kinds: `caught`, `run out`, `stumped` (ignore others for fielding events).
  - Insert `fielding_event` rows during ingest.
  - Recompute `fielding_data` aggregates for the ingested match (catches, run_outs, stumpings, runouts_direct_hits).
- Repository layer
  - Event insert (idempotent) and aggregate recompute routines.
- Unit tests (Go)
  - Parser cases for caught, c&b, runout (1/2 fielders, direct hit), stumped, and non-fielding dismissals.
  - Repo aggregation idempotency and correctness.

## Files to Create
- go-app/migrations/000X_fielding_event.sql
- go-app/migrations/000Y_fielding_data_extend.sql
- go-app/internal/db/repo_fielding_event.go
- go-app/internal/cricsheet/ingest_fielding_testdata.json (fixture for unit tests)
- go-app/internal/cricsheet/ingest_fielding_test.go
- go-app/internal/db/repo_fielding_event_test.go

## Files to Modify
- go-app/internal/cricsheet/cricsheet.go (verify/ensure wicket structure supports needed fields; minimal edits if required)
- go-app/internal/cricsheet/ingest.go (emit events and call recompute at end-of-match ingest)
- go-app/internal/db/repo_fielding.go (extend struct + upsert to include `stumpings`, `runouts_direct_hits`)
- go-app/Makefile (ensure `make migrate` and `make test` targets cover new paths if necessary)

## High-Level Changes by File

### Migrations
1) go-app/migrations/000X_fielding_event.sql
```
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

2) go-app/migrations/000Y_fielding_data_extend.sql
```
ALTER TABLE fielding_data
  ADD COLUMN IF NOT EXISTS stumpings INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS runouts_direct_hits INT NOT NULL DEFAULT 0;
-- (Optional generated column to be decided later; Phase 1 will compute at query/Go layer)
-- ALTER TABLE fielding_data
--   ADD COLUMN IF NOT EXISTS fielding_involvements INT GENERATED ALWAYS AS (catches + run_outs + stumpings) STORED;
```

### Repository Layer
- go-app/internal/db/repo_fielding_event.go
  - Types
    - `type FieldingEvent struct { MatchID int64; Innings, Over, Ball int; BatterOutID, FielderID, BowlerID *int64; Kind, AssistRole string; IsDirectHit bool; Notes *string }`
  - Interfaces
    - `InsertFieldingEvent(ctx context.Context, e FieldingEvent) error` — upsert-like insert that ignores conflicts on the unique composite key.
    - `RecomputeFieldingAggregates(ctx context.Context, matchID int64) error` — group by `(match_id, fielder_id)` and compute counts:
      - `catches` where `kind='caught'`
      - `run_outs` where `kind='run_out'`
      - `stumpings` where `kind='stumped'`
      - `runouts_direct_hits` where `kind='run_out' AND is_direct_hit`.
    - Use existing `UpsertFielding(...)` from `repo_fielding.go` extended with the new columns.

- go-app/internal/db/repo_fielding.go
  - Extend existing `Fielding` struct and `UpsertFielding` SQL to include `stumpings` and `runouts_direct_hits` (retain `catches`, `run_outs`, `dropped_catches`, `missed_run_outs`).

### Ingest
- go-app/internal/cricsheet/cricsheet.go
  - Confirm fields on `Wicket` include `Kind`, `Fielders []string` or equivalent, and availability of over/ball/innings context and bowler; add minimal fields if missing (no broad refactor).

- go-app/internal/cricsheet/ingest.go
  - During wicket handling:
    - Map ingest `kind` to `fielding_event.kind`:
      - caught → one event. If caught-and-bowled, use bowler as fielder.
      - run out → one or two fielders → emit one event per fielder with `assist_role='assist'`. If textual indicator shows direct hit, set `is_direct_hit=true` for the relevant fielder.
      - stumped → keeper credited with `assist_role='keeper'`.
    - Resolve names → `player_id` using existing player resolution utility.
    - Call `InsertFieldingEvent` for each event.
  - At the end of match ingest (within/after transaction), call `RecomputeFieldingAggregates(matchID)`.

## Tests (Go)
- go-app/internal/cricsheet/ingest_fielding_test.go
  - Table-driven tests using `ingest_fielding_testdata.json` covering:
    - Caught (single fielder)
    - Caught-and-bowled (bowler as fielder)
    - Run out (one fielder)
    - Run out (two fielders)
    - Run out (direct hit)
    - Stumped (keeper)
    - LBW/Bowled (no fielding events)
  - Assert: produced `FieldingEvent` slices match expectations.

- go-app/internal/db/repo_fielding_event_test.go
  - Insert duplicate events to confirm uniqueness (no duplicates).
  - Populate a few events, run `RecomputeFieldingAggregates`, and assert `fielding_data` rows contain correct counts for `catches`, `run_outs`, `stumpings`, and `runouts_direct_hits`.

## Acceptance Criteria (Phase 1)
1) Migrations create `fielding_event` and extend `fielding_data` with `stumpings` and `runouts_direct_hits`.
2) Ingest emits correct `fielding_event` rows for fielding-related dismissals.
3) Aggregation recompute populates/updates `fielding_data` accurately and idempotently per match.
4) Unit tests for parser and repository pass locally and in CI.

## Verification Commands
- Migrations
  - `cd go-app && make migrate` (or the project’s migration runner if different)
- Unit tests (Go)
  - `cd go-app && make test` (or `go test ./...`)
- Manual spot check (after ingesting a small sample via existing importer)
  - `SELECT * FROM fielding_event WHERE match_id=<id> ORDER BY innings, over, ball;`
  - `SELECT * FROM fielding_data WHERE match_id=<id> ORDER BY player_id;`

## Branching & Commits
- Create feature branch: `feat/phase1-fielding-events`
- Conventional commits for this phase:
  - `feat(db): add fielding_event table and extend fielding_data with stumpings and direct hits`
  - `feat(ingest): emit fielding events and recompute fielding aggregates`
  - `test(go): add unit tests for fielding event parsing and aggregation`

## Notes & Edge Cases
- Allow `NULL fielder_id` for unresolved names; exclude from aggregates.
- Caught-and-bowled counts as a catch for bowler (fielder_id=bowler_id).
- Two-fielder runouts create two events; both counted toward `run_outs`. Only mark `is_direct_hit=true` for the fielder indicated as direct hitter (if text available).
- No events for LBW/Bowled/Hit-wicket.
