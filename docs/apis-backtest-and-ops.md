# APIs, backtest, evaluate, and ops

API contracts (Go and ML), backtest/evaluate on played matches, and the ops status dashboard.

---

## API contracts (summary)

**Design:** Explicit request/response models, consistent error schema (`code`, `message`, optional `details`), validation at boundaries. Prefer additive changes; version for breaking changes.

### ML service (FastAPI) — base `http://localhost:8000`

- **Content-Type:** `application/json`. **Error format:** `{ "error": { "code": "INVALID_INPUT", "message": "...", "details": {...} } }`
- **GET /health** — 200 `{ "status": "ok" }`
- **POST /predict/batting** — Body: BattingFeatures (consistency, form, temp, wind, rain, humidity, cloud, pressure, viscosity, inning, session, toss, venue, opposition, season, player_name, format). Response: runs_scored, balls_faced, fours_scored, sixes_scored, batting_position, strike_rate. Constraints: batting_consistency ≥ 0, batting_inning ∈ {1,2}, batting_session ∈ {1,2,3}, format optional.
- **POST /predict/bowling** — Body: BowlingFeatures (bowling_* names, batting_inning, bowling_session, toss, bowling_venue, bowling_opposition, season, player_name, format). Response: runs_conceded, deliveries, wickets_taken, econ.
- **POST /predict/extras** — Body: array of **ExtrasFeatures** (format_id, venue_id, season_id, temp, wind, rain, humidity, cloud, pressure, viscosity, bat_consistency_sum, bowl_consistency_sum, bat_form_sum, bowl_form_sum; optional `format` for per-format model). Response: array of `{ "total_extras": float }`. Uses the same unified feature set as extras training.
- **POST /predict/win** — Body: array of **WinFeatures** (format_id, venue_id, team1_opposition_id, team2_opposition_id, toss_winner_opposition_id, weather columns, team1/team2 bat/bowl consistency and form sums; optional `format`). Response: array of `{ "team1_win_probability": float }`. Uses the same unified feature set as win training.
- **POST /predict-win** (legacy) — Body: array of PlayerPrediction (no winning_probability). Response: array with winning_probability. Distinct from **POST /predict/win** above (match-level WinFeatures).

### Go API (mux) — base `http://localhost:8080`

- **GET /health** — 200 `{ "status": "ok" }`
- **GET /readiness** — 200 `{ "status": "ready" }`; 503 when DB unavailable
- **POST /precompute** — 202 `{ "status": "started" }`; optional body `{ "season", "formats" }`
- **GET /precompute/status** — 200 with running, started_at, finished_at, season, formats, phase (starting|form|venue|opposition|consistency|done), last_error
- **POST /import/cricsheet** — Body: `{ "dir", "placeholders_fielding" }`; 202 started
- **GET /players/{id}?season=...&format=...** — Player details (id, player_name, is_wicket_keeper, batting_consistency, bowling_consistency, etc.)

Backtest and ops endpoints are described in the sections below. Keep contracts in sync with `ml-service/app/main.py` Pydantic models and Go `internal/contracts`.

---

## Backtesting predictions on played matches

**Purpose:** Evaluate model accuracy on already-played matches with a strict training cutoff at the match date; predict only for players who actually played; compare predictions vs actuals with summary metrics.

**Prerequisites:** Go API (e.g. localhost:8080), ML service (e.g. localhost:8000). Env: `ML_SERVICE_URL`, optional `VITE_API_URL`, `VITE_ML_SERVICE_URL` for frontend.

### Endpoints

1. **Select played matches by filters**  
   `GET /api/backtest/match?format=T20&team1=IND&team2=AUS`  
   Response: `filters`, `candidates[]` (match_id, stable_id, date, venue, season, format, team1, team2, winner_team_code).

2. **Evaluate a specific played match**  
   `GET /api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate&match_id=111`  
   Or SSE: `GET /api/backtest/evaluate-stream?format=...&team1=...&team2=...&match_id=...`  
   Or job: `POST /api/backtest/evaluate-start` (body or query: format, team1, team2, match_id).  
   Match scorecard (actual): `GET /api/backtest/scorecard?match_id=...`

   Response includes: `filters` (with `model_mode`: `"latest"` or `"strict_temporal"`), `match`, `players[]` (player_id, predicted, actual, errors), `match_aggregates` (predicted, actual, errors), `metrics` (player_runs_mae, player_runs_rmse, player_runs_r2, player_wickets_mae, player_economy_mae, player_catches_mae, player_run_outs_mae, match_*_mae, winner_accuracy), `predicted_scorecard`.

   **Model temporal mode** (query or body: `use_latest_model=1`):
   - **Latest model** (default for UI): Uses the current model (artifacts or train-on-the-fly with "now" cutoff). Fast; good for QA and sanity checks. May include the match being evaluated in training.
   - **Strict cutoff**: Model trained only on data before match date. Unbiased temporal validation; may require per-match training when artifacts unavailable.

**Notes:** Features are always computed at match-date cutoff (no future leakage). Players list = those who actually played. Match aggregates: predicted runs/wickets = sum of player preds; predicted extras from historical average per format/venue (`db.GetAverageExtrasForFormat`); actuals from DB. Fielding metrics (player_catches_mae, player_run_outs_mae) when fielding artifacts are loaded.

### ML backtest endpoint (used by Go backend)

- **URL:** `POST $ML_SERVICE_URL/ml/backtest/predict`
- **Player mode:** Request: `cutoff_date`, `player_ids`, `format`, `features` (required), optional `use_latest_model` (default false). Response: `players[]` with player_id, runs, wickets, economy (and catches/run_outs when fielding loaded). When artifacts are loaded, ML uses them; otherwise **train-on-the-fly** (fetch training data from go-app `GET /api/backtest/training-data?cutoff=...&format=all`, train in memory, predict). `use_latest_model=true`: train with "now" as cutoff (one model per format). `use_latest_model=false`: train strictly before `cutoff_date`. Requires **GO_APP_URL** for train-on-the-fly.
- **Match aggregates mode:** Request: `cutoff_date`, `teams`. Response: `match` (runs, wickets, extras, winner_team_code).

### Curl examples

```bash
curl "http://localhost:8080/api/backtest/match?format=T20&team1=IND&team2=AUS" | jq .
curl "http://localhost:8080/api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate&match_id=111" | jq .
```

### E2E smoke

- **Seed fixtures:** `make seed-fixtures` (minimal IND vs AUS T20, one played match).
- **Smoke:** `make e2e-backtest-smoke` — verifies backtest match list and evaluate response (players, metrics, match_aggregates). Uses Docker Compose service names.

### Frontend

Evaluate DB tab: load candidates by filters, select match, choose **model temporal mode** (Latest model recommended vs Strict cutoff), run evaluation (job-based, polls status), show actual scorecard, progress, result (player errors, summary metrics, predicted scorecard). MAE shown to 3 decimal places; missing as '-'.

### Troubleshooting

- Go backend cannot reach ML: set `ML_SERVICE_URL`, ensure ML is running.
- Missing match_aggregates: verify ML client returns `match` and DB has winner/innings data.
- Frontend: set `VITE_API_URL` to Go API.

---

## Evaluate DB pipeline (flow and reference)

**Flow:** Frontend (Evaluate DB tab) → `GET /api/backtest/match` (select) → user selects match → optional `GET /api/backtest/scorecard?match_id=N` → `POST /api/backtest/evaluate-start` or `GET /api/backtest/evaluate-stream` (with optional `use_latest_model=1`) → go-app runs `doEvaluateWork` → progress/result → UI shows actual scorecard, evaluation result, predicted scorecard. **Features** are always computed at match-date cutoff. **Model temporal mode:** `use_latest_model=true` uses the latest model (may include match in training); `use_latest_model=false` (strict) trains only on data before match date.

**Go-app evaluate steps (`doEvaluateWork`):** match_date (cutoff) → squad (playing XI from batting_data ∪ bowling_data) → features (at cutoff via `ComputeFeaturesAtCutoffForMatch` or legacy provider) → ml_predict (format + features + use_latest_model to ML) → actuals (from DB) → metrics → aggregates (predicted = sum of player preds; actual from DB) → scorecard (build predicted scorecard from actual layout + ML preds) → done. Response includes `filters.model_mode` (`"latest"` or `"strict_temporal"`). All external deps behind seams in `backtest_seams.go` for testing.

**Feature computation at cutoff:** For matchID > 0: `GetMatchFeatureContext` (format_id, venue_id, season_id, opposition IDs) then per-player batting/bowling snapshots at cutoff (EWM, consistency, venue, opposition; only data with match_date < cutoff). Missing history → 0; weather 0 when not available. Same semantics as training export.

**ML service:** `POST /ml/backtest/predict` with format, features, and `use_latest_model`. Uses loaded artifacts for that format or train-on-the-fly (fetch `GET /api/backtest/training-data?cutoff=...&format=all`). When `use_latest_model=true`, train-on-the-fly uses "now" as cutoff. No deterministic baseline. Fielding: when fielding artifacts loaded, returns catches/run_outs; else 0.

**Go-app training-data API:** `GET /api/backtest/training-data?cutoff=...&format=...` (cutoff required; format=all or specific). Response: batting, bowling (headers + rows); only matches with match_date < cutoff. Used by ML train-on-the-fly.

**SSE stream:** `GET /api/backtest/evaluate-stream?format&team1&team2&match_id` (optional `use_latest_model=1`, `use_unified_model=1`). Events: `progress` (step, message), then `result` (BacktestEvaluateResponse) or `error` (message). Frontend: `backtestEvaluateStream()` in api.ts.

**Evaluate job:** `POST /api/backtest/evaluate-start` (body or query: format, team1, team2, match_id, optional use_latest_model, use_unified_model). Returns 202 `{ "job_id": "..." }`. Poll `GET /api/backtest/evaluate-status?job_id=...` for status, steps, and result. EvaluateDbTab uses this flow.

**Scorecards:** Actual: `GET /api/backtest/scorecard?match_id=N` (repo_scorecard.GetMatchScorecard). Predicted: built in go-app from actual layout + ML predictions, returned in evaluate response.

**Key files:** backtest_handlers.go (doEvaluateWork, stream, training-data handler), backtest_seams.go, backtest.go, repo_backtest_features.go, exportqueries/training_snapshot.go, ml_backtest_client.go, repo_scorecard.go; ml-service: main.py (backtest_predict), train_on_the_fly.py, backtest_service.py, models.py; configs/feature_vectors.json; frontend: api.ts, EvaluateDbTab.tsx, MatchScorecard.tsx.

**Debugging:** Predictions random → check format and features reach ML; train-on-the-fly → set GO_APP_URL, ensure data for cutoff. 503 TRAIN_ON_THE_FLY_FAILED → check GO_APP_URL, go-app data, ML logs. Features wrong → check GetMatchFeatureContext and ComputeFeaturesAtCutoffForMatch, match format_id/venue_id/season_id. Squad empty → match must have batting_data/bowling_data. SSE never finishes → ensure one result or error event. Scorecard missing → same match_id, DB rows for match/innings/batting/bowling.

---

## Ops status dashboard

**Endpoint:** `GET /ops/status` (no query params). Aggregates: services (API + ML health), DB (connectivity, migration, counts), precompute freshness per format, CSV exports (root, per-format files with exists/rows/modified), ML artifacts (per-format batting/bowling, exists/loaded/modified), fielding and weather data availability (row counts), and an ordered list of **suggestions** (actionable make commands to get the system ready).

**Response sections:** `timestamp`, `services` (api_health, api_readiness, ml_health), `db` (connected, migration status/current/expected, counts: players, matches, innings), `precompute` (last_run, as_of, formats with status ok/stale/missing), `exports` (root from GO_APP_OUTPUT_DIR when set, formats with files: name, exists, rows, modified), `artifacts` (root; probes ML `/artifacts/status` when available else filesystem scan; per-format batting/bowling exists, loaded, modified), `fielding` (available, rows), `weather` (available, rows), `pipeline` (steps with running, completed, runnable per step), `suggestions[]` (reason, commands). Precompute freshness: format in last run is ok if run finished same UTC day else stale; formats not in run = missing. Suggestions priority: DB → Precompute → Exports → Artifacts → Services. Example commands: DB not ready → `make migrate && make cricsheet-import`; precompute missing/stale → `make precompute-asof`; exports missing → `make export-dataset`; artifacts missing → `make ml-install && make train-batting`, `make train-bowling`; ML down → `make dev-up` or `make dev-rebuild`. Fallback artifacts root when ML down: GO_APP_ARTIFACTS_ROOT or output/ml-service.

**Pipeline modes:** Prefer **Train** when params are known (config + DB from previous auto-tune); use **Auto-tune** when discovering or re-optimizing hyperparameters. See **docs/ml-and-training.md** (§ Pipeline modes) for the single-train principle and Mode A (fast path) vs Mode B (tuning path).

**Quick verification:** `make dev-up` then `curl -s http://localhost:8080/health | jq`, `curl -s http://localhost:8080/readiness | jq`, `curl -s http://localhost:8080/ops/status | jq`. Frontend: Ops Status UI shows Fielding and Weather tiles (available, rows); suggestions per section card.

**Force gaps to test:** Remove artifacts → `rm -rf output/ml-service/*`; recheck artifacts and suggestions. Remove exports for a format → recheck exports and suggestions. After `make train-all`, recheck artifacts and suggestions.
