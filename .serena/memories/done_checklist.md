### Done checklist before opening a PR / finishing a task

General
- Confirm `src/` remains unmodified (prototype is reference-only)
- Update relevant README(s) when adding/changing entrypoints, flags, or env vars
- Ensure Dockerfiles still build if the component’s dependencies changed

Go (go-app/)
- Format code: `make fmt` and verify with `make fmt-check`
- Lint: `make lint` and `make vet`
- Tests: `make test` (or `go test ./...`) are green
- If DB schema changed: update migrations and run `make migrate` locally
- If API changed: update handlers, contracts, and README docs

Python (ml-service/)
- Activate venv: `source .venv/bin/activate` (after `make init`)
- Format: `make fmt` and `make fmt-check`
- Lint: `make lint-check` (and `make lint` to auto-fix simple issues)
- If training/inference changed: run `make train-all` (as applicable) and verify artifacts load on service start
- Start service locally `make run` and check `/health` (or README-documented health endpoint)

Integration
- With Docker: `make dev-up` then `make logs` to confirm services healthy (postgres, ml-service, go-api)
- Run `make migrate`, `make export-dataset`, and `make precompute` on a sample dataset to verify the happy path
- If adding env vars/configs: document defaults and ensure they are wired in docker-compose and Makefiles

Deliverables
- Provide a brief summary of changes and how to verify them
- Link to relevant commands (from this checklist) used for validation
- No changes inside `src/` except documentation cross-references