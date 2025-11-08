# Plan: Fielding Pipeline — Phase 3 (ML-service schema updates + tests)

## Objective
Consume the newly exported fielding aggregates in the ML-service by updating dataset schemas, loaders, validators, and tests. Maintain backward compatibility (older datasets without the new columns still load with zeros/defaults). Ensure CI passes with coverage thresholds.

## Scope (Phase 3)
- Add the five fielding columns to ML input schemas where appropriate:
  - `catches` (int)
  - `run_outs` (int)
  - `stumpings` (int)
  - `runouts_direct_hits` (int)
  - `fielding_involvements` (int; computed as `catches + run_outs + stumpings` by go-app exporter)
- Update CSV readers/loaders in ML service to accept these fields for both training (encoded) and inference CSVs.
- Update validation utilities to check for presence when available; fill with zeros if missing (backward compatibility).
- Add unit tests and lightweight integration tests for loaders and schema.
- Keep models unchanged for now; only schema ingestion and propagation.

## Files to Create or Modify

- Modify (Python):
  - `ml-service/ml/dataset_definitions.py`
    - Extend `BattingFeatures` and `BowlingFeatures` column lists to include the 5 fielding columns.
    - Ensure column order matches exporters (append at the tail of current schemas).
  - `ml-service/ml/queries.py` or loader modules (where CSVs are parsed)
    - Accept and cast new columns to numeric (ints) with default 0 if absent.
    - Ensure inference loaders accept the same columns in the specified order.
  - `ml-service/app/main.py` or wherever request → feature mapping occurs (if applicable)
    - If the service constructs feature arrays from CSV or request data, include the new fields with defaults = 0 when not present.
  - `ml-service/validators/export_validation.py` (or equivalent)
    - Update header validation to allow either: presence of all 5 columns OR handle their absence by logging a compatibility warning and defaulting to zeros.

- Create (Python tests):
  - `ml-service/tests/test_dataset_definitions.py`
    - Assert the extended schema contains the 5 new columns at the end for both batting and bowling datasets (training and inference paths).
  - `ml-service/tests/test_loaders_with_fielding.py`
    - Use tiny CSV fixtures (inline or under `ml-service/tests/fixtures/`) that include the new columns and another fixture without them.
    - Test that loader returns arrays/dataframes with the 5 columns correctly typed and defaulted to zeros when the columns are missing.
  - `ml-service/tests/fixtures/` (if needed)
    - `batting_infer_T20_with_fielding.csv`
    - `bowling_infer_T20_with_fielding.csv`
    - Corresponding minimal training CSVs (unified or per-format) with the columns.

- Modify (CI, optional if already green):
  - Ensure `make validate-exports` and `make validate-exports-infer` accept the new columns in header checks.

## High-Level Changes per File

- `ml-service/ml/dataset_definitions.py`
  - Append to batting training schema: `catches, run_outs, stumpings, runouts_direct_hits, fielding_involvements`.
  - Append to bowling training schema: same 5 columns.
  - Append to batting inference schema: same 5 columns (inputs only order must match go-app `exportBattingFormatInference`).
  - Append to bowling inference schema: same 5 columns.

- `ml-service/ml/queries.py` (or loader module)
  - When reading CSVs, coerce the 5 columns to integers; if any are missing, add with zeros.
  - Keep column order stable after coercion.

- `ml-service/validators/export_validation.py`
  - Update expected headers to include the 5 columns but tolerate their absence with a warning in compatibility mode. Provide a parameter or env `STRICT_EXPORTS=1` to enforce presence in strict runs.

- Tests
  - Positive case: fixtures including the new columns should validate and load with non-zero values where provided.
  - Compatibility case: fixtures without the new columns should still load, with zeros substituted.

## Acceptance Criteria
1) ML-service can load training and inference CSVs that include the new 5 fielding columns (order exactly as exported) without errors.
2) ML-service can still load older CSVs without these columns; values default to zeros and a clear log/warning is emitted.
3) Unit tests for schema and loaders pass locally and in CI; coverage remains >= configured threshold (e.g., 80%).
4) `make validate-exports` and `make validate-exports-infer` pass when CSVs from go-app include the new columns.

## Verification Commands
- Local (assuming exported files exist under `output/go-app`):
  - `cd ml-service && make ci-setup`
  - `cd ml-service && make ci`
  - `cd ml-service && make validate-exports` (when `../output/go-app/*encoded*.csv` exist)
  - `cd ml-service && make validate-exports-infer` (when `../output/go-app/batting_infer_*.csv` exist)
- End-to-end (optional):
  - `cd go-app && make migrate`
  - Ingest fixture: `go run ./cmd/cricsheet-importer --dir ../tests/fixtures/cricsheet`
  - Unified export: `GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset --unified`
  - `cd ml-service && make validate-exports`

## Notes / Risks / Mitigations
- Header order must match exporters exactly. We append the 5 new columns at the tail in all schemas.
- Defaulting to zeros preserves backward compatibility for older datasets and historical artifacts.
- Keep tests small, deterministic, and offline. No external downloads.
- Do not modify `src/` directory (read-only by project rule).

## Branching & PRs
- Branch: `feat/phase3-ml-fielding-schema`
- Conventional commits:
  - `feat(ml): accept fielding columns in dataset schemas`
  - `feat(ml): default missing fielding columns to zeros in loaders`
  - `test(ml): add schema and loader tests for fielding columns`
  - `chore(ci): ensure export validators accept fielding columns`
