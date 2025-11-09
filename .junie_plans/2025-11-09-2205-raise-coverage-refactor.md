# Plan ID 1 — Raise Go Coverage > 80% by Extracting `cmd/` Logic and Introducing Testable Interfaces

Active path: 1
Parent: —

This plan targets the `go-app` module. We will refactor heavy logic out of `cmd/` into `internal/`, introduce clear interfaces (with mockery tags), and add table-driven tests with per-testcase assert functions. We will proceed incrementally with small PRs, enforcing TDD and documentation updates.

---

## 1. Baseline and Branching (Parent: 1)
- Create feature branch: `feat/raise-coverage-refactor`.
- Record baseline coverage:
  - `cd go-app && go test ./... -coverprofile=coverage.out`
  - `go tool cover -func=coverage.out | tee coverage.txt`
- Ensure Makefile targets exist: `test`, `coverage`, `coverhtml`, `mocks`, `lint`, `fmt`.

Deliverables:
- Baseline coverage numbers under `go-app/coverage.txt` (optional).
- Feature branch created.

Acceptance:
- Branch created; baseline captured.

Verification commands:
- `cd go-app && go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`

Status: planned

---

## 2. Target Architecture (Parent: 1)
Define layering and dependencies:
- `cmd/<tool>`: thin bootstrap (flags + call into internal).
- `internal/cli/<tool>`: option structs and pure parsing helpers.
- `internal/commands/<tool>`: use-case orchestration (`Runner.Run(ctx, opts)`).
- `internal/services/...`: reusable business logic (pure when possible).
- `internal/adapters/...`: concrete implementations for DB, HTTP/ML, FS, clock, random.
- `internal/domain/...`: entities and pure utilities.
- `internal/logging`: singleton logger and `Logger` interface.
- `internal/config`: env + flags merge/validation.

Dependency direction: `cmd -> cli -> commands -> services -> adapters`; `domain/logging/config` shared inward; nothing imports `cmd`.

Deliverables:
- ADR-style notes (optional) in README about layering.

Status: planned

---

## 3. Interfaces and Mockery (Parent: 1)
Introduce interfaces at seams (samples):
- `internal/logging`:
  - `type Logger interface { Debugf(ctx context.Context, fmt string, args ...any); Infof(...); Warnf(...); Errorf(...); With(key string, val any) Logger }`
  - `//go:generate mockery --name Logger --output internal/mocks --case underscore`
- `internal/adapters/clock`: `type Clock interface { Now() time.Time }` + go:generate.
- `internal/adapters/random`: `type Random interface { Intn(n int) int; Seed(seed int64) }` + go:generate.
- `internal/adapters/fsx`: `type FS interface { ReadFile(ctx context.Context, path string) ([]byte, error); WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error; MkdirAll(path string, perm fs.FileMode) error; Glob(pattern string) ([]string, error) }` + go:generate.
- `internal/adapters/httpx`: `type HTTPClient interface { Do(req *http.Request) (*http.Response, error) }` + go:generate.
- Domain-specific:
  - `internal/mlclient`: `type Client interface { PredictTeam(ctx context.Context, in PredictRequest) (PredictResponse, error); Reload(ctx context.Context) error }` + go:generate.
  - `internal/db`: `MatchRepo`, `WeatherRepo`, etc., with go:generate.
  - `internal/cricsheet`: `Loader`, `Parser` with go:generate.

Makefile:
- Add `mocks` target to run `go generate ./...`.
- Document `go install github.com/vektra/mockery/v2@latest`.

Acceptance:
- Interfaces exist with `//go:generate mockery` tags.
- `make mocks` generates mocks into `internal/mocks`.

Verification commands:
- `cd go-app && make mocks`

Status: planned

---

## 4. Incremental Extraction per Command (Parent: 1)
For each command X:
- Create `internal/cli/X` with `Options` and `Parse(args []string) (Options, error)`.
- Create `internal/commands/X` with `type Runner struct{ deps }` and `Run(ctx, opts) error`.
- Make `cmd/X/main.go` a minimal delegator (≤ 30 LOC besides imports/comments).
- Write unit tests for `cli` and `commands` with mocks. Tests are table-driven and use per-testcase assert functions; no if-statements in test bodies.

### 4.1 export-dataset (Parent: 1 → 4)
- Add:
  - `internal/cli/exportdataset/options.go`
  - `internal/commands/exportdataset/runner.go`
  - Tests: `internal/cli/exportdataset/options_test.go`, `internal/commands/exportdataset/runner_test.go`
- Likely dependencies: `FS`, `MatchRepo`, `Logger`.
- Thin `cmd/export-dataset/main.go`.

Acceptance:
- `cmd/export-dataset/main.go` ≤ 30 LOC; `internal/commands/exportdataset` ≥ 85% coverage.

Verification:
- `go test ./...` and check per-package coverage.

Status: planned

### 4.2 cricsheet-importer
- Introduce `Loader`, `Parser`, `MatchRepo`, `Logger` seams.
- Extract into `internal/cli/cricsheetimporter` and `internal/commands/cricsheetimporter`.

Acceptance: thin `cmd`, ≥85% coverage for `commands/cricsheetimporter`.

Status: planned

### 4.3 backfill-fielding
- Extract similarly; depends on `MatchRepo`, possibly `FS`, `Logger`.

Status: planned

### 4.x Other commands
- `etl-importer`, `weather-import`, `weather-worker`, `team-predictor`, `team-select`, `evaluate`, `tools/migrate` — repeat the pattern introducing needed interfaces.

Status: planned

---

## 5. Logging and Configuration (Parent: 1)
- `internal/logging`: singleton logger + `Logger` interface; log success and error paths.
- `internal/config`: env + flags merge and validation with tests.

Acceptance:
- Log lines added in success paths; config validation covered by table tests.

Verification:
- `go test ./...`

Status: planned

---

## 6. Adapters Hardening (Parent: 1)
- Ensure concrete adapters conform to interfaces or add wrappers:
  - `internal/adapters/db/postgres`: `MatchRepo`, `WeatherRepo`, etc.
  - `internal/adapters/mlclient/http`: implements `mlclient.Client` using `HTTPClient`.
  - `internal/adapters/fsx/osfs`: implements `FS` using stdlib.
  - `internal/adapters/httpx/std`: wraps `*http.Client`.
  - `internal/adapters/clock/system`, `internal/adapters/random/mathrand`.
- Add adapter-focused unit tests with `httptest` where applicable.

Acceptance:
- Adapters compile against interfaces; core request-building logic tested.

Status: planned

---

## 7. Testing Conventions and Tooling (Parent: 1)
- Table-driven tests only; no if-statements in test bodies. Use tiny assert helpers.
- Deterministic seeds; fixtures under `go-app/tests/fixtures/`.
- Makefile targets to add/ensure:
  - `test`: `go test ./...`
  - `coverage`: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`
  - `coverhtml`: `go tool cover -html=coverage.out -o coverage.html`
  - `mocks`: `go generate ./...`
  - Optional `coverage-gate`: fail if total < 80%.

Acceptance:
- `make coverage` shows ≥80% at plan end; mocks generated.

Status: planned

---

## 8. CI and Linting (Parent: 1)
- GitHub Actions to run `make fmt`, `make lint`, `make test`, `make coverage` on PRs.
- Upload coverage artifact.

Acceptance:
- CI green on PRs; coverage visible.

Status: planned

---

## 9. Documentation (Parent: 1)
- Update `go-app/README.md` with structure, commands, running tests, coverage, mocks.
- Add `.env.example` for any env vars.

Acceptance:
- Docs accurately reflect new layout and workflow.

Status: planned

---

## 10. Execution Strategy & PR Cadence (Parent: 1)
- PR 1: Extract `export-dataset` + tests + mocks.
- PR 2: Extract `cricsheet-importer` + tests.
- PR 3..N: Extract remaining commands (1–2 per PR) with tests.
- Separate PRs for config/logging and adapters hardening.

Acceptance:
- Small, reviewable PRs; each passes tests and raises/maintains coverage.

Status: planned

---

## 11. Global Acceptance Criteria
- Global `go-app` coverage ≥ 80% (`go tool cover -func=coverage.out`).
- Every `cmd/<tool>/main.go` reduced to thin bootstrap (≤ 30 LOC besides imports/comments).
- All external effects accessed only via interfaces with `//go:generate mockery` tags; mocks generated and used in tests.
- All new tests are table-driven with per-testcase assert functions; no `if` in test bodies.
- `make test`, `make coverage`, and `make lint` succeed locally and in CI.
- `go-app/README.md` and `.env.example` updated when behavior/config changes.

Verification commands:
- `cd go-app && make mocks && make fmt && make lint && make test && make coverage && go tool cover -func=coverage.out | tail -n 1`
- Manual check of `cmd/*/main.go` line counts (< 30 LOC target).

---

## Notes & Risk Mitigation
- Extract command-by-command to minimize risk and avoid large diffs.
- Maintain behavior while improving structure; keep flags/outputs stable.
- Use `context.Context` consistently.
- Keep functions small/single-responsibility.
- Use small deterministic fixtures; no network or real DB in unit tests.

---

## Sub-plan Hierarchy & Breadcrumbs
- 1 (root)
  - 2 (Architecture)
  - 3 (Interfaces + Mockery)
  - 4 (Per-command extractions)
    - 4.1 export-dataset (Parent: 1 → 4)
    - 4.2 cricsheet-importer (Parent: 1 → 4)
    - 4.3 backfill-fielding (Parent: 1 → 4)
    - 4.x remaining commands (Parent: 1 → 4)
  - 5 (Logging)
  - 6 (Adapters)
  - 7 (Testing tooling)
  - 8 (CI)
  - 9 (Docs)
  - 10 (Cadence)
  - 11 (Global acceptance)

We will mark each item’s status as we execute (✓ complete, * in progress, ! failed) and reconcile upward to the parent after each sub-plan completes, per the Anti‑Drift rule.
