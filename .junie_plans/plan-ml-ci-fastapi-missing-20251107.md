# Plan: Fix ml-service CI ImportError (FastAPI missing) — 2025-11-07

Owner: Junie
Scope: Python CI for ml-service only

## Problem Statement
CI fails during pytest collection with `ModuleNotFoundError: No module named 'fastapi'` because the workflow calls `make -C ml-service ci-setup`, which currently installs only dev tools and not runtime dependencies from `requirements.txt`.

## Objectives
- Ensure CI installs runtime dependencies (FastAPI, etc.) before running tests.
- Keep CI DRY by delegating to Makefile targets (no duplicated logic in workflows).
- Preserve existing coverage gate (>= 80%) and artifacts (`coverage.xml`).

## Changes
1) ml-service/Makefile
- Update target `ci-setup` to also install runtime dependencies:
  - `python -m pip install --upgrade pip`
  - `python -m pip install -r requirements.txt`
  - `python -m pip install black isort flake8 ruff pytest pytest-cov`
- Keep all other targets unchanged.

2) Workflows (no functional change required)
- `.github/workflows/ml-ci.yml` and `.github/workflows/ml-service-ci.yml` already call `make -C ml-service ci-setup` then `make -C ml-service ci`. These should pass after Makefile fix.

## Acceptance Criteria
- GitHub Actions for ml-service complete successfully.
- `pytest` runs without import errors; coverage gate `COV_MIN=80` passes.
- Coverage artifact `ml-service/coverage.xml` is uploaded in CI.
- Local reproduction succeeds:
  - `make -C ml-service ci-setup`
  - `COV_MIN=80 make -C ml-service ci`

## Verification Commands
- Local:
```
make -C ml-service ci-setup
COV_MIN=80 make -C ml-service ci
ls ml-service/coverage.xml
```
- CI: Observe green checks on PR with ml-service workflow logs showing successful dependency install and test run.

## Risks & Mitigations
- Risk: Version resolution differences on runner vs local.
  - Mitigation: Pin runtime deps in `requirements.txt` (already pinned); use `python -m pip` to ensure correct interpreter.

## Branching & Commits
- Branch: `fix/ml-ci-fastapi-missing`
- Commit message:
  - `ci(ml-service): install runtime deps in ci-setup to fix pytest import errors`

## Notes
- No application code changes are required.
- Optionally, we could add an explicit `import fastapi` smoke step in `ci` target, but unnecessary after `ci-setup` fix.
