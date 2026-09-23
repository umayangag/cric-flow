# Configuration and data

go-app and ml-service configuration and Cricsheet import. Model inputs:
[ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

**Precedence:** CLI/flags → env → component `config.json` → built-in defaults.

---

## Go application (go-app)

Config file: `go-app/config.json`

**Keys:**
- `server` (optional) — API/timeouts and URL fallbacks. All values have built-in defaults; omit or set 0 to use them.
  - `ml_health_timeout_sec` (default 10) — ML health proxy request timeout.
  - `ml_client_timeout_sec` (default 20) — ML client (predict/train) HTTP timeout.
  - `ml_health_body_limit_bytes` (default 1048576) — max ML health response size (DoS protection).
  - `ml_base_url_fallback` (default `http://localhost:8000`) — used when `ML_SERVICE_URL`/`ML_BASE_URL` are unset.
  - `readiness_timeout_sec` (default 2) — DB ping timeout for readiness probe.
  - `train_step_timeout_min` (default 30) — max wait for ML train endpoint (e.g. 10080 = 7 days for long training pipelines).
  - `pipeline_progress_interval_sec` (default 2) — SSE progress poll interval.
  - `db_probe_timeout_sec` (default 2), `db_probe_long_timeout_sec` (default 5) — ops DB probes.
  - `artifacts_timeout_sec` (default 3) — HTTP client timeout for artifacts check.
  - `http_read_timeout_sec`, `http_write_timeout_sec`, `http_idle_timeout_sec` (defaults 15, 30, 60) — HTTP server timeouts.
  - `listen_address` (default `:8080`) — overridden by `PORT` env.
- `inputs`
  - `cricsheet_dir` — the directory Cricsheet JSON is imported from. Used by
    cricsheet-importer, by `POST /import/cricsheet`, and reported in `/ops/status.dataset`.
    `GO_APP_CRICSHEET_DIR` overrides it. Import does **not** recurse: the `.json` files
    must be directly in this directory, and an import that finds none fails rather than
    reporting success with zero rows.

    A `_staging/` subdirectory under it holds downloaded archives (`POST /ops/data/fetch`).
    It is invisible to import, which skips directories and does not recurse, so a
    half-downloaded archive can never be mistaken for match data.
- `outputs`
  - `dir` — go-app's own output directory. It held the export CSVs until P-6 deleted them;
    what is left is the resource-observation file the import pipeline's concurrency is
    derived from. `GO_APP_OUTPUT_DIR` overrides it.
- `pipeline` (optional)
  - `import_timeout_ms` (int) — how long an import may run (default 86400000). 0 = no deadline.
  - `import_concurrency` (int) — 0 = auto. With a memory limit set (GOMEMLIMIT, or cgroup v2
    including Docker/K8s via `/proc/self/cgroup`), workers are sized so total usage stays at
    about **85 %** of it, at 150 MB per import worker; with no limit detected, `NumCPU()`.
    Import is the only pipeline go-app runs workers for — precompute, export, seqcalc and
    fielding went with their pipelines in P-6.
- `ops` (optional) — `migrations_page_default` (10), `migrations_page_max` (100), `migrations_page_cap` (10000), `recent_migrations_count` (100).
- `resources` (optional) — `import_mb_per_worker` (150), `memory_usage_fraction_percent` (85).

**Environment:** `GO_APP_CONFIG`, `GO_APP_INPUT_DIR`, `GO_APP_OUTPUT_DIR`, `IMPORT_CONCURRENCY`.

**Team selection** (under `team`): `min_bowlers`, `default_batters`, `default_bowlers`. Under
`selection`: `max_win_prob_eval_budget` (500 — how many XIs ml-service may score per side per
round) and `best_response_rounds` (3 — how many times the two sides answer each other).

There is nothing else left to configure about selection. The objective, the search and the
constraints all live in ml-service, and the settings that used to weigh a batting score against
a bowling one were retired in P-5 — as the `features.*`, `export.*`, `backtest.*`, `weather.*`
and precompute/seqcalc/export/fielding keys were in P-6. A `config.json` that still carries any
of them fails `ValidateForServer` with a message naming what replaced it
(`internal/config/retired_keys.go`), because encoding/json drops an unknown key and nothing
else would ever notice.

**Future-match prediction:** go-app sends player ids, a format, the two team ids, the venue id
and the date. It sends no features at all — the rating state lives in ml-service, which is what
makes the training and serving paths compute the same function of the same eleven names (H-8).
**Weather is not a model input.** It is wanted and was never implemented: no venue has
coordinates (Cricsheet gives names, not positions) and nothing fetches observations for a
(venue, date), so no weather feature has ever reached a model. P-6 dropped `weather_data` and
`weather_job` with the probes that read them; the `venue.latitude` / `longitude` / `city` /
`country` / `timezone` columns remain, ready for a geocoding pass. A request still carrying a
`weather` field is refused with `WEATHER_NOT_IMPLEMENTED` rather than ignored.

---

## Python ML service (ml-service)

Config file: `ml-service/config.json`

**Keys:** four, and that is the whole file. Everything the deleted models and the tuning stack
read went with them in P-5 and P-6; hyperparameters are the three-point grid inside `retrain`
and are recorded per run in `manifest.json`, not configured here.

- `inputs.training_subprocess_timeout_sec` — max time for a `/admin/train/*` subprocess
  (default 604800 = 7 days; a retrain is minutes, an evaluate is under an hour, and the
  deadline exists so a hung one does not hold its step's slot forever).
- `outputs.artifacts_dir` — the artifacts root, holding `runs/` and `current_run.json`.
- `ml.formats` — the format codes to train and serve. Checked against go-app's canonical list
  by `make frontend-backend-sync-check`.
- `ml.ratings_max_age_days` (14) — H-11: a live prediction against ratings older than this is
  refused with `RATINGS_STALE`. `XI_RATINGS_MAX_AGE_DAYS` overrides it; 0 turns the check off.

**Environment:** `ML_SERVICE_CONFIG`, `ML_SERVICE_OUTPUT_DIR`, `MODELS_DIR`, `ENABLE_HOT_RELOAD`,
`ADMIN_API_KEY`, `XI_RATINGS_MAX_AGE_DAYS`, `ML_MARKET_ODDS_DIR`.

**Cached market odds (X-4), not a model input.** `make evaluate` looks for closing-odds CSVs
in `data/market-odds/` at the repository root — `ML_MARKET_ODDS_DIR`, or
`make evaluate MARKET_ODDS_DIR=…`, overrides it. The directory is **git-ignored on purpose**:
the source (Betfair's published Big Bash / Women's Big Bash season summaries) charges nothing
and asks for no account, but grants no redistribution right, so the files are cached per
machine and never committed. A file is read only if its name starts with a declared
competition prefix (`BBL`, `WBBL`), because the file name is what says which gender the
runner names belong to. With the directory absent or empty the harness still emits its
`market_benchmark` section and reports zero coverage. **Odds are a yardstick only** — nothing
that builds a feature, fits a model or serves a prediction may read them, and a test in
`ml-service/tests/test_xi_market.py` enforces that by import graph. See
**ml-and-training.md** § Evaluation harness and **EXTERNAL_DATA_PLAN.md** § X-4.

**Artifacts naming:** `runs/<run_id>/` holds `xi_ratings.joblib`, `xi_win_<FORMAT>.joblib`,
`xi_perf_<FORMAT>.joblib`, the run's report and `manifest.json`; `current_run.json` at the root
names the run being served. See **ml-and-training.md** § Runs, manifests and staleness.

**Serving:** `format` is required on feature rows, all rows must share it, and a model for it must be loaded. A request without `format` returns 400 `MISSING_FORMAT`; a format with no loaded model returns 404 `MODEL_NOT_LOADED`.

---

## Database migrations

**Location:** `go-app/migrations/`. Applied by `go run ./cmd/migrate -dir ./migrations`, by `make migrate`, and automatically at API and importer startup.

**How the runner works:** `db.RunMigrationsFS` reads every `*.sql` in the directory, sorts by filename, and applies each version not already recorded in `schema_migrations`. Files ending `.down.sql` are skipped — they are rollback scripts, run by hand if ever, and were previously applied forward because `"down"` sorts before `"up"`.

**Baseline:** `0001_baseline.sql` is a consolidated schema that replaced migrations `0001`–`0095`. Those were squashed because the data is reproducible from the Cricsheet source files — migration `0090` already truncated every fact table on that basis, so the chain carried no state worth replaying.

It was generated with:

```bash
pg_dump --schema-only --no-owner --no-privileges --no-comments \
        --exclude-table=schema_migrations
```

against a database bootstrapped through the full chain, then hand-edited to drop psql meta-commands and session `SET` noise, and to restore the `match_format` seed rows that `--schema-only` omits. Those ids are a contract: `go-app/internal/formats` hardcodes `TEST=1, ODI=2, T20=3, T20I=4`.

**Adding a change:** write a new numbered migration (`0002_...sql`). Do not edit the baseline.

**Four tables are write-only.** `batting_data`, `bowling_data`, `fielding_data` and
`fielding_event` are scorecard aggregates the importer derives from `ball_event`. Their last
*reader* died in P-6 with the feature-history and backtest-feature repos, but the importer
still writes them, and a table goes only when its last reader and its last writer die in the
same PR — dropping them means changing the importer. The PR that stops the importer writing
them is the one that drops them. Nothing in the ML pipeline reads them; the rating pass reads
`ball_event` directly. See `ML_PIPELINE_REARCHITECTURE_PLAN.md` §9.1.

**`issued_prediction` is the prediction record (P2-3, migration `0013`).** One row per
successful `POST /api/predict/team-selection` answer, written by that endpoint before it
replies and by nothing else. It holds:

| Column | What it is |
|---|---|
| `id` (uuid), `issued_at` | The row's identity, minted by go-app *before* the insert — the id is inside `payload`, because what is stored is the answer the caller received, whole |
| `run_id`, `ratings_through` | The rating state the answer was computed from, read off the answer's own `served_ratings` stamp (P1-5), never off a status call |
| `format_code`, `team1_opposition_id`, `team2_opposition_id`, `gender`, `match_date` | The fixture, as the importer will present it when the match is played — the key a resolver joins on |
| `selection_objective` | `win`, `ratings` or `fixed`. `fixed` is an eleven the caller pinned in Play mode: a scenario, listed and never scored |
| `win_probability_team1`, `win_probability_source` | The headline claim and the model behind it — what a Brier is computed over |
| `request`, `payload` (jsonb) | The request as this API parsed it, and the answer it received. Stored whole so a scorer written later never finds that the field it needs was not one of the columns somebody thought of |

Every column beside the two documents is also inside one of them; they are columns because
a join and a sort should not have to parse a payload. **It is not a cache.** Migration
`0007` dropped one of those — `match_prediction_aggregates`, predictions about matches that
had already been played, every row recomputable by running the flow again. A row here names
a rating state that the next retrain replaces, so after that nothing in this system can
reproduce it. Rows are never edited and never deleted by the application: a prediction is a
claim that was made, and the record's whole value is that it cannot be tidied.

**The auction record is three tables (P3-1, migration `0015`).** Nothing in this schema held
an auction — no player list spanning clubs, no price, no buyer, no squad in progress — so
`0015` adds them. Every row is a fact the operator typed during a live auction; nothing is
fetched, and no importer, job or scheduler writes any of it.

| Table | What it holds |
|---|---|
| `auction` | One auction: `id` (uuid, minted by go-app), `name`, `format_code` (one the simulator serves), `buyer_opposition_id` (the side whose squad it fills), and the eleven's constraints `squad_size`, `min_bowlers`, `require_keeper` — the same two the predict path takes |
| `auction_venue` | The grounds one auction is for, as `venue_id`s. A child table rather than an array so the reference to `venue` is a real one: a ground the database cannot resolve cannot be named |
| `auction_player` | One listed player: `state` (`available` \| `sold` \| `unsold`, held by a CHECK constraint and declared in `contracts/ops-console.contract.json`), `buyer_name` and optional `buyer_opposition_id` on a sale, `price` (an integer in the auction room's own unit — no currency is implied), and `listed_at` / `state_changed_at`. A CHECK refuses half a sale: a sold row carries a buyer and a price, anything else carries neither |

The buyer's squad is the sold rows whose `buyer_opposition_id` is the auction's own side;
every other franchise is a name the operator typed, because they are buyers he observes and
not sides this record keeps a squad for.

**The projection's assumptions are three more (P3-2, migration `0016`).** A projection of a
candidate's output is conditional on the eleven he would join, the opposition it would face
and the grounds, and at an auction not one of those is a fact. `0016` puts the first two on
the record so every item after P3-2 reads one list rather than its own guess at one.

| Table | What it holds |
|---|---|
| `auction_likely_xi` | The eleven a candidate would be projected into, as the operator guessed it: `player_id` and the `position` they listed them in. Ten with a place open for the candidate is the usual state and eleven naming him already is the other, so no size is fixed here — which it is only matters once a candidate is named. It is not a batting order: nothing in this system predicts one |
| `auction_opposition` | The side a projection is against, as an `opposition_id`. Not decoration: the performance model reads a ground **only** through the team context, and `ml.xi.rows.team_context_or_neutral` falls back to neutral for the *pair* — taking the venue with it — whenever either side is unnamed, so an opposition with no side would make every ground read alike |
| `auction_opposition_player` | Its eleven. A whole eleven or nothing, unlike the likely eleven: there is no candidate joining it, so a ten-man opposition is a side nobody plays |

Neither is a fielded eleven. `match_player` holds elevens that actually played, and the
opposition is *seeded* from a side's last recorded one and then edited — the fixture these
two assume does not exist, so nothing here will ever be resolved against a match.

**The roles are deliberately not stored.** Whether a player is a keeper or a bowling option
is what the served rating vectors say *today*, read from ml-service's `POST /xi/player-roles`
on every read and stamped with the run and date it came from. Storing it would freeze one
run's answer into a record that outlives the run, and the surface would then show a role with
no date on it. `player.is_wicket_keeper` is a name-set flag from the import and is not the
model's role either.

**It is not a prediction, not a cache and not a feed.** `issued_prediction` holds forecasts of
fixtures, which P2-4 scores; nothing here is a forecast of anything, so nothing here is on the
track record and nothing here is ever scored. No row can be recomputed from anything else in
the database, because no other source of it exists.

**Existing databases:** a database created by the old chain has all 41 old versions in `schema_migrations` and will never match a fresh one. Recreate it:

```bash
make dev-destroy   # drops containers AND named volumes, including pgdata
make dev-up
```

Note `make dev-purge` does **not** drop the database — it stops the stack and removes `output/`. `dev-destroy` is the one that deletes volumes.

---

## Cricsheet import

**Why “imported” count can be less than files on disk**

1. **Fail-fast (default):** On the first file that errors (parse, unsupported `match_type`, DB error), the run stops. Count = files imported before that failure.
2. **Only top-level files:** Only direct children of the input directory are considered; subdirectories are not recursed.
3. **Supported match types:** Only `info.match_type` in: `TEST`, `MDM`, `ODI`, `ODM`, `T20`, `T20I`, `IT20`. Any other (e.g. `Friendly`, `T10`) causes an error and with fail-fast stops the run.

**Finding and fixing failing files**

- Re-run and check logs for the first error (filename and message).
- To import the rest and list skipped files, run with fail-fast off:

  ```bash
  make cricsheet-import FAIL_FAST=0
  ```
  Or: `go run ./go-app/cmd/cricsheet-importer -in=../data/go-app/cricsheet -fail-fast=false`

  Skipped files are logged; fix or remove them and re-run.

**Scope:** Input dir = `-in` / `GO_APP_INPUT_DIR`. One file = one match; count = files that completed `ImportMatchFile` without error. Importer: `go-app/cmd/cricsheet-importer`, `go-app/internal/cricsheet/ingest.go`; format: `go-app/internal/cricsheet/format.go` (`DetectFormat`).

### What identifies a venue

A ground is its **folded name**, `venue.normalized_name`, not the spelling a particular
match file used. The fold is `go-app/internal/venues.NormalizeName`, the same rule
`reference-data/venue-geocoding.csv`'s `venue_key` column applies in Python
(`ml-service/ml/weather/venues.py`): accents dropped, case folded, runs of punctuation and
whitespace collapsed to one space, and nothing else. The column is `NOT NULL` and uniquely
indexed, so a venue with no identity cannot exist.

The fold is deliberately conservative. It does **not** key on the part before a comma:
`County Ground` names nine different grounds in this archive — bare, Bristol, Chelmsford,
Derby, Hove, New Road, New Road/Worcester, Northampton and Taunton — and two of those
trailing parts are cities, so a rule that dropped them would make nine venues one.
Spellings that differ by more than punctuation, such as the four `Kensington Oval`
variants, also stay apart; merging those needs coordinates, not a string rule.

On the archive as it stands the fold merges four pairs, 896 venue rows to 892 (the CSV's
key count exactly): `Dr. Y.S. Rajasekhara Reddy ACA-VDCA Cricket Stadium`,
`Gahanga International Cricket Stadium, Rwanda`, `M Chinnaswamy Stadium` and
`R Premadasa Stadium` each absorb one punctuation variant. Migration
`0021_venue_identity.sql` backfills the key, moves the loser's matches, weather days and
auction rows onto the surviving row, and deletes the loser; the survivor is the lower id,
so `venue_name` keeps the spelling the archive recorded first.

Two consequences worth knowing:

- **A match file that names no venue is recorded with no venue.** `match.venue_id` is
  nullable and stays null. The importer used to fall back to `info.city`, which built
  venue rows named after cities that then accumulated familiarity and scoring history under
  a name no XI ever played at. The city now goes where it belongs, `venue.city`, filled by
  the first file that names one beside a ground and never rewritten.
- **Creating a venue is still the importer's act alone.** The prediction path resolves a
  named ground with `db.FindVenueIDByName`, which matches the same folded key and never
  writes; a name this database does not hold is `400 VENUE_NOT_FOUND` (GO-08).
  `/api/options/venues` offers `venue_name`, so every string it lists resolves.

---

## Player biographies (X-1a)

The archive says what happened, never who it happened to. Dates of birth, batting
handedness and bowling style are not in a Cricsheet file at any price, and X-1b wants to
test them as features — so X-1a acquires them, measures how far they reach, and stops
there. Since X-1b one column is read by `ml-service/`: `birth_date`, through
`ml/xi/biography.py`, so every player row carries the player's age at the match date and
whether a date of birth exists (`contract.AGE_COLS`; a missing date is its own category,
never an imputed age). The archive path has no table to read, so an offline retrain,
evaluate or parity run takes the same dates from a CSV:

```bash
make export-birth-dates BIRTH_DATES=data/go-app/player-birth-dates.csv   # from the database
make evaluate CRICSHEET_DIR=data/go-app/cricsheet BIRTH_DATES=data/go-app/player-birth-dates.csv
```

Without `BIRTH_DATES=` an archive run logs that every age reads as unknown, and the pass's
data-quality count `players_with_birth_date` says how many players it could see a date for
(`make xi-parity` compares it between the sources). Nothing else in the table — style,
hand, career end, death — is read by any model: X-1a found them for 265, 18, 3 and 25 of
13,662 players, and X-1b records the matchup family as not runnable on that.

**The source and the join.** Wikidata, licensed **CC0-1.0**, joined to the player registry
through the ESPNcricinfo player id that Cricsheet's [people
register](https://cricsheet.org/register/people.csv) and Wikidata (property `P2697`) both
carry. Both files are free and need no account, which is the standing constraint on this
plan. The properties read are `P569` date of birth, `P570` date of death, `P2032` end of
work period, `P741`/`P552` handedness and `P2545` bowling style.

**One command, resumable.**

```bash
make player-biographies                          # acquire, then write the coverage report
make player-biographies BIOGRAPHY_ARGS=-report-only   # re-measure what is stored, ask nothing
```

It is a step **beside** the cadence, not in it: a date of birth never changes and a bowling
style rarely does, so re-asking Wikidata on every weekly refresh would be work whose answer
is known. Run it when the registry has grown enough to matter, or weekly beside `make
cadence` if that is simpler to schedule.

Every batch's answers — **including its misses** — are appended to
`output/player-biographies/wikidata-lookups.jsonl` before the next batch starts, so a run
that is interrupted is resumed by running the same command again, and a second run over
unchanged data asks Wikidata nothing. The query service is asked in batches of 500 ids with
a second between them and a `User-Agent` that identifies the caller; set
`WIKIDATA_USER_AGENT` to put your own contact address in it, as the service asks.

**The answers are preserved in git.** `output/` is git-ignored and `make dev-purge` deletes
it, so that working cache is not a copy anyone should rely on. The durable one is
`reference-data/wikidata-player-lookups.jsonl`, committed with the people register it joins
through; Wikidata is CC0 and the register is ODC-By, so both may be redistributed.
`make restore-player-biographies` rebuilds the whole table from them without a network call.
See [reference-data/README.md](../reference-data/README.md) and § Recovering the external
data below.

**What is stored.** `player_biography` (migration `0010`), one row per player — *including
the players nothing was found for*. "Wikidata has no item carrying this id" is an answer;
no row at all means the pass has not run for him, and only the first is a measured coverage
figure. A bowling style is mapped into a small controlled vocabulary (`pace`, `medium`,
`off-spin`, `leg-spin`, `left-arm-orthodox`, `left-arm-wrist`, `unknown`) with the source
label kept beside it, so a mapping can be corrected later without re-fetching. A date of
death is recorded as itself and never written into `career_end_date`: a player who died in
2022 may have stopped playing in 2007.

**Curated overrides.** `configs/player_biography_overrides.json`, keyed by
`player.external_id`, lays hand-checked facts over the acquired ones — the same pattern as
the venue geocoding placeholder. Only the fields a row names change, every row needs a
`note` saying where the fact came from, and the vocabulary and dates are validated before
anything is written. Work down `docs/player-biography-coverage.md` § The largest gaps: it
lists unmatched players by appearances, so the top of the list is where an override buys
the most coverage.

**Where the coverage shows.** `GET /ops/data/biography-coverage` measures it live from the
table — per format and gender, weighted by appearances rather than by players, because that
is the share a feature would actually see — and the Ops tab renders it beside the dataset
registry. The committed figures are in
[player-biography-coverage.md](player-biography-coverage.md); what they mean for X-1b is in
[EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-1a.

**One consequence outside acquisition.** The retirement ledger's corroboration criteria
(D-12) are a pluggable list, and two of them read a date of birth and a career end date.
They reported themselves *unavailable* because nothing supplied either. They now read
`player_biography`, and answer per player rather than per deployment.

## Pre-match weather (X-2)

The archive says where a match was played and on what day, never what the air was like.
X-2 acquires that from Open-Meteo's ERA5 archive — free for non-commercial use, CC BY 4.0,
no key (§ Data-source licence register) — measured four families of pre-match weather
against the gates, and **shipped none**: every family is a recorded null, with the tables in
[EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-2. Nothing in `ml/xi/` reads the
weather. What stays is the data, because a venue's whereabouts and a match day's weather
are product facts in their own right and re-acquiring them is the one cost worth never
paying twice.

**Three things are held, all under `ml-service/ml/weather/`.**

- **Where each venue is** — `reference-data/venue-geocoding.csv`, one row per venue key
  (the name folded the identity way: case, accents and punctuation, so two spellings of one
  ground share a row). The geocoder was asked about the city Cricsheet names beside the
  venue (`info.city`, which the importer never stored) or, failing that, the venue name's
  own parts, and the sides that played there voted for the country — which is what makes
  Hamilton New Zealand's and not Ontario's — and, since DATA-01, a candidate in a country
  the top vote out-votes beyond chance is refused rather than taken (the counts are on the
  row in `countries_voted`, and a row placed anywhere but the top-voted country says so in
  its `note`). 112 rows the geocoder placed wrongly or not at all were then placed by hand,
  each with a `note` saying so; a venue nothing places is written `unmappable` with the
  reason rather than guessed at (today none is). Rows already in the file are never
  rewritten by a run, so a correction survives. ERA5 is a 0.25° reanalysis, so a ground's
  city places it as well as its gates would.
- **One reduced day per (venue, match day)** — `reference-data/era5-venue-days.jsonl`: the
  day's 24 local hours of temperature, relative humidity and precipitation, and the
  precipitation totals of the seven days before, a few hundred bytes per day rather than
  the hourly archive of every venue's every year. Multi-day matches are keyed by their
  first day. A day the archive holds no readings for is written as a **miss with its
  reason**, so a restore does not ask again. The file is append-only and flushed after
  every call, so an interrupted run loses at most the cluster in flight.
- **When the match started** — `ml/weather/sessions.py`. Cricsheet carries no start
  times, so a session window is *inferred* from competition and format norms: the league's
  usual hour (the earlier of a double-header from its match number), the country's usual
  hour for internationals, the weekday or weekend hour for a domestic T20, the morning for
  first-class and women's and associate cricket. Every match records the rule that placed
  it (`SessionWindow.rule`), the census is in the coverage report, and the error is
  inspectable rather than hidden in a feature.

**Every feature is fixed before the first ball (H-21).** The families read the mean of the
three hours *before* the inferred start (humidity, temperature), the night flag times that
humidity (dew), and the rain that had already fallen (the day before plus the match day's
hours before the start; the seven days before). Nothing reads the match's own hours. A
match without readings in its window is its own category (`wx_known` 0), never an
imputed climate.

**One command, resumable.**

```bash
make venue-weather                       # geocode new venues, fetch the days the file lacks, print coverage
make venue-weather WEATHER_ARGS=--to-db  # the same, and write the database
```

It is beside the cadence, not in it: the past does not change, and the archive trails the
present by several days, so a match day younger than a week is left unasked rather than
recorded as a miss. Calls are paced well under the published limits; the daily quota, if
it is ever hit, ends the run with the file intact and the same command resumes it.

**The database.** `make venue-weather WEATHER_ARGS=--to-db` (or the restore below) writes
the coordinates onto the `venue` columns that had waited empty since the baseline schema
(`latitude`, `longitude`, `timezone`, `city`, `country`, `source`, `verified_at`) and the
days into `venue_weather` (migration `0012`), keyed by `venue.id` through the venue name
the importer stored — the same string the archive spells.

---

---

## Acquiring a dataset

Getting a new Cricsheet archive onto the box used to require a shell. It is now two
tracked background jobs like every other pipeline step.

### Fetching it

| Endpoint | What it does |
|---|---|
| `GET /ops/data/feeds` | The named feeds, the host allowlist and the staging directory |
| `POST /ops/data/fetch` | Downloads an archive into staging. Body: `{"feed":"t20s"}` **or** `{"url":"https://cricsheet.org/..."}` — one or the other, never both |

The response is **202**, not the archive: a multi-hundred-megabyte download over a slow
link outlives any sane request timeout. Watch it on the existing SSE stream
(`GET /ops/pipeline/stream`), where the step reports `fetch` with bytes, rate and ETA.

**What it refuses, and why**

- **Any host outside the allowlist** (`cricsheet.org`, `www.cricsheet.org`), including
  redirects that leave it. A server-side fetch of a user-supplied URL is an SSRF
  primitive; the allowlist is the feature's definition, not a hardening pass. Matching
  is exact, so `cricsheet.org.attacker.example` is refused.
- **Plaintext http**, and a redirect that downgrades to it.
- **A URL whose last path segment is not a plain `*.zip`**, so the staging filename can
  never contain a separator or a `..`.
- **An archive over the size cap** (4 GiB), by its declared `Content-Length` or by
  outgrowing it mid-stream.
- **A download the filesystem cannot hold**, checked before it starts. When free space
  is unreportable this is logged and skipped rather than guessed at.
- **A short read.** The byte count is verified against the declared length and the
  SHA-256 recorded. A truncated archive fails here rather than becoming a corrupt
  dataset that only breaks during extraction.

Nothing is written into the live dataset directory. The archive lands in
`<cricsheet_dir>/_staging/` under a temp name and is renamed into place only after its
bytes and digest are known, alongside a `.meta.json` sidecar recording the source URL,
`ETag`, `Last-Modified`, size and digest. That sidecar is what makes a second fetch
conditional: an unchanged archive answers 304 and is reported as "already current"
rather than re-downloaded.

### Extracting it

| Endpoint | What it does |
|---|---|
| `GET /ops/data/staged` | Staged archives with their provenance, plus the manifest of the live dataset |
| `POST /ops/data/extract` | Inflates a staged archive into the dataset directory. Body: `{"archive":"all_json.zip"}`, or `{}` for the newest |

Also **202**, also visible on the SSE stream — the `extract` step reports entries and
bytes.

**Nothing enters the dataset directory until the whole archive has been inflated and
checked.** That ordering is the guarantee: reading paths and inflating bytes from an
archive we did not create happens entirely inside `_staging/`, so a malicious,
truncated or simply wrong archive fails with the live directory untouched. The final
step is same-filesystem renames of files already known to be good.

**What it refuses.** `archive/zip` does **not** sanitise entry names on your behalf —
`f.Name` is whatever the archive author wrote — so every one of these is checked here,
with a crafted fixture per case in `extract_test.go`:

- **Zip-slip**: `../escaped.json`, `a/b/../../../x`, `/etc/passwd`, backslash-separated
  traversals (separators are normalised *before* the check, not after), `C:\...` drive
  prefixes and `//host/share` UNC paths. The resolved path is then re-checked against
  the destination root with a trailing separator, so `/data/cricsheet-evil` cannot pass
  as a prefix match of `/data/cricsheet`.
- **Symlink entries**, and anything else that is not a regular file or directory. A link
  entry has an innocent name; it is the *next* entry written through it that escapes.
- **Zip bombs**: caps on entry count, total uncompressed bytes and per-entry expansion
  ratio, checked up front from the central directory *and* again mid-inflate — the
  declared sizes are attacker-controlled, so a bomb that understates itself is caught
  on the way in.
- **An archive with no match files.** Extracting "successfully" into an empty dataset
  is the silent-success failure this repo has been bitten by repeatedly.

Extraction **replaces** rather than merges, so a stale match file from the previous
dataset cannot survive into the new one. The displaced contents are moved to
`_staging/previous-<timestamp>/`, which makes a mistaken replace recoverable. A
`.dataset-manifest` is written beside the data recording entry count, bytes, the source
archive's digest and URL, and the time — the provenance A-3 and P-1 build on.

### Importing it, and when Import re-downloads

`POST /ops/pipeline/run/import` — the console's **Import** step — runs the whole
acquisition plan: fetch → extract → import. It does not always fetch.
`dataacquire.PlanAcquisition` asks one narrow question first: *does the dataset
directory already hold data from the configured source?* When the live
`.dataset-manifest` names that URL and the directory holds match files, both fetch and
extract are skipped, and the 202 says which and why:

```json
{"status":"started","plan":"import","steps":["fetch","extract","import"],
 "source_url":"https://cricsheet.org/downloads/all_json.zip",
 "skipped":{"fetch":"the dataset directory already holds all_json.zip from this source (12345 match files)",
            "extract":"the dataset directory already holds all_json.zip from this source (12345 match files)"}}
```

The console renders those reasons in the step dialog. **A skipped download that renders
as a completed one is the failure this reporting exists to prevent** — the symptom is an
operator re-running Import and finding DB Data Freshness unchanged, because the same
matches were re-imported.

It never asks whether the *upstream* archive changed. Cricsheet republishes under the
same URL often enough that a conditional request would answer "changed" almost every
time, so re-fetching is an explicit decision rather than something inferred from a
header. Make it with **`?refresh=1`**, which is what the dialog's *"Re-download the
archive even if this dataset is already on disk"* checkbox sends. That is the supported
way to pick up newer matches; `POST /ops/data/fetch` remains available for pulling a
source other than the configured one.

### What the innings record holds

`match_inning`, `batting_data`, `bowling_data`, `fielding_data`, `fielding_event` and
`ball_event` describe the **innings of the match** and nothing else. A Cricsheet file's
`innings` list is longer than that when a limited-overs tie was decided by a super over:
the tie-breaker is appended as one more entry per side, flagged `super_over: true`, and
the archive carries 226 of them (113 matches — IPL, BBL, CPL, T20Is and 20 ODI innings).
The importer leaves those out (`cricsheet.Match.PlayedInnings`), so a tied T20 has two
`match_inning` rows, no `ball_event` above innings 2, and no scorecard row for a player
who appeared only in the super over. Until IMPORT-01 was fixed they were stored as
innings 3 and 4: six deliveries of career balls, runs and dismissals per super over, a
batting position for a batter who did not bat in the match, and an "innings 3" total in
every context baseline the rating pass builds. The archive path of the rating pass
(`ml.xi.sources.played_innings`) applies the same rule, which is what keeps the H-8 parity
check comparing the same deliveries from both sources.

The choice was to leave super overs out rather than store them behind an `is_super_over`
flag, because every reader of those six tables — the rating pass, the scorecards, the
career totals — would then have to remember to exclude them, and one that forgot would
reproduce the defect silently. The fact that a match *was* decided by a super over is not
a property of its innings but of its outcome, which is where Cricsheet records it and
where the record keeps it — see the next section.

The same decode reads Cricsheet's other innings-level facts — `declared`, `forfeited`
(an innings with no `overs` at all, 14 in the archive) and `target` — so a short innings
can be told from a truncated file. Nothing derived from `declared` or `forfeited` is
stored yet; a forfeited innings simply has no `ball_event` row, and the innings after it
keep their own numbers — see *What the ball-event record holds* below.

**The target** is stored, in `match_inning.target_runs` and `target_overs`, and is the
archive's own figure wherever the archive states one. Cricsheet writes `innings[].target`
as `{runs, overs}` on the innings being chased: `runs` is the score that *wins*, `overs`
the limit that chase was given. 18,264 of the 22,905 files carry one. Where a file states
none, a limited-overs second innings gets the first innings' runs **plus one** — the
arithmetic of a chase — and a null over limit, because the allotment is already in
`match.scheduled_overs_per_innings`. A multi-day innings gets neither: no file in the
archive puts a target on one.

`target_overs` is a `real` in the scorer's overs-and-balls notation, like `overs_bowled`:
158 of these targets are fractional (`12.4` is twelve overs and four balls), and an integer
column would lose every one.

Until IMPORT-11 the importer read none of this and wrote the first innings' *runs* into
`target_runs` on every second innings. That was one short of the winning score on all
19,432 limited-overs chases; it replaced each of the 983 revised targets with the
unrevised total and discarded each of the 1,541 shortened over limits; and it put a
target on the 3,102 second innings of Tests and first-class matches, where the first
innings' total is not even the lead. The rows in the table keep those figures until the
directory is re-imported, which is what writes the corrected ones and clears the
multi-day rows to null. Nothing reads either column yet, so no model or served number was
ever built on them.

The revised target is not recoverable from `match.result_method`: that is
`info.outcome.method`, which says how the *result* was reached, and 577 of the 1,550
matches with a revised target name no method there at all because the revised chase was
completed normally. The method is therefore stored once, on the match, and not repeated
on the innings.

### What the outcome record holds

`match` says how a match was decided in three columns, all of them Cricsheet's own words
from `info.outcome`. Cricsheet writes a match that was won outright as `winner` (and the
margin, `outcome_by_runs` / `outcome_by_wickets`), and a match that was not as `result` —
`draw`, `no result` or `tie` — with no winner. A tie that a tie-breaker then settled keeps
`result: tie` and names the side that won it under `eliminator` (a super over; 109 files
in the archive) or `bowl_out` (2 files, both 2007). `method` is the rule that adjusted or
awarded the result: `D/L` on 1,018 files, `VJD`, `Awarded`, and once `Lost fewer wickets`.

- **`outcome_winner_opposition_id`** is the side the match went to: `winner`, else
  `eliminator`, else `bowl_out` (`cricsheet.Outcome.WinningTeam`, one rule for both
  sources — `ml.xi.sources.winning_team` on the archive path). A tie-breaker win is a win:
  the competition records it as one, a forecast of the match is scored against it, and
  both sides' Elo and form see it. NULL means a draw, a no-result or a tie nobody broke.
- **`result`** (migration `0017`) is `outcome.result` verbatim, NULL for a match won
  outright. It is kept beside a winner for a tie-breaker win, so `result = 'tie'` with a
  winner is a match that was tied and then decided — distinguishable from an outright win
  and from a tie left as one, without going back to the file. A reader that wants to
  weight such a win differently from an outright one can, from the row. The rating pass
  reads it in one place — a draw or an unbroken tie is half a win of form for each side
  (`team_form_diff`) — and `make xi-parity` compares `drawn_or_tied_matches` between the
  sources so that reading cannot silently differ again (FEAT-04).
- **`result_method`** (`0017`) is `outcome.method` verbatim, NULL where the archive names
  none. It is `varchar(32)`, not the 16 the audit proposed, because the archive's longest
  value is eighteen characters and a narrower column would have refused that file whole.

Until IMPORT-02 was fixed only `winner` was read, so every super-over win was stored with
no winner at all — the same row as an abandoned match — and the rating pass, which reads a
NULL winner as no result, left those matches out of the frame and out of both sides' Elo.
The archive path of the rating pass agreed with it, reading only `winner` too, which is why
the H-8 parity check never saw the difference. Both columns are written by the importer
and by nothing else; a match imported before `0017` carries NULL in both until the
directory is re-imported.

### What the competition record holds

The four format codes pool cricket the archive tells apart. `match_format` has `TEST`,
`ODI`, `T20` and `T20I`; Cricsheet's `match_type` has `Test`, `ODI`, `T20`, `IT20`, `MDM`
and `ODM`, and beside it `team_type` says whether both sides are national teams. The
importer folds `MDM` (Sheffield Shield, County Championship, Ranji Trophy) into `TEST` and
`ODM` (domestic one-day, and internationals without ODI status) into `ODI`, so on the
archive as imported 70.6 % of `TEST` rows and 39.3 % of `ODI` rows are not the format the
label names. Three columns on `match` keep the distinction recoverable (IMPORT-09):

- **`original_match_type`** is `match_type` verbatim, and is what separates a Test from a
  first-class round under the one code.
- **`competition_level`** (migration `0022`) is `team_type` verbatim, `international` or
  `club`. It is the T20 / T20I rule: Cricsheet writes every twenty-over match as `T20`,
  national sides included, so a `T20` whose level is `international` is stored as `T20I`
  and a `T20` whose level is `club` as `T20`. Until IMPORT-09 the rule was a hand list of
  twelve team names in `config.json`, which took the 3,888 T20s between other national
  sides — World Cup qualifiers, and the 87 World Cup matches the twelve played against
  them — for club cricket; the list is gone. A file with no `team_type` is refused rather
  than guessed at. Nothing but the importer reads the column yet: it is what the pooling
  decision — does `TEST` mean Test cricket or first-class cricket? — is measured against,
  and once taken it is applied with an `UPDATE` of `format_id`, not a re-import.
- **`match_type_number`** (`0022`) is the ICC's running number for an official Test, ODI
  or T20I, NULL for club matches and for the 320 `IT20` files, which are internationals
  played before their sides had T20I status.

Nothing back-fills the two new columns: a re-import of the whole directory is what writes
them, and the same re-import is what moves the 3,888 misfiled matches onto `T20I`.

### What the ball-event record holds

`ball_event` has one row per delivery, and its runs are in three parts as Cricsheet writes
them: `runs_batter`, `runs_extras` and their sum `runs_total`. The extras are then broken
out by kind — **`extras_wides`, `extras_noballs`, `extras_byes`, `extras_legbyes`,
`extras_penalty`** (migration `0018`), each Cricsheet's own count from the delivery's
`extras` object. The archive uses exactly those five keys and no others, `runs_extras` is
their sum on every one of its 11.6 million deliveries, and a delivery can carry two of
them: 800 no-balls have byes off them, 306 have leg-byes, and a penalty sits beside a wide
on 5. `extras_kind` is older and stays: one word chosen by precedence (wide, then no-ball,
then leg-bye, bye, penalty), which is a convenient summary and a lossy one — a no-ball
with four leg-byes reads `no_ball` alone. The five counts are what make it recoverable.

The breakdown matters because of who the runs belong to. Wides and no-balls are the
bowler's; byes, leg-byes and penalty runs go to the innings but not to him, and
`cricsheet.Delivery.RunsConcededByBowler` is the one rule for that — `runs_total` less
byes, leg-byes and penalty. `bowling_data.runs` (and so `econ`, and the per-over totals a
maiden is judged on) is built from it. Until IMPORT-04 was fixed the bowler was charged
the delivery's whole total, so every bowler's figures carried his keeper's misses, and
the rating pass — which summed `runs_total` for runs conceded — inherited the same noise.
FEAT-08 closed the other half: both rating sources now derive the runs charged to the
bowler through the same rule (`ml/xi/sources.py`, `runs_conceded_by_bowler`), the
Postgres source reading `extras_byes`, `extras_legbyes` and `extras_penalty` and the
archive source the delivery's `extras` object. Rows imported before `0018` hold zeros in
all five until the directory is re-imported; nothing back-fills them, and until then the
rating pass over the database charges the bowler everything — which `make xi-parity`
reports as `runs_not_charged_to_bowler` differing from the archive. The rating pass's
query names the three columns, so it needs the migration applied before it runs at all.

A ball faced and a ball bowled are two counts, and the row carries one of them.
**`is_legal` is the bowler's:** a delivery that is neither a wide nor a no-ball, each of
which he must bowl again — his balls and overs (`bowling_data.balls`, `overs`), the
innings' `balls_bowled`, `ball_seq` and the phase are all counted by it
(`cricsheet.Delivery.IsLegal`). **A ball faced is every delivery but a wide:** the batter
faces a no-ball — he may hit it — and does not face a wide, which passes out of his reach.
`batting_data.balls` and `strike_rate` are counted by that rule
(`cricsheet.Delivery.FacedByBatter`), and there is no `faced` column because the row
already says it: a ball faced is a row with `extras_wides = 0`, which the rating pass
derives by the same rule on both of its sources (`ml/xi/sources.py`, `faced_by_batter`,
read into `Deliveries.faced` and summed into the `balls_faced` target). Until IMPORT-05 was
fixed the importer counted the batter by the bowler's rule, so every no-ball he faced was
missing from his balls and his strike rate read high (58,068 no-balls in the archive), and
the rating pass counted every delivery as faced, wides included (202,331) — two definitions
erring in opposite directions. Like the extras, `extras_wides` is zero on rows imported
before `0018`, so until the directory is re-imported the rating pass over the database
counts every wide as faced, which `make xi-parity` reports as `deliveries_not_faced`
differing from the archive; `batting_data.balls` changes only when the file is re-imported.

**Every delivery the file lists is a row, and innings are numbered by position.** An
innings' number in `ball_event` is its place in `Match.PlayedInnings` — the same place its
`match_inning` row is numbered by, and the same place the archive path of the rating pass
enumerates (`ml/xi/sources.py`, `_deliveries_from_cricsheet`) — so the two sources always
call one innings by one number. That holds even where an innings has no rows at all: the
14 forfeited innings in the archive bowled nothing, and the innings after them keep their
own numbers rather than closing the gap. The rating pass's Postgres source therefore reads
`innings - 1` for its 0-based index and never the smallest innings present; normalising by
the smallest was the same number only while innings 1 bowled a ball, and a first innings
forfeited or made up entirely of extras would have renumbered the rest of the match on one
source and not the other, silently (IMPORT-12).

Until IMPORT-12 was fixed an innings with no *legal* ball was skipped whole, which is not
the same thing as an innings with no delivery. The archive holds one: match 514034, where
South Africa needed two to win and got them off a single no-ball. That delivery — one run
off the bat and one for the no-ball — never reached `ball_event`, while its `match_inning`
row kept `runs_scored = 2` and `balls_bowled = 0`, so the scorecard and the ball-by-ball
disagreed about the same innings. It is why `make xi-parity` read 9,345,813 runs from the
database and 9,345,815 from the files; the re-import writes the missing delivery and
closes the two runs. `is_legal` already says which deliveries are the bowler's count, so
nothing downstream needs an innings to be absent to know it faced no legal ball.

**A wicket is what the vocabulary says it is.** Cricsheet records fourteen kinds of wicket
and the scorecard does not treat them alike: six are the bowler's (`bowled`, `caught`,
`caught and bowled`, `hit wicket`, `lbw`, `stumped`), six are wickets the innings lost and
nobody took (`run out`, `retired out`, `obstructing the field`, `handled the ball`, `hit the
ball twice`, `timed out`), and two are not wickets at all — a batter who `retired hurt` or
`retired not out` may come back and is not out. **`configs/wicket_kinds.json` is the one
list**, read by the importer (`internal/wicketkinds`) and by the rating pass
(`ml/xi/wicketkinds.py`), so the two cannot carry different sets. `bowling_data.wickets`
counts the credited kinds only and `match_inning.wickets_lost` counts the dismissals
(`cricsheet.Delivery.TallyWickets`); the rating pass's `wickets` target, `dismissals` target
and innings wicket counts follow the same file on both of its sources
(`ml/xi/sources.py`, `wicket_columns`). A kind the file does not name fails the import and
the pass rather than being counted by guesswork: the only way a new kind arrives is
Cricsheet adding one, and the answer is a reviewed edit to the file. Until IMPORT-06 was
fixed every kind was credited to the bowler — 23,064 wickets in the archive that were not
his, 22,898 of them run outs — every kind was a wicket lost, 508 retirements not out
included, and the rating pass counted a retired-hurt batter as dismissed.

**Every wicket on a delivery is a row of `ball_event_wicket`** (migration `0019`): one
row per wicket, `wicket_number` in the order the file lists them, `kind` as the vocabulary
spells it, `player_out_id`. `ball_event` used to carry one `wicket_kind` and one
`player_out_id`, so a delivery with two wickets lost its second — 16 such deliveries in
the archive, 15 with two (a batter bowled while his partner retired hurt, two run outs, a
batter caught and another timed out) and one with ten (1483765, a side that retired its
whole order out on one ball), 17 wicket records in all. A child table rather than a second
column pair because that one delivery is real cricket and a pair would have kept two of
its ten. The migration moves the first wicket each `ball_event` row held into the new
table before dropping the two columns, so a database migrated but not yet re-imported
describes the cricket it did before rather than none; the re-import writes the other 17.
The rating pass's query reads the new table, so the migration must precede its next run
over the database, and `make xi-parity` compares `dismissals` — every wicket the
vocabulary calls one, on every ball — so a wicket record one source drops cannot pass
unnoticed again.

### What the squad record holds

`match_player` has one row per player a side fielded in a match — everyone `info.players`
lists, which is the only record of who was picked (the scorecard shows only whoever
batted or bowled). Cricsheet lists everyone who took the field, so a side that used a
concussion substitute, an impact player, a supersub or a covid replacement is listed in
full: 1,343 sides of twelve, 21 of thirteen and one of fourteen among the archive's 45,810.
**`is_replacement` (migration `0020`) says which of them came in after the start.** The
fact lives where it happened: not in `info`, but as a `replacements.match` entry on the
delivery he came in at — `in`, `out`, `team`, `reason` — and 1,342 of the 1,365 oversized
sides carry one (916 impact players, 164 concussion substitutes, 140 supersubs, 48 injury
substitutes, 41 national call-ups, 20 releases, 17 covid, 17 unknown, one tactical). The
rule is `cricsheet.Match.ReplacementPlayers`: the entries are read in playing order over
every innings and the `in` of each is a replacement *for the side the entry names* unless
he had already gone `out` of an earlier one — the archive's one swap-back (1234909, a
covid stand-in who went back out when the man he stood in for returned) reads as one
replacement, not two. A `role` entry (a substitute finishing an injured bowler's over)
changes nobody's membership and is not read. The rating pass's archive path applies the
same rule (`ml/xi/sources.py`, `replacement_keys`) and its Postgres path reads the flag, so
both build the same eleven from one match; `make xi-parity` compares
`replacement_players` (1,362 on the archive), and `oversized_squads` now counts the sides
still over eleven once their replacements are out.

Twenty-four sides stay oversized, and honestly so: 21 Syed Mushtaq Ali Trophy 2022 sides
of twelve and two Women's T20 Challenge 2018 sides of thirteen carry no replacement entry
at all, and one (1537342) names as the man who came in for one side a player the *other*
side lists — which is why the side is part of the rule: matched by name alone, that entry
would have taken a starter off the wrong team. The importer logs that one and keeps both
sides as listed rather than guessing. A flag rather than a shorter squad because
the row is a fact worth keeping: the replacement did play, his deliveries are his, and a
reader that wants everyone who took the field (appearances, biographies, the auction's
last fielded eleven) reads the table as before. Rows imported before `0020` read `false`
until the directory is re-imported; until then the rating pass over the database hands
over the twelve, which `make xi-parity` reports as `oversized_squads` and
`replacement_players` differing from the archive.

### What a re-import does to a match already in the database

**An import replaces a match, it does not merge into it.** The match id comes from the
file name, so importing a file whose match is already there is a *re-import* of that
match: the per-file transaction begins by clearing the match out of `ball_event`,
`fielding_event`, `batting_data`, `bowling_data`, `fielding_data` and `match_inning`
(`db.DeleteMatchFactsTx`), and out of `match_player` (`db.ReplaceMatchPlayersTx`), and
then writes the file as it now reads. The match row itself is upserted, because the match
still exists. All of it is one transaction, so a file that fails to import leaves the
previous version of the match intact rather than a half-deleted one.

This is what makes a **corrected** Cricsheet file land. Corrections remove things as well
as add them — an innings that was never played, a delivery scored twice, a player who was
not in the side — and a re-import that only upserted would leave every removed row behind,
so the match on record would become the union of every version of the file ever imported.
Until IMPORT-03 was fixed it was worse than that for `ball_event`: the insert skipped rows
whose key was already there, so a re-import of an already-imported match changed nothing
at all.

**Every importer fix requires a re-import of the whole directory.** Rows are written once,
by whichever version of the importer wrote them; nothing back-fills them later. A change
to what the importer extracts or how it derives a column — a super over that was being
stored as innings 3 (IMPORT-01), a super-over win stored with no winner (IMPORT-02), byes
that were being charged to the bowler (IMPORT-04), a run out credited to him and the
second wicket of a delivery dropped (IMPORT-06), an innings of nothing but extras dropped
whole (IMPORT-12), an `info.event` field that was being
dropped —
reaches only the matches imported after it. The re-import is the whole
directory, not the changed files, because the fix applies to every match: run Import again
with **`?refresh=1`** if the archive should be re-fetched too, and expect it to take as
long as the first one did. A model built on the old rows is unaffected until it is rebuilt,
so an importer fix that changes feature inputs is followed by `make retrain` — see
`docs/ml-and-training.md` § The pipeline.

### The dataset registry

`GET /ops/data/datasets` returns one row per acquired dataset, newest first, with the
one currently in the data directory marked `live`.

`data_migrations` already records every *run*, but a run is not a dataset: two fetches
of an unchanged archive are two runs and one dataset, and a fetch followed by an
extract is two runs and still one dataset. Without a table that says so, "which data
produced this model?" has to be reconstructed from job metadata every time.

**The archive's SHA-256 is the identity**, not the filename. Cricsheet reuses filenames
across releases — `all_json.zip` is always `all_json.zip` — so a filename-keyed registry
would conflate every dataset ever fetched into one row. Fetch and extract both upsert on
the digest: a re-fetch of unchanged bytes updates the existing row rather than adding a
duplicate, and it leaves the extract columns alone, because re-downloading an archive
does not un-extract it.

**There is no `is_live` column.** Which dataset is live is a property of the filesystem:
an operator who rsyncs files into the data directory changes it without touching
Postgres, and a stored flag would go on asserting the old answer. It is derived by
matching the on-disk `.dataset-manifest` digest against `sha256`. A consequence worth
knowing: `live_sha256` can be set while no row is marked live, which means the box holds
a dataset the registry has never seen.

Optional columns are genuinely unknown rather than zero. An archive placed in staging by
hand has no feed or source URL; one downloaded but not yet extracted has no entry count.
Extraction hashes an archive that arrives without a sidecar, so even a hand-placed one
can be identified — a dataset with no digest is exactly the case P-2 has to flag.

A registry write that fails is logged, not fatal: the step itself has already succeeded,
the bytes are on disk, and provenance also lives in the job's `data_migrations` metadata,
the archive's sidecar and the directory's manifest. The registry is the index over those,
not their only copy.

**Lanes.** Acquisition runs in the `data` lane, training and the rest of the pipeline in
`compute`. Steps within a lane run one at a time; the lanes overlap, so a download does
not block a training run. `POST /ops/pipeline/stop` stops everything by default, or one
lane with `?lane=data`.

**The lane lock lives in the database, not in the process.** A step claims its lane in one
transaction that takes a Postgres advisory lock on the lane, re-reads whether any command
in that lane is `IN_PROGRESS`, and inserts its own row — so two claimants that arrive
together are serialised by the database rather than by luck. It was a `SELECT` followed by
an unguarded `INSERT`, and two `POST /ops/pipeline/run/retrain` inside one round trip both
started (GO-06). A run plan claims itself the same way on its own key — it is in no
lane by design, so two `POST /ops/pipeline/run-plan` had the identical window. An
in-process mutex would not have been enough: `cmd/cricsheet-importer`
takes the same lane from a *separate process* against the same database. A claim that
cannot be made — the database unreachable, the transaction refused — now refuses the run
rather than being logged and treated as a free lane.

**A stop stops the work, not just the bookkeeping.** Stopping the compute lane asks
ml-service to terminate its training subprocess (`POST /admin/train/stop`) and waits for it
to be gone before answering; the reply's `training_stopped` names the steps whose process
really exited. When that cannot be confirmed the answer is `502` with
`status: "partially_cancelled"` — the local run is cancelled either way, but a stop nobody
confirmed is never reported as a stop. A `?lane=data` stop leaves training alone, which is
the point of the lanes. See D-11 in [FOLLOW_UP_PLAN.md](FOLLOW_UP_PLAN.md) § 1.4.

---

## Data-source licence register

Every external source the system reads today, with its licence **as read from the source
on 2026-09-04** — not from memory, and not from a third party's summary of it. The standing
constraint in [PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md) (a prototype on free, publicly
available, licence-clean data; nothing paid, nothing account-gated) is what this table is
checked against, so a source whose terms could not be established says exactly that rather
than carrying a guessed licence. Every source below is free to obtain and asks for no
account; what differs is what each permits afterwards. Re-verify a row whenever its source
is next touched — terms change, and the date on this section is the date of the reading.

| source | what the system reads | licence, as stated at the source | redistribution | what that leaves us with |
|---|---|---|---|---|
| **Cricsheet match archive** — `https://cricsheet.org/downloads/all_json.zip` and the per-competition zips | every match, ball by ball: the whole training, evaluation and serving population | **Not stated at the source.** The front page's footer reads *"Site © 2009–2026 Cricsheet. All rights reserved"*, which is the site, not the data. The downloads page, the format pages and the JSON archive's `README.txt` carry no licence statement at all. The CSV archive's README says, in the author's words: *"any feedback as to the licence the data should be released under would be greatly appreciated … I'd like to choose the 'right' licence. My basic criteria may be that: the data should be free, corrections are encouraged/required to be reported to the project, derivative works are allowed, you can't just take data and sell it."* Third-party pages describe the data as ODC-By 1.0; nothing on cricsheet.org does, so that is **not recorded as verified** here | **Not granted anywhere we could find.** The archive is fetched onto each box (`POST /ops/data/fetch`) into `data/`, which is git-ignored, and nothing that reproduces it is committed or published | The author's stated criteria — free, derivatives allowed, corrections reported, not resold — are the terms this prototype behaves as if it were under, and its use (a local, unsold prototype that credits Cricsheet in its README) sits inside all four. **Before anything ships publicly or is sold**, this is the row P0-2 has to settle, by asking the project directly; it cannot be settled from the site |
| **Cricsheet people register** — `https://cricsheet.org/register/people.csv` | the ESPNcricinfo id per player, the join key for X-1a | **ODC-By 1.0** (Open Data Commons Attribution). The register page states: *"This dataset is made available under the Open Data Commons Attribution License: http://opendatacommons.org/licenses/by/1.0/."* and *"You must attribute any public use of the dataset, or works produced from the dataset, in the manner specified in the license."* | Permitted, with the licence made clear and notices kept intact: *"For any use or redistribution of the dataset, or works produced from it, you must make clear to others the license of the dataset and keep intact any notices on the original dataset."* | Attribution is owed on any public use, including works produced from it. The comment in `go-app/internal/biography/biography.go` used to describe the register as **ODbL**, which carries a share-alike term ODC-By does not; that was B-6 in [BUG_BACKLOG.md](BUG_BACKLOG.md) and the comment now states ODC-By 1.0 and points back at this table. A pinned copy is **committed** as `reference-data/cricsheet-people-register.csv` with the licence and attribution stated in `reference-data/README.md`, because the Wikidata snapshot keys to nothing without it |
| **Wikidata** — the SPARQL query service; properties `P2697`, `P569`, `P570`, `P2032`, `P741`, `P552`, `P2545` | player biographies (X-1a) | **CC0 1.0.** Wikidata:Licensing states: *"All structured data in the main, property and lexeme namespaces is made available under the Creative Commons CC0 License"* | Permitted, without conditions | Nothing is owed; the licence is recorded per row as `player_biography.source_license` (`CC0-1.0`). The query service asks for an identifying `User-Agent`, which the backfill sends. Because redistribution is permitted and re-acquisition is a rate-limited pass over every player, the acquired answers — hits and misses — are **committed** as `reference-data/wikidata-player-lookups.jsonl`; see § Recovering the external data |
| **Betfair Exchange season summaries** — BBL / WBBL Match Odds CSVs at `betfair-datascientists.github.io/data/dataListing/` | closing odds, read by `make evaluate` only as a yardstick (X-4); never a feature | **No licence granted.** The page carries a warranty disclaimer and nothing else: *"By downloading this data, you acknowledge and agree that: (a) Betfair does not make any representations, or give any warranties, as to the accuracy or completeness of the data provided; and (b) you use the data at your own risk, and Betfair will not be liable for any loss suffered in using the data."* | **Not permitted** — no right is granted, so none is assumed. The files are cached under `data/market-odds/` (git-ignored) per machine and never committed | Usable as a local measurement, which is all X-4 does with them; the numbers derived from them (AUC, Brier, coverage) are published in the harness report, the rows are not |
| **Open-Meteo** — the historical weather archive `https://archive-api.open-meteo.com/v1/archive` (ERA5, `hourly=temperature_2m,relative_humidity_2m,precipitation`, `daily=precipitation_sum`, `timezone=auto`) and the geocoding API `https://geocoding-api.open-meteo.com/v1/search` | one reduced day of readings per (venue, match day) and the venue coordinates (X-2); read by nothing in the models today — every weather family is a recorded null | **CC BY 4.0, free for non-commercial use, no key.** Verified at the source on **2026-09-05**: the terms page says *"You may only use the free API services for non-commercial purposes"* and *"The data obtained through the API is provided under the terms of the CC-BY 4.0 licence"*, with the free limits *"Less than 10'000 API calls per day, 5'000 per hour and 600 per minute"*; the historical-weather docs page lists `apikey` as *"Only required to commercial use to access reserved API resources for customers"*; and the archive endpoint answered HTTP 200 to a keyless request. The pricing page's *"Historical, climate, ensemble, and satellite radiation APIs require the Professional API Plan or higher"* is a statement about the **commercial** plans (it sits under "Using the Standard API Plan can I use historical … data?"), not about the free non-commercial endpoint — the three pages are consistent read that way, and the endpoint behaves that way. The underlying ERA5 data is Copernicus's, CC-BY, citation DOI `10.24381/cds.adbb2d47` | Permitted with attribution (CC BY 4.0), which `reference-data/README.md` carries beside the ERA5/Copernicus citation | **Non-commercial use only.** This prototype is non-commercial, so the recurring cost is zero — but weather would be the **first input in the system to carry a recurring cost if it ever went commercial** (Open-Meteo's commercial plans are the paid route), which [PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md) records beside P0-4's zero-cost line. Because redistribution is permitted and re-acquisition is a rate-limited pass over ~20,000 venue-days, the reduced days and the curated coordinates are **committed** under `reference-data/`; see § Recovering the external data |

**Sources evaluated and rejected** on licence or cost — Betfair's historical-data service,
The Odds API, OddsPortal, OddsMatrix, aussportsbetting.com — are recorded with their terms
and prices in [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-4, and are not repeated
here because nothing reads them.

---

## Recovering the external data

None of the acquired data lives only in a database. Purging and rebuilding the database is
a normal thing to do here, `make dev-purge` deletes `output/`, and a fresh clone starts with
neither — so each external source has a stated way back. What differs per source is *where*
the copy is allowed to live, and that is decided by the licence register above, not by
convenience: the one source whose licence permits redistribution and whose re-acquisition
is expensive is the one tracked in git.

| source | how to get it back | why that way |
|---|---|---|
| **Wikidata biographies** | `make restore-player-biographies` — rebuilds `player_biography` from `reference-data/`, with no network call | CC0, so a copy may be committed; re-acquiring is a rate-limited SPARQL pass over every player in the archive, which is the one cost worth never paying twice |
| **Cricsheet people register** | Committed beside it as `reference-data/cricsheet-people-register.csv`; the same restore reads it | ODC-By permits redistribution with the licence made clear (it is, in `reference-data/README.md`). Without it the lookups key to nothing, so committing one and not the other would preserve neither |
| **Betfair BBL/WBBL odds** | Re-download the season CSVs from `betfair-datascientists.github.io/data/dataListing/` into `data/market-odds/` (git-ignored), then `make evaluate MARKET_ODDS_DIR=data/market-odds` | **Not committed, deliberately: the page grants no licence at all.** Only the numbers derived from them — AUC, Brier, coverage — are published, in the harness report. The download is free and unmetered, so nothing is lost by re-fetching |
| **Open-Meteo ERA5 days and venue coordinates** | `make restore-venue-weather` — rebuilds `venue.latitude/longitude/timezone/city/country` and `venue_weather` from `reference-data/venue-geocoding.csv` and `reference-data/era5-venue-days.jsonl`, with no network call | CC BY 4.0, so a copy may be committed with attribution; re-acquiring is ~4,600 paced archive calls over every venue's match days plus a hand-curated pass over 112 venues the geocoder placed wrongly or not at all, which is not a thing to do twice |
| **Cricsheet match archive** | `POST /ops/data/fetch`, then extract and import (§ Acquiring a dataset) | No licence is stated at the source, so nothing that reproduces it is committed. It is also ~4 GB, which is not a thing to put in git even if the terms allowed it |

### Restoring the biographies

The restore assumes the schema and the players are already there — it writes one
`player_biography` row per row of `player`, so run it *after* migrations and the Cricsheet
import, not before:

```
make migrate                       # schema
make cricsheet-import              # players
make restore-player-biographies    # biographies, offline
```

`-offline` refuses a remote register and the command is handed no Wikidata client at all,
so the run cannot reach either source. An id the snapshot has no answer for is counted and
logged as `unanswered` rather than guessed at or cached as a miss — that is the signal to
run `make player-biographies` once, online, to acquire the few ids a newer archive added.

Verified on 2026-09-05 against a scratch database (`cricket_flow_test`, the one
`SkipUnlessScratchDatabase` exists to insist on): the snapshots rebuilt all 13,662 rows —
7,178 matched, 6,967 with a date of birth — asking Wikidata nothing, and every column but
`fetched_at` hashed identical to the live table.

### Restoring the weather

The same shape, one table later: `venue` rows must exist (the days key through them), so
run it after the Cricsheet import:

```
make migrate                       # schema, including 0012
make cricsheet-import              # venues
make restore-venue-weather         # coordinates and venue_weather, offline
```

`--offline` opens no client at all: a venue the table has no row for, or a match day the
file has no line for, is counted as `unanswered` in the report the command prints, which
is the signal to run `make venue-weather` once, online, for the days a newer archive added.

Verified on 2026-09-05 against a scratch database (`cricket_flow_test`, seeded with the
live database's 896 venue names and nothing else): `make restore-venue-weather` with
`POSTGRES_DB=cricket_flow_test` placed all 896 venue rows and wrote 19,909 `venue_weather`
rows across 895 venue ids — the 19,561 snapshot keys, four spelling pairs of one ground
each landing on both rows — every one with 24 hourly values and 7 prior-day totals, asking
Open-Meteo nothing; a row read back (Wankhede Stadium, 2011-04-02) matches its snapshot
line value for value. The live database was read for the venue names and not written.
Re-verified on 2026-09-20 after DATA-01 re-placed 22 venues: the same command against the
same scratch database placed all 896 venue rows and wrote 20,001 `venue_weather` rows with
no network call, and `M Chinnaswamy Stadium` on 2007-06-06 read back matches its snapshot
line value for value in `Asia/Kolkata`; the live database was not written.
Both runs predate IMPORT-08: the "four spelling pairs of one ground" above are exactly the
four pairs `0021_venue_identity.sql` merges, so after that migration the restore places 892
venue rows, not 896, and each of those four grounds takes its weather on one id rather than
two. The snapshot itself is keyed by `venue_key` and does not change.
Note the default `WEATHER_CRICSHEET_DIR` is `data/go-app/cricsheet` relative to the
repository; name it when the archive lives elsewhere.
