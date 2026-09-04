# External data plan: what the archive cannot contain, gated the usual way

**Status: open.** Written 2026-09-03, after [FOLLOW_UP_PLAN.md](FOLLOW_UP_PLAN.md) closed
with three recorded nulls (A-1 venue context, A-2 chase response, A-3 lineup features).
Those nulls sharpen this plan's premise: the models are near the limit of what the
ball-by-ball archive contains, so the only candidates worth effort carry information the
archive **cannot** hold. Every item here is an experiment first and a feature only if its
gate clears; a recorded null closes its question and is a complete outcome. The inherited
rules apply throughout: choices on walk-forward folds, the locked window scored once and
never guiding a choice (H-19), H-21 unknowable-inputs audit for every candidate feature,
H-22 (width beside coverage), H-23 (varies / fixed / decides written before a gate runs),
H-24 (boundary literals in the generated contract), never commit to main.

Run each item's prompt in a fresh chat with the model noted; each ends by handing over
the push and PR commands. Order: **X-4 → X-1a → X-1b → X-3 → X-2.** X-4 went first
because it was meant to price the whole hunt: if the display model already sat at market
accuracy, the remaining items would be curiosities. **It did not price it** — the only
licence-clean free odds reach 4.2 % of one format, and the measured gap's interval spans
zero (§ Record). X-1a → X-2 therefore proceed on their own merits, unpriced, and the
cheapest way to price them later is coverage bought rather than code written. D-12 (a
defect fix, not an experiment) is runnable at any time and does not wait for X-1a.

**Explicit non-goals**, so effort is not re-spent: official rankings (redundant with the
system's own Elo), pitch reports (no structured source exists), injury/fitness data (not
freely or reliably available), squad/availability feeds (a product problem —
[PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md) Phase 2), other ball-by-ball archives (Cricsheet
is the free canonical source), and **odds as a model input** — X-4 uses the market as a
yardstick, never as a feature.

---

## X-4 — the market benchmark (model: Opus; this is PRODUCT_ROADMAP P0-1) — **measured, and the answer is "not yet resolvable"**

*Done on `feat/x-4-odds-benchmark`, under a **no-paid-data** rule: "acceptable cost" was
defined as zero, so only freely obtainable, licence-clean sources were eligible. The source
review, the join and the numbers are in § Record of outcomes below. The short version: a
free, licence-usable closing-odds series exists for exactly two competitions (BBL and WBBL),
it joins to 99.3 % of the markets it carries, and it reaches **4.2 % of T20 and 0 % of every
other format** — so the market gap is measured but its interval spans zero, and X-1…X-3 are
not priced by it. Every source with international coverage was paid or behind a gambling
account and was rejected; the costs are recorded as evidence, not as options.*

**What.** Join historical closing odds onto our matches and score the market's implied
probabilities in the harness beside the display model. **Gate it answers:** how far from
the practical ceiling each format actually is — the number that prices X-1…X-3 and the
sentence marketing may use.

```
Read docs/EXTERNAL_DATA_PLAN.md (the X-4 row) and docs/PRODUCT_ROADMAP.md §2 (this is
P0-1). Read how the harness scores the display model (ml/xi/evaluate.py) and the H-23
gate registry (ml/xi/gates.py). Branch off main as feat/x-4-odds-benchmark. Inherited
rules: locked window scored once (H-19); H-23 triple before the gate; H-24 for any new
wire literal; never commit to main; anchored edits; conventional commits with scope.

Do X-4: the market as a yardstick, never as a feature.
1. Source decision, documented before code: find a historical closing-odds dataset for
   cricket whose license permits this use (candidates to evaluate honestly: academic
   datasets, archived odds portals' published files, purchasable sets — record cost).
   If no clean source exists at acceptable cost, RECORD THAT FINDING in the X-4 row
   with what was evaluated and stop — that is a completed gate, not a failure.
2. Join: odds rows to matches by date, format and the two sides, through the identity
   layer (club + gender, the D-10 contract); unmatched rows counted and reported, never
   guessed. The joined coverage per format is part of the result — a benchmark that
   covers 30% of matches says so on its face.
3. Score: de-vig the closing odds to implied probabilities (state the method; the
   proportional method is fine, say so); score them in the harness beside the display
   model on the same matches only — AUC and Brier, per format, walk-forward folds and
   the locked window labelled. H-23 triple: varies the probability source, fixed the
   matches and the scoring, decides nothing automatically — this gate INFORMS (it is a
   measurement, not a keep/drop switch).
4. Report: a new harness section (market_benchmark) with the per-format table and the
   deltas; glossary entries for what it shows (L-1's completeness gate will demand
   them); the odds data itself cached under a documented path, never committed if the
   license forbids redistribution.
5. Write the result into docs/EXTERNAL_DATA_PLAN.md (X-4 row and §Record) and
   docs/PRODUCT_ROADMAP.md (P0-1 row), including the honest sentence the numbers
   support.

Acceptance: the benchmark table exists per format with joined-coverage stated, or the
no-clean-source finding is recorded; nothing anywhere consumes odds as a feature; make
check-all green; coverage gates never move down. Then stop and hand over the push and
PR commands.
```

## X-1a — player biographies acquired from Wikidata (model: Opus) — **acquired and measured**

**What.** Date of birth, batting handedness, bowling style and — where Wikidata carries
it — the career end date or a retirement statement, for the player registry,
joined via the ESPNcricinfo id that Cricsheet's people registry and Wikidata both carry
(CC0-licensed). Acquisition and coverage only — no features. **Gate:** coverage worth
building on — matched biography with DOB for players covering ≥ 80 % of match
appearances in each limited-overs format (report the true figure per format and gender
whatever it is).

*Done on `feat/x-1a-player-biographies`; the measured coverage and the three recorded
nulls are in § Record of outcomes below. The short version: **date of birth clears the
gate** at 85.3 % of appearances overall, and in every limited-overs format and gender
except women's T20 (61.4 %). **Bowling style, batting handedness and career end do not
exist in Wikidata at usable scale** — 265, 18 and 3 of 13,662 players — so X-1b's matchup
family has no labels to build from and only its age family is runnable.*

```
Read docs/EXTERNAL_DATA_PLAN.md (X-1a) — acquisition only; X-1b owns features. Read how
the importer consumes Cricsheet's registry (people ids) and the identity docs. Branch
off main as feat/x-1a-player-biographies. Rules: never commit to main; anchored edits;
conventional commits; H-24 for any new wire literal.

Do X-1a:
1. A backfill script joining the player registry to Wikidata through the ESPNcricinfo
   id (Wikidata carries it as a property; batch SPARQL or the entity API, cached
   locally, resumable, polite rate limits). Fields: date of birth, batting handedness,
   bowling style (mapped to a small controlled vocabulary — pace/medium/off-spin/
   leg-spin/left-arm-orthodox/left-arm-wrist, plus unknown), and the career end date /
   retirement statement where present (work-period-end and equivalent properties;
   coverage will be spotty — record it, never infer it). License: CC0 — note it.
2. Storage: a player_biography table (forward migration, next number) keyed by the
   registry id, written by the backfill; the importer does not change. The backfill
   becomes a documented optional step beside the cadence (it changes rarely; weekly
   with the cadence or on demand).
3. The coverage report, per format and gender, weighted by match appearances: matched /
   DOB present / style present / career-end present; unmatched players listed by appearance count so the
   top gaps are hand-fixable via a small curated overrides file (the venue-geocoding
   pattern). Fix the top gaps if a handful of overrides moves coverage materially.
4. One data-quality check in the ops surface: biography coverage shown beside the
   dataset registry, so staleness and gaps are visible (§8.7's spirit: the data's
   state is on the surface, not in a log).
5. Record the measured coverage in docs/EXTERNAL_DATA_PLAN.md (X-1a row, §Record); the
   X-1b gate reads it.

Acceptance: backfill runs from clean state in one command, resumable; coverage report
committed with the run's figures; migration forward-only; no model or feature changes
anywhere; make check-all green; coverage gates never move down. Then stop and hand
over the push and PR commands.
```

## X-1b — biography features, gated (model: Fable; requires X-1a merged)

**What.** The two mechanisms the archive cannot express: **age** (trajectory — ratings
only see the past; a 21-year-old and a 36-year-old with identical form differ
predictably) and **handedness/style matchups** (left–right combinations, spin-type vs
handedness — the effects with real empirical support, which A-3 could not test for want
of labels). Three families, each with its own gate; nulls per family acceptable.

**X-1a's coverage narrows this before it starts.** Families 1 and 3 (age, age-aware cold
start) have their input: a date of birth for 85.3 % of appearances, and ≥ 80 % in every
limited-overs format and gender bar women's T20 at 61.4 %, which is therefore out of
scope for them. **Family 2 (matchups) has no input at all** — Wikidata states a bowling
style for 265 players and a batting hand for 18 — so it is a recorded null on coverage
before a fold is run, and the honest step is to say so in its row rather than to fit a
model to 1.9 % of the registry. The E5 re-run beside it was the only part of family 2
that did not need labels; it can still be run on its own terms.

```
Read docs/EXTERNAL_DATA_PLAN.md (X-1b and X-1a's measured coverage) and, in
docs/ML_PIPELINE_REARCHITECTURE_PLAN.md, the A-3 record (§FOLLOW_UP_PLAN A-3: matchup
families from inferred labels failed their gates — these are the labelled versions) and
H-10 (cold start). Branch off main as feat/x-1b-biography-features. Rules: folds decide
over three seeds; locked window once (H-19); H-21 audit per family (DOB is static and
knowable; a style label is a current-day fact applied historically — a bowler who
changed style is mislabelled early-career: state this limitation, do not hide it);
H-22; H-23 triple per family before it runs; never commit to main.

Do X-1b, one family at a time:
1. AGE in the performance model: age at match date (and a curvature term or spline) in
   L2-B's inputs. Gate: pinball loss improves beyond the noise floor for runs or
   wickets in at least T20 and ODI, with interval coverage held (H-22). Missing DOB is
   a real category (its own indicator), never an imputed age.
2. MATCHUP features in the win models: side-level aggregates only computable with
   labels — left-right top-order balance, the XI's spin-type coverage against the
   opposition's handedness profile, as-of and per format. Gate: display AUC beyond
   noise with swap monotonicity intact; re-run the lineup-only E5 for T20 beside it
   and report (a pass there flips the scoping via the P-7 machinery; do not expect
   it).
3. AGE-AWARE cold start: the debutant prior shaped by age (a 19-year-old debutant and
   a 34-year-old one regress to different neutrals). Gate: the H-10 measurements
   (debutant-swap distribution) stay bounded AND held-out low-history players' pinball
   improves; nothing changes for players with history.
4. Ship only families that clear their gates; the fold table per family goes in the
   X-1b row either way. H-8 parity extended to whatever ships; glossary entries for
   anything the report now shows; update docs/ml-and-training.md.

Acceptance: per-family fold tables recorded (nulls included); no unknowable input
(H-21 audits written); parity 0.0 on both sources; make check-all green; coverage
gates never move down; docs/EXTERNAL_DATA_PLAN.md updated. Then stop and hand over
the push and PR commands.
```

## X-3 — match stakes, derived from data already held (model: Opus)

**What.** Tournament stage and dead-rubber flags reconstructed from Cricsheet's event
names, match numbers and dates — no new source. Two uses, gated separately: cleaning
the E5 measurement (rotation-heavy dead rubbers are exactly the noise §8.6 blamed), and
a small stakes feature.

```
Read docs/EXTERNAL_DATA_PLAN.md (X-3), §8.6 of docs/ML_PIPELINE_REARCHITECTURE_PLAN.md
(the rotation-noise reading of T20's E5), and what Cricsheet's event fields carry
through the importer. Branch off main as feat/x-3-match-stakes. Rules: folds decide;
H-19; H-21 (stage is knowable pre-match; anything needing the day's OTHER results to
be final is computed strictly as-of); H-23 per gate; never commit to main.

Do X-3:
1. Derivation: per match, as-of stage labels from event name + match number + date
   (group / knockout / final via the vocabulary event names actually use — measure
   and report label coverage; unlabelled stays unlabelled). A dead-rubber flag only
   where a points table is reconstructible as-of; report its coverage honestly.
2. Use 1 — E5 hygiene (a measurement change, not a model change): re-run the T20
   lineup-only E5 excluding (and separately, down-weighting) dead-rubber and knockout
   pairs. H-23 triple: varies the pair filter, fixed the objective and the bar's
   method, decides nothing automatically — it informs whether rotation noise was
   masking signal. If filtered E5 clears the bar, that is evidence for revisiting the
   scoping through the P-7 machinery — record it and say so; do not flip the scoping
   in this PR.
3. Use 2 — a stakes feature in the display model (knockout flag): gate is display AUC
   beyond noise, swap monotonicity intact. Expect a null; record it either way.
4. The derivation lands as as-of columns in the rating pass with parity extended;
   fold tables and both results into the X-3 row; glossary entries if anything ships
   to the report.

Acceptance: label-coverage figures recorded; both gates' tables recorded (nulls
included); parity 0.0; make check-all green; coverage gates never move down;
docs/EXTERNAL_DATA_PLAN.md updated. Then stop and hand over the push and PR commands.
```

## X-2 — weather (model: Fable; last, expect a null)

**What.** Pre-match weather context (day/night, humidity, dew likelihood, rain
probability) from the Open-Meteo ERA5 historical archive, mapped to inferred session
windows — Cricsheet has no start times, so competition/format norms infer them. The
A-1/A-2 nulls predict this fails its gates; it runs last and closes the question with
a number.

```
Read docs/EXTERNAL_DATA_PLAN.md (X-2) and docs/FOLLOW_UP_PLAN.md (the A-1 and A-2
nulls — venue-level context and the chase response both failed, which bounds what
weather can add). Branch off main as feat/x-2-weather-context. Rules: folds decide
over three seeds; H-19; H-21 (every feature computable BEFORE the match — pre-match
window readings or daily values, never in-match observations); H-22; H-23 per family;
H-24 for any new wire literal; never commit to main.

Do X-2:
1. Acquisition: a venue -> lat/lon table (curated file, name-normalised the identity
   way; unmappable venues recorded, not guessed) and a backfill pulling hourly ERA5
   data from the Open-Meteo historical API per match date and venue, cached and
   resumable. Start times are NOT in the data: infer a session window per match from
   competition and format norms (documented mapping with a day/night flag) and record
   the inference so its error is inspectable.
2. Families, H-23 triple each, tested one at a time: day/night flag; pre-match
   humidity and temperature; a dew-likelihood proxy (evening + humidity); rain
   probability. Gates: (a) display AUC beyond noise; (b) simulated totals and chase
   10-90 coverage/width and dispersion ratio, split day vs night (H-22); (c) E2
   unchanged.
3. A family ships only past its gate; if one ships, the backfill joins the cadence
   with a documented forecast-source rule for serving (what serving reads pre-match),
   H-8 parity extended, glossary entries added. If nothing ships, the scripts land
   under scripts/experiments/xi/, the venue geocoding table STAYS (X-1-adjacent
   product value regardless), and the write-up says what was measured.
4. Record per-family tables in the X-2 row either way.

Acceptance: per-family fold tables recorded (nulls included); no feature uses
information unavailable before the match; make check-all green; coverage gates never
move down; docs/EXTERNAL_DATA_PLAN.md updated. Then stop and hand over the push and
PR commands.
```

## D-12 — stale pools, manual pool picking, and the retirement ledger (model: Opus) — **fixed**

*Fixed on `fix/d-12-pool-and-retirement`. The measurement tables and what shipped are in
§ Record of outcomes below; the defect record with both causes named is in
[FOLLOW_UP_PLAN.md](FOLLOW_UP_PLAN.md) § 1.4.*

**What.** The Upcoming-match default pool selects retired and long-inactive players. Two
causes: the pool is all-time ("has ever appeared for the club in the format") with an
`is_retired` filter nothing populates, and ratings decay per match played, not per
elapsed time, so a player inactive since 2015 keeps the strong rating they stopped with.
The fix is at the product boundary (availability is the caller's knowledge), in three
reinforcing parts: a recency-bounded default, an optional manual pool picker, and a
**retirement ledger** — a user can flag a player retired, and the flag is promoted to a
stored fact only when corroborated by an independent criterion. Runs fine before X-1a;
when X-1a lands, its age and career-end facts join the corroboration criteria with no
schema change.

```
Read docs/EXTERNAL_DATA_PLAN.md (the D-12 row) and docs/FOLLOW_UP_PLAN.md §1 (the
defect-record style — record D-12 there with both causes named). Read
go-app/internal/db/repo_selection.go (ListPlayerPoolByOpposition and its dead
is_retired filter). Do NOT change the rating pass — time-decaying ratings is model
work needing its own gated experiment. Branch off main as fix/d-12-pool-and-retirement.
Rules: never commit to main; anchored edits; conventional commits with scope; H-24 for
any new wire literal; §8.7 — every substitution or filter visible on the wire.

Do the fix:
1. RECENCY DEFAULT. The default pool becomes recency-bounded: players who appeared for
   the club in that format within a window ending at the request's cutoff. Choose the
   window from evidence: over the last year of real matches per format, the window
   covering >= 95% of players who actually took the field while cutting the all-time
   pool hardest (per-format default in config, overridable per request). Record the
   measurement table in the D-12 entry.
2. MANUAL POOL PICKING (optional, off by default). When the user selects a team, they
   may open the candidate list — the recency pool by default, widenable to all-time —
   with each player's last-played date and, once X-1a lands, age; they tick a subset
   and that subset is the pool sent. No selection means the default pool, unchanged
   flow. must_include / extra ids still bypass every filter.
3. THE RETIREMENT LEDGER. A player_status store (forward migration): a user can flag a
   player retired from the candidate list. The flag alone excludes the player from
   that user's future default pools and is recorded as user_flagged. It is promoted to
   a stored is_retired fact only when corroborated by at least one independent
   criterion, evaluated at promotion time: (a) no match in any format for >= N years
   (config; default from step 1's evidence), (b) once X-1a lands, a Wikidata career
   end date before the cutoff, or (c) age above a per-format bound with >= M years
   inactive. The criteria are a pluggable list so X-1a extends them without schema
   change. Every promotion records which criterion corroborated it and when; an
   un-flag demotes the fact and records that too. The dead is_retired filter in the
   pool query now reads the ledger and is real.
4. HONEST SURFACE. The response states the window used, the pool size, and how many
   players the ledger excluded; the UI shows "pool: played for <team> in the last <N>
   months (<M> players, <K> excluded as retired)" with all-time one click away; a
   ledger-excluded player is visible (struck through with the reason), never silently
   gone — a user must be able to see and undo an exclusion.
5. BLAST RADIUS. The L4 harness and E5 build sides from fielded XIs, not this pool —
   verify nothing in the harness path reads ListPlayerPoolByOpposition or the ledger;
   diff the evaluation report before/after on the same run to prove the measured
   record is untouched. Backtest scripts that use pools get the window relative to
   their as-of date; the ledger applies only to future-dated (upcoming) requests, so
   a backtest at a 2019 cutoff still sees 2019 players.
6. VERIFY the D-6 way: rebuild containers, predict an upcoming match for two clubs
   with famously retired ex-players, confirm by looking that the pool is current-era;
   flag a player, corroborate, see the exclusion and its reason; un-flag and see the
   return. Unit tests pin the window arithmetic at the cutoff boundary and the
   promotion/demotion rules. While in the docs: add one line to
   docs/PRODUCT_ROADMAP.md Phase 1 Team Lab spec — a toss toggle (bat first / bowl
   first / unknown) on the Lab and Upcoming-match surfaces, wired to the
   team1_bats_first parameter the API already supports end to end.

Acceptance: the default upcoming-match pool contains no player inactive beyond the
window; manual picking works and is optional; a flag alone never becomes a global
fact — promotion requires corroboration and records its reason; exclusions are
visible and reversible; harness numbers untouched (report diff attached to the PR);
make check-all green; coverage gates never move down; docs/FOLLOW_UP_PLAN.md §1 and
this plan's record updated. Then stop and hand over the push and PR commands.
```

---

## Record of outcomes

| id | status |
|---|---|
| X-4 | **measured, on free sources only** — the one licence-clean free series covers BBL/WBBL; 4.2 % of T20 joined, 0 % elsewhere; market ahead by 0.052 AUC with a 95 % interval spanning zero. Paid and account-gated sources rejected. It does not price the rest |
| X-1a | **acquired and measured** — DOB clears the gate (85.3 % of appearances; ≥ 80 % in every limited-overs format and gender **except women's T20 at 61.4 %**); style, handedness and career end are **recorded nulls** — Wikidata carries them for 265, 18 and 3 of 13,662 players. Coverage report and backfill shipped; the D-12 criteria are wired |
| X-1b | open — X-1a's coverage supports the **age** family only; the matchup family has no labels to build from and its gate cannot be run |
| X-3 | open |
| X-2 | open — run last |
| D-12 | **fixed** — recency-bounded default pool, manual picking, the retirement ledger; measurement below |

### X-4 — the source review, the join, and what the numbers support

**1. The source decision, made before any code.** The question was whether a historical
closing-odds series for cricket exists whose licence permits this use, at an acceptable
cost — and **"acceptable cost" here is zero**: only sources that are free to obtain and free
to use are eligible. Costs are recorded below for the paid candidates as evidence of what
was looked at, not as options held open. Everything evaluated, and why it was taken or left:

| candidate | cricket coverage | cost | licence | verdict |
|---|---|---|---|---|
| **Betfair Exchange season summaries** (BBL / WBBL CSVs published by Betfair's data-science team) | BBL 2020-21 → 2025-26, WBBL 2020 → 2025; one row per runner per Match Odds market, best back and lay at the first ball and at nine in-play points | **free**, no account, direct download | published openly with a warranty disclaimer and **no** open licence — usable, not redistributable | **taken.** The only cricket odds series obtainable with no payment and no gambling account |
| Betfair Historical Data service (`historicdata.betfair.com`) | every Exchange market since 2016, cricket included — internationals and franchise leagues | Basic tier free (last traded price per minute, no volume); Advanced and Pro priced on application, not published | Betfair's own terms; copyright and database right asserted, no redistribution | **rejected on availability.** The free tier is gated behind a Betfair account — a gambling account, in a jurisdiction Betfair serves — which is not a source that is simply free to obtain. The paid tiers are out of scope under the zero-cost rule |
| The Odds API (`the-odds-api.com`) historical endpoint | from 2020-06-06 at 10-minute (later 5-minute) snapshots; Tests, ODIs, IPL, T20 World Cup and more | paid only: **$30/mo** (20k credits), **$59/mo** (100k), $119/mo (5M); a snapshot costs 10 credits per region per market, so ~3,000 matches ≈ 30,000 credits ≈ the $59 tier for one month | commercial terms; no redistribution | **rejected: paid.** It has the international coverage the free source lacks, and the price is recorded as evidence of what that coverage costs — not as an option to take |
| OddsPortal / oddsbase and similar odds archives | broad, back many years | free to browse | terms of use forbid extraction and reuse | rejected on licence — and not scraped |
| OddsMatrix historical odds feed | broad, 30+ bookmakers | enterprise, price on application only | commercial | **rejected: paid** (and no published price to record) |
| aussportsbetting.com | Big Bash only, one xlsx | free | "personal use only… should not be made available elsewhere" | rejected: duplicates the Betfair BBL coverage under a narrower permission, and the site blocks automated download |
| Sports Insights historical database | US sports; no cricket | — | — | rejected: no cricket |
| Open research repositories (Kaggle, Zenodo, figshare, Mendeley) | searched for a cricket odds series under CC0/CC-BY | free | open | **nothing found.** The open cricket datasets are ball-by-ball and scorecard data; no odds series |

So a free, licence-clean source exists, and it is small. The finding worth recording is not
"no source" but **"a free source that covers two competitions and no international cricket
at all"**. Every candidate with international coverage was either paid or behind a gambling
account, and all of them are rejected: the eligible universe of free cricket closing odds is
the Big Bash and the Women's Big Bash, and that is the ceiling on what this benchmark can
ever say without a change to that rule.

**2. The join.** 592 usable two-runner markets were loaded from twelve cached season files
(1,188 rows; 2 markets dropped for an unusable price). Each was joined on the exact match
date plus both sides resolved through the identity layer — `opposition_name` + gender folded
by `COALESCE(canonical_id, id)`, the same key the rating frame is built on (D-10), reached
through the new `MatchSource.team_key_for`. A runner name is tried as written and then, only
if the archive has never fielded a club of that name, without a trailing competition token
(`Perth Scorchers W` → `Perth Scorchers`).

| outcome | markets |
|---|---|
| joined to exactly one match | **588 (99.3 %)** |
| no match on that date | 4 |
| team the identity layer does not know | 0 |
| more than one candidate match | 0 |
| **joined quotes whose winner disagrees with ours** | **0** |

Zero winner disagreements across 588 joins is the integrity check on the whole mapping: an
odds row silently attached to the wrong fixture would show up here. The four misses are
date offsets of one day (three) and one fixture the archive does not hold; they stay
unmatched and counted, because a ±1-day join is a guess.

**3. Coverage, which is half the answer.** Of the 588 joined markets, **403 fall before the
harness's first walk-forward cutoff (2024-01-01)** and so sit in no scored window — the
report carries that count as `quotes_joined_outside_scored_windows`, so the join total and
the per-format counts cannot read as a contradiction. That leaves **185 scored matches, all
T20**:

| format | matches the harness scored | with a closing price | joined coverage |
|---|---|---|---|
| T20 | 4,387 | 185 | **4.2 %** |
| T20I | 433 | 0 | 0 % |
| ODI | 1,105 | 0 | 0 % |
| TEST | 443 | 0 | 0 % |

The locked window (from 2026-09-02) contains **no** priced match at all — the next Big Bash
season starts in December — so the market benchmark has nothing to say there, and says so.

The earlier seasons were deliberately *not* scored by extending the fold grid backwards.
Doing so would compare the 2026 market against a display model fitted with four fewer years
of history: the market's quality does not degrade with our training set, so the comparison
would be biased against us and would price a model nobody ships.

**4. The measurement.** De-vig is proportional on the best back price at the first ball
(each side's `1 / price`, divided by the pair's sum); the mean overround was **1.006**,
which on an exchange is the back/lay spread rather than a bookmaker's margin. Three arms,
same matches, same fold models — the H-23 triple is registered as gate `X-4`, and it
**informs**: nothing in the system changes on these numbers.

| fold (T20) | n | market AUC | display AUC | display AUC, toss-aware | market Brier | display Brier |
|---|---|---|---|---|---|---|
| 2024-01-01 → 2024-04-01 | 22 | 0.500 | 0.525 | 0.525 | 0.2530 | 0.2650 |
| 2024-10-01 → 2025-01-01 | 57 | 0.529 | 0.485 | 0.432 | 0.2507 | 0.2644 |
| 2025-01-01 → 2025-04-01 | 24 | 0.578 | 0.563 | 0.578 | 0.2470 | 0.2455 |
| 2025-09-01 → 2025-12-01 | 28 | 0.556 | 0.610 | 0.631 | 0.2404 | 0.2379 |
| 2025-12-01 → 2026-03-01 | 54 | 0.747 | 0.666 | 0.651 | 0.2157 | 0.2329 |
| **pooled** | **185** | **0.608** | **0.556** | **0.536** | **0.2387** | **0.2488** |

*(Folds not listed held fewer than twenty priced matches and are reported with their count
and no metrics.)*

- **Market − display, AUC: +0.052, 95 % interval −0.020 to +0.128** (paired bootstrap over
  matches, 2,000 replicates, seed 0). The interval spans zero.
- Against the toss-aware arm — the like-for-like one, since the closing price is struck
  after the toss while the served probability marginalises over it — **+0.072, 95 % interval
  −0.003 to +0.152**. Also spans zero, barely.
- **Market − display, Brier: −0.010.** The market's probabilities are better stated on these
  matches; on 185 matches that is not resolvable either.

**5. The honest sentence the numbers support.** *"On the only cricket matches for which we
could obtain closing odds — 185 Big Bash and Women's Big Bash fixtures, 4.2 % of our T20
matches and none of any other format — the market ranked winners at 0.608 AUC and our
displayed model at 0.556. The market is ahead on both AUC and Brier, but on this many
matches the gap's 95 % interval spans zero, so we cannot yet claim either parity or a
deficit. We have not benchmarked ourselves on internationals at all."*

Two further readings, both stated because they are easy to misread otherwise:

- **These are hard matches.** The display model's headline T20 AUC in the same run is
  **0.730 ± 0.050** over the whole format; on this subset it is 0.556 and the *market* only
  reaches 0.608. Big Bash fixtures are deliberately balanced, so the population priced here is
  close to a coin flip for everyone. The 0.730 and the 0.556 are not in conflict — they are
  different populations, and that is exactly why the coverage figure travels with every number.
- **The toss-aware arm scored worse than the served one** (0.536 vs 0.556) despite knowing
  more. Marginalising over the two batting orders averages two correlated readings and
  reduces variance; on 185 matches that is worth more than the toss. It is reported, not
  explained away.

**Blast radius: additive by construction.** The benchmark fits nothing. It is handed each
fold's already-fitted display models and reads them; it mutates no frame, and its bootstrap
draws from its own `RandomState` rather than the global stream, so it cannot perturb a
simulator seed. The only edit to existing logic is `_evaluate_fold` returning a `FoldOutcome`
instead of a three-tuple. The run above reports H-8 parity **0.0** across 50 matches, 1,100
player rows, 1,100 performance predictions and 50 simulations, and its walk-forward display
AUCs — T20 0.730 ± 0.050, T20I 0.753 ± 0.037, ODI 0.707 ± 0.069, TEST 0.646 ± 0.101 —
reproduce the last stored pre-change report on this machine to three decimals. That stored
report is a close control rather than an exact one: it predates three matches imported since,
so a field-by-field zero diff was not available and is not claimed.

**The run.** `make evaluate` against Postgres, 67 minutes, 21,096 match rows: gates pass
(H-23, X-4 registered), the glossary explains every metric the report prints (L-1), H-8
parity 0.0, the locked window still holds 0 matches.

**What shipped.** `ml-service/ml/xi/market.py` (load, de-vig, join, score), a
`market_benchmark` section in `xi_evaluate_report.json`, gate `X-4` in the H-23 registry,
eleven glossary entries (L-1, copied into [FOLLOW_UP_PLAN.md](FOLLOW_UP_PLAN.md) § 3's
table), and an *Evaluation report → Market benchmark* panel that prints the coverage beside
the numbers and says plainly when a format has none. The odds files are cached under
`data/market-odds/` (git-ignored; `ML_MARKET_ODDS_DIR` or `make evaluate MARKET_ODDS_DIR=…`
overrides it) and are **not** committed, because the source grants no redistribution right.
`tests/test_xi_market.py` asserts by import graph that nothing which builds a feature, fits a
model or serves a prediction can reach the odds.

**What would resolve the question, and why it stays open.** Coverage, not code: the machinery
scores whatever is in the cache, and a second reader in the loader is the only piece a new
file format would need. But no free, licence-clean source of international cricket closing
odds was found, and paid ones are out of scope, so the question stays open rather than
becoming a purchase decision. If a free source with international coverage appears — an
academic release, a permissively licensed archive — dropping its files in `data/market-odds/`
is the whole integration.

### X-1a — what Wikidata actually carries, and what it does not

*Done on `feat/x-1a-player-biographies`. Acquisition and measurement only: nothing under
`ml-service/` reads the table, no feature was built, and no model number moved.*

**The join, and why it is the only free one.** Cricsheet's people register
(`https://cricsheet.org/register/people.csv`, 18,507 people) carries `key_cricinfo` beside
the identifier `player.external_id` already holds, and Wikidata carries the same
ESPNcricinfo id as property `P2697`. Both files are free, need no account and are
licence-clean (Wikidata's data is CC0-1.0), which is the standing no-paid-sources rule.
The join reaches **13,638 of 13,662 players — 99.98 % of appearances**; the 24 misses are
people ESPNcricinfo does not list at all. 31,691 Wikidata items carry a `P2697` value, so
the source is not thin in principle.

**What the pass found, per player.** 13,662 rows written — one per player, *including* the
6,484 nothing was found for, because "asked and had nothing" is a measurement and "never
asked" is not:

| fact | Wikidata property | players | share of players |
|---|---|---:|---:|
| matched to an item | `P2697` | 7,178 | 52.5 % |
| date of birth | `P569` | 6,967 | 51.0 % |
| bowling style, placed in the vocabulary | `P2545` | 265 | 1.9 % |
| batting handedness | `P741` / `P552` | 18 | 0.13 % |
| career end date | `P2032` | 3 | 0.02 % |
| date of death | `P570` | 25 | 0.18 % |

**Weighted by appearances**, which is the share a feature would actually see — a biography
for a man who played once in 2004 is not worth one for a man in every eleven this season.
503,371 fielded player-sides:

| format | gender | appearances | matched | **DOB** | style | hand | career end |
|---|---|---:|---:|---:|---:|---:|---:|
| TEST | female | 528 | 100.0 % | 99.6 % | 3.0 % | 0.0 % | 0.0 % |
| TEST | male | 68,220 | 97.7 % | 97.2 % | 6.0 % | 0.7 % | 0.1 % |
| ODI | female | 20,865 | 97.3 % | **94.0 %** | 1.3 % | 0.3 % | 0.0 % |
| ODI | male | 94,058 | 94.0 % | **93.1 %** | 6.7 % | 0.8 % | 0.2 % |
| T20 | female | 63,175 | 63.4 % | **61.4 %** | 0.3 % | 0.3 % | 0.0 % |
| T20 | male | 209,675 | 83.3 % | **82.6 %** | 4.9 % | 0.2 % | 0.0 % |
| T20I | female | 16,881 | 95.5 % | **93.7 %** | 1.9 % | 0.1 % | 0.0 % |
| T20I | male | 29,969 | 92.8 % | **92.5 %** | 7.2 % | 0.4 % | 0.0 % |
| **all** | | **503,371** | **86.3 %** | **85.3 %** | 4.7 % | 0.4 % | 0.1 % |

The full report, with the unweighted player counts and the ranked gap list, is committed at
[player-biography-coverage.md](player-biography-coverage.md).

**The gate: date of birth passes, with one named exception.** The bar was "matched
biography with DOB for players covering ≥ 80 % of match appearances in each limited-overs
format". ODI **93.1 % / 94.0 %**, T20I **92.5 % / 93.7 %** and men's T20 **82.6 %** clear
it. **Women's T20 fails at 61.4 %.** That is not noise and it is not a bug in the join: the
T20 (All) format is mostly domestic and associate-nation cricket, and Wikidata's cricket
coverage falls away exactly where the players are not internationals. X-1b's age family may
therefore be run for ODI, T20I and men's T20; women's T20 is out of scope for it on
coverage grounds, and saying so here is cheaper than discovering it as a fold that will not
fit.

**Three recorded nulls, and none of them is fixable with more effort.**

1. **Bowling style is not in Wikidata at any usable scale.** `P2545` is stated for 1,145
   items *world-wide* and for 265 of the players in this registry (4.7 % of appearances),
   and its distribution is a bot import rather than a labelling: 179 left-arm-orthodox, 73
   leg-spin, and single figures for everything else. A vocabulary was still built and
   mapped (`pace` / `medium` / `off-spin` / `leg-spin` / `left-arm-orthodox` /
   `left-arm-wrist`, plus `unknown` for a label too coarse to place, such as "spin bowling"
   or "right arm") because X-1b needs a stable label set the day a source appears — but on
   today's coverage **X-1b's matchup family cannot be run at all**, which is the same
   conclusion A-3 reached from inferred labels and now has a labelled reason.
2. **Batting handedness is effectively absent.** 18 players. `P741` (playing hand) is on 23
   cricketer items world-wide and `P552` (handedness) on 29. The left–right top-order
   balance feature X-1b wanted has no labels.
3. **A career end date is effectively absent.** `P2032` is stated for 21 cricketer items
   world-wide and 3 in this registry. Wikidata records that a career happened, not when it
   stopped.

**Why nothing was inferred.** A date of death (`P570`, 25 players here) is stored as its own
column and is deliberately *never* written into `career_end_date`: a player who died in 2022
may have stopped playing in 2007, and a retirement ledger reading one as the other would
promote a claim on evidence that does not exist. A `P569` value is also stored as given —
Wikidata renders a year-precision date as the first of January and the `wdt:` shortcut does
not carry the precision qualifier, so a birth date here can be a year wearing a day. That
costs an age feature at most half a year and is recorded rather than smoothed away.

**Overrides do not close the gap, and were measured before being skipped.** The report
ranks unmatched players by appearances so a curator works down a list. Of the 23,126
unmatched women's T20 appearances, the **top 25 overrides recover 1,909** — 61.4 % → 64.4 %,
about three points. Reaching 80 % would need roughly 350 hand-checked rows. The top of the
men's T20 list is worth 1.0 point for 25 rows. "A handful of overrides moves coverage
materially" is false here, so `configs/player_biography_overrides.json` ships **empty**,
with its format, validation and the pattern in place for the day a specific player matters.
Every unmatched player in the top 25 is an associate-nation or domestic women's player
(Papua New Guinea, Thailand, Indonesia, Vanuatu, Uganda, Rwanda) — the shape of the gap,
not an accident of it.

**What shipped.** Migration `0010_player_biography.sql`, one row per player keyed by the
registry id, with the join key and the licence recorded per row; `internal/biography` (the
register parser, the SPARQL builder and decoder, the vocabulary, the resumable cache, the
override validator and the coverage arithmetic — all pure, all tested);
`cmd/player-biography-backfill` behind `make player-biographies`; and
`GET /ops/data/biography-coverage`, rendered on the Ops tab beside the dataset registry so
the state of the source is on a surface rather than in a log (§8.7). The importer did not
change and no model or feature did.

**Resumability, demonstrated rather than asserted.** Every batch's answers, misses
included, are appended to `output/player-biographies/wikidata-lookups.jsonl` before the
next batch starts. The first run was killed mid-way by a real 502 from the query service at
batch 9 of 28; re-running the same command resumed at 4,500 of 13,707 ids and finished. A
bounded retry (three attempts, doubling wait, only on 429 and 5xx) was added afterwards so
the common case does not need an operator. The database was then truncated and rebuilt from
the archive for the integration tests, and the backfill re-run: it asked Wikidata **nothing**
and produced **the same figures to the last digit** (86.33 % matched, 85.33 % DOB, 4.68 %
style).

**One consequence outside acquisition.** D-12 registered two corroboration criteria that
reported themselves *unavailable* because nothing supplied a date of birth or a career end
date. Both now read `player_biography` and answer per player. In practice: criterion (c)
(age with inactivity) is live for the 85 % of appearances with a date of birth, and
criterion (b) (career end) is live in code but will almost never have evidence — three
players. That is the honest state, and it is why the criteria report "unavailable" rather
than "no".

**H-24.** No new literal crosses a service boundary. The bowling-style vocabulary lives in
Go and in a Go-validated config file; the coverage endpoint returns counts, and the frontend
renders them without matching on any of the values. The day X-1b sends a style label to
ml-service, that vocabulary becomes an H-24 declaration — it is not one yet, and declaring a
literal only one side matches on would be the JSON-file-next-to-an-untested-seam the rule
warns against.

### D-12 — the measurement, and what shipped

**The window, measured rather than chosen.** Over the last year of real matches per format
(the fielded XIs in `match_player`, 56,039 player-sides), for every window *W* the question
was: of the players who actually took the field *and whom the all-time pool would have
offered at all*, what share had appeared for that club in that format within *W* months of
the match? The rule from the plan is the smallest window clearing 95%.

| window (months) | TEST | ODI | T20 | T20I |
|---|---|---|---|---|
| 6 | 86.24% | 87.22% | 90.00% | 93.53% |
| 9 | 93.43% | 90.88% | 91.97% | **95.13%** |
| 12 | **97.51%** | **95.33%** | **96.68%** | 97.45% |
| 15 | 98.36% | 97.22% | 97.73% | 98.21% |
| 18 | 98.66% | 98.04% | 98.17% | 98.81% |
| 24 | 99.30% | 98.69% | 99.03% | 99.21% |
| 36 | 99.70% | 99.47% | 99.60% | 99.64% |
| 60 | 99.91% | 99.75% | 99.88% | 99.88% |

**What each window cuts.** Mean pool size at the same club/date pairs, as a share of the
all-time pool it replaces:

| window (months) | TEST | ODI | T20 | T20I |
|---|---|---|---|---|
| 6 | 28.6% | 33.0% | 39.0% | 23.2% |
| 9 | 32.5% | 37.5% | 41.7% | **25.0%** |
| 12 | **44.7%** | **43.3%** | **52.3%** | 27.9% |
| 18 | 49.2% | 49.5% | 57.9% | 32.6% |
| 24 | 52.7% | 53.3% | 64.4% | 37.2% |
| 60 | 67.0% | 68.5% | 84.6% | 52.4% |

Mean all-time pool per club/date: TEST 66.2, ODI 58.3, T20 45.8, T20I 83.9 players.

**Defaults, in `go-app/config.json` under `pool.recency_months`:** TEST 12, ODI 12, T20 12,
**T20I 9**. T20I is the one format nine months already covers, and it clears the bar by
0.13 points — recorded here because that is a thin margin, and the window is overridable
per request precisely so a thin default is not a trap. Every format keeps a bound: an
unknown format falls back to twelve months, never to all-time, because unbounded is the
defect.

**The inactivity bound, also measured.** Criterion (a) promotes a flag when a player has
appeared in *no* format for N years. Of 54,835 fielded player-matches in the last year of
data with any prior history, the share that were a return after an absence of at least:

| absence | 1y | 2y | 3y | 4y | 5y | 6y |
|---|---|---|---|---|---|---|
| share of fielded player-matches | 1.807% | 0.554% | 0.228% | 0.117% | 0.060% | 0.047% |
| distinct players | 884 | 266 | 116 | 60 | 30 | 25 |

**N = 5 years** (`pool.retirement.inactive_years`): 30 players in a year, 0.060%, is where
promoting a user's claim to a fact about the player stops being a bet. Criterion (c)'s
inactivity half is 2 years, which corroborates only in combination with the age bound.

**What shipped.** The default pool is `[cutoff − window, cutoff)` with clamped month
arithmetic (31 March less one month is 28 February, not 3 March). Manual picking is a
candidate list at `GET /api/options/candidates`, optional and off by default; ticking
nothing is the unchanged flow, and `must_include` / extra ids bypass every filter as
before. The ledger is migration `0009_player_status.sql`: `player_status` holds one
claim per (player, user) and `player_status_event` the history, and a claim raises
`player.is_retired` only when one of three pluggable criteria corroborates it
(`internal/availability`) — inactivity today, career-end and age-with-inactivity
registered and reporting themselves *unavailable* until X-1a supplies dates of birth and
career end dates. The prediction response carries `team1_pool` / `team2_pool` (source,
window, size, and every excluded player with his reason), and the tab renders it with the
all-time pool one click away and an Undo beside each exclusion.

**Blast radius: none measured.** The L4 harness and the E5 gate build sides from fielded
XIs (`match_player`), not from this pool; nothing under `ml-service/` reads
`ListPlayerPoolByOpposition`, `player.is_retired`, or the ledger tables, and the change is
confined to Go, TypeScript and SQL.

`make evaluate` was run on the same database before the change and again after it, and
the two `xi_evaluate_report.json` files compared field by field. **125 fields differ, and
every one of them is a wall clock**: `performance.fit.fit_seconds` (18),
`simulation.latency.ms_per_fixture_at_default_samples` (18) and
`…_at_harness_samples` (18) per fold, plus the three means and three standard deviations
those roll up into, and `generated_at`. Every measured number is identical to the last
digit — the walk-forward and locked AUC, Brier, coverage, width, pinball and Spearman for
all four formats, `e5_lineup_only` and `selection_decision` per format, and the whole of
`gates`, `serving_parity`, `leak_canary`, `data_quality`, `e5_previous_elevens`, `seeds`,
`cutoffs`, `n_rows`, `n_player_rows` and `locked_window`. The measured record is untouched.
`scripts/experiments/xi/selection_gate_rerun.py` gets the window relative to each
fixture's own date and applies no ledger (a claim made in 2026 is not evidence about a
2019 pool, H-19); `--all-time-pool` restores the pre-D-12 pool for a like-for-like
comparison with the recorded run.
