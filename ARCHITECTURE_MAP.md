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
- `configs/feature_vectors.json` and `GET /api/backtest/training-data` still feed the windowed-form win model and the auto-tune stack, both of which P-6 removes.

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
make xi-evaluate → rolling origins + the locked window → xi_evaluate_report.json
    → GET /xi/evaluate-report (ml-service) → GET /api/backtest/report (go-app) → the Evaluation report tab
```

---

## 2. Core ML Models: Hyperparameters and Shape

Training params come from `ml-service/config.json` → `ml.training.<model>` (defaults in
`config.default.json`); the base estimator from `ml.utils.make_base_estimator`.

The shapes below are **generated** from the contracts themselves, so they cannot drift:

<!-- BEGIN GENERATED: models -- edit scripts/gen-architecture-map.py, not this block -->

| Model | Level | Inputs | Outputs | Input source |
|-------|-------|--------|---------|--------------|
| **XI win — objective** | Match | 42 | 1 — `team1_wins` | `ml.xi.contract.XI_FEATURE_COLS` — every column is a function of the two elevens |
| **XI win — display** | Match | 49 | 1 — `team1_wins` | `ml.xi.contract.DISPLAY_FEATURE_COLS` — the XI columns plus team and venue context |
| **Performance (L2-B)** | Player | 42 | 5 — `runs`, `balls_faced`, `wickets`, `runs_conceded`, `catches` | as-of player-match rows from `ml.xi.rows` |
| **Win — windowed form** | Match | 68 | 1 — `team1_wins` | `ml.win_features.WIN_ENHANCED_FEATURE_COLS` — superseded, removed in P-6 |

One model of each kind per format: `T20`, `T20I`, `ODI`, `TEST`. The simulator (L2-C) trains nothing — it draws from the performance model.

Input feature names, in order:

- **XI win — objective** (42): `d_pelo_mean`, `d_pelo_top3`, `d_pelo_min`, `d_imp_bat_sum`, `d_imp_bat_top6`, `d_imp_bat_tail`, `d_imp_bat_wk`, `d_imp_bowl_sum`, `d_imp_bowl_top5`, `d_imp_bowl_wk`, `d_imp_bowl_wk_top5`, `d_n_bowlers` … (+30 more, see `ml.xi.contract.XI_FEATURE_COLS`)
- **XI win — display** (49): `d_pelo_mean`, `d_pelo_top3`, `d_pelo_min`, `d_imp_bat_sum`, `d_imp_bat_top6`, `d_imp_bat_tail`, `d_imp_bat_wk`, `d_imp_bowl_sum`, `d_imp_bowl_top5`, `d_imp_bowl_wk`, `d_imp_bowl_wk_top5`, `d_n_bowlers` … (+37 more, see `ml.xi.contract.DISPLAY_FEATURE_COLS`)
- **Win — windowed form** (68): `venue_id`, `team1_opposition_id`, `team2_opposition_id`, `format_is_TEST`, `format_is_ODI`, `format_is_T20`, `format_is_T20I`, `format_is_OTHER`, `team1_bat_consistency_sum`, `team1_bat_consistency_mean`, `team1_bat_consistency_std`, `team1_bat_consistency_max` … (+56 more, see `ml.win_features`)

<!-- END GENERATED: models -->

Regenerate with `make gen-architecture-map`; CI fails if this block is stale.

---

## 2b. HTTP endpoints

<!-- BEGIN GENERATED: endpoints -- edit scripts/gen-architecture-map.py, not this block -->

**ml-service** (17 routes, from `app/main.py`):

| Method | Path |
|--------|------|
| GET | `/health` |
| GET | `/artifacts/status` |
| GET | `/model-metadata` |
| GET | `/model-stats` |
| POST | `/predict/win` |
| POST | `/predict/win-enhanced` |
| POST | `/admin/reload` |
| POST | `/admin/train/win` |
| POST | `/admin/train/auto-tune` |
| GET | `/admin/train/progress` |
| GET | `/admin/train/auto-tune/progress` |
| GET | `/xi/status` |
| GET | `/xi/evaluate-report` |
| POST | `/xi/predict-win` |
| POST | `/performance/predict` |
| POST | `/simulate` |
| POST | `/xi/optimize` |

**go-app** (32 routes, from `internal/server/router.go`):

| Method | Path |
|--------|------|
| GET | `/health` |
| GET | `/readiness` |
| POST | `/precompute` |
| GET | `/precompute/status` |
| POST | `/import/cricsheet` |
| GET | `/ops/status` |
| GET | `/ops/migrations` |
| GET | `/ops/migrations/{id:[0-9]+}/auto-tune` |
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
| GET | `/api/options/teams` |
| GET | `/api/options/teams-by-format` |
| GET | `/api/options/opponents` |
| GET | `/api/options/formats` |
| GET | `/api/canonical/formats` |
| GET | `/api/options/venues` |
| GET | `/players/{id}` |
| GET | `/matches/{id}` |
| GET, POST | `/api/predict/team-selection` |
| GET | `/api/backtest/training-data` |
| GET | `/api/ml/tuned-params/list` |
| GET | `/api/ml/tuned-params` |
| POST | `/api/ml/tuned-params` |

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
  replaced by a neutral, average one — the L3 explanation of why he is in it.
- **Where the objective does not rank** (H-17: TEST, holdout AUC under 0.65),
  `/xi/optimize` refuses the win objective and serves `objective: "ratings"` — the search's own
  seed order under the same constraints, evaluating no model — and every surface says the XI is
  not optimised.

## 4. Pipeline Order

**The XI layer:** import → `make train-xi CUTOFF=` → `POST /admin/reload`. It reads the event
store, so there is no precompute and no export in front of it. `make xi-evaluate` scores what
that produced and writes the report every backtest surface reads.

**The windowed-form win model** (P-6 removes it, and these steps with it): precompute → export
→ `make train-win CUTOFF=`, optionally with `make ml-auto-tune` before it when the feature space
has changed.
