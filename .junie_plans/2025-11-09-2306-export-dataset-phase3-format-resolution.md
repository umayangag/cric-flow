# Plan ID 1.4.1.3 — Export‑dataset Phase 3: Migrate Format Resolution into Internal (pure, testable)

Active path: 1 → 4 → 4.1 → 4.1.3
Parent: 1 → 4 → 4.1
Branch: feat/exportdataset-phase3

## Goal
Move all format selection logic (flags → formats list with config/env fallbacks) out of `cmd/export-dataset/main.go` into `internal/commands/exportdataset` as pure, testable functions. Keep `cmd` as a thin delegator and preserve current behavior identically.

This phase keeps the heavy DB/CSV export logic as-is; only format resolution moves. We will write table-driven tests with per-case assert helpers and achieve ≥85% coverage for the new logic.

---

## Scope
- Implement a pure resolver that computes the list of formats based on:
  - CLI options (`cli.Options`): `Formats`, `Unified`, etc.
  - App configuration (`config.Config`): `Export.RequiredFormat`, `Export.SplitByFormat` (mirroring legacy behavior).
- Update `cmd/export-dataset/main.go` to call the resolver instead of locally computing formats. Remove now-redundant format parsing/branching blocks from `main`.
- Preserve all existing outputs and defaults: when no flags are provided, fallback to config; if neither specifies, legacy default remains `[""]` (combined format, unsuffixed files), identical to current behavior.

Out of scope for this phase:
- Moving the DB/CSV writer logic; that will be the next phase.

---

## Files to Create/Modify
1) Create: `go-app/internal/commands/exportdataset/format.go`
   - Expose a minimal API:
     - `func ResolveFormats(opts cli.Options, cfg *config.Config) []string`
   - Behavior rules (must match current `main.go`):
     - If `opts.Formats` is non-nil/non-empty → return it as-is (normalized to `TEST|ODI|T20|T20I` already by CLI parser; keep order stable).
     - Else, read from `cfg`:
       - If `cfg.Export.RequiredFormat` is non-empty → single element list with normalized uppercased value.
       - Else if `cfg.Export.SplitByFormat` → `[]string{"TEST","ODI","T20","T20I"}`.
       - Else → `[]string{""}` (legacy combined, unsuffixed files).
   - Add small helpers if needed (e.g., `normFormat(s string) string`). Keep functions small and pure.

2) Modify: `go-app/cmd/export-dataset/main.go`
   - Replace the in-file logic that builds `list` of formats with a call to `exportdataset.ResolveFormats(opts, config.Load())`.
   - Remove now-unused variables `format`, `formats`, `allFormats` and the branching that constructs `list`.
   - Keep everything else unchanged for this phase (DB connect, unified handling, etc.).

3) Create tests: `go-app/internal/commands/exportdataset/format_test.go`
   - Table-driven tests (no `if` in test bodies) covering:
     - Flags precedence: `--format`, `--formats`, `--all-formats` handled upstream by CLI → ensure `opts.Formats` respected.
     - Config fallbacks: `RequiredFormat`, `SplitByFormat`.
     - Legacy default when neither flags nor config specify formats → `[]string{""}`.
     - Normalization cases (case/whitespace) where applicable.
   - Use per-testcase assert functions per guidelines.

---

## Tests to Add/Update
- `internal/commands/exportdataset/format_test.go` — table-driven, per-case assert helpers; target ≥90% coverage for `format.go`.
- No new tests in `cmd`.

---

## Acceptance Criteria
- `cmd/export-dataset/main.go` no longer computes formats itself; it calls `internal/commands/exportdataset.ResolveFormats`.
- Behavior preserved exactly vs. current implementation (manual smoke for a few combinations).
- `internal/commands/exportdataset` package coverage ≥ 85% (new files ≥ 90%).
- All tests green: `make test`; coverage runs.

---

## Verification Commands
- cd go-app
- make test
- make coverage && make coverage-func
- Smoke tests (examples):
  - `make export-dataset OUT=../output/go-app` (uses config fallback)
  - `go run ./cmd/export-dataset --formats TEST,T20I --out ../output/go-app` (uses flags)

---

## Risks & Mitigations
- Risk: Subtle divergence from legacy default behavior.
  - Mitigation: Encode the current behavior in table-driven tests; compare a few smoke runs.
- Risk: Hidden normalization differences.
  - Mitigation: Keep normalization responsibilities in CLI parser; resolver assumes normalized `opts.Formats`.

---

## Conventional Commits
- `refactor(exportdataset): move format resolution into internal resolver`
- `test(exportdataset): table-driven tests for format resolver`

---

## Notes (Anti‑Drift & Breadcrumbs)
- Active path: 1 → 4 → 4.1 → 4.1.3
- After completing this phase, reconcile status back to Plan ID 1, mark 4.1.3 ✓, and proceed to the next sub‑phase to extract CSV writer logic and introduce repository interfaces (with mockery tags).
