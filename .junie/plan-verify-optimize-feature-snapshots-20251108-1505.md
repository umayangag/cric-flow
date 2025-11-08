# Plan: Verify & Optimize Consolidated Feature Snapshot Tables

Owner: Junie
Date: 2025-11-08 15:05
Branching: feature branches only; never commit to main

## Goal
Confirm the consolidation is complete and optimize the new tables for the current read/write patterns. Then safely remove legacy tables.

## Current State (as observed)
- New tables exist (migration `0012_feature_snapshots.sql`):
  - `feature_form_snapshots(player_id, as_of_date, format_id, scope, scope_id, batting_value, bowling_value, alpha, n_samples_bat, n_samples_bowl, effective_n, created_at, source_version)`
  - `feature_consistency_snapshots(player_id, as_of_date, format_id, scope, scope_id, batting_value, bowling_value, window_n, n_samples_bat, n_samples_bowl, created_at, source_version)`
- Writers: `go-app/cmd/precompute-features/main.go` writes overall + opposition + venue (form) and overall (consistency) using new upserts.
- Readers: `go-app/cmd/export-dataset/main.go` switched to new tables using lateral queries with predicates:
  - `player_id`, `format_id`, `scope`, `scope_id (nullable)`, `as_of_date <= md.date` with `ORDER BY as_of_date DESC LIMIT 1`.
- ML service: not using legacy precomputed as-of tables; no change needed.
- Legacy tables still present from migration `0009_asof_features.sql`:
  - `player_form_asof`, `player_consistency_asof`, `player_vs_opposition_asof`, `player_at_venue_asof`.

## Phase 1 — Verification Checklist
- Migrations applied: `make -C go-app migrate` ✓
- Data populated: run precompute in replay for formats of interest:
  - `make -C go-app run-precompute ARGS="-format TEST -replay -ewm-alpha 0.3 -lastN 10"`
  - `make -C go-app run-precompute ARGS="-format ODI  -replay -ewm-alpha 0.3 -lastN 10"`
  - `make -C go-app run-precompute ARGS="-format T20I -replay -ewm-alpha 0.3 -lastN 10"`
  - `make -C go-app run-precompute ARGS="-format T20  -replay -ewm-alpha 0.3 -lastN 10"`
- Sanity SQL:
  - `SELECT scope, COUNT(*) FROM feature_form_snapshots GROUP BY scope;`
  - `SELECT COUNT(*) FROM feature_form_snapshots WHERE scope='overall' AND scope_id IS NOT NULL;`  -- expect 0
  - `SELECT COUNT(*) FROM feature_consistency_snapshots WHERE scope='overall' AND scope_id IS NOT NULL;`  -- expect 0
- Export works: `make -C go-app export-dataset` ✓

## Phase 2 — Index Review & Optimization (Proposed Additions)
Existing indexes:
- `(player_id, format_id, as_of_date)`
- `(scope, scope_id, player_id, as_of_date)`

Given query shape `... WHERE player_id=$1 AND format_id=$2 AND scope=... [AND scope_id=...] AND as_of_date <= $cutoff ORDER BY as_of_date DESC LIMIT 1`, suggest filtered descending indexes to accelerate the latest-snapshot lookup and enable index-only scans:

1) Overall scope fast-path (both tables)
```sql
-- Batting + bowling values are selected frequently; include sample counts
CREATE INDEX IF NOT EXISTS idx_f_form_overall_latest
  ON feature_form_snapshots (player_id, format_id, as_of_date DESC)
  WHERE scope='overall' AND scope_id IS NULL
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);

CREATE INDEX IF NOT EXISTS idx_f_cons_overall_latest
  ON feature_consistency_snapshots (player_id, format_id, as_of_date DESC)
  WHERE scope='overall' AND scope_id IS NULL
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);
```

2) Opposition scope fast-path (form table)
```sql
CREATE INDEX IF NOT EXISTS idx_f_form_opp_latest
  ON feature_form_snapshots (player_id, format_id, scope_id, as_of_date DESC)
  WHERE scope='opposition'
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);
```

3) Venue scope fast-path (form table)
```sql
CREATE INDEX IF NOT EXISTS idx_f_form_venue_latest
  ON feature_form_snapshots (player_id, format_id, scope_id, as_of_date DESC)
  WHERE scope='venue'
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);
```

Notes:
- DESC on `as_of_date` matches `ORDER BY ... DESC LIMIT 1` to avoid an extra sort.
- INCLUDE makes index-only scans possible on Postgres ≥ 11.
- Create these in a new migration (Phase 4) after verifying cardinality; if table size is small initially, existing indexes may suffice.

Optional for very large tables:
- BRIN on `as_of_date` for replay-heavy analytics:
```sql
CREATE INDEX IF NOT EXISTS brin_f_form_date ON feature_form_snapshots USING BRIN (as_of_date);
CREATE INDEX IF NOT EXISTS brin_f_cons_date ON feature_consistency_snapshots USING BRIN (as_of_date);
```

## Phase 3 — Maintenance & Stats
- Run `ANALYZE` after large replay loads; ensure autovacuum is enabled.
- Consider partitioning by month or by `format_id` if rows exceed ~50–100M per table; defer until needed.

## Phase 4 — Cleanup (Drop Legacy Tables)
Create migration to drop unused legacy tables once verification is complete and exporters use only new tables:
```sql
-- 00xx_drop_legacy_asof.sql
DROP TABLE IF EXISTS player_form_asof;
DROP TABLE IF EXISTS player_consistency_asof;
DROP TABLE IF EXISTS player_vs_opposition_asof;
DROP TABLE IF EXISTS player_at_venue_asof;
```
Also deprecate or remove legacy writer helpers in Go (`UpsertPlayerConsistencyAsOf`, `UpsertPlayerVsOppAsOf`, `UpsertPlayerAtVenueAsOf`) if no longer used.

## Acceptance Criteria
- Functional:
  - Exports succeed using only consolidated tables; ML-service unchanged and green.
- Performance:
  - EXPLAIN ANALYZE for representative lateral subqueries shows index usage and no explicit sort for latest lookup.
  - 95th percentile latency for export queries does not regress (>10%).
- Data:
  - Overall rows always have `scope_id IS NULL`.
- Operations:
  - Legacy tables removed safely after verification.

## Verification Commands
- Explain samples (run in psql):
```sql
EXPLAIN ANALYZE
SELECT batting_value, n_samples_bat FROM feature_form_snapshots
WHERE player_id=$PID AND format_id=$FMT AND scope='overall' AND scope_id IS NULL AND as_of_date <= $DATE
ORDER BY as_of_date DESC LIMIT 1;

EXPLAIN ANALYZE
SELECT batting_value, n_samples_bat FROM feature_form_snapshots
WHERE player_id=$PID AND format_id=$FMT AND scope='opposition' AND scope_id=$OPP AND as_of_date <= $DATE
ORDER BY as_of_date DESC LIMIT 1;
```
- CI:
  - `make -C go-app migrate` (with new index migration)
  - `make -C go-app export-dataset`
  - `make -C ml-service ci`

## PR Breakdown
1) `perf/add-indexes-feature-snapshots`
   - Add filtered DESC + INCLUDE indexes for overall/opposition/venue lookups.
   - Add ANALYZE hooks or doc notes.
2) `chore/drop-legacy-precompute-tables`
   - Drop legacy tables and remove/deprecate unused writer helpers.
   - Update README/docs.

## Rollback
- Index additions are safe; `DROP INDEX` to rollback.
- If any consumer still needs legacy tables, abort cleanup PR and keep them.
