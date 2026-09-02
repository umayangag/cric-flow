# ML models and training

The XI layer: the rating pass, the win models, the player performance model, the match
simulator and the L4 harness. It is the whole ML surface — the windowed-form win model, the
per-player models and the auto-tune stack are gone (P-5, P-6).
**Model inputs/outputs and hyperparameters:** [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

---

## What predicts what

| Layer | Level | Role |
|-------|-------|------|
| L1 rating pass (`ml/xi/builder.py`) | — | One chronological, as-of pass over the event store; emits the win frame, the player-match frame and the serving state |
| L2-A win models (`ml/xi/train.py`) | Match | Objective (additive, the value the optimiser maximises) and display (monotone GBM, the probability shown) |
| L2-B performance model (`ml/xi/performance.py`) | Player | Quantile runs / balls / runs conceded, a two-part Poisson of wickets, a catch rate, and P(bats) / P(bowls) |
| L2-C simulator (`ml/xi/simulator.py`) | Match | Draws whole matches from L2-B for two elevens; no training of its own |
| L3 selection (`ml/xi/optimizer.py`) | Match | The XI that maximises the objective, its marginal values, and — where the objective does not rank — the rating-ordered pick |
| L4 harness (`ml/xi/evaluate.py`) | — | Walk-forward folds and the locked window, the selection and performance metrics, the leak canary and the train/serve parity check |

Every serving call into the XI layer takes **player ids and a format**, never a feature map.
The rating state lives in ml-service, which is what makes the training and serving paths
compute the same function of the same eleven names (H-8).

**Training data:** there is none to export. The rating pass reads `match`, `match_player` and
`ball_event` directly, one ordered scan, and writes its frames into the run directory. The
precompute step, the export CSVs and go-app's `training-data` endpoint went with the models
that read them (P-6).

**Commands:** `make retrain CUTOFF=<YYYY-MM-DD>` builds one run; `make reload` serves it;
`make evaluate` runs L4 over it. See *The pipeline* below.

> **`CUTOFF` bounds the training data.** Rows with `match_date` on or after it are dropped,
> leaving everything from the cutoff onward as a holdout. To produce a model you can honestly
> evaluate, train with a cutoff that leaves a window behind it.

**Artifacts:** `runs/<run_id>/` holds `xi_ratings.joblib`, `xi_win_<FMT>.joblib`,
`xi_perf_<FMT>.joblib`, the run's report and `manifest.json`. `current_run.json` at the
artifacts root names the run being served. See *Runs, manifests and staleness* below.

---

## Data normalization and best practices

- **No future leakage:** every row a model trains on has `match_date < cutoff`, and every
  feature in it is an as-of accumulator that has seen only earlier matches (H-1). The rating
  pass folds a day's matches in at day close, so a match never sees a same-day result (H-18).
- **One feature computation:** training rows and serving rows come from the same code
  (`ml.xi.rows`) over the same rating state, which is what makes the two paths compute the
  same function of the same eleven names. The harness re-derives the last 50 matches through
  the as-of serving path and fails the run on any difference (H-8).
- **Input normalization (X):** the objective model is a `StandardScaler` + logistic regression
  pipeline, fitted on training rows only and saved with the model. The display model is a
  monotone-constrained gradient booster and needs none.
- **Targets (Y):** raw units, no scaling — the performance model's quantiles are runs and
  balls, and an inverse transform is one more place for a mistake to hide.
- **Missing values:** a player the state has never seen reads as a debutant by construction
  (H-10), not as a zero row.

---

## The pipeline

Three steps, and a harness beside them. Run them from the **frontend** (Ops Status → Pipeline)
or from the command line; the console enforces step order, streams progress and can be
cancelled, and the make targets do the same work without any of that.

| Step | Command | What it does |
|------|---------|--------------|
| **import** | `make cricsheet-import` | Fetch the configured archive, extract it, load the matches. Fetch and extract are skipped, and say so, when the dataset directory already holds that archive. |
| **retrain** | `make retrain CUTOFF=2025-09-01` | The whole model build: rating pass → XI win models (with the grid) → performance models → the run's report → `manifest.json`. Writes `runs/<run_id>/` and **publishes nothing**. |
| **reload** | `make reload [RUN=<id>]` | Point `current` at a run and load it into the running service. With no run id: the run `current` already names, or the newest one. |
| *evaluate* | `make evaluate` | L4 over the database: walk-forward folds, the locked window, the selection and performance metrics, the leak canary, the parity check. Touches no artifact `current` points at. Optional, and slow — see below. |

`make up-all CUTOFF=<date>` is the whole chain from an empty database; `make full-pipeline` is
retrain → reload against data already imported.

**Why reload is separate from retrain.** They answer different questions — "build a run" and
"serve that run" — and a retrain that published itself would leave no way back to the run
before it. Naming a run is how you swap back.

**Why evaluate is separate from both.** The harness refits every model per fold per format:
measured on the full database it takes ~54 minutes, against a whole pipeline that runs in a
fraction of that. Folding it into every retrain would make the pipeline unrunnable at any
sensible cadence. What a retrain records is its *own* holdout report — the numbers the models
it just fitted produced — and the manifest names it, so nothing quotes a measurement of a
different run.

> **`CUTOFF` bounds the training data.** Rows with `match_date` on or after it are dropped,
> leaving everything from the cutoff onward as a holdout. To produce a model you can honestly
> evaluate, train with a cutoff that leaves a window behind it.

---

## Hyperparameters

**Glossary keys** (L-1, `ml/xi/glossary.py`): none — the chosen values are inputs, and `hyperparameters` is declared a non-metric so the completeness gate does not ask anyone to explain a learning rate.

There is one search, it is three points wide, and it runs inside `retrain`.

`ml.xi.train.DISPLAY_GRID` holds three settings for the display model (depth, learning rate,
iterations). Each is fitted on the first 80 % of the training rows *by date* and scored on the
last 20 % — inside the training window, strictly before the holdout, so choosing a
hyperparameter cannot see the rows the run is scored on (H-19). The incumbent (the setting the
display model has always been fitted with) keeps its place unless a candidate beats it by more
than `DISPLAY_GRID_MARGIN` = 0.002 AUC: differences under the noise floor are not evidence
(H-14), and a grid that reshuffles the model on 0.001 every release is a source of drift.

The choice, the reason and every candidate's score go into `manifest.json` under
`hyperparameters`. That is the whole record — there is no tuned-params table, because a table
nothing could join back to an artifact was how a model came to carry parameters from a search
it had never seen.

This replaced a two-phase Optuna search with PyCaret and AutoGluon ranking. It was removed
because the model class was measured not to be the constraint, twice; the constraint is the
game (§1 of the re-architecture plan), and no amount of search moves it.

---

## Runs, manifests and staleness

**Glossary keys** (L-1, `ml/xi/glossary.py`): the manifest's headline metrics are `objective_auc` and `display_auc_mean`; `n_train` and `n_holdout` beside them are declared counts, not metrics.

**A run is a directory, and `current` is a pointer to one** (H-16):

```
output/ml-service/
  current_run.json                    {"run_id": "...", "updated_at": "..."}
  runs/20260902T101500Z-ab12cd34/
    manifest.json
    xi_ratings.joblib
    xi_win_<FMT>.joblib
    xi_perf_<FMT>.joblib
    xi_win_report.json
```

`manifest.json` carries the run id, when it was created, the cutoff, the dataset sha (a digest
of the matches the pass consumed — computed from what was read, because ml-service does not
mount the dataset directory), the git sha, the rating params, the hyperparameters the grid
chose *and why*, the run's headline metrics per format, and the rating state's shape. It is
written **last**, so a directory only becomes a run once everything it names is on disk: a
retrain that dies half-way leaves wreckage the loader never selects and `/artifacts/status`
lists as "no manifest".

**The loader refuses what it cannot serve.** `XiStore.load` reads the manifest first and
raises `RunArtifactsInvalid`, naming the run, when there is no manifest, when an array this
code reads is absent, or when a player array is narrower than the number of players the
payload registers. That is D-6: a rating artifact written before P-2 loaded without complaint
and then raised `IndexError` on the first request past slot 1024, while `/xi/status` reported
`loaded: true`. An artifact was trusted because it loaded; now it has to say which run it is
from and what shape it is in. The refusal reaches `/xi/status`, `/health`, `/ops/status`, a
409 `RUN_ARTIFACTS_INVALID` from `POST /admin/reload`, and the prediction tab.

**Staleness (H-11).** A live prediction against ratings older than
`ml.ratings_max_age_days` (default 14; `XI_RATINGS_MAX_AGE_DAYS` overrides) is refused with
`RATINGS_STALE` and a hint naming the step that fixes it. A request that names its own `as_of`
is not refused: a backtest asks for a date and gets it, and refusing one would break the
harness for a reason that does not describe it. Setting the limit to zero turns the check off
— a decision visible in config rather than a state the code can drift into. The verdict, not
just the date, is on `/xi/status` (`ratings.fresh`, `age_days`, `max_age_days`, `code`).

---

## Test coverage and CI gates (ML service)

**Goal:** ML‑service coverage gates should reflect the quality of **API and inference‑time code**, without being dominated by long‑running offline training/tuning CLIs.

- **Coverage configuration:**
  - `pyproject.toml` configures coverage to track `app` and `ml` packages, with `branch = true`.
  - Training/tuning entrypoints that are exercised via separate flows are excluded via `omit`:
    - none, since P-6: the training CLIs it excluded (`ml/train_win.py`, `ml/tuning/*.py`,
      `ml/validate_exports.py`) no longer exist, and `ml.xi.retrain` is thin enough to be
      covered by the same tests that cover what it calls.
  - The coverage number is focused on `app.main`, `app/xi_service.py`, the `ml/xi` package,
    config, and other request‑time paths.

- **Thresholds and CI integration:**
  - `ml-service/Makefile` defines `COV_MIN`, the minimum allowed coverage percentage for local `make coverage-check`.
  - The root `Makefile` exposes this as `COV_MIN_ML` so `make ml-service-check`/`make check-all` use the same gate.
  - `.github/workflows/ml-service-ci.yml` passes `COV_MIN` into `make -C ml-service coverage-check` in CI. **All three must stay in sync**; when overall coverage improves, raise all three together (e.g. from 57 → 74) and re‑run the gate locally before pushing.

- **Raising, never lowering:**
  - When `coverage` reports that actual coverage is above the current threshold, we **bump the threshold up to `floor(actual)`** (e.g. 74.99% → 74) in `ml-service/Makefile`, the root `Makefile`, and the CI workflow.
  - **`floor`, not the rounded figure the report prints.** pytest-cov decides `fail_under`
    on the *rounded* total but writes its FAIL line from the exact one, so a threshold of
    92 against 91.55 % prints "FAIL Required test coverage of 92% not reached" and still
    exits 0 — a gate that says FAIL and passes, which is worse than one that does neither.
  - We do **not** lower thresholds; if coverage regresses below the gate, the fix is to add or repair tests.

The same pattern applies to other components:

- **Frontend:** Vitest coverage thresholds live in `frontend/vite.config.ts` under `test.coverage` (lines, functions, statements, branches). Whenever we meaningfully improve tests, we raise each threshold to the floor of the corresponding metric.
- **Go app:** Go coverage gates use `COV_MIN` in `go-app/Makefile`, mirrored as `COV_MIN_GO` in the root `Makefile` and as the `COV_MIN` env var in `.github/workflows/go-app-ci.yml`. As with ML service, raise these only when coverage improves.

---

## XI-responsive win model (`ml.xi`)

**Glossary keys** (L-1, `ml/xi/glossary.py`): `objective_auc`, `display_auc`, `display_auc_mean`, `display_auc_seed_sd`, `objective_brier`, `display_brier_mean`, `base_rate_brier`, `marginal_value`, `win_probability`.

**Purpose:** a win model whose every input is a function of the two elevens, so it can rank
candidate XIs — the objective for team selection. It replaces the windowed-form aggregates
of `ml.win_features` for that job (held-out AUC 0.73 T20 / 0.69 ODI / 0.75 T20I against
0.63 / 0.56 / 0.59, on the Cricsheet-JSON source; the current figures, trained on Postgres and
measured walk-forward, are in `docs/ML_PIPELINE_REARCHITECTURE_PLAN.md` §8.4).

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

**Run:** `make retrain CUTOFF=2025-09-01` reads the database (`POSTGRES_*`); with
`CRICSHEET_DIR=data/go-app/cricsheet` it reads the raw Cricsheet JSON instead (same format
taxonomy as `format.go`, ~2 minutes for the full archive). Writes `xi_win_<FMT>.joblib`,
`xi_ratings.joblib` and `xi_win_report.json` (AUC and Brier for both models over three seeds,
base-rate Brier, and the best single column's AUC — a model that cannot beat its own best
column is not being measured). `POST /admin/reload` picks the artifacts up; `GET /xi/status`
shows what is loaded.

**Serving:** `POST /xi/predict-win` and `POST /xi/optimize` take player ids, not feature maps.
The optimiser (`ml/xi/optimizer.py`) seeds greedily, then steepest-ascent single swaps, then
pair swaps, under constraints expressed through the same vectors the model reads (a bowling
option is a player whose expected balls bowled clears the format threshold). **This is the
only selection path** — there is no flag, and `selection.win_model` is a retired config key
(P-5) that `ValidateForServer` refuses by name. A format that is not offered an optimised
selection is served the rating-ordered pick with its reason on the wire, never a silent
fallback to another optimiser (§8.7).

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

**Glossary keys** (L-1, `ml/xi/glossary.py`): none — `data_quality` is declared a non-metric subtree: these are counts of what the pass read and skipped, not measurements of a model.

Every rating pass counts what it dropped and what it found odd, and `retrain` fails on the
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
make retrain CUTOFF=2025-09-01 ACCEPT_DATA_QUALITY=1
```

Current baseline on the full dataset: 22,734 matches offered and 22,734 read, 1,710
undecided, 0 namesake sides, 1,358 sides of more than eleven (concussion and injury
replacements, which Cricsheet lists in full), 0 unresolved player keys, 13,569 players.

### Source parity (`make xi-parity`)

**Glossary keys** (L-1, `ml/xi/glossary.py`): `max_abs_difference`.

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

**Glossary keys** (L-1, `ml/xi/glossary.py`): none of its own: the rows are the population every performance metric is measured on.

The same day-close pass also emits one row per (match, player) — the training frame for the
performance model (L2-B). Each row carries the player's as-of vectors, the expected role
(`exp_bat_position`: decayed mean batting slot shrunk toward 7; `bat_innings_share`; batting
and bowling impact split by powerplay / middle / death, `contract.PHASE_BOUNDS`), the own-side
and opponent-side aggregates, and venue context — joined with what the player then did
(balls, runs, fours, sixes, dismissals, actual batting position, balls bowled, wickets,
runs conceded). Rows cover **all XI players**, never only those who batted: who got to bat
is decided by the result, and a population selected by the outcome is a leak (H-20).
`ml/xi/rows.py` assembles the rows for both the training pass and the parity check, so the
two cannot spell a column differently; the frame is never written to disk as a contract —
`retrain` and the harness both consume it in memory. Since P-3 the rows also carry `catches`
(each fielder named on a caught dismissal) and the sequence families
(`contract.SEQUENCE_FAMILIES`: dot streaks, reactions, spells — the ball-by-ball patterns the
deleted `seqcalc` tables precomputed, re-expressed as as-of accumulators with per-ball flags
from `ml/xi/sequence.py`). The columns are always in the frame so the question can be re-asked
without a new pass, but **E1 kept none of them**: `contract.SEQUENCE_FAMILIES_KEPT` is empty
and the performance model reads no sequence column.

### Performance model (L2-B, `ml/xi/performance.py`, P-3)

**Glossary keys** (L-1, `ml/xi/glossary.py`): `within_match_spearman`, `within_match_spearman_involved`, `top3_hit_rate`, `mae`, `pinball`, `pinball_by_level`, `coverage_80`, `coverage_80_strict`, `width_80`, `q10`, `q90`, `probabilities`, `reliability`, `vs_career_mean`, `vs_career_quantiles`.

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

**Inputs.** The row's as-of vectors and expected role, the sequence families E1 kept (none —
`SEQUENCE_FAMILIES_KEPT` is empty), both sides' aggregates, venue context, the Elo edge, and
the innings (bat first / chase). The
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

**Run.** `make retrain CUTOFF=…` fits the performance models beside the win models and
writes `xi_perf_<FMT>.joblib`; `xi_win_report.json` carries the holdout numbers per target
under `performance`. `make evaluate` is where the choice-facing numbers come from. The
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

**Glossary keys** (L-1, `ml/xi/glossary.py`): `dispersion_ratio`, `bias`, `median_mae`, `actual_sd_around_simulated_mean`, `simulated_sd_mean`, `below_q10`, `above_q90`, `pit_deciles`, `delta_brier_simulated_minus_display`, `p_bat_first_wins`, `spread_share`, `spread_runs`, `range_10_90`.

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

**Measured by (E2, `ml/xi/sim_harness.py`, in `make evaluate`).** Per format and window,
beside the display model on the same matches: Brier and reliability of the simulated P(win)
(pre-toss, the comparable one; toss-known beside it) against the display model's and the base
rate; coverage **and** width of the simulated totals' 10–90 interval against actual first
innings that ran their course (H-22 applied to totals), with the chase total the same way on
every match; margins; latency per fixture. E2's rule (plan §5): simulated P(win) worse than the
display model by more than 0.01 Brier on the walk-forward folds and it is a description, never
the displayed probability — `simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED` per format, and
`/simulate` returns both with `headline_source`. The H-8 parity check compares the simulator's
draws at a fixed seed from the as-of path and from the training frame's rows.

**First numbers** (plan §8.3), measured against the windows as they stood at P-4 — the locked
window has since rotated (A-4), so these are not comparable line-for-line with a run made
today. Walk-forward, 7 folds: without the shared factor the
first-innings totals' 10–90 coverage is 0.64 (T20) / 0.58 (ODI) with a dispersion ratio of
1.42 / 1.36 and a U-shaped PIT; with it 0.76 / 0.74 at ratio 1.02 / 1.02, the interval
widening from 61 to 83 runs (T20) and 107 to 151 (ODI) — the narrower one was the wrong one.
Locked window as it then stood (≥ 2025-09-01, scored once): coverage **0.786** (T20, 1,521 first innings) and
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
source), and the margin. Limited-overs formats only (422 otherwise; TEST has no innings length
and is served the rating-ordered XI, H-17). The go-app scorecard reads it unconditionally
(`predictteam/xi_simulation.go`): innings totals, per-player points and their `runs_range` /
`wickets_range` come from the draws, P(win) from the display model, and the response carries
`xi_simulation` (innings ranges, simulated P(win), which model is the headline) and
`explanation` — each selected player's marginal value from `/xi/optimize` and share of the
total's spread from the simulator (L3). The extras and innings models and the win-probability
rescale that used to sit on that path went with P-5 and P-6; nothing rescales a simulated
total toward anything, on any path.

### As-of serving (`ratings_as_of`, P-2)

**Glossary keys** (L-1, `ml/xi/glossary.py`): `max_abs_difference`, `fielded_eleven_max_abs_difference`.

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

### Evaluation harness (`make evaluate`, L4 / H-19)

**Glossary keys** (L-1, `ml/xi/glossary.py`): every key the report emits — `objective_auc`, `display_auc_mean`, `base_rate_brier`, `swap_violation_share`, `specific_vs_typical_delta`, `agreement`, `bar`, `expected_if_exactly_right`, `delta_brier_mean`, `auc`, `test_auc` and `max_abs_difference` among them.

One command, one JSON report (`xi_evaluate_report.json`): rolling-origin walk-forward over
quarterly cutoffs 2024-01 … 2026-06 for every choice-facing number, and the **locked
window** (matches ≥ 2026-09-02) scored once per release, labeled, never used for a choice.
The window's start date and the date it was last rotated travel in the report
(`locked_start`, `locked_window`) and on every locked figure's label, so a reader can tell
which window a number came from — see *Rotating the locked window* below.
Per format it reports, with mean ± spread over cutoffs (and seeds where a model has one):
objective/display AUC and Brier against the base rate; the specific-XI-beyond-typical-XI
delta and swap monotonicity (the selection gates that replace P-0's winner accuracy); the
best-single-column leak canary with the TEST-format control (H-2); and the performance
model on the player-match rows — per target and format, within-match Spearman, top-3 hit,
the median's MAE, pinball loss and the 10–90 interval's coverage beside its width (H-22),
with the career-mean, career-quantile and rating-expectation baselines on the same
population, and quantile targets whose walk-forward coverage is off nominal recalibrated
on a temporal fold for the locked window (H-5); the simulator (E2) — simulated P(win)
against the display model's, totals coverage and width, margins, latency, with E2's display
rule decided on the folds; and the natural experiment for selection (E5, below). It ends
with the train/serve parity check (H-8): the last 50 matches rebuilt from the as-of serving
path and compared with the training frame — rows, performance predictions and simulator
draws at a fixed seed alike — and the run fails if they differ.

**E5, lineup-only (`ml/xi/natural_experiment.py`, P-7).** The one selection gate that
varies one side while holding the rest of the world still. For each consecutive pair of one
side's matches in a format with 1–3 lineup changes, *both* elevens are scored against match
k+1's opponent at match k+1's as-of ratings — the previous eleven is read from the as-of
serving path (`AsOfRatings`) in one advancing pass, the fielded eleven from the frame's own
row, and the two are checked against each other as a parity check on the pairing — and the
metric is sign agreement between the objective's preference and the result change, over the
pairs whose result moved. A pair is scored by the objective of the fold whose window holds
match k+1, fitted before that fold's cutoff; the report carries the per-fold rates, the pooled
development rate (the decision) with its standard error and the effect size the objective
claims (median |Δ|), and the locked window beside them, labelled. §5's *as-played* definition
is not computed, and the report's slot says why in one line: scoring each match against its
own opponent makes a nonzero Δresult an identity on `won_k`, which match k+1's as-of state
already contains, so it measures mean reversion, not selection (plan §8.6).

*The bar is derived, not chosen* (plan §8.8). The harness takes the objective at its word —
every fixture's result Bernoulli at the objective's own probability, so the lineup-only Δ is
exactly what it claims — and simulates the agreement rate an exactly-right objective would
produce on these pairs; the bar is that distribution's 5th percentile, so it already carries
the sampling noise of the pairs available. A format passes or fails a bar it could in
principle reach, and the report says what an exactly-right objective would have scored beside
it. Per format the report then states the selection decision — `optimised selection: yes/no,
because E5 said X against bar Y` — with the serving policy read from
`ml.xi.optimizer.NOT_OPTIMISED_REASONS`, so a run whose verdict disagrees with the policy says
so. The policy itself is set by hand from the report, as E2's is. As of P-7 (plan §8.8): T20I
and ODI pass their derived bars and are searched on the win objective; **T20 fails its bar and
is served the rating-ordered eleven**, labelled with that reason, beside TEST's H-17 reason.

**Rotating the locked window (H-19, A-4).** A locked window is only a holdout while no
decision has read it. The moment its numbers have guided a release choice — a feature family
kept, a model class picked, a format scoped in or out — it is a window the system has already
fitted itself to, and scoring the next release on it measures optimism, not accuracy. So the
window rotates, on the following rule.

- **When.** A window rotates once its data has guided *any* release decision, and at the
  latest at the release that consumed it. In practice: whenever a batch of modelling work
  lands that read the locked figures, its merge closes the window.
- **Where the line is drawn.** At the date of the decision that spent the old window — the
  merge date of the work that read it. Everything before that line is data some choice has
  seen; everything at or after it is data no choice has read past, which is exactly the
  property H-19 needs. Rotating to a *later* date would discard clean matches, and to an
  earlier one would keep read data in the holdout.
- **What happens to the old window.** It retires into the walk-forward folds: the harness's
  `WALK_FORWARD_CUTOFFS` is extended on the same quarterly cadence up to the new line, so the
  retired window is scored as ordinary folds rather than thrown away. Folds are where reuse is
  allowed — they are the development surface — so the data keeps working, in the only place it
  still can.
- **Where it is written.** One place: `ml/xi/evaluate.py` (`LOCKED_START`, `LOCKED_ROTATED_ON`,
  `LOCKED_PREVIOUS_START`, `LOCKED_ROTATION_REASON`). The report carries all four in its
  `locked_window` block and repeats the start and rotation dates in the note on every
  locked-window figure, so the rotation is a record every run reprints rather than something a
  reader has to remember. The Evaluation tab and the System map render both dates from that
  block.
- **A freshly rotated window is empty, and says so.** Immediately after a rotation the window
  holds no matches: its per-format node carries `n_eval: 0` and a `skipped_reason` instead of
  numbers, which is the correct answer and not a failure. It fills as `make import` runs on
  cadence. Read nothing from it until it has enough matches to be worth a sentence — the folds
  are where the numbers are meanwhile. While it is too small to fit a model, the train/serve
  parity check (H-8) serves the most recent fold's performance model instead, and each format's
  `parity_model_window` names which window fitted the model it compared.

The current window was declared on **2026-09-02**, at the P-7 merge date, because every model
choice of the P-0…P-7 migration consulted the previous window (matches ≥ 2025-09-01). That
window is now the last four walk-forward folds (2025-09, 2025-12, 2026-03, 2026-06).

**Gate registry (H-23, `ml/xi/gates.py`).** Every gate the report prints — H-17's AUC line,
swap monotonicity, specific-vs-typical, E5, E2, the quantile coverage, width beside coverage,
the leak canary, parity, and E3 for the script that runs it — declares what it *varies*, what
it holds *fixed* and what *decides*, with the path at which the report carries its number. The
report embeds the registry, `gates.check_report` fails the run if a gate is printed without an
entry or an entry has nowhere to be read from, and the Evaluation tab renders the triple beside
each number. An experiment script prints its gate's triple before it runs.

**Metric glossary (L-1, `ml/xi/glossary.py`).** The same pattern for what the numbers *mean*:
one entry per reported metric key — a plain-language name, an explanation, the reference band
this system measured, and which direction is better. The report embeds the glossary,
`glossary.check_report` walks every metric key the report emits and names any that has neither
an entry nor a declared reason for not being a metric, and a harness test fails on one — so a
new metric cannot reach a surface unexplained. `GET /xi/metric-glossary` serves the same
entries from the code, with no artifacts needed, and every metric label in the frontend opens
the entry for its key, which is why no component holds metric prose of its own. The copy is
the table in `docs/FOLLOW_UP_PLAN.md` § 3.

```bash
make evaluate                                        # the database
make evaluate CRICSHEET_DIR=data/go-app/cricsheet    # the raw archive
```

It refits every model per fold per format, which on the full database takes about **54
minutes**. That is why it is the optional `evaluate` step rather than part of `retrain`, and
why it writes its report beside the runs rather than into one: it measures the harness's own
refits, not the run `current` points at.

## The Docker image

`ml-service/Dockerfile` builds one image, from `requirements.txt`.

```bash
docker build -f ml-service/Dockerfile -t cric-app-ml:latest .
```

There were two stages — a 1.23 GB `serve` and a 5.02 GB `train` carrying PyCaret, AutoGluon,
SHAP and their transitive weight (torch, tensorboard, chronos) — until P-6 deleted the
auto-tune stack they existed for. What is left of hyperparameter search is a three-point grid
inside `retrain`, which runs on scikit-learn, so the serving image is also the training image
and there is one dependency set for the image, for CI and for a local venv.

---

## Probability calibration (classifiers)

**Glossary keys** (L-1, `ml/xi/glossary.py`): `brier`, `reliability`.

For classifiers (e.g. the win model), predicted probabilities can be **calibrated** (Platt scaling or isotonic regression) so they reflect true frequencies, and evaluated with a reliability diagram, Brier score, or ECE.

**Calibration of what is displayed is H-5, and it is measured.** The harness reports a
reliability curve and Brier against the base rate per format for the win models, and per-end
quantile coverage for the performance model; an isotonic recalibration
(`ml/xi/perf_calibration.py`) is fitted on a temporal fold and applied when a quantile target's
coverage is off nominal. It ships in place and, so far, unused — no target has tripped the
check — and the harness re-decides it every run. The standalone `ml.calibrate` module that
predated all this was never wired into anything and went in C1-4/C1-6.

---
