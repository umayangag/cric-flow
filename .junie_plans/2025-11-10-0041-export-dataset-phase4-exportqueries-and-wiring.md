# Plan ID 1.4.1.4.2 — Export‑dataset Phase 4 (cont.): Implement exportqueries + Wire Services via Repo Adapter

Active path: 1 → 4 → 4.1 → 4.1.4 → 4.1.4.2
Parent: 1 → 4 → 4.1 → 4.1.4
Branch: feat/exportdataset-phase4

---

## 1) Objective
Complete Phase 4 by:
- Implementing read‑only query helpers under `internal/db/exportqueries` for batting and bowling (unified, legacy, inference per-format), returning `[][]string` (with headers as the first row) that match existing CSV outputs.
- Finalizing a thin adapter `internal/adapters/db/exportrepo` that satisfies `internal/db.DatasetRepo` by delegating to `exportqueries`.
- Wiring `cmd/export-dataset/main.go` to construct `repo → services → runner` and delegating unified, legacy combined, and inference‑only flows to `internal/services/exportdataset`. Keep per‑format training (non‑inference) in `cmd/` for the next sub‑phase.
- Ensuring behavior is preserved and package coverage targets are met.

---

## 2) Scope
- In scope: SQL extraction for read‑only helpers; repo adapter; runner/service wiring; tests/coverage verification; smoke run.
- Out of scope: Moving per‑format training (non‑inference) paths from `cmd/` (handled in follow‑up sub‑plan).

---

## 3) Files to Create/Modify
1. Create/Modify: `go-app/internal/db/exportqueries/batting.go`
   - Implement:
     - `BattingUnifiedRows(ctx context.Context) ([][]string, error)`
     - `BattingLegacyRows(ctx context.Context) ([][]string, error)`
     - `BattingInferenceRows(ctx context.Context, format string) ([][]string, error)`
   - Extract SQL from `cmd/export-dataset/main.go` (current batting paths) and map to `[][]string` exactly as CSV writer expects.
   - Robust error handling; no logging.

2. Create/Modify: `go-app/internal/db/exportqueries/bowling.go`
   - Implement:
     - `BowlingUnifiedRows(ctx context.Context) ([][]string, error)`
     - `BowlingLegacyRows(ctx context.Context) ([][]string, error)`
     - `BowlingInferenceRows(ctx context.Context, format string) ([][]string, error)`
   - Extract SQL from `cmd/export-dataset/main.go` (current bowling paths) and map to `[][]string`.

3. Modify: `go-app/internal/adapters/db/exportrepo/exportrepo.go`
   - Implement `DatasetRepo` methods by delegating to `exportqueries`.
   - Keep adapter thin and read‑only.

4. Modify: `go-app/cmd/export-dataset/main.go`
   - Construct `repo := exportrepo.New(/* deps as needed */)`.
   - Construct services: `bat := exportsvc.NewBattingService(repo)` and `bow := exportsvc.NewBowlingService(repo)`.
   - Use `runner := expcmd.NewRunnerWithServices(fs, bat, bow)` and call `runner.Run(ctx, opts)`.
   - Remove in‑file logic for unified (`*_encoded_all.csv`), legacy combined (`*_encoded.csv`), and inference‑only per‑format (`*_infer_<FORMAT>.csv`) in favor of runner/services.
   - Preserve filenames and output schemas.

5. Verify tests remain green in:
   - `internal/services/exportdataset/*_test.go`
   - `internal/commands/exportdataset/*_test.go`
   - Add a tiny negative-path or edge case if needed to maintain coverage thresholds.

---

## 4) Tests to Add/Update
- No new unit tests required if adapters only delegate and services/runner tests already cover flows.
- If coverage dips, add small table‑driven tests for `exportrepo` to assert delegation errors propagate (use minimal fakes; avoid DB).
- All tests must be table‑driven with per‑case assert helpers and no `if` statements in test bodies.

---

## 5) Acceptance Criteria
- `cmd/export-dataset/main.go` delegates unified, legacy combined, and inference‑only flows to internal services (LOC reduced accordingly).
- `internal/commands/exportdataset` package coverage ≥ 85%.
- `internal/services/exportdataset` package coverage ≥ 90% (currently ~93%; must stay ≥90%).
- `exportrepo` compiles and satisfies `db.DatasetRepo` by delegating to `exportqueries`.
- Behavior preserved: identical flags precedence, filenames, and outputs for the extracted flows.

---

## 6) Verification Commands
```
cd go-app
make test
make coverage && make coverage-func
# Focused checks
go test -cover ./internal/services/exportdataset -coverprofile=/tmp/svc_cover.out && go tool cover -func=/tmp/svc_cover.out | tail -1
go test -cover ./internal/commands/exportdataset -coverprofile=/tmp/cmd_cover.out && go tool cover -func=/tmp/cmd_cover.out | tail -1
# Smoke run (requires DB; ensures behavior preserved)
make export-dataset OUT=../output/go-app
```

---

## 7) Plan Hierarchy & Anti‑Drift
- Parent: 1 → 4 → 4.1 → 4.1.4
- This sub‑plan: 1.4.1.4.2 (Implement exportqueries + Wire via Repo)
- After completion:
  - Reconcile back to 1.4.1.4 and mark it ✓ for the covered flows.
  - Create next sub‑plan 1.4.1.5 for per‑format training extraction from `cmd/`.
- Active path to be included in PR descriptions: `1 → 4 → 4.1 → 4.1.4 → 4.1.4.2`.

---

## 8) Steps (with status)
1. Implement batting query helpers under `internal/db/exportqueries/batting.go`.  
   - Extract SQL; return `[][]string` with headers. 
   - Handle errors; no logging.  
   Status: *
2. Implement bowling query helpers under `internal/db/exportqueries/bowling.go`.  
   Status: 
3. Implement `internal/adapters/db/exportrepo` to satisfy `DatasetRepo` by delegating to `exportqueries`.  
   Status: 
4. Wire `cmd/export-dataset/main.go` to use `repo → services → runner` and remove in‑file logic for unified, legacy combined, and inference‑only flows.  
   Status: 
5. Run tests and coverage; ensure thresholds (services ≥90%, commands ≥85%).  
   Status: 
6. Smoke test via `make export-dataset OUT=../output/go-app`.  
   Status: 
7. Open PR on `feat/exportdataset-phase4` with Conventional Commits; reconcile to Plan ID 1.  
   Status: 

Legend: * = in progress, ✓ = complete, ! = failed

---

## 9) Risks & Mitigations
- Risk: SQL extraction mismatches CSV column order.
  - Mitigation: Mirror existing `main.go` CSV headers/ordering exactly; validate with smoke run.
- Risk: Coverage dip due to added adapter code.
  - Mitigation: Add a minimal table‑driven adapter test that asserts error propagation.
- Risk: Behavior drift.
  - Mitigation: Keep filenames and flows identical; rely on existing runner/service tests; smoke test.

---

## 10) Conventional Commits (expected)
- `feat(exportdataset): implement exportqueries and wire services via repo adapter`
- `refactor(cmd/export-dataset): delegate unified/legacy/inference flows to internal services`
- `test(exportdataset): maintain coverage thresholds with table-driven tests`
