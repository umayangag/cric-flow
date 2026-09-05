# Reference data — the acquired input that is tracked in git

Everything else this system reads from outside is fetched onto each box into `data/` or
`output/`, both of which are git-ignored. That is right for the things a box can simply
fetch again. It is wrong for the things it cannot: a purge (`make dev-purge` deletes
`output/`; rebuilding the database drops `player_biography`) or a fresh clone would leave
no copy anywhere, and re-acquiring means a rate-limited pass over every player in the
archive.

So the two files here are tracked. Both are redistributable — that is what decides it, not
convenience — and between them they rebuild `player_biography` with no network call:

```
make restore-player-biographies
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

## Refreshing these

Neither file is on the cadence — a date of birth does not change and a bowling style rarely
does. Refresh them when the player registry has grown enough to matter:

```
make player-biographies        # asks Wikidata only about ids not already answered
cp output/player-biographies/wikidata-lookups.jsonl reference-data/wikidata-player-lookups.jsonl
curl -o reference-data/cricsheet-people-register.csv https://cricsheet.org/register/people.csv
```

Then update the capture dates and counts above in the same commit, because a snapshot whose
recorded date is wrong is worse than one that is merely old.
