# Plan: Readers Cutover to Consolidated Feature Snapshot Tables

Owner: Junie
Date: 2025-11-08 14:23
Branching: feature branches per phase (never commit to main)

## Objective
Switch all reader paths (go-app exporters and ml-service) from legacy precompute tables to the new consolidated tables:
- `feature_form_snapshots`
- `feature_consistency_snapshots`

We will preserve functionality and column naming in outputs while using the new schema with scopes and separate batting/bowling columns.

## Out of scope
- No backward compatibility views or dual-write.
- Cleanup (dropping legacy tables) handled in a later PR after verification.

## Files to Modify
- go-app
  - `go-app/cmd/export-dataset/main.go` — replace lateral joins and direct reads of:
    - `player_form_asof` → `feature_form_snapshots` (scope='overall', scope_id IS NULL); select `batting_value` as `bat_form`, `n_samples_bat` as before.
    - `player_consistency_asof` → `feature_consistency_snapshots` (scope='overall', scope_id IS NULL); select `batting_value` as `bat_consistency`, `n_samples_bat`.
    - `player_vs_opposition_asof` (form) → `feature_form_snapshots` (scope='opposition', scope_id=md.opposition_id); select `batting_value` as `bat_value`, `n_samples_bat` as `n_samples`.
    - `player_at_venue_asof` (form) → `feature_form_snapshots` (scope='venue', scope_id=md.venue_id); select `batting_value` as `bat_value`, `n_samples_bat` as `n_samples`.
  - Maintain ORDER BY `as_of_date` DESC LIMIT 1 and `as_of_date <= md.date` filter semantics.
  - Ensure existing output aliases remain unchanged (e.g., `bat_form_ODI_asof`, `bat_vs_opp_T20_asof`).
- ml-service
  - Search and update any SQL or ORM queries referencing legacy tables (`player_form_asof`, `player_consistency_asof`, `player_vs_opposition_asof`, `player_at_venue_asof`) to the consolidated tables with the appropriate `scope` and `scope_id` predicates.
  - Explicitly select `batting_value` / `bowling_value` as needed; preserve existing variable/column names in Python code.

## SQL Replacement Patterns (illustrative)
- Overall form (per format):
  ```sql
  -- old
  SELECT bat_form, n_samples_bat FROM player_form_asof
  WHERE player_id=$PID AND format_id=$FMT AND as_of_date <= md.date
  ORDER BY as_of_date DESC LIMIT 1;

  -- new
  SELECT batting_value AS bat_form, n_samples_bat FROM feature_form_snapshots
  WHERE player_id=$PID AND format_id=$FMT AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
  ORDER BY as_of_date DESC LIMIT 1;
  ```
- Overall consistency (per format):
  ```sql
  -- old
  SELECT bat_consistency, n_samples_bat FROM player_consistency_asof ...
  -- new
  SELECT batting_value AS bat_consistency, n_samples_bat FROM feature_consistency_snapshots
  WHERE player_id=$PID AND format_id=$FMT AND scope='overall' AND scope_id IS NULL AND as_of_date <= md.date
  ORDER BY as_of_date DESC LIMIT 1;
  ```
- Vs opposition form:
  ```sql
  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
  WHERE player_id=$PID AND format_id=$FMT AND scope='opposition' AND scope_id=md.opposition_id AND as_of_date <= md.date
  ORDER BY as_of_date DESC LIMIT 1;
  ```
- At venue form:
  ```sql
  SELECT batting_value AS bat_value, n_samples_bat AS n_samples FROM feature_form_snapshots
  WHERE player_id=$PID AND format_id=$FMT AND scope='venue' AND scope_id=md.venue_id AND as_of_date <= md.date
  ORDER BY as_of_date DESC LIMIT 1;
  ```

Notes:
- If/when bowling-based exports are needed, select `bowling_value` similarly.
- Keep indexes in mind: `(player_id, format_id, as_of_date)` and `(scope, scope_id, player_id, as_of_date)` are available.

## Tests
- Go (go-app):
  - Add/adjust tests for export query components to ensure the SQL compiles and returns expected columns.
  - If integration tests exist for export-dataset, seed minimal fixtures and verify a small export completes and includes expected columns.
- Python (ml-service):
  - Update unit tests/fixtures that read precomputed tables to the new tables.
  - Ensure tests remain deterministic; use small fixtures under `tests/fixtures/`.

## Data Preparation (pre-requisite)
- Ensure migration 0012 is applied: `make -C go-app migrate`.
- Populate consolidated tables via replay before running exports/tests. For each required format:
  - `make -C go-app run-precompute ARGS="-format TEST -replay -ewm-alpha 0.3 -lastN 10"`
  - `make -C go-app run-precompute ARGS="-format ODI  -replay -ewm-alpha 0.3 -lastN 10"`
  - `make -C go-app run-precompute ARGS="-format T20I -replay -ewm-alpha 0.3 -lastN 10"`
  - `make -C go-app run-precompute ARGS="-format T20  -replay -ewm-alpha 0.3 -lastN 10"`

## Acceptance Criteria
- All references to legacy tables in go-app exporters and ml-service are replaced with consolidated tables.
- Export commands run successfully and produce the same columns as before (values may differ slightly only due to rounding; investigate otherwise).
- CI passes for both go-app and ml-service; ml-service coverage ≥ 80%.
- Spot SQL checks succeed:
  - `SELECT scope, COUNT(*) FROM feature_form_snapshots GROUP BY scope;`
  - `SELECT COUNT(*) FROM feature_form_snapshots WHERE scope='overall' AND scope_id IS NOT NULL;` → 0
  - Sample query for a player/opposition returns a row with most recent `as_of_date`.

## Verification Commands
- Migrations: `make -C go-app migrate`
- Precompute: see Data Preparation above
- Export (example): `make -C go-app export-dataset`
- ML Service CI: `make -C ml-service ci`

## Risks & Mitigations
- Missing data due to unpopulated consolidated tables → Run replay before tests; add guardrails/logging.
- Query typos on scope/scope_id → Add unit tests and a couple of SQL sanity checks.
- Performance regression → Rely on covering indexes; review explain plans if needed.

## Rollout / PR Breakdown
1) `feat/ml-read-feature-snapshots` (current phase)
   - Modify go-app exporter SQL to read consolidated tables; add/update tests.
2) `feat/ml-service-read-feature-snapshots`
   - Update ml-service queries and tests to use consolidated tables.
3) `chore/drop-legacy-precompute-tables`
   - Add migration to drop legacy tables and remove dead code; update README/docs.
