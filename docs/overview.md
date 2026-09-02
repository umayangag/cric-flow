# System overview

Architecture, data flow, and how to run the pipeline. For a concise data-flow and ML-shape reference, see [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## Components

- **Data:** Cricsheet JSON under `data/go-app/cricsheet`.
- **go-app (Go):**
  - `cmd/migrate` — apply SQL migrations
  - `cmd/cricsheet-importer` — import Cricsheet JSON into Postgres
  - `cmd/precompute-all` — run every precompute stage for a format
  - `cmd/precompute-features` — form, venue, opposition, consistency
  - `cmd/precompute-sequence-features` — sequence (recent-innings) features
  - `cmd/export-dataset` — export model-ready CSVs to output dir
  - `cmd/api` — HTTP API and pipeline orchestration
  - `cmd/print_canonical` — print the canonical format codes
- **Postgres** — system of record
- **ml-service (Python):**
  - `ml/xi/` — the rating pass, the win models, the performance model, the simulator, the L4 harness
  - `ml/train_win.py` — the windowed-form win trainer (P-6 removes it)
  - `app/main.py` — FastAPI; loads artifacts, selection and prediction endpoints
  - Artifacts in `output/ml-service/`

Config precedence: CLI → env → `config.json` → defaults. See [config-and-data.md](config-and-data.md).

---

## Data and control flow

```mermaid
flowchart LR
  subgraph LocalFS[Local filesystem]
    CS[Cricsheet JSON]
    GOEXP[Exported CSVs]
    ART[ML Artifacts]
  end

  subgraph GoApp[go-app]
    CI[cricsheet-importer]
    PRE[precompute-*]
    EXP[export-dataset]
    API[api]
  end

  subgraph DB[(Postgres)]
  end

  subgraph ML[ml-service]
    XI[ml.xi rating pass + models]
    TRAIN[train_win]
    SVC[FastAPI]
  end

  CS --> CI --> DB
  DB --> PRE --> DB
  DB --> EXP --> GOEXP
  GOEXP --> TRAIN --> ART
  DB --> XI --> ART
  ART --> SVC
  API -- player ids + fixture --> SVC
  SVC -- XI, P win, scorecard --> API
  API -. orchestration .-> CI
  API -. orchestration .-> PRE
  API -. orchestration .-> EXP
```

Detailed flow (selection, prediction, evaluation) is in [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## Pipeline and team prediction

**Pipeline order:** import → `make train-xi CUTOFF=` → `/admin/reload`. The XI layer reads the
event store, so precompute and export are not in front of it; they feed the windowed-form win
model, which P-6 removes. See [ml-and-training.md](ml-and-training.md).

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
make up-all
```

(Postgres, services, migrations, import, precompute, export, train, restart ML.)

**Step-by-step:**

1. `make migrate`
2. `make cricsheet-import`
3. `make precompute-all FORMAT=T20` (or `make precompute-all-all-formats` for every format; `make precompute` triggers the same work through a running API)
4. `make export-dataset`
5. `make train-xi CUTOFF=<YYYY-MM-DD>` (see [ml-and-training.md](ml-and-training.md))
6. `make ml-serve` if not using Docker

Dev: `make init`, `make dev-up`, `make dev-down`, `make logs`. E2E: `make e2e FORMAT=ODI SEASON=2019`. Copy `.env.example` to `.env`; see [config-and-data.md](config-and-data.md) for env vars.
