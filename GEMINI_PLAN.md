### GEMINI PLAN — Phase A–C: Refactor go-app for Readability + Add Unit Tests (TDD)

Scope
- Improve go-app readability and testability without changing observable behavior.
- Introduce table-driven unit tests starting with `internal/predictor` and selected `cmd` flag parsing.
- Keep offline, deterministic tests; consult `src/` as reference only.

Subproject
- go-app (Go 1.25+)

Out of Scope
- New features or behavior changes.
- Integration tests or network/db calls during unit tests.

Files to Modify/Create
- Modify: `go-app/internal/predictor/predictor.go` (introduce a pure function variant that accepts `config.Config`).
- Create: `go-app/internal/predictor/predictor_test.go` (table-driven tests).
- Create (if needed later in Phase C): small helpers for flag parsing tests under `go-app/cmd/team-predictor` (no behavior changes).
- Docs: `go-app/README.md` minor clarifications on commands/flags.

Design Overview
- Extract pure logic from `CalculateOverallPerformance` into `CalculateOverallPerformanceWithConfig(cfg *config.Config, players []PlayerPrediction, matchID int64) Team`.
- Keep existing `CalculateOverallPerformance` as a thin wrapper calling `config.Load()` and delegating to the new function (backward-compatible, behavior-preserving).
- Add deterministic tests by constructing a minimal `config.Config` inline in tests, avoiding env/files.

Phases
- Phase A — Tests first (predictor)
  1) Write failing table-driven tests for `CalculateOverallPerformanceWithConfig` (spec to be added with function stub in Phase B).
  2) Cover: correct aggregation of runs/balls/conceded/wickets; application of `TeamSize` and `DefaultExtras`.
  3) Edge cases: empty players slice (expect zeroed totals and extras only), single player.

- Phase B — Minimal refactor (predictor)
  1) Implement `CalculateOverallPerformanceWithConfig` and adapt existing `CalculateOverallPerformance` to call it with `config.Load()`.
  2) Ensure no change to exported structs/APIs beyond adding the new function.
  3) Add comments explaining the why (testability seam).

- Phase C — Broaden tests and light cleanup
  1) Add tests for wrapper `CalculateOverallPerformance` using a temporary override if necessary (or keep to WithConfig tests only).
  2) Light readability cleanup in `cmd/team-predictor/main.go`: extract flag parsing and orchestration into small helpers (no functional change). Add small table test for flag parsing if feasible without IO.
  3) Update `go-app/README.md` to clarify flags and make targets.

Acceptance Criteria
- New unit tests for predictor are present, table-driven, and deterministic.
- Predictor logic has a pure function accepting `config.Config` to enable unit testing; wrapper preserved.
- go-app passes `make test`, `make fmt-check`, and `make vet`; `golangci-lint` clean if configured.
- No observable behavior changes in CLI tools.
- README updated to document clarified usage.

Verification Commands
- `make -C go-app test`
- `make -C go-app fmt-check`
- `make -C go-app vet`
- Optional: `make -C go-app lint` (if golangci-lint is installed)

Branching & PR
- Branch: `refactor/go-app-readability-tests`
- Conventional commits, small iterative commits per phase.

Risk Management
- If config coupling blocks tests, keep tests on the pure function only.
- If cmd flag parsing proves hard to test without IO, limit Phase C to internal helpers and document follow-up.

Notes
- `src/` is read-only; use it only as behavioral reference. Maintain parity or improvements where applicable.


---

### Phases G–K: Continued Refactor & Test Coverage (behavior-preserving)

Scope
- Improve readability and testability in remaining go-app components (focus: cmd/team-select, config helpers, offline seams for mlclient/db).
- Add deterministic, offline unit tests. No behavior changes. `src/` remains read-only.

Phase G — cmd/team-select readability + testable seams (DONE)
- Files:
  - Create: `go-app/cmd/team-select/flags.go` (pure `parseFlags(args []string, cfg *config.Config) (options, error)`)
  - Create: `go-app/cmd/team-select/main_test.go` (table tests for success/defaults/errors)
  - Modify: `go-app/cmd/team-select/main.go` to use `parseFlags`; preserve behavior
- Acceptance:
  - `make -C go-app test` green; no observable behavior changes
- Status: Completed during this session

Phase H — Config validation/utilities (pure helpers + tests)
- Files:
  - Modify: `go-app/internal/config/config.go` (add small pure validators, e.g., `ValidateTeamSettings(cfg *Config) error`)
  - Create: `go-app/internal/config/config_test.go` (table-driven tests for default utils and validators)
- Acceptance:
  - Helpers are pure (no I/O); Load() behavior unchanged; tests green
- Verify:
  - `make -C go-app test && make -C go-app fmt-check && make -C go-app vet`
- Branch:
  - `refactor/config-validation-helpers`

Phase I — Interfaces and fakes for mlclient and db (unit-test friendly)
- Files:
  - Modify: `go-app/internal/mlclient` to expose `Predictor` interface; concrete client implements it
  - Create: `go-app/internal/mlclient/fake/fake_client.go` (deterministic stub)
  - Modify (if applicable): `go-app/internal/db` to add a minimal interface and a `db/fake/fake_db.go`
  - Update: `cmd/team-predictor` and `cmd/team-select` to depend on interfaces; `main` wires concretes
- Acceptance:
  - Unit tests run offline with fakes; no CLI behavior change
- Verify:
  - `make -C go-app test && make -C go-app fmt-check && make -C go-app vet`
- Branch:
  - `refactor/mlclient-db-interfaces-and-fakes`

Phase J — Extend unit tests for predictor/selector edge cases
- Files:
  - Update: `go-app/internal/predictor/predictor_test.go` (large TeamSize vs fewer players; rounding stability)
  - Update: `go-app/cmd/team-predictor/selector_test.go` (equal probability tie-breakers)
- Acceptance:
  - Coverage increases; behavior preserved
- Verify:
  - `make -C go-app test`
- Branch:
  - `test/predictor-and-selector-edgecases`

Phase K — Docs & Makefile hygiene (readability only)
- Files:
  - Update: `go-app/README.md` (document team-select flags/examples; testing with fakes)
  - Update (non-functional): `go-app/Makefile` (help target discoverability)
- Acceptance:
  - Docs reflect current commands/flags and testing approach; no behavior change
- Verify:
  - `make -C go-app fmt-check && make -C go-app vet && make -C go-app test`
- Branch:
  - `docs/go-app-cli-and-testing-notes`

Global Acceptance Criteria
- Unit tests are offline and deterministic
- `make -C go-app test`, `fmt-check`, and `vet` pass
- No changes under `src/`

Global Verification Commands
- `make -C go-app test`
- `make -C go-app fmt-check`
- `make -C go-app vet`
- Optional: `make -C go-app lint || true`
