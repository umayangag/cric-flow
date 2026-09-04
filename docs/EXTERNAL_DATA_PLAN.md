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
the push and PR commands. Order: **X-4 → X-1a → X-1b → X-3 → X-2.** X-4 goes first
because it prices the whole hunt: if the display model already sits at market accuracy,
the remaining items are curiosities and can be taken slowly or not at all. D-12 (a defect fix, not an experiment) is runnable at any time
and does not wait for X-1a.

**Explicit non-goals**, so effort is not re-spent: official rankings (redundant with the
system's own Elo), pitch reports (no structured source exists), injury/fitness data (not
freely or reliably available), squad/availability feeds (a product problem —
[PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md) Phase 2), other ball-by-ball archives (Cricsheet
is the free canonical source), and **odds as a model input** — X-4 uses the market as a
yardstick, never as a feature.

---

## X-4 — the market benchmark (model: Opus; this is PRODUCT_ROADMAP P0-1)

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

## X-1a — player biographies acquired from Wikidata (model: Opus)

**What.** Date of birth, batting handedness, bowling style and — where Wikidata carries
it — the career end date or a retirement statement, for the player registry,
joined via the ESPNcricinfo id that Cricsheet's people registry and Wikidata both carry
(CC0-licensed). Acquisition and coverage only — no features. **Gate:** coverage worth
building on — matched biography with DOB for players covering ≥ 80 % of match
appearances in each limited-overs format (report the true figure per format and gender
whatever it is).

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
| X-4 | open — run first; prices the rest |
| X-1a | open |
| X-1b | open — gated on X-1a's coverage |
| X-3 | open |
| X-2 | open — run last |
| D-12 | **fixed** — recency-bounded default pool, manual picking, the retirement ledger; measurement below |

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
