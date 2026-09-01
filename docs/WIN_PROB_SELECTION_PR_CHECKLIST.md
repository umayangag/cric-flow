# Win-probability team selection: PR checklist

**Goal.** Pick the XI from a player pool that **maximises win probability**, and be able to
show that it beats the XI we pick today.

Implement **one PR at a time**; mark status in the table below as work progresses.

> **Complete.** S-6 shipped in P-5 of
> [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md), on a gate that
> replaced the one this document proposed. Every other item is done, cancelled or superseded —
> see **Final disposition** under the status table. The code this document quotes as "today" was
> deleted in the same PR; read it as history, not as a description of `main`.

---

## Why this plan exists

The system's stated purpose is optimal team selection, but the running configuration
selects by **greedy top-11 on three hand-picked weights**:

```json
// go-app/config.json -> selection, as it was
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

   > **Qualified by S-3c.** The property holds, but the *training* side does not honour
   > it: the export aggregates each group over the players who appear in the scorecard,
   > not over the eleven who were picked. So the function the model learned is not the
   > function the optimiser evaluates, and the difference is the match result.
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
| S-3b | done, then removed | `select/s-3b-selection-backtest` | Selection backtest harness: greedy vs winprob over historical matches. **Deleted in P-5** with the greedy arm it compared against — see the disposition below |
| S-3c | done | `select/s-3c-export-over-squad` | **The win export aggregates over who batted, not over the XI** (all three parts; numbers below) |
| S-9 | done | `select/s-9-xi-win-model` | **The win model barely discriminates once the leak is gone.** Answered: XI-responsive ratings reach 0.73 T20 / 0.69 ODI / 0.75 T20I (numbers below) |
| S-10 | done | `select/s-10-xi-win-model` | **XI-responsive win model**, then behind `selection.win_model: "xi"` — DB acceptance run; the model reproduces its JSON-path numbers (0.72–0.75 objective, 0.72–0.75 display), the S-3b selection gate does not pass (numbers in "S-10 results"). **P-5 removed the flag**: the XI path is the only selection path |
| S-4 | skipped | — | Steepest-ascent, pair swaps, real budget — superseded by S-10's optimiser (`ml/xi/optimizer.py`) |
| S-5 | cancelled | — | Composite target for the combination-meta seed — the meta-model was deleted in P-5; the greedy seed no longer decides anything, and is now `_greedy_seed` inside `ml/xi/optimizer.py` |
| S-5b | cancelled | — | Auto-tune the combination meta-model — same reason as S-5 |
| S-6 | **shipped** | `arch/p-5-repoint-surfaces` | Turn on win-probability selection. **Shipped in P-5 on a replacement gate.** The original gate was run and not met — pooled winner accuracy 0.560 (`winprob`) vs 0.569 (`greedy`) over 332 locked-window matches — and was then shown to be unwinnable by an arm that optimises *both* sides: it asks whether the predicted winner of a **counterfactual** fixture matches the result of the real one, and strengthening both sides moves that fixture toward parity (mean \|p − 0.5\| 0.153 optimised vs 0.180 greedy). P-2 measured the replacements the plan named — specific-XI-beyond-typical-XI **+0.045 ± 0.021** AUC in T20, swap violations **0.3 %** against a 2 % line — and P-5 ships selection on the XI objective for T20 / T20I / ODI. There is no flag: the XI path is the only selection path, and TEST is served a rating-ordered XI marked not optimised (H-17) |
| S-7 | superseded, closed | — | Venue and opposition ID encoding — the XI model never feeds raw ids to a tree (venue and teams enter only through as-of context keyed by id); the identity half is IDENTITY I-3/I-4 = P-1 |
| S-8 | superseded, closed | — | Base-model features unavailable at decision time — the toss/innings case is solved for the win model by marginalising over batting order (H-3, done); the performance model in P-3 marginalises innings the same way; the base models themselves are replaced |

**Status legend:** `todo` | `in_progress` | `done` | `skipped` | `blocked` | `cancelled` | `superseded` | `shipped`

**This checklist is complete.** Every item is done, shipped, cancelled or superseded; nothing
here is a to-do. It is kept for the defect record — the leak in S-3c, the mis-specified gate in
S-6 — which is the part that was expensive to learn.

**Final disposition** ([ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md)):

- **S-1, S-2, S-3a, S-3c, S-9, S-10 — done.** They are the foundation the plan builds on.
- **S-3b — done, then removed.** The harness answered its question, including the one nobody
  wanted: its own headline metric could not decide S-6. P-5 deleted it along with the greedy arm
  it compared against, since there is no second selection path left to compare. Its replacements
  live in the L4 harness and are rendered by the Evaluation report tab.
- **S-4 — skipped.** S-10's optimiser (`ml/xi/optimizer.py`) does steepest-ascent single swaps,
  pair swaps and a real evaluation budget already.
- **S-5, S-5b — cancelled.** The meta-model they tuned was deleted in P-5.
- **S-6 — shipped in P-5**, on the replacement gate; see its row.
- **S-7 — superseded and closed.** The XI model feeds no raw id to a tree; the identity half
  shipped as P-1.
- **S-8 — superseded and closed.** Batting order is marginalised at prediction for both the win
  model (H-3) and the performance model (P-3), and the base models it was about are deleted.

Open decision #1 is moot with S-5; #2 (best-response rounds) still applies — the XI optimiser is
called inside the same alternating loop, capped by `selection.best_response_rounds`. Known
defects D-1 and D-2 close by dropping the tables and exports (P-6); D-3 by run identity (H-16,
P-6); D-5 closed when the artifact families it named were deleted in P-5.

**S-9 is answered and S-10 carries the fix.** See "S-9 results" and "S-10" below; the
paragraphs that follow are the history that led there.

**S-4 and S-6 were blocked on S-9, not on S-3c.** S-3c is done and the leak is gone.
What it revealed is that the objective underneath was mostly leak: held-out AUC across the
four formats is **0.56–0.63**, and Brier is *worse than predicting the base rate* in three
of them. Open decision #3 said to stop in exactly this case, and that is the answer.

**S-4 and S-6 were blocked on S-3c.** The objective they improve and switch on is trained
on a feature set that encodes the result of the match being predicted. Tuning a search
over that objective, or shipping it, would both be measuring the leak. S-5 is unaffected —
it concerns the combination-meta seed, not the win model.

**S-7 was blocked on [IDENTITY_PR_CHECKLIST.md](IDENTITY_PR_CHECKLIST.md); it no longer
is.** The IDs it would encode were themselves split (one franchise under two ids after a
rename) and merged (130 team names shared by a men's and a women's side), and encoding
those first only makes the error smoother. Both are closed: I-3 split the genders in P-1,
I-4 joined the nine renamed clubs. An `opposition_id` is now one club of one gender.

S-7 stays **superseded** for a different reason — the XI model feeds no raw id to a tree —
but the identity it would have encoded is now sound, so if a later model does want an
opposition encoding, nothing in this list blocks it.

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

> **What running it actually showed.** A held-out AUC of 0.95 does not mean the objective
> is sound either. The first real run cleared the gate comfortably and the number was the
> match result leaking through the export — **S-3c**. A single AUC is a necessary check,
> not a sufficient one: it is computed on the same contaminated columns the model trains
> on, so no holdout, however clean its dates, can reveal a leak that lives in the feature
> definitions. What exposed it was comparing the model against one raw column, and against
> the format where that column carries nothing.

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

## S-3c — The win export aggregates over who batted, not over the XI

**Found while running the S-3a report that was supposed to gate S-4.** The report passed
its gate. The gate was measuring the wrong thing.

### What the report said

Win model re-trained to a real cutoff (`make train-win CUTOFF=2025-09-01T00:00:00Z`,
19,969 rows kept, 2,456 held out), then `make win-discrimination
TRAIN_CUTOFF=2025-09-01T00:00:00Z`:

| format | matches | pos rate | AUC | Brier |
|---|---|---|---|---|
| ODI | 388 | 0.423 | **0.953** | 0.082 |
| T20 | 1619 | 0.468 | **0.930** | 0.107 |
| T20I | 188 | 0.473 | **0.955** | 0.076 |
| TEST | 261 | 0.314 | **0.674** | 0.219 |

Open decision #3 asked whether an AUC near 0.5 should stop the plan. These are near 0.95.
On the plan as written, S-4 was cleared to start.

### What the number actually is

**A single raw column from the export scores nearly the same, out of sample, on its own:**

| format | AUC of `team2_bat_form_count` alone | AUC of the whole 69-feature model |
|---|---|---|
| ODI | 0.939 | 0.953 |
| T20 | 0.893 | 0.930 |
| T20I | 0.925 | 0.955 |
| TEST | **0.539** | 0.674 |

That column is the number of team-2 players who came to the crease in innings 2, and it
is set by the result:

```
T20, team1 win rate by team2_bat_form_count
  count=2   n= 323   0.025      count=8   n=1148   0.513
  count=3   n= 673   0.037      count=9   n=1205   0.722
  count=4   n=1013   0.047      count=10  n=1420   0.861
  count=5   n=1162   0.071      count=11  n=2020   0.966
```

Team 2 chases with wickets in hand, four batters bat, team 1 loses. Team 2 is bowled out,
eleven bat, team 1 wins. The export is handing the model the scoreboard.

**TEST is the control that proves it.** Both sides bat their innings out regardless of who
wins, so the same column is flat there (0.539 — noise) — and TEST is exactly where the
model falls to 0.674. The gap between 0.95 and 0.67 is not a gap in difficulty between
formats. It is the size of the leak.

### Reproducing the two tables above

```bash
make train-win CUTOFF=2025-09-01T00:00:00Z
GO_APP_API_KEY=dev-local-key make win-discrimination TRAIN_CUTOFF=2025-09-01T00:00:00Z
```

```python
# the single-column baseline, against the same holdout the report uses
import pandas as pd
from sklearn.metrics import roc_auc_score

df = pd.read_csv("output/go-app/win_encoded_all.csv", low_memory=False)
df["md"] = pd.to_datetime(df["match_date"], errors="coerce")
hold = df[df["md"] >= "2025-09-01"]
for fmt, g in hold.groupby("format_code"):
    print(fmt, len(g), round(roc_auc_score(g["team1_wins"], g["team2_bat_form_count"]), 3))
```

**Worth folding into `ml.win_discrimination` as a follow-up:** report the best single-column
AUC alongside the model's. A model that cannot beat its own best raw column by a clear
margin is not being measured, and the check costs one pass over the holdout.

### Where it comes from

```sql
-- exportqueries/win.go:116-119
t1_bat  AS (SELECT ... FROM batting_data bd ... WHERE bd.inning_number = 1),
t1_bowl AS (SELECT ... FROM bowling_data bw ... WHERE bw.inning_number = 2),
t2_bat  AS (SELECT ... FROM batting_data bd ... WHERE bd.inning_number = 2),
t2_bowl AS (SELECT ... FROM bowling_data bw ... WHERE bw.inning_number = 1),
```

Every one of the 8 feature groups is aggregated over **the players who appear in the
scorecard**, not over the eleven who were picked. Who appears in `batting_data` is decided
by how many wickets fell; who appears in `bowling_data` is decided by how long the innings
lasted.

**It is not confined to the `_count` columns.** Every statistic of the bat groups inherits
the same conditioning — in a comfortable chase only the top order bats, and top-order
players have better windowed form, so the surviving team's *mean* form is higher:

| holdout AUC, alone | T20 | ODI | TEST (control) |
|---|---|---|---|
| `team2_bat_form_mean` | 0.259 | 0.233 | 0.499 |
| `team2_bat_consistency_mean` | 0.243 | 0.227 | 0.501 |
| `team2_bat_form_sum` | 0.616 | 0.664 | 0.530 |

0.259 is 0.741 read the other way up. Dropping the `_count` columns would not fix this.

### The other half: the serving path sends something else entirely

`aggregate_team_features_from_player_maps` (`win_features.py:232-238`) aggregates **every
group over all eleven players of the proposed XI**, bowling groups included. So:

| | training | serving |
|---|---|---|
| `team1_bat_*` population | players who batted (T20 mean 8.1) | 11 |
| `team1_bowl_*` population | players who bowled (T20 mean 5.7) | 11 |
| `team2_bat_*_count` | 0–11, and it *is* the result | always 11 |

Feeding the model what the serving path actually sends — the same holdout rows with every
`_count` set to 11 — moves it to the corner of its training distribution that means "team
bowled out":

| format | mean p(team1 wins) as exported | with counts = 11 | sd as exported | sd with counts = 11 |
|---|---|---|---|---|
| ODI | 0.435 | **0.880** | 0.446 | **0.157** |
| T20 | 0.476 | 0.685 | 0.405 | 0.290 |
| TEST | 0.232 | 0.186 | 0.222 | 0.198 |

The spread across real, differing matches collapses by two-thirds in ODI. TEST — no leak —
barely moves. This is what the optimiser has been hill-climbing on: a model pinned near one
end of its output range, ranking XIs by the residue.

### Change

1. **Store the squad each side picked.** ✅ `select/s-3c-import-playing-xi` — migration
   `0003_match_player.sql`, `Info.Players`, `SquadFromInfo`, `ReplaceMatchPlayersTx`.
2. **Aggregate over that squad on both sides**, for bat *and* bowl groups, so the export's
   population is the same set the serving path aggregates over. ✅
   `select/s-3c-export-over-squad`.
3. **Drop the `_count` columns** — once the population is the squad they are near-constant,
   carry almost nothing, and invite exactly this class of bug back. ✅ `bowl_depth_diff`
   went with them, being the difference of two counts. Win features 77 → 68.
4. Re-import, re-export, re-train, re-run S-3a.

Split across PRs: (1) importer + migration, (2) export query + feature contract, (3) the
re-run and its numbers. Item (1) needs a full re-import of ~22.7k match files.

> **Two corrections from building (1), both worth knowing before (2).**
>
> **`info.players` is not always eleven, and the table is `match_player`, not `match_xi`.**
> Across the 22,734 files in the current dataset: 44,105 sides of 11, **1,337 of 12**, 20
> of 13, one of 14 and five of 10 — concussion and injury replacements, which Cricsheet
> lists in full. So step 3's columns become *near*-constant rather than constant. Dropping
> them is still right (they would encode "did someone get concussed"), but a test asserting
> a flat 11 would be wrong.
>
> **Coverage is total, so no match has to be excluded.** Every one of the 22,734 files
> carries `info.players`, and its team names always match `info.teams`. The importer still
> handles absence — warn, record no squad, keep the ball-by-ball data — because a truncated
> file must not become a side of nobody, but the export's exclusion path should be rare
> enough that a non-zero count is a signal something is wrong.
>
> **The export was never reproducible, found while proving 2/3 correct.**
> `feature_raw_stats_snapshots` holds **443,308 duplicate `(player_id, format_id,
> as_of_date)` groups** for `scope='overall'` — 1.81M of its 2.32M rows. Its unique
> constraint includes `scope_id`, which is NULL for that scope, and Postgres treats NULLs
> as distinct, so the constraint never fires. **2,361 of those groups carry conflicting
> values**, which makes "the latest snapshot before the match" ambiguous: two equivalent
> formulations of the same query disagreed on 411 matches. The export query now breaks
> the tie explicitly (`ORDER BY as_of_date DESC, id DESC`) so it is at least reproducible.
> **The duplicates themselves are a precompute defect and are not fixed here** — they
> predate this plan and affect every model that reads a snapshot, not just win.
>
> **Namesakes, found by running the re-import.** Two files name the same player on both
> sides — `KV Sharma` (Vidarbha / Railways) and `J Butler` (Isle of Man / Guernsey). They
> are two people who share a scorecard name, and Cricsheet cannot separate them either:
> `info.registry.people` is keyed by name, so file 1130677 has 22 squad entries and 21
> registry identifiers. Since this repo also identifies players by name, they are already
> one `player_id`, which `match_player`'s primary key cannot hold twice for one match.
> The importer drops such a name from **both** squads and warns, leaving two matches with
> a ten-player side. Guessing a side would invent data; failing the match — which the first
> cut of the validation did — cost its ball-by-ball record over an ambiguity in the source.
> Fixed in `fix/squad-namesake-both-teams`. The underlying defect — player identity keyed
> by name — is [IDENTITY_PR_CHECKLIST.md](IDENTITY_PR_CHECKLIST.md); this behaviour stays
> even after it, because the registry genuinely cannot separate two namesakes inside one
> match.

**Tests.** Importer ✅: squad parsed and persisted for both sides including players who
never bat or bowl; a file with no `info.players` still imports and records no squad; a
player named by both teams is dropped from both and costs one row rather than the match;
the same name twice in one team is deduplicated, since the side is not in doubt; a
re-import replaces rather than accumulates. Export: a fixture where a team's squad and its scorecard
differ produces equal counts on both sides, and the bowl group includes players who bowled
no overs.

**Acceptance.** Re-run S-3a on the re-exported data. **The honest expectation is that AUC
falls sharply — toward the TEST figure.** A number that stays near 0.95 after this change
means the leak was not removed, not that the model is good. Only after that does open
decision #3 have a real answer, and only then are S-4 and S-6 worth doing.

**Risk.** The full slow loop (re-import → re-precompute is not required, but re-export and
re-train are), and every win-model baseline recorded before this item becomes
incomparable. That is the correct outcome: those baselines were measuring the scoreboard.

---

## S-3c results — what the fix revealed

Re-import → re-export → `make train-win CUTOFF=2025-09-01T00:00:00Z` →
`make win-discrimination TRAIN_CUTOFF=2025-09-01T00:00:00Z`, on the same holdout as
before (19,969 train / 2,456 held out).

### Held-out discrimination, before and after

| format | matches | pos rate | AUC **before** | AUC **after** | Brier after | base-rate Brier |
|---|---|---|---|---|---|---|
| ODI | 388 | 0.423 | 0.953 | **0.561** | 0.271 | 0.244 ❌ |
| T20 | 1619 | 0.469 | 0.930 | **0.632** | 0.240 | 0.249 ✅ |
| T20I | 188 | 0.473 | 0.955 | **0.591** | 0.276 | 0.249 ❌ |
| TEST | 261 | 0.314 | 0.674 | **0.572** | 0.235 | 0.215 ❌ |

Walk-forward CV accuracy moved the same way: ODI 0.874 → 0.580, T20 0.841 → 0.565,
T20I 0.841 → 0.588, TEST 0.677 → 0.648. **TEST barely moves, and TEST is the format that
never had the count leak.** That is the control behaving exactly as predicted, and it is
the strongest evidence that the drop is the leak leaving rather than the fix breaking
something.

### How to read these numbers

- **Only T20 is clearly better than chance.** z ≈ 9.5 against 0.5 on 1,619 matches. ODI
  (z ≈ 2.1), T20I (z ≈ 2.2) and TEST (z ≈ 1.9) are barely distinguishable from a coin.
- **Three of four formats have a Brier worse than predicting the base rate.** Calibration
  does not affect an argmax, so this does not by itself condemn selection — but a model
  that cannot beat a constant is not describing much.
- The earlier 0.93–0.96 was the scoreboard. Those numbers are void and should never be
  quoted again as a baseline.

### The decision this forces

**Open decision #3 asked exactly this question and answered it in advance: stop.** An
argmax over candidate XIs is only as good as the model's ranking, and the ranking is now
known to be weak. S-4 would be tuning a search over an objective that barely orders whole
matches, let alone two XIs differing by one player. S-6 would be shipping it.

So **S-4 and S-6 move from "blocked on S-3c" to "blocked on S-9"**, and S-9 is new work:
make the win model discriminate, or establish that it cannot with these features.

**This is the plan working, not failing.** S-3a and S-3b exist precisely so that this is a
finding rather than an opinion, and the honest outcome S-6 named — "if it does not, the
honest outcome is to leave the flag off and record why" — has arrived earlier than
expected and for a better reason.

---

## S-9 — Make the win model discriminate

**Problem.** With honest features the win model scores 0.56–0.63 held-out AUC. Team
selection maximises this model, so the whole plan rests on it.

**Why this is not surprising, stated plainly.** The model gets 68 numbers, all of them
aggregates of *windowed player form* over two squads, plus venue and opposition ids. It
has no innings state, no toss, no venue-conditioned scoring history, no batting order, no
head-to-head, no home advantage. Predicting a cricket result from "how well have these
22 players been batting and bowling lately" is a genuinely hard problem, and 0.63 in T20
may be close to what this feature set can support.

**Directions, cheapest first.**

1. **Home advantage and venue history.** Neither is in the contract today. `venue_id` is
   a raw integer (S-7), and whether a side is at home is not represented at all.
2. **Head-to-head and recent team form.** Team-level, not player-level: the current
   features cannot express "this side has won nine of its last ten".
3. **Toss and innings state.** Unknowable before selection (S-2, S-8) — but a *marginalised*
   prediction over both toss outcomes is available and is the statistically correct
   treatment.
4. **Player identity.** [IDENTITY_PR_CHECKLIST.md](IDENTITY_PR_CHECKLIST.md): 163 names
   hold 348 people, so ~1% of the roster has blended form. Small, but it is noise in
   exactly the inputs this model consumes.
5. **Accept the ceiling.** If the feature set tops out near 0.63, say so and decide whether
   an argmax over it is worth shipping at all. That is a legitimate outcome.

### Two directions already tested and closed

**Pooling the formats into one model: no.** Tested on the same holdout.

*Cross-format transfer is nil* — train on T20, evaluate elsewhere:

| holdout | T20 model | own-format model |
|---|---|---|
| ODI | 0.530 | 0.553 |
| T20I | 0.532 | 0.591 |
| TEST | **0.484** | 0.572 |

TEST lands below chance. The formats do not share exploitable structure in these features.

*A unified model (19,969 rows, format one-hots) does not beat per-format models once noise
is accounted for:* ODI +0.027, T20 +0.012, T20I +0.033, TEST **−0.018**. Every positive
delta is inside one standard error of its holdout, and the T20 gain is illusory — see
below. TEST's loss is the only clean signal, and it is negative.

**"T20 only looks better because it has more matches": partly, and it does not rescue the
others.** T20 trained on subsamples, evaluated on the same T20 holdout:

| training rows | AUC (3 seeds) |
|---|---|
| 1,918 (= T20I's size) | 0.610 ± 0.010 |
| 4,808 (= ODI's size) | 0.628 ± 0.005 |
| 10,403 (full) | 0.648 ± 0.009 |

Sample size is worth ~0.04 AUC across a 5.4× range. At equal data T20 (0.610) and T20I
(0.591) are indistinguishable, so that gap is a data-volume artefact — but T20 on 1,918
rows still beats ODI on 4,808 (0.553), so **ODI is genuinely harder, not merely
data-starved.** More data is not the lever.

**Selecting a subset of players instead of averaging over the XI: not as posed, but the
observation behind it is right.** The dilution is structural, not noise —
`bowling_mean_w5` averaged over eleven players includes the five who never bowl and sit
near zero, so a bowling statistic is being averaged over non-bowlers.

The cheapest form of the idea is already in the export and is a wash. `_top3_mean` is a
"only the players who matter" aggregate; against `_mean` on the T20 holdout it is inside
the noise band, and the two strongest single columns in the whole feature set are
full-squad means:

| group (T20 holdout, n=1619) | `_mean` | `_top3_mean` |
|---|---|---|
| team2_bat_consistency | **0.603** | 0.596 |
| team2_bat_form | **0.588** | 0.580 |
| team1_bowl_consistency | 0.538 | **0.560** |

Three reasons not to select a subset by *predicted* performance:

1. **It puts the base models on the critical path**, which this plan deliberately keeps
   them off — see "The architectural insight this plan rests on". Win-model quality would
   become bounded by batting/bowling-model quality, which is measured by player-level MAE
   and never by anything resembling ranking skill.
2. **A hard subset makes the objective discontinuous.** Swapping one player changes which
   players fall inside the subset, so the score jumps rather than moves, and S-4 is a
   hill-climb.
3. **It discards batting depth**, which is real signal — `_min` and `_std` over eleven
   currently capture "the #8 can bat" by accident.

**If it is built anyway, the training side must use the same predicted-subset rule.** The
natural implementation mistake is to train on the batsmen who actually batted, which is
the S-3c leak wearing a different hat: it would look like a large gain and mean nothing.

**The version worth trying instead: weight, do not truncate.** Keep all eleven and weight
each player by expected involvement — batting groups by expected balls faced, bowling
groups by expected overs, both from historical batting position and bowling workload,
which are known at selection time. That captures the insight without putting the base
models on the critical path, stays continuous for S-4, and keeps depth because nobody is
dropped. It is eight aggregation expressions in `exportqueries/win.go`, then the same
re-export/retrain/re-measure loop. Expect it to redistribute signal rather than add any:
the best single column in this feature set is ~0.60.

### The features the model does not have beat every feature it does

Measured as-of (each match rated on earlier matches only) on the S-3c holdout, and
verified twice — independently by two implementations agreeing to ~0.01:

| feature | ODI | T20 | T20I | TEST |
|---|---|---|---|---|
| `elo_diff` alone | **0.645** | **0.652** | **0.726** | 0.535 |
| `h2h_rate` alone | 0.639 | 0.670 | 0.714 | 0.509 |
| **entire 63-feature player model** | 0.561 | 0.632 | 0.591 | **0.572** |

**A single Elo number beats all 63 engineered player-aggregate features in every
limited-overs format.** The win model is being asked to infer team strength from windowed
batting and bowling averages when the match record states it directly. A 9-feature
match-level model (Elo, form, h2h, venue batting-first bias, venue familiarity) reaches
ODI 0.642, T20 0.677, T20I 0.707.

TEST is the exception in both directions — Elo 0.535, h2h 0.509, player model 0.572. Test
sides are few and stable, so head-to-head carries little and player quality matters more.

All of these come from the `match` table alone: no new precompute, no schema change.

### …but they cannot select a team

**Elo, form and head-to-head are constant with respect to the XI.** They would raise
outcome accuracy substantially and contribute *nothing* to choosing eleven players. A model
reaching 0.72 on Elo would select no better than one at 0.56.

**So this plan's goal is really two goals, and they need different work:**

| goal | lever | status |
|---|---|---|
| Outcome accuracy — the probability we *display* | match-level features (Elo, form, h2h, venue, home) | large, cheap, measured above |
| **Selection quality — which XI to pick** | XI-responsive features only | the actual blocker for S-4 and S-6 |

Only the second unblocks S-4 and S-6. The candidates are weighting players by expected
involvement (above) and player-level impact ratings — the individual analogue of Elo, which
would respond to XI changes in the way team Elo cannot.

**Naive concatenation is not automatically a win:** a quick combined fit gave T20 0.653
against 0.650 player-only and 0.662 team-only. Preliminary — unweighted, no variance
filter — but enough to say the combination needs real work rather than assumption.

### Responsiveness is not the problem — ranking is

AUC measures ranking across *whole matches*, where the two sides are entirely different
teams. Selection needs something finer: ordering XIs that differ by one player. Measured
directly on the S-3c models (upgrade team1's weakest batsman to match its best, moving
`_sum`, `_mean` and `_min`):

| | T20 (n=1619) | ODI (n=388) |
|---|---|---|
| predicted p | 0.029–0.983, sd 0.200 | 0.022–0.946, sd 0.223 |
| Δp from the swap | median +0.009, p90 +0.100, max +0.300 | median +0.057, p90 +0.163, max +0.333 |
| matches moving >0.05 | 39% | 60% |

**The model responds strongly to a one-player change.** That rules out one hypothesis —
the objective is not so flat that the optimiser is choosing between indistinguishable
candidates.

**This makes the case for blocking S-4 and S-6 stronger, not weaker.** A flat model would
be self-limiting: unable to express a preference, its choices would be arbitrary but
harmless. What exists instead swings by up to 0.30 on one substitution while ranking whole
matches at 0.56–0.63 — it will make *confident* selections on weak evidence. Improving the
search would find the maximum of that surface more thoroughly, which is not the same as
finding a better XI.

Worth keeping as a cheap diagnostic in S-9 regardless: a candidate feature set that does
*not* move p in response to a one-player swap cannot drive selection whatever its AUC. It
is not what fails here, but it is a fast way to rule a feature set out.

### Model class is not the constraint — but the shipped params are miscalibrated

T20, same holdout, same 63 features, three seeds each:

| model | held-out AUC |
|---|---|
| GradientBoosting, shallower + regularised (`max_depth=2`, `lr=0.03`, 300 trees) | **0.665 ± 0.000** |
| LogisticRegression | 0.651 |
| GradientBoosting **as shipped** (`max_depth=6`, `lr=0.1`, 100 trees) | 0.644 ± 0.004 |
| MLP (128, 64, 32) | 0.651 |
| MLP (64, 32) | 0.641 |

**Neural networks land at or below the linear baseline**, which is the expected result for
10,403 rows of 63 tabular features. Capacity is not the constraint; do not spend effort
there.

**Logistic regression beats the shipped boosted ensemble.** When a linear model
outperforms depth-6 boosting, there is no rich interaction structure to exploit — the
signal is weak and essentially additive. That is independent corroboration, from a
different direction, of one Elo number beating all 63 features.

**The nearly-free gain: `make auto-tune MODEL=win`.** The shipped configuration is
over-parameterised for this much signal and overfits; a shallower regularised fit is worth
**+0.021 AUC for no new data or features**. The repo already has the machinery, and
[#199](https://github.com/umayangag/cric-flow/pull/199) fixed it to optimise `roc_auc`
rather than accuracy precisely so tuning could not select a worse-ranking model — **it has
never been run for the win model since that fix.** Do this before any feature work, since
it needs no contract change.

Keep the scale in view: tuning reaches ~0.665, while match-level features alone already
reach 0.677 (T20) and 0.707 (T20I). **The feature gap dominates the model-class gap.**

*Note on the number:* the gain is +0.021 against the shipped config's three-seed mean of
0.644, not +0.028 against the single 0.632 draw recorded in the S-3c table. Same lesson as
the seeds section below.

### Measure with multiple seeds

Refitting T20 on identical data with only the row order changed moves held-out AUC by
**±0.009**, because `subsample=0.8` draws a different sample. So **any difference under
~0.02 is not evidence** — including TEST's 0.572 against ODI's 0.561 in the table above.
Report a mean and spread over seeds, not a single fit.

**Acceptance.** Held-out AUC materially above the numbers in the table above — by more
than the ±0.01 single-fit noise, over multiple seeds — measured by the same
`make win-discrimination` command on the same holdout, with the improvement attributable
to a named change rather than a re-roll.

**Risk.** The honest risk is spending effort to discover the ceiling is real. Bound it:
try the cheapest direction first and re-measure before continuing.

---

## S-9 results — the ceiling was the feature set, not the problem

**Method.** Rebuilt the win features from scratch, directly from the Cricsheet JSON, as one
chronological pass: for every match in date order, read features from state accumulated over
*earlier* matches only, then fold the match in. Nothing a feature reads can have seen the
match it describes, so the S-3c class of leak cannot exist by construction — there is no
snapshot table and no "latest as-of before the match" query to get wrong. Same format
taxonomy as `format.go`, same holdout (`2025-09-01`), three seeds, reported as mean ± sd.
The pass runs in ~100 s over all 22,734 files. Implementation: `ml-service/ml/xi/`.

### Held-out AUC by feature family

| feature family | T20 (n=1635) | ODI (n=375) | T20I (n=182) | TEST (n=157) |
|---|---|---|---|---|
| repo today, S-3c (68 windowed-form aggregates) | 0.632 | 0.561 | 0.591 | 0.572 |
| team-level only: Elo, form, h2h, venue (constant w.r.t. the XI) | 0.688 | 0.678 | 0.739 | 0.606 |
| player Elo only (XI-responsive) | 0.676 | 0.637 | 0.703 | 0.585 |
| ball-level impact ratings only (XI-responsive) | 0.687 | 0.642 | 0.682 | 0.559 |
| impact ratings + role coverage (XI-responsive) | **0.725** | **0.681** | **0.731** | 0.594 |
| all XI-responsive (player Elo + impact + roles) — the *objective* | **0.729** | **0.691** | **0.748** | 0.591 |
| everything (XI-responsive + team-level) — the *display* model | **0.741** | **0.716** | **0.755** | 0.601 |

Brier beats the base rate in every limited-overs format (T20 0.205 vs 0.250; ODI 0.210 vs
0.247; T20I 0.200 vs 0.250). TEST stays near chance for every family, as it did for Elo and
h2h in S-9's first table: Test sides are few, stable, and decided by things no lineup
feature carries. Leave TEST on the windowed-form model or, better, do not select for it.

**What the XI-responsive features are.** Every column is a function of the two elevens and
nothing else:

- **Ball-level impact ratings.** Per (player, format): runs above the context expectation per
  ball faced, dismissals below expectation, runs saved per ball bowled, bowler-credited
  wickets above expectation. The context expectation is the running average for (format,
  over number), so a death-overs 150 strike rate is worth more than a powerplay one and the
  ratings are venue- and era-neutral in the mean. Exponentially forgotten per match played
  (0.9) and shrunk toward zero with a 60-ball prior. Each is multiplied by the player's
  *expected involvement* — expected balls faced / bowled, known before the match — which is
  the "weight, do not truncate" idea from the S-9 plan: a bowling statistic is no longer
  averaged over the six players who never bowl.
- **Role coverage.** Number of bowling options (expected balls bowled ≥ 12 T20 / 30 ODI / 60
  TEST), keeper present (ever credited with a stumping), all-rounders, debutants, mean
  experience. Adding these to the impact ratings is the single largest jump in the ablation
  (T20 0.687 → 0.725). This is the *combination* signal the selector exists to use.
- **Player Elo.** Each player carries a rating in each format, moved by the results of the
  matches they played in — the individual analogue of the team Elo that beat the old model.

### The specific XI carries signal beyond the team's usual strength

The question a selector needs answered is not "can we predict matches" but "does *which
eleven* was fielded, as opposed to which team, change the outcome in a way the model sees".
Held-out AUC with team-level features plus:

| | T20 | ODI |
|---|---|---|
| the team's **typical** XI features (rolling mean of its previous five lineups) | 0.697 | 0.675 |
| the **actual** XI's features | **0.738** | **0.716** |

Knowing the actual lineup is worth +0.04 AUC over knowing the team's usual one. That is the
composition effect, and the features capture it. (T20I's 182 matches cannot resolve a 0.04
difference; its numbers are omitted rather than over-read.)

### Selection diagnostics (the S-9 "cheap diagnostic", extended)

- **One-player swap** (upgrade team1's weakest batter's batting to its best, everything else
  fixed): median Δp +0.026 T20 / +0.033 ODI, p90 +0.10 / +0.13. The objective moves on a
  single change, as the old model did — but now from a ranking of 0.73 rather than 0.63.
- **Objective must be monotone.** Unconstrained boosting lowers p for 12% of those upgrades
  (T20) and 20% (ODI) — tree interactions, not cricket. Monotone constraints cut that to
  6.5% / 16%; **logistic regression on the same columns is 0.4% / 3.7% at −0.005 AUC.** So the
  selection objective is the additive logistic model, and the boosted model is only the
  displayed probability. The old objective swung by up to 0.30 on one substitution while
  ranking whole matches at 0.56–0.63; this one is smaller per swap and better ranked, which
  is the right way round.
- **Cold start.** Replacing a player with a debutant moves p by a median −0.003 (p10 −0.05):
  unknown players regress to neutral, they do not explode.
- **Marginal value vs what the player then did.** Per holdout match, remove each team1 player
  (replace by an average one) and correlate Δp with the player's actual runs + wickets in
  that match: within-match Spearman +0.07 (T20, n=1633). Weak in absolute terms — one
  match's performance is mostly noise — but positive and grounded in ground truth, which no
  previous selection metric was.

### Closed by evidence

- The S-9 direction "player-level impact ratings — the individual analogue of Elo" is the
  answer; the direction "match-level features raise accuracy but cannot select" is confirmed
  (team-level only: 0.688; it adds +0.012 on top of the XI features for display).
- Model class is still not the constraint: logistic ≈ boosting on these columns too.
- The old export path (precompute snapshots → export CSV → train) is not on the critical path
  for selection any more. D-1 (duplicate snapshots) no longer affects the win objective.

---

## S-10 — The XI-responsive win model, wired behind a flag

**Change.** A new model family that owns its own features, so the objective the optimiser
maximises is the same function of the same eleven names at training and serving time.

| piece | location |
|---|---|
| Feature contract, monotone directions, rating hyperparameters | `ml-service/ml/xi/contract.py` |
| Match sources: Postgres (go-app schema) and Cricsheet JSON directory | `ml-service/ml/xi/sources.py` |
| The chronological rating pass, per-player vectors, side aggregation | `ml-service/ml/xi/ratings.py`, `builder.py` |
| Training + holdout report (both models, 3 seeds, base-rate Brier, best single column) | `ml-service/ml/xi/train.py` → `xi_win_<FMT>.joblib`, `xi_ratings.joblib`, `xi_win_report.json` |
| Serving store | `ml-service/ml/xi/store.py` |
| Optimiser: greedy seed → steepest-ascent single swaps → pair swaps, constraints via the same vectors the model reads | `ml-service/ml/xi/optimizer.py` |
| Endpoints `GET /xi/status`, `POST /xi/predict-win`, `POST /xi/optimize` (player ids in, no feature maps) | `ml-service/app/xi_service.py`, `app/models/xi.py`, routes in `app/main.py` |
| go-app: `XISelectionOptimizer` / `XIWinPredictor`, side optimiser, config `selection.win_model` | `predictteam/xi_selection.go`, `config/win_model.go`, `server/ml_xi_client.go`, seams + adapter |
| Make | `make train-xi CUTOFF=2025-09-01` (DB) or `CRICSHEET_DIR=data/go-app/cricsheet` (raw JSON) |

**Player identity.** The ML side keys ratings by the id string the source gives it: go-app
`player_id` from Postgres, the Cricsheet registry identifier from JSON. The Postgres path
therefore inherits [IDENTITY_PR_CHECKLIST.md](IDENTITY_PR_CHECKLIST.md)'s name-keyed merges
(163 names / 348 people; men's and women's sides sharing team ids). The JSON path does not,
which is why the numbers above are the cleaner measurement and why I-3/I-4 should land
before the DB-trained artifacts are trusted for women's cricket.

**What the flag does.** `selection.win_model: "xi"` in `go-app/config.json` routes
`selectTeamsByWinProbability` to `/xi/optimize` (still alternating best response, still
against the opponent's XI — S-1 holds) and `getMatchWinProbability` to `/xi/predict-win`.
Anything else, or any error from the XI path, falls back to the windowed-form model and logs
why. **Nothing changes until the flag is set.**

**Batting order is marginalised.** The training label makes team1 the side batting first, which is unknown when the XI is chosen. Both models are therefore scored in both orientations and averaged on the serving path, so `P(A, B) == 1 - P(B, A)` (unit-tested) and the optimiser's objective does not depend on the toss. Measured on the holdout this is also better: display AUC T20 0.741 -> 0.747, ODI 0.720 -> 0.730, T20I 0.715 -> 0.731 (`xi_win_report.json` reports both the oriented and the serving-path numbers). Once the toss is known `/xi/predict-win` accepts `team1_bats_first`. See ML_PIPELINE_REARCHITECTURE_PLAN.md, H-3.

**S-4 is superseded.** The XI optimiser already does steepest ascent, pair swaps when single
swaps stall, and a real budget (default 20,000 evaluations; a pool of 22 converges in ~3,000
and 0.4 s). It runs entirely inside ml-service, so the per-call path is not needed on this
objective.

**Tests.** `tests/unit/test_xi_ratings.py` (as-of property: a match's row is identical with
or without later matches and unaffected by its own result; per-format isolation; role
coverage; monotone directions; source ordering enforced; undecided matches rate players but
move no Elo) and `tests/unit/test_xi_optimizer_and_store.py` (store round trip; constraints,
must-include/exclude, infeasible pools; optimised ≥ fielded; marginal values; Cricsheet
parsing incl. bowler-credited vs run-out wickets and the format taxonomy). Go:
`predictteam/xi_selection_test.go` (ids not features are sent, opponent is an XI, fallback,
flag off means no call, name→id resolution).

**Acceptance — to run on the database.** Run; see "S-10 results" below.

1. `make train-xi CUTOFF=2025-09-01` → `output/ml-service/xi_win_report.json`. Expect T20
   objective AUC ≈ 0.72 and display ≈ 0.74; a large shortfall against the JSON-path numbers
   above means the Postgres source or player identity is dropping information, not that the
   model is worse.
2. `POST /admin/reload`, `GET /xi/status` shows the formats and the report.
3. Set `selection.win_model: "xi"`, run the S-3b selection comparison. Ship for limited-overs
   formats only if `winprob` beats `greedy` on winner accuracy; leave TEST on greedy.
4. `make check-all` (the Go side was written without a compiler in reach — see the S-10 note
   in "Where this stands").

**Risk.** Latency is one HTTP call per side per best-response round; measured in-process at
0.4–0.6 s per side for a 22–23 player pool. The rating state is ~13.6k players × 4 formats of
small arrays, loaded once.

---

## S-10 results — the model reproduces on Postgres; the selection gate does not pass

This is P-0 of [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md), run
end to end against the database. Two things were measured, and they came out differently: the
**model** reproduced its JSON-path numbers almost exactly, and the **selection gate** — does
`winprob` beat `greedy` on winner accuracy — did not pass.

### 1. `make train-xi CUTOFF=2025-09-01` on Postgres

22,425 matches read, 20,725 decided training rows, 1,700 undecided, 13,427 players, rating
state through 2026-08-25. The pass takes ~55 s; training all four formats ~5 s.

Serving-path numbers (both batting orders averaged, H-3) beside the JSON-path development
numbers this plan has been quoting:

| format | n train / holdout | objective AUC (JSON) | display AUC (JSON) | display Brier | base-rate Brier | best single column |
|---|---|---|---|---|---|---|
| T20 | 10,100 / 1,578 | **0.723** (0.73) | **0.751** (0.747) | 0.201 | 0.250 | `d_pelo_top3` 0.661 |
| ODI | 4,565 / 375 | **0.684** (0.69) | **0.726** (0.730) | 0.210 | 0.247 | `d_exp_mean_matches_all` 0.662 |
| T20I | 1,848 / 178 | **0.744** (0.75) | **0.721** (0.731) | 0.218 | 0.250 | `team_elo_diff` 0.727 |
| TEST | 1,924 / 157 | 0.585 | 0.585 | 0.258 | 0.250 | `t1_imp_bat_wk` 0.624 |

Every limited-overs figure lands within 0.01 of the JSON-path number. **There is no
Postgres shortfall**, so the name-keyed identity defect (I-3/I-4) is not costing the
aggregate what it might have; E4 still owes the women's-subset split, where it should show.

Two things the table says that are worth naming:

- **T20I display does not beat its own best single column** (0.721 vs `team_elo_diff` 0.727).
  On 178 matches that is inside the noise, but it is an H-2 canary and should be re-read at
  the next cutoff rather than waved through. The T20I *objective* does clear it (0.744).
- **TEST is 0.585.** Below the 0.65 floor in H-17, so TEST stays on greedy, as planned.

### 2. First locked-window report (H-19)

These *are* the locked-window numbers: trained strictly before 2025-09-01, scored on
2025-09-01 → 2026-08-25. The caveat §10.3 of the plan predicted a 0.01–0.02 drop from
holdout reuse; what the run actually isolates is the **source** delta (Postgres vs Cricsheet
JSON) at the *same* cutoff and the *same* window, because that window is the one that guided
the development choices. So it is the first locked-window *report*, not yet a locked-window
*measurement of unseen data*. The rolling-origin walk-forward in P-2 is what turns it into one.

### 3. S-3b selection comparison — the gate

`POST /api/backtest/selection-comparison` over every team pair with ≥ 5 decided matches in
the locked window: 55 pairs, 339 matches, both arms on every match, `selection.win_model:
"xi"`. One match failed for both arms (no squad recorded); the rest ran.

| format | decided matches | greedy | winprob (xi) | Δ |
|---|---|---|---|---|
| T20 | 179 | **0.598** | 0.548 | −0.050 |
| ODI | 51 | 0.471 | 0.471 | 0.000 |
| T20I | 102 | 0.569 | **0.628** | +0.059 |
| pooled | 332 | 0.569 (189) | 0.560 (186) | −0.009 |

**The gate is not met.** It asks for `winprob ≥ greedy` in limited-overs formats; T20 — the
largest sample — is 5 points the other way. Pooled, the two arms are three matches apart in
332, which is a tie.

Read honestly, none of the three format-level differences is resolvable at these sample
sizes: ±0.05 on 179 matches is ~1.3 standard errors, and T20 losing by 0.050 while T20I wins
by 0.059 is what noise looks like. The harness reports only aggregates, so a paired
(McNemar) test — the right one, since both arms saw the same matches — cannot be computed
from its output at all. **That is a gap in the harness, not a result.**

Supporting numbers: the two arms genuinely choose differently (31–36 of 44 selected players
differ per match; 5 identical XIs out of 339), so this is not a search that failed to move.
Both arms' probabilities come from the same XI display model — the arm changes *which XI*,
not which model scores it — so the comparison isolates the selection rule. The winprob arm's
probabilities are systematically less extreme (mean |p − 0.5| 0.153 vs 0.180 across pairs):
optimising *both* sides moves the matchup toward even, which costs winner accuracy without
saying the XIs are worse.

**Which is the deeper problem with the gate.** Winner accuracy asks whether the predicted
winner of a *counterfactual* fixture — our two chosen XIs — matches the result of the *real*
one, which was played by different teams. An arm that strengthens both sides toward parity
must lose on that metric even if every XI it picks is better. S-3b's own preamble already
warns that this metric "scores the win model and the selection jointly"; this run shows the
sharper version. The metrics that survive both sides being optimised are the ones the
re-architecture plan's L4 already lists: specific-XI-beyond-typical-XI, swap monotonicity,
and the E5 natural experiment (consecutive matches of the same side with 1–3 changes).

**Disposition.** Per S-6's own rule — "if it does not, the honest outcome is to leave the
flag off and record why" — win-probability selection stays off.
`selection.win_model: "xi"` is left set in `go-app/config.json`: with
`use_win_probability_selection` false it changes only which model reports P(win) and the
predicted winner, and that model is measured (0.72–0.75 AUC, Brier below base rate in every
limited-overs format) where the windowed-form model it replaces is 0.56–0.63. Turning the
*selection* on waits for a gate that can be passed by a working optimiser.

### 4. Two defects that blocked the run, both fixed here

Neither is S-10's; both stopped the acceptance dead and are repaired in this PR.

- **`ml-service/config.json` had `use_share_models: true` with no share artifacts and no way
  to build them.** Every prediction carrying match context — which is every backtest
  selection and every team prediction from the app — returned
  `503 TRAIN_ON_THE_FLY_FAILED: No artifacts loaded for format=…`. `batting_share_*` /
  `bowling_share_*` have never been trained here, and they cannot be: the trainer's
  `--share-targets` path needs an `innings_runs` column that the current export no longer
  carries. Set to `false`, which is what `config.default.json` ships and the only value the
  repo can actually serve. The reconciliation layer these models feed is deleted in P-4.
- **An interaction term was silently dropped at prediction, making every feature vector one
  column short.** The models are trained with three interactions, one of them
  `batting_trend_w5 × inning`; the serving contract (`configs/feature_vectors.json`, and the
  per-player map go-app sends) calls that column `batting_inning`. The lookup missed, the
  interaction was skipped without a word, and the 37-wide vector met a 38-wide scaler as
  `X has 37 features, but RobustScaler is expecting 38 features as input` — a message that
  names neither the model nor the column. `build_extended_vector_from_features` now resolves
  the operand through `CSV_COLUMN_MAP` (the same table training already uses, read the other
  way) and **raises** when an operand is genuinely absent, naming it, instead of returning a
  short vector.

### 5. What the harness could not do, and what P-2 owes it

Recorded so the next person does not rediscover them:

- **No as-of ratings on the serving path.** `XiStore` holds one rating state — through today
  — which is right for a live prediction and wrong for a backtest: scoring a 2025-11 match
  with ratings through 2026-08 lets team Elo carry that match's own result. This run worked
  around it with `scripts/experiments/xi/freeze_ratings.py`, which rebuilds the state with
  `PostgresSource(before=…)` and rewrites `xi_ratings.joblib`; the comparison above ran with
  ratings frozen at 2025-08-31, so the XI arm is *handicapped* (stale by up to a year) rather
  than flattered. **Fixed in P-2**: `ml.xi.asof` serves "ratings as of date D", `/xi/*`
  accept an `as_of` date, the comparison sends each match's date, and `freeze_ratings.py`
  is deleted.
- **The report is aggregate-only.** No per-match rows, so no paired test, no date filter, and
  no way to ask which matches the arms disagreed on. **Per-match rows landed in P-2**
  (`per_match` in the response, one entry per arm with probability, predicted winner and
  correctness). The endpoint still takes one team pair at a time with a limit of 50, so a
  window has to be assembled pair by pair from outside.
- **Pools are large.** `GetBacktestSquadPlayerIDs` returns 180–230 players, not the 22–23 the
  S-10 latency note assumed. The optimiser still converges in ~1,500–2,000 evaluations and
  110–150 ms, so the budget holds; but "overlap with the fielded XI" is near-meaningless when
  the pool is ten times the XI.

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

> **Reconsidered by S-5b, and the reason matters.** That note is conditional on S-6
> landing: it is only *once win probability is the objective* that these weights demote to
> a seed. **S-6 is `blocked` on S-3c**, which needs `info.players` parsed, a new
> `match_player` table, the export changed, then a full re-import, re-export and re-train.
> Until that completes the greedy path is not seeded by these weights — it *is* them, and
> they are currently chosen by an untuned `alpha=1.0` on a fit nothing has ever scored.
> That is what re-earns the rigour, and only that: the exclusions above still stand, minus
> the two columns S-5b adds to the contributions CSV. No post-cutoff holdout, no freshness
> gating, no non-linear meta model. See **S-5b**.

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

## S-5b — Auto-tune the combination meta-model

**Problem.** Nothing tunes the meta-model, and nothing measures it. `ml/tuning/cli.py:57`
accepts `batting|bowling|fielding|extras|win|innings|all` — no meta.
`training_orchestrator.py:185` invokes the trainer with only `--csv` and `--out`, so
`alpha=1.0`, global-not-per-format and `normalize_weights=True` are frozen at their
argparse defaults; the flags exist, nothing sets them. `train_and_export` fits one Ridge on
the whole CSV: no CV, no held-out score, no fit metric in the output beyond `n_samples`.
`registry_test.go:204` records the consequence in an assertion — "combination-meta has no
tuned-params model".

S-5 fixes what the model is asked to explain. This fixes how the answer is chosen, and
whether it was any good.

**Precedent.** `b9f29eb` switched the win model from `accuracy` to `roc_auc` because *"only
the order the model puts them in can change which side gets picked."* The same argument one
layer down: these weights rank a pool. A global MAE on `target` scores level-prediction,
which they are never used for, and with no `match_id` to group on it splits one match's
players across train and test folds. Two invariances follow and belong in the code: within
a match, ranking is unchanged by `intercept` (a constant shift) and by `normalize_weights`
(a uniform positive rescale). Neither enters the search; `normalize_weights` stays a
presentation flag.

**Change.** Four parts.

**(a) The contributions CSV gains `match_id` and `match_date`.** Both are in hand at export
time and thrown away; without them there is no grouping key and no temporal order, so any
CV here leaks. `ContributionRow` (`services/backtest/types.go:98`) gains the two fields, and
a `ContributionSource{Player, MatchID, MatchDate}` carries them — do **not** widen
`PlayerResult`, which is serialised into `EvaluateResponse` and must not grow export-only
fields. Both producers already have the values in scope: the batch path
(`backtest_export_contributions.go:231-238`) has `p.matchID` and `p.cutoff`, which *is* the
match date, from `getBacktestMatchDateFunc`; the fallback (`:265-276`) has `mid` and
`resp.Match.MatchDate`. Two columns and a wrapper struct is not the multi-fixture export
redesign S-5 declined.

**(b) A `--tune` path in `ml/train_combination_meta.py`.** Self-contained; it does **not**
route through `ml/tuning/`. The consumer contract — `accessors.go` reads
`bat`/`bowl`/`field`/`keeper_bonus` — forces a linear model, so the Optuna two-phase screen,
the PyCaret ranking, the AutoGluon comparison and every tree search space in
`search_space.py` produce estimators that cannot satisfy it. Reusing that stack would mean
threading a linear-only branch through each stage for a grid that runs in seconds.

| Knob | Values |
|---|---|
| `alpha` | `np.logspace(-3, 3, 7)` |
| `per_format` | `False`, `True` |
| estimator | `Ridge(...)`, `Ridge(..., positive=True, solver="lbfgs")` |

~24 fits. `positive=True` is available in the pinned sklearn (1.5.2, verified) and earns its
place independently of tuning: a negative bowl coefficient is a selection weight that says
*prefer worse bowlers*. S-5's acceptance criterion is a positive bowling coefficient — the
constraint guarantees it structurally rather than hoping a better target produces it.

*Objective:* mean per-match Spearman between `w·[bat, bowl, field, is_keeper]` and `target`
over held-out matches. Skip a match with fewer than 3 rows, or zero variance in either
vector, and report `n_matches_scored` beside the score so a number computed from almost
nothing is visible rather than silent.

*Validation:* an expanding window over **matches**, not rows — order unique `match_id` by
`match_date`, split the match list, map back to row indices. Neither `TimeSeriesSplit` (not
group-aware) nor `GroupKFold` (not time-aware) is correct alone; the helper is ~15 lines.
Below `n_splits + 1` matches, fall back to `GroupKFold`. Below 2 matches, do not tune: log
at error level and exit non-zero rather than emit a fitted-on-nothing result. Under
`per_format=True`, a format absent from a fold's training portion falls back to that fold's
global weights, logged.

*Output:* the existing JSON plus a `tuning` block — chosen `alpha`, `per_format`,
`positive`, `best_cv_score`, `scoring: "mean_per_match_spearman"`, `n_splits`, `n_matches`,
`n_matches_scored`. Reports record their `scoring`, so a file from before this change
identifies itself. New flags: `--tune`, `--tune-splits` (default 5), `--positive`.

*Persistence:* reuse `save_tuned_params_to_go_app(url, "combination_meta", "", …)`
(`ml/config.py:283`). Format key `""` — the meta model may be global, and `per_format` is a
knob inside the params, not a row key. `ml_tuned_params_handlers.go:14` has no model
whitelist, so no schema and no API change.

**(c) Close the loop at training time.** `run_combination_meta_training` reads the stored
params via `get_tuned_params_from_go_app(go_app_url, "combination_meta", "")`
(`ml/config.py:252`) and passes `--alpha` / `--per-format` / `--positive`, falling back to
argparse defaults when absent and logging which path it took. Deliberately **not**
`get_training_params`: it gates on `TRAINING_MODELS` (`ml/config.py:239`) and demands
`TRAINING_REQUIRED_KEYS` — `n_estimators`, `max_depth`, `joblib_compress` — none of which
mean anything to a four-coefficient Ridge, and inventing a config block of inapplicable keys
to reuse one function is the wrong trade. Argparse defaults are the config of record; the DB
overlays them. The function gains a `tune: bool`, threaded from a `tune` query param on
`POST /admin/train/combination-meta` (`app/main.py:844`), the way `auto-tune` takes
`rescreen`.

**(d) Wiring.** The step (`registry.go:296`) gains `Model: "combination_meta"` — it now has
a tuned-params row, so `IsTraining()` should be true, which routes it through
`confirmDefaultParams` (`pipeline_handlers.go:244`) and correctly warns when training on
untuned defaults. **`registry_test.go:204` flips** from `assert.False` to `assert.True`,
comment rewritten; the old comment was accurate when it was written.
`ml-service/Makefile:151` passes `$(if $(TUNE),--tune,)`, root `Makefile:247` forwards
`TUNE`, and the help text at `Makefile:684` mentions it. Docs: `docs/ml-and-training.md`
§306 and §315-317, `README.md` for the flag, and `make gen-architecture-map` — the CSV
column set is a contract.

**Reused, not rebuilt:** `save_tuned_params_to_go_app` / `get_tuned_params_from_go_app`, the
`ml_tuned_params` table and its handlers, `run_training_subprocess`.

**Tests.**

*Go* (`services/backtest`, external package, table-driven `testCases`, `for i := range`):
rows carry the match id and the match date; the header lists all eight columns; both the
batch and the fallback producer attribute a row to the right match.

*Python* (new `tests/test_train_combination_meta.py` — the module has no test file today):
the objective is invariant to a positive rescale of the weight vector and to the intercept;
a match with <3 rows or zero variance is skipped and excluded from `n_matches_scored`; the
splitter never puts one `match_id` on both sides of a split and the training side is always
earlier by `match_date`; `positive=True` yields no negative coefficient on a fixture where
an unconstrained Ridge does; fewer than 2 matches exits non-zero and writes no JSON;
`--tune` writes the `tuning` block and its absence leaves the output shape unchanged;
`training_orchestrator` passes stored params through and falls back cleanly when the lookup
returns nothing. Seeded fixtures under `tests/fixtures/`, no network.

Coverage ratchets up in all three places per component.

**Acceptance.** The tuned fit beats the `alpha=1.0` global baseline on mean per-match
Spearman over held-out matches, with both numbers and `n_matches_scored` in the PR body. No
coefficient is negative. **If the tuned fit does not beat the baseline, that is the
finding** — record it, keep the non-negativity constraint, which stands on its own, and say
in the PR body that the grid bought nothing.

**Depends on S-5.** Tuning against today's `target = actualRuns/batDiv` would select the
`alpha` that best explains runs from predicted *bowling* quality — optimising a target S-5
has already diagnosed as broken. Not blocked by S-3c: this concerns the combination-meta
seed, not the win model.

**Risk.** Low. The export writes two more columns; the tuner is opt-in behind `--tune`; the
training path is unchanged when no tuned-params row exists. The one behavioural change
outside the flag is `IsTraining()` becoming true, which adds a confirmation prompt to a step
that previously had none.

**Out of scope.** A non-linear meta model — `accessors.go` needs a weight vector. Adding
`combination_meta` to the auto-tune CLI or the frontend `AUTO_TUNE_MODELS` list: a
seconds-long grid with a different objective and a different artifact, surfaced beside the
six Optuna models, would imply a uniformity that does not exist.

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

**Team identity fragments on rename — found while checking the re-import.** A player's
team is per-match, which `match_player.opposition_id` records correctly (26% of the 13,419
players have played for more than one team; one for 31). But a *team* is not stable either:

```
 id  |       opposition_name       | matches | first_match | last_match
  292| Royal Challengers Bangalore |     258 | 2008-04-18  | 2024-03-17
 1038| Royal Challengers Bengaluru |      63 | 2024-03-22  | 2026-05-31
```

One franchise, two `opposition_id`s, non-overlapping dates. The model sees two unrelated
teams sitting at two arbitrary integers, and `team1_opposition_id` carries real weight in
the trained artifacts (TEST 0.050, T20I 0.019, T20 0.016). So S-7 is not only about
*encoding* the IDs — the identities being encoded are themselves split. Any target or
frequency encoding fitted before the split is resolved learns the rename as a new team
with 63 matches of history.

There are 394 opposition rows, and it is worse than one rename: **130 of the 394 names are
used by both a men's and a women's side**, sharing a single `opposition_id`, while 20% of
matches are women's cricket and the win export applies no gender filter.

**This is now [IDENTITY_PR_CHECKLIST.md](IDENTITY_PR_CHECKLIST.md), and S-7 is `blocked`
on its I-3 and I-4.** Encoding an identity that is itself split or merged just launders the
error into a smoother representation. That plan also covers the player side of the same
problem — 163 names holding 348 people — which is the larger defect and the one that
reaches these features. Run it **after** S-3c: the leak is the dominant effect, and
sequencing the two gives a measurement of each rather than one confounded jump.

**Not a problem, recorded so it is not re-litigated:** player form features are not
team-scoped — the win export reads `scope = 'overall'`, so a player's T20 form blends
every T20 side they have played for. Form is a property of the player, and unlike the
S-3c count leak it is not conditioned on the outcome.

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
| 1 | Are the three components of S-5's composite target summed with equal weight? Equal weighting says a 4-wicket spell at economy 6 is worth roughly 53 runs. **S-5b makes this measurable** — its per-match ranking objective scores a weighting against how well the resulting order matches what players actually did, so answer it there with a number rather than settling it by judgement in S-5 | S-5, revisited in S-5b | Equal weight, recorded in the S-5 PR body as an assumption |
| 2 | Best-response rounds: fixed count or iterate to a fixed point with a cap? | S-1 | 3 rounds, cap, return last completed round |
| 3 | ~~If S-3 shows win-model AUC near 0.5 on held-out matches, do we stop and improve the win model before S-4?~~ **Asked twice, answered twice.** The first run came back 0.93–0.96 and would have read as a green light; it was leakage (S-3c). Re-run on honest features: **0.56–0.63**, so the answer the decision named in advance now applies. **Stop.** S-4 and S-6 are blocked on the new S-9 | S-3, S-3c | Stopped. S-9 added |

---

## Where this stands, and what to do next

**Everything through S-3c is merged** (#214, #216, #218; the identity plan is #217). The
database has been re-imported, the export rebuilt over squads, and the win models
retrained to `CUTOFF=2025-09-01T00:00:00Z` — `output/ml-service/win_model_*.joblib` and
`win_discrimination.json` currently hold exactly that run.

**S-9 and S-10 are both done, and P-0 has been run.** `make check-all` is green (the Go side
needed only `gofumpt`; nothing failed to compile), `make train-xi CUTOFF=2025-09-01` on
Postgres reproduces the JSON-path numbers within 0.01, and the S-3b comparison ran over 332
locked-window matches with `selection.win_model: "xi"`. **The selection gate did not pass**
and win-probability selection stays off — see "S-10 results" for the numbers, the two
defects that had to be fixed to run it at all, and why winner accuracy is the wrong gate for
an arm that optimises both sides. The order S-9 was going to follow, kept for the record:

| order | action | expected | why first |
|---|---|---|---|
| 1 | `make auto-tune MODEL=win` | **+0.021 AUC** | No contract change. The machinery exists and has never been run since #199 fixed it to optimise `roc_auc` |
| 2 | Match-level features (Elo, form, h2h, venue, home) | **0.677 T20 / 0.707 T20I** vs 0.632 / 0.591 today | From the `match` table alone — no precompute, no schema change. **But it does nothing for selection** |
| 3 | XI-responsive features (weighted involvement, player impact ratings) | unknown | The *only* route that unblocks S-4 and S-6 |

**Closed with evidence — do not re-open without new information:** pooling the formats
into one model, neural networks, and selecting a subset of players by predicted
performance. All three were tested on this holdout and are recorded above with numbers.

**Two rules that now govern any further measurement here:**

1. **Report a mean over seeds.** A single fit's AUC moves ±0.01 from row ordering alone, so
   differences under ~0.02 are not evidence. This caught a mis-stated gain twice.
2. **Check the artifact's provenance, not just the arithmetic.** Read `n_samples` and
   `model_type` from the metadata sidecar before trusting a measurement — see D-3.

---

## Known defects found along the way, not owned by any item

Each was found while doing something else, verified, and left unfixed because it is out of
scope for the item that surfaced it. Recorded here so they are not rediscovered a third
time.

| # | Defect | Evidence | Impact |
|---|---|---|---|
| D-1 | **`feature_raw_stats_snapshots` holds duplicate rows.** 443,308 duplicate `(player_id, format_id, as_of_date)` groups for `scope='overall'` — 1.81M of 2.32M rows. The unique constraint includes `scope_id`, which is NULL there, and Postgres treats NULLs as distinct, so it never fires. **2,361 groups carry conflicting values.** | Two equivalent formulations of the win export disagreed on 411 matches | Every model that reads a snapshot. The win export now breaks the tie on `id DESC` so it is at least reproducible; nothing else does |
| D-2 | **The per-format win exports are wrong and unread.** `win_encoded_T20.csv` and `win_encoded_T20I.csv` are byte-identical and each contain *both* formats' rows | `SELECT format_code` over each file: both give `{T20: 12022, T20I: 2106}` | None today — only `win_encoded_all.csv` is consumed. It is a trap for the first person who reads the per-format files |
| D-3 | **`output/ml-service/` is shared mutable state with no run identity.** A concurrent job silently replaced the win artifacts behind a reported measurement; the `.joblib` and its metadata sidecar disagreed with each other for twelve minutes | Artifacts rewritten mid-session with full-data models while the metadata still described the cutoff-trained ones | Any measurement can be invalidated by an unrelated job. Detected only by checking `n_samples` in the sidecar |
| D-4 | **An interaction term was silently dropped at prediction, so every feature vector was one column short.** The models carry `batting_trend_w5 × inning`; the serving contract calls that column `batting_inning`, the lookup missed, and the skip was unlogged | `503 X has 37 features, but RobustScaler is expecting 38 features as input` on every match-context prediction | **Fixed in S-10's PR** — it blocked the acceptance run. The operand now resolves through `CSV_COLUMN_MAP` and a genuinely missing one raises, naming it |
| D-5 | **`ml-service/config.json` asked for share models that cannot exist.** `use_share_models: true`, but no `batting_share_*` / `bowling_share_*` artifact has ever been built here and the trainer's `--share-targets` path needs an `innings_runs` column the current export no longer carries | `503 TRAIN_ON_THE_FLY_FAILED: No artifacts loaded for format=…` on every prediction with match context | **Fixed in S-10's PR** — set to `false`, matching `config.default.json`. The reconciliation layer these models feed goes in P-4 |

D-1 is the one with real consequences and should be fixed before the identity work in
[IDENTITY_PR_CHECKLIST.md](IDENTITY_PR_CHECKLIST.md), since that plan rebuilds the same
tables. D-4 and D-5 together meant *no* prediction carrying match context had worked for some
time; nothing measured them because every metric in the repo is computed from exports rather
than through the serving path (H-8, train/serve parity, is exactly this hole).

---

## Out of scope

- Probability calibration of the win model. Monotone recalibration cannot change an
  argmax, so it is a reporting concern. See `docs/ml-and-training.md`.
- Batting-order and role optimisation within the chosen XI. Selection first.
- Any change to precompute, feature windows or the raw stats contract. Those are the
  slow loop described in `docs/ml-and-training.md` and would invalidate this plan's
  baselines mid-flight.
