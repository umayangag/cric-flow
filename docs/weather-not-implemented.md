# Weather: planned, not implemented

Weather is wanted as a model input but is **not implemented**. This note records what exists, what does not, and what building it would take, so the intent survives without dead code standing in for it.

## Current state

Nothing populates weather. There is no provider client in the repo, and there never was — the code removed in C2-2 was a queue and a set of repository methods with no fetcher behind them.

Two things blocked it, and both still do:

1. **No venue coordinates.** `venue.latitude` / `venue.longitude` exist in the schema but are empty: after a 21,253-file Cricsheet import, **0 of 877 venues** have coordinates. Cricsheet gives venue names, not positions.
2. **No weather source.** Nothing fetches observations for a (venue, date).

## What was removed, and why it was not a head start

| Removed | Why |
|---|---|
| `internal/db/repo_weather.go` | `UpsertWeather` — unreachable. Trivial CRUD. |
| `internal/db/repo_weather_backfill.go` | `EnqueueMissingWeatherJobs` — a backfill, re-derivable in one `INSERT … SELECT`. |
| `internal/db/repo_weather_job.go` | Queue drain — unreachable. |
| `internal/db/repo_venue.go` | Geocode read/write — unreachable. |
| `internal/weather/service.go` | Enqueue helper. Its only caller was the importer. |
| `cricsheet.WeatherClient` port + mock | A seam with one dead implementation. |
| `--placeholders-weather`, `--weather-enqueue` | See below. |

None of it was the hard part. The hard parts — geocoding 877 venues and choosing/ingesting a weather source — were never started, and the removed code would not have shortened them.

**The placeholder path was doubly inert.** `--placeholders-weather` defaulted to off, so `weather_data` was empty after a full import. Even when enabled it wrote rows with `session = 'inning1'/'inning2'`, while every export query filters `session = 'batting'/'bowling'` — so those rows could never have matched a join.

**The enqueue path was live but pointless.** `--weather-enqueue` defaulted to **on**, so a full import left **21,043 rows in `weather_job`** that nothing ever read.

## What was kept

- **`weather_data` and `weather_job` tables**, in `0001_baseline.sql`. They cost nothing and encode the session/queue design.
- **`venue.latitude` / `longitude` / `city` / `country` / `timezone`** columns, ready for a geocoding pass.

## Building it

1. **Geocode venues.** 877 rows, one-off. Nominatim or a commercial geocoder; write to the existing `venue` columns. Needs a normalized-name match and a manual pass for ambiguous grounds.
2. **Pick a source.** [Open-Meteo's historical archive](https://open-meteo.com/en/docs/historical-weather-api) is free, needs no key, covers 1940–present globally, and keys on lat/lon + date — a good fit for ~21k historical matches.
3. **Fetch and store.** Re-enqueue with an `INSERT … SELECT` over matches lacking a job, then drain the queue into `weather_data` with one row per session. Rate-limit and make it resumable.
4. **Re-add the features.** Seven names per section back into `configs/feature_vectors.json` (batting, bowling, fielding) and into `EXTRAS_FEATURE_COLS`, `WIN_FEATURE_COLS`, `INNINGS_FEATURE_COLS`.
5. **Re-export and retrain all six models.** Required regardless — models trained without weather cannot use it.

Step 5 is why the features were removed rather than left as constant zeros: the retrain is mandatory whenever real data arrives, so keeping the slots bought nothing while costing 17–47% constant-zero inputs per model in the meantime.
