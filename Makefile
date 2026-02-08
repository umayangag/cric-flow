# Convenience targets for local dev

# Common variables
DC:=docker compose
APP_SERVICES:=go-api ml-service
# Frontend configuration (override if your dev server uses a different port)
FRONTEND_PORT ?= 5173
# Absolute path to ml-service virtualenv bin (used where Python is needed from root)
ML_VENV_BIN := $(abspath ml-service/.venv/bin)

.PHONY: dev-up dev-up-with-frontend dev-down dev-rebuild dev-rebuild-nocache logs api migrate export-dataset export-off export-on precompute precompute-seq precompute-asof precompute-all precompute-all-all-formats go-test go-test-int ml-serve team-predictor ml-install train-batting train-bowling train-all fmt fmt-check fmt-go fmt-py lint-go lint-py install-hooks init init-go init-py cricsheet-import up-all build-apps build-apps-nocache recreate-apps e2e e2e-multi help help-all list ci ci-go ci-ml seed-fixtures e2e-backtest-smoke migrate-local frontend-stop

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

dev-down:
	$(DC) down -v
	@$(MAKE) frontend-stop --no-print-directory

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
	cd go-app && go run ./cmd/migrate -dir=./migrations

# Export datasets (unified exports only)
export-dataset:
	# Unified, cross-format CSVs with as-of per-format features
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -unified=1

# Convenience targets for sequence feature workflows (FORMAT defaults to T20)
precompute-seq:
	cd go-app; \
 	for F in TEST ODI T20I T20; do \
 	  echo "precompute-sequence-features for [$$F]"; \
 	  go run ./cmd/precompute-sequence-features -format=$$F -targets=all || exit 1; \
 	done

export-off:
	# Baseline export without optional sequence columns
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -format=$(FORMAT)

export-on:
	# Export with sequence columns appended (flag and env gate)
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app ENABLE_SEQ_FEATURES=1 go run ./cmd/export-dataset -format=$(FORMAT) -enable-seq=1

# Run API locally (assumes Postgres is reachable as configured in env)
api:
	cd go-app && PORT=8080 MIGRATIONS_DIR=./migrations go run ./cmd/api

# Run ML service locally
ml-serve:
	cd ml-service && uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload

# Variables for convenience (override like: make team-predictor MATCH=123 BAT=6 BOWL=5)
SEASON ?= 2019
FORMAT ?= T20
MATCH ?= 0
BAT ?= 6
BOWL ?= 5

# Run preprocessing computations (happy path)
precompute:
	curl -X POST http://localhost:8080/precompute

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
precompute-all:
	cd go-app && go run ./cmd/precompute-all -format=$(FORMAT) $(ARGS) || exit 1

# Run unified command for all formats (order: TEST, ODI, T20I, T20)
precompute-all-all-formats:
	cd go-app; \
	for F in TEST ODI T20I T20; do \
		echo "[unified] precompute-all for $$F"; \
		go run ./cmd/precompute-all -format=$$F $(ARGS) || exit 1; \
	done

# Team predictor (happy path): requires MATCH to be provided
team-predictor:
	@if [ "$(MATCH)" = "0" ]; then echo "Please pass MATCH=<match_id>, e.g., make team-predictor MATCH=123456"; exit 1; fi
	cd ml-service && $(ML_VENV_BIN)/python -m ml.export_pool $(MATCH)
	cd go-app && go run ./cmd/team-predictor -match=$(MATCH) -bat=$(BAT) -bowl=$(BOWL)

# Train ML artifacts from exported CSVs
ml-install:
	$(MAKE) -C ml-service install

train-batting:
	cd ml-service && $(ML_VENV_BIN)/python ml/train_batting_model.py

train-bowling:
	cd ml-service && $(ML_VENV_BIN)/python ml/train_bowling_model.py

train-all: train-batting train-bowling

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

# Seed tiny deterministic fixtures for E2E backtest smoke
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
e2e-backtest-smoke: seed-fixtures
	# Start services (Postgres is ensured by seed-fixtures)
	$(DC) up --build -d go-api ml-service
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
	STATUS=$$(curl -sS -o "$$SEL_JSON" -w "%{http_code}" "$$URL"); \
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
	@echo "[SMOKE] Evaluating match_id=9000111"; \
	EVAL=$$(curl -s "http://localhost:8080/api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate&match_id=9000111"); \
	echo $$EVAL | jq -e '(.players | length) > 0' >/dev/null; \
	echo $$EVAL | jq -e '(.metrics.player_runs_mae | type) == "number"' >/dev/null; \
	echo $$EVAL | jq -e '.match_aggregates.predicted' >/dev/null; \
	echo $$EVAL | jq -e '.match_aggregates.actual' >/dev/null; \
	echo $$EVAL | jq -e '.match_aggregates.errors' >/dev/null; \
	echo "[SMOKE] OK"

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
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -format=$(FORMAT)
	@echo "[5/5] Training ML artifacts for format $(FORMAT)..."
	$(MAKE) ml-install
	cd ml-service && .venv/bin/python -m ml.train_batting_model --format $(FORMAT) && .venv/bin/python -m ml.train_bowling_model --format $(FORMAT)
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
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -formats=$(FORMATS)
	@echo "[5/5] Training ML artifacts for formats $(FORMATS)..."
	$(MAKE) ml-install
	@for f in $$(echo "$(FORMATS)" | tr ',' ' '); do \
		echo "  Training for format $$f..."; \
		cd ml-service && $(ML_VENV_BIN)/python -m ml.train_batting_model --format $$f && $(ML_VENV_BIN)/python -m ml.train_bowling_model --format $$f; \
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
	$(MAKE) export-dataset || (echo "Export failed" && exit 1)
	@echo "[6/7] Training ML artifacts..."
	$(MAKE) train-all || (echo "Training failed" && exit 1)
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
		# Fallback: try to find process by port if pid file is missing
		if lsof -t -i :$(FRONTEND_PORT) -sTCP:LISTEN >/dev/null 2>&1; then \
			PID=$$(lsof -t -i :$(FRONTEND_PORT) -sTCP:LISTEN | head -n1); \
			echo "[frontend] Stopping dev server on port $(FRONTEND_PORT) (pid $$PID)..."; \
			kill $$PID || true; \
			echo "[frontend] Stopped."; \
		else \
			echo "[frontend] Not running."; \
		fi; \
	fi

# --- Formatting & hooks ---

ML_VENV_BIN := $(abspath ml-service/.venv/bin)

# Aggregate formatters for both components
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
	@echo "Git hooks installed. On commit, gofumpt/golines (Go) and black/isort (Python) will run automatically."

# --- Local environment bootstrap ---
# Initialize both components for local development
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

# Import Cricsheet JSON into DB using Go importer
cricsheet-import:
	cd go-app && GO_APP_INPUT_DIR=../data/go-app/cricsheet go run ./cmd/cricsheet-importer


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

# Aggregate test target (Go only by default)
test:
	cd go-app && make test

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
COV_MIN_GO ?= 80
COV_MIN_ML ?= 80

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

# Run both components' CI
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
	@echo "  dev-down           Stop and remove stack (volumes) and stop Frontend"
	@echo "  logs               Tail docker-compose logs"
	@echo "  api                Run Go API locally (outside Docker)"
	@echo "  ml-serve           Run ML service locally (uvicorn)"
	@echo "  frontend-dev       Run Frontend dev server in foreground (Ctrl+C to stop)"
	@echo "  frontend-stop      Stop Frontend dev server if started in background"
	@echo
	@echo "[Data & Pipeline]"
	@echo "  migrate            Run DB migrations"
	@echo "  cricsheet-import   Import Cricsheet JSON into DB"
	@echo "  precompute         Trigger precompute (via API)"
	@echo "  precompute-all        Run unified precompute (as-of/replay + sequential) for FORMAT (default T20)"
	@echo "  precompute-all-all-formats  Run unified precompute for all formats"
	@echo "  precompute-seq     Precompute sequence features: go-app/cmd/precompute-sequence-features (FORMAT?=$(FORMAT))"
	@echo "  export-dataset     Export training datasets (unified)"
	@echo "  export-off         Export without seq columns for FORMAT (default T20)"
	@echo "  export-on          Export with seq columns appended for FORMAT (uses -enable-seq and ENABLE_SEQ_FEATURES=1)"
	@echo "  team-predictor     Generate team prediction (MATCH, BAT, BOWL)"
	@echo
	@echo "[Testing & CI]"
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

# Run Context MCP Server
context-serve:
	cd context-provider && go run main.go