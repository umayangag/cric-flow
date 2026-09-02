# System overview

Architecture, data flow, and how to run the pipeline. For a concise data-flow and ML-shape reference, see [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## Components

- **Data:** Cricsheet JSON under `data/go-app/cricsheet`.
- **go-app (Go):**
  - `cmd/migrate` — apply SQL migrations
  - `cmd/cricsheet-importer` — import Cricsheet JSON into Postgres
  - `cmd/api` — HTTP API and pipeline orchestration
  - `cmd/print_canonical` — print the canonical format codes
- **Postgres** — system of record
- **ml-service (Python):**
  - `ml/xi/` — the rating pass, the win models, the performance model, the simulator, the L4 harness
  - `ml/xi/retrain.py` — the `retrain` step: one run, artifacts and manifest
  - `app/main.py` — FastAPI; loads a run, selection and prediction endpoints
  - Runs in `output/ml-service/runs/<run_id>/`, with `current_run.json` naming the one served

Config precedence: CLI → env → `config.json` → defaults. See [config-and-data.md](config-and-data.md).

---

## Data and control flow

```mermaid
flowchart LR
  subgraph LocalFS[Local filesystem]
    CS[Cricsheet JSON]
    RUNS["runs/&lt;run_id&gt;/ + current_run.json"]
  end

  subgraph GoApp[go-app]
    CI[cricsheet-importer]
    API[api]
  end

  subgraph DB[(Postgres)]
  end

  subgraph ML[ml-service]
    RT["ml.xi.retrain: rating pass + models + manifest"]
    EV["ml.xi.evaluate: L4"]
    SVC[FastAPI]
  end

  CS --> CI --> DB
  DB --> RT --> RUNS
  DB --> EV
  RUNS -- reload --> SVC
  API -- player ids + fixture --> SVC
  SVC -- XI, P win, scorecard --> API
  API -. orchestration .-> CI
  API -. "retrain / evaluate / reload" .-> SVC
```

There is one path from data to a model, and it reads the event store. The precompute pass,
the export CSVs and the models that consumed them are gone (P-5, P-6), so there is no second
feature computation to fall out of step with the first.

Detailed flow (selection, prediction, evaluation) is in [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## Pipeline and team prediction

**Pipeline order:** import → retrain → reload, with `evaluate` beside them. Retrain builds a run
and publishes nothing; reload points `current` at one and loads it, which is how you swap back
to an earlier run. See [ml-and-training.md](ml-and-training.md).

**Team prediction (per match):** go-app resolves the fixture and both pools, then sends **player
ids and a format** — never features. ml-service picks each XI (`/xi/optimize`, by alternating
best response), gives the displayed P(win) (`/xi/predict-win`), and either draws the match
(`/simulate`, limited-overs) or returns per-player distributions (`/performance/predict`). The
constraints go-app supplies are the ones it alone knows: team size, minimum bowlers, keeper.

**Formats:** models are per format (`TEST`, `ODI`, `T20`, `T20I`). Selection is *optimised* only
where the objective ranks (H-17): TEST is served a rating-ordered XI, marked not optimised.

---

## How to run

**Bootstrap:**

```bash
make up-all CUTOFF=2025-09-01
```

(Postgres, services, migrations, import, retrain, reload.)

**Step-by-step:**

1. `make migrate`
2. `make cricsheet-import`
3. `make retrain CUTOFF=<YYYY-MM-DD>` (see [ml-and-training.md](ml-and-training.md))
4. `make reload`
5. `make ml-serve` if not using Docker

Optional: `make evaluate` runs L4 over the database — the walk-forward folds, the locked window
and the parity check. It refits every model per fold per format and takes about an hour, which
is why it is not part of the pipeline.

Dev: `make init`, `make dev-up`, `make dev-down`, `make logs`. Copy `.env.example` to `.env`;
see [config-and-data.md](config-and-data.md) for env vars.
