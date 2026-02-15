# Backtesting predictions on played matches

This document explains how to use the new backtest flow to evaluate model accuracy on already‑played matches. The flow enforces a strict training cutoff at the match date, predicts only for players who actually played, and compares predictions vs actuals with summary metrics.

## Prerequisites
- Go API service (go-app) running locally, default on `http://localhost:8080`.
- ML service (ml-service) running locally, default on `http://localhost:8000`.
- Environment variables:
  - `ML_SERVICE_URL` (for go-app → ml-service, default `http://localhost:8000`)
  - Frontend (optional): `VITE_API_URL`, `VITE_ML_SERVICE_URL`

## Endpoints

Backend provides a single endpoint with two modes:

1) Select played matches by filters
```
GET /api/backtest/match?format=T20&team1=IND&team2=AUS
```
Response (abridged):
```
{
  "filters": {"format":"T20","team1":"IND","team2":"AUS"},
  "candidates": [
    {
      "match_id": 111,
      "stable_id": "2024-10-30-IND-AUS",
      "date": "2024-10-30T14:00:00Z",
      "venue": "Wankhede Stadium",
      "season": "2024",
      "format": "T20",
      "team1": "IND",
      "team2": "AUS",
      "winner_team_code": "IND"
    }
  ]
}
```

2) Evaluate a specific played match (strict cutoff enforced)
```
GET /api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate&match_id=111
```

Alternative for the Evaluate DB tab (SSE progress + same result):  
`GET /api/backtest/evaluate-stream?format=...&team1=...&team2=...&match_id=...` — streams `progress` events then a single `result` or `error`.  
Match scorecard (actual): `GET /api/backtest/scorecard?match_id=...`
Response (abridged):
```
{
  "filters": {"format":"T20","team1":"IND","team2":"AUS","match_id":111},
  "match": {"match_id":111, "date":"2024-10-30T14:00:00Z"},
  "players": [
    {
      "player_id": 1,
      "predicted": {"runs": 25, "catches": 1, "run_outs": 2},
      "actual": {"runs": 30, "catches": 2, "run_outs": 1},
      "errors": {"runs_mae": 5, "catches_mae": 1, "run_outs_mae": 1}
    }
  ],
  "match_aggregates": {
    "predicted": {"runs": 160, "wickets": 6, "extras": 12, "winner_team_code": "IND"},
    "actual": {"runs": 150, "wickets": 7, "extras": 10, "winner_team_code": "IND"},
    "errors": {"runs_mae": 10, "wickets_mae": 1, "extras_mae": 2}
  },
  "metrics": {
    "player_runs_mae": 3.667,
    "player_runs_rmse": 4.123,
    "player_runs_r2": 0.891,
    "player_wickets_mae": 0.5,
    "player_economy_mae": 0.5,
    "player_catches_mae": 0.5,
    "player_run_outs_mae": 1.0,
    "match_runs_mae": 10,
    "match_wickets_mae": 1,
    "match_extras_mae": 2,
    "winner_accuracy": 1
  },
  "predicted_scorecard": { "match_id": 111, "match_date": "...", "venue": "...", "innings": [ ... ] }
}
```

Notes:
- Training data used by ML is restricted to rows before the match date (cutoff).
- Players list includes only those who actually played.
- Match aggregates section is present when both ML and DB seams are wired; otherwise it may be omitted.
- Totals mapping: numeric match totals are summed from the database table `match_inning` across all innings — `runs_scored` as total runs, `wickets_lost` as total wickets, and `extras` as total extras. The `target_runs` column is not used for backtest accuracy metrics.

### Metrics definitions (player-level runs)

- player_runs_mae: mean absolute error of predicted runs vs actual runs for players included in the evaluation.
- player_runs_rmse: root mean squared error over player runs.
- player_runs_r2: coefficient of determination for player runs predictions, computed as `1 - SS_res/SS_tot` over the evaluated players. If all actuals are equal (degenerate case), R² is reported as 0.

Other metrics remain as previously documented: `player_wickets_mae`, `player_economy_mae`, `match_runs_mae`, `match_wickets_mae`, `match_extras_mae`, and `winner_accuracy`.

Fielding metrics (when available):
- `player_catches_mae`: mean absolute error of predicted vs actual catches across evaluated players.
- `player_run_outs_mae`: mean absolute error of predicted vs actual run-outs across evaluated players.

### Full pipeline vs baseline (player predictions)

When the Go backend sends **format** and **features** (per-player feature map at cutoff) along with **player_ids**, the ML service may use **pre-trained batting/bowling models** for that format (full pipeline). When any of these is missing or no model is loaded for the format, it falls back to a **deterministic baseline** (RNG seeded by cutoff + player_id). See **docs/evaluate-db-pipeline.md** for step-by-step flow, feature computation, SSE stream, scorecards, and debugging.

### ML service endpoint (used by backend)

The Go backend calls a dedicated ML endpoint to obtain predictions with a strict cutoff.

- URL: `POST $ML_SERVICE_URL/ml/backtest/predict`
- Request/Response have two modes depending on the payload:

1) Player predictions mode

Request (minimal — baseline path)
```
{
  "cutoff_date": "2024-10-30T14:00:00Z",
  "player_ids": [1, 2, 3]
}
```

Request (full pipeline — when format and features are sent)
```
{
  "cutoff_date": "2024-10-30T14:00:00Z",
  "player_ids": [1, 2, 3],
  "format": "T20",
  "features": {
    "1": {"batting_consistency": 0.5, "batting_form": 20.0, "bowling_consistency": 0.3, ...},
    "2": { ... }
  }
}
```

Response
```
{
  "players": [
    {"player_id": 1, "runs": 25.0, "wickets": 1.0, "economy": 7.5},
    {"player_id": 2, "runs": 12.0}
  ]
}
```

2) Match aggregates mode

Request
```
{
  "cutoff_date": "2024-10-30T14:00:00Z",
  "teams": ["IND", "AUS"]
}
```

Response
```
{
  "match": {"runs": 160.0, "wickets": 6.0, "extras": 12.0, "winner_team_code": "IND"}
}
```

Implementation notes
- The ML service must honor the strict cutoff (train/aggregate only from data earlier than `cutoff_date`).
- When the backend sends `format` and `features`, the ML service uses loaded per-format models if available; otherwise it uses a deterministic baseline (no DB, no training).
- For full pipeline details (feature computation, SSE evaluate-stream, scorecards, debugging), see **docs/evaluate-db-pipeline.md**.

## Curl examples

```
curl "http://localhost:8080/api/backtest/match?format=T20&team1=IND&team2=AUS" | jq .

curl "http://localhost:8080/api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate&match_id=111" | jq .
```

## Fixtures + Smoke (E2E)

For a quick end‑to‑end verification on a tiny deterministic dataset, we provide a seed script and Make targets.

Prerequisites:
- Docker running (for `docker compose` services `postgres`, `go-api`, `ml-service`).
- `psql` CLI installed locally (used by the seed target).

Commands:
```
# Seed fixtures (creates minimal data: IND vs AUS T20, one played match)
make seed-fixtures

# Run the E2E smoke (select → evaluate with jq assertions)
make e2e-backtest-smoke
```

What it does:
- Seeds a single played T20 match (match_id 9000111) with `match` and `match_inning` rows (totals: runs_scored/wickets_lost/extras per inning), two teams (IND, AUS), winner, and a few player rows with `batting_data` and `bowling_data`.
- Starts Postgres, then `go-api` and `ml-service`.
- Verifies:
  - `GET /api/backtest/match?format=T20&team1=IND&team2=AUS` returns candidates.
  - `GET /api/backtest/match?format=T20&team1=IND&team2=AUS&mode=evaluate&match_id=9000111` returns `players`, `metrics.player_runs_mae`, and `match_aggregates.predicted|actual|errors`.

Notes:
- The ML service baseline is deterministic, so results are stable for the same cutoff and inputs.
- The smoke uses `docker compose` service names defined in `docker-compose.yml`.

## E2E smoke example (recorded)

Below is a representative response captured from a local run using the deterministic ML baseline. Values will be deterministic for the same inputs (cutoff, players, teams):

```
{
  "filters": {"format": "T20", "team1": "IND", "team2": "AUS", "match_id": 111},
  "match": {"match_id": 111, "date": "2024-10-30T14:00:00Z"},
  "players": [
    {"player_id": 1, "predicted": {"runs": 25}, "actual": {"runs": 30}, "errors": {"runs_mae": 5}},
    {"player_id": 2, "predicted": {"runs": 10}, "actual": {"runs": 10}, "errors": {"runs_mae": 0}}
  ],
  "match_aggregates": {
    "predicted": {"runs": 160, "wickets": 6, "extras": 12, "winner_team_code": "IND"},
    "actual": {"runs": 150, "wickets": 7, "extras": 10, "winner_team_code": "IND"},
    "errors": {"runs_mae": 10, "wickets_mae": 1, "extras_mae": 2}
  },
  "metrics": {
    "player_runs_mae": 2.5,
    "match_runs_mae": 10,
    "match_wickets_mae": 1,
    "match_extras_mae": 2,
    "winner_accuracy": 1
  }
}
```

Notes:
- The example above mirrors the response schema enforced by unit tests.
- Player targets and match totals may expand over time; the schema is additive.

## Troubleshooting

- If Go backend cannot reach ML service, ensure `ML_SERVICE_URL` is set (default: `http://localhost:8000`) and ML is running.
- If `match_aggregates` section is missing, verify:
  - ML client is enabled and returns a `match` object for the teams, and
  - DB has winner information in `match` (outcome_winner_opposition_id) or `match_inning` for the selected `match_id`.
- Python tests: If pytest is not installed in your environment, install dev tools or run tests via your CI that includes pytest.
- Frontend: Set `VITE_API_URL` to the Go API (default: `http://localhost:8080`).

## Frontend

The Evaluate DB tab now uses the new backtest API: it loads candidates by filters, lets you choose a match, runs the evaluation, and renders player errors and summary metrics.

UI notes:
- Player metrics: runs are always shown; when available, bowling metrics (wickets, economy) are displayed with per‑player absolute errors and summary MAE metrics.
- Match aggregates: when available, the UI shows predicted vs actual totals (runs, wickets, extras) and winner, along with absolute errors and summary metrics (match_*_mae, winner_accuracy).
- Numeric formatting: MAE metrics are shown up to three decimal places; missing values are displayed as '-'.
- The UI remains backward compatible if match aggregates or bowling metrics are absent.
