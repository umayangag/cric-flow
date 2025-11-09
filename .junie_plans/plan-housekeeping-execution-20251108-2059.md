# Plan: Housekeeping Execution — Remove validate-fmt tool, scrub legacy refs, refresh docs

Owner: Junie
Date: 2025-11-08 20:59
Branching: feature branches only (never commit to main)

## Objective
Complete the consolidation cleanup by removing the obsolete validate-fmt tool, scrubbing any lingering references to legacy `*_fmt`/`*_asof` tables from active code, and updating docs/Makefile to reflect the unified, consolidated-table flow.

## Scope
- Delete `go-app/cmd/tools/validate-fmt` (dead tool for legacy fmt tables)
- Ensure no lingering references to legacy tables remain in active code (migrations are allowed to mention them historically)
- Update docs/README/Makefile if they still reference legacy flows
- No schema changes (cleanup migrations 0014/0015 already exist)

## Out of scope
- Adding CI grep guard (per prior decision to skip)
- Any changes to consolidated table schemas or precompute logic

## Files to modify/delete
- Delete: `go-app/cmd/tools/validate-fmt` (entire directory)
- Modify (if needed):
  - `Makefile` — ensure only unified export path is documented and used; remove any references to validate-fmt
  - `README.md` — remove mentions of `*_fmt` tables and old exporters, describe consolidated tables + unified exporters
  - `docs/` — adjust diagrams/notes if they mention legacy tables

## High-level changes per file
- Makefile
  - Confirm `export-dataset` target runs unified mode only: `go run ./cmd/export-dataset -unified=1`
  - Remove any targets referencing validate-fmt
- README.md / docs
  - Replace any mentions of legacy tables with:
    - `feature_form_snapshots`
    - `feature_consistency_snapshots`
  - Note scopes and columns (`scope`, `scope_id`, `batting_value`, `bowling_value`)
  - Show the supported commands to migrate, precompute, and export

## Verification & Acceptance Criteria
- Repo has no references to legacy tables in active code:
  - Search (exclude migrations):
    - `player_.*_asof`, `player_.*_data_fmt`, `player_form_data(?!_)`, `player_venue_data(?!_)`, `player_opposition_data(?!_)`
  - Expect 0 hits outside of migrations/plan files
- Build and run exporters successfully:
  - `make -C go-app migrate`
  - `make -C go-app run-precompute ARGS="-format ODI -replay -ewm-alpha 0.3 -lastN 10"`
  - `make -C go-app export-dataset` → writes unified CSVs
- ML Service CI remains green with coverage ≥ 80%:
  - `make -C ml-service ci`
- Documentation updated:
  - README/docs reference only consolidated tables and unified exporters

## Commands (verification)
- Code guards (from repo root):
  - `rg "player_.*_asof|player_.*_data_fmt|player_form_data(?!_)|player_venue_data(?!_)|player_opposition_data(?!_)" go-app ml-service -n`
- DB guards (psql): ensure legacy tables are gone after migrations (expected NULL):
  - `SELECT to_regclass('public.player_form_asof');`
  - `SELECT to_regclass('public.player_consistency_asof');`
  - `SELECT to_regclass('public.player_vs_opposition_asof');`
  - `SELECT to_regclass('public.player_at_venue_asof');`
  - `SELECT to_regclass('public.player_form_data_fmt');`
  - `SELECT to_regclass('public.player_consistency_data_fmt');`
  - `SELECT to_regclass('public.player_venue_data_fmt');`
  - `SELECT to_regclass('public.player_opposition_data_fmt');`

## Step-by-step Execution
1. Create feature branch
   - `git checkout -b chore/housekeeping-remove-validate-fmt`
2. Delete tool
   - Remove directory `go-app/cmd/tools/validate-fmt`
3. Scrub Makefile/docs
   - Ensure no mentions of validate-fmt remain
   - Ensure exporters documented via unified flag only
4. Sanity searches (active code only)
   - Run ripgrep guard listed above; if any hits in active code, replace with consolidated tables (scoped latest-as-of pattern)
5. Build and verify
   - `make -C go-app migrate`
   - (optional) `make -C go-app go-test`
   - `make -C go-app export-dataset`
   - `make -C ml-service ci`
6. Commit & PR
   - Conventional Commits example:
     - `chore(go-app): remove validate-fmt tool`
     - `docs: update README to consolidated snapshot tables and unified exporters`
   - Open PR with concise description and verification evidence

## Rollback
- Revert branch/PR; no DB schema changes in this plan

## Risks & Mitigations
- Hidden reference to validate-fmt or legacy tables in docs/targets
  - Mitigation: repository-wide search; review Makefile and docs in the same PR
- Export or CI regression due to accidental edit
  - Mitigation: keep changes minimal; run verification commands locally/CI
