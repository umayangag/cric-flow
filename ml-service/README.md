# ML Service (Python)

Standalone FastAPI microservice for cricket ML predictions, backtests, match generation, and training orchestration. Feature precomputation is owned by the Go app; this service does not expose a `/precompute` endpoint.

## Layout

| Path | Role |
|------|------|
| `app/` | HTTP layer: FastAPI (`main.py`), Pydantic models, artifact loading, prediction orchestration, backtest cache |
| `ml/` | Training scripts, tuning, reconciliation solvers, config helpers, plus `ml/baselines` and `ml/datasets` (small helpers for offline tooling and fixtures) |
| `tests/` | Unit, integration, and gated e2e tests |
| `docs/` | Service docs including [IMPROVEMENT_PR_CHECKLIST.md](docs/IMPROVEMENT_PR_CHECKLIST.md) |

Key modules:

- `app/main.py` — routes, middleware, lifespan
- `app/prediction_service.py` — backtest and batch prediction pipelines
- `app/artifacts.py` — in-memory joblib registries and reload
- `ml/train_batting.py`, `ml/train_bowling.py` — primary training CLIs (`python -m ml.train_batting`, etc.)

**Prerequisites:** Python 3.10+ locally; CI and Docker use Python 3.12.

## One-time setup

Create a virtualenv and dev tools aligned with CI:

```bash
make init
source .venv/bin/activate    # Linux/macOS
# .\.venv\Scripts\Activate.ps1   # Windows PowerShell
```

This installs runtime deps from `requirements.txt` and dev tools (ruff, pytest).

## Configuration

Directory resolution precedence:

1. CLI args (`--csv`, `--out` for training)
2. Environment variables (`GO_APP_OUTPUT_DIR`, `ML_SERVICE_OUTPUT_DIR`, `MODELS_DIR`)
3. `ml-service/config.json` (merged over `config.default.json`)
4. Built-in defaults

Minimal `config.json` schema:

```json
{
  "inputs": { "go_app_export_dir": "../output/go-app" },
  "outputs": { "artifacts_dir": "../output/ml-service" }
}
```

See [../docs/config-and-data.md](../docs/config-and-data.md) for full details.

### Model training parameters (config only, per model)

Hyperparameters are read from config (no hidden code defaults). Edit `ml.training.<model>` in `config.json`:

| Model | Config path | Used by |
|-------|-------------|--------|
| Batting | `ml.training.batting` | `ml.train_batting`, train-on-the-fly (batting) |
| Bowling | `ml.training.bowling` | `ml.train_bowling`, train-on-the-fly (bowling) |

Each block includes `n_estimators`, `max_depth`, `random_state`, `joblib_compress` (0–9). Fielding, extras, win, and innings have separate blocks; see [../docs/ml-and-training.md](../docs/ml-and-training.md).

**Feature vectors:** Ordered names in `configs/feature_vectors.json` (shared with go-app). Override with `FEATURE_CONFIG_PATH`.

## Common tasks

Run locally on port 8000 with reload:

```bash
make run
```

Train artifacts from exported CSVs or go-app API:

```bash
make train-models
# or per model:
python3 -m ml.train_batting --all-formats
python3 -m ml.train_bowling --all-formats
```

Auto-tune: see **docs/ml-and-training.md** — e.g. `make auto-tune MODEL=batting FORMAT=T20`.

Docker:

```bash
make docker-build
make docker-run
```

From repo root: `make export-dataset` → `output/go-app/`. See [../docs/overview.md](../docs/overview.md).

## HTTP API

OpenAPI schema: `GET /openapi.json` when the service is running.

### Health and metadata

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Service status and whether artifacts are loaded |
| GET | `/artifacts/status` | Per-format artifact presence and load state |
| GET | `/model-metadata` | Feature/metadata from `feature_vectors.json` |
| GET | `/model-stats` | Trained model file stats (algorithm, size, mtime, etc.) |

### Player / match predictions (artifacts required unless train-on-the-fly)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/predict/batting` | `BattingFeatures[]` → `BattingPrediction[]` |
| POST | `/predict/bowling` | `BowlingFeatures[]` → `BowlingPrediction[]` |
| POST | `/predict/extras` | `ExtrasFeatures[]` → `ExtrasPrediction[]` |
| POST | `/predict/win` | `WinFeatures[]` → `WinPrediction[]` |
| POST | `/predict/win-enhanced` | Per-player team features → single `WinPrediction` |
| POST | `/optimize/team-selection` | Pool + constraints → optimized XI and win probability |

### Backtest and match generation (go-app integration)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/ml/backtest/predict` | Player or team baseline backtest at cutoff |
| POST | `/ml/backtest/predict-batch` | Batch player predictions (shared model load) |
| POST | `/ml/backtest/match` | Historical match backtest |
| POST | `/api/ml/generate-match` | Reconciled match: players, innings, win probability |

Backtest player mode requires `format` and per-player `features`. Train-on-the-fly needs `GO_APP_URL` when artifacts are missing.

### Admin (gated)

Requires `ENABLE_HOT_RELOAD=1` and `X-Admin-API-Key` when `ADMIN_API_KEY` is set.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/reload` | Rescan models directory and reload joblib artifacts |
| POST | `/admin/train/batting` | Run batting training (`?cutoff=` optional) |
| POST | `/admin/train/bowling` | Run bowling training |
| POST | `/admin/train/fielding` | Run fielding training |
| POST | `/admin/train/extras` | Run extras training (`cutoff` required) |
| POST | `/admin/train/win` | Run win training (`cutoff` required) |
| POST | `/admin/train/innings` | Run innings training (`cutoff` required) |
| POST | `/admin/train/auto-tune` | Hyperparameter search (`cutoff` required) |
| GET | `/admin/train/auto-tune/progress` | Auto-tune progress (API key only) |

## Model loading

Artifact directory precedence at startup:

1. `ML_SERVICE_OUTPUT_DIR`
2. `MODELS_DIR` (legacy)
3. `config.outputs.artifacts_dir`
4. Fallback: `../../output/ml-service`

Expected joblib files, always per-format (e.g. `batting_model_T20.joblib`):

- Batting / bowling: `*_scaler`, `*_model`, optional `*_output_scaler`
- Fielding, extras, win, innings: format-specific names per **docs/ml-and-training.md**

Reload at runtime: `POST /admin/reload` when hot reload is enabled.

## Formatting and CI checks

```bash
make fmt          # format + ruff fix
make fmt-check    # CI: format + lint check only
make test         # unit tests
make coverage     # tests + coverage report
```

Keep feature contracts in sync with the Go app. End-to-end workflow: repo root [README.md](../README.md).
