# Plan ID 1.4.3.1 — Backfill-fielding Phase 1 (continue): Runner tests, thin cmd, mocks, coverage, PR

Active path: 1 → 4 → 4.3 → 4.3.1
Parent: 1 → 4 → 4.3
Branch: feat/backfill-fielding-phase1

---

## Objective
Finish extracting `backfill-fielding` by completing runner tests, thinning `cmd/backfill-fielding` to pure wiring, ensuring mocks/coverage, and opening a focused PR. Preserve behavior (dry-run via `--apply` off; concurrency validated). All tests table-driven with per-case assert helpers (no `if` in test bodies).

---

## Scope
- Add runner tests covering options and error propagation.
- Create thin repo adapter if needed; keep DB effects behind `FieldingRepo`.
- Thin `cmd/backfill-fielding/main.go` to ≤ 30 LOC (excluding imports/comments) that wires deps and calls `Runner.Run(ctx, opts)`.
- Ensure `make mocks` works with mockery tags; do not add DB/integration tests.
- Achieve per-package coverage targets.

Out of scope: Feature changes, schema changes, or non-deterministic tests.

---

## Files to Create/Modify
1) internal/commands/backfillfielding/runner_test.go
- Table-driven tests for:
  - `--all` vs `--match <id>` behavior
  - invalid options (e.g., neither `--all` nor `--match`)
  - nil service or repo dependency
  - service error propagation (list error, upsert error)

2) cmd/backfill-fielding/main.go
- Parse via `internal/cli/backfillfielding.ParseArgs`.
- Setup logger, context, DB connect/migrations as applicable (behavior-preserving).
- Construct thin repo adapter (if not already present) implementing `internal/db.FieldingRepo`.
- Build service + runner, call `runner.Run(ctx, opts)`.
- Keep ≤ 30 LOC excluding imports/comments.

3) internal/adapters/db/fieldingrepo/repo.go (if missing)
- Implement `internal/db.FieldingRepo` by delegating to existing query helpers.
- No DB logic in tests; adapter compiles and is used by `cmd` only.

4) Tests and mocks
- Ensure mockery tags exist for `FieldingRepo`; run `make mocks`.
- Keep tests table-driven, with per-case assert helper functions; no `if` in test bodies.

---

## Acceptance Criteria
- `cmd/backfill-fielding/main.go` ≤ 30 LOC (excluding imports/comments) and delegates only.
- Coverage thresholds:
  - `internal/services/fielding` ≥ 90%
  - `internal/commands/backfillfielding` ≥ 85%
  - `internal/cli/backfillfielding` ≥ 90%
- `make mocks`, `make test`, `make coverage`, and `make coverage-func` succeed.
- Behavior preserved: dry-run when `--apply` is not set; concurrency validated.

---

## Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused coverage checks
go test -cover ./internal/services/fielding -coverprofile=/tmp/field_svc.out && go tool cover -func=/tmp/field_svc.out | tail -1
go test -cover ./internal/commands/backfillfielding -coverprofile=/tmp/field_cmd.out && go tool cover -func=/tmp/field_cmd.out | tail -1
```

---

## Steps (with status)
1. Add table-driven runner tests for `internal/commands/backfillfielding` (all/dry-run/match/invalid/nil deps/error paths).  
   Status: *
2. Create thin repo adapter `internal/adapters/db/fieldingrepo` if needed; ensure it satisfies `FieldingRepo`.  
   Status: 
3. Thin `cmd/backfill-fielding/main.go` to parse via CLI, wire repo/service/runner, and delegate.  
   Status: 
4. Generate mocks (`make mocks`), run tests and coverage; ensure thresholds (services ≥90%, commands ≥85%).  
   Status: 
5. Open PR on `feat/backfill-fielding-phase1` with Conventional Commits; reconcile to Plan ID 1 (Active path breadcrumb).  
   Status: 

Legend: * = in progress, ✓ = complete, ! = failed

---

## Risks & Mitigations
- Risk: Adapter coupling to DB helpers. Mitigation: Keep adapter thin and focused on interface methods.
- Risk: Coverage dips. Mitigation: Add negative-path cases in runner tests; avoid dead code.
- Risk: Behavior drift in CLI wiring. Mitigation: Preserve flags and default behavior; smoke via Makefile target if present.

---

## Conventional Commits (expected)
- test(backfillfielding): add table-driven runner tests
- feat(backfillfielding): thin cmd to delegate to internal runner
- feat(backfillfielding): add thin fielding repo adapter (if needed)
- chore(mocks): regenerate mockery mocks for FieldingRepo
