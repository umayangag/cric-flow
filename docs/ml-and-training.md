# ML models and training

The XI layer — the rating pass, the win models, the player performance model, the match
simulator and the L4 harness — plus what is left of the windowed-form win model, which P-6
removes. **Model inputs/outputs and hyperparameters:** [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## What predicts what

| Layer | Level | Role |
|-------|-------|------|
| L1 rating pass (`ml/xi/builder.py`) | — | One chronological, as-of pass over the event store; emits the win frame, the player-match frame and the serving state |
| L2-A win models (`ml/xi/train.py`) | Match | Objective (additive, the value the optimiser maximises) and display (monotone GBM, the probability shown) |
| L2-B performance model (`ml/xi/performance.py`) | Player | Quantile runs / balls / runs conceded, a two-part Poisson of wickets, a catch rate, and P(bats) / P(bowls) |
| L2-C simulator (`ml/xi/simulator.py`) | Match | Draws whole matches from L2-B for two elevens; no training of its own |
| L3 selection (`ml/xi/optimizer.py`) | Match | The XI that maximises the objective, its marginal values, and — where the objective does not rank — the rating-ordered pick |
| Win, windowed form (`ml/train_win.py`) | Match | The original match-level classifier. Superseded; P-6 removes it |

Every serving call into the XI layer takes **player ids and a format**, never a feature map.
The rating state lives in ml-service, which is what makes the training and serving paths
compute the same function of the same eleven names (H-8).

**Training data (go-app):** `GET /api/backtest/training-data?cutoff=...&format=all` still serves
the win section to `ml.train_win` and the auto-tune stack. The XI layer does not use it: it
reads `match`, `match_player` and `ball_event` directly.

**Training commands:** `make train-xi CUTOFF=<YYYY-MM-DD>` builds the rating state and every XI
model; `make xi-evaluate` scores them; `make train-win CUTOFF=<RFC3339>` builds the windowed-form
win model.

> **`CUTOFF` bounds the training data.** Rows with `match_date` on or after it are dropped,
> leaving everything from the cutoff onward as a holdout. To produce a model you can honestly
> evaluate, train with a cutoff that leaves a window behind it.

**Artifacts:** `xi_ratings.joblib`, `xi_win_<FMT>.joblib` and `xi_perf_<FMT>.joblib` for the XI
layer (loaded by `ml.xi.store`); `win_model_<FMT>.joblib` for the windowed-form model.

---

## Data normalization and best practices

- **No future leakage:** Training uses only matches with `match_date < cutoff`. Same cutoff logic for export and for feature computation at prediction.
- **Same feature computation:** go-app uses identical logic for export rows and for feature map at prediction (`ComputeFeaturesAtCutoffForMatch`). Form = EWM, consistency = coefficient of variation, venue/opposition = EWM at scope. The v2 contract includes raw windowed stats (e.g. batting_mean_w3, bowling_std_w10) from `feature_raw_stats_snapshots` (overall scope); export and prediction both populate these from precomputed snapshots when available.
- **Feature order:** Training and prediction use the same order from `configs/feature_vectors.json` (batting, bowling, fielding). ML builds the vector from this config at prediction.
- **Input normalization (X):** `StandardScaler` fitted only on training data; same scaler saved and used at prediction. No test/future data in fit.
- **Targets (Y):** Kept in raw units (no scaling) for interpretability and to avoid inverse transform.
- **Missing values:** Training drops or fills (e.g. 0) per script; prediction uses `ml.feature_defaults` in config for missing keys.

## Pipeline

You can run the pipeline from the **frontend** (Ops Status → Pipeline) or from the command line.

**Steps:** (1) Import — migrate and import Cricsheet. (2) Precompute — form / consistency /
sequence per format. (3) Export — writes the cross-format and per-format CSVs. (4) Train Win.
(5) Optional: Auto-tune. Steps 2, 3 and 5 exist for the windowed-form model and the export
consumers, and P-6 removes them; the XI layer needs only the import.

**Auto-tune depends only on Export**, not on any trained artifact, so it can run before Train
Win — and in Mode B below it must.

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
    - `ml/train_win.py` (the windowed-form training CLI),
    - `ml/tuning/*.py` (auto‑tune orchestration),
    - `ml/validate_exports.py`.
  - This keeps the coverage number focused on `app.main`, `app/xi_service.py`, the `ml/xi`
    package, config, and other request‑time paths.

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

**Single-train principle:** train the win model **once** with the params you intend to use.
They come from `ml-service/config.json` (`ml.training.win`) and, when `GO_APP_URL` is set, are
overlaid by tuned params in the go-app DB from a previous auto-tune.

- **Mode A — params known.** Import → Precompute → Export → Train Win. Do not run auto-tune.
- **Mode B — params unknown or being refreshed.** Import → Precompute → Export → Auto-tune →
  Train Win, which is the `tune` run plan. **The search comes first** because hyperparameters
  are a function of the feature space: when the feature space has changed, training before the
  search produces artifacts the search invalidates an hour later.

The XI layer has no equivalent choice. Its hyperparameters are a small grid tuned inside the
L4 folds and recorded with the run, so `make train-xi` is the whole loop.

**Precompute parameters are a separate, slower loop:** change `features.*` → re-precompute →
re-export → re-train.

---

## Feature contract v2 (raw windowed stats)

**Contract version:** `configs/feature_vectors.json` and go-app use version **"2"**. Batting and bowling include **18 raw windowed stats** per type (e.g. `batting_mean_w3`, `batting_std_w10`, `batting_last_1`, …) alongside the existing formula features (form, form_short, form_long, momentum, consistency). These are computed in Go (`features.WindowedStats`) and stored in `feature_raw_stats_snapshots`; export and prediction emit them so the ML model can learn optimal combinations instead of fixed EWM/CV formulas.

**The comparison this section described is moot.** It weighed formula features against raw
windowed stats for the per-player models, which P-5 deleted. The contract survives because
go-app's export queries still emit it, and P-6 removes those with the export step. The XI layer
computes its own as-of features from the event store and reads none of this.

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
person and their artifacts are comparable. Teams are keyed by the **club**: one opposition row
per (team name, gender) since migration `0004_identity.sql`, folded onto the club's current row
by `opposition.canonical_id` since `0006_team_lineage.sql`, so a franchise that renames does not
restart its Elo and head-to-head. The renames are reviewed data in `configs/team_lineage.json`
(I-4), read by the go-app importer and by the Cricsheet-JSON source, so both agree. What the
identity work bought is measured in E4 (§5.1 of `ML_PIPELINE_REARCHITECTURE_PLAN.md`): nothing
the holdout can resolve, in any format or on either gender subset. It is correctness, not
discrimination.

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

The harness's parity check (H-8) is the other half of the same idea and found a fifth
difference in P-3: the Postgres source read deliveries in `ball_seq` order, which counts
legal balls only, so a wide shared its number with the ball before it and their order was
the planner's. Nothing noticed until a feature read delivery order. Deliveries are now read
in `(innings, over, ball)` order, the source's own. Comparing the two sources' performance
reports then found a sixth: the database source read fielders from `ball_event.fielder_ids`,
which the importer leaves NULL, so it credited no catches and knew no keeper (the flag comes
from stumpings) — the optimiser's `require_keeper` could not be met from the database. It
reads `fielding_event` now, and the importer no longer credits the bowler with a catch taken
by an unnamed substitute (452 rows), replacing a match's fielding events on re-import. The
plan's §10.4 has the account.

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
writes the frame as CSV when wanted; the harness consumes it in memory. Since P-3 the rows
also carry `catches` (each fielder named on a caught dismissal) and the sequence families
(`contract.SEQUENCE_FAMILIES`: dot streaks, reactions, spells — the `seqcalc` calculators
as as-of accumulators, per-ball flags from `ml/xi/sequence.py`), which are in the frame
whether or not the performance model consumes them (E1 decides that).

### Performance model (L2-B, `ml/xi/performance.py`, P-3)

**What it answers.** For two elevens, per player, *distributions* — never points — of runs,
balls faced and runs conceded (quantiles 0.1 / 0.5 / 0.9), wickets and catches (a Poisson
rate → P(0), P(1), P(2+)), and P(bats) / P(bowls). The point shown anywhere is the median;
the deliverable is a calibrated range and a ranking, because one innings is mostly noise
(plan §1: within-match Spearman ≈ 0.3 for any predictor on the players who batted).

**Population (H-20).** Every XI player of every decided match, with "did not bat" as 0 runs
from 0 balls and "did not bowl" as 0 wickets. The old batting model trained on "who batted",
which the result decides, and its headline MAE was pooled over five targets (S-3c); neither
survives here. Two structures per target are available and were chosen on the walk-forward
folds by pinball loss (plan P-3): `direct` — one gradient-boosting model per quantile (or a
Poisson model) on the unconditional rows; `two_part` — P(involved) from a classifier on the
same rows, times the distribution given involvement fitted on the rows where it happened,
with the involvement *predicted* and the served quantiles those of the mixture, so both
structures are scored on one population with one loss.

**Inputs.** The row's as-of vectors and expected role, the sequence families E1 kept, both
sides' aggregates, venue context, the Elo edge, and the innings (bat first / chase). The
innings is the toss, not the result: at prediction it is **marginalised** — predicted under
both and averaged — unless the caller passes `team1_bats_first`, the same knob
`/xi/predict-win` has. Nothing the model reads is a function of the match's own result
(`contract.performance_feature_cols` excludes every target column; a unit test asserts it).

**Fitting.** `HistGradientBoostingRegressor` with quantile and Poisson losses and a
classifier for involvement; a three-point grid (`performance.HYPERPARAMETER_GRID`) tuned
inside the walk-forward folds only, where it turned out flat (§ P-3 of the plan); every fit
on three seeds, which enter through the early-stopping split, with the members' outputs
averaged. Independently fitted quantiles can cross; they are sorted. When the harness finds a
quantile target's coverage off nominal it is **recalibrated on a temporal fold** (H-5,
`ml/xi/perf_calibration.py`): the last quarter of the training rows is held out of the fit,
each level is mapped by an isotonic binned correction fitted there, and the members never
see those rows (H-21). `performance.RECALIBRATED_TARGETS` records the decision.

**Measured by (H-12, H-22).** Per target and format, never pooled: within-match Spearman and
top-3 hit for the ranking (using the mean for counts — a median of 0 cannot rank bowlers —
and the median otherwise), the median's MAE, pinball loss as the proper score, and the
10–90 interval's coverage **beside its width**. Coverage is read twice because the targets
have a point mass at zero: a calibrated 0.1 quantile of a player who bats in half his
matches is 0, so the inclusive coverage of a calibrated interval legitimately exceeds
nominal while the strict one falls short; nominal sits between them, and H-5's check is per
end (`perf_calibration.coverage_off_nominal`). The career-mean, career-quantile and
rating-expectation baselines are scored on the same unconditional population
(`ml/xi/perf_baselines.py`), and a labelled diagnostic — the ranking among the players who
did bat / bowl — sits beside the headline because tie-averaging on the unconditional
population rewards a predictor that gives every non-bowler one identical value.

**First numbers** (plan §8.2; walk-forward, 7 folds × 3 seeds, model vs career mean on the
same unconditional rows): runs within-match Spearman 0.542 vs 0.502 (T20) and 0.475 vs 0.427
(ODI), pinball 2.93 vs 5.09 and 4.59 vs 7.96, median MAE 9 % lower; wickets pinball 0.141 vs
0.260 and 0.163 vs 0.302, but Spearman a tie in ODI and −0.03 in T20 — six of eleven take
no wickets and tie, and a predictor that gives them one identical value is rewarded for it
(among the bowlers the model ranks better). Locked-window coverage is nominal per quantile
end in every format without recalibration. Expect the point to stay modest: the ranking
among batters sits at ≈ 0.33, the ceiling the plan measured for every predictor; the
deliverable is the range.

**Run.** `make train-xi CUTOFF=…` fits the performance models beside the win models and
writes `xi_perf_<FMT>.joblib`; `xi_win_report.json` carries the holdout numbers per target
under `performance`. `make xi-evaluate` is where the choice-facing numbers come from. The
choices themselves (grid, structure, E1, E6) are reproduced by
`scripts/experiments/xi/perf_choices.py`, which runs on the walk-forward folds only.

**Serve.** `POST /performance/predict` takes both elevens by id, the format, optional team
and venue ids, `team1_bats_first` once the toss is known, and `as_of` for backtests, and
returns per player the median and 10–90 range of runs, balls faced and runs conceded,
P(bats) / P(bowls), wicket probabilities P(0) / P(1) / P(2+), and the expected catches.
Rows are assembled by `ml/xi/rows.py` from the same state the win path reads, and the H-8
parity check in the harness compares the served prediction with the prediction on the
training frame's row for the last 50 matches, output by output.

### Match simulator (L2-C, `ml/xi/simulator.py`, P-4)

**What it answers.** For two elevens, drawn N times (default 2,000, seeded, vectorised): each
side's total (median, 10–90), each player's median and 10–90 of runs, balls, wickets and runs
conceded, a **median-band scorecard** — each player's mean over the draws whose side total
lies in the central tenth of its distribution, which sums to that band's mean total by
construction, so the scorecard and the innings total shown are one picture — the margin as
cricket states it (runs when the side batting first wins, balls remaining and wickets in hand
when the chaser does), P(win) by simulation, and each player's share of the total's spread,
Cov(player, total) / Var(total). Nothing is trained: the design is written down in the plan
(§3, "The innings sample") and the module follows it.

**Inputs.** L2-B's forecasts for the fixture under both orientations — the three quantiles of
runs, balls faced and runs conceded, the wicket distribution, P(bats) / P(bowls) — plus the
row's as-of expected slot and expected balls bowled, and three **as-of context rates** the
rating pass now carries per format (`contract.SIMULATION_CONTEXT_COLS`,
`RatingState.simulation_context`): extras per delivery, deliveries per full first innings (one
not all out, so it ran its overs) and the bowler-credited share of dismissals. The only
constants are laws of the game (legal balls, ten wickets, a bowler's fifth). Nothing the
simulator consumes is in-sample for the fixture (H-21): the forecasts are as-of predictions,
the rates are running sums over matches before it.

**The innings sample.** A player's three quantiles become a quantile function (piecewise
linear through zero and the fitted levels, exponential tail above 0.9 with the (q50, q90)
scale). The batting side is authoritative: order by `exp_bat_position`; one uniform per draw
against each P(bats) sets how deep the innings goes (at least two bat); each batter draws
runs and balls *given that he bats* from the upper P(bats) part of his distribution; the
balls budget is the as-of deliveries per full innings — the batter at which it is crossed
keeps the remainder at his sampled strike rate, and when the sum falls short the not-out
pair face the rest at their expected rates; wickets = batters − 2 (10 when all out); extras
are Poisson at the as-of rate over the deliveries used. The chase ends at the target
(contributions counted with extras pro rata). Bowlers are *attributions* of that innings:
each bowls with P(bowls), topped up until the side can deliver the innings under the cap;
balls in proportion to expected balls; runs conceded a multinomial split of the total by
balls × as-of rate; the bowler-credited share of the wickets by balls × wicket rate. So the
bowlers' figures sum to the innings by construction and their own L2-B medians are not
reproduced — that would be a second estimate of the innings, and the whole point of taking the
batting side as authoritative is that there is only one. Toss unknown: half the draws each way, each with the matching forecasts (H-3).

**Runs and balls are coupled, not identical.** The first build drew a batter's runs and balls
from one uniform; with every strike rate fixed and the balls budget enforced, the side total's
spread collapsed (sd 4.6 on a synthetic side). Runs and balls are now drawn through a
Gaussian copula whose correlation comes from the training rows' rank correlation of runs and
balls among those who batted (`simulator.runs_balls_copula_rho`, stored in the artifact as
`PerformanceModels.simulation`), as-of for the fixture because the rows precede the cutoff.

**Shared match factor.** A pitch or a day is common to both innings, so independent batter
draws can under-disperse totals. E2 measures it (PIT and the dispersion ratio of actual
totals around the simulated mean); where needed, one multiplicative factor per draw, shared by
both innings, is sampled from the **as-of residual distribution** — actual / simulated-mean
first-innings totals on the last 92 days before the cutoff, the temporal calibration fold the
members do not train on (H-21), deconvolved of the simulator's own dispersion — never a
hand-set CV. `simulator.SHARED_FACTOR` records the decision; §8.3 of the plan the before/after.

**Measured by (E2, `ml/xi/sim_harness.py`, in `make xi-evaluate`).** Per format and window,
beside the display model on the same matches: Brier and reliability of the simulated P(win)
(pre-toss, the comparable one; toss-known beside it) against the display model's and the base
rate; coverage **and** width of the simulated totals' 10–90 interval against actual first
innings that ran their course (H-22 applied to totals), with the chase total the same way on
every match; margins; latency per fixture. E2's rule (plan §5): simulated P(win) worse than the
display model by more than 0.01 Brier on the walk-forward folds and it is a description, never
the displayed probability — `simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED` per format, and
`/simulate` returns both with `headline_source`. The H-8 parity check compares the simulator's
draws at a fixed seed from the as-of path and from the training frame's rows.

**First numbers** (plan §8.3). Walk-forward, 7 folds: without the shared factor the
first-innings totals' 10–90 coverage is 0.64 (T20) / 0.58 (ODI) with a dispersion ratio of
1.42 / 1.36 and a U-shaped PIT; with it 0.76 / 0.74 at ratio 1.02 / 1.02, the interval
widening from 61 to 83 runs (T20) and 107 to 151 (ODI) — the narrower one was the wrong one.
Locked window (≥ 2025-09-01, scored once): coverage **0.786** (T20, 1,521 first innings) and
**0.790** (ODI, 347), the acceptance's ±0.03 met; simulated P(win) Brier 0.2024 vs the
display model's 0.2032 (T20) and 0.2201 vs 0.2110 (ODI), within E2's tolerance, so the
display model stays the headline and the simulated probability is served beside it. The
chase total under-covers from the low side (0.72–0.73); margins cover 0.50–0.69 at nominal
0.80 — reported, not tuned. 5.4 ms per fixture at 1,000 draws, 9.4–9.9 ms at the served
2,000. The two sources agree on every locked-window figure to within the seed spread and
the H-8 parity check is 0.0 on both, simulator draws included.

**Serve.** `POST /simulate` takes what `/performance/predict` takes plus `n_samples` and
`seed`, and returns per side the total (median, 10–90, mean, sd, scorecard total), per player
the ranges and the scorecard line, the win probabilities (simulated, display, headline and its
source), and the margin. Limited-overs formats only (422 otherwise; TEST stays on the greedy
path, H-17). Behind `selection.win_model: "xi"` the go-app scorecard reads it
(`predictteam/xi_simulation.go`): innings totals, per-player points and their `runs_range` /
`wickets_range` come from the draws, P(win) from the display model, and the response carries
`xi_simulation` (innings ranges, simulated P(win), which model is the headline) and
`explanation` — each selected player's marginal value from `/xi/optimize` and share of the
total's spread from the simulator (L3). The extras and innings models and the win-probability
rescale are unused on that path; P-5 re-points the rest and P-6 deletes them.

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
model on the player-match rows — per target and format, within-match Spearman, top-3 hit,
the median's MAE, pinball loss and the 10–90 interval's coverage beside its width (H-22),
with the career-mean, career-quantile and rating-expectation baselines on the same
population, and quantile targets whose walk-forward coverage is off nominal recalibrated
on a temporal fold for the locked window (H-5); and the simulator (E2) — simulated P(win)
against the display model's, totals coverage and width, margins, latency, with E2's display
rule decided on the folds. It ends with the train/serve parity check (H-8): the last 50
matches rebuilt from the as-of serving path and compared with the training frame — rows,
performance predictions and simulator draws at a fixed seed alike — and the run fails if
they differ.

```bash
make xi-evaluate                                        # the database
make xi-evaluate CRICSHEET_DIR=data/go-app/cricsheet    # the raw archive
```

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

**The serve image is not limited to serving.** `/admin/train/win` shells out to `python -m ml.train_win`, which needs only scikit-learn; `/admin/train/auto-tune` runs `ml.auto_tune`, which needs Optuna. Both are in the serving set. AutoGluon and SHAP each sit behind a guarded import with a graceful fallback, so auto-tune degrades to Optuna-only instead of failing. Build `train` when you want AutoGluon's model ranking. The XI layer needs neither.

`requirements-serve.txt` is also what CI installs, so the test suite runs against the same dependency set the serving image ships.

> **PyCaret does not work on this project's Python and is not worth its weight.** PyCaret 3.3.0 raises at import on Python >= 3.12 — *"Pycaret only supports python 3.9, 3.10, 3.11"* — and both the Docker image (`python:3.12-slim`) and the local venv are 3.12. It is installed by `requirements.txt`, pulls a large dependency tree, and is rejected every time; `_HAS_PYCARET` is `False` in both. `ml/auto_tune_pycaret.py` is guarded, so nothing breaks — the cost is dead weight in the `train` image. Removing it from `requirements.in` is blocked by `make compile-requirements-docker` failing on a `setup.py egg_info` step (pre-existing, reproducible on unmodified input). See C6-3 in [CLEANUP_PR_CHECKLIST.md](CLEANUP_PR_CHECKLIST.md).

---

## Probability calibration (classifiers)

For classifiers (e.g. the win model), predicted probabilities can be **calibrated** (Platt scaling or isotonic regression) so they reflect true frequencies, and evaluated with a reliability diagram, Brier score, or ECE.

**Not currently implemented.** A `ml.calibrate` module existed but was never wired into training or serving — no caller, no pipeline step, no endpoint — and was removed in C1-4/C1-6 cleanup. The win model's output is used uncalibrated. If calibration is wanted, add it to the win training path in `ml/train_win.py` so it ships with the artifact, rather than as a standalone module.

---

## Model sidecars

`ml.artifact_sidecar` writes `win_model_<FMT>_metadata.json` beside each windowed-form win
artifact. It pins `feature_names` — the exact column order the model was fitted on — so that
per-format low-variance dropping cannot cause a shape mismatch at inference.

The XI models carry their own metadata inside their artifacts (`ml.xi.store`) and do not use
sidecars.

The match-level derived features (`form_differential`, `consistency_differential`) and the
`weather_composite` that preceded them went with the models that read them.

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

`app.artifacts.ARTIFACT_KINDS` is the single source of truth for which legacy model families
exist and how they are named on disk (`<kind>_model_<FMT>.joblib`, plus
`<kind>_scaler_<FMT>.joblib` where the kind has one). The loader, `/health` and
`/artifacts/status` all derive from it, and go-app (`opsstatus.artifactKinds`) and the frontend
(`utils/artifactKinds.ts`) mirror the list. **Adding a model kind means adding one entry per
layer** — not editing every reader; a kind the service can load but health never mentions makes
a completed run look like a missing model.

One kind is left, `win`, and P-6 removes it. The XI artifacts are loaded by `ml.xi.store`, which
keeps its own state and is reported through `GET /xi/status` rather than this registry.

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

`ml.tuning.data_loaders` has one loader left, for the win model, and it returns
`Dict[str, LoaderResult]` keyed by uppercase format code (`T20`, `ODI`, `TEST`, …). The envelope
makes the optional fields (`feature_names`, `sample_weight`) explicit and prevents shape drift
between training and tuning; `ml.tuning.cli` consumes them directly.

---

## Pending validation work

Not verified in this branch, and left as an operator follow-up because it needs a populated
training DB and non-trivial auto-tune time:

- **Feature transforms A/B for the win model.** Run `make auto-tune` per format with and without
  the transform block on the same cutoff and compare `best_cv_score`,
  `mlqa_audit.checks.overfitting.relative_delta`, `mlqa_audit.checks.stability.relative_cv_std`
  and the per-fold CV spread. If the transforms help, move the block from `config.json` into
  `config.default.json`; if not, remove it. P-6 may settle this by deleting the model.
