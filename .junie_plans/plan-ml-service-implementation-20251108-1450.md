# Plan: ML-Service Readers Cutover — Implementation (PR 2)

Owner: Junie
Date: 2025-11-08 14:50
Branch: feat/ml-service-read-feature-snapshots
Scope: Implement readers cutover in `ml-service` to use consolidated precompute tables and update tests.

## Objectives
- Replace all usages of legacy as-of tables with consolidated tables:
  - `player_form_asof` → `feature_form_snapshots` (scope='overall', scope_id IS NULL)
  - `player_consistency_asof` → `feature_consistency_snapshots` (scope='overall', scope_id IS NULL)
  - `player_vs_opposition_asof` → `feature_form_snapshots` (scope='opposition', scope_id=<opp_id>)
  - `player_at_venue_asof` → `feature_form_snapshots` (scope='venue', scope_id=<venue_id>)
- Preserve output shapes/aliases expected by downstream code.
- Keep test coverage ≥ 80% (as enforced by CI env `COV_MIN`).

## Design Notes
- Consolidated tables have separate `batting_value` and `bowling_value` columns, plus sample counts (`n_samples_bat`, `n_samples_bowl`).
- Readers must select the correct column based on context (batting vs bowling).
- Always filter by `as_of_date <= cutoff` and select latest via `ORDER BY as_of_date DESC LIMIT 1`.
- Keys and indexes available:
  - UNIQUE `(player_id, as_of_date, format_id, scope, scope_id)`
  - Index `(player_id, format_id, as_of_date)`
  - Index `(scope, scope_id, player_id, as_of_date)`

## Files to Modify (to be confirmed during discovery)
- ml-service/db/*.py — low-level DB access or query builders.
- ml-service/data_loaders/*.py — dataset readers that join or fetch precomputed features.
- ml-service/features/*.py — feature engineering layers that depend on precomputed values.
- ml-service/tests/** — unit/integration tests and fixtures referencing legacy tables.
- Optionally docs: README sections referencing precompute tables.

## Replacement SQL Patterns
- Overall form (batting):
```sql
SELECT batting_value AS bat_form, n_samples_bat
FROM feature_form_snapshots
WHERE player_id = %(player_id)s AND format_id = %(format_id)s
  AND scope='overall' AND scope_id IS NULL
  AND as_of_date <= %(cutoff)s
ORDER BY as_of_date DESC LIMIT 1;
```
- Overall consistency (batting):
```sql
SELECT batting_value AS bat_consistency, n_samples_bat
FROM feature_consistency_snapshots
WHERE player_id = %(player_id)s AND format_id = %(format_id)s
  AND scope='overall' AND scope_id IS NULL
  AND as_of_date <= %(cutoff)s
ORDER BY as_of_date DESC LIMIT 1;
```
- Vs opposition (batting):
```sql
SELECT batting_value AS bat_value, n_samples_bat AS n_samples
FROM feature_form_snapshots
WHERE player_id = %(player_id)s AND format_id = %(format_id)s
  AND scope='opposition' AND scope_id = %(opposition_id)s
  AND as_of_date <= %(cutoff)s
ORDER BY as_of_date DESC LIMIT 1;
```
- At venue (batting):
```sql
SELECT batting_value AS bat_value, n_samples_bat AS n_samples
FROM feature_form_snapshots
WHERE player_id = %(player_id)s AND format_id = %(format_id)s
  AND scope='venue' AND scope_id = %(venue_id)s
  AND as_of_date <= %(cutoff)s
ORDER BY as_of_date DESC LIMIT 1;
```
- Mirror for bowling: use `bowling_value` and `n_samples_bowl` with same predicates.

## Step-by-Step Plan
1. Discovery (search & inventory)
   - Search `ml-service/**` for legacy table names and list all affected modules/functions and expected output shapes.
2. Implementation
   - Replace SQL in affected modules per patterns above; keep aliases identical to previous code.
   - Ensure parameter binding matches existing DB layer (psycopg/SQLAlchemy style).
3. Tests
   - Update unit tests and fixtures to reference consolidated tables.
   - Add tests covering: overall form, overall consistency, vs-opposition form, at-venue form for both batting and bowling.
   - Ensure tests are deterministic and small; use fixtures under `tests/fixtures/`.
4. Docs & CI
   - Update any docs referencing legacy tables (if applicable).
   - Run CI locally and ensure coverage ≥ 80%.

## Acceptance Criteria
- All legacy table references removed from `ml-service`.
- All updated queries select correct columns and maintain previous aliases.
- `make -C ml-service ci` passes with coverage ≥ 80%.
- When consolidated tables are populated, end-to-end validations (if any) pass.

## Verification Commands
- Pre-requisites (from project root):
  - `make -C go-app migrate`
  - Populate consolidated tables (example):
    - `make -C go-app run-precompute ARGS="-format TEST -replay -ewm-alpha 0.3 -lastN 10"`
    - `make -C go-app run-precompute ARGS="-format ODI  -replay -ewm-alpha 0.3 -lastN 10"`
    - `make -C go-app run-precompute ARGS="-format T20I -replay -ewm-alpha 0.3 -lastN 10"`
    - `make -C go-app run-precompute ARGS="-format T20  -replay -ewm-alpha 0.3 -lastN 10"`
- Run ml-service CI locally:
  - `make -C ml-service ci`
- Optional (if exported CSVs exist):
  - `make -C ml-service validate-exports`

## Risks & Mitigations
- Missed legacy reference → Perform global search and add a guard test that fails if legacy names appear in code.
- Alias mismatch → Add unit tests asserting returned dict/columns include expected alias names.
- Data not populated → Document pre-requisite replay; include fail-fast checks/logging in readers if no row is found.

## Rollout
- Open PR on branch `feat/ml-service-read-feature-snapshots` with changes and tests.
- Keep PR focused and small; ensure CI green.
- After merge, proceed to cleanup PR to drop legacy tables.
