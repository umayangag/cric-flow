
# Variables
APP_NAME:=ml-service
VENV:=.venv
PY:=$(VENV)/bin/python3
PIP:=$(VENV)/bin/pip
UVICORN:=$(VENV)/bin/uvicorn
LINE_LENGTH?=120
# Raise over time; current ~78%. CI and make coverage-check use this.
COV_MIN?=77

.PHONY: all venv install run train-models auto-tune export-pool docker-build docker-run clean fmt fmt-check init lint lint-check wrap wrap-check test coverage coverage-check ci ci-setup help list _install_test_tools _install_ruff

all: run

venv:
	@test -d $(VENV) || python3 -m venv $(VENV)

install: venv
	$(PIP) install --upgrade pip
	PYO3_USE_ABI3_FORWARD_COMPATIBILITY=1 $(PIP) install -r requirements.txt

# Initialize local Python development environment (venv + deps + dev tools)
init: install
	@echo "Setting up Python dev environment in $(VENV)..."
	$(PIP) install ruff pytest pytest-cov
	@echo "Done. Activate the venv: 'source $(VENV)/bin/activate' (or '.\\$(VENV)\\Scripts\\activate' on Windows)"
	@echo "Then run: 'make fmt-check' or 'make run'"

run:
	$(UVICORN) app.main:app --host 0.0.0.0 --port 8000 --reload

# Per-model training (artifacts go to ML_SERVICE_OUTPUT_DIR or outputs.artifacts_dir).
# Prerequisites: precompute (go-app) and export (batting/bowling CSVs); fielding needs GO_APP_URL + CUTOFF or --csv.
train-batting:
	$(PY) -m ml.train_batting --all-formats

train-bowling:
	$(PY) -m ml.train_bowling --all-formats

# Fielding: set GO_APP_URL and CUTOFF for API, or pass FIELDING_CSV=path when invoking from root.
train-fielding:
	$(PY) -m ml.train_fielding

train-models: train-batting train-bowling train-fielding

# Auto-tune model(s): find best algorithm and hyperparameters (see docs/ML_AUTO_TUNE.md).
# Usage: make auto-tune MODEL=batting FORMAT=T20; or make auto-tune MODEL=all ALL_FORMATS=1
MODEL ?= batting
FORMAT ?=
ALL_FORMATS ?=
AUTO_TUNE_ARGS := --model $(MODEL)
ifneq ($(FORMAT),)
AUTO_TUNE_ARGS += --format $(FORMAT)
endif
ifeq ($(ALL_FORMATS),1)
AUTO_TUNE_ARGS += --all-formats
endif
auto-tune: venv
	$(PY) -m ml.auto_tune $(AUTO_TUNE_ARGS)

# Generate ml/pool.csv for a given match id
# Usage: make export-pool MATCH=<match_id>
MATCH ?= 0
export-pool:
	@if [ "$(MATCH)" = "0" ]; then echo "Please pass MATCH=<match_id>, e.g., make export-pool MATCH=1193505"; exit 1; fi
	$(PY) ml/export_pool.py $(MATCH)

validate-exports:
	(cd ml && ../$(PY) validate_exports.py --all-formats --schema training --use-golden)

validate-exports-infer:
	(cd ml && ../$(PY) validate_exports.py --all-formats --schema inference)

# Format Python code with ruff (format + lint fix)
fmt: venv _install_ruff
	$(VENV)/bin/ruff format --line-length $(LINE_LENGTH) .
	$(VENV)/bin/ruff check --fix .

# Auto-fix lint (unused imports, etc.)
lint: venv _install_ruff
	$(VENV)/bin/ruff check --fix .

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
# Install dev tools needed in CI (system pip is fine on runners)
ci-setup:
	python -m pip install --upgrade pip
	python -m pip install -r requirements.txt
	python -m pip install ruff pytest pytest-cov httpx

# Helper to ensure pytest and pytest-cov are available in the project venv
_install_test_tools: install
	@$(PIP) show pytest >/dev/null 2>&1 || { echo "Installing pytest into $(VENV)..."; $(PIP) install pytest >/dev/null; }
	@$(PIP) show pytest-cov >/dev/null 2>&1 || { echo "Installing pytest-cov into $(VENV)..."; $(PIP) install pytest-cov >/dev/null; }

# Run tests (quiet) ensuring runtime deps are installed in venv
test: _install_test_tools
	$(VENV)/bin/pytest -q

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
ci: fmt-check lint-check coverage coverage-check


docker-build:
	docker build -t $(APP_NAME):latest .

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
	@echo "  init               Install dev tools (ruff, pytest, etc.)"
	@echo
	@echo "[Run & Train]"
	@echo "  run                Start FastAPI with uvicorn (reload)"
	@echo "  export-pool        Generate ml/pool.csv for a match (MATCH=<id>)"
	@echo "  train-batting      Train batting model (from exported CSVs)"
	@echo "  train-bowling      Train bowling model (from exported CSVs)"
	@echo "  train-fielding     Train fielding model (GO_APP_URL + CUTOFF or FIELDING_CSV)"
	@echo "  train-models      Train batting + bowling + fielding (fielding needs env or CSV)"
	@echo "  auto-tune          Auto-tune model(s): best algorithm + hyperparameters (MODEL=, FORMAT=, ALL_FORMATS=1)"
	@echo ""
	@echo "  Integration order: precompute (go-app) → export-dataset → train-* → run (or restart ML service)."
	@echo
	@echo "[Validate & Test]"
	@echo "  validate-exports   Validate training exports (all formats, golden)"
	@echo "  validate-exports-infer  Validate inference schemas"
	@echo "  test               Run pytest"
	@echo "  coverage           Run pytest with coverage (xml + term)"
	@echo "  coverage-check     Enforce coverage threshold (COV_MIN)"
	@echo "  ci                 Run fmt-check, lint-check, and coverage gates"
	@echo
	@echo "[Quality]"
	@echo "  fmt / fmt-check    Format and check (ruff format + ruff check)"
	@echo "  lint / lint-check  Lint (ruff check) and check-only"
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
