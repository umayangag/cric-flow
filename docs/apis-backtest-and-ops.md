# APIs, prediction, evaluation, and ops

API contracts (Go and ML), the prediction and evaluation surfaces, and the ops status dashboard.

---

## API contracts (summary)

**Design:** Explicit request/response models, consistent error schema (`code`, `message`, optional `details`), validation at boundaries. Prefer additive changes; version for breaking changes.

### ML service (FastAPI) — base `http://localhost:8000`

- **Content-Type:** `application/json`. **Error format:** `{ "error": { "code": "INVALID_INPUT", "message": "...", "details": {...} } }`
- **GET /health** — 200 with `status`, `models_dir`, `loaded`, `run_id`, the loaded XI and
  performance formats, `ratings` (H-11's verdict) and `error` (the loader's refusal, when there
  is one). `status: "ok"` is about the process: a service with no model loaded is alive, and
  `loaded: false` is how it says so.
- **Player id contract:** every `*_player_ids` field on an XI endpoint, and every key of
  `marginal_values`, is the **Cricsheet registry id** — `player.external_id` in the database, a
  hex string such as `2911de16` — never the numeric `player.player_id`. That is the key the
  rating state is built under (P-1), so a numeric id sent here matches nobody and every player
  comes back unrated (D-7a). A player whose row has no `external_id` cannot be sent.
- **POST /xi/optimize** — Body: `format`, `pool_player_ids`, `opponent_player_ids` (not read by `objective: "ratings"`), `team_is_team1`, `constraints` (`team_size`, `min_bowlers`, `require_keeper`, `must_include`, `must_exclude`), `max_evaluations`, optional `as_of`, and `objective` — `"win"` searches for the XI that maximises the objective model's P(win), `"ratings"` returns the rating-ordered pick and evaluates no model. Response: `selected_player_ids`, `objective`, `optimised`, `win_probability` (null in ratings mode), `evaluations`, `improved_over_seed`, `unknown_player_ids`, `marginal_values`. **503 `XI_MODEL_UNAVAILABLE`** when `objective: "win"` is asked for a format that is not offered an optimised selection — the message carries the format's reason from `ml.xi.optimizer.NOT_OPTIMISED_REASONS` (H-17: the objective does not rank, TEST; or E5: the objective has not shown it selects, plan §8.8) and the hint names `"ratings"`.
- **POST /xi/predict-win** — Body: `format`, `team1_player_ids`, `team2_player_ids`, optional `team1_id` / `team2_id` / `venue_id` / `team1_bats_first` / `as_of`. Response: `team1_win_probability` (the displayed probability) and `objective_probability`.
- **POST /performance/predict** — Same body. Response: per player `p_bats`, `p_bowls`, the 0.1 / 0.5 / 0.9 quantiles of `runs`, `balls_faced` and `runs_conceded`, the wicket distribution (`expected`, `p0`, `p1`, `p2_plus`) and `catches_expected`; `innings_marginalised` is true when the toss was unknown and both batting orders were averaged.
- **POST /simulate** — Same body plus `n_samples` (default 2000) and `seed`. Response: per side the total (`q10`, `median`, `q90`, `mean`, `sd`, `scorecard`), extras, wickets lost, and per player ranges plus the median-band `scorecard` line and `spread_share`; `win_probability` carries `simulated`, `display`, `headline` and `headline_source`. **422 `SIMULATION_UNSUPPORTED_FORMAT`** for a format with no innings length.
- **GET /xi/status** — Loaded formats, `ratings_through`, player count, the run's own training
  report, and — since P-6 — `run_id` and `manifest` (H-16: run id, cutoff, dataset sha, git sha,
  the hyperparameters the grid chose, the run's headline metrics), `ratings` (H-11: `fresh`,
  `age_days`, `max_age_days`, `code`) and `error` (D-6: why a run on disk was refused).
- **GET /xi/evaluate-report** — L4's `xi_evaluate_report.json`. **503** with a hint to run `make evaluate` when the harness has not run.
- **GET /xi/metric-glossary** — What every reported metric means (L-1): `{"entries": {<metric key>: {key, name, explanation, band, better, direction}}}`, from `ml/xi/glossary.py`. Always 200 — it is served from the code, so a surface can explain its numbers before any run exists.
- **GET /artifacts/status** — Every run on disk, newest first, with `current_run`, `loaded_run`,
  the ratings verdict and the loader's refusal. A run directory with no manifest is listed as
  `has_manifest: false` rather than hidden: it is exactly what an operator is looking for when
  nothing loads.
- **POST /admin/reload?run=<id>** — Point `current` at a run and load it. Without `run`: the run
  `current` already names, or the newest one. **409 `RUN_ARTIFACTS_INVALID`** when the run is
  not one, or when its arrays are not the arrays this code reads (D-6); **403 `RELOAD_DISABLED`**
  when `ENABLE_HOT_RELOAD` is off.
- **POST /admin/train/retrain?cutoff=...** — Build one run. **400 `CUTOFF_REQUIRED`** without a
  cutoff. It publishes nothing; `/admin/reload` does that.
- **POST /admin/train/evaluate** — Run L4 and write its report. Touches no artifact `current`
  points at.

**Every XI endpoint refuses a stale live request.** A request with no `as_of` against ratings
older than `ml.ratings_max_age_days` (default 14) answers **503 `RATINGS_STALE`**, with a hint
naming the step that fixes it (H-11). A request that names its own `as_of` is served: a
backtest asks for a date and gets it.

### Go API (mux) — base `http://localhost:8080`

- **GET /health** — 200 `{ "status": "ok" }`
- **GET /readiness** — 200 `{ "status": "ready" }`; 503 when DB unavailable
- **POST /import/cricsheet** — Body: `{ "dir", "placeholders_fielding" }`; 202 started
- **GET /api/ml/xi-status** — proxies ml-service `GET /xi/status`: which run is loaded, what its
  manifest records, and whether its ratings are fresh enough to answer with
- **GET /players/{id}** — one player's row: `id`, `player_name`, `is_wicket_keeper`, `is_retired`.
  The consistency numbers it used to carry came from `feature_raw_stats_snapshots`, which P-6
  dropped with the precompute pass that filled it; a player's form is in the rating state, read
  through the XI endpoints.

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
| `selection` | `objective` (`win` / `ratings`), `optimised`, and a `note` explaining a rating-ordered XI — the format's reason (H-17 where the objective does not rank; E5 where it has not shown it selects) |
| `forecast` | `source` (`simulator` / `performance_quantiles`) and a `note` where the numbers did not come from the simulator |
| `win_probability` | `team1`, `source` (`display` / `simulator`), `simulated` where the simulator ran, `predicted_winner` |
| `scorecard` | Present only for a format with an innings length: `samples`, `toss_marginalised`, and per innings the median-band `total`, its `extras` and the 10-90 range of the draws |

The scorecard lines and extras sum to the innings total by construction — they come from the
same draws — so nothing is rescaled toward the win probability.

**Every substitution is named on the wire (§8.7).** Three fields say which model answered:
`selection` says whether the XIs were optimised or rating-ordered, `forecast` says whether the
per-player numbers came from the simulator's draws or from L2-B's own quantiles, and
`win_probability.source` says which model produced the headline. The rule exists because
go-app silently falling back from a refused `/xi/optimize` to another optimiser is what let a
broken arm report a number for months (§8.5). Where a substitution *cannot* be labelled — a
player the simulator or the performance model returned no line for — the request fails instead
of leaving that player's row at zeros, which would read as a forecast of nothing.

**Retired fields are refused, not ignored:** `weather`, `simulate`, `use_reconciled_scorecard`
and `include_both_scorecards` each return 400 with a code and a hint. A caller still sending one
would otherwise get an answer to a different question with no indication why.

---

## Evaluation

**`GET /api/backtest/report`** proxies ml-service's `GET /xi/evaluate-report`, which serves
`xi_evaluate_report.json` as `make evaluate` last wrote it. 503 with a hint when the harness
has not run.

The report carries, per format: the walk-forward folds and their summary (objective and display
AUC, Brier against the base rate, swap monotonicity, the specific-XI-beyond-typical-XI delta,
per-target performance metrics, the simulator's E2 section), the locked window in the same
shape, E2's serving decision, the lineup-only natural experiment (`e5_lineup_only`: per fold,
pooled with its derived bar, and the locked window labelled) and the selection decision it
implies (`selection_decision`: agreement, bar, pass/fail, whether optimised selection is served,
and the sentence saying why). Beside them: the data-quality counts, the leak canary with its
TEST control, the train/serve parity verdict, the gate registry (`gates`: every gate's
varies / fixed / decides triple and whether the report carries all of them, H-23) and the metric
glossary (`glossary`: one entry per metric key the report prints, and whether it explained all
of them, L-1).

**`GET /api/backtest/metric-glossary`** proxies ml-service's `GET /xi/metric-glossary`: the same
entries, served from the code rather than from a report on disk. It is what every metric label in
the frontend opens — the evaluation tables, the Workbench's manifest metrics, the run summary's
metrics and the prediction surfaces' ranges and marginal values — so no component carries prose
about a metric, and a rewording is a change to `ml/xi/glossary.py` alone.

**There is no per-match evaluate flow.** It scored the batting, bowling and fielding models
against actuals and went with them in P-5; what replaced it is the harness, which scores every
format over rolling origins in one run and never uses the locked window for a choice (H-19).

**There is no `training-data` endpoint.** It served rows to the windowed-form win trainer and
the auto-tune stack, and went with both in P-6. The rating pass reads the event store directly.

**Frontend:** the Evaluation report tab renders the report. It has no form — the folds, the
locked window and the seeds are the harness's, and a cutoff chosen in a browser would be a
choice made against the locked window.

**E2E smoke:** `make e2e-backtest-smoke` checks `/api/backtest/report` (200, or 503 when the
harness has not run), the options endpoint, and `/api/ml/xi-status` for the loaded run and its
freshness verdict.

---

## Ops status dashboard

**Endpoint:** `GET /ops/status` (no query params). Aggregates: services (API + ML health), DB
(connectivity, migration, counts), the runs on disk and which one is serving, fielding row
counts, DB freshness and completeness per format, the pipeline's per-step state, and an ordered
list of **suggestions** (the next make command the run history says is missing).

**Response sections:** `timestamp`, `services` (api_health, api_readiness, ml_health), `db`
(connected, migration status/current/expected, counts), `dataset` (the directory Import reads,
its manifest and match-file count), `artifacts`, `fielding` (available, rows), `db_freshness`,
`db_completeness`, `pipeline` (per step: running, completed, runnable, optional),
`suggestions[]`.

**`artifacts` reports runs, not a matrix.** It probes ml-service `GET /artifacts/status` and
copies the answer through whole: `current_run`, `loaded_run`, `ratings_through`, `ratings`
(H-11's verdict), `error` (the loader's refusal, D-6) and `runs[]` — each with `run_id`,
`created_at`, `cutoff`, `git_sha`, `dataset_sha`, `formats`, `has_manifest`, `current` and
`loaded`. It reports runs because a run is what an artifact belongs to now (H-16): "is the model
current?" is answered by which run `current` points at and whether that is the run the process
loaded, not by six per-format files that could each have come from a different session. When
ml-service cannot be reached, go-app scans `<root>/runs/*/manifest.json` itself and reports
`reachable: false` — the scan can say what exists, and does not claim to know what is loaded.

**Suggestions** walk the three-step chain: no import → `make migrate && make cricsheet-import`;
a retrain older than the import (or none) → `make retrain CUTOFF=...`; a reload older than the
retrain (or none) → `make reload`. Only the earliest unmet one is offered — telling an operator
to do three things in an order the message does not name is how the old list was read wrong.

**Quick verification:** `make dev-up` then `curl -s http://localhost:8080/health | jq`,
`curl -s http://localhost:8080/readiness | jq`, `curl -s http://localhost:8080/ops/status | jq`.

**Force gaps to test:** point `GO_APP_ARTIFACTS_ROOT` at an empty directory and recheck
`artifacts` and `suggestions`; remove a run's `manifest.json` and recheck that it is listed as
`has_manifest: false` and that a reload of it answers 409.
