# ML models and training

This document describes the ML models, data normalization, pipeline training (per-format and unified), auto-tune, walk-forward, calibration, and the combination meta-model.

---

## Combined models for prediction

| Model       | Level   | Outputs                                                                 | Used in                          |
|------------|---------|-------------------------------------------------------------------------|----------------------------------|
| Batting    | Player  | runs, balls, fours, sixes, batting_position, strike_rate                | Backtest, team selection score   |
| Bowling    | Player  | runs, deliveries, wickets, economy                                     | Backtest, team selection score   |
| Fielding   | Player  | catches, run_outs (stumpings in training)                               | Backtest, team selection score   |
| Extras     | Match   | total extras per match                                                  | Match aggregates (or historical)  |
| Win        | Match   | winner / team1_wins                                                     | Match outcome                    |
| Combination| —       | —                                                                       | Not a separate model; team selection uses batting + bowling + fielding scores and constraints (min bowlers, keeper). Optional extras/win improve aggregates and outcome. |

**Training data (go-app):** `GET /api/backtest/training-data?cutoff=...&format=all` returns batting, bowling, fielding, extras, win (headers + rows). Fielding/extras/win use cutoff and format.

**Pipeline order:** Precompute → export-dataset → train models → run (or restart) ML service. Batting/bowling use exported CSVs; fielding/extras/win can use API with cutoff.

**Training commands (from repo root or ml-service):** `make train-batting`, `make train-bowling`, `make train-fielding CUTOFF=<RFC3339>`, `make train-extras`, `make train-win`, `make train-all`. Fielding/extras/win need `GO_APP_URL` (and optionally `CUTOFF` or CSV path).

**Artifacts:** Per-format: `batting_scaler_<FMT>.joblib`, `batting_model_<FMT>.joblib` (same for bowling, fielding, extras, win). Legacy: unsuffixed names.

**Combined prediction flow:** (1) Player predictions from batting/bowling/fielding models; (2) match aggregates = sum of player preds + extras model if loaded (else historical average); (3) winner from win model or from team totals; (4) team selection = greedy selection with batting/bowling/fielding scores and constraints. When fielding artifacts are not loaded, go-app falls back to **enrichFieldingFromHistory** (EWM of historical fielding).

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

## Auto-tune

**Purpose:** Find best algorithm and hyperparameters per model (batting, bowling, fielding) via RandomizedSearchCV over RandomForest and GradientBoosting; save best scaler+model in the same artifact format; optionally write a tuning report with `config_snippet` for `ml.training.<model>`.

**Config:** In `ml-service/config.json`, optional `ml.tuning`: `cv_splits`, `n_iter`, `scoring` (e.g. `neg_mean_absolute_error`).

**Run:** From ml-service: `python -m ml.auto_tune --model batting --format T20` (or from CSV with `--csv`). From repo root: `make ml-auto-tune MODEL=batting FORMAT=T20` or `MODEL=all ALL_FORMATS=1`. Can also be triggered via API (e.g. pipeline UI). Copy `config_snippet` into config and re-run normal training.

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
