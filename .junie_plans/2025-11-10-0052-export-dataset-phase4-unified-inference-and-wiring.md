# Plan ID 1.4.1.4.2.c — Export‑dataset Phase 4 (cont.): Implement Unified + Inference Query Helpers and Wire CMD Delegation

Active path: 1 → 4 → 4.1 → 4.1.4 → 4.1.4.2 → 4.1.4.2.c
Parent: 1 → 4 → 4.1 → 4.1.4 → 4.1.4.2
Branch: feat/exportdataset-phase4

---

## Goal
Complete Phase 4 by:
- Implementing the remaining unified and inference helpers in `internal/db/exportqueries` (batting and bowling) mirroring the legacy SQL/CSV order from `cmd/export-dataset/main.go`.
- Wiring `cmd/export-dataset/main.go` to construct `repo → services → runner` and delegating unified, legacy combined, and inference‑only flows entirely to internal services.
- Preserving behavior and filenames while increasing coverage in internal packages and reducing `cmd/` logic.

---

## Scope
In scope:
- Implement the following functions with behavior identical to legacy code (headers + row order):
  - `internal/db/exportqueries/batting.go`:
    - `BattingUnifiedRows(ctx context.Context) ([][]string, error)`
    - `BattingInferenceRows(ctx context.Context, format string) ([][]string, error)`
  - `internal/db/exportqueries/bowling.go`:
    - `BowlingUnifiedRows(ctx context.Context) ([][]string, error)`
    - `BowlingInferenceRows(ctx context.Context, format string) ([][]string, error)`
- Update `cmd/export-dataset/main.go` to:
  - Instantiate `exportrepo.Repo`, services, and runner via `NewRunnerWithServices`.
  - Delegate unified, legacy combined (unsuffixed), and inference‑only flows to services through the runner.
  - Leave per‑format training exports (non‑inference) for a later phase.

Out of scope:
- Changing SQL semantics, CSV schemas, or filenames.
- Migrating per‑format training exports (handled in the next sub‑plan).

---

## Files to Create/Modify
1) Modify: `go-app/internal/db/exportqueries/batting.go`
- Implement `BattingUnifiedRows` by extracting SQL from `exportBattingUnified`.
- Implement `BattingInferenceRows` by extracting SQL from `exportBattingFormatInference`.
- Reuse local scan utilities; include CSV header row as the first entry.

2) Modify: `go-app/internal/db/exportqueries/bowling.go`
- Implement `BowlingUnifiedRows` by extracting SQL from `exportBowlingUnified` (legacy content in `main.go`).
- Implement `BowlingInferenceRows` by extracting from `exportBowlingFormatInference`.
- Reuse local scan utilities; include CSV header row as the first entry.

3) Modify: `go-app/cmd/export-dataset/main.go`
- Construct:
  - `fs := osfs.New()`
  - `repo := exportrepo.New()`
  - `bat := exportsvc.NewBattingService(repo)`
  - `bow := exportsvc.NewBowlingService(repo)`
  - `runner := expcmd.NewRunnerWithServices(fs, bat, bow)`
- Delegate:
  - Unified → runner (writes `batting_encoded_all.csv`, `bowling_encoded_all.csv`).
  - Legacy combined (empty format sentinel) → runner (writes `batting_encoded.csv`, `bowling_encoded.csv`).
  - Inference‑only per‑format → runner (writes `batting_infer_<FMT>.csv`, `bowling_infer_<FMT>.csv`).
- Remove/replace corresponding inline CSV writing branches in `main.go`.
- Keep per‑format training (non‑inference) temporarily.

---

## Tests to Add/Update
- No new DB‑backed unit tests (helpers use real DB; we avoid integration tests here).
- Ensure existing tests remain green:
  - `internal/services/exportdataset/*_test.go` (table‑driven, ≥90% coverage)
  - `internal/commands/exportdataset/runner_*test.go` (orchestration, ≥85% coverage)
- Optional: a compile‑time test for `exportrepo` construction (no DB access).

All tests must be table‑driven with per‑case assert helpers; no `if` statements in test bodies.

---

## Acceptance Criteria
- `internal/db/exportqueries` fully implements unified and inference helpers (batting + bowling) with column/row order identical to legacy.
- `cmd/export-dataset/main.go` delegates unified, legacy combined, and inference‑only flows to services via the runner; LOC reduced in those sections.
- `internal/commands/exportdataset` coverage ≥ 85%; `internal/services/exportdataset` coverage ≥ 90%.
- Behavior preserved (same filenames, same CSV columns, same order).
- `go test ./...` passes.

---

## Verification Commands
```bash
cd go-app
make test
make coverage && make coverage-func
# Focused checks (optional):
go test -cover ./internal/commands/exportdataset -coverprofile=/tmp/cmd_cover.out && go tool cover -func=/tmp/cmd_cover.out | tail -1
go test -cover ./internal/services/exportdataset -coverprofile=/tmp/svc_cover.out && go tool cover -func=/tmp/svc_cover.out | tail -1
# Smoke run (should produce same files as before for these flows)
make export-dataset OUT=../output/go-app
```

---

## Steps
1. Implement `BattingUnifiedRows` and `BattingInferenceRows` in `exportqueries/batting.go` (copy SQL from legacy functions, preserve headers and order).*
2. Implement `BowlingUnifiedRows` and `BowlingInferenceRows` in `exportqueries/bowling.go` (copy SQL from legacy functions, preserve headers and order).*
3. Wire `cmd/export-dataset/main.go` to build `repo → services → runner` and remove inline CSV writing for unified, legacy combined, and inference‑only paths.*
4. Run tests and coverage; perform a smoke run via Makefile.*
5. Commit to `feat/exportdataset-phase4` with Conventional Commits and open a focused PR. Reconcile back to Plan ID 1 (update breadcrumbs and statuses).*

Progress markers:
- Steps marked with * are intended for immediate execution in this sub‑plan.

---

## Conventional Commits
- `feat(exportdataset): implement unified/inference exportqueries and wire cmd delegation`
- `refactor(cmd/export-dataset): delegate unified/legacy/inference flows to services via runner`
- `test(commands/services): keep orchestration and service tests green`

---

## Risks & Mitigations
- Risk: SQL extraction mistakes or column order drift.
  - Mitigation: Copy verbatim; compare headers with legacy code; smoke test outputs.
- Risk: Behavior drift in filenames or path composition.
  - Mitigation: Keep filename literals identical to legacy in runner; orchestration tests already assert file paths.
- Risk: DB dependency in unit tests slows CI.
  - Mitigation: No DB tests added here; rely on smoke runs for adapter + queries.

---

## Breadcrumbs & Anti‑Drift
Active path: 1 → 4 → 4.1 → 4.1.4 → 4.1.4.2 → 4.1.4.2.c
After completing this sub‑plan, reconcile back to Plan ID 1, mark this node complete (✓), and proceed to the next sub‑plan to migrate per‑format training exports out of `cmd/`.
