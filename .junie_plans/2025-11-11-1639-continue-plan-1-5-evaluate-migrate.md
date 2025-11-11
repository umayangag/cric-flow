# Plan ID 1.5.cont — Continue Plan 1.5 to Complete Remaining 4.x Commands and Align With Guidelines

Active path: 1 → 4 → 4.x → 1.5 → 1.5.cont
Parent: 1.5 (Continue Refactor to ≥80% Coverage)
Branch (prefix): feat/plan-continue-1-5

---

## Goal
Complete the remaining `cmd/` extractions under 4.x (specifically `evaluate` and `tools/migrate`), keep `cmd/*` thin, raise/maintain coverage, and reconcile with the optimized directives in `.junie/guidelines.md`. No behavior changes.

This continuation strictly follows the SOP and engineering rules from `.junie/guidelines.md` (KISS, DRY, SOLID; small focused PRs; table-driven tests; mocks via mockery; no network; deterministic fixtures; feature branches only; Conventional Commits; acceptance criteria with exact verification commands).

---

## Context & Baseline
- Completed (per master and subplans): 4.1 export-dataset, 4.2 cricsheet-importer, 4.3 backfill-fielding.
- Completed in 4.x: `etl-importer`, `weather-import`, `weather-worker`, `team-predictor`, `team-select` (see `2025-11-11-0741-team-select-phase1.md` and `2025-11-11-1532-reconcile-plan-1-5.md`).
- Pending in 4.x: `evaluate`, `tools/migrate`.
- Sections 5–9 (logging/config, adapters, testing tooling, CI, docs) remain planned and will be executed after 4.x is closed.

---

## Scope (this continuation)
1) Extract and cover `evaluate` (make `cmd/evaluate` a thin delegator).
2) Thin and cover `tools/migrate` with a small runner orchestrating migrations.
3) Keep alignment with mockery, Makefile targets, and testing conventions; do not reintroduce repo-wide gates until stabilization steps are done (tracked in 1.5 reconciliation plan).

Out of scope:
- Feature or schema changes; behavior must remain stable.
- Network or real DB access in tests.

---

## Common Extraction Template (applies to both commands)
- CLI: `internal/cli/<tool>` with `Options` and `ParseArgs(fs, args)`; env defaults via config; validations. Tests: table‑driven; per‑testcase assert helpers; no ifs in test bodies.
- Commands: `internal/commands/<tool>` with `Runner` that orchestrates services/adapters. Tests with mocks.
- Services: `internal/services/<tool>` with pure business logic where possible. Coverage ≥90% for pure services.
- Interfaces + mockery tags added near seams (`internal/db`, `internal/logging`, `internal/fsx`, etc.).
- Thin `cmd/<tool>/main.go` ≤ 30 LOC (excluding imports/comments).

Verification (per tool):
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
```

---

## 1.5.cont.1 — Extract `evaluate`
Parent: 1.5.cont
Branch: feat/evaluate-extract

### Design
- CLI: `internal/cli/evaluate/options.go`
  - Flags: `--season <string>`, `--format <string>`; validations: non-empty; `format` in allowed set.
- Services: `internal/services/evaluate`
  - Pure aggregation functions over input DTOs (no I/O). Table‑driven tests to ≥90% coverage.
- Commands: `internal/commands/evaluate/runner.go`
  - Wire repos to load necessary stats; call services; render summary.
- Interfaces:
  - `internal/db` minimal read-only repos (e.g., `type EvaluationRepo interface { LoadSeason(ctx context.Context, season, format string) ([]MatchStat, error) }`) with `//go:generate mockery`.
  - `internal/logging.Logger` reused; optional.

### Acceptance
- `cmd/evaluate/main.go` ≤ 30 LOC (excl. imports/comments).
- Coverage: `internal/services/evaluate` ≥ 90%, `internal/commands/evaluate` ≥ 85%, `internal/cli/evaluate` ≥ 90%.
- All tests table‑driven; no ifs in test bodies; mocks generated via `make mocks`.

### Verification
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused
go test -cover ./internal/cli/evaluate
go test -cover ./internal/services/evaluate
go test -cover ./internal/commands/evaluate
```

---

## 1.5.cont.2 — Thin `tools/migrate`
Parent: 1.5.cont
Branch: feat/migrate-runner

### Design
- Commands: `internal/commands/migrate/runner.go`
  - `Runner.Run(ctx, opts)` calls a tested helper (e.g., `internal/db.RunMigrationsFS(ctx, fs, dir)`), handling logging and error propagation.
- CLI: `internal/cli/migrate/options.go`
  - Flags: `--dir` (defaults via env/config), optional DB DSN if not already centralized.
- Interfaces:
  - `internal/db.Connector` or `Migrator` interface.
  - `internal/fsx.FS` and `internal/logging.Logger`.

### Acceptance
- `cmd/tools/migrate/main.go` ≤ 30 LOC (excl. imports/comments).
- `internal/commands/migrate` ≥ 85% coverage; CLI ≥ 90%.
- Tests cover success and error paths; mocks via `mockery`.

### Verification
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused
go test -cover ./internal/cli/migrate
go test -cover ./internal/commands/migrate
```

---

## Alignment with `.junie/guidelines.md`
- Branching: never to main; use `feat/evaluate-extract` and `feat/migrate-runner`. Conventional Commits in PRs.
- TDD cadence: write failing tests first; then minimal code to green; refactor; keep functions small and pure when possible.
- Testing: table‑driven, deterministic, no network; mocks via `mockery`; fixtures small and local.
- Makefile: prefer `make mocks`, `make test`, `make coverage`, `make coverage-func`. Ensure `coverage-check` gate is only re‑enabled after stabilization (tracked elsewhere).
- Code quality: descriptive names, clear flow, robust error handling; singleton logger; comments explain why (not what).
- Project‑specific: do not modify `src/`.

---

## Deliverables
- New internal packages for `evaluate` and `migrate` (CLI, commands, services where applicable) with tests and mocks.
- Thinned `cmd/evaluate` and `cmd/tools/migrate`.
- Updated Makefile targets only if necessary to run focused tests (no breaking changes to global gates in this phase).

---

## Acceptance Criteria (this continuation)
- `evaluate` and `tools/migrate` follow the common extraction template; `cmd` files ≤ 30 LOC.
- Coverage thresholds met per package (services ≥ 90%, commands ≥ 85%, cli ≥ 90%).
- `make mocks`, `make test`, `make coverage`, `make coverage-func` succeed locally.

---

## Verification Commands (end of continuation)
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Optional check total
go tool cover -func=coverage.out | tail -1
```

---

## Execution Steps
1) Scaffold `evaluate` CLI + tests (failing), services + tests, and runner + tests; thin `cmd/evaluate`; iterate until green; open PR `feat/evaluate-extract`.
2) Scaffold `migrate` CLI + tests, runner + tests; thin `cmd/tools/migrate`; iterate until green; open PR `feat/migrate-runner`.
3) Reconcile statuses under Plan 1.5 after both are merged; then proceed to sections 5–9 per master plan (separate PRs), keeping alignment with `.junie/guidelines.md`.

---

## Progress Markers
- 1.5.cont.1 evaluate: 
- 1.5.cont.2 migrate: 

Legend: * = in progress, ✓ = complete, ! = failed
