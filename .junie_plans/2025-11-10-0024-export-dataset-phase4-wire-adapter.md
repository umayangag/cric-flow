# Plan ID 1.4.1.4.2 — Export‑dataset Phase 4 (cont.): Query Helpers + Repo Adapter + CMD Wiring

Active path: 1 → 4 → 4.1 → 4.1.4 → 4.1.4.2
Parent: 1 → 4 → 4.1 → 4.1.4
Branch: feat/exportdataset-phase4

---

## Goal
Complete Phase 4 by extracting read‑only query helpers into `internal/db/exportqueries`, implementing a thin repo adapter `internal/adapters/db/exportrepo` that satisfies `internal/db.DatasetRepo`, and wiring `cmd/export-dataset/main.go` to delegate unified, legacy combined, and inference‑only flows entirely to internal services via the `Runner`. Preserve behavior and raise coverage for internal packages.

---

## Scope
- Add `exportqueries` package exposing minimal functions that return `[][]string` for batting and bowling (unified, legacy, inference).
- Add `exportrepo` adapter implementing `db.DatasetRepo` by delegating to `exportqueries` and using existing DB connect helpers.
- Modify `cmd/export-dataset/main.go` to construct `repo → services → runner` and remove in‑file logic for unified, legacy combined, and inference‑only flows (per‑format training exports may remain temporarily and will be handled in a later phase).
- Keep CSV schemas and filenames unchanged.

Out of scope:
- Changing SQL, CSV shapes, or behavior.
- Per‑format training exports (non‑inference) — to be migrated in a subsequent sub‑plan.

---

## Files to Create/Modify
1) Create: `go-app/internal/db/exportqueries/batting.go`
   - Functions (signatures only here; read‑only queries inside):
     - `func BattingUnifiedRows(ctx context.Context) ([][]string, error)`
     - `func BattingLegacyRows(ctx context.Context) ([][]string, error)`
     - `func BattingInferenceRows(ctx context.Context, format string) ([][]string, error)`
   - Leverage existing SQL from `cmd/export-dataset/main.go` (copy/adapt without behavior changes).
   - Error wrapping; no logging.

2) Create: `go-app/internal/db/exportqueries/bowling.go`
   - Functions:
     - `func BowlingUnifiedRows(ctx context.Context) ([][]string, error)`
     - `func BowlingLegacyRows(ctx context.Context) ([][]string, error)`
     - `func BowlingInferenceRows(ctx context.Context, format string) ([][]string, error)`

3) Create: `go-app/internal/adapters/db/exportrepo/exportrepo.go`
   - Type `Repo` with dependency on existing DB connection (e.g., use `db.Connect(ctx)` internally).
   - Implement `internal/db.DatasetRepo` by delegating to `exportqueries.*` functions.
   - Construction: `func New() *Repo` (no global state).

4) Modify: `go-app/cmd/export-dataset/main.go`
   - Build adapters and services:
     - `fs := osfs.New()`
     - `repo := exportrepo.New()` (implements `db.DatasetRepo`)
     - `bat := services.NewBattingService(repo)`
     - `bow := services.NewBowlingService(repo)`
     - `runner := expcmd.NewRunnerWithServices(fs, bat, bow)`
   - Remove/replace the in‑file logic for:
     - Unified exports (batting/bowling)
     - Legacy combined exports (unsuffixed CSVs)
     - Inference‑only per‑format exports
   - Keep per‑format training exports (non‑inference) for a later phase.

5) Tests
   - `internal/services/exportdataset` tests already validate CSV writing (keep green).
   - `internal/commands/exportdataset` orchestration tests already cover unified/legacy/inference paths (keep green; update if file names/paths change — not expected).
   - Optional compile‑time/constructor test for `exportrepo` (no DB access in unit tests).

---

## Acceptance Criteria
- `cmd/export-dataset/main.go` delegates unified, legacy combined, and inference‑only flows to internal runner/services; LOC is reduced meaningfully in these sections.
- `internal/commands/exportdataset` package coverage ≥ 85% (orchestra tests pass).
- `internal/services/exportdataset` coverage remains ≥ 90%.
- New adapter compiles and satisfies `db.DatasetRepo`; no new flaky/integration tests introduced.
- Behavior preserved (same filenames and CSV schemas for all flows moved).

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
1. Implement `internal/db/exportqueries` for batting and bowling read‑only queries. (TDD limited to compile‑time/logic helpers; no DB in unit tests.)
2. Implement `internal/adapters/db/exportrepo` to satisfy `db.DatasetRepo` via `exportqueries`.
3. Wire `cmd/export-dataset/main.go` to use `repo → services → runner`; remove inline CSV writing for unified, legacy combined, and inference‑only flows.
4. Run `make test`, `make coverage`, and perform a smoke run using the Make target.
5. Commit to `feat/exportdataset-phase4` with Conventional Commits and open a PR. Reconcile back to Plan ID 1 (Active path breadcrumb update).

---

## Conventional Commits
- `feat(exportdataset): add exportqueries and exportrepo adapter for DatasetRepo`
- `refactor(cmd/export-dataset): delegate unified/legacy/inference flows to services via runner`
- `test(commands/services): keep orchestration and service tests green`

---

## Risks & Mitigations
- Risk: Query drift or SQL mistakes when extracting from `cmd`.
  - Mitigation: Copy queries verbatim; limit refactor to function boundaries; smoke test outputs.
- Risk: Accidental behavior change (filenames or order of rows).
  - Mitigation: Preserve file naming and write order; rely on existing service tests for CSV writing; manual smoke verification.
- Risk: DB dependency creeping into unit tests.
  - Mitigation: Do not add DB integration tests here; keep adapter untested beyond compile‑time; rely on smoke.
