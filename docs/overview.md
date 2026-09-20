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
- **frontend (React):** the control panel. Its **System map** tab draws the whole pipeline
  from the Cricsheet archive to the prediction, one step at a time, from
  `contracts/system-map.json` — whose anchors CI checks against the code
  (`make check-system-map`) and whose numbers are read live from the endpoints.

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
where the objective ranks (H-17) and has shown it selects (E5, plan §8.8); the others are served a
rating-ordered XI, marked not optimised, with the reason on the wire and in the UI
(`ml.xi.optimizer.NOT_OPTIMISED_REASONS`).

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

---

## Cadence

The pipeline above is what an operator runs. This is what a scheduler runs, unattended:

```bash
make cadence          # fetch → extract → import → retrain → reload
make cadence-dry-run  # the same preconditions, starting nothing
```

One command. It starts go-app's `refresh` run plan and waits for it
(`scripts/cadence.sh`); the ordering, the run history, the progress stream and Stop are the
server's, because a scheduler entry that chained `make` targets would be a second pipeline to
keep in step with the first, and it could not be stopped from the console.

**Reload is the last step, and the plan stops at its first failure** — so the existing
guarantee holds under automation without any new machinery: a retrain that fails publishes
nothing, and `current` goes on pointing at the run it already pointed at. The one thing in the
chain that assumes a human is the data-quality gate (H-15): `--accept-data-quality` means "I
have looked at the new counts and they are right", and the unattended path never passes it. A
gate failure therefore stops the cadence with the previous run still serving, which is the
intended answer at 06:00 on a Monday.

**How often: weekly.** Cricsheet's update rhythm decides it — the archive is republished
daily-ish (the copy this was written against carried `Last-Modified: Tue, 01 Sep 2026 22:28:22
GMT`), and a week of matches is the smallest batch worth a ~10-minute retrain. The whole chain
is ~12 minutes idle: fetch ~25 s, extract ~21 s, import ~1–2 min, retrain ~10 min, reload
seconds. Nothing about it needs a quiet hour. `deploy/cadence/` holds an example systemd
timer and crontab — documentation, copied to a host and edited, enabled by nothing here.

**What to check after a run** (the cadence prints the last of these itself, and exits non-zero
if it is not `fresh`):

| | Where |
|---|---|
| The run manifest — id, cutoff, `dataset_sha`, git sha, the hyperparameters chosen and why | `output/ml-service/runs/<id>/manifest.json`, or `GET /ops/status` → `artifacts` |
| What it would take to build the run again — the source, the library versions, the model constants, what the dataset sha covers | `runs/<id>/manifest.json` → `source`, `library_versions`, `model_params`, `performance_spec`, `dataset_digest` |
| The report's counts — rows, player rows, undecided, the data-quality block | `runs/<id>/xi_win_report.json` |
| The freshness verdict — `fresh`, `age_days`, `ratings_through` | `GET /api/ml/xi-status` → `ratings` |
| That the run being served is the run just built | `/ops/status` → `artifacts.current_run` = `loaded_run` |
| The harness verdicts, **which a retrain does not refit** | `make evaluate` (~2 h 10 min), then the Evaluation tab |

**A scheduled run carries no holdout metrics, by construction.** The plan runs the retrain at
go-app's default cutoff, which is today: rows at or after it are the holdout, and there are no
matches after today, so every format trains on everything and reports `n_holdout: 0`. The
manifest's `formats` and `metrics` are empty for that reason — not because the run is broken
(all four formats' models are written and served). Training on everything is what a refresh
should do; it means the run cannot also be the thing that scores itself, and the numbers come
from `make evaluate` instead. Recorded as B-3 in `docs/BUG_BACKLOG.md`, because a run summary
that renders blank is weak evidence even when the run is fine.

The last row is the one to watch. `make evaluate` is not in the cadence — it costs ~2 h 10 min
against the chain's 12 — but the verdicts it produces are the ones new data can flip, and the
plan's §4 says which are power-limited rather than model-limited: T20I's 111-pair walk-forward
E5, and the women's-holdout question (H-7). More matches is their only cure, so the point of a
cadence is partly that those two get re-asked with more evidence. **T20's scoping above all**:
T20 is served a rating-ordered XI because E5 failed there (0.490 against a 0.501 bar, P-7), and
it is the verdict most likely to move — run the harness after a few cadences and read
`selection` per format before assuming the scoping is still right.

**When the cadence lapses**, H-11 is what the reader sees. Ratings older than
`XI_RATINGS_MAX_AGE_DAYS` (default 14) are refused for live predictions with `RATINGS_STALE`;
an `as_of` request is still served, because a backtest asks for a date and gets it. The age is
measured from the **last match date in the data**, not from the last run — so a weekly cadence
normally sits a few days inside the limit, and one missed run still answers. Two missed runs is
a service refusing predictions. That is the slack the rhythm is chosen for: weekly against a
14-day limit is one free failure, not zero.
