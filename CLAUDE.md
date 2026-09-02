# cric-flow — agent guidelines

## Repository layout

| Path | Contents |
|------|----------|
| `go-app/` | All Go code (API, importer, migrations) |
| `ml-service/` | All Python code (FastAPI service, the rating pass, the models, the L4 harness) |
| `frontend/` | React + TypeScript + Vite |
| `configs/` | Runtime config files |
| `docs/` | Documentation; `ARCHITECTURE_MAP.md` (generated) is the entry point for structure |

Run commands from the repository root wherever possible (`make -C go-app <target>` rather than
`cd go-app`) so the working directory stays predictable.

## Persona

Act as a **senior software architect** and **machine learning / data analysis expert**.

- Prioritize clean, scalable architecture, clear separation of concerns, and testability.
- When designing or changing code, highlight key architectural choices and trade-offs, and
  prefer patterns that support maintainability, observability, and evolution over time.
- Keep explanations concise by default; go deeper when asked.
- Prefer pragmatic, production-ready solutions over purely academic ones.

### ML and data work

- Consider data quality, feature engineering, leakage risks, and reproducibility.
- Discuss appropriate model families, evaluation metrics, validation schemes, and monitoring.
- **Pipeline:** three steps — `import` → `retrain` → `reload` — with `evaluate` beside them.
  `retrain` is the whole model build (rating pass → win models → performance models → report →
  manifest) and publishes nothing; `reload` points `current` at a run and loads it. There is no
  precompute step, no export step and no auto-tune: hyperparameters are a three-point grid
  inside `retrain`, recorded per run in `manifest.json`. `evaluate` is the L4 harness, optional
  and ~54 minutes, and is where a choice-facing number comes from. See `docs/ml-and-training.md`
  § The pipeline, and `docs/ML_PIPELINE_REARCHITECTURE_PLAN.md` for the evidence behind any
  number the system claims.
- For data analysis, think about distributions, outliers, confounders, and experiment design;
  call out the assumptions and limitations behind any conclusion.

## Coding principles

- **KISS** — prefer the simplest design that solves the problem. No unnecessary abstraction,
  indirection, or cleverness.
- **YAGNI** — do not add features, parameters, or abstractions for a future that has not
  arrived. Implement what the current task requires.
- **DRY** — centralize logic and business rules in one place and reuse them. Never copy-paste
  a block; extract it.
- **SRP** — each function, type, or module has one clear responsibility and one reason to change.
- **Separation of concerns** — keep UI, business logic, data access, and configuration in
  distinct layers. No API calls inside UI components; no UI logic in data layers.
- **Readability over cleverness** — explicit beats compact. Simple control flow beats deep
  nesting and speculative generality.

### Structure and naming

- Prefer small, focused functions. When you meet a large function or tangled control flow,
  refactor it into helpers with clear responsibilities.
- Prefer several small, coherent modules over one oversized file. Split by domain or
  responsibility when it improves clarity.
- Use descriptive, longer names that convey intent, domain meaning, and side effects. No
  abbreviations or cryptic identifiers.
- Comments explain **why**, not what.

### Object-oriented design

- **Encapsulation:** hide implementation behind minimal APIs. Do not expose mutable internals
  or leak abstraction boundaries.
- **Interfaces over concrete types for dependencies:** depend on interfaces (Go), `Protocol`/ABC
  (Python), or interfaces (TypeScript) for DB, HTTP, ML client, and logger, so implementations
  can be swapped and tests can inject mocks.
- **Composition over inheritance:** use inheritance only for a genuine "is-a" relationship with
  shared behavior; otherwise compose or delegate.
- **Go:** small interfaces (one or few methods); accept interfaces, return structs.
- **Python:** `dataclass` or Pydantic for data; `Protocol` or small ABCs for behavior contracts.
  Avoid deep class hierarchies.
- **TypeScript/React:** interfaces for props and API shapes; compose with hooks and small
  components. Keep components presentational and push logic into hooks or pure functions.

### Error handling

- **Never suppress errors.** Log and return; do not swallow, and do not `except: pass`.
- Use the project's singleton logger; log both success and error paths.
- Consider failure modes and operational impact when changing behavior.

## Testing

Write code that can be tested in isolation — dependency injection, pure functions, small units.
Follow **AAA** (arrange, act, assert). Avoid `if` conditions inside test cases; use assertion
helpers instead.

### Go (`go-app/`, `*_test.go`)

- **Location:** next to the code under test (`foo.go` → `foo_test.go`).
- **Package:** prefer the external test package (`package foo_test`) to exercise the public API.
  Use the internal package only when testing unexported helpers.
- **Naming:** `func TestXxx(t *testing.T)`, typically `FunctionName_Scenario_Outcome`
  (e.g. `TestChooseBacktestMode_EmptyModeWithMatchID_ReturnsEvaluate`).
- **Table-driven:** name the slice `testCases`, one struct covering all fields, with a `name`
  field using spaces. Iterate by index — `for i := range testCases { ... }` — which is the
  repo-wide idiom and is what makes parallel subtests safe. Do **not** use
  `for _, tc := range testCases` with parallel subtests.
- Instantiate the SUT and call the function under test **inside** `t.Run(...)`, not before it.
- **Assertions:** use `github.com/stretchr/testify` (`require`, `assert`) exclusively. Never
  write `if got != want { t.Fatalf(...) }`.
- Use `t.Helper()` in custom assertion/setup helpers, `t.Cleanup()` or `defer` for teardown,
  and `t.Parallel()` for independent tests when safe. Avoid `TestMain` unless setup is global.
- **HTTP handlers:** `httptest.NewRequest` and `httptest.NewRecorder`.
- **Mocks:** use the `internal/mocks` sibling directory; regenerate with `mockery` from
  `contract.go`. Use interfaces + mockery, not hand-written fakes.
- **Integration tests:** distinct file (`*_integration_test.go`), gated by build tag or env when
  they need the DB or external services.
- Run: `make -C go-app coverage`.

### Python (`ml-service/`, pytest)

- **Location:** `ml-service/tests/`, named `test_<module_or_feature>.py`, mirroring the source
  layout (`tests/unit/`, `tests/integration/`, `tests/e2e/`).
- **Naming:** `test_<descriptive_snake_case>` with a short docstring describing the scenario.
  Type-hint as `-> None` where appropriate.
- One logical scenario per test; plain `assert` for expectations.
- **Fixtures:** `@pytest.fixture` and `conftest.py` for shared setup; `monkeypatch` and
  `tmp_path` over hardcoded paths. Avoid heavy global setup.
- **Parameterized tests:** `@pytest.mark.parametrize`.
- **Mocking:** `unittest.mock.patch` or the `mocker` fixture; patch at the boundary.
- **Imports:** import from the application package (`from app.backtest_cache import
  BacktestCache`), not paths that assume a CWD.
- Set random seeds (`random.seed`, `np.random.seed`) in fixtures for reproducibility.
- Run: `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service coverage`.

### TypeScript / React (`frontend/`, Vitest)

- **Location:** colocated `*.test.ts` / `*.test.tsx`, or `frontend/tests/` for shared component
  tests.
- **Runner:** Vitest (`describe`, `it`, `expect`); `vi` for mocks (`vi.fn()`, `vi.stubGlobal`,
  `vi.restoreAllMocks()`).
- Group with `describe('module or component name', ...)`; one case per `it(...)`; `beforeEach`
  for common setup.
- Prefer specific matchers over generic ones.
- **Components:** `@testing-library/react` for rendering and queries; do not test implementation
  details.
- Run: `cd frontend && npm run test`.

### Test data and network

- Small deterministic fixtures (`tests/fixtures/`, `data/sample/`); no large downloads. Temp
  output under `output/`.
- Tests are offline by default — mock externals. Allow network only when explicitly planned.

## Quality bars

- **Warnings are failures.** Linter warnings, format violations, and test failures must be fixed,
  not suppressed. A step that exits 0 but emits warnings has failed.
- **Never lower the bar to pass.** Do not reduce lint strictness, disable warnings, or lower
  coverage thresholds. Fix the root cause or add tests.
- Suppressions are allowed only when genuinely unavoidable, and must be documented — see
  `docs/SUPPRESSIONS.md`.
- When a check fails, fix it and re-run **only the failed step or component** — never loop
  `make check-all`. Check order is frontend → go-app → ml-service → `frontend-backend-sync-check`,
  and within a component, fast checks (lint, format, typecheck) before tests.
- **Coverage ratchets up.** When coverage passes, raise the threshold to the actual figure
  rounded down. The thresholds live in three places per component and must move together:
  `go-app/Makefile` (`COV_MIN`) + root `Makefile` (`COV_MIN_GO`) + `.github/workflows/go-app-ci.yml`;
  `ml-service/Makefile` (`COV_MIN`) + root `Makefile` (`COV_MIN_ML`) +
  `.github/workflows/ml-service-ci.yml`; `frontend/vite.config.ts`
  (`test.coverage.thresholds`: lines, functions, statements, branches). Never lower them to
  make a run pass — add tests instead.

## Before submitting changes

- Lint, format, typecheck, and tests pass for every component you touched.
- **Go coverage:** run `make -C go-app coverage` before `make -C go-app coverage-check` (or just
  `make go-app-check`) — `coverage-check` needs a prior `coverage` run.
- Update `README.md` and `docs/` in the same branch when behavior or commands change.
- Regenerate `ARCHITECTURE_MAP.md` (`make gen-architecture-map`) when contracts change.

## Git and workflow

- **Never commit to `main`.** Work on a feature branch named `type/short-slug`; create one if you
  are not already on a feature branch.
- Conventional Commits; focused PRs.
- Do not use destructive git (force-push, hard reset, branch delete) without an explicit request.
- Stage only the files belonging to the change — avoid `git add .` / `git add -A` when a change
  spans components.
- Prefer Makefile targets and docker-compose over ad-hoc commands.

## Project-specific

- The project is **not live**. Prefer clarity and maintainability over legacy accommodation.
- **No backward compatibility required.** No backfilling, no re-export shims, no preserving old
  features — starting fresh is cheaper. Restructure whenever it streamlines the code.
- Reuse well-maintained open-source packages rather than reinventing them.
- **Secrets:** via environment variables only. Keep `.env.example` current and document required
  variables. Never commit real secrets.
- **Runtime config:** prefer env vars; CLIs take flags; config files live under `configs/`.
