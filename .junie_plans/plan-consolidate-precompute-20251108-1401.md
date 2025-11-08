# Plan: Consolidate Precomputed Feature Tables (One table per feature, bat/bowl columns)

Owner: Junie
Date: 2025-11-08 14:01
Branching strategy: feature branches per phase (no commits to main)

## Scope
- Create two consolidated tables with explicit batting and bowling columns and a generic scope field:
  - `feature_form_snapshots`
  - `feature_consistency_snapshots`
- Update go-app writers (precompute CLI) to persist to these.
- Update readers (go-app exports, ml-service) to read from these.
- Recompute from raw histories (no backfill from old tables, no backward compatibility, no views).
- Drop legacy precompute tables in a later cleanup phase.

## Files to create/modify
- Create: `go-app/migrations/0012_feature_snapshots.sql` (DDL for the two tables + indexes).
- Modify: `go-app/internal/db` (new repo methods for inserts/upserts to new tables).
- Modify: `go-app/cmd/precompute-features/main.go` (write consolidated rows for overall/venue/opposition per player/date/format).
- Modify: go-app exporters or readers (if they directly query old tables; update to new tables).
- Modify: `ml-service` SQL that reads precomputed values to use new tables.
- Modify: `README.md` (schema and usage updates) and any docs if necessary.

## High-level changes per file
- 0012_feature_snapshots.sql: define `feature_form_snapshots` and `feature_consistency_snapshots` with keys and indexes:
  - Common columns: `player_id`, `as_of_date`, `format_id`, `scope`, `scope_id`, `created_at`, `source_version`.
  - Form: `batting_value`, `bowling_value`, `alpha`, `effective_n`.
  - Consistency: `batting_value`, `bowling_value`, `window_n`.
  - Unique: `(player_id, as_of_date, format_id, scope, scope_id)`.
- db repo: add `InsertOrUpdateFormSnapshot` and `InsertOrUpdateConsistencySnapshot` with ON CONFLICT upsert.
- precompute-features: compute both bat/bowl metrics per scope and call repo methods for overall, each venue, each opposition.
- readers: switch SQL to new tables; select appropriate bat/bowl columns.

## Tests to add/update
- go-app internal/db unit tests for insert, upsert, and select from both tables (table-driven tests).
- precompute-features integration test: seed small histories, run replay, assert expected rows for overall/venue/opposition with both bat and bowl values.
- ml-service: update tests reading new tables to validate returned shapes/values; keep fixtures deterministic.

## Acceptance criteria
- Migrations apply cleanly in CI and locally, creating both tables and indexes.
- Precompute job populates both tables (overall/venue/opposition) with correct batting/bowling columns and metadata.
- Upserts are idempotent under replay.
- ml-service queries updated and pass tests; end-to-end export/validation steps succeed.
- Performance within ±10% of baseline on primary queries filtering by `(player_id, format_id, as_of_date)`.

## Verification commands
- Apply migrations:
  - `make -C go-app migrate`
- Populate via replay:
  - `make -C go-app run-precompute ARGS="-format ODI -replay -ewm-alpha 0.3 -lastN 10"`
- Spot-check SQL:
  - `SELECT scope, COUNT(*) FROM feature_form_snapshots GROUP BY scope;`
  - `SELECT * FROM feature_consistency_snapshots WHERE player_id=$1 AND as_of_date=$2 AND format_id=$3 AND scope='opposition' AND scope_id=$4;`

## Risks & mitigations
- Schema mismatch vs code: ensure repo methods and CLI updated in the same phase; use ON CONFLICT.
- Performance: add covering indexes; adjust NUMERIC precision if needed.

## Phase breakdown (PRs)
1. feat/ddl-feature-snapshots — Add `0012_feature_snapshots.sql` migration. (This phase)
2. feat/precompute-write-feature-snapshots — Add repo methods, update writers, tests.
3. feat/ml-read-feature-snapshots — Update readers and tests.
4. chore/drop-legacy-precompute-tables — Remove old tables; update docs.
