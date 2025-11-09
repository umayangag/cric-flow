# Plan: Remove legacy *_fmt tables and code paths

Owner: Junie
Date: 2025-11-08 15:28
Branching: feature branches per phase (never commit to main)

## Context
- The project has consolidated to per-feature snapshot tables (`feature_form_snapshots`, `feature_consistency_snapshots`) with scopes and separate batting/bowling columns.
- Legacy per-format pre-aggregated tables ("*_fmt") remain referenced in a few code paths but are redundant now:
  - `player_form_data_fmt`
  - `player_consistency_data_fmt`
  - `player_venue_data_fmt`
  - `player_opposition_data_fmt`
- Unified exporters already read from consolidated tables; we can remove `*_fmt` tables and dependent code.

## Objectives
1. Remove all `*_fmt`-dependent code paths.
2. Drop the `*_fmt` tables via migration.
3. Update Makefile/docs to rely solely on unified exporters.
4. Keep CI green; no behavior change for supported exports.

## Files to create/modify
- Create: `go-app/migrations/0015_drop_fmt_tables.sql` (DROP legacy `*_fmt` tables).
- Modify: `go-app/cmd/export-dataset/main.go` (remove `*_fmt` export functions/usages).
- Delete: `go-app/cmd/tools/validate-fmt` (entire tool, only relevant to `*_fmt`).
- Modify: `Makefile` (remove targets that invoke legacy `*_fmt` flows/tools, if any).
- Modify: `README.md` and/or `docs/` (remove mentions of `*_fmt`, point to unified exporters only).

## High-level changes per file
- export-dataset/main.go:
  - Remove `exportBattingFormat` and `exportBowlingFormat` functions which rely on `*_fmt` tables.
  - Remove invocations to these functions (including inference-variant if it relies on `*_fmt`).
  - Ensure unified export codepaths (already using consolidated tables) remain default and documented.
- cmd/tools/validate-fmt:
  - Remove command; it validates coverage of `*_fmt` tables which will be dropped.
- Makefile:
  - Ensure `export-dataset` target uses only unified exporters.
  - Remove any helper targets referencing `validate-fmt` or `*_fmt` exports.
- Migration 0015:
  - Idempotent `DROP TABLE IF EXISTS` for all four `*_fmt` tables.

## Tests to add/update
- Go (go-app):
  - Build and run the exporter after removals to ensure compilation and runtime succeed.
  - Optional: add a small unit/smoke test around SQL assembly in unified exporters (if practical).
- CI guard:
  - Add a simple grep step in CI or a unit test to assert no occurrences of `"_fmt"` remain in code (optional but recommended).

## Acceptance criteria
- No references to `*_fmt` remain in the repository (verified by search for `"_fmt"` and exact table names).
- Migration `0015_drop_fmt_tables.sql` applies cleanly and tables are removed.
- `make -C go-app export-dataset` succeeds and produces the same outputs as before (unified exporters).
- Go and ML CI workflows pass (coverage thresholds unchanged).

## Verification commands
- Apply migrations:
  - `make -C go-app migrate`
- Populate consolidated tables (if required for local validation):
  - `make -C go-app run-precompute ARGS="-format ODI -replay -ewm-alpha 0.3 -lastN 10"`
- Export datasets:
  - `make -C go-app export-dataset`
- Code search guard:
  - `grep -R "_fmt" -n go-app ml-service | wc -l` (expect 0)
- DB guard:
  - `SELECT to_regclass('public.player_form_data_fmt');`
  - `SELECT to_regclass('public.player_consistency_data_fmt');`
  - `SELECT to_regclass('public.player_venue_data_fmt');`
  - `SELECT to_regclass('public.player_opposition_data_fmt');` (all should return NULL)

## Risks & mitigations
- Risk: hidden dependency on `*_fmt` tables.
  - Mitigation: global search before drop; remove code paths; run unified exporter; CI must pass.
- Risk: Makefile or docs still reference old flows.
  - Mitigation: update Makefile and docs in the same PR as code removal.

## Rollout & PR breakdown
1. PR `refactor/remove-fmt-readers`
   - Remove `exportBattingFormat`/`exportBowlingFormat` and code paths that use `*_fmt`.
   - Delete `go-app/cmd/tools/validate-fmt`.
   - Update `Makefile` and docs accordingly.
2. PR `chore/drop-fmt-tables`
   - Add `go-app/migrations/0015_drop_fmt_tables.sql` to drop `*_fmt` tables.
   - Re-run migrations and exporters in CI.

## Notes
- No backfill or views required. Consolidated tables remain the single source.
- Keep PRs small and focused; Conventional Commits for clarity.