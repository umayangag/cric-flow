# Convenience targets for local dev

.PHONY: dev-up dev-down logs api scraper migrate etl-importer export-dataset test go-test ml-serve

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
