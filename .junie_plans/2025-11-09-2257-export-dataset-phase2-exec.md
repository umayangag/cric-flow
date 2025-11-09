# Plan ID 1.4.1.2 — Export Dataset Phase 2: Delegate Flags + Mkdir from cmd to Internal

Active path: 1 → 4 → 4.1 → 4.1.2
Parent: 1 → 4 → 4.1

Goal: Replace legacy flag parsing and `os.MkdirAll` in `cmd/export-dataset/main.go` with the internal `cli` parser and `commands` runner while preserving behavior. Keep heavy SQL/export logic in place for now. This continues thinning `cmd/` toward a ≤30 LOC delegator in later phases.

---

## Scope
- Remove duplicate flag variables and parsing from `cmd/export-dataset/main.go`.
- Parse args using `internal/cli/exportdataset.ParseArgs` only.
- Resolve default `OutDir` precedence (flag > env `GO_APP_OUTPUT_DIR` > `config.DefaultExportDir()`), apply via internal runner.
- Replace direct `os.MkdirAll` with `internal/commands/exportdataset.Runner.Run` (already validated in tests), keeping logging & DB connect as-is.
- No change to downstream export logic yet (format loops, writers, SQL remain for next phases).

---

## Files to Modify
1) go-app/cmd/export-dataset/main.go
- Remove local flag variables and `flag.*Var` setup.
- Construct `flag.FlagSet` and call `exportdataset.ParseArgs(fs, os.Args[1:])`.
- Post-parse: if `opts.OutDir` empty, compute default (`env or config.DefaultExportDir()`), then set on `opts`.
- Instantiate real FS adapter `internal/adapters/fsx/osfs.New()`; create `commands/exportdataset.NewRunner(fs)` and call `Run(ctx, opts)`.
- Delete direct `os.MkdirAll(outDir, 0o755)`; rely on runner.
- Preserve logger setup and DB connection.
- Continue with legacy export logic using `opts` values (e.g., use `opts.Formats` when non-empty, fallback to legacy behavior otherwise), without altering outputs.

No other files are changed in this phase.

---

## Tests to Add/Update
- Unit tests already exist for:
  - `internal/cli/exportdataset.ParseArgs` (table-driven)
  - `internal/commands/exportdataset.Runner.Run` (mkdir + validation)
- Optional minor test extension (if needed): ensure env default precedence in `internal/cli/exportdataset/options_test.go` by adding a case for `GO_APP_OUTPUT_DIR` default.
- No E2E or integration tests in this phase.

All tests must be table-driven; no `if` in test bodies (use assert helpers).

---

## Acceptance Criteria
- `cmd/export-dataset/main.go` no longer defines/uses local flag variables nor calls `os.MkdirAll`.
- Flags/options are parsed exclusively via `internal/cli/exportdataset.ParseArgs`.
- Output directory creation is done only through `internal/commands/exportdataset.Runner.Run`.
- Behavior preserved for existing CLI usage; manual smoke works using Makefile target.
- New/updated tests (if any) pass; project builds.

---

## Verification Commands
- cd go-app
- make test
- make coverage && make coverage-func
- make export-dataset OUT=../output/go-app  # smoke run

---

## Branching & PR
- Branch: `feat/exportdataset-phase2`
- Conventional commits:
  - `refactor(exportdataset): delegate flag parsing to internal cli and mkdir to runner`
  - `test(cli): add table-driven case for GO_APP_OUTPUT_DIR default` (if added)
- Open PR: "export-dataset Phase 2: delegate flags + mkdir to internal"

---

## Notes & Next Steps (Breadcrumbs)
- Next phases under 1.4.1:
  - 1.4.1.3 — Migrate format selection logic fully into runner/service with tests.
  - 1.4.1.4 — Extract CSV writers and DB seams (repos + mockery) and add unit tests.
- After merging this PR, reconcile status back to Plan ID 1 and update active path.
