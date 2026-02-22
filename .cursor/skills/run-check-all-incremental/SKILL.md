---
name: run-check-all-incremental
description: Runs the same quality checks as 'make check-all' one component at a time (and within each component, fast checks before tests), fixing failures and re-running only the failed part until all pass. Use when fixing lint, format, or test failures, or when the user wants all checks to pass without rerunning the full check-all repeatedly.
---

# Run check-all incrementally until all pass

## Goal

Achieve the same outcome as `make check-all` (all lints, format checks, and tests pass) with minimal reruns: run checks **per component**, **fast checks before slow tests**, and after any failure **re-run only the failed component or step** until it passes, then continue.

## Order of components (match root Makefile)

1. **frontend**
2. **go-app**
3. **ml-service**

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

## Workflow

1. Start with **frontend**. Run steps in order (lint → format:check → typecheck → build → test). If a step fails:
   - Fix the code (or config) based on the error.
   - Re-run **only that step** (e.g. only `npm run lint` or only `npm run format:check`). Do not run the full `make check-all`.
   - When the step passes, continue with the next step.
2. When all frontend steps pass, switch to **go-app**. Run vet → fmt-check → lint → coverage. On failure: fix, re-run only the failed step (or the whole go-app sequence once if unsure), then continue.
3. Then **ml-service**: lint-check → fmt-check → coverage → coverage-check. Same rule: fix, re-run only what failed.
4. When all three components have passed all their steps, all checks are done. You can optionally run `make check-all` once to confirm.

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

For fixing and iterating, prefer the **per-step commands** in the tables above so you re-run only what failed.
