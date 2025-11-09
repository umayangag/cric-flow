# Plan ID 1.4.1.4 — Export‑dataset Phase 4: Extract CSV Writers + DB Seams into Services (with Mocks)

Active path: 1 → 4 → 4.1 → 4.1.4
Parent: 1 → 4 → 4.1
Branch: feat/exportdataset-phase4

## Goal
Move the heavy CSV export logic (batting/bowling, legacy and inference variants) out of `cmd/export-dataset/main.go` into testable internal services that depend on small repository interfaces. Introduce `//go:generate mockery` tags for these interfaces and add table‑driven unit tests (no `if` statements in test bodies). Further thin `cmd/` and raise coverage for the newly extracted code.

This phase keeps behavior 100% identical and focuses on restructuring + tests.

---

## Scope
- Introduce repository interfaces (`internal/db` or `internal/repo`) with minimal methods required by exporters. Add mockery tags for mocks.
- Create `internal/services/exportdataset` with separate services for batting and bowling exports (unified, legacy, and inference variants if applicable).
- Update `internal/commands/exportdataset.Runner` to orchestrate calls into these services based on resolved formats and flags.
- Keep `cmd/export-dataset/main.go` delegating only; remove duplicated writer logic.
- Add comprehensive table‑driven unit tests for services and updated runner. Use generated mocks for repos and small fixtures (no DB).

Out of scope:
- Changing CSV schemas or outputs.
- Performance tuning.

---

## Files to Create/Modify
1) Create: `go-app/internal/db/match_repo.go`
   - Content: minimal repository interfaces for exporters.
   - Example:
     ```go
     package db

     import "context"

     //go:generate mockery --name MatchExporterRepo --output internal/mocks --case underscore
     type MatchExporterRepo interface {
         // Provide the minimal data retrieval methods used by exporters.
         // Exact signatures will mirror current SQL usage in cmd/export-dataset.
         // Example (adjust per actual usage):
         // BattingRows(ctx context.Context, format string) ([][]string, error)
         // BowlingRows(ctx context.Context, format string) ([][]string, error)
     }
     ```
   - Note: define precise methods after scanning usages in `main.go` while extracting.

2) Create: `go-app/internal/services/exportdataset/batting.go`
   - Export interface + implementation:
     ```go
     package exportdataset

     import (
         "context"
         "io"
     )

     type BattingExporter interface {
         ExportUnified(ctx context.Context, w io.Writer) error
         ExportLegacy(ctx context.Context, w io.Writer) error
         ExportInference(ctx context.Context, format string, w io.Writer) error
     }

     type BattingService struct { Repo db.MatchExporterRepo /* + FS/Clock if needed */ }
     // Methods call Repo, transform rows, and write CSV via provided io.Writer.
     ```
   - CSV writing stays inside service, pure I/O via `io.Writer` for easy testing.

3) Create: `go-app/internal/services/exportdataset/bowling.go`
   - Mirror batting service for bowling paths (`ExportUnified`, `ExportLegacy`, `ExportInference`).

4) Create tests: `go-app/internal/services/exportdataset/batting_test.go`, `.../bowling_test.go`
   - Table‑driven tests using mockery mocks of `MatchExporterRepo`.
   - Use in‑memory `bytes.Buffer` as `io.Writer` and assert CSV rows/headers written.
   - No `if` in test bodies; assertions encapsulated in helper functions.

5) Modify: `go-app/internal/commands/exportdataset/runner.go`
   - Inject services via constructor:
     ```go
     type Runner struct {
         FS fsx.FS
         Bat BattingExporter
         Bow BowlingExporter
     }

     func NewRunner(fs fsx.FS, bat BattingExporter, bow BowlingExporter) *Runner
     ```
   - Update `Run` to delegate to services depending on `opts.Unified`, `opts.InferenceOnly`, and the formats list from `ResolveFormats`.

6) Update tests: `go-app/internal/commands/exportdataset/runner_test.go`
   - Replace local `mockFS` only test with comprehensive table‑driven tests asserting service method invocations using mock services (hand‑rolled or via mockery interfaces for exporters if desired).

7) Modify: `go-app/cmd/export-dataset/main.go`
   - Wire real dependencies:
     - FS adapter: `osfs.New()` (already present).
     - Repository: thin concrete adapter that satisfies `MatchExporterRepo` (for now, a wrapper that executes existing SQL paths in the short term; or pass a function closure until the adapter exists).
     - Services: `NewBattingService(repo)`, `NewBowlingService(repo)`.
     - Runner: `expcmd.NewRunner(fs, bat, bow)`.
   - Remove the in‑file CSV writer logic that now lives in services; keep only delegation.

8) Ensure mockery config present: `.mockery.yaml` at `go-app` root (already referenced by `Makefile`). If adjustments are needed, update config minimally.

---

## Tests to Add/Update
- `internal/services/exportdataset/*_test.go`
  - Table‑driven cases:
    - Happy paths for unified/legacy/inference write expected CSV headers and a small set of rows (use repo mocks returning deterministic data).
    - Error propagation from repo (e.g., data fetch error).
    - Writer error (wrap a failing `io.Writer`) if applicable.
- `internal/commands/exportdataset/runner_test.go`
  - Table‑driven cases asserting correct method calls based on options and formats.
- Maintain existing tests for `cli`, `osfs`, and `formats` resolver.

All tests must:
- Be table‑driven.
- Use per‑case assert helper functions (no `if` in test bodies).
- Be deterministic and small. No DB/network.

---

## Acceptance Criteria
- `cmd/export-dataset/main.go` is significantly reduced (target an additional ≥30% LOC reduction this phase). It should only parse options (already delegated), wire dependencies, and call runner.
- New services implemented and covered by tests ≥85% per package; updated `commands/exportdataset` package remains ≥85%.
- Interfaces for repositories defined with `//go:generate mockery` tags; `make mocks` succeeds.
- Behavior preserved for all combinations of flags: `--unified`, `--inference-only`, explicit formats, config fallbacks, and legacy combined mode.
- Global coverage increases from ~20.5% toward the 80% target.

---

## Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Optional: check only services/commands coverage
go test -cover ./internal/services/exportdataset -coverprofile=/tmp/cover1.out && go tool cover -func=/tmp/cover1.out | tail -1
go test -cover ./internal/commands/exportdataset -coverprofile=/tmp/cover2.out && go tool cover -func=/tmp/cover2.out | tail -1
# Smoke run to ensure behavior unchanged
make export-dataset OUT=../output/go-app
```

---

## Steps
1. Define repository interfaces with mockery tags (minimal required methods by exporters).
2. Implement batting and bowling services operating on those interfaces, writing CSV to `io.Writer`.
3. Write failing unit tests first (TDD) for services using repo mocks → implement until green.
4. Update `Runner` to depend on services; extend its tests (use service mocks) to assert orchestration.
5. Thin `cmd/export-dataset/main.go` by replacing inline CSV writer logic with service calls via runner.
6. Run `make mocks`, `make test`, and coverage; perform smoke test.
7. Commit to `feat/exportdataset-phase4` with Conventional Commits; open a small PR.

---

## Conventional Commits
- `feat(exportdataset): introduce batting/bowling services and repository interfaces`
- `test(exportdataset): table-driven tests for services and runner orchestration`
- `refactor(cmd/export-dataset): delegate CSV writing to internal services`

---

## Risks & Mitigations
- Risk: Interface method creep or tight coupling to current SQL.
  - Mitigation: Start with minimal interfaces; keep services focused on formatting/writing; adapters can evolve later.
- Risk: Behavior drift in CSV output.
  - Mitigation: Add golden-line assertions in tests for headers and sample rows; smoke test with Make target.
- Risk: Too‑large PR.
  - Mitigation: If needed, split into two PRs: (a) introduce interfaces + batting service + tests; (b) add bowling + runner wiring.

---

## Notes (Anti‑Drift & Breadcrumbs)
- After completing this sub‑plan, reconcile back to Plan ID 1: mark `1 → 4 → 4.1 → 4.1.4` ✓ and proceed to extract remaining commands (4.2+) following the same pattern.
- Keep using `mockery` via `make mocks`. Ensure generated mocks live under `go-app/internal/mocks` as configured.
