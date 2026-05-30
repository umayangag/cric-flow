When the user says "/update-deps" or asks to update dependencies, upgrade packages, bump deps, or refresh go-app/ml-service/frontend dependencies (on the current branch, without opening PRs)

# Update Dependencies

Update dependencies across **go-app**, **ml-service**, and **frontend**. Run from the repository root.

For **separate PRs per component off main**, use `/update-deps-prs` instead.

## Scope

| Component   | Location    | Mechanism        |
|------------|-------------|------------------|
| go-app     | `go-app/`   | Go modules       |
| ml-service | `ml-service/` | pip-tools (requirements.in → requirements.txt) |
| frontend   | `frontend/` | npm              |

## Instructions

1. **go-app (Go)**
   - `cd go-app && go get -u ./... && go mod tidy`
   - Commit changes to `go.mod` and `go.sum` if any.

2. **ml-service (Python, pip-tools)**
   - Direct dependencies live in `ml-service/requirements.in` only. Pinned output is generated into `requirements.txt` via pip-compile; do not edit `requirements.txt` by hand.
   - Ensure venv exists: `make -C ml-service venv` if needed.
   - To upgrade installed packages and refresh pins: `cd ml-service && make compile-requirements` (or: `.venv/bin/pip install pip-tools && .venv/bin/pip-compile -U requirements.in -o requirements.txt`), then `make -C ml-service install` to sync the venv.
   - To add a new direct dependency: add the package name to `requirements.in`, then run `make -C ml-service compile-requirements`.
   - Commit changes to `ml-service/requirements.in` and `ml-service/requirements.txt` if updated.

3. **frontend (npm)**
   - `cd frontend && npm update`
   - For major upgrades: run `npx npm-check-updates -u` then `npm install`, or use `npm install <pkg>@latest` for specific packages.
   - Commit changes to `package.json` and `package-lock.json` if any.

4. **Verification**
   - Run `make check-all` (or per-component: `make go-app-check`, `make ml-service-check`, `make frontend-check`) to ensure nothing is broken after updates.

## Optional: Update only one component

- **Go only:** `cd go-app && go get -u ./... && go mod tidy`
- **Python only:** run `make -C ml-service compile-requirements` to refresh pins from `requirements.in`, then `make -C ml-service install`.
- **Frontend only:** `cd frontend && npm update`

## Notes

- Go: `go get -u ./...` updates all direct and indirect dependencies to latest minor/patch within the module graph.
- Python: use pip-tools: maintain direct deps in `requirements.in`; run `pip-compile requirements.in` (or `make compile-requirements`) to regenerate pinned `requirements.txt` with full dependency tree.
- Frontend: `npm update` only bumps within semver ranges in `package.json`; use `npm-check-updates` or `@latest` for major bumps.
