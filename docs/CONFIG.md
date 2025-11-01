# Configuration Reference

This document describes how file paths and configurable options are resolved across the monorepo. Both components (go-app and ml-service) use the same precedence rules to resolve directories and options.

- Highest precedence to lowest:
  1. CLI flags/args (e.g., `-dir`, `-out`, `--csv`, `--out`)
  2. Environment variables (e.g., `GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`, `ML_SERVICE_OUTPUT_DIR`)
  3. Component config file (`go-app/config.json` or `ml-service/config.json`)
  4. Built-in defaults (checked into code for resiliency)

- Directory conventions:
  - Inputs are expected under `data/{package}/...`
    - Examples: `data/go-app/cricsheet`, `data/go-app/createdb`
  - Outputs are written under `output/{package}/...`
    - Examples: `output/go-app`, `output/ml-service`

## Go App (go-app)

Config file location search order:
- `GO_APP_CONFIG` (if set)
- `./config.json` (inside `go-app/`)
- `../config.json`
- `../../config.json`

Schema for `go-app/config.json`:
```json
{
  "inputs": {
    "cricsheet_dir": "../data/go-app/cricsheet",
    "etl_dir": "../data/go-app/createdb"
  },
  "outputs": {
    "export_dir": "../output/go-app"
  }
}
```

Key environment variables:
- `GO_APP_INPUT_DIR`: Default input directory for importers (`cricsheet-importer`, `etl-importer`).
- `GO_APP_OUTPUT_DIR`: Default output directory for `export-dataset`.

Common CLI flags:
- `cricsheet-importer`: `-dir` points to a directory with Cricsheet `.json` files.
- `etl-importer`: `-dir` points to a directory containing curated CSVs.
- `export-dataset`: `-out` points to the output directory for exported CSVs.

Examples:
- Use config defaults (no env/flags):
  - `make cricsheet-import`
  - `make etl-importer`
  - `make export-dataset`
- Override via env for one run:
  - `cd go-app && GO_APP_INPUT_DIR=../data/custom go run ./cmd/etl-importer`
  - `cd go-app && GO_APP_OUTPUT_DIR=../output/go-alt go run ./cmd/export-dataset`
- Override via flags:
  - `cd go-app && go run ./cmd/cricsheet-importer -dir=../data/go-app/cricsheet`
  - `cd go-app && go run ./cmd/export-dataset -out=../output/go-app`

## ML Service (ml-service)

Config file location search order:
- `ML_SERVICE_CONFIG` (if set)
- `./config.json` (inside `ml-service/`)
- `../config.json`

Schema for `ml-service/config.json`:
```json
{
  "inputs": {
    "go_app_export_dir": "../output/go-app"
  },
  "outputs": {
    "artifacts_dir": "../output/ml-service"
  }
}
```

Key environment variables:
- `GO_APP_OUTPUT_DIR`: Where training scripts look for exported CSV datasets. If not set, the config file value is used.
- `ML_SERVICE_OUTPUT_DIR`: Where training scripts save artifacts and where the service loads model/scaler files from. If not set, the config file value is used.
- `MODELS_DIR`: Optional legacy variable checked by the service after `ML_SERVICE_OUTPUT_DIR`.

Common CLI args (training scripts):
- `python -m ml.train_batting --csv <path/to/batting_encoded.csv> --out <artifacts_dir>`
- `python -m ml.train_bowling --csv <path/to/bowling_encoded.csv> --out <artifacts_dir>`

Default resolution in training scripts:
- CSV input directory: `${GO_APP_OUTPUT_DIR}` if set, otherwise `config.inputs.go_app_export_dir` from `ml-service/config.json`.
- Artifacts output directory: `${ML_SERVICE_OUTPUT_DIR}` if set, otherwise `config.outputs.artifacts_dir` from `ml-service/config.json`.

Default resolution in the FastAPI app (`app/main.py`):
- Models directory: `${ML_SERVICE_OUTPUT_DIR}` if set, otherwise `${MODELS_DIR}` if set, otherwise `config.outputs.artifacts_dir` from `ml-service/config.json`, and finally a built-in fallback `../../output/ml-service`.

## Tips
- Keep paths relative to each component directory to simplify local development.
- For CI systems, prefer environment variables to avoid editing config files in the repository.
- The Makefile targets rely on sensible defaults; override with env vars when needed.
