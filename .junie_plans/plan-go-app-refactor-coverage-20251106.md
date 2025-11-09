### Plan: Go-App Full Refactor Under Test Coverage + CI Coverage Gate ≥ 90%

#### Context
- Guidelines resynced (see `.junie/guidelines.md`).
- Prime directive: readable, maintainable code with strong tests; proceed in small, behavior-preserving phases.
- `src/` directory is read-only and serves as a reference only.
- All unit tests must be offline/deterministic (use fakes and `httptest` where needed; no real network/DB/file I/O).

---

### Objectives
1) Fully refactor `go-app` for clarity and testability without changing runtime behavior or CLI contracts.
2) Achieve and enforce ≥ 90% unit test coverage for `go-app` locally and in CI.
3) Add GitHub Actions workflows that gate PRs on formatting, vetting, and coverage threshold.

### Out of Scope
- New features or breaking changes.
- Integration tests (Docker Compose) unless explicitly approved in a later phase.
- Any modifications under `src/`.

---

### Phases & Deliverables

#### Phase 1 — Coverage Infrastructure (Makefile + Docs) [Behavior-Preserving]
- Files to modify:
  - `go-app/Makefile`
    - Add targets:
      - `coverage`: `go test -covermode=atomic -coverprofile=coverage.out ./...`
      - `coverage-html`: `go tool cover -html=coverage.out -o coverage.html`
      - `coverage-func`: `go tool cover -func=coverage.out | tail -1`
      - `coverage-check`: parse total from `coverage.out` and fail if < 90.0%
  - `go-app/README.md`
    - Document the new coverage commands and the ≥ 90% policy.
- Tests to add/update: none (infra only).
- Acceptance criteria:
  - `make -C go-app coverage` generates `coverage.out`.
  - `make -C go-app coverage-check` exits 0 when ≥ 90%, non-zero otherwise.
  - `make -C go-app fmt-check` and `make -C go-app vet` pass.
- Verification commands:
  - `make -C go-app fmt-check`
  - `make -C go-app vet`
  - `make -C go-app coverage`
  - `make -C go-app coverage-check`
- Branch/PR:
  - Branch: `feat/go-coverage-infra`
  - Commit: `feat(make): add coverage targets and docs; gate at >=90%`

#### Phase 2 — CI: GitHub Actions with Coverage Gate ≥ 90% [Behavior-Preserving]
- Files to add:
  - `.github/workflows/go-ci.yml`
    - Steps: checkout, setup-go, cache modules, `make -C go-app fmt-check`, `make -C go-app vet`, `make -C go-app coverage`, `make -C go-app coverage-check`, upload `coverage.out` artifact.
    - Optional: run `golangci-lint` if config present.
- Tests to add/update: none (CI only).
- Acceptance criteria:
  - CI runs on `pull_request` and `push` to `main`.
  - Workflow fails if coverage < 90% or fmt/vet fail.
- Verification commands:
  - Push a branch and open PR to observe checks.
- Branch/PR:
  - Branch: `ci/go-actions-coverage-gate`
  - Commit: `ci: add go workflow with coverage gate >=90%`

#### Phase 3 — Unit Tests Round 1 (Predictor/Config/CMD) [Offline/Deterministic]
- Files to modify/create (tests only unless tiny seams needed):
  - `go-app/internal/predictor/predictor_test.go`
    - Add cases: team size > player count; float/rounding stability; ensure extras and totals consistent.
  - `go-app/cmd/team-predictor/*`
    - Ensure `buildTeam` orchestration tests using fake predictor (happy + error path); keep existing `selectTop` and CSV parse tests.
  - `go-app/cmd/team-select/*`
    - Add orchestration seam (pure function) if needed; tests using DB fake covering defaults and error cases.
  - `go-app/internal/config/config_test.go`
    - Extend validator/default dir tests for additional negative/zero edges and cache resets.
- Acceptance criteria:
  - Coverage meaningfully increases; no behavior changes.
  - All tests pass offline.
- Verification commands:
  - `make -C go-app test`
  - `make -C go-app coverage && make -C go-app coverage-check`
- Branch/PR:
  - Branch: `test/go-raise-coverage-1`
  - Commit: `test(go-app): expand predictor/config/cmd tests; offline and deterministic`

#### Phase 4 — Unit Tests Round 2 (Cricsheet/MLClient/DB) [Offline/Deterministic]
- Files to modify/create:
  - `go-app/internal/cricsheet/*`
    - Add tests for pure helpers (e.g., `format.go`) using in-memory fixtures; avoid real file I/O.
  - `go-app/internal/mlclient/*`
    - Use `httptest.Server` to test request/response paths for `PredictWin`, `PredictBatting`, `PredictBowling` (no external network).
  - `go-app/internal/db/*`
    - Isolate DSN construction/getenv into a tiny pure helper (if needed) and add tests; avoid opening real connections.
- Acceptance criteria:
  - Overall `go-app` coverage ≥ 90% locally.
  - Tests remain offline and deterministic.
- Verification commands:
  - `make -C go-app test`
  - `make -C go-app coverage && make -C go-app coverage-check`
- Branch/PR:
  - Branch: `test/go-raise-coverage-2`
  - Commit: `test(go-app): add cricsheet/mlclient/db tests; offline via fakes and httptest`

#### Phase 5 — Small Refactors for Readability/Test Seams (Behavior-Preserving)
- Files to modify: limited, only where needed to introduce tiny pure helpers or interfaces to improve clarity and test seams.
- Tests to add/update: adjust/add unit tests aligned to refactors; no behavior changes.
- Acceptance criteria:
  - No CLI contract changes; tests stay green; coverage ≥ 90%.
- Verification commands:
  - `make -C go-app fmt-check && make -C go-app vet && make -C go-app coverage-check`
- Branch/PR:
  - Branch: `refactor/go-seams-readability`
  - Commit: `refactor(go-app): extract tiny pure helpers for readability/test seams (no behavior change)`

#### Phase 6 — Docs & Badge (Optional)
- Files to modify:
  - `go-app/README.md` and/or `docs/TESTING.md`: document offline testing strategy, fakes, and CI coverage gate; add coverage commands.
  - Optionally add a coverage badge (documented/manual for now).
- Acceptance criteria:
  - Documentation clearly explains how to run coverage locally and the CI policy.
- Verification commands:
  - `make -C go-app fmt-check`
- Branch/PR:
  - Branch: `docs/go-coverage-badge`
  - Commit: `docs(go-app): document coverage workflow and add badge notes`

---

### Acceptance Criteria (Global)
- Overall `go-app` unit test coverage ≥ 90% locally and in CI (per `go tool cover -func coverage.out` total).
- All unit tests are offline/deterministic (no real network/DB/file I/O; use fakes and `httptest`).
- `make -C go-app fmt-check` and `make -C go-app vet` pass.
- No changes under `src/`; runtime behavior preserved; CLIs maintain existing flags and outputs.

### Verification Commands (Global)
- `make -C go-app fmt-check`
- `make -C go-app vet`
- `make -C go-app coverage`
- `make -C go-app coverage-check`

### Risk Management
- Coverage parsing portability: implement `coverage-check` using POSIX `awk`/`grep`.
- Avoid flaky tests: use fakes and `httptest.Server`; seed data is in-memory and small.
- DB coupling: test only pure helpers; do not open real connections.

### Branching & PR Workflow
- Never commit to `main`/`master`.
- Create small, focused feature branches per phase.
- Use Conventional Commits and open PRs early for review.
