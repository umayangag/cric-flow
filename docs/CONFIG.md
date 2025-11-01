### Configuration overview

This document lists configuration keys for both the Go application (`go-app`) and the Python ML service (`ml-service`), along with precedence rules and examples.

#### Precedence rules
- Flags/CLI args
- Environment variables
- Component `config.json`
- Built-in defaults in code

---

### Go application (go-app)

Config file: `go-app/config.json`

Keys:
- `inputs`
  - `cricsheet_dir` — default directory containing Cricsheet JSON data. Used by `cmd/cricsheet-importer`.
  - `etl_dir` — default directory for curated CSVs for the optional `etl-importer` path.
- `outputs`
  - `export_dir` — directory where `cmd/export-dataset` writes exported CSVs.
- `formats`
  - `treat_t20i_as_subset` (bool) — when true, treat T20 matches between two international teams as `T20I`.
  - `international_teams` (list of strings) — list of ICC national teams used by the subset rule.
- `features`
  - `precompute_timeout_ms` (int) — timeout for `cmd/precompute` operations.
  - `min_batting_innings` (int) — minimum innings threshold for batting aggregates (reserved for future smoothing).
  - `min_bowling_innings` (int) — minimum innings threshold for bowling aggregates (reserved for future smoothing).
  - `form_shrinkage_alpha` (float) — shrinkage/regularization parameter for form (reserved for future smoothing).
  - `consistency_per_format` (bool) — if true, compute format-aware consistency (table can be introduced later).
- `export`
  - `split_by_format` (bool) — when true, `cmd/export-dataset` writes per-format CSVs by default.
  - `required_format` (string) — when set, exporter writes only this format unless overridden by flags.

Environment variables:
- `GO_APP_CONFIG` — path to an alternate `config.json`.
- `GO_APP_INPUT_DIR` — default input dir for cricsheet importer.
- `GO_APP_OUTPUT_DIR` — default output dir for exporter.

CLI examples:
- Precompute for specific formats: `go run ./go-app/cmd/precompute -season=2019 -formats=ODI,T20I`
- Export per-format datasets: `go run ./go-app/cmd/export-dataset -all-formats`

---

### Python ML service (ml-service)

Config file: `ml-service/config.json`

Keys:
- `inputs`
  - `go_app_export_dir` — directory where Go exports CSVs (read by training scripts).
- `outputs`
  - `artifacts_dir` — directory where training scripts write joblib artifacts and where FastAPI loads from.
- `ml`
  - `formats` — list of format codes to train/serve (e.g., `["TEST","ODI","T20","T20I"]`).
  - `random_state`, `cv_splits`, `test_size`, `n_estimators`, `max_depth`, `learning_rate`, `subsample`, `colsample_bytree`, `reg_lambda`, `reg_alpha`, `early_stopping_rounds`, `max_iter` — hyperparameters reserved for use by training scripts (current model uses a subset; others are placeholders for future models).
  - `artifact_template` — naming template for saved artifacts (informational).

Environment variables:
- `ML_SERVICE_CONFIG` — path to an alternate `config.json`.
- `ML_SERVICE_OUTPUT_DIR` — artifacts directory override at runtime.
- `MODELS_DIR` — legacy env var also recognized as an artifacts directory override.
- `GO_APP_OUTPUT_DIR` — training scripts use this to locate exported CSVs if not specified via `--csv`.

CLI examples:
- Train all configured formats (from `ml-service` directory): `make train-all`
- Serve FastAPI (hot reload): `make run`

Artifacts naming:
- Batting: `batting_scaler_<FORMAT>.joblib`, `batting_model_<FORMAT>.joblib`
- Bowling: `bowling_scaler_<FORMAT>.joblib`, `bowling_model_<FORMAT>.joblib`
- Legacy (no format provided): `batting_scaler.joblib`, `batting_model.joblib`, `bowling_scaler.joblib`, `bowling_model.joblib`

Request requirements (serving):
- Prediction endpoints accept a batch of features; when `format` is provided in the feature rows, all rows must share the same format, and a model for that format must be loaded.
- If `format` is omitted, the service will attempt to use legacy (unsuffixed) artifacts; otherwise returns a clear error.

---

### End-to-end per-format run (quickstart)
1) Migrate and import:
- `go run ./go-app/cmd/cricsheet-importer -dir ../data/go-app/cricsheet`
2) Precompute (per format):
- `go run ./go-app/cmd/precompute -season=2019 -formats=ODI,T20I`
3) Export (per format):
- `go run ./go-app/cmd/export-dataset -formats=ODI,T20I`
4) Train models (per format):
- `make -C ml-service train-all`
5) Serve models:
- `make -C ml-service run`
6) Predict teams (example):
- `go run ./go-app/cmd/team-predictor -match=<MATCH_ID> -format=ODI`
- `go run ./go-app/cmd/team-predictor -match=<MATCH_ID> -format=T20I`
