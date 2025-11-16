# Convenience targets for local dev
VENV:=.venv
PY:=$(VENV)/bin/python3
PIP:=$(VENV)/bin/pip

# Common variables
DC:=docker-compose
APP_SERVICES:=go-api ml-service

.PHONY: dev-up dev-down dev-rebuild dev-rebuild-nocache logs api migrate export-dataset export-off export-on precompute precompute-seq go-test go-test-int ml-serve team-predictor ml-install train-batting train-bowling train-all fmt fmt-check fmt-go fmt-py lint-go lint-py install-hooks init init-go init-py cricsheet-import up-all build-apps build-apps-nocache recreate-apps e2e e2e-multi help help-all list ci ci-go ci-ml

# docker-compose stack (Postgres + API + ML service)
dev-up:
	$(DC) up --build -d

dev-down:
	$(DC) down -v

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

# Import Cricsheet JSON into the DB (idempotent). Honors env flags to include optional placeholders.
# Usage examples:
#   make cricsheet-import
#   GO_APP_INPUT_DIR=../data/go-app/cricsheet make cricsheet-import
#   PLACEHOLDERS_WEATHER=1 PLACEHOLDERS_FIELDING=1 WEATHER_ENQUEUE=1 make cricsheet-import
cricsheet-import:
	cd go-app; \
	INDIR=$${GO_APP_INPUT_DIR:-../data/go-app/cricsheet}; \
	WFLAG=""; FFLAG=""; EFLAG=""; \
	if [ "$$PLACEHOLDERS_WEATHER" = "1" ]; then WFLAG="--placeholders-weather"; fi; \
	if [ "$$PLACEHOLDERS_FIELDING" = "1" ]; then FFLAG="--placeholders-fielding"; fi; \
	if [ "$$WEATHER_ENQUEUE" = "1" ]; then EFLAG="--weather-enqueue"; fi; \
	GO_APP_INPUT_DIR=$$INDIR go run ./cmd/cricsheet-importer -dir=$$INDIR $$WFLAG $$FFLAG $$EFLAG

# Export datasets (unified exports only)
export-dataset:
	# Unified, cross-format CSVs with as-of per-format features
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -unified=1

# Convenience targets for sequence feature workflows (FORMAT defaults to T20)
precompute-seq:
	cd go-app && go run ./cmd/precompute-sequence-features -format=$(FORMAT) -targets=all

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

# Team predictor (happy path): requires MATCH to be provided
team-predictor:
	@if [ "$(MATCH)" = "0" ]; then echo "Please pass MATCH=<match_id>, e.g., make team-predictor MATCH=123456"; exit 1; fi
	cd ml-service && .venv/bin/python -m ml.export_pool $(MATCH)
	cd go-app && go run ./cmd/team-predictor -match=$(MATCH) -bat=$(BAT) -bowl=$(BOWL)

# Train ML artifacts from exported CSVs
ml-install:
	$(MAKE) -C ml-service install

train-batting:
	cd ml-service && $(PY) ml/train_batting_model.py

train-bowling:
	cd ml-service && $(PY) ml/train_bowling_model.py

train-all: train-batting train-bowling

# Scoped ML tests for new readers/baselines (avoid full FastAPI test suite)
ml-test:
	cd ml-service && pytest -q tests/test_seq_reader.py tests/test_baselines.py

# Tiny T20 baselines using new readers on small fixtures (structure only)
train-batting-baseline:
	cd ml-service && $(PY) -c "from pathlib import Path; from ml_service.baselines import train_batting_from_csv; root=Path(__file__).resolve().parents[1]; csv=root/'tests/fixtures/exporter/t20/batting_on.csv'; res=train_batting_from_csv(str(csv)); print('batting baseline trained:', res.n_rows, 'rows', res.n_features, 'features')"

train-bowling-baseline:
	cd ml-service && $(PY) -c "from pathlib import Path; from ml_service.baselines import train_bowling_from_csv; root=Path(__file__).resolve().parents[1]; csv=root/'tests/fixtures/exporter/t20/bowling_on.csv'; res=train_bowling_from_csv(str(csv)); print('bowling baseline trained:', res.n_rows, 'rows', res.n_features, 'features')"

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
	@for f in $(subst ,,$(FORMATS)); do \
		echo "  Training for format $$f..."; \
		cd ml-service && .venv/bin/python -m ml.train_batting_model --format $$f && .venv/bin/python -m ml.train_bowling_model --format $$f; \
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
	@echo "Done. API at http://localhost:8080 (health/readiness), ML at http://localhost:8000 (health)."

# --- Formatting & hooks ---

# Aggregate formatters for both components
fmt: fmt-go fmt-py

fmt-check:
	$(MAKE) -C go-app fmt-check
	$(MAKE) -C ml-service fmt-check

fmt-go:
	@command -v gofumpt >/dev/null 2>&1 || (echo "Install gofumpt: go install mvdan.cc/gofumpt@latest" && exit 1)
	@command -v golines >/dev/null 2>&1 || (echo "Install golines: go install github.com/segmentio/golines@latest" && exit 1)
	cd go-app && make fmt-check

lint-go:
	cd go-app && go vet ./... && make lint

fmt-py:
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	cd ml-service && make fmt-check

lint-py:
	cd ml-service && make lint-check

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
	@command -v mockery >/dev/null 2>&1 || (echo "mockery not found. Install pinned version:\n  go install github.com/vektra/mockery/v3@v3.6.0" && exit 1)
	@ver=$$(mockery --version 2>/dev/null | awk '{print $$3}'); \
	if [ "$$ver" != "v3@v3.6.0" ]; then \
		echo "mockery version $$ver detected. Please install v3.6.0 for deterministic generation:"; \
		echo "  go install github.com/vektra/mockery/v3@v3.6.0"; \
		exit 2; \
	fi
	mockery --config go-app/.mockery.yml

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
	$(MAKE) -C ml-service ci COV_MIN=$(COV_MIN_ML)

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
	@echo "  dev-down           Stop and remove stack (volumes)"
	@echo "  logs               Tail docker-compose logs"
	@echo "  api                Run Go API locally (outside Docker)"
	@echo "  ml-serve           Run ML service locally (uvicorn)"
	@echo
	@echo "[Data & Pipeline]"
	@echo "  migrate            Run DB migrations"
	@echo "  cricsheet-import   Import Cricsheet JSON into DB"
	@echo "  precompute         Trigger precompute (via API)"
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
	@echo "  fmt / fmt-check    Run formatters across Go and Python"
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
	@echo "  - Run 'make list' to see all phony targets"

help-all:
	@echo "\n[Root]" && $(MAKE) help --no-print-directory || true
	@echo "\n[Go App]" && $(MAKE) -C go-app help --no-print-directory || true
	@echo "\n[ML Service]" && $(MAKE) -C ml-service help --no-print-directory || true

list:
	@awk '/^\.PHONY:/{for(i=2;i<=NF;i++)print $$i}' $(MAKEFILE_LIST) | sort -u
