# Parity Audit Findings (in-progress)

This document captures detailed findings from Step 1 (parity audit) against the `src/` prototype, focusing on modularity, readability, and maintainability.

## Summary so far
- Keepers and retired attributes exist in schema and are surfaced via API responses.
  - Evidence: `go-app/internal/db/repo_player.go` selects `is_wicket_keeper, is_retired`.
  - Evidence: `go-app/cmd/api/main.go` exposes fields in player response.
  - Gap: No dedicated importer/task found for setting/maintaining `is_wicket_keeper` and `is_retired` based on external data (prototype has `src/createdb/importers/import_keepers_data.py` and `import_retired.py`). Action: confirm how these flags are populated; add importer task if missing.

- Dataset export CLI exists with per-format support and legacy combined outputs.
  - Evidence: `go-app/cmd/export-dataset/main.go` supports `--format/--formats/--all-formats` and writes `batting_encoded_<FORMAT>.csv`, `bowling_encoded_<FORMAT>.csv`.
  - Mapping to ML expected columns:
    - ML expected batting input columns: see `ml-service/ml/dataset_definitions.py::input_batting_columns`.
    - ML expected bowling input columns: see `ml-service/ml/dataset_definitions.py::input_bowling_columns`.
  - Next: Create a strict schema map (export → ML expected) and run `ml/validate_exports.py` against actual outputs.

- ML service endpoints and models are defined; Pydantic models present in `ml-service/app/main.py`.
  - Endpoints: `health`, `predict_batting`, `predict_bowling`, `predict_win`, `precompute`, `admin_reload`.
  - Next: tighten validation (ranges/enums), ensure consistent error format, and document examples (draft in `docs/api_contracts.md`).

## Detailed checks

### 1) Keepers and Retired flags
- Prototype sources:
  - `src/createdb/importers/import_keepers_data.py`
  - `src/createdb/importers/import_retired.py`
- Implementation status:
  - Schema supports both fields; used in queries and API types.
  - No explicit importer command detected in `go-app/cmd/*`.
- Action:
  - Decide data source for these flags (CSV? external API?).
  - Add `go-app/cmd/import-keepers` and `go-app/cmd/import-retired` or integrate into existing ETL with idempotent UPSERTs.

### 2) Dataset export → ML input schema
- ML expected batting inputs:
  - `batting_consistency, batting_form, batting_temp, batting_wind, batting_rain, batting_humidity, batting_cloud, batting_pressure, batting_viscosity, batting_inning, batting_session, toss, venue, opposition, season, player_name`
- ML expected bowling inputs:
  - `bowling_consistency, bowling_form, bowling_temp, bowling_wind, bowling_rain, bowling_humidity, bowling_cloud, bowling_pressure, bowling_viscosity, batting_inning, bowling_session, toss, bowling_venue, bowling_opposition, season, player_name`
- Exports provide encoded batting/bowling CSVs (per-format or legacy combined)
- Action:
  - Produce an explicit column-by-column mapping in `docs/export_schemas.md`.
  - Wire `ml-service/Makefile` target `validate-exports` into CI to gate on schema drift and nullability issues.

### 3) Modularity & readability notes
- Keep feature-calculation logic encapsulated: `go-app/internal/features/*`, `ml-service/ml/calculate_features.py`.
- Ensure small, focused packages/commands (e.g., separate importers per responsibility, reusable DB repos).
- Keep config centralized: `go-app/internal/config/config.go`, `ml-service/config.py`.
- Prefer explicit Pydantic models and Go structs with JSON tags; avoid weakly typed maps.

## Next actions (Step 1 completion)
1. Confirm source of keeper/retired flags and implement idempotent importer(s) if missing (Go commands + repo methods).
2. Generate actual dataset exports locally and validate with `ml-service/ml/validate_exports.py`; document any diffs.
3. Produce `docs/export_schemas.md` with explicit mappings and constraints (types, nullability, enums).

This file will evolve as we close gaps and move to Steps 2–3.
