APP_NAME:=ml-service
VENV:=.venv
PY:=python3
PIP:=$(VENV)/bin/pip
UVICORN:=$(VENV)/bin/uvicorn
LINE_LENGTH?=100

.PHONY: all venv install run docker-build docker-run clean fmt fmt-check init lint lint-check wrap wrap-check

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

# Format Python code with line wrapping (isort + black)
fmt:
	make lint
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	isort --profile black --line-length $(LINE_LENGTH) .
	black --line-length $(LINE_LENGTH) .
	flake8 --max-line-length=120 --extend-ignore=E203,W503

# Auto-fix lint: remove unused imports via ruff (F401)
lint:
	@command -v ruff >/dev/null 2>&1 || (echo "Install ruff: pip install ruff" && exit 1)
	ruff check --fix --select F401 .

# Lint check only (CI-safe)
lint-check:
	@command -v ruff >/dev/null 2>&1 || (echo "Install ruff: pip install ruff" && exit 1)
	ruff check --select F401 .

# Check formatting only (same as CI)
fmt-check:
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	isort --check-only --diff --profile black --line-length $(LINE_LENGTH) .
	black --check --diff --line-length $(LINE_LENGTH) .
	flake8 --max-line-length=120 --extend-ignore=E203,W503

docker-build:
	docker build -t $(APP_NAME):latest .

docker-run:
	docker run --rm -p 8000:8000 --name $(APP_NAME) $(APP_NAME):latest

clean:
	rm -rf $(VENV)
