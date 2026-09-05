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
- `formats`
  - `treat_t20i_as_subset` (bool) — treat T20 between international teams as T20I.
  - `international_teams` (list) — ICC national teams for the subset rule.
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
  deadline exists so a hung one does not hold the semaphore forever).
- `outputs.artifacts_dir` — the artifacts root, holding `runs/` and `current_run.json`.
- `ml.formats` — the format codes to train and serve. Checked against go-app's canonical list
  by `make frontend-backend-sync-check`.
- `ml.ratings_max_age_days` (14) — H-11: a live prediction against ratings older than this is
  refused with `RATINGS_STALE`. `XI_RATINGS_MAX_AGE_DAYS` overrides it; 0 turns the check off.

**Environment:** `ML_SERVICE_CONFIG`, `ML_SERVICE_OUTPUT_DIR`, `MODELS_DIR`, `ENABLE_HOT_RELOAD`,
`ADMIN_API_KEY`, `XI_RATINGS_MAX_AGE_DAYS`, `MAX_CONCURRENT_TRAINING_JOBS`,
`ML_MARKET_ODDS_DIR`.

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
| **Cricsheet people register** — `https://cricsheet.org/register/people.csv` | the ESPNcricinfo id per player, the join key for X-1a | **ODC-By 1.0** (Open Data Commons Attribution). The register page states: *"This dataset is made available under the Open Data Commons Attribution License: http://opendatacommons.org/licenses/by/1.0/."* and *"You must attribute any public use of the dataset, or works produced from the dataset, in the manner specified in the license."* | Permitted, with the licence made clear and notices kept intact: *"For any use or redistribution of the dataset, or works produced from it, you must make clear to others the license of the dataset and keep intact any notices on the original dataset."* | Attribution is owed on any public use, including works produced from it. The comment in `go-app/internal/biography/biography.go` used to describe the register as **ODbL**, which carries a share-alike term ODC-By does not; that was B-6 in [BUG_BACKLOG.md](BUG_BACKLOG.md) and the comment now states ODC-By 1.0 and points back at this table |
| **Wikidata** — the SPARQL query service; properties `P2697`, `P569`, `P570`, `P2032`, `P741`, `P552`, `P2545` | player biographies (X-1a) | **CC0 1.0.** Wikidata:Licensing states: *"All structured data in the main, property and lexeme namespaces is made available under the Creative Commons CC0 License"* | Permitted, without conditions | Nothing is owed; the licence is recorded per row as `player_biography.source_license` (`CC0-1.0`). The query service asks for an identifying `User-Agent`, which the backfill sends |
| **Betfair Exchange season summaries** — BBL / WBBL Match Odds CSVs at `betfair-datascientists.github.io/data/dataListing/` | closing odds, read by `make evaluate` only as a yardstick (X-4); never a feature | **No licence granted.** The page carries a warranty disclaimer and nothing else: *"By downloading this data, you acknowledge and agree that: (a) Betfair does not make any representations, or give any warranties, as to the accuracy or completeness of the data provided; and (b) you use the data at your own risk, and Betfair will not be liable for any loss suffered in using the data."* | **Not permitted** — no right is granted, so none is assumed. The files are cached under `data/market-odds/` (git-ignored) per machine and never committed | Usable as a local measurement, which is all X-4 does with them; the numbers derived from them (AUC, Brier, coverage) are published in the harness report, the rows are not |

**Not yet used, and to be verified before it is — Open-Meteo (X-2).** Read on the same
date: the terms page says *"You may only use the free API services for non-commercial
purposes"*; the licence page says *"API data are offered under Attribution 4.0
International (CC BY 4.0)"*; the ERA5 dataset page at the Copernicus Climate Data Store
names a CC-BY licence and a citation DOI (`10.24381/cds.adbb2d47`). The pricing page also
says *"Historical, climate, ensemble, and satellite radiation APIs require the Professional
API Plan or higher"*, and whether that governs the free non-commercial endpoint or only the
paid customer endpoints could not be settled from the pages. X-2's worker settles it from
the source before acquiring anything, adds the row here, and stops with a recorded finding
if the answer is "paid" ([EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-2).

**Sources evaluated and rejected** on licence or cost — Betfair's historical-data service,
The Odds API, OddsPortal, OddsMatrix, aussportsbetting.com — are recorded with their terms
and prices in [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-4, and are not repeated
here because nothing reads them.
