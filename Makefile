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
.PHONY: logs api migrate output-dirs
.PHONY: go-test go-test-int ml-serve ml-install
.PHONY: retrain evaluate reload xi-parity full-pipeline cadence cadence-dry-run
.PHONY: fmt fmt-check fmt-go fmt-py lint lint-go lint-py lint-frontend install-hooks gen-architecture-map gen-architecture-map-check init init-go init-py cricsheet-import
.PHONY: up-all build-apps build-apps-nocache recreate-apps help help-all list
.PHONY: ci ci-go ci-ml seed-fixtures e2e-backtest-smoke migrate-local frontend-stop
.PHONY: check-all frontend-check go-app-check ml-service-check frontend-backend-sync-check e2e-pytest ml-test

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

# Variables for convenience (override like: make e2e FORMAT=ODI SEASON=2019)
SEASON ?= 2019
FORMAT ?= T20
# Recognized formats: TEST, ODI, T20, T20I
# Aliases: MDM -> TEST, ODM -> ODI, IT20 -> T20I
MATCH ?= 0
BAT ?= 6
BOWL ?= 5

# --- The pipeline: import -> retrain -> reload ---
#
# Three steps. It was import, precompute, export and six training commands, in an order
# that could be got wrong; P-6 deleted the producers and folded the rest into one
# retrain. The ops console runs the same three through /ops/pipeline/run/{step}.

GO_APP_URL ?= http://localhost:8080
ML_URL ?= http://localhost:8000
CUTOFF ?=

ml-install:
	$(MAKE) -C ml-service install

# The whole model build: rating pass -> XI win models (with the grid) -> performance
# models -> report -> run manifest, into output/ml-service/runs/<run_id>/. Reads the DB
# (POSTGRES_* from the environment or .env) or, with CRICSHEET_DIR=, the Cricsheet JSON.
# It publishes nothing; `make reload` moves `current`.
retrain:
	set -a; [ -f .env ] && . ./.env; set +a; \
	$(MAKE) -C ml-service retrain CUTOFF="$(CUTOFF)" $(if $(CRICSHEET_DIR),CRICSHEET_DIR="$(abspath $(CRICSHEET_DIR))",) $(if $(XI_OUT),XI_OUT="$(abspath $(XI_OUT))",) $(if $(ACCEPT_DATA_QUALITY),ACCEPT_DATA_QUALITY=1,)

# Point `current` at a run and load it into the running ML service. RUN=<id> names one;
# with none, the newest run on disk -- which is the run a retrain just built.
RUN ?=
reload:
	$(MAKE) -C ml-service reload API_KEY="$(API_KEY)" ML_URL="$(ML_URL)" RUN="$(RUN)"

# L4 evaluation harness (H-19): walk-forward + locked window, one JSON report. Reads the DB
# or, with CRICSHEET_DIR=, the raw Cricsheet JSON directory. Touches no artifact `current`
# points at, which is why it is a step beside the pipeline rather than in it.
evaluate:
	set -a; [ -f .env ] && . ./.env; set +a; \
	$(MAKE) -C ml-service evaluate $(if $(CRICSHEET_DIR),CRICSHEET_DIR="$(abspath $(CRICSHEET_DIR))",) $(if $(XI_OUT),XI_OUT="$(abspath $(XI_OUT))",) $(if $(GENDER_SPLIT_CONTEXT),GENDER_SPLIT_CONTEXT=1,)

# Compare the database against the Cricsheet archive (H-15). See ml-service/Makefile.
XI_PARITY_DIR ?= data/go-app/cricsheet
xi-parity:
	set -a; [ -f .env ] && . ./.env; set +a; \
	$(MAKE) -C ml-service xi-parity CRICSHEET_DIR="$(abspath $(XI_PARITY_DIR))"

# The pipeline against data already imported: build a run and serve it.
full-pipeline: retrain reload

# --- The cadence: the pipeline on a schedule (A-5) ---
#
# One command a scheduler runs unattended: fetch -> extract -> import -> retrain, and
# reload only if every step before it succeeded. It drives the `refresh` run plan
# through the API rather than chaining the targets above, so step ordering, run history,
# progress and Stop are the server's — a cron entry that chained make targets would be a
# second pipeline to keep in step with the first, and it could not be stopped.
#
# Weekly is the default rhythm: Cricsheet republishes its archive daily-ish, and H-11
# refuses predictions against ratings older than 14 days, so a weekly run can be missed
# once and still be inside the limit. See docs/overview.md § Cadence.
#
# The stack must be up (make dev-up) and API_KEY must match what it was started with.
CADENCE_PLAN ?= refresh
cadence:
	API_URL="$(API_URL)" API_KEY="$(API_KEY)" PLAN="$(CADENCE_PLAN)" bash scripts/cadence.sh

# The same command, checking its preconditions and starting nothing.
cadence-dry-run:
	DRY_RUN=1 $(MAKE) cadence --no-print-directory

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
	# The evaluation report (L4). Proxied from ml-service; 503 when the harness has not run,
	# which is a state a fresh box is legitimately in.
	@echo "[SMOKE] Checking backtest/report"; \
	STATUS=$$(curl -sS -o /dev/null -w "%{http_code}" -H "X-API-Key: test-api-key" "http://localhost:8080/api/backtest/report"); \
	if [ "$$STATUS" = "200" ]; then echo "  report OK (200)"; \
	elif [ "$$STATUS" = "503" ]; then echo "  [WARN] no evaluation report yet (run make evaluate)"; \
	else echo "report HTTP $$STATUS"; exit 2; fi
	# Options endpoints
	@echo "[SMOKE] Checking options/formats"; \
	curl -sS -H "X-API-Key: test-api-key" "http://localhost:8080/api/options/formats" | jq -e 'type == "array"' >/dev/null
	# Which run is loaded, and whether its ratings are fresh enough to answer with.
	@echo "[SMOKE] Checking ml/xi-status"; \
	curl -sS -H "X-API-Key: test-api-key" "http://localhost:8080/api/ml/xi-status" | jq -e 'has("loaded") and has("ratings")' >/dev/null
	echo "[SMOKE] OK"

# Run ML-service E2E pytest tests (requires ML service and optionally go-api to be up; set RUN_E2E=1)
e2e-pytest:
	cd ml-service && RUN_E2E=1 ML_SERVICE_URL=$${ML_SERVICE_URL:-http://localhost:8000} $(ML_VENV_BIN)/pytest -q -m e2e -v

# Scoped ML tests for the rating pass and the optimiser (avoid the full FastAPI suite)
ml-test:
	cd ml-service && $(ML_VENV_BIN)/pytest -q tests/test_xi_ratings.py tests/test_xi_optimizer_and_store.py

# --- End-to-end automation ---
# One chain from an empty database to a loaded, manifest-named run:
#   make up-all CUTOFF=2025-09-01

up-all:
	@echo "[1/5] Bringing up Docker stack (Postgres, API, ML)..."
	$(MAKE) dev-up
	@echo "[2/5] Applying DB migrations..."
	$(MAKE) migrate || (echo "Migrations failed" && exit 1)
	@echo "[3/5] Importing Cricsheet JSON (idempotent)..."
	$(MAKE) cricsheet-import || (echo "Cricsheet import failed" && exit 1)
	@echo "[4/5] Retraining (rating pass, models, report, manifest)..."
	$(MAKE) retrain CUTOFF="$${CUTOFF:-$$(date -u +%Y-%m-%d)}" || (echo "Retrain failed" && exit 1)
	@echo "[5/5] Reloading: pointing current at the new run..."
	$(MAKE) reload || (echo "Reload failed" && exit 1)
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
check-all: frontend-check go-app-check ml-service-check frontend-backend-sync-check check-system-map
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
	@echo "[go-app] Enforcing coverage threshold (COV_MIN_GO, default 74)..."
	COV_MIN=$${COV_MIN_GO:-74} $(MAKE) -C go-app coverage-check

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

# Regenerate the derived blocks of ARCHITECTURE_MAP.md from the contracts themselves.
# Several feature lists are built by concatenation, so they have to be imported rather
# than parsed -- hence a real interpreter with the ml-service deps. Locally that is the
# venv; in CI the deps are installed into the system interpreter, so fall back to it.
MAP_PYTHON := $(if $(wildcard $(ML_VENV_BIN)/python),$(ML_VENV_BIN)/python,python3)

gen-architecture-map:
	$(MAP_PYTHON) scripts/gen-architecture-map.py

gen-architecture-map-check:
	$(MAP_PYTHON) scripts/gen-architecture-map.py --check

# The System map tab draws contracts/system-map.json and tells its reader that this is
# what the system does. This is what stops that claim going quietly wrong: every anchor
# the map names must exist, and everything the code can enumerate must be on the map.
check-system-map:
	$(MAP_PYTHON) scripts/check-system-map.py

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
COV_MIN_GO ?= 74
COV_MIN_ML ?= 93

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
	@echo "The pipeline can be driven two ways. The API (POST /ops/pipeline/run/<step>, or the"
	@echo "Ops Status tab) enforces step order, streams progress and can be cancelled. These"
	@echo "targets run the same work directly and do not. Prefer the API for a real run;"
	@echo "targets marked [API] just call it."
	@echo ""
	@echo "[Orchestration]"
	@echo "  up-all             One-shot: docker up → migrate → import → retrain → reload"
	@echo "  full-pipeline      retrain → reload, against data already imported"
	@echo "  cadence            The scheduled refresh: fetch → extract → import → retrain → reload [API]"
	@echo "  cadence-dry-run    Check the cadence's preconditions and start nothing"
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
	@echo "  migrate-local      Apply migrations from the host rather than in-container"
	@echo "  seed-fixtures      Load test fixtures into the compose Postgres"
	@echo
	@echo "[ML — three steps, and the harness beside them. Everything reads the import.]"
	@echo "  retrain            Build one run: rating pass, models, report, manifest (CUTOFF=YYYY-MM-DD)"
	@echo "  reload             Point current at a run and load it (RUN=<id> optional)"
	@echo "  evaluate           L4 harness: walk-forward + locked window, one JSON report"
	@echo "  xi-parity          Check the database against the Cricsheet archive (H-15; XI_PARITY_DIR=)"
	@echo
	@echo "[Testing & CI]"
	@echo "  check-all          Run lint, fmt, typecheck, and tests for all components"
	@echo "  test               Run Go and Python unit tests"
	@echo "  frontend-test      Frontend unit tests"
	@echo "  ml-test            Only the rating-pass and optimiser tests, not the ML suite"
	@echo "  e2e-pytest         ML e2e tests against a running service (RUN_E2E=1)"
	@echo "  go-app-check / ml-service-check / frontend-check  Per-component gate"
	@echo "  frontend-backend-sync-check  Verify canonical formats and model metadata agree"
	@echo "  e2e-backtest-smoke Containerised end-to-end backtest smoke test"
	@echo "  mock               Regenerate mockery mocks"
	@echo "  go-test            Run Go unit tests"
	@echo "  go-test-int        Run Go integration tests (requires DB)"
	@echo "  ci-go              Go CI aggregate (vet, fmt-check, coverage gate)"
	@echo "  ci-ml              ML service CI aggregate"
	@echo "  ci                 Run both CI aggregates"
	@echo
	@echo "[Formatting & Lint]"
	@echo "  fmt / fmt-check    Run formatters across Go and Python (no implicit installs)"
	@echo "  fmt-go / fmt-py    Format only Go / Python"
	@echo "  lint               Lint Go, Python and the frontend"
	@echo "  lint-go / lint-py / lint-frontend  Lint one component"
	@echo
	@echo "[Generated files — do not hand-edit]"
	@echo "  gen-architecture-map        Regenerate the marked blocks of ARCHITECTURE_MAP.md"
	@echo "  gen-architecture-map-check  Fail if those blocks are stale (runs in CI)"
	@echo
	@echo "[Bootstrap]"
	@echo "  init               Initialize both components (tools, venv, hooks)"
	@echo "  init-go / init-py  Initialize only Go / Python"
	@echo "  install-hooks      Install git pre-commit hooks"
	@echo
	@echo "[Docker compose maintenance]"
	@echo "  build-apps         Build app images (build-apps-nocache to skip the cache)"
	@echo "  recreate-apps      Force-recreate app containers without touching deps"
	@echo "  dev-purge          dev-down, then delete output/ (trained models and exports)"
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