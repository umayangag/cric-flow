# Bug backlog

Bugs found while working a planned item — `docs/FOLLOW_UP_PLAN.md`,
`docs/EXTERNAL_DATA_PLAN.md` — that are outside that item's scope. A non-blocking bug is
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
| B-7 | X-3 | The **display** model's swap-violation share has never been measured, and it is **3–7 %**: 0.048 (T20), 0.068 (T20I), 0.031 (ODI), 0.062 (TEST) over the eleven walk-forward folds, on the same one-player upgrade H-4 uses. H-4's 2 % line is checked on the *objective*, whose surface is monotone-constrained on every stem it reads; the display model reads team context as well and its `monotonic_cst` gives those columns 0, so nothing constrains them. Found because X-3's stakes gate wrote the 2 % line into its own triple and then watched the control fail it. | `ml-service/ml/xi/train.py`, `make_display_model` / `contract.monotone_directions`; the probe is `scripts/experiments/xi/x3_match_stakes.py`, `_display_swap_probe` | Nothing selects on the display model — the optimiser reads the objective, which is inside H-4 — so no shipped behaviour is wrong. But the display model is the number a *user* sees change when they swap a player in the Team Lab, and one swap in twenty moves it the wrong way. Either that is acceptable and should be said out loud in the harness, or the constraint set should cover the context columns whose direction is knowable (`team_elo_diff`, `team_form_diff`, `venue_fam_diff`). It is currently neither measured nor stated. | no | **measured and stated** — see § B-7 below. The harness now reports `display_swap_violation_share` per format and `display_swap_monotonicity` per fold (`ml/xi/selection_metrics.display_swap_monotonicity`, one probe shared with `x3_match_stakes.py`), with a glossary entry saying H-4's 2 % line is the objective's contract and not this surface's; the Evaluation tab and the system map carry it. Measured **5.1 % T20, 7.1 % T20I, 3.4 % ODI, 6.1 % TEST** against the database. No prediction moved. Whether the surface can be made monotone, and at what price, is gated next |

---

## B-7 — the display surface, measured

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
