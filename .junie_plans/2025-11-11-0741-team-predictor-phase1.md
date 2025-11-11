# Plan ID 1.5.3 — Extract `team-predictor` into internal with testable seams

Active path: 1 → 4 → 4.x → 1.5 → 1.5.3
Parent: 1.5 (Continue Refactor to ≥80% Coverage)
Branch: feat/team-predictor-phase1

---

## Goal
Make `cmd/team-predictor` a thin delegator (≤ 30 LOC excluding imports/comments) by moving logic into testable internal packages with small interfaces. Use table‑driven tests (no `if` in test bodies; per‑case assert helpers). Preserve behavior and flags.

---

## Scope
- CLI: `internal/cli/teampredictor` with flags `--match`, `--format`, `--season`, `--bat`, `--bowl` (+ env/defaults); validate types and required.
- Interfaces with mockery tags:
  - `internal/mlclient.Client` — `PredictTeam(ctx, req) (resp, error)`; `Reload(ctx) error` (optional)
  - `internal/db` repos for inputs (e.g., `PlayerPoolRepo`, `MatchRepo` minimal methods used by service)
  - `internal/logging.Logger` (already present) — optional
- Service: `internal/services/teampredictor` — gather inputs via repos, assemble `PredictRequest`, call mlclient, transform result.
- Runner: `internal/commands/teampredictor` — validate opts, call service, render result (string slice or JSON string; pure logic).
- cmd: `cmd/team-predictor/main.go` — parse, wire deps, `Runner.Run(ctx, opts)`.

Out of scope: behavior changes or ML schema changes.

---

## Files to Create/Modify
1) internal/cli/teampredictor/options.go
- `type Options { MatchID int64; Format string; Season string; Bat int; Bowl int }`
- `ParseArgs(fs, args)` with env defaults; validations: `MatchID>0`, `Format in {TEST,ODI,T20I,T20}`, `Bat>=0`, `Bowl>=0`.
- Tests: `options_test.go` — table‑driven, assert helpers only.

2) internal/mlclient/interfaces.go
- `type PredictRequest struct { MatchID int64; Format string; Season string; Bat int; Bowl int }`
- `type PredictResponse struct { Players []string; Score float64 }`
- `//go:generate mockery --name Client --output internal/mocks --case underscore`
- `type Client interface { PredictTeam(ctx context.Context, in PredictRequest) (PredictResponse, error); Reload(ctx context.Context) error }`

3) internal/db/predictor_repos.go
- Minimal repos used by service (if required by current cmd behavior); add mockery tags.

4) internal/services/teampredictor/service.go
- `type Service struct { ML mlclient.Client; /* repos as needed */ }`
- `func (s *Service) Predict(ctx context.Context, opts cli.Options) (mlclient.PredictResponse, error)` — pure orchestration; validates/normalizes; calls ML.
- Tests: `service_test.go` — table‑driven: happy path, ml error, bad opts normalization, etc.

5) internal/commands/teampredictor/runner.go
- `type Runner struct { Svc *service.Service }`
- `func (r *Runner) Run(ctx context.Context, opts cli.Options) (mlclient.PredictResponse, error)` — validate opts; delegate to service.
- Tests: `runner_test.go` — nil svc, bad opts, happy path.

6) internal/adapters/mlclient/http/client.go (optional now)
- Thin adapter using an `httpx.HTTPClient` for real requests later; unit tests can use mocks; skip heavy networking.

7) cmd/team-predictor/main.go
- Thin wiring: parse via CLI; `logger.SetupFromEnv()`; `db.Connect(ctx)` if needed; build adapters (mlclient; repos if used), service + runner; run and print result. Keep ≤ 30 LOC excluding imports/comments.

---

## Tests
- All new tests are table‑driven with per‑testcase assert helpers and no `if` in test bodies.
- No network; mock `mlclient.Client` with mockery.
- Deterministic small fixtures.

---

## Acceptance Criteria
- `cmd/team-predictor/main.go` ≤ 30 LOC excluding imports/comments.
- Coverage thresholds:
  - `internal/services/teampredictor` ≥ 90%
  - `internal/commands/teampredictor` ≥ 85%
  - `internal/cli/teampredictor` ≥ 90%
- `make mocks`, `make test`, `make coverage`, `make coverage-func` succeed.
- Behavior preserved: same flags and output semantics.

---

## Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused
go test -cover ./internal/services/teampredictor -coverprofile=/tmp/tp_svc.out && go tool cover -func=/tmp/tp_svc.out | tail -1
go test -cover ./internal/commands/teampredictor -coverprofile=/tmp/tp_cmd.out && go tool cover -func=/tmp/tp_cmd.out | tail -1
```

---

## Execution Steps (TDD cadence)
1) Scaffold CLI + tests; implement until green.
2) Add `mlclient.Client` interface + mockery tag; minimal predictor repos if required.
3) Implement service + tests with mocks (happy and error cases); ensure ≥90% coverage.
4) Implement runner + tests; ensure ≥85% coverage.
5) Thin cmd wiring; run tests/coverage; open PR `feat/team-predictor-phase1` and reconcile Plan 1.5.

---

## Progress Markers
- CLI: 
- ML interface + repos: 
- Service: 
- Runner: 
- cmd wiring: 
- Mocks + coverage + PR:

Legend: * = in progress, ✓ = complete, ! = failed
