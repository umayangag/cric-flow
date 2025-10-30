APP_NAME:=ml-service
VENV:=.venv
PY:=python3
PIP:=$(VENV)/bin/pip
UVICORN:=$(VENV)/bin/uvicorn

.PHONY: all venv install run docker-build docker-run clean

all: run

venv:
	@test -d $(VENV) || $(PY) -m venv $(VENV)

install: venv
	$(PIP) install --upgrade pip
	$(PIP) install -r requirements.txt

run:
	$(UVICORN) app.main:app --host 0.0.0.0 --port 8000 --reload

docker-build:
	docker build -t $(APP_NAME):latest .

docker-run:
	docker run --rm -p 8000:8000 --name $(APP_NAME) $(APP_NAME):latest

clean:
	rm -rf $(VENV)
