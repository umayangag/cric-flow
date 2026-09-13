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
| 8 | GO-02 | High | Track record joins fixtures and winners in raw `opposition.id` space while the record stores canonical club ids — renamed clubs never resolve or score the wrong way | no |
| 9 | GO-03 | High | Forecasts issued after the match date are scored into Brier/coverage — with GO-01 those are leaked forecasts | no |
| 10 | IMPORT-02 | High | `outcome.result` / `eliminator` / `method` not decoded: super-over winners stored as "no winner", ties indistinguishable from no-results, drawn Tests get no form update on the Postgres path | yes |
| 11 | IMPORT-03 | High | Re-import is not idempotent for `ball_event` (`ON CONFLICT DO NOTHING`): corrected Cricsheet files and importer fixes never reach already-imported matches | no (but re-import) |
| 12 | IMPORT-04 | High | Byes, leg-byes and penalty runs are charged to the bowler; `ball_event` cannot carry the breakdown | yes |
| 13 | SERVE-01 | High | Every route is `async def` running CPU-bound work inline; the event loop (and `/health`) blocks for the whole optimise/simulate/as-of sweep | no |
| 14 | SERVE-02 | High | Request models accept sides of any size, duplicate ids and the same player on both sides; downstream silently assumes 11 distinct | no |
| 15 | DATA-01 | High | Geocoder picks a candidate in *any* voted country: Chinnaswamy → Pakistan, Shere Bangla → Punjab IN, Warner Park → South Australia, Providence → Rhode Island (~590 venue-days of wrong weather) | no (experiments only) |
| 16 | GO-04 | Medium | The same player can be in both pools and both XIs; merges keyed by registry id clobber one side | no |
| 17 | IMPORT-05 | Medium | No-balls are excluded from the batter's balls faced (`batting_data`), and the ML path counts wides as faced — two definitions | yes |
| 18 | IMPORT-06 | Medium | Every wicket kind credited to the bowler in `bowling_data`; retired-hurt counted as a wicket lost; only first wicket per ball reaches `ball_event` | partial |
| 19 | FEAT-02 | Medium | Sides of 12–13 (concussion subs) are rated and aggregated as full squads — train/serve mismatch on ~3 % of rows and a mild post-start leak | yes |
| 20 | FEAT-03 | Medium | A decided match with no `ball_event` rows emits 22 player rows with all-zero targets | yes |
| 21 | EVAL-05 | Medium | Manifest `objective_auc` is toss-known; harness's is marginalised — same glossary key, different quantity | no |
| 22 | EVAL-06 | Medium | Harness never runs the grid; walk-forward numbers describe grid point 0 only | no |
| 23 | EVAL-07 | Medium | Served display artifact is seed 0; headline is the seed mean | no |
| 24 | EVAL-08 | Medium | Latent crash in locked-window recalibration when the 92-day fold is thin; duplicate knots in `np.interp` | no |
| 25 | EVAL-09 | Medium | Performance-model early stopping uses a random split that puts rows of one match on both sides | yes |
| 26 | EVAL-10 | Medium | H-8 parity compares `rows.py` with `rows.py`; the saved artifact and `XiStore` serving path are outside it | no |
| 27 | EVAL-11 | Medium | Locked window is days old and every gate was decided on the same eleven folds — no untouched holdout | no |
| 28 | EVAL-12 | Medium | Manifest cannot reproduce a run: `git_sha` empty in the container, no library versions, `dataset_sha` blind to squad/delivery edits | no |
| 29 | SERVE-03 | Medium | `RATINGS_STALE` measures last *match* date, not data currency; refuses fresh runs in an off-season, global not per format | no |
| 30 | SERVE-04 | Medium | Serving fixtures are stamped with `state.last_date`, not the fixture date; no `match_date`/`gender` in the request | no |
| 31 | SERVE-05 | Medium | Simulator batting depth uses one shared uniform vs per-player `p_bats`; realised P(bats) contradicts L2-B when `p_bats` is not monotone in order | no |
| 32 | SERVE-06 | Medium | Training process registry keyed by module — concurrent runs overwrite each other's handle; stop races natural exit | no |
| 33 | GO-05 | Medium | Cancelled run plan can never record its outcome (cancelled ctx used for `Finish`) — row stuck `IN_PROGRESS`, every later plan 409 | no |
| 34 | GO-06 | Medium | Lane lock is check-then-insert; a second job in the same lane overwrites the first's cancel func | no |
| 35 | GO-07 | Medium | `/xi/predict-win` and `/performance/predict` are never told the toss (Go structs omit `team1_bats_first`) | no |
| 36 | GO-08 | Medium | Venue lookup failure is swallowed; prediction silently runs with no venue | no |
| 37 | GO-09 | Medium | `match_date` with a UTC offset is truncated in UTC; pool cutoff and stored date disagree and can include the fixture itself | no |
| 38 | IMPORT-07 | Medium | Fail-fast abort skips display-name settlement and team lineage — renamed clubs stay split | no |
| 39 | IMPORT-08 | Medium | Venue identity is the raw string; `normalized_name` never populated; city used as venue when blank | yes |
| 40 | IMPORT-09 | Medium | Format taxonomy merges MDM with TEST, ODM with ODI, men with women; T20I inferred from a hand list instead of `info.team_type` | yes (design) |
| 41 | IMPORT-10 | Medium | `-apply` (dry-run) flag is parsed and ignored; every run writes | no |
| 42 | IMPORT-11 | Medium | `match_inning.target_runs` ignores `innings[].target` (D/L) and is set on Test second innings | no |
| 43 | FEAT-04 | Medium | Postgres source hard-codes `result=None`; JSON path reads it — `team_form_diff` has two definitions | yes (with IMPORT-02) |
| 44 | FEAT-05 | Medium | No home-advantage and no toss feature although `venue.country` and `toss_*` are in the DB | yes |
| 45 | FEAT-06 | Medium | Ratings siloed per format; every T20⇄T20I / ODI⇄List-A crossover is a cold start shrunk toward zero | yes |
| 46 | DATA-02 | Medium | Venue-key normalisation does not merge spellings of one ground; 20 rows are country centroids | no |
| 47 | DATA-03 | Medium | Competition needle `"zimbabwe"` votes ZW for every venue Zimbabwe toured; other needles unanchored | no |
| 48 | DATA-04 | Medium | ERA5 hours indexed by local-time string with one fixed offset per call across DST transitions | no |
| 49 | DATA-05 | Medium | `wx_rain_prior_day_mm` sums match-day rain up to the *inferred* start — in-match rain leaks when the norm is late | no |
| 50 | OPS-01 | Medium | `reference-data/**`, `configs/**`, `scripts/**`, `contracts/**` trigger no CI | no |
| 51 | OPS-02 | Medium | `dev-local-key` admin default baked into compose, three Makefiles and `cadence.sh`; Postgres and both APIs bound to all interfaces; watcher holds the Docker socket | no |
| 52–79 | various | Low | See § 2–8 | — |

### 1b. Which model does the fix

Three tiers, chosen by how much the fix depends on judgment about *what the right behaviour is* rather than on executing a spec. The rule: if getting it slightly wrong would silently change a served number or a training label, use Fable; if the spec in this doc is complete and the risk is engineering (races, SQL, plumbing), use Opus; if it is mechanical, use Sonnet. A fixer may escalate one tier if the file turns out harder than described, never de-escalate.

**Fable (`claude-fable-5-1`)** — ML semantics, leakage, feature definitions, anything marked **retrain**, and the two Critical leaks. The fixer has to read the rating pass or harness deeply, decide the definition, and prove it with a targeted test and a before/after on the harness.
`GO-01`, `IMPORT-01`, `IMPORT-02`, `IMPORT-04`, `IMPORT-05`, `IMPORT-06`, `IMPORT-09`, `FEAT-01` … `FEAT-13`, `EVAL-01`, `EVAL-02`, `EVAL-03`, `EVAL-04`, `EVAL-05`, `EVAL-06`, `EVAL-07`, `EVAL-09`, `EVAL-10`, `EVAL-11`, `EVAL-13`, `EVAL-14`, `SERVE-05`, `SERVE-10`, `SERVE-12`, `DATA-01`, `DATA-05`.

**Opus (`claude-opus-4-1` or the newest Opus)** — well-specified engineering with a non-trivial blast radius: transactions, concurrency, id-space folding, API plumbing, schema migrations without a semantic decision.
`GO-02`, `GO-03`, `GO-04`, `GO-05`, `GO-06`, `GO-07`, `GO-08`, `GO-09`, `IMPORT-03`, `IMPORT-07`, `IMPORT-08`, `IMPORT-11`, `IMPORT-12`, `IMPORT-13`, `SERVE-01`, `SERVE-02`, `SERVE-03`, `SERVE-04`, `SERVE-06`, `SERVE-07`, `SERVE-08`, `EVAL-08`, `EVAL-12`, `EVAL-16`, `DATA-02`, `DATA-03`, `DATA-04`, `DATA-06`, `OPS-01`, `OPS-02`.

**Sonnet (newest Sonnet)** — mechanical, single-file, spec is the whole fix.
`GO-10` … `GO-16`, `IMPORT-10`, `IMPORT-14` … `IMPORT-18`, `SERVE-09`, `SERVE-11`, `SERVE-13`, `EVAL-15`, `DATA-07`, `DATA-08`, `DATA-09`, `OPS-03`.

**Dependencies that fix the order regardless of rank.** `IMPORT-03` (idempotent re-import) should land *before* any other `IMPORT-*` change, otherwise those fixes never reach already-imported matches. `IMPORT-02` before `FEAT-04`. `IMPORT-04` before `FEAT-08`. `SERVE-08` together with `GO-01` (or `as_of` becomes a freshness bypass). `SERVE-02` together with `GO-04`. `IMPORT-08` and `DATA-02` should agree on one venue key. Batch all **retrain** findings you intend to land this cycle, then run `make retrain` and `make evaluate` once at the end rather than once per PR; the harness (~1 h) is the acceptance test for the batch.

The orchestration prompt that drives this list is in `docs/AUDIT_FIX_RUNBOOK.md`.

---

## 2. Rating pass and features (`ml-service/ml/xi/`)

### FEAT-01 — `exp_balls_faced` / `exp_balls_bowled` use the wrong denominator  **High · retrain**

`ml/xi/ratings.py:236-240`
```python
bat_m = self.bat_matches[f, s]
bowl_m = self.bowl_matches[f, s]
"exp_balls_faced": np.where(bat_m > 0, self.bat_balls[f, s] / np.maximum(bat_m, 1e-9), 0.0),
"exp_balls_bowled": np.where(bowl_m > 0, self.bowl_balls[f, s] / np.maximum(bowl_m, 1e-9), 0.0),
```
`bat_matches` / `bowl_matches` are incremented (and decayed) only in `_accumulate` (`ratings.py:515-522`), i.e. only for players who actually faced/bowled a ball. A #11 who batted once for 30 balls in his last 20 matches reads `exp_balls_faced = 30`, the same as an opener who faces 30 every game. `aggregate_side` (`ratings.py:610-611`) weights `bat_rate * ebf`, so tailenders get opener-level involvement in `imp_bat_sum` / `imp_bat_top6`. For bowling, a part-timer who bowled 2 overs once reads `exp_balls_bowled = 12`, exactly `MIN_BOWLING_BALLS["T20"]` (`contract.py:41`), so `is_bowling_option` (`contract.py:44-48`) counts him as a bowler in the `n_bowlers` feature **and** in the optimiser's bowling-cover constraint. The correct denominator `xi_n` already exists and is used for `bat_innings_share` (`ratings.py:251`).

**Fix.** Define involvement per XI appearance: `bat_balls / xi_n` and `bowl_balls / xi_n`, and decay `bat_balls` / `bowl_balls` on every XI appearance (as `bat_pos_sum` / `xi_n` already do, `ratings.py:376-378`) so numerator and denominator share one decay clock. Test: a player in 10 XIs who batted once for 30 balls must read `exp_balls_faced ≈ 3`, not 30.

### FEAT-02 — Sides of 12–13 are rated and aggregated as full squads  **Medium · retrain**

`ratings.py:364-366, 372-378, 403-405`; `rows.py:146-149`; `builder.py:147-149` only counts oversized sides (1,358 of 45,468 per `quality.py:70`). Training rows for those matches sum `imp_bat_sum`, `n_bowlers`, `exp_balls_faced_sum`, `n_debutants` over 12–13 players while serving (`store.py:391-392`) always aggregates 11 — an upward bias on ~3 % of rows. A concussion replacement is decided *during* the match, so his presence is post-start information. Player Elo rewards 12 players.

**Fix.** Cricsheet carries `info.replacements`; persist an `is_replacement` flag on `match_player` (importer change) and exclude replacements from `team*_players`; or truncate to the first eleven of `info.players` on both sources and assert parity counts agree.

### FEAT-03 — Decided match with no deliveries emits 22 all-zero player rows  **Medium · retrain**

`builder.py:75-78` builds rows whenever `match.outcome is not None`; `rows.py:34-35` returns an empty actuals dict for empty deliveries and `rows.py:211-213` fills every player from `_ZERO_ACTUALS`. Every player gets `runs=0, balls_faced=0, wickets=0` as a genuine label; the win row carries `innings1_runs = 0`, which E2 then scores simulated totals against.

**Fix.** When `len(match.deliveries) == 0` and the match is decided, emit the win row (the label is real) but no player rows and no innings outcomes; count these in `DataQuality`.

### FEAT-04 — Postgres source hard-codes `result=None`  **Medium · retrain (with IMPORT-02)**

`sources.py:570-571` sets `result=None`; `ratings.py:406-408` appends 0.5 to `team_results` on `"tie"`/`"draw"`. Roughly a quarter of Tests are draws, so `team_form_diff` (a display-model column) has a different definition on the production source than on the archive path the parity tooling reads. `make xi-parity` compares counts and key sets, not this column.

**Fix.** Persist `outcome.result` (IMPORT-02), read it in `_MATCH_SQL`, add it to the parity count set.

### FEAT-05 — No home-advantage and no toss feature  **Medium · retrain**

`ratings.py:301-317` — the only venue-side context is `venue_fam_diff` (log1p of raw `(team, venue)` match counts). `venue.country` (`0001_baseline.sql:709`) and `toss_winner_opposition_id` / `toss_decision` (`0001_baseline.sql:482-483`) exist but `_MATCH_SQL` (`sources.py:423-438`) reads neither. Home advantage is the largest non-strength effect in international cricket; "chose to bat" vs "was made to bat" are different populations of batting-first sides.

**Fix.** Derive an as-of home flag (team country = venue country, team country inferred as-of from the modal venue country of past home fixtures or a reviewed config) and a `toss_won_by_team1` column; both are at-toss facts (H-21 compliant) and can be marginalised at serving the way batting order is.

### FEAT-06 — Ratings siloed per format; cold start on every crossover  **Medium · retrain**

`ratings.py:234-258` — every accumulator is `[f, s]`; a player with no history in the format reads the neutral vector (`bat_rate = 0`, `pelo = 1500`, counted as a debutant in `n_debutants`, `ratings.py:634`). A 100-cap T20I player making his IPL debut is an unknown. `PRIOR_BALLS=60` shrinks toward *zero* impact, not toward the player's own cross-format estimate.

**Fix.** Hierarchical shrinkage: shrink the format rate toward the pooled T20+T20I (or all-format) rate instead of zero. Only `side_vectors` changes.

### FEAT-07 — Batter's dismissal rate charges every wicket on the ball to the striker  **Low · retrain**

`ratings.py:446-447` uses `d.wicket` (any dismissal on the ball, `sources.py:44`); a non-striker run-out or "retired hurt" lowers the striker's `bat_wrate`, while the `dismissals` target (`rows.py:66-70`) uses `player_out`. Feature and target are defined on different events. **Fix.** Use `(d.player_out == d.batter)` as the batter's indicator.

### FEAT-08 — Bowler's "runs saved" and `runs_conceded` include byes and leg-byes  **Low · retrain (with IMPORT-04)**

`ratings.py:457` (`exp_runs - d.runs_total`), `rows.py:56`. `_BALLS_SQL` (`sources.py:481-496`) does not select `extras_kind` / `runs_extras`. Keeper-quality-correlated noise on `bowl_rate` and on the `runs_conceded` target. **Fix.** After IMPORT-04, subtract byes/leg-byes/penalty in the bowler's ledger; the JSON path has `extras: {byes, legbyes}` per delivery.

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

### EVAL-01 — Display model early stopping is `'auto'`  **High · retrain**

`train.py:89-98` builds `HistGradientBoostingClassifier(...)` with no `early_stopping` argument. In 1.5.2 `'auto'` enables early stopping iff `n_samples > 10000`, with a **random, shuffled 10 %** validation split. T20 has >10k rows; the others do not. So: `choose_display_params` fits candidates on the inner 80 % (<10k → early stopping off, full `max_iter`) then `train_format` refits the winner on all rows (>10k → early stopping on, far fewer iterations). The grid scores a model that is not the one fitted; `max_iter` in `DISPLAY_GRID` is not honoured for T20; walk-forward folds cross the 10k line mid-sequence so early and late folds fit different regimes; 10 % of the most recent rows are dropped from the final fit.

**Fix.** Pass `early_stopping=False` explicitly (the grid already controls `max_iter`), or `early_stopping=True` everywhere with a deterministic temporal split built by hand. Record `n_iter_` in the manifest. Test: assert `model.n_iter_ == max_iter` after `train_format` on a >10k-row synthetic frame.

### EVAL-02 — Three seeds produce bit-identical models below 10k rows  **High**

`train.py:231, 244-246`; `evaluate.py:191, 203-205`; `gates.py:243-245, 264-266, 319-321`. `random_state` in HGB only affects binning subsampling (n > 200k) and the early-stopping split. With early stopping off (T20I, ODI, TEST) the three seeds are identical, `display_auc_seed_sd = 0.0`, and gates X-3, B-7-display-monotone and X-2 that require a change to exceed "the control's seed-to-seed sd" have a zero floor that never binds. For T20 the "seed spread" is just the spread of the random early-stopping split.

**Fix.** Either drop the seed loop, or make seeds do something (row bagging via bootstrap `sample_weight`), and base the noise floor on fold-level paired SE only.

### EVAL-03 — Served performance model never trains on the last 92 days  **High · retrain**

`performance.py:542-556`:
```python
hold_out = bool(spec.recalibrate) or spec.shared_factor      # True in production (SHARED_FACTOR = True, simulator.py:62)
fit_rows, calibration_rows = _temporal_calibration_split(rows) if hold_out else ...
members = [_fit_member(x, fit_rows, spec, seed) for seed in spec.seeds]
```
The calibration fold is needed to fit the shared factor, but the members are never refitted on the full history afterwards. Served quantiles are ~3 months stale relative to the ratings they read. The harness has the same structure so it cannot see the cost.

**Fix.** Fit the shared factor / recalibration on the temporal fold, then refit the members on all rows and attach the fold-fitted parts (standard calibrate-on-fold, refit-on-full); or cross-fit over two folds.

### EVAL-04 — No gate can fail in code  **High**

`gates.py:532-553` `check_report` verifies only that a report **path exists** per registered gate; the `decides` text (H-17 "AUC ≥ 0.65", H-4 "< 2 %", E2 tolerance, H-5) is prose nothing evaluates. `evaluate.main` (`evaluate.py:463-465, 617-626`) returns non-zero only for parity/registry problems; `retrain.main` (`retrain.py:276-299`) only on the data-quality gate. `OPTIMISED_SELECTION_FORMATS` (`optimizer.py:57`) and `SIMULATED_WIN_PROBABILITY_DISPLAYED` are hand-set constants, so a fold AUC dropping below 0.65 changes nothing served.

**Fix.** Encode each gate's threshold beside its `report_path` and evaluate it in `check_report`; in retrain, refuse to write a manifest (or write `usable: false`) when holdout objective AUC is below the base-rate equivalent or below the previous accepted run minus a margin.

### EVAL-05 — Manifest `objective_auc` is toss-known; harness's is marginalised  **Medium**

`train.py:235-236, 243-247` uses `_score(model, x_te, y_te)` on actual batting order; `evaluate.py:192-193, 201-203` uses `_score_marginalised`. Same glossary key, and H-17's criterion reads the harness key. Toss-known is systematically more optimistic than what `/xi/predict-win` serves without `team1_bats_first`. `objective_marginalised` is computed at `train.py:241` but not promoted. **Fix.** Make the manifest headline the marginalised numbers, or add `_toss_known` keys.

### EVAL-06 — Harness never runs the grid  **Medium**

`evaluate.py:191` → `make_display_model(cols, seed)` → `params=None` → `DISPLAY_GRID[0]`. If retrain picks grid point 1 or 2, the served model has no walk-forward evidence. The inner split is by row fraction (`train.py:116-117`), not a date window. **Fix.** Run `choose_display_params(train)` inside `_evaluate_win_window` and record the per-fold pick; or freeze the grid.

### EVAL-07 — Served display artifact is seed 0, headline is the seed mean  **Medium**

`train.py:272` (`display=display_models[0]`) vs `:244`. With EVAL-01, for T20 the served model is one random early-stopping split. **Fix.** Persist all members and average at serving (as the performance model does), or report seed-0's score.

### EVAL-08 — Latent crash in locked-window recalibration; duplicate `np.interp` knots  **Medium**

`perf_calibration.py:54-55` raises below `MIN_ROWS`; `performance.py:558-560` calls `QuantileRecalibration.fit` unguarded; `evaluate.py:340-350` applies fold-decided recalibration to the locked window. A sparse format with a thin 92-day fold aborts the whole `evaluate` run. Separately `knots_x` are bin means of the predicted quantile; q10 predictions are often exactly 0 (`performance.py:361`), producing duplicate `xp` for which `np.interp` is undefined. **Fix.** Guard on `len(calibration_rows) >= MIN_ROWS`; use `IsotonicRegression(increasing=True)`.

### EVAL-09 — Performance-model early stopping splits rows of one match across both sides  **Medium · retrain**

`performance.py:83, 264-271`: `early_stopping=True, validation_fraction=0.1` shuffled. Player rows within a match share `own_*`, `opp_*`, `venue_*`, `elo_edge`, so the validation set is near-duplicate of training and the stopping point is optimistic. **Fix.** Group by match (`GroupShuffleSplit`) or temporal; choose `max_iter` on a grouped temporal split and fit with `early_stopping=False`.

### EVAL-10 — H-8 parity does not cover the saved artifact or `XiStore`  **Medium**

`asof.py:112-230` compares `build_match_rows` against itself from two states. It never round-trips `save_ratings → XiStore.load`, never calls `display_probability` / `win_probability` (which assemble the row via `row.get(c, 0.0)` and `serving_match` with `state.last_date`, `store.py:394-406`), and never exercises the registry-id contract go-app sends. D-6 (pickle shape drift) and B-7 (column-list drift) would pass this check. **Fix.** For the same 50 matches, compare `XiStore.display_probability` on a store loaded from a freshly written run directory against `marginalised_probabilities` on the frame rows.

### EVAL-11 — No untouched holdout  **Medium (validity)**

`evaluate.py:91-103, 116-120`: `LOCKED_START = "2026-09-02"` — days of matches. All ~17 gates (`gates.py`) were decided on the same eleven quarterly folds, so fold means are development-set scores under heavy multiple comparison. **Fix.** Hold back a full season no gate script may read; report it once per release; state beside every fold number how many gates consulted the folds.

### EVAL-12 — Manifest cannot reproduce a run  **Medium**

`runs.py:153-174` shells out to `git rev-parse HEAD`; the image (`ml-service/Dockerfile:11-35`) has no git and no `.git`, so `git_sha=""` on every served run. Missing: library versions, objective `C=0.3`, display `l2_regularization` / `min_samples_leaf`, `DISPLAY_SEEDS`, the performance `FitSpec`, source type. `dataset_sha` hashes `match_id|match_date` of decided matches only, so a re-import that changes squads or deliveries yields an identical sha. **Fix.** Inject `GIT_SHA` as build arg/env; add `library_versions`, `FitSpec.as_dict()` and the win-model constants; extend the digest with per-match delivery/squad counts.

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

### SERVE-01 — Every route is `async def` running CPU-bound work inline  **High**

`app/main.py:548-552, 557-564, 569-576, 586-599, 334-356`; `app/xi_service.py:264-271`. `optimize` (up to 200k model evaluations), `simulate` (up to 20k draws × 22 players), `_reload_run` and the as-of sweep (a sequential pass over `ball_event`, under `self._lock`) all run synchronously inside coroutines. Nothing else — including `/health` — is served until they return. A backtest going backwards in date rebuilds the pass from scratch (`asof.py:91-94`) on the loop thread. If the routes are later flipped to `def` (threadpool), the as-of path becomes a genuine race: `store.with_state(state)` hands out the live, mutating `AsOfRatings.state`, and `_read_slots` grows arrays on the read path (`ratings.py:211-213`).

**Fix.** Run prediction functions via `asyncio.to_thread` (or declare the routes `def`), make `store_as_of` snapshot/freeze the state before returning it, and bound the as-of sweep off the request path.

### SERVE-02 — Request models accept any side size, duplicates and overlap  **High**

`app/models/xi.py:39, 148-149, 294`: `team1_player_ids: List[str] = Field(..., min_length=1)` — no max, no uniqueness, no disjointness. Consequences: `simulator.py:585-590` reports 10 wickets once a 6-player side is all out; `/xi/predict-win` on 9 players returns a plausible P(win) for a different question; `/xi/optimize` with a duplicated pool id can choose the same player twice (`optimizer.py:156-157, 202-219` — `key_index` keeps the last position, `_greedy_seed` iterates positions); a player in both `pool_player_ids` and `opponent_player_ids` optimises a side against itself. `XiConstraints.team_size` allows 1–15 (`xi.py:30`) though the models only know 11.

**Fix.** Pydantic validators: exact length (`team_size`, default 11) for win/performance/simulate lists; `len(set(ids)) == len(ids)`; disjointness between sides and between pool and opponent; reject `team_size != 11` unless explicitly supported. Pair with GO-04.

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

### IMPORT-01 — Super overs imported as ordinary innings 3 and 4  **Critical · retrain**

`cricsheet.go:143-146` — `Innings{Team, Overs}` decodes neither `super_over` nor `declared`/`forfeited`/`target`; `ingest.go:333-334` numbers every entry `i+1`; `ball_event_emit.go:27-28` likewise. `grep super_over` across `go-app/` and `ml-service/` returns nothing. A tied T20/ODI with a super over writes `match_inning` rows 3 and 4, batting/bowling rows for the six balls, and `ball_event` rows with `innings = 3/4`. The rating pass then counts those deliveries as career balls, runs and dismissals (`rows.py:41-57`), adds them to `ctx_*` over-0 baselines and venue scoring (`ratings.py:466-468, 476-489`), gives a super-over batter who did not bat in the main innings a `batting_position` of 1–3 (`sources.py:63-73`), and emits `innings3_runs` / `innings4_runs` (`rows.py:91-92`). Every super over in the archive (IPL, BBL, CPL, T20Is) is polluted.

**Fix.** Add `SuperOver bool \`json:"super_over"\`` (and `Declared`, `Forfeited`, `Target *struct{Overs, Runs}`) to `Innings`; skip super-over innings in both the aggregate loop and `BuildBallEventRows`, or persist `is_super_over` on `match_inning` / `ball_event` and exclude in `_MATCH_SQL` / `_BALLS_SQL` and the JSON source (`sources.py:216-234`). Test: a fixture file with a super over must produce exactly two `match_inning` rows and no ball_event with `innings > 2`.

### IMPORT-02 — `outcome.result` / `eliminator` / `bowl_out` / `method` not decoded  **High · retrain**

`cricsheet.go:132-142`; no `result` column in `match` (`0001_baseline.sql:475-492`). A super-over win is `outcome: {result: "tie", eliminator: "<team>"}` with no `winner`, so it lands as `outcome_winner_opposition_id = NULL` — the same as "no result", "draw", abandoned and unresolved tie. Those matches are dropped from the frame and never update Elo (`sources.py:106-113` treats NULL winner as excluded, which is correct given the data, but the data is wrong). Drawn Tests get no form update on the Postgres path (FEAT-04). `method` ("D/L") is not stored so no reader can tell which chases were adjusted.

**Fix.** Add `Result`, `Method`, `Eliminator`, `BowlOut` to `Outcome`; migration adding `match.result varchar(16)`, `match.result_method varchar(16)`; set `outcome_winner_opposition_id` from `winner`, else `eliminator`, else `bowl_out`; have `_MATCH_SQL` read `result`.

### IMPORT-03 — Re-import is not idempotent for `ball_event` and the aggregate tables  **High · re-import**

`repo_ball_event.go:362, 416`: `ON CONFLICT (match_id, innings, "over", ball) DO NOTHING`. `ReplaceMatchPlayersTx` and `DeleteFieldingEventsForMatchTx` (`ingest.go:732-738`, `repo_fielding_event.go:105-110`) implement replace-on-reimport; `ball_event` does not. A corrected Cricsheet file (republished under the same id, `docs/config-and-data.md:457-459`) changes nothing; a match whose innings shrank keeps ghost rows; an importer fix (IMPORT-01, IMPORT-04) never reaches an already-imported match without a manual truncate. `batting_data` / `bowling_data` / `fielding_data` / `match_inning` are `DO UPDATE` so values refresh but vanished rows persist. No FK from `ball_event` to `match` (`0003_match_player.sql:55-57`) is what lets ghosts outlive a match.

**Fix.** `DELETE FROM ball_event WHERE match_id = $1` (and the same for the aggregate tables) at the top of the per-file transaction, mirroring `ReplaceMatchPlayersTx`; drop `ON CONFLICT`. Document that every importer fix requires a re-import of the whole directory.

### IMPORT-04 — Byes, leg-byes and penalty runs charged to the bowler; breakdown not stored  **High · retrain**

`ingest.go:389, 516-519`: `b.Runs += tr` where `tr = d.Runs.Total`. `ball_event` (`0001_baseline.sql:57-60`) stores `runs_batter`, `runs_extras`, `runs_total` and one `extras_kind` chosen by precedence (`ball_event_emit.go:76-93`), so a no-ball with 4 leg-byes is `extras_kind='no_ball', runs_extras=5` and the leg-byes are unrecoverable. The rating pass computes `runs_conceded = Σ runs_total` (FEAT-08).

**Fix.** Add `extras_wides / noballs / byes / legbyes / penalty smallint` (or `runs_bowler = total − byes − legbyes − penalty`) to `ball_event`; in `ingest.go` compute `b.Runs += tr - byes - legbyes - penalty`.

### IMPORT-05 — No-balls excluded from batter's balls faced; ML counts wides as faced  **Medium · retrain**

`ingest.go:392-396, 499-504`: the same `legal := wides == 0 && noballs == 0` is used for the bowler's balls (correct) and the batter's (wrong — a no-ball is faced, a wide is not). `batting_data.balls` and `strike_rate` are undercounted. Meanwhile the ML path (`rows.py:28, 41-49`; `ratings.py:481`) counts *every* delivery including wides as faced — a third definition. No test exercises a no-ball, a bye, a run-out or a super over (`grep` across `*_test.go` finds only the wide in `ingest_integration_test.go:38`).

**Fix.** `if wides == 0 { b.Balls++ }` for the batter; add a `faced` boolean to `ball_event` (or document `is_legal` as the bowler's definition); have the ML `balls_faced` exclude wides. Add fixture tests for each extras kind.

### IMPORT-06 — All wicket kinds credited to the bowler; retired-hurt counted as a wicket lost; only first wicket per ball stored  **Medium**

`ingest.go:399-400, 532-534`: `b.Wickets += len(*d.Wickets)` includes run-outs, retired-out, obstructing, timed-out; `match_inning.wickets_lost` includes `retired hurt` / `retired not out`. `ball_event_emit.go:94-110` keeps only `(*d.Wickets)[0]`. The ML applies `BOWLER_CREDITED_KINDS` (`sources.py:31`) for wickets but treats `retired hurt` as a dismissal (`sources.py:613`, `rows.py:64-68`, `ratings.py:482` MAX_WICKETS test).

**Fix.** Credited-kinds set for `b.Wickets`; exclude retired-hurt from `wkts` and from the ML dismissal target; a child table or `wicket_kind_2 / player_out_2` for the second wicket; normalise kinds at import.

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

### GO-04 — Same player can be in both pools and both XIs  **Medium**

`predict_team.go:429-440` loads pools independently; `xi_selection.go:355-364` `mergeMarginals` assumes "registry ids are unique across sides"; `selection_reason.go:151-161`; `xi_performance.go:75-84` keys one `byKey` for both sides and ignores `PlayerPerformance.side` (`ml_xi_client.go:467-473`); `play_mode.go:110-138` validates pinned XIs per side only. Franchise T20 with a 12-month window: a player who moved clubs is in both pools; alternating best response can select him for both sides. **Fix.** Drop intersection players from the club he played for less recently (by `LastPlayed`), pass the opposing XI as `must_exclude`, validate `team1_xi ∩ team2_xi = ∅`, key performance by `(side, player_id)`. Pair with SERVE-02.

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

### DATA-01 — Geocoder chooses a candidate in *any* voted country  **High (for the experiments)**

`ml/weather/geocoding.py:111-117`:
```python
rank = {country: i for i, country in enumerate(votes)}
return min(candidates, key=lambda c: (rank.get(c.country_code, len(rank)), -c.population))
```
`votes` is every country any visiting side ever came from, so when no candidate is in the top-voted country, the 7th-ranked one wins, and the `note` at `:141` stays empty because that country *is* somewhere in the votes. Verified in `reference-data/venue-geocoding.csv`: row 435 `M Chinnaswamy Stadium` → Bangalore Town, Sindh, **PK** (24.87, 67.08, Asia/Karachi; 111 cached days); row 684 `Shere Bangla National Stadium` → Mīrpur, Punjab, **IN** (129 days); row 850 `Warner Park, Basseterre` → South Australia (58 days; and `sessions.py:143-145` then applies Australia's 14:00 rule → `night=True`); rows 582/583 `Providence Stadium` → Guyana centroid / Rhode Island; 172 Coolidge → Arizona; 510/512 National Cricket Stadium Grenada → Wales; 797 Three Ws Oval → Jamaica; 876 Windsor Park Roseau → St Lucia; 652 Sabina Park → Jamaica interior; plus Suva → PG, Mantin → NP, Nisshin → KR, Sea Breeze Oval → Ohio, Somerset CC → Alabama. ≈590 venue-days carry another continent's ERA5 hours and clock, concentrated on Mirpur and Bengaluru. `reference-data/README.md:82` ("every row is a city-level fix") and `docs/EXTERNAL_DATA_PLAN.md:1067-1072` are wrong; X-2's gate tables (`EXTERNAL_DATA_PLAN.md:1100-1130`) were computed on this data. `make restore-venue-weather` writes these coordinates onto `venue` (`backfill.py:206-221`).

**Fix.** In `choose`, accept only candidates whose country is among the top-N votes (or has a minimum share), else return `None` so `locate` falls through to the next query / `unmappable`; write the note whenever the chosen country is not the top vote; add a test over the CSV asserting each mapped row's country equals its top vote unless `note` starts `hand-curated`. Re-place the rows above, re-fetch their ERA5 days, rerun X-2 for T20I/ODI.

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
