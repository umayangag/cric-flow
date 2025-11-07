# Plan: Refactor CI Workflows to Reuse Makefile Targets (2025-11-07)

Owner: Junie
Scope: Remove duplicated shell logic from GitHub workflows by delegating to reusable Makefile targets. Standardize CI commands across components.

## Goals
- DRY CI: Workflows call Make targets instead of inlined commands.
- Consistent local/CI behavior: The same targets can be run locally to reproduce CI.
- Preserve existing coverage gates (Go ≥ 90%, Python ≥ 80%).

## Out of Scope
- Changing test logic or coverage thresholds beyond what’s already enforced (Go: 90%, Python: 80%).
- Introducing new languages or services.

## Changes by File

1. ml-service/Makefile
- Add CI-focused targets used by workflow:
  - `ci-setup`: ensure dev tools installed in the venv (black, isort, flake8, ruff, pytest, pytest-cov).
  - `test`: run `pytest -q`.
  - `coverage`: run tests with coverage and produce `coverage.xml` (term + xml reports).
  - `coverage-check`: enforce minimum coverage using env `COV_MIN` (default 80).
  - `ci`: run `fmt-check`, `lint-check`, `coverage`, and `coverage-check`.
- Keep existing developer targets unchanged for backward compatibility.

2. Makefile (repo root)
- Add aggregate CI targets:
  - `ci-ml`: `$(MAKE) -C ml-service ci` (forward `COV_MIN` if set).
  - `ci-go`: `$(MAKE) -C go-app coverage` then `COV_MIN=$(COV_MIN_GO) $(MAKE) -C go-app coverage-check` (default `COV_MIN_GO=90`).
  - `ci`: run `ci-go` and `ci-ml` (useful for local verification or a future monorepo workflow).

3. .github/workflows/ml-ci.yml
- Replace ad-hoc install/lint/test commands with calls to Make targets:
  - `make -C ml-service init` (runtime deps)
  - `make -C ml-service ci-setup`
  - `COV_MIN=80 make -C ml-service ci`
- Keep artifact upload of `ml-service/coverage.xml`.

4. .github/workflows/go-ci.yml
- Option A (minimal change): keep as-is since it already uses `make` for vet/fmt/coverage.
- Option B (unify style): replace individual steps with `make ci-go`. This plan will implement Option A to minimize churn.

## Acceptance Criteria
- GitHub Actions workflows succeed without duplicating commands that are already available via Make.
- `ml-service` coverage gate enforced at 80% via `COV_MIN` in Makefile, not via hardcoded workflow flags.
- `go-app` workflow behavior unchanged (still enforces 90% via Make targets already present).
- Running locally from repo root reproduces CI:
  - `make -C ml-service ci-setup && COV_MIN=80 make -C ml-service ci` succeeds and generates `ml-service/coverage.xml`.
  - `make ci-go` passes and writes `go-app/coverage.out`.
  - `make ci` runs both components and succeeds.

## Verification Commands
- Python (ml-service):
  - `make -C ml-service init`
  - `make -C ml-service ci-setup`
  - `COV_MIN=80 make -C ml-service ci`
  - Verify artifact: `test -f ml-service/coverage.xml`
- Go (go-app):
  - `make -C go-app fmt-check`
  - `make -C go-app coverage && COV_MIN=90 make -C go-app coverage-check`
- Aggregate (root):
  - `make ci-ml`
  - `make ci-go`
  - `make ci`

## Risks & Mitigations
- Risk: CI runners may not have venv activated for `ml-service` tools.
  - Mitigation: `ci-setup` installs tools into the venv and calls them via venv bin or python -m; `init` ensures deps.
- Risk: Path differences for coverage artifact.
  - Mitigation: Standardize to `ml-service/coverage.xml` and keep upload step aligned.

## Branching & Commits
- Branch: `ci/reuse-make-in-workflows`
- Commits (examples):
  - `ci(ml-service): add CI Make targets (ci-setup, coverage, coverage-check, ci)`
  - `ci(root): add ci-go, ci-ml, ci aggregate targets`
  - `ci: refactor ml-service workflow to call Make targets`

## Rollout Plan
1) Add Make targets (ml-service and root).  
2) Update workflows to use the new targets.  
3) Push branch and validate in PR.  
4) If green, merge.
