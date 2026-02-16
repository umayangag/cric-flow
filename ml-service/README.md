# ML Service (Python)

Standalone FastAPI microservice that serves predictions and utilities around the ML artifacts. Precomputation of features is now owned by the Go app; this service no longer exposes a /precompute endpoint. The original `src/` prototype remains for reference.

Components:
- `app/main.py`: FastAPI app exposing health and prediction endpoints for batting and bowling.
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

### Model training parameters (config only, per model)
All training hyperparameters are read **strictly from config**; there are no magic defaults or env overrides in code. Edit `ml-service/config.json` under `ml.training.<model>` to tune each model independently:

| Model | Config path | Used by |
|-------|-------------|--------|
| Batting | `ml.training.batting` | train_batting_model, train_batting, train-on-the-fly (batting) |
| Bowling | `ml.training.bowling` | train_bowling_model, train_bowling, train-on-the-fly (bowling) |

Each block has: `n_estimators`, `max_depth`, `random_state`, `joblib_compress` (0–9). Example: set different `max_depth` for batting vs bowling, then run `make train-all`.

Additional models (fielding, extras, win) have their own config blocks and artifacts; see **docs/ML_MODELS_COMBINED.md** for training data, training scripts, and how all models are combined for final prediction.

## Unified cross-format datasets (new)
The Go exporter now emits unified, cross-format CSVs that include leakage-free, time-indexed (as-of) per-format features for TEST/ODI/T20I/T20.

Export them from the repo root:
```
make export-dataset
# writes to output/go-app/batting_encoded_all.csv and bowling_encoded_all.csv
```
Notes:
- For backward compatibility, the Makefile also generates legacy files `batting_encoded.csv` and `bowling_encoded.csv` which current training scripts read by default.
- If you switch training to the unified files, update the dataset path arguments or scripts accordingly.

## Common tasks (Makefile)
- Run the service locally on :8000 with auto-reload:
```
make run
```
- Train artifacts from exported CSVs (uses defaults above):
```bash
make train-all
# or directly
python3 -m ml.train_batting_model
python3 -m ml.train_bowling_model
```
- Generate a player pool CSV for team prediction (writes to `ml/pool.csv`):
```bash
# requires DB to be populated and accessible via env (POSTGRES_*)
make export-pool MATCH=1193505
# or directly
python3 -m ml.export_pool 1193505
```
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



## Centralized feature vectors (shared config)
This service constructs input vectors based on a single shared configuration file stored at `../configs/feature_vectors.json`. The file defines ordered lists of feature names for `batting` and `bowling`. The loader in `app/feature_config.py` reads this file and returns the order at runtime.

- Override path via environment:
  - `FEATURE_CONFIG_PATH=../configs/feature_vectors.json`
- Behavior when missing/invalid:
  - If the file is missing or malformed, the service will fail fast with a clear error. Set `FEATURE_CONFIG_PATH` or ensure `configs/feature_vectors.json` exists and is valid.
- Interop with Go:
  - The Go app reads and validates the same file via `go-app/internal/featurecfg` against its `internal/contracts` JSON tags, ensuring both sides use an identical order.

Example (override temporarily for experiments):
```
FEATURE_CONFIG_PATH=$(pwd)/configs/feature_vectors.json pytest -q -k feature_config
```
