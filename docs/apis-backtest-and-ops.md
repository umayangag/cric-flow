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
- **Side contract (SERVE-02):** every eleven these routes are given is **exactly eleven
  distinct registry ids**, and no player is on both sides. `team1_player_ids` and
  `team2_player_ids` (on `/xi/predict-win`, `/performance/predict` and `/simulate`) are each
  refused with **422** at any other length, with a repeated id, or where the two lists
  intersect; `pool_player_ids` must be distinct and at least eleven, and
  `opponent_player_ids`, where given, is a full distinct eleven disjoint from the pool.
  `constraints.team_size` is 11 and nothing else. Every model behind these routes was fitted
  on elevens and every number they return aggregates one side into one row, so a nine-player
  side used to come back with a plausible probability for a question nobody asked, a
  duplicated pool id could be selected twice, and a player on both sides was read into both
  aggregates — none of it visible on the response (§8.7). The message names the field and
  what is wrong with it.
- **POST /xi/optimize** — Body: `format`, `pool_player_ids`, `opponent_player_ids` (not read by `objective: "ratings"`), `team_is_team1`, `constraints` (`team_size`, `min_bowlers`, `require_keeper`, `must_include`, `must_exclude`), `max_evaluations`, optional `as_of`, and `objective` — `"win"` searches for the XI that maximises the objective model's P(win), `"ratings"` returns the rating-ordered pick and evaluates no model. Response: `selected_player_ids`, `objective`, `optimised`, `win_probability` (null in ratings mode), `evaluations`, `improved_over_seed`, `unknown_player_ids`, `marginal_values`. **503 `XI_MODEL_UNAVAILABLE`** when `objective: "win"` is asked for a format that is not offered an optimised selection — the message carries the format's reason from `ml.xi.optimizer.NOT_OPTIMISED_REASONS` (H-17: the objective does not rank, TEST; or E5: the objective has not shown it selects, plan §8.8) and the hint names `"ratings"`.
- **POST /xi/predict-win** — Body: `format`, `team1_player_ids`, `team2_player_ids`, optional `team1_id` / `team2_id` / `venue_id` / `team1_bats_first` / `as_of`, and optional `team1_constraints` / `team2_constraints` (P1-2: check the eleven being scored against these instead of selecting under them). Response: `team1_win_probability` (the displayed probability, read at `team1_bats_first` where one was sent and averaged over both batting orders where none was), `toss_marginalised` saying which of those two it is, `objective_probability` (always marginalised — the objective's model has no batting-order feature), and `team1_constraint_check` / `team2_constraint_check` where constraints were sent — the eleven's size, its bowler count by the optimiser's own definition, whether it holds a keeper, the `must_include` ids it does not hold, and whether it `met` them all.
- **Every prediction response** (`/xi/optimize`, `/xi/predict-win`, `/simulate`, `/performance/predict`) carries `served_ratings: {run_id, ratings_through}` — the run the answering store was loaded from and the last match date its ratings include, read off that store (P1-5). A live request against ratings older than `ml.ratings_max_age_days` is **503 `RATINGS_STALE`**, the message naming the date, the age and the limit and the hint the step that fixes it (H-11).
- **POST /performance/predict** — Same body. Response: per player `p_bats`, `p_bowls`, the 0.1 / 0.5 / 0.9 quantiles of `runs`, `balls_faced` and `runs_conceded`, the wicket distribution (`expected`, `p0`, `p1`, `p2_plus`) and `catches_expected`; `innings_marginalised` is true when the toss was unknown and both batting orders were averaged. `venue_context` (`venue_bf_rate`, `venue_n`, `neutral`) is the ground as the model read it, off the rows it consumed — the two columns the performance model reads a ground through and the only two, since A-1's scoring-level families were nulled and `FIXTURE_CONTEXT_FAMILIES_KEPT` is empty; `neutral` is true at `venue_n` 0, where the rows read at the prior and a caller must say so rather than present a substitution nobody can see (§8.7, P3-2).
- **POST /xi/player-roles** — Body: `format`, `player_ids` (1–500 registry ids). Response: `format`, `players` (per id `player_id`, `known`, `roles`), `unknown_player_ids` and `served_ratings`. The two roles are the objective's own constraint predicates read off the served as-of vectors (`ml.xi.roles`: `keeper` is a player the state has credited with a stumping, `bowling_option` is one whose expected balls bowled clear `contract.MIN_BOWLING_BALLS`), extracted so this read and `/xi/optimize`'s `selection_reasons` cannot drift — a test pins them equal on the same ids. An id the served state has never seen comes back `known: false` with no roles and is named in `unknown_player_ids`; no role is invented for him (§8.7). It is a live request, so H-11 refuses the whole read past the freshness limit. It answers for players who are in no eleven, which is what the auction module (P3-1) needs and what `selection_reasons` cannot give: that reports only on players a search already picked. Nothing in it evaluates the objective, orders a pool or scores an eleven.
- **POST /simulate** — Same body plus `n_samples` (default 2000) and `seed`. Response: per side the total (`q10`, `median`, `q90`, `mean`, `sd`, `scorecard`), extras, wickets lost, and per player ranges plus the median-band `scorecard` line and `spread_share`. Per player `p_bats` / `p_bowls` are L2-B's forecasts the draws were made from — the same numbers `/performance/predict` returns for the eleven and toss (marginalised half each way when the toss is unknown) — and `batted_share` / `bowled_share` are the share of draws that realised them. The two differ by the simulator's own dynamics (the deliveries budget and the chase end an innings before the forecast's depth; the bowling draft tops bowlers up), and both are served so the difference is visible rather than substituted (SERVE-05, §8.7); `win_probability` carries `simulated`, `display`, `headline` and `headline_source`. With `return_total_draws` (default false, P3-2) each side also carries `total_draws`, its total in every draw: a caller pooling several grounds needs the draws themselves, because a mixture's quantiles are not the mean of its parts' quantiles. **422 `SIMULATION_UNSUPPORTED_FORMAT`** for a format with no innings length.
- **GET /xi/status** — Loaded formats, `ratings_through`, player count, the run's own training
  report, and — since P-6 — `run_id` and `manifest` (H-16: run id, cutoff, dataset sha, git sha,
  the hyperparameters the grid chose, the run's headline metrics), `ratings` (H-11: `fresh`,
  `age_days`, `max_age_days`, `code`) and `error` (D-6: why a run on disk was refused).
- **GET /xi/evaluate-report** — L4's `xi_evaluate_report.json`. **503** with a hint to run `make evaluate` when the harness has not run.
- **GET /xi/metric-glossary** — What every reported metric means (L-1): `{"entries": {<metric key>: {key, name, explanation, band, better, direction, scale}}}`, from `ml/xi/glossary.py`. `scale` is the band as two numbers — `{bad, good}`, read through `direction` — for a surface that paints a value red-to-green instead of printing the sentence; it is `null` for a metric with no defensible anchor, which is why an interval width shows uncoloured. Always 200 — it is served from the code, so a surface can explain its numbers before any run exists.
- **GET /artifacts/status** — Every run on disk, newest first, with `current_run`, `loaded_run`,
  the ratings verdict and the loader's refusal. Each run carries its manifest summary, including
  `ratings_through` — the date its data runs through, answered without loading it (P2-2) — and
  `refused`: `null` on a loadable run, the reason on one that cannot be loaded (§8.7). A run
  directory with no manifest is listed as `has_manifest: false` rather than hidden, and a
  manifest written before `ratings_through` existed is listed with that as its `refused` reason:
  both are exactly what an operator is looking for when nothing loads.
- **POST /admin/reload?run=<id>** — Point `current` at a run and load it. Without `run`: the
  newest run on disk, which is the one the retrain before it built; naming a run is how you roll
  back to an earlier one. **409 `RUN_ARTIFACTS_INVALID`** when the run is
  not one, or when its arrays are not the arrays this code reads (D-6); **403 `RELOAD_DISABLED`**
  when `ENABLE_HOT_RELOAD` is off.
- **POST /admin/train/retrain?cutoff=...** — Build one run. **400 `CUTOFF_REQUIRED`** without a
  cutoff. It publishes nothing; `/admin/reload` does that.
- **POST /admin/train/evaluate** — Run L4 and write its report. Touches no artifact `current`
  points at.
- **One run per step.** A second `retrain` while a `retrain` is running is refused **409
  `TRAIN_ALREADY_RUNNING`**, and the message names the run it is refusing for — the step, its
  pid and how long it has been going — with the hint saying to wait or to
  `POST /admin/train/stop?step=<step>` (§8.7). Two runs of one step are not merely two handles
  in a registry: they write the same cross-run data-quality baseline at the artifacts root
  (H-15), and progress and results are published per *step*, so `/admin/train/progress` and the
  summary on the response would carry whichever run wrote last (SERVE-06). `retrain` and
  `evaluate` share no such file and may overlap.
- **POST /admin/train/stop?step=** — Stop the training subprocess of one step, or of every step.
  200 with `stopped` naming the steps whose process this call signalled and watched exit — empty
  when nothing was running, which is a normal answer. A run that finished on its own before the
  signal landed is not in the list and keeps the outcome it earned: reporting it as stopped
  would answer its caller **409 `TRAIN_STOPPED`** for a run that succeeded (SERVE-06).

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
- **GET /api/predictions**, **GET /api/predictions/{id}** — the prediction record: every
  answer this API has issued, as it was served (P2-3). See the section below.
- **GET /api/auctions**, **POST /api/auctions**, **GET /api/auctions/{id}**,
  **POST /api/auctions/{id}/players**, **POST /api/auctions/{id}/outcomes** — the auction
  record (P3-1). See the section below.
- **PUT /api/auctions/{id}/assumptions**, **GET /api/auctions/{id}/opposition-suggestion**,
  **POST /api/auctions/{id}/projection** — the projection's named assumptions and a
  candidate's projected output per ground (P3-2). See the section below.
- **GET /api/players/search** — a cross-club player search by name prefix. See the section below.
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
team 1 bats first, absent where it is unknown and both orders are read, P1-1), and
`team1_xi` / `team2_xi` (Play mode, P1-2).

**A named toss changes the answer (GO-07).** Sending `team1_bats_first` is not a display
preference: the win probability and every per-player forecast are then read at that batting
order instead of averaged over both, which in TEST moves the headline by 0.04 on average and
by up to 0.14. What it does *not* change is the eleven: the optimiser maximises the XI-only
objective, whose model reads per-side aggregates over eleven and has no batting-order feature,
and the constraint checks count roles. The response's `toss.reading` names which of the two
quantities came back, and its `note` names the toss-blind part.

**A named venue is resolved or the request is refused (GO-08).** `venue` is optional: leave
it out and the fixture is read without one, which the answer says in
`venue: {"resolved": false, "note": ...}`. Send one and it is looked up by its exact name —
the string `/api/options/venues` offers — and the answer names what was used in
`venue: {"resolved": true, "venue_id": …, "name": …}`. A name this database does not hold is
**`400 VENUE_NOT_FOUND`**. It used to be none of those: the lookup was a *get-or-create*, so
a typo inserted a venue row and the prediction ran at a ground with no history behind it,
while a database failure fell through to no venue at all — three different things reaching
the caller as one 200 with nothing on the wire about any of them.

**A `match_date` is a calendar day, in whatever zone it is written (GO-09).** An
offset-bearing value is read as the day the caller wrote: `2025-03-01T01:00:00-05:00` is the
first of March everywhere downstream — the pool's cutoff, the `as_of`, the `match_date`
ml-service reads, and the date the issued prediction is stored under. It used to be
truncated to a multiple of 24 hours, which is midnight UTC of a *neighbouring* day, so that
request bounded the pool at 28 February and silently dropped the previous day's matches from
the window while the stored record kept the first of March.

**A played match is a backtest (GO-01).** A `match_date` before today (UTC) is sent to
ml-service as `as_of` on every call the prediction makes, so the sides are rated on ratings
that stop strictly before the match — never on a state that already contains its result —
and the retirement ledger, which describes who is available *now*, is not applied to that
pool. A match today or later is a live request: no `as_of`, the through-today state, and
H-11's freshness refusal. The first past-dated request after a reload pays for an as-of
replay of the event store; requests in ascending date order share one pass. What the several
calls of one fixture are served is a *snapshot* of that pass, taken for the date they name
and shared between them: the pass keeps advancing as the backtest walks forward, and a
request reading the pass's own state would watch its ratings move under it (SERVE-01).

**Play mode: scoring an eleven the caller built (P1-2).** `team1_xi` / `team2_xi` name each
side's eleven by `player_id`. Sent, the selection step is skipped and exactly those players
are scored; omitted, the XIs are selected as they always were. Everything after the
selection is the same code either way — the displayed probability is still
`/xi/predict-win`'s and the totals, ranges and scorecard are still `/simulate`'s — so a
re-score and an Optimise for the same eleven return the same numbers. A pinned player joins
his side's pool whatever the window or the ledger says, the way a must-include id does.

Five refusals, because a repaired eleven is not the eleven that was sent: a side that is
not a full XI is **`400 XI_INCOMPLETE`** (every model here aggregates a whole side, so a
ten-man side would be a prediction for a match nobody plays), an id that names no player is
**`400 XI_PLAYER_UNKNOWN`**, one player named for *both* sides — pinned in both elevens, or
must-included on both — is **`400 XI_PLAYER_ON_BOTH_SIDES`** (GO-04: nobody plays both
elevens, and taking him off one of them is a change the user can see, so it is theirs to
make), and naming the same player twice or pinning one side while leaving the other to be
searched is a `400` naming what is wrong. Two player rows that carry one `external_id` are
one player to every model here, so pinning both is refused on the same ground.

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

**One player, one side (GO-04).** Both pools are built from one player table by club, so a
player who moved clubs inside the window is in both of them — franchise T20 with a
twelve-month window is the ordinary case, not an edge one. He is in exactly one side's pool
by the time anything is scored, and which one is decided in this order: a player the caller
named for one side (a `team1_xi` / `team2_xi` pin, or an `extra_team1` / `extra_team2`
must-include) stays there; otherwise the club he appeared for more recently keeps him; an
exact tie, including two sides that both entered him by id with no appearance at all, keeps
him with team1. Named for *both* sides, the request is refused (`XI_PLAYER_ON_BOTH_SIDES`)
rather than resolved.

The drop is never silent. He is in the losing side's `team1_pool` / `team2_pool`
`excluded` list with `reason: "both_sides"`, his last appearance for that side, and a
`detail` naming the side that kept him and the date it kept him on — the same shape the
retirement ledger's exclusions take (§8.7). It is not a ledger flag, so there is nothing to
undo: a caller who disagrees picks the candidates by hand (`team1_pool.players`), which the
detail says. Dropping him can take a pool under eleven, and that is `POOL_TOO_SMALL` with
its usual two ways out. The opposing eleven also reaches `/xi/optimize` as `must_exclude`,
which changes no answer while the pools are disjoint and states the rule where the search
can see it.

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
| `toss` | Which of the two readings the probabilities in this answer are: `team1_bats_first` (null where it was unknown and both orders were read), `reading` (`toss_aware` / `marginalised`) and, on a toss-aware answer, a `note` naming what in it stayed toss-blind. The two readings are different numbers — 0.04 apart on average in TEST and up to 0.14 — so the answer names its own (P1-1, GO-07, §8.7). `reading` is derived from what each model reported having done, not from the request: a model that answered a batting order other than the one asked for is refused, not relabelled |
| `scorecard` | Present only for a format with an innings length: `samples`, `toss_marginalised`, and `team1_innings` / `team2_innings` — named by side, not by batting position — each with the median-band `total`, its `extras` and the 10-90 range of the draws |
| `constraints` | Present only where the caller pinned the elevens (P1-2): `team_size`, `min_bowlers` and `require_keeper` as they were asked for, and per side the `size`, the `bowlers` count, `has_keeper`, any `missing_must_include` players and whether the eleven `met` what was asked. The counts are ml-service's, measured on the eleven that was scored — nothing is repaired to satisfy them |
| `team1_pool`, `team2_pool` | Which candidates each XI was chosen out of: `source` (`recency_window` / `all_time` / `manual`), the `window_months` and `since` it applied, the `size` it produced, and `retired_excluded` with the `excluded` players themselves — each with `reason` (`retired` / `user_flagged` / `both_sides`) and `detail`. `retired_excluded` counts the ledger's exclusions only; a `both_sides` entry is the other side keeping a player both pools held (GO-04), and there is no flag behind it to undo |
| `record` | Whether this answer went on the prediction record (P2-3): `stored`, and either the row's `id` and `issued_at` or the `reason` it was not stored. Always present |

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

**Every issued prediction is stored (P2-3).** A successful answer is written to
`issued_prediction` *before* it is served, and the answer names the row it was written to:
`record: {stored: true, id, issued_at}`. What is stored is the answer itself, whole — the
same bytes the caller received, `record` block and all — beside the request it answered, the
`run_id` and `ratings_through` it was served from, and the columns a resolver joins on
(format, both opposition ids, gender, `match_date`, the selection objective, the headline
probability and its source). The two opposition ids are club ids — written as such, and
read back folded through `COALESCE(canonical_id, id)` so a club renamed after the answer
was filed is still the same club to a reader (GO-02). A refused prediction stores nothing:
a refusal is not a prediction, so there is nothing to score against a result later.

A store failure never costs the caller the answer. The prediction is correct whether or not
it was filed, so it is served with `record: {stored: false, reason: "..."}` — on the wire of
the answer it failed to record, never only in a server log (§8.7), and the Lab shows it
beside the served date. The alternative — refusing — would turn a bookkeeping outage into an
outage of the only thing the product does, and would put a new single point of failure in
front of a path that has none.

**Retired fields are refused, not ignored:** `weather`, `simulate`, `use_reconciled_scorecard`
and `include_both_scorecards` each return 400 with a code and a hint. A caller still sending one
would otherwise get an answer to a different question with no indication why.

---

## The prediction record

Every answer `POST /api/predict/team-selection` has issued, as it was served (P2-3). It is
the ground truth P2-4's track record is scored from: a prediction names a rating state that
the next retrain replaces, so once `run_id` is no longer the loaded run nothing in this
system can reproduce a row here. That is the opposite of the cache migration `0007` dropped
(`match_prediction_aggregates`, predictions *about played matches*, all of them
recomputable).

**`GET /api/predictions`** — the record, newest first, paged with `limit` (default 20,
capped at 200) and `offset`. Returns `{predictions: [...], total, limit, offset}`, where
`total` is the whole record's count so a surface can say "20 of 143". Each row carries the
join columns and no payload: `id`, `issued_at`, `run_id`, `ratings_through`, `format`,
`team1_opposition_id`, `team2_opposition_id`, `gender`, `match_date`, `objective`
(`win` / `ratings` / `fixed`), `win_probability_team1` and `win_probability_source`. A
`limit` or `offset` that is not a whole number is `400 INVALID_PARAM` rather than a silent
default — a page you did not ask for looks exactly like the page you did.

**`GET /api/predictions/{id}`** — one stored answer: those same columns plus `request` (the
request as this API parsed it, so the POST and GET forms are recorded identically) and
`payload` (the answer, whole). An id the record does not hold is `404 PREDICTION_NOT_FOUND`
naming it. Ids come from a prediction's own `record` block.

`payload` is a `jsonb` column, which keeps the JSON *value* exactly — every field, every
number, every null — and normalises only insignificant whitespace and key order. The answer
is encoded once, and those bytes are both what the store keeps and what the caller receives,
so reading a prediction back reproduces what was served.

**Play mode is on the record too.** A re-score is a served answer, and P2-4 counts what this
store holds, so leaving re-scores off it would make the count dishonest by omission.
`objective` is what tells the two apart: `fixed` is an eleven the caller built — a scenario,
which the track record lists and never scores — and `win` / `ratings` is an eleven this
service chose. Everything else a reader might segment on (the venue, the toss, the pool
scope, the pinned elevens) is inside the two stored documents, so no new column is needed to
ask a new question of the record.

Since P2-4 each row also carries `simulator_shared_factor` (migration `0014`): whether the
simulator that served the answer carried a shared match factor, read off `/simulate`'s new
`shared_factor` field and the answer's own `scorecard.shared_factor`. NULL on rows stored
before the column existed and on answers no simulator served (a format with no innings
length); the track record tells those two apart from the payload.

---

## The track record

**`GET /api/track-record`** — the prediction record scored against what happened (P2-4),
computed on every read from `issued_prediction` and the match tables. There is no column, no
pipeline step and no scheduler: an import that brings the match a prediction was about
moves it to `scored` on the next read, with no operator step. There is no page and nothing
to configure — which forecast of a fixture is the last one issued is a question about the
whole record, so the whole record is read.

**Every stored prediction is in exactly one state** (`prediction_states` in the contract,
asserted from go-app and the frontend):

| state | meaning |
|---|---|
| `scenario` | a hand-built eleven (`objective: fixed`): listed, never scored, because the caller built a side that may not have played |
| `superseded` | an Optimise forecast of a fixture that a later Optimise of the same fixture, issued on or before the match date, replaced — one forecast is scored per fixture, the last one issued; the row names the id that superseded it |
| `unresolved` | the database holds no match for the fixture yet (Cricsheet lag, an import not run), shown with `days_past_match_date` (negative before the match); also a double-header the record cannot tell apart, with the reason in `state_note` |
| `no_result` | the match was played with no `outcome_winner_opposition_id` (no result, draw, or a tie nobody broke — a tie settled by a super over or a bowl-out has a winner and is scored): counted, not scored |
| `post_hoc` | the match was played and won by someone, but the forecast was issued **after** the match date: listed and counted, its own `score` on the row, and in none of the summaries below |
| `scored` | the forecast was issued on or before its match date and the match was played and won by someone — the rows every summary is over |

A fixture is resolved by the **exact** `match_date`, both clubs in either order, the
format code and the gender. A match between the same sides on a neighbouring date is a
different match and is not it (`db.MatchLookup`).

**A club that renames is one club here (GO-02).** Cricsheet names a team by whatever it
was called on the day, so a rebrand splits a club into two `opposition` rows and each
match keeps the row that played. Every opposition id the record handles is therefore
folded onto its club — `COALESCE(opposition.canonical_id, id)`, the same rule the player
pool uses — on **both** sides of the join: the lookup folds the match's sides, and the
store folds the two ids of a stored prediction as it reads them, because a club can be
renamed after an answer was filed. Matching raw ids instead would leave a renamed club's
fixture `unresolved` forever, and where one side happened to match would score the other
half the wrong way round: `team1_won` inverted, no `team1_total`, `team1_batted_first`
reversed, an `eleven_overlap` of zero. The ids on the wire — `team1.id`, `team2.id` and
`happened.winner_opposition_id` — are club ids for the same reason.

A forecast issued after the day it was
about neither supersedes nor is superseded: it stands alone in `post_hoc`, flagged
`issued_after_match_date` on the wire, with its score on the row and no summary over it.
The ratings behind such an answer can already contain the result — every row stored before
*A played match is a backtest* (GO-01) was served from a through-today state — so counting
its Brier or its coverage would be hindsight scoring itself (GO-03). It is excluded from
the aggregates, never hidden: the row shows what it claimed and what happened, like every
other row, and the state counts hold it.

**The scores**, over the `scored` predictions only, every one with its `n`:

- `win.overall` and `win.by_format`: `n`, `brier` (the served headline probability against
  the result), `base_rate` (the scored rows' own team1 win rate) and `base_rate_brier` (the
  Brier of predicting that rate for every row — the score to beat, from the same rows);
  `win.reliability` is the harness's curve in its ten equal-width bins (`reliability_bins`)
  with `n` per bin, empty bins omitted. Null over no rows; an empty record is an empty list.
- `coverage.rows`: one row per (`format`, `population`) with `first_innings` and `chase`
  each `{n, covered, coverage}` — whether the actual innings total fell inside the served
  10–90 range, inclusive at both ends, labelled by the innings actually played. **There is
  no pooled row.** `population` is one of `simulator_populations`: `with_shared_factor`,
  `without_shared_factor`, `unknown` (a simulated answer stored before migration `0014`),
  `not_simulated` (no scorecard); `coverage.populations` counts the scored predictions in
  each, every population present, so the denominators are visible (B-12). The ranges are the
  simulator's as served: no widening, no day/night adjustment (B-11 is open and the record
  shows it).
- `elevens`: over the scored predictions, how many of the named players took the field for
  the side they were named for — `mean_overlap`, `min_overlap`, `max_overlap`, `complete`
  (every named player played) — reported beside the scores and never used to exclude one.
- `predictions`: every row, newest first, with `state`, `claimed` (the probability and its
  source, the predicted winner, both served ranges, players named per side), `happened`
  (match id, winner, both totals in the prediction's orientation, who batted first) and
  `score` (Brier, whether each side's total was in range, the eleven overlap per side). A
  stored payload the record cannot read keeps its row with `payload_error` on it.

The arithmetic is the harness's (`ml/xi/sim_harness.py`: `_brier`, `reliability`, the
inclusive interval test), re-stated in Go because it is three lines each and the record is a
go-app read of go-app's tables; `ml-service/tests/fixtures/track_record_reliability.json` was
generated by the harness's functions and both sides assert against it, so the two cannot
drift apart unnoticed. `track_record_metric_keys` in the contract lists the L-1 keys the
record labels its numbers under (`brier`, `record_base_rate_brier`, `reliability`,
`coverage_80`, `eleven_overlap`); ml-service asserts every one has a glossary entry and the
frontend asserts the tab renders no other.

What it is not: evidence. Tens of predictions is a diagnostic; no number on it is a verdict,
nothing turns red at a threshold, nothing is filtered by outcome, and `make evaluate` remains
where a choice-facing number comes from. Frontend: the Track record tab, which shows the
harness's figure for the same format beside each of the record's, labelled by the window it
came from (the locked window where it was scored, otherwise the walk-forward fold means, and
B-12's calibrated-only coverage beside the pooled one where some fold had no factor).

---

## The auction record

**P3-1.** An auction is a list of players, each still available, sold (to whom, for how much)
or unsold; the buyer's own squad and open slots; and the format and the grounds it is for.
Nothing in the schema held one, so migration `0015` adds three tables (`auction`,
`auction_venue`, `auction_player` — see [config-and-data.md](config-and-data.md)) and these
endpoints enter and read them. **Every row is a fact the operator typed during a live
auction.** Nothing is fetched: a live auction feed is a paid or account-gated source and is
not built.

**The rule the module is built on: it is valuation and projection, never XI-picking.** The
system's own record is that in T20 optimised selection is indistinguishable from rating
order (plan §8.8) and the IPL is domestic T20, so **no auction endpoint calls `/xi/optimize`,
returns a win probability or returns a marginal value** — asserted in a test through the
client, not by inspection — and the Auction tab carries that sentence where the numbers are.

- **`POST /api/auctions`** — Body: `name`, `format` (one the simulator serves: every later
  Phase 3 item projects a total, and a total needs an innings length), `buyer_club_id` (the
  side whose squad this fills), optional `venue_ids`, `squad_size`, and the eleven's
  constraints `min_bowlers` (default 5) and `require_keeper` (default true) — the same two the
  predict path takes, so an open slot means the same thing on both surfaces.
- **`GET /api/auctions`** — the index, newest first, without the lists: how an operator finds
  their auction again after a reload.
- **`GET /api/auctions/{id}`** — the auction whole.
- **`POST /api/auctions/{id}/players`** — Body `player_ids`. Lists players who are not on the
  list yet; one already listed keeps the state he is in, because re-adding a sold player is a
  double-click and not an instruction to forget the sale.
- **`POST /api/auctions/{id}/outcomes`** — Body `player_id`, `state` (`available` | `sold` |
  `unsold`, the vocabulary declared in `contracts/ops-console.contract.json`), and on a sale
  `buyer_name`, optional `buyer_club_id` and `price`. A sale needs a buyer and a price and
  anything else may carry neither; an undo is this call with `available`, and it clears the
  buyer and the price with the state. **404 `AUCTION_NOT_FOUND`** for an auction nobody created
  or a player nobody listed.

**Every write answers with the auction as it now stands**, because that is what the
operator's next decision is made against: `auction` (the record), `squad`, `slots`,
`distribution` and `roles`.

**The roles are the model's, and they are stamped.** On every read, go-app asks ml-service's
`POST /xi/player-roles` for the listed players and puts `roles: {known, roles}` on each row,
with `roles.run_id` and `roles.ratings_through` beside them. A player who answers neither
predicate is a **batter by elimination** — the label means exactly that, and the L-1 glossary
entry says so; a player the served state has never seen is reported unknown with no role
invented for him. `player.is_wicket_keeper` is the *database's* name-set flag and is not the
model's role; it appears only on the player search, named as the database's flag.

**A refused role read does not refuse the auction** (§8.7). The record is facts the operator
typed and reading it back needs no model, so a stale registry leaves the list exactly as
entered, drops `distribution` and `slots.by_role` rather than showing zeroes, and names the
refusal on the wire: `roles: {available: false, code: "RATINGS_STALE", message, hint}`.

**`GET /api/players/search`** — a cross-club player search: `q` (a name prefix, at least two
characters, matching the start of the name or of any word in it), optional `format` and
`limit` (default 25, max 100). Per player: the registry-backed row, `is_wicket_keeper` as the
database's flag, `clubs` (most recent first), `formats`, `last_played`, and the retirement
ledger's verdict. It exists because `GET /api/options/candidates` is per club — a prediction
is about one side — and an auction room is not one side. It applies no recency window and
removes nobody: the ledger's opinion is shown beside a name rather than instead of one.

**Nothing here is on the track record.** `issued_prediction` (P2-3) holds forecasts of
fixtures, which P2-4 scores once the match is imported. An auction record holds what was
entered and what was shown, which is a different thing and cannot be scored: no auction
outcome data exists in the system to score a valuation against.

---

## The auction projection

**P3-2.** What a candidate is projected to produce in a named eleven, against a named
opposition, at the auction's grounds. Migration `0016` adds the two assumptions to the
record (`auction_likely_xi`, `auction_opposition`, `auction_opposition_player`).

**The three inputs are assumptions, and every answer names all three.** The eleven a
candidate would join is a guess; the opposition is a guess; the grounds are the auction's.
None is a fact months before a fixture exists, so each is held on the record, carried back
on every projection, and shown on the surface as an assumption. There is no default
opposition: a projection against a silently neutral side would be a projection for no
league (§8.7).

- **`PUT /api/auctions/{id}/assumptions`** — Body: `likely_xi` (up to eleven player ids) and
  `opposition` (`club_id` and eleven `player_ids`). Each is optional and an absent one is
  left exactly as it stands, because the operator names the opposition once and edits the
  likely eleven all through the auction as their squad fills; each named one is *replaced*
  rather than merged, since merging a shorter list into a longer one would leave the record
  holding a player just removed. The opposition carries a side and not only eleven names
  because the performance model reads a ground **only through the team context** —
  `ml.xi.rows.team_context_or_neutral` falls back to neutral for the *pair* and takes the
  venue with it — so an opposition with no `club_id` would make every ground read alike.
- **`GET /api/auctions/{id}/opposition-suggestion?club_id=`** — the eleven this database
  records that side last fielding in the auction's format, with the match it came from
  (`from_match.match_date`, `event_name`, `venue_name`), as a starting point to edit.
  Only a match whose recorded side holds a full eleven is offered — a partial team sheet
  would seed an assumption with a hole in it. **404 `NO_FIELDED_ELEVEN`** where the database
  records none, rather than a side assembled by rating: an invented opposition presented as
  a starting point would be a guess wearing evidence's clothes.
- **`POST /api/auctions/{id}/projection`** — Body: `player_id` (a candidate on this
  auction's list), optional `team1_bats_first` (absent is the toss unknown and the model
  marginalises over both batting orders), optional `venue_weights`
  (`[{venue_id, weight}]`).

**One row per ground, with two intervals told apart by name.** Each row carries the
candidate's `runs`, `balls_faced` and `runs_conceded` as `q10/median/q90` from
`/performance/predict` — `interval_source: "l2b_quantiles"`, at nominal coverage on the
harness (plan §8.2) — and his `wickets` as an expectation with `p0`, `p1`, `p2_plus` and
**no interval at all**, because L2-B has no quantile heads for wickets on this path and one
derived from the probabilities would be an interval the model never made. Beside them
`eleven_total` is the eleven's total with him in it from `/simulate` at the served draw
count, `interval_source: "simulator_draws"`, with his `spread_share` over the same draws.
`intervals` names both sources once with what each is, and the simulator's carries
`caveats: ["B-11", "B-14"]` — **B-11**, the interval is too narrow by day and too wide at
night (T20 first-innings coverage 0.734 / 0.841 at a nominal 0.80, six gated arms nulled),
and **B-14**, the performance artifact is not shape-checked at load so an older calibration
can restore silently: the run id shown is the run that answered and not a promise about its
calibration. Nothing is widened, narrowed, adjusted for day or night, or hidden.
`auction_interval_sources` in `contracts/ops-console.contract.json` is that vocabulary,
asserted from both sides (H-24).

**What a ground changes, said on the answer.** The performance model reads a ground through
two columns and nothing else — `venue_bf_rate` and `venue_n` — because the ground's scoring
level was gated and recorded as a null (A-1: population mix, not venue) and
`FIXTURE_CONTEXT_FAMILIES_KEPT` is empty. So the rows differ by what the toss does at each
ground and by nothing else about it, and `assumptions.what_a_ground_changes` says so. Each
row carries `ground: {bat_first_rate, matches, neutral, note}` off the rows the model
consumed; a ground the served state has no matches at reports `neutral: true` with a note
saying it read at the prior, rather than being shown as a projection at a ground the model
knows (§8.7).

**The mixture is over draws, never over quantiles.** With `venue_weights`, `mixture` is the
eleven's total over the named grounds, inverted from the simulator's drawn totals pooled
with those weights (`/simulate` is asked for `return_total_draws`). A mixture's quantiles
are not the mean of its parts' quantiles, so three per-ground summaries cannot be combined
at all; without weights there is no mixture, because how often an eleven plays where is a
fact nobody has entered.

**No win probability, and no marginal value.** `/simulate` answers a `win_probability`; the
Go type this endpoint maps it into has no such field, so the value never exists in this
process and cannot reach a surface by accident. A test walks every key of the rendered
payload at any depth and refuses one containing `win` or `marginal_value`, and asserts the
endpoint's requests reach `/performance/predict` and `/simulate` and never `/xi/optimize`.
A P(win) beside a purchase is the XI-picking claim in another coat.

**Refusals, each showing no number.** **400 `XI_INCOMPLETE`** where the likely eleven with
the candidate in it is not eleven — a ten-man side, and equally a full eleven plus an
outside candidate, which is twelve men — exactly as the predict path refuses one. **400
`ASSUMPTIONS_INCOMPLETE`** where the likely eleven, the opposition or the grounds are
unnamed. **400 `XI_PLAYER_ON_BOTH_SIDES`** where the likely eleven and the opposition name
one player between them: both elevens are the operator's own, this module has nothing to
pick a side with, and dropping him from one would answer for an eleven nobody named
(GO-04). **400 `XI_PLAYER_UNKNOWN`** where a named player carries no registry id; the ten
who resolved are not scored, because that would answer for an eleven nobody named. **404
`CANDIDATE_NOT_LISTED`** for a player this auction does not hold. **409
`SERVED_RUN_CHANGED`** where a reload landed mid-assembly. **503 `RATINGS_STALE`** past
H-11's limit — the record still reads back whole, because it is facts the operator typed.

**The cost, measured and not assumed** (dev stack, 30 timed repeats after 5 warm-ups, 2000
draws): **312 ms median / 358 ms p95 per candidate per ground**, and 967 ms / 1215 ms for
one candidate over three grounds. Of one ground, ml-service is 138 ms
(`/performance/predict`) plus 174 ms (`/simulate`); `return_total_draws` and the mixture are
free inside the noise. **The forecast cannot be batched over candidates.**
`ml.xi.rows.player_feature_rows` builds each row's `own_*` columns from `aggregate_side`
over the whole side it is handed, so listing N candidates on one side would project each
into a side of 10 + N rather than the eleven he would join, and `/simulate` draws exactly
the innings it is given. There is no cache: if one is warranted that is P3-5's decision,
made with these numbers.

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
(connected, migration status/current/expected, counts, `team_lineage`), `dataset` (the directory Import reads,
its manifest and match-file count), `artifacts`, `fielding` (available, rows), `freshness`,
`db_completeness`, `pipeline` (per step: running, completed, runnable, optional),
`suggestions[]`.

**`db.team_lineage` says whether renamed clubs are one club (IMPORT-07).** It is read off the
archive rather than off the last import's log, because an import that stopped before settlement
wrote no links and logged nothing:

- `status` — `ok`, `incomplete`, or `unknown` (the mapping or the database could not be read).
- `renames_configured`, `linked`, `absent`, `unlinked`, and `unlinked_renames[]`, which names
  them (`"Delhi Daredevils -> Delhi Capitals (male)"`).
- **The bad value is `incomplete`**, i.e. `unlinked > 0`: both rows of a rename are in the
  archive and `opposition.canonical_id` was never written, so that club is two clubs with two
  Elo histories, two form series and two head-to-head records, and no other surface says so.
  The remedy is to re-run the import; linking is idempotent. `absent` is not a fault — the
  mapping describes cricket, and a dataset may stop before a club renamed.

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
`created_at`, `cutoff`, `ratings_through` (the run's data date, off its manifest, P2-2),
`git_sha`, `dataset_sha`, `formats`, `has_manifest`, `refused` (`null`, or why the run cannot be
loaded), `current` and `loaded`. For the loaded run, `ratings_through` in its row equals the
top-level `ratings_through` and the stamp on every served prediction — the loader asserted it.
It reports runs because a run is what an artifact belongs to now (H-16): "is the model
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
`has_manifest: false` and that a reload of it answers 409; copy a run and delete
`ratings_through` from the copy's manifest and recheck that it is listed with `refused` naming
the field and that a reload of it answers 409 naming the run.

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
