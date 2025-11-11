# Plan ID 1.5 — Continue Refactor to ≥80% Coverage: Remaining cmd/ Extractions + Tooling

Active path: 1 → 4 → 4.x → 1.5
Parent: 1 (Plan ID 1 — Raise Go Coverage > 80%)
Branch (prefix): feat/coverage-phase-next

Goal: Proceed with implementation to thin remaining `cmd/` packages, raise unit test coverage, and finalize tooling/CI. Follow established pattern from prior phases (export-dataset, cricsheet-importer, backfill-fielding, etl-importer). All new tests table‑driven, with per‑testcase assert helpers and no `if` in test bodies. All external effects behind interfaces with mockery tags.

---

## Scope (this phase)
- Extract remaining commands into internal packages with testable seams:
  - 1.5.1 weather-import
  - 1.5.2 weather-worker
  - 1.5.3 team-predictor
  - 1.5.4 team-select
  - 1.5.5 evaluate
  - 1.5.6 tools/migrate
- Harden adapters and configuration where needed (logger, config validation) and add tests.
- Add/ensure CI coverage gate and documentation updates.

Out of scope:
- Feature changes or schema changes; maintain behavior.
- Network access in unit tests.

---

## Common Extraction Template (applies to each command)
For command X:
- Create `internal/cli/X` with `Options` + `ParseArgs` (env defaults via `config`, validations). Tests: table‑driven.
- Create `internal/commands/X` with orchestration `Runner` (small, pure logic). Tests with mocks.
- Create `internal/services/X` encapsulating business logic; depend on minimal interfaces (`db`, `httpx`, `fsx`, `clock`, `random`, `logging`). Tests (≥90% for pure services).
- Add adapter(s) under `internal/adapters/...` implementing interfaces as needed.
- Add `//go:generate mockery` tags for all new interfaces.
- Thin `cmd/X/main.go` to ≤ 30 LOC (excluding imports/comments) — parse, wire, `runner.Run(ctx, opts)`.
- Documentation updates if flags/usage clarified (no behavior change).

Verification (per command):
- `cd go-app && make mocks && make test && make coverage && make coverage-func`
- Focused package coverage checks (`go test -cover ./internal/{cli,commands,services}/X ...`).

---

## 1.5.1 weather-import (Parent: 1 → 4)
- Interfaces:
  - `internal/weather`: `Provider interface { Fetch(ctx, matchID) (WeatherRecord, error) }`.
  - `internal/db`: `WeatherRepo interface { Upsert(ctx, WeatherRecord) error }`.
  - `internal/adapters/httpx`: `HTTPClient` (already envisaged) to back provider.
- CLI: `--match <id> --provider <name> --apply` (+ env defaults).
- Service: orchestrate provider fetch → transform → repo upsert (dry‑run when `--apply` is false).
- Runner: validate opts, call service.
- Adapter(s): provider implementations; thin DB repo.
- Acceptance: `cmd/weather-import/main.go` ≤ 30 LOC; services ≥90% cov; commands ≥85%.

## 1.5.2 weather-worker (Parent: 1 → 4)
- Similar to weather-import but consumes a queue/source; for unit tests, introduce an interface `Jobs interface { Next(ctx) (matchID, bool, error) }`.
- Service: poll Next, fetch weather via Provider, upsert; stop on ctx cancel.
- Runner: wire and run once (or N iterations) for testability.
- Acceptance: thin cmd; services ≥90%.

## 1.5.3 team-predictor (Parent: 1 → 4)
- Interfaces:
  - `mlclient.Client` (already planned): `PredictTeam(ctx, PredictRequest) (PredictResponse, error)`.
  - `db` repos as needed for inputs.
- CLI: `--match --format --season --bat --bowl` (+ env defaults).
- Service: gather inputs (via repos), build request, call `mlclient.Client`, present result.
- Runner: validation + service call; result rendering.
- Acceptance: thin cmd; services/commands coverage thresholds.

## 1.5.4 team-select (Parent: 1 → 4)
- Similar to team-predictor but optimizing selection; introduce interfaces for constraints/heuristics; keep pure components heavily tested.
- Acceptance: thin cmd; pure selection logic ≥95%.

## 1.5.5 evaluate (Parent: 1 → 4)
- CLI: `--season --format`.
- Service: compute metrics from repos; keep pure aggregation functions; table-driven tests.
- Acceptance: thin cmd; services ≥90%.

## 1.5.6 tools/migrate (Parent: 1 → 4)
- CLI: `--dir` (defaults from env); runner delegates to `internal/db.RunMigrations` (already exists). Add logging interface if needed.
- Acceptance: `cmd/tools/migrate/main.go` ≤ 30 LOC and covered indirectly by unit tests of `db.RunMigrationsFS` and runner small tests.

---

## 1.5.7 Adapters Hardening and Config (Parent: 1)
- `internal/logging`: finalize `Logger` facade + tests for level parsing.
- `internal/config`: add table-driven tests for `Default*Dir()` and `ValidateTeamSettings` (already present) edge cases.
- Ensure `internal/adapters/httpx/std` and `internal/adapters/mlclient/http` exist and have small focused tests (request building; use `httptest`).

---

## 1.5.8 Testing, Coverage Gate, and CI (Parent: 1)
- Ensure `Makefile` targets present: `mocks`, `test`, `coverage`, `coverage-func`, `coverage-check` (COV_MIN=80 default).
- Add/update GitHub Actions workflow to run format/lint/test/coverage and enforce coverage gate (document if CI exists elsewhere).
- Document how to install `mockery` and run tests.

---

## Acceptance Criteria (for this phase)
- All remaining `cmd/*/main.go` files are thin delegators (≤ 30 LOC excluding imports/comments).
- Per-package coverage thresholds met:
  - `internal/services/*` new packages ≥ 90%.
  - `internal/commands/*` new packages ≥ 85%.
  - `internal/cli/*` new packages ≥ 90%.
- Global `go-app` coverage increases steadily and reaches ≥ 80% when the listed commands are extracted.
- `make mocks`, `make test`, `make coverage`, and `make coverage-func` succeed locally.
- CI (if configured) enforces coverage and runs on PRs.

Verification commands:
- `cd go-app && make mocks && make test && make coverage && make coverage-func`
- `go tool cover -func=coverage.out | tail -1` shows total ≥ 80% at end of plan.

---

## Execution Steps (TDD cadence)
1) 1.5.1 weather-import: CLI + Runner + Service + Adapters + Tests → PR `feat/weather-import-phase1`.
2) 1.5.2 weather-worker: same pattern → PR `feat/weather-worker-phase1`.
3) 1.5.3 team-predictor: extract and test pure logic, mock mlclient → PR.
4) 1.5.4 team-select: extract algorithms to pure services; heavy unit tests → PR.
5) 1.5.5 evaluate: extract aggregation into services; tests → PR.
6) 1.5.6 tools/migrate: thin to runner; tests cover runner + db migration helper → PR.
7) 1.5.7 adapters/config/logging tests and small fixes → PR.
8) 1.5.8 CI coverage gate (if not present) and docs updates → PR.

---

## Progress Markers
- 1.5.1 weather-import: 
- 1.5.2 weather-worker: 
- 1.5.3 team-predictor: 
- 1.5.4 team-select: 
- 1.5.5 evaluate: 
- 1.5.6 tools/migrate: 
- 1.5.7 adapters/config/logging: 
- 1.5.8 CI + docs:

Legend: * = in progress, ✓ = complete, ! = failed
