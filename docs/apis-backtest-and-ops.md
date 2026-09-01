# APIs, prediction, evaluation, and ops

API contracts (Go and ML), the prediction and evaluation surfaces, and the ops status dashboard.

---

## API contracts (summary)

**Design:** Explicit request/response models, consistent error schema (`code`, `message`, optional `details`), validation at boundaries. Prefer additive changes; version for breaking changes.

### ML service (FastAPI) — base `http://localhost:8000`

- **Content-Type:** `application/json`. **Error format:** `{ "error": { "code": "INVALID_INPUT", "message": "...", "details": {...} } }`
- **GET /health** — 200 `{ "status": "ok" }`
- **POST /xi/optimize** — Body: `format`, `pool_player_ids`, `opponent_player_ids` (not read by `objective: "ratings"`), `team_is_team1`, `constraints` (`team_size`, `min_bowlers`, `require_keeper`, `must_include`, `must_exclude`), `max_evaluations`, optional `as_of`, and `objective` — `"win"` searches for the XI that maximises the objective model's P(win), `"ratings"` returns the rating-ordered pick and evaluates no model. Response: `selected_player_ids`, `objective`, `optimised`, `win_probability` (null in ratings mode), `evaluations`, `improved_over_seed`, `unknown_player_ids`, `marginal_values`. **503 `XI_MODEL_UNAVAILABLE`** when `objective: "win"` is asked for a format whose objective does not rank (H-17: TEST) — the hint names `"ratings"`.
- **POST /xi/predict-win** — Body: `format`, `team1_player_ids`, `team2_player_ids`, optional `team1_id` / `team2_id` / `venue_id` / `team1_bats_first` / `as_of`. Response: `team1_win_probability` (the displayed probability) and `objective_probability`.
- **POST /performance/predict** — Same body. Response: per player `p_bats`, `p_bowls`, the 0.1 / 0.5 / 0.9 quantiles of `runs`, `balls_faced` and `runs_conceded`, the wicket distribution (`expected`, `p0`, `p1`, `p2_plus`) and `catches_expected`; `innings_marginalised` is true when the toss was unknown and both batting orders were averaged.
- **POST /simulate** — Same body plus `n_samples` (default 2000) and `seed`. Response: per side the total (`q10`, `median`, `q90`, `mean`, `sd`, `scorecard`), extras, wickets lost, and per player ranges plus the median-band `scorecard` line and `spread_share`; `win_probability` carries `simulated`, `display`, `headline` and `headline_source`. **422 `SIMULATION_UNSUPPORTED_FORMAT`** for a format with no innings length.
- **GET /xi/status** — Loaded formats, `ratings_through`, player count, the last training report.
- **GET /xi/evaluate-report** — L4's `xi_evaluate_report.json`. **503** with a hint to run `make xi-evaluate` when the harness has not run.
- **POST /predict/win**, **POST /predict/win-enhanced** — the windowed-form win model. Superseded; P-6 removes them.

### Go API (mux) — base `http://localhost:8080`

- **GET /health** — 200 `{ "status": "ok" }`
- **GET /readiness** — 200 `{ "status": "ready" }`; 503 when DB unavailable
- **POST /precompute** — 202 `{ "status": "started" }`; optional body `{ "season", "formats" }`
- **GET /precompute/status** — 200 with running, started_at, finished_at, season, formats, phase (starting|form|venue|opposition|consistency|done), last_error
- **POST /import/cricsheet** — Body: `{ "dir", "placeholders_fielding" }`; 202 started
- **GET /players/{id}?season=...&format=...** — Player details (id, player_name, is_wicket_keeper, batting_consistency, bowling_consistency, etc.)

Backtest and ops endpoints are described in the sections below. Keep contracts in sync with `ml-service/app/main.py` Pydantic models and Go `internal/contracts`.

---

## Prediction

**`POST /api/predict/team-selection`** (also GET with query params).

**Body:** `format`, `team1`, `team2`, `match_date` (RFC3339 or `YYYY-MM-DD`), optional `venue`,
`extra_team1` / `extra_team2` (extra player ids for the pool), `min_bowlers`, `require_keeper`.

**Response:**

| Field | Meaning |
|-------|---------|
| `team1`, `team2` | The selected XIs. Each player carries `runs`, `balls`, `wickets`, `runs_conceded` with a `*_range` (10-90) beside each, `economy` where balls bowled are known, `marginal_value` on an optimised XI and `spread_share` where the simulator ran |
| `selection` | `objective` (`win` / `ratings`), `optimised`, and a `note` explaining a rating-ordered XI |
| `win_probability` | `team1`, `source` (`display` / `simulator`), `simulated` where the simulator ran, `predicted_winner` |
| `scorecard` | Present only for a format with an innings length: `samples`, `toss_marginalised`, and per innings the median-band `total`, its `extras` and the 10-90 range of the draws |

The scorecard lines and extras sum to the innings total by construction — they come from the
same draws — so nothing is rescaled toward the win probability.

**Retired fields are refused, not ignored:** `weather`, `simulate`, `use_reconciled_scorecard`
and `include_both_scorecards` each return 400 with a code and a hint. A caller still sending one
would otherwise get an answer to a different question with no indication why.

---

## Evaluation

**`GET /api/backtest/report`** proxies ml-service's `GET /xi/evaluate-report`, which serves
`xi_evaluate_report.json` as `make xi-evaluate` last wrote it. 503 with a hint when the harness
has not run.

The report carries, per format: the walk-forward folds and their summary (objective and display
AUC, Brier against the base rate, swap monotonicity, the specific-XI-beyond-typical-XI delta,
per-target performance metrics, the simulator's E2 section), the locked window in the same
shape, and E2's serving decision. Beside them: the data-quality counts, the leak canary with its
TEST control, and the train/serve parity verdict.

**There is no per-match evaluate flow.** It scored the batting, bowling and fielding models
against actuals and went with them in P-5; what replaced it is the harness, which scores every
format over rolling origins in one run and never uses the locked window for a choice (H-19).

**`GET /api/backtest/training-data?cutoff=...&format=...`** still serves rows to the
windowed-form win trainer and the auto-tune stack. P-6 removes it with them.

**Frontend:** the Evaluation report tab renders the report. It has no form — the folds, the
locked window and the seeds are the harness's, and a cutoff chosen in a browser would be a
choice made against the locked window.

**E2E smoke:** `make e2e-backtest-smoke` checks `/api/backtest/report` (200, or 503 when the
harness has not run) and the options and model-stats endpoints.

---

## Ops status dashboard

**Endpoint:** `GET /ops/status` (no query params). Aggregates: services (API + ML health), DB (connectivity, migration, counts), precompute freshness per format, CSV exports (root, per-format files with exists/rows/modified), ML artifacts (per-format batting/bowling, exists/loaded/modified), fielding and weather data availability (row counts), and an ordered list of **suggestions** (actionable make commands to get the system ready).

**Response sections:** `timestamp`, `services` (api_health, api_readiness, ml_health), `db` (connected, migration status/current/expected, counts: players, matches, innings), `precompute` (last_run, as_of, formats with status ok/stale/missing), `exports` (root from GO_APP_OUTPUT_DIR when set, formats with files: name, exists, rows, modified), `artifacts` (root; probes ML `/artifacts/status` when available else filesystem scan; per-format batting/bowling/fielding/extras/win/innings with exists, loaded, modified, and — from the ML probe — `loaded_modified` and `stale`), `fielding` (available, rows), `weather` (available, rows), `pipeline` (steps with running, completed, runnable per step), `suggestions[]` (reason, commands). Precompute freshness: format in last run is ok if run finished same UTC day else stale; formats not in run = missing. Suggestions priority: DB → Precompute → Exports → Artifacts → Services. Example commands: DB not ready → `make migrate && make cricsheet-import`; precompute missing/stale → `make precompute-asof`; exports missing → `make export-dataset`; artifacts missing → `make ml-install && make train-batting`, `make train-bowling`; ML down → `make dev-up` or `make dev-rebuild`. Fallback artifacts root when ML down: GO_APP_ARTIFACTS_ROOT or output/ml-service.

**Pipeline modes:** Prefer **Train** when params are known (config + DB from previous auto-tune); use **Auto-tune** when discovering or re-optimizing hyperparameters. See **docs/ml-and-training.md** (§ Pipeline modes) for the single-train principle and Mode A (fast path) vs Mode B (tuning path).

**Quick verification:** `make dev-up` then `curl -s http://localhost:8080/health | jq`, `curl -s http://localhost:8080/readiness | jq`, `curl -s http://localhost:8080/ops/status | jq`. Frontend: Ops Status UI shows Fielding and Weather tiles (available, rows); suggestions per section card.

**Force gaps to test:** Remove artifacts → `rm -rf output/ml-service/*`; recheck artifacts and suggestions. Remove exports for a format → recheck exports and suggestions. After `make train-all`, recheck artifacts and suggestions.
