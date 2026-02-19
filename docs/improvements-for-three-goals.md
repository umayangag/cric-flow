# Five Improvements Toward the Three System Goals

This document suggests **5 improvements** that move the system closer to:

1. **Goal 1:** Select 11 players from a pool to **maximize overall team performance**, with ≥1 wicket keeper and ≥5 bowlers (including part-time).
2. **Goal 2:** **Predict individual player performance** with maximum accuracy for an upcoming match (inputs: match date, venue, opposition player pool, optional weather).
3. **Goal 3:** **Predict the scorecard summary** of the match from predicted player performance and the chosen team combination.

---

## 1. Replace Greedy Selection with Combinatorial Optimization (Goal 1)

**Current state:**  
`teamselect.Select()` sorts the pool by a fixed weighted score (bat 0.45, bowl 0.40, field 0.10, keeper 0.02), fills the XI greedily, then swaps to satisfy keeper and bowler constraints. This does **not** maximize “overall team performance”; it maximizes a weighted sum of normalized individual scores, which is a proxy.

**Improvement:**  
- **Option A (recommended):** Implement **constrained optimization** over the chosen XI:
  - **Objective:** Maximize expected team outcome, e.g. sum of predicted runs (batting) + value of wickets and economy (bowling) + fielding contribution, possibly combined with a win-probability term if the win model is used.
  - **Constraints:** Team size = 11, ≥1 wicket keeper, ≥5 bowlers (treat part-time bowlers as bowlers when `IsBowler` or similar is set).
  - **Method:** Integer linear program (ILP) or a small search (e.g. over valid XIs) using the same `ScorePlayer` or a **meta-model score** as the objective. Libraries: Go (e.g. `gonum` for linear algebra + simple branch-and-bound) or call out to a small Python/OR-tool script.
- **Option B (lighter touch):** Keep the current greedy + swap logic but **use learned weights** by default: ensure `selection.meta_model_path` is populated from `train_combination_meta` (see §5) so the scalar score reflects actual contribution rather than fixed weights.

**Files to touch:**  
- `go-app/internal/services/teamselect/select.go` (new optimizer or optional “optimize” mode).
- `go-app/internal/config/config.go` (EffectiveScoreWeightsForFormat already supports meta-model; ensure it’s used in `predictteam` and selection paths).
- Optional: new package `go-app/internal/services/teamselect/optimize.go` for ILP/search.

---

## 2. Improve Individual Prediction Accuracy (Goal 2)

**Current state:**  
- Features include form, consistency, venue, opposition, season, and weather (temp, wind, rain, humidity, cloud, pressure, viscosity).  
- **Future-match prediction** uses `ComputeFeaturesAtCutoffForFutureMatch`: venue, opposition, season, and optional `WeatherOverride` are set; **sequence features** (e.g. `bat_prev_sr`, `bat_window_sr_12_pp`, `bowl_window_econ_24_death`) are **not** in the future-match feature map (only form/momentum/consistency/venue/opposition/weather).  
- Training data (batting/bowling export) **does** join `weather_data`; if that table is sparse, models see mostly 0 for weather.  
- `OppositionPlayerIDs` is accepted in the team-selection API but not yet used to derive features (e.g. opposition strength or matchup).

**Improvements:**

1. **Align prediction-time features with training**  
   - Either: **Add sequence features** to `ComputeFeaturesAtCutoffForFutureMatch` (from precomputed/store or 0 when no history), so the vector matches `configs/feature_vectors.json` and training.  
   - Or: Define a **reduced feature set** for “future match” and train a dedicated model (or use the same model with 0 for missing sequence features and ensure training uses the same fill).  
   - **Single source of truth:** Ensure Go export (training-data API and CSV export) and ML (`feature_vectors.json`, `train_on_the_fly`, batch training) use the same column order and names everywhere.

2. **Weather end-to-end**  
   - **Training:** Keep/use `weather_data` in export; if the table is empty, consider placeholders or synthetic weather so the model sees non-zero variance (or document that weather is prediction-only until data exists).  
   - **Prediction:** Already supports optional `Weather` in `POST /api/predict/team-selection`; document that forecast can be passed here for best accuracy.

3. **Use opposition player pool**  
   - Use `OppositionPlayerIDs` (and optionally opposition roster from DB) to compute **opposition strength** or **matchup** features (e.g. average opposition bowling strength, key bowler indices) and add them to the feature map in `ComputeFeaturesAtCutoffForFutureMatch` and to the training snapshot so training and prediction stay aligned.

**Files to touch:**  
- `go-app/internal/db/exportqueries/training_snapshot.go` (`ComputeFeaturesAtCutoffForFutureMatch`: add sequence features or a documented reduced set; optional opposition-strength features).  
- `go-app/internal/features` or export queries: expose sequence features “as of” cutoff for a given player/format when no match context (e.g. last N innings).  
- `configs/feature_vectors.json` and ML `dataset_definitions.py` / `train_on_the_fly.py`: keep one contract; ML should not expect columns that prediction never fills.  
- Optional: new helper to compute opposition strength from `OppositionPlayerIDs` and feed into feature map.

---

## 3. Predicted Scorecard Summary for Upcoming Matches (Goal 3)

**Current state:**  
- **Backtest evaluate** builds a **predicted scorecard** from the **actual** match layout (innings, batting order, bowling order) and fills runs/wickets/economy from ML predictions; it also computes match aggregates and winner.  
- **Team selection** (`POST /api/predict/team-selection`) returns the **selected XI and per-player predictions** for both teams but **no** scorecard summary (no innings totals, no predicted winner, no single “match summary”).

**Improvement:**  
- Add a **predicted scorecard summary** for the **upcoming match** flow:
  - **Input:** Same as team-selection (or take the team-selection result): format, team1, team2, venue, match date, optional weather, optional extras.
  - **Logic:**  
    1. Get selected XI + player predictions for both teams (reuse `predictteam.PredictTeams` or its outputs).  
    2. **Innings 1:** Team A bats, Team B bowls → batting runs = sum of Team A batting predictions; bowling wickets/runs = from Team B bowling predictions; total = runs + (extras from model or default).  
    3. **Innings 2:** Team B bats, Team A bowls → same.  
    4. **Winner:** Compare innings totals (or use win model if available with selected XIs).  
  - **Output:** JSON with e.g. `innings1_total`, `innings2_total`, `predicted_winner`, `extras_innings1/2`, and optionally per-player predicted stats already returned by team-selection.

- **Implementation options:**  
  - **A:** Extend `POST /api/predict/team-selection` response with a `scorecard_summary` object.  
  - **B:** New endpoint e.g. `POST /api/predict/scorecard-summary` that accepts the same body as team-selection and returns selection + scorecard summary (avoids breaking existing clients).

**Files to touch:**  
- `go-app/internal/services/predictteam/predict_team.go`: add a function that, given two selected XIs and their predictions, plus optional extras, returns innings totals and winner.  
- `go-app/internal/server/predict_handlers.go`: either extend team-selection response or add scorecard-summary handler.  
- Reuse `buildPredictedScorecard`-style aggregation (sum runs, wickets, economy → runs) without needing an “actual” scorecard layout; you’re building a **summary** from two XIs only.

---

## 4. Data Processing and Feature Pipeline Consistency (Goals 1–3)

**Current state:**  
- Export and training-data API build rows from DB with as-of-date features; precompute fills form, consistency, venue, opposition, etc.  
- `configs/feature_vectors.json` defines batting/bowling/fielding feature lists; ML uses these (or a subset) for training and prediction.  
- There are two “flavors”: unified export (with `bat_*_asof`, format_code, etc.) and legacy; and `train_on_the_fly` uses a fixed list that must match the training-data API headers.  
- Weather: present in export (via `weather_data` join) and in future-match prediction via `WeatherOverride`; not always populated in DB.

**Improvements:**

1. **Single feature contract**  
   - Treat `configs/feature_vectors.json` (or one canonical list in Go) as the **single source of truth** for “what we send to the model.”  
   - Ensure:  
     - Go training-data API and CSV export emit columns in that order (with consistent names, e.g. `batting_temp` vs `temp` aligned with ML).  
     - `ComputeFeaturesAtCutoffForFutureMatch` and any backtest feature build produce exactly that set (or a documented subset with same fill rules).  
   - Update `ml-service/ml/dataset_definitions.py` and `train_on_the_fly.py` to read or mirror that contract so training and prediction never diverge.

2. **Weather ingestion**  
   - Document how `weather_data` is populated (e.g. CricSheet import with placeholders, or a separate job from an external API).  
   - If real weather is scarce, add a small script or pipeline step to backfill placeholders (e.g. seasonal averages by venue) so training has non-zero weather variance where needed.

3. **Sequence features at prediction**  
   - Precompute (or compute on demand) **sequence features** (e.g. `bat_prev_sr`, `bat_window_sr_12_pp`, bowling equivalents) “as of” a cutoff for each player/format, and feed them into `ComputeFeaturesAtCutoffForFutureMatch` so prediction uses the same features as training.  
   - If that’s costly, maintain a **reduced model** trained only on features available at prediction time (no sequence columns), and use it for future-match prediction.

**Files to touch:**  
- `configs/feature_vectors.json` and Go export queries / `training_snapshot.go`: align names and order.  
- `go-app/internal/db/exportqueries/training_snapshot.go`: ensure snapshot includes sequence features when available; document when they’re 0.  
- `ml-service/app/feature_config.py` and `ml-service/ml/dataset_definitions.py`: load from or stay in sync with the same contract.  
- Optional: `go-app/internal/weather` or import path to backfill `weather_data`.

---

## 5. Automate Pipeline and Meta-Model (Goals 1–3)

**Current state:**  
- Pipeline: import → precompute → export → train_batting, train_bowling, train_fielding, train_extras, train_win → optional auto_tune.  
- Combination meta-model: `train_combination_meta` produces JSON weights from a CSV of backtest contributions; go-app uses it when `selection.meta_model_path` is set.  
- Walk-forward exists but is a separate manual step; no automatic “retrain and evaluate” that updates artifacts or weights.

**Improvements:**

1. **Automate combination meta-model**  
   - After backtest evaluate (or a batch evaluate job), **produce** the contributions CSV (bat_score, bowl_score, field_score, is_keeper, format, target) from evaluate results.  
   - Run `train_combination_meta` (e.g. via pipeline step or Make) and write `combination_meta.json` to a known path.  
   - Set or suggest `selection.meta_model_path` in config so team selection uses learned weights by default (improves Goal 1 without changing the selection algorithm).

2. **Pipeline: optional walk-forward and promote**  
   - Add an optional pipeline step “walk_forward” (or “evaluate”) that runs walk-forward evaluation for one or more formats and stores metrics (e.g. MAE, winner accuracy) in a registry or DB.  
   - Optionally: “promote” artifacts (e.g. copy to a “stable” dir or tag) only if metrics exceed a threshold, so production always uses the last good model.

3. **One-command retrain and evaluate**  
   - Provide a single script or Make target that: runs import (if needed) → precompute → export → train all models → (optional) walk-forward or fixed holdout evaluation → report MAE/winner accuracy.  
   - This streamlines “data → features → models → accuracy” and makes it easy to iterate on features or models.

**Files to touch:**  
- `go-app/internal/server/backtest_handlers.go` or evaluate job: export backtest contributions in the format expected by `train_combination_meta`.  
- `go-app/internal/server/ops_status_pipeline.go` and `pipeline_handlers.go`: add step for “train_combination_meta” (call ML or run Make) and optionally “evaluate”/“walk_forward”.  
- `ml-service/ml/train_combination_meta.py`: accept input from URL or path produced by go-app.  
- Makefile / docs: document “full retrain + evaluate” and `meta_model_path` setup.

---

## Summary Table

| # | Improvement | Primary goal | Effort |
|---|-------------|---------------|--------|
| 1 | Combinatorial team selection (or meta-model by default) | Goal 1 | Medium–High |
| 2 | Feature alignment, weather, opposition features for accuracy | Goal 2 | Medium |
| 3 | Predicted scorecard summary for upcoming match | Goal 3 | Low–Medium |
| 4 | Single feature contract, weather ingestion, sequence at prediction | Goals 1–3 | Medium |
| 5 | Automate meta-model, walk-forward, one-command retrain+evaluate | Goals 1–3 | Low–Medium |

Implementing **3** and **5** (scorecard summary + automation) gives immediate value for Goals 2 and 3 with limited risk. **2** and **4** directly improve prediction accuracy (Goal 2). **1** makes team selection truly “maximize performance” (Goal 1).
