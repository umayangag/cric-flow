
# Variables
APP_NAME:=ml-service
VENV:=.venv
PY:=$(VENV)/bin/python3
PIP:=$(VENV)/bin/pip
UVICORN:=$(VENV)/bin/uvicorn
LINE_LENGTH?=120
# Enforced by CI and make coverage-check. Keep aligned with .github/workflows/ml-service-ci.yml
# COV_MIN and the root Makefile's COV_MIN_ML; tests/test_coverage_gate.py fails if the three drift.
# One requirements set now serves both CI and a local venv (P-6 removed the optional
# pycaret / autogluon / shap extras), so the local figure and the gate's are the same run.
#
# 95 -> 94: a correction, not a lowering. The 95 set in PR #341 was read off the report's
# rounded TOTAL row; the gate's own line said 94.94%, and the gate could not fail (see
# tests/test_coverage_gate.py), so 95 was never met on any run. 94 is 94.94% floored --
# the honest figure the gate now enforces. Raise it, never lower it.
COV_MIN?=94
REQUIREMENTS_FILE?=requirements.txt

.PHONY: all venv install compile-requirements compile-requirements-docker run retrain evaluate reload venue-weather
.PHONY: xi-parity serving-parity export-birth-dates docker-build docker-run clean fmt fmt-check init lint lint-check check-reachability test test-integration test-e2e
.PHONY: coverage coverage-check ci ci-setup help list _install_test_tools _install_ruff

all: run

venv:
	@test -d $(VENV) || python3 -m venv $(VENV)

install: venv
	$(PIP) install --upgrade pip
	PYO3_USE_ABI3_FORWARD_COMPATIBILITY=1 $(PIP) install -r $(REQUIREMENTS_FILE)

# Regenerate requirements.txt from requirements.in (pip-tools). Run after editing requirements.in or to upgrade pins.
# Uses local venv (may be Python 3.9); for Docker/Python 3.12 use compile-requirements-docker.
compile-requirements: venv
	$(PIP) install --upgrade pip pip-tools
	$(VENV)/bin/pip-compile -U --strip-extras requirements.in -o requirements.txt

# Regenerate requirements.txt targeting Python 3.12 (matches Docker). Use when Docker build fails on click/typer conflicts.
compile-requirements-docker:
	docker run --rm -v $(CURDIR):/app -w /app python:3.12-slim sh -c "pip install pip-tools && pip-compile -U --strip-extras requirements.in -o requirements.txt"

# Initialize local Python development environment (venv + deps + dev tools)
init: install
	@echo "Setting up Python dev environment in $(VENV)..."
	$(PIP) install ruff pytest pytest-cov
	@echo "Done. Activate the venv: 'source $(VENV)/bin/activate' (or '.\\$(VENV)\\Scripts\\activate' on Windows)"
	@echo "Then run: 'make fmt-check' or 'make run'"

run:
	$(UVICORN) app.main:app --host 0.0.0.0 --port 8000 --reload

# The whole model build, in one step: rating pass -> XI win models (with the grid) ->
# performance models -> report -> run manifest, written into runs/<run_id>/. It publishes
# nothing; `make reload` moves `current`.
retrain: venv
	@test -n "$(CUTOFF)" || (echo "CUTOFF is required (YYYY-MM-DD; rows at/after it are the holdout), e.g. make retrain CUTOFF=2025-09-01" && exit 1)
	$(PY) -m ml.xi.retrain --cutoff "$(CUTOFF)" $(if $(CRICSHEET_DIR),--cricsheet-dir "$(CRICSHEET_DIR)",--postgres) $(if $(BIRTH_DATES),--birth-dates "$(BIRTH_DATES)",) $(if $(XI_OUT),--out "$(XI_OUT)",) $(if $(ACCEPT_DATA_QUALITY),--accept-data-quality,)

# L4 evaluation harness (H-19): rolling-origin walk-forward + the locked window, selection
# and performance metrics, the leak canary, and the train/serve parity check (H-8). Writes
# one JSON report (xi_evaluate_report.json) and touches no artifact `current` points at.
evaluate: venv
	$(PY) -m ml.xi.evaluate $(if $(CRICSHEET_DIR),--cricsheet-dir "$(CRICSHEET_DIR)",--postgres) $(if $(BIRTH_DATES),--birth-dates "$(BIRTH_DATES)",) $(if $(XI_OUT),--out "$(XI_OUT)",) $(if $(GENDER_SPLIT_CONTEXT),--gender-split-context,) $(if $(MARKET_ODDS_DIR),--market-odds-dir "$(MARKET_ODDS_DIR)",)

# Serving parity of a run on disk (H-8, EVAL-10): load the run `current` names (or RUN=)
# through XiStore.load and hold what it serves to a fresh pass over its own data. Refuses a
# run the loader refuses, and one trained on other cricket than the source now holds.
# Usage: make serving-parity [RUN=<run_id>] [XI_OUT=<artifacts root>]
serving-parity: venv
	$(PY) -m ml.xi.asof $(if $(CRICSHEET_DIR),--cricsheet-dir "$(CRICSHEET_DIR)",--postgres) $(if $(BIRTH_DATES),--birth-dates "$(BIRTH_DATES)",) $(if $(XI_OUT),--out "$(XI_OUT)",) $(if $(RUN),--run "$(RUN)",)

# Check the database against the Cricsheet archive: both rating sources are supposed to
# describe the same cricket, and twice they did not (H-15). Needs both, so it is not part
# of a retrain. Usage: make xi-parity CRICSHEET_DIR=../data/go-app/cricsheet
xi-parity: venv
	@test -n "$(CRICSHEET_DIR)" || (echo "CRICSHEET_DIR is required, e.g. make xi-parity CRICSHEET_DIR=../data/go-app/cricsheet" && exit 1)
	$(PY) -m ml.xi.parity --cricsheet-dir "$(CRICSHEET_DIR)" $(if $(BIRTH_DATES),--birth-dates "$(BIRTH_DATES)",)

# The archive carries no biography (X-1b): export the database's dates of birth as the CSV
# the archive path reads, so an offline retrain, evaluate or parity run sees the same ages.
# Usage: make export-birth-dates BIRTH_DATES=../data/go-app/player-birth-dates.csv
export-birth-dates: venv
	@test -n "$(BIRTH_DATES)" || (echo "BIRTH_DATES is required, e.g. make export-birth-dates BIRTH_DATES=../data/go-app/player-birth-dates.csv" && exit 1)
	$(PY) -m ml.xi.biography --export "$(BIRTH_DATES)"

# X-2: the weather acquisition and its offline restore, one command (ml/weather/backfill.py).
# Usage: make venue-weather CRICSHEET_DIR=../data/go-app/cricsheet WEATHER_ARGS="--offline --to-db"
venue-weather: venv
	@test -n "$(CRICSHEET_DIR)" || (echo "CRICSHEET_DIR is required, e.g. make venue-weather CRICSHEET_DIR=../data/go-app/cricsheet" && exit 1)
	$(PY) -m ml.weather.backfill --cricsheet-dir "$(CRICSHEET_DIR)" $(WEATHER_ARGS)

# Point `current` at a run and load it into the running service (the `reload` step).
# RUN names one; with none, the newest run on disk -- the one a retrain just built.
#
# No default key (OPS-02): it must be the ADMIN_API_KEY the service was started with, and
# a default here was a shared secret every checkout knows. The root Makefile passes it
# down; called directly, set it on the command line or in the environment.
API_KEY ?=
ML_URL ?= http://localhost:8000
RUN ?=
reload:
	@test -n "$(API_KEY)" || { echo "API_KEY is required and has no default: make reload API_KEY=<the key the service was started with>" && exit 1; }
	curl -fsS -X POST -H "X-API-Key: $(API_KEY)" "$(ML_URL)/admin/reload$(if $(RUN),?run=$(RUN),)" | tee /dev/stderr >/dev/null

# Format Python code with ruff (format + lint fix)
fmt: venv _install_ruff
	$(VENV)/bin/ruff format --line-length $(LINE_LENGTH) .
	$(VENV)/bin/ruff check --fix .

# Auto-fix lint (unused imports, etc.)
lint: venv _install_ruff
	$(VENV)/bin/ruff check --fix .

# Import-graph reachability: fails on modules no live entrypoint can reach.
# ruff and pytest both stay quiet about these, because a module's own tests keep
# it "used". Needs no venv.
check-reachability:
	@cd .. && python3 scripts/py-reachability.py --check

# Lint check only (CI-safe)
lint-check: venv _install_ruff
	$(VENV)/bin/ruff check .

# Check formatting and lint (same as CI)
fmt-check: venv _install_ruff
	$(VENV)/bin/ruff format --check --line-length $(LINE_LENGTH) .
	$(VENV)/bin/ruff check .

# Private helper to ensure ruff is installed in the venv
_install_ruff: venv
	@$(PIP) show ruff >/dev/null 2>&1 || { echo "Installing ruff into $(VENV)..."; $(PIP) install ruff >/dev/null; }

# --- CI convenience targets ---
# Install deps for CI (lint + tests). The same set the image ships.
# Regenerate with: make compile-requirements-docker
ci-setup:
	python -m pip install --upgrade pip
	python -m pip install -r requirements.txt
	python -m pip install ruff pytest pytest-cov httpx

# Helper to ensure pytest and pytest-cov are available in the project venv
_install_test_tools: install
	@$(PIP) show pytest >/dev/null 2>&1 || { echo "Installing pytest into $(VENV)..."; $(PIP) install pytest >/dev/null; }
	@$(PIP) show pytest-cov >/dev/null 2>&1 || { echo "Installing pytest-cov into $(VENV)..."; $(PIP) install pytest-cov >/dev/null; }

# Run tests (quiet) ensuring runtime deps are installed in venv (includes integration)
test: _install_test_tools
	$(VENV)/bin/pytest -q

# Run only integration tests (multi-component, in-process)
test-integration: _install_test_tools
	$(VENV)/bin/pytest -q -m integration -v

# Run e2e tests (requires live ML service; set RUN_E2E=1)
test-e2e: _install_test_tools
	RUN_E2E=1 $(VENV)/bin/pytest -q -m e2e -v

# Generate coverage reports (xml + term) ensuring runtime deps are installed
coverage: _install_test_tools
	$(VENV)/bin/pytest -q \
		--cov \
		--cov-report=xml:coverage.xml \
		--cov-report=term

# Enforce minimum coverage threshold (may re-run tests) ensuring runtime deps are installed
coverage-check: _install_test_tools
	$(VENV)/bin/pytest -q \
		--cov \
		--cov-report=term \
		--cov-fail-under=$(COV_MIN)

# Full CI pipeline for ml-service
ci: fmt-check lint-check check-reachability coverage coverage-check


# Build from repo root: the Dockerfile's context is the repo, not ml-service.
docker-build:
	docker build -t $(APP_NAME):latest -f Dockerfile ..

docker-run:
	docker run --rm -p 8000:8000 --name $(APP_NAME) $(APP_NAME):latest

clean:
	rm -rf $(VENV)


# Grouped help and target listing
help:
	@echo "\nML Service — Make targets (grouped)"
	@echo "-----------------------------------"
	@echo "[Setup]"
	@echo "  venv               Create local virtualenv (.venv)"
	@echo "  install            Install runtime dependencies into venv"
	@echo "  compile-requirements  Regenerate requirements.txt from requirements.in (pip-tools)"
	@echo "  init               Install dev tools (ruff, pytest, etc.)"
	@echo
	@echo "[Run & Train]"
	@echo "  run                Start FastAPI with uvicorn (reload)"
	@echo "  retrain            Build one run (CUTOFF=YYYY-MM-DD); writes runs/<id>/, publishes nothing"
	@echo "  evaluate           Run L4 and write its report; touches no artifact current points at"
	@echo "  reload             Point current at a run and load it (RUN=<id> optional)"
	@echo ""
	@echo "  Pipeline: import (go-app) → retrain → reload."
	@echo
	@echo "[Validate & Test]"
	@echo "  xi-parity          Compare both rating sources (CRICSHEET_DIR=...)"
	@echo "  serving-parity     H-8 for a run on disk against a fresh pass (RUN=<run_id>, default current)"
	@echo "  export-birth-dates Write the database's dates of birth as the CSV the archive path reads (BIRTH_DATES=...)"
	@echo "  test               Run pytest"
	@echo "  coverage           Run pytest with coverage (xml + term)"
	@echo "  coverage-check     Enforce coverage threshold (COV_MIN)"
	@echo "  ci                 Run fmt-check, lint-check, check-reachability, and coverage gates"
	@echo
	@echo "[Quality]"
	@echo "  fmt / fmt-check    Format and check (ruff format + ruff check)"
	@echo "  lint / lint-check  Lint (ruff check) and check-only"
	@echo "  check-reachability Fail on ml modules unreachable from any entrypoint"
	@echo
	@echo "[Docker]"
	@echo "  docker-build       Build Docker image ($(APP_NAME):latest)"
	@echo "  docker-run         Run Docker image exposing :8000"
	@echo
	@echo "[Maintenance]"
	@echo "  clean              Remove virtualenv (.venv)"
	@echo
	@echo "Variables (with defaults):"
	@echo "  MATCH=$(MATCH)  LINE_LENGTH=$(LINE_LENGTH)  COV_MIN=$(COV_MIN)"
	@echo "\nTip: use 'make list' to see all phony targets."

list:
	@awk '/^\.PHONY:/{for(i=2;i<=NF;i++)print $$i}' $(MAKEFILE_LIST) | sort -u
