<!-- Generated from ../../.cursor/skills/run-check-all-incremental/SKILL.md by scripts/sync-junie-skills.py. Edit the Cursor copy, then re-run the script. -->

When the user says "/run-check-all-incremental" — Runs the same quality checks as 'make check-all' one component at a time (and within each component, fast checks before tests), fixing failures and re-running only the failed part until all pass. Coverage failures are handled by adding unit tests rather than lowering thresholds. Use when fixing lint, format, or test failures, or when the user wants all checks to pass without rerunning the full check-all repeatedly.

# Run check-all incrementally until all pass

## Goal

Achieve the same outcome as `make check-all` (all lints, format checks, and tests pass) with minimal reruns: run checks **per component**, **fast checks before slow tests**, and after any failure **re-run only the failed component or step** until it passes, then continue.

## Order of components (match root Makefile)

1. **frontend**
2. **go-app**
3. **ml-service**
4. **frontend-backend-sync** (ensures frontend FORMATS and model metadata match go-app and ml-service)

Do not run the next component until the current one passes all its steps.

## Per-component steps (run in order; stop at first failure)

### 1. Frontend

From repo root. Run each step; if it fails, fix and re-run **only that step**, then continue.

| Step       | Command |
|-----------|--------|
| install   | `make frontend-install` (optional; do once if needed) |
| lint      | `cd frontend && npm run lint` (ESLint with `--max-warnings 0`: fails on any warning) |
| format    | `cd frontend && npm run format:check` |
| typecheck | `cd frontend && npm run typecheck` |
| build     | `cd frontend && npm run build` |
| test      | `cd frontend && npm run test` |

### 2. Go-app

From repo root: `make -C go-app <target>` or `cd go-app && make <target>`.

| Step   | Command |
|--------|--------|
| vet    | `make -C go-app vet` |
| fmt    | `make -C go-app fmt-check` (fails if any file needs formatting; fix with `make -C go-app fmt`) |
| lint   | `make -C go-app lint` (uses golangci-lint; ensure GOPATH/bin on PATH if needed) |
| tests  | `make -C go-app coverage` |

### 3. ML service

From repo root. Use `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service <target>` so the venv is used (or set `ML_VENV_BIN` to `$(pwd)/ml-service/.venv/bin` and prepend to PATH).

| Step    | Command |
|---------|--------|
| lint    | `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service lint-check` |
| fmt     | `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service fmt-check` |
| coverage| `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service coverage` |
| cov-gate| `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service coverage-check` |

### 4. Frontend–backend sync

From repo root. Ensures frontend `FORMATS` (e.g. in `utils/opsStatusHelpers.ts`) and `defaultModelFeatures.ts` match go-app and ml-service.

| Step | Command |
|------|--------|
| sync | `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make frontend-backend-sync-check` |

## Workflow

1. Start with **frontend**. Run steps in order (lint → format:check → typecheck → build → test). If a step fails:
   - Fix the code (or config) based on the error.
   - Re-run **only that step** (e.g. only `npm run lint` or only `npm run format:check`). Do not run the full `make check-all`.
   - When the step passes, continue with the next step.
2. When all frontend steps pass, switch to **go-app**. Run vet → fmt-check → lint → coverage. On failure: fix, re-run only the failed step (or the whole go-app sequence once if unsure), then continue.
3. Then **ml-service**: lint-check → fmt-check → coverage → coverage-check. Same rule: fix, re-run only what failed.
4. Then run **frontend-backend-sync-check** (see step 4 table above). On failure: update frontend `FORMATS` in `utils/opsStatusHelpers.ts` and/or `constants/defaultModelFeatures.ts` to match go-app (`internal/formats`) and ml-service (`app/model_metadata.py`), then re-run only the sync step.
5. When all four (frontend, go-app, ml-service, sync) have passed, all checks are done. You can optionally run `make check-all` once to confirm.

## Warning handling

**Warnings must not be ignored.** Treat any warning emitted during a step as a failure to fix:

- **Frontend**
  - **ESLint**: Already uses `--max-warnings 0`; fix all lint warnings.
  - **React test warnings**: Fix `act(...)` warnings by wrapping state-triggering actions in `act()` or awaiting `waitFor`; fix "Each child in a list should have a unique key prop" by adding `key` to list children.
  - **Vite build**: Fix chunk-size warnings (e.g. "Some chunks are larger than 500 kB") via code-splitting, `manualChunks`, or `chunkSizeWarningLimit` only if code-splitting is impractical.
  - **Vitest/Node**: Fix `--localstorage-file` or similar Node/env warnings via config (e.g. `NODE_OPTIONS`, jsdom `environmentOptions`) rather than ignoring them.
- **Go-app**: golangci-lint uses `max-issues-per-linter: 0`; all linter output is treated as failure.
- **ML-service**: Ruff treats all findings as failures; fix them. **Pytest**: Fix RuntimeWarning/UserWarning (e.g. empty-slice, pd.to_datetime format) in tests or source; do not ignore via `-W ignore` unless unavoidable.

If a step completes with exit code 0 but emits warnings, treat it as a failure and fix the underlying issues before continuing.

## Coverage failure handling

When coverage or coverage-check fails because code coverage is below the specified threshold:

- **Do NOT** decrease or lower the coverage threshold in config files.
- **Do** add unit tests to improve coverage until the threshold is met. Inspect the coverage report to identify uncovered code paths (files, functions, branches), then write tests that exercise them.
- Re-run the coverage step (and coverage-check for ml-service) after adding tests to verify the threshold is satisfied.

## Raise coverage thresholds when passing

When coverage **passes** (actual ≥ threshold), raise the threshold to match the actual coverage (rounded down to the nearest integer). Example: threshold 60%, actual 61.7% → set new threshold to 61%. This keeps the bar moving up over time.

| Component   | Config location | Update |
|-------------|-----------------|--------|
| **go-app**  | `go-app/Makefile` | `COV_MIN ?= N` (line ~148) |
|             | Root `Makefile` | `COV_MIN_GO ?= N` (line ~588) |
|             | `.github/workflows/go-app-ci.yml` | `COV_MIN: "N"` in coverage-check step |
| **frontend**| `frontend/vite.config.ts` | In `test.coverage`, set each of `lines`, `functions`, `statements`, `branches` to floor(that metric’s actual %). E.g. if lines=62.3% and threshold 50, set lines to 62. |
| **ml-service** | `ml-service/Makefile` | `COV_MIN?=N` (line ~11) |
|             | Root `Makefile` | `COV_MIN_ML ?= N` (line ~589) |
|             | `.github/workflows/ml-service-ci.yml` | `COV_MIN: "N"` in coverage-check step |

After raising, re-run the coverage-check step for that component to confirm it still passes.

## Efficiency rules

- **Never** run `make check-all` in a loop. Use it only once at the end to confirm, or omit it.
- After a failure, **re-run only the failed step or the failed component**, not the entire check-all.
- Within a component, **do not** run tests before lint/fmt pass (they are cheaper and catch many issues).
- If the user has only changed one area (e.g. frontend), you may run only that component’s steps.

## Quick reference: one-liner per component

Use these only when you are sure the component is already passing and you want a single command:

- Frontend: `make frontend-check`
- Go-app: `make go-app-check`
- ML: `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make ml-service-check` (from root; or run from root `make ml-service-check` which uses `ML_VENV_BIN`)
- Sync: `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make frontend-backend-sync-check`

For fixing and iterating, prefer the **per-step commands** in the tables above so you re-run only what failed.
