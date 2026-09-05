# Reference data — the acquired input that is tracked in git

Everything else this system reads from outside is fetched onto each box into `data/` or
`output/`, both of which are git-ignored. That is right for the things a box can simply
fetch again. It is wrong for the things it cannot: a purge (`make dev-purge` deletes
`output/`; rebuilding the database drops `player_biography`) or a fresh clone would leave
no copy anywhere, and re-acquiring means a rate-limited pass over every player in the
archive.

So the files here are tracked. All are redistributable — that is what decides it, not
convenience. The first two rebuild `player_biography` with no network call, the last two
rebuild the venue coordinates and `venue_weather` the same way:

```
make restore-player-biographies
make restore-venue-weather
```

Which sources may and may not be committed is settled in
[docs/config-and-data.md § Data-source licence register](../docs/config-and-data.md#data-source-licence-register).
What is *not* here, and why, is in § Recovering the external data in the same file — the
Betfair odds (no licence granted) and the Cricsheet match archive (no licence stated, ~4 GB,
re-fetchable) are both deliberately absent.

## `wikidata-player-lookups.jsonl`

Every answer Wikidata has given about a player, one JSON object per line, keyed by
ESPNcricinfo id.

| | |
|---|---|
| **Source** | Wikidata Query Service (SPARQL), properties `P2697`, `P569`, `P570`, `P2032`, `P741`, `P552`, `P2545` |
| **Licence** | **CC0 1.0** — public domain dedication, no conditions, redistribution permitted. Recorded per stored row as `player_biography.source_license = CC0-1.0` |
| **Captured** | **2026-09-04**, by X-1a's backfill (`make player-biographies`) |
| **Size** | 13,707 lookups — 7,184 hits, 6,523 misses |
| **Written by** | `go-app/internal/biography` (`Cache`), append-only |

**A miss is a recorded answer, not a gap.** A line carrying only a `cricinfo_id` means
"Wikidata has no item carrying this id" — which is a fact about Wikidata, established by
asking. It is stored precisely so a later run does not ask again; for this dataset the
misses are almost half the file. Do not prune them, and do not treat a missing line and a
miss line as the same thing: a missing line means the id has never been asked about.

Fields beyond `cricinfo_id` are present only when Wikidata had them: `qid`, `birth_date`
(6,973 lines), `bowling_style_raw` (270), `death_date` (25), `batting_hand_raw` (19),
`career_end_date` (3). The file is append-only JSON Lines rather than one JSON document so
that an interrupted run costs at most the batch in flight.

## `cricsheet-people-register.csv`

Cricsheet's people register, verbatim: one row per person, mapping the identifier the match
files carry to that person's id on the sites that have one. It is the join between a player
in this database and a lookup in the file above — without it the lookups key to nothing.

| | |
|---|---|
| **Source** | `https://cricsheet.org/register/people.csv` |
| **Licence** | **ODC-By 1.0** (Open Data Commons Attribution), stated on the register page. Redistribution is permitted provided the licence is made clear and notices are kept intact — which is what this section is |
| **Captured** | **2026-09-05** |
| **Size** | 18,507 people |
| **Read by** | `go-app/internal/biography` (`ParseRegister`), by header name rather than column position |

**Attribution.** This file is Cricsheet's, made available under the Open Data Commons
Attribution License 1.0 (<http://opendatacommons.org/licenses/by/1.0/>). Any public use of
it, or of work produced from it, must say so.

It is pinned here rather than re-fetched for a second reason beyond being offline: the
register grows, so a restore that fetched a newer one would rebuild against a different
population and quietly not reproduce the recorded coverage figures.

## `venue-geocoding.csv`

Where each of the archive's grounds is: one row per venue key (the Cricsheet venue name
folded the identity way — case, accents and punctuation — so two spellings of one ground
share a row), with the coordinates, country and IANA timezone the weather is fetched at.

| | |
|---|---|
| **Source** | Open-Meteo geocoding API (`https://geocoding-api.open-meteo.com/v1/search`), asked about the city Cricsheet names beside the venue (`info.city`) or the venue name's own parts, the country chosen by the sides that played there; **140 rows then placed by hand** (`note` starts `hand-curated`) where the geocoder placed a ground wrongly (Lincoln, Nebraska for Lincoln, Canterbury) or not at all |
| **Licence** | The geocoding data is Open-Meteo's, free for non-commercial use under **CC BY 4.0**; the hand placements are this project's |
| **Captured** | **2026-09-05**, by X-2's backfill (`make venue-weather`) and the curation pass recorded in each row's `note` |
| **Size** | «GEO_ROWS» venue keys — «GEO_MAPPED» mapped, «GEO_UNMAPPED» `unmappable` (left so on purpose rather than guessed: the archive names no city for it and its name places it nowhere) |
| **Read by** | `ml-service/ml/weather/geocoding.py`; written onto `venue.latitude/longitude/timezone/city/country` by the restore |

**A row, once written, is never rewritten by a run**: `make venue-weather` asks only about
venues the file has no row for. That is what makes the file curated rather than cached —
correct a row by hand, say so in its `note`, and it stays corrected. ERA5 is a 0.25°
reanalysis (~28 km cells), so a ground's city places it as well as its gates would; the
`query` column says what was asked and `place` what answered.

## `era5-venue-days.jsonl`

One reduced day of weather per (venue key, match day), one JSON object per line: the
day's 24 local hours of `t` (temperature, °C), `rh` (relative humidity, %) and `p`
(precipitation, mm) in the row's `tz`, and `p7`, the daily precipitation totals of the
seven days before it, oldest first. Multi-day matches are keyed by their first day.

| | |
|---|---|
| **Source** | Open-Meteo historical weather API (`https://archive-api.open-meteo.com/v1/archive`), which serves the **ERA5** reanalysis: Hersbach, H. et al. (2023): *ERA5 hourly data on single levels from 1940 to present.* Copernicus Climate Change Service (C3S) Climate Data Store (CDS), DOI [10.24381/cds.adbb2d47](https://doi.org/10.24381/cds.adbb2d47) |
| **Licence** | **CC BY 4.0** — Open-Meteo's terms: *"The data obtained through the API is provided under the terms of the CC-BY 4.0 licence"*, for **non-commercial use** of the free API; the ERA5 data is Copernicus's under its CC-BY licence. Recorded per stored row as `venue_weather.source_license = CC-BY-4.0` |
| **Captured** | **2026-09-05**, by X-2's backfill (`make venue-weather`), «CALLS» archive calls over every venue's match days |
| **Size** | «DAYS» venue-days and «MISSES» misses, covering «COVERED» of the archive's 22,818 matches («COVERED_PCT»); «SIZE_MB» MB |
| **Written by** | `ml-service/ml/weather/archive.py` (`WeatherCache`), append-only, flushed after every call |

**Attribution.** Weather data by [Open-Meteo.com](https://open-meteo.com/), CC BY 4.0;
contains modified Copernicus Climate Change Service information (ERA5). Any public use of
this file, or of work produced from it, must say so.

**A miss is a recorded answer, not a gap.** A line carrying `miss` means the archive
holds no hourly readings for that venue and day — established by asking — and is stored
so a restore does not ask again. A match day younger than a week is not asked at all
(the archive trails the present), so it has no line and is counted as `unanswered` by the
report, which is the signal to run `make venue-weather` once the archive has caught up.

**The session window is not in the file.** When a match started is inferred per match by
`ml-service/ml/weather/sessions.py` from competition and format norms, and recorded as a
rule id; the file holds the whole local day so any window can be recomputed without
asking again if a rule is corrected.

## Refreshing these

Neither file is on the cadence — a date of birth does not change and a bowling style rarely
does. Refresh them when the player registry has grown enough to matter:

```
make player-biographies        # asks Wikidata only about ids not already answered
cp output/player-biographies/wikidata-lookups.jsonl reference-data/wikidata-player-lookups.jsonl
curl -o reference-data/cricsheet-people-register.csv https://cricsheet.org/register/people.csv
```

The weather files are appended to in place — they are the cache — so a refresh is the
acquisition command itself, after a newer archive has been imported:

```
make venue-weather             # geocodes new venues, fetches the match days the file lacks, prints coverage
```

Then update the capture dates and counts above in the same commit, because a snapshot whose
recorded date is wrong is worse than one that is merely old.
