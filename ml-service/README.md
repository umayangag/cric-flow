# ML Service (Python)

Standalone FastAPI microservice that serves predictions and utilities around the ML artifacts. The original `src/` prototype remains for reference.

Components:
- `app/main.py`: FastAPI app exposing health and prediction endpoints for batting and bowling.
- `ml/*`: encoders, dataset/feature definitions, training scripts.

Prerequisites:
- Python 3.10+

## One-time setup
Create and prepare a local virtualenv, plus dev tools aligned with CI:
```
make init
# then activate it in your shell
source .venv/bin/activate    # Linux/macOS
# or on Windows (PowerShell):
# .\\.venv\\Scripts\\Activate.ps1
```
This installs runtime deps from `requirements.txt` and dev tools `black`, `isort`, `flake8`.

## Common tasks (Makefile)
- Run the service locally on :8000 with auto-reload:
```
make run
```
- Docker image and container:
```
make docker-build
make docker-run
```

## Formatting and checks
- Format code to match CI (reads `pyproject.toml`):
```
make fmt
```
- Check formatting only (fails on diff), mirrors CI:
```
make fmt-check
```

Notes:
- Keep feature contracts in sync with the Go app.
- See repo root `README.md` for the end-to-end workflow (export datasets, train models, run full stack).
