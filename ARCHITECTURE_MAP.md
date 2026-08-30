# Architecture Map

Concise reference for data flow, ML models, and aggregation. Use `@ARCHITECTURE_MAP.md` to avoid re-reading source files.

> Model shapes and the endpoint list are **generated** from the contracts — see the marked
> blocks below. Regenerate with `make gen-architecture-map`; `make gen-architecture-map-check`
> (and CI) fails if they are stale. The surrounding prose is hand-written.

---

## 1. Data Flow: Simulator → Models

### Sources
- **go-app** computes features at cutoff (rolling form windows, venue, opposition, cyclical match date, sequential features).
- **Feature vectors** are defined in `configs/feature_vectors.json` (single source of truth). Match date is encoded cyclically (`match_month_sin/cos`, `match_day_of_week_sin/cos`); the older `match_date_unix` was replaced in v3.
- Training data: `GET /api/backtest/training-data?format=all&cutoff=...` or exported CSVs (`go-app/export-dataset` → `batting_encoded_*.csv`, etc.). Sections now cover **batting**, **bowling**, **fielding**, **extras**, **win**, and **innings**.

### Flow (backtest / team prediction)

```
DB (match/squad/cutoff) → go-app features (ComputeFeaturesAtCutoffForMatch / ComputeFeaturesAtCutoffForFutureMatch)
    → ml-service:
        - /predict/batting, /predict/bowling (player-level; fielding has no direct endpoint)
        - /predict/extras, /predict/win (match-level)
        - innings model (no direct endpoint; used via /ml/backtest and team prediction for hybrid reconciliation)
    → per-player predictions (runs, balls, wickets, catches, etc.)
    → aggregator (sum runs, winner from team totals; extras from model or historical avg)
```

### Flow (Monte Carlo simulation)

```
Same as above: player predictions from batting/bowling/fielding
    → SelectTopK XIs per team (optimizer with score weights)
    → For each (XI₁, XI₂): sample N outcomes
        - Per player: sampleRuns(mean, CV) from Normal(mean, mean×CV)
        - Innings total = sum(sampled runs) + extras
    → Win probability = count(tot1>tot2)/N, count(tot2>tot1)/N, draw/N
    → Innings percentiles: P10, P50, P90
```

**Simulation defaults** (config): `RunsCV=0.35`, `WicketsCV=0.4`, `EconomyCV=0.15`; `TopKPerTeam=50`, `NumSamplesPerMatchup=500`.

---

## 2. Core ML Models: Hyperparameters and Shape

Training params come from `ml-service/config.json` → `ml.training.<model>` (defaults in
`config.default.json`); the base estimator from `ml.utils.make_base_estimator`.

The shapes below are **generated** from the contracts themselves, so they cannot drift:

<!-- BEGIN GENERATED: models -- edit scripts/gen-architecture-map.py, not this block -->

| Model | Level | Inputs | Outputs | Input source |
|-------|-------|--------|---------|--------------|
| **Batting** | Player | 35 | 5 — `runs`, `balls`, `fours`, `sixes`, `batting_position` | `configs/feature_vectors.json` → `batting` |
| **Bowling** | Player | 35 | 3 — `runs`, `balls`, `wickets` | `configs/feature_vectors.json` → `bowling` |
| **Fielding** | Player | 10 | 3 — `catches`, `run_outs`, `stumpings` | `configs/feature_vectors.json` → `fielding` |
| **Extras** | Match | 17 | 1 — `total_extras` | `ml.train_extras.EXTRAS_FEATURE_COLS` |
| **Win** | Match | 77 | 1 — `team1_wins` | `ml.win_features.WIN_ENHANCED_FEATURE_COLS` |
| **Innings** | Innings | 19 | 2 — `innings_runs`, `innings_wickets` | `ml.train_innings.INNINGS_FEATURE_COLS` |

Input feature names, in order:

- **Batting** (35): `batting_mean_w3`, `batting_mean_w5`, `batting_mean_w10`, `batting_mean_w20`, `batting_std_w5`, `batting_std_w10`, `batting_max_w10`, `batting_min_w10`, `batting_median_w10`, `batting_last_1`, `batting_last_2`, `batting_last_3`, `batting_career_mean`, `batting_career_count`, `batting_pct_zero_w10`, `batting_trend_w5`, `batting_days_since_last`, `batting_innings_in_last_90d`, `batting_inning`, `batting_session`, `toss`, `venue`, `opposition`, `match_month_sin`, `match_month_cos`, `match_day_of_week_sin`, `match_day_of_week_cos`, `bat_prev_sr`, `bat_prev_out_rate`, `bat_window_sr_12_pp`, `bat_window_boundary_rate_12_pp`, `bat_entry_sr_1_6`, `bat_set_sr_13_30`, `bat_react_after_dot_sr`, `bat_after_k_dots_boundary_p_k2`
- **Bowling** (35): `bowling_mean_w3`, `bowling_mean_w5`, `bowling_mean_w10`, `bowling_mean_w20`, `bowling_std_w5`, `bowling_std_w10`, `bowling_max_w10`, `bowling_min_w10`, `bowling_median_w10`, `bowling_last_1`, `bowling_last_2`, `bowling_last_3`, `bowling_career_mean`, `bowling_career_count`, `bowling_pct_zero_w10`, `bowling_trend_w5`, `bowling_days_since_last`, `bowling_innings_in_last_90d`, `batting_inning`, `bowling_session`, `toss`, `bowling_venue`, `bowling_opposition`, `match_month_sin`, `match_month_cos`, `match_day_of_week_sin`, `match_day_of_week_cos`, `bowl_prev_wkt_rate`, `bowl_window_econ_24_death`, `bowl_window_wkt_rate_24_death`, `bowl_extras_wide_rate_pp`, `bowl_react_after_boundary_wkt_rate_next`, `bowl_spell_first_over_wkt_rate`, `bowl_over_ball1_wkt_rate`, `bowl_over_ball6_wkt_rate`
- **Fielding** (10): `fielding_consistency`, `fielding_form`, `inning`, `toss`, `fielding_venue`, `fielding_opposition`, `match_month_sin`, `match_month_cos`, `match_day_of_week_sin`, `match_day_of_week_cos`
- **Extras** (17): `venue_id`, `match_month_sin`, `match_month_cos`, `match_day_of_week_sin`, `match_day_of_week_cos`, `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum`, `form_differential`, `consistency_differential`, `weather_composite` … (+5 more, see `ml.train_extras`)
- **Win** (77): `venue_id`, `team1_opposition_id`, `team2_opposition_id`, `format_is_TEST`, `format_is_ODI`, `format_is_T20`, `format_is_T20I`, `format_is_OTHER`, `team1_bat_consistency_sum`, `team1_bat_consistency_mean`, `team1_bat_consistency_std`, `team1_bat_consistency_max` … (+65 more, see `ml.win_features`)
- **Innings** (19): `venue_id`, `inning_number`, `opposition_id`, `match_month_sin`, `match_month_cos`, `match_day_of_week_sin`, `match_day_of_week_cos`, `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum`, `form_differential` … (+7 more, see `ml.train_innings`)

<!-- END GENERATED: models -->

Regenerate with `make gen-architecture-map`; CI fails if this block is stale.

---

## 2b. HTTP endpoints

<!-- BEGIN GENERATED: endpoints -- edit scripts/gen-architecture-map.py, not this block -->

**ml-service** (25 routes, from `app/main.py`):

| Method | Path |
|--------|------|
| GET | `/health` |
| GET | `/artifacts/status` |
| GET | `/model-metadata` |
| GET | `/model-stats` |
| POST | `/ml/backtest/predict` |
| POST | `/ml/backtest/predict-batch` |
| POST | `/api/ml/generate-match` |
| POST | `/ml/backtest/match` |
| POST | `/predict/batting` |
| POST | `/predict/bowling` |
| POST | `/predict/extras` |
| POST | `/predict/win` |
| POST | `/predict/win-enhanced` |
| POST | `/optimize/team-selection` |
| POST | `/admin/reload` |
| POST | `/admin/train/batting` |
| POST | `/admin/train/bowling` |
| POST | `/admin/train/fielding` |
| POST | `/admin/train/extras` |
| POST | `/admin/train/win` |
| POST | `/admin/train/innings` |
| POST | `/admin/train/combination-meta` |
| POST | `/admin/train/auto-tune` |
| GET | `/admin/train/progress` |
| GET | `/admin/train/auto-tune/progress` |

**go-app** (44 routes, from `internal/server/router.go`):

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
| POST | `/predict/batting` |
| POST | `/predict/bowling` |
| GET, POST | `/api/predict/team-selection` |
| GET | `/api/backtest/match` |
| GET | `/api/backtest/evaluate-stream` |
| GET, POST | `/api/backtest/evaluate-start` |
| GET | `/api/backtest/evaluate-status` |
| GET | `/api/backtest/scorecard` |
| GET | `/api/backtest/training-data` |
| GET | `/api/backtest/matches` |
| GET | `/api/backtest/holdout-data` |
| GET | `/api/backtest/accuracy-trend` |
| POST | `/api/backtest/export-contributions` |
| GET | `/api/backtest/export-contributions-status` |
| GET | `/api/ml/tuned-params/list` |
| GET | `/api/ml/tuned-params` |
| POST | `/api/ml/tuned-params` |

<!-- END GENERATED: endpoints -->

---

## 3. Aggregator: How Results Are Combined

### Match aggregates (backtest)

- **predRuns** = sum of player `runs` predictions.
- **predWickets** = sum of player `wickets` predictions.
- **predWinner** = team with higher sum of player runs (team totals from `playerTeams`).
- **predExtras** = extras model output if available; else historical average for format+venue.

**Hybrid reconciliation (innings model)**:

- When the **innings model** is loaded and match context is provided (team assignments, format, venue), ml-service predicts `innings_runs` and `innings_wickets` per innings.
- Player-level `runs`, `wickets`, and bowling economy are then **rescaled** so that:
  - sum(batsman runs) = innings_runs
  - sum(bowler wickets) = innings_wickets
  - runs conceded by bowlers match innings totals

### Player combinator (team prediction)

- **total_score** = `sum(runs_scored) × (team_size / len(pool)) + extras`.
- **target** = `sum(runs_conceded) × magic_number`.
- **batting_contribution** = runs_scored / total_score.
- **bowling_contribution** = runs_conceded / target.

**Scorecard summary and win model (team prediction)**:

- For a selected XI per team, `innings1_total` and `innings2_total` are built from summed `runs` plus **extras per innings** (currently from DB average via `GetAverageExtrasForFormat`, split across innings; extras model is used in backtest and other flows when explicitly requested).
- Baseline winner = team with higher innings total.
- When the **win model** is loaded, go-app builds match-level `WinFeatures` from the selected XIs and feature map, calls `/predict/win`, and:
  - sets `team1_win_probability` from the model output.
  - rescales player `Runs` so that innings totals are consistent with the win probability (feedback from match-level to player level).

### Monte Carlo simulation

- Per matchup: for each XI, `innings_total = sum(sampleRuns(p.Runs, RunsCV)) + extras`.
- **win_probability_team1** = count(tot1 > tot2) / N.
- **win_probability_team2** = count(tot2 > tot1) / N.
- **draw_probability** = count(tot1 == tot2) / N.
- **innings_total_mean/std** and **p10/p50/p90** from sampled innings totals.

### Team selection (optimizer vs greedy)

- **Optimizer** (SelectOptimized): maximize total score = bat_score×w_bat + bowl_score×w_bowl + field_score×w_field + keeper_bonus; constraints: min bowlers, require keeper.
- **Greedy** (SelectTopK / PredictWin): rank by `WinningProbability`, pick top XI; swap to satisfy bowler constraint.
- Score weights from config or combination meta-model (`selection.meta_model_path`).

---

## 4. Pipeline Order

1. Precompute (form, consistency, sequences).
2. Export dataset (CSVs or API).
3. Train models (batting, bowling, fielding, extras, win, innings).
4. Run ml-service (load artifacts).
5. Optional: auto-tune, walk-forward, combination meta-model.
