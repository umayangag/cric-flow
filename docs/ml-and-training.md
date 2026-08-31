# ML models and training

ML models, data normalization, per-format pipeline training, auto-tune, walk-forward, calibration, and the combination meta-model. **Model inputs/outputs and hyperparameters:** [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

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

> **`CUTOFF` bounds the training data, on the CSV path as well as the API one.** Rows
> with `match_date` on or after it are dropped, matching the export's own
> `match_date < cutoff` and leaving everything from the cutoff onward as a holdout.
>
> It did not always. The trainers prefer the export CSV over the API and used to read it
> whole, so `CUTOFF` governed only the fallback nobody takes: a run asked to train to a
> cutoff trained on every exported match. Artifacts built before this fix were trained on
> all available data — check `n_samples` in `win_model_<FMT>_metadata.json` against the
> row count for that format in the export; if they match, there is no holdout and any
> evaluation of that artifact is in-sample.
>
> **To produce a model you can honestly evaluate**, train with a cutoff that leaves a
> window behind it, then pass the same value to `make win-discrimination TRAIN_CUTOFF=`.
> The two are complementary by construction: training keeps rows strictly before, the
> report keeps rows on or after.

**Artifacts:** Always per-format: `batting_scaler_<FMT>.joblib`, `batting_model_<FMT>.joblib` (same for bowling, fielding, extras, win).

**Combined prediction flow:** (1) Player predictions from batting/bowling/fielding models; (2) match aggregates = sum of player preds + extras model if loaded (else historical average); (3) winner from win model or from team totals; (4) team selection = greedy selection with batting/bowling/fielding scores and constraints. When fielding artifacts are not loaded, go-app falls back to **enrichFieldingFromHistory** (EWM of historical fielding).

**Unified features for extras and win:** Extras and win models use the same feature families as batting, bowling, and fielding so that player quality and context influence match-level predictions. Training data from go-app includes:

- **Extras:** `format_id`, `venue_id`, `season_id`, match-level **weather** (temp, wind, rain, humidity, cloud, pressure, viscosity), and **match-level aggregates** of player features: `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum` (sums of `batting_std_w10`/`bowling_std_w10` for consistency and `batting_mean_w5`/`bowling_mean_w5` for form from `feature_raw_stats_snapshots` as of match date).
- **Win:** Same weather columns plus **team-level aggregates**: team1 = batting in inning 1, team2 = bowling in inning 1; `team1_bat_consistency_sum`, `team1_bowl_consistency_sum`, `team2_bat_consistency_sum`, `team2_bowl_consistency_sum`, and the corresponding `*_form_sum` columns. Original IDs (format_id, venue_id, team1_opposition_id, team2_opposition_id) remain. **The toss winner is not a feature**: a side is selected before the toss, so the value is unknowable at decision time and the serving path could only ever send zero (S-2).

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

## Pipeline: per-format training

You can run the full pipeline from the **frontend** (Ops Status → Pipeline) or from the **command line**. Each train step produces per-format models.

**Pipeline steps:** (1) Import — migrate and import Cricsheet. (2) Precompute — form/consistency/sequence per format. (3) Export — writes the cross-format `*_encoded_all.csv` and per-format CSVs. (4) Train Batting — per-format. (5) Train Bowling — same. (6) Train Fielding — API data. (7) Train Extras, (8) Train Win — same pattern. (9) Optional: Auto-tune (from UI or API).

**Auto-tune is placed last on the graph but depends only on Export.** It reads the exported CSVs (batting, bowling) and the training-data API (fielding, extras, win) — the same inputs the train steps read, and no trained artifact. So it can be run before the train steps, and in Mode B below it must be. The graph shows it last because that is where it is offered, not because it is gated behind training.

**Prerequisites:** Stack running (`make dev-up`). Exports are always per-format (the `export.split_by_format` flag was removed in C5-3 — it no longer changed anything). For fielding/extras/win: `GO_APP_URL` set for ML service. Cutoff for those steps: default UTC now, or API param `?cutoff=...`.

**CLI:** `make precompute-all-all-formats`, `make export-dataset`, `make train-batting`, `make train-bowling`, `make train-fielding CUTOFF=...`, `make train-extras`, `make train-win`. Same outcome: per-format artifacts. ML resolves the model by the request's `format`, which is required — there is no fallback tier.

---

## Export width contract

Every trainer and tuning loader reads its `*_encoded_all.csv` through `ml.export_csv.read_export_csv`, which compares the header against the first data row and raises `MisalignedExportError` when the two disagree.

The check exists because pandas stays silent about the one corruption that matters here: a header naming fewer columns than the rows carry makes `read_csv` absorb the surplus leading fields as an index and shift every named column left by that many places. The win export shipped 64 header names over 72-field rows, so `team1_wins` took the values of `team1_bat_consistency_top3_mean`, the target collapsed to a single class, and the run died minutes later inside GradientBoosting complaining about class counts — nowhere near the cause.

If a train step now fails with `header names N columns but the first data row has M fields`, the CSV on disk predates the current exporter. Re-run `make export-dataset`, then re-run the train step.

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

**Run it as the `tune` plan.** `POST /ops/pipeline/run-plan {"plan":"tune"}`, or the Plan dropdown in Ops Status, runs auto-tune followed by every train step in one sequence, with per-step live state, Stop and resume. It assumes the export is current; run `data-refresh` (or `full`) first when it is not.

**Why the search comes first.** Hyperparameters are a function of the feature space. When the feature space has changed — the feature contract, a `features.*` precompute parameter, a new format — the saved params no longer describe an optimum, so training before the search produces artifacts the search invalidates an hour later. Training first is only right in Mode A, where the params are already the ones you mean to use.

**Summary:** Train = produce artifacts from current params (config + DB). Auto-tune = discover and persist params (and optionally artifacts). Avoid running train with defaults and then auto-tune for the same models; choose one of the two modes above.

### How a single-train run is scored

Mode A is the path you run most often, so it has to be falsifiable on its own: without a
score, a model trained from a broken export is indistinguishable in the UI from a good one.

Before fitting the model it ships, `TrainingPipeline.train_and_save` holds back the last
`ml.pipeline_common.holdout_fraction` of the rows (default `0.2`), fits the same recipe on
the rest, and scores the holdout. The split is positional, which is a **time** split
because go-app exports every training CSV `ORDER BY match_date ASC` — the same assumption
the tuning search's `walk_forward` CV already rests on. The scaler and the target clipping
are fitted on the training slice only, and the holdout is scored against its **unclipped**
targets, so the clipping under test cannot flatter the result.

The scores land in the artifact's `<kind>_metadata_<FMT>.json` sidecar alongside
`trained_at`, `duration_seconds` and the algorithm, and the ML Model Stats tab reads them
when no tuning report exists. They are labelled `score_source: holdout` and shown with a
`holdout` chip.

**A holdout score and a tuned score are not comparable.** The tuned figure is
cross-validated over the whole dataset; the holdout is one slice of recent rows. Compare
holdout to holdout across retrains — never a holdout MAE against a tuned MAE. A `holdout`
row is also not audited: the MLQA checks measure a search's fold behaviour, so the Audit
column stays empty until the model is auto-tuned.

**Cost:** one extra fit on ~80% of the rows, so roughly 1.8× the training time. Set
`ml.pipeline_common.holdout_fraction` to `0` to skip it. Runs below 250 rows skip it
automatically — a score from a handful of rows describes the split, not the model.

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

> **`scoring` applies to the regression models only.** The win model is a classifier and
> is tuned for **`roc_auc`**, set in code rather than read from here. Team selection takes
> an argmax over candidate XIs, so only the model's *ranking* of them can change which
> side is picked: a threshold metric like accuracy is blind to every improvement that
> does not cross 0.5, and rewards leaning on the majority outcome. Tuning the win model
> for accuracy can buy a model that selects worse than the one it replaced.
>
> AutoGluon, when enabled, is given the same metric — the two scores are compared with a
> plain `>`, so a different metric on either side would decide the win model on a
> category error rather than a close call.
>
> Tuning reports carry the metric that produced them in their `scoring` field. A report
> from before this change says `accuracy`, and its `best_cv_score` is not comparable with
> a newer one.

- **algorithms** — `"all"` or a list like `["rf", "gb"]`. Available: `rf` (RandomForest), `gb` (GradientBoosting), `et` (ExtraTrees), `hgb` (HistGradientBoosting), `quantile` (regression only), `stacked` (batting/bowling/fielding only). Extras and win support `rf`, `gb`, `et`, `hgb`.
- **validation_method** — `"walk_forward"` (default; TimeSeriesSplit, temporal validation) or `"kfold"`.

**Run:** From ml-service: `python -m ml.auto_tune --model batting --format T20` (or from CSV with `--csv`). From repo root: `make ml-auto-tune MODEL=batting FORMAT=T20` or `MODEL=all ALL_FORMATS=1`. **A format is required** — pass `--format` or `--all-formats`; a run without one exits with a hint, because both the artifact name and the tuned-params row are keyed by format. Options: `--algorithms rf,gb --validation-method walk_forward` or `make ml-auto-tune MODEL=batting ALGORITHMS="rf,gb" VALIDATION_METHOD=walk_forward`. Use `--parallel` to run multiple (model, format) tasks in parallel, using up to 80% of available CPUs (each subprocess uses one job to avoid oversubscription). Use `--fast` to reduce Optuna trials and skip PyCaret/AutoGluon; `--no-pycaret` to skip PyCaret ranking; `--no-autogluon` to skip AutoGluon. Can also be triggered via API (e.g. pipeline UI). Copy `config_snippet` into config and re-run normal training. When `GO_APP_URL` is set, best params (including `algorithms` and `validation_method`) are saved to the DB.

**Resources:** Auto-tune and training use up to **80%** of available memory (config `ml.resources.memory_usage_fraction_percent`, default 80) and resource-aware `n_jobs` from `ml.resources` and `ml.tuning.n_jobs` (-1 = auto from CPU and memory). Set `AUTO_TUNE_N_JOBS` or `ML_N_JOBS` to override.

---

## Win-model discrimination report

**Purpose:** answer "does the win model rank teams at all?" on matches it never trained on,
before spending effort on the search that maximises its output.

**Run:** `make win-discrimination TRAIN_CUTOFF=2024-01-01T00:00:00Z` (optionally
`EVAL_CUTOFF=...`; needs `GO_APP_URL`). Writes `win_discrimination.json` next to the
artifacts and logs a per-format table of **AUC**, **Brier** and a reliability curve.

The holdout is every exported match on or after `TRAIN_CUTOFF`. Features come from the
trainer's own frame builder, and the columns come from each model's
`win_model_<FMT>_metadata.json` — not re-derived, because the trainer's low-variance filter
is fitted on the training batch and would select differently here.

**Reading it.** AUC is the number that matters for selection: the optimiser takes an argmax,
so only the model's *ranking* affects which XI it picks. An AUC near 0.5 means the search is
maximising noise. Brier and the reliability curve describe the probability that gets
*displayed*; no monotone recalibration can change an argmax, so poor calibration alone is not
a reason to distrust a selection.

Formats that cannot be scored — no artifact, no metadata sidecar, a one-sided window, a model
returning one constant probability — are listed with the reason rather than omitted.

---

## XI-responsive win model (`ml.xi`)

**Purpose:** a win model whose every input is a function of the two elevens, so it can rank
candidate XIs — the objective for team selection. It replaces the windowed-form aggregates
of `ml.win_features` for that job (held-out AUC 0.73 T20 / 0.69 ODI / 0.75 T20I against
0.63 / 0.56 / 0.59; see `docs/WIN_PROB_SELECTION_PR_CHECKLIST.md`, S-9 results and S-10).

**How it works.** One chronological pass over match history (`ml/xi/ratings.py`): for each
match in date order, features are read from state built over earlier matches only, then the
match is folded in. There is no snapshot table; the as-of guarantee is structural. Per
(player, format) the state holds ball-level impact ratings (runs above the format×over
expectation per ball faced, dismissals below expectation, runs saved per ball bowled,
bowler-credited wickets above expectation; forgotten at 0.9 per match, shrunk with a 60-ball
prior), expected involvement (balls faced / bowled per match), experience, a keeper flag and
a player Elo. A side's eleven vectors aggregate to `contract.SIDE_FEATURE_STEMS`: batting and
bowling impact weighted by involvement, top-6 / top-5 sums, role coverage (bowling options,
keeper, all-rounders, debutants), Elo summaries. Team-level context (team Elo, form,
head-to-head, venue bat-first bias, venue familiarity) is kept in a separate column list
because it cannot distinguish two XIs.

**Two models per format.** `objective` — logistic regression on the XI columns, additive and
so monotone in practice (a one-player upgrade lowers p in <1% of cases vs 12% for
unconstrained boosting); this is what `/xi/optimize` maximises. `display` —
monotone-constrained gradient boosting on XI + team-context columns; the probability shown.

**Run:** `make train-xi CUTOFF=2025-09-01` reads the database (`POSTGRES_*`); with
`CRICSHEET_DIR=data/go-app/cricsheet` it reads the raw Cricsheet JSON instead (same format
taxonomy as `format.go`, ~2 minutes for the full archive). Writes `xi_win_<FMT>.joblib`,
`xi_ratings.joblib` and `xi_win_report.json` (AUC and Brier for both models over three seeds,
base-rate Brier, and the best single column's AUC — a model that cannot beat its own best
column is not being measured). `POST /admin/reload` picks the artifacts up; `GET /xi/status`
shows what is loaded.

**Serving:** `POST /xi/predict-win` and `POST /xi/optimize` take player ids, not feature maps.
The optimiser (`ml/xi/optimizer.py`) seeds greedily, then steepest-ascent single swaps, then
pair swaps, under constraints expressed through the same vectors the model reads (a bowling
option is a player whose expected balls bowled clears the format threshold). go-app uses
these endpoints when `selection.win_model` is `"xi"` and falls back to the windowed-form
model otherwise or on error.

**Identity.** Both sources key ratings by the Cricsheet registry identifier: `player.external_id`
from Postgres, `info.registry.people` from JSON, with the same `name:<name>` fallback for a
person the source has no entry for. The two paths therefore produce the same key for the same
person and their artifacts are comparable. Teams are keyed by `opposition_id`, which is one row
per (team name, gender) since migration `0004_identity.sql`. What that change bought is measured
in E4 (§5.1 of `ML_PIPELINE_REARCHITECTURE_PLAN.md`): nothing the holdout can resolve, in any
format or on either gender subset. A franchise that renames is still two clubs — I-4 in
`IDENTITY_PR_CHECKLIST.md`.

`xi_win_report.json` splits its holdout discrimination by gender, for information rather than as
a gate: 20% of the dataset is women's cricket, and the men's subset dominates any aggregate.

### Data-quality gate (H-15)

Every rating pass counts what it dropped and what it found odd, and `train-xi` fails on the
counts before the artifacts are worth anything. `ml/xi/quality.py` holds two rules:

- **Everything is accounted for.** A source offers N matches; N must equal the matches it
  yielded plus those out of scope plus those it could not use. A match dropped for a reason
  nothing names fails the run. This is the check that would have caught the database holding
  22,425 matches for 22,734 files.
- **Nothing doubles quietly.** Any quality count over twice the last accepted run's — or one
  that was zero and is not any more — fails. Data does not usually get twice as broken
  between two runs of the same pipeline.

The counts go into `xi_win_report.json` under `data_quality`, with any failures beside them.
The *accepted* counts live separately in `xi_data_quality_baseline.json`, and a failing run
does **not** update it, so re-running cannot clear the gate. When the new numbers are right,
say so explicitly:

```bash
make train-xi CUTOFF=2025-09-01 ACCEPT_DATA_QUALITY=1
```

Current baseline on the full dataset: 22,734 matches offered and 22,734 read, 1,710
undecided, 0 namesake sides, 1,358 sides of more than eleven (concussion and injury
replacements, which Cricsheet lists in full), 0 unresolved player keys, 13,569 players.

### Source parity (`make xi-parity`)

The two rating sources are supposed to describe the same cricket, and three times they did
not — a hashed match id that lost 309 matches, an unnamed substitute fielder folded into a
fictional player, and a namesake rule implemented on one side only. Each was a one-line
difference in a count that nobody was printing.

```bash
make xi-parity                                    # defaults to data/go-app/cricsheet
make xi-parity XI_PARITY_DIR=path/to/cricsheet
```

It runs the rating pass over both sources, prints their counts side by side and exits
non-zero if any count or the player-key sets differ. It needs the archive as well as the
database, which is why it is a separate command rather than part of a retrain. Run it after
changing the importer or either source.

### Player-match rows (L1, P-2)

The same day-close pass also emits one row per (match, player) — the training frame for the
performance model (L2-B). Each row carries the player's as-of vectors, the expected role
(`exp_bat_position`: decayed mean batting slot shrunk toward 7; `bat_innings_share`; batting
and bowling impact split by powerplay / middle / death, `contract.PHASE_BOUNDS`), the own-side
and opponent-side aggregates, and venue context — joined with what the player then did
(balls, runs, fours, sixes, dismissals, actual batting position, balls bowled, wickets,
runs conceded). Rows cover **all XI players**, never only those who batted: who got to bat
is decided by the result, and a population selected by the outcome is a leak (H-20).
`ml/xi/rows.py` assembles the rows for both the training pass and the parity check, so the
two cannot spell a column differently. `python -m ml.xi.train --player-frame-out <path>`
writes the frame as CSV when wanted; the harness consumes it in memory.

### As-of serving (`ratings_as_of`, P-2)

The serving artifact holds ratings **through today** — right for a live prediction, wrong
for a backtest, whose team Elo would carry the results of the matches being scored (P-0 had
to freeze the artifact by hand; `freeze_ratings.py` is retired). `ml/xi/asof.py` advances a
fresh state through a match source and answers "ratings as of date D": every match strictly
before D folded in, nothing at or after it — asking for a date the pass has already crossed
raises rather than guesses. `/xi/predict-win` and `/xi/optimize` accept an optional
`as_of` date; the go-app selection comparison sends the match date (its match list is
date-ascending, so the whole run costs one pass over the source), and its report now
carries per-match rows so the arms can be compared pairwise and filtered by date. Live
predictions omit `as_of` and are served from the loaded state unchanged.

### Evaluation harness (`make xi-evaluate`, L4 / H-19)

One command, one JSON report (`xi_evaluate_report.json`): rolling-origin walk-forward over
quarterly cutoffs 2024-01 … 2025-06 for every choice-facing number, and the **locked
window** (matches ≥ 2025-09-01) scored once per release, labeled, never used for a choice.
Per format it reports, with mean ± spread over cutoffs (and seeds where a model has one):
objective/display AUC and Brier against the base rate; the specific-XI-beyond-typical-XI
delta and swap monotonicity (the selection gates that replace P-0's winner accuracy); the
best-single-column leak canary with the TEST-format control (H-2); and the performance
baselines from the player-match rows — within-match Spearman, top-3 hit and per-target MAE
for the career-mean and rating-expectation predictors, with interval width and coverage
columns that stay empty until P-3 (H-22). It ends with the train/serve parity check (H-8):
the last 50 matches rebuilt from the as-of serving path and compared with the training
frame, and the run fails if they differ.

```bash
make xi-evaluate                                        # the database
make xi-evaluate CRICSHEET_DIR=data/go-app/cricsheet    # the raw archive
```

---

## Walk-forward

**Purpose:** Evaluate temporal performance: train on data before cutoff → predict next X matches (holdout) → score (e.g. MAE) → record in registry → advance cutoff and repeat. Builds a registry (e.g. `walk_forward_registry.json`) of model type, format, cutoff, window_x, params, metrics.

**Use:** Compare accuracy for different X; find underperforming windows and re-run auto_tune or retrain. Run as a **separate step** from auto_tune. Requires go-app and endpoints: `GET /api/backtest/matches?after=...`, `GET /api/backtest/training-data?cutoff=...`, `GET /api/backtest/holdout-data?cutoff=...&limit=...`.

**Run:** `make walk-forward INITIAL_CUTOFF=... WINDOW_X=50 WALK_FORMAT=T20 WALK_MODEL=batting` (or from ml-service with `GO_APP_URL`).

---

## Docker images: serve vs train

`ml-service/Dockerfile` has two targets.

| target | requirements | size | what it is for |
|---|---|---|---|
| `serve` (compose default) | `requirements-serve.txt` | **1.23 GB** | Serving predictions, `/admin/train/*`, and Optuna-only auto-tune |
| `train` | `requirements.txt` | **5.02 GB** | Adds AutoGluon model ranking and SHAP explanations |

```bash
docker build -f ml-service/Dockerfile --target serve -t cric-app-ml:serve .
docker build -f ml-service/Dockerfile --target train -t cric-app-ml:train .
```

**The serve image is not limited to serving.** `/admin/train/*` shells out to `python -m ml.train_*`, which needs only scikit-learn; `/admin/train/auto-tune` runs `ml.auto_tune`, which needs Optuna. Both are in the serving set. AutoGluon and SHAP each sit behind a guarded import with a graceful fallback, so auto-tune degrades to Optuna-only instead of failing. Build `train` when you want AutoGluon's model ranking.

`requirements-serve.txt` is also what CI installs, so the test suite runs against the same dependency set the serving image ships.

> **PyCaret does not work on this project's Python and is not worth its weight.** PyCaret 3.3.0 raises at import on Python >= 3.12 — *"Pycaret only supports python 3.9, 3.10, 3.11"* — and both the Docker image (`python:3.12-slim`) and the local venv are 3.12. It is installed by `requirements.txt`, pulls a large dependency tree, and is rejected every time; `_HAS_PYCARET` is `False` in both. `ml/auto_tune_pycaret.py` is guarded, so nothing breaks — the cost is dead weight in the `train` image. Removing it from `requirements.in` is blocked by `make compile-requirements-docker` failing on a `setup.py egg_info` step (pre-existing, reproducible on unmodified input). See C6-3 in [CLEANUP_PR_CHECKLIST.md](CLEANUP_PR_CHECKLIST.md).

---

## Probability calibration (classifiers)

For classifiers (e.g. the win model), predicted probabilities can be **calibrated** (Platt scaling or isotonic regression) so they reflect true frequencies, and evaluated with a reliability diagram, Brier score, or ECE.

**Not currently implemented.** A `ml.calibrate` module existed but was never wired into training or serving — no caller, no pipeline step, no endpoint — and was removed in C1-4/C1-6 cleanup. The win model's output is used uncalibrated. If calibration is wanted, add it to the win training path in `ml/train_win.py` so it ships with the artifact, rather than as a standalone module.

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

**Reconciliation layers** (hybrid rescale vs constraint solver): see [ml-service/docs/reconciliation.md](../ml-service/docs/reconciliation.md).

**Derived features** (`ml.match_level_derived_features`) are computed in one place and reused by `train_innings`, `train_extras`, and the inference path in `app.reconciliation`:

- `form_differential = bat_form_sum - bowl_form_sum`
- `consistency_differential = bat_consistency_sum - bowl_consistency_sum`

There was a third, `weather_composite`, a configurable blend of `rain`, `humidity` and `cloud`. C2-2b removed those three inputs from every export because nothing has ever populated `weather_data`, which left the composite computing a constant zero from columns that were no longer there; the low-variance filter then discarded it on every fit, so no trained artifact ever named it. It is gone, along with the `ml.match_level_derived` config block that only it used.

**Artifact sidecars (`ml.artifact_sidecar`)**:

| File | Written by | Read by |
|------|------------|---------|
| `innings_meta_<FMT>.json` | `ml.train_innings.train_and_save` | `app.artifacts.reload` → `app.reconciliation.predict_innings` |
| `extras_meta_<FMT>.json` | `ml.train_extras.train_and_save` | `app.artifacts.reload` (available to prediction code as `EXTRAS_META`) |

The sidecar pins two things:

- `feature_names`: exact column order the scaler/model were fitted on, so per-format `drop_low_variance_columns` and format one-hot exclusion cannot cause a shape mismatch at inference.

A sidecar written before this change may still name `weather_composite`. Feature selection is by name with a `0.0` default, so such an artifact degrades to a zero column rather than raising — which is exactly what the constant-zero feature contributed anyway.

**Operational note:** old artifacts without sidecars still load; `build_innings_feature_vector` falls back to `LEGACY_INNINGS_FEATURE_COLS` + the current config. Retrain any per-format model whose training data included format-only columns that were dropped during low-variance filtering so its sidecar is written and inference stops relying on the fallback.

---

## Data-quality: scale-aware low-variance column drop

`ml.data_quality.drop_low_variance_columns` removes effectively constant columns before fitting. The threshold is **scale-aware**: a column is dropped when `std ≤ threshold · (|mean| + 1)`. The `+ 1` term gives a sensible bar for zero-mean features (like `form_differential`) while still flagging tiny noise on large-mean ones (like a raw venue or season id). The knob lives under `ml.data_quality.low_variance_threshold` (default `1e-6`); values are coefficients, not absolute variance thresholds.

This is what absorbed `weather_composite` for as long as it survived: its inputs were removed in C2-2b, so it computed a constant zero and the filter discarded it at every fit. That is also why removing it needed no retrain — no artifact had ever named it.

---

## Fielding per-inning migration

Migration `0095_fielding_data_inning_number.sql` adds `inning_number` to `fielding_data` (default 1) and replaces the unique constraint with `(match_id, inning_number, player_id)`.

Historical rows remain at `inning_number = 1` until operators run a full re-import/recompute flow from source event data. `RecomputeFieldingAggregates` (see `go-app/internal/db/repo_fielding_event.go`) can split aggregates per inning when `fielding_event` is available for the target matches.

---

## Artifact kinds, reload, and staleness

`app.artifacts.ARTIFACT_KINDS` is the single source of truth for which model families exist
and how they are named on disk (`<kind>_model_<FMT>.joblib`, plus `<kind>_scaler_<FMT>.joblib`
where the kind has one). The loader, `/health` and `/artifacts/status` all derive from it, and
go-app (`opsstatus.artifactKinds`) and the frontend (`utils/artifactKinds.ts`) mirror the list
for the models `make train-models` produces. **Adding a model kind means adding one entry per
layer** — not editing every reader. Innings artifacts were trained, written and loadable while
all three readers still carried a five-kind list that omitted them, so a completed run looked
like a missing model.

**A finished `/admin/train/*` run reloads the artifacts before it returns.** Training runs in a
subprocess and writes to `MODELS_DIR`; the serving process holds its registries in memory. Without
that reload the run is recorded `COMPLETED` in `data_migrations` and changes nothing about what
the service predicts with until a restart. A reload failure is logged
(`admin.train.artifacts_reload_failed`) but does not fail the run — the artifacts are on disk
either way. `POST /admin/reload` still does the same thing on demand.

**`/artifacts/status` reports `stale`.** `loaded` only says the registry holds an object for a
format; `stale` says the file on disk is newer than the one that object was loaded from. A
registry entry that cannot be attributed to a file this process loaded also reports `stale`,
because being current cannot be claimed for it. The verdict passes through go-app `/ops/status`
to the ops console, where a stale kind shows an amber loaded dot.

---

## Loader contract: `LoaderResult`

All tuning data loaders in `ml.tuning.data_loaders` return a typed envelope:

- Single-pack loaders (batting, bowling): `LoaderResult(X, Y, feature_names, sample_weight=None)`.
- Per-format loaders (extras, win, fielding, innings): `Dict[str, LoaderResult]`, keyed by uppercase format code (e.g. `T20`, `ODI`, `TEST`, `OTHER`). The `_LEGACY_` pooling key these once emitted is gone with the unified models it fed, so the name no longer collides with the removed artifact registry.

Call sites in `ml.tuning.cli` consume `result.X / result.Y / result.feature_names / result.sample_weight` directly; the previous `unpack_xy_with_feature_names` / `_extras_feature_names_if_consistent` helpers have been removed. To add a new loader, return a `LoaderResult` (or `Dict[str, LoaderResult]`) from the outset — it keeps optional fields explicit and prevents shape drift between training and tuning.

---

## Pending validation work

The following is **not** yet verified in this branch and is deliberately left as an operator follow-up because it requires a populated training DB and non-trivial auto-tune time:

- **Feature transforms A/B (`config.json` → `feature_transforms`)**: `add_log1p` for `*_career_count`, `*_days_since_last`, `*_innings_in_last_90d` and three hand-picked interactions per side are enabled for batting/bowling. Before accepting them as defaults, run `make auto-tune` per format with and without transforms (toggle `feature_transforms.batting.add_interactions` / `add_log1p` and the matching bowling block) on the same cutoff and compare:
  - `best_cv_score` (lower MAE is better).
  - `mlqa_audit.checks.overfitting.relative_delta` and `mlqa_audit.checks.stability.relative_cv_std`.
  - Per-fold CV score spread.

  If transforms help, migrate the block from `config.json` (environment-local override) to `config.default.json` so it ships with defaults. If they don't, remove them to avoid the redundant feature-name plumbing cost at inference.

- **Derived-feature ablation**: the `form_differential` and `consistency_differential` columns are pure subtractions of features the model also sees, and tree-based models can (and usually do) recover them from the raw columns on their own. Run the same auto-tune sweep with and without each and keep only the ones that improve hold-out MAE or MLQA stability; drop the rest from `MATCH_LEVEL_DERIVED_FEATURE_COLS`. (`weather_composite` was the third such column and is already gone — it was constant zero.)
