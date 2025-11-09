# Plan ID 1.4.1.2 — Export‑dataset Phase 2: Replace legacy flag parsing + mkdir with internal parser/runner

Active path: 1 → 4 → 4.1 → 4.1.2
Parent: 1 → 4 → 4.1
Branch: feat/exportdataset-phase2

## Goal
Further thin `cmd/export-dataset/main.go` by removing duplicated flag parsing and `MkdirAll` logic, delegating these responsibilities to `internal/cli/exportdataset` and `internal/commands/exportdataset`. Maintain identical user‑visible behavior and outputs.

## Scope
- Do not migrate the heavy SQL/export logic in this phase. Only delegation of flags/options and output directory preparation.
- Wire parsed `cli.Options` (formats/unified/inference-only/outdir) into the existing main flow until subsequent phases extract more logic.

## Files to Modify
1. go-app/cmd/export-dataset/main.go
   - Remove legacy `flag.*Var` declarations for: `out`, `format`, `formats`, `all-formats`, `unified`, `inference-only`.
   - Replace with `exportcli.ParseArgs(flag.NewFlagSet(...), os.Args[1:])`.
   - Use returned `Options` to populate variables used later in the file (e.g., `outDir`, formats list, `unified`, `inferenceOnly`).
   - Remove direct `os.MkdirAll` and call `expcmd.Runner.Run(ctx, opts)` for outdir preparation.
   - Keep the remainder of the file (DB connect, export logic) unchanged.

2. go-app/internal/commands/exportdataset/runner.go
   - No functional change expected. Ensure exported API remains `NewRunner(fs fsx.FS) *Runner` and `Run(ctx, opts) error`.

## Files to Create/Ensure Present (no changes if already present)
- go-app/internal/cli/exportdataset/options.go (already present)
- go-app/internal/commands/exportdataset/runner.go (already present)
- go-app/internal/adapters/fsx/osfs/osfs.go (already present)

## Tests to Add/Update
- No new tests required in this phase (main package is thin and not unit-tested). Ensure existing unit tests stay green.
- If subtle logic mapping from `Options` to legacy variables arises, consider adding a small unit test in `internal/cli/exportdataset` to cover any new helper (but prefer not to add new helpers now).

## Acceptance Criteria
- `cmd/export-dataset/main.go` shows a net reduction of ≥20% lines compared to its pre‑phase2 state and contains only bootstrap/wiring logic for flags + runner call (heavy logic still present for now).
- Legacy behavior preserved:
  - Default output directory precedence remains: flag > env `GO_APP_OUTPUT_DIR` > `config.DefaultExportDir()`.
  - Format selection behavior identical for `--format`, `--formats`, `--all-formats`, and fallback to config settings.
  - `unified` and `inference-only` flags preserved.
- All existing tests pass.

## Verification Commands
```bash
cd go-app
make test
make coverage && make coverage-func
# optional: compare LOC before/after (for info)
wc -l cmd/export-dataset/main.go
```

## Risks & Mitigations
- Risk: Divergence in flag behavior vs legacy parsing.
  - Mitigation: Use `internal/cli/exportdataset.ParseArgs` to mirror flags; ensure fallback behavior in main (config-based defaults) remains the same when `Options.Formats` is empty.
- Risk: Creating outdir twice.
  - Mitigation: Remove `os.MkdirAll` in main once `Runner.Run` is invoked; do not duplicate.

## Steps
1. Refactor `cmd/export-dataset/main.go` to exclusively use `exportcli.ParseArgs` and `expcmd.Runner.Run` for outdir.
2. Map `cli.Options` → legacy variables used by remaining logic:
   - `outDir = opts.OutDir`
   - `unified = opts.Unified`
   - `inferenceOnly = opts.InferenceOnly`
   - formats list: if `opts.Formats != nil` use it; otherwise retain legacy config fallback logic.
3. Run tests and coverage; ensure no behavior regressions.
4. Commit on `feat/exportdataset-phase2` with Conventional Commit message, push, and open PR.

## Conventional Commit Examples
- refactor(cmd/export-dataset): delegate flags and mkdir to internal cli/runner (phase 2)

## Notes
- This plan strictly adheres to the Anti‑Drift rule: After completion, reconcile back to Plan ID 1, mark 4.1.2 complete, and update the active path.
