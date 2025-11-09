# Plan ID 1.4.1.3 — Export‑dataset Phase 3: Move Format Resolution into internal/commands

Active path: 1 → 4 → 4.1 → 4.1.3
Parent: 1 → 4 → 4.1
Branch: feat/exportdataset-phase3

## Goal
Extract the format resolution logic from `cmd/export-dataset/main.go` into `internal/commands/exportdataset`, with comprehensive, table‑driven unit tests and no `if` statements in test bodies. Preserve behavior exactly while increasing test coverage and continuing to thin the `cmd/` package.

## Scope
- Introduce a pure function `ResolveFormats(opts cli.Options, cfg *config.Config) []string` (or `([]string, error)` if validation needed) inside `internal/commands/exportdataset`.
- Mirror current behavior:
  - Precedence from CLI: `--all-formats` > `--formats` (CSV) > `--format` (single).
  - If CLI provides none, fall back to config: if `cfg.Export.RequiredFormat` set → single; else if `cfg.Export.SplitByFormat` → all four; else legacy combined (return `[]string{""}` sentinel used downstream by current logic).
- Add table‑driven tests for all cases, including whitespace, case, and dedup behavior. No `if` in test bodies; use small assert helpers.
- Wire `cmd/export-dataset/main.go` to call `ResolveFormats` instead of duplicating logic.
- Keep heavy export/DB logic unchanged for now.

## Files to Create/Modify
1) Create: `go-app/internal/commands/exportdataset/formats.go`
   - Implement `ResolveFormats(opts cli.Options, cfg *config.Config) []string`.
   - Keep pure and deterministic; no I/O, no logging.

2) Create: `go-app/internal/commands/exportdataset/formats_test.go`
   - Table‑driven tests covering:
     - `--all-formats`
     - `--formats` CSV with trimming, case‑insensitivity, deduplication
     - `--format` single
     - Empty CLI → config fallbacks (`RequiredFormat`, `SplitByFormat`, legacy combined)
     - Edge cases: spaces, empty tokens in CSV, unknown formats (kept as provided by CLI without validation to preserve legacy)
   - Use per‑case assert functions; no `if` in test bodies.
   - Target ≥90% coverage for this file.

3) Modify: `go-app/cmd/export-dataset/main.go`
   - Replace in‑file logic that builds `list` of formats with a call to `exportdataset.ResolveFormats(opts, config.Load())`.
   - Remove now‑redundant local parsing for `formats/format/allFormats` variables if still present.
   - Preserve behavior and downstream use of the resolved list.

## Tests to Add/Update
- `internal/commands/exportdataset/formats_test.go` (new) — table‑driven, assert helpers only.
- Ensure existing tests remain green: `cli/exportdataset`, `commands/exportdataset/runner`, `adapters/fsx/osfs`.

## Acceptance Criteria
- Format resolution is implemented in `internal/commands/exportdataset` and fully covered by tests (≥90% for new file).
- `cmd/export-dataset/main.go` delegates format list computation to `ResolveFormats` and contains less bespoke logic.
- All tests pass via `make test`.
- Behavior is preserved for all existing flag/config combinations; smoke run still works.

## Verification Commands
```bash
cd go-app
make test
make coverage && make coverage-func
# Optional: check only the commands/exportdataset package coverage
go test -cover ./internal/commands/exportdataset -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out | tail -1
# Smoke run (no DB actions verified here):
make export-dataset OUT=../output/go-app
```

## Conventional Commits (Phase 3)
- `feat(exportdataset): add ResolveFormats and table-driven tests`
- `refactor(cmd/export-dataset): delegate format resolution to internal command`

## Risks & Mitigations
- Risk: Subtle behavior drift versus legacy.
  - Mitigation: Encode the full legacy precedence in tests; wire `cmd/` to use resolver; keep return `[]string{""}` for legacy combined behavior.
- Risk: Validation rejecting unknown formats could break existing use.
  - Mitigation: Do not validate format names in this phase; treat inputs as opaque strings (uppercase + trim only), matching legacy.

## Next Steps (Breadcrumbs)
- After this phase, proceed to extract CSV writer logic and DB seams:
  - 1 → 4 → 4.1 → 4.1.4: Introduce repository interfaces (with mockery tags) and move writer functions into services with unit tests.
