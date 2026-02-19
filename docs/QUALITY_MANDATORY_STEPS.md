# Mandatory steps for system quality

This document lists **mandatory** quality steps and the current status. Use it to keep the bar consistent and to onboard contributors.

---

## 1. Local: `make check-all` must match CI

**Goal:** Running `make check-all` locally should enforce the same gates as CI so nothing passes locally and fails in CI (or the reverse).

| Component        | check-all today                    | CI today                         | Action |
|-----------------|------------------------------------|----------------------------------|--------|
| frontend        | lint, format:check, typecheck, build, test | build, test only                 | **CI:** Add lint + typecheck to frontend workflow. |
| go-app          | vet, fmt-check, lint, coverage     | vet, fmt-check (lint workflow), tests + coverage-check (30%) | **Local:** Add `coverage-check` to go-app-check. **CI:** Align COV_MIN (see below). |
| ml-service      | lint-check, fmt-check, coverage, coverage-check | coverage-check (80%)             | OK. |

**Done in this repo:** go-app-check now includes `coverage-check`. Frontend workflow now runs lint and typecheck. go-app CI COV_MIN documented below.

---

## 2. Coverage thresholds

**Goal:** Clear, consistent coverage gates so coverage doesn’t regress.

| Component        | Default (Makefile) | CI workflow      | Recommendation |
|-----------------|--------------------|------------------|----------------|
| go-app          | COV_MIN_GO=80      | COV_MIN=30 in workflow | Use one value everywhere. If 80% is not feasible yet, set COV_MIN in the workflow explicitly (e.g. 50) and document in this file. |
| ml-service      | 80%                | 80%              | OK. |

**Action:** go-app workflow currently uses COV_MIN=30 so CI stays green (total coverage is ~33%). Raise COV_MIN in `.github/workflows/go-app-tests.yml` as coverage improves; Makefile default is 80 for `make ci-go` and `make check-all`.

---

## 3. CI on shared/config changes

**Goal:** PRs that only change shared config (e.g. `configs/feature_vectors.json`, `go-app/config.json`, `Makefile`) still run the right checks.

- Workflows use **path filters**. Changing only `configs/` or root `Makefile` may not trigger go-app or ml-service workflows.
- **Options:**  
  - Add those paths to the relevant workflow(s), or  
  - Add a lightweight “config change” job that runs at least vet/lint for go-app and ml-service when those paths change.

---

## 4. Pre-commit hooks

**Goal:** Format and lint run before commit so CI doesn’t fail on style alone.

- **Status:** `.githooks/pre-commit` runs gofumpt/golines (Go), black/isort/ruff (Python), prettier/eslint (frontend) on staged files.
- **Requirement:** Everyone runs `make install-hooks` (or equivalent) after clone so hooks are active.

---

## 5. Branch protection (repository settings)

**Goal:** main (or default branch) cannot be updated without passing the right checks.

- **Recommended:** Require status checks to pass before merge. Required checks should include at least:
  - Go App Lint (or equivalent)
  - Go App Tests
  - ML Service Tests
  - Frontend Tests
- Optional: Require “check-all” or a single aggregated workflow if you add one.

---

## 6. PR template and testing standards

**Goal:** Every PR confirms that tests were run and (for Go) follows testing standards.

- **Status:** `.github/PULL_REQUEST_TEMPLATE.md` includes a Go unit test standards checklist and reminders to run go-app and ml-service tests.
- **Suggestion:** Add a checkbox: “Ran `make check-all` (or the relevant component checks) and all passed.”

---

## Summary checklist

- [x] go-app-check includes coverage-check (local check-all enforces coverage).
- [x] Frontend CI runs lint and typecheck (not only build + test).
- [ ] go-app CI COV_MIN set and documented (e.g. 80 or 50 in workflow).
- [ ] Path filters updated or “config” job added so shared config changes run tests.
- [ ] Branch protection requires the relevant CI status checks.
- [ ] PR template includes “make check-all or component checks passed” (optional).

After completing the unchecked items, the system has a consistent quality bar locally and in CI.
