# Accuracy Trend Dashboard & API

This guide explains how to use the Accuracy Trend API and the accompanying frontend dashboard to visualize how prediction accuracy evolves from earlier matches to more recent matches.

## API: GET /api/backtest/accuracy-trend

Query parameters (all optional unless noted):
- `format`: Match format code, e.g., `T20`, `ODI`, `TEST`.
- `start_date`: Inclusive start date `YYYY-MM-DD`.
- `end_date`: Inclusive end date `YYYY-MM-DD`.
- `team1`: Team code/name (as stored in DB), e.g., `IND`.
- `team2`: Team code/name, e.g., `AUS`.
- `order`: `asc` (default) or `desc` by match date.
- `limit`: Safety cap on number of matches evaluated (default `100`, max `500`).
- `cache`: `off|read|readwrite` (default `readwrite`). Controls use of cached match-level aggregate predictions:
  - `off`: Always compute via ML; never read/write cache.
  - `read`: Use cache when present; compute on miss but do not write.
  - `readwrite`: Use cache when present; compute and upsert when missing.
- `metrics`: Optional subset in `player` and/or `team` (comma-separated). Defaults to both when omitted or invalid.

Response shape (excerpt):
```
{
  "filters": { "format": "T20", "team1": "IND", "team2": "AUS", "order": "asc", "limit": 50, "cache": "readwrite" },
  "count": 12,
  "results": [
    {
      "match_id": 123,
      "date": "2024-11-03T14:00:00Z",
      "format": "T20",
      "team1": "IND",
      "team2": "AUS",
      "metrics": {
        "player_runs_mae": 3.67,
        "team_runs_mae": 5.0,
        "team_winner_accuracy": 1
      }
    }
  ],
  "summary": {
    "player_runs_mae_avg": 2.91,
    "team_runs_mae_avg": 5.0,
    "team_winner_accuracy_avg": 0.58,
    "n": 12
  },
  "progressive": [
    {"n": 1,  "player_runs_mae_avg": 4.10, "team_runs_mae_avg": 6.0,  "team_winner_accuracy_avg": 0.0},
    {"n": 12, "player_runs_mae_avg": 2.91, "team_runs_mae_avg": 5.0,  "team_winner_accuracy_avg": 0.58}
  ]
}
```

Notes:
- Player metrics (e.g., `player_runs_mae`) are computed from XI predictions vs actuals.
- Team aggregates may use cached predictions depending on `cache` mode; actuals always come from DB.
- The order affects the `progressive` cumulative averages.

Examples:
```
curl -s "http://localhost:8080/api/backtest/accuracy-trend?format=T20&team1=IND&team2=AUS&start_date=2024-10-01&end_date=2024-12-31&order=asc&limit=25&cache=readwrite" | jq '.'

curl -s "http://localhost:8080/api/backtest/accuracy-trend?format=T20&metrics=team&order=asc&limit=100" | jq '.summary'
```

## Frontend Dashboard

Route: `/dashboard/accuracy-trend`

The dashboard renders:
- A line chart of the `progressive` series (player MAE, team MAE, winner accuracy).
- A table of per‑match metrics.

Controls:
- Format, start/end dates, team1, team2
- Order, limit, cache mode
- Metrics toggles for `player` and `team`

Behavior:
- Filters are reflected in the URL query string and automatically refetch data.
- Default `limit` is 100; maximum 500.
- `metrics` can be used to skip expensive computations server-side.

## Development

Backend quality gates:
```
make fmt-check && make lint && make test
```

Frontend (if using Next.js):
```
cd frontend
npm install
npm run dev
# then open http://localhost:3000/dashboard/accuracy-trend
```

If the API base differs across environments, configure `NEXT_PUBLIC_API_BASE` to point the dashboard at the desired backend.
