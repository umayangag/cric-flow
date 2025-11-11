# Plan ID 1.5.2 — Extract `weather-worker` into internal with testable seams

Active path: 1 → 4 → 4.x → 1.5 → 1.5.2
Parent: 1.5 (Continue Refactor to ≥80% Coverage)
Branch: feat/weather-worker-phase1

---

## Goal
Make `cmd/weather-worker` a thin delegator (≤ 30 LOC excluding imports/comments) by moving logic into internal packages with small interfaces and high unit-test coverage. Reuse the abstractions introduced in 1.5.1 (`internal/wx.Provider`, `internal/db.WeatherRepo`). Add a small `Jobs` interface for work intake. Keep behavior identical and write table‑driven tests (no `if` in test bodies; per‑test assert helpers).

---

## Scope
- CLI: `internal/cli/weatherworker` with flags: `--iterations` (default 1), `--batch-size` (default 10), `--apply` (dry‑run when false), and optional `--timeout-sec` for graceful exit.
- Runner: `internal/commands/weatherworker.Runner` — validates opts and orchestrates service.
- Service: `internal/services/weatherworker.Service` using `Jobs` + `wx.Provider` + `db.WeatherRepo` to fetch and upsert weather for queued match IDs.
- Adapters: thin `internal/adapters/jobs/dummy` or similar for unit tests only (in‑memory), and a thin DB adapter if needed for `WeatherRepo` (already added in 1.5.1).
- cmd: `cmd/weather-worker/main.go` — parse, wire deps, call `Runner.Run`.

Out of scope:
- Network calls in tests (mock `wx.Provider`).
- Behavior changes.

---

## Interfaces (with mockery tags)
- internal/wx (already exists):
  - `Provider` — fetches weather `Record` by match ID.
- internal/db (already exists):
  - `WeatherRepo` — upserts a weather `Record`.
- New: `internal/jobs` (lightweight seam):
  ```go
  package jobs
  
  //go:generate mockery --name Source --output internal/mocks --case underscore
  type Source interface {
      // Next returns next matchID if available. ok=false ends the loop.
      Next(ctx context.Context, batch int) (ids []int64, ok bool, err error)
  }
  ```

---

## Files to Create/Modify
1) internal/cli/weatherworker/options.go
- `type Options { Iterations int; BatchSize int; Apply bool; TimeoutSec int }`
- Defaults via env: `WEATHER_WORKER_ITERATIONS=1`, `WEATHER_WORKER_BATCH=10`, `WEATHER_WORKER_TIMEOUT=60`.
- `ParseArgs(fs *flag.FlagSet, args []string) (Options, error)` with validation (`Iterations>=1`, `BatchSize>=1`, `TimeoutSec>=1`).
- Tests: `options_test.go` — table‑driven, assert helpers only.

2) internal/services/weatherworker/service.go
- `type Service struct { Jobs jobs.Source; Prov wx.Provider; Repo db.WeatherRepo }`
- `func (s *Service) Run(ctx context.Context, iterations, batch int, apply bool) (processed int, err error)`
  - Loop `iterations` times: `ids, ok, err := Jobs.Next(ctx, batch)`; for each id: `rec, err := Prov.Fetch(ctx, id)`; when `apply`, `Repo.Upsert(ctx, rec)`.
  - Stop on ctx cancel or `ok=false`.
- Tests: `service_test.go` — table‑driven cases: dry‑run, apply happy path, provider error, repo error, jobs error, context cancel mid‑way. Use fakes/mocks; no network.

3) internal/commands/weatherworker/runner.go
- `type Runner struct { Svc *service.Service }`
- `func (r *Runner) Run(ctx context.Context, opts cli.Options) error` — validates opts, builds a timeout context when `TimeoutSec>0`, calls `Svc.Run`.
- Tests: `runner_test.go` — nil svc, bad opts, happy path, timeout path.

4) internal/adapters/jobs/dummy/dummy.go (optional for manual smoke)
- A trivial in‑memory queue that implements `jobs.Source`.

5) cmd/weather-worker/main.go
- Thin wiring: parse CLI → setup logger → connect DB → construct `jobs.Source` (for production this may pull from DB or queue; start with a simple adapter or a placeholder behind env flag) + `wx.Provider` adapter (dummy or std http in future) + `WeatherRepo` adapter (already present) + Service + Runner → `Runner.Run(ctx, opts)`.

---

## Tests
- All tests must be table‑driven, with per‑testcase assert helpers (no `if` in test bodies).
- No network; provider and jobs must be mocked/faked.
- Use deterministic small fixtures inline.

---

## Acceptance Criteria
- `cmd/weather-worker/main.go` ≤ 30 LOC (excluding imports/comments), delegating only.
- Coverage thresholds:
  - `internal/services/weatherworker` ≥ 90%
  - `internal/commands/weatherworker` ≥ 85%
  - `internal/cli/weatherworker` ≥ 90%
- `make mocks`, `make test`, `make coverage`, and `make coverage-func` succeed.
- Behavior preserved: applies only when `--apply` true; stops gracefully when source depleted or timeout reached.

---

## Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused checks
go test -cover ./internal/services/weatherworker -coverprofile=/tmp/ww_svc.out && go tool cover -func=/tmp/ww_svc.out | tail -1
go test -cover ./internal/commands/weatherworker -coverprofile=/tmp/ww_cmd.out && go tool cover -func=/tmp/ww_cmd.out | tail -1
```

---

## Execution Steps (TDD cadence)
1. Scaffold CLI options + tests; implement until green.  
2. Define `jobs.Source` interface with mockery tag; add simple fake in tests.  
3. Implement `Service.Run` with table‑driven tests (dry‑run, apply, error paths, cancel).  
4. Implement Runner + tests.  
5. Thin `cmd/weather-worker/main.go` to wiring only.  
6. Run `make mocks`, `make test`, `make coverage` and ensure thresholds.  
7. Open PR `feat/weather-worker-phase1` with Conventional Commits; reconcile back to Plan ID 1 (update breadcrumbs and statuses).

---

## Progress Markers
- CLI: 
- Service: 
- Runner: 
- cmd wiring: 
- Mocks + coverage + PR:

Legend: * = in progress, ✓ = complete, ! = failed
