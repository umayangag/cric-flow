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

**Standing constraint (2026-09-04).** This system is a **prototype built on publicly
available, free data**, and the rule X-4 was run under — "acceptable cost" is zero — is now
the rule for the whole plan rather than that item's choice: **no paid, purchasable,
subscription or account-gated sources, and no human-subject research** (interviews,
surveys, user studies). Free, publicly available, licence-clean sources only, each licence
verified at the source and recorded in [config-and-data.md](config-and-data.md) § Data-source
licence register. [PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md) states the same constraint at
its head and records what it removes there (P0-2 deferred, P0-3 skipped, P0-4 forced).
Here it changes two things. **X-4 was already run under it** and needs no re-run, but the
pricing route named above — "coverage bought rather than code written" — is closed unless
the user lifts the constraint, so the market question stays open on coverage grounds.
And **X-2 must verify its source's terms before it acquires anything** (§ X-2): the
expectation is free, but it is an expectation, and a paid finding is a completed gate.

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

## X-1b — biography features, gated (model: Fable; requires X-1a merged) — **run: three recorded nulls**

**What.** The two mechanisms the archive cannot express: **age** (trajectory — ratings
only see the past; a 21-year-old and a 36-year-old with identical form differ
predictably) and **handedness/style matchups** (left–right combinations, spin-type vs
handedness — the effects with real empirical support, which A-3 could not test for want
of labels). Three families, each with its own gate; nulls per family acceptable.

*Done on `feat/x-1b-biography-features`; the fold tables and the readings are in § Record
of outcomes below and in [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md)
§8.12. The short version: **the matchup family could not run** — 18 batting hands and 265
bowling styles of 13,662 players is not a label set, and this is A-3's null confirmed with
the labels; **age in the performance model moves the pinball loss by 0.02 %** on the
deciding slices, inside the noise band, with women's T20 scoped out at 61.4 % and reported;
and **an age-band debut prior keeps H-10 bounded and makes every format's debut forecasts
worse**, because the model already learns its own debutant neutral. Nothing shipped; the
date of birth reaches every player row and both switches stay off.*

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

## X-3 — match stakes, derived from data already held (model: Opus) — **run: two recorded nulls, and an importer field the archive had been losing**

*Done on `feat/x-3-match-stakes`; the coverage figures, both gates' fold tables and the H-21
audit are in § Record of outcomes below. The short version: the derivation is real and
cheap — a stage label for 96.5 % of matches, a dead-rubber flag for the 72.9 % where a table
is reconstructible as-of — and **neither use clears its gate**. Cleaning E5's pairs does not
rescue T20 (four of five arms move the agreement down; the fifth clears its bar by one
twentieth of a standard error), and a knockout flag adds nothing to the display model
outside T20I. The one thing that was not a null is the plumbing: Cricsheet states
`event.stage` and `event.group` and the importer was discarding both.*

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

**Licence check first, under the standing constraint.** Open-Meteo's current terms must
be verified against the source and recorded in the licence register by this item's
worker **before any acquisition**. The expectation is *free for non-commercial use with
attribution*: read on 2026-09-04, its terms page said *"You may only use the free API
services for non-commercial purposes"*, its licence page *"API data are offered under
Attribution 4.0 International (CC BY 4.0)"*, and the ERA5 dataset page at the Copernicus
Climate Data Store names a CC-BY licence. But its pricing page also said *"Historical,
climate, ensemble, and satellite radiation APIs require the Professional API Plan or
higher"*, and whether that sentence governs the free non-commercial endpoint
(`archive-api.open-meteo.com`) or only the paid customer endpoints could not be settled
from the pages alone. That is the question to answer from the source, not from memory.
If the historical archive turns out to require payment for this use, X-2 **records that
as its finding in § Record and stops** — a completed gate, the X-4 way — rather than
buying access or substituting an account-gated source.

```
Read docs/EXTERNAL_DATA_PLAN.md (X-2, including the licence check that precedes any
acquisition) and docs/FOLLOW_UP_PLAN.md (the A-1 and A-2
nulls — venue-level context and the chase response both failed, which bounds what
weather can add). Branch off main as feat/x-2-weather-context. Rules: folds decide
over three seeds; H-19; H-21 (every feature computable BEFORE the match — pre-match
window readings or daily values, never in-match observations); H-22; H-23 per family;
H-24 for any new wire literal; never commit to main.

Do X-2:
0. LICENCE FIRST: verify Open-Meteo's current terms for the historical archive at the
   source (terms, licence and pricing pages) and record them in
   docs/config-and-data.md § Data-source licence register. If this use is free and
   non-commercial with attribution, proceed. If it requires payment or an account,
   record that in the X-2 row and § Record as the finding and STOP — no purchase, no
   substitute source.
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
| X-1b | **done — three recorded nulls, nothing shipped** — the **matchup** family is *not runnable for want of labels* (Wikidata: a batting hand for 18 and a bowling style for 265 of 13,662 players; the labelled confirmation of A-3's null; E5 not re-run, X-3 owns it); the **age** family moves the performance model's runs and wickets pinball by ~0.02 % on the deciding slices (T20 men +0.02 / +0.02 %, ODI +0.02 / −0.01 %), inside E1's band, women's T20 out of scope at 61.4 % and reported; the **age-aware cold start** keeps H-10 bounded and moves no player with history, and worsens the debut rows' pinball in every format (T20 −0.30 / −0.53 %, ODI −0.69 / −1.11 %). The date of birth now reaches every player row (`AGE_COLS`, missing = its own indicator) and both models read it only through flags that stay off; `make evaluate` after the choice reproduces A-4's baseline with H-8 parity 0.0 on both sources |
| X-3 | **done — two recorded nulls, and one importer defect fixed** — the archive already held the stakes: Cricsheet's `info.event` carries `stage` and `group` and **the importer was dropping both** (migration `0011`, re-imported: 1,584 stages, 6,448 groups, the archive's own counts). `ml/xi/stakes.py` derives a stage label for **96.5 % of matches** (T20 97.7 %, T20I 96.9 %, ODI 95.4 %, TEST 93.0 %; 53 of the archive's 55 stage spellings recognised) and a dead-rubber flag only where a table is reconstructible as-of — **72.9 % of matches**, 2,196 dead rubbers, bilateral scorelines and leagues whose playoff cut is observable, nothing guessed. **Use 1 (E5 hygiene) is a null:** of five pair filters in T20 only "exclude dead rubbers" crosses its re-derived bar, by **0.0006 against a standard error of 0.0117**, while excluding knockouts (0.4976), excluding both (0.4972) and down-weighting (0.5005) all move *below* the 0.5030 control — rotation noise was not masking selection signal, and the T20 scoping does not move. The control reproduces P-7's figures exactly on the pre-A-4 window (T20 0.4897/1,358, ODI 0.5618/429, T20I 0.5856/111). **Use 2 (a knockout flag in the display model) is a null:** ΔAUC +0.0000 in T20, ODI and TEST, +0.0021 ± 0.0010 in T20I alone, so `STAKES_FEATURES_KEPT` stays `False`; the gate's own swap clause was mis-specified against H-4's 2 % objective line, which the *control* display surface already fails at 3–7 %, and that is recorded. `make xi-parity`: the four new counts agree on both sources |
| X-2 | open — run last; the licence check in § X-2 precedes any acquisition, and a paid finding closes the item |
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
Losing that cache costs nothing but a download: the twelve season CSVs are re-fetched from
`betfair-datascientists.github.io/data/dataListing/` into the same directory — see
[config-and-data.md](config-and-data.md) § Recovering the external data.
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

That cache was written under `output/`, which is git-ignored and which `make dev-purge`
deletes, so it was one machine away from being gone. The durable copy is now committed as
`reference-data/wikidata-player-lookups.jsonl` with the people register it joins through,
and `make restore-player-biographies` rebuilds the table from them offline — see
[config-and-data.md](config-and-data.md) § Recovering the external data.

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

### X-1b — the two families that could run, and the one that could not

*Done on `feat/x-1b-biography-features`. The design, the H-21 audit, the H-23 triples and
the full fold tables are in [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md)
§8.12; this is the record. Every switch the item added is off: nothing ships, and the
plumbing that lets the question be re-asked without a new pass stays.*

**What was built, so the gates could run.** The rating pass now reads one column of
`player_biography` — the date of birth — through `ml/xi/biography.py`: Postgres reads the
table, the archive path reads the CSV `make export-birth-dates BIRTH_DATES=<path>` writes
from it (`BIRTH_DATES=` on `make retrain` / `evaluate` / `xi-parity`), and the pass reports
`players_with_birth_date` beside its other counts (6,955 of 13,605 rated players on this
archive; `make xi-parity` compares it between the sources). Every player row carries
`age` (years at the match date) and `age_known` (`contract.AGE_COLS`); a player without a
date of birth reads **0.0 / 0.0 — a category of his own, never an imputed age**. The rating
state holds the dates (persisted with the artifact) and, for family 3, an age-band debut
pool. H-8's parity check compares the two new columns on every player row it rebuilds.

**Family 2 — MATCHUP features — not runnable, for want of labels.** The left–right
top-order balance and the spin-type coverage against the opposition's handedness profile
need a batting hand and a bowling style per player. X-1a measured Wikidata's supply at
**18 batting hands and 265 bowling styles of 13,662 players** (0.4 % and 4.7 % of
appearances; `P741`/`P552` on 23 and 29 cricketer items world-wide, `P2545` on 1,145, the
265 a bot import — 179 left-arm-orthodox, 73 leg-spin — rather than a labelling). There is
no side-level aggregate to build from that, and fitting one to 1.9 % of the registry would
measure which players a bot tagged. **This is the labelled confirmation of A-3's null**
(plan §8.11): A-3 tried the matchup axis from what the archive carries — the innings phase —
because "Cricsheet carries no bowling style" and recorded a null; asked again with the
labels, the answer is that the labels do not exist either. Nothing was fitted, no inferred
label was substituted (A-3 already tested that route), and the E5 re-run that travelled with
the family was not run — it goes with the family, and X-3 owns E5's hygiene question.

**Family 1 — AGE in the performance model — a recorded null.** Gate `X-1b-age` (H-23
triple registered first; two arms per fold, `none` and `age`, eleven folds 2024-01 …
2026-06, three seeds, every target fitted, everything else fixed; decided by the pinball
loss of runs or wickets improving by more than E1's 0.5 % band **and** one fold-level
standard error, in both T20 and ODI, with coverage held) **kept nothing**. The deciding
rows, control → age with the paired difference (positive = age better) and its size
relative to the control:

| format | deciding slice | folds | rows | age known | runs pinball | wickets pinball | runs 10–90 coverage / width | verdict |
|---|---|---:|---:|---:|---|---|---|---|
| T20 | men (women's T20 out of scope at 61.4 %) | 11 | 67,086 | 0.653 | 3.1632 → 3.1624 (+0.0008 ± 0.0006, **+0.02 %**) | 0.1415 → 0.1415 (+0.0000 ± 0.0001, +0.02 %) | 0.897 → 0.896 / 31.7 → 31.4 | **fails** |
| ODI | all | 11 | 24,324 | 0.837 | 4.7136 → 4.7126 (+0.0011 ± 0.0018, **+0.02 %**) | 0.1591 → 0.1591 (−0.0000 ± 0.0001, −0.01 %) | 0.900 → 0.899 / 47.7 → 47.5 | **fails** |
| T20I | all (reported) | 10 | 9,532 | 0.947 | 3.2236 → 3.2241 (−0.0006 ± 0.0017, −0.02 %) | 0.1315 → 0.1317 (−0.0002 ± 0.0002, −0.18 %) | 0.888 → 0.888 / 31.1 → 31.1 | reported: fails |
| TEST | all (reported) | 11 | 9,812 | 0.921 | 8.3351 → 8.3372 (−0.0022 ± 0.0030, −0.03 %) | 0.3037 → 0.3034 (+0.0003 ± 0.0006, +0.10 %) | 0.774 → 0.774 / 82.1 → 82.1 | reported: fails |
| T20 | women (reported, out of scope) | 11 | 30,099 | 0.463 | 2.3718 → 2.3721 (−0.0003 ± 0.0007, −0.01 %) | 0.1401 → 0.1400 (+0.0001 ± 0.0002, +0.08 %) | 0.898 → 0.897 / 23.4 → 23.2 | reported |

Every deciding delta is of the order of 0.02 % — twenty-five times under the band — and the
low-history slices, where an age effect had most room, say the same (debut rows: runs
−0.02 … −0.47 %, standard errors two to five times the effect; the one reading past a
standard error is wickets *worsening* on T20I's 207 debut rows). The tree already reads
`career`, `career_all` and the decayed rates, with which age is strongly collinear; the
residual — the trajectory at a given history — is worth nothing the loss can see. The
per-slice tables, the readings and a note on window coverage (the folds' rows carry a
date of birth for 65 % of men's T20 rows against X-1a's 82.6 % of all-time appearances,
because the recent windows hold more associate and domestic newcomers) are in plan §8.12.
`contract.AGE_FEATURES_KEPT` stays False.

**Family 3 — AGE-AWARE cold start — a recorded null.** The rating state pools, per format
and age band (cuts at 22 / 26 / 30 / 34, the population's quartiles), what earlier
debutants of that band did in their debut match, and — with `contract.AGE_AWARE_COLD_START`
on — `side_vectors` reads a player with no history in the format and a known age as that
band's profile (balls per match and the four shrunk impacts) instead of the neutral vector;
everyone with a match behind him, and every debutant without a date of birth, reads what
he read before. Gate `X-1b-cold-start` (H-23 triple registered first; two passes over the
archive, the prior off and on, every model refitted per fold on each pass's frame; decided
in both T20 and ODI by H-10 staying bounded against the control, the debut rows' pinball
improving beyond the band and a standard error, no player with history moving, and a
display-AUC / H-4 guard) **kept nothing**:

| format | folds | debut rows | history vectors max abs diff | display AUC none → prior | swap share | debut runs pinball none → prior | debut wickets pinball none → prior | H-10 bounded (4 probes) | verdict |
|---|---:|---:|---:|---|---:|---|---|---|---|
| T20 (men decide) | 11 | 3,381 | **0.0** | 0.730 → 0.729 (−0.0005 ± 0.0010) | 0.0031 | 2.1510 → 2.1574 (**−0.30 %** ± 0.15) | 0.1574 → 0.1582 (**−0.53 %** ± 0.32) | yes | **fails** |
| ODI | 11 | 1,025 | **0.0** | 0.708 → 0.706 (−0.0025 ± 0.0036) | 0.0000 | 3.5317 → 3.5559 (**−0.69 %** ± 0.73) | 0.1946 → 0.1967 (**−1.11 %** ± 1.0) | yes | **fails** |
| T20I (reported) | 10 | 207 | 0.0 | 0.750 → 0.745 (−0.0048 ± 0.0042) | 0.0055 | 2.5401 → 2.6088 (−2.71 % ± 1.6) | 0.1668 → 0.1767 (−5.92 % ± 2.6) | yes | reported: fails (and the AUC guard) |
| TEST (reported) | 11 | 422 | 0.0 | 0.643 → 0.649 (+0.0061 ± 0.0092) | 0.0023 | 7.3233 → 7.4991 (−2.40 % ± 1.2) | 0.4808 → 0.4966 (−3.28 % ± 3.4) | yes | reported: fails |

The prior does what it claims — a 19-year-old debutant now moves P(win) by −0.032 in ODI
where a 34-year-old moves it −0.014, no player with history moved by a bit, H-4 holds —
and the forecasts are **worse for it, in every format and for both targets**. The
performance model already reads `career` = 0 and learns its own debutant neutral jointly
with the side, the venue and the innings; handing it the band's *conditional* mean (balls
per match batted, over the matches with a ball) as if it were the player's history moves the
row into the region of established, poor batters and loses that conditioning. The full
tables, the H-10 probe per age and the reading are in plan §8.12; H-10's row now carries
the re-measured bounds (median Δp −0.034 / −0.022 / −0.021 / −0.013 for T20 / ODI / T20I /
TEST under today's neutral vector). `contract.AGE_AWARE_COLD_START` stays False; the pool
and the read path stay in the code, off.

**The harness after the choice.** `make evaluate` once on each source with the decided
configuration (no family kept): 88 min on the database (run beside the last cold-start
folds), 60 min on the archive with `BIRTH_DATES=` pointing at the exported CSV. Both read
22,818 matches, 21,096 training rows, 465,402 player rows and **6,955 players with a date
of birth** — the new `players_with_birth_date` count, the same on both sources. The
walk-forward table is A-4's baseline to the printed decimals (T20 objective 0.697 ± 0.039,
display 0.730 ± 0.050, runs pinball 2.921; ODI 0.673 / 0.707 / 4.715; E5 T20 0.503 against
0.506, fails, not served); the locked window (≥ 2026-09-02) holds the three matches
imported since A-3 and is still too small to score; H-8 parity **0.0 on both sources**
over 50 matches, 1,100 player rows, 1,100 performance predictions and 50 simulations, with
`age` and `age_known` now among the columns it compares on every player row; gates (both
X-1b triples in the embedded registry) and glossary pass.

**What shipped.** No feature. `ml/xi/biography.py` and the `birth_dates()` half of the
source protocol, the two age columns on every player row, the debut pool on the state, the
`--birth-dates` / `BIRTH_DATES=` route for the archive path and `make export-birth-dates`,
gates `X-1b-age` and `X-1b-cold-start` in the H-23 registry, the `frame-biography` node on
the system map, `scripts/experiments/xi/x1b_biography_features.py` (resumable per fold),
and `tests/test_xi_biography.py`. The rating artifact's payload grew a table and two arrays,
so a run written before this branch is refused with "retrain" (the D-6 policy) — the next
`make retrain` produces one this code serves. Nothing on the wire changed and no new
literal crosses a service boundary (H-24).

**Judgment calls, recorded.** *No curvature term:* the model is a tree ensemble, whose
splits are invariant to a monotone transform of a column, and age squared is monotone over
every age a cricketer has — the arm is age plus the indicator, and the record says why there
is no third column. *T20 decided on men's rows:* the fit still sees every row (production
serves women's T20 from the same artifact, through the indicator), the *verdict* is read
where four appearances in ten do not lack an age; the women's rows are reported beside it
and read the same as the men's. *Family 3 ran without family 1's columns:* the gate fixes
the performance model's columns at family 1's decided setting, and family 1 had failed in
ODI before family 3 started. *The debut pool has no decay,* so the archive's first seasons
(when every player is a "debutant" against a context baseline still at its prior) sit in
the lifetime sums; by the folds they are diluted (190–1,150 debut matches per T20 band),
and a decayed pool is the fix if the family were kept — it is not.

### X-3 — what the archive already knew about stakes, and what it bought

*Done on `feat/x-3-match-stakes`. No new source: the whole derivation reads Cricsheet's own
`info.event`, and the only acquisition was noticing that **the importer had been throwing
two of its four fields away**. Both uses are recorded nulls. The derivation ships anyway,
because it is now the thing either question would be re-asked with.*

**The gap that made the item possible.** `info.event` carries `name`, `match_number`,
`stage` and `group`; go-app stored the first two. Deriving a round from the event *name*
does not work — of the 1,102 distinct names in the archive, the 72 containing a stage word
are naming a *tournament* ("ICC Men's T20 World Cup Qualifier" is a qualifying competition,
every one of its 70 matches included), not the round a match was played in. `event.stage`
is the round, and it is present on 1,584 of 22,818 matches — 7 %, which sounds useless until
you see *where*: in a league with playoffs it marks exactly the playoffs, and the league
matches carry a `match_number` instead (Indian Premier League: 74 stage-labelled of 1,243,
the other 1,169 numbered). Migration `0011_match_event_stage.sql` adds `event_stage` and
`event_group`; the importer stores both verbatim (a pool is a string in most files and a
bare number in the rest, so `cricsheet.FlexibleTag` normalises); the archive was re-imported
and the database now holds 1,584 stages and 6,448 groups — the archive's own counts to the
digit. `UpsertMatch` and `UpsertMatchTx` ran two hand-copied statements and now share one,
because a column added to one of them would have reached half the callers.

**The derivation** is `ml-service/ml/xi/stakes.py`, called by *both* rating-pass sources over
the matches they are about to yield, so the two produce the same labels or `make xi-parity`
says which count they differ on. A **stage label** per match — `group` / `knockout` / `final`
/ `bilateral`, or unlabelled — read from `event.stage` through the vocabulary the archive
actually spells, and where it states none, from the shape of the *edition*: two clubs makes a
bilateral series, three or more with a number or a pool makes a group fixture, anything else
stays unlabelled. An edition is one competition, one format, one gender, with no gap longer
than 60 days between consecutive fixtures — because a table is a fact about one running of a
competition, and "Indian Premier League" is one event name and eighteen tables.

**Label coverage, over all 22,818 matches.**

| scope | matches | stage known | group | knockout | final | bilateral | unlabelled | table reconstructible | dead rubbers |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| T20 | 12,356 | 12,076 (97.7 %) | 10,284 | 699 | 306 | 787 | 280 | 9,794 (79.3 %) | 1,266 |
| T20 men | 9,485 | 9,236 (97.4 %) | 7,997 | 537 | 213 | 489 | 249 | 7,603 (80.2 %) | 952 |
| T20 women | 2,871 | 2,840 (98.9 %) | 2,287 | 162 | 93 | 298 | 31 | 2,191 (76.3 %) | 314 |
| T20I | 2,129 | 2,063 (96.9 %) | 728 | 91 | 53 | 1,191 | 66 | 1,792 (84.2 %) | 240 |
| ODI | 5,218 | 4,978 (95.4 %) | 2,854 | 142 | 106 | 1,876 | 240 | 3,933 (75.4 %) | 603 |
| TEST | 3,115 | 2,897 (93.0 %) | 1,969 | 0 | 14 | 914 | 218 | 1,113 (35.7 %) | 87 |
| **all** | **22,818** | **22,014 (96.5 %)** | 15,835 | 932 | 479 | 4,768 | 804 | **16,632 (72.9 %)** | 2,196 |

The archive spells **55 distinct stages**; the vocabulary recognises 53. The two it does not
are `ODI` and `T20` (4 matches), which name a format rather than a round, and leaving them
unlabelled is the right answer rather than a gap. The unlabelled 804 are matches with no
event name (a fixture belonging to no competition the archive names) or a tournament fixture
carrying neither a number nor a pool.

**The dead-rubber flag exists only where a table is reconstructible as-of, and that is
stated per match rather than assumed.** Two shapes qualify. A **bilateral series** — the
"table" is the scoreline, and the series is dead once one side has won more than half of its
fixtures. A **league with playoffs** — the number of qualifying places is the number of clubs
that played the edition's knockout matches, and a side is dead when it is *mathematically*
out of the top *k* (at least *k* rivals already hold more points than it can reach by winning
everything left) or *mathematically* in it (at most *k* − 1 rivals can reach its current
points at all). Both tests use rivals' current points against the side's maximum, which
cannot fire on a table still open, and both ignore net run rate, which only ever makes a
place harder to take. A match is a dead rubber when **either** side is dead. Two situations
are honestly *not* flagged: a round robin whose knockout round the archive does not label
(no observable cut), and a cut that takes every club — a four-team edition whose knockout is
two semi-finals decides nothing, so it says nothing. That is why the flag reaches 72.9 % of
matches and not more, and why TEST reaches only 35.7 % (its "tournament" is the World Test
Championship, whose final is one match between two of nine clubs).

Spot-checked by hand against a season anyone can verify: IPL 2019 flags **4 of its 56 league
matches**, all in the last five days (matches 50, 54, 55 and 56), and no knockout — which is
what that season's table did.

**H-21, stated precisely, because half of this is fixture knowledge and half is results.**
The stage label uses only what is knowable before the first ball: the fixture's own event
fields and the number of clubs in its edition. The dead-rubber flag additionally uses the
edition's **fixture list** (who still has matches, and how many) and the **number of
qualifying places** — both published before a season starts, and both read here from the
archive as a proxy for that publication, which is the one backward-looking input and is
recorded rather than hidden. **Results are read strictly as-of**: only matches on dates
strictly *before* a fixture's own date reach its table, so a match never learns the result of
another played the same day (the rating pass's day-close rule, applied to a points table).
This is exactly why the two halves are used differently: the **knockout flag is the one X-3
offers a model**, and the **dead-rubber flag is measurement-only** and is deliberately not a
column in the training frame.

**What reaches the frame.** `contract.STAKES_COLS` — `stakes_knockout` and
`stakes_stage_known` — on every win row, from both sources, compared by H-8 (a serving record
built for an unplayed match carries no stakes and reads 0.0 on both, the unlabelled category
rather than an implied league game). `make xi-parity` gained four counts and they agree
exactly across the database and the archive: **22,014 stage labels, 1,411 knockouts, 16,632
reconstructible tables, 2,196 dead rubbers**, on both sources.

#### Use 1 — E5 hygiene: the null survives the cleaning

§8.6 read T20's E5 failure as rotation noise — "in domestic T20, much of a 1–3 player change
is squad rotation". Gate `X-3-e5` tests that reading directly: the same pairs, the same
objectives, the same lineup-only definition, varying only which pairs the measurement reads,
with the bar re-derived on each arm's own weights under three seeds (a filter changes the
sampling noise as well as the sample, and a bar that did not move with it would be rigged).
A pair is suspect when **either** of its two matches was a dead rubber or a knockout, because
rotation in either contaminates the comparison.

| format | arm | pairs | effective | agreement ± se | exactly-right | bar (3 seeds) | verdict |
|---|---|---:|---:|---|---:|---|---|
| **T20** | all (control) | 5,194 | 2,169 | 0.5030 ± 0.0114 | 0.522 | 0.5050–0.5059 | fails |
| **T20** | exclude dead rubbers | 4,398 | 1,833 | **0.5046** ± 0.0117 | 0.521 | 0.5030–0.5040 | **clears, by 0.0006** |
| **T20** | exclude knockouts | 4,563 | 1,865 | 0.4976 ± 0.0116 | 0.522 | 0.5039–0.5049 | fails |
| **T20** | exclude both | 3,919 | 1,601 | 0.4972 ± 0.0125 | 0.521 | 0.5011–0.5031 | fails |
| **T20** | down-weight both (½) | 5,194 | 1,885 | 0.5005 ± 0.0115 | 0.521 | 0.5042–0.5053 | fails |
| ODI | all (control) | 1,475 | 692 | 0.5665 ± 0.0188 | 0.534 | 0.5030–0.5045 | passes |
| ODI | exclude dead rubbers | 1,238 | 578 | 0.5796 ± 0.0205 | 0.535 | 0.5008–0.5035 | passes |
| ODI | exclude knockouts | 1,413 | 654 | 0.5749 ± 0.0193 | 0.534 | 0.5031–0.5043 | passes |
| ODI | exclude both | 1,192 | 551 | 0.5844 ± 0.0210 | 0.534 | 0.4991–0.5018 | passes |
| ODI | down-weight both (½) | 1,475 | 622 | 0.5744 ± 0.0198 | 0.534 | 0.5023–0.5043 | passes |
| T20I | all (control) | 571 | 220 | 0.5636 ± 0.0334 | 0.523 | 0.4715–0.4720 | passes |
| T20I | exclude both | 371 | 152 | 0.5658 ± 0.0402 | 0.517 | 0.4533–0.4534 | passes |
| TEST | all (control) | 461 | 213 | 0.5493 ± 0.0341 | 0.531 | 0.4759–0.4785 | passes |
| TEST | exclude both | 417 | 191 | 0.5602 ± 0.0359 | 0.532 | 0.4740–0.4759 | passes |

Suspect share of the pooled pairs: **T20 dead rubber 15.3 %, knockout 12.1 %**; T20I 31.0 % /
4.6 %; ODI 16.1 % / 4.2 %; TEST 8.0 % / 1.7 %. The filters have real bite — the T20 arm that
drops both loses a quarter of the pairs — so this is not a null for want of a treatment.

**The wiring check, because a control has to be the metric it claims to be.** Restricted to
the pairs the folds held *before* A-4 rotated the locked window, the control reads **T20
0.4897 over 1,358 pairs, ODI 0.5618 over 429, T20I 0.5856 over 111** — P-7's recorded
walk-forward figures (0.490 / n=1,358, 0.562 / n=429, 0.586 / n=111) to every digit the plan
prints. The control above differs from those only because a year of pairs retired into the
folds with the rotation; the arms vary the filter and nothing else.

**The reading, and it is a null.** One arm of five crosses its bar, and it crosses it by
**0.0006 against a standard error of 0.0117** — one twentieth of a standard error, which is
not a measurement, and A-1's lesson (a gate with no effect-size floor passed on 0.2 runs) is
exactly this shape. Every other arm moves the number **down**: removing knockouts costs
0.005, removing both costs 0.006, and down-weighting instead of dropping costs 0.003. If
rotation noise were masking a signal, cleaning it out would raise the agreement in every arm
that removes it and raise it most where most is removed; the opposite happens. **Rotation
noise was not masking selection signal in T20.** The scoping does not move, T20 stays
rating-ordered (`optimizer.NOT_OPTIMISED_REASONS`), and nothing in this PR touches it — the
gate said before it ran that it decides nothing, and it decided nothing.

The one thing worth carrying forward is on the other side of the ledger: **ODI's agreement
rises monotonically with the cleaning** (0.5665 → 0.5749 → 0.5796 → 0.5844 as more suspect
pairs go), and TEST's rises with the knockouts removed. Both already pass; the movement is
inside one standard error in both, so it is a direction and not a finding, and it is recorded
here rather than acted on.

#### Use 2 — a stakes feature in the display model: a null

Gate `X-3-stakes`, on the same eleven folds and three display seeds, with the two stakes
columns added to `XI_FEATURE_COLS + TEAM_CONTEXT_COLS` and everything else held.

| format | display AUC (control) | with stakes | Δ ± se (paired over folds) | control seed sd | Brier Δ | swap-violation share (control → arm) |
|---|---:|---:|---|---:|---|---:|
| T20 | 0.7299 | 0.7299 | +0.0000 ± 0.0002 | 0.0008 | −0.0000 | 0.0478 → 0.0498 |
| T20I | 0.7495 | 0.7516 | **+0.0021 ± 0.0010** | 0.0000 | −0.0001 | 0.0684 → 0.0659 |
| ODI | 0.7083 | 0.7084 | +0.0000 ± 0.0005 | 0.0000 | −0.0001 | 0.0307 → 0.0307 |
| TEST | 0.6431 | 0.6431 | +0.0000 ± 0.0000 | 0.0000 | +0.0000 | 0.0616 → 0.0616 |

The registered rule asks for a rise beyond the noise **in every format**. Three formats move
by less than a ten-thousandth of AUC; only T20I moves at all, by 0.0021 (two standard errors)
in the format where knockouts are 7 % of matches and a World Cup semi-final is a different
kind of fixture from a bilateral tour game. One format of four does not keep a family, so
**`STAKES_FEATURES_KEPT` stays `False`** and the display model reads neither column. The
columns stay in the frame, which is what makes the question re-askable without another
rating pass.

**A defect in this gate's own wording, recorded rather than quietly fixed.** The registered
triple also asked for "the swap-violation share of the display surface still under H-4's 2 %".
That line belongs to the *objective*, whose surface is monotone-constrained on every stem it
reads; the **display** model reads team context as well and violates at **3–7 % in the
control**, so the clause fails identically in both arms and cannot discriminate. The
comparison it was reaching for is arm against control, and that is **+0.0020 (T20), −0.0025
(T20I), 0.0000 (ODI), 0.0000 (TEST)** — the stakes columns do not reshape the surface. The
verdict above therefore rests on the AUC clause alone, and the next gate of this kind states
its bar against the control it will actually be measured against. (This is the same class of
mistake A-1 recorded: a rule written before the control's own value was known.)

#### What shipped, and what did not

Shipped: migration `0011`, the two importer fields, `ml/xi/stakes.py` and its unit tests, the
stakes columns on the win row from both sources, four parity counts, two H-23 gate entries,
and `scripts/experiments/xi/x3_match_stakes.py` (coverage, both gates, `--decide`). Not
shipped: any change to a model, a feature, a threshold or a format's scoping.
`STAKES_FEATURES_KEPT` is `False`, `DISPLAY_FEATURE_COLS` is byte-for-byte what it was, and
`make evaluate` reproduces the run before it.

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
