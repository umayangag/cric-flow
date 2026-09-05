# ML pipeline re-architecture: selection first, performance second

**Goal.** Primary: pick the XI from a pool that maximises win probability against a given
opponent and venue. Secondary: predict how each player will perform. Everything in the ML
pipeline should be justified by one of those two, and measured by the metric its consumer
actually needs.

This plan was written separately from two earlier ones, both of which it has since outlived
and which were deleted once the migration completed: the **win-probability selection
checklist** (`docs/WIN_PROB_SELECTION_PR_CHECKLIST.md`, which repaired the existing selection
path and, in S-10, replaced its objective) and the **original optimal-XI plan**
(`docs/ML_PLAN_optimal_xi.md`, the first-principles feature and modelling plan, part of which
S-10 implemented). Both are in git history at those paths. This plan answers a different
question: given what S-9/S-10 showed, what should the pipeline *be*.

---

## 1. Are the models healthy? A verdict with numbers

Measured on the same data (Cricsheet, 22,734 matches) with a 2025-09-01 temporal holdout.
Repo numbers are read from the artifact sidecars in `output/ml-service/`; comparison numbers
come from `scripts/experiments/xi/perf_experiment.py` (as-of features only, no leakage by
construction).

| model | what the sidecar says | what it means |
|---|---|---|
| **win** (`ml.win_features`) | CV accuracy 0.56; held-out AUC 0.56–0.63 (S-3c) | Not usable as a selection objective. **Fixed by S-10:** 0.74 T20 / 0.72 ODI / 0.76 T20I display, 0.73 / 0.69 / 0.75 objective on the Cricsheet-JSON source; trained on Postgres in P-0, 0.751 / 0.726 / 0.721 display and 0.723 / 0.684 / 0.744 objective. As a *displayed probability* this is a clear replacement; as a *selection* objective it did not beat greedy on the P-0 gate (§6, P-0). |
| **batting** (RF, 5 targets) | `mae 5.13, r2 0.371` | **Misleading.** `ml/metrics.py` ravels the five targets — runs, balls, fours, sixes, batting position — into one vector, so "target mean 8.35" is the average of five unrelated scales and R² 0.37 is mostly the model telling runs apart from fours. The per-target figure is `mae_runs 12.85` (T20), `18.61` (ODI). |
| batting, runs only | — | Career mean alone: 13.67 / 18.76. A contextual GBM (as-of ratings + opponent bowling + venue + expected role): **12.06 / 18.01**. The RF is barely ahead of a constant per player, and behind a small model that knows who it is batting against. |
| batting, *ranking* | not measured | Within-match Spearman between predicted and actual runs is **0.31–0.35 for every predictor tried**, career mean included. Top-3-performer hit rate 0.35–0.37 vs 0.33 by chance. One innings is mostly noise; this is the ceiling of the game, not of the model. |
| **bowling** (3 targets) | no metrics recorded at all | The trainer never writes them. Wickets per bowling innings: career mean MAE 0.87 vs 0.80 for a Poisson GBM with context; within-match Spearman 0.12–0.17 (T20), 0.20–0.26 (ODI). Wickets are even less predictable than runs. |
| **fielding** | `mae_catches 0.41` on a mean of 0.44 | Predicting a Poisson(0.44) count; no model can do much. Its outputs carry `field: 0.1` of the selection weight today for no measurable reason. |
| **extras**, **innings** | `extras_meta`, `innings_meta` sidecars only | Exist to reconcile player predictions into innings totals ("hybrid reconciliation") and to feed a Normal-sampling Monte Carlo. Both are patches over the fact that the base models are not a generative model of a match. |
| **combination meta** (Ridge on bat/bowl/field scores) | never tuned, never scored (S-5, S-5b) | Chooses the greedy XI today. With S-10 the objective no longer needs it. |
| **precompute snapshots** (`feature_raw_stats_snapshots`) | 1.81M of 2.32M rows are duplicates, 2,361 groups conflict (D-1) | Every base model reads them. The S-10 rating pass does not, and reproduces better numbers in ~100 s from raw events. |

**Verdict.** The pipeline runs, is well instrumented, and is not accurate enough for either
goal. For selection the objective was the problem and S-10 replaces it. For performance
prediction the models are within a hair of "the player's own average", they are scored on a
metric that hides that, and the two downstream layers built on them (reconciliation,
Monte Carlo) inherit their noise. The honest ceiling for single-innings prediction is low;
the useful outputs are *distributions* and *rankings*, and neither is what the pipeline
produces today.

Two things worth keeping in view: (a) a quantile model gives correctly calibrated 10–90
intervals (coverage 0.80 measured, 0.80 ideal), so distributions are achievable even where
point accuracy is not; (b) the largest single lever for performance prediction is *context*
— expected batting slot, opponent bowling strength, venue — which the current player-level
models do not see.

---

## 2. Design principles for the rebuilt pipeline

1. **One as-of pass, one feature store.** Every feature — for the win model, the
   performance model, the simulator — comes out of the same chronological pass over
   `ball_event`. No snapshot tables, no export CSVs as an intermediate contract, no "latest
   snapshot before the match" query to get wrong. The S-3c and D-1 classes of defect cannot
   recur because the mechanism that produced them is gone.
2. **Every model is scored by its consumer's metric.** Selection objective: held-out AUC
   *and* the specific-XI-beyond-typical-XI test *and* swap monotonicity. Performance model:
   within-match Spearman, top-k hit rate, quantile coverage / pinball loss, per target,
   never pooled. A number the consumer cannot act on is not reported as a headline.
3. **Distributions, not points.** Runs, balls and wickets are predicted as quantiles (or a
   count distribution), the scorecard shows a range, and the match simulator samples from
   them. Point predictions are the median of that distribution, not a separate model.
4. **Selection first.** The optimiser maximises the XI objective directly. Player-level
   outputs explain and display; they never gate the choice through hand-picked weights.
5. **Fewer models, each with a reason.** Two model families (XI win, player performance)
   plus one derived simulator replace six trained models, a meta-model and a
   reconciliation layer.
6. **Run identity for artifacts.** Every training run writes to `output/ml-service/runs/<run_id>/`
   with a manifest (data hash, cutoff, git sha, metrics); `current` is a pointer. D-3 goes away.

---

## 3. Target architecture

```
L0  event store        match, match_player, ball_event          (keep: importer + migrations)
                             │
L1  rating pass        ml.xi.ratings  ── one chronological pass ──►  per-match rows   (win)
    (as-of, structural)                                          ►  per-player-match rows (performance)
                                                                 ►  serving RatingState  (through today, or as of a date for L4)
                             │
L2  models             A. XI win model: objective (additive) + display (monotone GBM)     [S-10, done]
                       B. player performance: quantile / count models conditioned on XI context
                       C. match simulator: sample B for a chosen XI vs opponent XI  (derived, no training)
                             │
L3  selection          optimiser over A  →  XI, P(win), marginal values, role coverage
                       B for the chosen XI →  scorecard with ranges, batting-order suggestion
                       C →  distribution of totals / margins  (replaces Normal Monte Carlo)
                             │
L4  evaluation         one temporal backtest harness: selection metrics + performance metrics + calibration
L5  ops                one `make retrain CUTOFF=` (or ops step) → run dir + manifest; /admin/reload swaps `current`
```

### L1 — extend the rating pass to emit player-match rows

`ml.xi.builder.build` already reads each side's per-player vectors before folding the match
in. Emit them as rows: `(match_id, date, format, player_key, side, vectors…, own-side
aggregates, opponent-side aggregates, venue context)` joined after the pass with what the
player then did (balls, runs, fours, sixes, wickets, runs conceded, dismissal, batting
position). This is the training frame for L2-B, and it is produced by the same code path
the serving store uses — the property S-3c was about, now for the performance model too.

Additions the pass should carry for L2-B (all as-of, all cheap):

- **Expected batting slot**: decayed mean batting position, and share of innings where the
  player batted at all (top-order vs finisher vs tail). The strongest predictor of runs
  after the player's own rate, and a *decision* the selector can later optimise (§5, E3).
- **Phase splits**: impact rates by powerplay / middle / death (bowling and batting).
- **Opponent-specific rates**: vs this opposition (shrunk hard).
- **Sequence features** (`seqcalc`: dot streaks, reactions, spells) *only if E1 shows they
  add to L2-B* — today nothing measures whether they do.

### L2-B — the performance model

Per format, per target, gradient boosting with a distributional loss:

| target | population (as built in P-3) | loss | outputs |
|---|---|---|---|
| runs | **all XI players**, "did not bat" = 0 (H-20) | quantile (0.1, 0.5, 0.9), direct | median + interval |
| balls faced | same | quantile, direct | median + interval |
| wickets | all XI players, "did not bowl" = 0 | Poisson, **two-part** (P(bowls) × Poisson given bowling; §5.3) | P(0), P(1), P(2+), mean |
| runs conceded | same | quantile, direct | median + interval |
| catches | all XI players | Poisson | rate only (never a headline) |

P(bats) and P(bowls) are fitted on the same rows and served beside the distributions. The
original draft of this table conditioned the batting population on expected involvement;
P-3 replaced that with the unconditional population and let the folds choose between a
direct fit and a two-part one per target.

Inputs: the player's vectors, expected batting slot, phase splits, own-side batting
strength (how many balls are left for this slot), opponent bowling strength (phase-wise),
venue par and bat-first bias, innings (bat first / chase) *marginalised at prediction* (the
S-8 fix: predict under both and average, or condition on the toss once known).

Measured by: within-match Spearman, top-k hit rate, per-target MAE of the median, pinball
loss and interval coverage. Reported per target; never pooled.

Expected result, from the experiment: modest MAE gains (≈ 6% over career mean), correctly
calibrated intervals, ranking at the game's ceiling. That is the honest deliverable for the
secondary goal, and it is more useful than a point that is wrong by 12 runs on average.

### L2-C — the simulator

No training. For a fixed (XI, opponent XI, venue), sample each batter's runs and balls and
each bowler's wickets from L2-B's distributions, with a simple innings-length constraint
(balls faced sum to the innings; wickets cap at 10), N times. Outputs: distribution of
totals, margin, P(win)-by-simulation. The display model's P(win) remains the headline; the
simulator's P(win) is reported beside it as a *consistency check* (E2), and the scorecard's
totals are the simulator's medians. This replaces the extras model, the innings model, the
hybrid reconciliation rescaling and the Normal(mean, mean×CV) Monte Carlo, and it makes the
scorecard and the win probability come from one coherent picture of the match instead of
being rescaled toward each other after the fact.

#### The innings sample (P-4 design, written before the code)

An innings has two views that must agree — the batting side's runs and the bowling side's
runs conceded — and one budget: the balls. The sample takes the **batting side as
authoritative** and treats the bowling side's figures as *attributions* of that innings, so
the total is produced once and never rescaled toward a second estimate (the thing the hybrid
reconciliation did). Every number the sample consumes is an as-of feature or an L2-B output
for the fixture (H-21); the only constants are laws of the game — legal balls per innings,
ten wickets, a bowler's fifth of the overs.

*Inputs.* Per player of both elevens, from L2-B at the given `as_of` and orientation: the
0.1 / 0.5 / 0.9 quantiles of runs, balls faced and runs conceded; the wicket distribution
(mean, P(0), P(1), P(2+)); P(bats) and P(bowls); and from the same as-of row
`exp_bat_position` and `exp_balls_bowled`. Per fixture, three as-of rates from the rating
pass, per format (and per context group, like the run baselines): extras per delivery,
deliveries per full first innings (an innings not all out, so it ran its overs), and the
bowler-credited share of dismissals. No trained extras or innings model, no Normal.

*A forecast becomes a distribution.* A player's three quantiles define a quantile function:
piecewise linear through (0, 0), (0.1, q10), (0.5, q50), (0.9, q90), and above 0.9 an
exponential tail with scale (q90 − q50) / ln 5 — the scale an exponential upper half would
have — so the reconstruction reproduces the fitted quantiles exactly and invents nothing
below them. One batter's runs and balls are drawn comonotonically (one uniform for both): a
batter who faces more balls scores more.

*The batting innings, sequentially by expected slot.*

1. Order the XI by `exp_bat_position`.
2. **Depth.** One uniform v per draw; B = #{k : P(bats)_k > v} batters bat (at least two),
   the first B in slot order. Each batter keeps his marginal P(bats) — exactly when P(bats)
   falls with the slot — and the count is coherent: nobody at eight bats while seven sits.
3. **Given that he bats**, batter k draws (runs, balls) from the upper P(bats)_k part of his
   unconditional quantile function (level 1 − p + p·u), never from the zero mass.
4. **Innings length.** C is the as-of deliveries per full innings. Cumulative balls in slot
   order: the batter at which they cross C keeps the remainder at his sampled strike rate
   and later batters do not bat; if B < 11 and the sum falls short of C, the not-out pair
   faces the remainder at their sampled strike rates. All out (B = 11 without truncation)
   ends the innings where the balls do.
5. **Wickets** = batters − 2 (the not-out pair), 10 when all out; never above 10.
6. **Extras** ~ Poisson(as-of extras per delivery × deliveries used).
7. **Total** = Σ runs + extras.

*The chase.* Target = first-innings total + 1. Each batter's contribution is runs plus extras
pro rata to his balls; the innings stops at the batter whose cumulative contribution reaches
the target, truncated to it (balls pro rata); a chase that runs out of balls or batters loses
by the difference; equal is a tie. The chasing side's L2-B forecasts are its chasing
orientation — the innings is a known feature once the toss is — so the model's chasing rates
and the truncation both act. Whether that double-counts is a measurement (folds only): the
sample can be fed the bat-first orientation for the chaser instead, and the choice is a
recorded constant.

*Bowling attribution.* A bowler bowls with P(bowls), topped up in `exp_balls_bowled` order
when too few bowl to deliver the innings under the per-bowler cap (a fifth of the innings);
balls in proportion to `exp_balls_bowled`, excess above the cap redistributed. Runs conceded:
a multinomial split of the innings total with weights balls × as-of rate (median runs
conceded / expected balls). Wickets: the as-of bowler-credited share of the innings' wickets
(the rest are run-outs), split multinomially with weights balls × wicket rate. Bowlers'
figures therefore sum to the innings by construction; their own L2-B medians are not
reproduced and are not meant to be — that would be the second estimate.

*Toss.* Unknown: half the draws under each orientation, each with the matching L2-B
forecasts, so P(win) is marginalised like everything else (H-3); known: every draw under it.

*Outputs, all from the same draws.* Per side the total (median, 10–90, mean); per player the
median and 10–90 of runs, balls, wickets and runs conceded; the **median-band scorecard** —
each player's mean over the draws whose total lies in the central tenth of the total's
distribution — which sums to that band's mean total by construction, so the scorecard and
the innings total shown are one picture; P(win) and P(tie); the margin as cricket states it
(runs when the side batting first wins, balls remaining and wickets in hand when the chaser
does); and each player's contribution to the total's spread, Cov(player, total) / Var(total).

*Shared match factor.* Independent batter draws under the balls constraint may under-disperse
totals: a pitch or a day is shared by both sides. This is measured first (E2, walk-forward
folds): the PIT of actual first-innings totals in the simulated distribution and the ratio of
actual to simulated dispersion. If under-dispersed, one multiplicative factor per draw,
shared by both innings and applied to every batter's runs draw, is sampled from the **as-of
residual distribution**: the ratios actual / simulated-mean total over the last 92 days
before the cutoff — the temporal calibration fold the members do not train on (H-21) —
deconvolved of the simulator's own dispersion (shrunk toward 1 by √(excess / residual
variance)). Never a hand-set CV; decided on the folds, before/after recorded.

*Budget and scope.* N draws (default 2,000) configurable and seeded, vectorised over draws;
under a second per fixture. Limited-overs formats only: TEST has no innings length and stays
on the greedy path (H-17). Not modelled, and said so: the overshoot at the target (a chase
ends exactly at it), rain and DLS, per-ball sequencing, batting-order optimisation (E3, P-7).

### L3 — selection and explanation

Already in S-10: optimiser, marginal values, role coverage. Add: the batting-order
suggestion from L2-B's expected-slot model (E3), and "why this XI" as the top marginal
values and the role-coverage deltas against the greedy alternative. The greedy seed stays
only as the optimiser's starting point.

### L4 — one evaluation harness

Fold `win_discrimination`, `walk_forward`, `selection-comparison` and the player-level
backtest into one temporal harness with one report:

- Selection: objective/display AUC + Brier vs base rate; specific-XI-beyond-typical-XI
  delta; swap monotonicity (share of upgrades that lower p); natural experiment — for
  consecutive matches of the same side with 1–3 lineup changes, does Δobjective agree with
  Δoutcome more often than chance. **Selection-comparison winner accuracy is reported but
  never gates**: P-0 showed why. Both arms' probabilities come from the same win model, so
  the metric asks whether the predicted winner of a *counterfactual* fixture matches the
  result of the real one — and an arm that optimises both sides moves that fixture toward
  parity, losing accuracy whether or not its XIs are better (measured: mean |p − 0.5| 0.153
  optimised vs 0.180 greedy). The harness must also emit **per-match rows**, so the arms can
  be compared with a paired test and filtered by date; today it emits aggregates only and
  neither is possible.
- Performance: per-target Spearman, top-k hit, pinball loss, coverage.
- Simulation (E2, done in P-4): Brier and reliability of simulated P(win) against the display model's; coverage and width of the simulated totals' 10–90 interval against actual innings totals, with the PIT and the dispersion ratio; margins; latency.

### L5 — ops

`make retrain CUTOFF=…` runs L1 → L2 → L4 and writes `runs/<id>/` with `manifest.json`
(dataset sha, cutoff, git sha, rating hyperparameters, all metrics). `current` is a symlink
or pointer file; `/admin/reload` swaps it atomically. The ops pipeline steps become:
import → retrain → reload. Precompute and export steps are deleted.

---

## 4. Keep / change / remove

| component | decision | why |
|---|---|---|
| Cricsheet importer, `match` / `match_player` / `ball_event` | **keep** | The event store is the only input L1 needs. |
| Player identity (planned in `docs/IDENTITY_PR_CHECKLIST.md`, since deleted; in git history) | **keep, do first** | Name-keyed ids merge 348 people into 163 names and men's/women's sides into one team id. Every rating inherits it. The Cricsheet registry id is the fix and the importer already reads it. |
| Precompute (`internal/precompute`, `feature_raw_stats_snapshots`, `player_window_features`) | **remove** | Replaced by L1. Carries D-1. **Done (P-6)**: the package, the two commands and both tables are gone (migration `0008`); the rating pass reads `ball_event` directly. |
| Sequence features (`seqcalc`, `*_features` tables) | **conditional** | Keep only the calculators E1 proves useful, re-implemented inside L1 as as-of accumulators; drop the tables. **Done (P-6)**: E1 kept no family (§5.3), so the calculators went with the nine tables rather than being re-implemented. |
| Export CSVs (`export-dataset`, `exportqueries`) | **remove** | L1 writes the training frames itself. D-2 goes away. **Done (P-6)**: the command, the service, the query package and `configs/feature_vectors.json` are gone, and with them the `training-data` endpoint their last consumers read. |
| `ml.win_features`, `train_win`, `win_discrimination` | **remove after S-6** | Superseded by `ml.xi` + L4. S-6 shipped in P-5 (the XI path is the only selection path). **Done (P-6)**: the model, its trainer, its discrimination report, the feature module and the per-format artifact registry they shared all go, with `/predict/win`, `/predict/win-enhanced`, `/model-metadata` and `/model-stats`. |
| batting / bowling / fielding trainers (RF multi-output) | **replaced** by L2-B | Wrong loss, pooled metrics, no context. **Done (P-5)**: deleted with their shared pipeline, their endpoints and the regression half of the tuning stack. |
| extras, innings models; the rescaling layers | **removed** | Replaced by L2-C. **Done (P-5)**: the solver, its adapter and service, the app-side rescale, the consistency checker and losses, the coherence metrics and the match-projection endpoint all deleted. |
| Monte Carlo (the Normal sampler in `predictteam`) | **replaced** by L2-C | Normal(mean, 0.35·mean) is not a runs distribution. **Done (P-5)**. |
| combination meta-model, the greedy weights, greedy selection | **demoted** to optimiser seed; meta-model deleted (S-5/S-5b cancelled) | Objective is the XI model. **Done (P-5)**: the whole `teamselect` package, the weight and normalisation settings and the meta-model are gone; the greedy seed is `_greedy_seed` inside `ml/xi/optimizer.py`, which is also the rating-ordered pick TEST is served. |
| Auto-tune stack (Optuna two-phase, PyCaret, AutoGluon) | **remove** | Model class is not the constraint (measured twice). A small grid inside the retrain step is enough and runs in minutes. P-5 removed its regression half with the trainers it tuned. **Done (P-6)**: the rest goes with the win model, and Optuna, PyCaret, AutoGluon and SHAP leave `requirements.in` and CI — which collapses the two Docker stages into one image. What replaces it is `ml.xi.train.DISPLAY_GRID`: three points for the display model, scored on a temporal split *inside* the training rows, the incumbent kept unless a candidate beats it by more than 0.002 AUC, and the choice with its evidence written into the run manifest. |
| Per-call hill-climb in go-app | **removed** | Selection runs in ml-service against L2-A. **Done (P-5)**. |
| Backtest API surface (`/api/backtest/*`), frontend tabs | **kept, re-pointed** | **Done (P-5)**: `/api/backtest/report` proxies L4's report from ml-service's new `GET /xi/evaluate-report`, and the Evaluate tab renders it. **P-6 removed `training-data`** with the two consumers it existed for. |
| Ops console, pipeline runner | **keep, shorten** | Three steps instead of six. P-5 removed the six training steps whose commands it deleted. **Done (P-6)**: the registry is import → retrain → reload, with `evaluate` beside them on the same surface but `Optional`; `confirmDefaultParams` has nothing left to confirm and is gone, and the run plans lose `tune` and `data-refresh`. |

Net effect: the ML surface shrinks from six trained models + meta-model + reconciliation
to two model families and one derived simulator, all fed by one pass over one table.

---

## 5. Experiments to run before committing (each ≤ a day, all on the L1 frame)

| id | question | method | decision rule |
|---|---|---|---|
| E1 | Do sequence features add to L2-B? | Ablate `seqcalc` families as extra as-of accumulators in L1; measure Spearman / pinball on the holdout | Keep a family only if it moves pinball loss by > 1% over three seeds; otherwise drop it and its tables — **run in P-3, §5.3: no family moves pinball by more than 0.21 %; none kept** |
| E2 | Is the simulator consistent with the display model? | Simulated P(win) vs display P(win) on holdout matches; calibration of each | If simulated P(win) is worse-calibrated by > 0.01 Brier, keep it as a display-only distribution and never as a probability — **run in P-4, §8.3: simulated − display Brier +0.0028 ± 0.0057 T20, +0.0034 ± 0.0131 ODI on the folds — within tolerance, a probability, served beside the display model's, which stays the headline** |
| E3 | Can batting order be optimised? | Expected-slot model + L2-B; for the chosen XI, evaluate objective / simulated totals under permutations of the top 7 | If reordering moves simulated totals by > 3% for > 30% of XIs, add batting-order suggestion to L3; else leave order to the captain — **run in P-7, §8.8: the best of 64 sampled orders of the top seven, confirmed with a fresh seed, moves the simulated median total by > 3 % for 25.0 % ± 3.1 of T20 elevens and 12.6 % ± 2.3 of ODI elevens (median move 1.5–1.7 %); under the line in both, order stays with the captain** |
| E4 | How much does identity cost? | Re-run S-10 on the Postgres source before and after IDENTITY I-3/I-4 | Report the AUC delta; expect the women's-cricket subset to move most — **run in P-1, §5.1** |
| E5 | Natural experiment for selection | Same side, consecutive matches, 1–3 changes: sign agreement between Δobjective and Δresult | ~~If agreement > 55% on ≥ 300 pairs~~ — **the bar is derived from the objective's own claimed effect size (P-7, §8.8): an exactly-right objective would score about 0.52, so 0.55 was never reachable.** The metric is the *lineup-only* form, in L4 (`ml/xi/natural_experiment.py`): both elevens scored against match k+1's opponent at match k+1's as-of; §5's as-played form is confounded (Δresult is an identity on `won_k`, §8.6) and is not computed. Walk-forward, against the derived bar: **T20I 0.586 (n=111) vs 0.469 — passes; ODI 0.562 (n=429) vs 0.487 — passes; T20 0.490 (n=1,358) vs 0.501 — fails.** On every development pair (in-sample for the weights, well-powered): T20I 0.589 vs 0.496, ODI 0.521 vs 0.504, T20 0.509 vs 0.512 — the same verdicts. Optimised selection is scoped off in T20 on this reading, on beside TEST's H-17 rule. **X-3 re-ran the T20 measurement with the rotation-heavy fixtures filtered out (§8.6) and the null held**: all five pair filters fail their re-derived bars, and four of the five move the agreement below the control (EXTERNAL_DATA_PLAN.md § X-3). The scoping stands |
| E7 | Do gender-split context baselines help? | Split the (format, over) baseline by gender in the rating pass; measure objective AUC overall and on the women's subset | Keep if the women's subset improves by > 0.01 without hurting men's — **run in P-2, §5.2: no effect; the split ships off** |
| E6 | Format transfer for L2-B | Train T20 + T20I jointly with a format indicator vs separately | Keep separate unless joint wins by > 0.01 Spearman (for the win model it lost; the performance model may differ) — **run in P-3, §5.3: joint moves Spearman by at most 0.003; separate stays** |

Already answered by S-9/S-10 (do not re-run): pooling formats for the win model (no),
neural nets (no gain), subset-by-prediction (no; weight by involvement instead), monotone
objective (logistic wins for the argmax).

### 5.1 E4, run — what identity is worth

**Design.** Two Postgres databases built from the same 22,734 Cricsheet files. *Before* is
the pre-P-1 importer: players keyed by name, one `opposition` row per name. *After* is the
P-1 importer: players keyed by the Cricsheet registry id, teams keyed by (name, gender).
Both are then read by the *same* `ml.xi.train` at `--cutoff 2025-09-01`, with the same
seeds `(0, 1, 2)` for the display model; the objective is a logistic regression and has no
seed. Only the identity in the data differs — no model code, no hyperparameter and no
cutoff changes between arms, which is what makes the delta attributable.

Ratings: 13,428 player keys before, 13,568 after. Frame: 20,722 rows before, 20,723 after
(that one row is the `StableMatchID` non-determinism, §10.4, not identity).

Both arms were built before §10.4's fix, so both are missing the same 309 matches. That
does not weaken the comparison — the two arms differ only in identity, which is what E4
asks — but the absolute AUCs are from a database 1.4% smaller than the source. §10.4
records what the repair alone did to the same report.

**Aggregate, held-out (2025-09-01 → 2026-08-25).** Objective is the serving,
toss-marginalised figure — the number the optimiser's argmax is judged on. `± resolvable`
is the smallest difference the holdout can distinguish at 95% (Hanley–McNeil SE on the AUC
difference), which is the column that decides whether any of these deltas mean anything.

| format | n | objective before | after | Δ | display before | after | Δ | ± resolvable |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| T20  | 1577 | 0.723 | 0.719 | −0.004 | 0.737 ± 0.003 | 0.737 ± 0.002 | −0.001 | 0.035 |
| T20I |  178 | 0.742 | 0.741 | −0.000 | 0.693 ± 0.000 | 0.726 ± 0.000 | +0.033 | 0.105 |
| ODI  |  375 | 0.684 | 0.682 | −0.003 | 0.723 ± 0.000 | 0.706 ± 0.000 | −0.017 | 0.076 |
| TEST |  157 | 0.585 | 0.583 | −0.002 | 0.588 ± 0.000 | 0.601 ± 0.000 | +0.013 | 0.125 |

**By gender**, which is where the plan expected the gain, since 20% of the dataset is
women's cricket and it carried both defects — shared team ids and blended careers:

| format | subset | n | objective before | after | Δ | display before | after | Δ | ± resolvable |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| T20  | women | 533 | 0.767 | 0.766 | −0.000 | 0.795 | 0.803 | +0.007 | 0.053 |
| T20  | men  | 1044 | 0.698 | 0.692 | −0.006 | 0.714 | 0.706 | −0.008 | 0.045 |
| T20I | women |  70 | 0.769 | 0.772 | +0.003 | 0.740 | 0.781 | +0.042 | 0.154 |
| T20I | men  |  108 | 0.730 | 0.729 | −0.001 | 0.700 | 0.701 | +0.000 | 0.139 |
| ODI  | women | 141 | 0.776 | 0.773 | −0.003 | 0.808 | 0.813 | +0.005 | 0.104 |
| ODI  | men   | 234 | 0.620 | 0.617 | −0.003 | 0.671 | 0.649 | −0.022 | 0.101 |
| TEST | women |   2 | — | — | — | — | — | — | — |
| TEST | men   | 155 | 0.581 | 0.580 | −0.001 | 0.578 | 0.582 | +0.004 | 0.127 |

**Reading it.** Every delta is inside its holdout's resolution, most of them by a factor of
three or more, and the signs are inconsistent across formats and subsets. The objective —
the model that actually selects — moves by at most 0.006 anywhere, with a seed spread of
zero because it is deterministic. The largest number in the table, T20I women +0.042, sits
on 70 matches that cannot resolve less than ±0.154. **The honest conclusion is that P-1
buys no measurable discrimination, in the aggregate or in women's cricket.**

That is the outcome the identity checklist's I-5 wrote down in advance as the likely one: 163
collisions out of 13,568 people is about 1% of the roster, and the affected players are not
the ones being selected. It does not make the change wrong. Two people sharing one rating
is a defect whether or not the aggregate notices, and the rest of the plan — the
performance model in P-3, the simulator in P-4, and any per-player number the UI shows —
attributes to a *person*. The reason to record this is so nobody later justifies the work
with a metric that never moved.

**What it does not say.** These are two arms on one holdout window, not a walk-forward, so
they price identity for *this* window only. The women's subsets are too small to resolve
anything, which is itself the finding to carry into E7: gender-split context baselines
cannot be evaluated on the women's holdout of a format until there are more of them.

### 5.2 E7, run — gender-split context baselines (P-2)

**Design.** `RatingState(gender_split_context=True)` keeps the (format, over) runs and
wickets baselines separately for women's and men's matches, so a woman's impact is measured
against women's cricket rather than a blend. Two `ml.xi.train` runs over the same
Cricsheet-JSON source at `--cutoff 2025-09-01`, seeds (0, 1, 2), differing only in the flag.

| format | subset | n | objective off | on | Δ | display off | on | Δ |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| T20  | women | 560  | 0.765 | 0.765 | −0.000 | 0.794 | 0.789 | −0.005 |
| T20  | men   | 1075 | 0.694 | 0.694 | +0.000 | 0.712 | 0.707 | −0.005 |
| T20I | women | 74   | 0.779 | 0.795 | +0.015 | 0.758 | 0.772 | +0.014 |
| T20I | men   | 108  | 0.728 | 0.727 | −0.001 | 0.714 | 0.696 | −0.019 |
| ODI  | women | 141  | 0.774 | 0.772 | −0.002 | 0.810 | 0.810 | +0.000 |
| ODI  | men   | 234  | 0.619 | 0.621 | +0.001 | 0.670 | 0.666 | −0.004 |

**Reading it.** T20 — the only women's subset large enough to say anything — moves by
exactly nothing. The one delta over the decision rule's 0.01, T20I women +0.015, sits on 74
matches that cannot resolve less than ±0.15, and the same arm costs T20I men −0.019 display,
so the rule's "without hurting men's" clause fails even if the noise were believed.
**No effect; the simpler option ships: one shared baseline per (format, over).** The
mechanism stays behind `--gender-split-context` (the flag is recorded in the artifact and
the run report), because §5.1's caveat still stands — the women's holdouts are the thing
that has to grow before this question can be answered better, and re-asking it is then one
flag rather than a branch.

### 5.3 P-3's choices — grid, structure, E1 and E6 (walk-forward folds only)

All four were made by `scripts/experiments/xi/perf_choices.py` on the quarterly walk-forward
folds (2024-01 … 2025-06) of the Cricsheet-JSON source, fitting the two acceptance targets
(runs, wickets) in T20 and ODI; the locked window was not scored until every choice was
fixed. Pinball loss is the mean over the 0.1 / 0.5 / 0.9 levels on the unconditional
population — every XI player, "did not bat" as 0 (H-20).

**Grid** (one seed; the grid is coarse and the seed spread is measured below):

| point | learning rate | leaves | min leaf | L2 | pinball relative to `medium` |
|---|---:|---:|---:|---:|---:|
| small | 0.05 | 15 | 200 | 1.0 | 1.0003 |
| **medium** | 0.05 | 31 | 100 | 1.0 | 1.0000 |
| large | 0.10 | 63 | 50 | 0.0 | 1.0013 |

Flat to a tenth of a percent, as §4 predicted ("model class is not the constraint");
`medium` stays.

**Structure** (three seeds), direct on the unconditional rows vs two-part
(P(involved) × distribution given involvement, the involvement predicted, the served
quantiles those of the mixture):

| format | target | direct pinball | two-part pinball | two-part gain | direct Spearman | two-part Spearman |
|---|---|---:|---:|---:|---:|---:|
| T20 | runs | 2.927 | 2.921 | +0.2 % | 0.542 | 0.544 |
| ODI | runs | 4.590 | 4.592 | −0.05 % | 0.475 | 0.475 |
| T20 | wickets | 0.1424 | 0.1410 | +1.0 % | 0.486 | 0.483 |
| ODI | wickets | 0.1647 | 0.1632 | +0.9 % | 0.557 | 0.552 |

Two-part costs three times the fits (nine conditional quantile models instead of three),
so it has to earn its place: the rule is *two-part only where it beats direct by more than
0.5 % in every format*. It does for wickets — a zero-inflated Poisson is the natural model
of a count whose zeros are mostly "did not bowl" — and not for runs, where 0.2 % and
−0.05 % are a tie. **Wickets ship two-part; every quantile target ships direct.** The
ranking is unmoved either way, which is the first sign of the finding recorded under P-3:
on the unconditional population, within-match Spearman is set by who is involved, not by
how much they do.

**E1 — sequence families** (three seeds; each family added to the base inputs on its own;
rule: keep if pinball falls by more than 1 % somewhere and rises nowhere):

| family (columns) | T20 runs | T20 wickets | ODI runs | ODI wickets | kept |
|---|---:|---:|---:|---:|---|
| dot streaks (`bat_stuck_share`, `bat_release_rate`, `bowl_squeeze_share`, `bowl_squeeze_wrate`) | +0.04 % | −0.14 % | +0.01 % | −0.21 % | no |
| reactions (`bat_after_boundary_rate`, `bowl_after_boundary_rate`, `bowl_after_wicket_rate`) | −0.00 % | +0.06 % | +0.02 % | +0.07 % | no |
| spells (`bowl_spell_first_rate`, `bowl_spell_later_rate`, `bowl_spell_overs`) | +0.01 % | −0.21 % | +0.01 % | −0.21 % | no |

Pinball change against the base inputs (negative is better); base pinball 2.927 / 0.1410
(T20 runs / wickets) and 4.590 / 0.1632 (ODI). Nothing comes within a fifth of the 1 % line.
The bowling families lean the right way on wickets — a fifth of a percent, in both formats —
which is worth knowing and not worth ten columns. **No family is kept**: `SEQUENCE_FAMILIES_KEPT`
is empty, the accumulators stay in the rating pass (`ml/xi/sequence.py`, ten as-of columns
in every player row, checked by the parity test) so the question is one constant away from
being re-asked, and P-6 drops the nine `*_features` tables with nothing to re-implement.

**E6 — format transfer** (three seeds; T20 + T20I jointly with a format indicator vs each
alone; rule: joint only if it wins by more than 0.01 Spearman everywhere it is scored):

| scored on | folds | target | separate Spearman | joint Spearman | Δ | separate pinball | joint pinball |
|---|---:|---|---:|---:|---:|---:|---:|
| T20 | 7 | runs | 0.542 | 0.541 | −0.001 | 2.927 | 2.928 |
| T20 | 7 | wickets | 0.483 | 0.483 | +0.000 | 0.1410 | 0.1411 |
| T20I | 6 | runs | 0.598 | 0.597 | −0.001 | 3.164 | 3.151 |
| T20I | 6 | wickets | 0.565 | 0.568 | +0.003 | 0.1302 | 0.1301 |

Nothing moves by a third of the 0.01 line; the joint fit buys T20I 0.4 % pinball on runs
and nothing else. **Separate models stay** (`E6_JOINT_T20_FORMATS = False`), which is also
what the win model found. The mechanism (a format indicator column, one artifact under both
names) is kept so the question is one constant away.

---

## 6. Migration sequence

Ordered so selection quality improves first and nothing is deleted before its replacement
is measured. One PR each, branch `arch/<id>-<slug>`, same conventions as the selection
checklist.

| id | PR | acceptance |
|---|---|---|
| P-0 | Land S-10; run its acceptance on the DB; set `selection.win_model: "xi"` for limited-overs formats (S-6) | **run; acceptance not met.** The model reproduces on Postgres (objective 0.72 T20 / 0.68 ODI / 0.74 T20I, display 0.75 / 0.73 / 0.72 — within 0.01 of the JSON path), but selection-comparison over 332 locked-window matches reported `xi` 0.560 vs `greedy` 0.569 winner accuracy, so win-probability selection stayed off. **That comparison has since been shown not to have run the arm it named** (§8.5, D-7a): the broken id contract made `/xi/optimize` refuse every limited-overs call on the bowler constraint, and go-app silently fell back to the windowed-form optimiser, so the figure belongs to two other arms. Re-run with the contract fixed it is `winprob` 0.622 vs a rating-ordered `ratings` 0.625 over 357 matches — one match apart, and still not an answer, because the gate is mis-specified: an arm that optimises *both* sides moves the fixture toward parity and must lose winner accuracy regardless of XI quality. Replace it with L4's specific-XI-beyond-typical-XI, swap monotonicity and E5 |
| P-1 | Identity: Cricsheet registry id as `player.external_id`, team + gender as the team key (IDENTITY I-3/I-4) | **done** (`arch/p-1-identity`). 13,483 name-keyed player rows → **13,623** identity-keyed (140 people recovered, 0 fallbacks); `opposition` 394 → **524** (+130, the predicted count); squad gender disagrees with `match.gender` on 0 rows; the rating pass keys off `external_id` on both sources. E4 recorded below: **no format and no gender subset moves by more than its holdout can resolve.** Re-import is reproducible in row counts and in identity content; per-match squads became reproducible one PR later, with §10.4's match-identity fix. Franchise lineage (I-4) is not in this PR |
| P-2 | L1 emits player-match rows + expected batting slot + phase splits; L4 harness skeleton with the performance metrics; **an as-of serving path** (`XiStore` answers "ratings as of date D", not only "through today") and **per-match rows** in the selection report | **done** (`arch/p-2-rating-rows-harness`). The day-close pass emits 463,818 player-match rows — all XI players, never only those who batted (H-20) — with expected batting slot, innings share and phase-split impact rates, identical from both sources. `ml.xi.asof` answers `ratings_as_of(D)` and raises rather than run backwards, so a backtest at date D provably cannot see D or later; `freeze_ratings.py` is retired, `/xi/*` accept `as_of`, the selection comparison sends each match's date and its report carries per-match rows. `make xi-evaluate` runs the walk-forward + locked window + H-8 parity from one command into one JSON report. Acceptance: career-mean within-match Spearman on the locked window 0.317 T20 / 0.318 ODI (the script's ≈ 0.32 / 0.34, computed there with cross-format career means — inside the 0.31–0.35 band §1 calls the ceiling); parity max abs difference 0.0 on both sources; E7 measured, no effect (§5.2) |
| P-3 | L2-B performance model (quantile runs/balls, Poisson wickets) + `/performance/predict` taking XI ids; E1, E6 | **done** (`arch/p-3-performance-model`). `ml/xi/performance.py`: per format, quantile (0.1 / 0.5 / 0.9) models of runs, balls faced and runs conceded, a two-part zero-inflated Poisson of wickets, a Poisson rate of catches, and P(bats) / P(bowls) — all on the unconditional population (H-20), innings marginalised at prediction, three seeds, a three-point grid tuned inside the folds (flat), E1 (no family kept) and E6 (separate) in §5.3. Walk-forward over 7 folds, 3 seeds (§8.2): **runs** beat the career mean on Spearman (+0.040 ± 0.007 T20, +0.048 ± 0.013 ODI) and pinball (2.93 vs 5.09, 4.59 vs 7.96), median MAE −9 %; **wickets** beat it on pinball (0.141 vs 0.260, 0.163 vs 0.302) and tie on Spearman in ODI (+0.001 ± 0.012) but **trail it by 0.029 ± 0.012 in T20** — a tie-averaging artifact of the unconditional metric, recorded below rather than gamed; among the players who bowled the model ranks better in both. Locked window: per-end coverage inside ±0.03 everywhere, no recalibration triggered (H-5); width reported beside coverage (H-22). `POST /performance/predict` serves it; H-8 parity holds for rows and predictions on both sources at 0.0 — after it found the fifth defect, §10.4 |
| P-4 | L2-C simulator; scorecard and totals from it; E2 | **done** (`arch/p-4-simulator`). Design written first (§3, "The innings sample"); `ml/xi/simulator.py` draws whole matches from L2-B's forecasts — sequential by expected slot under the as-of innings length, chase ended at the target, bowlers attributed, toss marginalised — with one shared as-of match factor (deconvolved residuals of the calibration fold) that the folds showed was needed (§8.3: dispersion ratio 1.42 → 1.02 T20, 1.36 → 1.02 ODI; coverage 0.64 → 0.76, 0.58 → 0.74; before/after recorded). Locked window (§8.3): first-innings 10–90 coverage **0.786 T20 / 0.790 ODI** (acceptance ±0.03 met), dispersion ratio 0.98 / 1.02, width 90.6 / 154.1 runs; 9.4–9.9 ms per fixture at 2,000 draws. E2 within tolerance (+0.003 / +0.003 Brier on the folds), so the simulated P(win) is served as a probability beside the display model, which stays the headline. `POST /simulate`; the go-app xi scorecard reads it (totals, points and ranges from the draws; extras / innings models and the rescale unused on that path); L3 explanation = marginal values + spread shares. H-8 parity extended to the draws at a fixed seed: max abs difference 0.0 over 39 simulated matches on both sources, and the two sources agree on every locked-window figure (T20 coverage 0.786 / 0.786, ODI 0.790 / 0.787, Brier to 0.0004) |
| P-5 | Re-point team prediction and backtest surfaces to L2/L3; delete the greedy weights, the meta-model, the rescaling layers, the Normal Monte Carlo and the per-call optimiser | **done** (`arch/p-5-repoint-surfaces`). A limited-overs `/api/predict/*` response now carries the display model's P(win) with `source` on the wire, the simulated totals with their 10-90 ranges, per-player ranges and marginal values, and a `scorecard` block whose lines and extras sum to the total by construction; nothing rescales a simulated total toward anything, on any path. **S-6 ships here**: the XI path is the only selection path for T20 / T20I / ODI, on L4's replacement gates rather than P-0's mis-specified winner accuracy. **Re-measured on the database for this PR (§8.4), and one of the two gates is weaker than §8.1 recorded**: swap monotonicity passes comfortably (0.0–0.8 % violations against a 2 % line), but the specific-XI-beyond-typical-XI delta is **+0.012 ± 0.010 in T20** over seven folds — positive, about 1.2 sd from zero — and inside its own noise in ODI (+0.024 ± 0.069) and T20I (+0.021 ± 0.068), not the +0.045 ± 0.021 this document carried. The switch ships on that reading, recorded rather than rounded up. **TEST** gets `objective: "ratings"` on `/xi/optimize` — the search's own seed order under the same constraints, evaluating no model — labelled `optimised: false` in the API and shown as a *Not optimised* notice in the UI (H-17); its per-player numbers come from `/performance/predict`, and it has no innings total because it has no innings length. The backtest surface is L4's report, served by the new `GET /xi/evaluate-report` and proxied at `/api/backtest/report`: walk-forward folds with the locked window labelled beside them, the two selection metrics with a labelled slot for E5, per-target performance with width beside coverage, the E2 section and the serving-parity verdict. `ml/metrics.py` and both its callers are gone (H-12). Migration `0007` drops `match_prediction_aggregates`, the one table whose last reader and last writer both died here; nothing else was orphaned by P-5 that P-6 does not already own. Coverage ratcheted: Go 68, ml-service 86, frontend 74/69/74/77. **Also fixes two defects found while smoke-testing this PR (D-7a, D-7b, §10.6)**: the id contract now carries the registry id end to end, and a read of the rating state no longer mutates it |
| P-6 | Delete precompute, snapshots, exports, auto-tune stack, old win model; three-step ops pipeline; run-id artifacts | **done** (`arch/p-6-run-pipeline`). **Deleted, with every reader re-pointed or removed in the same commit:** `internal/precompute` and its two commands; `internal/seqcalc` and its nine calculators; `internal/services/exportdataset`, `internal/db/exportqueries`, `internal/features` and the `export-dataset` command; go-app's `training-data` endpoint, the tuned-params store and handlers, the model-stats DB enrichment and the weather probes; and in ml-service `ml.train_win`, `ml.win_features`, `ml.win_discrimination`, `ml.auto_tune` with its PyCaret / AutoGluon / Optuna wrappers, the whole `ml/tuning` package, `ml.export_csv`, `ml.validate_exports`, and the per-format artifact registry they shared (`app/artifacts.py`, `app/artifact_service.py`, `app/model_metadata.py`, `app/model_stats_service.py`, `app/prediction_service`). Optuna, PyCaret, AutoGluon and SHAP leave `requirements.in` and CI, which **collapses the two Docker stages into one image** and the two requirements files into one. **Migration `0008` drops fourteen tables** — `feature_raw_stats_snapshots`, `player_window_features`, the nine `*_features` tables, `ml_tuned_params`, `weather_data`, `weather_job` — every one of which lost its last reader *and* its last writer here. **Survivors, recorded rather than dropped:** `batting_data`, `bowling_data`, `fielding_data` and `fielding_event` lost their last reader but the importer still writes them, and the importer is the one component §4 marks *keep*; they are write-only now, and the PR that stops the importer writing them is the one that drops them. **Three steps:** import → retrain → reload, with `evaluate` beside them as `Optional`; `Requires` is retrain←import, reload←retrain, evaluate←import (asking "what would this have scored?" is not gated behind producing the artifacts it is not measuring). `confirmDefaultParams` had nothing left to confirm and is gone; the run plans lose `tune` and `data-refresh`. **The grid** is `ml.xi.train.DISPLAY_GRID`: three points for the display model, scored on a temporal split *inside* the training rows, the incumbent kept unless a candidate beats it by more than 0.002 AUC (H-14 applied to a choice). Measured on the database, **it kept the incumbent in all four formats** — the manifest records the reason and every candidate's score — so **no number in this document moves**: objective/display AUC is 0.721/0.737 T20, 0.748/0.731 T20I, 0.677/0.716 ODI, 0.575/0.598 TEST, identical to the pre-P-6 report on the same data. **H-16 and D-6 close together**, because they are the same question: every retrain writes `runs/<id>/` with the artifacts, the run's report and `manifest.json` (run id, cutoff, dataset sha, git sha, rating params, the hyperparameters chosen *and why*, headline metrics, and the rating state's shape), `current` is a pointer file naming one run, and the loader refuses a run with no manifest or whose arrays are not the arrays this code reads, with an error naming the run and the widths (§10.5). **H-11**: a live prediction against ratings older than `ml.ratings_max_age_days` (default 14) is refused with `RATINGS_STALE`; an `as_of` request is not, because a backtest asks for a date and gets it. **§8.7's rule is enforced on every substitution**: `selection`, the new `forecast` block and `win_probability.source` each name which model answered, and the two paths that used to leave a player's row at zeros when a response omitted him now fail instead — a substitution that cannot be labelled must not be made. **Acceptance, measured on this database (22,734 matches, on the machine this branch was written on).** From an empty database: migrate (1 s) → import (**2 min 06 s**) → retrain (**12 min 26 s**) → reload (**6 s**), **14 min 40 s** end to end, inside the 15-minute line but not comfortably — and that retrain was competing with a test suite; the same retrain uncontended took **9 min 52 s** (rating pass 3 min 34 s, then the four formats' win and performance models), which puts an idle chain at about 12 minutes. `fetch` is not in the figure: it is a ~4 GB download whose time is the link's, and the archive was already extracted. The run the chain produced loaded under its own manifest — 13,569 players, four formats, `ratings_through 2026-08-25`, fresh against the 14-day limit — and its `dataset_sha` (`38f99adb…`) is **identical** to the one a retrain against the pre-existing database produced, which is the digest doing its job. A second retrain wrote a second run and `POST /admin/reload?run=<id>` swapped between them in both directions. The two D-6 shapes were checked against the real artifacts: a run with its `manifest.json` removed, and one with the nine post-P-2 arrays stripped from `xi_ratings.joblib`, each answered **409 `RUN_ARTIFACTS_INVALID`** naming the run and the missing arrays rather than loading and raising `IndexError` later. With `XI_RATINGS_MAX_AGE_DAYS=3` against 8-day-old ratings, `/xi/predict-win` answered **503 `RATINGS_STALE`** with the hint naming retrain, while the same request carrying `as_of` was served. The L4 harness is **not** in that chain and is not in `retrain`: measured here it takes **53 min 40 s**, so folding it in would put the budget out of reach — `evaluate` is the optional step, and what a retrain records is its own holdout report, which the manifest names. Coverage ratcheted: Go 68 → 74, ml-service 86 → 91, frontend lines/statements 74 → 75 and functions 69 → 73 |
| P-7 | E3 batting-order suggestion; E5 natural-experiment metric in L4 | **done** (`arch/p-7-e5-and-order`). **E5 lives in L4** (`ml/xi/natural_experiment.py`), lineup-only, per format: both elevens scored in the later fixture at its as-of, the previous eleven read from the as-of serving path in one advancing pass and the fielded one checked against the frame (parity 0.0 over 25,650 pairs), per fold, pooled, and on the locked window labelled; §5's as-played form is not computed and the slot says why. The wiring test (`e5_reproduce.py`) lands on §8.6 exactly (T20 0.509, ODI 0.521, T20I 0.589; same pairs, same counts). **The bar is derived** (§8.8): Bernoulli outcomes at the objective's own probabilities give the agreement an exactly-right objective would score — about 0.52 in every format, so 0.55 was never reachable — and the bar is that distribution's 5th percentile for the pairs available. Walk-forward: **T20I 0.586 vs 0.469 passes, ODI 0.562 vs 0.487 passes (a flip of §8.6's reading), T20 0.490 vs 0.501 fails** — the same verdicts on every development pair. **Scoping decided**: T20 is served the rating-ordered eleven (`NOT_OPTIMISED_REASONS`, mirrored in go-app, the reason on the wire and in the UI as TEST's is); T20I and ODI stay optimised; the harness restates the decision every run beside its verdict. **E3 answered**: the best of 64 sampled top-seven orders, confirmed with a fresh seed, moves the simulated median total by > 3 % for 25.0 % ± 3.1 of T20 and 12.6 % ± 2.3 of ODI development elevens — under the 30 % line, order stays with the captain. **H-23 enforced**: `ml/xi/gates.py` registers every gate's varies / fixed / decides triple, the report embeds it and fails without it, the UI renders it beside each number, E3 printed its triple before running. Coverage ratcheted: ml-service 91 → 92, frontend lines/statements 75 → 76 and functions 73 → 74; Go holds at 74 |

Each of P-2 … P-6 removes more than it adds. The end state is smaller than the current tree.

**The migration is complete (P-7, 2026-09-02).** Every claim this document makes is now a
number `make evaluate` reproduces, and every gate it runs says what it varies. Where the
system's numbers stand, walk-forward unless labelled: objective / display AUC 0.721 / 0.737
T20, 0.748 / 0.731 T20I, 0.677 / 0.716 ODI, 0.575 / 0.598 TEST (§8.4, unchanged by P-6's grid);
swap monotonicity under 1 % everywhere; specific-XI delta +0.012 ± 0.010 T20; E5 lineup-only
against its derived bar — T20I 0.586 vs 0.469, ODI 0.562 vs 0.487, T20 0.490 vs 0.501 (§8.8);
runs Spearman +0.040 / +0.048 over the career mean with intervals at nominal coverage (§8.2);
simulated totals' 10–90 coverage 0.786 T20 / 0.790 ODI on the locked window, simulated P(win)
within 0.003 Brier of the display model (§8.3); parity 0.0 on both sources. **Selection is
optimised in T20I and ODI, rating-ordered in T20 (E5) and TEST (H-17)**, each with its reason on
the wire. Still open, none of it blocking: the T20 objective's lineup-only agreement is the
number to move, and its bar is written down; T20 wickets trail the career mean on the
unconditional Spearman (a tie artifact, §8.2); the chase total under-covers from the low side
(§8.3); the four write-only importer tables await the PR that stops writing them (P-6); the
women's-holdout question behind H-7 waits for more women's matches. The plan has no further
PRs.

---

## 7. Risks and what bounds them

- **Performance prediction disappoints in absolute terms.** It will: the ceiling is set by
  the game (Spearman ≈ 0.35 for any predictor). The plan's answer is to ship calibrated
  distributions and rankings, and to say so in the UI ("expected 18 (8–41)") rather than a
  point that reads as a promise. *Measured in P-3 (§8.2): 0.33 among the batters, a 9 %
  median-MAE gain, intervals at nominal coverage — as forecast.*
- **Identity work delays everything.** P-1 is first because ratings inherit it, but P-2/P-3
  can be developed on the Cricsheet-JSON source in parallel (which does not have the defect)
  and re-run on the DB when P-1 lands.
- **Deleting the base models breaks displayed features.** P-5 re-points every consumer
  before P-6 deletes anything; the frontend contract check guards it.
- **Latency.** L2-B inference for 22 players is one batch call; the simulator at N=2,000 is
  sub-second in numpy. Budget both in P-4.
- **Test formats.** Nothing here helps TEST selection (AUC ≈ 0.6 under every feature
  family). Keep TEST on greedy, say so, and spend the effort on limited-overs formats.

---

## 8. Model health checklist — what "healthy" means for every model we keep

Sections 3–6 make the models *right*; this section is what keeps them *healthy* in use.
Each item is a gate in the L4 harness (fails the retrain) or a rule of the serving path.
Where an item was measured in this session the number is given; where it was not, the item
names the experiment that will.

| id | property | applies to | gate / rule | status |
|---|---|---|---|---|
| H-1 | **As-of by construction.** A row's features cannot depend on its own match or any later one | all | Unit test: a match's row is identical with or without later matches and unaffected by its own result (`test_features_are_as_of_and_never_see_their_own_match`) | done (S-10) |
| H-2 | **Leak canaries.** The model must beat its own best single column by a clear margin, and no column may be a scoreboard read-through | win, performance | Harness reports best-single-column AUC (done); add the TEST-as-control check from S-3c: a column whose AUC collapses in TEST but not elsewhere is suspect | **done (P-2)** — `make xi-evaluate` reports both. Its first run flagged one suspect, `team_h2h` in T20I (0.70 there, 0.55 in TEST): reviewed and benign — a team-context column, excluded from the objective, and head-to-head genuinely means less where draws exist. The point is that the flag fired and someone had to say why it was fine |
| H-3 | **Toss / batting-order marginalisation.** The label defines team1 as the side batting first; at selection time that is unknown | win (objective + display), simulator | Serving path averages both orientations; `P(A,B) == 1 − P(B,A)` is a unit test. Measured: display AUC T20 0.741 → **0.747**, ODI 0.720 → **0.730**, T20I 0.715 → **0.731**; the oriented probability moved by 0.07–0.12 on an unknown. Once the toss is known, pass `team1_bats_first` | **done (this PR)** |
| H-4 | **Monotone objective.** Upgrading a player never lowers the selection score | win objective | Harness: share of one-player upgrades with Δp < 0 must stay < 2% (measured 0.4% T20 / 3.7% ODI for logistic; 12% / 20% for unconstrained boosting) | **done (P-2)** — reported per fold in `make xi-evaluate`; walk-forward means 0.3% T20 / 0.8% T20I / 0.0% ODI / 0.6% TEST, all under the 2% line. **The line covers the objective only** (B-7): the *display* surface is now probed the same way and reported beside it (`display_swap_violation_share`, 5.1% T20 / 7.1% T20I / 3.4% ODI / 6.1% TEST), as a measurement and not a gate, so the gap between the surface a selector reads and the surface a person watches is stated rather than invisible. Constraining the display model's team-context columns was gated (`B-7-display-monotone`) and is a **recorded null** that moves the violations the wrong way; it never could have worked, because the probe holds team context at the fixture's values. The whole effect is `pelo_std`, the one column an upgrade moves that the contract leaves free, and removing it from the display columns takes the share to exactly 0.0000 for ~0.005 of AUC in ODI and TEST — priced by the informing gate `B-7-pelo-spread` and left for a deliberate decision in `docs/BUG_BACKLOG.md` § B-7. Until it is taken, H-4's guarantee is the objective's alone |
| H-5 | **Calibration of what is displayed.** The probability shown must mean what it says | win display, simulator, performance quantiles | Harness: reliability curve + Brier vs base rate per format (done for win); isotonic recalibration fitted on a temporal fold if Brier is worse than base rate; quantile coverage within ±0.03 of nominal (measured 0.80 / 0.79 on 0.80) | **done for the performance quantiles (P-3) and the simulator (P-4)**. Simulator: simulated P(win) within 0.01 Brier of the display model on the folds (§8.3), so served as a probability beside it; totals' 10–90 coverage is the gate the shared factor was added for — on the locked window 0.786 (T20, 1,521 innings) and 0.790 (ODI, 347) with the PIT flat to 0.04; the chase total under-covers from the low side (0.72–0.73), recorded in §8.3. The performance check is per quantile end, because the targets have a point mass at zero: a level must sit between its strict and inclusive exceedance ± 0.03 (`perf_calibration.coverage_off_nominal`). On the locked window P(y ≤ q90) is 0.896 / 0.892 / 0.896 / 0.899 for runs (T20 / T20I / ODI / TEST) and P(y < q10) ≤ 0.12 everywhere; the 3-way wicket probabilities are within 0.02 of observed. No target trips the check, so the isotonic recalibration (`ml/xi/perf_calibration.py`, fitted on the last quarter of the training rows, applied at prediction, unit-tested) ships in place and unused, and the harness re-decides it every run |
| H-6 | **Rating hyperparameters are not load-bearing.** decay and prior were chosen by judgement | rating pass | Measured sweep (`health_experiment.py`): decay 0.80–0.97 and prior 20–150 move objective AUC by ≤ 0.01 in T20 and ODI — flat. Keep 0.90 / 60; re-run the sweep when the pass changes | done |
| H-7 | **Gender- and competition-aware baselines.** Context expectations (runs per ball per over) are per format only; women's and men's matches share them, and so do the IPL and a club league | rating pass | E7: split the context baseline by gender (cheap, gender is on every match); measure the women's-subset AUC before/after. Competition tiers only if E7 shows gender matters | **measured (P-2, §5.2): no effect** — the T20 women's subset moves by 0.000 and the only larger delta sits inside a 74-match holdout while hurting the men's display. The split ships off, behind `--gender-split-context`, to be re-asked when the women's holdouts grow; competition tiers are therefore not pursued |
| H-8 | **Train / serve parity.** The serving store must compute the same features the training frame holds | all | Harness: for the last 50 holdout matches, rebuild the row from the serving store *as of that date* and assert equality with the training frame (the S-3c defect, made a test) | **done (P-2)** — `serving_parity` in `ml.xi.asof` rebuilds the last 50 matches' win *and* player-match rows through the as-of path and fails `make xi-evaluate` on any difference; measured max abs difference 0.0 on both sources. The state evolution is genuinely different code on the two sides (day-close buffering vs a strict date threshold) while row assembly is shared (`ml.xi.rows`), so the D-4 class — a column spelled differently in serving — cannot recur, and drift in the as-of logic is caught. (P-0's find, for the record: the legacy path served a 37-wide vector to a 38-wide scaler over `inning` vs `batting_inning`) |
| H-9 | **Identity.** Ratings keyed by name merge people | all | P-1 (IDENTITY I-3/I-4); E4 measures the delta | **done**: players key off the Cricsheet registry id, teams off (club, gender) — one row per (name, gender) since P-1, folded onto the club by `opposition.canonical_id` since I-4 — on both sources, so the two paths produce the same keys and their artifacts are comparable. E4 found the correction worth ≤ 0.01 AUC everywhere it can be resolved, and the lineage merge is smaller again; both are correctness, not discrimination. S-7's opposition encoding is unblocked |
| H-10 | **Cold start is bounded.** A player with no history must regress to neutral, never explode | win, performance | Measured: replacing a player by a debutant moves p by a median −0.003, p10 −0.05. Unit test on `side_vectors` for an unseen key | done — **re-measured by X-1b (§8.12)** on A-4's folds, replacing team1's lowest-Elo player: median Δp −0.034 / −0.022 / −0.021 / −0.013 and p10 −0.077 / −0.061 / −0.069 / −0.042 (T20 / ODI / T20I / TEST). An age-shaped neutral (the age-band debut prior) stayed inside those bounds and worsened the debut rows' forecasts in every format, so the neutral vector stays |
| H-11 | **Staleness.** Ratings are only as fresh as the last import | rating pass | `/xi/status` reports `ratings_through`; the ops step fails a prediction request with a clear code if it is older than N days (config, default 14). Retrain is one command, so the cadence is "after every import" | **done (P-6)** — the limit is `ml.ratings_max_age_days` / `XI_RATINGS_MAX_AGE_DAYS`, default 14, and it is applied in one place (`XiRegistry.store_as_of`), so the verdict a live request is refused on and the verdict `/xi/status` reports are the same computation. A live request past the limit is refused with `RATINGS_STALE` and a hint naming the step that fixes it; a request that names its own `as_of` is not, because a backtest asks for a date and gets it — refusing one would break the harness for a reason that does not describe it. `/xi/status`, `/health` and `/ops/status` all carry the verdict (`fresh`, `age_days`, `max_age_days`, `code`), not just the date, and the prediction tab says so before the request is made. Zero turns the check off, which is a decision visible in config rather than a state the code can drift into |
| H-12 | **Per-target, never pooled metrics.** A headline number must be for one target on one population | performance | `ml/metrics.py`'s raveled multi-output MAE is retired; L4 reports per target | **done (P-3)** for everything P-3 touches: `ml/xi/perf_metrics.py` scores one target on one population and the harness and `xi_win_report.json` carry the numbers per target and format. **`ml/metrics.py` is deleted (P-5)** along with both its callers — the shared multi-output training pipeline and the regression half of the tuning stack — so a pooled, raveled metric can no longer be computed at all. Every number the harness reports names one target on one population |
| H-13 | **Consumer metric first.** AUC for an argmax, Spearman/top-k for a ranking, coverage for an interval | all | Every model in L4 has a named consumer and its metric is the one that gates | rule |
| H-14 | **Seeds and noise floor.** Differences under the seed spread are not evidence | all | Every reported number is a mean over ≥ 3 seeds with the spread (done for win) | done |
| H-15 | **Data-quality gate.** Undecided matches, sides without squads, namesakes, replacement players | rating pass | Two checks, both in `ml/xi/quality.py`. **Accounting:** a source offers N matches and must yield, scope out or reject exactly N — a match dropped for a reason nothing names fails the run. **Doubling:** any quality count over twice the last accepted run's, or one that was zero and is not, fails. The accepted counts live in `xi_data_quality_baseline.json`, which a *failing* run does not update, so re-running cannot clear the gate; `--accept-data-quality` is the one way to move it. Beside it, `make xi-parity` runs both sources and compares every count and the player-key sets | **done**. Measured baseline: 22,734 offered = 22,734 read, 1,710 undecided, 0 namesake sides, 1,358 sides over eleven, 0 unresolved player keys, 13,569 player keys, 514 clubs — identical from both sources. Its first two real runs found two more defects; see §10.4 |
| H-16 | **Run identity.** A measurement must name the artifact it measured | all | `runs/<id>/manifest.json` with dataset sha, cutoff, git sha, hyperparameters, metrics (D-3) | **done (P-6)** — every retrain writes `runs/<run_id>/` holding the artifacts, the run's report and `manifest.json`: run id, created-at, cutoff, dataset sha (a digest of the matches the pass consumed, computed from what was read because ml-service does not mount the dataset directory), git sha, rating params, the hyperparameters the grid chose *and why*, the run's headline metrics per format, and the rating state's shape. `current` is a pointer file naming one run, not a pile of files, so reload can swap between runs and a bad retrain leaves the run before it exactly where it was. The manifest is written last, so a directory only becomes a run once everything it names is on disk — a retrain that dies half-way leaves wreckage that `/artifacts/status` lists as "no manifest" and that the loader never selects. `/xi/status` carries the run id and the manifest summary beside `ratings_through`, and the Workbench renders them |
| H-17 | **Format scope.** Selection is only offered where the objective ranks | win | TEST gets a selection but not an optimised one, with a note in the UI; an objective with holdout AUC < 0.65 is not used for selection in that format | **done (P-5)** — the rule is enforced in one place, `ml.xi.optimizer.OPTIMISED_SELECTION_FORMATS`, and `/xi/optimize` refuses `objective: "win"` outside it rather than serving a search over a model that does not rank. TEST is served `objective: "ratings"`: the search's own seed order (as-of rating, keeper and bowler constraints first), evaluating no model and needing no opponent XI — which is what let P-5 delete the greedy weights without leaving TEST unselectable. The response carries `optimised: false` and the go-app path turns it into the *Not optimised* notice the tab shows; a test asserts the marginal-value column disappears with it |
| H-18 | **Day-close batching.** A match never sees a same-day result | rating pass | Implemented in `ml.xi.builder`; unit-tested; cost ≤ 0.003 AUC | done |
| H-19 | **Walk-forward evaluation + locked window.** Choices are made on rolling cutoffs; one final window is scored once per release | all | L4 reports mean ± spread over cutoffs; the locked window (≥ 2025-09-01) is never used for a choice | **done (P-2)** — `make xi-evaluate`: quarterly rolling origins 2024-01 … 2025-06, the locked window scored once and labeled. First per-format walk-forward table in §8.1; the locked-window figures sit inside the fold spreads, toward the top for T20 — what later origins with more training data should produce — so the development-window reuse §10.3 could only estimate is now priced. (P-0's locked-window report, 2025-09-01 → 2026-08-25 with no drop, was the precursor: same window, but the one that guided the choices). **The window has since rotated** (follow-up A-4, 2026-09-02): every choice of this migration read the ≥ 2025-09-01 window, so it retired into the walk-forward folds and the line moved to the P-7 merge date. Every locked figure quoted in this document is the retired window's; the rotation policy is in `docs/ml-and-training.md` § Rotating the locked window |
| H-20 | **Unconditional training population.** Rows are never selected by the outcome (who batted, who bowled) | performance | Training rows are all XI players with as-of expected involvement; two-part targets allowed only if both parts are unconditional | **done (P-3)**: every target trains and is scored on all XI players with "did not bat / bowl" as 0; the baselines are defined on the same population (the unconditional career mean, not the mean over innings batted). The one two-part target, wickets, fits P(bowls) on the unconditional rows and reads the involvement from that classifier at prediction, never from the outcome; a unit test asserts no outcome column is an input |
| H-21 | **No in-sample stacking.** A model output consumed downstream is out-of-sample for that row | performance → simulator, any meta-model | as-of features or out-of-fold predictions from a temporal split; the harness asserts the second stage never scores a row the first stage trained on | **done for P-3's second stage**: the only fitted consumer of a model output is H-5's quantile recalibration, and it is fitted on the last quarter of the training rows by date, which the members do not train on (`performance._temporal_calibration_split`; unit-tested). The two-part mixture is arithmetic, not a fit. **Done for the simulator (P-4):** it consumes L2-B's forecasts for the fixture (as-of predictions, never a row's own outcome), three as-of rates from the rating pass (`SIMULATION_CONTEXT_COLS`), the runs–balls copula correlation from the training rows, and a shared factor whose residual distribution comes from the 92-day calibration fold the members do not train on; the harness's E2 rows are fixtures after each fold's cutoff |
| H-22 | **Sharpness at fixed calibration is the progress metric.** For a distributional system "better" means narrower intervals while coverage stays nominal, never a smaller point error | performance, simulator | L4 reports mean 80% interval width beside coverage, per target and format, release over release; narrower with coverage held is progress, narrower with coverage falling is a regression and fails the gate. CRPS / pinball as the single proper score | **reported (P-3)** — width beside coverage for every target and format in §8.2 and in `xi_evaluate_report.json`, with the career-quantile baseline's width and coverage beside them. First release: T20 runs 29.1 wide at 0.897 inclusive coverage against the career quantiles' 26.0 at 0.782 — the baseline is narrower only by under-covering. Pinball is the proper score (mean over the three levels; CRPS was not added: two proper scores buy nothing a second column cannot). The release-over-release gate has one release to compare against so far. **Applied to totals (P-4, §8.3):** the simulated 10–90 interval's coverage and width per format — without the shared factor 0.638 / 60.6 (T20) and 0.576 / 107.3 (ODI), with it 0.764 / 82.8 and 0.743 / 150.6 on the folds; the narrower interval was the under-covering one, so the wider is the progress. Locked window: first-innings 10–90 coverage 0.786 at 90.6 runs wide (T20), 0.790 at 154.1 (ODI), the widths the next release has to narrow without giving that up |
| H-23 | **A gate must name what varies between its arms and what is held fixed.** Three gates in this project reported a clean pass or fail while moving for a reason other than the thing they named: P-0's winner accuracy scored the *fixture* (both arms optimise both sides, pushing it toward parity, §8.5); E5's as-played form scores mean reversion (a nonzero Δresult is an identity on `won_k`, which the as-of rating update already anticipates, §8.6); and the P-0 run reached neither arm it named because the id contract made the optimiser refuse (D-7a) | all | Every gate in L4 states, beside its number, the quantity it varies and the quantities it holds fixed; a gate that cannot say so is not reported as a pass or a fail | **done (P-7)** — `ml/xi/gates.py` is the registry: every gate the report prints (H-17's AUC line, H-4, specific-vs-typical, E5, E2, H-5, H-22, H-2, H-8) and the one a script runs (E3) declares its *varies / fixed / decides* triple and the path at which the report carries its number. The report embeds the registry, `gates.check_report` fails `make evaluate` if a gate is printed without an entry or an entry has nowhere to be read from (unit-tested both ways), the Evaluation tab renders the triple beside each number, and `e3_batting_order.py` prints its triple before it runs. The first gate written under the rule, E5 lineup-only, is the one whose as-played form the rule would have caught |

Nothing in the list is open: H-11 and H-16 closed in P-6, H-23 in P-7 (H-2, H-4, H-7, H-8,
H-15 and H-19 as of P-2; H-5, H-12, H-20, H-21 and H-22 as of P-3 and, for the simulator,
P-4). Every row is now a gate the harness runs or a rule the serving path enforces.

### 8.1 First walk-forward report (H-19, P-2)

`make xi-evaluate` on the full dataset, quarterly rolling origins 2024-01-01 … 2025-06-01
(each fold trains strictly before its cutoff and scores the window to the next one; the
locked window ≥ 2025-09-01 is scored once, never used for a choice). Walk-forward columns
are mean ± sd over folds; the display model additionally averages three seeds per fold.
Identical to 4 dp from the Postgres and Cricsheet-JSON sources.

| format | folds | objective AUC, walk-forward | display AUC, walk-forward | objective, locked | display, locked | n locked |
|---|---:|---:|---:|---:|---:|---:|
| T20  | 7 | 0.684 ± 0.044 | 0.713 ± 0.059 | 0.721 | 0.746 | 1635 |
| T20I | 6 | 0.772 ± 0.042 | 0.752 ± 0.054 | 0.747 | 0.733 | 182 |
| ODI  | 7 | 0.670 ± 0.077 | 0.708 ± 0.082 | 0.683 | 0.729 | 375 |
| TEST | 7 | 0.637 ± 0.121 | 0.671 ± 0.090 | 0.582 | 0.594 | 157 |

T20I's first fold is skipped for too few evaluation matches, hence six folds. The spreads
are the honest error bars this document previously lacked: a quarterly T20I or TEST window
holds 30–90 matches, so their ±0.04–0.12 is mostly window size, not model drift. The
locked-window figures sit inside every spread — toward the top for T20, which is what later
origins with more training data should produce. Beside the win model, the same folds carry
swap monotonicity (0.0–0.8 % violations, all under H-4's 2 % line), the
specific-XI-beyond-typical-XI delta (+0.045 ± 0.021 T20; within noise elsewhere — **but see §8.4: the same harness on the database measures +0.012 ± 0.010 for this quantity, so treat the figure in this paragraph as superseded**) and the
performance baselines (career-mean within-match Spearman 0.300 ± 0.011 T20 / 0.306 ± 0.043
ODI over folds, 0.317 / 0.318 on the locked window); the full detail is per fold in
`xi_evaluate_report.json`.

### 8.2 First performance report (P-3)

`make xi-evaluate` on the full dataset, the same quarterly origins as §8.1, the model fitted
per fold on every XI player before the cutoff (three seeds) and scored pre-toss on the
window; the baselines on the same rows. Ranking is within-match Spearman (the mean for
counts, the median otherwise); pinball is the mean over the 0.1 / 0.5 / 0.9 levels; the
interval is the 10–90 range, coverage inclusive, width its mean. Targets marked `*` are
reported, never a headline. The Postgres and Cricsheet-JSON sources agree to 4 dp on every
baseline (the frames hold the same rows) and to within 0.005 Spearman and 0.35 % pinball on
the model for every headline target: the two sources order same-day rows differently and the
early-stopping split follows row order, which is the seed spread showing up as a source
spread. Catches, a target with almost no signal, wanders by up to 0.1 Spearman for the same
reason. The table is the Cricsheet-JSON run.

**Walk-forward** (mean over folds; Δ is model minus career mean, ± sd over folds):

| format | target | folds | Spearman: model | career mean | Δ ± sd | pinball: model | career mean | career quantiles | median MAE: model | career mean | 80 % interval: coverage / width, model | career quantiles | ranking among the involved: model | career mean |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|---|---:|---:|
| T20 | runs | 7 | **0.542** | 0.502 | 0.040 ± 0.007 | **2.927** | 5.087 | 3.162 | 9.21 | 10.17 | 0.897 / 29.1 | 0.782 / 26.0 | 0.333 | 0.311 |
| T20 | balls_faced | 7 | **0.587** | 0.548 | 0.039 ± 0.006 | **2.315** | 4.006 | 2.532 | 7.36 | 8.01 | 0.895 / 24.1 | 0.763 / 20.8 | 0.363 | 0.342 |
| T20 | wickets | 7 | **0.483** | 0.512 | -0.029 ± 0.012 | **0.141** | 0.260 | 0.156 | 0.45 | 0.52 | 0.950 / 1.3 | 0.901 / 1.2 | 0.167 | 0.133 |
| T20 | runs_conceded | 7 | **0.733** | 0.737 | -0.004 ± 0.005 | **1.921** | 3.139 | 1.991 | 5.96 | 6.28 | 0.912 / 22.1 | 0.809 / 15.4 | 0.272 | 0.243 |
| T20 | catches * | 7 | **0.036** | 0.146 | -0.112 ± 0.000 | **0.104** | 0.218 | 0.114 | 0.31 | 0.44 | 0.956 / 1.0 | 0.905 / 0.9 | — | — |
| T20I | runs | 6 | **0.598** | 0.547 | 0.051 ± 0.017 | **3.164** | 5.436 | 3.445 | 10.02 | 10.87 | 0.885 / 30.5 | 0.784 / 26.8 | 0.388 | 0.342 |
| T20I | balls_faced | 6 | **0.639** | 0.600 | 0.039 ± 0.019 | **2.278** | 3.909 | 2.481 | 7.26 | 7.82 | 0.883 / 23.6 | 0.775 / 20.4 | 0.428 | 0.398 |
| T20I | wickets | 6 | **0.565** | 0.565 | -0.000 ± 0.006 | **0.130** | 0.243 | 0.144 | 0.41 | 0.49 | 0.955 / 1.3 | 0.915 / 1.2 | 0.188 | 0.169 |
| T20I | runs_conceded | 6 | **0.801** | 0.810 | -0.009 ± 0.018 | **1.869** | 2.980 | 1.870 | 5.51 | 5.96 | 0.902 / 22.0 | 0.813 / 14.8 | 0.298 | 0.311 |
| T20I | catches * | 6 | **0.079** | 0.142 | -0.070 ± 0.153 | **0.112** | 0.226 | 0.121 | 0.34 | 0.45 | 0.952 / 1.0 | 0.901 / 0.9 | — | — |
| ODI | runs | 7 | **0.475** | 0.427 | 0.048 ± 0.013 | **4.590** | 7.961 | 5.110 | 14.47 | 15.92 | 0.904 / 46.9 | 0.723 / 38.8 | 0.342 | 0.312 |
| ODI | balls_faced | 7 | **0.510** | 0.467 | 0.043 ± 0.010 | **5.272** | 9.090 | 5.883 | 16.73 | 18.18 | 0.901 / 56.7 | 0.702 / 45.6 | 0.371 | 0.350 |
| ODI | wickets | 7 | **0.552** | 0.551 | 0.001 ± 0.012 | **0.163** | 0.302 | 0.189 | 0.52 | 0.60 | 0.948 / 1.5 | 0.872 / 1.4 | 0.202 | 0.176 |
| ODI | runs_conceded | 7 | **0.782** | 0.763 | 0.019 ± 0.006 | **2.878** | 4.835 | 3.124 | 8.80 | 9.67 | 0.915 / 34.4 | 0.786 / 23.2 | 0.338 | 0.293 |
| ODI | catches * | 7 | **0.172** | 0.176 | -0.004 ± 0.041 | **0.118** | 0.241 | 0.133 | 0.37 | 0.48 | 0.953 / 1.1 | 0.879 / 1.0 | — | — |
| TEST | runs | 7 | **0.455** | 0.411 | 0.043 ± 0.018 | **8.441** | 14.181 | 9.381 | 26.50 | 28.36 | 0.771 / 82.3 | 0.677 / 71.6 | 0.440 | 0.401 |
| TEST | balls_faced | 7 | **0.477** | 0.437 | 0.040 ± 0.017 | **13.822** | 23.728 | 15.460 | 43.59 | 47.46 | 0.774 / 136.3 | 0.673 / 123.5 | 0.460 | 0.427 |
| TEST | wickets | 7 | **0.775** | 0.735 | 0.040 ± 0.023 | **0.308** | 0.531 | 0.344 | 0.96 | 1.06 | 0.882 / 2.3 | 0.849 / 2.6 | 0.442 | 0.407 |
| TEST | runs_conceded | 7 | **0.837** | 0.820 | 0.017 ± 0.023 | **6.065** | 10.161 | 6.622 | 17.94 | 20.32 | 0.915 / 72.9 | 0.795 / 48.8 | 0.527 | 0.491 |
| TEST | catches * | 7 | **0.403** | 0.413 | -0.010 ± 0.036 | **0.227** | 0.388 | 0.240 | 0.73 | 0.78 | 0.919 / 2.0 | 0.833 / 1.7 | — | — |

**Locked window** (≥ 2025-09-01, scored once). Strict coverage counts y strictly inside
the interval, inclusive counts the ends; with a point mass at zero the nominal 0.80 sits
between them. The last two columns are the per-end exceedances H-5 reads: a level must
sit between them (± 0.03). No target trips it; `recalibrated_targets` is empty everywhere.

| format | target | n rows | Spearman: model | career mean | pinball: model | career mean | career quantiles | median MAE: model | career mean | coverage: strict / inclusive | width | P(y < q90) / P(y ≤ q90) | P(y < q10) / P(y ≤ q10) |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|---:|---|---|
| T20 | runs | 36228 | **0.549** | 0.514 | **2.941** | 5.064 | 3.138 | 9.26 | 10.13 | 0.539 / 0.896 | 29.2 | 0.896 / 0.896 | 0.000 / 0.357 |
| T20 | balls_faced | 36228 | **0.590** | 0.550 | **2.325** | 4.028 | 2.511 | 7.39 | 8.06 | 0.621 / 0.893 | 24.0 | 0.893 / 0.893 | 0.000 / 0.272 |
| T20 | wickets | 36228 | **0.472** | 0.513 | **0.141** | 0.259 | 0.153 | 0.45 | 0.52 | 0.184 / 0.951 | 1.3 | 0.547 / 0.951 | 0.000 / 0.677 |
| T20 | runs_conceded | 36228 | **0.727** | 0.727 | **1.967** | 3.212 | 2.024 | 6.14 | 6.42 | 0.458 / 0.911 | 22.4 | 0.911 / 0.911 | 0.000 / 0.453 |
| T20 | catches * | 36228 | **0.016** | 0.140 | **0.104** | 0.217 | 0.113 | 0.31 | 0.43 | 0.017 / 0.957 | 1.0 | 0.754 / 0.957 | 0.000 / 0.741 |
| T20I | runs | 4004 | **0.575** | 0.561 | **3.313** | 5.562 | 3.499 | 10.47 | 11.12 | 0.548 / 0.892 | 31.7 | 0.892 / 0.892 | 0.000 / 0.344 |
| T20I | balls_faced | 4004 | **0.621** | 0.601 | **2.301** | 3.932 | 2.415 | 7.35 | 7.86 | 0.615 / 0.882 | 23.7 | 0.890 / 0.890 | 0.008 / 0.274 |
| T20I | wickets | 4004 | **0.540** | 0.563 | **0.133** | 0.244 | 0.144 | 0.43 | 0.49 | 0.203 / 0.956 | 1.3 | 0.511 / 0.956 | 0.000 / 0.677 |
| T20I | runs_conceded | 4004 | **0.802** | 0.796 | **1.953** | 3.094 | 1.946 | 5.77 | 6.19 | 0.439 / 0.896 | 22.5 | 0.896 / 0.896 | 0.000 / 0.457 |
| T20I | catches * | 4004 | **0.066** | 0.130 | **0.115** | 0.235 | 0.123 | 0.35 | 0.47 | 0.015 / 0.947 | 1.1 | 0.732 / 0.947 | 0.000 / 0.718 |
| ODI | runs | 8255 | **0.506** | 0.459 | **4.895** | 8.384 | 5.340 | 15.62 | 16.77 | 0.648 / 0.896 | 49.1 | 0.896 / 0.896 | 0.000 / 0.247 |
| ODI | balls_faced | 8255 | **0.566** | 0.516 | **5.352** | 9.235 | 5.891 | 17.15 | 18.47 | 0.731 / 0.900 | 56.7 | 0.900 / 0.900 | 0.000 / 0.168 |
| ODI | wickets | 8255 | **0.592** | 0.587 | **0.157** | 0.291 | 0.176 | 0.50 | 0.58 | 0.233 / 0.954 | 1.5 | 0.536 / 0.954 | 0.000 / 0.641 |
| ODI | runs_conceded | 8255 | **0.808** | 0.792 | **2.932** | 4.802 | 3.074 | 8.86 | 9.60 | 0.477 / 0.914 | 35.2 | 0.914 / 0.914 | 0.000 / 0.437 |
| ODI | catches * | 8255 | **0.182** | 0.193 | **0.123** | 0.248 | 0.136 | 0.39 | 0.50 | 0.043 / 0.947 | 1.1 | 0.738 / 0.947 | 0.000 / 0.695 |
| TEST | runs | 3501 | **0.442** | 0.410 | **8.192** | 13.821 | 8.987 | 25.83 | 27.64 | 0.777 / 0.778 | 81.7 | 0.899 / 0.899 | 0.120 / 0.122 |
| TEST | balls_faced | 3501 | **0.469** | 0.427 | **13.807** | 23.530 | 15.291 | 43.83 | 47.06 | 0.781 / 0.781 | 136.9 | 0.894 / 0.894 | 0.113 / 0.113 |
| TEST | wickets | 3501 | **0.778** | 0.746 | **0.297** | 0.500 | 0.319 | 0.93 | 1.00 | 0.245 / 0.883 | 2.3 | 0.494 / 0.925 | 0.042 / 0.629 |
| TEST | runs_conceded | 3501 | **0.844** | 0.830 | **5.752** | 9.217 | 5.986 | 16.85 | 18.43 | 0.467 / 0.912 | 70.2 | 0.912 / 0.912 | 0.000 / 0.445 |
| TEST | catches * | 3501 | **0.411** | 0.415 | **0.215** | 0.377 | 0.226 | 0.70 | 0.75 | 0.238 / 0.930 | 1.9 | 0.782 / 0.935 | 0.006 / 0.544 |

**Wicket probabilities**, locked window, predicted against observed:

| format | n | Brier (3-way) | P(0): predicted / observed | P(1) | P(2+) |
|---|---:|---:|---|---|---|
| T20 | 36228 | 0.387 | 0.668 / 0.677 | 0.197 / 0.180 | 0.135 / 0.143 |
| T20I | 4004 | 0.369 | 0.663 / 0.677 | 0.195 / 0.173 | 0.143 / 0.150 |
| ODI | 8255 | 0.383 | 0.623 / 0.641 | 0.198 / 0.172 | 0.179 / 0.186 |
| TEST | 3501 | 0.289 | 0.520 / 0.568 | 0.103 / 0.099 | 0.377 / 0.332 |


**Reading it.**

- *The deliverable is where it was expected.* Median MAE for runs improves by 9 % over the
  career mean (9.21 vs 10.17 T20, 14.47 vs 15.92 ODI) — the ≈ 6 % §3 forecast, a little
  more because the population now includes the tail, whose zeros a contextual model
  predicts and a career mean smears. Among the players who did bat, the ranking sits at
  0.33 / 0.34 (T20 / ODI): the ≈ 0.34 ceiling §1 measured for every predictor, now with
  the career mean at 0.31 beside it. The gains are real, small, and mostly in the range
  rather than the point — which is what the intervals are for.
- *Ranking on the unconditional population has an artifact, and it decides wickets.* Six
  of eleven players take 0 wickets and tie; the career mean gives most of them the same
  exact 0 and tie-averaging rewards that, while any model resolves them and is charged for
  it. Runs are barely affected (few unconditional means are exactly 0) and the model wins
  by 0.04–0.05; wickets are dominated by it, so the model ties in ODI and trails by 0.03 in
  T20 in every fold, while beating the career mean by 45 % on pinball and by 0.03 among
  the bowlers. The headline metric stays as defined — H-13 says the consumer's metric, and
  the selector does rank all eleven — and the diagnostic sits beside it; what is *not*
  done is teaching the model to emit exact zeros to win the tie. **P-3's acceptance is
  therefore met for runs and for wickets' pinball, and not met for wickets' Spearman in
  T20**, for a reason that is now written down.
- *Calibration is nominal without recalibration.* P(y ≤ q90) reads 0.89–0.90 for the
  quantile targets in every format and P(y < q10) ≤ 0.12; the inclusive coverage of 0.89–
  0.91 in limited overs is the zero mass (q10 = 0 for a third of rows), not
  over-dispersion, and TEST runs — where almost everyone bats — sit at 0.78 / 0.78. The
  wicket distribution's P(0) / P(1) / P(2+) are within 0.02 of observed. So H-5's mechanism
  ships in place and idle, and the harness will say when that changes.
- *Sharpness (H-22).* The career quantiles are narrower (26.0 vs 29.1, T20 runs) and
  under-cover (0.78 vs 0.90 inclusive); the model's interval is the one that means what it
  says, and its width is the number the next release has to beat without giving that up.
- *Catches* rank badly (0.02–0.07 in the T20 formats against 0.13–0.15 for the career
  mean) — a Poisson(0.4) count with almost no signal, which §1 predicted; the rate is
  reported for the simulator and nothing else.
- *Cost.* The locked-window fit is 4–6 minutes per limited-overs format for three seeds;
  `make xi-evaluate` takes ≈ 2.5 h on both sources together, dominated by the 13
  estimators × 3 seeds × 8 windows the performance section adds.

---

### 8.3 First simulation report (P-4)

The simulator trains nothing, so its report is E2: is it consistent with the display model,
and are its totals calibrated? Two choices were made on the walk-forward folds
(`scripts/experiments/xi/sim_choices.py`, same quarterly origins as §8.1, the performance
model fitted per fold on three seeds, 1,000 draws per fixture) and the locked window was
then scored once by `make xi-evaluate`. Totals are scored on first innings that ran their
course (all out or the legal balls delivered); the chase total on every decided match.
"Ratio" is the dispersion ratio — the spread of actual totals around the simulated mean over
the simulated spread — so 1 is calibrated and above 1 is under-dispersed; the PIT columns are
the share of actual totals below the simulated 10th and above the 90th percentile (0.10 each
when calibrated).

**Chase orientation** (T20 / ODI, 7 folds): feeding the chasing side its chasing forecasts
against its bat-first ones moves the simulated P(win) Brier by 0.0003 / 0.0004 and the
simulated share of bat-first wins by 0.015 / 0.018 (toward the actual 0.48 / 0.44 with the
bat-first forecasts, but inside the fold spread). No effect; the design's default — the
innings as a known feature — stays (`simulator.CHASE_ORIENTATION`).

**Shared match factor** — the design's question, measured before it was added. Without it,
independent batter draws under the balls budget under-disperse the totals; the row pairs
"calibration fit" share the same members (fitted short of the 92-day calibration fold), so
the factor's effect is isolated, and the production fit (all rows, no factor) is what P-3
would have shipped:

| format | simulator | folds | first-innings 10–90 coverage | width | ratio | bias | PIT < q10 / > q90 | chase: coverage / width | margin coverage: runs / balls |
|---|---|---:|---|---:|---|---:|---|---|---|
| T20 | without factor (production fit) | 7 | 0.638 ± 0.046 | 60.6 | 1.42 ± 0.11 | +1.4 | 0.18 / 0.19 | 0.555 / 48.1 | 0.65 / 0.60 |
| T20 | without factor (calibration fit) | 7 | 0.634 ± 0.049 | 60.2 | 1.43 ± 0.12 | +1.2 | 0.18 / 0.19 | 0.549 / 47.8 | 0.66 / 0.60 |
| T20 | **with factor** (calibration fit) | 7 | **0.764 ± 0.083** | 82.8 | **1.02 ± 0.15** | +1.3 | 0.12 / 0.13 | 0.702 / 68.7 | 0.67 / 0.61 |
| ODI | without factor (production fit) | 7 | 0.576 ± 0.058 | 107.3 | 1.36 ± 0.15 | −4.0 | 0.22 / 0.20 | 0.537 / 93.3 | 0.65 / 0.52 |
| ODI | without factor (calibration fit) | 7 | 0.572 ± 0.061 | 107.2 | 1.36 ± 0.16 | −4.3 | 0.22 / 0.21 | 0.542 / 93.3 | 0.64 / 0.52 |
| ODI | **with factor** (calibration fit) | 7 | **0.743 ± 0.068** | 150.6 | **1.02 ± 0.17** | +0.2 | 0.13 / 0.13 | 0.700 / 131.4 | 0.65 / 0.52 |

The factor is sampled from the as-of residual distribution of the calibration fold (T20:
250–400 complete first innings per fold, ODI: 30–190), deconvolved of the simulator's own
dispersion; the deconvolution keeps 53–78 % of the residual (`shrink`), and the resulting
factor sd is 0.10–0.25 by quarter — the residual dispersion itself moves with the season, and
that is where the fold spread of the coverage comes from (T20 per fold with the factor: 0.82,
0.72, 0.73, 0.84, 0.84, 0.60, 0.79). What the factor cannot fix is a level shift between the
calibration quarter and the scored one: the ODI folds carry a per-fold bias of −47 … +35 runs
(40–190 matches each), so the ratio sits at 1.0 while coverage stays under nominal. The
runs–balls copula correlation is 0.92 in both formats. **Decision:** `simulator.SHARED_FACTOR
= True`; the before/after is the table.

**E2 — is the simulated P(win) a probability?** Pre-toss, on the same matches as the display
model (three display seeds averaged):

| format | folds | Brier: simulated | display | Δ (simulated − display) | P(bat-first wins): simulated / actual | ms per fixture (1,000 draws) |
|---|---:|---:|---:|---|---|---:|
| T20 | 7 | 0.2121 | 0.2097 | +0.0024 ± 0.0057 (harness, factor on: +0.0028 ± 0.0057) | 0.521 / 0.482 | 5.4 |
| ODI | 7 | 0.2254 | 0.2211 | +0.0043 ± 0.0143 (harness, factor on: +0.0034 ± 0.0131) | 0.474 / 0.435 | 5.3 |

Within the 0.01 tolerance in both formats (T20I too: −0.0048 ± 0.0120 over 6 folds), so the simulated P(win) is a probability — served
as one by `/simulate`, beside the display model's — but not a better one: the difference is
inside the fold spread, and §3's rule that the display model remains the headline stands
(`simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED` is false for every format, recorded per
format in the harness report as `simulation_decision`). The simulator leans toward the side
batting first by 0.04 in both formats, a bias the shared factor does not touch.

**Locked window** (≥ 2025-09-01, scored once, factor on):

| format | matches | Brier: simulated | toss known | display | base rate | Δ (simulated − display) | P(bat-first wins): simulated / actual | first innings: n | 10–90 coverage | width | ratio | PIT < q10 / > q90 | bias | chase: coverage / width / bias | margin coverage: runs / balls | ms per fixture, 2,000 draws |
|---|---:|---:|---:|---:|---:|---|---|---:|---|---:|---:|---|---:|---|---|---:|
| T20 | 1635 | 0.2024 | 0.2035 | 0.2032 | 0.2497 | −0.0008 | 0.519 / 0.483 | 1521 | **0.786** | 90.6 | 0.98 | 0.13 / 0.09 | −3.6 | 0.733 / 74.3 / −10.3 | 0.66 / 0.60 | 9.9 |
| T20I | 182 | 0.2145 | 0.2172 | 0.2083 | 0.2500 | +0.0062 | 0.511 / 0.495 | 177 | 0.751 | 76.7 | 1.14 | 0.10 / 0.15 | +4.9 | 0.665 / 63.0 / −3.9 | 0.66 / 0.68 | 9.7 |
| ODI | 375 | 0.2201 | 0.2224 | 0.2110 | 0.2474 | +0.0091 | 0.492 / 0.437 | 347 | **0.790** | 154.1 | 1.02 | 0.14 / 0.08 | −8.7 | 0.720 / 132.7 / −15.6 | 0.69 / 0.50 | 9.4 |

The table is the Cricsheet-JSON run; the Postgres run agrees to within the seed spread (T20
0.786 / 90.6 / 0.98 identically, ODI 0.787 / 154.1 / 1.02, T20I 0.763, Brier within 0.0004),
and H-8 parity is 0.0 on both sources, simulator draws included. The acceptance — simulated
totals' 10–90 coverage within ±0.03 of nominal on the locked window for T20 and ODI — is met
with the shared factor on (0.786 and 0.790; without it the
folds say ≈ 0.64 and 0.58), and the PIT is flat to within 0.04 in both. T20I (177 first
innings, which cannot resolve 0.03) sits at 0.751 with a ratio of 1.14: its calibration fold
held 48 matches and its factor sd (0.11) came out half of T20's. The simulated P(win) is
within tolerance on the walk-forward folds in every format (the decision), and on the locked
window it is better than the display model in T20 (−0.0008), worse in T20I (+0.006) and ODI
(+0.009) — all inside the fold spread; the reliability curves of the two track each other
bin for bin. The chase total under-covers (0.72–0.73) from the low side: one in five actual
chases ends below the simulated 10th percentile, the simulated chase runs 10–16 runs high —
collapses and one-sided losses are heavier in the data than in the draws. The margins are
narrower than they should be (runs 0.66–0.69, balls remaining 0.50–0.68 at nominal 0.80),
reported, not tuned.

**Reading it.**

- *The design's measurement came out the way the design said it might, and the fix it named
  worked on the spread:* independent draws under-disperse totals by 40 %, one shared as-of
  factor takes the dispersion ratio to 1.0 and flattens the PIT, and coverage moves from 0.64
  / 0.58 to 0.76 / 0.74 on the folds. The intervals widen from 61 to 83 runs (T20) and from
  107 to 151 (ODI): the narrower interval was the wrong one (H-22).
- *What is left is level, not spread.* Per-quarter bias of the simulated mean — the mean of
  L2-B's forecasts for a quarter's fixtures — is the residual miss, largest where a quarter is
  40 matches of one competition. The plan's route for that is not a wider factor but a
  fixture-conditional level (venue and competition context in L2-B), which is P-5/P-7 work if
  the locked window says it is needed.
- *A design flaw found by measurement before any real data:* drawing a batter's runs and balls
  from one uniform fixed every strike rate and, under the balls budget, collapsed the total's
  spread to a few runs. The copula with the as-of rank correlation replaced it; the test that
  reproduces the collapse stays in the suite.
- *E2 answered:* the simulated P(win) is as calibrated as the display model's, within the
  noise, and is served beside it; the display model remains the headline. The margin
  distributions cover 0.65–0.67 (runs, when the side batting first wins) and 0.52–0.61 (balls
  remaining, when the chaser wins) at nominal 0.80 — narrower than they should be, reported,
  not tuned.
- *Cost:* 5.4 ms per fixture at 1,000 draws; the served default of 2,000 is 9.4–9.9 ms —
  two hundred times under the budget. `make xi-evaluate` gains the simulation section and the
  shared-factor fit per window.

---

### 8.4 P-5 verification run (`make xi-evaluate --postgres`, 2026-09-01)

The harness was re-run end to end on the database as P-5's acceptance. It is the same
command §8.1–§8.3 report, so the numbers are directly comparable, and the point of running
it was to check that what P-5 *ships on* is what the harness *currently measures*.

**H-8 parity holds at zero.** 50 matches, 1,105 player rows, 1,105 performance predictions
and 39 simulations rebuilt through the as-of serving path; max absolute difference 0.0. The
simulator is included at a fixed seed, so the draws agree too.

**The simulator reproduces §8.3.** Locked window: first-innings 10–90 coverage 0.786 at 90.6
runs wide (T20) and 0.787 at 154.1 (ODI), dispersion 0.98 and 1.02. E2 is within tolerance in
both (Δ Brier +0.0018 T20, +0.0062 ODI), so the simulated P(win) is a probability and the
display model stays the headline — unchanged.

**One selection gate is weaker than §8.1 recorded.** Over the seven folds:

| format | matches | objective AUC | display AUC | specific-XI Δ | swap violations |
|---|---|---|---|---|---|
| T20 | 11,948 | 0.684 ± 0.044 | 0.720 ± 0.058 | **+0.012 ± 0.010** | 0.3 % |
| T20I | 2,047 | 0.772 ± 0.042 | 0.763 ± 0.035 | +0.021 ± 0.068 | 0.8 % |
| ODI | 4,945 | 0.670 ± 0.077 | 0.694 ± 0.082 | +0.024 ± 0.069 | 0.0 % |
| TEST | 2,084 | 0.637 ± 0.121 | 0.672 ± 0.093 | **−0.014 ± 0.049** | 0.7 % |

Swap monotonicity passes everywhere, comfortably (H-4's line is 2 %). The specific-XI delta
does not read the way §8.1 does: **+0.012 ± 0.010 in T20**, about 1.2 sd from zero, and inside
its own noise in ODI and T20I, where a quarterly window holds too few matches to resolve a
delta this size. §8.1 recorded +0.045 ± 0.021 for the same quantity; two runs today, from both
sources, agree on 0.012, so the older figure is superseded rather than a source difference.

**What that supports.** That the objective reads the eleven at all, in T20, weakly — the sign
is right and it is the same sign on the locked window (+0.050, which H-19 forbids using for
the choice and which is not used for it here). It does **not** support a claim that
win-probability selection is measurably better than the alternative in ODI or T20I; those
windows cannot resolve it either way. S-6 ships on this reading, and the honest summary is
that the gate that replaced P-0's is *passed but thin*, not passed comfortably.

**TEST is confirmed excluded.** Its delta is negative and its locked display AUC is 0.586,
under H-17's 0.65 line, which is why it is served a rating-ordered XI and told so.

**What was meant to settle it** was E5, the natural experiment (P-7): consecutive matches of
one side with 1–3 lineup changes, asking whether Δobjective agrees with Δoutcome more often
than chance. It has since been run for all three limited-overs formats (§8.6),
on the lineup-only reading that §5's as-played definition turns out to confound: it settles
selection **in favour in T20I** (0.589 ± 0.016, and 0.622 on the confound-free same-opponent
arm), leaves ODI above chance but short of the line (0.521 ± 0.010), and excludes it in T20
(0.509 ± 0.007). It remains the labelled empty slot in the Evaluation report tab, and P-7
should fill it with the lineup-only metric, per format.

### 8.5 P-0's selection gate, re-run with the id contract fixed (2026-09-02)

`scripts/experiments/xi/selection_gate_rerun.py`, 360 decided locked-window matches sampled
120 per format (seed 20260902), 357 with a pool large enough to select from. Every arm's
winner comes from the display model on the two XIs it chose, which is how P-0 scored its arms;
every call carries `as_of` = the match date, so no arm is scored by a state containing its own
result. Constraints are P-0's gate handler's own defaults: team size 11, `min_bowlers` 5,
`require_keeper` false.

| arm | what it is | winner accuracy | mean P(team1) |
|---|---|---|---|
| `winprob` | both XIs by alternating best response on the win objective | 0.622 ± 0.026 (222/357) | 0.497 |
| `ratings` | both XIs rating-ordered, evaluating no model | 0.625 ± 0.026 (223/357) | 0.496 |
| `d7a` | `winprob` with the numeric player id P-0 sent | **no result — see below** | — |
| `fielded` | the XIs that actually took the field | 0.650 ± 0.025 (232/357) | 0.499 |

Per format, `winprob` versus `ratings`: T20 0.644 / 0.636, T20I 0.625 / 0.592, ODI 0.597 /
0.647. Divergence between the two arms is 9.6 players per match out of 22, so the search is
doing something substantial; it is the *metric* that cannot see it.

**P-0's `xi` arm never ran.** Reproducing D-7a exactly — sending `player.id` where the store
is keyed by `player.external_id` — `/xi/optimize` returns `422
OPTIMIZATION_CONSTRAINT_ERROR: "pool cannot satisfy the constraints (size / bowlers / keeper)"`
on **every one of the 357 matches**, and did so identically before P-5: with every player
unrated, `_Pool.is_bowler` reads `exp_balls_bowled` as 0 for the whole pool, so `_greedy_seed`
can never reach `min_bowlers`. go-app then took the branch below it —

    slog.WarnContext(ctx, "xi selection unavailable, falling back to the windowed-form win model", ...)

— so "xi 0.560 vs greedy 0.569" was the **windowed-form** best-response optimiser versus
greedy, both scored by the XI display model reading eleven debutants a side, which leaves only
team and venue context. The number is not a depressed measurement of the xi arm; it is a
measurement of two other arms under its label.

**And the corrected number does not overturn P-0's conclusion either, because the gate still
cannot answer the question.** `winprob` and `ratings` are one match apart out of 357 — inside
noise by any reading — while the *fielded* XIs, the only asymmetric arm, score highest at
0.650. That is the mis-specification §6 already named, now visible in the numbers: both
selection arms optimise **both** sides, which moves the fixture toward parity (mean P(team1)
0.497 and 0.496, against 0.499 for the real teams), and an arm that moves a fixture toward
parity must lose winner accuracy however good its XIs are. Winner accuracy scores the
*fixture*, not the selection.

So P-5's decision stands on the same ground it already stood on: S-6 ships on L4's replacement
gates (§8.4), not on this one, and the right resolution of the question is E5 (P-7), which is
the only gate that varies one side at a time. What changes is the standing of the old figure —
§8.1's "did not beat greedy on the P-0 gate" should be read as **unmeasured**, not as evidence
against win-probability selection.

**The `greedy` arm is not recoverable.** It scored players on the per-player batting, bowling
and fielding models P-5 deleted, so there is nothing left to run it with. `ratings` stands in
for it: the same pool and the same constraints, differing from `winprob` in nothing but whether
the objective is consulted — a cleaner contrast than the original, which varied the features
and the objective at once.

### 8.6 E5 for all three limited-overs formats (2026-09-02)

`scripts/experiments/xi/e5_natural_experiment.py`. Consecutive-match pairs of one club with
1–3 lineup changes, every call carrying `as_of` = the match date. The decision runs on the
development pairs; the locked window is scored beside them and labelled (H-19). Zero call
failures in all three runs.

**The metric as §5 specified it is confounded, so it is not the one to read.** Among the pairs
E5 scores there is an identity: a nonzero Δresult forces Δresult = −1 **iff** the club won
match k. So "sign agreement between Δobjective and Δresult" asks whether Δobjective
anti-correlates with having won the previous match — and match k+1's as-of state contains
match k's result, so the rating update pushes Δobjective the other way. The same-opponent
as-played column below is that artifact with the opponent variation stripped out: 0.349 in
T20 and 0.353 in ODI, both far *below* chance. Across all pairs the artifact is diluted by
genuine opponent-strength variation — which the objective does price, and which is the AUC we
already have — and the two opposing biases net out near 0.57–0.59 in every format.

**Read the lineup-only columns.** They score both elevens in the same fixture at the same
as-of, so nothing varies but the eleven — the thing selection actually does.

| format | development pairs | lineup-only | 95 % CI | same opponent, lineup-only | as-played (confounded) | same opponent, as-played |
|---|---|---|---|---|---|---|
| T20 | 12,413 | 0.509 ± 0.007 (n=5,504) | [0.496, 0.522] | 0.491 ± 0.024 (n=430) | 0.570 ± 0.007 | 0.349 ± 0.023 |
| ODI | 5,882 | 0.521 ± 0.010 (n=2,568) | [0.502, 0.541] | 0.517 ± 0.019 (n=712) | 0.561 ± 0.010 | 0.353 ± 0.018 |
| **T20I** | 2,185 | **0.589 ± 0.016 (n=890)** | **[0.556, 0.621]** | **0.622 ± 0.025 (n=381)** | 0.585 ± 0.017 | 0.446 ± 0.025 |

Locked window (labelled, never used for the choice), lineup-only: T20 0.520 ± 0.018 (n=784),
ODI 0.572 ± 0.031 (n=250), T20I 0.514 ± 0.048 (n=109); same-opponent lineup-only 0.564, 0.547,
0.585.

**The answer is format-specific, and it is not the one the plan expected.**

- **T20I passes.** 0.589 with 0.55 below the interval's floor, and the confound-free
  same-opponent arm is *stronger* at 0.622 [0.573, 0.671] rather than weaker — which is what a
  real effect should do when you remove a bias that was pulling against it, and what a fluke of
  one slice should not. Its same-opponent as-played figure, 0.446, is also the least depressed
  of the three, i.e. the format where genuine signal most nearly offsets the artifact. E5 is
  met in T20I: **the objective is selecting on real signal there.**
- **ODI fails, narrowly and consistently.** 0.521 [0.502, 0.541] is above chance by two
  standard errors but the 0.55 line sits outside the interval; the same-opponent arm agrees at
  0.517. There is *something* there, and it is smaller than E5 asked for.
- **T20 fails.** 0.509 [0.496, 0.522] — 0.55 excluded rather than missed, and the
  same-opponent arm is 0.491. This is the format with by far the most pairs, so it is the
  best-powered null of the three.

**The effect size is the same everywhere, so the difference is accuracy, not ambition.** The
objective claims a median |Δ| of 0.020–0.022 win probability for a 1–3 player change in all
three formats (p90 0.065–0.073). T20I does not pass because the objective claims more there;
it passes because what it claims is more often right. That ordering — T20I best, ODI
middling, T20 worst — only partly tracks the objective's holdout AUC on Postgres (T20I 0.74,
T20 0.72, ODI 0.68, P-0): T20I is top in both, but T20 and ODI swap. It is a better match for
how much of a "1–3 player change" is a change in strength at all — in domestic T20, much of it
is squad rotation.

**Where that leaves S-6.** P-5 shipped the XI path as the only selection path for
T20 / T20I / ODI on swap monotonicity and the specific-XI delta, before this run. E5 now says
that decision is **earned in T20I, thin in ODI and unsupported in T20**. Nothing here argues
for restoring a flag — the greedy arm it would switch back to was deleted in P-5, and the
display model is not in question — but the plan should stop describing selection as settled
across limited-overs formats. The accurate statement is: the objective is a good win model
everywhere it is served, a demonstrated selector in T20I, and an unproven one in T20 and ODI.

**What P-7 should implement** is the lineup-only metric, per format, not §5's as-played
definition. The E5 slot in L4's report is still the labelled empty one; this is an experiment
script, and wiring it in is P-7's.

**The rotation reading above was tested and is not supported (X-3, 2026-09-05).** "In
domestic T20, much of a 1–3 player change is squad rotation" was a hypothesis with a
measurable consequence: strip the fixtures where rotation is heaviest and the agreement
should rise. `ml/xi/stakes.py` derives a stage label for 96.5 % of matches and a dead-rubber
flag for the 72.3 % where a table is reconstructible as-of, from Cricsheet's own event
fields; `scripts/experiments/xi/x3_match_stakes.py` re-ran E5 with the suspect pairs excluded
and, separately, halved. The suspect pairs are 15.5 % (dead rubber) and 12.1 % (knockout) of
T20's pooled pairs, so the treatment has bite — and **all five arms fail their re-derived
bars, four of them below the control** (control 0.5030; excluding dead rubbers 0.5014,
excluding knockouts 0.4976, excluding both 0.4944, down-weighting both 0.4993). The T20 null
is not rotation noise. Fold tables in [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-3;
the control there reproduces this section's walk-forward figures exactly on the pre-A-4
window.


### 8.7 What §8.5 and §8.6 set up for P-6 and P-7

Four things follow from the two runs above. None of them is P-5's — it moved consumers and
deleted — and none changes shipped behaviour; they are what the next two PRs inherit.

**1. P-7 implements the lineup-only E5 metric, per format, in L4.** Definition: for each
consecutive pair of one club's matches with 1–3 lineup changes, score *both* elevens against
match k+1's opponent at match k+1's as-of, and report sign agreement with Δresult over the
pairs whose result moved. Not §5's as-played definition, which §8.6 shows is confounded.
`scripts/experiments/xi/e5_natural_experiment.py` is the reference implementation and the
figures to reproduce. The report's E5 slot stays labelled and empty until then.

**2. P-7 decides whether selection is scoped by E5 the way serving is scoped by AUC (H-17).**
E5 demonstrates the objective as a selector in T20I only. The mechanism to scope it already
exists and already ships: `objective: "ratings"` returns the rating-ordered eleven under the
same constraints, evaluating no model, labelled `optimised: false` — which is exactly what
TEST gets. So "turn optimisation off for T20 and ODI" is a one-line policy change, not new
code. It is a real decision with a real trade-off and P-5 does not pre-empt it:

- *For scoping it off in T20:* lineup-only agreement is 0.509 [0.496, 0.522]; the objective's
  preference among elevens is not distinguishable from chance at predicting outcomes, and the
  search costs a best-response loop per request.
- *Against:* 0.509 is not *below* chance, and §8.4's specific-XI delta is +0.012 ± 0.010 —
  positive on both gates, just small. Turning it off would also mean the headline P(win) and
  the selected XI stop coming from the same model, which is a coherence cost the plan has
  otherwise paid to avoid.
- ODI sits between: 0.521 [0.502, 0.541], above chance and short of the line.

**3. E5's 0.55 threshold should be re-derived from the measured effect size.** §5 set it with
no estimate of how much the objective claims a lineup change is worth. It claims a median |Δ|
of 0.020–0.022 win probability for 1–3 players, in every format. A threshold for sign
agreement should follow from that number and the sample available, not precede it — otherwise
a format can fail a bar that was never reachable. T20's null is trustworthy because it is
well-powered (5,504 scored pairs); ODI's near-miss and T20I's pass should both be re-read
against a threshold derived this way.

**4. H-23 is the general form of the lesson**, and it is P-7's to enforce: a gate must state
what varies between its arms and what is held fixed. All three failures in §8.5 and §8.6 —
scoring the fixture, scoring mean reversion, and reaching neither named arm — would have been
visible at design time under that rule.

For **P-6**, nothing here changes the scope: it still removes the producers, and it still
carries D-6 (§10.5). What §8.5 adds is a reason to keep the fallback it will inherit honest —
go-app silently falling back from a refused `/xi/optimize` to another optimiser is what let a
broken arm report a number for months. A fallback that changes which model answered should say
so in the response, not only in a log line.

**Done in P-6.** The audit found three substitutions on the prediction path and one
non-substitution. `selection` already named the first (optimised vs rating-ordered, H-17) and
`win_probability.source` the second (display vs simulator, E2). The third was invisible: a
format with no innings length gets L2-B's own quantiles instead of the simulator's draws, and
the only sign was a `scorecard` that quietly was not there — it now has a `forecast` block
naming the source and saying why. The fourth was not a fallback at all but a silent zero-fill:
a player the simulator or the performance model returned no line for was logged and left at
zeros, which reads on screen as a forecast of nothing rather than as a missing forecast. That
one is now an error, on the rule that a substitution which cannot be labelled must not be made.

### 8.8 P-7: E5's bar re-derived, the selection scope decided, E3 answered (2026-09-02)

Three questions §8.7 left for this PR, in the order they have to be answered: what bar E5
can reach at all, whether each format clears it, and whether batting order is worth
optimising. Every number below is one `make evaluate` reproduces, and each gate's
*varies / fixed / decides* triple is in `ml/xi/gates.py` (H-23), stated before the gate ran.

**Where the metric lives.** `ml/xi/natural_experiment.py` is E5, lineup-only, in L4: for
each consecutive pair of one side's matches with 1–3 lineup changes, both elevens scored
against match k+1's opponent at match k+1's as-of — the previous eleven read from the as-of
serving path (`AsOfRatings`) in one advancing pass over the source, the fielded eleven from
the frame's own row, the two checked against each other as a parity check on the pairing
(max abs difference **0.0** over 25,650 pairs). §5's as-played form is not computed, and the
report's slot says why in one line. **The wiring test** (`scripts/experiments/xi/e5_reproduce.py`)
scores the same pairs through the same as-of path with one run's objective, and lands on
§8.6 exactly — the same 12,413 / 5,882 / 2,185 development pairs, the same 5,505 / 2,568 /
890 scored, the same 2,801 / 1,339 / 524 agreed: **T20 0.509 ± 0.007, ODI 0.521 ± 0.010,
T20I 0.589 ± 0.016**, and the same locked-window figures (0.520 / 0.572 / 0.514).

**1. The bar, derived.** §5 set 0.55 with no estimate of what the objective claims a lineup
change is worth. It claims a median |Δ| of **0.022 / 0.020 / 0.022** win probability (mean
0.029 / 0.029 / 0.032, p90 0.065–0.073) for a 1–3 player change in T20 / ODI / T20I. Take the
objective at its word: every fixture's result is Bernoulli at the objective's own P(win) —
match k's at its as-played probability, match k+1's at its as-played probability, so the
lineup-only Δ is exactly what the objective says it is and the opponent change between the
two matches is priced the way the objective prices it. The sign-agreement rate an
exactly-right objective would then produce has a closed form (per pair, P(result moves up) =
(1−a)·b and P(down) = a·(1−b); agreement is the one the claimed sign picks, over the pairs
that move), and 2,000 Bernoulli replicates (seed 20260902) give its sampling distribution for
the pairs available. The bar is that distribution's **5th percentile** — the number minus
sampling noise, so an exactly-right objective falls below it one run in twenty:

| format | pairs (dev) | exactly-right agreement | simulated sd | **derived bar** | observed | verdict |
|---|---|---|---|---|---|---|
| T20 | 12,392 | 0.523 | 0.006 | **0.512** | 0.509 ± 0.007 | **fails**, by 0.003 |
| ODI | 5,882 | 0.520 | 0.010 | **0.504** | 0.521 ± 0.010 | **passes**, at the exactly-right figure |
| T20I | 2,182 | 0.522 | 0.016 | **0.496** | 0.589 ± 0.016 | **passes**, 4 se above exactly-right |

Two things follow. **0.55 was never reachable.** A 2 pp lineup effect implies an agreement
rate of about 0.52, in every format; the objective's opponent pricing, which is real, does
not help here because both matches in a pair move the fixture probability by more than the
lineup does and in unrelated directions. §5's threshold asked the objective to be more
right than the effect it claims could ever show. **And the derived bar flips one reading and
sharpens another.** ODI, which §8.6 called a narrow fail, agrees with the result *exactly as
often as an exactly-right objective would* (0.521 against 0.520) — it passes, and there is
nothing left for it to gain on this gate short of claiming a larger effect. T20, the
well-powered null, fails the derived bar by 0.003 — half a standard error — and sits 2.1 se
below the exactly-right figure: the objective's direction on a T20 lineup change is not
distinguishable from chance (0.509 is 1.3 se above 0.5), and it is *less* right than its own
claims. T20I is right far more often than its claimed magnitudes imply (0.589 against 0.522,
4 se), which says its lineup deltas are too *small* there — a shrunk effect with the right
sign, not a lucky slice; the same-opponent arm in §8.6 (0.622) said the same.

Locked window, labelled and never used for the choice: T20 0.520 ± 0.018 against a bar of
0.494, ODI 0.572 ± 0.031 against 0.498, T20I 0.514 ± 0.048 against 0.436 — all above their
bars, all inside two standard errors of the development figures.

The figures above are the well-powered ones, and they are in-sample for the model's
*weights*: the run's objective is fitted through its cutoff on rows including these pairs
(the ratings are as-of either way). The choice-facing figure is the harness's walk-forward
one (H-19), where each pair is scored by the objective fitted before its fold's cutoff, on the
2024-01 → 2025-09 pairs only, with the bar re-derived for that many pairs:

| format | fold pairs | scored | walk-forward agreement | per-fold mean ± sd | exactly-right | **derived bar** | verdict | locked (labelled) |
|---|---|---|---|---|---|---|---|---|
| T20 | 3,202 | 1,358 | **0.490 ± 0.014** [0.463, 0.516] | 0.492 ± 0.030 (7) | 0.522 | **0.501** | **fails** | 0.520 ± 0.018 vs 0.494 |
| ODI | 941 | 429 | **0.562 ± 0.024** [0.515, 0.609] | 0.569 ± 0.078 (7) | 0.525 | **0.487** | **passes** | 0.572 ± 0.031 vs 0.498 |
| T20I | 320 | 111 | **0.586 ± 0.047** [0.494, 0.677] | 0.571 ± 0.104 (6) | 0.539 | **0.469** | **passes** | 0.514 ± 0.048 vs 0.436 |
| TEST | 339 | 157 | 0.529 ± 0.040 [0.451, 0.607] | 0.524 ± 0.081 (7) | 0.528 | 0.463 | passes — moot, off under H-17 | 0.625 ± 0.065 vs 0.425 |

The walk-forward verdicts are the in-sample ones, format for format. T20 is below a coin
flip on the folds (five of seven under 0.52, the 2024-10 fold at 0.439, three-player changes
at 0.417) and fails a bar of 0.501; ODI and T20I clear their bars with room and again sit above
the exactly-right figure. T20I has only 111 scored pairs in the fold windows (one fold has too
few rows to fit an objective at all), so its walk-forward interval is wide; the 890-pair
development figure is the one that carries its pass. TEST passes E5 at low power and stays
off: H-17 and E5 are two rules, and an objective whose AUC does not clear 0.65 is not searched
over whatever E5 says of it. The report carries every one of these numbers under
`e5_lineup_only` — per fold, pooled, effect size, bar, locked window — with the gate's triple
beside them; the harness run was 70 minutes contended with two other jobs, of which the E5 pass
is under three.


**2. The scoping decision.** §8.7 point 2 asked whether selection is scoped by E5 the way
serving is scoped by AUC (H-17), and listed what to weigh: T20's well-powered null and the
cost of the search, against the positive-but-small specific-XI delta (+0.012 ± 0.010, §8.4)
and the coherence of the XI and P(win) coming from one model. With the derived bar in hand
the decision is per format, and it is driven by that bar, not by the old 0.55:

- **T20I — optimised selection stays on.** Passes the derived bar on the walk-forward folds
  (0.586 against 0.469) and on every development pair (0.589 against 0.496), and in both it
  sits *above* what an exactly-right objective would score. The objective is a demonstrated
  selector here; §8.6 already said so and the derived bar agrees.
- **ODI — optimised selection stays on.** §8.6 read it as a narrow fail against 0.55; against
  the bar it can reach it passes on the folds (0.562 against 0.487) and on every development
  pair (0.521 against 0.504, with the exactly-right figure at 0.520). The objective agrees
  with the result exactly as often as its own effect size implies. The old reading was the
  threshold's fault, not the format's.
- **T20 — optimised selection is scoped off.** It fails the derived bar on the walk-forward
  folds (0.490 over 1,358 pairs against 0.501; below a coin flip, though not significantly)
  and on every development pair (0.509 over 5,505 against 0.512, 2.1 se under the
  exactly-right 0.523). This is the best-powered E5 of the three, and it fails a bar the
  objective could in principle reach. Weighed against it: the specific-XI delta is positive
  but 1.2 sd from zero; the search is a best-response loop per request whose output E5 cannot
  distinguish from the rating order; and coherence is kept where it matters — the headline
  P(win) still reads the fielded eleven through the display model, only the *choice* of
  eleven stops consulting the objective. Rating order is not a downgrade E5 can see: it is
  the search's own seed, under the same constraints. Domestic T20 is also where §8.6 noted
  that a "1–3 player change" is most often squad rotation rather than a change in strength,
  which is a reason the signal is weakest there and not a reason to keep searching for it.

So the rule is now **two rules in one map**, `ml.xi.optimizer.NOT_OPTIMISED_REASONS`, mirrored
by go-app's `notOptimisedReasons`: a format is offered an optimised selection unless its
objective does not rank (H-17: TEST) or has not shown it selects (E5: T20), and the reason
travels — `/xi/optimize` refuses `objective: "win"` for a listed format with the reason in the
503, go-app serves the rating-ordered eleven with `optimised: false` and the reason as the
`selection.note`, and the Upcoming-match tab shows it as the *Not optimised* notice, exactly as
TEST's. The harness **revisits the decision every run**: per format the report states
"optimised selection: yes/no, because E5 said X against bar Y" with the served policy read
from the map beside the verdict, so a run whose verdict disagrees with the policy says so in
the report and in the Evaluation tab. The policy itself is set by hand from the report, as E2's
is. Covered by tests both ways in ml-service (T20 refused with its reason, T20I optimised, the
map and the optimised set partition the formats) and go-app (T20 rating-ordered with its note,
T20I searched against the opposing eleven, every reason names its rule).

What this does *not* say: that win-probability selection is wrong in T20, or that the
rating-ordered eleven is better. E5 cannot see a difference between them there, at the best
power this dataset has, and the search costs a loop per request; the cheaper arm is served
until a run of the harness shows the other one selecting. The number to move is the T20
objective's lineup-only agreement, and the bar it has to clear is now written down beside it.


**3. E3 — batting order stays with the captain.** `scripts/experiments/xi/e3_batting_order.py`,
gate stated before it ran (H-23): *varies* the order of the top seven batting slots; *fixed*
the eleven, the opponent eleven, the as-of date, batting first, and the random numbers;
*decides* the share of elevens whose best sampled order moves the confirmed simulated median
first-innings total by more than 3 %, against §5's 30 % line. Method: 100 fixtures sampled
per format and window (seed 20260902), both fielded elevens each, the expected-slot order as
the baseline; **N = 64** random permutations of the top seven (identity included), each
reassigning the seven's slot *values* among the seven so L2-B reads the same feature set with
different holders — the tail untouched — re-predicted through L2-B and simulated batting
first at 1,000 draws with common random numbers; the best order by search median then
**re-simulated with a fresh seed at 4,000 draws** beside the baseline, because the best of 64
noisy medians is biased upward by the search itself, and the baseline re-simulated once more
for the noise floor. The decision runs on the last walk-forward fold's fixtures
(2025-06-01 → 2025-09-01, performance model fitted before its cutoff); the locked window is
scored beside it with its own pre-locked fit and labelled (H-19).

| format | window | elevens | confirmed Δ median | p90 | max | share > 3 % | share > 1 % | share > 5 % | noise floor p90 |
|---|---|---|---|---|---|---|---|---|---|
| T20 | development (decides) | 200 | 1.7 % | 4.7 % | 18.3 % | **25.0 % ± 3.1** | 74 % | 9 % | 1.1 % |
| T20 | locked (labelled) | 180 | 1.7 % | 5.1 % | 21.3 % | 19.4 % ± 2.9 | 67 % | 11 % | 1.2 % |
| ODI | development (decides) | 199 | 1.5 % | 3.2 % | 4.6 % | **12.6 % ± 2.3** | 68 % | 0 % | 1.0 % |
| ODI | locked (labelled) | 199 | 1.6 % | 3.2 % | 5.6 % | 10.6 % ± 2.2 | 73 % | 2 % | 1.1 % |

**Neither format crosses the line.** Reordering the top seven moves the simulated median total
by a median of 1.5–1.7 % (about 2.5 runs on a T20 total of 158, 4 runs on an ODI 262), by more
than 3 % for **25 % of T20 elevens and 13 % of ODI elevens**, against 30 %; by more than 5 %
for 9 % of T20 elevens and none in ODI. The search-only share was 27 % / 19 % — the
confirmation step removed a winner's curse worth 2–6 points of share, which is what it was
there for — and the noise floor (the same order re-simulated) sits at a p90 of 1.1 %, so the
3 % line is well clear of Monte Carlo error. T20 is the closer call: 25 % ± 3 % is inside two
standard errors of 30 %, and the tail is real (one eleven in ten gains 5 % or more, once 18 %),
which is the profile of a few elevens with a badly placed hitter rather than a general
sensitivity. Per §5 the outcome is **recorded and order stays with the captain**; nothing is
added to L3, and the simulator keeps ordering by expected slot. A caveat that bounds the
number rather than moves it: the counterfactual runs through L2-B's response to
`exp_bat_position`, which is a correlational feature — a tail-ender promoted to open is
forecast the way players who *usually* open are forecast, which if anything overstates what
reordering can do. The distribution and every eleven's best order are in
`output/e3_batting_order.json`; the run is reproducible from the seed.


---

### 8.9 A-1: fixture-conditional level in the performance model (2026-09-02)

The row in `docs/FOLLOW_UP_PLAN.md` § 4. This section was written **before the run**: the
design, the leakage surface and the gate's H-23 triple first, the tables after. The tables
are appended below the design under *Results*; nothing above that heading was edited once
a number existed. (The design was first written against the pre-rotation window and
revised, still before any run, once A-4 had rotated it: eleven folds rather than seven,
and a locked window that is empty by construction — see the last paragraph before
*Results*.)

**The evidence it chases (§8.3).** The shared factor took the simulated totals' dispersion
ratio to 1.0 and the PIT flat; what it cannot fix is a level shift between the calibration
quarter and the scored one. The ODI folds carry a per-quarter bias of the simulated
first-innings mean of −47 … +35 runs, largest where a quarter is forty matches of one
competition; on the locked window the chase total runs 10–16 runs high. The simulated mean
is the mean of L2-B's forecasts for the quarter's fixtures, so the level error is the
performance model's, and the route §8.3 named is a fixture-conditional level: something on
the row that says *this ground scores under par* or *this competition is a second-tier
one-day cup*.

**What L2-B can see today, and what it cannot.** A player's rates are relative to the
(format, over) context baseline, so they are venue- and era-neutral in the mean by
construction (`ratings.py`, module docstring). The only fixture-level inputs are
`venue_bf_rate` and `venue_n` (how often the side batting first wins at the ground, and on
how many matches), `elo_edge`, and the two sides' aggregates. Nothing on the row carries
the *scoring level* of the ground or of the competition; a side of average players at a
ground where 140 is par is forecast the same total as at one where 190 is.

**Design: two families, two columns each, all as-of.**

| family | column | definition |
|---|---|---|
| venue | `venue_run_rate_rel` | shrunk runs per delivery at (format, venue), over the format's as-of runs per delivery |
| venue | `venue_wicket_rate_rel` | the same for dismissals per delivery |
| competition | `competition_run_rate_rel` | shrunk runs per delivery in (format, competition), over the format's as-of runs per delivery |
| competition | `competition_wicket_rate_rel` | the same for dismissals per delivery |

For a key *k* (a venue or a competition, within a format) with as-of sums *R_k*, *W_k*,
*B_k* — runs, dismissals and deliveries over every ball of every match under that key
folded in before the match's date — the run-rate column is

    ((R_k + P · r̄) / (B_k + P)) / r̄  =  1 + (R_k − B_k · r̄) / ((B_k + P) · r̄)

where *r̄* is the format's as-of runs per delivery (the context baseline the impact ratings
are measured against, summed over overs: `ctx_runs / ctx_balls`, the unsplit group), and
*P* = `contract.FIXTURE_CONTEXT_PRIOR_BALLS` = 600 deliveries — five T20 innings, two ODI
innings. The wicket column is the same with *W_k* and the format's as-of dismissals per
delivery. The code computes the right-hand form — the key's runs above the format's
expectation, shrunk, over the expectation, the shape the impact ratings already have — so
a key the state has never seen, or a fixture that names none, reads **exactly 1.0** in
floating point: the shrinkage target at zero deliveries, which is also what a serving
request reads when it names no venue or competition. The neutral value is a property of
the formula, not a second code path; and the read uses `dict.get`, so a fixture naming an
unseen ground does not write a key into the serving state (the D-7 class; B-1 in
`docs/BUG_BACKLOG.md` records that `team_context` still does).

*Why relative rather than absolute.* T20 scoring has risen across the archive. An absolute
runs-per-ball at a ground whose history is 2012 would read as low against a 2025 side, and
the model would learn era, not venue. Numerator and reference are both lifetime as-of sums,
so the ratio drifts with neither; it is the stationary quantity, and it is the same shape
the impact ratings already have (above expectation, not absolute).

*Why the competition's scoring rate rather than a tier.* A tier is a hand-made mapping over
the 1,386 distinct (match type, event name) pairs in the archive, maintained forever, and
what the model needs from it is exactly the level — which the as-of rate carries without a
table. A competition's identity enters as its own scoring history: a first season reads as
neutral (no history yet), and a rebrand ("NatWest T20 Blast" → "Vitality Blast" →
"Vitality Blast Men") starts a fresh accumulator. That is the limitation team identity had
before I-4, and if the family is kept and the fold spread says the rebrands matter, the same
fix (a lineage file under `configs/`) applies. Not built here.

*Keys.* Venue is (format, venue) as `venue_bat_first` already keys it — the venue string in
the archive, the venue id in Postgres. Competition is (format, event name): `info.event.name`
in the archive and `match.event_name` in Postgres, the same string on both sources (the
importer stores the name verbatim), which is what lets the two sources agree on the column
value as they do on every other. An **unnamed key is no key**: a match without a venue or an
event accumulates under neither and reads 1.0, on both sources — which needed one change to
the Postgres source, whose match query used to spell a missing `venue_id` as `0` where the
archive spells it as the empty string (the archive currently has no such match; the
database is what a future import writes). Gender is not in either key: the team key
carries it (I-3) and the eleven's aggregates carry the eleven's level, and H-7 measured the
unsplit context baseline as costing nothing. Recorded as a limitation: a women's match at a
men's ground reads the ground's blended rate.

*No decay, no tuning.* Pitches are relaid, but slowly; `venue_bat_first` does not decay
either, and a lifetime ratio against a lifetime reference is the stationary quantity. The
prior is one number, chosen by the size of an innings and not swept (H-6's finding was that
the pass's priors are not load-bearing; if A-1 is kept, the sweep can include it).

**Where it is computed, and where it flows.** `RatingState.fixture_context(match)` reads
the four columns from the state — before `update`, exactly as `team_context` is read — and
`update` accumulates the match's deliveries under both keys once the rows are built, at day
close (H-18). `ml.xi.rows.player_feature_rows` puts the columns on the win row and on every
player row through the one assembly training and serving share (H-8); the serving request
already names a venue, and names no competition, which reads neutral. The frame always
carries the columns — as it carries the sequence families, measured and then kept or not —
and `contract.FIXTURE_CONTEXT_FAMILIES_KEPT` records the decision that
`performance_feature_cols` reads. The simulator is untouched: it consumes the forecasts,
and a forecast that knows the ground is a different forecast. The shared factor is refitted
per arm on the calibration fold, so its residual distribution is the residual *after* the
arm's level.

**H-21 audit — the leakage surface of the four columns.**

1. *The match's own result.* Read before `update`, at day close: the match's deliveries and
   every same-day match's are absent from the accumulators (H-18). The unit test that
   guards H-1, `test_features_are_as_of_and_never_see_their_own_match`, is extended to the
   four columns — identical with and without later matches, and the match's own deliveries
   and result do not move its row.
2. *Future matches.* None can enter: the pass is chronological and `AsOfRatings` folds by a
   strict date threshold. The H-8 parity check compares both code paths on the new columns
   (they sit in `PLAYER_MATCH_FEATURE_COLS`, which parity iterates, and the win-row list is
   extended with `FIXTURE_CONTEXT_COLS`).
3. *Outcome columns.* No target column enters; `performance_feature_cols` is what the model
   reads and `test_performance_feature_cols_never_include_an_outcome_column` covers the new
   columns without change. Both innings' deliveries feed the accumulators, the chase
   included; neither the numerator nor the reference is a function of who won.
4. *Scoreboard read-through.* A ground with one prior T20 reads that match's rate at weight
   240 / 840 ≈ 0.29; a ground with a season's history reads its own. Never the fixture's own
   scoreboard. The H-2 canary sweeps `DISPLAY_FEATURE_COLS`; the new columns are not win
   features (both sides play at the same ground) and are not added to it.
5. *In-sample stacking.* The columns are sums of raw deliveries, not a model's outputs;
   nothing is fitted to produce them. The one fitted consumer of L2-B, the shared factor,
   keeps its temporal fold.
6. *Serving.* A live request reads the same function of the same state; a venue it names is
   keyed as training keys it, and a competition it does not name reads 1.0 — the value a
   fixture with no history reads in training, so serving never sees a value training could
   not have produced.

**H-23 triple — gate A-1** (`ml.xi.gates`, registered before the script ran; the script
prints it first):

- *varies:* which fixture-context families the performance model reads — none, venue,
  competition, both — one fit per arm per fold.
- *fixed:* the rows, the eleven quarterly cutoffs (A-4's rotated set, 2024-01 … 2026-06),
  the three seeds, the hyperparameters, the shared factor's fitting rule (the 92-day
  calibration fold the members do not train on), the display models (fitted once per fold,
  shared by the arms), the simulator, its draw count and its seeds (common random numbers
  across arms), the labels.
- *decides:* a family is kept only if, against the no-context arm on the same folds, the
  mean over folds of |bias| of the simulated first-innings mean shrinks, the first-innings
  10–90 coverage stays within ± 0.03, the interval's width does not grow (H-22), and the
  pinball loss of every headline target is no worse by more than 0.5 % (E1's noise band) —
  in **both** T20 and ODI, the formats the evidence names and whose folds can resolve it.
  The `both` arm is shipped only if it passes the same test; T20I is reported, not decided
  on (ten folds of 23–57 matches); TEST has no simulator and is reported for pinball only.
  A recorded null — no family shrinks the bias — ships no feature. The script is
  `scripts/experiments/xi/a1_fixture_context.py`; it prints the triple first and writes
  one JSON per format under `output/ml-service/a1/`.

**The locked window, after the choice.** A-4 rotated the window to ≥ 2026-09-02 before this
item started, and the database ends 2026-09-01, so the window holds **no matches**: `make
evaluate` scores it once with the decided configuration and reports `n_eval: 0`, which is
the correct answer (`ml-and-training.md` § Rotating the locked window), not a number. The
before/after this section can honestly report is therefore the harness's walk-forward
table — A-4's baseline in `docs/FOLLOW_UP_PLAN.md` § 5 against the same run with the decided
configuration — on the same eleven folds, the same source (the database) and the same
seeds; the locked-window line is reported as empty in both. The evaluate run's other job
is H-8: the parity check rebuilds the last 50 matches through the as-of path with the four
new columns in both the win row and every player row, on both sources.

**Results** (`scripts/experiments/xi/a1_fixture_context.py`, run 2026-09-02/03 on the
archive; eleven folds 2024-01 … 2026-06, three seeds, 1,000 draws per fixture, the display
models and the simulator's random numbers shared by the arms). The gate's table, one row
per arm and format, means over folds; "verdict" is the registered rule applied per format:

| format | arm | folds | mean \|bias\| | mean bias | coverage | width | chase bias | chase coverage | runs pinball | wickets pinball | balls pinball | conceded pinball | Δ Brier | verdict |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| T20 | none | 11 | 3.1 | +0.5 | 0.770 | 84.7 | −6.6 | 0.713 | 2.920 | 0.1413 | 2.320 | 1.935 | +0.0018 | control |
| T20 | venue | 11 | 3.2 | +0.5 | 0.770 | 84.6 | −6.5 | 0.713 | 2.920 | 0.1412 | 2.320 | 1.933 | +0.0016 | fails: \|bias\| does not shrink |
| T20 | competition | 11 | 3.1 | +0.5 | 0.767 | 84.4 | −6.6 | 0.709 | 2.920 | 0.1411 | 2.320 | 1.933 | +0.0014 | fails: \|bias\| does not shrink |
| T20 | both | 11 | 3.1 | +0.5 | 0.772 | 84.5 | −6.5 | 0.717 | 2.919 | 0.1412 | 2.320 | 1.931 | +0.0014 | fails: \|bias\| does not shrink |
| ODI | none | 11 | 14.1 | +0.5 | 0.753 | 149.4 | −8.9 | 0.700 | 4.715 | 0.1591 | 5.294 | 2.886 | +0.0078 | control |
| ODI | venue | 11 | 13.9 | +0.5 | 0.750 | 148.9 | −9.0 | 0.701 | 4.713 | 0.1589 | 5.294 | 2.883 | +0.0093 | passes (by the letter; see below) |
| ODI | competition | 11 | 14.1 | +0.3 | 0.753 | 149.9 | −9.0 | 0.706 | 4.714 | 0.1591 | 5.294 | 2.885 | +0.0087 | fails: width grows |
| ODI | both | 11 | 13.7 | +0.3 | 0.757 | 149.1 | −9.2 | 0.701 | 4.713 | 0.1589 | 5.295 | 2.883 | +0.0084 | passes (by the letter; see below) |
| T20I | none | 10 | 7.6 | +2.9 | 0.783 | 80.9 | −2.5 | 0.688 | 3.222 | 0.1326 | 2.288 | 1.902 | −0.0017 | control |
| T20I | venue | 10 | 7.2 | +2.3 | 0.776 | 80.6 | −3.0 | 0.694 | 3.221 | 0.1324 | 2.289 | 1.902 | +0.0000 | reported: passes |
| T20I | competition | 10 | 7.0 | +2.6 | 0.771 | 79.9 | −2.6 | 0.684 | 3.220 | 0.1324 | 2.288 | 1.898 | +0.0003 | reported: passes |
| T20I | both | 10 | 6.7 | +2.5 | 0.790 | 79.9 | −2.9 | 0.691 | 3.221 | 0.1326 | 2.289 | 1.897 | +0.0002 | reported: passes |
| TEST | none | 11 | — | — | — | — | — | — | 8.335 | 0.3037 | 13.783 | 5.951 | — | control |
| TEST | venue | 11 | — | — | — | — | — | — | 8.341 | 0.3032 | 13.780 | 5.957 | — | reported: pinball no worse |
| TEST | competition | 11 | — | — | — | — | — | — | 8.334 | 0.3032 | 13.771 | 5.954 | — | reported: pinball no worse |
| TEST | both | 11 | — | — | — | — | — | — | 8.330 | 0.3029 | 13.765 | 5.957 | — | reported: pinball no worse |

The effect size against its own noise — the paired difference in |bias| per fold, arm minus
control, mean ± standard error over folds:

| arm | T20 | ODI | T20I |
|---|---:|---:|---:|
| venue | +0.18 ± 0.06 | −0.22 ± 0.27 | −0.43 ± 0.24 |
| competition | +0.05 ± 0.11 | −0.02 ± 0.21 | −0.64 ± 0.21 |
| both | +0.03 ± 0.11 | −0.41 ± 0.25 | −0.93 ± 0.30 |

And the ODI per-fold bias of the simulated first-innings mean, the quantity §8.3 named,
by arm (folds 2024-01 … 2026-06):

| arm | 24-01 | 24-04 | 24-07 | 24-10 | 25-01 | 25-04 | 25-06 | 25-09 | 25-12 | 26-03 | 26-06 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| none | −47.0 | +5.6 | +2.9 | −7.6 | +1.0 | +34.9 | +11.3 | −17.1 | +13.6 | +11.2 | −3.1 |
| venue | −48.4 | +4.6 | +2.4 | −7.0 | +1.0 | +34.1 | +12.5 | −17.0 | +13.8 | +10.9 | −1.5 |
| competition | −47.8 | +6.4 | +2.3 | −7.4 | +0.8 | +36.1 | +10.5 | −17.6 | +12.8 | +10.7 | −3.0 |
| both | −47.1 | +5.7 | +2.4 | −7.1 | +0.3 | +34.7 | +12.0 | −17.9 | +11.6 | +10.3 | −1.7 |

**Decision: a recorded null.** No family passes in both formats, so nothing ships:
`contract.FIXTURE_CONTEXT_FAMILIES_KEPT` stays empty and the performance model reads no
fixture-context column. The four columns stay in the frame, as the sequence families did
after E1, so the question can be re-asked without a new pass.

**Reading it.**

- *T20 is a clean fail.* The per-quarter |bias| is already small there (3.1 runs on totals
  of ~160) and none of the arms moves it: +0.18 ± 0.06 (venue, worse), +0.05 and +0.03
  (inside the noise). Coverage, width and every pinball are flat to the third decimal. A
  ground's level is something the model already carries through the elevens that play
  there, and 42 T20 columns do not need two more to say it.
- *ODI passes by the letter and not by any honest reading.* `venue` and `both` satisfy all
  four clauses — but the shrinkage is 0.22 and 0.41 runs on a mean |bias| of 14.1, each
  within about one standard error of zero over the folds, and the quarter the evidence
  named, 2024-01 at −47, reads −48.4, −47.8 and −47.1 under the three arms. The gate as
  registered had **no effect-size floor** — a design omission of this section, found by
  its own result — and shipping on it would be exactly the H-23 failure the registry exists
  to prevent: a pass for a reason other than the thing the gate named. The judgment call,
  recorded here and in the PR: the rule is applied as written, the arm's verdict is
  reported as it came out, and the family is not shipped because a shrinkage the folds
  cannot distinguish from zero is not the shrinkage §8.3 asked for. The next gate of this
  kind states its floor before it runs (one fold-level standard error is the natural one).
- *Why the ground and the competition cannot fix that quarter.* The 2024-01 ODI fold is 40
  matches: 29 men's — Nepal, Canada, Scotland, Western Australia — and 11 women's
  (Australia, Zimbabwe), with a mean first-innings total of 197 against the format's ~240.
  Their fixture context reads almost neutral (competition 0.97, venue 1.01 on average,
  12 % of the competitions in their first season): these grounds and competitions have
  either no history or a history at the format's level. The level of that quarter is *who
  is playing* — associate men's and women's ODIs scored against one unsplit baseline — not
  where or in what. The same holds for 2025-04 (+35: 55 of 78 matches women's) and 2025-09
  (−17: 53 of 97). §8.3's attribution, "largest where a quarter is forty matches of one
  competition", was right about the quarter and wrong about the cause: the per-quarter
  bias is a population-mix effect, and the place to look is the baseline the elevens are
  measured against (H-7 measured the gender split as costing nothing on AUC; nobody has
  measured it on totals), not the fixture.
- *T20I improves on every arm* — |bias| 7.6 → 7.2 / 7.0 / 6.7, two to three standard errors,
  with coverage held and width narrower — and is not decided on: ten folds of 23–57
  matches, the format whose T20 counterpart shows nothing. It is the one place the
  hypothesis has support, reported as such.
- *TEST* moves pinball by less than 0.1 % either way.
- *Cost:* the pass gains four columns and two keyed tables; the fit time is unchanged
  (190–200 s per T20 fold either way).

**The harness after the choice** (`make evaluate`, run once on each source with the decided
configuration — no family kept — 2026-09-03; 71 min on the database, 67 on the archive).
The locked window (≥ 2026-09-02) holds **0 matches** on both sources and says so; the
walk-forward table on the database is A-4's baseline (`docs/FOLLOW_UP_PLAN.md` § 5) to every
printed decimal — the same rows (21,093 / 465,336), the same columns read, a deterministic
pass — so "before" and "after" are one table. The archive run agrees within source noise
(T20 first-innings coverage / width / bias 0.770 / 84.7 / +0.5 against 0.772 / 84.8 / +0.5;
ODI 0.753 / 149.4 / +0.5 against 0.758 / 149.6 / +0.4; runs pinball 2.920 / 4.715 on both).
H-8: 50 matches, 1,100 player rows, 1,100 performance predictions, 50 simulations at max
abs difference **0.0 on both sources**, with the four fixture-context columns now in the win
row and every player row the check compares. Gates and glossary pass on both.

---

### 8.10 A-2: the chase's tails — a target-conditional chasing innings (2026-09-03)

The row in `docs/FOLLOW_UP_PLAN.md` § 4. Written **before the run**, as §8.9 was: the
evidence, the design, the leakage surface and the gate's H-23 triple first, with the
effect-size floor §8.9 asked the next gate of this kind to state; the tables are appended
below under *Results* and nothing above that heading is edited once a number exists.

**The evidence it chases (§8.3, A-4's baseline in `docs/FOLLOW_UP_PLAN.md` § 5).** The
simulated chase total under-covers from the low side in every limited-overs format: 10–90
coverage 0.711 / 0.689 / 0.699 (T20 / T20I / ODI, eleven folds) against nominal 0.80, with
0.192 / 0.184 / 0.181 of real chases ending *below* the simulated 10th percentile against a
nominal 0.10, and the simulated chase mean 6.5 / 2.7 / 9.0 runs high (10–16 on the retired
locked window). The miss above the 90th percentile is 0.10–0.13 and is a different thing:
a won chase finishes at the target *plus the winning hit*, and the design says the
overshoot is not modelled — so a simulated chase's 90th percentile is the target exactly
and any actual overshoot sits above it. That ceiling (about 0.88–0.90) is not this item's;
the low tail is. The margins say the same: the run margin when the side batting first wins
covers 0.66 / 0.74 / 0.65 and the balls remaining when the chaser wins 0.60 / 0.65 / 0.52,
at nominal 0.80 — the simulated losses are too small and too alike.

**What the chase path does today, and why that produces exactly this shape.** The chasing
side's innings is drawn as a first innings is — its L2-B forecasts in the chasing
orientation (`CHASE_ORIENTATION`), the balls budget, the shared factor — and then `_chase`
truncates it at the target. The untruncated draw *X* does not know the target: P(chase
reached) is P(X ≥ T), and a lost chase's total is a draw from the side's ordinary
distribution that happened to fall short. In the data a lost chase is not that. A side
chasing a target well above its expected score takes on risk it would not otherwise take,
loses wickets doing so and is often bowled out for far less than its unconditional
forecast; a side chasing well under its expected score gets there more surely than
P(X ≥ T) says. Both effects push the same way as the numbers: real chases win more often
than the draws (the simulator leans toward the side batting first by 0.04, §8.3) and lose
by more when they lose (the low tail, the narrow margins). The difficulty of the target is
the one thing the innings knows that the draw does not.

**Design: a chase response, fitted on the calibration fold.** Per draw *i* of a match, the
target *T_i* is the simulated first innings plus one, *φ_i* is the draw's shared factor
(the pitch, common to both innings), and *m₀* is the chasing side's expected total under
its own forecasts — the mean over the draws of its untruncated chase total divided by its
factor, i.e. what the eleven would be forecast to score on an average day, at the as-of
deliveries per innings. The **difficulty** of the chase is

    r_i = T_i / (φ_i · m₀)

— the required rate over the side's as-of scoring rate, on the day's pitch — and the
chasing side's runs draws are multiplied by

    g_i = exp(level + slope · ln r_i)

before the target truncation (the same lever the shared factor uses, applied after the
balls budget, which commutes with it). With slope < 0 a hard chase (*r* > 1) scores less
than its unconditional forecast and an easy one (*r* < 1) more; at *r* = 1 the draw is
the forecast times exp(level). Under level = slope = 0 the simulator is exactly today's.

*Fitted, never set.* `level` and `slope` are fitted by censored maximum likelihood on the
92-day calibration fold the members do not train on — the shared factor's fold, the same
matches (complete first innings, so a rain-shortened target does not enter). For each
calibration match *j* the fit reads the actual target *T_j* = actual first innings + 1,
the actual chase total *C_j*, whether the chaser won, the chasing side's *m₀,j* from the
same no-factor simulation the shared factor's fit already runs, and *φ̂_j*: the shared
factor's own per-match value for that match (1 + (actual / simulated − 1) · shrink), so
that the pitch is taken out of the difficulty at fit time exactly as the sampled factor
takes it out at draw time — without this, a high target on a flat pitch would read as a
hard chase that scored well, and the slope would learn the pitch. With
*x_j* = ln(T_j / (φ̂_j m₀,j)) and *y_j* = ln(C_j / (φ̂_j m₀,j)):

    y_j = level + slope · x_j + ε_j,   ε ~ N(0, σ²),   y_j ≥ x_j where the chaser won (censored)

— a Tobit regression: a lost chase contributes its density, a won chase only the fact that
the untruncated innings would have reached the target. σ is a nuisance parameter and is
reported beside the simulated chase's own spread on the log scale, because if σ is the
larger the chase is under-dispersed even with its level right — the shared factor was
fitted on first innings, and a collapse may carry dispersion of its own. Nothing is
hand-set: the two coefficients come from the fold, the functional form (log-linear in the
log difficulty) is the design, and the only guard is the shared factor's — fewer than
`MIN_SHARED_FACTOR_MATCHES` (30) calibration matches and the response is not fitted, the
simulator runs as today and the report says so. `simulator.CHASE_RESPONSE` records the
decision (which arm ships) the way `SHARED_FACTOR` does, and `FitSpec` carries it into the
run manifest.

*Why the runs and not the wickets.* A collapse is wickets falling; in this simulator the
number of batters who bat is drawn from P(bats) and a deeper innings *adds* the tail's
runs rather than removing anyone's, so depth is not a collapse lever here — the balls
budget is what ends an innings, and a side that bats eleven deep for fewer balls each is
the case the copula does not produce. The total is what the gate measures and the runs
draw is the one place a multiplicative response is coherent with the shared factor, so
that is where it goes; the wickets a real collapse loses are not modelled by this change
and the margin's *wickets in hand* is reported, not expected to move. Recorded as a
limitation, with the overshoot, DLS and per-ball required-rate dynamics.

*What could confound it.* A level miss of the chasing-orientation forecasts would also
show as chase bias, and a level correction alone would move coverage too. The gate
therefore carries a `level` arm — the same fit with the slope held at zero — as a control:
if it does as well as the slope arms, the effect is level, not difficulty, and the item's
hypothesis is not supported whatever the coverage says (H-23: a gate must not pass for a
reason other than the thing it names).

**H-21 audit — what the response consumes.**

1. *At draw time:* *T_i* is a simulated first innings (L2-B forecasts under the fixture's
   as-of), *m₀* the mean of the chasing side's own L2-B draws, *φ_i* the as-of shared
   factor, and (level, slope) the fold's fit. No outcome of the fixture enters.
2. *At fit time:* the calibration matches' actual first-innings totals, chase totals and
   results — matches before the cutoff, held out of the members' training
   (`performance._temporal_calibration_split`; the harness's E2 rows are after the cutoff).
   The response is therefore out-of-sample for every row it is applied to.
3. *Serving:* the artifact carries the fitted response with the shared factor
   (`PerformanceModels.simulation`); a live request applies the same function of the same
   draws. H-8's parity check compares both sides' totals and every player's runs at a fixed
   seed and draw count, which the response changes deterministically: it consumes no random
   numbers of its own. *m₀* is a mean within one call, so the draw depends on the draw
   count the way the quantiles already do; a fixed-*n* comparison is unaffected.
4. *The toss.* Marginalised as before: each orientation's half of the draws computes its
   own *m₀* from its own chasing side.

**H-23 triple — gate A-2** (`ml.xi.gates`, registered before the script ran; the script
prints it first):

- *varies:* the chase response the simulator applies to the chasing side's runs draws —
  `none` (today's simulator), `level` (the control: slope held at zero), `slope` (level
  held at zero), `both` — all four from **one** L2-B fit per fold and one fitted sample.
- *fixed:* the rows, the eleven quarterly cutoffs (A-4's rotated set, 2024-01 … 2026-06),
  the three seeds, the hyperparameters, the performance model (fitted once per fold, shared
  by the arms), the display models, the shared factor and its fitting rule, the simulator's
  draw count and its seeds (common random numbers: the response consumes none), the labels.
- *decides*, in **both** T20 and ODI, an arm against `none` on the same folds, paired per
  fold, with **one fold-level standard error of the paired difference as the effect-size
  floor** (§8.9's omission, stated here first):
  1. the chase 10–90 coverage's distance from 0.80 shrinks by more than one standard error;
  2. the mean over folds of |chase bias| shrinks by more than one standard error;
  3. the first-innings 10–90 coverage stays within ± 0.03 and its width does not grow
     (H-22; the first innings is untouched by construction, so this is a check that it is);
  4. E2 does not degrade: the arm's mean Δ Brier (simulated − display) stays within E2's
     0.01 tolerance and its paired difference from `none` is not worse by more than one
     standard error.
  A candidate arm (`slope` or `both`) ships only if it passes all four **and** beats the
  `level` control on clause 1's quantity by more than one standard error — otherwise the
  improvement is a level correction and is recorded as such, not shipped as a difficulty
  effect. If both candidates pass, `both` ships only if it beats `slope` on clause 1 by
  more than one standard error; else `slope`. Whether coverage lands inside 0.80 ± 0.03 is
  reported beside the verdict, not part of it (the overshoot ceiling is above the item's
  reach). The margins, the chase width, the below-q10 and above-q90 shares, σ against the
  simulated chase spread and the per-fold coefficients are reported for every arm; T20I is
  reported, not decided on; TEST has no simulator. A recorded null ships nothing. The script
  is `scripts/experiments/xi/a2_chase_tails.py`; it writes one JSON per format under
  `output/ml-service/a2/` and `--decide` prints the table and the verdict.

**The locked window, after the choice.** As in §8.9: the window (≥ 2026-09-02) is empty
until the data catches up, so `make evaluate` after the choice reports it as such, and the
before/after this section can honestly show is the harness's walk-forward table — A-4's
baseline against the same run with the decided configuration, same folds, same source,
same seeds — with H-8's parity on the last 50 matches, simulator draws included.

**Results** (`scripts/experiments/xi/a2_chase_tails.py`, run 2026-09-03 on the archive
frames; eleven folds 2024-01 … 2026-06, three seeds, 1,000 draws per fixture, one L2-B fit
per fold shared by the four arms, the display models, the shared factor and the simulator's
random numbers common to them). The calibration fold held 252–543 complete first innings per
T20 fold (mean 358), 41–178 per ODI fold (92) and 34–57 per T20I fold (44); ODI's 2026-03
fold and T20I's 2025-06 fold were too thin for a shared factor and so for a response, and
T20I's 2025-04 fold too small to score — those folds are absent from every arm alike. The
gate's table, means over folds; "σ vs sim" is the fitted residual scale on the log scale
against the simulated untruncated chase's own log-sd on the same folds:

| format | arm | folds | chase coverage | below q10 | above q90 | chase width | chase bias | mean \|bias\| | first coverage / width | Δ Brier | P(bat-first wins) sim / actual | margin runs cov / width | margin balls cov / width | level | slope | σ vs sim | verdict |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|---:|---|---|---|---:|---:|---|---|
| T20 | none | 11 | 0.713 | 0.192 | 0.095 | 70.0 | −6.6 | 6.7 | 0.770 / 84.7 | +0.0018 | 0.518 / 0.482 | 0.661 / 60.4 | 0.602 / 41.3 | — | — | — vs 0.230 | control |
| T20 | level | 11 | 0.714 | 0.203 | 0.083 | 71.4 | −8.5 | 8.5 | 0.770 / 84.7 | +0.0018 | 0.474 / 0.482 | 0.657 / 59.2 | 0.600 / 42.2 | +0.033 | 0 | 0.432 vs 0.230 | control: fails 1, 2 |
| T20 | slope | 11 | 0.702 | 0.141 | 0.158 | 68.6 | +1.1 | 3.5 | 0.770 / 84.7 | +0.0027 | 0.531 / 0.482 | 0.714 / 78.9 | 0.630 / 55.2 | 0 | −0.897 | 0.383 vs 0.230 | fails 1, 4 |
| T20 | both | 11 | 0.719 | 0.152 | 0.128 | 70.9 | −2.4 | 3.8 | 0.770 / 84.7 | +0.0028 | 0.454 / 0.482 | 0.734 / 79.8 | 0.596 / 59.1 | +0.098 | −1.114 | 0.406 vs 0.230 | fails 4 (1 inside one s.e.) |
| ODI | none | 10 | 0.713 | 0.180 | 0.107 | 133.4 | −9.3 | 17.8 | 0.770 / 153.7 | +0.0068 | 0.478 / 0.429 | 0.658 / 100.6 | 0.546 / 111.7 | — | — | — vs 0.248 | control |
| ODI | level | 10 | 0.712 | 0.195 | 0.093 | 136.9 | −13.2 | 19.9 | 0.770 / 153.7 | +0.0066 | 0.417 / 0.429 | 0.653 / 98.1 | 0.533 / 113.4 | +0.047 | 0 | 0.456 vs 0.248 | control: fails 1, 2 |
| ODI | slope | 10 | 0.685 | 0.145 | 0.169 | 129.5 | +3.7 | 14.5 | 0.770 / 153.7 | +0.0100 | 0.491 / 0.429 | 0.725 / 132.5 | 0.606 / 159.2 | 0 | −1.050 | 0.432 vs 0.248 | fails 1, 2, 4 |
| ODI | both | 10 | 0.701 | 0.149 | 0.150 | 133.3 | −0.2 | 14.3 | 0.770 / 153.7 | +0.0106 | 0.440 / 0.429 | 0.725 / 133.8 | 0.578 / 163.3 | +0.077 | −1.236 | 0.455 vs 0.248 | fails 1, 4 |
| T20I | none | 9 | 0.692 | 0.193 | 0.115 | 68.1 | −3.8 | 5.5 | 0.792 / 82.7 | −0.0005 | 0.519 / 0.499 | 0.691 / 63.3 | 0.640 / 40.6 | — | — | — vs 0.206 | control |
| T20I | level | 9 | 0.701 | 0.194 | 0.105 | 68.8 | −4.9 | 5.2 | 0.792 / 82.7 | −0.0003 | 0.491 / 0.499 | 0.690 / 62.6 | 0.640 / 41.3 | +0.019 | 0 | 0.260 vs 0.206 | reported: fails 1, 2 |
| T20I | slope | 9 | 0.700 | 0.155 | 0.145 | 67.5 | −0.2 | 3.9 | 0.792 / 82.7 | +0.0006 | 0.523 / 0.499 | 0.752 / 72.6 | 0.697 / 47.1 | 0 | −0.394 | 0.243 vs 0.206 | reported: fails 1 |
| T20I | both | 9 | 0.712 | 0.154 | 0.135 | 69.0 | −1.9 | 3.9 | 0.792 / 82.7 | +0.0001 | 0.462 / 0.499 | 0.768 / 75.1 | 0.647 / 51.2 | +0.060 | −0.617 | 0.255 vs 0.206 | reported: fails 1 |

The effect sizes against their own noise — the paired difference per fold, arm minus
`none`, mean ± standard error over folds, for the three quantities the gate reads (a
negative coverage distance is *toward* nominal):

| arm | T20: coverage distance | \|bias\| (runs) | simulated Brier | ODI: coverage distance | \|bias\| | Brier | T20I: coverage distance | \|bias\| | Brier |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| level | −0.001 ± 0.002 | +1.81 ± 0.53 | +0.00000 ± 0.00022 | +0.006 ± 0.007 | +2.08 ± 1.71 | −0.00019 ± 0.00047 | −0.002 ± 0.008 | −0.28 ± 0.88 | +0.00019 ± 0.00091 |
| slope | +0.011 ± 0.006 | −3.16 ± 1.51 | +0.00090 ± 0.00045 | +0.025 ± 0.023 | −3.31 ± 3.89 | +0.00320 ± 0.00089 | −0.002 ± 0.011 | −1.64 ± 1.40 | +0.00114 ± 0.00132 |
| both | −0.007 ± 0.007 | −2.91 ± 0.69 | +0.00108 ± 0.00054 | +0.020 ± 0.018 | −3.53 ± 3.03 | +0.00382 ± 0.00101 | +0.002 ± 0.013 | −1.65 ± 1.54 | +0.00059 ± 0.00107 |

Against the `level` control on the coverage distance: T20 slope +0.012 ± 0.007, both
−0.006 ± 0.007; ODI +0.019 ± 0.022, +0.015 ± 0.018; T20I −0.000 ± 0.011, +0.004 ± 0.011 —
no candidate beats it by one standard error anywhere.

The fitted slope per fold (`both` arm), the quantity the hypothesis stands on:

| format | 24-01 | 24-04 | 24-07 | 24-10 | 25-01 | 25-04 | 25-06 | 25-09 | 25-12 | 26-03 | 26-06 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| T20 | −2.02 | −1.22 | −1.43 | −0.87 | −1.30 | −0.82 | −0.76 | −0.95 | −0.79 | −0.83 | −1.26 |
| ODI | −2.38 | −1.91 | −0.66 | −0.84 | −1.54 | −0.84 | −0.26 | −1.15 | −1.72 | — | −1.07 |
| T20I | +0.36 | −0.63 | −0.08 | −0.06 | −0.42 | — | — | −0.94 | −1.34 | −0.56 | −1.89 |

And what the response did to the two tails, fold by fold in T20 (`none` → `both`: share of
real chases below the simulated 10th percentile / above the 90th): 24-01 0.145 / 0.077 →
0.077 / 0.123; 24-04 0.217 / 0.105 → 0.162 / 0.146; 24-07 0.247 / 0.076 → 0.179 / 0.098;
24-10 0.148 / 0.078 → 0.117 / 0.086; 25-01 0.157 / 0.096 → 0.129 / 0.154; 25-04 0.250 /
0.209 → 0.231 / 0.220; 25-06 0.187 / 0.078 → 0.163 / 0.094; 25-09 0.222 / 0.065 → 0.165 /
0.100; 25-12 0.156 / 0.046 → 0.124 / 0.100; 26-03 0.214 / 0.121 → 0.175 / 0.170; 26-06
0.173 / 0.093 → 0.154 / 0.117. In every fold the low tail thins and the high tail thickens
by about as much.

**Decision: a recorded null.** No candidate passes clause 1 in either decided format, none
beats the level control, and both candidates fail E2's clause in both; nothing ships.
`simulator.CHASE_RESPONSE` stays `"none"`, the harness and the retrain fit no response, and
the fit stays in the code — as the sequence and fixture-context families stayed in the
frame — so the next candidate can be measured on the same calibration sample.

**Reading it.**

- *The hypothesis is right about the direction and consistent about it.* The slope is
  negative in every T20 and ODI fold and in eight of nine T20I folds, at −0.8 to −2.4: a
  chase ten per cent harder than the side's expected score is forecast eight to twenty per
  cent fewer runs, which is the collapse the evidence described. It fixes the **level** —
  chase bias −6.6 → −2.4 / +1.1 (T20), −9.3 → −0.2 / +3.7 (ODI), −3.8 → −1.9 / −0.2 (T20I)
  — and it thins the **low tail** in every fold: below-q10 0.19 → 0.14–0.15 in all three
  formats. The margins, reported not decided, move the way the evidence wanted: the run
  margin when the side batting first wins covers 0.66 → 0.71–0.73 (T20), 0.66 → 0.73 (ODI),
  0.69 → 0.75–0.77 (T20I) and the balls remaining when the chaser wins 0.60 → 0.63, 0.55 →
  0.61, 0.64 → 0.70 (slope arm) — the simulated losses were too small, and a lost chase that
  collapses is a bigger loss.
- *And it does not move the coverage, because the miss is not a level by difficulty but a
  shape.* In every fold the mass that leaves the low tail reappears above the 90th
  percentile — 0.095 → 0.158 / 0.128 (T20), 0.107 → 0.169 / 0.150 (ODI) — so the 10–90
  coverage stays where it was (0.713 → 0.702 / 0.719, 0.713 → 0.685 / 0.701). The mechanism
  is visible in the table: once the response lowers a hard chase's draws, P(reach) falls
  under 0.10 for many fixtures, the simulated 90th percentile drops below the target, and
  every hard chase that was in fact *won* lands above it. A multiplicative shift moves the
  whole draw with the difficulty; a real chase is two-lobed — the side collapses or gets
  there — and the fitted residual scale says exactly that: σ = 0.38–0.46 on the log scale
  (T20 / ODI) against the simulated untruncated chase's own log-sd of 0.23–0.25. The
  residual *after* the response is nearly twice the spread the draws have. The shared factor
  fixed the first innings' dispersion; the chase has dispersion of its own that a location
  response cannot supply, and thinning one tail by fattening the other is what such a
  response does to a bimodal target.
- *E2 degrades, and says why.* The slope-only arm pushes the simulated P(bat-first wins)
  *away* from the actual (0.518 → 0.531 vs 0.482 in T20, 0.478 → 0.491 vs 0.429 in ODI) and
  `both` over-corrects the other way (0.454, 0.440); the Brier worsens by +0.0009–0.0011
  (T20, two standard errors) and +0.0032–0.0038 (ODI, three to four), with the ODI arms
  sitting on E2's 0.01 tolerance itself. A response fitted to chase *totals* moves P(win)
  through the same draws, and the folds say it moves it the wrong way: the location fix
  buys total accuracy at the cost of the probability, which is the headline the simulator
  is served beside.
- *The level control found the double-counting P-4 measured as harmless.* The fitted level
  is +0.02 to +0.05 in every format: the chasing side's untruncated expected total sits
  two to five per cent under the chases it actually makes. That is `CHASE_ORIENTATION`: the
  chasing-orientation forecasts are fitted to chase rows whose runs the target already
  truncated, and the simulator truncates again. §8.3 measured it on Brier and P(bat-first
  wins) as no effect (0.0003 and 0.015) and it still is — the level arm moves coverage by
  −0.001 / +0.006 and *worsens* |bias| — but it is a real, small, named level miss and is
  recorded here as such.
- *T20I* is the same picture with a shallower slope (−0.4 to −0.6 on average, one fold
  positive on 34 calibration matches) and is not decided on.
- *What this leaves for the chase.* The number to chase is the residual dispersion: a
  chase-specific spread, or a two-component mixture (collapse or reach) fitted on the same
  censored calibration sample, is the candidate the next item should state — the sample,
  the censoring and the fold are built and the gate's table is the baseline to beat. A
  response of the *level* alone is not it, and this section says so before anyone re-runs
  it.
- *Cost:* four arms per fold from one fit; a fold took 3–12 minutes with the three formats
  sharing one machine, the fit itself 400–680 s at three seeds.

**The harness after the choice** (`make evaluate`, run once on each source with the decided
configuration — no response — 2026-09-03, the two sources in parallel, 3 h 35 min each).
The locked window (≥ 2026-09-02) holds **0 matches** on both sources and says so. The
walk-forward table on the database is A-4's baseline (`docs/FOLLOW_UP_PLAN.md` § 5) to
every value compared — objective and display AUC, E2's Δ Brier, first-innings and chase
coverage / width / bias, the below-q10 share, both margins, every pinball, E5 — on the same
21,093 / 465,336 rows, and the archive run equals A-1's archive run the same way: under
`none` the simulator's draws are the draws it made before, and the report's calibration
node now carries `chase_response: null` beside the shared factor. H-8: 50 matches, 1,100
player rows, 1,100 performance predictions, 50 simulations at max abs difference **0.0 on
both sources**. Gates (A-2 now in the embedded registry) and glossary pass on both.

### 8.11 A-3: the T20 lineup signal — feature families in the selection objective (2026-09-03)

The row in `docs/FOLLOW_UP_PLAN.md` § 4. As in §8.9 and §8.10: the evidence, the design,
the leakage surface and the gate's H-23 triple first — the triple was registered in
`ml/xi/gates.py` and printed by the script before anything ran — and the tables under
*Results*, with nothing above that heading edited once a number existed.

**The evidence it chases (§8.8, A-4's baseline in `docs/FOLLOW_UP_PLAN.md` § 5).** T20 is
the one format whose objective fails the bar written beside it: lineup-only E5 **0.503
against a derived bar of 0.506** over 2,168 walk-forward pairs (0.490 against 0.501 on the
seven pre-rotation folds when P-7 decided it). §8.8 read the number two ways and both
still hold. The objective's direction on a T20 lineup change is not distinguishable from
chance (0.503 is 0.3 standard errors above 0.5), and it is *less right than its own claims*:
an exactly-right objective with the same claimed effect (median |Δ| 0.021 win probability for
a 1–3 player change) would agree 0.521 of the time. §8.6 named the likely reason — in
domestic T20 much of a 1–3 player change is squad rotation, a change of names that is not a
change of strength — which is a reason the signal is weakest there, not a reason it cannot
be found. Optimised selection is scoped off in T20 on that number
(`optimizer.NOT_OPTIMISED_REASONS`), the harness re-verdicts it every run, and this item is
the attempt to move it. **The judge is the folds' lineup-only E5 against its re-derived bar
— never AUC, never the locked window.** AUC and swap monotonicity are reported as a guard
(a family must not degrade them), not as the decision.

**What the objective can see today, and what it cannot.** The objective is a logistic
regression on `XI_FEATURE_COLS` — 42 columns, every one a sum, a mean, a count or an
extreme of per-player as-of ratings over an eleven, plus the differences between the two
sides. It is additive by design (H-4: a one-player upgrade never lowers the score, which is
what makes the search well-behaved). Three things follow. The per-player *phase* rates the
pass already accumulates (`bat_pp_rate` … `bowl_death_rate`, in `PLAYER_ROLE_KEYS`) reach
the performance model through every player row but reach the objective through nothing: no
side aggregate reads them, so a side whose batting is front-loaded into the powerplay and one
whose hitting is at the death read the same. Nothing in the row is a function of *both*
sides beyond a difference: the objective prices "our batting" and "their bowling" but never
"our batting against their bowling". And nothing within a side is a product: five bowlers
and six batters is `n_bowlers`, `n_allrounders` and `has_keeper`, never whether the side has
*both* a top order and an attack.

**Design: three candidate families, one at a time.** Each family is one arm: the objective
refitted per fold on `XI_FEATURE_COLS` plus the family's columns, the same model class and
regularisation (`make_objective_model`), everything else held fixed. A family's columns
follow the contract's pattern — `d_`, `t1_`, `t2_` per side stem, plus one `x_` column per
cross term of the two sides.

*(a) Phase matchup.* The follow-up plan named "batters' spin/pace splits against the opposing
attack's composition". **Cricsheet carries no bowling style** — the ball-by-ball JSON names
the bowler and nothing about how they bowl, and the people registry is identifiers only —
so a spin/pace split is not constructible from anything the rating pass reads, and building
one would mean an external registry of bowling styles, which is a data item, not a feature.
The per-player split the pass *does* accumulate is by innings phase (powerplay / middle /
death, `PHASE_BOUNDS`), and that is the matchup axis used here. Per side and phase *p*:

| stem | definition |
|---|---|
| `bat_p` | Σ over the eleven of `bat_p_rate` × `exp_balls_faced` — the phase-resolved form of `imp_bat_sum` |
| `bowl_p` | Σ of `bowl_p_rate` × `exp_balls_bowled` — the phase-resolved form of `imp_bowl_sum` |
| `x_p_matchup` | `t1_bat_p` · `t2_bowl_p` − `t2_bat_p` · `t1_bowl_p` — team1's batting in *p* against team2's attack in *p*, minus the reverse, so the H-3 marginalisation is coherent |

Six stems (18 columns) and three cross terms: 21 columns.

*(b) Role balance.* Beyond the three counts the objective has, seven stems (21 columns), four
of them roles the additive objective cannot count and three of them within-side products it
cannot express:

| stem | definition |
|---|---|
| `n_top_order` | players whose as-of expected batting position is ≤ 3.5 |
| `n_specialist_batters` | players who bat in ≥ 60 % of their appearances and are not a bowling option |
| `bowl_depth_6th` | the sixth-highest expected balls bowled — the sixth bowling option |
| `keeper_bat` | the keeper's batting impact (0 without one) |
| `bat_x_bowl` | `imp_bat_top6` × `imp_bowl_top5` — a side needs both |
| `allround_x_tail` | `n_allrounders` × `imp_bat_tail` |
| `attack_balance` | −\|`n_bowlers` − 6\| — an attack of the usual size |

*(c) Reweighting the evidence by the claimed |Δ|.* Not an arm: **a change of the
measurement, not of the model**, and treated as such. The rotation reading says the pairs
that dilute T20's agreement are the ones on which the objective itself claims almost nothing
(|Δ| under 0.01, a third of the scored pairs). Three readings are computed on every arm
beside the unweighted E5: each pair weighted by the |Δ| the arm claims; the top half of
pairs by |Δ| unweighted; and agreement by tercile of |Δ| against that tercile's own
exactly-right expectation. **The bar is re-derived under the same weighting** — the
weighted agreement an exactly-right objective would produce, its closed form and the 5th
percentile of 2,000 replicates — so a weighting that concentrates on the pairs the objective
is surest about raises the bar with the number, which is the point. Whether (c) could ever
*decide* is settled before the run rather than after: it cannot. A gate whose decider is
replaced once its number is known is the failure H-23 was written against, and a reweighted
bar that a format passes by less than its own noise is not a verdict. (c) is reported as
evidence about *why* the null occurs; if it were to argue for changing what E5 measures,
that would be a new gate, registered first, with its own run.

**Method (`scripts/experiments/xi/a3_t20_lineup_signal.py`).** Every eleven E5 scores is
rebuilt from per-player as-of vectors, so that a family can be evaluated without re-running
the pass: the fielded elevens of match *k* and *k+1* and their opponents from the player
frame — the same numbers `RatingState.side_vectors` served when the row was built (H-8),
checked by recomputing the 20 base stems for every training row against the win frame's own
columns (max abs difference **0.0** in every fold of every arm) — and match *k*'s eleven at
match *k+1*'s as-of from one advancing pass of `AsOfRatings` over the archive, exactly as
`natural_experiment.score_previous_elevens` reads it, with the fielded eleven read from the
same state and compared with the frame (max abs difference **0.0** over 25,751 pairs). Per
arm, format and fold: the objective fitted on the rows before the cutoff; the marginalised
AUC on the fold window (H-3, as the harness); H-4's swap probe — one player's five ratings
raised by one population sd, the first 50 matches of the window — with the arm's own
feature construction; E5 on the window's pairs with the fold's objective; then the pooled
decision with `natural_experiment.agreement`, `effect_size` and `derived_bar`, the bar
under **three seeds** (20260902, 20260903, 20260904). The objective is a deterministic fit,
so the seeds are the bar's — the only stochastic element in the decision — and an arm must
clear all three. The locked window (≥ 2026-09-02) is empty by construction (A-4) and is
scored by `make evaluate` after the choice, as §8.9 and §8.10 did.

**H-21 audit — what the families consume.** Every input is an as-of vector of the eleven
read from the state at the match date (H-1): the phase rates, expected balls, batting
position, innings share, keeper flag and the base aggregates. The cross terms are functions
of the two elevens' as-of aggregates and nothing else. No outcome enters a feature; no
fitted output is consumed by another fit — the arms are single logistic fits on rows
before each cutoff. Family (c)'s weights are the arm's own claimed Δ, a function of as-of
features, never of the result; the null world the weighted bar is derived from is the same
Bernoulli world as the unweighted one. Serving would read the same stems through
`aggregate_side` and `side_vectors`, the one read path, so a kept family would inherit H-8's
parity check without new code on the serving side.

**H-23 triple — gate A-3** (`ml.xi.gates`, registered before the script ran; the script
prints it first):

- *varies:* the feature family the selection objective reads beyond `XI_FEATURE_COLS` —
  `none` (today's objective), `phase_matchup` (a), `role_balance` (b) — one logistic fit per
  arm per fold; and, as a measurement diagnostic on every arm rather than an arm, E5's
  evidence reweighted by the |Δobjective| the arm itself claims (c).
- *fixed:* the rows, the eleven quarterly cutoffs (A-4's rotated set, 2024-01 … 2026-06),
  E5's pairs (the same consecutive 1–3-change pairs, the previous eleven read once from the
  same as-of pass), the bar's derivation (Bernoulli at the arm's own probabilities, 2,000
  replicates, the 5th percentile), the objective's model class and regularisation, the
  display models, the labels.
- *decides:* in **T20**, an arm's pooled walk-forward lineup-only agreement at or above the
  bar re-derived from that arm's own claimed effect size, under each of three bar seeds. An
  arm that clears it ships only if, in **every** format, its fold objective AUC is not lower
  than today's by more than one fold-level standard error (paired over folds) and its
  swap-violation share stays under H-4's 2 % — the contract's columns are one list for all
  formats, so a family that breaks another format's objective is not shippable for T20's
  sake. The reweighted (c) reading is reported beside the verdict and never decides on its
  own. A recorded null ships nothing and T20 stays rating-ordered.

**Results** (`scripts/experiments/xi/a3_t20_lineup_signal.py`, run 2026-09-03 on the
archive frames; eleven folds 2024-01 … 2026-06, 25,751 lineup pairs across the formats,
the whole run six minutes of which the as-of pass is 40 seconds). "Bar" is the range over
the three seeds; "by changes" is the agreement on 1- / 2- / 3-player changes; the AUC delta
and the swap share are the guard.

| arm | format | folds | objective AUC (Δ vs none ± se) | swap share | E5 pairs scored | agreement ± se | claimed median \|Δ\| | exactly-right | bar (3 seeds) | by changes 1 / 2 / 3 | verdict |
|---|---|---:|---|---:|---:|---|---:|---:|---|---|---|
| none | **T20** | 11 | 0.697 | 0.0023 | 2,169 | **0.503 ± 0.011** | 0.0208 | 0.521 | 0.505–0.506 | 0.512 / 0.512 / 0.452 | control: fails |
| none | T20I | 10 | 0.756 | 0.0075 | 220 | 0.564 ± 0.033 | 0.0228 | 0.523 | 0.472–0.474 | 0.575 / 0.610 / 0.482 | reported: passes |
| none | ODI | 11 | 0.672 | 0.0000 | 692 | 0.566 ± 0.019 | 0.0241 | 0.534 | 0.502–0.504 | 0.588 / 0.529 / 0.591 | reported: passes |
| none | TEST | 11 | 0.626 | 0.0050 | 213 | 0.549 ± 0.034 | 0.0234 | 0.531 | 0.476–0.477 | 0.629 / 0.597 / 0.432 | reported: passes |
| phase_matchup | **T20** | 11 | 0.700 (+0.003 ± 0.002) | 0.0012 | 2,169 | **0.510 ± 0.011** | 0.0217 | 0.522 | 0.505–0.506 | 0.521 / 0.523 / 0.442 | **clears the bar; guard fails** |
| phase_matchup | T20I | 10 | 0.753 (−0.004 ± 0.010) | 0.0076 | 220 | **0.486 ± 0.034** | 0.0275 | 0.511 | 0.459–0.462 | 0.517 / 0.481 / 0.446 | reported: passes |
| phase_matchup | ODI | 11 | 0.671 (−0.002 ± 0.003) | **0.2431** | 692 | 0.566 ± 0.019 | 0.0240 | 0.535 | 0.504 | 0.602 / 0.529 / 0.565 | reported: passes; **guard fails (H-4)** |
| phase_matchup | TEST | 11 | 0.619 (**−0.007 ± 0.006**) | 0.0048 | 213 | 0.549 ± 0.034 | 0.0246 | 0.533 | 0.478–0.481 | 0.629 / 0.545 / 0.486 | reported: passes; **guard fails (AUC)** |
| role_balance | **T20** | 11 | 0.701 (+0.003 ± 0.001) | **0.0287** | 2,169 | **0.496 ± 0.011** | 0.0210 | 0.521 | 0.504–0.506 | 0.504 / 0.510 / 0.439 | **fails; guard fails (H-4)** |
| role_balance | T20I | 10 | 0.750 (**−0.006 ± 0.004**) | 0.0107 | 220 | 0.559 ± 0.033 | 0.0238 | 0.521 | 0.471–0.472 | 0.563 / 0.597 / 0.500 | reported: passes; guard fails (AUC) |
| role_balance | ODI | 11 | 0.676 (+0.004 ± 0.004) | 0.0000 | 692 | 0.548 ± 0.019 | 0.0257 | 0.533 | 0.501–0.502 | 0.570 / 0.525 / 0.545 | reported: passes |
| role_balance | TEST | 11 | 0.622 (−0.004 ± 0.009) | 0.0050 | 213 | 0.512 ± 0.034 | 0.0248 | 0.531 | 0.476–0.479 | 0.581 / 0.532 / 0.432 | reported: passes |

Per-fold T20 agreement, none / phase_matchup / role_balance (pairs in the window):
2024-01 0.508 / 0.534 / 0.471 (411) · 2024-04 0.512 / 0.482 / 0.494 (447) · 2024-07 0.519 /
0.533 / 0.537 (496) · 2024-10 0.439 / 0.424 / 0.444 (472) · 2025-01 0.529 / 0.543 / 0.536
(318) · 2025-04 0.468 / 0.511 / 0.426 (253) · 2025-06 0.469 / 0.472 / 0.458 (805) · 2025-09
0.517 / 0.510 / 0.517 (360) · 2025-12 0.526 / 0.512 / 0.573 (492) · 2026-03 0.487 / 0.532 /
0.449 (413) · 2026-06 0.551 / 0.568 / 0.527 (725).

Family (c), the measurement reweighted by the claimed |Δ|, on every arm — agreement /
exactly-right / bar (three seeds), and the terciles of |Δ| as agreement vs exactly-right:

| arm | format | unweighted | \|Δ\|-weighted (effective pairs) | top half by \|Δ\| | terciles bottom / middle / top (median \|Δ\| 0.006 / 0.021 / 0.052 in T20) |
|---|---|---|---|---|---|
| none | **T20** | 0.503 / 0.521 / 0.505–0.506 fails | 0.524 / 0.544 / 0.522–0.523 passes (65) | 0.521 / 0.539 / 0.515–0.516 passes (1,123) | **0.476 vs 0.502** (n=683) / 0.495 vs 0.513 (721) / **0.535 vs 0.549** (765) |
| none | T20I | 0.564 / 0.523 / 0.472–0.474 passes | 0.561 / 0.548 / 0.475–0.479 passes | 0.564 / 0.536 / 0.463–0.466 passes | 0.559 vs 0.499 / 0.605 vs 0.525 / 0.526 vs 0.543 |
| none | ODI | 0.566 / 0.534 / 0.502–0.504 passes | **0.612 / 0.557** / 0.512–0.515 passes | 0.616 / 0.551 / 0.507–0.512 passes | 0.527 vs 0.516 / 0.563 vs 0.527 / 0.607 vs 0.557 |
| none | TEST | 0.549 / 0.531 / 0.476–0.477 passes | 0.555 / 0.563 / 0.488–0.492 passes | 0.529 / 0.562 / 0.482–0.486 passes | 0.533 vs 0.501 / 0.656 vs 0.519 / 0.473 vs 0.571 |
| phase_matchup | T20 | 0.510 / 0.522 / 0.505–0.506 passes | 0.519 / 0.543 / 0.521 fails | 0.516 / 0.540 / 0.517–0.518 fails | 0.515 vs 0.503 / 0.477 vs 0.511 / 0.535 vs 0.550 |
| phase_matchup | T20I | 0.486 / 0.511 / 0.459–0.462 passes | 0.572 / 0.552 / 0.478–0.481 passes | 0.559 / 0.548 / 0.476–0.481 passes | 0.377 vs 0.473 / 0.500 vs 0.493 / 0.575 vs 0.561 |
| phase_matchup | ODI | 0.566 / 0.535 / 0.504 passes | 0.606 / 0.557 / 0.512–0.517 passes | 0.602 / 0.550 / 0.507–0.508 passes | 0.541 vs 0.518 / 0.552 vs 0.521 / 0.604 vs 0.563 |
| role_balance | T20 | 0.496 / 0.521 / 0.504–0.506 fails | 0.526 / 0.548 / 0.525–0.526 passes | 0.524 / 0.541 / 0.518–0.519 passes | 0.454 vs 0.497 / 0.501 vs 0.512 / 0.529 vs 0.553 |
| role_balance | ODI | 0.548 / 0.533 / 0.501–0.502 passes | 0.591 / 0.556 / 0.511–0.514 passes | 0.576 / 0.551 / 0.506–0.508 passes | 0.489 vs 0.511 / 0.571 vs 0.527 / 0.579 vs 0.558 |

**Neither family ships; the null is recorded per family and T20 stays rating-ordered.**

- *The control reproduces the harness.* 0.503 over 2,169 pairs against 0.505–0.506, with
  1,091 agreed — the harness's 1,091 agreed over 2,168 against 0.506, on either source. The
  one pair apart is in the 2025-06 fold: the harness reads the objective as indifferent on
  it (Δ exactly 0, the swapped players unrated) and excludes it, the reconstruction's
  summation order leaves a rounding-level Δ and scores it as a miss (0.469 against the
  harness's 0.470 on that fold). Every other fold agrees to the fourth decimal, as do the
  AUCs and swap shares in every format. The reconstruction from per-player vectors reads
  what the pass wrote (parity 0.0 twice over), so what the arms measure is the objective and
  not the plumbing.
- *Phase matchup clears T20's bar and is not shippable.* 0.510 ± 0.011 against 0.505–0.506:
  over the bar by 0.004, **under half a standard error**, and +0.007 over the control — the
  same pairs, better in seven folds of eleven and worse in four — which is inside the noise
  floor H-14 draws (the fold sd of the control's agreement is 0.030). Even on the letter
  of the decider it passes, and the guard says why it must not ship: in **ODI a quarter of
  one-player upgrades lower P(win)** (1,422 of 5,667; 11–41 % per fold, against H-4's 2 %).
  The mechanism is collinearity: the three phase impacts sum to almost the whole batting or
  bowling impact, so the fit is free to put the strength on the phase columns and a
  *negative* weight on the unresolved total — and an upgrade of a player's overall rating,
  which moves the total but not the phase columns, then reads as a downgrade. T20's fit
  happened not to (0.12 %), which is the point of a guard in every format: a column list is
  one contract. TEST's AUC falls by 0.007 ± 0.006, and — reported, not decided on, but the
  most telling number in the table — **T20I's agreement falls from 0.564 to 0.486** (2.3
  standard errors, on the 220 pairs of the one format where the objective is a proven
  selector): the family does not add a lineup signal, it re-spends the one there is.
- *Role balance fails outright.* T20 0.496 ± 0.011 against 0.504–0.506, worse than the
  control on 1- and 2-player changes alike; the within-side products make T20's own
  surface non-monotone (2.9 % of upgrades lower P(win), over H-4's line), T20I's AUC falls
  by 0.006 ± 0.004, ODI's agreement drops 0.566 → 0.548. Counting roles the objective does
  not count changes what it prefers and not how often it is right.
- *Family (c): the rotation reading is half right, and it is not a verdict.* On the control,
  T20's agreement rises with the claimed |Δ| — **0.476 / 0.495 / 0.535** across the terciles
  (median |Δ| 0.006 / 0.021 / 0.052) — so the pairs on which the objective claims almost
  nothing, the rotation pairs, carry no sign (the bottom tercile is 1.3 standard errors
  *under* chance) and the pairs it is surest about agree more often. But every tercile sits
  under its own exactly-right expectation (0.502 / 0.513 / 0.549): the objective is not
  right as often as it claims anywhere in its range, which is §8.8's reading again, one
  slice at a time. The reweighted numbers pass their re-derived bars by 0.001–0.005 —
  |Δ|-weighted 0.524 against 0.522–0.523 on an effective sample of 65 pairs (the weights sit
  on the p90 tail); top half 0.521 against 0.515–0.516 — which is a coin balanced on its
  edge, not a selector. ODI shows what a real one looks like under the same weighting:
  0.566 → 0.612 against an exactly-right 0.557. So (c) is **dropped as a measurement** —
  it would flip T20's verdict on a margin smaller than its noise, after the number was
  seen, on a reading whose own expectation it still fails — and **kept as the diagnosis**:
  the T20 null is the rotation pairs' chance-level sign diluting a small-|Δ| signal that
  is itself under its claims.
- *What this leaves.* Two families that address what the objective cannot see leave the
  number where it was; the constraint is not the feature set tried. To clear its bar at the
  claimed effect the T20 objective needs about +0.02 of agreement — two standard errors at
  2,169 pairs — and more pairs (A-5's cadence) shrink the bar's noise but do not move
  0.503. The candidates the record supports are: a matchup axis the data does not carry
  (bowling style, which is an external registry and a data item before it is a feature);
  and an objective fitted *on the pairs* — Δresult regressed on the lineup Δ directly, the
  one model change that targets this number rather than AUC, which the migration's "model
  class is not the constraint" lesson (§8.2) did not test because it was measured on AUC.
  Either is its own item with its own gate; neither is this one.
- *Two things the guard taught, recorded for the next family.* H-4's probe upgrades five
  base ratings; a family whose columns are collinear with the base impacts can pass the
  probe in one format and fail it by 25 % in another from the same fit, so the guard has to
  run in every format the contract serves. And a family that reads per-player keys the
  probe does not upgrade (the phase rates) is tested for monotonicity in the base ratings
  only — a kept family of that kind would need the probe extended to its inputs.
- *Cost:* six minutes for three arms × four formats × eleven folds; the as-of pass 40
  seconds with the pairs cached (`output/ml-service/a3/pairs.pkl`).

**Judgment calls, recorded.** The spin/pace split is not in the data, so family (a) is the
phase matchup (above). "Three seeds" is applied to the bar, the decision's only stochastic
element, because the objective is a deterministic fit. The guard is in every format because
`XI_FEATURE_COLS` is one list. The reason on the wire (`NOT_OPTIMISED_REASONS` and go-app's
mirror) now quotes A-4's rotated-fold figure (0.503 against 0.506 over 2,168 pairs) beside
P-7's, and says A-3 did not change it.

**The harness after the choice** (`make evaluate`, run once on each source with the decided
configuration — no family — 2026-09-03, the two sources in parallel, 3 h 33 min on the
archive and 3 h 36 min on the database). The locked window (≥ 2026-09-02) holds **0
matches** on both sources and says so. Compared node for node with A-2's post-choice
reports on the same sources (A-4's baseline, `docs/FOLLOW_UP_PLAN.md` § 5), the only fields
that differ are fit and latency timings: every objective and display AUC, swap share, E5
rate, bar and verdict (T20 **0.503 over 2,168 against 0.506, fails, not served**; T20I 0.564
against 0.472 and ODI 0.566 against 0.503, pass, served; TEST 0.549 against 0.478, passes,
off under H-17), every pinball, coverage, width, bias and margin is the baseline's to the
last digit, on 21,093 / 465,336 rows — which is what a recorded null that ships no code on
the pipeline should produce, and is now shown rather than assumed. H-8: 50 matches, 1,100
player rows, 1,100 performance predictions, 50 simulations at max abs difference **0.0 on
both sources**. Gates (A-3 now in the embedded registry) and glossary pass on both. The
`selection_decision` node restates the policy beside the verdict per format, and the two
agree everywhere.

---

### 8.12 X-1b: biography features — age in the performance model, the age-aware cold start, and the matchups that could not run (2026-09-04)

The row in `docs/EXTERNAL_DATA_PLAN.md` § X-1b. As in §8.9–§8.11: the evidence, the design,
the leakage surface and the gates' H-23 triples first — both triples were registered in
`ml/xi/gates.py` and printed by the script before anything ran — and the tables under
*Results*, with nothing above that heading edited once a number existed.

**The evidence it chases (§ X-1a).** The archive records what happened and nothing about
who it happened to, and the follow-up plan's three nulls (A-1, A-2, A-3) said the models sit
near the limit of what the archive holds. X-1a acquired the one biographical fact Wikidata
carries at usable scale — a date of birth for **85.3 % of appearances**: ODI 93.1 / 94.0 %
(men / women), T20I 92.5 / 93.7 %, men's T20 82.6 %, and **women's T20 61.4 %** — and found
the two it does not: a bowling style for **265** players, a batting hand for **18**, a
career end for **3**, of 13,662. Two mechanisms the ratings cannot express follow from a date
of birth. *Trajectory:* the as-of ratings are a decayed summary of the past, so a 21-year-old
and a 36-year-old with the same recent form read the same, while their next innings do not
come from the same distribution. *Cold start:* a player with no history in the format reads
the neutral vector (H-10) whatever his age, while a 19-year-old debutant and a 34-year-old
one are drawn from different populations — the second has usually been kept out of the side
for a reason, or has come from another format.

**Family 2 — matchups — is not runnable, and this is the labelled confirmation of A-3.**
The left–right top-order balance and the spin-type coverage against the opposition's
handedness profile need a batting hand and a bowling style per player. X-1a measured those
at **18 and 265 players of 13,662** (0.4 % and 4.7 % of appearances), and measured *why*:
`P741` / `P552` are stated on 23 and 29 cricketer items world-wide, `P2545` on 1,145, and the
265 are a bot import (179 left-arm-orthodox, 73 leg-spin, single figures elsewhere) rather
than a labelling. There is no side-level aggregate to build from a label 1.9 % of the
registry carries, and fitting one to that 1.9 % would be measuring the population Wikidata
happened to tag, not a matchup. A-3 (§8.11) tried the matchup axis from what the archive
does carry — the innings phase — because "Cricsheet carries no bowling style", and recorded
a null; X-1b's family 2 is the same question asked with the labels, and the answer is that
the labels do not exist either. Nothing was fitted, no inferred label was substituted (A-3
already tested that route), and the E5 re-run the family carried is not run: it goes with
the family, and X-3 owns E5's hygiene question on its own terms. Recorded, not attempted.

**Family 1 — age in the performance model — design.** Two columns on every player row,
`contract.AGE_COLS`:

| column | definition |
|---|---|
| `age` | years between the date of birth and the match date, `(match_date − birth_date).days / 365.25`; **0.0 when no date of birth exists** |
| `age_known` | 1.0 when a date of birth exists, else 0.0 |

The pair is what makes a missing date **a category and never an imputed age**: a row
without one reads `(0.0, 0.0)`, a value no cricketer has at a match, and the model reads the
indicator beside it. The columns are computed by `rows.player_feature_rows` from the dates
of birth the *source* supplies (`MatchSource.birth_dates()` — `player_biography.birth_date`
joined to `player` on Postgres; for the archive, which has no biography, the CSV
`make export-birth-dates` writes from that table, so both sources read the same dates) and
held on the `RatingState` (`state.birth_dates`, persisted with the artifact), so training,
the as-of serving path and a live request read one function of one map (H-8). The frame
always carries both columns; the performance model reads them only if
`contract.AGE_FEATURES_KEPT` is true, exactly as the sequence and fixture-context families.

*No curvature term.* The prompt allowed "a curvature term or spline". The performance model
is a tree ensemble (`HistGradientBoostingRegressor`), whose splits are invariant to any
monotone transform of a column, and `age²` is monotone over every age a cricketer has; a
spline basis is likewise nothing a depth-limited tree cannot cut for itself along one axis.
Adding either would be a second copy of the same ordering, so the arm is `age` +
`age_known` and the record says why there is no third column.

*Scope.* X-1a's coverage decides which rows can carry the gate. ODI (93.1 / 94.0 %) and T20I
(92.5 / 93.7 %) clear the 80 % bar for both genders and are scored whole. In T20 the men's
rows (82.6 %) clear it and the women's rows (**61.4 %**) do not: a family judged on rows where
four appearances in ten have no age would be judged on the indicator, so **T20's verdict is
read on men's rows only**, and the women's T20 rows are reported beside it, never deciding.
The model itself is still fitted on every T20 row (production serves women's T20 from the
same artifact, and the indicator is how it reads a row without an age), so the scoping is of
the *verdict*, not of the fit. TEST is reported, as A-1 reported it.

**Family 3 — the age-aware cold start — design.** The neutral vector a no-history player
reads today is the shrinkage target of every rate (0 above expectation, 0 balls, Elo 1500).
The prior shaped by age replaces six of those values — `exp_balls_faced`, `bat_rate`,
`bat_wrate`, `exp_balls_bowled`, `bowl_rate`, `bowl_wrate` (`ratings.DEBUT_PRIOR_KEYS`) —
with the **as-of debut profile of the player's age band**: per format and band, the state
pools, over every earlier player who debuted in the format at that age, what he did *in
his debut match* — the impact numerator (runs above expectation, runs saved), the balls, the
wicket numerator and the matches with at least one ball (`RatingState.debut_bat` /
`debut_bowl`, shape formats × bands × 4) — and reads the pooled sums through the *same
formulas* `side_vectors` applies to a player's own sums: balls per match batted (bowled),
and each impact shrunk over `PRIOR_BALLS` (`ratings.debut_prior_vectors`). A band nobody has
debuted in yet therefore reads exactly the neutral vector from the formula, not from a
second code path. Elo and the role keys keep their neutral values: the prior is about what a
debutant of that age does with the ball, not where he bats.

*Bands.* Four cuts at **22 / 26 / 30 / 34** years (`contract.AGE_BANDS`, five bands), chosen
from the population before any outcome was read: the quartiles of age at match date sit at
24.3 / 27.8 / 31.3 (ODI), 24.6 / 28.2 / 32.1 (T20), 24.5 / 28.0 / 31.4 (T20I), so the cuts
straddle them with a young band under 22 and an old band from 34.

*Who it touches.* `side_vectors` applies the prior only where `career == 0` in the format
**and** a date of birth exists. Every player with a match behind him reads what he read
before, to the last bit; every debutant without a date of birth reads the neutral vector.
The pool itself accumulates at day close, for the XI members whose `career` was 0 when the
match was read and whose age is known, from the same per-ball quantities `_accumulate`
lands on the player — pooled by band and never decayed, because it is a population prior.
It reaches the win models through the side aggregates (`imp_bat_sum` is Σ rate × balls, so a
side fielding a known-age debutant changes), the optimiser through the same `side_vectors`,
the performance model through the debutant's own row, and E5's previous eleven through the
as-of path, all read at the fixture's date (`on=`), with a live request reading the state's
own date. Off unless `contract.AGE_AWARE_COLD_START` is true; recorded in the artifact and
the run manifest.

*A limitation, stated.* The pool has no decay, so the archive's first seasons — when every
player is a "debutant" and the context baseline is still its prior (1.2 runs per ball in
Test cricket that scores 0.5) — sit in the lifetime sums for ever. By the folds they are
diluted (the T20 bands hold 190–1,150 debut matches each, TEST 94–857), but a debutant of
2024 is read against a pool that includes 2003's warm-up. A decayed or era-windowed pool is
the fix if the family were kept; it is not built for a family that is not.

**H-21 audit — what the two families consume.**

1. *A date of birth is static and knowable.* The same value is right for a 2010 row and a
   2026 request, and nothing in `update` touches the map; the age on a row is a function of
   that map and the match date, both known before the toss. The unit test that guards H-1
   is unaffected because the columns are not accumulators; `test_serving_rows_carry_the_same_age_columns_as_the_training_frame`
   asserts the serving path spells them as the frame does.
2. *Wikidata's date precision.* A year-precision `P569` renders as the first of January and
   the acquisition did not carry the qualifier (§ X-1a), so an age here can be up to a year
   high for such a player. It is a bounded, symmetric-in-sign error the model sees on both
   sides of a cutoff, recorded rather than smoothed.
3. *The style label is not applied historically, because it is not applied at all.* The
   prompt's caution — a current-day style label applied to a bowler's early career
   mislabels the years before he changed — would have been a limitation of family 2. Family
   2 did not run, so no current-day fact is projected backward anywhere in this item. The
   date of birth has no such problem: it does not change.
4. *The debut pool is as-of.* It accumulates at day close, after the match's rows are built
   (H-18), so a debutant's own debut is never in the pool his row reads; every entry is an
   earlier player's earlier match. The two-pass frames are checked row for row before a
   format's folds run — the per-player vectors of every row with history must be identical
   between the prior-off and prior-on passes — so the only rows the prior may move are the
   debut rows and, through the side aggregates, the rows of sides fielding a known-age
   debutant; the check's number is under *Results*.
5. *Outcome columns.* No target enters either family. `performance_feature_cols` still
   excludes every target column with or without `age`, and the pool's sums are runs above
   expectation and balls, never who won.
6. *In-sample stacking.* Nothing fitted produces either input; the age is arithmetic and the
   pool is sums. The one fitted consumer of L2-B keeps its temporal fold.
7. *Serving.* A live request reads `age` at the state's ratings-through date rather than the
   fixture's date (the serving match is stamped `state.last_date`), so a served age can be
   up to H-11's 14 days younger than the true one — 0.04 years, under the year-precision
   error above, and the band a live debutant is read in is the same for all but a player
   whose birthday falls in that fortnight. The as-of path reads the fixture's own date.

**H-23 triples** (`ml.xi.gates`, registered before the script ran; the script prints them
first):

- **Gate X-1b-age.** *varies:* whether the performance model reads `AGE_COLS` — `none`
  (today's model) or `age` — one fit per arm per fold; a player without a date of birth
  reads age 0 with the indicator 0. *fixed:* the rows (one frame, the age columns on every
  row, both arms read the same rows), the eleven quarterly cutoffs (A-4's rotated set), the
  three seeds, the hyperparameters, the structure per target, every other input column,
  the labels; the simulator is not run — this is a performance-model gate. *decides:* the
  family is kept only if, against the no-age arm on the same folds, the mean pinball loss
  of runs or of wickets improves by more than **0.5 %** (E1's noise band) **and** by more
  than **one fold-level standard error** of the paired difference (the floor §8.9 asked the
  next gate of this kind to state), in **both T20 and ODI**, with every quantile headline
  target's 10–90 coverage within ± 0.03 of the control's (H-22; width reported beside it).
  T20 is decided on men's rows; women's T20 (61.4 %) is reported and never decides. T20I
  and TEST are reported. A recorded null ships nothing.
- **Gate X-1b-cold-start.** *varies:* what a player with no history in the format and a
  known age reads from the state — the neutral vector (today's cold start) or his age
  band's as-of debut profile — two passes over the source, every model refitted per fold
  on each pass's frame. *fixed:* the source, the dates of birth, the age bands, the
  cutoffs, the seeds, the hyperparameters, the model classes, the performance model's
  columns (family 1's decided setting, the same for both arms), the labels, and the H-10
  probe: the same 50 evaluation matches per fold, the same replaced player (team1's lowest
  player Elo), the same probe ages (19 / 27 / 34 / unknown), the end-of-pass debut tables.
  *decides:* kept only if, in **both T20 and ODI**: (a) **H-10 stays bounded** — for every
  probe age the arm's median debutant-swap Δp is within ± 0.02 of the control's and its
  10th percentile within ± 0.03 of the control's; (b) the pinball loss of runs or of
  wickets on the **held-out debut rows** (career 0 in the format, the only rows whose own
  vectors the prior changes) improves against the control by more than 0.5 % and more than
  one fold-level standard error; (c) the per-player vectors of every row with history are
  identical between the arms (max abs difference 0.0 — a check, not a metric); and, as a
  guard in every format, the display AUC does not fall by more than one paired fold-level
  standard error and H-4's swap share stays under 2 %. T20 is decided on men's rows, as
  family 1. A recorded null ships nothing.

The performance fits in family 3 are runs and wickets only (`FitSpec.targets`), the two the
gate names, so the two passes cost what one family-1 arm costs. The locked window (≥
2026-09-02) holds no matches on either source and is scored once by `make evaluate` after
the choice, as §8.9–§8.11 did.

**Results — family 1, age in the performance model** (`scripts/experiments/xi/x1b_biography_features.py --family age`,
run 2026-09-04 on the archive frames built with the exported dates of birth; eleven folds
2024-01 … 2026-06, three seeds, every target fitted, the simulator not run). One row per
format and slice; "age known" is the share of the slice's evaluation rows with a date of
birth; each pinball cell is control → age with the paired difference (control − age,
positive = age better) ± its fold-level standard error and its size relative to the
control; the verdict is the registered rule applied on the deciding slice:

| format | slice | folds | rows | age known | runs pinball none → age (Δ ± se, rel) | wickets pinball none → age (Δ ± se, rel) | balls pinball | conceded pinball | runs coverage none → age | runs width | balls coverage | conceded coverage | verdict |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| T20 | all | 11 | 97,185 | 0.597 | 2.9202 → 2.9197 (+0.0005 ± 0.0005, +0.02 %) | 0.1410 → 0.1410 (+0.0000 ± 0.0001, +0.02 %) | 2.3199 → 2.3200 (−0.0000 ± 0.0001, −0.00 %) | 1.9338 → 1.9338 (+0.0000 ± 0.0004, +0.00 %) | 0.897 → 0.896 | 29.1 → 28.8 | 0.894 → 0.893 | 0.912 → 0.912 | reported |
| T20 | **men (decides)** | 11 | 67,086 | 0.653 | 3.1632 → 3.1624 (+0.0008 ± 0.0006, +0.02 %) | 0.1415 → 0.1415 (+0.0000 ± 0.0001, +0.02 %) | 2.2668 → 2.2668 (+0.0000 ± 0.0002, +0.00 %) | 2.0187 → 2.0187 (+0.0000 ± 0.0004, +0.00 %) | 0.897 → 0.896 | 31.7 → 31.4 | 0.895 → 0.894 | 0.914 → 0.914 | **fails** |
| T20 | women | 11 | 30,099 | 0.463 | 2.3718 → 2.3721 (−0.0003 ± 0.0007, −0.01 %) | 0.1401 → 0.1400 (+0.0001 ± 0.0002, +0.08 %) | 2.4363 → 2.4365 (−0.0003 ± 0.0004, −0.01 %) | 1.7442 → 1.7441 (+0.0001 ± 0.0006, +0.01 %) | 0.898 → 0.897 | 23.4 → 23.2 | 0.892 → 0.892 | 0.910 → 0.909 | reported (out of scope) |
| T20 | debut | 11 | 3,381 | 0.136 | 2.1510 → 2.1514 (−0.0004 ± 0.0018, −0.02 %) | 0.1574 → 0.1577 (−0.0003 ± 0.0004, −0.19 %) | 2.2066 → 2.2057 (+0.0009 ± 0.0005, +0.04 %) | 2.7143 → 2.7160 (−0.0016 ± 0.0019, −0.06 %) | 0.898 → 0.896 | 18.5 → 18.0 | 0.888 → 0.887 | 0.897 → 0.898 | reported |
| T20 | low history (≤ 2) | 11 | 8,644 | 0.159 | 2.0020 → 1.9993 (+0.0026 ± 0.0013, +0.13 %) | 0.1463 → 0.1464 (−0.0000 ± 0.0003, −0.02 %) | 2.0415 → 2.0411 (+0.0004 ± 0.0006, +0.02 %) | 2.3203 → 2.3208 (−0.0005 ± 0.0013, −0.02 %) | 0.902 → 0.901 | 18.2 → 17.8 | 0.894 → 0.893 | 0.891 → 0.892 | reported |
| ODI | **all (decides)** | 11 | 24,324 | 0.837 | 4.7136 → 4.7126 (+0.0011 ± 0.0018, +0.02 %) | 0.1591 → 0.1591 (−0.0000 ± 0.0001, −0.01 %) | 5.2945 → 5.2945 (−0.0000 ± 0.0013, −0.00 %) | 2.8839 → 2.8833 (+0.0006 ± 0.0009, +0.02 %) | 0.900 → 0.899 | 47.7 → 47.5 | 0.900 → 0.901 | 0.915 → 0.914 | **fails** |
| ODI | men | 11 | 16,554 | 0.806 | 4.8163 → 4.8171 (−0.0009 ± 0.0024, −0.02 %) | 0.1608 → 0.1607 (+0.0001 ± 0.0001, +0.05 %) | 5.2810 → 5.2821 (−0.0011 ± 0.0014, −0.02 %) | 2.9350 → 2.9341 (+0.0010 ± 0.0011, +0.03 %) | 0.902 → 0.900 | 48.6 → 48.3 | 0.900 → 0.901 | 0.916 → 0.915 | reported |
| ODI | women | 11 | 7,770 | 0.886 | 4.4217 → 4.4171 (+0.0046 ± 0.0026, +0.10 %) | 0.1577 → 0.1579 (−0.0002 ± 0.0004, −0.14 %) | 5.2057 → 5.2030 (+0.0027 ± 0.0024, +0.05 %) | 2.7881 → 2.7885 (−0.0004 ± 0.0013, −0.01 %) | 0.904 → 0.903 | 45.7 → 45.6 | 0.905 → 0.905 | 0.915 → 0.916 | reported |
| ODI | debut | 11 | 1,025 | 0.465 | 3.5317 → 3.5484 (−0.0167 ± 0.0229, −0.47 %) | 0.1946 → 0.1946 (−0.0000 ± 0.0011, −0.02 %) | 4.5591 → 4.5683 (−0.0092 ± 0.0112, −0.20 %) | 4.5850 → 4.5882 (−0.0032 ± 0.0075, −0.07 %) | 0.908 → 0.896 | 34.3 → 32.9 | 0.906 → 0.905 | 0.895 → 0.892 | reported |
| ODI | low history (≤ 2) | 11 | 2,888 | 0.497 | 3.4460 → 3.4463 (−0.0004 ± 0.0080, −0.01 %) | 0.1843 → 0.1839 (+0.0004 ± 0.0007, +0.23 %) | 4.3362 → 4.3398 (−0.0036 ± 0.0044, −0.08 %) | 3.5467 → 3.5494 (−0.0027 ± 0.0035, −0.08 %) | 0.923 → 0.915 | 34.9 → 34.1 | 0.913 → 0.912 | 0.907 → 0.905 | reported |
| T20I | all | 10 | 9,532 | 0.947 | 3.2236 → 3.2241 (−0.0006 ± 0.0017, −0.02 %) | 0.1315 → 0.1317 (−0.0002 ± 0.0002, −0.18 %) | 2.2882 → 2.2877 (+0.0005 ± 0.0007, +0.02 %) | 1.8941 → 1.8931 (+0.0009 ± 0.0007, +0.05 %) | 0.888 → 0.888 | 31.1 → 31.1 | 0.884 → 0.884 | 0.902 → 0.901 | reported: fails |
| T20I | men | 10 | 5,392 | 0.975 | 3.4637 → 3.4672 (−0.0035 ± 0.0021, −0.10 %) | 0.1356 → 0.1360 (−0.0004 ± 0.0004, −0.29 %) | 2.2912 → 2.2912 (−0.0000 ± 0.0011, −0.00 %) | 1.9757 → 1.9749 (+0.0008 ± 0.0008, +0.04 %) | 0.884 → 0.884 | 33.0 → 33.0 | 0.878 → 0.879 | 0.904 → 0.902 | reported |
| T20I | women | 10 | 4,140 | 0.914 | 2.9196 → 2.9180 (+0.0016 ± 0.0024, +0.05 %) | 0.1252 → 0.1251 (+0.0001 ± 0.0005, +0.06 %) | 2.2838 → 2.2831 (+0.0006 ± 0.0012, +0.03 %) | 1.7948 → 1.7937 (+0.0011 ± 0.0015, +0.06 %) | 0.892 → 0.892 | 28.4 → 28.4 | 0.891 → 0.888 | 0.898 → 0.898 | reported |
| T20I | debut | 10 | 207 | 0.671 | 2.5401 → 2.5485 (−0.0084 ± 0.0085, −0.33 %) | 0.1668 → 0.1717 (−0.0049 ± 0.0028, −2.92 %) | 1.9858 → 1.9835 (+0.0023 ± 0.0040, +0.12 %) | 3.4600 → 3.4542 (+0.0058 ± 0.0068, +0.17 %) | 0.906 → 0.910 | 24.6 → 24.0 | 0.919 → 0.916 | 0.872 → 0.872 | reported |
| T20I | low history (≤ 2) | 10 | 573 | 0.710 | 2.4412 → 2.4469 (−0.0057 ± 0.0054, −0.23 %) | 0.1473 → 0.1505 (−0.0032 ± 0.0012, −2.16 %) | 1.8059 → 1.8055 (+0.0004 ± 0.0033, +0.02 %) | 2.5580 → 2.5627 (−0.0047 ± 0.0047, −0.18 %) | 0.914 → 0.915 | 22.8 → 22.5 | 0.918 → 0.916 | 0.890 → 0.890 | reported |
| TEST | all | 11 | 9,812 | 0.921 | 8.3351 → 8.3372 (−0.0022 ± 0.0030, −0.03 %) | 0.3037 → 0.3034 (+0.0003 ± 0.0006, +0.10 %) | 13.7828 → 13.7778 (+0.0050 ± 0.0074, +0.04 %) | 5.9513 → 5.9544 (−0.0031 ± 0.0030, −0.05 %) | 0.774 → 0.774 | 82.1 → 82.1 | 0.778 → 0.777 | 0.915 → 0.914 | reported: fails |
| TEST | debut | 11 | 422 | 0.601 | 7.3233 → 7.3197 (+0.0036 ± 0.0416, +0.05 %) | 0.4808 → 0.4754 (+0.0055 ± 0.0065, +1.14 %) | 12.2955 → 12.2025 (+0.0930 ± 0.0458, +0.76 %) | 10.4626 → 10.4458 (+0.0168 ± 0.0451, +0.16 %) | 0.782 → 0.775 | 76.2 → 75.4 | 0.778 → 0.787 | 0.895 → 0.892 | reported |
| TEST | low history (≤ 2) | 11 | 1,006 | 0.659 | 7.5944 → 7.6127 (−0.0183 ± 0.0171, −0.24 %) | 0.3867 → 0.3824 (+0.0043 ± 0.0034, +1.11 %) | 13.2333 → 13.1992 (+0.0341 ± 0.0297, +0.26 %) | 8.3910 → 8.3764 (+0.0146 ± 0.0135, +0.17 %) | 0.770 → 0.771 | 73.5 → 72.9 | 0.771 → 0.773 | 0.907 → 0.897 | reported |

*(TEST's men's and women's rows — 9,680 and 132 — are in the run's JSON and add nothing the
whole-population row does not say.)*

**Decision — family 1 is a recorded null.** In every format the deciding deltas are of
the order of 0.02 % — runs +0.02 % (T20 men), +0.02 % (ODI), −0.02 % (T20I), −0.03 %
(TEST); wickets +0.02 %, −0.01 %, −0.18 %, +0.10 % — twenty-five times under E1's 0.5 %
band, and inside one fold-level standard error on every deciding slice except T20 men's
runs (+0.0008 ± 0.0006, an improvement of 0.02 % that clears one standard error and not
the band) and T20I's wickets (−0.0002 ± 0.0002, a *worsening* of 0.18 %). Coverage moves by at most 0.002
and width by at most 0.3 runs (narrower, by an amount that means nothing at that
coverage). `contract.AGE_FEATURES_KEPT` stays False and the performance model reads no age
column; the columns stay on every row, as the sequence and fixture-context families did,
so the question can be re-asked without a new pass.

**Reading it.**

- *The model already knows most of what age says.* The tree reads `career` (matches in the
  format), `career_all`, the decayed rates and the expected role, and a player's age is
  strongly collinear with those: a 34-year-old with 200 matches and a 21-year-old with 4
  are already different rows. What age adds is the residual — the trajectory of a player
  *at* a given history — and on these folds that residual is worth 0.02 % of pinball, which
  is to say nothing the loss can see.
- *The low-history slices are where age could have shown, and they say the same.* On the
  debut and ≤ 2-match rows — where the ratings carry least and an age prior has most room —
  the deltas are −0.5 … +0.1 % for runs with standard errors two to five times their size
  (1,025 ODI debut rows spread over eleven folds), and the only readings past one standard
  error are wickets *worsening* on T20I's 207 debut rows (−2.9 % ± 1.0) and T20's low-history
  runs improving by +0.13 % ± 0.07: both under the band, opposite in sign, and on the
  smallest slices in the table. There is no age effect hiding in the debutants that the
  population average buried.
- *Coverage in the windows is lower than X-1a's archive figure, and the scoping held.* The
  folds' rows carry a date of birth for 65 % of men's T20 rows (X-1a: 82.6 % of all-time
  appearances), 46 % of women's (61.4 %), 84 % of ODI, 95 % of T20I and 92 % of TEST: the
  recent windows hold more associate and domestic newcomers than the archive as a whole,
  which is where Wikidata's coverage falls away. The men's-only T20 verdict is the same
  as the whole-population one to the second decimal of a percent, and the women's T20 rows
  — reported, never deciding — read −0.01 % / +0.08 %, the indicator doing exactly the
  work it was put there to do.
- *Cost:* two columns on every row; the fit time is unchanged (136–142 s per T20 fold,
  either arm).

**Results — family 3, the age-aware cold start** (`--family cold-start`, run 2026-09-04 on
the two archive passes, the prior off and on; eleven folds, three seeds; the performance
model reads no age column, family 1 having failed its gate before this ran). Per format:
check (c) on the whole frame, then the fold means — the display AUC (control → prior, with
the paired difference, positive = prior better), H-4's swap share under the prior, and the
pinball of runs and wickets on the debut rows (control → prior, control − prior ± its
fold-level standard error, relative) with the whole-population reading beside it:

| format | folds | debut rows scored | history vectors max abs diff | debut vectors moved | rows whose side aggregates moved | display AUC none → prior (Δ ± se) | swap share (prior) | debut runs pinball none → prior (Δ ± se, rel) | debut wickets pinball none → prior (Δ ± se, rel) | population runs pinball | population wickets pinball | verdict |
|---|---:|---:|---:|---|---|---|---:|---|---|---|---|---|
| **T20** (men decide) | 11 | 3,381 | **0.0** | 5,544 of 11,707 | 55,990 of 265,005 | 0.730 → 0.729 (−0.0005 ± 0.0010) | 0.0031 | 2.1510 → 2.1574 (−0.0064 ± 0.0032, **−0.30 %**) | 0.1574 → 0.1582 (−0.0008 ± 0.0005, **−0.53 %**) | 3.1632 → 3.1641 (−0.0009 ± 0.0004, −0.03 %) | 0.1415 → 0.1416 (−0.0001 ± 0.0001, −0.09 %) | **fails: debut pinball does not improve** |
| **ODI** | 11 | 1,025 | **0.0** | 3,842 of 5,191 | 37,827 of 109,267 | 0.708 → 0.706 (−0.0025 ± 0.0036) | 0.0000 | 3.5317 → 3.5559 (−0.0243 ± 0.0257, **−0.69 %**) | 0.1946 → 0.1967 (−0.0022 ± 0.0020, **−1.11 %**) | 4.7136 → 4.7160 (−0.0024 ± 0.0017, −0.05 %) | 0.1591 → 0.1588 (+0.0003 ± 0.0002, +0.19 %) | **fails: debut pinball does not improve** |
| T20I | 10 | 207 | 0.0 | 2,299 of 3,267 | 18,553 of 45,090 | 0.750 → 0.745 (−0.0048 ± 0.0042) | 0.0055 | 2.5401 → 2.6088 (−0.0688 ± 0.0418, −2.71 %) | 0.1668 → 0.1767 (−0.0099 ± 0.0044, −5.92 %) | 3.2236 → 3.2242 (−0.0007 ± 0.0014, −0.02 %) | 0.1315 → 0.1320 (−0.0005 ± 0.0005, −0.39 %) | reported: fails (debut pinball, display AUC guard) |
| TEST | 11 | 422 | 0.0 | 1,764 of 1,964 | 18,538 of 46,040 | 0.643 → 0.649 (+0.0061 ± 0.0092) | 0.0023 | 7.3233 → 7.4991 (−0.1758 ± 0.0914, −2.40 %) | 0.4808 → 0.4966 (−0.0158 ± 0.0165, −3.28 %) | 8.3351 → 8.3390 (−0.0039 ± 0.0066, −0.05 %) | 0.3037 → 0.3043 (−0.0006 ± 0.0012, −0.19 %) | reported: fails (debut pinball) |

H-10's probe — team1's lowest-Elo player replaced by a debutant in the first 50 evaluation
matches of every fold, Δp of the objective, marginalised — control against the prior, per
probe age (means over folds of the per-fold median / 10th / 90th percentile):

| format | probe age | control Δp median / p10 / p90 | prior Δp median / p10 / p90 | bounded |
|---|---|---|---|---|
| T20 | 19 | −0.0337 / −0.0773 / −0.0084 | −0.0298 / −0.0788 / +0.0016 | yes |
| T20 | 27 | −0.0337 / −0.0773 / −0.0084 | −0.0309 / −0.0797 / +0.0014 | yes |
| T20 | 34 | −0.0337 / −0.0773 / −0.0084 | −0.0344 / −0.0835 / −0.0016 | yes |
| T20 | unknown | −0.0337 / −0.0773 / −0.0084 | −0.0353 / −0.0784 / −0.0096 | yes |
| ODI | 19 | −0.0217 / −0.0606 / +0.0027 | −0.0320 / −0.0737 / −0.0030 | yes |
| ODI | 27 | −0.0217 / −0.0606 / +0.0027 | −0.0211 / −0.0618 / +0.0070 | yes |
| ODI | 34 | −0.0217 / −0.0606 / +0.0027 | −0.0142 / −0.0549 / +0.0143 | yes |
| ODI | unknown | −0.0217 / −0.0606 / +0.0027 | −0.0346 / −0.0733 / −0.0072 | yes |
| T20I | 19 | −0.0208 / −0.0693 / +0.0144 | −0.0299 / −0.0776 / +0.0059 | yes |
| T20I | 27 | −0.0208 / −0.0693 / +0.0144 | −0.0202 / −0.0690 / +0.0139 | yes |
| T20I | 34 | −0.0208 / −0.0693 / +0.0144 | −0.0029 / −0.0444 / +0.0347 | yes |
| T20I | unknown | −0.0208 / −0.0693 / +0.0144 | −0.0392 / −0.0890 / −0.0034 | yes |
| TEST | 19 | −0.0127 / −0.0422 / +0.0073 | −0.0298 / −0.0595 / −0.0090 | yes |
| TEST | 27 | −0.0127 / −0.0422 / +0.0073 | −0.0201 / −0.0473 / +0.0023 | yes |
| TEST | 34 | −0.0127 / −0.0422 / +0.0073 | −0.0158 / −0.0437 / +0.0065 | yes |
| TEST | unknown | −0.0127 / −0.0422 / +0.0073 | −0.0140 / −0.0419 / +0.0074 | yes |

**Decision — family 3 is a recorded null.** The prior does what it was built to do and
the forecasts are worse for it. `contract.AGE_AWARE_COLD_START` stays False; the pool and
the read path stay in the code, off, so the question can be re-asked with a different
prior without a new design.

**Reading it.**

- *Three of the four clauses pass, and the one that decides fails everywhere.* H-10 stays
  bounded for every probe in every format — the prior moves a debutant's Δp by at most
  0.018 at the median and 0.020 at the 10th percentile, against bounds of 0.02 and 0.03 —
  and it moves it in the direction the mechanism claims: in ODI a 19-year-old debutant
  reads −0.032 against a 34-year-old's −0.014, in T20I −0.030 against −0.003, so the state
  now says that an old debutant is closer to the player he replaces than a young one, which
  is what the debut pools contain. No player with history moved (max abs difference 0.0 on
  every per-player vector column, 41,823 T20I rows, 253,298 T20 rows). H-4 holds under the
  prior (0.0000–0.0055). But the **debut rows' pinball worsens in every format and for both
  targets**: runs −0.30 % (T20 men, ± 0.15), −0.69 % (ODI, ± 0.73), −2.71 % (T20I), −2.40 %
  (TEST); wickets −0.53 % (T20, one standard error), −1.11 % (ODI), −5.92 % (T20I, two
  standard errors), −3.28 % (TEST). The gate asked for an improvement beyond 0.5 % and one
  standard error; the arm delivers a *degradation* that in T20 and T20I clears both.
- *Why a truthful prior can worsen the forecast.* The performance model already reads
  `career` = 0 and `exp_balls_faced` = 0 for a debutant, and has fitted, from every earlier
  debut row, what a debutant does — which is the same population the pool averages, only
  learned *jointly* with the side aggregates, the venue and the innings rather than as a
  band mean. Handing it the band mean as if it were the player's own history moves the row
  onto the part of the input space where `exp_balls_faced` is 30 and `bat_rate` −0.1 — the
  region of established, poor batters — and the tree reads him as one: the pool's balls
  per match batted is the *conditional* mean (matches with at least one ball), so a
  debutant who bats in half his matches is read as one who bats every match. The model
  was already regressing the debutant to the right neutral; the prior replaced a learned
  neutral with an arithmetic one, and lost the conditioning.
- *The population reading is flat and the display AUC does not move,* which is the
  expected shape: debut rows are 3.5 % of T20's evaluation rows and 4.2 % of ODI's, and the
  side aggregates move on 21–41 % of rows by amounts the win models do not resolve
  (−0.0005 ± 0.0010 T20, −0.0025 ± 0.0036 ODI). T20I's display AUC falls by 0.0048 ± 0.0042
  — the guard's one failure, on the smallest format, reported and not decided on.
- *What this says about H-10.* The cold start measured in P-2 (median −0.003, p10 −0.05
  for a debutant swap) is, on A-4's folds and the lowest-Elo replacement, median −0.034 /
  −0.022 / −0.021 / −0.013 (T20 / ODI / T20I / TEST) and p10 −0.077 / −0.061 / −0.069 /
  −0.042 under today's neutral vector — bounded, and the H-10 row is updated with these
  numbers. Shaping that neutral by age is not the improvement; if a better debutant prior
  exists it is a *conditional* one (the pool split by expected role, or the model's own
  learned neutral left alone), which is a different design and not this item's.
- *Cost:* two passes (98 s each) and, per format, a win-model fit, the two probes and a
  runs + wickets fit per arm and fold — 23 minutes for T20, ten for ODI, six for T20I.

**The harness after the choice** (`make evaluate`, run once on each source with the
decided configuration — no family kept — 2026-09-04; 88 min on the database, run beside
the last cold-start folds, and 60 min on the archive with `BIRTH_DATES=` pointing at the
exported CSV). Both passes read 22,818 matches, 21,096 training rows, 465,402 player rows
and **6,955 players with a date of birth** (`players_with_birth_date`, the new count both
sources agree on). The locked window (≥ 2026-09-02) now holds the three matches imported
since A-3 — two T20, scored as "window too small" — and no fold model, so H-8 serves the
last fold's (`parity_model_window: 2026-06-01`). The walk-forward table is A-4's baseline
(`docs/FOLLOW_UP_PLAN.md` § 5) to the printed decimals on both sources — T20 objective
0.697 ± 0.039, display 0.730 ± 0.050, runs pinball 2.921, wickets Spearman 0.477 (−0.034);
ODI 0.673 / 0.707 / 4.715; T20I 0.756 / 0.753 / 3.220; TEST 0.626 / 0.646 / 8.334; E5 T20
0.503 against 0.506 (fails, not served), T20I 0.564 / 0.472, ODI 0.566 / 0.503, TEST 0.549
/ 0.478 — with the three new matches accounting for the last-decimal moves in the
simulator's totals (T20 first innings 0.772 / 84.8 / +0.5 as the baseline; ODI 0.753 /
150.0 / +0.1 against 0.758 / 149.6 / +0.4). H-8: 50 matches, 1,100 player rows, 1,100
performance predictions, 50 simulations at max abs difference **0.0 on both sources**, the
two age columns now among the columns compared on every player row. Gates (X-1b-age and
X-1b-cold-start in the embedded registry) and glossary pass on both.

---

## 9. Database schema and pipeline steps: what changes, what does not

The short answer is that the schema is not torn apart; it is *pruned by consequence*. The
event core stays exactly as it is, two identity columns are added, and roughly half the
tables lose their last reader as the models that read them go. Deletions happen in the
migration PR that removes the reader, never before, and always as a migration (the repo's
no-backward-compatibility rule applies: no shims, no views kept "just in case").

### 9.1 Tables

| table | today | target | when |
|---|---|---|---|
| `match`, `match_inning`, `match_format`, `match_player`, `ball_event`, `opposition`, `player`, `venue`, `season` | the event core | **keep, unchanged.** `ball_event` already carries striker, bowler, runs split, extras kind, wicket kind, `player_out_id`, `fielder_ids`, `is_legal`, `ball_seq` — everything the rating pass and the performance model need. No column is added to it | — |
| `player.external_id` (new), `player.name_as_of` (new), `opposition.gender` (new), `opposition.canonical_id` (new) | player identity is by name; 130 team names span both genders | **done (P-1, migration `0004_identity.sql`)**: Cricsheet registry id on `player`, gender on the team key. `player_name`'s unique constraint is gone — the name is a display attribute now, settled to the spelling of the player's latest match, which `name_as_of` records. `external_id` is nullable-unique with a partial unique index on `player_name` for the rows that have no registry entry (0 today). The migration truncates the match-derived and precomputed tables and the importer writes them again: re-pointing 20 player-bearing columns across 17 tables in place would need the per-match source to say which of two namesakes each row belongs to, which makes the backfill a re-import in disguise. **`canonical_id` (migration `0006`, I-4)** points a superseded team row at the club's current row and is null for a club that has never renamed, so "the club" is `COALESCE(canonical_id, id)`: 524 rows, 514 clubs from ten reviewed renames. The mapping is reviewed data in `configs/team_lineage.json` — a detector run on every import would merge two real clubs the first time a coincidence cleared its threshold — and the importer resolves it by name at the end of a run, because opposition ids are not stable across a rebuild | P-1, I-4 |
| `datasets`, `data_migrations` | import provenance, migration ledger | **keep** | — |
| `feature_raw_stats_snapshots` (2.32M rows, 1.81M duplicates, D-1) | read by precompute, exports, base models | **dropped (P-6, migration `0008`)** | P-6 |
| `player_window_features` | rolling windows for the sequence exports | **dropped (P-6, migration `0008`)** | P-6 |
| nine `*_features` tables from `seqcalc` (`batting_transition`, `bowling_sequence`, `bowling_spell`, `dot_streak`, `event_reaction`, `extras_discipline`, `wicket_mode`, `over_boundary_wicket`, `over_end_pressure`) | precomputed sequence features, read only by the sequence exports and one repo | **dropped (P-6, migration `0008`)**. E1 kept no family (§5.3), so nothing was re-implemented as an as-of accumulator: the calculators went with the tables | P-6 |
| `batting_data`, `bowling_data`, `fielding_data`, `fielding_event` | scorecard tables derived from `ball_event` at import | **kept (survivor of P-6).** Their last *reader* died in P-6 with the feature-history and backtest-feature repos, but the importer still writes them, and P-6's rule is that a table goes only when its last reader and its last writer both die in the same PR. Dropping them means changing the importer, which is a change to the one component §4 marks "keep". Recorded here rather than done: they are now write-only, and the PR that stops the importer writing them is the one that drops them | after P-6 — the importer is the last writer |
| `match_prediction_aggregates` | backtest aggregates cache | **dropped (P-5, migration `0007`)**; L4 writes its report to the artifacts directory. Its last reader (the accuracy trend) and its last writer (the per-match evaluate flow) both scored the deleted models and died together, which is the only condition under which P-5 drops a table | P-5 |
| `weather_data`, `weather_job` | nothing populates them (no venue had coordinates and nothing fetched observations; `docs/weather-not-implemented.md` recorded it and is in git history); read by ops probes, the training snapshot export and prediction defaults | **dropped (P-6, migration `0008`)**, with the ops probe, the `weather.*` config block and the seven `temp/wind/rain/…` columns — the last of which went with the win contract | P-6 |
| `ml_tuned_params` | Optuna / combination-meta parameter store | **dropped (P-6, migration `0008`)** with the auto-tune stack; the hyperparameters a run chose live in `runs/<id>/manifest.json`, beside the artifacts they produced | P-6 |

Net after P-6: 30 tables → 16. Nothing in the kept set changed shape, so the importer, the
ops status probes and the backtest match listing kept working throughout. The four
scorecard tables are the gap between 16 and the 13 this section forecast; they are write-only
now, and the row above says what has to happen before they go.

### 9.2 Why the event core is enough

Everything the rating pass reads — and everything the performance model and simulator will
read — is a function of `(match, match_player, ball_event)` ordered by date. The
S-10 pass reproduces its numbers from the raw JSON in ~100 s; over Postgres it is one
ordered scan of `ball_event` (11.5M rows, ~1.6 GB with indexes on `(match_id, innings,
ball_seq)`, which exist). There is no feature table to keep in sync, so there is no
precompute step to schedule, no snapshot-date ambiguity and no duplicate rows to reconcile.
The one thing the DB does not carry that the JSON does is the registry identifier, which is
P-1.

### 9.3 Pipeline steps (ops console)

| step today | target |
|---|---|
| `fetch`, `extract`, `import` | **kept** (import gained the identity columns, P-1) |
| `precompute` | **removed (P-6)** |
| `export` | **removed (P-6)**; the rating pass writes its frames into the run directory |
| `train_batting`, `train_bowling`, `train_fielding`, `train_extras`, `train_innings`, `train_win`, `train_combination_meta` | **replaced by one `retrain`** step: rating pass → XI win models → performance models → run report → run manifest. P-5 removed six of the seven — a step whose command no longer exists is a broken surface, not a deferred one — and **P-6 folded `train_win` into `retrain`** |
| `auto_tune` | **removed (P-6)**; the three-point grid runs inside `retrain` and records its choice, and its evidence, in the manifest |
| (new) `evaluate` | **done (P-6)**: L4 on demand, writing its report and touching no artifact `current` points at. `Optional` in the registry, so no plan implies it |
| (new) `reload` | **done (P-6)**: point `current` at a run and load it; `POST /admin/reload?run=<id>` |

Six training steps and two feature steps became three: import → retrain → reload, with
`evaluate` beside them. The step registry's `Requires` graph is `retrain` needs `import`,
`reload` needs `retrain`, `evaluate` needs `import` only — asking "what would this have
scored?" is not gated behind producing the artifacts it is not measuring. The "confirm
untuned defaults" prompt (`confirmDefaultParams`) had nothing left to confirm and is gone.

**What retrain does not do, and why.** §9.3 above lists "L4 report" inside `retrain`. It is
the run's *own* report — per-format holdout discrimination, per-target performance with
width beside coverage, and the data-quality gate — not L4's walk-forward. The harness refits
every model per fold per format: measured on this database it takes **54 minutes**, against a
15-minute budget for the whole pipeline. Folding it into every retrain would make the
pipeline unrunnable at any sensible cadence, so it is the `evaluate` step, which is optional
and which an operator runs when a number is going to be quoted. The manifest names the report
the run actually produced, so nothing quotes a measurement of a different run.

### 9.4 Other steps in the request path

- **Pool construction** (`GetBacktestSquadPlayerIDs`, `default_pool_csv`) stays in go-app:
  availability is the caller's knowledge, not the model's.
- **Per-player feature assembly** (`ComputeFeaturesAtCutoffForMatch`, the `allFeats` maps
  threaded through `predictteam`) goes: the ML side computes features from ids. This is the
  largest simplification in `predict_team.go` (~2,500 lines today) and lands with P-5.
- **Reconciliation / rescaling** of player predictions to the win probability goes with
  P-4; the scorecard and the probability come from one simulator.
- **Frontend**: the Workbench, Evaluate and Prediction surfaces keep their routes; the
  auto-tune and per-model training controls disappear (CONSUMER_SURFACES has the pattern),
  and the scorecard gains ranges and marginal values. **Done (P-6)**: the pipeline graph is
  the four registry steps, the step dialog takes a cutoff or a run id in place of the
  auto-tune form, the ops console reports *runs* rather than a formats-by-model-kind matrix,
  and the Workbench reads the run manifest. The ML-model-stats tab went with the endpoint
  behind it.

What is *not* worth doing: normalising `ball_event` further, moving to a columnar store, or
rewriting the importer. The event core is the right shape; the problems were all downstream
of it.

---

## 10. Leakage and ML-standards audit

"No leaks" is a property of mechanisms and gates, not of intentions. This section lists each
leak channel that exists in a system like this, how the plan closes it, and what is *still
open*. The last part matters most: three items below are not yet closed, and the numbers
quoted in this document should be read with them in mind.

### 10.1 Leak channels and their closure

| channel | how it leaked before | closure | status |
|---|---|---|---|
| **Feature sees its own outcome** (S-3c: scoreboard membership as a feature) | export read who batted, which the result decides | features are read from state accumulated over *prior days only*, then the match is folded in; no snapshot table exists to query wrongly. Unit test H-1 | closed, tested |
| **Same-day ordering** | matches on one date processed in id order, so a match could see a same-day result it may have preceded (78% of matches share a date with another in their format) | **day-close batching** in `ml.xi.builder.build`: every match on a date reads prior dates only, the whole day is applied afterwards. Cost ≤ 0.003 AUC (measured). Unit test `test_same_day_matches_do_not_see_each_other` | closed, tested |
| **Future data in as-of state** (D-1 snapshot ambiguity) | duplicate snapshot rows, "latest before match" undefined | no snapshots; state is the pass itself | closed by construction |
| **Outcome-conditioned training population** | rows exist only for players who batted/bowled — decided by the result — and the model learns the population, not the quantity | **rule H-20**: the performance model's training rows are *all XI players*, with involvement predicted as-of (expected balls faced/bowled), never rows selected by what happened. A two-part target (P(bats) × runs given bats) is allowed only if both parts are trained on the unconditional population. The experiment in §1 conditioned on "batted" and is therefore an upper bound for the given-batted quantity, not the deliverable | **closed (P-3)** — L2-B trains and is scored on the unconditional rows; its baselines too |
| **Stacked predictions used in-sample** (classic stacking leak) | the greedy selector, the Monte Carlo and the combination meta-model consumed base-model predictions for matches those models had trained on; a depth-12 RF partly memorises its rows, so second-stage fits learned to over-trust them | **rule H-21**: any model output consumed by another model or by an evaluation must be out-of-sample for that row — as-of ratings by construction, or out-of-fold predictions from a temporal split. The simulator (L2-C) consumes L2-B only through as-of features, never through in-sample fits | closed for P-3's recalibration (fitted on a temporal fold the members did not see); rule for P-4 |
| **Unknowable-at-decision inputs** (toss, innings, batting order) | trained on real values, served constants (S-2, S-8) | marginalised on the serving path (H-3); the display model accepts the toss once known | closed for win and performance (P-3 marginalises the innings and accepts `team1_bats_first`) |
| **Target leakage through team context** | none found: Elo, form, h2h, venue bat-first rate all update *after* the day closes | — | closed |
| **Train / serve skew** | serving aggregated over 11, training over the scorecard | same code path; parity test H-8 turns it into a permanent check | test open — P-2 |
| **Identity leakage** (two people as one, one person as two) | name-keyed ids; men's and women's sides share team ids | P-1; the JSON-path numbers use registry ids and are unaffected | **closed**: players by registry id and teams by gender in P-1, franchise renames in I-4. `make xi-parity` holds both sources to the same keys |
| **Holdout reuse** (model-selection leakage) | — new — | the 2025-09-01 holdout was used in this session to choose feature families, model class, monotone constraints and marginalisation. Every reported number is therefore mildly optimistic as an estimate of *future* performance, even though each individual comparison is valid. Closure: **rule H-19** — L4 evaluates by rolling-origin walk-forward over several cutoffs (e.g. quarterly from 2024-01 to 2025-09) and reports mean ± spread; and a **locked final window** (matches after the latest cutoff, 2025-09-01 → present) is scored once per release, never used for choices. The numbers in this document are the *development* numbers; P-0 reports the first locked-window numbers | **closed (P-2)** — `make xi-evaluate` evaluates by rolling origins and labels the locked window; H-19 has the first walk-forward numbers |
| **Leak canaries** | S-3c was found by comparing the model to one raw column and to the TEST format as control | H-2: best-single-column AUC (done) plus the TEST-control check: a feature whose AUC collapses in TEST but not in limited-overs formats is suspect | **closed (P-2)** — both in `make xi-evaluate`; see H-2 for the first flag it raised |

### 10.2 ML standards, and where the plan stands

| standard | plan | status |
|---|---|---|
| Temporal validation, no random splits | rolling-origin walk-forward + locked window (H-19) | **done (P-2)** — `make xi-evaluate` |
| Reported with uncertainty | ≥ 3 seeds, mean ± spread (H-14); walk-forward spread across cutoffs | done (P-2 adds the cutoff spread) |
| Baselines that are hard to beat | base-rate Brier, best single column, career mean, team Elo — all reported beside the model | done |
| Metric matches the consumer | AUC + monotonicity for an argmax; Spearman/top-k/coverage for a ranking or interval; per target, never pooled (H-12, H-13) | **done (P-3)** — per target and format throughout; the pooled metric has no new reader and goes with its last caller in P-5 |
| Calibration of any probability shown | reliability + Brier; isotonic recalibration on a temporal fold if needed; interval coverage (H-5) | **done for win, performance (P-3) and the simulator (P-4)** — E2 in §8.3 |
| Simulation | derived from the trained model, no training of its own; every input as-of or out-of-sample for the fixture (H-21); totals judged by coverage and width (H-22), P(win) by Brier against the display model (E2); shared-match dispersion from the residual distribution, never a hand-set CV | **done (P-4)** — `ml/xi/simulator.py`, §3 "The innings sample", §8.3 |
| Distribution-aware losses | quantile / Poisson for counts; MAE-optimal points are not the deliverable | **done (P-3)** |
| Progress measured as sharpness at fixed calibration | interval width tracked beside coverage across releases; proper scores (CRPS / pinball / Brier) are the headline, never MAE (H-22) | **reported (P-3)**; the first release is the baseline the next is measured against |
| Reproducibility | dataset sha, cutoff, git sha, hyperparameters and metrics in a run manifest (H-16); deterministic seeds; the pass is a pure function of the event table | P-6 |
| Data-quality gates | undecided matches, sides without squads, namesakes, replacement players counted per run and gated (H-15) | **done** — `ml/xi/quality.py` gates the retrain, `make xi-parity` compares the two sources |
| Serving = training | same feature code, parity test (H-8); ids in, features computed inside | **done (P-2)** — parity runs in the harness and fails it |
| Monitoring in use | staleness of ratings (H-11); prediction-distribution drift per format between runs (add to the manifest diff) | P-6 |
| Hyperparameters not tuned on the test window | grid inside the walk-forward folds only; the locked window never sees a choice | H-19 |
| Sample size honesty | T20I (182) and TEST (157) holdouts cannot resolve differences under ~0.03 AUC; the plan says so wherever they are quoted | done |

### 10.3 What this means for the numbers quoted here

The development numbers (0.73–0.75 AUC objective, 0.74–0.76 display) are leak-free with
respect to features — the mechanism admits no future information — but they are
*selection-optimistic* because one holdout guided several choices. The right expectation
for the first locked-window report in P-0 was a small drop (0.01–0.02 is typical for this
degree of reuse), not a large one; a large one would say something was wrong with the
window rather than the model. The walk-forward spread in P-2 replaces this guess with a
measurement.

**What P-0 measured, and what it did not.** The Postgres run reports objective 0.723 T20 /
0.684 ODI / 0.744 T20I and display 0.751 / 0.726 / 0.721 — every limited-overs figure within
0.01 of the JSON-path number, in both directions. That is a useful negative result about the
*source*: Postgres with name-keyed player ids loses nothing at the aggregate against
Cricsheet registry ids, so E4's delta, if there is one, lives in the women's subset rather
than overall. It is **not** the drop this paragraph was predicting, because the window that
was scored is the same window the choices were made on. Nothing has yet been evaluated on
data that guided no decision. P-2's rolling origin is still the whole of the answer.

**P-0's E4 guess was right.** The delta is not in the women's subset either; §5.1 has the
numbers. What P-0 read as "the source costs nothing at the aggregate" turns out to be true
of every subset the holdout can resolve.

### 10.4 Match identity — found while running P-1, fixed in `fix/match-id-from-source`

The re-import was run three times over the same 22,734 files to check it is reproducible.
Row counts were stable and `player` and `opposition` were byte-identical between runs, but
**per-match squads were not**: 80 `match_player` rows on 14 matches differed between two
runs of the same directory.

The cause was `StableMatchID(date, teams[0], teams[1])` — a hash of the match's *content*,
which is not an identity. Two sides can play twice in a day, and **618 of the 22,734 files
shared a (date, team, team) with another file**, in 309 pairs. That is why the database held
22,425 matches for 22,734 files: 309 real matches had no row of their own.

What the collision did to the data was worse than losing them, because the writes are not
uniform. `match_player` is delete-then-insert per match, so the squad was whichever file's
transaction committed last — scheduling, hence the non-determinism. `ball_event` inserts
`ON CONFLICT (match_id, innings, over, ball) DO NOTHING`, so the *first* file won ball for
ball and the second file's longer innings appended its tail: **63,725 deliveries were lost
and 6,223 were filed under a match they did not belong to** — a splice of two real matches,
read by the rating pass as one. `batting_data` and `bowling_data` upsert on
`(match_id, inning_number, player_id)`, so a collided scorecard held the union of two
matches' players.

**Fixed by keying on the source.** Cricsheet names each file by its own match id
(`1130677.json`) and that is the only match identity the source publishes — nothing inside
the JSON names the match — so `match_id` is now that number, for 22,709 of the 22,734 files.
The 25 named with a prefix (`wi_211824`) cannot be a bigint and keep a hash, which now
includes the file identifier and is logged; derived ids start at `100000000000`, above every
Cricsheet id, so the two spaces cannot be confused. The database now holds **22,734 matches**,
and two imports of the same directory produce byte-identical `match_player` and `ball_event`.

**The check that proves it.** Build the training frame from Postgres and from the raw JSON
directory at the same cutoff and compare. Before the fix, Postgres gave 20,723 rows against
the JSON path's 21,024. After it the two agree exactly — 21,024 rows, 1,710 undecided, and
per format:

| format | n_train | n_holdout | objective AUC, Postgres | objective AUC, JSON |
|---|---:|---:|---:|---:|
| T20  | 10,313 | 1,635 | 0.7208 | 0.7208 |
| T20I |  1,865 |   182 | 0.7466 | 0.7466 |
| ODI  |  4,570 |   375 | 0.6832 | 0.6831 |
| TEST |  1,927 |   157 | 0.5823 | 0.5823 |

The database now holds what the source holds. Note the T20I holdout is 182, which is the
number §10.2 has always quoted from the JSON path; the Postgres path had been reporting 178.

Recovering the 309 matches moved the report by less than the holdouts resolve — objective
+0.001 T20, +0.005 T20I, +0.002 ODI, −0.000 TEST against the same run before the fix — which
is what 1.4% more data should do. The point of the fix is not the AUC; it is that the
measurement now names data that actually exists.

**One difference remained, and it was the JSON path's** — fixed in
`fix/unnamed-substitute-fielder`. Its rating state held 13,570 player keys to Postgres's
13,569, and the extra one was the literal key `name:`. Cricsheet records 469 dismissals
(451 caught, 13 run out, 5 stumped, across 365 matches) whose fielder is
`{"substitute": true}` and nothing else, and `_deliveries_from_cricsheet` keyed all of them
on the empty name, so the JSON path carried one fictional cricketer with a fielding record
assembled from 365 different matches. A fielder the source cannot name is credited to
nobody, which is what the go-app importer has always done
(`Collection.UnmarshalJSON` keeps only entries with a name). The 3,324 substitute fielders
that *are* named keep their credit — a substitute is a person; only an unnamed one is
nobody — and the dismissal itself was never at stake, since it is counted from its kind.

With that, the two sources produce **identical player key sets**: 13,569 on both, none on
either side alone, and no metric moves, because the phantom was absorbing credit no real
player would have received.

Recorded here rather than in P-1 because it is a different kind of error: player and team
identity were read from the wrong *field*, match identity was not read at all. It still
belongs to **H-16** (run identity), which P-6 owns.

**H-15 now exists, and found a third defect on its first real run.** `make xi-parity`
reported `namesake_sides` as 0 from Postgres and **4** from the archive. Cricsheet's
registry is keyed by name within a file, so two people who share a scorecard name collapse
into one identifier: `KV Sharma` for Vidarbha and Railways, `J Butler` for the Isle of Man
and Guernsey. The go-app importer has always dropped such a name from both squads rather
than guessing a side (S-3c). The JSON source did not, so in those two matches it handed one
player's ratings to *both teams at once*. The rule was written down and implemented on one
side only; the gate is what noticed. The JSON source now applies it too, and the two
sources agree on every count:

```
  count                   postgres   cricsheet
  offered_matches            22734       22734
  matches_read               22734       22734
  undecided_matches           1710        1710
  namesake_sides                 0           0
  oversized_squads            1358        1358
  unknown_player_keys            0           0
  player_keys                13569       13569
```

**And a fourth, on the run after that.** I-4 added a `team_keys` count to the comparison —
distinct clubs — and it came back **515 from Postgres against 386 from the archive** (before
the tenth rename was added). The
database had keyed teams by `(name, gender)` since P-1; the JSON source had never done so,
and was still giving Australia's men's and women's sides one Elo and one head-to-head
record. I-3 had been implemented on one side only for two PRs without anything saying so.
Both sources now key a team by its club and its gender, and agree.

Four defects in this area have now been found by comparing two numbers — 22,425 against
22,734, 13,570 against 13,569, 4 against 0, and 515 against 386. That is the argument for
the gate: none of them was subtle, and none of them was visible. Two of the four were found
*by* the gate, within minutes of it existing, and the fourth was found by extending it while
making an unrelated change.

**A fifth, found by H-8 in P-3 — delivery order.** The Postgres source read a match's
deliveries `ORDER BY innings, ball_seq`, and `ball_seq` is the *legal*-ball counter: a wide
or no-ball carries the same number as the legal delivery before it, 238,772 such pairs in
the table, and the order within each pair was whatever the planner produced. Every feature
before P-3 was a sum over deliveries and could not tell; the sequence families are the first
to read the order, and the parity check's first Postgres run found two players whose
dot-streak shares differed by 1e-5 to 2e-4 between the training pass and the as-of rebuild
of the same match. Fixed by ordering on `(innings, over, ball)`, which is unique and is the
source's order; the Cricsheet path was never affected. Small — but it is precisely the class
of difference that turns a reproducible number into one that drifts between two runs of the
same command, and the check exists so that it is a failed run rather than a mystery.

**A sixth, found by comparing the two sources' performance numbers — fielders.** With the
sequence order fixed, the two harness reports agreed on every headline target to 3 dp and
disagreed on catches everywhere, because the Postgres source read fielders from
`ball_event.fielder_ids`, which the importer has always left NULL ("omitted for
simplicity"), and wrote them to `fielding_event` instead. So from the database the pass
credited no catches and — since the keeper flag is set by the fielder of a stumping — held
no keeper at all: `has_keeper` was 0 for every side and a pool from the database could not
satisfy the optimiser's `require_keeper`. The win model never noticed because `has_keeper`
is not one of its columns. Reading `fielding_event` per ball fixes both; keepers then agree
exactly per format and catches to within 0.4 % — and that residue was the importer's own
defect: a "caught and bowled" special case that could never see a real caught-and-bowled
(its own kind, skipped above it) and so fired only for a *caught* dismissal with no named
fielder, crediting the bowler with 452 catches taken by unnamed substitutes. The archive
path credits such a catch to nobody (§10.4 above); the importer now does the same, and
replaces a match's fielding events on re-import rather than inserting with `ON CONFLICT DO
NOTHING`, which would have kept the 452 rows through every re-import that no longer wrote
them. Re-imported; the two sources now agree on catches and keepers row for row.

### 10.5 Stale rating artifact — found while smoke-testing P-5, fixed in P-6 (D-6)

The sixth defect, and the first found by pointing a browser at a rebuilt container rather
than by comparing two numbers.

`POST /xi/predict-win` — a path P-5 does not touch — returned
`500 index 13433 is out of bounds for axis 1 with size 1024`. The artifact on the box was
written before P-2, so it carries none of the nine arrays P-2 and P-3 added (`bat_pos_sum`,
`bat_pos_n`, `xi_n`, the four phase splits, `seq_num`, `seq_den`). `_state_from_payload`
builds a fresh `RatingState` — every array at the constructor's initial width — registers
the 13,427 saved keys, and then assigns *only the arrays the payload happens to carry*. The
nine it does not carry stay at that initial width, so the first request touching a player
past slot 1024 raises, while `/xi/status` reports `loaded: true, players: 13427`.

Two things were wrong and only one was the array. The other is that **an artifact was
trusted because it loaded**: nothing asked which run produced it or whether that run's state
has the shape this code expects. That is H-16's question, which is why the fix waited for the
run manifest rather than being patched in P-5 — the alternative was to zero-fill nine arrays
and hope.

**Fixed in P-6.** `XiStore.load` takes a *run directory*, reads its manifest first, and
refuses what it cannot serve:

- **No manifest** → `RunArtifactsInvalid`, naming the directory. A pile of joblib files that
  nothing can attribute to a run is not a run.
- **A missing array** → the refusal names every array this code reads and the payload does
  not carry. That is the D-6 artifact itself.
- **A player array narrower than the keys the payload registers** → the refusal names the
  array, its width and the width expected (`pelo has width 2, expected 5`). This is the same
  failure with the arrays half-written rather than absent.

The check distinguishes the 21 player-indexed arrays, whose last axis is the player slot,
from the nine context arrays indexed by (innings, format) or (innings, format, over) —
checking a context array's width against the player count would refuse every healthy run.
Writing that distinction down is itself part of the fix: it was implicit in the constructor
and in nothing else.

The refusal is kept and reported rather than swallowed. `/xi/status`, `/health` and
`/ops/status` carry `error`, `POST /admin/reload` answers 409 `RUN_ARTIFACTS_INVALID`, and
the prediction tab says why a prediction will not be answered. Loading is the last moment an
operator can be told to retrain; an exception caught quietly there becomes an `IndexError`
in a prediction an hour later, which is exactly what happened.

Tests: `ml-service/tests/test_runs_and_reload.py` builds the pre-P-2 artifact (a payload with
five arrays removed) and a half-written one, and asserts both are refused with the run named.

### 10.6 The id contract and a mutating read — found while smoke-testing P-5, fixed in P-5 (D-7)

Two defects on the serving path, both invisible to every test because every test used one
id space throughout.

**D-7a — the wire carried the wrong id.** P-1 keyed the rating state by the Cricsheet
registry id (`player.external_id`, a hex string such as `2911de16`), but the request models
never followed: `pool_player_ids`, `team1_player_ids`, `selected_player_ids` and the
`marginal_values` keys were all `int` — go-app's `player.player_id` — and `xi_service`
stringified them at the boundary. So every id go-app sent missed the store, every player
came back unrated, and `/xi/optimize` chose an XI of eleven debutants while
`unknown_player_ids` sat in a response nothing displayed. Fixed by making the registry id
the type on the wire (`str` throughout `app/models/xi.py`, identity in `xi_service._keys`),
carrying `external_id` on `db.PlayerPoolRow`, and keying go-app's selection path by it —
`PoolPlayerKeys`, `SelectedPlayerKeys`, `MarginalValues map[string]float64` — resolving back
to `player_id` only where the response names a player. A pool row with no `external_id` is
dropped with a warning rather than sent as a key the store cannot know.

This also invalidates P-0's selection gate (§8.1). The arm it scored as "winprob 0.560 vs
greedy 0.569" ran through this contract, and re-running it (§8.5) shows the consequence was
harsher than "chose on all-debutant ratings": with nobody rated, nobody is a bowling option,
so `/xi/optimize` refused **every** limited-overs call on `min_bowlers` and go-app fell back
to the windowed-form optimiser without the caller ever seeing it. The gate never ran the arm
it named. P-5's own decision is unaffected — it ships on L4's gates, which run inside
ml-service and never crossed the boundary.

**D-7b — a read of the rating state mutated it.** `side_vectors`, the read every prediction
takes, resolved its keys through `_slots`, which *assigns* a slot to any key it has not seen.
Serving therefore grew the state: a request naming an unknown player registered him at a
fresh slot with zeroed ratings, and that slot persisted for the life of the process, so the
in-memory state drifted away from the artifact on disk and two identical requests either
side of a third could disagree. Reads now go through `read_slots`, which maps an unseen key
to a single reserved column that no update ever writes, leaving the state exactly as the
artifact left it. The update path still assigns, which is where assignment belongs.

Both are the same shape as §10.5 and §10.4 before them: a boundary that answered
successfully while answering about the wrong thing.

---

## Appendix — experiment record (T20 / ODI batting, 2025-09-01 holdout, batters with ≥ 3 prior innings)

| predictor | T20 MAE | T20 Spearman/match | T20 top-3 hit | ODI MAE | ODI Spearman/match |
|---|---|---|---|---|---|
| global mean | 14.34 | — | 0.333 | 20.73 | — |
| career mean (as-of) | 13.67 | 0.318 | 0.354 | 18.76 | 0.338 |
| EWM mean | 13.85 | 0.310 | 0.346 | 19.10 | 0.330 |
| rating × expected balls | 13.21 | 0.312 | 0.356 | 23.97 | 0.319 |
| GBM, player vectors only | 12.76 | 0.333 | 0.362 | 18.88 | 0.348 |
| GBM + XI context (MAE loss) | **12.06** | 0.338 | 0.367 | **18.01** | 0.349 |
| GBM + XI context (MSE loss) | 12.67 | **0.340** | **0.371** | 18.80 | **0.350** |
| repo RF (sidecar, per-target) | 12.85 | not measured | not measured | 18.61 | not measured |
| quantile model 10–90 coverage | 0.80 | | | 0.79 | |

Wickets per bowling innings: T20 career mean MAE 0.874 / Spearman 0.119; Poisson GBM +
context 0.804 / 0.167. ODI 0.951 / 0.201 vs 0.905 / 0.255.

Script: `scripts/experiments/xi/perf_experiment.py` (reads the parsed frames produced by
`parse.py` and `build_features.py` in the same directory).
