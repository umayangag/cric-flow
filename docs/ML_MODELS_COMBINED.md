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

**Pipeline order:** Precompute (go-app) → export-dataset → train models → run (or restart) ML service.

Optional steps (separate from the core pipeline):

- **Auto-tune** — find best hyperparameters for a given cutoff/CSV; then update `ml.training.<model>` and re-train. See **docs/ML_AUTO_TUNE.md**.
- **Walk-forward** — evaluate models over time (train → predict next X matches → score → absorb, repeat); writes a registry for feedback and tuning X. See **docs/ML_WALK_FORWARD.md**. Run as a **separate step** (e.g. `make walk-forward`), not inside auto_tune.

| Model    | From repo root | From ml-service |
|----------|----------------|-----------------|
| Batting  | `make train-batting` | `make train-batting` |
| Bowling  | `make train-bowling` | `make train-bowling` |
| Fielding | `make train-fielding CUTOFF=<RFC3339>` or `FIELDING_CSV=<path>` | `make train-fielding` (set `GO_APP_URL`, `CUTOFF` or `FIELDING_CSV`) |
| All three | `make train-models` (fielding needs `CUTOFF` or `FIELDING_CSV`) | `make train-all` |
| Auto-tune | `make ml-auto-tune MODEL=batting FORMAT=T20` or `MODEL=all ALL_FORMATS=1` | `make auto-tune MODEL=... FORMAT=...` |
| Walk-forward | `make walk-forward INITIAL_CUTOFF=... WINDOW_X=50 WALK_FORMAT=T20 WALK_MODEL=batting` | `make walk-forward` (requires GO_APP_URL) |

- **Batting / Bowling**: Use exported CSVs (after `make export-dataset`). Or train-on-the-fly when no artifacts are loaded (ML service fetches from go-app at prediction time).
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

## Integration: Evaluate DB and upcoming match

- **Evaluate DB** (backtest evaluate, `GET /api/backtest/match?mode=evaluate&match_id=...` or evaluate-stream): Uses the same ML backtest endpoint with format + features. When **fielding artifacts** are loaded for that format, the ML service returns catches and run_outs per player; the go-app computes **player_catches_mae** and **player_run_outs_mae** and includes them in the response. When fielding artifacts are not loaded, ML returns 0 for catches/run_outs.
- **Upcoming match** (`POST /api/predict/team-selection`): Uses the same ML backtest endpoint. When the ML service returns non-zero catches or run_outs (fielding model loaded), those values are used for **FieldScore** in team selection. When the ML returns all zeros (no fielding model), the go-app falls back to **enrichFieldingFromHistory** (EWM of historical fielding involvements) so fielding still contributes to selection. Train and deploy fielding artifacts per format to use the ML fielding model in both flows.
