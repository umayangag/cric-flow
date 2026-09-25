# Architecture Map

Concise reference for data flow, ML models, and aggregation. Use `@ARCHITECTURE_MAP.md` to avoid re-reading source files.

> Model shapes and the endpoint list are **generated** from the contracts — see the marked
> blocks below. Regenerate with `make gen-architecture-map`; `make gen-architecture-map-check`
> (and CI) fails if they are stale. The surrounding prose is hand-written.

---

## 1. Data Flow: Simulator → Models

### Sources
- **The event store** — `match`, `match_player`, `ball_event` — is the only input the XI layer reads. One chronological, as-of pass (`ml.xi.builder`) produces the win frame, the player-match frame and the serving rating state.
- **go-app** knows who is available (the pool) and which fixture this is; it sends **registry ids** (`player.external_id`, the key the rating state is built under), a format, team ids, a venue id and a date. It sends no features.
- There is no second source. The precompute snapshots, the export CSVs, the shared feature-vector
  file and go-app's `training-data` endpoint went with the models that read them (P-5, P-6), so
  there is no second feature computation to fall out of step with the first.

### Flow (prediction)

```
DB (pools for the two sides) → go-app resolves format / teams / venue / date
    → ml-service, by registry id (`player.external_id`, not `player_id` — see D-7a):
        - POST /xi/optimize        objective="win"     → the XI that maximises P(win), + marginal values
                                   objective="ratings" → the rating-ordered XI (H-17: TEST), optimised=false
        - POST /xi/predict-win                          → the displayed P(team1 wins)
        - POST /simulate           (T20, T20I, ODI)     → totals with 10-90 ranges, per-player ranges,
                                                          the median-band scorecard, P(win), spread shares
        - POST /performance/predict (no innings length) → per-player medians and 10-90 intervals
    → one response: two XIs, how they were chosen, P(win) with its source, and — where the
      format has an innings length — a scorecard whose lines and extras sum to the total
```

Both sides are chosen by **alternating best response**: each side is optimised against the
other side's currently selected XI, never against the other side's whole pool, because every
feature the objective reads is an aggregate over one eleven. The loop is capped
(`selection.best_response_rounds`) and stops early at a fixed point.

### Flow (evaluation)

```
make evaluate → rolling origins + the locked window → xi_evaluate_report.json
    → GET /xi/evaluate-report (ml-service) → GET /api/backtest/report (go-app) → the Evaluation report tab
```

---

## 2. Core ML Models: Hyperparameters and Shape

Hyperparameters come from `ml.xi.train.DISPLAY_GRID`: three points for the display model,
chosen on a temporal split inside the training rows, with the choice and its evidence written
into the run's `manifest.json`. There is no tuned-params table and no config block.

The shapes below are **generated** from the contracts themselves, so they cannot drift:

<!-- BEGIN GENERATED: models -- edit scripts/gen-architecture-map.py, not this block -->

| Model | Level | Inputs | Outputs | Input source |
|-------|-------|--------|---------|--------------|
| **XI win — objective** | Match | 40 | 1 — `team1_wins` | `ml.xi.contract.XI_FEATURE_COLS` — every column is a function of the two elevens |
| **XI win — display** | Match | 49 | 1 — `team1_wins` | `ml.xi.contract.DISPLAY_FEATURE_COLS` — the XI columns bar the Elo spread (B-7), plus team and venue context, home advantage and the toss |
| **Performance (L2-B)** | Player | 40 | 5 — `runs`, `balls_faced`, `wickets`, `runs_conceded`, `catches` | as-of player-match rows from `ml.xi.rows` |

One model of each kind per format: `T20`, `T20I`, `ODI`, `TEST`. The simulator (L2-C) trains nothing — it draws from the performance model.

Input feature names, in order:

- **XI win — objective** (40): `d_pelo_mean`, `d_pelo_top3`, `d_pelo_min`, `d_imp_bat_sum`, `d_imp_bat_top6`, `d_imp_bat_tail`, `d_imp_bat_wk`, `d_imp_bowl_sum`, `d_imp_bowl_top5`, `d_imp_bowl_wk`, `d_imp_bowl_wk_top5`, `d_n_bowlers` … (+28 more, see `ml.xi.contract.XI_FEATURE_COLS`)
- **XI win — display** (49): `d_pelo_mean`, `d_pelo_top3`, `d_pelo_min`, `d_imp_bat_sum`, `d_imp_bat_top6`, `d_imp_bat_tail`, `d_imp_bat_wk`, `d_imp_bowl_sum`, `d_imp_bowl_top5`, `d_imp_bowl_wk`, `d_imp_bowl_wk_top5`, `d_n_bowlers` … (+37 more, see `ml.xi.contract.DISPLAY_FEATURE_COLS`)

<!-- END GENERATED: models -->

Regenerate with `make gen-architecture-map`; CI fails if this block is stale.

---

## 2b. HTTP endpoints

<!-- BEGIN GENERATED: endpoints -- edit scripts/gen-architecture-map.py, not this block -->

**ml-service** (15 routes, from `app/main.py`):

| Method | Path |
|--------|------|
| GET | `/health` |
| GET | `/artifacts/status` |
| POST | `/admin/reload` |
| POST | `/admin/train/retrain` |
| POST | `/admin/train/evaluate` |
| POST | `/admin/train/stop` |
| GET | `/admin/train/progress` |
| GET | `/xi/status` |
| GET | `/xi/evaluate-report` |
| GET | `/xi/metric-glossary` |
| POST | `/xi/predict-win` |
| POST | `/performance/predict` |
| POST | `/xi/player-roles` |
| POST | `/simulate` |
| POST | `/xi/optimize` |

**go-app** (43 routes, from `internal/server/router.go`):

| Method | Path |
|--------|------|
| GET | `/health` |
| GET | `/readiness` |
| POST | `/import/cricsheet` |
| GET | `/api/health/ml` |
| GET | `/api/ml/xi-status` |
| GET | `/ops/status` |
| GET | `/ops/migrations` |
| GET | `/ops/suggestions` |
| POST | `/ops/pipeline/run/{step}` |
| POST | `/ops/pipeline/stop` |
| POST | `/ops/pipeline/run-plan` |
| GET | `/ops/pipeline/plan` |
| GET | `/ops/pipeline/stream` |
| GET | `/ops/data/feeds` |
| POST | `/ops/data/fetch` |
| GET | `/ops/data/staged` |
| POST | `/ops/data/extract` |
| GET | `/ops/data/datasets` |
| GET | `/ops/data/biography-coverage` |
| GET | `/api/options/teams-by-format` |
| GET | `/api/options/opponents` |
| GET | `/api/options/formats` |
| GET | `/api/canonical/formats` |
| GET | `/api/options/venues` |
| GET | `/api/options/candidates` |
| GET | `/api/players/search` |
| GET | `/players/{id}` |
| DELETE, POST | `/api/players/{id}/retirement` |
| GET | `/matches/{id}` |
| GET, POST | `/api/predict/team-selection` |
| GET | `/api/predictions` |
| GET | `/api/predictions/{id}` |
| GET | `/api/track-record` |
| GET | `/api/auctions` |
| POST | `/api/auctions` |
| GET | `/api/auctions/{id}` |
| POST | `/api/auctions/{id}/players` |
| POST | `/api/auctions/{id}/outcomes` |
| PUT | `/api/auctions/{id}/assumptions` |
| GET | `/api/auctions/{id}/opposition-suggestion` |
| POST | `/api/auctions/{id}/projection` |
| GET | `/api/backtest/report` |
| GET | `/api/backtest/metric-glossary` |

<!-- END GENERATED: endpoints -->

---

## 3. How the numbers are combined

### One picture, no rescaling

The totals, the per-player lines and the spread shares all come from **one set of draws**
(`ml.xi.simulator`). The batting side is authoritative: its innings is drawn sequentially by
expected slot under an as-of innings length, extras are Poisson at the as-of rate, and the
bowlers' figures are *attributions* of that innings — balls in proportion to expected balls
bowled, runs conceded a multinomial split of the total. So the scorecard lines plus extras sum
to the total shown by construction, and nothing is rescaled toward a second estimate. The
rescaling layers that used to do that were deleted in P-5.

### What the scorecard shows

- **Per player:** the median-band line — the mean over the draws whose side total lies in the
  central tenth of the total's distribution — with the 10-90 range of runs, balls, wickets and
  runs conceded beside it, and the player's share of the total's variance.
- **Per innings:** the median-band total, its extras, and the 10-90 range of the draws.
- **Per match:** P(win) with its source. E2 decided per format whether the simulated P(win) is
  a probability (within Brier tolerance of the display model on the folds) or a description of
  the draws; the display model is the headline unless the constant
  `simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED` says otherwise, and the other model's answer
  is reported beside it, never blended with it.

Where the format has no innings length there is no total at all: the per-player numbers are
L2-B's own medians and intervals, and the response carries no scorecard block rather than
summing eleven medians and calling it an innings.

### Selection

- **The objective** is the XI win model: additive in the XI features, so upgrading a player
  cannot lower the score (H-4, measured at 0.3 % violations).
- **Marginal value** per selected player is P(win) with the XI minus P(win) with that player
  replaced by a par player in his role (`optimizer.par_replacement`: the same expected balls
  faced and bowled and the same keeper flag, impact rates at zero, the initial rating) — the
  L3 explanation of why he is in it.
- **Where the objective does not rank** (H-17: TEST, holdout AUC under 0.65),
  `/xi/optimize` refuses the win objective and serves `objective: "ratings"` — the search's own
  seed order under the same constraints, evaluating no model — and every surface says the XI is
  not optimised.

## 4. Pipeline Order

**Three steps:** import → `make retrain CUTOFF=` → `make reload` (`POST /admin/reload`). The
rating pass reads the event store, so there is no precompute and no export in front of it.

- **retrain** writes one run into `runs/<run_id>/` — artifacts, the run's report and
  `manifest.json` — and publishes nothing.
- **reload** points `current` at a run and loads it. Naming a run is how you swap back to an
  earlier one; a retrain that published itself would leave nothing to swap back to.
- **evaluate** (`make evaluate`) runs L4 beside the pipeline and writes the report every
  backtest surface reads. It refits every model per fold per format, takes about an hour on the
  full database, and touches no artifact `current` points at — which is why it is optional.

An artifact set with no manifest, or whose arrays are not the arrays this code reads, is
**refused at load** with an error naming the run (H-16, D-6), and a live prediction against
ratings older than `ml.ratings_max_age_days` is refused with `RATINGS_STALE` (H-11).
