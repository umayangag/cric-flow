---
name: update-deps
description: Updates dependencies for go-app (Go modules), ml-service (Python pip), and frontend (npm). Use when the user asks to update dependencies, upgrade packages, bump deps, run update-deps, or refresh go-app/ml-service/frontend dependencies.
---

# Update Dependencies

Update dependencies across **go-app**, **ml-service**, and **frontend**. Run from the repository root.

## Scope

| Component   | Location    | Mechanism        |
|------------|-------------|------------------|
| go-app     | `go-app/`   | Go modules       |
| ml-service | `ml-service/` | pip + requirements.txt |
| frontend   | `frontend/` | npm              |

## Instructions

1. **go-app (Go)**
   - `cd go-app && go get -u ./... && go mod tidy`
   - Commit changes to `go.mod` and `go.sum` if any.

2. **ml-service (Python)**
   - Ensure venv exists: `make -C ml-service venv` if needed.
   - `cd ml-service && .venv/bin/pip install --upgrade -r requirements.txt`
   - To refresh pinned versions in `requirements.txt`: from `ml-service/` run `.venv/bin/pip freeze > requirements.txt` then trim to direct deps only if the project keeps a minimal list.
   - Commit changes to `ml-service/requirements.txt` if updated.

3. **frontend (npm)**
   - `cd frontend && npm update`
   - For major upgrades: run `npx npm-check-updates -u` then `npm install`, or use `npm install <pkg>@latest` for specific packages.
   - Commit changes to `package.json` and `package-lock.json` if any.

4. **Verification**
   - Run `make check-all` (or per-component: `make go-app-check`, `make ml-service-check`, `make frontend-check`) to ensure nothing is broken after updates.

## Optional: Update only one component

- **Go only:** `cd go-app && go get -u ./... && go mod tidy`
- **Python only:** use `ml-service/.venv/bin/pip install --upgrade -r ml-service/requirements.txt` (and optionally refresh `requirements.txt` as above).
- **Frontend only:** `cd frontend && npm update`

## Notes

- Go: `go get -u ./...` updates all direct and indirect dependencies to latest minor/patch within the module graph.
- Python: `requirements.txt` is pinned; upgrading the venv is safe; overwriting `requirements.txt` with `pip freeze` may add transitive deps—trim to direct deps if the project maintains a minimal list.
- Frontend: `npm update` only bumps within semver ranges in `package.json`; use `npm-check-updates` or `@latest` for major bumps.
