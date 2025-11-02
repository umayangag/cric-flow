# ML Service (Python)

Standalone FastAPI microservice that serves predictions and utilities around the ML artifacts. It now also handles feature precomputation. The original `src/` prototype remains for reference.

Components:
- `app/main.py`: FastAPI app exposing health, prediction endpoints for batting and bowling, and a `/precompute` endpoint for feature calculation.
- `ml/calculate_features.py`: Script to calculate and store player features (form, consistency, venue, opposition) in the database.
- `ml/export_pool.py`: Script to generate `pool.csv` for team prediction.
- `ml/train_batting_model.py`: Script to train the batting prediction model.
- `ml/train_bowling_model.py`: Script to train the bowling prediction model.
- `ml/*`: encoders, dataset/feature definitions, and other utility scripts.

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
- Precompute features (calls the `/precompute` endpoint of the ML service):
```
make precompute
```
- Train artifacts from exported CSVs (uses defaults above):
```
make train-all
# or directly
python -m ml.train_batting_model
python -m ml.train_bowling_model
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

Expected files (can be format-specific, e.g., `batting_model_ODI.joblib`):
- `batting_scaler.joblib`, `batting_model.joblib`, `batting_output_scaler.joblib`
- `bowling_scaler.joblib`, `bowling_model.joblib`, `bowling_output_scaler.joblib`

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
