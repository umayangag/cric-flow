# Architecture Map

Concise reference for data flow, ML models, and aggregation. Use `@ARCHITECTURE_MAP.md` to avoid re-reading source files.

---

## 1. Data Flow: Simulator → Models

### Sources
- **go-app** computes features at cutoff (form, consistency, weather, venue, opposition, season, sequential features).
- **Feature vectors** are defined in `configs/feature_vectors.json` (single source of truth).
- Training data: `GET /api/backtest/training-data?format=all&cutoff=...` or exported CSVs (`go-app/export-dataset` → `batting_encoded_*.csv`, etc.).

### Flow (backtest / team prediction)

```
DB (match/squad/cutoff) → go-app features (ComputeFeaturesAtCutoffForMatch)
    → ml-service /predict/batting, /predict/bowling, /predict/fielding
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

## 2. Five ML Models: Hyperparameters and Shape

All training params from `ml-service/config.json` → `ml.training.<model>`. Base estimator via `ml.utils.make_base_estimator` (RandomForest, GBM, quantile, or stacked).

| Model | Level | Input dim | Output dim | n_estimators | max_depth | Other |
|-------|-------|-----------|------------|--------------|-----------|-------|
| **Batting** | Player | 26 | 5 | 200 | 12 | StandardScaler on X; MultiOutputRegressor |
| **Bowling** | Player | 26 | 3 | 200 | 12 | StandardScaler on X; MultiOutputRegressor |
| **Fielding** | Player | 14 | 3 | 150 | 10 | StandardScaler on X; MultiOutputRegressor |
| **Extras** | Match | 14 | 1 | 100 | 8 | RandomForestRegressor; no scaler |
| **Win** | Match | 20 | 1 (prob) | 100 | 8 | RandomForestClassifier; no scaler |

### Batting
- **Inputs** (26): `batting_consistency`, `batting_form`, `batting_form_short`, `batting_form_long`, `batting_momentum`, `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity`, `inning`, `batting_session`, `toss`, `venue`, `opposition`, `season`, + 8 seq: `bat_prev_sr`, `bat_prev_out_rate`, `bat_window_sr_12_pp`, `bat_window_boundary_rate_12_pp`, `bat_entry_sr_1_6`, `bat_set_sr_13_30`, `bat_react_after_dot_sr`, `bat_after_k_dots_boundary_p_k2`.
- **Outputs** (5): `runs`, `balls`, `fours`, `sixes`, `batting_position` (strike_rate derived).

### Bowling
- **Inputs** (26): `bowling_consistency`, `bowling_form`, `bowling_form_short`, `bowling_form_long`, `bowling_momentum`, `bowling_temp`, `bowling_wind`, `bowling_rain`, `bowling_humidity`, `bowling_cloud`, `bowling_pressure`, `bowling_viscosity`, `inning`, `bowling_session`, `toss`, `bowling_venue`, `bowling_opposition`, `season`, + 8 seq: `bowl_prev_wkt_rate`, `bowl_window_econ_24_death`, `bowl_window_wkt_rate_24_death`, `bowl_extras_wide_rate_pp`, `bowl_react_after_boundary_wkt_rate_next`, `bowl_spell_first_over_wkt_rate`, `bowl_over_ball1_wkt_rate`, `bowl_over_ball6_wkt_rate`.
- **Outputs** (3): `runs_conceded`, `deliveries`, `wickets_taken` (econ derived).

### Fielding
- **Inputs** (14): `fielding_consistency`, `fielding_form`, `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity`, `inning`, `toss`, `fielding_venue`, `fielding_opposition`, `season_id`.
- **Outputs** (3): `catches`, `run_outs`, `stumpings`.

### Extras
- **Inputs** (14): `format_id`, `venue_id`, `season_id`, `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity`, `bat_consistency_sum`, `bowl_consistency_sum`, `bat_form_sum`, `bowl_form_sum`.
- **Output** (1): `total_extras`.

### Win
- **Inputs** (20): `format_id`, `venue_id`, `team1_opposition_id`, `team2_opposition_id`, `toss_winner_opposition_id`, `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity`, `team1_bat_consistency_sum`, `team1_bowl_consistency_sum`, `team2_bat_consistency_sum`, `team2_bowl_consistency_sum`, `team1_bat_form_sum`, `team1_bowl_form_sum`, `team2_bat_form_sum`, `team2_bowl_form_sum`.
- **Output** (1): `team1_win_probability` (0–1).

---

## 3. Aggregator: How Results Are Combined

### Match aggregates (backtest)

- **predRuns** = sum of player `runs` predictions.
- **predWickets** = sum of player `wickets` predictions.
- **predWinner** = team with higher sum of player runs (team totals from `playerTeams`).
- **predExtras** = extras model output if available; else historical average for format+venue.

### Player combinator (team prediction)

- **total_score** = `sum(runs_scored) × (team_size / len(pool)) + extras`.
- **target** = `sum(runs_conceded) × magic_number`.
- **batting_contribution** = runs_scored / total_score.
- **bowling_contribution** = runs_conceded / target.

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
3. Train models (batting, bowling, fielding, extras, win).
4. Run ml-service (load artifacts).
5. Optional: auto-tune, walk-forward, combination meta-model.
