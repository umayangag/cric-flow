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

- **Extras:** `format_id`, `venue_id`, `season_id`, match-level **weather** (temp, wind, rain, humidity, cloud, pressure, viscosity), and **match-level aggregates** of player features: `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum` (sums of `batting_std_w10`/`bowling_std_w10` for consistency and `batting_mean_w5`/`bowling_mean_w5` for form from `feature_raw_stats_snapshots` as of match date).
- **Win:** Same weather columns plus **team-level aggregates**: team1 = batting in inning 1, team2 = bowling in inning 1; `team1_bat_consistency_sum`, `team1_bowl_consistency_sum`, `team2_bat_consistency_sum`, `team2_bowl_consistency_sum`, and the corresponding `*_form_sum` columns. Original IDs (format_id, venue_id, team1_opposition_id, team2_opposition_id, toss_winner_opposition_id) remain.

At prediction time, to use the trained extras or win model, callers must supply the same feature vector (e.g. format, venue, season/teams, weather, and the relevant consistency/form aggregates for the selected XI or match). The training scripts accept both the extended columns and the legacy subset (only IDs); missing columns are omitted from X.

---

## Data normalization and best practices

- **No future leakage:** Training uses only matches with `match_date < cutoff`. Same cutoff logic for export and for feature computation at prediction.
- **Same feature computation:** go-app uses identical logic for export rows and for feature map at prediction (`ComputeFeaturesAtCutoffForMatch`). Form = EWM, consistency = coefficient of variation, venue/opposition = EWM at scope. The v2 contract includes raw windowed stats (e.g. batting_mean_w3, bowling_std_w10) from `feature_raw_stats_snapshots` (overall scope); export and prediction both populate these from precomputed snapshots when available.
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

## Test coverage and CI gates (ML service)

**Goal:** ML‑service coverage gates should reflect the quality of **API and inference‑time code**, without being dominated by long‑running offline training/tuning CLIs.

- **Coverage configuration:**
  - `pyproject.toml` configures coverage to track `app` and `ml` packages, with `branch = true`.
  - Training/tuning entrypoints that are exercised via separate flows are excluded via `omit`:
    - `ml/train_*.py` (per‑model training CLIs, including `train_batting`, `train_bowling`, `train_extras`, `train_win`, `train_innings`, `train_fielding`, `train_combination_meta`, etc.),
    - `ml/training_pipeline.py` (shared training helpers),
    - `ml/tuning/*.py` (auto‑tune orchestration),
    - `ml/walk_forward.py`,
    - `ml/validate_exports.py`.
  - This keeps the coverage number focused on `app.main`, `app/prediction_service.py`, reconciliation (`ml/reconciliation_*`), consistency checking, config, and other request‑time paths.

- **Thresholds and CI integration:**
  - `ml-service/Makefile` defines `COV_MIN`, the minimum allowed coverage percentage for local `make coverage-check`.
  - The root `Makefile` exposes this as `COV_MIN_ML` so `make ml-service-check`/`make check-all` use the same gate.
  - `.github/workflows/ml-service-ci.yml` passes `COV_MIN` into `make -C ml-service coverage-check` in CI. **All three must stay in sync**; when overall coverage improves, raise all three together (e.g. from 57 → 74) and re‑run the gate locally before pushing.

- **Raising, never lowering:**
  - When `coverage` reports that actual coverage is above the current threshold, we **bump the threshold up to `floor(actual)`** (e.g. 74.99% → 74) in `ml-service/Makefile`, the root `Makefile`, and the CI workflow.
  - We do **not** lower thresholds; if coverage regresses below the gate, the fix is to add or repair tests.

The same pattern applies to other components:

- **Frontend:** Vitest coverage thresholds live in `frontend/vite.config.ts` under `test.coverage` (lines, functions, statements, branches). Whenever we meaningfully improve tests, we raise each threshold to the floor of the corresponding metric.
- **Go app:** Go coverage gates use `COV_MIN` in `go-app/Makefile`, mirrored as `COV_MIN_GO` in the root `Makefile` and as the `COV_MIN` env var in `.github/workflows/go-app-ci.yml`. As with ML service, raise these only when coverage improves.

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

## Feature contract v2 (raw windowed stats)

**Contract version:** `configs/feature_vectors.json` and go-app use version **"2"**. Batting and bowling include **18 raw windowed stats** per type (e.g. `batting_mean_w3`, `batting_std_w10`, `batting_last_1`, …) alongside the existing formula features (form, form_short, form_long, momentum, consistency). These are computed in Go (`features.WindowedStats`) and stored in `feature_raw_stats_snapshots`; export and prediction emit them so the ML model can learn optimal combinations instead of fixed EWM/CV formulas.

**Comparing feature sets:** To evaluate (a) old-only, (b) new-only, (c) combined:

1. **Export** with v2 (current export already includes both formula and raw stats).
2. **Old-only:** Temporarily restrict `FEATURE_COLS` in `ml/train_batting.py` / `ml/train_bowling.py` to the 5 formula + env/context columns (no raw stat names), then train and record metrics.
3. **New-only:** Restrict to raw stat names + env/context (no form/consistency/momentum), train and record metrics.
4. **Combined:** Use current `FEATURE_COLS` (formula + raw + env), train and record metrics.

Use **walk-forward** and **feature importance** (e.g. from `TrainingPipeline.extract_feature_importance` or auto-tune report) to compare and to identify low-signal raw stats.

**After evaluation:** If new (or combined) features improve accuracy:

- **Deprecate** old form/consistency/momentum from the feature contract and from export/prediction (remove from `feature_vectors.json` and Go contract).
- **Prune** raw stats with very low importance if needed to reduce dimensionality (especially for the win model).
- **Clean up** Go precompute: stop computing and storing the old form/consistency snapshots once no consumer uses them; optionally simplify `WindowedStats` to only the windows/stats that matter.

Until then, both formula and raw stats remain in the contract and in precompute for phased rollout.

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

---

## Match-level derived features and model sidecars

**Derived features** (`ml.match_level_derived_features`) are computed in one place and reused by `train_innings`, `train_extras`, and the inference path in `app.reconciliation`:

- `form_differential = bat_form_sum - bowl_form_sum`
- `consistency_differential = bat_consistency_sum - bowl_consistency_sum`
- `weather_composite = wr · rain + wh · (humidity / 100) + wc · (cloud / 100)`

The three `weather_composite_*_weight` values live under `ml.match_level_derived` in config.

**Caveats — `weather_composite`:**

1. It is a **linear, hand-picked blend** of three raw weather features that are *also* kept in the feature list. Tree models can learn interactions from the raw features on their own; the composite is justified only for linear/kernel models that can’t. Treat it as optional and A/B-test whether dropping the raw weather cols (or the composite) improves validation MAE before committing to it.
2. Because the weights are config-driven, they must match between training and inference. From this change onward each trained artifact writes a **sidecar file** next to the joblib (e.g. `innings_meta_T20.json` / `extras_meta.json`) containing `feature_names` and `derived_weights`. `app/reconciliation.build_innings_feature_vector` reads this sidecar at inference time, so retuning the config after training does not silently drift predictions.

**Artifact sidecars (`ml.artifact_sidecar`)**:

| File | Written by | Read by |
|------|------------|---------|
| `innings_meta_<FMT>.json` / `innings_meta.json` | `ml.train_innings.train_and_save(_legacy)` | `app.artifacts.reload` → `app.reconciliation.predict_innings` |
| `extras_meta_<FMT>.json` / `extras_meta.json` | `ml.train_extras.train_and_save(_legacy)` | `app.artifacts.reload` (available to prediction code as `EXTRAS_META`) |

The sidecar pins two things:

- `feature_names`: exact column order the scaler/model were fitted on, so per-format `drop_low_variance_columns` and format one-hot exclusion cannot cause a shape mismatch at inference.
- `derived_weights`: the `ml.match_level_derived` block as it was at training time.

**Operational note:** old artifacts without sidecars still load; `build_innings_feature_vector` falls back to `INNINGS_FEATURE_COLS` + the current config. Retrain any per-format model whose training data included format-only columns that were dropped during low-variance filtering so its sidecar is written and inference stops relying on the fallback.

---

## Data-quality: scale-aware low-variance column drop

`ml.data_quality.drop_low_variance_columns` removes effectively constant columns before fitting. The threshold is **scale-aware**: a column is dropped when `std ≤ threshold · (|mean| + 1)`. The `+ 1` term gives a sensible bar for zero-mean features (like `form_differential`) while still flagging tiny noise on large-mean ones (like `match_date_unix`). The knob lives under `ml.data_quality.low_variance_threshold` (default `1e-6`); values are coefficients, not absolute variance thresholds. Weather fields are protected by default — they are often empty historically but will be populated over time.

---

## Fielding per-inning migration

Migration `0095_fielding_data_inning_number.sql` adds `inning_number` to `fielding_data` (default 1) and replaces the unique constraint with `(match_id, inning_number, player_id)`. Migration `0096_backfill_fielding_data_inning_number.sql` then recomputes per-inning aggregates from `fielding_event` where event-level data exists, preserving manually entered `dropped_catches` and `missed_run_outs` on inning 1 (these are not tracked in `fielding_event`).

Matches with no `fielding_event` data retain their pre-existing single-row representation at `inning_number = 1`. If per-inning event data is ingested later for such matches, running `RecomputeFieldingAggregates` (see `go-app/internal/db/repo_fielding_event.go`) will split the aggregates correctly.

---

## Loader contract: `LoaderResult`

All tuning data loaders in `ml.tuning.data_loaders` return a typed envelope:

- Single-pack loaders (batting, bowling): `LoaderResult(X, Y, feature_names, sample_weight=None)`.
- Per-format loaders (extras, win, fielding, innings): `Dict[str, LoaderResult]`, keyed by uppercase format code (e.g. `T20`, `ODI`, `TEST`, `OTHER`). Extras additionally emits a special `_LEGACY_` key holding the aggregated unified-model pool.

Call sites in `ml.tuning.cli` consume `result.X / result.Y / result.feature_names / result.sample_weight` directly; the previous `unpack_xy_with_feature_names` / `_extras_feature_names_if_consistent` helpers have been removed. To add a new loader, return a `LoaderResult` (or `Dict[str, LoaderResult]`) from the outset — it keeps optional fields explicit and prevents shape drift between training and tuning.

---

## Pending validation work

The following is **not** yet verified in this branch and is deliberately left as an operator follow-up because it requires a populated training DB and non-trivial auto-tune time:

- **Feature transforms A/B (`config.json` → `feature_transforms`)**: `add_log1p` for `*_career_count`, `*_days_since_last`, `*_innings_in_last_90d` and three hand-picked interactions per side are enabled for batting/bowling. Before accepting them as defaults, run `make auto-tune` per format with and without transforms (toggle `feature_transforms.batting.add_interactions` / `add_log1p` and the matching bowling block) on the same cutoff and compare:
  - `best_cv_score` (lower MAE is better).
  - `mlqa_audit.checks.overfitting.relative_delta` and `mlqa_audit.checks.stability.relative_cv_std`.
  - Per-fold CV score spread.

  If transforms help, migrate the block from `config.json` (environment-local override) to `config.default.json` so it ships with defaults. If they don't, remove them to avoid the redundant feature-name plumbing cost at inference.

- **Weather-composite ablation**: `weather_composite = wr·rain + wh·humidity/100 + wc·cloud/100` is a hand-picked linear blend. The `form_differential` and `consistency_differential` columns are pure subtractions of features the model also sees. Tree-based models can (and usually do) recover these from raw columns on their own. Run the same auto-tune sweep with and without `form_differential` / `consistency_differential` / `weather_composite` and keep only the ones that improve hold-out MAE or MLQA stability; drop the rest from `MATCH_LEVEL_DERIVED_FEATURE_COLS`. Note that `resolve_weights` now logs a warning if the resolved weight sum falls outside `[0, 1.5]`, to catch accidental weight drift during experimentation.
