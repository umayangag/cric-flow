# Technical Debt Remediation Plan for `cric-flow`

## 1. Purpose and Scope

This document is a **playbook for an AI coding agent** acting as a senior system architect on the `cric-flow` codebase. It describes **what** to improve, **why**, and **how**, with a focus on:

- **Go backend** (`go-app`)
- **Python ML service** (`ml-service`)
- **Frontend** (`frontend`)
- **Cross‑cutting contracts and configuration**

The plan assumes the agent can:

- Run existing `make`/npm workflows and tests
- Open PRs and iterate with CI
- Make incremental, backwards‑compatible changes

---

## 2. Global Architecture and Constraints

### 2.1. High‑Level System View

- **Go backend (`go-app`)**
  - Owns the primary API, DB schema, migrations, and feature/dataset export logic.
  - Orchestrates long‑running pipelines, backtests, and interactions with the ML service.
- **ML service (`ml-service`)**
  - FastAPI app serving predictions, backtest behavior, training, and auto‑tune.
  - Manages model artifacts and metadata.
- **Frontend (`frontend`)**
  - React/TypeScript console for operations, backtest control, and ML observability.
  - Talks mostly to the Go API; some concepts mirror ML service metadata internally.
- **Shared domain**
  - Feature vectors (`configs/feature_vectors.json` and related code in both Go and Python).
  - Training/export pipelines that cross all three components.

The system is **already well‑engineered** with solid tooling and tests. The primary debts are **complexity hotspots and coupling**, not outright “bad code”.

### 2.2. Non‑Negotiable Guardrails

The AI agent **must** respect these constraints:

- **Do not break public or inter‑service contracts**
  - HTTP paths, JSON shapes, and status codes must remain stable unless:
    - A backward‑compatible extension is added, and
    - All known callers (Go services, frontend) and tests are updated accordingly.
- **Always keep CI workflows green**
  - Any meaningful change must pass:
    - Relevant component tests (`make -C go-app test`, `make -C ml-service test`, `npm test` in `frontend`)
    - Lint/format/type checks
    - Eventually, the entire `make check-all` pipeline (preferably via the `run-check-all-incremental` skill).
- **Prefer small, cohesive PRs**
  - Avoid sprawling refactors that simultaneously change Go, Python, and frontend behavior.
  - Each PR should:
    - Have a clear theme (e.g., “Extract backtest services from Go handlers”).
    - Include tests that characterize/lock in existing behavior.

---

## 3. Global Strategy for Reducing Technical Debt

### 3.1. Priority Order

1. **Isolate and simplify complexity hotspots**
   - `ml-service/app/main.py`
   - `go-app/internal/server` backtest & pipeline handlers
   - Large frontend tab components
2. **Clarify and centralize cross‑cutting concerns**
   - Tracking and pipeline lifecycle
   - Model modes (legacy, per‑format, unified, shared)
   - Feature vector schema and its consumers
3. **Harden configuration and observability**
   - Shared configuration semantics between Go and Python
   - Observability surfaces and logging conventions

### 3.2. Workflow Template for Each Refactor

For any specific debt area, the agent should:

1. **Discover**
   - Locate relevant files and tests.
   - Map call graphs and data flow (handlers → services → DB/ML).
2. **Characterize behavior**
   - Identify existing tests.
   - For untested but critical paths, add targeted tests that capture current behavior before refactoring.
3. **Refactor without semantic change**
   - Introduce new modules/abstractions.
   - Move logic behind interfaces.
   - Keep inputs/outputs and API behavior unchanged.
4. **Test and validate**
   - Run the smallest relevant test subset.
   - Then use `run-check-all-incremental` for broader assurance if changes are non‑trivial.
5. **Document the new boundaries**
   - Brief comments/docstrings for new modules/interfaces.
   - Clear PR descriptions explaining:
     - What was extracted.
     - Why complexity decreased.
     - How behavior is guaranteed unchanged.

---

## 4. Go Backend (`go-app`) Remediation Plan

### 4.1. Decompose Backtest and Pipeline HTTP Handlers

**Problem**

- `internal/server/backtest_handlers.go`, `backtest_services.go`, `pipeline_handlers.go`, and `ops_status_*` are orchestration “hubs” that:
  - Mix HTTP concerns with domain logic.
  - Depend on tracking, jobs, DB, and ML client details.
- This increases coupling and makes isolated testing harder.

**Objectives**

- Achieve **thin handlers**, **thick services**.
- Make backtest and pipeline flows understandable via well‑named service interfaces.

**Actions**

1. **Introduce dedicated service packages (if not already present or too thin)**
   - `internal/services/backtest`
   - `internal/services/pipeline`
   - Responsibilities:
     - Pure orchestration of domain operations.
     - Business rules independent of HTTP.

2. **Refactor handlers to delegate to services**
   - For each handler group:
     - Identify its core orchestration logic.
     - Extract into service methods such as:
       - `BacktestService.SelectCandidates(...)`
       - `BacktestService.EvaluateRun(...)`
       - `PipelineService.StartRun(...)`
       - `PipelineService.StopRun(...)`
       - `PipelineService.StreamStatus(...)`
     - Ensure services receive standard domain types (not `http.Request` or framework contexts).

3. **Strengthen tests**
   - Add/extend unit tests for:
     - Service methods (pure logic).
     - Handlers (request/response shape, error mapping, auth behavior as needed).
   - Keep existing tests passing to prove behavior is unchanged.

**Success criteria**

- Handlers are mostly request/response translation and error mapping.
- Backtest and pipeline behavior is testable without HTTP plumbing.
- No API contract or tracking semantics changed.

### 4.2. Clarify Tracking and Ops Status Lifecycle

**Problem**

- Pipeline and job tracking spans:
  - `internal/tracking/*`
  - `internal/jobs/*`
  - `internal/server/ops_status_*`
  - Startup behavior in `cmd/api/main.go`
- Understanding “what happens when a pipeline runs” is non‑trivial.

**Objectives**

- Have a **single conceptual model** of pipeline runs, states, and transitions.
- Make ops status endpoints thin wrappers around this model.

**Actions**

1. **Model lifecycle explicitly**
   - In `internal/tracking`, define core types and state transitions, e.g.:
     - `RunID`, `RunState`, `RunType`, `RunEvent`.
     - Functions like `StartRun`, `MarkStepComplete`, `MarkFailed`, `ListActiveRuns`.
   - Add a focused doc (e.g. `internal/tracking/doc.go`) describing:
     - States and transitions.
     - Where runs are created, updated, and surfaced.

2. **Refactor ops status**
   - Adjust `ops_status_*` to:
     - Query tracking abstractions instead of directly stitching DB/join logic where possible.
     - Transform tracking data into stable JSON contracts consumed by the frontend.

3. **Encapsulate startup reconciliation**
   - Move “cancel stale runs on startup” behavior from `cmd/api/main.go` into a well‑named function such as:
     - `tracking.ReconcileStaleRuns(ctx, db, logger)`
   - Optionally gate this with config/env to decouple deployment flows.

**Success criteria**

- Adding a new pipeline type or step mostly touches tracking, not arbitrary parts of server code.
- Ops Status behavior is easier to understand and modify without guessing about embedded state machines.

### 4.3. Reduce Duplication in DB and Export Queries

**Problem**

- `internal/db/repo_*.go` and `internal/db/exportqueries/*.go` encode overlapping logic.
- Schema changes risk drift between in‑app queries and dataset exports.

**Objectives**

- Centralize reusable query fragments and business rules.
- Maintain consistent semantics between:
  - Interactive API behavior.
  - Offline exports feeding ML training.

**Actions**

1. **Identify overlaps**
   - Scan for repeated patterns (joins, filters, groupings) across repos and export queries.
2. **Abstract shared building blocks**
   - Introduce reusable helpers in a shared module, e.g.:
     - `internal/db/query_fragments.go`
     - Or narrowly scoped helpers alongside each repo/domain.
   - Focus on:
     - Common `WHERE` clauses.
     - Common joins that encode important semantics.

3. **Add validation tests**
   - For at least one representative domain (e.g. batting):
     - Tests asserting that a controlled fixture yields:
       - The same row counts and key metrics via repo and export query.

**Success criteria**

- Key business semantics live in a single place per domain.
- Adding a new metric or constraint in DB logic does not require manual updates in multiple files without tests catching drift.

### 4.4. Make Migrations and Startup Behavior More Flexible

**Problem**

- The API process currently couples:
  - DB connection.
  - Migrations.
  - Tracking reconciliation.
- Failed migrations can block the API, making deployment strategies less flexible.

**Objectives**

- Allow operations teams to choose:
  - “Run migrations in CI / predeploy job” vs “run migrations at startup”.
- Reduce coupling between infra concerns and serving logic.

**Actions**

1. **Extract orchestration functions**
   - Encapsulate:
     - `RunMigrations(ctx, db, logger)`
     - `ReconcileStaleRuns(ctx, db, logger)`
   - From `cmd/api/main.go` into dedicated internal packages.

2. **Expose configuration toggles**
   - If not already present, introduce env/config flags controlling:
     - Whether to auto‑run migrations on startup.
     - Whether to run tracking reconciliation on startup.

3. **Tests**
   - Add small tests for startup orchestration functions checking:
     - Behavior when migrations fail.
     - Correct ordering and logging.

**Success criteria**

- Teams can move toward “migrations in CI / dedicated job” without rewriting app logic.
- Startup path is simpler and easier to read.

---

## 5. ML Service (`ml-service`) Remediation Plan

### 5.1. Modularize `app/main.py`

**Problem**

- `app/main.py` is a single, large file responsible for:
  - FastAPI app creation and middleware
  - Prediction and backtest endpoints
  - Caching and train‑on‑the‑fly behavior
  - Training and auto‑tune orchestration
  - Artifact loading and model metadata endpoints

**Objectives**

- Move to a **package‑oriented FastAPI design**:
  - `main.py` becomes a wiring layer.
  - Logic lives in cohesive modules.

**Actions**

1. **Identify responsibility clusters**
   Group functions and inner helpers into conceptual domains:
   - Core prediction (single‑shot predictions, feature transforms).
   - Backtest and train‑on‑the‑fly.
   - Admin training and auto‑tune.
   - Artifact loading and model metadata.
   - Health and status endpoints.

2. **Introduce domain modules**
   - Create modules such as:
     - `app/prediction.py`
     - `app/backtest.py`
     - `app/training_orchestrator.py`
     - `app/cache.py`
     - `app/status_endpoints.py`
   - Move:
     - Inner helper functions.
     - Non‑HTTP logic.
     - Cache management logic.
   - Keep FastAPI route definitions in `main.py` initially, delegating to these modules.

3. **Optional later step: route modules**
   - Once logic is stable in domain modules, define `APIRouter`s in:
     - `app/api/prediction.py`
     - `app/api/backtest.py`
     - `app/api/admin.py`
   - Assemble routers in `main.py`.

4. **Testing discipline**
   - Before moving a block, locate all tests that cover it.
   - After moving:
     - Ensure all tests pass.
     - Add new tests where newly created pure functions have clear inputs/outputs.

**Success criteria**

- `app/main.py` shrinks to a small “composition” file.
- Each area (prediction, backtest, training) has its own module with cohesive responsibilities.
- New contributors can locate relevant logic quickly.

### 5.2. Centralize Configuration and Global State

**Problem**

- Behavior is strongly controlled by env vars scattered across `main.py` and `settings.py`.
- Caches and registries often live as module‑level globals.

**Objectives**

- Promote **explicit, testable configuration**.
- Minimize hidden coupling via globals.

**Actions**

1. **Single configuration object**
   - In `app/settings.py` (or similar), define a configuration model (e.g. Pydantic) capturing:
     - Flags (train‑on‑the‑fly, backtest cache, hot reload).
     - Limits (max batch sizes, timeouts, concurrency).
     - Paths and URLs (artifacts directories, Go app URL).
   - Load this config exactly once per process and pass it into modules needing it.

2. **Encapsulate caches/registries**
   - Wrap global dicts / caches in classes with explicit:
     - Initialization.
     - Reset/clear operations (useful for tests).
   - Expose factory functions (e.g. `build_prediction_cache(config)`).

3. **Tests**
   - Add tests for:
     - Default configuration.
     - Behavior under specific env combinations.
   - Use dependency injection / fixtures to provide config and caches in tests.

**Success criteria**

- No new code reads env vars directly from service logic.
- Tests can instantiate “minimal config” scenarios without global state leaks.

### 5.3. Make Backtest Caching and Train‑on‑the‑Fly Semantics Explicit

**Problem**

- Caching and train‑on‑the‑fly combine:
  - Cutoff rounding.
  - Player IDs and formats.
  - Multiple artifact modes and edge conditions.
- The behavior is correct but hard to reason about.

**Objectives**

- Turn the caching strategy into a **first‑class concept** with explicit types and tests.

**Actions**

1. **Document modes and policies**
   - In a `backtest.py` module docstring, describe:
     - How cache keys are constructed.
     - When train‑on‑the‑fly is allowed.
     - How innings/fielding reconciliation interacts with caching.

2. **Introduce a `BacktestCache` abstraction**
   - Provide methods such as:
     - `get_prediction_for_cutoff(request, settings)`
     - `invalidate(...)` as needed.
   - Internally handle:
     - Key construction.
     - Cache policy (max size, TTL or cutoff granularity).

3. **Strengthen tests**
   - Add unit tests around this abstraction to verify:
     - Hits vs misses for identical inputs.
     - Distinct keys for different cutoffs or modes.
     - Behavior when cache is disabled via config.

**Success criteria**

- Caching logic is localized and well‑tested.
- New cache policy changes do not require navigating multiple unrelated functions.

### 5.4. Decouple Training Orchestration from Request Handling

**Problem**

- `/admin/train/*` and `/admin/train/auto-tune` start training subprocesses directly from request handlers, mixing:
  - User‑facing API behavior.
  - Process and resource management.

**Objectives**

- Encapsulate training orchestration so it can evolve independently.
- Keep APIs unchanged while reducing coupling.

**Actions**

1. **Extract orchestration into `app/training_orchestrator.py`**
   - Provide functions like:
     - `start_training_job(params, settings)`
     - `start_auto_tune_job(params, settings)`
     - `get_training_status(job_id)`
   - Handle:
     - Subprocess invocation.
     - Concurrency limits and semaphores.
     - Error reporting and logging.

2. **Route handlers call orchestrator only**
   - Handlers validate input and call orchestrator functions.
   - Map orchestrator results/errors to HTTP responses without embedding process logic.

3. **Tests**
   - Introduce tests for orchestrator behavior including:
     - Happy path.
     - Subprocess failure.
     - Hitting concurrency limits.

**Success criteria**

- Changing underlying training scripts/subprocess behavior mostly touches `training_orchestrator`, not the rest of the app.
- Admin APIs remain stable and well‑tested.

---

## 6. Frontend (`frontend`) Remediation Plan

### 6.1. Split Large Tab Components into Containers and Presentational Pieces

**Problem**

- `EvaluateDbTab`, `OpsStatusTab`, `WorkbenchTab`, `MLModelStatsTab` combine:
  - Data fetching.
  - Polling and side effects.
  - State persistence (e.g. `localStorage`).
  - Complex MUI layouts.

**Objectives**

- Move towards:
  - **Container components** (data + side effects).
  - **Presentational components** (layout + rendering).

**Actions**

1. **For each large tab, perform a “responsibility triage”**
   - Identify:
     - Data fetching and polling logic.
     - Long‑running job management (evaluation runs, pipelines).
     - Persistence (e.g. job IDs in `localStorage`).
     - UI composition.

2. **Extract custom hooks**
   - Example for `EvaluateDbTab`:
     - `useEvaluateOptions()` for formats/teams/opponents.
     - `useEvaluateJob()` for:
       - Starting an evaluation.
       - Persisting job state.
       - Polling status and handling completion.
   - Hooks should be:
     - Well‑typed.
     - Focused on logic with minimal UI awareness.

3. **Extract subcomponents**
   - Break UI into:
     - Filters panel.
     - Progress/status panel.
     - Results table.
   - Each subcomponent receives props and is unaware of side effects.

4. **Maintain and extend tests**
   - Update component tests to use:
     - New container + child components.
   - Optionally add hook tests for complex logic (where feasible).

**Success criteria**

- Each tab file shrinks noticeably.
- Logical responsibilities are easier to identify.
- Test coverage is at least as strong as before.

### 6.2. Normalize Side‑Effect Patterns and Remove ESLint Disables

**Problem**

- `useEffect` usage includes:
  - Manual cleanup flags.
  - ESLint disables for dependency arrays.
- This hints at complexity and risk of subtle bugs.

**Objectives**

- Use composable, reusable side‑effect patterns.
- Rely on lint rules for correctness rather than suppressing them.

**Actions**

1. **Identify recurring side‑effect patterns**
   - Polling (e.g., Ops Status, evaluation status).
   - Fire‑and‑forget fetches for options.
   - Long‑running job monitoring.

2. **Abstract into hooks**
   - Introduce a generic `usePolling` hook or similar pattern that:
     - Accepts a callback and interval.
     - Handles mount/unmount behavior correctly.
   - Replace ad‑hoc intervals with this hook.

3. **Revisit `useEffect` dependencies**
   - Rewrite effects to:
     - Depend on stable values.
     - Use memoized callbacks where necessary.
   - Remove ESLint disables once dependencies are accurate.

4. **Tests**
   - Ensure tests cover:
     - Start/stop behavior on mount/unmount.
     - Behavior when key inputs change.

**Success criteria**

- No new ESLint disables for React hook rules.
- Polling and side effect code is centralized and reusable.

### 6.3. Reduce Drift Between Frontend and Backend Model Metadata

**Problem**

- Frontend `DEFAULT_MODEL_FEATURES` in `WorkbenchTab` mirrors backend `model_metadata.py`.
- Manual sync is error‑prone.

**Objectives**

- Align on a **single source of truth** for model metadata.

**Actions**

1. **Use backend metadata as canonical source**
   - Ensure there is an endpoint (or extend an existing one) that returns schema‑ed model metadata.
   - On the frontend:
     - Fetch metadata on load.
     - Use it to configure Workbench displays.

2. **Retain `DEFAULT_MODEL_FEATURES` as a pure fallback**
   - Use only when metadata endpoint is unavailable.
   - Document this behavior clearly in `WorkbenchTab`.

3. **Type alignment**
   - Define a `ModelMetadata` TypeScript interface.
   - Mirror its structure in a Pydantic model on the backend.
   - Keep them in lock‑step as part of API evolution.

4. **Tests**
   - Add tests ensuring:
     - Workbench uses backend metadata when available.
     - Fallback works if metadata endpoint fails.

**Success criteria**

- Normal operation uses backend metadata only.
- Fallbacks are explicitly documented and rarely triggered.

---

## 7. Cross‑Cutting Concerns

### 7.1. Model Modes (Legacy, Per‑Format, Unified, Shared)

**Problem**

- Multiple parallel notions of model modes are implemented:
  - Legacy vs per‑format.
  - Unified vs componentized.
  - Shared models.
- These modes exist across Go, ML service, and frontend.

**Objectives**

- Centralize mode definitions and semantics.
- Provide a stable, versioned abstraction.

**Actions**

1. **Define a “model mode registry”**
   - In `ml-service/app/model_metadata.py` (or a new module), define a structure listing:
     - Each mode’s name, availability, and deprecation status.
   - Use this registry in:
     - Prediction logic.
     - Model stats endpoints.
     - Metadata endpoints used by the frontend.

2. **Surface mode information to frontend**
   - Ensure Ops/Workbench UIs receive:
     - Mode names.
     - Deprecation flags.
     - Human‑readable descriptions (e.g., “legacy unified model for ODI”).

3. **Plan deprecation paths**
   - For truly legacy modes:
     - Mark them as deprecated in metadata.
     - Eventually remove support only after:
       - Frontend is updated to ignore/hide them.
       - Ops workflows no longer depend on them.

**Success criteria**

- Adding/removing a mode happens by editing the registry and updating tests, not by implicitly adding new branches in multiple places.

### 7.2. Configuration Semantics

**Problem**

- Config precedence and flags are rich but implemented separately in Go and Python.

**Objectives**

- Provide a **single mental model** for configuration across the system.

**Actions**

1. **Central documentation**
   - Create or expand a config doc (e.g., `docs/configuration.md`) summarizing:
     - Precedence: CLI > env > config file > defaults.
     - Key env vars and their roles across:
       - DB and storage.
       - Pipeline behavior.
       - ML behavior (train‑on‑the‑fly, caches).
   - Ensure both Go and Python conform to the documented model.

2. **Converge env var naming where practical**
   - For similar concepts, use consistent names and semantics.
   - If renaming, support old names as deprecated aliases for a transition period.

3. **Automated checks**
   - Add small tests verifying:
     - Expected precedence behavior.
     - Proper defaulting when config files are missing.

**Success criteria**

- Developers and operators can predict system behavior from env/config with minimal cross‑referencing code.

### 7.3. Feature Vector Schema as a First‑Class Contract

**Problem**

- Feature vectors are central but enforced via convention across:
  - `configs/feature_vectors.json`
  - Go exporters
  - ML training scripts
  - Frontend displays

**Objectives**

- Treat the feature vector schema as a **versioned contract**.

**Actions**

1. **Define a versioned schema**
   - Consider including a `version` field in `feature_vectors.json`.
   - Document expected backward compatibility properties.

2. **Add validation tooling**
   - Implement a small check (Go or Python) that:
     - Validates:
       - No unknown features used in dataset exporters.
       - Required features are correctly produced.
     - Can be run from `make check-all`.

3. **Update training and UI paths to read schema**
   - Where possible, derive:
     - Training feature lists.
     - Workbench displays.
   - Directly or indirectly from `feature_vectors.json` rather than manual duplication.

**Success criteria**

- Adding/removing features must:
  - Update `feature_vectors.json`.
  - Pass validation for all consumers.
  - Be caught by CI if any consumer drifts.

### 7.4. Observability Map

**Problem**

- Observability is strong but distributed: Go ops status, ML model stats, frontend visualizations, logs.

**Objectives**

- Provide a clear **map of observability entry points**.

**Actions**

1. **Create `docs/observability.md`**
   - Summarize:
     - Backends:
       - Go: `/ops/status`, DB insights.
       - ML service: `/health`, `/artifacts/status`, `/model-stats`.
     - Frontend:
       - Tabs and components that surface these signals.
   - Describe:
     - Common log fields and conventions.
     - How to interpret pipeline states and artifacts statuses.

2. **Harmonize logging fields**
   - Align on core keys:
     - `pipeline_id`, `run_id`, `format`, `artifact_type`, etc.
   - Ensure new logs align with this convention.

**Success criteria**

- New engineers (or agents) can read a single doc and know where to look for system health and performance information.

---

## 8. Execution Roadmap for the AI Agent

### 8.1. Suggested Sequence of Refactors

1. **ML service**
   - Modularize `app/main.py` and centralize config.
   - Extract training orchestrator and backtest cache.
2. **Go backend**
   - Extract backtest and pipeline services from server handlers.
   - Clarify tracking/ops status lifecycle and startup reconciliation.
   - Tidy DB/export query duplication where most risky.
3. **Frontend**
   - Break down large tab components with hooks and subcomponents.
   - Normalize polling/side‑effect patterns and align metadata usage with backend.
4. **Cross‑cutting**
   - Introduce model mode registry and feature schema validation.
   - Add/extend config and observability docs.

### 8.2. Per‑PR Checklist for the Agent

For each PR, the agent should:

- **Define scope**: One clearly articulated technical debt slice.
- **Ensure tests exist**:
  - Add missing tests before or with refactor.
- **Refactor**:
  - Move logic to new modules/interfaces.
  - Preserve contracts and semantics.
- **Run checks**:
  - Component tests and lint/type/format checks.
  - Incremental `check-all` when changes are non‑local.
- **Explain**:
  - Summarize the refactor in the PR description.
  - Explicitly state “no behavior change expected” vs “intentional behavior change” and why.

---