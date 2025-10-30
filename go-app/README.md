# Go Application (Scraper/ETL/API)

This is the initial scaffold for the Go port of the prototype. It introduces service boundaries and basic commands without changing the original Python code under `src/`.

Components (scaffolded):
- cmd/scraper: CLI/service to scrape match lists and matches and emit structured JSON or write to DB.
- cmd/api: Minimal HTTP API server (health endpoint) and future orchestration endpoints.
- internal/cricinfo: Parsers for list and match pages (stubs for now).
- internal/contracts: Strongly-typed data contracts for matches, players, weather, features.
- internal/db: DB connection and repositories (stubbed).
- internal/features: Placeholder for preprocessing logic (form/venue/opposition/consistency).
- internal/mlclient: HTTP client to the Python ML service (stubbed).

Quick start:
- Requires Go 1.22+
- Set environment variables for DB if you plan to connect: `DB_HOST`, `DB_USER`, `DB_PASS`, `DB_NAME`.

Makefile targets:
- `make build` — build binaries for `scraper` and `api`.
- `make run-scraper` — run the scraper CLI with example flags.
- `make run-api` — run the HTTP API on :8080.
- `make docker-build` — build docker images.
- `make docker-run` — run dockerized API.

This is just a starting skeleton; implementations will be filled incrementally while keeping source parity with the Python prototype.
