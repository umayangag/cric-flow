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
| L4 harness (`ml/xi/evaluate.py`) | — | Walk-forward folds and the locked window, the selection and performance metrics, the leak canary, the train/serve parity check and the market benchmark (X-4) |

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
  the as-of serving path — from a store written to a run directory and loaded back the way
  `reload` loads one — and fails the run on any difference in the rows or in the served
  probabilities (H-8, EVAL-10).
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
| **retrain** | `make retrain CUTOFF=2025-09-01` | The whole model build: rating pass → XI win models (with the grid) → performance models → the run's report → `manifest.json`, which carries the run's **usability verdict** (EVAL-04, below). Writes `runs/<run_id>/` and **publishes nothing**; exits 1 when the run is not usable, and `reload` refuses to publish it. |
| **reload** | `make reload [RUN=<id>]` | Point `current` at a run and load it into the running service. With no run id: the newest run on disk, which is the one the retrain before it built. Naming a run is how you roll back to an earlier one. |
| *evaluate* | `make evaluate` | L4 over the database: walk-forward folds, the locked window, the selection and performance metrics, the leak canary, the parity check. Touches no artifact `current` points at. Optional, and slow — see below. |

`make up-all CUTOFF=<date>` is the whole chain from an empty database; `make full-pipeline` is
retrain → reload against data already imported.

**Why reload is separate from retrain.** They answer different questions — "build a run" and
"serve that run" — and a retrain that published itself would leave no way back to the run
before it. Naming a run is how you swap back.

**After a merge that changes an artifact's shape, a deploy is `retrain → reload`, not a
rebuild.** A rebuilt container against the runs already on disk is exactly the refusal
§ Runs, manifests and staleness describes, which also lists the merges that have done this.

**Why evaluate is separate from both.** The harness refits every model per fold per format:
measured on the full database it takes **~2 h 10 min**: measured at 2 h 05 min on the full database once EVAL-03 gave every format a second member fit, plus EVAL-06's measured 177 s for the display grid the harness now runs in every window over the eleven folds the
rotation left (A-4), against a whole pipeline that runs in a fraction of that. The figure has
moved twice and both moves are fits the harness did not use to do, so budget the current one:
it was ~54 minutes over seven folds, ~67 over eleven, and 2 h 05 once the performance members
were fitted on the fold *and* on the full history. Folding it into every retrain would make
the pipeline unrunnable at any sensible cadence. What a retrain records is its *own* holdout report — the numbers the models
it just fitted produced — and the manifest names it, so nothing quotes a measurement of a
different run.

> **`CUTOFF` bounds the training data.** Rows with `match_date` on or after it are dropped,
> leaving everything from the cutoff onward as a holdout. To produce a model you can honestly
> evaluate, train with a cutoff that leaves a window behind it.

---

## Hyperparameters

**Glossary keys** (L-1, `ml/xi/glossary.py`): none — the chosen values are inputs, and `hyperparameters` is declared a non-metric so the completeness gate does not ask anyone to explain a learning rate.

There is one search, it is three points wide, and it runs inside `retrain` — and inside every
window the harness scores, because the grid is part of the recipe and the harness measures
the recipe (EVAL-06, below).

`ml.xi.train.DISPLAY_GRID` holds three settings for the display model (depth, learning rate,
iterations). Each is fitted on the first 80 % of the training rows *by date* and scored on the
last 20 % — inside the training window, strictly before the holdout, so choosing a
hyperparameter cannot see the rows the run is scored on (H-19). The incumbent (the setting the
display model has always been fitted with) keeps its place unless a candidate beats it by more
than `DISPLAY_GRID_MARGIN` = 0.002 AUC: differences under the noise floor are not evidence
(H-14), and a grid that reshuffles the model on 0.001 every release is a source of drift.

**The harness fits the model the grid would ship (EVAL-06).** `train_format` and the
harness's `_evaluate_win_window` fit the display model through one function,
`fit_display_model_as_shipped`: run the grid on the window's training rows, fit the pick on all
of them, record the choice. Each walk-forward fold and the locked window therefore carry a
`hyperparameters` record — pick, reason, every candidate's inner-split score, `n_iter` — under
the same key the run manifest uses, and the walk-forward `display_auc` describes the model a
retrain at that cutoff would have served. Before this the harness fitted grid point 0
regardless, so a format whose retrain had picked another point had walk-forward evidence for
a model it never served — and that was not hypothetical: TEST picked point 1 in five of the
ten manifests on disk, and on the harness's own windows the grid moves off point 0 in **nine
of T20I's twelve** (measured on the full database at `7652a0e8`; T20 and ODI never, TEST
once). The grid costs the harness **177 s over the 48 windows** on that measurement (T20
6.7 s per window, T20I 6.3 s, ODI and TEST under 1 s, taken while a test run shared the
cores, so an upper bound) — about 2.5 % of a two-hour run. Where the grid moved in T20I, the
pick's out-of-window AUC against the incumbent's read +0.012, +0.011, +0.004, 0.000, −0.003,
−0.009, −0.009, −0.023 on windows of 23–57 matches: the grid is not buying anything the
harness can resolve there, and the margin of 0.002 sits well under the noise of a
~350-row inner split. That is a finding about the grid, recorded in `docs/AUDIT_FINDINGS.md`
under EVAL-06's entry; the harness now reports it every run instead of hiding it.

**The model the grid scored is the model fitted (EVAL-01).** `make_display_model` passes
`early_stopping=False` explicitly. sklearn's default is `'auto'`, which switches early stopping
on above 10,000 rows with a *random* 10 % validation split — so the grid, scoring on the inner
80 % of a format's rows, fitted every candidate to its full `max_iter`, and the winner was then
refitted on all the rows — above the line in T20 only — to a different, early-stopped
iteration count on a random 90 % of them (measured: 117 of 300 on a 10,200-row frame). With
it off, `max_iter` is the iteration count in every format, walk-forward folds no longer change
regime as they cross 10,000 rows, and the most recent rows are not held back from the final
fit. The grid is the whole regularisation choice; a temporal early-stopping split built by hand
was the alternative, and was not taken because the grid already fixes the iteration count and
a second, hand-built split would be a second thing to get right.

The choice, the reason, every candidate's score and **`n_iter`** — the iterations the served
model actually ran, equal to the chosen `max_iter` by construction — go into `manifest.json`
under `hyperparameters`. That is the whole record — there is no tuned-params table, because a
table nothing could join back to an artifact was how a model came to carry parameters from a
search it had never seen.

This replaced a two-phase Optuna search with PyCaret and AutoGluon ranking. It was removed
because the model class was measured not to be the constraint, twice; the constraint is the
game (§1 of the re-architecture plan), and no amount of search moves it.

---

## Runs, manifests and staleness

**Glossary keys** (L-1, `ml/xi/glossary.py`): the manifest's headline metrics are `objective_auc` and `display_auc_mean`; `n_train` and `n_holdout` beside them are declared counts, not metrics.

**One key, one quantity** (EVAL-05). Both headline numbers are the *served* reading: the
probability marginalised over the toss — the mean of team1-bats-first and team2-bats-first —
because that is what the optimiser maximises (`XiStore.objective_probability`) and what
`/xi/predict-win` answers when the caller does not say who bats first. Since GO-07 the serving
path forwards a toss the caller *does* name, and the answer is then the toss-aware reading —
a different quantity, which these two keys do not score; `/xi/predict-win` reports
`toss_marginalised` so a caller can tell the two apart. The harness reports the
same quantity under the same keys in every walk-forward fold, so a manifest's `objective_auc`
and a fold's are one number. Until EVAL-05 the manifest quoted the model read at the batting
order that actually happened under the harness's key, so `objective_auc` meant two things
depending on which file wrote it. On the one scored run on record (cutoff 2025-09-01) the two
readings differ by −0.0002 T20, +0.0013 T20I, −0.0062 ODI and −0.0075 TEST (toss-aware minus
marginalised): inside one standard error of the AUC in every format, and the marginalised
reading is the *higher* one in three of four, so the toss-aware headline was not optimistic —
it was a different number. The run report (`xi_win_report.json`) carries both readings per
model, named `objective_marginalised` / `objective_toss_aware` and `display_marginalised` /
`display_toss_aware`; nothing in it is called plain `objective` any more. Manifests written
before this carry the toss-aware reading under `objective_auc`; the only scored one predates
`ratings_through` and is already refused by the loader, and every cadence run since was trained
at today's cutoff and scored nothing, so no loadable manifest's headline changed meaning.

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

`manifest.json` carries the run id, when it was created, the cutoff, **`ratings_through`**,
the dataset sha (a digest of the cricket the pass consumed — computed from what was read,
because ml-service does not mount the dataset directory), the git sha, the rating params, the
hyperparameters the grid chose *and why* with the iterations the display model ran (`n_iter`),
the run's headline metrics per format, and the rating state's shape. It is written **last**, so a directory only becomes a run once
everything it names is on disk: a retrain that dies half-way leaves wreckage the loader never
selects and `/artifacts/status` lists as "no manifest".

**Could this run be built again?** (EVAL-12) Six fields answer it, and until EVAL-12 two of
them answered it wrongly and four were not there.

- **`git_sha`** — the commit the code came from. It is read from the checkout first (with
  `-dirty` appended when the tree carried uncommitted changes, because a clean sha over a
  dirty tree names code that was never committed), from the `GIT_SHA` environment variable
  second, and failing both it records the word `unknown`. The second route is what the
  serving image needs: the image carries neither the version-control binary nor a repository
  directory, so `git rev-parse` fails inside it and every run built through `/admin/train/*`
  used to record an empty string — which renders on every surface exactly as a field that is
  not there does. `make` passes the checkout's commit to `docker compose`, which bakes it in
  as a build argument; a bare `docker build` should pass
  `--build-arg GIT_SHA=$(git rev-parse HEAD)`.
- **`dataset_sha` and `dataset_digest`** — the sha is over three kinds of line: one per
  decided match (identity, sides, venue, label), one per player-match row (who was in the XI
  and what the deliveries did to him), and one per pass-level count, which are summed over
  *every* match read and are the only cover for undecided matches, since those produce no
  row. Before EVAL-12 it hashed `match_id|match_date` of the decided matches alone, so a
  re-import that rewrote every squad and every delivery produced a byte-identical sha — which
  is the whole use the field has. Measured on the full archive (21,293 matches, 468,461
  player-match rows): **0.96 s**, against a 216 s rating pass. `dataset_digest` names the
  scheme and the row counts, so two shas are only ever compared when they were computed the
  same way. What it still cannot see: a change *inside* one undecided match that leaves the
  pass totals alone — closing that would mean digesting every delivery, which is the one scan
  this avoids.
- **`source`** — `PostgresSource` or `CricsheetJsonSource`. The two have disagreed before
  (FEAT-04, IMPORT-05/06), so which one a run read is part of building it again.
- **`library_versions`** — python plus scikit-learn, numpy, scipy, pandas and joblib. Pinning
  the commit without these does not reproduce a fit.
- **`model_params`** — the win models' constants the grid never varies: the objective's `C`
  and iteration ceiling, the display model's `l2_regularization`, `min_samples_leaf`,
  `early_stopping` and `random_state`, the grid's margin and validation fraction. The grid's
  *choice* stays in `hyperparameters` and is not restated here.
- **`performance_spec`** — the performance model's `FitSpec` per format, quoted from the run's
  own report so the two cannot fall out of step.

`dataset_digest` is **required**: a manifest without it was written when the digest could not
see a squad or a delivery, so its sha answers a different question, and the loader refuses the
run by name rather than inviting a comparison of two numbers that do not mean the same thing.

**`cutoff` and `ratings_through` are two dates** (P2-2). The cutoff is the training boundary
the operator asked for — today, for a refresh — and rows at or after it are the holdout.
`ratings_through` is the last match date the rating pass actually consumed, the state's own
`last_date`, and it is the date every served prediction is "as of". On the dev box they
differed by a day on the served run (cutoff `2026-09-03`, ratings through `2026-09-02`),
because the archive lags the calendar. Retrain writes `ratings_through` from the state it just
built, so "what date is this run's data?" is answered from the manifest — and from
`/artifacts/status`, which lists it per run — without loading anything. The field is
**required**: a manifest written before it existed is refused by name (below), listed with
that reason, and never served with a date read off its joblib instead (§8.7). Nothing is
backfilled; an older run is retrained, not patched.

**`formats` is what the run trained, not what it managed to score** (B-3). Rows at or after
the cutoff are the holdout, so a retrain at today's cutoff — which is what a scheduled run
does, `DefaultCutoff` being today UTC — has no holdout at all and reports no AUCs. The
formats it trained are still listed, with their row counts, and `format_notes` says per
format *why* the discrimination numbers are missing: `trained on N rows but not scored: …
(0 rows at or after the cutoff)`, or `not trained: insufficient training rows (…)` for a
format that fitted nothing. An empty `formats` therefore means nothing was trained and
nothing can be served; a populated one with notes means the models exist and this run
measured nothing about them — use `make evaluate`, or a cutoff that leaves a holdout, to
judge them.

**`usable` is the run's own verdict on whether it may be published** (EVAL-04,
`ml/xi/run_usability.py`). Before it, a run whose objective had stopped ranking — a broken
feature join, an inverted label — was written, listed and served by the next `reload` exactly
as a sound one. Retrain now evaluates two clauses per format it scored and writes the result
into the manifest as `usable` with `unusable_reasons` per format: the objective's holdout AUC
(the served, toss-marginalised reading the manifest quotes — EVAL-05)
must be above the base rate's 0.5 (a constant predictor's AUC — an objective not above it does
not rank, and a run with nothing to select on in a format is not published); and, **only when
the run `current` points at was trained at the same cutoff** — the same holdout — this run's
AUC may not fall under that run's by more than one Hanley–McNeil standard error of the AUC on
this holdout. The margin is the measurement's own noise, computed from the counts the report
carries (0.013 T20 on 1,635 rows, 0.036 T20I, 0.028 ODI, 0.046 TEST on the one scored run on
record); a seed spread is zero by construction for a logistic objective, and the harness's
fold spread is between-window variance, so neither is a floor. Across different cutoffs the
two AUCs score different matches and no comparison is made. An unusable run is still written —
the operator has to read what it produced — and `retrain` exits 1; `runs.set_current` refuses
to point `current` at it, so a `reload` naming it answers 409 `RUN_ARTIFACTS_INVALID` with the
reasons and keeps serving what was serving, and a reload naming nothing skips it for the newest
usable run. **What the gate cannot judge is a run with no holdout** — which is every run a
scheduled retrain at today's cutoff writes; such a run is `usable` with `format_notes` saying it
was not scored, because the gate has nothing to read and says so rather than inventing a
number.

**The loader refuses what it cannot serve.** `XiStore.load` reads the manifest first and
raises `RunArtifactsInvalid`, naming the run, when there is no manifest, when the manifest
carries no `ratings_through`, when the manifest's `ratings_through` disagrees with the
state's `last_date` (the refusal names both dates — the two were written by one retrain and
cannot differ unless the directory is not the run its manifest describes), when an array this
code reads is absent, when a player array is narrower than the number of players the
payload registers, or when a win artifact was fitted on `objective_cols` / `display_cols`
that are not the contract's. The date assertion is what makes the manifest's date *the*
date: `/xi/status`, the served-ratings stamp and `/artifacts/status` keep reading it off the
state that computed the answer, and the manifest agrees with them by construction rather
than by a second read. That last refusal is B-7's: the serving path builds its row from
the artifact's *own* column list, so a run predating the display change loads perfectly and
answers with the surface the change removed. That is D-6: a rating artifact written before P-2 loaded without complaint
and then raised `IndexError` on the first request past slot 1024, while `/xi/status` reported
`loaded: true`. An artifact was trusted because it loaded; now it has to say which run it is
from and what shape it is in. The refusal reaches `/xi/status`, `/health`, `/ops/status`, a
409 `RUN_ARTIFACTS_INVALID` from `POST /admin/reload`, and the prediction tab. A run on disk
that the manifest reader refuses is still listed by `/artifacts/status`, with `refused` set
to the reason and `null` on every loadable run, and is never picked as "the newest run" by a
reload with no run named; the Ops runs panel shows the reason under the run.

**A change to what an artifact contains makes every run on disk unloadable.** The rating
payload's arrays and tables, a win artifact's feature list, the manifest's required fields:
each is read by name, and a run written before the name existed does not carry it. Nothing
backfills or shims an old run (`ratings_through` above; B-7's `display_cols`), so after such a
merge the next `reload` refuses every run on disk by name, and the only remedy is a
**retrain** — about ten minutes on this database — then a `reload`. What the operator sees: the
refusal names the run and what it lacks; if a run was serving, it keeps serving with the
refusal in `error` beside it (B-13, `docs/BUG_BACKLOG.md`); a container rebuilt from the new
code starts with nothing loaded, because the run `current` points at is the refused one, and
the Lab answers nothing until a run is built. Nothing does that for you — ingest is run by
hand (`docs/PRODUCT_ROADMAP.md` § 2, route (a)), and a scheduled cadence retrains on its
rhythm, not on a merge. `manifest.git_sha` records which code wrote a run, so whether a run
predates a change is read off the manifest or `/artifacts/status` without loading it. It has
happened six times:

- **X-1b (#257)** — the rating payload gained the `birth_dates` table and the `debut_bat` /
  `debut_bowl` arrays; a payload without them is refused.
- **B-7 (#267)** — `t1_pelo_std` / `t2_pelo_std` left `DISPLAY_FEATURE_COLS`; a win artifact
  fitted on the old list is refused.
- **FEAT-14** — the same two columns left `XI_FEATURE_COLS`, and the objective became a
  sign-bounded fit; a win artifact whose `objective_cols` still carry them is refused.
- **P2-2 (#280)** — the manifest gained the required `ratings_through`; a manifest without it
  is refused.
- **EVAL-12** — the manifest gained the required `dataset_digest`; a manifest without it is
  refused, because its `dataset_sha` was computed by a formula that could not see a squad or a
  delivery change and is not comparable with this code's. Every run on disk at the merge is
  refused by name, in the listing and on `/artifacts/status`; the remedy is the usual retrain.
- **B-11 (#285, and again in §8.15)** — `SimulatorCalibration` gained `chase_dispersion`, and
  §8.15 widened it: the field now holds either of two classes (`ChaseDispersion` or
  `CorrelatedChaseDispersion`, behind the `ChaseDispersionTerm` protocol) and
  `CHASE_DISPERSION` / `FitSpec.chase_dispersion` became an arm *name* where they were a bool.
  Neither is refused: the performance artifact is joblib-loaded with no shape check, so an
  older pickle restores without the field and reads the class default, `None` — which, under
  the default arm (`"none"`), is also what a fresh retrain writes, so nothing served is wrong
  yet. The shape still moved twice; the loader has no opinion on that artifact, and a run
  written before the class changed is served as it was. A load-time check for the performance
  artifact is the standing follow-up (B-14).

**Staleness (H-11).** A live prediction from a run whose **data boundary** is further back
than `ml.ratings_max_age_days` (default 14; `XI_RATINGS_MAX_AGE_DAYS` overrides) is refused
with `RATINGS_STALE` and a hint naming the steps that fix it.

The boundary is the manifest's `cutoff` — the date the run's data was built to, which
`retrain` stamps with the day it was run — and **not** `ratings_through`, the last match the
rating pass folded in (SERVE-03). The two are different quantities: the first belongs to the
pipeline, the second to the cricket calendar. Measuring the second meant a run retrained this
morning was refused whenever cricket had paused — and the hint's retrain would produce a run
with the same last match and be refused again, so the refusal was unclearable by anything it
named. It failed the other way too: the rating pass folds in every match the source offers
whatever the cutoff says, so a run trained to a boundary years back was called fresh as long
as the archive it read held a recent match.

There is one verdict per run and not one per format: a run has one cutoff, and nothing in the
artifacts dates a *format's* data except the last match played in it. The per-format question
— "has anything been imported that the served run never saw?" — is answered in go-app's
`/ops/status` (`freshness.retrain_due`), which is the component that can see the archive.

A request that names an `as_of` the as-of pass serves is not refused: a backtest asks for a
date and gets it, and refusing one would break the harness for a reason that does not describe
it. An `as_of` past everything the loaded state holds is answered from the through-today state
unchanged, so H-11 applies to it exactly as to a live request — otherwise naming any date after
`ratings_through` would be a way around the refusal (SERVE-08). Setting the limit to zero turns
the check off — a decision visible in config rather than a state the code can drift into. The
verdict, not just the date, is on `/xi/status`: `ratings.fresh`, `data_age_days`,
`max_age_days`, `data_through`, `ratings_through` and `code`. Both dates are reported so a
reader can tell "nobody has retrained" from "the archive is behind".

**The fixture's own date (SERVE-04).** `/performance/predict` and `/simulate` take a
`match_date` — the day the fixture is played — and a `gender`. Every date-dependent feature in
the rows those answers are built from (each player's age, and the age-aware cold start's band
for a debutant) is read at that date, and the gender picks the context baseline the fixture's
scoring rates come from. Until SERVE-04 the serving path stamped `state.last_date` on every
fixture and `gender=""`, so a match next month was aged as of the last match in the state —
twelve days behind on the dev box, and further for any fixture worth asking about. A caller who
sends no `match_date` still gets one, because the features cannot be computed without a date:
it is `as_of` where a backtest gave one and today otherwise, and the answer's `fixture` block
says which (`match_date`, `match_date_source`, `gender`), so a defaulted date is never
invisible. go-app always sends the date the caller typed.

**Every prediction names the state it was served from (P1-5).** `/xi/optimize`,
`/xi/predict-win`, `/simulate` and `/performance/predict` each answer with
`served_ratings: {run_id, ratings_through}`, read off the store that computed the answer —
the same manifest and `state.last_date` `/xi/status` reports, so the two cannot disagree
about a store. It rides on the answer rather than being read from `/xi/status` afterwards
because a status read describes whatever is loaded *now*, and a reload can land between a
prediction and the read. go-app requires every answer a prediction is assembled from to
carry the same stamp and puts it on `POST /api/predict/team-selection` as `ratings_through`
and `run_id` (both required); a prediction whose calls straddled a reload is refused with
`409 SERVED_RUN_CHANGED`, and a store that cannot name its run is refused rather than
stamped blank. A backtest's answer (`as_of` named) is dated by the as-of state it was served
from, not by the through-today state.

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

**Glossary keys** (L-1, `ml/xi/glossary.py`): `objective_auc`, `display_auc`, `display_auc_mean`, `objective_brier`, `display_brier_mean`, `base_rate_brier`, `marginal_value`, `win_probability`.

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
prior), expected involvement (`exp_balls_faced` / `exp_balls_bowled`: balls faced / bowled per
**XI appearance** — every match the player was named for, batted or bowled in or not, the
balls and the appearances forgotten on one clock; a tailender who batted once in ten reads a
tenth of that innings, not the innings), experience, a keeper flag and a player Elo. A
player is a **bowling option** — the one predicate behind `n_bowlers`, `n_allrounders`, the
roles a surface prints and the optimiser's bowling-cover constraint — when his expected
balls bowled per appearance clear `contract.MIN_BOWLING_BALLS` (3 / 4 / 19 / 40 in T20 /
T20I / ODI / TEST). Those numbers are the bar the pass applied before FEAT-01 (12 / 12 /
30 / 60 balls per match *bowled in*) translated into the per-appearance unit by prevalence:
the per-appearance quantile that admits the same share of the archive's XI appearances the
old bar admitted, derived from the population before any outcome was read (FEAT-15; the
derivation is recorded beside the constant). Read per appearance, the old numbers were a
far higher bar — a frontline bowler who bowls his allocation in half his appearances sat
on the line, and a fifth of T20I sides and a quarter of T20 sides read as short of five
options; at the translated bar the share of decided sides since 2024 under five is back at
3-4 % in the T20 formats and 13 % in ODI and TEST, against 4-5 % and 16-17 % before. **A
side is the eleven that started** (FEAT-02): everyone the source lists for it less its
replacements — the concussion substitute, impact player or supersub who came in after the
start, whom Cricsheet lists with the rest and names in a `replacements.match` entry on the
delivery he joined at. Both sources leave him out of `team1_players` / `team2_players`
(the archive path by `sources.replacement_keys`, the Postgres path by
`match_player.is_replacement`), so he is not rated as a member, not counted as a debutant
or a bowling option, gets no player row and no share of the result's Elo; his deliveries
stay his own, so what he did still reaches his ledger. Before this the 1,342 sides that
used one were rated and aggregated as twelve while serving always aggregates eleven, and
his presence was post-start information in a pre-match row. A side's eleven vectors aggregate to `contract.SIDE_FEATURE_STEMS`: batting and
bowling impact weighted by involvement, top-6 / top-5 sums, role coverage (bowling options,
keeper, all-rounders, debutants), Elo summaries. Team-level context (team Elo, form,
head-to-head, venue bat-first bias, venue familiarity) is kept in a separate column list
because it cannot distinguish two XIs.

**Two models per format.** `objective` — logistic regression on the XI columns, fitted under
the contract's signs (`ml/xi/signed_logistic.py`: the same L2 log-loss as sklearn's `C=0.3`,
solved by L-BFGS-B with each coefficient bounded by its column's `monotone_directions`
entry). Additive *and* monotone by construction: a `+1` stem's own-side sensitivity is
non-negative in both batting orders, so a one-player upgrade never lowers p and the
harness's swap probe reads exactly 0 in every format. This is what `/xi/optimize` maximises.
Until FEAT-14 the fit was unconstrained and its monotonicity was only empirical — the T20I
objective carried a negative own-side weight on `pelo_mean`, and H-4 held only while
FEAT-01's broken involvement denominator inflated every upgrade's rate terms. `display` —
monotone-constrained gradient boosting on XI + team-context columns; the probability shown.
Neither model reads `t1_pelo_std` / `t2_pelo_std`, the spread of player Elo across an
eleven: it is the only column an upgrade moves whose direction the contract cannot declare,
B-7 dropped it from the display model (a tree's step response to it made one swap in twenty
lower the displayed probability) and FEAT-14 from the objective (with every other stem
sign-bound, it was where every remaining violation came from), each at a measured cost
recorded below and in `contract._SIDE_STEMS`.

**Run:** `make retrain CUTOFF=2025-09-01` reads the database (`POSTGRES_*`); with
`CRICSHEET_DIR=data/go-app/cricsheet` it reads the raw Cricsheet JSON instead (same format
taxonomy as `format.go`, ~2 minutes for the full archive). Writes `xi_win_<FMT>.joblib`,
`xi_ratings.joblib` and `xi_win_report.json` (AUC and Brier for both models, each fitted once,
base-rate Brier, and the best single column's AUC — a model that cannot beat its own best
column is not being measured).

**One fit, no seed spread (EVAL-02).** The display model is fitted once, under a fixed
`DISPLAY_RANDOM_STATE`. `HistGradientBoostingClassifier`'s `random_state` reaches only the
early-stopping validation split (now off explicitly — EVAL-01, § Hyperparameters) and the
binning subsample above 200,000 rows, so the three "seeds" the pipeline used to fit were
bit-identical in every format but T20 and, in T20 while sklearn still switched early stopping
on by itself above 10,000 rows, differed only by the luck of that split. The `display_auc_sd` / `display_auc_seed_sd` the report carried was therefore exactly
zero, and every clause that read a change against "the control's seed-to-seed sd" had a floor
that never bound. Nothing in the report now names a seed for the display model; the noise a
difference is read against is the fold-level paired standard error (H-14), and for a single
holdout the Hanley–McNeil standard error `run_usability.py` computes. `display_auc_mean` and
`display_brier_mean` keep their names — they are the wire keys the report, the run manifest and
the Workbench share — and are the one fit's own score. The performance model is one fit too
since EVAL-09 took its early-stopping split away (§ Player performance, *Fitting*): its
`random_state` reaches nothing below 200,000 rows and the binning subsample alone above it,
measured as bit-identical refits in T20I, ODI and TEST. `POST /admin/reload` picks the
artifacts up; `GET /xi/status` shows what is loaded.

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
(I-4), read by the go-app importer and by the Cricsheet-JSON source, so both agree. The
importer writes the links after the files, whether or not every file landed, because an
aborted run does not undo what already committed; `/ops/status` reports the coverage under
`db.team_lineage`, where `incomplete` means both rows of a rename are in the archive and the
link between them was never written (IMPORT-07). The
wicket kinds are the same shape: `configs/wicket_kinds.json` says which kinds are the
bowler's, which are wickets nobody took and which are not wickets at all, and the importer's
`bowling_data.wickets` / `wickets_lost` and the pass's `wickets` / `dismissals` targets read
it on every source (IMPORT-06; `docs/config-and-data.md` § the wicket record). What the
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

**The scheduled cadence never says it.** `--accept-data-quality` is a judgement — "I have
looked at the new counts and they are right" — and nobody is present to make it when
`make cadence` runs at 06:00 on a Monday, so neither `/admin/train/retrain` nor the
`refresh` run plan passes the flag. A gate failure there fails the retrain, the plan stops
before `reload`, and the previous run goes on serving until a human looks. That is the
whole non-interactive contract: the automated path can refuse to publish, and cannot
approve.

Current baseline on the full dataset: 22,734 matches offered and 22,734 read, 1,710
undecided, 0 namesake sides, 1,358 sides of more than eleven (concussion and injury
replacements, which Cricsheet lists in full), 0 unresolved player keys, 13,569 players.
Since FEAT-02 `oversized_squads` counts the sides still over eleven once their
replacements are taken out — 24 on the archive (23 with no replacement entry, one whose
entry names a player the other side lists) — and the pass also counts
`replacement_players` (1,362 on the archive), compared across sources rather than gated: it reads zero on a
database imported before migration `0020`, and the doubling rule is what would notice a
source that stopped seeing its replacements.

Beside `undecided_matches` the pass counts `drawn_or_tied_matches` (FEAT-04): of the
undecided, the draws and the ties no tie-breaker settled — the matches form reads as half
a win for each side (`MatchRecord.drawn_or_tied`, the rule `RatingState.update` applies to
`team_results`). It is the only count that can see `match.result`, and it is compared
across sources rather than gated by the doubling rule: it is a fact about the cricket, and
it goes from zero to roughly a quarter of all Tests the first time a database imported
before migration `0017` is re-imported.

It also counts `decided_matches_without_deliveries` (FEAT-03): matches with a recorded
winner that the source handed over with no deliveries at all — a forfeit, a result awarded
without play, or an importer that kept the match's result and squads and lost its ball
events. Such a match used to yield twenty-two player rows labelled `runs=0`,
`balls_faced=0`, `wickets=0` and a win row with `innings1_runs = 0` for E2 to score
simulated totals against; the label is not "he scored nothing" but "nobody observed what
he did". The pass now keeps the win row — its label is real — with the innings outcomes
unobserved (`NaN`, which `simulator.complete_first_innings` never counts as complete), and
builds no player rows. Zero on the current dataset from either source (every one of the
22,905 archive files carries deliveries, and every match in the database has ball events),
so it is **gated** by the doubling rule for the same reason as `unknown_player_keys`: one
appearing is the importer losing ball events, not the cricket changing.

The pass also counts `runs_not_charged_to_bowler` (FEAT-08): the byes, leg-byes and penalty
runs over every delivery it read — the part of `runs_total` that `Deliveries.runs_bowler`
leaves off the bowler. Both sources derive `runs_bowler` through one rule,
`sources.runs_conceded_by_bowler` (total less byes, leg-byes and penalty), which is the rule
the importer applies to `bowling_data.runs` (`cricsheet.Delivery.RunsConcededByBowler`,
IMPORT-04); the Postgres source reads the three `extras_*` columns migration `0018` added,
the archive source reads the delivery's `extras` object. The bowler's runs-saved ledger
(`bowl_rate`, its phase and debut splits, and the sequence families' `bowl_saved`) and the
`runs_conceded` target charge him `runs_bowler`; the over's expectation, the simulator's
extras rate and the innings outcomes stay on `runs_total`, because those describe the
innings. Like the draw count it is a fact about the cricket, compared across sources and not
gated: a database migrated to `0018` but not yet re-imported holds zeros in the three
columns, charges every bowler everything, and reads 0 here against the archive's total.

And it counts `deliveries_not_faced` (IMPORT-05): the wides over every delivery it read —
the deliveries no batter faced. Both sources derive `Deliveries.faced` through one rule,
`sources.faced_by_batter` (every delivery but a wide: a no-ball is faced, a wide is not),
which is the rule the importer applies to `batting_data.balls`
(`cricsheet.Delivery.FacedByBatter`); the Postgres source reads `extras_wides`, the archive
source the delivery's `extras` object. The `balls_faced` target is the sum of `faced` per
batter. Every other ball count in the pass is still a count of deliveries, wides included:
the as-of `exp_balls_faced` and `exp_balls_bowled`, the `balls_bowled` target, the over's
expectation, the simulator's innings length and its extras rate — those describe how long
the innings ran, and a wide takes a delivery without taking a ball from anyone. Compared
across sources and not gated, for the same reason as the runs count: it reads 0 on a
database migrated to `0018` but not yet re-imported.

### Source parity (`make xi-parity`)

**Glossary keys** (L-1, `ml/xi/glossary.py`): `max_abs_difference`.

The two rating sources are supposed to describe the same cricket, and four times they did
not — a hashed match id that lost 309 matches, an unnamed substitute fielder folded into a
fictional player, a namesake rule implemented on one side only, and a match result the
Postgres source hard-coded to `None` while the archive path read it (FEAT-04), so a drawn
Test moved both sides' form on one source and neither's on the other. Each was a one-line
difference in a count that nobody was printing — and the last one was a difference no
count could see, because a draw is undecided whether or not its result is read. The
comparison covers every `DataQuality` count that is a property of the cricket rather than
of the store (`ml/xi/parity.py`, `_COMPARED_COUNTS`), the number of training rows and the
player-key sets; `drawn_or_tied_matches` is in that set, which is what makes the result
column part of the guarantee, and `runs_not_charged_to_bowler` (FEAT-08) and
`deliveries_not_faced` (IMPORT-05) are in it for the same reason: a source that charges the
bowler byes and leg-byes, or counts a wide as a ball faced, agrees with one that does not
on every other count. `decided_matches_without_deliveries` (FEAT-03) is in it because the
two sources can genuinely differ here: the archive path drops a file with no innings as
unusable, while the database offers a match with an innings row and no `ball_event` rows,
and a database that lost one match's ball events agrees with the archive on every other
count — the win row is built either way.

```bash
make xi-parity                                    # defaults to data/go-app/cricsheet
make xi-parity XI_PARITY_DIR=path/to/cricsheet
```

It runs the rating pass over both sources, prints their counts side by side and exits
non-zero if any count or the player-key sets differ. It needs the archive as well as the
database, which is why it is a separate command rather than part of a retrain. Run it after
changing the importer or either source.

### Serving parity (`make serving-parity`)

The other direction of the same question: does the run the service serves compute what a
fresh pass over its own data says? `python -m ml.xi.asof` loads the run `current` names (or
`RUN=<run_id>`) through `XiStore.load` — so a run this code cannot serve is refused by name,
D-6 — runs the rating pass, and holds what the store serves for the last 50 matches to the
harness's tolerance: the rows, the display and objective probabilities as a backtest is
answered (the store over the as-of state) and as a live request is answered (the loaded
through-today state, against the same models over the freshly folded state), the performance
predictions and the simulator's draws. It refuses (exit 2) a run whose `dataset_sha` is not
the source's, because a through-today comparison against other cricket would measure the
data and not the artifact; it exits 1 on a difference. It costs a rating pass plus the as-of
sweep — about eight minutes on the full database — and is the command to run after a reload
that should have changed nothing.

```bash
make serving-parity                 # the run `current` names, against the database
make serving-parity RUN=<run_id>    # a named run under the artifacts root
```

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
(balls faced — every delivery but a wide, IMPORT-05 — runs, fours, sixes, dismissals,
actual batting position, balls bowled, wickets, runs conceded — the runs charged to the
bowler, byes, leg-byes and penalties left to the innings, FEAT-08). Rows cover **all XI players**, never only those who batted: who got to bat
is decided by the result, and a population selected by the outcome is a leak (H-20). The
one exception is a decided match with no deliveries at all (FEAT-03): its win row is built,
because the label is real, but it has no player rows — a nought nobody observed is not a
label — and its innings outcomes read `NaN`, so E2 never scores a simulated total against
it. `ml/xi/rows.py` assembles the rows for both the training pass and the parity check, so the
two cannot spell a column differently; the frame is never written to disk as a contract —
`retrain` and the harness both consume it in memory. Since P-3 the rows also carry `catches`
(each fielder named on a caught dismissal) and the sequence families
(`contract.SEQUENCE_FAMILIES`: dot streaks, reactions, spells — the ball-by-ball patterns the
deleted `seqcalc` tables precomputed, re-expressed as as-of accumulators with per-ball flags
from `ml/xi/sequence.py`). The columns are always in the frame so the question can be re-asked
without a new pass, but **E1 kept none of them**: `contract.SEQUENCE_FAMILIES_KEPT` is empty
and the performance model reads no sequence column. Since A-1 every row also carries the
**fixture context** (`contract.FIXTURE_CONTEXT_COLS`): the ground's and the competition's
as-of scoring level — runs and dismissals per delivery over every ball played there (or in
it) before the match, shrunk toward the format's rate over
`contract.FIXTURE_CONTEXT_PRIOR_BALLS` deliveries and expressed relative to it, so 1.0 is
an average ground and a ground or competition with no history reads exactly 1.0. Venue is
keyed as `venue_bf_rate` keys it; competition is Cricsheet's event name (`match.event_name`
in the database), and an unnamed key is no key. `RatingState.fixture_context` reads them
before `update` folds the match in, at day close (H-18), and the H-8 parity check compares
them on the win row and on every player row. The same rule as the sequence families
applies: the columns are always in the frame, and the performance model reads a family
only if gate A-1 kept it — and **A-1 kept none** (`contract.FIXTURE_CONTEXT_FAMILIES_KEPT`
is empty; plan §8.9): on the walk-forward folds neither the ground nor the competition
moved the simulator's per-quarter bias in T20, and in ODI by 0.2–0.4 runs on a mean of 14,
inside the fold noise. The quarters with the large biases turned out to be population-mix
quarters (associate men's and women's ODIs against one baseline), not venue or competition
effects. Since X-1b every player row also carries the player's **age at the match date**
(`contract.AGE_COLS`: `age` in years and `age_known`), computed by `rows.player_feature_rows`
from the dates of birth the source supplies (`player_biography.birth_date` from Postgres; the
CSV `make export-birth-dates` writes for the archive path, `ml/xi/biography.py`) — a static,
knowable fact read at a date (H-21). A player without a date of birth reads `0.0 / 0.0`:
his own category, never an imputed age. The same rule again: the columns are always on the
rows, and the performance model reads them only if gate X-1b's age family kept them — and
**it kept none** (`contract.AGE_FEATURES_KEPT` is False; plan §8.12): on the walk-forward
folds the pinball loss of runs and wickets moved by 0.02 % on the deciding slices (at most
0.2 % anywhere), inside E1's 0.5 % band, with coverage unchanged. The rating state also
holds an **age-band debut prior** (`RatingState.debut_bat` / `debut_bowl`: per format and
age band, what earlier debutants of that band did in their debut match), which
`side_vectors` applies to a player with no history in the format and a known age only when
`contract.AGE_AWARE_COLD_START` is on — and gate X-1b-cold-start left it **off** (plan §8.12):
H-10 stayed bounded, but the debut rows' pinball worsened in every format (runs −0.3 … −2.7 %),
because the model already learns its own debutant neutral jointly with the rest of the row.

Since X-3 every **win row** also carries the match's **stakes** (`contract.STAKES_COLS`:
`stakes_knockout` and `stakes_stage_known`), derived by `ml/xi/stakes.py` from Cricsheet's
`info.event` — the round (`event.stage`), the pool (`event.group`), the fixture number and
the shape of the competition's edition (`match.event_stage` / `match.event_group` in the
database, added by migration `0011`). Both sources produce them, `make xi-parity` compares
four counts of them, and H-8 compares the columns themselves; a record built for the serving
path carries no stakes and reads `0.0 / 0.0`, the unlabelled category rather than an implied
league game. The same rule once more: the columns are always on the win row, and the display
model reads them only if gate X-3 kept them — and **it kept none**
(`contract.STAKES_FEATURES_KEPT` is False; [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md)
§ X-3): on the walk-forward folds a knockout flag moved display AUC by +0.0000 in T20, ODI
and TEST and +0.0021 in T20I alone. The **dead-rubber flag** the same derivation produces is
deliberately *not* a frame column: it needs the edition's fixture list and its qualifying
cut, which makes it fit to clean a measurement (X-3's E5 hygiene run) and unfit to be a
feature (H-21).

### Performance model (L2-B, `ml/xi/performance.py`, P-3)

**Glossary keys** (L-1, `ml/xi/glossary.py`): `within_match_spearman`, `within_match_spearman_involved`, `top3_hit_rate`, `mae`, `pinball`, `pinball_by_level`, `coverage_80`, `coverage_80_strict`, `width_80`, `q10`, `q90`, `probabilities`, `reliability`, `vs_career_mean`, `vs_career_quantiles`.

**What it answers.** For two elevens, per player, *distributions* — never points — of runs,
balls faced and runs conceded (quantiles 0.1 / 0.5 / 0.9), wickets and catches (a Poisson
rate → P(0), P(1), P(2+)), and P(bats) / P(bowls). The point shown anywhere is the median;
the deliverable is a calibrated range and a ranking, because one innings is mostly noise
(plan §1: within-match Spearman ≈ 0.3 for any predictor on the players who batted).

**Population (H-20).** Every XI player of every decided match that has deliveries, with
"did not bat" as 0 runs from 0 balls and "did not bowl" as 0 wickets; a decided match with
no deliveries contributes no rows (FEAT-03), since nobody's nought was observed there. The
old batting model trained on "who batted",
which the result decides, and its headline MAE was pooled over five targets (S-3c); neither
survives here. Two structures per target are available and were chosen on the walk-forward
folds by pinball loss (plan P-3): `direct` — one gradient-boosting model per quantile (or a
Poisson model) on the unconditional rows; `two_part` — P(involved) from a classifier on the
same rows, times the distribution given involvement fitted on the rows where it happened,
with the involvement *predicted* and the served quantiles those of the mixture, so both
structures are scored on one population with one loss.

**Inputs.** The row's as-of vectors and expected role, the sequence families E1 kept (none —
`SEQUENCE_FAMILIES_KEPT` is empty), both sides' aggregates, venue context, the Elo edge, the
fixture-context families gate A-1 kept (none — `FIXTURE_CONTEXT_FAMILIES_KEPT` is empty; the
ground's and the competition's as-of scoring level, above), the age columns gate X-1b kept
(none — `AGE_FEATURES_KEPT` is False; the age at the match date, above), and
the innings (bat first / chase). The
innings is the toss, not the result: at prediction it is **marginalised** — predicted under
both and averaged — unless the caller passes `team1_bats_first`, the same knob
`/xi/predict-win` has. Nothing the model reads is a function of the match's own result
(`contract.performance_feature_cols` excludes every target column; a unit test asserts it).

**Fitting.** `HistGradientBoostingRegressor` with quantile and Poisson losses and a
classifier for involvement; a three-point grid (`performance.HYPERPARAMETER_GRID`) tuned
inside the walk-forward folds only, where it turned out flat (§ P-3 of the plan); one fit per
booster, for an iteration count **chosen on a temporal fold, then refitted on every row**
(EVAL-09). sklearn's own early stopping drew a shuffled tenth of the rows as its validation
set, and the eleven rows of one side share 44 of the model's 62 inputs (`own_*`, `opp_*`,
`venue_*`, `elo_edge`, the innings), so ten match-mates of nearly every validation row were
training rows and the stopping point was optimistic — measured on the archive at
`f63bf83e`, the shuffled split let `p_bats` run to 236 of 300 iterations in TEST against an
out-of-sample optimum at 5, and read `wickets` as still improving at 216 in T20 against 144.
Now `performance.choose_iterations` fits each booster to `MAX_ITER` on the rows before the
most recent tenth by date (`ITERATION_CHOICE_FRACTION`, cut at a match boundary so no
match's rows land on both sides) and takes the iteration at which the booster's own loss —
log loss, the pinball loss at its level, the Poisson deviance — on the rows after the cut
is lowest; the served booster is a fresh fit on every row for exactly that count with
`early_stopping=False`. The count is re-derived by every fit, so a retrain re-chooses it on
the new rows and each harness window on its own, and the fit's metadata records the cut,
the rows on each side and the count per booster (`fit.iteration_choice`) beside the counts
the served boosters ran (`fit.iterations`). A history too short to cut runs the ceiling and
says so. Independently fitted quantiles can cross; they are sorted. When the harness finds a
quantile target's coverage off nominal it is **recalibrated on a temporal fold** (H-5,
`ml/xi/perf_calibration.py`): members fitted on the rows before the last 92 days
(`performance.CALIBRATION_DAYS`) predict those days, which they did not train on (H-21), and
each level is mapped by an isotonic binned correction fitted on that residual.
`performance.RECALIBRATED_TARGETS` records the decision.

A fold too thin to carry that correction gets none. Below `perf_calibration.MIN_ROWS` a binned
empirical quantile is ten outcomes a bin, so the fit is refused and the target keeps its
uncorrected quantiles — and because that is a substitution nobody could otherwise see (plan
§ 8.7), it is named rather than logged: the run report carries `locked.recalibration_requested`
beside `locked.recalibrated_targets` and `locked.recalibration_skipped`, the fit's metadata
carries `recalibrated` and `recalibration_skipped`, and `POST /performance/predict` answers with
`recalibrated_targets` read off the model that served it. Before EVAL-08 the thin fold raised
instead, which aborted the whole two-hour `evaluate` run.

**Calibrate on the fold, refit on the full history (EVAL-03).** The fold members exist only
to produce out-of-sample residuals — for the recalibration and for the simulator's shared
match factor (below) — and are discarded; the members served are a second fit on every row,
so the served quantiles read the same recent history as the ratings beside them instead of
ending 92 days short of it. The run report records `train_to` (the served members' last row)
beside `calibration_from` (the fold's first). The fold members differ from the served ones by
the fold's rows alone (2–5 % of a format's history), and plan § 8.3 measured the two kinds
of members side by side on the walk-forward folds — the same first-innings coverage, width
and dispersion ratio to within 0.01 — which is what licenses reading the fold's residuals as
the served members'. The cost is a second member fit per format, roughly doubling the
performance fit (≈ 6 minutes over the four formats in the last run).

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
against each P(bats) sets how deep the innings goes (at least two bat), and the D players who
bat are the D most likely to — not the first D slots — so each realises exactly his own
P(bats) and they bat in slot order. The two orders are not the same thing: `exp_bat_position`
is the decayed mean of the positions a player batted at, shrunk toward 7, so a rarely-batting
player sits at the prior; the classifier's P(bats) is the marginal over the innings' length
and his position, and it is not monotone down the slot order in 94 % of real sides (SERVE-05,
measured on the served run over 166 sides). Taking the first D slots made the player at
slot j realise the side's j-th largest P(bats) rather than his own — mean |gap| 0.021, 5 % of
players off by more than 0.10, one by 0.50. Each batter then draws
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
On the wire, `/simulate` reports each player's `p_bats` / `p_bowls` as the forecasts the draws
were made from — the same numbers `/performance/predict` returns — and the share of draws that
realised them as `batted_share` / `bowled_share`. The two are different numbers and are served
as two: on the served run the batted share runs 0.14 below P(bats) on average (0.2–0.3 for
slots 7–11), because the deliveries budget and the chase end the innings before the forecast's
depth — a classifier trained on real innings has already priced both, so the simulator applies
them twice (B-20, recorded in `docs/BUG_BACKLOG.md`; not SERVE-05).

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
first-innings totals on the last 92 days before the cutoff, the temporal calibration fold,
predicted by members that did not train on it (H-21; the members served are then refitted on
every row — *Fitting* above), deconvolved of the simulator's own dispersion — never a
hand-set CV. `simulator.SHARED_FACTOR` records the decision; §8.3 of the plan the before/after.
The pool is **one per format**, and that is a known limitation, not an assumption: the same
interval is too narrow for a day game and too wide for a night one (T20 first-innings
coverage 0.734 / 0.841 at nominal 0.80, dispersion 1.099 / 0.865 — `docs/BUG_BACKLOG.md`
B-11), and conditioning the pool on the pre-match day/night label was gated and **is a
recorded null** in both of its arms, because one factor serves both innings and at night
they want opposite corrections (plan §8.13). Giving the chase a dispersion term of its own so
that conditioning the factor no longer drags it — the answer §8.13's conclusion pointed at —
was gated too and is **also a recorded null** (plan §8.14, *Chase dispersion* below): it
removes the opposite-corrections problem and is stopped by a night population the folds are
too thin to decide and by E2. A fold whose calibration window holds fewer
than 30 complete first innings fits no factor at all, and its intervals are the much
narrower un-widened ones: the walk-forward summary counts those folds, names their windows
and repeats the totals over the folds that did have a factor
(`simulation.shared_factor_folds`, shown on the Evaluation tab), so the pooled figure and
the calibrated-only one are both published rather than the first standing for the second
— B-12.

**Chase response (A-2, plan §8.10).** The chase is drawn as a first innings is and truncated
at the target, so the untruncated draw does not know the target — and the data's chases do:
a side chasing well above its expected score collapses more often than its ordinary
distribution says, and one chasing well under it gets there more surely. The candidate is a
response of the chasing side's runs draws to the target's **difficulty** — the target over
the side's expected total on the draw's pitch, `r = T / (φ · m₀)` with `φ` the draw's shared
factor and `m₀` the mean of the side's own untruncated chase draws — multiplying every runs
draw by `exp(level + slope · ln r)` before the truncation, the same lever the shared factor
uses. The two coefficients are **fitted, never set**: censored (Tobit) maximum likelihood on
the shared factor's calibration fold, reading each calibration match's actual target and
chase, whether the chaser won (in which case the untruncated innings reached the target and
the observation is censored), the chasing side's expected total from the same no-factor
simulation, and the shared factor's own per-match value so the pitch is taken out of the
difficulty at fit time as the sampled factor takes it out at draw time. A fold too thin for a
shared factor fits no response either. `simulator.CHASE_RESPONSE` names the arm that ships
(`none`, `level`, `slope` or `both`; gate A-2 decided it on the folds with a level-only control
arm and one fold-level standard error as the effect-size floor); `FitSpec.chase_response`
carries it into the run manifest and the artifact carries the fitted response beside the
shared factor. **A-2's verdict was a recorded null and the arm is `none`** (plan §8.10): the
slope is negative in every T20 and ODI fold and fixes the chase's level (bias −6.6 / −9.3 →
within ±3) and thins the low tail (below-q10 0.19 → 0.15), but the mass moves above the 90th
percentile — hard chases that were nevertheless won — so 10–90 coverage does not move and E2
degrades; the fitted residual scale is nearly twice the simulated chase's spread, which names
the miss as the chase's *dispersion* (collapse or get there), the next candidate's target. Not modelled by it, and said so: the wickets a collapse loses (the runs fall,
the depth does not), the overshoot of a won chase, DLS, per-ball required-rate dynamics.

**Chase dispersion — a term the two innings do not share (B-11, plan §8.14).** The next
candidate A-2 named, and the one §8.13's conclusion pointed at from the other side. The shared
match factor keeps its meaning — a pitch is common to both innings — and the *chasing* side's
runs draws are multiplied by a second factor drawn independently per draw with **mean one**,
so it adds spread and no level (A-2 gated the level and nulled it, and a term that smuggled
one in would make the gate unreadable). Its log spread is **fitted, never set**: the excess of
the chase's residual scale about its expected total on the factor's own pitch over what the
draws themselves produce on the same calibration matches, deconvolved the way
`fit_shared_factor` deconvolves the first innings, with A-2's censored (Tobit) estimator doing
the fitting because roughly half the calibration chases are won and so right-censored at the
target — a pool of the lost chases alone would be selected on the residual it measures.
`simulator.CHASE_DISPERSION` records which arm ships and `FitSpec.chase_dispersion` carries it
into the run manifest; the artifact carries the fitted term beside the shared factor.
**The verdict is a recorded null and the term is off** (plan §8.14, gates `SIM-IN-chase` and
`SIM-IN-both`, decided on T20's eleven folds — ODI and T20I could not decide it and a
feasibility probe said so before the arms ran). It does what it was designed to do: the
chase's tails go from 0.191 below the 10th percentile against 0.096 above it to **0.084 /
0.097**, both at nominal, in one step; the day chase's coverage 0.689 → 0.811 and the first
innings, composed with §8.13's per-population factor, 0.734 → 0.760 by day and 0.841 → 0.748
at night with the pooled width *falling* 84.7 → 81.6 — so the chase no longer pays for the
first innings, which is how §8.13's two candidates failed. It is stopped by the night
population, which eleven folds of 139 matches cannot resolve, and by **E2**: an independent
chase term widens the *difference* between the innings, which is what decides the match, so
the simulated P(win) moves toward 0.5 and Brier(simulated) − Brier(display) grows by three
standard errors (still inside its 0.01 tolerance). Interval calibration bought with
probability calibration is the cost a shared factor does not have, and the next candidate is
a chase dispersion that does not decorrelate the two innings.

**Chase dispersion, correlated with the first innings (B-11, plan §8.15).** That next
candidate, gated. The same widening, drawn so that it **moves with the first innings' realised
log residual** in the same draw — `exp(slope · (ln T1 − the draws' mean ln T1) +
independent_log_sd · z)`, centred to mean one per fixture, read from `target − 1` inside
`batting_innings`, which is the first innings' own total and is settled before the chase
begins, so it is not future information — and fitted against the chase's **own** expectation
rather than through the shared factor's shrunk per-match value. Both coefficients are the
difference between a censored regression of the calibration fold's chase residual on its first
innings' residual and the same two quantities under the control, composed from the shared
factor's log variance and each innings' own draw spread
(`simulator.CorrelatedChaseDispersion` / `fit_correlated_chase_dispersion`).
`CHASE_DISPERSION` is now an arm name — `"none"`, `"independent"`, `"correlated"` — as the
chase response already was. **The verdict is a fourth recorded null and the term is off**
(gates `SIM-IN-corr` and `SIM-IN-corrboth`, decided on T20's eleven folds with ODI reported).
It fails the very clause it was built to pass: E2 moved **further** the wrong way than the
independent term did, +0.0056 ± 0.0013 against +0.0039 ± 0.0013, paired on the same folds and
the same draws. The reason is a sign, and it is in one column of the fold table — the fitted
chase-on-first-innings slope is **negative in 11 of 11 T20 folds and 10 of 10 ODI ones** while
the control's own is positive in every one, because a regression on the first innings' residual
cannot separate the pitch (positive, and already carried by the shared factor) from the chasing
side's reaction to the target it sets (negative — A-2's response, gated and nulled). The
simulator says the same thing in its tails: the chase's mass below the simulated 10th
percentile against above its 90th reads 0.191 / 0.096 in the control, 0.084 / 0.097 under the
independent term and **0.100 / 0.179** under the correlated one, which is A-2's recorded tail
flip reproduced from a different lever. What §8.15 leaves for the next candidate: correlate
with an estimate of the *pitch* that is not the first innings' own residual, or fit the
*margin* — whose coverage, 0.50–0.69 at nominal 0.80, is the weakest number the harness reports
and the one E2 depends on.

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

**Glossary keys** (L-1, `ml/xi/glossary.py`): every key the report emits — `objective_auc`, `display_auc_mean`, `base_rate_brier`, `swap_violation_share`, `display_swap_violation_share`, `specific_vs_typical_delta`, `agreement`, `bar`, `expected_if_exactly_right`, `delta_brier_mean`, `auc`, `test_auc`, `max_abs_difference`, and X-4's `market_auc`, `market_minus_display_auc` and `joined_share` among them.

One command, one JSON report (`xi_evaluate_report.json`): rolling-origin walk-forward over
quarterly cutoffs 2024-01 … 2026-06 for every choice-facing number, and the **locked
window** (matches ≥ 2026-09-02) scored once per release, labeled, never used for a choice.
Against the archive, pass `BIRTH_DATES=<csv>` (from `make export-birth-dates`) so the
rows carry the same ages the database run's do; without it every age reads as unknown and
the run says so.
The window's start date and the date it was last rotated travel in the report
(`locked_start`, `locked_window`) and on every locked figure's label, so a reader can tell
which window a number came from — see *Rotating the locked window* below. The two surfaces
are also told apart where the number is (EVAL-11): every summary over folds is
`{mean, sd, n_folds, gates_consulted}` — the last being how many registered gates have read
the folds, 29 of 30 today — and a holdout figure is a bare value under `locked`, beside a
`holdout` record saying how much of the season has accrued and that no gate consulted it.
Per format it reports, with mean ± spread over cutoffs (and seeds where a model has one):
objective/display AUC and Brier against the base rate; the specific-XI-beyond-typical-XI
delta and swap monotonicity (the selection gates that replace P-0's winner accuracy), and
the same swap probe against the **display** surface — reported, never a gate, because H-4's
2 % line is a contract on the objective the optimiser reads (see *The display surface's swap
share* below); the
best-single-column leak canary with the TEST-format control (H-2); and the performance
model on the player-match rows — per target and format, within-match Spearman, top-3 hit,
the median's MAE, pinball loss and the 10–90 interval's coverage beside its width (H-22),
with the career-mean, career-quantile and rating-expectation baselines on the same
population, and quantile targets whose walk-forward coverage is off nominal recalibrated
on a temporal fold for the locked window (H-5); the simulator (E2) — simulated P(win)
against the display model's, totals coverage and width, margins, latency, with E2's display
rule decided on the folds; and the natural experiment for selection (E5, below). It ends
with the train/serve parity check (H-8): the last 50 matches rebuilt from the as-of serving
path and compared with the training frame — rows, the served display and objective
probabilities, performance predictions and simulator draws at a fixed seed alike — and the
run fails if they differ. The store it serves from is not the models in memory: the locked
window's win and performance models and the pass's final state are written as a run
directory and loaded back through `XiStore.load` (`round_trip_store`), so the artifact
contract — the rating payload's arrays, the win artifacts' column lists, the performance
pickles — is what is under test, a run this code cannot serve is refused by name (D-6), and
the numbers compared are the ones the routes answer with (EVAL-10). The served probabilities
are compared twice: as a backtest is answered, from the store over the as-of state, against
the harness's marginalised score of the frame's row; and as a live request is answered, from
the loaded through-today state, against the same models over the state the as-of pass ends
on — which is the only comparison that can see an accumulator the payload does not carry.

**The display surface's swap share (B-7).** H-4 holds the *objective* to under 2 % of
one-player upgrades lowering P(win), and it measures 0.0–0.8 %. The display model — the
number a person actually watches move in the Team Lab — had never been probed at all, and
when it was, it violated at **3–7 %**: 5.1 % T20, 7.1 % T20I, 3.4 % ODI, 6.1 % TEST over
the folds (the database source; the archive frame reads 4.8 / 6.8 / 3.1 / 6.2, the same
difference in ground keying X-3 recorded).
`display_swap_violation_share` carries that figure per format, with the fold-level
counts under `display_swap_monotonicity`, and the glossary states that H-4's line is not
its contract. Nothing selects on the display model, so nothing shipped was wrong; what was
wrong was that the number did not exist. **It now reads 0.0000 in every format**, because
the column that caused it was taken out of the display contract — see the end of this
section for what that cost.

The cause is not the team-context columns the constraint set leaves free, as first
supposed: the probe holds team context at the fixture's values, because a selector cannot
change them, so no constraint on them can move the probe at all — and constraining them
was measured to make the violations *worse* (gate `B-7-display-monotone`, a recorded null;
`DISPLAY_CONTEXT_MONOTONE_KEPT` stays `False`). Every column an upgrade moves that the
contract constrains moves the way it should, on 100 % of upgrades; the one free column it
moves, also on 100 % of upgrades, is `pelo_std`, the spread of player Elo across an eleven,
whose direction is genuinely unknown and so is declared 0. The tree model's step response
to it is the whole of the effect: with `t1_pelo_std` / `t2_pelo_std` removed from the
display columns the share is exactly 0.0000 in all four formats, for −0.0004 (T20), +0.0088
(T20I), −0.0055 (ODI), −0.0047 (TEST) of AUC. That reading is gate `B-7-pelo-spread`, which
informs and ships nothing on its own judgement.

**The trade was taken, and it was not free.** The Team Lab's what-if — swap a player, watch
the probability move — is what the product sells, and a swap that moves it the wrong way
one time in twenty undermines the proposition whatever the AUC says. So the two columns are
out of `DISPLAY_FEATURE_COLS`, the display surface is
monotone under the probe by construction, and **display AUC in ODI and TEST is about half a
point lower for it** (−0.0055 and −0.0047, 1.4 and 0.9 fold-level standard errors: real
losses, not rounding), against −0.0004 in T20 and +0.0088 in T20I. The objective kept the
column at the time — it is linear there, and H-4 then measured under 1 % on it — but that
figure was FEAT-01's broken denominator talking; FEAT-14 took it out of the objective too,
once the other stems were sign-bound and it was the source of every remaining violation,
so the two win models now read the same XI columns. Because the display artifact's feature list moved with it, the store now
refuses a win artifact whose `display_cols` or `objective_cols` are not the contract's
(D-6): an older run is retrained, never reloaded. The decision and its cost are recorded in
`docs/BUG_BACKLOG.md` § B-7.

**The market benchmark (X-4, `ml/xi/market.py`).** Where closing odds have been cached, the
report also carries a `market_benchmark` section: the market's de-vigged probability scored
beside the display model on the matches both cover, per format, per fold and pooled, with
the joined coverage printed beside every number. Three arms, same matches, same fold models
— the market's closing price, the display model as served (marginalised over the toss) and
the display model read at the batting order that happened, which is the market's own
information set. It is registered as gate `X-4` and it **informs**: nothing in the system
changes on its result, and no model anywhere reads odds as a feature (a test asserts the
import graph). The odds files live in `data/market-odds/` — git-ignored, since the source
grants no redistribution right — overridable with `ML_MARKET_ODDS_DIR` or
`make evaluate MARKET_ODDS_DIR=…`; with no files there the section reports zero coverage
rather than disappearing. What is cached today and what it measured is in
[EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-4.

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
is served the rating-ordered eleven**, labelled with that reason, beside TEST's H-17 reason. Follow-up
A-3 (plan §8.11) tried to move that number — two feature families in the objective, phase matchup and
role balance, one at a time on the folds, and a reading of E5 reweighted by the objective's own claimed
\|Δ\| — and recorded a null per family: the one that clears the bar does so by less than half a
standard error and breaks H-4's monotonicity in ODI, so nothing shipped and T20 stays rating-ordered
(0.503 against 0.506 on A-4's rotated folds).

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
  parity check (H-8) serves the most recent fold's models instead, and each format's
  `parity_model_window` (the performance model) and `parity_win_model_window` (the win models)
  name which window fitted what it compared.

- **The holdout is a season, and the folds cannot reach it (EVAL-11).** A line that moves to
  the date of the last decision leaves a window only as old as the time since that decision
  — on 2026-09-20 it held 52 T20, 24 ODI, 9 TEST and 3 T20I matches, days rather than a
  window, while all ~30 gates had been decided on the same eleven folds. So the window is
  now defined as a **season**: `LOCKED_SEASON_DAYS` (365) of matches accruing from the line,
  with `season_end` in the report's `locked_window` block and, per format, a `holdout`
  record beside the locked numbers (`n_matches`, `days_covered`, `season_complete`,
  `gates_consulted: 0`). A release verdict on the holdout is due when the season is
  complete *and unread*; until then the record says so and the folds remain the only
  numbers. Three properties of the code keep it untouched rather than a convention: the
  fold path is handed a frame that ends at the line, so nothing computed in a fold can reach
  a holdout row; `fold_windows()` refuses a cutoff at or past the line, so a rotation cannot
  extend the folds into the holdout and no experiment script (all of which take their
  windows from it) can obtain a window there; and a gate whose `Threshold` would read the
  `locked` node is refused when it is registered (`ml/xi/gates.py`). Rotating before the
  season completes is still the honest response to a window that has been read — but the
  record then shows a season that never completed, rather than a holdout that was.
- **What the holdout can and cannot certify.** Its models are trained on the development
  rows only (the locked window trains before `LOCKED_START` under the same cutoff rule as
  every fold), so a holdout number is the score of the *selection procedure* — the features,
  the model classes, the thresholds and the scoping chosen on the folds — on data none of
  those choices consulted. It does not certify the served run's own weights, which
  `retrain` fits through today (the served run's manifest carries its own holdout report
  at its cutoff); and at one season it is thin where the game is thin — a full season of
  the archive holds roughly 1,700 T20, 390 ODI, 190 T20I and 160 TEST decided matches, so
  it can decide H-17 in T20I (mean 0.766 against 0.65, Hanley–McNeil SE ≈ 0.04), is
  marginal in ODI (0.674 against 0.65, SE ≈ 0.03), and cannot decide E5 anywhere (about 40
  T20I and 160 ODI pairs a year against an agreement-minus-bar of 0.06–0.09 at SE
  0.04–0.08). A holdout that cannot decide a gate is named so, not read as a pass.

The current window was declared on **2026-09-02**, at the P-7 merge date, because every model
choice of the P-0…P-7 migration consulted the previous window (matches ≥ 2025-09-01). That
window is now the last four walk-forward folds (2025-09, 2025-12, 2026-03, 2026-06). Its
season completes on **2027-09-02**.

**Gate registry (H-23, `ml/xi/gates.py`).** Every gate the report prints — H-17's AUC line,
swap monotonicity, specific-vs-typical, E5, E2, the quantile coverage, width beside coverage,
the leak canary, parity, and E3, A-1 and A-2 for the scripts that run them — declares what it *varies*, what
it holds *fixed* and what *decides*, with the path at which the report carries its number. The
report embeds the registry, `gates.check_report` fails the run if a gate is printed without an
entry or an entry has nowhere to be read from, and the Evaluation tab renders the triple beside
each number. An experiment script prints its gate's triple before it runs.

**The clause is evaluated, not only printed (EVAL-04).** Until this, `check_report` verified
that each gate's path existed and nothing read the number: a served format whose walk-forward
AUC had fallen under H-17's line reported `gates.passed: true`. Every standing gate now carries a
`Threshold` beside its `report_path` — the clause as code plus the rule a reader can check by
hand, which the report embeds beside the triple — and `check_report` evaluates it, naming the
format and the value when it fails; `make evaluate` exits 1 on any failure. The clauses encode
what the prose already says: H-17, E5 and specific-vs-typical decide *scoping*, and the serving
policy is set by hand from the report (`optimizer.OPTIMISED_SELECTION_FORMATS`), so their
enforceable form is the contrapositive — a format **served** an optimised selection must carry
the evidence (mean objective AUC ≥ 0.65, E5 at or above its derived bar, the specific eleven
adding more than zero over the typical one) and fails the run when it does not, while a format
already scoped off is what the rule asks for; E2 is the same shape for the simulated headline
(served only within tolerance); H-4 is unconditional, under 2 % in every format the folds
scored; H-8 is `passed`. H-5's clause decides a recalibration the harness already applies where the
fold can carry one — and names the targets it could not, rather than refusing the run — H-22
compares against the previous release, and H-2 and X-4 inform, so they carry no threshold and
say so; a gate a script runs is the script's to evaluate. On the batch-1 report every standing
gate passes under these clauses (T20I and ODI served at 0.761 / 0.670 with E5 passing; swap
share 0.0–0.8 %; TEST at 0.626 is scoped off, not failed). No clause anywhere is read against
a seed-to-seed spread: the display model is one fit per window and the spread the harness used
to report across seeds was identically zero (EVAL-02). The three experiment gates whose clause
named it — X-3-stakes, B-7-display-monotone and the X-2 families — say so in place in the
registry; each was decided, in practice, on the fold-level standard error of the paired
difference, which is the floor every gate reads (H-14).

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

### The track record beside the harness

The Track record tab (P2-4, `GET /api/track-record`) scores every prediction the Lab issued
against the match once it is imported — the same Brier, reliability bins and inclusive 10–90
coverage the harness computes, applied continuously to the served run on new data as it
arrives, misses included. It is worth saying what each is evidence of, because they look
alike and are not:

- **The harness** (`make evaluate`) scores thousands of matches over walk-forward folds and a
  locked window, with the model refitted per fold, the folds' spread reported, and every gate's
  varies / fixed / decides triple recorded (H-23). It is where a choice-facing number comes
  from, and the only place one comes from.
- **The record** scores the tens of predictions one operator issued, from whichever run was
  loaded at the time, against the fixtures that person happened to ask about. Its base rate
  comes from the same rows it scores. No threshold is set on it and nothing on it turns red:
  a Brier over tens is a diagnostic, and a reliability bin holding two predictions is a
  count. A record that disagrees with the harness is a thing to investigate and write into
  `docs/BUG_BACKLOG.md`, not a verdict on the model.

The tab shows the harness's figure for the same format beside each of the record's, labelled
by the window it came from, so a reader sees the record against the number the model was
accepted on. Two of the harness's disciplines carry over unchanged: the factored and
factorless simulator populations are never pooled (B-12; a simulated answer stored before the
store recorded its simulator is a third population, "unknown"), and the ranges are the
simulator's as served, with no day/night adjustment (B-11 is open and the record will show
it). Glossary keys the record adds (L-1): `record_base_rate_brier`, `eleven_overlap`.

### The auction projection, and what it is not (P3-2)

The auction module's projection (`POST /api/auctions/{id}/projection`) reads the two models
above and nothing else: L2-B's per-player quantiles from `/performance/predict`, and the
eleven's total from `/simulate`'s draws. It is worth saying plainly what it is, because it
looks like a prediction and is not one:

- **It is a forecast of no fixture.** The eleven is a guess the operator typed, the
  opposition is a guess they named, and the grounds are a mix nobody has played. There is
  no match to resolve it against, so it is **not stored as a prediction, is not on the
  track record and is never scored** — `issued_prediction` (P2-3) holds answers about
  fixtures, and P2-4 scores them once the match is imported. The auction record holds what
  was entered and what was shown, which is a different thing and says so.
- **It is valuation, never XI-picking.** Nothing in it calls `/xi/optimize`, nothing
  computes a marginal value, and the Go type it maps `/simulate` into carries no
  `win_probability` field at all, so the value never exists in that process. The record is
  that in T20 optimised selection is indistinguishable from rating order (plan §8.8) and
  the IPL is domestic T20; a P(win) beside a purchase would be that claim in another coat.
- **It changes no model and measures nothing.** Two additive serving fields exist for it —
  `venue_context` on `/performance/predict` (the two columns the model reads a ground
  through, off the rows it consumed) and an opt-in `return_total_draws` on `/simulate` (the
  drawn totals themselves, so a caller pooling grounds inverts the pool rather than
  averaging summaries). Neither is a feature, neither enters a fit, and no measured number
  moves.
- **Its two intervals are different populations and are labelled as such.** L2-B's quantile
  heads are at nominal coverage on the harness (§8.2); the simulator's drawn totals are
  B-11's open defect and are shown with B-11 and B-14 named beside them. Nothing is
  widened, narrowed, adjusted for day or night, or hidden.
- **A ground moves it only through the toss.** `venue_bf_rate` and `venue_n` are the whole
  of what the performance model reads about a ground — A-1's fixture-context families were
  gated and nulled, and `FIXTURE_CONTEXT_FAMILIES_KEPT` is empty — so per-ground rows differ
  by what the toss does there and by nothing else, and the answer says so. A ground the
  served state has no matches at reads neutral and is reported neutral (§8.7).

Glossary keys the projection adds (L-1): `auction_projected_output`,
`auction_projected_total`, `interval_source_l2b_quantiles`,
`interval_source_simulator_draws`.

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
