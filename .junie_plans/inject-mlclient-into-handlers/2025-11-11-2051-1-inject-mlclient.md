# Plan: Inject a single mlclient into API handlers

Parent: N/A (Top-level plan 1)

## Context
Currently, `predictBattingHandler` and `predictBowlingHandler` create a new `mlclient.Client` per request via `mlclient.New()`. We want to instantiate the ML client once at startup and inject it into handlers to improve performance and resource management.

## Scope
- Introduce an application struct to hold shared dependencies (initially ML client; extensible for future deps).
- Instantiate ML client once during startup in `main()`.
- Wire ML client into the HTTP layer; convert prediction handlers to methods on the application struct.
- Add unit tests that verify handler behavior uses injected client and avoids per-request instantiation.
- Preserve API surface and behavior (paths, payloads, responses, status codes).

Out of scope:
- Changing request/response schemas.
- Modifying other services or the `src/` prototype directory.

## Proposed Changes (Files)
- go-app/cmd/api/app.go (new): defines `type app struct { ml mlclient.Service }` and constructor if needed.
- go-app/cmd/api/main.go: create the ML client once; initialize `*app`; pass to router.
- go-app/cmd/api/server.go: update `newRouter(app *app) *mux.Router` (function signature change) and route registrations to use method handlers.
- go-app/cmd/api/handlers.go: convert `predictBattingHandler` and `predictBowlingHandler` to methods `func (a *app) predictBattingHandler(...)` and `func (a *app) predictBowlingHandler(...)` that use `a.ml`.
- Optionally adjust other handlers to methods later for consistency (not required for this task, unless needed to keep code idioms consistent).
- go-app/internal/mlclient/...: no changes expected.
- Tests (new):
  - go-app/cmd/api/handlers_test.go: unit tests using a mock for `mlclient.Service` to verify usage and responses.

## Design Notes
- Depend on the `mlclient.Service` interface from `internal/mlclient/interfaces.go` to allow mocking.
- The ML client concrete constructor can be `mlclient.New()` as before, but called once in `main()`.
- Handlers stay functionally identical; only dependency acquisition changes.
- Keep logging; add logs for client creation success/failure.

## Phases

1. Introduce application struct and dependency wiring
   - Add `app` struct with `ml mlclient.Service`.
   - In `main.go`, instantiate the ML client once and construct `*app`.
   - Change `newRouter()` signature to accept `*app`.

2. Convert handlers to methods on `*app`
   - Update prediction handlers to methods using `a.ml`.
   - Update route bindings to method receivers.

3. Tests and verification
   - Add tests using a mock `mlclient.Service` to assert:
     - Correct request decoding and passing to `PredictBatting`/`PredictBowling`.
     - Returned predictions are written with `200 OK`.
     - Errors from service propagate as error responses.
   - Ensure there is no per-request client instantiation (verified implicitly by using the injected mock and not calling constructors inside handlers).

4. Docs and cleanup
   - If any env vars/config for ML client are introduced, update README.
   - Ensure code comments explain the dependency injection pattern.

## Acceptance Criteria
- A single ML client is created at application startup and reused by prediction handlers.
- `POST /predict/batting` and `POST /predict/bowling` responses and status codes remain unchanged compared to current behavior.
- No `mlclient.New()` calls exist inside `predictBattingHandler` or `predictBowlingHandler`.
- Unit tests cover success and error paths for both prediction endpoints using a mock service.
- `go build ./...` under `go-app` passes without errors.
- Smoke test via `go run ./cmd/api` and simple curl requests succeed.

## Risks & Rollback
- Risk: Incorrect wiring may break route registration. Mitigation: compile early; run smoke tests.
- Rollback: Revert to previous handler functions and per-request client instantiation.

## Exact Commands

Branching (never commit to main):
- git checkout -b feat/inject-mlclient-into-handlers

Phase 1 — Wiring and struct:
- Implement code changes.
- go fmt ./...
- go build ./...

Phase 2 — Handlers to methods:
- Implement code changes.
- go fmt ./...
- go build ./...

Phase 3 — Tests:
- cd go-app
- go test ./...

Manual run & smoke checks:
- cd go-app
- go run ./cmd/api
- curl -s -X POST localhost:8080/predict/batting -d '[]' -H 'Content-Type: application/json'
- curl -s -X POST localhost:8080/predict/bowling -d '[]' -H 'Content-Type: application/json'

Lint/format (if configured):
- make fmt
- make lint

## Active Path
- 1 → 2 → 3 → 4

## PR Strategy
- Small, focused PRs per phase with Conventional Commits:
  - feat(api): introduce app struct and wire mlclient
  - refactor(api): convert prediction handlers to methods
  - test(api): add handler tests using mock mlclient

