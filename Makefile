# Convenience targets for local dev

.PHONY: dev-up dev-down logs api scraper migrate etl-importer export-dataset precompute test go-test ml-serve team-predictor

# Docker Compose stack (Postgres + API + ML service)
dev-up:
	docker compose up --build -d

dev-down:
	docker compose down -v

logs:
	docker compose logs -f --tail=200

# Run Go unit tests
go-test:
	cd go-app && go test ./...

# Apply DB migrations against local Postgres (env vars can override defaults)
migrate:
	cd go-app && go run ./cmd/tools/migrate -dir=./migrations

# Run the scraper locally (defaults to Sri Lanka ODIs and narrow window)
scraper:
	cd go-app && go run ./cmd/scraper -team="Sri Lanka" -from=2010-01-01 -to=2010-02-01

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

# Convenience targets for local dev

.PHONY: dev-up dev-down logs api scraper migrate etl-importer export-dataset precompute test go-test ml-serve team-predictor train-batting train-bowling train-all

# Docker Compose stack (Postgres + API + ML service)
dev-up:
	docker compose up --build -d

dev-down:
	docker compose down -v

logs:
	docker compose logs -f --tail=200

# Run Go unit tests
go-test:
	cd go-app && go test ./...

# Apply DB migrations against local Postgres (env vars can override defaults)
migrate:
	cd go-app && go run ./cmd/tools/migrate -dir=./migrations

# Run the scraper locally (defaults to Sri Lanka ODIs and narrow window)
scraper:
	cd go-app && go run ./cmd/scraper -team="Sri Lanka" -from=2010-01-01 -to=2010-02-01

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

# One-shot bootstrap: spin up stack, migrate, scrape, precompute, export, train, and restart ML service
up-all:
	@echo "[1/7] Bringing up Docker stack (Postgres, API, ML)..."
	docker compose up --build -d
	@echo "[2/7] Applying DB migrations..."
	$(MAKE) migrate || (echo "Migrations failed" && exit 1)
	@echo "[3/7] Scraping a tiny window (idempotent)..."
	$(MAKE) scraper || (echo "Scraper failed" && exit 1)
	@echo "[4/7] Precomputing metrics (season=2019)..."
	$(MAKE) precompute SEASON=2019 || (echo "Precompute failed" && exit 1)
	@echo "[5/7] Exporting datasets..."
	$(MAKE) export-dataset || (echo "Export failed" && exit 1)
	@echo "[6/7] Training ML artifacts..."
	$(MAKE) train-all || (echo "Training failed" && exit 1)
	@echo "[7/7] Restarting ML service to load artifacts..."
	docker compose restart ml-service
	@echo "Done. API at http://localhost:8080 (health/readiness), ML at http://localhost:8000 (health)."