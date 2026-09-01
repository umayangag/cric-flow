# ML pipeline re-architecture: selection first, performance second

**Goal.** Primary: pick the XI from a pool that maximises win probability against a given
opponent and venue. Secondary: predict how each player will perform. Everything in the ML
pipeline should be justified by one of those two, and measured by the metric its consumer
actually needs.

This plan is separate from [WIN_PROB_SELECTION_PR_CHECKLIST.md](WIN_PROB_SELECTION_PR_CHECKLIST.md)
(which repairs the existing selection path and, in S-10, replaces its objective) and from
[ML_PLAN_optimal_xi.md](ML_PLAN_optimal_xi.md) (the original feature and modelling plan, part
of which S-10 implemented). It answers a different question: given what S-9/S-10 showed,
what should the pipeline *be*.

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
- Simulation: calibration of simulated P(win) against outcomes; simulated totals vs actual.

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
| Player identity (`IDENTITY_PR_CHECKLIST.md`) | **keep, do first** | Name-keyed ids merge 348 people into 163 names and men's/women's sides into one team id. Every rating inherits it. The Cricsheet registry id is the fix and the importer already reads it. |
| Precompute (`internal/precompute`, `feature_raw_stats_snapshots`, `player_window_features`) | **remove** | Replaced by L1. Carries D-1. |
| Sequence features (`seqcalc`, `*_features` tables) | **conditional** | Keep only the calculators E1 proves useful, re-implemented inside L1 as as-of accumulators; drop the tables. |
| Export CSVs (`export-dataset`, `exportqueries`) | **remove** | L1 writes the training frames itself. D-2 goes away. |
| `ml.win_features`, `train_win`, `win_discrimination` | **remove after S-6** | Superseded by `ml.xi` + L4. |
| batting / bowling / fielding trainers (RF multi-output) | **replace** by L2-B | Wrong loss, pooled metrics, no context. |
| extras, innings models; reconciliation (`reconciliation_*`, `win_coherence`) | **remove** | Replaced by L2-C. |
| Monte Carlo (`predictteam/simulation.go`, Normal sampling) | **replace** by L2-C | Normal(mean, 0.35·mean) is not a runs distribution. |
| combination meta-model, `score_weights`, greedy selection | **demote** to optimiser seed; delete meta-model (S-5/S-5b cancelled) | Objective is the XI model. |
| Auto-tune stack (Optuna two-phase, PyCaret, AutoGluon) | **remove** | Model class is not the constraint (measured twice). A 10-point grid per model inside L4 is enough and runs in minutes. |
| Per-call hill-climb (`teamselect/optimize.go`) | **remove** | Selection runs in ml-service against L2-A. |
| Backtest API surface (`/api/backtest/*`), frontend tabs | **keep, re-point** | Consumers stay; they read L4's report and L3's outputs. |
| Ops console, pipeline runner | **keep, shorten** | Three steps instead of six. |

Net effect: the ML surface shrinks from six trained models + meta-model + reconciliation
to two model families and one derived simulator, all fed by one pass over one table.

---

## 5. Experiments to run before committing (each ≤ a day, all on the L1 frame)

| id | question | method | decision rule |
|---|---|---|---|
| E1 | Do sequence features add to L2-B? | Ablate `seqcalc` families as extra as-of accumulators in L1; measure Spearman / pinball on the holdout | Keep a family only if it moves pinball loss by > 1% over three seeds; otherwise drop it and its tables — **run in P-3, §5.3: no family moves pinball by more than 0.21 %; none kept** |
| E2 | Is the simulator consistent with the display model? | Simulated P(win) vs display P(win) on holdout matches; calibration of each | If simulated P(win) is worse-calibrated by > 0.01 Brier, keep it as a display-only distribution and never as a probability |
| E3 | Can batting order be optimised? | Expected-slot model + L2-B; for the chosen XI, evaluate objective / simulated totals under permutations of the top 7 | If reordering moves simulated totals by > 3% for > 30% of XIs, add batting-order suggestion to L3; else leave order to the captain |
| E4 | How much does identity cost? | Re-run S-10 on the Postgres source before and after IDENTITY I-3/I-4 | Report the AUC delta; expect the women's-cricket subset to move most — **run in P-1, §5.1** |
| E5 | Natural experiment for selection | Same side, consecutive matches, 1–3 changes: sign agreement between Δobjective and Δresult | If agreement > 55% on ≥ 300 pairs the objective is selecting on real signal; record either way |
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

That is the outcome IDENTITY_PR_CHECKLIST I-5 wrote down in advance as the likely one: 163
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
| P-0 | Land S-10; run its acceptance on the DB; set `selection.win_model: "xi"` for limited-overs formats (S-6) | **run; acceptance not met.** The model reproduces on Postgres (objective 0.72 T20 / 0.68 ODI / 0.74 T20I, display 0.75 / 0.73 / 0.72 — within 0.01 of the JSON path), but selection-comparison over 332 locked-window matches gives `xi` 0.560 vs `greedy` 0.569 winner accuracy, so win-probability selection stays off. The gate is also mis-specified: an arm that optimises *both* sides moves the fixture toward parity and must lose winner accuracy regardless of XI quality. Replace it with L4's specific-XI-beyond-typical-XI, swap monotonicity and E5 |
| P-1 | Identity: Cricsheet registry id as `player.external_id`, team + gender as the team key (IDENTITY I-3/I-4) | **done** (`arch/p-1-identity`). 13,483 name-keyed player rows → **13,623** identity-keyed (140 people recovered, 0 fallbacks); `opposition` 394 → **524** (+130, the predicted count); squad gender disagrees with `match.gender` on 0 rows; the rating pass keys off `external_id` on both sources. E4 recorded below: **no format and no gender subset moves by more than its holdout can resolve.** Re-import is reproducible in row counts and in identity content; per-match squads became reproducible one PR later, with §10.4's match-identity fix. Franchise lineage (I-4) is not in this PR |
| P-2 | L1 emits player-match rows + expected batting slot + phase splits; L4 harness skeleton with the performance metrics; **an as-of serving path** (`XiStore` answers "ratings as of date D", not only "through today") and **per-match rows** in the selection report | **done** (`arch/p-2-rating-rows-harness`). The day-close pass emits 463,818 player-match rows — all XI players, never only those who batted (H-20) — with expected batting slot, innings share and phase-split impact rates, identical from both sources. `ml.xi.asof` answers `ratings_as_of(D)` and raises rather than run backwards, so a backtest at date D provably cannot see D or later; `freeze_ratings.py` is retired, `/xi/*` accept `as_of`, the selection comparison sends each match's date and its report carries per-match rows. `make xi-evaluate` runs the walk-forward + locked window + H-8 parity from one command into one JSON report. Acceptance: career-mean within-match Spearman on the locked window 0.317 T20 / 0.318 ODI (the script's ≈ 0.32 / 0.34, computed there with cross-format career means — inside the 0.31–0.35 band §1 calls the ceiling); parity max abs difference 0.0 on both sources; E7 measured, no effect (§5.2) |
| P-3 | L2-B performance model (quantile runs/balls, Poisson wickets) + `/performance/predict` taking XI ids; E1, E6 | **done** (`arch/p-3-performance-model`). `ml/xi/performance.py`: per format, quantile (0.1 / 0.5 / 0.9) models of runs, balls faced and runs conceded, a two-part zero-inflated Poisson of wickets, a Poisson rate of catches, and P(bats) / P(bowls) — all on the unconditional population (H-20), innings marginalised at prediction, three seeds, a three-point grid tuned inside the folds (flat), E1 (no family kept) and E6 (separate) in §5.3. Walk-forward over 7 folds, 3 seeds (§8.2): **runs** beat the career mean on Spearman (+0.040 ± 0.007 T20, +0.048 ± 0.013 ODI) and pinball (2.93 vs 5.09, 4.59 vs 7.96), median MAE −9 %; **wickets** beat it on pinball (0.141 vs 0.260, 0.163 vs 0.302) and tie on Spearman in ODI (+0.001 ± 0.012) but **trail it by 0.029 ± 0.012 in T20** — a tie-averaging artifact of the unconditional metric, recorded below rather than gamed; among the players who bowled the model ranks better in both. Locked window: per-end coverage inside ±0.03 everywhere, no recalibration triggered (H-5); width reported beside coverage (H-22). `POST /performance/predict` serves it; H-8 parity holds for rows and predictions on both sources at 0.0 — after it found the fifth defect, §10.4 |
| P-4 | L2-C simulator; scorecard and totals from it; E2 | scorecard medians and P(win) come from one source; extras/innings models unused |
| P-5 | Re-point team prediction and backtest surfaces to L2/L3; delete greedy weights, meta-model, reconciliation, Normal Monte Carlo, per-call optimiser | frontend shows ranges + marginal values; `make check-all` green; coverage gates ratchet |
| P-6 | Delete precompute, snapshots, exports, auto-tune stack, old win model; three-step ops pipeline; run-id artifacts | full pipeline from raw JSON to loaded artifacts in one command, < 15 min |
| P-7 | E3 batting-order suggestion; E5 natural-experiment metric in L4 | recorded in the harness report |

Each of P-2 … P-6 removes more than it adds. The end state is smaller than the current tree.

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
| H-4 | **Monotone objective.** Upgrading a player never lowers the selection score | win objective | Harness: share of one-player upgrades with Δp < 0 must stay < 2% (measured 0.4% T20 / 3.7% ODI for logistic; 12% / 20% for unconstrained boosting) | **done (P-2)** — reported per fold in `make xi-evaluate`; walk-forward means 0.3% T20 / 0.8% T20I / 0.0% ODI / 0.6% TEST, all under the 2% line |
| H-5 | **Calibration of what is displayed.** The probability shown must mean what it says | win display, simulator, performance quantiles | Harness: reliability curve + Brier vs base rate per format (done for win); isotonic recalibration fitted on a temporal fold if Brier is worse than base rate; quantile coverage within ±0.03 of nominal (measured 0.80 / 0.79 on 0.80) | **done for the performance quantiles (P-3)**; the simulator's P(win) waits for P-4. The check is per quantile end, because the targets have a point mass at zero: a level must sit between its strict and inclusive exceedance ± 0.03 (`perf_calibration.coverage_off_nominal`). On the locked window P(y ≤ q90) is 0.896 / 0.892 / 0.896 / 0.899 for runs (T20 / T20I / ODI / TEST) and P(y < q10) ≤ 0.12 everywhere; the 3-way wicket probabilities are within 0.02 of observed. No target trips the check, so the isotonic recalibration (`ml/xi/perf_calibration.py`, fitted on the last quarter of the training rows, applied at prediction, unit-tested) ships in place and unused, and the harness re-decides it every run |
| H-6 | **Rating hyperparameters are not load-bearing.** decay and prior were chosen by judgement | rating pass | Measured sweep (`health_experiment.py`): decay 0.80–0.97 and prior 20–150 move objective AUC by ≤ 0.01 in T20 and ODI — flat. Keep 0.90 / 60; re-run the sweep when the pass changes | done |
| H-7 | **Gender- and competition-aware baselines.** Context expectations (runs per ball per over) are per format only; women's and men's matches share them, and so do the IPL and a club league | rating pass | E7: split the context baseline by gender (cheap, gender is on every match); measure the women's-subset AUC before/after. Competition tiers only if E7 shows gender matters | **measured (P-2, §5.2): no effect** — the T20 women's subset moves by 0.000 and the only larger delta sits inside a 74-match holdout while hurting the men's display. The split ships off, behind `--gender-split-context`, to be re-asked when the women's holdouts grow; competition tiers are therefore not pursued |
| H-8 | **Train / serve parity.** The serving store must compute the same features the training frame holds | all | Harness: for the last 50 holdout matches, rebuild the row from the serving store *as of that date* and assert equality with the training frame (the S-3c defect, made a test) | **done (P-2)** — `serving_parity` in `ml.xi.asof` rebuilds the last 50 matches' win *and* player-match rows through the as-of path and fails `make xi-evaluate` on any difference; measured max abs difference 0.0 on both sources. The state evolution is genuinely different code on the two sides (day-close buffering vs a strict date threshold) while row assembly is shared (`ml.xi.rows`), so the D-4 class — a column spelled differently in serving — cannot recur, and drift in the as-of logic is caught. (P-0's find, for the record: the legacy path served a 37-wide vector to a 38-wide scaler over `inning` vs `batting_inning`) |
| H-9 | **Identity.** Ratings keyed by name merge people | all | P-1 (IDENTITY I-3/I-4); E4 measures the delta | **done**: players key off the Cricsheet registry id, teams off (club, gender) — one row per (name, gender) since P-1, folded onto the club by `opposition.canonical_id` since I-4 — on both sources, so the two paths produce the same keys and their artifacts are comparable. E4 found the correction worth ≤ 0.01 AUC everywhere it can be resolved, and the lineage merge is smaller again; both are correctness, not discrimination. S-7's opposition encoding is unblocked |
| H-10 | **Cold start is bounded.** A player with no history must regress to neutral, never explode | win, performance | Measured: replacing a player by a debutant moves p by a median −0.003, p10 −0.05. Unit test on `side_vectors` for an unseen key | done |
| H-11 | **Staleness.** Ratings are only as fresh as the last import | rating pass | `/xi/status` reports `ratings_through`; the ops step fails a prediction request with a clear code if it is older than N days (config, default 14). Retrain is one command and ~2 minutes, so the cadence is "after every import" | open |
| H-12 | **Per-target, never pooled metrics.** A headline number must be for one target on one population | performance | `ml/metrics.py`'s raveled multi-output MAE is retired; L4 reports per target | **done (P-3)** for everything P-3 touches: `ml/xi/perf_metrics.py` scores one target on one population and the harness and `xi_win_report.json` carry the numbers per target and format. `ml/metrics.py` has two callers left, the legacy multi-output trainer and its tuning path, which P-5 deletes; nothing new reads it |
| H-13 | **Consumer metric first.** AUC for an argmax, Spearman/top-k for a ranking, coverage for an interval | all | Every model in L4 has a named consumer and its metric is the one that gates | rule |
| H-14 | **Seeds and noise floor.** Differences under the seed spread are not evidence | all | Every reported number is a mean over ≥ 3 seeds with the spread (done for win) | done |
| H-15 | **Data-quality gate.** Undecided matches, sides without squads, namesakes, replacement players | rating pass | Two checks, both in `ml/xi/quality.py`. **Accounting:** a source offers N matches and must yield, scope out or reject exactly N — a match dropped for a reason nothing names fails the run. **Doubling:** any quality count over twice the last accepted run's, or one that was zero and is not, fails. The accepted counts live in `xi_data_quality_baseline.json`, which a *failing* run does not update, so re-running cannot clear the gate; `--accept-data-quality` is the one way to move it. Beside it, `make xi-parity` runs both sources and compares every count and the player-key sets | **done**. Measured baseline: 22,734 offered = 22,734 read, 1,710 undecided, 0 namesake sides, 1,358 sides over eleven, 0 unresolved player keys, 13,569 player keys, 514 clubs — identical from both sources. Its first two real runs found two more defects; see §10.4 |
| H-16 | **Run identity.** A measurement must name the artifact it measured | all | `runs/<id>/manifest.json` with dataset sha, cutoff, git sha, hyperparameters, metrics (D-3) | open (P-6) |
| H-17 | **Format scope.** Selection is only offered where the objective ranks | win | TEST stays on greedy with a note in the UI; an objective with holdout AUC < 0.65 is not used for selection in that format | rule |
| H-18 | **Day-close batching.** A match never sees a same-day result | rating pass | Implemented in `ml.xi.builder`; unit-tested; cost ≤ 0.003 AUC | done |
| H-19 | **Walk-forward evaluation + locked window.** Choices are made on rolling cutoffs; one final window is scored once per release | all | L4 reports mean ± spread over cutoffs; the locked window (≥ 2025-09-01) is never used for a choice | **done (P-2)** — `make xi-evaluate`: quarterly rolling origins 2024-01 … 2025-06, the locked window scored once and labeled. First per-format walk-forward table in §8.1; the locked-window figures sit inside the fold spreads, toward the top for T20 — what later origins with more training data should produce — so the development-window reuse §10.3 could only estimate is now priced. (P-0's locked-window report, 2025-09-01 → 2026-08-25 with no drop, was the precursor: same window, but the one that guided the choices) |
| H-20 | **Unconditional training population.** Rows are never selected by the outcome (who batted, who bowled) | performance | Training rows are all XI players with as-of expected involvement; two-part targets allowed only if both parts are unconditional | **done (P-3)**: every target trains and is scored on all XI players with "did not bat / bowl" as 0; the baselines are defined on the same population (the unconditional career mean, not the mean over innings batted). The one two-part target, wickets, fits P(bowls) on the unconditional rows and reads the involvement from that classifier at prediction, never from the outcome; a unit test asserts no outcome column is an input |
| H-21 | **No in-sample stacking.** A model output consumed downstream is out-of-sample for that row | performance → simulator, any meta-model | as-of features or out-of-fold predictions from a temporal split; the harness asserts the second stage never scores a row the first stage trained on | **done for P-3's second stage**: the only fitted consumer of a model output is H-5's quantile recalibration, and it is fitted on the last quarter of the training rows by date, which the members do not train on (`performance._temporal_calibration_split`; unit-tested). The two-part mixture is arithmetic, not a fit. The rule still binds the simulator in P-4 |
| H-22 | **Sharpness at fixed calibration is the progress metric.** For a distributional system "better" means narrower intervals while coverage stays nominal, never a smaller point error | performance, simulator | L4 reports mean 80% interval width beside coverage, per target and format, release over release; narrower with coverage held is progress, narrower with coverage falling is a regression and fails the gate. CRPS / pinball as the single proper score | **reported (P-3)** — width beside coverage for every target and format in §8.2 and in `xi_evaluate_report.json`, with the career-quantile baseline's width and coverage beside them. First release: T20 runs 29.1 wide at 0.897 inclusive coverage against the career quantiles' 26.0 at 0.782 — the baseline is narrower only by under-covering. Pinball is the proper score (mean over the three levels; CRPS was not added: two proper scores buy nothing a second column cannot). The release-over-release gate has one release to compare against so far |

Items marked *open* are folded into the migration: H-11 into P-6 alongside H-16 (H-2, H-4,
H-7, H-8, H-15 and H-19 are done as of P-2; H-5, H-12, H-20, H-21 and H-22 as of P-3, bar
what the simulator adds in P-4). Nothing left in the list needs new modelling; it is
measurement, guards and two small serving rules.

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
specific-XI-beyond-typical-XI delta (+0.045 ± 0.021 T20; within noise elsewhere) and the
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
| `feature_raw_stats_snapshots` (2.32M rows, 1.81M duplicates, D-1) | read by precompute, exports, base models | **drop** | P-6, after P-5 re-points the last consumer |
| `player_window_features` | rolling windows for the sequence exports | **drop** | P-6 |
| nine `*_features` tables from `seqcalc` (`batting_transition`, `bowling_sequence`, `bowling_spell`, `dot_streak`, `event_reaction`, `extras_discipline`, `wicket_mode`, `over_boundary_wicket`, `over_end_pressure`) | precomputed sequence features, read only by the sequence exports and one repo | **drop as tables**. Any family that survives E1 is re-implemented as an as-of accumulator inside the rating pass, which is where the same numbers were computed from anyway | P-6 (E1 decides which calculators live on in code) |
| `batting_data`, `bowling_data`, `fielding_data`, `fielding_event` | scorecard tables derived from `ball_event` at import; read by backtest metrics, scorecard display, feature history, the S-3c-era exports | **keep for now, then derive.** They are redundant with `ball_event` (the leak in S-3c came from reading them as if they were squads). Once L4 computes actuals from `ball_event` (P-2) and the scorecard view reads the simulator (P-4), they have no reader and go in P-6. Until then they are the cheapest way to show a real scorecard | P-6 |
| `match_prediction_aggregates` | backtest aggregates cache | **drop**; L4 writes its report to the run directory | P-5 |
| `weather_data`, `weather_job` | nothing populates them (`weather-not-implemented.md`); read by ops probes, the training snapshot export and prediction defaults | **drop**, and with them the seven `temp/wind/rain/…` columns the win contract still carries as constants | P-6 |
| `ml_tuned_params` | Optuna / combination-meta parameter store | **drop** with the auto-tune stack; tuned values live in the run manifest | P-6 |

Net: 30 tables → about 13. Nothing in the kept set changes shape, so the importer, the
ops status probes and the backtest match listing keep working throughout.

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
| `fetch`, `extract`, `import` | **keep** (import gains the identity columns, P-1) |
| `precompute` | **remove** (P-6) |
| `export` | **remove** (P-6); L1 writes training frames into the run directory |
| `train_batting`, `train_bowling`, `train_fielding`, `train_extras`, `train_innings`, `train_win`, `train_combination_meta` | **replace by one `retrain`** step: rating pass → XI win models → performance models → L4 report → run manifest (P-3 introduces it beside the old steps; P-6 deletes them) |
| `auto_tune` | **remove**; a fixed small grid runs inside `retrain` and records its choice in the manifest |
| (new) `evaluate` | L4 on demand for an arbitrary cutoff, without retraining `current` |
| (new) `reload` | swap `current` to a named run; `/admin/reload` |

Six training steps and two feature steps become three: import → retrain → reload, with
`evaluate` beside them. The step registry's `Requires` graph shrinks accordingly and the
"confirm untuned defaults" prompt (`confirmDefaultParams`) has nothing left to confirm.

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
  and the scorecard gains ranges and marginal values.

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
| Calibration of any probability shown | reliability + Brier; isotonic recalibration on a temporal fold if needed; interval coverage (H-5) | **done for win and performance (P-3)**; simulator in P-4 |
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
