# Bug backlog

Bugs found while working a planned item — `docs/FOLLOW_UP_PLAN.md`,
`docs/EXTERNAL_DATA_PLAN.md`, `docs/PRODUCT_ROADMAP.md` — that are outside that item's
scope. A non-blocking bug is
recorded here and left alone; a blocking one gets its own `fix/<slug>` PR stacked under the
item, and the row names it.

| id | found during | symptom | where in the code | why it matters | blocking | status |
|---|---|---|---|---|---|---|
| B-1 | A-1 | Reading team-level context for a team, venue or head-to-head pair the state has never seen *writes* that key into the state: `team_context` indexes `defaultdict`s, so a serving request naming an unknown opposition id (or one that names teams but no venue) grows `team_elo`, `team_results`, `head_to_head`, `venue_bat_first` and `team_venue_matches` by one entry per read. The numbers served are the right neutral ones; the loaded state is not the state that was loaded. | `ml-service/ml/xi/ratings.py`, `RatingState.team_context` (and `update`, whose `defaultdict` indexing is correct there) | The D-7 defect class (plan §10.6): a read that mutates the serving state. `_read_slots` closed it for players in P-5; the keyed tables were left as they were. `fixture_context` (A-1) reads with `.get` for this reason. Harmless to the numbers today; it is what makes a long-lived `XiStore` drift under traffic and would make an H-8 comparison after serving depend on what was served. | no | open |
| B-2 | A-5 | `POST /admin/reload` with no `?run=` loaded the run `current` already named instead of the newest one. `current` is set by every reload, so on any box that had reloaded once, the reload after a retrain re-loaded the run already serving: 200, run plan complete, run history green, and `/xi/status` still reporting the older run's `ratings_through`. Two runs built on the dev box were never served — the database held matches through 2026-09-01 while the served ratings ran through 2026-08-25 (9 days, H-11 refuses at 14). | `ml-service/app/main.py`, `admin_reload`; the belief was written down in `go-app/internal/server/step_work.go` (`StepRequest.RunID`: "the retrain before it in the same plan has just written that pointer" — a retrain publishes nothing) | It is the publish step of a three-step pipeline silently not publishing, which no green step, plan or run-history row could show. Fatal to A-5: a cadence that never publishes lets the served ratings age until H-11 refuses predictions, whatever the rhythm. | **yes** | fixed — PR #246 (`fix/reload-publishes-the-new-run`) |
| B-3 | A-5 | A retrain at today's cutoff writes a manifest with `formats: []` and `metrics: {}`. Rows at or after the cutoff are the holdout, and there are none after today, so every format reports `n_holdout: 0` and `manifest_metrics` skips a report entry with no `objective` block. The run is fine — all four formats' models are written and served — but its manifest cannot answer "is this run usable?", and the Workbench's run summary renders blank for every scheduled run. Measured on run `20260903T160602Z-0e1e39c2`: 21,096 rows, four formats trained, `formats: []`. | `ml-service/ml/xi/retrain.py`, `manifest_metrics` (skips `if "objective" not in report`); the cutoff comes from `go-app/internal/services/pipeline.DefaultCutoff` (today UTC), which a run plan cannot override | Not wrong, but weak evidence where the plan says evidence should be (H-16). Training on everything is what a refresh should do, so the fix is not a different cutoff: it is for the manifest to say *why* it has no holdout metrics rather than to omit the formats it trained. | no | open |
| B-4 | X-1a | Five `internal/db` integration tests failed against a real database with `ERROR: null value in column "match_id" of relation "match" violates not-null constraint`. `insertAppearance` inserted into `match` without a `match_id` (and without `original_match_type`, also `NOT NULL`), and neither column has a default — `match_id` is assigned by the importer from the Cricsheet file, not by a sequence. The tests therefore cannot have passed since they were written. They are gated behind `RUN_DB_TESTS=1`, which `make check-all` does not set, so nothing had ever reported it. | `go-app/internal/db/repo_selection_integration_test.go`, `insertAppearance` (and the four cases through it, plus `TestPlayerStatusStore_PromotionAndDemotionAreOneStateChange_Integration`) | D-12's pool-window arithmetic and its ledger promotion/demotion path are the parts of that item with no working test against the database — the half most likely to be wrong in SQL rather than in Go. A gated test that has never run is a test that does not exist. | no | fixed — PR #255 (`fix/b-4-b-6-integration-tests`). The fixture now gives each match an explicit id and match type, the way the importer does. With the tests running, **the D-12 SQL they cover was found correct**: the window is closed at `since` and half-open at the cutoff, `last_played` is the latest appearance, and a promotion reaches `player.is_retired`, `player_status` and `player_status_event` together. Two of the tests were passing vacuously and were tightened — a `LastPlayed` assertion inside a `continue` loop that skipped an absent row, and a case named for the cutoff boundary that had no appearance on the cutoff day; a fourth fixture player now supplies one. **43 database integration tests run**, and `.github/workflows/go-app-ci.yml` now runs them on every PR (`make -C go-app test-db`) |
| B-5 | X-1a | A `TRUNCATE` in an integration fixture must name *every* table with a foreign key into one it truncates, or Postgres refuses the whole statement. `player_status` and `player_status_event` (D-12) were not added to the identity fixture's list, so all 11 `repo_identity_integration_test.go` cases failed with `cannot truncate a table referenced in a foreign key constraint` — again invisibly, behind `RUN_DB_TESTS=1`. Fixed in this branch, along with adding `player_biography` to both fixtures, since X-1a's own table would have compounded it. | `go-app/internal/db/repo_identity_integration_test.go` and `repo_selection_integration_test.go`, the `TRUNCATE` lists | It is the failure mode a new per-player table causes every time, and the reason it went unnoticed is B-4's reason: the suite is not in `make check-all`. Worth deciding whether it should be. | no | fixed in X-1a's branch. The open question it left — whether the suite belongs in a check that runs — is answered by B-4: it runs in CI now, so a missing table in a `TRUNCATE` list fails a PR instead of nothing |
| B-6 | the licence register (`docs/free-data-prototype-scope`) | The package comment in `go-app/internal/biography/biography.go` said Cricsheet "publishes the register under ODbL". The register page at `https://cricsheet.org/register/` says **ODC-By 1.0** (Open Data Commons Attribution), read 2026-09-04 — attribution only, no share-alike term. The value stored per row (`source_license = CC0-1.0`) is Wikidata's and is correct; only the comment about the join key's source was wrong. | `go-app/internal/biography/biography.go`, the package comment | A licence stated wrongly in code is the kind of claim `docs/config-and-data.md` § Data-source licence register exists to stop: ODbL would oblige share-alike on derived databases, ODC-By does not. Nothing behaves differently today; the record should not disagree with itself. | no | fixed — PR #255. The comment now states ODC-By 1.0, says what it does and does not oblige, and points at the register as the record it must agree with; the register's row no longer reports the discrepancy |
| B-7 | X-3 | The **display** model's swap-violation share has never been measured, and it is **3–7 %**: 0.048 (T20), 0.068 (T20I), 0.031 (ODI), 0.062 (TEST) over the eleven walk-forward folds, on the same one-player upgrade H-4 uses. H-4's 2 % line is checked on the *objective*, whose surface is monotone-constrained on every stem it reads; the display model reads team context as well and its `monotonic_cst` gives those columns 0, so nothing constrains them. Found because X-3's stakes gate wrote the 2 % line into its own triple and then watched the control fail it. | `ml-service/ml/xi/train.py`, `make_display_model` / `contract.monotone_directions`; the probe is `scripts/experiments/xi/x3_match_stakes.py`, `_display_swap_probe` | Nothing selects on the display model — the optimiser reads the objective, which is inside H-4 — so no shipped behaviour is wrong. But the display model is the number a *user* sees change when they swap a player in the Team Lab, and one swap in twenty moves it the wrong way. Either that is acceptable and should be said out loud in the harness, or the constraint set should cover the context columns whose direction is knowable (`team_elo_diff`, `team_form_diff`, `venue_fam_diff`). It is currently neither measured nor stated. | no | **measured and stated** — see § B-7 below. The harness now reports `display_swap_violation_share` per format and `display_swap_monotonicity` per fold (`ml/xi/selection_metrics.display_swap_monotonicity`, one probe shared with `x3_match_stakes.py`), with a glossary entry saying H-4's 2 % line is the objective's contract and not this surface's; the Evaluation tab and the system map carry it. Measured **5.1 % T20, 7.1 % T20I, 3.4 % ODI, 6.1 % TEST** against the database, and no prediction moved (checked against a `main` run on the same database: every measured number bit-identical). **Gate `B-7-display-monotone` is a null and then some:** constraining the three context columns moves the violations the *wrong* way (T20 +0.0073 ± 0.0039, T20I **+0.0638 ± 0.0078**, ODI +0.0171 ± 0.0062, TEST −0.0007 ± 0.0042) at no AUC gain, so nothing shipped. It cannot have worked, and the diagnosis above is corrected: the probe holds team context at the fixture's values, so those columns never move under it. What does move, on **100 %** of upgrades in every format, is `t1_pelo_std` — the only column of `DISPLAY_FEATURE_COLS` an upgrade touches that the contract leaves at 0 — while no constrained column ever moves against its direction. Gate `B-7-pelo-spread` (informs, ships nothing) priced the fix that follows: dropping `t1_pelo_std` / `t2_pelo_std` from the display columns takes the share to **exactly 0.0000 in all four formats** for −0.0004 / +0.0088 / −0.0055 / −0.0047 of AUC. **That trade was taken by decision (PR #267) and B-7 is closed**: the display model no longer reads the Elo spread, `make evaluate` reports `display_swap_violation_share` 0.0000 in every format, and **display AUC was spent to buy it — the gate priced the loss at −0.0055 ODI / −0.0047 TEST and the confirming database run reads −0.0022 ODI / −0.0059 TEST: real losses either way**. See § B-7 part 3 |
| B-8 | P1-2 | The Lab's rating order and the display model disagree about who is better, and Play mode invites a user to act on the first. Swapping an eleven's player for a pool player the rating order ranks *higher* — `select_xi_by_ratings`'s composite of decayed batting and bowling impact plus Elo, which is the order the Lab serves as the eleven in T20 and TEST — **lowers** the displayed probability in 1 of 8 ODI cases, 4 of 8 T20 and 6 of 8 TEST (worst **−0.0805**, TEST), measured through the real stack on the served run. A swap for a player who is at least as good on *every* as-of vector never lowers it (0 falls in 20, all four formats), so B-7's guarantee is intact; what is not true is the stronger thing a user will read into the surface. | `ml-service/ml/xi/optimizer.py` `_greedy_seed`'s ordering against `ml/xi/train.py`'s `DISPLAY_FEATURE_COLS`; the probe is `scripts/probes/p1_2_play_mode.py` (its diagnostic arm) | Play mode's core interaction is "swap a better player in and watch the number move", and the Lab offers no ranking of its own — so the ranking a user brings is either their own judgement or the rating-ordered eleven the Lab itself shows. Neither is the display model's preference. The honest fixes are a *shown* ordering the surface can stand behind (the objective's marginal value already ranks an eleven's players and is on the wire where a format is optimised) or a sentence saying which ordering the guarantee holds under; both are P1-3 / P1-4 work, not P1-2's. | no | **half fixed — P1-3 (2026-09-06); the other half is P1-4.** The "why this player" card's rating-ordered state (T20, TEST) now *says* which ordering it is showing: "picked by rating, not by the win model", the selection rating with its pool percentile, and one sentence — the ordering on screen is the selection's own composite and is not the win model's ranking, so swapping in a higher-rated player can move the displayed probability down. The glossary's `selection_rating` entry carries the same statement and points here. That closes the *shown ordering the surface can stand behind* half for the formats where the surface offers rating order at all, and it changes no model and no selection policy — the measurement above is untouched. What is still open: on a searched format the Lab now has an ordering it can stand behind (the marginal value, on the wire and on the card) but does not yet rank the board by it, and the Play-mode surface itself carries no sentence about which ordering the swap guarantee holds under. Both are P1-4's sweep. **Closed on the surface — P1-4 (2026-09-06).** A searched board is ordered by the marginal value the response already carries and says so ("the objective's own ranking of this eleven, which is what a swap here is measured against"); the rating-ordered board says it is in rating order and not the win model's ranking; the hand-built board says nothing ranks it; and Play mode carries the sentence about which ordering the swap guarantee holds under — a player at least as good on every rated axis, not one the rating order merely ranks higher. The measurement above is untouched: no model and no selection policy changed, and a swap on rating order in T20 and TEST can still lower the number — the surface now says so before it is made. |
| B-9 | P1-3 | go-app's coverage total is **machine-dependent by one statement**, and it sits on the ratchet's rounding boundary. `TestFetch_StopsWhenTheJobContextIsCancelled` cancels the job 50 ms into a transfer the test server dribbles out 4 KB at a time, and asserts only that `Fetch` errors — which it does either way. Whether the error comes from `ctxReader.Read` noticing the cancelled context (covering its `return 0, err`) or from the HTTP transport failing the underlying read first is a race. On this box `ctxReader.Read` is **100.0 %** on five runs out of five; on the CI runner it is **66.7 %**. That one statement moves the go-app total across 76.95, so the same commit measures **77.0 % locally and 76.9 % in CI** — verified by diffing the CI run's own `coverage.out` artifact against a local profile, which differ in exactly this one function and nothing else. | `go-app/internal/services/dataacquire/fetch_test.go`, `TestFetch_StopsWhenTheJobContextIsCancelled`, against `fetch.go`'s `ctxReader.Read` | It cost P1-3 a red check and would cost the next person the same: the ratchet rule says raise the threshold to the measured figure rounded down, and the figure a worker measures locally is not the one the gate applies. It is also a test that does not test what it is named for — it passes whether or not the cancellation reaches `ctxReader`, so the mechanism it exists to protect is unasserted on the machine where it does not fire. The fix is to make the test deterministic (drive `ctxReader.Read` directly with an already-cancelled context, and assert `Fetch` returns `context.Canceled` rather than merely erroring), not to special-case the threshold. | no | **fixed — the mechanism is asserted deterministically.** `ctxReader.Read` is now driven directly by two tests: an already-cancelled context must return `context.Canceled` **and must not read the wrapped reader** (a reader that fails the test if it is read at all), and a live context must pass straight through to it. Neither depends on a race, a sleep or a machine's speed, so the branch is covered on any runner rather than on a fast one. The end-to-end test is kept and renamed for what it actually guarantees — `TestFetch_CancellingTheJobFailsTheTransfer`, now asserting `errors.Is(err, context.Canceled)` rather than merely that something failed; it deliberately does *not* assert which of `ctxReader` and the transport noticed first, because that is the race, and the two tests above are what cover the reader. Stressed 20 runs at `-cpu=1 -race` locally, all green. **The gate moved on the CI run's own figure, not a local one** — which is the mistake this row records: PR #274's Go App CI job read **`Coverage 77.0% meets threshold (>= 76%)`**, up from 76.9 on the same runner before the fix, so `COV_MIN` went 76 → 77 in all three places (`go-app/Makefile`, root `Makefile` `COV_MIN_GO`, `.github/workflows/go-app-ci.yml`) in a follow-up commit on that PR. The workflow comment saying 77 was out of reach is replaced by what actually happened. |
| B-10 | P1-4 | **Must-include is not enforced on the Optimise path.** `go-app/internal/server/ml_xi_client.go` `OptimizeXI` sends `MustInclude: []string{}` whatever the request carried, so a must-include id joins its side's *pool* (P1-1) and the search — or the rating order — may leave him out. Measured on the dev stack (T20I India men, `extra_team1: [11877, 395]`): Ashok Sharma left out, Bumrah in, both silently before P1-4. | `go-app/internal/server/ml_xi_client.go` `OptimizeXI` (the empty `MustInclude`); `go-app/internal/services/predictteam/xi_selection.go` `optimizeSide`; ml-service's `Constraints.must_include` in `ml/xi/optimizer.py` already honours a lock | A user who ticks "must include" reasonably expects the pick to be forced; a label that says "must" over a constraint nothing applies is the D-9 shape with a person in the middle. P1-4 made the label true instead — "added to the pool, checked after" — and put the check on the wire: `selection.must_include` names, per side, how many were asked for and each one the selection left out, so the outcome is on screen in both cases. **The real fix is to pass the resolved registry ids to `/xi/optimize` as `must_include`**, which changes the search: the seed must hold them, every neighbour must keep them, and the marginal values and best alternatives are then conditional on the lock. That is a gated change to the selection (its E5 agreement and the swap-monotonicity check would have to be re-run under the lock), not a product item, and it is not taken here. | no | **fixed — the lock is enforced (2026-09-06).** `OptimizeXI` sends the request's resolved registry ids as `must_include`; go-app resolves them once, on the fixture, so the searched path and Play mode's check read one list. **The default is unchanged and was measured, not argued**: with no must-include the same fixture returns the identical eleven and the identical probability before and after (see the PR). **The gated worry above did not arise**: no measured number moved, because every harness, gate and evaluation call sends an empty `must_include` — the lock only narrows the feasible set of a request that asks for one, and E5 agreement and the swap-monotonicity check never ask. **All three conflicts are on the wire with their reason, never silently resolved (§8.7):** a must-include id this side cannot field (no player row, or no `external_id`) is `400 MUST_INCLUDE_UNRESOLVABLE` from go-app, which is the only side that can name him; more required players than places, a lock that leaves no room for the keeper or the minimum bowlers, and an id the pool does not hold are `422 OPTIMIZATION_CONSTRAINT_ERROR` from ml-service, whose message now names *which* constraint failed instead of "size / bowlers / keeper" (`ml.xi.optimizer.ConstraintConflict`). P1-4's `selection.must_include` is kept and re-read as the postcondition: an ordinary answer holds every id, and a named "left out" player is a lock the stack accepted and did not honour — logged at ERROR and shown as a defect, not as a choice. The Lab's input label is corrected to "required in the eleven", and § 4's must-include omission keeps its place with its reason rewritten: a lock is the caller's input echoed back, not something the selection read about the player |

---

## B-7 — the display surface: measured, and what the gate found

Two separable pieces, in this order, because the first is worth having whatever the second
says.

### Part 1 — measure it and say so (no prediction changed)

`ml/xi/selection_metrics.display_swap_monotonicity` runs H-4's probe against the display
model: one player's five ratings raised by a population standard deviation each, the other
ten and the opponent held, **the match's team context held exactly as the fixture had it**
because a selector cannot change it, and the probability marginalised over the batting
order the way the serving path marginalises it. `make evaluate` reports it per fold
(`display_swap_monotonicity`) and as a walk-forward mean per format
(`display_swap_violation_share`); the glossary explains it and states in its band that
H-4's 2 % line is a contract on the *objective*, whose every column is
monotone-constrained, and not on this surface. The Evaluation tab shows it beside H-4's
tile, without a gate triple, because nothing decides on it. X-3's script no longer keeps
its own copy of the probe — there is one implementation, and the harness and the
experiment share it.

**What it reads.** Against the database: **5.1 % T20, 7.1 % T20I, 3.4 % ODI, 6.1 % TEST**
over the folds, beside H-4's own 0.2 / 0.8 / 0.0 / 0.5 % on the objective. Against the
archive frame the same probe reads 4.8 / 6.8 / 3.1 / 6.2 — the ground-keying difference
between the two sources that X-3 recorded, and the reason the gate below quotes the
archive figures as its control.

**It moves no number, and that is checked rather than argued.** `make evaluate` was run
twice against the same database — once from this branch and once from `main` at
`e529449c` — and the two reports were compared field by field at **exact equality**. Of
126 differences, 124 are `performance.fit.fit_seconds` and
`simulation.latency.ms_per_fixture_*` (wall-clock, and declared non-metrics in the
glossary for that reason) and 2 are the gate-registry entries part 2 adds. **Every
measured number is bit-identical**: objective AUC 0.6973 / 0.7561 / 0.6725 / 0.6259,
display AUC 0.7299 / 0.7533 / 0.7067 / 0.6456, H-4's own swap share 0.0023 / 0.0075 /
0.0000 / 0.0052, every performance target, every simulation figure, specific-vs-typical
and E5, with H-8 serving parity max abs difference **0.0** on both runs. Nothing is fitted
differently; a measurement was added after the models were fitted. (The comparison had to
be against a fresh `main` run: the last recorded run predates X-3's re-import, and the
extra `stage`/`group` columns move the performance model's early-stopping iteration counts
even though nothing about the harness changed.)

### Part 2 — gate `B-7-display-monotone`: a recorded null, and a corrected diagnosis

Eleven quarterly folds, three display seeds, one frame both arms read, everything but the
constraint held (the triple is in `ml/xi/gates.py` and is printed before the run;
`scripts/experiments/xi/b7_display_monotonicity.py`). The control reproduces X-3's recorded
figures exactly, which is the wiring check.

| format | swap (control) | swap (constrained) | Δ ± se | material floor | display AUC (control) | Δ AUC ± se | verdict |
|---|---:|---:|---|---:|---:|---|---|
| T20 | 0.0478 | 0.0551 | +0.0073 ± 0.0039 | −0.0069 | 0.7299 | −0.0003 ± 0.0008 | no |
| T20I | 0.0684 | 0.1322 | **+0.0638 ± 0.0078** | −0.0121 | 0.7495 | −0.0006 ± 0.0063 | no |
| ODI | 0.0307 | 0.0478 | +0.0171 ± 0.0062 | −0.0027 | 0.7083 | +0.0009 ± 0.0017 | no |
| TEST | 0.0616 | 0.0609 | −0.0007 ± 0.0042 | −0.0104 | 0.6431 | −0.0018 ± 0.0051 | no |

Not a null in the flat sense: three of four formats get *worse*, T20I by eight standard
errors, and the fourth moves by a sixth of one. AUC is unchanged everywhere. Nothing
shipped; `DISPLAY_CONTEXT_MONOTONE_KEPT` stays `False`.

**Why it could not have worked, measured two ways.** The recorded diagnosis — the
context columns' `monotonic_cst` is 0, so nothing constrains them — describes the model
correctly and the *probe* not at all. The probe holds team context at the fixture's values,
so those columns do not move under it, and a constraint on a column that does not move
cannot change the count. Two diagnostics beside the arms say so directly:

| format | swap, boosting with **no team context at all** | swap, control (with it) | share of upgrades moving a *constrained* column the wrong way | share moving an *unconstrained* column |
|---|---:|---:|---:|---:|
| T20 | 0.0504 | 0.0478 | 0.000 | **1.000** |
| T20I | 0.1364 | 0.0684 | 0.000 | **1.000** |
| ODI | 0.0352 | 0.0307 | 0.000 | **1.000** |
| TEST | 0.0568 | 0.0616 | 0.000 | **1.000** |

A display model that never sees team context violates as much or more. In feature space,
with no model involved, **no constrained column ever moves against its direction**, and the
one unconstrained column an upgrade moves — on every single upgrade, in every format — is
`t1_pelo_std`, the spread of player Elo across the eleven. Its direction is 0 because it is
genuinely unknown: raising a player above the side's mean widens the spread, raising one
below it narrows the spread, so no sign is defensible. The objective survives the same
column because it is linear and its coefficient there is small; a boosted tree responds in
steps, and some of those steps go down. Constraining team context makes matters worse
presumably because it removes fitting capacity that then routes through the free column —
an explanation the numbers are consistent with and this run did not separately test.

### `B-7-pelo-spread` — the fix the mechanism points at, priced and not taken

Registered as an **informing** gate (`ml/xi/gates.py`), because B-7 scoped a constraint on
team context, not a change to what the display model reads, and dropping a column from
`DISPLAY_FEATURE_COLS` changes a served artifact's feature list. Same folds, same seeds,
same everything else:

| format | swap (control) | swap without `pelo_std` | Δ ± se | display AUC (control) | Δ AUC ± se |
|---|---:|---:|---|---:|---|
| T20 | 0.0478 | **0.0000** | −0.0478 ± 0.0082 | 0.7299 | −0.0004 ± 0.0015 |
| T20I | 0.0684 | **0.0000** | −0.0684 ± 0.0076 | 0.7495 | **+0.0088 ± 0.0062** |
| ODI | 0.0307 | **0.0000** | −0.0307 ± 0.0029 | 0.7083 | −0.0055 ± 0.0039 |
| TEST | 0.0616 | **0.0000** | −0.0616 ± 0.0098 | 0.6431 | −0.0047 ± 0.0052 |

Exactly zero violations in every format and every fold — which is the confirmation that the
mechanism is fully identified, not merely correlated with. **The decision is not this
gate's to make**, and is recorded here so it can be made deliberately:

- **Take it.** The Team Lab's what-if becomes coherent: every upgrade raises the displayed
  probability, in every format. It costs about half a point of AUC in ODI and TEST (−0.0055
  and −0.0047, 1.4 and 0.9 fold-level standard errors — real but not resolved), nothing in
  T20, and *gains* 0.0088 in T20I. It is a change to `DISPLAY_FEATURE_COLS`, so it needs a
  retrain, and H-8 parity and the stored `display_cols` move with it.
- **Leave it.** Nothing selects on this model, the AUC is the number the display model
  exists for, and one swap in twenty going the wrong way is now stated in the harness, the
  glossary, the Evaluation tab and the system map rather than being invisible.

Either way the measurement from part 1 stays.

### Part 3 — the decision: taken, and what it cost

**Taken.** The user's decision, in their words: *"take the pelo_std change, its worth it for
the surface."* The reasoning is a product one, not a statistical one.
[PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md) § 3 sells interactive what-if — swap a player,
watch the probability move — as the core of the thing, and Phase 1's Team Lab is that
surface. A swap that moves the probability the wrong way one time in twenty undermines the
whole proposition, and half a point of AUC in two formats is a price worth paying for a
surface that is coherent by construction rather than on average.

**What shipped.** `t1_pelo_std` / `t2_pelo_std` are out of `DISPLAY_FEATURE_COLS`, named in
`ml.xi.contract.DISPLAY_EXCLUDED_COLS` with the reason. The **objective is untouched** and
still reads them: it is linear, its coefficient there is small, and H-4 measures 0.0–0.8 %
on it. Because a served artifact's feature list moved, `XiStore.load` now refuses a win
artifact whose `objective_cols` or `display_cols` are not the contract's, naming the run and
saying "Retrain" — the D-6 policy, which the ratings had and the models did not: the serving
path builds its row from the artifact's *own* column list, so an older run would have loaded
without complaint and served exactly the surface this change removes.

**Confirmed against the database, not asserted** (`make evaluate`, 2026-09-06, Postgres
source, 22,818 matches, eleven quarterly folds):

| format | display swap-violation share | folds at zero | display AUC, before → after | Δ |
|---|---:|---:|---:|---:|
| T20 | **0.0000** | 11 / 11 | 0.7299 → 0.7296 | −0.0003 |
| T20I | **0.0000** | 10 / 10 | 0.7533 → 0.7540 | **+0.0007** |
| ODI | **0.0000** | 11 / 11 | 0.7067 → 0.7045 | **−0.0022** |
| TEST | **0.0000** | 11 / 11 | 0.6456 → 0.6397 | **−0.0059** |

Zero violations of 6,107 upgrades in T20, 4,601 in T20I, 5,667 in ODI and 4,835 in TEST —
every fold, not a mean that rounds to zero. **H-8 serving parity is 0.0** (50 matches, 1,100
player rows, 1,100 performance predictions, 50 simulations, `passed`), re-established after
the feature list moved.

**The rest of the report is the control, and it did not move.** Objective AUC 0.6973 /
0.7561 / 0.6725 / 0.6259 and H-4's own swap share 0.0023 / 0.0075 / 0.0000 / 0.0052 are
**bit-identical** to the `main` run part 1 recorded on this database, which is what makes
the display AUC column above attributable to the dropped column and nothing else. E2 still
serves the simulated probability as a probability in every format it runs (Δ Brier +0.0015
T20, +0.0007 T20I, +0.0080 ODI, tolerance 0.01), and E5's verdicts are unchanged.

**The cost, stated plainly.** This was a trade, not a free win. Display AUC fell in three of
four formats, and in TEST by 0.0059 — more than the informing gate's paired archive-frame
reading of −0.0047, and larger than the T20I gain. The gate's figures and this run's differ
in magnitude because the gate paired three seeds per fold on the archive frame while this is
one database run against another (the ground-keying difference X-3 recorded), but the signs
agree in every format and every difference sits inside the gate's own fold-level standard
errors (±0.0015 / ±0.0062 / ±0.0039 / ±0.0052). Nobody reading this later should record it
as an improvement: **the displayed probability discriminates slightly worse, and it was
spent on the Team Lab's coherence, deliberately.**

**Status: B-7 is closed.** The measurement from part 1 stays and now reads zero; the
glossary band, the Evaluation tab's tile and H-4's row in the plan say what the zero cost.

---

## Running the database suites

The 43 `_Integration` tests across `internal/db`, `runplan` and `datasetregistry` are the
only checks that run this project's SQL against Postgres rather than against a mock. Every
one of their fixtures begins with a `TRUNCATE`, so they run against a scratch database:

```sh
createdb -h localhost -U postgres cricket_flow_test   # once
make -C go-app test-db                                 # POSTGRES_DB=cricket_flow_test
```

`dbtest.SkipUnlessScratchDatabase` refuses to run them when `POSTGRES_DB` is unset or names
the working database, because the default connection points at the imported archive and the
first fixture would delete it. `.github/workflows/go-app-ci.yml` runs them on every PR
against its own `postgres:15-alpine` service, which is why B-4's class of defect — a gated
test nothing gates on — cannot recur silently.
