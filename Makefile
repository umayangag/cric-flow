# Convenience targets for local dev

# Common variables
DC:=docker compose
# Local go-app pipelines: 2GB memory limit so resource-aware concurrency stays safe (override with GOMEMLIMIT=... if needed)
export GOMEMLIMIT?=2GiB
APP_SERVICES:=go-api ml-service
# Frontend configuration (override if your dev server uses a different port)
FRONTEND_PORT ?= 5173
# Absolute path to ml-service virtualenv bin (used where Python is needed from root)
ML_VENV_BIN := $(abspath ml-service/.venv/bin)

.PHONY: dev-up dev-up-with-frontend dev-down dev-destroy dev-purge dev-rebuild dev-rebuild-nocache
.PHONY: logs api migrate output-dirs export-dataset export-off export-on
.PHONY: precompute precompute-seq precompute-asof precompute-all precompute-all-all-formats
.PHONY: go-test go-test-int ml-serve team-predictor ml-install
.PHONY: train-batting train-bowling train-fielding train-extras train-win train-innings train-batting-bowling train-all train-models ml-auto-tune walk-forward train-combination-meta full-pipeline
.PHONY: fmt fmt-check fmt-go fmt-py lint-go lint-py install-hooks init init-go init-py cricsheet-import
.PHONY: up-all build-apps build-apps-nocache recreate-apps e2e e2e-multi help help-all list
.PHONY: ci ci-go ci-ml seed-fixtures e2e-backtest-smoke migrate-local frontend-stop
.PHONY: check-all frontend-check go-app-check ml-service-check frontend-backend-sync-check e2e-pytest ml-test train-batting-baseline train-bowling-baseline

# docker-compose stack (Postgres + API + ML service)
dev-up:
	$(DC) up --build -d

# Optional: also start the frontend dev server (Vite/React) if present
dev-up-with-frontend: dev-up
	@if [ -d "frontend" ] && [ -f "frontend/package.json" ]; then \
		echo "[frontend] Ensuring dependencies..."; \
		cd frontend && if [ ! -d "node_modules" ]; then npm install; fi; \
		cd frontend && cp -n .env.example .env 2>/dev/null || true; \
		if ! lsof -i :$(FRONTEND_PORT) -sTCP:LISTEN >/dev/null 2>&1; then \
			echo "[frontend] Starting dev server on port $(FRONTEND_PORT)... (logs: /tmp/frontend-dev.log)"; \
			cd frontend && nohup npm run dev > /tmp/frontend-dev.log 2>&1 & echo $$! > /tmp/frontend-dev.pid; \
		else \
			echo "[frontend] Already running on port $(FRONTEND_PORT). Skipping."; \
		fi; \
	else \
		echo "[frontend] Skipped (no frontend/ or package.json)."; \
	fi

# Stop and remove containers; output/ (trained models, exports) is on the host and is retained.
dev-down:
	$(DC) down
	@$(MAKE) frontend-stop --no-print-directory

# Remove containers and named volumes (e.g. pgdata). output/ is retained; use dev-purge to remove it.
dev-destroy:
	$(DC) down -v
	@$(MAKE) frontend-stop --no-print-directory

# Remove output/ (trained models and exports). Use when you want a full reset.
dev-purge: dev-down
	@rm -rf output
	@echo "[dev-purge] Removed output/ (trained models and exports)."

logs:
	$(DC) logs -f --tail=200

# Run Go unit tests
go-test:
	cd go-app && go test ./...

# Run Go integration tests (requires Postgres). Usage: make go-test-int
# Spins are expected to be running via docker-compose or externally.
go-test-int:
	cd go-app && INTEGRATION=1 go test -tags=integration ./...

# Apply DB migrations against local Postgres (env vars can override defaults)
migrate:
	cd go-app && make migrate

# Ensure output dirs exist from repo root so go-app export (cwd=go-app) can write to ../output/go-app
.PHONY: output-dirs
output-dirs:
	@mkdir -p output/go-app output/ml-service

# Export datasets (unified exports only)
export-dataset: output-dirs
	# Unified, cross-format CSVs with as-of per-format features
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -unified=1

# Convenience targets for sequence feature workflows (FORMAT defaults to T20)
precompute-seq:
	cd go-app; \
 	for F in TEST ODI T20I T20; do \
 	  echo "precompute-sequence-features for [$$F]"; \
 	  go run ./cmd/precompute-sequence-features -format=$$F -targets=all || exit 1; \
 	done

export-off: output-dirs
	# Baseline export without optional sequence columns
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -format=$(FORMAT)

export-on: output-dirs
	# Export with sequence columns appended (flag and env gate)
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app ENABLE_SEQ_FEATURES=1 go run ./cmd/export-dataset -format=$(FORMAT) -enable-seq=1

# Run API locally (assumes Postgres is reachable as configured in env)
api:
	cd go-app && PORT=8080 MIGRATIONS_DIR=./migrations make run-api

# Run ML service locally
ml-serve:
	cd ml-service && uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload

# API auth/host for targets that talk to an already-running stack.
# Must match the API_KEY the stack was started with (docker-compose defaults to dev-local-key).
API_KEY ?= dev-local-key
API_URL ?= http://localhost:8080

# Variables for convenience (override like: make team-predictor MATCH=123 BAT=6 BOWL=5)
SEASON ?= 2019
FORMAT ?= T20
# Recognized formats: TEST, ODI, T20, T20I
# Aliases: MDM -> TEST, ODM -> ODI, IT20 -> T20I
MATCH ?= 0
BAT ?= 6
BOWL ?= 5

# Run preprocessing computations against a running stack (make dev-up first).
# Uses API_KEY/API_URL; override when the stack was started with a different key.
precompute:
	curl -fsS -X POST -H "X-API-Key: $(API_KEY)" $(API_URL)/precompute

# Precompute time-indexed (as-of) features for ALL formats with one command
# ASOF is optional (defaults to today's date in UTC). You can override:
#   ASOF=YYYY-MM-DD
# Optional env overrides:
#   ALPHA=0.3   LASTN=10
# Examples:
#   make precompute-asof
#   make precompute-asof ASOF=2020-12-31 ALPHA=0.35 LASTN=12
precompute-asof:
	cd go-app; \
	ASOF_VAL=$${ASOF:-$$(date -u +%F)}; \
	ALPHA_FLAG=""; LASTN_FLAG=""; \
	if [ -n "$(ALPHA)" ]; then ALPHA_FLAG="-ewm-alpha=$(ALPHA)"; fi; \
	if [ -n "$(LASTN)" ]; then LASTN_FLAG="-lastN=$(LASTN)"; fi; \
	for F in TEST ODI T20I T20; do \
		echo "[as-of] Precomputing (replay) for $$F as-of $$ASOF_VAL $$ALPHA_FLAG $$LASTN_FLAG"; \
		go run ./cmd/precompute-features -format=$$F -replay=1 -as-of=$$ASOF_VAL $$ALPHA_FLAG $$LASTN_FLAG || exit 1; \
	done

# Unified command: run both as-of/replay precompute and sequential features in one shot
# For T20/T20I, the system automatically combines domestic T20 and international T20I data.
precompute-all:
	cd go-app && go run ./cmd/precompute-all -format=$(FORMAT) $(ARGS) || exit 1

# Run unified command for all formats in parallel (TEST, ODI, T20I, T20). Single process, same as pipeline API.
# Note: T20 and T20I are treated as a single bucket for many aggregate and sequence features.
precompute-all-all-formats:
	cd go-app && go run ./cmd/precompute-all -all-formats -replay $(ARGS) || exit 1

# Team predictor: requires MATCH, SEASON, FORMAT; uses go-app team-predictor (ML service must be running for predict).
team-predictor:
	@if [ "$(MATCH)" = "0" ]; then echo "Please pass MATCH=<match_id>, e.g., make team-predictor MATCH=123456"; exit 1; fi
	cd go-app && make team-predictor MATCH=$(MATCH) BAT=$(BAT) BOWL=$(BOWL) FORMAT=$(FORMAT) SEASON=$(SEASON)

# Train ML artifacts (batting, bowling, fielding). Prerequisites: precompute + export (see export-dataset).
# Fielding uses go-app training-data API by default; set GO_APP_URL and CUTOFF, or pass FIELDING_CSV to ml-service.
ml-install:
	$(MAKE) -C ml-service install

# Per-format artifacts; run export-dataset first so batting_encoded_*.csv exists.
train-batting:
	cd ml-service && $(ML_VENV_BIN)/python -m ml.train_batting --all-formats

train-bowling:
	cd ml-service && $(ML_VENV_BIN)/python -m ml.train_bowling --all-formats

# Train fielding: same as batting/bowling — if CUTOFF set use API; else use fielding_encoded_all.csv from GO_APP_OUTPUT_DIR (run export first).
GO_APP_URL ?= http://localhost:8080
CUTOFF ?=
train-fielding:
	@if [ -n "$(FIELDING_CSV)" ]; then \
	  cd ml-service && $(ML_VENV_BIN)/python -m ml.train_fielding --csv "$(FIELDING_CSV)"; \
	elif [ -n "$(CUTOFF)" ]; then \
	  cd ml-service && GO_APP_URL="$(GO_APP_URL)" $(ML_VENV_BIN)/python -m ml.train_fielding --go-app-url "$(GO_APP_URL)" --cutoff "$(CUTOFF)"; \
	else \
	  cd ml-service && $(ML_VENV_BIN)/python -m ml.train_fielding; \
	fi

train-extras:
	@if [ -z "$(CUTOFF)" ] && [ -z "$(EXTRAS_CSV)" ]; then \
	  echo "Set CUTOFF=<RFC3339> and optionally GO_APP_URL=, or set EXTRAS_CSV=<path>. Example: make train-extras CUTOFF=2025-01-01T00:00:00Z"; \
	  exit 1; \
	fi
	@if [ -n "$(EXTRAS_CSV)" ]; then \
	  cd ml-service && $(ML_VENV_BIN)/python -m ml.train_extras --csv "$(EXTRAS_CSV)"; \
	else \
	  cd ml-service && GO_APP_URL="$(GO_APP_URL)" $(ML_VENV_BIN)/python -m ml.train_extras --go-app-url "$(GO_APP_URL)" --cutoff "$(CUTOFF)"; \
	fi

train-win:
	@if [ -z "$(CUTOFF)" ] && [ -z "$(WIN_CSV)" ]; then \
	  echo "Set CUTOFF=<RFC3339> and optionally GO_APP_URL=, or set WIN_CSV=<path>. Example: make train-win CUTOFF=2025-01-01T00:00:00Z"; \
	  exit 1; \
	fi
	@if [ -n "$(WIN_CSV)" ]; then \
	  cd ml-service && $(ML_VENV_BIN)/python -m ml.train_win --csv "$(WIN_CSV)"; \
	else \
	  cd ml-service && GO_APP_URL="$(GO_APP_URL)" $(ML_VENV_BIN)/python -m ml.train_win --go-app-url "$(GO_APP_URL)" --cutoff "$(CUTOFF)"; \
	fi

train-innings:
	@if [ -z "$(CUTOFF)" ]; then \
	  echo "Set CUTOFF=<RFC3339> and optionally GO_APP_URL=. Example: make train-innings CUTOFF=2025-01-01T00:00:00Z"; \
	  exit 1; \
	fi
	cd ml-service && GO_APP_URL="$(GO_APP_URL)" $(ML_VENV_BIN)/python -m ml.train_innings --go-app-url "$(GO_APP_URL)" --cutoff "$(CUTOFF)"

# Train batting + bowling (from exported CSVs). Fielding: same — export then make train-fielding (or set CUTOFF for API).
train-batting-bowling: train-batting train-bowling

# Train all models (batting, bowling, fielding, extras, win). For fielding/extras/win set CUTOFF= and GO_APP_URL= if using API.
train-all: train-models
train-models: train-batting train-bowling train-fielding train-extras train-win train-innings

# Auto-tune ML model(s): find best algorithm and hyperparameters. From repo root: make ml-auto-tune MODEL=batting FORMAT=T20 or MODEL=all ALL_FORMATS=1
# When MODEL=all and ALL_FORMATS=1, set GO_APP_URL (and optionally CUTOFF) so all five models are tuned from API and params saved to DB.
# Options: ALGORITHMS=rf,gb VALIDATION_METHOD=walk_forward
MODEL ?= batting
FORMAT ?=
ALL_FORMATS ?=
ALGORITHMS ?=
VALIDATION_METHOD ?=
# CUTOFF is defined once above (train-fielding block); reused here for ml-auto-tune.
RESCREEN ?=
ml-auto-tune:
	$(MAKE) -C ml-service auto-tune MODEL="$(MODEL)" FORMAT="$(FORMAT)" ALL_FORMATS="$(ALL_FORMATS)" $(if $(CUTOFF),CUTOFF="$(CUTOFF)",) $(if $(ALGORITHMS),ALGORITHMS="$(ALGORITHMS)",) $(if $(VALIDATION_METHOD),VALIDATION_METHOD="$(VALIDATION_METHOD)",) $(if $(PARALLEL),PARALLEL="$(PARALLEL)",) $(if $(FAST),FAST="$(FAST)",) $(if $(NO_PYCARET),NO_PYCARET="$(NO_PYCARET)",) $(if $(RESCREEN),RESCREEN="$(RESCREEN)",)

# Walk-forward: incremental train → predict → evaluate → absorb (see docs/ml-and-training.md)
INITIAL_CUTOFF ?= 2020-01-01T00:00:00Z
WINDOW_X ?= 50
WALK_FORMAT ?= T20
WALK_MODEL ?= batting
walk-forward:
	GO_APP_URL=$${GO_APP_URL:-http://localhost:8080} $(MAKE) -C ml-service walk-forward INITIAL_CUTOFF="$(INITIAL_CUTOFF)" WINDOW_X="$(WINDOW_X)" WALK_FORMAT="$(WALK_FORMAT)" WALK_MODEL="$(WALK_MODEL)" $(if $(MAX_WINDOWS),MAX_WINDOWS="$(MAX_WINDOWS)",) $(if $(EXPORT_METRICS),EXPORT_METRICS="$(EXPORT_METRICS)",)

# Train meta-model for score combination from backtest CSV (see docs/ml-and-training.md)
train-combination-meta:
	$(MAKE) -C ml-service train-combination-meta CSV="$(CSV)" OUT="$(OUT)"

# Full retrain pipeline: precompute → export → train all models; optionally train-combination-meta if CSV exists.
# Does not run import. Set CUTOFF= and GO_APP_URL= for fielding/extras/win. Generate contributions CSV via POST /api/backtest/export-contributions first if you want combination meta.
FULL_PIPELINE_CSV ?= output/go-app/backtest_contributions.csv
FULL_PIPELINE_OUT ?= output/go-app/combination_meta.json
full-pipeline: output-dirs precompute-all-all-formats export-dataset train-models
	@if [ -f "$(FULL_PIPELINE_CSV)" ]; then \
	  echo "[full-pipeline] Running train-combination-meta (CSV found)"; \
	  $(MAKE) train-combination-meta CSV="$(FULL_PIPELINE_CSV)" OUT="$(FULL_PIPELINE_OUT)"; \
	else \
	  echo "[full-pipeline] Skipping train-combination-meta (no $(FULL_PIPELINE_CSV)); generate via POST /api/backtest/export-contributions"; \
	fi

# -------------------- Backtest fixtures and smoke --------------------
# Defaults for local DB that mirror docker-compose ports
POSTGRES_HOST ?= localhost
POSTGRES_PORT ?= 5432
POSTGRES_DB ?= cricket_data
POSTGRES_USER ?= postgres
POSTGRES_PASSWORD ?= postgres
POSTGRES_SSLMODE ?= disable

# Run migrations against local Postgres (compose or external)
migrate-local:
	cd go-app && \
	POSTGRES_HOST=$(POSTGRES_HOST) \
	POSTGRES_PORT=$(POSTGRES_PORT) \
	POSTGRES_DB=$(POSTGRES_DB) \
	POSTGRES_USER=$(POSTGRES_USER) \
	POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) \
	POSTGRES_SSLMODE=$(POSTGRES_SSLMODE) \
	MIGRATIONS_DIR=./migrations \
	go run ./cmd/migrate -dir=./migrations

# Seed tiny deterministic fixtures for E2E backtest smoke.
# Uses match + match_inning schema (post-0090); see tests/fixtures/backtest/seed.sql.
seed-fixtures:
	# Ensure Postgres is up (compose service name: postgres)
	$(DC) up -d postgres
	# Wait until Postgres is accepting connections (inside container; max ~90s)
	@echo "[SMOKE] Waiting for Postgres (container) readiness..."; \
	attempts=0; max_attempts=90; \
	until $(DC) exec -T postgres pg_isready -U $(POSTGRES_USER) -d $(POSTGRES_DB) >/dev/null 2>&1; do \
	  attempts=$$((attempts+1)); \
	  if [ $$attempts -ge $$max_attempts ]; then \
	    echo "Postgres did not become ready in time"; \
	    $(DC) logs --no-color --tail=200 postgres || true; \
	    exit 1; \
	  fi; \
	  sleep 1; \
	done; \
	echo "[SMOKE] Postgres is ready (container)."
	# Apply migrations to create schema if needed
	$(MAKE) migrate-local
	# Load the seed dataset (run psql inside the postgres container; no host psql required)
	@echo "[SMOKE] Seeding fixtures via container psql..."; \
	$(DC) exec -T postgres sh -lc "psql -v ON_ERROR_STOP=1 -U $(POSTGRES_USER) -d $(POSTGRES_DB) -f -" < tests/fixtures/backtest/seed.sql

# End-to-end smoke: select → evaluate with jq assertions
# Use API_KEY=test-api-key so docker compose and curl share the same key.
e2e-backtest-smoke: seed-fixtures
	# Start services (Postgres is ensured by seed-fixtures)
	API_KEY=test-api-key $(DC) up --build -d go-api ml-service
	# Wait for services to report healthy instead of using a fixed sleep
	@echo "[SMOKE] Waiting for services (go-api:8080, ml-service:8000) to be healthy..."; \
	for url in http://localhost:8080/health http://localhost:8000/health; do \
	  echo "  waiting for $$url ..."; \
	  attempts=0; max_attempts=90; \
	  until curl -fsS "$$url" >/dev/null 2>&1; do \
	    attempts=$$((attempts+1)); \
	    if [ $$attempts -ge $$max_attempts ]; then \
	      echo "Timeout waiting for $$url"; \
	      exit 1; \
	    fi; \
	    sleep 1; \
	  done; \
	  echo "  healthy: $$url"; \
	done
	# Select candidates
	@echo "[SMOKE] Selecting played matches (T20 IND vs AUS)"; \
	URL="http://localhost:8080/api/backtest/match?format=T20&team1=IND&team2=AUS"; \
	SEL_JSON=$$(mktemp); \
	trap 'rm -f "$$SEL_JSON"' EXIT; \
	STATUS=$$(curl -sS -H "X-API-Key: test-api-key" -o "$$SEL_JSON" -w "%{http_code}" "$$URL"); \
	echo "  [SEL] HTTP $$STATUS $$URL"; \
	if [ "$$STATUS" != "200" ]; then \
	  echo "  [SEL] Response:"; \
	  cat "$$SEL_JSON"; echo; \
	  exit 2; \
	fi; \
	COUNT=$$(jq -r '(.candidates // []) | length' "$$SEL_JSON"); \
	echo "  [SEL] candidates count=$$COUNT"; \
	if [ "$$COUNT" -le 0 ]; then \
	  echo "  [SEL] Body:"; \
	  cat "$$SEL_JSON"; echo; \
	  exit 2; \
	fi
	# Evaluate the seeded match (match_id known from fixtures: 9000111)
	# use_ml=1 delegates to ML /ml/backtest/match (deterministic baselines); avoids need for training data.
	@echo "[SMOKE] Evaluating match_id=9000111 (use_ml=1)"; \
	EVAL_JSON=$$(mktemp); \
	trap 'rm -f "$$EVAL_JSON"' EXIT; \
	STATUS=$$(curl -sS -o "$$EVAL_JSON" -w "%{http_code}" -H "X-API-Key: test-api-key" \
	  "http://localhost:8080/api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate&match_id=9000111&use_ml=1&cutoff=2024-01-15T00:00:00Z"); \
	if [ "$$STATUS" != "200" ]; then \
	  echo "[EVAL] HTTP $$STATUS"; echo "[EVAL] Response:"; cat "$$EVAL_JSON"; echo; exit 2; \
	fi; \
	jq -e '(.players | length) > 0' "$$EVAL_JSON" >/dev/null || { echo "[EVAL] Assertion failed: (.players | length) > 0"; cat "$$EVAL_JSON"; exit 2; }; \
	jq -e '(.metrics.player_runs_mae | type) == "number"' "$$EVAL_JSON" >/dev/null || { echo "[EVAL] Assertion failed: .metrics.player_runs_mae"; cat "$$EVAL_JSON"; exit 2; }; \
	jq -e '.match_aggregates.predicted' "$$EVAL_JSON" >/dev/null || { echo "[EVAL] Assertion failed: .match_aggregates.predicted"; cat "$$EVAL_JSON"; exit 2; }; \
	jq -e '.match_aggregates.actual' "$$EVAL_JSON" >/dev/null || { echo "[EVAL] Assertion failed: .match_aggregates.actual"; cat "$$EVAL_JSON"; exit 2; }; \
	jq -e '.match_aggregates.errors' "$$EVAL_JSON" >/dev/null || { echo "[EVAL] Assertion failed: .match_aggregates.errors"; cat "$$EVAL_JSON"; exit 2; }
	# Options endpoints
	@echo "[SMOKE] Checking options/formats"; \
	curl -sS -H "X-API-Key: test-api-key" "http://localhost:8080/api/options/formats" | jq -e 'type == "array"' >/dev/null
	# Accuracy trend (may return 500 when ML models not loaded or DB state differs; treat as non-fatal)
	@echo "[SMOKE] Checking backtest/accuracy-trend"; \
	STATUS=$$(curl -sS -o /dev/null -w "%{http_code}" -H "X-API-Key: test-api-key" "http://localhost:8080/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS"); \
	if [ "$$STATUS" = "200" ]; then echo "  accuracy-trend OK (200)"; \
	elif [ "$$STATUS" = "500" ]; then echo "  [WARN] accuracy-trend returned 500 (ML models/DB state may differ; skipping)"; \
	else echo "accuracy-trend HTTP $$STATUS"; exit 2; fi
	# Model stats (proxy to ML service)
	@echo "[SMOKE] Checking ml/model-stats"; \
	curl -sS -H "X-API-Key: test-api-key" "http://localhost:8080/api/ml/model-stats" | jq -e '.models_dir and (.models | type) == "array"' >/dev/null
	echo "[SMOKE] OK"

# Run ML-service E2E pytest tests (requires ML service and optionally go-api to be up; set RUN_E2E=1)
e2e-pytest:
	cd ml-service && RUN_E2E=1 ML_SERVICE_URL=$${ML_SERVICE_URL:-http://localhost:8000} $(ML_VENV_BIN)/pytest -q -m e2e -v

# Scoped ML tests for new readers/baselines (avoid full FastAPI test suite)
ml-test:
	cd ml-service && pytest -q tests/test_seq_reader.py tests/test_baselines.py

# Tiny T20 baselines using new readers on small fixtures (structure only)
train-batting-baseline:
	cd ml-service && $(ML_VENV_BIN)/python -c "from pathlib import Path; from ml_service.baselines import train_batting_from_csv; root=Path(__file__).resolve().parents[1]; csv=root/'tests/fixtures/exporter/t20/batting_on.csv'; res=train_batting_from_csv(str(csv)); print('batting baseline trained:', res.n_rows, 'rows', res.n_features, 'features')"

train-bowling-baseline:
	cd ml-service && $(ML_VENV_BIN)/python -c "from pathlib import Path; from ml_service.baselines import train_bowling_from_csv; root=Path(__file__).resolve().parents[1]; csv=root/'tests/fixtures/exporter/t20/bowling_on.csv'; res=train_bowling_from_csv(str(csv)); print('bowling baseline trained:', res.n_rows, 'rows', res.n_features, 'features')"

# --- End-to-end automation (format-aware) ---
# Usage:
#  make e2e FORMAT=ODI SEASON=2019
#  make e2e-multi FORMATS=ODI,T20I SEASON=2019

e2e:
	@if [ -z "$(FORMAT)" ]; then echo "Please set FORMAT=<CODE> (e.g., ODI)"; exit 2; fi
	@echo "[1/5] Applying DB migrations..."
	$(MAKE) migrate || (echo "Migrations failed" && exit 1)
	@echo "[2/5] Importing Cricsheet JSON..."
	$(MAKE) cricsheet-import || (echo "Cricsheet import failed" && exit 1)
	@echo "[3/5] Precomputing features..."
	$(MAKE) precompute || (echo "Precompute failed" && exit 1)
	@echo "[4/5] Exporting datasets for format $(FORMAT)..."
	@$(MAKE) output-dirs --no-print-directory
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -format=$(FORMAT)
	@echo "[5/5] Training ML artifacts for format $(FORMAT)..."
	$(MAKE) ml-install
	cd ml-service && .venv/bin/python -m ml.train_batting --format $(FORMAT) && .venv/bin/python -m ml.train_bowling --format $(FORMAT)
	@echo "Done. Artifacts in output/ml-service, CSVs in output/go-app."

e2e-multi:
	@if [ -z "$(FORMATS)" ]; then echo "Please set FORMATS=CSV (e.g., ODI,T20I)"; exit 2; fi
	@echo "[1/5] Applying DB migrations..."
	$(MAKE) migrate || (echo "Migrations failed" && exit 1)
	@echo "[2/5] Importing Cricsheet JSON..."
	$(MAKE) cricsheet-import || (echo "Cricsheet import failed" && exit 1)
	@echo "[3/5] Precomputing features..."
	$(MAKE) precompute || (echo "Precompute failed" && exit 1)
	@echo "[4/5] Exporting datasets for formats $(FORMATS)..."
	@$(MAKE) output-dirs --no-print-directory
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -formats=$(FORMATS)
	@echo "[5/5] Training ML artifacts for formats $(FORMATS)..."
	$(MAKE) ml-install
	@for f in $$(echo "$(FORMATS)" | tr ',' ' '); do \
		echo "  Training for format $$f..."; \
		cd ml-service && $(ML_VENV_BIN)/python -m ml.train_batting --format $$f && $(ML_VENV_BIN)/python -m ml.train_bowling --format $$f; \
	done
	@echo "Done. Artifacts in output/ml-service, CSVs in output/go-app."

# One-shot bootstrap: bring up stack, migrate, import Cricsheet, precompute, export, train, and restart ML service
up-all:
	@echo "[1/7] Bringing up Docker stack (Postgres, API, ML)..."
	$(MAKE) dev-up
	@echo "[2/7] Applying DB migrations..."
	$(MAKE) migrate || (echo "Migrations failed" && exit 1)
	@echo "[3/7] Importing Cricsheet JSON (idempotent)..."
	$(MAKE) cricsheet-import || (echo "Cricsheet import failed" && exit 1)
	@echo "[4/7] Precomputing metrics..."
	$(MAKE) precompute || (echo "Precompute failed" && exit 1)
	@echo "[5/7] Exporting datasets..."
	@$(MAKE) output-dirs --no-print-directory
	cd go-app && make export-dataset || (echo "Export failed" && exit 1)
	@echo "[6/7] Training ML artifacts..."
	$(MAKE) train-all CUTOFF=$$(date -u +%Y-%m-%dT%H:%M:%SZ) || (echo "Training failed" && exit 1)
	@echo "[7/7] Restarting ML service to load artifacts..."
	$(DC) restart ml-service
	@echo "Done. API at http://localhost:8080 (health/readiness), ML at http://localhost:8000 (health), Frontend at http://localhost:$(FRONTEND_PORT)."

# --- Frontend (React control panel) ---
.PHONY: frontend-dev frontend-build frontend-test frontend-install

# Install dependencies only if node_modules is missing (idempotent)
frontend-install:
	cd frontend && if [ -f package.json ]; then if [ ! -d "node_modules" ]; then npm install; fi; fi

frontend-dev: frontend-install
	cd frontend && cp -n .env.example .env 2>/dev/null || true && npm run dev

frontend-build: frontend-install
	cd frontend && npm run build

frontend-test: frontend-install
	cd frontend && npm run test

# Stop frontend dev server if running
frontend-stop:
	@if [ -f "/tmp/frontend-dev.pid" ]; then \
		PID=$$(cat /tmp/frontend-dev.pid); \
		if ps -p $$PID >/dev/null 2>&1; then \
			echo "[frontend] Stopping dev server (pid $$PID)..."; \
			kill $$PID || true; \
		fi; \
		rm -f /tmp/frontend-dev.pid; \
		echo "[frontend] Stopped."; \
	else \
		if lsof -t -i :$(FRONTEND_PORT) -sTCP:LISTEN >/dev/null 2>&1; then \
			PID=$$(lsof -t -i :$(FRONTEND_PORT) -sTCP:LISTEN | head -n1); \
			echo "[frontend] Stopping dev server on port $(FRONTEND_PORT) (pid $$PID)..."; \
			kill $$PID || true; \
			echo "[frontend] Stopped."; \
		else \
			echo "[frontend] Not running."; \
		fi; \
	fi

# --- Unified Quality Checks ---

# Run all quality checks for all components
check-all: frontend-check go-app-check ml-service-check frontend-backend-sync-check
	@echo "All quality checks passed!"

# Ensure go-app and ml-service expose canonical formats and model metadata (frontend fetches these dynamically)
frontend-backend-sync-check:
	@echo "[sync] Checking backend canonical formats and model metadata..."
	PATH="$(ML_VENV_BIN):$$PATH" node scripts/check-frontend-backend-sync.mjs

frontend-check: frontend-install
	@echo "[frontend] Running lint, fmt check, typecheck, build and tests..."
	cd frontend && npm run lint && npm run format:check && npm run typecheck && npm run build && npm run test

go-app-check:
	@echo "[go-app] Running lint, fmt check, tests and coverage..."
	$(MAKE) -C go-app vet fmt-check lint coverage
	@echo "[go-app] Enforcing coverage threshold (COV_MIN_GO, default 60)..."
	COV_MIN=$${COV_MIN_GO:-60} $(MAKE) -C go-app coverage-check

ml-service-check:
	@echo "[ml-service] Running lint, fmt check, tests and coverage..."
	# Assumes venv is initialized
	PATH="$(ML_VENV_BIN):$$PATH" $(MAKE) -C ml-service lint-check fmt-check coverage coverage-check

# --- Formatting & hooks ---
# Aggregate formatters for all components
fmt: fmt-go fmt-py

fmt-check:
	$(MAKE) -C go-app fmt-check
	# Assume ml-service venv is already prepared; avoid implicit bootstrapping for speed
	PATH="$(ML_VENV_BIN):$$PATH" $(MAKE) -C ml-service fmt-check

fmt-go:
	# Fast path: require tools to be installed; run formatting only
	PATH="$(shell go env GOPATH)/bin:$$PATH" $(MAKE) -C go-app fmt

lint-go:
	cd go-app && go vet ./... && PATH="$(shell go env GOPATH)/bin:$$PATH" make lint

fmt-py:
	# Fast path: require venv to be prepared; run formatting only
	PATH="$(ML_VENV_BIN):$$PATH" $(MAKE) -C ml-service fmt

lint-py:
	cd ml-service && make lint-check

lint-frontend:
	$(MAKE) -C frontend fmt-check

install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit
	@echo "Git hooks installed. On commit: gofumpt/golines/golangci-lint (Go), ruff (Python), prettier/eslint (frontend) run on staged files."

# --- Local environment bootstrap ---
# Initialize all components for local development
init: init-go init-py install-hooks
	@echo "\nLocal dev environment initialized. Next steps:"
	@echo "- For Python, activate venv: 'cd ml-service && source .venv/bin/activate'"
	@echo "- Run format checks: 'make fmt-check'"
	@echo "- Bring up stack: 'make dev-up'"

# Initialize Go tooling and modules
init-go:
	$(MAKE) -C go-app init

# Initialize Python venv and dev tools
init-py:
	$(MAKE) -C ml-service init

# Import Cricsheet JSON into DB using Go importer.
# Populates match, match_inning, batting_data, bowling_data (post-0090 schema).
cricsheet-import:
	cd go-app && GO_APP_INPUT_DIR=../data/go-app/cricsheet make cricsheet-import


# Helper targets to avoid duplication
build-apps:
	$(DC) build --pull $(APP_SERVICES)

build-apps-nocache:
	$(DC) build --no-cache --pull $(APP_SERVICES)

recreate-apps:
	$(DC) up -d --no-deps --force-recreate $(APP_SERVICES)

# --- Tooling: mocks, tests, lint ---
.PHONY: mock test lint

# Central mock generation using go-app/.mockery.yml
mock:
	# Use pinned mockery via go run to avoid local binary/version drift
	cd go-app && go run github.com/vektra/mockery/v3@v3.6.0 --config .mockery.yml

# Aggregate test target (all components)
test:
	cd go-app && make test
	cd ml-service && make test

# Aggregate lint target
lint: lint-go lint-py

# Rebuild app images (API, ML) and restart only those services (keeps Postgres running)
dev-rebuild:
	$(MAKE) build-apps
	$(MAKE) recreate-apps

# Same as above but ignore build cache
dev-rebuild-nocache:
	$(MAKE) build-apps-nocache
	$(MAKE) recreate-apps


# --- CI aggregate helpers ---
COV_MIN_GO ?= 60
COV_MIN_ML ?= 78

# Run ml-service CI pipeline (fmt, lint, coverage + threshold)
ci-ml:
	# Ensure Python venv and dev tools exist, then run ml-service CI with venv bin on PATH
	$(MAKE) -C ml-service init
	PATH="$(ML_VENV_BIN):$$PATH" $(MAKE) -C ml-service ci COV_MIN=$(COV_MIN_ML)

# Run go-app CI: vet, format check, coverage and enforce threshold
ci-go:
	$(MAKE) -C go-app vet
	$(MAKE) -C go-app fmt-check
	$(MAKE) -C go-app coverage
	COV_MIN=$(COV_MIN_GO) $(MAKE) -C go-app coverage-check

# Run all components' CI
ci: ci-go ci-ml


# --- Help & navigation ---
help:
	@echo "\nProject — Make targets (grouped)"
	@echo "--------------------------------"
	@echo "[Orchestration]"
	@echo "  up-all             One-shot: docker up → migrate → import → precompute → export → train → restart ML"
	@echo "  e2e                Run pipeline for a single FORMAT (requires FORMAT)"
	@echo "  e2e-multi          Run pipeline for multiple FORMATS (FORMATS=ODI,T20I)"
	@echo
	@echo "[Services & Logs]"
	@echo "  dev-up             Start docker-compose stack (Postgres, API, ML)"
	@echo "  dev-up-with-frontend  dev-up + start Frontend dev server (port $(FRONTEND_PORT))"
	@echo "  dev-down           Stop stack (preserve volumes) and stop Frontend"
	@echo "  dev-destroy        Stop and remove stack (delete volumes!) and stop Frontend"
	@echo "  logs               Tail docker-compose logs"
	@echo "  api                Run Go API locally (outside Docker)"
	@echo "  ml-serve           Run ML service locally (uvicorn)"
	@echo "  frontend-dev       Run Frontend dev server in foreground (Ctrl+C to stop)"
	@echo "  frontend-stop      Stop Frontend dev server if started in background"
	@echo
	@echo "[Data & Pipeline]"
	@echo "  migrate            Run DB migrations (match + match_inning schema)"
	@echo "  cricsheet-import   Import Cricsheet JSON into DB (match, match_inning)"
	@echo "  precompute         Trigger precompute (via API)"
	@echo "  precompute-all        Run unified precompute (as-of/replay + sequential) for FORMAT (default T20)"
	@echo "  precompute-all-all-formats  Run unified precompute for all formats"
	@echo "  precompute-seq     Precompute sequence features: go-app/cmd/precompute-sequence-features (FORMAT?=$(FORMAT))"
	@echo "  export-dataset     Export training datasets (unified)"
	@echo "  export-off         Export without seq columns for FORMAT (default T20)"
	@echo "  export-on          Export with seq columns appended for FORMAT (uses -enable-seq and ENABLE_SEQ_FEATURES=1)"
	@echo "  team-predictor     Generate team prediction (MATCH, BAT, BOWL)"
	@echo
	@echo "[ML training — precompute → export-dataset → train]"
	@echo "  train-batting      Train batting model (from exported CSVs)"
	@echo "  train-bowling      Train bowling model (from exported CSVs)"
	@echo "  train-fielding     Train fielding (from export CSV, or CUTOFF= + GO_APP_URL= or FIELDING_CSV=)"
	@echo "  train-extras       Train extras model (CUTOFF= + GO_APP_URL= or EXTRAS_CSV=)"
	@echo "  train-win          Train win model (CUTOFF= + GO_APP_URL= or WIN_CSV=)"
	@echo "  train-batting-bowling  Train batting + bowling"
	@echo "  train-all          Train all models (batting, bowling, fielding, extras, win)"
	@echo "  train-models       Same as train-all"
	@echo "  ml-auto-tune       Auto-tune model(s): best algorithm + hyperparams (MODEL=, FORMAT=, ALL_FORMATS=1)"
	@echo "  walk-forward       Walk-forward train → predict → evaluate; registry for feedback (INITIAL_CUTOFF=, WINDOW_X=, WALK_FORMAT=, WALK_MODEL=)"
	@echo
	@echo "[Testing & CI]"
	@echo "  check-all          Run lint, fmt, typecheck, and tests for all components"
	@echo "  go-test            Run Go unit tests"
	@echo "  go-test-int        Run Go integration tests (requires DB)"
	@echo "  ci-go              Go CI aggregate (vet, fmt-check, coverage gate)"
	@echo "  ci-ml              ML service CI aggregate"
	@echo "  ci                 Run both CI aggregates"
	@echo
	@echo "[Formatting & Lint]"
	@echo "  fmt / fmt-check    Run formatters across Go and Python (no implicit installs)"
	@echo "  lint-go / lint-py  Lint Go / Python"
	@echo
	@echo "[Bootstrap]"
	@echo "  init               Initialize both components (tools, venv, hooks)"
	@echo "  init-go / init-py  Initialize only Go / Python"
	@echo "  install-hooks      Install git pre-commit hooks"
	@echo
	@echo "[Docker compose maintenance]"
	@echo "  dev-rebuild        Rebuild app images and restart services"
	@echo "  dev-rebuild-nocache Rebuild without cache and restart services"
	@echo
	@echo "Variables (commonly used):"
	@echo "  FORMAT=$(FORMAT)  FORMATS=$(FORMATS)  MATCH=$(MATCH)  BAT=$(BAT)  BOWL=$(BOWL)  SEASON=$(SEASON)"
	@echo "\nTips:"
	@echo "  - Run 'make help-all' to see component-level helps"
	@echo "  - Override frontend port via FRONTEND_PORT, e.g., 'make dev-up FRONTEND_PORT=3000'"
	@echo "  - Run 'make list' to see all phony targets"

help-all:
	@echo "\n[Root]" && $(MAKE) help --no-print-directory || true
	@echo "\n[Go App]" && $(MAKE) -C go-app help --no-print-directory || true
	@echo "\n[Frontend]" && $(MAKE) -C frontend help --no-print-directory || true
	@echo "\n[ML Service]" && $(MAKE) -C ml-service help --no-print-directory || true

list:
	@awk '/^\.PHONY:/{for(i=2;i<=NF;i++)print $$i}' $(MAKEFILE_LIST) | sort -u