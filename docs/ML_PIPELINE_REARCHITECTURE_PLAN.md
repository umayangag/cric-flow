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
| **win** (`ml.win_features`) | CV accuracy 0.56; held-out AUC 0.56–0.63 (S-3c) | Not usable as a selection objective. **Fixed by S-10:** 0.74 T20 / 0.72 ODI / 0.76 T20I display, 0.73 / 0.69 / 0.75 objective. |
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
                                                                 ►  serving RatingState  (ratings through today)
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

| target | population | loss | outputs |
|---|---|---|---|
| runs | players expected to bat (exp_balls_faced > 0) | quantile (0.1, 0.5, 0.9) or NB | median + interval |
| balls faced | same | quantile | median + interval |
| wickets | players expected to bowl | Poisson | rate → P(0), P(1), P(2+) |
| runs conceded | same | quantile | median + interval |
| catches | fielders | Poisson | rate only (never a headline) |

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
  delta; swap monotonicity (share of upgrades that lower p); selection-comparison winner
  accuracy (greedy vs xi); natural experiment — for consecutive matches of the same side
  with 1–3 lineup changes, does Δobjective agree with Δoutcome more often than chance.
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
| E1 | Do sequence features add to L2-B? | Ablate `seqcalc` families as extra as-of accumulators in L1; measure Spearman / pinball on the holdout | Keep a family only if it moves pinball loss by > 1% over three seeds; otherwise drop it and its tables |
| E2 | Is the simulator consistent with the display model? | Simulated P(win) vs display P(win) on holdout matches; calibration of each | If simulated P(win) is worse-calibrated by > 0.01 Brier, keep it as a display-only distribution and never as a probability |
| E3 | Can batting order be optimised? | Expected-slot model + L2-B; for the chosen XI, evaluate objective / simulated totals under permutations of the top 7 | If reordering moves simulated totals by > 3% for > 30% of XIs, add batting-order suggestion to L3; else leave order to the captain |
| E4 | How much does identity cost? | Re-run S-10 on the Postgres source before and after IDENTITY I-3/I-4 | Report the AUC delta; expect the women's-cricket subset to move most |
| E5 | Natural experiment for selection | Same side, consecutive matches, 1–3 changes: sign agreement between Δobjective and Δresult | If agreement > 55% on ≥ 300 pairs the objective is selecting on real signal; record either way |
| E7 | Do gender-split context baselines help? | Split the (format, over) baseline by gender in the rating pass; measure objective AUC overall and on the women's subset | Keep if the women's subset improves by > 0.01 without hurting men's |
| E6 | Format transfer for L2-B | Train T20 + T20I jointly with a format indicator vs separately | Keep separate unless joint wins by > 0.01 Spearman (for the win model it lost; the performance model may differ) |

Already answered by S-9/S-10 (do not re-run): pooling formats for the win model (no),
neural nets (no gain), subset-by-prediction (no; weight by involvement instead), monotone
objective (logistic wins for the argmax).

---

## 6. Migration sequence

Ordered so selection quality improves first and nothing is deleted before its replacement
is measured. One PR each, branch `arch/<id>-<slug>`, same conventions as the selection
checklist.

| id | PR | acceptance |
|---|---|---|
| P-0 | Land S-10; run its acceptance on the DB; set `selection.win_model: "xi"` for limited-overs formats (S-6) | selection-comparison: `xi` ≥ `greedy` on winner accuracy |
| P-1 | Identity: Cricsheet registry id as `player.external_id`, team + gender as the team key (IDENTITY I-3/I-4) | E4 delta recorded; re-import reproducible |
| P-2 | L1 emits player-match rows + expected batting slot + phase splits; L4 harness skeleton with the performance metrics | frame reproduces `perf_experiment.py` baselines (career-mean Spearman ≈ 0.32 T20) |
| P-3 | L2-B performance model (quantile runs/balls, Poisson wickets) + `/performance/predict` taking XI ids; E1, E6 | beats career mean on Spearman and pinball for every target, 3 seeds; coverage within ±0.03 of nominal |
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
  point that reads as a promise.
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
| H-2 | **Leak canaries.** The model must beat its own best single column by a clear margin, and no column may be a scoreboard read-through | win, performance | Harness reports best-single-column AUC (done); add the TEST-as-control check from S-3c: a column whose AUC collapses in TEST but not elsewhere is suspect | partly |
| H-3 | **Toss / batting-order marginalisation.** The label defines team1 as the side batting first; at selection time that is unknown | win (objective + display), simulator | Serving path averages both orientations; `P(A,B) == 1 − P(B,A)` is a unit test. Measured: display AUC T20 0.741 → **0.747**, ODI 0.720 → **0.730**, T20I 0.715 → **0.731**; the oriented probability moved by 0.07–0.12 on an unknown. Once the toss is known, pass `team1_bats_first` | **done (this PR)** |
| H-4 | **Monotone objective.** Upgrading a player never lowers the selection score | win objective | Harness: share of one-player upgrades with Δp < 0 must stay < 2% (measured 0.4% T20 / 3.7% ODI for logistic; 12% / 20% for unconstrained boosting) | done, gate to add |
| H-5 | **Calibration of what is displayed.** The probability shown must mean what it says | win display, simulator, performance quantiles | Harness: reliability curve + Brier vs base rate per format (done for win); isotonic recalibration fitted on a temporal fold if Brier is worse than base rate; quantile coverage within ±0.03 of nominal (measured 0.80 / 0.79 on 0.80) | partly |
| H-6 | **Rating hyperparameters are not load-bearing.** decay and prior were chosen by judgement | rating pass | Measured sweep (`health_experiment.py`): decay 0.80–0.97 and prior 20–150 move objective AUC by ≤ 0.01 in T20 and ODI — flat. Keep 0.90 / 60; re-run the sweep when the pass changes | done |
| H-7 | **Gender- and competition-aware baselines.** Context expectations (runs per ball per over) are per format only; women's and men's matches share them, and so do the IPL and a club league | rating pass | E7: split the context baseline by gender (cheap, gender is on every match); measure the women's-subset AUC before/after. Competition tiers only if E7 shows gender matters | open (E7) |
| H-8 | **Train / serve parity.** The serving store must compute the same features the training frame holds | all | Harness: for the last 50 holdout matches, rebuild the row from the serving store *as of that date* and assert equality with the training frame (the S-3c defect, made a test) | open |
| H-9 | **Identity.** Ratings keyed by name merge people | all | P-1 (IDENTITY I-3/I-4); E4 measures the delta. Until then the JSON-path numbers are the trusted ones | open |
| H-10 | **Cold start is bounded.** A player with no history must regress to neutral, never explode | win, performance | Measured: replacing a player by a debutant moves p by a median −0.003, p10 −0.05. Unit test on `side_vectors` for an unseen key | done |
| H-11 | **Staleness.** Ratings are only as fresh as the last import | rating pass | `/xi/status` reports `ratings_through`; the ops step fails a prediction request with a clear code if it is older than N days (config, default 14). Retrain is one command and ~2 minutes, so the cadence is "after every import" | open |
| H-12 | **Per-target, never pooled metrics.** A headline number must be for one target on one population | performance | `ml/metrics.py`'s raveled multi-output MAE is retired; L4 reports per target | open (P-3) |
| H-13 | **Consumer metric first.** AUC for an argmax, Spearman/top-k for a ranking, coverage for an interval | all | Every model in L4 has a named consumer and its metric is the one that gates | rule |
| H-14 | **Seeds and noise floor.** Differences under the seed spread are not evidence | all | Every reported number is a mean over ≥ 3 seeds with the spread (done for win) | done |
| H-15 | **Data-quality gate.** Undecided matches, sides without squads, namesakes, replacement players | rating pass | Harness reports counts per retrain and fails on a jump > 2× the previous run | open |
| H-16 | **Run identity.** A measurement must name the artifact it measured | all | `runs/<id>/manifest.json` with dataset sha, cutoff, git sha, hyperparameters, metrics (D-3) | open (P-6) |
| H-17 | **Format scope.** Selection is only offered where the objective ranks | win | TEST stays on greedy with a note in the UI; an objective with holdout AUC < 0.65 is not used for selection in that format | rule |
| H-18 | **Day-close batching.** A match never sees a same-day result | rating pass | Implemented in `ml.xi.builder`; unit-tested; cost ≤ 0.003 AUC | done |
| H-19 | **Walk-forward evaluation + locked window.** Choices are made on rolling cutoffs; one final window is scored once per release | all | L4 reports mean ± spread over cutoffs; the locked window (≥ 2025-09-01) is never used for a choice | open (P-0 reports, P-2 implements) |
| H-20 | **Unconditional training population.** Rows are never selected by the outcome (who batted, who bowled) | performance | Training rows are all XI players with as-of expected involvement; two-part targets allowed only if both parts are unconditional | open (P-3) |
| H-21 | **No in-sample stacking.** A model output consumed downstream is out-of-sample for that row | performance → simulator, any meta-model | as-of features or out-of-fold predictions from a temporal split; the harness asserts the second stage never scores a row the first stage trained on | rule (P-3, P-4) |

Items marked *open* are folded into the migration: H-7, H-8 and H-19 into P-2, H-5, H-12 and
H-20 into P-3, H-11 and H-15 into P-6 alongside H-16. Nothing in the list needs new modelling; it is
measurement, guards and two small serving rules.

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
| `player.external_id` (new), `opposition.gender` (new) | player identity is by name; 130 team names span both genders | **add**: Cricsheet registry id on `player`, gender on the team key. The importer already reads both from `info.registry` / `info.gender` | P-1 |
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
| **Outcome-conditioned training population** | rows exist only for players who batted/bowled — decided by the result — and the model learns the population, not the quantity | **rule H-20**: the performance model's training rows are *all XI players*, with involvement predicted as-of (expected balls faced/bowled), never rows selected by what happened. A two-part target (P(bats) × runs given bats) is allowed only if both parts are trained on the unconditional population. The experiment in §1 conditioned on "batted" and is therefore an upper bound for the given-batted quantity, not the deliverable | open — P-3 |
| **Stacked predictions used in-sample** (classic stacking leak) | the greedy selector, the Monte Carlo and the combination meta-model consumed base-model predictions for matches those models had trained on; a depth-12 RF partly memorises its rows, so second-stage fits learned to over-trust them | **rule H-21**: any model output consumed by another model or by an evaluation must be out-of-sample for that row — as-of ratings by construction, or out-of-fold predictions from a temporal split. The simulator (L2-C) consumes L2-B only through as-of features, never through in-sample fits | rule; P-3/P-4 |
| **Unknowable-at-decision inputs** (toss, innings, batting order) | trained on real values, served constants (S-2, S-8) | marginalised on the serving path (H-3); the display model accepts the toss once known | closed for win; P-3 for performance |
| **Target leakage through team context** | none found: Elo, form, h2h, venue bat-first rate all update *after* the day closes | — | closed |
| **Train / serve skew** | serving aggregated over 11, training over the scorecard | same code path; parity test H-8 turns it into a permanent check | test open — P-2 |
| **Identity leakage** (two people as one, one person as two) | name-keyed ids; men's and women's sides share team ids | P-1; the JSON-path numbers use registry ids and are unaffected | open — P-1 |
| **Holdout reuse** (model-selection leakage) | — new — | the 2025-09-01 holdout was used in this session to choose feature families, model class, monotone constraints and marginalisation. Every reported number is therefore mildly optimistic as an estimate of *future* performance, even though each individual comparison is valid. Closure: **rule H-19** — L4 evaluates by rolling-origin walk-forward over several cutoffs (e.g. quarterly from 2024-01 to 2025-09) and reports mean ± spread; and a **locked final window** (matches after the latest cutoff, 2025-09-01 → present) is scored once per release, never used for choices. The numbers in this document are the *development* numbers; P-0 reports the first locked-window numbers | open — P-0 / P-2 |
| **Leak canaries** | S-3c was found by comparing the model to one raw column and to the TEST format as control | H-2: best-single-column AUC (done) plus the TEST-control check: a feature whose AUC collapses in TEST but not in limited-overs formats is suspect | partly |

### 10.2 ML standards, and where the plan stands

| standard | plan | status |
|---|---|---|
| Temporal validation, no random splits | rolling-origin walk-forward + locked window (H-19) | rule set; harness in P-2 |
| Reported with uncertainty | ≥ 3 seeds, mean ± spread (H-14); walk-forward spread across cutoffs | done / P-2 |
| Baselines that are hard to beat | base-rate Brier, best single column, career mean, team Elo — all reported beside the model | done |
| Metric matches the consumer | AUC + monotonicity for an argmax; Spearman/top-k/coverage for a ranking or interval; per target, never pooled (H-12, H-13) | rule; P-3 retires the pooled metric |
| Calibration of any probability shown | reliability + Brier; isotonic recalibration on a temporal fold if needed; interval coverage (H-5) | partly |
| Distribution-aware losses | quantile / Poisson for counts; MAE-optimal points are not the deliverable | P-3 |
| Reproducibility | dataset sha, cutoff, git sha, hyperparameters and metrics in a run manifest (H-16); deterministic seeds; the pass is a pure function of the event table | P-6 |
| Data-quality gates | undecided matches, sides without squads, namesakes, replacement players counted per run and gated (H-15) | P-6 |
| Serving = training | same feature code, parity test (H-8); ids in, features computed inside | S-10 / P-2 |
| Monitoring in use | staleness of ratings (H-11); prediction-distribution drift per format between runs (add to the manifest diff) | P-6 |
| Hyperparameters not tuned on the test window | grid inside the walk-forward folds only; the locked window never sees a choice | H-19 |
| Sample size honesty | T20I (182) and TEST (157) holdouts cannot resolve differences under ~0.03 AUC; the plan says so wherever they are quoted | done |

### 10.3 What this means for the numbers quoted here

The development numbers (0.73–0.75 AUC objective, 0.74–0.76 display) are leak-free with
respect to features — the mechanism admits no future information — but they are
*selection-optimistic* because one holdout guided several choices. The right expectation
for the first locked-window report in P-0 is a small drop (0.01–0.02 is typical for this
degree of reuse), not a large one; a large one would say something was wrong with the
window rather than the model. The walk-forward spread in P-2 replaces this guess with a
measurement.

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
