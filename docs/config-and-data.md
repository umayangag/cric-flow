# Configuration and data

This document covers configuration for go-app and ml-service, Cricsheet import behavior, and export-schema alignment with ML.

---

## Configuration overview

**Precedence:** Flags/CLI args → environment variables → component `config.json` → built-in defaults.

---

## Go application (go-app)

Config file: `go-app/config.json`

**Keys:**
- `server` (optional) — API/timeouts and URL fallbacks. All values have built-in defaults; omit or set 0 to use them.
  - `ml_health_timeout_sec` (default 10) — ML health proxy request timeout.
  - `ml_client_timeout_sec` (default 20) — ML client (predict/train) HTTP timeout.
  - `ml_health_body_limit_bytes` (default 1048576) — max ML health response size (DoS protection).
  - `ml_base_url_fallback` (default `http://localhost:8000`) — used when `ML_SERVICE_URL`/`ML_BASE_URL` are unset.
  - `readiness_timeout_sec` (default 2) — DB ping timeout for readiness probe.
  - `train_step_timeout_min` (default 30) — max wait for ML train endpoint (e.g. 10080 = 7 days for long training pipelines).
  - `pipeline_progress_interval_sec` (default 2) — SSE progress poll interval.
  - `db_probe_timeout_sec` (default 2), `db_probe_long_timeout_sec` (default 5) — ops DB probes.
  - `artifacts_timeout_sec` (default 3) — HTTP client timeout for artifacts check.
  - `http_read_timeout_sec`, `http_write_timeout_sec`, `http_idle_timeout_sec` (defaults 15, 30, 60) — HTTP server timeouts.
  - `listen_address` (default `:8080`) — overridden by `PORT` env.
- `inputs`
  - `cricsheet_dir` — default directory for Cricsheet JSON (used by cricsheet-importer).
  - `etl_dir` — default directory for curated CSVs (optional etl-importer path).
- `outputs`
  - `export_dir` — where `export-dataset` writes CSVs.
- `formats`
  - `treat_t20i_as_subset` (bool) — treat T20 between international teams as T20I.
  - `international_teams` (list) — ICC national teams for the subset rule.
- `features`
  - `precompute_timeout_ms` (int) — timeout for precompute/import pipeline steps (default 86400000). 0 = no deadline.
  - `export_timeout_ms` (int) — timeout for the export-dataset step only. 0 = use `precompute_timeout_ms`. Set higher than the pipeline timeout if export writes many format CSVs and was hitting "context canceled" (e.g. 3600000 = 60 min).
  - `min_batting_innings`, `min_bowling_innings`, `form_shrinkage_alpha`, `consistency_per_format`, `history_window_matches` — reserved or optional.
  - **Feature extraction:** `ewm_alpha` (0.3), `ewm_alpha_short` (0.5), `ewm_alpha_long` (0.2), `consistency_last_n` (10), `form_window_n` (0), `momentum_last_n` (5).
  - `fielding_enrich` — when ML has no fielding model: `ewm_alpha`, `form_to_catches_ratio` (0.7).
- `pipeline` (optional) — `precompute_concurrency`, `import_concurrency`, `seqcalc_concurrency`, `export_concurrency`, `fielding_concurrency` (0 = auto from GOMEMLIMIT/cgroup). **Precompute and memory:** When no limit is set, precompute uses a low default concurrency (2) to avoid OOM. When a limit is set (GOMEMLIMIT or cgroup v2, including Docker/K8s via `/proc/self/cgroup`), each pipeline’s concurrency is derived from the limit: workers are sized so total usage stays at about **80%** of the limit. Per-worker estimates: precompute 450 MB, import 150 MB, export 100 MB, seqcalc 500 MB, fielding 50 MB. For containers with ≤2GB memory, **seqcalc is capped at 1 worker** (each calculator can use more than the estimate; one worker can still exceed the limit on very large formats—set GOMEMLIMIT or use a larger container if needed). Replay: `replay_match_page_size` (500). See `resources` for per-worker MB and thresholds.
- `backtest` (optional) — `list_default_limit` (50), `list_max_limit` (500), `accuracy_trend_default_limit` (100), `accuracy_trend_max_limit` (500). `job`: `job_cleanup_age_hours` (24), `job_cleanup_interval_min` (15), `export_contributions_job_max_duration_hr` (2), `eval_job_max_duration_hr` (6), `eval_job_concurrency_min` (2), `eval_job_concurrency_max` (8).
- `ops` (optional) — `migrations_page_default` (10), `migrations_page_max` (100), `migrations_page_cap` (10000), `recent_migrations_count` (100).
- `resources` (optional) — `precompute_mb_per_worker` (450), `import_mb_per_worker` (150), `export_mb_per_worker` (100), `seqcalc_mb_per_worker` (500), `fielding_mb_per_worker` (50), `memory_usage_fraction_percent` (80), `seqcalc_low_memory_limit_gib` (2), `precompute_concurrency_when_no_limit` (2).
- `export`
  - `split_by_format` (bool) — write per-format CSVs by default.
  - `required_format` (string) — restrict export to this format unless overridden by flags.

**Environment:** `GO_APP_CONFIG`, `GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`, `PRECOMPUTE_CONCURRENCY`, `IMPORT_CONCURRENCY`, `SEQCALC_CONCURRENCY`, `EXPORT_CONCURRENCY`, `FIELDING_CONCURRENCY`.

**Team selection** (under `team`): `min_bowlers`, `default_batters`, `default_bowlers`. Under `selection`: `default_pool_csv`, `max_pool_size_for_full_enum` (18 — above this use greedy + hill-climb instead of full enumeration), `score_weights` (bat, bowl, field, keeper_bonus), `score_normalization` (per-format divisors), `score_weights_by_format`, `meta_model_path` (optional JSON from combination-meta training), `use_optimizer` (bool, default false). When `use_optimizer` is true, team selection uses constrained optimization to maximize total score over valid XIs (size 11, ≥1 keeper, ≥5 bowlers); when false, uses greedy selection with constraint swaps. See **ml-and-training.md** for meta-model.

**Future-match prediction features:** The feature map for team-selection prediction includes form, venue, opposition, season, optional weather override (`WeatherOverride`), and sequence features (bat_*, bowl_* from `configs/feature_vectors.json`). Sequence features are set to 0 until precompute/seqcalc export them per player. Opposition strength (`opposition_batting_strength`, `opposition_bowling_strength`) is computed from the opposition team’s pool when available and added to the map (training does not yet include these; when extended, accuracy can improve). Weather: training joins `weather_data` (may be empty); prediction accepts optional `Weather` in the API. When a weather source is added, use the same feature names (e.g. `batting_temp`) in training and prediction.

---

## Python ML service (ml-service)

Config file: `ml-service/config.json`

**Keys:**
- `inputs` — `go_app_export_dir`, `training_data_fetch_timeout_sec` (default 604800 = 7 days — HTTP timeout when fetching training data from go-app), `training_data_fetch_timeout_invalid_fallback_sec` (600), `training_subprocess_timeout_sec` (default 604800 = 7 days — max time for each /admin/train/* subprocess; set in config so long training runs don’t hit context deadline), `go_app_request_timeout_sec` (30 — tuned-params GET/POST).
- `outputs` — `artifacts_dir`.
- `ml`
  - `resources` (optional) — `training_mb_per_job` (400), `tuning_mb_per_job` (500), `prediction_mb_per_job` (100), `memory_usage_fraction_percent` (70), `training_low_memory_threshold_mb` (2560 — when process memory limit is at or below this MB, training uses 1 job to avoid OOM). Used for resource-aware n_jobs when a memory limit is set.
  - `formats` — list of format codes to train/serve.
  - `training` — **required** per-model block: `batting`, `bowling`, `fielding`, `extras`, `win` each with `n_estimators`, `max_depth`, `random_state`, `joblib_compress`; optional `estimator` (rf/gb/stacked/quantile), `learning_rate`, `quantile_level`.
  - `feature_defaults` (optional) — defaults when go-app feature map omits keys: `common` (weather/context), `fielding`.
  - `tuning` (optional) — for auto_tune: `cv_splits`, `n_iter`, `scoring`, `algorithms` (rf, gb, quantile, stacked or "all"), `validation_method` (kfold or walk_forward).
  - `walk_forward` (optional) — for walk-forward: `initial_cutoff`, `window_x`, `registry_path`.
  - `prediction_defaults` — e.g. `economy` (default 6.0).
  - `team_prediction` — `team_size` (11), `max_wickets_per_innings` (10).

**Environment:** `ML_SERVICE_CONFIG`, `ML_SERVICE_OUTPUT_DIR`, `MODELS_DIR`, `GO_APP_OUTPUT_DIR`, `ENABLE_HOT_RELOAD`, `ML_N_JOBS`, `ML_N_JOBS_MAX`, `ML_MEMORY_LIMIT_MB`.

**Artifacts naming:** `batting_scaler_<FORMAT>.joblib`, `batting_model_<FORMAT>.joblib` (same for bowling, fielding, etc.); legacy unsuffixed names when format is omitted.

**Serving:** When `format` is present in feature rows, all rows must share that format and a model for it must be loaded; otherwise legacy artifacts are used or an error is returned.

---

## Cricsheet import

**Why “imported” count can be less than files on disk**

1. **Fail-fast (default):** On the first file that errors (parse, unsupported `match_type`, DB error), the run stops. Count = files imported before that failure.
2. **Only top-level files:** Only direct children of the input directory are considered; subdirectories are not recursed.
3. **Supported match types:** Only `info.match_type` in: `TEST`, `MDM`, `ODI`, `ODM`, `T20`, `T20I`, `IT20`. Any other (e.g. `Friendly`, `T10`) causes an error and with fail-fast stops the run.

**Finding and fixing failing files**

- Re-run and check logs for the first error (filename and message).
- To import the rest and list skipped files, run with fail-fast off:

  ```bash
  make cricsheet-import FAIL_FAST=0
  ```
  Or: `go run ./go-app/cmd/cricsheet-importer -in=../data/go-app/cricsheet -fail-fast=false`

  Skipped files are logged; fix or remove them and re-run.

**Scope:** Input dir = `-in` / `GO_APP_INPUT_DIR`. One file = one match; count = files that completed `ImportMatchFile` without error. Importer: `go-app/cmd/cricsheet-importer`, `go-app/internal/cricsheet/ingest.go`; format: `go-app/internal/cricsheet/format.go` (`DetectFormat`).

---

## Export schemas and ML input mapping

**Goal:** go-app dataset exports match ML service expected inputs (column order and types).

**Sources:** Exporter: `go-app/cmd/export-dataset/main.go`. ML: `ml-service/ml/dataset_definitions.py`, `configs/feature_vectors.json`.

**Outputs:** Legacy: `batting_encoded.csv`, `bowling_encoded.csv`. Per-format: `batting_encoded_<FORMAT>.csv`, `bowling_encoded_<FORMAT>.csv` (FORMAT ∈ TEST, ODI, T20, T20I).

**Batting:** ML expects (in order) consistency, form, temp, wind, rain, humidity, cloud, pressure, viscosity, inning, session, toss, venue, opposition, season, player_name. Exporter provides these via feature tables and weather/context; `viscosity_encoded` (0/1), `session_encoded` (1..3), venue/opposition aggregates. Use COALESCE for non-null numerics.

**Bowling:** Same pattern with bowling_* names; `batting_inning` shared; bowling_venue, bowling_opposition, bowling_session.

**Validation:** Run `make -C ml-service validate-exports` (checks headers/types against golden). CI runs this. Keep column order and encodings (session, toss, viscosity) stable in exporter and ML config.
