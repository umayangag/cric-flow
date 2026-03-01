# Phase 3: Share-Based Player Models (Future Enhancement)

Phase 1 and 2 implement the Hybrid approach: an innings-level model predicts `(innings_runs, innings_wickets)` per innings, and player predictions are rescaled proportionally so totals match. This achieves consistency: sum(batsman runs) = sum(bowler runs_conceded) = innings_runs.

**Phase 3** would provide consistency *by construction* by training player models to predict **shares** instead of absolute values:

## 3.1 Training Target Change

- **Batting**: Change target from `runs` to `runs_share = runs / innings_runs` (0–1)
- **Bowling**: Change targets from `runs_conceded`, `wickets` to `runs_share`, `wickets_share`

### Export Changes

1. **Batting export** (`batting.go`): Add `innings_runs` column via join:
   ```sql
   (SELECT SUM(runs) FROM batting_data b2 WHERE b2.match_id = bd.match_id AND b2.inning_number = bd.inning_number) AS innings_runs
   ```

2. **Bowling export** (`bowling.go`): Add `innings_runs` and `innings_wickets`:
   ```sql
   (SELECT SUM(runs) FROM batting_data b2 WHERE b2.match_id = b.match_id AND b2.inning_number = b.inning_number) AS innings_runs,
   (SELECT SUM(wickets) FROM bowling_data bw2 WHERE bw2.match_id = b.match_id AND bw2.inning_number = b.inning_number) AS innings_wickets
   ```

### Training Script Changes

- `train_batting.py`: Add `--share-targets` flag; when set, use `runs_share` as target (replace `runs`)
- `train_bowling.py`: Add `--share-targets`; use `runs_share`, `wickets_share` as targets
- Handle `innings_runs = 0` (skip or cap share)

## 3.2 Prediction Flow

1. Predict innings model first → `(innings1_runs, innings1_wickets)`, `(innings2_runs, innings2_wickets)`
2. Predict player shares from batting/bowling share models
3. `player_runs = player_runs_share * innings_runs` (guarantees consistency)

### Artifacts

- Share models: `batting_share_model_<FMT>.joblib`, `bowling_share_model_<FMT>.joblib` (or same path with different config)
- Config flag: `ml.use_share_models` to switch between absolute and share-based prediction

## Benefits

- Consistency by construction (no rescaling step)
- Potentially better generalization (model learns relative contribution)
- Same innings model and reconciliation infrastructure can remain as fallback
