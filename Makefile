# Convenience targets for local dev

.PHONY: dev-up dev-down logs api migrate export-dataset precompute go-test ml-serve team-predictor ml-install train-batting train-bowling train-all fmt fmt-check fmt-go fmt-py lint-go lint-py install-hooks init init-go init-py cricsheet-import up-all

# docker-compose stack (Postgres + API + ML service)
dev-up:
	docker-compose up --build -d

dev-down:
	docker-compose down -v

logs:
	docker-compose logs -f --tail=200

# Run Go unit tests
go-test:
	cd go-app && go test ./...

# Run Go integration tests (requires Postgres). Usage: make go-test-int
# Spins are expected to be running via docker-compose or externally.
go-test-int:
	cd go-app && INTEGRATION=1 go test -tags=integration ./...

# Apply DB migrations against local Postgres (env vars can override defaults)
migrate:
	cd go-app && go run ./cmd/tools/migrate -dir=./migrations

# Export datasets similar to src/final_data/queries.py
export-dataset:
	cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset

# Run API locally (assumes Postgres is reachable as configured in env)
api:
	cd go-app && PORT=8080 MIGRATIONS_DIR=./migrations go run ./cmd/api

# Run ML service locally
ml-serve:
	cd ml-service && uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload

# Variables for convenience (override like: make team-predictor MATCH=123 BAT=6 BOWL=5)
SEASON ?= 2019
MATCH ?= 0
BAT ?= 6
BOWL ?= 5

# Run preprocessing computations (happy path)
precompute:
	curl -X POST http://localhost:8080/precompute

# Team predictor (happy path): requires MATCH to be provided
team-predictor:
	@if [ "$(MATCH)" = "0" ]; then echo "Please pass MATCH=<match_id>, e.g., make team-predictor MATCH=123456"; exit 1; fi
	cd ml-service && .venv/bin/python -m ml.export_pool $(MATCH)
	cd go-app && go run ./cmd/team-predictor -match=$(MATCH) -bat=$(BAT) -bowl=$(BOWL)

# Train ML artifacts from exported CSVs
ml-install:
	$(MAKE) -C ml-service install

train-batting: ml-install
	cd ml-service && .venv/bin/python -m ml.train_batting_model

train-bowling: ml-install
	cd ml-service && .venv/bin/python -m ml.train_bowling_model

train-all: train-batting train-bowling

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
	curl -X POST http://localhost:8080/precompute || (echo "Precompute failed" && exit 1)
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
	curl -X POST http://localhost:8080/precompute || (echo "Precompute failed" && exit 1)
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
	docker-compose up --build -d
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
	docker-compose restart ml-service
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
	cd go-app && go vet ./...

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
