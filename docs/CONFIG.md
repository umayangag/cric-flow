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
  - `precompute_timeout_ms` (int) — timeout for pipeline jobs: precompute, import, export. Default 86400000 (24 hours). Set to 0 to disable (no deadline). Precompute and large imports/exports can take many hours on big datasets (T20/T20I).
  - `min_batting_innings` (int) — minimum innings threshold for batting aggregates (reserved for future smoothing).
  - `min_bowling_innings` (int) — minimum innings threshold for bowling aggregates (reserved for future smoothing).
  - `form_shrinkage_alpha` (float) — shrinkage/regularization parameter for form (reserved for future smoothing).
  - `consistency_per_format` (bool) — if true, compute format-aware consistency (table can be introduced later).
  - `history_window_matches` (int) — when &gt; 0, limit form/consistency to the last N matches (used by precompute-all).
  - **Feature extraction (tunable):** Used by training-data export and by `cmd/precompute-all` when CLI flags are not overridden.
    - `ewm_alpha` (float, default 0.3) — alpha for exponentially weighted mean (form). Must be in (0, 1].
    - `ewm_alpha_short` (float, default 0.5) — alpha for form_short (more weight to recent innings).
    - `ewm_alpha_long` (float, default 0.2) — alpha for form_long (longer horizon).
    - `consistency_last_n` (int, default 10) — last-N innings window for consistency (coefficient of variation).
    - `form_window_n` (int, default 0) — max number of innings to use for form; 0 = no limit.
    - `momentum_last_n` (int, default 5) — last-N innings for momentum slope (positive = improving form).
  - `fielding_enrich` — used when ML does not return fielding predictions and go-app enriches from history (team selection).
    - `ewm_alpha` (float, default 0.3) — EWM alpha for fielding form.
    - `form_to_catches_ratio` (float, default 0.7) — split of form into catches; run_outs = form × (1 − ratio).
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
  - `training_data_fetch_timeout_sec` — timeout in seconds when fetching training data from go-app (train-on-the-fly, train_fielding, walk_forward). Default 3600 (1 hour). Large datasets may need longer.
- `outputs`
  - `artifacts_dir` — directory where training scripts write joblib artifacts and where FastAPI loads from.
- `ml`
  - `formats` — list of format codes to train/serve (e.g., `["TEST","ODI","T20","T20I"]`).
  - `artifact_template` — naming template for saved artifacts (informational).
  - `training` — **required** per-model block used strictly by all training scripts (no defaults or env overrides in code). Each model has its own parameters so you can tune batting vs bowling (and future models) independently.
    - `batting` — parameters for batting model training and artifacts.
      - `n_estimators` (int), `max_depth` (int), `random_state` (int), `joblib_compress` (int, 0–9).
      - `estimator` (optional, default `rf`) — `rf` for RandomForest, `gb`/`gbm` for GradientBoosting, `stacked` for RF+GB+Ridge ensemble, `quantile` for GBM with pinball loss (prediction intervals).
      - `learning_rate` (optional, for GBM/quantile, default 0.1) — used when `estimator` is `gb` or `quantile`.
      - `quantile_level` (optional, for quantile only, default 0.5) — quantile to predict (0.5 = median). Use 0.05/0.95 for interval bounds.
    - `bowling` — parameters for bowling model training and artifacts.
      - Same keys as `batting`. Add further keys (e.g. `fielding`, `win`) when those models are implemented.
  - `cv_splits`, `test_size`, `learning_rate`, `subsample`, `colsample_bytree`, `reg_lambda`, `reg_alpha`, `early_stopping_rounds`, `max_iter` — reserved for future models.
  - `feature_defaults` (optional) — defaults used when building feature vectors from a sparse go-app map at prediction time (missing keys). Tune these to match “no history” or environment assumptions.
    - `common` — weather/context: `temp`, `humidity`, `wind`, `rain`, `cloud`, `pressure`, `viscosity`, `inning`, `session`, `toss` (same defaults used for batting/bowling/fielding where applicable).
    - `fielding` — `consistency`, `form`, `venue`, `opposition` (used when fielding or venue/opposition keys are missing).
  - `tuning` (optional) — used by `ml.auto_tune`. Keys: `cv_splits`, `n_iter`, `n_jobs`, `random_state`, `scoring`; optionally `search_space` with `rf` and `gb` defining param ranges for RandomizedSearchCV.
  - `walk_forward` (optional) — used by `ml.walk_forward`. Keys: `initial_cutoff`, `window_x`, `registry_path`. See **docs/ML_WALK_FORWARD.md**.
  - `prediction_defaults` (optional) — `economy` (default 6.0 when bowling model returns no economy).
  - `team_prediction` — `team_size` (11), `max_wickets_per_innings` (10, used by player_combinator).

For data normalization, feature computation, and ML practices from import to prediction, see **docs/ML_DATA_AND_NORMALIZATION.md**.

Environment variables:
- `ML_SERVICE_CONFIG` — path to an alternate `config.json`.
- `ML_SERVICE_OUTPUT_DIR` — artifacts directory override at runtime.
- `MODELS_DIR` — legacy env var also recognized as an artifacts directory override.
- `GO_APP_OUTPUT_DIR` — training scripts use this to locate exported CSVs if not specified via `--csv`.
- `ENABLE_HOT_RELOAD` — when set to `1/true/yes`, enables `POST /admin/reload` to rescan and reload artifacts without restarting the server.

CLI examples:
- Train all configured formats (from `ml-service` directory): `make train-all`
- Serve FastAPI (hot reload): `make run`

Artifacts naming:
- Batting: `batting_scaler_<FORMAT>.joblib`, `batting_model_<FORMAT>.joblib`
- Bowling: `bowling_scaler_<FORMAT>.joblib`, `bowling_model_<FORMAT>.joblib`
- Legacy (no format provided): `batting_scaler.joblib`, `batting_model.joblib`, `bowling_scaler.joblib`, `bowling_model.joblib`

Request requirements (serving):
- Prediction endpoints accept a batch of features; when `format` is provided in the feature rows, all rows must share the same format, and a model for that format must be loaded.
- If `format` is omitted, the service will attempt to use legacy (unsuffixed) artifacts; otherwise returns a structured error with a hint.

---

### Team selection configuration (go-app)
New keys in `go-app/config.json` under `team`:
- `min_bowlers` — minimum number of bowlers the selector must include (default 5).
- `default_batters` — default number of batters to pick when `-bat` not provided (default 6).
- `default_bowlers` — default number of bowlers to pick when `-bowl` not provided (default 5 or `min_bowlers`).

Under `selection`:
- `score_weights` — optional weights for combining batting/bowling/fielding signals when selecting best XI: `bat` (default 0.45), `bowl` (0.40), `field` (0.10), `keeper_bonus` (0.02). These affect which players are ranked higher in team selection.
- `score_normalization` — optional per-format divisors for converting raw predictions to [0,1] scores. Keys: format codes (e.g. T20, ODI, TEST). Each value: `bat_divisor`, `wicket_divisor`, `econ_base`, `field_divisor`. T20/ODI/TEST have format-specific typical maxima; defaults apply when absent.
- `score_weights_by_format` — optional per-format overrides for `score_weights`. When set for a format, overrides the global `score_weights` for that format.
- `meta_model_path` — optional path to JSON from `ml.train_combination_meta`. When set, learned weights override `score_weights` and `score_weights_by_format`. See **docs/ML_COMBINATION_META.md**.

CLI overrides still apply: `go run ./go-app/cmd/team-predictor -match=<id> -format=<CODE> -bat=6 -bowl=5`.

---

### End-to-end per-format run (quickstart)
With one command per format or multiple formats:
- Single format: `make e2e FORMAT=ODI SEASON=2019`
- Multiple formats: `make e2e-multi FORMATS=ODI,T20I SEASON=2019`

Or step-by-step:
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
