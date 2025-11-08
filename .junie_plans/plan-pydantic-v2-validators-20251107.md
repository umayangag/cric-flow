# Plan: Migrate Pydantic v1 `@validator` to v2 `@field_validator` (2025-11-07)

Owner: Junie
Scope: ml-service (Python)

## Problem Statement
CI/logs show `PydanticDeprecatedSince20: Pydantic V1 style @validator validators are deprecated`. We must migrate to Pydantic v2 `@field_validator` and keep behavior unchanged.

## Files to Modify
- ml-service/app/main.py

## High-level Changes
- Replace `from pydantic import BaseModel, Field, validator` with `from pydantic import BaseModel, Field, field_validator`.
- For models `BattingFeatures` and `BowlingFeatures`:
  - Replace `@validator("format")` with `@field_validator("format", mode="before")`.
  - Preserve normalization: allow `None`, strip and uppercase non-null values, accept values even if not in the known set (route-level validation enforces formats when required).

## Acceptance Criteria
- No deprecation warning about Pydantic V1 validators during tests.
- All Python tests pass locally and in CI.
- Coverage gate (>= 80%) passes and `ml-service/coverage.xml` is generated.
- No change in API behavior for `format` handling.

## Verification Commands
- Setup (CI-equivalent):
  - `make -C ml-service ci-setup`
- Run CI pipeline locally:
  - `COV_MIN=80 make -C ml-service ci`
- Direct pytest run:
  - `cd ml-service && .venv/bin/python -m pytest -q`

## Risks & Mitigations
- Risk: Input types to validators differ under v2 `mode="before"`.
  - Mitigation: Guard `None` explicitly and operate on string values only; logic mirrors previous behavior.

## Branching & Commits
- Branch: `fix/pydantic-v2-validators`
- Commits:
  - `refactor(ml-service): migrate Pydantic @validator to @field_validator (v2)`
  - `docs(ml-service): add plan for Pydantic v2 validator migration`
