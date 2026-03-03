# ML models and training

ML models, data normalization, pipeline training (per-format and unified), auto-tune, walk-forward, calibration, and the combination meta-model. **Model inputs/outputs and hyperparameters:** [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## Combined models for prediction

| Model        | Level  | Outputs / role |
|-------------|--------|----------------|
| Batting     | Player | runs, balls, fours, sixes, batting_position, strike_rate → backtest, team score |
| Bowling     | Player | runs_conceded, deliveries, wickets, economy → backtest, team score |
| Fielding    | Player | catches, run_outs, stumpings → backtest, team score |
| Extras      | Match  | total_extras → match aggregates (or historical average) |
| Win         | Match  | team1_win_probability → match outcome |
| Combination | —      | Learned weights for bat/bowl/field scores; not a separate model. Team selection uses constraints (≥1 keeper, ≥5 bowlers). |

Input dimensions, estimators, and aggregation (e.g. sum runs, win prob) are in [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

**Training data (go-app):** `GET /api/backtest/training-data?cutoff=...&format=all` returns batting, bowling, fielding, extras, win (headers + rows). Fielding/extras/win use cutoff and format.

**Pipeline order:** Precompute → export-dataset → train models → run (or restart) ML service. Batting/bowling use exported CSVs; fielding/extras/win can use API with cutoff.

**Training commands (from repo root or ml-service):** `make train-batting`, `make train-bowling`, `make train-fielding CUTOFF=<RFC3339>`, `make train-extras`, `make train-win`, or `make train-all` to run all train steps in sequence. Each step uses params from config and, when `GO_APP_URL` is set, from the go-app tuned-params DB. Fielding/extras/win need `GO_APP_URL` (and optionally `CUTOFF` or CSV path).

**Artifacts:** Per-format: `batting_scaler_<FMT>.joblib`, `batting_model_<FMT>.joblib` (same for bowling, fielding, extras, win). Legacy: unsuffixed names.

**Combined prediction flow:** (1) Player predictions from batting/bowling/fielding models; (2) match aggregates = sum of player preds + extras model if loaded (else historical average); (3) winner from win model or from team totals; (4) team selection = greedy selection with batting/bowling/fielding scores and constraints. When fielding artifacts are not loaded, go-app falls back to **enrichFieldingFromHistory** (EWM of historical fielding).

**Unified features for extras and win:** Extras and win models use the same feature families as batting, bowling, and fielding so that player quality and context influence match-level predictions. Training data from go-app includes:

- **Extras:** `format_id`, `venue_id`, `season_id`, match-level **weather** (temp, wind, rain, humidity, cloud, pressure, viscosity), and **match-level aggregates** of player features: `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum` (sums over all players who batted or bowled in the match, from `feature_consistency_snapshots` / `feature_form_snapshots` as of match date).
- **Win:** Same weather columns plus **team-level aggregates**: team1 = batting in inning 1, team2 = bowling in inning 1; `team1_bat_consistency_sum`, `team1_bowl_consistency_sum`, `team2_bat_consistency_sum`, `team2_bowl_consistency_sum`, and the corresponding `*_form_sum` columns. Original IDs (format_id, venue_id, team1_opposition_id, team2_opposition_id, toss_winner_opposition_id) remain.

At prediction time, to use the trained extras or win model, callers must supply the same feature vector (e.g. format, venue, season/teams, weather, and the relevant consistency/form aggregates for the selected XI or match). The training scripts accept both the extended columns and the legacy subset (only IDs); missing columns are omitted from X.

---

## Data normalization and best practices

- **No future leakage:** Training uses only matches with `match_date < cutoff`. Same cutoff logic for export and for feature computation at prediction.
- **Same feature computation:** go-app uses identical logic for export rows and for feature map at prediction (`ComputeFeaturesAtCutoffForMatch`). Form = EWM, consistency = coefficient of variation, venue/opposition = EWM at scope.
- **Feature order:** Training and prediction use the same order from `configs/feature_vectors.json` (batting, bowling, fielding). ML builds the vector from this config at prediction.
- **Input normalization (X):** `StandardScaler` fitted only on training data; same scaler saved and used at prediction. No test/future data in fit.
- **Targets (Y):** Kept in raw units (no scaling) for interpretability and to avoid inverse transform.
- **Missing values:** Training drops or fills (e.g. 0) per script; prediction uses `ml.feature_defaults` in config for missing keys.

---

## Pipeline: per-format and unified training

You can run the full pipeline from the **frontend** (Ops Status → Pipeline) or from the **command line**. Each train step produces **both** per-format and unified (legacy) models.

**Pipeline steps:** (1) Import — migrate and import Cricsheet. (2) Precompute — form/consistency/sequence per format. (3) Export — with `split_by_format: true`, writes unified (`batting_encoded_all.csv`, etc.) and per-format CSVs. (4) Train Batting — per-format then unified (legacy artifacts). (5) Train Bowling — same. (6) Train Fielding — API data, per-format + unified. (7) Train Extras, (8) Train Win — same pattern. (9) Optional: Auto-tune (from UI or API).

**Prerequisites:** Stack running (`make dev-up`). `export.split_by_format: true` in go-app config. For fielding/extras/win: `GO_APP_URL` set for ML service. Cutoff for those steps: default UTC now, or API param `?cutoff=...`.

**CLI:** `make precompute-all-all-formats`, `make export-dataset`, `make train-batting`, `make train-bowling`, `make train-fielding CUTOFF=...`, `make train-extras`, `make train-win`. Same outcome: per-format and legacy artifacts. ML loads them and uses per-format when request has format; falls back to legacy when format missing or no per-format model (e.g. fielding).

---

## Pipeline modes: params known vs unknown

**Single-train principle:** Train each model **once** with the params you intend to use. Params come from `ml-service/config.json` (`ml.training.<model>`) and, when `GO_APP_URL` is set, are **overlaid** by tuned params stored in the go-app DB (from a previous auto-tune). So you either train with known params (config + DB) or run auto-tune to discover params, then train once with those.

### Mode A — Params known (fast path)

When you already have good hyperparameters (in config or from a previous auto-tune saved to DB):

1. **Import** → **Precompute** → **Export** → **Train all** (batting, bowling, fielding, extras, win, innings).
2. Do **not** run auto-tune. Each train step reads params from config and, when available, from the go-app tuned-params API; one pass produces all artifacts.

Use this for routine retrains (e.g. after new data or a fixed cutoff) when you are not re-optimizing hyperparameters.

### Mode B — Params unknown or re-optimizing (tuning path)

When you need to discover or refresh best algorithm and hyperparameters:

1. **Import** → **Precompute** → **Export** → **Auto-tune** (per model/format or all).
2. Auto-tune finds best algorithm + hyperparameters, saves params to the go-app DB (and writes artifacts). Optionally run **Train all** afterward so every artifact is produced by the same train scripts using the new DB params (single code path for artifacts).

Use this when setting up a new format, after major data changes, or when you want to re-run algorithm screening or Optuna fine-tuning.

**Summary:** Train = produce artifacts from current params (config + DB). Auto-tune = discover and persist params (and optionally artifacts). Avoid running train with defaults and then auto-tune for the same models; choose one of the two modes above.

---

## Precompute and feature parameters

Feature-engineering parameters (go-app config: `features.ewm_alpha`, `features.consistency_last_n`, `form_window_n`, `momentum_last_n`, etc.) control how form, consistency, and venue/opposition features are computed. They are used in **precompute** and in the export/training-data path (`GetFeatureExtractionParams()`). Changing them changes the feature space, so you must **re-precompute → re-export → re-train** (or re-auto-tune). There is no joint optimization of precompute params and model params in one run; treat precompute-param tuning as a separate, slower loop (e.g. change config → precompute → export → train/eval → compare metrics). See **config-and-data.md** for the full list of `features.*` keys.

---

## Auto-tune

**Purpose:** Two-phase coarse-to-fine search: (1) **Algorithm screening** — coarse search over RF, GBM, ExtraTrees, HistGradientBoosting, quantile, stacked to pick the best; (2) **Fine-tuning** — Optuna TPE on the winner(s) for converging hyperparameter optimization. Saves best scaler+model in the same artifact format; writes live progress to a JSON file for frontend display (phase, algorithm, hyperparams, trial).

**Config:** In `ml-service/config.json`, optional `ml.tuning`: `cv_splits`, `n_iter`, `scoring` (e.g. `neg_mean_absolute_error`), `algorithms`, `validation_method`.

- **algorithms** — `"all"` or a list like `["rf", "gb"]`. Available: `rf` (RandomForest), `gb` (GradientBoosting), `et` (ExtraTrees), `hgb` (HistGradientBoosting), `quantile` (regression only), `stacked` (batting/bowling/fielding only). Extras and win support `rf`, `gb`, `et`, `hgb`.
- **validation_method** — `"walk_forward"` (default; TimeSeriesSplit, temporal validation) or `"kfold"`.

**Run:** From ml-service: `python -m ml.auto_tune --model batting --format T20` (or from CSV with `--csv`). From repo root: `make ml-auto-tune MODEL=batting FORMAT=T20` or `MODEL=all ALL_FORMATS=1`. **Unified model:** `--unified` tunes one model on all formats combined (saves to legacy names like `win_model.joblib`, `tuning_report_win.json`). Example: `make ml-auto-tune MODEL=win UNIFIED=1 CUTOFF=... ALGORITHMS=mlp` or via API with `unified=1`. Options: `--algorithms rf,gb --validation-method walk_forward` or `make ml-auto-tune MODEL=batting ALGORITHMS="rf,gb" VALIDATION_METHOD=walk_forward`. Use `--parallel` to run multiple (model, format) tasks in parallel, using up to 80% of available CPUs (each subprocess uses one job to avoid oversubscription). Use `--fast` to reduce Optuna trials and skip PyCaret/AutoGluon; `--no-pycaret` to skip PyCaret ranking; `--no-autogluon` to skip AutoGluon. Can also be triggered via API (e.g. pipeline UI). Copy `config_snippet` into config and re-run normal training. When `GO_APP_URL` is set, best params (including `algorithms` and `validation_method`) are saved to the DB.

**Resources:** Auto-tune and training use up to **80%** of available memory (config `ml.resources.memory_usage_fraction_percent`, default 80) and resource-aware `n_jobs` from `ml.resources` and `ml.tuning.n_jobs` (-1 = auto from CPU and memory). Set `AUTO_TUNE_N_JOBS` or `ML_N_JOBS` to override.

---

## Walk-forward

**Purpose:** Evaluate temporal performance: train on data before cutoff → predict next X matches (holdout) → score (e.g. MAE) → record in registry → advance cutoff and repeat. Builds a registry (e.g. `walk_forward_registry.json`) of model type, format, cutoff, window_x, params, metrics.

**Use:** Compare accuracy for different X; find underperforming windows and re-run auto_tune or retrain. Run as a **separate step** from auto_tune. Requires go-app and endpoints: `GET /api/backtest/matches?after=...`, `GET /api/backtest/training-data?cutoff=...`, `GET /api/backtest/holdout-data?cutoff=...&limit=...`.

**Run:** `make walk-forward INITIAL_CUTOFF=... WINDOW_X=50 WALK_FORMAT=T20 WALK_MODEL=batting` (or from ml-service with `GO_APP_URL`).

---

## Probability calibration (classifiers)

For classifiers (e.g. win model), predicted probabilities can be **calibrated** (Platt scaling or isotonic regression) so they reflect true frequencies. Evaluation: reliability diagram, Brier score, ECE. Module: `ml.calibrate` — `calibrate_classifier()`, `reliability_diagram_data()`, `evaluate_calibration()`. Use when you need calibrated probabilities for the win (or other) classifier.

---

## Combination meta-model

**Purpose:** Learn weights for combining batting, bowling, and fielding scores in team selection (instead of fixed weights). Ridge meta-model: inputs (bat_score, bowl_score, field_score, is_keeper, format), target = actual contribution from backtest/evaluate-db.

**Run:** `python -m ml.train_combination_meta --csv path/to/backtest_contributions.csv --out ../output/ml-service/combination_meta.json` (optional `--per-format`, `--alpha`). CSV columns: bat_score, bowl_score, field_score, is_keeper, format, target. go-app loads the JSON when `selection.meta_model_path` is set in config and uses it instead of `score_weights` / `score_weights_by_format`. When `selection.use_optimizer` is true, team selection maximizes total score over valid XIs (same weights from meta-model or config); when false, greedy selection with constraint swaps is used.

---

## Combination meta-model automation

**Goal:** Produce learned weights for team selection from backtest outcomes and optionally run them in the pipeline.

1. **Generate contributions CSV** — Call `POST /api/backtest/export-contributions` with body `{ "format", "team1", "team2", "match_ids": [ ... ] }`. The server starts a background job and returns `202 Accepted` with `job_id`. Poll `GET /api/backtest/export-contributions-status?job_id=<id>` until `status` is `done` or `error`. When done, the response includes `path` and `rows`; the CSV is written to the configured export dir (e.g. `output/go-app/backtest_contributions.csv`).
2. **Train meta-model** — From repo root: `make train-combination-meta CSV=<path-to-csv> OUT=<path-to-json>`, or use the pipeline step “Train Combination Meta” (API returns the exact command with paths). Default paths: CSV = `output/go-app/backtest_contributions.csv`, OUT = `output/go-app/combination_meta.json`.
3. **Use learned weights** — Set `selection.meta_model_path` in go-app config to the output JSON path. Restart or reload config so team selection uses the meta-model weights instead of fixed `score_weights`.
4. **Full pipeline** — `make full-pipeline` runs precompute → export → train all models; if `backtest_contributions.csv` exists in the default location, it also runs train-combination-meta. Override paths with `FULL_PIPELINE_CSV` and `FULL_PIPELINE_OUT`.

---

## Monte Carlo simulation (win probability and outcome distributions)

**Purpose:** Instead of a single predicted scorecard, get **win probability** and **outcome distributions** by sampling over many possible team combinations and match outcomes. Feasible on a domestic PC by limiting the space: top-k XIs per team (not all combinations) and a fixed number of samples per matchup.

**How it works:** (1) Use the same optimizer to get the **top-k** valid XIs per team (when pool size ≤ 18, all valid XIs are enumerated and sorted by score). (2) For each (XI₁, XI₂) pair (up to a cap), sample **N** match outcomes: for each player, sample runs (and optionally wickets/economy) from a distribution around the point prediction (e.g. Normal(mean, mean×CV)). (3) Sum runs + extras per innings, compare to get winner; aggregate over all samples to get win probability and innings total percentiles (P10, P50, P90).

**API:** `POST /api/predict/team-selection` (or GET) with `simulate=true` (or `?simulate=true`). Optional: `simulation_top_k` (default 50), `simulation_samples` (default 500 per matchup), `simulation_max_pairs` (0 = no cap). Response includes `team1`, `team2`, `scorecard_summary` as usual, plus `simulation`: `win_probability_team1`, `win_probability_team2`, `draw_probability`, `innings1_total_mean`, `innings1_total_std`, `innings1_total_p10/p50/p90`, and the same for innings 2, plus `num_matchups` and `num_samples`.

**Resource use:** Example: 50×50 = 2,500 matchups × 500 samples = 1.25M samples; typically completes in under a minute on a modern PC. Reduce `simulation_top_k` or `simulation_max_pairs` for faster responses.
