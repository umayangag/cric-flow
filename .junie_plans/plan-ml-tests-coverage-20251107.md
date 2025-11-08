# Plan: Fix ml-service Python tests and stabilize coverage (2025-11-07)

Owner: Junie
Scope: Make the ml-service test suite pass reliably under pytest and enforce a stable >=80% coverage gate in CI using Makefile targets and `.coveragerc`.

## Goals
- Convert brittle unittest-style tests to idiomatic pytest functions using fixtures (no `unittest.TestCase`).
- Ensure environment-dependent imports are reloaded safely in tests.
- Limit coverage measurement to the runtime package (`app/`) for stability.
- Keep CI workflows unchanged except for relying on Makefile targets.

## Files to Modify/Create
- Modify: `ml-service/tests/test_health.py` (convert to pytest function).
- Modify: `ml-service/tests/test_predict_validation.py` (convert to pytest functions, fix helpers/client creation).
- Modify: `ml-service/tests/test_feature_vectors.py` (convert to pytest functions).
- Create: `ml-service/.coveragerc` (coverage run/report config; scope to `app`).
- Modify: `ml-service/Makefile` (coverage targets use `.coveragerc` by passing `--cov` without explicit paths).
- (Optional, only if needed for margin) Add small tests for `/admin/reload` 403 and missing-format error path.

## High-level Changes
- Tests: remove `unittest.TestCase`, use `monkeypatch` and `tmp_path` fixtures, ensure `ML_SERVICE_OUTPUT_DIR` is set before importing `app.main`, then `importlib.reload` and construct `TestClient`.
- Coverage: add `.coveragerc` with `[run] source = app`, omit tests/venv, enable branch coverage; update Make targets to respect it.

## Acceptance Criteria
- `pytest -q` in `ml-service` passes locally (no failures, no import errors).
- `make -C ml-service coverage` produces `ml-service/coverage.xml` and shows a terminal report.
- `COV_MIN=80 make -C ml-service coverage-check` passes locally twice in a row (deterministic).
- GitHub Actions `ml-service` workflow passes with coverage >= 80% using Makefile targets.

## Verification Commands
- Initialize and tools:
  - `make -C ml-service init`
- Test run:
  - `cd ml-service && .venv/bin/python -m pytest -q`
- Coverage:
  - `make -C ml-service coverage`
  - `test -f ml-service/coverage.xml`
  - `COV_MIN=80 make -C ml-service coverage-check`
- CI-like aggregate:
  - `COV_MIN=80 make -C ml-service ci`

## Branching & Commits
- Branch: `fix/ml-tests-coverage`
- Conventional Commits:
  - `test(ml-service): convert tests to pytest and fix env-dependent imports`
  - `ci(ml-service): add .coveragerc and use it in coverage targets`
  - `docs(ml-service): update README testing/coverage notes (if needed)`

## Risks & Mitigations
- Risk: Import order may still leak env if not reloaded. Mitigate by setting env before import and calling `importlib.reload` in every test that needs a fresh state.
- Risk: Coverage dips if new files are added. Mitigate by scoping to `app/` and adding small tests if gate is close to threshold.
