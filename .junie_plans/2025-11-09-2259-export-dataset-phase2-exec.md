# Plan ID 1.4.1.2 — Export‑dataset Phase 2 Execution: Delegate flags + outdir to internal

Active path: 1 → 4 → 4.1 → 4.1.2
Parent: 1 → 4 → 4.1
Branch: feat/exportdataset-phase2

## Goal
Replace legacy flag parsing and outdir creation in `cmd/export-dataset/main.go` with the internal `cli` parser and `commands` runner while preserving behavior. This continues thinning `cmd/` without migrating heavy export logic yet.

## Scope
- Delegate all flag parsing to `internal/cli/exportdataset.ParseArgs`.
- Delegate output directory creation to `internal/commands/exportdataset.Runner.Run` (backed by `fsx.FS`).
- Do NOT move SQL/export logic in this phase; only wiring and delegation.

## Files to Modify
1. go-app/cmd/export-dataset/main.go
   - Remove legacy `flag.*Var` for: `out`, `format`, `formats`, `all-formats`, `unified`, `inference-only`.
   - Parse args via `exportcli.ParseArgs(flag.NewFlagSet("export-dataset", flag.ContinueOnError), os.Args[1:])`.
   - Build `fs := osfs.New()` and `runner := expcmd.NewRunner(fs)`.
   - Call `runner.Run(ctx, opts)` to create the output dir (remove direct `os.MkdirAll`).
   - Map `opts` → existing variables for the remaining legacy flow:
     - `outDir = opts.OutDir`
     - `unified = opts.Unified`
     - `inferenceOnly = opts.InferenceOnly`
     - formats list: if `opts.Formats != nil`, use it; otherwise retain legacy/config fallback behavior.
   - Keep DB connect and heavy export logic unchanged for now.

2. go-app/internal/cli/exportdataset/options_test.go (optional)
   - Add a case verifying env‑default precedence for `GO_APP_OUTPUT_DIR` when `-out` not provided.

## Files to Ensure Present (no change expected)
- go-app/internal/cli/exportdataset/options.go
- go-app/internal/commands/exportdataset/runner.go
- go-app/internal/adapters/fsx/osfs/osfs.go

## Tests to Add/Update
- No new tests required for `main` package.
- Maintain existing unit tests for `cli`, `commands`, and `osfs`.
- Optional: extend `options_test.go` with env default precedence.

## Acceptance Criteria
- `cmd/export-dataset/main.go` delegates flag parsing and `MkdirAll` via internal packages; duplicate legacy parsing removed.
- Net LOC reduction in `cmd/export-dataset/main.go` by ≥20% relative to pre‑phase2 baseline; final ≤30 LOC target remains for later phases.
- Behavior preserved:
  - Output dir precedence: flag > env `GO_APP_OUTPUT_DIR` > `config.DefaultExportDir()`.
  - Formats behavior matches legacy for `--format`, `--formats`, `--all-formats`; if `opts.Formats` empty, legacy/config fallback still applies.
  - `--unified` and `--inference-only` flags preserved.
- All tests pass: `make test`.

## Verification Commands
```bash
cd go-app
make test
make coverage && make coverage-func
# Optional: check line count trend
wc -l cmd/export-dataset/main.go
# Smoke run
make export-dataset OUT=../output/go-app
```

## Steps
1. Refactor `cmd/export-dataset/main.go` to exclusively use `ParseArgs` and `Runner.Run`.
2. Map `cli.Options` into variables used by the remaining legacy logic.
3. Remove direct `os.MkdirAll` (outdir created by `Runner.Run`).
4. Run tests and coverage; perform a smoke run.
5. Commit to `feat/exportdataset-phase2` with a Conventional Commit and open a small PR.

## Conventional Commit Examples
- refactor(cmd/export-dataset): delegate flags + outdir to internal cli/runner (phase 2)

## Risks & Mitigations
- Risk: Subtle divergence in flag behavior. Mitigation: reuse shared parser; retain config fallback when `Formats` empty; add/keep targeted tests.
- Risk: Outdir created twice. Mitigation: remove direct `os.MkdirAll` once `Runner.Run` is invoked.

## Notes
- Adheres to Anti‑Drift rule. After completion, reconcile back to Plan ID 1: mark 1 → 4 → 4.1 → 4.1.2 complete, then proceed to the next sub‑plan.
