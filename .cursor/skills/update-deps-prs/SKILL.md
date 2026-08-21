---
name: update-deps-prs
description: Updates go-app, ml-service, and frontend dependencies and opens one independent PR per component off main. Use when the user says /update-deps-prs, asks to bump deps with separate PRs, or refresh the whole repo's packages like the chore/update-*-deps workflow.
disable-model-invocation: true
---

# Update dependencies (separate PRs per component)

When the user says **/update-deps-prs** (or asks for repo-wide dependency bumps as **separate PRs**), update **go-app**, **ml-service**, and **frontend** independently. Each component gets its own branch off `main`, local verification, commit, push, and `gh pr create`.

For bump commands and pip/npm mechanics only (no PRs), see [update-deps](../update-deps/SKILL.md). For check commands, see [run-check-all-incremental](../run-check-all-incremental/SKILL.md).

## Prerequisites

- Repository root; **GitHub CLI (`gh`)** authenticated
- **Go 1.26+**, **Python 3.12** (ml-service venv), **Node 20.x**
- User has approved creating branches, commits, pushes, and PRs (explicit `/update-deps-prs` counts as approval)

## Hard rules

- Base every PR on **`main`**: `git fetch origin main && git checkout main && git pull origin main` before each slice
- **One PR per component** — do not combine go-app + ml-service + frontend in one PR
- **Stage only that component's files** — never `git add .` / `git add -A`
- If a component has **no diff** after update, skip commit/push/PR for that slice
- Do not use destructive git (force-push, hard reset, branch delete) without explicit user request

## Slices (run in this order)

| # | Component | Branch | Files to commit |
|---|-----------|--------|-----------------|
| 1 | go-app | `chore/update-go-app-deps` | `go-app/go.mod`, `go-app/go.sum` |
| 2 | ml-service | `chore/update-ml-service-deps` | `ml-service/requirements.txt`, `ml-service/requirements-ci.txt` |
| 3 | frontend | `chore/update-frontend-deps` | `frontend/package.json`, `frontend/package-lock.json` (lockfile often only) |

Optional: user may request a **single** slice (e.g. "ml-service only") — still branch from `main` and open one PR.

## Per-slice workflow

Repeat for each slice:

### 1. Branch from main

```bash
git fetch origin main
git checkout main
git pull origin main
git checkout -b <branch-from-table>
```

### 2. Update dependencies

**go-app**

```bash
cd go-app && go get -u ./... && go mod tidy && cd ..
```

**ml-service**

```bash
make -C ml-service compile-requirements
make -C ml-service compile-requirements-ci
make -C ml-service install
```

Do not edit `requirements.txt` or `requirements-ci.txt` by hand. Direct deps live in `requirements.in` / `requirements-ci.in`.

**frontend**

```bash
cd frontend && npm update && cd ..
```

For **major** frontend bumps (optional, only if user asks): `npx npm-check-updates -u` then `npm install` in `frontend/`.

### 3. Verify (component only)

Run checks for **this slice only** before commit. Fix failures; re-run only the failed step.

**go-app:** `make -C go-app vet fmt-check lint coverage`

**ml-service:**

```bash
PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service lint-check fmt-check coverage coverage-check
```

**frontend:** from `frontend/`: `npm run lint`, `format:check`, `typecheck`, `build`, `test`

Dependency-only PRs do **not** need `frontend-backend-sync-check` unless metadata/format constants changed.

### 4. Commit, push, open PR

```bash
git add <only files from table>
git commit -m "$(cat <<'EOF'
<commit subject — see templates below>

<one-line body>
EOF
)"
git push -u origin HEAD
gh pr create --base main --head <branch> --title "<title>" --body "$(cat <<'EOF'
## Summary
- <bullet: what was upgraded>

## Test plan
- [x] <checks you ran>
EOF
)"
```

**Commit / PR titles**

| Component | Commit subject | PR title |
|-----------|----------------|----------|
| go-app | `chore(go-app): bump Go module dependencies` | same |
| ml-service | `chore(ml-service): refresh pinned Python dependencies` | same (CI pins can be a second commit: `chore(ml-service): refresh CI requirements pins`) |
| frontend | `chore(frontend): bump npm lockfile dependencies` | same |

Record each PR URL. After all slices, report a short table: component → PR link.

## Optional: major frontend or requirements.in changes

- If `package.json` version ranges change, mention in PR summary
- If `requirements.in` changes, include it in the ml-service commit (still one ml-service PR)

## Notes

- **go-app:** `go get -u ./...` updates the module graph to latest compatible versions
- **ml-service:** runtime pins (`requirements.txt`) and CI-minimal pins (`requirements-ci.txt`) must both be regenerated
- **frontend:** `npm update` usually only changes `package-lock.json` within semver ranges in `package.json`
- PRs are independent and can merge in any order

## Acceptance criteria

- [ ] Up to three PRs off `main` (skipped slices with no diff)
- [ ] Each merged slice passed its component checks before push
- [ ] User receives PR URLs for go-app, ml-service, and frontend
