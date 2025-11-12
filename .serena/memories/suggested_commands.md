### Common workflows and commands

Prerequisites:
- Docker + Docker Compose
- Go 1.25+
- Python 3.10+ (only if running ML locally without Docker)

Bootstrap tooling:
- One-time init for all:
  - `make init`
- Component init:
  - `make -C go-app init`
  - `make -C ml-service init`
- Activate Python venv (after ML init):
  - `cd ml-service && source .venv/bin/activate`

Orchestrate with Docker Compose:
- Bring up stack (Postgres + API + ML): `make dev-up`
- Tear down and remove volumes: `make dev-down`
- Tail logs: `make logs`
- End-to-end happy path (bootstrap all, including training): `make up-all`

Database migrations:
- Apply migrations locally (env can override defaults): `make migrate`
- Go-only migrations runner inside the repo: `cd go-app && go run ./cmd/migrate -dir=./migrations`

Data import/export and preprocessing:
- Export ML-ready datasets (to `output/go-app/`): `make export-dataset`
- Trigger preprocessing via API (assumes API running): `make precompute`

Run locally (without Docker):
- Start Go API (port 8080): `make api`
- Start ML service (port 8000): `make ml-serve`

Go app (inside go-app/):
- Run API: `make run-api` (or `go run ./cmd/api`)
- Test: `make test` or `go test ./...`
- Format: `make fmt` (gofumpt + golines)
- Format check: `make fmt-check`
- Lint: `make lint` (golangci-lint)
- Vet: `make vet`
- Docker build: `make docker-build`
- Docker run (uses env for DB): `make docker-run`

ML service (inside ml-service/):
- Create venv and install deps/tools: `make init`
- Run FastAPI with reload: `make run`
- Train models: `make train-all`
- Validate dataset exports: `make validate-exports`
- Format: `make fmt` (isort + black + flake8)
- Format check: `make fmt-check`
- Lint auto-fix (ruff F401): `make lint`
- Lint check (CI-safe): `make lint-check`
- Docker build: `make docker-build`
- Docker run: `make docker-run`

Team selection (happy path):
- Requires a match id. Example: `make team-predictor MATCH=123456` (see root Makefile target)

Environment defaults (override via env):
- `POSTGRES_HOST=localhost`, `POSTGRES_PORT=5432`, `POSTGRES_DB=cricket_data`
- `POSTGRES_USER=postgres`, `POSTGRES_PASSWORD=postgres`, `POSTGRES_SSLMODE=disable`
- `ML_SERVICE_URL=http://localhost:8000` (for the Go API)

Darwin notes:
- Ensure `$(go env GOPATH)/bin` is in your `PATH` so `gofumpt`, `golines`, and `golangci-lint` are available.
- Use `source ml-service/.venv/bin/activate` to activate the Python venv when working on ML locally.