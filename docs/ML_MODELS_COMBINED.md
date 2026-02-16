# Combined ML models for prediction

The system uses **multiple models** whose outputs are combined for the final prediction:

| Model | Level | Outputs | Used in |
|-------|--------|---------|--------|
| **Batting** | Player | runs, balls, fours, sixes, batting_position, strike_rate | Backtest player preds, team selection score |
| **Bowling** | Player | runs, deliveries, wickets, economy | Backtest player preds, team selection score |
| **Fielding** | Player | catches, run_outs (stumpings in training) | Backtest player preds, team selection score |
| **Extras** | Match | total extras per match | Match aggregates (when model loaded); else historical average |
| **Win** | Match | winner / team1_wins | Match outcome (when model loaded) |
| **Team combination** | — | — | **Not a separate model.** Final team selection uses batting + bowling + fielding scores and constraints (min bowlers, keeper). Optional: match-level extras and win models improve aggregate and outcome prediction. |

## Training data (go-app)

- **GET /api/backtest/training-data?cutoff=...&format=all** returns:
  - `batting`: headers + rows (player-level)
  - `bowling`: headers + rows (player-level)
  - `fielding`: headers + rows (player-level; fielding_form, fielding_consistency at cutoff)
  - `extras`: headers + rows (match-level; format_id, venue_id, season_id, total_extras)
  - `win`: headers + rows (match-level; format_id, venue_id, team1/2_opposition_id, toss_winner, team1_wins)

## Training (ml-service)

- **Batting / Bowling**: `make train-all` or train-on-the-fly when no artifacts are loaded.
- **Fielding**: `python -m ml.train_fielding --cutoff <RFC3339>` (requires `GO_APP_URL`) or `--csv <path>`.
- **Extras / Win**: Training scripts can be added (e.g. `train_extras.py`, `train_win.py`) that read from the same API and save `extras_model_<FMT>.joblib`, `win_model_<FMT>.joblib`. Config: `ml.training.extras`, `ml.training.win`.

## Config (per-model parameters)

In `ml-service/config.json`, `ml.training` has one block per model so you can tune each independently:

- `batting`, `bowling`, `fielding`, `extras`, `win`: each with `n_estimators`, `max_depth`, `random_state`, `joblib_compress`.

## Artifacts

- Batting: `batting_scaler_<FMT>.joblib`, `batting_model_<FMT>.joblib`
- Bowling: `bowling_scaler_<FMT>.joblib`, `bowling_model_<FMT>.joblib`
- Fielding: `fielding_scaler_<FMT>.joblib`, `fielding_model_<FMT>.joblib`
- Extras: `extras_model_<FMT>.joblib` (optional)
- Win: `win_model_<FMT>.joblib` (optional)

## Combined prediction flow

1. **Player predictions**: For each player, batting and bowling models (and fielding when loaded) produce runs, wickets, economy, catches, run_outs. Go-app sends feature maps; ML returns per-player preds.
2. **Match aggregates**: Predicted runs/wickets = sum of player preds; predicted extras = from extras model if loaded, else `db.GetAverageExtrasForFormat`.
3. **Winner**: From win model if loaded; else derived from predicted team totals (compare sum of runs).
4. **Team selection**: Greedy selection using batting + bowling + fielding scores and constraints; no separate “team combination” model—combination is the use of all of the above together.
