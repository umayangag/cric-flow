# Prototype → Implementation Parity Checklist

This document tracks feature parity between the Python prototype in `src/` (read‑only) and the active implementations: `go-app/` (Go backend + ETL) and `ml-service/` (Python FastAPI ML inference/training).

Legend: [ ] planned, [~] partial, [x] complete, [!] blocked, [?] needs verification.

## 1) Data importers and DB lifecycle
- [x] DB schema creation & migrations
  - Prototype: `src/createdb/create_db.py`, `src/createdb/create_tables.py`, `src/createdb/queries/`.
  - Implementation: `go-app/migrations/*.sql`, `go-app/cmd/tools/migrate`.
  - Notes: migrations exist. Ensure up/down paths cover parity with prototype tables.
- [x] Cricsheet match details import
  - Prototype: `src/createdb/importers/import_match_details.py`
  - Implementation: `go-app/cmd/cricsheet-importer`, `go-app/internal/cricsheet/*`
  - Notes: present; verify field coverage vs prototype.
- [x] Batting data import
  - Prototype: `src/createdb/importers/import_batting_data.py`
  - Implementation: part of cricsheet importer + DB repos.
  - Notes: present by module structure; needs column-level verification. [?]
- [x] Bowling data import
  - Prototype: `src/createdb/importers/import_bowling_data.py`
  - Implementation: part of cricsheet importer + DB repos.
  - Notes: present; verify null handling & uniques. [?]
- [~] Keepers data import
  - Prototype: `src/createdb/importers/import_keepers_data.py`
  - Implementation: search indicates no dedicated command; may be merged into main importer.
  - Action: confirm presence; add task if missing. [?]
- [~] Retired players import/flagging
  - Prototype: `src/createdb/importers/import_retired.py`
  - Implementation: not clearly present; confirm approach in Go app. [?]
- [x] Weather data backfill & service integration
  - Prototype: `src/createdb/importers/import_weather_data.py`
  - Implementation: `go-app/cmd/weather-backfill`, `go-app/internal/weather/*`, `go-app/cmd/weather-worker`.
  - Notes: present; confirm rate limiting and retries.

## 2) Feature engineering and dataset exports
- [x] Dataset definitions / feature columns
  - Prototype: `src/team_selection/dataset_definitions.py`
  - Implementation: `ml-service/ml/dataset_definitions.py`, `go-app/internal/features/features_fmt.go`
- [~] Feature calculation & consistency metrics
  - Prototype: `src/team_selection/create_final_dataset.py`, `src/team_selection/shared/*`
  - Implementation: `ml-service/ml/calculate_features.py`, `go-app/internal/features/*`
  - Notes: parity likely; verify exact transformations and encodings. [?]
- [~] Export dataset CLI
  - Prototype: flows embedded in scripts
  - Implementation: `go-app/cmd/export-dataset`
  - Notes: compare exported schema against prototype; add validator in CI.

## 3) Model training, artifacts, and lifecycle
- [x] Batting & bowling model training
  - Prototype: `src/final_data/batting_regressor.py`, `src/final_data/bowling_regressor.py` (usage)
  - Implementation: `ml-service/ml/train_batting_model.py`, `ml-service/ml/train_bowling_model.py`
- [x] Inference code (regressors + win model)
  - Implementation: `ml-service/ml/batting_regressor.py`, `ml-service/ml/bowling_regressor.py`, `ml-service/ml/match_win_predict.py`
- [~] Model artifact locations/versioning
  - Implementation: `ml-service/app/main.py` loads from configurable models dir
  - Action: standardize artifact paths, add versioned filenames, and a symlink `latest`. [?]
- [~] Startup validation and hot reload
  - Implementation: `admin_reload` endpoint in FastAPI, `ENABLE_HOT_RELOAD`
  - Action: add checksum/metadata validation and fail-fast if artifacts missing. [?]

## 4) APIs and contracts
- [x] ML Service endpoints (FastAPI)
  - Implementation: `ml-service/app/main.py` provides `health`, `predict_batting`, `predict_bowling`, `predict_win`, `precompute`, `admin_reload`.
  - Action: document request/response schemas and examples; add input validation constraints.
- [~] Go API endpoints
  - Implementation: `go-app/cmd/api`, `go-app/internal/contracts`, `go-app/internal/mlclient`
  - Action: enumerate routes, ensure consistency with ML contracts; generate OpenAPI/Swagger if applicable.

## 5) Orchestration and idempotency
- [x] Makefile/targets for common flows
  - Top-level and per-component Makefiles exist.
- [~] Idempotent ETL/import, retries, checkpoints
  - Action: review importer upserts/uniques and weather worker job semantics; add retry/backoff and resume markers.

## 6) CI/CD & quality gates
- [ ] CI for Go (format/lint/tests)
- [ ] CI for Python (fmt/lint/tests)
- [ ] Docker build pipelines for both services
- [ ] Optional smoke test via docker-compose

## 7) Observability & ops
- [x] Health endpoints present (ML), TBD for Go API
- [~] Structured logging & correlation IDs
- [ ] Basic metrics (request counts, latency)
- [~] Env vars documented (partially in READMEs)

## 8) Documentation
- [~] Update READMEs with runbooks and API references
- [ ] Add CHANGELOG template

---

## Actionable Gap List (next)
1. Verify keepers/retired importer parity; add/implement if missing. [~][?]
2. Add dataset export validator (`ml-service/ml/validate_exports.py`) to CI; compare schemas vs prototype. [ ]
3. Define and publish API contracts (Pydantic models + Go structs) with examples and error schemas. [ ]
4. Standardize model artifact layout and implement startup validation + `latest` symlink. [ ]
5. Add idempotency/retry patterns to ETL and weather worker; document behavior. [ ]
6. Set up CI (lint/format/test) for Go and Python; add Docker builds and optional compose smoke test. [ ]
7. Document env vars and add sample `.env` files; confirm docker-compose uses them. [ ]
8. Add minimal metrics & structured logging guidelines; ensure request IDs propagated. [ ]

Owners: TBD (assign per repository).
