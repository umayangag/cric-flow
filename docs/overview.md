# System overview

Architecture, data flow, and how to run the pipeline. For a concise data-flow and ML-shape reference, see [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## Components

- **Data:** Cricsheet JSON under `data/go-app/cricsheet`; optional curated CSVs under `data/go-app/createdb`.
- **go-app (Go):**
  - `cmd/cricsheet-importer` — import Cricsheet JSON into Postgres
  - `cmd/etl-importer` — import curated CSVs (optional)
  - `cmd/precompute` — form, venue, opposition, consistency, sequences
  - `cmd/export-dataset` — export model-ready CSVs to output dir
  - `cmd/api` — HTTP API and pipeline orchestration
  - `cmd/team-predictor` — CLI: features → ML → team selection
- **Postgres** — system of record
- **ml-service (Python):**
  - `ml/train_*.py` — train from exported CSVs or go-app API
  - `app/main.py` — FastAPI; loads artifacts, prediction endpoints
  - Artifacts in `output/ml-service/`

**Config precedence:** CLI → env → `config.json` → defaults. See [config-and-data.md](config-and-data.md).

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
    PRE[precompute]
    EXP[export-dataset]
    API[api]
    TP[team-predictor]
  end

  subgraph DB[(Postgres)]
  end

  subgraph ML[ml-service]
    TRAIN[train_*.py]
    SVC[FastAPI]
  end

  CS --> CI --> DB
  DB --> PRE --> DB
  DB --> EXP --> GOEXP
  GOEXP --> TRAIN --> ART
  ART --> SVC
  TP --> SVC
  SVC --> TP
  API -.-> CI
  API -.-> PRE
  API -.-> EXP
```

Detailed flow (backtest, team prediction, Monte Carlo) is in [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## Pipeline and team prediction

**Pipeline order:** Precompute → export-dataset → train models → run/restart ML service. Batting/bowling use CSVs; fielding/extras/win can use API with cutoff. See [ml-and-training.md](ml-and-training.md).

**Team prediction (per match):** go-app gets players and context from DB, builds feature vectors, calls ML `/predict/batting`, `/predict/bowling`, `/predict/fielding`, then selects best XI (e.g. ≥5 bowlers, ≥1 keeper).

**Formats:** Features and export are per-format (`TEST`, `ODI`, `T20`, `T20I`). Export can produce per-format and unified CSVs; ML trains both; prediction uses format when present.

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
3. `make precompute SEASON=2019` (or `make precompute-all-all-formats`)
4. `make export-dataset`
5. `make train-all` (see [ml-and-training.md](ml-and-training.md))
6. `make ml-serve` if not using Docker
7. `make team-predictor MATCH=<id> BAT=6 BOWL=5`

**Dev:** `make init`, `make dev-up`, `make dev-down`, `make logs`. E2E: `make e2e FORMAT=ODI SEASON=2019`. Copy `.env.example` to `.env`; see [config-and-data.md](config-and-data.md) for env vars.
