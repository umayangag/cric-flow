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

### 2.3. Logs

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

