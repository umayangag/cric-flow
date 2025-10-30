# ML Service (Python)

This directory contains a standalone ML microservice that encapsulates the machine learning parts of the prototype. It keeps the original `src/` code intact for reference, while enabling an API-driven deployment for predictions.

Components (initial scaffold):
- `app/main.py`: FastAPI app exposing health and stub prediction endpoints for batting and bowling.
- `ml/encoders.py`: Copied encoders from the prototype.
- `ml/dataset_definitions.py`: Copied dataset/feature column contracts.
- `ml/queries.py`: Copied SQL queries used to assemble datasets (for reference).

Makefile targets:
- `make venv` — create a virtualenv.
- `make install` — install dependencies into venv.
- `make run` — run FastAPI with uvicorn on :8000.
- `make docker-build` — build docker image.
- `make docker-run` — run dockerized service on :8000.

Notes:
- Model training code from the prototype can be ported here in future steps; for now, endpoints are stubs ready to be wired with trained artifacts (e.g., `.pkl` and scalers).
- Keep feature contracts aligned with the Go app via shared schemas.
