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
the remaining items are curiosities and can be taken slowly or not at all.

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

**What.** Date of birth, batting handedness and bowling style for the player registry,
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
   leg-spin/left-arm-orthodox/left-arm-wrist, plus unknown). License: CC0 — note it.
2. Storage: a player_biography table (forward migration, next number) keyed by the
   registry id, written by the backfill; the importer does not change. The backfill
   becomes a documented optional step beside the cadence (it changes rarely; weekly
   with the cadence or on demand).
3. The coverage report, per format and gender, weighted by match appearances: matched /
   DOB present / style present; unmatched players listed by appearance count so the
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

---

## Record of outcomes

| id | status |
|---|---|
| X-4 | open — run first; prices the rest |
| X-1a | open |
| X-1b | open — gated on X-1a's coverage |
| X-3 | open |
| X-2 | open — run last |
