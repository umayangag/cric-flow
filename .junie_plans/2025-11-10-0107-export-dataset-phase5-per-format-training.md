# Plan ID 1.4.1.5 — Export‑dataset Phase 5: Extract Per‑Format Training Exports from cmd to Internal

Active path: 1 → 4 → 4.1 → 4.1.5
Parent: 1 → 4 → 4.1
Branch: feat/exportdataset-phase5

---

## 1) Objective
Complete the extraction of remaining heavy logic from `cmd/export-dataset/main.go` by moving the per‑format training (non‑inference) export paths into internal services behind interfaces, and orchestrate them via `internal/commands/exportdataset.Runner`. Keep behavior, filenames, and CSV schemas identical. Maintain testing discipline (table‑driven, no ifs in test bodies) and coverage thresholds.

---

## 2) Scope
- Add per‑format training export methods to batting/bowling services.
- Extend `internal/db.DatasetRepo` and `internal/db/exportqueries` to provide per‑format training rows.
- Update `internal/commands/exportdataset.Runner` to orchestrate per‑format training exports and write files using `FS`.
- Thin `cmd/export-dataset/main.go` to pure wiring (flags → opts, deps wiring → runner call).
- Keep unified, legacy combined, and inference‑only flows as already delegated (no changes needed there).

Out of scope:
- Any change in SQL, CSV shapes, filenames, or behavior.
- Extraction of other commands (handled in subsequent plan items under 1 → 4).

---

## 3) Files to Create/Modify
1. Modify: `go-app/internal/db/dataset_repo.go`
   - Add methods with mockery tag still valid:
     - `BattingFormatRows(ctx context.Context, format string) ([][]string, error)`
     - `BowlingFormatRows(ctx context.Context, format string) ([][]string, error)`
   - Ensure `//go:generate mockery` continues to cover `DatasetRepo`.

2. Modify: `go-app/internal/db/exportqueries/batting.go`
   - Implement read‑only helper mirroring legacy `exportBattingFormat` (header + order identical):
     - `func BattingFormatRows(ctx context.Context, format string) ([][]string, error)`

3. Modify: `go-app/internal/db/exportqueries/bowling.go`
   - Implement read‑only helper mirroring legacy `exportBowlingFormat` (header + order identical):
     - `func BowlingFormatRows(ctx context.Context, format string) ([][]string, error)`

4. Modify: `go-app/internal/adapters/db/exportrepo/exportrepo.go`
   - Delegate new `DatasetRepo` methods to `exportqueries.BattingFormatRows` and `exportqueries.BowlingFormatRows`.

5. Modify: `go-app/internal/services/exportdataset/batting.go`
   - Extend interface and service with:
     - `ExportFormat(ctx context.Context, format string, w io.Writer) error`
   - Implement using `Repo.BattingFormatRows` → `writeCSV`.

6. Modify: `go-app/internal/services/exportdataset/bowling.go`
   - Extend interface and service with:
     - `ExportFormat(ctx context.Context, format string, w io.Writer) error`
   - Implement using `Repo.BowlingFormatRows` → `writeCSV`.

7. Modify: `go-app/internal/commands/exportdataset/runner.go`
   - In `Run`, when `opts.InferenceOnly == false` and formats contain specific codes (non‑empty, non‑combined sentinel), write:
     - `batting_encoded_<FORMAT>.csv`
     - `bowling_encoded_<FORMAT>.csv`
   - Use `writeUsing` with service calls `Bat.ExportFormat` and `Bow.ExportFormat`.

8. Modify: `go-app/cmd/export-dataset/main.go`
   - Remove remaining in‑file per‑format training branches; leave only wiring and a single `runner.Run(ctx, opts)`.

---

## 4) Tests to Add/Update
- `internal/services/exportdataset/batting_test.go`
  - Add table‑driven cases for `ExportFormat` (happy path, repo error, writer error).
- `internal/services/exportdataset/bowling_test.go`
  - Add table‑driven cases for `ExportFormat` (happy path, repo error, writer error).
- `internal/commands/exportdataset/runner_orchestration_test.go`
  - Add a table‑driven case for per‑format training: ensure correct filenames `batting_encoded_<F>.csv` and `bowling_encoded_<F>.csv`, correct service method invocation counts, and FS write permissions preserved (0644).
- Do not add DB/integration tests.

All tests must:
- Be table‑driven.
- Use per‑case assert helpers (no ifs in test bodies).
- Remain deterministic.

---

## 5) Acceptance Criteria
- `cmd/export-dataset/main.go` contains only thin wiring (flags → opts, deps → runner) with no export logic.
- `internal/commands/exportdataset` orchestrates all flows: unified, legacy combined, inference‑only, and per‑format training.
- `internal/services/exportdataset` exposes `ExportFormat` for batting/bowling; package coverage remains ≥ 90%.
- `internal/commands/exportdataset` package coverage remains ≥ 85%.
- `DatasetRepo` extended and mockery tags intact; `make mocks` works.
- Behavior unchanged: filenames, column order, and outputs identical to legacy.

---

## 6) Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused coverage (optional)
go test -cover ./internal/services/exportdataset -coverprofile=/tmp/svc_cover.out && go tool cover -func=/tmp/svc_cover.out | tail -1
go test -cover ./internal/commands/exportdataset -coverprofile=/tmp/cmd_cover.out && go tool cover -func=/tmp/cmd_cover.out | tail -1
# Smoke run (requires DB)
make export-dataset OUT=../output/go-app
```

---

## 7) Steps (with status)
1. Extend `DatasetRepo` and add `exportqueries` helpers for per‑format training (batting/bowling).  *
2. Update `exportrepo` to delegate the new methods.  *
3. Extend services with `ExportFormat` and add unit tests (happy + error + writer error).  *
4. Update `Runner` to orchestrate per‑format training file writes; extend orchestration tests.
5. Thin `cmd/export-dataset/main.go` by removing per‑format training logic and rely solely on the runner.
6. Run `make mocks`, tests, and coverage; ensure thresholds met.
7. Smoke test `make export-dataset OUT=../output/go-app` to confirm behavior.
8. Commit to `feat/exportdataset-phase5` with Conventional Commits and open a PR; reconcile back to Plan ID 1.

Legend: * = in progress, ✓ = complete, ! = failed

---

## 8) Conventional Commits
- `feat(exportdataset): add per-format training exports to services and repo`
- `refactor(exportdataset): delegate per-format training to runner`
- `refactor(cmd/export-dataset): remove in-file per-format training branches`
- `test(exportdataset): table-driven tests for per-format training flows`

---

## 9) Risks & Mitigations
- Risk: Column/order drift when replicating legacy per‑format queries.
  - Mitigation: Copy SQL and headers verbatim from `exportBattingFormat` / `exportBowlingFormat`; validate with smoke tests.
- Risk: Coverage dips due to added code.
  - Mitigation: Add focused unit tests for new methods and orchestration path.
- Risk: Interface churn on `DatasetRepo`.
  - Mitigation: Keep additions minimal; document intent and keep adapter thin.
