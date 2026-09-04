# ML Service (Python)

Standalone FastAPI microservice for cricket ML: XI selection, win probability, the player
performance model, the match simulator, the L4 harness, and the retrain / evaluate steps.
Every feature the models read is computed here, by one as-of pass over the event store.

## Layout

| Path | Role |
|------|------|
| `app/` | HTTP layer: FastAPI (`main.py`), Pydantic models, artifact loading, XI serving (`xi_service.py`) |
| `ml/` | `ml/xi/` — the rating pass, the win models, the performance model, the simulator, the L4 harness, and run identity (`runs.py`) |
| `tests/` | Unit, integration, and gated e2e tests |

Key modules:

- `app/main.py` — routes, middleware, lifespan
- `app/xi_service.py` — selection, win probability, performance, simulation, L4's report
- `ml/xi/retrain.py` — the `retrain` command: one run, its artifacts and its manifest
- `ml/xi/runs.py` — run identity (H-16): the manifest, the `current` pointer, the refusal (D-6)
- `ml/xi/evaluate.py` — the L4 harness (`make evaluate`)
- `ml/xi/market.py` — X-4's market benchmark: closing odds scored beside the display model
  inside the harness. A yardstick only; nothing that fits or serves a model may import it

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

1. CLI args (`--out` for retrain and evaluate)
2. Environment variables (`ML_SERVICE_OUTPUT_DIR`, `MODELS_DIR`, `XI_RATINGS_MAX_AGE_DAYS`)
3. `ml-service/config.json` (merged over `config.default.json`)
4. Built-in defaults

The whole `config.json`:

```json
{
  "inputs": { "training_subprocess_timeout_sec": 604800 },
  "outputs": { "artifacts_dir": "../output/ml-service" },
  "ml": { "formats": ["TEST", "ODI", "T20", "T20I"], "ratings_max_age_days": 14 }
}
```

**There are no model hyperparameters here.** They are the three-point grid in
`ml.xi.train.DISPLAY_GRID`, chosen inside the training rows and recorded per run in
`manifest.json`. A table of tuned parameters nothing could join back to an artifact was how a
model came to carry parameters from a search it had never seen.

See [../docs/config-and-data.md](../docs/config-and-data.md) for full details.

## Common tasks

Run locally on port 8000 with reload:

```bash
make run
```

Build a run (the rating pass, the win models and the performance models), serve it, and score it:

```bash
make retrain CUTOFF=2025-09-01   # writes runs/<run_id>/, publishes nothing
make reload                      # points current at it and loads it
make evaluate                    # L4 over the database; touches no artifact current points at
make evaluate MARKET_ODDS_DIR=../data/market-odds   # …with X-4's market benchmark beside it
```

Docker:

```bash
make docker-build
make docker-run
```

From repo root: `make up-all CUTOFF=<date>` is the whole chain from an empty database. See
[../docs/overview.md](../docs/overview.md).

## HTTP API

OpenAPI schema: `GET /openapi.json` when the service is running.

### Health and metadata

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Alive, which run is loaded, and whether its ratings are fresh enough to answer with |
| GET | `/artifacts/status` | Every run on disk, which one `current` names, which is loaded, and the loader's refusal when there is one |

### Selection and prediction (the XI layer)

Every one of these takes player ids and a format, never a feature map: the rating state lives
here, which is what makes the training and serving paths compute the same function of the same
eleven names.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/xi/status` | The loaded run and its manifest (H-16), loaded formats, how far the ratings run and whether they are stale (H-11), the run's own report |
| GET | `/xi/evaluate-report` | L4's evaluation report (`xi_evaluate_report.json`) |
| POST | `/xi/optimize` | Pool + constraints → XI. `objective: "win"` maximises P(win); `objective: "ratings"` is the rating-ordered pick, the only mode offered where the format is not optimised — because the objective does not rank (H-17) or has not shown it selects (E5) — with the reason in the 503 |
| POST | `/xi/predict-win` | Two elevens → displayed P(team1 wins) |
| POST | `/performance/predict` | Two elevens → per-player distributions (L2-B) |
| POST | `/simulate` | Two elevens → totals, per-player ranges, the median-band scorecard and P(win), all from one set of draws (L2-C). Limited-overs formats only |

A live request (no `as_of`) against ratings older than `ml.ratings_max_age_days` answers
**503 `RATINGS_STALE`** rather than predicting from a squad that has moved on (H-11).

### Admin (gated)

Requires `ENABLE_HOT_RELOAD=1` and `X-Admin-API-Key` when `ADMIN_API_KEY` is set.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/reload?run=<id>` | Point `current` at a run and load it. Without `run`: the run `current` names, or the newest. 409 `RUN_ARTIFACTS_INVALID` when the run is not one, or its arrays are not the arrays this code reads |
| POST | `/admin/train/retrain?cutoff=` | Build one run (`cutoff` required). Publishes nothing |
| POST | `/admin/train/evaluate` | Run L4 and write its report |
| GET | `/admin/train/progress?step=` | Live progress for a step (API key only) |

## Model loading

Artifact directory precedence at startup:

1. `ML_SERVICE_OUTPUT_DIR`
2. `MODELS_DIR` (legacy)
3. `config.outputs.artifacts_dir`
4. Fallback: `../../output/ml-service`

Under that root:

```
current_run.json                    {"run_id": "...", "updated_at": "..."}
runs/<run_id>/
  manifest.json                     what the run trained on and what it chose (H-16)
  xi_ratings.joblib
  xi_win_<FMT>.joblib
  xi_perf_<FMT>.joblib
  xi_win_report.json
```

At startup, and on `POST /admin/reload`, the service loads the run `current` names — or the
newest one if nothing does, publishing it. A directory with no manifest, or one whose arrays are
not the arrays this code reads, is **refused** with an error naming the run rather than loaded
(D-6); the refusal reaches `/xi/status`, `/health` and `/artifacts/status`.

## Formatting and CI checks

```bash
make fmt          # format + ruff fix
make fmt-check    # CI: format + lint check only
make test         # unit tests
make coverage     # tests + coverage report
```

`make check-reachability` fails on any module no live entrypoint can reach. End-to-end workflow:
repo root [README.md](../README.md).
