APP_NAME:=ml-service
VENV:=.venv
PY:=python3
PIP:=$(VENV)/bin/pip
UVICORN:=$(VENV)/bin/uvicorn

.PHONY: all venv install run docker-build docker-run clean fmt fmt-check init

all: run

venv:
	@test -d $(VENV) || $(PY) -m venv $(VENV)

install: venv
	$(PIP) install --upgrade pip
	$(PIP) install -r requirements.txt

# Initialize local Python development environment (venv + deps + dev tools)
init: install
	@echo "Setting up Python dev environment in $(VENV)..."
	$(PIP) install black isort flake8
	@echo "Done. Activate the venv: 'source $(VENV)/bin/activate' (or '.\\$(VENV)\\Scripts\\activate' on Windows)"
	@echo "Then run: 'make fmt-check' or 'make run'"

run:
	$(UVICORN) app.main:app --host 0.0.0.0 --port 8000 --reload

# Format Python code to align with CI (isort + black, reads pyproject.toml)
fmt:
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	isort .
	black .

# Check formatting only (same as CI)
fmt-check:
	@command -v isort >/dev/null 2>&1 || (echo "Install isort: pip install isort" && exit 1)
	@command -v black >/dev/null 2>&1 || (echo "Install black: pip install black" && exit 1)
	isort --check-only --diff .
	black --check --diff .

docker-build:
	docker build -t $(APP_NAME):latest .

docker-run:
	docker run --rm -p 8000:8000 --name $(APP_NAME) $(APP_NAME):latest

clean:
	rm -rf $(VENV)
