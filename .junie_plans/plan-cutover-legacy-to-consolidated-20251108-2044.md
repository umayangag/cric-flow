# Plan: Purge Legacy Table References and Finalize Cutover to Consolidated Feature Tables

Owner: Junie
Date: 2025-11-08 20:44
Branching: feature branches only; never commit to main

## Effective Issue
Check in the go-app and ml-service where the old tables are still referred and fix them to use the new consolidated tables.

Legacy tables (and related per-format pre-aggregates) to eliminate from active code:
- player_form_asof, player_consistency_asof, player_vs_opposition_asof, player_at_venue_asof
- player_form_data_fmt, player_consistency_data_fmt, player_venue_data_fmt, player_opposition_data_fmt
- Historical references in migrations are fine and must remain.

Consolidated tables to use:
- feature_form_snapshots
- feature_consistency_snapshots

## Scope
- Ensure all active code in go-app and ml-service reads/writes exclusively from the consolidated tables with proper scope predicates.
- Remove or quarantine any leftover tools or modules that only target legacy tables.
- Keep behavior (output schemas/aliases) and performance intact.

## Files to Create/Modify/Delete
- Create: this plan (.junie/plan-cutover-legacy-to-consolidated-20251108-2044.md)
- go-app
  - Modify: cmd/export-dataset/main.go (verify and finish replacing any stray legacy/"*_fmt" references, especially inference exporters). ✓ previously addressed; re-verify and clean if needed.
  - Delete (or archive): cmd/tools/validate-fmt (validates only *_fmt tables).
  - Optional Modify: internal/db/* (remove unused legacy writer helpers if any remain).
  - Migrations: 0014_drop_legacy_asof_tables.sql (already present) ✓, 0015_drop_fmt_tables.sql (already present) ✓.
- ml-service
  - Modify: ml/queries.py (ensure no legacy table names; use consolidated tables with latest-as-of semantics). ✓ re-verify.
  - Delete/Archive: ml/match_data.py, ml/calculate_features.py if they write to *_fmt tables (legacy helpers not used by CI).
  - Update tests if any asserted legacy table names.
- CI/Docs
  - Optional: add CI guard step to fail on reintroduction of legacy table names.
  - Update README/docs references to new tables only.

## High-level Changes per Area
- Exporters (go-app):
  - All latest-as-of lookups should use lateral joins to feature_form_snapshots/feature_consistency_snapshots.
  - Scopes:
    - overall → scope='overall' AND scope_id IS NULL
    - opposition → scope='opposition' AND scope_id = md.opposition_id
    - venue → scope='venue' AND scope_id = md.venue_id
  - Columns: batting_value/bowling_value (and n_samples_bat/bowl where needed). Keep original aliases.
- ML queries (ml-service):
  - Replace any references to player_form_data, player_venue_data, player_opposition_data, etc., with consolidated-table latest-as-of subqueries or upstream CSV usage. Maintain output aliases.
- Housekeeping:
  - Remove validate-fmt tool and *_fmt dependencies. Drop *_fmt tables via existing migration 0015.

## Detailed Step-by-Step Plan
1. Discovery (read-only) *
   - Global search for legacy names in active code (exclude migrations):
     - player_form_asof, player_consistency_asof, player_vs_opposition_asof, player_at_venue_asof
     - player_form_data_fmt, player_consistency_data_fmt, player_venue_data_fmt, player_opposition_data_fmt
     - player_form_data(?!_), player_venue_data(?!_), player_opposition_data(?!_)
   - Inventory any matches outside migrations and list exact locations.
2. go-app fixes *
   - Ensure cmd/export-dataset/main.go contains only consolidated-table queries (including inference exporters).
   - Remove cmd/tools/validate-fmt (and any Makefile/docs references).
   - Optionally delete/deprecate unused legacy writer helpers in internal/db.
3. ml-service fixes *
   - Ensure ml/queries.py and any data loader use consolidated tables or exported CSVs; remove legacy table joins.
   - If ml/match_data.py or ml/calculate_features.py write to *_fmt, delete or archive.
   - Update tests to avoid legacy table names; keep aliases stable.
4. Migrations & DB cleanup ✓
   - Apply 0014 (drop legacy as-of) and 0015 (drop *_fmt) migrations.
5. Guards & CI *
   - Add CI grep guard to prevent legacy names reappearing in active code.
   - Run Go and ML CI; ensure coverage thresholds met (COV_MIN=80 for ml-service).
6. Docs
   - Update README/docs to reference consolidated tables and unified exporters only.

## Tests to Add/Update
- Go (optional, smoke): build exporters and run a minimal export end-to-end; verify column headers unchanged.
- Python (ml-service): if any unit tests expect legacy names, update them. Add a tiny test (or CI step) asserting no legacy names in active code paths.

## Acceptance Criteria
- No references to legacy tables in active code (confirmed by search), migrations excluded.
- Unified exporters run successfully and produce expected columns; inference exporters compile and run.
- Migrations apply cleanly; legacy tables (`*_asof`, `*_fmt`) no longer exist in DB.
- CI green for both go-app and ml-service; ml-service coverage ≥ 80%.

## Verification Commands
- Apply migrations:
  - make -C go-app migrate
- Populate consolidated tables (example for ODI):
  - make -C go-app run-precompute ARGS="-format ODI -replay -ewm-alpha 0.3 -lastN 10"
- Export datasets:
  - make -C go-app export-dataset
- Run ML CI:
  - make -C ml-service ci
- Code guards (from repo root):
  - rg "player_.*_asof|player_.*_data_fmt|player_form_data(?!_)|player_venue_data(?!_)|player_opposition_data(?!_)" go-app ml-service -n
    - Expect: 0 hits in active code; matches in migrations are acceptable.
- DB guards (psql):
  - SELECT to_regclass('public.player_form_asof');
  - SELECT to_regclass('public.player_consistency_asof');
  - SELECT to_regclass('public.player_vs_opposition_asof');
  - SELECT to_regclass('public.player_at_venue_asof');
  - SELECT to_regclass('public.player_form_data_fmt');
  - SELECT to_regclass('public.player_consistency_data_fmt');
  - SELECT to_regclass('public.player_venue_data_fmt');
  - SELECT to_regclass('public.player_opposition_data_fmt');
    - Expect: all NULL

## Risks & Mitigations
- Hidden legacy usage in seldom-run tools → Use global search; delete known tool (validate-fmt); CI guard.
- Alias/shape mismatches → Preserve aliases; add smoke tests.
- Data not populated → Run precompute replay before exports/CI.

## Rollout & Branching
- feat/remove-fmt-readers
  - Delete validate-fmt tool, clean exporters, update Makefile/docs.
- chore/drop-fmt-tables
  - Migration 0015 already present. Ensure applied.
- chore/remove-legacy-writer-helpers (optional)
  - Remove unused DB methods and references.
- ci/guard-legacy-names (optional)
  - Add grep step to CI to fail on legacy name reintroduction.
