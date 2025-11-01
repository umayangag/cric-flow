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

## Configuration
Default directories are resolved with this precedence:
1. CLI args (`--csv`, `--out` for training)
2. Environment variables (`GO_APP_OUTPUT_DIR`, `ML_SERVICE_OUTPUT_DIR`)
3. `ml-service/config.json`
4. Built-in defaults

Schema for `ml-service/config.json`:
```json
{
  "inputs": { "go_app_export_dir": "../output/go-app" },
  "outputs": { "artifacts_dir": "../output/ml-service" }
}
```
See `../docs/CONFIG.md` for full details and examples.

## Common tasks (Makefile)
- Run the service locally on :8000 with auto-reload:
```
make run
```
- Train artifacts from exported CSVs (uses defaults above):
```
make -C .. train-batting
make -C .. train-bowling
# or directly
python -m ml.train_batting --csv $(GO_APP_OUTPUT_DIR)/batting_encoded.csv --out $(ML_SERVICE_OUTPUT_DIR)
python -m ml.train_bowling --csv $(GO_APP_OUTPUT_DIR)/bowling_encoded.csv --out $(ML_SERVICE_OUTPUT_DIR)
```
- Docker image and container:
```
make docker-build
make docker-run
```

## Model loading
At startup the service looks for artifacts in this order:
1. `ML_SERVICE_OUTPUT_DIR`
2. `MODELS_DIR` (legacy)
3. `config.outputs.artifacts_dir` from `ml-service/config.json`
4. Built-in fallback: `../../output/ml-service`

Expected files:
- `batting_scaler.joblib`, `batting_model.joblib`
- `bowling_scaler.joblib`, `bowling_model.joblib`

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
