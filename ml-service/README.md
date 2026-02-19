# ML Service (Python)

Standalone FastAPI microservice that serves predictions and utilities around the ML artifacts. Precomputation of features is now owned by the Go app; this service no longer exposes a /precompute endpoint.

Components:
- `app/main.py`: FastAPI app exposing health and prediction endpoints for batting and bowling.
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
This installs runtime deps from `requirements.txt` and dev tools (ruff, pytest).

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
See `../docs/config-and-data.md` for full details and examples.

### Model training parameters (config only, per model)
All training hyperparameters are read **strictly from config**; there are no magic defaults or env overrides in code. Edit `ml-service/config.json` under `ml.training.<model>` to tune each model independently:

| Model | Config path | Used by |
|-------|-------------|--------|
| Batting | `ml.training.batting` | train_batting_model, train_batting, train-on-the-fly (batting) |
| Bowling | `ml.training.bowling` | train_bowling_model, train_bowling, train-on-the-fly (bowling) |

Each block has: `n_estimators`, `max_depth`, `random_state`, `joblib_compress` (0–9). Example: set different `max_depth` for batting vs bowling, then run `make train-models`.

Additional models (fielding, extras, win) have their own config blocks and artifacts; see **docs/ml-and-training.md** for training data, training scripts, and how all models are combined for final prediction.

**Export (from repo root):** `make export-dataset` writes to `output/go-app/` (unified and per-format CSVs). See **docs/overview.md** and **docs/config-and-data.md**.

## Common tasks
- Run the service locally on :8000 with auto-reload:
```
make run
```
- Train artifacts from exported CSVs (uses defaults above):
```bash
make train-models
# or directly
python3 -m ml.train_batting_model
python3 -m ml.train_bowling_model
```
- Auto-tune models (find best algorithm and hyperparameters per model/format): see **docs/ml-and-training.md**. Example: `make auto-tune MODEL=batting FORMAT=T20` or `make auto-tune MODEL=all ALL_FORMATS=1`.
- Docker image and container:
```
make docker-build
make docker-run
```

## Endpoints
- `GET /health` → service status and whether artifacts are loaded
- `POST /predict/batting` → array of `BattingFeatures` rows → array of `BattingPrediction`
- `POST /predict/bowling` → array of `BowlingFeatures` rows → array of `BowlingPrediction`
- `POST /admin/reload` → reload artifacts (enable with `ENABLE_HOT_RELOAD=1`)

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



**Feature vectors:** Ordered names in `configs/feature_vectors.json` (shared with go-app). Override: `FEATURE_CONFIG_PATH`. See **docs/config-and-data.md**.
