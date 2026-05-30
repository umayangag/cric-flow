When the user says "/update-deps-prs" or asks to update dependencies across the repo with separate PRs per component (go-app, ml-service, frontend)

# Update dependencies (separate PRs per component)

Update **go-app**, **ml-service**, and **frontend** independently. Each component gets its own branch off `main`, local verification, commit, push, and `gh pr create`.

For bump commands only (no PRs), use `/update-deps`. For detailed check commands, use `/run-check-all-incremental`.

## Prerequisites

- Repository root; **GitHub CLI (`gh`)** authenticated
- **Go 1.26+**, **Python 3.12** (ml-service venv), **Node 20.x**

## Hard rules

- Base every PR on **`main`**: fetch, checkout, pull before each slice
- **One PR per component** — never combine all three in one PR
- **Stage only that component's files** — never `git add .` / `git add -A`
- If no diff after update, skip commit/push/PR for that slice

## Slices (in order)

| # | Component | Branch | Files to commit |
|---|-----------|--------|-----------------|
| 1 | go-app | `chore/update-go-app-deps` | `go-app/go.mod`, `go-app/go.sum` |
| 2 | ml-service | `chore/update-ml-service-deps` | `ml-service/requirements.txt`, `ml-service/requirements-ci.txt` |
| 3 | frontend | `chore/update-frontend-deps` | `frontend/package.json`, `frontend/package-lock.json` |

User may request one component only — still branch from `main`.

## Per-slice workflow

1. `git fetch origin main && git checkout main && git pull origin main && git checkout -b <branch>`
2. **Update**
   - go-app: `cd go-app && go get -u ./... && go mod tidy && cd ..`
   - ml-service: `make -C ml-service compile-requirements compile-requirements-ci install`
   - frontend: `cd frontend && npm update && cd ..`
3. **Verify (this component only)**
   - go-app: `make -C go-app vet fmt-check lint coverage`
   - ml-service: `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service lint-check fmt-check coverage coverage-check`
   - frontend: `npm run lint`, `format:check`, `typecheck`, `build`, `test` in `frontend/`
4. **Commit, push, PR** — stage only table files; use HEREDOC commit message; `gh pr create --base main`

## PR titles

- go-app: `chore(go-app): bump Go module dependencies`
- ml-service: `chore(ml-service): refresh pinned Python dependencies` (+ CI pins commit if needed)
- frontend: `chore(frontend): bump npm lockfile dependencies`

Report PR URLs when done. PRs are independent and can merge in any order.
