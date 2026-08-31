# Player and team identity: PR checklist

**Goal.** Every player is one key and one career. Every team is one key per side that
actually took the field. Today neither holds, and the features every model consumes are
aggregated over the wrong sets because of it.

Implement **one PR at a time**; mark status in the table below as work progresses.

---

## Why this plan exists

Players and teams are identified **by name string**:

```go
// db/cache.go:29 — the only player identity in the system
func (c *EntityCache) GetPlayerID(ctx context.Context, name string) (int64, error) {
	if id, ok := c.players.Load(name); ok { return id.(int64), nil }
	...
	id, err := GetOrCreateByName(ctx, name)
```

A name is not an identity. Measured against the 22,734 Cricsheet files, restricted to
people who actually appear in a squad:

| | count | effect today |
|---|---|---|
| Names covering **more than one person** | **163** | **348 people** collapsed into 163 `player_id`s |
| People spelled **more than one way** | **44** | one career split across **88** `player_id`s |
| Team names used by **both a men's and a women's side** | **130** of 394 | one `opposition_id` for two teams |
| Franchise renames | ≥ 9 confirmed | one club under two `opposition_id`s |

Cricsheet gives 13,568 distinct squad people; the name strings give 13,427. The database
has 13,419 player rows. **The system is short roughly 150 people and has invented ~44
duplicates.**

### What it costs

```
Shahid Afridi     two people    465 + 25 squad appearances
Rashid Khan       three people    2 + 400 + 17
SR Taylor         two people    293 + 148          ← a near-even blend
A Mishra          three people    3 + 1 + 255
```

`feature_raw_stats_snapshots` and `player_window_features` are keyed by `player_id`. So
`batting_mean_w5` and `batting_std_w10` for "SR Taylor" average two different cricketers'
careers — and those are exactly the values the win model aggregates over a proposed XI
(`ml/win_features.py`, `_GROUP_TO_PLAYER_KEY`). The same applies to every base model.

On the team side, `team1_opposition_id` and `team2_opposition_id` are model inputs and
carry real weight in the trained win artifacts (TEST 0.050, T20I 0.019, T20 0.016). With
130 names shared across genders, one of those integers can mean Australia's men's side or
Australia's women's side depending on the row — and the win export applies no gender
filter, so both train the same model:

| format | female | male |
|---|---|---|
| ODI | 942 | 4,254 |
| T20 | 2,763 | 9,259 |
| T20I | 760 | 1,346 |
| TEST | 24 | 3,077 |

**4,489 of 22,425 matches — 20% — are women's cricket**, sharing team identities with the
men's game.

---

## The thing that makes this tractable

**Cricsheet already ships a stable person identifier.** `info.registry.people` maps each
name in a file to an id that is consistent across the whole dataset:

```json
"registry": { "people": { "NR Sciver-Brunt": "f3a18a0c", "KH Sciver-Brunt": "6a434bd3" } }
```

It resolves both directions at once, which no name heuristic can do:

```
f3a18a0c  ->  {"NR Sciver": 283, "NR Sciver-Brunt": 171}     one person, two names
"S Smith" ->  {0fca9dee, 18a0ba24}                            one name, two people
```

**This is why the plan does not build a name-normalisation map.** Normalising names to the
most frequent spelling would fix the 44 split careers and make the 163 collisions worse,
since merging is the operation that creates them. It is also not stable: the most frequent
spelling of `f3a18a0c` is currently the *former* name `NR Sciver` (283 vs 171), and the
winner flips as matches accumulate — a canonical key that silently changes over time.

Use the identifier as the key; use the most recent name for display.

**One case the registry cannot resolve**, recorded so it is not rediscovered: the registry
is keyed by name *within a file*, so when two namesakes appear in the same match it
collapses them. `1130677.json` has 22 squad entries and 21 identifiers — one `KV Sharma`
for both Vidarbha and Railways. Two files in the dataset do this and the importer already
drops the contested name from both squads (S-3c in
[WIN_PROB_SELECTION_PR_CHECKLIST.md](WIN_PROB_SELECTION_PR_CHECKLIST.md)). That behaviour
stays: the source genuinely does not know.

---

## Status

| ID | Status | PR branch (when done) | Title |
|----|--------|----------------------|-------|
| I-1 | done | `arch/p-1-identity` | Capture the Cricsheet person identifier at import |
| I-2 | done | `arch/p-1-identity` | Key players by that identifier, not by name |
| I-3 | done | `arch/p-1-identity` | Team identity includes gender |
| I-4 | todo | `identity/i-4-team-lineage` | Franchise renames are one club, not two |
| I-5 | partly | `arch/p-1-identity` | Rebuild the derived data and price the change |

**Status legend:** `todo` | `in_progress` | `done` | `skipped` | `blocked` | `partly`

I-1, I-2 and I-3 landed together as **P-1** of
[ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md), which is the
active plan; splitting them would have meant three re-imports of the same 22,734 files to
measure one change. The column is named `player.external_id` there rather than
`cricsheet_person_id`. I-5 is `partly` because the XI path is rebuilt and priced (E4
below) while the legacy base models still need `precompute-features` re-run — see its
section.

**Dependency order is strict.** I-2 needs I-1's column populated. I-5 needs everything
above it, and nothing above it is measurable until I-5 runs.

---

## Relationship to the win-probability plan

[WIN_PROB_SELECTION_PR_CHECKLIST.md](WIN_PROB_SELECTION_PR_CHECKLIST.md) is the active
plan and **should finish S-3c first**. Two reasons:

1. **The leak is the larger, proven effect.** S-3c removes a feature that scores held-out
   AUC 0.89–0.94 on its own. Identity error blends careers at the margin; it does not hand
   the model the scoreboard.
2. **Sequencing gives two measurements instead of one.** Run S-3c, retrain, record. Then
   run this plan, retrain, record. Doing both at once produces a single number that cannot
   be attributed to either.

**S-7 (venue and opposition ID encoding) is blocked on I-3 and I-4.** Any target or
frequency encoding fitted before the identities are merged learns the rename as a new team
with 63 matches of history, and learns one encoding for two genders of the same nation.

---

## Conventions for every PR in this list

- **Branch:** `identity/<id>-<slug>`, lowercase. Never commit to `main`.
- **Commit:** Conventional Commits with a scope and the ID, e.g.
  `feat(import): capture the Cricsheet person identifier (I-1)`.
- **One concern per PR.** Split anything past ~600 changed lines.
- **Docs in the same branch.** `docs/ml-and-training.md` and `ARCHITECTURE_MAP.md`
  (`make gen-architecture-map`) when a contract changes.
- **Baseline verification** (before and after; both must pass): `make check-all`.
- **Coverage ratchets up, never down.** Current gates: `go-app` `COV_MIN=65`,
  `ml-service` `COV_MIN=81`, frontend lines 65 / functions 66 / statements 65 /
  branches 74.
- **Update this file in the same PR:** set the row to `done`, fill in the branch.

---

## I-1 — Capture the Cricsheet person identifier at import

**Problem.** The importer parses `info.players` (S-3c) but discards `info.registry.people`,
which is the only thing in the source that distinguishes two people who share a name.

**Change.**

1. `cricsheet.Info` gains `Registry` with `People map[string]string`.
2. Migration: `player.cricsheet_person_id character varying(32)`, nullable and `UNIQUE`.
   Nullable because a file may lack a registry entry for a name; unique because that is the
   claim being made.
3. The importer resolves the identifier per file and stores it on the player row it
   creates or finds. **This PR changes no keys** — `GetPlayerID(name)` still decides
   identity, and the column is written alongside for I-2 to act on.

Splitting capture from re-keying keeps the risky migration separate from the parsing, and
lets the column be inspected on real data before anything depends on it.

**Tests.** Registry parsed; a name with no registry entry stores null rather than an empty
string; the identifier is stable across two files naming the same person; the in-match
namesake case stores the one identifier the registry gives and does not fail.

**Acceptance.** After a re-import, `SELECT count(*) FROM player WHERE cricsheet_person_id
IS NULL` is small and every null is explainable. Report the number in the PR body.

**Risk.** Low. Nothing reads the column yet.

> **Measured (P-1, `arch/p-1-identity`).** The column is `player.external_id`.
> `SELECT count(*) FROM player WHERE external_id IS NULL` is **0** of 13,623 rows: every
> name used in a squad or on a delivery across the 22,734 files has a registry entry. The
> fallback path exists and is logged, and never fired on this dataset.
>
> **The lookup has to trim both sides.** Four registry *keys* in the dataset carry a
> trailing space -- `"Lalchhuanliana "` -- while the squad and delivery entries naming the
> same person do not. An exact-match lookup dropped that person onto the name-keyed
> fallback and split one career across two rows, which is the bug this item removes. No
> file has two identifiers whose names differ only by surrounding space, so trimming
> cannot merge two people.

---

## I-2 — Key players by that identifier, not by name

**Problem.** 163 names hold 348 people; 44 people hold 88 rows. Every derived feature
inherits both errors.

**Change.** Identity becomes the person identifier, with the name demoted to a display
attribute:

- `GetPlayerID` is replaced by a lookup on `cricsheet_person_id`, falling back to name only
  when the source has no identifier. The fallback must be **explicit and logged**, not a
  silent default — it is the path that reintroduces the bug.
- `player.player_name` becomes the *most recent* name seen for that identifier, not the
  first or the most frequent. `NR Sciver` → `NR Sciver-Brunt` is a person's current name;
  frequency picks the old one and changes its mind later.

**Rebuild rather than backfill.** An in-place migration would have to re-point
`batting_data`, `bowling_data`, `fielding_data`, `fielding_event`, `ball_event` (including
`fielder_ids bigint[]`), `match_player` and eight feature tables — 20 player-bearing
columns across 17 tables — and for a collapsed name it would need the per-match source to
know *which* of the two people each row belongs to. That information is in the JSON, which
means the backfill is a re-import wearing a disguise. The import takes ~75 seconds and this
project has no backward-compatibility requirement, so: wipe the match-derived tables and
re-import. Say so in the PR body rather than discovering it halfway.

**Tests.** Two people sharing a name get two ids; one person under two spellings gets one
id; the display name follows the most recent match; a name with no registry entry falls
back and logs; the in-match namesake still yields no `match_player` row for either side.

**Acceptance.** `SELECT count(*) FROM player` lands near **13,568** (from 13,419), and the
163 known collisions each resolve to ≥ 2 rows. Spot-check `SR Taylor`, `Rashid Khan` and
`Shahid Afridi` in the PR body.

**Risk.** This invalidates every precomputed feature and every model artifact. That is
what I-5 is for. Do not run it on a day when a model number is needed.

> **Measured (P-1).** `player` holds **13,623** rows, from **13,483** on the same dataset
> under name-keying: **140 people recovered**. 13,568 is the count of registry ids that
> appear in a *squad*; the extra 55 are people who appear only on a delivery, as
> substitute fielders.
>
> The spot-checks resolve: `SR Taylor` → 2 rows, `Rashid Khan` → 3, `Shahid Afridi` → 2,
> `A Mishra` → 3. `NR Sciver-Brunt` is one row under her current name.
>
> **The database is not the source's own count of shared names.** In the source, 163 squad
> names cover 348 people. In `player` afterwards, **162 names are still shared by 345
> people** -- because the display name settles to the *most recent* spelling, so a person
> whose current name differs from their namesake's no longer shares the string.
>
> **Display name.** `player.player_name` is the spelling from the player's latest match,
> with `player.name_as_of` recording which date it came from. The rule cannot be "first
> written": the import is concurrent, so first-writer-wins makes two runs of the same
> dataset disagree on a name. It is settled once at the end of the import from a rule
> that does not depend on arrival order (latest date, then the greater string), and rows
> on the name-keyed fallback are excluded, because for those the name *is* the identity.

---

## I-3 — Team identity includes gender

**Problem.** 130 of 394 team names are used by both a men's and a women's side, sharing one
`opposition_id`. `Australia` is one row for two teams. 20% of matches are women's cricket
and the win export applies no gender filter, so `team1_opposition_id` means different
things in different training rows.

**Change.** `opposition` gains `gender`, and identity becomes `(opposition_name, gender)`.
`match.gender` already carries the value, so the importer has it at hand.

**Open decision — see below.** Whether the *models* should also be split by gender is a
separate question from whether the *identity* should be. This item settles identity only;
mixing men's and women's matches in one model is then a deliberate choice rather than an
accident of the schema.

**Tests.** Two sides with the same name and different genders get different ids; a match's
squad rows carry the opposition matching that match's gender; an existing single-gender
team is unaffected.

**Acceptance.** `opposition` grows by roughly 130 rows. No match has a squad whose
opposition gender disagrees with `match.gender` — assert it as a query in the PR body.

**Risk.** Low in isolation; it changes the meaning of an existing model feature, which I-5
measures.

> **Measured (P-1).** `opposition` went from **394** rows to **524** — exactly the
> predicted **+130**, split 364 men's and 160 women's. The disagreement query returns
> zero:
>
> ```sql
> SELECT count(*) FROM match_player mp
> JOIN match m ON m.match_id = mp.match_id
> JOIN opposition o ON o.id = mp.opposition_id
> WHERE o.gender IS DISTINCT FROM m.gender;   -- 0
> ```
>
> Open decision 1 keeps its default: identities only. The models still mix genders, but
> now because nobody has decided otherwise rather than because the schema could not tell
> them apart.
>
> **One serving-path consequence.** `PredictTeams` takes a team *name* and a format and no
> gender, so a name alone can no longer name one team. `db.FindOppositionIDForFormat`
> resolves it to the side that has actually played that format, most recently, and logs
> when the name was ambiguous. The proper fix is for the caller to carry the gender, which
> arrives with P-5's ids-in serving path.

---

## I-4 — Franchise renames are one club, not two

**Problem.** A club that renames becomes two unrelated integers:

```
 id  |       opposition_name       | matches | first_match | last_match
  292| Royal Challengers Bangalore |     258 | 2008-04-18  | 2024-03-17
 1038| Royal Challengers Bengaluru |      63 | 2024-03-22  | 2026-05-31
```

**How to find them, and what does not work.** String similarity is useless here: it pairs
`Barbados Tridents` with `Barbados Royals` (a real rename) and `Birmingham Bears` with
`Birmingham Phoenix` (two different teams in two different competitions, coexisting).

What works is **no temporal overlap plus roster carry-over across the boundary**. Comparing
whole-history rosters fails — RCB's 180 people over sixteen years against Bengaluru's 69
overlap only 41%. Comparing the **last 15 team-innings before against the first 15 after**
gives a clean signal. Run over the dataset it yields eight candidates and every one is a
genuine rebrand:

```
 87%  Himachal          -> Himachal Pradesh
 78%  Lightning         -> The Blaze
 65%  Jaffna Stallions  -> Jaffna Kings
 63%  Deccan Chargers   -> Sunrisers Hyderabad
 61%  Kings XI Punjab   -> Punjab Kings
 55%  Delhi Daredevils  -> Delhi Capitals
 50%  Surrey Stars      -> South East Stars
 50%  Kathmandu Gurkhas -> Kathmandu Gorkhas
```

The whole-history variant of the same rule produced `West Indies -> Jamaica Kingsmen`, a
national side and a franchise. The boundary-local version does not.

**RCB is missed by the rule and must be added by hand**, for a reason worth knowing: the
club fields a men's and a women's side under one name, so the innings either side of the
boundary are WPL and IPL squads and barely overlap. After I-3 splits the genders the rule
would find it, which is why this item comes after I-3.

**Change.** `opposition.canonical_id`, self-referencing and null for a club that has never
renamed. Exports and features resolve through it; the display name stays per-row so a
2019 scorecard still reads `Delhi Daredevils`.

**Detection is a one-off, not a runtime feature.** Produce the candidate list with a
throwaway script, review it by hand, and commit the resulting mapping as data. A heuristic
that runs on every import would silently merge two clubs the first time a coincidence
clears its threshold. Nine mappings do not need automation; they need a reviewer.

**Tests.** A match under either name resolves to one canonical id; the display name is the
one recorded for that match; a club with no successor resolves to itself.

**Acceptance.** The nine known pairs resolve to nine canonical ids. `Royal Challengers`
returns one club across 321 matches.

**Risk.** A wrong merge is invisible and corrupts a team's whole history. Hence hand
review, and hence the mapping is committed rather than computed.

---

## I-5 — Rebuild the derived data and price the change

**Problem.** I-1 to I-4 change what a player and a team *are*. Nothing downstream knows
until it is rebuilt, and no claim about whether it helped can be made until it is measured.

**Change.** Re-import → re-precompute → re-export → re-train → re-measure.

**Budget it honestly.** Precompute is the expensive step and dominates everything else:

| step | measured |
|---|---|
| import | ~75 s |
| **precompute-features** | **2 h 40 m – 3 h 50 m** |
| export-dataset | ~4 m 30 s |
| train (per model) | 5 s – 4 m 30 s |

**Acceptance.** Report before and after, on the same cutoff:

- Win-model held-out AUC and Brier per format (`make win-discrimination`), against the
  numbers S-3c leaves behind — **not** against the pre-S-3c numbers, which measured a leak.
- Player-level MAE for batting and bowling, which is where merging 348 careers into 163 and
  splitting 44 across 88 should show up most directly.
- `SR Taylor`, `Rashid Khan`, `NR Sciver-Brunt` career rows before and after, as the
  human-readable check that the change did what it claims.

**An honest possible outcome:** the metrics barely move. 163 collisions out of 13,568
people is ~1% of the roster, and the affected players are not uniformly the ones being
selected. Record that if it happens — the identity is still wrong today, and a plan that
can only be justified by a metric win will quietly rot the moment the metric disagrees.

**Risk.** Long-running and hard to interrupt cleanly; `precompute-features` has been
cancelled mid-run before. Run it when nothing else needs the box.

> **Measured (P-1) — that is what happened.** E4 in
> [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md) trains the XI
> win models on two databases built from the same 22,734 files by the same model code at
> the same cutoff, differing only in identity. No format and no gender subset moves by
> more than its own holdout can resolve; the largest observed delta, T20I women +0.042
> display AUC, sits on 70 matches whose 95% resolution is ±0.154. The full table is in the
> plan's P-1 row.
>
> The identity is still wrong without this change, and the rebuild is what makes the
> claim checkable at all. It is a correctness change that does not pay in discrimination,
> and saying so here is the point of writing the expectation down first.
>
> **Still to do for I-5:** only the XI path is rebuilt. `precompute-features`,
> `export-dataset` and the legacy base models read tables the migration truncates, so
> **no batting/bowling/fielding number can be quoted until precompute is re-run**. That is
> deliberate: those tables and their models are deleted by P-5/P-6, and spending three
> hours of precompute to price a model that is being removed is not worth the box time.
> The player-level MAE half of this item's acceptance is therefore not measured, and the
> plan's migration sequence is where it would be if it were.

---

## Open decisions

| # | Decision | Needed by | Default if unanswered |
|---|---|---|---|
| 1 | Should the models also be **split** by gender, or only the identities? 20% of matches are women's cricket, and a T20 model currently learns both | I-3 | Identities only. Splitting the models halves the data for every format and is a modelling decision this plan should not smuggle in — **taken as the default in P-1**; the XI report now splits its *metrics* by gender (E4) so the question can be answered with numbers when H-7's E7 asks it |
| 2 | When the source has no registry entry for a name, fall back to name-keying or refuse the row? | I-2 | Fall back **and log**. Refusing costs real matches over a source gap, which is the trade S-3c already rejected — **implemented in P-1**; it never fired on this dataset |
| 3 | Should `canonical_id` merge a franchise that was *replaced* rather than renamed — Deccan Chargers → Sunrisers Hyderabad was a new owner and a new squad, not a rebrand | I-4 | Merge it. The alternative is a judgement about corporate continuity that the data cannot settle; record the assumption in the PR body |

---

## Out of scope

- **Venue identity.** Venues have the same class of problem (`venue_id` is name-keyed and
  fed to trees raw) but no equivalent registry to resolve it, so it needs a different
  method. S-7 in the win plan covers the encoding question.
- **Splitting the models by gender.** See open decision 1.
- **Any change to feature windows or the raw stats contract.** This plan changes *who* a
  row belongs to, not *what* is computed. Keeping those separate is what makes I-5's
  before/after comparison mean anything.
