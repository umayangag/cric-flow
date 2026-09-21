# Observability Map for `cric-flow`

This document maps where to look for system health and behavior across the Go API, ML service, and frontend.

It is intended for operators, developers, and AI agents diagnosing issues or validating deployments.

---

## 1. Go Backend (`go-app`)

### 1.1. HTTP Endpoints

- **`GET /ops/status` (API health + pipeline + DB)**
  - **Purpose**: Single JSON snapshot summarizing API readiness, database state, which run is serving, and pipeline runnability.
  - **Key sections** (top-level fields):
    - `timestamp` (ISO string).
    - `services.api_health`, `services.api_readiness`, `services.ml_health`.
    - `db.counts` (e.g. `players`, `matches`) and `db.last_match_import_at`.
    - `artifacts.current_run` / `artifacts.loaded_run` — the run `current` points at, and the
      run the ML service actually loaded. They differ when a reload has not happened yet.
    - `freshness` — **the one freshness verdict, assembled once and read by every surface**
      (P2-1). Three named facts and nothing else: `freshness.served` is H-11's verdict copied
      through from ml-service (`status` — `fresh` | `stale` | `not_loaded` | `unknown` —
      `fresh`, `data_age_days`, `max_age_days`, `data_through`, `ratings_through`, `code`), and
      it is the only badge and the only thing that says whether a prediction would be refused.
      The age is measured from `data_through`, the served run's own training boundary, and
      never from `ratings_through`, the last match it folded in (SERVE-03): an off-season moves
      the second and no retrain can move it back;
      `freshness.database[FORMAT]` is the import's lag as facts (`latest_match_date`,
      `age_days`, `match_count`, and a `note` when there is no date) with no status of its own;
      `freshness.retrain_due` is whether the database holds matches the served run never saw
      (`status` — `up_to_date` | `retrain_due` | `unknown` — `days_behind`,
      `latest_match_date`, `format`). **One threshold in the whole system**:
      `ml.ratings_max_age_days`, applied by ml-service and read off the verdict — go-app holds
      no copy of it. The vocabulary is declared in `contracts/ops-console.contract.json`
      (`freshness_statuses`, `retrain_statuses`, `ratings_stale_code`) and asserted from all
      three components (H-24). It replaced `db_freshness`, whose 7/30-day buckets were a second
      rule that read *stale* while H-11 read *fresh* on the same box.
    - `artifacts.error` — why a run on disk was refused (D-6). An empty panel and a refused
      artifact set look the same otherwise, and only one is something to act on.
    - `artifacts.runs[]` — every run directory, newest first, with its manifest summary
      (including `ratings_through`, the date the run's data runs through, P2-2), `has_manifest`
      for the ones that are not runs, and `refused` — `null`, or why the run cannot be loaded.
    - `pipeline.steps[step_id].running|runnable|completed|optional` and `pipeline.order` (derived from `data_migrations` and the step registry).
    - `dataset.path|exists|match_files|bytes|newest_file|newest_modified` — the Cricsheet directory Import reads from, resolved by `GO_APP_CRICSHEET_DIR` → `inputs.cricsheet_dir` → built-in default.
    - Optional: `fielding`, `db_completeness` (a different question — is the import
      empty? — and not a freshness verdict), `suggestions`.
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
| `POST /ops/pipeline/stop` | Stops the plan, the step it is on, **and the training process on ml-service** |

- **Named plans are derived from the step registry**, not written out: `full` is every
  non-optional pipeline step (import → retrain → reload), `retrain-only` is the same
  without the import. Optional steps are never implied — a "run everything" that silently
  included the L4 harness would spend an hour nobody asked for.
- **`import` is the exception, and states its own order**: fetch → extract → import. No
  filter over registry order could produce it, because acquisition sits on a different
  surface and carries no `Requires`. There is no longer a `tune` plan: the grid runs
  inside `retrain`, so searching and training cannot be run in the order that throws the
  artifacts away.
- **`refresh` is the scheduled cadence** (A-5): `import` followed by `retrain-only`, so
  fetch → extract → import → retrain → reload. It is composed from the two plans rather
  than written out, and it exists because those two being separate is what lets them come
  apart — on the box A-5 was written on the `import` plan had run with no retrain after
  it, leaving the served ratings 9 days old (H-11 refuses at 14) while the database held
  matches 2 days old. `make cadence` is the unattended way in; see
  [overview.md](overview.md) § Cadence.
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
  a run that cancelled itself — which is how a premature cancellation inside a worker
  pool went unexplained for two full runs.
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

### 1.2.2. What "completed" means

A step is complete when its **most recent** run in `data_migrations` is `COMPLETED` —
not when it has ever completed. "Has this command ever succeeded?" is a question about
the box's history rather than about the data on it, and answering it kept a green tick
on a step whose latest run had failed or been cancelled, while `runnable` let the steps
after it run against output that was never rebuilt.

The same rule reaches the run listing from the other direction. A retrain writes its
manifest last, so a run that stopped early leaves a directory with no manifest: the
loader never selects it, `/artifacts/status` lists it as `has_manifest: false`, and the
console's own check treats artifacts on disk as evidence of a run only when run history
also says the step completed.

`BuildPipelineSection` decides completion; the console's own checks — files on disk,
freshness dates — only fill in for steps the backend has said nothing about, and can no
longer raise a step to green that run history says is not done.

### 1.3. Logs

- **Log shape** (via `log/slog`):
  - Structured keys for pipeline/train jobs, including:
    - `step` / `command` (e.g. `cricsheet-import`, `xi-retrain`, `xi-reload`).
    - `format`, `cutoff`, `job_id` or similar identifiers.
    - `err` for structured errors.
  - **Where to look**:
    - API process logs (stdout/stderr from `go-app/cmd/api`).
    - CLI runners: the importer binary.

---

## 2. ML Service (`ml-service`)

### 2.1. Health and Artifacts

- **`GET /health`**
  - **Purpose**: Is the ML service alive, and can it answer?
  - **Typical fields**:
    - `status` (`ok` / `warn`).
    - `models_dir` (resolved artifacts root), `loaded`, `run_id`.
    - `loaded_xi_formats`, `loaded_performance_formats` — the two model families that are
      left. The per-model lists (`loaded_batting_formats` and its four siblings) went with the
      models in P-6.
    - `ratings` (H-11's verdict), `error` (the loader's refusal, D-6).
    - `metadata`, `counters`.

    `status: "ok"` is about the process. A service with no run loaded is alive, and
    `loaded: false` is how it says so — conflating the two is what let a box with no model
    report itself healthy.

    It answers while the service is computing. The handler runs on the event loop and reads
    only what is already in memory, and every route that computes or touches the disk runs
    on the threadpool instead (SERVE-01) — so a probe during an optimise or a simulate is
    answered in about a millisecond rather than waiting the whole of it out. Ten concurrent
    simulates measured 4,110 ms for a single answered probe before that change and a median
    of 2.6 ms over 32 probes after it.

    Each served prediction computes with one thread per numeric library, which the log
    records nowhere because it is not a per-request event; it is `app.serving_compute`, and
    it is why several predictions at once no longer put twelve threads each on twelve
    cores. The `retrain` and `evaluate` subprocesses are deliberately outside it and keep
    every core.
  - **Consumers**:
    - Frontend `HealthTab` (latency + artifacts summary).
    - Operators verifying which models are in memory.

- **`GET /artifacts/status`**
  - **Purpose**: Every run on disk, which one `current` names, and which one is loaded.
  - **Fields**:
    - `timestamp`, `root`, `reachable`, `current_run`, `loaded_run`, `ratings_through`,
      `ratings` (H-11's verdict), `error` (the loader's refusal), and `runs[]` — each run
      newest first with `run_id`, `created_at`, `cutoff`, `git_sha`, `dataset_sha`, `formats`,
      `has_manifest`, `current` and `loaded`.
    - It reports **runs, not a formats-by-model-kind matrix** (H-16): a run is what an artifact
      belongs to now, so "is the model current?" is answered by which run `current` points at
      and whether that is the run the process loaded. A directory with no manifest is listed as
      `has_manifest: false` rather than hidden — it is exactly what an operator is looking for
      when nothing loads.
  - **Consumers**: go-app `/ops/status`; debugging run deployment / reload issues.

### 2.2. Run identity

- **`GET /xi/status`**
  - **Purpose**: which run is serving, and what that run recorded about itself (H-16).
  - **Key fields**: `loaded`, `run_id`, `formats`, `performance_formats`, `players`,
    `ratings_through`, `ratings` (H-11's verdict), `error` (the loader's refusal, D-6),
    `manifest` (run id, created-at, cutoff, dataset sha, git sha, formats, the
    hyperparameters the grid chose, the run's headline metrics) and `report` (the run's
    own training report).
  - **Consumers**:
    - Frontend `WorkbenchRunSection` — "what is this model?" answered from the record
      rather than inferred from filenames.
    - go-app `/api/ml/xi-status`, and `/ops/status` via `/artifacts/status`.
  - **Why it replaced `/model-stats`**: that endpoint described artifacts by scanning them
    and joined tuning metadata from a database table nothing could tie back to a file. The
    manifest is written by the run, beside the artifacts it produced.

### 2.3. Cross-process progress

A training step runs as a subprocess of ml-service, and
`training_orchestrator.run_training_subprocess` calls `subprocess.run(...,
capture_output=True)` — which buffers everything until exit, discards it on success and
logs only a tail on failure. Nothing about a ten-minute run is observable from its
output. A subprocess cannot push into its parent's memory, but it can write a file, and
`ml/run_progress.py` is that channel.

- **Event schema** (versioned; `v` is bumped on a breaking envelope change):

  ```json
  { "v": 1, "run_id": "1234", "step": "retrain", "phase": "grid",
    "current": 3, "total": 5, "metrics": {"rmse": 24.1}, "ts": "2026-08-26T12:00:00Z" }
  ```

  Step-specific fields (the grid's `params` and `auc`) are merged at
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
  would be noise.

- **Folded into one stream.** go-app polls the endpoint for any step the registry marks
  as `RunsOnMLService()` while it is in flight, and puts the event in the step's
  `training` field on `/ops/pipeline/stream`. `import` and `reload` run
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
digest onto it from the manifest in the dataset directory — ml-service reads the event store,
not the dataset directory, so it does not know which archive produced the matches it trained
on, and go-app does. That join is what makes
*"which data produced this model?"* a lookup:

```
GET /ops/migrations  ->  metadata.provenance.dataset_sha256
                         metadata.summary.formats[].metrics
                         metadata.summary.dropped_columns
```

Provenance that cannot be established is omitted rather than blanked: a dataset
directory populated by hand has no manifest, and a hand-placed archive has a digest but
no feed or URL. Absent means genuinely unknown.

**What `retrain` emits** (`ml/training_progress.py`), by phase. There is one training step
now; the six per-model trainers that shared this channel went in P-5 and P-6:

| Phase | Carries |
|---|---|
| `load` | rows, features, targets — a run against 200 stale rows looks like one against 200,000 until something says otherwise |
| `features` | low-variance columns dropped, by name, with kept/dropped counts |
| `fit` | rows and features at the moment fitting starts — the long silent stretch |
| `grid` | the display model's grid point and its inner-split AUC. Only `retrain` searches, and only over three points; a step emitting fake folds to look busy would be worse than one saying nothing |
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

- **Retrain / evaluate**
  - Logs include:
    - `model_name`, `match_format` — every model is per-format, so the format is the mode.
    - `pipeline_id` / `run_id` equivalents where available.
    - The display grid's candidate scores and the incumbent-vs-candidate decision, and the
      baseline comparison metrics. There are no tuning CV scores: the Optuna / PyCaret /
      AutoGluon stack went in P-6 and what searches now is a three-point grid inside `retrain`.
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
    - ML models directory, the loaded run, its formats, and the ratings' freshness verdict.

- **`OpsStatusTab`**
  - **Endpoint**: `GET /ops/status` (Go API).
  - **Displays**:
    - Pipeline graph (import → retrain → reload, with evaluate beside them).
    - Pipeline progress panel (current running step and actions).
    - Database stats, table counts, last match import.
    - The runs on disk, which is current, which is loaded, and the ratings verdict.
    - Migration history, format hierarchy, suggestions.
  - **Polling**:
    - Uses `usePolling` to auto-refresh at a fixed interval while mounted.

- **`EvaluationReportTab`**
  - **Endpoints**:
    - `GET /api/backtest/report` (L4's evaluation report; 503 when the harness has not run).
    - `GET /api/backtest/metric-glossary`, through the shared `MetricGlossaryProvider`, for
      each metric's explainer and for the band its value is painted against (L-1).
  - **Displays**:
    - Walk-forward folds per format, with the locked window labelled beside them.
    - The two selection metrics, and a labelled slot for E5.
    - Per-target performance with interval width beside coverage; the simulator's E2 section.
    - The train/serve parity verdict, as a success or error alert.
    - Every measured number painted red-to-green against its own reference band, from the
      glossary's `scale` — never against a page-wide guess. A metric the glossary anchors
      (AUC, Brier, coverage, the violation share) is read against those anchors; one in the
      target's own units (pinball, MAE, the hit rate) is read against the baseline printed
      beside it in the same row; one with no good direction alone — an interval width — is
      left uncoloured on purpose, and the legend above the tables says so.
  - **Polling**: none. The report is a file the harness writes; there is nothing to poll.

- **`TeamLabTab`** (the Team Lab, `/lab` — the Upcoming-match tab grown up, P1-1)
  - **Endpoints**:
    - `GET /api/options/formats`, `/api/options/teams-by-format`, `/api/options/opponents`,
      `/api/options/venues` (the fixture pickers; sides, never bare names — D-10).
    - `POST /api/predict/team-selection` (both XIs, the probability, the scorecard). The
      one surface on this endpoint: two would drift and only one of them would be right.
    - `GET /api/options/candidates` and `POST` / `DELETE /api/players/{id}/retirement`
      (the candidate pool and the retirement ledger — D-12).
    - `GET /ops/status`, for the readiness notice that says what a prediction will be
      missing before it is run rather than after it has answered on zeros.
  - **Inputs**: format, both sides, venue, date, each side's candidate pool, the **toss**
    (bat first / bowl first / unknown, wired to `team1_bats_first`; unknown is sent by
    omission and is the marginalised default), and the constraints — minimum bowlers, a
    keeper, and must-include player ids that join the pool whatever the window or the
    ledger says.
  - **Displays**:
    - Both XIs with their ranges, the selection and forecast notes, the probability with
      its source, and the simulated scorecard where the format has an innings length.
    - **Which toss the numbers assume**, from the response's `toss`: the side that bats
      first where it is known, "both batting orders averaged" where it is not, and the
      reason where a named toss could not be used (§8.7).
    - **The pool each XI was chosen out of**: "played for `<team>` in the last `<N>`
      months (`<M>` players)", with the all-time pool one click away, and every player the
      retirement ledger removed shown struck through with his reason and an Undo. A filter
      that is not shown is indistinguishable from no filter, which is what D-12 was.
    - The candidate dialog: last-played beside every name, tick a subset to send it as the
      pool, or close it and keep the default.
  - **Polling**: none. Every call is a thing the user just asked for.

- **`SystemMapTab`**
  - **Endpoints**:
    - `GET /ops/status`, `GET /api/ml/xi-status`, `GET /api/backtest/report` (Go API) — the
      three the map's live values are read from. No endpoint of its own.
    - `GET /api/backtest/metric-glossary`, through the shared `MetricGlossaryProvider`, for
      the explainer on a metric the map shows (L-1).
  - **Displays**:
    - The whole pipeline as a pan-and-zoom graph, from the Cricsheet archive to the
      prediction surfaces, drawn from `contracts/system-map.json`.
    - Per step: a plain-language summary, the code it is (modules, packages, endpoints,
      tables, make targets, artifacts), the documents that describe it, and its current
      live values. A value the endpoints do not carry reads as a dash.
    - Inner structure for the three steps that have it: the rating pass's feature
      families, the frames, and the harness's gates — the gates read from the report's own
      H-23 registry, so the map holds no copy of their terms.
  - **Read-only.** The control surface is `OpsStatusTab`'s step graph; this one describes
    the pipeline rather than driving it.
  - **Polling**: none. A Refresh button re-reads all three.

- **Data acquisition** (`OpsDatasetSection` + `DatasetRegistrySection`, inside `OpsStatusTab`)
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

- **`WorkbenchTab`**
  - **Endpoints**:
    - `GET /api/ml/xi-status` (the loaded run and its manifest: H-16).
  - **Displays**:
    - **What the loaded model is** (`WorkbenchRunSection`): the run id, its cutoff, how far
      its ratings run, the dataset digest, the commit, the formats it serves, the
      hyperparameters the grid chose and the run's headline metrics — all from
      `manifest.json`, which the run wrote beside the artifacts it produced (H-16). A run
      the loader refused says so, with the reason (D-6). A format the run trained but had
      no holdout to score reads its row counts plus the manifest's note saying why the
      AUCs are missing (B-3), rather than an empty table that would look like a run which
      trained nothing.
    - How well the models predict is the Evaluation report tab, which reads L4's own
      measurements rather than re-scoring anything here. That includes the walk-forward
      numbers: the tab renders L4's per-fold, per-format tables. The Workbench uploads
      nothing (F-1, D-8) — the walk-forward registry upload it used to carry asked for a
      file no module has written since P-5 deleted `ml.walk_forward`.

---

## 4. Common Logging and IDs (In Progress)

To simplify correlation across systems (Go, ML, frontend), logs and payloads should converge on a small set of common identifiers:

- `pipeline_id`: Logical pipeline execution (import → retrain → reload).
- `run_id`: Individual run within a pipeline (e.g. one data migration or training run).
- `format`: Match/series format (`TEST`, `ODI`, `T20I`, `T20`, etc.).
- `artifact_type`: Model family (`xi_win`, `xi_perf`, `xi_ratings`).
- `cutoff`: Time cutoff for features/training (ISO string).

Where practical:

- Include these fields in:
  - Go logs for pipeline steps and `/ops/status` suggestions.
  - ML logs for retrain and evaluate runs.
  - JSON responses where they help debugging (e.g. model stats, backtest/evaluate steps).
- Keep naming consistent between Go and Python to ease cross-service searching.

---

## 5. How to Use This Map

When debugging or validating a deployment:

The console has five tabs: Health, Ops Status, Workbench, Evaluation report and the Team
Lab. The ML-model-stats tab went with the endpoint behind it (P-6), and data acquisition is
a section of Ops Status rather than a tab of its own.

1. **Check basic health**
   - `frontend → HealthTab` (Go + ML status, latency, the loaded run and its freshness).
2. **Inspect pipeline and data readiness**
   - `frontend → OpsStatusTab` (pipeline, DB, the runs on disk and which is serving).
   - `GET /ops/status` directly if needed (for scripting).
3. **Investigate model quality and versions**
   - `frontend → WorkbenchTab` (the run manifest: cutoff, dataset digest, commit, chosen
     hyperparameters, headline metrics).
   - `GET /xi/status` and `GET /health` (ML).
4. **Validate model behaviour**
   - `frontend → EvaluationReportTab` (L4's folds, locked window, parity check) — including
     the walk-forward tables, which are the harness's and nothing else's.
5. **Correlate logs**
   - Filter Go and ML logs by `pipeline_id`, `run_id`, `format`, and `artifact_type` when present.

This should give new engineers (or agents) a clear starting point for understanding and monitoring `cric-flow` in production.

