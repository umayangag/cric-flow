# Plan: Housekeeping — Remove validate-fmt tool, tidy dead code refs, refresh docs

Owner: Junie
Date: 2025-11-08 20:58
Branch: chore/housekeeping-validate-fmt-and-docs

## Objective
Cleanup residual legacy artifacts now that consolidated feature snapshot tables are fully adopted:
- Remove obsolete tool `go-app/cmd/tools/validate-fmt`
- Ensure no dead code paths reference legacy `*_fmt` or dropped `*_asof` tables
- Refresh README/docs to reflect the consolidated tables and unified exporters

## Scope
- Codebase cleanup (Go app); minimal/no behavior change
- Documentation updates
- No DB schema changes (migrations already dropped legacy tables)

## Files to create/modify/delete
- Delete: `go-app/cmd/tools/validate-fmt` (entire directory)
- Modify: `README.md` — update export instructions to use unified exporters; document consolidated tables
- Modify: `docs/` (if any mentions of `*_fmt` or `*_asof`) — update terminology/examples
- Modify: `Makefile` — verify no targets reference `validate-fmt`; remove if present
- Verify only (no changes unless found): search and ensure no active code references to legacy tables or `*_fmt` helpers

## Steps
1. Discovery
   - Repo-wide search (excluding migrations) for legacy names:
     - `player_form_asof`, `player_consistency_asof`, `player_vs_opposition_asof`, `player_at_venue_asof`
     - `player_form_data_fmt`, `player_consistency_data_fmt`, `player_venue_data_fmt`, `player_opposition_data_fmt`
     - `validate-fmt`
   - Confirm only migrations and the soon-to-be-deleted tool remain.

2. Remove obsolete tool
   - Delete `go-app/cmd/tools/validate-fmt` folder
   - Ensure build still succeeds

3. Docs updates
   - README.md:
     - Add a short section: "Consolidated Feature Snapshot Tables" describing `feature_form_snapshots` and `feature_consistency_snapshots`, scopes, and columns
     - Update export instructions to prefer unified exports: `make -C go-app export-dataset`
     - Remove any mention of `*_fmt` tables/tools if present
   - docs/: update any references accordingly

4. Makefile sanity
   - Ensure `export-dataset` target uses unified exporters (already in place)
   - Remove any targets that invoke the deleted tool (if present)

5. Verification
   - Build: `make -C go-app build` (or ensure `go run` paths compile)
   - Grep guards (active code only):
     - `rg "player_.*_asof|player_.*_data_fmt|validate-fmt" go-app ml-service -n --glob '!go-app/migrations/**'`
   - Run migrations (no new ones expected): `make -C go-app migrate`
   - Export datasets: `make -C go-app export-dataset`
   - ML CI: `make -C ml-service ci` (should remain green)

## Acceptance Criteria
- `go-app/cmd/tools/validate-fmt` is removed
- No active code references to legacy `*_asof` or `*_fmt` tables
- README/docs accurately describe consolidated tables and unified export commands
- `make -C go-app export-dataset` succeeds; CI remains green

## Risks & Mitigations
- Hidden references to deleted tool or tables → global search before deletion; compile and run exporters after change
- Docs drift → include explicit README updates in this PR

## Rollout / PR
- Single PR: `chore/housekeeping-validate-fmt-and-docs`
  - Conventional Commit: `chore: remove validate-fmt tool, tidy legacy refs, refresh docs`

## Notes
- We intentionally do not remove any migrations; historical DDL remains for audit.
- If additional deprecated helpers are discovered, deprecate in a follow-up PR to keep this change small.
