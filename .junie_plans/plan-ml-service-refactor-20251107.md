# ML-Service Refactor Plan (Phase 1)

Date: 2025-11-07
Owner: Junie (AI)
Scope: Refactor ml-service Python code to align with guidelines without changing external behavior.

## Goals
- Improve maintainability and readability (KISS, SRP, modularity).
- Centralize configuration, logging, error responses, and artifacts handling.
- Keep API behavior stable; add tests to lock current behavior.

## Out of Scope
- Model changes or retraining.
- New endpoints or features.
- Performance optimizations beyond structural cleanup.

## Files to Create/Modify

Create:
- ml-service/app/errors.py
  - Error payload factory and Pydantic error schemas.
- ml-service/app/features.py
  - Helpers for batting/bowling feature vector construction.
- ml-service/app/artifacts.py
  - Resolve models dir; load/reload artifacts into registries (bat/bowl).
- ml-service/app/settings.py
  - Minimal settings with environment precedence consistent with current behavior.
- ml-service/app/logging.py
  - Singleton logger factory with sensible defaults.
- ml-service/tests/test_health.py
  - Smoke tests for /health output structure.
- ml-service/tests/test_predict_validation.py
  - Validation/error payload tests for predict endpoints.
- ml-service/tests/test_feature_vectors.py
  - Unit tests for feature vector helpers.
- ml-service/.env.example
  - Document env vars (ML_SERVICE_OUTPUT_DIR, ENABLE_HOT_RELOAD, ML_SERVICE_CONFIG).

Modify:
- ml-service/app/main.py
  - Use new modules: settings, logging, artifacts, features, errors.
  - Keep routes and response models intact; remove duplicate logic.
- ml-service/README.md
  - Add notes about settings, running tests, and env vars.
- ml-service/Makefile (only if needed)
  - Optional: add a `test` target for pytest.

## High-Level Changes per File

- app/settings.py: provide `get_models_dir()` following precedence: `ML_SERVICE_OUTPUT_DIR` then `MODELS_DIR` then config default then built-in fallback.
- app/logging.py: provide `get_logger()` singleton with INFO level and structured prefix.
- app/errors.py: `_error_payload(code, message, hint=None, available=None)` and `ErrorDetail` Pydantic model for structure.
- app/features.py: `_batting_feature_vector(...)`, `_bowling_feature_vector(...)` pure functions.
- app/artifacts.py:
  - `load_artifacts(models_dir)` -> dict summaries; manage `BAT_MODELS`, `BOWL_MODELS` registries; expose `reload(models_dir)`.
  - Mirror legacy + per-format loading identical to current logic.
- app/main.py:
  - Import from above modules; initialize models dir via settings; call artifacts loader; implement `/admin/reload` via artifacts.
  - Keep existing request/response schemas.
- tests/*.py: use FastAPI TestClient to exercise routes; avoid real model files; assert error payload structure and health keys.

## Tests to Add/Update

- test_health.py:
  - GET /health returns 200 with keys: status, models_dir, artifacts, metadata, counters, loaded_* and legacy_* flags.
- test_predict_validation.py:
  - POST /predict/batting with [] -> 400 and code=EMPTY_BATCH.
  - POST /predict/bowling with [] -> 400 and code=EMPTY_BATCH.
  - POST /predict/batting with mixed formats -> 400 and code=MIXED_FORMATS.
- test_feature_vectors.py:
  - Construct minimal feature objects and ensure vectors length and values match order.

## Acceptance Criteria

- Code compiles and server starts: `make -C ml-service init && make -C ml-service run` (manual run).
- Formatting/lint checks pass:
  - `make -C ml-service fmt-check`
  - `make -C ml-service lint-check`
- Tests pass locally (no network, no real artifacts needed):
  - `cd ml-service && .venv/bin/python -m pytest -q`
- API behavior preserved for validation and health responses as per tests.

## Verification Commands

- Initialize venv and tools:
  - `make -C ml-service init`
- Run linters/formatters:
  - `make -C ml-service fmt-check && make -C ml-service lint-check`
- Run tests:
  - `cd ml-service && .venv/bin/python -m pytest -q`

## Branching & Commits

- Create feature branch: `git checkout -b refactor/ml-service-structure`
- Use Conventional Commits:
  - `refactor(ml-service): extract artifacts, features, errors, settings, logging`
  - `test(ml-service): add FastAPI and unit tests for validation and helpers`
  - `docs(ml-service): document env vars and testing`

## Risks & Mitigations

- Import path issues after refactor -> keep module names under `app/` and adjust relative imports carefully.
- Hidden behavior changes -> tests lock the validation paths; avoid touching prediction math.

