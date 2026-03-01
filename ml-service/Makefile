
# Variables
APP_NAME:=ml-service
VENV:=.venv
PY:=$(VENV)/bin/python3
PIP:=$(VENV)/bin/pip
UVICORN:=$(VENV)/bin/uvicorn
LINE_LENGTH?=120
# Enforced by CI and make coverage-check. Raise as untested CLI/training modules get tests.
COV_MIN?=60

.PHONY: all venv install compile-requirements run train-batting train-bowling train-fielding train-extras train-win train-models
.PHONY: auto-tune walk-forward train-combination-meta validate-exports validate-exports-infer
.PHONY: docker-build docker-run clean fmt fmt-check init lint lint-check test test-integration test-e2e
.PHONY: coverage coverage-check ci ci-setup help list _install_test_tools _install_ruff

all: run

venv:
	@test -d $(VENV) || python3 -m venv $(VENV)

install: venv
	$(PIP) install --upgrade pip
	PYO3_USE_ABI3_FORWARD_COMPATIBILITY=1 $(PIP) install -r requirements.txt

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

# Per-model training (artifacts go to ML_SERVICE_OUTPUT_DIR or outputs.artifacts_dir).
# Batting/bowling: if CUTOFF is set (e.g. make train-batting CUTOFF=2025-01-01T00:00:00Z), use API; else read from CSVs.
# Fielding: set GO_APP_URL and CUTOFF for API, or pass FIELDING_CSV=path when invoking from root.
train-batting:
	$(if $(CUTOFF),$(PY) -m ml.train_batting --from-api --cutoff "$(CUTOFF)" --all-formats,$(PY) -m ml.train_batting --all-formats)

train-bowling:
	$(if $(CUTOFF),$(PY) -m ml.train_bowling --from-api --cutoff "$(CUTOFF)" --all-formats,$(PY) -m ml.train_bowling --all-formats)

# Fielding: set GO_APP_URL and CUTOFF for API, or pass FIELDING_CSV=path when invoking from root.
train-fielding:
	$(PY) -m ml.train_fielding

# Extras and win: same as fielding (GO_APP_URL + CUTOFF or --csv from root).
train-extras:
	$(PY) -m ml.train_extras

train-win:
	$(PY) -m ml.train_win

train-models: train-batting train-bowling train-fielding train-extras train-win

# Auto-tune model(s): find best algorithm and hyperparameters (see docs/ml-and-training.md).
# Usage: make auto-tune MODEL=batting FORMAT=T20; or make auto-tune MODEL=all ALL_FORMATS=1
# When MODEL=all and ALL_FORMATS=1, uses --from-api and CUTOFF so all five models (incl. extras, win) are tuned and params saved to DB when GO_APP_URL is set.
MODEL ?= batting
FORMAT ?=
ALL_FORMATS ?=
UNIFIED ?=
CUTOFF ?= 2024-01-01T00:00:00Z
ALGORITHMS ?=
VALIDATION_METHOD ?=
FAST ?=
NO_PYCARET ?=
RESCREEN ?=
AUTO_TUNE_ARGS := --model $(MODEL)
ifneq ($(FORMAT),)
AUTO_TUNE_ARGS += --format $(FORMAT)
endif
ifeq ($(ALL_FORMATS),1)
AUTO_TUNE_ARGS += --all-formats
ifeq ($(MODEL),all)
AUTO_TUNE_ARGS += --from-api --cutoff "$(CUTOFF)"
endif
endif
ifeq ($(UNIFIED),1)
AUTO_TUNE_ARGS += --unified
# extras and win require API; model=all includes them
ifneq ($(filter extras win all,$(MODEL)),)
AUTO_TUNE_ARGS += --from-api --cutoff "$(CUTOFF)"
endif
endif
ifneq ($(ALGORITHMS),)
AUTO_TUNE_ARGS += --algorithms "$(ALGORITHMS)"
endif
ifneq ($(VALIDATION_METHOD),)
AUTO_TUNE_ARGS += --validation-method $(VALIDATION_METHOD)
endif
ifeq ($(PARALLEL),1)
AUTO_TUNE_ARGS += --parallel
endif
ifeq ($(FAST),1)
AUTO_TUNE_ARGS += --fast
endif
ifeq ($(NO_PYCARET),1)
AUTO_TUNE_ARGS += --no-pycaret
endif
ifeq ($(NO_AUTOGLUON),1)
AUTO_TUNE_ARGS += --no-autogluon
endif
ifeq ($(RESCREEN),1)
AUTO_TUNE_ARGS += --rescreen
endif
auto-tune: venv
	$(PY) -m ml.auto_tune $(AUTO_TUNE_ARGS)

# Walk-forward training and evaluation (sequential data). Requires GO_APP_URL and go-app running.
# Usage: make walk-forward INITIAL_CUTOFF=2020-01-01 WINDOW_X=50 WALK_FORMAT=T20 WALK_MODEL=batting
INITIAL_CUTOFF ?= 2020-01-01T00:00:00Z
WINDOW_X ?= 50
WALK_FORMAT ?= T20
WALK_MODEL ?= batting
MAX_WINDOWS ?=
EXPORT_METRICS ?=
walk-forward: venv
	$(PY) -m ml.walk_forward --initial-cutoff "$(INITIAL_CUTOFF)" --window-x $(WINDOW_X) --format $(WALK_FORMAT) --model $(WALK_MODEL) $(if $(MAX_WINDOWS),--max-windows $(MAX_WINDOWS),) $(if $(EXPORT_METRICS),--export-metrics $(EXPORT_METRICS),)

# Train combination meta-model (Ridge) from backtest CSV. See docs/ml-and-training.md.
# Usage: make train-combination-meta CSV=path/to/backtest.csv OUT=output/ml-service/combination_meta.json
CSV ?=
OUT ?=
train-combination-meta: venv
	@if [ -z "$(CSV)" ] || [ -z "$(OUT)" ]; then echo "Usage: make train-combination-meta CSV=<path> OUT=<path>"; exit 1; fi
	$(PY) -m ml.train_combination_meta --csv "$(CSV)" --out "$(OUT)"

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
ci: fmt-check lint-check coverage coverage-check


# Build from repo root so configs/feature_vectors.json is available (app/feature_config.py).
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
	@echo "  train-batting      Train batting model (from exported CSVs)"
	@echo "  train-bowling      Train bowling model (from exported CSVs)"
	@echo "  train-fielding    Train fielding (GO_APP_URL + CUTOFF or FIELDING_CSV)"
	@echo "  train-extras      Train extras model (GO_APP_URL + CUTOFF or EXTRAS_CSV)"
	@echo "  train-win         Train win model (GO_APP_URL + CUTOFF or WIN_CSV)"
	@echo "  train-models      Train all (batting, bowling, fielding, extras, win)"
	@echo "  auto-tune         Auto-tune: best algorithm + hyperparams (MODEL=, FORMAT=, ALL_FORMATS=1)"
	@echo ""
	@echo "  Pipeline: precompute (go-app) → export-dataset → train-* → run (or restart ML service)."
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
