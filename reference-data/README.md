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
| **Source** | Open-Meteo geocoding API (`https://geocoding-api.open-meteo.com/v1/search`), asked about the city Cricsheet names beside the venue (`info.city`) or the venue name's own parts, the country chosen by the sides that played there (*How a row is chosen*, below); **112 rows then placed by hand** (`note` starts `hand-curated`) where the geocoder placed a ground wrongly (Lincoln, Nebraska for Lincoln, Canterbury; Bangalore Town, Sindh for the Chinnaswamy) or not at all |
| **Licence** | The geocoding data is Open-Meteo's, free for non-commercial use under **CC BY 4.0**; the hand placements are this project's |
| **Captured** | **2026-09-05**, by X-2's backfill (`make venue-weather`) and the curation pass recorded in each row's `note`; **re-placed 2026-09-20** under DATA-01 (`docs/AUDIT_FINDINGS.md`): 22 rows moved, 2 by the corrected chooser and 20 by hand, and every row's `countries_voted` re-counted; **re-counted and re-placed 2026-09-25** under DATA-03 and DATA-02: 81 rows' votes changed when the competition table stopped voting for the country a tour is named after, and 7 rows moved off a country centroid onto the city the ground is in |
| **Size** | 892 venue keys (896 archive spellings), **892 mapped**: 673 in the archive's top-voted country, 59 in a country the top vote does not out-vote beyond chance, 44 in a country nobody voted for against a weak top vote, 4 with no vote at all, 112 placed by hand — each kind but the first says so in its `note`. A row is a city-level placement, which is the data's own resolution (see below), except **21 with an empty `admin1`** — 9 of them the country's own point — where the ground is in a city-state (Singapore, Gibraltar) or a dependent territory the geocoder holds no place inside (Jersey, Guernsey, Bermuda, Antigua, Grenada, Rwanda). Those are what is left of DATA-02: the chooser now holds a country answer and takes a place in the same country when a later query finds one, which moved 7 rows (the Kensington Oval off the Barbados centroid onto Bridgetown, the Queen's Park Oval off Trinidad onto Port of Spain), and keeps the centroid when nothing better exists rather than taking the Californian *Lords* that answers a Bermudian ground's name. The one ground neither the archive nor the geocoder could place, `F B Colony Ground`, was placed from Wikipedia's alias for it (the former Alembic No 2 Ground, Vadodara) with the provenance in its `note` |
| **Read by** | `ml-service/ml/weather/geocoding.py`; written onto `venue.latitude/longitude/timezone/city/country` by the restore |

**A row, once written, is never rewritten by a run**: `make venue-weather` asks only about
venues the file has no row for. That is what makes the file curated rather than cached —
correct a row by hand, say so in its `note`, and it stays corrected. ERA5 is a 0.25°
reanalysis (~28 km cells), so a ground's city places it as well as its gates would; the
`query` column says what was asked and `place` what answered.

**The key does not merge spellings of one ground, deliberately.** `venue_key` folds case,
accents and punctuation and nothing else — the same fold `venues.NormalizeName` applies to
`venue.normalized_name` in the database (IMPORT-08), which is why the two tables join. It
does *not* key on the first comma-part: `County Ground` is **nine different grounds** in
this archive (bare, Bristol, Chelmsford, Derby, Hove, New Road, New Road/Worcester,
Northampton, Taunton), and Bristol and Derby are real cities, so a rule that dropped the
trailing parts when they look like a place would make those nine one. So four spellings of
the Kensington Oval remain four rows; what DATA-02 fixed is that they no longer sit in two
different places. Merging them into one row needs coordinates or a person, not a longer
delimiter rule, and is not done here.

**How a row is chosen.** The archive's votes are on the row, in `countries_voted`
(`BD:105 ZW:37 IN:33 …`, most votes first — one vote per fixture for each international
side's home country and for a domestic competition's country). A candidate in the top-voted
country is taken, with no note. A candidate anywhere else is refused as a homonym when the
top vote's lead over its country is one chance would not produce — a one-sided sign test at
p < 0.05, so Bangladesh's 105 votes refuse the Punjab Mirpur's 33 and Grenada's 15 refuse a
Bermuda with none — and the next query is asked; when the lead is not evidence either way
(an associate ground everyone visits, a neutral venue, a one-series ground) the candidate is
kept and the `note` says how the votes fell. A venue whose every candidate is refused is
written `unmappable` with the refused places named in its `note`, which is what a hand row
then corrects. `ml-service/tests/test_weather.py` asserts this over the whole file.

## `era5-venue-days.jsonl`

One reduced day of weather per (venue key, match day), one JSON object per line: the
day's 24 local hours of `t` (temperature, °C), `rh` (relative humidity, %) and `p`
(precipitation, mm) in the row's `tz`, and `p7`, the daily precipitation totals of the
seven days before it, oldest first. Multi-day matches are keyed by their first day.

| | |
|---|---|
| **Source** | Open-Meteo historical weather API (`https://archive-api.open-meteo.com/v1/archive`), which serves the **ERA5** reanalysis: Hersbach, H. et al. (2023): *ERA5 hourly data on single levels from 1940 to present.* Copernicus Climate Change Service (C3S) Climate Data Store (CDS), DOI [10.24381/cds.adbb2d47](https://doi.org/10.24381/cds.adbb2d47) |
| **Licence** | **CC BY 4.0** — Open-Meteo's terms: *"The data obtained through the API is provided under the terms of the CC-BY 4.0 licence"*, for **non-commercial use** of the free API; the ERA5 data is Copernicus's under its CC-BY licence. Recorded per stored row as `venue_weather.source_license = CC-BY-4.0` |
| **Captured** | **2026-09-05**, by X-2's backfill (`make venue-weather`), 4,638 archive calls (one per cluster of match days at a venue, days within 45 days fetched together) over every venue's match days, across three runs — the service's hourly quota is weighted by the data a call returns, and the client now waits it out; **2026-09-20**, 182 further calls after DATA-01 re-placed 22 venues: their 602 cached days, fetched at another continent's coordinates, were dropped and fetched again at the corrected ones, together with the match days the archive had gained since; **2026-09-25**, 881 calls under DATA-04 and DATA-02: the 2,523 days whose hours were an hour out (`timezone=auto` stamps a whole range with the offset the zone is on *at the moment of the call*) and the 226 days of the 7 venues that moved off a country centroid, dropped and fetched again — the other 16,904 lines are untouched, and sixteen unaffected days re-fetched with the corrected client came back identical to them value for value |
| **Size** | **19,653 venue-days, 0 misses**, covering **22,905 of the archive's 22,905 matches (100 %)** as of 2026-09-25. 8.8 MB — the whole local day is kept rather than only the hours a session rule reads today (which would save ~1.5 MB), because the whole day is what lets a window be recomputed if a start-time rule is corrected later |
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

**The hours are local, built here rather than asked for.** The call asks Open-Meteo for
**UTC** and `ml/weather/archive.py` places each hour on the venue's clock from the IANA zone
in `venue-geocoding.csv`, resolving the offset per hour. The service's own `timezone=auto`
returns a single `utc_offset_seconds` for the whole range — the offset the zone is on *at the
moment of the call*, not at the dates asked for — which is what left 2,523 of these days an
hour out until DATA-04 (`docs/AUDIT_FINDINGS.md` § 9). A zone whose offset is not a whole
number of hours (`Asia/Kolkata`'s +5:30) is floored to the whole hour ERA5 is gridded on,
which is what the service does for its own local grid. `p7` is each prior local day's own
hourly total, summed here for the same reason.

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
