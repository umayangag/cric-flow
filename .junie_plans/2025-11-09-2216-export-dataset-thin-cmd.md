# Plan ID 1.4.1 — Thin cmd/export-dataset and Delegate to Internal (Phase 1)

Active path: 1 → 4 → 4.1
Parent: 1 → 4

Goal: Make `cmd/export-dataset` a thin bootstrap that parses flags and delegates to `internal/cli/exportdataset` and `internal/commands/exportdataset.Runner`. Preserve current behavior while moving a minimal, safe slice of logic (flag parsing + output dir prep + format selection) into internal, enabling incremental extraction of the remaining heavy logic in follow-up phases.

---

## Scope of this Phase
- Add a concrete filesystem adapter `internal/adapters/fsx/osfs` implementing `fsx.FS` using the standard library.
- Update `cmd/export-dataset/main.go` to:
  - Parse flags using `internal/cli/exportdataset.ParseArgs`.
  - Initialize a real `FS` adapter and a context.
  - Delegate initial responsibilities to `internal/commands/exportdataset.Runner.Run` (currently: validation + `MkdirAll`).
  - Preserve existing behavior for the remainder of the command (export logic) for now to avoid regressions (will be migrated in sub‑phases 1.4.1.1+).
- Add unit tests for the new adapter where feasible (smoke tests), and extend command tests to cover integration of the parsing path where practical (table-driven).
- Keep file size of `cmd/export-dataset/main.go` reduced but allow legacy logic temporarily until follow-up sub‑phases.

---

## Files to Create/Modify
1) Create: `go-app/internal/adapters/fsx/osfs/osfs.go`
   - Implement `FS` using `os.ReadFile`, `os.WriteFile`, `os.MkdirAll`, `filepath.Glob`.
   - Small, documented, and tested.

2) Modify: `go-app/cmd/export-dataset/main.go`
   - Replace in-file flag parsing with a call to `internal/cli/exportdataset.ParseArgs` using a fresh `flag.FlagSet` constructed over `os.Args[1:]`.
   - Build `osfs` adapter instance and pass to `exportdataset.NewRunner`.
   - Call `runner.Run(ctx, opts)` before executing legacy export logic.
   - Gradually gate parts of old logic behind checks using data from `opts` (e.g., formats list when available) without altering external behavior.

3) Create: `go-app/internal/adapters/fsx/osfs/osfs_test.go`
   - Table-driven unit tests for `MkdirAll` happy path and error propagation (use temporary directories).

4) Optional small docs tweak: `go-app/README.md` to mention new internal structure briefly (phase note).

---

## Tests to Add/Update
- `internal/adapters/fsx/osfs/osfs_test.go`: table-driven; assert helpers; no ifs in test bodies.
- Extend `internal/cli/exportdataset/options_test.go` if new parsing nuances arise (e.g., default out dir precedence remains via env).
- No end-to-end tests for full export yet (covered in follow-up sub‑plans when logic moves).

---

## Sub‑plans (under 1.4.1)
- 1.4.1.1 — Migrate format selection from `cmd` to `internal/commands/exportdataset` with tests (unified/all-formats/legacy defaults).
- 1.4.1.2 — Extract CSV writers for batting/bowling into services with interfaces for DB access.
- 1.4.1.3 — Introduce repository interfaces (e.g., `MatchRepo`) with mockery tags and write unit tests for runners/services using mocks.

---

## Acceptance Criteria
- `cmd/export-dataset/main.go` delegates flag parsing and output directory creation to internal packages; file length reduced (target: ≥20% reduction from current 1326 LOC; final ≤ 30 LOC will be achieved in later sub‑plans).
- New `osfs` adapter exists and is unit-tested with table-driven tests (no ifs in test bodies).
- All tests pass: `make test` (Go app).
- No behavior regression for existing CLI flags (manual smoke by running with `--all-formats --out ../output/go-app`).

---

## Verification Commands
- cd go-app
- make test
- make coverage && make coverage-func
- make export-dataset OUT=../output/go-app  # smoke test

---

## Branching and PR
- Branch: `feat/exportdataset-phase1`
- Conventional Commits within this phase:
  - `feat(exportdataset): add osfs adapter implementing fsx.FS`
  - `refactor(exportdataset): parse flags via internal cli and delegate mkdir to Runner`
  - `test(fsx): add table-driven tests for osfs adapter`
- Open PR titled: "export-dataset Phase 1: thin cmd, introduce osfs adapter".

---

## Risk & Mitigation
- Risk: Behavior change due to re-parsing flags. Mitigation: ensure `ParseArgs` mirrors existing defaults; add tests and manual smoke.
- Risk: Partial delegation could introduce duplication. Mitigation: keep legacy logic intact temporarily; next sub‑plans will continue extraction.

---

## Notes
- Continue using mockery via `make mocks` when new interfaces are added in 1.4.1.2/1.4.1.3.
- Follow the Anti‑Drift rule: after completing this sub‑plan, reconcile status in Plan ID 1 and proceed to 1.4.1.1.
