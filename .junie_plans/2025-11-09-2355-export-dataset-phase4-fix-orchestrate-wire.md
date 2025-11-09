# Plan ID 1.4.1.4 — Export‑dataset Phase 4 (continue): Fix orchestration tests, align interfaces, and wire services into cmd

Active path: 1 → 4 → 4.1 → 4.1.4  
Parent: 1 → 4 → 4.1  
Branch: feat/exportdataset-phase4

## Goal
Resolve current build/test failures in `internal/commands/exportdataset` orchestration tests, align service interfaces (`io.Writer`), and wire the new services into `cmd/export-dataset/main.go` through a thin DB repo adapter. Preserve behavior and increase coverage toward the ≥80% target.

## Scope
- Fix compile issues in `runner_orchestration_test.go` (helper/type name collisions and signature mismatches).
- Ensure exporter services consistently accept `io.Writer`.
- Introduce a thin DB adapter that implements `internal/db.DatasetRepo` (wrapping existing SQL paths) to be injected into services from `cmd/`.
- Wire `cmd/export-dataset/main.go` to construct `repo -> services -> runner` and delegate unified, legacy combined, and inference-only flows to internal services. Keep remaining per-format training exports in `cmd/` for a later phase.
- Re-run tests, ensure per-package coverage targets for services and commands.

Out of scope (next phases):
- Moving per-format training exports from `cmd/`.
- Broader refactors of other commands.

## Files to Create/Modify
1) internal/commands/exportdataset/runner_orchestration_test.go (modify)
   - Rename conflicting helpers to unique names (e.g., `assertNoErrorLegacyOrch`, `assertNoErrorInferOrch`).
   - Import `io` where needed; ensure fake service methods accept `(ctx context.Context, w io.Writer)` or `(ctx, format, w)` as required by interfaces.
   - Keep tests table-driven with per-case assert functions (no `if` in bodies).

2) internal/commands/exportdataset/runner.go (verify/modify minimally)
   - Ensure `BattingExporter`/`BowlingExporter` method signatures use `io.Writer` consistently.
   - Ensure `writeUsing` persists buffers via `FS.WriteFile` with `0644` perms.

3) internal/adapters/db/exportrepo/exportrepo.go (create)
   - `package exportrepo`
   - `type Repo struct { /* conn or accessors as needed */ }`
   - `func New(/* deps */) *Repo`
   - Implement `internal/db.DatasetRepo` methods by delegating to existing DB/query paths used previously in `cmd/export-dataset`.
   - Minimal surface required by current services only; keep simple to avoid scope creep.
   - Add `//go:generate` comment if wrappers require mocks later (optional for now; primary mocks via `DatasetRepo` interface already covered).

4) cmd/export-dataset/main.go (modify)
   - Construct repo adapter (`exportrepo.New(...)`) using existing DB connection/access patterns.
   - Construct `BattingService` and `BowlingService` with the repo.
   - Use `expcmd.NewRunnerWithServices(fs, bat, bow)`; call `runner.Run(ctx, opts)`.
   - Remove in-file CSV writer logic for:
     - Unified exports (`*_encoded_all.csv`).
     - Legacy combined (`*_encoded.csv`).
     - Inference-only per-format (`*_infer_<FORMAT>.csv`).
   - Preserve filenames and behavior. Leave per-format training exports for a later phase.

5) internal/services/exportdataset/* (verify/tests)
   - Add one additional negative-path test if needed to keep coverage ≥90%.

## Tests to Add/Update
- Fix and finalize `runner_orchestration_test.go` (table-driven, assert helpers only).
- Optional: additional service negative-path test to push coverage ≥90%.
- No E2E tests in this phase.

## Acceptance Criteria
- `go test ./go-app/...` passes locally.
- `internal/commands/exportdataset` package coverage ≥85%.
- `internal/services/exportdataset` package coverage ≥90%.
- `cmd/export-dataset/main.go` delegates unified/legacy-combined/inference-only flows to internal services; file size reduced accordingly.
- Behavior preserved (flags precedence, filenames, and outputs).

## Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused checks
go test -cover ./internal/commands/exportdataset -coverprofile=/tmp/cmd_cover.out && go tool cover -func=/tmp/cmd_cover.out | tail -1
go test -cover ./internal/services/exportdataset -coverprofile=/tmp/svc_cover.out && go tool cover -func=/tmp/svc_cover.out | tail -1
# Smoke run
make export-dataset OUT=../output/go-app
```

## Conventional Commits
- `test(exportdataset): fix orchestration tests and align signatures`
- `feat(exportdataset): wire services into cmd via exportrepo adapter`
- `refactor(cmd/export-dataset): delegate unified/legacy/inference flows to internal services`

## Risks & Mitigations
- Risk: DB adapter may couple to unstable internal SQL helpers.
  - Mitigation: Keep adapter thin and focused on the minimal methods required; adjust in later phases as needed.
- Risk: Behavior drift during delegation.
  - Mitigation: Preserve filenames and flow; smoke test via Makefile; keep resolver/runner logic as single source of truth.

## Breadcrumbs & Anti‑Drift
Active path: 1 → 4 → 4.1 → 4.1.4  
After completing this phase, reconcile back to Plan ID 1, mark this sub‑plan complete (✓), then proceed to extracting per-format training exports in the next sub‑plan (1 → 4 → 4.1 → 4.1.5).
