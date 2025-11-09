# Plan: ML Service Readers Cutover to Consolidated Feature Tables (PR 2)

Owner: Junie
Date: 2025-11-08 14:48
Branch: feat/ml-service-read-feature-snapshots
Scope: Update ml-service to read from consolidated precompute tables without backward-compat shims.

## Context
- Consolidated tables created in go-app migrations:
  - `feature_form_snapshots`
  - `feature_consistency_snapshots`
- Writers updated in go-app; go-app exporters already switched (PR 1).
- This PR updates ml-service SQL/data loaders to use the new tables and keeps tests ≥ 80% coverage.

## Objectives
1. Replace all reads of legacy as-of tables with consolidated equivalents:
   - `player_form_asof` → `feature_form_snapshots` (scope='overall', scope_id IS NULL)
   - `player_consistency_asof` → `feature_consistency_snapshots` (scope='overall', scope_id IS NULL)
   - `player_vs_opposition_asof` → `feature_form_snapshots` with `scope='opposition'` and `scope_id=opposition_id`
   - `player_at_venue_asof` → `feature_form_snapshots` with `scope='venue'` and `scope_id=venue_id`
2. Preserve column aliases and output shapes expected by downstream code.
3. Update/extend tests and fixtures accordingly.

## Work Breakdown

### Phase A — Discovery (read-only)
- Search ml-service for references to legacy tables/columns:
  - `player_form_asof`, `player_consistency_asof`, `player_vs_opposition_asof`, `player_at_venue_asof`.
- Identify modules and functions issuing these queries (raw SQL or ORM).
- Inventory expected output schemas/aliases in these paths.

### Phase B — Implementation
- Update SQL queries:
  - Overall form (batting):
    ```sql
    SELECT batting_value AS bat_form, n_samples_bat
    FROM feature_form_snapshots
    WHERE player_id=%(player_id)s AND format_id=%(format_id)s
      AND scope='overall' AND scope_id IS NULL
      AND as_of_date <= %(cutoff)s
    ORDER BY as_of_date DESC LIMIT 1;
    ```
  - Overall consistency (batting):
    ```sql
    SELECT batting_value AS bat_consistency, n_samples_bat
    FROM feature_consistency_snapshots
    WHERE player_id=%(player_id)s AND format_id=%(format_id)s
      AND scope='overall' AND scope_id IS NULL
      AND as_of_date <= %(cutoff)s
    ORDER BY as_of_date DESC LIMIT 1;
    ```
  - Vs opposition (batting):
    ```sql
    SELECT batting_value AS bat_value, n_samples_bat AS n_samples
    FROM feature_form_snapshots
    WHERE player_id=%(player_id)s AND format_id=%(format_id)s
      AND scope='opposition' AND scope_id = %(opposition_id)s
      AND as_of_date <= %(cutoff)s
    ORDER BY as_of_date DESC LIMIT 1;
    ```
  - At venue (batting):
    ```sql
    SELECT batting_value AS bat_value, n_samples_bat AS n_samples
    FROM feature_form_snapshots
    WHERE player_id=%(player_id)s AND format_id=%(format_id)s
      AND scope='venue' AND scope_id = %(venue_id)s
      AND as_of_date <= %(cutoff)s
    ORDER BY as_of_date DESC LIMIT 1;
    ```
  - Mirror equivalents for bowling (`bowling_value`, `n_samples_bowl`).
- Ensure parameter binding matches existing code style (psycopg/SQLAlchemy placeholders).
- Keep ordering and `LIMIT 1` semantics to select the latest snapshot `<= cutoff`.

### Phase C — Tests (TDD)
- Locate tests under `ml-service/tests/` referencing legacy tables.
- Update fixtures if needed: tests should pass after new precompute job populates consolidated tables.
- Add tests to assert that for a sample player/date/format, the queries return expected values for:
  - overall form, overall consistency
  - vs-opposition form, at-venue form
- Ensure tests use small deterministic fixtures; no network access.

### Phase D — Docs & CI
- If README or docs mention old tables, update to reflect consolidated tables.
- Ensure CI (`.github/workflows/ml-service-ci.yml`) runs `make -C ml-service ci` successfully.

## Files To Modify (expected)
- `ml-service/**` Python modules that construct SQL for precomputed features, e.g.:
  - data loading utilities (e.g., `ml-service/data_loaders/*.py` or similar)
  - feature engineering modules (e.g., `ml-service/features/*.py`)
  - repository/dao modules (e.g., `ml-service/db/*.py`)
- `ml-service/tests/**` — update tests and fixtures accordingly.
- `README.md` or `docs/` if they document table names (optional in this PR if not referenced).

Note: Exact file list to be finalized after Phase A discovery.

## Acceptance Criteria
- All ml-service references to legacy tables are removed/replaced.
- Unit tests and CI pass with coverage ≥ 80% (per workflow env `COV_MIN`).
- End-to-end (optional) validations succeed when consolidated tables are populated via go-app precompute.
- Query outputs maintain previous shapes and aliases (`bat_form`, `bat_consistency`, `bat_value`, `bowl_form`, etc.).

## Verification Commands
- Prepare DB and data:
  - `make -C go-app migrate`
  - Populate consolidated tables per format (example):
    - `make -C go-app run-precompute ARGS="-format TEST -replay -ewm-alpha 0.3 -lastN 10"`
- Run ml-service CI locally:
  - `make -C ml-service ci`
- Optional validation when exported CSVs present:
  - `make -C ml-service validate-exports` (guarded in CI already)

## Risks & Mitigations
- Risk: Missed reference to a legacy table.
  - Mitigation: global search for all four legacy table names; add a unit test ensuring no legacy names remain.
- Risk: Schema/alias mismatch.
  - Mitigation: keep aliasing exactly as before; add regression tests on shapes.

## Rollout
1. Open PR `feat/ml-service-read-feature-snapshots` with only ml-service changes + tests.
2. Ensure CI green.
3. After merge, proceed to PR 3 (cleanup legacy tables and docs).
