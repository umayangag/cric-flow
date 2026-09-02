---
name: update-deps
description: Updates dependencies for go-app (Go modules), ml-service (pip-tools) and frontend (npm). Bumps on the current branch by default; opens one independent PR per component off main only when explicitly asked. Use when the user asks to update dependencies, upgrade packages, bump deps, or refresh the repo's packages, with or without separate PRs.
---

# Update dependencies

Update dependencies across **go-app**, **ml-service**, and **frontend**. Run from the repository
root.

Two modes:

| Mode | When | What it does |
|------|------|--------------|
| **Bump in place** (default) | "update the deps", `/update-deps` | Bumps on the current branch. No branches, no pushes, no PRs. |
| **PR per component** | Only when the user explicitly asks for separate PRs | One branch off `main` per component, each verified, pushed, and opened as its own PR. |

> **Hard rule:** never create a branch, commit, push, or open a PR unless the user explicitly
> asked for the PR mode. The default mode stops after the bumps and verification.

## Scope

| Component | Location | Mechanism |
|-----------|----------|-----------|
| go-app | `go-app/` | Go modules |
| ml-service | `ml-service/` | pip-tools (`requirements.in` → `requirements.txt`) |
| frontend | `frontend/` | npm |

---

## Bump commands

These are the single source for how each component is bumped. Both modes use them.

### go-app

```bash
cd go-app && go get -u ./... && go mod tidy && cd ..
```

Touches `go-app/go.mod` and `go-app/go.sum`. `go get -u ./...` updates direct and indirect
dependencies to the latest compatible versions in the module graph.

### ml-service

Direct dependencies live in `requirements.in`. `requirements.txt` is
**generated** — never edit it by hand.

| File | Contents | Regenerate with |
|------|----------|-----------------|
| `requirements.txt` | The runtime set the image ships **and CI installs** | `make -C ml-service compile-requirements-docker` |

```bash
make -C ml-service compile-requirements
make -C ml-service install   # sync the local venv to the new pins
```

Notes:

- `compile-requirements` uses the **local venv**, whose Python may not match CI. If a Docker build
  later fails on a resolution conflict (e.g. click/typer), regenerate with
  `make -C ml-service compile-requirements-docker` instead, which pins under Python 3.12.
- Prefer the **`-docker`** variant: CI runs Python 3.12 and installs `requirements.txt`
  directly, so the pins should be resolved under that interpreter.
- To **add** a direct dependency, add it to `requirements.in` first, then recompile.

### frontend

```bash
cd frontend && npm update && cd ..
```

Usually only changes `package-lock.json`, since `npm update` stays inside the semver ranges in
`package.json`. For **major** bumps (only if the user asks): `npx npm-check-updates -u` then
`npm install`, or `npm install <pkg>@latest` for a specific package.

---

## Verify

Run checks for the components you actually bumped. Follow the check order and the
"warnings are failures / never lower a threshold" rules in [CLAUDE.md](../../../CLAUDE.md)
§ Quality bars.

| Component | Command |
|-----------|---------|
| go-app | `make -C go-app vet fmt-check lint coverage` |
| ml-service | `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service lint-check fmt-check coverage coverage-check` |
| frontend | from `frontend/`: `npm run lint`, `format:check`, `typecheck`, `build`, `test` |
| all | `make check-all` |

Dependency-only changes do **not** need `frontend-backend-sync-check` unless format constants or
model metadata moved.

In the default mode, stop here and report what changed.

---

## Mode: one PR per component

Only when the user explicitly asked. Each component gets its own branch off `main`, verified,
committed, pushed, and opened as an independent PR.

### Prerequisites

- **GitHub CLI (`gh`)** authenticated with `repo` scope
- Go 1.26+, Python 3.12 (ml-service venv), Node 20.x

### Hard rules

- Base every PR on **`main`** — fetch and pull before each slice
- **One PR per component.** Never combine go-app + ml-service + frontend
- **Stage only that component's files.** Never `git add .` / `git add -A`
- If a component has **no diff** after the bump, skip its commit, push, and PR
- No destructive git (force-push, hard reset, branch delete) without an explicit request

### Slices

| # | Component | Branch | Files to commit |
|---|-----------|--------|-----------------|
| 1 | go-app | `chore/update-go-app-deps` | `go-app/go.mod`, `go-app/go.sum` |
| 2 | ml-service | `chore/update-ml-service-deps` | `ml-service/requirements.txt` (plus `requirements.in` if direct deps changed) |
| 3 | frontend | `chore/update-frontend-deps` | `frontend/package.json`, `frontend/package-lock.json` (often lockfile only) |

The user may ask for a single slice ("ml-service only") — still branch from `main` and open one PR.

### Per-slice workflow

```bash
# 1. Branch from main
git fetch origin main && git checkout main && git pull origin main
git checkout -b <branch-from-table>

# 2. Bump — see "Bump commands" above for this component

# 3. Verify — see "Verify" above, this component only

# 4. Commit, push, open PR
git add <only the files from the table>
git commit -m "<subject from the table below>"
git push -u origin HEAD
gh pr create --base main --head <branch> --title "<title>" --body "$(cat <<'BODY'
## Summary
- <what was upgraded>

## Test plan
- [x] <checks you ran>
BODY
)"
```

### Commit / PR titles

| Component | Subject (commit and PR title) |
|-----------|-------------------------------|
| go-app | `chore(go-app): bump Go module dependencies` |
| ml-service | `chore(ml-service): refresh pinned Python dependencies` |
| frontend | `chore(frontend): bump npm lockfile dependencies` |

If `package.json` ranges or an `.in` file changed, say so in the PR summary.

### Report

After all slices, print a table: component → PR URL (or "skipped, no diff").

### Acceptance criteria

- [ ] Up to three PRs off `main`, skipping components with no diff
- [ ] Each slice passed its own component checks before push
- [ ] `ml-service/requirements.txt` regenerated from `requirements.in`, never hand-edited
- [ ] User has the PR URLs
