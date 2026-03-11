# Audit: Alignment with project coding principles and remediation plan

**Date:** 2025-03-11  
**Scope:** Full codebase (frontend, go-app, ml-service) vs. [.cursor/rules/coding-principles.mdc](../.cursor/rules/coding-principles.mdc) and [architect-ml-expert](../.cursor/rules/architect-ml-expert.mdc).

---

## 1. Executive summary

- **Automated checks:** Lint, format, typecheck, build, and tests **pass** for all three components when run per-component from repo root (frontend, go-app, ml-service). Go-app `coverage-check` passes when run from `go-app` (60.3% ≥ 60%); ensure it is always run after `make -C go-app coverage` so the correct profile is used.
- **Principles alignment:** The codebase is largely consistent with KISS, DRY, and separation of concerns. Main gaps are: **oversized files** (SRP/structural preference), **repeated fetch/loading/error patterns** in the frontend (DRY), **some API calls in components** (separation of concerns), **large bundle chunk** (guideline says prefer code-splitting over raising limit), and **scattered lint/test suppressions** (warnings-as-failure principle).
- **Remediation:** Prioritized plan below: fix check reliability first, then tackle large files and DRY in frontend, then ML/go-app structure and suppressions.

---

## 2. Check results (current state)

| Component   | Lint | Format | Typecheck | Build | Tests | Coverage / cov-gate |
|------------|------|--------|-----------|--------|-------|----------------------|
| Frontend   | ✅   | ✅     | ✅        | ✅     | ✅    | (Vitest coverage configured; thresholds in vite.config.ts) |
| Go-app     | ✅   | ✅     | ✅        | —      | ✅    | ✅ 60.3% (when run from go-app after `make coverage`) |
| ML-service | ✅   | ✅     | —         | —      | ✅    | ✅ 75% |

**Note:** Run `make -C go-app coverage` then `make -C go-app coverage-check` from repo root (or run both from `go-app`). Running `coverage-check` without a fresh `coverage` in `go-app` can use a stale or wrong profile and fail (e.g. 35.6% from an outdated total).

---

## 3. Alignment with coding principles

### 3.1 KISS / YAGNI

- **Finding:** Generally good. No obvious speculative or over-engineered features.
- **Gap:** Some modules are large enough that “simplest design” is harder to preserve (see 3.4).

### 3.2 DRY (Don’t Repeat Yourself)

- **Finding:** API layer is centralized (`frontend/src/api.ts`, `api/client.ts`). Backtest logic is delegated to `internal/services/backtest` from server handlers.
- **Gap (Frontend):** Repeated “fetch → setLoading/setError → then/catch” patterns across components (e.g. `MLModelStatsTab`, `OpsStatusTab`, `HealthTab`, `OpsSuggestions`, `PipelineStepDialog`, `OpsMigrationsTable`, `PipelineProgressPanel`). Hooks like `useEvaluateDb` and `useUpcomingMatch` encapsulate some of this; others call `api.*` and manage loading/error state inline. **Recommendation:** Introduce a small `useApiCall` or reuse a single “async data” hook to centralize loading/error handling and reduce duplication.

### 3.3 Single Responsibility Principle (SRP) and file size

- **Finding:** Many packages are focused (e.g. `config`, `eval`, `formats`, `logger`). Server and ML layers delegate to services.
- **Gap (Structural):** Several files are very large and mix multiple responsibilities or many variants of similar behavior:

| Area        | File / path | Lines (approx) | Concern |
|------------|-------------|-----------------|---------|
| Go-app     | `internal/server/backtest_handlers.go` | ~1123 | Many HTTP handlers + helpers in one file |
| Go-app     | `internal/server/ml_backtest_client.go` | ~870  | Client + orchestration |
| Go-app     | `internal/server/handlers.go`           | ~530  | General handlers |
| Frontend   | `hooks/useEvaluateDb.ts`                | ~479  | Large hook with many responsibilities |
| Frontend   | `api.ts`                                | ~455  | Many endpoints in one module |
| Frontend   | `components/EvaluateDbSection.tsx`     | ~434  | UI + state + flow |
| ML-service | `app/prediction_service.py`            | ~1295 | Core prediction + orchestration + transforms |
| ML-service | `app/main.py`                          | ~896  | Routes + wiring |
| ML-service | `ml/generalized_pipeline.py`           | ~641  | Pipeline + many branches |

**Recommendation:** Split by domain or responsibility (e.g. backtest handlers by subdomain, `useEvaluateDb` into smaller hooks, `prediction_service` into feature/orchestration modules, `main.py` into routers + thin app).

### 3.4 Separation of concerns

- **Finding:** Go-app keeps HTTP in `internal/server`, DB in `internal/db`, business logic in `internal/services/*`. ML-service keeps routes in `app`, core ML in `ml/`. Frontend has `api` and `hooks` for data.
- **Gap (Frontend):** Several components call `api.*` directly and own loading/error state (see 3.2). Prefer data-fetching in hooks and pass data/callbacks as props so components stay presentational where possible.
- **Gap (ML):** `prediction_service.py` mixes feature building, orchestration, and integration; some of this could live in `ml/` with the app layer only wiring and HTTP.

### 3.5 Readability and naming

- **Finding:** Names are generally descriptive (e.g. `BuildPredictedScorecard`, `chooseBacktestMode`, `useEvaluateDb`). No systematic terse or cryptic naming observed.
- **Gap:** A few very long files reduce local readability; splitting them will help.

### 3.6 Warnings and suppressions

- **Principle:** “Treat linter warnings, format violations, and test failures as must-fix. Do not ignore or suppress without a documented, unavoidable reason.”
- **Finding:** There are multiple suppressions across the repo:
  - **Frontend:** `eslint-disable` in at least `useEvaluateDb.ts`, `BacktestFilters.tsx`, and one test file.
  - **Go:** `//nolint:` in several test and source files (e.g. `deps.go`, `training_snapshot.go`, `win.go`, exportqueries, seqcalc tests).
  - **ML:** `# noqa`, `# type: ignore` in several files (e.g. `prediction_service.py`, `main.py`, `auto_tune.py`, `train_*.py`, tests).

**Recommendation:** For each suppression: (1) document why it is unavoidable, and (2) where possible, fix the underlying issue (e.g. types, test structure) and remove the suppression. Track in a short “suppressions” list in the repo or in this doc until resolved.

### 3.7 Frontend bundle size

- **Guideline (run-check-all-incremental):** Fix chunk-size warnings (e.g. “Some chunks are larger than 500 kB”) via code-splitting or `manualChunks`; use `chunkSizeWarningLimit` only if code-splitting is impractical.
- **Finding:** Main JS chunk is ~792 kB (gzip ~245 kB). `vite.config.ts` sets `chunkSizeWarningLimit: 800`, so the build does not warn but the principle prefers smaller chunks.
- **Recommendation:** Add route-based or feature-based code-splitting (e.g. lazy routes, `manualChunks` for heavy screens) to get the main chunk below 500 kB where practical; only then consider keeping or lowering the warning limit.

### 3.8 Test coverage and thresholds

- **Finding:** Go-app and ml-service coverage gates pass (60% and 75% respectively). Frontend has coverage thresholds in `vite.config.ts` (lines/functions/statements/branches). Per guidelines, do not lower thresholds; add tests to meet or exceed them, and when passing, raise thresholds to the current value (rounded down).
- **Gap:** Some go-app packages have low coverage (e.g. `db/connection`, `selection`, `services/pipeline`, `services/precomputefeatures`, `services/predictteam`). They are partially or fully excluded from the coverage set via `COVERAGE_EXCLUDE`. If they are ever included in the future, coverage will need to be added to avoid dropping the overall percentage.

---

## 4. Remediation plan (prioritized)

### P0 – Check reliability and docs

1. **Go-app coverage-check:** Document in the repo (e.g. root `Makefile` or `go-app/README`) that `coverage-check` must be run after `make -C go-app coverage` (or from within `go-app`). Ensure CI continues to run `make -C go-app coverage` then `make -C go-app coverage-check` from root so the same profile is used.
2. **Run-check-all-incremental:** No code change; the skill already says “from repo root” and per-step commands. Optionally add a one-line note in `docs/` or the coding-principles rule that go-app coverage-check is run from root after `make -C go-app coverage`.

### P1 – High-impact structural (SRP / file size)

3. **Go-app server:** Split `backtest_handlers.go` (and optionally `ml_backtest_client.go`) by subdomain or handler group into smaller files (e.g. `backtest_handlers_*.go` or by feature), keeping handlers thin and delegating to existing services.
4. **Frontend:** Split `useEvaluateDb.ts` into smaller hooks (e.g. candidates, scorecard, evaluate job, filters) and/or extract pure helpers to a separate module. Split or group `api.ts` by domain (e.g. backtest, ops, options) if it grows further.
5. **ML-service:** Split `prediction_service.py` into modules (e.g. feature building, orchestration, HTTP-facing helpers) and keep `app/main.py` to routing and wiring; consider moving more logic into `ml/` for testability.

### P2 – DRY and separation of concerns (frontend)

6. **Data-fetching pattern:** Introduce a small generic hook (e.g. `useApiCall<T>(fn)` or `useAsyncData`) that encapsulates loading/error state and optional refetch, and refactor `MLModelStatsTab`, `OpsStatusTab`, `HealthTab`, `OpsSuggestions`, `PipelineStepDialog`, `OpsMigrationsTable`, and similar components to use it (or existing hooks like `usePolling` where appropriate).
7. **Components vs API:** Where it simplifies testing and clarity, move remaining direct `api.*` calls from components into hooks and pass data and callbacks as props.

### P3 – Bundle size and suppressions

8. **Frontend bundle:** Implement code-splitting (lazy routes and/or `manualChunks`) to reduce the main chunk toward or below 500 kB; then set `chunkSizeWarningLimit` to 500 (or remove if default is acceptable).
9. **Suppressions:** Audit all `eslint-disable`, `//nolint`, `# noqa`, and `# type: ignore`; document unavoidable ones (e.g. in this doc or a `SUPPRESSIONS.md`); fix and remove the rest.

### P4 – Ongoing

10. **Coverage:** When adding features, add tests for new code paths. When coverage exceeds the threshold, raise the threshold (go-app, ml-service, frontend) per the run-check-all-incremental skill.
11. **New code:** Apply coding-principles and architect-ml-expert rules to all new code (small units, descriptive names, no new suppressions without justification).

---

## 5. Quick reference

- **Coding principles:** [.cursor/rules/coding-principles.mdc](../.cursor/rules/coding-principles.mdc)
- **Architecture & ML:** [.cursor/rules/architect-ml-expert.mdc](../.cursor/rules/architect-ml-expert.mdc)
- **Unit tests:** [.cursor/rules/unit-tests.mdc](../.cursor/rules/unit-tests.mdc)
- **Check workflow:** `.cursor/skills/run-check-all-incremental/SKILL.md`

---

*This audit reflects the state of the repository at the time of writing. Re-run checks and spot-checks after changes.*
