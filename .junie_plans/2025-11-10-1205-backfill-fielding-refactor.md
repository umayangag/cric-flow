# Plan ID 1.4.3 — Extract `backfill-fielding` from cmd into internal with testable seams

Active path: 1 → 4 → 4.3
Parent: 1 → 4  
Branch: feat/backfill-fielding-phase1

---

## 1) Objective
Make `cmd/backfill-fielding` a thin delegator (flags + wiring only) by moving its logic into `internal/cli`, `internal/commands`, and `internal/services`. Introduce minimal interfaces (with mockery tags) for DB and time dependencies, enabling table‑driven unit tests (no `if` in test bodies) and raising coverage. Preserve behavior and outputs.

---

## 2) Scope
- Introduce CLI parser for `backfill-fielding` flags (e.g., `--all`, `--match <id>`, `--apply`, `--concurrency`).
- Define interfaces in `internal/db` (e.g., `FieldingRepo`) used by services to read `fielding_event` and write aggregates into `fielding_data` (or equivalent tables already used by exporters).
- Implement a pure service (`internal/services/fielding/backfill`) that:
  - Computes per‑player fielding aggregates per match from `fielding_event`.
  - Supports `Apply=false` (dry‑run) and bounded concurrency.
  - Emits deterministic results suitable for unit tests with in‑memory fixtures/mocks.
- Add a `Runner` in `internal/commands/backfillfielding` to orchestrate service calls based on CLI options.
- Thin `cmd/backfill-fielding/main.go` to parse → wire → `Runner.Run(ctx, opts)`.
- Tests: Table‑driven with per‑testcase assert helpers; mocks via `mockery` for repos and clock.

Out of scope:
- Schema changes or new features. We mirror existing behavior.

---

## 3) Files to Create/Modify
1. Create: `go-app/internal/cli/backfillfielding/options.go`
   - `type Options struct { All bool; MatchID int64; Apply bool; Concurrency int }`
   - `func ParseArgs(fs *flag.FlagSet, args []string) (Options, error)`
   - Defaults from env when sensible (e.g., `BACKFILL_CONCURRENCY`, default 4).

2. Create: `go-app/internal/cli/backfillfielding/options_test.go`
   - Table‑driven tests for flags precedence and validation (no `if` in test bodies).

3. Create: `go-app/internal/db/fielding_repo.go`
   - Minimal repo used by service. Add mockery tag.
   - Example (adjust to current schema/queries):
   ```go
   //go:generate mockery --name FieldingRepo --output internal/mocks --case underscore
   type FieldingRepo interface {
       // Source events
       ListFieldingEvents(ctx context.Context, matchID *int64) ([]FieldingEvent, error)
       // Destination upsert
       UpsertFieldingAggregates(ctx context.Context, rows []FieldingAggregate) error
   }
   ```
   - Define `FieldingEvent` and `FieldingAggregate` small DTOs in the same package or in `internal/domain` to avoid cycles.

4. Create: `go-app/internal/services/fielding/backfill.go`
   - `type Service struct { Repo db.FieldingRepo; Clock clock.Clock }`
   - Methods:
     - `BackfillAll(ctx context.Context, apply bool, concurrency int) (int, error)`
     - `BackfillMatch(ctx context.Context, matchID int64, apply bool) (int, error)`
   - Pure aggregation logic separate from persistence; write via repo only when `apply`.

5. Create: `go-app/internal/services/fielding/backfill_test.go`
   - Table‑driven: happy paths (per‑match and all), repo error paths, apply=false (no write), and concurrency.
   - Use small fake data fixtures in test file; avoid filesystem/DB.

6. Create: `go-app/internal/commands/backfillfielding/runner.go`
   - `type Runner struct { Svc *fieldingsvc.Service }`
   - `func (r *Runner) Run(ctx context.Context, opts cli.Options) error`
   - Decide between `BackfillAll` vs `BackfillMatch` per options.

7. Create: `go-app/internal/commands/backfillfielding/runner_test.go`
   - Use a mock service or real service with a fake repo; assert orchestration (which method called) and error propagation.

8. Modify: `go-app/cmd/backfill-fielding/main.go`
   - Replace legacy flag parsing with internal CLI.
   - Setup logger and DB connection (and migrations if legacy does).
   - Build concrete repo adapter (thin wrapper around existing queries) and service, then call runner.
   - Keep file ≤ 30 LOC besides imports/comments.

9. Create: `go-app/internal/adapters/clock/system/system.go` (if not yet present)
   - `type SystemClock struct{}` implementing `Clock` interface, with `Now() time.Time`.
   - `//go:generate mockery --name Clock --output internal/mocks --case underscore` placed near interface definition under `internal/adapters/clock`.

10. Ensure/Update: `.mockery.yaml` and `Makefile` target `mocks` (already present) to generate mocks into `internal/mocks`.

11. Optional docs: Update `go-app/README.md` to reference new structure and how to run backfill.

---

## 4) Tests to Add/Update
- `internal/cli/backfillfielding/options_test.go`: flags and validation.
- `internal/services/fielding/backfill_test.go`: 
  - Cases: empty events, single match aggregation, multiple players, writer/repo errors, dry‑run.
  - No `if` in test bodies; assert helpers only.
- `internal/commands/backfillfielding/runner_test.go`: orchestration for `--all` vs `--match`, error propagation.
- Deterministic: use small in‑code fixtures; do not hit DB.

---

## 5) Acceptance Criteria
- `cmd/backfill-fielding/main.go` ≤ 30 LOC besides imports/comments; parses via internal CLI and delegates to `Runner`.
- `internal/services/fielding` coverage ≥ 90%; `internal/commands/backfillfielding` coverage ≥ 85%; `internal/cli/backfillfielding` coverage ≥ 90%.
- All external effects behind interfaces with `//go:generate mockery` tags; `make mocks` succeeds.
- Behavior preserved: same flags, destinations, and side effects. Dry‑run when `--apply` not set.
- `make test`, `make coverage`, and `make coverage-func` succeed.

---

## 6) Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused checks
go test -cover ./internal/services/fielding -coverprofile=/tmp/field_svc.out && go tool cover -func=/tmp/field_svc.out | tail -1
go test -cover ./internal/commands/backfillfielding -coverprofile=/tmp/field_cmd.out && go tool cover -func=/tmp/field_cmd.out | tail -1
```

---

## 7) Execution Steps (TDD cadence)
1. Scaffold CLI options + tests (failing → implement until green).
2. Define `FieldingRepo` interface + DTOs with mockery tags; no implementation yet.  
3. Implement service aggregation logic with tests using a fake repo (in‑memory).
4. Add Runner + tests (use a mock/fake service to assert which method is called).
5. Wire `cmd/backfill-fielding/main.go` (thin) with a minimal repo adapter delegating to existing query helpers; no DB tests.
6. Generate mocks; run all tests + coverage and adjust tests to meet thresholds.
7. Update README (optional) and open a focused PR on `feat/backfill-fielding-phase1`.

---

## 8) Risks & Mitigations
- Risk: Hidden coupling to legacy SQL.  
  Mitigation: Keep a thin adapter; service works against interface + DTOs.
- Risk: Coverage dip.  
  Mitigation: Add negative‑path tests and writer/repo error cases.
- Risk: Behavior drift.  
  Mitigation: Preserve flags and repo upsert method signatures; add smoke run if needed.

---

## 9) Breadcrumbs & Anti‑Drift
Active path: 1 → 4 → 4.3. After completing this sub‑plan, reconcile back to Plan ID 1, mark 4.3 ✓, then proceed to the next command extraction per Plan 1.
