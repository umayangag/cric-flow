# Plan ID 1.5.4 — Extract `team-select` into internal with testable seams

Active path: 1 → 4 → 4.x → 1.5 → 1.5.4
Parent: 1.5 (Continue Refactor to ≥80% Coverage)
Branch: feat/team-select-phase1

---

## Goal
Make `cmd/team-select` a thin delegator (≤ 30 LOC excluding imports/comments) by extracting selection logic into pure, heavily tested services (≥95% coverage for core algorithm). All tests table‑driven with per‑testcase assert helpers (no ifs in test bodies). Preserve behavior and flags.

---

## Scope
- CLI: `internal/cli/teamselect` with flags (aligned with Makefile usage):
  - `--match`, `--format`, `--season`, `--size`, `--min-bowlers`, `--require-keeper`, `--from-db` (bool), `--pool` (csv path when not from DB)
- Interfaces with mockery tags:
  - `internal/db` minimal repos for player pool, stats lookup
  - `internal/logging.Logger` (reused) optional
- Services:
  - `internal/services/teamselect/pool` — load pool from DB or CSV (pure parsing for CSV)
  - `internal/services/teamselect/score` — pure scoring functions (no side effects)
  - `internal/services/teamselect/select` — selection orchestrator enforcing constraints (size, min bowlers, keeper)
- Runner: `internal/commands/teamselect` — validate opts, orchestrate pool load → select → render
- cmd: `cmd/team-select/main.go` — parse, wire deps, `Runner.Run(ctx, opts)`

Out of scope: changing scoring model or DB schema.

---

## Files to Create/Modify
1) internal/cli/teamselect/options.go
- `type Options { MatchID int64; Format string; Season string; Size int; MinBowlers int; RequireKeeper bool; FromDB bool; PoolCSV string }`
- `ParseArgs(fs, args)` with env defaults; validations: `MatchID>0`, `Size>=1`, `MinBowlers>=0`, format in set.
- Tests: `options_test.go` — table‑driven, assert helpers only.

2) internal/services/teamselect/pool.go
- `LoadFromCSV(r io.Reader) ([]Player, error)` — pure CSV parse; tests with tiny fixtures.
- `LoadFromDB(ctx, repo, matchID, format, season) ([]Player, error)` — depends on small repo interface (mock in tests).

3) internal/services/teamselect/score.go
- Pure scoring helpers on `Player` struct; table‑driven tests to ≥95% cov.

4) internal/services/teamselect/select.go
- `Select(opts, pool, constraints) ([]Player, error)` — deterministic; enforces `size`, `min bowlers`, `require keeper`; tie‑breaking documented; tests include constraint and edge cases.

5) internal/commands/teamselect/runner.go
- `Runner` wires pool load (from DB or CSV) and calls select service; tests for invalid opts, DB error, CSV parse error, and happy path.

6) cmd/team-select/main.go
- Thin wiring: parse via CLI, `logger.SetupFromEnv()`, `db.Connect(ctx)` when FromDB, build adapters (repos), call runner; print chosen players.

---

## Tests
- All new tests are table‑driven with per‑testcase assert helpers; no `if` in test bodies.
- No network; CSV fixtures as small strings.
- For repo dependencies, use mockery‑generated mocks.

---

## Acceptance Criteria
- `cmd/team-select/main.go` ≤ 30 LOC excluding imports/comments.
- Coverage thresholds:
  - `internal/services/teamselect/*` ≥ 95% for core pure scoring/selection
  - `internal/commands/teamselect` ≥ 85%
  - `internal/cli/teamselect` ≥ 90%
- `make mocks`, `make test`, `make coverage`, `make coverage-func` succeed.

---

## Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused
go test -cover ./internal/services/teamselect -coverprofile=/tmp/ts_svc.out && go tool cover -func=/tmp/ts_svc.out | tail -1
```

---

## Execution Steps (TDD cadence)
1) Scaffold CLI + tests; implement until green.
2) Define minimal repo interfaces with mockery tags for pool load from DB.
3) Implement `pool` CSV loader + tests; add DB path tests with mocks.
4) Implement pure scoring helpers + tests (≥95% coverage).
5) Implement selection orchestrator + tests (constraints, edges).
6) Implement runner + tests; thin cmd wiring; run tests/coverage.
7) Open PR `feat/team-select-phase1` and reconcile Plan 1.5.

---

## Progress Markers
- CLI: ✓
- Pool service: ✓
- Score service: ✓
- Select service: ✓
- Runner: ✓
- cmd wiring: ✓
- Mocks + coverage + PR: ✓

Legend: * = in progress, ✓ = complete, ! = failed

---

## Reconciliation Note
- Subplan 1.5.4 is reconciled with Plan 1.5. See: `.junie_plans/2025-11-11-1532-reconcile-plan-1-5.md`.
- Acceptance for this subplan is verified via targeted package tests; repo-wide gates will be restored under 1.5.2–1.5.5.
