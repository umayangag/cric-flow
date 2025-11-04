# API Contracts and Validation (Draft)

This document defines the API contracts for both services. Goal: modular, human-readable, maintainable schemas with strict validation and helpful errors.

Design principles:
- Explicit request/response models with examples
- Consistent error schema with codes and messages
- Backwards-compatible evolution (additive changes; versioning if breaking)
- Validation at boundaries (types, ranges, enums)

## ML Service (FastAPI)

Base: `http://localhost:8000`

### Common
- Content-Type: `application/json`
- Error format:
```json
{
  "error": {
    "code": "INVALID_INPUT",
    "message": "batting_consistency must be >= 0",
    "details": {"field": "batting_consistency"}
  }
}
```

### GET /health
- 200 OK: `{ "status": "ok" }`

### POST /predict/batting
Request body (`BattingFeatures`):
```json
{
  "batting_consistency": 0.75,
  "batting_form": 0.62,
  "batting_temp": 28,
  "batting_wind": 8,
  "batting_rain": 0,
  "batting_humidity": 60,
  "batting_cloud": 20,
  "batting_pressure": 1008,
  "batting_viscosity": 1,
  "batting_inning": 1,
  "batting_session": 2,
  "toss": 1,
  "venue": 12.0,
  "opposition": 5.0,
  "season": 2019,
  "player_name": "Player A",
  "format": "T20"
}
```
Constraints:
- `batting_consistency >= 0`
- `batting_inning in {1,2}`
- `batting_session in {1,2,3}`
- `format in {"ODI","T20","TEST"} (optional)`

Response (`BattingPrediction`):
```json
{
  "runs_scored": 35.7,
  "balls_faced": 25.2,
  "fours_scored": 4.1,
  "sixes_scored": 1.2,
  "batting_position": 3.0,
  "strike_rate": 142.0
}
```

### POST /predict/bowling
Request body (`BowlingFeatures`):
```json
{
  "bowling_consistency": 0.7,
  "bowling_form": 0.55,
  "bowling_temp": 28,
  "bowling_wind": 8,
  "bowling_rain": 0,
  "bowling_humidity": 60,
  "bowling_cloud": 20,
  "bowling_pressure": 1008,
  "bowling_viscosity": 1,
  "batting_inning": 1,
  "bowling_session": 2,
  "toss": 0,
  "bowling_venue": 12.0,
  "bowling_opposition": 5.0,
  "season": 2019,
  "player_name": "Bowler B",
  "format": "T20"
}
```
Constraints:
- `bowling_consistency >= 0`
- `batting_inning in {1,2}`
- `bowling_session in {1,2,3}`

Response (`BowlingPrediction`):
```json
{
  "runs_conceded": 28.3,
  "deliveries": 24.0,
  "wickets_taken": 1.4,
  "econ": 7.1
}
```

### POST /predict-win
Request body (array of `PlayerPrediction` minus `winning_probability`):
```json
[
  {
    "player_name": "Player A",
    "runs_scored": 35.7,
    "balls_faced": 25.2,
    "fours_scored": 4.1,
    "sixes_scored": 1.2,
    "batting_position": 3.0,
    "strike_rate": 142.0,
    "runs_conceded": 0,
    "deliveries": 0,
    "wickets_taken": 0,
    "econ": 0
  }
]
```
Response (current implementation):
```json
[
  {
    "player_name": "Player A",
    "runs_scored": 35.7,
    "balls_faced": 25.2,
    "fours_scored": 4.1,
    "sixes_scored": 1.2,
    "batting_position": 3.0,
    "strike_rate": 142.0,
    "runs_conceded": 0,
    "deliveries": 0,
    "wickets_taken": 0,
    "econ": 0,
    "winning_probability": 0.62
  }
]
```
Notes:
- The service returns a per-player list enriched with `winning_probability` only. The team-level average can be computed client-side as the mean of `winning_probability`.
- If you need a response wrapper with `team_win_probability`, consider adding it at the client layer or extend the service in a backward-compatible way.

## Go API (mux)

Base: `http://localhost:8080`

### GET /health
- 200 OK: `{ "status": "ok" }`

### GET /readiness
- 200 OK: `{ "status": "ready" }`
- 503 when DB unavailable: `{ "status": "db_unavailable", "error": "..." }`

### POST /precompute
- 202 Accepted: `{ "status": "started" }`
- Triggers internal precompute in go-app asynchronously (no ML dependency)
- Optional JSON body to filter scope:
```json
{ "season": "2019", "formats": ["ODI", "T20I"] }
```

### GET /precompute/status
- 200 OK: returns the in-memory status of the last run (resets on process restart)
```json
{
  "running": true,
  "started_at": "2025-11-04T16:40:00Z",
  "finished_at": "",
  "season": "2019",
  "formats": ["ODI","T20I"],
  "phase": "venue",
  "last_error": ""
}
```
- Notes:
  - `phase` is one of: `starting`, `form`, `venue`, `opposition`, `consistency`, `done`.

### POST /import/cricsheet
Body:
```json
{ "dir": "../data", "placeholders_weather": true, "placeholders_fielding": true }
```
- 202 Accepted: `{ "status": "started" }`

### GET /players/{id}?season=2019&format=T20
Response (excerpt based on current handler):
```json
{
  "id": 123,
  "player_name": "Player A",
  "is_wicket_keeper": 0,
  "is_retired": 0,
  "batting_consistency": 12.34,
  "bowling_consistency": 8.9
}
```

## Validation & Errors
- Prefer numeric types for metrics; bound checks where applicable
- Enumerations: `format` => `ODI|T20|TEST`
- Always return the error envelope with `code`, `message`, and optional `details`

## Compatibility
- Add new optional fields; avoid removing/renaming without a version bump
- If breaking change required, introduce `/v2` routes while keeping `/v1`

---

Status: Draft. To be kept in sync with `ml-service/app/main.py` Pydantic models and Go `internal/contracts`.
