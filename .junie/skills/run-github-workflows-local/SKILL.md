When the user says "/run-github-workflows-local" or wants to verify CI locally before push

# Run GitHub Workflows Locally

Verify that all runnable GitHub workflows pass locally before pushing, without relying on CI. Map each workflow to equivalent local commands and run them in the correct order.

## Prerequisites

- **Go 1.26+**, **Python 3.12**, **Node 20.x** (or match CI versions)
- **Docker** (for go-app-tests Postgres, ci-backtest E2E)
- **Postgres** reachable on localhost:5432 for go-app tests (or `docker compose up -d postgres`)
- **ml-service venv** initialized (`make -C ml-service init`) — CI uses system Python; locally use venv

## Workflow → Local Command Mapping

| Workflow | Local equivalent | Notes |
|----------|------------------|-------|
| go-app-lint | `make -C go-app init && make -C go-app fmt-check vet lint` | |
| go-app-tests | Postgres + `make migrate-local` + `make -C go-app coverage` + `COV_MIN=60 make -C go-app coverage-check` | Needs Postgres |
| ml-service-lint | `make -C ml-service init` then `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service fmt-check` | Use venv |
| ml-service-tests | `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service coverage` + `COV_MIN=76 make -C ml-service coverage-check` | |
| frontend-lint | `cd frontend && npm ci && npm run typecheck && npm run lint` | |
| frontend-tests | `cd frontend && npm ci && npm run lint format:check typecheck build && npm test` | |
| ci-backtest | `make e2e-backtest-smoke` | Needs Docker; API_KEY=test-api-key is set by Makefile |
| junie | **Skip** — event-triggered (@junie-agent), needs JUNIE_API_KEY | Not runnable locally |

## Efficient One-Shot Commands

From repo root:

### All unit/lint checks (no Docker E2E)

```bash
make check-all
```

This runs `frontend-check`, `go-app-check`, `ml-service-check`. For **go-app-check** to pass, Postgres must be up and migrations applied:

```bash
docker compose up -d postgres
make migrate-local
make check-all
```

### Per-component (faster iteration)

- **Frontend:** `make frontend-check`
- **Go-app:** `make go-app-check` (requires Postgres + migrate-local)
- **ML-service:** `make ml-service-check` (requires `make -C ml-service init` first)

### E2E backtest smoke

```bash
make e2e-backtest-smoke
```

Uses `API_KEY=test-api-key` for docker compose and curl. Requires Docker.

## Order of Execution (full local CI run)

1. **Bootstrap (once):** `make init` (Go + Python tools), `make -C ml-service init`, `docker compose up -d postgres`, `make migrate-local`
2. **Lint + tests:** `make check-all`
3. **E2E smoke (optional):** `make e2e-backtest-smoke`

## Known Differences from CI

- **ml-service:** CI uses `make -C ml-service ci-setup` (system pip); locally prefer venv: `make -C ml-service init` then prepend `ml-service/.venv/bin` to PATH
- **Python:** macOS may have `python3` but not `python`; `ci-setup` expects `python` — use venv to avoid
- **accuracy-trend:** E2E smoke may fail with HTTP 500 on `/api/backtest/accuracy-trend` if ML models are not loaded or DB state differs; select/evaluate steps are the main smoke

## Quick Reference: Full Local CI

```bash
# One-time setup
make init
make -C ml-service init
docker compose up -d postgres
make migrate-local

# Run all checks
make check-all

# Optional: E2E smoke
make e2e-backtest-smoke
```
