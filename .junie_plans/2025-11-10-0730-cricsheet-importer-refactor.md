# Plan ID 1.4.2 — Extract `cricsheet-importer` from cmd into internal with testable seams

Active path: 1 → 4 → 4.2
Parent: 1 → 4  
Branch: feat/cricsheet-importer-phase1

---

## Goal
Refactor the `cmd/cricsheet-importer` command so that `cmd/` becomes a thin delegator (flags + wiring only), with all logic moved into internal packages behind interfaces that can be mocked via mockery. Achieve ≥85% coverage for the new internal packages using table‑driven tests (no `if` in test bodies; use assert helpers).

This follows the same extraction pattern used for `export-dataset` and maintains identical user-visible behavior.

---

## Scope
- Introduce CLI options parser under `internal/cli/cricsheetimporter`.
- Introduce a `Runner` under `internal/commands/cricsheetimporter` to orchestrate ingestion steps.
- Extract core logic into `internal/services/cricsheetimporter` depending on minimal interfaces:
  - `cricsheet.Loader` (loads YAML/JSON scorecards from a directory or stream)
  - `cricsheet.Parser` (parses into domain `Match` or batch of `Match`)
  - `db.MatchRepo` (persists or upserts matches and associated records)
  - `logging.Logger` (optional; interface already exists)
- Add mockery tags on interfaces and generate mocks to use in unit tests.
- Thin `cmd/cricsheet-importer/main.go` to ≤ 30 LOC (besides imports/comments).
- Preserve all flags and behavior (directory defaults via config, etc.).

Out of scope (later phases): Performance tuning, new features, or behavior changes.

---

## Files to Create/Modify
1) Create: `go-app/internal/cli/cricsheetimporter/options.go`
   - `type Options struct { InDir string; Apply bool; Concurrency int }` (adjust after reviewing current flags)
   - `func ParseArgs(fs *flag.FlagSet, args []string) (Options, error)`
   - Defaults: `InDir` from `config.DefaultCricsheetDir()` when not provided; `Concurrency` sensible default (e.g., 4 or 8).

2) Create: `go-app/internal/cli/cricsheetimporter/options_test.go`
   - Table‑driven tests covering flags precedence and defaults (no `if` in bodies; use helpers).

3) Create: `go-app/internal/commands/cricsheetimporter/runner.go`
   - `type Runner struct { FS fsx.FS; Loader cricsheet.Loader; Parser cricsheet.Parser; Repo db.MatchRepo; Log logging.Logger }`
   - `func (r *Runner) Run(ctx context.Context, opts cli.Options) error`
   - Responsibility: iterate files, load, parse, hand off to Repo; no direct I/O besides FS dependency.

4) Create: `go-app/internal/commands/cricsheetimporter/runner_test.go`
   - Table‑driven tests using mocks for `Loader`, `Parser`, `Repo`, and a memory FS stub; assert orchestration and error paths.

5) Create: `go-app/internal/services/cricsheetimporter/ingest.go`
   - Small service(s) encapsulating file iteration and per‑file processing, called by Runner. Keep functions small and pure where possible.

6) Create: `go-app/internal/services/cricsheetimporter/ingest_test.go`
   - Table‑driven tests using mocks; deterministic fixtures under `go-app/tests/fixtures/` if needed (tiny YAML/JSON samples).

7) Modify: `go-app/internal/cricsheet` (interfaces + mockery tags)
   - Add/ensure interfaces:
     - `//go:generate mockery --name Loader --output internal/mocks --case underscore`
     - `//go:generate mockery --name Parser --output internal/mocks --case underscore`
   - If concrete types already exist, keep them; only extract interfaces + tags.

8) Modify: `go-app/internal/db` (repo interface with mockery tag)
   - Add/ensure minimal `MatchRepo` (if not present) with required methods: `UpsertMatch(ctx, m Match) error`, etc.
   - `//go:generate mockery --name MatchRepo --output internal/mocks --case underscore`

9) Modify: `go-app/cmd/cricsheet-importer/main.go`
   - Replace in‑file logic with: parse opts → build deps (FS, Loader, Parser, Repo, Logger) → `runner.Run(ctx, opts)`.
   - Keep file ≤ 30 LOC besides imports/comments.

10) Ensure: `go-app/.mockery.yaml` (or root `.mockery.yaml`) supports `internal/mocks` output; Makefile `mocks` target already exists.

---

## Tests to Add/Update
- `internal/cli/cricsheetimporter/options_test.go`: flags + defaults.
- `internal/commands/cricsheetimporter/runner_test.go`: orchestration happy/negative paths.
- `internal/services/cricsheetimporter/ingest_test.go`: per‑file processing and error handling.
- All tests must be table‑driven with per‑case assert helper functions; no `if` in test bodies.

---

## Acceptance Criteria
- `cmd/cricsheet-importer/main.go` reduced to thin wiring (≤ 30 LOC excluding imports/comments).
- New internal packages (`cli/cricsheetimporter`, `commands/cricsheetimporter`, `services/cricsheetimporter`) achieve ≥85% coverage each (services target ≥90%).
- Interfaces include mockery tags; `make mocks` succeeds and generates mocks under `internal/mocks`.
- Behavior preserved: same flags, defaults (config/environment), and side effects when `-apply` is used.
- `make test`, `make coverage`, and `make coverage-func` succeed.

---

## Verification Commands
```bash
cd go-app
make mocks
make test
make coverage && make coverage-func
# Focused coverage checks
go test -cover ./internal/commands/cricsheetimporter -coverprofile=/tmp/cri_cmd.out && go tool cover -func=/tmp/cri_cmd.out | tail -1
go test -cover ./internal/services/cricsheetimporter -coverprofile=/tmp/cri_svc.out && go tool cover -func=/tmp/cri_svc.out | tail -1
```

---

## Steps
1. Create CLI options and tests under `internal/cli/cricsheetimporter`. *
2. Add interfaces with mockery tags in `internal/cricsheet` and `internal/db` (`MatchRepo` if missing). *
3. Implement `Runner` and table‑driven tests in `internal/commands/cricsheetimporter`. 
4. Implement ingestion service(s) and tests in `internal/services/cricsheetimporter`. 
5. Thin `cmd/cricsheet-importer/main.go` to delegate entirely to the internal runner. 
6. Run `make mocks`, `make test`, and coverage commands; ensure per‑package thresholds (services ≥90%, commands ≥85%). 
7. Open a focused PR on `feat/cricsheet-importer-phase1` with Conventional Commits and reconcile status back to Plan ID 1.

Progress markers: * = in progress, ✓ = complete, ! = failed

---

## Risks & Mitigations
- Risk: Hidden coupling to concrete types in `internal/cricsheet` or DB layer.
  - Mitigation: Define minimal interfaces close to consumers; add thin adapters if needed.
- Risk: Behavior drift during extraction.
  - Mitigation: Preserve flags/defaults; keep smoke tests/manual verification; ensure table‑driven tests cover core flows.
- Risk: Coverage dips due to new packages.
  - Mitigation: Add negative‑path tests; keep functions small and pure where possible.

---

## Conventional Commits (expected)
- `feat(cricsheetimporter): add cli options and parser`
- `feat(cricsheetimporter): introduce runner and services with testable interfaces`
- `test(cricsheetimporter): table-driven tests for cli, runner, and services`
- `refactor(cmd/cricsheet-importer): thin to delegate to internal runner`
