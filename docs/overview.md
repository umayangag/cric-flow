# System overview

This document describes the current architecture, data flow, and how to run the pipeline.

---

## Components

- **Data sources:** Cricsheet JSON under `data/go-app/cricsheet`; optional curated CSVs under `data/go-app/createdb` (legacy).
- **Go application (go-app):**
  - `cmd/cricsheet-importer` — imports Cricsheet JSON into Postgres
  - `cmd/etl-importer` — imports curated CSVs (optional)
  - `cmd/precompute` — computes per-player features (form, venue, opposition, consistency)
  - `cmd/export-dataset` — exports model-ready CSVs to `output/go-app/`
  - `cmd/api` — HTTP API (optional orchestration)
  - `cmd/team-predictor` — CLI: assembles features, calls ML service, selects team
- **Postgres** — system of record for ingested and computed data
- **ML service (ml-service):**
  - `ml/train_batting.py`, `train_bowling.py`, `train_fielding.py`, etc. — train models from exported data or API
  - `app/main.py` — FastAPI that loads artifacts and serves prediction endpoints
  - Artifacts (*.joblib) in `output/ml-service/`

**Configuration:** Precedence is CLI flags/args > environment variables > component `config.json` > built-in defaults. See **config-and-data.md** for full config.

---

## Data and control flow

```mermaid
flowchart LR
  subgraph LocalFS[Local filesystem]
    CS[Cricsheet JSON\n data/go-app/cricsheet]
    CSV[Curated CSVs (optional)\n data/go-app/createdb]
    GOEXP[Exported CSVs\n output/go-app]
    ART[ML Artifacts\n output/ml-service]
  end

  subgraph GoApp[go-app]
    CI[cricsheet-importer]
    ETL[etl-importer]
    PRE[precompute]
    EXP[export-dataset]
    API[api (optional)]
    TP[team-predictor]
  end

  subgraph DB[(Postgres)]
  end

  subgraph ML[ml-service]
    TRAINB[train_batting.py]
    TRAINW[train_bowling.py]
    SVC[FastAPI app\n app/main.py]
  end

  CS --> CI --> DB
  CSV -. optional .-> ETL --> DB
  DB --> PRE --> DB
  DB --> EXP --> GOEXP
  GOEXP --> TRAINB --> ART
  GOEXP --> TRAINW --> ART
  ART --> SVC

  TP -- features from DB + context --> SVC
  SVC --> TP

  API -. optional orchestration .-> CI
  API -. optional orchestration .-> PRE
  API -. optional orchestration .-> EXP
```

---

## Generating a team prediction (happy path)

1. **Prerequisites (one-time or periodic):** Cricsheet JSON → `cricsheet-importer` → DB; `precompute`; `export-dataset` → CSVs; `train_batting` / `train_bowling` (and optionally fielding, extras, win) → artifacts; ML service loads artifacts from `output/ml-service`.
2. **Per match:** go-app queries DB for players and context, builds feature vectors, calls ML `POST /predict/batting` and `POST /predict/bowling` (and fielding when loaded), then selects best XI (constraints: e.g. min 5 bowlers).

---

## Format-aware pipeline

- Features are computed **per format** (`TEST`, `ODI`, `T20`, `T20I`). Schema: `match_details.format_id`, format-keyed feature tables (`player_form_data_fmt`, etc.).
- Export produces per-format CSVs (e.g. `batting_encoded_ODI.csv`) and optionally unified files (`batting_encoded_all.csv`). ML trains per-format and unified (legacy) artifacts.
- Prediction requests include `format`; the ML service routes to the matching artifact (or legacy fallback).

**Quick commands:**

- Precompute: `go run ./go-app/cmd/precompute -formats=ODI,T20I -season=2019`
- Export: `go run ./go-app/cmd/export-dataset -formats=ODI,T20I`
- Train: `make -C ml-service train-all`
- Predict: `go run ./go-app/cmd/team-predictor -match=<id> -format=ODI`

---

## How to run the pipeline

**One-line bootstrap:**

```bash
make up-all
```

Brings up Postgres and services, runs migrations, import, precompute, export, trains ML artifacts, restarts ML service.

**Step-by-step:**

1. `make migrate`
2. `make cricsheet-import`
3. `make precompute SEASON=2019` (or `make precompute-all-all-formats`)
4. `make export-dataset`
5. `make train-all` (from ml-service or repo root via Makefile)
6. `make ml-serve` (if not using Docker)
7. `make team-predictor MATCH=<match_id> BAT=6 BOWL=5`

**Dev environment:**

- `make init` — tooling init
- `make dev-up` — Docker services (Postgres, API, ML)
- `make dev-down` — tear down
- `make logs` — tail logs
- `make e2e FORMAT=ODI SEASON=2019` — format-aware end-to-end

Environment: copy `.env.example` to `.env`. Go services use `POSTGRES_*`, optional `LOG_LEVEL`, `GO_APP_CONFIG`, `GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`, `MIGRATIONS_DIR`.
