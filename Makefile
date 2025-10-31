# Convenience targets for local dev

.PHONY: dev-up dev-down logs api migrate etl-importer export-dataset precompute go-test ml-serve team-predictor train-batting train-bowling train-all fmt fmt-check fmt-go fmt-py lint-go lint-py install-hooks init init-go init-py cricsheet-ingest cricsheet-import up-all

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

# Apply DB migrations against local Postgres (env vars can override defaults)
migrate:
	cd go-app && go run ./cmd/tools/migrate -dir=./migrations

# Import curated CSVs from the Python prototype
etl-importer:
	cd go-app && go run ./cmd/etl-importer -dir=../src/createdb/data

# Export datasets similar to src/final_data/queries.py
export-dataset:
	cd go-app && go run ./cmd/export-dataset -out=../src/final_data/output

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
	cd go-app && go run ./cmd/precompute -season=$(SEASON)

# Team predictor (happy path): requires MATCH to be provided
team-predictor:
	@if [ "$(MATCH)" = "0" ]; then echo "Please pass MATCH=<match_id>, e.g., make team-predictor MATCH=123456"; exit 1; fi
	cd go-app && go run ./cmd/team-predictor -match=$(MATCH) -bat=$(BAT) -bowl=$(BOWL)

# Train ML artifacts from exported CSVs
train-batting:
	cd ml-service && python -m ml.train_batting --csv ../src/final_data/output/batting_encoded.csv --out ./models

train-bowling:
	cd ml-service && python -m ml.train_bowling --csv ../src/final_data/output/bowling_encoded.csv --out ./models

train-all: train-batting train-bowling

# One-shot bootstrap: bring up stack, migrate, import Cricsheet, precompute, export, train, and restart ML service
up-all:
	@echo "[1/7] Bringing up Docker stack (Postgres, API, ML)..."
	docker-compose up --build -d
	@echo "[2/7] Applying DB migrations..."
	$(MAKE) migrate || (echo "Migrations failed" && exit 1)
	@echo "[3/7] Importing Cricsheet JSON (idempotent)..."
	$(MAKE) cricsheet-import || (echo "Cricsheet import failed" && exit 1)
	@echo "[4/7] Precomputing metrics (season=2019)..."
	$(MAKE) precompute SEASON=2019 || (echo "Precompute failed" && exit 1)
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
	cd go-app && gofumpt -w . && golines -w -m 120 .

lint-go:
	cd go-app && go vet ./...

fmt-py:
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	cd ml-service && isort . && black .

lint-py:
	cd ml-service && isort --check-only --diff . && black --check --diff .

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

# Generate curated CSVs from Cricsheet JSON and people.csv
cricsheet-ingest:
	python -m src.cricsheet.ingest --data-dir=data --out-dir=src/createdb/data

# Import Cricsheet JSON into DB using Go importer
cricsheet-import:
	cd go-app && go run ./cmd/cricsheet-importer -dir=../data
