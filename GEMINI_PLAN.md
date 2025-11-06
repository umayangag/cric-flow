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
