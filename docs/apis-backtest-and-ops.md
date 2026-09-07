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
- **POST /xi/predict-win** — Body: `format`, `team1_player_ids`, `team2_player_ids`, optional `team1_id` / `team2_id` / `venue_id` / `team1_bats_first` / `as_of`, and optional `team1_constraints` / `team2_constraints` (P1-2: check the eleven being scored against these instead of selecting under them). Response: `team1_win_probability` (the displayed probability), `objective_probability`, and `team1_constraint_check` / `team2_constraint_check` where constraints were sent — the eleven's size, its bowler count by the optimiser's own definition, whether it holds a keeper, the `must_include` ids it does not hold, and whether it `met` them all.
- **Every prediction response** (`/xi/optimize`, `/xi/predict-win`, `/simulate`, `/performance/predict`) carries `served_ratings: {run_id, ratings_through}` — the run the answering store was loaded from and the last match date its ratings include, read off that store (P1-5). A live request against ratings older than `ml.ratings_max_age_days` is **503 `RATINGS_STALE`**, the message naming the date, the age and the limit and the hint the step that fixes it (H-11).
- **POST /performance/predict** — Same body. Response: per player `p_bats`, `p_bowls`, the 0.1 / 0.5 / 0.9 quantiles of `runs`, `balls_faced` and `runs_conceded`, the wicket distribution (`expected`, `p0`, `p1`, `p2_plus`) and `catches_expected`; `innings_marginalised` is true when the toss was unknown and both batting orders were averaged.
- **POST /simulate** — Same body plus `n_samples` (default 2000) and `seed`. Response: per side the total (`q10`, `median`, `q90`, `mean`, `sd`, `scorecard`), extras, wickets lost, and per player ranges plus the median-band `scorecard` line and `spread_share`; `win_probability` carries `simulated`, `display`, `headline` and `headline_source`. **422 `SIMULATION_UNSUPPORTED_FORMAT`** for a format with no innings length.
- **GET /xi/status** — Loaded formats, `ratings_through`, player count, the run's own training
  report, and — since P-6 — `run_id` and `manifest` (H-16: run id, cutoff, dataset sha, git sha,
  the hyperparameters the grid chose, the run's headline metrics), `ratings` (H-11: `fresh`,
  `age_days`, `max_age_days`, `code`) and `error` (D-6: why a run on disk was refused).
- **GET /xi/evaluate-report** — L4's `xi_evaluate_report.json`. **503** with a hint to run `make evaluate` when the harness has not run.
- **GET /xi/metric-glossary** — What every reported metric means (L-1): `{"entries": {<metric key>: {key, name, explanation, band, better, direction, scale}}}`, from `ml/xi/glossary.py`. `scale` is the band as two numbers — `{bad, good}`, read through `direction` — for a surface that paints a value red-to-green instead of printing the sentence; it is `null` for a metric with no defensible anchor, which is why an interval width shows uncoloured. Always 200 — it is served from the code, so a surface can explain its numbers before any run exists.
- **GET /artifacts/status** — Every run on disk, newest first, with `current_run`, `loaded_run`,
  the ratings verdict and the loader's refusal. A run directory with no manifest is listed as
  `has_manifest: false` rather than hidden: it is exactly what an operator is looking for when
  nothing loads.
- **POST /admin/reload?run=<id>** — Point `current` at a run and load it. Without `run`: the
  newest run on disk, which is the one the retrain before it built; naming a run is how you roll
  back to an earlier one. **409 `RUN_ARTIFACTS_INVALID`** when the run is
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

**Body:** `format`, `match_date` (RFC3339 or `YYYY-MM-DD`), the two sides, and optionally
`venue`, `extra_team1` / `extra_team2` (must-include player ids: they join the pool *and*
the selection is required to pick them), `min_bowlers`,
`require_keeper`, `team1_pool` / `team2_pool`, `team1_bats_first` (the toss: true where
team 1 bats first, absent where it is unknown and both orders are drawn, P1-1), and
`team1_xi` / `team2_xi` (Play mode, P1-2).

**Play mode: scoring an eleven the caller built (P1-2).** `team1_xi` / `team2_xi` name each
side's eleven by `player_id`. Sent, the selection step is skipped and exactly those players
are scored; omitted, the XIs are selected as they always were. Everything after the
selection is the same code either way — the displayed probability is still
`/xi/predict-win`'s and the totals, ranges and scorecard are still `/simulate`'s — so a
re-score and an Optimise for the same eleven return the same numbers. A pinned player joins
his side's pool whatever the window or the ledger says, the way a must-include id does.

Four refusals, because a repaired eleven is not the eleven that was sent: a side that is
not a full XI is **`400 XI_INCOMPLETE`** (every model here aggregates a whole side, so a
ten-man side would be a prediction for a match nobody plays), an id that names no player is
**`400 XI_PLAYER_UNKNOWN`**, and naming the same player twice or pinning one side while
leaving the other to be searched is a `400` naming what is wrong.

**The candidate pool (D-12).** Each side's XI is chosen out of the players who appeared for
that club in that format within a **recency window** ending at `match_date` — twelve months
for TEST, ODI and T20, nine for T20I, measured as the smallest window covering ≥ 95% of the
players who actually took the field (`pool.recency_months` in `go-app/config.json`;
[EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § D-12 has the table). It used to be all-time,
which is why an XI could contain a player who retired a decade ago.

`team1_pool` / `team2_pool` change one side's pool: `{"window_months": 24}` for a different
window, `{"all_time": true}` for everyone who has ever played for the club, or
`{"players": [...]}` for a pool the user picked by hand — a manual pick *is* the pool, and
neither the window nor the ledger is applied to it. `window_months` and `all_time` are also
readable off the query string on the GET form, and apply to both sides there. Omitting the
field is the default window; there is no way to ask for an unbounded pool by leaving
something out. `extra_team1` / `extra_team2` bypass every filter, as before, and since
B-10 they are enforced: their registry ids reach `/xi/optimize` as `must_include`, so the
seed holds them and no swap removes them. A lock nothing can satisfy is refused with its
reason rather than quietly relaxed — a must-include id this side cannot field (no player
row, or no `external_id`) is **`400 MUST_INCLUDE_UNRESOLVABLE`**, and more required players
than places, or a lock that leaves no room for the keeper or the minimum bowlers, is
ml-service's **`422 OPTIMIZATION_CONSTRAINT_ERROR`** naming which constraint failed.

A pool with fewer than eleven players is **`400 POOL_TOO_SMALL`**, whose message names the
window and whose hint names the two ways out.

**`GET /api/options/candidates?format=&club_id=`** (optionally `match_date`, `window_months`,
`all_time`) is the list a manual pool is ticked out of: every candidate with `player_id`,
`player_name`, `is_wicket_keeper`, `last_played`, and — for anyone the retirement ledger is
keeping out — `excluded`, `reason` and `detail`. Excluded players are on the list, not missing
from it, so the exclusion can be seen and undone.

**`POST` / `DELETE /api/players/{id}/retirement`** is the retirement ledger. A POST records
this user's claim that a player has retired; the claim hides him from that user's default
pools immediately and becomes the stored `player.is_retired` fact **only** where an
independent criterion corroborates it — no match in any format for five years today, plus a
Wikidata career-end date and an age-with-inactivity rule that report themselves `unchecked`
until X-1a supplies their evidence. The response says `promoted`, the `criterion` that
corroborated it, its `detail`, and the `notes` each criterion left. A DELETE withdraws the
claim and demotes the fact it had raised. `X-User-Id` names whose ledger is read and written;
absent, it is the single default user. The ledger applies to upcoming-match requests only —
a backtest names an `as_of` date, and a claim made today is not evidence about who was
available then (H-19).

**Naming a side.** A team is `(name, gender)` — 130 of the 394 names in the dataset are used
by both a men's and a women's side — so a side is named by `team1_id` / `team2_id`, the
`club_id` from `GET /api/options/teams-by-format`, or by `team1` / `team2` with
`team1_gender` / `team2_gender`. A bare name is accepted only where the format holds one side
of that name; where it holds two the request is **`400 TEAM_AMBIGUOUS`** with both candidates
in `available`, never a silent pick (D-10). A side that has not played the format is
`400 TEAM_NOT_FOUND`, and a fixture whose two sides are different genders is
**`400 FIXTURE_CROSS_GENDER`** — no such match is played, so a probability for one would have
no referent.

**Options.** `GET /api/options/teams-by-format?format=` returns sides, not names:
`{club_id, name, gender, display_name}`, one row per side, folded onto the club so a renamed
club appears once under its current name. `GET /api/options/opponents?format=&team_id=`
returns the sides that club has played, in the same shape.

**Response:**

| Field | Meaning |
|-------|---------|
| `ratings_through`, `run_id` | Which rating state every number was computed from: the last match date the served ratings include (`YYYY-MM-DD`) and the run they were loaded from. Both required, never omitted (P1-5) — they are ml-service's `served_ratings` stamp, which every call the prediction made must agree on |
| `team1_side`, `team2_side` | The sides that were actually scored — `club_id`, `name`, `gender`, `display_name` — echoed on every prediction, not only an ambiguous one |
| `team1`, `team2` | The selected XIs. Each player carries `runs`, `balls`, `wickets`, `runs_conceded` with a `*_range` (10-90) beside each, `economy` where balls bowled are known, `marginal_value` on an optimised XI and `spread_share` where the simulator ran |
| `selection` | `objective` (`win` / `ratings` / `fixed`), `optimised`, and a `note` explaining an XI that was not optimised — the format's reason (H-17 where the objective does not rank; E5 where it has not shown it selects), or, for `fixed`, that the caller pinned the eleven and nothing was searched for (P1-2). `must_include` is present where any were asked for on a selected eleven: per side, how many were required and each one the eleven does not hold — a postcondition on the lock since B-10, so `missing` is empty on any ordinary answer |
| `forecast` | `source` (`simulator` / `performance_quantiles`) and a `note` where the numbers did not come from the simulator |
| `win_probability` | `team1`, `source` (`display` / `simulator`), `simulated` where the simulator ran, `predicted_winner` |
| `toss` | Which batting order the numbers assume: `team1_bats_first` (null where it was unknown and both orders were drawn), `honoured`, and a `note` where a named toss could not be used (P1-1) |
| `scorecard` | Present only for a format with an innings length: `samples`, `toss_marginalised`, and `team1_innings` / `team2_innings` — named by side, not by batting position — each with the median-band `total`, its `extras` and the 10-90 range of the draws |
| `constraints` | Present only where the caller pinned the elevens (P1-2): `team_size`, `min_bowlers` and `require_keeper` as they were asked for, and per side the `size`, the `bowlers` count, `has_keeper`, any `missing_must_include` players and whether the eleven `met` what was asked. The counts are ml-service's, measured on the eleven that was scored — nothing is repaired to satisfy them |
| `team1_pool`, `team2_pool` | Which candidates each XI was chosen out of: `source` (`recency_window` / `all_time` / `manual`), the `window_months` and `since` it applied, the `size` it produced, and `retired_excluded` with the `excluded` players themselves — each with `reason` (`retired` / `user_flagged`) and `detail` |

The scorecard lines and extras sum to the innings total by construction — they come from the
same draws — so nothing is rescaled toward the win probability.

**Every substitution is named on the wire (§8.7).** Five fields say what answered: `team1_side`
and `team2_side` say which sides were scored — substituting the men's side for the women's is
a substitution, and it used to be announced only in a server log (D-10) — `selection` says
whether the XIs were optimised or rating-ordered, `forecast` says whether the per-player
numbers came from the simulator's draws or from L2-B's own quantiles, and
`win_probability.source` says which model produced the headline, and `team1_pool` /
`team2_pool` say which candidates the XIs were chosen out of and who the ledger removed — a
filter is a substitution too, and a pool that silently dropped a player was D-12. The rule
exists because
go-app silently falling back from a refused `/xi/optimize` to another optimiser is what let a
broken arm report a number for months (§8.5). Where a substitution *cannot* be labelled — a
player the simulator or the performance model returned no line for — the request fails instead
of leaving that player's row at zeros, which would read as a forecast of nothing.

**A prediction is dated or refused, never served stale (P1-5, H-11).** A live prediction
against ratings older than `ml.ratings_max_age_days` is **`503 RATINGS_STALE`**, relayed from
ml-service with its code, its message (the date, the age, the limit) and its hint intact —
go-app passes an upstream 503 through rather than rewriting it as 502, because a named
refusal is not a broken gateway. A prediction whose ml-service calls were answered from two
different runs — a reload landed mid-request — is **`409 SERVED_RUN_CHANGED`** naming both,
and the remedy is to run it again.

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
of them, L-1). `market_benchmark` (X-4) carries the market arm: the source and its licence, the
de-vig method and mean overround, the join's counts, and per format the joined coverage with
the per-fold and pooled AUC/Brier of the market, the display model as served and the display
model read toss-aware. A format with no joined closing price still has a node, stating zero
coverage. At the top it names the window every locked figure came from: `locked_start`,
and `locked_window` with the date the line was last moved, the window it replaced and why
(A-4 — see **docs/ml-and-training.md** § Rotating the locked window).

**`GET /api/backtest/metric-glossary`** proxies ml-service's `GET /xi/metric-glossary`: the same
entries, served from the code rather than from a report on disk. It is what every metric label in
the frontend opens — the evaluation tables, the Workbench's manifest metrics, the run summary's
metrics and the prediction surfaces' ranges and marginal values — so no component carries prose
about a metric, and a rewording is a change to `ml/xi/glossary.py` alone. The evaluation tab also
paints every number it shows from these entries' `scale`, so a colour cannot disagree with the
band in the popover beside it, and moving an anchor is the same one-file change.

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
counts, one freshness verdict and DB completeness per format, the pipeline's per-step state,
and an ordered list of **suggestions** (the next make command the run history says is missing).

**Response sections:** `timestamp`, `services` (api_health, api_readiness, ml_health), `db`
(connected, migration status/current/expected, counts), `dataset` (the directory Import reads,
its manifest and match-file count), `artifacts`, `fielding` (available, rows), `freshness`,
`db_completeness`, `pipeline` (per step: running, completed, runnable, optional),
`suggestions[]`.

**`freshness` is one verdict, assembled once (P2-1).** Every surface that shows freshness — the
Health tab, the Ops badge, the Workbench's loaded-run card, the system map, and the Lab's
readiness notice — reads this object and nothing else:

- `freshness.served` — **H-11's verdict, exactly as ml-service reports it**, copied through and
  never recomputed: `fresh`, `age_days`, `max_age_days`, `ratings_through`, `code`
  (`RATINGS_STALE` when a live prediction would be refused), plus `status`, the word that
  spells it: `fresh`, `stale`, `not_loaded` (nothing is loaded, so there is no age and the
  remedy is a reload) or `unknown` (ml-service did not answer, so there is no verdict to
  report). This is the only badge and the only thing that decides whether a prediction would be
  refused. **`ml.ratings_max_age_days` is the only threshold in the system**; go-app holds no
  copy of it and reads the limit off the verdict.
- `freshness.database[FORMAT]` — the import's lag as facts: `latest_match_date`, `age_days`,
  `match_count`, and a `note` when there is no date (no matches imported, or the database could
  not be read). No bucket and no status: a Test played a fortnight apart is not a fault, which
  is what the deleted `db_freshness` buckets called *stale*.
- `freshness.retrain_due` — whether the database holds matches the served run never saw (B-2):
  `status` (`up_to_date` | `retrain_due` | `unknown`), `days_behind`, `latest_match_date` and
  the `format` that holds it.

The status words and the refusal code are declared once in
`contracts/ops-console.contract.json` (`freshness_statuses`, `retrain_statuses`,
`ratings_stale_code`) and asserted from go-app, the frontend and ml-service (H-24).

**`artifacts` reports runs, not a matrix.** It probes ml-service `GET /artifacts/status` and
copies the answer through whole: `current_run`, `loaded_run`, `ratings_through`, `ratings`
(ml-service's own verdict, which is where `freshness.served` is read from — no surface reads it
from here), `error` (the loader's refusal, D-6) and `runs[]` — each with `run_id`,
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

---

## The cadence (unattended)

`make cadence` is the scheduled counterpart of the console: it starts the `refresh` run plan
(fetch → extract → import → retrain → reload) through `POST /ops/pipeline/run-plan`, polls
`GET /ops/pipeline/plan` until it stops, and turns the outcome into an exit code. Nothing about
the pipeline lives in the script — ordering, lanes, run history and "stop at the first failure"
are already the server's, and a scheduler with its own copy of them would be a second pipeline.
`make cadence-dry-run` checks the same preconditions (API reachable, key accepted, the plan
offered, nothing already running) and starts nothing.

| Exit | |
|---|---|
| 0 | the plan completed and `ratings.fresh` is true |
| 1 | precondition: missing `curl`/`jq`, unreachable API, unknown plan |
| 2 | a step failed; nothing after it ran, so `current` did not move |
| 3 | a plan was already in flight |
| 4 | cancelled, or still running after `TIMEOUT_MINUTES` |
| 5 | the plan completed but the ratings are still outside H-11's limit |

Exit 5 is the interesting one: the chain worked and the service will still refuse live
predictions, which means the data itself is old (no matches published, or a source problem) —
not something another run will fix. `POST /ops/pipeline/stop` stops a cadence exactly as it
stops a console run, including the training process on ml-service.

Environment: `API_URL`, `API_KEY`, `PLAN`, `POLL_SECONDS`, `TIMEOUT_MINUTES`, `DRY_RUN`.
Scheduler examples are in `deploy/cadence/`; the rhythm and what to check afterwards are in
[overview.md](overview.md) § Cadence.
