# Win-probability team selection: PR checklist

**Goal.** Pick the XI from a player pool that **maximises win probability**, and be able to
show that it beats the XI we pick today.

Implement **one PR at a time**; mark status in the table below as work progresses.

---

## Why this plan exists

The system's stated purpose is optimal team selection, but the running configuration
selects by **greedy top-11 on three hand-picked weights**:

```json
// go-app/config.json -> selection
"score_weights": { "bat": 0.45, "bowl": 0.4, "field": 0.1, "keeper_bonus": 0.02 }
```

Win probability is never consulted. The block has no `use_win_probability_selection`
key and no `use_optimizer` key; both are plain `bool` in `config/types.go:144-145`, so
both are `false`, and `predict_team.go:457-458` reads them straight into the branch that
decides everything.

This plan does **not** introduce win-probability selection. That was already built. It
repairs the four things that would make it select badly, gives us a way to prove it, and
then switches it on.

---

## What already exists

Verified against `main`, not assumed. It is more than it looks, and it changes what this
plan has to build:

| Piece | Location |
|---|---|
| Hill-climb over XIs with win probability as the objective | `teamselect/optimize.go:290` `SelectByWinProbability` |
| Server-side optimiser with vectorised batch inference | `ml-service/ml/team_optimizer.py` (448 lines) |
| Optimiser endpoint | `ml-service/app/main.py:587` `POST /optimize/team-selection` |
| Both paths wired into prediction, with fallback | `predictteam/predict_team.go:462`, `:1002-1015` |
| Win model whose features are a function of the chosen XI | `ml-service/ml/win_features.py` |
| Per-team eval budget and iteration caps | `config/constants.go:137-138` (50 iterations, 500 evals) |

**Nothing in this plan is greenfield.** Every item is a repair, a measurement, or a
config change.

---

## The architectural insight this plan rests on

The win model does **not** consume the batting/bowling/fielding models' predictions. It
consumes *raw windowed player features* aggregated over whichever XI you propose:

```python
# ml/win_features.py:62-70
_GROUP_TO_PLAYER_KEY = [
    ("team1_bat_consistency",  "batting_std_w10", 1),
    ("team1_bowl_consistency", "bowling_std_w10", 1),
    ("team1_bat_form",         "batting_mean_w5", 1),
    ...
]
```

For each of 8 groups (team x bat/bowl x consistency/form) it takes
`{sum, mean, std, max, min, top3_mean, count}`, then adds 13 derived matchup, depth and
spread features. 69 model inputs, every one of them a function of the set of players
selected.

Two consequences that shape the whole plan:

1. **Win probability is directly computable for any candidate XI.** No additive
   player-score proxy is needed. This is the property that makes the goal reachable at
   all, and most systems do not have it.
2. **The base player models are not on the critical path for the objective.** They feed
   the greedy *seed*, the displayed scorecard and the team-score aggregates — but not the
   quantity being maximised. That is why S-5 shrinks and S-8 is deferred.

---

## Status

| ID | Status | PR branch (when done) | Title |
|----|--------|----------------------|-------|
| S-1 | done | `select/s-1-opponent-xi` | Optimise against the opponent's XI, not their whole pool |
| S-2 | done | `select/s-2-drop-toss-feature` | Remove `toss_winner_opposition_id` from the win contract |
| S-3a | done | `select/s-3-selection-backtest` | Win-model discrimination report (AUC, Brier, reliability) |
| S-3b | done | `select/s-3b-selection-backtest` | Selection backtest harness: greedy vs winprob over historical matches |
| S-4 | todo | `select/s-4-search-upgrade` | Steepest-ascent, pair swaps, multi-start, real budget |
| S-5 | todo | `select/s-5-meta-seed-target` | Composite target for the combination-meta seed |
| S-6 | todo | `select/s-6-enable-winprob` | Turn on win-probability selection |
| S-7 | todo | `select/s-7-id-encoding` | Venue and opposition ID encoding (deferred) |
| S-8 | todo | `select/s-8-unknowable-features` | Base-model features unavailable at decision time (deferred) |

**Status legend:** `todo` | `in_progress` | `done` | `skipped` | `blocked`

---

## Conventions for every PR in this list

- **Branch:** `select/<id>-<slug>`, lowercase. Never commit to `main`.
- **Commit:** Conventional Commits with a scope and the ID, e.g.
  `fix(selection): optimise against the opponent XI (S-1)`.
- **One concern per PR.** Split anything past ~600 changed lines.
- **Docs in the same branch.** `docs/ml-and-training.md`, `docs/overview.md` and
  `ARCHITECTURE_MAP.md` (`make gen-architecture-map`) when a contract changes.
- **Baseline verification** (before and after; both must pass):
  ```bash
  make check-all
  ```
- **Coverage ratchets up, never down.** Current gates: `go-app` `COV_MIN=63`,
  `ml-service` `COV_MIN=78`, frontend lines 65 / functions 66 / statements 65 /
  branches 74. When a run comes in above its gate, raise the gate to `floor(actual)` in
  all three places for that component.
- **Update this file in the same PR:** set the row to `done`, fill in the branch.

---

## S-1 — Optimise against the opponent's XI, not their whole pool

**Problem.** Each team is optimised against the opponent's *entire pool*. Both selection
paths do it:

```go
// predict_team.go:1194-1198 (per-call path)
opponentIDs := make([]int64, 0, len(opponentNameToID))
for _, id := range opponentNameToID { opponentIDs = append(opponentIDs, id) }

// predict_team.go:1062 (server-side path)
opp2Feats := collectPlayerFeatures(nameToID2, allFeats)   // every player in pool 2
```

`nameToID2` is built from `pool2` — the full pool, not a selected XI.

**Why it matters.** The candidate side contributes exactly 11 values to each feature
group; the opponent side contributes however many are in their pool (16, 18, 25). The
model consumes the group sizes directly:

- `*_count` is a model input for all 8 groups.
- `bowl_depth_diff` = `team1_bowl_consistency_count - team2_bowl_consistency_count`
  (`win_features.py`, derived features). Every candidate XI is scored as if the
  opposition fielded their entire squad.
- `_sum` scales with pool size; `_std`, `_max` and `_top3_mean` are all drawn from a
  differently-sized population on each side.

The objective is therefore evaluated on a match that cannot occur. This is first because
no amount of better search helps while the thing being searched is wrong.

**Change.** Alternating best response in `selectTeamsByWinProbability`:

1. Greedy-seed both XIs (existing `Select`).
2. Optimise team1 against team2's **current XI**.
3. Optimise team2 against team1's **new XI**.
4. Repeat for `selection.best_response_rounds` (default 3) or until neither XI changes.

Applies to both the server-side and the per-call paths — they must not disagree about
what is being optimised. The ML-side `_precompute_fixed_team_stats` already treats the
opponent as fixed per call, so the change is in what go-app sends, plus the outer loop.

**Tests.**
- `predictteam`: opponent features passed to the optimiser contain exactly `Size` players,
  not the pool — asserted on both paths via the existing mock predictor.
- `predictteam`: the loop terminates on a fixed point and respects the round cap.
- `teamselect`: unchanged; the optimiser is oblivious to who the opponent is.

**Acceptance.** For a pool of 18 v 18, the optimiser's win-prob evaluations use
`count == 11` on both sides. Recorded before/after win probability for one fixture in the
PR body.

**Risk.** Best response can cycle rather than converge. The round cap bounds it, and the
last completed round is returned — cheap and predictable; no equilibrium claim is made.

---

## S-2 — Remove `toss_winner_opposition_id` from the win contract

**Problem.** The feature is real at training time and fabricated at selection time.

```sql
-- exportqueries/win.go:107
COALESCE(m.toss_winner_opposition_id, 0) AS toss_winner_opposition_id,
```

```go
// predict_team.go:851 (per-call) and :1049 (server-side)
TossWinnerOppositionID: 0,
"toss_winner_opposition_id": 0,
```

**Why it matters.** Team selection happens *before* the toss. The value is not merely
missing at inference, it is unknowable in principle. The model allocates capacity to a
variable it will always receive as `0`, and whatever it learned about the real value is
dead weight at best and skew at worst.

**Change.** Drop the column from `MATCH_CONTEXT_BASE_COLS` in `ml/win_features.py`
(and therefore from `WIN_ENHANCED_FEATURE_COLS`), from the go-app win export
(`exportqueries/win.go:107,172,252`), and from the two feature builders. Re-export,
re-train win. Note that `format_id` is already carried but excluded from the model's
feature list — follow that pattern only if a consumer needs the column; otherwise remove
it outright, per the project's no-backward-compatibility rule.

**Tests.**
- `ml-service`: `WIN_ENHANCED_FEATURE_COLS` no longer contains the column; length assertion
  updated.
- `go-app`: export header alignment tests (the C2-2b-pre guard) catch the header change.
- `frontend-backend-sync-check` must pass.

**Acceptance.** Win model re-trained; AUC and Brier from S-3 recorded before and after.
A drop in training AUC is expected and acceptable — it is the removal of a variable the
serving path never had.

**Risk.** None to serving; the value was already constant there. Training metrics will
look worse and be more honest.

---

## S-3 — Selection backtest harness and win-model discrimination report

**Split into S-3a and S-3b while implementing.** The two deliverables below are one
concern only in the sense that both measure. (b) is a Python report over held-out win
rows; (a) is a go-app CLI that drives selection over historical matches. They share no
code, and (b) gates (a) — a selection comparison run against a model that ranks nothing
measures nothing. (b) shipped first as **S-3a**; (a) is **S-3b**.

**Problem.** Every metric in the system is player-level MAE. Nothing measures whether a
*selection* was good, so S-4, S-5 and S-6 have no acceptance criterion.

**Change.** Two deliverables.

**(a) Selection backtest.** For each historical match in a window:

1. Build the pool as of the match date (existing `GetBacktestSquadPlayerIDs` and
   `ComputeFeaturesAtCutoffForMatch` already do this correctly — features are computed at
   the match's own cutoff, so there is no feature leakage to fix here).
2. Run selection under each arm: `greedy` (today), `winprob` (the optimiser).
3. Record: selected XI, predicted win probability, actual result, and the overlap with
   the XI actually fielded.

Report per arm: mean predicted win probability of the chosen XI, and — the metric that
matters — **realised win rate of the team whose XI the optimiser preferred**.

> **Correction, found while building S-3b.** That last metric is not measurable. The
> match was played by the teams that were actually fielded; asking how our XI would have
> fared means replaying it, which needs a simulator whose accuracy is exactly what is in
> doubt. Any number claiming to answer it would be the simulator grading itself.
>
> What S-3b reports instead, and what each is worth:
>
> - **Winner accuracy** — how often the arm's predicted winner was the real one. The only
>   metric here grounded in ground truth, and the one S-6 should be decided on.
> - **Mean predicted win probability** — how far an arm moves its own objective. An
>   internal consistency check: an arm can win this and be worse in reality.
> - **Divergence between arms** — how many players the two arms choose differently. If
>   the optimiser returns the greedy XI, the search is not doing anything, and neither of
>   the other numbers changes that.
> - **Overlap with the fielded XI** — context only. Real selectors are not optimal, so
>   agreeing with them is not evidence of being right.

**(b) Win-model discrimination.** On matches after the training cutoff: **AUC**, **Brier
score** and a reliability curve for the win model.

**Why (b) gates everything.** The optimiser's argmax is only as good as the model's
ranking. If AUC is near 0.5 on held-out matches, hill-climbing on it is choosing noise,
and S-4's search improvements would be measuring how thoroughly we can find the noise's
maximum. Establish this number before spending effort on search.

**Note on calibration.** The win model is uncalibrated (`docs/ml-and-training.md` —
calibration was removed in C1-4/C1-6). This does **not** affect selection: any monotone
recalibration leaves the argmax unchanged. Calibration matters for the probability we
*display*, not for which XI wins the search. Do not let it block this plan.

**Tests.** Harness is a CLI plus a thin service; unit-test the arm assignment, the
metric computation and the cutoff filter with fixtures. No network in tests.

**Acceptance.** One command produces each report for a named format and window. The
numbers go in the PR body and become the baseline every later item is measured against.

**S-3b as shipped.** `POST /api/backtest/selection-comparison`, not a CLI: the driver needs
the prediction path's ML adapter and DB seams, which live in the server package, and every
other backtest capability in this repo is already exposed the same way. The comparison
logic itself is in `internal/services/selectionbacktest`, free of HTTP and the database,
driven through a `Selector` function so it is testable without an ML service or a trained
model. `predictteam.Input` gained a `SelectionMode` override so one process can run both
arms without writing to the global config and hoping nothing else read it in between.

**S-3a as shipped.** `make win-discrimination TRAIN_CUTOFF=<RFC3339>` → `ml.win_discrimination`.
It carves the holdout out of the training-data export by `match_date`, builds features with
the trainer's own `build_win_feature_frame`, and selects the columns the model recorded in
its metadata sidecar rather than re-deriving them — the trainer's variance filter is fitted
on its batch, so re-running it on the holdout would score a different matrix than the model
expects. Every way the question cannot be asked (no artifact, no sidecar, a column the export
no longer carries, a one-sided window, a single-class model) is reported per format as a
named reason rather than raised or, worse, returned as an empty report.

**Risk.** Runtime. Bound the match count and reuse the existing
`export-contributions` concurrency configuration rather than inventing another knob.

---

## S-4 — Steepest-ascent, pair swaps, multi-start, real budget

**Problem.** The search barely moves off its seed.

```go
// optimize.go:214-223 — first improvement wins, then break out to the outer loop
if s > currentScore { ...; improved = true; break }
```

- The seed is `Select(pool, w, c)` — greedy on the **hand-picked weights**, i.e. the
  objective this plan exists to replace.
- **First-improve**, not steepest-ascent: the first swap that helps is taken, and the
  scan restarts at slot 0, so early slots are re-examined repeatedly while later slots
  may never be reached within budget.
- **Single swaps only.** Pairs that only help together are unreachable.
- Budget: 50 iterations / 500 evals per team. On the per-call path each eval is a
  separate HTTP round-trip.

The result is the greedy XI with a few swaps — the win model arbitrates at the margin
rather than choosing the team.

**Change.** In `teamselect/optimize.go` and the mirroring `ml/team_optimizer.py`:

- Steepest-ascent: evaluate all valid single swaps for a slot, take the best.
- Add pair swaps (two out, two in) as a second neighbourhood, entered when single swaps
  reach a local optimum.
- Multi-start: greedy seed plus N random constraint-satisfying restarts; keep the best.
- Raise the eval budget substantially on the server-side path, where
  `_batch_evaluate_candidates` already scores a whole neighbourhood in one
  `predict_proba`. Keep the per-call path's budget low; it is the fallback.

The two implementations must stay behaviourally equivalent — that is already asserted by
the docstring in `team_optimizer.py` and should become a test.

**Tests.**
- `teamselect`: steepest-ascent picks the best swap, not the first (constructed score
  function where they differ).
- `teamselect`: pair swap escapes a local optimum that single swaps cannot.
- `teamselect`: multi-start is deterministic under a fixed seed.
- Parity test: Go and Python optimisers return the same XI for the same fixture.

**Acceptance.** On S-3's harness, mean win probability of the selected XI improves versus
S-4's own pre-change baseline, at equal or better wall-clock time.

**Risk.** Budget growth costs latency. Measure it in the PR body; the caps stay in config.

---

## S-5 — Composite target for the combination-meta seed

**Problem.** Every row's target is the player's actual runs:

```go
// services/backtest/contributions.go:41-44
target := 0.0
if batDiv > 0 { target = actualRuns / batDiv }
```

The features are `bat_score`, `bowl_score`, `field_score`, `is_keeper`. A Ridge asked to
explain *runs* from *predicted bowling quality* will drive the bowl coefficient to zero
or below — bowlers bat last. Those coefficients then load as selection weights via
`config/loader.go:137-138` `Selection.MetaModelPath`.

**Scope note — this item is deliberately small.** Once win probability is the objective,
the meta weights only produce the *seed* and a display quantity; they no longer decide
the team. So fix the target and stop. Do **not** build the out-of-fold machinery, the
post-cutoff holdout, the freshness gating or the multi-fixture export redesign that a
decision-making model would need. A seed heuristic does not earn that rigour.

**Change.** Build the target with the same normalisers the features use, applied to
actuals instead of predictions:

```
target = NormalizeBatScore(actual_runs, batDiv)
       + NormalizeBowlScore(actual_wickets, actual_economy, wicketDiv, econBase)
       + NormalizeFieldScore(actual_catches, actual_run_outs, fieldDiv)
```

Two guards this exposes:

- `NormalizeBowlScore(0, 0, 5, 12)` returns **0.5** — `econPart = max(0, 1 - 0/12) = 1`.
  A player who never bowled scores half marks for bowling. Include the bowling component
  only when the player actually bowled; this is wrong on the feature side too and should
  be fixed in `teamselect.NormalizeBowlScore`'s callers, not by special-casing the target.
- Actual `catches` and `run_outs` are not populated on the delegated backtest path
  (`backtest_handlers.go:341-356` carries `runs`, `wickets`, `economy` only). They exist
  in `services/backtest/metrics.go:89-92`; plumb them through or omit the field term
  explicitly rather than silently sending zeros.

**Open decision — see below.** Whether the three components are summed with equal weight
is a cricket judgement, not a code one.

**Tests.** `services/backtest`: a bowler's row has a materially non-zero target; a
non-bowler contributes no bowling component; existing contributions tests updated.

**Acceptance.** Refitting the meta model yields a positive bowling coefficient on a
fixture where bowlers demonstrably contributed.

**Risk.** Low. The model is inert today — `meta_model_path` is set in no config file — so
there is no behaviour to preserve.

---

## S-6 — Turn on win-probability selection

**Problem.** The feature is off. That is the whole item.

**Change.** Add to `go-app/config.json` under `selection`:

```json
"use_win_probability_selection": true,
"best_response_rounds": 3
```

**This is last on purpose.** Switching it on before S-1 through S-4 would select against
a phantom opposition, using a feature that is always zero, with a search that barely
leaves its seed, and with no way to tell that any of that was happening. The likely
conclusion would be "win-probability selection does not work", and it would be wrong.

**Tests.** `predictteam`: the flag routes to the win-prob path and falls back cleanly when
the predictor does not implement `EnhancedWinPredictor`. Both already have coverage;
extend rather than duplicate.

**Acceptance.** S-3's harness run on both arms, numbers in the PR body. Ship only if
`winprob` beats `greedy` on realised win rate. **If it does not, the honest outcome is to
leave the flag off and record why** — the harness exists precisely so that is a finding
rather than an opinion.

**Risk.** Latency on the prediction path. The optimiser's budget caps bound it; record
p50/p95 for a real fixture.

---

## S-7 — Venue and opposition ID encoding (deferred)

`venue_id`, `team1_opposition_id` and `team2_opposition_id` are fed to tree models as
**raw integers**. Splits then fall on an arbitrary ordering of surrogate keys, and an ID
unseen in training lands wherever its integer happens to sit.

Options: target or frequency encoding fitted on training folds only; or drop venue in
favour of venue-level aggregates already computed elsewhere. Deferred because it changes
the feature contract and is best measured on S-3's harness once that exists.

---

## S-8 — Base-model features unavailable at decision time (deferred)

The same defect as S-2, one layer down. `toss`, `inning` and `batting_session` are real
columns in the batting, bowling and fielding exports
(`exportqueries/batting.go:54`, `bowling.go:57`, `features/contract.go:39-56`) and are
constants at prediction time:

```json
// ml-service/config.json -> ml.feature_defaults.common
"inning": 1, "session": 1, "toss": 0
```

`inning` is the costly one: batting first and chasing are different tasks, and every
prediction is made as if the player bats first.

**Deferred, with the reason stated:** these features feed the *base* models, which per the
architectural insight above do not enter the win-probability objective. They affect the
seed, the displayed scorecard and team-score aggregates — not the quantity being
maximised. Fix after S-6, when the harness can price the change.

Options when it is picked up: drop the columns; or marginalise at prediction (predict
under `inning=1` and `inning=2` and average), which is the statistically correct answer
and costs one extra batch call.

---

## Open decisions

| # | Decision | Needed by | Default if unanswered |
|---|---|---|---|
| 1 | Are the three components of S-5's composite target summed with equal weight? Equal weighting says a 4-wicket spell at economy 6 is worth roughly 53 runs | S-5 | Equal weight, recorded in the PR body as an assumption |
| 2 | Best-response rounds: fixed count or iterate to a fixed point with a cap? | S-1 | 3 rounds, cap, return last completed round |
| 3 | If S-3 shows win-model AUC near 0.5 on held-out matches, do we stop and improve the win model before S-4? | S-3 | Yes — stop. Search quality is meaningless on a non-discriminative objective |

---

## Out of scope

- Probability calibration of the win model. Monotone recalibration cannot change an
  argmax, so it is a reporting concern. See `docs/ml-and-training.md`.
- Batting-order and role optimisation within the chosen XI. Selection first.
- Any change to precompute, feature windows or the raw stats contract. Those are the
  slow loop described in `docs/ml-and-training.md` and would invalidate this plan's
  baselines mid-flight.
