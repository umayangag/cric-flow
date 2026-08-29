# Observability Map for `cric-flow`

This document maps where to look for system health and behavior across the Go API, ML service, and frontend.

It is intended for operators, developers, and AI agents diagnosing issues or validating deployments.

---

## 1. Go Backend (`go-app`)

### 1.1. HTTP Endpoints

- **`GET /ops/status` (API health + pipeline + DB)**
  - **Purpose**: Single JSON snapshot summarizing API readiness, database state, precompute/export status, artifacts, and pipeline runnability.
  - **Key sections** (top-level fields):
    - `timestamp` (ISO string).
    - `services.api_health`, `services.api_readiness`, `services.ml_health`.
    - `db.counts` (e.g. `players`, `matches`) and `db.last_match_import_at`.
    - `precompute.formats[FORMAT].status` (`ok` / `stale` / `missing` / `unknown`).
    - `exports.formats[FORMAT].files[]` (per-export file presence).
    - `artifacts.formats[FORMAT].batting|bowling.exists/loaded`.
    - `pipeline.steps[step_id].running|runnable|completed|optional` and `pipeline.order` (derived from `data_migrations` and the step registry).
    - `dataset.path|exists|match_files|bytes|newest_file|newest_modified` — the Cricsheet directory Import reads from, resolved by `GO_APP_CRICSHEET_DIR` → `inputs.cricsheet_dir` → built-in default.
    - Optional: `fielding`, `weather`, `hierarchy`, `suggestions`.
  - **Consumers**:
    - Frontend `OpsStatusTab` (auto-refreshing).
    - CLI / scripts (for quick readiness checks).

- **`GET /health` (Go API health)**
  - **Purpose**: Lightweight readiness probe for the API itself.
  - **Typical fields**: `status`, optional DB connectivity summary.
  - **Consumers**: k8s/infra health checks; frontend `HealthTab`.

### 1.2. Tracking and Pipeline State

- **Table**: `data_migrations`
  - **Columns**: `id`, `command`, `args`, `status`, `started_at`, `completed_at`, `metadata`, `error_message`.
  - **State model**:
    - `IN_PROGRESS` → `COMPLETED` / `FAILED` / `CANCELLED`.
  - **Key helpers** (in `internal/tracking`):
    - `Start`, `Tracker.Complete/Fail/Cancel`, `CaptureExit`.
    - `HasInProgressForCommand`, `HasCompletedSuccessfullyForCommand`.
    - `GetInProgressMigrations`, `GetRecentMigrations`.
    - `CancelInProgressMigration`, `ReconcileStaleRuns`.
  - **Usage**:
    - Ops Status pipeline section.
    - Pipeline `/ops/pipeline/*` handlers and `internal/services/pipeline`.
    - Run plans, under the command `pipeline-plan` (below).

### 1.2.1. Run plans (`internal/services/runplan`)

`make up-all` and `make full-pipeline` have existed in the Makefile for as long as the
pipeline has; from the ops console the same thing was eleven manual clicks with waiting
in between. A run plan is the server-side executor that closes that asymmetry.

| Endpoint | |
|---|---|
| `POST /ops/pipeline/run-plan` | A named plan or an explicit step list, never both. An empty body means `full` |
| `GET /ops/pipeline/plan` | The latest plan, running or not, with `resume_from` |
| `POST /ops/pipeline/stop` | Stops the plan **and** the step it is on |

- **Named plans are derived from the step registry**, not written out: `full` is every
  non-optional pipeline step, `retrain-only` the ml-service ones, `data-refresh` the
  go-app ones. Optional steps are never implied — a "run everything" that silently
  included auto-tune would take hours nobody asked for.
- **State lives in `data_migrations`** under `pipeline-plan`, written before and after
  every step rather than only at the end. A page reload, another tab, or a browser
  closed overnight does not lose the run.
- **`pipeline-plan` is deliberately not a registry step.** Were it one, the plan's own
  IN_PROGRESS row would put it in the compute lane and `LaneBusy` would block the very
  steps the plan exists to run. `UpdateMigrationMetadata` exists for the same shape of
  reason: the status-changing update stamps `completed_at`, which would mark a plan
  finished on its first step.
- **Stops at the first failure**, leaving the remaining steps `PENDING` so the plan
  resumes from where it stopped rather than from the top.
- **A stop is logged** (`pipeline stop: cancelled by user`, with the lanes it hit).
  `context canceled` reaches the logs from every goroutine that was holding work, so
  without a line at the endpoint itself there is no way to tell an operator's Stop from
  a run that cancelled itself — which is how a premature cancellation inside the
  precompute worker pool went unexplained for two full runs.
- **Ordering comes from `CanRunPipelineStep`**, injected rather than reimplemented, so a
  plan and a single-step trigger cannot disagree about whether a step may run.
- **`Execute` runs every step; `Resume` skips what the run being resumed completed.**
  Skipping is driven by that run's own state, never by history at large — a version
  that asked "has this step ever succeeded?" would skip everything on a box that had
  run the pipeline once, and report success having done nothing. The failed step is
  where a resume starts, not something to skip past.
- **Cancellation stops the plan first**: cancelling only the current step would end that
  step and let the plan start the next one, which is not what Stop means.
- **Rendered by `RunPlanPanel`** in the Ops Status pipeline card. It holds no plan state
  of its own — it reads `GET /ops/pipeline/plan`, and polls while idle as well as while
  running, so a plan started from another tab appears without a reload. Progress counts
  *finished* steps, not started ones, and a plan that is not running says so.

### 1.3. Logs

- **Log shape** (via `log/slog`):
  - Structured keys for pipeline/train jobs, including:
    - `step` / `command` (e.g. `cricsheet-import`, `export-dataset`).
    - `format`, `cutoff`, `job_id` or similar identifiers.
    - `err` for structured errors.
  - **Where to look**:
    - API process logs (stdout/stderr from `go-app/cmd/api`).
    - CLI runners: precompute, export, importer binaries.

---

## 2. ML Service (`ml-service`)

### 2.1. Health and Artifacts

- **`GET /health`**
  - **Purpose**: Combined ML service + artifacts health.
  - **Typical fields**:
    - `status` (`ok` / `warn`).
    - `models_dir` (resolved artifacts directory).
    - `loaded_batting_formats`, `loaded_bowling_formats`, `loaded_fielding_formats`, `loaded_extras_formats`, `loaded_win_formats`.
    - `artifacts[model_type][]` with `file`, `size_bytes`, `modified` (unix timestamp).
    - `metadata`, `counters`.

    The `legacy_*_available` fields are **gone**: they reported the `_LEGACY_` artifact
    tier removed in C3-1. Built by `app/artifact_service.py:build_health_response`.
  - **Consumers**:
    - Frontend `HealthTab` (latency + artifacts summary).
    - Operators verifying which models are in memory.

- **`GET /artifacts/status`**
  - **Purpose**: Detailed status of model artifacts on disk vs loaded in memory.
  - **Fields**:
    - `timestamp`, `root`, and `formats[<format>][<kind>]` carrying `exists`, `path`,
      `modified` and `loaded`. No legacy tier — see `artifact_service.build_artifacts_status`.
  - **Consumers**: debugging model deployment / reload issues.

### 2.2. Model Stats

- **`GET /model-stats`**
  - **Purpose**: Exposes per-model training/tuning/evaluation metadata for observability.
  - **Key fields per model**:
    - `model_name`, `match_format`, `algorithm`.
    - `best_cv_score`, `scoring`, `accuracy_display`.
    - `tuned`, `duration_seconds`, `trained_at`, `size_bytes`.
    - `tuned_parameters`, `metrics`, `feature_importance`.
    - Optional `mlqa_audit` (status, key findings, stability metrics).
  - **Consumers**:
    - Frontend `MLModelStatsTab` (tables, chips, and tuning insights).
    - Manual inspection of which models are “good enough” to promote.

### 2.3. Cross-process progress

A training step runs as a subprocess of ml-service, and
`training_orchestrator.run_training_subprocess` calls `subprocess.run(...,
capture_output=True)` — which buffers everything until exit, discards it on success and
logs only a tail on failure. Nothing about a ten-minute run is observable from its
output. A subprocess cannot push into its parent's memory, but it can write a file, and
`ml/run_progress.py` is that channel.

- **Event schema** (versioned; `v` is bumped on a breaking envelope change):

  ```json
  { "v": 1, "run_id": "1234", "step": "auto_tune", "phase": "cv",
    "current": 3, "total": 5, "metrics": {"rmse": 24.1}, "ts": "2026-08-26T12:00:00Z" }
  ```

  Step-specific fields (auto-tune's `algorithm`, `trial`, `best_score`) are merged at
  the top level beside the envelope, which is where existing consumers already read
  them. Envelope keys are reserved and cannot be shadowed.

- **One file per run**, at `<artifacts>/progress/<step>__<run_id>.json`. A single global
  path means two runs — concurrent, or one started before the reader noticed the last
  finished — overwrite each other's state, and the reader cannot tell which run it is
  looking at. `run_id` comes from `PIPELINE_RUN_ID` or the pid, and is sanitised into
  the filename rather than trusted.

- **Writes are atomic**: temp file in the *destination directory*, then `os.replace`
  (only atomic within one filesystem, so `/tmp` will not do). A reader polling the file
  sees the old event or the new one, never half of either.

- **Reading**: `latest_for_step` returns the newest **non-stale** file for a step —
  older than 30 minutes is treated as abandoned. A crashed run leaves its file behind,
  and reporting that as current shows a run that is not happening.

- **Emission never breaks a step.** Unwritable directory, full disk, an observer that
  raises: all logged and swallowed. Progress is telemetry.

- **Served by** `GET /admin/train/progress?step=&run_id=`. With no `run_id` it reports
  the *live* run — the newest non-stale file for that step. Naming a `run_id` reads
  exactly that run, finished or stale. An empty object means "nothing is running",
  which is a normal answer, not an error: go-app polls on a timer and a 404 per tick
  would be noise. `GET /admin/train/auto-tune/progress` survives as a delegate — one
  implementation, two routes. `AUTO_TUNE_PROGRESS_FILE` pins an explicit path for
  tests, or to `tail` one file, and applies only to auto-tune.

- **Folded into one stream.** go-app polls the endpoint for any step the registry marks
  as `RunsOnMLService()` while it is in flight, and puts the event in the step's
  `training` field on `/ops/pipeline/stream`. `import`, `precompute` and `export` run
  inside go-app and are never asked.

- **Unreachable is reported as unknown, not failure.** `progress_unavailable: true`
  means ml-service could not be asked; an absent `training` field means the step has
  published nothing yet. Both render as an empty panel otherwise, but one is a run
  about to report and the other is a broken link. The UI says so in words, including
  that the step is still running — saying "failed" about a healthy run whose telemetry
  link is down would be worse than saying nothing.

**The run's outcome outlives its progress.** `finish()` writes a `.result.json` beside
the progress file and then removes the progress file. They have different lifetimes on
purpose: progress that lingers reads as a run still going, while the outcome has to
survive because whoever wants it — go-app, persisting into `data_migrations.metadata` —
asks only after the run is over. ml-service returns the summary on the training
endpoint's response, which is the only moment it is still available.

The summary carries per-format rows and features, per-format metrics, the low-variance
columns dropped, and the artifacts written with their sizes. go-app stamps the dataset
digest onto it from the manifest in the dataset directory — ml-service does not know
which dataset produced the CSVs it trained on, and go-app does. That join is what makes
*"which data produced this model?"* a lookup:

```
GET /ops/migrations  ->  metadata.provenance.dataset_sha256
                         metadata.summary.formats[].metrics
                         metadata.summary.dropped_columns
```

Provenance that cannot be established is omitted rather than blanked: a dataset
directory populated by hand has no manifest, and a hand-placed archive has a digest but
no feed or URL. Absent means genuinely unknown.

**What the six trainers emit** (`ml/training_progress.py`), by phase:

| Phase | Carries |
|---|---|
| `load` | rows, features, targets — a run against 200 stale rows looks like one against 200,000 until something says otherwise |
| `features` | low-variance columns dropped, by name, with kept/dropped counts |
| `fit` | rows and features at the moment fitting starts — the long silent stretch |
| `cv` | fold index/total and per-fold metrics. **Only `train_win` cross-validates**; the rest fit once, and a step emitting fake folds to look busy would be worse than one saying nothing |
| `artifact` | each artifact's basename and size. Size is the postcondition worth checking: a model file of a few hundred bytes is a failed fit that reported success |
| `done` | per-format completion, then a final event for the run |

`current`/`total` count **completed formats**, not "the one running". Formats train
concurrently (`ML_TRAIN_FORMAT_WORKERS`), so there is no single current format; the
format an event is about travels in `extra.format`.

Emission is total. Every emitter is wrapped so neither the transport nor the argument
handling can propagate — a trainer that finished successfully must not be reported as
failed because a label was the wrong type. Metrics are coerced and filtered on the way
out: numpy scalars become floats, and NaN or infinity is dropped rather than written as
the literal `NaN`, which is not valid JSON and would make the file unreadable.

### 2.4. Logs

- **Training / auto-tune / backtest**
  - Logs include:
    - `model_name`, `match_format`, `mode` (legacy vs per-format).
    - `pipeline_id` / `run_id` equivalents where available.
    - Tuning CV scores, baseline comparison metrics.
  - **Where**: ML service process logs (stdout/stderr) for training and backtest workloads.

---

## 3. Frontend (`frontend`)

### 3.1. Tabs and Their Data Sources

- **`HealthTab`**
  - **Endpoints**:
    - `GET /health` (Go API).
    - `GET /health` (ML service).
  - **Displays**:
    - Status pills for API and ML.
    - Latency for each health check.
    - ML models directory, loaded formats, artifacts count, total size, latest modified.

- **`OpsStatusTab`**
  - **Endpoint**: `GET /ops/status` (Go API).
  - **Displays**:
    - Pipeline graph (import → precompute → export → train → auto-tune).
    - Pipeline progress panel (current running step and actions).
    - Database stats, table counts, last match import.
    - Precompute/export/artifacts readiness per format.
    - Migration history, format hierarchy, suggestions.
  - **Polling**:
    - Uses `usePolling` to auto-refresh at a fixed interval while mounted.

- **`EvaluateDbTab` (Backtest UI)**
  - **Endpoints**:
    - `GET /api/backtest/select` (candidate matches).
    - `POST /api/backtest/evaluate` (long-running evaluate job).
    - `GET /api/backtest/evaluate/status` (job status, steps, result).
    - `GET /api/backtest/match-scorecard` (scorecard for selected match).
  - **Displays**:
    - Candidate match list (by format/team1/team2).
    - Evaluation progress (step log) and MAE / accuracy metrics.
    - Match scorecard for selected candidate.
  - **Polling**:
    - Uses `usePolling` to track evaluate job status while a job is running.

- **`DataTab`**
  - **Endpoints**:
    - `GET /ops/data/feeds` (named feeds, host allowlist, staging directory).
    - `GET /ops/data/staged` (archives available to extract, plus the live manifest).
    - `GET /ops/data/datasets` (the dataset registry, live row marked).
    - `POST /ops/data/fetch`, `POST /ops/data/extract` (both answer 202).
    - `GET /ops/status` (whether a data-lane step is already running).
  - **Displays**:
    - Feed picker or explicit URL, with the allowlist stated before submission.
    - Live fetch (bytes, rate, ETA) and extract (entries) progress, rendered by
      `PipelineStepProgressCard` on the shared `/ops/pipeline/stream` SSE — there is
      no second progress channel for acquisition.
    - Registry table with the live dataset marked, and an explicit warning when the
      data directory holds a dataset the registry has never seen.
  - **Polling**:
    - Refetches staged archives, registry and busy state every 4s while a data step
      runs, so the tab settles by itself when the job finishes.

- **Run history drill-down** (`OpsMigrationsTable` → `RunSummaryPanel`)
  - **Source**: `GET /ops/migrations`, whose `metadata` a training run now fills (O-4).
  - **Displays**: which dataset produced the run (digest, feed, source URL, match-file
    count), per-format rows and artifacts, metrics **with their change against the
    previous completed run of the same step**, and the low-variance columns dropped.
  - **Direction matters**: a metric moving up is only good news for some metrics.
    Spread measures (`_std`, `variance`) are lower-is-better whatever they measure —
    `cv_accuracy_std` contains "accuracy" and must not be read as higher-is-better, or
    a model that got *less consistent* is reported as improved. A metric matching no
    rule is shown with its delta and no verdict.
  - **Comparison window**: opening the dialog fetches 100 recent runs to find the
    previous run of that step, because it is usually not on the page being viewed. A
    failed run is never used as a baseline. Finding none says so.
  - **Failures**: `error_message` already carries `CODE: message — hint` from go-app's
    `MLError`; the dialog splits it so the next action is its own block.

- **Model provenance** (`WorkbenchProvenanceSection`)
  - **Source**: `GET /api/ml/model-stats`. ml-service attaches each model's
    `provenance` from its sidecar; go-app attaches `live_dataset` and, per model,
    `dataset_is_live`.
  - **Displays**: per model — dataset digest, training cutoff, trained-at, accuracy —
    and a warning listing models trained on a dataset that is no longer on the box.
  - **Three states, not two**: `current`, `stale`, `unknown`. A model with no recorded
    dataset is *unaccounted for*, not out of date; `dataset_is_live` is absent rather
    than false, because flagging every such model as stale would warn about every model
    on a box that has not retrained since.
  - **The comparison is go-app's**: ml-service can say what a model trained on, but
    only go-app knows whether that is still what is on disk.

- **`WorkbenchTab`**
  - **Endpoints**:
    - `GET /api/formats` (available formats).
    - `GET /api/ml/model-metadata` (model metadata for features/outputs/artifacts).
    - `GET /api/backtest/accuracy-trend` (accuracy trend data).
  - **Displays**:
    - **Internal process explanation**: import → precompute → export → train → prediction.
    - **Accuracy trend** (via `WorkbenchAccuracyTrendSection`):
      - Per-match metrics (e.g. `player_runs_mae`, `team_runs_mae`, `team_winner_accuracy`).
      - Filters for format/date/limit, and model mode (format vs unified).
    - **Walk-forward registry**: uploaded JSON describing rolling-window evaluations.
    - **Model metadata**:
      - Features, outputs, and artifact patterns per model type.
      - Backend `/model-metadata` is canonical; `DEFAULT_MODEL_FEATURES` is a documented fallback.

- **`MLModelStatsTab`**
  - **Endpoint**: `GET /model-stats` (ML service).
  - **Displays**:
    - Per-model table with accuracy, MLQA status, size, training time.
    - Expandable rows for tuned parameters, metrics (flattened), feature importance, and audit findings.

---

## 4. Common Logging and IDs (In Progress)

To simplify correlation across systems (Go, ML, frontend), logs and payloads should converge on a small set of common identifiers:

- `pipeline_id`: Logical pipeline execution (import → precompute → export → train).
- `run_id`: Individual run within a pipeline (e.g. one data migration or training run).
- `format`: Match/series format (`TEST`, `ODI`, `T20I`, `T20`, etc.).
- `artifact_type`: Model family (`batting`, `bowling`, `fielding`, `extras`, `win`, `combination_meta`).
- `cutoff`: Time cutoff for features/training (ISO string).

Where practical:

- Include these fields in:
  - Go logs for pipeline steps and `/ops/status` suggestions.
  - ML logs for training, auto-tune, and backtest runs.
  - JSON responses where they help debugging (e.g. model stats, backtest/evaluate steps).
- Keep naming consistent between Go and Python to ease cross-service searching.

---

## 5. How to Use This Map

When debugging or validating a deployment:

1. **Check basic health**
   - `frontend → HealthTab` (Go + ML status, latency, artifacts).
2. **Inspect pipeline and data readiness**
   - `frontend → OpsStatusTab` (pipeline, DB, exports, artifacts).
   - `GET /ops/status` directly if needed (for scripting).
3. **Investigate model quality and versions**
   - `frontend → MLModelStatsTab` (per-model stats and tuning).
   - `GET /model-stats` and `GET /health` (ML).
4. **Validate backtest behavior**
   - `frontend → WorkbenchTab` (Accuracy trend, walk-forward).
   - `frontend → EvaluateDbTab` (per-match eval).
5. **Correlate logs**
   - Filter Go and ML logs by `pipeline_id`, `run_id`, `format`, and `artifact_type` when present.

This should give new engineers (or agents) a clear starting point for understanding and monitoring `cric-flow` in production.

