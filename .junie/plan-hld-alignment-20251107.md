### Plan: HLD Alignment — Gap Analysis and Execution Roadmap (2025-11-07)

This document records the gaps found between the current implementation and the High‑Level Design (HLD) diagram, and defines a pragmatic, phased plan to close them. It includes acceptance criteria and exact verification commands for each phase.

---

#### 1) Current vs HLD — Mapping and Gaps

- Data sources → Database
  - Implemented: Cricsheet import (`go-app/cmd/cricsheet-importer`), weather schema & repos (`internal/db/repo_weather*.go`, `0006_weather_job.sql`).
  - Gap: No unified Go-native weather ingestion (historical + forecast). Missing scheduling/backfill story.

- Precompute features (form, consistency, venue, opposition)
  - Implemented: `go-app/cmd/precompute`, per-format feature tables and validations.
  - Gaps: No gated check that precompute ran before export; limited documentation on re-run/backfill strategy.

- Export encoded datasets for ML training
  - Implemented: `go-app/cmd/export-dataset`, per-format exports, schema docs; ML-side validator `ml-service/ml/validate_exports.py`.
  - Gap: Validator not enforced in CI as a hard gate (confirm in workflow and wire if missing). Optional data quality extensions.

- Train player-performance models
  - Implemented: `ml-service/ml/train_batting_model.py`, `ml-service/ml/train_bowling_model.py`, FastAPI server loads joblibs.
  - Gap: Artifact provenance/version manifest not saved; no simple registry/promotion flow documented.

- Predict individual player performance
  - Implemented: `go-app/cmd/team-predictor` performs feature assembly and calls ML service; Go `internal/predictor` aggregates player predictions into team stats.
  - Gap: End-to-end parity CI between training features and online inference features not automated.

- Win contribution predictor → Predict match result (team level)
  - Implemented: None as a true model; only team aggregation exists. Legacy/prototype `_DummyPredictor` in Python is not wired.
  - Gap: Missing trained and served win-probability (or win-contribution) model; `Team.WinningProbability` not set from a real predictor.

- Feedback loop: Get actual player performance and match results → Compare & validate
  - Implemented: DB has ground truth (Cricsheet); no dedicated evaluation tool/job.
  - Gap: Missing evaluation pipeline and storage for metrics (player-level MAE/RMSE; team-level calibration/Brier/accuracy), and reporting.

---

#### 2) Phased Execution Plan

We deliver in small, reviewable PRs, each with tests and docs.

Phase A — Weather ingestion (historical + forecast)
- Create `go-app/cmd/weather-import` CLI to ingest weather by `match_id` and session (batting/bowling) from a provider (stub interface + mock; real provider can be plugged later).
- Extend `internal/db` weather repos as needed; ensure idempotent upserts and indexes.
- Add a simple scheduler script/Make target for periodic forecast updates pre‑match.
- Docs: README updates for configuration (API keys via env) and usage.
Acceptance criteria:
- `go run ./go-app/cmd/weather-import --match=<id> --provider=dummy --apply` writes 2 rows (bat/bowl) with non-null core fields.
- Backfill mode: `--from-file <csv>` imports legacy CSVs deterministically.
Verification commands:
- `make -C go-app migrate`
- `go run ./go-app/cmd/weather-import --match=1193505 --provider=dummy --apply`
- `psql -c "select count(*)>0 from weather_data where match_id=1193505" $PGURL`

Phase B — Win contribution / Match win model
- Feature definition: derive team-level features from player predictions + context (venue, opposition strength, recent form aggregates); align per format.
- ML training: add `ml-service/ml/train_win_model.py` producing `win_model_<FORMAT>.joblib` (+ scaler if needed).
- Serving: add `POST /predict/win` to FastAPI that returns `{win_probability: float}`.
- Go integration: in `cmd/team-predictor`, call `/predict/win` after assembling team features and set `Team.WinningProbability`.
- Tests: unit tests for feature assembly; integration smoke test using a golden payload with a tiny model.
Acceptance criteria:
- `make -C ml-service train-win` writes `output/ml-service/win_model_<FORMAT>.joblib`.
- `curl /predict/win` returns `200` and `0<=win_probability<=1`.
- `go run ./go-app/cmd/team-predictor -match=<id> -format=T20` prints a team with non-zero `WinningProbability`.
Verification commands:
- `make -C ml-service train-win`
- `make -C ml-service run &` then `curl -X POST localhost:8000/predict/win -d @tests/fixtures/win_request.json`
- `make -C go-app team-predictor MATCH=<id> FORMAT=T20`

Phase C — Post‑match evaluation pipeline
- Schema: add `model_evaluations.*` tables (runs, metrics, per-entity rows).
- Go CLI `go-app/cmd/evaluate`: joins stored predictions (persisted by team‑predictor) with actuals to compute metrics (MAE/RMSE; Brier; calibration bins). Writes to DB and outputs a markdown summary.
- Tests: unit tests for metric calculators (pure functions); golden test for a tiny dataset.
- Docs: how to run backtests by season/format.
Acceptance criteria:
- `go run ./go-app/cmd/evaluate -season=2019 -format=T20` produces metrics and inserts rows in `model_evaluations`.
Verification commands:
- `psql -c "select * from model_evaluations_runs order by created_at desc limit 1" $PGURL`

Phase D — Parity and CI gates
- Wire `make -C ml-service validate-exports` into `.github/workflows/go-ci.yml` as a required job.
- Add a smoke inference check: start ML service in CI (or run in-process) and validate response schemas for batting/bowling (and win once added) using frozen payloads.
- Optional: extend validator thresholds (null %, min row counts).
Acceptance criteria:
- CI fails when export schemas drift or inference schemas do not match expected.
Verification commands:
- `gh workflow run go-ci.yml` (or push PR) shows the new gates.

Phase E — Artifact provenance
- On every training run, write a manifest JSON next to artifacts with: git SHA, training window, format(s), feature schema hash, and hyperparameters.
- Expose `/models` endpoint to list loaded artifacts + metadata.
Acceptance criteria:
- `ls output/ml-service/*_manifest_<FORMAT>.json` exists; `/models` returns metadata.
Verification commands:
- `curl localhost:8000/models | jq` shows win/batting/bowling models with manifest data.

---

#### 3) Files to create/modify (high-level)
- New (Go):
  - `go-app/cmd/weather-import/main.go`
  - `go-app/internal/weather/` (provider interface, DTOs, stub provider, scheduler helper)
  - `go-app/cmd/evaluate/main.go`
  - `go-app/internal/eval/` (metrics calc: MAE, RMSE, Brier, calibration)
  - `go-app/internal/teamwin/` (team feature engineering for win model)
- Modify (Go):
  - `go-app/internal/db/repo_weather*.go` (helper upserts)
  - `go-app/cmd/team-predictor` (call `/predict/win`, persist predictions)
  - `go-app/migrations/` (evaluation tables, indexes)
  - `go-app/Makefile` (targets: `weather-import`, `evaluate`, coverage)
- New (Python, ml-service):
  - `ml-service/ml/train_win_model.py`
  - Add `/predict/win` route in `ml-service/app/main.py`
  - Manifests: write `*_manifest_<FORMAT>.json` alongside artifacts
  - Tests and fixtures under `ml-service/tests/`
- CI:
  - `.github/workflows/go-ci.yml` add steps for export validation and inference smoke.
- Docs:
  - `docs/ARCHITECTURE.md` updates: win model and evaluation loop
  - `README.md` quickstart updates and Make targets

---

#### 4) Testing strategy
- Go: table-driven unit tests; avoid if statements in tests; use `httptest` for HTTP clients.
- Python: `pytest -q`; deterministic fixtures in `tests/fixtures/`, set random seeds.
- Golden tests: fixed small CSVs/payloads for reproducibility.

---

#### 5) Make targets and commands
- Weather: `make -C go-app weather-import MATCH=<id> PROVIDER=dummy APPLY=1`
- Win model: `make -C ml-service train-win && make -C ml-service run`
- Predictor: `make -C go-app team-predictor MATCH=<id> SEASON=YYYY FORMAT=T20`
- Evaluate: `make -C go-app evaluate SEASON=YYYY FORMAT=T20`
- Validate exports: `make -C ml-service validate-exports`

---

#### 6) Acceptance checklist (global)
- [ ] Weather rows present for target matches (both sessions) and used at inference time.
- [ ] Team predictions include `WinningProbability` from a trained win model.
- [ ] Evaluation metrics are computed and stored post‑match; backtest command works.
- [ ] CI gates schema/inference parity; PRs fail on drift.
- [ ] Artifacts ship with provenance manifests; `/models` exposes them.

---

#### 7) Risks & mitigations
- Data gaps in weather provider → Allow CSV backfill and a stub provider; record data lineage in manifests.
- Feature parity drift → Keep export/inference headers under golden tests; CI smoke inference.
- Model instability → Fix random seeds; small baseline models first; track metrics over time via evaluation tables.

---

#### 8) Timeline (suggested)
- A: 1–2 days (stub provider + import + docs)
- B: 2–4 days (features, training, serving, integration)
- C: 2 days (schema, metrics, CLI, golden test)
- D: 0.5 day (CI wiring)
- E: 0.5 day (manifests + endpoint)
