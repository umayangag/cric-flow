# Architecture Map

Concise reference for data flow, ML models, and aggregation. Use `@ARCHITECTURE_MAP.md` to avoid re-reading source files.

---

## 1. Data Flow: Simulator → Models

### Sources
- **go-app** computes features at cutoff (form, consistency, weather, venue, opposition, season, sequential features).
- **Feature vectors** are defined in `configs/feature_vectors.json` (single source of truth, including `match_date_unix` where used).
- Training data: `GET /api/backtest/training-data?format=all&cutoff=...` or exported CSVs (`go-app/export-dataset` → `batting_encoded_*.csv`, etc.). Sections now cover **batting**, **bowling**, **fielding**, **extras**, **win**, and **innings**.

### Flow (backtest / team prediction)

```
DB (match/squad/cutoff) → go-app features (ComputeFeaturesAtCutoffForMatch / ComputeFeaturesAtCutoffForFutureMatch)
    → ml-service:
        - /predict/batting, /predict/bowling, /predict/fielding (player-level)
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

All training params from `ml-service/config.json` → `ml.training.<model>` (defaults in `config.default.json`). Base estimator via `ml.utils.make_base_estimator` (RandomForest, GBM, quantile, ExtraTrees, stacked, etc.).

| Model | Level | Input dim | Output dim | n_estimators | max_depth | Other |
|-------|-------|-----------|------------|--------------|-----------|-------|
| **Batting** | Player | 27 | 5 | 200 | 12 | StandardScaler on X; MultiOutputRegressor |
| **Bowling** | Player | 26 | 3 | 200 | 12 | StandardScaler on X; MultiOutputRegressor |
| **Fielding** | Player | 15 | 3 | 150 | 10 | StandardScaler on X; MultiOutputRegressor |
| **Extras** | Match | 16 | 1 | 100 | 8 | RandomForestRegressor; no scaler |
| **Win** | Match | 22 | 1 (prob) | 100 | 8 | RandomForestClassifier; no scaler |
| **Innings** | Innings | 17 | 2 | 100 | 8 | StandardScaler on X; MultiOutputRegressor (runs, wickets) |

### Batting
- **Inputs** (27, from `feature_vectors.batting`): `batting_consistency`, `batting_form`, `batting_form_short`, `batting_form_long`, `batting_momentum`, `batting_temp`, `batting_wind`, `batting_rain`, `batting_humidity`, `batting_cloud`, `batting_pressure`, `batting_viscosity`, `batting_inning`, `batting_session`, `toss`, `venue`, `opposition`, `season`, `match_date_unix`, + 8 seq: `bat_prev_sr`, `bat_prev_out_rate`, `bat_window_sr_12_pp`, `bat_window_boundary_rate_12_pp`, `bat_entry_sr_1_6`, `bat_set_sr_13_30`, `bat_react_after_dot_sr`, `bat_after_k_dots_boundary_p_k2`.
- **Outputs** (5): `runs`, `balls`, `fours`, `sixes`, `batting_position` (strike_rate derived).

### Bowling
- **Inputs** (26, from `feature_vectors.bowling`): `bowling_consistency`, `bowling_form`, `bowling_momentum`, `bowling_career_avg`, `bowling_temp`, `bowling_wind`, `bowling_rain`, `bowling_humidity`, `bowling_cloud`, `bowling_pressure`, `bowling_viscosity`, `batting_inning`, `bowling_session`, `toss`, `bowling_venue`, `bowling_opposition`, `season`, `match_date_unix`, + 8 seq: `bowl_prev_wkt_rate`, `bowl_window_econ_24_death`, `bowl_window_wkt_rate_24_death`, `bowl_extras_wide_rate_pp`, `bowl_react_after_boundary_wkt_rate_next`, `bowl_spell_first_over_wkt_rate`, `bowl_over_ball1_wkt_rate`, `bowl_over_ball6_wkt_rate`.
- **Outputs** (3): `runs_conceded`, `deliveries`, `wickets_taken` (econ derived).

### Fielding
- **Inputs** (16, from `feature_vectors.fielding`): `fielding_consistency`, `fielding_form`, `fielding_temp`, `fielding_wind`, `fielding_rain`, `fielding_humidity`, `fielding_cloud`, `fielding_pressure`, `fielding_viscosity`, `inning`, `toss`, `fielding_venue`, `fielding_opposition`, `season_id`, `match_date_unix`.
- **Outputs** (3): `catches`, `run_outs`, `stumpings`.

### Extras
- **Inputs** (16, from `ml.train_extras.EXTRAS_FEATURE_COLS`): `format_id`, `venue_id`, `season_id`, `match_date_unix`, `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity`, `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum`.
- **Output** (1): `total_extras`.

### Win
- **Inputs** (22, from `ml.train_win.WIN_FEATURE_COLS`): `format_id`, `venue_id`, `match_date_unix`, `team1_opposition_id`, `team2_opposition_id`, `toss_winner_opposition_id`, `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity`, `team1_bat_consistency_sum`, `team1_bowl_consistency_sum`, `team2_bat_consistency_sum`, `team2_bowl_consistency_sum`, `team1_bat_form_sum`, `team1_bowl_form_sum`, `team2_bat_form_sum`, `team2_bowl_form_sum`.
- **Output** (1): `team1_win_probability` (0–1).

### Innings
- **Inputs** (17, from `ml.train_innings.INNINGS_FEATURE_COLS`): `format_id`, `venue_id`, `season_id`, `match_date_unix`, `inning_number`, `opposition_id`, `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity`, `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum`.
- **Outputs** (2): `innings_runs`, `innings_wickets` (used for hybrid reconciliation; no public predict endpoint).

---

## 3. Aggregator: How Results Are Combined

### Match aggregates (backtest)

- **predRuns** = sum of player `runs` predictions.
- **predWickets** = sum of player `wickets` predictions.
- **predWinner** = team with higher sum of player runs (team totals from `playerTeams`).
- **predExtras** = extras model output if available; else historical average for format+venue.

**Hybrid reconciliation (innings model)**:

- When the **innings model** is loaded and match context is provided (team assignments, format, venue, weather), ml-service predicts `innings_runs` and `innings_wickets` per innings.
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
