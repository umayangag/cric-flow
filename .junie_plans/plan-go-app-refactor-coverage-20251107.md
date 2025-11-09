### Plan: Go-App — Finalize Refactor & Stabilize ≥90% Coverage (Scoped Gate)

#### Context
- Previous phases completed: coverage infra (Makefile), CI gate at 90% (scoped), cricsheet ingest refactor to adapters with offline tests, cmd/team-select orchestration tests, predictor/config unit tests.
- `src/` is read-only — no changes there.
- Tests must remain offline/deterministic.

---

### Objectives
1) Keep the CI coverage gate at ≥90% for the scoped internal packages and land small stabilizing tests (no behavior changes).
2) Harden `internal/cricsheet` edge cases and keep ingest adapter usage intact (no direct `db.*` calls in `ingest.go`).
3) Document the testing/coverage workflow and the adapter seams used for offline tests.

### Scope & Non-Goals
- Scope: tests, tiny refactors for seams/readability, docs. Behavior-preserving only.
- Non-Goals: broad `internal/db` repository tests, runtime/CLI changes, integration tests.

---

### Phases & Deliverables

#### Phase 1 — Re-baseline & Verify (no code changes)
- Actions:
  - Run fmt/vet and current scoped coverage locally.
  - Confirm CI uses `COV_MIN=90` and default `COVERAGE_PACKAGES` scope.
- Acceptance:
  - `make -C go-app fmt-check && make -C go-app vet` pass.
  - `make -C go-app coverage && COV_MIN=90 make -C go-app coverage-check` pass.
- Verification:
  - `make -C go-app fmt-check`
  - `make -C go-app vet`
  - `make -C go-app coverage && make -C go-app coverage-check`

#### Phase 2 — Cricsheet Tests Hardening (offline, adapter-only)
- Files:
  - `go-app/internal/cricsheet/ingest_integration_test.go` (extend if new edges found)
  - `go-app/internal/cricsheet/format*_test.go` and helper tests already present (add small edge cases if needed)
- High-level changes:
  - Cover edges: unknown `match_type` → error; `balls_per_over<=0` fallback to 6 is preserved.
  - Re-verify adapter-only usage in `ingest.go` (no direct `db.*` CRUD calls).
- Acceptance:
  - Package `internal/cricsheet` ≥85% coverage (currently ≈91%).
  - Tests deterministic; no real I/O.
- Verification:
  - `make -C go-app coverage COVERAGE_PACKAGES=./internal/cricsheet`
  - `COV_MIN=85 make -C go-app coverage-check COVERAGE_PACKAGES=./internal/cricsheet`

#### Phase 3 — Maintain Predictor/Config/CMD Coverage (stability)
- Files:
  - `go-app/internal/predictor/predictor_test.go`
  - `go-app/internal/config/*_test.go`
  - `go-app/cmd/team-select/*_test.go`
- Changes:
  - Keep tests green; add minor assertions only if flakiness observed.
- Acceptance:
  - Packages remain ≥90% where they currently stand.
- Verification:
  - `make -C go-app test`
  - `make -C go-app coverage && COV_MIN=90 make -C go-app coverage-check`

#### Phase 4 — Documentation
- Files:
  - `go-app/README.md` or `docs/TESTING.md`
- Changes:
  - Document: adapter pattern in `internal/cricsheet` (DB + Weather interfaces), coverage commands (`coverage`, `coverage-html`, `coverage-func`, `coverage-check`), how to tweak `COVERAGE_PACKAGES` and `COV_MIN` locally, and the scoped CI policy.
- Acceptance:
  - Devs can reproduce local coverage and understand scope/gate policy.
- Verification:
  - `make -C go-app fmt-check`

---

### Files to Create/Modify
- Modify tests (as needed, small):
  - `go-app/internal/cricsheet/*_test.go`
  - `go-app/cmd/team-select/*_test.go`
  - `go-app/internal/predictor/*_test.go`
  - `go-app/internal/config/*_test.go`
- Modify docs:
  - `go-app/README.md` (or add `docs/TESTING.md`)

### Tests to Add/Update (Examples)
- `internal/cricsheet`: one targeted test each for:
  - `ImportMatchFile` error on unknown `match_type`.
  - `oversFromBalls` with pathological `bpo` values (<=0) to assert fallback.
- Others: Only if gaps discovered; keep tests tiny and deterministic.

### Acceptance Criteria (Global)
- Scoped coverage (default `COVERAGE_PACKAGES`) ≥90% locally and in CI.
- `make -C go-app fmt-check` and `make -C go-app vet` pass.
- No runtime/CLI behavior changes; tests are offline/deterministic.
- Documentation updated explaining coverage scope/gate and adapter seams.

### Verification Commands (Global)
- `make -C go-app fmt-check`
- `make -C go-app vet`
- `make -C go-app coverage`
- `COV_MIN=90 make -C go-app coverage-check`
- Optional (package focus):
  - `make -C go-app coverage COVERAGE_PACKAGES=./internal/cricsheet && COV_MIN=85 make -C go-app coverage-check COVERAGE_PACKAGES=./internal/cricsheet`

### Branching & PR Workflow
- Feature branches only (never commit to `main`/`master`). Suggested branches:
  - `test/cricsheet-harden`
  - `docs/coverage-workflow`
- Conventional commits; small, focused PRs (1–3 files + tests). Ensure CI green before merge.

### Risks & Mitigations
- Risk: accidental direct `db.*` usage reintroduced in `ingest.go`.
  - Mitigation: review imports; rely on adapter interfaces; keep integration-style tests using fakes.
- Risk: coverage drift below 90% due to scope creep.
  - Mitigation: keep scope limited, use Makefile variables to verify focused packages.
