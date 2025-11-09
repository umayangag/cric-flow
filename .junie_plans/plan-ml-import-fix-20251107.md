# Plan: Fix ImportError in ml-service (relative import beyond top-level package)

Date: 2025-11-07
Owner: Junie
Scope: Resolve `ImportError: attempted relative import beyond top-level package` triggered in `ml-service/app/main.py` during tests/CI, and ensure tests & coverage pass deterministically.

## Problem Statement
CI reported:
```
from ..ml.match_win_predict import predict_for_team
ImportError: attempted relative import beyond top-level package
```
The `app` package is placed at the top level under `ml-service/` alongside the `ml/` package. Using `..ml` attempts to escape beyond the package root when importing `app.main` as `app.main` in tests.

## Root Cause
- `app.main` used a relative import: `from ..ml.match_win_predict import predict_for_team`.
- When `app` is a top-level package (imported as `app.main` with CWD=ml-service), `..ml` is invalid.

## Solution Overview
- Switch to absolute import: `from ml.match_win_predict import predict_for_team`.
- Ensure `ml/` is a Python package (`ml/__init__.py` present) so `ml` resolves correctly.
- Keep pytest runs and CI working directory as `ml-service/` so `.` is on `PYTHONPATH`.

## Files to Modify
- ml-service/app/main.py
  - Change import to absolute: `from ml.match_win_predict import predict_for_team`.

(Optional hardening; only if subsequent local runs show path issues)
- ml-service/Makefile
  - Prefix `test`, `coverage`, and `coverage-check` commands with `PYTHONPATH=.` to make resolution explicit.

## Acceptance Criteria
- `pytest` in `ml-service` runs without ImportError.
- Coverage report `ml-service/coverage.xml` is generated.
- Coverage gate passes: total coverage ≥ 80% (configured via `COV_MIN`).
- Endpoints using team prediction (`/predict-win`, `/predict/win`) import and execute without import errors.

## Verification Commands
1) Setup (installs runtime + dev tools):
- `make -C ml-service ci-setup`

2) Run tests with coverage (xml + term):
- `make -C ml-service coverage`

3) Enforce coverage threshold (80%):
- `COV_MIN=80 make -C ml-service coverage-check`

4) Full CI target locally:
- `COV_MIN=80 make -C ml-service ci`

5) Smoke run of app (optional):
- `make -C ml-service run` and GET `http://localhost:8000/health`

## Rollout Steps
1. Create feature branch: `git checkout -b fix/ml-absolute-import`
2. Commit changes:
   - `fix(ml-service): use absolute import for match_win_predict to avoid package escape`
   - (optional) `ci(ml-service): harden pytest path by setting PYTHONPATH=.`
3. Push branch and open PR: "ml-service: fix ImportError from relative import; stabilize test imports".
4. Ensure CI is green.

## Risks & Mitigations
- Risk: Local runs outside `ml-service/` directory may fail to import `ml`.
  - Mitigation: Document running tests from `ml-service/` and optionally set `PYTHONPATH=.` in Makefile test/coverage targets.
- Risk: Hidden transitive imports in `ml/` modules.
  - Mitigation: Run full test suite and exercise `/predict-win` endpoints in a smoke test.
