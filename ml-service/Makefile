APP_NAME:=ml-service
VENV:=.venv
PY:=$(VENV)/bin/python3
PIP:=$(VENV)/bin/pip
UVICORN:=$(VENV)/bin/uvicorn
LINE_LENGTH?=120

.PHONY: all venv install run train-all export-pool docker-build docker-run clean fmt fmt-check init lint lint-check wrap wrap-check

all: run

venv:
	@test -d $(VENV) || $(PY) -m venv $(VENV)

install: venv
	$(PIP) install --upgrade pip
	$(PIP) install -r requirements.txt

# Initialize local Python development environment (venv + deps + dev tools)
init: install
	@echo "Setting up Python dev environment in $(VENV)..."
	$(PIP) install black isort flake8 ruff
	@echo "Done. Activate the venv: 'source $(VENV)/bin/activate' (or '.\\$(VENV)\\Scripts\\activate' on Windows)"
	@echo "Then run: 'make fmt-check' or 'make run'"

run:
	$(UVICORN) app.main:app --host 0.0.0.0 --port 8000 --reload

train-all:
	$(PY) ml/train_batting_model.py
	$(PY) ml/train_bowling_model.py

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

# Format Python code with line wrapping (isort + black)
fmt:
	make lint
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	$(VENV)/bin/isort --profile black --line-length $(LINE_LENGTH) .
	$(VENV)/bin/black --line-length $(LINE_LENGTH) .
	flake8 --max-line-length=$(LINE_LENGTH) --extend-ignore=E203,W503 --exclude=.venv

# Auto-fix lint: remove unused imports via ruff (F401)
lint:
	@command -v ruff >/dev/null 2>&1 || (echo "Install ruff: pip install ruff" && exit 1)
	$(VENV)/bin/ruff check --fix --select F401 .

# Lint check only (CI-safe)
lint-check:
	@command -v ruff >/dev/null 2>&1 || (echo "Install ruff: pip install ruff" && exit 1)
	$(VENV)/bin/ruff check --select F401 .

# Check formatting only (same as CI)
fmt-check:
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	$(VENV)/bin/isort --check-only --diff --profile black --line-length $(LINE_LENGTH) .
	$(VENV)/bin/black --check --diff --line-length $(LINE_LENGTH) .
	flake8 --max-line-length=120 --extend-ignore=E203,E501,W503 --exclude=.venv

docker-build:
	docker build -t $(APP_NAME):latest .

docker-run:
	docker run --rm -p 8000:8000 --name $(APP_NAME) $(APP_NAME):latest

clean:
	rm -rf $(VENV)
