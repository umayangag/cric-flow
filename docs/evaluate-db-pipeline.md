# Evaluate DB tab: full pipeline and debugging reference

This document describes the **Evaluate DB** flow end-to-end: how the frontend, go-app, and ML service work together for “Evaluate Selected Match”, including feature computation, full vs baseline prediction, SSE progress, and scorecards. Use it to refine the process or debug issues.

---

## 1. High-level flow

```
Frontend (Evaluate DB tab)
  → GET /api/backtest/match (select mode) → list candidates
  → User selects match
  → GET /api/backtest/scorecard?match_id=N → actual match scorecard (optional)
  → GET /api/backtest/evaluate-stream?format&team1&team2&match_id=N (SSE)
       → go-app: doEvaluateWork (steps below)
       → SSE events: progress (step, message) then result or error
  → UI shows: actual scorecard, progress steps, evaluation result, predicted scorecard
```

**Key idea:** Training data is **strictly before the match date** (cutoff). No future or same-match data is used for features or ML.

---

## 2. Go-app evaluate pipeline (doEvaluateWork)

Location: `go-app/internal/server/backtest_handlers.go` — `doEvaluateWork()`.

Steps run in order (each can emit an SSE `progress` event when using the stream endpoint):

| Step ID       | Description |
|---------------|-------------|
| `match_date`  | Get match date from DB → used as **cutoff** for all later steps. |
| `squad`       | Get playing XI: distinct `player_id` from `batting_data` ∪ `bowling_data` for this `match_id` (no `player_match` table). |
| `features`    | **Feature computation at cutoff:** `getBacktestFeaturesAtCutoffFunc(ctx, cutoff, squad)` → `map[player_id]map[feature_name]float64`. Uses `db.DefaultFeatureProviderInst.GetPlayerFeaturesAtCutoff()` (see below). |
| `ml_predict`  | Call ML service with **cutoff, format, player_ids, and features**. If features are non-empty, ML may use the **full pipeline** (real models); otherwise **baseline** (deterministic RNG). |
| `actuals`     | Load actual match stats from DB: runs, wickets, economy, catches, run_outs per player (from `batting_data`, `bowling_data`, `fielding_data`). |
| `metrics`     | Compute per-player and summary metrics (MAE, RMSE, R², etc.) from predictions vs actuals. |
| `aggregates`  | Fetch match-level aggregates (predicted and actual: runs, wickets, extras, winner). |
| `scorecard`   | Build **predicted scorecard** from actual scorecard layout + ML predictions (runs, wickets, economy per player). |
| `done`        | Evaluation complete. |

**Seams (test hooks):** All external dependencies are behind functions in `go-app/internal/server/backtest_seams.go` (and wired in `backtest.go` init): `getBacktestMatchDateFunc`, `getBacktestSquadPlayerIDsFunc`, `getBacktestFeaturesAtCutoffFunc`, `mlBacktestPredictFunc`, `getBacktestPlayerActualsForMatchFunc`, etc. Tests override these.

---

## 3. Feature computation at cutoff (go-app)

**Provider:** `go-app/internal/db/repo_features_cutoff.go` — `DefaultFeatureProviderInst.GetPlayerFeaturesAtCutoff(ctx, cutoff, playerIDs)`.

**Contract:** Returns `map[int64]map[string]float64`: for each player, a map of feature name → value. Only data with `match_date <= cutoff` (or season ≤ cutoff year for precomputed form) is used.

**Features produced (examples):**

- From `player`: `batting_consistency`, `bowling_consistency`
- From `player_form_data` + `season` (season ≤ cutoff year): `batting_form`, `bowling_form`
- From `batting_data` + `match` (match_date ≤ cutoff): `avg_runs`; used as `batting_form` fallback
- From `bowling_data` + `match` (match_date ≤ cutoff): `avg_wickets`, `avg_economy`; used as `bowling_form` fallback

**Used by:** `doEvaluateWork` passes this map to `mlBacktestPredictFunc`, which sends it to the ML service when calling `POST /ml/backtest/predict` with `format` and `features`.

---

## 4. ML service: full pipeline vs baseline

**Endpoint:** `POST $ML_SERVICE_URL/ml/backtest/predict`

**Request body (player predictions):**

- `cutoff_date` (RFC3339), `player_ids` (required for player mode)
- **Optional:** `format` (e.g. `"T20"`, `"ODI"`) and `features` (`Dict[str, Dict[str, float]]`: key = `player_id` as string, value = feature name → value)

**Behavior:**

- **Full pipeline:** When **all** of `player_ids`, `format` (non-empty), and `features` (non-empty) are present **and** the ML service has loaded models for that format (`BAT_MODELS`, `BOWL_MODELS` in `ml-service/app/artifacts.py`):
  - For each player, build `BattingFeatures` and `BowlingFeatures` from `features` + defaults (see `ml-service/app/backtest_service.py`: `build_batting_features_from_map`, `build_bowling_features_from_map`).
  - Build feature vectors using `configs/feature_vectors.json` and run **loaded** batting/bowling models (scaler + model).
  - Return runs (from batting model), wickets and economy (from bowling model) per player. Catches/run_outs not from models; currently 0 in full pipeline.
- **Baseline (fallback):** If any of the above is missing (e.g. no `features`, or no model for `format`), the ML service uses `predict_players_baseline(cutoff, player_ids)`: deterministic pseudo-random numbers (RNG seeded by cutoff + player_id). No DB, no feature computation, no training.

**Important:** The full pipeline does **not** train on the fly; it uses **pre-trained artifacts** for the given format. Training is done offline (e.g. `train_batting` / `train_bowling`); artifacts must be present under the ML service’s models directory.

**Code references:**

- Request model: `ml-service/app/models.py` — `BacktestPredictRequest` (`format`, `features` optional).
- Full-pipeline path: `ml-service/app/main.py` — `_predict_players_with_features()`, and in `backtest_predict` when `use_full_pipeline` is true.
- Feature builders: `ml-service/app/backtest_service.py` — `build_batting_features_from_map`, `build_bowling_features_from_map` (defaults for missing keys: e.g. temp=25, venue/opposition=0.5, season=cutoff year).
- Baseline: `ml-service/app/backtest_service.py` — `predict_players_baseline()`.

---

## 5. Go-app → ML client

**File:** `go-app/internal/server/ml_backtest_client.go`

**Method:** `predictPlayers(ctx, cutoff, format, playerIDs, features)`

- When `features != nil` and non-empty, the request body includes `format` and `features` (player IDs as string keys).
- Response: `players[]` with `player_id`, `runs`, `wickets`, `economy`, `catches`, `run_outs` → mapped to `map[int64]playerPredictions` for use in `computePlayerResultsAndMetrics` and `buildPredictedScorecard`.

---

## 6. SSE stream endpoint (evaluate-stream)

**Endpoint:** `GET /api/backtest/evaluate-stream?format=...&team1=...&team2=...&match_id=...`

**Handler:** `go-app/internal/server/backtest_handlers.go` — stream handler that calls `doEvaluateWork` with a progress callback. For each step it sends:

- `event: progress`  
  `data: {"step":"<step_id>","message":"<human-readable>"}`

Then either:

- `event: result`  
  `data: <JSON BacktestEvaluateResponse>`
- or `event: error`  
  `data: {"message":"..."}`

**Frontend:** `frontend/src/api.ts` — `backtestEvaluateStream()` parses SSE (split by `\n\n`), handles `progress` / `result` / `error`, and calls `onProgress`, `onResult`, or `onError`. `EvaluateDbTab.tsx` uses this and shows a step list and a linear progress indicator while evaluating.

---

## 7. Scorecards

**Actual scorecard (before/after evaluate):**

- **API:** `GET /api/backtest/scorecard?match_id=N`
- **Backend:** `go-app/internal/db/repo_scorecard.go` — `GetMatchScorecard(ctx, matchID)`: match date, venue, innings with batting/bowling tables from `match_inning`, `batting_data`, `bowling_data`.
- **Frontend:** When user selects a match, `getMatchScorecard(matchId)` is called and the result is shown via `MatchScorecard` (title “Match summary”).

**Predicted scorecard (after evaluate):**

- Built in go-app: `buildPredictedScorecard(actualCard, preds)` in `backtest_handlers.go`. Uses the **actual** scorecard layout (innings, batting order, bowling order) and fills runs from batting predictions, wickets/economy (and derived runs) from bowling predictions. Returned in the evaluate response as `predicted_scorecard`.
- **Frontend:** Rendered with `MatchScorecard` (title “Predicted scorecard”, subtitle “ML prediction using only data before the match date…”).

---

## 8. Key files quick reference

| Area | File(s) |
|------|--------|
| Evaluate pipeline + SSE | `go-app/internal/server/backtest_handlers.go` (`doEvaluateWork`, stream handler) |
| Seams / test hooks | `go-app/internal/server/backtest_seams.go`, `go-app/internal/server/backtest.go` (init) |
| Features at cutoff | `go-app/internal/db/repo_features_cutoff.go` |
| ML client | `go-app/internal/server/ml_backtest_client.go` |
| Scorecard DB | `go-app/internal/db/repo_scorecard.go` |
| ML backtest predict + full pipeline | `ml-service/app/main.py` (`backtest_predict`, `_predict_players_with_features`) |
| ML feature builders | `ml-service/app/backtest_service.py` (`build_*_features_from_map`, `predict_players_baseline`) |
| ML request/response models | `ml-service/app/models.py` |
| Feature vector config | `configs/feature_vectors.json` |
| Frontend evaluate + SSE | `frontend/src/api.ts`, `frontend/src/components/EvaluateDbTab.tsx` |
| Frontend scorecard UI | `frontend/src/components/MatchScorecard.tsx` |

---

## 9. Debugging tips

- **Predictions look random / not changing with features:** Check that the ML service receives `format` and `features` (e.g. log request body in go-app or ML service). Ensure models for that format are loaded (e.g. artifacts under the ML service models dir; see `BAT_MODELS` / `BOWL_MODELS`).
- **“Model for format X not loaded”:** Train and export artifacts for that format; ensure the ML service loads them at startup (see `ml-service/app/artifacts.py` and config).
- **Features empty or wrong:** In go-app, confirm `getBacktestFeaturesAtCutoffFunc` is the DB provider and that `match_date` and `player`/`player_form_data`/`batting_data`/`bowling_data` exist for the cutoff. Check `repo_features_cutoff.go` queries and filters (`<= cutoff`, season ≤ cutoff year).
- **Squad empty:** Match must have rows in `batting_data` or `bowling_data` for that `match_id`; squad is the union of those `player_id`s.
- **SSE never finishes:** Ensure the stream handler sends exactly one `result` or `error` event; check for panics or early returns in `doEvaluateWork` or in the handler.
- **Scorecard missing:** Verify `GetMatchScorecard` and the scorecard API are called with the same `match_id`; check DB for `match`, `match_inning`, `batting_data`, `bowling_data` for that match.

---

## 10. Extending or refining the pipeline

- **Add features:** Extend `GetPlayerFeaturesAtCutoff` (and any precomputed tables it uses) and ensure the ML service’s `build_*_features_from_map` and `feature_vectors.json` include the new names and defaults.
- **Use sequential / window features:** If the project has sequential or window features (e.g. from precompute), they can be computed at cutoff in go-app and passed in `features`, or the ML service could accept a separate payload; the same “strictly before cutoff” rule applies.
- **Train at cutoff:** Current design uses **pre-trained** models. Training at cutoff (e.g. train on all data &lt; cutoff then predict) would require a new ML endpoint or long-running job and likely async + progress reporting; it is not implemented in the current full pipeline.
- **Caching:** Full-pipeline responses are not cached in the ML service when `features` are sent (by design). Baseline path can still use the existing backtest cache keyed by cutoff + player_ids.
