# Codebase audit — bugs, leaks and accuracy limiters

**Audited at:** commit `e91c207f` (`main`, 2026-09-08).
**Method:** line-by-line review of `go-app/`, `ml-service/`, `reference-data/`, `scripts/`, config and CI by six focused reviewers (rating pass / leakage; training and evaluation; simulator and serving; Cricsheet import and schema; go-app prediction path; weather, reference data and ops), followed by a second pass that re-verified every Critical and High finding against the source. Line numbers refer to that commit and will drift — search for the quoted code, not the line.

**How to use this document.** Each finding has a stable id (`AREA-nn`), a severity, the files involved, the failure it causes, and a fix with a suggested test. Pick them up in the order of § 1 (the ranked list) unless a dependency note says otherwise. When a finding is fixed, move it to § 9 with the PR number rather than deleting it, so the next audit knows what was already covered. Findings that change a feature definition (marked **retrain**) invalidate every run on disk: after landing them, `make retrain` and `make reload` before believing any number.

Severity scale: **Critical** — a leak or wrong answer on the live path today; **High** — a wrong number that reaches a served or reported result, or a data defect that biases the models; **Medium** — a real defect with a narrower or latent blast radius; **Low** — correctness or hygiene, worth fixing when in the file.

---

## 1. Ranked fix order

| # | Id | Sev | One line | Retrain? |
|---|----|-----|----------|----------|
| 1 | GO-01 | Critical | `as_of` is never sent to ml-service; a past `match_date` is answered from through-today ratings that already contain the result, and today's retirement ledger is applied to the past pool | no |
| 2 | IMPORT-01 | Critical | Super-over innings are imported as ordinary innings 3/4 and counted in every rating, target and context baseline | yes |
| 3 | FEAT-01 | High | `exp_balls_faced` / `exp_balls_bowled` divide by matches *batted/bowled*, not XI appearances — corrupts involvement weights and the bowling-option constraint | yes |
| 4 | EVAL-01 | High | `HistGradientBoostingClassifier` early stopping is `'auto'`: silently on for T20 (>10k rows) with a random split, off elsewhere; the grid scores a different model from the one served | yes |
| 5 | EVAL-02 | High | Three "seeds" produce bit-identical display models below 10k rows; gate noise floors built on that sd are zero | no |
| 6 | EVAL-03 | High | The served performance model never trains on the last 92 days before the cutoff (calibration fold held out permanently) | yes |
| 7 | EVAL-04 | High | No gate can fail in code — `check_report` only checks that a report path exists; a regressed model ships on cadence | no |
| 8 | FEAT-14 | High | The objective is an unconstrained logistic regression — its fitted own-side sign on `pelo_mean` is negative in T20I, H-4 held only because FEAT-01's broken denominator inflated every upgrade's rate terms, and it fails in T20I (0.0230) once involvement is honest | yes |
| 9 | GO-02 | High | Track record joins fixtures and winners in raw `opposition.id` space while the record stores canonical club ids — renamed clubs never resolve or score the wrong way | no |
| 10 | GO-03 | High | Forecasts issued after the match date are scored into Brier/coverage — with GO-01 those are leaked forecasts | no |
| 11 | IMPORT-02 | High | `outcome.result` / `eliminator` / `method` not decoded: super-over winners stored as "no winner", ties indistinguishable from no-results, drawn Tests get no form update on the Postgres path | yes |
| 12 | IMPORT-03 | High | Re-import is not idempotent for `ball_event` (`ON CONFLICT DO NOTHING`): corrected Cricsheet files and importer fixes never reach already-imported matches | no (but re-import) |
| 13 | IMPORT-04 | High | Byes, leg-byes and penalty runs are charged to the bowler; `ball_event` cannot carry the breakdown | yes |
| 14 | SERVE-01 | High | Every route is `async def` running CPU-bound work inline; the event loop (and `/health`) blocks for the whole optimise/simulate/as-of sweep | no |
| 15 | SERVE-02 | High | Request models accept sides of any size, duplicate ids and the same player on both sides; downstream silently assumes 11 distinct | no |
| 16 | DATA-01 | High | Geocoder picks a candidate in *any* voted country: Chinnaswamy → Pakistan, Shere Bangla → Punjab IN, Warner Park → South Australia, Providence → Rhode Island (~590 venue-days of wrong weather) | no (experiments only) |
| 17 | FEAT-15 | Medium | `MIN_BOWLING_BALLS` is documented per match and was calibrated on the pre-FEAT-01 denominator; read per XI appearance the same number is a far higher bar, and a fifth of T20I sides and a quarter of T20 sides now read as short of five bowling options | yes |
| 18 | GO-04 | Medium | The same player can be in both pools and both XIs; merges keyed by registry id clobber one side | no |
| 19 | IMPORT-05 | Medium | No-balls are excluded from the batter's balls faced (`batting_data`), and the ML path counts wides as faced — two definitions | yes |
| 20 | IMPORT-06 | Medium | Every wicket kind credited to the bowler in `bowling_data`; retired-hurt counted as a wicket lost; only first wicket per ball reaches `ball_event` | partial |
| 21 | FEAT-02 | Medium | Sides of 12–13 (concussion subs) are rated and aggregated as full squads — train/serve mismatch on ~3 % of rows and a mild post-start leak | yes |
| 22 | FEAT-03 | Medium | A decided match with no `ball_event` rows emits 22 player rows with all-zero targets | yes |
| 23 | EVAL-05 | Medium | Manifest `objective_auc` is toss-known; harness's is marginalised — same glossary key, different quantity | no |
| 24 | EVAL-06 | Medium | Harness never runs the grid; walk-forward numbers describe grid point 0 only | no |
| 25 | EVAL-07 | Medium | Served display artifact is seed 0; headline is the seed mean | no |
| 26 | EVAL-08 | Medium | Latent crash in locked-window recalibration when the 92-day fold is thin; duplicate knots in `np.interp` | no |
| 27 | EVAL-09 | Medium | Performance-model early stopping uses a random split that puts rows of one match on both sides | yes |
| 28 | EVAL-10 | Medium | H-8 parity compares `rows.py` with `rows.py`; the saved artifact and `XiStore` serving path are outside it | no |
| 29 | EVAL-11 | Medium | Locked window is days old and every gate was decided on the same eleven folds — no untouched holdout | no |
| 30 | EVAL-12 | Medium | Manifest cannot reproduce a run: `git_sha` empty in the container, no library versions, `dataset_sha` blind to squad/delivery edits | no |
| 31 | SERVE-03 | Medium | `RATINGS_STALE` measures last *match* date, not data currency; refuses fresh runs in an off-season, global not per format | no |
| 32 | SERVE-04 | Medium | Serving fixtures are stamped with `state.last_date`, not the fixture date; no `match_date`/`gender` in the request | no |
| 33 | SERVE-05 | Medium | Simulator batting depth uses one shared uniform vs per-player `p_bats`; realised P(bats) contradicts L2-B when `p_bats` is not monotone in order | no |
| 34 | SERVE-06 | Medium | Training process registry keyed by module — concurrent runs overwrite each other's handle; stop races natural exit | no |
| 35 | GO-05 | Medium | Cancelled run plan can never record its outcome (cancelled ctx used for `Finish`) — row stuck `IN_PROGRESS`, every later plan 409 | no |
| 36 | GO-06 | Medium | Lane lock is check-then-insert; a second job in the same lane overwrites the first's cancel func | no |
| 37 | GO-07 | Medium | `/xi/predict-win` and `/performance/predict` are never told the toss (Go structs omit `team1_bats_first`) | no |
| 38 | GO-08 | Medium | Venue lookup failure is swallowed; prediction silently runs with no venue | no |
| 39 | GO-09 | Medium | `match_date` with a UTC offset is truncated in UTC; pool cutoff and stored date disagree and can include the fixture itself | no |
| 40 | IMPORT-07 | Medium | Fail-fast abort skips display-name settlement and team lineage — renamed clubs stay split | no |
| 41 | IMPORT-08 | Medium | Venue identity is the raw string; `normalized_name` never populated; city used as venue when blank | yes |
| 42 | IMPORT-09 | Medium | Format taxonomy merges MDM with TEST, ODM with ODI, men with women; T20I inferred from a hand list instead of `info.team_type` | yes (design) |
| 43 | IMPORT-10 | Medium | `-apply` (dry-run) flag is parsed and ignored; every run writes | no |
| 44 | IMPORT-11 | Medium | `match_inning.target_runs` ignores `innings[].target` (D/L) and is set on Test second innings | no |
| 45 | FEAT-04 | Medium | Postgres source hard-codes `result=None`; JSON path reads it — `team_form_diff` has two definitions | yes (with IMPORT-02) |
| 46 | FEAT-05 | Medium | No home-advantage and no toss feature although `venue.country` and `toss_*` are in the DB | yes |
| 47 | FEAT-06 | Medium | Ratings siloed per format; every T20⇄T20I / ODI⇄List-A crossover is a cold start shrunk toward zero | yes |
| 48 | DATA-02 | Medium | Venue-key normalisation does not merge spellings of one ground; 20 rows are country centroids | no |
| 49 | DATA-03 | Medium | Competition needle `"zimbabwe"` votes ZW for every venue Zimbabwe toured; other needles unanchored | no |
| 50 | DATA-04 | Medium | ERA5 hours indexed by local-time string with one fixed offset per call across DST transitions | no |
| 51 | DATA-05 | Medium | `wx_rain_prior_day_mm` sums match-day rain up to the *inferred* start — in-match rain leaks when the norm is late | no |
| 52 | OPS-01 | Medium | `reference-data/**`, `configs/**`, `scripts/**`, `contracts/**` trigger no CI | no |
| 53 | OPS-02 | Medium | `dev-local-key` admin default baked into compose, three Makefiles and `cadence.sh`; Postgres and both APIs bound to all interfaces; watcher holds the Docker socket | no |
| 54–81 | various | Low | See § 2–8 | — |

### 1b. Which model does the fix

Three tiers, chosen by how much the fix depends on judgment about *what the right behaviour is* rather than on executing a spec. The rule: if getting it slightly wrong would silently change a served number or a training label, use Fable; if the spec in this doc is complete and the risk is engineering (races, SQL, plumbing), use Opus; if it is mechanical, use Sonnet. A fixer may escalate one tier if the file turns out harder than described, never de-escalate.

**Fable (`claude-fable-5-1`)** — ML semantics, leakage, feature definitions, anything marked **retrain**, and the two Critical leaks. The fixer has to read the rating pass or harness deeply, decide the definition, and prove it with a targeted test and a before/after on the harness.
`GO-01`, `IMPORT-01`, `IMPORT-02`, `IMPORT-04`, `IMPORT-05`, `IMPORT-06`, `IMPORT-09`, `FEAT-01` … `FEAT-15`, `EVAL-01`, `EVAL-02`, `EVAL-03`, `EVAL-04`, `EVAL-05`, `EVAL-06`, `EVAL-07`, `EVAL-09`, `EVAL-10`, `EVAL-11`, `EVAL-13`, `EVAL-14`, `SERVE-05`, `SERVE-10`, `SERVE-12`, `DATA-01`, `DATA-05`.

**Opus (`claude-opus-4-1` or the newest Opus)** — well-specified engineering with a non-trivial blast radius: transactions, concurrency, id-space folding, API plumbing, schema migrations without a semantic decision.
`GO-02`, `GO-03`, `GO-04`, `GO-05`, `GO-06`, `GO-07`, `GO-08`, `GO-09`, `IMPORT-03`, `IMPORT-07`, `IMPORT-08`, `IMPORT-11`, `IMPORT-12`, `IMPORT-13`, `SERVE-01`, `SERVE-02`, `SERVE-03`, `SERVE-04`, `SERVE-06`, `SERVE-07`, `SERVE-08`, `EVAL-08`, `EVAL-12`, `EVAL-16`, `DATA-02`, `DATA-03`, `DATA-04`, `DATA-06`, `OPS-01`, `OPS-02`.

**Sonnet (newest Sonnet)** — mechanical, single-file, spec is the whole fix.
`GO-10` … `GO-16`, `IMPORT-10`, `IMPORT-14` … `IMPORT-18`, `SERVE-09`, `SERVE-11`, `SERVE-13`, `EVAL-15`, `DATA-07`, `DATA-08`, `DATA-09`, `OPS-03`.

**Dependencies that fix the order regardless of rank.** `IMPORT-03` (idempotent re-import) should land *before* any other `IMPORT-*` change, otherwise those fixes never reach already-imported matches. `IMPORT-02` before `FEAT-04`. `IMPORT-04` before `FEAT-08`. `SERVE-08` together with `GO-01` (or `as_of` becomes a freshness bypass). `SERVE-02` together with `GO-04`. `IMPORT-08` and `DATA-02` should agree on one venue key. Batch all **retrain** findings you intend to land this cycle, then run `make retrain` and `make evaluate` once at the end rather than once per PR; the harness (~1 h) is the acceptance test for the batch.

The orchestration prompt that drives this list is in `docs/AUDIT_FIX_RUNBOOK.md`.

---

## 2. Rating pass and features (`ml-service/ml/xi/`)

### FEAT-05 — No home-advantage and no toss feature  **Medium · retrain**

`ratings.py:301-317` — the only venue-side context is `venue_fam_diff` (log1p of raw `(team, venue)` match counts). `venue.country` (`0001_baseline.sql:709`) and `toss_winner_opposition_id` / `toss_decision` (`0001_baseline.sql:482-483`) exist but `_MATCH_SQL` (`sources.py:423-438`) reads neither. Home advantage is the largest non-strength effect in international cricket; "chose to bat" vs "was made to bat" are different populations of batting-first sides.

**Fix.** Derive an as-of home flag (team country = venue country, team country inferred as-of from the modal venue country of past home fixtures or a reviewed config) and a `toss_won_by_team1` column; both are at-toss facts (H-21 compliant) and can be marginalised at serving the way batting order is.

### FEAT-06 — Ratings siloed per format; cold start on every crossover  **Medium · retrain**

`ratings.py:234-258` — every accumulator is `[f, s]`; a player with no history in the format reads the neutral vector (`bat_rate = 0`, `pelo = 1500`, counted as a debutant in `n_debutants`, `ratings.py:634`). A 100-cap T20I player making his IPL debut is an unknown. `PRIOR_BALLS=60` shrinks toward *zero* impact, not toward the player's own cross-format estimate.

**Fix.** Hierarchical shrinkage: shrink the format rate toward the pooled T20+T20I (or all-format) rate instead of zero. Only `side_vectors` changes.

### FEAT-07 — Batter's dismissal rate charges every wicket on the ball to the striker  **Low · retrain**

`ratings.py:446-447` uses `d.wicket` (any dismissal on the ball, `sources.py:44`); a non-striker run-out or "retired hurt" lowers the striker's `bat_wrate`, while the `dismissals` target (`rows.py:66-70`) uses `player_out`. Feature and target are defined on different events. **Fix.** Use `(d.player_out == d.batter)` as the batter's indicator.

### FEAT-09 — Multi-day matches fold at the close of their *start* date  **Low (narrow leak)**

`sources.py:287` (`dates[0]`), `builder.py:70-73`, `asof.py:66`. A Test running Jan 1–5 is in the state for any match dated Jan 2–5 — another TEST starting Jan 3 reads context baselines and `competition_scoring` containing all five days; other formats see only `career_all` and `team_venue_matches`. Narrow, but the `AsOfRatings` docstring ("nothing at `d` or after") is false for Tests. **Fix.** Buffer a match until the day-close of `dates[-1]` (persist `match_end_date`), keep `dates[0]` as the feature date.

### FEAT-10 — Keeper flag is global, permanent and stumping-only  **Low**

`ratings.py:469-472` sets `keeper[slot] = 1.0` (no format, no decay) on any stumping ever. A player who kept once in 2009 satisfies `require_keeper` in 2025; a real keeper whose side never recorded a stumping in the format reads 0. **Fix.** A decayed per-format "stumpings + catches-as-keeper share" from `fielding_event`.

### FEAT-11 — Postgres source counts no-innings matches as `out_of_scope`; JSON path counts them `unusable`  **Low**

`sources.py:432` inner-joins `inning_number = 1`; `sources.py:537` derives `out_of_scope = offered - len(matches)`. The two sources' split (compared by `make xi-parity`) disagrees on the first toss-then-abandoned file. **Fix.** Filter by format only in SQL; detect the missing first innings in Python.

### FEAT-12 — Decay and shrinkage are untuned constants  **Low · retrain**

`contract.py:34-35`: `DECAY_PER_MATCH = 0.90`, `PRIOR_BALLS = 60`. Effective sample ≈ 10 innings ≈ 200 T20 balls for a top-order batter; 0.90 per Test forgets a year in 10 matches. Nothing sweeps these. **Fix.** Per-format decay and a ball-count half-life; evaluate on the existing harness.

### FEAT-13 — Gender split at serving would diverge if `gender_split_context` is ever on  **Low (latent)**

`serving_match` stamps `gender=""` (`rows.py:116`), reading context group 0. Matches training only while the split is off. **Fix.** Guard in `store.py` refusing a state built with the split on, or carry `gender` in the request (SERVE-04).

---

## 3. Training, evaluation and gates (`ml-service/ml/xi/`)

Pinned `scikit-learn==1.5.2` (`ml-service/requirements.txt:65`). EVAL-01/02 depend on that version's `HistGradientBoosting*` behaviour.

### EVAL-13 — No recency weighting, one global `C`, collinear objective inputs  **Low · retrain**

`train.py:53` (`LogisticRegression(C=0.3)`, no `sample_weight`); `contract.py:276-278` (`d_x` and `t1_x`, `t2_x` for the same stems — exactly collinear). Rows from 2005 and 2026 carry equal weight; the same `C` serves 1.9k-row TEST and 10k-row T20. **Fix.** Exponential age weights (half-life chosen on the folds), per-format `C` from the inner temporal split, and drop either the diffs or the raw pair.

### EVAL-14 — Averaging quantiles and Poisson parameters separately  **Low**

`performance.py:364, 369-379, 415-418`: `_average` averages `zero_inflation` and `rate` independently (mean becomes `avg(p)·avg(rate)`); `predict_marginalised` averages the two orientations' quantiles level-by-level, which narrows the 10–90 interval for bimodal cases. **Fix.** Average CDFs (or draws) and invert.

### EVAL-15 — Fold spread uses `ddof=0` on 11 folds  **Low**

`evaluate.py:165`; `perf_harness.py:227`. Understates spread ~5 %. **Fix.** `ddof=1`.

### EVAL-16 — Career baselines ignore day-close; models saved for formats with 50 rows  **Low**

`perf_baselines.py:40-50` (`shift(1).expanding()` across same-day matches — the baseline sees a same-day earlier match the model does not); `train.py:223` floor of 50 rows. **Fix.** Group the shift by (player, format, date); raise the floor to ~500.

---

## 4. Simulator, optimizer and serving (`ml-service/ml/xi/`, `ml-service/app/`)

### SERVE-03 — `RATINGS_STALE` measures the wrong quantity  **Medium**

`app/xi_service.py:297-303` compares `today - state.last_date` (date of the last match folded in) with `ratings_max_age_days`. In an off-season a run retrained yesterday is refused with a hint to retrain, which changes nothing; a run built with a far-back `--cutoff` whose data includes a recent match is accepted. The verdict is global, not per format. **Fix.** Compare against the manifest's cutoff / import high-water mark, per requested format.

### SERVE-04 — Serving fixtures are stamped with `state.last_date`  **Medium**

`app/xi_service.py:565-573` → `serving_match(..., store.state.last_date)`; consumed at `rows.py:146-155` (`on=match.match_date`, `age_vectors`). No request model carries a fixture date; `as_of` only selects a state. Every date-dependent feature is computed as of the last match in the state, and `gender=""` (`rows.py:115`) reads the men's context group if `gender_split_context` is on. **Fix.** Add `match_date` (and `gender`) to the request models; pass `match_date or as_of or today` to `serving_match`.

### SERVE-05 — Simulator batting depth contradicts per-player `p_bats`  **Medium**

`simulator.py:651-659`: `depth = (p_bats > u).sum()` with one shared uniform, then the *first* `depth` slots in `exp_bat_position` order bat. `exp_bat_position` (rating accumulator) and `p_bats` (L2-B classifier) are independent, so with order `[…, 0.9, 0.4, 0.7, …]` the 0.4 slot bats whenever the 0.7 slot would (realised 0.6). The reported `p_bats` (line 894) then disagrees with `/performance/predict` for the same fixture. **Fix.** Enforce a monotone envelope (`min(p_bats[:j+1])`) or reorder by `p_bats` for the depth decision and document it.

### SERVE-06 — Training process registry keyed by module  **Medium**

`app/training_orchestrator.py:73-80, 205, 218`; `settings.py:110` allows `MAX_CONCURRENT_TRAINING_JOBS > 1`. Two `/admin/train/retrain` calls: the first becomes unaddressable by `stop_training`, and whichever finishes first unregisters the survivor's handle. `stop()` marks `_stopped` before checking whether the process already exited (lines 98-102), so a stop racing a natural exit turns a success into `TrainingStopped` and a 409. **Fix.** Key by `Popen.pid` (or refuse a second run of the same module); treat `process.poll() is not None` as nothing to stop.

### SERVE-07 — `AsOfRatings` is built without `age_aware_cold_start`  **Medium (latent)**

`asof.py:50`; `xi_service.py:266-268` forward only `gender_split_context`. If `C.AGE_AWARE_COLD_START` (`contract.py:182`) is flipped on, live requests read the age-band prior while `as_of` requests read the neutral vector. **Fix.** Thread every state flag from the loaded store into `AsOfServer`.

### SERVE-09 — Chase "balls remaining" uses max observed deliveries  **Low**

`simulator.py:931, 940`: `capacity = second_deliveries.max()`. Biased low when no draw runs the full innings. **Fix.** Carry `context.deliveries` on `MatchDraws`.

### SERVE-10 — Marginal value's "neutral" player is a zero-impact debutant, not "average"  **Low**

`optimizer.py:509-514` zeroes impact rates and sets `pelo = ELO_INITIAL` but leaves `exp_balls_faced`, `exp_balls_bowled`, `keeper`, phase and sequence rates. The value systematically favours high-workload players; `models/xi.py:136` promises "an average one". **Fix.** Set every field to the pool median, or fix the description.

### SERVE-11 — `max_evaluations` cannot bound a pair-swap sweep  **Low**

`optimizer.py:322-335`: budget checked only between neighbourhoods; one pair sweep is C(11,2)×C(19,2) = 9,405 candidates. **Fix.** Pass the remaining budget into `_best_neighbour`.

### SERVE-12 — Bowling attribution splits the total including extras; all-zero weights spread uniformly over all eleven  **Low**

`simulator.py:743-748, 761-764`. **Fix.** Distribute `runs.sum(axis=1)` plus wide/no-ball share only; restrict the fallback to columns with `balls > 0`.

### SERVE-13 — Unknown format returns 503, not 422  **Low**

`xi_service.py:235-236, 413-418`; `main.py:553-554`. **Fix.** Validate `format` against `C.FORMAT_CODES` in the Pydantic validator (`xi.py:59-61`).

---

## 5. Cricsheet import and schema (`go-app/`)

Trace used: `Delivery` (`internal/cricsheet/cricsheet.go:159-166`) → aggregates in `importMatchFile` (`ingest.go:388-537`) and `BuildBallEventRows` (`ball_event_emit.go:49-129`) → `InsertBallEventsTx` (`db/repo_ball_event.go:347-418`) → `ball_event` (`migrations/0001_baseline.sql:46-64`, PK `:869`). `info.outcome` → `Outcome{Winner, By}` (`cricsheet.go:132-142`) → `ingest.go:272-289, 318-319` → `upsertMatchSQL` (`repo_match.go:48-73`).

### IMPORT-07 — Fail-fast abort skips display-name settlement and team lineage  **Medium**

`ingest.go:113-121, 126, 136`: on `g.Wait()` error the function returns before `UpdatePlayerDisplayNames` and `applyTeamLineage`. Committed files stay; `opposition.canonical_id` is never written, so Delhi Daredevils/Capitals etc. remain two clubs with reset Elo. `ImportMatchFile` (`ingest.go:186-188`) never applies lineage either. `ApplyTeamLineage` returning 0 is "the healthy answer" (`repo_lookup.go:472-474`) so nothing flags it. **Fix.** Run settlement after `Wait` regardless of error; surface `canonical_id` coverage in `/ops/status`.

### IMPORT-08 — Venue identity is the raw string; `normalized_name` never populated  **Medium · retrain**

`ingest.go:251-257` (`firstNonEmpty(info.Venue, info.City)`), `repo_lookup.go:434-436` upserts on `venue_name` only; `0001_baseline.sql:703-706` has `normalized_name` with a unique index that nothing writes. "M Chinnaswamy Stadium" / "…, Bangalore" / "…, Bengaluru" are three venues with separate familiarity and scoring baselines (`docs/config-and-data.md:599` already notes "four spelling pairs of one ground"). A blank venue makes the city a venue. `GetVenueID` errors are swallowed (`ingest.go:254`).

**Fix.** Fold the name as `reference-data/venue-geocoding.csv` does (`venue_key`) into `normalized_name` and upsert on that; drop the city fallback; store `info.city` in `venue.city`; log lookup errors.

### IMPORT-09 — Format taxonomy merges domestic multi-day with Tests, List-A with ODI, men with women  **Medium (design) · retrain**

`format.go:151-168`; `formats.go:56-68`; `config.json:13-28`; `cricsheet.go:24-45` does not decode `info.team_type` or `match_type_number`. `MDM` (Sheffield Shield, County Championship) is rated with Tests, `ODM` with ODIs, and every format pools men's and women's cricket with gender only a context group. T20I is inferred from a hand-maintained 12-team list rather than `team_type: international`. `original_match_type` is stored (`ingest.go:312`) so this is recoverable.

**Fix.** Decode `team_type`; use it for the T20I rule; add a `competition_level` column; evaluate a per-gender split on the harness (E7 exists) and at minimum exclude `MDM`/`ODM` from the TEST/ODI training population or document the pooling.

### IMPORT-10 — `-apply` flag parsed and ignored  **Medium**

`services/cricsheetimporter/options.go:100, 127-134`; `cmd/cricsheet-importer/main.go:46-54`: `Options.Apply` is never read. An operator expecting a dry run gets a full import that IMPORT-03 makes irreversible. **Fix.** Honour it or delete it.

### IMPORT-11 — `match_inning.target_runs` ignores `innings[].target`  **Medium**

`ingest.go:552-556, 879-887`: second innings target = first-innings runs (not +1), set even for Tests; D/L revised targets never stored. **Fix.** Decode `target` and `method`; store `target.runs/overs` when present, else `first + 1` for limited-overs innings 2 only.

### IMPORT-12 — Forfeited / zero-legal-ball innings dropped from `ball_event` but keep their number  **Low**

`ball_event_emit.go:40-42` (`if totalLegal == 0 { continue }`); consumer `sources.py:604` normalises by `innings.min()`, so the DB path renumbers innings differently from the JSON path. An innings of only wides has `balls_bowled=0` and `run_rate=0` (`ingest.go:548-551`). **Fix.** Emit rows for every delivery (`is_legal` exists) or write a `forfeited/declared` flag; never use `innings.min()`.

### IMPORT-13 — Dimension rows created outside the per-file transaction; lookups swallow errors  **Low**

`ingest.go:254-257, 297-300, 699-703`; `ball_event_emit.go:59-67`. Acknowledged in a comment. A failed file leaves `player` / `opposition` / `venue` rows and its spellings win display-name settlement (`identity.go:147-149`); striker/non-striker resolution errors write NULL with no log. **Fix.** Resolve dimensions through the tx; log every swallowed error; observe display names only after commit.

### IMPORT-14 — Maiden overs: wides do not break a maiden; partial overs count  **Low**

`ingest.go:394-397, 539-544, 898-906`. **Fix.** Accumulate `tr` for every delivery and require `legal == ballsPerOver`.

### IMPORT-15 — Registry fallback can duplicate a person; `PersonIDsByName` is map-order dependent  **Low**

`identity.go:135-151`; `cricsheet.go:63-80` ("later key wins" over Go map iteration); a name missing from one file's registry becomes a separate `name:<X>` row. Latent (coverage is total today) and only `Warn`. **Fix.** Look up by name + non-null `external_id` before creating; sort keys.

### IMPORT-16 — Process-global `EntityCache` never invalidated  **Low**

`cache.go:38-43, 51-65`: after a `dev-destroy` / re-migrate while go-app is up, cached ids point at rows that no longer exist (or, after `RESTART IDENTITY`, at different people). **Fix.** Clear at the start of `ImportDir`, or key by a schema generation stamp.

### IMPORT-17 — `MatchDate()` defaults to 1970-01-01  **Low**

`cricsheet.go:85-90`; `ingest.go:213`. A file with no `dates` gets an as-of date before every other match. **Fix.** Parse error, like the missing-team cases at `ingest.go:338-355`.

### IMPORT-18 — `ball_event` PK conflicts dropped silently  **Low**

`repo_ball_event.go:362, 416`: two `overs[]` entries with the same `over` number (a real source defect) lose the second silently. Fixed by IMPORT-03's delete-then-insert if `ON CONFLICT` is removed and the duplicate is made an error.

---

## 6. go-app prediction path, track record and ops

### GO-05 — Cancelled run plan can never record its outcome  **Medium**

`services/runplan/executor.go:130-135, 144-147, 179-184` call `Store.Finish(ctx, …)` / `save(ctx, …)` with the already-cancelled plan ctx; pgx returns `context canceled`, only logged. `pipeline_handlers.go:40-49` cancels compute-lane rows but `pipeline-plan` is in no lane (`executor.go:14-19`). The row stays `IN_PROGRESS`; `TrackingStore.Active` reports running; `run_plan_handlers.go:61` and `Executor.run:120` answer 409 until `TRACKING_STALE_CANCEL_AGE` (24h). Same on SIGTERM (`cmd/api/main.go:130`, then `db.Close()`). **Fix.** `context.WithoutCancel(ctx)` with a short timeout for `save` / `Finish` (as `tracking.CaptureExit` already does); include `runplan.PlanCommand` in the stop's cancel set.

### GO-06 — Lane lock is check-then-insert; cancel funcs overwritten  **Medium**

`internal/pipeline/job.go:52-62` (`LaneBusy` SELECT then `tracking.Start` INSERT, no lock; on `LaneBusy` *error* it proceeds as not busy); `server/app.go:74-84` `a.jobCancels[lane] = cancel` unconditional; `run_plan.go:96-102, 125-131` `planCancel.set` / deferred `clear()` clears whoever is stored. Two `POST /ops/pipeline/run/retrain` within one round-trip both start; stop cancels neither. **Fix.** Per-lane `sync.Mutex` across check+insert, or a partial unique index on `data_migrations (command) WHERE status='IN_PROGRESS'` / `pg_try_advisory_lock`; compare-and-clear for cancel funcs.

### GO-07 — `/xi/predict-win` and `/performance/predict` never told the toss  **Medium**

`ml_xi_client.go:100-114` (`mlXIWinRequest`) and `:453-461` (`mlPerformanceRequest`) omit `team1_bats_first`, which `models/xi.py:155-157, 253` accept and `xi_service.py:540, 586-597` use. For TEST (non-simulated) the headline P(win) and per-player numbers are toss-marginalised even when the caller sent the toss, and the response's `honoured: false` note blames the format. `objective_probability` / constraint checks are toss-blind everywhere. **Fix.** Add `Team1BatsFirst *bool` to both structs and thread `fix.team1BatsFirst` through.

### GO-08 — Venue lookup failure swallowed  **Medium**

`predict_team.go:406-411`: `if id, verr := GetVenueID(...); verr == nil { venueID = id }` — a typo, unknown venue or DB error yields a "no venue" prediction with nothing on the wire saying so. **Fix.** `400 VENUE_NOT_FOUND` on miss, propagate other errors, echo `venue: {resolved: false}`.

### GO-09 — `match_date` with a UTC offset truncated in UTC  **Medium**

`predict_handlers.go:393-402` accepts RFC3339 with offset; `predict_team.go:389`, `candidates.go:77`, `repo_selection.go:101, 115` use `MatchDate.Truncate(24h)` (UTC); `prediction_record.go:137` stores the untruncated value (pgx encodes `date` from Y/M/D in the value's own location). `2025-03-01T22:00:00-05:00` → cutoff 03-02, so the pool includes matches played *on* 1 March — including the fixture itself if imported. **Fix.** Normalise once in the handler to `time.Date(y, m, d, 0,0,0,0, UTC)` from the parsed value's own Y/M/D.

### GO-10 — Predicted winner at exactly 0.5 differs between served answer and track record  **Low**

`predict_team.go:573-583` (`>= 0.5` → team1) vs `trackrecord/build.go:115-122` (`> 0.5` → team1); each comment claims the other's rule. **Fix.** One helper.

### GO-11 — POST body ignored unless `Content-Type` is exactly `application/json`  **Low**

`predict_handlers.go:101`. `application/json; charset=utf-8` falls through to query parsing and a misleading 400. **Fix.** `mime.ParseMediaType`.

### GO-12 — ml-service 422 becomes `500 INTERNAL`  **Low**

`ml_client.go:94-115`, `json.go:53-81`: FastAPI's `{"detail": [ … ]}` array is not decoded. **Fix.** Map 4xx with unstructured detail to a `VALIDATION_ERROR` relay.

### GO-13 — `config.Load` ignores JSON decode errors  **Low**

`config/loader.go:27-45`: `_ = json.Unmarshal(b, cfg)`; a corrupt `config.json` passes `ValidateForServer` with zero values. **Fix.** Return the error.

### GO-14 — N+1 queries on prediction and track-record paths  **Low**

`repo_selection.go:260-300` (one `QueryRow` per manual-pool id); `repo_track_record.go:66-70, 75-112` (1 + 2 queries per fixture); `repo_prediction.go:132-163` reads every payload unpaged; `track_record_handler.go:27-45` has no paging or caching. **Fix.** `= ANY($1)` batching.

### GO-15 — `RunJob` returns nil after a recovered panic; `colorHandler` drops attrs  **Low**

`pipeline/job.go:84-108` (unnamed result); `logger/logger.go:58-64` (`WithAttrs` / `WithGroup` return the receiver unchanged, so `LOG_COLOR=1` loses every `slog.With` attribute). **Fix.** Named return; implement the handler methods.

### GO-16 — Non-UUID prediction id answers 500  **Low**

`prediction_record_handlers.go:115-131`; `repo_prediction.go:64-77`; `22P02` is not `pgx.ErrNoRows`. **Fix.** `uuid.Parse` first.

---

## 7. Weather, reference data and geocoding

Context: no weather, age or retirement column reaches a served model (`contract.py:158, 173-182`; `predict_handlers.go:32-35, 88` refuses a `weather` field). These findings limit the recorded experiment verdicts (X-2, X-1b) and the tracked reference data, not live predictions — until someone wires a family in.

### DATA-02 — Venue-key normalisation does not merge spellings; 20 rows are country centroids  **Medium**

`ml/weather/venues.py:209-213` folds case/accents/punctuation only. 896 spellings → 892 keys. `Kensington Oval, Bridgetown` (Barbados centroid) vs `…, Barbados` (Bridgetown); `Queen's Park Oval` likewise; rows with `admin1 == ""` and place == country cover 489 venue-days. **Fix.** Key by the first comma-part when trailing parts are a known city/country; reject a candidate with empty `admin1` when the query was a country name. Coordinate with IMPORT-08 so the DB and the CSV share one venue key.

### DATA-03 — Competition needle `"zimbabwe"` votes ZW for every venue Zimbabwe toured  **Medium**

`venues.py:203` `("zimbabwe", ("ZW",))` applied to every fixture via `_votes` (`:316-320`). Verified `countries_voted`: `Tony Ireland Stadium, Townsville` → `ZW AU`; `Goodyear Park` → `ZW ZA`; `Niaz Stadium, Hyderabad` → `ZW PK`; `Bready` → `ZW IE GB`. Needles `"friends"`, `"marsh"`, `"vitality"`, `"4-day"` (`:148-189`) are equally unanchored. **Fix.** Drop `"zimbabwe"`; anchor league needles to full event names; weight competition votes below team votes.

### DATA-04 — ERA5 hours indexed by local-time string with one fixed offset across DST  **Medium**

`ml/weather/archive.py:198` requests `timezone=auto`; `:264-273` looks up `f"{day}T{hour:02d}:00"` for 24 hours and ignores `utc_offset_seconds`; `:249-258` chains days ≤45 apart into one call (an English season is one March–October request; 5,566 of 19,561 cached days are `Europe/London`). All 19,561 rows in `era5-venue-days.jsonl` have exactly 24 slots, including the 9 rows on DST-transition dates — so every day on the far side of a transition inside a cluster is shifted by one hour. **Fix.** Request `timezone=UTC` and convert per hour with `zoneinfo`; cap cluster span.

### DATA-05 — `wx_rain_prior_day_mm` sums match-day rain up to the *inferred* start  **Medium (leak in the X-2 rain family)**

`ml/weather/features.py:64-67` adds `precipitation_mm[h] for h in range(0, start)` where `start` is a schedule *norm* from `sessions.py:88-116` (IPL 19, BBL 19, GB weekday 18). Any match that began earlier includes rain that fell during play. Humidity/temperature averages (`:55-57`) have the same exposure. **Fix.** End the window a fixed margin before the *earliest* plausible start for the rule, or use only the previous day's total; evaluate the family only on rules with a documented single start hour.

### DATA-06 — `WeatherCache` drops any unparsable line silently; a permanent `Miss` from one bad 200  **Low-Medium**

`archive.py:134-139` swallows `(ValueError, KeyError)` per line with no log; `:274-275` writes a `Miss` whenever a day's temperatures are all `None`, and the README makes misses permanent. `misses = 0` today. **Fix.** Log and count skipped lines; fail unless the skipped line is the last; only record a `Miss` when neighbouring days have data.

### DATA-07 — Year-precision birth dates stored as 1 January; `age_known` is a survivorship signal  **Low**

`wikidata-player-lookups.jsonl`: 258 of 6,973 `birth_date` values are `-01-01` (3.7 % vs ~0.27 % expected; `go-app/internal/biography/wikidata.go:202-204` acknowledges the query cannot distinguish); `ml/xi/biography.py:53-57` computes age to the day. `age_known` (`contract.py:172`) means "has a Wikidata item in the 2026-09-04 snapshot", which for a 2015 debutant is correlated with later notability — a leak in the X-1b arm (which happened to read null). **Fix.** Fetch the precision qualifier; treat year-precision as mid-year ±0.5; exclude `age_known` from any fold predating the snapshot or document it.

### DATA-08 — Session rules hard-code day games for women's ODI / ODM; associate T20 slots by venue rank  **Low**

`sessions.py:141-149, 170-178`. The 35.2 % night share (`EXTERNAL_DATA_PLAN.md:1088-1090`) is a rule artefact for the 3,469+ `t20i_associate` / `t20i_women` rows. **Fix.** Evaluate the day/night family only on rules with a documented single start hour.

### DATA-09 — Documentation counts do not match the files  **Low**

`reference-data/README.md:79` / `contracts/system-map.json:276` say 140 hand-placed rows; the CSV has 88 `hand-curated` notes and 47 unreviewed "country not among the archive's votes" rows. `README.md:82` "every row is a city-level fix" is contradicted by 20 centroid rows.

---

## 8. Ops, CI, security

### OPS-01 — Reference data, configs, scripts and contracts trigger no CI  **Medium**

`.github/workflows/ml-service-ci.yml:8-20`, `go-app-ci.yml:8-24` trigger on `ml-service/**`, `go-app/**`, `Makefile`, workflow files only. A hand edit to `venue-geocoding.csv` (the documented curation flow), a broken `player_biography_overrides.json`, or a stale `contracts/system-map.json` merges green. **Fix.** Add those paths; add a test that parses both CSV/JSONL files and asserts unique keys, country ∈ top votes or hand-curated, lat/lon inside the country, 24-slot arrays.

### OPS-02 — Default admin key and open ports in three places; watcher holds the Docker socket  **Medium**

`docker-compose.yml:43` `API_KEY: ${API_KEY:-dev-local-key}`, `:100` `ADMIN_API_KEY` same fallback; root `Makefile:88`, `ml-service/Makefile:83`, `scripts/cadence.sh:37` default to it; Postgres `:10` `"5432:5432"` and both APIs (`:68`, `:106`) bind all interfaces with `POSTGRES_PASSWORD` defaulting to `postgres`; the watcher (`:130`) mounts `/var/run/docker.sock`. `go-app/internal/server/auth.go:21-23` already fails closed when `API_KEY` is unset — the compose fallback defeats that. **Fix.** Drop the `:-dev-local-key` fallbacks; bind Postgres to `127.0.0.1`; read-only socket proxy for the watcher.

### OPS-03 — Frontend keeps the admin key in `localStorage` and attaches it by port heuristic  **Low**

`frontend/src/context/AuthContext.tsx:12-15, 31`; `frontend/src/api.ts:36-40, 50-52` attaches `X-API-Key` when the URL contains `:8080`, and `httpClient` accepts absolute URLs. **Fix.** In-memory / sessionStorage; send only when `url` starts with `BASE_API_URL`.

---

## 9. Fixed

### EVAL-12 — Manifest cannot reproduce a run  **Medium**

`runs.py:153-174` shells out to `git rev-parse HEAD`; the image (`ml-service/Dockerfile:11-35`) has no git and no `.git`, so `git_sha=""` on every served run. Missing: library versions, objective `C=0.3`, display `l2_regularization` / `min_samples_leaf`, `DISPLAY_SEEDS`, the performance `FitSpec`, source type. `dataset_sha` hashes `match_id|match_date` of decided matches only, so a re-import that changes squads or deliveries yields an identical sha. **Fix.** Inject `GIT_SHA` as build arg/env; add `library_versions`, `FitSpec.as_dict()` and the win-model constants; extend the digest with per-match delivery/squad counts. — PR #323.

**Three of the spec's seven items were stale, and one of its two defects was overstated.** `DISPLAY_SEEDS` does not exist: EVAL-02 (#309) made the display model one fit and EVAL-09 (#322) removed the performance seeds, so there is no seed concept left to record and none was added. `FitSpec.as_dict()` is already written per run — `xi_win_report.json` → `formats[].performance.fit.spec` — so it was not missing from the *run*, only from the manifest; the manifest now quotes that same object rather than rebuilding it, so the two cannot disagree. The source type is likewise already in the report under `data_quality.source`, and is now named in the manifest too. And `git_sha=""` is **not** true "on every served run": the run serving on the dev box carries `6021d58de0f5315d8663c103a1cac838a03bfad8`, because `make retrain` runs on the host where the checkout answers. The defect is exact for the containerised path and only that: `docker exec cric-ml-service python -c "from ml.xi.runs import git_sha; print(repr(git_sha()))"` prints `''` with `runs.git_sha.failed error=[Errno 2] No such file or directory: 'git'`, and the image carries no version-control binary and no repository directory. Every run built through `/admin/train/*` or the ops console's retrain step is therefore uncommitted-to.

**The commit.** Read from the checkout first — it cannot be left stale by an exported variable, and it appends `-dirty` when the tree carried uncommitted changes, because a clean sha over a dirty tree names code that was never committed — from `GIT_SHA` second, and failing both it records the word `unknown` with a warning naming the build argument to set. `ml-service/Dockerfile` takes `ARG GIT_SHA` after the `COPY`s (so setting it rebuilds nothing above it), `docker-compose.yml` passes it as a build argument and the root `Makefile` exports it from the checkout, so `make dev-up` bakes it in. On the wire an unestablished commit is the string `unknown`, not a blank: `shortCommit` in the frontend renders it as the word rather than slicing it to `unknow`, renders `-dirty` intact, and still renders a manifest with no commit at all as `—`.

**The digest.** It hashed `match_id|match_date` over the decided matches, which answers "the same list of fixtures?" — not the question a run's provenance has to answer. Demonstrated on the archive itself (21,293 decided matches, 468,461 player-match rows, 1,612 undecided), where the old digest reads `501c2488…`, exactly the `dataset_sha` of the run then serving:

| variation on the archive | old digest | new digest |
|---|---|---|
| one player swapped out of one XI | `501c2488…` **unchanged** | `bd9bf9eb…` |
| one more run off one delivery | `501c2488…` **unchanged** | `e09469aa…` |
| four more runs inside a drawn match | `501c2488…` **unchanged** | `da78890e…` |

The new digest hashes one line per decided match (identity, sides, venue, label), one per player-match row (the XI and the ten outcome columns — never the as-of feature columns, which are a function of the ratings and would make the digest of the data partly a digest of the model), and one per pass-level count. The counts are the only cover for undecided matches, which produce no row at all and still fold into the ratings; `DataQuality` gained `runs_scored` (runs off every delivery read) for that, beside the three difference-counts already there. **Residual limit, stated rather than hidden:** a change inside a single undecided match that leaves the pass totals alone is still invisible; closing it would mean digesting every delivery, which is the one scan this avoids. **Cost, measured on the full archive: 0.96 s** (old: 0.087 s), against a 216 s rating pass and a ~12 min retrain — nothing is re-read and no delivery is scanned twice.

**Also added, one enumeration each:** `library_versions` (python, scikit-learn, numpy, scipy, pandas, joblib — an unresolved one records `unknown`, never a blank); `model_params` (the objective's `C` and iteration ceiling, the display model's `l2_regularization`, `min_samples_leaf`, `early_stopping` and `random_state`, the grid margin and validation fraction) — the display model's fixed settings were literals inside `make_display_model` and are now a named constant it spreads, so the manifest cannot drift from the constructor; `performance_spec` per format; `source`. EVAL-06's `hyperparameters` stays the only record of what the grid *picked*, and a test asserts the two enumerations do not overlap.

**D-6: every run on disk becomes unloadable, and is refused by name.** `dataset_digest` is required, on the `ratings_through` precedent (P2-2, #280): a manifest without it carries a sha computed by a formula that could not see a squad or a delivery, so the two numbers are not comparable and a silent comparison is worse than a refusal. `read_manifest` raises naming the run and the reason; `list_runs` keeps the run listed with that reason in `refused`; `newest_run_id` skips it; `set_current` refuses to publish it; go-app's fallback scan (`opsstatus/artifacts.go`) mirrors the refusal so the offline listing agrees with the live one. All five runs under `output/ml-service/runs/` are refused after this merge, the served one included, and the remedy is the documented one — a retrain, about ten minutes on this database, then a reload. **Not retrain-flagged in the feature/label sense** (no feature, label or served model changes) but it does require the batch-3 retrain to restore a loadable run, so it rides with FEAT-15 and EVAL-09 and needs no pipeline run of its own.

**Rebased onto EVAL-10 (#324), which had to land first.** Making `dataset_digest` required refuses every run on disk, and EVAL-10's brief was to measure whether H-8 parity holds on the real served run — so it went first. Two couplings came out of the rebase and both are closed. (1) EVAL-10 moved the identity of what the pass consumed onto `BuildResult.match_keys()`; the digest now lives there too, as `BuildResult.dataset_digest()`, so `retrain` and `ml.xi.asof` read one computation rather than two. (2) EVAL-10's `round_trip_store` writes a run directory and loads it back through `XiStore.load` — the H-8 check and every harness window go through it — and it wrote no digest block, so the new refusal would have refused the parity check itself. It now records the absence explicitly (`runs.no_dataset_digest`, scheme `no-dataset`): a run that never walked a source is a different answer from a manifest written before the digest existed, and must not render as the same blank (§8.7). `make serving-parity`'s freshness comparison was rebuilding the *old* digest to compare against a run's, which would have mismatched on every run; it now calls `result.dataset_digest()`. The three archive values above were re-measured after the rebase and are unchanged, cost included.

**Tests, failing on `main` for the defect rather than for a missing name.** `test_the_commit_is_unknown_rather_than_blank_where_no_checkout_can_be_asked` patches `subprocess.run` to raise `FileNotFoundError` the way the image does and asserts the recorded commit is not empty — on main, `assert '' != ''`. `test_a_re_imported_squad_is_invisible_to_the_decided_match_list_and_not_to_the_digest` runs two real retrains whose decided-match lists are identical and whose XIs differ, and compares the `dataset_sha` each manifest records — on main, two identical shas. Beside them: the delivery change, the undecided-match change, digest stability, the coverage block, that the digest reads no rating-derived column, the checkout outranking the environment, `-dirty`, the two version-control failure modes, an uninstalled library, and in Go the mirrored refusal in the fallback listing. No threshold, gate or coverage floor moved.

### Batch 1 — the re-import, retrain and harness record

Nine PRs — **GO-01 + SERVE-08 (#292), GO-03 (#293), GO-02 (#294), IMPORT-03 (#295), IMPORT-01 (#296), IMPORT-02 (#297), FEAT-04 (#298), IMPORT-04 (#299), FEAT-08 (#300)** — six of them re-import- or retrain-flagged. Per § 1 rule 6 of `docs/AUDIT_FIX_RUNBOOK.md` the batch was landed first and the pipeline run **once** at the end, on main at `1b960459`. This is that record. Steps ran in order: migrate → re-import → `make xi-parity` → `make retrain` → `make reload` → `make evaluate`.

#### Step 1 — migrate

Migrations `0017_match_result.sql` (`match.result`, `match.result_method`) and `0018_ball_event_extras_breakdown.sql` (`extras_wides / noballs / byes / legbyes / penalty` on `ball_event`) were in the repo but unapplied, and `_BALLS_SQL` names the `0018` columns — so the rating pass, `evaluate`, `xi-parity` and as-of serving all failed on an undefined column until this ran.

| | before | after |
|---|---|---|
| migration level | `0016_auction_projection_assumptions.sql` (16 applied) | `0018_ball_event_extras_breakdown.sql` (18 applied) |

Wall clock **2 s**. Eight columns exist afterwards that did not before: `match.result`, `match.result_method`, and the five `ball_event.extras_*` counts beside the pre-existing lossy `extras_kind`.

#### Step 2 — the whole-archive re-import

`make cricsheet-import` over all **22,905** files, fail-fast on, concurrency 12 — not an incremental run. IMPORT-03 (#295) is what makes this rewrite rather than skip: `repo_match_facts.go` now deletes a match's `ball_event` and aggregate rows at the top of its transaction, so the importer's fixes reach matches that were already in the database.

Wall clock **2 min 34 s** (14:05:58–14:08:32 UTC). **Zero errors**; 27 warnings, all of them the two documented benign cases — 25 × "file name is not a Cricsheet match id, deriving one" (the bounded, logged hash fallback § 10 records as sound) and 2 × "player named on both teams, omitted from both squads".

| table | before | after | delta |
|---|---:|---:|---:|
| `match` | 22,905 | 22,905 | 0 |
| `ball_event` | 11,579,603 | 11,578,345 | −1,258 |
| `match_inning` | 50,691 | 50,465 | −226 |
| `batting_data` | 434,557 | 434,001 | −556 |
| `bowling_data` | 299,686 | 299,460 | −226 |
| `fielding_data` | 176,700 | 176,576 | −124 |
| `match_player` | 505,287 | 505,287 | 0 |
| `player_biography` | 13,662 | 13,662 | 0 |

Every delta is exactly the super-over removal; nothing else moved. `player_biography` is untouched because the Wikidata backfill owns it, not the importer, and it was deliberately not re-run.

**Super overs (IMPORT-01).** `match_inning` rows with `inning_number > 2` outside Tests: **226 across 110 matches → 0**. `ball_event` rows with `innings > 2` outside Tests: **1,258 → 0**. The importer logged `super_over_innings` for exactly 110 files totalling 226 innings, and a direct scan of the archive confirms **110** files carry 226 super-over innings — so the innings count quoted when the batch was planned (226) is right and the file count (113) was not; the correct figure is 110.

**Tie-breakers (IMPORT-02).** Matches with a NULL winner: **1,723 → 1,612**, so **111 matches gained a winner** — and exactly **111** rows now read `result = 'tie'` beside a non-NULL winner, which is the same set. `match.result` is populated as 1,028 `draw`, 491 `no result`, 204 `tie`, the rest NULL (an outright result). `result_method` records 1,018 `D/L`, 5 `VJD`, 5 `Awarded` and one `Lost fewer wickets` — eighteen characters, the value that vindicates IMPORT-02's widening to `varchar(32)`.

**Extras breakdown (IMPORT-04).** Stored across 11.58 M deliveries: 246,122 wides, 76,980 no-balls, 78,492 byes, 154,876 leg-byes, 954 penalty. The five parts sum to `runs_extras` on **every** delivery (0 mismatches), and **234,322 runs** (byes + leg-byes + penalty) are now separable from what the bowler is charged.

#### Step 3 — `make xi-parity`: **passes**

This is the batch's acceptance test at the data layer. It had been expected to fail since #298 and #300 added two compared counts Postgres could not satisfy before the re-import. After it, **all seventeen counts agree** and the run exits clean (4 min 42 s):

| count | postgres | cricsheet |
|---|---:|---:|
| offered_matches / matches_read | 22,905 | 22,905 |
| out_of_scope / unusable_matches | 0 | 0 |
| undecided_matches | 1,612 | 1,612 |
| **drawn_or_tied_matches** (FEAT-04, #298) | **1,121** | **1,121** |
| **runs_not_charged_to_bowler** (FEAT-08, #300) | **234,322** | **234,322** |
| namesake_sides / unknown_player_keys | 0 | 0 |
| oversized_squads | 1,365 | 1,365 |
| player_keys / team_keys | 13,639 / 522 | 13,639 / 522 |
| players_with_birth_date | 6,955 | 6,955 |
| matches_with_stage_label / knockout | 22,101 / 1,420 | 22,101 / 1,420 |
| reconstructible_table / dead_rubber | 16,587 / 2,215 | 16,587 / 2,215 |

The two new counts are the ones worth reading. `drawn_or_tied_matches` = 1,121 is exactly the 1,028 draws plus 204 ties less the 111 tie-breakers somebody won — the rule FEAT-04 named, now computed identically on both sources. `runs_not_charged_to_bowler` = 234,322 agrees to the run with the direct SQL sum of `extras_byes + extras_legbyes + extras_penalty` over `ball_event`, so the two paths are charging bowlers the same quantity. Both rating passes produce the same 21,293 training rows and 469,743 player-match rows.

**One operator note, recorded because the first attempt failed.** Run without `BIRTH_DATES=`, parity fails on a single count — `players_with_birth_date: postgres 6955, cricsheet 0` — because the archive carries no biography (X-1b) and every age reads as unknown, which the run warns about in as many words. That is a flag omission, not a source disagreement, and the `ml-service` Makefile documents the remedy next to the target: `make export-birth-dates BIRTH_DATES=…` (6,967 players) then `make xi-parity BIRTH_DATES=…`. The sixteen other counts, including both new ones, already agreed on that first attempt. Anyone re-running this must pass `BIRTH_DATES`.

#### Step 4 — retrain and reload

`make retrain CUTOFF=2026-09-13` — **9 min 47 s** (14:20:15–14:30:02 UTC), clean, no error or traceback. Then `make reload`, **2 s**.

| | |
|---|---|
| run id | **`20260913T142341Z-ab4caa13`** |
| cutoff | 2026-09-13 |
| ratings through | 2026-09-09 (13,639 players) |
| dataset sha | `501c24882251…` |
| git sha | `53fa134d` |
| training rows | T20 12,130 · ODI 4,995 · TEST 2,095 · T20I 2,073 |
| `data_quality_failures` | none |

Reload returned `status: reloaded, loaded: true` for all four formats, ratings `fresh: true` (age 4 days against a 14-day bar), and its served `data_quality` block carries `drawn_or_tied_matches: 1121` and `runs_not_charged_to_bowler: 234322` — the new columns reaching the serving path, not just the training one.

The hyperparameter grid moved one format: TEST took `max_depth 3, learning_rate 0.08, max_iter 200` ("beat the incumbent on the inner split", 0.6329 vs 0.6303); T20, T20I and ODI all kept the incumbent because no candidate beat it by more than 0.002. That is the three-point grid behaving as `docs/ml-and-training.md` describes, not a tuning result.

**Every format reports `n_holdout: 0` and no headline metric, with a warning each.** This is expected, not a regression: the cutoff is today and the archive ends 2026-09-09, so no row falls at or after it and there is nothing to score. A production retrain at today's cutoff is meant to train on everything; the choice-facing numbers come from the L4 harness in step 5, which is the whole reason `evaluate` is a step beside the pipeline rather than in it.

#### Step 5 — `make evaluate`: the batch's acceptance test

**72 min 12 s** (14:30:25–15:42:37 UTC), launched detached and polled; clean exit, no traceback, report written. Before is the last recorded report (generated 2026-09-02, `n_rows` 21,093); after is this run (generated 2026-09-13, `n_rows` **21,293**). Walk-forward means:

| fmt | obj AUC | disp AUC | obj Brier | base-rate Brier | swap share | n_rows (fmt matches) |
|---|---|---|---|---|---|---|
| T20 | 0.6974 → **0.6963** (−0.0011) | 0.7300 → **0.7279** (−0.0021) | 0.2180 → **0.2184** (+0.0004) | 0.2498 → **0.2498** (0.0000) | 0.0023 → **0.0020** | 11,997 → 12,130 |
| T20I | 0.7561 → **0.7609** (+0.0049) | 0.7533 → **0.7506** (−0.0027) | 0.1987 → **0.1982** (−0.0005) | 0.2501 → **0.2501** (0.0000) | 0.0075 → **0.0084** | 2,049 → 2,073 |
| ODI | 0.6725 → **0.6698** (−0.0027) | 0.7067 → **0.7045** (−0.0022) | 0.2259 → **0.2264** (+0.0005) | 0.2472 → **0.2473** (0.0000) | 0.0000 → **0.0000** | 4,961 → 4,995 |
| TEST | 0.6259 → **0.6257** (−0.0003) | 0.6456 → **0.6379** (−0.0077) | 0.2376 → **0.2378** (+0.0002) | 0.2503 → **0.2503** (0.0000) | 0.0052 → **0.0052** | 2,086 → 2,095 |

**Nothing here is a movement anyone should read as a result.** The largest change in either direction — T20I objective AUC +0.0049, TEST display AUC −0.0077 — is a fraction of that format's own across-fold standard deviation (T20I 0.039, TEST 0.100 on those same columns). Every other cell is a thousandth or two. The batch was a correctness batch, and the headline numbers say what a correctness batch should say when the defects were rare: they did not move.

**The confound, stated plainly.** The before report was computed by the pre-fix code on a pre-re-import archive **and** before the user's own import of 2026-09-13 08:43 added 87 matches — `offered_matches` was 22,815 then against 22,905 now, and every format gained matches. So the difference above reflects **both** the nine PRs and a larger, differently-shaped dataset, and the two cannot be separated without a second hour-long run that nobody has asked for. No number in this table should be attributed to a specific finding: none of the movements is large enough or mechanically specific enough to carry such a claim, and the honest reading is that the fixes changed the data materially (226 innings, 1,258 ball events, 111 winners, 234,322 runs re-attributed) while changing the model's discrimination not at all.

**Gates.** `gates.passed` **true**, `problems` empty — same as before. `serving_parity` **passes**, 50 matches / 50 win rows / 1,100 player rows / 1,100 performance predictions / 50 simulations compared, `max_abs_difference` **0.0**, zero mismatches — identical to before. `leak_canary` is unchanged in kind and nearly in value: the best single column is still `team_elo_diff` for T20/T20I/ODI and `d_exp_mean_matches` for TEST, and the same three test-control suspects appear (`team_h2h` in T20 and T20I, `d_pelo_min` in T20I) at AUCs within 0.002 of before. **No gate that passed before fails now, and no new suspect appeared.**

**Servability is unchanged in all four formats.** T20 selection still fails its derived bar (agreement 0.5034 against bar 0.5055) and is not served; T20I still passes and is served (0.5636 → 0.5818 against bar 0.4770); ODI still passes and is served (0.5665 → 0.5597); TEST still passes its bar but is still not served. Simulation stays within tolerance for T20, T20I and ODI, and TEST still has no simulated folds. Not one decision flipped.

#### What this batch is accepted on

Parity — the acceptance test the two new counts were added to be — **passes on all seventeen counts**, including `drawn_or_tied_matches` and `runs_not_charged_to_bowler`, which could not agree before the re-import. The data moved exactly as the nine PRs predicted and nowhere else. Every harness gate that passed before still passes. The model numbers did not move outside noise, and the confound above means they could not have settled anything if they had. **No finding was fixed or worked around during this pass**, and nothing regressed.

### Batch 2 — the re-import, retrain and harness record

Nine PRs — **IMPORT-05 (#302), EVAL-04 (#303), IMPORT-06 (#305), FEAT-01 (#306), FEAT-03 (#307), FEAT-02 (#308), EVAL-02 (#309), EVAL-01 (#310), EVAL-03 (#311)** — most of them re-import- or retrain-flagged. Per § 1 rule 6 of `docs/AUDIT_FIX_RUNBOOK.md` the batch was landed first and the pipeline run **once** at the end, on main at `da89f149`. This is that record. Steps ran in order: migrate → re-import → `make xi-parity` → `make retrain` → `make reload` → `make evaluate`.

#### Step 1 — migrate

Migrations `0019_ball_event_wicket.sql` (the `ball_event_wicket` child table; `ball_event.wicket_kind` / `player_out_id` dropped) and `0020_match_player_replacement.sql` (`match_player.is_replacement`) were in the repo but unapplied, and `ml/xi/sources.py` names both — so the rating pass, `retrain`, `evaluate`, `xi-parity` and as-of serving all failed on an undefined column or a missing relation until this ran.

| | before | after |
|---|---|---|
| migration level | `0018_ball_event_extras_breakdown.sql` (18 applied) | `0020_match_player_replacement.sql` (20 applied) |

Wall clock **10 s**. One table exists afterwards that did not before (`ball_event_wicket`), one column (`match_player.is_replacement`), and two columns are gone (`ball_event.wicket_kind`, `ball_event.player_out_id`).

`0019` carries the first wicket of every delivery into the new table as it drops the columns, so a database migrated but not yet re-imported describes the same cricket it did before rather than none: **353,547 rows**, one per delivery that held a wicket. `0020`'s column defaults to false for every existing row, so **0 replacements** are flagged. The re-import is what writes the other wickets and the flags; until it runs, `make xi-parity` reports the difference, which is what step 3 measures.

#### Step 2 — the whole-archive re-import

`make cricsheet-import` over all **22,905** files, fail-fast on, concurrency 12 — not an incremental run. IMPORT-03 (#295, batch 1) is what makes this rewrite rather than skip.

Wall clock **3 min 1 s** (12:00:09–12:03:10 UTC). **Zero errors**; 28 warnings — the 27 documented benign ones (25 × "file name is not a Cricsheet match id, deriving one", 2 × "player named on both teams, omitted from both squads") and **one new kind**, `cricsheet: replacement not in the side's info.players, side kept as listed` on **1537342**, which is exactly the case FEAT-02 (#308) recorded in advance and logged deliberately.

| table | before | after | delta |
|---|---:|---:|---:|
| `match` | 22,905 | 22,905 | 0 |
| `ball_event` | 11,578,345 | 11,578,345 | 0 |
| `match_inning` | 50,465 | 50,465 | 0 |
| `batting_data` | 434,001 | 434,001 | 0 |
| `bowling_data` | 299,460 | 299,460 | 0 |
| `fielding_data` | 176,576 | 176,576 | 0 |
| `match_player` | 505,287 | 505,287 | 0 |
| `player` | 13,694 | 13,694 | 0 |
| `player_biography` | 13,662 | 13,662 | 0 |
| **`ball_event_wicket`** | 353,547 *(0019's carry)* | **353,571** | **+24** |
| **`match_player` where `is_replacement`** | 0 | **1,362** | **+1,362** |

**No row count moved.** That is the right answer and worth saying plainly: batch 1 had already re-imported the archive, and batch 2's importer changes redefine *what a row says*, not which rows exist. Only the two new stores moved, and both moved to exactly the figure their finding predicted.

The figures the brief asked for, measured after the run:

| | |
|---|---:|
| `ball_event` | **11,578,345** |
| `match_inning` rows above innings 2 outside Tests | **0** |
| `ball_event_wicket` rows | **353,571** |
| `match_player` rows with `is_replacement = true` | **1,362** |
| matches with a non-null `result` | **1,723** |

The first two are batch 1's results holding (super overs stay out of the innings record; the importer logged `super_over_innings` for the same 110 files). The last three are batch 2's.

**Wickets (IMPORT-06, #305).** The store now holds **353,571** wicket records, which is *exactly* the count of a direct scan of the archive's non-super-over deliveries — so no wicket in the archive is now missing from the database. Fourteen kinds appear and all fourteen are in `configs/wicket_kinds.json`: 200,579 caught · 69,621 bowled · 39,610 lbw · 22,898 run out · 10,252 caught and bowled · 9,664 stumped · 463 retired hurt · 273 hit wicket · 121 retired out · 45 retired not out · 34 obstructing the field · 8 handled the ball · 2 timed out · 1 hit the ball twice. So **23,064** wickets are not the bowler's and **508** are not dismissals at all, both agreeing with the finding's survey to the record. **16** deliveries carry more than one wicket and the largest carries **10** (1483765), as predicted.

**A doc slip, recorded because the measurement contradicts the prose.** Migration `0019`'s header comment and IMPORT-06's § 9 entry both say the first-only store lost "17 wicket records across 16 deliveries". The measured figure is **24**, and 24 is forced by the same entry's own survey: 15 deliveries of two wickets lose one each, and the one delivery of ten loses nine — 15 + 9 = 24. The archive scan and the `+24` delta above agree independently. The behaviour is right and the fix is complete; only the number in the prose is wrong. No code or data change follows from it.

**Replacements (FEAT-02, #308).** **1,362** `match_player` rows are flagged, across **1,341** sides of the archive's 45,810. Squad sizes resolve exactly as the finding predicted:

| | sides |
|---|---:|
| over eleven as listed | 1,365 |
| **over eleven once replacements are out** | **24** |

The 24 are the ones the route could not fix and recorded in advance: 23 oversized sides that carry no replacement entry (21 Syed Mushtaq Ali Trophy 2022, 2 Women's T20 Challenge 2018) and 1537342, whose named replacement belongs to the other side — the one warning above. Everything else is now an eleven.

**Balls faced (IMPORT-05, #302).** The archive holds 202,331 wides and 58,068 no-balls. Under the new rule a batter faces every delivery but a wide, so the database should charge batters `11,578,345 − 202,331 = 11,376,014` balls. `sum(batting_data.balls)` reads **11,376,015** — right on the rule but for **one ball**, traced below.

**A pre-existing defect this measurement surfaced — recorded, not fixed (B-17 candidate).** The one ball is match **514034** innings 4 (South Africa v Sri Lanka, 2012-01-03, Test, won by 10 wickets): a fourth innings consisting of a single delivery, a no-ball off which the batter scored one, winning the match. `batting_data` charges the batter that ball (correct under IMPORT-05) and `match_inning` records the innings with 2 runs and 0 legal balls (also correct), but **`ball_event` holds no row for it at all** — the innings is absent from the ball store. The cause is in `go-app/internal/cricsheet/ball_event_emit.go:46-53`: `BuildBallEventRows` counts the innings' legal deliveries for phase clamping and then `if totalLegal == 0 { continue }`, so an innings whose every delivery is illegal emits nothing. It is not a batch-2 regression — `ball_event` did not move in this re-import — and it is the only such innings in 11.58 M deliveries, but it is a real disagreement between two stores of the same cricket, and the rating pass reads the one that is missing the ball. Reported to the master rather than fixed here; § 1 rule 6's pass does not fix findings.

#### Step 3 — `make xi-parity`: **passes**

This is the batch's acceptance test at the data layer, and the reason it is the step that could have stopped the pass. Four PRs added a compared count that Postgres could not satisfy before the re-import — `deliveries_not_faced` (IMPORT-05, #302), `dismissals` (IMPORT-06, #305), `decided_matches_without_deliveries` (FEAT-03, #307) and `replacement_players` (FEAT-02, #308) — so the check has been failing by design since they landed, and `oversized_squads` changed meaning under #308 from "over eleven as listed" to "over eleven once replacements are out". After the migration and the re-import, **all twenty-one counts agree** and the run exits clean (**5 min 7 s**, 12:05:31–12:10:38 UTC):

| count | postgres | cricsheet |
|---|---:|---:|
| offered_matches / matches_read | 22,905 | 22,905 |
| out_of_scope / unusable_matches | 0 | 0 |
| undecided_matches | 1,612 | 1,612 |
| drawn_or_tied_matches | 1,121 | 1,121 |
| **decided_matches_without_deliveries** (FEAT-03, #307) | **0** | **0** |
| runs_not_charged_to_bowler | 234,322 | 234,322 |
| **deliveries_not_faced** (IMPORT-05, #302) | **202,331** | **202,331** |
| **dismissals** (IMPORT-06, #305) | **353,063** | **353,063** |
| namesake_sides / unknown_player_keys | 0 | 0 |
| **oversized_squads** (FEAT-02, #308 — new meaning) | **24** | **24** |
| **replacement_players** (FEAT-02, #308) | **1,362** | **1,362** |
| player_keys / team_keys | 13,639 / 522 | 13,639 / 522 |
| players_with_birth_date | 6,955 | 6,955 |
| matches_with_stage_label / knockout | 22,101 / 1,420 | 22,101 / 1,420 |
| reconstructible_table / dead_rubber | 16,587 / 2,215 | 16,587 / 2,215 |

`source parity: the database and the archive agree`.

The four new counts are the ones worth reading, and each is independently checkable against step 2's direct measurements. **`deliveries_not_faced` = 202,331** is exactly the archive's wide count and exactly `ball_event`'s `extras_wides > 0` count, so both paths are now excluding the same deliveries from a batter's ledger. **`dismissals` = 353,063** is the 353,571 wicket records less the 508 that `configs/wicket_kinds.json` calls `not_out` (463 retired hurt, 45 retired not out) — the vocabulary applied identically in Go and in Python, which is the whole point of the shared config. **`replacement_players` = 1,362** matches the flagged `match_player` rows to the row, so the Go importer's rule and `sources.replacement_keys`' rule agree on who came in. **`oversized_squads` = 24** is the residual both sources compute the same way, and it agreeing at 24 rather than at 1,365 is what says the flag reached the database.

**`decided_matches_without_deliveries` = 0 on both sources**, which confirms FEAT-03's own survey: the guard it added still guards a case that has not occurred. Note that this is a *match*-level count and so does not see the one-innings gap step 2 found at match 514034 — that match has three other innings full of deliveries, so it is decided and has deliveries, and the count is right to read 0. The two observations are consistent; neither covers the other.

Both rating passes produce the same **21,293** training rows — unchanged from batch 1 — and the same **468,461** player-match rows, down from batch 1's 469,743. That fall of 1,282 is FEAT-02 doing what it was merged to do: a replacement is no longer one of the eleven, so he no longer contributes a player row to the match he came into. It is smaller than the 1,362 flagged rows because some replacements appear in matches that yield no player rows at all (the 1,612 undecided ones), which is the expected direction and magnitude.

Both languages logged the 1537342 case in their own words, as FEAT-02 said they would: Go as `Dambulla Sixers: RMMP Rathnayake`, Python as `Dambulla Sixers: afe830a2`, the same man by name and by registry key. Two namesake warnings appeared on the archive path, matching the importer's two.

**The operator note from batch 1 still stands and was obeyed:** `BIRTH_DATES=` must be supplied or `players_with_birth_date` fails spuriously at postgres 6,955 against cricsheet 0, because the archive carries no biography (X-1b). `make export-birth-dates BIRTH_DATES=…` wrote 6,967 players first; the Wikidata backfill was **not** re-run, and `player_biography` is untouched at 13,662 rows.

#### Step 4 — retrain, and a reload that **failed**

`make retrain CUTOFF=2026-09-14` — **15 min 2 s** (12:11:26–12:26:28 UTC), clean, no error or traceback, `data_quality_failures: []`. That is half again batch 1's 9 min 47 s, and the extra is where EVAL-03 (#311) put it: the performance members are now fitted twice per format, once on the fold and once on the whole history.

| | |
|---|---|
| run id | **`20260914T121506Z-da5b6680`** |
| cutoff | 2026-09-14 |
| ratings through | 2026-09-09 (13,639 players) |
| dataset sha | `501c24882251…` |
| git sha | `8e2f7863` |
| training rows | T20 12,130 · ODI 4,995 · TEST 2,095 · T20I 2,073 |
| player-match rows | 468,461 |
| `usable` | **true** |
| `unusable_reasons` | `{}` (empty) |

**`usable: true` (EVAL-04, #303).** The run is publishable on the gate's own terms. Read what that verdict actually rests on, though: both of EVAL-04's retrain clauses need a holdout, every format reports `n_holdout: 0` because the cutoff is today and the archive ends 2026-09-09, and so neither clause had a number to read. The run is usable because nothing disqualified it, not because something confirmed it. This is the spec gap EVAL-04's own entry recorded in advance ("every served run is trained at today's cutoff and has no holdout, so the retrain clauses have nothing to read on cadence"), and this pass is the first run to demonstrate it rather than predict it. The four `format_notes` say so in as many words.

**`n_iter` (EVAL-01, #310).** Every format reports **`n_iter: 300`**, equal to the `max_iter` the grid chose. The field did not exist in batch 1's manifest, and on batch 1's code T20 — the one format above the 10,000-row line — would have stopped at roughly 117 of those 300 on a random 90 % of the rows. So T20's served display model is now the model its grid scored, and the other three, which were always below the line, are unchanged in kind.

**`train_to` and `calibration_from` (EVAL-03, #311).** Both fields are new, and they show the staleness closed:

| fmt | train_from | **train_to** | **calibration_from** | perf n_train | n_calibration |
|---|---|---|---|---:|---:|
| T20 | 2007-09-01 | **2026-09-09** | **2026-06-09** | 266,877 | 12,541 |
| T20I | 2005-02-17 | **2026-09-06** | **2026-06-11** | 45,606 | 1,078 |
| ODI | 2002-06-27 | **2026-09-09** | **2026-06-09** | 109,888 | 4,640 |
| TEST | 2002-12-26 | **2026-09-08** | **2026-06-12** | 46,090 | 1,012 |

`train_to` now reaches the last match in the archive in every format, and `calibration_from` sits **92 days** before it — the exact interval EVAL-03 named. The decisive number is that `performance.fit.n_train` *equals* `performance.n_train` (266,877 for T20): the members saw every row, calibration fold included, where before they saw only the rows before the fold. Batch 1's report is the control — every one of its folds reads a `train_to` about 94 days short of its own cutoff (fold 1: cutoff 2024-01-01, `train_to` 2023-09-29; fold 11: cutoff 2026-06-01, `train_to` 2026-02-27) and carries no `calibration_from` key at all.

**The grid moved nothing.** All four formats kept the incumbent (`max_depth 3, learning_rate 0.04, max_iter 300`) on "no candidate beat the incumbent by more than 0.002" — where batch 1 had moved TEST to `0.08 / 200`. The inner-split scores behind that did move, and not uniformly: T20I's incumbent rose 0.7280 → 0.7566 and TEST's fell 0.6303 → 0.6133. These are inner-split numbers on a split that is not the harness's, they are the only scored figures a holdout-less retrain produces, and they should not be read as results; step 5 is where the choice-facing numbers are.

**`dataset_sha` is unchanged at `501c24882251…`, and that is correct rather than suspicious.** The digest is `sha256` over `match_id|match_date` for every match the pass walked (`runs.py:186`), and the re-import changed no match's id and no match's date. Worth stating plainly because it is a trap for a reader: the sha answers "were these two runs trained on the same cricket?", not "were they trained on the same features". Batch 1's run and this one share a sha while differing in what a ball faced is, what a wicket is, how many men a side has and how involvement is computed. Anyone using the sha to tell these two runs apart will be misled; the run id and the git sha (`53fa134d` against `8e2f7863`) are what separate them.

##### `make reload` **failed — HTTP 409, and the run was not published**

```
{"code": "RUN_ARTIFACTS_INVALID",
 "message": "run 20260914T121506Z-da5b6680: the rating artifact is missing 2 array(s) this code
             reads (bat_matches, bowl_matches); it was written by an older pass and cannot be
             served. Retrain to produce a run with all 32 arrays.",
 "hint": "20260913T142341Z-ab4caa13 is still serving"}
```

**This is not a defect in the batch, and it is not a bad run.** It is the deployed ML service running **pre-batch-2 code**. FEAT-01 (#306) deleted `bat_matches` / `bowl_matches` from the rating state and added `xi_bat_balls` / `xi_bowl_balls`, and recorded that "an older artifact is refused by name". The guard fired in the other direction: the artifact is new and the *reader* is old. Confirmed directly in the container — `store.PLAYER_ARRAY_NAMES` has **21** entries there and includes `bat_matches` and not `xi_bat_balls`, against the **32** the new run writes — and the image is dated **2026-09-13 08:42 UTC**, built before any of batch 2's nine PRs landed. The compatibility check did exactly what it was written to do: it refused to serve a mismatch instead of loading one and producing nonsense.

**The consequence, stated plainly.** `current` still points at batch 1's run `20260913T142341Z-ab4caa13`; the new run is on disk, `usable: true`, and **unpublished**. The stack must be rebuilt from current `main` before `make reload` can succeed. That was not done here: rebuilding the application images is outside this pass's remit, this worktree is not the checkout the running stack was built from, and § 1 rule 6's pass reports a failing step rather than working around it.

**Step 5 is unaffected and its numbers stand.** `make evaluate` does not talk to the service: there is no HTTP client anywhere in `ml/xi/`, the harness reads Postgres directly and builds the served recipe in-process (`asof.serving_parity`). So the harness below measures this batch's code against this batch's data, which is what it is for — it simply describes a system that is not yet the one on the port.

#### Step 5 — `make evaluate`: the batch's acceptance test, and **a gate fails**

**2 h 5 min 42 s** (12:33:02–14:38:44 UTC), launched detached and polled; the report was written in full, and then **the command exited 1**. That is EVAL-04 (#303) working: `check_report` now evaluates each standing gate's threshold and `make evaluate` fails the build on a violation, where before it checked only that the report carried a path. This is the first pass under that enforcement and it is the first pass to go red.

The run is three times batch 1's 72 minutes, well past the 54–70 the runbook quotes. EVAL-03's second member fit per format is part of it (T20 alone spent 52 minutes on its eleven folds, at 211–292 s per performance fit), but it does not account for all of it; anyone budgeting this step should now budget two hours.

##### The headline

Before is batch 1's run `20260913T142341Z-ab4caa13` (generated 2026-09-13, `n_rows` 21,293); after is this run (generated 2026-09-14, `n_rows` **21,293**, `n_player_rows` 469,743 → **468,461**). Walk-forward means, with each format's across-fold standard deviation on the *after* column:

| fmt | objective AUC | fold sd | display AUC | fold sd | n_rows (fmt matches) |
|---|---|---:|---|---:|---:|
| T20 | 0.6963 → **0.6980** (+0.0018) | 0.0386 | 0.7279 → **0.7295** (+0.0017) | 0.0479 | 12,130 |
| T20I | 0.7609 → **0.7670** (+0.0061) | 0.0441 | 0.7506 → **0.7618** (+0.0111) | 0.0413 | 2,073 |
| ODI | 0.6698 → **0.6733** (+0.0035) | 0.0729 | 0.7045 → **0.7110** (+0.0065) | 0.0755 | 4,995 |
| TEST | 0.6257 → **0.6114** (−0.0143) | 0.1135 | 0.6379 → **0.6286** (−0.0094) | 0.0969 | 2,095 |

**Not one of these movements is a result.** Every delta is a small fraction of its own format's fold sd — the largest, TEST's −0.0143, is 0.13 of that format's 0.1135. Three formats drifted up and one drifted down, which is what noise looks like. Objective Brier moved the same way and as little (T20 0.2184 → 0.2179, T20I 0.1982 → 0.1954, ODI 0.2264 → 0.2261, TEST 0.2378 → 0.2406) against base-rate Briers that did not move at all in any format.

**This pass is cleaner than batch 1's in one respect worth stating:** batch 1's comparison was confounded by 87 extra matches arriving between the two reports. Here `n_rows` is **21,293 on both sides** and both reports were built from the same 22,905 matches. The only differences between these two runs are the nine PRs' code and the data they rewrote. So the deltas above are attributable to the batch in a way batch 1's were not — and what they say is that the batch did not move discrimination.

**Four things this batch changed that these numbers may reflect** — named, as the runbook asks, without attributing a movement to one unless the mechanism is unambiguous:

- **T20's display model is now fitted the way its grid scored it** (EVAL-01): all 300 chosen iterations on every row, instead of ~117 of 300 on a random 90 %. The expectation going in was that T20 and only T20 would move from this. **The report does not show that.** T20's display AUC moved +0.0017, the *smallest* display movement of the four; T20I's moved +0.0111, the largest, in a format EVAL-01 left bit-identical. Both are inside noise, so the honest conclusion is not "the expectation was wrong" but "this report cannot see the effect": a change worth 0.001–0.01 is invisible against a fold sd of 0.04–0.05. The manifest's `n_iter: 300` is the evidence the fix landed; the AUC is not.
- **The performance members now train on the full history** (EVAL-03), not 92 days stale — and in the harness too, which matters below.
- **Sides are eleven, not twelve** (FEAT-02) on 1,341 sides; involvement is per XI appearance (FEAT-01).
- **A ball faced and a wicket are redefined** (IMPORT-05, IMPORT-06).

##### The gates: `passed: false`

```
gate H-4: T20I fails its threshold: the share of one-player upgrades that lower the
          objective's P(win) is 0.0230, not under 0.02
```

**H-4 fails in T20I.** It is the only entry in `problems`, and it is a genuine failure, not a registry or plumbing complaint. H-4 is unconditional — it binds in every format the folds scored, not only where something is served — so this is a red gate on a served format, and `make evaluate` exited 1 on it.

The swap-monotonicity numbers across the batch, in raw counts as well as shares:

| fmt | share before → after | upgrades before → after | **violations before → after** |
|---|---|---:|---:|
| T20 | 0.0020 → **0.0099** | 6,107 → 6,050 | **12 → 60** |
| **T20I** | 0.0084 → **0.0230** ❌ | 4,634 → 4,631 | **38 → 103** |
| ODI | 0.0000 → 0.0000 | 5,667 → 5,665 | 0 → 0 |
| TEST | 0.0052 → 0.0052 | 4,835 → 4,807 | 24 → 24 |

This is not noise and should not be read as noise. The number of one-player upgrades barely moved in any format, and in the two T20-family formats the number of upgrades that *lowered* the objective's P(win) **tripled in T20I (38 → 103) and quintupled in T20 (12 → 60)** — while ODI (0) and TEST (24) did not move by a single violation. A shift that large, that one-sided, and that sharply split by format is a change in behaviour, and T20I's crossed the line H-4 exists to hold.

**What can and cannot be said about the cause.** **EVAL-01 is excluded**: H-4 measures the *objective* model, and the separate `display_swap_violation_share` is **0.0000 in every format both before and after**, so the display model's refit cannot be behind it. That leaves the feature-definition changes. **FEAT-01 (#306) is the leading candidate** and is named as a candidate only: it is the one change that alters both the inputs the objective reads and the set of swaps the optimiser proposes — it redefines `exp_balls_faced` / `exp_balls_bowled`, and through `is_bowling_option` and `MIN_BOWLING_BALLS` (12 in T20) it changes which players count as bowlers for `n_bowlers` and for the bowling-cover constraint. That mechanism is format-shaped in a way consistent with T20 and T20I moving while ODI and TEST do not. **It is not established here**, and this pass deliberately did not try to establish it: separating FEAT-01 from FEAT-02, IMPORT-05 and IMPORT-06 needs a run per candidate, which is four more two-hour runs nobody has asked for.

**No threshold was adjusted, and no finding was fixed.** H-4's 0.02 stands as written. This is reported to the master as the batch's one red gate.

##### Every other standing gate

| gate | verdict | on what |
|---|---|---|
| **H-17** (objective AUC ≥ 0.65 where an optimised selection is served) | **passes** | served formats are T20I **0.7670** and ODI **0.6733**, both over the line |
| **H-4** (swap violations < 2 %) | **FAILS** | T20I **0.0230** |
| **specific-vs-typical** (specific XI adds AUC where served) | **passes** | T20I +0.0224, ODI +0.0145, both positive |
| **E5** (lineup-only agreement clears the derived bar where served) | **passes** | T20I 0.5677 vs bar 0.4823; ODI 0.5565 vs 0.4978 |
| **E2** (simulated P(win) within tolerance where served as the headline) | **passes** | no format serves the simulated headline |
| **H-8 / serving parity** | **passes** | `max_abs_difference` **0.0** |
| H-5, H-22, H-2, X-4 | no threshold | declared as informational in the registry |

**The ODI warning in the brief did not materialise, in the direction feared.** ODI was 0.020 above H-17's 0.65 line with a fold sd of 0.067 and could have gone red on noise alone; it went **up**, to 0.6733, and H-17 passes. **TEST fell furthest** (0.6257 → 0.6114) and does not trip H-17 — but only because H-17 is a scoping gate that binds where an optimised selection is served, and TEST serves none. On the number alone TEST is now **0.039 below** H-17's 0.65 line, where batch 1 had it 0.024 below. It is the format's scoping, not its AUC, that keeps it off the problems list, and if TEST were ever proposed for serving this is the figure that would have to move first.

**Two verdicts flipped without failing a gate. Both are recorded because a future pass should not rediscover them as new.**

- **E2 in ODI flipped from "a probability" to "a description only."** Δ Brier (simulated − display) 0.0070 → **0.0114**, against a tolerance of 0.01, over the same 11 folds; `simulated_win_probability_within_tolerance` went `true` → **`false`**. It fails no gate because E2 binds only where the simulated headline is served, and ODI serves the display model. But ODI's simulated P(win) is no longer within tolerance of the model it stands beside, and the E2 registry entry is the place that decision lives.
- **`specific_vs_typical_delta` in TEST went negative**, 0.0021 → **−0.0082**: the specific eleven now scores *worse* than a typical eleven of that side. It fails no gate for the same scoping reason (TEST serves no optimised selection), and at a fold sd of 0.10 it is not distinguishable from zero in either report. Recorded as a sign change, not as a finding.

**Nothing else flipped.** All four E5 verdicts are unchanged: T20 still fails its derived bar and is not served (0.5034 → 0.5043 against a bar that rose 0.5055 → 0.5126, so it fails by slightly more); T20I still passes and is served (0.5818 → 0.5677 against 0.4770 → 0.4823); ODI still passes and is served (0.5597 → 0.5565 against 0.5037 → 0.4978); TEST still clears its bar and is still not served. Pairs scored rose in every format (T20 2,187 → 2,465 the most), which is FEAT-02 and FEAT-01 changing which previous elevens are comparable, not a change of verdict.

##### Serving parity, the leak canary, and the simulator's intervals

**`serving_parity` passes with `max_abs_difference` = 0.0** — 50 matches, 50 win rows, 1,100 player rows, 1,100 performance predictions and 49 simulations compared, zero mismatches. Identical to batch 1. The training path and the serving path compute the same numbers to the last bit, through every one of this batch's redefinitions.

**`leak_canary` is unchanged in kind and almost unchanged in value.** The best single column is still `team_elo_diff` for T20 (0.6614), T20I (0.7526) and ODI (0.6327) — identical to four decimal places, which is expected since team Elo reads match results and none of this batch's changes touch them — and `d_exp_mean_matches` for TEST (0.6380 → 0.6354). The same three test-control suspects appear and no others: `team_h2h` in T20 (0.6534) and T20I (0.7293), `d_pelo_min` in T20I (0.7037 → 0.7032). **No new suspect appeared, and no suspect's AUC moved by more than 0.003.**

**The simulator's interval coverage — and why it means more this time.** EVAL-03 makes the harness fit the served recipe per fold, so these figures describe the system that would actually be served rather than a stale relative of it. That is the first time that is true, and it bears on the open **B-11**:

| fmt | walk-forward `coverage_80` | width | locked |
|---|---|---:|---|
| T20 | 0.7703 → **0.7665** | 84.5 → 82.6 | 0.7451 → 0.7451 |
| T20I | 0.7812 → **0.7898** | 80.8 → 79.3 | — (no locked folds) |
| ODI | 0.7579 → **0.7509** | 149.7 → 148.6 | 0.6190 → **0.6667** |
| TEST | — (no simulated folds) | | |

The nominal figure is 0.80. **Every format's 80 % interval covers less than 80 %, before and after**, on the walk-forward folds (0.75–0.79) and further short on the locked window (T20 0.745, ODI 0.667). The refit did not fix it and did not make it worse: the changes are a few thousandths on a fold sd of 0.06–0.09. What the refit does change is the standing of the number — this is now a measurement of the served recipe, so "the simulator's first-innings intervals are too narrow" is a statement about the real system and not an artefact of the harness fitting something else. ODI's locked coverage rising 0.619 → 0.667 is 2 matches on a window of 21 and should not be read as an improvement.

**One reading note.** The report's `generated_at` (2026-09-14T12:40:27Z) is stamped when the rating pass completes, about seven minutes into the run, not when the JSON is written two hours later. Batch 1's report has the same property. It is not a clock error.

##### Wall clock

| step | command | wall clock | result |
|---|---|---:|---|
| 1 | `make migrate` | **10 s** | 0018 → 0020 |
| 2 | `make cricsheet-import` (22,905 files) | **3 min 1 s** | 0 errors, 28 warnings |
| 3 | `make xi-parity BIRTH_DATES=…` | **5 min 7 s** | **passes**, 21/21 counts |
| 4a | `make retrain CUTOFF=2026-09-14` | **15 min 2 s** | run `20260914T121506Z-da5b6680`, `usable: true` |
| 4b | `make reload` | 1 s | **FAILED — HTTP 409**, run unpublished |
| 5 | `make evaluate` | **2 h 5 min 42 s** | report written, **exit 1**, H-4 fails in T20I |
| | (`make export-birth-dates`, prerequisite to step 3) | 1 s | 6,967 players |
| | **total** | **≈ 2 h 29 min** | |

##### What this batch is accepted on, and what it is not

Parity — the acceptance test at the data layer — **passes on all twenty-one counts**, and every count the four new findings added agrees between the database and the archive at exactly the figure a direct scan of the archive gives. The data moved precisely as the nine PRs predicted: 353,571 wicket records where the store had kept 353,547, 1,362 replacements taken out of the elevens, 202,331 wides out of the batters' ledgers, and 24 sides still over eleven for reasons recorded in advance. `serving_parity` is 0.0. The leak canary found nothing new. Discrimination did not move outside noise in any format, on a comparison that — unlike batch 1's — is not confounded by a changed dataset.

**The batch is not accepted clean.** `make evaluate` exits 1, `gates.passed` is **false**, and **H-4 fails in T20I at 0.0230 against a 0.02 line** with the violation count tripled there and quintupled in T20 while ODI and TEST did not move at all. That is the batch's one red gate, it is a change in behaviour rather than noise, and its cause is not established. Two further verdicts flipped without failing a gate (E2 in ODI, the sign of TEST's specific-vs-typical delta). Separately, **`make reload` failed and the run is unpublished** because the deployed service predates the batch. None of these was fixed or worked around here, and no threshold was touched.

### Batch 3 — the re-import, retrain and harness record

Eleven PRs — **FEAT-14 (#314), FEAT-15 (#315), SERVE-02 + GO-04 (#316), SERVE-01 (#317), DATA-01 (#318), EVAL-05 (#319), EVAL-06 (#320), EVAL-08 (#321), EVAL-09 (#322), EVAL-12 (#323), EVAL-10 (#324)** — four of them retrain-flagged (FEAT-14, FEAT-15, EVAL-09, and EVAL-12 by way of D-6: it made `dataset_digest` a required manifest field, so every run on disk is refused and there was **no loadable run** when this pass began). Per § 1 rule 6 of `docs/AUDIT_FIX_RUNBOOK.md` the batch was landed first and the pipeline run **once** at the end, on main at `926a924d`. This is that record. Steps ran in order: migrate → re-import → `make xi-parity` → `make retrain` → `make reload` → `make serving-parity` → `make evaluate`. Three predictions were recorded by fixers before the run — EVAL-09's, FEAT-15's and EVAL-12's — and each is tested against the numbers below.

#### Step 1 — migrate: nothing to apply

The migration level was verified against the database, not quoted. Before anything ran: `select (select count(*) from match), (select count(*) from player_biography), (select max(version) from schema_migrations)` → **22905 | 13662 | 0020_match_player_replacement.sql**, and the eight import tables read exactly batch 2's after-figures (`ball_event` 11,578,345 · `ball_event_wicket` 353,571 · `match_inning` 50,465 · `match_player` 505,287 · `player` 13,694 · `batting_data` 434,001 · `bowling_data` 299,460 · `fielding_data` 176,576 · 1,362 replacement flags). `make migrate`: `files_total 20, applied 0, skipped 20, seen 20` in 1.5 s — the repo holds twenty migrations and the database had all twenty. No PR in this batch added one.

Two things noted on the way in, neither a change to the data. **`/ops/status`'s `table_stats` are planner estimates, not counts**: it reported `ball_event` 11,643,465 and `ball_event_wicket` 353,022 against exact counts of 11,578,345 and 353,571 — the numbers a reader should not carry into a record. And `output/ml-service/runs/20260920T165052Z-cb30121b/` held a lone `xi_win_T20.joblib` (17:08 UTC today) and no manifest — the remains of a retrain killed part-way, one of this pass's earlier attempts; it was left in place so the loader's handling of a manifest-less directory could be read in step 5.

#### Step 2 — the whole-archive re-import: nothing moved, and that is the answer

`make cricsheet-import` over all **22,905** files, fail-fast on, concurrency 12. Wall clock **3 min 16 s** (17:39:44–17:43:00 UTC, first file to `cricsheet-importer finished successfully`). **Zero errors**; **28 warnings**, the same three kinds as batch 2 with the same counts — 25 × "file name is not a Cricsheet match id, deriving one", 2 × "player named on both teams, omitted from both squads" (1130677, KV Sharma), 1 × "replacement not in the side's info.players, side kept as listed" (1537342, Dambulla Sixers: RMMP Rathnayake) — and 110 `super_over_innings` notes, as in batch 1 and 2.

| | before | after |
|---|---|---|
| `match` / `player_biography` / migration | 22905 / 13662 / `0020_match_player_replacement.sql` | **22905 / 13662 / `0020_match_player_replacement.sql`** |

| table | before | after | delta |
|---|---:|---:|---:|
| `match` | 22,905 | 22,905 | 0 |
| `ball_event` | 11,578,345 | 11,578,345 | 0 |
| `ball_event_wicket` | 353,571 | 353,571 | 0 |
| `match_inning` | 50,465 | 50,465 | 0 |
| `batting_data` | 434,001 | 434,001 | 0 |
| `bowling_data` | 299,460 | 299,460 | 0 |
| `fielding_data` | 176,576 | 176,576 | 0 |
| `match_player` | 505,287 | 505,287 | 0 |
| `match_player` where `is_replacement` | 1,362 | 1,362 | 0 |
| `player` | 13,694 | 13,694 | 0 |
| `player_biography` | 13,662 | 13,662 | 0 |
| `match` with non-null `result` | 1,723 | 1,723 | 0 |
| `sum(batting_data.balls)` | 11,376,015 | 11,376,015 | 0 |

**Not a row moved, in any table.** That is the expected result stated in advance rather than discovered: no PR in this batch touched `go-app/internal/cricsheet` or a migration, so the importer that ran is batch 2's importer over batch 2's archive, and IMPORT-03's idempotence (#295) is what makes a rewrite reproduce the same 11,578,345 deliveries rather than duplicate them. The re-import was still run rather than skipped, because § 1 rule 6 says the pass runs the whole pipeline, and because "nothing changed" is a measurement when it is measured and an assumption when it is not. The one-ball B-17 residue at match 514034 (batch 2, step 2) is still there: `sum(batting_data.balls)` reads 11,376,015 against the rule's 11,376,014.

The three synthetic `player` rows with ids 1–3 (no match or ball rows) are **not** cleared by a re-import — the importer upserts what the archive names and deletes nothing it does not name. Out of scope here, recorded as asked.

#### Step 3 — `make xi-parity`: **passes** on every compared count — and one uncompared count disagrees

`make export-birth-dates` wrote 6,967 players first (the operator note from batches 1 and 2, still required), then `make xi-parity` ran two rating passes: Postgres in 3 min 58 s, the archive in 1 min 24 s, exit 0 in **5 min 23 s** (17:43:30–17:48:53 UTC). Both passes produce the same **21,293** training rows and **468,461** player-match rows — batch 2's figures, unchanged. The archive path logged the same two namesake keys (`119678fd`, `efcb778e`) and the 1537342 replacement warning as batch 2.

| count | postgres | cricsheet |
|---|---:|---:|
| offered_matches / matches_read | 22,905 | 22,905 |
| out_of_scope / unusable_matches | 0 | 0 |
| undecided_matches | 1,612 | 1,612 |
| drawn_or_tied_matches | 1,121 | 1,121 |
| decided_matches_without_deliveries | 0 | 0 |
| runs_not_charged_to_bowler | 234,322 | 234,322 |
| **runs_scored** (EVAL-12, #323 — new, *not compared*) | **9,345,813** | **9,345,815** |
| deliveries_not_faced | 202,331 | 202,331 |
| dismissals | 353,063 | 353,063 |
| namesake_sides / unknown_player_keys | 0 | 0 |
| oversized_squads | 24 | 24 |
| replacement_players | 1,362 | 1,362 |
| player_keys / team_keys | 13,639 / 522 | 13,639 / 522 |
| players_with_birth_date | 6,955 | 6,955 |
| matches_with_stage_label / knockout | 22,101 / 1,420 | 22,101 / 1,420 |
| reconstructible_table / dead_rubber | 16,587 / 2,215 | 16,587 / 2,215 |

`source parity: the database and the archive agree`. Every one of the twenty counts in `_COMPARED_COUNTS` agrees, as do the training-row and player-row totals, and every figure is batch 2's figure — which is the right answer for a re-import that moved no row.

**A new finding, recorded and not fixed — B-17 has claimed its first count.** The table prints twenty-two rows and the check compares twenty. `out_of_scope_matches` is left out on purpose (the two sources filter at different points, and the code says so). `runs_scored` is left out by omission: EVAL-12 (#323) added it to `DataQuality` as the digest's cover for undecided matches, and did not add it to `_COMPARED_COUNTS` — exactly the failure mode `docs/BUG_BACKLOG.md` B-17 describes ("a count added to the dataclass is compared only if someone remembers to add it there — the opposite of what the docstring says"). And it is the one count that disagrees: **postgres 9,345,813, cricsheet 9,345,815**, a difference of **2 runs**, which the check reports as agreement and exit 0. The 2 runs are known cricket: match **514034**'s fourth innings — one delivery, a no-ball off which the batter scored one, `runs.total` 2 in the archive — the innings batch 2's step 2 found `ball_event` does not hold at all (`ball_event_emit.go`'s `if totalLegal == 0 { continue }`). `match_inning` records that innings as 2 runs off 0 legal balls; the rating pass reads `ball_event`, so the Postgres pass is 2 runs short of the archive, and the archive is right. Two consequences worth stating. (1) Parity's verdict is true of the twenty counts it compares and untrue of the cricket: a check whose docstring promises "every comparable count" has, in its first run after a count was added, missed the one that differs. (2) `dataset_digest` folds `runs_scored` in, so a run built from Postgres and a run built from the archive over the same 22,905 files now carry **different digests** — which means `make serving-parity CRICSHEET_DIR=…` against a Postgres-trained run will be refused as "trained on other cricket" until either the one-innings gap is closed or the digest and the parity check agree on what counts. Neither is fixed here. B-17 (the docstring and the tuple) and the 514034 gap (batch 2's "B-17 candidate", the ball-event emitter) are two defects with one visible symptom; both stay open, and this is the evidence the backlog entry lacked — a real count, uncompared, disagreeing.

#### Step 4 — `make retrain`: a run with provenance, and the iteration counts EVAL-09 predicted

`make retrain CUTOFF=2026-09-20` — **10 min 34 s** (17:49:16–17:59:50 UTC; rating pass 3 min 38 s, models 6 min 55 s), exit 0, `data_quality_failures: []`. That is **30 % under batch 2's 15 min 2 s** on the same 22,905 matches, and the whole of the saving is in the performance fits (below).

| | |
|---|---|
| run id | **`20260920T175255Z-71339c52`** |
| cutoff | 2026-09-20 |
| ratings through | 2026-09-09 (13,639 players) |
| `dataset_sha` | `7e301346aa3d…` (batch 2: `501c24882251…`) |
| `dataset_digest` | `{scheme: matches+xi-outcomes+pass-counts/1, matches 21293, player_rows 468461, pass_counts 19, undecided_matches 1612}` |
| `git_sha` | `ebd10a4c1bebcfd61fea3301bc6c451cd8f32c74-dirty` (see below) |
| `source` | `PostgresSource` |
| `library_versions` | python 3.12.14 · scikit-learn 1.5.2 · numpy 1.26.4 · scipy 1.11.4 · pandas 2.1.4 · joblib 1.6.0 |
| `model_params` | objective `C` 0.3 / `max_iter` 3000; display `l2_regularization` 1.0, `min_samples_leaf` 40, `early_stopping` false, `random_state` 0; grid margin 0.002, validation fraction 0.2 |
| training rows | T20 12,130 · ODI 4,995 · TEST 2,095 · T20I 2,073 (batch 2's, unchanged) |
| player-match rows | 468,461 (unchanged) |
| `usable` / `unusable_reasons` | **true** / `{}` |

**The retrain's own log carries D-6 refusing the served run by name**, as EVAL-12 said it would: `ERROR retrain: the served run 20260919T162357Z-85ff133f cannot be read, so no regression comparison is made: run 20260919T162357Z-85ff133f: manifest.json carries no dataset_digest … cannot be loaded. Run make retrain to produce a run that records its provenance.` The regression comparison against the previous run therefore did not happen this once — the first run after a digest change has nothing comparable to read, and the log says so rather than comparing two incomparable shas.

**`git_sha` reads `-dirty` on a tree with no uncommitted change to any tracked file — a new finding, recorded and not fixed.** `git status --porcelain --untracked-files=no` was empty before and after the run; the only entries in `git status --porcelain` were two untracked, unignored directories (`.claude/worktree-notes-archive/`, `.codex/`), neither of which holds code the retrain imported. `runs.py:273` reads `git status --porcelain` whole, so any untracked file anywhere under the checkout marks the run dirty. The suffix's stated purpose — "a clean sha over a dirty tree names code that was never committed" — is right, and an untracked `.py` under `ml/` *would* be such code; but an untracked notes directory is not, and on a developer box with editor state this rule will fire on nearly every run, which turns a warning that should be rare into one that is ignored. The commit itself is real and correct: `ebd10a4c` is this branch's head, whose only difference from `926a924d` is `docs/AUDIT_FINDINGS.md` (65 added lines, this record); the code that ran is main's. Recorded as a candidate fix (`--untracked-files=no`, or a path filter under `ml-service/`), not made here.

**No headline metric, and why (EVAL-05, #319).** Every format reports `objective AUC None, display AUC None` and `n_holdout: 0`, with `format_notes` saying "trained on N rows but not scored: … 0 rows at or after the cutoff". The cutoff is today and the archive ends 2026-09-09, exactly batch 2's situation; EVAL-05 made the manifest headline the served, toss-marginalised score *on the holdout*, and there is no holdout. `usable` is true because nothing disqualified it. The choice-facing numbers come from step 7.

**The grid moved nothing, again.** All four formats kept `max_depth 3, learning_rate 0.04, max_iter 300` on "no candidate beat the incumbent by more than 0.002" with `n_iter: 300`; the inner-split incumbents read T20 0.7402, T20I 0.7555, ODI and TEST as recorded in the manifest.

##### EVAL-09's prediction, tested

EVAL-09 (#322) recorded before the run: per-booster `fit.iterations` move materially — involvement classifiers from ~300 toward 150–240, T20 wickets from ~216 toward 120–145; `iteration_choice` appears in every fit; fit time falls by roughly 40 %; headline performance metrics within fold noise, any visible gain on `p_bats` / `p_bowls`. The first three are testable here (the fourth is a harness question, step 7). Batch 2's figures are the mean over its three seeds' early-stopped members; batch 3's are the one served booster's count, chosen on the most recent tenth of training rows by date.

| fmt | booster | batch 2 | **batch 3** | predicted band | in band? |
|---|---|---:|---:|---|---|
| T20 | `p_bats` | 300 | **276** | 150–240 | no (fell, but less) |
| T20 | `p_bowls` | 300 | **251** | 150–240 | no (fell, but less) |
| T20I | `p_bats` | 300 | **84** | 150–240 | no (fell further) |
| T20I | `p_bowls` | 178 | **117** | 150–240 | no (fell further) |
| ODI | `p_bats` | 300 | **127** | 150–240 | no (fell further) |
| ODI | `p_bowls` | 245 | **176** | 150–240 | **yes** |
| TEST | `p_bats` | 263 | **44** | 150–240 | no (fell further) |
| TEST | `p_bowls` | 172 | **127** | 150–240 | no (fell further) |
| T20 | `wickets` | 155 | **122** | 120–145 | **yes** |

**Direction held in all nine; the band held in two.** Every involvement classifier chose fewer iterations than its early-stopped predecessor ran, which is the mechanism EVAL-09 named (the shuffled tenth was optimistic, so the stop came late); but the fall is smaller than predicted in T20 (276 / 251, the one format with a quarter-million rows) and larger everywhere else — T20I's `p_bats` at 84, TEST's at 44. The T20 wickets count lands inside its band. The "~216" EVAL-09 quoted for T20 wickets was its own one-seed measurement; batch 2's served mean was 155, so the reference the band was set against was not the served number. Read plainly: the prediction was right about *what* would happen and wrong about *how much* in seven of nine cases.

`iteration_choice` **appears in every fit** — four of four — with the cut recorded: T20 chosen on 26,753 rows from 2026-01-14 (fitted on 240,124), T20I 4,576 from 2025-07-22, ODI 11,064 from 2025-08-08, TEST 4,642 from 2025-05-16.

**Fit time fell 41 %, as predicted:** T20 290.9 s → **185.4 s** (−36 %), T20I 91.6 → **54.6** (−40 %), ODI 155.0 → **85.3** (−45 %), TEST 109.3 → **56.1** (−49 %); 646.8 s → 381.4 s over the four formats. `n_train`, `n_calibration`, `train_to` and `calibration_from` are identical to batch 2's in every format, so this is the same fit on the same rows, one member and a choice fit instead of three members fitted twice.

**Something the prediction did not foresee: three q0.1 boosters chose one iteration.** `runs_q0.1`, `balls_faced_q0.1` and `runs_conceded_q0.1` chose **1** in T20, T20I and ODI (and `runs_conceded_q0.1` in TEST); the 0.1-quantile of a player's runs is 0 for most of the eleven, so the pinball loss at that level is minimised by the initial constant and every further tree only hurts on the temporal fold. That is the honest optimum for that loss, not a fault in the choice; but a booster of one tree is a constant, and the interval's lower bound is now a format-wide floor rather than a per-player prediction. At the other end `runs_conceded_q0.5` chose **300 — the ceiling — in T20 and T20I**, so that booster wanted more than `MAX_ITER` allows. Both are new facts about the model this batch ships and neither is EVAL-09's failure; whether the coverage gate notices is step 7's question. These booster names did not exist in batch 2's report (it recorded one count per target, `runs`, `balls_faced`, …), which is P-3's distributional model (`_q0.1/_q0.5/_q0.9`) now recorded per level, so the two columns of the table above are not comparable for the regressors and are not compared.

##### FEAT-15's prediction, tested — **held, to the decimal**

FEAT-15 (#315) re-derived `MIN_BOWLING_BALLS` in the per-appearance unit (3 / 4 / 19 / 40) and recorded before this run that the share of decided sides since 2024 with fewer than five bowling options would return to ~**3.2 / 3.9 / 12.8 / 12.5 %** (T20 / T20I / ODI / TEST) with mean options **6.65 / 6.14 / 5.55 / 5.39**. The repo's own test reads a 150-side sample per format; this pass measured the whole population instead — every decided side since 2024-01-01 in one rating pass over the re-imported database, `t1_n_bowlers` and `t2_n_bowlers` on the training frame, the same `is_bowling_option` the objective reads:

| fmt | decided sides since 2024 | share under five | predicted | mean options | predicted |
|---|---:|---:|---:|---:|---:|
| T20 | 8,932 | **3.20 %** | 3.2 % | **6.653** | 6.65 |
| T20I | 888 | **3.94 %** | 3.9 % | **6.135** | 6.14 |
| ODI | 2,264 | **12.81 %** | 12.8 % | **5.551** | 5.55 |
| TEST | 904 | **12.50 %** | 12.5 % | **5.386** | 5.39 |

Every figure and every side count is the one FEAT-15 recorded, which is the expected outcome for a threshold derived from this population, on this population, after a re-import that moved no row — the prediction was a consistency check on the derivation surviving the pipeline, and it did. Against batch 2's numbers in the new unit (25.1 / 19.1 / 38.2 / 21.5 % under five; 5.03 / 5.11 / 4.64 / 5.03 mean) the served objective now reads sides that carry five bowlers as carrying five bowlers. The share under *four* — the level at which the optimiser's bowling-cover constraint has no eleven to reshape into — reads 1.4 / 1.0 / 3.5 / 4.2 %.

#### Step 5 — `make reload`: the new run serves, and every older run is refused by name

**The stack had to be rebuilt first, and the rebuild is a story of its own.** The running ML image (built 2026-09-19 16:18 UTC) predated EVAL-10 and EVAL-12 — its `runs.py` had no `dataset_digest` at all — so it was still serving `85ff133f` and could not have refused anything. Batch 2 declined to rebuild because it was not in the checkout the stack was built from; this pass was, so the rebuild was in scope. Three things then went wrong before any image existed, each recorded here because the next operator will meet them:

1. **`make build-apps` hung for six minutes on `resolve image config for docker-image://docker.io/docker/dockerfile:1`**, and `docker pull` hung the same way while `curl` reached the registry and a container on the VM reached PyPI. The cause was the CLI, not the network: `~/.docker/config.json` names `credsStore: desktop`, and every `docker build` and `docker pull` spawns `docker-credential-desktop list` before it sends a byte; in this headless session each spawn hung forever (three were found stuck, one per attempt), while the same helper run directly with a closed stdin returned `{}` at once. Pointing `DOCKER_CONFIG` at a scratch directory with `{"auths":{}}` and `DOCKER_HOST` at the daemon's socket bypassed the helper and the build ran in 1 min 30 s.
2. **There is no `.dockerignore` anywhere in the repository, and `docker-compose.yml` builds the ML image with `context: .`** — the repo root, under which `data/` weighs **22 GB** and `output/` **3.3 GB**. The legacy builder spent three minutes tarring that context into the daemon before it was stopped; BuildKit sends it lazily but still walks it, and the daemon's build cache stood at 24.98 GB (19.44 reclaimable) when measured. The Dockerfile copies five paths, all under `ml-service/`; a context holding just those is **1.0 MB**. **New finding (OPS), recorded and not fixed:** add a root `.dockerignore` excluding `data/`, `output/`, `.venv/`, `node_modules/` and `.git/`, or narrow the context.
3. `go-app/Dockerfile.api` builds `FROM golang:1.26-alpine`, which is **not in the local image cache**, so the API image could not have been rebuilt without a working pull path.

The images actually serving were then built and recreated by the operator directly (both containers created 18:22:15 UTC; the ML container runs `9c6ad26b…` with `GIT_SHA=acbc0fa0…`, this branch's head at the time, and its `runs.py` carries the digest code); the slim-context image built here (`eed100d3…`) landed on the `umayangag/cric-app-ml:latest` tag thirty seconds later and is not the one running — cosmetic, noted so nobody reads the tag as the container.

**Before reload, on the rebuilt service:** `/health` → `loaded: false, run_id: null`, `error: "run 20260919T162357Z-85ff133f: manifest.json carries no dataset_digest, so nothing says what its dataset_sha is a digest of; it was written before the digest could see a squad or a delivery (EVAL-12) and cannot be loaded. Run make retrain …"`. That is the state the brief described — **no loadable run** — observed live: `current` still named `85ff133f` and the service refused it by name on start-up.

**`make reload`** (no `RUN=`, so the newest run on disk) at 18:23:28 UTC → `{"status":"reloaded","loaded":true,"formats":["ODI","T20","T20I","TEST"],"performance_formats":[…same…],"players":13639,"ratings_through":"2026-09-09", …}`. After: `/health` → `loaded: true, run_id: 20260920T175255Z-71339c52`, ratings fresh (11 days old against a 14-day limit); `current_run.json` → `71339c52`; go-api's `/ops/status` → `current_run` and `loaded_run` both `71339c52`, `ml_health: true`. **A served run is restored.**

**Every older run is refused by name**, in `/artifacts/status`'s listing, each with its reason:

| run | refused for |
|---|---|
| `20260919T162357Z-85ff133f` (was serving) | no `dataset_digest` (EVAL-12) |
| `20260914T121506Z-da5b6680` (batch 2) | no `dataset_digest` |
| `20260913T142341Z-ab4caa13` (batch 1) | no `dataset_digest` |
| `20260908T051956Z-626dc507`, `20260907T062657Z-6b16045e` | no `dataset_digest` |
| `20260906T083819Z-36689f80`, `20260903T160602Z-0e1e39c2`, `20260903T154222Z-4e009a52`, `20260902T163535Z-739b9d62`, `20260902T102135Z-2818a6b7` | no `ratings_through` (P2-2, #280 — refused before this batch too) |

Ten of twelve directories refused, each naming itself and the field it lacks; none silently skipped.

**The twelfth directory is an orphan run, and it loads.** `20260920T165052Z-cb30121b` — noted in step 1 as a manifest-less directory holding one file — was by 17:46:58 UTC a complete run: every artifact, a manifest, `usable: true`, `git_sha 76e7a894…-dirty` (this branch's step-2 commit), `dataset_sha` **identical** to this pass's run (`7e301346…`). Its run id is 16:50:52 UTC, before this session existed: it is the detached retrain of an earlier attempt at this pass, which outlived the session that launched it and ran through this pass's re-import and parity steps unnoticed. Three things follow. It touched nothing — a retrain reads the database and writes only its own directory — and its digest equalling this pass's says the re-import it overlapped produced the same cricket row for row. It was not published: `reload` with no `RUN=` takes the newest run, which is this pass's. And it is a free determinism check — two runs of the same code on the same data — reported under step 6 below.

##### EVAL-12's prediction, tested — **held**

EVAL-12 (#323) recorded that the new run would carry a populated `dataset_digest` and a real `git_sha`, would load, and that old runs would stay refused by name. `dataset_digest` = `{scheme: matches+xi-outcomes+pass-counts/1, matches 21293, player_rows 468461, pass_counts 19, undecided_matches 1612}` with `dataset_sha 7e301346…` — a sha the old formula could not have produced, and one that differs from the `501c2488…` every run since batch 1 carried on the same 22,905 fixtures. `git_sha` = `ebd10a4c…`, a real commit, this branch's head when the manifest was written — with the `-dirty` suffix earned by two untracked directories (step 4's finding), which is the one blemish on this prediction: the sha is real and right, and the suffix is true to the rule and untrue to the tree. The run loaded on the first `reload`. Ten older runs are refused by name, above. Held.

#### Step 6 — `make serving-parity` (EVAL-10, #324): **1.11e-16, passed**

The run `current` names (`71339c52`), loaded through `XiStore.load` the way `reload` loads it, against a fresh pass over the database: **50 matches, 1,100 player rows, 50 served probabilities, 50 from the loaded artifact, 1,100 performance predictions, 49 simulations, max abs difference 1.11e-16, passed**, exit 0. Wall clock **7 min 4 s** (18:23:57–18:31:01 UTC; rating pass 3 min 35 s, second pass and comparisons 3 min 27 s) against EVAL-10's 7 min 58 s on the old run. The 1.11e-16 is half the 2.22e-16 EVAL-10 measured — the same one-ULP `team_h2h` asymmetry it explained, on a different set of last-50 matches — and it sits eight orders of magnitude under the 1e-9 tolerance. The run's digest equalled the pass's, so the check was not refused as "trained on other cricket"; a run built from the archive would have been (step 3).

##### A free determinism check, and a new finding: T20's served performance model depends on the physical order of `match_player`

The orphan run of step 5 (`cb30121b`) and this pass's run (`71339c52`) were built by the same code on the same cricket — identical `dataset_sha`, identical `state_shape`, identical grid scores to six decimals in all four formats (T20 0.740196, T20I 0.755528, ODI 0.670620, TEST 0.618198). Their performance boosters chose **identical iteration counts in T20I, ODI and TEST — and different ones in T20**: `p_bats` 201 vs 276, `p_bowls` 265 vs 251, `catches` 189 vs 255, `balls_faced_q0.9` 297 vs 175, `runs_q0.5` 196 vs 238, four more by smaller amounts. Two runs that should be one model are two models in one format.

The mechanism fits three facts exactly. (1) `RANDOM_STATE = 0` is fixed and, as `performance.py:96-99` says, reaches "only … the binning subsample above 200,000 rows" — and T20 is the one format above that line (`n_fit` 240,124 for the choice, 266,877 served; the next largest is ODI at 98,824). Under a fixed seed the subsample is a fixed set of *row indices*, so which rows define the bin thresholds depends on the order the rows arrive in. (2) The Postgres source's XI query (`sources.py:684-688`, `SELECT … FROM match_player mp … WHERE mp.match_id = %s`) has **no `ORDER BY`**, so a side's eleven — and therefore the eleven player-match rows per side in the performance frame — arrive in physical row order; the matches themselves are ordered `(match_date, match_id)` and are not the problem. (3) The orphan's rating pass read `match_player` before and during this pass's re-import, which rewrote every row of it (IMPORT-03's delete-and-reinsert); this pass read it afterwards. Same rows, different physical order, different rows in the binning subsample, different fit, different argmin over a flat loss curve. The digest cannot see it because `runs.dataset_sha` sorts its lines before hashing — order-independence is its design, and here it is why two runs with one digest are not one model. Below 200,000 rows every row defines the bins and the fit is order-invariant, which is why the other three formats agree to the iteration.

This is the same species as the ball-order defect P-3 fixed in `ball_event` (the comment at `sources.py:699-701` records that ordering by `ball_seq` "left their relative order to the query planner"), now on the squad query. It is not a harness or gate question — parity compares a run against a pass in one process, where the order is whatever it is on both sides — and it is not a batch-3 regression: EVAL-02 (#309) recorded the binning draw as the one thing a seed does not fix, and the order sensitivity was always beneath it. **Recorded, not fixed.** Candidate fix: `ORDER BY mp.id` (or the batting order the importer knows) on the XI query, and the archive path's equivalent (`sources.py:623` sorts matches, not the eleven), so that the frame's row order is a function of the cricket; then a second run on the same digest would reproduce T20's counts too. Until then a T20 retrain is not reproducible to the iteration, and any T20 performance number quoted between two runs of identical code carries this as unmeasured noise.

#### Step 7 — `make evaluate`: the batch's acceptance test — **every gate passes**, and batch 2's red gate is green

**1 h 19 min 49 s** (18:31:40–19:51:29 UTC), launched detached and polled; report written in full, `make evaluate` **exited 0**. That is **36 % under batch 2's 2 h 5 min 42 s** and under the ~2 h 10 min every doc now quotes: T20's eleven folds took 33 minutes against batch 2's 52, and the saving is EVAL-09's — one performance member and a choice fit per fold instead of three early-stopped members fitted twice. The same 11 cutoffs, the same locked window (from 2026-09-02, rotated on 2026-09-02), `n_rows` **21,293** and `n_player_rows` **468,461** on both sides, so — as in batch 2 — the two reports were built from the same matches and the deltas are the batch's code and nothing else.

##### The gates: `passed: true`

```
"gates": {"passed": true, "problems": []}
```

Every one of the 30 registry entries stands as it did in batch 2 (no gate added, none removed). The ten with a threshold, each with its number:

| gate | threshold | batch 2 | **batch 3** | result |
|---|---|---|---|---|
| **H-4** swap monotonicity | share of one-player upgrades that lower P(win) **< 0.02** in every format the folds scored | T20 0.0099 · **T20I 0.0230 ❌** · ODI 0.0000 · TEST 0.0052 | **T20 0.0000 · T20I 0.0000 · ODI 0.0000 · TEST 0.0000** | **passes** |
| H-17 objective ranks | mean walk-forward objective AUC ≥ 0.65 where an optimised selection is served (T20I, ODI) | T20I 0.7670 · ODI 0.6733 | **T20I 0.7659 · ODI 0.6744** | passes |
| specific-vs-typical | AUC(specific) − AUC(typical) > 0 where served | T20I +0.0224 · ODI +0.0145 | **T20I +0.0209 · ODI +0.0144** | passes |
| E5 lineup-only | `passes_derived_bar` where served | T20I 0.5677 vs 0.4691 · ODI 0.5565 vs 0.4964 | **T20I 0.5633 vs 0.4691 · ODI 0.5565 vs 0.4964** | passes |
| E2 simulated P(win) | within 0.01 Brier of the display where the simulation is the headline (it is not, in any format) | — | T20 +0.0014 · T20I +0.0064 · **ODI +0.0116 (over, not served)** · TEST none | passes (not binding) |
| H-8 serving parity | `passed` is true | 0.0 | **1.11e-16**, 50 matches, 1,100 player rows, 50 served / 50 artifact probabilities, 1,100 performance predictions, 49 simulations | passes |
| H-5 recalibration | `locked.recalibrated_targets` | `[]` | `[]` in every format | passes (nothing recalibrated) |
| H-22 performance | walk-forward performance summary present | present | present, every format | passes |
| H-2 leak canary | `test_control_suspects` | 3 | **the same 3** (T20 `team_h2h` 0.6534 / test 0.5317; T20I `d_pelo_min` 0.7032 / 0.5303; T20I `team_h2h` 0.7293 / 0.5317) — identical to four decimals, as they must be on an unchanged frame | passes |
| X-4 market benchmark | formats block present | present | present (T20 185 of 4,466 matches joined; TEST 0 of 443) | passes |

**H-4 goes from batch 2's one red gate to zero violations in every format**, on the same upgrade counts:

| fmt | share batch 2 → 3 | upgrades | **violations batch 2 → 3** |
|---|---|---:|---:|
| T20 | 0.0099 → **0.0000** | 6,050 → 6,050 | **60 → 0** |
| T20I | 0.0230 ❌ → **0.0000** | 4,631 → 4,631 | **103 → 0** |
| ODI | 0.0000 → 0.0000 | 5,665 → 5,665 | 0 → 0 |
| TEST | 0.0052 → **0.0000** | 4,807 → 4,807 | **24 → 0** |

This is FEAT-14 (#314) doing exactly what its title says — the objective is fitted under the contract's signs, so a one-player upgrade cannot lower P(win) by construction — and it is the mechanism batch 2's record asked for rather than a tolerance: the 0.02 line stands as written, the display swap share stays 0.0000 in every format as it was, and the number of upgrades tested did not move by one. **No threshold was adjusted.**

**Two things in the decisions worth reading, neither a gate.** (1) **T20 misses its E5 bar by 0.0006**: agreement 0.5091 over 2,465 pairs against a derived bar of 0.5097 (batch 2: 0.5043 against 0.5126) — the objective moved toward the bar and the bar moved toward it, and T20 stays scoped off (P-7), as the log says: "optimised selection not served in T20 … fails". A T20 selection is still not served and this pass changes nothing there; it records that the miss is now inside one standard error (0.0101). (2) **ODI's simulated P(win) sits 0.0116 over the display, past the 0.01 tolerance, as it did in batch 2 (0.0114)** — E2 does not bind because the display model is the headline in every format, but the number has now been over the line twice and the simulator is not what it was in batch 2 either: EVAL-08 (#321) now refuses to fit the chase response where the calibration fold holds under 30 complete first innings, and it did so in **T20I (14) and ODI (24)** on the locked window, so "the simulator ships without a shared factor, a chase response or a chase dispersion" there. That is EVAL-08's guard working as written, on real folds, and it is why the ODI E2 delta is unchanged rather than better.

##### The headline

Before is batch 2's run `20260914T121506Z-da5b6680`; after is this run. Walk-forward means over the same 11 folds (10 in T20I), with each format's across-fold standard deviation on the *after* column, and the movement expressed in units of that sd:

| fmt | objective AUC | fold sd | move / sd | display AUC | fold sd | move / sd |
|---|---|---:|---:|---|---:|---:|
| T20 | 0.6980 → **0.6966** (−0.0015) | 0.0385 | **0.04** | 0.7295 → **0.7294** (−0.0001) | 0.0481 | 0.00 |
| T20I | 0.7670 → **0.7659** (−0.0011) | 0.0399 | **0.03** | 0.7618 → **0.7624** (+0.0006) | 0.0485 | 0.01 |
| ODI | 0.6733 → **0.6744** (+0.0011) | 0.0683 | **0.02** | 0.7110 → **0.7074** (−0.0036) | 0.0739 | 0.05 |
| TEST | 0.6114 → **0.6105** (−0.0009) | 0.1041 | **0.01** | 0.6286 → **0.6331** (+0.0046) | 0.0977 | 0.05 |

**No movement exceeds its own fold sd; none reaches a twentieth of it.** The largest objective move is T20's −0.0015 at 0.04 sd; the largest display move is TEST's +0.0046 at 0.05 sd. Objective Brier moved by at most 0.0004 (T20 0.2179 → 0.2183, T20I 0.1954 → 0.1954, ODI 0.2261 → 0.2258, TEST 0.2406 → 0.2408) against base-rate Briers that did not move. Batch 2's fold sds were 0.0386 / 0.0441 / 0.0729 / 0.1135; the brief's "T20I 0.7687" is not in either report — batch 2's T20I objective AUC is 0.7670 in its report and in its record, and that is what the comparison uses. The three things this batch changed in the objective's inputs — FEAT-14's monotone fit, FEAT-15's bowling-option threshold (which moved `n_bowlers` by most of a bowler on a quarter of sides), and nothing else — cost it nothing measurable in discrimination while removing every swap violation. The locked window (52 T20 and 24 ODI matches since 2026-09-02, never used for a choice) reads T20 0.7793 → 0.7881 and ODI 0.7413 → 0.7622, too few matches to be more than consistent.

##### EVAL-09's fourth clause, tested — **held**

The prediction's last clause was that the performance headline metrics move within fold noise, with any visible gain on `p_bats` / `p_bowls`. Walk-forward means of the model's within-match Spearman, pinball loss and 80 % interval coverage, twenty targets across the four formats, each movement in units of its own fold sd: **every one of the sixty movements is inside 0.7 sd, fifty-four of them inside 0.25 sd.** The largest are T20 catches Spearman 0.0379 → 0.1495 (+0.68 sd, a target whose ranking signal was near zero), T20I balls-faced coverage 0.8827 → 0.8895 (+0.64 sd) and T20I runs-conceded Spearman 0.8199 → 0.8250 (+0.53 sd); everything on runs, balls faced and wickets is within 0.1 sd in every format (T20 runs Spearman 0.5440 → 0.5431, pinball 2.9184 → 2.9188, coverage 0.8973 → 0.8965). The q0.1 boosters that chose one iteration in step 4 did not move the interval's coverage: it reads 0.90 / 0.90 / 0.95 / 0.91 / 0.96 on T20's five targets as it did. The harness reports no `p_bats` / `p_bowls` headline of its own, so the "visible gain" clause has nothing to be read against; the nearest thing, the involvement-conditioned Spearman on runs, reads 0.3401 → 0.3389 (T20), 0.3595 → 0.3596 (T20I), 0.3523 → 0.3522 (ODI), 0.4367 → 0.4385 (TEST) — flat. Within fold noise, as predicted; the gain, if any, is not visible here.

##### What this batch is accepted on, and what it is not

Parity passes on every compared count; the run loads, serves and round-trips at 1.11e-16; **`gates.passed` is true with no problem listed, and H-4 — batch 2's red gate — is at zero violations in every format** on the same upgrades. The headline discrimination is unchanged to within a twentieth of a fold sd in every format. Of the three predictions, **FEAT-15's held to the decimal, EVAL-12's held, and EVAL-09's held in direction and in fit time (−41 %) and in the performance metrics, but not in the size of the iteration fall** (one of eight involvement classifiers inside its 150–240 band; T20's fell less than predicted, the other three formats' fell further). **The batch is accepted.**

Five things it is *not* accepted clean of, none fixed here: (1) `runs_scored`, added by EVAL-12, is not in `_COMPARED_COUNTS` and is the one count that disagrees between the sources — by the 2 runs of 514034's missing fourth innings — so B-17 and the ball-emitter gap now have a live symptom, and a Postgres-built run and an archive-built run of the same files carry different digests; (2) `git_sha` reads `-dirty` on untracked notes directories; (3) T20's served performance model is not reproducible across runs of the same code on the same digest because the XI query has no `ORDER BY` and T20 alone sits above sklearn's 200,000-row binning subsample; (4) there is no `.dockerignore`, the ML image's build context is the 25 GB repo root, and on this box every `docker build`/`pull` hangs on the `desktop` credential helper; (5) three q0.1 boosters are one-tree constants and two q0.5 boosters hit the 300 ceiling. Beside them, an orphan run `cb30121b` from an earlier attempt sits on disk, loadable and unpublished. The one-ball B-17 residue, the E2 ODI overshoot and the T20 E5 miss carry over unchanged.

Closing count triple, 19:53 UTC: **22905 | 13662 | 0020_match_player_replacement.sql** — the database this pass began with.

### EVAL-03 — Served performance model never trains on the last 92 days  **High · retrain**

`performance.py:542-556`:
```python
hold_out = bool(spec.recalibrate) or spec.shared_factor      # True in production (SHARED_FACTOR = True, simulator.py:62)
fit_rows, calibration_rows = _temporal_calibration_split(rows) if hold_out else ...
members = [_fit_member(x, fit_rows, spec, seed) for seed in spec.seeds]
```
The calibration fold is needed to fit the shared factor, but the members are never refitted on the full history afterwards. Served quantiles are ~3 months stale relative to the ratings they read. The harness has the same structure so it cannot see the cost.

**Fix.** Fit the shared factor / recalibration on the temporal fold, then refit the members on all rows and attach the fold-fitted parts (standard calibrate-on-fold, refit-on-full); or cross-fit over two folds. — PR #311

### EVAL-01 — Display model early stopping is `'auto'`  **High · retrain**

`train.py:89-98` builds `HistGradientBoostingClassifier(...)` with no `early_stopping` argument. In 1.5.2 `'auto'` enables early stopping iff `n_samples > 10000`, with a **random, shuffled 10 %** validation split. T20 has >10k rows; the others do not. So: `choose_display_params` fits candidates on the inner 80 % (<10k → early stopping off, full `max_iter`) then `train_format` refits the winner on all rows (>10k → early stopping on, far fewer iterations). The grid scores a model that is not the one fitted; `max_iter` in `DISPLAY_GRID` is not honoured for T20; walk-forward folds cross the 10k line mid-sequence so early and late folds fit different regimes; 10 % of the most recent rows are dropped from the final fit.

**Fix.** Pass `early_stopping=False` explicitly (the grid already controls `max_iter`), or `early_stopping=True` everywhere with a deterministic temporal split built by hand. Record `n_iter_` in the manifest. Test: assert `model.n_iter_ == max_iter` after `train_format` on a >10k-row synthetic frame. — PR #310. Route taken: `early_stopping=False`, explicit, in `make_display_model` — the one call site EVAL-02 left, which the harness shares. The grid already fixes the iteration count and is the pipeline's whole regularisation choice, so the model it scored is now the model fitted, `max_iter` is honoured in every format, folds no longer change regime across 10,000 rows, and the most recent 10 % of rows are no longer held back from the final fit. The hand-built temporal split was not taken: it would keep a second split, built not to leak, to choose a number the grid already chooses, and the random split *is* the defect. The spec held against the code: measured on a 10,200-row synthetic frame under the unmodified constructor, 117 iterations of the chosen 300 with `early_stopping='auto'`. `n_iter` — the iterations the served model ran — is recorded in the train report and the run manifest beside the grid's choice, equal to `max_iter` by construction. Tests pin the finding's own assertion (`n_iter_ == max_iter` after `train_format` on 10,200 training rows; on main it reads 117 ≠ 300), the setting on the constructor, and `n_iter` in the manifest. Retrain-flagged: T20 is the one format above the line, so its served display model changes and its display AUC is expected to move; EVAL-04's gates read it. T20I, ODI and TEST fit bit-identically. Not run here.

### EVAL-02 — Three seeds produce bit-identical models below 10k rows  **High**

`train.py:231, 244-246`; `evaluate.py:191, 203-205`; `gates.py:243-245, 264-266, 319-321`. `random_state` in HGB only affects binning subsampling (n > 200k) and the early-stopping split. With early stopping off (T20I, ODI, TEST) the three seeds are identical, `display_auc_seed_sd = 0.0`, and gates X-3, B-7-display-monotone and X-2 that require a change to exceed "the control's seed-to-seed sd" have a zero floor that never binds. For T20 the "seed spread" is just the spread of the random early-stopping split.

**Fix.** Either drop the seed loop, or make seeds do something (row bagging via bootstrap `sample_weight`), and base the noise floor on fold-level paired SE only. — PR #309. Route taken: the seed loop is dropped. The display model is one fit under `DISPLAY_RANDOM_STATE = 0` (the seed-0 model every run already saved, so the served artifact is bit-identical); `seeds`, `display.auc_sd`, `display_auc_seed_sd` and `display_auc_seed_sd_mean` are gone from the train report, the harness report, the manifest and the glossary, and the wire keys `display_auc_mean` / `display_brier_mean` are glossaried as the one fit's own score. `gates.py` annotates the six `decides` clauses that named the spread (X-3-stakes, B-7-display-monotone, the four X-2 families): each also required one fold-level paired standard error, which is what bound, so no recorded verdict moves; the eight experiment scripts fit once and their verdicts read the paired SE alone. H-14 is restated as the paired-SE floor (Hanley–McNeil on a single holdout). Confirmed on the model's own settings under sklearn 1.5.2: identical predictions at 2,000 and 9,999 rows, divergent only at 12,000 where early stopping switches on — which is EVAL-01's, and EVAL-01 is now a one-line change. Not retrain-flagged. Bagging was rejected: a bootstrap spread would be non-zero but answers no question the eleven paired folds do not already answer, and would change the served model.

### FEAT-01 — `exp_balls_faced` / `exp_balls_bowled` use the wrong denominator  **High · retrain**

`ml/xi/ratings.py:236-240`
```python
bat_m = self.bat_matches[f, s]
bowl_m = self.bowl_matches[f, s]
"exp_balls_faced": np.where(bat_m > 0, self.bat_balls[f, s] / np.maximum(bat_m, 1e-9), 0.0),
"exp_balls_bowled": np.where(bowl_m > 0, self.bowl_balls[f, s] / np.maximum(bowl_m, 1e-9), 0.0),
```
`bat_matches` / `bowl_matches` are incremented (and decayed) only in `_accumulate` (`ratings.py:515-522`), i.e. only for players who actually faced/bowled a ball. A #11 who batted once for 30 balls in his last 20 matches reads `exp_balls_faced = 30`, the same as an opener who faces 30 every game. `aggregate_side` (`ratings.py:610-611`) weights `bat_rate * ebf`, so tailenders get opener-level involvement in `imp_bat_sum` / `imp_bat_top6`. For bowling, a part-timer who bowled 2 overs once reads `exp_balls_bowled = 12`, exactly `MIN_BOWLING_BALLS["T20"]` (`contract.py:41`), so `is_bowling_option` (`contract.py:44-48`) counts him as a bowler in the `n_bowlers` feature **and** in the optimiser's bowling-cover constraint. The correct denominator `xi_n` already exists and is used for `bat_innings_share` (`ratings.py:251`).

**Fix.** Define involvement per XI appearance: `bat_balls / xi_n` and `bowl_balls / xi_n`, and decay `bat_balls` / `bowl_balls` on every XI appearance (as `bat_pos_sum` / `xi_n` already do, `ratings.py:376-378`) so numerator and denominator share one decay clock. Test: a player in 10 XIs who batted once for 30 balls must read `exp_balls_faced ≈ 3`, not 30. — PR #306. Balls per XI appearance, with the numerator on the appearance clock: two accumulators, `xi_bat_balls` / `xi_bowl_balls`, decay with `xi_n` on every appearance and take the match's deliveries, so `exp_balls_faced` is `per_xi_appearance(xi_bat_balls, xi_n)` and a tailender in ten XIs who batted once reads a decayed tenth of the innings (1.78 if it was the first of the ten, 4.61 if the last), an opener who faces 30 every match reads exactly 30, and a part-timer with one two-over spell reads 12 / 6.51, under `MIN_BOWLING_BALLS` — so the one predicate behind `n_bowlers` and the optimiser's bowling cover stops counting him. Not the finding's letter: `bat_balls` / `bowl_balls` were left on the impact rates' own clock, because they are the denominators of `bat_rate` / `bat_wrate` / `bowl_rate` / `bowl_wrate` and decaying them on appearances without their numerators would have inflated every rate for every player who sat out an innings; a dedicated numerator meets the intent (one clock) without touching the ratings. `bat_matches` / `bowl_matches` had no other reader and are gone, from the state and from `store.PLAYER_ARRAY_NAMES`, so an older artifact is refused by name. The age-band debut prior computes the same quantity for a debutant, so `DEBUT_MATCHES` now counts every debut appearance rather than only those with a ball; `per_xi_appearance` is the one formula in both places and for `bat_innings_share`. Only players named for the match land balls on the accumulators — a batter the deliveries name but no eleven does has no appearance to be divided by. No `xi-parity` count: involvement is derived inside the pass by code both sources share, so the two sources cannot disagree on it. Tests pin the finding's own assertion (first and last of ten), the part-timer through `is_bowling_option`, `count_bowling_options`, `aggregate_side` and `roles_of`, the unnamed batter, and the debut prior over two same-band debutants of whom one batted; all five fail on main. Retrain required: this is an input to selection, not only a column, and the served run's numbers may move either way.

### FEAT-02 — Sides of 12–13 are rated and aggregated as full squads  **Medium · retrain**

`ratings.py:364-366, 372-378, 403-405`; `rows.py:146-149`; `builder.py:147-149` only counts oversized sides (1,358 of 45,468 per `quality.py:70`). Training rows for those matches sum `imp_bat_sum`, `n_bowlers`, `exp_balls_faced_sum`, `n_debutants` over 12–13 players while serving (`store.py:391-392`) always aggregates 11 — an upward bias on ~3 % of rows. A concussion replacement is decided *during* the match, so his presence is post-start information. Player Elo rewards 12 players.

**Fix.** Cricsheet carries `info.replacements`; persist an `is_replacement` flag on `match_player` (importer change) and exclude replacements from `team*_players`; or truncate to the first eleven of `info.players` on both sources and assert parity counts agree. — PR #308. The flag, by decision: it identifies the man, where the first eleven of a list is not necessarily the eleven that started, and batch 2 already re-imports. The survey first, because the finding's citation is wrong in one respect: `info.replacements` is in none of the 22,905 files; the record is delivery-level, `innings[].overs[].deliveries[].replacements.match`, a uniform `{in, out, team, reason}` on all 1,364 entries (916 impact players, 164 concussion substitutes, 140 supersubs, 48 injury, 41 national call-ups, 20 releases, 17 covid, 17 unknown, 1 tactical), beside a `role` list (a substitute finishing an over) that changes nobody's membership. It explains 1,342 of the 1,365 oversized sides (1,343 of twelve, 21 of thirteen, one of fourteen among 45,810); every entry's side is oversized. Migration `0020` adds `match_player.is_replacement`; `cricsheet.Match.ReplacementPlayers` reads the entries in playing order and flags the `in` of each *for the side the entry names* unless he had already gone `out` of an earlier one — the archive's one swap-back (1234909, a covid stand-in who went back out when the man he stood in for returned) is one replacement, not two. `ml/xi/sources.py` applies the same rule on the archive path (`replacement_keys`) and reads the flag on the Postgres path, so both sources hand `MatchRecord` the eleven that started with the replacement named apart in `replacements`; `ratings.py` and `rows.py` need no change, since every site the finding cites reads `team*_players`, and his deliveries stay in `deliveries` so his own ledger still sees what he did. The rule is structural, not a vocabulary, so there is nothing for a `configs/` file; the pass counts `replacement_players` and `make xi-parity` compares it — the add-a-count case, because the flag is the source's data (written in Go, derived in Python), not something shared code derives — and `oversized_squads`, already compared, now means "still over eleven once replacements are out": 24 on the archive, 1,365 on a database until it is re-imported. What this route cannot fix, recorded rather than left quiet: **23 oversized sides carry no replacement entry** (21 Syed Mushtaq Ali Trophy 2022 sides of twelve, 2 Women's T20 Challenge 2018 sides of thirteen), and **1537342** names as the man who came in for Dambulla a player Colombo lists (`RMMP Rathnayake`, registry `afe830a2`, against Dambulla's `P Rathnayake`, `c1fdfb46`) — found by counting the archive with a name-only rule, which would have taken a Colombo starter off, and the reason the side is part of the rule; both languages log it and keep both sides as listed. Tests pin the rule case by case in both languages, the flag through the spy import and the scratch database (nine rows, one flagged, it is the `in`), both source paths yielding eleven, the pass end to end (`t1_n_debutants` 11 not 12, 22 player rows not 23, his Elo untouched and career 0 while a starter's moved), the count, and the parity difference a database that flags nobody makes. Not changed: `db.ReadLastFieldedEleven` (auction P3-2) still requires `COUNT(*) = 11` and skips a side of twelve rather than reading the flag. Re-import and retrain required; the pass's squad query names the column, so the migration must precede its next run over the database.

### FEAT-03 — Decided match with no deliveries emits 22 all-zero player rows  **Medium · retrain**

`builder.py:75-78` builds rows whenever `match.outcome is not None`; `rows.py:34-35` returns an empty actuals dict for empty deliveries and `rows.py:211-213` fills every player from `_ZERO_ACTUALS`. Every player gets `runs=0, balls_faced=0, wickets=0` as a genuine label; the win row carries `innings1_runs = 0`, which E2 then scores simulated totals against.

**Fix.** When `len(match.deliveries) == 0` and the match is decided, emit the win row (the label is real) but no player rows and no innings outcomes; count these in `DataQuality`. — PR #307. The scale first: zero such matches exist on either source — every one of the 22,905 matches in `cricket_data` has an inning-1 row and ball events, and every one of the 22,905 archive files has innings with deliveries — so the fix is a guard against a case that has not yet occurred, and the retrain population is unchanged by it today. The finding's boundary is coherent and is the one implemented: the win row's features all come from the as-of state, none from this match's deliveries, so the row stands without them; `build_match_rows` returns it with the six `INNINGS_OUTCOME_COLS` as `NaN` — unobserved, not nought — and no player rows. `NaN` is what every consumer needs: `simulator.complete_first_innings` never counts it complete, so the calibration fold and the E2 harness leave the match out of the totals check, and `fixtures_from_rows` builds no fixture for a match with no player rows. The pass counts `decided_matches_without_deliveries`; it is gated by the doubling rule (zero today, and one appearing means the importer kept a result and lost its ball events) and, unlike FEAT-01's involvement, it **is** an `xi-parity` count, because it measures what the source handed over rather than what shared code derived: the archive path drops a file with no innings as unusable while the database offers a match with an innings row and no `ball_event` rows, and a database missing one match's balls agrees with the archive on every other count and on the number of training rows. The H-8 rebuild treats two `NaN` outcomes as agreement rather than as a difference `abs(nan - nan)` cannot register. Tests pin the outcomes reading `NaN`, the scenario through `build` (win row with its label, no player rows, count 1), the count against an undecided no-delivery match, the gate on 0 → 1, the parity difference one lost set of ball events makes, and H-8 passing on such a frame (on main it compares 66 player rows, the 22 invented ones among them); all six fail on main. Retrain-flagged: the row builder changed and the baseline gains a count.

### FEAT-14 — The objective is not monotone by construction; H-4 held on FEAT-01's broken denominator and fails in T20I without it  **High · retrain**

`train.py:55` — `make_objective_model` is `make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=3000))`: nothing constrains a coefficient's sign. `contract.monotone_directions` and `_STEM_DIRECTION` (`contract.py:213-234, 465-480`) are read by `make_display_model` only (`train.py:116`). The record says otherwise: `train.py:10-13` and `docs/ml-and-training.md:354-356` call the objective "additive and so monotone in practice (<1 %)", the glossary band for `swap_violation_share` (`glossary.py:433, 444`) says "this system measures under 1 %", and `docs/BUG_BACKLOG.md`'s B-7 row says H-4 is checked on "the objective, whose surface is monotone-constrained on every stem it reads". None of that is in the code. The probe is `selection_metrics.py:93-126`, the gate `gates.py:62, 120-130`, and the surface `/xi/optimize` maximises in T20I and ODI is `optimizer.py:157-172` (`score_many`) over `aggregate_side` (`ratings.py:634-668`). Found by batch 2's acceptance pass (§ 9, *Batch 2*, step 5), which reports the red gate and named FEAT-01 (#306) as a candidate; this entry is the investigation and does not restate that record.

**The failure, established by reproducing the harness.** Rebuilding the frames from `cricket_data` at migration `0020` with batch 2's code (the rating pass alone, 219 s) and refitting the per-fold objective reproduces the report **to the violation in every fold**: T20I 16/407, 15/550, 9/407, 16/550, 9/253, 5/473, 3/451, 4/550, 12/462, 14/528; T20 8, 3, 3, 13, 4, 16, 8, 0, 4, 1, 0 of 550 (the harness skips T20I's 2025-04-01 fold at 5 evaluation rows, where the probe reads 4/55). The rise is spread over the folds, not concentrated: seven of T20I's ten scored folds are over 2 %. On that reproduction:

1. **Every violation is on the Elo axis.** Raising only `pelo` by its population sd lowers the objective's P(win) on **30.8 %** of T20I upgrades (1,444 of 4,686), 4.3 % of T20, 12.5 % of TEST and 0.2 % of ODI; raising only the four impact rates lowers it on **0 of 4,686 / 6,050 / 4,807 / 5,665**. H-4's five-axis upgrade violates exactly when the rate terms are too small to cover the Elo term.
2. **Why: the fitted signs.** With the toss marginalised, a stem's own-side sensitivity is `2·w_d + w_t1 − w_t2` in raw units. In the served T20I objective (run `20260914T121506Z-da5b6680`) that is **−0.016 for `pelo_mean`** (−0.013 in batch 1's run) against +0.017 for `pelo_top3`, +0.004 for `pelo_min` and −0.006 for `pelo_std`, so an Elo upgrade to a player who is neither top-three nor the minimum of his side is *negative* on this surface — the mean per-stem contribution to ΔP is `pelo_mean` −0.0115 on every upgrade and `pelo_top3` +0.023 for the top three, 0 for the rest. Not one of 1,278 upgrades of a side's top-three Elo player violates; at Elo rank 9 the rate is 5.6 %. T20 does the same through `pelo_min` and `pelo_std` (28 of its 60 violations are the side's lowest-rated player, 32 are cold starts). The negative signs sit on collinear pairs the contract marks `+1` on both sides (`imp_bowl_wk` +0.37 against `imp_bowl_wk_top5` −0.18; `pelo_mean` against `pelo_top3`), EVAL-13's collinearity resolved by the fit in whichever direction the rows lean.
3. **What hid it: the broken denominator.** Under batch 1, `exp_balls_*` divided by matches batted (bowled), so anyone who had ever faced or bowled a ball read a full innings' involvement and his upgrade's rate terms covered the Elo term. FEAT-01 makes involvement per XI appearance and the cover goes where involvement is small: T20I's violation share is **16.7 % at ≤ 3 balls per appearance, 13.8 % at 3–9, 3.5 % at 9–18 and 0.6 % above 18**; 3.6 % for players who are not bowling options against 0.7 % for those who are; 22 % for cold starts; the violators' median `exp_balls_bowled` is 0. On the served states the share of T20I players at ≤ 18 balls per appearance rose from 18.8 % to 34.1 % (T20 28.1 % → 46.9 %), 57 of 511 lost `is_bowling_option` (median `exp_balls_bowled` 14.3 → 6.6) and none gained it, and the training rows' `t1_n_bowlers` mean fell 5.95 → 4.82 in T20I, 6.45 → 5.02 in T20, 5.45 → 4.69 in ODI, 5.23 → 4.86 in TEST.
4. **Attribution, by the one experiment that isolates it.** The same pass with **only** FEAT-01's definition reverted — `bat_matches` / `bowl_matches` restored by monkeypatching `RatingState` in the probe process; IMPORT-05, IMPORT-06, FEAT-02, FEAT-03, the rates and the elevens held at batch 2 — and the same per-fold refit:

| fmt | batch 1 (§ 9) | batch 2 (§ 9) | batch 2, involvement reverted | Elo-only share, batch 2 / reverted |
|---|---|---|---|---|
| **T20I** | 38 / 4,634 (0.0084) | **103 / 4,631 (0.0230)** | **38 / 4,686 (0.0081)** | 0.308 / 0.302 |
| T20 | 12 / 6,107 (0.0020) | 60 / 6,050 (0.0099) | 14 / 6,050 (0.0023) | 0.043 / 0.026 |
| ODI | 0 / 5,667 | 0 / 5,665 | 0 / 5,665 | 0.000 / 0.002 |
| TEST | 24 / 4,835 (0.0052) | 24 / 4,807 (0.0052) | 23 / 4,807 (0.0048) | 0.125 / 0.125 |

(The reverted column scores T20I's skipped fold; over the ten the harness scores it is 37.) **The involvement definition alone accounts for the rise, to within one violation per format, and the Elo-only share does not move with it**: the wrong sign is the model's and is there under both definitions; the definition decides how often the rate terms cover it. A cross on the served artifacts agrees in direction — batch 2's model and vectors give 17 violations over the last five windows' 2,464 T20I upgrades, the same with batch 1's involvement swapped in gives 4, and batch 1's model and vectors go from 0 to 8 when batch 2's involvement is swapped in.

**What this means for the record.** FEAT-01 is correct and is not the defect: it removed the accident that satisfied H-4. **H-4's 2 % line was never satisfied for the right reasons** — the objective's monotonicity is empirical, not structural, and the batch-1 figures that passed it (0.2 / 0.8 / 0.0 / 0.5 %, the numbers B-7, P-5 § 8.4 and the glossary band quote) were computed on the broken involvement definition. There is no monotone guarantee on the surface `/xi/optimize` maximises: in T20I, a served format, the hill-climb can prefer the lower-Elo of two otherwise-equal low-involvement players, and the marginal value the Team Lab prints for such a player can carry the wrong sign. The gate is right and the threshold is not negotiable; what it measures is a defect in the objective, not in the probe. **Severity High** rather than Medium because a standing gate is red on a served format, the failure is on the number selection acts on, and it describes the served run's behaviour once the stack is rebuilt and `make reload` accepts the batch-2 run — not a harness artefact. **Ranked 8 in § 1**, first among the open findings and ahead of SERVE-01 / SERVE-02 / DATA-01, because it is the only open finding on which a gate is currently failing and because the batch-2 run cannot be relied on for T20I selection until it lands. Retrain-flagged: any fix changes the served objective. Not established, and not needed for the fix: how much IMPORT-05 / IMPORT-06's rate redefinitions would move the share on their own (the reverted run bounds it at one violation per format).

A separate observation, not this finding's subject: with involvement counted honestly, 20.0 % of T20I sides and 25.8 % of T20 sides since 2024 carry fewer than five bowling options (39.0 % of ODI sides; 13.2 % under four), so `Constraints.min_bowlers` binds on real elevens far more often than the batch-1 counts implied. Whether the constraint's default is right is a question for the optimiser, not for the objective.

**Fix.** Make the objective's monotonicity structural: fit it under the contract's signs — the same `_STEM_DIRECTION` the display model already obeys — for example an L2-regularised logistic regression with non-negative coefficients on direction-signed columns (negate the `−1` stems; give the `0` stems a sign by decision or drop them from the own-side set), fitted with `scipy.optimize.minimize` under bounds, or on a design that folds each stem's `d_`, `t1_` and `t2_` columns into one signed own-side column so a pair like `pelo_mean` / `pelo_top3` cannot be fitted with opposite signs. Resolve the collinear top-k pairs (EVAL-13) in the same change, since they are where the negative signs come from. Do not revert FEAT-01 and do not move the 2 % line. Test: in `tests/test_xi_train.py`, fit `make_objective_model` on a synthetic frame in which `d_pelo_mean` and `d_pelo_top3` are collinear and assert every `+1` stem's own-side sensitivity `2·w_d + w_t1 − w_t2` is ≥ 0 (fails on main); and in the harness, report H-4's probe split by axis — Elo only, rates only — beside the combined share, so the next definition change cannot hide a wrong sign behind a large term again. Whether the axis split also becomes a gate is a decision for the fix. — PR #314. Fitted under the contract's signs: `ml/xi/signed_logistic.py` is an L2 logistic regression — the same loss sklearn's `C=0.3` minimises, and that model to solver tolerance with every sign free — solved by L-BFGS-B under a box per coefficient taken from `contract.monotone_directions`, the directions the display model already obeys. For a `+1` stem `w_d ≥ 0`, `w_t1 ≥ 0`, `w_t2 ≤ 0`, so the own-side sensitivity is non-negative in *each* batting order separately (`w_d + w_t1` batting first, `w_d − w_t2` batting second) — stronger than the marginalised `2·w_d + w_t1 − w_t2` above, which can be positive while one orientation is negative and the two sit at different slopes of the sigmoid. Not the finding's letter on the collinear pairs: EVAL-13's `pelo_mean` / `pelo_top3` and `imp_bowl_wk` / `imp_bowl_wk_top5` stay in, because once both carry the sign the fit cannot resolve them with opposite signs, and how it splits weight between two `+1` stems that both rise under an upgrade is a discrimination question for EVAL-13, not a monotonicity one; the exact `d_` / `t1_` / `t2_` redundancy needs no folding, since box bounds on the triple span exactly the pairs of orientation sensitivities `(a ≥ 0, b ≤ 0)` a folded design would. With every signed stem bounded, `t1_pelo_std` / `t2_pelo_std` — direction 0, moved by every upgrade — were where every remaining violation came from (3 / 0 / 1 / 1 in T20I / ODI / T20 / TEST on the folds; Elo-only still 4.9 % in T20I), so they leave `XI_FEATURE_COLS` as B-7 took them out of the display model, for the reason B-7 gave; the three direction-0 columns left (`d_exp_balls_bowled_top5`, `d_exp_balls_faced_sum`, `d_n_allrounders`) are not moved by the five-axis upgrade, and a test pins that set. Measured by refitting per fold on batch 2's frames (the reproduction above; arm A of the same script reproduces the report's four AUCs and T20I's 103 / 4,631 to the violation): H-4 reads **exactly 0 / 4,631, 0 / 5,665, 0 / 6,050, 0 / 4,807** in T20I / ODI / T20 / TEST — combined, Elo-only and rates-only — and walk-forward objective AUC moves **T20I 0.7670 → 0.7687 (+0.0017 ± 0.0030 se), ODI 0.6733 → 0.6755 (+0.0022 ± 0.0025), T20 0.6980 → 0.6970 (−0.0011 ± 0.0008), TEST 0.6114 → 0.6113 (−0.0001 ± 0.0058)**: the sound surface cost nothing the folds can resolve, and H-17 stays cleared in both served formats. So the question this finding carried — can T20I be optimised — is answered yes: the objective ranks (0.769) and is now monotone by construction, and T20I stays in `OPTIMISED_SELECTION_FORMATS`; nothing was tuned to reach that, and the 2 % line did not move. The harness reports the probe per axis (`swap_violation_share_pelo_only` / `_rates_only`, informing beside the gate) and `train.own_side_sensitivities` exposes the fitted per-stem sensitivities in each batting order. Tests: every signed stem's sensitivity in both orientations on rows engineered to pull `pelo_mean` negative (main's fit reads −1.54 there), the swap probe at exactly zero on that fitted surface (main violates 24 of 220, every one on the Elo axis), the served artifact through `train_format`, the estimator against sklearn with every sign free and against its bounds when the rows pull the other way, and the axis split; the first three fail on main. One fixture changed: `test_xi_retrain_manifest`'s synthetic history had the winner follow a fixed A, A, B cycle, so the rating gap was widest right before the weaker side won and only a fit reading Elo negatively could rank it (main did, at 0.825); the star batter's side wins now and both fits rank it at 1.0. FEAT-01 untouched. Retrain required — the served objective changes in every format, and a run whose `objective_cols` still carry the spread is refused (D-6). The "separate observation" above is now FEAT-15.

### IMPORT-06 — All wicket kinds credited to the bowler; retired-hurt counted as a wicket lost; only first wicket per ball stored  **Medium · retrain**

`ingest.go:399-400, 532-534`: `b.Wickets += len(*d.Wickets)` includes run-outs, retired-out, obstructing, timed-out; `match_inning.wickets_lost` includes `retired hurt` / `retired not out`. `ball_event_emit.go:94-110` keeps only `(*d.Wickets)[0]`. The ML applies `BOWLER_CREDITED_KINDS` (`sources.py:31`) for wickets but treats `retired hurt` as a dismissal (`sources.py:613`, `rows.py:64-68`, `ratings.py:482` MAX_WICKETS test).

**Fix.** Credited-kinds set for `b.Wickets`; exclude retired-hurt from `wkts` and from the ML dismissal target; a child table or `wicket_kind_2 / player_out_2` for the second wicket; normalise kinds at import. — PR #305. The survey first: the archive's 353,571 wicket records are fourteen kinds, every one already lower case and trimmed — 23,064 not the bowler's (22,898 run outs), 508 not a dismissal at all (463 `retired hurt`, 45 `retired not out`; `retired out`, 121, is one) — and 16 deliveries carry more than one wicket, 15 with two and one with **ten** (1483765, a side retiring its whole order out on one ball), 17 records the first-only store lost. One set, not two: `configs/wicket_kinds.json` lists the kinds by what the scorecard makes of them (six the bowler's, six wickets nobody took, two not out) and both languages read it — `internal/wicketkinds` for `cricsheet.Delivery.TallyWickets`, the rule behind `bowling_data.wickets` and `match_inning.wickets_lost`; `ml/xi/wicketkinds.py` for `sources.wicket_columns`, the rule behind `Deliveries.wicket`, `bowler_wicket` and `players_out` on both sources and so the `wickets` and `dismissals` targets. A kind the file does not carry fails the import and the pass rather than being guessed at, which is what "normalise at import" came to once the survey showed no mapping was needed. A child table rather than the column pair, because the one delivery that needed more than two is real cricket: migration `0019` adds `ball_event_wicket` (one row per wicket, in the file's order), moves the first wicket each `ball_event` row held into it and drops `wicket_kind` / `player_out_id`; `_BALLS_SQL` reads the new table as two arrays. `Deliveries.wicket` is the count of dismissals on the ball and `players_out` names everyone dismissed on it, so a run out at the non-striker's end and the second wicket of a delivery count and a retired-hurt batter does not. The parity check could not have seen any of this — the archive path mirrored the first-only store on purpose — so the pass counts `dismissals` and `make xi-parity` compares it. Tests pin each part separately on both sides: the tally by kind, every wicket as a row, an unknown kind failing the file, the bowling and innings figures through the import (spy and scratch database), the two source paths reading one over identically, the targets, and the parity difference a dropped record makes. Not fixed here and recorded as B-16: the batter's wicket-rate ledger still charges the striker with every dismissal on the ball, 11,255 of them at the non-striker's end. Re-import and retrain required; the migration must precede the next rating pass over the database.

### EVAL-04 — No gate can fail in code  **High**

`gates.py:532-553` `check_report` verifies only that a report **path exists** per registered gate; the `decides` text (H-17 "AUC ≥ 0.65", H-4 "< 2 %", E2 tolerance, H-5) is prose nothing evaluates. `evaluate.main` (`evaluate.py:463-465, 617-626`) returns non-zero only for parity/registry problems; `retrain.main` (`retrain.py:276-299`) only on the data-quality gate. `OPTIMISED_SELECTION_FORMATS` (`optimizer.py:57`) and `SIMULATED_WIN_PROBABILITY_DISPLAYED` are hand-set constants, so a fold AUC dropping below 0.65 changes nothing served.

**Fix.** Encode each gate's threshold beside its `report_path` and evaluate it in `check_report`; in retrain, refuse to write a manifest (or write `usable: false`) when holdout objective AUC is below the base-rate equivalent or below the previous accepted run minus a margin. — PR #303. Every standing gate (H-17, H-4, specific-vs-typical, E5, E2, H-8) carries a `Threshold` beside its `report_path` and `check_report` evaluates it, naming the format and the value; the scoping gates are enforced as their contrapositive — a format *served* an optimised selection (or the simulated headline) must carry the evidence, and a format already scoped off is what the prose asks for. H-5 decides an action the harness already applies, H-22 needs the previous release, H-2 / X-4 inform: no threshold, said so in the registry. No encoded clause is seed-relative, so none waits on EVAL-02. Retrain writes `usable` / `unusable_reasons` into the manifest (`ml/xi/run_usability.py`): holdout objective AUC not above 0.5 fails; under the served run's on the **same cutoff** by more than one Hanley–McNeil standard error of the AUC (0.013 T20 / 0.036 T20I / 0.028 ODI / 0.046 TEST on the one scored run) fails; `set_current` refuses to publish, `newest_run_id` skips, `retrain` exits 1. On the batch-1 report every standing gate passes (ODI served at 0.670, 0.020 over the line with a fold sd of 0.067). **Spec gap recorded:** every served run is trained at today's cutoff and has no holdout, so the retrain clauses have nothing to read on cadence and no two runs share a cutoff; giving a retrain a number to read (a throw-away objective on the last quarter) is a separate finding.

### IMPORT-05 — No-balls excluded from batter's balls faced; ML counts wides as faced  **Medium · retrain**

`ingest.go:392-396, 499-504`: the same `legal := wides == 0 && noballs == 0` is used for the bowler's balls (correct) and the batter's (wrong — a no-ball is faced, a wide is not). `batting_data.balls` and `strike_rate` are undercounted. Meanwhile the ML path (`rows.py:28, 41-49`; `ratings.py:481`) counts *every* delivery including wides as faced — a third definition. No test exercises a no-ball, a bye, a run-out or a super over (`grep` across `*_test.go` finds only the wide in `ingest_integration_test.go:38`).

**Fix.** `if wides == 0 { b.Balls++ }` for the batter; add a `faced` boolean to `ball_event` (or document `is_legal` as the bowler's definition); have the ML `balls_faced` exclude wides. Add fixture tests for each extras kind. — PR #302. One rule on both sources, the FEAT-08 shape: `cricsheet.Delivery.IsLegal` (the bowler's count — his balls, the innings' `balls_bowled`, `ball_seq`, `ball_event.is_legal`) and `Delivery.FacedByBatter` (every delivery but a wide) replace the inline condition and the batter is counted by the second; `sources.faced_by_batter` fills a required `Deliveries.faced` on both rating sources (`_BALLS_SQL` reads `extras_wides`, the archive path `extras.wides`) and `balls_faced` in `rows.py` is its sum. No `faced` column: a ball faced is a row with `extras_wides = 0`, which every reader derives by the same rule, and `is_legal` is documented as the bowler's definition (`docs/config-and-data.md`). Every other ball count in the pass — `exp_balls_*`, the `balls_bowled` target, the over's expectation, the simulator's innings length and extras rate — stays a count of deliveries; `balls_bowled` counting every delivery is a sibling defect the finding does not name and the PR does not touch. Parity could not have seen the defect, so the pass counts `deliveries_not_faced` and `make xi-parity` compares it. Tests pin every extras kind on both rules, `is_legal` and `ball_seq` on the emitted rows, and the batting figure through the import (a spy and the scratch database: A1 faces four of five, strike rate 125 not 166.7); on the ML side both source paths, the target, the count and the parity difference. In the archive 58,068 deliveries are no-balls the importer was not counting and 202,331 are wides the rating pass was. Re-import and retrain required.

### FEAT-08 — Bowler's "runs saved" and `runs_conceded` include byes and leg-byes  **Low · retrain (with IMPORT-04)**

`ratings.py:457` (`exp_runs - d.runs_total`), `rows.py:56`. `_BALLS_SQL` (`sources.py:481-496`) does not select `extras_kind` / `runs_extras`. Keeper-quality-correlated noise on `bowl_rate` and on the `runs_conceded` target. **Fix.** After IMPORT-04, subtract byes/leg-byes/penalty in the bowler's ledger; the JSON path has `extras: {byes, legbyes}` per delivery. — PR #300. `_BALLS_SQL` selected neither `extras_kind` nor `runs_extras`, only `runs_batter` and `runs_total`, and nothing that could split the bowler's runs from the keeper's; it now reads `extras_byes / extras_legbyes / extras_penalty` (migration `0018`), and the archive path reads the delivery's `extras` object. `Deliveries.runs_bowler` is derived on both sources by one function, `sources.runs_conceded_by_bowler` (total less byes, leg-byes and penalty) — the rule the importer applies to `bowling_data.runs` — and is what the four ledger sites in `ratings.py` (main, phase, debut, the sequence families' `bowl_saved`) and `runs_conceded` in `rows.py` charge; every innings-level use of `runs_total` (over expectation, extras rate, fixture context, innings outcomes, dot flag) is unchanged. The parity check could not have seen the defect — a source charging the bowler everything agrees with one that does not on every count, the FEAT-04 shape — so the pass counts `runs_not_charged_to_bowler` and `make xi-parity` compares it; it reads 0 on a database imported before `0018`. The query names the new columns, so the migrate step must precede the next rating pass over the database. Retrain required.

### IMPORT-04 — Byes, leg-byes and penalty runs charged to the bowler; breakdown not stored  **High · retrain**

`ingest.go:389, 516-519`: `b.Runs += tr` where `tr = d.Runs.Total`. `ball_event` (`0001_baseline.sql:57-60`) stores `runs_batter`, `runs_extras`, `runs_total` and one `extras_kind` chosen by precedence (`ball_event_emit.go:76-93`), so a no-ball with 4 leg-byes is `extras_kind='no_ball', runs_extras=5` and the leg-byes are unrecoverable. The rating pass computes `runs_conceded = Σ runs_total` (FEAT-08).

**Fix.** Add `extras_wides / noballs / byes / legbyes / penalty smallint` (or `runs_bowler = total − byes − legbyes − penalty`) to `ball_event`; in `ingest.go` compute `b.Runs += tr - byes - legbyes - penalty`. — PR #299. Five columns, not a derived `runs_bowler`: the finding's own complaint is that the leg-byes are unrecoverable, and a derived column loses the same facts; the five counts are what FEAT-08 needs (byes, leg-byes and penalty separable from wides and no-balls) and what makes `extras_kind`'s precedence recoverable. `extras_kind` stays as the lossy summary it was. `Delivery.Extras` is a typed breakdown and `Delivery.RunsConcededByBowler` (total less byes, leg-byes and penalty) is the one rule, applied to `bowling_data.runs` and to the per-over totals a maiden is judged on — the same quantity by over. The archive survey found exactly the five keys, `sum(extras) == runs.extras` on every delivery, 800 no-balls with byes and 306 with leg-byes off them, a largest value of 12 (a penalty), and four deliveries spelling no leg-byes as `legbyes: 0`. `bowling_data.dots` still tests `runs_total == 0` and is left for a decision. Re-import and retrain required.

### IMPORT-01 — Super overs imported as ordinary innings 3 and 4  **Critical · retrain**

`cricsheet.go:143-146` — `Innings{Team, Overs}` decodes neither `super_over` nor `declared`/`forfeited`/`target`; `ingest.go:333-334` numbers every entry `i+1`; `ball_event_emit.go:27-28` likewise. `grep super_over` across `go-app/` and `ml-service/` returns nothing. A tied T20/ODI with a super over writes `match_inning` rows 3 and 4, batting/bowling rows for the six balls, and `ball_event` rows with `innings = 3/4`. The rating pass then counts those deliveries as career balls, runs and dismissals (`rows.py:41-57`), adds them to `ctx_*` over-0 baselines and venue scoring (`ratings.py:466-468, 476-489`), gives a super-over batter who did not bat in the main innings a `batting_position` of 1–3 (`sources.py:63-73`), and emits `innings3_runs` / `innings4_runs` (`rows.py:91-92`). Every super over in the archive (IPL, BBL, CPL, T20Is) is polluted.

**Fix.** Add `SuperOver bool \`json:"super_over"\`` (and `Declared`, `Forfeited`, `Target *struct{Overs, Runs}`) to `Innings`; skip super-over innings in both the aggregate loop and `BuildBallEventRows`, or persist `is_super_over` on `match_inning` / `ball_event` and exclude in `_MATCH_SQL` / `_BALLS_SQL` and the JSON source (`sources.py:216-234`). Test: a fixture file with a super over must produce exactly two `match_inning` rows and no ball_event with `innings > 2`. — PR #296. Skipped at import, not flagged; both write paths and the archive source read one rule (`Match.PlayedInnings`, `ml.xi.sources.played_innings`). `Target.Overs` is a `float64`: 158 innings carry a rain-revised target such as `12.4`, which the proposed integer would have refused. Re-import and retrain required.

### IMPORT-02 — `outcome.result` / `eliminator` / `bowl_out` / `method` not decoded  **High · retrain**

`cricsheet.go:132-142`; no `result` column in `match` (`0001_baseline.sql:475-492`). A super-over win is `outcome: {result: "tie", eliminator: "<team>"}` with no `winner`, so it lands as `outcome_winner_opposition_id = NULL` — the same as "no result", "draw", abandoned and unresolved tie. Those matches are dropped from the frame and never update Elo (`sources.py:106-113` treats NULL winner as excluded, which is correct given the data, but the data is wrong). Drawn Tests get no form update on the Postgres path (FEAT-04). `method` ("D/L") is not stored so no reader can tell which chases were adjusted.

**Fix.** Add `Result`, `Method`, `Eliminator`, `BowlOut` to `Outcome`; migration adding `match.result varchar(16)`, `match.result_method varchar(16)`; set `outcome_winner_opposition_id` from `winner`, else `eliminator`, else `bowl_out`; have `_MATCH_SQL` read `result`. — PR #297. `Outcome.WinningTeam` is the one rule (winner, else eliminator, else bowl_out) and `ml.xi.sources.winning_team` applies it on the archive path, which had the same blind spot. `result_method` is `varchar(32)`, not 16: one file's method is `Lost fewer wickets`, eighteen characters, which the proposed width would have refused whole. `_MATCH_SQL` reads `result` (FEAT-04's first two steps; its parity count set is still FEAT-04's). Whether a tie-breaker win should move Elo like an outright one is a modelling question left open, decidable from the row (`result = 'tie'` beside a winner) without another re-import. Re-import and retrain required.

### FEAT-04 — Postgres source hard-codes `result=None`  **Medium · retrain (with IMPORT-02)**

`sources.py:570-571` sets `result=None`; `ratings.py:406-408` appends 0.5 to `team_results` on `"tie"`/`"draw"`. Roughly a quarter of Tests are draws, so `team_form_diff` (a display-model column) has a different definition on the production source than on the archive path the parity tooling reads. `make xi-parity` compares counts and key sets, not this column.

**Fix.** Persist `outcome.result` (IMPORT-02), read it in `_MATCH_SQL`, add it to the parity count set. — PR #298. IMPORT-02 (#297) had already done the first two steps -- `match.result` persisted by migration `0017` and read by `_MATCH_SQL` -- and this PR did the third: `MatchRecord.drawn_or_tied` names the rule form applies (a draw or a tie nobody broke is half a win each; a tie-breaker win is a win), the rating pass counts it as `DataQuality.drawn_or_tied_matches`, and `make xi-parity` compares the count, which is the first count that can see the column -- a draw is `undecided_matches` whether or not its result is read, which is how both paths disagreed for years with every count equal. The count is compared, not gated, because a re-import of a pre-`0017` database takes it from zero to about a quarter of all Tests. Until that re-import the parity check fails on it, correctly. Retrain required.

### IMPORT-03 — Re-import is not idempotent for `ball_event` and the aggregate tables  **High · re-import**

`repo_ball_event.go:362, 416`: `ON CONFLICT (match_id, innings, "over", ball) DO NOTHING`. `ReplaceMatchPlayersTx` and `DeleteFieldingEventsForMatchTx` (`ingest.go:732-738`, `repo_fielding_event.go:105-110`) implement replace-on-reimport; `ball_event` does not. A corrected Cricsheet file (republished under the same id, `docs/config-and-data.md:457-459`) changes nothing; a match whose innings shrank keeps ghost rows; an importer fix (IMPORT-01, IMPORT-04) never reaches an already-imported match without a manual truncate. `batting_data` / `bowling_data` / `fielding_data` / `match_inning` are `DO UPDATE` so values refresh but vanished rows persist. No FK from `ball_event` to `match` (`0003_match_player.sql:55-57`) is what lets ghosts outlive a match.

**Fix.** `DELETE FROM ball_event WHERE match_id = $1` (and the same for the aggregate tables) at the top of the per-file transaction, mirroring `ReplaceMatchPlayersTx`; drop `ON CONFLICT`. Document that every importer fix requires a re-import of the whole directory. — PR #295

### GO-02 — Track record matches in raw `opposition.id` space; record stores canonical club ids  **High**

`server/prediction_record.go:134-136` stores `result.Team1Side.ClubID` (= `COALESCE(canonical_id, id)`); `db/repo_track_record.go:39-46` matches `$4 IN (SELECT batting_team_opposition_id … UNION SELECT opposition_id FROM match_player …)` in raw ids; `trackrecord/build.go:270` `teamOneWon := *match.WinnerOppositionID == e.stored.Team1OppositionID`; `build.go:307-320` and `scores.go:128` likewise. The pool query (`repo_selection.go:118-122`) folds aliases; the track record does not. Any match whose rows reference the superseded row stays `unresolved` forever; where one side resolves, `team1_won` can be false when team1 won, `Team1Total` stays nil, `Team1BattedFirst` is wrong, `eleven_overlap` reads 0.

**Fix.** In `FindMatches` / `fill`, fold every opposition id through `COALESCE(o.canonical_id, o.id)` so `PlayedMatch` is in club ids. Test: a renamed club fixture must resolve and score. — PR #294

### GO-03 — Forecasts issued after the match date are scored  **High**

`trackrecord/build.go:239` sets `IssuedAfterMatchDate` but `State` still becomes `StateScored`; `scored()` (`build.go:342-350`) selects on `State` only, so they enter `win.overall`, `by_format`, `reliability`, `coverage`, `elevens`. With GO-01 those forecasts were computed from ratings containing the result; `markSuperseded` keys on `IssuedAt` so a post-match re-run is never superseded. **Fix.** Exclude `IssuedAfterMatchDate` from `scored()` (a separate `post_hoc` state), keep `score` on the row. — PR #293

### GO-01 — `as_of` is never sent; historical `match_date` leaks the future  **Critical**

`internal/server/predict_handlers.go:185-208` — `buildPredictInput` never sets `AsOf` (grep confirms only tests and the field definition assign it). `predict_team.go:417` `applyLedger := input.AsOf.IsZero()` is therefore always true, and `predict_team.go:463` / `ml_xi_client.go:204, 239, 397, 495` send no `as_of`, so `xi_service.py:246-263` serves the through-today state. For `POST /api/predict/team-selection` with a past `match_date`: the pool is correctly as-of (`m.match_date < $2`) but `/xi/optimize`, `/xi/predict-win` and `/simulate` are answered from ratings that include the match's own result and everything after it; today's `is_retired` / user flags remove players from a 2019 pool (exactly what H-19 forbids); and the answer is filed in `issued_prediction` and scored by the track record (GO-03).

**Fix.** In `buildPredictInput`, set `input.AsOf = matchDate` when `matchDate` is before today (UTC day) — or add an explicit `as_of` request field — and make `applyLedger` depend on it. Alternatively refuse `match_date < today` on the live endpoint. Test: a handler test asserting the ML client receives `as_of == match_date` for a past date and nothing for a future one. Pair with SERVE-08 so `as_of` cannot bypass freshness. — PR #292

### SERVE-08 — `as_of` bypasses the freshness refusal  **Low-Medium**

`xi_service.py:252-263`: any `as_of > last_date` returns the through-today store with no H-11 check. **Fix.** Apply the check whenever the served state is the through-today one. — PR #292

---

### FEAT-15 — `MIN_BOWLING_BALLS` was calibrated on the old involvement denominator; since FEAT-01 the same number is a much higher bar  **Medium · retrain**

`contract.py:38-40` — `MIN_BOWLING_BALLS = {"T20": 12, "T20I": 12, "ODI": 30, "TEST": 60}`, documented as "expected legal balls bowled **per match** for a player to count as a bowling option", is read by `is_bowling_option` (`contract.py:43-48`), the one predicate behind the `n_bowlers` stem (`ratings.py`, `aggregate_side` — a `+1` stem the objective reads), `n_allrounders`, `roles_of`, and the optimiser's bowling-cover constraint (`optimizer.py`, `Constraints.min_bowlers`). The thresholds were chosen when `exp_balls_bowled` was `bowl_balls / bowl_matches` — balls per match *in which he bowled* — so 12 meant "two overs on the days he bowls". FEAT-01 (#306) redefined it as balls per XI appearance without re-deriving the thresholds, so the same 12 now means "two overs averaged over every match he plays, whether or not he bowled": a frontline bowler who plays ten and bowls 24 balls in five of them drops from 24 to 12 (exactly at the line), and a part-timer with one two-over spell in ten appearances from 12 to 1.2. That is why, with involvement counted honestly, 57 of the 511 T20I players in FEAT-14's probe fixtures lost `is_bowling_option` and none gained it (median `exp_balls_bowled` 14.3 → 6.6), the training rows' `t1_n_bowlers` mean fell 5.95 → 4.82 in T20I, 6.45 → 5.02 in T20, 5.45 → 4.69 in ODI and 5.23 → 4.86 in TEST, and 20.0 % of T20I sides, 25.8 % of T20 sides and 39.0 % of ODI sides since 2024 (13.2 % under four) read as short of five bowling options (all from FEAT-14's investigation, which measured the shift and deliberately did not touch it). Real elevens carry five bowlers; a definition under which a fifth of them do not is measuring the threshold, not the eleven.

Blast radius. (1) `n_bowlers` and `n_allrounders` are objective inputs whose level shifted by most of a bowler; the fitted objective absorbs the level, so no sign is wrong, but every served run since #306 was trained on the shifted columns. (2) The bowling-cover constraint binds on real elevens far more often than the batch-1 counts implied: at its default, `/xi/optimize` will reshape a side a selector would field in T20I and ODI — the two formats where it is served — to admit a "fifth bowler" the eleven already has, and the "why this player" card names a constraint a genuine fifth bowler does not satisfy. (3) The optimiser's `roles_of` and the auction module's role reads inherit the same predicate. None of this is a leak or a wrong sign: it is a bar set in one unit and read in another.

**Fix.** Re-derive `MIN_BOWLING_BALLS` in the new unit from the population, before any outcome is read — for example the balls-per-appearance threshold at which the expected number of qualifying players per real decided eleven is five in each format, or the old thresholds scaled by the format's mean share of appearances in which a bowling option actually bowls — and record the derivation beside the constant, as `AGE_BANDS` does. Change the docstring from "per match" to "per XI appearance" whatever the number becomes. Test: the share of decided sides since 2024 with fewer than five bowling options, per format, returns to the pre-FEAT-01 level (the counts above are the before/after), and a frontline bowler who bowls his full allocation in half his appearances is a bowling option. Do not change the threshold inside another finding's PR. Cross-references: FEAT-01 (#306) changed the unit; FEAT-14 measured the consequence while establishing H-4's mechanism and left it. **Severity Medium, ranked 17 in § 1**: below the Highs because nothing served carries a wrong sign or a leak, above GO-04 because it changes what `/xi/optimize` fields in the two formats where it is served. Tier: Fable — the fix is a definition and a derivation, not plumbing. Retrain-flagged: `n_bowlers` and `n_allrounders` are objective inputs. — PR #315. Translated by prevalence, not by either route the finding offered, because both fail the finding's own test: measured on one rating pass over `cricket_data` at migration 0020 (22,905 matches, 13,662 `player_biography` rows), recording per XI appearance the as-of `exp_balls_bowled` in both units, scaling the old numbers by the share of appearances in which an option bowls (0.83 / 0.85 / 0.91 / 0.95 in T20 / T20I / ODI / TEST → 10 / 10 / 27 / 57) still leaves 15.5 / 12.2 / 28.1 / 20.8 % of decided sides since 2024 under five options, because the players the line decides bowl in about half their appearances while the frontline bowlers who dominate that mean bowl in nearly all of theirs; and the threshold at which the expected count is five (12.1 / 12.7 / 26.3 / 62.1) leaves 23–26 % under five by construction, since the pre-FEAT-01 means were 6.75 / 6.35 / 5.59 / 5.38. What was taken: the old bar admitted 58.4 / 54.2 / 49.7 / 47.5 % of the archive's XI appearances (272,949 / 46,904 / 115,322 / 68,750 with deliveries), and the new bar is the per-appearance quantile that admits the same share — 3.30 / 3.81 / 18.97 / 40.34, rounded to **3 / 4 / 19 / 40**; the agreement-maximising threshold against the old predicate (3.5 / 4.5 / 20.5 / 46) and the since-2024 prevalence (2.5 / 3.1 / 18.6 / 41.9) land beside it. On the 8,932 / 888 / 2,264 / 904 decided sides since 2024 the share under five reads 3.2 / 3.9 / 12.8 / 12.5 % against 4.3 / 4.8 / 17.3 / 15.8 % before FEAT-01 and 25.1 / 19.1 / 38.2 / 21.5 % on the old numbers in the new unit (FEAT-14's 20.0 / 25.8 / 39.0 reproduced), and the mean count 6.65 / 6.14 / 5.55 / 5.39 against 6.75 / 6.35 / 5.59 / 5.38 before and 5.03 / 5.11 / 4.64 / 5.03 on main; the old options still dropped bowl in a median 5–7 % of their appearances in the T20 formats, the one-spell part-timers FEAT-01's own test excludes. The derivation is recorded beside the constant, the docstring says per XI appearance, and `docs/ml-and-training.md` states the predicate. Tests: a frontline bowler who bowls his allocation in half his appearances is a bowling option in every format (fails on main in all four), and `tests/test_bowling_option_population.py` pins the share under five and the mean count per side on a fixture of 150 decided sides per format since 2024 to the pre-FEAT-01 level (fails on main in T20, T20I and ODI; TEST's shift was within tolerance on main). FEAT-01, the objective and `min_bowlers` untouched. Retrain required: `n_bowlers` and `n_allrounders` are objective inputs; joins batch 3.

---

### SERVE-02 — Request models accept any side size, duplicates and overlap  **High**

`app/models/xi.py:39, 148-149, 294`: `team1_player_ids: List[str] = Field(..., min_length=1)` — no max, no uniqueness, no disjointness. Consequences: `simulator.py:585-590` reports 10 wickets once a 6-player side is all out; `/xi/predict-win` on 9 players returns a plausible P(win) for a different question; `/xi/optimize` with a duplicated pool id can choose the same player twice (`optimizer.py:156-157, 202-219` — `key_index` keeps the last position, `_greedy_seed` iterates positions); a player in both `pool_player_ids` and `opponent_player_ids` optimises a side against itself. `XiConstraints.team_size` allows 1–15 (`xi.py:30`) though the models only know 11.

**Fix.** Pydantic validators: exact length (`team_size`, default 11) for win/performance/simulate lists; `len(set(ids)) == len(ids)`; disjointness between sides and between pool and opponent; reject `team_size != 11` unless explicitly supported. Pair with GO-04. — PR #316. Both refusals landed together, because landing only this one would have had a real franchise fixture answered with 422s from a Lab that had done nothing wrong. `app/models/xi.py` carries the rule and `TEAM_SIZE`: `XiWinRequest` — and so `PerformancePredictRequest` and `SimulateRequest`, which inherit it — takes exactly eleven distinct ids a side with the two sides disjoint; `XiOptimizeRequest` takes a distinct pool of at least `constraints.team_size` and, where one is given, an `opponent_player_ids` that is a full distinct eleven disjoint from the pool; `XiConstraints.team_size` is 11 and nothing else. The message names the field and what is wrong with it, and the refusal is a 422 at parse time, before any model is read. The one part of the spec not taken literally: `objective="win"` with an empty opponent list stays `xi_service`'s 503 rather than becoming a validation error, since changing that status was not this finding's to make. Verified on the dev stack against run `20260919T162357Z-85ff133f`: a nine-player `/xi/predict-win` is `200` with `team1_win_probability = 0.576` on main and `422` — "team1_player_ids holds 9 players and a side is scored as 11" — here. Tests: `tests/test_xi_request_models.py`, each refusal on every model that carries two elevens. Not retrain-flagged: what is accepted changed, no feature or label did.

---

### GO-04 — Same player can be in both pools and both XIs  **Medium**

`predict_team.go:429-440` loads pools independently; `xi_selection.go:355-364` `mergeMarginals` assumes "registry ids are unique across sides"; `selection_reason.go:151-161`; `xi_performance.go:75-84` keys one `byKey` for both sides and ignores `PlayerPerformance.side` (`ml_xi_client.go:467-473`); `play_mode.go:110-138` validates pinned XIs per side only. Franchise T20 with a 12-month window: a player who moved clubs is in both pools; alternating best response can select him for both sides. **Fix.** Drop intersection players from the club he played for less recently (by `LastPlayed`), pass the opposing XI as `must_exclude`, validate `team1_xi ∩ team2_xi = ∅`, key performance by `(side, player_id)`. Pair with SERVE-02. — PR #316. Fixed at the root — the two pools are disjoint before anything resolves a must-include id or scores an eleven (`predictteam/shared_players.go`) — and with the spec's resolution rule amended on three points, because as written it silently removes a real player from one side. **(1)** A player the caller named for one side keeps him there whatever `LastPlayed` says: a `must_include` id is a lock B-10 made real and a pinned eleven is the caller's own answer, both better evidence about who he plays for than an appearance record. **(2)** Named for *both* sides — pinned in both elevens, or must-included on both — the request is refused (`400 XI_PLAYER_ON_BOTH_SIDES`) rather than resolved: the caller has said two contradictory things, and correcting one silently is what §8.7 and P1-4 forbid. **(3)** Otherwise the club he appeared for more recently keeps him, and the drop is visible in the losing side's `PoolSummary.excluded` with `reason: "both_sides"` (a declared vocabulary, contract and frontend included), his last appearance for that side, and a detail naming the side that kept him and the date — the shape D-12's ledger exclusions take. An exact tie, including two sides that both entered him by id with no appearance, keeps him with team1; that is arbitrary, stated in the summary, and overridable by picking the candidates by hand, which the detail says. Refusing the fixture outright was rejected: the fixture is real and one of the two clubs is the one he plays for now. Beside the pools: the opposing eleven reaches `/xi/optimize` as `must_exclude`, the two chosen elevens are checked disjoint after selection, `mergeMarginals` and `mergeSelectionReasons` are gone in favour of per-side answers, `/performance/predict` rows are read by `(side, player_id)` (`PlayerPerformance.side` was on the wire and read by nobody), a pool offers one registered person once and two pinned ids that are one registered player are refused, and the auction projection refuses a likely eleven and an opposition that share a player. `predictor.team_size` is retired as a config key. Verified on the dev stack: Joburg Super Kings vs Texas Super Kings (T20, 2026-09-19) share eight players inside the twelve-month window; on main three of them — F du Plessis, D Forrester, RR Rossouw — are in **both elevens** of one answer with nothing saying so, and here each is in exactly one, with all eight named in Joburg's pool summary ("also a candidate for Texas Super Kings (men), whom he played for more recently (2026-07-12)") and that pool 20 → 12. Pinning du Plessis in both elevens answers `P(team1) = 0.644` on main and `400 XI_PLAYER_ON_BOTH_SIDES` here. `must_exclude` changes no answer while the pools are disjoint: the same `/xi/optimize` call with and without it returns the identical eleven, probability and evaluation count. Tests: `shared_players_test.go` (every resolution case, the detail each produces, the refusal, the postcondition), the `must_exclude` payload, the per-side performance rows, the two new 400s, and a scratch-DB case pinning the premise — a player who moved clubs inside the window is in both clubs' pools, with the two dates the resolution decides on. Not retrain-flagged: which side a player lands on changed, no feature or label did.

---

### DATA-01 — Geocoder chooses a candidate in *any* voted country  **High (for the experiments)**

`ml/weather/geocoding.py:111-117`:
```python
rank = {country: i for i, country in enumerate(votes)}
return min(candidates, key=lambda c: (rank.get(c.country_code, len(rank)), -c.population))
```
`votes` is every country any visiting side ever came from, so when no candidate is in the top-voted country, the 7th-ranked one wins, and the `note` at `:141` stays empty because that country *is* somewhere in the votes. Verified in `reference-data/venue-geocoding.csv`: row 435 `M Chinnaswamy Stadium` → Bangalore Town, Sindh, **PK** (24.87, 67.08, Asia/Karachi; 111 cached days); row 684 `Shere Bangla National Stadium` → Mīrpur, Punjab, **IN** (129 days); row 850 `Warner Park, Basseterre` → South Australia (58 days; and `sessions.py:143-145` then applies Australia's 14:00 rule → `night=True`); rows 582/583 `Providence Stadium` → Guyana centroid / Rhode Island; 172 Coolidge → Arizona; 510/512 National Cricket Stadium Grenada → Wales; 797 Three Ws Oval → Jamaica; 876 Windsor Park Roseau → St Lucia; 652 Sabina Park → Jamaica interior; plus Suva → PG, Mantin → NP, Nisshin → KR, Sea Breeze Oval → Ohio, Somerset CC → Alabama. ≈590 venue-days carry another continent's ERA5 hours and clock, concentrated on Mirpur and Bengaluru. `reference-data/README.md:82` ("every row is a city-level fix") and `docs/EXTERNAL_DATA_PLAN.md:1067-1072` are wrong; X-2's gate tables (`EXTERNAL_DATA_PLAN.md:1100-1130`) were computed on this data. `make restore-venue-weather` writes these coordinates onto `venue` (`backfill.py:206-221`).

**Fix.** In `choose`, accept only candidates whose country is among the top-N votes (or has a minimum share), else return `None` so `locate` falls through to the next query / `unmappable`; write the note whenever the chosen country is not the top vote; add a test over the CSV asserting each mapped row's country equals its top vote unless `note` starts `hand-curated`. Re-place the rows above, re-fetch their ERA5 days, rerun X-2 for T20I/ODI. — PR #318. **The spec's number does not exist.** Over the whole table, wrong rows sit at every vote share — 0.02 (Chinnaswamy), 0.31 (Mirpur), 0.71 (Providence, because the CPL votes US) and 1.0 (Three Ws Oval, Warner Park's other spellings, Mantin: tied at the top under the ten-territory West Indies vote or with no vote at all for the true country) — and right rows sit from 0.33 upward (Sheikh Zayed Stadium at 29 to Pakistan's 58 is the same shape as Mirpur at 33 to 105); the spec's own test would fail on ~200 correct rows because every West Indian ground reads `JM` first and Dublin reads `GB IE`. The rule landed is a one-sided sign test at p < 0.05 on the top country's votes against the candidate's, zero votes included (so 15 to none refuses the Bermuda the second query offered for Grenada, while 4 to none does not): a refused candidate is never chosen and the next query is asked; a weaker lead keeps the candidate with `country not the archive's top vote (NA 6 to 2)` on the row; a venue whose every candidate is refused is `unmappable` with the refused places named. The counts are now on every row (`countries_voted` as `BD:105 ZW:37 …`), and `test_curated_table_places_every_row_where_its_votes_allow_or_says_why` asserts the invariant over the whole file. 22 rows moved — Warner Park and Coolidge by the rule, 20 by hand with the reason on the row, including every row named above; four correct rows the rule refuses (Sheikh Zayed, Bulawayo Athletic Club, ICC Academy Ground No 2, Sportpark Het Schootsveld, Deventer) keep their coordinates under a hand note. 602 cached days dropped and fetched again (182 calls, 0 misses; 19,653 venue-days, 22,905 of 22,905 matches); restore re-verified offline on the scratch database. 175 matches change session rule, 30 change the night flag (Warner Park's 21 to day). **Gate (a) re-run on the same code, superseded against corrected coordinates: no family's Δ moved by more than 0.002 AUC and the four nulls stand**; the simulator tables are marked as computed on the superseded coordinates and not re-run. Not retrain-flagged: no model reads a weather column. Found beside it and not fixed: `National Stadium` merges Karachi's with Bermuda's National Sports Centre (four 2010 Americas matches carry Karachi's weather) — a venue-name collision across countries for DATA-02 / IMPORT-08; 26 rows sit at a country centroid (DATA-02); the comment above `frontend/vite.config.ts`'s thresholds says branches "stay at 73" while the value is 74.

### EVAL-05 — Manifest `objective_auc` is toss-known; harness's is marginalised  **Medium**

`train.py:235-236, 243-247` uses `_score(model, x_te, y_te)` on actual batting order; `evaluate.py:192-193, 201-203` uses `_score_marginalised`. Same glossary key, and H-17's criterion reads the harness key. Toss-known is systematically more optimistic than what `/xi/predict-win` serves without `team1_bats_first`. `objective_marginalised` is computed at `train.py:241` but not promoted. **Fix.** Make the manifest headline the marginalised numbers, or add `_toss_known` keys. — PR #319. **Route taken: the marginalised reading is the headline**, because the governing rule is that a reported number is the number that ships, and the shipped number is the marginalised one on *both* paths — `XiStore.objective_probability` (the optimiser's objective) and `/xi/predict-win` without `team1_bats_first` both average the two batting orders; a `_toss_known` sibling would have left a number nothing serves as the headline. The run report now names both readings per model — `objective_marginalised` / `objective_toss_aware`, `display_marginalised` / `display_toss_aware` (the repo's word, from the market benchmark's `display_toss_aware_auc`); nothing in it is called plain `objective` any more — and `retrain.headline_metrics` quotes the marginalised block as `objective_auc` / `display_auc_mean`, the keys the harness writes the same quantity under. **The spec's premise is not borne out.** On the one scored run on record (`20260902T102135Z-2818a6b7`, cutoff 2025-09-01; every later run was trained at today's cutoff and scored nothing) the toss-aware minus marginalised objective AUC reads **T20 −0.0002, T20I +0.0013, ODI −0.0062, TEST −0.0075** — the marginalised reading is the *higher* one in three of four formats, and every gap is inside one Hanley–McNeil standard error of the AUC on that holdout (0.013 / 0.036 / 0.028 / 0.046). So the toss-aware headline was not optimistic; it was a different number under the harness's key, and the defect is the two-meanings key, which is what the fix ends. The display key had the identical defect (`display_auc_mean` toss-aware in the manifest, marginalised in the harness) and is fixed the same way. **Historical manifests:** the one scored manifest predates `ratings_through` and is already refused by the loader, the runs listing and the ops console (P2-2), and the nine cadence manifests carry counts only, so no loadable manifest's headline changed meaning and nothing is backfilled. Readers of `metrics.<fmt>.objective_auc`: the EVAL-04 usability gate's same-cutoff comparison against the served run, the Workbench's headline table via `/xi/status`, and the runs listing; the go-app copies no metric through. **Gates: H-17 reads `walk_forward.summary.objective_auc` from the harness, which was and is marginalised — it reads the same number as before; its threshold is untouched.** The EVAL-04 usability gate now reads the marginalised holdout AUC — the number the manifest quotes and compares against the served run's — instead of the toss-aware one; same two clauses, same threshold, the input is the served reading. Tests: `test_the_manifest_headline_is_the_number_the_harness_reports_under_the_same_key` fits both writers on the same rows and cutoff and asserts the manifest's two keys equal the harness fold's (fails on main, 0.9638 vs 0.9704 on the fixture); the report's block names; and the usability gate judging the served reading while a toss-aware reading under the base rate sits beside it. The parity test pins the display grid to its incumbent because the harness never runs the grid — that is **EVAL-06**, untouched here, and it is the only reason the two writers could still disagree on `display_auc_mean` for a format whose grid picks point 1 or 2. **EVAL-07** appears moot since EVAL-02 (#309): `display_models[0]` no longer exists, the display model is one fit, and the manifest's `display_auc_mean` is that fit's own score. The declared copy of the glossary in `docs/FOLLOW_UP_PLAN.md` § 3 was already stale before this (it still lists `display_auc_seed_sd`, removed by EVAL-02) and is left as the record it is. Not retrain-flagged: no feature, label or model changes; the served artifacts are byte-identical, and the next retrain's manifest carries the served reading under the same keys.

### EVAL-06 — Harness never runs the grid  **Medium**

`evaluate.py:191` → `make_display_model(cols, seed)` → `params=None` → `DISPLAY_GRID[0]`. If retrain picks grid point 1 or 2, the served model has no walk-forward evidence. The inner split is by row fraction (`train.py:116-117`), not a date window. **Fix.** Run `choose_display_params(train)` inside `_evaluate_win_window` and record the per-fold pick; or freeze the grid. — PR #320. **Spec verified:** the line is `evaluate.py:195` today, the claim is exact -- `make_display_model(C.DISPLAY_FEATURE_COLS)` with no params is `DISPLAY_GRID[0]`, whatever `retrain` would have picked. **Route taken: A, the grid runs in every harness window**, through one function both writers fit the display model with (`train.fit_display_model_as_shipped`: grid on the training rows, fit the pick on all of them, record the choice), so `train_format` and `_evaluate_win_window` cannot diverge again; each fold and the locked window carry the record under `hyperparameters`, the manifest's own key, which the glossary already declares a non-metric. **The choice was made on evidence, not convenience.** Across the ten manifests on disk (40 format-picks) the grid picks point 0 in T20, T20I and ODI every time and **TEST picks point 1 in five of ten**, by 0.0026-0.0048 on its inner split, flipping between consecutive days' runs. On the harness's own windows, measured on the full database at `7652a0e8`, **T20I moves off point 0 in nine of twelve windows** (six times to point 2, three to point 1; today's cutoff happens to land inside the margin, which is why no manifest shows it), TEST in one, T20 and ODI in none. So route B (freeze the grid) would not have been nearly free: it would have changed the served recipe in a served format in most windows -- a modelling decision, not a measurement fix -- and the harness is the instrument that decision needs. **Cost, measured:** 177 s over all 48 windows (T20 6.7 s per window, T20I 6.3 s, ODI and TEST under 1 s; taken while a test run shared the cores, so an upper bound), about 2.5 % of the 2 h 05 min run. **What the grid is worth where it moves:** in T20I the pick's out-of-window AUC minus the incumbent's reads +0.012, +0.011, +0.004, 0.000, -0.003, -0.009, -0.009, -0.023 on windows of 23-57 matches (mean -0.002); in TEST's one window +0.019 on 52. Nothing the harness can resolve either way, and `DISPLAY_GRID_MARGIN` = 0.002 sits well under the noise of a ~350-row inner split (Hanley-McNeil SE ~0.03), so in T20I the grid is the drift source the margin was written to prevent. That is a finding about the grid, not about the harness -- **candidate: freeze the grid or raise the margin to the inner split's noise, retrain-flagged, decided on the per-window record the harness now writes** -- and it is left for the coordinator to queue. **Gates:** H-17 reads the objective model's number, untouched; no threshold or criterion changes. What changes is the input to everything that reads a fold's display model in a window whose grid would have moved -- `display_auc`, the market benchmark's `market_minus_display_auc`, the B-7 display swap probe, E2's simulated-minus-display Brier decision -- in T20I and TEST; those numbers now describe the served model, which is the finding's point. **Test:** `test_the_harness_fits_the_display_model_the_grid_would_ship` swaps in a grid whose incumbent is a one-stump model so the grid must move, and asserts the harness window is fitted at the pick, records it under the manifest's key and carries the configuration `train_format` ships; on main it fails with `KeyError: 'hyperparameters'`. EVAL-05's parity test no longer pins the grid to its incumbent: both writers run it on the same rows, so the pin was the only reason they could still disagree on `display_auc_mean`, and its removal is the proof they cannot. Not retrain-flagged: no feature, label or served model changes; only the harness's fits do.

### EVAL-08 — Latent crash in locked-window recalibration; duplicate `np.interp` knots  **Medium**

`perf_calibration.py:54-55` raises below `MIN_ROWS`; `performance.py:558-560` calls `QuantileRecalibration.fit` unguarded; `evaluate.py:340-350` applies fold-decided recalibration to the locked window. A sparse format with a thin 92-day fold aborts the whole `evaluate` run. Separately `knots_x` are bin means of the predicted quantile; q10 predictions are often exactly 0 (`performance.py:361`), producing duplicate `xp` for which `np.interp` is undefined. **Fix.** Guard on `len(calibration_rows) >= MIN_ROWS`; use `IsotonicRegression(increasing=True)`. — PR #321. **Both halves verified against the code** (the line numbers had drifted from `e91c207f`: the unguarded call is `performance.py:566-568`, the locked-window application `evaluate.py:352-366`, the clip at 0 `performance.py:359`).

**(a) Latent, and it has never fired.** `RECALIBRATED_TARGETS` is `()`, so the served path never asks for a correction and the walk-forward folds always pass `recalibrate=()`; only the locked window can request one, and on the batch-2 report `locked.recalibrated_targets` is `[]` in all four formats. **How close it is**, from that report's own `performance.fit.n_calibration` against `MIN_ROWS` = 200: T20 5,786 thinnest / 13,047 locked, **T20I 330** (fold 2025-06-01) — 1.65×, about nine matches of slack — ODI 572 / 4,224, TEST 726. **The guard is a fallback and §8.7 governs it:** the fit's metadata carries `recalibrated` / `recalibration_skipped`, the report carries `locked.recalibration_requested` beside `locked.recalibrated_targets` and `locked.recalibration_skipped`, and `POST /performance/predict` answers with `recalibrated_targets` read off the model that served it, as `SimulateResponse.shared_factor` already does. **H-5's `report_path` now carries what was applied, not what was requested** — today `[]` either way, so no number moves — and whether a skip should *fail* a run is left open deliberately as a new failure mode the prose does not yet state.

**(b) The repeated knots are near-total on real data.** Measured on the locked calibration fold per format and quantile target, repeats out of ten knots: the q10 level reads **9 in ten of the twelve (format, target) cells** — all ten knots collapse to one value, so `np.interp` is undefined across the whole map — with TEST's runs and balls_faced the only exceptions, and two median and three q90 levels also affected. What it returns there, measured: the **last** repeated knot, which after `np.maximum.accumulate` is the **maximum** of the ten per-bin quantiles. The test pins the consequence: 280 rows share a predicted q10 of 0, their pooled 0.1 quantile is 0, and today's code maps every one of them to 100.

**Route taken.** Two changes, because the repeated knot has two causes: a bin is now grown out of whole tie groups (rows the map cannot tell apart are one estimate, and the knots are strictly increasing), and the monotone repair is `IsotonicRegression(increasing=True)` rather than a running maximum — which is a max-envelope, not the isotonic regression the docstring claimed, and resolved every violation upward.

**What moves: nothing that is reported.** Recalibration is applied nowhere, so H-5's list, H-22's widths and coverages and every documented coverage figure are untouched; all forty `xi_perf_*.joblib` on disk load under the new dataclass and none carries a populated calibration. The forced counterfactual — H-5 on for all three quantile targets, both recipes fitted on the first half of each format's locked calibration fold and scored on the second — has **`after` narrower than `before` in eleven of twelve cells** (0.1 %–9.7 %) with coverage moving at most 0.015 and usually back toward the raw model's, which is H-22's direction of progress. TEST's runs and balls_faced are the two cells with no repeated knots and they still move: that is max-envelope versus PAVA on its own. Not retrain-flagged: with `RECALIBRATED_TARGETS` empty the served model's `calibration` is `{}` on both sides, so no feature, label or served model changes.

### EVAL-09 — Performance-model early stopping splits rows of one match across both sides  **Medium · retrain**

`performance.py:83, 264-271`: `early_stopping=True, validation_fraction=0.1` shuffled. Player rows within a match share `own_*`, `opp_*`, `venue_*`, `elo_edge`, so the validation set is near-duplicate of training and the stopping point is optimistic. **Fix.** Group by match (`GroupShuffleSplit`) or temporal; choose `max_iter` on a grouped temporal split and fit with `early_stopping=False`. — PR #322. **The spec held against the code** (the constants had moved to `performance.py:83-85` and the constructors to `266-273`), and it is worse than near-duplicate: **44 of the model's 62 inputs are constant within a side** — the 20 `own_*`, the 20 `opp_*`, `venue_bf_rate`, `venue_n`, `elo_edge` and the innings, with the venue pair constant across the whole match — measured column by column on the archive's training rows in every format. A shuffled tenth is about 2.2 rows per match, so essentially every match sat on both sides of the split (1,833 / 1,814 / 4,272 / 10,324 matches in TEST / T20I / ODI / T20, against 0 under either grouped split) and a validation row's ten match-mates, identical in 71 % of the columns, were training rows.

**Measured** on the archive at `f63bf83e` (22,905 matches at migration 0020; one seed; cutoff 2026-06-01 with June–September 2026 as the honest holdout; eight of the thirteen boosters per format; three validation splits built by hand and stopped on sklearn's own patience rule — the shuffled tenth, a `GroupShuffleSplit` tenth by match, the most recent tenth by date at a match boundary — each fitted to 300 iterations on its own 90 % and read off staged predictions; and one fit on every training row whose staged predictions on the holdout give the honest loss at any count). **The leak by construction:** against a grouped-random tenth of the same population the shuffled tenth reads its own best log loss **35 % / 17 % / 7.7 % / 2.9 %** lower for `p_bats` and 0.4–2.7 % lower for `p_bowls`; on the quantile and count regressors the difference is within one draw's noise (±5 %, mixed sign). **Where it stops:** the shuffled split runs the involvement classifiers to or near the ceiling — sklearn's own fits stop at 273 / 300 / 300 / 300 for `p_bats` — where the honest optimum is 5 / 144 / 160 / 225, and reads T20 `wickets` as still improving at 216 against an honest 144. **What that costs:** stopping on the temporal fold instead improves the held-out loss by **2.1 % / 1.3 % / 0.14 % / 0.17 %** on `p_bats`, up to 0.9 % on `p_bowls` and 0.12 % on T20 wickets, and is a wash (±0.15 %) on every quantile regressor; neither rule sits more than 0.4 % above the honest optimum on the regressors. The defect is real and the fix right in kind; its measurable effect is on the classifiers and small.

**Route taken: temporal, per booster, per fit.** `choose_iterations` fits each booster to `MAX_ITER` on the rows before the most recent tenth of the training rows by date (`ITERATION_CHOICE_FRACTION`, cut at a match boundary) and takes the iteration at which the booster's own loss — log loss, the pinball loss at its level, the Poisson deviance — on the rows after the cut is lowest; the served booster is a fresh fit on every row for exactly that count with `early_stopping=False`. One enumeration (`boosters`) feeds the choice and the fit, so the population a count is chosen on is the one it is fitted on. The count is re-derived by every fit — a retrain re-chooses on the new rows, each harness window on its own — and recorded in the fit's metadata (`fit.iteration_choice`: the cut, the rows on each side, the count per booster) beside the counts the served boosters ran (`fit.iterations`); a history too short to cut runs the ceiling and warns. Grouped-random was not taken: both close the leak, the honest loss at the two stopping points differs by ≤ 0.2 % in every case in both directions, and temporal is how the model is used, what EVAL-01 chose for the win models and what the 92-day calibration fold inside this fit already is. **Consistent with EVAL-01** in principle — the served model runs the count it was scored at, chosen on rows after the ones it was fitted on — and deliberately different in one respect: the display grid already chooses `max_iter`, so EVAL-01 let it stand; the performance model has no grid over iterations, so the count is chosen on the fold rather than frozen. **Forced consequence:** the three members' seeds entered, by the module's own docstring, through the early-stopping split; with it gone `random_state` reaches nothing below 200,000 rows — refits under seeds 0 and 1 are bit-identical in T20I, ODI and TEST, and differ in T20 (267k rows) through sklearn's binning subsample alone (max |Δ| 2.16 runs on the median over 14k holdout rows), binning noise rather than an ensemble — so the model is one fit under `RANDOM_STATE = 0`, `seeds` leaves `FitSpec` and the glossary (EVAL-02's route), and the artifact's `SeedMember` class keeps its name so the served run still loads until batch 3 refits it. Tests: the choice split is temporal and never puts a match on both sides; every served booster carries `early_stopping=False` and ran exactly its chosen count, or the ceiling where the fold was too thin (fails on main); the chosen count is the argmin of a log-loss curve reproduced with sklearn directly; a history too short to cut runs the ceiling and warns. **Retrain required; joins batch 3.** Prediction for the pass: per-booster `fit.iterations` move materially (the involvement classifiers down from ~300 toward 150–240, T20 wickets from ~216 toward 120–145) and `iteration_choice` appears; the performance headline metrics move within fold noise, with any visible gain on `p_bats` / `p_bowls`; fit time falls by roughly 40 % (one member and a 300-iteration choice fit, instead of three early-stopped members fitted twice). Not EVAL-09 and left as observed: TEST's `p_bats` honest optimum at 5 iterations on a 1,034-row holdout says the classifier barely generalises past the base rate and role there over three months.

**Two stale records corrected in the same PR.** "the three display seeds" (stale since EVAL-02, #309) appears six times in `gates.py`, not once, with five more instances of "one fit per arm per fold per seed" for display fits — all corrected, the `decides` clauses' historical parentheticals left as the record they are. The harness runtime was quoted as ~67 minutes (`docs/ml-and-training.md`), ~54 (`CLAUDE.md`) and ~54 twice more (`docs/overview.md`); the batch-2 record measured **2 h 05 min** after EVAL-03's second member fit and EVAL-06 added a measured 177 s, so all four now read **~2 h 10 min** with the reason it moved.

### EVAL-10 — H-8 parity does not cover the saved artifact or `XiStore`  **Medium**

`asof.py:112-230` compares `build_match_rows` against itself from two states. It never round-trips `save_ratings → XiStore.load`, never calls `display_probability` / `win_probability` (which assemble the row via `row.get(c, 0.0)` and `serving_match` with `state.last_date`, `store.py:394-406`), and never exercises the registry-id contract go-app sends. D-6 (pickle shape drift) and B-7 (column-list drift) would pass this check. **Fix.** For the same 50 matches, compare `XiStore.display_probability` on a store loaded from a freshly written run directory against `marginalised_probabilities` on the frame rows. — PR #324. **The spec held, and understated the gap.** The quoted code is where the entry says (`store.py` had shrunk to 355 lines; `row.get(c, 0.0)` and `serving_match(..., self.state.last_date)` are its last twenty), with two corrections: there is no `XiStore.win_probability` — the objective the optimiser maximises is `objective_probability`, and `win_probability` is the optimiser's response field — and the harness's `parity_model` was a `PerformanceModels`, so before this PR H-8 served **no win model at all**: the display and objective artifacts never entered the check in any form, not even in memory.

**What the check is now.** `serving_parity` takes a `store` that has come through `XiStore.load` — in the harness, `round_trip_store` writes the locked window's win and performance models and the pass's final state as a run directory (`save_ratings`, `save_models`, `save_performance`, `write_manifest`) and loads it back, so a run this code cannot serve is refused by name (D-6) before anything is compared. For each of the last 50 matches it then holds three more things to the 1e-9 tolerance beside the rows: the served `display_probability` and `objective_probability` from the store over the as-of state — exactly how `XiRegistry._as_of_store` answers a backtest, the frame's player keys being the registry ids go-app sends — against `marginalised_probabilities` of the artifact's models on the frame's row; the same two numbers from the *loaded* through-today state, as a live request is answered, against the same models over the state the as-of pass ends on; and the performance predictions and simulator draws from the store's own loaded performance artifact. The second comparison exists because the first replaces the loaded state: a payload that does not carry an accumulator the served number reads shows only there. `objective_probability` is covered for the reason the finding names `display_probability` — it is the number `/xi/optimize` maximises, its row is assembled by the same `xi_feature_vector` on both paths, and leaving it out would have left the selection objective the one served number no gate reads. `python -m ml.xi.asof` (`make serving-parity [RUN=]`) runs the same check for a run on disk against a fresh pass over its own data, refusing (exit 2) a run the loader refuses or one whose `dataset_sha` is not the source's — a through-today comparison against other cricket would measure the data, not the artifact.

**Parity holds today on the served run.** `20260919T162357Z-85ff133f` (written at `6021d58d`, before batch 2; `age_aware_cold_start` off) against a fresh pass over the 22,905-match database at migration 0020, whose digest equals the run's: 50 matches, 1,100 player rows, **50 served probabilities, 50 from the loaded artifact**, 1,100 performance predictions, 49 simulations, max abs difference **2.22e-16**, passed, exit 0; 7 min 58 s wall (rating pass 3 m 37 s, sweep and comparisons 4 m 18 s). The 2.22e-16 is one ULP on `team_h2h`: the harness marginalises the swapped orientation as `1 − mean(x)` while the store reads the reversed key's `mean(1 − x)`, which the row-only check (0.0 exactly, then and now) could not see — a bounded rounding asymmetry, not a defect, and it sits seven orders of magnitude under the tolerance.

**Proof the gate is now load-bearing, on the historical shapes.** Three fixtures in `tests/test_xi_asof.py`, each run through the row-only check (the whole of H-8 before this PR) and through the store-aware one. *B-7's shape* — a win artifact whose `display_cols` carry `t1_pelo_std` / `t2_pelo_std`: the row-only check passes it; the store-aware check cannot begin, because `XiStore.load` refuses the run naming it and the columns. *The next B-7, past the loader* — a display column the contract lists and the frame carries but the store's row assembly reads as `row.get(c, 0.0)` (the stakes columns are exactly that, `rows.stakes_columns`; the fixture kept `stakes_knockout` and made every fourth match a final): the loader passes the artifact (its column list is the contract's), the rows agree, and the served display probability differs from the frame's on precisely the knockout matches among the compared ones. *D-6's shape, one level above the loader's list* — `pelo` removed from `STATE_ARRAY_NAMES` / `PLAYER_ARRAY_NAMES` as the writer and loader see them, so the payload does not carry an accumulator the display reads: the loader passes the run, the rows agree, the as-of served probabilities agree, and only the reading from the loaded artifact differs (the objective's sign-bounded fit clamps `pelo` to zero on the synthetic history, so it is the display reading that shows it). Before this PR every one of the three passed H-8.

**What it does not reach.** B-14 (the performance artifact is the one artifact `XiStore.load` does not shape-check) is not caught by a round trip: a fresh round trip writes the current class and restores it whole, and on a run on disk the performance comparison uses the loaded model on both rows, so an old pickle restoring with class defaults agrees with itself. B-14's fix is the load-time check its record already describes, in its own PR. B-17's docstring is in `ml/xi/parity.py`, the H-15 *source* parity (Postgres against the archive), not H-8; it is untouched here and still false. **Cost.** In the harness, one `XiStore.load` of a temp run directory plus 200 predictions on top of the sweep H-8 already pays — seconds against ~2 h 10 min; H-8 is not on the 11-minute cadence and this PR does not put it there, because the as-of sweep is the cost (measured 4 min 18 s here) and `retrain` already runs to the cadence's edge. Not retrain-flagged: no feature, label or artifact definition changed; the served run loads and passes. The report gains `served_probabilities_compared`, `artifact_probabilities_compared` and `run_id` beside the existing counts, and each format's `parity_win_model_window` beside `parity_model_window`.

### EVAL-11 — No untouched holdout  **Medium (validity)**

`evaluate.py:91-103, 116-120`: `LOCKED_START = "2026-09-02"` — days of matches. All ~17 gates (`gates.py`) were decided on the same eleven quarterly folds, so fold means are development-set scores under heavy multiple comparison. **Fix.** Hold back a full season no gate script may read; report it once per release; state beside every fold number how many gates consulted the folds. — PR #326.

**The spec, verified.** The line numbers hold (`LOCKED_START` at `evaluate.py:130` after the docstring grew). "~17 gates" undercounts: the registry holds **30**, and **29 consulted the folds** — every standing gate (H-17, H-4, specific-vs-typical, E5, E2, H-5, H-22, H-2, X-4) and every experiment gate (E3, A-1…A-3, X-1b, X-2 families, X-3, B-7, B-11's SIM-* family); only **H-8** parity reads no fold. On 2026-09-20 the "window" held **52 T20, 24 ODI, 9 TEST, 3 T20I** matches — 19 days, because A-4 moves the line to the date of the last decision, so the window is never older than the time since one.

**What "untouched" means here.** The holdout (the `locked` node, `LOCKED_START` on) is excluded from **gate evaluation and from every harness-fitted model's training** — the locked window's models train on rows before the line under the same cutoff rule every fold obeys — and **not** from the served run's training, which `retrain` fits through today. So a holdout number certifies the *selection procedure* (features, model classes, thresholds, scoping — all chosen on the folds) on data none of those choices consulted; it does not certify the served run's own weights, which its manifest reports at its own cutoff. Holding the season out of the served model too would cost it the freshest season for a certificate about weights the harness never fits, and would make this retrain-flagged. Not taken; **not retrain-flagged**, no served artifact moves.

**Enforced by the code, three ways, each pinned by a test that fails on `main`.** (1) `evaluate_format` hands the fold path a frame that ends at `LOCKED_START` — nothing computed in a fold can reach a holdout row because the rows are not in the frame it is given (`test_the_fold_path_is_handed_no_holdout_row`: on `main` the fold calls receive rows past the line). (2) `fold_windows()` refuses a cutoff at or past the line, so a rotation cannot extend the folds into the holdout and no experiment script — all of which take their windows from it — can obtain a window there (`test_fold_windows_refuse_a_cutoff_inside_the_holdout`: on `main` it returns the window). (3) A `Gate` whose `Threshold` would read the `locked` node, at any depth or scope, is refused in `__post_init__` when the registry is built (`test_a_threshold_that_would_read_the_holdout_is_refused_at_registration`). **The disclosure travels with the number**: every summary over folds — win, performance and E5 alike, through one constructor `ml.xi.folds.summarise_over_folds` — is `{mean, sd, n_folds, gates_consulted}`, and a holdout figure is a bare value under `locked` beside a `holdout` record (`n_matches`, `first_match`, `last_match`, `days_covered`, `season_complete`, `gates_consulted: 0`); each `Gate` declares `consults_folds` and the registry embeds it; the Evaluation tab labels the mean row *development surface — over N folds, read by K gates* and the locked row *holdout — D of 365 season days, incomplete, read by no gate*.

**The season, and where it is too thin.** `LOCKED_SEASON_DAYS = 365` from the line; `season_end` 2027-09-02 in the report. A full season of the archive holds **~1,689 T20, 392 ODI, 185 T20I, 159 TEST** decided matches (2025-09-02 → 2026-09-01). At that size the holdout can decide H-17 in T20I (0.766 against 0.65, Hanley–McNeil SE ≈ 0.04), is marginal in ODI (0.674 against 0.65, SE ≈ 0.03), and **cannot decide E5 anywhere** (≈ 40 T20I and 160 ODI pairs a year against agreement-minus-bar of 0.06–0.09 at SE 0.04–0.08). Today's 19-day window reads T20 0.788 over 52 matches and ODI 0.762 over 24, T20I and TEST unscorable, E5 0.50 over 22 T20 pairs and 0.42 over 12 ODI pairs — all inside the noise of their development numbers. **No gate's honest number is known to differ from its development number, and none can yet be known**; a rotation before 2027-09-02 will leave the record showing a season that never completed, which is the honest outcome and is what the record will say.

**Thresholds, sorted.** *Contaminated (chosen or re-derived with the fold numbers in view):* E5's bar — the a-priori 0.55 was replaced by the derived bar after the folds showed it unreachable, and ODI flipped fail → pass on the replacement (§8.8); H-4's 2 % — the plan's H-4 row states the line beside the measured shares (0.4 % T20 / 3.7 % ODI), now moot since FEAT-14 made violations structurally zero; the format scoping `OPTIMISED_SELECTION_FORMATS` — set by hand from the fold verdicts, by design; FEAT-15's `MIN_BOWLING_BALLS` — derived on the population including the current window's rows. *A priori (stated in the plan before the harness ran):* H-17's 0.65, E2's 0.01 Brier, H-5's ±0.03, E3's 3 % / 30 %, specific-vs-typical's > 0, H-14's 0.002 grid margin.

**No fold number moves.** Every fold already trained before its cutoff and scored inside its window; the partition removes only rows no fold could reach (`specific_vs_typical` walks the frame in date order and scores in-window rows only). `make evaluate` was not run: it would reprint batch 3's figures with the new provenance keys and a holdout that decides nothing.

### SERVE-01 — Every route is `async def` running CPU-bound work inline  **High**

`app/main.py:548-552, 557-564, 569-576, 586-599, 334-356`; `app/xi_service.py:264-271`. `optimize` (up to 200k model evaluations), `simulate` (up to 20k draws × 22 players), `_reload_run` and the as-of sweep (a sequential pass over `ball_event`, under `self._lock`) all run synchronously inside coroutines. Nothing else — including `/health` — is served until they return. A backtest going backwards in date rebuilds the pass from scratch (`asof.py:91-94`) on the loop thread. If the routes are later flipped to `def` (threadpool), the as-of path becomes a genuine race: `store.with_state(state)` hands out the live, mutating `AsOfRatings.state`, and `_read_slots` grows arrays on the read path (`ratings.py:211-213`).

**Fix.** Run prediction functions via `asyncio.to_thread` (or declare the routes `def`), make `store_as_of` snapshot/freeze the state before returning it, and bound the as-of sweep off the request path. — PR #317. Route taken: `def`, which is the one-word version of `to_thread`, on the nine handlers that compute or touch the disk (`/xi/predict-win`, `/performance/predict`, `/xi/player-roles`, `/simulate`, `/xi/optimize`, `/admin/reload`, `/artifacts/status`, `/xi/evaluate-report`, `/admin/train/progress`). `/health`, `/xi/status` and `/xi/metric-glossary` stay on the loop deliberately: they read only what is in memory, and a liveness probe queuing for one of the threadpool's 40 threads behind a burst of predictions is the same outage in a different place. Measured on run `20260919T162357Z-85ff133f`, ten concurrent `/simulate` (20k draws): `/health` was answered **once, after 3,974 ms** on main, and **711 times, median 0.7 ms**, here. The freeze is the half that mattered: `RatingState.snapshot()` copies the arrays, the player index and every table value `update` mutates in place (the form lists are appended to, the venue pair and the ground's scoring sums incremented — a `dict()` copy still moves with the pass), taken inside the lock that already serialises the sweep, one snapshot per as-of date so a fixture's four calls read identical ratings and one copy is in flight (24 MB, ~3.5 ms at 13,569 players), dropped with the pass on reload. The other half of the race is `_read_slots` growing the arrays for the reserved unrated column, so a loaded state and every snapshot reserve it once while still private to one thread. The state's field lists move to `ratings.py` beside the accumulators, so the artifact writer, its D-6 refusal and the snapshot walk one enumeration. **Not taken:** bounding the sweep off the request path — the caller asked for that date and must wait for it either way, so a background pass would be speculative work and another place to go wrong; what stays unbounded is the rebuild-from-scratch when a caller asks backwards, which is logged, returns the correct answer, and is a finding of its own if it ever bites. Not retrain-flagged: an as-of answer is the same answer, read from a copy. **The bill, paid in the same PR:** numpy's BLAS and scikit-learn's OpenMP each start a thread per core inside every call, so once the routes compute on the threadpool, N concurrent requests put N x cores threads on cores — four concurrent simulates went from 1.67 s serialised on main to 7.43 s. `app/serving_compute.py` gives one serving call one thread per library, applied **on the thread that computes**: `omp_set_num_threads` is per-thread, so a limit applied once at import pins the import thread and nothing else (measured: four concurrent simulates 7.59 s, no better than none). Nothing is written to the environment, which is what keeps it away from `retrain` and `evaluate` — they are subprocesses, a child inherits the environment and cannot inherit a per-thread setting, and their HistGradientBoosting genuinely wants every core. Both halves are pinned by a subprocess probe (`inside` reads 1 on a worker thread, `child` reads the machine's 12, no `*_NUM_THREADS` in the environment) and by a test on a real `predict_win`. Every serving path is faster for it, sequentially: optimize 268 -> 192 ms, predict-win 7.5 -> 1.8 ms, performance 265 -> 33 ms, simulate 390 -> 140 ms. Combined with the threadpool move, ten concurrent simulates go **4.17 s -> 1.06 s** while `/health` goes **4,110 ms (one answer) -> 2.6 ms median over 32**. `threadpoolctl` was already installed as scikit-learn's dependency and is now named in `requirements.in`; `requirements.txt` was regenerated by pip-compile and only its `# via` comment changed.

---

## 10. Verified OK — what was checked and found sound

So the next audit does not repeat it. Line references are to `e91c207f`.

- **Day-close within a date.** `builder.py:68-84` buffers every match on a date and applies the day after building rows; out-of-order input raises; `asof.py:66` folds strictly `< as_of`; `serving_parity` (`asof.py:160-205`) checks the last 50 matches column-for-column (but see EVAL-10 for what it does not cover).
- **Reads never precede their own update** in the rating pass (`ratings.py:428-430` vs `:466-468`; `:368` vs `:372`; `.get` defaults at `:302-316, :339-340`); unknown players map to a reserved never-written slot (`:201-213`).
- **Targets are not features** (`contract.py:276-343, 364-373`; `train.py:122-140`).
- **team1 = side batting first** on both sources (`sources.py:81, 269, 432-433`), not home or toss winner; serving marginalises the toss exactly as training's `swap_orientation` (`train.py:148-165` ≡ `store.py:394-406`); NULL winner is excluded, never a loss (`sources.py:106-113`; `trackrecord/build.go:265-268`).
- **Fold boundaries** `< cutoff` for training, `[cutoff, end)` for evaluation (`evaluate.py:174-175`; `train.py:216, 288`; `perf_harness.py:137, 183-187`); the rating pass is genuinely as-of, not refit per fold; hyperparameter choice uses an inner temporal split (`train.py:115-117`); recalibration and E2/E5 decisions read the folds, never the locked window.
- **Odds never enter training** (`market.py` imported only by `evaluate.py`; both arms scored on the same joined subset).
- **Degenerate-class guards** on every AUC; coverage semantics strict/inclusive per end; career baselines shifted so a row never sees itself; leak canary computed on development windows only; simulator/bootstrap seeds fixed.
- **Biography:** only date of birth crosses into `ml`; retirement / `career_end_date` is read only by the serving-time ledger (`repo_player_status.go:82-96`; `availability/criteria.go:30, 79`).
- **Stakes:** dead-rubber (which reads the future fixture list) is deliberately not a frame column (`contract.py:313-316`).
- **Weather is not served** — nothing under `ml/xi` or `app` imports `ml.weather`.
- **Match identity** from the file name with a bounded, logged hash fallback; **player identity** via `registry.people` on both sides, `external_id` nullable-unique; biography joins by `external_id → key_cricinfo`, never name; **team identity** `(name, gender)` with validated lineage; display names settled deterministically; squads replaced not upserted; fielding events deleted before insert; bowler's legal-ball count and `ball_seq` exclude wides/no-balls; playing order read as `(innings, over, ball)`; all timestamps `timestamptz`; indexes on the ML hot path present; importer concurrency (errgroup, per-file tx, sync.Map, mutexes) race-free.
- **go-app auth** constant-time compare, key never logged, fails closed with 503 when unset; ml-service `hmac.compare_digest`; CORS single origin; every query parameterised; `rows.Close` / `rows.Err` present; pipeline `{step}` validated against the registry; SSE stream exits on ctx done; `StopMLTraining` uses `WithoutCancel`; predictions paged and capped.
- **Registry-id contract (D-7a)** honoured on every payload; JSON field names/types line up with `models/xi.py` except GO-07's omission and GO-04's ignored `side`.
- **ml-service serving:** feature column order asserted equal to the contract at load (`store.py:200-213, 369, 395-397`); P(A,B) = 1 − P(B,A); reload is one reference swap with refusal leaving the old run serving; optimiser deterministic with named constraint conflicts; wickets ≤ 10, balls truncated at innings length, chase ends exactly at target, spread shares sum to 1, median band falls back to all draws.
- **Reference files** internally clean (unique keys, no nulls, one tz per venue); Cricsheet downloads registered by SHA-256; Python deps fully pinned; coverage thresholds agree across the three places per component; no `continue-on-error`; pipeline order `retrain → reload` and cadence reload only after every earlier step succeeds; frontend displays `win_probability.team1` under `team1` with no client-side rescaling.
- **EVAL-07 (Served display artifact is seed 0, headline is the seed mean) is moot since EVAL-02 (PR #309, `b552dd9c`).** Verified against the code at `7652a0e8`, not asserted: `display_models` occurs nowhere under `ml/` or `app/`; `train_format` fits one display model (`fit_display_model_as_shipped`, one `HistGradientBoostingClassifier.fit` under `DISPLAY_RANDOM_STATE`); `FormatModels(display=<that object>)` is what `save_models` writes to `xi_win_<FMT>.joblib`; and the report's `display_marginalised` -- the manifest's `display_auc_mean` since EVAL-05 (#319) -- is `_score_marginalised` of that same object. There is no seed mean and no seed-0 member: the served artifact is the fit the headline scores. Nothing to fix; recorded here in PR #320.
