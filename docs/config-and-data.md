# Configuration and data

This document covers configuration for go-app and ml-service, Cricsheet import behavior, and export-schema alignment with ML.

---

## Configuration overview

**Precedence:** Flags/CLI args → environment variables → component `config.json` → built-in defaults.

---

## Go application (go-app)

Config file: `go-app/config.json`

**Keys:**
- `inputs`
  - `cricsheet_dir` — default directory for Cricsheet JSON (used by cricsheet-importer).
  - `etl_dir` — default directory for curated CSVs (optional etl-importer path).
- `outputs`
  - `export_dir` — where `export-dataset` writes CSVs.
- `formats`
  - `treat_t20i_as_subset` (bool) — treat T20 between international teams as T20I.
  - `international_teams` (list) — ICC national teams for the subset rule.
- `features`
  - `precompute_timeout_ms` (int) — timeout for precompute/import/export (default 86400000). 0 = no deadline.
  - `min_batting_innings`, `min_bowling_innings`, `form_shrinkage_alpha`, `consistency_per_format`, `history_window_matches` — reserved or optional.
  - **Feature extraction:** `ewm_alpha` (0.3), `ewm_alpha_short` (0.5), `ewm_alpha_long` (0.2), `consistency_last_n` (10), `form_window_n` (0), `momentum_last_n` (5).
  - `fielding_enrich` — when ML has no fielding model: `ewm_alpha`, `form_to_catches_ratio` (0.7).
- `pipeline` (optional) — `precompute_concurrency`, `import_concurrency`, `seqcalc_concurrency`, `export_concurrency`, `fielding_concurrency` (0 = auto from GOMEMLIMIT/cgroup).
- `export`
  - `split_by_format` (bool) — write per-format CSVs by default.
  - `required_format` (string) — restrict export to this format unless overridden by flags.

**Environment:** `GO_APP_CONFIG`, `GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`, `PRECOMPUTE_CONCURRENCY`, `IMPORT_CONCURRENCY`, `SEQCALC_CONCURRENCY`, `EXPORT_CONCURRENCY`, `FIELDING_CONCURRENCY`.

**Team selection** (under `team`): `min_bowlers`, `default_batters`, `default_bowlers`. Under `selection`: `score_weights` (bat, bowl, field, keeper_bonus), `score_normalization` (per-format divisors), `score_weights_by_format`, `meta_model_path` (optional JSON from combination-meta training), `use_optimizer` (bool, default false). When `use_optimizer` is true, team selection uses constrained optimization to maximize total score over valid XIs (size 11, ≥1 keeper, ≥5 bowlers); when false, uses greedy selection with constraint swaps. See **ml-and-training.md** for meta-model.

**Future-match prediction features:** The feature map for team-selection prediction includes form, venue, opposition, season, optional weather override (`WeatherOverride`), and sequence features (bat_*, bowl_* from `configs/feature_vectors.json`). Sequence features are set to 0 until precompute/seqcalc export them per player. Opposition strength (`opposition_batting_strength`, `opposition_bowling_strength`) is computed from the opposition team’s pool when available and added to the map (training does not yet include these; when extended, accuracy can improve). Weather: training joins `weather_data` (may be empty); prediction accepts optional `Weather` in the API. When a weather source is added, use the same feature names (e.g. `batting_temp`) in training and prediction.

---

## Python ML service (ml-service)

Config file: `ml-service/config.json`

**Keys:**
- `inputs` — `go_app_export_dir`, `training_data_fetch_timeout_sec` (default 3600).
- `outputs` — `artifacts_dir`.
- `ml`
  - `formats` — list of format codes to train/serve.
  - `training` — **required** per-model block: `batting`, `bowling`, `fielding`, `extras`, `win` each with `n_estimators`, `max_depth`, `random_state`, `joblib_compress`; optional `estimator` (rf/gb/stacked/quantile), `learning_rate`, `quantile_level`.
  - `feature_defaults` (optional) — defaults when go-app feature map omits keys: `common` (weather/context), `fielding`.
  - `tuning` (optional) — for auto_tune: `cv_splits`, `n_iter`, `scoring`, etc.
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
