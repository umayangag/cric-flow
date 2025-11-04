### Project Overview

- Name: Cric App (Monorepo)
- Purpose: End-to-end cricket data platform that ingests Cricsheet JSON into Postgres, computes features/metrics, exports ML-ready datasets, trains ML models, and serves predictions via a Python FastAPI ML service. A Go service provides API endpoints for orchestration and data access. The `src/` directory contains the original Python prototype and must remain read-only as a reference.

### Architecture
- go-app (Go 1.25+)
  - CLI tools: migrations runner, Cricsheet importer, dataset exporter, team predictor
  - HTTP API: health/readiness, orchestration endpoints (e.g., `/precompute`, `/import/cricsheet`)
  - Integrates with Postgres and calls the ML service (`ML_SERVICE_URL`)
- ml-service (Python 3.10+)
  - FastAPI app served by uvicorn
  - Feature precomputation and model training scripts
  - Serves prediction endpoints and utilities
- postgres (Docker Compose)
  - Default DB: `cricket_data`, user/password: `postgres`/`postgres`
- src (Python prototype)
  - Kept intact for reference only; do not modify. Used to guide reimplementation.

### Key Data Flows
1. Import Cricsheet JSON → Postgres (Go importer)
2. Precompute features/metrics (primarily in ML service now)
3. Export ML-ready CSV datasets (Go exporter) to `output/go-app/`
4. Train ML artifacts (Python) and load into ML service
5. Go API orchestrates and exposes endpoints; ML service handles inference.

### Runtime & Ports
- Postgres: 5432
- Go API: 8080 (Docker and local)
- ML Service: 8000 (Docker and local)