# Algorithms Implementation Map (Initial Draft)

Purpose: Map each baseline algorithm from the prototype to concrete implementation locations in `go-app` and `ml-service`, enabling side-by-side validation and targeted fixes. Keep it modular and human-maintainable.

Sources reviewed
- Prototype: `src/team_selection/select_pool.py`, `src/team_selection/shared/*`, `src/team_selection/create_final_dataset.py`, `src/final_data/*`
- Implementation (Go): `go-app/cmd/export-dataset`, `go-app/internal/features/*`, `go-app/internal/db/*`
- Implementation (ML): `ml-service/ml/*.py`, `ml-service/app/main.py`

---

## Feature Encodings & Inputs

- Session encoding (1..3)
  - Prototype: `encode_session(session)` used in `select_pool.py`
  - Implementation: SQL CASE in `go-app/cmd/export-dataset` queries for batting/bowling format exports
  - Validation: Compare exporter output column `*_session` values against prototype encoding table

- Viscosity encoding (dry→0, humid→1, NULL→0)
  - Prototype: `encode_viscosity(viscosity)` in `select_pool.py`
  - Implementation: SQL CASE/COALESCE in exporter queries (both batting and bowling paths)
  - Validation: Ensure no NULLs; validator must assert only {0,1}

- Toss encoding (0|1)
  - Prototype: encoded numeric (used as `toss` input)
  - Implementation: SQL CASE in exporter maps `match_details.toss` text to numeric
  - Validation: Confirm mapping parity with prototype assumptions (bat-first vs field-first)

- Venue/opposition aggregates (numeric features)
  - Prototype: `get_player_metric(..., dim='venue' | 'opposition')`
  - Implementation: Aggregates in exporter queries (`AVG(...)::real`) joined by venue/opposition
  - Validation: Compare example rows for representative players and venues/oppositions

---

## Player Form & Consistency

- Consistency (stored fields)
  - Prototype: stored in `player.(batting_consistency, bowling_consistency)`; used for pool filtering
  - Implementation: same columns in Postgres schema; surfaced by DB repos and exporter
  - Validation: DB rows non-null (or default 0) where expected; pool filtering logic in consumers

- Form (prior season S-1)
  - Prototype: uses `get_player_metric(..., dim='season', key=S-1)` in `select_pool.py`
  - Implementation:
    - Go exporter joins `player_form_data` for relevant season; per-format exports include `*_form`
    - ML service `ml/calculate_features.py` also computes or validates forms during precompute
  - Validation: Spot-check that forms for season S read from S-1 rows; define tolerance for missing → 0

---

## Per-player Predictions

- Batting regressor
  - Prototype: `final_data/batting_regressor.py` used via `predict_batting(...)`
  - Implementation: `ml-service/ml/batting_regressor.py` and FastAPI endpoint `/predict/batting`
  - Inputs: `dataset_definitions.input_batting_columns`
  - Outputs: `runs_scored, balls_faced, fours_scored, sixes_scored, batting_position` (+ derived: strike_rate)

- Bowling regressor
  - Prototype: `final_data/bowling_regressor.py` used via `predict_bowling(...)`
  - Implementation: `ml-service/ml/bowling_regressor.py` and FastAPI endpoint `/predict/bowling`
  - Inputs: `dataset_definitions.input_bowling_columns`
  - Outputs: `runs_conceded, deliveries, wickets_taken` (+ derived: econ)

- Derived metrics
  - Prototype: `strike_rate`, `econ`, contributions
  - Implementation: `ml-service/ml/calculate_features.py` and Pydantic models (derived fields are computed in service responses or post-processing)

---

## Team Selection & Combination

- Pool generation and constraints
  - Prototype: `select_pool.py` filters by retirement and consistency; ensures at least one keeper; identifies bowlers
  - Implementation: pool & selection is a consumer concern; Go API offers data; ML service provides predictions; combination logic to be orchestrated in client or future service module
  - Validation: Reproduce prototype’s selection for golden dataset using predictions + constraints in a harness

- Combination evaluation and ranking
  - Prototype: combinations of XI evaluated via overall win probability (see below)
  - Implementation: Use ML service predictions + win model; tie-break rules documented in the harness

---

## Final Team Prediction (Win Probability)

- Team win model
  - Prototype: `final_data/match_win_predict.py::predict_for_team`
  - Implementation: `ml-service/ml/match_win_predict.py` and endpoint `/predict/win`
  - Inputs: array of `PlayerPrediction` minus `winning_probability`; optional `format`
  - Outputs: `players[*].winning_probability`, `team_win_probability`

- Validation: For golden dataset, compare prototype vs ML service outputs; set small tolerance if model artifacts differ; otherwise expect identical values

---

## Artifacts & Model Lifecycle

- Training
  - Prototype: training invoked in scripts under `src/final_data`
  - Implementation: `ml-service/ml/train_batting_model.py`, `ml-service/ml/train_bowling_model.py`

- Loading
  - Implementation: `ml-service/app/main.py` with artifact directory precedence; hot reload via `/admin_reload`

- Validation: Ensure artifacts correspond to format codes where applicable; standardize directory layout and `latest` symlink

---

## Entry Points and Contracts

- ML service
  - `app/main.py` Pydantic models: `BattingFeatures`, `BowlingFeatures`, `PlayerPrediction`
  - Endpoints: `/health`, `/predict/batting`, `/predict/bowling`, `/predict/win`, `/admin/reload`
  - Contract examples: `docs/api_contracts.md`

- Go app
  - Exporter: `cmd/export-dataset`
  - API: `cmd/api` (health, readiness, precompute trigger, import/cricsheet)
  - Contracts: `internal/contracts` (structs used by handlers and ML client)

---

## What the Harness Will Compare (Golden Dataset)

1. Exported feature CSV headers and dtypes vs `dataset_definitions`
2. Encoded values: session {1,2,3}, viscosity {0,1}, toss {0,1}
3. Player form: season S uses S-1; defaults applied consistently
4. Per-player predictions: batting & bowling
5. Team selection result (XI) subject to constraints; tie-breakers documented
6. Team win probability from ML service vs prototype

---

## Open Items

- Confirm exact toss mapping parity with prototype
- Minimum bowlers per format for constraint checks
- Whether to host team selection/combination logic in Go API or keep it as client-side/harness logic for now
