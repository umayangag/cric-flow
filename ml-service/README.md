# ML Service (Python)

Standalone FastAPI microservice for cricket ML: XI selection, win probability, the player
performance model, the match simulator, and training orchestration. Feature precomputation
is owned by the Go app; this service does not expose a `/precompute` endpoint.

## Layout

| Path | Role |
|------|------|
| `app/` | HTTP layer: FastAPI (`main.py`), Pydantic models, artifact loading, XI serving (`xi_service.py`) |
| `ml/` | `ml/xi/` — the rating pass, the win models, the performance model, the simulator and the L4 harness; plus the windowed-form win trainer and the tuning stack, which P-6 removes |
| `tests/` | Unit, integration, and gated e2e tests |
| `docs/` | Service docs including [IMPROVEMENT_PR_CHECKLIST.md](docs/IMPROVEMENT_PR_CHECKLIST.md) |

Key modules:

- `app/main.py` — routes, middleware, lifespan
- `app/xi_service.py` — selection, win probability, performance, simulation, L4's report
- `app/artifacts.py` — in-memory joblib registries and reload
- `ml/xi/train.py`, `ml/xi/evaluate.py` — the training and evaluation CLIs (`make train-xi`, `make xi-evaluate`)

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
| Win (windowed form) | `ml.training.win` | `ml.train_win`, auto-tune |

The block includes `n_estimators`, `max_depth`, `random_state`, `joblib_compress` (0–9). The XI
models read their hyperparameters from `ml/xi/`; see [../docs/ml-and-training.md](../docs/ml-and-training.md).

## Common tasks

Run locally on port 8000 with reload:

```bash
make run
```

Train the XI models (the rating pass, the win models and the performance models) and score them:

```bash
make train-xi CUTOFF=2025-09-01
make xi-evaluate
```

Auto-tune the windowed-form win model: see **docs/ml-and-training.md** — e.g. `make auto-tune FORMAT=T20`.

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
| GET | `/model-metadata` | The win model's feature order, output and artifact naming |
| GET | `/model-stats` | Trained model file stats (algorithm, size, mtime, etc.) |

### Selection and prediction (the XI layer)

Every one of these takes player ids and a format, never a feature map: the rating state lives
here, which is what makes the training and serving paths compute the same function of the same
eleven names.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/xi/status` | Loaded formats, how far the ratings run, the last training report |
| GET | `/xi/evaluate-report` | L4's evaluation report (`xi_evaluate_report.json`) |
| POST | `/xi/optimize` | Pool + constraints → XI. `objective: "win"` maximises P(win); `objective: "ratings"` is the rating-ordered pick, the only mode offered where the objective does not rank (H-17) |
| POST | `/xi/predict-win` | Two elevens → displayed P(team1 wins) |
| POST | `/performance/predict` | Two elevens → per-player distributions (L2-B) |
| POST | `/simulate` | Two elevens → totals, per-player ranges, the median-band scorecard and P(win), all from one set of draws (L2-C). Limited-overs formats only |

### Windowed-form win model (P-6 removes it)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/predict/win` | `WinFeatures[]` → `WinPrediction[]` |
| POST | `/predict/win-enhanced` | Per-player team features → single `WinPrediction` |

### Admin (gated)

Requires `ENABLE_HOT_RELOAD=1` and `X-Admin-API-Key` when `ADMIN_API_KEY` is set.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/reload` | Rescan models directory and reload joblib artifacts |
| POST | `/admin/train/win` | Run win training (`cutoff` required) |
| POST | `/admin/train/auto-tune` | Hyperparameter search (`cutoff` required) |
| GET | `/admin/train/auto-tune/progress` | Auto-tune progress (API key only) |

## Model loading

Artifact directory precedence at startup:

1. `ML_SERVICE_OUTPUT_DIR`
2. `MODELS_DIR` (legacy)
3. `config.outputs.artifacts_dir`
4. Fallback: `../../output/ml-service`

Expected joblib files:

- Win (windowed form): `win_model_<FMT>.joblib`, with `win_model_<FMT>_metadata.json` beside it
- XI layer: `xi_ratings.joblib`, `xi_win_<FMT>.joblib`, `xi_perf_<FMT>.joblib` — loaded by
  `ml.xi.store`, not by the per-format registry above

Reload at runtime: `POST /admin/reload` when hot reload is enabled.

## Formatting and CI checks

```bash
make fmt          # format + ruff fix
make fmt-check    # CI: format + lint check only
make test         # unit tests
make coverage     # tests + coverage report
```

Keep feature contracts in sync with the Go app. End-to-end workflow: repo root [README.md](../README.md).
